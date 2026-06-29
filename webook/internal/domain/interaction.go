package domain

type Interaction struct {
	BizId        int64  `json:"bizId"`
	Biz          string `json:"biz"`
	ReadCount    int64  `json:"readCount"`
	LikeCount    int64  `json:"likeCount"`
	CollectCount int64  `json:"collectCount"`
	Liked        bool   `json:"liked"`
	Collected    bool   `json:"collected"`
}

// 业务类型常量
const (
	BizArticle = "article"
	BizComment = "comment" // 评论点赞复用 interaction，biz="comment"、bizId=commentId
)
