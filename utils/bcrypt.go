package utils

func randKey() string {
	return MD5(AddStr(GetUUID(), AnyToStr(UnixNano()), RandStr(32)))
}

func newHash(password, salt, randKey string) string {
	return HMAC_SHA512(HMAC_SHA512(password, randKey), HMAC_SHA512(HMAC_SHA512(salt, randKey), HMAC_SHA512(GetLocalSecretKey(), randKey)))
}

func mergeHash(hash, key string) string {
	return AddStr(hash, key)
}

func PasswordHash(password, salt string) string {
	key := randKey()
	hash := newHash(password, salt, key)
	return mergeHash(hash, key)
}

func PasswordVerify(password, salt, target string) bool {
	if len(target) != 160 {
		return false
	}
	key := target[128:]
	hash := newHash(password, salt, key)
	return mergeHash(hash, key) == target
}
