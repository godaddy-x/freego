package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
	"github.com/godaddy-x/freego/zlog"

	"testing"

	"github.com/godaddy-x/freego/node"
	"github.com/godaddy-x/freego/utils"
	"github.com/godaddy-x/freego/utils/sdk"
	"github.com/valyala/fasthttp"
)

const (
	// 服务端 Ed25519 私钥（与 utils/crypto TestFixtureEd25519KeysB64 种子 freego-ed25519-server 一致）
	serverPrk = "iOtq+nAOZieY+puLqeFPw3CvrI1OO8iQ9GrhccdovZ5+/Ta7hgR19V2RA4jk9PnQdljPvHJmWfsVMyPGNZhWHA=="
	// 服务端 Ed25519 公钥
	serverPub = "fv02u4YEdfVdkQOI5PT50HZYz7xyZln7FTMjxjWYVhw="
	// 客户端 Ed25519 私钥（种子 freego-ed25519-client）
	clientPrk = "T9arYQw2qGrcyN1kLvrVyP7jXKJe+cXIW5RNFXrvLEx1kuxLxKR5GXUihsj75z8GT+Xh0rfDxM0TOdXqQI1fog=="
	// 客户端 Ed25519 公钥
	clientPub = "dZLsS8SkeRl1IobI++c/Bk/l4dK3w8TNEznV6kCNX6I="

	// wsTestServerPool 用于 TestCreateWsServer / TestWebSocketSDKUsage：提高 maxConn、放宽 Upgrade 限流，
	// 便于压测高并发；生产环境请按容量收紧。单机若句柄/端口耗尽需调 OS 或降低 wsTestServerMaxConn。
	wsTestServerMaxConn     = 200000 // 最大并发连接数
	wsTestServerConnPerSec  = 100000 // 每秒允许的新建连接（serveHTTP 里 limiter.Allow）
	wsTestServerConnBurst   = 250000 // 突发允许的建连次数（短窗大量 Upgrade 时不易被限流误伤）
	wsTestServerPingSeconds = 30     // 心跳间隔（秒）
)

// testMessageHandler 测试用的消息处理器
type testMessageHandler struct {
	receivedMessages []*node.JsonResp
	messageCount     int
	mu               sync.Mutex
}

// HandleMessage 实现MessageHandler接口
func (h *testMessageHandler) HandleMessage(message *node.JsonResp) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.receivedMessages = append(h.receivedMessages, message)
	h.messageCount++

	if zlog.IsDebug() {
		zlog.Debug("test handler received message", 0,
			zlog.String("data", message.Data),
			zlog.String("router", message.Router))
	}

	return nil
}

// rawWsHandshakeThenClose 与 serverAddr 建立 TCP 连接，发送 WebSocket HTTP 升级（带 token），读 101 后立即关闭连接，不发送 WS Close 帧，用于模拟客户端意外断开。
func rawWsHandshakeThenClose(serverAddr, token string) error {
	keyBuf := make([]byte, 16)
	_, _ = rand.Read(keyBuf)
	secKey := base64.StdEncoding.EncodeToString(keyBuf)
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return err
	}
	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + serverAddr + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Key: " + secKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Authorization: " + token + "\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		return err
	}
	buf := make([]byte, 512)
	_, _ = conn.Read(buf)
	_ = conn.Close()
	return nil
}

// TestWebSocketManyClient 300 个用户并发连接、发消息，并随机让部分用户“意外断开”（不发 WS Close），模拟真实场景；需自行在 8088 端口启动服务端。
func TestWebSocketManyClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping many-client WebSocket test in short mode")
	}

	const numClients = 300
	const numUnexpectedDisconnect = 50 // 随机 50 个用户意外断开，模拟掉线
	config := jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	}

	// 预生成 300 个用户的 token
	tokens := make([]sdk.AuthToken, numClients)
	for i := 0; i < numClients; i++ {
		subject := &jwt.Subject{}
		token := subject.Create(utils.NextSID()).Dev("APP").Generate(config)
		secretBytes := subject.GetTokenSecret(token, config.TokenKey)
		tokens[i] = sdk.AuthToken{
			Token:   token,
			Secret:  utils.Base64Encode(utils.Bytes2Str(secretBytes)),
			Expired: subject.Payload.Exp,
		}
	}

	// 随机选出“意外断开”的用户下标（可复现）
	rng := rand.New(rand.NewSource(99))
	dropIndices := make(map[int]bool)
	for len(dropIndices) < numUnexpectedDisconnect {
		dropIndices[rng.Intn(numClients)] = true
	}

	// 使用已有服务端，请自行在 8088 端口启动
	serverAddr := "localhost:8088"
	disconnectCh := make(chan struct{})
	var wg sync.WaitGroup
	var failMu sync.Mutex
	var connectFails, sendFails int

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		idx := i
		isDrop := dropIndices[idx]
		u := tokens[idx]
		if isDrop {
			// 模拟意外断开：原始 TCP 握手后立即关连接，不发送 WS Close
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(idx) * 50 * time.Millisecond)
				if err := rawWsHandshakeThenClose(serverAddr, u.Token); err != nil && idx < 3 {
					t.Logf("drop client %d raw handshake failed: %v", idx, err)
				}
			}()
		} else {
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(idx) * 50 * time.Millisecond)
				wsSdk := sdk.NewSocketSDK(serverAddr)
				wsSdk.AuthToken(u)
				wsSdk.SetClientNo(1)
				_ = wsSdk.SetEd25519Object(wsSdk.ClientNo, clientPrk, serverPub)
				wsSdk.SetHealthPing(10)
				if err := wsSdk.ConnectWebSocket(); err != nil {
					failMu.Lock()
					connectFails++
					failMu.Unlock()
					if idx < 3 {
						t.Logf("client %d connect failed: %v", idx, err)
					}
					return
				}
				defer wsSdk.DisconnectWebSocket()
				req := map[string]interface{}{"test": "并发用户"}
				resp1 := &sdk.AuthToken{}
				if err := wsSdk.SendWebSocketMessage("/ws/user", req, resp1, true, false, 10); err != nil {
					failMu.Lock()
					sendFails++
					failMu.Unlock()
					if idx < 3 {
						t.Logf("client %d send /ws/user failed: %v", idx, err)
					}
					return
				}
				resp2 := &sdk.AuthToken{}
				if err := wsSdk.SendWebSocketMessage("/ws/user2", req, resp2, true, true, 10); err != nil {
					failMu.Lock()
					sendFails++
					failMu.Unlock()
					if idx < 3 {
						t.Logf("client %d send /ws/user2 failed: %v", idx, err)
					}
					return
				}
				<-disconnectCh // 等主流程校验完连接数后再断开
			}()
		}
	}

	// 等待所有连接就绪（250 正常 + 50 已“意外断开”）
	time.Sleep(time.Duration(numClients)*50*time.Millisecond + 2*time.Second)
	time.Sleep(800 * time.Millisecond)
	close(disconnectCh)
	wg.Wait()

	if connectFails > 0 || sendFails > 0 {
		t.Errorf("300 并发: 连接失败 %d, 发送失败 %d", connectFails, sendFails)
	} else {
		t.Logf("300 并发: 全部连接并发送成功，其中 %d 个模拟意外断开", numUnexpectedDisconnect)
	}
}

// TestWebSocketStressConnectionRate1Minute 压测「1 分钟内成功建连次数（吞吐）」：建连后立即断开，测的是握手速率而非并发在线。
// 若要看「同一时间窗内有多少连接能到位并保持」，请用 TestWebSocketStressConnectionsHeld1Minute。
//
// 与 TestWebSocketManyClient 相同方式生成 JWT；每个 worker 循环：新 token → ConnectWebSocket() → 立即 DisconnectWebSocket。
//
// 前置：已启动 WS 服务（默认 localhost:8088），JWT 与 TestWebSocketManyClient 一致（TokenKey=123456），且服务端已配置与 socket_test 一致的 Ed25519（clientPrk/serverPub）。
//
// 环境变量（可选）：
//   - WS_STRESS_ADDR      服务地址，默认 localhost:8088
//   - WS_STRESS_WORKERS   并发 worker 数，默认 512
//   - WS_STRESS_DURATION  压测时长，time.ParseDuration 格式，默认 1m
//
// 运行示例：go test -run TestWebSocketStressConnectionRate1Minute -timeout 5m .
func TestWebSocketStressConnectionRate1Minute(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket stress test in short mode")
	}

	serverAddr := os.Getenv("WS_STRESS_ADDR")
	if serverAddr == "" {
		serverAddr = "localhost:8088"
	}
	workers := 512
	if v := os.Getenv("WS_STRESS_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			workers = n
		}
	}
	dur := time.Minute
	if v := os.Getenv("WS_STRESS_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			dur = d
		}
	}

	jwtConfig := jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	}

	deadline := time.Now().Add(dur)
	var okCnt, failCnt uint64

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				subject := &jwt.Subject{}
				token := subject.Create(utils.NextSID()).Dev("APP").Generate(jwtConfig)
				secretBytes := subject.GetTokenSecret(token, jwtConfig.TokenKey)
				auth := sdk.AuthToken{
					Token:   token,
					Secret:  utils.Base64Encode(utils.Bytes2Str(secretBytes)),
					Expired: subject.Payload.Exp,
				}

				wsSdk := sdk.NewSocketSDK(serverAddr)
				wsSdk.AuthToken(auth)
				wsSdk.SetClientNo(1)
				_ = wsSdk.SetEd25519Object(wsSdk.ClientNo, clientPrk, serverPub)
				wsSdk.SetHealthPing(30)

				if err := wsSdk.ConnectWebSocket(); err != nil {
					atomic.AddUint64(&failCnt, 1)
					continue
				}
				atomic.AddUint64(&okCnt, 1)
				wsSdk.DisconnectWebSocket()
			}
		}()
	}
	wg.Wait()

	sec := dur.Seconds()
	if sec < 1 {
		sec = 1
	}
	t.Logf("WS 建连压测完成 addr=%s workers=%d window=%s", serverAddr, workers, dur)
	t.Logf("成功建连=%d 失败=%d 平均 %.1f 连接/秒 (≈ %.0f 连接/分钟)",
		okCnt, failCnt, float64(okCnt)/sec, float64(okCnt)/sec*60)
	// 无 -v 时 t.Log 不可见，stdout 汇总便于直接 go test
	fmt.Printf("[WS stress churn] addr=%s window=%v workers=%d | 成功=%d 失败=%d | 平均 %.1f/s (≈%.0f/分钟)\n",
		serverAddr, dur, workers, okCnt, failCnt, float64(okCnt)/sec, float64(okCnt)/sec*60)
	if okCnt == 0 {
		t.Fatal("无成功建连，请检查服务是否启动、端口、JWT 与 Ed25519 是否与测试一致")
	}
}

// wsHeldFailReasons 聚合并输出建连失败原因（按 err.Error() 文本归类）。
type wsHeldFailReasons struct {
	mu sync.Mutex
	m  map[string]int64
}

func newWsHeldFailReasons() *wsHeldFailReasons {
	return &wsHeldFailReasons{m: make(map[string]int64)}
}

func wsHeldShortReason(err error) string {
	if err == nil {
		return "(nil)"
	}
	s := strings.TrimSpace(err.Error())
	if s == "" {
		return "(empty error)"
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

func bytesToMB(n uint64) float64 {
	return float64(n) / 1024.0 / 1024.0
}

func readMemStats() runtime.MemStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms
}

func (w *wsHeldFailReasons) Add(err error) {
	if err == nil {
		return
	}
	key := wsHeldShortReason(err)
	w.mu.Lock()
	w.m[key]++
	w.mu.Unlock()
}

func (w *wsHeldFailReasons) report(t *testing.T, title string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.m) == 0 {
		return
	}
	keys := make([]string, 0, len(w.m))
	for k := range w.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("[%s] 建连失败原因明细 (共 %d 类):\n", title, len(keys))
	for _, k := range keys {
		line := fmt.Sprintf("  x%d  %s", w.m[k], k)
		fmt.Println(line)
		if t != nil {
			t.Log(line)
		}
	}
}

// stressHeldWave 启动 workers 条并发：抖动后建连，成功则保持到 ctx 结束再断开。返回成功数、失败数及失败原因聚合。
func stressHeldWave(ctx context.Context, serverAddr string, workers, jitterMS int, jwtConfig jwt.JwtConfig) (okHeld, failHeld uint64, reasons *wsHeldFailReasons) {
	reasons = newWsHeldFailReasons()
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if jitterMS > 0 {
				time.Sleep(time.Duration(rand.Intn(jitterMS)) * time.Millisecond)
			}
			subject := &jwt.Subject{}
			token := subject.Create(utils.NextSID()).Dev("APP").Generate(jwtConfig)
			secretBytes := subject.GetTokenSecret(token, jwtConfig.TokenKey)
			auth := sdk.AuthToken{
				Token:   token,
				Secret:  utils.Base64Encode(utils.Bytes2Str(secretBytes)),
				Expired: subject.Payload.Exp,
			}

			wsSdk := sdk.NewSocketSDK(serverAddr)
			wsSdk.AuthToken(auth)
			wsSdk.SetClientNo(1)
			_ = wsSdk.SetEd25519Object(wsSdk.ClientNo, clientPrk, serverPub)
			wsSdk.SetHealthPing(30)

			if err := wsSdk.ConnectWebSocket(); err != nil {
				atomic.AddUint64(&failHeld, 1)
				reasons.Add(err)
				return
			}
			atomic.AddUint64(&okHeld, 1)
			<-ctx.Done()
			wsSdk.DisconnectWebSocket()
		}()
	}
	wg.Wait()
	return atomic.LoadUint64(&okHeld), atomic.LoadUint64(&failHeld), reasons
}

// TestWebSocketStressConnectionsHeld1Minute 压测「固定时间窗内有多少条连接能成功到位并保持到窗口结束」。
// 每个 goroutine：独立 JWT（NextSID）→ ConnectWebSocket() → 阻塞到时间窗结束 → Disconnect。
// 默认约 1.6 万并发尝试、保持 3 分钟（可按环境变量改）；成功数即该窗口内稳定活跃连接数上界估计。
// 若有建连失败，会按 err.Error() 归类打印原因明细。
//
// 环境变量（可选）：
//   - WS_STRESS_ADDR       与 churn 测试共用，默认 localhost:8088（本机端口紧张时可试 127.0.0.1:8088）
//   - WS_HOLD_WORKERS      同时发起的连接数，默认 16200（按本机测试收敛到约 1.6 万稳定活跃）
//   - WS_HOLD_DURATION     保持时长，默认 3m
//   - WS_HOLD_JITTER_MS    建连前随机休眠 [0,jitter) 毫秒，默认 7500（进一步削峰以降低本机端口/缓冲失败）
//
// 运行示例：go test -run TestWebSocketStressConnectionsHeld1Minute -timeout 10m .
// 分档探测上限见 TestWebSocketStressHeldStepProbe。
func TestWebSocketStressConnectionsHeld1Minute(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket held-connection stress test in short mode")
	}

	serverAddr := os.Getenv("WS_STRESS_ADDR")
	if serverAddr == "" {
		serverAddr = "localhost:8088"
	}
	workers := 16200
	if v := os.Getenv("WS_HOLD_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			workers = n
		}
	}
	dur := 3 * time.Minute
	if v := os.Getenv("WS_HOLD_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			dur = d
		}
	}
	jitterMS := 7500
	if v := os.Getenv("WS_HOLD_JITTER_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			jitterMS = n
		}
	}

	jwtConfig := jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	}

	beforeMS := readMemStats()
	beforeAt := time.Now()
	beforeGoroutines := runtime.NumGoroutine()
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	okHeld, failHeld, reasons := stressHeldWave(ctx, serverAddr, workers, jitterMS, jwtConfig)
	afterMS := readMemStats()
	afterAt := time.Now()
	afterGoroutines := runtime.NumGoroutine()

	t.Logf("WS 并发「到位」压测 addr=%s workers=%d 保持窗口=%s jitter_ms=%d", serverAddr, workers, dur, jitterMS)
	t.Logf("成功保持到窗口结束=%d 建连失败=%d（在 workers 次尝试下，成功数即当前时间窗内能稳定活跃的连接规模估计）",
		okHeld, failHeld)
	var failPct float64
	if workers > 0 {
		failPct = float64(failHeld) * 100 / float64(workers)
	}
	fmt.Printf("[WS stress held] addr=%s window=%v workers=%d jitter_ms=%d | 成功保持=%d 建连失败=%d (占尝试 %.1f%%)\n",
		serverAddr, dur, workers, jitterMS, okHeld, failHeld, failPct)
	gcCountDelta := int64(afterMS.NumGC) - int64(beforeMS.NumGC)
	pauseDeltaMs := float64(int64(afterMS.PauseTotalNs)-int64(beforeMS.PauseTotalNs)) / 1e6
	allocDeltaMB := bytesToMB(afterMS.TotalAlloc - beforeMS.TotalAlloc)
	fmt.Printf("[WS stress runtime] elapsed=%v goroutines=%d->%d GOMAXPROCS=%d | alloc=%.1fMB heap_alloc=%.1fMB heap_inuse=%.1fMB sys=%.1fMB | GC_count_delta=%d GC_pause_delta_ms=%.2f\n",
		afterAt.Sub(beforeAt),
		beforeGoroutines, afterGoroutines, runtime.GOMAXPROCS(0),
		allocDeltaMB, bytesToMB(afterMS.HeapAlloc), bytesToMB(afterMS.HeapInuse), bytesToMB(afterMS.Sys),
		gcCountDelta, pauseDeltaMs)
	if failHeld > 0 {
		reasons.report(t, "WS stress held")
	}
	if okHeld == 0 {
		t.Fatal("无成功连接，请检查服务、端口、JWT、Ed25519 是否与 socket_test 一致")
	}
}

// TestWebSocketStressHeldStepProbe 分档探测「大约多少并发同时到位」：多轮 workers 递增，每轮短保持后全部断开再下一轮。
// 便于观察从哪一档开始出现大量失败及失败原因（池满、429、网络拒绝等）。
//
// 环境变量：
//   - WS_STRESS_ADDR           默认 localhost:8088
//   - WS_HOLD_PROBE_STEPS      逗号分隔 workers，默认 15000,22000,30000,38000,46000,54000,62000,72000
//   - WS_HOLD_PROBE_HOLD       每轮保持时长，默认 30s（可设 1m）
//   - WS_HOLD_PROBE_JITTER_MS  每轮抖动上限毫秒，默认 4000
//
// 运行：go test -v -run TestWebSocketStressHeldStepProbe -timeout 45m .
func TestWebSocketStressHeldStepProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket step probe in short mode")
	}

	serverAddr := os.Getenv("WS_STRESS_ADDR")
	if serverAddr == "" {
		serverAddr = "localhost:8088"
	}
	stepsStr := os.Getenv("WS_HOLD_PROBE_STEPS")
	var steps []int
	if strings.TrimSpace(stepsStr) == "" {
		steps = []int{15000, 22000, 30000, 38000, 46000, 54000, 62000, 72000}
	} else {
		for _, p := range strings.Split(stepsStr, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			n, err := strconv.Atoi(p)
			if err != nil || n <= 0 {
				t.Fatalf("WS_HOLD_PROBE_STEPS 无效片段: %q", p)
			}
			steps = append(steps, n)
		}
		if len(steps) == 0 {
			t.Fatal("WS_HOLD_PROBE_STEPS 解析后为空")
		}
	}

	holdPer := 30 * time.Second
	if v := os.Getenv("WS_HOLD_PROBE_HOLD"); strings.TrimSpace(v) != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			holdPer = d
		}
	}
	jitterMS := 4000
	if v := os.Getenv("WS_HOLD_PROBE_JITTER_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			jitterMS = n
		}
	}

	jwtConfig := jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	}

	fmt.Printf("[WS step probe] addr=%s hold_per_wave=%v jitter_ms=%d steps=%v\n", serverAddr, holdPer, jitterMS, steps)
	t.Logf("WS step probe addr=%s hold=%v jitter=%d steps=%v", serverAddr, holdPer, jitterMS, steps)

	for _, n := range steps {
		ctx, cancel := context.WithTimeout(context.Background(), holdPer)
		ok, fail, reasons := stressHeldWave(ctx, serverAddr, n, jitterMS, jwtConfig)
		cancel()
		pct := float64(0)
		if n > 0 {
			pct = float64(fail) * 100 / float64(n)
		}
		fmt.Printf("[WS step probe] wave workers=%d | 成功=%d 失败=%d (失败率 %.1f%%)\n", n, ok, fail, pct)
		t.Logf("wave workers=%d ok=%d fail=%d fail_pct=%.1f", n, ok, fail, pct)
		if fail > 0 {
			reasons.report(t, fmt.Sprintf("WS step probe wave=%d", n))
		}
	}
}

// TestWebSocketSendRoundTripPerf 压测 SendWebSocketMessage 的服务端来回性能（请求+响应）。
// 流程：先并发建连并保持，再在固定窗口内每连接循环发送 "/ws/user" 消息，统计吞吐与延迟。
//
// 环境变量（可选）：
//   - WS_SEND_ADDR            默认读取 WS_STRESS_ADDR，否则 localhost:8088
//   - WS_SEND_CLIENTS         并发连接数，默认 3000
//   - WS_SEND_DURATION        发送窗口，默认 1m
//   - WS_SEND_CONNECT_JITTER  建连抖动毫秒，默认 3000
//   - WS_SEND_TIMEOUT_SEC     单次 SendWebSocketMessage 超时秒，默认 5
//   - WS_SEND_ROUTE           发送路由，默认 /ws/user
//
// 运行示例：
//
//	go test -count=1 -v -run TestWebSocketSendRoundTripPerf -timeout 15m .
func TestWebSocketSendRoundTripPerf(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket send roundtrip stress in short mode")
	}

	serverAddr := os.Getenv("WS_SEND_ADDR")
	if serverAddr == "" {
		serverAddr = os.Getenv("WS_STRESS_ADDR")
	}
	if serverAddr == "" {
		serverAddr = "localhost:8088"
	}
	clientsN := 3000
	if v := os.Getenv("WS_SEND_CLIENTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			clientsN = n
		}
	}
	sendWindow := time.Minute
	if v := os.Getenv("WS_SEND_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			sendWindow = d
		}
	}
	connectJitterMs := 3000
	if v := os.Getenv("WS_SEND_CONNECT_JITTER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			connectJitterMs = n
		}
	}
	timeoutSec := 5
	if v := os.Getenv("WS_SEND_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}
	route := os.Getenv("WS_SEND_ROUTE")
	if strings.TrimSpace(route) == "" {
		route = "/ws/user"
	}

	jwtConfig := jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	}
	beforeMS := readMemStats()
	beforeAt := time.Now()
	beforeGoroutines := runtime.NumGoroutine()

	type wsClient struct {
		sdk *sdk.SocketSDK
	}
	liveClients := make([]*wsClient, 0, clientsN)
	var liveMu sync.Mutex
	connectReasons := newWsHeldFailReasons()

	// 1) 建连阶段
	var connOk, connFail uint64
	var connWG sync.WaitGroup
	for i := 0; i < clientsN; i++ {
		connWG.Add(1)
		go func() {
			defer connWG.Done()
			if connectJitterMs > 0 {
				time.Sleep(time.Duration(rand.Intn(connectJitterMs)) * time.Millisecond)
			}
			subject := &jwt.Subject{}
			token := subject.Create(utils.NextSID()).Dev("APP").Generate(jwtConfig)
			secretBytes := subject.GetTokenSecret(token, jwtConfig.TokenKey)
			auth := sdk.AuthToken{
				Token:   token,
				Secret:  utils.Base64Encode(utils.Bytes2Str(secretBytes)),
				Expired: subject.Payload.Exp,
			}
			wsSdk := sdk.NewSocketSDK(serverAddr)
			wsSdk.AuthToken(auth)
			wsSdk.SetClientNo(1)
			_ = wsSdk.SetEd25519Object(wsSdk.ClientNo, clientPrk, serverPub)
			wsSdk.SetHealthPing(30)
			if err := wsSdk.ConnectWebSocket(); err != nil {
				atomic.AddUint64(&connFail, 1)
				connectReasons.Add(err)
				return
			}
			atomic.AddUint64(&connOk, 1)
			liveMu.Lock()
			liveClients = append(liveClients, &wsClient{sdk: wsSdk})
			liveMu.Unlock()
		}()
	}
	connWG.Wait()

	if len(liveClients) == 0 {
		t.Fatalf("建连全部失败 addr=%s clients=%d", serverAddr, clientsN)
	}

	// 2) 发送阶段
	sendReasons := newWsHeldFailReasons()
	sendCtx, sendCancel := context.WithTimeout(context.Background(), sendWindow)
	defer sendCancel()

	var totalReq, okResp, failResp uint64
	var totalLatencyNs uint64
	var maxLatencyNs int64

	latSample := make([]int64, 0, 200000)
	var latMu sync.Mutex
	sampleEvery := uint64(20) // 每20次采样一次，降低内存占用

	var sendWG sync.WaitGroup
	for _, c := range liveClients {
		client := c
		sendWG.Add(1)
		go func() {
			defer sendWG.Done()
			for {
				select {
				case <-sendCtx.Done():
					return
				default:
				}

				reqID := atomic.AddUint64(&totalReq, 1)
				reqBody := &sdk.AuthToken{
					Token:   "send_perf",
					Expired: int64(reqID),
				}
				respBody := &sdk.AuthToken{}
				begin := time.Now()
				err := client.sdk.SendWebSocketMessage(route, reqBody, respBody, true, false, int64(timeoutSec))
				latNs := time.Since(begin).Nanoseconds()

				atomic.AddUint64(&totalLatencyNs, uint64(latNs))
				for {
					old := atomic.LoadInt64(&maxLatencyNs)
					if latNs <= old || atomic.CompareAndSwapInt64(&maxLatencyNs, old, latNs) {
						break
					}
				}
				if reqID%sampleEvery == 0 {
					latMu.Lock()
					latSample = append(latSample, latNs)
					latMu.Unlock()
				}

				if err != nil {
					atomic.AddUint64(&failResp, 1)
					sendReasons.Add(err)
					continue
				}
				atomic.AddUint64(&okResp, 1)
			}
		}()
	}
	sendWG.Wait()

	// 3) 收尾：断开连接
	for _, c := range liveClients {
		if c != nil && c.sdk != nil {
			c.sdk.DisconnectWebSocket()
		}
	}

	// 4) 汇总输出
	connFailPct := float64(0)
	if clientsN > 0 {
		connFailPct = float64(connFail) * 100 / float64(clientsN)
	}
	totalDone := okResp + failResp
	failPct := float64(0)
	if totalDone > 0 {
		failPct = float64(failResp) * 100 / float64(totalDone)
	}
	sec := sendWindow.Seconds()
	if sec < 1 {
		sec = 1
	}
	avgMs := float64(0)
	if totalDone > 0 {
		avgMs = float64(totalLatencyNs) / float64(totalDone) / 1e6
	}
	p95Ms := float64(0)
	latMu.Lock()
	if len(latSample) > 0 {
		sort.Slice(latSample, func(i, j int) bool { return latSample[i] < latSample[j] })
		idx := int(float64(len(latSample)-1) * 0.95)
		p95Ms = float64(latSample[idx]) / 1e6
	}
	latMu.Unlock()
	maxMs := float64(maxLatencyNs) / 1e6

	fmt.Printf("[WS send perf] addr=%s route=%s clients=%d connected=%d conn_fail=%d(%.2f%%) window=%v\n",
		serverAddr, route, clientsN, connOk, connFail, connFailPct, sendWindow)
	fmt.Printf("[WS send perf] total_req=%d ok=%d fail=%d fail_rate=%.2f%% qps=%.1f avg_ms=%.2f p95_ms=%.2f max_ms=%.2f sample_n=%d\n",
		totalDone, okResp, failResp, failPct, float64(totalDone)/sec, avgMs, p95Ms, maxMs, len(latSample))
	afterMS := readMemStats()
	afterAt := time.Now()
	afterGoroutines := runtime.NumGoroutine()
	gcCountDelta := int64(afterMS.NumGC) - int64(beforeMS.NumGC)
	pauseDeltaMs := float64(int64(afterMS.PauseTotalNs)-int64(beforeMS.PauseTotalNs)) / 1e6
	allocDeltaMB := bytesToMB(afterMS.TotalAlloc - beforeMS.TotalAlloc)
	fmt.Printf("[WS send runtime] elapsed=%v goroutines=%d->%d GOMAXPROCS=%d | alloc=%.1fMB heap_alloc=%.1fMB heap_inuse=%.1fMB sys=%.1fMB | GC_count_delta=%d GC_pause_delta_ms=%.2f\n",
		afterAt.Sub(beforeAt),
		beforeGoroutines, afterGoroutines, runtime.GOMAXPROCS(0),
		allocDeltaMB, bytesToMB(afterMS.HeapAlloc), bytesToMB(afterMS.HeapInuse), bytesToMB(afterMS.Sys),
		gcCountDelta, pauseDeltaMs)

	t.Logf("WS send perf addr=%s route=%s clients=%d connected=%d conn_fail=%d window=%v",
		serverAddr, route, clientsN, connOk, connFail, sendWindow)
	t.Logf("WS send perf total=%d ok=%d fail=%d fail_rate=%.2f%% qps=%.1f avg_ms=%.2f p95_ms=%.2f max_ms=%.2f sample_n=%d",
		totalDone, okResp, failResp, failPct, float64(totalDone)/sec, avgMs, p95Ms, maxMs, len(latSample))

	if connFail > 0 {
		connectReasons.report(t, "WS send perf connect")
	}
	if failResp > 0 {
		sendReasons.report(t, "WS send perf send")
	}
	if okResp == 0 {
		t.Fatal("发送阶段无成功响应，请检查服务端 /ws/user 路由与鉴权配置")
	}
}

func TestCreateWsServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket SDK usage test in short mode")
	}

	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.INFO, Console: true}) // 测试环境使用空logger，避免输出干扰

	fmt.Println("=== WebSocket SDK 完整使用流程测试 ===")

	// 0. 启动测试服务器
	fmt.Println("0. 启动测试服务器...")

	// 创建WebSocket服务器实例
	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的Ed25519
	cipher, _ := crypto.CreateEd25519WithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 配置连接池（限流见文件顶部 wsTestServer* 常量）
	err := server.NewPool(wsTestServerMaxConn, wsTestServerConnPerSec, wsTestServerConnBurst, wsTestServerPingSeconds)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 添加业务路由处理器
	err = server.AddRouter("/ws/key", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		req := &node.PublicKey{}
		if err := utils.JsonUnmarshal(body, req); err != nil {
			return nil, err
		}
		return server.BuildPlan2KeyResponse(req)
	}, &node.RouterConfig{UseRSA: true, KeyRoute: true})
	if err != nil {
		t.Fatalf("Failed to add key router: %v", err)
	}

	err = server.AddRouter("/ws/login", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		req := &sdk.AuthToken{}
		if err := utils.JsonUnmarshal(body, req); err != nil {
			return nil, err
		}
		jwtConfig := jwt.JwtConfig{
			TokenTyp: jwt.JWT,
			TokenAlg: jwt.HS256,
			TokenKey: "123456",
			TokenExp: jwt.TWO_WEEK,
		}
		subject := &jwt.Subject{}
		token := subject.Create("1").Dev("APP").Generate(jwtConfig)
		secret := subject.GetTokenSecret(token, jwtConfig.TokenKey)
		return &sdk.AuthToken{
			Token:   token,
			Secret:  utils.Base64Encode(secret),
			Expired: utils.UnixSecond() + jwtConfig.TokenExp,
		}, nil
	}, &node.RouterConfig{UseRSA: true, LoginRoute: true})
	if err != nil {
		t.Fatalf("Failed to add login router: %v", err)
	}

	err = server.AddRouter("/ws/user", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		//fmt.Println("test", connCtx.GetUserID())
		ret := &sdk.AuthToken{
			Token:  "鲨鱼宝宝获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{AesRequest: true, AesResponse: true})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	err = server.AddRouter("/ws/user2", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		ret := &sdk.AuthToken{
			Token:  "鲨鱼爸爸获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	// 同步阻塞监听；用于本机常驻起服务时直接跑本测试。压测并发请把 WS_HOLD_WORKERS 调到接近或略大于 wsTestServerMaxConn。
	serverAddr := "localhost:8088"
	if err := server.StartWebsocket(serverAddr); err != nil {
		t.Errorf("Server start failed: %v", err)
	}
}

func TestWebSocketPlan2LoginGetToken(t *testing.T) {
	wsUserSdk := sdk.NewSocketSDK("localhost:8088")
	wsUserSdk.SetClientNo(1)
	wsUserSdk.SetLanguage("zh-CN")
	if err := wsUserSdk.SetEd25519Object(1, clientPrk, serverPub); err != nil {
		t.Fatalf("set user sdk ed25519 failed: %v", err)
	}
	wsUserSdk.SetTokenExpiredCallback(func() {
		loginSdk := sdk.NewSocketSDK("localhost:8088")
		loginSdk.SetClientNo(1)
		loginSdk.SetLanguage("zh-CN")
		if err := loginSdk.SetEd25519Object(1, clientPrk, serverPub); err != nil {
			t.Logf("token callback set ed25519 failed: %v", err)
			return
		}
		defer loginSdk.DisconnectWebSocket()
		req := sdk.AuthToken{Token: "plan2_refresh"}
		resp := sdk.AuthToken{}
		if err := loginSdk.LoginByWebSocketPlan2Auto("/ws/key", "/ws/login", &req, &resp, 5); err != nil {
			t.Logf("token callback plan2 auto login failed: %v", err)
			return
		}
		fmt.Println("token: ", resp)
		wsUserSdk.AuthToken(resp) // 回调里自动填充 token
	})
	if err := wsUserSdk.ConnectWebSocket(); err != nil {
		t.Fatalf("connect with jwt token failed: %v", err)
	}
	defer wsUserSdk.DisconnectWebSocket()

	userReq := map[string]interface{}{"test": "plan2_to_user"}
	userResp := &sdk.AuthToken{}
	if err := wsUserSdk.SendWebSocketMessage("/ws/user", userReq, userResp, true, true, 5); err != nil {
		t.Fatalf("ws user route call failed: %v", err)
	}
	if len(userResp.Token) == 0 {
		t.Fatalf("invalid user response: %+v", userResp)
	}
	fmt.Println("user: ", userResp)
}

// TestWebSocketSDKUsage 测试完整的SDK使用流程（包含服务器管理）
func TestWebSocketSDKUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket SDK usage test in short mode")
	}

	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true}) // 测试环境使用空logger，避免输出干扰

	fmt.Println("=== WebSocket SDK 完整使用流程测试 ===")

	// 0. 启动测试服务器
	fmt.Println("0. 启动测试服务器...")

	// 创建WebSocket服务器实例
	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的Ed25519
	cipher, _ := crypto.CreateEd25519WithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 配置连接池（限流见文件顶部 wsTestServer* 常量）
	err := server.NewPool(wsTestServerMaxConn, wsTestServerConnPerSec, wsTestServerConnBurst, wsTestServerPingSeconds)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 添加业务路由处理器
	err = server.AddRouter("/ws/user", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		ret := &sdk.AuthToken{
			Token:  "鲨鱼宝宝获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	err = server.AddRouter("/ws/user2", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		ret := &sdk.AuthToken{
			Token:  "鲨鱼爸爸获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	// 在goroutine中启动服务器
	serverAddr := "localhost:8088"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 使用 defer 确保服务器被停止
	defer func() {
		fmt.Println("正在停止测试服务器...")
		if err := server.StopWebsocket(); err != nil {
			t.Logf("Server stop failed: %v", err)
		}
		// 等待服务器完全停止
		select {
		case <-serverDoneCh:
			fmt.Println("测试服务器已停止")
		case <-time.After(5 * time.Second):
			t.Logf("服务器停止超时")
		}
	}()

	access_token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIyMDMyOTk2NTg1Mjg5Mjg1NjMzIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiZmQwMjAyZmI0NGI2NDNkODgzZGE3NGE4ODY3NGEyMDMiLCJleHQiOiIiLCJpYXQiOjAsImV4cCI6MTc4NTYzNTEzMX0=.OZpZC5/pFqm9H+PiolACHj0sP0SrTZrakhPz0FSWEFU="
	token_secret := "DjPI2P8Pud2dVUKfKCuAqu20/JC+7xIE3jECeID9vfU="
	token_expire := int64(1785635131)

	// {eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIyMDMyOTk2NTg1Mjg5Mjg1NjMzIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiZmQwMjAyZmI0NGI2NDNkODgzZGE3NGE4ODY3NGEyMDMiLCJleHQiOiIiLCJpYXQiOjAsImV4cCI6MTc4NTYzNTEzMX0=.OZpZC5/pFqm9H+PiolACHj0sP0SrTZrakhPz0FSWEFU= DjPI2P8Pud2dVUKfKCuAqu20/JC+7xIE3jECeID9vfU= 1785635131}
	// 1. 初始化SDK
	fmt.Println("1. 初始化SDK...")
	wsSdk := sdk.NewSocketSDK(serverAddr)

	// 2. 设置认证Token
	fmt.Println("2. 设置认证Token...")
	authToken := sdk.AuthToken{
		Token:   access_token,
		Secret:  token_secret,
		Expired: token_expire,
	}
	wsSdk.AuthToken(authToken)

	wsSdk.SetClientNo(1)
	wsSdk.SetEd25519Object(wsSdk.ClientNo, clientPrk, serverPub)
	wsSdk.SetHealthPing(5)

	// 5. 尝试连接WebSocket（预期成功，因为服务器已启动）
	fmt.Println("5. 尝试连接WebSocket（预期成功）...")
	err = wsSdk.ConnectWebSocket()
	if err != nil {
		t.Error("连接失败：", err)
		return
	}

	// 6. 发送WebSocket消息
	fmt.Println("6. 发送WebSocket消息...")
	requestObject := map[string]interface{}{"test": "张三"}
	responseObject := &sdk.AuthToken{}
	err = wsSdk.SendWebSocketMessage("/ws/user", requestObject, responseObject, true, false, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("明文响应结果1:", responseObject)

	requestObject = map[string]interface{}{"test": "张三"}
	responseObject = &sdk.AuthToken{}
	err = wsSdk.SendWebSocketMessage("/ws/user2", requestObject, responseObject, true, true, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("加密响应结果2:", responseObject)

	// 添加延迟等待响应
	time.Sleep(60 * time.Minute)

	// 验证连接状态
	if !wsSdk.IsWebSocketConnected() {
		t.Error("连接状态应该是true")
	}

	// 6. 测试Token过期回调（设置过期的token）
	fmt.Println("6. 测试Token过期场景...")
	expiredToken := sdk.AuthToken{
		Token:   "expired-token",
		Secret:  "expired-secret",
		Expired: utils.UnixSecond() - 100, // 已经过期
	}
	wsSdk.AuthToken(expiredToken)

	// 8. 测试发送同步消息（连接断开状态下）
	fmt.Println("8. 测试发送同步消息（连接断开状态）...")
	req := map[string]interface{}{"content": "hello"}
	res := map[string]interface{}{}
	err = wsSdk.SendWebSocketMessage("/ws/chat", &req, &res, true, true, 5)
	if err == nil {
		t.Error("在连接断开状态下发送消息应该失败")
	} else {
		fmt.Printf("   -> 发送失败（预期）: %v\n", err)
	}
	if len(res) != 0 {
		t.Error("断开连接时响应应该为nil")
	}

	// 9. 测试发送异步消息（连接断开状态下）
	//fmt.Println("9. 测试发送异步消息（连接断开状态）...")
	//err = wsSdk.SendWebSocketMessage("/ws/chat", map[string]interface{}{"content": "async hello"}, false, 0)
	//if err == nil {
	//	t.Error("在连接断开状态下发送异步消息应该失败")
	//} else {
	//	fmt.Printf("   -> 异步发送失败（预期）: %v\n", err)
	//}

	// 10. 测试重连功能
	fmt.Println("10. 测试重连功能...")
	// 这里会触发重连，但由于没有服务器会失败
	time.Sleep(2 * time.Second) // 等待可能的第一次重连尝试

	// 11. 强制重连测试
	fmt.Println("11. 测试强制重连...")
	err = wsSdk.ForceReconnect()
	if err == nil {
		t.Error("强制重连应该失败（无服务器）")
	} else {
		fmt.Printf("   -> 强制重连失败（预期）: %v\n", err)
	}

	// 13. 最终清理
	fmt.Println("13. 最终清理...")
	wsSdk.DisconnectWebSocket()

	// 验证清理后状态
	if wsSdk.IsWebSocketConnected() {
		t.Error("断开连接后状态应该是false")
	}

	fmt.Println("🎉 WebSocket SDK 完整使用流程测试完成!")
}

// TestWebSocketTokenExpiredCallback 测试Token过期回调功能
func TestWebSocketTokenExpiredCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket token expired callback test in short mode")
	}

	// 模拟外部认证接口
	type AuthResponse struct {
		Token   string `json:"token"`
		Secret  string `json:"secret"`
		Expired int64  `json:"expired"`
	}

	// 模拟认证成功次数
	authCallCount := 0

	// 使用与服务器相同的JWT密钥
	serverJwtKey := "123456_fixed_test_key_for_token_verification"

	// 外部认证函数 (模拟调用外部认证接口)
	externalAuthFunc := func() (*AuthResponse, error) {
		authCallCount++
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("External auth called (attempt %d)", authCallCount), 0)
		}

		// 使用与服务器相同的密钥生成token
		jwtConfig := jwt.JwtConfig{
			TokenTyp: jwt.JWT,
			TokenAlg: jwt.HS256,
			TokenKey: serverJwtKey,
			TokenExp: jwt.TWO_WEEK,
		}

		subject := &jwt.Subject{}
		token := subject.Create(fmt.Sprintf("user_%d", authCallCount)).Dev("APP").Generate(jwtConfig)

		// 生成32字节的密钥作为secret
		keyBytes := make([]byte, 32)
		for i := range keyBytes {
			keyBytes[i] = byte(65 + i%26)
		}
		secret := utils.Base64EncodeWithPool(keyBytes)

		return &AuthResponse{
			Token:   token,
			Secret:  secret,
			Expired: utils.UnixSecond() + jwt.TWO_WEEK,
		}, nil
	}

	// 启动测试服务器
	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true})

	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: serverJwtKey,
		TokenExp: jwt.TWO_WEEK,
	})

	cipher, _ := crypto.CreateEd25519WithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	serverAddr := "localhost:8089"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(200 * time.Millisecond)

	defer func() {
		fmt.Println("正在停止测试服务器...")
		if err := server.StopWebsocket(); err != nil {
			t.Logf("Server stop failed: %v", err)
		}
		select {
		case <-serverDoneCh:
			fmt.Println("测试服务器已停止")
		case <-time.After(5 * time.Second):
			t.Logf("服务器停止超时")
		}
	}()

	select {
	case <-serverDoneCh:
		t.Fatalf("Server failed to start")
	default:
	}

	// 创建SDK实例
	wsSdk := sdk.NewSocketSDK(serverAddr)

	// 确保Ed25519密钥设置正确
	if err := wsSdk.SetEd25519Object(1, clientPrk, serverPub); err != nil {
		t.Fatalf("Failed to set Ed25519 object: %v", err)
	}

	// 1. 设置初始认证信息（即将过期）
	initialAuth := sdk.AuthToken{
		Token:   "expired_token", // 使用无效token
		Secret:  "expired_secret",
		Expired: utils.UnixSecond() - 100, // 已经过期
	}
	wsSdk.AuthToken(initialAuth)

	// 2. 设置Token过期回调
	tokenRefreshCount := 0
	wsSdk.SetTokenExpiredCallback(func() {
		tokenRefreshCount++
		if zlog.IsDebug() {
			zlog.Debug(fmt.Sprintf("Token expired callback triggered (refresh %d)", tokenRefreshCount), 0)
		}

		// 调用外部认证接口获取新的token
		authResp, err := externalAuthFunc()
		if err != nil {
			zlog.Error("Failed to refresh token from external auth", 0, zlog.AddError(err))
			return
		}

		// 更新SDK的认证信息
		newAuth := sdk.AuthToken{
			Token:   authResp.Token,
			Secret:  authResp.Secret,
			Expired: authResp.Expired,
		}
		wsSdk.AuthToken(newAuth)

		if zlog.IsDebug() {
			zlog.Debug("Token refreshed successfully", 0)
		}

		// 重置token过期标志，允许下次继续触发回调
		// 注意：这是一个内部字段，在实际使用中可能需要SDK提供公共方法
		// wsSdk.tokenExpiredCalled = false
	})

	// 3. 尝试连接（应该触发token过期回调）
	err = wsSdk.ConnectWebSocket()
	if err != nil {
		// 预期的错误，因为初始token已过期
		if !strings.Contains(err.Error(), "token empty or token expired") {
			t.Fatalf("Unexpected connection error: %v", err)
		}
	}

	// 等待回调执行
	time.Sleep(500 * time.Millisecond)

	// 4. 验证回调被触发
	if tokenRefreshCount != 1 {
		t.Errorf("Expected token refresh callback to be called once, got %d", tokenRefreshCount)
	}

	// 5. 验证外部认证接口被调用
	if authCallCount != 1 {
		t.Errorf("Expected external auth to be called once, got %d", authCallCount)
	}

	// 6. 再次尝试连接（应该成功，因为token已刷新）
	err = wsSdk.ConnectWebSocket()
	if err != nil {
		t.Fatalf("Failed to connect after token refresh: %v", err)
	}

	// 7. 验证连接成功
	if !wsSdk.IsWebSocketConnected() {
		t.Error("WebSocket should be connected after token refresh")
	}

	// 8. 测试发送消息
	response := &node.JsonResp{}
	err = wsSdk.SendWebSocketMessage("/ws/test", map[string]interface{}{"test": "data"}, response, true, true, 5)
	if err != nil {
		t.Fatalf("Failed to send message after token refresh: %v", err)
	}

	wsSdk.DisconnectWebSocket()

	t.Logf("✅ Token expired callback test completed successfully")
	t.Logf("   - Callback triggered: %d times", tokenRefreshCount)
	t.Logf("   - External auth called: %d times", authCallCount)
}

// TestWebSocketMessageSubscription 测试消息订阅功能（单个客户端）
func TestWebSocketMessageSubscription(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket subscription test in short mode")
	}

	// 1. 启动测试服务器
	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true})

	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的Ed25519
	cipher, _ := crypto.CreateEd25519WithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 配置连接池
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 添加推送触发路由处理器（一次性推送10条消息）
	err = server.AddRouter("/ws/trigger-push", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		// 解析触发请求
		var triggerData map[string]interface{}
		if err := utils.JsonUnmarshal(body, &triggerData); err != nil {
			return nil, fmt.Errorf("invalid trigger data: %v", err)
		}

		// 获取目标路由
		targetRouter, ok := triggerData["target_router"].(string)
		if !ok || targetRouter == "" {
			return nil, fmt.Errorf("missing target_router")
		}

		// 获取消息内容前缀
		baseMessage, _ := triggerData["message"].(string)
		if baseMessage == "" {
			baseMessage = "Test push message"
		}

		// 持续推送10条消息
		go func() {
			time.Sleep(200 * time.Millisecond) // 确保响应先发送

			for i := 1; i <= 10; i++ {
				// 构造第i条推送消息
				pushMessage := &node.JsonResp{
					Code:    200,
					Message: fmt.Sprintf("push notification #%d", i),
					Data:    fmt.Sprintf("%s #%d", baseMessage, i),
					Router:  targetRouter,
					Time:    utils.UnixSecond(),
					Plan:    0,
				}

				// 广播消息给所有连接的客户端（subject=""）
				if err := server.GetConnManager().SendToSubject("", targetRouter, map[string]interface{}{
					"sequence": i,
					"message":  pushMessage.Message,
					"data":     pushMessage.Data,
				}); err != nil {
					zlog.Error("failed to push message", 0, zlog.AddError(err))
					continue
				}

				if zlog.IsDebug() {
					zlog.Debug("sent push message", 0,
						zlog.String("router", targetRouter),
						zlog.Int("sequence", i),
						zlog.String("data", pushMessage.Data))
				}

				// 消息间隔500ms
				if i < 10 {
					time.Sleep(500 * time.Millisecond)
				}
			}

			zlog.Info("completed sending 10 push messages", 0,
				zlog.String("target_router", targetRouter))
		}()

		return map[string]interface{}{
			"status":         "pushing_started",
			"target_router":  targetRouter,
			"total_messages": 10,
			"interval_ms":    500,
		}, nil
	}, &node.RouterConfig{})

	// 添加持续推送路由处理器（持续推送消息直到客户端断开）
	err = server.AddRouter("/ws/start-continuous-push", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		// 解析请求
		var pushData map[string]interface{}
		if err := utils.JsonUnmarshal(body, &pushData); err != nil {
			return nil, fmt.Errorf("invalid push data: %v", err)
		}

		// 获取目标路由
		targetRouter, ok := pushData["target_router"].(string)
		if !ok || targetRouter == "" {
			return nil, fmt.Errorf("missing target_router")
		}

		// 获取推送间隔（秒）
		intervalSeconds, _ := pushData["interval_seconds"].(float64)
		if intervalSeconds <= 0 {
			intervalSeconds = 2 // 默认2秒间隔
		}
		interval := time.Duration(intervalSeconds) * time.Second

		// 获取消息内容前缀
		baseMessage, _ := pushData["message"].(string)
		if baseMessage == "" {
			baseMessage = "Continuous push message"
		}

		// 获取持续时间（秒），默认60秒
		durationSeconds, _ := pushData["duration_seconds"].(float64)
		if durationSeconds <= 0 {
			durationSeconds = 60
		}

		// 启动持续推送goroutine
		go func() {
			messageCount := 0
			startTime := time.Now()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			zlog.Info("started continuous push", 0,
				zlog.String("target_router", targetRouter),
				zlog.Float64("interval_seconds", intervalSeconds),
				zlog.Float64("duration_seconds", durationSeconds),
				zlog.String("client", connCtx.GetUserIDString()))

			for {
				select {
				case <-ctx.Done():
					// 连接断开，停止推送
					zlog.Info("continuous push stopped due to connection close", 0,
						zlog.String("target_router", targetRouter),
						zlog.Int("total_messages", messageCount))
					return

				case <-ticker.C:
					// 检查是否超过持续时间
					if time.Since(startTime).Seconds() >= durationSeconds {
						zlog.Info("continuous push completed", 0,
							zlog.String("target_router", targetRouter),
							zlog.Int("total_messages", messageCount))
						return
					}

					messageCount++
					currentTime := utils.UnixSecond()

					// 构造推送消息
					pushMessage := &node.JsonResp{
						Code:    200,
						Message: fmt.Sprintf("continuous push #%d", messageCount),
						Data:    fmt.Sprintf("%s #%d at %d", baseMessage, messageCount, currentTime),
						Router:  targetRouter,
						Time:    currentTime,
						Plan:    0,
					}

					// 广播消息给所有连接的客户端（subject=""）
					if err := server.GetConnManager().SendToSubject("", targetRouter, map[string]interface{}{
						"sequence": messageCount,
						"message":  pushMessage.Message,
						"data":     pushMessage.Data,
						"time":     currentTime,
					}); err != nil {
						zlog.Error("failed to push continuous message", 0, zlog.AddError(err))
						continue
					}

					if zlog.IsDebug() {
						zlog.Debug("sent continuous push message", 0,
							zlog.String("router", targetRouter),
							zlog.Int("sequence", messageCount),
							zlog.String("data", pushMessage.Data))
					}
				}
			}
		}()

		return map[string]interface{}{
			"status":             "continuous_pushing_started",
			"target_router":      targetRouter,
			"interval_seconds":   intervalSeconds,
			"duration_seconds":   durationSeconds,
			"estimated_messages": int(durationSeconds / intervalSeconds),
		}, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add trigger-push router: %v", err)
	}

	// 在goroutine中启动服务器
	serverAddr := "localhost:8089"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 使用 defer 确保服务器被停止
	defer func() {
		fmt.Println("正在停止测试服务器...")
		if err := server.StopWebsocket(); err != nil {
			t.Logf("Server stop failed: %v", err)
		}
		select {
		case <-serverDoneCh:
			fmt.Println("测试服务器已停止")
		case <-time.After(5 * time.Second):
			t.Logf("服务器停止超时")
		}
	}()

	// 检查服务器是否成功启动
	select {
	case <-serverDoneCh:
		t.Fatalf("Server failed to start")
	default:
		// 服务器成功启动，继续测试
	}

	// 2. 创建SDK实例并连接到测试服务器
	wsSdk := sdk.NewSocketSDK(serverAddr)

	handler := &testMessageHandler{
		receivedMessages: make([]*node.JsonResp, 0),
	}

	// 测试订阅消息
	t.Run("SubscribeMessage", func(t *testing.T) {
		subscriptionID, err := wsSdk.SubscribeMessage("/ws/test", handler)
		if err != nil {
			t.Fatalf("Failed to subscribe message: %v", err)
		}

		if subscriptionID == "" {
			t.Error("Subscription ID should not be empty")
		}

		// 验证订阅是否成功
		subscriptions := wsSdk.GetSubscriptions()
		if len(subscriptions) != 1 {
			t.Errorf("Expected 1 subscription, got %d", len(subscriptions))
		}

		if sub, exists := subscriptions["/ws/test"]; !exists {
			t.Error("Subscription for /ws/test should exist")
		} else {
			if sub.ID != subscriptionID {
				t.Errorf("Subscription ID mismatch: expected %s, got %s", subscriptionID, sub.ID)
			}
			if sub.Router != "/ws/test" {
				t.Errorf("Subscription router mismatch: expected /ws/test, got %s", sub.Router)
			}
		}
	})

	// 测试消息分发（单个客户端）
	t.Run("MessageDispatch", func(t *testing.T) {
		// 创建消息处理器用于接收推送消息
		dispatchHandler := &testMessageHandler{
			receivedMessages: make([]*node.JsonResp, 0),
		}

		// 订阅推送消息
		_, err := wsSdk.SubscribeMessage("/ws/push", dispatchHandler)
		if err != nil {
			t.Fatalf("Failed to subscribe to push messages: %v", err)
		}
		defer wsSdk.UnsubscribeMessage("/ws/push")

		// 使用预定义的认证参数
		authToken := sdk.AuthToken{
			Token:   access_token,
			Secret:  token_secret,
			Expired: token_expire,
		}
		wsSdk.AuthToken(authToken)

		// 连接到服务器
		err = wsSdk.ConnectWebSocket()
		if err != nil {
			t.Fatalf("Failed to connect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待连接建立
		time.Sleep(200 * time.Millisecond)

		// 通过同一个客户端发送触发推送的请求给自己
		testData := map[string]interface{}{
			"action":        "trigger_push",
			"target_router": "/ws/push",
			"message":       "Hello from push test!",
		}

		response := &node.JsonResp{}
		err = wsSdk.SendWebSocketMessage("/ws/trigger-push", testData, response, true, true, 5)
		if err != nil {
			t.Fatalf("Push trigger failed: %v", err)
		}

		// 验证触发响应
		if response.Message != "pushing_started" {
			t.Logf("Trigger response: %s", response.Message)
		}

		// 等待足够的时间来接收所有10条消息 (10条消息 + 9个500ms间隔 = 约6秒)
		time.Sleep(7 * time.Second)

		// 验证是否接收到10条推送消息
		dispatchHandler.mu.Lock()
		messageCount := len(dispatchHandler.receivedMessages)
		dispatchHandler.mu.Unlock()

		if messageCount != 10 {
			t.Errorf("Expected 10 push messages, got %d", messageCount)
		} else {
			t.Logf("Successfully received all 10 push messages")
		}

		// 验证消息内容和顺序
		dispatchHandler.mu.Lock()
		for i, msg := range dispatchHandler.receivedMessages {
			expectedSeq := i + 1
			if msg.Router != "/ws/push" {
				t.Errorf("Message %d: expected router /ws/push, got %s", expectedSeq, msg.Router)
			}

			expectedData := fmt.Sprintf("Hello from push test! #%d", expectedSeq)
			if msg.Data != expectedData {
				t.Errorf("Message %d: expected data %s, got %s", expectedSeq, expectedData, msg.Data)
			}

			expectedMessage := fmt.Sprintf("push notification #%d", expectedSeq)
			if msg.Message != expectedMessage {
				t.Errorf("Message %d: expected message %s, got %s", expectedSeq, expectedMessage, msg.Message)
			}

			t.Logf("✓ Received push message %d: %s", expectedSeq, msg.Data)
		}
		dispatchHandler.mu.Unlock()
	})

	// 测试持续消息推送（客户端连接后持续接收消息）
	t.Run("ContinuousMessagePush", func(t *testing.T) {
		// 创建消息处理器用于接收持续推送消息
		continuousHandler := &testMessageHandler{
			receivedMessages: make([]*node.JsonResp, 0),
		}

		// 订阅持续推送消息
		_, err := wsSdk.SubscribeMessage("/ws/continuous", continuousHandler)
		if err != nil {
			t.Fatalf("Failed to subscribe to continuous messages: %v", err)
		}
		defer wsSdk.UnsubscribeMessage("/ws/continuous")

		// 使用预定义的认证参数
		authToken := sdk.AuthToken{
			Token:   access_token,
			Secret:  token_secret,
			Expired: token_expire,
		}
		wsSdk.AuthToken(authToken)

		// 连接到服务器
		err = wsSdk.ConnectWebSocket()
		if err != nil {
			t.Fatalf("Failed to connect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待连接建立
		time.Sleep(200 * time.Millisecond)

		// 发送启动持续推送的请求
		continuousData := map[string]interface{}{
			"action":           "start_continuous_push",
			"target_router":    "/ws/continuous",
			"message":          "Continuous test message",
			"interval_seconds": 1.0, // 每1秒推送一条消息
			"duration_seconds": 5.0, // 持续5秒
		}

		response := &node.JsonResp{}
		err = wsSdk.SendWebSocketMessage("/ws/start-continuous-push", continuousData, response, true, true, 5)
		if err != nil {
			t.Fatalf("Failed to start continuous push: %v", err)
		}

		// 验证启动响应
		if response.Message != "success" {
			t.Logf("Continuous push start response: %s", response.Message)
		}

		// 等待持续推送完成（5秒 + 1秒缓冲）
		time.Sleep(7 * time.Second)

		// 验证接收到的消息数量（大约5条消息，间隔1秒）
		continuousHandler.mu.Lock()
		continuousMessageCount := len(continuousHandler.receivedMessages)
		continuousHandler.mu.Unlock()

		if continuousMessageCount < 4 || continuousMessageCount > 6 {
			t.Errorf("Expected 4-6 continuous messages, got %d", continuousMessageCount)
		} else {
			t.Logf("Successfully received %d continuous messages", continuousMessageCount)
		}

		// 验证消息内容和时序
		continuousHandler.mu.Lock()
		for i, msg := range continuousHandler.receivedMessages {
			if msg.Router != "/ws/continuous" {
				t.Errorf("Continuous message %d: expected router /ws/continuous, got %s", i+1, msg.Router)
			}

			expectedPrefix := "Continuous test message #"
			if !strings.HasPrefix(msg.Data, expectedPrefix) {
				t.Errorf("Continuous message %d: expected data to start with '%s', got %s", i+1, expectedPrefix, msg.Data)
			}

			if i > 0 {
				// 检查时间戳是否递增（每秒一条消息）
				prevTime := continuousHandler.receivedMessages[i-1].Time
				currTime := msg.Time
				timeDiff := currTime - prevTime
				if timeDiff < 0 || timeDiff > 2 { // 允许1秒误差
					t.Errorf("Continuous message %d: unexpected time difference %d seconds", i+1, timeDiff)
				}
			}

			t.Logf("✓ Received continuous message %d: %s", i+1, msg.Data)
		}
		continuousHandler.mu.Unlock()

		// 等待一段时间确保推送已停止
		time.Sleep(2 * time.Second)
	})

	// 测试重连后自动重新订阅
	t.Run("ReconnectAutoResubscribe", func(t *testing.T) {
		// 创建消息处理器用于测试重连重新订阅
		reconnectHandler := &testMessageHandler{
			receivedMessages: make([]*node.JsonResp, 0),
		}

		// 订阅测试路由
		testRouter := "/ws/reconnect-test"
		_, err := wsSdk.SubscribeMessage(testRouter, reconnectHandler)
		if err != nil {
			t.Fatalf("Failed to subscribe to reconnect test: %v", err)
		}
		defer wsSdk.UnsubscribeMessage(testRouter)

		// 连接到服务器
		err = wsSdk.ConnectWebSocket()
		if err != nil {
			t.Fatalf("Failed to connect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待连接建立
		time.Sleep(200 * time.Millisecond)

		// 断开连接
		wsSdk.DisconnectWebSocket()

		// 等待断开完成
		time.Sleep(100 * time.Millisecond)

		// 验证连接已断开
		if wsSdk.IsWebSocketConnected() {
			t.Error("WebSocket should be disconnected")
		}

		// 重新设置认证信息（模拟重连时的token更新）
		authToken := sdk.AuthToken{
			Token:   access_token,
			Secret:  token_secret,
			Expired: token_expire,
		}
		wsSdk.AuthToken(authToken)

		// 重新连接（这会触发自动重新订阅）
		err = wsSdk.ConnectWebSocket()
		if err != nil {
			t.Fatalf("Failed to reconnect WebSocket: %v", err)
		}
		defer wsSdk.DisconnectWebSocket()

		// 等待重连和重新订阅完成
		time.Sleep(500 * time.Millisecond)

		// 验证重新连接成功
		if !wsSdk.IsWebSocketConnected() {
			t.Error("WebSocket should be reconnected")
		}

		// 验证订阅仍然存在
		subscriptions := wsSdk.GetSubscriptions()
		if len(subscriptions) != 1 {
			t.Errorf("Expected 1 subscription after reconnect, got %d", len(subscriptions))
		}

		if _, exists := subscriptions[testRouter]; !exists {
			t.Errorf("Subscription for %s should still exist after reconnect", testRouter)
		}

		t.Logf("✓ Reconnect auto-resubscribe test completed successfully")
	})
}

// TestWebSocketMessageSizeLimit 测试消息大小限制
func TestWebSocketMessageSizeLimit(t *testing.T) {
	server := node.NewWsServer(node.SubjectDeviceUnique)
	serverAddr := "localhost:8089"

	access_token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTkyODAwOTk4Mzg4NjYyMjczIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiMjgyZjAwMmQtNTY3MS00YTlhLTgwMDMtMzA5ZmI0ZGNkNTZjIiwiZXh0IjoiIiwiaWF0IjowLCJleHAiOjE3NjUxNjUzNTd9.tbuDc+g0Scge9WNRDESF/acdMG7Fqwgu6F4vWgv69WQ="
	token_secret := "nt/YcHhS6Y8npXInAhBr9PMdSNLZlGbNCfnqaQWo09HNd67Swoy0qHZeVqN2A42g/SHVoTWkLs3XQna8bEUxeA=="
	token_expire := int64(1765165357)

	server.AddRouter("/ws/user", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return map[string]interface{}{"message": "success"}, nil
	}, &node.RouterConfig{})

	// 启动服务器
	go func() {
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()
	defer server.StopWebsocket()
	time.Sleep(100 * time.Millisecond)

	// 初始化SDK
	wsSdk := sdk.NewSocketSDK(serverAddr)

	// 设置认证Token
	authToken := sdk.AuthToken{
		Token:   access_token,
		Secret:  token_secret,
		Expired: token_expire,
	}
	wsSdk.AuthToken(authToken)

	// 连接WebSocket
	err := wsSdk.ConnectWebSocket()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer wsSdk.DisconnectWebSocket()

	// 创建超过1MB的消息（大约1.1MB）
	largeMessage := make([]byte, 1024*1024+100*1024) // 1.1MB
	for i := range largeMessage {
		largeMessage[i] = byte(i % 256)
	}

	requestObject := map[string]interface{}{
		"data": string(largeMessage), // 将大字节数组转换为字符串
	}
	responseObject := &sdk.AuthToken{}

	// 发送大消息，预期会失败
	err = wsSdk.SendWebSocketMessage("/ws/user", requestObject, responseObject, true, true, 5)
	if err == nil {
		t.Error("Expected message size limit error, but got success")
	} else {
		t.Logf("✓ Message size limit correctly rejected large message: %v", err)
	}
}

// TestWebSocketGracefulShutdownWithTimeout 测试带超时的优雅关闭
func TestWebSocketGracefulShutdownWithTimeout(t *testing.T) {
	server := node.NewWsServer(node.SubjectDeviceUnique)
	serverAddr := "localhost:8090"

	// 初始化连接池和心跳服务
	if err := server.NewPool(100, 10, 5, 30); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}

	server.AddRouter("/ws/test", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return map[string]interface{}{"message": "success"}, nil
	}, &node.RouterConfig{})

	// 启动服务器
	go func() {
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 使用带超时的优雅关闭
	err := server.StopWebsocketWithTimeout(3 * time.Second)
	if err != nil {
		t.Errorf("StopWebsocketWithTimeout failed: %v", err)
	}

	t.Logf("✓ Graceful shutdown with timeout completed successfully")
}

// TestWebSocketClientUnexpectedDisconnect 测试客户端意外断开（如网络断开、进程被杀）时服务端能正确清理连接
func TestWebSocketClientUnexpectedDisconnect(t *testing.T) {
	server := node.NewWsServer(node.SubjectDeviceUnique)
	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})
	cipher, _ := crypto.CreateEd25519WithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)
	if err := server.NewPool(10, 20, 100, 15); err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	serverAddr := "localhost:8093"
	serverDoneCh := make(chan bool, 1)
	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()
	time.Sleep(200 * time.Millisecond)

	defer func() {
		_ = server.StopWebsocket()
		select {
		case <-serverDoneCh:
		case <-time.After(5 * time.Second):
		}
	}()

	config := jwt.JwtConfig{TokenTyp: jwt.JWT, TokenAlg: jwt.HS256, TokenKey: "123456", TokenExp: jwt.TWO_WEEK}
	subject := &jwt.Subject{}
	token := subject.Create(utils.NextSID()).Dev("APP").Generate(config)

	if err := rawWsHandshakeThenClose(serverAddr, token); err != nil {
		t.Fatalf("raw handshake then close failed: %v", err)
	}

	// 等待服务端 read loop 收到 EOF 并执行 cleanup（RemoveByConn）
	time.Sleep(600 * time.Millisecond)
	if count := server.GetConnManager().Count(); count != 0 {
		t.Errorf("expected 0 connections after client unexpected disconnect, got %d", count)
	} else {
		t.Logf("✓ Server correctly cleaned up after client unexpected disconnect")
	}

	// 再次用 SDK 连接，确认服务端仍可用（需与 TestWebSocketSDKUsage 一致配置 Ed25519）
	wsSdk := sdk.NewSocketSDK(serverAddr)
	secretBytes := subject.GetTokenSecret(token, config.TokenKey)
	wsSdk.AuthToken(sdk.AuthToken{
		Token:   token,
		Secret:  utils.Base64Encode(utils.Bytes2Str(secretBytes)),
		Expired: subject.Payload.Exp,
	})
	wsSdk.SetClientNo(1)
	_ = wsSdk.SetEd25519Object(wsSdk.ClientNo, clientPrk, serverPub)
	if err := wsSdk.ConnectWebSocket(); err != nil {
		t.Fatalf("reconnect after disconnect test failed: %v", err)
	}
	defer wsSdk.DisconnectWebSocket()
	t.Logf("✓ Client can reconnect and server still works after unexpected disconnect")
}

// TestRemoteIPSecurity 测试RemoteIP的安全性，防止IP伪造
func TestRemoteIPSecurity(t *testing.T) {
	// 创建一个模拟的Context
	ctx := &node.Context{}
	ctx.RequestCtx = &fasthttp.RequestCtx{}
	ctx.RequestCtx.Request.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1, 203.0.113.1")
	ctx.RequestCtx.Request.Header.Set("X-Real-Ip", "10.0.0.2")

	// 测试X-Forwarded-For优先级（取第一个有效IP）
	ip := ctx.RemoteIP()
	if ip != "192.168.1.100" {
		t.Errorf("Expected first IP from X-Forwarded-For, got %s", ip)
	}

	// 测试无效IP的情况
	ctx.RequestCtx.Request.Header.Set("X-Forwarded-For", "invalid-ip, 192.168.1.101")
	ip = ctx.RemoteIP()
	if ip != "192.168.1.101" {
		t.Errorf("Expected valid IP after invalid one, got %s", ip)
	}

	// 测试X-Real-Ip回退
	ctx.RequestCtx.Request.Header.Del("X-Forwarded-For")
	ip = ctx.RemoteIP()
	if ip != "10.0.0.2" {
		t.Errorf("Expected X-Real-Ip fallback, got %s", ip)
	}

	// 测试完全无效的情况（应该回退到RemoteIP()）
	ctx.RequestCtx.Request.Header.Del("X-Real-Ip")
	// 这里我们无法直接设置RemoteIP()的返回值，所以只验证方法不panic
	ip = ctx.RemoteIP()
	if ip == "" {
		t.Error("RemoteIP should not return empty string")
	}

	t.Logf("✓ RemoteIP security test completed - IP spoofing protection working")
}

// TestDevConnConcurrentSafety 测试 DevConn 的并发安全性（Send / UpdateLast / LastSeen）
func TestDevConnConcurrentSafety(t *testing.T) {
	devConn := &node.DevConn{
		Sub:  "test_subject",
		Dev:  "test_device",
		Last: utils.UnixSecond(),
		Conn: nil,
	}

	const numGoroutines = 10
	const numCalls = 100
	done := make(chan bool, numGoroutines)
	errorChan := make(chan error, numGoroutines*numCalls)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < numCalls; j++ {
				// Send(Conn==nil) 应安全返回错误
				if err := devConn.Send([]byte("x")); err == nil {
					errorChan <- fmt.Errorf("goroutine %d call %d: expected Send to fail when Conn is nil", id, j)
					return
				}
				devConn.UpdateLast()
				_ = devConn.LastSeen()
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
	close(errorChan)
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		t.Errorf("Concurrent DevConn ops failed with %d errors:", len(errors))
		for i, err := range errors {
			t.Errorf("  Error %d: %v", i+1, err)
		}
	} else {
		t.Logf("✓ %d goroutines with %d calls each completed without race conditions", numGoroutines, numCalls)
	}
}

// TestWebSocketErrorHandling 测试错误处理的上下文信息记录
func TestWebSocketErrorHandling(t *testing.T) {
	server := node.NewWsServer(node.SubjectDeviceUnique)
	serverAddr := "localhost:8092"

	// 添加JWT配置
	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的Ed25519
	cipher, _ := crypto.CreateEd25519WithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 初始化连接池和心跳服务
	if err := server.NewPool(100, 10, 5, 30); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}

	server.AddRouter("/ws/test", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return map[string]interface{}{"message": "success"}, nil
	}, &node.RouterConfig{})

	// 添加一个会失败的路由来触发错误处理
	server.AddRouter("/ws/error", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		return nil, fmt.Errorf("test error for error handling")
	}, &node.RouterConfig{})

	// 启动服务器
	go func() {
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Failed to start server: %v", err)
		}
	}()
	defer server.StopWebsocket()
	time.Sleep(200 * time.Millisecond)

	// 初始化SDK并建立连接
	wsSdk := sdk.NewSocketSDK(serverAddr)
	authToken := sdk.AuthToken{
		Token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxOTkyODAwOTk4Mzg4NjYyMjczIiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiMjgyZjAwMmQtNTY3MS00YTlhLTgwMDMtMzA5ZmI0ZGNkNTZjIiwiZXh0IjoiIiwiaWF0IjowLCJleHAiOjE3NjUxNjUzNTd9.tbuDc+g0Scge9WNRDESF/acdMG7Fqwgu6F4vWgv69WQ=",
		Secret:  "nt/YcHhS6Y8npXInAhBr9PMdSNLZlGbNCfnqaQWo09HNd67Swoy0qHZeVqN2A42g/SHVoTWkLs3XQna8bEUxeA==",
		Expired: int64(1765165357),
	}
	wsSdk.AuthToken(authToken)

	err := wsSdk.ConnectWebSocket()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer wsSdk.DisconnectWebSocket()

	// 发送请求到会失败的路由，触发错误处理
	requestObject := map[string]interface{}{"test": "error"}
	responseObject := &sdk.AuthToken{}

	// 发送到错误路由，应该会记录详细的错误日志
	err = wsSdk.SendWebSocketMessage("/ws/error", requestObject, responseObject, true, true, 5)
	if err == nil {
		t.Error("Expected error from /ws/error route, but got success")
	}

	t.Logf("✓ Error handling test completed - check logs for detailed context information")
}

func TestWebSocketServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket SDK usage test in short mode")
	}

	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.DEBUG, Console: true}) // 测试环境使用空logger，避免输出干扰

	fmt.Println("=== WebSocket SDK 完整使用流程测试 ===")

	// 0. 启动测试服务器
	fmt.Println("0. 启动测试服务器...")

	// 创建WebSocket服务器实例
	server := node.NewWsServer(node.SubjectDeviceUnique)

	server.AddJwtConfig(jwt.JwtConfig{
		TokenTyp: jwt.JWT,
		TokenAlg: jwt.HS256,
		TokenKey: "123456",
		TokenExp: jwt.TWO_WEEK,
	})

	// 增加双向验签的Ed25519
	cipher, _ := crypto.CreateEd25519WithBase64(serverPrk, clientPub)
	server.AddCipher(1, cipher)

	// 配置连接池
	err := server.NewPool(100, 10, 5, 30)
	if err != nil {
		t.Fatalf("Failed to initialize connection pool: %v", err)
	}

	// 添加业务路由处理器
	err = server.AddRouter("/ws/user", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		ret := &sdk.AuthToken{
			Token:  "鲨鱼宝宝获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	err = server.AddRouter("/ws/user2", func(ctx context.Context, connCtx *node.ConnectionContext, body []byte) (interface{}, error) {
		ret := &sdk.AuthToken{
			Token:  "鲨鱼爸爸获取websocket",
			Secret: connCtx.GetUserIDString(),
		}
		return ret, nil
	}, &node.RouterConfig{})
	if err != nil {
		t.Fatalf("Failed to add router: %v", err)
	}

	// 在goroutine中启动服务器
	serverAddr := "localhost:8088"
	serverDoneCh := make(chan bool, 1)

	go func() {
		defer func() { serverDoneCh <- true }()
		if err := server.StartWebsocket(serverAddr); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	go func() {
		for {
			// 2019955939305586689
			_ = server.GetConnectionManager().SendToSubject("2019955939305586689", "test push", map[string]string{"push data": "hello tony!"})
			time.Sleep(3 * time.Second)
		}
	}()

	select {}

}

func TestWebSocketClient(t *testing.T) {
	access_token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIyMDE5OTU1OTM5MzA1NTg2Njg5IiwiYXVkIjoiIiwiaXNzIjoiIiwiZGV2IjoiQVBQIiwianRpIjoiZDRiOTJhNjhmODU3NGY4NTg4NjljNDNlYTk4YzZlNDAiLCJleHQiOiIiLCJpYXQiOjAsImV4cCI6MTc4MjUyNTk5OX0=.8hUWn5+sEkbabRV1rDTqFLbBIMcxQ0WplRqlz0MJKRc="
	token_secret := "P2wvYCoyzsFI97gJj10tNofO2YFYmK9jmFPrkiZ4qhowL4OefGgdgzIgVM0anz1KdY8KaqASeTZysYAC21AZ6Q=="
	token_expire := int64(1782525999)

	// 1. 初始化SDK
	fmt.Println("1. 初始化SDK...")
	wsSdk := sdk.NewSocketSDK("localhost:8088")

	// 2. 设置认证Token
	fmt.Println("2. 设置认证Token...")
	authToken := sdk.AuthToken{
		Token:   access_token,
		Secret:  token_secret,
		Expired: token_expire,
	}
	wsSdk.AuthToken(authToken)

	wsSdk.SetClientNo(1)
	wsSdk.SetEd25519Object(wsSdk.ClientNo, clientPrk, serverPub)
	wsSdk.SetHealthPing(3) // 3秒心跳间隔，便于测试

	// 设置推送消息回调 - 客户端通过code=300识别推送消息，已自动处理验签和解密
	wsSdk.SetPushMessageCallback(func(router string, data []byte) {
		fmt.Printf("📨 收到推送消息 - Router: %s\n", router)
		fmt.Printf("📦 推送数据: %s\n", string(data))

		// 示例：解析推送数据为结构化对象
		var pushData map[string]interface{}
		if err := utils.JsonUnmarshal(data, &pushData); err != nil {
			fmt.Printf("❌ 解析推送数据失败: %v\n", err)
			return
		}

		// 处理不同类型的推送消息
		switch router {
		case "/push/notification":
			fmt.Printf("🔔 收到通知推送: %v\n", pushData)
			// 处理通知逻辑...

		case "/push/user/status":
			fmt.Printf("👤 用户状态更新: %v\n", pushData)
			// 处理用户状态逻辑...

		case "/push/system/alert":
			fmt.Printf("🚨 系统告警: %v\n", pushData)
			// 处理系统告警逻辑...

		default:
			fmt.Printf("📬 收到未知类型推送: %s\n", router)
			fmt.Printf("📋 数据内容: %v\n", pushData)
		}
	})

	// 4. 启用自动重连
	fmt.Println("4. 启用自动重连...")
	wsSdk.EnableReconnect() // 启用重连，默认无限次，初始间隔1秒，最大间隔8秒

	// 5. 尝试连接WebSocket（预期成功，因为服务器已启动）
	fmt.Println("5. 尝试连接WebSocket（预期成功）...")
	_ = wsSdk.ConnectWebSocket()

	// 6. 发送WebSocket消息
	fmt.Println("6. 发送WebSocket消息...")
	requestObject := map[string]interface{}{"test": "张三"}
	responseObject := &sdk.AuthToken{}
	err := wsSdk.SendWebSocketMessage("/ws/user", requestObject, responseObject, true, false, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("明文响应结果1:", responseObject)

	requestObject = map[string]interface{}{"test": "张三"}
	responseObject = &sdk.AuthToken{}
	err = wsSdk.SendWebSocketMessage("/ws/user2", requestObject, responseObject, true, true, 5)
	if err != nil {
		t.Errorf("发送消息失败：%v", err)
		// 打印详细错误信息
		t.Logf("错误详情: %v", err)
		return
	}
	fmt.Println("加密响应结果2:", responseObject)

	select {}
}
