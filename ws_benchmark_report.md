# WebSocket 并发活跃压测报告（Held Connections）

> 阅读指引：本报告覆盖 `socket_test.go` 中 `TestWebSocketStressConnectionsHeld1Minute` 的实测结果。  
> 口径为「固定时间窗内，成功建连并保持到窗口结束的连接数」。

## 结果总览（速读）

在本机（Windows 10）对本地服务 `localhost:8088` 进行多轮压测后：

- 当前参数下可稳定达到 **~1.6 万活跃连接**
- 推荐默认参数（已落地代码）：`workers=16200`、`duration=3m`、`jitter=7500ms`
- 最新一轮（3 分钟活跃）结果：**成功 16197 / 失败 3（0.02%）**
- 最新一轮（3 分钟发送往返）结果：**7,119,557 请求 / 0 失败 / 39,553 QPS**
- 失败主要来自客户端本机网络栈资源压力（`bind ... insufficient buffer space or queue was full`），不是服务端 pool 先撞满

---

## 1. 测试对象与口径

| 项目 | 说明 |
|------|------|
| 测试方法 | `TestWebSocketStressConnectionsHeld1Minute` |
| 代码位置 | `socket_test.go` |
| 压测语义 | 每个 goroutine：生成 JWT -> `ConnectWebSocket("/ws")` -> 保持到窗口结束 -> 断开 |
| 成功定义 | 在窗口结束时仍保持连接（`okHeld`） |
| 失败定义 | 建连阶段 `ConnectWebSocket` 返回 error（`failHeld`） |
| 失败归类 | 按 `err.Error()` 文本聚合并输出 |
| 服务端启动参数 | `NewPool(wsTestServerMaxConn, wsTestServerConnPerSec, wsTestServerConnBurst, wsTestServerPingSeconds)` |

---

## 2. 关键轮次结果汇总

| 轮次 | workers | window | jitter | 成功保持 | 建连失败 | 失败率 | 备注 |
|------|--------:|-------:|-------:|---------:|---------:|-------:|------|
| Run A | 8000 | 1m | 500ms | 7373 | 627 | 7.8% | 调参前，尖峰失败偏多 |
| Run B | 8000 | 1m | 3000ms | 7999 | 1（≈0） | <0.01% | 调参后，稳定性明显提升（近似 0 失败） |
| Run C | 28000 | 1m | 4500ms | 16247 | 11753 | 42.0% | 超高并发下本机侧资源瓶颈明显 |
| Run D | 16500 | 3m | 5000ms | 16225 | 275 | 1.7% | 接近目标 1.6 万 |
| Run E | 16300 | 3m | 7000ms | 16227 | 73 | 0.4% | 更平滑 |
| Run F | 16200 | 3m | 7500ms | 16199 | 1（≈0） | <0.01% | 当前推荐默认（近似 0 失败） |
| Run G | 16200 | 3m | 7500ms | 16197 | 3 | 0.02% | 最新复测（含 runtime 指标） |

---

## 3. 失败原因统计（典型）

高并发时（例如 Run C / Run D）主要失败原因：

- `WebSocket connection failed: dial tcp [::1]:8088: bind: ... system lacked sufficient buffer space or because a queue was full.`
  - 含义：客户端本机 socket/端口/缓冲资源压力，属于 OS 网络栈限制
- `handshake response read failed (auth sync required): timeout`
  - 含义：握手阶段超时，通常是高压下连带现象
- `connectex: ... actively refused`
  - 含义：瞬时连接被拒（排队或系统瞬时压力）

结论：失败主因是 **客户端本机资源约束**，不是服务端 pool 参数先撞顶。

---

## 4. 运行时指标（最新 3 分钟活跃轮次）

轮次：`workers=16200, duration=3m, jitter=7500ms`

- 成功保持：`16197`
- 失败：`3`
- 运行时指标：
  - `elapsed=3m0.337s`
  - `goroutines=6->6`
  - `GOMAXPROCS=20`
  - `alloc=905.2MB`（本轮累计分配增量）
  - `heap_alloc=301.1MB`
  - `heap_inuse=336.6MB`
  - `sys=1047.3MB`
  - `GC_count_delta=15`
  - `GC_pause_delta_ms=2.30`

---

## 5. Send 往返性能（3 分钟，约 1.6 万连接）

测试方法：`TestWebSocketSendRoundTripPerf`  
参数：`clients=16200`、`window=3m`、`connect_jitter=7500ms`、`route=/ws/user`

- 建连阶段：`connected=16200`，`conn_fail=0 (0.00%)`
- 发送阶段：`total_req=7,119,557`，`ok=7,119,557`，`fail=0 (0.00%)`
- 吞吐：`QPS=39,553.1`
- 延迟：`avg=409.62ms`，`p95=884.19ms`，`max=3817.67ms`
- 运行时指标：
  - `elapsed=3m9.090s`
  - `goroutines=6->6`
  - `GOMAXPROCS=20`
  - `alloc=37,108.1MB`（发送窗口内累计分配，属高频请求正常现象）
  - `heap_alloc=265.4MB`
  - `heap_inuse=343.6MB`
  - `sys=1140.8MB`
  - `GC_count_delta=191`
  - `GC_pause_delta_ms=197.00`

---

## 6. 当前建议参数（目标稳定 1w6）

### 6.1 客户端压测默认（已在代码中）

| 变量 | 默认值 |
|------|--------|
| `WS_HOLD_WORKERS` | `16200` |
| `WS_HOLD_DURATION` | `3m` |
| `WS_HOLD_JITTER_MS` | `7500` |

### 6.2 服务端测试池（已在代码中）

| 参数 | 值 |
|------|---:|
| `wsTestServerMaxConn` | 200000 |
| `wsTestServerConnPerSec` | 100000 |
| `wsTestServerConnBurst` | 250000 |
| `wsTestServerPingSeconds` | 30 |

---

## 7. 复现命令

```powershell
go test -count=1 -v -run TestWebSocketStressConnectionsHeld1Minute -timeout 12m .
```

如需覆盖参数：

```powershell
$env:WS_STRESS_ADDR="127.0.0.1:8088"
$env:WS_HOLD_WORKERS="16200"
$env:WS_HOLD_DURATION="3m"
$env:WS_HOLD_JITTER_MS="7500"
go test -count=1 -v -run TestWebSocketStressConnectionsHeld1Minute -timeout 12m .
```

分档探测上限：

```powershell
go test -count=1 -v -run TestWebSocketStressHeldStepProbe -timeout 45m .
```

3 分钟发送往返性能（约 1.6 万连接）：

```powershell
$env:WS_SEND_CLIENTS="16200"
$env:WS_SEND_DURATION="3m"
$env:WS_SEND_CONNECT_JITTER="7500"
$env:WS_SEND_TIMEOUT_SEC="5"
go test -count=1 -v -run TestWebSocketSendRoundTripPerf -timeout 35m .
```

---

## 8. 结论

- 以当前代码与机器环境，**稳定活跃连接规模约 1.6 万**（3 分钟窗口）。
- 在该连接规模下，`/ws/user` 往返发送压测达到 **~3.96 万 QPS**，发送阶段 **0 失败**。
- 继续把尝试并发推到更高（如 2 万+）时，首先暴露的是本机客户端网络栈瓶颈，而非服务端池参数上限。
- 若要进一步提高有效并发，优先方向是：
  - 压测客户端分机（避免同机 `localhost`）；
  - OS 网络参数调优（端口范围/队列/缓冲）；
  - 保持较高 jitter 以削峰。

---

_报告数据批次：2026-04-24；测试代码：`socket_test.go`。_
