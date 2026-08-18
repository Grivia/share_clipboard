package api

import (
	"net/http"
	"time"

	"fastcopy/server/internal/config"
	"fastcopy/server/internal/hub"
	"fastcopy/server/internal/store"
)

type API struct {
	config       config.Config
	store        *store.Store
	hub          *hub.Hub
	loginLimiter *limiter
}

func New(cfg config.Config, dataStore *store.Store, connectionHub *hub.Hub) *API {
	return &API{
		config:       cfg,
		store:        dataStore,
		hub:          connectionHub,
		loginLimiter: newLimiter(10, 15*time.Minute),
	}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /v1/auth/session", a.session)
	mux.HandleFunc("POST /v1/auth/refresh", a.refresh)
	mux.HandleFunc("POST /v1/auth/logout", a.authenticate(a.logout))
	mux.HandleFunc("GET /v1/devices", a.authenticate(a.devices))
	mux.HandleFunc("PATCH /v1/devices/{deviceID}", a.authenticate(a.renameDevice))
	mux.HandleFunc("POST /v1/devices/{deviceID}/revoke", a.authenticate(a.revokeDevice))
	mux.HandleFunc("POST /v1/clips", a.authenticate(a.createClip))
	mux.HandleFunc("GET /v1/clips", a.authenticate(a.clips))
	mux.HandleFunc("POST /v1/sync/ack", a.authenticate(a.ack))
	mux.HandleFunc("GET /v1/events/ws", a.authenticate(a.eventsWebSocket))
	return requestLogger(securityHeaders(mux))
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := a.store.Healthy(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "database is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}
