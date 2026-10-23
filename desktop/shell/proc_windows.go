package shell

import (
	"os/exec"
	"syscall"
)

// hide keeps the engine's console window from flashing up on Windows.
func hide(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
