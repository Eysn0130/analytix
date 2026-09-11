//go:build !darwin && !linux && !windows

package finalauthority

import "errors"

type acceptedFinalCASRootAuthority struct{}

func captureAcceptedFinalCASRoot(string) (acceptedFinalCASRootAuthority, error) {
	return acceptedFinalCASRootAuthority{}, errors.New("accepted final CAS secure reader is unsupported")
}

func (acceptedFinalCASRootAuthority) readPrimaryThreadJSON(string) ([]byte, error) {
	return nil, errors.New("accepted final CAS secure reader is unsupported")
}

func (acceptedFinalCASRootAuthority) readThreadFile(string, string) ([]byte, error) {
	return nil, errors.New("Core snapshot secure reader is unsupported")
}
