package api

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"fastcopy/server/internal/hub"
	"fastcopy/server/internal/ids"
	"fastcopy/server/internal/model"
)

type clipUploadRequest struct {
	ClientEventID string `json:"client_event_id"`
	ContentType   string `json:"content_type"`
	Algorithm     string `json:"algorithm"`
	Nonce         string `json:"nonce"`
	Ciphertext    string `json:"ciphertext"`
}

type ackRequest struct {
	Seq int64 `json:"seq"`
}

func (a *API) createClip(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	var request clipUploadRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !ids.IsUUID(request.ClientEventID) {
		writeError(w, http.StatusBadRequest, "INVALID_CLIENT_EVENT_ID", "client_event_id must be a UUID")
		return
	}
	if request.ContentType != "text/plain" || request.Algorithm != "AES-256-GCM" {
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_ENVELOPE", "only AES-256-GCM text/plain envelopes are supported")
		return
	}
	nonce, err := base64.StdEncoding.DecodeString(request.Nonce)
	if err != nil || len(nonce) != 12 {
		writeError(w, http.StatusBadRequest, "INVALID_NONCE", "nonce must be 12 bytes encoded with base64")
		return
	}
	ciphertext, err := base64.StdEncoding.DecodeString(request.Ciphertext)
	if err != nil || len(ciphertext) < 16 || len(ciphertext) > 256*1024 {
		writeError(w, http.StatusBadRequest, "INVALID_CIPHERTEXT", "ciphertext must contain 16 to 262144 bytes")
		return
	}
	result, err := a.store.CreateClip(r.Context(), principal, model.ClipUpload{
		ClientEventID: request.ClientEventID,
		ContentType:   request.ContentType,
		Algorithm:     request.Algorithm,
		Nonce:         nonce,
		Ciphertext:    ciphertext,
	}, a.config.ClipTTL, a.config.IdempotencyTTL)
	if err != nil {
		storeError(w, err)
		return
	}
	if result.Created {
		a.hub.Publish(principal.UserID, principal.DeviceID, hub.Event{
			Type: "clip.created",
			Data: result.Event,
		})
		if a.pushNotifier != nil {
			a.pushNotifier.ClipCreated(principal.UserID, principal.DeviceID, result.Event.Seq)
		}
		writeJSON(w, http.StatusCreated, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) clips(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	afterSeq, err := strconv.ParseInt(r.URL.Query().Get("after_seq"), 10, 64)
	if err != nil && r.URL.Query().Get("after_seq") != "" {
		writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "after_seq must be an integer")
		return
	}
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 200")
			return
		}
		limit = parsed
	}
	clips, err := a.store.ClipsAfter(r.Context(), principal.UserID, afterSeq, limit)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"clips": clips})
}

func (a *API) ack(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	var request ackRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Seq < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "seq cannot be negative")
		return
	}
	if err := a.store.Ack(r.Context(), principal.DeviceID, request.Seq); err != nil {
		storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
