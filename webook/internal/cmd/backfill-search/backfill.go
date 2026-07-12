package main

import (
	"context"
	"fmt"

	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	tagv1 "github.com/boyxs/train-go/webook/api/gen/tag/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/repository/dao"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// backfillBatch 每批分页大小；TagsByBiz 一次批量取标签、逐篇 IndexArticle。
const backfillBatch = 100

// SearchBackfiller 存量已发表文章 → webook-search ES 索引的一次性回填工具。
//
// 复用发布写路径的下游契约（tag.TagsByBiz + search.IndexArticle），但语义是「重建索引」而非「同步标签」：
// 逐篇取 published_article 当前已解析的标签一起写 ES，绝不调 SyncTags（那会按空 names 清掉已打的标签）。
// 幂等：IndexArticle 按 id 覆盖写别名，重复跑安全。
type SearchBackfiller struct {
	dao       dao.ArticleReaderDAO
	searchCli searchv1.SearchServiceClient
	tagCli    tagv1.TagServiceClient
	l         logger.LoggerX
}

func NewSearchBackfiller(d dao.ArticleReaderDAO, searchCli searchv1.SearchServiceClient, tagCli tagv1.TagServiceClient, l logger.LoggerX) *SearchBackfiller {
	return &SearchBackfiller{dao: d, searchCli: searchCli, tagCli: tagCli, l: l}
}

// Run 分页遍历 published_article，逐篇带当前标签写 ES。
// 返回 error 仅代表「读源库失败」这类致命错误；单篇 IndexArticle 失败计数后继续（不中断整批）。
func (b *SearchBackfiller) Run(ctx context.Context) error {
	total, err := b.dao.Count(ctx)
	if err != nil {
		return fmt.Errorf("统计 published_article 总数: %w", err)
	}
	b.l.Info("backfill 开始", logger.Int64("total", total), logger.Int("batch", backfillBatch))

	var indexed, failed int
	for offset := 0; ; offset += backfillBatch {
		rows, err := b.dao.Page(ctx, offset, backfillBatch)
		if err != nil {
			return fmt.Errorf("分页读取 published_article offset=%d: %w", offset, err)
		}
		if len(rows) == 0 {
			break
		}
		ids := make([]int64, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.Id)
		}
		tagMap := b.tagSlugsByBiz(ctx, ids)
		for _, r := range rows {
			doc := toArticleDoc(r, tagMap[r.Id])
			if _, err := b.searchCli.IndexArticle(ctx, &searchv1.IndexArticleRequest{Doc: doc}); err != nil {
				failed++
				b.l.Error("索引文章失败", logger.Int64("id", r.Id), logger.Error(err))
				continue
			}
			indexed++
		}
		b.l.Info("backfill 进度",
			logger.Int("indexed", indexed), logger.Int("failed", failed), logger.Int64("total", total))
	}

	b.l.Info("backfill 完成", logger.Int("indexed", indexed), logger.Int("failed", failed))
	// 提醒：search.Index 对 embed/ES 写失败是「非致命降级」（gRPC 仍返成功），本工具无法感知静默失败。
	// 完整性以 `make -f mk/es.mk count` 复核 ES 实际文档数。
	b.l.Info("完整性复核请跑 `make -f mk/es.mk count`（search.Index 对 ES 写失败静默降级，gRPC 不报错）")
	if failed > 0 {
		return fmt.Errorf("backfill 完成，但有 %d/%d 篇 gRPC 索引失败（见日志）", failed, total)
	}
	return nil
}

// tagSlugsByBiz 批量取每篇文章当前已解析的标签 slug；失败整批降级不带标签（不阻断回填）。
func (b *SearchBackfiller) tagSlugsByBiz(ctx context.Context, ids []int64) map[int64][]string {
	resp, err := b.tagCli.TagsByBiz(ctx, &tagv1.TagsByBizRequest{Biz: domain.BizArticle, BizIds: ids})
	if err != nil {
		b.l.Warn("批量取标签失败，本批降级不带标签索引", logger.Error(err))
		return map[int64][]string{}
	}
	out := make(map[int64][]string, len(resp.GetTags()))
	for bizId, list := range resp.GetTags() {
		slugs := make([]string, 0, len(list.GetTags()))
		for _, t := range list.GetTags() {
			slugs = append(slugs, t.GetSlug())
		}
		out[bizId] = slugs
	}
	return out
}

// toArticleDoc published_article 行 → search ES 文档；author_name 两侧读路径均不带（与发布索引语义一致），
// search 服务端据 title/abstract 现算 content_vec。
func toArticleDoc(r dao.PublishedArticle, tagSlugs []string) *searchv1.ArticleDoc {
	return &searchv1.ArticleDoc{
		Id:         r.Id,
		Title:      r.Title,
		Abstract:   r.Abstract,
		AuthorId:   r.AuthorId,
		AuthorName: "",
		Status:     uint32(r.Status),
		Category:   r.Category,
		Tags:       tagSlugs,
		CreatedAt:  r.CreatedAt,
	}
}
