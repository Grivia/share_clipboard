package store

import (
	"context"
	"errors"
	"time"

	"fastcopy/server/internal/model"

	"github.com/jackc/pgx/v5"
)

var (
	ErrDeviceNotFound    = errors.New("device not found")
	ErrDevicePermission  = errors.New("device permission denied")
	ErrInvalidDeviceRole = errors.New("invalid device role")
)

func (s *Store) Devices(ctx context.Context, userID, currentDeviceID string) ([]model.Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id::text, d.user_id::text, d.reported_name, d.custom_name,
			COALESCE(NULLIF(d.custom_name, ''), d.reported_name),
			d.platform, d.os_version, d.app_version, d.role, d.first_login_at,
			d.last_login_at, d.last_seen_at, d.revoked_at,
			EXISTS (
				SELECT 1 FROM auth_sessions s
				WHERE s.device_id = d.id AND s.revoked_at IS NULL
				  AND s.refresh_expires_at > now()
			), d.id = $2
		FROM devices d
		WHERE d.user_id = $1
		ORDER BY (d.id = $2) DESC, d.last_seen_at DESC NULLS LAST, d.last_login_at DESC`,
		userID, currentDeviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []model.Device
	for rows.Next() {
		var device model.Device
		if err := rows.Scan(
			&device.ID, &device.UserID, &device.ReportedName, &device.CustomName,
			&device.DisplayName, &device.Platform, &device.OSVersion,
			&device.AppVersion, &device.Role, &device.FirstLoginAt, &device.LastLoginAt,
			&device.LastSeenAt, &device.RevokedAt, &device.LoggedIn, &device.Current,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	var actorRole model.DeviceRole
	for _, device := range devices {
		if device.Current {
			actorRole = device.Role
			break
		}
	}
	for index := range devices {
		device := &devices[index]
		active := device.RevokedAt == nil
		device.CanRevoke = active && model.CanRevokeDevice(actorRole, device.Role, device.Current)
		device.CanChangeRole = active && model.CanChangeDeviceRole(actorRole, device.Role, device.Current)
	}
	return devices, rows.Err()
}

func (s *Store) RenameDevice(ctx context.Context, userID, deviceID, name string) (model.Device, error) {
	var device model.Device
	err := s.pool.QueryRow(ctx, `
		UPDATE devices SET custom_name = $3
		WHERE user_id = $1 AND id = $2 AND revoked_at IS NULL
		RETURNING id::text, user_id::text, reported_name, custom_name,
			COALESCE(NULLIF(custom_name, ''), reported_name), platform,
			os_version, app_version, role, first_login_at, last_login_at,
			last_seen_at, revoked_at`, userID, deviceID, name,
	).Scan(
		&device.ID, &device.UserID, &device.ReportedName, &device.CustomName,
		&device.DisplayName, &device.Platform, &device.OSVersion,
		&device.AppVersion, &device.Role, &device.FirstLoginAt, &device.LastLoginAt,
		&device.LastSeenAt, &device.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Device{}, ErrDeviceNotFound
	}
	return device, err
}

func (s *Store) RevokeDevice(ctx context.Context, userID, actorDeviceID, deviceID string) error {
	if actorDeviceID == deviceID {
		return ErrDevicePermission
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	actorRole, targetRole, err := lockedDeviceRoles(ctx, tx, userID, actorDeviceID, deviceID)
	if err != nil {
		return err
	}
	if !model.CanRevokeDevice(actorRole, targetRole, false) {
		return ErrDevicePermission
	}
	result, err := tx.Exec(ctx, `
		UPDATE devices SET revoked_at = now()
		WHERE user_id = $1 AND id = $2 AND revoked_at IS NULL`, userID, deviceID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = now()
		WHERE device_id = $1 AND revoked_at IS NULL`, deviceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM device_push_tokens WHERE device_id = $1`, deviceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetDeviceRole(
	ctx context.Context,
	userID, actorDeviceID, deviceID string,
	role model.DeviceRole,
) error {
	if !model.ValidAssignableDeviceRole(role) {
		return ErrInvalidDeviceRole
	}
	if actorDeviceID == deviceID {
		return ErrDevicePermission
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	actorRole, targetRole, err := lockedDeviceRoles(ctx, tx, userID, actorDeviceID, deviceID)
	if err != nil {
		return err
	}
	if !model.CanChangeDeviceRole(actorRole, targetRole, false) {
		return ErrDevicePermission
	}
	result, err := tx.Exec(ctx, `
		UPDATE devices SET role = $3
		WHERE user_id = $1 AND id = $2 AND revoked_at IS NULL`,
		userID, deviceID, role)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return tx.Commit(ctx)
}

func lockedDeviceRoles(
	ctx context.Context,
	tx pgx.Tx,
	userID, actorDeviceID, targetDeviceID string,
) (model.DeviceRole, model.DeviceRole, error) {
	var actorRole, targetRole model.DeviceRole
	err := tx.QueryRow(ctx, `
		SELECT actor.role, target.role
		FROM devices actor
		JOIN devices target ON target.user_id = actor.user_id
		WHERE actor.user_id = $1
		  AND actor.id = $2
		  AND actor.revoked_at IS NULL
		  AND target.id = $3
		  AND target.revoked_at IS NULL
		FOR UPDATE OF actor, target`, userID, actorDeviceID, targetDeviceID,
	).Scan(&actorRole, &targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrDeviceNotFound
	}
	return actorRole, targetRole, err
}

func (s *Store) TouchDevice(ctx context.Context, deviceID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET last_seen_at = $2 WHERE id = $1`, deviceID, at)
	return err
}
