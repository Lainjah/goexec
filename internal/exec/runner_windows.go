//go:build windows

package exec

import "syscall"

// defaultSysProcAttr returns default process attributes for Windows.
// Windows doesn't support Setpgid/Pgid, so we return nil.
func defaultSysProcAttr() *syscall.SysProcAttr {
	return nil
}

// extractSignal is a no-op on Windows as signals work differently.
func extractSignal(_ interface{}) (syscall.Signal, bool) {
	return 0, false
}
