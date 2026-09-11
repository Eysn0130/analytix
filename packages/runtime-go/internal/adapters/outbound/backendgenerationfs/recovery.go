package backendgenerationfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainbackend "analytix.local/runtime-go/internal/domain/backendgeneration"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const (
	privateCASRecoveryParticipantIDV4 = "backend-generation"
	privateCASRecoveryRootIDV4        = "runtime-sidecar-authority-v1/allocations"
	privateCASRecoverySemanticV4      = "analytix.backend-generation-private-cas-validator/v4"
)

type PreparedRecoveryV1 struct {
	prepared  *finalauthority.PreparedSecurePrivateCASRecoveryV1
	validated bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root, err := canonicalRootV1(root)
	if err != nil || access == nil {
		return nil, errors.New("backend generation recovery configuration is invalid")
	}
	prepared, err := finalauthority.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, root, domainbackend.MaxAllocationRecordBytesV1, access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{prepared: prepared}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.prepared == nil {
		return errors.New("backend generation recovery plan is invalid")
	}
	materials := make([]domainbackend.AllocationMaterialV1, 0)
	if err := prepared.prepared.VisitCommittedFiles(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		if len(materials) >= MaxAllocationInventoryV1 {
			return errors.New("backend generation recovery inventory exceeds its limit")
		}
		materials = append(materials, domainbackend.AllocationMaterialV1{
			Digest: file.Digest, Body: append([]byte(nil), file.Body...),
		})
		return nil
	}); err != nil {
		return err
	}
	if _, err := domainbackend.ValidateAllocationChainV1(materials); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.prepared == nil || !prepared.validated {
		return errors.New("backend generation recovery plan is not validated")
	}
	return prepared.prepared.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) ObserveJournalAuthorityPreflightV1(ctx context.Context) (string, error) {
	if err := prepared.Revalidate(ctx); err != nil {
		return "", err
	}
	digest := prepared.PrivateCASRecoveryAdditionalSemanticDigestV4()
	if digest == "" {
		return "", errors.New("backend generation journal-authority preflight digest is unavailable")
	}
	return digest, nil
}

func (prepared *PreparedRecoveryV1) ValidateJournalAuthorityPreflightV1(
	ctx context.Context,
	expected string,
) error {
	if err := prepared.Revalidate(ctx); err != nil || expected == "" ||
		prepared.PrivateCASRecoveryAdditionalSemanticDigestV4() != expected {
		return errors.New("backend generation journal-authority preflight changed")
	}
	return nil
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthority.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.prepared == nil {
		return nil
	}
	return []*finalauthority.PreparedSecurePrivateCASRecoveryV1{prepared.prepared}
}

func (*PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return nil
}

// PrivateCASRecoveryAdditionalSemanticDigestV4 binds the exact validator
// version which accepted the allocation chain. The frozen CAS plan separately
// binds every committed material and recovery target.
func (prepared *PreparedRecoveryV1) PrivateCASRecoveryAdditionalSemanticDigestV4() string {
	if prepared == nil || prepared.prepared == nil || !prepared.validated {
		return ""
	}
	digest := sha256.Sum256([]byte(privateCASRecoverySemanticV4))
	return hex.EncodeToString(digest[:])
}

// ApplyV4 is the standalone runtime-authority-command path. It deliberately
// requires the installation-signed journal; normal runtime startup includes
// this same root in its complete multi-owner V4 transaction.
func (prepared *PreparedRecoveryV1) ApplyV4(
	ctx context.Context,
	journal privatecasport.RecoveryJournalV1,
) error {
	if journal == nil {
		return errors.New("backend generation signed recovery journal is unavailable")
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	semanticDigest := prepared.PrivateCASRecoveryAdditionalSemanticDigestV4()
	if semanticDigest == "" {
		return errors.New("backend generation semantic recovery authority is unavailable")
	}
	return finalauthority.ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		ctx,
		[]finalauthority.PreparedSecurePrivateCASRecoveryParticipantV4{{
			ParticipantID:           privateCASRecoveryParticipantIDV4,
			SemanticAuthorityDigest: semanticDigest,
			Authority:               prepared,
			Roots: []finalauthority.SecurePrivateCASRecoveryRootBindingV4{{
				RootID: privateCASRecoveryRootIDV4, RootPath: prepared.prepared.RootPath(),
			}},
		}},
		journal,
	)
}
