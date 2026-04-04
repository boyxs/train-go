package dao

import (
	"context"

	"gorm.io/gorm"
)

type MessageDAO interface {
	Insert(ctx context.Context, msg Message) (Message, error)
	// ListRecent 获取最新 limit 条（初始加载），返回按 created_at ASC
	ListRecent(ctx context.Context, convId int64, limit int) ([]Message, error)
	// ListBefore 获取 id < beforeId 的 limit 条（上滑加载更早），返回按 created_at ASC
	ListBefore(ctx context.Context, convId int64, beforeId int64, limit int) ([]Message, error)
	// ListAll 获取全部消息（buildPrompt 用）
	ListAll(ctx context.Context, convId int64) ([]Message, error)
}

type GormMessageDAO struct {
	db *gorm.DB
}

func NewGormMessageDAO(db *gorm.DB) MessageDAO {
	return &GormMessageDAO{db: db}
}

func (d *GormMessageDAO) Insert(ctx context.Context, msg Message) (Message, error) {
	// CreatedAt 由 GORM autoCreateTime:milli 自动填充
	err := d.db.WithContext(ctx).Create(&msg).Error
	return msg, err
}

func (d *GormMessageDAO) ListRecent(ctx context.Context, convId int64, limit int) ([]Message, error) {
	var msgs []Message
	// 子查询取最新 N 条（DESC），外层再 ASC 排列
	err := d.db.WithContext(ctx).
		Where("conversation_id = ?", convId).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	// 反转为 ASC 顺序
	reverse(msgs)
	return msgs, nil
}

func (d *GormMessageDAO) ListBefore(ctx context.Context, convId int64, beforeId int64, limit int) ([]Message, error) {
	var msgs []Message
	err := d.db.WithContext(ctx).
		Where("conversation_id = ? AND id < ?", convId, beforeId).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	reverse(msgs)
	return msgs, nil
}

func (d *GormMessageDAO) ListAll(ctx context.Context, convId int64) ([]Message, error) {
	var msgs []Message
	err := d.db.WithContext(ctx).
		Where("conversation_id = ?", convId).
		Order("created_at ASC").
		Find(&msgs).Error
	return msgs, err
}

func reverse(msgs []Message) {
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
}

// Message GORM 模型
type Message struct {
	Id             int64   `gorm:"primaryKey,autoIncrement"`
	ConversationId int64   `gorm:"index:idx_conv_created;not null"`
	Role           string  `gorm:"type:varchar(16);not null"`
	Content        string  `gorm:"type:text;not null"`
	ToolCalls      *string `gorm:"type:json"`
	TokenUsed      int     `gorm:"not null;default:0"`
	CreatedAt      int64   `gorm:"autoCreateTime:milli;index:idx_conv_created"`
}

func (Message) TableName() string {
	return "message"
}
