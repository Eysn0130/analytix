//go:build !darwin && !analytix_prod

package processauthority

func dispatchBootstrap(_ []string, _ int) error {
	return ErrUnavailable
}
