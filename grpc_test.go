package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/consul/grpcx/impl"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"google.golang.org/grpc"
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
	res, err := grpcx.CallRPC(&grpcx.GRPC{
		Service: "PubWorker",
		CallRPC: func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) {
			return pb.NewPubWorkerClient(conn).GenerateId(ctx, &pb.GenerateIdReq{})
		}})
	if err != nil {
		panic(err)
	}
	object, _ := res.(*pb.GenerateIdRes)
	fmt.Println("call rpc:", object)
}
