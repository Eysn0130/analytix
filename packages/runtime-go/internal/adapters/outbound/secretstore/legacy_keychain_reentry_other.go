//go:build !darwin

package secretstore

func InspectLegacyKeychainProfile(string) (*LegacyKeychainProfile, error) {
	return nil, nil
}

func verifyLegacyKeychainMarker(string) error {
	return nil
}
