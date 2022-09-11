package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/consul/grpcx/impl"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"github.com/shimingyah/pool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"net"
	"testing"
)

func TestConsulxRunGRPCServer(t *testing.T) {
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
	grpcx.RunClient(grpcx.ClientConfig{Appid: APPID, Timeout: 30, Addrs: []string{"localhost:20998"}})
	conn, err := grpcx.NewClientConn(grpcx.GRPC{Service: "PubWorker"})
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	ctx, cancel := grpcx.NewContext(5000)
	defer cancel()
	res, err := pb.NewPubWorkerClient(conn.Value()).GenerateId(ctx, &pb.GenerateIdReq{})
	if err != nil {
		panic(err)
	}
	fmt.Println("call rpc:", res)
}

func TestGRPCServer(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:8889")

	if err != nil {
		panic(err)
	}

	fmt.Println("listen on 127.0.0.1:8889")
	opts := []grpc.ServerOption{
		grpc.InitialWindowSize(pool.InitialWindowSize),
		grpc.InitialConnWindowSize(pool.InitialConnWindowSize),
		grpc.MaxSendMsgSize(pool.MaxSendMsgSize),
		grpc.MaxRecvMsgSize(pool.MaxRecvMsgSize),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    pool.KeepAliveTime,
			Timeout: pool.KeepAliveTimeout,
		}),
	}
	grpcServer := grpc.NewServer(opts...)

	pb.RegisterPubWorkerServer(grpcServer, &impl.PubWorker{})

	err = grpcServer.Serve(l)

	if err != nil {
		println(err)
	}
}

var conn *grpc.ClientConn

func init() {
	conn, _ = grpc.Dial("localhost:8889", grpc.WithTransportCredentials(insecure.NewCredentials()))
}

const accessToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJlMTBhZGMzOTQ5YmE1OWFiYmU1NmUwNTdmMjBmODgzZSIsImF1ZCI6IiIsImlzcyI6IiIsImlhdCI6MCwiZXhwIjoxNjYyNjIxMTk0LCJkZXYiOiJHUlBDIiwianRpIjoiaTRkS1FaQ25zejJZd0JSbVFkUHRnUT09IiwiZXh0Ijp7fX0=.wqWOml4WecF2f5U1eaxzv/wYmIwFG5JZGk6nVW/sWIQ="

func TestGRPCClient(t *testing.T) {
	fmt.Println(conn.Target())
	md := metadata.New(map[string]string{"token": accessToken})
	ctx := context.Background()
	ctx = metadata.NewOutgoingContext(ctx, md)
	client := pb.NewPubWorkerClient(conn)
	response, err := client.GenerateId(ctx, &pb.GenerateIdReq{})
	if err != nil {
		panic(err)
	}
	fmt.Println(response)
}

func BenchmarkGRPCClient(b *testing.B) {
	b.StopTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ { //use b.N for looping
		ctx := context.Background()
		//md := metadata.New(map[string]string{"token": accessToken})
		//ctx = metadata.NewOutgoingContext(ctx, md)
		client := pb.NewPubWorkerClient(conn)
		_, err := client.GenerateId(ctx, &pb.GenerateIdReq{})
		if err != nil {
			panic(err)
		}
		//fmt.Println(r)
	}
}
