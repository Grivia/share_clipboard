package main

import "time"

const daemonVersion = "0.3.3"

type UserConfig struct {
	Enabled   bool   `json:"enabled"`
	ServerURL string `json:"server_url"`
	Account   string `json:"account"`
	Password  string `json:"password,omitempty"`
}

type RuntimeState struct {
	InstallationID     string `json:"installation_id"`
	AccountFingerprint string `json:"account_fingerprint"`
	AccessToken        string `json:"access_token"`
	RefreshToken       string `json:"refresh_token"`
	UserID             string `json:"user_id"`
	DeviceID           string `json:"device_id"`
	SharedKey          string `json:"shared_key"`
	KeyVersion         int    `json:"key_version"`
	LastSeq            int64  `json:"last_seq"`
}

type DeviceInput struct {
	InstallationID string `json:"installation_id"`
	ReportedName   string `json:"reported_name"`
	Platform       string `json:"platform"`
	OSVersion      string `json:"os_version"`
	AppVersion     string `json:"app_version"`
}

type AuthRequest struct {
	Account  string      `json:"account"`
	Password string      `json:"password"`
	Device   DeviceInput `json:"device"`
}

type AuthResponse struct {
	User struct {
		ID      string `json:"id"`
		Account string `json:"account"`
	} `json:"user"`
	Device struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"device"`
	Tokens SessionTokens `json:"tokens"`
}

type SessionTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type RefreshResponse struct {
	Tokens SessionTokens `json:"tokens"`
}

type ClipUpload struct {
	ClientEventID string `json:"client_event_id"`
	ContentType   string `json:"content_type"`
	Algorithm     string `json:"algorithm"`
	Nonce         string `json:"nonce"`
	Ciphertext    string `json:"ciphertext"`
}

type ClipEvent struct {
	EventID        string `json:"event_id"`
	ClientEventID  string `json:"client_event_id"`
	Seq            int64  `json:"seq"`
	OriginDeviceID string `json:"origin_device_id"`
	OriginName     string `json:"origin_name"`
	ContentType    string `json:"content_type"`
	Algorithm      string `json:"algorithm"`
	Nonce          string `json:"nonce"`
	Ciphertext     string `json:"ciphertext"`
}

type ClipCreateResponse struct {
	Event  ClipEvent `json:"event"`
	Status string    `json:"status"`
}

type ClipsResponse struct {
	Clips []ClipEvent `json:"clips"`
}

type DeviceSummary struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Platform    string `json:"platform"`
	OSVersion   string `json:"os_version"`
	AppVersion  string `json:"app_version"`
	Online      bool   `json:"online"`
	Current     bool   `json:"current"`
}

type DevicesResponse struct {
	Devices []DeviceSummary `json:"devices"`
}

type APIErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type DaemonStatus struct {
	State             string          `json:"state"`
	Message           string          `json:"message"`
	Connected         bool            `json:"connected"`
	Authenticated     bool            `json:"authenticated"`
	LastSyncAt        time.Time       `json:"last_sync_at,omitempty"`
	Pending           int             `json:"pending"`
	DeviceID          string          `json:"device_id,omitempty"`
	OnlineDevices     []DeviceSummary `json:"online_devices"`
	DevicesLoaded     bool            `json:"devices_loaded"`
	DevicesRefreshing bool            `json:"devices_refreshing"`
	DevicesError      string          `json:"devices_error,omitempty"`
	DevicesUpdatedAt  *time.Time      `json:"devices_updated_at,omitempty"`
	Version           string          `json:"version"`
	UpdatedAt         time.Time       `json:"updated_at"`
}
