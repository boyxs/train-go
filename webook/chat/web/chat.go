package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/webook/chat/consts"
	"github.com/webook/chat/errs"
	"github.com/webook/chat/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/jwtx"
	"github.com/webook/pkg/logger"
	"github.com/webook/pkg/ratelimit"
)

type ChatHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type InternalChatHandler struct {
	svc     service.ChatService
	l       logger.LoggerX
	limiter ratelimit.Limiter
}

func NewInternalChatHandler(svc service.ChatService, l logger.LoggerX, limiter ratelimit.Limiter) ChatHandler {
	return &InternalChatHandler{svc: svc, l: l, limiter: limiter}
}

func (h *InternalChatHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/chat")
	g.POST("/conversation/create", ginx.WrapClaims[jwtx.UserClaims](consts.UserKey, h.CreateConversation))
	g.POST("/conversation/list", ginx.WrapClaims[jwtx.UserClaims](consts.UserKey, h.ListConversations))
	g.POST("/conversation/delete", ginx.WrapReqClaims[conversationIdReq, jwtx.UserClaims](consts.UserKey, h.DeleteConversation))
	g.POST("/message/list", ginx.WrapReqClaims[listMessagesReq, jwtx.UserClaims](consts.UserKey, h.ListMessages))
	g.POST("/message/send", h.SendMessage) // SSE 不能 wrap
	g.POST("/stop", ginx.WrapReqClaims[conversationIdReq, jwtx.UserClaims](consts.UserKey, h.StopGeneration))
	g.POST("/conversation/generating", ginx.WrapReqClaims[conversationIdReq, jwtx.UserClaims](consts.UserKey, h.IsGenerating))
	g.GET("/message/stream", h.ResumeStream) // SSE 不能 wrap
	g.POST("/message/feedback", ginx.WrapReqClaims[setFeedbackReq, jwtx.UserClaims](consts.UserKey, h.SetFeedback))
}

type conversationIdReq struct {
	ConversationId int64 `json:"conversationId"`
}

func (h *InternalChatHandler) IsGenerating(ctx *gin.Context, req conversationIdReq, uc jwtx.UserClaims) (ginx.Result, error) {
	// 校验 convId 归属当前用户：复用 ListMessages 触发 convRepo.Find(uid, convId)，
	// limit=1 + beforeId=0 是最廉价的探测，不存在/越权 → ErrConversationNotFound (404)
	if _, err := h.svc.ListMessages(ctx.Request.Context(), uc.Userid, req.ConversationId, 0, 1); err != nil {
		return ginx.Result{}, err
	}
	generating := h.svc.IsGenerating(ctx.Request.Context(), req.ConversationId)
	return ginx.Result{Data: generating}, nil
}

type listMessagesReq struct {
	ConversationId int64 `json:"conversationId"`
	BeforeId       int64 `json:"beforeId"` // 0 = 加载最新，>0 = 加载更早的
	Limit          int   `json:"limit"`    // 默认 20，最大 50
}

type sendMessageReq struct {
	ConversationId int64  `json:"conversationId"`
	Content        string `json:"content"`
}

func (h *InternalChatHandler) CreateConversation(ctx *gin.Context, uc jwtx.UserClaims) (ginx.Result, error) {
	conv, err := h.svc.CreateConversation(ctx.Request.Context(), uc.Userid)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: conv}, nil
}

func (h *InternalChatHandler) ListConversations(ctx *gin.Context, uc jwtx.UserClaims) (ginx.Result, error) {
	convs, err := h.svc.ListConversations(ctx.Request.Context(), uc.Userid)
	if err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Data: convs}, nil
}

func (h *InternalChatHandler) DeleteConversation(ctx *gin.Context, req conversationIdReq, uc jwtx.UserClaims) (ginx.Result, error) {
	if err := h.svc.DeleteConversation(ctx.Request.Context(), uc.Userid, req.ConversationId); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

func (h *InternalChatHandler) ListMessages(ctx *gin.Context, req listMessagesReq, uc jwtx.UserClaims) (ginx.Result, error) {
	msgs, err := h.svc.ListMessages(ctx.Request.Context(), uc.Userid, req.ConversationId, req.BeforeId, req.Limit)
	if err != nil {
		return ginx.Result{}, err // ErrConversationNotFound 自带 404，其他自动 500
	}
	return ginx.Result{Data: msgs}, nil
}

func (h *InternalChatHandler) SendMessage(ctx *gin.Context) {
	var req sendMessageReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ginx.WriteError(ctx, errs.ErrChatInvalidArgs)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(jwtx.UserClaims)

	// 限流检查：复用 pkg/ratelimit.Limiter
	key := fmt.Sprintf(consts.ChatRateLimitPattern, uc.Userid)
	limited, limitErr := h.limiter.Limit(ctx.Request.Context(), key)
	if limitErr != nil {
		h.l.Error("限流检查失败", logger.Int64("uid", uc.Userid), logger.Error(limitErr))
	}
	if limited {
		ginx.WriteError(ctx, errs.ErrChatRateLimit)
		return
	}

	ch, err := h.svc.SendMessage(ctx.Request.Context(), uc.Userid, req.ConversationId, req.Content)
	if err != nil {
		// *errs.Error（ErrConversationNotFound 404 / ErrMessageTooLong 400）自带 HTTP code，
		// 其他系统错误兜底 500。日志统一由 ginx.WriteError 内部记录（path + err）
		ginx.WriteError(ctx, err)
		return
	}

	// SSE 响应
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	ctx.Stream(func(w io.Writer) bool {
		event, ok := <-ch
		if !ok {
			return false
		}
		ctx.SSEvent(event.Type, event)
		return true
	})
}

func (h *InternalChatHandler) StopGeneration(ctx *gin.Context, req conversationIdReq, uc jwtx.UserClaims) (ginx.Result, error) {
	if err := h.svc.StopGeneration(ctx.Request.Context(), uc.Userid, req.ConversationId); err != nil {
		return ginx.Result{}, err
	}
	return ginx.Result{Msg: "OK"}, nil
}

// ResumeStream SSE 重连端点：GET /chat/message/stream?conversationId=xx
// 浏览器带 Last-Event-ID header 从断点续传。
// 鉴权：JWT 中间件验签后通过 ListMessages 探测 convId 归属，防止越权窃听他人对话流。
func (h *InternalChatHandler) ResumeStream(ctx *gin.Context) {
	convId, err := strconv.ParseInt(ctx.Query("conversationId"), 10, 64)
	if err != nil || convId <= 0 {
		ginx.WriteError(ctx, errs.ErrChatInvalidArgs)
		return
	}
	uc, ok := ctx.Get(consts.UserKey)
	if !ok {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	claims, ok := uc.(jwtx.UserClaims)
	if !ok {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	// 校验 convId 归属：service.ListMessages 内部 convRepo.Find(uid, convId)，
	// 不存在 / 越权 → ErrConversationNotFound (404)
	if _, err := h.svc.ListMessages(ctx.Request.Context(), claims.Userid, convId, 0, 1); err != nil {
		ginx.WriteError(ctx, err)
		return
	}
	lastId := ctx.GetHeader(consts.LastEventIDHeader)
	if lastId == "" {
		lastId = "0"
	}

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	reqCtx := ctx.Request.Context()
	ch := make(chan string, 32) // 已格式化的 SSE 文本
	go h.pollStream(reqCtx, convId, lastId, ch)

	ctx.Stream(func(w io.Writer) bool {
		line, ok := <-ch
		if !ok {
			return false
		}
		fmt.Fprint(w, line)
		return true
	})
}

// pollStream 从 Redis Stream 读事件，格式化为 SSE 文本推入 ch
func (h *InternalChatHandler) pollStream(
	ctx context.Context, convId int64, lastId string, ch chan<- string,
) {
	defer close(ch)

	// 补发历史（lastId="$" 跳过）
	if lastId != "$" {
		events, ids, generating := h.svc.ReadStream(ctx, convId, lastId)
		for i, e := range events {
			if line := h.formatSSE(ids[i], e.Type, e); line != "" {
				ch <- line
			}
		}
		if len(ids) > 0 {
			lastId = ids[len(ids)-1]
		}
		if !generating {
			ch <- "event: stream_end\ndata: {}\n\n"
			return
		}
	} else if !h.svc.IsGenerating(ctx, convId) {
		ch <- "event: stream_end\ndata: {}\n\n"
		return
	}

	// 阻塞读新事件
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		events, ids := h.svc.BlockReadStream(ctx, convId, lastId, 2*time.Second)
		for i, e := range events {
			if line := h.formatSSE(ids[i], e.Type, e); line != "" {
				ch <- line
			}
		}
		if len(ids) > 0 {
			lastId = ids[len(ids)-1]
		}
		if !h.svc.IsGenerating(ctx, convId) {
			ch <- "event: stream_end\ndata: {}\n\n"
			return
		}
	}
}

type setFeedbackReq struct {
	ConversationId int64 `json:"conversationId"`
	MessageId      int64 `json:"messageId"`
	Feedback       int8  `json:"feedback"`
}

func (h *InternalChatHandler) SetFeedback(ctx *gin.Context, req setFeedbackReq, uc jwtx.UserClaims) (ginx.Result, error) {
	if req.MessageId <= 0 || req.ConversationId <= 0 {
		return ginx.Result{}, errs.ErrChatInvalidArgs
	}
	if req.Feedback != -1 && req.Feedback != 0 && req.Feedback != 1 {
		return ginx.Result{}, errs.ErrFeedbackInvalid
	}
	if err := h.svc.SetFeedback(ctx.Request.Context(), uc.Userid, req.ConversationId, req.MessageId, req.Feedback); err != nil {
		return ginx.Result{}, err // ErrConversationNotFound 自带 404
	}
	return ginx.Result{Msg: "OK"}, nil
}

// formatSSE 将事件格式化为 SSE 文本；序列化失败返回空串由调用方丢弃，避免推空 frame 误导前端
func (h *InternalChatHandler) formatSSE(id, eventType string, event any) string {
	data, err := json.Marshal(event)
	if err != nil {
		h.l.Error("SSE 事件序列化失败",
			logger.String("eventType", eventType),
			logger.Error(err))
		return ""
	}
	if id != "" {
		return fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", id, eventType, string(data))
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(data))
}
