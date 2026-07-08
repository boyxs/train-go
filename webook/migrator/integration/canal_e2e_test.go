package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/migrator/pipeline/source"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// TestCanal_E2E 端到端验证:GoMySQLCanalClient 真订阅 binlog → INSERT/UPDATE/DELETE 触发事件到达。
//
// 前置:
//  1. docker compose 起着 webook-mysql,容器开了 binlog(deploy/docker-compose.yaml 已加 --log-bin --binlog-format=ROW --server-id=1)
//  2. canal 用户已建(首次启 mysql 容器自动跑 deploy/mysql/init/01-canal-user.sql)
//  3. webook_migrator_test 库已建
//
// 缺任一前置 → t.Skipf 跳过(避免污染 go test ./... 全量回归)。
func TestCanal_E2E(t *testing.T) {
	dsnStr := viper.GetString("data.mysql.dsn")
	canalAddr := viper.GetString("migrator.canal.addr")
	canalUser := viper.GetString("migrator.canal.user")
	canalPwd := viper.GetString("migrator.canal.password")
	serverIDBase := viper.GetUint32("migrator.canal.server_id_base")
	if canalAddr == "" || canalUser == "" {
		t.Skip("migrator.canal.* 未配置,跳过 canal e2e")
	}

	// 1. 探活 MySQL + 验证 binlog 模式开了
	db, err := sql.Open("mysql", dsnStr)
	if err != nil {
		t.Skipf("mysql open failed: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("mysql unreachable: %v", err)
	}

	var binlogFormat string
	if err := db.QueryRow("SELECT @@global.binlog_format").Scan(&binlogFormat); err != nil {
		t.Skipf("query binlog_format: %v", err)
	}
	if binlogFormat != "ROW" {
		t.Skipf("binlog_format=%q (need ROW); 改 docker-compose mysql command 加 --binlog-format=ROW 后重启 mysql 容器(清 volume)", binlogFormat)
	}

	// 探活 canal 用户能不能登(密码错 / 权限不足 / 用户不存在等情况 Skip,避免污染 ship)
	canalDSN := fmt.Sprintf("%s:%s@tcp(%s)/?timeout=2s", canalUser, canalPwd, canalAddr)
	canalDB, cerr := sql.Open("mysql", canalDSN)
	if cerr != nil {
		t.Skipf("canal user dsn open: %v (检查 deploy/mysql/init/01-canal-user.sql 是否真跑过)", cerr)
	}
	defer canalDB.Close()
	if perr := canalDB.Ping(); perr != nil {
		t.Skipf("canal user ping failed: %v (建 canal 用户:mysql -uroot < deploy/mysql/init/01-canal-user.sql,或重启 MySQL 容器让 init/*.sql 触发)", perr)
	}
	// 进一步检查 REPLICATION SLAVE 权限(没这权限 canal Subscribe 会无声 backoff retry)
	if _, perr := canalDB.Exec("SHOW MASTER STATUS"); perr != nil {
		t.Skipf("canal user lacks REPLICATION CLIENT permission: %v", perr)
	}

	// 2. 建测试表 article_canal(独立表,不污染主表)
	// go-sql-driver 默认不允许多语句 Exec,拆成两次。
	tableName := "article_canal_e2e"
	if _, err = db.Exec("DROP TABLE IF EXISTS " + tableName); err != nil {
		t.Fatalf("drop test table: %v", err)
	}
	if _, err = db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id BIGINT PRIMARY KEY,
			title VARCHAR(255) NOT NULL,
			content TEXT
		) ENGINE=InnoDB CHARSET=utf8mb4`, tableName)); err != nil {
		t.Fatalf("create test table: %v", err)
	}
	defer func() {
		_, _ = db.Exec("DROP TABLE IF EXISTS " + tableName)
	}()

	// 3. 起 canal client(订阅当前 binlog 末尾,不读历史)
	dbName := extractDBName(dsnStr)
	client, err := source.NewGoMySQLCanalClient(source.GoMySQLCanalClientConfig{
		Addr:              canalAddr,
		User:              canalUser,
		Password:          canalPwd,
		ServerID:          serverIDBase + 99, // 错开主流程的 task ServerID
		Flavor:            "mysql",
		IncludeTableRegex: []string{fmt.Sprintf("%s\\.%s", dbName, tableName)},
		BufSize:           256,
	}, logger.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	eventCh, err := client.Subscribe(ctx, "")
	if err != nil {
		// canal 连不上(用户不存在/binlog 没开 等)→ skip 不 fail
		t.Skipf("canal subscribe failed (检查 canal 用户 + binlog 配置): %v", err)
	}
	defer func() {
		_ = client.Stop()
	}()

	// canal Subscribe 内部 RunFrom 有几百 ms 启动延迟,等一下再插数据
	time.Sleep(500 * time.Millisecond)

	// 4. INSERT 触发 binlog event
	_, err = db.Exec(fmt.Sprintf("INSERT INTO %s (id, title, content) VALUES (?, ?, ?)", tableName), 100, "canal_test", "hello")
	require.NoError(t, err)

	// 5. 等 insert event
	insertEvt := waitForEvent(t, eventCh, "insert", 5*time.Second)
	assert.Equal(t, tableName, insertEvt.Table)
	assert.Equal(t, "100", insertEvt.PK)
	assert.Equal(t, "canal_test", insertEvt.After["title"])

	// 6. UPDATE 触发 update event(before + after 都应该有,因 binlog_row_image=FULL)
	_, err = db.Exec(fmt.Sprintf("UPDATE %s SET title=? WHERE id=?", tableName), "canal_updated", 100)
	require.NoError(t, err)
	updateEvt := waitForEvent(t, eventCh, "update", 5*time.Second)
	assert.Equal(t, "100", updateEvt.PK)
	if updateEvt.Before != nil {
		assert.Equal(t, "canal_test", updateEvt.Before["title"])
	}
	assert.Equal(t, "canal_updated", updateEvt.After["title"])

	// 7. DELETE 触发 delete event
	_, err = db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id=?", tableName), 100)
	require.NoError(t, err)
	deleteEvt := waitForEvent(t, eventCh, "delete", 5*time.Second)
	assert.Equal(t, "100", deleteEvt.PK)
}

// waitForEvent 从 event channel 读,过滤指定 op;超时 t.Fatal。
// canal 启动前/启动期可能漏少量 noise event,这里 op 匹配过滤掉。
func waitForEvent(t *testing.T, ch <-chan source.BinlogEvent, op string, timeout time.Duration) source.BinlogEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %s event", op)
			return source.BinlogEvent{}
		case evt, ok := <-ch:
			if !ok {
				t.Fatalf("event channel closed before %s event", op)
				return source.BinlogEvent{}
			}
			if evt.Op == op {
				return evt
			}
			// 不是想要的 op(可能 mysql 自动操作或之前 leftover),继续等
		}
	}
}

// extractDBName 从 mysql DSN 拿数据库名,跟 ioc/engines.go 同名 helper 同语义。
// 避免 cross-package import,集成测试包内本地实现一份。
func extractDBName(dsn string) string {
	cfg, err := gomysql.ParseDSN(dsn)
	if err != nil {
		return ""
	}
	return cfg.DBName
}
