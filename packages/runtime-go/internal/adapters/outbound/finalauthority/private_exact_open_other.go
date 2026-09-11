//go:build !darwin

package finalauthority

func platformPrivateCASUnixOpenExactBoundDirectory(int, string, uint64) (int, bool, bool, error) {
	return -1, false, false, nil
}
