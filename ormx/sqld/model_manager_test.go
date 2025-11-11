package sqld

import (
	"bytes"
	"testing"

	"github.com/godaddy-x/freego/ormx/sqlc"
)

// TestSecureEraseBytes 测试安全擦除字节数组功能
type TestSecureModel struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	Password []byte `json:"password" safe:"true"` // 标记为安全擦除
	Token    []byte `json:"token" safe:"true"`    // 标记为安全擦除
	Data     []byte `json:"data"`                 // 普通字节数组，不会被擦除
}

func (o *TestSecureModel) GetTable() string {
	return "test_secure_model"
}

func (o *TestSecureModel) NewObject() sqlc.Object {
	return &TestSecureModel{}
}

func (o *TestSecureModel) AppendObject(data interface{}, target sqlc.Object) {
	*data.(*[]*TestSecureModel) = append(*data.(*[]*TestSecureModel), target.(*TestSecureModel))
}

func (o *TestSecureModel) NewIndex() []sqlc.Index {
	return []sqlc.Index{}
}

// TestSecureEraseBytes 测试安全擦除功能
func TestSecureEraseBytes(t *testing.T) {
	// 创建测试对象并设置初始数据
	originalPassword := []byte("super_secret_password_123")
	originalToken := []byte("auth_token_xyz_456")
	originalData := []byte("normal_data_789")

	model := &TestSecureModel{
		Id:       1,
		Name:     "test_user",
		Password: make([]byte, len(originalPassword)),
		Token:    make([]byte, len(originalToken)),
		Data:     make([]byte, len(originalData)),
	}

	// 复制原始数据
	copy(model.Password, originalPassword)
	copy(model.Token, originalToken)
	copy(model.Data, originalData)

	// 验证初始数据正确设置
	if !bytes.Equal(model.Password, originalPassword) {
		t.Errorf("初始密码数据设置失败")
	}
	if !bytes.Equal(model.Token, originalToken) {
		t.Errorf("初始令牌数据设置失败")
	}
	if !bytes.Equal(model.Data, originalData) {
		t.Errorf("初始普通数据设置失败")
	}

	// 注册模型（确保模型驱动存在）
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	// 执行安全擦除
	erased, err := SecureEraseBytes(model)
	if err != nil {
		t.Fatalf("SecureEraseBytes 执行失败: %v", err)
	}

	// 验证返回结果
	if !erased {
		t.Error("期望擦除操作被执行，但返回值为 false")
	}

	// 验证安全字段已被擦除
	expectedZeroPassword := make([]byte, len(originalPassword))
	expectedZeroToken := make([]byte, len(originalToken))

	if !bytes.Equal(model.Password, expectedZeroPassword) {
		t.Errorf("密码字段未被正确擦除，期望全零，实际: %v", model.Password)
	}
	if !bytes.Equal(model.Token, expectedZeroToken) {
		t.Errorf("令牌字段未被正确擦除，期望全零，实际: %v", model.Token)
	}

	// 验证非安全字段未被擦除
	if !bytes.Equal(model.Data, originalData) {
		t.Errorf("普通数据字段被意外修改，期望保持不变: %v", model.Data)
	}

	// 验证其他字段未受影响
	if model.Id != 1 {
		t.Errorf("ID字段被意外修改: %d", model.Id)
	}
	if model.Name != "test_user" {
		t.Errorf("名称字段被意外修改: %s", model.Name)
	}

	t.Logf("✅ 安全擦除测试通过")
	t.Logf("   密码字段长度: %d, 内容已清零", len(model.Password))
	t.Logf("   令牌字段长度: %d, 内容已清零", len(model.Token))
	t.Logf("   普通数据字段长度: %d, 内容保持不变", len(model.Data))
}

// TestNormalModel 测试模型（没有安全字段）
type TestNormalModel struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
	Data []byte `json:"data"` // 没有 safe 标签
}

func (o *TestNormalModel) GetTable() string {
	return "test_normal_model"
}

func (o *TestNormalModel) NewObject() sqlc.Object {
	return &TestNormalModel{}
}

func (o *TestNormalModel) AppendObject(data interface{}, target sqlc.Object) {
	*data.(*[]*TestNormalModel) = append(*data.(*[]*TestNormalModel), target.(*TestNormalModel))
}

func (o *TestNormalModel) NewIndex() []sqlc.Index {
	return []sqlc.Index{}
}

// TestSecureEraseBytes_NoSafeFields 测试没有安全字段的情况
func TestSecureEraseBytes_NoSafeFields(t *testing.T) {
	model := &TestNormalModel{
		Id:   1,
		Name: "normal_user",
		Data: []byte("some_data"),
	}

	// 注册模型
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	// 执行擦除（应该不执行任何操作）
	erased, err := SecureEraseBytes(model)
	if err != nil {
		t.Fatalf("SecureEraseBytes 执行失败: %v", err)
	}

	// 验证没有执行擦除
	if erased {
		t.Error("不应该执行擦除操作，但返回值为 true")
	}

	// 验证数据保持不变
	if !bytes.Equal(model.Data, []byte("some_data")) {
		t.Errorf("数据被意外修改: %v", model.Data)
	}

	t.Logf("✅ 无安全字段测试通过")
}

// TestSecureEraseBytes_EmptySlice 测试空切片的情况
func TestSecureEraseBytes_EmptySlice(t *testing.T) {
	model := &TestSecureModel{
		Id:       1,
		Name:     "test_user",
		Password: []byte{}, // 空切片
		Token:    []byte("valid_token"),
		Data:     []byte("normal_data"),
	}

	// 注册模型
	if err := ModelDriver(model); err != nil {
		t.Fatalf("注册模型失败: %v", err)
	}

	// 执行擦除
	erased, err := SecureEraseBytes(model)
	if err != nil {
		t.Fatalf("SecureEraseBytes 执行失败: %v", err)
	}

	// 验证只擦除了非空的 Token 字段
	if !erased {
		t.Error("应该擦除非空的安全字段")
	}

	// 验证 Token 被擦除
	expectedZeroToken := make([]byte, len("valid_token"))
	if !bytes.Equal(model.Token, expectedZeroToken) {
		t.Errorf("令牌字段未被正确擦除: %v", model.Token)
	}

	// 验证空切片 Password 保持不变
	if len(model.Password) != 0 {
		t.Errorf("空密码切片被意外修改: %v", model.Password)
	}

	t.Logf("✅ 空切片测试通过")
}
