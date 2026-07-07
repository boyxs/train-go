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

	"github.com/webook/pkg/viperx"
	"github.com/webook/relation/ioc"
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

	// gRPC server：relation 的核心对外接口，供 core / chat / 未来 feed 调用
	go func() {
		if err := app.GRPCServer.Register(); err != nil {
			log.Printf("[relation][gRPC] etcd 注册失败: %v", err)
			return
		}
		log.Printf("[relation][gRPC] serving on %s", app.GRPCServer.Addr)
		if err := app.GRPCServer.Serve(); err != nil {
			log.Printf("[relation][gRPC] server exited: %v", err)
		}
	}()

	// 最小 HTTP：仅 /metrics（Prometheus 抓取）+ /health（relation 不直接对前端，业务走 gRPC）
	httpAddr := viper.GetString(confkey.ServerHTTPAddr)
	if httpAddr == "" {
		httpAddr = ":8060"
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"relation"}`))
	})
	httpSrv := &http.Server{Addr: httpAddr, Handler: mux}
	go func() {
		log.Printf("[relation][HTTP] metrics/health on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[relation][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[relation][shutdown] 收到信号，开始优雅停机…")
	if err := app.GRPCServer.Close(); err != nil {
		log.Printf("[relation][gRPC] 关闭: %v", err)
	}
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[relation][HTTP] 关闭: %v", err)
	}
	cleanup()
	log.Println("[relation][shutdown] 完成")
}
