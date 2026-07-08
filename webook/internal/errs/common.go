// Package errs 集中定义 webook-core 跨层共享的业务 sentinel，与 internal/consts 平行。
// Error 类型本身在 pkg/errs（项目通用），本包仅定义业务错误变量。
//
// 文件按主题分类：
//
//	common.go   通用：NotFound / KeyNotExist 等技术语义错误（alias 底层库 sentinel）
//	auth.go     用户认证：DuplicateEmail / InvalidPassword 等
//	article.go  文章 + 润色 + 搜索：ArticleNotFound / PolishEmptyTitle 等
//	code.go     验证码：CodeInvalid / CodeSendTooMany 等
//
// 调用方一律 import "github.com/boyxs/train-go/webook/internal/errs"，禁止跨层引用各自的旧 Err 变量。
package errs

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ErrRecordNotFound 通用「记录不存在」，alias gorm.ErrRecordNotFound 以便 errors.Is 直通底层。
var ErrRecordNotFound = gorm.ErrRecordNotFound

// ErrKeyNotExist Redis 键不存在，alias redis.Nil。
var ErrKeyNotExist = redis.Nil
