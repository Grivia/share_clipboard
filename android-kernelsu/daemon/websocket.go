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
	pushedClips chan<- ClipEvent,
	reset <-chan struct{},
	connected *atomic.Bool,
) {
	defer connected.Store(false)
	backoff := time.Second
	for ctx.Err() == nil {
		token := state().Token
		if token == "" {
			waitForWebSocketRetry(ctx, reset, 2*time.Second)
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
			waitForWebSocketRetry(ctx, reset, backoff)
			backoff = minDuration(backoff*2, 30*time.Second)
			continue
		}

		connectionContext, cancelConnection := context.WithCancel(ctx)
		resetWatcherDone := make(chan struct{})
		go func() {
			defer close(resetWatcherDone)
			for {
				select {
				case <-connectionContext.Done():
					return
				case <-reset:
					if state().Token != token {
						cancelConnection()
						return
					}
				}
			}
		}()

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
		for connectionContext.Err() == nil {
			_, payload, err := connection.Read(connectionContext)
			if err != nil {
				break
			}
			var event struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(payload, &event); err != nil {
				log.Printf("decode WebSocket event: %v", err)
				continue
			}
			switch event.Type {
			case "hello":
				notify(wake)
			case "clip.created":
				var clip ClipEvent
				if len(event.Data) == 0 || json.Unmarshal(event.Data, &clip) != nil || clip.Seq <= 0 {
					notify(wake)
					continue
				}
				select {
				case pushedClips <- clip:
				default:
					notify(wake)
				}
			}
		}
		cancelConnection()
		<-resetWatcherDone
		_ = connection.Close(websocket.StatusNormalClosure, "reconnecting")
		wasConnected = connected.Swap(false)
		reporter.Update(func(status *DaemonStatus) { status.Connected = false })
		if wasConnected {
			notify(wake)
		}
	}
}

func waitForWebSocketRetry(ctx context.Context, reset <-chan struct{}, duration time.Duration) {
	select {
	case <-ctx.Done():
	case <-reset:
	case <-time.After(duration):
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
