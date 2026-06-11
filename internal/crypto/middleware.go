package crypto

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
)

// HeaderEncryption 在加密响应上标注算法，便于客户端识别需先解信封再解析。
const HeaderEncryption = "X-Payload-Encryption"

// Middleware 返回一个 HTTP 中间件。
//   - 未启用（none）时：直接放行，零开销，响应为明文 JSON。
//   - 启用（aes-gcm）时：缓冲下游写出的 JSON 响应体，整体用 AES-256-GCM 加密，
//     以信封 JSON 替换响应体，并设置 X-Payload-Encryption 头。状态码与非 JSON
//     响应（如已是错误信封/空体）保持不变。
//
// 仅作用于响应序列化层，不改接口语义/字段（见 architecture §8 / spec §6）。
func (c *Cipher) Middleware(next http.Handler) http.Handler {
	if !c.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cw := &captureWriter{header: http.Header{}, status: http.StatusOK, body: &bytes.Buffer{}}
		next.ServeHTTP(cw, r)

		// 仅封装 JSON 响应体；其余原样透传。
		ct := cw.header.Get("Content-Type")
		if cw.body.Len() == 0 || !strings.HasPrefix(ct, "application/json") {
			copyHeader(w.Header(), cw.header)
			w.WriteHeader(cw.status)
			_, _ = w.Write(cw.body.Bytes())
			return
		}

		envelope, err := c.EncryptJSON(cw.body.Bytes())
		if err != nil {
			// 加密失败绝不回退为明文（那会静默削弱安全保证）；返回安全的 500。
			slog.Error("payload encryption failed", "err", err)
			http.Error(w, `{"status":"internal_error","message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		copyHeader(w.Header(), cw.header)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(HeaderEncryption, algoName)
		w.Header().Del("Content-Length") // 长度已变，交由 server 重新计算
		w.WriteHeader(cw.status)
		_, _ = w.Write(envelope)
	})
}

// captureWriter 缓冲下游的响应以便整体加密。
type captureWriter struct {
	header      http.Header
	status      int
	body        *bytes.Buffer
	wroteHeader bool
}

func (c *captureWriter) Header() http.Header { return c.header }

func (c *captureWriter) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.status = status
	c.wroteHeader = true
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.body.Write(b)
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
