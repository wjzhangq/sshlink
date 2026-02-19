//go:build windows

package client

// fixPermissions is a no-op on Windows; file ACLs are managed by the OS.
func fixPermissions(path string) error {
	return nil
}
