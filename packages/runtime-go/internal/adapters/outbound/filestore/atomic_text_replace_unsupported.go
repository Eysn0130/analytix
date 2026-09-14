//go:build !darwin && !linux && !windows

package filestore

import "fmt"

func inspectAtomicTextTargetPlatform(string, bool, int64, atomicTextReadPolicy) (atomicTextState, error) {
	return atomicTextState{}, fmt.Errorf("%w: secure no-follow replacement is unavailable", ErrAtomicTextUnsupportedPlatform)
}

func atomicReplaceTextPlatform(atomicTextReplaceRequest, *atomicTextTestHooks) error {
	return fmt.Errorf("%w: secure no-follow replacement is unavailable", ErrAtomicTextUnsupportedPlatform)
}
