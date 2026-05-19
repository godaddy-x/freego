# FreeGo High-Performance Framework (Quantum Attack Resistant)

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.26-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/godaddy-x/freego)](https://goreportcard.com/report/github.com/godaddy-x/freego)

**Languages:** [简体中文](README.md) · [English](README_EN.md) · [繁體中文](README_TW.md)

> 🚀 **A Go enterprise framework focused on extreme performance, quantum attack resistance, and strong security** (architecture and flows in [`README_SECURITY_EN.md`](./README_SECURITY_EN.md))

FreeGo targets high-concurrency, security-sensitive workloads: **Plan2 login / key exchange** uses **NIST post-quantum algorithms (ML-KEM-1024 / ML-DSA-87)**; logged-in **JWT signatures use HMAC-SHA256 (typical HS256)**, business messages use field **`s` for HMAC-SHA256 integrity**, with optional **AES-256-GCM** for payloads. Core strengths are the **service access layer**, **data access layer**, and the supporting components below:

- **API framework**: HTTP / WebSocket / RPCX ingress, filter chains, authentication, authorization, integrity verification, and anti-replay (see [`README_SECURITY_EN.md`](./README_SECURITY_EN.md)).
- **ORM framework**: MySQL / Mongo with **minimal reflection on hot paths** and allocation-friendly data access (`ormx`).

Also includes cache (local and Redis), distributed locks and rate limiting, AMQP messaging, structured logging, and YAML configuration loading. **HTTPS/TLS is usually terminated at a reverse proxy or gateway**; the framework focuses on application-layer routing, filters, and cryptography (details in the security doc).

## 📚 Table of Contents

- [🚀 Framework Features](#-framework-features)
  - [Quantum Attack Resistance & High-Strength Cryptography](#quantum-attack-resistance--high-strength-cryptography)
  - [Core Architecture](#core-architecture)
- [🔧 Quick Start](#-quick-start)
- [🔐 Security](#-security)
- [📈 Performance](#-performance)
- [🗄️ ORM](#-orm)
- [🎯 Choosing FreeGo](#-choosing-freego)
- [📞 Contact & Support](#-contact--support)

## 🚀 Framework Features

### 🌐 Server & API Framework

| Feature | Description | Benefit |
| -------- | ----------- | ------- |
| 🚀 High-performance HTTP | HTTP engine with significantly higher throughput than `net/http` in typical benchmarks | 50,000+ QPS per node (hardware-dependent) |
| 🛡️ Quantum attack resistance | Plan2: **ML-KEM-1024** negotiation + **ML-DSA-87** mutual outer signatures | NIST PQC; login/key exchange without ECC/RSA |
| 🔐 High-strength symmetric stack | JWT: **HMAC-SHA256 (HS256)**; messages: **`s`: HMAC-SHA256** + (optional) AES-256-GCM | Aligns with common JWT / message MAC stacks |
| 🔒 Anti-replay | Protocol `n` (32B random) + timestamp + `s` (HMAC-SHA256) | Unified MAC for business and push (see security doc) |
| 👥 RBAC | Role-based access control | Enterprise-grade authorization |
| ⚡ Three-tier rate limiting | Gateway / method / user limits | Overload protection |
| 🔧 Filter chain | Full middleware pipeline | Extensible |

### 🗄️ ORM Database Framework

| Feature | Description | Performance |
| -------- | ----------- | ------------- |
| 💾 Zero memory waste | Precise capacity pre-allocation | 90%+ less GC pressure in typical batch workloads |
| ⚡ Minimal reflection | Fewer reflection hot spots vs typical ORMs; metadata may still use reflection (`ormx`) | Direct assembly + type constraints |
| 🧠 Smart estimation | Recursive OR conditions for exact capacity | Faster complex queries |
| 🔄 High concurrency | Smart connection pool + atomics | 10,000+ concurrent connections |

### Quantum Attack Resistance & High-Strength Cryptography

FreeGo uses a **NIST-standardized, production-ready** post-quantum cryptography stack:

| Phase | Capability | Algorithm | Quantum-resistant role |
| ----- | ------------ | --------- | ---------------------- |
| **Anonymous login / key exchange** | Plan2 (`p=2`) | **ML-KEM-1024** encapsulation + **ML-DSA-87** outer sign | NIST PQC; **1024-level KEM** replaces X25519/ECDH |
| **Token issue & verify** | JWT third segment | **HMAC-SHA256 (HS256)** | Standard JWT signature over part3; short `exp` and key rotation |
| **Session secret** | `GetTokenSecret` | Material derived with the signing key (see source) | For message MAC / (optional) AES-GCM; derived on demand, not stored |
| **Message integrity** | Field `s` | **HMAC-SHA256** | Unified business and push integrity; matches `SignBodyMessage` (see security doc) |
| **Payload confidentiality** | Plan1/2 encryption | **AES-256-GCM** | 256-bit keys; ~**128-bit** effective strength under Grover |

**How to read “quantum attack resistant”:**

- **Asymmetric (main PQ upgrade)**: Plan2 login and `/key` handshake use **ML-KEM + ML-DSA**, not Ed25519/X25519/RSA, addressing public-key threats under large-scale quantum models.
- **Symmetric (after token)**: JWT and message MAC use **HMAC-SHA256**; payloads may use **AES-256-GCM**. Combine **key rotation, short `exp`, replay controls (`n`/`t`/`s`), and TLS**.
- **Scope**: Plan0/1 post-login frames do not carry ML-DSA outer signatures; they rely on **JWT (HMAC-SHA256) + message HMAC-SHA256 + (optional) GCM**. For per-frame PQ outer signatures, use Plan2 or RPCX ML-DSA paths. Terminate **TLS 1.3** at the gateway when possible.

Flows and layered defenses: [`README_SECURITY_EN.md`](./README_SECURITY_EN.md).

### Core Architecture

```text
┌─────────────────────────────────────────────────────────────────┐
│                      FreeGo Framework                           │
├─────────────────────────────────────────────────────────────────┤
│              HTTP · WebSocket · RPCX                              │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  • 50,000+ QPS (typical HTTP, hardware-dependent)         │   │
│  │  • Sub-millisecond latency (typical routes)               │   │
│  └──────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                  Filter Chain                                     │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │ Rate limit │ Params     │ Session    │ RBAC       │ Custom│   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
├─────────────────────────────────────────────────────────────────┤
│           Security & Crypto (Plan2 PQ + JWT / HMAC-SHA256)        │
│  ┌────────────┬────────────┬────────────┬────────────┬──────┐   │
│  │ JWT/HS256  │ ML-DSA-87  │ ML-KEM-1024│ AES-256-GCM│HMAC256│   │
│  └────────────┴────────────┴────────────┴────────────┴──────┘   │
├─────────────────────────────────────────────────────────────────┤
│                Business Logic Layer                               │
├─────────────────────────────────────────────────────────────────┤
│                  ORM Layer                                        │
├─────────────────────────────────────────────────────────────────┤
│                 MySQL · MongoDB · Redis · Locks                   │
└─────────────────────────────────────────────────────────────────┘

Benchmark excerpts (2026-05-19, `http_test.go`, 1m local runs; see [`http_benchmark_report.md`](./http_benchmark_report.md)):
• `PostByPlan2` (ML-KEM+ML-DSA Plan2 login, `/login`): ≈3,393 TPS  • `PostByPlan01` (JWT+HMAC-SHA256 logged-in, `/getUser`): ≈52,320 TPS
• PostByPlan2 ns/op: 358,253  • PostByPlan01 ns/op: 22,932  • Fail rate: 0.00%
• MySQL FindOne: 11,169 ns/op (FreeGo) vs 16,471 ns/op (GORM)
• MySQL Update: 180,680 ns/op (FreeGo) vs 358,455 ns/op (GORM)
• Concurrent connections: 10,000+  • Failure rate: 0.00% (sample batch)
```

## 🔧 Quick Start

### 📦 Install

```bash
go get github.com/godaddy-x/freego
```

### 🚀 Basic Example

```go
package main

import (
    "github.com/godaddy-x/freego/node"
    "github.com/godaddy-x/freego/utils/jwt"
)

func main() {
    httpNode := &node.HttpNode{}

    httpNode.AddJwtConfig(jwt.JwtConfig{
        TokenKey: "your-256-bit-secret-key",
        TokenExp: jwt.ONE_HOUR,
    })

    httpNode.GET("/health", func(ctx *node.Context) error {
        return ctx.Json(map[string]interface{}{"status": "ok"})
    })

    httpNode.StartServer(":8080")
}
```

## 🔐 Security

### Quantum Attack Resistance & Authentication

- **Plan2 login (PQ)**: **ML-KEM-1024** encapsulation + **ML-DSA-87** mutual outer signatures (`e`); HTTP/WebSocket/RPCX main path no longer uses Ed25519/X25519
- **JWT**: Typically **HMAC-SHA256 (HS256)** for the third segment; short `exp` and RBAC (see `utils/jwt` and route config)
- **Session secret**: `GetTokenSecret` derives with the signing key on demand, not stored; used for message MAC / (optional) encryption
- **HMAC-SHA256**: Unified integrity MAC for business `s` and push (`SignBodyMessage`)
- **AES-256-GCM**: Authenticated encryption (per Plan / route)

### Post-Login Authentication Modes

- **Plan0/1 (logged in)**: JWT (HMAC-SHA256) + message **`s` (HMAC-SHA256)** + (optional) AES-GCM
- **Plan2 (anonymous)**: Above plus ML-KEM/ML-DSA asymmetric protection (see security doc)

### Security Mechanisms

Enforced **before business handlers** on HTTP / WebSocket / RPCX (see source for details):

| Mechanism | Purpose | Implementation |
| --------- | ------- | ---------------- |
| **Time window** | Reject stale requests | Default **±5 minutes** (`jwt.FIVE_MINUTES`), `body.t` |
| **Protocol nonce** | Anti-replay, uniqueness | `n` = Base64(**32B** CSPRNG); `ValidProtocolNonce`; Redis dedup (~**10 min** TTL) |
| **Signature dedup** | Block replay of same `s` | `validReplayAttack` caches HMAC signatures |
| **Canonical binding** | Anti cross-route / downgrade | MAC/AAD bind **path + d + n + t + p (+ u)**; HTTP requires empty `body.r` |
| **Plan routing** | Mode-specific validation | HTTP uses **`p`**; WS also **`UsePlan2`** + `KeyRoute`/`LoginRoute` |
| **Dual verify (Plan2)** | Identity + integrity | **ML-DSA** outer `e`, then **HMAC-SHA256** (`s`) and GCM |
| **Push verify** | Anti forged broadcast | `c=300`; same MAC algorithm; **broadcast key** (`PushKeyProvider` / `SetBroadcastKey`) |
| **Filter chain** | Auth & abuse control | **Three-tier** rate limits; **SessionFilter** (JWT); **RoleFilter** (RBAC) |
| **Constant-time compare** | Timing attack mitigation | `CompareBase64Sign` / `subtle.ConstantTimeCompare` for `s` |

**Field cheat sheet:**

- **`n`**: Protocol nonce (32 bytes), ≠ 12-byte GCM IV inside ciphertext, ≠ subscription UUID.
- **`s`**: Symmetric MAC (**HMAC-SHA256**); Plan2 also has asymmetric **`e`** (ML-DSA).
- **Token / Secret**: JWT uses the **HMAC-SHA256** stack; session secret comes from `GetTokenSecret` with the signing key (see source).

Architecture and Plan flows: [`README_SECURITY_EN.md`](./README_SECURITY_EN.md).

## 📈 Performance

### HTTP API

| Scenario | SDK API | Benchmark | Run | Throughput | `ns/op` | `B/op` | Fail rate |
| -------- | ------- | --------- | --- | ---------- | ------- | ------ | --------- |
| Plan2 anonymous login (ML-DSA + ML-KEM) | `PostByPlan2` | `BenchmarkHttpSDK_PostByPlan2` | 1m × 1 | ≈ 3,393/s | **358,253** | 175,721 | 0.00% |
| Plan0/1 authenticated request | `PostByPlan01` | `BenchmarkHttpSDK_PostByPlan01` | 1m × 1 | ≈ 52,320/s | **22,932** | 4,996 | 0.00% |

> Full methodology: [`http_benchmark_report.md`](./http_benchmark_report.md).

### ORM (MySQL, 60s isolated process)

| Scenario | FreeGo (sqld) | GORM | GORM / FreeGo |
| -------- | ------------- | ---- | ------------- |
| FindOne `ns/op` | **11,169** | **16,471** | **≈ 1.47×** |
| FindList 100 `ns/op` | **165,937** | **253,354** | **≈ 1.53×** |
| FindList 500 `ns/op` | **596,669** | **825,536** | **≈ 1.38×** |
| FindList 1000 `ns/op` | **422,738** | **1,472,001** | **≈ 3.48×** |
| FindList 2000 `ns/op` | **751,271** | **3,189,665** | **≈ 4.25×** |
| Save `ns/op` | **301,592** | **368,179** | **≈ 1.22×** |
| Update `ns/op` | **180,680** | **358,455** | **≈ 1.98×** |

> From [`orm_performance_report.md`](./orm_performance_report.md). MongoDB: [`mongodb_performance_report.md`](./mongodb_performance_report.md).

## 🗄️ ORM

### Core Techniques

- **Zero memory waste**: Exact pre-allocation, fewer expansions
- **Minimal reflection**: Hot paths favor direct assembly; metadata via `ormx`
- **Smart estimation**: Recursive OR capacity calculation
- **High concurrency**: Atomics and connection pooling

### Use Cases

- High-frequency database access
- Large data volumes
- Memory-sensitive applications
- Strong consistency, high performance, data-intensive systems

## 🎯 Choosing FreeGo

| Scenario | FreeGo advantage | Typical projects |
| -------- | ---------------- | ---------------- |
| 🚀 High performance | Lower `ns/op` than GORM on many MySQL paths (~1.22×–4.25×) | High-concurrency web services |
| 🔒 Strong security / payments | Plan2 PQ login + JWT / message **HMAC-SHA256** (see security doc) | Finance, payments |
| 💾 Memory optimization | Lower `B/op` and `allocs/op` on Save/Update paths | Memory-sensitive apps |
| 🗄️ Data-intensive | Low-reflection ORM, smart capacity estimation | Data platforms |

### Quick Deploy

```dockerfile
FROM golang:1.26-alpine
WORKDIR /app
COPY . .
RUN go build -o main .
CMD ["./main"]
```

## 📞 Contact & Support

- 📧 **GitHub**: [https://github.com/godaddy-x/freego](https://github.com/godaddy-x/freego)
- 🐛 **Issues**: [Report an issue](https://github.com/godaddy-x/freego/issues)
- 📖 **Security**: [`README_SECURITY_EN.md`](./README_SECURITY_EN.md)
- 📊 **Benchmarks**: [`http_benchmark_report.md`](./http_benchmark_report.md) · [`orm_performance_report.md`](./orm_performance_report.md) · [`mongodb_performance_report.md`](./mongodb_performance_report.md)

Feedback and suggestions welcome via [Issues](https://github.com/godaddy-x/freego/issues).
