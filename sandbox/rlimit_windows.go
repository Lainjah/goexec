//go:build windows

package sandbox

// Rlimit resource types (Windows stubs - not supported).
const (
	RlimitCPU     = 0 // CPU time - not supported on Windows
	RlimitFSize   = 1 // Maximum file size - not supported on Windows
	RlimitData    = 2 // Maximum data segment size - not supported on Windows
	RlimitStack   = 3 // Maximum stack size - not supported on Windows
	RlimitCore    = 4 // Maximum core file size - not supported on Windows
	RlimitNOFile  = 5 // Maximum open files - not supported on Windows
	RlimitAS      = 6 // Maximum address space - not supported on Windows
	RlimitNProc   = 7 // Maximum processes - not supported on Windows
	RlimitMemlock = 8 // Maximum locked memory - not supported on Windows
)

// setRlimitImpl is a no-op on Windows.
func setRlimitImpl(_ int, _, _ uint64) error {
	return nil
}

// getRlimitImpl returns zero values on Windows.
func getRlimitImpl(_ int) (soft, hard uint64, err error) {
	return 0, 0, nil
}

// rlimitSupported returns false on Windows.
func rlimitSupported() bool {
	return false
}
