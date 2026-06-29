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

	"github.com/webook/internal/ioc"
	"github.com/webook/pkg/viperx"
)

func main() {
	if err := viperx.LoadLocal(); err != nil {
		panic(err)
	}
	var cfg viperx.EtcdConfig
	if err := viper.UnmarshalKey("etcd", &cfg); err != nil {
		panic(err)
	}
	viperx.WatchRemote(cfg, func() {
		for _, fn := range ioc.ConfigChangeCallbacks {
			fn()
		}
	})

	app, cleanup, err := InitWebServer()
	if err != nil {
		panic(err)
	}

	// 监听 SIGINT / SIGTERM,收到信号后优雅停机
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 后台启动 Kafka 消费者
	go func() {
		if err := app.Consumer.Start(context.Background()); err != nil {
			log.Printf("[Kafka] consumer exited: %v", err)
		}
	}()
	// 后台启动 gRPC server,供下游 RPC 调用
	go func() {
		if err := app.GRPCServer.Register(); err != nil {
			log.Printf("[gRPC] etcd 注册失败: %v", err)
			return
		}
		log.Printf("[gRPC] serving on :%d", app.GRPCServer.Port)
		if err := app.GRPCServer.Serve(); err != nil {
			log.Printf("[gRPC] server exited: %v", err)
		}
	}()
	// HTTP server 用 http.Server(而非 engine.Run)以支持优雅关闭
	httpAddr := viper.GetString("http.addr")
	if httpAddr == "" {
		httpAddr = ":8010"
	}
	httpSrv := &http.Server{Addr: httpAddr, Handler: app.Server}
	go func() {
		log.Printf("[HTTP] serving on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[core][shutdown] 收到信号，开始优雅停机…")
	// gRPC 先于 cleanup 关闭:注销 etcd 端点 + GracefulStop(保留 OTel 上报)
	if err := app.GRPCServer.Close(); err != nil {
		log.Printf("[gRPC] 关闭: %v", err)
	}
	// HTTP:等在途请求处理完,最多 10s
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[HTTP] 关闭: %v", err)
	}
	cleanup() // wire cleanup(OTel TracerProvider.Shutdown 等)
	log.Println("[core][shutdown] 完成")
}
