package node

import (
	"net/http"

	"github.com/godaddy-x/freego/cache"
	DIC "github.com/godaddy-x/freego/common"
	"github.com/godaddy-x/freego/ex"
	"github.com/godaddy-x/freego/utils"
	fgocrypto "github.com/godaddy-x/freego/utils/crypto"
	"github.com/godaddy-x/freego/utils/jwt"
)

func (self *Context) validJsonBody() error {
	if self.JsonBody == nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request json body is nil"}
	}
	body := self.JsonBody
	// Step 1: 轻量无状态校验（字段完整性、长度、基础类型、plan 合法性）
	if err := self.validJsonBodyCommon(body); err != nil {
		return err
	}

	if body.Plan == 2 {
		return self.validJsonBodyPlan2Flow(body)
	}
	return self.validJsonBodyPlan01Flow(body)
}

func (self *Context) validJsonBodyPlan01Flow(body *JsonBody) error {
	sharedKey, err := self.getPlan01DerivedKey(body)
	if err != nil {
		return err
	}
	defer DIC.ClearData(sharedKey)

	// Step 2: HMAC 验证（快速拒绝大多数无效请求）
	sign, _ := SignAndDigestBodyMessage(self.Path, body.Data, body.Nonce, body.Time, body.Plan, body.User, sharedKey)
	if !utils.CompareBase64Sign(sign, body.Sign) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}

	if err := self.validJsonBodyTimeWindow(body); err != nil {
		return err
	}

	// Step 3: 有状态深度校验（时间窗、重放检测）后进入解密与业务处理（Plan0/1 无外层 Valid）
	if err := self.validReplayAttack(body.Sign); err != nil {
		return err
	}

	var rawData []byte
	if body.Plan == 0 { // 登录状态 P0 Base64
		rawData = utils.Base64Decode(body.Data)
		if len(rawData) == 0 {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "parameter Base64 parsing failed"}
		}
	} else { // 登录状态 P1 AES
		rawData, err = utils.AesGCMDecryptBase(body.Data, sharedKey[:32], AppendBodyMessage(self.Path, "", body.Nonce, body.Time, body.Plan, body.User))
		if err != nil {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "failed to parse data", Err: err}
		}
	}
	self.JsonBody.Data = utils.Bytes2Str(rawData)
	return nil
}

func (self *Context) validJsonBodyPlan2Flow(body *JsonBody) error {
	// Plan2 独立流程：
	// 1) ML-DSA 验签（提前拒绝伪造请求）
	// 2) 握手材料校验 + ML-KEM/HKDF 协商 sharedKey
	// 3) HMAC 验证（会话完整性）
	// 4) 时间窗 + 重放检查（有状态校验）
	// 5) AES-GCM 解密，并缓存 sharedKey 供响应复用
	cipher, exists := self.PQCipher[body.User]
	if !exists {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "plan2 cipher not found for user"}
	}
	cipher, err := self.CheckOuterSign(cipher, DigestBodyMessage(self.Path, body.Data, body.Nonce, body.Time, body.Plan, body.User), utils.Base64Decode(body.Valid))
	if err != nil {
		return err
	}

	// Step 3: 握手材料校验 + ECDH + HKDF 派生共享密钥
	sharedKey, err := self.negotiatePlan2SharedKey(body)
	if err != nil {
		return err
	}
	defer DIC.ClearData(sharedKey)

	// Step 4: HMAC 验证（会话完整性）
	sign := SignBodyMessage(self.Path, body.Data, body.Nonce, body.Time, body.Plan, body.User, sharedKey)
	if !utils.CompareBase64Sign(sign, body.Sign) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature invalid"}
	}

	if err := self.validJsonBodyTimeWindow(body); err != nil {
		return err
	}

	// Step 5: 有状态深度校验（时间窗、重放检测）后进入解密与业务处理。
	self.AddStorage(Cipher, cipher)
	if err := self.validReplayAttack(body.Sign); err != nil {
		return err
	}

	rawData, err := utils.AesGCMDecryptBase(body.Data, sharedKey[:32], AppendBodyMessage(self.Path, "", body.Nonce, body.Time, body.Plan, body.User))
	if err != nil {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "failed to parse data", Err: err}
	}
	// 复制一份秘钥，执行 defer 会清空当前秘钥
	copySharedKey := DIC.CopyData(sharedKey)
	self.AddStorage(SharedKey, copySharedKey)
	self.JsonBody.Data = utils.Bytes2Str(rawData)
	return nil
}

func (self *Context) validJsonBodyCommon(body *JsonBody) error {
	// Step 1 子步骤：仅做无状态、低成本的格式与边界检查。
	if len(body.Router) > 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request router invalid"}
	}
	if len(body.Data) == 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request data is nil"}
	}
	if !utils.CheckInt64(body.Plan, 0, 1, 2) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request plan invalid"}
	}
	if !utils.CheckLen(body.Nonce, 16, 64) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request nonce invalid"}
	}
	if body.Time <= 0 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time must be > 0"}
	}
	if self.RouterConfig.AesRequest && body.Plan != 1 {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use encryption"}
	}
	if !utils.CheckStrLen(body.Sign, 32, 64) {
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request signature length invalid"}
	}
	if JsonBodyRequiresOuterSignature(body.Plan, false) {
		if !fgocrypto.CheckOuterSignatureB64Valid(body.Valid) {
			return ex.Throw{Code: http.StatusBadRequest, Msg: "request outer signature length invalid"}
		}
	}
	return nil
}

func (self *Context) validJsonBodyTimeWindow(body *JsonBody) error {
	// Step 4 子步骤：签名通过后再做精确时间窗口校验，减少时间探测面。
	if utils.MathAbs(utils.UnixSecond()-body.Time) > jwt.FIVE_MINUTES { // 判断绝对时间差超过5分钟
		return ex.Throw{Code: http.StatusBadRequest, Msg: "request time invalid"}
	}
	return nil
}

func (self *Context) getPlan01DerivedKey(body *JsonBody) ([]byte, error) {
	if !utils.CheckInt64(body.Plan, 0, 1) {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request plan invalid"}
	}
	if len(self.GetRawTokenBytes()) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request header token is nil"}
	}
	sharedKey := self.GetTokenSecret()
	if len(sharedKey) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request token secret is nil"}
	}
	return sharedKey, nil
}

func (self *Context) negotiatePlan2SharedKey(body *JsonBody) ([]byte, error) {
	if !self.RouterConfig.UseRSA {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "request parameters must use plan2 (ML-KEM) route"}
	}
	authBs := self.RequestCtx.Request.Header.Peek(Authorization)
	if len(authBs) <= 0 || len(authBs) > fgocrypto.MaxPlan2AuthorizationB64Len() {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "client public key invalid"}
	}
	public := &PublicKey{}
	if err := utils.JsonUnmarshal(utils.Base64Decode(authBs), public); err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "client public key parse error", Err: err}
	}

	c, err := self.GetCacheObject()
	if err != nil {
		return nil, err
	}

	cipher, exists := self.PQCipher[body.User]
	if !exists {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "plan2 cipher not found for user"}
	}
	if err := CheckPublicKey(c, public, cipher); err != nil {
		return nil, err
	}

	cacheKey := utils.FNV1a64(utils.AddStr(public.Key, ":", public.Usr))
	var prkObject *PrivateKey
	if c.Mode() == cache.LOCAL {
		if v, b, err := c.Get(cacheKey, nil); err != nil || !b {
			return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "prk read error", Err: err}
		} else {
			prkObject = v.(*PrivateKey)
		}
	} else {
		prkObject = &PrivateKey{}
		if _, b, err := c.Get(cacheKey, prkObject); err != nil || !b {
			return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "prk read error", Err: err}
		}
	}
	defer c.Del(cacheKey)
	if len(prkObject.Key) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "prk read is nil"}
	}
	shared, err := fgocrypto.DecapsulatePeerCiphertext(prkObject.Key, public.Tag)
	if err != nil || len(shared) == 0 {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "ML-KEM shared key error", Err: err}
	}
	defer DIC.ClearData(shared)
	sharedKey, err := HKDFKey(shared, prkObject.Noc)
	if err != nil {
		return nil, ex.Throw{Code: http.StatusBadRequest, Msg: "shared key kdf error", Err: err}
	}
	return sharedKey, nil
}
