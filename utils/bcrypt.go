package utils

func newKey() string {
	return MD5(AddStr(GetUUID(), AnyToStr(UnixNano()), RandNonce()))
}

func newHash(password, salt, key string) string {
	return HmacSHA512(HmacSHA512(password, key), HmacSHA512(HmacSHA512(salt, key), HmacSHA512(GetLocalSecretKey(), key)))
}

func newHashR(hash, salt, key string) string {
	return HmacSHA256(HmacSHA512(hash, key), HmacSHA512(HmacSHA512(salt, key), HmacSHA512(GetLocalSecretKey(), key)))
}

func recHash(hash, salt, key string, index, depth int) string {
	if index > depth {
		return hash
	}
	hash = HmacSHA512(hash, HmacSHA256(salt, key))
	index++
	return recHash(hash, salt, key, index, depth)
}

func mergeHash(hash, key string) string {
	return AddStr(hash, key)
}

func PasswordHash(password, salt string, depth ...int) string {
	dp := 16
	if len(depth) > 0 {
		dp = depth[0]
	}
	key := newKey()
	hash := newHash(password, salt, key)
	hash = recHash(hash, salt, key, 0, dp)
	return mergeHash(newHashR(hash, salt, key), key)
}

func PasswordVerify(password, salt, target string, depth ...int) bool {
	if len(target) != 96 {
		return false
	}
	dp := 16
	if len(depth) > 0 {
		dp = depth[0]
	}
	key := target[64:]
	hash := newHash(password, salt, key)
	hash = recHash(hash, salt, key, 0, dp)
	return mergeHash(newHashR(hash, salt, key), key) == target
}
