//go:build darwin

package client

import (
	"os/exec"
	"strings"
)

// CheckSSHService checks whether Remote Login (SSH) is enabled on macOS.
func CheckSSHService() (bool, string) {
	out, err := exec.Command("systemsetup", "-getremotelogin").Output()
	if err == nil && strings.Contains(string(out), "On") {
		return true, ""
	}
	return false, "SSH not enabled. Enable with:\n  sudo systemsetup -setremotelogin on"
}
