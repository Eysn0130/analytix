//go:build !darwin && !linux && !windows

package secureconfigfs

func readBundle(normalizedBundle) (map[string][]byte, error) {
	return nil, ErrUnsupported
}
