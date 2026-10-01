package evidenceauthorityhostlocal

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
)

const hostLocalHeadsLeafV1 = "heads"

// PreparedRecoveryV1 binds only the mode-specific immutable head history.
// It never selects a current head or admits a profile. The runtime fixed
// point must anchor every record to its installation and exact selector.
type PreparedRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PrepareRecoveryV1(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("host-local recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("host-local recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(ctx, absolute,
		[]finalauthorityadapter.SecurePrivateCASOwnerLeafV1{{Name: hostLocalHeadsLeafV1, MaxBytes: maxHostLocalHeadBytesV1}}, access)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("host-local recovery plan is invalid")
	}
	err := prepared.owner.ValidateDomainSemantics(ctx, func(visit finalauthorityadapter.PrivateCASDomainVisitor, _ finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
		heads := make(map[string]domainhost.HeadV1)
		if err := visit(hostLocalHeadsLeafV1, func(file finalauthorityadapter.SecurePrivateCASFile) error {
			if len(heads) >= maxHostLocalHeadCountV1 {
				return errors.New("host-local recovery head inventory exceeds its bound")
			}
			head, err := domainhost.ParseHeadV1(file.Body)
			if err != nil || head.RecordDigest != file.Digest {
				return errors.Join(errors.New("host-local recovery contains a corrupt head"), err)
			}
			if _, duplicate := heads[file.Digest]; duplicate {
				return errors.New("host-local recovery repeats a head")
			}
			heads[file.Digest] = head
			return nil
		}); err != nil {
			return err
		}
		return validateHostLocalRecoveryChainV1(heads)
	})
	prepared.validated = err == nil
	return err
}

func validateHostLocalRecoveryChainV1(heads map[string]domainhost.HeadV1) error {
	if len(heads) == 0 {
		return nil
	}
	children := make(map[string]uint64, len(heads))
	genesisCount := 0
	for _, head := range heads {
		if head.Generation == 0 {
			if validateHostLocalGenesisV1(head) != nil {
				return errors.New("host-local recovery mode commitment is invalid")
			}
			genesisCount++
			continue
		}
		previous, exists := heads[head.PreviousHeadDigest]
		if !exists || domainhost.ValidateHeadTransitionV1(previous, head) != nil {
			return errors.New("host-local recovery head lost its exact predecessor")
		}
		children[head.PreviousHeadDigest]++
		if children[head.PreviousHeadDigest] > 1 {
			return errors.New("host-local recovery head has a fork")
		}
	}
	if genesisCount != 1 {
		return errors.New("host-local recovery has no unique mode commitment")
	}
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil || !prepared.validated {
		return errors.New("host-local recovery plan is not validated")
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
