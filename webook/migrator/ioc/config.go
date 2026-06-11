package ioc

// ConfigChangeCallbacks 远程配置（etcd）变更时调用，main.go 统一遍历执行。
// 当前仅 access log 中间件可热更，初始化时由 web.go 追加回调。三服务同构（internal/ioc、chat/ioc 各持一份）。
var ConfigChangeCallbacks []func()
