package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	connectedReconcileInterval    = 5 * time.Minute
	disconnectedReconcileInterval = 30 * time.Second
	waitingUnlockRetryInterval    = 10 * time.Second
	authRequiredRetryInterval     = 5 * time.Minute
)

var networkRetryIntervals = [...]time.Duration{
	2 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

type syncOutcome uint8

const (
	syncReady syncOutcome = iota
	syncWaitingUnlock
	syncNetworkError
	syncAuthRequired
)

type syncScheduler struct {
	networkFailures int
}

func (s *syncScheduler) NextDelay(outcome syncOutcome, connected bool) time.Duration {
	switch outcome {
	case syncReady:
		s.networkFailures = 0
		if connected {
			return connectedReconcileInterval
		}
		return disconnectedReconcileInterval
	case syncWaitingUnlock:
		s.networkFailures = 0
		return waitingUnlockRetryInterval
	case syncAuthRequired:
		s.networkFailures = 0
		return authRequiredRetryInterval
	default:
		index := s.networkFailures
		if index >= len(networkRetryIntervals) {
			index = len(networkRetryIntervals) - 1
		}
		if s.networkFailures < len(networkRetryIntervals) {
			s.networkFailures++
		}
		return networkRetryIntervals[index]
	}
}

type Daemon struct {
	user        UserConfig
	runtime     RuntimeState
	pending     []ClipUpload
	stores      *Stores
	api         *APIClient
	bridge      *Bridge
	reporter    *StatusReporter
	wake        chan struct{}
	wsState     atomic.Value
	wsConnected atomic.Bool
	lastDigest  [32]byte
	hasDigest   bool
}

func NewDaemon(
	user UserConfig,
	runtime RuntimeState,
	pending []ClipUpload,
	stores *Stores,
	api *APIClient,
	bridge *Bridge,
	reporter *StatusReporter,
) *Daemon {
	daemon := &Daemon{
		user: user, runtime: runtime, pending: pending, stores: stores,
		api: api, bridge: bridge, reporter: reporter, wake: make(chan struct{}, 1),
	}
	daemon.publishWebSocketState()
	return daemon
}

func (d *Daemon) Run(ctx context.Context) {
	go d.bridge.Run(ctx)
	go websocketLoop(ctx, d.api, d.webSocketState, d.reporter, d.wake, &d.wsConnected)
	deviceRefreshSignals := make(chan os.Signal, 1)
	signal.Notify(deviceRefreshSignals, syscall.SIGUSR1)
	defer signal.Stop(deviceRefreshSignals)

	d.reporter.Update(func(status *DaemonStatus) {
		status.State = "connecting"
		status.Message = "Connecting"
		status.Pending = len(d.pending)
		status.DeviceID = d.runtime.DeviceID
		status.Authenticated = d.runtime.HasSession()
	})
	scheduler := syncScheduler{}
	outcome := d.syncOnce(ctx)
	timer := time.NewTimer(scheduler.NextDelay(outcome, d.wsConnected.Load()))
	defer timer.Stop()
	runSync := func() {
		outcome := d.syncOnce(ctx)
		resetTimer(timer, scheduler.NextDelay(outcome, d.wsConnected.Load()))
	}
	for {
		select {
		case <-ctx.Done():
			d.reporter.Set("stopped", "Stopped")
			return
		case event := <-d.bridge.Events():
			if d.handleBridgeEvent(event) {
				runSync()
			}
		case <-d.wake:
			runSync()
		case <-timer.C:
			runSync()
		case <-deviceRefreshSignals:
			if err := d.refreshOnlineDevices(ctx); err != nil {
				log.Printf("refresh devices: %v", err)
				if isAuthRequiredError(err) {
					d.networkError(err)
				}
			}
		}
	}
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func (d *Daemon) handleBridgeEvent(event BridgeEvent) bool {
	switch event.Kind {
	case BridgeReady:
		d.reporter.Update(func(status *DaemonStatus) {
			if status.State == "starting" {
				status.State = "connecting"
			}
			status.Message = "Clipboard bridge ready"
		})
		return true
	case BridgeInitial:
		d.lastDigest = contentDigest(event.Text)
		d.hasDigest = true
		return false
	case BridgeClipboard:
		if err := d.queueLocalClipboard(event.Text); err != nil {
			log.Printf("queue clipboard: %v", err)
			d.reporter.Set("error", err.Error())
			return false
		}
		return true
	}
	return false
}

func (d *Daemon) syncOnce(ctx context.Context) syncOutcome {
	if err := d.flushPending(ctx); err != nil {
		d.networkError(err)
		return syncOutcomeForError(err)
	}
	if err := d.synchronize(ctx); err != nil {
		if errors.Is(err, errClipboardUnavailable) {
			log.Printf("clipboard update deferred: %v", err)
			d.reporter.Set("waiting_unlock", "Unlock Android to apply the clipboard")
			return syncWaitingUnlock
		}
		d.networkError(err)
		return syncOutcomeForError(err)
	}
	return syncReady
}

func syncOutcomeForError(err error) syncOutcome {
	if isAuthRequiredError(err) {
		return syncAuthRequired
	}
	return syncNetworkError
}

func isAuthRequiredError(err error) bool {
	return isUnauthorized(err) || strings.Contains(err.Error(), "password is required")
}

func (d *Daemon) networkError(err error) {
	log.Printf("sync: %v", err)
	state := "offline"
	if isAuthRequiredError(err) {
		state = "auth_required"
	}
	d.reporter.Update(func(status *DaemonStatus) {
		status.State = state
		status.Message = err.Error()
		if state == "auth_required" {
			status.Authenticated = false
			status.OnlineDevices = nil
			status.Devices = nil
			status.DevicesLoaded = false
			status.DevicesRefreshing = false
			status.DevicesError = ""
			status.DevicesUpdatedAt = nil
		}
	})
}

func (d *Daemon) withAuth(ctx context.Context, operation func(string) error) error {
	if err := d.ensureAuthenticated(ctx); err != nil {
		return err
	}
	err := operation(d.runtime.AccessToken)
	if !isUnauthorized(err) {
		return err
	}
	if refreshErr := d.refreshSession(ctx); refreshErr != nil {
		if isUnauthorized(refreshErr) {
			d.runtime.AccessToken = ""
			d.runtime.RefreshToken = ""
			_ = d.stores.SaveRuntime(d.runtime)
			d.publishWebSocketState()
		}
		return refreshErr
	}
	return operation(d.runtime.AccessToken)
}

func (d *Daemon) ensureAuthenticated(ctx context.Context) error {
	if d.runtime.HasDerivedKey() && d.runtime.AccessToken != "" {
		return nil
	}
	if d.runtime.HasDerivedKey() && d.runtime.RefreshToken != "" {
		if err := d.refreshSession(ctx); err == nil {
			return nil
		} else if !isUnauthorized(err) {
			return err
		}
	}
	if utf8.RuneCountInString(d.user.Password) < 4 {
		return fmt.Errorf("password is required; open KernelSU WebUI and sign in again")
	}
	return d.authenticate(ctx)
}

func (d *Daemon) authenticate(ctx context.Context) error {
	response, err := d.api.Authenticate(ctx, AuthRequest{
		Account:  d.user.Account,
		Password: d.user.Password,
		Device:   androidDevice(d.runtime.InstallationID),
	})
	if err != nil {
		return err
	}
	d.user.Account = response.User.Account
	d.runtime.SharedKey = deriveSharedKey(response.User.Account, d.user.Password)
	d.runtime.KeyVersion = keyDerivationVersion
	d.runtime.AccessToken = response.Tokens.AccessToken
	d.runtime.RefreshToken = response.Tokens.RefreshToken
	d.runtime.UserID = response.User.ID
	d.runtime.DeviceID = response.Device.ID
	d.runtime.AccountFingerprint = d.user.Fingerprint()
	if err := d.stores.SaveRuntime(d.runtime); err != nil {
		return err
	}
	d.user.Password = ""
	if err := d.stores.SaveUserConfig(d.user); err != nil {
		return err
	}
	d.publishWebSocketState()
	d.reporter.Update(func(status *DaemonStatus) {
		status.Authenticated = true
		status.DeviceID = d.runtime.DeviceID
		status.OnlineDevices = nil
		status.Devices = nil
		status.DevicesLoaded = false
		status.DevicesRefreshing = false
		status.DevicesError = ""
		status.DevicesUpdatedAt = nil
	})
	return nil
}

func (d *Daemon) refreshSession(ctx context.Context) error {
	if d.runtime.RefreshToken == "" {
		return &APIError{Status: 401, Code: "SESSION_EXPIRED", Message: "session expired"}
	}
	response, err := d.api.Refresh(ctx, d.runtime.RefreshToken)
	if err != nil {
		return err
	}
	d.runtime.AccessToken = response.Tokens.AccessToken
	d.runtime.RefreshToken = response.Tokens.RefreshToken
	if err := d.stores.SaveRuntime(d.runtime); err != nil {
		return err
	}
	d.publishWebSocketState()
	d.reporter.Update(func(status *DaemonStatus) { status.Authenticated = true })
	return nil
}

func (d *Daemon) refreshOnlineDevices(ctx context.Context) error {
	d.reporter.Update(func(status *DaemonStatus) {
		status.DevicesRefreshing = true
		status.DevicesError = ""
	})
	var response DevicesResponse
	err := d.withAuth(ctx, func(token string) error {
		var requestErr error
		response, requestErr = d.api.Devices(ctx, token)
		return requestErr
	})
	if err != nil {
		d.reporter.Update(func(status *DaemonStatus) {
			status.DevicesRefreshing = false
			status.DevicesError = err.Error()
		})
		return err
	}
	online := filterOnlineDevices(response.Devices)
	updatedAt := time.Now().UTC()
	d.reporter.Update(func(status *DaemonStatus) {
		status.Authenticated = true
		status.Devices = response.Devices
		status.OnlineDevices = online
		status.DevicesLoaded = true
		status.DevicesRefreshing = false
		status.DevicesError = ""
		status.DevicesUpdatedAt = &updatedAt
	})
	return nil
}

func filterOnlineDevices(devices []DeviceSummary) []DeviceSummary {
	online := make([]DeviceSummary, 0, len(devices))
	for _, device := range devices {
		if device.Online {
			online = append(online, device)
		}
	}
	return online
}

func (d *Daemon) publishWebSocketState() {
	d.wsState.Store(WebSocketState{Token: d.runtime.AccessToken})
}

func (d *Daemon) webSocketState() WebSocketState {
	return d.wsState.Load().(WebSocketState)
}

func androidDevice(installationID string) DeviceInput {
	manufacturer := property("ro.product.manufacturer")
	model := property("ro.product.model")
	name := strings.TrimSpace(strings.TrimSpace(manufacturer) + " " + strings.TrimSpace(model))
	if name == "" {
		name = "Android"
	}
	if len([]rune(name)) > 64 {
		name = string([]rune(name)[:64])
	}
	return DeviceInput{
		InstallationID: installationID,
		ReportedName:   name,
		Platform:       "android",
		OSVersion:      property("ro.build.version.release"),
		AppVersion:     daemonVersion,
	}
}

func property(name string) string {
	output, err := exec.Command("getprop", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
