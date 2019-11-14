package main

import (
	"fmt"
	"github.com/godaddy-x/freego/sqld"
	"github.com/godaddy-x/freego/util"
	"testing"
)

func BenchmarkSave(b *testing.B) {
	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
	b.StartTimer() //重新开始时间
	for i := 0; i < b.N; i++ { //use b.N for looping
		db, err := new(sqld.MGOManager).Get()
		if err != nil {
			panic(err)
		}
		defer db.Close()
		//l := util.Time()
		o := OwWallet{
			AppID:    util.GetSnowFlakeStrID(),
			WalletID: util.GetSnowFlakeStrID(),
		}
		if err := db.Save(&o); err != nil {
			fmt.Println(err)
		}
	}
	//fmt.Println(wallet.Id)
}

//func BenchmarkFind(b *testing.B) {
//	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
//	b.StartTimer() //重新开始时间
//	for i := 0; i < b.N; i++ { //use b.N for looping
//		db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
//		if err != nil {
//			panic(err)
//		}
//		defer db.Close()
//		o := OwWallet{}
//		if err := db.FindOne(sqlc.M(&OwWallet{}).Eq("id", 1110012978914131972), &o); err != nil {
//			fmt.Println(err)
//		}
//		//fmt.Println(wallet.Id)
//	}
//}
//
//func BenchmarkUpdateBy(b *testing.B) {
//	b.StopTimer()  //调用该函数停止压力测试的时间计数go test -run="webbench_test.go" -test.bench="."*
//	b.StartTimer() //重新开始时间
//	for i := 0; i < b.N; i++ { //use b.N for looping
//		db, err := new(sqld.MGOManager).Get(sqld.Option{OpenTx: &sqld.TRUE})
//		if err != nil {
//			panic(err)
//		}
//		defer db.Close()
//		if err := db.UpdateByCnd(sqlc.M(&OwWallet{}).Eq("id", 1110012978914131972).UpdateKeyValue([]string{"appID", "ctime"}, util.GetSnowFlakeStrID(), 1)); err != nil {
//			fmt.Println(err)
//		}
//		//fmt.Println(wallet.Id)
//	}
//}
