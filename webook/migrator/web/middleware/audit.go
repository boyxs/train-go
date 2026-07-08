package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/boyxs/train-go/webook/migrator/consts"
	"github.com/boyxs/train-go/webook/migrator/domain"
	"github.com/boyxs/train-go/webook/migrator/repository"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

const (
	// AuditUserIDCtxKey JWT 中间件向 ctx 注入 user_id 的 key。
	AuditUserIDCtxKey = "user_id"
	// AuditActorAnonymous 未携带 JWT 时的兜底 actor 值（local 调试 / 测试场景）。
	AuditActorAnonymous = "anonymous"
	// AuditActionUnknown actionFor 路由表未匹配到的兜底动作。
	AuditActionUnknown = "unknown"
)

// dsnRefPattern 匹配 sourceDsnRef / sinkDsnRef 字段；脱敏成 "***"。
// 不解析 JSON 是为了兼容非标准（如带注释、字段顺序变化）的 payload。
var dsnRefPattern = regexp.MustCompile(`("(?:source|sink)DsnRef":\s*)"[^"]*"`)

// AuditMiddleware 写操作审计中间件。
//
// 行为：
//
//	GET 请求         c.Next 跳过；
//	写操作           抓 request body + mask DSN → c.Next → 取 response status/body → 异步落 audit_log；
//	异步落库失败     log Warn，不阻塞业务（审计写失败不破坏业务一致性）；
//	actor 缺失       fallback "anonymous"（JWT middleware 注入 ctx user_id 后才有真实 actor）。
//
// 构造函数返回 *struct + Build() gin.HandlerFunc。
// 落表经 AuditLogRepository（横切中间件，不挂业务 service，直达仓储层）。
type AuditMiddleware struct {
	repo repository.AuditLogRepository
	l    logger.LoggerX
}

func NewAuditMiddleware(repo repository.AuditLogRepository, l logger.LoggerX) *AuditMiddleware {
	return &AuditMiddleware{repo: repo, l: l}
}

func (m *AuditMiddleware) Build() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		// 抓 request body — handler 会消费 ctx.Request.Body，必须读完后复位
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			m.l.Warn("audit read request body failed", logger.Error(err))
			bodyBytes = nil
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		payload := string(dsnRefPattern.ReplaceAll(bodyBytes, []byte(`$1"***"`)))

		// 包装 ResponseWriter 抓响应字节流（用于 create 路径从 data.taskId 拿 task_id）
		rw := &auditBodyWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = rw

		c.Next()

		result := consts.AuditResultSuccess
		errorMsg := ""
		if rw.Status() >= http.StatusBadRequest {
			result = consts.AuditResultFail
			errorMsg = extractErrorMsg(rw.buf.Bytes(), rw.Status())
		}
		m.tryAudit(
			taskIdFor(c, rw.buf.Bytes()),
			actorFor(c),
			actionFor(c.Request.URL.Path, c.Request.Method, bodyBytes),
			payload, result, errorMsg, c.ClientIP(),
		)
	}
}

// extractErrorMsg 从响应 body 解析 {code, msg} 拿 msg 填 audit_log.error_msg。
//
// 优先级：
//  1. 标准业务响应 {"code": N, "msg": "..."} → 用 msg
//  2. 解不出 JSON → fallback "HTTP <status>"
//
// 截断到 512 字节（与 audit_log.error_msg 列宽对齐）。
func extractErrorMsg(body []byte, status int) string {
	const maxLen = 512
	if len(body) > 0 {
		var r struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal(body, &r); err == nil && r.Msg != "" {
			if len(r.Msg) > maxLen {
				return r.Msg[:maxLen]
			}
			return r.Msg
		}
	}
	// fallback：纯 HTTP 错误（gin 默认 abort 等情况）
	return fmt.Sprintf("HTTP %d", status)
}

// tryAudit 异步落 audit_log；用 context.Background() 解耦请求 ctx（避免父 ctx cancel 把审计写挂）。
func (m *AuditMiddleware) tryAudit(taskId int64, actor, action, payload, result, errorMsg, clientIp string) {
	go func() {
		if _, err := m.repo.Create(context.Background(), domain.AuditLog{
			TaskId:   taskId,
			Actor:    actor,
			Action:   action,
			Payload:  payload,
			Result:   result,
			ErrorMsg: errorMsg,
			ClientIp: clientIp,
		}); err != nil {
			m.l.Warn("audit insert failed",
				logger.Int64("task_id", taskId),
				logger.String("action", action),
				logger.Error(err))
		}
	}()
}

func actorFor(c *gin.Context) string {
	if v, ok := c.Get(AuditUserIDCtxKey); ok {
		return fmt.Sprintf("%v", v)
	}
	return AuditActorAnonymous
}

// actionFor 路由 + 请求体 → audit action。
// 多数端点按 path 即可定；start / switch 的细分动作藏在 body 里（phase / action），解出来映射到对应 action。
func actionFor(path, method string, body []byte) string {
	if method != http.MethodPost {
		return AuditActionUnknown // 只审计写操作
	}
	switch {
	case path == "/migrator/tasks":
		return consts.AuditActionCreate
	case path == "/migrator/preflight":
		return consts.AuditActionPreflight
	case strings.HasSuffix(path, "/start"):
		if phaseOf(body) == "incr" {
			return consts.AuditActionStartIncr
		}
		return consts.AuditActionStartFull
	case strings.HasSuffix(path, "/pause"):
		return consts.AuditActionPause
	case strings.HasSuffix(path, "/throttle"):
		return consts.AuditActionThrottle
	case strings.HasSuffix(path, "/gray"):
		return consts.AuditActionSetGray
	case strings.HasSuffix(path, "/switch"):
		return switchActionOf(body)
	case strings.HasSuffix(path, "/verify"):
		return consts.AuditActionVerify
	case strings.HasSuffix(path, "/repair"):
		return consts.AuditActionRepair
	case strings.HasSuffix(path, "/replay-dl"):
		return consts.AuditActionReplayDL
	}
	return AuditActionUnknown
}

// phaseOf 解 start 请求体的 phase（full / incr）；解不出返空，调用方按 full 兜底。
func phaseOf(body []byte) string {
	var r struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return ""
	}
	return r.Phase
}

// switchActionOf 按 switch 请求体的 action 细分；无 action / 解不出当作阶段推进（set_stage_SRC_FIRST）。
func switchActionOf(body []byte) string {
	var r struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return consts.AuditActionSetStageSRCFirst
	}
	switch r.Action {
	case "rollback":
		return consts.AuditActionRollback
	case "propose":
		return consts.AuditActionCutoverPropose
	case "approve":
		return consts.AuditActionCutoverApprove
	default:
		return consts.AuditActionSetStageSRCFirst
	}
}

// taskIdFor 优先 path param :id；其次 create 路径从 response.data.taskId 拿；都拿不到 = 0。
func taskIdFor(c *gin.Context, respBody []byte) int64 {
	if idStr := c.Param("id"); idStr != "" {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			return id
		}
	}
	var r struct {
		Data struct {
			TaskId int64 `json:"taskId"`
		} `json:"data"`
	}
	if json.Unmarshal(respBody, &r) == nil {
		return r.Data.TaskId
	}
	return 0
}

type auditBodyWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w *auditBodyWriter) Write(data []byte) (int, error) {
	w.buf.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *auditBodyWriter) WriteString(s string) (int, error) {
	w.buf.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
