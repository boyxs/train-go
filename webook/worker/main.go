package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/viper"

	"github.com/webook/pkg/viperx"
	"github.com/webook/shared/confkey"
)

// worker 调度器：cron 定时任务 + Kafka 消费者，全部经 gRPC 派发给业务服务，自身零业务数据/逻辑
// （对齐 bolee-task）。infra 装配在 worker/ioc + wire，main 只做加载配置 / 启动 / 优雅停机。
func main() {
	if err := viperx.LoadLocal(); err != nil {
		panic(err)
	}

	app, cleanup, err := InitApp()
	if err != nil {
		panic(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// cron 定时任务（触发 core 重算/归档）
	app.Cron.Start()
	log.Println("[worker] cron 已启动")

	// Kafka 消费者：read 事件 → interaction gRPC，自管连接，启动不依赖 Kafka。
	// consumerDone 让停机阶段能等它退出，避免 cleanup 关 interaction 连接时撞在途 IncrReadCount。
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		if err := app.Consumer.Start(ctx); err != nil {
			log.Printf("[worker][consumer] exited: %v", err)
		}
	}()

	// 最小 HTTP：/metrics + /health
	httpAddr := viper.GetString(confkey.ServerHTTPAddr)
	if httpAddr == "" {
		httpAddr = ":8050"
	}
	httpSrv := &http.Server{Addr: httpAddr, Handler: app.Web}
	go func() {
		log.Printf("[worker][HTTP] metrics/health on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[worker][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[worker][shutdown] 停机…")
	<-app.Cron.Stop().Done() // 等 in-flight cron 跑完
	// 等消费者 goroutine 退出（ctx 取消后 Start 跳出消费循环）再 cleanup：
	// 否则 cleanup 关 interaction gRPC 连接会撞上在途的 handleBatch→IncrReadCount。
	// 有界等待，极端卡死不无限阻塞停机。
	select {
	case <-consumerDone:
	case <-time.After(10 * time.Second):
		log.Println("[worker][shutdown] 消费者退出超时，强制继续")
	}
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[worker][HTTP] 关闭: %v", err)
	}
	cleanup() // wire cleanup：关 etcd / 下游 gRPC 连接
	log.Println("[worker][shutdown] 完成")
}
