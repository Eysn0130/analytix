package evidenceauthority

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

const (
	bundlesLeafV1      = "bundles"
	observationsLeafV1 = "observations"
)

// PreparedRecoveryV1 binds the two existing shared-evidence immutable stores
// into the runtime's one signed private-CAS recovery transaction.
type PreparedRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("evidence authority prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("evidence authority prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx,
		absolute,
		[]finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: bundlesLeafV1, MaxBytes: maxEvidenceAuthorityBundleBytes},
			{Name: observationsLeafV1, MaxBytes: maxEvidenceObservationBytes},
		},
		access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("evidence authority recovery plan is invalid")
	}
	err := prepared.owner.ValidateDomainSemantics(ctx, func(visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
		return prepared.validateDomainSemantics(ctx, visit, visitMaterials)
	})
	prepared.validated = err == nil
	return err
}

func (prepared *PreparedRecoveryV1) validateDomainSemantics(ctx context.Context, visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
	bundles := make(map[string]domainevidence.EvidenceAuthorityBundleV1)
	observations := make(map[string]struct {
		bundleDigest string
		bundle       domainevidence.EvidenceAuthorityBundleV1
	})
	if err := visit(bundlesLeafV1, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		bundle, err := parseStoredBundle(file.Digest, file.Body)
		if err != nil || bundle.RecordDigest != file.Digest {
			return errors.Join(errors.New("evidence authority recovery contains a corrupt bundle"), err)
		}
		if _, duplicate := bundles[file.Digest]; duplicate {
			return errors.New("evidence authority recovery contains a duplicate bundle")
		}
		bundles[file.Digest] = bundle
		return nil
	}); err != nil {
		return err
	}
	if err := visit(observationsLeafV1, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		observation, err := parseStoredObservation(file.Digest, file.Body)
		if err != nil || observation.Observation.ObservationDigest != file.Digest {
			return errors.Join(errors.New("evidence authority recovery contains a corrupt observation"), err)
		}
		if _, duplicate := observations[file.Digest]; duplicate {
			return errors.New("evidence authority recovery contains a duplicate observation")
		}
		observations[file.Digest] = struct {
			bundleDigest string
			bundle       domainevidence.EvidenceAuthorityBundleV1
		}{bundleDigest: observation.Bundle.RecordDigest, bundle: observation.Bundle}
		return nil
	}); err != nil {
		return err
	}
	for _, bundle := range bundles {
		if bundle.Generation == 1 {
			if bundle.PreviousBundleDigest != "" {
				return errors.New("evidence authority recovery genesis has a predecessor")
			}
			continue
		}
		previous, found := bundles[bundle.PreviousBundleDigest]
		if !found || domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, bundle) != nil {
			return errors.New("evidence authority recovery bundle lost its exact predecessor")
		}
	}
	for _, observation := range observations {
		bundle, found := bundles[observation.bundleDigest]
		if !found || !equalBundle(bundle, observation.bundle) {
			return errors.New("evidence authority recovery observation lost its exact bundle")
		}
	}
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil || !prepared.validated {
		return errors.New("evidence authority recovery plan is not validated")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.owner}
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}
