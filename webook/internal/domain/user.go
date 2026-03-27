package domain

import "time"

type User struct {
	Id         int64      `json:"id"`
	Email      string     `json:"email"`
	Password   string     `json:"-"` // 不序列化到 JSON
	Nickname   string     `json:"nickname"`
	Birthday   time.Time  `json:"birthday"`
	AboutMe    string     `json:"aboutMe"`
	Phone      string     `json:"phone"`
	WechatAuth WechatAuth `json:"-"` // 不序列化到 JSON
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
