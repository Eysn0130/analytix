//go:build linux

package persistencefs

func platformSecureOpenExistingAbsoluteDirectory(string) (int, bool, error) {
	return -1, false, nil
}
