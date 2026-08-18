package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type authResult struct {
	User struct {
		ID      string `json:"id"`
		Account string `json:"account"`
	} `json:"user"`
	Device struct {
		ID string `json:"id"`
	} `json:"device"`
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	} `json:"tokens"`
}

type clipResult struct {
	Event struct {
		EventID string `json:"event_id"`
		Seq     int64  `json:"seq"`
	} `json:"event"`
	Status string `json:"status"`
}

func TestAPIWorkflowAndIdempotencyScopes(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("FASTCOPY_INTEGRATION_URL"), "/")
	if baseURL == "" {
		t.Skip("FASTCOPY_INTEGRATION_URL is not set")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	account := "Integration 用户 + " + strings.ReplaceAll(newUUID(t), "-", "")
	password := "短 密码 2026"
	installationA := newUUID(t)

	deviceA := authenticate(t, client, baseURL, "  "+account+"  ", password, installationA, "Integration A", http.StatusCreated)
	if deviceA.User.Account != account {
		t.Fatalf("server returned account %q, want %q", deviceA.User.Account, account)
	}
	requestJSON(t, client, http.MethodPost, baseURL+"/v1/auth/session", "", authBody(
		account, "wrong password", newUUID(t), "Wrong password",
	), http.StatusUnauthorized, nil)
	eventID := newUUID(t)
	upload := map[string]any{
		"client_event_id": eventID,
		"content_type":    "text/plain",
		"algorithm":       "AES-256-GCM",
		"nonce":           randomBase64(t, 12),
		"ciphertext":      randomBase64(t, 32),
	}

	first := postClip(t, client, baseURL, deviceA.Tokens.AccessToken, upload, http.StatusCreated)
	if first.Status != "created" || first.Event.EventID == "" || first.Event.Seq <= 0 {
		t.Fatalf("unexpected first upload response: %+v", first)
	}
	retry := postClip(t, client, baseURL, deviceA.Tokens.AccessToken, upload, http.StatusOK)
	if retry.Status != "already_created" || retry.Event.EventID != first.Event.EventID || retry.Event.Seq != first.Event.Seq {
		t.Fatalf("retry was not idempotent: first=%+v retry=%+v", first, retry)
	}

	conflict := cloneMap(upload)
	conflict["ciphertext"] = randomBase64(t, 32)
	requestJSON(t, client, http.MethodPost, baseURL+"/v1/clips", deviceA.Tokens.AccessToken, conflict, http.StatusConflict, nil)

	reloginA := authenticate(t, client, baseURL, account, password, installationA, "Integration A renamed", http.StatusOK)
	if reloginA.Device.ID != deviceA.Device.ID {
		t.Fatalf("same installation created a new device: old=%s new=%s", deviceA.Device.ID, reloginA.Device.ID)
	}
	sameInstallationRetry := postClip(t, client, baseURL, reloginA.Tokens.AccessToken, upload, http.StatusOK)
	if sameInstallationRetry.Event.EventID != first.Event.EventID {
		t.Fatal("same device did not retain its idempotency scope")
	}

	deviceB := authenticate(t, client, baseURL, account, password, newUUID(t), "Integration B", http.StatusOK)
	if deviceB.Device.ID == deviceA.Device.ID {
		t.Fatal("a new installation reused the old device ID")
	}

	websocketURL := strings.Replace(baseURL, "http://", "ws://", 1)
	websocketURL = strings.Replace(websocketURL, "https://", "wss://", 1) + "/v1/events/ws"
	header := http.Header{"Authorization": {"Bearer " + deviceB.Tokens.AccessToken}}
	connection, _, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("connect WebSocket: %v", err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "test complete")
	readEventType(t, connection, "hello")

	secondUpload := map[string]any{
		"client_event_id": newUUID(t),
		"content_type":    "text/plain",
		"algorithm":       "AES-256-GCM",
		"nonce":           randomBase64(t, 12),
		"ciphertext":      randomBase64(t, 48),
	}
	postClip(t, client, baseURL, reloginA.Tokens.AccessToken, secondUpload, http.StatusCreated)
	readEventType(t, connection, "clip.created")

	var clips struct {
		Clips []struct {
			EventID string `json:"event_id"`
			Seq     int64  `json:"seq"`
		} `json:"clips"`
	}
	requestJSON(t, client, http.MethodGet, baseURL+"/v1/clips?after_seq=0&limit=200", deviceB.Tokens.AccessToken, nil, http.StatusOK, &clips)
	if len(clips.Clips) < 2 {
		t.Fatalf("recovery endpoint returned %d clips, want at least 2", len(clips.Clips))
	}

	deviceC := authenticate(t, client, baseURL, account, password, newUUID(t), "Reinstalled device", http.StatusOK)
	newScope := postClip(t, client, baseURL, deviceC.Tokens.AccessToken, upload, http.StatusCreated)
	if newScope.Event.EventID == first.Event.EventID {
		t.Fatal("new installation incorrectly shared the old device idempotency scope")
	}
}

func authenticate(
	t *testing.T,
	client *http.Client,
	baseURL, account, password, installationID, name string,
	status int,
) authResult {
	t.Helper()
	body := authBody(account, password, installationID, name)
	var result authResult
	requestJSON(t, client, http.MethodPost, baseURL+"/v1/auth/session", "", body, status, &result)
	return result
}

func authBody(account, password, installationID, name string) map[string]any {
	return map[string]any{
		"account":  account,
		"password": password,
		"device": map[string]string{
			"installation_id": installationID,
			"reported_name":   name,
			"platform":        "linux",
			"os_version":      "integration",
			"app_version":     "test",
		},
	}
}

func postClip(
	t *testing.T,
	client *http.Client,
	baseURL, token string,
	body map[string]any,
	status int,
) clipResult {
	t.Helper()
	var result clipResult
	requestJSON(t, client, http.MethodPost, baseURL+"/v1/clips", token, body, status, &result)
	return result
}

func requestJSON(
	t *testing.T,
	client *http.Client,
	method, target, token string,
	body any,
	wantStatus int,
	result any,
) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, target, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s returned %d, want %d: %s", method, target, response.StatusCode, wantStatus, data)
	}
	if result != nil {
		if err := json.Unmarshal(data, result); err != nil {
			t.Fatalf("decode response: %v: %s", err, data)
		}
	}
}

func readEventType(t *testing.T, connection *websocket.Conn, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := connection.Read(ctx)
	if err != nil {
		t.Fatalf("read WebSocket event: %v", err)
	}
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != want {
		t.Fatalf("WebSocket event type is %q, want %q", event.Type, want)
	}
}

func newUUID(t *testing.T) string {
	t.Helper()
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func randomBase64(t *testing.T, size int) string {
	t.Helper()
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(value)
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
