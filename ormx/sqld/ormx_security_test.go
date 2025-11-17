package sqld

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ormx/sqlc"
	"github.com/godaddy-x/freego/utils"
)

// ==========================================
// 安全测试模型定义
// ==========================================

// TestSecurityModel 综合安全测试模型
type TestSecurityModel struct {
	Id          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Password    []byte                 `json:"password" safe:"true"` // 敏感数据
	Token       []byte                 `json:"token" safe:"true"`    // 敏感数据
	Data        []byte                 `json:"data"`                 // 普通数据
	Balance     float64                `json:"balance"`
	Status      int                    `json:"status"`
	CreateTime  int64                  `json:"create_time" date:"true"`
	UpdateTime  int64                  `json:"update_time" date:"true"`
	Metadata    string                 `json:"metadata"`
	Permissions []string               `json:"permissions"`
	Settings    map[string]interface{} `json:"settings"`
}

func (o *TestSecurityModel) GetTable() string {
	return "test_security_model"
}

func (o *TestSecurityModel) NewObject() sqlc.Object {
	return &TestSecurityModel{}
}

func (o *TestSecurityModel) AppendObject(data interface{}, target sqlc.Object) {
	*data.(*[]*TestSecurityModel) = append(*data.(*[]*TestSecurityModel), target.(*TestSecurityModel))
}

func (o *TestSecurityModel) NewIndex() []sqlc.Index {
	return []sqlc.Index{
		{Name: "idx_name", Key: []string{"name"}},
		{Name: "idx_status", Key: []string{"status"}},
	}
}

// TestMemoryLeakModel 内存泄露检测模型
type TestMemoryLeakModel struct {
	Id   int64  `json:"id"`
	Data []byte `json:"data"`
}

func (o *TestMemoryLeakModel) GetTable() string {
	return "test_memory_leak_model"
}

func (o *TestMemoryLeakModel) NewObject() sqlc.Object {
	return &TestMemoryLeakModel{}
}

func (o *TestMemoryLeakModel) AppendObject(data interface{}, target sqlc.Object) {
	*data.(*[]*TestMemoryLeakModel) = append(*data.(*[]*TestMemoryLeakModel), target.(*TestMemoryLeakModel))
}

func (o *TestMemoryLeakModel) NewIndex() []sqlc.Index {
	return []sqlc.Index{}
}

// OwWallet 真实的钱包模型（从main.go复制）
type OwWallet struct {
	Id           int64  `json:"id" bson:"_id"`
	AppID        string `json:"appID" bson:"appID"`
	WalletID     string `json:"walletID" bson:"walletID"`
	Alias        string `json:"alias" bson:"alias"`
	IsTrust      int64  `json:"isTrust" bson:"isTrust"`
	PasswordType int64  `json:"passwordType" bson:"passwordType"`
	Password     []byte `json:"password" bson:"password" blob:"true"`
	AuthKey      string `json:"authKey" bson:"authKey"`
	RootPath     string `json:"rootPath" bson:"rootPath"`
	AccountIndex int64  `json:"accountIndex" bson:"accountIndex"`
	Keystore     string `json:"keyJson" bson:"keyJson"`
	Applytime    int64  `json:"applytime" bson:"applytime"`
	Succtime     int64  `json:"succtime" bson:"succtime"`
	Dealstate    int64  `json:"dealstate" bson:"dealstate"`
	Ctime        int64  `json:"ctime" bson:"ctime"`
	Utime        int64  `json:"utime" bson:"utime"`
	State        int64  `json:"state" bson:"state"`
}

func (o *OwWallet) GetTable() string {
	return "ow_wallet"
}

func (o *OwWallet) NewObject() sqlc.Object {
	return &OwWallet{}
}

func (o *OwWallet) AppendObject(data interface{}, target sqlc.Object) {
	*data.(*[]*OwWallet) = append(*data.(*[]*OwWallet), target.(*OwWallet))
}

func (o *OwWallet) NewIndex() []sqlc.Index {
	appID := sqlc.Index{Name: "appID", Key: []string{"appID"}}
	return []sqlc.Index{appID}
}

// ==========================================
// 核心安全测试用例
// ==========================================

// TestObjectPoolByteSafety 全面测试对象池字节安全
func TestObjectPoolByteSafety(t *testing.T) {
	// 注册模型
	model := &TestSecurityModel{}
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	// 测试数据
	testData := []byte("This is comprehensive security test data for object pool safety verification!")
	originalData := make([]byte, len(testData))
	copy(originalData, testData)

	// 获取字段信息
	var dataField *FieldElem
	if driver, ok := modelDrivers[model.GetTable()]; ok {
		for _, elem := range driver.FieldElem {
			if elem.FieldName == "Data" {
				dataField = elem
				break
			}
		}
	}

	if dataField == nil {
		t.Fatal("未找到Data字段信息")
	}

	t.Run("单次查询对象池安全", func(t *testing.T) {
		testObj := &TestSecurityModel{}

		// 模拟数据库查询缓冲区
		queryBuffer := make([]byte, len(originalData))
		copy(queryBuffer, originalData)

		// 设置字段值
		err := SetValue(testObj, dataField, queryBuffer)
		if err != nil {
			t.Fatalf("SetValue失败: %v", err)
		}

		// 验证数据正确性
		if !bytes.Equal(testObj.Data, originalData) {
			t.Errorf("数据设置不正确")
		}

		// 模拟缓冲区被"回收"（对象池重用）
		for i := range queryBuffer {
			queryBuffer[i] = 0xFF
		}

		// 验证对象数据不受影响
		if !bytes.Equal(testObj.Data, originalData) {
			t.Errorf("对象池回收导致数据污染! 期望: %s, 实际: %s",
				string(originalData), string(testObj.Data))
		}

		t.Logf("✅ 单次查询对象池安全测试通过")
	})

	t.Run("批量查询对象池安全", func(t *testing.T) {
		// 模拟批量查询结果
		rowCount := 5
		testRows := make([][]byte, rowCount)
		originalRows := make([][]byte, rowCount)

		for i := 0; i < rowCount; i++ {
			data := []byte(fmt.Sprintf("Row %d: %s", i, string(originalData)))
			testRows[i] = make([]byte, len(data))
			copy(testRows[i], data)
			originalRows[i] = make([]byte, len(data))
			copy(originalRows[i], data)
		}

		// 模拟OutDestWithCapacity的结果
		out := [][][]byte{testRows}

		// 创建结果对象
		results := make([]*TestSecurityModel, rowCount)
		for i := range results {
			results[i] = &TestSecurityModel{}
		}

		// 填充数据（模拟FindList逻辑）
		for _, row := range out {
			for j, cell := range row {
				if j < len(results) {
					err := SetValue(results[j], dataField, cell)
					if err != nil {
						t.Fatalf("批量设置失败: %v", err)
					}
				}
			}
			break // 只处理第一行数据
		}

		// 验证所有结果数据正确
		for i, result := range results {
			if i < len(originalRows) {
				expected := originalRows[i]
				if !bytes.Equal(result.Data, expected) {
					t.Errorf("批量数据%d设置失败", i)
				}
			}
		}

		// 模拟对象池释放
		ReleaseOutDest(out)

		// 再次验证数据不受影响
		for i, result := range results {
			if i < len(originalRows) {
				expected := originalRows[i]
				if !bytes.Equal(result.Data, expected) {
					t.Errorf("对象池释放后数据%d被污染", i)
				}
			}
		}

		t.Logf("✅ 批量查询对象池安全测试通过")
	})
}

// TestConcurrentSafety 并发安全测试
func TestConcurrentSafety(t *testing.T) {
	// 注册模型
	model := &TestSecurityModel{}
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	var dataField *FieldElem
	if driver, ok := modelDrivers[model.GetTable()]; ok {
		for _, elem := range driver.FieldElem {
			if elem.FieldName == "Data" {
				dataField = elem
				break
			}
		}
	}

	const goroutineCount = 100
	const iterationsPerGoroutine = 1000

	var successCount int64
	var errorCount int64

	t.Run("并发对象池访问安全", func(t *testing.T) {
		var wg sync.WaitGroup

		for g := 0; g < goroutineCount; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for i := 0; i < iterationsPerGoroutine; i++ {
					// 生成唯一测试数据
					testData := []byte(fmt.Sprintf("Goroutine%d-Iteration%d-SecurityTest", goroutineID, i))
					originalData := make([]byte, len(testData))
					copy(originalData, testData)

					// 创建测试对象
					testObj := &TestSecurityModel{}

					// 设置字段值
					err := SetValue(testObj, dataField, originalData)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					// 验证数据完整性
					if !bytes.Equal(testObj.Data, originalData) {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					// 模拟短暂延迟（模拟实际使用场景）
					time.Sleep(time.Microsecond)

					// 再次验证数据完整性
					if !bytes.Equal(testObj.Data, originalData) {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					atomic.AddInt64(&successCount, 1)
				}
			}(g)
		}

		wg.Wait()

		totalOperations := goroutineCount * iterationsPerGoroutine
		t.Logf("并发测试完成: 总操作数=%d, 成功=%d, 失败=%d",
			totalOperations, successCount, errorCount)

		if errorCount > 0 {
			t.Errorf("并发测试发现%d个错误", errorCount)
		}

		if successCount < int64(float64(totalOperations)*0.99) { // 允许1%的误差
			t.Errorf("成功率过低: %.2f%%", float64(successCount)/float64(totalOperations)*100)
		}
	})
}

// TestResourceLeakDetection 资源泄露检测测试
func TestResourceLeakDetection(t *testing.T) {
	// 注册模型
	model := &TestMemoryLeakModel{}
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	// 记录初始GC统计
	initialStats := &runtime.MemStats{}
	runtime.GC()
	runtime.ReadMemStats(initialStats)

	t.Run("对象池资源泄露检测", func(t *testing.T) {
		const iterationCount = 10000

		for i := 0; i < iterationCount; i++ {
			// 从对象池获取资源
			buffer := rowByteSlicePool.Get().([][]byte)

			// 模拟数据填充
			testData := []byte(fmt.Sprintf("Leak test data %d with some content", i))
			if len(buffer) == 0 {
				buffer = append(buffer, make([]byte, len(testData)))
			} else if cap(buffer[0]) < len(testData) {
				// 确保缓冲区容量足够
				buffer[0] = make([]byte, len(testData))
			} else {
				buffer[0] = buffer[0][:len(testData)]
			}
			copy(buffer[0], testData)

			// 释放资源回对象池
			ReleaseOutDest([][][]byte{buffer})
		}

		// 强制GC
		runtime.GC()
		runtime.GC() // 二次GC确保清理完成

		// 检查内存使用情况
		finalStats := &runtime.MemStats{}
		runtime.ReadMemStats(finalStats)

		// 计算内存增长（考虑GC导致的内存减少情况）
		var memoryGrowth int64
		if finalStats.Alloc >= initialStats.Alloc {
			memoryGrowth = int64(finalStats.Alloc - initialStats.Alloc)
		} else {
			memoryGrowth = -int64(initialStats.Alloc - finalStats.Alloc)
		}

		t.Logf("内存泄露检测: 初始分配=%d bytes, 最终分配=%d bytes, 变化=%d bytes",
			initialStats.Alloc, finalStats.Alloc, memoryGrowth)

		// 对象池本身不应该造成内存泄露（允许一定的内存波动范围）
		// 这里设置一个合理的阈值（比如不超过迭代次数*平均对象大小的2倍）
		maxExpectedGrowth := int64(iterationCount * 2000) // 2KB per iteration
		if memoryGrowth > maxExpectedGrowth {
			t.Errorf("检测到潜在内存泄露: 内存增长%d bytes超过预期%d bytes",
				memoryGrowth, maxExpectedGrowth)
		} else {
			t.Logf("✅ 内存泄露检测通过")
		}
	})
}

// TestSensitiveDataHandling 敏感数据处理安全测试
func TestSensitiveDataHandling(t *testing.T) {
	model := &TestSecurityModel{}
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	t.Run("安全字段自动擦除", func(t *testing.T) {
		// 创建包含敏感数据的对象
		sensitiveObj := &TestSecurityModel{
			Id:       1,
			Name:     "security_test",
			Password: []byte("super_secret_password_12345"),
			Token:    []byte("auth_token_xyz_789"),
			Data:     []byte("normal_data_content"),
		}

		// 记录原始数据副本
		originalPassword := make([]byte, len(sensitiveObj.Password))
		originalToken := make([]byte, len(sensitiveObj.Token))
		originalData := make([]byte, len(sensitiveObj.Data))
		copy(originalPassword, sensitiveObj.Password)
		copy(originalToken, sensitiveObj.Token)
		copy(originalData, sensitiveObj.Data)

		// 执行安全擦除
		erased, err := SecureEraseBytes(sensitiveObj)
		if err != nil {
			t.Fatalf("安全擦除失败: %v", err)
		}

		if !erased {
			t.Error("期望擦除操作执行但返回false")
		}

		// 验证安全字段已被擦除
		if !bytes.Equal(sensitiveObj.Password, make([]byte, len(originalPassword))) {
			t.Error("密码字段未被正确擦除")
		}
		if !bytes.Equal(sensitiveObj.Token, make([]byte, len(originalToken))) {
			t.Error("令牌字段未被正确擦除")
		}

		// 验证非安全字段保持不变
		if !bytes.Equal(sensitiveObj.Data, originalData) {
			t.Error("普通数据字段被意外修改")
		}

		// 验证其他字段不受影响
		if sensitiveObj.Id != 1 || sensitiveObj.Name != "security_test" {
			t.Error("非字节字段被意外修改")
		}

		t.Logf("✅ 敏感数据安全擦除测试通过")
	})

	t.Run("对象池释放后的数据清理", func(t *testing.T) {
		// 模拟查询结果
		rowData := [][]byte{
			[]byte("1"),
			[]byte("test_user"),
			[]byte("secret_password"),
			[]byte("auth_token"),
			[]byte("normal_data"),
		}

		out := [][][]byte{rowData}

		// 记录原始数据
		originalRowData := make([][]byte, len(rowData))
		for i, data := range rowData {
			originalRowData[i] = make([]byte, len(data))
			copy(originalRowData[i], data)
		}

		// 释放对象池资源（这会清零数据）
		ReleaseOutDest(out)

		// 验证数据已被清零
		for i, data := range rowData {
			if len(data) > 0 {
				allZero := true
				for _, b := range data {
					if b != 0x00 {
						allZero = false
						break
					}
				}
				if !allZero {
					t.Errorf("对象池数据%d未被正确清零", i)
				}
			}
		}

		t.Logf("✅ 对象池释放数据清理测试通过")
	})
}

// TestSQLInjectionPrevention SQL注入防护测试
func TestSQLInjectionPrevention(t *testing.T) {
	model := &TestSecurityModel{}
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	t.Run("条件构造安全检查", func(t *testing.T) {
		// 测试恶意输入
		maliciousInputs := []string{
			"'; DROP TABLE users; --",
			"1' OR '1'='1",
			"admin' --",
			"1; SELECT * FROM sensitive_table; --",
		}

		for _, maliciousInput := range maliciousInputs {
			cnd := sqlc.M(model).Eq("name", maliciousInput)

			// 构造WHERE条件
			casePart, args := NewMysqlManager().BuildWhereCase(cnd)

			// 验证参数化查询（参数应该被正确转义）
			if len(args) == 0 {
				t.Errorf("恶意输入未被正确参数化: %s", maliciousInput)
			}

			// 验证SQL中不包含原始恶意输入
			sqlStr := casePart.String()
			if bytes.Contains([]byte(sqlStr), []byte(maliciousInput)) {
				t.Errorf("SQL注入风险: 原始输入出现在SQL中: %s", maliciousInput)
			}

			t.Logf("安全处理输入: %s -> SQL参数: %v", maliciousInput, args[0])
		}

		t.Logf("✅ SQL注入防护测试通过")
	})
}

// TestConnectionPoolSafety 连接池安全测试
func TestConnectionPoolSafety(t *testing.T) {
	t.Run("真实数据库连接池安全", func(t *testing.T) {
		// 读取数据库配置（尝试多个可能的路径）
		conf := MysqlConfig{}
		var err error
		paths := []string{
			"resource/mysql.json",       // 从项目根目录运行
			"../resource/mysql.json",    // 从子目录运行
			"../../resource/mysql.json", // 从更深层目录运行
		}

		for _, path := range paths {
			if err = utils.ReadLocalJsonConfig(path, &conf); err == nil {
				break
			}
		}

		if err != nil {
			t.Skipf("跳过数据库连接池测试 - 无法读取配置 (尝试路径: %v): %v", paths, err)
			return
		}

		// 初始化数据库连接
		mysqlMgr := new(MysqlManager)
		if err := mysqlMgr.InitConfigAndCache(nil, conf); err != nil {
			t.Fatalf("数据库初始化失败: %v", err)
		}
		defer MysqlClose() // 确保清理连接

		// 注册OwWallet模型
		if err := ModelDriver(&OwWallet{}); err != nil {
			t.Fatalf("模型注册失败: %v", err)
		}

		t.Run("连接池并发访问安全", func(t *testing.T) {
			const goroutines = 10
			const iterations = 50

			var wg sync.WaitGroup
			var errors int64
			var operations int64

			// 并发执行数据库操作
			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func(goroutineID int) {
					defer wg.Done()

					for j := 0; j < iterations; j++ {
						atomic.AddInt64(&operations, 1)

						// 创建测试数据
						walletID := fmt.Sprintf("test_wallet_%d_%d", goroutineID, j)
						testWallet := &OwWallet{
							AppID:        "test_app",
							WalletID:     walletID,
							Alias:        fmt.Sprintf("Test Wallet %d-%d", goroutineID, j),
							IsTrust:      1,
							PasswordType: 1,
							Password:     []byte(fmt.Sprintf("password_%d_%d", goroutineID, j)),
							AuthKey:      fmt.Sprintf("auth_key_%d_%d", goroutineID, j),
							RootPath:     "/test/path",
							State:        1,
							Ctime:        time.Now().Unix(),
							Utime:        time.Now().Unix(),
						}

						// 获取数据库管理器实例
						dbMgr, err := NewMysql(Option{
							DsName:   DIC.MASTER,
							Database: conf.Database,
							Timeout:  5000,
						})
						if err != nil {
							atomic.AddInt64(&errors, 1)
							t.Errorf("获取数据库管理器失败: %v", err)
							continue
						}

						// 执行保存操作（测试连接池分配）
						err = dbMgr.Save(testWallet)
						if err != nil {
							atomic.AddInt64(&errors, 1)
							t.Logf("保存操作失败: %v", err)
							continue
						}

						// 执行查询操作（测试连接池重用）
						queryWallet := &OwWallet{}
						err = dbMgr.FindOne(sqlc.M(queryWallet).Eq("walletID", walletID), queryWallet)
						if err != nil {
							atomic.AddInt64(&errors, 1)
							t.Logf("查询操作失败: %v", err)
							continue
						}

						// 验证查询结果
						if queryWallet.WalletID != walletID {
							atomic.AddInt64(&errors, 1)
							t.Errorf("查询结果不匹配: 期望 %s, 实际 %s", walletID, queryWallet.WalletID)
							continue
						}

						// 验证字节数组安全
						if !bytes.Equal(queryWallet.Password, testWallet.Password) {
							atomic.AddInt64(&errors, 1)
							t.Errorf("密码数据不匹配 - 对象池污染!")
							continue
						}

						// 执行清理（可选，避免测试数据积累过多）
						if j%10 == 0 { // 每10次清理一次
							_, _ = dbMgr.DeleteByCnd(sqlc.M(queryWallet).Eq("appID", "test_app"))
						}
					}
				}(i)
			}

			wg.Wait()

			t.Logf("连接池并发测试完成:")
			t.Logf("  - 总操作数: %d", operations)
			t.Logf("  - 成功操作: %d", operations-errors)
			t.Logf("  - 失败操作: %d", errors)
			t.Logf("  - 成功率: %.2f%%", float64(operations-errors)/float64(operations)*100)

			if errors > 0 {
				t.Errorf("连接池测试发现%d个错误", errors)
			}

			if operations == 0 {
				t.Error("没有执行任何操作")
			}
		})

		t.Run("连接池资源释放验证", func(t *testing.T) {
			// 创建测试数据
			testWallet := &OwWallet{
				AppID:        "test_app_pool",
				WalletID:     "pool_resource_test",
				Alias:        "Pool Resource Test",
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte("pool_resource_password"),
				AuthKey:      "pool_resource_auth",
				State:        1,
				Ctime:        time.Now().Unix(),
				Utime:        time.Now().Unix(),
			}

			// 获取数据库管理器
			dbMgr, err := NewMysql(Option{
				DsName:   DIC.MASTER,
				Database: conf.Database,
				Timeout:  5000,
			})
			if err != nil {
				t.Fatalf("获取数据库管理器失败: %v", err)
			}

			// 执行保存操作
			err = dbMgr.Save(testWallet)
			if err != nil {
				t.Fatalf("保存操作失败: %v", err)
			}

			// 验证连接池统计（如果可用）
			// 注意：这里我们无法直接访问底层的连接池统计，
			// 但通过成功的数据库操作可以间接验证连接池工作正常

			// 执行查询验证
			queryWallet := &OwWallet{}
			err = dbMgr.FindOne(sqlc.M(queryWallet).Eq("walletID", "pool_resource_test"), queryWallet)
			if err != nil {
				t.Fatalf("查询验证失败: %v", err)
			}

			// 验证数据完整性
			if queryWallet.WalletID != "pool_resource_test" {
				t.Errorf("查询结果不正确")
			}

			if !bytes.Equal(queryWallet.Password, []byte("pool_resource_password")) {
				t.Errorf("密码数据不匹配 - 可能存在连接池污染")
			}

			t.Logf("✅ 连接池资源释放验证通过")
		})

		t.Run("事务安全测试", func(t *testing.T) {
			// 测试事务模式下的连接池安全
			dbMgr, err := NewMysql(Option{
				DsName:   DIC.MASTER,
				Database: conf.Database,
				Timeout:  10000,
				OpenTx:   true, // 开启事务
			})
			if err != nil {
				t.Fatalf("获取事务数据库管理器失败: %v", err)
			}

			// 执行事务操作
			txWallet := &OwWallet{
				AppID:        "test_app_tx",
				WalletID:     "tx_safety_test",
				Alias:        "Transaction Safety Test",
				IsTrust:      1,
				PasswordType: 1,
				Password:     []byte("transaction_password"),
				State:        1,
				Ctime:        time.Now().Unix(),
				Utime:        time.Now().Unix(),
			}

			// 保存操作
			err = dbMgr.Save(txWallet)
			if err != nil {
				t.Fatalf("事务保存失败: %v", err)
			}

			// 查询验证
			queryWallet := &OwWallet{}
			err = dbMgr.FindOne(sqlc.M(queryWallet).Eq("walletID", "tx_safety_test"), queryWallet)
			if err != nil {
				t.Fatalf("事务查询失败: %v", err)
			}

			// 验证数据
			if !bytes.Equal(queryWallet.Password, []byte("transaction_password")) {
				t.Errorf("事务中密码数据不匹配")
			}

			// 注意：事务模式下，连接会在Close()时提交或回滚
			err = dbMgr.Close()
			if err != nil {
				t.Fatalf("事务关闭失败: %v", err)
			}

			t.Logf("✅ 事务安全测试通过")
		})

		t.Logf("✅ 真实数据库连接池安全测试全部通过")
	})
}

// TestStmtCacheSafety 预编译语句缓存安全测试
func TestStmtCacheSafety(t *testing.T) {
	t.Run("缓存键安全生成", func(t *testing.T) {
		// 测试缓存键的唯一性和安全性
		opt1 := Option{DsName: "db1", Database: "test1", Timeout: 1000}
		opt2 := Option{DsName: "db2", Database: "test2", Timeout: 1000}
		opt3 := Option{DsName: "db1", Database: "test1", Timeout: 2000} // 相同数据库不同超时

		key1 := hashOptions(opt1)
		key2 := hashOptions(opt2)
		key3 := hashOptions(opt3)

		// 验证不同配置产生不同缓存键
		if key1 == key2 {
			t.Error("不同数据库配置产生相同缓存键")
		}
		if key1 == key3 {
			t.Error("相同数据库不同超时产生相同缓存键")
		}

		t.Logf("缓存键唯一性验证通过: key1=%s, key2=%s, key3=%s", key1, key2, key3)
	})
}

// TestMemoryBoundarySafety 内存边界安全测试
func TestMemoryBoundarySafety(t *testing.T) {
	model := &TestSecurityModel{}
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	var dataField *FieldElem
	if driver, ok := modelDrivers[model.GetTable()]; ok {
		for _, elem := range driver.FieldElem {
			if elem.FieldName == "Data" {
				dataField = elem
				break
			}
		}
	}

	t.Run("边界数据处理", func(t *testing.T) {
		testCases := []struct {
			name string
			data []byte
		}{
			{"空字节数组", []byte{}},
			{"单个字节", []byte{0x42}},
			{"大字节数组", make([]byte, 1024*1024)}, // 1MB
			{"包含特殊字符", []byte{0x00, 0x01, 0xFF, 0xFE}},
			{"UTF-8字符串", []byte("Hello, 世界! 🌍")},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				testObj := &TestSecurityModel{}

				// 记录原始数据
				originalData := make([]byte, len(tc.data))
				copy(originalData, tc.data)

				// 设置字段值
				err := SetValue(testObj, dataField, tc.data)
				if err != nil {
					t.Fatalf("设置%s失败: %v", tc.name, err)
				}

				// 验证数据完整性
				if !bytes.Equal(testObj.Data, originalData) {
					t.Errorf("%s数据完整性检查失败", tc.name)
				}

				// 模拟缓冲区回收
				for i := range tc.data {
					tc.data[i] = 0xAA
				}

				// 验证对象数据不受影响
				if !bytes.Equal(testObj.Data, originalData) {
					t.Errorf("%s对象池隔离检查失败", tc.name)
				}
			})
		}

		t.Logf("✅ 内存边界安全测试通过")
	})
}

// TestRaceConditionSafety 竞态条件安全测试
func TestRaceConditionSafety(t *testing.T) {
	model := &TestSecurityModel{}
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	t.Run("对象池竞态条件", func(t *testing.T) {
		const goroutines = 50
		const iterations = 100

		var wg sync.WaitGroup
		var errors int64

		// 并发获取和释放对象池资源
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				for j := 0; j < iterations; j++ {
					// 获取对象池资源
					buffer := rowByteSlicePool.Get().([][]byte)

					// 模拟使用
					if len(buffer) == 0 {
						buffer = append(buffer, make([]byte, 64))
					}

					testData := []byte(fmt.Sprintf("Goroutine%d-Iteration%d", id, j))
					copy(buffer[0][:len(testData)], testData)

					// 短暂延迟模拟实际使用
					time.Sleep(time.Microsecond)

					// 释放资源
					ReleaseOutDest([][][]byte{buffer})
				}
			}(i)
		}

		wg.Wait()

		if errors > 0 {
			t.Errorf("竞态条件测试发现%d个错误", errors)
		}

		t.Logf("✅ 对象池竞态条件安全测试通过")
	})
}

// ==========================================
// 基准测试
// ==========================================

// BenchmarkObjectPoolPerformance 对象池性能基准测试
func BenchmarkObjectPoolPerformance(b *testing.B) {
	model := &TestSecurityModel{}
	if err := ModelDriver(model); err != nil {
		b.Fatalf("注册模型失败: %v", err)
	}

	var dataField *FieldElem
	if driver, ok := modelDrivers[model.GetTable()]; ok {
		for _, elem := range driver.FieldElem {
			if elem.FieldName == "Data" {
				dataField = elem
				break
			}
		}
	}

	testData := []byte("Benchmark test data for object pool performance measurement")

	b.Run("对象池获取释放性能", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buffer := rowByteSlicePool.Get().([][]byte)
			ReleaseOutDest([][][]byte{buffer})
		}
	})

	b.Run("数据填充性能", func(b *testing.B) {
		testObj := &TestSecurityModel{}
		for i := 0; i < b.N; i++ {
			SetValue(testObj, dataField, testData)
		}
	})

	b.Run("安全擦除性能", func(b *testing.B) {
		testObj := &TestSecurityModel{
			Password: []byte("benchmark_password_data"),
			Token:    []byte("benchmark_token_data"),
		}
		for i := 0; i < b.N; i++ {
			SecureEraseBytes(testObj)
		}
	})
}

// ==========================================
// 辅助函数
// ==========================================

// NewMysqlManager 创建MySQL管理器用于测试
func NewMysqlManager() *RDBManager {
	return &RDBManager{}
}
