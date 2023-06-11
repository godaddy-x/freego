package main

import (
	"fmt"
	"github.com/godaddy-x/freego/rpcx"
	"github.com/godaddy-x/freego/rpcx/impl"
	"github.com/godaddy-x/freego/rpcx/pb"
	"google.golang.org/grpc"
	"net/http"
	_ "net/http/pprof"
	"testing"
)

func TestConsulxRunGRPCServer(t *testing.T) {
	initConsul()
	go func() {
		_ = http.ListenAndServe(":8848", nil)
	}()
	objects := []*rpcx.GRPC{
		{
			Address: "localhost",
			Service: "PubWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterPubWorkerServer(server, &impl.PubWorker{}) },
		},
	}
	rpcx.RunServer("", false, objects...)
}

func TestConsulxCallGRPC_GenID(t *testing.T) {
	initConsul()
	rpcx.RunClient()
	conn, err := rpcx.NewClientConn(rpcx.GRPC{Service: "PubWorker", Cache: 30})
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
	rpcx.RunClient(appConfig.AppId)
	conn, err := rpcx.NewClientConn(rpcx.GRPC{Service: "PubWorker", Cache: 30})
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
	rpcx.RunClient(appConfig.AppId)
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		conn, err := rpcx.NewClientConn(rpcx.GRPC{Service: "PubWorker", Cache: 30})
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
