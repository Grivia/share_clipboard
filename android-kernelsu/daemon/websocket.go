package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type WebSocketState struct {
	Token string
}

func websocketLoop(
	ctx context.Context,
	client *APIClient,
	state func() WebSocketState,
	reporter *StatusReporter,
	wake chan<- struct{},
	connected *atomic.Bool,
) {
	defer connected.Store(false)
	backoff := time.Second
	for ctx.Err() == nil {
		token := state().Token
		if token == "" {
			sleepContext(ctx, 2*time.Second)
			continue
		}

		wsURL := *client.baseURL
		if wsURL.Scheme == "https" {
			wsURL.Scheme = "wss"
		} else {
			wsURL.Scheme = "ws"
		}
		wsURL.Path = strings.TrimRight(wsURL.Path, "/") + "/v1/events/ws"
		wsURL.RawQuery = ""
		wsURL.Fragment = ""
		headers := http.Header{"Authorization": {"Bearer " + token}, "User-Agent": {"FastCopyAndroid/" + daemonVersion}}
		connection, response, err := websocket.Dial(ctx, wsURL.String(), &websocket.DialOptions{
			HTTPClient: client.wsHTTP,
			HTTPHeader: headers,
		})
		if err != nil {
			if response != nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusUnauthorized {
					notify(wake)
				}
			}
			wasConnected := connected.Swap(false)
			reporter.Update(func(status *DaemonStatus) { status.Connected = false })
			if wasConnected {
				notify(wake)
			}
			sleepContext(ctx, backoff)
			backoff = minDuration(backoff*2, 30*time.Second)
			continue
		}

		backoff = time.Second
		wasConnected := connected.Swap(true)
		reporter.Update(func(status *DaemonStatus) {
			status.Connected = true
			status.State = "ready"
			status.Message = "Connected"
		})
		if !wasConnected {
			notify(wake)
		}
		for ctx.Err() == nil {
			_, payload, err := connection.Read(ctx)
			if err != nil {
				break
			}
			var event struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(payload, &event); err != nil {
				log.Printf("decode WebSocket event: %v", err)
				continue
			}
			switch event.Type {
			case "hello", "clip.created":
				notify(wake)
			}
		}
		_ = connection.Close(websocket.StatusNormalClosure, "reconnecting")
		wasConnected = connected.Swap(false)
		reporter.Update(func(status *DaemonStatus) { status.Connected = false })
		if wasConnected {
			notify(wake)
		}
	}
}

func notify(channel chan<- struct{}) {
	select {
	case channel <- struct{}{}:
	default:
	}
}

func sleepContext(ctx context.Context, duration time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(duration):
	}
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
