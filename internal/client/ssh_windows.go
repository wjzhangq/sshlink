//go:build windows

package client

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/wjzhangq/sshlink/internal/common"
)

// CheckSSHService checks whether the SSH service is running on Windows.
func CheckSSHService() (bool, string) {
	out, err := exec.Command("powershell", "-Command",
		"(Get-Service -Name sshd -ErrorAction SilentlyContinue).Status").Output()
	if err == nil && strings.TrimSpace(string(out)) == "Running" {
		return true, ""
	}

	_, err = exec.LookPath("sshd")
	if err == nil {
		common.Info("sshd found but not running, try: Start-Service sshd")
		return false, "Start-Service sshd"
	}

	return false, fmt.Sprintf(
		"SSH not installed. Install with:\n  Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0\n  Start-Service sshd\n  Set-Service -Name sshd -StartupType Automatic")
}
