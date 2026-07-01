package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ecc "github.com/godaddy-x/eccrypto"
	"github.com/godaddy-x/freego/utils"
)

// Plan2WrapKeyEnv 用户自行设置的 AES 包装口令；程序只读取，不生成、不落盘。
const Plan2WrapKeyEnv = "MPC_PLAN2_WRAP_KEY"

const (
	Plan2DefaultProvisionDir = "plan2-provision"
	Plan2PublicFile          = "public.pem"
	Plan2PrivateFile         = "private.key"

	plan2ProvisionEncPrefix = "plan2-provision-v1:"
	plan2ProvisionAAD       = "plan2-provision-v1"
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
	Encrypted bool
}

// PrivateFile 私钥文件名（固定 private.key）。
func (r *Plan2ProvisionResult) PrivateFile() string {
	return Plan2PrivateFile
}

// PublicPath 公钥文件完整路径。
func (r *Plan2ProvisionResult) PublicPath() string {
	if r == nil {
		return ""
	}
	return filepath.Join(r.Dir, Plan2PublicFile)
}

// PrivatePath 私钥文件完整路径。
func (r *Plan2ProvisionResult) PrivatePath() string {
	if r == nil {
		return ""
	}
	return filepath.Join(r.Dir, Plan2PrivateFile)
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

// WritePlan2KeyProvision 写入 public.pem、private.key（各一行纯 Base64，无 JSON/PEM 头尾）。
// wrapKey 非空时 private.key 为 AES-GCM 密文（plan2-provision-v1: 前缀），否则明文一行 Base64。
func WritePlan2KeyProvision(dir, wrapKey string, key *Plan2KeyPair) (*Plan2ProvisionResult, error) {
	if key == nil {
		return nil, errors.New("plan2 key pair is nil")
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

	pubBody := []byte(strings.TrimSpace(key.PublicKeyB64) + "\n")
	if err := os.WriteFile(filepath.Join(dir, Plan2PublicFile), pubBody, 0o644); err != nil {
		return nil, err
	}

	privBody := []byte(strings.TrimSpace(key.PrivateKeyB64) + "\n")
	wrapKey = strings.TrimSpace(wrapKey)
	if wrapKey == "" {
		if err := os.WriteFile(filepath.Join(dir, Plan2PrivateFile), privBody, 0o600); err != nil {
			return nil, err
		}
		return &Plan2ProvisionResult{Dir: dir, Encrypted: false}, nil
	}

	aesKey := utils.SHA256_BASE(utils.Str2Bytes(wrapKey))
	encB64, err := utils.AesGCMEncryptBase(privBody, aesKey, utils.Str2Bytes(plan2ProvisionAAD))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, Plan2PrivateFile), []byte(plan2ProvisionEncPrefix+encB64), 0o600); err != nil {
		return nil, err
	}
	return &Plan2ProvisionResult{Dir: dir, Encrypted: true}, nil
}

// ReadPlan2PrivateKey 读取 private.key（明文或 AES 加密），返回一行 Base64 私钥。
func ReadPlan2PrivateKey(dir, wrapKey string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, Plan2PrivateFile))
	if err != nil {
		return "", err
	}
	if bytes.HasPrefix(raw, []byte(plan2ProvisionEncPrefix)) {
		return decryptPlan2PrivateKey(raw, wrapKey)
	}
	return parseOneBase64Line(raw, "private.key")
}

// ReadPlan2PublicKey 读取 public.pem，返回一行 Base64 公钥。
func ReadPlan2PublicKey(dir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, Plan2PublicFile))
	if err != nil {
		return "", err
	}
	return parseOneBase64Line(raw, "public.pem")
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

func decryptPlan2PrivateKey(raw []byte, wrapKey string) (string, error) {
	wrapKey = strings.TrimSpace(wrapKey)
	if wrapKey == "" {
		return "", errors.New("plan2 wrap key is required for encrypted private.key")
	}
	if !bytes.HasPrefix(raw, []byte(plan2ProvisionEncPrefix)) {
		return "", errors.New("private.key: invalid encrypted format")
	}
	encB64 := string(bytes.TrimPrefix(raw, []byte(plan2ProvisionEncPrefix)))
	aesKey := utils.SHA256_BASE(utils.Str2Bytes(wrapKey))
	plain, err := utils.AesGCMDecryptBase(encB64, aesKey, utils.Str2Bytes(plan2ProvisionAAD))
	if err != nil {
		return "", fmt.Errorf("private.key decrypt: %w", err)
	}
	return parseOneBase64Line(plain, "private.key")
}
