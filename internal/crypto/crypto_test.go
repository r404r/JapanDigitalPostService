package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testKeyB64(t *testing.T) string {
	t.Helper()
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestNew_Modes(t *testing.T) {
	if c, err := New(ModeNone, "", ""); err != nil || c.Enabled() {
		t.Fatalf("none: err=%v enabled=%v", err, c.Enabled())
	}
	if c, err := New("", "", ""); err != nil || c.Enabled() {
		t.Fatalf("empty defaults to none: err=%v enabled=%v", err, c.Enabled())
	}
	if c, err := New(ModeAESGCM, testKeyB64(t), "k1"); err != nil || !c.Enabled() {
		t.Fatalf("aes-gcm: err=%v enabled=%v", err, c.Enabled())
	}
	// 非法配置
	if _, err := New(ModeAESGCM, "not-base64!!!", ""); err == nil {
		t.Error("expected error for bad base64 key")
	}
	if _, err := New(ModeAESGCM, base64.StdEncoding.EncodeToString([]byte("short")), ""); err == nil {
		t.Error("expected error for wrong key length")
	}
	if _, err := New(Mode("rot13"), "", ""); err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	c, _ := New(ModeAESGCM, testKeyB64(t), "k1")
	plaintext := []byte(`{"status":"ok","total_count":3}`)

	env, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if env.Enc != algoName || env.KeyID != "k1" {
		t.Errorf("envelope metadata: %+v", env)
	}
	// 密文不得包含明文
	if strings.Contains(env.Ciphertext, "total_count") {
		t.Error("ciphertext leaked plaintext")
	}
	got, err := c.Decrypt(env)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("roundtrip mismatch: %q", got)
	}
}

func TestEncrypt_NonceUnique(t *testing.T) {
	c, _ := New(ModeAESGCM, testKeyB64(t), "")
	pt := []byte("same plaintext")
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		env, err := c.Encrypt(pt)
		if err != nil {
			t.Fatal(err)
		}
		if seen[env.Nonce] {
			t.Fatalf("nonce reused: %s", env.Nonce)
		}
		seen[env.Nonce] = true
	}
}

func TestDecrypt_TamperFails(t *testing.T) {
	c, _ := New(ModeAESGCM, testKeyB64(t), "")
	env, _ := c.Encrypt([]byte("secret"))
	// 篡改密文
	bad := *env
	ctBytes, _ := base64.StdEncoding.DecodeString(env.Ciphertext)
	ctBytes[0] ^= 0xff
	bad.Ciphertext = base64.StdEncoding.EncodeToString(ctBytes)
	if _, err := c.Decrypt(&bad); err == nil {
		t.Error("tampered ciphertext should fail authentication")
	}
	// 错误的 nonce
	bad2 := *env
	bad2.Nonce = base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := c.Decrypt(&bad2); err == nil {
		t.Error("invalid nonce should fail")
	}
}

func TestDisabled_NoOp(t *testing.T) {
	c, _ := New(ModeNone, "", "")
	if _, err := c.Encrypt([]byte("x")); err != ErrDisabled {
		t.Errorf("disabled encrypt: %v", err)
	}
	if _, err := c.Decrypt(&Envelope{}); err != ErrDisabled {
		t.Errorf("disabled decrypt: %v", err)
	}
}

func TestMiddleware_DisabledPassthrough(t *testing.T) {
	c, _ := New(ModeNone, "", "")
	body := `{"status":"ok"}`
	h := c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != body {
		t.Errorf("none mode should pass body through unchanged, got %q", rec.Body.String())
	}
	if rec.Header().Get(HeaderEncryption) != "" {
		t.Error("none mode should not set encryption header")
	}
}

func TestMiddleware_EnabledEncryptsJSON(t *testing.T) {
	key := testKeyB64(t)
	c, _ := New(ModeAESGCM, key, "k1")
	plain := `{"status":"ok","version":"0.1.0"}`
	h := c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(plain))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get(HeaderEncryption) != algoName {
		t.Errorf("missing %s header", HeaderEncryption)
	}
	if strings.Contains(rec.Body.String(), "version") {
		t.Error("response body leaked plaintext")
	}
	// 客户端按信封约定解密应还原原始 JSON。
	var env Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not an envelope: %v", err)
	}
	got, err := c.Decrypt(&env)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != plain {
		t.Errorf("decrypted = %q, want %q", got, plain)
	}
}

func TestMiddleware_NonJSONPassthrough(t *testing.T) {
	c, _ := New(ModeAESGCM, testKeyB64(t), "")
	h := c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("plain text"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "plain text" {
		t.Errorf("non-JSON should pass through, got %q", rec.Body.String())
	}
}
