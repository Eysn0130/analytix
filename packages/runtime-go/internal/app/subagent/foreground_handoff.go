package subagent

import (
	"context"
	"crypto/rand"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	modelapp "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const foregroundHandoffTTL = 10 * time.Minute

// ForegroundParentCapabilityFieldV1 is process-private settlement material.
// It must be removed before any durable, SSE, history, or provider projection.
const ForegroundParentCapabilityFieldV1 = "_hostForegroundParentCapabilityV1"

type foregroundSubmission struct {
	HandoffKind             string
	ChildRunID              string
	ChildThreadID           string
	ChildTurnID             string
	WorkspaceRealPath       string
	ChildContextDigest      string
	ChildContextEpoch       uint64
	ChildExecutionGrantID   string
	ChildToolCallID         string
	SubmissionDigest        string
	PrivacyProjectionDigest string
	ProjectedResult         string
	CaseAllowedBinding      *domainjob.CaseDelegatedAnswerSlotBindingV1
	CaseResult              *domainjob.CaseForegroundChildResultV1
	ConsumptionNonce        string
	SubmittedAt             time.Time
	ExpiresAt               time.Time
	Consumed                bool
}

// StageCase binds the one current host-private answer-slot group to the
// existing process-local foreground registry after the durable child record
// has been created. Restart intentionally loses this allowance and therefore
// fails closed instead of rehydrating typed data from durable output/prompt.
func (registry *ForegroundSubmissionRegistry) StageCase(
	record domainjob.Record,
	parent domainsecurity.TurnSecurityContext,
	binding domainjob.CaseDelegatedAnswerSlotBindingV1,
) error {
	commitment, err := domainjob.NewCaseDelegatedAnswerSlotCommitmentV1(binding)
	if registry == nil || err != nil || strings.TrimSpace(record.Status) != string(domainjob.StatusQueued) ||
		record.Background || record.CaseDelegation == nil || len(record.CaseDelegation.Semantic.AnswerSlots) != 1 ||
		commitment != record.CaseDelegation.Semantic.AnswerSlots[0] ||
		domainjob.ValidateCaseDelegationContextV1(record.CaseDelegation, record.SecurityBinding) != nil ||
		domainjob.ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) != nil ||
		!domainjob.SecurityBindingMatchesContext(record.SecurityBinding, parent) ||
		record.ParentThreadID != parent.ThreadID || record.ParentTurnID != parent.TurnID ||
		record.ParentToolCallID != record.SecurityBinding.ParentToolCallID {
		return errors.New("foreground case allowance is invalid")
	}
	now := registry.currentTime()
	copyBinding := binding
	copyBinding.AnswerSlot.Gaps = append([]string{}, binding.AnswerSlot.Gaps...)
	copyBinding.Claims = append([]domainjob.CaseDelegatedClaimReferenceV1{}, binding.Claims...)
	copyBinding.Evidence = append([]domainjob.CaseDelegatedEvidenceReferenceV1{}, binding.Evidence...)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneLocked(now)
	if _, exists := registry.entries[record.ID]; exists {
		return errors.New("foreground case allowance already exists")
	}
	registry.entries[record.ID] = foregroundSubmission{
		HandoffKind: ForegroundHandoffCaseTypedV1, ChildRunID: record.ID,
		WorkspaceRealPath: parent.WorkspaceRealPath, CaseAllowedBinding: &copyBinding,
		SubmittedAt: now, ExpiresAt: now.Add(foregroundHandoffTTL),
	}
	return nil
}

// ForegroundSubmissionRegistry owns the only live result capability. Durable
// receipts deliberately cannot rehydrate it after restart.
type ForegroundSubmissionRegistry struct {
	mu      sync.Mutex
	entries map[string]foregroundSubmission
	now     func() time.Time
}

type ForegroundChildRunReader interface {
	LoadChildRun(string) (domainjob.Record, error)
}

func NewForegroundSubmissionRegistry() *ForegroundSubmissionRegistry {
	return &ForegroundSubmissionRegistry{
		entries: map[string]foregroundSubmission{},
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func SubmitForegroundChildResult(
	ctx context.Context,
	authority *ForegroundHandoffAuthority,
	childRuns ForegroundChildRunReader,
	pending modelapp.PendingToolCall,
	args map[string]any,
) (any, bool) {
	if authority == nil || foregroundChildRunReaderIsNil(childRuns) || pending.SubagentDepth != 1 ||
		len(pending.ToolScope) != 1 || strings.TrimSpace(pending.ToolScope[0]) != toolcatalogForegroundSubmitToolName {
		return map[string]any{"code": "foreground_handoff_unavailable", "error": "foreground child handoff authority is unavailable"}, true
	}
	_, childRunID, _, _, _ := modelapp.PendingProviderRuntimeValues(pending)
	record, err := childRuns.LoadChildRun(childRunID)
	if err != nil || strings.TrimSpace(childRunID) == "" || record.ID != childRunID {
		return map[string]any{"code": "foreground_handoff_invalid", "error": "foreground child run is unavailable"}, true
	}
	var output map[string]any
	if record.CaseDelegation != nil {
		if len(args) != 1 || args["caseResult"] == nil {
			return map[string]any{"code": "validation_error", "error": "caseResult is required"}, true
		}
		output, err = authority.SubmitCase(ctx, record, pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID, args["caseResult"])
	} else {
		result, ok := args["result"].(string)
		if !ok || len(args) != 1 {
			return map[string]any{"code": "validation_error", "error": "result is required"}, true
		}
		output, err = authority.Submit(record, pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID, result)
	}
	if err != nil {
		return map[string]any{"code": "foreground_handoff_rejected", "error": err.Error()}, true
	}
	return output, false
}

func foregroundChildRunReaderIsNil(reader ForegroundChildRunReader) bool {
	if reader == nil {
		return true
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (registry *ForegroundSubmissionRegistry) Submit(
	record domainjob.Record,
	child domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	toolCallID string,
	result string,
) (map[string]any, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	if registry == nil || domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil ||
		strings.TrimSpace(record.Kind) != "subagent" || strings.TrimSpace(record.Status) != string(domainjob.StatusRunning) ||
		record.Background || record.CaseDelegation != nil || !foregroundSubmitOnlyRecord(record) ||
		domainsecurity.ValidateTurnSecurityContextForExecution(child) != nil ||
		!domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(child) ||
		strings.TrimSpace(record.ChildThreadID) == "" || record.ChildThreadID != child.ThreadID ||
		record.Workspace != child.WorkspaceRealPath || record.SecurityBinding.ParentWorkspaceRealPath != child.WorkspaceRealPath ||
		record.SecurityBinding.TenantID != child.TenantID || record.SecurityBinding.UserID != child.UserID ||
		domainsecurity.ValidateExecutionGrantForContext(grant, child) != nil ||
		grant.ToolName != toolcatalogForegroundSubmitToolName || grant.ToolCallID != toolCallID || !grant.ReadOnly {
		return nil, errors.New("foreground child submission authority is invalid")
	}
	public, err := domainevent.FilterPublicText(result)
	if err != nil {
		return nil, errors.New("foreground child submission contains private reasoning")
	}
	if domainsecurity.ContainsProtectedCaseFactCandidate(public) {
		return nil, errors.New("foreground child submission contains a protected case fact candidate")
	}
	projected := strings.TrimSpace(domainordinary.ProjectTextV1(public))
	if projected == "" || len([]byte(projected)) > 8192 {
		return nil, errors.New("foreground child submission is empty or exceeds its bound")
	}
	now := registry.currentTime()
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || !now.Before(expiresAt) {
		return nil, errors.New("foreground child submission grant is expired")
	}
	if max := now.Add(foregroundHandoffTTL); expiresAt.After(max) {
		expiresAt = max
	}
	nonce, err := foregroundConsumptionNonce()
	if err != nil {
		return nil, err
	}
	entry := foregroundSubmission{
		HandoffKind: ForegroundHandoffGeneralV1,
		ChildRunID:  record.ID, ChildThreadID: child.ThreadID, ChildTurnID: child.TurnID,
		WorkspaceRealPath: child.WorkspaceRealPath, ChildContextDigest: child.ContextDigest,
		ChildContextEpoch: child.ContextEpoch, ChildExecutionGrantID: grant.GrantID, ChildToolCallID: toolCallID,
		SubmissionDigest:        domainsecurity.SHA256Hex([]byte(result)),
		PrivacyProjectionDigest: domainsecurity.SHA256Hex([]byte(projected)),
		ProjectedResult:         projected, ConsumptionNonce: nonce, SubmittedAt: now, ExpiresAt: expiresAt,
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneLocked(now)
	if _, exists := registry.entries[record.ID]; exists {
		return nil, errors.New("foreground child submission already exists")
	}
	registry.entries[record.ID] = entry
	return map[string]any{
		"status": "accepted", "submissionDigest": entry.SubmissionDigest,
		"privacyProjectionDigest": entry.PrivacyProjectionDigest,
		"factAnswerAllowed":       false, "evidenceAuthority": false,
	}, nil
}

func (registry *ForegroundSubmissionRegistry) SubmitCase(
	record domainjob.Record,
	child domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	toolCallID string,
	selection domainjob.CaseForegroundChildSelectionV1,
	result domainjob.CaseForegroundChildResultV1,
	validateCurrent func() error,
) (map[string]any, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	if registry == nil || domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil ||
		strings.TrimSpace(record.Kind) != "subagent" || strings.TrimSpace(record.Status) != string(domainjob.StatusRunning) ||
		record.Background || !foregroundSubmitOnlyRecord(record) ||
		domainjob.ValidateCaseDelegationContextV1(record.CaseDelegation, record.SecurityBinding) != nil ||
		domainjob.ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil ||
		record.ParentToolCallID != record.SecurityBinding.ParentToolCallID ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(child) != nil ||
		strings.TrimSpace(record.ChildThreadID) == "" || record.ChildThreadID != child.ThreadID ||
		record.Workspace != child.WorkspaceRealPath || record.SecurityBinding.ParentWorkspaceRealPath != child.WorkspaceRealPath ||
		record.SecurityBinding.TenantID != child.TenantID || record.SecurityBinding.UserID != child.UserID ||
		record.SecurityBinding.ParentCaseID != child.CaseID ||
		record.SecurityBinding.ParentCaseBindingHash != child.CaseBindingHash ||
		record.SecurityBinding.ParentDatasetSnapshot != child.DatasetSnapshotID ||
		record.SecurityBinding.ParentSourceManifest != child.SourceManifestHash ||
		domainsecurity.ValidateExecutionGrantForContext(grant, child) != nil ||
		grant.ToolName != toolcatalogForegroundSubmitToolName || grant.ToolCallID != toolCallID || !grant.ReadOnly {
		return nil, errors.New("foreground case child submission authority is invalid")
	}
	if domainjob.ValidateNormalizedCaseForegroundChildSelectionV1(selection, record.CaseDelegation) != nil ||
		domainjob.ValidateCaseForegroundChildResultV1(result, record.CaseDelegation) != nil || validateCurrent == nil {
		return nil, errors.New("foreground case child submission is not a delegated typed result")
	}
	canonical, err := domainjob.CanonicalCaseForegroundChildResultV1(result)
	if err != nil {
		return nil, errors.New("foreground case child submission is not a delegated typed result")
	}
	now := registry.currentTime()
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || !now.Before(expiresAt) {
		return nil, errors.New("foreground case child submission grant is expired")
	}
	if max := now.Add(foregroundHandoffTTL); expiresAt.After(max) {
		expiresAt = max
	}
	nonce, err := foregroundConsumptionNonce()
	if err != nil {
		return nil, err
	}
	digest := domainsecurity.SHA256Hex(canonical)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneLocked(now)
	entry, exists := registry.entries[record.ID]
	if !exists || entry.CaseAllowedBinding == nil || entry.CaseResult != nil || entry.ConsumptionNonce != "" ||
		entry.HandoffKind != ForegroundHandoffCaseTypedV1 || entry.ChildRunID != record.ID ||
		entry.WorkspaceRealPath != child.WorkspaceRealPath || !now.Before(entry.ExpiresAt) {
		return nil, errors.New("foreground case child submission already exists")
	}
	commitment, commitmentErr := domainjob.NewCaseDelegatedAnswerSlotCommitmentV1(*entry.CaseAllowedBinding)
	if commitmentErr != nil || commitment.Digest != selection.AnswerSlotDigest ||
		!reflect.DeepEqual(entry.CaseAllowedBinding.AnswerSlot, result.AnswerSlots[0]) ||
		!reflect.DeepEqual(entry.CaseAllowedBinding.Claims, result.Claims) ||
		!reflect.DeepEqual(entry.CaseAllowedBinding.Evidence, result.Evidence) || validateCurrent() != nil {
		return nil, errors.New("foreground case child submission is stale")
	}
	if entry.ExpiresAt.After(expiresAt) {
		entry.ExpiresAt = expiresAt
	}
	entry.ChildThreadID = child.ThreadID
	entry.ChildTurnID = child.TurnID
	entry.ChildContextDigest = child.ContextDigest
	entry.ChildContextEpoch = child.ContextEpoch
	entry.ChildExecutionGrantID = grant.GrantID
	entry.ChildToolCallID = toolCallID
	entry.SubmissionDigest = digest
	entry.PrivacyProjectionDigest = digest
	entry.CaseAllowedBinding = nil
	entry.CaseResult = domainjob.CloneCaseForegroundChildResultV1(&result)
	entry.ConsumptionNonce = nonce
	entry.SubmittedAt = now
	registry.entries[record.ID] = entry
	return map[string]any{
		"status": "accepted", "submissionDigest": digest,
		"privacyProjectionDigest": digest, "typedResultAccepted": true,
		"factAnswerAllowed": false, "evidenceAuthority": false, "finalGateRequired": true,
	}, nil
}

type PreparedForegroundHandoff struct {
	receipt domainjob.ForegroundChildHandoffReceiptV1
	valid   bool
}

type VerifiedForegroundHandoff struct {
	receipt             domainjob.ForegroundChildHandoffReceiptV1
	result              string
	caseResult          *domainjob.CaseForegroundChildResultV1
	parentContextDigest string
	valid               bool
}

// ForegroundParentResultCapabilityV1 is an unforgeable-in-data process-local
// witness created only after the live registry capability was consumed.
type ForegroundParentResultCapabilityV1 struct {
	mu                  sync.Mutex
	receipt             domainjob.ForegroundChildHandoffReceiptV1
	result              string
	caseResult          *domainjob.CaseForegroundChildResultV1
	parentContextDigest string
	caseOpened          bool
	continuationUsed    bool
	valid               bool
}

// ForegroundChildTerminalV1 is the closed app-layer projection of one exact
// host-committed ordinary child terminal. The digest identifies the canonical
// general-terminal outbox winner; it is audit metadata only and never evidence
// or case-fact authority.
type ForegroundChildTerminalV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	TerminalDigest  string
	TerminalStatus  string
	TerminalReason  string
}

type ForegroundChildTerminalResolver func(string, string) (ForegroundChildTerminalV1, bool)

type ForegroundCaseCompletionVerifier interface {
	Rehydrate(context.Context, domainjob.Record) (VerifiedChildCompletion, error)
}

type ForegroundCaseResultResolver func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domainjob.CaseForegroundChildResultV1,
) error

type ForegroundCaseDelegationValidator func(
	context.Context,
	domainjob.Record,
	domainsecurity.TurnSecurityContext,
) error

// ForegroundHandoffAuthority binds the live submission to exact committed
// parent/child contexts and the child's accepted metadata-only terminal.
type ForegroundHandoffAuthority struct {
	registry               *ForegroundSubmissionRegistry
	threads                JobSecurityThreadReader
	contexts               ChildCompletionContextResolver
	validateCurrent        ChildCompletionCurrentValidator
	terminals              ForegroundChildTerminalResolver
	caseCompletions        ForegroundCaseCompletionVerifier
	resolveCase            ForegroundCaseResultResolver
	validateCaseDelegation ForegroundCaseDelegationValidator
}

func NewForegroundHandoffAuthorityWithCaseTyped(
	registry *ForegroundSubmissionRegistry,
	threads JobSecurityThreadReader,
	contexts ChildCompletionContextResolver,
	validateCurrent ChildCompletionCurrentValidator,
	terminals ForegroundChildTerminalResolver,
	caseCompletions ForegroundCaseCompletionVerifier,
	resolveCase ForegroundCaseResultResolver,
	validateCaseDelegation ForegroundCaseDelegationValidator,
) *ForegroundHandoffAuthority {
	authority := NewForegroundHandoffAuthority(registry, threads, contexts, validateCurrent, terminals)
	authority.caseCompletions = caseCompletions
	authority.resolveCase = resolveCase
	authority.validateCaseDelegation = validateCaseDelegation
	return authority
}

func NewForegroundHandoffAuthority(
	registry *ForegroundSubmissionRegistry,
	threads JobSecurityThreadReader,
	contexts ChildCompletionContextResolver,
	validateCurrent ChildCompletionCurrentValidator,
	terminals ForegroundChildTerminalResolver,
) *ForegroundHandoffAuthority {
	return &ForegroundHandoffAuthority{
		registry: registry, threads: threads, contexts: contexts,
		validateCurrent: validateCurrent, terminals: terminals,
	}
}

func (authority *ForegroundHandoffAuthority) Submit(record domainjob.Record, child domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, toolCallID, result string) (map[string]any, error) {
	if authority == nil || authority.registry == nil {
		return nil, errors.New("foreground child handoff authority is unavailable")
	}
	return authority.registry.Submit(record, child, grant, toolCallID, result)
}

func (authority *ForegroundHandoffAuthority) StageCase(
	record domainjob.Record,
	parent domainsecurity.TurnSecurityContext,
	binding domainjob.CaseDelegatedAnswerSlotBindingV1,
) error {
	if authority == nil || authority.registry == nil {
		return errors.New("foreground case allowance authority is unavailable")
	}
	return authority.registry.StageCase(record, parent, binding)
}

func (authority *ForegroundHandoffAuthority) SubmitCase(ctx context.Context, record domainjob.Record, child domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, toolCallID string, result any) (map[string]any, error) {
	if ctx == nil || authority == nil || authority.registry == nil || authority.contexts == nil || authority.validateCurrent == nil ||
		authority.caseCompletions == nil ||
		authority.resolveCase == nil || authority.validateCaseDelegation == nil {
		return nil, errors.New("foreground case child handoff authority is unavailable")
	}
	parent, parentOK := authority.contexts(record.ParentThreadID, record.ParentTurnID)
	currentChild, childOK := authority.contexts(record.ChildThreadID, child.TurnID)
	if !parentOK || !childOK || currentChild != child ||
		validateForegroundCaseContextsV1(record, parent, child) != nil ||
		authority.validateCurrent(ctx, parent) != nil || authority.validateCurrent(ctx, child) != nil ||
		authority.validateCaseDelegation(ctx, record, parent) != nil {
		return nil, errors.New("foreground case child handoff authority is stale")
	}
	selection, _, selectionErr := domainjob.ParseCaseForegroundChildSelectionV1(result, record.CaseDelegation)
	binding, bindingOK := authority.registry.caseBindingForSelection(record.ID, selection)
	hostResult, resultErr := domainjob.NewCaseForegroundChildResultV1(record.CaseDelegation, binding)
	if selectionErr != nil || !bindingOK || resultErr != nil || authority.resolveCase(ctx, parent, hostResult) != nil {
		return nil, errors.New("foreground case child selection is not current")
	}
	return authority.registry.SubmitCase(record, child, grant, toolCallID, selection, hostResult, func() error {
		if authority.validateCurrent(ctx, parent) != nil || authority.validateCurrent(ctx, child) != nil ||
			authority.validateCaseDelegation(ctx, record, parent) != nil {
			return errors.New("foreground case child context changed during submission")
		}
		return authority.resolveCase(ctx, parent, hostResult)
	})
}

func (registry *ForegroundSubmissionRegistry) caseBindingForSelection(
	childRunID string,
	selection domainjob.CaseForegroundChildSelectionV1,
) (domainjob.CaseDelegatedAnswerSlotBindingV1, bool) {
	if registry == nil || !domainsecurity.IsSHA256Hex(selection.AnswerSlotDigest) {
		return domainjob.CaseDelegatedAnswerSlotBindingV1{}, false
	}
	now := registry.currentTime()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneLocked(now)
	entry, ok := registry.entries[strings.TrimSpace(childRunID)]
	if !ok || entry.CaseAllowedBinding == nil || entry.CaseResult != nil || entry.ConsumptionNonce != "" ||
		entry.HandoffKind != ForegroundHandoffCaseTypedV1 || !now.Before(entry.ExpiresAt) {
		return domainjob.CaseDelegatedAnswerSlotBindingV1{}, false
	}
	commitment, err := domainjob.NewCaseDelegatedAnswerSlotCommitmentV1(*entry.CaseAllowedBinding)
	if err != nil || commitment.Digest != selection.AnswerSlotDigest {
		return domainjob.CaseDelegatedAnswerSlotBindingV1{}, false
	}
	binding := *entry.CaseAllowedBinding
	binding.AnswerSlot.Gaps = append([]string{}, binding.AnswerSlot.Gaps...)
	binding.Claims = append([]domainjob.CaseDelegatedClaimReferenceV1{}, binding.Claims...)
	binding.Evidence = append([]domainjob.CaseDelegatedEvidenceReferenceV1{}, binding.Evidence...)
	return binding, true
}

func (authority *ForegroundHandoffAuthority) Delete(childRunID string) {
	if authority != nil && authority.registry != nil {
		authority.registry.Delete(childRunID)
	}
}

func (authority *ForegroundHandoffAuthority) Prepare(ctx context.Context, record domainjob.Record, childTurnID string) (PreparedForegroundHandoff, error) {
	childTurnID = strings.TrimSpace(childTurnID)
	if authority == nil || authority.registry == nil || authority.threads == nil || authority.contexts == nil ||
		authority.validateCurrent == nil || ctx == nil ||
		strings.TrimSpace(record.Status) != string(domainjob.StatusRunning) || record.Background ||
		!foregroundSubmitOnlyRecord(record) || domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil || childTurnID == "" {
		return PreparedForegroundHandoff{}, errors.New("foreground child handoff authority is unavailable")
	}
	now := authority.registry.currentTime()
	if blocker := (JobSecurityAuthorizer{Threads: authority.threads}).BlockerAt(record, now); blocker != "" {
		return PreparedForegroundHandoff{}, errors.New("foreground child parent authority is stale: " + blocker)
	}
	parent, parentOK := authority.contexts(record.ParentThreadID, record.ParentTurnID)
	child, childOK := authority.contexts(record.ChildThreadID, childTurnID)
	if !parentOK || !childOK {
		return PreparedForegroundHandoff{}, errors.New("foreground child handoff contexts do not match")
	}
	caseTyped := record.CaseDelegation != nil
	childAcceptedTerminalDigest := ""
	var caseBinding *domainjob.ForegroundCaseHandoffBindingV1
	if caseTyped {
		if authority.caseCompletions == nil || authority.resolveCase == nil || authority.validateCaseDelegation == nil ||
			validateForegroundCaseContextsV1(record, parent, child) != nil || record.ChildCompletionReceipt == nil ||
			record.ChildCompletionReceipt.ChildContext.TurnID != childTurnID ||
			authority.validateCaseDelegation(ctx, record, parent) != nil {
			return PreparedForegroundHandoff{}, errors.New("foreground case child handoff contexts do not match")
		}
		childAcceptedTerminalDigest = record.ChildCompletionReceipt.AcceptedFinalDigest
	} else {
		if authority.terminals == nil {
			return PreparedForegroundHandoff{}, errors.New("foreground child terminal authority is unavailable")
		}
		childTerminal, terminalOK := authority.terminals(record.ChildThreadID, childTurnID)
		if !terminalOK || childTerminal.SecurityContext != child ||
			!domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(parent) ||
			!domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(child) ||
			!domainjob.SecurityBindingMatchesContext(record.SecurityBinding, parent) ||
			parent.WorkspaceRealPath != child.WorkspaceRealPath || parent.TenantID != child.TenantID || parent.UserID != child.UserID ||
			!validForegroundChildTerminalV1(childTerminal, child) {
			return PreparedForegroundHandoff{}, errors.New("foreground child handoff contexts do not match")
		}
		childAcceptedTerminalDigest = childTerminal.TerminalDigest
	}
	if err := authority.validateCurrent(ctx, parent); err != nil {
		return PreparedForegroundHandoff{}, errors.Join(errors.New("foreground parent context is stale"), err)
	}
	if err := authority.validateCurrent(ctx, child); err != nil {
		return PreparedForegroundHandoff{}, errors.Join(errors.New("foreground child context is stale"), err)
	}
	authority.registry.mu.Lock()
	defer authority.registry.mu.Unlock()
	authority.registry.pruneLocked(now)
	entry, ok := authority.registry.entries[record.ID]
	if !ok || entry.Consumed || entry.ChildThreadID != child.ThreadID || entry.ChildTurnID != child.TurnID ||
		entry.ChildContextDigest != child.ContextDigest || entry.ChildContextEpoch != child.ContextEpoch ||
		entry.WorkspaceRealPath != child.WorkspaceRealPath || !now.Before(entry.ExpiresAt) ||
		(caseTyped && (entry.HandoffKind != ForegroundHandoffCaseTypedV1 || entry.CaseResult == nil || entry.ProjectedResult != "")) ||
		(!caseTyped && (entry.HandoffKind != ForegroundHandoffGeneralV1 || entry.CaseResult != nil || entry.ProjectedResult == "")) {
		return PreparedForegroundHandoff{}, errors.New("foreground child live submission is unavailable")
	}
	if caseTyped {
		var bindingErr error
		caseBinding, bindingErr = domainjob.NewForegroundCaseHandoffBindingV1(
			parent, child, record.SecurityBinding, record.CaseDelegation, record.DelegatedToolManifest,
			record.ToolScope, record.ToolSchemaHash, entry.ChildExecutionGrantID, entry.ChildToolCallID,
			*record.ChildCompletionReceipt, entry.SubmissionDigest,
		)
		if bindingErr != nil {
			return PreparedForegroundHandoff{}, bindingErr
		}
	}
	receipt, err := domainjob.NewForegroundChildHandoffReceiptV1(domainjob.ForegroundChildHandoffReceiptInputV1{
		ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID, ParentRunID: parent.TurnID,
		ChildThreadID: child.ThreadID, ChildTurnID: child.TurnID, ChildRunID: record.ID,
		WorkspaceRealPath: parent.WorkspaceRealPath, ParentContextEpoch: parent.ContextEpoch, ChildContextEpoch: child.ContextEpoch,
		ParentExecutionGrantID: record.SecurityBinding.ParentExecutionGrantID, ParentToolCallID: record.SecurityBinding.ParentToolCallID,
		ChildAcceptedTerminalDigest: childAcceptedTerminalDigest,
		SubmissionDigest:            entry.SubmissionDigest, PrivacyProjectionDigest: entry.PrivacyProjectionDigest,
		ConsumptionNonce: entry.ConsumptionNonce, IssuedAt: now, ExpiresAt: entry.ExpiresAt, CaseBinding: caseBinding,
	})
	if err != nil {
		return PreparedForegroundHandoff{}, err
	}
	return PreparedForegroundHandoff{receipt: receipt, valid: true}, nil
}

func (prepared PreparedForegroundHandoff) ReceiptForPersistence() (*domainjob.ForegroundChildHandoffReceiptV1, error) {
	if !prepared.valid || domainjob.ValidateForegroundChildHandoffReceiptV1(prepared.receipt) != nil {
		return nil, errors.New("prepared foreground child handoff is unavailable")
	}
	return domainjob.CloneForegroundChildHandoffReceiptV1(&prepared.receipt), nil
}

func (authority *ForegroundHandoffAuthority) Consume(ctx context.Context, record domainjob.Record, prepared PreparedForegroundHandoff) (VerifiedForegroundHandoff, error) {
	if authority == nil || authority.registry == nil || authority.threads == nil || authority.contexts == nil ||
		authority.validateCurrent == nil || !prepared.valid || ctx == nil ||
		!domainjob.ForegroundChildHandoffReceiptsEqualV1(record.ForegroundChildHandoffReceipt, &prepared.receipt) ||
		strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) || record.Background {
		return VerifiedForegroundHandoff{}, errors.New("foreground child handoff durable state is invalid")
	}
	now := authority.registry.currentTime()
	if domainjob.ValidateForegroundChildHandoffReceiptForConsumptionV1(prepared.receipt, prepared.receipt.ConsumptionNonce, now) != nil {
		return VerifiedForegroundHandoff{}, errors.New("foreground child handoff receipt is not consumable")
	}
	if blocker := (JobSecurityAuthorizer{Threads: authority.threads}).BlockerAt(record, now); blocker != "" {
		return VerifiedForegroundHandoff{}, errors.New("foreground child parent authority changed before consumption: " + blocker)
	}
	parent, parentOK := authority.contexts(prepared.receipt.ParentThreadID, prepared.receipt.ParentTurnID)
	child, childOK := authority.contexts(prepared.receipt.ChildThreadID, prepared.receipt.ChildTurnID)
	if !parentOK || !childOK || parent.WorkspaceRealPath != prepared.receipt.WorkspaceRealPath ||
		child.WorkspaceRealPath != prepared.receipt.WorkspaceRealPath ||
		parent.ContextEpoch != prepared.receipt.ParentContextEpoch || child.ContextEpoch != prepared.receipt.ChildContextEpoch {
		return VerifiedForegroundHandoff{}, errors.New("foreground child handoff context changed before consumption")
	}
	caseTyped := prepared.receipt.CaseBinding != nil
	if caseTyped {
		if authority.caseCompletions == nil || authority.resolveCase == nil || authority.validateCaseDelegation == nil ||
			validateForegroundCaseContextsV1(record, parent, child) != nil || record.ChildCompletionReceipt == nil ||
			prepared.receipt.ChildAcceptedTerminalDigest != record.ChildCompletionReceipt.AcceptedFinalDigest ||
			authority.validateCaseDelegation(ctx, record, parent) != nil {
			return VerifiedForegroundHandoff{}, errors.New("foreground case child handoff context changed before consumption")
		}
		verifiedCompletion, completionErr := authority.caseCompletions.Rehydrate(ctx, record)
		trustedReceipt, trustedErr := verifiedCompletion.ReceiptForPersistence()
		if completionErr != nil || trustedErr != nil || trustedReceipt == nil || *trustedReceipt != *record.ChildCompletionReceipt {
			return VerifiedForegroundHandoff{}, errors.New("foreground case child accepted final is not trusted")
		}
	} else {
		if record.CaseDelegation != nil || record.ChildCompletionReceipt != nil || authority.terminals == nil ||
			!domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(parent) ||
			!domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(child) ||
			!domainjob.SecurityBindingMatchesContext(record.SecurityBinding, parent) ||
			parent.TenantID != child.TenantID || parent.UserID != child.UserID {
			return VerifiedForegroundHandoff{}, errors.New("foreground child handoff context changed before consumption")
		}
		childTerminal, terminalOK := authority.terminals(child.ThreadID, child.TurnID)
		if !terminalOK || !validForegroundChildTerminalV1(childTerminal, child) ||
			childTerminal.TerminalDigest != prepared.receipt.ChildAcceptedTerminalDigest {
			return VerifiedForegroundHandoff{}, errors.New("foreground child terminal changed before consumption")
		}
	}
	if err := authority.validateCurrent(ctx, parent); err != nil {
		return VerifiedForegroundHandoff{}, errors.Join(errors.New("foreground parent context changed before consumption"), err)
	}
	if err := authority.validateCurrent(ctx, child); err != nil {
		return VerifiedForegroundHandoff{}, errors.Join(errors.New("foreground child context changed before consumption"), err)
	}
	authority.registry.mu.Lock()
	defer authority.registry.mu.Unlock()
	entry, ok := authority.registry.entries[record.ID]
	if !ok || entry.Consumed || entry.ConsumptionNonce != prepared.receipt.ConsumptionNonce ||
		entry.SubmissionDigest != prepared.receipt.SubmissionDigest ||
		entry.PrivacyProjectionDigest != prepared.receipt.PrivacyProjectionDigest ||
		entry.ChildThreadID != prepared.receipt.ChildThreadID || entry.ChildTurnID != prepared.receipt.ChildTurnID ||
		!now.Before(entry.ExpiresAt) ||
		(caseTyped && (entry.HandoffKind != ForegroundHandoffCaseTypedV1 || entry.CaseResult == nil || entry.ProjectedResult != "")) ||
		(!caseTyped && (entry.HandoffKind != ForegroundHandoffGeneralV1 || entry.CaseResult != nil || entry.ProjectedResult == "")) {
		return VerifiedForegroundHandoff{}, errors.New("foreground child handoff live capability is invalid")
	}
	if caseTyped {
		wantBinding, bindingErr := domainjob.NewForegroundCaseHandoffBindingV1(
			parent, child, record.SecurityBinding, record.CaseDelegation, record.DelegatedToolManifest,
			record.ToolScope, record.ToolSchemaHash, entry.ChildExecutionGrantID, entry.ChildToolCallID,
			*record.ChildCompletionReceipt, prepared.receipt.SubmissionDigest,
		)
		if bindingErr != nil || wantBinding == nil || *wantBinding != *prepared.receipt.CaseBinding {
			return VerifiedForegroundHandoff{}, errors.New("foreground case child handoff binding changed before consumption")
		}
		if domainjob.ValidateCaseForegroundChildResultV1(*entry.CaseResult, record.CaseDelegation) != nil ||
			authority.resolveCase(ctx, parent, *entry.CaseResult) != nil {
			return VerifiedForegroundHandoff{}, errors.New("foreground case child result is not current")
		}
		result := domainjob.CloneCaseForegroundChildResultV1(entry.CaseResult)
		entry.Consumed = true
		entry.CaseResult = nil
		authority.registry.entries[record.ID] = entry
		return VerifiedForegroundHandoff{
			receipt: prepared.receipt, caseResult: result, parentContextDigest: parent.ContextDigest, valid: true,
		}, nil
	}
	result := entry.ProjectedResult
	entry.Consumed = true
	entry.ProjectedResult = ""
	authority.registry.entries[record.ID] = entry
	return VerifiedForegroundHandoff{
		receipt: prepared.receipt, result: result, parentContextDigest: parent.ContextDigest, valid: true,
	}, nil
}

func validateForegroundCaseContextsV1(
	record domainjob.Record,
	parent domainsecurity.TurnSecurityContext,
	child domainsecurity.TurnSecurityContext,
) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(child) != nil ||
		domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil ||
		!domainjob.SecurityBindingMatchesContext(record.SecurityBinding, parent) ||
		domainjob.ValidateCaseDelegationContextV1(record.CaseDelegation, record.SecurityBinding) != nil ||
		domainjob.ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil ||
		parent.ThreadID != record.ParentThreadID || parent.TurnID != record.ParentTurnID ||
		record.ParentToolCallID != record.SecurityBinding.ParentToolCallID ||
		child.ThreadID != record.ChildThreadID || parent.ThreadID == child.ThreadID ||
		parent.WorkspaceRealPath != child.WorkspaceRealPath || parent.TenantID != child.TenantID || parent.UserID != child.UserID ||
		parent.CaseID != child.CaseID || parent.CaseBindingHash != child.CaseBindingHash ||
		parent.DatasetSnapshotID != child.DatasetSnapshotID || parent.SourceManifestHash != child.SourceManifestHash {
		return errors.New("foreground case child contexts are invalid")
	}
	return nil
}

func validForegroundChildTerminalV1(terminal ForegroundChildTerminalV1, child domainsecurity.TurnSecurityContext) bool {
	return terminal.SecurityContext == child &&
		domainsecurity.ValidateTurnSecurityContextForExecution(child) == nil &&
		domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(child) &&
		domainsecurity.IsSHA256Hex(terminal.TerminalDigest) &&
		terminal.TerminalStatus == "completed" && terminal.TerminalReason == "success"
}

func (verified VerifiedForegroundHandoff) OutputProjection(record domainjob.Record) map[string]any {
	if !verified.valid || !domainjob.ForegroundChildHandoffReceiptsEqualV1(record.ForegroundChildHandoffReceipt, &verified.receipt) {
		return ChildOutputMetadataProjection(record)
	}
	if verified.caseResult != nil && verified.result == "" && verified.receipt.CaseBinding != nil {
		capability := &ForegroundParentResultCapabilityV1{
			receipt: verified.receipt, caseResult: domainjob.CloneCaseForegroundChildResultV1(verified.caseResult),
			parentContextDigest: verified.parentContextDigest, valid: true,
		}
		return map[string]any{
			"kind": "subagent_task", "childRunId": record.ID, "jobId": record.ID,
			"status":                       string(domainjob.StatusCompleted),
			"handoffReceiptDigest":         verified.receipt.ReceiptDigest,
			"submissionDigest":             verified.receipt.SubmissionDigest,
			"privacyProjectionDigest":      verified.receipt.PrivacyProjectionDigest,
			"childCompletionReceiptDigest": verified.receipt.CaseBinding.ChildCompletionReceiptDigest,
			"typedResultAccepted":          true, "finalGateRequired": true,
			"factAnswerAllowed": false, "evidenceAuthority": false,
			"parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
			ForegroundParentCapabilityFieldV1: capability,
		}
	}
	if verified.result == "" || verified.caseResult != nil || verified.receipt.CaseBinding != nil {
		return ChildOutputMetadataProjection(record)
	}
	capability := &ForegroundParentResultCapabilityV1{
		receipt: verified.receipt, result: verified.result,
		parentContextDigest: verified.parentContextDigest, valid: true,
	}
	return map[string]any{
		"kind": "subagent_task", "childRunId": record.ID, "jobId": record.ID,
		"status": string(domainjob.StatusCompleted), "result": verified.result,
		"handoffReceiptDigest":    verified.receipt.ReceiptDigest,
		"submissionDigest":        verified.receipt.SubmissionDigest,
		"privacyProjectionDigest": verified.receipt.PrivacyProjectionDigest,
		"factAnswerAllowed":       false, "evidenceAuthority": false,
		"parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
		ForegroundParentCapabilityFieldV1: capability,
	}
}

// OpenForParent proves that an in-memory result belongs to the exact current
// parent context, execution grant and tool call. It never grants evidence,
// case-fact, Goal, or Todo completion authority.
func (capability *ForegroundParentResultCapabilityV1) OpenForParent(
	parent domainsecurity.TurnSecurityContext,
	parentExecutionGrantID string,
	parentToolCallID string,
	output map[string]any,
) (string, bool) {
	if capability == nil || !capability.valid || domainjob.ValidateForegroundChildHandoffReceiptV1(capability.receipt) != nil ||
		capability.receipt.CaseBinding != nil || capability.caseResult != nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(parent) != nil ||
		!domainsecurity.TurnSecurityContextUsesHostGeneralOnlyRisk(parent) ||
		capability.parentContextDigest != parent.ContextDigest ||
		capability.receipt.ParentThreadID != parent.ThreadID || capability.receipt.ParentTurnID != parent.TurnID ||
		capability.receipt.ParentRunID != parent.TurnID || capability.receipt.WorkspaceRealPath != parent.WorkspaceRealPath ||
		capability.receipt.ParentContextEpoch != parent.ContextEpoch ||
		capability.receipt.ParentExecutionGrantID != strings.TrimSpace(parentExecutionGrantID) ||
		capability.receipt.ParentToolCallID != strings.TrimSpace(parentToolCallID) {
		return "", false
	}
	result, resultOK := output["result"].(string)
	childRunID, childRunOK := output["childRunId"].(string)
	if !resultOK || !childRunOK || result != capability.result || childRunID != capability.receipt.ChildRunID ||
		foregroundHandoffString(output["jobId"]) != childRunID ||
		foregroundHandoffString(output["handoffReceiptDigest"]) != capability.receipt.ReceiptDigest ||
		foregroundHandoffString(output["submissionDigest"]) != capability.receipt.SubmissionDigest ||
		foregroundHandoffString(output["privacyProjectionDigest"]) != capability.receipt.PrivacyProjectionDigest ||
		domainsecurity.SHA256Hex([]byte(result)) != capability.receipt.PrivacyProjectionDigest ||
		domainsecurity.ContainsProtectedCaseFactCandidate(result) {
		return "", false
	}
	public, err := domainevent.FilterPublicText(result)
	if err != nil || strings.TrimSpace(domainordinary.ProjectTextV1(public)) != result {
		return "", false
	}
	return result, true
}

func (capability *ForegroundParentResultCapabilityV1) OpenCaseForParent(
	parent domainsecurity.TurnSecurityContext,
	parentExecutionGrantID string,
	parentToolCallID string,
	output map[string]any,
) (domainjob.CaseForegroundChildResultV1, bool) {
	if capability == nil {
		return domainjob.CaseForegroundChildResultV1{}, false
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	result, valid := capability.inspectCaseForParentLocked(parent, parentExecutionGrantID, parentToolCallID, output)
	if !valid {
		return domainjob.CaseForegroundChildResultV1{}, false
	}
	capability.caseOpened = true
	capability.caseResult = nil
	return result, true
}

// VerifyParentContinuation proves that the exact process-local case carrier
// was opened once for the current parent provider attempt. The durable child
// completion receipt alone cannot implement this capability after restart.
func (capability *ForegroundParentResultCapabilityV1) VerifyParentContinuation(
	parent domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	toolCallID string,
) (string, bool) {
	if capability == nil ||
		domainsecurity.ValidateExecutionGrantForContext(grant, parent) != nil ||
		grant.ToolCallID != strings.TrimSpace(toolCallID) {
		return "", false
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.valid || !capability.caseOpened || capability.continuationUsed || capability.caseResult != nil || capability.result != "" ||
		capability.receipt.CaseBinding == nil ||
		domainjob.ValidateForegroundChildHandoffReceiptV1(capability.receipt) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) != nil ||
		capability.parentContextDigest != parent.ContextDigest ||
		capability.receipt.ParentThreadID != parent.ThreadID || capability.receipt.ParentTurnID != parent.TurnID ||
		capability.receipt.ParentRunID != parent.TurnID || capability.receipt.WorkspaceRealPath != parent.WorkspaceRealPath ||
		capability.receipt.ParentContextEpoch != parent.ContextEpoch ||
		capability.receipt.ParentExecutionGrantID != grant.GrantID ||
		capability.receipt.ParentToolCallID != strings.TrimSpace(toolCallID) {
		return "", false
	}
	capability.continuationUsed = true
	return capability.receipt.ReceiptDigest, true
}

func (capability *ForegroundParentResultCapabilityV1) inspectCaseForParent(
	parent domainsecurity.TurnSecurityContext,
	parentExecutionGrantID string,
	parentToolCallID string,
	output map[string]any,
) (domainjob.CaseForegroundChildResultV1, bool) {
	if capability == nil {
		return domainjob.CaseForegroundChildResultV1{}, false
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	return capability.inspectCaseForParentLocked(parent, parentExecutionGrantID, parentToolCallID, output)
}

func (capability *ForegroundParentResultCapabilityV1) inspectCaseForParentLocked(
	parent domainsecurity.TurnSecurityContext,
	parentExecutionGrantID string,
	parentToolCallID string,
	output map[string]any,
) (domainjob.CaseForegroundChildResultV1, bool) {
	if !capability.valid || capability.caseOpened || capability.caseResult == nil || capability.result != "" ||
		domainjob.ValidateForegroundChildHandoffReceiptV1(capability.receipt) != nil || capability.receipt.CaseBinding == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(parent) != nil ||
		capability.parentContextDigest != parent.ContextDigest ||
		capability.receipt.ParentThreadID != parent.ThreadID || capability.receipt.ParentTurnID != parent.TurnID ||
		capability.receipt.ParentRunID != parent.TurnID || capability.receipt.WorkspaceRealPath != parent.WorkspaceRealPath ||
		capability.receipt.ParentContextEpoch != parent.ContextEpoch ||
		capability.receipt.ParentExecutionGrantID != strings.TrimSpace(parentExecutionGrantID) ||
		capability.receipt.ParentToolCallID != strings.TrimSpace(parentToolCallID) {
		return domainjob.CaseForegroundChildResultV1{}, false
	}
	childRunID, childRunOK := output["childRunId"].(string)
	canonical, canonicalErr := domainjob.CanonicalCaseForegroundChildResultV1(*capability.caseResult)
	if !childRunOK || canonicalErr != nil ||
		childRunID != capability.receipt.ChildRunID || foregroundHandoffString(output["jobId"]) != childRunID ||
		foregroundHandoffString(output["handoffReceiptDigest"]) != capability.receipt.ReceiptDigest ||
		foregroundHandoffString(output["submissionDigest"]) != capability.receipt.SubmissionDigest ||
		foregroundHandoffString(output["privacyProjectionDigest"]) != capability.receipt.PrivacyProjectionDigest ||
		foregroundHandoffString(output["childCompletionReceiptDigest"]) != capability.receipt.CaseBinding.ChildCompletionReceiptDigest ||
		domainsecurity.SHA256Hex(canonical) != capability.receipt.CaseBinding.TypedResultDigest {
		return domainjob.CaseForegroundChildResultV1{}, false
	}
	return *domainjob.CloneCaseForegroundChildResultV1(capability.caseResult), true
}

// IsForegroundHandoffPendingToolCallV1 identifies the sole top-level bounded
// foreground delegation shape. Parent TSC authority selects either the
// unchanged general lane or the case-typed lane; provider arguments cannot.
func IsForegroundHandoffPendingToolCallV1(pending modelapp.PendingToolCall) bool {
	return toolcatalogapp.HostForegroundTaskCallForContextV1(
		pending.Call, pending.SubagentDepth, pending.SecurityContext,
	)
}

// ProjectForegroundParentToolOutputV1 recognizes only the exact process-local
// capability produced after live handoff consumption. Private result text may
// continue to the current parent provider call, while durable/SSE/history
// projections retain bounded metadata and digests only.
func ProjectForegroundParentToolOutputV1(
	pending modelapp.PendingToolCall,
	output any,
) (private map[string]any, public map[string]any, applicable bool, valid bool) {
	return projectForegroundParentToolOutputV1(pending, output, true)
}

// ProjectForegroundParentPublicToolOutputV1 validates the same exact live
// capability without opening its case carrier. It exists solely for the
// durable/SSE/history projection that precedes the one current provider
// attempt; the private projector above burns the carrier exactly once.
func ProjectForegroundParentPublicToolOutputV1(
	pending modelapp.PendingToolCall,
	output any,
) (public map[string]any, applicable bool, valid bool) {
	_, public, applicable, valid = projectForegroundParentToolOutputV1(pending, output, false)
	return public, applicable, valid
}

func projectForegroundParentToolOutputV1(
	pending modelapp.PendingToolCall,
	output any,
	openPrivateCase bool,
) (private map[string]any, public map[string]any, applicable bool, valid bool) {
	if !IsForegroundHandoffPendingToolCallV1(pending) {
		return nil, nil, false, false
	}
	invalid := domaintoolresult.ForegroundHandoffInvalidPublicOutputV1()
	record, ok := output.(map[string]any)
	capability, capabilityOK := record[ForegroundParentCapabilityFieldV1].(*ForegroundParentResultCapabilityV1)
	if !ok || !capabilityOK ||
		domainsecurity.ValidateExecutionGrantForContext(pending.ExecutionGrant, pending.SecurityContext) != nil ||
		pending.ExecutionGrant.ToolName != pending.Call.Name || pending.ExecutionGrant.ToolCallID != pending.Call.ID ||
		foregroundHandoffString(record["kind"]) != "subagent_task" ||
		foregroundHandoffString(record["status"]) != string(domainjob.StatusCompleted) ||
		foregroundHandoffString(record["childRunId"]) == "" ||
		foregroundHandoffString(record["jobId"]) != foregroundHandoffString(record["childRunId"]) ||
		!domainsecurity.IsSHA256Hex(foregroundHandoffString(record["handoffReceiptDigest"])) ||
		!domainsecurity.IsSHA256Hex(foregroundHandoffString(record["submissionDigest"])) ||
		!domainsecurity.IsSHA256Hex(foregroundHandoffString(record["privacyProjectionDigest"])) ||
		!exactFalseForegroundParentOutputFieldsV1(record, "factAnswerAllowed", "evidenceAuthority", "parentGoalCompletionAllowed", "parentTodoCompletionAllowed") {
		return nil, invalid, true, false
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(pending.SecurityContext) == nil {
		if !exactForegroundCaseParentOutputFieldsV1(record) ||
			!domainsecurity.IsSHA256Hex(foregroundHandoffString(record["childCompletionReceiptDigest"])) ||
			!exactTrueForegroundParentOutputFieldsV1(record, "typedResultAccepted", "finalGateRequired") {
			return nil, invalid, true, false
		}
		var result domainjob.CaseForegroundChildResultV1
		var opened bool
		if openPrivateCase {
			result, opened = capability.OpenCaseForParent(
				pending.SecurityContext, pending.ExecutionGrant.GrantID, pending.Call.ID, record,
			)
		} else {
			result, opened = capability.inspectCaseForParent(
				pending.SecurityContext, pending.ExecutionGrant.GrantID, pending.Call.ID, record,
			)
		}
		if !opened {
			return nil, invalid, true, false
		}
		public = map[string]any{
			"kind": record["kind"], "childRunId": record["childRunId"], "jobId": record["jobId"], "status": record["status"],
			"handoffReceiptDigest": record["handoffReceiptDigest"], "submissionDigest": record["submissionDigest"],
			"privacyProjectionDigest":      record["privacyProjectionDigest"],
			"childCompletionReceiptDigest": record["childCompletionReceiptDigest"],
			"typedResultAccepted":          true, "finalGateRequired": true,
			"answerSlotCount": len(result.AnswerSlots), "gapCount": len(result.Gaps),
			"factAnswerAllowed": false, "evidenceAuthority": false,
			"parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
		}
		if !openPrivateCase {
			return nil, public, true, true
		}
		private = cloneMap(public)
		private["caseResult"] = result
		return private, public, true, true
	}
	if !exactForegroundParentOutputFieldsV1(record) {
		return nil, invalid, true, false
	}
	result, opened := capability.OpenForParent(
		pending.SecurityContext, pending.ExecutionGrant.GrantID, pending.Call.ID, record,
	)
	if !opened || strings.TrimSpace(result) == "" || len([]byte(result)) > toolcatalogapp.ForegroundMaxSubmissionBytes {
		return nil, invalid, true, false
	}
	public = map[string]any{
		"kind": record["kind"], "childRunId": record["childRunId"], "jobId": record["jobId"], "status": record["status"],
		"handoffReceiptDigest": record["handoffReceiptDigest"], "submissionDigest": record["submissionDigest"],
		"privacyProjectionDigest": record["privacyProjectionDigest"], "factAnswerAllowed": false,
		"evidenceAuthority": false, "parentGoalCompletionAllowed": false, "parentTodoCompletionAllowed": false,
	}
	private = cloneMap(public)
	private["result"] = result
	if !openPrivateCase {
		return nil, public, true, true
	}
	return private, public, true, true
}

func exactForegroundCaseParentOutputFieldsV1(record map[string]any) bool {
	if len(record) != 15 {
		return false
	}
	allowed := map[string]bool{
		"kind": true, "childRunId": true, "jobId": true, "status": true,
		"handoffReceiptDigest": true, "submissionDigest": true, "privacyProjectionDigest": true,
		"childCompletionReceiptDigest": true, "typedResultAccepted": true, "finalGateRequired": true,
		"factAnswerAllowed": true, "evidenceAuthority": true,
		"parentGoalCompletionAllowed": true, "parentTodoCompletionAllowed": true,
		ForegroundParentCapabilityFieldV1: true,
	}
	for key := range record {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func exactForegroundParentOutputFieldsV1(record map[string]any) bool {
	if len(record) != 13 {
		return false
	}
	allowed := map[string]bool{
		"kind": true, "childRunId": true, "jobId": true, "status": true, "result": true,
		"handoffReceiptDigest": true, "submissionDigest": true, "privacyProjectionDigest": true,
		"factAnswerAllowed": true, "evidenceAuthority": true,
		"parentGoalCompletionAllowed": true, "parentTodoCompletionAllowed": true,
		ForegroundParentCapabilityFieldV1: true,
	}
	for key := range record {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func exactFalseForegroundParentOutputFieldsV1(record map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := record[key].(bool)
		if !ok || value {
			return false
		}
	}
	return true
}

func exactTrueForegroundParentOutputFieldsV1(record map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := record[key].(bool)
		if !ok || !value {
			return false
		}
	}
	return true
}

func foregroundHandoffString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func (registry *ForegroundSubmissionRegistry) Delete(childRunID string) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	delete(registry.entries, strings.TrimSpace(childRunID))
	registry.mu.Unlock()
}

func (registry *ForegroundSubmissionRegistry) currentTime() time.Time {
	if registry != nil && registry.now != nil {
		return registry.now().UTC()
	}
	return time.Now().UTC()
}

func (registry *ForegroundSubmissionRegistry) pruneLocked(now time.Time) {
	for key, entry := range registry.entries {
		if !now.Before(entry.ExpiresAt) {
			delete(registry.entries, key)
		}
	}
}

func foregroundConsumptionNonce() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", errors.New("foreground child handoff nonce authority is unavailable")
	}
	return domainsecurity.SHA256Hex(value), nil
}

func foregroundSubmitOnlyRecord(record domainjob.Record) bool {
	return len(record.ToolScope) == 1 && strings.TrimSpace(record.ToolScope[0]) == toolcatalogForegroundSubmitToolName
}
