package domain

import "time"

type Article struct {
	Id        int64         `json:"id"`
	Title     string        `json:"title"`
	Content   string        `json:"content"`
	Author    Author        `json:"author"`
	Status    ArticleStatus `json:"status"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type Author struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
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
