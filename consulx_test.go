package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/consul/grpcx"
	"github.com/godaddy-x/freego/consul/grpcx/pb"
	"github.com/godaddy-x/freego/util"
	"google.golang.org/grpc"
	"testing"
	"time"
)

func TestConsulxRunGRPCServer(t *testing.T) {
	objects := []*grpcx.GRPC{
		{
			Address: "localhost",
			Service: "PubWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterPubWorkerServer(server, &grpcx.PubWorker{}) },
		},
	}
	grpcx.RunServer("", true, objects...)
}

func TestConsulxCallGRPC_GenID(t *testing.T) {
	token, err := new(grpcx.GRPCManager).CreateTokenAuth(util.MD5("123456"), func(res *pb.RPCLoginRes) error {
		fmt.Println("rpc token:", res.Token, res.Expired)
		return nil
	})
	if err != nil {
		panic(err)
	}
	res, err := grpcx.CallRPC(&grpcx.GRPC{
		Token:   token,
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

func TestConsulxCallGRPC_Login(t *testing.T) {
	token, err := new(grpcx.GRPCManager).CreateTokenAuth(util.MD5("123456"), func(res *pb.RPCLoginRes) error {
		fmt.Println("rpc token:", res.Token, res.Expired)
		return nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("test rpc login: ", token)
	time.Sleep(1 * time.Hour)
}
