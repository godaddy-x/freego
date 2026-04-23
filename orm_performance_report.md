# MySQL ORM 性能对比测试报告（FreeGo sqld vs GORM）

> 阅读指引：本报告仅保留 **独立进程 + 每项 60s** 的结果。  
> 结论请看 **§3（FindOne）**、**§4（FindList）** 与 **§6（总表）**。

## 1. 测试环境

| 项目 | 说明 |
|------|------|
| 操作系统 | Windows 10（示例：本机实测） |
| CPU | 13th Gen Intel Core i5-13600KF（20 线程，`GOMAXPROCS` 由 automaxprocs 决定） |
| Go | 以本机 `go version` 为准（模块 `go 1.26`） |
| 数据库 | MySQL，配置来自仓库 `resource/mysql.json` |
| 压测方式 | `go test -benchmem`，`b.RunParallel`（与 `mysql_benchmark_test.go` 一致） |

### 1.1 推荐：独立进程 + 每项 60 秒（避免混跑干扰）

**不要**在同一次 `go test` 里用正则匹配多个 `Benchmark*`，否则同一进程内会连续跑多种压测，**连接池状态、prepare 缓存、CPU 缓存**会互相沾染，可比性变差。

推荐用仓库脚本：**每个基准单独启动一次 `go test` 进程**，默认每项计时 **60 秒**（可用参数改时长）：

| 平台 | 命令 |
|------|------|
| Windows（PowerShell） | 仓库根目录：``.\scripts\bench_mysql_compare_60s.ps1``（默认 60s）；``.\scripts\bench_mysql_compare_60s.ps1 -BenchSeconds 120`` |
| Linux / macOS | ``chmod +x scripts/bench_mysql_compare_60s.sh`` 后：``./scripts/bench_mysql_compare_60s.sh``；``BENCH_SECONDS=120 ./scripts/bench_mysql_compare_60s.sh`` |

结果追加写入仓库根目录 **`bench_60s_isolated.log`**，便于粘贴进本报告第 3～4 节。

**顺序**（脚本内已固定）：`MysqlFindOne` → `GormFindOne` → 各 `FindList` 子项（100/500/1000/2000）先 FreeGo 再 GORM。

**说明**：下列结果均为 **独立进程、各 60s**。GORM 侧已 **`PrepareStmt: true`**。完整原始输出见 **`bench_60s_isolated.log`**、**`bench_findlist_60s_last.txt`**（FindList 追加段）。

---

## 2. 对齐条件（公平性）

### 2.1 连接池与 DSN

两侧 **不是** 共用一个 `*sql.DB`，而是 **各开一个池**，但 **数值与 DSN 形态** 与 `ormx/sqld/mysql_manager.go` 中逻辑对齐：

| 参数 | 来源 | FreeGo（sqld） | GORM |
|------|------|----------------|--------|
| 配置文件 | `resource/mysql.json` | 经 `InitConfigAndCache` 读入 | 同文件再次读入 |
| `SetMaxOpenConns` | 未配置时用默认 | 100 | 100 |
| `SetMaxIdleConns` | 未配置时用默认 | 10 | 10 |
| `SetConnMaxLifetime` | 秒 → `time.Duration` | 3600s | 3600s |
| `SetConnMaxIdleTime` | 秒 → `time.Duration` | 300s | 300s |
| DSN 模板 | 与 `mysql_manager` 一致 | `charset`、`loc`、`timeout`×3 | 同左 |

实现位置：

- FreeGo：`mysql_benchmark_test.go` → `ensureMysqlBenchmarkOnce()` → `initMysqlDB()`
- GORM：`scripts/bench_gorm_temp_60s.ps1` / `scripts/bench_gorm_temp_60s.sh` 动态生成临时 benchmark（仓库内不引入 gorm 代码）

### 2.2 查询语义对齐

| 场景 | FreeGo（基准名） | GORM（基准名） |
|------|------------------|----------------|
| 单条 | `BenchmarkMysqlFindOne`：`FindOne(sqlc.M().Eq("id", benchCompareFindOneID), …)` | `BenchmarkGormFindOne`：`Where("id = ?").First(&w)` |
| 列表 | `BenchmarkMysqlFindList`：`Between(min,max)` + `Orderby(id,DESC)` + `Offset(0,n)` | `BenchmarkGormFindList`：`Where id BETWEEN ? AND ?` + `Order("id DESC")` + `Limit(n)` |

**常量**（FreeGo benchmark 与临时 GORM benchmark 共同使用）：

| 常量 | 值 |
|------|-----|
| `benchCompareFindOneID` | `1988433892066983936` |
| `benchCompareListMin` | `1988433892066983936` |
| `benchCompareListMax` | `1990301977933774874` |

### 2.3 实现差异（表格外说明）

| 维度 | FreeGo | GORM（压测代码） |
|------|--------|------------------|
| 预编译语句 | 非事务路径：`defaultPrepareManager` + 本地 **TTLCache** 缓存 `*sql.Stmt` | `gorm.Config{ PrepareStmt: true }`：GORM 对 SQL **预编译并缓存**，减少重复解析（与 FreeGo「走 prepare」方向对齐；**缓存实现、生命周期与 TTL 仍不同**） |
| 其它 | （默认） | `SkipDefaultTransaction: true`：关闭 GORM 隐式事务包裹读（读路径轻微降耗） |
| 模型 | `OwWallet` + `sqlc` 条件 | `gormOwWallet` + `gorm` 标签列名与 `ow_wallet` 一致 |

**说明**：GORM 还可选用 `Session(&gorm.Session{QueryFields: true})` 等进一步约束 SELECT 列；当前压测为全行扫描，与 FreeGo `FindOne`/`FindList` 默认行为一致，故未开。

---

## 3. FindOne（单条主键）

**基准**：`BenchmarkMysqlFindOne` vs `BenchmarkGormFindOne`（`benchCompareFindOneID`，FreeGo 在 `mysql_benchmark_test.go`，GORM 由脚本临时生成）

### 独立进程各 60 秒（2026-04-23 本机实测）

两次 benchmark **独立进程执行**（FreeGo 一次、GORM 一次），每次仅跑一个 `-bench`，`-benchtime=60s`，`-count=1`。GORM 已 `PrepareStmt: true`。

| 指标 | FreeGo（sqld） | GORM | GORM / FreeGo |
|------|----------------|------|---------------|
| `ns/op` | **14,143** | **16,714** | **≈ 1.18×**（GORM 更慢） |
| `B/op` | **5,685** | **8,592** | **≈ 1.51×** |
| `allocs/op` | **88** | **129** | **≈ 1.47×** |
| **60s 总执行次数（N）** | **5,163,781** | **4,315,933** | — |
| **平均每秒执行次数（N/60）** | **≈ 86,063 /s** | **≈ 71,932 /s** | **≈ 0.84×** |

**命令（PowerShell）**：

```powershell
go test '-run=^$' -bench='^BenchmarkMysqlFindOne$' -benchmem -benchtime=60s -count=1 .
.\scripts\bench_gorm_temp_60s.ps1 -Scenario findone -BenchSeconds 60
```

**结论（本机、本组 60s）**：FindOne 场景下 FreeGo 快于 GORM，延迟差距约 **18%**；GORM 的分配量约高 **50%**。

---

## 4. FindList（范围 + 倒序 + 限制条数）

**基准**：`BenchmarkMysqlFindList/<N>_records` vs `BenchmarkGormFindList/<N>_records`（`benchCompareListMin`～`benchCompareListMax`）

### 独立进程各 60 秒（2026-04-23 本机实测）

每个规模执行 **2 次独立进程 benchmark**（先 FreeGo 子项 60s，再 GORM 同子项 60s）；四档规模共 **8** 次进程；`-benchtime=60s`，`-count=1`。GORM `PrepareStmt: true`。

| 规模 | FreeGo `ns/op` | GORM `ns/op` | GORM / FreeGo | FreeGo `B/op` | GORM `B/op` | FreeGo `allocs/op` | GORM `allocs/op` | FreeGo 60s 总次数（N） | GORM 60s 总次数（N） | FreeGo 平均每秒次数 | GORM 平均每秒次数 |
|------|----------------|--------------|---------------|---------------|-------------|--------------------|------------------|------------------------|----------------------|--------------------|------------------|
| 100 | **165,937** | **253,354** | **≈1.53×** | 164,130 | 176,786 | 3,573 | 3,908 | **416,199** | **262,132** | **≈ 6,937 /s** | **≈ 4,369 /s** |
| 500 | **596,669** | **825,536** | **≈1.38×** | 798,299 | 792,622 | 17,577 | 19,126 | **123,879** | **86,836** | **≈ 2,065 /s** | **≈ 1,447 /s** |
| 1000 | **422,738** | **1,472,001** | **≈3.48×** | 1,561,076 | 1,796,209 | 34,981 | 38,076 | **172,712** | **48,658** | **≈ 2,879 /s** | **≈ 811 /s** |
| 2000 | **751,271** | **3,189,665** | **≈4.25×** | 3,000,998 | 3,197,950 | 69,856 | 75,990 | **94,597** | **22,664** | **≈ 1,577 /s** | **≈ 378 /s** |

**命令示例（PowerShell，与脚本顺序一致）**：

```powershell
go test '-run=^$' -bench='BenchmarkMysqlFindList/100_records'  -benchmem -benchtime=60s -count=1 .
.\scripts\bench_gorm_temp_60s.ps1 -Scenario list100 -BenchSeconds 60
# …500 / 1000 / 2000 同理替换子项名
```

**结论（本机、本组 60s）**：100/500 条场景下 GORM 慢约 **38%～53%**；1000/2000 条差距扩大到约 **3.5×～4.3×**。

---

## 5. 复现命令速查

**首选（独立、默认 60s/项）**：见 **§1.1** 脚本 `scripts/bench_mysql_compare_60s.ps1` / `scripts/bench_mysql_compare_60s.sh`。

手动单次（PowerShell 注意给 `-run=^$` 加引号）：

```powershell
# 仅 FreeGo FindOne，60 秒，单独进程
go test '-run=^$' -bench='^BenchmarkMysqlFindOne$' -benchmem -benchtime=60s -count=1 .

# 仅 GORM FindOne，60 秒，单独进程
.\scripts\bench_gorm_temp_60s.ps1 -Scenario findone -BenchSeconds 60

# 仅某一档 FindList（示例：100 条），单独进程
go test '-run=^$' -bench='BenchmarkMysqlFindList/100_records' -benchmem -benchtime=60s -count=1 .
.\scripts\bench_gorm_temp_60s.ps1 -Scenario list100 -BenchSeconds 60
```

```bash
# Linux / bash 示例
go test -run='^$' -bench='^BenchmarkMysqlFindOne$' -benchmem -benchtime=60s -count=1 .
```

**不推荐**（易混跑）：同一条命令里用 `|` 匹配多个 `Benchmark`，或一次跑 FreeGo+GORM 多个子项——除非仅做粗测。

```text
# 统计更稳：对「独立多次」输出的 log 用 benchstat（需安装 golang.org/x/perf/cmd/benchstat）
benchstat run1.txt run2.txt run3.txt
```

---

## 6. 总表（速览）

| 场景 | FreeGo 优势（本报告数据） | 备注 |
|------|---------------------------|------|
| FindOne | 延迟约 **1.18×**（GORM 更慢） | 独立 60s、GORM `PrepareStmt: true` |
| FindList 100 | GORM 慢 **≈1.53×** | 独立 60s |
| FindList 500 | GORM 慢 **≈1.38×** | 独立 60s |
| FindList 1000 | GORM 慢 **≈3.48×** | 独立 60s |
| FindList 2000 | GORM 慢 **≈4.25×** | 独立 60s |

---

_报告数据批次：2026-04-23；基准实现：`mysql_benchmark_test.go`（FreeGo）+ `scripts/bench_gorm_temp_60s.ps1`（GORM 临时工程）。_
