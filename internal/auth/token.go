package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenPrefix 是所有明文 token 的可见前缀，便于在日志/UI 中识别来源，
// 同时让前 8 位（"jdps_" + 3 随机字符）作为 Token.Prefix 存库。
const tokenPrefix = "jdps_"

// tokenEntropyBytes 是随机部分的字节数。32 字节 = 256 bit，base64url 后
// 约 43 个字符，足以抵抗暴力枚举。
const tokenEntropyBytes = 32

// generatePlaintext 生成一个高熵明文 token。返回的字符串仅在发行时返回给
// 调用方一次，绝不落库。
func generatePlaintext() (string, error) {
	buf := make([]byte, tokenEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token entropy: %w", err)
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashToken 计算明文 token 的 SHA-256（hex 编码）。存库与比对都只用 hash。
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// prefixOf 取明文 token 的前 8 位作为可显示前缀（不足则取全部）。
func prefixOf(plaintext string) string {
	if len(plaintext) <= 8 {
		return plaintext
	}
	return plaintext[:8]
}

// newUUID 生成一个 RFC 4122 v4 UUID 字符串。骨架阶段仅依赖标准库；引入
// google/uuid 后可替换，对外形状不变。
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
