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

	"github.com/boyxs/train-go/webook/chat/ioc"
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

	// 监听 SIGINT / SIGTERM，收到信号后优雅停机
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// HTTP server 用 http.Server（而非 engine.Run）以支持优雅关闭：SSE 长连在超时内排空
	// http.addr 由 yaml 提供；fallback 仅在 yaml 漏配时兜底，避免 nil 监听
	httpAddr := viper.GetString(confkey.ServerHTTPAddr)
	if httpAddr == "" {
		httpAddr = ":8020"
	}
	httpSrv := &http.Server{Addr: httpAddr, Handler: app.Server}
	go func() {
		log.Printf("[chat][HTTP] serving on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[chat][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[chat][shutdown] 收到信号，开始优雅停机…")
	// HTTP：等在途请求处理完（含 SSE 长连排空），最多 20s
	sctx, scancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[chat][HTTP] 关闭: %v", err)
	}
	cleanup() // wire cleanup：关到 core 的 gRPC 连接 / OTel 上报等
	log.Println("[chat][shutdown] 完成")
}
