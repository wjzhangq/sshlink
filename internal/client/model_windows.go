//go:build windows

package client

import (
	"os/exec"
	"runtime"
	"strings"
)

func machineModel() string {
	if out, err := exec.Command("powershell", "-Command",
		"(Get-WmiObject -Class Win32_ComputerSystem).Model").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}
