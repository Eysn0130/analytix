package server

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	subagentstartupport "analytix.local/runtime-go/internal/ports/subagentstartup"
)

const (
	caseCompactionCommitPhaseBeforeLock  = "before_lock"
	caseCompactionCommitPhaseAfterApply  = "after_apply"
	caseCompactionCommitPhaseBeforeWrite = "before_write"
)

func (s *DurableEventSessionStore) ReadThreadStartBaseline(threadID string) (map[string]any, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, digest, err := s.readThreadMutationBaselineNoLock(threadID)
	if err != nil || thread == nil {
		return thread, digest, err
	}
	// A terminal CAS may be durable while its outbox or usage settlement is
	// still missing. Resolve that existing authority before a new turn can
	// register a context or append another turn behind the unsettled result.
	if err := s.settleGeneralTerminalPublicationsBeforeHistoryMutationNoLock(threadID); err != nil {
		return nil, "", err
	}
	return thread, digest, nil
}

func (s *DurableEventSessionStore) ReadThreadMutationBaseline(threadID string) (map[string]any, string, error) {
	return s.readThreadMutationBaseline(threadID)
}

func (s *DurableEventSessionStore) readThreadMutationBaseline(threadID string) (map[string]any, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readThreadMutationBaselineNoLock(threadID)
}

func (s *DurableEventSessionStore) readThreadMutationBaselineNoLock(threadID string) (map[string]any, string, error) {
	thread, err := s.readThreadNoLock(threadID)
	if err != nil || thread == nil {
		return thread, "", err
	}
	digest, err := threadapp.ReadMutationBaseline(thread, threadID)
	if err != nil {
		return nil, "", err
	}
	view, err := s.caseThreadView(threadID, thread)
	return view, digest, err
}

func (s *DurableEventSessionStore) AppendTurnToThread(threadID string, turn map[string]any, providerID string, threadPatch map[string]any) error {
	return s.appendTurnToThread(threadID, turn, providerID, threadPatch, "", "")
}

func (s *DurableEventSessionStore) AppendTurnToThreadIfBaseline(threadID string, turn map[string]any, providerID string, threadPatch map[string]any, expectedWorkspace, expectedDigest string) error {
	return s.appendTurnToThread(threadID, turn, providerID, threadPatch, expectedWorkspace, expectedDigest)
}

func (s *DurableEventSessionStore) appendTurnToThread(threadID string, turn map[string]any, providerID string, threadPatch map[string]any, expectedWorkspace, expectedDigest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedDigest != "" {
		if err := s.settleGeneralTerminalPublicationsBeforeHistoryMutationNoLock(threadID); err != nil {
			return err
		}
	}
	thread, err := s.readThreadNoLock(threadID)
	if err == nil && thread == nil {
		err = os.ErrNotExist
	}
	if err == nil {
		thread, err = turnstartapp.ApplyAppendMutation(turnstartapp.AppendMutationInput{
			Thread: thread, Turn: turn, ThreadID: threadID, ProviderID: providerID, ThreadPatch: threadPatch,
			ExpectedWorkspace: expectedWorkspace, ExpectedDigest: expectedDigest, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	if err == nil {
		err = s.upsertThreadNoLock(thread, false)
	}
	return err
}

func (s *DurableEventSessionStore) RewindThread(threadID, turnID string) (map[string]any, error) {
	return s.RewindThreadIfBaseline(threadID, turnID, "")
}

func (s *DurableEventSessionStore) RewindThreadIfBaseline(threadID, turnID, expectedDigest string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.caseThreads != nil && s.caseThreads.IsCaseThread(threadID) {
		return nil, threadapp.ErrCaseRewindSignedArchive
	}
	if err := s.settleGeneralTerminalPublicationsBeforeHistoryRewriteNoLock(threadID); err != nil {
		return nil, err
	}
	thread, err := s.readThreadNoLock(threadID)
	if err == nil && thread == nil {
		err = os.ErrNotExist
	}
	if err == nil {
		thread, err = s.caseThreadView(threadID, thread)
	}
	if err != nil {
		return nil, err
	}
	plan, err := threadapp.ApplyRewindMutation(thread, threadID, turnID, expectedDigest, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	if err := s.upsertThreadNoLock(plan.Thread, false); err != nil {
		return nil, err
	}
	return cloneMap(plan.Response), nil
}

func (s *DurableEventSessionStore) CommitRewindMutation(request threadapp.RewindMutationCommitRequest) (threadapp.RewindMutationCommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	caseBound := s.caseThreads != nil && s.caseThreads.IsCaseThread(request.ThreadID)
	if !caseBound {
		if err := s.settleGeneralTerminalPublicationsBeforeHistoryRewriteNoLock(request.ThreadID); err != nil {
			return threadapp.RewindMutationCommitResult{}, err
		}
	}
	return threadapp.CommitRewindMutationUnlessCaseBound(
		request, caseBound, s.readThreadNoLock, s.writeThreadMutationNoLock,
	)
}

func (s *DurableEventSessionStore) CommitCompaction(request threadapp.CompactionCommitRequest) (threadapp.CompactionCommitResult, error) {
	if hook := s.caseCompactionCommitHook; hook != nil {
		hook(caseCompactionCommitPhaseBeforeLock, request.ThreadID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	caseBound := s.caseThreads != nil && s.caseThreads.IsCaseThread(request.ThreadID)
	if caseBound != request.CaseBound {
		return threadapp.CompactionCommitResult{}, threadapp.ErrCaseCompactionRequiresTrustedArchive
	}
	if caseBound {
		thread, err := s.readThreadNoLock(request.ThreadID)
		if err != nil || thread == nil {
			return threadapp.CompactionCommitResult{}, errors.Join(threadapp.ErrCaseCompactionRequiresTrustedArchive, err)
		}
		inherited, err := threadapp.ActiveInheritedCompactionRecordV1(context.Background(), thread, s.caseThreads, s)
		if err != nil {
			return threadapp.CompactionCommitResult{}, err
		}
		if !reflect.DeepEqual(inherited, request.ActiveInheritedHistory) {
			return threadapp.CompactionCommitResult{}, threadapp.ErrCompactionBaselineConflict
		}
		prepared, committedAuthority, targetCommitted, err := validateCaseCompactionCASAuthorityV1(
			s.caseThreads, thread, request,
		)
		if err != nil {
			return threadapp.CompactionCommitResult{}, errors.Join(threadapp.ErrCaseCompactionRequiresTrustedArchive, err)
		}
		if err := s.settleGeneralTerminalPublicationsBeforeHistoryRewriteNoLock(request.ThreadID); err != nil {
			return threadapp.CompactionCommitResult{}, err
		}
		if hook := s.caseCompactionCommitHook; hook != nil {
			hook(caseCompactionCommitPhaseAfterApply, request.ThreadID)
		}
		if !targetCommitted {
			// Once the exact source baseline has been replayed under the same
			// thread mutex used by every durable mutation, stage and sign the
			// target while still holding that mutex. A concurrent goal, Todo, or
			// patch therefore either wins before this replay (and no target is
			// signed) or runs only after the signed target is durably pruned.
			commitContext := context.Background()
			if err := casethreadapp.RegisterRequired(commitContext, committedAuthority, prepared.SecurityContext); err != nil {
				return threadapp.CompactionCommitResult{}, errors.Join(threadapp.ErrCaseCompactionRequiresTrustedArchive, err)
			}
			if err := casethreadapp.CommitRequired(
				commitContext, committedAuthority, prepared.SecurityContext, prepared.EpochState, time.Now().UTC(),
			); err != nil {
				return threadapp.CompactionCommitResult{}, errors.Join(threadapp.ErrCaseCompactionRequiresTrustedArchive, err)
			}
		}
		if err := validateCaseCompactionCommittedTargetV1(committedAuthority, prepared, request); err != nil {
			return threadapp.CompactionCommitResult{}, errors.Join(threadapp.ErrCaseCompactionRequiresTrustedArchive, err)
		}
		if s.pendingCaseCompactions == nil {
			s.pendingCaseCompactions = map[string]threadapp.PreparedCompaction{}
		}
		s.pendingCaseCompactions[request.ThreadID] = prepared
		if hook := s.caseCompactionCommitHook; hook != nil {
			hook(caseCompactionCommitPhaseBeforeWrite, request.ThreadID)
		}
		result, commitErr := s.commitPreparedCaseCompactionNoLock(prepared)
		result.AuthorityCommitted = true
		if result.Committed {
			delete(s.pendingCaseCompactions, request.ThreadID)
		}
		return result, commitErr
	}
	if err := s.settleGeneralTerminalPublicationsBeforeHistoryRewriteNoLock(request.ThreadID); err != nil {
		return threadapp.CompactionCommitResult{}, err
	}
	return threadapp.CommitCompactionWithReadWrite(request, s.readThreadNoLock, s.writeThreadMutationNoLock)
}

func validateCaseCompactionCASAuthorityV1(
	authority casethreadapp.Authority,
	thread map[string]any,
	request threadapp.CompactionCommitRequest,
) (threadapp.PreparedCompaction, casethreadapp.CommittedAuthority, bool, error) {
	committed, ok := authority.(casethreadapp.CommittedAuthority)
	if !ok || !committed.CanExecute(request.ThreadID) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(request.CaseSourceContextDigest)) {
		return threadapp.PreparedCompaction{}, nil, false, errors.New("case compaction committed authority is unavailable")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	state, stateErr := domaincontextepoch.ParseState(thread["contextEpochState"])
	currentCommitted, found := committed.CommittedContext(request.ThreadID, current.TurnID)
	if err != nil || stateErr != nil || !found || current.ThreadID != request.ThreadID ||
		currentCommitted.SecurityContext != current || !reflect.DeepEqual(currentCommitted.EpochState, state) ||
		strings.TrimSpace(request.CaseSourceContextDigest) != current.ContextDigest {
		return threadapp.PreparedCompaction{}, nil, false, errors.New("case compaction current committed context changed before CAS")
	}
	prepared, err := threadapp.ApplyCompactionCommit(thread, request)
	if err != nil {
		return threadapp.PreparedCompaction{}, nil, false, err
	}
	targetCommitted, targetFound := committed.CommittedContext(
		request.ThreadID, strings.TrimSpace(request.ExpectedTurnID),
	)
	if targetFound && (targetCommitted.SecurityContext != prepared.SecurityContext ||
		!reflect.DeepEqual(targetCommitted.EpochState, prepared.EpochState)) {
		return threadapp.PreparedCompaction{}, nil, false, errors.New("case compaction target committed authority conflicts with the exact replay")
	}
	wantedOld, err := caseCompactionSourceAuthorityInventoryV1(request)
	if err != nil {
		return threadapp.PreparedCompaction{}, nil, false, err
	}
	if err := validateCaseCompactionCommittedInventoryV1(committed, request, targetFound); err != nil {
		return threadapp.PreparedCompaction{}, nil, false, err
	}
	turns, _ := thread["turns"].([]any)
	durable := map[string]map[string]any{}
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		turnID := strings.TrimSpace(stringField(turn, "id"))
		if turn == nil || turnID == "" || durable[turnID] != nil {
			return threadapp.PreparedCompaction{}, nil, false, errors.New("case compaction durable turn inventory is invalid")
		}
		durable[turnID] = turn
	}
	for turnID := range wantedOld {
		turn := durable[turnID]
		frozen, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		authorityRecord, authorityFound := committed.CommittedContext(request.ThreadID, turnID)
		if turn == nil || parseErr != nil || !authorityFound || frozen != authorityRecord.SecurityContext {
			return threadapp.PreparedCompaction{}, nil, false, errors.New("case compaction would orphan a committed context")
		}
	}
	return prepared, committed, targetFound, nil
}

func validateCaseCompactionCommittedTargetV1(
	authority casethreadapp.CommittedAuthority,
	prepared threadapp.PreparedCompaction,
	request threadapp.CompactionCommitRequest,
) error {
	target, found := authority.CommittedContext(request.ThreadID, strings.TrimSpace(request.ExpectedTurnID))
	if !found || target.SecurityContext != prepared.SecurityContext ||
		!reflect.DeepEqual(target.EpochState, prepared.EpochState) {
		return errors.New("case compaction target lacks exact committed authority readback")
	}
	return validateCaseCompactionCommittedInventoryV1(authority, request, true)
}

func validateCaseCompactionCommittedInventoryV1(
	authority casethreadapp.CommittedAuthority,
	request threadapp.CompactionCommitRequest,
	includeTarget bool,
) error {
	wanted, err := caseCompactionSourceAuthorityInventoryV1(request)
	if err != nil {
		return err
	}
	if includeTarget {
		wanted[strings.TrimSpace(request.ExpectedTurnID)] = true
	}
	liveIDs := casethreadapp.CommittedTurnIDs(authority, request.ThreadID)
	if len(liveIDs) != len(wanted) {
		return errors.New("case compaction authority inventory changed before CAS")
	}
	for _, value := range liveIDs {
		turnID := strings.TrimSpace(value)
		if !wanted[turnID] {
			return errors.New("case compaction authority inventory changed before CAS")
		}
		delete(wanted, turnID)
	}
	if len(wanted) != 0 {
		return errors.New("case compaction authority inventory is incomplete")
	}
	return nil
}

func caseCompactionSourceAuthorityInventoryV1(
	request threadapp.CompactionCommitRequest,
) (map[string]bool, error) {
	wanted := map[string]bool{}
	targetTurnID := strings.TrimSpace(request.ExpectedTurnID)
	for _, value := range request.CaseAuthorityTurnIDs {
		turnID := strings.TrimSpace(value)
		if turnID == "" || wanted[turnID] || turnID == targetTurnID {
			return nil, errors.New("case compaction authority inventory is invalid")
		}
		wanted[turnID] = true
	}
	if len(wanted) == 0 {
		return nil, errors.New("case compaction authority inventory is invalid")
	}
	return wanted, nil
}

func (s *DurableEventSessionStore) commitPreparedCaseCompactionNoLock(
	prepared threadapp.PreparedCompaction,
) (threadapp.CompactionCommitResult, error) {
	targetDigest, err := threadapp.CompactionBaselineDigest(prepared.Thread)
	if err != nil {
		return threadapp.CompactionCommitResult{}, err
	}
	result := threadapp.CompactionCommitResult{
		Result: prepared.Result, SecurityContext: prepared.SecurityContext, EpochState: prepared.EpochState,
	}
	writeErr := s.writeThreadMutationNoLock(prepared.Thread)
	reloaded, readErr := s.readThreadNoLock(prepared.Result.ThreadID)
	readbackDigest, digestErr := threadapp.CompactionBaselineDigest(reloaded)
	result.Committed = readErr == nil && digestErr == nil && readbackDigest == targetDigest
	if writeErr != nil || readErr != nil || digestErr != nil {
		return result, errors.Join(writeErr, readErr, digestErr)
	}
	if !result.Committed {
		return result, errors.New("durable case compaction readback diverged")
	}
	return result, nil
}

func (s *DurableEventSessionStore) writeThreadMutationNoLock(thread map[string]any) error {
	return s.upsertThreadNoLock(thread, false)
}

func (s *DurableEventSessionStore) AppendItemToTurn(threadID, turnID string, item map[string]any) error {
	if err := domainevent.ValidatePublicRecord(item); err != nil {
		return err
	}
	if stringField(item, "kind") == "assistant_text" {
		return errors.New("assistant text requires atomic terminal persistence")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return os.ErrNotExist
	}
	if err := turnapp.ValidateSecurityBoundAppend(thread, turnID, item); err != nil {
		return err
	}
	thread, ok := turnapp.AppendItemToTurn(turnapp.AppendItemInput{
		Thread:    thread,
		TurnID:    turnID,
		Item:      item,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if !ok {
		return os.ErrNotExist
	}
	return s.upsertThreadNoLock(thread, false)
}

// EnsureBackgroundDeliveryItemExact owns the atomic append-once boundary for
// background completion and delivery-ledger projections. The complete
// read/compare/write/readback sequence runs under the durable thread lock;
// ordinary AppendItemToTurn semantics remain unchanged.
func (s *DurableEventSessionStore) EnsureBackgroundDeliveryItemExact(
	threadID, turnID string,
	item map[string]any,
) (subagentapp.BackgroundDeliveryEnsureItemResultV1, error) {
	if err := domainevent.ValidatePublicRecord(item); err != nil {
		return "", err
	}
	if stringField(item, "kind") != "tool_progress" {
		return "", errors.New("background delivery item kind is invalid")
	}
	changed, err := s.ensureItemToTurnExact(threadID, turnID, item)
	if errors.Is(err, turnapp.ErrTurnItemIdentityConflict) {
		return subagentapp.BackgroundDeliveryItemConflictV1, nil
	}
	if err != nil {
		return "", err
	}
	if !changed {
		return subagentapp.BackgroundDeliveryItemExistingExactV1, nil
	}
	return subagentapp.BackgroundDeliveryItemInsertedV1, nil
}

// EnsureRecoveredParentToolSettlementExact owns the atomic recovered parent
// settlement. Security and parent identity are revalidated under the same
// durable thread lock as the patch, append, write, and exact readback.
func (s *DurableEventSessionStore) EnsureRecoveredParentToolSettlementExact(
	input subagentstartupport.RecoveredParentToolSettlementInput,
) (subagentstartupport.RecoveredParentToolSettlementResult, error) {
	if err := domainevent.ValidatePublicRecord(input.ResultItem); err != nil {
		return "", err
	}
	closedResult, ok := domaintoolresult.PrivateDurableToolResultItemRecordV1(input.ResultItem)
	if !ok || !reflect.DeepEqual(contracts.CloneMap(input.ResultItem), contracts.CloneMap(closedResult)) {
		return subagentstartupport.RecoveredParentToolSettlementConflict, nil
	}
	if stringField(input.ResultItem, "id") != domaintoolresult.ToolResultItemIDV1(
		strings.TrimSpace(input.Record.ParentTurnID), strings.TrimSpace(input.CallID),
	) {
		return subagentstartupport.RecoveredParentToolSettlementConflict, nil
	}
	timestamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.Timestamp))
	if err != nil {
		return "", errors.New("recovered parent tool settlement timestamp is invalid")
	}
	threadID := strings.TrimSpace(input.Record.ParentThreadID)
	turnID := strings.TrimSpace(input.Record.ParentTurnID)
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil || thread == nil {
		if err == nil {
			err = os.ErrNotExist
		}
		return "", err
	}
	if blocker := subagentapp.RecoveredParentSettlementBlockerAt(input.Record, thread, timestamp); blocker != "" {
		return subagentstartupport.RecoveredParentToolSettlementConflict, nil
	}
	itemID, callID, toolName, ok := subagentapp.JobToolIdentity(thread, input.Record)
	if !ok || itemID != strings.TrimSpace(input.ToolCallItemID) || callID != strings.TrimSpace(input.CallID) ||
		toolName != strings.TrimSpace(input.ToolName) {
		return subagentstartupport.RecoveredParentToolSettlementConflict, nil
	}
	mutation := turnapp.RecoveredToolSettlementInput{
		Thread: thread, ThreadID: threadID, TurnID: turnID,
		ToolCallItemID: itemID, CallID: callID, ToolName: toolName,
		Status: strings.TrimSpace(input.Status), Timestamp: strings.TrimSpace(input.Timestamp), ResultItem: input.ResultItem,
	}
	next, changed, err := turnapp.EnsureRecoveredToolSettlementExact(mutation)
	if errors.Is(err, turnapp.ErrTurnItemIdentityConflict) {
		return subagentstartupport.RecoveredParentToolSettlementConflict, nil
	}
	if err != nil {
		return "", err
	}
	if !changed {
		return subagentstartupport.RecoveredParentToolSettlementExistingExact, nil
	}
	writeErr := s.upsertThreadNoLock(next, false)
	persisted, readErr := s.readThreadNoLock(threadID)
	if readErr != nil || persisted == nil {
		if readErr == nil {
			readErr = os.ErrNotExist
		}
		return "", errors.Join(writeErr, readErr)
	}
	mutation.Thread = persisted
	_, stillChanged, exactErr := turnapp.EnsureRecoveredToolSettlementExact(mutation)
	if exactErr == nil && !stillChanged {
		return subagentstartupport.RecoveredParentToolSettlementInserted, nil
	}
	if errors.Is(exactErr, turnapp.ErrTurnItemIdentityConflict) {
		return subagentstartupport.RecoveredParentToolSettlementConflict, nil
	}
	if writeErr != nil || exactErr != nil {
		return "", errors.Join(writeErr, exactErr)
	}
	return "", errors.New("durable recovered parent tool settlement readback is missing")
}

func (s *DurableEventSessionStore) ensureItemToTurnExact(threadID, turnID string, item map[string]any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil || thread == nil {
		if err == nil {
			err = os.ErrNotExist
		}
		return false, err
	}
	next, changed, err := turnapp.EnsureItemToTurnExact(turnapp.AppendItemInput{
		Thread: thread, TurnID: turnID, Item: item, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil || !changed {
		return changed, err
	}
	if err := turnapp.ValidateSecurityBoundAppend(thread, turnID, item); err != nil {
		return false, err
	}
	writeErr := s.upsertThreadNoLock(next, false)
	persisted, readErr := s.readThreadNoLock(threadID)
	if readErr != nil || persisted == nil {
		if readErr == nil {
			readErr = os.ErrNotExist
		}
		return false, errors.Join(writeErr, readErr)
	}
	_, stillChanged, exactErr := turnapp.EnsureItemToTurnExact(turnapp.AppendItemInput{
		Thread: persisted, TurnID: turnID, Item: item,
	})
	if exactErr == nil && !stillChanged {
		return true, nil
	}
	if exactErr != nil {
		return false, errors.Join(writeErr, exactErr)
	}
	if writeErr != nil {
		return false, writeErr
	}
	return false, errors.New("durable exact turn item readback is missing")
}

// EnsureGateRequestItemExact is the durable projection boundary for a signed
// continuation request. It recognizes an exact prior commit and rejects an
// item-id collision instead of blindly appending a duplicate.
func (s *DurableEventSessionStore) EnsureGateRequestItemExact(threadID, turnID string, item map[string]any) error {
	if err := domainevent.ValidatePublicRecord(item); err != nil {
		return err
	}
	kind := stringField(item, "kind")
	if kind != "approval" && kind != "user_input" {
		return errors.New("gate request item kind is invalid")
	}
	_, err := s.ensureItemToTurnExact(threadID, turnID, item)
	return err
}

// EnsureApprovalTransitionItemExact makes the approved-grant registry change
// an exact, append-once projection. A retry after an ambiguous write observes
// the identical item; an item-id collision with different authority fails.
func (s *DurableEventSessionStore) EnsureApprovalTransitionItemExact(threadID, turnID string, item map[string]any) error {
	if err := domainevent.ValidatePublicRecord(item); err != nil {
		return err
	}
	if stringField(item, "kind") != executiongrantapp.GrantTransitionItemKind {
		return errors.New("approval transition item kind is invalid")
	}
	_, err := s.ensureItemToTurnExact(threadID, turnID, item)
	return err
}

// EnsurePrivateToolResultItemExact persists only the closed host-private
// tool-result shape. Exact replay is checked before registry settlement so an
// acknowledged-lost first write cannot be rejected merely because the grant
// is already durably settled.
func (s *DurableEventSessionStore) EnsurePrivateToolResultItemExact(threadID, turnID string, item map[string]any) error {
	if err := domainevent.ValidatePublicRecord(item); err != nil {
		return err
	}
	if stringField(item, "kind") != "tool_result" {
		return errors.New("private tool result item kind is invalid")
	}
	projected, ok := domaintoolresult.PrivateDurableToolResultItemRecordV1(item)
	if !ok || !reflect.DeepEqual(contracts.CloneMap(item), contracts.CloneMap(projected)) {
		return errors.New("private tool result item is not canonically closed")
	}
	_, err := s.ensureItemToTurnExact(threadID, turnID, projected)
	return err
}

func (s *DurableEventSessionStore) PatchTurnItemStatus(threadID, turnID, itemID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return os.ErrNotExist
	}
	if err := turnapp.ValidatePatchTurnItemStatus(thread, turnID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	thread, ok := turnapp.PatchTurnItemStatus(turnapp.PatchItemStatusInput{
		Thread:     thread,
		TurnID:     turnID,
		ItemID:     itemID,
		Status:     status,
		FinishedAt: now,
		UpdatedAt:  now,
	})
	if !ok {
		return os.ErrNotExist
	}
	return s.upsertThreadNoLock(thread, false)
}

func (s *DurableEventSessionStore) FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status string, appendItems []map[string]any, fields map[string]any) (bool, string, error) {
	return s.commitTerminalUpdateWithAuthority(
		threadID, turnID, status, appendItems, fields,
		domainevidence.PrivateAcceptedFinalRecord{}, nil,
	)
}

func (s *DurableEventSessionStore) FinishTurnIfActiveWithAcceptedFinalAuthority(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority turnapp.FactFinalMutationAuthority,
) (bool, string, error) {
	return s.commitTerminalUpdateWithAuthority(
		threadID, turnID, status, appendItems, fields, privateFinal, factAuthority,
	)
}

func (s *DurableEventSessionStore) TurnStatus(threadID, turnID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return "", err
	}
	if thread == nil {
		return "", os.ErrNotExist
	}
	turns, _ := thread["turns"].([]any)
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		return stringField(turn, "status"), nil
	}
	return "", os.ErrNotExist
}

func (s *DurableEventSessionStore) commitTerminalUpdateWithAuthority(
	threadID, turnID, status string,
	appendItems []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority turnapp.FactFinalMutationAuthority,
) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return false, "", err
	}
	if thread == nil {
		return false, "", os.ErrNotExist
	}
	prepared, err := turnapp.PrepareTerminalCASMutation(turnapp.PrepareTerminalCASInput{
		Thread: thread, ThreadID: threadID, TurnID: turnID, Status: status,
		AppendItems: appendItems, Fields: fields, PrivateFinal: privateFinal, FactAuthority: factAuthority,
		CaseThread: s.caseThreads != nil && s.caseThreads.IsCaseThread(threadID), Now: time.Now().UTC(),
		LoadGeneralTerminalEvents: func() ([]map[string]any, error) {
			loaded, err := s.loadEventsSinceFileNoLock(threadID, 0)
			if err != nil || len(loaded.Diagnostics) != 0 {
				return nil, errors.Join(err, errors.New("general terminal CAS cannot validate the existing event inventory"))
			}
			return loaded.Events, nil
		},
	})
	if errors.Is(err, turnapp.ErrTerminalCASTurnNotFound) {
		return false, "", os.ErrNotExist
	}
	if err != nil || !prepared.Applied {
		return false, prepared.CurrentStatus, err
	}
	if s.beforeTerminalWrite != nil {
		if err := s.beforeTerminalWrite(); err != nil {
			return false, "", err
		}
	}
	if !prepared.HasAcceptedFinal {
		return true, status, s.upsertThreadNoLock(prepared.Thread, false)
	}
	return true, status, s.upsertThreadNoLockWithAtomicWriteAuthority(prepared.Thread, false, func(write func() error) error {
		return turnapp.UseAcceptedFinalCASAuthority(
			threadID, turnID, status, appendItems, fields, privateFinal, factAuthority, write,
		)
	})
}
