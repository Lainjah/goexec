//go:build unix

package exec

import "syscall"

// defaultSysProcAttr returns secure default process attributes for Unix systems.
func defaultSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Create a new process group so we can kill all children
		Setpgid: true,
		Pgid:    0,
	}
}

// extractSignal extracts the signal from the process state if the process was signaled.
func extractSignal(state interface{}) (syscall.Signal, bool) {
	if ws, ok := state.(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return ws.Signal(), true
		}
	}
	return 0, false
}
