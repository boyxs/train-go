package domain

import "time"

type Article struct {
	Id        int64
	Title     string
	Content   string
	Author    Author
	Status    ArticleStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Author struct {
	Id   int64
	Name string
}

type ArticleStatus uint8

func (as ArticleStatus) ToUint8() uint8 {
	return uint8(as)
}

const (
	ArticleStatusUnknown     ArticleStatus = iota //未知状态
	ArticleStatusUnpublished                      //未发表
	ArticleStatusPublished                        //已发表
	ArticleStatusPrivate                          //仅自己可见
)
