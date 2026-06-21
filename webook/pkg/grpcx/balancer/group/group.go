// Package group 定义负载均衡"节点分组"标签在 resolver.Address 上的携带约定:resolver 侧用
// Set 写入,balancer 侧用 Of 读取。仿 weight 包,让 resolver 与 balancer 互不依赖。
//
// 标签同样存 Attributes(不用已 deprecated 的 BalancerAttributes)。见 weight 包。
package group

import "google.golang.org/grpc/resolver"

// key 是把分组标签塞进地址属性的私有键(空结构体,外部无法伪造)。
type key struct{}

// Set 给地址打分组标签,返回带标签副本。
func Set(addr resolver.Address, g string) resolver.Address {
	addr.Attributes = addr.Attributes.WithValue(key{}, g)
	return addr
}

// Of 取地址分组标签;没打返回空串(归入默认组)。
func Of(addr resolver.Address) string {
	if g, ok := addr.Attributes.Value(key{}).(string); ok {
		return g
	}
	return ""
}
