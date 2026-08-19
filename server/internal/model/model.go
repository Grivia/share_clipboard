package model

import "time"

type DeviceRole string

const (
	DeviceRoleSuperAdmin DeviceRole = "super_admin"
	DeviceRoleAdmin      DeviceRole = "admin"
	DeviceRoleMember     DeviceRole = "member"
)

func ValidAssignableDeviceRole(role DeviceRole) bool {
	return role == DeviceRoleAdmin || role == DeviceRoleMember
}

func CanRevokeDevice(actorRole, targetRole DeviceRole, sameDevice bool) bool {
	if sameDevice || targetRole == DeviceRoleSuperAdmin {
		return false
	}
	return actorRole == DeviceRoleSuperAdmin || actorRole == DeviceRoleAdmin
}

func CanChangeDeviceRole(actorRole, targetRole DeviceRole, sameDevice bool) bool {
	return !sameDevice && actorRole == DeviceRoleSuperAdmin && targetRole != DeviceRoleSuperAdmin
}

type DeviceInput struct {
	InstallationID string `json:"installation_id"`
	ReportedName   string `json:"reported_name"`
	Platform       string `json:"platform"`
	OSVersion      string `json:"os_version"`
	AppVersion     string `json:"app_version"`
}

type User struct {
	ID        string    `json:"id"`
	Account   string    `json:"account"`
	CreatedAt time.Time `json:"created_at"`
}

type Device struct {
	ID             string     `json:"id"`
	UserID         string     `json:"-"`
	InstallationID string     `json:"installation_id,omitempty"`
	ReportedName   string     `json:"reported_name"`
	CustomName     string     `json:"custom_name,omitempty"`
	DisplayName    string     `json:"display_name"`
	Platform       string     `json:"platform"`
	OSVersion      string     `json:"os_version"`
	AppVersion     string     `json:"app_version"`
	Role           DeviceRole `json:"role"`
	FirstLoginAt   time.Time  `json:"first_login_at"`
	LastLoginAt    time.Time  `json:"last_login_at"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	LoggedIn       bool       `json:"logged_in"`
	Online         bool       `json:"online"`
	Current        bool       `json:"current"`
	CanRevoke      bool       `json:"can_revoke"`
	CanChangeRole  bool       `json:"can_change_role"`
}

type Principal struct {
	UserID    string
	DeviceID  string
	SessionID string
	Role      DeviceRole
	ExpiresAt time.Time
}

type SessionTokens struct {
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type AuthResult struct {
	User   User          `json:"user"`
	Device Device        `json:"device"`
	Tokens SessionTokens `json:"tokens"`
}

type ClipUpload struct {
	ClientEventID string `json:"client_event_id"`
	ContentType   string `json:"content_type"`
	Algorithm     string `json:"algorithm"`
	Nonce         []byte `json:"-"`
	Ciphertext    []byte `json:"-"`
}

type ClipEvent struct {
	EventID        string    `json:"event_id"`
	ClientEventID  string    `json:"client_event_id"`
	Seq            int64     `json:"seq"`
	OriginDeviceID string    `json:"origin_device_id"`
	OriginName     string    `json:"origin_name"`
	ContentType    string    `json:"content_type"`
	Algorithm      string    `json:"algorithm"`
	Nonce          string    `json:"nonce"`
	Ciphertext     string    `json:"ciphertext"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type ClipCreateResult struct {
	Event   ClipEvent `json:"event"`
	Created bool      `json:"-"`
	Status  string    `json:"status"`
}
