//go:build !windows && !darwin

package client

import (
	"os"
	"runtime"
	"strings"
)

func machineModel() string {
	if data, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_name"); err == nil {
		return strings.TrimSpace(string(data))
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}
