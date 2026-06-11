package domain

// AuditLog 一条写操作审计记录（append-only，audit 中间件异步落表）。
type AuditLog struct {
	Id        int64
	TaskId    int64  // 关联任务；create 路径从响应 data.taskId 回填，拿不到为 0
	Actor     string // 操作人（JWT user_id；未登录 "anonymous"）
	Action    string // consts.AuditAction*
	Payload   string // 请求体（DSN 已脱敏）
	Result    string // consts.AuditResultSuccess / AuditResultFail
	ErrorMsg  string // 失败时的业务 msg（截断 512）
	ClientIp  string
	CreatedAt int64
}
