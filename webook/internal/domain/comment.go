package domain

// CommentView 评论树视图——core 聚合 comment gRPC（评论树）+ interaction（likeCnt/liked）
// + user（昵称）后的领域对象，接入层再映射为 VO。
type CommentView struct {
	Id        int64
	User      CommentUser
	Content   string
	RootId    int64
	Pid       int64
	ReplyCnt  int64
	LikeCnt   int64
	Liked     bool
	CreatedAt int64
	Children  []CommentView
}

// CommentUser 评论者（core 按 uid 解析昵称填入；comment 服务只存 uid）。
type CommentUser struct {
	Id   int64
	Name string
}
