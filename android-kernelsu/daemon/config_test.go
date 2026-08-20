package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingConfigCreatesPrivateFile(t *testing.T) {
	stores := configTestStores(t)
	config, err := stores.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.ServerURL != "https://zhy.hair/fastcopy" {
		t.Fatalf("default server URL = %q", config.ServerURL)
	}
	if !config.Enabled {
		t.Fatal("new configuration did not enable synchronization")
	}
	info, err := os.Stat(stores.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %o, want 600", info.Mode().Perm())
	}
}

func TestCredentialSubmissionEnablesDaemon(t *testing.T) {
	stores := configTestStores(t)
	configJSON := `{"enabled":false,"server_url":"https://example.test","account":"alice","password":"secret"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(configJSON))
	if err := stores.SaveEncodedUserConfig(encoded); err != nil {
		t.Fatal(err)
	}

	config, err := stores.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enabled {
		t.Fatal("credential submission left the daemon disabled")
	}
}

func TestEncodedConfigRoundTripAndLegacyCleanup(t *testing.T) {
	stores := configTestStores(t)
	legacy := `{"enabled":true,"server_url":"https://example.test/","email":" User+1 ","password":"pass word","auth_action":"login","shared_key":"obsolete"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(legacy))
	if err := stores.SaveEncodedUserConfig(encoded); err != nil {
		t.Fatal(err)
	}

	config, err := stores.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enabled || config.ServerURL != "https://example.test" || config.Account != "User+1" || config.Password != "pass word" {
		t.Fatalf("unexpected migrated config: %+v", config)
	}
	data, err := os.ReadFile(stores.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"email", "auth_action", "shared_key"} {
		if strings.Contains(string(data), removed) {
			t.Fatalf("legacy field %q remains in %s", removed, data)
		}
	}
	gotEncoded, err := stores.EncodedUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base64.StdEncoding.DecodeString(gotEncoded); err != nil {
		t.Fatalf("config-get returned invalid Base64: %v", err)
	}
}

func TestUserConfigAcceptsBroadAccount(t *testing.T) {
	config := UserConfig{
		ServerURL: "https://zhy.hair/fastcopy",
		Account:   "用户 Name+1",
		Password:  "短 密码",
	}
	if err := config.Validate(false); err != nil {
		t.Fatal(err)
	}
}

func TestUserConfigRejectsControlCharacterInAccount(t *testing.T) {
	config := UserConfig{
		ServerURL: "https://zhy.hair/fastcopy",
		Account:   "user\nname",
		Password:  "password",
	}
	if err := config.Validate(false); err == nil {
		t.Fatal("account containing a control character was accepted")
	}
}

func TestRuntimeSessionRequiresDerivedKey(t *testing.T) {
	state := RuntimeState{
		AccessToken: "token",
		SharedKey:   deriveSharedKey("alice", "correct horse battery staple"),
		KeyVersion:  keyDerivationVersion,
	}
	if !state.HasSession() {
		t.Fatal("valid token and derived key were not recognized")
	}
	state.KeyVersion++
	if state.HasSession() {
		t.Fatal("unknown key version was accepted")
	}
}

func TestAccountFingerprintIsCaseSensitive(t *testing.T) {
	upper := UserConfig{ServerURL: "https://example.test", Account: "Alice"}
	lower := UserConfig{ServerURL: "https://example.test", Account: "alice"}
	if upper.Fingerprint() == lower.Fingerprint() {
		t.Fatal("case-sensitive accounts produced the same fingerprint")
	}
}

func configTestStores(t *testing.T) *Stores {
	t.Helper()
	directory := t.TempDir()
	return &Stores{
		DataDir:    directory,
		ConfigPath: filepath.Join(directory, "settings.json"),
	}
}
