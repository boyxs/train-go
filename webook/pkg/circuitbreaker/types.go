package circuitbreaker

// CircuitBreaker 熔断器接口
//
//go:generate mockgen -source=./types.go -package=cbmocks -destination=mocks/circuitbreaker.mock.go CircuitBreaker
type CircuitBreaker interface {
	// Allow 是否允许请求通过。熔断打开时返回 false
	Allow() bool
	// Success 标记一次成功，重置失败计数
	Success()
	// Fail 标记一次失败，累计触发熔断
	Fail()
}
