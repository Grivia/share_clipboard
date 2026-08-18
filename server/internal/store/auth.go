package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fastcopy/server/internal/ids"
	"fastcopy/server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrAccountExists     = errors.New("account already registered")
	ErrUserLimitReached  = errors.New("user limit reached")
	ErrInvalidCredential = errors.New("invalid credentials")
	ErrSessionExpired    = errors.New("session expired")
)

type UserCredentials struct {
	User         model.User
	PasswordHash string
	Status       string
}

type TokenMaterial struct {
	AccessToken      string
	AccessHash       []byte
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshHash      []byte
	RefreshExpiresAt time.Time
}

func NewTokenMaterial(now time.Time, accessTTL, refreshTTL time.Duration) (TokenMaterial, error) {
	access, accessHash, err := ids.Token()
	if err != nil {
		return TokenMaterial{}, err
	}
	refresh, refreshHash, err := ids.Token()
	if err != nil {
		return TokenMaterial{}, err
	}
	return TokenMaterial{
		AccessToken:      access,
		AccessHash:       accessHash,
		AccessExpiresAt:  now.Add(accessTTL),
		RefreshToken:     refresh,
		RefreshHash:      refreshHash,
		RefreshExpiresAt: now.Add(refreshTTL),
	}, nil
}

func (m TokenMaterial) Public() model.SessionTokens {
	return model.SessionTokens{
		AccessToken:      m.AccessToken,
		AccessExpiresAt:  m.AccessExpiresAt,
		RefreshToken:     m.RefreshToken,
		RefreshExpiresAt: m.RefreshExpiresAt,
	}
}

func (s *Store) UserLimitReached(ctx context.Context, maxUsers int) (bool, error) {
	if maxUsers <= 0 {
		return false, nil
	}
	var userCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		return false, err
	}
	return userCount >= maxUsers, nil
}

func (s *Store) Register(
	ctx context.Context,
	account string,
	passwordHash string,
	device model.DeviceInput,
	tokens TokenMaterial,
	remoteIP string,
	maxUsers int,
) (model.AuthResult, error) {
	now := time.Now().UTC()
	userID, err := ids.UUID()
	if err != nil {
		return model.AuthResult{}, err
	}
	deviceID, err := ids.UUID()
	if err != nil {
		return model.AuthResult{}, err
	}
	sessionID, err := ids.UUID()
	if err != nil {
		return model.AuthResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.AuthResult{}, err
	}
	defer tx.Rollback(ctx)
	if maxUsers > 0 {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1178686275, 1)`); err != nil {
			return model.AuthResult{}, fmt.Errorf("lock registration: %w", err)
		}
		var accountExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE account = $1)`, account).Scan(&accountExists); err != nil {
			return model.AuthResult{}, fmt.Errorf("check account: %w", err)
		}
		if accountExists {
			return model.AuthResult{}, ErrAccountExists
		}
		var userCount int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
			return model.AuthResult{}, fmt.Errorf("count users: %w", err)
		}
		if userCount >= maxUsers {
			return model.AuthResult{}, ErrUserLimitReached
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, account, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)`, userID, account, passwordHash, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_account_unique" {
			return model.AuthResult{}, ErrAccountExists
		}
		return model.AuthResult{}, fmt.Errorf("insert user: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO devices (
			id, user_id, installation_id, reported_name, platform, os_version,
			app_version, first_login_at, last_login_at, last_seen_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $8)`,
		deviceID, userID, device.InstallationID, device.ReportedName, device.Platform,
		device.OSVersion, device.AppVersion, now)
	if err != nil {
		return model.AuthResult{}, fmt.Errorf("insert device: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO device_cursors (device_id) VALUES ($1)`, deviceID); err != nil {
		return model.AuthResult{}, fmt.Errorf("insert cursor: %w", err)
	}
	if err := insertSession(ctx, tx, sessionID, userID, deviceID, tokens, now); err != nil {
		return model.AuthResult{}, err
	}
	if err := insertLoginEvent(ctx, tx, userID, deviceID, account, true, remoteIP); err != nil {
		return model.AuthResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AuthResult{}, err
	}

	return model.AuthResult{
		User: model.User{ID: userID, Account: account, CreatedAt: now},
		Device: model.Device{
			ID: deviceID, UserID: userID, InstallationID: device.InstallationID,
			ReportedName: device.ReportedName, DisplayName: device.ReportedName,
			Platform: device.Platform, OSVersion: device.OSVersion,
			AppVersion: device.AppVersion, FirstLoginAt: now, LastLoginAt: now,
			LastSeenAt: &now, LoggedIn: true, Online: false, Current: true,
		},
		Tokens: tokens.Public(),
	}, nil
}

func (s *Store) CredentialsByAccount(ctx context.Context, account string) (UserCredentials, error) {
	var credentials UserCredentials
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, account, password_hash, status, created_at
		FROM users WHERE account = $1`, account,
	).Scan(
		&credentials.User.ID,
		&credentials.User.Account,
		&credentials.PasswordHash,
		&credentials.Status,
		&credentials.User.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserCredentials{}, ErrInvalidCredential
	}
	return credentials, err
}

func (s *Store) Login(
	ctx context.Context,
	user model.User,
	device model.DeviceInput,
	tokens TokenMaterial,
	remoteIP string,
) (model.AuthResult, bool, error) {
	now := time.Now().UTC()
	newDeviceID, err := ids.UUID()
	if err != nil {
		return model.AuthResult{}, false, err
	}
	sessionID, err := ids.UUID()
	if err != nil {
		return model.AuthResult{}, false, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.AuthResult{}, false, err
	}
	defer tx.Rollback(ctx)

	var result model.Device
	var isNew bool
	err = tx.QueryRow(ctx, `
		WITH existing AS (
			SELECT id FROM devices WHERE user_id = $1 AND installation_id = $2
		), upserted AS (
			INSERT INTO devices (
				id, user_id, installation_id, reported_name, platform, os_version,
				app_version, first_login_at, last_login_at, last_seen_at
			) VALUES ($3, $1, $2, $4, $5, $6, $7, $8, $8, $8)
			ON CONFLICT (user_id, installation_id) DO UPDATE SET
				reported_name = EXCLUDED.reported_name,
				platform = EXCLUDED.platform,
				os_version = EXCLUDED.os_version,
				app_version = EXCLUDED.app_version,
				last_login_at = EXCLUDED.last_login_at,
				last_seen_at = EXCLUDED.last_seen_at,
				revoked_at = NULL
			RETURNING id, user_id, installation_id, reported_name, custom_name,
				platform, os_version, app_version, first_login_at, last_login_at,
				last_seen_at, revoked_at
		)
		SELECT u.id::text, u.user_id::text, u.installation_id::text,
			u.reported_name, u.custom_name,
			COALESCE(NULLIF(u.custom_name, ''), u.reported_name),
			u.platform, u.os_version, u.app_version, u.first_login_at,
			u.last_login_at, u.last_seen_at, u.revoked_at,
			NOT EXISTS (SELECT 1 FROM existing)
		FROM upserted u`,
		user.ID, device.InstallationID, newDeviceID, device.ReportedName,
		device.Platform, device.OSVersion, device.AppVersion, now,
	).Scan(
		&result.ID, &result.UserID, &result.InstallationID, &result.ReportedName,
		&result.CustomName, &result.DisplayName, &result.Platform, &result.OSVersion,
		&result.AppVersion, &result.FirstLoginAt, &result.LastLoginAt,
		&result.LastSeenAt, &result.RevokedAt, &isNew,
	)
	if err != nil {
		return model.AuthResult{}, false, fmt.Errorf("upsert device: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = $2
		WHERE device_id = $1 AND revoked_at IS NULL`, result.ID, now)
	if err != nil {
		return model.AuthResult{}, false, fmt.Errorf("revoke old sessions: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO device_cursors (device_id) VALUES ($1)
		ON CONFLICT (device_id) DO NOTHING`, result.ID)
	if err != nil {
		return model.AuthResult{}, false, fmt.Errorf("ensure cursor: %w", err)
	}
	if err := insertSession(ctx, tx, sessionID, user.ID, result.ID, tokens, now); err != nil {
		return model.AuthResult{}, false, err
	}
	if err := insertLoginEvent(ctx, tx, user.ID, result.ID, user.Account, true, remoteIP); err != nil {
		return model.AuthResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AuthResult{}, false, err
	}
	result.LoggedIn = true
	result.Current = true
	return model.AuthResult{User: user, Device: result, Tokens: tokens.Public()}, isNew, nil
}

func (s *Store) RecordFailedLogin(ctx context.Context, account, remoteIP string) {
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO login_events (account_hash, success, remote_ip)
		VALUES ($1, false, $2)`, ids.DigestString(account), remoteIP)
}

func (s *Store) Authenticate(ctx context.Context, accessToken string) (model.Principal, error) {
	var principal model.Principal
	err := s.pool.QueryRow(ctx, `
		SELECT s.user_id::text, s.device_id::text, s.id::text, s.access_expires_at
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		JOIN devices d ON d.id = s.device_id
		WHERE s.access_token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.access_expires_at > now()
		  AND u.status = 'active'
		  AND d.revoked_at IS NULL`, ids.DigestString(accessToken),
	).Scan(&principal.UserID, &principal.DeviceID, &principal.SessionID, &principal.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Principal{}, ErrSessionExpired
	}
	return principal, err
}

func (s *Store) Refresh(
	ctx context.Context,
	refreshToken string,
	tokens TokenMaterial,
) (model.SessionTokens, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.SessionTokens{}, err
	}
	defer tx.Rollback(ctx)

	var sessionID string
	err = tx.QueryRow(ctx, `
		SELECT s.id::text
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		JOIN devices d ON d.id = s.device_id
		WHERE s.refresh_token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.refresh_expires_at > now()
		  AND u.status = 'active'
		  AND d.revoked_at IS NULL
		FOR UPDATE`, ids.DigestString(refreshToken),
	).Scan(&sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.SessionTokens{}, ErrSessionExpired
	}
	if err != nil {
		return model.SessionTokens{}, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE auth_sessions SET
			access_token_hash = $2,
			refresh_token_hash = $3,
			access_expires_at = $4,
			refresh_expires_at = $5,
			last_used_at = now()
		WHERE id = $1`, sessionID, tokens.AccessHash, tokens.RefreshHash,
		tokens.AccessExpiresAt, tokens.RefreshExpiresAt)
	if err != nil {
		return model.SessionTokens{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SessionTokens{}, err
	}
	return tokens.Public(), nil
}

func (s *Store) Logout(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, sessionID)
	return err
}

func insertSession(
	ctx context.Context,
	tx pgx.Tx,
	sessionID, userID, deviceID string,
	tokens TokenMaterial,
	now time.Time,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO auth_sessions (
			id, user_id, device_id, access_token_hash, refresh_token_hash,
			access_expires_at, refresh_expires_at, created_at, last_used_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		sessionID, userID, deviceID, tokens.AccessHash, tokens.RefreshHash,
		tokens.AccessExpiresAt, tokens.RefreshExpiresAt, now)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func insertLoginEvent(
	ctx context.Context,
	tx pgx.Tx,
	userID, deviceID, account string,
	success bool,
	remoteIP string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO login_events (
			user_id, device_id, account_hash, success, remote_ip
		) VALUES ($1, $2, $3, $4, $5)`, userID, deviceID,
		ids.DigestString(account), success, remoteIP)
	return err
}
