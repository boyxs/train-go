package ioc

import "github.com/golang-module/carbon/v2"

// TimezoneReady 标记类型，确保时区初始化在其他 provider 之前完成
type TimezoneReady struct{}

// InitTimezone 全局时区初始化，确保 carbon 按东八区计算
// 存储保持 UTC，查询/展示按 Asia/Shanghai
func InitTimezone() TimezoneReady {
	carbon.SetTimezone("Asia/Shanghai")
	return TimezoneReady{}
}
