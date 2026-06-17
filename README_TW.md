# FreeGo 高效能框架（抗量子攻擊）

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.26-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/godaddy-x/freego)](https://goreportcard.com/report/github.com/godaddy-x/freego)

**語言 / Languages:** [简体中文](README.md) · [English](README_EN.md) · [繁體中文](README_TW.md)

> 🚀 **專注於極致效能優化、抗量子攻擊與強安全取向的 Go 語言企業級框架**（安全架構與實作流程見 [`README_SECURITY_TW.md`](./README_SECURITY_TW.md)）

FreeGo 面向高併發與強安全場景：**Plan2 登入/金鑰交換**採用 **NIST 抗量子標準演算法（ML-KEM-1024 / ML-DSA-87）**；登入態 **JWT 簽名為 HMAC-SHA256（典型 HS256）**，業務報文欄位 **`s` 為 HMAC-SHA256 完整性校驗**，載荷可選 **AES-256-GCM**。核心能力聚焦在**服務接入層、資料存取層**及下文工程化配套元件：

- **API 框架**：提供 HTTP / WebSocket / RPCX 接入與過濾器鏈能力，並提供認證、授權、完整性校驗和防重放能力；安全流程與威脅模型見 [`README_SECURITY_TW.md`](./README_SECURITY_TW.md)。
- **ORM 框架**：面向 MySQL / Mongo 的**極低反射熱點路徑**與預分配友好資料存取能力，兼顧吞吐與記憶體效率（以 `ormx` 為準）。

同時提供快取（含本地與 Redis）、分散式鎖與限流、AMQP 訊息、結構化日誌、YAML 配置裝載等工程化配套能力，便於在生產環境拼裝服務。**對外 HTTPS/TLS 一般由反向代理或閘道終止**；框架側重應用層路由、過濾器與密碼學能力（細則見安全文件）。

## 📚 目錄

- [🚀 框架特性](#-框架特性)
  - [抗量子攻擊與高強度密碼](#抗量子攻擊與高強度密碼)
  - [核心元件架構](#核心元件架構)
- [🔧 快速開始](#-快速開始)
- [🔐 安全特性](#-安全特性)
- [📈 效能對比](#-效能對比)
- [🗄️ ORM 特性](#-orm-特性)
- [🎯 選擇指南](#-選擇指南)
- [📞 聯絡與支援](#-聯絡與支援)

## 🚀 框架特性

### 🌐 Server & API 框架

| 特性 | 描述 | 優勢 |
| ---------------- | ----------------------------------------------- | -------------------------------- |
| 🚀 高效能 HTTP | 高效能 HTTP 引擎，典型場景吞吐顯著高於 net/http（同場景壓測） | 單機 QPS 50,000+（視硬體與壓測） |
| 🛡️ 抗量子攻擊 | Plan2：**ML-KEM-1024** 協商 + **ML-DSA-87** 雙向外層簽 | NIST 成熟 PQC 方案；登入/金鑰交換不依賴 ECC/RSA |
| 🔐 高強度對稱棧 | JWT：**HMAC-SHA256（HS256）**；報文 **`s`：HMAC-SHA256** +（可選）AES-256-GCM | 與常見 JWT/報文 MAC 棧一致，易對接與稽核 |
| 🔒 防重放攻擊 | 協議 `n`（32B 隨機）+ 時間戳 + `s`（HMAC-SHA256） | 業務/推送統一 MAC（見安全文件） |
| 👥 RBAC 權限控制 | 角色權限管理系統 | 企業級存取控制 |
| ⚡ 三級限流 | 閘道/方法/使用者限流 | 防止系統過載 |
| 🔧 過濾器鏈 | 完整的中介軟體系統 | 支援自訂擴充 |

### 🗄️ ORM 資料庫框架

| 特性 | 描述 | 效能提升 |
| ------------- | --------------------- | ----------------- |
| 💾 零記憶體浪費 | 精確容量預分配 | 減少 90%+ GC 壓力（典型批次場景，以實測為準） |
| ⚡ 極低反射開銷 | 相對典型反射型 ORM，熱點路徑更少依賴反射；中繼資料/映射等仍可能使用反射（以 `ormx` 為準） | 直接裝配 + 型別約束 |
| 🧠 智慧預估 | 遞迴 OR 條件精確計算 | 複雜查詢效能優化 |
| 🔄 高併發支援 | 智慧連線池 + 原子操作 | 支援 10,000+ 併發 |

### 抗量子攻擊與高強度密碼

FreeGo 採用 **NIST 已標準化、工程可部署** 的抗量子密碼組合：

| 階段 | 能力 | 演算法（實作） | 抗量子含義 |
|------|------|--------------|------------|
| **匿名登入 / 金鑰交換** | Plan2（`p=2`） | **ML-KEM-1024** 封裝 + **ML-DSA-87** 外層簽 | NIST PQC 標準族（成熟有效）；**1024 級 KEM** 替代 X25519/ECDH |
| **Token 簽發與校驗** | JWT 第三段 | **HMAC-SHA256（HS256）** | 標準 JWT 簽名驗證第三段；短 `exp` 與金鑰輪換 |
| **會話 Secret** | `GetTokenSecret` | 與簽發金鑰綁定的派生材料（實作見原始碼） | 供報文 MAC /（可選）AES-GCM，不落庫、按需派生 |
| **業務完整性** | 欄位 `s` | **HMAC-SHA256** | 統一業務與推送報文完整性；與 `SignBodyMessage` 等路徑一致（見安全文件） |
| **載荷機密性** | Plan1/2 加密 | **AES-256-GCM** | 256 位元對稱金鑰；量子下 Grover 約等價 **128 位元**安全強度 |

**如何理解「能抗量子破解」：**

- **非對稱面（最大短板）**：Plan2 登入與 `/key` 握手已用 **ML-KEM + ML-DSA**，不再使用 Ed25519/X25519/RSA，可應對「大規模量子電腦破解公鑰」類威脅模型。
- **對稱面（Token 之後）**：JWT 與報文 MAC 為 **HMAC-SHA256**；載荷可選 **AES-256-GCM**。需配合**金鑰輪換、短 `exp`、防重放（`n`/`t`/`s`）與 TLS**。
- **邊界說明**：Plan0/1 登入態業務幀不重複攜帶 ML-DSA 外層簽，依賴 **JWT（HMAC-SHA256）+ 報文 HMAC-SHA256 +（可選）GCM**；若要求**每一幀**均帶抗量子非對稱外層簽，應使用 Plan2 或 RPCX 的 ML-DSA 路徑。傳輸層 **TLS 1.3** 仍建議由閘道/反向代理終止。

實作流程與分層防護見 [`README_SECURITY_TW.md`](./README_SECURITY_TW.md)。

### 核心元件架構

```text
┌─────────────────────────────────────────────────────────────────┐
│                      FreeGo Framework                           │
├─────────────────────────────────────────────────────────────────┤
│              三大服務端（HTTP · WebSocket · RPCX）                │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  HTTP · WebSocket · RPCX                                  │   │
│  │  • 單機 QPS: 50,000+（HTTP 典型場景，視硬體）              │   │
│  │  • 響應延遲: 亞毫秒級（典型 HTTP 路由）                   │   │
│  └──────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                  Filter Chain (過濾器鏈)                         │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │ 限流過濾器  │ 參數過濾器  │ 會話過濾器  │ 權限過濾器  │ 自訂│   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
├─────────────────────────────────────────────────────────────────┤
│           Security & Crypto（Plan2 抗量子 + JWT/報文 HMAC-SHA256）   │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │ JWT/HS256  │ ML-DSA-87  │ ML-KEM-1024│ AES-256-GCM│HMAC256│   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
├─────────────────────────────────────────────────────────────────┤
│                Business Logic Layer (業務層)                     │
├─────────────────────────────────────────────────────────────────┤
│                  ORM Layer (資料存取層)                          │
├─────────────────────────────────────────────────────────────────┤
│                 Database Layer (MySQL · Mongo · Redis)           │
└─────────────────────────────────────────────────────────────────┘

【效能指標摘錄】（2026-05-19，`http_test.go` 本機 1m 壓測；詳見 [`http_benchmark_report.md`](./http_benchmark_report.md)）
• `PostByPlan2`（ML-KEM+ML-DSA Plan2 登入，`/login`）: ≈3,393 TPS  • `PostByPlan01`（JWT+HMAC-SHA256 登入態，`/getUser`）: ≈52,320 TPS
• PostByPlan2 ns/op: 358,253  • PostByPlan01 ns/op: 22,932  • 失敗率: 0.00%
• MySQL FindOne: 11,169 ns/op（FreeGo） vs 16,471 ns/op（GORM）
• MySQL Update: 180,680 ns/op（FreeGo） vs 358,455 ns/op（GORM）
• 併發連線: 10,000+  • 失敗率: 0.00%
```

## 🔧 快速開始

### 📦 安裝

```bash
go get github.com/godaddy-x/freego
```

### 🚀 基礎範例

```go
package main

import (
    "github.com/godaddy-x/freego/node"
    "github.com/godaddy-x/freego/utils/jwt"
)

func main() {
    httpNode := &node.HttpNode{}

    // 配置 JWT 認證
    httpNode.AddJwtConfig(jwt.JwtConfig{
        TokenKey: "your-256-bit-secret-key",
        TokenExp: jwt.ONE_HOUR,
    })

    // 新增路由
    httpNode.GET("/health", func(ctx *node.Context) error {
        return ctx.Json(map[string]interface{}{"status": "ok"})
    })

    // 啟動服務
    httpNode.StartServer(":8080")
}
```

## 🔐 安全特性

### 抗量子攻擊與認證體系

- **Plan2 登入授權（抗量子攻擊）**: **ML-KEM-1024** 單向封裝協商會話金鑰 + **ML-DSA-87** 雙向外層身份簽（`e`），HTTP/WebSocket/RPCX 主鏈路已移除 Ed25519/X25519
- **JWT Token**: 典型為 **HMAC-SHA256（HS256）** 簽名第三段，支援短 `exp` 與 RBAC（細節以 `utils/jwt` 與路由配置為準）
- **會話 Secret**: `GetTokenSecret` 與簽發金鑰協同派生，不落庫、按需派生，供報文 MAC /（可選）加密
- **HMAC-SHA256**: 業務 `s` 與推送統一完整性 MAC（`SignBodyMessage`）
- **AES-256-GCM**: 載荷認證加密（按 Plan / 路由啟用）

### 多重認證體系（登入後）

- 登入態 **Plan0/1**：JWT（HMAC-SHA256）+ 報文 **`s`（HMAC-SHA256）** +（可選）AES-GCM
- 匿名 **Plan2**：在上列基礎上增加 ML-KEM/ML-DSA 非對稱保護（詳見安全文件）

### 安全機制

在密碼學能力之上，框架在**請求進入業務前**強制執行下列機制（HTTP / WebSocket / RPCX 主鏈路一致，細節以原始碼為準）：

| 機制 | 作用 | 實作要點 |
|------|------|----------|
| **時間窗** | 限制過期請求 | 預設 **±5 分鐘**（`jwt.FIVE_MINUTES`），校驗 `body.t` |
| **協議 Nonce** | 防重放、唯一請求 | `n` = Base64(**32B** CSPRNG)；`ValidProtocolNonce`；Redis 去重（典型 TTL **10 分鐘**） |
| **簽名去重** | 防同一 `s` 重放 | `validReplayAttack` 對 HMAC 簽名快取拒絕重複 |
| **規範串綁定** | 防跨介面/降級 | MAC/AAD 綁定 **path + d + n + t + p (+ u)**；HTTP 要求 `body.r` 為空 |
| **Plan 分流** | 按模式校驗 | HTTP 以 **`p`** 區分 Plan01 / Plan2；WS 另結合 **`UsePlan2`** 與 `KeyRoute`/`LoginRoute` |
| **雙層校驗（Plan2）** | 身份 + 完整性 | 先驗 **ML-DSA** 外層 `e`，再驗 **HMAC-SHA256**（`s`）與 GCM |
| **推送驗簽** | 防偽造廣播 | `c=300`；`s` 演算法與業務相同，金鑰為 **廣播金鑰**（`PushKeyProvider` / `SetBroadcastKey`） |
| **過濾器鏈** | 認證與濫用控制 | 閘道/方法/使用者 **三級限流**；**SessionFilter**（JWT）；**RoleFilter**（RBAC） |
| **常數時間比較** | 降低時序風險 | `CompareBase64Sign` / `subtle.ConstantTimeCompare` 校驗 `s` |

**欄位速查（易混項）：**

- **`n`**：協議隨機數（32 位元組），≠ GCM 密文內 12 位元組 IV，≠ 訂閱 ID 用的 UUID。
- **`s`**：對稱 MAC（**HMAC-SHA256**）；Plan2 另有非對稱 **`e`**（ML-DSA）。
- **Token / Secret**：JWT 為 **HMAC-SHA256** 典型棧；會話 Secret 由 `GetTokenSecret` 等與簽發金鑰協同派生（實作見原始碼）。

安全架構與 Plan 流程見 [`README_SECURITY_TW.md`](./README_SECURITY_TW.md)。

## 📈 效能對比

### HTTP API 效能

| 測試場景 | SDK 方法 | Benchmark | 壓測口徑 | 每秒執行數 | `ns/op` | `B/op` | 失敗率 |
| ------------ | -------- | --------- | -------- | ---------- | ------- | ------ | ------ |
| Plan2 匿名登入（ML-DSA + ML-KEM） | `PostByPlan2` | `BenchmarkHttpSDK_PostByPlan2` | 1m × 1 run | ≈ 3,393/s | **358,253** | 175,721 | 0.00% |
| Plan0/1 登入態請求 | `PostByPlan01` | `BenchmarkHttpSDK_PostByPlan01` | 1m × 1 run | ≈ 52,320/s | **22,932** | 4,996 | 0.00% |

> 完整方法、原始輸出與失敗統計口徑見 [`http_benchmark_report.md`](./http_benchmark_report.md)。

### MySQL ORM：FreeGo 全面領先 GORM（獨立進程 60s）

獨立進程、每項 **60s** 壓測下，FreeGo（sqld）在讀/寫 **7 個場景 `ns/op` 均低於 GORM**；FindList 1000/2000 條優勢最大（分別快 **≈3.5× / 4.3×**），各分項失敗率均為 **0%**。

| 場景 | FreeGo 領先 | FreeGo（sqld）`ns/op` | GORM `ns/op` |
| -------------- | ----------- | --------------------- | ------------ |
| FindOne | **快 1.47×** | **11,169** | 16,471 |
| FindList 100 | **快 1.53×** | **165,937** | 253,354 |
| FindList 500 | **快 1.38×** | **596,669** | 825,536 |
| FindList 1000 | **快 3.48×** | **422,738** | 1,472,001 |
| FindList 2000 | **快 4.25×** | **751,271** | 3,189,665 |
| Save | **快 1.22×** | **301,592** | 368,179 |
| Update | **快 1.98×** | **180,680** | 358,455 |

> 完整環境、對齊條件與原始輸出見 [`orm_performance_report.md`](./orm_performance_report.md)。MongoDB 見 [`mongodb_performance_report.md`](./mongodb_performance_report.md)。

## 🗄️ ORM 特性

### 核心優化技術

- **零記憶體浪費**: 精確容量預分配，避免擴容
- **極低反射開銷**: 熱點路徑側重直接裝配與預分配，減少反射依賴；中繼資料解析（以 `ormx` 為準）
- **智慧預估**: 遞迴 OR 條件精確容量計算
- **高併發支援**: 原子操作和智慧連線池

### 適用場景

- 高頻資料庫操作
- 大資料量處理
- 記憶體敏感應用
- 強一致、高效能或資料密集型業務系統

## 🎯 選擇指南

### 選擇 FreeGo 的理由

| 需求場景 | FreeGo 優勢 | 適用專案 |
| ------------- | -------------------------------------------------- | --------------- |
| 🚀 高效能 | MySQL 多場景 `ns/op` 低於 GORM（約 1.22×~4.25×） | 高併發 Web 服務 |
| 🔒 強安全 / 支付場景 | Plan2 抗量子登入 + JWT/報文 **HMAC-SHA256**（見安全文件） | 金融、支付系統 |
| 💾 記憶體優化 | 在 Save / Update 等寫入路徑 `B/op`、`allocs/op` 更低 | 記憶體敏感應用 |
| 🗄️ 資料庫密集 | 極低反射熱點 ORM，智慧容量預估 | 資料密集型系統 |

### 快速部署

```dockerfile
FROM golang:1.26-alpine
WORKDIR /app
COPY . .
RUN go build -o main .
CMD ["./main"]
```

## 📞 聯絡與支援

- 📧 **GitHub**: [https://github.com/godaddy-x/freego](https://github.com/godaddy-x/freego)
- 🐛 **Issues**: [回報問題](https://github.com/godaddy-x/freego/issues)
- 📖 **安全文件**: [`README_SECURITY_TW.md`](./README_SECURITY_TW.md)
- 📊 **效能報告**: [`http_benchmark_report.md`](./http_benchmark_report.md) · [`orm_performance_report.md`](./orm_performance_report.md) · [`mongodb_performance_report.md`](./mongodb_performance_report.md)

歡迎透過 [Issues](https://github.com/godaddy-x/freego/issues) 回饋問題與建議。
