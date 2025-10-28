package main

import (
	"github.com/godaddy-x/freego/utils"
	"testing"
)

// TestAesGCMEncryptDecrypt 测试基本的 GCM 加密解密
func TestAesGCMEncryptDecrypt(t *testing.T) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte("transfer:amount=10000,to=account123我是中文啊")

	// 加密
	encrypted, err := utils.AesGCMEncryptWithAAD(plaintext, key, "123456789")
	if err != nil {
		t.Fatalf("AesGCMEncrypt failed: %v", err)
	}

	// 解密
	decrypted, err := utils.AesGCMDecryptWithAAD(encrypted, key, "123456789")
	if err != nil {
		t.Fatalf("AesGCMDecrypt failed: %v", err)
	}

	// 验证
	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted text mismatch: got %s, want %s", decrypted, plaintext)
	}
}

// TestAesGCMTamperDetection 测试 GCM 的篡改检测能力（关键测试）
func TestAesGCMTamperDetection(t *testing.T) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte(`{"amount": 100, "to": "account1"}`)

	// 加密
	encrypted, err := utils.AesGCMEncrypt(plaintext, key)
	if err != nil {
		t.Fatalf("AesGCMEncrypt failed: %v", err)
	}

	// 模拟攻击者篡改密文（修改某个字节）
	encryptedBytes := []byte(encrypted)
	if len(encryptedBytes) > 20 {
		encryptedBytes[20] ^= 0xFF // 翻转一个字节
	}
	tamperedEncrypted := string(encryptedBytes)

	// 尝试解密被篡改的密文
	_, err = utils.AesGCMDecrypt(tamperedEncrypted, key)

	// ✅ 期望解密失败（认证失败）
	if err == nil {
		t.Fatal("Expected authentication failure for tampered data, but decryption succeeded!")
	}

	t.Logf("✅ Tamper detection works correctly: %v", err)
}

// TestAesGCMVsCBC 对比 GCM 和 CBC 的篡改检测能力
func TestAesGCMVsCBC(t *testing.T) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte(`{"amount": 100, "to": "account1"}`)

	t.Run("CBC - No Tamper Detection", func(t *testing.T) {
		// CBC 加密
		encrypted, err := utils.AesCBCEncrypt(plaintext, key)
		if err != nil {
			t.Fatalf("AesCBCEncrypt failed: %v", err)
		}

		// 篡改密文
		data := utils.Base64Decode(encrypted)
		if len(data) > 20 {
			data[20] ^= 0x01 // 小幅修改
		}
		tamperedEncrypted := utils.Base64Encode(data)

		// ⚠️ CBC 解密可能"成功"（虽然数据损坏）
		decrypted, err := utils.AesCBCDecrypt(tamperedEncrypted, key)

		// CBC 可能解密成功（返回损坏的数据）或 Padding 错误
		if err == nil {
			t.Logf("⚠️ CBC decryption succeeded with tampered data: %s (corrupted!)", decrypted)
		} else {
			t.Logf("⚠️ CBC decryption failed: %v (Padding error, not authentication failure)", err)
		}
	})

	t.Run("GCM - Strong Tamper Detection", func(t *testing.T) {
		// GCM 加密
		encrypted, err := utils.AesGCMEncrypt(plaintext, key)
		if err != nil {
			t.Fatalf("AesGCMEncrypt failed: %v", err)
		}

		// 篡改密文
		data := utils.Base64Decode(encrypted)
		if len(data) > 20 {
			data[20] ^= 0x01 // 小幅修改
		}
		tamperedEncrypted := utils.Base64Encode(data)

		// ✅ GCM 解密必然失败（认证失败）
		_, err = utils.AesGCMDecrypt(tamperedEncrypted, key)
		if err == nil {
			t.Fatal("❌ GCM should fail authentication for tampered data!")
		}

		t.Logf("✅ GCM authentication failed correctly: %v", err)
	})
}

// BenchmarkAesGCMEncrypt 性能基准测试：GCM 加密
func BenchmarkAesGCMEncrypt(b *testing.B) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte("transfer:amount=10000,to=account123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = utils.AesGCMEncrypt(plaintext, key)
	}
}

// BenchmarkAesCBCEncrypt 性能基准测试：CBC 加密
func BenchmarkAesCBCEncrypt(b *testing.B) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte("transfer:amount=10000,to=account123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = utils.AesCBCEncrypt(plaintext, key)
	}
}

// BenchmarkAesGCMDecrypt 性能基准测试：GCM 解密
func BenchmarkAesGCMDecrypt(b *testing.B) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte("transfer:amount=10000,to=account123")
	encrypted, _ := utils.AesGCMEncrypt(plaintext, key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = utils.AesGCMDecrypt(encrypted, key)
	}
}

// BenchmarkAesCBCDecrypt 性能基准测试：CBC 解密
func BenchmarkAesCBCDecrypt(b *testing.B) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte("transfer:amount=10000,to=account123")
	encrypted, _ := utils.AesCBCEncrypt(plaintext, key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = utils.AesCBCDecrypt(encrypted, key)
	}
}
