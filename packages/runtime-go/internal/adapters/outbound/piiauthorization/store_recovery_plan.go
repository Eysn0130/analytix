package piiauthorization

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
)

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
		return nil, errors.New("PII authorization prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("PII authorization prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx, absolute, originalGrantLeavesV1(), access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("PII authorization recovery plan is invalid")
	}
	err := prepared.owner.ValidateDomainSemantics(ctx, func(visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
		return prepared.validateDomainSemantics(ctx, visit, visitMaterials, nil)
	})
	prepared.validated = err == nil
	return err
}

func (prepared *PreparedRecoveryV1) validateDomainSemantics(ctx context.Context, visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor, checkInstallation func(string, string) error) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := visit("grants", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		grant, err := domainpii.ParsePIIProjectionGrantV1(file.Body)
		canonical, canonicalErr := domainpii.PIIProjectionGrantV1Bytes(grant)
		if err != nil || canonicalErr != nil || grant.RecordDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("PII authorization recovery contains a non-canonical or content-address corrupt grant"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(grant.AuthorityKeyID, grant.AuthorityPublicKey); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("PII authorization recovery plan is invalid")
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

func (prepared *PreparedRecoveryV1) ValidateInstallation(ctx context.Context, authority *finalauthorityadapter.AnchoredFileAuthority) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("private domain recovery plan is unavailable")
	}
	err := prepared.owner.ValidateDomainInstallation(ctx, authority, func(visit finalauthorityadapter.PrivateCASDomainVisitor, materials finalauthorityadapter.PrivateCASDomainMaterialVisitor, check func(string, string) error) error {
		return prepared.validateDomainSemantics(ctx, visit, materials, check)
	})
	prepared.validated = err == nil
	return err
}

func originalGrantLeavesV1() []finalauthorityadapter.SecurePrivateCASOwnerLeafV1 {
	return []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{{Name: "grants", MaxBytes: maxPIIProjectionGrantBytesV1}}
}

// PrepareOriginalObservationV1 observes all raw entries without opening a writable store.
func PrepareOriginalObservationV1(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority, originals ...*finalauthorityadapter.PreparedSecurePrivateCASOriginalCreateResiduesV1) (*finalauthorityadapter.OriginalFixedOwnerObservationV1, error) {
	return finalauthorityadapter.PrepareOriginalFixedOwnerObservationV1(ctx, root, originalGrantLeavesV1(), access, originals...)
}

// ValidateOriginalEntriesV1 checks a complete original or signed endpoint with the existing domain rules.
func ValidateOriginalEntriesV1(ctx context.Context, files map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1, authority *finalauthorityadapter.AnchoredFileAuthority, localCheck func(string, string) error, creates *finalauthorityadapter.PreparedSecurePrivateCASOriginalCreateResiduesV1) error {
	return finalauthorityadapter.ValidateOriginalDomainEntriesV1(ctx, files, originalGrantLeavesV1(), authority, localCheck, creates, func(visit finalauthorityadapter.PrivateCASDomainVisitor, materials finalauthorityadapter.PrivateCASDomainMaterialVisitor, check func(string, string) error) error {
		return (&PreparedRecoveryV1{}).validateDomainSemantics(ctx, visit, materials, check)
	})
}
