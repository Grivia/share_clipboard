package api

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fastcopy/server/internal/hub"
	"fastcopy/server/internal/ids"
	"fastcopy/server/internal/model"
	"fastcopy/server/internal/password"
	"fastcopy/server/internal/store"
)

type authRequest struct {
	Account  string            `json:"account"`
	Password string            `json:"password"`
	Device   model.DeviceInput `json:"device"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (a *API) session(w http.ResponseWriter, r *http.Request) {
	var request authRequest
	if !decodeJSON(w, r, &request) || !validateAuthRequest(w, &request) {
		return
	}
	key := remoteIP(r) + "|" + request.Account
	if !a.loginLimiter.Allow(key) {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many authentication attempts")
		return
	}

	credentials, err := a.store.CredentialsByAccount(r.Context(), request.Account)
	if err == nil {
		result, isNew, loginErr := a.loginExisting(r, request, credentials)
		if loginErr != nil {
			if errors.Is(loginErr, store.ErrInvalidCredential) {
				a.invalidCredentials(w, r, request.Account)
			} else {
				storeError(w, loginErr)
			}
			return
		}
		a.loginLimiter.Reset(key)
		a.publishDeviceLogin(result, isNew)
		writeJSON(w, http.StatusOK, result)
		return
	}
	if !errors.Is(err, store.ErrInvalidCredential) {
		storeError(w, err)
		return
	}
	if !a.config.RegistrationEnabled {
		writeError(w, http.StatusForbidden, "REGISTRATION_DISABLED", "registration is disabled")
		return
	}

	hash, err := password.Hash(request.Password)
	if err != nil {
		storeError(w, err)
		return
	}
	tokens, err := store.NewTokenMaterial(time.Now().UTC(), a.config.AccessTokenTTL, a.config.RefreshTokenTTL)
	if err != nil {
		storeError(w, err)
		return
	}
	result, err := a.store.Register(
		r.Context(), request.Account, hash, request.Device, tokens, remoteIP(r), a.config.MaxUsers,
	)
	if err == nil {
		a.loginLimiter.Reset(key)
		writeJSON(w, http.StatusCreated, result)
		return
	}
	if !errors.Is(err, store.ErrAccountExists) {
		storeError(w, err)
		return
	}

	// Another first-login request may have created this account while the
	// password hash was being calculated. Treat that race as a normal login.
	credentials, err = a.store.CredentialsByAccount(r.Context(), request.Account)
	if err != nil {
		storeError(w, err)
		return
	}
	result, isNew, err := a.loginExisting(r, request, credentials)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCredential) {
			a.invalidCredentials(w, r, request.Account)
		} else {
			storeError(w, err)
		}
		return
	}
	a.loginLimiter.Reset(key)
	a.publishDeviceLogin(result, isNew)
	writeJSON(w, http.StatusOK, result)
}

func (a *API) loginExisting(
	r *http.Request,
	request authRequest,
	credentials store.UserCredentials,
) (model.AuthResult, bool, error) {
	if credentials.Status != "active" || !password.Verify(credentials.PasswordHash, request.Password) {
		return model.AuthResult{}, false, store.ErrInvalidCredential
	}
	tokens, err := store.NewTokenMaterial(time.Now().UTC(), a.config.AccessTokenTTL, a.config.RefreshTokenTTL)
	if err != nil {
		return model.AuthResult{}, false, err
	}
	return a.store.Login(r.Context(), credentials.User, request.Device, tokens, remoteIP(r))
}

func (a *API) invalidCredentials(w http.ResponseWriter, r *http.Request, account string) {
	a.store.RecordFailedLogin(r.Context(), account, remoteIP(r))
	writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "account or password is incorrect")
}

func (a *API) publishDeviceLogin(result model.AuthResult, isNew bool) {
	if !isNew {
		return
	}
	a.hub.Publish(result.User.ID, result.Device.ID, hub.Event{
		Type: "device.logged_in",
		Data: result.Device,
	})
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	var request refreshRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if len(request.RefreshToken) < 40 {
		writeError(w, http.StatusBadRequest, "INVALID_REFRESH_TOKEN", "refresh token is invalid")
		return
	}
	tokens, err := store.NewTokenMaterial(time.Now().UTC(), a.config.AccessTokenTTL, a.config.RefreshTokenTTL)
	if err != nil {
		storeError(w, err)
		return
	}
	result, err := a.store.Refresh(r.Context(), request.RefreshToken, tokens)
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": result})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	if err := a.store.Logout(r.Context(), principal.SessionID); err != nil {
		storeError(w, err)
		return
	}
	a.hub.CloseDevice(principal.UserID, principal.DeviceID)
	w.WriteHeader(http.StatusNoContent)
}

func validateAuthRequest(w http.ResponseWriter, request *authRequest) bool {
	request.Account = strings.TrimSpace(request.Account)
	request.Device.InstallationID = strings.TrimSpace(request.Device.InstallationID)
	request.Device.ReportedName = strings.TrimSpace(request.Device.ReportedName)
	request.Device.Platform = strings.ToLower(strings.TrimSpace(request.Device.Platform))
	request.Device.OSVersion = strings.TrimSpace(request.Device.OSVersion)
	request.Device.AppVersion = strings.TrimSpace(request.Device.AppVersion)

	accountLength := utf8.RuneCountInString(request.Account)
	if accountLength == 0 || accountLength > 128 || containsControlCharacter(request.Account) {
		writeError(w, http.StatusBadRequest, "INVALID_ACCOUNT", "account must contain 1 to 128 characters without control characters")
		return false
	}
	passwordLength := utf8.RuneCountInString(request.Password)
	if passwordLength < 4 || passwordLength > 256 || len(request.Password) > 1024 {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "password must contain 4 to 256 characters")
		return false
	}
	if !ids.IsUUID(request.Device.InstallationID) {
		writeError(w, http.StatusBadRequest, "INVALID_INSTALLATION_ID", "installation_id must be a UUID")
		return false
	}
	if request.Device.ReportedName == "" || len([]rune(request.Device.ReportedName)) > 64 {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_NAME", "device name must contain 1 to 64 characters")
		return false
	}
	allowedPlatforms := map[string]bool{
		"macos": true, "android": true, "windows": true, "linux": true, "ios": true,
	}
	if !allowedPlatforms[request.Device.Platform] {
		writeError(w, http.StatusBadRequest, "INVALID_PLATFORM", "platform is not supported")
		return false
	}
	if len(request.Device.OSVersion) > 64 || len(request.Device.AppVersion) > 64 {
		writeError(w, http.StatusBadRequest, "INVALID_DEVICE_METADATA", "device metadata is too long")
		return false
	}
	return true
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}
