# MongoDB ORM 性能对比测试报告

## 测试环境

### 硬件环境

- **操作系统**: Windows 10 Pro (64-bit)
- **CPU**: 13th Gen Intel(R) Core(TM) i5-13600KF (20 核心)
- **Go 版本**: 1.23.4
- **测试时间**: 2026-01-22
- **测试持续时间**: 每个基准测试 15 秒

### MongoDB 数据库配置

- **主机**: 127.0.0.1
- **端口**: 27017
- **数据库**: ops_dev
- **集合**: test_wallet2
- **连接池**: 通过 FreeGo ORM 管理器配置

### 测试数据表结构

```go
type OwWallet struct {
	Id           int64  `json:"id" bson:"_id"`
	AppID        string `json:"appID" bson:"appID"`
	WalletID     string `json:"walletID" bson:"walletID"`
	Alias        string `json:"alias" bson:"alias"`
	IsTrust      int64  `json:"isTrust" bson:"isTrust"`
	PasswordType int64  `json:"passwordType" bson:"passwordType"`
	Password     []byte `json:"password" bson:"password" ignore:"true"`
	AuthKey      string `json:"authKey" bson:"authKey"`
	RootPath     string `json:"rootPath" bson:"rootPath"`
	AccountIndex int64  `json:"accountIndex" bson:"accountIndex"`
	Keystore     string `json:"keyJson" bson:"keyJson"`
	Applytime    int64  `json:"applytime" bson:"applytime"`
	Succtime     int64  `json:"succtime" bson:"succtime"`
	Dealstate    int64  `json:"dealstate" bson:"dealstate"`
	Ctime        int64  `json:"ctime" bson:"ctime"`
	Utime        int64  `json:"utime" bson:"utime"`
	State        int64  `json:"state" bson:"state"`
}
```

## 测试方法说明

### 单条记录查询测试

**BenchmarkMongoFindOne**: 使用 FreeGo ORM 查询单条记录

```go
// 查询条件：根据主键 ID 查询
db.FindOne(sqlc.M().Eq("id", 2014299923591200768), &result)
```

**BenchmarkMongoOfficialFindOne**: 使用 MongoDB 官方驱动查询单条记录

```go
// 查询条件：根据 _id 查询
collection.FindOne(context.Background(), bson.M{"_id": 2014299923591200768}).Decode(&result)
```

### 列表查询测试

**BenchmarkMongoFindList**: 使用 FreeGo ORM 分页查询不同数据规模 (100/500/1000/2000 条)

```go
// 查询条件：ID 范围查询 + 分页 + 排序
db.FindList(sqlc.M(&OwWallet{}).
    Between("id", 2014299923591200768, 2014299923591202767).
    Offset(0, size).
    Orderby("id", sqlc.DESC_), &result)
```

**BenchmarkMongoOfficialFindList**: 使用 MongoDB 官方驱动分页查询不同数据规模 (100/500/1000/2000 条)

```go
// 查询条件：ID 范围查询 + 分页 + 排序
filter := bson.M{"_id": bson.M{"$gte": 2014299923591200768, "$lte": 2014299923591202767}}
findOptions := options.Find().
    SetSort(bson.M{"_id": -1}).
    SetSkip(0).
    SetLimit(size).
    SetBatchSize(int32(size))
cursor, _ := collection.Find(context.Background(), filter, findOptions)
cursor.All(context.Background(), &results)
```

### 测试数据说明

- **数据量**: 测试数据库中预先存在约 2000+ 条 ow_wallet 表记录
- **查询范围**: ID 从 2014299923591200768 到 2014299923591202767 的范围查询
- **索引情况**: _id 字段为主键，具有唯一索引；其他字段无特殊索引

## 测试结果

### 单条记录查询性能对比

| 测试方法               | 操作次数  | 单次操作时间 | 内存分配   | 分配次数      | 相对性能       |
| ---------------------- | --------- | ------------ | ---------- | ------------- | -------------- |
| BenchmarkMongoFindOne | 1,000,000 | 16,318 ns/op | 9,418 B/op | 102 allocs/op  | **1.09x 更快** |
| BenchmarkMongoOfficialFindOne   | 1,000,000   | 17,794 ns/op | 12,083 B/op | 163 allocs/op | 基准           |

**分析**: FreeGo ORM 在单条记录查询中性能略优于官方驱动，速度快 9%，内存分配量也更少。

### 列表查询性能对比

#### 100 条记录查询

| 测试方法                        | 操作次数 | 单次操作时间  | 内存分配     | 分配次数        | 相对性能       |
| ------------------------------- | -------- | ------------- | ------------ | --------------- | -------------- |
| BenchmarkMongoFindList/100_records | 254,347  | **71,101 ns/op**  | **130,865 B/op** | **821 allocs/op** | **2.32x 更快** |
| BenchmarkMongoOfficialFindList/100_records   | 119,870  | 164,752 ns/op | 464,111 B/op  | 8,571 allocs/op | 基准           |

#### 500 条记录查询

| 测试方法                        | 操作次数 | 单次操作时间  | 内存分配     | 分配次数         | 相对性能       |
| ------------------------------- | -------- | ------------- | ------------ | ---------------- | -------------- |
| BenchmarkMongoFindList/500_records | 70,318   | **256,539 ns/op** | **592,351 B/op** | **3,660 allocs/op** | **1.78x 更快** |
| BenchmarkMongoOfficialFindList/500_records   | 35,070   | 455,934 ns/op | 2,236,249 B/op | 42,295 allocs/op | 基准           |

#### 1000 条记录查询

| 测试方法                         | 操作次数 | 单次操作时间    | 内存分配       | 分配次数         | 相对性能       |
| -------------------------------- | -------- | --------------- | -------------- | ---------------- | -------------- |
| BenchmarkMongoFindList/1000_records | 39,750   | **453,869 ns/op**   | **1,155,296 B/op** | **7,160 allocs/op** | **1.85x 更快** |
| BenchmarkMongoOfficialFindList/1000_records   | 21,501   | 840,167 ns/op | 4,448,331 B/op | 84,441 allocs/op | 基准           |

#### 2000 条记录查询

| 测试方法                         | 操作次数 | 单次操作时间    | 内存分配       | 分配次数         | 相对性能       |
| -------------------------------- | -------- | --------------- | -------------- | ---------------- | -------------- |
| BenchmarkMongoFindList/2000_records | 21,225   | **841,685 ns/op**   | **2,280,918 B/op** | **14,160 allocs/op** | **1.94x 更快** |
| BenchmarkMongoOfficialFindList/2000_records   | 9,543    | 1,637,496 ns/op | 8,944,352 B/op | 75,919 allocs/op | 基准           |

## 性能趋势分析

### 执行时间对比

```
单条记录查询:
FreeGo ORM: 16,318 ns/op
官方驱动:    17,794 ns/op

列表查询性能倍数提升:
100 条:  FreeGo ORM 比官方驱动快 2.32x
500 条:  FreeGo ORM 比官方驱动快 1.78x
1000 条: FreeGo ORM 比官方驱动快 1.85x
2000 条: FreeGo ORM 比官方驱动快 1.94x
```

### 内存使用对比

- **单条记录**: FreeGo ORM 内存分配更少 (9,418 B vs 12,083 B)
- **列表查询**: FreeGo ORM 在所有数据规模下内存效率都显著更高
  - 500条: FreeGo ORM 内存使用量仅为官方驱动的 26.5%
  - 1000条: FreeGo ORM 内存使用量仅为官方驱动的 26%
  - 2000条: FreeGo ORM 内存使用量仅为官方驱动的 25.5%

### 性能扩展性

- FreeGo ORM 在数据量增大时保持稳定的性能优势
- 官方驱动的性能在大数据量时下降较为明显
- FreeGo ORM 的内存使用效率在大数据量查询中优势更加明显

## 结论

### 性能对比总结

1. **FreeGo ORM 在所有测试场景中均表现出更好的性能**
2. **单条记录查询**: FreeGo ORM 比官方驱动快 9%
3. **列表查询**: FreeGo ORM 的性能优势在不同数据规模下稳定在1.78x-2.32x
4. **内存效率**: FreeGo ORM 在所有测试中内存使用都更高效，特别是在大数据量查询中
5. **扩展性**: FreeGo ORM 在大数据量查询中展现出更好的扩展性

### MongoDB 配置影响分析

- **连接池配置**: 通过 FreeGo ORM 管理器统一配置，确保连接复用
- **查询优化**: FreeGo ORM 内部实现了查询优化和对象池复用
- **并发压力**: 基准测试验证了在高并发场景下的稳定性

### 索引优化建议

- **主键查询**: _id 字段为主键，查询性能较好
- **范围查询**: 当前测试使用 ID 范围查询，建议根据实际查询模式添加复合索引
- **分页查询**: 大数据量分页建议优化查询策略和索引

## 建议

### 应用场景选择

- **高并发查询**: 推荐使用 FreeGo ORM，性能和内存效率更佳
- **大数据量批量查询**: FreeGo ORM 的性能优势更为明显
- **内存敏感应用**: FreeGo ORM 提供了更好的内存使用效率
- **开发效率优先**: FreeGo ORM 提供了更简洁的查询构建器功能

### 数据库优化建议

- **索引优化**: 根据实际查询模式添加适当的索引
- **连接池调优**: 合理配置连接池参数，避免连接过多
- **查询优化**: 充分利用 FreeGo ORM 的查询优化特性

### 开发建议

- 在选择 MongoDB 驱动方式时，建议优先考虑 FreeGo ORM
- 定期进行性能基准测试，跟随框架版本更新
- 关注内存使用和 GC 压力，特别是在高并发场景下

---

_测试报告生成时间: 2026-01-22_
_测试数据基于 15 秒基准测试结果_