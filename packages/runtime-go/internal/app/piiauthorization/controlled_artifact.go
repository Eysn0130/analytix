package piiauthorization

import (
	"context"
	"errors"
	"reflect"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

// PrepareControlledArtifactInputV1 deliberately has no raw-value field. The
// host renderer can only copy exact controlled values from verified claims in
// the supplied ledger after current context and evidence validation.
type PrepareControlledArtifactInputV1 struct {
	SecurityContext       domainsecurity.TurnSecurityContext
	ClaimLedger           domainpublication.ClaimLedgerV1
	FieldBindings         []domainpii.FieldBindingV1
	ProjectionRulesetHash string
	TargetIdentityDigest  string
	AllowedAccessActions  []string
	AccessPolicyDigest    string
	RetentionPolicyDigest string
	RetentionUntil        time.Time
	ExpiresAt             time.Time
}

// PreparedControlledArtifactV1 is a hash-only approval intent. It carries no
// exact PII value or artifact body. Authorization deterministically rerenders
// from the then-current verified claim ledger and requires an exact metadata,
// hash, and length match before a terminal lease can be created.
type PreparedControlledArtifactV1 struct {
	Metadata            domainpii.ControlledPIIArtifactMetadataV1
	ProtectedSHA256     string
	ProtectedByteLength uint64
	Approval            PreparedApprovalV1
}

type AuthorizePreparedControlledArtifactInputV1 struct {
	SecurityContext      domainsecurity.TurnSecurityContext
	ClaimLedger          domainpublication.ClaimLedgerV1
	Prepared             PreparedControlledArtifactV1
	ApprovalID           string
	ApprovalRecordDigest string
}

type AuthorizedControlledArtifactV1 struct {
	Metadata            domainpii.ControlledPIIArtifactMetadataV1
	ProtectedSHA256     string
	ProtectedByteLength uint64
	Grant               domainpii.PIIProjectionGrantV1
	PIIProjection       domainpublication.PIIProjectionV1
	Lease               *ControlledPIITerminalLeaseV1 `json:"-"`
}

// PrepareControlledArtifact creates the exact byte candidate whose digest is
// presented for human approval. It validates current context/evidence both
// before reading controlled claim values and again after deterministic render,
// closing the model/caller injection and stale-epoch gaps.
func (service *Service) PrepareControlledArtifact(
	ctx context.Context,
	input PrepareControlledArtifactInputV1,
) (PreparedControlledArtifactV1, error) {
	if service == nil || ctx == nil {
		return PreparedControlledArtifactV1{}, ErrUnavailable
	}
	if err := service.validateLedgerBindings(input.SecurityContext, input.ClaimLedger, input.FieldBindings); err != nil {
		return PreparedControlledArtifactV1{}, err
	}
	evidence := piiauthorizationport.EvidenceValidationV1{
		Context: input.SecurityContext, ClaimLedger: input.ClaimLedger, FieldBindings: input.FieldBindings,
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return PreparedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.evidence.ValidateCurrent(ctx, evidence); err != nil {
		return PreparedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return PreparedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}

	renderedAt := service.now().UTC()
	if renderedAt.IsZero() {
		return PreparedControlledArtifactV1{}, ErrUnavailable
	}
	body, err := service.renderControlledPIIArtifactBytesFromCurrentAuthorityV2(ctx, evidence, controlledPIIArtifactRenderInputV2{
		ProjectionRulesetHash: input.ProjectionRulesetHash, TargetIdentityDigest: input.TargetIdentityDigest,
		RenderedAt: renderedAt,
	})
	if err != nil {
		return PreparedControlledArtifactV1{}, err
	}
	defer clearControlledArtifactBytesV1(body)
	contentHash := domainsecurity.SHA256Hex(body)
	metadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
	if err != nil {
		return PreparedControlledArtifactV1{}, errors.Join(ErrIntegrity, err)
	}
	approval, err := service.prepareApprovalAt(ctx, PrepareApprovalInputV1{
		SecurityContext: input.SecurityContext, ClaimLedger: input.ClaimLedger, FieldBindings: metadata.FieldBindings,
		ProjectionRulesetHash: metadata.ProjectionRulesetHash, ProjectedContentSHA256: contentHash,
		PreservedControlledFieldCount: metadata.PreservedControlledFieldCount,
		TargetIdentityDigest:          metadata.TargetIdentityDigest, AllowedAccessActions: input.AllowedAccessActions,
		AccessPolicyDigest: input.AccessPolicyDigest, RetentionPolicyDigest: input.RetentionPolicyDigest,
		RetentionUntil: input.RetentionUntil, ExpiresAt: input.ExpiresAt,
	}, service.now().UTC())
	if err != nil {
		return PreparedControlledArtifactV1{}, err
	}
	return PreparedControlledArtifactV1{
		Metadata: metadata, ProtectedSHA256: contentHash, ProtectedByteLength: uint64(len(body)), Approval: approval,
	}, nil
}

// AuthorizePreparedControlledArtifact rerenders from current verified claims;
// it never trusts or accepts staged raw bytes. Only an exact match to the
// hash-only approval intent can enter the single-consumer terminal lease.
func (service *Service) AuthorizePreparedControlledArtifact(
	ctx context.Context,
	input AuthorizePreparedControlledArtifactInputV1,
) (AuthorizedControlledArtifactV1, error) {
	if service == nil || ctx == nil {
		return AuthorizedControlledArtifactV1{}, ErrUnavailable
	}
	prepared := input.Prepared
	if domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
		prepared.Metadata, input.SecurityContext, input.ClaimLedger.LedgerDigest,
		prepared.Metadata.ProjectionRulesetHash, prepared.Metadata.TargetIdentityDigest,
		prepared.ProtectedSHA256, prepared.ProtectedByteLength, prepared.Metadata.PreservedControlledFieldCount,
	) != nil || prepared.Metadata.ClaimLedgerDigest != input.ClaimLedger.LedgerDigest ||
		prepared.ProtectedSHA256 != prepared.Metadata.SHA256 || prepared.ProtectedByteLength != prepared.Metadata.ByteLength {
		return AuthorizedControlledArtifactV1{}, ErrMismatch
	}
	if err := service.validateLedgerBindings(input.SecurityContext, input.ClaimLedger, prepared.Metadata.FieldBindings); err != nil {
		return AuthorizedControlledArtifactV1{}, err
	}
	evidence := piiauthorizationport.EvidenceValidationV1{
		Context: input.SecurityContext, ClaimLedger: input.ClaimLedger, FieldBindings: prepared.Metadata.FieldBindings,
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.evidence.ValidateCurrent(ctx, evidence); err != nil {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	renderedAt, err := time.Parse(time.RFC3339Nano, prepared.Metadata.RenderedAt)
	if err != nil {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	body, err := service.renderControlledPIIArtifactBytesFromCurrentAuthorityV2(ctx, evidence, controlledPIIArtifactRenderInputV2{
		ProjectionRulesetHash: prepared.Metadata.ProjectionRulesetHash,
		TargetIdentityDigest:  prepared.Metadata.TargetIdentityDigest, RenderedAt: renderedAt,
	})
	if err != nil {
		return AuthorizedControlledArtifactV1{}, err
	}
	defer func() { clearControlledArtifactBytesV1(body) }()
	metadata, err := domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
	if err != nil || !reflect.DeepEqual(metadata, prepared.Metadata) ||
		domainsecurity.SHA256Hex(body) != prepared.ProtectedSHA256 || uint64(len(body)) != prepared.ProtectedByteLength {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrIntegrity, err)
	}
	arguments, err := ParseControlledPIIApprovalArgumentsV1(prepared.Approval.PrivateArguments)
	if err != nil || arguments.ApprovalScopeDigest != prepared.Approval.ApprovalScopeDigest ||
		arguments.ProjectedContentSHA256 != prepared.ProtectedSHA256 ||
		arguments.TargetIdentityDigest != prepared.Metadata.TargetIdentityDigest ||
		arguments.ExpiresAt != prepared.Approval.ExpiresAt {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, arguments.ExpiresAt)
	if err != nil {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	retentionUntil, err := time.Parse(time.RFC3339Nano, arguments.RetentionUntil)
	if err != nil {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrMismatch, err)
	}
	grant, err := service.Issue(ctx, IssueInputV1{
		SecurityContext: input.SecurityContext, ClaimLedger: input.ClaimLedger, FieldBindings: prepared.Metadata.FieldBindings,
		ProjectionRulesetHash:         prepared.Metadata.ProjectionRulesetHash,
		ProjectedContentSHA256:        prepared.ProtectedSHA256,
		PreservedControlledFieldCount: prepared.Metadata.PreservedControlledFieldCount,
		TargetIdentityDigest:          prepared.Metadata.TargetIdentityDigest,
		AllowedAccessActions:          arguments.AllowedAccessActions,
		AccessPolicyDigest:            arguments.AccessPolicyDigest,
		RetentionPolicyDigest:         arguments.RetentionPolicyDigest,
		RetentionUntil:                retentionUntil,
		ApprovalID:                    input.ApprovalID, ApprovalRecordDigest: input.ApprovalRecordDigest, ExpiresAt: expiresAt,
	})
	if err != nil {
		return AuthorizedControlledArtifactV1{}, err
	}
	if grant.ApprovalScopeDigest != prepared.Approval.ApprovalScopeDigest ||
		grant.ProjectedContentSHA256 != prepared.ProtectedSHA256 {
		return AuthorizedControlledArtifactV1{}, ErrIntegrity
	}
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{
		ProjectionClass: domainpublication.PIIProjectionControlledFull,
		RulesetHash:     prepared.Metadata.ProjectionRulesetHash, ProjectedContentSHA256: prepared.ProtectedSHA256,
		RestrictedFieldCount:          prepared.Metadata.PreservedControlledFieldCount,
		PreservedControlledFieldCount: prepared.Metadata.PreservedControlledFieldCount,
		AuthorizationAuditDigest:      grant.RecordDigest,
	})
	if err != nil {
		return AuthorizedControlledArtifactV1{}, errors.Join(ErrIntegrity, err)
	}
	if err := service.ValidateControlledArtifactCurrent(ctx, input.SecurityContext, projection, metadata); err != nil {
		return AuthorizedControlledArtifactV1{}, err
	}
	lease, err := newControlledPIITerminalLeaseV1(ControlledPIITerminalLeaseMetadataV1{
		ContextDigest: input.SecurityContext.ContextDigest, ContextEpoch: input.SecurityContext.ContextEpoch,
		DatasetSnapshotID: input.SecurityContext.DatasetSnapshotID, ClaimLedgerDigest: metadata.ClaimLedgerDigest,
		TargetIdentityDigest: metadata.TargetIdentityDigest, ArtifactSHA256: metadata.SHA256,
		ArtifactByteLength: metadata.ByteLength, MediaType: metadata.MediaType,
		PIIAuthorizationDigest: grant.RecordDigest, PIIProjectionDigest: projection.ProjectionDigest,
		ArtifactMetadata: metadata,
	}, body)
	if err != nil {
		return AuthorizedControlledArtifactV1{}, err
	}
	body = nil
	return AuthorizedControlledArtifactV1{
		Metadata: metadata, ProtectedSHA256: prepared.ProtectedSHA256, ProtectedByteLength: prepared.ProtectedByteLength,
		Grant: grant, PIIProjection: projection, Lease: lease,
	}, nil
}

type controlledPIIArtifactRenderInputV2 struct {
	ProjectionRulesetHash string
	TargetIdentityDigest  string
	RenderedAt            time.Time
}

// renderControlledPIIArtifactBytesFromCurrentAuthorityV2 is the only bridge
// from source-exact fields to protected artifact bytes. The exact field slice
// is rendered while the witnessed registry capability is active and is never
// returned to the caller or retained as an ordinary application value.
func (service *Service) renderControlledPIIArtifactBytesFromCurrentAuthorityV2(
	ctx context.Context,
	validation piiauthorizationport.EvidenceValidationV1,
	input controlledPIIArtifactRenderInputV2,
) ([]byte, error) {
	render := piiauthorizationport.ControlledPIIArtifactRenderInputV2{
		ProjectionRulesetHash: input.ProjectionRulesetHash,
		TargetIdentityDigest:  input.TargetIdentityDigest,
		RenderedAt:            input.RenderedAt,
	}
	authority, ok := service.evidence.(piiauthorizationport.ControlledPIIEvidenceAuthorityV2)
	if !ok {
		return nil, ErrMismatch
	}
	return authority.RenderCurrentControlledPIIArtifactV2(ctx, validation, render)
}
