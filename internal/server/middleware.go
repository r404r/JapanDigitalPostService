package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type ctxKey int

const requestIDKey ctxKey = iota

// requestIDHeader 是入站/出站请求关联 id 的头名。
const requestIDHeader = "X-Request-Id"

// requestIDMiddleware 为每个请求建立一个关联 id：优先沿用客户端传入的
// X-Request-Id，否则生成一个随机 id。id 存入 context 并回写到响应头，
// 供日志与错误体（trace_id）引用，形成端到端可追踪链路。
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestIDFrom 取出当前请求的关联 id（无则返回空串）。
func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-unknown"
	}
	return hex.EncodeToString(b[:])
}

// chain 按从外到内的顺序套用中间件（mws[0] 最外层）。
func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
