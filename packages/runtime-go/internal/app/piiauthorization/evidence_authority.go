package piiauthorization

import (
	"context"
	"errors"
	"sort"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

type WitnessedEvidenceAuthority struct {
	snapshots registryport.WitnessedSnapshotAuthority
}

func NewWitnessedEvidenceAuthority(reader registryport.WitnessedSnapshotReader) (*WitnessedEvidenceAuthority, error) {
	snapshots, ok := reader.(registryport.WitnessedSnapshotAuthority)
	if reader == nil || !ok {
		return nil, ErrUnavailable
	}
	return &WitnessedEvidenceAuthority{snapshots: snapshots}, nil
}

var _ piiauthorizationport.EvidenceAuthority = (*WitnessedEvidenceAuthority)(nil)
var _ piiauthorizationport.ControlledPIIEvidenceAuthorityV2 = (*WitnessedEvidenceAuthority)(nil)

func (authority *WitnessedEvidenceAuthority) ValidateCurrent(ctx context.Context, input piiauthorizationport.EvidenceValidationV1) error {
	if authority == nil || authority.snapshots == nil || ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainpublication.ValidateClaimLedgerV1(input.ClaimLedger) != nil || len(input.FieldBindings) == 0 {
		return ErrMismatch
	}
	return authority.snapshots.WithWitnessedSnapshotAuthority(ctx, input.Context, func(
		snapshot registryport.WitnessedSnapshot,
		capability registryport.WitnessedSnapshotCapability,
	) error {
		return capability.UseExact(snapshot, func() error {
			return authority.validateSnapshotEvidenceV2(ctx, input, snapshot)
		})
	})
}

// ValidateWitnessed validates an exact snapshot only while the fresh-reader
// capability that issued it remains active.
func (authority *WitnessedEvidenceAuthority) ValidateWitnessed(
	ctx context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	snapshot registryport.WitnessedSnapshot,
	capability registryport.WitnessedSnapshotCapability,
) error {
	if authority == nil || ctx == nil || capability == nil {
		return ErrMismatch
	}
	return capability.UseExact(snapshot, func() error {
		return authority.validateSnapshotEvidenceV2(ctx, input, snapshot)
	})
}

func (authority *WitnessedEvidenceAuthority) validateSnapshotEvidenceV2(
	ctx context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	snapshot registryport.WitnessedSnapshot,
) error {
	if authority == nil || ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainpublication.ValidateClaimLedgerV1(input.ClaimLedger) != nil || len(input.FieldBindings) == 0 ||
		!sameControlledPIIApprovalContextV1(snapshot.Context, input.Context) ||
		domainevidence.ValidateEvidenceReceiptRegistry(snapshot.Registry) != nil ||
		snapshot.Registry.ContextDigest != input.Context.ContextDigest {
		return ErrMismatch
	}
	if err := evidenceapp.VerifyClaimLedgerForPublication(ctx, snapshot.Registry, input.Context, input.ClaimLedger, true); err != nil {
		return errors.Join(ErrMismatch, err)
	}
	return nil
}

func (authority *WitnessedEvidenceAuthority) ValidateCurrentSourceFieldsV2(
	ctx context.Context,
	input piiauthorizationport.EvidenceValidationV1,
) error {
	if authority == nil || authority.snapshots == nil || ctx == nil {
		return ErrMismatch
	}
	return authority.snapshots.WithWitnessedSnapshotAuthority(ctx, input.Context, func(
		snapshot registryport.WitnessedSnapshot,
		capability registryport.WitnessedSnapshotCapability,
	) error {
		return capability.UseExact(snapshot, func() error {
			if err := authority.validateSnapshotEvidenceV2(ctx, input, snapshot); err != nil {
				return err
			}
			return authority.validateSnapshotSourceFieldsV2(input, snapshot)
		})
	})
}

func (authority *WitnessedEvidenceAuthority) RenderCurrentControlledPIIArtifactV2(
	ctx context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	render piiauthorizationport.ControlledPIIArtifactRenderInputV2,
) ([]byte, error) {
	if authority == nil || authority.snapshots == nil || ctx == nil {
		return nil, ErrMismatch
	}
	var body []byte
	err := authority.snapshots.WithWitnessedSnapshotAuthority(ctx, input.Context, func(
		snapshot registryport.WitnessedSnapshot,
		capability registryport.WitnessedSnapshotCapability,
	) error {
		return capability.UseExact(snapshot, func() error {
			if err := authority.validateSnapshotEvidenceV2(ctx, input, snapshot); err != nil {
				return err
			}
			sourceFields, err := sourceFieldBindingsForControlledAccountsV2(snapshot.Registry, input)
			if err != nil {
				return errors.Join(ErrMismatch, err)
			}
			fields, err := controlledPIIFieldsFromEvidenceV1(input, sourceFields)
			if err != nil {
				return errors.Join(ErrMismatch, err)
			}
			defer clearControlledPIIFieldsV1(fields)
			body, err = renderControlledPIIArtifactBytesV2(input, render, fields)
			return err
		})
	})
	if err != nil {
		clearControlledArtifactBytesV1(body)
		return nil, errors.Join(ErrMismatch, err)
	}
	return body, nil
}

func (authority *WitnessedEvidenceAuthority) ValidateWitnessedSourceFieldsV2(
	ctx context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	snapshot registryport.WitnessedSnapshot,
	capability registryport.WitnessedSnapshotCapability,
) error {
	if authority == nil || ctx == nil || capability == nil {
		return ErrMismatch
	}
	return capability.UseExact(snapshot, func() error {
		if err := authority.validateSnapshotEvidenceV2(ctx, input, snapshot); err != nil {
			return err
		}
		return authority.validateSnapshotSourceFieldsV2(input, snapshot)
	})
}

func (authority *WitnessedEvidenceAuthority) validateSnapshotSourceFieldsV2(
	input piiauthorizationport.EvidenceValidationV1,
	snapshot registryport.WitnessedSnapshot,
) error {
	bindings, err := sourceFieldBindingsForControlledAccountsV2(snapshot.Registry, input)
	if err != nil {
		return errors.Join(ErrMismatch, err)
	}
	fields, err := controlledPIIFieldsFromEvidenceV1(input, bindings)
	if err != nil {
		return errors.Join(ErrMismatch, err)
	}
	clearControlledPIIFieldsV1(fields)
	return nil
}

func sourceFieldBindingsForControlledAccountsV2(
	registry domainevidence.EvidenceReceiptRegistry,
	input piiauthorizationport.EvidenceValidationV1,
) ([]domainevidence.SourceFieldBindingV2, error) {
	claims := make(map[string]domainevidence.ClaimRecord, len(input.ClaimLedger.Claims))
	for _, claim := range input.ClaimLedger.Claims {
		claims[claim.ClaimID] = claim
	}
	resolved := map[string]domainevidence.SourceFieldBindingV2{}
	for _, fieldBinding := range input.FieldBindings {
		if fieldBinding.PIIClass != domainpii.PIIClassFinancialAccountV1 {
			continue
		}
		claim, found := claims[fieldBinding.ClaimID]
		if !found || fieldBinding.FieldName != domainevidence.SourceFieldBindingCanonicalFieldAccount ||
			claim.NormalizedPayload.AccountID == "" {
			return nil, ErrMismatch
		}
		for _, receiptID := range fieldBinding.EvidenceReceiptIDs {
			registered, err := domainevidence.VerifyEvidenceReceiptMembership(registry, input.Context, receiptID)
			if err != nil {
				return nil, err
			}
			material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
			if err != nil || material.SchemaVersion != domainevidence.CanonicalEvidenceVersionV2 {
				return nil, ErrMismatch
			}
			matchedReceipt := false
			for _, fact := range material.Facts {
				if fact.ClaimType != claim.ClaimType ||
					!domainevidence.ClaimPayloadExactlyMatches(fact.NormalizedPayload, claim.NormalizedPayload) {
					continue
				}
				for _, sourceBinding := range material.SourceFieldBindings {
					if sourceBinding.FactID != fact.FactID ||
						sourceBinding.CanonicalFieldName != fieldBinding.FieldName ||
						sourceBinding.CanonicalAccountID != claim.NormalizedPayload.AccountID ||
						sourceBinding.SourceExactValueSHA256 != fieldBinding.ValueSHA256 {
						continue
					}
					resolved[sourceBinding.BindingDigest] = sourceBinding
					matchedReceipt = true
				}
			}
			if !matchedReceipt {
				return nil, ErrMismatch
			}
		}
	}
	out := make([]domainevidence.SourceFieldBindingV2, 0, len(resolved))
	for _, binding := range resolved {
		out = append(out, binding)
	}
	sort.Slice(out, func(left, right int) bool { return out[left].BindingDigest < out[right].BindingDigest })
	return out, nil
}
