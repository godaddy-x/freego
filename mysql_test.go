package main

import (
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"testing"
)

func init() {
	initDriver()
}

func TestMysqlSave(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 20; i++ {
		wallet := OwWallet{
			AppID:        utils.MD5(utils.NextSID(), true),
			WalletID:     utils.NextSID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     utils.AddStr("m/44'/88'/", i, "'"),
			Alias:        utils.AddStr("hello_qtum_", i),
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := utils.UnixMilli()
	if err := db.Save(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlUpdate(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 10; i++ {
		wallet := OwWallet{
			Id:           1570732354786295848,
			AppID:        "123456",
			WalletID:     utils.NextSID(),
			PasswordType: 1,
			Password:     "123456",
			RootPath:     "m/44'/88'/0'",
			Alias:        "hello_qtum111333",
			IsTrust:      0,
			AuthKey:      "",
		}
		vs = append(vs, &wallet)
	}
	l := utils.UnixMilli()
	if err := db.Update(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlUpdateByCnd(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Upset([]string{"password"}, "1111").Eq("id", 1570732354786295848)); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlDetele(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 2000; i++ {
		wallet := OwWallet{
			Id: 1570732354786295848,
		}
		vs = append(vs, &wallet)
	}
	l := utils.UnixMilli()
	if err := db.Delete(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlFindById(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	wallet := OwWallet{
		Id: 1570732354786295849,
	}
	if err := db.FindById(&wallet); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlFindOne(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysql()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	wallet := OwWallet{}
	if err := db.FindOne(sqlc.M().Eq("id", 1570732354786295849), &wallet); err != nil {
		panic(err)
	}
}

func TestMysqlFindList(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	var result []*OwWallet
	if err := db.FindList(sqlc.M(&OwWallet{}).Orderby("id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlFindListComplex(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	var result []*OwWallet
	if err := db.FindListComplex(sqlc.M(&OwWallet{}).Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlFindOneComplex(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	result := OwWallet{}
	if err := db.FindOneComplex(sqlc.M().Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlCount(t *testing.T) {
	initMysqlDB()
	db, err := new(sqld.MysqlManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	if c, err := db.Count(sqlc.M(&OwWallet{}).Orderby("id", sqlc.DESC_).Limit(1, 30)); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}
