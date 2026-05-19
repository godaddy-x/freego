# HTTP SDK 授权压测报告（Plan2 + Plan01）

> 阅读指引：本报告覆盖 `BenchmarkHttpSDK_PostByPlan2` 与 `BenchmarkHttpSDK_PostByPlan01` 的稳定性与失败率。  
> 失败定义采用基准代码口径：请求函数返回 error 并被 `b.Logf` 记录时，记一次失败日志。

## 结果总览（速读）

在本机（Windows 10 / i5-13600KF）对 `BenchmarkHttpSDK_PostByPlan2` 进行 **三轮 1 分钟压测** 后：

- 三轮均 **0 失败**（未出现 `Plan2并发请求失败`、`request too frequent`、`429`、`rate limit exceeded`）
- 失败率均为 **0.00%**
- `ns/op` 分别为 **92,116**、**91,738**、**94,001**，处于同一数量级，整体稳定

---

## 1. 测试对象与口径

| 项目 | 说明 |
|------|------|
| 基准方法 | `BenchmarkHttpSDK_PostByPlan2`（`http_test.go`） |
| 业务语义 | ML-KEM + ML-DSA 的 Plan2 匿名登录链路 |
| 压测时长 | `-benchtime=1m` |
| 并发模型 | `b.RunParallel` |
| 指标来源 | `go test -benchmem -v` 标准输出 |
| 失败判定 | 基准内 `PostByPlan2` 返回 error 时，执行 `b.Logf("Plan2并发请求失败 ...")` |

### 1.1 失败计数规则（与你当前判断一致）

若请求触发限流（例如 `request too frequent` 或 `429 ... rate limit exceeded`），会以 error 形式返回到 benchmark 循环，并记入一次 `Plan2并发请求失败` 日志，因此可视为 **失败数 +1**。

---

## 2. 三轮 1 分钟压测结果

| 轮次 | N（总请求数） | `ns/op` | `B/op` | `allocs/op` | 单轮目标时长 | 每秒执行数（N/秒） | 失败日志计数 | 失败率 |
|------|---------------:|--------:|-------:|------------:|-------------:|-------------------:|-------------:|-------:|
| Run 1 | 752,416 | 92,116 | 22,115 | 186 | 60s | 12,540.3/s | 0 | 0.00% |
| Run 2 | 861,972 | 91,738 | 22,370 | 186 | 60s | 14,366.2/s | 0 | 0.00% |
| Run 3 | 856,099 | 94,001 | 22,385 | 186 | 60s | 14,268.3/s | 0 | 0.00% |

### 2.1 失败关键字扫描结果

三轮输出日志均未匹配到以下关键字：

- `Plan2并发请求失败`
- `request too frequent`
- `rate limit exceeded`
- `429`

---

## 3. PostByPlan01 三轮 1 分钟压测结果（JWT + HMAC-SHA512→32B + AES-GCM）

| 轮次 | N（总请求数） | `ns/op` | `B/op` | `allocs/op` | 单轮目标时长 | 每秒执行数（N/秒） | 失败日志计数 | 失败率 |
|------|---------------:|--------:|-------:|------------:|-------------:|-------------------:|-------------:|-------:|
| Run 1 | 2,016,852 | 36,388 | 9,856 | 83 | 60s | 33,614.2/s | 0 | 0.00% |
| Run 2 | 1,923,520 | 37,549 | 9,830 | 83 | 60s | 32,058.7/s | 0 | 0.00% |
| Run 3 | 1,876,887 | 39,015 | 9,799 | 83 | 60s | 31,281.5/s | 0 | 0.00% |

### 3.1 失败关键字扫描结果

三轮输出日志均未匹配到以下关键字：

- `认证请求失败`
- `request too frequent`
- `rate limit exceeded`
- `429`

---

## 4. 命令与复现

```powershell
go test -run ^$ -bench ^BenchmarkHttpSDK_PostByPlan2$ -benchmem -benchtime=1m -count=3 -v
```

如需保留原始输出并后续做失败关键字统计，可使用：

```powershell
go test -run ^$ -bench ^BenchmarkHttpSDK_PostByPlan2$ -benchmem -benchtime=1m -count=3 -v 2>&1 | Tee-Object -FilePath "bench_plan2_1m_3runs.txt"
```

PostByPlan01（JWT + HMAC-SHA512→32B + AES-GCM）：

```powershell
go test -run ^$ -bench ^BenchmarkHttpSDK_PostByPlan01$ -benchmem -benchtime=1m -count=3 -v
go test -run ^$ -bench ^BenchmarkHttpSDK_PostByPlan01$ -benchmem -benchtime=1m -count=3 -v 2>&1 | Tee-Object -FilePath "bench_plan01_1m_3runs.txt"
```

---

## 5. 结论

- 当前服务端参数下，`BenchmarkHttpSDK_PostByPlan2` 在 1 分钟窗口（三轮）内可稳定运行，**未观察到限流导致的失败**。
- `BenchmarkHttpSDK_PostByPlan01` 在同口径（1 分钟三轮）下也为 **0 失败 / 0.00%**，且吞吐显著高于 Plan2 登录链路。
- 以 benchmark 的失败定义看，本批次两组压测均为 **0 失败 / 0.00%**。
- 若后续恢复更严格限流配置，建议继续保留失败关键字统计，以便对比限流策略对稳定性的影响。

---

_报告数据批次：2026-04-23；基准代码：`http_test.go` 的 `BenchmarkHttpSDK_PostByPlan2` 与 `BenchmarkHttpSDK_PostByPlan01`。_
