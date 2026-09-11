package piiauthorization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

const (
	ControlledPIIApprovalArgumentsSchemaVersionV1 = 1
	ControlledPIIApprovalArgumentsPurposeV1       = "analytix.controlled-pii-approval/v1"
	maxControlledPIIApprovalArgumentsBytesV1      = 4096
)

// ControlledPIIApprovalArgumentsV1 is the complete private continuation
// argument contract shown to the approval host. It contains no raw PII: the
// semantic scope digest binds the exact verified fields and receipts, while
// the duplicate report/target hashes make confused-deputy mismatches explicit.
type ControlledPIIApprovalArgumentsV1 struct {
	SchemaVersion          int      `json:"schemaVersion"`
	Purpose                string   `json:"purpose"`
	ApprovalScopeDigest    string   `json:"approvalScopeDigest"`
	TargetIdentityDigest   string   `json:"targetIdentityDigest"`
	ProjectedContentSHA256 string   `json:"projectedContentSha256"`
	AllowedAccessActions   []string `json:"allowedAccessActions"`
	AccessPolicyDigest     string   `json:"accessPolicyDigest"`
	RetentionPolicyDigest  string   `json:"retentionPolicyDigest"`
	RetentionUntil         string   `json:"retentionUntil"`
	ExpiresAt              string   `json:"expiresAt"`
}

type ControlledPIIApprovalArgumentsInputV1 struct {
	ApprovalScopeDigest    string
	TargetIdentityDigest   string
	ProjectedContentSHA256 string
	AllowedAccessActions   []string
	AccessPolicyDigest     string
	RetentionPolicyDigest  string
	RetentionUntil         time.Time
	ExpiresAt              time.Time
}

func CanonicalControlledPIIApprovalArgumentsV1(input ControlledPIIApprovalArgumentsInputV1) (json.RawMessage, error) {
	accessActions, err := canonicalControlledPIIAccessActionsV1(input.AllowedAccessActions)
	if err != nil {
		return nil, err
	}
	arguments := ControlledPIIApprovalArgumentsV1{
		SchemaVersion: ControlledPIIApprovalArgumentsSchemaVersionV1, Purpose: ControlledPIIApprovalArgumentsPurposeV1,
		ApprovalScopeDigest: strings.TrimSpace(input.ApprovalScopeDigest), TargetIdentityDigest: strings.TrimSpace(input.TargetIdentityDigest),
		ProjectedContentSHA256: strings.TrimSpace(input.ProjectedContentSHA256), AllowedAccessActions: accessActions,
		AccessPolicyDigest:    strings.TrimSpace(input.AccessPolicyDigest),
		RetentionPolicyDigest: strings.TrimSpace(input.RetentionPolicyDigest),
		RetentionUntil:        input.RetentionUntil.UTC().Format(time.RFC3339Nano),
		ExpiresAt:             input.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	if err := validateControlledPIIApprovalArgumentsV1(arguments); err != nil {
		return nil, err
	}
	body, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}
	canonical, err := domaincontinuation.CanonicalPrivateToolArgumentsV2(body)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func ParseControlledPIIApprovalArgumentsV1(body []byte) (ControlledPIIApprovalArgumentsV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxControlledPIIApprovalArgumentsBytesV1, MaxDepth: 3, MaxTokens: 64, MaxStringBytes: 512,
	}); err != nil {
		return ControlledPIIApprovalArgumentsV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var arguments ControlledPIIApprovalArgumentsV1
	if err := decoder.Decode(&arguments); err != nil {
		return ControlledPIIApprovalArgumentsV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ControlledPIIApprovalArgumentsV1{}, errors.New("controlled PII approval arguments contain trailing JSON")
	}
	canonical, err := domaincontinuation.CanonicalPrivateToolArgumentsV2(body)
	if err != nil || !bytes.Equal(body, canonical) {
		return ControlledPIIApprovalArgumentsV1{}, errors.New("controlled PII approval arguments are not canonical JSON")
	}
	return arguments, validateControlledPIIApprovalArgumentsV1(arguments)
}

func validateControlledPIIApprovalArgumentsV1(arguments ControlledPIIApprovalArgumentsV1) error {
	expiresAt, err := time.Parse(time.RFC3339Nano, arguments.ExpiresAt)
	retentionUntil, retentionErr := time.Parse(time.RFC3339Nano, arguments.RetentionUntil)
	accessActions, accessActionErr := canonicalControlledPIIAccessActionsV1(arguments.AllowedAccessActions)
	if arguments.SchemaVersion != ControlledPIIApprovalArgumentsSchemaVersionV1 || arguments.Purpose != ControlledPIIApprovalArgumentsPurposeV1 ||
		!domainsecurity.IsSHA256Hex(arguments.ApprovalScopeDigest) || !domainsecurity.IsSHA256Hex(arguments.TargetIdentityDigest) ||
		!domainsecurity.IsSHA256Hex(arguments.ProjectedContentSHA256) ||
		accessActionErr != nil || !sameControlledPIIAccessActionsV1(arguments.AllowedAccessActions, accessActions) ||
		!canonicalControlledPIIDigestV1(arguments.AccessPolicyDigest) ||
		!canonicalControlledPIIDigestV1(arguments.RetentionPolicyDigest) ||
		err != nil || retentionErr != nil || expiresAt.IsZero() || retentionUntil.Before(expiresAt) ||
		expiresAt.UTC().Format(time.RFC3339Nano) != arguments.ExpiresAt ||
		retentionUntil.UTC().Format(time.RFC3339Nano) != arguments.RetentionUntil {
		return errors.New("controlled PII approval arguments are invalid")
	}
	return nil
}

type ContinuationApprovalAuthority struct {
	continuations *continuationapp.Service
	now           func() time.Time
}

func NewContinuationApprovalAuthority(continuations *continuationapp.Service, now func() time.Time) (*ContinuationApprovalAuthority, error) {
	if continuations == nil || !continuations.Available() {
		return nil, ErrUnavailable
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &ContinuationApprovalAuthority{continuations: continuations, now: now}, nil
}

var _ piiauthorizationport.ApprovalAuthority = (*ContinuationApprovalAuthority)(nil)

func (authority *ContinuationApprovalAuthority) ValidateCurrent(ctx context.Context, input piiauthorizationport.ApprovalValidationV1) error {
	if authority == nil || authority.continuations == nil || authority.now == nil || ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		input.RequesterUserID != input.Context.UserID || input.DisclosurePurpose != domainpii.DisclosurePurposeCaseReportV1 ||
		!domainsecurity.IsSHA256Hex(input.ApprovalScopeDigest) || !domainsecurity.IsSHA256Hex(input.ApprovalRecordDigest) ||
		!domainsecurity.IsSHA256Hex(input.TargetIdentityDigest) || !domainsecurity.IsSHA256Hex(input.ProjectedContentHash) ||
		!canonicalControlledPIIDigestV1(input.AccessPolicyDigest) ||
		!canonicalControlledPIIDigestV1(input.RetentionPolicyDigest) {
		return ErrMismatch
	}
	receipt, disposition, err := authority.continuations.ResolveTrustedDisposition(ctx, input.ApprovalID)
	if err != nil {
		return errors.Join(ErrMismatch, err)
	}
	if receipt.Payload.Kind != domaincontinuation.KindApproval || receipt.Payload.GateID != input.ApprovalID ||
		receipt.Payload.ToolName != pendingworkapp.ReportStageToolName || receipt.Payload.ExecutionGrant.ServerIdentity != "host:builtin" ||
		receipt.Payload.ExecutionGrant.ReadOnly || receipt.Payload.ExecutionGrant.ApprovalState != "pending" ||
		len(receipt.Payload.ToolScope) != 1 || receipt.Payload.ToolScope[0] != pendingworkapp.ReportStageToolName ||
		!sameControlledPIIApprovalContextV1(receipt.Payload.SecurityContext, input.Context) ||
		disposition.GateID != input.ApprovalID || disposition.DispositionID != input.ApprovalRecordDigest ||
		disposition.Status != domaincontinuation.StatusAllowed || disposition.ReasonCode != "approval_allowed" {
		return ErrMismatch
	}
	arguments, err := ParseControlledPIIApprovalArgumentsV1(receipt.Payload.Arguments)
	if err != nil || arguments.ApprovalScopeDigest != input.ApprovalScopeDigest ||
		arguments.TargetIdentityDigest != input.TargetIdentityDigest || arguments.ProjectedContentSHA256 != input.ProjectedContentHash ||
		!sameControlledPIIAccessActionsV1(arguments.AllowedAccessActions, input.AllowedAccessActions) ||
		arguments.AccessPolicyDigest != input.AccessPolicyDigest ||
		arguments.RetentionPolicyDigest != input.RetentionPolicyDigest ||
		arguments.RetentionUntil != input.RetentionUntil || arguments.ExpiresAt != input.ExpiresAt {
		return errors.Join(ErrMismatch, err)
	}
	now := authority.now().UTC()
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, arguments.ExpiresAt)
	grantExpiresAt, grantExpiryErr := time.Parse(time.RFC3339Nano, receipt.Payload.ExecutionGrant.ExpiresAt)
	receiptIssuedAt, receiptIssueErr := time.Parse(time.RFC3339Nano, receipt.Payload.IssuedAt)
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if now.IsZero() || expiresErr != nil || grantExpiryErr != nil || receiptIssueErr != nil || disposedErr != nil ||
		now.Before(disposedAt) || !now.Before(expiresAt) || disposedAt.Before(receiptIssuedAt) || !disposedAt.Before(expiresAt) ||
		expiresAt.After(grantExpiresAt) {
		return ErrExpired
	}
	return nil
}

func canonicalControlledPIIAccessActionsV1(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > domainpii.PIIProjectionGrantMaxAccessActionsV1 {
		return nil, ErrMismatch
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != domainpii.ControlledArtifactAccessActionDisplayV1 &&
			value != domainpii.ControlledArtifactAccessActionExportV1 {
			return nil, ErrMismatch
		}
		if _, exists := seen[value]; exists {
			return nil, ErrMismatch
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func sameControlledPIIAccessActionsV1(left []string, right []string) bool {
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

func canonicalControlledPIIDigestV1(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func sameControlledPIIApprovalContextV1(left, right domainsecurity.TurnSecurityContext) bool {
	return left.Version == right.Version && left.ThreadID == right.ThreadID && left.TurnID == right.TurnID &&
		left.WorkspaceRealPath == right.WorkspaceRealPath && left.TenantID == right.TenantID && left.UserID == right.UserID &&
		left.CaseID == right.CaseID && left.CaseBindingHash == right.CaseBindingHash &&
		left.DatasetSnapshotID == right.DatasetSnapshotID && left.SourceManifestHash == right.SourceManifestHash &&
		left.ContextEpoch == right.ContextEpoch && left.IssuedAt == right.IssuedAt && left.ContextDigest == right.ContextDigest
}
