package api

import (
	"sync"
	"time"
)

type attempt struct {
	count   int
	resetAt time.Time
}

type limiter struct {
	mu       sync.Mutex
	attempts map[string]attempt
	limit    int
	window   time.Duration
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{attempts: make(map[string]attempt), limit: limit, window: window}
}

func (l *limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.attempts[key]
	if entry.resetAt.Before(now) {
		entry = attempt{resetAt: now.Add(l.window)}
	}
	entry.count++
	l.attempts[key] = entry
	return entry.count <= l.limit
}

func (l *limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}
