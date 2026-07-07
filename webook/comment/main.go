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

	"github.com/webook/comment/ioc"
	"github.com/webook/pkg/viperx"
	"github.com/webook/shared/confkey"
)

func main() {
	if err := viperx.LoadLocal(); err != nil {
		panic(err)
	}
	var cfg viperx.EtcdConfig
	if err := viper.UnmarshalKey(confkey.Etcd, &cfg); err != nil {
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

	// gRPC server：comment 的核心对外接口，供 core 调用
	go func() {
		if err := app.GRPCServer.Register(); err != nil {
			log.Printf("[comment][gRPC] etcd 注册失败: %v", err)
			return
		}
		log.Printf("[comment][gRPC] serving on %s", app.GRPCServer.Addr)
		if err := app.GRPCServer.Serve(); err != nil {
			log.Printf("[comment][gRPC] server exited: %v", err)
		}
	}()

	// 最小 HTTP：仅 /metrics（Prometheus 抓取）+ /health（comment 不直接对前端，业务走 gRPC）
	httpAddr := viper.GetString(confkey.ServerHTTPAddr)
	if httpAddr == "" {
		httpAddr = ":8030"
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// health 探测响应，写失败无副作用（探测方会按超时/状态码判定），无需处理 err
		_, _ = w.Write([]byte(`{"status":"ok","service":"comment"}`))
	})
	httpSrv := &http.Server{Addr: httpAddr, Handler: mux}
	go func() {
		log.Printf("[comment][HTTP] metrics/health on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[comment][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[comment][shutdown] 收到信号，开始优雅停机…")
	// gRPC 先关：注销 etcd 端点 + GracefulStop
	if err := app.GRPCServer.Close(); err != nil {
		log.Printf("[comment][gRPC] 关闭: %v", err)
	}
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[comment][HTTP] 关闭: %v", err)
	}
	cleanup()
	log.Println("[comment][shutdown] 完成")
}
