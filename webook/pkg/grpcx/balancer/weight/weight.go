// Package weight 定义负载均衡权重在 resolver.Address 上的携带约定:resolver 侧用 Set 写入,
// balancer 侧用 Of 读取。抽成中立包,让 resolver 与具体 balancer 互不依赖。
//
// 权重存 Attributes:base 系均衡器只读 Address(不读 Endpoint),从 SubConnInfo.Address 取得到。
// 不用 BalancerAttributes(已 deprecated),也用不上 Endpoint.Attributes(base 读不到 Endpoint)。
package weight

import "google.golang.org/grpc/resolver"

// key 是把权重塞进地址属性的私有键(空结构体,外部无法伪造)。
type key struct{}

// Set 给地址打权重,返回带权副本。
func Set(addr resolver.Address, weight int) resolver.Address {
	addr.Attributes = addr.Attributes.WithValue(key{}, weight)
	return addr
}

// Of 取地址权重;没打或非正数按 1 计。
func Of(addr resolver.Address) int {
	if w, ok := addr.Attributes.Value(key{}).(int); ok && w > 0 {
		return w
	}
	return 1
}
