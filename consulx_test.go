package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/component/consul/grpcx"
	"github.com/godaddy-x/freego/component/consul/grpcx/pb"
	"github.com/godaddy-x/freego/util"
	"google.golang.org/grpc"
	"testing"
)

const rpcToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJlMTBhZGMzOTQ5YmE1OWFiYmU1NmUwNTdmMjBmODgzZSIsImF1ZCI6IiIsImlzcyI6IiIsImlhdCI6MCwiZXhwIjoxNjY5Nzk1MDYyLCJkZXYiOiIiLCJqdGkiOiJ4TVlDRnc3QjNtUU1vTmREY3pheUJRPT0iLCJleHQiOnt9fQ==.AXLSwotawZvI+lcGGgT0vQS59v9TYRno3EMXSuc8N6o="

func TestConsulxRunGRPCServer(t *testing.T) {
	initConsul()
	objects := []*grpcx.GRPC{
		{
			Address: "localhost",
			Service: "PubWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterPubWorkerServer(server, &grpcx.PubWorker{}) },
		},
	}
	client := &grpcx.GRPCManager{Authentic: true, ConsulDs: ""}
	client.RunServer(objects...)
}

func TestConsulxCallGRPC_GenID(t *testing.T) {
	initConsul()
	res, err := grpcx.CallRPC(&grpcx.GRPC{
		Token:   rpcToken,
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
	req := &pb.RPCLoginReq{
		Appid: util.MD5("123456"),
		Nonce: util.RandStr(16),
		Time:  util.TimeSecond(),
	}
	appkey := "123456"
	req.Signature = util.HMAC_SHA256(util.AddStr(req.Appid, req.Nonce, req.Time), appkey, true)
	res, err := grpcx.CallRPC(&grpcx.GRPC{
		Service: "PubWorker",
		CallRPC: func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) {
			return pb.NewPubWorkerClient(conn).RPCLogin(ctx, req)
		}})
	if err != nil {
		panic(err)
	}
	object, _ := res.(*pb.RPCLoginRes)
	fmt.Println("call rpc:", object)
}
