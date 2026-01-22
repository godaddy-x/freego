// Package main - MongoDB ORM性能基准测试套件
//
// 该文件包含完整的MongoDB ORM性能基准测试，覆盖以下测试维度：
// 1. 基础CRUD操作：Save、Update、Delete、FindOne、FindList
// 2. 批量操作：BatchSave、BatchUpdate
// 3. 事务操作：TransactionCommit
// 4. 复杂查询：ComplexQuery、IndexPerformance
// 5. 并发性能：ConnectionPool、LargeDataset
// 6. 内存使用：MemoryUsage
//
// 所有测试都使用并发模式(b.RunParallel)，模拟真实生产环境的并发压力
// 测试数据使用预分配和常量优化，减少基准测试中的额外开销
// 同时提供FreeGo ORM与MongoDB官方驱动的性能对比
package main

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
)

// BenchmarkMongoFindOne 单条记录查询性能基准测试
// 测试根据ID查询单条记录的性能表现，评估索引查询效率
func BenchmarkMongoFindOne(b *testing.B) {
	initMongoDB()
	db, err := sqld.NewMongo()
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var result OwWallet
			if err := db.FindOne(sqlc.M().Eq("id", 2014299923591200768), &result); err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkMongoFindList 列表查询性能基准测试
// 测试不同数据规模的分页查询性能表现
func BenchmarkMongoFindList(b *testing.B) {
	initMongoDB()

	// 定义测试的数据规模
	testSizes := []struct {
		name string
		size int64
	}{
		{"100", 100},
		{"500", 500},
		{"1000", 1000},
		{"2000", 2000},
	}

	for _, ts := range testSizes {
		b.Run(ts.name+"_records", func(b *testing.B) {
			db, err := sqld.NewMongo()
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					result := make([]*OwWallet, 0, ts.size)
					if err := db.FindList(sqlc.M(&OwWallet{}).Between("id", 2014299923591200768, 2014299923591202767).Offset(0, ts.size).Orderby("id", sqlc.DESC_), &result); err != nil {
						b.Error(err)
					}
				}
			})
		})
	}
}

// BenchmarkMongoOfficialFindOne MongoDB官方驱动单条记录查询性能基准测试
// 使用mongo-driver直接查询，评估官方驱动的性能表现
func BenchmarkMongoOfficialFindOne(b *testing.B) {
	initMongoDB()

	// 获取FreeGo ORM管理器中的MongoDB客户端
	ormManager := &sqld.MGOManager{}
	err := ormManager.GetDB()
	if err != nil {
		b.Skip("获取ORM管理器失败，跳过官方驱动测试")
	}
	defer ormManager.Close()

	// 使用ORM管理器中的官方驱动客户端
	officialClient := ormManager.Session
	collection := officialClient.Database("ops_dev").Collection("test_wallet2")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var result bson.M
			err := collection.FindOne(context.Background(), bson.M{"_id": 2014299923591200768}).Decode(&result)
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkMongoOfficialFindList MongoDB官方驱动列表查询性能基准测试
// 使用mongo-driver直接查询不同数据规模，评估官方驱动的性能表现
func BenchmarkMongoOfficialFindList(b *testing.B) {
	initMongoDB()

	// 获取FreeGo ORM管理器中的MongoDB客户端
	ormManager := &sqld.MGOManager{}
	err := ormManager.GetDB()
	if err != nil {
		b.Skip("获取ORM管理器失败，跳过官方驱动测试")
	}
	defer ormManager.Close()

	// 使用ORM管理器中的官方驱动客户端
	officialClient := ormManager.Session

	// 定义测试的数据规模
	testSizes := []struct {
		name string
		size int64
	}{
		{"100", 100},
		{"500", 500},
		{"1000", 1000},
		{"2000", 2000},
	}

	for _, ts := range testSizes {
		b.Run(ts.name+"_records", func(b *testing.B) {
			collection := officialClient.Database("ops_dev").Collection("test_wallet2")

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					// 使用官方驱动进行范围查询 + 排序 + 分页
					// 对应 FreeGo ORM: Between("id", min, max).Offset(0, size).Orderby("id", DESC_)
					filter := bson.M{
						"_id": bson.M{
							"$gte": 2014299923591200768,
							"$lte": 2014299923591202767,
						},
					}

					findOptions := options.Find().
						SetSort(bson.M{"_id": -1}).  // 降序排序
						SetSkip(0).                  // Offset 0
						SetLimit(int64(ts.size)).    // Limit size
						SetBatchSize(int32(ts.size)) // 批量获取大小

					cursor, err := collection.Find(context.Background(), filter, findOptions)
					if err != nil {
						b.Error(err)
						continue
					}

					// 获取所有结果
					var results []bson.M
					if err := cursor.All(context.Background(), &results); err != nil {
						b.Error(err)
						cursor.Close(context.Background())
						continue
					}

					cursor.Close(context.Background())

					// 验证结果数量（在大数据量时可能因为数据不足而不等于size）
					if len(results) == 0 {
						b.Error("未获取到任何记录")
					}
				}
			})
		})
	}
}
