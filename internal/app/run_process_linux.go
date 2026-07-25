//go:build linux

package app

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureRunProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func runProcessGroupID(pid int) int {
	return pid
}

func runProcessTerminationSignal() os.Signal {
	return syscall.SIGTERM
}

func runProcessTerminationSignalName() string {
	return "SIGTERM"
}

func forwardRunProcessSignal(processGroupID int, sig os.Signal) {
	if processGroupID <= 0 {
		return
	}
	if unixSignal, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(-processGroupID, unixSignal)
	}
}

func getRunProcessSessionID(pid int) (int, error) {
	sid, _, errno := syscall.Syscall(syscall.SYS_GETSID, uintptr(pid), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(sid), nil
}

func waitForRunProcessExit(waitCh <-chan error, processGroupID int, grace time.Duration) error {
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case err := <-waitCh:
		return err
	case <-timer.C:
		if processGroupID > 0 {
			_ = syscall.Kill(-processGroupID, syscall.SIGKILL)
		}
		return <-waitCh
	}
}
