package service

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/status"

	articlev1 "github.com/webook/api/gen/article/v1"
	interactionv1 "github.com/webook/api/gen/interaction/v1"
	searchv1 "github.com/webook/api/gen/search/v1"
	"github.com/webook/chat/domain"
	"github.com/webook/pkg/llm"
	"github.com/webook/pkg/logger"
)

// chatToolDefinitions 聊天模块使用的工具定义（传给 LLM）。
var chatToolDefinitions = []llm.Tool{
	{
		Name:        "search_articles",
		Description: "在平台文章中搜索与用户问题相关的内容，当用户询问文章知识、技术问题时使用",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "搜索关键词",
				},
			},
			"required": []string{"query"},
		},
	},
	{
		Name:        "get_hot_articles",
		Description: "获取平台热门文章列表，当用户请求推荐、热门文章时使用",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "返回数量，默认 5，最多 10",
					"default":     5,
				},
			},
		},
	},
	{
		Name:        "get_my_favorites",
		Description: "查询当前登录用户收藏的文章列表，当用户询问自己的收藏时使用",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "返回数量，默认 5，最多 10",
					"default":     5,
				},
			},
		},
	},
}

// ToolExecutor 工具执行器，供 AIChatService 调用。
type ToolExecutor interface {
	Definitions() []llm.Tool
	Execute(ctx context.Context, uid int64, name string, args map[string]any) (domain.ToolResultData, error)
}

// AIChatToolExecutor 通过 gRPC 调用主仓 webook-core 暴露的 search/article/interaction 服务。
// chat 服务自身只依赖 webook/api/gen/* 生成的客户端，禁止 import webook/internal/*。
type AIChatToolExecutor struct {
	searchCli  searchv1.SearchServiceClient
	articleCli articlev1.ArticleReaderServiceClient
	intrCli    interactionv1.InteractionServiceClient
	l          logger.LoggerX
}

func NewAIChatToolExecutor(
	searchCli searchv1.SearchServiceClient,
	articleCli articlev1.ArticleReaderServiceClient,
	intrCli interactionv1.InteractionServiceClient,
	l logger.LoggerX,
) ToolExecutor {
	return &AIChatToolExecutor{
		searchCli:  searchCli,
		articleCli: articleCli,
		intrCli:    intrCli,
		l:          l,
	}
}

func (e *AIChatToolExecutor) Definitions() []llm.Tool {
	return chatToolDefinitions
}

func (e *AIChatToolExecutor) Execute(ctx context.Context, uid int64, name string, args map[string]any) (domain.ToolResultData, error) {
	switch name {
	case "search_articles":
		return e.searchArticles(ctx, args)
	case "get_hot_articles":
		return e.getHotArticles(ctx, args)
	case "get_my_favorites":
		return e.getMyFavorites(ctx, uid, args)
	default:
		return domain.ToolResultData{Name: name, Error: fmt.Sprintf("未知工具: %s", name)}, nil
	}
}

func (e *AIChatToolExecutor) searchArticles(ctx context.Context, args map[string]any) (domain.ToolResultData, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return domain.ToolResultData{Name: "search_articles", Error: "缺少搜索关键词"}, nil
	}
	resp, err := e.searchCli.SearchArticles(ctx, &searchv1.SearchArticlesRequest{
		Query: query, Page: 1, Size: 5,
	})
	if err != nil {
		e.l.Warn("search_articles 工具调用失败", logger.String("query", query), logger.Error(err))
		return domain.ToolResultData{Name: "search_articles", Error: "搜索失败，请稍后重试"}, nil
	}
	cards := make([]domain.ArticleCard, 0, len(resp.GetArticles()))
	for _, a := range resp.GetArticles() {
		cards = append(cards, domain.ArticleCard{
			Id:       a.GetId(),
			Title:    a.GetTitle(),
			Abstract: a.GetAbstract(),
			Url:      fmt.Sprintf("/article/%d", a.GetId()),
		})
	}
	return domain.ToolResultData{Name: "search_articles", Articles: cards}, nil
}

func (e *AIChatToolExecutor) getHotArticles(ctx context.Context, args map[string]any) (domain.ToolResultData, error) {
	limit := parseLimit(args, 5)
	resp, err := e.intrCli.GetHotBizIds(ctx, &interactionv1.GetHotBizIdsRequest{
		Biz: "article", Limit: int32(limit),
	})
	if err != nil {
		e.l.Warn("get_hot_articles 查询热门 ID 失败", logger.Error(err))
		return domain.ToolResultData{Name: "get_hot_articles", Error: "获取热门文章失败"}, nil
	}
	if len(resp.GetBizIds()) == 0 {
		return domain.ToolResultData{Name: "get_hot_articles", Articles: []domain.ArticleCard{}}, nil
	}
	cards := e.fetchArticleCards(ctx, resp.GetBizIds(), "get_hot_articles")
	return domain.ToolResultData{Name: "get_hot_articles", Articles: cards}, nil
}

func (e *AIChatToolExecutor) getMyFavorites(ctx context.Context, uid int64, args map[string]any) (domain.ToolResultData, error) {
	limit := parseLimit(args, 5)
	resp, err := e.intrCli.GetCollectedBizIds(ctx, &interactionv1.GetCollectedBizIdsRequest{
		Uid: uid, Biz: "article", Limit: int32(limit),
	})
	if err != nil {
		e.l.Warn("get_my_favorites 查询收藏 ID 失败", logger.Int64("uid", uid), logger.Error(err))
		return domain.ToolResultData{Name: "get_my_favorites", Error: "获取收藏失败"}, nil
	}
	if len(resp.GetBizIds()) == 0 {
		return domain.ToolResultData{Name: "get_my_favorites", Articles: []domain.ArticleCard{}}, nil
	}
	cards := e.fetchArticleCards(ctx, resp.GetBizIds(), "get_my_favorites")
	return domain.ToolResultData{Name: "get_my_favorites", Articles: cards}, nil
}

// fetchArticleCards 一次 RPC 批量取文章详情；NotFound 由 server 端静默过滤。
func (e *AIChatToolExecutor) fetchArticleCards(ctx context.Context, ids []int64, toolName string) []domain.ArticleCard {
	resp, err := e.articleCli.BatchGetArticles(ctx, &articlev1.BatchGetArticlesRequest{Ids: ids})
	if err != nil {
		e.l.Warn(toolName+" 批量获取文章详情失败",
			logger.String("code", status.Code(err).String()), logger.Error(err))
		return nil
	}
	cards := make([]domain.ArticleCard, 0, len(resp.GetArticles()))
	for _, a := range resp.GetArticles() {
		cards = append(cards, domain.ArticleCard{
			Id:       a.GetId(),
			Title:    a.GetTitle(),
			Abstract: a.GetAbstract(),
			Url:      fmt.Sprintf("/article/%d", a.GetId()),
		})
	}
	return cards
}

// parseLimit 从 args 解析 limit，超出范围则用默认值
func parseLimit(args map[string]any, defaultVal int) int {
	raw, ok := args["limit"]
	if !ok {
		return defaultVal
	}
	switch v := raw.(type) {
	case float64:
		n := int(v)
		if n <= 0 || n > 10 {
			return defaultVal
		}
		return n
	case json.Number:
		n, err := v.Int64()
		if err != nil || n <= 0 || n > 10 {
			return defaultVal
		}
		return int(n)
	}
	return defaultVal
}
