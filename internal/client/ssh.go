package client

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/wjzhangq/sshlink/internal/common"
)

// CheckSSHService 检查 SSH 服务是否运行
// 返回 (running bool, installHint string)
func CheckSSHService() (bool, string) {
	switch runtime.GOOS {
	case "windows":
		return checkSSHWindows()
	case "darwin":
		return checkSSHDarwin()
	default:
		return checkSSHLinux()
	}
}

func checkSSHWindows() (bool, string) {
	// 检查 sshd 服务
	out, err := exec.Command("powershell", "-Command",
		"(Get-Service -Name sshd -ErrorAction SilentlyContinue).Status").Output()
	if err == nil && strings.TrimSpace(string(out)) == "Running" {
		return true, ""
	}

	// 检查 OpenSSH 可执行文件
	_, err = exec.LookPath("sshd")
	if err == nil {
		common.Info("sshd found but not running, try: Start-Service sshd")
		return false, "Start-Service sshd"
	}

	return false, fmt.Sprintf(
		"SSH not installed. Install with:\n  Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0\n  Start-Service sshd\n  Set-Service -Name sshd -StartupType Automatic")
}

func checkSSHDarwin() (bool, string) {
	out, err := exec.Command("systemsetup", "-getremotelogin").Output()
	if err == nil && strings.Contains(string(out), "On") {
		return true, ""
	}

	return false, "SSH not enabled. Enable with:\n  sudo systemsetup -setremotelogin on"
}

func checkSSHLinux() (bool, string) {
	// 尝试 systemctl
	out, err := exec.Command("systemctl", "is-active", "sshd").Output()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		return true, ""
	}

	// 尝试 ssh 服务名
	out, err = exec.Command("systemctl", "is-active", "ssh").Output()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		return true, ""
	}

	// 检查进程
	out, err = exec.Command("pgrep", "-x", "sshd").Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return true, ""
	}

	return false, "SSH not running. Install/start with:\n  # Debian/Ubuntu: sudo apt install openssh-server && sudo systemctl start sshd\n  # RHEL/CentOS:  sudo yum install openssh-server && sudo systemctl start sshd"
}
