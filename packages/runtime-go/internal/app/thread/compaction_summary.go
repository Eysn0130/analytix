package thread

import (
	"context"
	"errors"
	"strings"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type OrdinarySummaryInput struct {
	Thread         map[string]any
	BaselineDigest string
	TurnID         string
	CompactionID   string
	Messages       []domainmodel.Message
	Reservation    *controlapp.ForegroundReservation
	Automatic      bool
}

type SummarizeCompactionFunc func(context.Context, OrdinarySummaryInput) (string, error)
type ProjectCompactionHistoryFunc func(context.Context, map[string]any) ([]domainmodel.Message, error)

func (s *Service) ordinarySummaryAuthority(thread map[string]any, threadID string) error {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil || caseSensitive || caseThreadKnown(s.caseThreads, threadID) {
		return errors.Join(ErrCaseCompactionRequiresTrustedArchive, err)
	}
	current, contextErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if contextErr != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(current) || domainsecurity.ValidateTurnSecurityContextForExecution(current) != nil ||
		current.ThreadID != threadID {
		return errors.New("ordinary compaction authority is unavailable")
	}
	return s.validateCompactionWorkspaceAuthority(thread)
}

// The model runs before the existing context transition and CAS. Only a
// complete, projected, smaller candidate can enter that transaction; commit
// revalidation reuses sealed facts and never invokes a Provider.
func (s *Service) compactOrdinarySummary(ctx context.Context, thread map[string]any, threadID, reason string, auto bool, reservation *controlapp.ForegroundReservation) (response map[string]any, err error) {
	at := time.Now().UTC()
	selection, err := prepareCompaction(thread, threadID, reason, at, auto, nil)
	if err != nil {
		return nil, err
	}
	if selection.Result.ReplacedTokens <= 0 {
		return map[string]any{"ok": true, "threadId": threadID, "replacedTokens": float64(0), "pinnedConstraints": stringListAny(selection.Result.PinnedConstraints)}, nil
	}
	// An unchanged history has no summary effect and needs no new authority.
	// In particular, the first turn must reach the existing hard-limit guard.
	if err := s.ordinarySummaryAuthority(thread, threadID); err != nil {
		return nil, err
	}
	if s.summarizeCompaction == nil || s.projectCompactionHistory == nil || s.beginTransition == nil {
		return nil, errors.New("ordinary compaction summary producer is unavailable")
	}
	if auto {
		if err := reservation.Validate(ctx, threadID); err != nil {
			return nil, err
		}
	}
	turns, _ := thread["turns"].([]any)
	prefix := contracts.CloneMap(thread)
	prefix["turns"] = append([]any(nil), turns[:selection.Result.CompactedTurns]...)
	messages, err := s.projectCompactionHistory(ctx, prefix)
	if err != nil || len(messages) == 0 {
		return nil, errors.Join(errors.New("ordinary compaction selected history is unavailable"), err)
	}
	selectedTokens := appmodel.EstimateMessagesTokensV1(messages)
	count := 0
	for _, raw := range turns[:selection.Result.CompactedTurns] {
		turn, ok := raw.(map[string]any)
		items, itemsOK := turn["items"].([]any)
		if !ok || !itemsOK {
			return nil, errors.New("ordinary compaction inventory is invalid")
		}
		count += len(items)
	}
	started := map[string]any{"kind": "compaction_started", "threadId": threadID, "turnId": selection.Result.TurnID,
		"itemId": selection.Result.ItemID, "compactionId": selection.Result.ItemID, "schemaVersion": float64(3),
		"auto": auto, "reason": reason, "startedAt": at.Format(time.RFC3339Nano)}
	if _, _, err := s.repository.RecordEvent(started); err != nil {
		return nil, err
	}
	committedHistory := false
	defer func() {
		if err != nil && !committedHistory {
			_, _, recordErr := s.repository.RecordEvent(map[string]any{"kind": "compaction_failed", "threadId": threadID,
				"turnId": selection.Result.TurnID, "itemId": selection.Result.ItemID, "compactionId": selection.Result.ItemID,
				"schemaVersion": float64(3), "auto": auto, "reasonCode": "summary_unavailable", "finishedAt": time.Now().UTC().Format(time.RFC3339Nano)})
			err = errors.Join(err, recordErr)
		}
	}()
	text, err := s.summarizeCompaction(ctx, OrdinarySummaryInput{Thread: contracts.CloneMap(thread), BaselineDigest: selection.BaselineDigest,
		TurnID: selection.Result.TurnID, CompactionID: selection.Result.ItemID, Messages: messages, Reservation: reservation, Automatic: auto})
	if err != nil {
		return nil, err
	}
	text, err = domainevent.FilterPublicText(text)
	if err != nil {
		return nil, err
	}
	text = privacyprojectionapp.ProjectOrdinaryText(text)
	if strings.TrimSpace(text) == "" || text == domainsecret.RedactedV1 || domainevent.ValidatePublicRecord(map[string]any{"text": text}) != nil {
		return nil, errors.New("ordinary compaction summary is empty or unavailable")
	}
	facts, err := domainthread.SealCompactionSummaryFactsV1(domainthread.CompactionSummaryFactsV1{
		CompactionID: selection.Result.ItemID, Summary: text, SourceDigest: selection.Result.SourceDigest,
		SourceItemIDs: append([]string(nil), selection.Result.SourceItemIDs...), ShadowedItemCount: count,
		ReplacedTokens: selectedTokens, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}
	prepared, err := prepareCompaction(thread, threadID, reason, at, auto, nil, &facts)
	if err != nil {
		return nil, err
	}
	before, err := s.projectCompactionHistory(ctx, thread)
	if err != nil {
		return nil, err
	}
	after, err := s.projectCompactionHistory(ctx, prepared.Thread)
	if err != nil {
		return nil, err
	}
	if appmodel.EstimateMessagesTokensV1(after) >= appmodel.EstimateMessagesTokensV1(before) {
		return nil, errors.New("ordinary compaction summary does not shrink the projected history")
	}
	transition, err := s.beginTransition(ctx, prepared.SecurityContext)
	if err != nil || transition == nil {
		return nil, errors.Join(errors.New("compaction transition is unavailable"), err)
	}
	defer transition.Abort()
	current, err := s.repository.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrThreadNotFound
	}
	if err := s.ordinarySummaryAuthority(current, threadID); err != nil {
		return nil, err
	}
	digest, err := CompactionBaselineDigest(current)
	if err != nil || digest != selection.BaselineDigest {
		return nil, ErrCompactionBaselineConflict
	}
	timeout := s.transitionTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if err := transition.Prepare(ctx, prepared.SecurityContext, timeout); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if auto {
		if err := reservation.Validate(ctx, threadID); err != nil {
			return nil, err
		}
	}
	// Registry classification is checked again immediately before the existing
	// repository CAS, independently of the frozen general context.
	if err := s.ordinarySummaryAuthority(current, threadID); err != nil {
		return nil, err
	}
	committed, commitErr := s.repository.CommitCompaction(prepared.CommitRequest())
	if committed.Committed {
		committedHistory = true
		if committed.SecurityContext != prepared.SecurityContext || committed.EpochState.StateDigest != prepared.EpochState.StateDigest {
			return nil, errors.New("ordinary compaction readback diverged")
		}
		if err := transition.Commit(); err != nil {
			return nil, errors.Join(commitErr, err)
		}
	}
	if commitErr != nil {
		return nil, commitErr
	}
	if !committed.Committed {
		return nil, errors.New("ordinary compaction commit is unavailable")
	}
	completed, err := OrdinaryCompactionCompletedEventV3(prepared.Thread, prepared.Result.TurnID)
	if err != nil {
		return nil, err
	}
	completed["reason"] = reason
	recorded, _, err := s.repository.RecordEvent(completed)
	if err != nil {
		return nil, err
	}
	response = contracts.CloneMap(completed)
	delete(response, "kind")
	delete(response, "reason")
	delete(response, "seq")
	delete(response, "timestamp")
	response["ok"], response["eventSeq"] = true, recorded["seq"]
	return response, nil
}

// Reused by startup recovery. Its only input is the existing committed marker;
// an interrupted operation never acquires credentials or reissues a Provider.
func OrdinaryCompactionCompletedEventV3(thread map[string]any, turnID string) (map[string]any, error) {
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		if len(items) != 1 {
			break
		}
		item, _ := items[0].(map[string]any)
		if !domainevent.ValidGeneralCompactionProviderHistoryItemV5(item) {
			break
		}
		// This is historical checkpoint evidence, never a live execution grant.
		// A copied marker or an uncommitted candidate cannot prove persistence;
		// the durable owner supplies its actual primary thread to this function.
		if !appmodel.GeneralCompactionCheckpointBoundV5(thread, item) {
			break
		}
		public, err := ProjectPublicThread(map[string]any{"id": stringField(thread, "id"), "status": "idle", "turns": []any{turn}})
		if err != nil {
			return nil, err
		}
		event := map[string]any{"kind": "compaction_completed", "schemaVersion": float64(3), "turn": contracts.CloneValue(listAny(public["turns"])[0])}
		for _, key := range []string{"threadId", "turnId", "auto", "compactionId", "summaryKind", "summary", "replacedTokens", "pinnedConstraints", "sourceDigest", "digestMarker", "sourceItemIds", "summaryDigest", "checkpointDigest", "shadowedItemCount", "tokenCountKind", "tokenEstimatorVersion", "reasoningExcluded", "reasoningExclusionProof", "createdAt", "finishedAt"} {
			event[key] = contracts.CloneValue(item[key])
		}
		event["itemId"] = item["id"]
		return event, nil
	}
	return nil, errors.New("committed ordinary compaction summary is unavailable")
}
