package redislock

import "github.com/redis/go-redis/v9"

// NewClient 单机 / 集群共用工厂：传入 *redis.Client（单机）或 *redis.ClusterClient（集群），
// 二者均实现 redis.UniversalClient。切换拓扑只换注入类型，库代码零改动；集群靠 key
// hash tag 同槽化解 CROSSSLOT（§2.2 / §3.7）。多主 quorum 走独立的 NewQuorumClient。
func NewClient(uc redis.UniversalClient) Client {
	return &RedisClient{cmd: uc}
}
