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

	"github.com/boyxs/train-go/webook/pkg/viperx"
	"github.com/boyxs/train-go/webook/search/ioc"
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

	// gRPC server：search 的核心对外接口，供 core BFF / chat 调用
	go func() {
		if err := app.GRPCServer.Register(); err != nil {
			log.Printf("[search][gRPC] etcd 注册失败: %v", err)
			return
		}
		log.Printf("[search][gRPC] serving on %s", app.GRPCServer.Addr)
		if err := app.GRPCServer.Serve(); err != nil {
			log.Printf("[search][gRPC] server exited: %v", err)
		}
	}()

	// 最小 HTTP：仅 /metrics（Prometheus 抓取）+ /health（search 不直接对前端，业务走 gRPC）
	httpAddr := viper.GetString(confkey.ServerHTTPAddr)
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promhttp.Handler().ServeHTTP(w, r)
		log.Printf("[metrics] %s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
	}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"search"}`))
	})
	httpSrv := &http.Server{Addr: httpAddr, Handler: mux}
	go func() {
		log.Printf("[search][HTTP] metrics/health on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[search][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[search][shutdown] 收到信号，开始优雅停机…")
	if err := app.GRPCServer.Close(); err != nil {
		log.Printf("[search][gRPC] 关闭: %v", err)
	}
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[search][HTTP] 关闭: %v", err)
	}
	cleanup()
	log.Println("[search][shutdown] 完成")
}
