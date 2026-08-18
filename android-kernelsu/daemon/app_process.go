package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	androidSystemUID = 1000
	androidShellUID  = 2000
)

type appProcessLaunchMode int

const (
	launchWithCredential appProcessLaunchMode = iota
	launchWithSuUID
	launchWithSuName
	launchWithCurrentIdentity
)

type androidIdentity struct {
	name string
	uid  uint32
	gid  uint32
}

type appProcessLauncher struct {
	name     string
	mode     appProcessLaunchMode
	identity androidIdentity
}

var (
	shellIdentity  = androidIdentity{name: "shell", uid: androidShellUID, gid: androidShellUID}
	systemIdentity = androidIdentity{name: "system", uid: androidSystemUID, gid: androidSystemUID}
)

func clipboardAppProcessLaunchers() []appProcessLauncher {
	return []appProcessLauncher{
		{name: "shell-credential", mode: launchWithCredential, identity: shellIdentity},
		{name: "shell-su-uid", mode: launchWithSuUID, identity: shellIdentity},
		{name: "shell-su-name", mode: launchWithSuName, identity: shellIdentity},
		{name: "system-credential", mode: launchWithCredential, identity: systemIdentity},
		{name: "system-su-uid", mode: launchWithSuUID, identity: systemIdentity},
	}
}

func dnsAppProcessLaunchers() []appProcessLauncher {
	return []appProcessLauncher{
		{name: "shell-su-uid", mode: launchWithSuUID, identity: shellIdentity},
		{name: "shell-credential", mode: launchWithCredential, identity: shellIdentity},
		{name: "shell-su-name", mode: launchWithSuName, identity: shellIdentity},
		{name: "current-identity", mode: launchWithCurrentIdentity},
	}
}

func (launcher appProcessLauncher) command(
	ctx context.Context,
	jarPath string,
	mainClass string,
	args ...string,
) (*exec.Cmd, error) {
	appProcess := findAppProcess()
	processArgs := append([]string{"/", mainClass}, args...)

	switch launcher.mode {
	case launchWithCredential:
		command := exec.CommandContext(ctx, appProcess, processArgs...)
		command.Env = append(os.Environ(), "CLASSPATH="+jarPath)
		if err := applyProcessIdentity(command, launcher.identity.uid, launcher.identity.gid); err != nil {
			return nil, err
		}
		return command, nil
	case launchWithCurrentIdentity:
		command := exec.CommandContext(ctx, appProcess, processArgs...)
		command.Env = append(os.Environ(), "CLASSPATH="+jarPath)
		return command, nil
	case launchWithSuUID, launchWithSuName:
		suPath, err := findSu()
		if err != nil {
			return nil, err
		}
		user := strconv.FormatUint(uint64(launcher.identity.uid), 10)
		if launcher.mode == launchWithSuName {
			user = launcher.identity.name
		}
		launch := []string{
			"CLASSPATH=" + shellQuote(jarPath),
			"exec",
			shellQuote(appProcess),
		}
		for _, argument := range processArgs {
			launch = append(launch, shellQuote(argument))
		}
		return exec.CommandContext(ctx, suPath, user, "-c", strings.Join(launch, " ")), nil
	default:
		return nil, fmt.Errorf("unsupported app_process launcher %q", launcher.name)
	}
}

func findAppProcess() string {
	for _, path := range []string{"/system/bin/app_process", "/system/bin/app_process64"} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "/system/bin/app_process"
}

func findSu() (string, error) {
	if path, err := exec.LookPath("su"); err == nil {
		return path, nil
	}
	for _, path := range []string{"/system/bin/su", "/system/xbin/su", "/sbin/su"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("su executable was not found")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
