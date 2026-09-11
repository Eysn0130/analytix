package piiauthorization

import (
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

// This file is the private exact-value renderer. Exact source and controlled
// field structs stay behind the operation-only PII authority port; ordinary
// preparation, approval, report, and publication services receive only the
// canonical protected artifact bytes or raw-value-free metadata.

func renderControlledPIIArtifactBytesV2(
	validation piiauthorizationport.EvidenceValidationV1,
	render piiauthorizationport.ControlledPIIArtifactRenderInputV2,
	fields []domainpii.ControlledPIIFieldV1,
) ([]byte, error) {
	artifact, err := domainpii.NewControlledPIIArtifactV1(domainpii.ControlledPIIArtifactInputV1{
		SecurityContext: validation.Context, ClaimLedgerDigest: validation.ClaimLedger.LedgerDigest,
		ProjectionRulesetHash: render.ProjectionRulesetHash, TargetIdentityDigest: render.TargetIdentityDigest,
		Fields: fields, RenderedAt: render.RenderedAt,
	})
	if err != nil {
		return nil, errors.Join(ErrMismatch, err)
	}
	body, err := domainpii.ControlledPIIArtifactV1Bytes(artifact)
	if err != nil {
		return nil, errors.Join(ErrIntegrity, err)
	}
	return body, nil
}

func requiresSourceExactFieldsV2(bindings []domainpii.FieldBindingV1) bool {
	for _, binding := range bindings {
		if binding.PIIClass == domainpii.PIIClassFinancialAccountV1 {
			return true
		}
	}
	return false
}

func clearControlledPIIFieldsV1(fields []domainpii.ControlledPIIFieldV1) {
	for index := range fields {
		fields[index].ExactValue = ""
		clear(fields[index].EvidenceReceiptIDs)
		fields[index].EvidenceReceiptIDs = nil
	}
	clear(fields)
}

func controlledPIIFieldsFromEvidenceV1(
	validation piiauthorizationport.EvidenceValidationV1,
	sourceFields []domainevidence.SourceFieldBindingV2,
) ([]domainpii.ControlledPIIFieldV1, error) {
	claims := make(map[string]domainevidence.ClaimRecord, len(validation.ClaimLedger.Claims))
	for _, claim := range validation.ClaimLedger.Claims {
		claims[claim.ClaimID] = claim
	}
	fields := make([]domainpii.ControlledPIIFieldV1, 0, len(validation.FieldBindings))
	for _, binding := range validation.FieldBindings {
		claim, ok := claims[binding.ClaimID]
		if !ok {
			return nil, ErrMismatch
		}
		value, ok := controlledFieldExactValueV2(claim, binding, sourceFields)
		if !ok || domainsecurity.SHA256Hex([]byte(value)) != binding.ValueSHA256 {
			return nil, ErrMismatch
		}
		fields = append(fields, domainpii.ControlledPIIFieldV1{
			PIIClass: binding.PIIClass, ClaimID: binding.ClaimID, ClaimRecordDigest: binding.ClaimRecordDigest,
			ClaimType: binding.ClaimType, FieldName: binding.FieldName, ExactValue: value, ValueSHA256: binding.ValueSHA256,
			EvidenceReceiptIDs: append([]string(nil), binding.EvidenceReceiptIDs...),
		})
	}
	return fields, nil
}

func controlledFieldExactValueV2(
	claim domainevidence.ClaimRecord,
	binding domainpii.FieldBindingV1,
	sourceFields []domainevidence.SourceFieldBindingV2,
) (string, bool) {
	if binding.PIIClass != domainpii.PIIClassFinancialAccountV1 {
		return claimFieldValueV1(claim, binding)
	}
	value := ""
	found := false
	for _, sourceField := range sourceFields {
		if sourceField.CanonicalFieldName != binding.FieldName ||
			sourceField.ClaimType != claim.ClaimType ||
			sourceField.CanonicalAccountID != claim.NormalizedPayload.AccountID ||
			sourceField.SourceExactValueSHA256 != binding.ValueSHA256 {
			continue
		}
		if found && value != sourceField.SourceExactValue {
			return "", false
		}
		value = sourceField.SourceExactValue
		found = true
	}
	return value, found
}
