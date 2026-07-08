package source

import (
	"testing"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	"github.com/go-mysql-org/go-mysql/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

func TestNewGoMySQLCanalClient(t *testing.T) {
	t.Run("ServerID=0 → error（必须显式配置避免撞槽位）", func(t *testing.T) {
		_, err := NewGoMySQLCanalClient(GoMySQLCanalClientConfig{
			Addr: "127.0.0.1:3306", User: "root", Password: "root",
		}, logger.NewNopLogger())
		require.Error(t, err)
		assert.ErrorContains(t, err, "ServerID")
	})

	t.Run("ServerID > 0 → 构造成功返 BinlogClient（默认配置由 ctor 内部填充，不通过 API 暴露）", func(t *testing.T) {
		c, err := NewGoMySQLCanalClient(GoMySQLCanalClientConfig{
			Addr: "127.0.0.1:3306", ServerID: 1, User: "root", Password: "root",
		}, logger.NewNopLogger())
		require.NoError(t, err)
		assert.NotNil(t, c)
	})
}

func TestCanalActionToOp(t *testing.T) {
	assert.Equal(t, "insert", canalActionToOp(canal.InsertAction))
	assert.Equal(t, "update", canalActionToOp(canal.UpdateAction))
	assert.Equal(t, "delete", canalActionToOp(canal.DeleteAction))
	assert.Equal(t, "unknown", canalActionToOp("unknown"))
}

func TestPKColumnIndex(t *testing.T) {
	t.Run("单列主键 → 返回 index", func(t *testing.T) {
		tbl := &schema.Table{PKColumns: []int{0}}
		assert.Equal(t, 0, pkColumnIndex(tbl))
	})
	t.Run("复合主键 → 返回首列 index", func(t *testing.T) {
		tbl := &schema.Table{PKColumns: []int{2, 5}}
		assert.Equal(t, 2, pkColumnIndex(tbl))
	})
	t.Run("无主键 → -1", func(t *testing.T) {
		tbl := &schema.Table{PKColumns: nil}
		assert.Equal(t, -1, pkColumnIndex(tbl))
	})
}

func TestRowToMap(t *testing.T) {
	tbl := &schema.Table{
		Name:    "article",
		Columns: []schema.TableColumn{{Name: "id"}, {Name: "title"}, {Name: "view"}},
	}
	t.Run("正常映射", func(t *testing.T) {
		m := rowToMap(tbl, []any{int64(42), "hello", int64(100)})
		assert.Equal(t, int64(42), m["id"])
		assert.Equal(t, "hello", m["title"])
		assert.Equal(t, int64(100), m["view"])
	})
	t.Run("行长度短于 columns → 只映射前 N", func(t *testing.T) {
		m := rowToMap(tbl, []any{int64(1)})
		assert.Equal(t, int64(1), m["id"])
		_, ok := m["title"]
		assert.False(t, ok)
	})
	t.Run("nil → nil", func(t *testing.T) {
		assert.Nil(t, rowToMap(tbl, nil))
	})
}

func TestToInt64Loose(t *testing.T) {
	assert.Equal(t, int64(42), toInt64Loose(int64(42)))
	assert.Equal(t, int64(42), toInt64Loose(int(42)))
	assert.Equal(t, int64(42), toInt64Loose(int32(42)))
	assert.Equal(t, int64(42), toInt64Loose(uint64(42)))
	assert.Equal(t, int64(42), toInt64Loose(uint32(42)))
	assert.Equal(t, int64(42), toInt64Loose([]byte("42")))
	assert.Equal(t, int64(42), toInt64Loose("42"))
	assert.Equal(t, int64(0), toInt64Loose(nil))
	assert.Equal(t, int64(0), toInt64Loose("not-a-number"))
}

func TestBuildBinlogEvent(t *testing.T) {
	tbl := &schema.Table{
		Name:      "article",
		Columns:   []schema.TableColumn{{Name: "id"}, {Name: "title"}},
		PKColumns: []int{0},
	}
	t.Run("insert", func(t *testing.T) {
		be := buildBinlogEvent(tbl, "insert", nil, []any{int64(7), "x"}, 1700000000, "mysql-bin.000001/4096")
		assert.Equal(t, "insert", be.Op)
		assert.Equal(t, "article", be.Table)
		assert.Equal(t, "7", be.PK)
		assert.Equal(t, "x", be.After["title"])
		assert.Nil(t, be.Before)
		assert.Equal(t, "mysql-bin.000001/4096", be.BinlogPos)
		assert.Equal(t, int64(1700000000000), be.EventTs)
	})
	t.Run("delete → Before 填，PK 取 before 行", func(t *testing.T) {
		be := buildBinlogEvent(tbl, "delete", []any{int64(9), "gone"}, nil, 0, "")
		assert.Equal(t, "9", be.PK)
		assert.Equal(t, "gone", be.Before["title"])
		assert.Equal(t, "", be.BinlogPos)
	})
}

func TestResolveBinlogStart(t *testing.T) {
	t.Run("malformed → error", func(t *testing.T) {
		// 这里只测纯字符串解析路径，不依赖真 canal 实例
		_, err := parseBinlogPosStr("no-slash")
		assert.Error(t, err)
		_, err = parseBinlogPosStr("file/notnum")
		assert.Error(t, err)
	})
	t.Run("正常解析", func(t *testing.T) {
		p, err := parseBinlogPosStr("mysql-bin.000003/4096")
		require.NoError(t, err)
		assert.Equal(t, "mysql-bin.000003", p.Name)
		assert.Equal(t, uint32(4096), p.Pos)
	})
}

// ── canalEventHandler 位点跟踪测试 ──────────────────────────
// 覆盖重连续订点逻辑:OnRotate / OnRow / OnPosSynced 实时更新位点;snapshotPos 重连用。
// 这些是 canal 自动重连（Subscribe 内循环）的核心 — 重连时拿 handler 最新位点 RunFrom。

func TestCanalEventHandler_SnapshotPos(t *testing.T) {
	t.Run("尚无事件 → 返 initialPos", func(t *testing.T) {
		h := &canalEventHandler{l: logger.NewNopLogger()}
		h.initialPos.Store("mysql-bin.000005/100")
		assert.Equal(t, "mysql-bin.000005/100", h.snapshotPos())
	})

	t.Run("initialPos 为空 + 无事件 → 返空串", func(t *testing.T) {
		h := &canalEventHandler{l: logger.NewNopLogger()}
		h.initialPos.Store("")
		assert.Equal(t, "", h.snapshotPos())
	})

	t.Run("OnPosSynced 更新后 → 返新位点", func(t *testing.T) {
		h := &canalEventHandler{l: logger.NewNopLogger()}
		h.initialPos.Store("mysql-bin.000001/4")
		err := h.OnPosSynced(nil, mysql.Position{Name: "mysql-bin.000003", Pos: 2048}, nil, false)
		require.NoError(t, err)
		assert.Equal(t, "mysql-bin.000003/2048", h.snapshotPos())
	})

	t.Run("OnRotate 切换 binlog file → snapshotPos 跟着切", func(t *testing.T) {
		h := &canalEventHandler{l: logger.NewNopLogger()}
		h.initialPos.Store("mysql-bin.000001/100")
		err := h.OnRotate(nil, &replication.RotateEvent{
			NextLogName: []byte("mysql-bin.000007"),
			Position:    4,
		})
		require.NoError(t, err)
		assert.Equal(t, "mysql-bin.000007/4", h.snapshotPos())
	})

	t.Run("OnPosSynced 部分字段 → 已有字段不被空值覆盖", func(t *testing.T) {
		h := &canalEventHandler{l: logger.NewNopLogger()}
		// 先 OnRotate 给个 file
		_ = h.OnRotate(nil, &replication.RotateEvent{NextLogName: []byte("mysql-bin.000010"), Position: 4})
		// OnPosSynced 传空 Name(canal SDK 某些 event 可能这样) → 不覆盖现有 file
		_ = h.OnPosSynced(nil, mysql.Position{Name: "", Pos: 9999}, nil, false)
		assert.Equal(t, "mysql-bin.000010/9999", h.snapshotPos())
	})
}

func TestCanalEventHandler_OnRow_UpdatesPos(t *testing.T) {
	out := make(chan BinlogEvent, 10)
	h := &canalEventHandler{out: out, l: logger.NewNopLogger()}
	h.initialPos.Store("mysql-bin.000001/100")
	_ = h.OnRotate(nil, &replication.RotateEvent{NextLogName: []byte("mysql-bin.000002"), Position: 4})

	// 构造一个 insert RowsEvent
	tbl := &schema.Table{
		Name:      "article",
		Columns:   []schema.TableColumn{{Name: "id"}, {Name: "title"}},
		PKColumns: []int{0},
	}
	err := h.OnRow(&canal.RowsEvent{
		Table:  tbl,
		Action: canal.InsertAction,
		Header: &replication.EventHeader{LogPos: 5000, Timestamp: 1700000000},
		Rows:   [][]any{{int64(42), "hello"}},
	})
	require.NoError(t, err)

	// 位点已被更新到 OnRow 的 LogPos
	assert.Equal(t, "mysql-bin.000002/5000", h.snapshotPos())

	// channel 收到事件
	evt := <-out
	assert.Equal(t, "insert", evt.Op)
	assert.Equal(t, "42", evt.PK)
	assert.Equal(t, "hello", evt.After["title"])
}

// TestCanalEventHandler_OnRow_FillsBinlogPos 回归用例：
// BinlogEvent.BinlogPos 必须被 OnRow 填上 "file/logPos"，否则 incr.runPartition.flush()
// 守卫 `if lastPos != ""` 永不命中 → checkpoint phase=incr 永不入库 → 重启丢事件。
func TestCanalEventHandler_OnRow_FillsBinlogPos(t *testing.T) {
	tbl := &schema.Table{
		Name:      "article",
		Columns:   []schema.TableColumn{{Name: "id"}, {Name: "title"}},
		PKColumns: []int{0},
	}

	t.Run("OnRotate 已设 file → BinlogPos = file/logPos", func(t *testing.T) {
		out := make(chan BinlogEvent, 10)
		h := &canalEventHandler{out: out, l: logger.NewNopLogger()}
		h.initialPos.Store("mysql-bin.000001/4")
		_ = h.OnRotate(nil, &replication.RotateEvent{NextLogName: []byte("mysql-bin.000002"), Position: 4})

		err := h.OnRow(&canal.RowsEvent{
			Table:  tbl,
			Action: canal.InsertAction,
			Header: &replication.EventHeader{LogPos: 5000, Timestamp: 1700000000},
			Rows:   [][]any{{int64(42), "hello"}},
		})
		require.NoError(t, err)

		evt := <-out
		assert.Equal(t, "mysql-bin.000002/5000", evt.BinlogPos)
	})

	t.Run("update 类型 → 两个事件都带 BinlogPos", func(t *testing.T) {
		out := make(chan BinlogEvent, 10)
		h := &canalEventHandler{out: out, l: logger.NewNopLogger()}
		_ = h.OnRotate(nil, &replication.RotateEvent{NextLogName: []byte("mysql-bin.000003"), Position: 4})

		err := h.OnRow(&canal.RowsEvent{
			Table:  tbl,
			Action: canal.UpdateAction,
			Header: &replication.EventHeader{LogPos: 7777, Timestamp: 1700000001},
			Rows:   [][]any{{int64(1), "old"}, {int64(1), "new"}},
		})
		require.NoError(t, err)

		evt := <-out
		assert.Equal(t, "mysql-bin.000003/7777", evt.BinlogPos)
	})

	t.Run("file 尚未被 OnRotate/OnPosSynced 设过 → BinlogPos 留空，不写残废格式", func(t *testing.T) {
		out := make(chan BinlogEvent, 10)
		h := &canalEventHandler{out: out, l: logger.NewNopLogger()}
		// 注意：不调 OnRotate / OnPosSynced，currentFile 为空

		err := h.OnRow(&canal.RowsEvent{
			Table:  tbl,
			Action: canal.InsertAction,
			Header: &replication.EventHeader{LogPos: 9999, Timestamp: 1700000002},
			Rows:   [][]any{{int64(3), "x"}},
		})
		require.NoError(t, err)

		evt := <-out
		assert.Equal(t, "", evt.BinlogPos, "file 未知时禁止写 /9999 残废格式")
	})
}

func TestCanalClient_IsStopped(t *testing.T) {
	t.Run("初始未 stop → false", func(t *testing.T) {
		c := &GoMySQLCanalClient{stopped: make(chan struct{})}
		assert.False(t, c.isStopped())
	})

	t.Run("Stop 后 → true(close stopped channel)", func(t *testing.T) {
		c := &GoMySQLCanalClient{stopped: make(chan struct{})}
		close(c.stopped) // 模拟 Stop 触发
		assert.True(t, c.isStopped())
	})
}
