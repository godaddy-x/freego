package grpcgw_example

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cgrpc "github.com/godaddy-x/freego/client/grpcgw"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
)

// Plan0/Plan1：登录一次后 b.ResetTimer，仅并发 getUser。
//
//   go test ./server/grpcgw/example/ -run=^$ -bench='BenchmarkGRPCGW_Plan0|BenchmarkGRPCGW_Plan1' -benchmem -count=3

const benchPlan01User = int64(40001)

func benchGetUserPlan0(cli *cgrpc.Client) error {
	res := &pb.GetUserResponse{}
	return cgrpc.CallPlan0Proto(cli, "getUser", &pb.GetUserRequest{Name: "bench-plan0"}, res, 0)
}

func benchGetUserPlan1(cli *cgrpc.Client) error {
	res := &pb.GetUserResponse{}
	return cgrpc.CallPlan1Proto(cli, "getUser", &pb.GetUserRequest{Name: "bench-plan1"}, res, 0)
}

func TestConcurrentPlan01GetUserSmoke(t *testing.T) {
	if os.Getenv("GRPCGW_LOGIN_BENCH") == "" {
		t.Skip("set GRPCGW_LOGIN_BENCH=1 to run against a live server")
	}
	target := e2eTarget()

	runSmoke := func(name string, call func(*cgrpc.Client) error) {
		cli, err := benchAuthenticatedClient(target, benchPlan01User)
		if err != nil {
			t.Fatalf("%s login: %v", name, err)
		}
		defer cli.Close()

		const workers = 32
		const perWorker = 20
		var ok, fail int64
		start := time.Now()
		done := make(chan struct{}, workers)
		for w := 0; w < workers; w++ {
			go func() {
				for i := 0; i < perWorker; i++ {
					if err := call(cli); err != nil {
						atomic.AddInt64(&fail, 1)
						if fail <= 3 {
							t.Logf("%s fail: %v", name, err)
						}
					} else {
						atomic.AddInt64(&ok, 1)
					}
				}
				done <- struct{}{}
			}()
		}
		for w := 0; w < workers; w++ {
			<-done
		}
		elapsed := time.Since(start)
		t.Logf("%s target=%s ok=%d fail=%d elapsed=%s qps=%.2f",
			name, target, ok, fail, elapsed, float64(ok)/elapsed.Seconds())
		if fail > 0 {
			t.Fatalf("%s: %d calls failed", name, fail)
		}
	}

	runSmoke("plan0", benchGetUserPlan0)
	runSmoke("plan1", benchGetUserPlan1)
}

func benchmarkPlan01GetUser(b *testing.B, plan int) {
	target := e2eTarget()
	cli, err := benchAuthenticatedClient(target, benchPlan01User)
	if err != nil {
		b.Fatalf("login: %v", err)
	}
	defer cli.Close()

	if plan == 0 {
		if err := benchGetUserPlan0(cli); err != nil {
			b.Fatalf("warmup plan0: %v", err)
		}
	} else if err := benchGetUserPlan1(cli); err != nil {
		b.Fatalf("warmup plan1: %v", err)
	}

	reqName := fmt.Sprintf("bench-plan%d", plan)
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		req := &pb.GetUserRequest{Name: reqName}
		for p.Next() {
			res := &pb.GetUserResponse{}
			var err error
			if plan == 0 {
				err = cgrpc.CallPlan0Proto(cli, "getUser", req, res, 0)
			} else {
				err = cgrpc.CallPlan1Proto(cli, "getUser", req, res, 0)
			}
			if err != nil {
				if strings.Contains(err.Error(), "time window") {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				b.Fatalf("getUser plan%d: %v", plan, err)
			}
			if res.Name != reqName || res.Echo != "1" {
				b.Fatalf("unexpected response: %+v", res)
			}
		}
	})
}

func BenchmarkGRPCGW_Plan0GetUser(b *testing.B) { benchmarkPlan01GetUser(b, 0) }
func BenchmarkGRPCGW_Plan1GetUser(b *testing.B) { benchmarkPlan01GetUser(b, 1) }
