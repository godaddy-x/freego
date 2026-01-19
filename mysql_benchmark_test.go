// Package main - MySQL ORM性能基准测试套件
//
// 该文件包含完整的MySQL ORM性能基准测试，覆盖以下测试维度：
// 1. 基础CRUD操作：Save、Update、Delete、FindOne、FindList
// 2. 批量操作：BatchSave、BatchUpdate
// 3. 事务操作：TransactionCommit
// 4. 复杂查询：ComplexQuery、IndexPerformance
// 5. 并发性能：ConnectionPool、LargeDataset
// 6. 内存使用：MemoryUsage
//
// 所有测试都使用并发模式(b.RunParallel)，模拟真实生产环境的并发压力
// 测试数据使用预分配和常量优化，减少基准测试中的额外开销
package main

import (
	"fmt"
	"testing"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
)

// BenchmarkMysqlSave INSERT操作性能基准测试
// 测试单条记录插入的性能表现，包含数据序列化和网络传输开销
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
		authKey  = "bench_auth_key_abcdefghijkl"
		rootPath = "/bench/path/to/wallet/abcdefgh"
	)

	// 密码常量（字节数组）
	password := []byte("bench_password_abcdefghij")
	keystore := `{"version":3,"id":"bench-1234-5678-9abc-def0","address":"benchabcd1234ef567890","crypto":{"ciphertext":"bench_cipher","cipherparams":{"iv":"bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"bench_salt","n":8192,"r":8,"p":1},"mac":"bench_mac"}}`

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

// BenchmarkMysqlUpdate UPDATE操作性能基准测试
// 测试记录更新的性能表现，包含事务处理和数据一致性保证
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
		originalAuthKey  = "bench_auth_key_abcdefghijkl"
		originalRootPath = "/bench/path/to/wallet/abcdefgh"
		originalKeystore = `{"version":3,"id":"bench-1234-5678-9abc-def0","address":"benchabcd1234ef567890","crypto":{"ciphertext":"bench_cipher","cipherparams":{"iv":"bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"bench_salt","n":8192,"r":8,"p":1},"mac":"bench_mac"}}`

		updatedAppID    = "updated_bench_app_789012"
		updatedWalletID = "updated_bench_wallet_hijklmn"
		updatedAlias    = "updated_bench_wallet_efgh"
		updatedAuthKey  = "updated_bench_auth_key_mnopqrstuvwx"
		updatedRootPath = "/updated/bench/path/to/wallet/hijklmn"
	)

	// 密码常量（字节数组）
	originalPassword := []byte("bench_password_abcdefghij")
	updatedPassword := []byte("updated_bench_password_jklmnopqr")
	updatedKeystore := `{"version":3,"id":"updated-bench-5678-9abc-def0","address":"updatedbenchabcd1234ef567890","crypto":{"ciphertext":"updated_bench_cipher","cipherparams":{"iv":"updated_bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"updated_bench_salt","n":8192,"r":8,"p":1},"mac":"updated_bench_mac"}}`

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

// BenchmarkMysqlFindOne 单条记录查询性能基准测试
// 测试根据ID查询单条记录的性能表现，评估索引查询效率
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
			if err := db.FindOne(sqlc.M().Eq("id", 1988433892066983936), &result); err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkMysqlFindOneComplex 复杂单条查询性能基准测试
// 测试包含JOIN连接查询的单条记录查询性能，评估复杂查询的开销
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

// BenchmarkMysqlFindList 列表查询性能基准测试
// 测试分页查询多条记录的性能表现，包含排序和限制结果集
func BenchmarkMysqlFindList(b *testing.B) {
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
			if err := db.FindList(sqlc.M(&OwWallet{}).Between("id", 1987689412850352128, 1988229401988300802).Limit(1, 3000).Orderby("id", sqlc.DESC_), &result); err != nil {
				fmt.Println(err)
			}
		}
	})
}

// BenchmarkMysqlFindListComplex 复杂列表查询性能基准测试
// 测试包含JOIN连接查询的列表查询性能，评估复杂查询的数据处理开销
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

// BenchmarkMysqlCount 记录计数查询性能基准测试
// 测试COUNT聚合查询的性能表现，评估统计查询的开销
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
			Password:     []byte("count_bench_password_" + utils.RandStr(10)),
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

// BenchmarkMysqlExists 记录存在性检查性能基准测试
// 测试EXISTS查询的性能表现，评估布尔值查询的开销
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
		Password:     []byte("exists_bench_password_" + utils.RandStr(10)),
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

// BenchmarkMysqlDelete DELETE操作性能基准测试
// 测试记录删除的性能表现，包含级联删除和索引更新的开销
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
				Password:     []byte("del_bench_password_" + utils.RandStr(10)),
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

// BenchmarkMysqlBatchSave 批量保存操作性能基准测试
// 测试一次性批量插入50条记录的性能表现，评估批量操作的吞吐量和效率
// 包含数据序列化、参数绑定、批量网络传输等完整开销
func BenchmarkMysqlBatchSave(b *testing.B) {
	initMysqlDB()

	const batchSize = 50 // 每次批量保存50条记录，模拟中等规模的批量操作

	// 预先计算时间戳，避免在循环中重复调用
	now := utils.UnixMilli()

	// 预定义常量字符串，避免动态字符串操作
	const (
		appID    = "batch_bench_app_123456"
		walletID = "batch_bench_wallet_abcdefgh"
		alias    = "batch_bench_wallet_abcd"
		password = "batch_bench_password_abcdefghij"
		authKey  = "batch_bench_auth_key_abcdefghijkl"
		rootPath = "/batch/bench/path/to/wallet/abcdefgh"
		keystore = `{"version":3,"id":"batch-bench-1234-5678-9abc-def0","address":"batchbenchabcd1234ef567890","crypto":{"ciphertext":"batch_bench_cipher","cipherparams":{"iv":"batch_bench_iv"},"cipher":"aes-128-ctr","kdf":"scrypt","kdfparams":{"dklen":32,"salt":"batch_bench_salt","n":8192,"r":8,"p":1},"mac":"batch_bench_mac"}}`
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			db, err := sqld.NewMysqlTx(false)
			if err != nil {
				b.Fatal(err)
			}

			// 准备批量数据
			var wallets []sqlc.Object
			for i := 0; i < batchSize; i++ {
				wallet := &OwWallet{
					Id:           utils.NextIID(),
					AppID:        appID,
					WalletID:     walletID,
					Alias:        alias,
					IsTrust:      1,
					PasswordType: 1,
					Password:     []byte(password),
					AuthKey:      authKey,
					RootPath:     rootPath,
					AccountIndex: int64(i),
					Keystore:     keystore,
					Applytime:    now,
					Succtime:     now,
					Dealstate:    1,
					Ctime:        now,
					Utime:        now,
					State:        1,
				}
				wallets = append(wallets, wallet)
			}

			if err := db.Save(wallets...); err != nil {
				b.Error(err)
			}
			db.Close()
		}
	})
}

// BenchmarkMysqlBatchUpdate 批量更新操作性能基准测试
// 测试批量更新多条记录的性能表现，评估更新操作的并发处理能力
// 预先准备100条测试记录，每个goroutine循环更新其中的20条记录
func BenchmarkMysqlBatchUpdate(b *testing.B) {
	initMysqlDB()

	const batchSize = 20     // 每次批量更新20条记录，模拟小批量更新场景
	const totalRecords = 100 // 预先准备100条测试记录，确保数据充足

	// 预先计算时间戳
	now := utils.UnixMilli()

	// 创建预设的测试数据
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}

	var testWallets []sqlc.Object
	for i := 0; i < totalRecords; i++ {
		wallet := &OwWallet{
			Id:           utils.NextIID(),
			AppID:        fmt.Sprintf("batch_update_bench_app_%d", i),
			WalletID:     fmt.Sprintf("batch_update_bench_wallet_%d", i),
			Alias:        fmt.Sprintf("batch_update_bench_alias_%d", i),
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte(fmt.Sprintf("batch_update_bench_password_%d", i)),
			AuthKey:      fmt.Sprintf("batch_update_bench_auth_%d", i),
			RootPath:     fmt.Sprintf("/batch/update/bench/path/%d", i),
			AccountIndex: int64(i),
			Keystore:     fmt.Sprintf(`{"batch_update":"data_%d"}`, i),
			Applytime:    now,
			Succtime:     now,
			Dealstate:    1,
			Ctime:        now,
			Utime:        now,
			State:        1,
		}
		testWallets = append(testWallets, wallet)
	}

	// 预先保存测试数据
	if err := db.Save(testWallets...); err != nil {
		b.Fatal(err)
	}
	db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		localIndex := 0
		for pb.Next() {
			db, err := sqld.NewMysqlTx(false)
			if err != nil {
				b.Fatal(err)
			}

			// 更新一批记录
			for i := 0; i < batchSize; i++ {
				if localIndex >= len(testWallets) {
					localIndex = 0
				}
				wallet := testWallets[localIndex].(*OwWallet)
				wallet.Alias = fmt.Sprintf("updated_batch_alias_%d", localIndex)
				wallet.Utime = utils.UnixMilli()

				if err := db.Update(wallet); err != nil {
					b.Error(err)
				}
				localIndex++
			}
			db.Close()
		}
	})
}

// BenchmarkMysqlTransactionCommit 事务提交操作性能基准测试
// 测试事务中包含多个CRUD操作的完整提交性能，评估ACID保证的开销
// 每个事务包含：插入2条记录、更新1条记录、事务提交
func BenchmarkMysqlTransactionCommit(b *testing.B) {
	initMysqlDB()

	// 预先计算时间戳，避免在事务中重复调用
	now := utils.UnixMilli()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			db, err := sqld.NewMysqlTx(true) // 开启事务
			if err != nil {
				b.Fatal(err)
			}

			// 在事务中执行多个操作
			wallet1 := &OwWallet{
				Id:           utils.NextIID(),
				AppID:        "tx_bench_app_1_" + utils.RandStr(4),
				WalletID:     "tx_bench_wallet_1_" + utils.RandStr(6),
				Alias:        "tx_bench_alias_1",
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte("tx_bench_password_1"),
				AuthKey:      "tx_bench_auth_1_" + utils.RandStr(8),
				RootPath:     "/tx/bench/path/1",
				AccountIndex: 0,
				Keystore:     `{"tx":"data_1"}`,
				Applytime:    now,
				Succtime:     now,
				Dealstate:    1,
				Ctime:        now,
				Utime:        now,
				State:        1,
			}

			wallet2 := &OwWallet{
				Id:           utils.NextIID(),
				AppID:        "tx_bench_app_2_" + utils.RandStr(4),
				WalletID:     "tx_bench_wallet_2_" + utils.RandStr(6),
				Alias:        "tx_bench_alias_2",
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte("tx_bench_password_2"),
				AuthKey:      "tx_bench_auth_2_" + utils.RandStr(8),
				RootPath:     "/tx/bench/path/2",
				AccountIndex: 1,
				Keystore:     `{"tx":"data_2"}`,
				Applytime:    now,
				Succtime:     now,
				Dealstate:    1,
				Ctime:        now,
				Utime:        now,
				State:        1,
			}

			// 保存两个记录
			if err := db.Save(wallet1, wallet2); err != nil {
				b.Error(err)
				db.Close()
				continue
			}

			// 更新第一个记录
			wallet1.Alias = "updated_tx_bench_alias_1"
			wallet1.Utime = utils.UnixMilli()
			if err := db.Update(wallet1); err != nil {
				b.Error(err)
				db.Close()
				continue
			}

			// 提交事务
			if err := db.Close(); err != nil { // 事务自动提交
				b.Error(err)
			}
		}
	})
}

// BenchmarkMysqlComplexQuery 复杂查询条件性能基准测试
// 测试包含多条件过滤、范围查询、模糊匹配的复杂WHERE子句性能
// 包含6个查询条件：等值、IN范围、LIKE模糊、GTE/LTE范围、时间范围
func BenchmarkMysqlComplexQuery(b *testing.B) {
	initMysqlDB()

	// 预先准备200条测试数据，包含各种条件组合用于复杂查询
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}

	now := utils.UnixMilli()
	var testWallets []sqlc.Object
	for i := 0; i < 200; i++ {
		wallet := &OwWallet{
			Id:           utils.NextIID(),
			AppID:        fmt.Sprintf("complex_bench_app_%d", i%10), // 重复的appID用于测试
			WalletID:     fmt.Sprintf("complex_bench_wallet_%d", i),
			Alias:        fmt.Sprintf("complex_bench_alias_%d", i%5), // 重复的alias用于测试
			IsTrust:      int64(i % 3),                               // 0, 1, 2循环
			PasswordType: int64(i % 2),                               // 0, 1循环
			Password:     []byte(fmt.Sprintf("complex_bench_password_%d", i)),
			AuthKey:      fmt.Sprintf("complex_bench_auth_%d", i),
			RootPath:     fmt.Sprintf("/complex/bench/path/%d", i),
			AccountIndex: int64(i % 10),
			Keystore:     fmt.Sprintf(`{"complex":"data_%d"}`, i),
			Applytime:    now,
			Succtime:     now,
			Dealstate:    int64(i % 4), // 0, 1, 2, 3循环
			Ctime:        now,
			Utime:        now,
			State:        int64(i % 2), // 0, 1循环
		}
		testWallets = append(testWallets, wallet)
	}

	// 保存测试数据
	if err := db.Save(testWallets...); err != nil {
		b.Fatal(err)
	}
	db.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			db, err := sqld.NewMysqlTx(false)
			if err != nil {
				b.Fatal(err)
			}

			// 执行复杂的多条件查询
			var results []*OwWallet
			err = db.FindList(sqlc.M(&OwWallet{}).
				Eq("isTrust", 1).
				In("dealstate", 0, 1, 2).
				Like("appID", "complex_bench_app_%").
				Gte("accountIndex", 2).
				Lte("accountIndex", 7).
				Between("ctime", now-1000, now+1000).
				Orderby("id", sqlc.DESC_).
				Limit(1, 20), &results)

			if err != nil {
				b.Error(err)
			}
			db.Close()
		}
	})
}

// BenchmarkMysqlIndexPerformance 索引性能对比基准测试
// 对比有索引字段和无索引字段的查询性能差异，量化索引优化的效果
// 预先准备500条测试数据，分别测试appID(有索引)和alias(无索引)字段查询
func BenchmarkMysqlIndexPerformance(b *testing.B) {
	initMysqlDB()

	// 预先准备500条测试数据，确保索引字段和非索引字段都有足够的测试数据
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}

	now := utils.UnixMilli()
	var testWallets []sqlc.Object
	for i := 0; i < 500; i++ {
		wallet := &OwWallet{
			Id:           utils.NextIID(),
			AppID:        fmt.Sprintf("index_bench_app_%d", i),    // appID有索引
			WalletID:     fmt.Sprintf("index_bench_wallet_%d", i), // walletID无索引
			Alias:        fmt.Sprintf("index_bench_alias_%d", i),  // alias无索引
			IsTrust:      int64(i % 2),
			PasswordType: 1,
			Password:     []byte(fmt.Sprintf("index_bench_password_%d", i)),
			AuthKey:      fmt.Sprintf("index_bench_auth_%d", i),
			RootPath:     fmt.Sprintf("/index/bench/path/%d", i),
			AccountIndex: int64(i),
			Keystore:     fmt.Sprintf(`{"index":"data_%d"}`, i),
			Applytime:    now,
			Succtime:     now,
			Dealstate:    1,
			Ctime:        now,
			Utime:        now,
			State:        1,
		}
		testWallets = append(testWallets, wallet)
	}

	// 保存测试数据
	if err := db.Save(testWallets...); err != nil {
		b.Fatal(err)
	}
	db.Close()

	b.Run("IndexedFieldQuery", func(b *testing.B) { // 测试有索引字段(appID)的查询性能
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					b.Fatal(err)
				}

				var result OwWallet
				// appID字段在OwWallet中定义了索引
				if err := db.FindOne(sqlc.M().Eq("appID", "index_bench_app_123"), &result); err != nil {
					b.Error(err)
				}
				db.Close()
			}
		})
	})

	b.Run("NonIndexedFieldQuery", func(b *testing.B) { // 测试无索引字段(alias)的查询性能
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					b.Fatal(err)
				}

				var result OwWallet
				// alias字段在OwWallet中没有定义索引
				if err := db.FindOne(sqlc.M().Eq("alias", "index_bench_alias_123"), &result); err != nil {
					b.Error(err)
				}
				db.Close()
			}
		})
	})
}

// BenchmarkMysqlConnectionPool 连接池性能基准测试
// 测试高并发场景和事务工作负载下连接池的使用效率和性能表现
// 包含两个子测试：高并发查询和事务工作负载
func BenchmarkMysqlConnectionPool(b *testing.B) {
	initMysqlDB()

	now := utils.UnixMilli()

	b.Run("HighConcurrency", func(b *testing.B) { // 高并发场景下的连接池性能
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					b.Fatal(err)
				}

				// 执行一个简单的查询操作
				var result OwWallet
				if err := db.FindOne(sqlc.M().Eq("id", 1983821936127377408), &result); err != nil {
					// 忽略查询错误，只测试连接性能
				}
				db.Close()
			}
		})
	})

	b.Run("TransactionalWorkload", func(b *testing.B) { // 事务工作负载下的连接池性能
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(true) // 事务模式
				if err != nil {
					b.Fatal(err)
				}

				// 在事务中执行CRUD操作
				wallet := &OwWallet{
					Id:           utils.NextIID(),
					AppID:        "pool_bench_app_" + utils.RandStr(4),
					WalletID:     "pool_bench_wallet_" + utils.RandStr(6),
					Alias:        "pool_bench_alias",
					IsTrust:      1,
					PasswordType: 1,
					Password:     []byte("pool_bench_password"),
					AuthKey:      "pool_bench_auth_" + utils.RandStr(8),
					RootPath:     "/pool/bench/path",
					AccountIndex: 0,
					Keystore:     `{"pool":"data"}`,
					Applytime:    now,
					Succtime:     now,
					Dealstate:    1,
					Ctime:        now,
					Utime:        now,
					State:        1,
				}

				if err := db.Save(wallet); err != nil {
					b.Error(err)
					db.Close()
					continue
				}

				wallet.Alias = "updated_pool_bench_alias"
				if err := db.Update(wallet); err != nil {
					b.Error(err)
					db.Close()
					continue
				}

				// 提交事务
				if err := db.Close(); err != nil {
					b.Error(err)
				}
			}
		})
	})
}

// BenchmarkMysqlLargeDataset 大数据集操作基准测试
// 测试1000条记录大数据集下的查询和聚合操作性能，评估系统扩展性
// 包含两个子测试：大数据集查询和大数据集聚合统计
func BenchmarkMysqlLargeDataset(b *testing.B) {
	initMysqlDB()

	const datasetSize = 1000 // 测试1000条记录的大数据集

	// 预先准备大数据集
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		b.Fatal(err)
	}

	now := utils.UnixMilli()
	var largeDataset []sqlc.Object
	for i := 0; i < datasetSize; i++ {
		wallet := &OwWallet{
			Id:           utils.NextIID(),
			AppID:        fmt.Sprintf("large_bench_app_%d", i%50), // 重复的appID用于分组查询
			WalletID:     fmt.Sprintf("large_bench_wallet_%d", i),
			Alias:        fmt.Sprintf("large_bench_alias_%d", i),
			IsTrust:      int64(i % 2),
			PasswordType: 1,
			Password:     []byte(fmt.Sprintf("large_bench_password_%d", i)),
			AuthKey:      fmt.Sprintf("large_bench_auth_%d", i),
			RootPath:     fmt.Sprintf("/large/bench/path/%d", i),
			AccountIndex: int64(i % 20),
			Keystore:     fmt.Sprintf(`{"large":"data_%d"}`, i),
			Applytime:    now,
			Succtime:     now,
			Dealstate:    int64(i % 3),
			Ctime:        now,
			Utime:        now,
			State:        1,
		}
		largeDataset = append(largeDataset, wallet)
	}

	// 保存大数据集
	if err := db.Save(largeDataset...); err != nil {
		b.Fatal(err)
	}
	db.Close()

	b.Run("LargeDatasetQuery", func(b *testing.B) { // 大数据集查询性能
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					b.Fatal(err)
				}

				var results []*OwWallet
				// 查询大数据集中的记录
				if err := db.FindList(sqlc.M(&OwWallet{}).
					Like("appID", "large_bench_app_%").
					Orderby("id", sqlc.DESC_).
					Limit(1, 100), &results); err != nil {
					b.Error(err)
				}
				db.Close()
			}
		})
	})

	b.Run("LargeDatasetAggregation", func(b *testing.B) { // 大数据集聚合查询性能
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					b.Fatal(err)
				}

				// 执行聚合查询
				count, err := db.Count(sqlc.M(&OwWallet{}).
					Like("appID", "large_bench_app_%").
					Eq("state", 1))

				if err != nil {
					b.Error(err)
				}
				_ = count // 使用结果避免编译器优化
				db.Close()
			}
		})
	})
}

// BenchmarkMysqlMemoryUsage 内存使用基准测试
// 测试ORM在大量数据处理时的内存分配和GC效率，评估内存使用模式
// 包含两个子测试：内存高效查询(预分配容量)和内存密集查询(动态扩容)
func BenchmarkMysqlMemoryUsage(b *testing.B) {
	initMysqlDB()

	b.Run("MemoryEfficientQuery", func(b *testing.B) { // 内存高效查询测试
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					b.Fatal(err)
				}

				// 使用预分配的slice容量
				results := make([]*OwWallet, 0, 100)
				if err := db.FindList(sqlc.M(&OwWallet{}).Limit(1, 100), &results); err != nil {
					b.Error(err)
				}
				db.Close()

				// 手动释放引用，帮助GC
				results = nil
			}
		})
	})

	b.Run("MemoryIntensiveQuery", func(b *testing.B) { // 内存密集查询测试
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					b.Fatal(err)
				}

				// 查询大量数据
				var results []*OwWallet
				if err := db.FindList(sqlc.M(&OwWallet{}).Limit(1, 500), &results); err != nil {
					b.Error(err)
				}
				db.Close()

				// 强制GC（仅用于基准测试）
				// runtime.GC()
			}
		})
	})
}
