package thread

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

// RecoverCommittedCaseCompactions completes the prune half of a case
// compaction whose installation-signed target was durably committed before a
// crash. It runs during exclusive startup, before generic committed-context
// repair and HTTP admission. Prepared-only records are deliberately ignored:
// their source history remains authoritative and intact.
func RecoverCommittedCaseCompactions(
	ctx context.Context,
	authority casethreadapp.CommittedAuthority,
	store casethreadapp.CommittedContextRepairStore,
) error {
	if ctx == nil || authority == nil || store == nil {
		return errors.New("case compaction recovery dependencies are unavailable")
	}
	contexts := authority.CommittedContexts()
	targets := committedCaseCompactionRecoveryTargets(contexts)
	quarantine := make(map[string]string)
	for _, committed := range contexts {
		threadID := strings.TrimSpace(committed.SecurityContext.ThreadID)
		if authority.RestartPreservesThreadV1(threadID) {
			continue
		}
		if threadID != "" && authority.IsCaseThread(threadID) && !authority.CanExecute(threadID) {
			quarantine[threadID] = "case thread is quarantined by host authority"
		}
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		threadID := strings.TrimSpace(target.SecurityContext.ThreadID)
		if authority.RestartPreservesThreadV1(threadID) {
			continue
		}
		if quarantine[threadID] != "" {
			continue
		}
		source, err := store.GetThreadForAuthorityRepair(threadID)
		if err != nil {
			quarantine[threadID] = "case compaction recovery is unavailable"
			continue
		}
		inherited, err := ActiveInheritedCompactionRecordV1(ctx, source, authority, store)
		if err != nil {
			quarantine[threadID] = "case compaction recovery is unavailable"
			continue
		}
		if err := authority.WithRestartRecoveryV1(threadID, func() error {
			return recoverCommittedCaseCompaction(ctx, authority, store, target, inherited)
		}); err != nil && !errors.Is(err, casethreadapp.ErrRestartPreserved) {
			quarantine[threadID] = "case compaction recovery is unavailable"
		}
	}
	authority.ReplaceQuarantine(quarantine)
	return nil
}

func committedCaseCompactionRecoveryTargets(
	contexts []casethreadapp.CommittedContext,
) []casethreadapp.CommittedContext {
	targets := make([]casethreadapp.CommittedContext, 0)
	for _, committed := range contexts {
		if _, ok := committedCaseCompactionStamp(committed); ok {
			targets = append(targets, committed)
		}
	}
	sort.Slice(targets, func(left, right int) bool {
		leftContext := targets[left].SecurityContext
		rightContext := targets[right].SecurityContext
		if leftContext.ThreadID != rightContext.ThreadID {
			return leftContext.ThreadID < rightContext.ThreadID
		}
		if leftContext.ContextEpoch != rightContext.ContextEpoch {
			return leftContext.ContextEpoch < rightContext.ContextEpoch
		}
		return leftContext.TurnID < rightContext.TurnID
	})
	return targets
}

func committedCaseCompactionStamp(
	committed casethreadapp.CommittedContext,
) (time.Time, bool) {
	target := committed.SecurityContext
	state := committed.EpochState
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(target) != nil ||
		domaincontextepoch.ValidateState(state) != nil ||
		state.ThreadID != target.ThreadID || state.AcceptedSnapshot.Epoch != target.ContextEpoch ||
		!domainsecurity.IsSHA256Hex(state.AcceptedSnapshot.RecoveryDigest) ||
		!containsCompactionRecoveryReason(state.AcceptedSnapshot.ChangeReasons) {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, target.IssuedAt)
	if err != nil || at.UnixNano() <= 0 {
		return time.Time{}, false
	}
	expectedTurnID := fmt.Sprintf(
		"turn_%s_compaction_%d", contracts.SafeRecordID(target.ThreadID), at.UnixNano(),
	)
	return at.UTC(), target.TurnID == expectedTurnID
}

func containsCompactionRecoveryReason(reasons []domaincontextepoch.ChangeReason) bool {
	for _, reason := range reasons {
		if reason == domaincontextepoch.ReasonCompactionRecovery {
			return true
		}
	}
	return false
}

func recoverCommittedCaseCompaction(
	ctx context.Context,
	authority casethreadapp.CommittedAuthority,
	store casethreadapp.CommittedContextRepairStore,
	target casethreadapp.CommittedContext,
	inherited *domainsecurity.CaseThreadAuthorityRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	at, ok := committedCaseCompactionStamp(target)
	if !ok {
		return errors.New("committed case compaction recovery target is invalid")
	}
	threadID := target.SecurityContext.ThreadID
	thread, err := store.GetThreadForAuthorityRepair(threadID)
	if err != nil || thread == nil {
		return errors.Join(errors.New("committed case compaction source is unavailable"), err)
	}
	present, err := exactCompactionTurnPresent(thread, target.SecurityContext.TurnID)
	if err != nil {
		return err
	}
	if present {
		return validateDurableCommittedCaseCompactionTarget(authority, thread, target, at)
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || current.ThreadID != threadID || current.TurnID == target.SecurityContext.TurnID ||
		target.SecurityContext.ContextEpoch != current.ContextEpoch+1 {
		return errors.New("committed case compaction source authority is invalid")
	}
	currentState, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	currentCommitted, found := authority.CommittedContext(threadID, current.TurnID)
	if err != nil || !found || currentCommitted.SecurityContext != current ||
		!reflect.DeepEqual(currentCommitted.EpochState, currentState) {
		return errors.New("committed case compaction source is not the exact committed context")
	}
	authorityTurnIDs, err := recoverySourceAuthorityTurnIDs(
		authority, thread, threadID, target.SecurityContext.TurnID,
	)
	if err != nil {
		return err
	}
	continuation, err := turnapp.BuildTaskContinuationSnapshotV1(thread)
	if err != nil {
		return errors.Join(errors.New("committed case compaction continuation could not be rebuilt"), err)
	}
	auto, err := contextepochapp.CompactionAutoMode(target.EpochState)
	if err != nil {
		return err
	}
	prepared, err := PrepareCaseCompaction(
		thread, threadID, "startup_recovery", at, auto,
		CaseCompactionAuthorization{
			Continuation:           continuation,
			SourceContextDigest:    current.ContextDigest,
			AuthorityTurnIDs:       authorityTurnIDs,
			ActiveInheritedHistory: cloneActiveInheritedHistoryRecordV1(inherited),
		},
	)
	if err != nil {
		return errors.Join(errors.New("committed case compaction replay failed"), err)
	}
	if prepared.Result.ReplacedTokens <= 0 ||
		prepared.Result.TurnID != target.SecurityContext.TurnID ||
		prepared.Result.SourceDigest != target.EpochState.AcceptedSnapshot.RecoveryDigest ||
		prepared.SecurityContext != target.SecurityContext ||
		!reflect.DeepEqual(prepared.EpochState, target.EpochState) {
		return errors.New("committed case compaction replay does not match signed authority")
	}
	if err := validateRecoveredCaseCompactionBinding(prepared); err != nil {
		return err
	}
	targetDigest, err := CompactionBaselineDigest(prepared.Thread)
	if err != nil {
		return err
	}
	if err := store.ReplaceThreadForAuthorityRepair(threadID, prepared.Thread); err != nil {
		return err
	}
	reloaded, err := store.GetThreadForAuthorityRepair(threadID)
	if err != nil || reloaded == nil {
		return errors.Join(errors.New("committed case compaction recovery readback is unavailable"), err)
	}
	readbackDigest, err := CompactionBaselineDigest(reloaded)
	if err != nil || readbackDigest != targetDigest {
		return errors.Join(errors.New("committed case compaction recovery readback diverged"), err)
	}
	return nil
}

func exactCompactionTurnPresent(thread map[string]any, turnID string) (bool, error) {
	turns, ok := thread["turns"].([]any)
	if !ok {
		return false, errors.New("committed case compaction durable turns are invalid")
	}
	found := false
	for _, raw := range turns {
		turn, structured := raw.(map[string]any)
		if !structured {
			return false, errors.New("committed case compaction durable turn is invalid")
		}
		if strings.TrimSpace(stringField(turn, "id")) != strings.TrimSpace(turnID) {
			continue
		}
		if found {
			return false, errors.New("committed case compaction target is duplicated")
		}
		found = true
	}
	return found, nil
}

func validateDurableCommittedCaseCompactionTarget(
	authority caseCompactionCommittedContextReaderV1,
	thread map[string]any,
	target casethreadapp.CommittedContext,
	at time.Time,
) error {
	current, contextErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	state, stateErr := domaincontextepoch.ParseState(thread["contextEpochState"])
	currentCommitted, currentFound := authority.CommittedContext(current.ThreadID, current.TurnID)
	currentAt, currentAtErr := time.Parse(time.RFC3339Nano, current.IssuedAt)
	if contextErr != nil || stateErr != nil || currentAtErr != nil || !currentFound ||
		currentCommitted.SecurityContext != current || !reflect.DeepEqual(currentCommitted.EpochState, state) ||
		current.ThreadID != target.SecurityContext.ThreadID ||
		current.ContextEpoch < target.SecurityContext.ContextEpoch ||
		(current.ContextEpoch == target.SecurityContext.ContextEpoch && currentAt.Before(at)) {
		return errors.New("durable case compaction target does not match signed current authority")
	}
	turns, _ := thread["turns"].([]any)
	var targetTurn map[string]any
	durableTurns := make(map[string]map[string]any, len(turns))
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		turnID := strings.TrimSpace(stringField(turn, "id"))
		if turn == nil || turnID == "" || durableTurns[turnID] != nil {
			return errors.New("durable case compaction turn inventory is invalid")
		}
		durableTurns[turnID] = turn
		if turnID == target.SecurityContext.TurnID {
			targetTurn = turn
		}
	}
	frozen, frozenErr := domainsecurity.ParseTurnSecurityContext(targetTurn["securityContext"])
	items, _ := targetTurn["items"].([]any)
	if frozenErr != nil || frozen != target.SecurityContext ||
		strings.TrimSpace(stringField(targetTurn, "caseHistoryProjection")) != "compaction_authority_v1" ||
		len(items) != 1 {
		return errors.New("durable case compaction target turn is invalid")
	}
	item, _ := items[0].(map[string]any)
	mode, modeErr := contextepochapp.CompactionAutoMode(target.EpochState)
	itemMode, itemModeOK := item["auto"].(bool)
	binding, err := turnapp.ParseCaseCompactionOperationBindingV1(item["caseCompactionBinding"])
	if err != nil {
		return err
	}
	digest, err := turnapp.CaseCompactionOperationDigestV1(binding)
	continuation, continuationErr := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
	safeThreadID := contracts.SafeRecordID(target.SecurityContext.ThreadID)
	expectedTurnID := fmt.Sprintf("turn_%s_compaction_%s", safeThreadID, binding.OperationStamp)
	expectedItemID := fmt.Sprintf("compaction_%s_%s", safeThreadID, binding.OperationStamp)
	_, targetCommitted := authority.CommittedContext(
		target.SecurityContext.ThreadID, target.SecurityContext.TurnID,
	)
	sourceCommitted := false
	for _, turnID := range binding.AuthorityTurnIDs {
		if turnID == target.SecurityContext.TurnID {
			return errors.New("durable case compaction binding includes its target authority")
		}
		committed, found := authority.CommittedContext(target.SecurityContext.ThreadID, turnID)
		durableContext, durableErr := domainsecurity.ParseTurnSecurityContext(
			durableTurns[turnID]["securityContext"],
		)
		if !found || durableErr != nil || committed.SecurityContext != durableContext ||
			committed.SecurityContext.ThreadID != target.SecurityContext.ThreadID ||
			committed.SecurityContext.TurnID != turnID ||
			domainsecurity.ValidateTurnSecurityContextForCasePublication(committed.SecurityContext) != nil {
			return errors.New("durable case compaction prior authority is unavailable")
		}
		if committed.SecurityContext.ContextDigest == binding.SourceContextDigest {
			sourceCommitted = true
		}
	}
	if err != nil || continuationErr != nil || modeErr != nil || !itemModeOK || itemMode != mode ||
		!targetCommitted || !sourceCommitted || strings.TrimSpace(stringField(item, "kind")) != "compaction" ||
		target.SecurityContext.TurnID != expectedTurnID || strings.TrimSpace(stringField(item, "id")) != expectedItemID ||
		digest != target.EpochState.AcceptedSnapshot.RecoveryDigest ||
		strings.TrimSpace(stringField(item, "sourceDigest")) != digest ||
		strings.TrimSpace(stringField(item, "sourceContextDigest")) != binding.SourceContextDigest ||
		binding.ThreadIDHash != domainsecurity.SHA256Hex([]byte(target.SecurityContext.ThreadID)) ||
		binding.OperationStamp != strconv.FormatInt(at.UnixNano(), 10) ||
		binding.ContinuationDigest != continuation.StateDigest {
		return errors.Join(errors.New("durable case compaction binding does not match signed authority"), err, continuationErr)
	}
	return nil
}

func recoverySourceAuthorityTurnIDs(
	authority casethreadapp.CommittedAuthority,
	thread map[string]any,
	threadID,
	targetTurnID string,
) ([]string, error) {
	turns, ok := thread["turns"].([]any)
	if !ok {
		return nil, errors.New("committed case compaction source turns are invalid")
	}
	durable := make(map[string]map[string]any, len(turns))
	for _, raw := range turns {
		turn, structured := raw.(map[string]any)
		turnID := strings.TrimSpace(stringField(turn, "id"))
		if !structured || turnID == "" || durable[turnID] != nil {
			return nil, errors.New("committed case compaction source turn identity is invalid")
		}
		durable[turnID] = turn
	}
	committedTurnIDs := casethreadapp.CommittedTurnIDs(authority, threadID)
	wanted := make([]string, 0, len(committedTurnIDs))
	for _, turnID := range committedTurnIDs {
		if turnID == targetTurnID {
			continue
		}
		turn := durable[turnID]
		frozen, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		committed, found := authority.CommittedContext(threadID, turnID)
		if turn == nil || parseErr != nil || !found || frozen != committed.SecurityContext {
			return nil, errors.New("committed case compaction source authority inventory is incomplete")
		}
		wanted = append(wanted, turnID)
	}
	if len(wanted) == 0 || len(wanted)+1 != len(committedTurnIDs) {
		return nil, errors.New("committed case compaction target authority inventory is inconsistent")
	}
	return canonicalCompactionAuthorityTurnIDs(wanted), nil
}

func validateRecoveredCaseCompactionBinding(prepared PreparedCompaction) error {
	turns, _ := prepared.Thread["turns"].([]any)
	var target map[string]any
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if strings.TrimSpace(stringField(turn, "id")) == prepared.SecurityContext.TurnID {
			target = turn
			break
		}
	}
	items, _ := target["items"].([]any)
	if len(items) != 1 {
		return errors.New("recovered case compaction binding target is invalid")
	}
	item, _ := items[0].(map[string]any)
	binding, err := turnapp.ParseCaseCompactionOperationBindingV1(item["caseCompactionBinding"])
	if err != nil {
		return err
	}
	digest, err := turnapp.CaseCompactionOperationDigestV1(binding)
	if err != nil || digest != prepared.Result.SourceDigest ||
		digest != prepared.EpochState.AcceptedSnapshot.RecoveryDigest {
		return errors.Join(errors.New("recovered case compaction binding does not match signed recovery digest"), err)
	}
	return nil
}
