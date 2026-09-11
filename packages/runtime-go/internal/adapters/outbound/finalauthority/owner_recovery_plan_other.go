//go:build !darwin && !linux && !windows

package finalauthority

import "errors"

func observeSecurePrivateCASOwnerContainerV1(
	privateCASRootAuthority,
	[]string,
	map[string]int64,
) (securePrivateCASOwnerContainerObservationV1, error) {
	return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS owner recovery is unsupported on this platform")
}

func discoverSecurePrivateCASOwnerContainerV2(
	privateCASRootAuthority,
	*securePrivateCASOwnerDiscoveryV2,
) (securePrivateCASOwnerContainerObservationV1, []SecurePrivateCASOwnerEntryV2, error) {
	return securePrivateCASOwnerContainerObservationV1{}, nil, errors.New("private CAS owner discovery is unsupported on this platform")
}
