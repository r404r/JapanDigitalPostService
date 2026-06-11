package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Mode 是传输载荷加密模式。
//
//	none    —— 仅依赖 TLS（默认、推荐）。响应为明文 JSON，行为与未启用完全一致。
//	aes-gcm —— 在 TLS 之上对响应体再做 AES-256-GCM 应用层封装（少数高安全场景可选）。
//
// 决策依据见 docs/architecture.md §8。
type Mode string

const (
	ModeNone   Mode = "none"
	ModeAESGCM Mode = "aes-gcm"
)

// algoName 是写入信封并对客户端公布的算法标识。
const algoName = "AES-256-GCM"

// ErrDisabled 在加密未启用时调用 Encrypt/Decrypt 返回。
var ErrDisabled = errors.New("payload encryption disabled")

// keySize 是 AES-256 的密钥长度（字节）。
const keySize = 32

// Cipher 是可选的应用层载荷加密器。none 模式下 Enabled() 为 false，
// 所有封装逻辑短路，零开销。aes-gcm 模式下持有一个 AES-256-GCM AEAD。
//
// 不自研加密协议——仅在标准原语之上做可配置封装；每次随机 nonce 随密文传输；
// 密钥从环境/KMS 注入，绝不入库、绝不硬编码（见 architecture §8）。
type Cipher struct {
	mode  Mode
	aead  cipher.AEAD
	keyID string
}

// Envelope 是 aes-gcm 模式下响应体的对外结构，承载客户端解密所需的全部信息
// （算法、key id、nonce、密文），但绝不含密钥。约定写入 spec §6。
type Envelope struct {
	Enc        string `json:"enc"`           // 算法标识，如 "AES-256-GCM"
	KeyID      string `json:"kid,omitempty"` // 密钥标识，用于轮换
	Nonce      string `json:"nonce"`         // base64(标准) 编码的随机 nonce
	Ciphertext string `json:"ciphertext"`    // base64(标准) 编码的密文（含 GCM tag）
}

// New 按配置构造 Cipher。
//
//	mode    : "none" | "aes-gcm"
//	keyB64  : aes-gcm 模式下的 base64(标准) 编码 32 字节密钥（256 bit）
//	keyID   : 可选密钥标识，便于轮换
//
// 配置非法（未知 mode、密钥缺失/长度错误）返回错误——由启动流程处理，
// 错误文案不含密钥本身。
func New(mode Mode, keyB64, keyID string) (*Cipher, error) {
	switch mode {
	case "", ModeNone:
		return &Cipher{mode: ModeNone}, nil
	case ModeAESGCM:
		key, err := base64.StdEncoding.DecodeString(keyB64)
		if err != nil {
			return nil, errors.New("payload encryption key is not valid base64")
		}
		if len(key) != keySize {
			return nil, fmt.Errorf("payload encryption key must be %d bytes (got %d)", keySize, len(key))
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("init aes cipher: %w", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("init gcm: %w", err)
		}
		return &Cipher{mode: ModeAESGCM, aead: aead, keyID: keyID}, nil
	default:
		return nil, fmt.Errorf("unknown payload encryption mode %q", mode)
	}
}

// Enabled 报告是否启用了应用层加密。
func (c *Cipher) Enabled() bool { return c != nil && c.mode == ModeAESGCM }

// Encrypt 用 AES-256-GCM 加密 plaintext，返回可序列化的信封。每次生成新的
// 随机 nonce。未启用时返回 ErrDisabled。
func (c *Cipher) Encrypt(plaintext []byte) (*Envelope, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ct := c.aead.Seal(nil, nonce, plaintext, nil)
	return &Envelope{
		Enc:        algoName,
		KeyID:      c.keyID,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}, nil
}

// Decrypt 还原一个信封。失败（nonce/密文非法、认证不通过）返回统一错误，
// 不泄露具体原因。未启用时返回 ErrDisabled。该方法供客户端/测试使用。
func (c *Cipher) Decrypt(env *Envelope) ([]byte, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	if env == nil {
		return nil, errors.New("decrypt: empty envelope")
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil || len(nonce) != c.aead.NonceSize() {
		return nil, errors.New("decrypt: invalid envelope")
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, errors.New("decrypt: invalid envelope")
	}
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, errors.New("decrypt: authentication failed")
	}
	return pt, nil
}

// EncryptJSON 是便捷方法：加密一段明文并返回其信封的 JSON 字节。
func (c *Cipher) EncryptJSON(plaintext []byte) ([]byte, error) {
	env, err := c.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}
