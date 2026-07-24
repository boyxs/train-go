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

	"github.com/boyxs/train-go/webook/pkg/viperx"
	"github.com/boyxs/train-go/webook/shared/confkey"
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

	// Kafka 消费者：read→interaction、article/relation→feed，自管连接，启动不依赖 Kafka。
	// 各自 done channel 让停机阶段能等其退出，避免 cleanup 关下游 gRPC 连接时撞在途 RPC。
	consumers := []struct {
		name  string
		start func(context.Context) error
	}{
		{"interaction", app.Consumer.Start},
		{"feed-article", app.FeedArticleConsumer.Start},
		{"feed-relation", app.FeedRelationConsumer.Start},
	}
	consumerDones := make([]chan struct{}, len(consumers))
	for i, c := range consumers {
		done := make(chan struct{})
		consumerDones[i] = done
		go func(name string, start func(context.Context) error, done chan struct{}) {
			defer close(done)
			if err := start(ctx); err != nil {
				log.Printf("[worker][consumer:%s] exited: %v", name, err)
			}
		}(c.name, c.start, done)
	}

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
	log.Println("[worker][shutdown] 收到信号，开始优雅停机…")
	<-app.Cron.Stop().Done() // 等 in-flight cron 跑完
	// 等所有消费者 goroutine 退出（ctx 取消后 Start 跳出消费循环）再 cleanup：
	// 否则 cleanup 关下游 gRPC 连接会撞上在途的 handleBatch→RPC。有界等待，极端卡死不无限阻塞停机。
	consumersDrained := make(chan struct{})
	go func() {
		for _, done := range consumerDones {
			<-done
		}
		close(consumersDrained)
	}()
	select {
	case <-consumersDrained:
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
