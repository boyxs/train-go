package domain

type User struct {
	Id         int64      `json:"id"`
	Email      string     `json:"email"`
	Password   string     `json:"-"` // 不序列化到 JSON
	Nickname   string     `json:"nickname"`
	Birthday   int64      `json:"birthday"`
	AboutMe    string     `json:"aboutMe"`
	Phone      string     `json:"phone"`
	WechatAuth WechatAuth `json:"-"` // 不序列化到 JSON
	CreatedAt  int64      `json:"createdAt"`
	UpdatedAt  int64      `json:"updatedAt"`
}
