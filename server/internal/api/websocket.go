package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"fastcopy/server/internal/hub"
	"fastcopy/server/internal/ids"

	"github.com/coder/websocket"
)

func (a *API) eventsWebSocket(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	connectionID, err := ids.UUID()
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "could not create connection")
		return
	}
	client := &hub.Client{
		ID: connectionID, UserID: principal.UserID, DeviceID: principal.DeviceID,
		Conn: conn, Send: make(chan []byte, 16), Done: make(chan struct{}),
	}
	first := a.hub.Add(client)
	if first {
		now := time.Now().UTC()
		_ = a.store.TouchDevice(r.Context(), principal.DeviceID, now)
		a.hub.Publish(principal.UserID, principal.DeviceID, hub.Event{
			Type: "device.presence_changed",
			Data: map[string]any{"device_id": principal.DeviceID, "online": true, "last_seen_at": now},
		})
	}

	hello := hub.Event{Type: "hello", Data: map[string]any{
		"connection_id": connectionID,
		"server_time":   time.Now().UTC(),
		"device_id":     principal.DeviceID,
	}}
	if payload, err := jsonBytes(hello); err == nil {
		client.Send <- payload
	}

	ctx, cancel := context.WithDeadline(r.Context(), principal.ExpiresAt)
	defer cancel()
	var lastTouch atomic.Int64
	lastTouch.Store(time.Now().Unix())
	touch := func() {
		now := time.Now().UTC()
		previous := lastTouch.Load()
		if now.Unix()-previous < 120 || !lastTouch.CompareAndSwap(previous, now.Unix()) {
			return
		}
		go func() { _ = a.store.TouchDevice(context.Background(), principal.DeviceID, now) }()
	}
	writerDone := make(chan struct{})
	go func() {
		_ = hub.RunWriter(ctx, client, touch)
		close(writerDone)
		cancel()
	}()

	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			break
		}
		touch()
	}
	client.Close(websocket.StatusNormalClosure, "connection closed")
	<-writerDone
	last := a.hub.Remove(client)
	now := time.Now().UTC()
	_ = a.store.TouchDevice(context.Background(), principal.DeviceID, now)
	if last {
		a.hub.Publish(principal.UserID, principal.DeviceID, hub.Event{
			Type: "device.presence_changed",
			Data: map[string]any{"device_id": principal.DeviceID, "online": false, "last_seen_at": now},
		})
	}
}

func jsonBytes(value any) ([]byte, error) {
	return json.Marshal(value)
}
