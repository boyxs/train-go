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

	"github.com/boyxs/train-go/webook/feed/ioc"
	"github.com/boyxs/train-go/webook/pkg/viperx"
	"github.com/boyxs/train-go/webook/shared/confkey"
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

	// gRPC server：feed 的核心对外接口，供 core BFF 读 + worker 写扩散调用
	go func() {
		if err := app.GRPCServer.Register(); err != nil {
			log.Printf("[feed][gRPC] etcd 注册失败: %v", err)
			return
		}
		log.Printf("[feed][gRPC] serving on %s", app.GRPCServer.Addr)
		if err := app.GRPCServer.Serve(); err != nil {
			log.Printf("[feed][gRPC] server exited: %v", err)
		}
	}()

	// 最小 HTTP：仅 /metrics（Prometheus 抓取）+ /health（feed 不直接对前端，业务走 gRPC）
	httpAddr := viper.GetString(confkey.ServerHTTPAddr)
	if httpAddr == "" {
		httpAddr = ":8100"
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"feed"}`))
	})
	httpSrv := &http.Server{Addr: httpAddr, Handler: mux}
	go func() {
		log.Printf("[feed][HTTP] metrics/health on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[feed][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[feed][shutdown] 收到信号，开始优雅停机…")
	if err := app.GRPCServer.Close(); err != nil {
		log.Printf("[feed][gRPC] 关闭: %v", err)
	}
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[feed][HTTP] 关闭: %v", err)
	}
	cleanup()
	log.Println("[feed][shutdown] 完成")
}
