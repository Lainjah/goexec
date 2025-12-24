//go:build unix

package sandbox

import "syscall"

// Rlimit resource types (Unix-specific).
const (
	RlimitCPU     = syscall.RLIMIT_CPU    // CPU time in seconds
	RlimitFSize   = syscall.RLIMIT_FSIZE  // Maximum file size
	RlimitData    = syscall.RLIMIT_DATA   // Maximum data segment size
	RlimitStack   = syscall.RLIMIT_STACK  // Maximum stack size
	RlimitCore    = syscall.RLIMIT_CORE   // Maximum core file size
	RlimitNOFile  = syscall.RLIMIT_NOFILE // Maximum open files
	RlimitAS      = syscall.RLIMIT_AS     // Maximum address space
	RlimitNProc   = 6                     // Maximum processes (RLIMIT_NPROC on Linux)
	RlimitMemlock = 8                     // Maximum locked memory (RLIMIT_MEMLOCK on Linux)
)

// setRlimitImpl sets a resource limit (Unix implementation).
func setRlimitImpl(resource int, soft, hard uint64) error {
	rlim := syscall.Rlimit{
		Cur: soft,
		Max: hard,
	}
	return syscall.Setrlimit(resource, &rlim)
}

// getRlimitImpl gets a resource limit (Unix implementation).
func getRlimitImpl(resource int) (soft, hard uint64, err error) {
	var rlim syscall.Rlimit
	err = syscall.Getrlimit(resource, &rlim)
	return rlim.Cur, rlim.Max, err
}

// rlimitSupported returns true if rlimit is supported on this platform.
func rlimitSupported() bool {
	return true
}
