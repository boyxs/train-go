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

	"github.com/webook/migrator/ioc"
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

	// 监听 SIGINT / SIGTERM，收到信号后优雅停机
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// HTTP server 用 http.Server（而非 engine.Run）以支持优雅关闭
	// http.addr 由 yaml 提供；fallback 仅在漏配时兜底（migrator 走运维段 :8200，与业务 80xx 段错开）
	httpAddr := viper.GetString("http.addr")
	if httpAddr == "" {
		httpAddr = ":8200"
	}
	httpSrv := &http.Server{Addr: httpAddr, Handler: app.Server}
	go func() {
		log.Printf("[migrator][HTTP] serving on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[migrator][HTTP] exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[migrator][shutdown] 收到信号，开始优雅停机…")
	// HTTP：等在途请求处理完，最多 10s（运行中的迁移任务有 checkpoint，crash-safe，由 cleanup 收尾）
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	if err := httpSrv.Shutdown(sctx); err != nil {
		log.Printf("[migrator][HTTP] 关闭: %v", err)
	}
	cleanup() // wire cleanup：停迁移引擎 / 关 canal / OTel 上报等
	log.Println("[migrator][shutdown] 完成")
}
