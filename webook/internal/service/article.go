package service

import (
	"context"
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

type InternalArticleAuthorService struct {
	authorRepo repository.ArticleAuthorRepository
	readerRepo repository.ArticleReaderRepository
	searchSvc  ArticleSearchService
	l          logger.LoggerX
}

func NewInternalArticleAuthorService(
	authorRepo repository.ArticleAuthorRepository,
	readerRepo repository.ArticleReaderRepository,
	searchSvc ArticleSearchService,
	l logger.LoggerX,
) ArticleAuthorService {
	return &InternalArticleAuthorService{
		authorRepo: authorRepo,
		readerRepo: readerRepo,
		searchSvc:  searchSvc,
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
	// 从 DB 回查完整数据（含 AuthorName / CreatedAt），再写入 ES
	go func(articleId, uid int64) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		complete, err := s.authorRepo.FindById(bgCtx, articleId, uid)
		if err != nil {
			s.l.Error("索引文章：回查完整数据失败",
				logger.Int64("articleId", articleId), logger.Error(err))
			return
		}
		if err := s.searchSvc.IndexArticle(bgCtx, complete); err != nil {
			s.l.Error("索引文章失败", logger.Int64("articleId", articleId), logger.Error(err))
		}
	}(id, article.Author.Id)
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
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.searchSvc.RemoveArticle(bgCtx, id); err != nil {
			s.l.Error("移除搜索索引失败", logger.Int64("articleId", id), logger.Error(err))
		}
	}()
	return nil
}

func (s *InternalArticleAuthorService) Detail(ctx context.Context, id int64, uid int64) (domain.Article, error) {
	return s.authorRepo.FindById(ctx, id, uid)
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

func (s *InternalArticleAuthorService) Delete(ctx context.Context, id int64, uid int64) error {
	if err := s.authorRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	if err := s.readerRepo.Delete(ctx, id, uid); err != nil {
		return err
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.searchSvc.RemoveArticle(bgCtx, id); err != nil {
			s.l.Error("移除搜索索引失败", logger.Int64("articleId", id), logger.Error(err))
		}
	}()
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
		s.l.Error("批量获取文章互动数据失败，降级", logger.Error(err))
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
		s.l.Error("批量获取文章评论数失败，降级", logger.Error(err))
		return map[int64]int64{}
	}
	return resp.GetCounts()
}

// likedTotalByAuthor 聚合作者全部已发布文章的点赞数（他人主页「获赞」）；失败降级 0。
func (s *InternalArticleReaderService) likedTotalByAuthor(ctx context.Context, uid int64) int64 {
	ids, err := s.readerRepo.ListIdsByAuthor(ctx, uid)
	if err != nil {
		s.l.Error("获取作者文章 id 失败", logger.Int64("author_id", uid), logger.Error(err))
		return 0
	}
	if len(ids) == 0 {
		return 0
	}
	m, err := s.intrSvc.FindByBizIds(ctx, domain.BizArticle, ids)
	if err != nil {
		s.l.Error("聚合获赞失败", logger.Int64("author_id", uid), logger.Error(err))
		return 0
	}
	var total int64
	for _, intr := range m {
		total += intr.LikeCount
	}
	return total
}
