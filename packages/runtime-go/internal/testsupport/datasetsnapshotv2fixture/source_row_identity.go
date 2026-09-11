package datasetsnapshotv2fixture

import (
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type SourceRowIdentityV1 struct {
	SourceRecordID  string
	SourceFileID    string
	SourceRowNumber uint64
}

func NewSourceRowIdentityV1(
	securityContext domainsecurity.TurnSecurityContext,
	sourceFileID string,
	sourceRowNumber uint64,
) (SourceRowIdentityV1, error) {
	policy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(
		domainevidence.FundsCanonicalTransactionSourceRowPolicyIDV1,
	)
	if !ok {
		return SourceRowIdentityV1{}, errors.New("canonical source-row producer policy is unavailable")
	}
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID:                 securityContext.TenantID,
			UserID:                   securityContext.UserID,
			WorkspaceRealPath:        securityContext.WorkspaceRealPath,
			CaseID:                   securityContext.CaseID,
			CaseBindingHash:          securityContext.CaseBindingHash,
			BindingObservationDigest: securityContext.PublicationPolicy.BindingObservationDigest,
		},
	)
	if err != nil {
		return SourceRowIdentityV1{}, err
	}
	locator, err := domainevidence.NewSourceRowLocatorV1(
		policy,
		domainevidence.SourceRowLocatorInputV1{
			SourceFileID:    sourceFileID,
			SourceRowNumber: sourceRowNumber,
		},
	)
	if err != nil {
		return SourceRowIdentityV1{}, err
	}
	sourceRecordID, err := domainevidence.DeriveSourceRowRecordIDV1(
		policy,
		binding,
		domainsecurity.SHA256Hex([]byte("identity-only-source-artifact:"+sourceFileID)),
		locator,
	)
	if err != nil {
		return SourceRowIdentityV1{}, err
	}
	return SourceRowIdentityV1{
		SourceRecordID:  sourceRecordID,
		SourceFileID:    sourceFileID,
		SourceRowNumber: sourceRowNumber,
	}, nil
}
