package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrPushNotSupported = errors.New("push notifications are not supported for this device")

type PushToken struct {
	DeviceID    string
	Token       string
	Environment string
}

func (s *Store) UpsertAPNsToken(
	ctx context.Context,
	userID, deviceID, token, environment string,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var eligible bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM devices
			WHERE id = $1 AND user_id = $2 AND platform = 'ios'
			  AND revoked_at IS NULL
		)`, deviceID, userID).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrPushNotSupported
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM device_push_tokens
		WHERE provider = 'apns' AND environment = $1 AND token = $2
		  AND device_id <> $3`, environment, token, deviceID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO device_push_tokens (
			device_id, user_id, provider, environment, token
		) VALUES ($1, $2, 'apns', $3, $4)
		ON CONFLICT (device_id, provider) DO UPDATE SET
			environment = EXCLUDED.environment,
			token = EXCLUDED.token,
			updated_at = now()`, deviceID, userID, environment, token)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteAPNsToken(ctx context.Context, userID, deviceID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM device_push_tokens
		WHERE user_id = $1 AND device_id = $2 AND provider = 'apns'`,
		userID, deviceID)
	return err
}

func (s *Store) APNsTokensForUser(
	ctx context.Context,
	userID, excludeDeviceID string,
) ([]PushToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.device_id::text, p.token, p.environment
		FROM device_push_tokens p
		JOIN devices d ON d.id = p.device_id AND d.user_id = p.user_id
		WHERE p.user_id = $1 AND p.provider = 'apns'
		  AND p.device_id <> $2 AND d.revoked_at IS NULL`,
		userID, excludeDeviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]PushToken, 0)
	for rows.Next() {
		var token PushToken
		if err := rows.Scan(&token.DeviceID, &token.Token, &token.Environment); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (s *Store) DeleteAPNsTokenValue(
	ctx context.Context,
	token, environment string,
) error {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM device_push_tokens
		WHERE provider = 'apns' AND token = $1 AND environment = $2`,
		token, environment)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
