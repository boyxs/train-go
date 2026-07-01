package ioc

import "github.com/golang-module/carbon/v2"

// TimezoneReady 标记类型，保证时区初始化先于依赖日期计算的 provider（cron）。
type TimezoneReady struct{}

// InitTimezone 全局时区初始化。
func InitTimezone() TimezoneReady {
	carbon.SetTimezone("Asia/Shanghai")
	return TimezoneReady{}
}
