package domain

type Article struct {
	Id        int64         `json:"id"`
	Title     string        `json:"title"`
	Content   string        `json:"content"`
	Abstract  string        `json:"abstract"`
	Author    Author        `json:"author"`
	Status    ArticleStatus `json:"status"`
	Category  string        `json:"category"`
	CreatedAt int64         `json:"createdAt"`
	UpdatedAt int64         `json:"updatedAt"`
}

type Author struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

type ArticleStatus uint8

func (as ArticleStatus) ToUint8() uint8 {
	return uint8(as)
}

// PolishResult AI 润色结果
type PolishResult struct {
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Content  string `json:"content"`
}

const (
	ArticleStatusUnknown     ArticleStatus = iota //未知状态
	ArticleStatusUnpublished                      //未发表
	ArticleStatusPublished                        //已发表
	ArticleStatusPrivate                          //仅自己可见
)
