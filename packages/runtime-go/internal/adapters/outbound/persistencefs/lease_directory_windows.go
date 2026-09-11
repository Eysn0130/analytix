//go:build windows

package persistencefs

import (
	"path/filepath"
	"syscall"
)

func platformLeaseDirectory() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()
	buffer := make([]uint16, 32768)
	length := uint32(len(buffer))
	if err := syscall.GetUserProfileDirectory(token, &buffer[0], &length); err != nil {
		return "", err
	}
	profile := syscall.UTF16ToString(buffer[:length])
	return filepath.Join(profile, "AppData", "Local", "Analytix", "persistence-leases-v1"), nil
}
