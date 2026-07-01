package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
)

// Plan2WrapKeyEnv 用户自行设置的 AES 包装口令；程序只读取，不生成、不落盘。
const Plan2WrapKeyEnv = "MPC_PLAN2_WRAP_KEY"

const (
	Plan2DefaultProvisionDir = "plan2-provision"
	Plan2KeyFileExt          = ".key"
	Plan2PublicFileExt       = ".pem"

	plan2ClientNoSuffixMod  int64 = 10_000 // 与 open_scanner model.GenerateAppCipherNo 一致
	plan2ProvisionEncPrefix       = "plan2-provision-v1:"
	plan2ProvisionAAD             = "plan2-provision-v1"
)

// Plan2KeyPair ML-DSA-87 单端身份密钥对（Base64）。
type Plan2KeyPair struct {
	PrivateKeyB64 string
	PublicKeyB64  string
}

// Plan2Binding Plan2 双向身份：client 持有 Client 私钥 + Server 公钥；server 持有 Server 私钥 + Client 公钥。
type Plan2Binding struct {
	Client Plan2KeyPair
	Server Plan2KeyPair
}

// Plan2ProvisionResult WritePlan2KeyProvision 落盘结果。
type Plan2ProvisionResult struct {
	Dir       string
	ClientNo  int64
	Encrypted bool
}

// PublicFile 公钥文件名（{clientNo}.pem）。
func (r *Plan2ProvisionResult) PublicFile() string {
	if r == nil {
		return ""
	}
	return Plan2PublicFileName(r.ClientNo)
}

// PrivateFile 私钥文件名（{clientNo}.key）。
func (r *Plan2ProvisionResult) PrivateFile() string {
	if r == nil {
		return ""
	}
	return Plan2PrivateFileName(r.ClientNo)
}

// PublicPath 公钥文件完整路径。
func (r *Plan2ProvisionResult) PublicPath() string {
	if r == nil {
		return ""
	}
	return filepath.Join(r.Dir, r.PublicFile())
}

// PrivatePath 私钥文件完整路径。
func (r *Plan2ProvisionResult) PrivatePath() string {
	if r == nil {
		return ""
	}
	return filepath.Join(r.Dir, r.PrivateFile())
}

// Plan2PublicFileName 公钥文件名：{clientNo}.pem。
func Plan2PublicFileName(clientNo int64) string {
	return strconv.FormatInt(clientNo, 10) + Plan2PublicFileExt
}

// Plan2PrivateFileName 私钥文件名：{clientNo}.key。
func Plan2PrivateFileName(clientNo int64) string {
	return strconv.FormatInt(clientNo, 10) + Plan2KeyFileExt
}

// GeneratePlan2ClientNo 生成 PQC 证书编号（clientNo）：yyyyMMddHHmmss + 4 位随机数（共 18 位）。
// 算法与 open_scanner/model.GenerateAppCipherNo 一致，供 Plan2 Cipher 路由使用。
func GeneratePlan2ClientNo(at time.Time) (int64, error) {
	prefix, err := strconv.ParseInt(at.In(time.Local).Format("20060102150405"), 10, 64)
	if err != nil {
		return 0, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(plan2ClientNoSuffixMod))
	if err != nil {
		return 0, err
	}
	return prefix*plan2ClientNoSuffixMod + n.Int64(), nil
}

// GeneratePlan2KeyPair 随机生成一对 ML-DSA-87 密钥。
func GeneratePlan2KeyPair() (*Plan2KeyPair, error) {
	obj := &MLDSA87Object{}
	if err := obj.CreateMLDSA87(); err != nil {
		return nil, fmt.Errorf("generate ml-dsa-87 key pair: %w", err)
	}
	return &Plan2KeyPair{
		PrivateKeyB64: obj.PrivateKeyBase64,
		PublicKeyB64:  obj.PublicKeyBase64,
	}, nil
}

// GeneratePlan2Binding 生成 Plan2 双向绑定所需的两对 ML-DSA-87 密钥。
func GeneratePlan2Binding() (*Plan2Binding, error) {
	client, err := GeneratePlan2KeyPair()
	if err != nil {
		return nil, err
	}
	server, err := GeneratePlan2KeyPair()
	if err != nil {
		return nil, err
	}
	return &Plan2Binding{Client: *client, Server: *server}, nil
}

// ValidateMLDSA87PublicKeyB64 校验 ML-DSA-87 公钥 Base64 格式。
func ValidateMLDSA87PublicKeyB64(pubB64 string) error {
	pubB64 = strings.TrimSpace(pubB64)
	if pubB64 == "" {
		return fmt.Errorf("ml-dsa-87 public key is empty")
	}
	if _, err := ecc.LoadMLDSA87PublicKeyFromBase64(pubB64); err != nil {
		return fmt.Errorf("ml-dsa-87 public key invalid: %w", err)
	}
	return nil
}

// WritePlan2KeyProvision 写入 {clientNo}.pem、{clientNo}.key（各一行纯 Base64，无 JSON/PEM 头尾）。
// clientNo 为 0 时自动生成 18 位编号；wrapKey 非空时私钥文件为 AES-GCM 密文（plan2-provision-v1: 前缀）。
func WritePlan2KeyProvision(dir, wrapKey string, key *Plan2KeyPair) (*Plan2ProvisionResult, error) {
	return WritePlan2KeyProvisionWithClientNo(dir, wrapKey, 0, key)
}

// WritePlan2KeyProvisionWithClientNo 同 WritePlan2KeyProvision，可指定 clientNo（0 表示自动生成）。
func WritePlan2KeyProvisionWithClientNo(dir, wrapKey string, clientNo int64, key *Plan2KeyPair) (*Plan2ProvisionResult, error) {
	if key == nil {
		return nil, errors.New("plan2 key pair is nil")
	}
	if clientNo <= 0 {
		var err error
		clientNo, err = GeneratePlan2ClientNo(time.Now())
		if err != nil {
			return nil, fmt.Errorf("generate clientNo: %w", err)
		}
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = Plan2DefaultProvisionDir
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve provision dir: %w", err)
	}
	dir = absDir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	pubFile := Plan2PublicFileName(clientNo)
	privFile := Plan2PrivateFileName(clientNo)

	pubBody := []byte(strings.TrimSpace(key.PublicKeyB64) + "\n")
	if err := os.WriteFile(filepath.Join(dir, pubFile), pubBody, 0o644); err != nil {
		return nil, err
	}

	privBody := []byte(strings.TrimSpace(key.PrivateKeyB64) + "\n")
	wrapKey = strings.TrimSpace(wrapKey)
	if wrapKey == "" {
		if err := os.WriteFile(filepath.Join(dir, privFile), privBody, 0o600); err != nil {
			return nil, err
		}
		return &Plan2ProvisionResult{Dir: dir, ClientNo: clientNo, Encrypted: false}, nil
	}

	aesKey := utils.SHA256_BASE(utils.Str2Bytes(wrapKey))
	encB64, err := utils.AesGCMEncryptBase(privBody, aesKey, utils.Str2Bytes(plan2ProvisionAAD))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, privFile), []byte(plan2ProvisionEncPrefix+encB64), 0o600); err != nil {
		return nil, err
	}
	return &Plan2ProvisionResult{Dir: dir, ClientNo: clientNo, Encrypted: true}, nil
}

// ReadPlan2PrivateKey 读取 {clientNo}.key（明文或 AES 加密），返回一行 Base64 私钥。
func ReadPlan2PrivateKey(dir string, clientNo int64, wrapKey string) (string, error) {
	if clientNo <= 0 {
		return "", errors.New("clientNo is required")
	}
	raw, err := os.ReadFile(filepath.Join(dir, Plan2PrivateFileName(clientNo)))
	if err != nil {
		return "", err
	}
	name := Plan2PrivateFileName(clientNo)
	if bytes.HasPrefix(raw, []byte(plan2ProvisionEncPrefix)) {
		return decryptPlan2PrivateKey(raw, wrapKey, name)
	}
	return parseOneBase64Line(raw, name)
}

// ReadPlan2PublicKey 读取 {clientNo}.pem，返回一行 Base64 公钥。
func ReadPlan2PublicKey(dir string, clientNo int64) (string, error) {
	if clientNo <= 0 {
		return "", errors.New("clientNo is required")
	}
	raw, err := os.ReadFile(filepath.Join(dir, Plan2PublicFileName(clientNo)))
	if err != nil {
		return "", err
	}
	return parseOneBase64Line(raw, Plan2PublicFileName(clientNo))
}

func parseOneBase64Line(raw []byte, name string) (string, error) {
	line := strings.TrimSpace(string(raw))
	if line == "" {
		return "", fmt.Errorf("%s: empty", name)
	}
	if strings.Contains(line, "\n") {
		return "", fmt.Errorf("%s: expected single line base64", name)
	}
	return line, nil
}

func decryptPlan2PrivateKey(raw []byte, wrapKey, name string) (string, error) {
	wrapKey = strings.TrimSpace(wrapKey)
	if wrapKey == "" {
		return "", fmt.Errorf("%s: plan2 wrap key is required for encrypted private key", name)
	}
	if !bytes.HasPrefix(raw, []byte(plan2ProvisionEncPrefix)) {
		return "", fmt.Errorf("%s: invalid encrypted format", name)
	}
	encB64 := string(bytes.TrimPrefix(raw, []byte(plan2ProvisionEncPrefix)))
	aesKey := utils.SHA256_BASE(utils.Str2Bytes(wrapKey))
	plain, err := utils.AesGCMDecryptBase(encB64, aesKey, utils.Str2Bytes(plan2ProvisionAAD))
	if err != nil {
		return "", fmt.Errorf("%s decrypt: %w", name, err)
	}
	return parseOneBase64Line(plain, name)
}
