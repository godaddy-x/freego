# WebSocket 压测报告

> 本文仅保留当前结果，不包含历史对比。  
> 测试代码：`socket_test.go`

## 结果总览

- 活跃连接压测（3 分钟窗口）：**16197 成功 / 3 失败（0.02%）**
- 发送往返压测（3 分钟，`/ws/user`）：**11,266,546 请求 / 0 发送失败 / 62,591.9 QPS**
- 当前结论：在本机环境下，服务端可稳定承载约 **1.6 万活跃连接**，并在该连接规模下达到约 **6.26 万 QPS** 的发送往返能力。

---

## 1) 活跃连接压测（3 分钟）

### 测试口径

- 方法：`TestWebSocketStressConnectionsHeld1Minute`
- 参数：`workers=16200`、`duration=3m`、`jitter=7500ms`
- 成功定义：窗口结束仍保持连接

### 结果

| 指标 | 数值 |
|------|------|
| 成功保持 | 16197 |
| 建连失败 | 3 |
| 失败率 | 0.02% |

### 运行时指标

- `elapsed=3m0.337s`
- `goroutines=6->6`
- `GOMAXPROCS=20`
- `alloc=905.2MB`
- `heap_alloc=301.1MB`
- `heap_inuse=336.6MB`
- `sys=1047.3MB`
- `GC_count_delta=15`
- `GC_pause_delta_ms=2.30`

### 失败原因

- `handshake response read failed (auth sync required): timeout`（3 次）

---

## 2) 发送往返压测（3 分钟，约 1.6 万连接）

### 测试口径

- 方法：`TestWebSocketSendRoundTripPerf`
- 参数：`clients=16200`、`window=3m`、`connect_jitter=7500ms`、`route=/ws/user`
- 统计：请求总数、发送失败率、QPS、延迟分位与 runtime 指标

### 结果

| 指标 | 数值 |
|------|------|
| connected | 16196 |
| conn_fail | 4（0.02%） |
| total_req | 11,266,546 |
| send_fail | 0 |
| QPS | 62,591.9 |
| avg latency | 258.60ms |
| p95 latency | 410.57ms |
| max latency | 1824.89ms |

### 运行时指标

- `elapsed=3m13.336s`
- `goroutines=6->6`
- `GOMAXPROCS=20`
- `alloc=58,682.7MB`
- `heap_alloc=295.1MB`
- `heap_inuse=357.6MB`
- `sys=1232.3MB`
- `GC_count_delta=316`
- `GC_pause_delta_ms=14435.66`

### 失败原因

- 建连失败：`handshake response read failed (auth sync required): timeout`（4 次）
- 发送失败：0

---

## 3) 当前建议参数（目标 1w6）

| 变量 | 推荐值 |
|------|--------|
| `WS_HOLD_WORKERS` | `16200` |
| `WS_HOLD_DURATION` | `3m` |
| `WS_HOLD_JITTER_MS` | `7500` |
| `WS_SEND_CLIENTS` | `16200` |
| `WS_SEND_DURATION` | `3m` |
| `WS_SEND_CONNECT_JITTER` | `7500` |

---

## 4) 复现命令

```powershell
# 3 分钟活跃连接
go test -count=1 -v -run TestWebSocketStressConnectionsHeld1Minute -timeout 12m .

# 3 分钟发送往返
$env:WS_SEND_CLIENTS="16200"
$env:WS_SEND_DURATION="3m"
$env:WS_SEND_CONNECT_JITTER="7500"
$env:WS_SEND_TIMEOUT_SEC="5"
go test -count=1 -v -run TestWebSocketSendRoundTripPerf -timeout 35m .
```

---

_报告批次：2026-04-24_
