package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
)

func main() {
	c := cron.New(cron.WithSeconds(), cron.WithLogger(
		cron.VerbosePrintfLogger(log.New(os.Stdout, "[cron] ", log.LstdFlags)),
	))

	// 每 5 秒执行一次
	if _, err := c.AddFunc("*/5 * * * * *", func() {
		fmt.Printf("[tick-5s] %s\n", time.Now().Format("15:04:05"))
	}); err != nil {
		log.Fatalf("add tick-5s: %v", err)
	}

	// 每分钟第 0 秒执行一次
	if _, err := c.AddFunc("0 * * * * *", func() {
		fmt.Printf("[tick-1m] %s\n", time.Now().Format("15:04:05"))
	}); err != nil {
		log.Fatalf("add tick-1m: %v", err)
	}

	c.Start()
	log.Println("cron started, Ctrl+C to stop")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	ctx := c.Stop()
	<-ctx.Done()
	log.Println("cron stopped")
}
