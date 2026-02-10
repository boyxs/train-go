package domain

type User struct {
	Id       int64
	Email    string
	Password string
	Nickname string
	Birthday string
	AboutMe  string
	Phone    string
	// UTC 0 的时区
	//CreatedAt time.Time
	//UpdatedAt time.Time
	// 字符串类型
	CreatedAt string
	UpdatedAt string
}
