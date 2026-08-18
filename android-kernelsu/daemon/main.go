package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
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
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
