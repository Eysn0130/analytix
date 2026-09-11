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

// PreparedAccessRecoveryV1 owns only the private audit journal for controlled
// artifact access. PII grants remain under the existing pii-authorization
// owner, so deployed grant-only installations do not require an unsafe
// one-to-three-leaf topology migration.
type PreparedAccessRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

func PrepareAccessRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedAccessRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("controlled artifact access prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("controlled artifact access prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx,
		absolute,
		originalAccessLeavesV1(),
		access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedAccessRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedAccessRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("controlled artifact access recovery plan is invalid")
	}
	err := prepared.owner.ValidateDomainSemantics(ctx, func(visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
		return prepared.validateDomainSemantics(ctx, visit, visitMaterials, nil)
	})
	prepared.validated = err == nil
	return err
}

func (prepared *PreparedAccessRecoveryV1) validateDomainSemantics(ctx context.Context, visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor, checkInstallation func(string, string) error) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	receipts := make(map[string]domainpii.ControlledArtifactAccessReceiptV1)
	if err := visit("access-receipts", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainpii.ParseControlledArtifactAccessReceiptV1(file.Body)
		canonical, canonicalErr := domainpii.ControlledArtifactAccessReceiptV1Bytes(receipt)
		if err != nil || canonicalErr != nil || receipt.AccessID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("controlled artifact access recovery contains a corrupt receipt"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(receipt.AuthorityKeyID, receipt.AuthorityPublicKey); err != nil {
				return err
			}
		}
		receipts[file.Digest] = receipt
		return nil
	}); err != nil {
		return err
	}
	if err := visit("access-dispositions", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domainpii.ParseControlledArtifactAccessDispositionV1(file.Body)
		canonical, canonicalErr := domainpii.ControlledArtifactAccessDispositionV1Bytes(disposition)
		receipt, found := receipts[file.Digest]
		if err != nil || canonicalErr != nil || disposition.AccessID != file.Digest || !bytes.Equal(canonical, file.Body) ||
			!found || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt) != nil {
			return errors.Join(errors.New("controlled artifact access recovery contains an orphan or corrupt disposition"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(disposition.AuthorityKeyID, disposition.AuthorityPublicKey); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (prepared *PreparedAccessRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("controlled artifact access recovery plan is invalid")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedAccessRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.owner}
}

func (prepared *PreparedAccessRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}

func (prepared *PreparedAccessRecoveryV1) ValidateInstallation(ctx context.Context, authority *finalauthorityadapter.AnchoredFileAuthority) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("private domain recovery plan is unavailable")
	}
	err := prepared.owner.ValidateDomainInstallation(ctx, authority, func(visit finalauthorityadapter.PrivateCASDomainVisitor, materials finalauthorityadapter.PrivateCASDomainMaterialVisitor, check func(string, string) error) error {
		return prepared.validateDomainSemantics(ctx, visit, materials, check)
	})
	prepared.validated = err == nil
	return err
}

func originalAccessLeavesV1() []finalauthorityadapter.SecurePrivateCASOwnerLeafV1 {
	return []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
		{Name: "access-receipts", MaxBytes: domainpii.MaxControlledArtifactAccessRecordBytesV1},
		{Name: "access-dispositions", MaxBytes: domainpii.MaxControlledArtifactAccessRecordBytesV1},
	}
}

// PrepareOriginalAccessObservationV1 observes all raw entries without opening a writable store.
func PrepareOriginalAccessObservationV1(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority, originals ...*finalauthorityadapter.PreparedSecurePrivateCASOriginalCreateResiduesV1) (*finalauthorityadapter.OriginalFixedOwnerObservationV1, error) {
	return finalauthorityadapter.PrepareOriginalFixedOwnerObservationV1(ctx, root, originalAccessLeavesV1(), access, originals...)
}

// ValidateOriginalAccessEntriesV1 checks a complete original or signed endpoint with the existing domain rules.
func ValidateOriginalAccessEntriesV1(ctx context.Context, files map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1, authority *finalauthorityadapter.AnchoredFileAuthority, localCheck func(string, string) error, creates *finalauthorityadapter.PreparedSecurePrivateCASOriginalCreateResiduesV1) error {
	return finalauthorityadapter.ValidateOriginalDomainEntriesV1(ctx, files, originalAccessLeavesV1(), authority, localCheck, creates, func(visit finalauthorityadapter.PrivateCASDomainVisitor, materials finalauthorityadapter.PrivateCASDomainMaterialVisitor, check func(string, string) error) error {
		return (&PreparedAccessRecoveryV1{}).validateDomainSemantics(ctx, visit, materials, check)
	})
}
