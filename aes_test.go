package main

import (
	"testing"

	utils "github.com/godaddy-x/freego/core/str"
)

// TestAesGCMEncryptDecrypt 测试基本的 GCM 加密解密
func TestAesGCMEncryptDecrypt(t *testing.T) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte("transfer:amount=10000,to=account123我是中文啊")

	encrypted, err := utils.AesGCMEncryptWithAAD(plaintext, key, "123456789")
	if err != nil {
		t.Fatalf("AesGCMEncrypt failed: %v", err)
	}

	decrypted, err := utils.AesGCMDecryptWithAAD(encrypted, key, "123456789")
	if err != nil {
		t.Fatalf("AesGCMDecrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted text mismatch: got %s, want %s", decrypted, plaintext)
	}
}

// TestAesGCMTamperDetection 测试 GCM 的篡改检测能力
func TestAesGCMTamperDetection(t *testing.T) {
	key := "my-super-secret-key-for-aes-256"
	plaintext := []byte(`{"amount": 100, "to": "account1"}`)

	encrypted, err := utils.AesGCMEncrypt(plaintext, key)
	if err != nil {
		t.Fatalf("AesGCMEncrypt failed: %v", err)
	}

	encryptedBytes := []byte(encrypted)
	if len(encryptedBytes) > 20 {
		encryptedBytes[20] ^= 0xFF
	}
	tamperedEncrypted := string(encryptedBytes)

	_, err = utils.AesGCMDecrypt(tamperedEncrypted, key)
	if err == nil {
		t.Fatal("Expected authentication failure for tampered data, but decryption succeeded!")
	}

	t.Logf("Tamper detection works correctly: %v", err)
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
