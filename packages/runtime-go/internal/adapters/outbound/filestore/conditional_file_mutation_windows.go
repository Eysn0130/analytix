//go:build windows

package filestore

import (
	"fmt"
)

func openConditionalMutationAuthorityPlatform(string) (ConditionalMutationAuthority, error) {
	return ConditionalMutationAuthority{}, nil
}

func openExistingConditionalMutationAuthorityPlatform(string) (ConditionalMutationAuthority, bool, error) {
	return ConditionalMutationAuthority{}, false, nil
}

func prepareConditionalMoveSourcePlatform(string) (conditionalMoveSourceObservation, error) {
	return conditionalMoveSourceObservation{}, fmt.Errorf("%w: Windows conditional move identity CAS is unavailable", ErrAtomicTextUnsupportedPlatform)
}

func prepareConditionalMoveDestinationPlatform(string) (bool, error) {
	return false, fmt.Errorf("%w: Windows conditional move identity CAS is unavailable", ErrAtomicTextUnsupportedPlatform)
}

func applyConditionalMovePlatform(MoveRegularFilePlan) error {
	return fmt.Errorf("%w: Windows conditional move identity CAS is unavailable", ErrAtomicTextUnsupportedPlatform)
}

func conditionalDeleteExactPlatform(string, string, ConditionalMutationAuthority, *atomicTextTestHooks) error {
	return fmt.Errorf("%w: Windows conditional delete identity CAS is unavailable", ErrAtomicTextUnsupportedPlatform)
}
