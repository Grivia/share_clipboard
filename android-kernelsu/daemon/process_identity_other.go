//go:build !android

package main

import (
	"fmt"
	"os/exec"
)

func applyProcessIdentity(_ *exec.Cmd, uid, gid uint32) error {
	return fmt.Errorf("changing process identity to %d:%d requires Android", uid, gid)
}
