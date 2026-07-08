package ioc

import (
	"context"
	"time"

	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	mongoopts "go.mongodb.org/mongo-driver/mongo/options"

	"github.com/boyxs/train-go/webook/pkg/logger"
)

// InitMongoClient 进程级共享 mongo 客户端；启动期 Ping 一次，不可达返 nil（让 mongo task 启动期报错而非运行期）。
func InitMongoClient(l logger.LoggerX) *mongo.Client {
	uri := viper.GetString("migrator.mongo.uri")
	if uri == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := mongoopts.Client().ApplyURI(uri).SetServerSelectionTimeout(3 * time.Second)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		l.Warn("mongo connect failed", logger.Error(err))
		return nil
	}
	if err := client.Ping(ctx, nil); err != nil {
		l.Warn("mongo ping failed", logger.Error(err))
		_ = client.Disconnect(context.Background())
		return nil
	}
	return client
}
