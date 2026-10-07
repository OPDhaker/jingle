//go:build !windows

package player

import (
	"os/exec"
	"syscall"
)

// detach starts the player in a new session, so it outlives the hook and
// gets no signals from the terminal git ran in.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
