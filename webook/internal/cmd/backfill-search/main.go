// Command backfill-search 把存量 published_article 一次性回填进 webook-search 的 ES 索引。
//
// 用法（与 core 同款配置装载：APP_ENV 指向某份 config yaml）：
//
//	cd webook/internal && APP_ENV=config/local.yaml go run ./cmd/backfill-search
//
// 依赖运行中的 MySQL（源库）+ etcd（服务发现）+ webook-search / webook-tag gRPC。
// 幂等，可重复跑。完成后用 `make -f mk/es.mk count` 复核 ES 实际文档数。
package main

import (
	"context"
	"log"
	"time"

	"github.com/boyxs/train-go/webook/pkg/viperx"
)

func main() {
	if err := viperx.LoadLocal(); err != nil {
		panic(err)
	}
	b, cleanup, err := InitSearchBackfiller()
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// 存量大时给足时间；到点 context 取消，已写入的部分不回滚（幂等重跑即可续）。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := b.Run(ctx); err != nil {
		log.Fatalf("[backfill-search] %v", err)
	}
	log.Println("[backfill-search] 全部成功")
}
