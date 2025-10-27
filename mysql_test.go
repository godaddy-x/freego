package main

import (
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"testing"
	"time"
)

func init() {
	initDriver()
}

func TestMysqlSave(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysql(sqld.Option{OpenTx: true, AutoID: true})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 1; i++ {
		wallet := OwAuth{
			//Ctime: utils.UnixMilli(),
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
	for i := 0; i < 1; i++ {
		wallet := OwAuth{
			Id: 1649040212178763776,
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
	//object := UserProfileNatural{}
	//object.CreatedAt = utils.UnixMilli()
	//object.UpdatedAt = utils.UnixMilli()
	//if err := db.Save(&object); err != nil {
	//	fmt.Println(err)
	//}
	//fmt.Println(object)
	object2 := UserProfileNatural{}
	if err := db.FindOne(sqlc.M().Eq("user_id", 1900478443951226880), &object2); err != nil {
		fmt.Println(err)
	}
	fmt.Println(object2)
	a, _ := utils.Str2FormatTime("1965-10-25", utils.DateFmt, time.UTC)
	object2.BirthDate = a
	if err := db.Update(&object2); err != nil {
		fmt.Println(err)
	}
}

func TestMysqlDeleteById(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysql()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	ret, err := db.DeleteById(&OwWallet{}, 1136187262606770177, 1136187262606770178)
	if err != nil {
		panic(err)
	}
	fmt.Println(ret)
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
	if err := db.FindList(sqlc.M(&OwWallet{}).Eq("id", 1109996130134917121).Orderby("id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
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
