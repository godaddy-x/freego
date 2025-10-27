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
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 3; i++ {
		wallet := OwWallet{
			Ctime: utils.UnixMilli(),
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
	db, err := sqld.NewMysqlTx(true)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			Id:    1982733730401222656,
			AppID: "123456777",
			//Secret: "1111122",
			//Ctime:  utils.UnixMilli(),
			//Seed:   "3321",
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
	if _, err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Upset([]string{"appID", "utime"}, "123456789", utils.UnixMilli()).Eq("id", 1649040212178763776)); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlDelete(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			Id: 1982733730401222656,
		}
		vs = append(vs, &wallet)
	}
	l := utils.UnixMilli()
	if err := db.Delete(vs...); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlDeleteById(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysql()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	ret, err := db.DeleteById(&OwWallet{}, 1982734524403941376, 1982734572302893056)
	if err != nil {
		panic(err)
	}
	fmt.Println(ret)
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
	if err := db.FindOne(sqlc.M().Eq("id", 1982734572302893056).Eq("appID", 1).NotIn("id", 2).Between("ctime", 1, 2).Orderby("id", sqlc.DESC_), &wallet); err != nil {
		fmt.Println(err)
	}
	fmt.Println(wallet)
}

func TestMysqlFindList(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	var result []*OwWallet
	if err := db.FindList(sqlc.M(&OwWallet{}).Eq("id", 1109996130134917121).Orderby("id", sqlc.DESC_), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlCount(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	if c, err := db.Count(sqlc.M(&OwWallet{}).Orderby("id", sqlc.DESC_).Groupby("id").Limit(1, 30)); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlExists(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	if c, err := db.Exists(sqlc.M(&OwWallet{}).Eq("id", 12).Eq("appID", 123).Orderby("id", sqlc.DESC_).Groupby("id").Limit(1, 30)); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
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
	if err := db.FindListComplex(sqlc.M(&OwWallet{}).UnEscape().Fields("count(a.id) as `id`").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
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
	if err := db.FindOneComplex(sqlc.M().UnEscape().Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}
