package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"fastcopy/server/internal/ids"
	"fastcopy/server/internal/model"

	"github.com/jackc/pgx/v5"
)

var ErrEventIDReused = errors.New("client event ID reused with different payload")

func ClipRequestDigest(upload model.ClipUpload) []byte {
	hash := sha256.New()
	hash.Write([]byte(upload.ClientEventID))
	hash.Write([]byte{0})
	hash.Write([]byte(upload.ContentType))
	hash.Write([]byte{0})
	hash.Write([]byte(upload.Algorithm))
	hash.Write([]byte{0})
	hash.Write(upload.Nonce)
	hash.Write(upload.Ciphertext)
	return hash.Sum(nil)
}

func (s *Store) CreateClip(
	ctx context.Context,
	principal model.Principal,
	upload model.ClipUpload,
	clipTTL, idempotencyTTL time.Duration,
) (model.ClipCreateResult, error) {
	now := time.Now().UTC()
	eventID, err := ids.UUID()
	if err != nil {
		return model.ClipCreateResult{}, err
	}
	digest := ClipRequestDigest(upload)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.ClipCreateResult{}, err
	}
	defer tx.Rollback(ctx)

	var seq int64
	if err := tx.QueryRow(ctx, `SELECT nextval('clipboard_event_seq')`).Scan(&seq); err != nil {
		return model.ClipCreateResult{}, err
	}
	var insertedEventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO clip_idempotency (
			origin_device_id, client_event_id, user_id, event_id, seq,
			request_digest, processed_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (origin_device_id, client_event_id) DO NOTHING
		RETURNING event_id::text`,
		principal.DeviceID, upload.ClientEventID, principal.UserID, eventID, seq,
		digest, now, now.Add(idempotencyTTL),
	).Scan(&insertedEventID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.ClipCreateResult{}, fmt.Errorf("insert idempotency record: %w", err)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		var existingDigest []byte
		var existingEventID string
		var existingSeq int64
		err = tx.QueryRow(ctx, `
			SELECT event_id::text, seq, request_digest
			FROM clip_idempotency
			WHERE origin_device_id = $1 AND client_event_id = $2`,
			principal.DeviceID, upload.ClientEventID,
		).Scan(&existingEventID, &existingSeq, &existingDigest)
		if err != nil {
			return model.ClipCreateResult{}, err
		}
		if subtle.ConstantTimeCompare(existingDigest, digest) != 1 {
			return model.ClipCreateResult{}, ErrEventIDReused
		}
		event, err := s.clipByIdentity(ctx, tx, existingEventID, existingSeq, upload.ClientEventID, principal.DeviceID)
		if err != nil {
			return model.ClipCreateResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return model.ClipCreateResult{}, err
		}
		return model.ClipCreateResult{Event: event, Created: false, Status: "already_created"}, nil
	}

	var event model.ClipEvent
	var nonce, ciphertext []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO clipboard_events (
			event_id, user_id, origin_device_id, client_event_id, seq,
			content_type, algorithm, nonce, ciphertext, created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING event_id::text, client_event_id::text, seq,
			origin_device_id::text, content_type, algorithm, nonce,
			ciphertext, created_at, expires_at`,
		eventID, principal.UserID, principal.DeviceID, upload.ClientEventID, seq,
		upload.ContentType, upload.Algorithm, upload.Nonce, upload.Ciphertext,
		now, now.Add(clipTTL),
	).Scan(
		&event.EventID, &event.ClientEventID, &event.Seq, &event.OriginDeviceID,
		&event.ContentType, &event.Algorithm, &nonce, &ciphertext,
		&event.CreatedAt, &event.ExpiresAt,
	)
	if err != nil {
		return model.ClipCreateResult{}, fmt.Errorf("insert clip: %w", err)
	}
	event.Nonce = base64.StdEncoding.EncodeToString(nonce)
	event.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(custom_name, ''), reported_name)
		FROM devices WHERE id = $1`, principal.DeviceID).Scan(&event.OriginName); err != nil {
		return model.ClipCreateResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ClipCreateResult{}, err
	}
	return model.ClipCreateResult{Event: event, Created: true, Status: "created"}, nil
}

func (s *Store) clipByIdentity(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
	seq int64,
	clientEventID string,
	deviceID string,
) (model.ClipEvent, error) {
	var event model.ClipEvent
	var nonce, ciphertext []byte
	err := tx.QueryRow(ctx, `
		SELECT e.event_id::text, e.client_event_id::text, e.seq,
			e.origin_device_id::text,
			COALESCE(NULLIF(d.custom_name, ''), d.reported_name),
			e.content_type, e.algorithm, e.nonce, e.ciphertext,
			e.created_at, e.expires_at
		FROM clipboard_events e
		JOIN devices d ON d.id = e.origin_device_id
		WHERE e.event_id = $1`, eventID,
	).Scan(
		&event.EventID, &event.ClientEventID, &event.Seq, &event.OriginDeviceID,
		&event.OriginName, &event.ContentType, &event.Algorithm, &nonce,
		&ciphertext, &event.CreatedAt, &event.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ClipEvent{
			EventID: eventID, ClientEventID: clientEventID, Seq: seq,
			OriginDeviceID: deviceID,
		}, nil
	}
	if err != nil {
		return model.ClipEvent{}, err
	}
	event.Nonce = base64.StdEncoding.EncodeToString(nonce)
	event.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)
	return event, nil
}

func (s *Store) ClipsAfter(ctx context.Context, userID string, afterSeq int64, limit int) ([]model.ClipEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.event_id::text, e.client_event_id::text, e.seq,
			e.origin_device_id::text,
			COALESCE(NULLIF(d.custom_name, ''), d.reported_name),
			e.content_type, e.algorithm, e.nonce, e.ciphertext,
			e.created_at, e.expires_at
		FROM clipboard_events e
		JOIN devices d ON d.id = e.origin_device_id
		WHERE e.user_id = $1 AND e.seq > $2 AND e.expires_at > now()
		ORDER BY e.seq ASC
		LIMIT $3`, userID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	clips := make([]model.ClipEvent, 0)
	for rows.Next() {
		var event model.ClipEvent
		var nonce, ciphertext []byte
		if err := rows.Scan(
			&event.EventID, &event.ClientEventID, &event.Seq,
			&event.OriginDeviceID, &event.OriginName, &event.ContentType,
			&event.Algorithm, &nonce, &ciphertext, &event.CreatedAt,
			&event.ExpiresAt,
		); err != nil {
			return nil, err
		}
		event.Nonce = base64.StdEncoding.EncodeToString(nonce)
		event.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)
		clips = append(clips, event)
	}
	return clips, rows.Err()
}

func (s *Store) Ack(ctx context.Context, deviceID string, seq int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_cursors (device_id, last_acked_seq, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (device_id) DO UPDATE SET
			last_acked_seq = GREATEST(device_cursors.last_acked_seq, EXCLUDED.last_acked_seq),
			updated_at = now()`, deviceID, seq)
	return err
}
