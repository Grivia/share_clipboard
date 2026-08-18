package main

import (
	"testing"
	"time"
)

func TestSyncSchedulerReadyIntervals(t *testing.T) {
	scheduler := syncScheduler{networkFailures: 3}
	if delay := scheduler.NextDelay(syncReady, true); delay != 5*time.Minute {
		t.Fatalf("connected ready delay = %s, want 5m", delay)
	}
	if scheduler.networkFailures != 0 {
		t.Fatalf("network failures = %d, want reset", scheduler.networkFailures)
	}
	if delay := scheduler.NextDelay(syncReady, false); delay != 30*time.Second {
		t.Fatalf("disconnected ready delay = %s, want 30s", delay)
	}
}

func TestSyncSchedulerSpecialStates(t *testing.T) {
	scheduler := syncScheduler{}
	if delay := scheduler.NextDelay(syncWaitingUnlock, true); delay != 10*time.Second {
		t.Fatalf("waiting-unlock delay = %s, want 10s", delay)
	}
	if delay := scheduler.NextDelay(syncAuthRequired, false); delay != 5*time.Minute {
		t.Fatalf("auth-required delay = %s, want 5m", delay)
	}
}

func TestSyncSchedulerNetworkBackoff(t *testing.T) {
	scheduler := syncScheduler{}
	want := []time.Duration{
		2 * time.Second,
		5 * time.Second,
		15 * time.Second,
		30 * time.Second,
		60 * time.Second,
		60 * time.Second,
	}
	for index, expected := range want {
		if delay := scheduler.NextDelay(syncNetworkError, false); delay != expected {
			t.Fatalf("network delay %d = %s, want %s", index, delay, expected)
		}
	}
	if delay := scheduler.NextDelay(syncReady, true); delay != 5*time.Minute {
		t.Fatalf("ready delay after recovery = %s, want 5m", delay)
	}
	if delay := scheduler.NextDelay(syncNetworkError, false); delay != 2*time.Second {
		t.Fatalf("network delay after recovery = %s, want 2s", delay)
	}
}
