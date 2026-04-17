package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/webook/internal/domain"
	"github.com/webook/internal/repository"
	"github.com/webook/internal/service/ai"
	"github.com/webook/pkg/logger"
)

// chatToolDefinitions 聊天模块使用的工具定义（传给 LLM）
var chatToolDefinitions = []ai.Tool{
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

// ToolExecutor 工具执行器，供 AIChatService 调用
type ToolExecutor interface {
	// Definitions 返回传给 LLM 的工具定义列表
	Definitions() []ai.Tool
	// Execute 执行指定工具，返回可序列化结果
	Execute(ctx context.Context, uid int64, name string, args map[string]any) (domain.ToolResultData, error)
}

// AIChatToolExecutor 实现 ToolExecutor，封装 3 个工具
type AIChatToolExecutor struct {
	search  ArticleSearchService
	reader  ArticleReaderService
	intrRepo repository.InteractionRepository
	l       logger.LoggerX
}

func NewAIChatToolExecutor(
	search ArticleSearchService,
	reader ArticleReaderService,
	intrRepo repository.InteractionRepository,
	l logger.LoggerX,
) ToolExecutor {
	return &AIChatToolExecutor{
		search:   search,
		reader:   reader,
		intrRepo: intrRepo,
		l:        l,
	}
}

func (e *AIChatToolExecutor) Definitions() []ai.Tool {
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
	articles, _, err := e.search.Search(ctx, query, 1, 5)
	if err != nil {
		e.l.Warn("search_articles 工具调用失败", logger.String("query", query), logger.Error(err))
		return domain.ToolResultData{Name: "search_articles", Error: "搜索失败，请稍后重试"}, nil
	}
	return domain.ToolResultData{
		Name:     "search_articles",
		Articles: toArticleCards(articles),
	}, nil
}

func (e *AIChatToolExecutor) getHotArticles(ctx context.Context, args map[string]any) (domain.ToolResultData, error) {
	limit := parseLimit(args, 5)
	ids, err := e.intrRepo.ListHotBizIds(ctx, "article", limit)
	if err != nil {
		e.l.Warn("get_hot_articles 查询热门 ID 失败", logger.Error(err))
		return domain.ToolResultData{Name: "get_hot_articles", Error: "获取热门文章失败"}, nil
	}
	if len(ids) == 0 {
		return domain.ToolResultData{Name: "get_hot_articles", Articles: []domain.ArticleCard{}}, nil
	}

	cards := make([]domain.ArticleCard, 0, len(ids))
	for _, id := range ids {
		article, err := e.reader.Detail(ctx, id)
		if err != nil {
			e.l.Warn("get_hot_articles 获取文章详情失败", logger.Int64("articleId", id), logger.Error(err))
			continue
		}
		cards = append(cards, domain.ArticleCard{
			Id:       article.Id,
			Title:    article.Title,
			Abstract: article.Abstract,
			Url:      fmt.Sprintf("/article/%d", article.Id),
		})
	}
	return domain.ToolResultData{Name: "get_hot_articles", Articles: cards}, nil
}

func (e *AIChatToolExecutor) getMyFavorites(ctx context.Context, uid int64, args map[string]any) (domain.ToolResultData, error) {
	limit := parseLimit(args, 5)
	ids, err := e.intrRepo.ListCollectedBizIds(ctx, uid, "article", limit)
	if err != nil {
		e.l.Warn("get_my_favorites 查询收藏 ID 失败", logger.Int64("uid", uid), logger.Error(err))
		return domain.ToolResultData{Name: "get_my_favorites", Error: "获取收藏失败"}, nil
	}
	if len(ids) == 0 {
		return domain.ToolResultData{Name: "get_my_favorites", Articles: []domain.ArticleCard{}}, nil
	}

	cards := make([]domain.ArticleCard, 0, len(ids))
	for _, id := range ids {
		article, err := e.reader.Detail(ctx, id)
		if err != nil {
			// 单篇获取失败静默跳过，不阻断整体
			e.l.Warn("get_my_favorites 获取文章详情失败", logger.Int64("articleId", id), logger.Error(err))
			continue
		}
		cards = append(cards, domain.ArticleCard{
			Id:       article.Id,
			Title:    article.Title,
			Abstract: article.Abstract,
			Url:      fmt.Sprintf("/article/%d", article.Id),
		})
	}
	return domain.ToolResultData{Name: "get_my_favorites", Articles: cards}, nil
}

// toArticleCards 将 domain.Article 切片转为 ArticleCard 切片
func toArticleCards(articles []domain.Article) []domain.ArticleCard {
	cards := make([]domain.ArticleCard, 0, len(articles))
	for _, a := range articles {
		cards = append(cards, domain.ArticleCard{
			Id:       a.Id,
			Title:    a.Title,
			Abstract: a.Abstract,
			Url:      fmt.Sprintf("/article/%d", a.Id),
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
	// JSON 数字默认解析为 float64
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
