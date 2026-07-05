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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"

	"github.com/webook/interaction/ioc"
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

	app, cleanup, err := InitApp()
	if err != nil {
		panic(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// gRPC server：interaction 的核心对外接口，供 core / chat 调用
	go func() {
		if err := app.GRPCServer.Register(); err != nil {
			log.Printf("[interaction][gRPC] etcd 注册失败: %v", err)
			return
		}
		log.Printf("[interaction][gRPC] serving on %s", app.GRPCServer.Addr)
		if err := app.GRPCServer.Serve(); err != nil {
			log.Printf("[interaction][gRPC] server exited: %v", err)
		}
	}()

	// 最小 HTTP：仅 /metrics（Prometheus 抓取）+ /health（interaction 不直接对前端，业务走 gRPC）
	httpAddr := viper.GetString("server.http.addr")
	if httpAddr == "" {
		httpAddr = ":8040"
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// health 探测响应，写失败无副作用（探测方会按超时/状态码判定），无需处理 err
		_, _ = w.Write([]byte(`{"status":"ok","service":"interaction"}`))
	})
	httpSrv := &http.Server{Addr: httpAddr, Handler: mux}
	go func() {
		log.Printf("[interaction][HTTP] metrics/health on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[interaction][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[interaction][shutdown] 收到信号，开始优雅停机…")
	// gRPC 先关：注销 etcd 端点 + GracefulStop
	if err := app.GRPCServer.Close(); err != nil {
		log.Printf("[interaction][gRPC] 关闭: %v", err)
	}
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[interaction][HTTP] 关闭: %v", err)
	}
	cleanup()
	log.Println("[interaction][shutdown] 完成")
}
