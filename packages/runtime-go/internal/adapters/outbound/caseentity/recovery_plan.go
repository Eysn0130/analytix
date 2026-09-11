package caseentity

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
)

// PreparedRecoveryV1 reuses the shared SecurePrivateCAS owner recovery while
// binding the exact three case-entity leaves and their canonical domain
// records. It does not grant dataset, entity-resolution, or disclosure
// authority.
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
		return nil, errors.New("case entity private state recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("case entity private state recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx,
		absolute,
		[]finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
			{Name: bindingPartitionV1, MaxBytes: domaincaseentity.MaxCaseEntityBindingRecordBytesV1},
			{Name: ingressPartitionV1, MaxBytes: domaincaseentity.MaxCaseIngressRecordBytesV1},
			{Name: threadContextPartitionV1, MaxBytes: domaincaseentity.MaxThreadCaseContextRecordBytesV1},
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
		return errors.New("case entity private state recovery plan is invalid")
	}
	prepared.validated = false
	err := prepared.owner.ValidateDomainSemantics(ctx, func(visit finalauthorityadapter.PrivateCASDomainVisitor, _ finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
		bindings := make(map[string]domaincaseentity.CaseEntityBindingRecord)
		if err := visit(bindingPartitionV1, func(file finalauthorityadapter.SecurePrivateCASFile) error {
			record, err := recoveredBindingV1(file)
			if err != nil {
				return err
			}
			bindings[record.BindingKey] = record
			return nil
		}); err != nil {
			return err
		}
		if _, err := normalizeBindingInventoryV1(bindings); err != nil {
			return errors.New("case entity private state recovery contains an invalid case-scoped stable ordinal inventory")
		}
		if err := visit(ingressPartitionV1, validateRecoveredIngressV1); err != nil {
			return err
		}
		if err := visit(threadContextPartitionV1, validateRecoveredThreadContextV1); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil || !prepared.validated {
		return errors.New("case entity private state recovery plan is not validated")
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

func validateRecoveredBindingV1(file finalauthorityadapter.SecurePrivateCASFile) error {
	_, err := recoveredBindingV1(file)
	return err
}

func recoveredBindingV1(file finalauthorityadapter.SecurePrivateCASFile) (domaincaseentity.CaseEntityBindingRecord, error) {
	record, parseErr := domaincaseentity.ParseCaseEntityBindingRecordV1(file.Body)
	canonical, canonicalErr := domaincaseentity.CaseEntityBindingRecordV1Bytes(record)
	if parseErr != nil || canonicalErr != nil || record.BindingKey != file.Digest || !bytes.Equal(canonical, file.Body) {
		return domaincaseentity.CaseEntityBindingRecord{}, errors.Join(
			errors.New("case entity private state recovery contains a non-canonical or key-mismatched binding"),
			parseErr,
			canonicalErr,
		)
	}
	return record, nil
}

func validateRecoveredIngressV1(file finalauthorityadapter.SecurePrivateCASFile) error {
	record, parseErr := domaincaseentity.ParseCaseIngressRecordV1(file.Body)
	canonical, canonicalErr := domaincaseentity.CaseIngressRecordV1Bytes(record)
	if parseErr != nil || canonicalErr != nil || record.IngressID != file.Digest || !bytes.Equal(canonical, file.Body) {
		return errors.Join(
			errors.New("case entity private state recovery contains a non-canonical or key-mismatched ingress"),
			parseErr,
			canonicalErr,
		)
	}
	return nil
}

func validateRecoveredThreadContextV1(file finalauthorityadapter.SecurePrivateCASFile) error {
	record, parseErr := domaincaseentity.ParseThreadCaseContextRecordV1(file.Body)
	canonical, canonicalErr := domaincaseentity.ThreadCaseContextRecordV1Bytes(record)
	if parseErr != nil || canonicalErr != nil || record.StorageKey != file.Digest || !bytes.Equal(canonical, file.Body) {
		return errors.Join(
			errors.New("case entity private state recovery contains a non-canonical or key-mismatched thread context"),
			parseErr,
			canonicalErr,
		)
	}
	return nil
}
