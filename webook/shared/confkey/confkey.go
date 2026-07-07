// Package confkey 收敛跨服务共享的 infra 配置键常量，yaml 键名改动只需改这一处。
// 只收跨 ≥2 服务、稳定的 infra 段键；服务私有键（llm / data.es / client.grpc.* 等）留各自 ioc。
package confkey

const (
	// ── data ────────────────────────────────────────────────
	DataMySQLDSN = "data.mysql.dsn" // 叶子·GetString
	DataRedis    = "data.redis"     // 段·UnmarshalKey
	DataKafka    = "data.kafka"     // 段·UnmarshalKey

	// ── etcd ─────────────────────────────────────────────────
	Etcd = "etcd" // 段·UnmarshalKey

	// ── server.http ─────────────────────────────────────────
	ServerHTTPAddr    = "server.http.addr"
	ServerHTTPTimeout = "server.http.timeout"

	// ── server.grpc ─────────────────────────────────────────
	ServerGRPCAddr    = "server.grpc.addr"
	ServerGRPCName    = "server.grpc.name"
	ServerGRPCHost    = "server.grpc.host"
	ServerGRPCTTL     = "server.grpc.ttl"
	ServerGRPCWeight  = "server.grpc.weight"
	ServerGRPCTimeout = "server.grpc.timeout"

	// ── 可观测 ───────────────────────────────────────────────
	OTel   = "otel"   // 段·UnmarshalKey
	Logger = "logger" // 段·UnmarshalKey
)
