package main

import (
	"fmt"
	"github.com/godaddy-x/freego/sqlc"
	"github.com/godaddy-x/freego/sqld"
	"github.com/godaddy-x/freego/util"
	"testing"
)

func TestMongoFindOne(t *testing.T) {
	l := util.Time()
	db, err := new(sqld.MGOManager).Get()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	o := &OwWallet{}
	if err := db.FindOne(sqlc.M().Fields("password").Eq("id", 1182663723102240768), o); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", util.Time()-l)
}