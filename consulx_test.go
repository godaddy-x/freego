package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/consul/grpc/pb"
	"github.com/godaddy-x/freego/util"
	"google.golang.org/grpc"
	"testing"
)

type IdWorker struct {
	pb.UnimplementedIdWorkerServer
}

func (self *IdWorker) GenerateId(ctx context.Context, req *pb.GenerateIdReq) (*pb.GenerateIdRes, error) {
	return &pb.GenerateIdRes{Value: util.GetSnowFlakeIntID(req.Node)}, nil
}

func TestConsulxRunGRPC(t *testing.T) {
	new(consul.ConsulManager).InitConfig(nil, consul.ConsulConfig{})
	client, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}
	objects := []*consul.GRPC{
		{
			Address: "127.0.0.1",
			Service: "IdWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterIdWorkerServer(server, &IdWorker{}) },
		},
	}
	client.RunGRPC(objects...)
}

func TestConsulxCallGRPC_ID(t *testing.T) {
	new(consul.ConsulManager).InitConfig(nil, consul.ConsulConfig{})
	client, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}
	res, err := client.CallGRPC(&consul.GRPC{Service: "IdWorker", CallRPC: func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) {
		rpc := pb.NewIdWorkerClient(conn)
		return rpc.GenerateId(ctx, &pb.GenerateIdReq{})
	}})
	if err != nil {
		panic(err)
	}
	object, _ := res.(*pb.GenerateIdRes)
	fmt.Println("call rpc:", object)
}
