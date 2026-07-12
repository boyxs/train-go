package dao

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TagDAO 标签本体：find-or-create（并发靠 uni(slug,type) 兜底）、批量取名、typeahead 前缀补全。
type TagDAO interface {
	// UpsertTags 批量 find-or-create（幂等 ensure，不覆盖既有 name/description）：一次 INSERT
	// ON CONFLICT DO NOTHING 建缺失，再一次回查取全部真实 id。替代逐个 Upsert 的 N 次串行往返。
	UpsertTags(ctx context.Context, tags []Tag) ([]Tag, error)
	// FindBySlugs 批量按 slug 取（facet/推荐补名）。
	FindBySlugs(ctx context.Context, slugs []string) ([]Tag, error)
	// FindByIds 批量按 id 取（列表每项标签补全）。
	FindByIds(ctx context.Context, ids []int64) ([]Tag, error)
	// Suggest 前缀补全（name/slug 前缀，ref_count DESC，limit 截断）。
	Suggest(ctx context.Context, prefix string, limit int) ([]Tag, error)
}

type GormTagDAO struct {
	db *gorm.DB
}

func NewGormTagDAO(db *gorm.DB) TagDAO {
	return &GormTagDAO{db: db}
}

// UpsertTags 批量 find-or-create：uni(slug,type) 冲突 DoNothing 只建缺失、不覆盖既有 name/description，
// 并发下他人已建天然兜底；再按 (type,slug) 一次回查取全部真实 id（含并发新建）。入参需同一 type、slug 已去重。
func (d *GormTagDAO) UpsertTags(ctx context.Context, tags []Tag) ([]Tag, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	db := d.db.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(tags, 100).Error; err != nil {
		return nil, err
	}
	typ := tags[0].Type
	slugs := make([]string, 0, len(tags))
	for _, t := range tags {
		slugs = append(slugs, t.Slug)
	}
	var list []Tag
	if err := db.Where("type = ? AND slug IN ?", typ, slugs).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (d *GormTagDAO) FindBySlugs(ctx context.Context, slugs []string) ([]Tag, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	var list []Tag
	err := d.db.WithContext(ctx).Where("slug IN ?", slugs).Find(&list).Error
	return list, err
}

func (d *GormTagDAO) FindByIds(ctx context.Context, ids []int64) ([]Tag, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []Tag
	err := d.db.WithContext(ctx).Where("id IN ?", ids).Find(&list).Error
	return list, err
}

// Suggest 前缀补全：name/slug 前缀匹配（utf8mb4_0900_ai_ci 大小写不敏感），ref_count DESC。
// 空前缀返回空——不建议全表扫（typeahead 语义：无输入无候选）。
func (d *GormTagDAO) Suggest(ctx context.Context, prefix string, limit int) ([]Tag, error) {
	if prefix == "" {
		return nil, nil
	}
	like := escapeLike(prefix) + "%"
	var list []Tag
	err := d.db.WithContext(ctx).
		Where("name LIKE ? OR slug LIKE ?", like, like).
		Order("ref_count DESC, id ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// escapeLike 转义 LIKE 元字符（\ % _），令前缀里的通配符按字面量匹配（MySQL 默认转义符 \）。
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Tag 标签本体（与被标注对象解耦）。MVP 无删除路径，硬删风格，不加 soft_delete。
type Tag struct {
	Id          int64  `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"type:varchar(30);not null;default:''"`
	Slug        string `gorm:"type:varchar(30);not null;default:'';uniqueIndex:uk_tag_slug,priority:1"`
	Type        string `gorm:"type:varchar(16);not null;default:'topic';uniqueIndex:uk_tag_slug,priority:2;index:idx_tag_type_refcount,priority:1"`
	Description string `gorm:"type:varchar(255);not null;default:''"`
	RefCount    int64  `gorm:"not null;default:0;index:idx_tag_type_refcount,priority:2"`
	FollowCount int64  `gorm:"not null;default:0"` // 关注数（多少用户关注此标签）
	CreatedAt   int64  `gorm:"autoCreateTime:milli"`
	UpdatedAt   int64  `gorm:"autoUpdateTime:milli"`
}

func (Tag) TableName() string { return "tag" }
