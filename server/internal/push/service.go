package push

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"fastcopy/server/internal/config"
	"fastcopy/server/internal/store"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

type clipNotice struct {
	UserID         string
	OriginDeviceID string
	Seq            int64
}

type Service struct {
	store      *store.Store
	bundleID   string
	production *apns2.Client
	sandbox    *apns2.Client
	queue      chan clipNotice
}

func New(cfg config.Config, dataStore *store.Store) (*Service, error) {
	service := &Service{store: dataStore, queue: make(chan clipNotice, 128)}
	if !cfg.APNsEnabled {
		return service, nil
	}
	authKey, err := token.AuthKeyFromFile(cfg.APNsPrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load APNs private key: %w", err)
	}
	authToken := &token.Token{
		AuthKey: authKey,
		KeyID:   cfg.APNsKeyID,
		TeamID:  cfg.APNsTeamID,
	}
	service.bundleID = cfg.APNsBundleID
	service.production = apns2.NewTokenClient(authToken).Production()
	service.sandbox = apns2.NewTokenClient(authToken).Development()
	return service, nil
}

func (s *Service) Enabled() bool {
	return s.production != nil && s.sandbox != nil
}

func (s *Service) ClipCreated(userID, originDeviceID string, seq int64) {
	if !s.Enabled() {
		return
	}
	select {
	case s.queue <- clipNotice{UserID: userID, OriginDeviceID: originDeviceID, Seq: seq}:
	default:
		slog.Warn("APNs queue is full", "user_id", userID)
	}
}

func (s *Service) Run(ctx context.Context) {
	if !s.Enabled() {
		slog.Info("APNs notifications disabled")
		<-ctx.Done()
		return
	}
	slog.Info("APNs notifications enabled", "bundle_id", s.bundleID)
	for {
		select {
		case <-ctx.Done():
			return
		case notice := <-s.queue:
			s.sendClipNotice(ctx, notice)
		}
	}
}

func (s *Service) sendClipNotice(ctx context.Context, notice clipNotice) {
	tokens, err := s.store.APNsTokensForUser(ctx, notice.UserID, notice.OriginDeviceID)
	if err != nil {
		slog.Error("load APNs tokens", "error", err)
		return
	}
	payload, err := json.Marshal(map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{
				"title": "粘贴板助手",
				"body":  "另一台设备复制了新文本",
			},
			"sound":             "default",
			"content-available": 1,
			"thread-id":         "clipboard",
		},
		"event": map[string]any{
			"type":             "clip.created",
			"seq":              notice.Seq,
			"origin_device_id": notice.OriginDeviceID,
		},
	})
	if err != nil {
		slog.Error("encode APNs payload", "error", err)
		return
	}

	for _, pushToken := range tokens {
		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		response, sendErr := s.client(pushToken.Environment).PushWithContext(sendCtx, &apns2.Notification{
			DeviceToken: pushToken.Token,
			Topic:       s.bundleID,
			CollapseID:  "clipboard-latest",
			Expiration:  time.Now().Add(time.Hour),
			Priority:    apns2.PriorityHigh,
			PushType:    apns2.PushTypeAlert,
			Payload:     payload,
		})
		cancel()
		if sendErr != nil {
			slog.Error("send APNs notification", "device_id", pushToken.DeviceID, "error", sendErr)
			continue
		}
		if response.StatusCode == 200 {
			continue
		}
		slog.Warn("APNs rejected token",
			"device_id", pushToken.DeviceID,
			"status", response.StatusCode,
			"reason", response.Reason,
		)
		if invalidTokenReason(response.Reason) {
			if err := s.store.DeleteAPNsTokenValue(context.Background(), pushToken.Token, pushToken.Environment); err != nil {
				slog.Error("delete invalid APNs token", "device_id", pushToken.DeviceID, "error", err)
			}
		}
	}
}

func (s *Service) client(environment string) *apns2.Client {
	if environment == "sandbox" {
		return s.sandbox
	}
	return s.production
}

func invalidTokenReason(reason string) bool {
	switch reason {
	case "BadDeviceToken", "DeviceTokenNotForTopic", "MissingDeviceToken", "Unregistered":
		return true
	default:
		return false
	}
}
