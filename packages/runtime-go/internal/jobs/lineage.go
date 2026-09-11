package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type Manager struct {
	mu                    sync.Mutex
	root                  string
	artifactAuthorityRoot string
	completionVerifier    ChildCompletionReceiptVerifier
	seq                   int
	jobs                  []Record
	locked                map[string]bool
	restartPreserved      *restartPreservationV1
	restartEffectsStarted bool
	pendingSteerStarts    map[string]*pendingSteerStartChainV1
}

// ChildCompletionReceiptVerifier anchors a persisted receipt to host-trusted
// authority. A receipt's embedded self-signature is never sufficient on its
// own because an attacker can mint a new key and recompute the JSON record.
type ChildCompletionReceiptVerifier interface {
	VerifyStoredChildCompletion(context.Context, domainjob.Record) error
}

type Record = domainjob.Record
type StartRequest = domainjob.StartRequest
type UpdateRequest = domainjob.UpdateRequest

const (
	defaultTaskJobLeaseOwner   = "analytix-runtime"
	defaultTaskJobStaleAfter   = 30 * time.Second
	defaultTaskJobLeaseTimeout = 5 * time.Minute
)

func defaultRuntimeLeaseOwner() string {
	pid := os.Getpid()
	if pid <= 0 {
		return defaultTaskJobLeaseOwner
	}
	return fmt.Sprintf("%s:%d", defaultTaskJobLeaseOwner, pid)
}

func NewManager(root string) (*Manager, error) {
	return newManager(root, root, nil, nil, false)
}

func NewManagerWithChildCompletionVerifier(root string, verifier ChildCompletionReceiptVerifier, floors ...domainpendingwork.ChildIdentityFloorsV1) (*Manager, error) {
	return newManager(root, root, verifier, nil, false, floors...)
}

// NewManagerForSemanticStartup reads and writes only the private stage while
// validating/persisting artifact identities against the eventual live root.
func NewManagerForSemanticStartup(stageRoot string, authorityRoot string) (*Manager, error) {
	return newManager(stageRoot, authorityRoot, nil, nil, true)
}

func NewManagerForSemanticStartupWithChildCompletionVerifier(stageRoot string, authorityRoot string, verifier ChildCompletionReceiptVerifier) (*Manager, error) {
	return newManager(stageRoot, authorityRoot, verifier, nil, true)
}

func NewManagerForSemanticStartupWithWitness(
	stageRoot string,
	authorityRoot string,
	completionVerifier ChildCompletionReceiptVerifier,
	legacyLineageWitness *FrozenLegacyTypeScriptLineageWitnessV1,
	floors ...domainpendingwork.ChildIdentityFloorsV1,
) (*Manager, error) {
	return newManager(stageRoot, authorityRoot, completionVerifier, legacyLineageWitness, true, floors...)
}

func newManager(
	root string,
	artifactAuthorityRoot string,
	verifier ChildCompletionReceiptVerifier,
	legacyLineageWitness *FrozenLegacyTypeScriptLineageWitnessV1,
	normalizeSemanticStage bool,
	floorInputs ...domainpendingwork.ChildIdentityFloorsV1,
) (*Manager, error) {
	return newManagerWithRestartPreservationV1(context.Background(), root, artifactAuthorityRoot, verifier, legacyLineageWitness, normalizeSemanticStage, nil, floorInputs...)
}

func newManagerWithRestartPreservationV1(ctx context.Context, root, artifactAuthorityRoot string, verifier ChildCompletionReceiptVerifier, legacyLineageWitness *FrozenLegacyTypeScriptLineageWitnessV1, normalizeSemanticStage bool, preserved *RestartPreservationInputV1, floorInputs ...domainpendingwork.ChildIdentityFloorsV1) (*Manager, error) {
	if ctx == nil {
		return nil, errors.New("child-run constructor context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	floors, err := domainpendingwork.MergeChildIdentityFloorsV1(floorInputs...)
	if err != nil {
		return nil, err
	}
	manager := &Manager{root: strings.TrimSpace(root), completionVerifier: verifier, locked: map[string]bool{}, seq: floors.JobSequence}
	if manager.root == "" {
		if preserved != nil {
			return nil, errors.New("child-run restart storage root is unavailable")
		}
		return manager, nil
	}
	canonicalRoot, err := canonicalJobRootWithoutCreate(manager.root)
	if err != nil {
		return nil, errors.New("child-run storage root is invalid")
	}
	manager.root = canonicalRoot
	authorityRoot, err := canonicalJobRootWithoutCreate(artifactAuthorityRoot)
	if err != nil {
		return nil, errors.New("child-run artifact authority root is invalid")
	}
	manager.artifactAuthorityRoot = authorityRoot
	manager.restartPreserved, err = prepareConstructorRestartPreservationV1(ctx, manager.root, preserved)
	if err != nil {
		return nil, err
	}
	if manager.restartPreserved != nil {
		for id := range manager.restartPreserved.jobIDs {
			if reserved := sequence(id); reserved > manager.seq {
				manager.seq = reserved
			}
		}
	}
	records, seq, err := readRecords(ctx, manager.root, manager.artifactAuthorityRoot, verifier, legacyLineageWitness, normalizeSemanticStage, manager.seq, manager.restartPreserved)
	if err != nil {
		return nil, err
	}
	manager.jobs = records
	manager.seq = seq
	return manager, nil
}

func (m *Manager) LockChildRun(id string) (func(), error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("child run id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for _, record := range m.jobs {
		if record.ID == id && m.restartPreservesRecordNoLock(record) {
			return nil, ErrRestartPreserved
		}
	}
	if m.locked == nil {
		m.locked = map[string]bool{}
	}
	if m.locked[id] {
		return nil, fmt.Errorf("subagent reference %q is already running; retry after it finishes", id)
	}
	m.locked[id] = true
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.locked, id)
	}, nil
}

func (m *Manager) Start(parentGoalID, parentThreadID, kind string) (Record, error) {
	return m.StartChildRun(StartRequest{
		ParentGoalID:   parentGoalID,
		ParentThreadID: parentThreadID,
		Kind:           kind,
	})
}

func (m *Manager) StartChildRun(request StartRequest) (Record, error) {
	return m.startChildRun(context.Background(), request, nil)
}

func (m *Manager) startChildRun(ctx context.Context, request StartRequest, reservation *ChildRunReservationV1) (Record, error) {
	if ctx == nil || ctx.Err() != nil {
		return Record{}, errors.New("child-run start context is unavailable")
	}
	request.ParentGoalID = strings.TrimSpace(request.ParentGoalID)
	request.ParentThreadID = strings.TrimSpace(request.ParentThreadID)
	if request.ParentGoalID == legacyTypeScriptParentGoalIDV1 ||
		strings.HasPrefix(strings.TrimSpace(request.SourceRef), legacyTypeScriptSourceRefV1) {
		return Record{}, errors.New("retired TypeScript child-run namespace is migration-only")
	}
	if request.ParentGoalID == "" || request.ParentThreadID == "" {
		return Record{}, errors.New("parent goal and thread lineage are required")
	}
	if strings.TrimSpace(request.Kind) == "" {
		request.Kind = "child-run"
	}
	if err := domainmodel.ValidateReasoningEffortV1(request.Effort); err != nil {
		return Record{}, err
	}
	if request.SecurityBinding != nil {
		if err := domainjob.ValidateSecurityBinding(request.SecurityBinding); err != nil ||
			request.SecurityBinding.ParentThreadID != request.ParentThreadID || request.SecurityBinding.ParentTurnID != strings.TrimSpace(request.ParentTurnID) ||
			request.SecurityBinding.ParentToolCallID != strings.TrimSpace(request.ParentToolCallID) {
			return Record{}, errors.New("job security binding does not match parent lineage")
		}
		if strings.TrimSpace(request.Kind) == "subagent" && request.Output != "" {
			return Record{}, errors.New("security-bound child output cannot be written")
		}
		if strings.TrimSpace(request.Kind) == "subagent" && request.Usage != nil {
			return Record{}, errors.New("security-bound child usage cannot be written to the job record")
		}
	}
	if err := domainjob.ValidatePersistedModelExecutionV1(request.ModelExecution); err != nil {
		return Record{}, err
	}
	if err := domainjob.ValidatePersistedUsageV1(request.Kind, request.Usage); err != nil {
		return Record{}, err
	}
	status := strings.TrimSpace(request.Status)
	if status == "" {
		status = "completed"
	}
	if err := domainjob.ValidateStatusV1(status); err != nil {
		return Record{}, err
	}
	if strings.TrimSpace(request.Kind) == "subagent" && request.SecurityBinding != nil &&
		request.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID && status == string(domainjob.StatusCompleted) {
		return Record{}, errors.New("case-bound child completion requires a trusted completion receipt")
	}
	securityBoundSubagent := strings.TrimSpace(request.Kind) == "subagent" && request.SecurityBinding != nil
	caseBoundSubagent := securityBoundSubagent && request.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID
	if caseBoundSubagent && (status == string(domainjob.StatusQueued) || status == string(domainjob.StatusRunning)) {
		if err := domainjob.ValidateExecutableCaseDelegationV1(
			request.Kind, request.Name, request.Label, request.Prompt, request.SecurityBinding, request.CaseDelegation,
		); err != nil {
			return Record{}, err
		}
	} else if !caseBoundSubagent && request.CaseDelegation != nil {
		return Record{}, errors.New("job case delegation is outside a case-bound subagent")
	}
	if securityBoundSubagent && (status == string(domainjob.StatusQueued) || status == string(domainjob.StatusRunning)) &&
		domainjob.ValidateDelegatedToolManifestV1(request.DelegatedToolManifest, request.ToolScope, request.ToolSchemaHash) != nil {
		return Record{}, errors.New("security-bound child delegated tool manifest is invalid")
	}
	failureCode := strings.TrimSpace(request.FailureCode)
	if !domainjob.ValidFailureCode(failureCode) {
		return Record{}, errors.New("child-run failure code is invalid")
	}
	failureCode = domainjob.NormalizeFailureCode(failureCode, status, strings.TrimSpace(request.Error) != "")
	request.Output = domainjob.ProjectPersistableUntrustedOutputV1(request.Output)
	request.Error = ""
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m.restartPreservesRecordNoLock(Record{ParentThreadID: request.ParentThreadID, ChildThreadID: request.ChildThreadID}) {
		return Record{}, ErrRestartPreserved
	}
	childRuns, maxChildSeq := m.childRunStatsNoLock(request.ParentThreadID)
	if request.MaxChildRunsSet && request.MaxChildRuns >= 0 {
		if childRuns >= request.MaxChildRuns {
			return Record{}, fmt.Errorf("subagent child run limit reached: maxChildRuns=%d", request.MaxChildRuns)
		}
	}
	if maxChildSeq == maxChildRunSequence {
		return Record{}, errors.New("child-run lineage counter is exhausted")
	}
	jobID, err := m.consumeChildRunIDNoLock(reservation)
	if err != nil {
		return Record{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	queuedAt := strings.TrimSpace(request.QueuedAt)
	if queuedAt == "" {
		queuedAt = now
	}
	startedAt := now
	queuedMs := elapsedMilliseconds(queuedAt, now)
	if status == "queued" {
		startedAt = ""
		queuedMs = 0
	}
	record := Record{
		ID:                    jobID,
		ChildSeq:              maxChildSeq + 1,
		ParentGoalID:          request.ParentGoalID,
		ParentGoalObjective:   strings.TrimSpace(request.ParentGoalObjective),
		ParentThreadID:        request.ParentThreadID,
		ParentTurnID:          strings.TrimSpace(request.ParentTurnID),
		ParentToolItemID:      strings.TrimSpace(request.ParentToolItemID),
		ParentToolCallID:      strings.TrimSpace(request.ParentToolCallID),
		SecurityBinding:       domainjob.CloneSecurityBinding(request.SecurityBinding),
		CaseDelegation:        domainjob.CloneCaseDelegationContextV1(request.CaseDelegation),
		ChildThreadID:         strings.TrimSpace(request.ChildThreadID),
		ChildTurnID:           strings.TrimSpace(request.ChildTurnID),
		Kind:                  strings.TrimSpace(request.Kind),
		Name:                  strings.TrimSpace(request.Name),
		Label:                 strings.TrimSpace(request.Label),
		Prompt:                strings.TrimSpace(request.Prompt),
		Status:                status,
		LineageKey:            request.ParentGoalID + "/" + request.ParentThreadID,
		Workspace:             strings.TrimSpace(request.Workspace),
		Model:                 strings.TrimSpace(request.Model),
		ProviderID:            strings.TrimSpace(request.ProviderID),
		EndpointFormat:        strings.TrimSpace(request.EndpointFormat),
		Variant:               strings.TrimSpace(request.Variant),
		ModelSource:           strings.TrimSpace(request.ModelSource),
		ModelExecution:        cloneUsage(request.ModelExecution),
		Effort:                request.Effort,
		MaxModelSteps:         cloneOptionalInt(request.MaxModelSteps),
		ProfileName:           strings.TrimSpace(request.ProfileName),
		ProfileSource:         strings.TrimSpace(request.ProfileSource),
		ToolPolicy:            strings.TrimSpace(request.ToolPolicy),
		ToolScope:             sortedStrings(request.ToolScope),
		ToolSchemaHash:        strings.TrimSpace(request.ToolSchemaHash),
		DelegatedToolManifest: domainjob.CloneDelegatedToolManifestV1(request.DelegatedToolManifest),
		SystemPromptHash:      strings.TrimSpace(request.SystemPromptHash),
		SkillPackageDigest:    strings.TrimSpace(request.SkillPackageDigest),
		ProfileMode:           strings.TrimSpace(request.ProfileMode),
		ProfileDescription:    strings.TrimSpace(request.ProfileDescription),
		ProfileColor:          strings.TrimSpace(request.ProfileColor),
		ProfileIcon:           strings.TrimSpace(request.ProfileIcon),
		ReturnFormat:          strings.TrimSpace(request.ReturnFormat),
		TokenBudget:           request.TokenBudget,
		TimeBudgetMs:          request.TimeBudgetMs,
		DefaultModelInherited: request.DefaultModelInherited,
		ParallelGroupID:       strings.TrimSpace(request.ParallelGroupID),
		ParallelIndex:         request.ParallelIndex,
		ContinueFrom:          strings.TrimSpace(request.ContinueFrom),
		ForkFrom:              strings.TrimSpace(request.ForkFrom),
		SourceRef:             strings.TrimSpace(request.SourceRef),
		Background:            request.Background,
		AutoContinueParent:    request.AutoContinueParent,
		IsolationMode:         strings.TrimSpace(request.IsolationMode),
		WorktreePath:          strings.TrimSpace(request.WorktreePath),
		WorktreeBranch:        strings.TrimSpace(request.WorktreeBranch),
		BaseCommit:            strings.TrimSpace(request.BaseCommit),
		CurrentCommit:         strings.TrimSpace(request.CurrentCommit),
		ChangedFiles:          cloneChangedFiles(request.ChangedFiles),
		DiffSummary:           strings.TrimSpace(request.DiffSummary),
		MergeStatus:           strings.TrimSpace(request.MergeStatus),
		QueuedAt:              queuedAt,
		QueuedMs:              queuedMs,
		StartedAt:             startedAt,
		UpdatedAt:             now,
		LastHeartbeatAt:       strings.TrimSpace(request.LastHeartbeatAt),
		LeaseOwner:            strings.TrimSpace(request.LeaseOwner),
		LeaseExpiresAt:        strings.TrimSpace(request.LeaseExpiresAt),
		StaleAfterMs:          request.StaleAfterMs,
		Output:                request.Output,
		Error:                 strings.TrimSpace(request.Error),
		FailureCode:           failureCode,
		Usage:                 cloneUsage(request.Usage),
		ToolInvocations:       request.ToolInvocations,
	}
	applyRuntimeLeaseDefaults(&record, now)
	record.PauseState = childRunPauseState(record)
	if terminalStatus(status) {
		record.FinishedAt = now
	}
	if record.ProfileSource == "" && record.DefaultModelInherited {
		record.ProfileSource = "parent-default"
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if err := m.writeRecordNoLock(&record); err != nil {
		return Record{}, err
	}
	m.jobs = append(m.jobs, record)
	return cloneRecord(record), nil
}

func (m *Manager) childRunStatsNoLock(parentThreadID string) (int, int) {
	count := 0
	maxChildSeq := 0
	for _, record := range m.jobs {
		if strings.TrimSpace(record.ParentThreadID) != parentThreadID {
			continue
		}
		count++
		if record.ChildSeq > maxChildSeq {
			maxChildSeq = record.ChildSeq
		}
	}
	return count, maxChildSeq
}

// ReserveBackgroundAutoContinueV1 is the sole durable sibling-arbitration
// owner. The callback is pure and runs while the manager's existing global
// lock covers the current record, every sibling, and the closed-state write.
func (m *Manager) ReserveBackgroundAutoContinueV1(
	id string,
	turnID string,
	gate func(Record, []Record) string,
) (Record, bool, error) {
	id = strings.TrimSpace(id)
	turnID = strings.TrimSpace(turnID)
	if id == "" || turnID == "" || gate == nil {
		return Record{}, false, errors.New("background auto-continue reservation is incomplete")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), false, ErrRestartPreserved
		}
		if strings.TrimSpace(record.AutoContinueStatus) != "" {
			return cloneRecord(record), false, nil
		}
		if !record.Background || !record.AutoContinueParent ||
			strings.TrimSpace(record.Status) != string(domainjob.StatusCompleted) ||
			strings.TrimSpace(record.CompletionDeliveryStatus) != "delivered" ||
			strings.TrimSpace(record.CompletionDeliveryID) == "" || strings.TrimSpace(record.CompletionDeliveryItemID) == "" {
			return cloneRecord(record), false, errors.New("background auto-continue reservation record is ineligible")
		}
		original := cloneRecord(record)
		record.AutoContinueStatus = "starting"
		record.AutoContinueTurnID = turnID
		record.AutoContinueReason = ""
		reserved := true
		records := make([]Record, 0, len(m.jobs))
		for _, candidate := range m.jobs {
			records = append(records, cloneRecord(candidate))
		}
		rawReason := strings.TrimSpace(gate(cloneRecord(original), records))
		reason := domainjob.NormalizeOperationalReasonV1(rawReason)
		if rawReason != "" && (reason == "" || reason == domainjob.OperationalReasonWithheld) {
			reason = "job_auto_continue_state_changed"
		}
		if reason != "" {
			record.AutoContinueStatus = "skipped"
			record.AutoContinueTurnID = ""
			record.AutoContinueReason = reason
			reserved = false
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		record.AutoContinueUpdatedAt = now
		record.UpdatedAt = now
		if err := domainjob.ValidateOperationalStatusTransitionV1(original, record); err != nil {
			return cloneRecord(original), false, err
		}
		if err := m.writeRecordNoLock(&record); err != nil {
			return Record{}, false, err
		}
		m.jobs[index] = record
		return cloneRecord(record), reserved, nil
	}
	return Record{}, false, os.ErrNotExist
}

func (m *Manager) UpdateChildRun(id string, request UpdateRequest) (Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Record{}, errors.New("child run id is required")
	}
	if status := strings.TrimSpace(request.Status); status != "" {
		if err := domainjob.ValidateStatusV1(status); err != nil {
			return Record{}, err
		}
	}
	if status := strings.TrimSpace(request.AutoContinueStatus); status != "" {
		if err := domainjob.ValidateAutoContinueStatusV1(status); err != nil {
			return Record{}, err
		}
	}
	if status := strings.TrimSpace(request.CompletionDeliveryStatus); status != "" {
		if err := domainjob.ValidateCompletionDeliveryStatusV1(status); err != nil {
			return Record{}, err
		}
	}
	if status := strings.TrimSpace(request.RecoveryStatus); status != "" {
		if err := domainjob.ValidateRecoveryStatusV1(status); err != nil {
			return Record{}, err
		}
	}
	if err := validateOperationalMetadataUpdateV1(request); err != nil {
		return Record{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), ErrRestartPreserved
		}
		if legacyTypeScriptTombstoneCandidateV1(record) {
			return cloneRecord(record), errors.New("legacy TypeScript child-run tombstone is immutable")
		}
		originalRecord := cloneRecord(record)
		if err := domainmodel.ValidateReasoningEffortV1(record.Effort); err != nil {
			return cloneRecord(record), err
		}
		if err := domainjob.ValidatePersistedModelExecutionV1(record.ModelExecution); err != nil {
			return cloneRecord(record), err
		}
		if err := domainjob.ValidatePersistedUsageV1(record.Kind, record.Usage); err != nil {
			return cloneRecord(record), err
		}
		if err := validateOperationalIdentityUpdateV1(record, request); err != nil {
			return cloneRecord(record), err
		}
		if exactOperationalStatusReplayV1(record, request) {
			return cloneRecord(record), nil
		}
		autoContinueStatusChanged := strings.TrimSpace(request.AutoContinueStatus) != "" && strings.TrimSpace(request.AutoContinueStatus) != strings.TrimSpace(record.AutoContinueStatus)
		completionDeliveryStatusChanged := strings.TrimSpace(request.CompletionDeliveryStatus) != "" && strings.TrimSpace(request.CompletionDeliveryStatus) != strings.TrimSpace(record.CompletionDeliveryStatus)
		recoveryStatusChanged := strings.TrimSpace(request.RecoveryStatus) != "" && strings.TrimSpace(request.RecoveryStatus) != strings.TrimSpace(record.RecoveryStatus)
		record.Output = domainjob.ProjectPersistableUntrustedOutputV1(record.Output)
		record.Error = ""
		projectPersistableOperationalFieldsV1(&record)
		if request.Output != "" && record.SecurityBinding != nil && strings.TrimSpace(record.Kind) == "subagent" {
			return cloneRecord(record), errors.New("security-bound child output cannot be persisted")
		}
		if request.Usage != nil && record.SecurityBinding != nil && strings.TrimSpace(record.Kind) == "subagent" {
			return cloneRecord(record), errors.New("security-bound child usage cannot be persisted in the job record")
		}
		if err := domainjob.ValidatePersistedUsageV1(record.Kind, request.Usage); err != nil {
			return cloneRecord(record), err
		}
		outputUpdate := request.Output != ""
		errorUpdate := request.Error != ""
		autoContinueErrorUpdate := strings.TrimSpace(request.AutoContinueError) != "" || strings.TrimSpace(request.AutoContinueStatus) == "failed"
		completionDeliveryErrorUpdate := strings.TrimSpace(request.CompletionDeliveryError) != "" || strings.TrimSpace(request.CompletionDeliveryStatus) == "dead_letter"
		request.AutoContinueReason = domainjob.NormalizeOperationalReasonV1(request.AutoContinueReason)
		request.AutoContinueError = ""
		request.LateCompletionReason = domainjob.NormalizeOperationalReasonV1(request.LateCompletionReason)
		request.CompletionDeliveryReason = domainjob.NormalizeOperationalReasonV1(request.CompletionDeliveryReason)
		request.CompletionDeliveryError = ""
		request.RecoveryReason = domainjob.NormalizeOperationalReasonV1(request.RecoveryReason)
		request.DeadLetterReason = domainjob.NormalizeOperationalReasonV1(request.DeadLetterReason)
		failureCode := strings.TrimSpace(request.FailureCode)
		if !domainjob.ValidFailureCode(failureCode) {
			return cloneRecord(record), errors.New("child-run failure code is invalid")
		}
		failureStatus := firstNonEmptyString(strings.TrimSpace(request.Status), strings.TrimSpace(record.Status))
		failureCode = domainjob.NormalizeFailureCode(failureCode, failureStatus, strings.TrimSpace(request.Error) != "")
		request.Output = domainjob.ProjectPersistableUntrustedOutputV1(request.Output)
		request.Error = ""
		if value := strings.TrimSpace(request.ChildThreadID); value != "" && strings.TrimSpace(record.ChildThreadID) != "" && value != record.ChildThreadID {
			return cloneRecord(record), errors.New("child thread identity is immutable")
		}
		if value := strings.TrimSpace(request.ChildTurnID); value != "" && strings.TrimSpace(record.ChildTurnID) != "" && value != record.ChildTurnID {
			return cloneRecord(record), errors.New("child turn identity is immutable")
		}
		if exactChildCompletionReceiptReplay(record, request) {
			return cloneRecord(record), nil
		}
		if record.ForegroundChildHandoffReceipt != nil && request.ForegroundChildHandoffReceipt != nil &&
			!domainjob.ForegroundChildHandoffReceiptsEqualV1(record.ForegroundChildHandoffReceipt, request.ForegroundChildHandoffReceipt) {
			return cloneRecord(record), errors.New("foreground child handoff receipt is immutable")
		}
		if exactForegroundChildHandoffReceiptReplay(record, request) {
			return cloneRecord(record), nil
		}
		if request.ChildCompletionReceipt != nil {
			if strings.TrimSpace(request.Status) != string(domainjob.StatusCompleted) || request.Output != "" {
				return cloneRecord(record), errors.New("child completion receipt requires an output-free completed update")
			}
		}
		if request.ForegroundChildHandoffReceipt != nil {
			if strings.TrimSpace(request.Status) != string(domainjob.StatusCompleted) || request.Output != "" {
				return cloneRecord(record), errors.New("foreground child handoff receipt requires an output-free completed update")
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if value := strings.TrimSpace(request.Status); value != "" {
			if terminalStatus(record.Status) {
				if record.Status != value || externallyStoppedStatus(record.Status) {
					record.LateCompletionSuppressed = true
					record.LateCompletionReason = lateCompletionSuppressionReason(record.Status, value)
					record.LateCompletionUpdatedAt = now
					if strings.TrimSpace(record.AutoContinueStatus) == "" && record.AutoContinueParent {
						record.AutoContinueStatus = "skipped"
						record.AutoContinueReason = record.LateCompletionReason
						record.AutoContinueUpdatedAt = now
					}
					record.UpdatedAt = now
					if err := domainjob.ValidateOperationalStatusTransitionV1(originalRecord, record); err != nil {
						return cloneRecord(originalRecord), err
					}
					if err := m.writeRecordNoLock(&record); err != nil {
						return Record{}, err
					}
					m.jobs[index] = record
					return cloneRecord(record), nil
				}
			}
			previousStatus := record.Status
			record.Status = value
			if value == "running" && (strings.TrimSpace(record.StartedAt) == "" || previousStatus == "queued") {
				if strings.TrimSpace(record.QueuedAt) == "" {
					record.QueuedAt = now
				}
				record.StartedAt = now
				record.QueuedMs = elapsedMilliseconds(record.QueuedAt, now)
			}
			if terminalStatus(value) {
				if strings.TrimSpace(record.QueuedAt) == "" {
					record.QueuedAt = now
				}
				if strings.TrimSpace(record.StartedAt) == "" {
					record.QueuedMs = elapsedMilliseconds(record.QueuedAt, now)
				}
				record.FinishedAt = now
				record.Steers = expireQueuedSteerMessages(record.Steers, now, "child_run_terminal")
				record.PauseRequests = expireOpenPauseRequests(record.PauseRequests, now, "child_run_terminal")
			} else if shouldRefreshRuntimeLease(value) {
				refreshRuntimeLease(&record, now)
			}
		}
		if value := strings.TrimSpace(request.ChildThreadID); value != "" {
			record.ChildThreadID = value
		}
		if value := strings.TrimSpace(request.ChildTurnID); value != "" {
			record.ChildTurnID = value
		}
		if request.ChildCompletionReceipt != nil {
			record.ChildCompletionReceipt = domainjob.CloneChildCompletionReceiptV1(request.ChildCompletionReceipt)
		}
		if request.ForegroundChildHandoffReceipt != nil {
			record.ForegroundChildHandoffReceipt = domainjob.CloneForegroundChildHandoffReceiptV1(request.ForegroundChildHandoffReceipt)
		}
		if value := strings.TrimSpace(request.Workspace); value != "" {
			record.Workspace = value
		}
		if outputUpdate {
			record.Output = request.Output
		}
		if errorUpdate {
			record.Error = strings.TrimSpace(request.Error)
		}
		if failureCode != "" {
			record.FailureCode = failureCode
		}
		if request.Usage != nil {
			record.Usage = cloneUsage(request.Usage)
		}
		if request.ToolInvocations != nil {
			record.ToolInvocations = *request.ToolInvocations
		}
		if request.Isolation != nil {
			updateRecordIsolation(&record, *request.Isolation)
		}
		if request.MergeDecision != nil {
			record.MergeDecisions = append(cloneMergeDecisions(record.MergeDecisions), cloneMergeDecision(*request.MergeDecision))
		}
		if request.CleanupReceipt != nil {
			record.CleanupReceipts = append(cloneCleanupReceipts(record.CleanupReceipts), cloneCleanupReceipt(*request.CleanupReceipt))
		}
		if request.AcceptDecision != nil {
			record.AcceptDecisions = append(cloneAcceptDecisions(record.AcceptDecisions), cloneAcceptDecision(*request.AcceptDecision))
		}
		if request.ConflictReport != nil {
			record.ConflictReports = append(cloneConflictReports(record.ConflictReports), cloneConflictReport(*request.ConflictReport))
		}
		if request.RepairPatchReview != nil {
			record.RepairPatchReviews = append(cloneRepairPatchReviews(record.RepairPatchReviews), cloneRepairPatchReview(*request.RepairPatchReview))
		}
		if request.RepairDecision != nil {
			record.RepairDecisions = append(cloneRepairDecisions(record.RepairDecisions), cloneRepairDecision(*request.RepairDecision))
		}
		if request.ChildTodoList != nil {
			record.ChildTodoLists = appendOrReplaceChildTodoList(record.ChildTodoLists, *request.ChildTodoList)
		}
		if request.ChildTodoProjection != nil {
			record.ChildTodoProjections = appendOrReplaceChildTodoProjection(record.ChildTodoProjections, *request.ChildTodoProjection)
		}
		if request.ProjectionDecision != nil {
			record.ProjectionDecisions = appendOrReplaceProjectionDecision(record.ProjectionDecisions, *request.ProjectionDecision)
		}
		if value := strings.TrimSpace(request.AutoContinueStatus); value != "" {
			if autoContinueStatusChanged {
				record.AutoContinueStatus = value
				record.AutoContinueUpdatedAt = now
			}
		}
		if value := strings.TrimSpace(request.AutoContinueTurnID); value != "" {
			record.AutoContinueTurnID = value
			record.AutoContinueUpdatedAt = now
		}
		if autoContinueStatusChanged {
			record.AutoContinueReason = strings.TrimSpace(request.AutoContinueReason)
			record.AutoContinueUpdatedAt = now
		}
		if autoContinueErrorUpdate {
			record.AutoContinueError = ""
			record.AutoContinueUpdatedAt = now
		}
		if request.LateCompletionSuppressed != nil {
			record.LateCompletionSuppressed = *request.LateCompletionSuppressed
			record.LateCompletionReason = strings.TrimSpace(request.LateCompletionReason)
			record.LateCompletionUpdatedAt = now
		}
		if strings.TrimSpace(request.CompletionDeliveryID) != "" ||
			strings.TrimSpace(request.CompletionDeliveryStatus) != "" ||
			strings.TrimSpace(request.CompletionDeliveryItemID) != "" ||
			strings.TrimSpace(request.CompletionDeliveryReason) != "" ||
			completionDeliveryErrorUpdate {
			if value := strings.TrimSpace(request.CompletionDeliveryID); value != "" {
				record.CompletionDeliveryID = value
			}
			if value := strings.TrimSpace(request.CompletionDeliveryItemID); value != "" {
				record.CompletionDeliveryItemID = value
			}
			if value := strings.TrimSpace(request.CompletionDeliveryStatus); value != "" {
				if completionDeliveryStatusChanged {
					record.CompletionDeliveryStatus = value
					record.CompletionDeliveryAttempts++
				}
				if completionDeliveryStatusChanged {
					switch value {
					case "delivered", "skipped":
						record.CompletionDeliveryAt = now
						if value == "delivered" && (record.RecoveryStatus == "dead_lettered" || record.RecoveryStatus == "recovering") {
							record.RecoveryStatus = "recovered"
							record.RecoveryReason = firstNonEmptyString(strings.TrimSpace(request.CompletionDeliveryReason), "completion_delivery_recovered")
							record.RecoveryUpdatedAt = now
							record.DeadLetterReason = ""
						}
					case "dead_letter":
						record.CompletionDeadLetterAt = now
					case "retry":
						if record.RecoveryStatus == "dead_lettered" {
							record.RecoveryStatus = "recovering"
							record.RecoveryReason = firstNonEmptyString(strings.TrimSpace(request.CompletionDeliveryReason), "completion_delivery_retry")
							record.RecoveryUpdatedAt = now
						}
					}
				}
			}
			if completionDeliveryStatusChanged {
				record.CompletionDeliveryReason = strings.TrimSpace(request.CompletionDeliveryReason)
			}
			if completionDeliveryErrorUpdate {
				record.CompletionDeliveryError = ""
			}
			if strings.TrimSpace(request.CompletionDeliveryStatus) == "dead_letter" {
				record.DeadLetterReason = firstNonEmptyString(strings.TrimSpace(request.CompletionDeliveryReason), "completion_delivery_dead_letter")
				record.RecoveryStatus = "dead_lettered"
				record.RecoveryReason = record.DeadLetterReason
				record.RecoveryUpdatedAt = now
			}
		}
		if value := strings.TrimSpace(request.LastHeartbeatAt); value != "" {
			record.LastHeartbeatAt = value
		}
		if value := strings.TrimSpace(request.LeaseOwner); value != "" {
			record.LeaseOwner = value
		}
		if value := strings.TrimSpace(request.LeaseExpiresAt); value != "" {
			record.LeaseExpiresAt = value
		}
		if request.StaleAfterMs > 0 {
			record.StaleAfterMs = request.StaleAfterMs
		}
		if request.Orphaned != nil {
			record.Orphaned = *request.Orphaned
		}
		if value := strings.TrimSpace(request.RecoveryStatus); value != "" {
			if recoveryStatusChanged {
				record.RecoveryStatus = value
				record.RecoveryAttempt++
				record.RecoveryUpdatedAt = now
			}
		}
		if recoveryStatusChanged {
			record.RecoveryReason = strings.TrimSpace(request.RecoveryReason)
			record.RecoveryUpdatedAt = now
		}
		if value := strings.TrimSpace(request.DeadLetterReason); value != "" {
			record.DeadLetterReason = value
			if strings.TrimSpace(record.RecoveryStatus) == "" {
				record.RecoveryStatus = "dead_lettered"
				record.RecoveryUpdatedAt = now
			}
		}
		if shouldRefreshRuntimeLease(record.Status) &&
			strings.TrimSpace(request.LastHeartbeatAt) == "" &&
			strings.TrimSpace(request.LeaseExpiresAt) == "" {
			refreshRuntimeLease(&record, now)
		}
		applyRuntimeLeaseDefaults(&record, now)
		record.UpdatedAt = now
		record.SteerState = childRunState(record)
		record.PauseState = childRunPauseState(record)
		if err := domainjob.ValidateOperationalStatusTransitionV1(originalRecord, record); err != nil {
			return cloneRecord(originalRecord), err
		}
		if err := m.writeRecordNoLock(&record); err != nil {
			return Record{}, err
		}
		m.jobs[index] = record
		return cloneRecord(record), nil
	}
	return Record{}, os.ErrNotExist
}

func (m *Manager) AddPauseRequest(id string, request domainjob.PauseRequest) (Record, domainjob.PauseRequest, error) {
	id = strings.TrimSpace(id)
	request.ID = strings.TrimSpace(request.ID)
	request.Status = firstNonEmptyString(strings.TrimSpace(request.Status), "requested")
	requestedAtProvided := strings.TrimSpace(request.RequestedAt) != ""
	request.RejectedReason = domainjob.NormalizeOperationalReasonV1(request.RejectedReason)
	if id == "" {
		return Record{}, domainjob.PauseRequest{}, errors.New("child run id is required")
	}
	if request.ID == "" {
		return Record{}, domainjob.PauseRequest{}, errors.New("pause request id is required")
	}
	if err := domainjob.ValidatePauseRequestStatusV1(request.Status); err != nil {
		return Record{}, domainjob.PauseRequest{}, err
	}
	if request.Status != "requested" && request.Status != "rejected" {
		return Record{}, domainjob.PauseRequest{}, errors.New("new pause request must be requested or rejected")
	}
	if strings.TrimSpace(request.PausedAt) != "" || strings.TrimSpace(request.ResumedAt) != "" || strings.TrimSpace(request.ResumeToken) != "" ||
		strings.TrimSpace(request.ResumeTokenIssuedAt) != "" || strings.TrimSpace(request.ResumeTokenExpiresAt) != "" {
		return Record{}, domainjob.PauseRequest{}, errors.New("new pause request carries settlement authority")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.PauseRequest{}, ErrRestartPreserved
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		request.ParentThreadID = strings.TrimSpace(request.ParentThreadID)
		request.ChildRunID = firstNonEmptyString(strings.TrimSpace(request.ChildRunID), record.ID)
		request.JobID = firstNonEmptyString(strings.TrimSpace(request.JobID), record.ID)
		request.RequestedAt = firstNonEmptyString(strings.TrimSpace(request.RequestedAt), now)
		for _, existing := range record.PauseRequests {
			if existing.ID == request.ID {
				if !samePauseRequestReplayV1(existing, request, requestedAtProvided) {
					return cloneRecord(record), domainjob.PauseRequest{}, errors.New("pause request replay does not match durable content")
				}
				return cloneRecord(record), existing, nil
			}
		}
		record.PauseRequests = append(clonePauseRequests(record.PauseRequests), request)
		switch request.Status {
		case "requested":
			record.Status = string(domainjob.StatusPauseRequested)
		case "paused":
			record.Status = string(domainjob.StatusPaused)
		case "resumed":
			record.Status = string(domainjob.StatusResumeRequested)
		}
		record.UpdatedAt = now
		record.SteerState = childRunState(record)
		record.PauseState = childRunPauseState(record)
		if err := m.writeRecordNoLock(&record); err != nil {
			return Record{}, domainjob.PauseRequest{}, err
		}
		m.jobs[index] = record
		return cloneRecord(record), record.PauseRequests[len(record.PauseRequests)-1], nil
	}
	return Record{}, domainjob.PauseRequest{}, os.ErrNotExist
}

func (m *Manager) MarkPauseRequestPaused(id string, requestID string, pausedAt string, token domainjob.ResumeToken) (Record, domainjob.PauseRequest, error) {
	return m.updatePauseRequest(id, requestID, "paused", pausedAt, "", token)
}

func (m *Manager) MarkPauseResumeRequested(id string, requestID string) (Record, domainjob.PauseRequest, error) {
	return m.transitionPauseMainStatus(id, requestID, string(domainjob.StatusPaused), string(domainjob.StatusResumeRequested), "paused")
}

func (m *Manager) RollbackPauseResumeRequested(id string, requestID string) (Record, domainjob.PauseRequest, error) {
	return m.transitionPauseMainStatus(id, requestID, string(domainjob.StatusResumeRequested), string(domainjob.StatusPaused), "paused")
}

func (m *Manager) MarkPauseRequestResumed(id string, requestID string, resumedAt string) (Record, domainjob.PauseRequest, error) {
	return m.updatePauseRequest(id, requestID, "resumed", resumedAt, "", domainjob.ResumeToken{})
}

func (m *Manager) CompletePauseResume(id string, requestID string) (Record, domainjob.PauseRequest, error) {
	return m.transitionPauseMainStatus(id, requestID, string(domainjob.StatusResuming), string(domainjob.StatusRunning), "resumed")
}

func (m *Manager) RejectPauseRequest(id string, request domainjob.PauseRequest, reason string) (Record, domainjob.PauseRequest, error) {
	request.Status = "rejected"
	request.RejectedReason = domainjob.NormalizeOperationalReasonV1(reason)
	if request.RequestedAt == "" {
		request.RequestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return m.AddPauseRequest(id, request)
}

func (m *Manager) ExpirePauseRequest(id string, requestID string, expiredAt string, reason string) (Record, domainjob.PauseRequest, error) {
	return m.updatePauseRequest(id, requestID, "expired", expiredAt, reason, domainjob.ResumeToken{})
}

func (m *Manager) transitionPauseMainStatus(id string, requestID string, fromStatus string, toStatus string, requestStatus string) (Record, domainjob.PauseRequest, error) {
	id = strings.TrimSpace(id)
	requestID = strings.TrimSpace(requestID)
	if id == "" {
		return Record{}, domainjob.PauseRequest{}, errors.New("child run id is required")
	}
	if requestID == "" {
		return Record{}, domainjob.PauseRequest{}, errors.New("pause request id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.PauseRequest{}, ErrRestartPreserved
		}
		if strings.TrimSpace(record.Status) != strings.TrimSpace(fromStatus) {
			return cloneRecord(record), domainjob.PauseRequest{}, errors.New("pause main-state compare-and-swap failed")
		}
		for _, request := range record.PauseRequests {
			if strings.TrimSpace(request.ID) != requestID {
				continue
			}
			if strings.TrimSpace(request.Status) != strings.TrimSpace(requestStatus) ||
				strings.TrimSpace(record.PauseState.PauseRequestID) != requestID {
				return cloneRecord(record), request, errors.New("pause request authority does not match the durable main state")
			}
			record.Status = strings.TrimSpace(toStatus)
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			record.SteerState = childRunState(record)
			record.PauseState = childRunPauseState(record)
			if err := m.writeRecordNoLock(&record); err != nil {
				return Record{}, domainjob.PauseRequest{}, err
			}
			m.jobs[index] = record
			return cloneRecord(record), request, nil
		}
		return cloneRecord(record), domainjob.PauseRequest{}, os.ErrNotExist
	}
	return Record{}, domainjob.PauseRequest{}, os.ErrNotExist
}

func (m *Manager) updatePauseRequest(id string, requestID string, status string, at string, reason string, token domainjob.ResumeToken) (Record, domainjob.PauseRequest, error) {
	id = strings.TrimSpace(id)
	requestID = strings.TrimSpace(requestID)
	if id == "" {
		return Record{}, domainjob.PauseRequest{}, errors.New("child run id is required")
	}
	if requestID == "" {
		return Record{}, domainjob.PauseRequest{}, errors.New("pause request id is required")
	}
	if err := domainjob.ValidatePauseRequestStatusV1(status); err != nil {
		return Record{}, domainjob.PauseRequest{}, err
	}
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.PauseRequest{}, ErrRestartPreserved
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if strings.TrimSpace(at) == "" {
			at = now
		}
		requests := clonePauseRequests(record.PauseRequests)
		for requestIndex := range requests {
			if requests[requestIndex].ID != requestID {
				continue
			}
			if strings.TrimSpace(requests[requestIndex].Status) == strings.TrimSpace(status) {
				if exactPauseSettlementReplayV1(requests[requestIndex], status, at, reason, token) {
					return cloneRecord(record), requests[requestIndex], nil
				}
				return cloneRecord(record), requests[requestIndex], errors.New("pause settlement replay does not match durable content")
			}
			if strings.TrimSpace(status) == "resumed" && strings.TrimSpace(record.Status) != string(domainjob.StatusResumeRequested) {
				return cloneRecord(record), requests[requestIndex], errors.New("resume settlement requires durable resume-requested state")
			}
			if err := domainjob.ValidatePauseRequestTransitionV1(requests[requestIndex].Status, status); err != nil {
				return cloneRecord(record), requests[requestIndex], err
			}
			requests[requestIndex].Status = strings.TrimSpace(status)
			switch strings.TrimSpace(status) {
			case "paused":
				requests[requestIndex].PausedAt = strings.TrimSpace(at)
				requests[requestIndex].ResumeToken = strings.TrimSpace(token.ResumeToken)
				requests[requestIndex].ResumeTokenIssuedAt = strings.TrimSpace(token.IssuedAt)
				requests[requestIndex].ResumeTokenExpiresAt = strings.TrimSpace(token.ExpiresAt)
				record.Status = string(domainjob.StatusPaused)
			case "resumed":
				requests[requestIndex].ResumedAt = strings.TrimSpace(at)
				requests[requestIndex].ResumeToken = ""
				record.Status = string(domainjob.StatusResuming)
			case "expired":
				requests[requestIndex].RejectedReason = reason
				requests[requestIndex].ResumeToken = ""
				requests[requestIndex].ResumedAt = ""
				record.Status = string(domainjob.StatusRunning)
			case "rejected":
				requests[requestIndex].RejectedReason = reason
			}
			record.PauseRequests = requests
			record.UpdatedAt = now
			record.SteerState = childRunState(record)
			record.PauseState = childRunPauseState(record)
			if err := m.writeRecordNoLock(&record); err != nil {
				return Record{}, domainjob.PauseRequest{}, err
			}
			m.jobs[index] = record
			return cloneRecord(record), record.PauseRequests[requestIndex], nil
		}
		return cloneRecord(record), domainjob.PauseRequest{}, os.ErrNotExist
	}
	return Record{}, domainjob.PauseRequest{}, os.ErrNotExist
}

func (m *Manager) QueueSteerMessage(id string, authority domainjob.SteerQueueAuthorityV1, message domainjob.SteerMessage) (Record, domainjob.SteerMessage, error) {
	id = strings.TrimSpace(id)
	message.ID = strings.TrimSpace(message.ID)
	if id == "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("child run id is required")
	}
	if message.ID == "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("steer message id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.SteerMessage{}, ErrRestartPreserved
		}
		if err := domainjob.ValidateSteerQueueAuthorityForRecordV1(authority, record); err != nil {
			return Record{}, domainjob.SteerMessage{}, err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		message.ParentThreadID = strings.TrimSpace(message.ParentThreadID)
		message.ChildRunID = firstNonEmptyString(strings.TrimSpace(message.ChildRunID), record.ID)
		message.JobID = firstNonEmptyString(strings.TrimSpace(message.JobID), record.ID)
		message.Text = strings.TrimSpace(message.Text)
		message.SourceTurnID = strings.TrimSpace(message.SourceTurnID)
		message.SourceToolCallID = strings.TrimSpace(message.SourceToolCallID)
		message.ContextDigest = strings.TrimSpace(authority.ContextDigest)
		message.AuthorityDigest = message.ContextDigest
		if message.AuthorityDigest == "" {
			message.AuthorityDigest = strings.TrimSpace(authority.SecurityBindingDigest)
		}
		message.QueueAuthorityDigest = domainjob.SteerQueueAuthorityDigestV1(authority)
		if err := domainjob.ValidateSteerTextProjectionV1(message.Text); err != nil {
			return Record{}, domainjob.SteerMessage{}, err
		}
		if message.ProjectionVersion == 0 {
			message.ProjectionVersion = domainjob.SteerMessageProjectionVersionV1
		}
		if message.ProjectionVersion != domainjob.SteerMessageProjectionVersionV1 {
			return Record{}, domainjob.SteerMessage{}, errors.New("steer message projection version is invalid")
		}
		expectedContentDigest := domainjob.SteerMessageContentDigestV1(message)
		if message.ContentDigest != "" && strings.TrimSpace(message.ContentDigest) != expectedContentDigest {
			return Record{}, domainjob.SteerMessage{}, errors.New("steer message content digest is invalid")
		}
		message.ContentDigest = expectedContentDigest
		message.Status = firstNonEmptyString(strings.TrimSpace(message.Status), "queued")
		if message.Status != "queued" {
			return Record{}, domainjob.SteerMessage{}, errors.New("new steer message must be queued")
		}
		message.CreatedAt = firstNonEmptyString(strings.TrimSpace(message.CreatedAt), now)
		for _, existing := range record.Steers {
			if existing.ID == message.ID {
				if existing.Status != "queued" || !domainjob.SteerMessagesHaveSameContentV1(existing, message) {
					return Record{}, domainjob.SteerMessage{}, errors.New("steer message replay does not match queued content")
				}
				return cloneRecord(record), existing, nil
			}
		}
		beforeQueue := cloneRecord(record)
		priorQueueChain := m.pendingSteerStarts[record.ID]
		record.Steers = append(cloneSteerMessages(record.Steers), message)
		record.UpdatedAt = now
		record.SteerState = childRunState(record)
		if err := m.writeRecordNoLock(&record); err != nil {
			return Record{}, domainjob.SteerMessage{}, err
		}
		m.jobs[index] = record
		if authority.PendingUntilChildTurn {
			m.recordPendingSteerStartAdvanceNoLock(beforeQueue, record, priorQueueChain)
		}
		return cloneRecord(record), message, nil
	}
	return Record{}, domainjob.SteerMessage{}, os.ErrNotExist
}

func (m *Manager) AdmitSteerMessage(id string, messageID string, admittedAt string) (Record, domainjob.SteerMessage, error) {
	return m.updateSteerMessage(id, messageID, "admitted", admittedAt, "")
}

func (m *Manager) SettleSteerPromotionExact(
	id string,
	expected domainjob.SteerMessage,
	settlement domainjob.SteerPromotionSettlementV1,
) (Record, domainjob.SteerMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.TrimSpace(expected.ID) == "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("steer promotion settlement identity is invalid")
	}
	if err := domainjob.ValidateSteerPromotionSettlementV1(settlement, expected); err != nil {
		return Record{}, domainjob.SteerMessage{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.SteerMessage{}, ErrRestartPreserved
		}
		steers := cloneSteerMessages(record.Steers)
		for steerIndex := range steers {
			current := steers[steerIndex]
			if current.ID != strings.TrimSpace(expected.ID) {
				continue
			}
			if !domainjob.SteerMessageMatchesPromotionAuthorityV1(current, expected) {
				return Record{}, domainjob.SteerMessage{}, errors.New("steer promotion settlement does not match durable queue authority")
			}
			if current.Status == "admitted" {
				if current.AdmittedAt != settlement.PromotedAt || current.PromotionCommitID != settlement.PromotionCommitID ||
					current.PromotionEntryID != settlement.PromotionEntryID {
					return Record{}, domainjob.SteerMessage{}, errors.New("steer promotion settlement conflicts with committed admission")
				}
				return cloneRecord(record), current, nil
			}
			steers[steerIndex].Status = "admitted"
			steers[steerIndex].AdmittedAt = settlement.PromotedAt
			steers[steerIndex].PromotionCommitID = settlement.PromotionCommitID
			steers[steerIndex].PromotionEntryID = settlement.PromotionEntryID
			record.Steers = steers
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			record.SteerState = childRunState(record)
			if err := m.writeRecordNoLock(&record); err != nil {
				return Record{}, domainjob.SteerMessage{}, err
			}
			m.jobs[index] = record
			return cloneRecord(record), steers[steerIndex], nil
		}
		return cloneRecord(record), domainjob.SteerMessage{}, os.ErrNotExist
	}
	return Record{}, domainjob.SteerMessage{}, os.ErrNotExist
}

func (m *Manager) RejectSteerMessage(id string, message domainjob.SteerMessage, reason string) (Record, domainjob.SteerMessage, error) {
	id = strings.TrimSpace(id)
	message.ID = strings.TrimSpace(message.ID)
	if id == "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("child run id is required")
	}
	if message.ID == "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("steer message id is required")
	}
	message.ParentThreadID = strings.TrimSpace(message.ParentThreadID)
	message.ChildRunID = strings.TrimSpace(message.ChildRunID)
	message.JobID = strings.TrimSpace(message.JobID)
	message.Text = strings.TrimSpace(message.Text)
	message.SourceTurnID = strings.TrimSpace(message.SourceTurnID)
	message.SourceToolCallID = strings.TrimSpace(message.SourceToolCallID)
	if message.ProjectionVersion == 0 {
		message.ProjectionVersion = domainjob.SteerMessageProjectionVersionV1
	}
	if message.ContentDigest == "" {
		message.ContentDigest = domainjob.SteerMessageContentDigestV1(message)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.SteerMessage{}, ErrRestartPreserved
		}
		steers := cloneSteerMessages(record.Steers)
		for steerIndex := range steers {
			if steers[steerIndex].ID != message.ID {
				continue
			}
			reason = domainjob.NormalizeOperationalReasonV1(reason)
			if steers[steerIndex].Status == "rejected" {
				if steers[steerIndex].RejectedReason != reason ||
					!domainjob.RejectedSteerMatchesQueuedContentV1(steers[steerIndex], message) {
					return Record{}, domainjob.SteerMessage{}, errors.New("rejected steer message replay does not match committed tombstone")
				}
				return cloneRecord(record), steers[steerIndex], nil
			}
			if !domainjob.SteerMessagesHaveSameContentV1(steers[steerIndex], message) {
				return Record{}, domainjob.SteerMessage{}, errors.New("steer message rejection does not match queued content")
			}
			if steers[steerIndex].Status != "queued" {
				return Record{}, domainjob.SteerMessage{}, errors.New("steer message cannot transition to rejected")
			}
			steers[steerIndex].Status = "rejected"
			steers[steerIndex].RejectedReason = reason
			steers[steerIndex].Text = ""
			steers[steerIndex].SourceTurnID = ""
			steers[steerIndex].SourceToolCallID = ""
			now := time.Now().UTC().Format(time.RFC3339Nano)
			record.Steers = steers
			record.UpdatedAt = now
			record.SteerState = childRunState(record)
			if err := m.writeRecordNoLock(&record); err != nil {
				return Record{}, domainjob.SteerMessage{}, err
			}
			m.jobs[index] = record
			return cloneRecord(record), steers[steerIndex], nil
		}
		return cloneRecord(record), domainjob.SteerMessage{}, os.ErrNotExist
	}
	return Record{}, domainjob.SteerMessage{}, os.ErrNotExist
}

func (m *Manager) updateSteerMessage(id string, messageID string, status string, admittedAt string, reason string) (Record, domainjob.SteerMessage, error) {
	id = strings.TrimSpace(id)
	messageID = strings.TrimSpace(messageID)
	status = strings.TrimSpace(status)
	admittedAt = strings.TrimSpace(admittedAt)
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	if id == "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("child run id is required")
	}
	if messageID == "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("steer message id is required")
	}
	if status == "admitted" {
		parsed, err := time.Parse(time.RFC3339Nano, admittedAt)
		if err != nil || parsed.Location() != time.UTC || parsed.UTC().Format(time.RFC3339Nano) != admittedAt {
			return Record{}, domainjob.SteerMessage{}, errors.New("steer message admittedAt is invalid")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != id {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.SteerMessage{}, ErrRestartPreserved
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		steers := cloneSteerMessages(record.Steers)
		for steerIndex := range steers {
			if steers[steerIndex].ID != messageID {
				continue
			}
			if status == "admitted" && steers[steerIndex].Status == "admitted" {
				if steers[steerIndex].AdmittedAt != admittedAt {
					return Record{}, domainjob.SteerMessage{}, errors.New("admitted steer message replay does not match committed time")
				}
				return cloneRecord(record), steers[steerIndex], nil
			}
			if status == "admitted" && steers[steerIndex].Status != "queued" {
				return Record{}, domainjob.SteerMessage{}, errors.New("only a queued steer message can be admitted")
			}
			steers[steerIndex].Status = status
			if admittedAt != "" {
				steers[steerIndex].AdmittedAt = admittedAt
			}
			if reason != "" {
				steers[steerIndex].RejectedReason = reason
			}
			record.Steers = steers
			record.UpdatedAt = now
			record.SteerState = childRunState(record)
			if err := m.writeRecordNoLock(&record); err != nil {
				return Record{}, domainjob.SteerMessage{}, err
			}
			m.jobs[index] = record
			return cloneRecord(record), steers[steerIndex], nil
		}
		return cloneRecord(record), domainjob.SteerMessage{}, os.ErrNotExist
	}
	return Record{}, domainjob.SteerMessage{}, os.ErrNotExist
}

func (m *Manager) Load(id string) (Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Record{}, errors.New("child run id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, record := range m.jobs {
		if record.ID == id {
			return cloneRecord(record), nil
		}
	}
	return Record{}, os.ErrNotExist
}

func (m *Manager) LoadChildRun(id string) (Record, error) {
	return m.Load(id)
}

func (m *Manager) RecordsByParentThread(parentThreadID string) []Record {
	parentThreadID = strings.TrimSpace(parentThreadID)
	if parentThreadID == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	records := []Record{}
	for _, record := range m.jobs {
		if strings.TrimSpace(record.ParentThreadID) == parentThreadID {
			records = append(records, cloneRecord(record))
		}
	}
	return records
}

func (m *Manager) List(parentThreadID string) ([]Record, error) {
	return m.RecordsByParentThread(parentThreadID), nil
}

func (m *Manager) CleanupStaleRunning() (int, error) {
	records, err := m.CleanupStaleRunningRecords()
	if err != nil {
		return 0, err
	}
	return len(records), nil
}

func (m *Manager) CleanupStaleRunningRecords() ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	if m.restartPreserved != nil {
		if _, err := m.revalidateRestartPreservationNoLock(); err != nil {
			return nil, err
		}
	}
	cleaned := []Record{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for index := range m.jobs {
		if m.restartPreservesRecordNoLock(m.jobs[index]) {
			continue
		}
		if !runtimeRestartRecoverableStatus(m.jobs[index].Status) {
			continue
		}
		record := m.jobs[index]
		record.Status = "interrupted"
		record.FinishedAt = now
		record.UpdatedAt = now
		record.Orphaned = true
		// Manager cleanup has no current thread/context/grant authority. It may
		// close the abandoned process lifecycle, but only the server recovery
		// coordinator can classify delivery as recovering, recovered, or
		// dead-lettered after revalidating the frozen authority.
		record.RecoveryStatus = ""
		record.RecoveryReason = "runtime_restart_orphaned_running_job"
		record.RecoveryUpdatedAt = now
		record.SteerState = childRunState(record)
		record.PauseState = childRunPauseState(record)
		if err := m.writeRecordNoLock(&record); err != nil {
			return cleaned, err
		}
		m.jobs[index] = record
		cleaned = append(cleaned, cloneRecord(record))
	}
	return cleaned, nil
}

func (m *Manager) StartParallel(parentGoalID, parentThreadID, parentTurnID, model, effort string, tasks []string) ([]Record, error) {
	if len(tasks) < 2 {
		return nil, errors.New("parallel child execution requires at least two tasks")
	}
	if err := domainmodel.ValidateReasoningEffortV1(effort); err != nil {
		return nil, err
	}
	groupID := fmt.Sprintf("parallel-%d", time.Now().UTC().UnixNano())
	out := make([]Record, len(tasks))
	errs := make(chan error, len(tasks))
	var wg sync.WaitGroup
	for index, task := range tasks {
		wg.Add(1)
		go func(index int, task string) {
			defer wg.Done()
			record, err := m.StartChildRun(StartRequest{
				ParentGoalID:          parentGoalID,
				ParentThreadID:        parentThreadID,
				ParentTurnID:          parentTurnID,
				Kind:                  "parallel-child-run",
				Model:                 model,
				Effort:                effort,
				ProfileSource:         "parent-default",
				DefaultModelInherited: true,
				ParallelGroupID:       groupID,
				ParallelIndex:         index + 1,
				Output:                task,
			})
			if err != nil {
				errs <- err
				return
			}
			out[index] = record
		}(index, task)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (m *Manager) Records() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]any, 0, len(m.jobs))
	for _, record := range m.jobs {
		out = append(out, cloneRecord(record))
	}
	return out
}

func (m *Manager) AllRecords() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.jobs))
	for _, record := range m.jobs {
		out = append(out, cloneRecord(record))
	}
	return out
}

func (m *Manager) writeRecordNoLock(record *Record) error {
	m.restartEffectsStarted = true
	// Any attempted write burns prior process-local queue advancement. Only a
	// confirmed successful pending queue may publish a replacement chain.
	if record != nil {
		delete(m.pendingSteerStarts, record.ID)
	}
	if record == nil || !validPersistedJobID(record.ID) {
		return errors.New("child-run record identity is invalid")
	}
	if m.restartPreservesRecordNoLock(*record) {
		return ErrRestartPreserved
	}
	// Live writes must reject unknown or malformed closed metadata before the
	// normalizer can remove it. Projection is reserved for the isolated
	// semantic-startup stage; it is never a live fail-open repair mechanism.
	if err := domainjob.ValidatePersistedModelExecutionV1(record.ModelExecution); err != nil {
		return err
	}
	if err := domainjob.ValidatePersistedUsageV1(record.Kind, record.Usage); err != nil {
		return err
	}
	if err := domainmodel.ValidateReasoningEffortV1(record.Effort); err != nil {
		return err
	}
	if !domainjob.ValidFailureCode(record.FailureCode) {
		return errors.New("child-run failure code is invalid")
	}
	if err := validateLiveSteerProjectionV1(record.Steers); err != nil {
		return err
	}
	*record = domainjob.NormalizePersistedRecordV1(*record)
	if err := domainjob.ValidateStatusV1(record.Status); err != nil {
		return err
	}
	if err := domainjob.ValidatePersistedProjectionV1(*record); err != nil {
		return err
	}
	if err := domainjob.ValidateOperationalStateV1(*record); err != nil {
		return err
	}
	if err := validateDurableChildRunRecord(context.Background(), *record, m.completionVerifier); err != nil {
		return err
	}
	if strings.TrimSpace(m.root) == "" {
		return nil
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(filepath.Join(m.root, safeID(record.ID)+".json"), data, 0o600)
}

func projectPersistableOperationalFieldsV1(record *Record) {
	if record == nil {
		return
	}
	*record = domainjob.NormalizePersistedRecordV1(*record)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := createChildRunTemporaryFileV1(dir, filepath.Base(path))
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	syncDirectoryBestEffort(dir)
	return nil
}

func syncDirectoryBestEffort(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	defer handle.Close()
	_ = handle.Sync()
}

func RunLineageContractExercise() map[string]any {
	manager, _ := NewManager("")
	topLevelErr := ""
	if _, err := manager.Start("", "thr_contract", "task"); err != nil {
		topLevelErr = "parent_goal_lineage_required"
	}
	taskJob, taskErr := manager.StartChildRun(StartRequest{
		ParentGoalID:          "goal_contract",
		ParentThreadID:        "thr_contract",
		Kind:                  "task",
		Model:                 "deepseek-chat",
		ProfileSource:         "parent-default",
		DefaultModelInherited: true,
	})
	childJob, childErr := manager.StartChildRun(StartRequest{
		ParentGoalID:          "goal_contract",
		ParentThreadID:        "thr_contract",
		Kind:                  "child-run",
		Model:                 "deepseek-chat",
		ProfileSource:         "parent-default",
		DefaultModelInherited: true,
	})
	parallelJobs, parallelErr := manager.StartParallel("goal_contract", "thr_contract", "turn_contract", "deepseek-chat", "", []string{"left", "right"})
	return map[string]any{
		"runtimeGoContractParitySlice":  true,
		"parentGoalLineageRequired":     topLevelErr != "",
		"topLevelStartError":            topLevelErr,
		"taskJob":                       taskJob,
		"childJob":                      childJob,
		"parallelJobs":                  parallelJobs,
		"taskJobError":                  contractExerciseFailureCode("task_job_contract_failed", taskErr),
		"childJobError":                 contractExerciseFailureCode("child_job_contract_failed", childErr),
		"parallelJobError":              contractExerciseFailureCode("parallel_job_contract_failed", parallelErr),
		"sameParentLineage":             taskJob.LineageKey == childJob.LineageKey && taskJob.LineageKey != "",
		"defaultModelInherited":         taskJob.DefaultModelInherited && childJob.DefaultModelInherited,
		"parallelExecutionAvailable":    parallelErr == nil && len(parallelJobs) == 2,
		"durableChildRunStore":          true,
		"topLevelRouteExposed":          false,
		"reasonixPublicProtocolAllowed": false,
		"rendererVisibleRouteExposed":   false,
	}
}

func contractExerciseFailureCode(code string, err error) string {
	if err == nil {
		return ""
	}
	return code
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func sortedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func cloneRecord(record Record) Record {
	record.SecurityBinding = domainjob.CloneSecurityBinding(record.SecurityBinding)
	record.CaseDelegation = domainjob.CloneCaseDelegationContextV1(record.CaseDelegation)
	record.ChildCompletionReceipt = domainjob.CloneChildCompletionReceiptV1(record.ChildCompletionReceipt)
	record.ForegroundChildHandoffReceipt = domainjob.CloneForegroundChildHandoffReceiptV1(record.ForegroundChildHandoffReceipt)
	record.DelegatedToolManifest = domainjob.CloneDelegatedToolManifestV1(record.DelegatedToolManifest)
	record.ToolScope = append([]string(nil), record.ToolScope...)
	record.Usage = cloneUsage(record.Usage)
	record.ModelExecution = cloneUsage(record.ModelExecution)
	record.MaxModelSteps = cloneOptionalInt(record.MaxModelSteps)
	record.Steers = cloneSteerMessages(record.Steers)
	record.SteerState = childRunState(record)
	record.PauseRequests = clonePauseRequests(record.PauseRequests)
	record.PauseState = childRunPauseState(record)
	record.ChangedFiles = cloneChangedFiles(record.ChangedFiles)
	record.MergeDecisions = cloneMergeDecisions(record.MergeDecisions)
	record.CleanupReceipts = cloneCleanupReceipts(record.CleanupReceipts)
	record.AcceptDecisions = cloneAcceptDecisions(record.AcceptDecisions)
	record.ConflictReports = cloneConflictReports(record.ConflictReports)
	record.RepairPatchReviews = cloneRepairPatchReviews(record.RepairPatchReviews)
	record.RepairDecisions = cloneRepairDecisions(record.RepairDecisions)
	record.ChildTodoLists = cloneChildTodoLists(record.ChildTodoLists)
	record.ChildTodoProjections = cloneChildTodoProjections(record.ChildTodoProjections)
	record.ProjectionDecisions = cloneProjectionDecisions(record.ProjectionDecisions)
	return record
}

func updateRecordIsolation(record *Record, isolation domainjob.WorktreeIsolation) {
	if record == nil {
		return
	}
	if value := strings.TrimSpace(isolation.IsolationMode); value != "" {
		record.IsolationMode = value
	}
	if value := strings.TrimSpace(isolation.WorktreePath); value != "" {
		record.WorktreePath = value
	}
	if value := strings.TrimSpace(isolation.WorktreeBranch); value != "" {
		record.WorktreeBranch = value
	}
	if value := strings.TrimSpace(isolation.BaseCommit); value != "" {
		record.BaseCommit = value
	}
	if value := strings.TrimSpace(isolation.CurrentCommit); value != "" {
		record.CurrentCommit = value
	}
	if isolation.ChangedFiles != nil {
		record.ChangedFiles = cloneChangedFiles(isolation.ChangedFiles)
	}
	if value := strings.TrimSpace(isolation.DiffSummary); value != "" {
		record.DiffSummary = value
	}
	if value := strings.TrimSpace(isolation.MergeStatus); value != "" {
		record.MergeStatus = value
	}
}

func cloneChangedFiles(values []domainjob.ChangedFile) []domainjob.ChangedFile {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ChangedFile, 0, len(values))
	for _, value := range values {
		path := strings.TrimSpace(value.Path)
		status := strings.TrimSpace(value.Status)
		if path == "" && status == "" {
			continue
		}
		out = append(out, domainjob.ChangedFile{Path: path, Status: status})
	}
	return out
}

func cloneMergeDecision(value domainjob.MergeDecision) domainjob.MergeDecision {
	return domainjob.MergeDecision{
		ID:             strings.TrimSpace(value.ID),
		ParentThreadID: strings.TrimSpace(value.ParentThreadID),
		ChildRunID:     strings.TrimSpace(value.ChildRunID),
		JobID:          strings.TrimSpace(value.JobID),
		Decision:       strings.TrimSpace(value.Decision),
		CreatedAt:      strings.TrimSpace(value.CreatedAt),
		Reason:         strings.TrimSpace(value.Reason),
		ApprovalID:     strings.TrimSpace(value.ApprovalID),
	}
}

func cloneMergeDecisions(values []domainjob.MergeDecision) []domainjob.MergeDecision {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.MergeDecision, 0, len(values))
	for _, value := range values {
		cloned := cloneMergeDecision(value)
		if cloned.ID == "" && cloned.Decision == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneCleanupReceipt(value domainjob.CleanupReceipt) domainjob.CleanupReceipt {
	return domainjob.CleanupReceipt{
		ID:               strings.TrimSpace(value.ID),
		AcceptDecisionID: strings.TrimSpace(value.AcceptDecisionID),
		WorktreePath:     strings.TrimSpace(value.WorktreePath),
		Branch:           strings.TrimSpace(value.Branch),
		Removed:          value.Removed,
		RetainedReason:   strings.TrimSpace(value.RetainedReason),
		CreatedAt:        strings.TrimSpace(value.CreatedAt),
	}
}

func cloneCleanupReceipts(values []domainjob.CleanupReceipt) []domainjob.CleanupReceipt {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.CleanupReceipt, 0, len(values))
	for _, value := range values {
		cloned := cloneCleanupReceipt(value)
		if cloned.ID == "" && cloned.WorktreePath == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneAcceptDecision(value domainjob.AcceptDecision) domainjob.AcceptDecision {
	return domainjob.AcceptDecision{
		ID:                 strings.TrimSpace(value.ID),
		ParentThreadID:     strings.TrimSpace(value.ParentThreadID),
		ChildRunID:         strings.TrimSpace(value.ChildRunID),
		JobID:              strings.TrimSpace(value.JobID),
		MergeRequestID:     strings.TrimSpace(value.MergeRequestID),
		ApprovalID:         strings.TrimSpace(value.ApprovalID),
		BaseCommit:         strings.TrimSpace(value.BaseCommit),
		ParentHeadBefore:   strings.TrimSpace(value.ParentHeadBefore),
		ParentHeadAfter:    strings.TrimSpace(value.ParentHeadAfter),
		ChangedFiles:       cloneChangedFiles(value.ChangedFiles),
		AppliedPatchDigest: strings.TrimSpace(value.AppliedPatchDigest),
		CreatedAt:          strings.TrimSpace(value.CreatedAt),
	}
}

func cloneAcceptDecisions(values []domainjob.AcceptDecision) []domainjob.AcceptDecision {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.AcceptDecision, 0, len(values))
	for _, value := range values {
		cloned := cloneAcceptDecision(value)
		if cloned.ID == "" && cloned.ApprovalID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneConflictReport(value domainjob.ConflictReport) domainjob.ConflictReport {
	return domainjob.ConflictReport{
		ID:                     strings.TrimSpace(value.ID),
		ParentThreadID:         strings.TrimSpace(value.ParentThreadID),
		ChildRunID:             strings.TrimSpace(value.ChildRunID),
		JobID:                  strings.TrimSpace(value.JobID),
		BaseCommit:             strings.TrimSpace(value.BaseCommit),
		ParentHeadAtConflict:   strings.TrimSpace(value.ParentHeadAtConflict),
		ChildHeadAtConflict:    strings.TrimSpace(value.ChildHeadAtConflict),
		SourcePatchDigest:      strings.TrimSpace(value.SourcePatchDigest),
		ConflictFiles:          cloneChangedFiles(value.ConflictFiles),
		ConflictSummary:        strings.TrimSpace(value.ConflictSummary),
		CreatedAt:              strings.TrimSpace(value.CreatedAt),
		TouchedParentWorkspace: value.TouchedParentWorkspace,
	}
}

func cloneConflictReports(values []domainjob.ConflictReport) []domainjob.ConflictReport {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ConflictReport, 0, len(values))
	for _, value := range values {
		cloned := cloneConflictReport(value)
		if cloned.ID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneRepairPatchReview(value domainjob.RepairPatchReview) domainjob.RepairPatchReview {
	return domainjob.RepairPatchReview{
		ID:                 strings.TrimSpace(value.ID),
		ConflictReportID:   strings.TrimSpace(value.ConflictReportID),
		ParentThreadID:     strings.TrimSpace(value.ParentThreadID),
		ChildRunID:         strings.TrimSpace(value.ChildRunID),
		JobID:              strings.TrimSpace(value.JobID),
		RepairPatchDigest:  strings.TrimSpace(value.RepairPatchDigest),
		ExpectedParentHead: strings.TrimSpace(value.ExpectedParentHead),
		ChangedFiles:       cloneChangedFiles(value.ChangedFiles),
		DryRunStatus:       strings.TrimSpace(value.DryRunStatus),
		RejectedReason:     strings.TrimSpace(value.RejectedReason),
		CreatedAt:          strings.TrimSpace(value.CreatedAt),
		RepairPatch:        value.RepairPatch,
	}
}

func cloneRepairPatchReviews(values []domainjob.RepairPatchReview) []domainjob.RepairPatchReview {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.RepairPatchReview, 0, len(values))
	for _, value := range values {
		cloned := cloneRepairPatchReview(value)
		if cloned.ID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneRepairDecision(value domainjob.RepairDecision) domainjob.RepairDecision {
	return domainjob.RepairDecision{
		ID:                 strings.TrimSpace(value.ID),
		RepairReviewID:     strings.TrimSpace(value.RepairReviewID),
		ApprovalID:         strings.TrimSpace(value.ApprovalID),
		ParentHeadBefore:   strings.TrimSpace(value.ParentHeadBefore),
		ParentHeadAfter:    strings.TrimSpace(value.ParentHeadAfter),
		AppliedPatchDigest: strings.TrimSpace(value.AppliedPatchDigest),
		CreatedAt:          strings.TrimSpace(value.CreatedAt),
	}
}

func cloneRepairDecisions(values []domainjob.RepairDecision) []domainjob.RepairDecision {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.RepairDecision, 0, len(values))
	for _, value := range values {
		cloned := cloneRepairDecision(value)
		if cloned.ID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneChildTodoItem(value domainjob.ChildTodoItem) domainjob.ChildTodoItem {
	return domainjob.ChildTodoItem{
		ID:            strings.TrimSpace(value.ID),
		Content:       strings.TrimSpace(value.Content),
		Status:        strings.TrimSpace(value.Status),
		EvidenceIDs:   sortedStrings(value.EvidenceIDs),
		ParentTodoRef: strings.TrimSpace(value.ParentTodoRef),
		CreatedAt:     strings.TrimSpace(value.CreatedAt),
		UpdatedAt:     strings.TrimSpace(value.UpdatedAt),
	}
}

func cloneChildTodoItems(values []domainjob.ChildTodoItem) []domainjob.ChildTodoItem {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ChildTodoItem, 0, len(values))
	for _, value := range values {
		cloned := cloneChildTodoItem(value)
		if cloned.ID == "" && cloned.Content == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneChildTodoProjectionItem(value domainjob.ChildTodoProjectionItem) domainjob.ChildTodoProjectionItem {
	return domainjob.ChildTodoProjectionItem{
		ID:            strings.TrimSpace(value.ID),
		Content:       strings.TrimSpace(value.Content),
		Status:        strings.TrimSpace(value.Status),
		EvidenceIDs:   sortedStrings(value.EvidenceIDs),
		ParentTodoRef: strings.TrimSpace(value.ParentTodoRef),
	}
}

func cloneChildTodoProjectionItems(values []domainjob.ChildTodoProjectionItem) []domainjob.ChildTodoProjectionItem {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ChildTodoProjectionItem, 0, len(values))
	for _, value := range values {
		cloned := cloneChildTodoProjectionItem(value)
		if cloned.ID == "" && cloned.Content == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneChildTodoList(value domainjob.ChildTodoList) domainjob.ChildTodoList {
	return domainjob.ChildTodoList{
		ID:             strings.TrimSpace(value.ID),
		ParentThreadID: strings.TrimSpace(value.ParentThreadID),
		ChildThreadID:  strings.TrimSpace(value.ChildThreadID),
		ChildRunID:     strings.TrimSpace(value.ChildRunID),
		JobID:          strings.TrimSpace(value.JobID),
		Scope:          strings.TrimSpace(value.Scope),
		Items:          cloneChildTodoItems(value.Items),
		CreatedAt:      strings.TrimSpace(value.CreatedAt),
		UpdatedAt:      strings.TrimSpace(value.UpdatedAt),
		SourceTurnID:   strings.TrimSpace(value.SourceTurnID),
	}
}

func cloneChildTodoLists(values []domainjob.ChildTodoList) []domainjob.ChildTodoList {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ChildTodoList, 0, len(values))
	for _, value := range values {
		cloned := cloneChildTodoList(value)
		if cloned.ID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneChildTodoProjection(value domainjob.ChildTodoProjection) domainjob.ChildTodoProjection {
	return domainjob.ChildTodoProjection{
		ID:              strings.TrimSpace(value.ID),
		ParentThreadID:  strings.TrimSpace(value.ParentThreadID),
		ChildThreadID:   strings.TrimSpace(value.ChildThreadID),
		ChildRunID:      strings.TrimSpace(value.ChildRunID),
		JobID:           strings.TrimSpace(value.JobID),
		ChildTodoListID: strings.TrimSpace(value.ChildTodoListID),
		ProjectedItems:  cloneChildTodoProjectionItems(value.ProjectedItems),
		Summary:         strings.TrimSpace(value.Summary),
		EvidenceIDs:     sortedStrings(value.EvidenceIDs),
		Status:          strings.TrimSpace(value.Status),
		CreatedAt:       strings.TrimSpace(value.CreatedAt),
		AcceptedAt:      strings.TrimSpace(value.AcceptedAt),
		RejectedAt:      strings.TrimSpace(value.RejectedAt),
	}
}

func cloneChildTodoProjections(values []domainjob.ChildTodoProjection) []domainjob.ChildTodoProjection {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ChildTodoProjection, 0, len(values))
	for _, value := range values {
		cloned := cloneChildTodoProjection(value)
		if cloned.ID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneAcceptedProjectionItem(value domainjob.AcceptedProjectionItem) domainjob.AcceptedProjectionItem {
	return domainjob.AcceptedProjectionItem{
		ChildTodoID:          strings.TrimSpace(value.ChildTodoID),
		ParentTodoID:         strings.TrimSpace(value.ParentTodoID),
		Action:               strings.TrimSpace(value.Action),
		PreviousParentStatus: strings.TrimSpace(value.PreviousParentStatus),
		NextParentStatus:     strings.TrimSpace(value.NextParentStatus),
		EvidenceIDs:          sortedStrings(value.EvidenceIDs),
	}
}

func cloneAcceptedProjectionItems(values []domainjob.AcceptedProjectionItem) []domainjob.AcceptedProjectionItem {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.AcceptedProjectionItem, 0, len(values))
	for _, value := range values {
		cloned := cloneAcceptedProjectionItem(value)
		if cloned.ChildTodoID == "" && cloned.ParentTodoID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneSkippedProjectionItem(value domainjob.SkippedProjectionItem) domainjob.SkippedProjectionItem {
	return domainjob.SkippedProjectionItem{
		ChildTodoID:  strings.TrimSpace(value.ChildTodoID),
		ParentTodoID: strings.TrimSpace(value.ParentTodoID),
		Reason:       strings.TrimSpace(value.Reason),
		EvidenceIDs:  sortedStrings(value.EvidenceIDs),
	}
}

func cloneSkippedProjectionItems(values []domainjob.SkippedProjectionItem) []domainjob.SkippedProjectionItem {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.SkippedProjectionItem, 0, len(values))
	for _, value := range values {
		cloned := cloneSkippedProjectionItem(value)
		if cloned.ChildTodoID == "" && cloned.Reason == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func cloneProjectionDecision(value domainjob.ProjectionDecision) domainjob.ProjectionDecision {
	return domainjob.ProjectionDecision{
		ID:                           strings.TrimSpace(value.ID),
		ParentThreadID:               strings.TrimSpace(value.ParentThreadID),
		ChildThreadID:                strings.TrimSpace(value.ChildThreadID),
		ChildRunID:                   strings.TrimSpace(value.ChildRunID),
		JobID:                        strings.TrimSpace(value.JobID),
		ProjectionID:                 strings.TrimSpace(value.ProjectionID),
		Decision:                     strings.TrimSpace(value.Decision),
		ApprovalID:                   strings.TrimSpace(value.ApprovalID),
		AcceptedItems:                cloneAcceptedProjectionItems(value.AcceptedItems),
		SkippedItems:                 cloneSkippedProjectionItems(value.SkippedItems),
		ParentTodosBeforeDigest:      strings.TrimSpace(value.ParentTodosBeforeDigest),
		ParentTodosAfterDigest:       strings.TrimSpace(value.ParentTodosAfterDigest),
		ExpectedParentTodosUpdatedAt: strings.TrimSpace(value.ExpectedParentTodosUpdatedAt),
		CreatedAt:                    strings.TrimSpace(value.CreatedAt),
	}
}

func cloneProjectionDecisions(values []domainjob.ProjectionDecision) []domainjob.ProjectionDecision {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ProjectionDecision, 0, len(values))
	for _, value := range values {
		cloned := cloneProjectionDecision(value)
		if cloned.ID == "" {
			continue
		}
		out = append(out, cloned)
	}
	return out
}

func appendOrReplaceChildTodoList(values []domainjob.ChildTodoList, value domainjob.ChildTodoList) []domainjob.ChildTodoList {
	out := cloneChildTodoLists(values)
	next := cloneChildTodoList(value)
	if next.ID == "" {
		return out
	}
	for index := range out {
		if out[index].ID == next.ID {
			out[index] = next
			return out
		}
	}
	return append(out, next)
}

func appendOrReplaceChildTodoProjection(values []domainjob.ChildTodoProjection, value domainjob.ChildTodoProjection) []domainjob.ChildTodoProjection {
	out := cloneChildTodoProjections(values)
	next := cloneChildTodoProjection(value)
	if next.ID == "" {
		return out
	}
	for index := range out {
		if out[index].ID == next.ID {
			out[index] = next
			return out
		}
	}
	return append(out, next)
}

func appendOrReplaceProjectionDecision(values []domainjob.ProjectionDecision, value domainjob.ProjectionDecision) []domainjob.ProjectionDecision {
	out := cloneProjectionDecisions(values)
	next := cloneProjectionDecision(value)
	if next.ID == "" {
		return out
	}
	for index := range out {
		if out[index].ID == next.ID {
			out[index] = next
			return out
		}
	}
	return append(out, next)
}

func cloneSteerMessages(values []domainjob.SteerMessage) []domainjob.SteerMessage {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.SteerMessage, len(values))
	copy(out, values)
	return out
}

func clonePauseRequests(values []domainjob.PauseRequest) []domainjob.PauseRequest {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.PauseRequest, len(values))
	copy(out, values)
	return out
}

func samePauseRequestReplayV1(existing domainjob.PauseRequest, request domainjob.PauseRequest, requestedAtProvided bool) bool {
	if strings.TrimSpace(existing.ID) != strings.TrimSpace(request.ID) ||
		strings.TrimSpace(existing.ParentThreadID) != strings.TrimSpace(request.ParentThreadID) ||
		strings.TrimSpace(existing.ChildRunID) != strings.TrimSpace(request.ChildRunID) ||
		strings.TrimSpace(existing.JobID) != strings.TrimSpace(request.JobID) ||
		strings.TrimSpace(existing.Status) != strings.TrimSpace(request.Status) ||
		strings.TrimSpace(existing.SourceTurnID) != strings.TrimSpace(request.SourceTurnID) ||
		strings.TrimSpace(existing.RejectedReason) != strings.TrimSpace(request.RejectedReason) {
		return false
	}
	return !requestedAtProvided || strings.TrimSpace(existing.RequestedAt) == strings.TrimSpace(request.RequestedAt)
}

func exactPauseSettlementReplayV1(existing domainjob.PauseRequest, status string, at string, reason string, token domainjob.ResumeToken) bool {
	status = strings.TrimSpace(status)
	at = strings.TrimSpace(at)
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	switch status {
	case "paused":
		return strings.TrimSpace(existing.PausedAt) == at &&
			strings.TrimSpace(existing.ResumeToken) == strings.TrimSpace(token.ResumeToken) &&
			strings.TrimSpace(existing.ResumeTokenIssuedAt) == strings.TrimSpace(token.IssuedAt) &&
			strings.TrimSpace(existing.ResumeTokenExpiresAt) == strings.TrimSpace(token.ExpiresAt)
	case "resumed":
		return strings.TrimSpace(existing.ResumedAt) == at && strings.TrimSpace(existing.ResumeToken) == ""
	case "expired", "rejected":
		return strings.TrimSpace(existing.RejectedReason) == reason && strings.TrimSpace(existing.ResumeToken) == ""
	default:
		return false
	}
}

func expireQueuedSteerMessages(values []domainjob.SteerMessage, at string, reason string) []domainjob.SteerMessage {
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	out := cloneSteerMessages(values)
	for index := range out {
		if strings.TrimSpace(out[index].Status) != "queued" {
			continue
		}
		out[index].Status = "expired"
		out[index].RejectedReason = reason
		out[index].Text = ""
		out[index].SourceTurnID = ""
		out[index].SourceToolCallID = ""
		if strings.TrimSpace(out[index].AdmittedAt) == "" {
			out[index].AdmittedAt = strings.TrimSpace(at)
		}
	}
	return out
}

func expireOpenPauseRequests(values []domainjob.PauseRequest, _ string, reason string) []domainjob.PauseRequest {
	reason = domainjob.NormalizeOperationalReasonV1(reason)
	out := clonePauseRequests(values)
	for index := range out {
		switch strings.TrimSpace(out[index].Status) {
		case "requested", "paused":
			out[index].Status = "expired"
			out[index].RejectedReason = reason
			out[index].ResumeToken = ""
			out[index].ResumedAt = ""
		}
	}
	return out
}

func childRunState(record Record) domainjob.ChildRunState {
	state := domainjob.ChildRunState{}
	for _, message := range record.Steers {
		switch strings.TrimSpace(message.Status) {
		case "queued":
			state.PendingSteers++
		case "admitted":
			state.AdmittedSteers++
		}
		if strings.TrimSpace(message.CreatedAt) != "" && message.CreatedAt > state.LastSteerAt {
			state.LastSteerAt = message.CreatedAt
		}
		if strings.TrimSpace(message.AdmittedAt) != "" && message.AdmittedAt > state.LastSteerAt {
			state.LastSteerAt = message.AdmittedAt
		}
	}
	state.SteerCount = len(record.Steers)
	status := strings.TrimSpace(record.Status)
	state.CanAcceptSteer = record.Background &&
		(status == string(domainjob.StatusQueued) || status == string(domainjob.StatusRunning)) &&
		strings.TrimSpace(record.ChildThreadID) != ""
	return state
}

func childRunPauseState(record Record) domainjob.ChildRunPauseState {
	state := domainjob.ChildRunPauseState{}
	latest := domainjob.PauseRequest{}
	for _, request := range record.PauseRequests {
		if latest.ID == "" || strings.TrimSpace(request.RequestedAt) >= strings.TrimSpace(latest.RequestedAt) {
			latest = request
		}
		if strings.TrimSpace(request.RequestedAt) != "" && request.RequestedAt > state.RequestedAt {
			state.RequestedAt = request.RequestedAt
		}
		if strings.TrimSpace(request.PausedAt) != "" && request.PausedAt > state.LastPausedAt {
			state.LastPausedAt = request.PausedAt
		}
		if strings.TrimSpace(request.ResumedAt) != "" && request.ResumedAt > state.LastResumedAt {
			state.LastResumedAt = request.ResumedAt
		}
		if strings.TrimSpace(request.PausedAt) != "" {
			state.PauseCount++
		}
	}
	if strings.TrimSpace(latest.ID) != "" {
		state.PauseRequestID = strings.TrimSpace(latest.ID)
		state.Status = strings.TrimSpace(latest.Status)
		state.RequestedAt = strings.TrimSpace(latest.RequestedAt)
		if state.Status == "paused" {
			state.ResumeTokenIssuedAt = strings.TrimSpace(latest.ResumeTokenIssuedAt)
			state.ResumeTokenExpiresAt = strings.TrimSpace(latest.ResumeTokenExpiresAt)
		}
	}
	status := strings.TrimSpace(record.Status)
	state.Paused = status == string(domainjob.StatusPaused) && state.Status == "paused"
	state.CanPause = record.Background &&
		!terminalStatus(status) &&
		strings.TrimSpace(record.ChildThreadID) != "" &&
		(status == string(domainjob.StatusQueued) || status == string(domainjob.StatusRunning))
	state.CanResume = record.Background &&
		!terminalStatus(status) &&
		strings.TrimSpace(record.ChildThreadID) != "" &&
		status == string(domainjob.StatusPaused) && state.Status == "paused"
	return state
}

func applyRuntimeLeaseDefaults(record *Record, now string) {
	if record == nil {
		return
	}
	if record.StaleAfterMs <= 0 {
		record.StaleAfterMs = defaultTaskJobStaleAfter.Milliseconds()
	}
	if !shouldRefreshRuntimeLease(record.Status) {
		return
	}
	if strings.TrimSpace(record.LastHeartbeatAt) == "" {
		record.LastHeartbeatAt = now
	}
	if strings.TrimSpace(record.LeaseOwner) == "" {
		record.LeaseOwner = defaultRuntimeLeaseOwner()
	}
	if strings.TrimSpace(record.LeaseExpiresAt) == "" {
		record.LeaseExpiresAt = runtimeLeaseExpiresAt(now)
	}
}

func refreshRuntimeLease(record *Record, now string) {
	if record == nil || !shouldRefreshRuntimeLease(record.Status) {
		return
	}
	if strings.TrimSpace(now) == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}
	record.LastHeartbeatAt = now
	if strings.TrimSpace(record.LeaseOwner) == "" {
		record.LeaseOwner = defaultRuntimeLeaseOwner()
	}
	record.LeaseExpiresAt = runtimeLeaseExpiresAt(now)
	if record.StaleAfterMs <= 0 {
		record.StaleAfterMs = defaultTaskJobStaleAfter.Milliseconds()
	}
}

func runtimeLeaseExpiresAt(now string) string {
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(now))
	if err != nil || at.IsZero() {
		at = time.Now().UTC()
	}
	return at.Add(defaultTaskJobLeaseTimeout).UTC().Format(time.RFC3339Nano)
}

func shouldRefreshRuntimeLease(status string) bool {
	switch strings.TrimSpace(status) {
	case string(domainjob.StatusQueued), string(domainjob.StatusRunning), string(domainjob.StatusPauseRequested), string(domainjob.StatusResumeRequested), string(domainjob.StatusResuming):
		return true
	default:
		return false
	}
}

func runtimeRestartRecoverableStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case string(domainjob.StatusQueued),
		string(domainjob.StatusRunning),
		string(domainjob.StatusPauseRequested),
		string(domainjob.StatusPaused),
		string(domainjob.StatusResumeRequested),
		string(domainjob.StatusResuming):
		return true
	default:
		return false
	}
}

func cloneOptionalInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneUsage(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func terminalStatus(status string) bool {
	return domainjob.TerminalStatusV1(status)
}

func externallyStoppedStatus(status string) bool {
	return domainjob.ExternallyStoppedStatusV1(status)
}

func validateOperationalMetadataUpdateV1(request UpdateRequest) error {
	if strings.TrimSpace(request.AutoContinueStatus) == "" &&
		(strings.TrimSpace(request.AutoContinueTurnID) != "" || strings.TrimSpace(request.AutoContinueReason) != "" || strings.TrimSpace(request.AutoContinueError) != "") {
		return errors.New("auto-continue metadata requires an explicit closed status")
	}
	if strings.TrimSpace(request.CompletionDeliveryStatus) == "" &&
		(strings.TrimSpace(request.CompletionDeliveryID) != "" || strings.TrimSpace(request.CompletionDeliveryItemID) != "" ||
			strings.TrimSpace(request.CompletionDeliveryReason) != "" || strings.TrimSpace(request.CompletionDeliveryError) != "") {
		return errors.New("completion-delivery metadata requires an explicit closed status")
	}
	if strings.TrimSpace(request.RecoveryStatus) == "" && strings.TrimSpace(request.CompletionDeliveryStatus) == "" &&
		(strings.TrimSpace(request.RecoveryReason) != "" || strings.TrimSpace(request.DeadLetterReason) != "") {
		return errors.New("recovery metadata requires an explicit closed recovery or delivery status")
	}
	return nil
}

func validateOperationalIdentityUpdateV1(record Record, request UpdateRequest) error {
	for _, pair := range [][2]string{
		{record.AutoContinueTurnID, request.AutoContinueTurnID},
		{record.CompletionDeliveryID, request.CompletionDeliveryID},
		{record.CompletionDeliveryItemID, request.CompletionDeliveryItemID},
	} {
		before, after := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
		if before != "" && after != "" && before != after {
			return errors.New("job operational lifecycle identity is immutable")
		}
	}
	return nil
}

func exactOperationalStatusReplayV1(record Record, request UpdateRequest) bool {
	hasStatus := false
	for _, pair := range [][2]string{
		{record.AutoContinueStatus, request.AutoContinueStatus},
		{record.CompletionDeliveryStatus, request.CompletionDeliveryStatus},
		{record.RecoveryStatus, request.RecoveryStatus},
	} {
		requested := strings.TrimSpace(pair[1])
		if requested == "" {
			continue
		}
		hasStatus = true
		if strings.TrimSpace(pair[0]) != requested {
			return false
		}
	}
	if !hasStatus {
		return false
	}
	for _, pair := range [][2]string{
		{record.AutoContinueTurnID, request.AutoContinueTurnID},
		{record.CompletionDeliveryID, request.CompletionDeliveryID},
		{record.CompletionDeliveryItemID, request.CompletionDeliveryItemID},
	} {
		if requested := strings.TrimSpace(pair[1]); requested != "" && strings.TrimSpace(pair[0]) != requested {
			return false
		}
	}
	remainder := request
	remainder.AutoContinueStatus = ""
	remainder.AutoContinueTurnID = ""
	remainder.AutoContinueReason = ""
	remainder.AutoContinueError = ""
	remainder.CompletionDeliveryID = ""
	remainder.CompletionDeliveryStatus = ""
	remainder.CompletionDeliveryItemID = ""
	remainder.CompletionDeliveryReason = ""
	remainder.CompletionDeliveryError = ""
	remainder.RecoveryStatus = ""
	remainder.RecoveryReason = ""
	remainder.DeadLetterReason = ""
	return reflect.DeepEqual(remainder, UpdateRequest{})
}

func lateCompletionSuppressionReason(existingStatus string, attemptedStatus string) string {
	return "late_completion_suppressed"
}

func elapsedMilliseconds(start, end string) int {
	startTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(start))
	if err != nil {
		return 0
	}
	endTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(end))
	if err != nil || endTime.Before(startTime) {
		return 0
	}
	return int(endTime.Sub(startTime).Milliseconds())
}

func readRecords(
	ctx context.Context,
	root string,
	artifactAuthorityRoot string,
	verifier ChildCompletionReceiptVerifier,
	legacyLineageWitness *FrozenLegacyTypeScriptLineageWitnessV1,
	normalizeSemanticStage bool,
	minimumSequence int,
	preserved *restartPreservationV1,
) ([]Record, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if minimumSequence < 0 {
		return nil, 0, errors.New("child-run sequence floor is invalid")
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		return nil, 0, err
	}
	if err := preserved.validateInventory(inventory); err != nil {
		return nil, 0, err
	}
	if normalizeSemanticStage && legacyLineageWitness != nil {
		if err := legacyLineageWitness.verifySourceInventoryV1(inventory); err != nil {
			return nil, 0, errors.New("legacy TypeScript child-run lineage witness source inventory changed")
		}
	}
	if !inventory.RootExists {
		return []Record{}, minimumSequence, nil
	}
	legacyTypeScriptPresent := false
	for _, entry := range inventory.Entries {
		if entry.Kind == ChildRunInventoryLegacyTypeScriptRecordV1 {
			legacyTypeScriptPresent = true
			break
		}
	}
	if normalizeSemanticStage && legacyTypeScriptPresent {
		if legacyLineageWitness == nil {
			return nil, 0, errors.New("legacy TypeScript child-run lineage witness is not bound to the source inventory")
		}
	}
	records := []Record{}
	residue := []ChildRunInventoryEntryV1{}
	legacyTypeScriptRecords := []ChildRunInventoryEntryV1{}
	legacyProjectionSourceByTarget := map[string]ChildRunInventoryEntryV1{}
	securityBoundSources := map[string]Record{}
	seenIDs := map[string]bool{}
	maxSeq := minimumSequence
	for _, entry := range inventory.Entries {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		if entry.Kind == ChildRunInventoryLegacyTypeScriptRecordV1 {
			if !normalizeSemanticStage {
				return nil, 0, errors.New("child-run storage contains a retired TypeScript record requiring semantic migration")
			}
			legacyTypeScriptRecords = append(legacyTypeScriptRecords, entry)
			residue = append(residue, entry)
			continue
		}
		if entry.Kind != ChildRunInventoryRecordV1 {
			if preserved.ownsJob(entry.JobID) {
				continue
			}
			if !normalizeSemanticStage {
				return nil, 0, errors.New("child-run storage contains legacy private residue requiring semantic migration")
			}
			residue = append(residue, entry)
			continue
		}
		data, err := readChildRunInventoryEntryV1(root, entry)
		if err != nil {
			return nil, 0, err
		}
		if err := domainjsonstrict.Validate(data, domainjsonstrict.Options{RequireObject: true, MaxBytes: 16 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 8 * 1024 * 1024}); err != nil {
			return nil, 0, errors.New("child-run record JSON is invalid")
		}
		data, err = compactEmbeddedForegroundHandoffReceiptV1(data)
		if err != nil {
			return nil, 0, errors.New("child-run foreground handoff receipt is invalid")
		}
		var record Record
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return nil, 0, errors.New("child-run record shape is invalid")
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, 0, errors.New("child-run record shape is invalid")
		}
		if !validPersistedJobID(record.ID) || entry.Name != record.ID+".json" || seenIDs[record.ID] {
			return nil, 0, errors.New("child-run record identity is invalid")
		}
		held := preserved.ownsJob(record.ID)
		if preserved != nil && preserved.ownsThreads(record) != held {
			return nil, 0, errors.New("child-run restart dependency scope changed")
		}
		if held {
			digests, err := restartRecordDigestsV1([]Record{record})
			if err != nil || digests[record.ID] != preserved.records[record.ID] || record.ChildSeq <= 0 {
				return nil, 0, errors.New("child-run original record cannot be backfilled or rewritten")
			}
		}
		expectedArtifact := expectedJobArtifactPath(artifactAuthorityRoot, record.ID)
		// ArtifactPath is retired, untrusted legacy metadata. Live activation
		// still requires exact current-host authority. The isolated semantic
		// stage must not dereference a copied record's stale absolute path: it
		// clears the field below, while artifact deletion is driven exclusively
		// by the stage-root inventory's identity- and SHA-bound entries.
		// Authority-bound records remain immutable because
		// ValidateSemanticMigrationSourceV1 rejects any field that normalization
		// would change.
		if !normalizeSemanticStage && strings.TrimSpace(record.ArtifactPath) != "" && record.ArtifactPath != expectedArtifact {
			return nil, 0, errors.New("child-run artifact path is outside host authority")
		}
		if err := domainjob.ValidateStatusV1(record.Status); err != nil {
			return nil, 0, err
		}
		if normalizeSemanticStage && !held {
			if err := domainjob.ValidateSemanticMigrationSourceV1(record); err != nil {
				return nil, 0, err
			}
			if record.SecurityBinding != nil {
				securityBoundSources[record.ID] = cloneRecord(record)
			}
			record = domainjob.NormalizePersistedRecordV1(record)
			if status := strings.TrimSpace(record.PauseState.Status); status != "" {
				if err := domainjob.ValidatePauseRequestStatusV1(status); err != nil {
					return nil, 0, err
				}
			}
			record.PauseState = childRunPauseState(record)
			record.SteerState = childRunState(record)
		}
		if err := validateDurableChildRunRecord(ctx, record, verifier); err != nil {
			return nil, 0, err
		}
		if !normalizeSemanticStage || held {
			if err := domainjob.ValidatePersistedProjectionV1(record); err != nil {
				return nil, 0, err
			}
		}
		seenIDs[record.ID] = true
		records = append(records, record)
		if seq := sequence(record.ID); seq > maxSeq {
			maxSeq = seq
		}
	}
	for _, entry := range legacyTypeScriptRecords {
		data, err := readChildRunInventoryEntryV1(root, entry)
		if err != nil {
			return nil, 0, err
		}
		if maxSeq == int(^uint(0)>>1) {
			return nil, 0, errors.New("child-run sequence is exhausted")
		}
		maxSeq++
		record, err := projectLegacyTypeScriptChildRunV1(data, entry, fmt.Sprintf("job-%d", maxSeq), legacyLineageWitness)
		if err != nil {
			return nil, 0, err
		}
		if preserved.ownsThreads(record) || preserved.ownsJob(record.ID) {
			return nil, 0, ErrRestartPreserved
		}
		if err := validateDurableChildRunRecord(ctx, record, verifier); err != nil {
			return nil, 0, err
		}
		legacyProjectionSourceByTarget[record.ID+".json"] = entry
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		left := sequence(records[i].ID)
		right := sequence(records[j].ID)
		if left != 0 && right != 0 && left != right {
			return left < right
		}
		return records[i].ID < records[j].ID
	})
	nextChildSeq := map[string]int{}
	for _, record := range records {
		parentThreadID := strings.TrimSpace(record.ParentThreadID)
		if record.ChildSeq <= 0 {
			continue
		}
		if record.ChildSeq > nextChildSeq[parentThreadID] {
			nextChildSeq[parentThreadID] = record.ChildSeq
		}
	}
	for index := range records {
		parentThreadID := strings.TrimSpace(records[index].ParentThreadID)
		if records[index].ChildSeq > 0 {
			continue
		}
		nextChildSeq[parentThreadID]++
		records[index].ChildSeq = nextChildSeq[parentThreadID]
	}
	if normalizeSemanticStage {
		projectedWrites := make([][]byte, len(records))
		legacyProjectionReadbacks := []legacyTypeScriptProjectionReadbackV1{}
		for index := range records {
			if preserved.ownsJob(records[index].ID) {
				continue
			}
			records[index] = domainjob.NormalizePersistedRecordV1(records[index])
			records[index].PauseState = childRunPauseState(records[index])
			records[index].SteerState = childRunState(records[index])
			if err := domainjob.ValidatePersistedProjectionV1(records[index]); err != nil {
				return nil, 0, err
			}
			if source, securityBound := securityBoundSources[records[index].ID]; securityBound {
				if !reflect.DeepEqual(source, records[index]) {
					return nil, 0, errors.New("security-bound child-run record cannot be backfilled or rewritten")
				}
				// Byte-preserve already-bound records. Reformatting their JSON would
				// be an unauthorized owner mutation even when decoded values match.
				continue
			}
			data, err := json.MarshalIndent(&records[index], "", "  ")
			if err != nil {
				return nil, 0, err
			}
			projectedWrites[index] = append(data, '\n')
			if source, legacyProjection := legacyProjectionSourceByTarget[records[index].ID+".json"]; legacyProjection {
				legacyProjectionReadbacks = append(legacyProjectionReadbacks, legacyTypeScriptProjectionReadbackV1{
					SourceName: source.Name, TargetName: records[index].ID + ".json", Bytes: projectedWrites[index],
				})
			}
		}
		if err := ValidateChildRunInventoryV1(root, inventory); err != nil {
			return nil, 0, err
		}
		for index := range records {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
			if projectedWrites[index] == nil {
				continue
			}
			if err := writeFileAtomic(filepath.Join(root, records[index].ID+".json"), projectedWrites[index], 0o600); err != nil {
				return nil, 0, err
			}
		}
		if err := validateLegacyTypeScriptProjectionReadbackV1(root, legacyProjectionReadbacks, false); err != nil {
			return nil, 0, err
		}
		for _, entry := range residue {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
			var removeErr error
			if entry.Kind == ChildRunInventoryLegacyTypeScriptRecordV1 {
				removeErr = removeLegacyTypeScriptChildRunSourceV1(root, entry)
			} else {
				removeErr = removeChildRunInventoryResidueV1(root, entry)
			}
			if removeErr != nil {
				return nil, 0, removeErr
			}
		}
		if err := validateLegacyTypeScriptProjectionReadbackV1(root, legacyProjectionReadbacks, true); err != nil {
			return nil, 0, err
		}
	}
	if preserved != nil {
		current, err := BuildChildRunInventoryV1(root)
		if err != nil {
			return nil, 0, err
		}
		if err := preserved.validateInventory(current); err != nil {
			return nil, 0, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	return records, maxSeq, nil
}

// ForegroundChildHandoffReceiptV1 deliberately accepts only its canonical
// standalone wire encoding. Child-run files are human-readable JSON, so the
// outer MarshalIndent adds whitespace inside that embedded object. Compact
// only this one value before strict record decoding; unknown top-level fields
// remain present and are still rejected by DisallowUnknownFields.
func compactEmbeddedForegroundHandoffReceiptV1(data []byte) ([]byte, error) {
	if !bytes.Contains(data, []byte(`"foregroundChildHandoffReceipt"`)) {
		return data, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	raw, ok := fields["foregroundChildHandoffReceipt"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return data, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}
	fields["foregroundChildHandoffReceipt"] = append(json.RawMessage(nil), compact.Bytes()...)
	return json.Marshal(fields)
}

func validPersistedJobID(value string) bool {
	value = strings.TrimSpace(value)
	seq := sequence(value)
	return seq > 0 && value == fmt.Sprintf("job-%d", seq)
}

func expectedJobArtifactPath(root string, jobID string) string {
	return filepath.ToSlash(filepath.Join(root, jobID+".log"))
}

func canonicalJobRootWithoutCreate(value string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(value) == "" {
		return "", errors.New("child-run root is required")
	}
	absolute = filepath.Clean(absolute)
	existing := absolute
	missing := []string{}
	for {
		info, statErr := os.Lstat(existing)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				resolved, resolveErr := filepath.EvalSymlinks(existing)
				if resolveErr != nil {
					return "", resolveErr
				}
				existing = resolved
			}
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", statErr
		}
		missing = append(missing, filepath.Base(existing))
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, missing[index])
	}
	return filepath.Clean(resolved), nil
}

func sequence(id string) int {
	var seq int
	if _, err := fmt.Sscanf(id, "job-%d", &seq); err != nil {
		return 0
	}
	return seq
}

func safeID(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	cleaned := replacer.Replace(strings.TrimSpace(value))
	cleaned = strings.ReplaceAll(cleaned, "..", "__")
	if cleaned == "" || cleaned == "." {
		return "_"
	}
	return cleaned
}
