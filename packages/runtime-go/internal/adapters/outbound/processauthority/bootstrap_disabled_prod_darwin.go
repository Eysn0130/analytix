//go:build darwin && analytix_prod

package processauthority

const (
	darwinBootstrapMarker      = ""
	darwinBootstrapEnvironment = ""
	darwinBootstrapNonceEnv    = ""
)

// These declarations keep the quarantined non-production build-probe session
// source type-checkable under production tags. No production entry point calls
// it, and command encoding can never reach an exec effect.
type darwinBootstrapCommand struct {
	target      string
	arguments   []string
	environment []string
}

func encodeDarwinBootstrapCommand(darwinBootstrapCommand) ([]byte, error) {
	return nil, ErrUnavailable
}
