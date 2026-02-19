//go:build !windows && !darwin

package client

import (
	"os/exec"
	"strings"
)

// CheckSSHService checks whether the SSH daemon is running on Linux.
func CheckSSHService() (bool, string) {
	out, err := exec.Command("systemctl", "is-active", "sshd").Output()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		return true, ""
	}

	out, err = exec.Command("systemctl", "is-active", "ssh").Output()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		return true, ""
	}

	out, err = exec.Command("pgrep", "-x", "sshd").Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return true, ""
	}

	return false, "SSH not running. Install/start with:\n  # Debian/Ubuntu: sudo apt install openssh-server && sudo systemctl start sshd\n  # RHEL/CentOS:  sudo yum install openssh-server && sudo systemctl start sshd"
}
