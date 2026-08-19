package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmicroseconds)
	stores, err := NewStores()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		if err := runUtility(stores, os.Args[1:]); err != nil {
			log.Print(err)
			os.Exit(2)
		}
		return
	}
	reporter := NewStatusReporter(stores.StatusPath)
	user, err := stores.LoadUserConfig()
	if err != nil {
		reporter.Set("error", err.Error())
		log.Fatal(err)
	}
	runtime, err := stores.LoadRuntime()
	if err != nil {
		reporter.Set("error", err.Error())
		log.Fatal(err)
	}
	pending, err := stores.LoadPending()
	if err != nil {
		reporter.Set("error", err.Error())
		log.Fatal(err)
	}

	if runtime.AccountFingerprint != user.Fingerprint() {
		runtime.AccessToken = ""
		runtime.RefreshToken = ""
		runtime.UserID = ""
		runtime.DeviceID = ""
		runtime.SharedKey = ""
		runtime.KeyVersion = 0
		runtime.LastSeq = 0
		runtime.AccountFingerprint = user.Fingerprint()
		pending = nil
		if err := stores.SaveRuntime(runtime); err != nil {
			log.Fatal(err)
		}
		if err := stores.SavePending(pending); err != nil {
			log.Fatal(err)
		}
	}
	if user.Password != "" {
		runtime.AccessToken = ""
		runtime.RefreshToken = ""
		runtime.SharedKey = ""
		runtime.KeyVersion = 0
		if err := stores.SaveRuntime(runtime); err != nil {
			log.Fatal(err)
		}
	} else if !runtime.HasDerivedKey() && (runtime.AccessToken != "" || runtime.RefreshToken != "") {
		runtime.AccessToken = ""
		runtime.RefreshToken = ""
		if err := stores.SaveRuntime(runtime); err != nil {
			log.Fatal(err)
		}
	}
	reporter.Update(func(status *DaemonStatus) {
		status.Authenticated = runtime.HasSession()
		status.DeviceID = runtime.DeviceID
	})
	if !user.Enabled {
		reporter.Set("disabled", "Disabled")
		return
	}
	if err := user.Validate(runtime.HasSession()); err != nil {
		reporter.Set("unconfigured", err.Error())
		return
	}
	api, err := NewAPIClient(user.ServerURL)
	if err != nil {
		reporter.Set("error", err.Error())
		log.Fatal(err)
	}
	moduleDir := os.Getenv("FASTCOPY_MODDIR")
	if moduleDir == "" {
		moduleDir = "/data/adb/modules/" + moduleID
	}
	bridgePath := os.Getenv("FASTCOPY_BRIDGE_JAR")
	if bridgePath == "" {
		bridgePath = filepath.Join(moduleDir, "bin", "fastcopy-bridge.jar")
	}
	bridge := NewBridge(bridgePath)
	daemon := NewDaemon(user, runtime, pending, stores, api, bridge, reporter)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	daemon.Run(ctx)
}

func runUtility(stores *Stores, args []string) error {
	switch args[0] {
	case "config-get":
		if len(args) != 1 {
			return fmt.Errorf("usage: fastcopyd config-get")
		}
		encoded, err := stores.EncodedUserConfig()
		if err != nil {
			return err
		}
		fmt.Println(encoded)
		return nil
	case "config-set":
		if len(args) != 2 {
			return fmt.Errorf("usage: fastcopyd config-set <base64-json>")
		}
		return stores.SaveEncodedUserConfig(args[1])
	case "logout":
		if len(args) != 1 {
			return fmt.Errorf("usage: fastcopyd logout")
		}
		return logoutSession(stores)
	case "device-revoke":
		if len(args) != 2 {
			return fmt.Errorf("usage: fastcopyd device-revoke <device-id>")
		}
		return runDeviceUtility(stores, func(ctx context.Context, api *APIClient, token string) error {
			return api.RevokeDevice(ctx, token, args[1])
		})
	case "device-role":
		if len(args) != 3 || (args[2] != "admin" && args[2] != "member") {
			return fmt.Errorf("usage: fastcopyd device-role <device-id> <admin|member>")
		}
		return runDeviceUtility(stores, func(ctx context.Context, api *APIClient, token string) error {
			return api.UpdateDeviceRole(ctx, token, args[1], args[2])
		})
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runDeviceUtility(
	stores *Stores,
	operation func(context.Context, *APIClient, string) error,
) error {
	user, err := stores.LoadUserConfig()
	if err != nil {
		return err
	}
	runtime, err := stores.LoadRuntime()
	if err != nil {
		return err
	}
	api, err := NewAPIClient(user.ServerURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return utilityAuthorized(ctx, stores, api, &runtime, operation)
}

func utilityAuthorized(
	ctx context.Context,
	stores *Stores,
	api *APIClient,
	runtime *RuntimeState,
	operation func(context.Context, *APIClient, string) error,
) error {
	if runtime.AccessToken != "" {
		err := operation(ctx, api, runtime.AccessToken)
		if err == nil || !isUnauthorized(err) {
			return err
		}
	}
	if runtime.RefreshToken == "" {
		return &APIError{Status: 401, Code: "SESSION_EXPIRED", Message: "session expired"}
	}
	response, err := api.Refresh(ctx, runtime.RefreshToken)
	if err != nil {
		if isUnauthorized(err) {
			runtime.ClearAuthentication()
			_ = stores.SaveRuntime(*runtime)
		}
		return err
	}
	runtime.AccessToken = response.Tokens.AccessToken
	runtime.RefreshToken = response.Tokens.RefreshToken
	if err := stores.SaveRuntime(*runtime); err != nil {
		return err
	}
	return operation(ctx, api, runtime.AccessToken)
}

func logoutSession(stores *Stores) error {
	user, err := stores.LoadUserConfig()
	if err != nil {
		return err
	}
	runtime, err := stores.LoadRuntime()
	if err != nil {
		return err
	}

	if runtime.AccessToken != "" || runtime.RefreshToken != "" {
		if api, apiErr := NewAPIClient(user.ServerURL); apiErr != nil {
			log.Printf("remote logout skipped: %v", apiErr)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			remoteErr := revokeRemoteSession(ctx, api, runtime)
			cancel()
			if remoteErr != nil {
				log.Printf("remote logout failed; clearing local session: %v", remoteErr)
			}
		}
	}

	return clearLocalSession(stores, user, runtime)
}

func clearLocalSession(stores *Stores, user UserConfig, runtime RuntimeState) error {
	user.Password = ""
	runtime.ClearAuthentication()
	if err := stores.SaveUserConfig(user); err != nil {
		return err
	}
	if err := stores.SaveRuntime(runtime); err != nil {
		return err
	}
	if err := stores.SavePending([]ClipUpload{}); err != nil {
		return err
	}
	return writeJSONAtomic(stores.StatusPath, DaemonStatus{
		State:         "unconfigured",
		Message:       "Signed out",
		OnlineDevices: []DeviceSummary{},
		Devices:       []DeviceSummary{},
		Version:       daemonVersion,
		UpdatedAt:     time.Now().UTC(),
	})
}

func revokeRemoteSession(ctx context.Context, api *APIClient, runtime RuntimeState) error {
	if runtime.AccessToken != "" {
		err := api.Logout(ctx, runtime.AccessToken)
		if err == nil {
			return nil
		}
		if !isUnauthorized(err) {
			return err
		}
	}
	if runtime.RefreshToken == "" {
		return nil
	}
	response, err := api.Refresh(ctx, runtime.RefreshToken)
	if isUnauthorized(err) {
		return nil
	}
	if err != nil {
		return err
	}
	err = api.Logout(ctx, response.Tokens.AccessToken)
	if isUnauthorized(err) {
		return nil
	}
	return err
}
