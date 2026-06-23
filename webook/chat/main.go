package main

import (
	"log"

	"github.com/spf13/viper"

	"github.com/webook/chat/ioc"
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
	defer cleanup()

	// http.addr 由 yaml 提供；这里 fallback 仅在 yaml 漏配时兜底，避免 nil 监听
	addr := viper.GetString("http.addr")
	if addr == "" {
		addr = ":8020"
	}
	log.Printf("[chat] listening on %s", addr)
	if err := app.Server.Run(addr); err != nil {
		log.Fatalf("[chat] exit: %v", err)
	}
}
