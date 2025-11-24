package utils

import (
	"crypto/sha512"
	"encoding/hex"
	"golang.org/x/crypto/pbkdf2"
)

func PasswordHash(password, salt string) string {
	return hex.EncodeToString(pbkdf2.Key(Str2Bytes(password), Str2Bytes(salt), 10000, 64, sha512.New))
}

func PasswordVerify(password, salt, target string) bool {
	return PasswordHash(password, salt) == target
}
