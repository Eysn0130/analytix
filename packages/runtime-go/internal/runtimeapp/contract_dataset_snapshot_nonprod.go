//go:build !analytix_prod

package runtimeapp

import (
	"context"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type contractDatasetSnapshotAuthority struct {
	installation finalauthorityport.Authority
}

func newContractDatasetSnapshotAuthority(
	config Config,
	installation finalauthorityport.Authority,
) (datasetsnapshotport.Authority, bool, error) {
	if strings.TrimSpace(config.DurableTempDir) == "" ||
		strings.TrimSpace(config.ProductionDurableRoot) != "" ||
		strings.TrimSpace(config.CandidateDurableRoot) != "" {
		return nil, false, nil
	}
	if installation == nil || !domainsecurity.IsSHA256Hex(installation.KeyID()) {
		return nil, true, errors.New("contract dataset snapshot authority requires an installation authority")
	}
	return contractDatasetSnapshotAuthority{installation: installation}, true, nil
}

func (authority contractDatasetSnapshotAuthority) ResolveWitnessed(
	ctx context.Context,
	input datasetsnapshotport.ResolveInput,
) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	if ctx == nil || authority.installation == nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	observation := input.Observation
	if domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.State != domainsecurity.CaseBindingStateValid ||
		strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.UserID) == "" {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrMismatch
	}
	publicKey := append([]byte(nil), authority.installation.PublicKey()...)
	keyID := strings.TrimSpace(authority.installation.KeyID())
	record, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(
		domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
			InstallationID: keyID, TenantID: input.TenantID, UserID: input.UserID,
			WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
			CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("analytix.contract-dataset-source-manifest/v1\x00" + observation.ObservationDigest)),
			RawManifestSHA256:  observation.BindingSHA256, ParserVersion: "analytix-contract-case-binding/v1",
			AcceptedAt: time.Unix(1, 0).UTC(), AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
		},
		func(message []byte) ([]byte, error) { return authority.installation.Sign(ctx, message) },
	)
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	return record, nil
}
