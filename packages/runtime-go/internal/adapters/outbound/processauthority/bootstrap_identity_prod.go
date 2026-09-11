//go:build darwin && analytix_prod

package processauthority

func currentBootstrapMainAllowed() bool {
	return false
}
