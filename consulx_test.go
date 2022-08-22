package main

import (
	"context"
	"fmt"
	"github.com/godaddy-x/freego/component/consul"
	"github.com/godaddy-x/freego/component/consul/grpcx"
	"github.com/godaddy-x/freego/component/consul/grpcx/pb"
	"github.com/godaddy-x/freego/util"
	"google.golang.org/grpc"
	"testing"
)

const rpc_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJlMTBhZGMzOTQ5YmE1OWFiYmU1NmUwNTdmMjBmODgzZSIsImF1ZCI6IiIsImlzcyI6IiIsImlhdCI6MCwiZXhwIjoxNjY5Nzk1MDYyLCJkZXYiOiIiLCJqdGkiOiJ4TVlDRnc3QjNtUU1vTmREY3pheUJRPT0iLCJleHQiOnt9fQ==.AXLSwotawZvI+lcGGgT0vQS59v9TYRno3EMXSuc8N6o="

func TestConsulxRunGRPC(t *testing.T) {
	initConsul()
	c, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}
	objects := []*grpcx.GRPC{
		{
			Address: "localhost",
			Service: "PubWorker",
			Tags:    []string{"ID Generator"},
			AddRPC:  func(server *grpc.Server) { pb.RegisterPubWorkerServer(server, &grpcx.PubWorker{}) },
		},
	}
	grpcx.NewGRPC(c).RunServer(objects...)
}

func TestConsulxCallGRPC_GenID(t *testing.T) {
	initConsul()
	c, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}
	res, err := grpcx.NewGRPC(c, rpc_token).CallRPC(&grpcx.GRPC{Service: "PubWorker", CallRPC: func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) {
		rpc := pb.NewPubWorkerClient(conn)
		return rpc.GenerateId(ctx, &pb.GenerateIdReq{})
	}})
	if err != nil {
		panic(err)
	}
	object, _ := res.(*pb.GenerateIdRes)
	fmt.Println("call rpc:", object)
}

func TestConsulxCallGRPC_Login(t *testing.T) {
	initConsul()
	c, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}
	req := &pb.RPCLoginReq{
		Appid: util.MD5("123456"),
		Nonce: util.RandStr(16),
		Time:  util.TimeSecond(),
	}
	appkey := "123456"
	req.Signature = util.HMAC_SHA256(util.AddStr(req.Appid, req.Nonce, req.Time), appkey, true)
	res, err := grpcx.NewGRPC(c).CallRPC(&grpcx.GRPC{Service: "PubWorker", CallRPC: func(conn *grpc.ClientConn, ctx context.Context) (interface{}, error) {
		rpc := pb.NewPubWorkerClient(conn)
		return rpc.RPCLogin(ctx, req)
	}})
	if err != nil {
		panic(err)
	}
	object, _ := res.(*pb.RPCLoginRes)
	fmt.Println("call rpc:", object)
}
