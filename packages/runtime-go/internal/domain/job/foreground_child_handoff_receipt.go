package job

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ForegroundChildHandoffReceiptSchemaVersionV1 = 1
	ForegroundChildHandoffReceiptPurposeV1       = "analytix.foreground-child-handoff-receipt/v1"
	ForegroundChildHandoffReceiptAcceptedV1      = "accepted"

	maxForegroundChildHandoffReceiptBytes = 64 * 1024
	maxForegroundChildHandoffTextBytes    = 8 * 1024
	maxForegroundChildHandoffTTL          = 15 * time.Minute
)

var foregroundChildHandoffReceiptDigestDomainV1 = []byte("analytix.foreground-child-handoff-receipt-digest/v1\x00")

// ForegroundChildHandoffReceiptV1 is a bounded, non-authoritative handoff
// capability for one accepted child terminal. It contains no child output,
// reasoning, evidence, or PII. A durable application adapter must atomically
// consume ConsumptionNonce exactly once before continuing the parent.
type ForegroundChildHandoffReceiptV1 struct {
	SchemaVersion               int                             `json:"schemaVersion"`
	Purpose                     string                          `json:"purpose"`
	Status                      string                          `json:"status"`
	ParentThreadID              string                          `json:"parentThreadId"`
	ParentTurnID                string                          `json:"parentTurnId"`
	ParentRunID                 string                          `json:"parentRunId"`
	ChildThreadID               string                          `json:"childThreadId"`
	ChildTurnID                 string                          `json:"childTurnId"`
	ChildRunID                  string                          `json:"childRunId"`
	WorkspaceRealPath           string                          `json:"workspaceRealPath"`
	ParentContextEpoch          uint64                          `json:"parentContextEpoch"`
	ChildContextEpoch           uint64                          `json:"childContextEpoch"`
	ParentExecutionGrantID      string                          `json:"parentExecutionGrantId"`
	ParentToolCallID            string                          `json:"parentToolCallId"`
	ChildAcceptedTerminalDigest string                          `json:"childAcceptedTerminalDigest"`
	SubmissionDigest            string                          `json:"submissionDigest"`
	PrivacyProjectionDigest     string                          `json:"privacyProjectionDigest"`
	ConsumptionNonce            string                          `json:"consumptionNonce"`
	IssuedAt                    string                          `json:"issuedAt"`
	ExpiresAt                   string                          `json:"expiresAt"`
	FactAnswerAllowed           bool                            `json:"factAnswerAllowed"`
	EvidenceAuthority           bool                            `json:"evidenceAuthority"`
	ParentGoalCompletionAllowed bool                            `json:"parentGoalCompletionAllowed"`
	ParentTodoCompletionAllowed bool                            `json:"parentTodoCompletionAllowed"`
	CaseBinding                 *ForegroundCaseHandoffBindingV1 `json:"caseBinding,omitempty"`
	ReceiptDigest               string                          `json:"receiptDigest"`
}

const ForegroundCaseHandoffBindingPurposeV1 = "analytix.foreground-case-handoff-binding/v1"

// ForegroundCaseHandoffBindingV1 is the optional case-only extension of the
// existing receipt family. It contains digests only; ordinary general receipts
// canonically omit it and therefore retain their original byte shape.
type ForegroundCaseHandoffBindingV1 struct {
	Purpose                      string `json:"purpose"`
	ParentContextDigest          string `json:"parentContextDigest"`
	ChildContextDigest           string `json:"childContextDigest"`
	WorkspaceScopeDigest         string `json:"workspaceScopeDigest"`
	CaseScopeDigest              string `json:"caseScopeDigest"`
	SecurityBindingDigest        string `json:"securityBindingDigest"`
	DelegationDigest             string `json:"delegationDigest"`
	ToolManifestDigest           string `json:"toolManifestDigest"`
	ChildExecutionGrantID        string `json:"childExecutionGrantId"`
	ChildToolCallDigest          string `json:"childToolCallDigest"`
	ChildCompletionReceiptDigest string `json:"childCompletionReceiptDigest"`
	ChildAcceptedFinalDigest     string `json:"childAcceptedFinalDigest"`
	TypedResultDigest            string `json:"typedResultDigest"`
	BindingDigest                string `json:"bindingDigest"`
}

type ForegroundChildHandoffReceiptInputV1 struct {
	ParentThreadID              string
	ParentTurnID                string
	ParentRunID                 string
	ChildThreadID               string
	ChildTurnID                 string
	ChildRunID                  string
	WorkspaceRealPath           string
	ParentContextEpoch          uint64
	ChildContextEpoch           uint64
	ParentExecutionGrantID      string
	ParentToolCallID            string
	ChildAcceptedTerminalDigest string
	SubmissionDigest            string
	PrivacyProjectionDigest     string
	ConsumptionNonce            string
	IssuedAt                    time.Time
	ExpiresAt                   time.Time
	CaseBinding                 *ForegroundCaseHandoffBindingV1
}

type foregroundChildHandoffReceiptWireV1 ForegroundChildHandoffReceiptV1

func NewForegroundChildHandoffReceiptV1(input ForegroundChildHandoffReceiptInputV1) (ForegroundChildHandoffReceiptV1, error) {
	if input.IssuedAt.IsZero() || input.ExpiresAt.IsZero() {
		return ForegroundChildHandoffReceiptV1{}, errors.New("foreground child handoff receipt time authority is missing")
	}
	receipt := ForegroundChildHandoffReceiptV1{
		SchemaVersion:  ForegroundChildHandoffReceiptSchemaVersionV1,
		Purpose:        ForegroundChildHandoffReceiptPurposeV1,
		Status:         ForegroundChildHandoffReceiptAcceptedV1,
		ParentThreadID: input.ParentThreadID, ParentTurnID: input.ParentTurnID, ParentRunID: input.ParentRunID,
		ChildThreadID: input.ChildThreadID, ChildTurnID: input.ChildTurnID, ChildRunID: input.ChildRunID,
		WorkspaceRealPath: input.WorkspaceRealPath, ParentContextEpoch: input.ParentContextEpoch,
		ChildContextEpoch: input.ChildContextEpoch, ParentExecutionGrantID: input.ParentExecutionGrantID,
		ParentToolCallID: input.ParentToolCallID, ChildAcceptedTerminalDigest: input.ChildAcceptedTerminalDigest,
		SubmissionDigest: input.SubmissionDigest, PrivacyProjectionDigest: input.PrivacyProjectionDigest,
		ConsumptionNonce: input.ConsumptionNonce, IssuedAt: input.IssuedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:         input.ExpiresAt.UTC().Format(time.RFC3339Nano),
		FactAnswerAllowed: false, EvidenceAuthority: false,
		ParentGoalCompletionAllowed: false, ParentTodoCompletionAllowed: false,
		CaseBinding: cloneForegroundCaseHandoffBindingV1(input.CaseBinding),
	}
	receipt.ReceiptDigest = foregroundChildHandoffReceiptDigestV1(receipt)
	if err := ValidateForegroundChildHandoffReceiptV1(receipt); err != nil {
		return ForegroundChildHandoffReceiptV1{}, err
	}
	return receipt, nil
}

func (receipt *ForegroundChildHandoffReceiptV1) UnmarshalJSON(raw []byte) error {
	if receipt == nil {
		return errors.New("foreground child handoff receipt target is nil")
	}
	parsed, err := parseForegroundChildHandoffReceiptV1(raw)
	if err != nil {
		return err
	}
	*receipt = parsed
	return nil
}

func parseForegroundChildHandoffReceiptV1(raw []byte) (ForegroundChildHandoffReceiptV1, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxForegroundChildHandoffReceiptBytes, MaxDepth: 2, MaxTokens: 128,
		MaxStringBytes: maxForegroundChildHandoffTextBytes,
	}); err != nil {
		return ForegroundChildHandoffReceiptV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var decoded foregroundChildHandoffReceiptWireV1
	if err := decoder.Decode(&decoded); err != nil {
		return ForegroundChildHandoffReceiptV1{}, err
	}
	candidate := ForegroundChildHandoffReceiptV1(decoded)
	canonical, err := json.Marshal(candidate)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ForegroundChildHandoffReceiptV1{}, errors.New("foreground child handoff receipt is not canonically encoded")
	}
	if err := ValidateForegroundChildHandoffReceiptV1(candidate); err != nil {
		return ForegroundChildHandoffReceiptV1{}, err
	}
	return candidate, nil
}

func ValidateForegroundChildHandoffReceiptV1(receipt ForegroundChildHandoffReceiptV1) error {
	if receipt.SchemaVersion != ForegroundChildHandoffReceiptSchemaVersionV1 ||
		receipt.Purpose != ForegroundChildHandoffReceiptPurposeV1 ||
		receipt.Status != ForegroundChildHandoffReceiptAcceptedV1 ||
		!foregroundChildHandoffCanonicalText(receipt.ParentThreadID) ||
		!foregroundChildHandoffCanonicalText(receipt.ParentTurnID) ||
		!foregroundChildHandoffCanonicalText(receipt.ParentRunID) ||
		!foregroundChildHandoffCanonicalText(receipt.ChildThreadID) ||
		!foregroundChildHandoffCanonicalText(receipt.ChildTurnID) ||
		!foregroundChildHandoffCanonicalText(receipt.ChildRunID) ||
		!foregroundChildHandoffCanonicalText(receipt.WorkspaceRealPath) ||
		receipt.ParentThreadID == receipt.ChildThreadID || receipt.ParentRunID == receipt.ChildRunID ||
		receipt.ParentContextEpoch == 0 || receipt.ChildContextEpoch == 0 ||
		!foregroundChildHandoffCanonicalSHA256(receipt.ParentExecutionGrantID) ||
		!foregroundChildHandoffCanonicalText(receipt.ParentToolCallID) ||
		!foregroundChildHandoffCanonicalSHA256(receipt.ChildAcceptedTerminalDigest) ||
		!foregroundChildHandoffCanonicalSHA256(receipt.SubmissionDigest) ||
		!foregroundChildHandoffCanonicalSHA256(receipt.PrivacyProjectionDigest) ||
		!foregroundChildHandoffCanonicalSHA256(receipt.ConsumptionNonce) ||
		receipt.FactAnswerAllowed || receipt.EvidenceAuthority ||
		receipt.ParentGoalCompletionAllowed || receipt.ParentTodoCompletionAllowed ||
		(receipt.CaseBinding != nil && (ValidateForegroundCaseHandoffBindingV1(*receipt.CaseBinding) != nil ||
			receipt.CaseBinding.TypedResultDigest != receipt.SubmissionDigest ||
			receipt.CaseBinding.TypedResultDigest != receipt.PrivacyProjectionDigest ||
			receipt.CaseBinding.ChildAcceptedFinalDigest != receipt.ChildAcceptedTerminalDigest)) ||
		!foregroundChildHandoffCanonicalSHA256(receipt.ReceiptDigest) {
		return errors.New("foreground child handoff receipt is incomplete")
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
	if issuedErr != nil || expiresErr != nil || issuedAt.IsZero() || expiresAt.IsZero() ||
		issuedAt.UTC().Format(time.RFC3339Nano) != receipt.IssuedAt ||
		expiresAt.UTC().Format(time.RFC3339Nano) != receipt.ExpiresAt ||
		!expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > maxForegroundChildHandoffTTL {
		return errors.New("foreground child handoff receipt validity window is invalid")
	}
	if foregroundChildHandoffReceiptDigestV1(receipt) != receipt.ReceiptDigest {
		return errors.New("foreground child handoff receipt integrity is invalid")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || len(canonical) > maxForegroundChildHandoffReceiptBytes {
		return errors.New("foreground child handoff receipt exceeds its canonical bound")
	}
	return nil
}

// ValidateForegroundChildHandoffReceiptForConsumptionV1 verifies the exact
// nonce and validity window. It does not make consumption durable; callers
// must use a consume-once CAS keyed by ConsumptionNonce before parent resume.
func ValidateForegroundChildHandoffReceiptForConsumptionV1(receipt ForegroundChildHandoffReceiptV1, expectedNonce string, at time.Time) error {
	if err := ValidateForegroundChildHandoffReceiptV1(receipt); err != nil {
		return err
	}
	if !foregroundChildHandoffCanonicalSHA256(expectedNonce) || expectedNonce != receipt.ConsumptionNonce {
		return errors.New("foreground child handoff consumption nonce is invalid")
	}
	if at.IsZero() {
		return errors.New("foreground child handoff consumption time is missing")
	}
	issuedAt, _ := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	expiresAt, _ := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
	at = at.UTC()
	if at.Before(issuedAt) || !at.Before(expiresAt) {
		return errors.New("foreground child handoff receipt is not consumable at this time")
	}
	return nil
}

func ParseForegroundChildHandoffReceiptV1(raw []byte) (ForegroundChildHandoffReceiptV1, error) {
	return parseForegroundChildHandoffReceiptV1(raw)
}

func ForegroundChildHandoffReceiptV1Bytes(receipt ForegroundChildHandoffReceiptV1) ([]byte, error) {
	if err := ValidateForegroundChildHandoffReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func CloneForegroundChildHandoffReceiptV1(receipt *ForegroundChildHandoffReceiptV1) *ForegroundChildHandoffReceiptV1 {
	if receipt == nil {
		return nil
	}
	clone := *receipt
	clone.CaseBinding = cloneForegroundCaseHandoffBindingV1(receipt.CaseBinding)
	return &clone
}

func ForegroundChildHandoffReceiptsEqualV1(left, right *ForegroundChildHandoffReceiptV1) bool {
	return left != nil && right != nil && left.ReceiptDigest == right.ReceiptDigest && reflect.DeepEqual(left, right)
}

func NewForegroundCaseHandoffBindingV1(
	parent domainsecurity.TurnSecurityContext,
	child domainsecurity.TurnSecurityContext,
	securityBinding *SecurityBinding,
	delegation *CaseDelegationContextV1,
	manifest *DelegatedToolManifestV1,
	toolScope []string,
	toolSchemaHash string,
	childExecutionGrantID string,
	childToolCallID string,
	completion ChildCompletionReceiptV1,
	typedResultDigest string,
) (*ForegroundCaseHandoffBindingV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(child) != nil ||
		ValidateSecurityBinding(securityBinding) != nil || !SecurityBindingMatchesContext(securityBinding, parent) ||
		ValidateCaseDelegationContextV1(delegation, securityBinding) != nil ||
		ValidateDelegatedToolManifestV1(manifest, toolScope, toolSchemaHash) != nil ||
		!domainsecurity.IsSHA256Hex(childExecutionGrantID) || !foregroundChildHandoffCanonicalText(childToolCallID) ||
		ValidateChildCompletionReceiptV1(completion) != nil ||
		!ChildCompletionContextRefV1MatchesContext(completion.ParentContext, parent) ||
		!ChildCompletionContextRefV1MatchesContext(completion.ChildContext, child) ||
		completion.ChildRunID == "" || completion.SecurityBindingDigest != securityBinding.BindingDigest ||
		completion.ParentExecutionGrantID != securityBinding.ParentExecutionGrantID ||
		completion.ParentToolCallID != securityBinding.ParentToolCallID || !completion.CanContinueParent ||
		!domainsecurity.IsSHA256Hex(typedResultDigest) {
		return nil, errors.New("foreground case handoff binding authority is invalid")
	}
	workspaceScope := domainsecurity.SHA256Hex([]byte(strings.Join([]string{
		"analytix.foreground-case-workspace-scope/v1", parent.TenantID, parent.UserID, parent.WorkspaceRealPath,
	}, "\x00")))
	caseScope := domainsecurity.SHA256Hex([]byte(strings.Join([]string{
		"analytix.foreground-case-scope/v1", parent.CaseID, parent.CaseBindingHash,
		parent.DatasetSnapshotID, parent.SourceManifestHash, parent.ContextDigest,
		child.ContextDigest, fmt.Sprint(parent.ContextEpoch), fmt.Sprint(child.ContextEpoch),
	}, "\x00")))
	binding := &ForegroundCaseHandoffBindingV1{
		Purpose:             ForegroundCaseHandoffBindingPurposeV1,
		ParentContextDigest: parent.ContextDigest, ChildContextDigest: child.ContextDigest,
		WorkspaceScopeDigest: workspaceScope, CaseScopeDigest: caseScope,
		SecurityBindingDigest: securityBinding.BindingDigest, DelegationDigest: delegation.DelegationDigest,
		ToolManifestDigest: manifest.ManifestHash, ChildExecutionGrantID: childExecutionGrantID,
		ChildToolCallDigest: domainsecurity.SHA256Hex([]byte(childToolCallID)), ChildCompletionReceiptDigest: completion.ReceiptDigest,
		ChildAcceptedFinalDigest: completion.AcceptedFinalDigest, TypedResultDigest: typedResultDigest,
	}
	binding.BindingDigest = foregroundCaseHandoffBindingDigestV1(*binding)
	if ValidateForegroundCaseHandoffBindingV1(*binding) != nil {
		return nil, errors.New("foreground case handoff binding is invalid")
	}
	return binding, nil
}

func ValidateForegroundCaseHandoffBindingV1(binding ForegroundCaseHandoffBindingV1) error {
	if binding.Purpose != ForegroundCaseHandoffBindingPurposeV1 ||
		!foregroundChildHandoffCanonicalSHA256(binding.ParentContextDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.ChildContextDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.WorkspaceScopeDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.CaseScopeDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.SecurityBindingDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.DelegationDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.ToolManifestDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.ChildExecutionGrantID) ||
		!foregroundChildHandoffCanonicalSHA256(binding.ChildToolCallDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.ChildCompletionReceiptDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.ChildAcceptedFinalDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.TypedResultDigest) ||
		!foregroundChildHandoffCanonicalSHA256(binding.BindingDigest) ||
		binding.BindingDigest != foregroundCaseHandoffBindingDigestV1(binding) {
		return errors.New("foreground case handoff binding is invalid")
	}
	return nil
}

// ForegroundCaseHandoffBindingMatchesDurableV1 rebinds the digest-only case
// extension to the existing durable child-run owners. It does not rehydrate
// the live typed result or its one-use consumption capability.
func ForegroundCaseHandoffBindingMatchesDurableV1(
	binding ForegroundCaseHandoffBindingV1,
	securityBinding *SecurityBinding,
	delegation *CaseDelegationContextV1,
	manifest *DelegatedToolManifestV1,
	completion *ChildCompletionReceiptV1,
) bool {
	if ValidateForegroundCaseHandoffBindingV1(binding) != nil ||
		ValidateSecurityBinding(securityBinding) != nil ||
		ValidateCaseDelegationContextV1(delegation, securityBinding) != nil ||
		manifest == nil ||
		ValidateDelegatedToolManifestV1(manifest, []string{"submit_child_result"}, manifest.ToolSchemaHash) != nil ||
		completion == nil || ValidateChildCompletionReceiptV1(*completion) != nil ||
		completion.SecurityBindingDigest != securityBinding.BindingDigest ||
		completion.ParentExecutionGrantID != securityBinding.ParentExecutionGrantID ||
		completion.ParentToolCallID != securityBinding.ParentToolCallID {
		return false
	}
	workspaceScope := domainsecurity.SHA256Hex([]byte(strings.Join([]string{
		"analytix.foreground-case-workspace-scope/v1", completion.ParentContext.TenantID,
		completion.ParentContext.UserID, completion.ParentContext.WorkspaceRealPath,
	}, "\x00")))
	caseScope := domainsecurity.SHA256Hex([]byte(strings.Join([]string{
		"analytix.foreground-case-scope/v1", completion.ParentContext.CaseID,
		completion.ParentContext.CaseBindingHash, completion.ParentContext.DatasetSnapshotID,
		completion.ParentContext.SourceManifestHash, completion.ParentContext.ContextDigest,
		completion.ChildContext.ContextDigest, fmt.Sprint(completion.ParentContext.ContextEpoch),
		fmt.Sprint(completion.ChildContext.ContextEpoch),
	}, "\x00")))
	return binding.ParentContextDigest == completion.ParentContext.ContextDigest &&
		binding.ChildContextDigest == completion.ChildContext.ContextDigest &&
		binding.WorkspaceScopeDigest == workspaceScope && binding.CaseScopeDigest == caseScope &&
		binding.SecurityBindingDigest == securityBinding.BindingDigest &&
		binding.DelegationDigest == delegation.DelegationDigest &&
		binding.ToolManifestDigest == manifest.ManifestHash &&
		binding.ChildCompletionReceiptDigest == completion.ReceiptDigest &&
		binding.ChildAcceptedFinalDigest == completion.AcceptedFinalDigest
}

func cloneForegroundCaseHandoffBindingV1(binding *ForegroundCaseHandoffBindingV1) *ForegroundCaseHandoffBindingV1 {
	if binding == nil {
		return nil
	}
	clone := *binding
	return &clone
}

func foregroundCaseHandoffBindingDigestV1(binding ForegroundCaseHandoffBindingV1) string {
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	return domainsecurity.SHA256Hex(append([]byte("analytix.foreground-case-handoff-binding/digest/v1\x00"), body...))
}

func foregroundChildHandoffReceiptDigestV1(receipt ForegroundChildHandoffReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	payload := append([]byte(nil), foregroundChildHandoffReceiptDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func foregroundChildHandoffCanonicalSHA256(value string) bool {
	return value == strings.ToLower(value) && domainsecurity.IsSHA256Hex(value)
}

func foregroundChildHandoffCanonicalText(value string) bool {
	if value == "" || len(value) > maxForegroundChildHandoffTextBytes || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
