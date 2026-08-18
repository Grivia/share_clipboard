package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"fastcopy/server/internal/model"
	"fastcopy/server/internal/store"
)

type contextKey string

const principalKey contextKey = "principal"

func (a *API) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "access token is required")
			return
		}
		principal, err := a.store.Authenticate(r.Context(), strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "access token is invalid or expired")
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, principal)
		next(w, r.WithContext(ctx))
	}
}

func principalFrom(r *http.Request) model.Principal {
	return r.Context().Value(principalKey).(model.Principal)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func remoteIP(r *http.Request) string {
	if value := r.Header.Get("CF-Connecting-IP"); value != "" {
		return value
	}
	if value := r.Header.Get("X-Forwarded-For"); value != "" {
		if first, _, ok := strings.Cut(value, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(value)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func storeError(w http.ResponseWriter, err error) {
	switch err {
	case store.ErrAccountExists:
		writeError(w, http.StatusConflict, "ACCOUNT_EXISTS", "account is already registered")
	case store.ErrUserLimitReached:
		writeError(w, http.StatusForbidden, "REGISTRATION_LIMIT_REACHED", "the server is not accepting more accounts")
	case store.ErrInvalidCredential:
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "account or password is incorrect")
	case store.ErrSessionExpired:
		writeError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "session is invalid or expired")
	case store.ErrDeviceNotFound:
		writeError(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "device was not found")
	case store.ErrEventIDReused:
		writeError(w, http.StatusConflict, "CLIENT_EVENT_ID_REUSED", "client_event_id was already used with different content")
	default:
		slog.Error("request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "the server could not complete the request")
	}
}
