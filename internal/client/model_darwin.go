//go:build darwin

package client

import (
	"os/exec"
	"runtime"
	"strings"
)

func machineModel() string {
	if out, err := exec.Command("sysctl", "-n", "hw.model").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}
