//go:build !windows && !darwin

package persistencefs

func platformPathVolumeCaseInsensitive(string) (bool, bool) {
	return false, false
}

func platformKnownVolumeCaseInsensitive(string, uint64) (bool, bool) {
	return false, true
}
