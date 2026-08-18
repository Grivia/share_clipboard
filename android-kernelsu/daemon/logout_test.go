package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIClientLogout(t *testing.T) {
	baseURL, err := url.Parse("https://fastcopy.test/base")
	if err != nil {
		t.Fatal(err)
	}
	requestReceived := false
	client := &APIClient{
		baseURL: baseURL,
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestReceived = true
			if request.Method != http.MethodPost || request.URL.Path != "/base/v1/auth/logout" {
				t.Errorf("request = %s %s, want POST /base/v1/auth/logout", request.Method, request.URL.Path)
			}
			if authorization := request.Header.Get("Authorization"); authorization != "Bearer access-token" {
				t.Errorf("Authorization = %q", authorization)
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		})},
	}

	if err := client.Logout(context.Background(), "access-token"); err != nil {
		t.Fatal(err)
	}
	if !requestReceived {
		t.Fatal("remote logout request was not sent")
	}
}

func TestRevokeRemoteSessionRefreshesExpiredAccessToken(t *testing.T) {
	baseURL, err := url.Parse("https://fastcopy.test")
	if err != nil {
		t.Fatal(err)
	}
	requestCount := 0
	client := &APIClient{
		baseURL: baseURL,
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			status := http.StatusNoContent
			body := ""
			switch requestCount {
			case 1:
				if request.URL.Path != "/v1/auth/logout" || request.Header.Get("Authorization") != "Bearer expired-access" {
					t.Errorf("unexpected first request: %s %s", request.Method, request.URL.Path)
				}
				status = http.StatusUnauthorized
				body = `{"error":{"code":"SESSION_EXPIRED","message":"expired"}}`
			case 2:
				if request.URL.Path != "/v1/auth/refresh" {
					t.Errorf("second path = %s, want /v1/auth/refresh", request.URL.Path)
				}
				status = http.StatusOK
				body = `{"tokens":{"access_token":"renewed-access","refresh_token":"renewed-refresh"}}`
			case 3:
				if request.URL.Path != "/v1/auth/logout" || request.Header.Get("Authorization") != "Bearer renewed-access" {
					t.Errorf("unexpected final request: %s %s", request.Method, request.URL.Path)
				}
			default:
				t.Errorf("unexpected extra request %d", requestCount)
			}
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    request,
			}, nil
		})},
	}

	err = revokeRemoteSession(context.Background(), client, RuntimeState{
		AccessToken: "expired-access", RefreshToken: "valid-refresh",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 3 {
		t.Fatalf("request count = %d, want 3", requestCount)
	}
}

func TestClearLocalSessionPreservesInstallationAndCursor(t *testing.T) {
	directory := t.TempDir()
	stores := &Stores{
		DataDir:     directory,
		ConfigPath:  filepath.Join(directory, "settings.json"),
		RuntimePath: filepath.Join(directory, "runtime.json"),
		PendingPath: filepath.Join(directory, "pending.json"),
		StatusPath:  filepath.Join(directory, "status.json"),
	}
	user := UserConfig{
		Enabled: true, ServerURL: "https://fastcopy.test", Account: "alice", Password: "secret",
	}
	runtime := RuntimeState{
		InstallationID: "installation-1", AccountFingerprint: "fingerprint",
		AccessToken: "access-token", RefreshToken: "refresh-token",
		UserID: "user-1", DeviceID: "device-1", SharedKey: "secret-key",
		KeyVersion: keyDerivationVersion, LastSeq: 42,
	}
	if err := stores.SavePending([]ClipUpload{{ClientEventID: "event-1"}}); err != nil {
		t.Fatal(err)
	}

	if err := clearLocalSession(stores, user, runtime); err != nil {
		t.Fatal(err)
	}

	runtime, err := stores.LoadRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.InstallationID != "installation-1" || runtime.AccountFingerprint != "fingerprint" {
		t.Fatalf("installation identity was not preserved: %+v", runtime)
	}
	if runtime.AccessToken != "" || runtime.RefreshToken != "" || runtime.UserID != "" ||
		runtime.DeviceID != "" || runtime.SharedKey != "" || runtime.KeyVersion != 0 || runtime.LastSeq != 42 {
		t.Fatalf("authentication state was not cleared correctly: %+v", runtime)
	}
	pending, err := stores.LoadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending uploads = %d, want 0", len(pending))
	}
	config, err := stores.LoadUserConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Password != "" || config.Account != "alice" {
		t.Fatalf("unexpected config after logout: %+v", config)
	}
	var status DaemonStatus
	if err := readJSON(stores.StatusPath, &status); err != nil {
		t.Fatal(err)
	}
	if status.Authenticated || status.State != "unconfigured" {
		t.Fatalf("unexpected logout status: %+v", status)
	}
}
