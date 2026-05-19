//go:build windows

package tui

import "os/exec"

func pauseProcess(cmd *exec.Cmd) error {
	// Signal-based pausing is not supported natively on Windows
	return nil
}

func resumeProcess(cmd *exec.Cmd) error {
	return nil
}
