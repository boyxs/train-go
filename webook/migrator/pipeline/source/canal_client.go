package source

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	"github.com/go-mysql-org/go-mysql/schema"

	"github.com/webook/pkg/logger"
)

// 重连退避参数（指数退避：1s → 2s → 4s → ... → 30s 封顶）。
const (
	canalRetryInitialBackoff = time.Second
	canalRetryMaxBackoff     = 30 * time.Second
)

// GoMySQLCanalClient 是 BinlogClient 的 go-mysql canal SDK 实现。
// 直连 MySQL master 拉 binlog stream（要求 master my.cnf 配 server-id + binlog_format=ROW + binlog_row_image=FULL）。
//
// 重连机制：master 网络抖动 / 主从切换 / 短暂不可达时，自动指数退避（1s → 30s 封顶）+ 从最近位点续订。
// 重放期间 IncrEngine 拿到的事件 PK 可能重复，Sink 幂等（upsert + version 乐观锁）兜底。
// 续订位点来自 handler 实时跟踪的 OnRow / OnRotate；重连时拿最新位点 RunFrom。
type GoMySQLCanalClient struct {
	cfg GoMySQLCanalClientConfig
	l   logger.LoggerX

	canalSrv     atomic.Pointer[canal.Canal] // 重连后会换新实例,用 atomic 保证 Stop 安全
	out          chan BinlogEvent
	stopOnce     sync.Once
	stopped      chan struct{}
	lastPosOwner *canalEventHandler // 跟踪最新 binlog 位点(供重连续订用),Subscribe 内构造
}

// GoMySQLCanalClientConfig 构造 GoMySQLCanalClient 的参数。
//
//	Addr/User/Password 连接 master；ServerID 必须与所有其他 canal 实例错开（避免 binlog 抢同一槽位）；
//	IncludeTableRegex 限定订阅的表名（如 `webook\.article` 单表）。
type GoMySQLCanalClientConfig struct {
	Addr              string // host:port
	User              string
	Password          string
	ServerID          uint32 // canal 实例唯一 ID（同一 master 多实例时各自不同）
	Flavor            string // "mysql" / "mariadb"，默认 "mysql"
	IncludeTableRegex []string
	BufSize           int // 事件 channel buffer，默认 4096
}

// NewGoMySQLCanalClient 构造 BinlogClient（go-mysql canal SDK 实现）。
// Subscribe 启动 canal.Canal 并把 RowsEvent 转 BinlogEvent 推到内部 channel。
func NewGoMySQLCanalClient(cfg GoMySQLCanalClientConfig, l logger.LoggerX) (BinlogClient, error) {
	if cfg.Flavor == "" {
		cfg.Flavor = "mysql"
	}
	if cfg.BufSize <= 0 {
		cfg.BufSize = 4096
	}
	if cfg.ServerID == 0 {
		return nil, fmt.Errorf("ServerID 必须 > 0（与其他 canal 实例错开）")
	}
	return &GoMySQLCanalClient{
		cfg:     cfg,
		l:       l,
		stopped: make(chan struct{}),
	}, nil
}

// Subscribe 启动 binlog 订阅 + 自动重连循环,返回事件 channel。
//
// 行为：
//   - 首次连接：从 fromPos（"file/pos" 格式；空则从 master 当前位点）订阅
//   - 网络断开 / master 切换：指数退避（1s → 30s 封顶）+ 从最近收到事件的位点续订
//   - ctx cancel 或 Stop()：退出循环，close out channel
//
// 重放兜底：重连时从最新位点续订，期间可能重放少量已处理事件 — 上层 IncrEngine.Sink 必须幂等（upsert + version 乐观锁，当前已实现）。
func (c *GoMySQLCanalClient) Subscribe(ctx context.Context, fromPos string) (<-chan BinlogEvent, error) {
	c.out = make(chan BinlogEvent, c.cfg.BufSize)
	// handler 跨重连周期共享:lastPos 持续更新,重连用最新位点续订
	c.lastPosOwner = &canalEventHandler{out: c.out, l: c.l}
	c.lastPosOwner.initialPos.Store(fromPos)

	// 监听 ctx / stopped 触发 Stop（让当前 RunFrom 返回）
	go func() {
		select {
		case <-ctx.Done():
			c.Stop()
		case <-c.stopped:
		}
	}()

	// 重连循环
	go func() {
		defer close(c.out)
		backoff := canalRetryInitialBackoff
		attempt := 0
		for {
			if ctx.Err() != nil || c.isStopped() {
				return
			}
			attempt++
			startPos := c.lastPosOwner.snapshotPos() // 重连用最新位点
			if startPos == "" {
				startPos = fromPos // 首次还没收到任何事件,用初始位点
			}
			runErr := c.runOnce(ctx, startPos)
			if ctx.Err() != nil || c.isStopped() {
				return // 正常退出
			}
			// runOnce 异常 → 退避后重连
			c.l.Warn("canal disconnected, retry with backoff",
				logger.Int("attempt", attempt),
				logger.String("last_pos", startPos),
				logger.Error(runErr))
			select {
			case <-ctx.Done():
				return
			case <-c.stopped:
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > canalRetryMaxBackoff {
				backoff = canalRetryMaxBackoff
			}
		}
	}()
	return c.out, nil
}

// runOnce 跑一次 canal NewCanal + RunFrom,直到错误返回或 ctx cancel。
// 返回 nil 表示需要重连(底层 RunFrom 返回了);ctx cancel 时也返 nil 由上层判 ctx.Err()。
//
// 已知 race window(可接受):NewCanal 创建 srv → c.canalSrv.Store(srv) 之间(<1ms),
// 如果 Stop() 被并发调用,Stop 拿到 atomic.Pointer 内**旧** srv 关闭,新 srv 不被 Stop 直接 close。
// 但 select case `<-c.stopped` 会立即触发 srv.Close(),所以新 srv 不会真正进 RunFrom 业务循环。
// 极小概率 leak:新 srv 在 NewCanal 内已经建了 net.Conn 但 select 还没轮到 stopped case → conn 短暂泄露,
// 等 GC 或 OS TCP keepalive 回收。生产场景 Stop 调用极低频(graceful shutdown / 服务退出),可接受。
func (c *GoMySQLCanalClient) runOnce(ctx context.Context, fromPos string) error {
	canalCfg := canal.NewDefaultConfig()
	canalCfg.Addr = c.cfg.Addr
	canalCfg.User = c.cfg.User
	canalCfg.Password = c.cfg.Password
	canalCfg.ServerID = c.cfg.ServerID
	canalCfg.Flavor = c.cfg.Flavor
	canalCfg.IncludeTableRegex = c.cfg.IncludeTableRegex
	canalCfg.Dump.ExecutionPath = "" // 关闭 mysqldump（只跑 binlog 增量）

	srv, err := canal.NewCanal(canalCfg)
	if err != nil {
		return fmt.Errorf("canal new: %w", err)
	}
	c.canalSrv.Store(srv)
	srv.SetEventHandler(c.lastPosOwner)

	startPos, err := resolveBinlogStart(srv, fromPos)
	if err != nil {
		srv.Close()
		return fmt.Errorf("resolve start pos: %w", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- srv.RunFrom(startPos) }()

	select {
	case <-ctx.Done():
		srv.Close()
		<-runDone // 等 goroutine 退出避免泄露
		return nil
	case <-c.stopped:
		srv.Close()
		<-runDone
		return nil
	case err := <-runDone:
		srv.Close()
		if err == nil {
			// RunFrom 正常退出（理论上 binlog stream 不会自然结束;但保护性返 nil 触发重连）
			return fmt.Errorf("canal RunFrom returned nil unexpectedly")
		}
		return err
	}
}

// isStopped 检查 Stop 是否已被调用（非 atomic 读 close(channel) 是安全的）。
func (c *GoMySQLCanalClient) isStopped() bool {
	select {
	case <-c.stopped:
		return true
	default:
		return false
	}
}

// Stop 关闭 canal 实例（幂等）。
// 通知重连循环退出 + Close 当前 canal 实例让 RunFrom 立即返回。
// 重连循环 goroutine 看到 c.stopped close 后 close(c.out)。
func (c *GoMySQLCanalClient) Stop() error {
	c.stopOnce.Do(func() {
		close(c.stopped)
		if srv := c.canalSrv.Load(); srv != nil {
			srv.Close()
		}
	})
	return nil
}

// resolveBinlogStart 把 "file/pos" 字符串解析成 mysql.Position；空 → master GetMasterPos 取当前位点。
func resolveBinlogStart(srv *canal.Canal, fromPos string) (mysql.Position, error) {
	if fromPos == "" {
		// 从 master 当前位点起拉（首次启动）
		return srv.GetMasterPos()
	}
	return parseBinlogPosStr(fromPos)
}

// parseBinlogPosStr 把 "file/pos" 字符串解析成 mysql.Position。
func parseBinlogPosStr(s string) (mysql.Position, error) {
	idx := strings.LastIndex(s, "/")
	if idx == -1 {
		return mysql.Position{}, fmt.Errorf("invalid binlog pos %q (expected file/pos)", s)
	}
	pos, err := strconv.ParseUint(s[idx+1:], 10, 32)
	if err != nil {
		return mysql.Position{}, fmt.Errorf("invalid binlog pos %q: %w", s, err)
	}
	return mysql.Position{Name: s[:idx], Pos: uint32(pos)}, nil
}

// canalEventHandler 实现 canal.EventHandler 接口，把 RowsEvent → BinlogEvent。
//
// 跨重连周期共享:Subscribe 构造一次,重连时 SetEventHandler 复用同一实例。
// lastPos 字段记录最新成功收到事件的 binlog 位点,供重连续订用(精度到 transaction 边界,Sink 幂等兜底重放)。
type canalEventHandler struct {
	canal.DummyEventHandler
	out         chan<- BinlogEvent
	l           logger.LoggerX
	initialPos  atomic.Value // string - 首次启动时的 fromPos,供 snapshotPos fallback
	currentFile atomic.Value // string - 最新 binlog 文件名(OnRotate 更新)
	currentPos  atomic.Uint64
}

// snapshotPos 返回 "file/pos" 格式的最新位点;OnRotate/OnRow 未触发过返回 initialPos。
// 重连循环用此续订,避免重连后从 master 当前位点开始漏事件。
func (h *canalEventHandler) snapshotPos() string {
	file, _ := h.currentFile.Load().(string)
	pos := h.currentPos.Load()
	if file == "" || pos == 0 {
		init, _ := h.initialPos.Load().(string)
		return init
	}
	return fmt.Sprintf("%s/%d", file, pos)
}

// OnRotate binlog rotate 时(文件切换/master 重启)同步最新文件名和起始位点。
func (h *canalEventHandler) OnRotate(_ *replication.EventHeader, e *replication.RotateEvent) error {
	h.currentFile.Store(string(e.NextLogName))
	h.currentPos.Store(e.Position)
	return nil
}

func (h *canalEventHandler) OnRow(e *canal.RowsEvent) error {
	// 更新位点（在事件发出前,确保重连后续订点至少不漏）
	h.currentPos.Store(uint64(e.Header.LogPos))

	// 拼当前 binlog "file/pos" 透传给 BinlogEvent.BinlogPos —
	// 驱动 incr.runPartition.flush() 写 checkpoint。
	// file 未知时（OnRotate / OnPosSynced 还没触发）留空串，避免 "/12345" 残废格式导致下次 ParseBinlogPos 失败。
	file, _ := h.currentFile.Load().(string)
	var binlogPos string
	if file != "" {
		binlogPos = fmt.Sprintf("%s/%d", file, e.Header.LogPos)
	}

	op := canalActionToOp(e.Action)
	// update 类型 e.Rows 是 [before, after, before, after, ...]
	if op == "update" {
		for i := 0; i+1 < len(e.Rows); i += 2 {
			be := buildBinlogEvent(e.Table, op, e.Rows[i], e.Rows[i+1], e.Header.Timestamp, binlogPos)
			h.out <- be
		}
		return nil
	}
	for _, row := range e.Rows {
		var before, after []any
		switch op {
		case "insert":
			after = row
		case "delete":
			before = row
		}
		be := buildBinlogEvent(e.Table, op, before, after, e.Header.Timestamp, binlogPos)
		h.out <- be
	}
	return nil
}

// OnPosSynced binlog 位点定期 sync(每个事件后被调,canal 内部位点跟踪)。
// 我们用 OnRow 内的 e.Header.LogPos 已经够精确;OnPosSynced 主要用于补 currentFile 在没 OnRotate 时的初始填充。
func (h *canalEventHandler) OnPosSynced(_ *replication.EventHeader, pos mysql.Position, _ mysql.GTIDSet, _ bool) error {
	if pos.Name != "" {
		h.currentFile.Store(pos.Name)
	}
	if pos.Pos > 0 {
		h.currentPos.Store(uint64(pos.Pos))
	}
	return nil
}

func canalActionToOp(action string) string {
	switch action {
	case canal.InsertAction:
		return "insert"
	case canal.UpdateAction:
		return "update"
	case canal.DeleteAction:
		return "delete"
	}
	return action
}

// buildBinlogEvent 把 (table, before, after, ts, binlogPos) 转 BinlogEvent。
// PK 取第一列（业务表通常 id 为 PK 第一列；自定义 PK 在 table schema 中按 PrimaryKey 字段索引）。
// binlogPos 形如 "file/pos"；空串表示当前 file 未知（首次连接 OnRotate/OnPosSynced 还没触发的边界），
// 调用方写 ckpt 守卫会跳过这种事件。
func buildBinlogEvent(table *schema.Table, op string, before, after []any, eventTs uint32, binlogPos string) BinlogEvent {
	beforeMap := rowToMap(table, before)
	afterMap := rowToMap(table, after)
	var pk int64
	if pkIdx := pkColumnIndex(table); pkIdx >= 0 {
		row := after
		if op == "delete" {
			row = before
		}
		if pkIdx < len(row) {
			pk = toInt64Loose(row[pkIdx])
		}
	}
	return BinlogEvent{
		Op:        op,
		Table:     table.Name,
		PK:        strconv.FormatInt(pk, 10),
		Before:    beforeMap,
		After:     afterMap,
		BinlogPos: binlogPos,
		EventTs:   time.Unix(int64(eventTs), 0).UnixMilli(),
	}
}

// pkColumnIndex 取主键列索引；多列主键取第一列。
func pkColumnIndex(t *schema.Table) int {
	if len(t.PKColumns) > 0 {
		return t.PKColumns[0]
	}
	return -1
}

// rowToMap 把 RowsEvent.Rows 元素映射成 map；字符列 []byte 转 string 与 GORM map scan 对齐。
func rowToMap(t *schema.Table, row []any) map[string]any {
	if row == nil {
		return nil
	}
	m := make(map[string]any, len(t.Columns))
	for i, col := range t.Columns {
		if i >= len(row) {
			break
		}
		m[col.Name] = normalizeCanalColumn(col, row[i])
	}
	return m
}

// normalizeCanalColumn 字符列 []byte → string；BINARY/BLOB 保留 []byte。
func normalizeCanalColumn(col schema.TableColumn, v any) any {
	b, ok := v.([]byte)
	if !ok {
		return v
	}
	switch col.Type {
	case schema.TYPE_STRING, schema.TYPE_JSON, schema.TYPE_ENUM, schema.TYPE_SET:
		return string(b)
	}
	return v
}

// toInt64Loose 把 driver 返回的各种数值类型转 int64（PK 列通常 int 系列）。
func toInt64Loose(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case uint64:
		return int64(x)
	case uint32:
		return int64(x)
	case []byte:
		n, err := strconv.ParseInt(string(x), 10, 64)
		if err != nil {
			return 0
		}
		return n
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// 编译期断言：GoMySQLCanalClient 实现 BinlogClient 接口；canalEventHandler 实现 canal.EventHandler。
var (
	_ BinlogClient       = (*GoMySQLCanalClient)(nil)
	_ canal.EventHandler = (*canalEventHandler)(nil)
)
