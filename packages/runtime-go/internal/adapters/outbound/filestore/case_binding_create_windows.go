//go:build windows

package filestore

import (
	"context"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func createCaseBindingPlatform(context.Context, string, domainsecurity.CaseBindingObservationV1, string, string, []byte, caseBindingReadHook) (domainsecurity.CaseBindingObservationV1, error) {
	return domainsecurity.CaseBindingObservationV1{}, ErrCaseBindingCreateUnavailable
}

func observeMissingImportWorkspacePlatform(string) (domainsecurity.CaseBindingObservationV1, string, error) {
	return domainsecurity.CaseBindingObservationV1{}, "", ErrCaseBindingCreateUnavailable
}
