package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr          string
	DatabaseURL         string
	PublicBaseURL       string
	RegistrationEnabled bool
	MaxUsers            int
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	ClipTTL             time.Duration
	IdempotencyTTL      time.Duration
	APNsEnabled         bool
	APNsKeyID           string
	APNsTeamID          string
	APNsBundleID        string
	APNsPrivateKeyPath  string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:          env("FASTCOPY_LISTEN_ADDR", ":8083"),
		DatabaseURL:         os.Getenv("FASTCOPY_DATABASE_URL"),
		PublicBaseURL:       env("FASTCOPY_PUBLIC_BASE_URL", "http://localhost:8083"),
		RegistrationEnabled: envBool("FASTCOPY_REGISTRATION_ENABLED", true),
		APNsEnabled:         envBool("FASTCOPY_APNS_ENABLED", false),
		APNsKeyID:           os.Getenv("FASTCOPY_APNS_KEY_ID"),
		APNsTeamID:          os.Getenv("FASTCOPY_APNS_TEAM_ID"),
		APNsBundleID:        env("FASTCOPY_APNS_BUNDLE_ID", "hair.zhy.fastcopy.ios"),
		APNsPrivateKeyPath:  os.Getenv("FASTCOPY_APNS_PRIVATE_KEY_PATH"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("FASTCOPY_DATABASE_URL is required")
	}
	var err error
	if cfg.MaxUsers, err = envInt("FASTCOPY_MAX_USERS", 0); err != nil {
		return Config{}, err
	}
	if cfg.MaxUsers < 0 {
		return Config{}, fmt.Errorf("FASTCOPY_MAX_USERS cannot be negative")
	}

	if cfg.AccessTokenTTL, err = envDuration("FASTCOPY_ACCESS_TOKEN_TTL", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.RefreshTokenTTL, err = envDuration("FASTCOPY_REFRESH_TOKEN_TTL", 90*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.ClipTTL, err = envDuration("FASTCOPY_CLIP_TTL", 7*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.IdempotencyTTL, err = envDuration("FASTCOPY_IDEMPOTENCY_TTL", 30*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.IdempotencyTTL < cfg.ClipTTL {
		return Config{}, fmt.Errorf("FASTCOPY_IDEMPOTENCY_TTL must be at least FASTCOPY_CLIP_TTL")
	}
	if cfg.APNsEnabled && (cfg.APNsKeyID == "" || cfg.APNsTeamID == "" ||
		cfg.APNsBundleID == "" || cfg.APNsPrivateKeyPath == "") {
		return Config{}, fmt.Errorf("APNs is enabled but its key ID, team ID, bundle ID, or private key path is missing")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}
