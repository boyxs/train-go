package ioc

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"github.com/webook/internal/migratorsdk"
	"github.com/webook/pkg/logger"
)

// InitMigratorSDKSwitchReader 按 yaml flag 切 NoOp / Redis 实现。
//
// yaml：
//
//	migrator:
//	  sdk:
//	    enabled: true   → RedisSwitchReader（读 migrator:stage:{name} + migrator:gray:{name}）
//	    enabled: false  → NoOpSwitchReader（默认；零 Redis 调用、零路径开销）
//
// 业务方不感知具体实现 — 注入 SwitchReader 接口即可。
func InitMigratorSDKSwitchReader(cmd redis.Cmdable, l logger.LoggerX) migratorsdk.SwitchReader {
	if viper.GetBool("migrator.sdk.enabled") {
		return migratorsdk.NewRedisSwitchReader(cmd, l)
	}
	return migratorsdk.NewNoOpSwitchReader()
}

// InitMigratorSDKDualWriter 同 InitMigratorSDKSwitchReader 的开关逻辑。
//
// 启用时 FailureRecorder 默认 NoOpFailureRecorder（仅 log warn）；
// 如需落 dead_letter 表，业务方可在调用处用 NewRedisDualWriter 显式注入自定义 Recorder。
func InitMigratorSDKDualWriter(cmd redis.Cmdable, l logger.LoggerX) migratorsdk.DualWriter {
	if viper.GetBool("migrator.sdk.enabled") {
		return migratorsdk.NewRedisDualWriter(cmd, nil, l)
	}
	return migratorsdk.NewNoOpDualWriter()
}

// InitMigratorSDKTaskName 读 yaml `migrator.sdk.task_name`，返 migratorsdk.TaskName named type。
func InitMigratorSDKTaskName() migratorsdk.TaskName {
	return migratorsdk.TaskName(viper.GetString("migrator.sdk.task_name"))
}
