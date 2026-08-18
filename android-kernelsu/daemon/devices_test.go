package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
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
				Body:       io.NopCloser(strings.NewReader(`{"devices":[{"id":"device-1","display_name":"Pixel","platform":"android","os_version":"16","app_version":"0.3.3","online":true,"current":true}]}`)),
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
	if device.DisplayName != "Pixel" || !device.Online || !device.Current {
		t.Fatalf("unexpected device: %+v", device)
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
