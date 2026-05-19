# FreeGo 高性能框架（抗量子攻击）

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.26-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/godaddy-x/freego)](https://goreportcard.com/report/github.com/godaddy-x/freego)

**语言 / Languages:** [简体中文](README.md) · [English](README_EN.md) · [繁體中文](README_TW.md)

> 🚀 **专注于极致性能优化、抗量子攻击与强安全取向的 Go 语言企业级框架**（安全架构与实现流程见 [`README_SECURITY.md`](./README_SECURITY.md)）

FreeGo 面向高并发与强安全场景：**Plan2 登录/密钥交换**采用 **NIST 抗量子标准算法（ML-KEM-1024 / ML-DSA-87）**；登录态 **JWT 签名为 HMAC-SHA256（典型 HS256）**，业务报文字段 **`s` 为 HMAC-SHA256 完整性校验**，载荷可选 **AES-256-GCM**。核心能力聚焦在**服务接入层、数据访问层**及下文工程化配套组件：

- **API 框架**：提供 HTTP / WebSocket / RPCX 接入与过滤器链能力，并提供认证、授权、完整性校验和防重放能力；安全流程与威胁模型见 [`README_SECURITY.md`](./README_SECURITY.md)。
- **ORM 框架**：面向 MySQL / Mongo 的**极低反射热点路径**与预分配友好数据访问能力，兼顾吞吐与内存效率（以 `ormx` 为准）。

同时提供缓存（含本地与 Redis）、分布式锁与限流、AMQP 消息、结构化日志、YAML 配置装载等工程化配套能力，便于在生产环境拼装服务。**对外 HTTPS/TLS 一般由反向代理或网关终止**；框架侧重应用层路由、过滤器与密码学能力（细则见安全文档）。

## 📚 目录

- [🚀 框架特性](#框架特性)
  - [抗量子攻击与高强度密码](#抗量子攻击与高强度密码)
  - [核心组件架构](#核心组件架构)
- [🔧 快速开始](#快速开始)
- [🔐 安全特性](#安全特性)
- [📈 性能对比](#性能对比)
- [🗄️ ORM 特性](#orm-特性)
- [🎯 选择指南](#选择指南)
- [📞 联系与支持](#联系与支持)

## 🚀 框架特性

### 🌐 Server & API 框架

| 特性             | 描述                                                             | 优势                                               |
| ---------------- | ---------------------------------------------------------------- | -------------------------------------------------- |
| 🚀 高性能 HTTP   | 高性能 HTTP 引擎，典型场景吞吐显著高于 net/http（同场景压测）    | 单机 QPS 50,000+（视硬件与压测）                   |
| 🛡️ 抗量子攻击    | Plan2：**ML-KEM-1024** 协商 + **ML-DSA-87** 双向外层签           | NIST 成熟 PQC 方案；登录/密钥交换不依赖 ECC/RSA    |
| 🔐 高强度对称栈  | JWT：**HMAC-SHA256（HS256）**；报文 **`s`：HMAC-SHA256** +（可选）AES-256-GCM | 与常见 JWT/报文 MAC 栈一致，易对接与审计           |
| 🔒 防重放攻击    | 协议 `n`（32B 随机）+ 时间戳 + `s`（HMAC-SHA256）                 | 业务/推送统一 MAC（见安全文档）                    |
| 👥 RBAC 权限控制 | 角色权限管理系统                                                 | 企业级访问控制                                     |
| ⚡ 三级限流      | 网关/方法/用户限流                                               | 防止系统过载                                       |
| 🔧 过滤器链      | 完整的中间件系统                                                 | 支持自定义扩展                                     |

### 🗄️ ORM 数据库框架

| 特性            | 描述                                                                                    | 性能提升                                      |
| --------------- | --------------------------------------------------------------------------------------- | --------------------------------------------- |
| 💾 零内存浪费   | 精确容量预分配                                                                          | 减少 90%+ GC 压力（典型批量场景，以实测为准） |
| ⚡ 极低反射开销 | 相对典型反射型 ORM，热点路径更少依赖反射；元数据/映射等仍可能使用反射（以 `ormx` 为准） | 直接装配 + 类型约束                           |
| 🧠 智能预估     | 递归 OR 条件精确计算                                                                    | 复杂查询性能优化                              |
| 🔄 高并发支持   | 智能连接池 + 原子操作                                                                   | 支持 10,000+ 并发                             |

### 抗量子攻击与高强度密码

FreeGo 采用 **NIST 已标准化、工程可部署** 的抗量子密码组合：

| 阶段                    | 能力             | 算法（实现）                                   | 抗量子含义                                                                            |
| ----------------------- | ---------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------- |
| **匿名登录 / 密钥交换** | Plan2（`p=2`）   | **ML-KEM-1024** 封装 + **ML-DSA-87** 外层签    | NIST PQC 标准族（成熟有效）；**1024 级 KEM** 替代 X25519/ECDH                         |
| **Token 签发与校验**    | JWT 第三段       | **HMAC-SHA256（HS256）**                       | 标准 JWT 签名验证第三段；配合短 `exp`、密钥轮换                                     |
| **会话 Secret**         | `GetTokenSecret` | 与签发密钥绑定的派生材料（实现见源码）         | 供报文 MAC /（可选）AES-GCM，不落库、按需派生                                         |
| **业务完整性**          | 字段 `s`         | **HMAC-SHA256**                                | 统一业务与推送报文完整性；与 `SignBodyMessage` 等路径一致（见安全文档）                 |
| **载荷机密性**          | Plan1/2 加密     | **AES-256-GCM**                                | 256 位对称密钥；量子下 Grover 约等价 **128 位**安全强度，仍高于 128 位纯 AES 默认观感 |

**如何理解「能抗量子破解」：**

- **非对称面（最大短板）**：Plan2 登录与 `/key` 握手已用 **ML-KEM + ML-DSA**，不再使用 Ed25519/X25519/RSA，可应对「大规模量子计算机破解公钥」类威胁模型。
- **对称面（Token 之后）**：JWT 与报文 MAC 为 **HMAC-SHA256**；载荷可选 **AES-256-GCM**。需配合**密钥轮换、短 `exp`、防重放（`n`/`t`/`s`）与 TLS**。
- **边界说明**：Plan0/1 登录态业务帧不重复携带 ML-DSA 外层签，依赖 **JWT（HMAC-SHA256）+ 报文 HMAC-SHA256 +（可选）GCM**；若要求**每一帧**均带抗量子非对称外层签，应使用 Plan2 或 RPCX 的 ML-DSA 路径。传输层 **TLS 1.3** 仍建议由网关/反向代理终止。

实现流程与分层防护见 [`README_SECURITY.md`](./README_SECURITY.md)。

### 核心组件架构

```text
┌─────────────────────────────────────────────────────────────────┐
│                      FreeGo Framework                           │
├─────────────────────────────────────────────────────────────────┤
│              三大服务端（HTTP · WebSocket · RPCX）                │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  HTTP · WebSocket · RPCX                                  │   │
│  │  • 单机 QPS: 50,000+（HTTP 典型场景，视硬件）              │   │
│  │  • 响应延迟: 亚毫秒级（典型 HTTP 路由）                   │   │
│  └──────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                  Filter Chain (过滤器链)                         │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │ 限流过滤器  │ 参数过滤器  │ 会话过滤器  │ 权限过滤器  │ 自定义│   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
├─────────────────────────────────────────────────────────────────┤
│           Security & Crypto（Plan2 抗量子 + JWT/报文 HMAC-SHA256）   │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │ JWT/HS256  │ ML-DSA-87  │ ML-KEM-1024│ AES-256-GCM│HMAC256│   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
├─────────────────────────────────────────────────────────────────┤
│                Business Logic Layer (业务层)                     │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │ 请求上下文  │ 路由管理    │ 中间件管理  │ 错误处理    │ 监控 │   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
├─────────────────────────────────────────────────────────────────┤
│                  ORM Layer (数据访问层)                          │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  FreeGo ORM                                              │   │
│  │  • 零内存浪费 • 极低反射热点 • 精确容量预估                │   │
│  │  • 高并发优化，性能显著提升                                │   │
│  └──────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                 Database Layer (数据库层)                        │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │   MySQL    │  MongoDB   │ Redis 缓存 │ 锁 / 限流  │ 扩展  │   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
└─────────────────────────────────────────────────────────────────┘

【性能指标摘录】（2026-05-19，`http_test.go` 本机 1m 压测；详见 [`http_benchmark_report.md`](./http_benchmark_report.md)）
• `PostByPlan2`（ML-KEM+ML-DSA Plan2 登录，`/login`）: ≈3,393 TPS  • `PostByPlan01`（JWT+HMAC-SHA256 登录态，`/getUser`）: ≈52,320 TPS
• PostByPlan2 ns/op: 358,253  • PostByPlan01 ns/op: 22,932  • 失败率: 0.00%
• MySQL FindOne: 11,169 ns/op（FreeGo） vs 16,471 ns/op（GORM）
• MySQL Update: 180,680 ns/op（FreeGo） vs 358,455 ns/op（GORM）
• 并发连接: 10,000+                         • 失败率: 0.00%（报告样本批次）
```

## 🔧 快速开始

### 📦 安装

```bash
go get github.com/godaddy-x/freego
```

### 🚀 基础示例

```go
package main

import (
    "github.com/godaddy-x/freego/node"
    "github.com/godaddy-x/freego/utils/jwt"
)

func main() {
    httpNode := &node.HttpNode{}

    // 配置 JWT 认证
    httpNode.AddJwtConfig(jwt.JwtConfig{
        TokenKey: "your-256-bit-secret-key",
        TokenExp: jwt.ONE_HOUR,
    })

    // 添加路由
    httpNode.GET("/health", func(ctx *node.Context) error {
        return ctx.Json(map[string]interface{}{"status": "ok"})
    })

    // 启动服务
    httpNode.StartServer(":8080")
}
```

## 🔐 安全特性

### 抗量子攻击与认证体系

- **Plan2 登录授权（抗量子攻击）**: **ML-KEM-1024** 单向封装协商会话密钥 + **ML-DSA-87** 双向外层身份签（`e`），HTTP/WebSocket/RPCX 主链路已移除 Ed25519/X25519
- **JWT Token**: 典型为 **HMAC-SHA256（HS256）** 签名第三段，支持短 `exp` 与 RBAC（细节以 `utils/jwt` 与路由配置为准）
- **会话 Secret**: `GetTokenSecret` 与签发密钥协同派生，不落库、按需派生，供报文 MAC /（可选）加密
- **HMAC-SHA256**: 业务 `s` 与推送统一完整性 MAC（`SignBodyMessage`）
- **AES-256-GCM**: 载荷认证加密（按 Plan / 路由启用）

### 多重认证体系（登录后）

- 登录态 **Plan0/1**：JWT（HMAC-SHA256）+ 报文 **`s`（HMAC-SHA256）** +（可选）AES-GCM
- 匿名 **Plan2**：在上列基础上增加 ML-KEM/ML-DSA 非对称保护（详见安全文档）

### 安全机制

在密码学能力之上，框架在**请求进入业务前**强制执行下列机制（HTTP / WebSocket / RPCX 主链路一致，细节以源码为准）：

| 机制                  | 作用             | 实现要点                                                                                  |
| --------------------- | ---------------- | ----------------------------------------------------------------------------------------- |
| **时间窗**            | 限制过期请求     | 默认 **±5 分钟**（`jwt.FIVE_MINUTES`），校验 `body.t`                                     |
| **协议 Nonce**        | 防重放、唯一请求 | `n` = Base64(**32B** CSPRNG)；`ValidProtocolNonce`；Redis 去重（典型 TTL **10 分钟**）    |
| **签名去重**          | 防同一 `s` 重放  | `validReplayAttack` 对 HMAC 签名缓存拒绝重复                                              |
| **规范串绑定**        | 防跨接口/降级    | MAC/AAD 绑定 **path + d + n + t + p (+ u)**；HTTP 要求 `body.r` 为空                      |
| **Plan 分流**         | 按模式校验       | HTTP 以 **`p`** 区分 Plan01 / Plan2；WS 另结合 **`UsePlan2`** 与 `KeyRoute`/`LoginRoute`  |
| **双层校验（Plan2）** | 身份 + 完整性    | 先验 **ML-DSA** 外层 `e`，再验 **HMAC-SHA256**（`s`）与 GCM                                |
| **推送验签**          | 防伪造广播       | `c=300`；`s` 算法与业务相同，密钥为 **广播密钥**（`PushKeyProvider` / `SetBroadcastKey`） |
| **过滤器链**          | 认证与滥用控制   | 网关/方法/用户 **三级限流**；**SessionFilter**（JWT）；**RoleFilter**（RBAC）             |
| **常量时间比较**      | 降低时序风险     | `CompareBase64Sign` / `subtle.ConstantTimeCompare` 校验 `s`                               |

**字段速查（易混项）：**

- **`n`**：协议随机数（32 字节），≠ GCM 密文内 12 字节 IV，≠ 订阅 ID 用的 UUID。
- **`s`**：对称 MAC（**HMAC-SHA256**）；Plan2 另有非对称 **`e`**（ML-DSA）。
- **Token / Secret**：JWT 为 **HMAC-SHA256** 典型栈；会话 Secret 由 `GetTokenSecret` 等与签发密钥协同派生（实现见源码）。

安全架构与 Plan 流程见 [`README_SECURITY.md`](./README_SECURITY.md)。

## 📈 性能对比

### HTTP API 性能

| 测试场景                          | SDK 方法     | Benchmark                     | 压测口径   | 每秒执行数 | `ns/op`     | `B/op`   | 失败率 |
| --------------------------------- | ------------ | ----------------------------- | ---------- | ---------- | ----------- | -------- | ------ |
| Plan2 匿名登录（ML-DSA + ML-KEM） | `PostByPlan2`  | `BenchmarkHttpSDK_PostByPlan2`  | 1m × 1 run | ≈ 3,393/s  | **358,253** | 175,721  | 0.00%  |
| Plan0/1 登录态请求                | `PostByPlan01` | `BenchmarkHttpSDK_PostByPlan01` | 1m × 1 run | ≈ 52,320/s | **22,932**  | 4,996    | 0.00%  |

> 完整方法、原始输出与失败统计口径见 [`http_benchmark_report.md`](./http_benchmark_report.md)。

### ORM 性能对比（MySQL，独立进程 60s）

| 场景                  | FreeGo（sqld） | GORM          | GORM / FreeGo |
| --------------------- | -------------- | ------------- | ------------- |
| FindOne `ns/op`       | **11,169**     | **16,471**    | **≈ 1.47×**   |
| FindList 100 `ns/op`  | **165,937**    | **253,354**   | **≈ 1.53×**   |
| FindList 500 `ns/op`  | **596,669**    | **825,536**   | **≈ 1.38×**   |
| FindList 1000 `ns/op` | **422,738**    | **1,472,001** | **≈ 3.48×**   |
| FindList 2000 `ns/op` | **751,271**    | **3,189,665** | **≈ 4.25×**   |
| Save `ns/op`          | **301,592**    | **368,179**   | **≈ 1.22×**   |
| Update `ns/op`        | **180,680**    | **358,455**   | **≈ 1.98×**   |

> 上表来自 [`orm_performance_report.md`](./orm_performance_report.md) 的独立进程 60s 批次；各分项失败率均为 0.00%。Mongo 见 [`mongodb_performance_report.md`](./mongodb_performance_report.md)。

## 🗄️ ORM 特性

### 核心优化技术

- **零内存浪费**: 精确容量预分配，避免扩容
- **极低反射开销**: 热点路径侧重直接装配与预分配，减少反射依赖；元数据解析（以 `ormx` 为准）
- **智能预估**: 递归 OR 条件精确容量计算
- **高并发支持**: 原子操作和智能连接池

### 适用场景

- 高频数据库操作
- 大数据量处理
- 内存敏感应用
- 强一致、高性能或数据密集型业务系统

## 🎯 选择指南

### 选择 FreeGo 的理由

| 需求场景             | FreeGo 优势                                           | 适用项目        |
| -------------------- | ----------------------------------------------------- | --------------- |
| 🚀 高性能            | MySQL 多场景 `ns/op` 低于 GORM（约 1.22×~4.25×）      | 高并发 Web 服务 |
| 🔒 强安全 / 支付场景 | Plan2 抗量子登录 + JWT/报文 **HMAC-SHA256**（见安全文档） | 金融、支付系统  |
| 💾 内存优化          | 在 Save / Update 等写路径 `B/op`、`allocs/op` 更低    | 内存敏感应用    |
| 🗄️ 数据库密集        | 极低反射热点 ORM，智能容量预估                        | 数据密集型系统  |

### 快速部署

```dockerfile
# 示例：Go 版本与 go.mod 一致；适用于你的应用仓库（本仓库为库时需自带 main 包再构建）
FROM golang:1.26-alpine
WORKDIR /app
COPY . .
RUN go build -o main .
CMD ["./main"]
```

## 📞 联系与支持

- 📧 **GitHub**: [https://github.com/godaddy-x/freego](https://github.com/godaddy-x/freego)
- 🐛 **Issues**: [报告问题](https://github.com/godaddy-x/freego/issues)
- 📖 **安全文档**: [`README_SECURITY.md`](./README_SECURITY.md)
- 📊 **性能报告**: [`http_benchmark_report.md`](./http_benchmark_report.md) · [`orm_performance_report.md`](./orm_performance_report.md) · [`mongodb_performance_report.md`](./mongodb_performance_report.md)

欢迎通过 [Issues](https://github.com/godaddy-x/freego/issues) 反馈问题与建议。
