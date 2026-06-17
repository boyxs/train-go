package registry

import "context"

// Registry 是服务注册器的通用契约。通用名留给接口,
// 具体实现按后端技术加前缀(如 EtcdRegistry)。
type Registry interface {
	// Register 注册一个实例;重复注册同一 (Name, Addr) 视为覆盖更新。
	Register(ctx context.Context, ins ServiceInstance) error
	// Deregister 注销一个实例,并停止其续租。
	Deregister(ctx context.Context, ins ServiceInstance) error
	// Close 注销本注册器写入的所有实例并释放资源;不关闭外部传入的 client。
	Close() error
}

// ServiceInstance 描述一个服务实例的注册信息。
// 一个 Registry 可注册多个不同 Name 的实例(多服务)。
type ServiceInstance struct {
	Name     string            // 服务名,对应 etcd key 前缀 "service/<Name>/"
	Addr     string            // host:port,客户端实际拨号地址
	Weight   uint32            // 权重,供加权负载均衡(0 表示用默认)
	Metadata map[string]string // 扩展元数据:版本 / 分组 / 灰度标记等
}
