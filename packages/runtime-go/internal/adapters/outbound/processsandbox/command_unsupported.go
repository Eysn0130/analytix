//go:build !darwin

package processsandbox

func preparePlatform(Invocation, []string, []string, uint16) (Invocation, error) {
	return Invocation{}, ErrUnavailable
}
