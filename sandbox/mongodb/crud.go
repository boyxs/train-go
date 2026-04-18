package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Product 对应 gorm 示例中的 Product 结构体
type Product struct {
	Id    int64  `bson:"id,omitempty"`
	Code  string `bson:"code,omitempty"`
	Price uint   `bson:"price,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 连接 MongoDB（对应 gorm.Open）
	monitor := &event.CommandMonitor{
		Started: func(ctx context.Context, evt *event.CommandStartedEvent) {
			fmt.Println(evt.Command)
		},
	}
	opts := options.Client().
		ApplyURI("mongodb://root:13520@localhost:27018/").
		SetMonitor(monitor)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}
	defer client.Disconnect(ctx)

	// 获取集合（对应 db.AutoMigrate — MongoDB 无需显式建表）
	col := client.Database("webook").Collection("products")

	// 先清空旧数据
	col.Drop(ctx)

	// ===== Create =====
	// 对应 db.Create(&Product{Code: "D42", Price: 100})
	insertRes, err := col.InsertOne(ctx, Product{
		Id:    1,
		Code:  "D42",
		Price: 100,
	})
	if err != nil {
		log.Fatal("create failed:", err)
	}
	fmt.Println("插入成功, ID:", insertRes.InsertedID)

	// 批量插入
	_, err = col.InsertMany(ctx, []interface{}{
		Product{Id: 2, Code: "A10", Price: 50},
		Product{Id: 3, Code: "B20", Price: 200},
	})
	if err != nil {
		log.Fatal("batch create failed:", err)
	}

	// ===== Read =====
	// 对应 db.First(&product, 1) — 按主键查找
	var product Product
	err = col.FindOne(ctx, bson.M{"id": 1}).Decode(&product)
	if err != nil {
		log.Fatal("read by id failed:", err)
	}
	fmt.Printf("按 ID 查找: %+v\n", product)

	// 对应 db.First(&product, "code = ?", "D42") — 按条件查找
	err = col.FindOne(ctx, bson.M{"code": "D42"}).Decode(&product)
	if err != nil {
		log.Fatal("read by code failed:", err)
	}
	fmt.Printf("按 Code 查找: %+v\n", product)

	// 查找多条
	cursor, err := col.Find(ctx, bson.M{"price": bson.M{"$gte": 100}})
	if err != nil {
		log.Fatal("find many failed:", err)
	}
	var products []Product
	err = cursor.All(ctx, &products)
	if err != nil {
		log.Fatal("decode cursor failed:", err)
	}
	fmt.Printf("Price >= 100 的产品: %+v\n", products)

	// ===== Update =====
	// 对应 db.Model(&product).Update("Price", 200) — 更新单个字段
	updateRes, err := col.UpdateOne(ctx,
		bson.M{"id": 1},
		bson.M{"$set": bson.M{"price": 200}},
	)
	if err != nil {
		log.Fatal("update one field failed:", err)
	}
	fmt.Println("更新单字段, 影响行数:", updateRes.ModifiedCount)

	// 对应 db.Model(&product).Updates(Product{Price: 200, Code: "F42"}) — 更新多个字段
	updateRes, err = col.UpdateOne(ctx,
		bson.M{"id": 1},
		bson.M{"$set": bson.M{"price": 300, "code": "F42"}},
	)
	if err != nil {
		log.Fatal("update multiple fields failed:", err)
	}
	fmt.Println("更新多字段, 影响行数:", updateRes.ModifiedCount)

	// UpdateMany — 批量更新
	updateManyRes, err := col.UpdateMany(ctx,
		bson.M{"price": bson.M{"$lt": 100}},
		bson.M{"$set": bson.M{"price": 99}},
	)
	if err != nil {
		log.Fatal("update many failed:", err)
	}
	fmt.Println("批量更新, 影响行数:", updateManyRes.ModifiedCount)

	// ===== Delete =====
	// 对应 db.Delete(&product, 1)
	delRes, err := col.DeleteOne(ctx, bson.M{"id": 1})
	if err != nil {
		log.Fatal("delete failed:", err)
	}
	fmt.Println("删除, 影响行数:", delRes.DeletedCount)

	// DeleteMany — 批量删除
	delManyRes, err := col.DeleteMany(ctx, bson.M{"price": bson.M{"$lte": 99}})
	if err != nil {
		log.Fatal("delete many failed:", err)
	}
	fmt.Println("批量删除, 影响行数:", delManyRes.DeletedCount)
}
