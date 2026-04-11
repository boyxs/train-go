package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"

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

