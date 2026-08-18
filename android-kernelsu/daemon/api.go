package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("server returned HTTP %d", e.Status)
}

func isUnauthorized(err error) bool {
	apiError, ok := err.(*APIError)
	return ok && apiError.Status == http.StatusUnauthorized
}

type APIClient struct {
	baseURL *url.URL
	http    *http.Client
	wsHTTP  *http.Client
}

func NewAPIClient(rawBaseURL string) (*APIClient, error) {
	baseURL, err := url.Parse(strings.TrimRight(rawBaseURL, "/"))
	if err != nil || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid server URL")
	}
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	dialContext := dialer.DialContext
	if runtime.GOOS == "android" {
		dialContext = newAndroidResolver(dialer, os.Getenv("FASTCOPY_BRIDGE_JAR")).DialContext
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &APIClient{
		baseURL: baseURL,
		http:    &http.Client{Transport: transport, Timeout: 30 * time.Second},
		wsHTTP:  &http.Client{Transport: transport},
	}, nil
}

func (c *APIClient) Authenticate(ctx context.Context, request AuthRequest) (AuthResponse, error) {
	var response AuthResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/session", nil, "", request, &response)
	return response, err
}

func (c *APIClient) Refresh(ctx context.Context, refreshToken string) (RefreshResponse, error) {
	var response RefreshResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/refresh", nil, "", map[string]string{
		"refresh_token": refreshToken,
	}, &response)
	return response, err
}

func (c *APIClient) Upload(ctx context.Context, token string, upload ClipUpload) (ClipCreateResponse, error) {
	var response ClipCreateResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/clips", nil, token, upload, &response)
	return response, err
}

func (c *APIClient) Clips(ctx context.Context, token string, afterSeq int64) (ClipsResponse, error) {
	var response ClipsResponse
	query := url.Values{
		"after_seq": {fmt.Sprintf("%d", afterSeq)},
		"limit":     {"200"},
	}
	err := c.doJSON(ctx, http.MethodGet, "/v1/clips", query, token, nil, &response)
	return response, err
}

func (c *APIClient) Devices(ctx context.Context, token string) (DevicesResponse, error) {
	var response DevicesResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/devices", nil, token, nil, &response)
	return response, err
}

func (c *APIClient) Acknowledge(ctx context.Context, token string, seq int64) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/sync/ack", nil, token, map[string]int64{
		"seq": seq,
	}, nil)
}

func (c *APIClient) endpoint(path string, query url.Values) string {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = query.Encode()
	endpoint.Fragment = ""
	return endpoint.String()
}

func (c *APIClient) doJSON(
	ctx context.Context,
	method, path string,
	query url.Values,
	token string,
	requestBody any,
	responseBody any,
) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint(path, query), body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "FastCopyAndroid/"+daemonVersion)
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var envelope APIErrorEnvelope
		if json.Unmarshal(data, &envelope) == nil && envelope.Error.Message != "" {
			return &APIError{
				Status: response.StatusCode, Code: envelope.Error.Code, Message: envelope.Error.Message,
			}
		}
		return &APIError{Status: response.StatusCode}
	}
	if responseBody == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode server response: %w", err)
	}
	return nil
}
