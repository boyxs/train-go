package domain

import "github.com/boyxs/train-go/webook/pkg/stringx"

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
	// Tags 标签标识：发布时=作者输入的标签名（待 tag.SyncTags 归一）；搜索结果=已命中标签 slug。两条流不交叉。
	Tags []string `json:"tags,omitempty"`
}

// ArticleWithStats 读者视角文章 + 聚合计数（互动/评论）。
type ArticleWithStats struct {
	Article
	ReadCnt    int64
	LikeCnt    int64
	CommentCnt int64
}

// ArticleBrief 文章轻量投影（feed 关注流回源用）：仅 id + 发布时间，刻意排除正文/标题/摘要。
type ArticleBrief struct {
	Id          int64
	PublishedAt int64 // = published_article.updated_at（编辑重发置顶，微博同语义）
}

// FeedArticleItem 关注流卡片（core BFF 五源聚合结果）：文章 + 作者昵称 + 互动/评论计数 + 标签。
// 复用 Author / Tag 既有类型；PublishedAt 取自 feed 事件 score（非文章 CreatedAt）。
type FeedArticleItem struct {
	ArticleId   int64
	Title       string
	Abstract    string
	Author      Author
	PublishedAt int64
	LikeCnt     int64
	CollectCnt  int64
	CommentCnt  int64
	Tags        []Tag
}

// AbstractMaxRunes 文章摘要缺省从正文截取的字符数上限。
const AbstractMaxRunes = 128

// DisplayAbstract 展示用摘要：显式 Abstract 优先，否则从正文按 rune 截断。
// 是业务规则（非展示格式化），供接入层映射 VO/pb 时调用，避免各处重复截断逻辑。
func (a Article) DisplayAbstract() string {
	if a.Abstract != "" {
		return a.Abstract
	}
	return stringx.Abbreviate(a.Content, AbstractMaxRunes)
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
