package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/auth"
)

func authenticate(tokens *auth.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if raw == "" {
				writeResponse(w, Error(401, "未登录或token已过期"))
				return
			}
			claims, err := tokens.Validate(raw)
			if err != nil {
				writeResponse(w, Error(401, "无效的token或token已过期"))
				return
			}
			next.ServeHTTP(w, withClaims(r, claims))
		})
	}
}

func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := claimsFrom(r)
		if !ok {
			writeResponse(w, Error(401, "无法获取用户权限信息"))
			return
		}
		if claims.RoleID != 0 {
			writeResponse(w, Error(403, "权限不足，仅管理员可操作"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if value := recover(); value != nil {
					logger.Error("request panic", "method", r.Method, "path", r.URL.Path, "panic", value, "stack", string(debug.Stack()))
					writeResponse(w, Error(-2, "服务内部错误"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			if r.URL.Path == "/system-info" {
				next.ServeHTTP(w, r)
				logger.Info("websocket request ended", "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
				return
			}
			wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(wrapped, r)
			logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", wrapped.status, "duration_ms", time.Since(started).Milliseconds())
		})
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
