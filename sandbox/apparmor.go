package sandbox

import (
	"fmt"
	"runtime"

	"github.com/victoralfred/gowritter/safepath"
)

const (
	apparmorFSPath  = "/sys/kernel/security/apparmor"
	procAttrCurrent = "self/attr/current"
	procAttrExec    = "self/attr/exec"
)

// procFS provides safe access to /proc filesystem.
var procFS *safepath.SafePath

// sysFS provides safe access to /sys filesystem.
var sysFS *safepath.SafePath

func init() {
	if runtime.GOOS == "linux" {
		var err error
		procFS, err = safepath.New("/proc")
		if err != nil {
			procFS = nil
		}
		sysFS, err = safepath.New("/sys")
		if err != nil {
			sysFS = nil
		}
	}
}

// ApplyAppArmorProfile sets the AppArmor profile for the current process.
func ApplyAppArmorProfile(profile string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	if !isAppArmorSupported() {
		return nil
	}

	if procFS == nil {
		return fmt.Errorf("proc filesystem not available")
	}

	content := fmt.Sprintf("exec %s", profile)
	return procFS.WriteFile(procAttrExec, []byte(content), 0)
}

// ChangeAppArmorProfile changes to a profile immediately.
func ChangeAppArmorProfile(profile string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	if !isAppArmorSupported() {
		return nil
	}

	if procFS == nil {
		return fmt.Errorf("proc filesystem not available")
	}

	content := fmt.Sprintf("changeprofile %s", profile)
	return procFS.WriteFile(procAttrCurrent, []byte(content), 0)
}

// GetCurrentAppArmorProfile returns the current AppArmor profile.
func GetCurrentAppArmorProfile() (string, error) {
	if runtime.GOOS != "linux" {
		return "", nil
	}

	if !isAppArmorSupported() {
		return "", nil
	}

	if procFS == nil {
		return "", fmt.Errorf("proc filesystem not available")
	}

	data, err := procFS.ReadFile(procAttrCurrent)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// isAppArmorSupported checks if AppArmor is available.
func isAppArmorSupported() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	if sysFS == nil {
		return false
	}

	exists, _ := sysFS.Exists("kernel/security/apparmor")
	return exists
}

// AppArmorProfile represents an AppArmor profile.
type AppArmorProfile struct {
	// Name is the profile name.
	Name string

	// Content is the profile content.
	Content string
}

// CommonAppArmorProfiles provides common AppArmor profiles.
var CommonAppArmorProfiles = map[string]*AppArmorProfile{
	"goexec-minimal": {
		Name: "goexec-minimal",
		Content: `
#include <tunables/global>

profile goexec-minimal flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  # Deny network access
  deny network,

  # Deny raw socket access
  deny capability net_raw,

  # Deny ptrace
  deny ptrace,

  # Allow reading common libraries
  /lib/** r,
  /usr/lib/** r,

  # Deny access to sensitive files
  deny /etc/shadow r,
  deny /etc/sudoers r,
  deny /root/** rw,
}
`,
	},
	"goexec-restricted": {
		Name: "goexec-restricted",
		Content: `
#include <tunables/global>

profile goexec-restricted flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  #include <abstractions/nameservice>

  # Allow network access (for git, curl, etc)
  network inet stream,
  network inet6 stream,
  network inet dgram,
  network inet6 dgram,

  # Deny dangerous capabilities
  deny capability sys_admin,
  deny capability sys_ptrace,
  deny capability net_admin,

  # Allow reading common paths
  /lib/** r,
  /usr/lib/** r,
  /usr/bin/** rix,
  /bin/** rix,
  /tmp/** rw,

  # Deny access to sensitive files
  deny /etc/shadow r,
  deny /etc/sudoers r,
  deny /root/** rw,
  deny /home/*/.ssh/** rw,
}
`,
	},
	"goexec-git": {
		Name: "goexec-git",
		Content: `
#include <tunables/global>

profile goexec-git flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  #include <abstractions/nameservice>
  #include <abstractions/ssl_certs>

  # Network access for git operations
  network inet stream,
  network inet6 stream,

  # Git binary
  /usr/bin/git rix,
  /usr/libexec/git-core/** rix,

  # Allow reading common paths
  /lib/** r,
  /usr/lib/** r,
  /usr/share/git-core/** r,

  # Git repositories
  owner /home/**/projects/** rwk,
  owner /home/**/.gitconfig r,
  owner /home/**/.git-credentials r,

  # SSH for git
  /usr/bin/ssh rix,
  owner /home/**/.ssh/** r,

  # Deny sensitive files
  deny /etc/shadow r,
  deny /root/** rw,
}
`,
	},
}
