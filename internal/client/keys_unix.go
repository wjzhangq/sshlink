//go:build !windows

package client

import "os"

func fixPermissions(path string) error {
	return os.Chmod(path, 0600)
}
