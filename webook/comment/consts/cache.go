package consts

import "fmt"

// CommentCountKey 评论总数缓存 key：comment:cnt:{biz}:{bizId}
func CommentCountKey(biz string, bizId int64) string {
	return fmt.Sprintf("comment:cnt:%s:%d", biz, bizId)
}
