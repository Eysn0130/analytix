//go:build !windows

package persistencefs

import (
	"fmt"
	"os"
	"path/filepath"
)

func platformLeaseDirectory() (string, error) {
	return filepath.Join("/tmp", fmt.Sprintf("analytix-persistence-leases-v1-%d", os.Geteuid())), nil
}
