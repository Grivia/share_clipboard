package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const moduleID = "fastcopy_kernelsu"

type Stores struct {
	DataDir     string
	ConfigPath  string
	RuntimePath string
	PendingPath string
	StatusPath  string
}

func NewStores() (*Stores, error) {
	dataDir := os.Getenv("FASTCOPY_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data/adb/fastcopy"
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure data directory: %w", err)
	}
	return &Stores{
		DataDir:     dataDir,
		ConfigPath:  filepath.Join(dataDir, "settings.json"),
		RuntimePath: filepath.Join(dataDir, "runtime.json"),
		PendingPath: filepath.Join(dataDir, "pending.json"),
		StatusPath:  filepath.Join(dataDir, "status.json"),
	}, nil
}

func (s *Stores) LoadUserConfig() (UserConfig, error) {
	data, err := os.ReadFile(s.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		config := UserConfig{
			ServerURL: "https://zhy.hair/fastcopy",
		}
		if err := s.SaveUserConfig(config); err != nil {
			return UserConfig{}, err
		}
		return config, nil
	}
	if err != nil {
		return UserConfig{}, fmt.Errorf("read module settings: %w", err)
	}
	config, migrated, err := decodeUserConfig(data)
	if err != nil {
		return UserConfig{}, err
	}
	if migrated {
		if err := s.SaveUserConfig(config); err != nil {
			return UserConfig{}, fmt.Errorf("migrate module settings: %w", err)
		}
	}
	return config, nil
}

func decodeUserConfig(data []byte) (UserConfig, bool, error) {
	var config UserConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return UserConfig{}, false, fmt.Errorf("parse module settings: %w", err)
	}
	var legacy struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(data, &legacy)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	migrated := fields["email"] != nil || fields["auth_action"] != nil || fields["shared_key"] != nil
	if config.Account == "" {
		config.Account = legacy.Email
	}
	config.ServerURL = strings.TrimRight(strings.TrimSpace(config.ServerURL), "/")
	config.Account = strings.TrimSpace(config.Account)
	return config, migrated, nil
}

func (s *Stores) SaveUserConfig(config UserConfig) error {
	config.ServerURL = strings.TrimRight(strings.TrimSpace(config.ServerURL), "/")
	config.Account = strings.TrimSpace(config.Account)
	return writeJSONAtomic(s.ConfigPath, config)
}

func (s *Stores) EncodedUserConfig() (string, error) {
	config, err := s.LoadUserConfig()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (s *Stores) SaveEncodedUserConfig(encoded string) error {
	if len(encoded) > 16*1024 {
		return fmt.Errorf("module settings are too large")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return fmt.Errorf("decode module settings: %w", err)
	}
	if len(data) > 8*1024 {
		return fmt.Errorf("module settings are too large")
	}
	config, _, err := decodeUserConfig(data)
	if err != nil {
		return err
	}
	return s.SaveUserConfig(config)
}

func (s *Stores) LoadRuntime() (RuntimeState, error) {
	var state RuntimeState
	err := readJSON(s.RuntimePath, &state)
	if errors.Is(err, os.ErrNotExist) {
		state.InstallationID, err = newUUID()
		if err != nil {
			return RuntimeState{}, err
		}
		return state, s.SaveRuntime(state)
	}
	if err != nil {
		return RuntimeState{}, err
	}
	if state.InstallationID == "" {
		state.InstallationID, err = newUUID()
		if err != nil {
			return RuntimeState{}, err
		}
		if err := s.SaveRuntime(state); err != nil {
			return RuntimeState{}, err
		}
	}
	return state, nil
}

func (s *Stores) SaveRuntime(state RuntimeState) error {
	return writeJSONAtomic(s.RuntimePath, state)
}

func (s *Stores) LoadPending() ([]ClipUpload, error) {
	var pending []ClipUpload
	err := readJSON(s.PendingPath, &pending)
	if errors.Is(err, os.ErrNotExist) {
		return []ClipUpload{}, nil
	}
	return pending, err
}

func (s *Stores) SavePending(pending []ClipUpload) error {
	return writeJSONAtomic(s.PendingPath, pending)
}

func (c UserConfig) Validate(hasSession bool) error {
	parsed, err := url.Parse(c.ServerURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("invalid server URL")
	}
	if parsed.Scheme == "http" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return fmt.Errorf("HTTPS is required for non-local servers")
	}
	accountLength := utf8.RuneCountInString(c.Account)
	if accountLength == 0 || accountLength > 128 || containsControlCharacter(c.Account) {
		return fmt.Errorf("account must contain 1 to 128 characters without control characters")
	}
	passwordLength := utf8.RuneCountInString(c.Password)
	if !hasSession && (passwordLength < 4 || passwordLength > 256 || len(c.Password) > 1024) {
		return fmt.Errorf("password is required to sign in")
	}
	return nil
}

func (c UserConfig) Fingerprint() string {
	hash := sha256.Sum256([]byte(c.ServerURL + "\x00" + c.Account))
	return hex.EncodeToString(hash[:])
}

func (s RuntimeState) HasDerivedKey() bool {
	key, err := base64.StdEncoding.DecodeString(s.SharedKey)
	return err == nil && len(key) == 32 && s.KeyVersion == keyDerivationVersion
}

func (s RuntimeState) HasSession() bool {
	return s.HasDerivedKey() && (s.AccessToken != "" || s.RefreshToken != "")
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
