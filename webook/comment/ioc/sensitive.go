package ioc

import (
	"github.com/spf13/viper"

	"github.com/webook/pkg/sensitive"
)

// InitSensitiveFilter 从 yaml sensitive.words 加载敏感词构建 DFA 过滤器。
// 词库为空时返回空过滤器（不拦截任何内容）。后续可扩展为文件/远程加载 + 热更。
func InitSensitiveFilter() sensitive.Filter {
	var words []string
	if err := viper.UnmarshalKey("sensitive.words", &words); err != nil {
		panic(err)
	}
	return sensitive.NewDFAFilter(words)
}
