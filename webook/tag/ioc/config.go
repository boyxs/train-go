package ioc

// ConfigChangeCallbacks 远程配置（etcd）变更时调用，main.go 统一遍历执行。
// tag 当前无热更挂点（纯 gRPC server，无 web 中间件），占位保留。
var ConfigChangeCallbacks []func()
