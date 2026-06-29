package dao

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/plugin/soft_delete"
)

// Comment 评论 DAO 模型。
// 盖楼无限嵌套：RootId 标记所属楼（一级评论=0，回复=楼主评论 id），Pid 直接父评论（一级=NULL）。
// 整楼回复靠 RootId 一索引查出；Pid 自关联外键保证父子引用完整性。
// 点赞/热度由 interaction 服务（biz="comment"）负责，本表不存 like_cnt、不按热度排序。
type Comment struct {
	Id     int64         `gorm:"primaryKey,autoIncrement"`
	Biz    string        `gorm:"type:varchar(32);index:idx_comment_biz_root,priority:1"`
	BizId  int64         `gorm:"index:idx_comment_biz_root,priority:2"`
	Uid    int64         `gorm:"index:idx_comment_uid"`
	RootId int64         `gorm:"index:idx_comment_biz_root,priority:3;index:idx_comment_root"`
	Pid    sql.NullInt64 `gorm:"index"` // 直接父评论 ID；一级评论为 NULL
	// ParentComment 自关联外键。注意：删除走软删（Delete 在 soft_delete 模型上是 UPDATE deleted_at），
	// OnDelete:CASCADE 仅在物理 DELETE 时触发，软删场景不级联，子回复树得以保留（与原型"保留子树"一致）。
	ParentComment *Comment `gorm:"foreignKey:Pid;references:Id;constraint:OnDelete:CASCADE"`
	Content       string   `gorm:"type:varchar(1000)"`
	ReplyCnt      int64    // 回复数：一级评论=整楼回复数（写/删回复时增减楼根，对齐扁平展开条数）；楼内回复恒 0
	// Deleted 软删除占位标记：删除「有子回复」的评论时不真删，置 true 并清空 Content，
	// 行保留让查询返回占位（前端渲染「该评论已删除」），子回复树得以挂在其下（保住讨论串）。
	Deleted bool `gorm:"not null;default:0"`

	CreatedAt int64                 `gorm:"autoCreateTime:milli"`
	UpdatedAt int64                 `gorm:"autoUpdateTime:milli"`
	DeletedAt soft_delete.DeletedAt `gorm:"softDelete:milli"`
}

func (Comment) TableName() string {
	return "comment"
}

// CommentDAO 评论数据访问。
type CommentDAO interface {
	// Insert 写入评论；回复（Pid 有效）时自动计算 RootId 并维护父评论 ReplyCnt。
	Insert(ctx context.Context, c Comment) (Comment, error)
	FindById(ctx context.Context, id int64) (Comment, error)
	// BatchGet 按 id 批量取评论（供 core 拿 interaction.GetHotBizIds 的热门 id 后回查详情）。
	BatchGet(ctx context.Context, ids []int64) ([]Comment, error)
	// PageRoots 分页查一级评论（RootId=0），按 id 倒序（最新）。最热排序由 core+interaction 负责。
	PageRoots(ctx context.Context, biz string, bizId int64, offset, limit int) ([]Comment, error)
	// ListReplies 查整楼回复（RootId=rootId），按时间正序。
	ListReplies(ctx context.Context, rootId int64, offset, limit int) ([]Comment, error)
	// Delete 删除评论，仅当 uid 为评论作者本人；实现为软删（置 deleted_at，保留子回复树）。返回是否实际删除。
	Delete(ctx context.Context, id, uid int64) (bool, error)
	Count(ctx context.Context, biz string, bizId int64) (int64, error)
}

type GormCommentDAO struct {
	db *gorm.DB
}

func NewGormCommentDAO(db *gorm.DB) CommentDAO {
	return &GormCommentDAO{db: db}
}

func (d *GormCommentDAO) Insert(ctx context.Context, c Comment) (Comment, error) {
	// 一级评论（Pid 无效）：RootId 恒为 0，直接写入。
	if !c.Pid.Valid {
		c.RootId = 0
		err := d.db.WithContext(ctx).Create(&c).Error
		return c, err
	}
	// 回复：事务内取父评论算 RootId（父是一级则指向父 id，否则继承父 RootId），并 +1 楼根 ReplyCnt。
	// reply_cnt 维护在「楼根(root_id)」而非直接父——一级评论 reply_cnt 即整楼回复数，
	// 与扁平展开（GetReplies 按 root_id 拉整楼）的条数一致，避免多级盖楼时少算。
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent Comment
		if err := tx.Where("id = ?", c.Pid.Int64).First(&parent).Error; err != nil {
			return err
		}
		if parent.RootId == 0 {
			c.RootId = parent.Id
		} else {
			c.RootId = parent.RootId
		}
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		return tx.Model(&Comment{}).Where("id = ?", c.RootId).
			Update("reply_cnt", gorm.Expr("reply_cnt + 1")).Error
	})
	return c, err
}

func (d *GormCommentDAO) FindById(ctx context.Context, id int64) (Comment, error) {
	var c Comment
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&c).Error
	return c, err
}

func (d *GormCommentDAO) BatchGet(ctx context.Context, ids []int64) ([]Comment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []Comment
	err := d.db.WithContext(ctx).Where("id IN ?", ids).Find(&list).Error
	return list, err
}

func (d *GormCommentDAO) PageRoots(ctx context.Context, biz string, bizId int64, offset, limit int) ([]Comment, error) {
	var list []Comment
	err := d.db.WithContext(ctx).
		Where("biz = ? AND biz_id = ? AND root_id = 0", biz, bizId).
		Order("id DESC").Offset(offset).Limit(limit).
		Find(&list).Error
	return list, err
}

func (d *GormCommentDAO) ListReplies(ctx context.Context, rootId int64, offset, limit int) ([]Comment, error) {
	var list []Comment
	err := d.db.WithContext(ctx).
		Where("root_id = ?", rootId).
		Order("id ASC").Offset(offset).Limit(limit).
		Find(&list).Error
	return list, err
}

func (d *GormCommentDAO) Delete(ctx context.Context, id, uid int64) (bool, error) {
	var deleted bool
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// WHERE 限定 uid 实现鉴权：非作者本人或评论不存在 → 匹配不到，deleted 保持 false
		var c Comment
		if err := tx.Where("id = ? AND uid = ?", id, uid).First(&c).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		// 是否有子回复（任何 pid=id 的行，含已是占位的）
		var childCount int64
		if err := tx.Model(&Comment{}).Where("pid = ?", id).Count(&childCount).Error; err != nil {
			return err
		}
		if childCount > 0 {
			// 有子回复：占位保留——标记删除 + 清空内容，行不删，子树仍可查（占位仍显示，不减父 reply_cnt）
			if err := tx.Model(&Comment{}).Where("id = ?", id).Updates(map[string]any{
				"deleted":    true,
				"content":    "",
				"updated_at": time.Now().UnixMilli(),
			}).Error; err != nil {
				return err
			}
		} else {
			// 无子回复：软删（置 deleted_at）从视图消失
			if err := tx.Where("id = ?", id).Delete(&Comment{}).Error; err != nil {
				return err
			}
			// 若删的是回复（RootId!=0），递减其楼根的 reply_cnt（与 Insert 的 +1 楼根对称，防"展开 N 条"虚高）
			if c.RootId != 0 {
				if err := tx.Model(&Comment{}).Where("id = ?", c.RootId).
					Update("reply_cnt", gorm.Expr("GREATEST(0, reply_cnt - 1)")).Error; err != nil {
					return err
				}
			}
		}
		deleted = true
		return nil
	})
	return deleted, err
}

func (d *GormCommentDAO) Count(ctx context.Context, biz string, bizId int64) (int64, error) {
	var n int64
	// 占位（deleted=true）不计入"N 条评论"
	err := d.db.WithContext(ctx).Model(&Comment{}).
		Where("biz = ? AND biz_id = ? AND deleted = ?", biz, bizId, false).
		Count(&n).Error
	return n, err
}
