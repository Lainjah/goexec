package sandbox

import (
	"fmt"
	"runtime"
)

// SeccompAction defines the action for a syscall.
type SeccompAction int

const (
	// SeccompAllow allows the syscall.
	SeccompAllow SeccompAction = iota
	// SeccompDeny denies the syscall with EPERM.
	SeccompDeny
	// SeccompLog logs the syscall but allows it.
	SeccompLog
	// SeccompTrap sends SIGSYS.
	SeccompTrap
	// SeccompKill kills the process.
	SeccompKill
)

// SeccompProfile defines a seccomp-bpf filter.
type SeccompProfile struct {
	// Name is the profile name.
	Name string

	// DefaultAction is the default action for unlisted syscalls.
	DefaultAction SeccompAction

	// Syscalls are the syscall rules.
	Syscalls []SyscallRule
}

// SyscallRule defines a rule for syscalls.
type SyscallRule struct {
	// Names are the syscall names.
	Names []string

	// Action is the action to take.
	Action SeccompAction

	// Args are argument filters.
	Args []SyscallArg
}

// SyscallArg defines an argument filter for a syscall.
type SyscallArg struct {
	// Index is the argument index (0-5).
	Index uint

	// Value is the value to compare.
	Value uint64

	// Op is the comparison operator.
	Op SyscallArgOp
}

// SyscallArgOp defines comparison operations for syscall arguments.
type SyscallArgOp int

const (
	// OpEqual checks if arg == value.
	OpEqual SyscallArgOp = iota
	// OpNotEqual checks if arg != value.
	OpNotEqual
	// OpLessThan checks if arg < value.
	OpLessThan
	// OpLessOrEqual checks if arg <= value.
	OpLessOrEqual
	// OpGreaterThan checks if arg > value.
	OpGreaterThan
	// OpGreaterOrEqual checks if arg >= value.
	OpGreaterOrEqual
	// OpMaskedEqual checks if (arg & value) == value.
	OpMaskedEqual
)

// SeccompFilter provides seccomp-bpf filtering.
type SeccompFilter struct {
	profile *SeccompProfile
}

// NewSeccompFilter creates a new seccomp filter from a profile.
func NewSeccompFilter(profile *SeccompProfile) (*SeccompFilter, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("seccomp is only supported on Linux")
	}

	return &SeccompFilter{profile: profile}, nil
}

// Apply applies the seccomp filter to the current process.
// Note: This is a stub. Full implementation requires libseccomp-golang.
func (f *SeccompFilter) Apply() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	// Full implementation would use libseccomp-golang:
	// filter, err := libseccomp.NewFilter(toScmpAction(f.profile.DefaultAction))
	// for _, rule := range f.profile.Syscalls { ... }
	// return filter.Load()

	return nil
}

// isSeccompSupported checks if seccomp is available.
func isSeccompSupported() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	// Check /proc/sys/kernel/seccomp/actions_avail
	// For now, assume supported on Linux
	return true
}

// CommonSeccompProfiles provides common seccomp profiles.
var CommonSeccompProfiles = map[string]*SeccompProfile{
	"allow-all": {
		Name:          "allow-all",
		DefaultAction: SeccompAllow,
	},
	"deny-dangerous": {
		Name:          "deny-dangerous",
		DefaultAction: SeccompAllow,
		Syscalls: []SyscallRule{
			{
				Names:  []string{"ptrace"},
				Action: SeccompDeny,
			},
			{
				Names:  []string{"process_vm_readv", "process_vm_writev"},
				Action: SeccompDeny,
			},
			{
				Names:  []string{"kexec_load", "kexec_file_load"},
				Action: SeccompDeny,
			},
			{
				Names:  []string{"init_module", "finit_module", "delete_module"},
				Action: SeccompDeny,
			},
			{
				Names:  []string{"reboot"},
				Action: SeccompDeny,
			},
			{
				Names:  []string{"swapon", "swapoff"},
				Action: SeccompDeny,
			},
			{
				Names:  []string{"mount", "umount", "umount2"},
				Action: SeccompDeny,
			},
		},
	},
	"minimal": {
		Name:          "minimal",
		DefaultAction: SeccompDeny,
		Syscalls: []SyscallRule{
			{
				Names: []string{
					"read", "write", "close", "fstat", "lstat", "stat",
					"poll", "lseek", "mmap", "mprotect", "munmap", "brk",
					"rt_sigaction", "rt_sigprocmask", "rt_sigreturn",
					"ioctl", "access", "pipe", "select", "sched_yield",
					"mremap", "msync", "mincore", "madvise",
					"dup", "dup2", "pause", "nanosleep", "getitimer", "alarm",
					"setitimer", "getpid", "socket", "connect", "accept",
					"sendto", "recvfrom", "sendmsg", "recvmsg", "shutdown",
					"bind", "listen", "getsockname", "getpeername", "socketpair",
					"setsockopt", "getsockopt", "clone", "fork", "vfork",
					"execve", "exit", "wait4", "kill", "uname",
					"fcntl", "flock", "fsync", "fdatasync", "truncate",
					"ftruncate", "getdents", "getcwd", "chdir", "fchdir",
					"rename", "mkdir", "rmdir", "creat", "link", "unlink",
					"symlink", "readlink", "chmod", "fchmod", "chown", "fchown",
					"lchown", "umask", "gettimeofday", "getrlimit", "getrusage",
					"sysinfo", "times", "getuid", "syslog", "getgid", "setuid",
					"setgid", "geteuid", "getegid", "setpgid", "getppid",
					"getpgrp", "setsid", "setreuid", "setregid", "getgroups",
					"setgroups", "setresuid", "getresuid", "setresgid",
					"getresgid", "getpgid", "setfsuid", "setfsgid", "getsid",
					"capget", "capset", "rt_sigpending", "rt_sigtimedwait",
					"rt_sigqueueinfo", "rt_sigsuspend", "sigaltstack",
					"utime", "mknod", "statfs", "fstatfs", "sysfs",
					"getpriority", "setpriority", "sched_setparam",
					"sched_getparam", "sched_setscheduler", "sched_getscheduler",
					"sched_get_priority_max", "sched_get_priority_min",
					"sched_rr_get_interval", "mlock", "munlock", "mlockall",
					"munlockall", "vhangup", "pivot_root", "prctl",
					"arch_prctl", "adjtimex", "setrlimit", "chroot", "sync",
					"acct", "settimeofday", "sethostname", "setdomainname",
					"ioperm", "iopl", "create_module", "get_kernel_syms",
					"query_module", "quotactl", "nfsservctl", "getpmsg",
					"putpmsg", "afs_syscall", "tuxcall", "security",
					"gettid", "readahead", "setxattr", "lsetxattr", "fsetxattr",
					"getxattr", "lgetxattr", "fgetxattr", "listxattr",
					"llistxattr", "flistxattr", "removexattr", "lremovexattr",
					"fremovexattr", "tkill", "time", "futex", "sched_setaffinity",
					"sched_getaffinity", "set_thread_area", "io_setup",
					"io_destroy", "io_getevents", "io_submit", "io_cancel",
					"get_thread_area", "lookup_dcookie", "epoll_create",
					"epoll_ctl_old", "epoll_wait_old", "remap_file_pages",
					"getdents64", "set_tid_address", "restart_syscall",
					"semtimedop", "fadvise64", "timer_create", "timer_settime",
					"timer_gettime", "timer_getoverrun", "timer_delete",
					"clock_settime", "clock_gettime", "clock_getres",
					"clock_nanosleep", "exit_group", "epoll_wait", "epoll_ctl",
					"tgkill", "utimes", "vserver", "mbind", "set_mempolicy",
					"get_mempolicy", "mq_open", "mq_unlink", "mq_timedsend",
					"mq_timedreceive", "mq_notify", "mq_getsetattr",
					"kexec_load", "waitid", "add_key", "request_key", "keyctl",
					"ioprio_set", "ioprio_get", "inotify_init", "inotify_add_watch",
					"inotify_rm_watch", "migrate_pages", "openat", "mkdirat",
					"mknodat", "fchownat", "futimesat", "newfstatat", "unlinkat",
					"renameat", "linkat", "symlinkat", "readlinkat", "fchmodat",
					"faccessat", "pselect6", "ppoll", "unshare", "set_robust_list",
					"get_robust_list", "splice", "tee", "sync_file_range",
					"vmsplice", "move_pages", "utimensat", "epoll_pwait",
					"signalfd", "timerfd_create", "eventfd", "fallocate",
					"timerfd_settime", "timerfd_gettime", "accept4", "signalfd4",
					"eventfd2", "epoll_create1", "dup3", "pipe2", "inotify_init1",
					"preadv", "pwritev", "rt_tgsigqueueinfo", "perf_event_open",
					"recvmmsg", "fanotify_init", "fanotify_mark", "prlimit64",
					"name_to_handle_at", "open_by_handle_at", "clock_adjtime",
					"syncfs", "sendmmsg", "setns", "getcpu", "process_vm_readv",
					"process_vm_writev", "kcmp", "finit_module", "sched_setattr",
					"sched_getattr", "renameat2", "seccomp", "getrandom",
					"memfd_create", "bpf", "execveat", "userfaultfd", "membarrier",
					"mlock2", "copy_file_range", "preadv2", "pwritev2",
					"pkey_mprotect", "pkey_alloc", "pkey_free", "statx",
					"io_pgetevents", "rseq",
				},
				Action: SeccompAllow,
			},
		},
	},
}
