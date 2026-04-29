package ioc

// ConfigChangeCallbacks 远程配置变更时调用，与主仓 internal/ioc/web.go 同构。
// 暂未注册回调（chat 没有 logger 中间件那种需热更的组件），后续按需追加。
var ConfigChangeCallbacks []func()
