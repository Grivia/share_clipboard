package main

import (
	"log"
	"sync"
	"time"
)

type StatusReporter struct {
	mu     sync.Mutex
	path   string
	status DaemonStatus
}

func NewStatusReporter(path string) *StatusReporter {
	reporter := &StatusReporter{path: path}
	reporter.status = DaemonStatus{
		State:   "starting",
		Message: "Starting",
		Version: daemonVersion,
	}
	reporter.flushLocked()
	return reporter
}

func (r *StatusReporter) Update(update func(*DaemonStatus)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	update(&r.status)
	r.flushLocked()
}

func (r *StatusReporter) Set(state, message string) {
	r.Update(func(status *DaemonStatus) {
		status.State = state
		status.Message = message
	})
}

func (r *StatusReporter) flushLocked() {
	r.status.UpdatedAt = time.Now().UTC()
	if err := writeJSONAtomic(r.path, r.status); err != nil {
		log.Printf("write status: %v", err)
	}
}
