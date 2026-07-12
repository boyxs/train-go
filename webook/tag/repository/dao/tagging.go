package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// TaggingDAO 通用多态标签关联（biz + biz_id + source）。
// SyncByBiz 是核心：事务内 diff 目标对象的标签集，增删关联并同步 tag.ref_count（+1/-1）。
type TaggingDAO interface {
	// SyncByBiz 将 (biz,bizId) 的标签全量对齐到 tagIds：新增缺的、删除多的，并同步 ref_count。幂等。
	// 返回 ref_count 发生变化的标签 id（新增 ∪ 删除），供上层精确失效详情缓存。
	SyncByBiz(ctx context.Context, biz string, bizId int64, tagIds []int64, source string) (affectedTagIds []int64, err error)
	// ListByBiz 取某对象的全部标签关联。
	ListByBiz(ctx context.Context, biz string, bizId int64) ([]Tagging, error)
	// BatchByBiz 批量取多个对象的标签关联（列表页消 N+1），返回 bizId → 关联。
	BatchByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]Tagging, error)
	// BizIdsByTag 某标签下某 biz 的对象 id（按关联 created_at DESC，窗口封顶 + total）。通用：biz 入参，名字不绑定具体业务。
	// 不 join 业务表：tag 服务不依赖调用方 schema；下架由调用方清关联保证只剩在架内容。
	BizIdsByTag(ctx context.Context, tagId int64, biz string, limit int) ([]int64, int64, error)
	// CountRecentByTag 统计某标签 created_at >= since 的关联数（"本周新增"，跨 biz、与 ref_count 同口径）。
	CountRecentByTag(ctx context.Context, tagId int64, since int64) (int64, error)
}

type GormTaggingDAO struct {
	db *gorm.DB
}

func NewGormTaggingDAO(db *gorm.DB) TaggingDAO {
	return &GormTaggingDAO{db: db}
}

// SyncByBiz 事务内把 (biz,bizId) 的标签集全量对齐到 tagIds：
// 新增缺的关联并 ref_count+1，删除多的关联并 ref_count-1（GREATEST 防负）。幂等——
// 已对齐则无增删、ref_count 不动。硬删风格，uk_tagging_dedup 保活跃行唯一、无软删幽灵冲突。
// 增删批量化：新增走一次 CreateInBatches，删除走一次 DELETE...IN，ref_count 按同向分组各一条
// UPDATE，替代逐行往返——缩短事务/热门 tag 行锁持有时间（发文低频写路径消 N+1）。
func (d *GormTaggingDAO) SyncByBiz(ctx context.Context, biz string, bizId int64, tagIds []int64, source string) ([]int64, error) {
	var affected []int64
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []Tagging
		if err := tx.Where("biz = ? AND biz_id = ?", biz, bizId).Find(&existing).Error; err != nil {
			return err
		}
		have := make(map[int64]struct{}, len(existing))
		for _, t := range existing {
			have[t.TagId] = struct{}{}
		}
		want := make(map[int64]struct{}, len(tagIds))
		for _, id := range tagIds {
			want[id] = struct{}{}
		}
		// diff：新增(want 有 have 无) / 删除(have 有 want 无)
		var (
			toAdd     []Tagging
			addTagIds []int64
			delTagIds []int64
		)
		for id := range want {
			if _, ok := have[id]; !ok {
				toAdd = append(toAdd, Tagging{TagId: id, Biz: biz, BizId: bizId, Source: source})
				addTagIds = append(addTagIds, id)
			}
		}
		for _, t := range existing {
			if _, ok := want[t.TagId]; !ok {
				delTagIds = append(delTagIds, t.TagId)
			}
		}
		now := time.Now().UnixMilli()
		// 新增：批量插关联 + ref_count 分组 +1
		if len(toAdd) > 0 {
			if err := tx.CreateInBatches(toAdd, 100).Error; err != nil {
				return err
			}
			if err := incrRefCountBatch(tx, addTagIds, 1, now); err != nil {
				return err
			}
		}
		// 删除：一次 DELETE...IN 清关联 + ref_count 分组 -1
		if len(delTagIds) > 0 {
			if err := tx.Where("biz = ? AND biz_id = ? AND tag_id IN ?", biz, bizId, delTagIds).
				Delete(&Tagging{}).Error; err != nil {
				return err
			}
			if err := incrRefCountBatch(tx, delTagIds, -1, now); err != nil {
				return err
			}
		}
		affected = append(affected, addTagIds...)
		affected = append(affected, delTagIds...)
		return nil
	})
	return affected, err
}

// incrRefCountBatch 事务内批量对多个 tag 的 ref_count 同向增减（同 delta），GREATEST(0,...) 防负。
// 新增组 delta=+1、删除组 delta=-1，各一条 UPDATE 替代逐行往返。
func incrRefCountBatch(tx *gorm.DB, tagIds []int64, delta int, now int64) error {
	if len(tagIds) == 0 {
		return nil
	}
	return tx.Model(&Tag{}).Where("id IN ?", tagIds).
		Updates(map[string]any{
			"ref_count":  gorm.Expr("GREATEST(0, ref_count + ?)", delta),
			"updated_at": now,
		}).Error
}

func (d *GormTaggingDAO) ListByBiz(ctx context.Context, biz string, bizId int64) ([]Tagging, error) {
	var list []Tagging
	err := d.db.WithContext(ctx).Where("biz = ? AND biz_id = ?", biz, bizId).Find(&list).Error
	return list, err
}

func (d *GormTaggingDAO) BatchByBiz(ctx context.Context, biz string, bizIds []int64) (map[int64][]Tagging, error) {
	if len(bizIds) == 0 {
		return map[int64][]Tagging{}, nil
	}
	var list []Tagging
	if err := d.db.WithContext(ctx).
		Where("biz = ? AND biz_id IN ?", biz, bizIds).Find(&list).Error; err != nil {
		return nil, err
	}
	m := make(map[int64][]Tagging, len(bizIds))
	for _, t := range list {
		m[t.BizId] = append(m[t.BizId], t)
	}
	return m, nil
}

func (d *GormTaggingDAO) BizIdsByTag(ctx context.Context, tagId int64, biz string, limit int) ([]int64, int64, error) {
	var total int64
	if err := d.db.WithContext(ctx).Model(&Tagging{}).
		Where("tag_id = ? AND biz = ?", tagId, biz).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ids []int64
	err := d.db.WithContext(ctx).Model(&Tagging{}).
		Where("tag_id = ? AND biz = ?", tagId, biz).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Pluck("biz_id", &ids).Error
	return ids, total, err
}

func (d *GormTaggingDAO) CountRecentByTag(ctx context.Context, tagId int64, since int64) (int64, error) {
	var n int64
	err := d.db.WithContext(ctx).Model(&Tagging{}).
		Where("tag_id = ? AND created_at >= ?", tagId, since).
		Count(&n).Error
	return n, err
}

// Tagging 通用多态标签关联（谁·被打了什么·谁打的）。untag = 物理删。
type Tagging struct {
	Id        int64  `gorm:"primaryKey;autoIncrement"`
	TagId     int64  `gorm:"not null;default:0;uniqueIndex:uk_tagging_dedup,priority:3;index:idx_tagging_tag_biz,priority:1"`
	Biz       string `gorm:"type:varchar(32);not null;default:'';uniqueIndex:uk_tagging_dedup,priority:1;index:idx_tagging_tag_biz,priority:2;index:idx_tagging_target,priority:1"`
	BizId     int64  `gorm:"not null;default:0;uniqueIndex:uk_tagging_dedup,priority:2;index:idx_tagging_target,priority:2"`
	Source    string `gorm:"type:varchar(16);not null;default:'author'"`
	CreatedAt int64  `gorm:"autoCreateTime:milli"`
	UpdatedAt int64  `gorm:"autoUpdateTime:milli"`
}

func (Tagging) TableName() string { return "tagging" }
