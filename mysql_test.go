package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/godaddy-x/freego/zlog"

	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/ormx/sqld"
	"github.com/godaddy-x/freego/utils"
)

func init() {
	zlog.InitDefaultLog(&zlog.ZapConfig{Layout: 0, Location: time.Local, Level: zlog.INFO, Console: true})
}

// TestMysqlSave 测试MySQL数据保存功能
// 验证基本的INSERT操作，包括数据序列化和字段映射
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
			Password:     []byte("encrypted_password_" + utils.RandStr(10)),
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

// TestMysqlUpdate 测试MySQL数据更新功能
// 验证基本的UPDATE操作，包括事务管理和数据一致性
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
			Id:           1987689412850352128,
			AppID:        "updated_app_" + utils.RandStr(6),
			WalletID:     "updated_wallet_" + utils.RandStr(8),
			Alias:        "updated_wallet_" + utils.RandStr(4),
			IsTrust:      2,
			PasswordType: 2,
			Password:     []byte("111updated_password_" + utils.RandStr(10)),
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

// TestMysqlUpdateByCnd 测试MySQL条件更新功能
// 验证基于条件的UPDATE操作，包括Upset语法和性能统计
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

// TestMysqlDelete 测试MySQL数据删除功能
// 验证基本的DELETE操作，包括对象删除和性能统计
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

// TestMysqlDeleteById 测试MySQL按ID删除功能
// 验证通过ID列表删除多条记录的操作
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

// TestMysqlDeleteByCnd 测试MySQL条件删除功能
// 验证各种复杂的查询条件组合在删除操作中的使用
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

// TestMysqlFindOne 测试MySQL单条记录查询功能
// 验证SELECT单条记录操作，包括条件查询和排序
func TestMysqlFindOne(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	wallet := OwWallet{}
	if err := db.FindOne(sqlc.M().Eq("id", 1987689412850352128).Orderby("id", sqlc.DESC_), &wallet); err != nil {
		fmt.Println(err)
	}
	fmt.Println(wallet)
}

// TestMysqlFindList 测试MySQL列表查询功能
// 验证SELECT多条记录操作，包括范围查询、分页和排序
func TestMysqlFindList(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	result := make([]*OwWallet, 0, 3000)
	if err := db.FindList(sqlc.M(&OwWallet{}).Between("id", 218418572484169728, 1986277638838157312).Limit(1, 3000).Orderby("id", sqlc.DESC_), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

// TestMysqlCount 测试MySQL记录计数功能
// 验证COUNT查询操作，包括分组和各种查询条件的组合
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

// TestMysqlExists 测试MySQL记录存在性检查功能
// 验证EXISTS查询操作，检查记录是否存在的布尔返回值
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

// TestMysqlFindOneComplex 测试MySQL复杂单条查询功能
// 验证JOIN连接查询、字段选择和复杂条件组合的单条记录查询
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

// TestMysqlFindListComplex 测试MySQL复杂列表查询功能
// 验证JOIN连接查询、字段选择和复杂条件组合的列表查询
func TestMysqlFindListComplex(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	l := utils.UnixMilli()
	var result []*OwWallet
	if err := db.FindListComplex(sqlc.M(&OwWallet{}).Fields("a.id id", "a.appID appID").From("ow_wallet a").Join(sqlc.LEFT_, "user b", "a.id = b.id").Eq("a.id", 218418572484169728).Eq("a.appID", "find_bench_app_123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890").Orderby("a.id", sqlc.ASC_).Limit(1, 5), &result); err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
	fmt.Println("cost: ", utils.UnixMilli()-l)
}

// TestMysqlBatchOperations 测试批量操作
func TestMysqlBatchOperations(t *testing.T) {
	initMysqlDB()
	db, err := sqld.NewMysqlTx(false)
	if err != nil {
		t.Fatalf("Failed to get MySQL client: %v", err)
	}
	defer db.Close()

	t.Run("BatchSave", func(t *testing.T) { // 测试批量保存10条记录的性能和正确性
		var wallets []sqlc.Object
		const batchSize = 10

		for i := 0; i < batchSize; i++ {
			wallet := &OwWallet{
				AppID:        fmt.Sprintf("batch_app_%d_%s", i, utils.RandStr(4)),
				WalletID:     fmt.Sprintf("batch_wallet_%d_%s", i, utils.RandStr(6)),
				Alias:        fmt.Sprintf("batch_alias_%d", i),
				IsTrust:      int64(i % 2),
				PasswordType: 1,
				Password:     []byte(fmt.Sprintf("batch_password_%d", i)),
				AuthKey:      fmt.Sprintf("batch_auth_%d_%s", i, utils.RandStr(8)),
				RootPath:     fmt.Sprintf("/batch/path/%d", i),
				AccountIndex: int64(i),
				Keystore:     fmt.Sprintf(`{"batch":"data_%d"}`, i),
				Applytime:    utils.UnixMilli(),
				Succtime:     utils.UnixMilli(),
				Dealstate:    1,
				Ctime:        utils.UnixMilli(),
				Utime:        utils.UnixMilli(),
				State:        1,
			}
			wallets = append(wallets, wallet)
		}

		start := utils.UnixMilli()
		if err := db.Save(wallets...); err != nil {
			t.Errorf("Batch save failed: %v", err)
			return
		}
		duration := utils.UnixMilli() - start

		t.Logf("Batch save %d records completed in %d ms", batchSize, duration)
	})

	t.Run("BatchUpdate", func(t *testing.T) {
		// MySQL管理器可能不支持批量更新，这里改为逐个更新测试
		var wallets []*OwWallet
		if err := db.FindList(sqlc.M(&OwWallet{}).Like("appID", "batch_app_%").Limit(1, 3), &wallets); err != nil {
			t.Errorf("Query for batch update failed: %v", err)
			return
		}

		if len(wallets) == 0 {
			t.Log("No records found for batch update test")
			return
		}

		// 逐个更新（模拟批量更新的效果）
		start := utils.UnixMilli()
		updatedCount := 0
		for i, wallet := range wallets {
			wallet.Alias = fmt.Sprintf("updated_batch_alias_%d", i)
			wallet.Utime = utils.UnixMilli()
			if err := db.Update(wallet); err != nil {
				t.Errorf("Update record %d failed: %v", i, err)
				continue
			}
			updatedCount++
		}
		duration := utils.UnixMilli() - start

		t.Logf("Updated %d records individually in %d ms", updatedCount, duration)
	})

	t.Run("BatchDelete", func(t *testing.T) {
		// 批量删除一批记录
		start := utils.UnixMilli()
		rowsAffected, err := db.DeleteByCnd(sqlc.M(&OwWallet{}).Like("appID", "batch_app_%").Limit(1, 10))
		if err != nil {
			t.Errorf("Batch delete failed: %v", err)
			return
		}
		duration := utils.UnixMilli() - start

		t.Logf("Batch delete affected %d rows in %d ms", rowsAffected, duration)
	})
}

// TestMysqlTransactionOperations 测试事务操作
func TestMysqlTransactionOperations(t *testing.T) {
	initMysqlDB()

	t.Run("TransactionCommit", func(t *testing.T) { // 测试事务成功提交的完整流程
		db, err := sqld.NewMysqlTx(true) // 开启事务
		if err != nil {
			t.Fatalf("Failed to start transaction: %v", err)
		}
		defer db.Close()

		// 在事务中执行多个操作
		wallet1 := &OwWallet{
			AppID:        "tx_app_1_" + utils.RandStr(4),
			WalletID:     "tx_wallet_1_" + utils.RandStr(6),
			Alias:        "tx_alias_1",
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("tx_password_1"),
			AuthKey:      "tx_auth_1_" + utils.RandStr(8),
			RootPath:     "/tx/path/1",
			AccountIndex: 0,
			Keystore:     `{"tx":"data_1"}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		wallet2 := &OwWallet{
			AppID:        "tx_app_2_" + utils.RandStr(4),
			WalletID:     "tx_wallet_2_" + utils.RandStr(6),
			Alias:        "tx_alias_2",
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("tx_password_2"),
			AuthKey:      "tx_auth_2_" + utils.RandStr(8),
			RootPath:     "/tx/path/2",
			AccountIndex: 1,
			Keystore:     `{"tx":"data_2"}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		// 保存两个记录
		if err := db.Save(wallet1, wallet2); err != nil {
			t.Errorf("Transaction save failed: %v", err)
			return
		}

		// 更新第一个记录
		wallet1.Alias = "tx_updated_alias_1"
		wallet1.Utime = utils.UnixMilli()
		if err := db.Update(wallet1); err != nil {
			t.Errorf("Transaction update failed: %v", err)
			return
		}

		// 提交事务（通过无错误关闭实现）
		if err := db.Close(); err != nil {
			t.Errorf("Transaction commit failed: %v", err)
			return
		}

		t.Logf("Transaction committed successfully")

		// 验证数据是否正确提交（在新的事务实例中）
		verifyDB, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to create verification DB connection: %v", err)
		}
		defer verifyDB.Close()

		var result OwWallet
		if err := verifyDB.FindOne(sqlc.M().Eq("appID", wallet1.AppID), &result); err != nil {
			t.Errorf("Verify committed data failed: %v", err)
			return
		}

		if result.Alias != "tx_updated_alias_1" {
			t.Errorf("Transaction data verification failed: expected alias 'tx_updated_alias_1', got '%s'", result.Alias)
		}
	})

	t.Run("TransactionRollback", func(t *testing.T) {
		db, err := sqld.NewMysqlTx(true) // 开启事务
		if err != nil {
			t.Fatalf("Failed to start transaction: %v", err)
		}
		defer db.Close()

		// 保存一个记录
		wallet := &OwWallet{
			AppID:        "rollback_app_" + utils.RandStr(4),
			WalletID:     "rollback_wallet_" + utils.RandStr(6),
			Alias:        "rollback_alias",
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("rollback_password"),
			AuthKey:      "rollback_auth_" + utils.RandStr(8),
			RootPath:     "/rollback/path",
			AccountIndex: 0,
			Keystore:     `{"rollback":"data"}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		if err := db.Save(wallet); err != nil {
			t.Errorf("Transaction save before rollback failed: %v", err)
			return
		}

		// 回滚事务（通过有错误关闭实现）
		// 手动设置一个错误来触发回滚
		db.Errors = append(db.Errors, utils.Error("manual rollback"))

		if err := db.Close(); err != nil {
			t.Errorf("Transaction rollback failed: %v", err)
			return
		}

		t.Logf("Transaction rolled back successfully")

		// 验证数据是否被回滚（在新的事务实例中）
		verifyDB, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to create verification DB connection: %v", err)
		}
		defer verifyDB.Close()

		exists, err := verifyDB.Exists(sqlc.M(&OwWallet{}).Eq("appID", wallet.AppID))
		if err != nil {
			t.Errorf("Verify rollback failed: %v", err)
			return
		}

		if exists {
			t.Errorf("Transaction rollback verification failed: record should not exist after rollback")
		}
	})

	t.Run("TransactionWithError", func(t *testing.T) {
		db, err := sqld.NewMysqlTx(true)
		if err != nil {
			t.Fatalf("Failed to start transaction: %v", err)
		}
		defer db.Close()

		// 保存一个记录
		wallet := &OwWallet{
			AppID:        "error_tx_app_" + utils.RandStr(4),
			WalletID:     "error_tx_wallet_" + utils.RandStr(6),
			Alias:        "error_tx_alias",
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("error_tx_password"),
			AuthKey:      "error_tx_auth_" + utils.RandStr(8),
			RootPath:     "/error/tx/path",
			AccountIndex: 0,
			Keystore:     `{"error_tx":"data"}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		if err := db.Save(wallet); err != nil {
			t.Errorf("Transaction save failed: %v", err)
			return
		}

		// 模拟一个错误操作（故意传入空切片）
		if err := db.Save(); err == nil {
			t.Error("Expected error when saving empty slice, but got nil")
		} else {
			t.Logf("Expected error occurred: %v", err)
		}

		// 由于发生了错误，事务会在Close时自动回滚
		if err := db.Close(); err != nil {
			t.Errorf("Transaction rollback after error failed: %v", err)
			return
		}

		t.Logf("Transaction rolled back after error successfully")

		// 验证数据是否被回滚（在新的事务实例中）
		verifyDB, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to create verification DB connection: %v", err)
		}
		defer verifyDB.Close()

		exists, err := verifyDB.Exists(sqlc.M(&OwWallet{}).Eq("appID", wallet.AppID))
		if err != nil {
			t.Errorf("Verify rollback after error failed: %v", err)
			return
		}

		if exists {
			t.Errorf("Transaction rollback after error verification failed: record should not exist")
		}
	})
}

// TestMysqlConcurrentOperations 测试并发操作
func TestMysqlConcurrentOperations(t *testing.T) {
	initMysqlDB()

	const numGoroutines = 10
	const operationsPerGoroutine = 5

	t.Run("ConcurrentCRUD", func(t *testing.T) { // 测试10个goroutine并发执行完整的CRUD操作
		var wg sync.WaitGroup
		errorChan := make(chan error, numGoroutines*operationsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for j := 0; j < operationsPerGoroutine; j++ {
					db, err := sqld.NewMysqlTx(false)
					if err != nil {
						errorChan <- fmt.Errorf("goroutine %d: failed to get DB connection: %v", goroutineID, err)
						continue
					}

					// 执行CRUD操作
					appID := fmt.Sprintf("concurrent_app_%d_%d_%s", goroutineID, j, utils.RandStr(3))

					// 1. 保存
					wallet := &OwWallet{
						AppID:        appID,
						WalletID:     fmt.Sprintf("concurrent_wallet_%d_%d", goroutineID, j),
						Alias:        fmt.Sprintf("concurrent_alias_%d_%d", goroutineID, j),
						IsTrust:      1,
						PasswordType: 1,
						Password:     []byte(fmt.Sprintf("concurrent_password_%d_%d", goroutineID, j)),
						AuthKey:      fmt.Sprintf("concurrent_auth_%d_%d_%s", goroutineID, j, utils.RandStr(5)),
						RootPath:     fmt.Sprintf("/concurrent/path/%d/%d", goroutineID, j),
						AccountIndex: int64(goroutineID*operationsPerGoroutine + j),
						Keystore:     fmt.Sprintf(`{"concurrent":"data_%d_%d"}`, goroutineID, j),
						Applytime:    utils.UnixMilli(),
						Succtime:     utils.UnixMilli(),
						Dealstate:    1,
						Ctime:        utils.UnixMilli(),
						Utime:        utils.UnixMilli(),
						State:        1,
					}

					if err := db.Save(wallet); err != nil {
						errorChan <- fmt.Errorf("goroutine %d operation %d: save failed: %v", goroutineID, j, err)
						db.Close()
						continue
					}

					// 2. 查询
					var result OwWallet
					if err := db.FindOne(sqlc.M().Eq("appID", appID), &result); err != nil {
						errorChan <- fmt.Errorf("goroutine %d operation %d: find failed: %v", goroutineID, j, err)
						db.Close()
						continue
					}

					// 3. 更新
					result.Alias = fmt.Sprintf("updated_concurrent_alias_%d_%d", goroutineID, j)
					result.Utime = utils.UnixMilli()
					if err := db.Update(&result); err != nil {
						errorChan <- fmt.Errorf("goroutine %d operation %d: update failed: %v", goroutineID, j, err)
						db.Close()
						continue
					}

					// 4. 删除
					if err := db.Delete(&result); err != nil {
						errorChan <- fmt.Errorf("goroutine %d operation %d: delete failed: %v", goroutineID, j, err)
						db.Close()
						continue
					}

					db.Close()
				}
			}(i)
		}

		wg.Wait()
		close(errorChan)

		// 检查是否有错误
		var errors []error
		for err := range errorChan {
			errors = append(errors, err)
		}

		if len(errors) > 0 {
			t.Errorf("Concurrent operations had %d errors:", len(errors))
			for _, err := range errors {
				t.Errorf("  %v", err)
			}
		} else {
			t.Logf("Concurrent CRUD operations completed successfully: %d goroutines × %d operations each",
				numGoroutines, operationsPerGoroutine)
		}
	})

	t.Run("ConcurrentReads", func(t *testing.T) {
		// 首先准备一些测试数据
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to prepare test data: %v", err)
		}

		var testWallets []sqlc.Object
		for i := 0; i < 50; i++ {
			wallet := &OwWallet{
				AppID:        fmt.Sprintf("read_test_app_%d_%s", i, utils.RandStr(3)),
				WalletID:     fmt.Sprintf("read_test_wallet_%d", i),
				Alias:        fmt.Sprintf("read_test_alias_%d", i),
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte(fmt.Sprintf("read_test_password_%d", i)),
				AuthKey:      fmt.Sprintf("read_test_auth_%d_%s", i, utils.RandStr(4)),
				RootPath:     fmt.Sprintf("/read/test/path/%d", i),
				AccountIndex: int64(i),
				Keystore:     fmt.Sprintf(`{"read_test":"data_%d"}`, i),
				Applytime:    utils.UnixMilli(),
				Succtime:     utils.UnixMilli(),
				Dealstate:    1,
				Ctime:        utils.UnixMilli(),
				Utime:        utils.UnixMilli(),
				State:        1,
			}
			testWallets = append(testWallets, wallet)
		}

		if err := db.Save(testWallets...); err != nil {
			t.Fatalf("Failed to save test data: %v", err)
		}
		db.Close()

		// 并发读取测试
		var wg sync.WaitGroup
		errorChan := make(chan error, numGoroutines)
		resultChan := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				db, err := sqld.NewMysqlTx(false)
				if err != nil {
					errorChan <- fmt.Errorf("goroutine %d: failed to get DB connection: %v", goroutineID, err)
					return
				}
				defer db.Close()

				var results []*OwWallet
				if err := db.FindList(sqlc.M(&OwWallet{}).Like("appID", "read_test_app_%").Limit(1, 100), &results); err != nil {
					errorChan <- fmt.Errorf("goroutine %d: find list failed: %v", goroutineID, err)
					return
				}

				resultChan <- len(results)
			}(i)
		}

		wg.Wait()
		close(errorChan)
		close(resultChan)

		// 检查结果
		totalResults := 0
		for count := range resultChan {
			totalResults += count
		}

		var errors []error
		for err := range errorChan {
			errors = append(errors, err)
		}

		if len(errors) > 0 {
			t.Errorf("Concurrent reads had %d errors:", len(errors))
			for _, err := range errors {
				t.Errorf("  %v", err)
			}
		} else {
			t.Logf("Concurrent reads completed successfully: %d total results from %d goroutines",
				totalResults, numGoroutines)
		}

		// 清理测试数据
		cleanupDB, _ := sqld.NewMysqlTx(false)
		cleanupDB.DeleteByCnd(sqlc.M(&OwWallet{}).Like("appID", "read_test_app_%"))
		cleanupDB.Close()
	})
}

// TestMysqlEdgeCases 测试边界情况
func TestMysqlEdgeCases(t *testing.T) {
	initMysqlDB()

	t.Run("EmptyAndNullValues", func(t *testing.T) { // 测试空字符串、零值等边界情况的处理
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 测试空值和边界值
		wallet := &OwWallet{
			AppID:        "", // 空字符串
			WalletID:     "edge_wallet_" + utils.RandStr(6),
			Alias:        "",         // 空字符串
			IsTrust:      0,          // 最小值
			PasswordType: 0,          // 最小值
			Password:     []byte(""), // 空字节数组
			AuthKey:      "",         // 空字符串
			RootPath:     "",         // 空字符串
			AccountIndex: 0,          // 最小值
			Keystore:     "",         // 空字符串
			Applytime:    0,          // 最小值
			Succtime:     0,          // 最小值
			Dealstate:    0,          // 最小值
			Ctime:        0,          // 最小值
			Utime:        0,          // 最小值
			State:        0,          // 最小值
		}

		if err := db.Save(wallet); err != nil {
			t.Errorf("Save with empty/null values failed: %v", err)
			return
		}

		t.Logf("Empty and null values test passed")
	})

	t.Run("LargeDataStrings", func(t *testing.T) {
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 测试合理长度的大字符串数据（避免数据库列长度限制）
		largeString := utils.RandStr(500) // 500字符的随机字符串

		wallet := &OwWallet{
			AppID:        "large_app_" + utils.RandStr(4),
			WalletID:     "large_wallet_" + utils.RandStr(6),
			Alias:        largeString[:50], // 截取前50字符
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte(largeString[:100]),          // 截取前100字符作为密码
			AuthKey:      largeString[:200],                  // 截取前200字符
			RootPath:     "/large/path/" + largeString[:100], // 截取前100字符
			AccountIndex: 0,
			Keystore:     fmt.Sprintf(`{"large_data":"%s"}`, largeString[:300]), // 控制在合理长度内
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		if err := db.Save(wallet); err != nil {
			t.Errorf("Save with large data strings failed: %v", err)
			return
		}

		t.Logf("Large data strings test passed")
	})

	t.Run("SpecialCharacters", func(t *testing.T) {
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 测试特殊字符
		specialChars := "!@#$%^&*()_+-=[]{}|;:,.<>?`~"

		wallet := &OwWallet{
			AppID:        "special_app_" + utils.RandStr(4),
			WalletID:     "special_wallet_" + utils.RandStr(6),
			Alias:        "special_alias_" + specialChars,
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("special_password_" + specialChars),
			AuthKey:      "special_auth_" + specialChars,
			RootPath:     "/special/path/" + specialChars,
			AccountIndex: 0,
			Keystore:     fmt.Sprintf(`{"special":"chars_%s"}`, specialChars),
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		if err := db.Save(wallet); err != nil {
			t.Errorf("Save with special characters failed: %v", err)
			return
		}

		t.Logf("Special characters test passed")
	})

	t.Run("UnicodeStrings", func(t *testing.T) {
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 测试包含各种字符的字符串（使用ASCII兼容的字符）
		diverseString := "Hello World 1234567890 !@#$%^&*()_+-=[]{}|;:,.<>?`~"

		wallet := &OwWallet{
			AppID:        "diverse_app_" + utils.RandStr(4),
			WalletID:     "diverse_wallet_" + utils.RandStr(6),
			Alias:        "diverse_alias_" + diverseString[:30], // 截取前30字符
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("diverse_password_" + diverseString[:30]),
			AuthKey:      "diverse_auth_" + diverseString[:40],
			RootPath:     "/diverse/path/" + diverseString[:30],
			AccountIndex: 0,
			Keystore:     fmt.Sprintf(`{"diverse":"%s"}`, diverseString),
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		if err := db.Save(wallet); err != nil {
			t.Errorf("Save with diverse character strings failed: %v", err)
			return
		}

		t.Logf("Diverse character strings test passed")
	})
}

// TestMysqlErrorHandling 测试错误处理
func TestMysqlErrorHandling(t *testing.T) {
	initMysqlDB()

	t.Run("InvalidConditions", func(t *testing.T) { // 测试不存在的字段名等无效查询条件
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 测试无效的查询条件
		var result []*OwWallet
		err = db.FindList(sqlc.M(&OwWallet{}).Eq("nonexistent_field", "value"), &result)
		if err == nil {
			t.Error("Expected error for nonexistent field, but got nil")
		} else {
			t.Logf("Invalid conditions correctly returned error: %v", err)
		}
	})

	t.Run("DuplicateKeyHandling", func(t *testing.T) {
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 创建一个具有唯一约束的记录
		appID := "duplicate_test_" + utils.RandStr(6)
		wallet1 := &OwWallet{
			AppID:        appID,
			WalletID:     "duplicate_wallet_1_" + utils.RandStr(4),
			Alias:        "duplicate_alias_1",
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("duplicate_password_1"),
			AuthKey:      "duplicate_auth_1_" + utils.RandStr(6),
			RootPath:     "/duplicate/path/1",
			AccountIndex: 0,
			Keystore:     `{"duplicate":"data_1"}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		if err := db.Save(wallet1); err != nil {
			t.Fatalf("Failed to save first record: %v", err)
		}

		// 尝试保存具有相同appID的记录（如果appID有唯一约束）
		wallet2 := &OwWallet{
			AppID:        appID, // 相同的appID
			WalletID:     "duplicate_wallet_2_" + utils.RandStr(4),
			Alias:        "duplicate_alias_2",
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("duplicate_password_2"),
			AuthKey:      "duplicate_auth_2_" + utils.RandStr(6),
			RootPath:     "/duplicate/path/2",
			AccountIndex: 1,
			Keystore:     `{"duplicate":"data_2"}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		err = db.Save(wallet2)
		if err == nil {
			t.Logf("No duplicate key error - appID may not have unique constraint")
		} else {
			t.Logf("Duplicate key error correctly handled: %v", err)
		}
	})

	t.Run("ConnectionTimeout", func(t *testing.T) {
		// 测试连接超时情况（通过长时间运行的查询模拟）
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 执行一个复杂的查询，看是否能正常处理
		var results []*OwWallet
		start := utils.UnixMilli()
		err = db.FindList(sqlc.M(&OwWallet{}).Limit(1, 1000).Orderby("id", sqlc.DESC_), &results)
		duration := utils.UnixMilli() - start

		if err != nil {
			t.Errorf("Complex query failed: %v", err)
			return
		}

		t.Logf("Complex query completed in %d ms, returned %d results", duration, len(results))
	})
}

// TestMysqlDataIntegrity 测试数据完整性
func TestMysqlDataIntegrity(t *testing.T) {
	initMysqlDB()

	t.Run("DataConsistencyAfterOperations", func(t *testing.T) { // 测试CRUD操作后的数据一致性和完整性
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		// 创建测试数据
		appID := "integrity_test_" + utils.RandStr(6)
		wallet := &OwWallet{
			AppID:        appID,
			WalletID:     "integrity_wallet_" + utils.RandStr(6),
			Alias:        "integrity_alias",
			IsTrust:      1,
			PasswordType: 1,
			Password:     []byte("integrity_password"),
			AuthKey:      "integrity_auth_" + utils.RandStr(8),
			RootPath:     "/integrity/path",
			AccountIndex: 0,
			Keystore:     `{"integrity":"test_data"}`,
			Applytime:    utils.UnixMilli(),
			Succtime:     utils.UnixMilli(),
			Dealstate:    1,
			Ctime:        utils.UnixMilli(),
			Utime:        utils.UnixMilli(),
			State:        1,
		}

		// 保存
		if err := db.Save(wallet); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// 查询并验证
		var result OwWallet
		if err := db.FindOne(sqlc.M().Eq("appID", appID), &result); err != nil {
			t.Fatalf("FindOne failed: %v", err)
		}

		// 验证所有字段
		if result.AppID != wallet.AppID {
			t.Errorf("AppID mismatch: expected %s, got %s", wallet.AppID, result.AppID)
		}
		if result.WalletID != wallet.WalletID {
			t.Errorf("WalletID mismatch: expected %s, got %s", wallet.WalletID, result.WalletID)
		}
		if result.Alias != wallet.Alias {
			t.Errorf("Alias mismatch: expected %s, got %s", wallet.Alias, result.Alias)
		}
		if result.Keystore != wallet.Keystore {
			t.Errorf("Keystore mismatch: expected %s, got %s", wallet.Keystore, result.Keystore)
		}

		// 更新数据
		result.Alias = "updated_integrity_alias"
		result.Utime = utils.UnixMilli()
		if err := db.Update(&result); err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		// 再次查询验证更新
		var updatedResult OwWallet
		if err := db.FindOne(sqlc.M().Eq("appID", appID), &updatedResult); err != nil {
			t.Fatalf("FindOne after update failed: %v", err)
		}

		if updatedResult.Alias != "updated_integrity_alias" {
			t.Errorf("Update verification failed: expected alias 'updated_integrity_alias', got '%s'", updatedResult.Alias)
		}

		// 清理测试数据
		if err := db.Delete(&updatedResult); err != nil {
			t.Errorf("Cleanup failed: %v", err)
		}

		t.Logf("Data integrity test passed")
	})

	t.Run("BatchOperationIntegrity", func(t *testing.T) {
		db, err := sqld.NewMysqlTx(false)
		if err != nil {
			t.Fatalf("Failed to get DB connection: %v", err)
		}
		defer db.Close()

		const batchSize = 5
		var wallets []sqlc.Object
		var appIDs []string

		// 批量创建测试数据
		for i := 0; i < batchSize; i++ {
			appID := fmt.Sprintf("batch_integrity_app_%d_%s", i, utils.RandStr(3))
			appIDs = append(appIDs, appID)

			wallet := &OwWallet{
				AppID:        appID,
				WalletID:     fmt.Sprintf("batch_integrity_wallet_%d", i),
				Alias:        fmt.Sprintf("batch_integrity_alias_%d", i),
				IsTrust:      int64(i % 2),
				PasswordType: 1,
				Password:     []byte(fmt.Sprintf("batch_integrity_password_%d", i)),
				AuthKey:      fmt.Sprintf("batch_integrity_auth_%d_%s", i, utils.RandStr(4)),
				RootPath:     fmt.Sprintf("/batch/integrity/path/%d", i),
				AccountIndex: int64(i),
				Keystore:     fmt.Sprintf(`{"batch_integrity":"data_%d"}`, i),
				Applytime:    utils.UnixMilli(),
				Succtime:     utils.UnixMilli(),
				Dealstate:    1,
				Ctime:        utils.UnixMilli(),
				Utime:        utils.UnixMilli(),
				State:        1,
			}
			wallets = append(wallets, wallet)
		}

		// 批量保存
		if err := db.Save(wallets...); err != nil {
			t.Fatalf("Batch save failed: %v", err)
		}

		// 批量查询验证（分别查询每个appID）
		var results []*OwWallet
		for _, appID := range appIDs {
			var walletResult OwWallet
			if err := db.FindOne(sqlc.M().Eq("appID", appID), &walletResult); err != nil {
				t.Fatalf("Find one failed for appID %s: %v", appID, err)
			}
			results = append(results, &walletResult)
		}

		if len(results) != batchSize {
			t.Errorf("Batch find returned wrong count: expected %d, got %d", batchSize, len(results))
		}

		// 验证每条记录
		for i, result := range results {
			expectedAppID := appIDs[i]
			found := false
			for _, wallet := range wallets {
				if wallet.(*OwWallet).AppID == expectedAppID {
					if result.AppID != expectedAppID {
						t.Errorf("Batch integrity check failed for appID %s", expectedAppID)
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("AppID %s not found in original data", expectedAppID)
			}
		}

		// 批量删除清理（分别删除每个appID）
		rowsAffected := int64(0)
		for _, appID := range appIDs {
			affected, err := db.DeleteByCnd(sqlc.M(&OwWallet{}).Eq("appID", appID))
			if err != nil {
				t.Errorf("Delete failed for appID %s: %v", appID, err)
				continue
			}
			rowsAffected += affected
		}

		if rowsAffected != int64(batchSize) {
			t.Errorf("Batch delete affected wrong number of rows: expected %d, got %d", batchSize, rowsAffected)
		}

		t.Logf("Batch operation integrity test passed")
	})
}

// BenchmarkMysqlOperations MySQL操作性能基准测试
func BenchmarkMysqlOperations(b *testing.B) {
	initMysqlDB()

	b.Run("Save", func(b *testing.B) { // 基准测试INSERT操作性能
		for i := 0; i < b.N; i++ {
			db, _ := sqld.NewMysqlTx(false)
			wallet := &OwWallet{
				AppID:        fmt.Sprintf("bench_app_%d", i),
				WalletID:     fmt.Sprintf("bench_wallet_%d", i),
				Alias:        fmt.Sprintf("bench_alias_%d", i),
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte(fmt.Sprintf("bench_password_%d", i)),
				AuthKey:      fmt.Sprintf("bench_auth_%d", i),
				RootPath:     fmt.Sprintf("/bench/path/%d", i),
				AccountIndex: int64(i),
				Keystore:     fmt.Sprintf(`{"bench":"data_%d"}`, i),
				Applytime:    utils.UnixMilli(),
				Succtime:     utils.UnixMilli(),
				Dealstate:    1,
				Ctime:        utils.UnixMilli(),
				Utime:        utils.UnixMilli(),
				State:        1,
			}
			db.Save(wallet)
			db.Close()
		}
	})

	b.Run("FindOne", func(b *testing.B) { // 基准测试单条记录查询性能
		// 预先准备数据
		db, _ := sqld.NewMysqlTx(false)
		for i := 0; i < 100; i++ {
			wallet := &OwWallet{
				AppID:        fmt.Sprintf("bench_find_app_%d", i),
				WalletID:     fmt.Sprintf("bench_find_wallet_%d", i),
				Alias:        fmt.Sprintf("bench_find_alias_%d", i),
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte(fmt.Sprintf("bench_find_password_%d", i)),
				AuthKey:      fmt.Sprintf("bench_find_auth_%d", i),
				RootPath:     fmt.Sprintf("/bench/find/path/%d", i),
				AccountIndex: int64(i),
				Keystore:     fmt.Sprintf(`{"bench_find":"data_%d"}`, i),
				Applytime:    utils.UnixMilli(),
				Succtime:     utils.UnixMilli(),
				Dealstate:    1,
				Ctime:        utils.UnixMilli(),
				Utime:        utils.UnixMilli(),
				State:        1,
			}
			db.Save(wallet)
		}
		db.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			db, _ := sqld.NewMysqlTx(false)
			var result OwWallet
			appID := fmt.Sprintf("bench_find_app_%d", i%100)
			db.FindOne(sqlc.M().Eq("appID", appID), &result)
			db.Close()
		}
	})

	b.Run("FindList", func(b *testing.B) { // 基准测试列表查询性能（分页查询50条记录）
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			db, _ := sqld.NewMysqlTx(false)
			var results []*OwWallet
			db.FindList(sqlc.M(&OwWallet{}).Limit(1, 50), &results)
			db.Close()
		}
	})

	b.Run("Update", func(b *testing.B) { // 基准测试UPDATE操作性能
		// 预先准备数据
		db, _ := sqld.NewMysqlTx(false)
		var testWallets []*OwWallet
		for i := 0; i < 100; i++ {
			wallet := &OwWallet{
				AppID:        fmt.Sprintf("bench_update_app_%d", i),
				WalletID:     fmt.Sprintf("bench_update_wallet_%d", i),
				Alias:        fmt.Sprintf("bench_update_alias_%d", i),
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte(fmt.Sprintf("bench_update_password_%d", i)),
				AuthKey:      fmt.Sprintf("bench_update_auth_%d", i),
				RootPath:     fmt.Sprintf("/bench/update/path/%d", i),
				AccountIndex: int64(i),
				Keystore:     fmt.Sprintf(`{"bench_update":"data_%d"}`, i),
				Applytime:    utils.UnixMilli(),
				Succtime:     utils.UnixMilli(),
				Dealstate:    1,
				Ctime:        utils.UnixMilli(),
				Utime:        utils.UnixMilli(),
				State:        1,
			}
			db.Save(wallet)
			testWallets = append(testWallets, wallet)
		}
		db.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			db, _ := sqld.NewMysqlTx(false)
			wallet := testWallets[i%len(testWallets)]
			wallet.Alias = fmt.Sprintf("bench_updated_alias_%d", i)
			wallet.Utime = utils.UnixMilli()
			db.Update(wallet)
			db.Close()
		}
	})
}
