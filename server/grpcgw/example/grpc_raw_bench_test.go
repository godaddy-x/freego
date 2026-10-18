package grpcgw_example

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// 裸 gRPC unary 基线：标准 Health/Check，无 grpcgw、无业务 handler。
// 用于估算「协议栈 + 本机回环」下限，与 BenchmarkGRPCGW_Plan0GetUser 对比。

func startHealthBenchServer() (addr string, stop func(), err error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	s := grpc.NewServer()
	healthpb.RegisterHealthServer(s, health.NewServer())
	go func() { _ = s.Serve(lis) }()
	return lis.Addr().String(), func() { s.Stop() }, nil
}

func BenchmarkGRPC_RawHealthCheck(b *testing.B) {
	addr, stop, err := startHealthBenchServer()
	if err != nil {
		b.Fatal(err)
	}
	defer stop()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	cli := healthpb.NewHealthClient(conn)
	req := &healthpb.HealthCheckRequest{Service: ""}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			_, err := cli.Check(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
