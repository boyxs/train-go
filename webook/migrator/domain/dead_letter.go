package domain

// DeadLetter 一条死信（Sink 重试耗尽后落 dead_letter 表，等待 replay-dl 重放）。
//
// BizTable 是源表名（task.tables[].src）——重放按它反查 tableIdx 路由到对应 Sink；
// Payload 是行数据 JSON（列名→值）。
type DeadLetter struct {
	Id           int64
	TaskId       int64
	Op           string // sink.OpInsert / OpUpdate / OpDelete
	BizTable     string
	BizId        string
	Payload      string
	LastError    string
	RetryCount   int
	Replayed     int8 // 0=待重放 1=已重放
	ReplayFailed int8 // 1=多次重放仍失败，转人工
	CreatedAt    int64
	ReplayedAt   int64
}
