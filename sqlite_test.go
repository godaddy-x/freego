package main

import (
	"fmt"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
	"os/exec"
	"strings"
	"testing"
)

func TestSqliteSave(t *testing.T) {
	initSqliteDB()
	db, err := sqld.NewSqlite(sqld.Option{OpenTx: true})
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

func TestSqliteUpdate(t *testing.T) {
	initSqliteDB()
	db, err := new(sqld.SqliteManager).Get(sqld.Option{OpenTx: true})
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

func TestSqliteUpdateByCnd(t *testing.T) {
	initSqliteDB()
	db, err := new(sqld.SqliteManager).Get(sqld.Option{OpenTx: true})
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

func TestSqliteDelete(t *testing.T) {
	initSqliteDB()
	db, err := new(sqld.SqliteManager).Get(sqld.Option{OpenTx: true})
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

func TestSqliteFindById(t *testing.T) {
	initSqliteDB()
	db, err := new(sqld.SqliteManager).Get()
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

func TestSqliteFindOne(t *testing.T) {
	initSqliteDB()
	db, err := sqld.NewSqlite()
	if err != nil {
		panic(err)
	}
	defer db.Close()
	wallet := OwWallet{}
	if err := db.FindOne(sqlc.M().Eq("id", 1109996130134917121), &wallet); err != nil {
		panic(err)
	}
}

func TestSqliteDeleteById(t *testing.T) {
	initSqliteDB()
	db, err := sqld.NewSqlite()
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

func TestSqliteFindList(t *testing.T) {
	initSqliteDB()
	db, err := new(sqld.SqliteManager).Get(sqld.Option{OpenTx: false})
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	var result []*OwAuth
	if err := db.FindList(sqlc.M(&OwAuth{}).Gt("id", 0).Orderby("id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestSqliteFindListComplex(t *testing.T) {
	//initSqliteDB()
	//db, err := new(sqld.SqliteManager).Get(sqld.Option{OpenTx: false})
	//if err != nil {
	//	panic(err)
	//}
	//defer db.Close()
	//l := utils.UnixMilli()
	//var result []*OwWallet
	//if err := db.FindListComplex(sqlc.M(&OwWallet{}).UnEscape().Fields("count(a.id) as `id`").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
	//	fmt.Println(err)
	//}
	//fmt.Println("cost: ", utils.UnixMilli()-l)
	fmt.Println(1 << 15)
}

func getMachineIDWindows() (string, error) {
	// On Windows, use the registry to get the MachineGuid
	cmd := exec.Command("reg", "query", "HKLM\\SOFTWARE\\Microsoft\\Cryptography", "/v", "MachineGuid")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	lastLine := lines[len(lines)-1]
	parts := strings.Split(lastLine, "    ")
	return parts[len(parts)-1], nil
}

func TestSqliteFindOneComplex(t *testing.T) {
	fmt.Println(getMachineIDWindows())
	//initSqliteDB()
	//db, err := new(sqld.SqliteManager).Get(sqld.Option{OpenTx: false})
	//if err != nil {
	//	panic(err)
	//}
	//defer db.Close()
	//l := utils.UnixMilli()
	//result := OwWallet{}
	//if err := db.FindOneComplex(sqlc.M().UnEscape().Fields("count(a.id) as id").From("ow_wallet a").Orderby("a.id", sqlc.DESC_).Limit(1, 5), &result); err != nil {
	//	fmt.Println(err)
	//}
	//fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestSqliteCount(t *testing.T) {
	initSqliteDB()
	db, err := new(sqld.SqliteManager).Get(sqld.Option{OpenTx: false})
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
