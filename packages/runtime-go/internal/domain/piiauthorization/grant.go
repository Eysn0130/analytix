package piiauthorization

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PIIProjectionGrantSchemaVersionV1    = 1
	PIIProjectionGrantPurposeV1          = "analytix.pii-projection-grant/v1"
	PIIProjectionGrantAlgorithmV1        = "Ed25519"
	PIIProjectionGrantMaxTTL             = 15 * time.Minute
	PIIProjectionGrantMaxFieldsV1        = 256
	PIIProjectionGrantMaxAccessActionsV1 = 2

	DisclosurePurposeCaseReportV1     = "case_report"
	DeliveryScopeControlledArtifactV1 = "controlled_artifact"

	PIIClassFinancialAccountV1 = "financial_account_identifier"
	PIIClassDeviceIdentifierV1 = "device_identifier"
	PIIClassAddressV1          = "address"
	PIIClassPersonNameV1       = "person_name"
)

var (
	piiProjectionGrantSigningDomainV1 = []byte("analytix.pii-projection-grant/signature/v1\x00")
	piiProjectionGrantDigestDomainV1  = []byte("analytix.pii-projection-grant/record/v1\x00")
	piiProjectionGrantIDDomainV1      = []byte("analytix.pii-projection-grant/id/v1\x00")
	piiApprovalScopeDomainV1          = []byte("analytix.pii-projection-approval-scope/v1\x00")
	piiApprovalIDPattern              = regexp.MustCompile(`^appr_[A-Za-z0-9_-]{8,160}$`)
)

type ContextBindingV1 struct {
	Version            int    `json:"version"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	WorkspaceRealPath  string `json:"workspaceRealPath"`
	TenantID           string `json:"tenantId"`
	UserID             string `json:"userId"`
	CaseID             string `json:"caseId"`
	CaseBindingHash    string `json:"caseBindingHash"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	ContextIssuedAt    string `json:"contextIssuedAt"`
	ContextDigest      string `json:"contextDigest"`
}

// FieldBindingV1 never carries the raw PII value. ValueSHA256 binds the exact
// field copied by the deterministic host renderer to one verified claim and
// its concrete evidence receipts.
type FieldBindingV1 struct {
	PIIClass           string                   `json:"piiClass"`
	ClaimID            string                   `json:"claimId"`
	ClaimRecordDigest  string                   `json:"claimRecordDigest"`
	ClaimType          domainevidence.ClaimType `json:"claimType"`
	FieldName          string                   `json:"fieldName"`
	ValueSHA256        string                   `json:"valueSha256"`
	EvidenceReceiptIDs []string                 `json:"evidenceReceiptIds"`
}

type PIIProjectionGrantV1 struct {
	SchemaVersion                 int              `json:"schemaVersion"`
	Purpose                       string           `json:"purpose"`
	GrantID                       string           `json:"grantId"`
	Context                       ContextBindingV1 `json:"context"`
	RequesterUserID               string           `json:"requesterUserId"`
	DisclosurePurpose             string           `json:"disclosurePurpose"`
	DeliveryScope                 string           `json:"deliveryScope"`
	ClaimLedgerDigest             string           `json:"claimLedgerDigest"`
	FieldBindings                 []FieldBindingV1 `json:"fieldBindings"`
	FieldBindingSetDigest         string           `json:"fieldBindingSetDigest"`
	EvidenceReceiptIDs            []string         `json:"evidenceReceiptIds"`
	EvidenceReceiptSetDigest      string           `json:"evidenceReceiptSetDigest"`
	ProjectionRulesetHash         string           `json:"projectionRulesetHash"`
	ProjectedContentSHA256        string           `json:"projectedContentSha256"`
	PreservedControlledFieldCount uint64           `json:"preservedControlledFieldCount"`
	TargetIdentityDigest          string           `json:"targetIdentityDigest"`
	AllowedAccessActions          []string         `json:"allowedAccessActions"`
	AccessActionSetDigest         string           `json:"accessActionSetDigest"`
	AccessPolicyDigest            string           `json:"accessPolicyDigest"`
	RetentionPolicyDigest         string           `json:"retentionPolicyDigest"`
	RetentionUntil                string           `json:"retentionUntil"`
	ApprovalID                    string           `json:"approvalId"`
	ApprovalRecordDigest          string           `json:"approvalRecordDigest"`
	ApprovalScopeDigest           string           `json:"approvalScopeDigest"`
	IssuedAt                      string           `json:"issuedAt"`
	ExpiresAt                     string           `json:"expiresAt"`
	AuthorityAlgorithm            string           `json:"authorityAlgorithm"`
	AuthorityKeyID                string           `json:"authorityKeyId"`
	AuthorityPublicKey            string           `json:"authorityPublicKey"`
	AuthoritySignature            string           `json:"authoritySignature"`
	RecordDigest                  string           `json:"recordDigest"`
}

type GrantInputV1 struct {
	SecurityContext               domainsecurity.TurnSecurityContext
	RequesterUserID               string
	DisclosurePurpose             string
	ClaimLedgerDigest             string
	FieldBindings                 []FieldBindingV1
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
	IssuedAt                      time.Time
	ExpiresAt                     time.Time
	AuthorityKeyID                string
	AuthorityPublicKey            []byte
}

// ApprovalScopeInputV1 is the complete semantic disclosure scope presented
// for human approval before a continuation gate id or disposition exists. It
// deliberately excludes those later record identities so approval request
// construction cannot become circular. The signed grant binds both the scope
// digest and the resulting approval records.
type ApprovalScopeInputV1 struct {
	SecurityContext               domainsecurity.TurnSecurityContext
	RequesterUserID               string
	DisclosurePurpose             string
	ClaimLedgerDigest             string
	FieldBindings                 []FieldBindingV1
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

type SignFuncV1 func([]byte) ([]byte, error)

func NewPIIProjectionGrantV1(input GrantInputV1, sign SignFuncV1) (PIIProjectionGrantV1, error) {
	contextBinding, err := contextBindingFromSecurityContextV1(input.SecurityContext)
	if err != nil {
		return PIIProjectionGrantV1{}, err
	}
	bindings, err := canonicalFieldBindingsV1(input.FieldBindings)
	if err != nil {
		return PIIProjectionGrantV1{}, err
	}
	evidenceIDs := fieldBindingEvidenceIDsV1(bindings)
	accessActions, err := canonicalPIIAccessActionsV1(input.AllowedAccessActions)
	if err != nil {
		return PIIProjectionGrantV1{}, err
	}
	issuedAt := input.IssuedAt.UTC()
	expiresAt := input.ExpiresAt.UTC()
	retentionUntil := input.RetentionUntil.UTC()
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	grant := PIIProjectionGrantV1{
		SchemaVersion: PIIProjectionGrantSchemaVersionV1, Purpose: PIIProjectionGrantPurposeV1,
		Context: contextBinding, RequesterUserID: strings.TrimSpace(input.RequesterUserID),
		DisclosurePurpose: strings.TrimSpace(input.DisclosurePurpose), DeliveryScope: DeliveryScopeControlledArtifactV1,
		ClaimLedgerDigest: strings.TrimSpace(input.ClaimLedgerDigest), FieldBindings: bindings,
		FieldBindingSetDigest: fieldBindingSetDigestV1(bindings), EvidenceReceiptIDs: evidenceIDs,
		EvidenceReceiptSetDigest:      evidenceReceiptSetDigestV1(evidenceIDs),
		ProjectionRulesetHash:         strings.TrimSpace(input.ProjectionRulesetHash),
		ProjectedContentSHA256:        strings.TrimSpace(input.ProjectedContentSHA256),
		PreservedControlledFieldCount: input.PreservedControlledFieldCount,
		TargetIdentityDigest:          strings.TrimSpace(input.TargetIdentityDigest),
		AllowedAccessActions:          accessActions,
		AccessActionSetDigest:         piiAccessActionSetDigestV1(accessActions),
		AccessPolicyDigest:            strings.TrimSpace(input.AccessPolicyDigest),
		RetentionPolicyDigest:         strings.TrimSpace(input.RetentionPolicyDigest),
		RetentionUntil:                retentionUntil.Format(time.RFC3339Nano),
		ApprovalID:                    strings.TrimSpace(input.ApprovalID), ApprovalRecordDigest: strings.TrimSpace(input.ApprovalRecordDigest),
		IssuedAt: issuedAt.Format(time.RFC3339Nano), ExpiresAt: expiresAt.Format(time.RFC3339Nano),
		AuthorityAlgorithm: PIIProjectionGrantAlgorithmV1, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	grant.ApprovalScopeDigest = approvalScopeDigestV1(grant)
	grant.GrantID = piiProjectionGrantIDV1(grant)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || grant.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return PIIProjectionGrantV1{}, errors.New("PII projection grant signing authority is invalid")
	}
	if err := validatePIIProjectionGrantUnsignedV1(grant); err != nil {
		return PIIProjectionGrantV1{}, err
	}
	signature, err := sign(PIIProjectionGrantSigningBytesV1(grant))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PIIProjectionGrantV1{}, errors.New("PII projection grant signing failed")
	}
	grant.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	grant.RecordDigest = piiProjectionGrantRecordDigestV1(grant)
	if err := ValidatePIIProjectionGrantV1(grant); err != nil {
		return PIIProjectionGrantV1{}, err
	}
	return grant, nil
}

func ValidatePIIProjectionGrantV1(grant PIIProjectionGrantV1) error {
	if err := validatePIIProjectionGrantUnsignedV1(grant); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(grant.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(grant.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != grant.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != grant.AuthoritySignature ||
		grant.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PIIProjectionGrantSigningBytesV1(grant), signature) {
		return errors.New("PII projection grant signature is invalid")
	}
	if !domainsecurity.IsSHA256Hex(grant.RecordDigest) || grant.RecordDigest != piiProjectionGrantRecordDigestV1(grant) {
		return errors.New("PII projection grant record digest is invalid")
	}
	return nil
}

func ValidatePIIProjectionGrantForBindingsV1(
	grant PIIProjectionGrantV1,
	securityContext domainsecurity.TurnSecurityContext,
	claimLedgerDigest string,
	projectionRulesetHash string,
	projectedContentSHA256 string,
	preservedControlledFieldCount uint64,
	targetIdentityDigest string,
) error {
	if ValidatePIIProjectionGrantV1(grant) != nil {
		return errors.New("PII projection grant is invalid")
	}
	contextBinding, err := contextBindingFromSecurityContextV1(securityContext)
	if err != nil || grant.Context != contextBinding || grant.RequesterUserID != securityContext.UserID ||
		grant.ClaimLedgerDigest != strings.TrimSpace(claimLedgerDigest) ||
		grant.ProjectionRulesetHash != strings.TrimSpace(projectionRulesetHash) ||
		grant.ProjectedContentSHA256 != strings.TrimSpace(projectedContentSHA256) ||
		grant.PreservedControlledFieldCount != preservedControlledFieldCount ||
		grant.TargetIdentityDigest != strings.TrimSpace(targetIdentityDigest) {
		return errors.New("PII projection grant context, content, or target binding is invalid")
	}
	return nil
}

func ParsePIIProjectionGrantV1(body []byte) (PIIProjectionGrantV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 2 << 20, MaxDepth: 12, MaxTokens: 100_000, MaxStringBytes: 256 << 10,
	}); err != nil {
		return PIIProjectionGrantV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var grant PIIProjectionGrantV1
	if err := decoder.Decode(&grant); err != nil {
		return PIIProjectionGrantV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PIIProjectionGrantV1{}, errors.New("PII projection grant contains trailing JSON")
	}
	canonical, err := json.Marshal(grant)
	if err != nil || !bytes.Equal(body, canonical) {
		return PIIProjectionGrantV1{}, errors.New("PII projection grant is not canonically encoded")
	}
	return grant, ValidatePIIProjectionGrantV1(grant)
}

func PIIProjectionGrantV1Bytes(grant PIIProjectionGrantV1) ([]byte, error) {
	if err := ValidatePIIProjectionGrantV1(grant); err != nil {
		return nil, err
	}
	return json.Marshal(grant)
}

func PIIProjectionGrantSigningBytesV1(grant PIIProjectionGrantV1) []byte {
	grant.AuthoritySignature = ""
	grant.RecordDigest = ""
	body, _ := json.Marshal(grant)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), piiProjectionGrantSigningDomainV1...)
	return append(out, digest[:]...)
}

func PIIProjectionGrantAuthorityMaterialV1(grant PIIProjectionGrantV1) (string, []byte, []byte, error) {
	if err := ValidatePIIProjectionGrantV1(grant); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(grant.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(grant.AuthoritySignature)
	return grant.AuthorityKeyID, publicKey, signature, nil
}

func ApprovalScopeDigestV1(input ApprovalScopeInputV1) (string, error) {
	contextBinding, err := contextBindingFromSecurityContextV1(input.SecurityContext)
	if err != nil {
		return "", err
	}
	bindings, err := canonicalFieldBindingsV1(input.FieldBindings)
	if err != nil {
		return "", err
	}
	evidenceIDs := fieldBindingEvidenceIDsV1(bindings)
	accessActions, err := canonicalPIIAccessActionsV1(input.AllowedAccessActions)
	if err != nil {
		return "", err
	}
	grant := PIIProjectionGrantV1{
		SchemaVersion: PIIProjectionGrantSchemaVersionV1, Purpose: PIIProjectionGrantPurposeV1,
		Context: contextBinding, RequesterUserID: strings.TrimSpace(input.RequesterUserID),
		DisclosurePurpose: strings.TrimSpace(input.DisclosurePurpose), DeliveryScope: DeliveryScopeControlledArtifactV1,
		ClaimLedgerDigest: strings.TrimSpace(input.ClaimLedgerDigest), FieldBindings: bindings,
		FieldBindingSetDigest: fieldBindingSetDigestV1(bindings), EvidenceReceiptIDs: evidenceIDs,
		EvidenceReceiptSetDigest: evidenceReceiptSetDigestV1(evidenceIDs),
		ProjectionRulesetHash:    strings.TrimSpace(input.ProjectionRulesetHash), ProjectedContentSHA256: strings.TrimSpace(input.ProjectedContentSHA256),
		PreservedControlledFieldCount: input.PreservedControlledFieldCount, TargetIdentityDigest: strings.TrimSpace(input.TargetIdentityDigest),
		AllowedAccessActions: accessActions, AccessActionSetDigest: piiAccessActionSetDigestV1(accessActions),
		AccessPolicyDigest:    strings.TrimSpace(input.AccessPolicyDigest),
		RetentionPolicyDigest: strings.TrimSpace(input.RetentionPolicyDigest),
		RetentionUntil:        input.RetentionUntil.UTC().Format(time.RFC3339Nano),
		ExpiresAt:             input.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	digest := approvalScopeDigestV1(grant)
	contextIssuedAt, contextTimeErr := time.Parse(time.RFC3339Nano, grant.Context.ContextIssuedAt)
	canonicalBindings, bindingErr := canonicalFieldBindingsV1(grant.FieldBindings)
	if grant.RequesterUserID != grant.Context.UserID || grant.DisclosurePurpose != DisclosurePurposeCaseReportV1 ||
		grant.DeliveryScope != DeliveryScopeControlledArtifactV1 || !domainsecurity.IsSHA256Hex(grant.ClaimLedgerDigest) ||
		bindingErr != nil || !equalFieldBindingsV1(grant.FieldBindings, canonicalBindings) || len(grant.FieldBindings) == 0 ||
		len(grant.FieldBindings) > PIIProjectionGrantMaxFieldsV1 || grant.FieldBindingSetDigest != fieldBindingSetDigestV1(grant.FieldBindings) ||
		!canonicalEvidenceIDsV1(grant.EvidenceReceiptIDs) || !equalStringsV1(grant.EvidenceReceiptIDs, fieldBindingEvidenceIDsV1(grant.FieldBindings)) ||
		grant.EvidenceReceiptSetDigest != evidenceReceiptSetDigestV1(grant.EvidenceReceiptIDs) ||
		!domainsecurity.IsSHA256Hex(grant.ProjectionRulesetHash) || !domainsecurity.IsSHA256Hex(grant.ProjectedContentSHA256) ||
		grant.PreservedControlledFieldCount != uint64(len(grant.FieldBindings)) || !domainsecurity.IsSHA256Hex(grant.TargetIdentityDigest) ||
		!equalStringsV1(grant.AllowedAccessActions, accessActions) ||
		grant.AccessActionSetDigest != piiAccessActionSetDigestV1(accessActions) ||
		!domainsecurity.IsSHA256Hex(grant.AccessPolicyDigest) || !domainsecurity.IsSHA256Hex(grant.RetentionPolicyDigest) ||
		input.ExpiresAt.IsZero() || contextTimeErr != nil || !input.ExpiresAt.UTC().After(contextIssuedAt) ||
		input.ExpiresAt.UTC().Format(time.RFC3339Nano) != grant.ExpiresAt || input.RetentionUntil.IsZero() ||
		input.RetentionUntil.UTC().Before(input.ExpiresAt.UTC()) ||
		input.RetentionUntil.UTC().Format(time.RFC3339Nano) != grant.RetentionUntil {
		return "", errors.New("PII projection approval scope is incomplete")
	}
	return digest, nil
}

func ApprovalScopeInputFromGrantInputV1(input GrantInputV1) ApprovalScopeInputV1 {
	return ApprovalScopeInputV1{
		SecurityContext: input.SecurityContext, RequesterUserID: input.RequesterUserID,
		DisclosurePurpose: input.DisclosurePurpose, ClaimLedgerDigest: input.ClaimLedgerDigest,
		FieldBindings: append([]FieldBindingV1(nil), input.FieldBindings...), ProjectionRulesetHash: input.ProjectionRulesetHash,
		ProjectedContentSHA256: input.ProjectedContentSHA256, PreservedControlledFieldCount: input.PreservedControlledFieldCount,
		TargetIdentityDigest: input.TargetIdentityDigest, AllowedAccessActions: append([]string(nil), input.AllowedAccessActions...),
		AccessPolicyDigest: input.AccessPolicyDigest, RetentionPolicyDigest: input.RetentionPolicyDigest,
		RetentionUntil: input.RetentionUntil, ExpiresAt: input.ExpiresAt,
	}
}

func validatePIIProjectionGrantUnsignedV1(grant PIIProjectionGrantV1) error {
	if err := validatePIIProjectionGrantSemanticV1(grant); err != nil {
		return err
	}
	if grant.AuthorityAlgorithm != PIIProjectionGrantAlgorithmV1 || !domainsecurity.IsSHA256Hex(grant.AuthorityKeyID) ||
		strings.TrimSpace(grant.AuthorityPublicKey) == "" || grant.GrantID != piiProjectionGrantIDV1(grant) ||
		grant.ApprovalScopeDigest != approvalScopeDigestV1(grant) {
		return errors.New("PII projection grant authority or integrity is incomplete")
	}
	return nil
}

func validatePIIProjectionGrantSemanticV1(grant PIIProjectionGrantV1) error {
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	retentionUntil, retentionErr := time.Parse(time.RFC3339Nano, grant.RetentionUntil)
	contextIssuedAt, contextTimeErr := time.Parse(time.RFC3339Nano, grant.Context.ContextIssuedAt)
	canonicalBindings, bindingErr := canonicalFieldBindingsV1(grant.FieldBindings)
	canonicalActions, accessActionErr := canonicalPIIAccessActionsV1(grant.AllowedAccessActions)
	if grant.SchemaVersion != PIIProjectionGrantSchemaVersionV1 || grant.Purpose != PIIProjectionGrantPurposeV1 ||
		!validContextBindingV1(grant.Context) || grant.RequesterUserID != grant.Context.UserID ||
		grant.DisclosurePurpose != DisclosurePurposeCaseReportV1 || grant.DeliveryScope != DeliveryScopeControlledArtifactV1 ||
		!domainsecurity.IsSHA256Hex(grant.ClaimLedgerDigest) || bindingErr != nil || !equalFieldBindingsV1(grant.FieldBindings, canonicalBindings) ||
		len(grant.FieldBindings) == 0 || len(grant.FieldBindings) > PIIProjectionGrantMaxFieldsV1 ||
		grant.FieldBindingSetDigest != fieldBindingSetDigestV1(grant.FieldBindings) ||
		!canonicalEvidenceIDsV1(grant.EvidenceReceiptIDs) ||
		!equalStringsV1(grant.EvidenceReceiptIDs, fieldBindingEvidenceIDsV1(grant.FieldBindings)) ||
		grant.EvidenceReceiptSetDigest != evidenceReceiptSetDigestV1(grant.EvidenceReceiptIDs) ||
		!domainsecurity.IsSHA256Hex(grant.ProjectionRulesetHash) || !domainsecurity.IsSHA256Hex(grant.ProjectedContentSHA256) ||
		grant.PreservedControlledFieldCount != uint64(len(grant.FieldBindings)) || !domainsecurity.IsSHA256Hex(grant.TargetIdentityDigest) ||
		accessActionErr != nil || !equalStringsV1(grant.AllowedAccessActions, canonicalActions) ||
		grant.AccessActionSetDigest != piiAccessActionSetDigestV1(canonicalActions) ||
		!canonicalPIISHA256V1(grant.AccessPolicyDigest) || !canonicalPIISHA256V1(grant.RetentionPolicyDigest) ||
		!piiApprovalIDPattern.MatchString(grant.ApprovalID) || !domainsecurity.IsSHA256Hex(grant.ApprovalRecordDigest) ||
		!domainsecurity.IsSHA256Hex(grant.ApprovalScopeDigest) || issuedErr != nil || expiresErr != nil || retentionErr != nil ||
		contextTimeErr != nil ||
		issuedAt.IsZero() || expiresAt.IsZero() || issuedAt.Before(contextIssuedAt) || !expiresAt.After(issuedAt) ||
		expiresAt.Sub(issuedAt) > PIIProjectionGrantMaxTTL || issuedAt.UTC().Format(time.RFC3339Nano) != grant.IssuedAt ||
		expiresAt.UTC().Format(time.RFC3339Nano) != grant.ExpiresAt || retentionUntil.Before(expiresAt) ||
		retentionUntil.UTC().Format(time.RFC3339Nano) != grant.RetentionUntil {
		return errors.New("PII projection grant is incomplete")
	}
	return nil
}

func contextBindingFromSecurityContextV1(context domainsecurity.TurnSecurityContext) (ContextBindingV1, error) {
	if err := domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context); err != nil {
		return ContextBindingV1{}, err
	}
	return ContextBindingV1{
		Version: context.Version, ThreadID: context.ThreadID, TurnID: context.TurnID, WorkspaceRealPath: context.WorkspaceRealPath,
		TenantID: context.TenantID, UserID: context.UserID, CaseID: context.CaseID, CaseBindingHash: context.CaseBindingHash,
		DatasetSnapshotID: context.DatasetSnapshotID, SourceManifestHash: context.SourceManifestHash,
		ContextEpoch: context.ContextEpoch, ContextIssuedAt: context.IssuedAt, ContextDigest: context.ContextDigest,
	}, nil
}

func validContextBindingV1(binding ContextBindingV1) bool {
	for _, value := range []string{
		binding.ThreadID, binding.TurnID, binding.WorkspaceRealPath, binding.TenantID, binding.UserID, binding.CaseID,
		binding.CaseBindingHash, binding.DatasetSnapshotID, binding.SourceManifestHash, binding.ContextIssuedAt, binding.ContextDigest,
	} {
		if value == "" || value != strings.TrimSpace(value) {
			return false
		}
	}
	return binding.Version == domainsecurity.TurnSecurityContextVersionV2 && binding.ContextEpoch > 0 &&
		binding.CaseID != domainsecurity.UnboundCaseID && domainsecurity.IsSHA256Hex(binding.CaseBindingHash) &&
		domainsecurity.IsDatasetSnapshotIDV2Syntax(binding.DatasetSnapshotID) &&
		domainsecurity.IsSHA256Hex(binding.SourceManifestHash) && domainsecurity.IsSHA256Hex(binding.ContextDigest)
}

func canonicalFieldBindingsV1(values []FieldBindingV1) ([]FieldBindingV1, error) {
	if values == nil {
		return nil, errors.New("PII field bindings are required")
	}
	out := make([]FieldBindingV1, len(values))
	for index, value := range values {
		value.PIIClass = strings.TrimSpace(value.PIIClass)
		value.ClaimID = strings.TrimSpace(value.ClaimID)
		value.ClaimRecordDigest = strings.TrimSpace(value.ClaimRecordDigest)
		value.FieldName = strings.TrimSpace(value.FieldName)
		value.ValueSHA256 = strings.TrimSpace(value.ValueSHA256)
		value.EvidenceReceiptIDs = canonicalEvidenceIDListV1(value.EvidenceReceiptIDs)
		if !validFieldBindingV1(value) {
			return nil, errors.New("PII field binding is invalid")
		}
		out[index] = value
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.PIIClass != right.PIIClass {
			return left.PIIClass < right.PIIClass
		}
		if left.ClaimID != right.ClaimID {
			return left.ClaimID < right.ClaimID
		}
		if left.FieldName != right.FieldName {
			return left.FieldName < right.FieldName
		}
		return left.ValueSHA256 < right.ValueSHA256
	})
	for index := 1; index < len(out); index++ {
		if out[index-1].PIIClass == out[index].PIIClass && out[index-1].ClaimID == out[index].ClaimID &&
			out[index-1].FieldName == out[index].FieldName && out[index-1].ValueSHA256 == out[index].ValueSHA256 {
			return nil, errors.New("PII field binding is duplicated")
		}
	}
	return out, nil
}

func validFieldBindingV1(binding FieldBindingV1) bool {
	if binding.ClaimID == "" || !domainsecurity.IsSHA256Hex(binding.ClaimRecordDigest) || !domainsecurity.IsSHA256Hex(binding.ValueSHA256) ||
		len(binding.EvidenceReceiptIDs) == 0 || !canonicalEvidenceIDsV1(binding.EvidenceReceiptIDs) {
		return false
	}
	switch binding.PIIClass {
	case PIIClassFinancialAccountV1:
		return binding.FieldName == "accountId" && oneClaimTypeV1(binding.ClaimType,
			domainevidence.ClaimAccount, domainevidence.ClaimAmount, domainevidence.ClaimDirection, domainevidence.ClaimDateRange)
	case PIIClassDeviceIdentifierV1:
		return binding.FieldName == "deviceIdentifier" && binding.ClaimType == domainevidence.ClaimDeviceIdentifier
	case PIIClassAddressV1:
		return binding.FieldName == "attributeValue" && binding.ClaimType == domainevidence.ClaimAddress
	case PIIClassPersonNameV1:
		return binding.FieldName == "attributeValue" && binding.ClaimType == domainevidence.ClaimBidEditMetadata
	default:
		return false
	}
}

func oneClaimTypeV1(value domainevidence.ClaimType, allowed ...domainevidence.ClaimType) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func fieldBindingEvidenceIDsV1(bindings []FieldBindingV1) []string {
	values := []string{}
	for _, binding := range bindings {
		values = append(values, binding.EvidenceReceiptIDs...)
	}
	return canonicalEvidenceIDListV1(values)
}

func canonicalEvidenceIDListV1(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validEvidenceIDV1(value) || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func canonicalEvidenceIDsV1(values []string) bool {
	if values == nil {
		return false
	}
	for index, value := range values {
		if !validEvidenceIDV1(value) || index > 0 && value <= values[index-1] {
			return false
		}
	}
	return true
}

func validEvidenceIDV1(value string) bool {
	return strings.HasPrefix(value, "evr_") && domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, "evr_"))
}

func equalFieldBindingsV1(left, right []FieldBindingV1) bool {
	leftBody, _ := json.Marshal(left)
	rightBody, _ := json.Marshal(right)
	return bytes.Equal(leftBody, rightBody)
}

func equalStringsV1(left, right []string) bool {
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

func fieldBindingSetDigestV1(bindings []FieldBindingV1) string {
	body, _ := json.Marshal(bindings)
	return domainsecurity.SHA256Hex(append([]byte("analytix.pii-field-binding-set/v1\x00"), body...))
}

func evidenceReceiptSetDigestV1(ids []string) string {
	body, _ := json.Marshal(ids)
	return domainsecurity.SHA256Hex(append([]byte("analytix.pii-evidence-set/v1\x00"), body...))
}

func canonicalPIIAccessActionsV1(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > PIIProjectionGrantMaxAccessActionsV1 {
		return nil, errors.New("PII access actions are required")
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validControlledArtifactAccessActionV1(value) {
			return nil, errors.New("PII access action is invalid")
		}
		if _, exists := seen[value]; exists {
			return nil, errors.New("PII access action is duplicated")
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func piiAccessActionSetDigestV1(actions []string) string {
	body, _ := json.Marshal(actions)
	return domainsecurity.SHA256Hex(append([]byte("analytix.pii-access-action-set/v1\x00"), body...))
}

func canonicalPIISHA256V1(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func approvalScopeDigestV1(grant PIIProjectionGrantV1) string {
	value := struct {
		Context                       ContextBindingV1 `json:"context"`
		RequesterUserID               string           `json:"requesterUserId"`
		DisclosurePurpose             string           `json:"disclosurePurpose"`
		DeliveryScope                 string           `json:"deliveryScope"`
		ClaimLedgerDigest             string           `json:"claimLedgerDigest"`
		FieldBindingSetDigest         string           `json:"fieldBindingSetDigest"`
		EvidenceReceiptSetDigest      string           `json:"evidenceReceiptSetDigest"`
		ProjectionRulesetHash         string           `json:"projectionRulesetHash"`
		ProjectedContentSHA256        string           `json:"projectedContentSha256"`
		PreservedControlledFieldCount uint64           `json:"preservedControlledFieldCount"`
		TargetIdentityDigest          string           `json:"targetIdentityDigest"`
		AllowedAccessActions          []string         `json:"allowedAccessActions"`
		AccessActionSetDigest         string           `json:"accessActionSetDigest"`
		AccessPolicyDigest            string           `json:"accessPolicyDigest"`
		RetentionPolicyDigest         string           `json:"retentionPolicyDigest"`
		RetentionUntil                string           `json:"retentionUntil"`
		ExpiresAt                     string           `json:"expiresAt"`
	}{
		grant.Context, grant.RequesterUserID, grant.DisclosurePurpose, grant.DeliveryScope, grant.ClaimLedgerDigest,
		grant.FieldBindingSetDigest, grant.EvidenceReceiptSetDigest, grant.ProjectionRulesetHash,
		grant.ProjectedContentSHA256, grant.PreservedControlledFieldCount, grant.TargetIdentityDigest,
		grant.AllowedAccessActions, grant.AccessActionSetDigest, grant.AccessPolicyDigest,
		grant.RetentionPolicyDigest, grant.RetentionUntil, grant.ExpiresAt,
	}
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), piiApprovalScopeDomainV1...), body...))
}

func piiProjectionGrantIDV1(grant PIIProjectionGrantV1) string {
	grant.GrantID = ""
	grant.AuthorityAlgorithm = ""
	grant.AuthorityKeyID = ""
	grant.AuthorityPublicKey = ""
	grant.AuthoritySignature = ""
	grant.RecordDigest = ""
	body, _ := json.Marshal(grant)
	digest := sha256.Sum256(append(append([]byte(nil), piiProjectionGrantIDDomainV1...), body...))
	return "piig_" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func piiProjectionGrantRecordDigestV1(grant PIIProjectionGrantV1) string {
	grant.RecordDigest = ""
	body, _ := json.Marshal(grant)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), piiProjectionGrantDigestDomainV1...), body...))
}
