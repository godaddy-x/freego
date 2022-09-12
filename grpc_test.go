package main

import (
	"fmt"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/consul/grpcx/impl"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"google.golang.org/grpc"
	"net/http"
	_ "net/http/pprof"
	"testing"
)

func TestConsulxRunGRPCServer(t *testing.T) {
	go func() {
		_ = http.ListenAndServe(":8848", nil)
	}()
	objects := []*grpcx.GRPC{
		{
			Address: "localhost",
			Service: "PubWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterPubWorkerServer(server, &impl.PubWorker{}) },
		},
	}
	grpcx.RunServer("", true, objects...)
}

func TestConsulxCallGRPC_GenID(t *testing.T) {
	grpcx.RunClient(APPID)
	conn, err := grpcx.NewClientConn(grpcx.GRPC{Service: "PubWorker", Cache: 30})
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	res, err := pb.NewPubWorkerClient(conn.Value()).GenerateId(conn.Context(), &pb.GenerateIdReq{})
	if err != nil {
		panic(err)
	}
	fmt.Println("call rpc:", res)
}

func TestGRPCClient(t *testing.T) {
	grpcx.RunClient(APPID)
	conn, err := grpcx.NewClientConn(grpcx.GRPC{Service: "PubWorker", Cache: 30})
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	res, err := pb.NewPubWorkerClient(conn.Value()).GenerateId(conn.Context(), &pb.GenerateIdReq{})
	if err != nil {
		panic(err)
	}
	fmt.Println("call result: ", res)
}

func BenchmarkGRPCClient(b *testing.B) {
	grpcx.RunClient(APPID)
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		conn, err := grpcx.NewClientConn(grpcx.GRPC{Service: "PubWorker", Cache: 30})
		if err != nil {
			fmt.Println(err)
			return
		}
		_, err = pb.NewPubWorkerClient(conn.Value()).GenerateId(conn.Context(), &pb.GenerateIdReq{})
		if err != nil {
			fmt.Println(err)
			return
		}
		conn.Close()
	}
}
