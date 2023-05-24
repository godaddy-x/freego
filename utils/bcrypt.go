package utils

func newKey() string {
	return MD5(AddStr(GetUUID(), AnyToStr(UnixNano()), RandStr(32)))
}

func newHash(password, salt, key string) string {
	return HMAC_SHA256(HMAC_SHA512(password, key), HMAC_SHA512(HMAC_SHA512(salt, key), HMAC_SHA512(GetLocalSecretKey(), key)))
}

func recHash(hash, salt, key string, index int) string {
	if index > 16 {
		return hash
	}
	hash = HMAC_SHA256(hash, HMAC_SHA256(salt, key))
	index++
	return recHash(hash, salt, key, index)
}

func mergeHash(hash, key string) string {
	return AddStr(hash, key)
}

func PasswordHash(password, salt string) string {
	key := newKey()
	hash := newHash(password, salt, key)
	hash = recHash(hash, salt, key, 0)
	return mergeHash(hash, key)
}

func PasswordVerify(password, salt, target string) bool {
	if len(target) != 96 {
		return false
	}
	key := target[64:]
	hash := newHash(password, salt, key)
	hash = recHash(hash, salt, key, 0)
	return mergeHash(hash, key) == target
}
