package service

import (
	"context"
	"runtime/debug"
	"time"

	commentv1 "github.com/boyxs/train-go/webook/api/gen/comment/v1"
	"github.com/boyxs/train-go/webook/internal/domain"
	"github.com/boyxs/train-go/webook/internal/repository"
	"github.com/boyxs/train-go/webook/pkg/logger"
)

// ── 作者端 ─────────────────────────────────────────────────────────────────

type ArticleAuthorService interface {
	Edit(ctx context.Context, article domain.Article) (int64, error)
	Publish(ctx context.Context, article domain.Article) (int64, error)
	Withdraw(ctx context.Context, id int64, uid int64) error
	Detail(ctx context.Context, id int64, uid int64) (domain.Article, error)
	Page(ctx context.Context, uid int64, page int, pageSize int) ([]domain.Article, int64, error)
	List(ctx context.Context, uid int64) ([]domain.Article, error)
	Delete(ctx context.Context, id int64, uid int64) error
}

// articleTagSource 发布时标签来源标记（作者手动输入；AI 推荐入库同理走 tag 服务另计）。
const articleTagSource = "author"

type InternalArticleAuthorService struct {
	authorRepo repository.ArticleAuthorRepository
	readerRepo repository.ArticleReaderRepository
	searchSvc  ArticleSearchService
	tagSvc     TagService
	l          logger.LoggerX
}

func NewInternalArticleAuthorService(
	authorRepo repository.ArticleAuthorRepository,
	readerRepo repository.ArticleReaderRepository,
	searchSvc ArticleSearchService,
	tagSvc TagService,
	l logger.LoggerX,
) ArticleAuthorService {
	return &InternalArticleAuthorService{
		authorRepo: authorRepo,
		readerRepo: readerRepo,
		searchSvc:  searchSvc,
		tagSvc:     tagSvc,
		l:          l,
	}
}

func (s *InternalArticleAuthorService) Edit(ctx context.Context, article domain.Article) (int64, error) {
	article.Status = domain.ArticleStatusUnpublished
	if article.Id > 0 {
		err := s.authorRepo.Update(ctx, article)
		return article.Id, err
	}
	return s.authorRepo.Create(ctx, article)
}

func (s *InternalArticleAuthorService) Publish(ctx context.Context, article domain.Article) (int64, error) {
	article.Status = domain.ArticleStatusPublished
	var id int64
	var err error
	if article.Id > 0 {
		err = s.authorRepo.Update(ctx, article)
		id = article.Id
	} else {
		id, err = s.authorRepo.Create(ctx, article)
	}
	if err != nil {
		return 0, err
	}
	article.Id = id
	err = s.readerRepo.Upsert(ctx, article)
	if err != nil {
		return 0, err
	}
	// 后台：全量对齐标签（拿回已解析 slug）+ 回查完整数据（含 AuthorName / CreatedAt）写 ES。
	// 跨 core+tag+search 三库无事务，tag/search 失败非致命降级（记日志，靠重发/backfill 最终一致）。
	uid, tagNames := article.Author.Id, article.Tags
	s.goSafe("发布文章-同步标签+索引", func(bgCtx context.Context) {
		var slugs []string
		tags, err := s.tagSvc.SyncTags(bgCtx, domain.BizArticle, id, tagNames, articleTagSource)
		if err != nil {
			s.l.Error("发布文章：同步标签失败，降级不带标签索引",
				logger.Int64("articleId", id), logger.Error(err))
		} else {
			slugs = make([]string, 0, len(tags))
			for _, t := range tags {
				slugs = append(slugs, t.Slug)
			}
		}
		complete, err := s.authorRepo.FindById(bgCtx, id, uid)
		if err != nil {
			s.l.Error("索引文章：回查完整数据失败",
				logger.Int64("articleId", id), logger.Error(err))
			return
		}
		complete.Tags = slugs
		if err := s.searchSvc.Index(bgCtx, complete); err != nil {
			s.l.Error("索引文章失败", logger.Int64("articleId", id), logger.Error(err))
		}
	})
	return id, nil
}

func (s *InternalArticleAuthorService) Withdraw(ctx context.Context, id int64, uid int64) error {
	// UpdateStatus 自带权限校验（WHERE author_id=uid AND status=from），RowsAffected=0 即失败
	err := s.authorRepo.UpdateStatus(ctx, id, uid,
		domain.ArticleStatusPublished.ToUint8(),
		domain.ArticleStatusPrivate.ToUint8())
	if err != nil {
		return err
	}
	if err := s.readerRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	s.clearTagAndIndexAsync(id)
	return nil
}

func (s *InternalArticleAuthorService) Detail(ctx context.Context, id int64, uid int64) (domain.Article, error) {
	article, err := s.authorRepo.FindById(ctx, id, uid)
	if err != nil {
		return domain.Article{}, err
	}
	// 回显：补该文当前标签名（编辑器预填）；tag 服务失败降级不带标签，不阻断详情。
	tagMap, tErr := s.tagSvc.TagsByBiz(ctx, domain.BizArticle, []int64{id})
	if tErr != nil {
		s.l.WithContext(ctx).Error("文章详情：取标签失败，降级不带标签",
			logger.Int64("articleId", id), logger.Error(tErr))
		return article, nil
	}
	names := make([]string, 0, len(tagMap[id]))
	for _, t := range tagMap[id] {
		names = append(names, t.Name)
	}
	article.Tags = names
	return article, nil
}

func (s *InternalArticleAuthorService) Page(ctx context.Context, uid int64, page int, pageSize int) ([]domain.Article, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return s.authorRepo.Page(ctx, uid, offset, pageSize)
}

func (s *InternalArticleAuthorService) List(ctx context.Context, uid int64) ([]domain.Article, error) {
	return s.authorRepo.List(ctx, uid)
}

// clearTagAndIndexAsync 下架/删除后台清理：清标签关联 + 移除搜索索引，均非致命降级（Withdraw/Delete 共用）。
func (s *InternalArticleAuthorService) clearTagAndIndexAsync(id int64) {
	s.goSafe("下架文章-清标签+移除索引", func(bgCtx context.Context) {
		if err := s.tagSvc.ClearTags(bgCtx, domain.BizArticle, id); err != nil {
			s.l.Error("清标签关联失败", logger.Int64("articleId", id), logger.Error(err))
		}
		if err := s.searchSvc.Remove(bgCtx, id); err != nil {
			s.l.Error("移除搜索索引失败", logger.Int64("articleId", id), logger.Error(err))
		}
	})
}

// goSafe 起后台 goroutine 执行 detached 任务：兜 panic（防单个后台任务拖垮整个进程，
// gin Recovery 只覆盖请求 goroutine）+ 统一 30s 超时。用 context.Background()（非请求 ctx）：
// 请求返回后任务仍需完成，不随请求取消。镜像 pkg/cronx、pkg/pool 的 recover 约定。
func (s *InternalArticleAuthorService) goSafe(task string, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.l.Error("后台任务 panic",
					logger.String("task", task),
					logger.Field{Key: "panic", Val: r},
					logger.String("stack", string(debug.Stack())))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fn(ctx)
	}()
}

func (s *InternalArticleAuthorService) Delete(ctx context.Context, id int64, uid int64) error {
	if err := s.authorRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	if err := s.readerRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	s.clearTagAndIndexAsync(id)
	return nil
}

// ── 读者端 ─────────────────────────────────────────────────────────────────

type ArticleReaderService interface {
	Detail(ctx context.Context, id int64) (domain.Article, error)
	// BatchDetail 批量取文章详情；不存在的 id 静默跳过，返回保留入参顺序
	BatchDetail(ctx context.Context, ids []int64) ([]domain.Article, error)
	Page(ctx context.Context, page int, pageSize int) ([]domain.Article, int64, error)
	// AuthorArticles 他人主页「TA 的文章」：某作者已发布文章分页 + 每篇互动/评论计数 + 获赞总数。
	// 跨 interaction / comment 聚合的业务逻辑集中在此，web 只映射 VO。
	AuthorArticles(ctx context.Context, uid int64, page int, pageSize int) (items []domain.ArticleWithStats, total int64, likedTotal int64, err error)
}

type InternalArticleReaderService struct {
	readerRepo repository.ArticleReaderRepository
	intrSvc    InteractionService
	commentCli commentv1.CommentServiceClient
	l          logger.LoggerX
}

func NewInternalArticleReaderService(
	readerRepo repository.ArticleReaderRepository,
	intrSvc InteractionService,
	commentCli commentv1.CommentServiceClient,
	l logger.LoggerX,
) ArticleReaderService {
	return &InternalArticleReaderService{
		readerRepo: readerRepo,
		intrSvc:    intrSvc,
		commentCli: commentCli,
		l:          l,
	}
}

func (s *InternalArticleReaderService) Detail(ctx context.Context, id int64) (domain.Article, error) {
	return s.readerRepo.FindById(ctx, id)
}

func (s *InternalArticleReaderService) BatchDetail(ctx context.Context, ids []int64) ([]domain.Article, error) {
	return s.readerRepo.FindByIds(ctx, ids)
}

func (s *InternalArticleReaderService) Page(ctx context.Context, page int, pageSize int) ([]domain.Article, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return s.readerRepo.Page(ctx, offset, pageSize)
}

func (s *InternalArticleReaderService) AuthorArticles(ctx context.Context, uid int64, page int, pageSize int) ([]domain.ArticleWithStats, int64, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	articles, total, err := s.readerRepo.PageByAuthor(ctx, uid, offset, pageSize)
	if err != nil {
		return nil, 0, 0, err
	}
	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.Id)
	}
	intrMap := s.batchInteraction(ctx, ids)
	cmtMap := s.commentCounts(ctx, ids)
	items := make([]domain.ArticleWithStats, 0, len(articles))
	for _, a := range articles {
		items = append(items, domain.ArticleWithStats{
			Article:    a,
			ReadCnt:    intrMap[a.Id].ReadCount,
			LikeCnt:    intrMap[a.Id].LikeCount,
			CommentCnt: cmtMap[a.Id],
		})
	}
	return items, total, s.likedTotalByAuthor(ctx, uid), nil
}

// batchInteraction 批量取互动计数；失败降级空 map，不阻断整页。
func (s *InternalArticleReaderService) batchInteraction(ctx context.Context, ids []int64) map[int64]domain.Interaction {
	if len(ids) == 0 {
		return map[int64]domain.Interaction{}
	}
	m, err := s.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.WithContext(ctx).Error("批量文章互动计数失败，降级填零", logger.Error(err))
		return map[int64]domain.Interaction{}
	}
	return m
}

// commentCounts 一次 BatchCountComment 取评论数（消 N+1）；失败降级空 map。
func (s *InternalArticleReaderService) commentCounts(ctx context.Context, ids []int64) map[int64]int64 {
	if len(ids) == 0 {
		return map[int64]int64{}
	}
	resp, err := s.commentCli.BatchCountComment(ctx, &commentv1.BatchCountCommentRequest{Biz: domain.BizArticle, BizIds: ids})
	if err != nil {
		s.l.WithContext(ctx).Error("批量获取文章评论数失败，降级", logger.Error(err))
		return map[int64]int64{}
	}
	return resp.GetCounts()
}

// likedTotalByAuthor 聚合作者全部已发布文章的点赞数（他人主页「获赞」）；失败降级 0。
func (s *InternalArticleReaderService) likedTotalByAuthor(ctx context.Context, uid int64) int64 {
	ids, err := s.readerRepo.ListIdsByAuthor(ctx, uid)
	if err != nil {
		s.l.WithContext(ctx).Error("获取作者文章 id 失败", logger.Int64("author_id", uid), logger.Error(err))
		return 0
	}
	if len(ids) == 0 {
		return 0
	}
	m, err := s.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.WithContext(ctx).Error("聚合获赞失败", logger.Int64("author_id", uid), logger.Error(err))
		return 0
	}
	var total int64
	for _, intr := range m {
		total += intr.LikeCount
	}
	return total
}
