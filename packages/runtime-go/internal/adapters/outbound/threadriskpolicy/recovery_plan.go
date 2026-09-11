package threadriskpolicy

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

// PreparedRecoveryV1 validates every committed policy before the shared
// private-CAS recovery transaction may mutate any residue.
type PreparedRecoveryV1 struct {
	cas       *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1
	validated bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("thread risk policy prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("thread risk policy prepared recovery root is invalid")
	}
	plan, err := finalauthorityadapter.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, absolute, maxThreadRiskPolicyBytes, access,
	)
	if err != nil {
		return nil, err
	}
	if err := plan.Revalidate(ctx); err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{cas: plan}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.cas == nil {
		return errors.New("thread risk policy recovery plan is invalid")
	}
	if err := prepared.cas.VisitCommittedFiles(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		policy, err := parseStoredPolicy(file.Digest, file.Body)
		if err != nil || policy.PolicyDigest != file.Digest {
			return errors.Join(errors.New("thread risk policy recovery contains a corrupt record"), err)
		}
		return nil
	}); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.cas == nil || !prepared.validated {
		return errors.New("thread risk policy recovery plan is not validated")
	}
	return prepared.cas.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return nil
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.cas == nil {
		return nil
	}
	return []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1{prepared.cas}
}
