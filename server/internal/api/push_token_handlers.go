package api

import (
	"net/http"
	"strings"
)

type apnsTokenRequest struct {
	Token       string `json:"token"`
	Environment string `json:"environment"`
}

func (a *API) putAPNsToken(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	var request apnsTokenRequest
	if !decodeJSON(w, r, &request) || !validateAPNsToken(w, &request) {
		return
	}
	if err := a.store.UpsertAPNsToken(
		r.Context(), principal.UserID, principal.DeviceID,
		request.Token, request.Environment,
	); err != nil {
		storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) deleteAPNsToken(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	if err := a.store.DeleteAPNsToken(r.Context(), principal.UserID, principal.DeviceID); err != nil {
		storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateAPNsToken(w http.ResponseWriter, request *apnsTokenRequest) bool {
	request.Token = strings.ToLower(strings.TrimSpace(request.Token))
	request.Environment = strings.ToLower(strings.TrimSpace(request.Environment))
	if request.Environment != "sandbox" && request.Environment != "production" {
		writeError(w, http.StatusBadRequest, "INVALID_PUSH_ENVIRONMENT", "environment must be sandbox or production")
		return false
	}
	if len(request.Token) < 32 || len(request.Token) > 512 || len(request.Token)%2 != 0 {
		writeError(w, http.StatusBadRequest, "INVALID_PUSH_TOKEN", "APNs token is invalid")
		return false
	}
	for _, character := range request.Token {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			writeError(w, http.StatusBadRequest, "INVALID_PUSH_TOKEN", "APNs token is invalid")
			return false
		}
	}
	return true
}
