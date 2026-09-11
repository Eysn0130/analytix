//go:build darwin

package persistencefs

import (
	"errors"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const darwinPathconfCaseSensitive = 11

var darwinVolumeCaseSensitivity sync.Map

func platformPathVolumeCaseInsensitive(path string) (bool, bool) {
	current := filepath.Clean(path)
	for {
		var stat unix.Stat_t
		err := unix.Stat(current, &stat)
		if err == nil {
			device := uint64(stat.Dev)
			return platformKnownVolumeCaseInsensitive(current, device)
		}
		if !errors.Is(err, unix.ENOENT) {
			return false, false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, false
		}
		current = parent
	}
}

func platformKnownVolumeCaseInsensitive(path string, device uint64) (bool, bool) {
	if device == 0 {
		return false, false
	}
	if cached, ok := darwinVolumeCaseSensitivity.Load(device); ok {
		return cached.(bool), true
	}
	value, err := unix.Pathconf(path, darwinPathconfCaseSensitive)
	if err != nil {
		return false, false
	}
	caseInsensitive := value == 0
	cached, _ := darwinVolumeCaseSensitivity.LoadOrStore(device, caseInsensitive)
	return cached.(bool), true
}
