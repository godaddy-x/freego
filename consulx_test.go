package main

import (
	"fmt"
	"github.com/godaddy-x/freego/component/consul"
	"testing"
)

type ReqObj struct {
	Uid  int
	Name string
}

type ResObj struct {
	Name   string
	Title  string
	Status int
}

type UserService interface {
	FindUser(req *ReqObj, obj *ResObj) error
	FindUserList(req *ReqObj, obj *ResObj) error
}

type UserServiceImpl struct {
}

func (self *UserServiceImpl) FindUser(req *ReqObj, obj *ResObj) error {
	fmt.Println("findUser: ", req)
	obj.Name = "张三"
	obj.Status = 5
	obj.Title = "title message"
	return nil
}

func (self *UserServiceImpl) FindUserList(req *ReqObj, obj *ResObj) error {
	fmt.Println("findUserList: ", req)
	obj.Name = "李四"
	obj.Status = 1
	obj.Title = "title message"
	return nil
}

func TestConsulxAddRPC(t *testing.T) {
	new(consul.ConsulManager).InitConfig(consul.ConsulConfig{})

	mgr, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}

	mgr.AddRPC(&consul.CallInfo{
		Tags:  []string{"用户服务"},
		Iface: &UserServiceImpl{},
	})

	go consul.StartSnowflakeServe()

	mgr.StartListenAndServe()

}

func TestConsulxCallRPC_USER(t *testing.T) {
	new(consul.ConsulManager).InitConfig(consul.ConsulConfig{})

	mgr, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}

	req := &ReqObj{123, "托尔斯泰"}
	res := &ResObj{}

	if err := mgr.CallRPC(&consul.CallInfo{
		Service:  "UserServiceImpl",
		Method:   "FindUser",
		Request:  req,
		Response: res,
	}); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("rpc result: ", res)
}

func TestConsulxCallRPC_ID(t *testing.T) {
	new(consul.ConsulManager).InitConfig(consul.ConsulConfig{})

	mgr, err := new(consul.ConsulManager).Client()
	if err != nil {
		panic(err)
	}

	req := &consul.ReqObj{}
	res := &consul.ResObj{}

	if err := mgr.CallRPC(&consul.CallInfo{
		Service:  "SnowflakeWorkId",
		Method:   "Generate",
		Request:  req,
		Response: res,
	}); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("rpc result: ", res)
}
