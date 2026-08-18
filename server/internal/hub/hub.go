package hub

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Client struct {
	ID       string
	UserID   string
	DeviceID string
	Conn     *websocket.Conn
	Send     chan []byte
	Done     chan struct{}
	once     sync.Once
}

func (c *Client) Close(status websocket.StatusCode, reason string) {
	c.once.Do(func() {
		close(c.Done)
		_ = c.Conn.Close(status, reason)
	})
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
	users   map[string]map[string]map[string]*Client
}

func New() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
		users:   make(map[string]map[string]map[string]*Client),
	}
}

// Add returns true when this is the device's first live connection.
func (h *Hub) Add(client *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.users[client.UserID] == nil {
		h.users[client.UserID] = make(map[string]map[string]*Client)
	}
	if h.users[client.UserID][client.DeviceID] == nil {
		h.users[client.UserID][client.DeviceID] = make(map[string]*Client)
	}
	first := len(h.users[client.UserID][client.DeviceID]) == 0
	h.clients[client.ID] = client
	h.users[client.UserID][client.DeviceID][client.ID] = client
	return first
}

// Remove returns true when the device no longer has any live connections.
func (h *Hub) Remove(client *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, client.ID)
	devices := h.users[client.UserID]
	if devices == nil {
		return true
	}
	connections := devices[client.DeviceID]
	delete(connections, client.ID)
	last := len(connections) == 0
	if last {
		delete(devices, client.DeviceID)
	}
	if len(devices) == 0 {
		delete(h.users, client.UserID)
	}
	return last
}

func (h *Hub) IsOnline(userID, deviceID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID][deviceID]) > 0
}

func (h *Hub) Publish(userID, excludeDeviceID string, event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	clients := make([]*Client, 0)
	for deviceID, connections := range h.users[userID] {
		if deviceID == excludeDeviceID {
			continue
		}
		for _, client := range connections {
			clients = append(clients, client)
		}
	}
	h.mu.RUnlock()

	for _, client := range clients {
		select {
		case client.Send <- payload:
		default:
			client.Close(websocket.StatusPolicyViolation, "slow client")
		}
	}
}

func (h *Hub) CloseDevice(userID, deviceID string) {
	h.mu.RLock()
	connections := h.users[userID][deviceID]
	clients := make([]*Client, 0, len(connections))
	for _, client := range connections {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	for _, client := range clients {
		client.Close(websocket.StatusPolicyViolation, "device revoked")
	}
}

func RunWriter(ctx context.Context, client *Client, touched func()) error {
	heartbeat := time.NewTicker(45 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-client.Done:
			return nil
		case payload := <-client.Send:
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := client.Conn.Write(writeCtx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				return err
			}
		case <-heartbeat.C:
			pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := client.Conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return err
			}
			touched()
		}
	}
}
