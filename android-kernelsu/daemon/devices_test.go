package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestAPIClientDevices(t *testing.T) {
	baseURL, err := url.Parse("https://fastcopy.test/base")
	if err != nil {
		t.Fatal(err)
	}
	client := &APIClient{
		baseURL: baseURL,
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", request.Method)
			}
			if request.URL.Path != "/base/v1/devices" {
				t.Fatalf("path = %s, want /base/v1/devices", request.URL.Path)
			}
			if authorization := request.Header.Get("Authorization"); authorization != "Bearer access-token" {
				t.Fatalf("Authorization = %q", authorization)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"devices":[{"id":"device-1","display_name":"Pixel","platform":"android","os_version":"16","app_version":"0.3.5","role":"super_admin","logged_in":true,"online":true,"current":true,"can_revoke":false,"can_change_role":false}]}`)),
				Request:    request,
			}, nil
		})},
	}

	result, err := client.Devices(context.Background(), "access-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Devices) != 1 {
		t.Fatalf("device count = %d, want 1", len(result.Devices))
	}
	device := result.Devices[0]
	if device.DisplayName != "Pixel" || device.Role != "super_admin" || !device.LoggedIn || !device.Online || !device.Current {
		t.Fatalf("unexpected device: %+v", device)
	}
}

func TestAPIClientDeviceManagement(t *testing.T) {
	baseURL, err := url.Parse("https://fastcopy.test/base")
	if err != nil {
		t.Fatal(err)
	}
	requestCount := 0
	client := &APIClient{
		baseURL: baseURL,
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			if authorization := request.Header.Get("Authorization"); authorization != "Bearer access-token" {
				t.Errorf("Authorization = %q", authorization)
			}
			switch requestCount {
			case 1:
				if request.Method != http.MethodPost || request.URL.Path != "/base/v1/devices/device-2/revoke" {
					t.Errorf("request = %s %s, want POST revoke", request.Method, request.URL.Path)
				}
			case 2:
				if request.Method != http.MethodPatch || request.URL.Path != "/base/v1/devices/device-2/role" {
					t.Errorf("request = %s %s, want PATCH role", request.Method, request.URL.Path)
				}
				var body map[string]string
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["role"] != "admin" {
					t.Errorf("role = %q, want admin", body["role"])
				}
			default:
				t.Errorf("unexpected request %d", requestCount)
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		})},
	}

	if err := client.RevokeDevice(context.Background(), "access-token", "device-2"); err != nil {
		t.Fatal(err)
	}
	if err := client.UpdateDeviceRole(context.Background(), "access-token", "device-2", "admin"); err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
}

func TestDeviceManagementRefreshesExpiredAccessToken(t *testing.T) {
	baseURL, err := url.Parse("https://fastcopy.test")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	stores := &Stores{RuntimePath: filepath.Join(directory, "runtime.json")}
	runtime := RuntimeState{
		InstallationID: "installation-1",
		AccessToken:    "expired-access",
		RefreshToken:   "valid-refresh",
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
				if request.URL.Path != "/v1/devices/device-2/role" || request.Header.Get("Authorization") != "Bearer expired-access" {
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
				if request.URL.Path != "/v1/devices/device-2/role" || request.Header.Get("Authorization") != "Bearer renewed-access" {
					t.Errorf("unexpected final request: %s %s", request.Method, request.URL.Path)
				}
			default:
				t.Errorf("unexpected request %d", requestCount)
			}
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    request,
			}, nil
		})},
	}

	err = utilityAuthorized(context.Background(), stores, client, &runtime, func(ctx context.Context, api *APIClient, token string) error {
		return api.UpdateDeviceRole(ctx, token, "device-2", "admin")
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.AccessToken != "renewed-access" || runtime.RefreshToken != "renewed-refresh" {
		t.Fatalf("runtime tokens were not refreshed: %+v", runtime)
	}
	saved, err := stores.LoadRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != "renewed-access" || saved.RefreshToken != "renewed-refresh" {
		t.Fatalf("saved tokens were not refreshed: %+v", saved)
	}
}

func TestRefreshOnlineDevicesInvalidSessionRequiresLogin(t *testing.T) {
	baseURL, err := url.Parse("https://fastcopy.test")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	stores := &Stores{
		RuntimePath: filepath.Join(directory, "runtime.json"),
		StatusPath:  filepath.Join(directory, "status.json"),
	}
	runtime := RuntimeState{
		InstallationID: "installation-1",
		AccessToken:    "deleted-access",
		RefreshToken:   "deleted-refresh",
		UserID:         "deleted-user",
		DeviceID:       "deleted-device",
		SharedKey:      base64.StdEncoding.EncodeToString(make([]byte, 32)),
		KeyVersion:     keyDerivationVersion,
		LastSeq:        42,
	}
	requestCount := 0
	client := &APIClient{
		baseURL: baseURL,
		http: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			if requestCount == 1 && request.URL.Path != "/v1/devices" {
				t.Errorf("first path = %s, want /v1/devices", request.URL.Path)
			}
			if requestCount == 2 && request.URL.Path != "/v1/auth/refresh" {
				t.Errorf("second path = %s, want /v1/auth/refresh", request.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"code":"SESSION_EXPIRED","message":"session is invalid or expired"}}`,
				)),
				Request: request,
			}, nil
		})},
	}
	reporter := NewStatusReporter(stores.StatusPath)
	daemon := NewDaemon(
		UserConfig{ServerURL: baseURL.String(), Account: "alice"},
		runtime,
		nil,
		stores,
		client,
		nil,
		reporter,
	)

	err = daemon.refreshOnlineDevices(context.Background())
	if !isUnauthorized(err) {
		t.Fatalf("error = %v, want unauthorized", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
	saved, err := stores.LoadRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != "" || saved.RefreshToken != "" || saved.UserID != "" ||
		saved.DeviceID != "" || saved.SharedKey != "" || saved.KeyVersion != 0 || saved.LastSeq != 0 {
		t.Fatalf("expired authentication was not cleared: %+v", saved)
	}
	var status DaemonStatus
	if err := readJSON(stores.StatusPath, &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "auth_required" || status.Authenticated || status.Connected ||
		status.DeviceID != "" || status.DevicesLoaded || status.DevicesRefreshing {
		t.Fatalf("unexpected expired-session status: %+v", status)
	}
}

func TestFilterOnlineDevices(t *testing.T) {
	devices := []DeviceSummary{
		{ID: "current", DisplayName: "Android", Online: true, Current: true},
		{ID: "offline", DisplayName: "Mac", Online: false},
		{ID: "remote", DisplayName: "Linux", Online: true},
	}

	online := filterOnlineDevices(devices)
	if len(online) != 2 {
		t.Fatalf("online device count = %d, want 2", len(online))
	}
	if online[0].ID != "current" || online[1].ID != "remote" {
		t.Fatalf("unexpected online devices: %+v", online)
	}

	empty := filterOnlineDevices(nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty result = %#v, want non-nil empty slice", empty)
	}
}
