package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type ConversationDAO interface {
	Create(ctx context.Context, conv Conversation) (Conversation, error)
	List(ctx context.Context, uid int64) ([]Conversation, error)
	Find(ctx context.Context, uid int64, convId int64) (Conversation, error)
	UpdateTitle(ctx context.Context, uid int64, convId int64, title string) error
	Delete(ctx context.Context, uid int64, convId int64) error
}

type GormConversationDAO struct {
	db *gorm.DB
}

func NewGormConversationDAO(db *gorm.DB) ConversationDAO {
	return &GormConversationDAO{db: db}
}

func (d *GormConversationDAO) Create(ctx context.Context, conv Conversation) (Conversation, error) {
	now := time.Now()
	conv.CreatedAt = now
	conv.UpdatedAt = now
	err := d.db.WithContext(ctx).Create(&conv).Error
	return conv, err
}

func (d *GormConversationDAO) List(ctx context.Context, uid int64) ([]Conversation, error) {
	var convs []Conversation
	err := d.db.WithContext(ctx).
		Where("user_id = ?", uid).
		Order("updated_at DESC").
		Find(&convs).Error
	return convs, err
}

func (d *GormConversationDAO) Find(ctx context.Context, uid int64, convId int64) (Conversation, error) {
	var conv Conversation
	err := d.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", convId, uid).
		First(&conv).Error
	return conv, err
}

func (d *GormConversationDAO) UpdateTitle(ctx context.Context, uid int64, convId int64, title string) error {
	return d.db.WithContext(ctx).
		Model(&Conversation{}).
		Where("id = ? AND user_id = ?", convId, uid).
		Updates(map[string]any{
			"title":      title,
			"updated_at": time.Now(),
		}).Error
}

func (d *GormConversationDAO) Delete(ctx context.Context, uid int64, convId int64) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", convId).Delete(&Message{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND user_id = ?", convId, uid).Delete(&Conversation{}).Error
	})
}

// Conversation GORM 模型
type Conversation struct {
	Id        int64  `gorm:"primaryKey,autoIncrement"`
	UserId    int64  `gorm:"index:idx_user_updated;not null"`
	Title     string `gorm:"type:varchar(128);not null;default:''"`
	CreatedAt time.Time
	UpdatedAt time.Time `gorm:"index:idx_user_updated"`
}

func (Conversation) TableName() string {
	return "conversation"
}
