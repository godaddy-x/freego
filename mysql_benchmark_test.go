package main

import (
	"fmt"
	"testing"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
)

func BenchmarkMysqlSave(b *testing.B) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// 预先计算时间戳，避免在循环中重复调用
	now := utils.UnixMilli()

	// 预定义常量字符串，避免动态字符串操作
	const (
		appID    = "bench_app_123456"
		walletID = "bench_wallet_abcdefgh"
		alias    = "bench_wallet_abcd"
		password = "bench_password_abcdefghij"
		authKey  = "bench_auth_key_abcdefghijkl"
		rootPath = "/bench/path/to/wallet/abcdefgh"
		keystore = `{"version":3,"id":"bench-1234-5678-9abc-def0","address":"benchabcd1234ef567890","crypto":{"ciphertext":"bench_cipher","cipherparams":{"iv":"bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"bench_salt","n":8192,"r":8,"p":1},"mac":"bench_mac"}}`
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 每次操作都创建一个新的wallet对象，避免ID冲突
			wallet := &OwWallet{
				Id:           utils.NextIID(),
				AppID:        appID,
				WalletID:     walletID,
				Alias:        alias,
				IsTrust:      1,
				PasswordType: 1,
				Password:     password,
				AuthKey:      authKey,
				RootPath:     rootPath,
				AccountIndex: 0,
				Keystore:     keystore,
				Applytime:    now,
				Succtime:     now,
				Dealstate:    1,
				Ctime:        now,
				Utime:        now,
				State:        1,
			}

			if err := db.Save(wallet); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkMysqlUpdate(b *testing.B) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(true)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// 预先计算时间戳，避免在循环中重复调用
	now := utils.UnixMilli()

	// 预定义常量字符串，避免动态字符串操作
	const (
		originalAppID    = "bench_app_123456"
		originalWalletID = "bench_wallet_abcdefgh"
		originalAlias    = "bench_wallet_abcd"
		originalPassword = "bench_password_abcdefghij"
		originalAuthKey  = "bench_auth_key_abcdefghijkl"
		originalRootPath = "/bench/path/to/wallet/abcdefgh"
		originalKeystore = `{"version":3,"id":"bench-1234-5678-9abc-def0","address":"benchabcd1234ef567890","crypto":{"ciphertext":"bench_cipher","cipherparams":{"iv":"bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"bench_salt","n":8192,"r":8,"p":1},"mac":"bench_mac"}}`

		updatedAppID    = "updated_bench_app_789012"
		updatedWalletID = "updated_bench_wallet_hijklmn"
		updatedAlias    = "updated_bench_wallet_efgh"
		updatedPassword = "updated_bench_password_jklmnopqr"
		updatedAuthKey  = "updated_bench_auth_key_mnopqrstuvwx"
		updatedRootPath = "/updated/bench/path/to/wallet/hijklmn"
		updatedKeystore = `{"version":3,"id":"updated-bench-5678-9abc-def0","address":"updatedbenchabcd1234ef567890","crypto":{"ciphertext":"updated_bench_cipher","cipherparams":{"iv":"updated_bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"updated_bench_salt","n":8192,"r":8,"p":1},"mac":"updated_bench_mac"}}`
	)

	// 创建固定数量的测试数据 (100个)，避免预创建过多对象
	const testDataCount = 100
	var wallets []sqlc.Object
	for i := 0; i < testDataCount; i++ {
		wallet := &OwWallet{
			Id:           utils.NextIID(),
			AppID:        originalAppID,
			WalletID:     originalWalletID,
			Alias:        originalAlias,
			IsTrust:      1,
			PasswordType: 1,
			Password:     originalPassword,
			AuthKey:      originalAuthKey,
			RootPath:     originalRootPath,
			AccountIndex: 0,
			Keystore:     originalKeystore,
			Applytime:    now,
			Succtime:     now,
			Dealstate:    1,
			Ctime:        now,
			Utime:        now,
			State:        1,
		}
		wallets = append(wallets, wallet)
	}

	// 先保存数据
	if err := db.Save(wallets...); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// 为每个goroutine分配固定的wallet索引，避免竞争
		localIndex := 0
		for pb.Next() {
			if localIndex >= len(wallets) {
				localIndex = 0
			}

			wallet := wallets[localIndex].(*OwWallet)
			updateWallet := &OwWallet{
				Id:           wallet.Id,
				AppID:        updatedAppID,
				WalletID:     updatedWalletID,
				Alias:        updatedAlias,
				IsTrust:      2,
				PasswordType: 2,
				Password:     updatedPassword,
				AuthKey:      updatedAuthKey,
				RootPath:     updatedRootPath,
				AccountIndex: 1,
				Keystore:     updatedKeystore,
				Applytime:    now,
				Succtime:     now,
				Dealstate:    2,
				Utime:        now,
				State:        2,
			}

			if err := db.Update(updateWallet); err != nil {
				b.Error(err)
			}
			localIndex++
		}
	})
}

func BenchmarkMysqlFindOne(b *testing.B) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var result OwWallet
			if err := db.FindOne(sqlc.M().Eq("id", 1983821936127377408), &result); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkMysqlFindOneComplex(b *testing.B) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var result OwWallet
			if err := db.FindOneComplex(sqlc.M().Fields("a.id id", "a.appID appID").From("ow_wallet a").Join(sqlc.LEFT_, "user b", "a.id = b.id").Eq("a.id", 218418572484169728).Eq("a.appID", "updated_app_yzNQSr").Orderby("a.id", sqlc.ASC_).Limit(1, 5), &result); err != nil {
				fmt.Println(err)
			}
		}
	})
}

func BenchmarkMysqlFindList(b *testing.B) { // 测试1000行数据查询性能
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result := make([]*OwWallet, 0, 3000)
			if err := db.FindList(sqlc.M(&OwWallet{}).Between("id", 218418572484169728, 1986277638838157312).Limit(1, 3000).Orderby("id", sqlc.DESC_), &result); err != nil {
				fmt.Println(err)
			}
		}
	})
}

func BenchmarkMysqlFindListComplex(b *testing.B) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var result []*OwWallet
			if err := db.FindListComplex(sqlc.M(&OwWallet{}).Fields("a.id id", "a.appID appID").From("ow_wallet a").Join(sqlc.LEFT_, "user b", "a.id = b.id").Eq("a.id", 218418572484169728).Eq("a.appID", "find_bench_app_123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890").Orderby("a.id", sqlc.ASC_).Limit(1, 5), &result); err != nil {
				fmt.Println(err)
			}
		}
	})
}

func BenchmarkMysqlCount(b *testing.B) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// 预先计算时间戳，避免在循环中重复调用
	now := utils.UnixMilli()

	// 确保有测试数据
	var wallets []sqlc.Object
	for i := 0; i < 50; i++ {
		wallet := OwWallet{
			AppID:        "count_bench_app_" + utils.RandStr(6),
			WalletID:     "count_bench_wallet_" + utils.RandStr(8),
			Alias:        "count_bench_wallet_" + utils.RandStr(4),
			IsTrust:      1,
			PasswordType: 1,
			Password:     "count_bench_password_" + utils.RandStr(10),
			AuthKey:      "count_bench_auth_key_" + utils.RandStr(12),
			RootPath:     "/count/bench/path/to/wallet/" + utils.RandStr(8),
			AccountIndex: 0,
			Keystore:     `{"version":3,"id":"count-bench-1234-5678-9abc-def0","address":"countbenchabcd1234ef567890","crypto":{"ciphertext":"count_bench_cipher","cipherparams":{"iv":"count_bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"count_bench_salt","n":8192,"r":8,"p":1},"mac":"count_bench_mac"}}`,
			Applytime:    now,
			Succtime:     now,
			Dealstate:    1,
			Ctime:        now,
			Utime:        now,
			State:        1,
		}
		wallets = append(wallets, &wallet)
	}

	if err := db.Save(wallets...); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := db.Count(sqlc.M(&OwWallet{}).Eq("state", 1)); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkMysqlExists(b *testing.B) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// 预先计算时间戳，避免在循环中重复调用
	now := utils.UnixMilli()

	// 确保有测试数据
	wallet := OwWallet{
		AppID:        "exists_bench_app_" + utils.RandStr(6),
		WalletID:     "exists_bench_wallet_" + utils.RandStr(8),
		Alias:        "exists_bench_wallet_" + utils.RandStr(4),
		IsTrust:      1,
		PasswordType: 1,
		Password:     "exists_bench_password_" + utils.RandStr(10),
		AuthKey:      "exists_bench_auth_key_" + utils.RandStr(12),
		RootPath:     "/exists/bench/path/to/wallet/" + utils.RandStr(8),
		AccountIndex: 0,
		Keystore:     `{"version":3,"id":"exists-bench-1234-5678-9abc-def0","address":"existsbenchabcd1234ef567890","crypto":{"ciphertext":"exists_bench_cipher","cipherparams":{"iv":"exists_bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"exists_bench_salt","n":8192,"r":8,"p":1},"mac":"exists_bench_mac"}}`,
		Applytime:    now,
		Succtime:     now,
		Dealstate:    1,
		Ctime:        now,
		Utime:        now,
		State:        1,
	}
	if err := db.Save(&wallet); err != nil {
		b.Fatal(err)
	}

	// 获取保存后的ID
	var savedWallet OwWallet
	if err := db.FindOne(sqlc.M().Orderby("id", sqlc.DESC_), &savedWallet); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := db.Exists(sqlc.M(&OwWallet{}).Eq("id", savedWallet.Id).Eq("state", 1)); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkMysqlDelete(b *testing.B) {
	initMysqlDB()

	// 预先计算时间戳，避免在循环中重复调用
	now := utils.UnixMilli()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			b.Fatal(err)
		}
		defer db.Close()

		for pb.Next() {
			// 为每个删除操作创建新的测试数据
			wallet := OwWallet{
				AppID:        "del_bench_app_" + utils.RandStr(6),
				WalletID:     "del_bench_wallet_" + utils.RandStr(8),
				Alias:        "del_bench_wallet_" + utils.RandStr(4),
				IsTrust:      1,
				PasswordType: 1,
				Password:     "del_bench_password_" + utils.RandStr(10),
				AuthKey:      "del_bench_auth_key_" + utils.RandStr(12),
				RootPath:     "/del/bench/path/to/wallet/" + utils.RandStr(8),
				AccountIndex: 0,
				Keystore:     `{"version":3,"id":"del-bench-1234-5678-9abc-def0","address":"delbenchabcd1234ef567890","crypto":{"ciphertext":"del_bench_cipher","cipherparams":{"iv":"del_bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"del_bench_salt","n":8192,"r":8,"p":1},"mac":"del_bench_mac"}}`,
				Applytime:    now,
				Succtime:     now,
				Dealstate:    1,
				Ctime:        now,
				Utime:        now,
				State:        1,
			}

			// 先保存再删除
			if err := db.Save(&wallet); err != nil {
				b.Error(err)
				continue
			}

			// 获取刚保存的数据ID
			var savedWallet OwWallet
			if err := db.FindOne(sqlc.M().Orderby("id", sqlc.DESC_), &savedWallet); err != nil {
				b.Error(err)
				continue
			}

			deleteWallet := OwWallet{Id: savedWallet.Id}
			if err := db.Delete(&deleteWallet); err != nil {
				b.Error(err)
			}
		}
	})
}
