package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"gitee.com/train-cloud/geektime-basic-go/internal/consts"
	"gitee.com/train-cloud/geektime-basic-go/internal/service"
	"gitee.com/train-cloud/geektime-basic-go/internal/web/jwt"
	"gitee.com/train-cloud/geektime-basic-go/pkg/logger"
	"gitee.com/train-cloud/geektime-basic-go/pkg/ratelimit"
	"github.com/gin-gonic/gin"
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
	g.POST("/conversation/create", h.CreateConversation)
	g.POST("/conversation/list", h.ListConversations)
	g.POST("/conversation/delete", h.DeleteConversation)
	g.POST("/message/list", h.ListMessages)
	g.POST("/message/send", h.SendMessage)
	g.POST("/stop", h.StopGeneration)
	g.POST("/conversation/generating", h.IsGenerating)
	g.GET("/message/stream", h.ResumeStream)
	g.POST("/message/feedback", h.SetFeedback)
}

type conversationIdReq struct {
	ConversationId int64 `json:"conversationId"`
}

func (h *InternalChatHandler) IsGenerating(ctx *gin.Context) {
	var req conversationIdReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "参数错误"})
		return
	}
	generating := h.svc.IsGenerating(ctx, req.ConversationId)
	ctx.JSON(http.StatusOK, Result{Data: generating})
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

func (h *InternalChatHandler) CreateConversation(ctx *gin.Context) {
	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)
	conv, err := h.svc.CreateConversation(ctx.Request.Context(), uc.Userid)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("创建对话失败", logger.Int64("uid", uc.Userid), logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Data: conv})
}

func (h *InternalChatHandler) ListConversations(ctx *gin.Context) {
	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)
	convs, err := h.svc.ListConversations(ctx.Request.Context(), uc.Userid)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("获取对话列表失败", logger.Int64("uid", uc.Userid), logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Data: convs})
}

func (h *InternalChatHandler) DeleteConversation(ctx *gin.Context) {
	var req conversationIdReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)
	err := h.svc.DeleteConversation(ctx.Request.Context(), uc.Userid, req.ConversationId)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("删除对话失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("convId", req.ConversationId),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Msg: "OK"})
}

func (h *InternalChatHandler) ListMessages(ctx *gin.Context) {
	var req listMessagesReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)
	msgs, err := h.svc.ListMessages(ctx.Request.Context(), uc.Userid, req.ConversationId, req.BeforeId, req.Limit)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "对话不存在"})
			return
		}
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("获取消息列表失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("convId", req.ConversationId),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Data: msgs})
}

func (h *InternalChatHandler) SendMessage(ctx *gin.Context) {
	var req sendMessageReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)

	// 限流检查：复用 pkg/ratelimit.Limiter
	key := fmt.Sprintf(consts.ChatRateLimitPattern, uc.Userid)
	limited, limitErr := h.limiter.Limit(ctx.Request.Context(), key)
	if limitErr != nil {
		h.l.Error("限流检查失败", logger.Int64("uid", uc.Userid), logger.Error(limitErr))
	}
	if limited {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "发送过于频繁，请稍后再试"})
		return
	}

	ch, err := h.svc.SendMessage(ctx.Request.Context(), uc.Userid, req.ConversationId, req.Content)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "对话不存在"})
			return
		}
		if errors.Is(err, service.ErrMessageTooLong) {
			ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "消息内容过长"})
			return
		}
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("发送消息失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("convId", req.ConversationId),
			logger.Error(err))
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

func (h *InternalChatHandler) StopGeneration(ctx *gin.Context) {
	var req conversationIdReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)
	err := h.svc.StopGeneration(ctx.Request.Context(), uc.Userid, req.ConversationId)
	if err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		return
	}
	ctx.JSON(http.StatusOK, Result{Msg: "OK"})
}

// ResumeStream SSE 重连端点：GET /chat/message/stream?conversationId=xx
// 浏览器带 Last-Event-ID header 从断点续传
// ResumeStream SSE 重连端点：GET /chat/message/stream?conversationId=xx
func (h *InternalChatHandler) ResumeStream(ctx *gin.Context) {
	convId, _ := strconv.ParseInt(ctx.Query("conversationId"), 10, 64)
	if convId <= 0 {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "参数错误"})
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
			ch <- formatSSE(ids[i], e.Type, e)
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
			ch <- formatSSE(ids[i], e.Type, e)
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

func (h *InternalChatHandler) SetFeedback(ctx *gin.Context) {
	var req setFeedbackReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "参数错误"})
		return
	}
	if req.MessageId <= 0 || req.ConversationId <= 0 {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "参数错误"})
		return
	}
	if req.Feedback != -1 && req.Feedback != 0 && req.Feedback != 1 {
		ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "无效的反馈值"})
		return
	}
	uc := ctx.MustGet(consts.UserKey).(jwt.UserClaims)
	err := h.svc.SetFeedback(ctx.Request.Context(), uc.Userid, req.ConversationId, req.MessageId, req.Feedback)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			ctx.JSON(http.StatusOK, Result{Code: 4, Msg: "对话不存在"})
			return
		}
		ctx.JSON(http.StatusOK, Result{Code: 5, Msg: "系统错误"})
		h.l.Error("设置反馈失败",
			logger.Int64("uid", uc.Userid),
			logger.Int64("msgId", req.MessageId),
			logger.Error(err))
		return
	}
	ctx.JSON(http.StatusOK, Result{Msg: "OK"})
}

// formatSSE 将事件格式化为 SSE 文本
func formatSSE(id, eventType string, event any) string {
	data, _ := json.Marshal(event)
	if id != "" {
		return fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", id, eventType, string(data))
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(data))
}
