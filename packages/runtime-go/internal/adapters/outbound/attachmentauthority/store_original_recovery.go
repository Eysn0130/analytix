package attachmentauthority

import (
	"context"
	"errors"
	"sort"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

// This explicit preservation adapter retains a complete five-leaf denominator,
// including proved missing leaves in an original partial owner. It supplies no
// creation or cleanup decision; runtime preservation freezes every target, and
// live opening still requires all five leaves to be present.
type originalAttachmentRecoveryOwnerV1 struct {
	*PreparedOriginalInventoryV1
	plans []*finalauthority.PreparedSecurePrivateCASRecoveryV1
}

func PrepareRecoveryPreservingOriginalCreateResiduesV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, proof *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1) (_ *PreparedRecoveryV1, resultErr error) {
	original, err := PrepareOriginalInventoryWithCreateResiduesV1(ctx, root, access, proof)
	if err != nil {
		return nil, err
	}
	owner := &originalAttachmentRecoveryOwnerV1{PreparedOriginalInventoryV1: original}
	defer func() { resultErr = errors.Join(resultErr, owner.Revalidate(ctx)) }()
	names := make([]string, 0, len(originalAttachmentLeafLimitsV1))
	for name := range originalAttachmentLeafLimitsV1 {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		plan := original.leaves[name]
		if plan == nil {
			return nil, errors.New("original attachment physical leaf denominator is incomplete")
		}
		owner.plans = append(owner.plans, plan)
	}
	return &PreparedRecoveryV1{root: original.root, owner: owner}, nil
}

func (owner *originalAttachmentRecoveryOwnerV1) Revalidate(ctx context.Context) error {
	if owner == nil || owner.PreparedOriginalInventoryV1 == nil {
		return errors.New("original attachment body recovery is unavailable")
	}
	return owner.PreparedOriginalInventoryV1.Revalidate(ctx)
}

func (owner *originalAttachmentRecoveryOwnerV1) SecurePrivateCASRecoveryPlansV2() []*finalauthority.PreparedSecurePrivateCASRecoveryV1 {
	return append([]*finalauthority.PreparedSecurePrivateCASRecoveryV1(nil), owner.plans...)
}

func (owner *originalAttachmentRecoveryOwnerV1) PrivateCASRecoveryTopologiesV3() []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3{owner.topology}
}
