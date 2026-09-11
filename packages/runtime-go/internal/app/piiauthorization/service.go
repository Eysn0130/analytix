package piiauthorization

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

var (
	ErrUnavailable = errors.New("PII projection authorization is unavailable")
	ErrMismatch    = errors.New("PII projection authorization binding is invalid")
	ErrExpired     = errors.New("PII projection authorization is expired")
	ErrIntegrity   = errors.New("PII projection authorization integrity failure")
)

type CurrentContextValidator func(context.Context, domainsecurity.TurnSecurityContext) error

type Config struct {
	Authority       finalauthorityport.Authority
	Store           piiauthorizationport.Store
	Ledgers         publicationport.ClaimLedgerStore
	Approval        piiauthorizationport.ApprovalAuthority
	Evidence        piiauthorizationport.EvidenceAuthority
	ValidateCurrent CurrentContextValidator
	Now             func() time.Time
}

type Service struct {
	authority       finalauthorityport.Authority
	store           piiauthorizationport.Store
	ledgers         publicationport.ClaimLedgerStore
	approval        piiauthorizationport.ApprovalAuthority
	evidence        piiauthorizationport.EvidenceAuthority
	validateCurrent CurrentContextValidator
	now             func() time.Time
}

type IssueInputV1 struct {
	SecurityContext               domainsecurity.TurnSecurityContext
	ClaimLedger                   domainpublication.ClaimLedgerV1
	FieldBindings                 []domainpii.FieldBindingV1
	ProjectionRulesetHash         string
	ProjectedContentSHA256        string
	PreservedControlledFieldCount uint64
	TargetIdentityDigest          string
	AllowedAccessActions          []string
	AccessPolicyDigest            string
	RetentionPolicyDigest         string
	RetentionUntil                time.Time
	ApprovalID                    string
	ApprovalRecordDigest          string
	ExpiresAt                     time.Time
}

type PrepareApprovalInputV1 struct {
	SecurityContext               domainsecurity.TurnSecurityContext
	ClaimLedger                   domainpublication.ClaimLedgerV1
	FieldBindings                 []domainpii.FieldBindingV1
	ProjectionRulesetHash         string
	ProjectedContentSHA256        string
	PreservedControlledFieldCount uint64
	TargetIdentityDigest          string
	AllowedAccessActions          []string
	AccessPolicyDigest            string
	RetentionPolicyDigest         string
	RetentionUntil                time.Time
	ExpiresAt                     time.Time
}

type PreparedApprovalV1 struct {
	ApprovalScopeDigest string
	PrivateArguments    []byte
	ExpiresAt           string
}

func New(config Config) (*Service, error) {
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.Authority == nil || config.Store == nil || config.Ledgers == nil || config.Approval == nil ||
		config.Evidence == nil || config.ValidateCurrent == nil || config.Now == nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(config.Authority.KeyID())) || len(config.Authority.PublicKey()) == 0 {
		return nil, ErrUnavailable
	}
	return &Service{
		authority: config.Authority, store: config.Store, ledgers: config.Ledgers, approval: config.Approval,
		evidence: config.Evidence, validateCurrent: config.ValidateCurrent, now: config.Now,
	}, nil
}

func (service *Service) Issue(ctx context.Context, input IssueInputV1) (domainpii.PIIProjectionGrantV1, error) {
	if service == nil || ctx == nil {
		return domainpii.PIIProjectionGrantV1{}, ErrUnavailable
	}
	issuedAt := service.now().UTC()
	expiresAt := input.ExpiresAt.UTC()
	if issuedAt.IsZero() || expiresAt.IsZero() || !issuedAt.Before(expiresAt) || expiresAt.Sub(issuedAt) > domainpii.PIIProjectionGrantMaxTTL {
		return domainpii.PIIProjectionGrantV1{}, ErrExpired
	}
	grantInput := domainpii.GrantInputV1{
		SecurityContext: input.SecurityContext, RequesterUserID: input.SecurityContext.UserID,
		DisclosurePurpose: domainpii.DisclosurePurposeCaseReportV1, ClaimLedgerDigest: input.ClaimLedger.LedgerDigest,
		FieldBindings: input.FieldBindings, ProjectionRulesetHash: input.ProjectionRulesetHash,
		ProjectedContentSHA256:        input.ProjectedContentSHA256,
		PreservedControlledFieldCount: input.PreservedControlledFieldCount, TargetIdentityDigest: input.TargetIdentityDigest,
		AllowedAccessActions: input.AllowedAccessActions, AccessPolicyDigest: input.AccessPolicyDigest,
		RetentionPolicyDigest: input.RetentionPolicyDigest, RetentionUntil: input.RetentionUntil,
		ApprovalID: input.ApprovalID, ApprovalRecordDigest: input.ApprovalRecordDigest,
		IssuedAt: issuedAt, ExpiresAt: expiresAt, AuthorityKeyID: service.authority.KeyID(), AuthorityPublicKey: service.authority.PublicKey(),
	}
	prepared, err := service.prepareApprovalAt(ctx, prepareApprovalInputFromIssueV1(input), issuedAt)
	if err != nil {
		return domainpii.PIIProjectionGrantV1{}, err
	}
	approval := approvalValidationV1(grantInput, prepared.ApprovalScopeDigest)
	evidence := piiauthorizationport.EvidenceValidationV1{Context: input.SecurityContext, ClaimLedger: input.ClaimLedger, FieldBindings: input.FieldBindings}
	if err := service.approval.ValidateCurrent(ctx, approval); err != nil {
		return domainpii.PIIProjectionGrantV1{}, errors.Join(ErrMismatch, err)
	}
	grant, err := domainpii.NewPIIProjectionGrantV1(grantInput, func(message []byte) ([]byte, error) {
		return service.authority.Sign(ctx, message)
	})
	if err != nil {
		return domainpii.PIIProjectionGrantV1{}, err
	}
	if err := service.ledgers.PutIfAbsent(ctx, input.ClaimLedger); err != nil {
		return domainpii.PIIProjectionGrantV1{}, err
	}
	storedLedger, err := service.ledgers.Resolve(ctx, input.ClaimLedger.LedgerDigest)
	if err != nil || !reflect.DeepEqual(storedLedger, input.ClaimLedger) {
		return domainpii.PIIProjectionGrantV1{}, errors.Join(ErrIntegrity, err)
	}
	if err := service.store.PutGrantIfAbsent(ctx, grant); err != nil {
		return domainpii.PIIProjectionGrantV1{}, err
	}
	stored, err := service.store.ResolveGrant(ctx, grant.RecordDigest)
	if err != nil || !reflect.DeepEqual(stored, grant) || service.verifyTrusted(ctx, stored) != nil {
		return domainpii.PIIProjectionGrantV1{}, errors.Join(ErrIntegrity, err)
	}
	if err := service.validateSourceBoundEvidenceCurrentV2(ctx, evidence); err != nil {
		return domainpii.PIIProjectionGrantV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.approval.ValidateCurrent(ctx, approval); err != nil {
		return domainpii.PIIProjectionGrantV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return domainpii.PIIProjectionGrantV1{}, errors.Join(ErrMismatch, err)
	}
	return stored, nil
}

// PrepareApproval validates the exact current claim/evidence scope before a
// human approval is requested. The returned private continuation arguments
// contain hashes only; raw account/card values never enter the approval event,
// ordinary persistence, SSE, or provider history.
func (service *Service) PrepareApproval(ctx context.Context, input PrepareApprovalInputV1) (PreparedApprovalV1, error) {
	if service == nil || ctx == nil {
		return PreparedApprovalV1{}, ErrUnavailable
	}
	return service.prepareApprovalAt(ctx, input, service.now().UTC())
}

func (service *Service) prepareApprovalAt(ctx context.Context, input PrepareApprovalInputV1, now time.Time) (PreparedApprovalV1, error) {
	expiresAt := input.ExpiresAt.UTC()
	if now.IsZero() || expiresAt.IsZero() || !now.Before(expiresAt) || expiresAt.Sub(now) > domainpii.PIIProjectionGrantMaxTTL {
		return PreparedApprovalV1{}, ErrExpired
	}
	if err := service.validateLedgerBindings(input.SecurityContext, input.ClaimLedger, input.FieldBindings); err != nil {
		return PreparedApprovalV1{}, err
	}
	evidence := piiauthorizationport.EvidenceValidationV1{Context: input.SecurityContext, ClaimLedger: input.ClaimLedger, FieldBindings: input.FieldBindings}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return PreparedApprovalV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.validateSourceBoundEvidenceCurrentV2(ctx, evidence); err != nil {
		return PreparedApprovalV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return PreparedApprovalV1{}, errors.Join(ErrMismatch, err)
	}
	scopeDigest, err := domainpii.ApprovalScopeDigestV1(domainpii.ApprovalScopeInputV1{
		SecurityContext: input.SecurityContext, RequesterUserID: input.SecurityContext.UserID,
		DisclosurePurpose: domainpii.DisclosurePurposeCaseReportV1, ClaimLedgerDigest: input.ClaimLedger.LedgerDigest,
		FieldBindings: input.FieldBindings, ProjectionRulesetHash: input.ProjectionRulesetHash,
		ProjectedContentSHA256: input.ProjectedContentSHA256, PreservedControlledFieldCount: input.PreservedControlledFieldCount,
		TargetIdentityDigest: input.TargetIdentityDigest, AllowedAccessActions: input.AllowedAccessActions,
		AccessPolicyDigest: input.AccessPolicyDigest, RetentionPolicyDigest: input.RetentionPolicyDigest,
		RetentionUntil: input.RetentionUntil, ExpiresAt: expiresAt,
	})
	if err != nil {
		return PreparedApprovalV1{}, errors.Join(ErrMismatch, err)
	}
	arguments, err := CanonicalControlledPIIApprovalArgumentsV1(ControlledPIIApprovalArgumentsInputV1{
		ApprovalScopeDigest: scopeDigest, TargetIdentityDigest: input.TargetIdentityDigest,
		ProjectedContentSHA256: input.ProjectedContentSHA256,
		AllowedAccessActions:   input.AllowedAccessActions, AccessPolicyDigest: input.AccessPolicyDigest,
		RetentionPolicyDigest: input.RetentionPolicyDigest, RetentionUntil: input.RetentionUntil,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return PreparedApprovalV1{}, errors.Join(ErrMismatch, err)
	}
	return PreparedApprovalV1{ApprovalScopeDigest: scopeDigest, PrivateArguments: append([]byte(nil), arguments...), ExpiresAt: expiresAt.Format(time.RFC3339Nano)}, nil
}

func prepareApprovalInputFromIssueV1(input IssueInputV1) PrepareApprovalInputV1 {
	return PrepareApprovalInputV1{
		SecurityContext: input.SecurityContext, ClaimLedger: input.ClaimLedger,
		FieldBindings: input.FieldBindings, ProjectionRulesetHash: input.ProjectionRulesetHash,
		ProjectedContentSHA256: input.ProjectedContentSHA256, PreservedControlledFieldCount: input.PreservedControlledFieldCount,
		TargetIdentityDigest: input.TargetIdentityDigest, AllowedAccessActions: append([]string(nil), input.AllowedAccessActions...),
		AccessPolicyDigest: input.AccessPolicyDigest, RetentionPolicyDigest: input.RetentionPolicyDigest,
		RetentionUntil: input.RetentionUntil, ExpiresAt: input.ExpiresAt,
	}
}

// ValidateCurrent satisfies reportpublication.PIIAuthorizationAuthority. It
// resolves the audit digest from the protected store; a nonempty string is
// never authorization. Every call revalidates context, approval, evidence,
// expiry, report bytes, ruleset, field count, and target identity.
func (service *Service) ValidateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	targetIdentityDigest string,
) error {
	return service.validateCurrentProjection(ctx, securityContext, projection, targetIdentityDigest, nil, nil, nil)
}

// ValidateControlledArtifactCurrent strengthens the generic projection check
// with the canonical artifact's raw-value-free ledger and field bindings.
func (service *Service) ValidateControlledArtifactCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	metadata domainpii.ControlledPIIArtifactMetadataV1,
) error {
	return service.validateCurrentProjection(
		ctx, securityContext, projection, metadata.TargetIdentityDigest, &metadata, nil, nil,
	)
}

// ValidateControlledArtifactCurrentWithinSnapshot performs the same exact
// binding check while reusing the final-admission evidence snapshot.
func (service *Service) ValidateControlledArtifactCurrentWithinSnapshot(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	metadata domainpii.ControlledPIIArtifactMetadataV1,
	snapshot registryport.WitnessedSnapshot,
	capability registryport.WitnessedSnapshotCapability,
) error {
	return service.validateCurrentProjection(
		ctx, securityContext, projection, metadata.TargetIdentityDigest, &metadata, &snapshot, capability,
	)
}

type witnessedPIIEvidenceValidatorV1 interface {
	ValidateWitnessed(
		context.Context,
		piiauthorizationport.EvidenceValidationV1,
		registryport.WitnessedSnapshot,
		registryport.WitnessedSnapshotCapability,
	) error
}

type witnessedPIISourceFieldEvidenceValidatorV2 interface {
	ValidateWitnessedSourceFieldsV2(
		context.Context,
		piiauthorizationport.EvidenceValidationV1,
		registryport.WitnessedSnapshot,
		registryport.WitnessedSnapshotCapability,
	) error
}

func (service *Service) validateCurrentProjection(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	targetIdentityDigest string,
	metadata *domainpii.ControlledPIIArtifactMetadataV1,
	snapshot *registryport.WitnessedSnapshot,
	capability registryport.WitnessedSnapshotCapability,
) error {
	if service == nil || ctx == nil || domainpublication.ValidatePIIProjectionV1(projection) != nil ||
		projection.ProjectionClass != domainpublication.PIIProjectionControlledFull ||
		!domainsecurity.IsSHA256Hex(projection.AuthorizationAuditDigest) {
		return ErrUnavailable
	}
	grant, err := service.store.ResolveGrant(ctx, projection.AuthorizationAuditDigest)
	if err != nil {
		return errors.Join(ErrMismatch, err)
	}
	if err := service.verifyTrusted(ctx, grant); err != nil {
		return err
	}
	if err := domainpii.ValidatePIIProjectionGrantForBindingsV1(
		grant, securityContext, grant.ClaimLedgerDigest, projection.RulesetHash, projection.ProjectedContentSHA256,
		projection.PreservedControlledFieldCount, targetIdentityDigest,
	); err != nil || projection.AuthorizationAuditDigest != grant.RecordDigest {
		return errors.Join(ErrMismatch, err)
	}
	if metadata != nil {
		if err := domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
			*metadata, securityContext, grant.ClaimLedgerDigest, projection.RulesetHash,
			targetIdentityDigest, projection.ProjectedContentSHA256, metadata.ByteLength,
			projection.PreservedControlledFieldCount,
		); err != nil || grant.ClaimLedgerDigest != metadata.ClaimLedgerDigest ||
			grant.FieldBindingSetDigest != metadata.FieldBindingSetDigest ||
			!reflect.DeepEqual(grant.FieldBindings, metadata.FieldBindings) {
			return errors.Join(ErrMismatch, err)
		}
	}
	now := service.now().UTC()
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if issuedErr != nil || expiresErr != nil || now.Before(issuedAt) || !now.Before(expiresAt) {
		return ErrExpired
	}
	if metadata != nil {
		renderedAt, renderedErr := time.Parse(time.RFC3339Nano, metadata.RenderedAt)
		if renderedErr != nil || renderedAt.After(issuedAt) {
			return ErrMismatch
		}
	}
	if err := service.validateCurrent(ctx, securityContext); err != nil {
		return errors.Join(ErrMismatch, err)
	}
	ledger, err := service.ledgers.Resolve(ctx, grant.ClaimLedgerDigest)
	if err != nil || ledger.LedgerDigest != grant.ClaimLedgerDigest {
		return errors.Join(ErrIntegrity, err)
	}
	if err := service.validateLedgerBindings(securityContext, ledger, grant.FieldBindings); err != nil {
		return err
	}
	evidence := piiauthorizationport.EvidenceValidationV1{Context: securityContext, ClaimLedger: ledger, FieldBindings: grant.FieldBindings}
	evidenceErr := service.validateSourceBoundEvidenceV2(ctx, evidence, snapshot, capability)
	if evidenceErr != nil {
		return errors.Join(ErrMismatch, evidenceErr)
	}
	if err := service.approval.ValidateCurrent(ctx, approvalValidationFromGrantV1(grant, securityContext)); err != nil {
		return errors.Join(ErrMismatch, err)
	}
	if err := service.validateCurrent(ctx, securityContext); err != nil {
		return errors.Join(ErrMismatch, err)
	}
	return nil
}

func (service *Service) validateSourceBoundEvidenceCurrentV2(
	ctx context.Context,
	input piiauthorizationport.EvidenceValidationV1,
) error {
	return service.validateSourceBoundEvidenceV2(ctx, input, nil, nil)
}

func (service *Service) validateSourceBoundEvidenceV2(
	ctx context.Context,
	input piiauthorizationport.EvidenceValidationV1,
	snapshot *registryport.WitnessedSnapshot,
	capability registryport.WitnessedSnapshotCapability,
) error {
	if !requiresSourceExactFieldsV2(input.FieldBindings) {
		if snapshot == nil {
			return service.evidence.ValidateCurrent(ctx, input)
		}
		validator, ok := service.evidence.(witnessedPIIEvidenceValidatorV1)
		if !ok {
			return ErrUnavailable
		}
		return validator.ValidateWitnessed(ctx, input, *snapshot, capability)
	}
	if snapshot == nil {
		authority, ok := service.evidence.(piiauthorizationport.ControlledPIIEvidenceAuthorityV2)
		if !ok {
			return ErrUnavailable
		}
		return authority.ValidateCurrentSourceFieldsV2(ctx, input)
	}
	validator, ok := service.evidence.(witnessedPIISourceFieldEvidenceValidatorV2)
	if !ok {
		return ErrUnavailable
	}
	return validator.ValidateWitnessedSourceFieldsV2(ctx, input, *snapshot, capability)
}

func (service *Service) verifyTrusted(ctx context.Context, grant domainpii.PIIProjectionGrantV1) error {
	keyID, publicKey, signature, err := domainpii.PIIProjectionGrantAuthorityMaterialV1(grant)
	if err != nil || service.authority.VerifyTrusted(ctx, keyID, publicKey, domainpii.PIIProjectionGrantSigningBytesV1(grant), signature) != nil {
		return errors.Join(ErrIntegrity, err)
	}
	return nil
}

func (service *Service) validateLedgerBindings(
	securityContext domainsecurity.TurnSecurityContext,
	ledger domainpublication.ClaimLedgerV1,
	bindings []domainpii.FieldBindingV1,
) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainpublication.ValidateClaimLedgerV1(ledger) != nil || !ledgerMatchesContextV1(ledger, securityContext) ||
		len(bindings) == 0 || len(bindings) > domainpii.PIIProjectionGrantMaxFieldsV1 {
		return ErrMismatch
	}
	return validateLedgerFieldBindingsV1(ledger, bindings)
}

func validateLedgerFieldBindingsV1(
	ledger domainpublication.ClaimLedgerV1,
	bindings []domainpii.FieldBindingV1,
) error {
	claims := make(map[string]domainevidence.ClaimRecord, len(ledger.Claims))
	ledgerEvidence := make(map[string]bool, len(ledger.EvidenceReceiptIDs))
	for _, receiptID := range ledger.EvidenceReceiptIDs {
		ledgerEvidence[receiptID] = true
	}
	for _, claim := range ledger.Claims {
		claims[claim.ClaimID] = claim
	}
	seen := map[string]bool{}
	for _, binding := range bindings {
		claim, found := claims[binding.ClaimID]
		if !found || domainevidence.ValidateClaimRecord(claim) != nil || claim.SupportState != domainevidence.ClaimVerified ||
			claim.RequiresHumanReview || claim.RecordDigest != binding.ClaimRecordDigest || claim.ClaimType != binding.ClaimType {
			return ErrMismatch
		}
		valueMatches := false
		if binding.PIIClass == domainpii.PIIClassFinancialAccountV1 {
			valueMatches = binding.FieldName == domainevidence.SourceFieldBindingCanonicalFieldAccount &&
				claim.NormalizedPayload.AccountID != ""
		} else {
			value, ok := claimFieldValueV1(claim, binding)
			valueMatches = ok && domainsecurity.SHA256Hex([]byte(value)) == binding.ValueSHA256
		}
		if !valueMatches || !sameEvidenceIDsV1(binding.EvidenceReceiptIDs, claim.EvidenceIDs) {
			return ErrMismatch
		}
		for _, receiptID := range binding.EvidenceReceiptIDs {
			if !ledgerEvidence[receiptID] {
				return ErrMismatch
			}
		}
		key := binding.PIIClass + "\x00" + binding.ClaimID + "\x00" + binding.FieldName + "\x00" + binding.ValueSHA256
		if seen[key] {
			return ErrMismatch
		}
		seen[key] = true
	}
	return nil
}

func ledgerMatchesContextV1(ledger domainpublication.ClaimLedgerV1, context domainsecurity.TurnSecurityContext) bool {
	return ledger.ThreadID == context.ThreadID && ledger.TurnID == context.TurnID && ledger.ContextDigest == context.ContextDigest &&
		ledger.CaseID == context.CaseID && ledger.CaseBindingHash == context.CaseBindingHash && ledger.ContextEpoch == context.ContextEpoch &&
		ledger.DatasetSnapshotID == context.DatasetSnapshotID && ledger.SourceManifestHash == context.SourceManifestHash
}

func claimFieldValueV1(claim domainevidence.ClaimRecord, binding domainpii.FieldBindingV1) (string, bool) {
	switch binding.PIIClass {
	case domainpii.PIIClassFinancialAccountV1:
		return claim.NormalizedPayload.AccountID, binding.FieldName == "accountId" && claim.NormalizedPayload.AccountID != ""
	case domainpii.PIIClassDeviceIdentifierV1:
		return claim.NormalizedPayload.DeviceIdentifier, binding.FieldName == "deviceIdentifier" && claim.NormalizedPayload.DeviceIdentifier != ""
	case domainpii.PIIClassAddressV1:
		return claim.NormalizedPayload.AttributeValue, binding.FieldName == "attributeValue" && claim.ClaimType == domainevidence.ClaimAddress && claim.NormalizedPayload.AttributeValue != ""
	case domainpii.PIIClassPersonNameV1:
		attribute := strings.ToLower(strings.TrimSpace(claim.NormalizedPayload.AttributeName))
		return claim.NormalizedPayload.AttributeValue, binding.FieldName == "attributeValue" && claim.ClaimType == domainevidence.ClaimBidEditMetadata &&
			(attribute == "document_author" || attribute == "editor") && claim.NormalizedPayload.AttributeValue != ""
	default:
		return "", false
	}
}

func sameEvidenceIDsV1(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func approvalValidationV1(input domainpii.GrantInputV1, scopeDigest string) piiauthorizationport.ApprovalValidationV1 {
	accessActions, _ := canonicalControlledPIIAccessActionsV1(input.AllowedAccessActions)
	return piiauthorizationport.ApprovalValidationV1{
		Context: input.SecurityContext, ApprovalID: strings.TrimSpace(input.ApprovalID), ApprovalRecordDigest: strings.TrimSpace(input.ApprovalRecordDigest),
		ApprovalScopeDigest: scopeDigest, RequesterUserID: strings.TrimSpace(input.RequesterUserID),
		DisclosurePurpose: strings.TrimSpace(input.DisclosurePurpose), TargetIdentityDigest: strings.TrimSpace(input.TargetIdentityDigest),
		ProjectedContentHash:  strings.TrimSpace(input.ProjectedContentSHA256),
		AllowedAccessActions:  accessActions,
		AccessPolicyDigest:    strings.TrimSpace(input.AccessPolicyDigest),
		RetentionPolicyDigest: strings.TrimSpace(input.RetentionPolicyDigest),
		RetentionUntil:        input.RetentionUntil.UTC().Format(time.RFC3339Nano),
		ExpiresAt:             input.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
}

func approvalValidationFromGrantV1(grant domainpii.PIIProjectionGrantV1, securityContext domainsecurity.TurnSecurityContext) piiauthorizationport.ApprovalValidationV1 {
	return piiauthorizationport.ApprovalValidationV1{
		Context:    securityContext,
		ApprovalID: grant.ApprovalID, ApprovalRecordDigest: grant.ApprovalRecordDigest, ApprovalScopeDigest: grant.ApprovalScopeDigest,
		RequesterUserID: grant.RequesterUserID, DisclosurePurpose: grant.DisclosurePurpose, TargetIdentityDigest: grant.TargetIdentityDigest,
		ProjectedContentHash: grant.ProjectedContentSHA256, AllowedAccessActions: append([]string(nil), grant.AllowedAccessActions...),
		AccessPolicyDigest: grant.AccessPolicyDigest, RetentionPolicyDigest: grant.RetentionPolicyDigest,
		RetentionUntil: grant.RetentionUntil, ExpiresAt: grant.ExpiresAt,
	}
}
