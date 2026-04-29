package web

import (
	"os"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon/v2"

	"github.com/webook/internal/consts"
	"github.com/webook/internal/domain"
	"github.com/webook/internal/errs"
	"github.com/webook/internal/service"
	"github.com/webook/pkg/ginx"
	"github.com/webook/pkg/logger"
)

// 日期格式 YYYY-MM-DD 校验，防 Redis key 注入
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// RankingHandler 榜单 HTTP 接口
type RankingHandler interface {
	RegisterRoutes(server *gin.Engine)
}

type ArticleRankingHandler struct {
	svc service.RankingService
	l   logger.LoggerX
}

func NewArticleRankingHandler(svc service.RankingService, l logger.LoggerX) RankingHandler {
	return &ArticleRankingHandler{svc: svc, l: l}
}

func (h *ArticleRankingHandler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/article/ranking")
	g.POST("/page", ginx.WrapReq[rankingPageReq](h.Page))
	g.POST("/archive/dates", ginx.Wrap(h.ArchiveDates))
	// archive 是运维 / 测试用接口（手动触发归档，cron 自动归档已覆盖业务），
	// 生产环境禁用以防越权触发：DEPLOY_ENV=prod 时不注册路由。
	// 直接读 os.Getenv 不走 viper：viper.AutomaticEnv 对非嵌套 key 会查 "DEPLOYENV"
	// （SetEnvKeyReplacer 只替换 dot），与 .env 里的 DEPLOY_ENV 对不上，gate 永不触发
	if os.Getenv("DEPLOY_ENV") != "prod" {
		g.POST("/archive", ginx.WrapReq[rankingArchiveReq](h.Archive))
	}
	g.POST("/click", ginx.WrapReqClaims[rankingClickReq, UserClaims](consts.UserKey, h.Click))
}

type rankingPageReq struct {
	Dimension string `json:"dimension"`
	Category  string `json:"category"`
	Date      string `json:"date"` // YYYY-MM-DD，空字符串表示今日
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
}

func (h *ArticleRankingHandler) Page(ctx *gin.Context, req rankingPageReq) (Result, error) {
	if req.Dimension != "" && !domain.Dimension(req.Dimension).Valid() {
		return Result{}, errs.ErrInvalidDimension
	}
	if req.Date != "" && !datePattern.MatchString(req.Date) {
		return Result{}, errs.ErrInvalidDate
	}
	list, total, err := h.svc.Page(ctx, req.Date, req.Dimension, req.Category, req.Page, req.PageSize)
	if err != nil {
		return Result{}, err
	}
	return Result{Msg: "ok", Data: ginx.PageResult{List: list, Total: int64(total)}}, nil
}

func (h *ArticleRankingHandler) ArchiveDates(ctx *gin.Context) (Result, error) {
	dates, err := h.svc.ListArchiveDates(ctx)
	if err != nil {
		return Result{}, err
	}
	return Result{Msg: "ok", Data: dates}, nil
}

type rankingArchiveReq struct {
	Date string `json:"date"` // 空则归档今日
}

// Archive 手动触发归档指定日期的榜单，主要给测试/运维用
func (h *ArticleRankingHandler) Archive(ctx *gin.Context, req rankingArchiveReq) (Result, error) {
	date := req.Date
	if date == "" {
		date = carbon.Now().ToDateString()
	}
	if !datePattern.MatchString(date) {
		return Result{}, errs.ErrInvalidDate
	}
	if err := h.svc.Archive(ctx, date); err != nil {
		return Result{}, err
	}
	return Result{Msg: "ok"}, nil
}

type rankingClickReq struct {
	ArticleId int64  `json:"articleId"`
	Rank      int    `json:"rank"`
	Dimension string `json:"dimension"`
}

// 榜单 Top100，上报 rank 超过这个值视为伪造
const rankingClickMaxRank = 100

func (h *ArticleRankingHandler) Click(ctx *gin.Context, req rankingClickReq, uc UserClaims) (Result, error) {
	if req.ArticleId <= 0 {
		return Result{}, errs.ErrClickInvalidArgs
	}
	if req.Dimension != "" && !domain.Dimension(req.Dimension).Valid() {
		return Result{}, errs.ErrInvalidDimension
	}
	if req.Rank < 0 || req.Rank > rankingClickMaxRank {
		return Result{}, errs.ErrInvalidRank
	}
	if err := h.svc.OnClick(ctx, uc.Userid, req.ArticleId, req.Rank, req.Dimension); err != nil {
		return Result{}, err
	}
	return Result{Msg: "ok"}, nil
}
