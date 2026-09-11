//go:build darwin

package nativecomponenthost

// OpenProduction admits either the release-trusted packaged Darwin host or the
// separately bounded local non-publishable development host. The candidate
// proves the exact package/runtime/native identities before it creates private
// roots directly beneath the caller-validated exact profile root; missing trust
// leaves only the optional Funds capability unavailable.
func OpenProduction(profileRoot string) (*Owner, error) {
	return openDarwinHostCandidate(profileRoot)
}
