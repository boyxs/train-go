package domain

// Comment 评论领域模型。
// 点赞数（likeCnt）/是否已赞（liked）不在此 —— 由 core 聚合 interaction(biz="comment") 后填入对外 VO。
type Comment struct {
	Id      int64  `json:"id"`
	BizId   int64  `json:"bizId"`
	Biz     string `json:"biz"`
	UserId  int64  `json:"userId"` // 评论者 uid；昵称由 core 聚合 user 填入 VO
	Content string `json:"content"`
	RootId  int64  `json:"rootId"` // 根评论 id（一级评论=0）
	Pid     int64  `json:"pid"`    // 父评论 id（一级评论=0）
	// RootComment / ParentComment / Children 由 service 按需组装（repository 单条转换不填）
	RootComment   *Comment  `json:"rootComment,omitempty"`
	ParentComment *Comment  `json:"parentComment,omitempty"`
	Children      []Comment `json:"children,omitempty"`
	ReplyCnt      int64     `json:"replyCnt"` // 一级评论=整楼回复数（对齐展开条数）；楼内回复恒 0
	Deleted       bool      `json:"deleted"`  // 已删除占位（有子回复时保留，内容清空）
	CreatedAt     int64     `json:"createdAt"`
	UpdatedAt     int64     `json:"updatedAt"`
}
