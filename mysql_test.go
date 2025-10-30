package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/godaddy-x/freego/zlog"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
)

func init() {
	initDriver()
	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.INFO, Console: false})
}

func TestMysqlSave(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var vs []sqlc.Object
	for i := 0; i < 1; i++ {
		wallet := OwWallet{
			AppID:        "test_app_" + utils.RandStr(6),
			WalletID:     "wallet_" + utils.RandStr(8),
			Alias:        "test_wallet_" + utils.RandStr(4),
			IsTrust:      1,
			PasswordType: 1,
			Password:     "encrypted_password_" + utils.RandStr(10),
			AuthKey:      "auth_key_" + utils.RandStr(12),
			RootPath:     "/path/to/wallet/" + utils.RandStr(8),
			AccountIndex: 0,
			Keystore:     `{"version":3,"id":"1234-5678-9abc-def0","address":"abcd1234ef567890","crypto":{"ciphertext":"cipher","cipherparams":{"iv":"iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"salt","n":8192,"r":8,"p":1},"mac":"mac"}}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
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
			Id:           1983681980977381376,
			AppID:        "updated_app_" + utils.RandStr(6),
			WalletID:     "updated_wallet_" + utils.RandStr(8),
			Alias:        "updated_wallet_" + utils.RandStr(4),
			IsTrust:      2,
			PasswordType: 2,
			Password:     "updated_password_" + utils.RandStr(10),
			AuthKey:      "updated_auth_key_" + utils.RandStr(12),
			RootPath:     "/updated/path/to/wallet/" + utils.RandStr(8),
			AccountIndex: 1,
			Keystore:     `{"version":3,"id":"updated-1234-5678-9abc-def0","address":"updatedabcd1234ef567890","crypto":{"ciphertext":"updated_cipher","cipherparams":{"iv":"updated_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"updated_salt","n":8192,"r":8,"p":1},"mac":"updated_mac"}}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    2,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        2,
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
	db, err := sqld.NewMysqlTx(true)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	if _, err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Upset([]string{"appID", "utime"}, "222222222", utils.UnixMilli()).Eq("id", 1982735905676328960)); err != nil {
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
	db, err := sqld.NewMysqlTx(false)
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

func TestMysqlDeleteByCnd(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	// 使用条件删除
	rowsAffected, err := db.DeleteByCnd(sqlc.M(&OwWallet{}).UnEscape().
		Eq("appID", "1").NotEq("id", 1).
		Gte("ctime", 1).Lte("ctime", 2).
		IsNull("appID").IsNotNull("appID").
		Between("id", 1, 2).
		NotBetween("id", 1, 2).
		In("id", 1, 2, 3, 4).
		NotIn("id", 1, 2).
		Like("appID", "test").
		NotLike("appID", "test").
		Or(sqlc.M().Eq("id", 1), sqlc.M().In("id", 1, 2, 3)).
		Or(sqlc.M().Eq("appID", 1), sqlc.M().In("appID", 1, 2, 3)).
		Or(sqlc.M().Eq("appID", 1).In("id", 1, 2, 3), sqlc.M().In("appID", 1, 2, 3).Gt("ctime", 12).Lt("ctime", 23)))
	if err != nil {
		fmt.Println("DeleteByCnd failed:", err)
		return
	}
	fmt.Println("Deleted rows:", rowsAffected)
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlFindOne(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	wallet := OwWallet{}
	if err := db.FindOne(sqlc.M().Eq("id", 1983681980977381376).Orderby("id", sqlc.DESC_), &wallet); err != nil {
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
	if err := db.FindList(sqlc.M(&OwWallet{}).Eq("id", 1983681980977381376).Between("id", 1983681980977381376, 1983681980977381376).Limit(1, 1000).Orderby("id", sqlc.DESC_).Orderby("appID", sqlc.ASC_), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
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
	if c, err := db.Count(sqlc.M(&OwWallet{}).UseEscape().Eq("id", 1983681980977381376).Orderby("id", sqlc.DESC_).Groupby("id").Limit(1, 30)); err != nil {
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
	if c, err := db.Exists(sqlc.M(&OwWallet{}).UseEscape().Eq("id", 1983681980977381376).Eq("appID", "updated_app_yzNQSr")); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(c)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlFindOneComplex(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	result := OwWallet{}
	if err := db.FindOneComplex(sqlc.M().Fields("a.id id", "a.appID appID").From("ow_wallet a").Join(sqlc.LEFT_, "user b", "a.id = b.id").Eq("a.id", 218418572484169728).Eq("a.appID", "updated_app_yzNQSr").Orderby("a.id", sqlc.ASC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

func TestMysqlFindListComplex(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	var result []*OwWallet
	if err := db.FindListComplex(sqlc.M(&OwWallet{}).Fields("a.id id", "a.appID appID").From("ow_wallet a").Join(sqlc.LEFT_, "user b", "a.id = b.id").Eq("a.id", 218418572484169728).Eq("a.appID", "updated_app_yzNQSr").Orderby("a.id", sqlc.ASC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
	fmt.Println("cost: ", utils.UnixMilli()-l)
}
