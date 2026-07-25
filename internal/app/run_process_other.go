//go:build !linux

package app

import (
	"os"
	"os/exec"
	"time"
)

func configureRunProcessGroup(cmd *exec.Cmd) {}

func runProcessGroupID(pid int) int {
	return pid
}

func runProcessTerminationSignal() os.Signal {
	return os.Interrupt
}

func runProcessTerminationSignalName() string {
	return "interrupt"
}

func forwardRunProcessSignal(processGroupID int, sig os.Signal) {}

func getRunProcessSessionID(pid int) (int, error) {
	return 0, os.ErrInvalid
}

func waitForRunProcessExit(waitCh <-chan error, processGroupID int, grace time.Duration) error {
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case err := <-waitCh:
		return err
	case <-timer.C:
		return <-waitCh
	}
}
