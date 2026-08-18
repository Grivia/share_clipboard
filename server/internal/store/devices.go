package store

import (
	"context"
	"errors"
	"time"

	"fastcopy/server/internal/model"

	"github.com/jackc/pgx/v5"
)

var ErrDeviceNotFound = errors.New("device not found")

func (s *Store) Devices(ctx context.Context, userID, currentDeviceID string) ([]model.Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id::text, d.user_id::text, d.reported_name, d.custom_name,
			COALESCE(NULLIF(d.custom_name, ''), d.reported_name),
			d.platform, d.os_version, d.app_version, d.first_login_at,
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
			&device.AppVersion, &device.FirstLoginAt, &device.LastLoginAt,
			&device.LastSeenAt, &device.RevokedAt, &device.LoggedIn, &device.Current,
		); err != nil {
			return nil, err
		}
		devices = append(devices, device)
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
			os_version, app_version, first_login_at, last_login_at,
			last_seen_at, revoked_at`, userID, deviceID, name,
	).Scan(
		&device.ID, &device.UserID, &device.ReportedName, &device.CustomName,
		&device.DisplayName, &device.Platform, &device.OSVersion,
		&device.AppVersion, &device.FirstLoginAt, &device.LastLoginAt,
		&device.LastSeenAt, &device.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Device{}, ErrDeviceNotFound
	}
	return device, err
}

func (s *Store) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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

func (s *Store) TouchDevice(ctx context.Context, deviceID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET last_seen_at = $2 WHERE id = $1`, deviceID, at)
	return err
}
