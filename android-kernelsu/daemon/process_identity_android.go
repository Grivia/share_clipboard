//go:build android

package main

import (
	"os/exec"
	"syscall"
)

func applyProcessIdentity(command *exec.Cmd, uid, gid uint32) error {
	command.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uid, Gid: gid},
	}
	return nil
}
