package main

import (
	"errors"
	"github.com/godaddy-x/freego/ex"
	"strings"
	"testing"
)

// TestErrorChainBasic 测试基础错误链保持功能
func TestErrorChainBasic(t *testing.T) {
	// 创建原始错误
	originalErr := errors.New("database connection failed: timeout after 30s")

	// 创建Throw对象，包含原始错误
	throw := ex.Throw{
		Code: ex.DATA,
		Msg:  "数据服务异常",
		Url:  "/api/users",
		Arg:  []string{"user_id", "12345", "action", "query"},
		Err:  originalErr,
	}

	// 验证原始对象
	if throw.Code != ex.DATA {
		t.Errorf("期望Code为%d，实际为%d", ex.DATA, throw.Code)
	}
	if throw.Msg != "数据服务异常" {
		t.Errorf("期望Msg为%s，实际为%s", "数据服务异常", throw.Msg)
	}
	if throw.Err == nil || throw.Err.Error() != originalErr.Error() {
		t.Errorf("原始错误设置失败")
	}

	// 序列化为字符串（使用easyjson）
	serialized := throw.Error()
	if serialized == "" {
		t.Error("序列化结果为空")
	}

	// 使用Catch反序列化
	caught := ex.Catch(throw)

	// 验证反序列化结果
	if caught.Code != throw.Code {
		t.Errorf("Code不匹配：期望%d，实际%d", throw.Code, caught.Code)
	}
	if caught.Msg != throw.Msg {
		t.Errorf("Msg不匹配：期望%s，实际%s", throw.Msg, caught.Msg)
	}
	if caught.Url != throw.Url {
		t.Errorf("Url不匹配：期望%s，实际%s", throw.Url, caught.Url)
	}
	if len(caught.Arg) != len(throw.Arg) {
		t.Errorf("Arg长度不匹配")
	}
	for i, arg := range throw.Arg {
		if caught.Arg[i] != arg {
			t.Errorf("Arg[%d]不匹配：期望%s，实际%s", i, arg, caught.Arg[i])
		}
	}

	// 验证错误链保持
	if caught.Err == nil {
		t.Error("反序列化后错误丢失")
	} else if caught.Err.Error() != originalErr.Error() {
		t.Errorf("错误信息不匹配：期望%s，实际%s", originalErr.Error(), caught.Err.Error())
	}

	// 验证序列化后的JSON包含错误信息
	if !strings.Contains(serialized, originalErr.Error()) {
		t.Errorf("序列化JSON不包含错误信息：%s", serialized)
	}
}

// TestErrorChainComplex 测试复杂错误信息保持
func TestErrorChainComplex(t *testing.T) {
	// 创建包含详细信息的错误
	complexErr := errors.New("network error: dial tcp 192.168.1.100:5432: connect: connection refused (context deadline exceeded)")

	throw := ex.Throw{
		Code: ex.SYSTEM,
		Msg:  "系统网络异常",
		Err:  complexErr,
	}

	// 序列化
	serialized := throw.Error()
	if serialized == "" {
		t.Error("序列化失败")
	}

	// 反序列化
	caught := ex.Catch(throw)

	// 验证错误信息完全保持
	if caught.Err == nil {
		t.Error("复杂错误信息丢失")
	} else if caught.Err.Error() != complexErr.Error() {
		t.Errorf("复杂错误信息不匹配：\n期望: %s\n实际: %s", complexErr.Error(), caught.Err.Error())
	}

	// 验证序列化后的JSON包含错误信息
	if !strings.Contains(serialized, complexErr.Error()) {
		t.Errorf("序列化JSON不包含复杂错误信息：%s", serialized)
	}
}

// TestErrorChainNilError 测试空错误情况
func TestErrorChainNilError(t *testing.T) {
	throw := ex.Throw{
		Code: ex.BIZ,
		Msg:  "普通业务提示",
		Url:  "/api/login",
		// Err字段为nil
	}

	// 验证原始状态
	if throw.Err != nil {
		t.Error("期望Err为nil")
	}

	// 序列化
	serialized := throw.Error()
	if serialized == "" {
		t.Error("序列化失败")
	}

	// 反序列化
	caught := ex.Catch(throw)

	// 验证空错误情况正确处理
	if caught.Err != nil {
		t.Error("空错误情况下Err应该为nil")
	}
	if throw.ErrMsg != "" {
		t.Error("空错误情况下ErrMsg应该为空")
	}
}

// TestCatchDirectThrow 测试Catch函数直接处理Throw对象
func TestCatchDirectThrow(t *testing.T) {
	// 创建Throw对象
	throw := ex.Throw{
		Code: ex.CACHE,
		Msg:  "缓存服务异常",
		Err:  errors.New("redis connection pool exhausted"),
	}

	// 直接用Catch处理Throw对象（不经过字符串序列化）
	caught := ex.Catch(throw)

	// 验证直接处理结果
	if caught.Code != throw.Code {
		t.Errorf("Code不匹配：期望%d，实际%d", throw.Code, caught.Code)
	}
	if caught.Msg != throw.Msg {
		t.Errorf("Msg不匹配：期望%s，实际%s", throw.Msg, caught.Msg)
	}
	if caught.Err == nil || caught.Err.Error() != throw.Err.Error() {
		t.Error("直接Catch时错误信息丢失")
	}
}

// TestCatchJsonError 测试Catch函数处理JSON字符串错误
func TestCatchJsonError(t *testing.T) {
	// 创建Throw对象
	originalErr := errors.New("test error for JSON serialization")
	throw := ex.Throw{
		Code: ex.JSON,
		Msg:  "JSON处理异常",
		Err:  originalErr,
	}

	// 获取JSON字符串
	jsonStr := throw.Error()

	// 模拟从其他地方接收到这个JSON字符串，然后创建error
	jsonError := errors.New(jsonStr)

	// 使用Catch处理这个JSON错误
	caught := ex.Catch(jsonError)

	// 验证反序列化结果
	if caught.Code != ex.JSON {
		t.Errorf("JSON反序列化Code错误：期望%d，实际%d", ex.JSON, caught.Code)
	}
	if caught.Msg != "JSON处理异常" {
		t.Errorf("JSON反序列化Msg错误：期望%s，实际%s", "JSON处理异常", caught.Msg)
	}
	if caught.Err == nil || caught.Err.Error() != originalErr.Error() {
		t.Error("JSON反序列化错误信息丢失")
	}
}

// TestCatchInvalidJson 测试Catch处理无效JSON
func TestCatchInvalidJson(t *testing.T) {
	// 创建无效的JSON字符串
	invalidJson := errors.New("invalid json string")

	caught := ex.Catch(invalidJson)

	// 验证错误处理
	if caught.Code != ex.UNKNOWN {
		t.Errorf("无效JSON处理Code错误：期望%d，实际%d", ex.UNKNOWN, caught.Code)
	}
	if caught.Msg != "failed to catch exception" {
		t.Errorf("无效JSON处理Msg错误")
	}
}

// TestCatchNilError 测试Catch处理nil错误
func TestCatchNilError(t *testing.T) {
	caught := ex.Catch(nil)

	if caught.Code != ex.UNKNOWN {
		t.Errorf("nil错误处理Code错误：期望%d，实际%d", ex.UNKNOWN, caught.Code)
	}
	if caught.Msg != "catch error is nil" {
		t.Errorf("nil错误处理Msg错误")
	}
	if caught.Err != nil {
		t.Error("nil错误情况下Err应该为nil")
	}
}

// TestErrorMethodCodeSetting 测试Error方法自动设置Code
func TestErrorMethodCodeSetting(t *testing.T) {
	throw := ex.Throw{
		// Code为0，不设置
		Msg: "测试消息",
	}

	// 验证原始Code为0
	if throw.Code != 0 {
		t.Errorf("期望初始Code为0，实际为%d", throw.Code)
	}

	// 调用Error方法
	jsonStr := throw.Error()

	// 验证JSON包含正确的Code (BIZ = 100000)
	if !strings.Contains(jsonStr, `"Code":100000`) {
		t.Errorf("JSON中Code没有被设置为BIZ(100000)：%s", jsonStr)
	}

	// 验证Catch后的对象Code正确
	caught := ex.Catch(errors.New(jsonStr))
	if caught.Code != ex.BIZ {
		t.Errorf("Catch后Code错误：期望%d，实际%d", ex.BIZ, caught.Code)
	}
}
