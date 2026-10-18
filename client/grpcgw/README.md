# grpcgw 客户端（Plan2 / Plan1）

与 `server/grpcgw` 配套，供 **wallet-mpc-broker**、**wallet-mpc-node**、**open_gateway api_main** 等使用 gRPC 承载原 WebSocket 信封语义（`transport_version = grpcgw-v1`）。

协议字段与 canonical 见 open_gateway：[GRPC_GATEWAY_PROTOCOL.md](https://github.com/godaddy-x/open_gateway/blob/main/docs/GRPC_GATEWAY_PROTOCOL.md)。

---

## 目录

| 路径 | 用途 |
|------|------|
| `client.go` | 低层连接、Plan2 握手 |
| `plan2_session.go` / `plan01_session.go` | 会话密钥与 Seal/Unseal |
| `seal.go` | `p=0/1/2` 载荷加密 |
| `sdk/sdk.go` | **业务 SDK**：`Call(route, req, resp)`、流式 push |
| `sdk/login.go` | Plan2 登录重试 |
| `relay/sdk.go` | **节点中继**侧 SDK（broker `:port+100`） |

服务端注册与拦截器：`server/grpcgw/`（`AddMethod`、`MethodConfig` 的 `UsePlan2` / `AesRequest` 等）。

---

## 典型用法（CLI 租户）

```go
import grpcsdk "github.com/godaddy-x/freego/client/grpcgw/sdk"

cli, err := grpcsdk.New(grpcsdk.Options{
    Domain:    "127.0.0.1:9422",
    ClientNo:  3,
    Source:    "<tenant>",
    ClientPrk: "<ML-DSA-87 private b64>",
    ServerPub: "<broker ML-DSA-87 public b64>",
    KeyRoute:  "/api/PublicKey",
    LoginRoute: "/api/Login",
})
// Plan2 登录 + 保持长连接
tok, err := cli.LoginPlan2WithRetry(...)
// Plan1 业务：route 名与 broker apiproto 注册一致，如 "SignTransaction"
err = cli.Call("SignTransaction", reqBytes, respBytes, timeoutSec)
```

broker 封装：`wallet-mpc-broker/grpcli`（`OpenSession`、`CallAPI` + `protocol/apiproto` pb）。

节点中继：`:9522`，`client/grpcgw/relay` + `protocol/brokerproto` / `protocol/mpcproto`（与 broker `app/grpc_relay.go` 对齐）。

---

## 与 WebSocket 栈差异

| 项 | WS（历史） | grpcgw |
|----|------------|--------|
| 传输 | WebSocket 帧 | gRPC `Gateway` 服务 |
| 业务体 | JSON dto / easyjson | **protobuf**（L2）+ 信封 `d/n/s/e`（L1） |
| canonical | `r\|d\|n\|t\|p\|u` 文本 | `GRPCGWv1` 二进制域分离（见协议文档） |
| 同 tenant 长连 | `SubjectDeviceUnique` 踢线 | 同样仅保留一条 DeviceAPI 会话 |

压测与 E2E：`wallet-mpc-broker` 的 `sign_stress_*`、`keygen_stress_test.go`、`keygen_grpc_e2e_test.go` 均走本 SDK。

---

## 相关

- `protocol/grpcgw/envelope.proto`、`gateway.proto`
- `server/grpcgw/example/` — Plan2 登录与 bench 样例
