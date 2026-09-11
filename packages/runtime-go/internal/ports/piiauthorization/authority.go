package piiauthorization

import (
	"context"
	"errors"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrNotFound        = errors.New("PII authorization record is not found")
	ErrConflict        = errors.New("PII authorization record conflicts")
	ErrCorrupt         = errors.New("PII authorization record is corrupt")
	ErrAlreadyReserved = errors.New("PII controlled access is already reserved")
	ErrIndeterminate   = errors.New("PII controlled access commit is indeterminate")
)

type GrantResolver interface {
	ResolveGrant(context.Context, string) (domainpii.PIIProjectionGrantV1, error)
}

type Store interface {
	GrantResolver
	PutGrantIfAbsent(context.Context, domainpii.PIIProjectionGrantV1) error
}

// InventoryStore is used only by startup trust verification. Structural CAS
// parsing or a self-contained signature is not installation trust.
type InventoryStore interface {
	VisitGrants(context.Context, func(domainpii.PIIProjectionGrantV1) error) error
}

type ApprovalValidationV1 struct {
	Context               domainsecurity.TurnSecurityContext
	ApprovalID            string
	ApprovalRecordDigest  string
	ApprovalScopeDigest   string
	RequesterUserID       string
	DisclosurePurpose     string
	TargetIdentityDigest  string
	ProjectedContentHash  string
	AllowedAccessActions  []string
	AccessPolicyDigest    string
	RetentionPolicyDigest string
	RetentionUntil        string
	ExpiresAt             string
}

// ApprovalAuthority validates a host-owned, already resolved human approval.
// A provider-supplied approval id or decision is never sufficient.
type ApprovalAuthority interface {
	ValidateCurrent(context.Context, ApprovalValidationV1) error
}

type EvidenceValidationV1 struct {
	Context       domainsecurity.TurnSecurityContext
	ClaimLedger   domainpublication.ClaimLedgerV1
	FieldBindings []domainpii.FieldBindingV1
}

// EvidenceAuthority rechecks current same-context registry membership and
// exact claim support for every controlled field.
type EvidenceAuthority interface {
	ValidateCurrent(context.Context, EvidenceValidationV1) error
}

type ControlledPIIArtifactRenderInputV2 struct {
	ProjectionRulesetHash string
	TargetIdentityDigest  string
	RenderedAt            time.Time
}

// ControlledPIIEvidenceAuthorityV2 exposes only bounded operations over
// source-exact PII. Exact source or controlled-field structs never cross this
// port; the only value-bearing output is a canonical protected artifact body.
// V1 evidence and claim-normalized values cannot implement this authority.
type ControlledPIIEvidenceAuthorityV2 interface {
	EvidenceAuthority
	ValidateCurrentSourceFieldsV2(context.Context, EvidenceValidationV1) error
	RenderCurrentControlledPIIArtifactV2(context.Context, EvidenceValidationV1, ControlledPIIArtifactRenderInputV2) ([]byte, error)
}
