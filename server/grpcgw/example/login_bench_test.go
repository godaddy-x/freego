package grpcgw_example

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cgrpc "github.com/godaddy-x/freego/client/grpcgw"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

// 压测前请先启动 example 服务（默认 127.0.0.1:19090）。
// 可选：GRPCGW_E2E_ADDR=127.0.0.1:19090
//
//   go test ./server/grpcgw/example/ -run=^$ -bench=BenchmarkGRPCGW_Plan2Login -benchmem -count=3
//
// Plan2 会话元数据非并发安全，须每 goroutine 独立 Dial（同 http_test Plan2 压测）。

func TestConcurrentPlan2LoginSmoke(t *testing.T) {
	if os.Getenv("GRPCGW_LOGIN_BENCH") == "" {
		t.Skip("set GRPCGW_LOGIN_BENCH=1 to run against a live server")
	}
	target := e2eTarget()
	loginBody := benchLoginBody()
	const workers = 32
	const perWorker = 5

	var ok, fail int64
	start := time.Now()
	done := make(chan struct{}, workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			for i := 0; i < perWorker; i++ {
				usr := int64(20000 + id*1000 + i)
				if err := plan2LoginOnce(target, usr, loginBody); err != nil {
					atomic.AddInt64(&fail, 1)
					t.Logf("login fail usr=%d: %v", usr, err)
				} else {
					atomic.AddInt64(&ok, 1)
				}
			}
			done <- struct{}{}
		}(w)
	}
	for w := 0; w < workers; w++ {
		<-done
	}
	elapsed := time.Since(start)
	t.Logf("target=%s ok=%d fail=%d total=%d elapsed=%s qps=%.2f",
		target, ok, fail, ok+fail, elapsed, float64(ok)/elapsed.Seconds())
	if fail > 0 {
		t.Fatalf("%d logins failed", fail)
	}
}

func BenchmarkGRPCGW_Plan2Login(b *testing.B) {
	target := e2eTarget()
	loginBody := benchLoginBody()
	var seq int64

	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		cipher, err := ExampleClientCipher()
		if err != nil {
			b.Fatal(err)
		}
		cli, err := cgrpc.Dial(target)
		if err != nil {
			b.Fatal(err)
		}
		defer cli.Close()

		local := 0
		for p.Next() {
			local++
			usr := int64(30000 + atomic.AddInt64(&seq, 1))
			plain, err := plan2LoginWithRetry(cli, usr, cipher, loginBody)
			if err != nil {
				if strings.Contains(err.Error(), "time window") {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				b.Logf("Plan2 login failed usr=%d iter=%d: %v", usr, local, err)
				continue
			}
			tok := &pb.AuthToken{}
			if err := pb.UnmarshalProto(plain, tok); err != nil {
				b.Fatal(err)
			}
			if len(tok.Token) == 0 {
				b.Fatal("token empty")
			}
		}
	})
}
