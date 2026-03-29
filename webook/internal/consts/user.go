package consts

import "time"

var (
	AccessKey        = []byte("k6CswdUm75WKcbM68UQUuxVsHSpTCwgK")
	RefreshKey       = []byte("k6CswdUm77WKcbM68UQUuxVsHSpTCwgA")
	AccessHeader     = "x-access-token"
	RefreshHeader    = "x-refresh-token"
	Authorization    = "Authorization"
	UserAgent        = "User-Agent"
	UserKey          = "user_claims"
	Interval         = time.Second * 10
	ExpireTime       = time.Minute * 30
	RefreshTime      = time.Hour * 24 * 7
	RefreshThreshold = ExpireTime - Interval
	Expiration       = time.Minute * 30
	CacheTTL         = time.Minute * 30
)

const (
	UserPattern        = "user:%d"
	UserSsidPattern    = "user:ssid:%s"
	ArticlePattern     = "article:author:%d:%d" // article:author:{uid}:{id}
	InteractionPattern = "interaction:%s:%d" // interaction:{biz}:{bizId}
)

var (
	InteractionTTL  = 24 * time.Hour
	FirstPageTTL    = 3 * time.Minute
)

const (
	ReaderFirstPageKey = "article:reader:first_page"
)

// 2006 年 1 月 2 日 下午 3 点 4 分 5 秒 减 7 小时时区
// “1 2 3 4 5 6 7” 其中 6 是年份
// “7” 代表的就是时区（Timezone） 它的不同写法如下
// 模板占位符  输出示例(以北京时间为例)  含义
// -0700     +0800                  数字偏移量（无冒号）
// Z07:00    +08:00                 数字偏移量（带冒号，最标准）
// MST       CST                    时区缩写名称
// Z         Z或+08:00              如果是 UTC 则显示 Z，否则显示偏移量
// 避坑 .000 vs .999
// .000：如果毫秒是 0，它也会显示（比如 .000）
// .999：如果毫秒是 0，它会直接省略掉不显示
const (
	DateTime        = "2006-01-02 15:04:05"
	DateTimeFull    = "2006-01-02 15:04:05.000 Z07:00"
	DateTimeMilli   = "2006-01-02 15:04:05.000"
	DateOnly        = "2006-01-02"
	YearOnly        = "2006"
	MonthOnly       = "01"
	DayOnly         = "02"
	CompactDateTime = "20060102150405"
)
