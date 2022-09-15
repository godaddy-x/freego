package main

import (
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"testing"
)

func TestMongoFindOne(t *testing.T) {
	l := utils.UnixMilli()
	db, err := new(sqld.MGOManager).Get()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	o := &OwWallet{}
	if err := db.FindOne(sqlc.M().Fields("password").Eq("id", 1182663723102240768), o); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMongoUpdateByCnd(t *testing.T) {
	l := utils.UnixMilli()
	db, err := new(sqld.MGOManager).Get()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Eq("id", 1182663723102240768).Upset([]string{"password", "rootPath"}, "123456test", "/test/123")); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}
