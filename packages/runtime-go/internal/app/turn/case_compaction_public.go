package turn

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

// ProjectCaseCompactionPublicMarker derives the fixed public marker from the
// private, authority-bound compaction item. Continuation and operation-binding
// material stay in the private durable turn and are never copied to the marker.
func ProjectCaseCompactionPublicMarker(item map[string]any) (map[string]any, error) {
	if !validPrivateCaseCompactionMarkerSource(item) {
		return nil, errors.New("case compaction public marker source is invalid")
	}
	marker := selectCaseCompactionPublicFields(item, []string{
		"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind",
		"auto", "sourceDigest", "digestMarker",
	})
	marker["summary"] = CaseCompactionSummaryTextV1
	marker["pinnedConstraints"] = []any{"user: preserve recent turns"}
	marker["sourceItemIds"] = []any{}
	marker["schemaVersion"] = float64(3)
	marker["reasoningExcluded"] = true
	marker["assistantProseExcluded"] = true
	marker["toolPayloadsExcluded"] = true
	marker["caseFactsExcluded"] = true
	marker["caseHistoryProjectionVersion"] = float64(2)
	marker["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(marker)
	if !ValidateCaseCompactionPublicMarker(marker) {
		return nil, errors.New("case compaction public marker is invalid")
	}
	return marker, nil
}

func ValidateCaseCompactionPublicMarker(item map[string]any) bool {
	sourceDigest := strings.TrimSpace(stringField(item, "sourceDigest"))
	pinnedConstraints, pinnedConstraintsOK := item["pinnedConstraints"].([]any)
	sourceItemIDs, sourceItemIDsOK := item["sourceItemIds"].([]any)
	_, autoOK := item["auto"].(bool)
	if item == nil || stringField(item, "kind") != "compaction" ||
		stringField(item, "role") != "system" || stringField(item, "status") != "completed" ||
		stringField(item, "summary") != CaseCompactionSummaryTextV1 ||
		stringField(item, "id") == "" || stringField(item, "threadId") == "" ||
		stringField(item, "turnId") == "" || stringField(item, "createdAt") == "" ||
		stringField(item, "finishedAt") != stringField(item, "createdAt") ||
		caseCompactionNumeric(item["schemaVersion"]) != 3 ||
		caseCompactionNumeric(item["caseHistoryProjectionVersion"]) != 2 ||
		item["reasoningExcluded"] != true || item["assistantProseExcluded"] != true ||
		item["toolPayloadsExcluded"] != true || item["caseFactsExcluded"] != true ||
		!autoOK || !pinnedConstraintsOK ||
		len(pinnedConstraints) != 1 || pinnedConstraints[0] != "user: preserve recent turns" ||
		!sourceItemIDsOK || len(sourceItemIDs) != 0 || !domainsecurity.IsSHA256Hex(sourceDigest) ||
		stringField(item, "digestMarker") != "sha256:"+sourceDigest[:12] ||
		stringField(item, "reasoningExclusionProof") != domainevent.ReasoningExclusionProof(item) ||
		domainevent.ValidatePublicRecord(item) != nil {
		return false
	}
	allowed := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true,
		"createdAt": true, "finishedAt": true, "kind": true, "summary": true,
		"auto": true, "pinnedConstraints": true, "sourceDigest": true,
		"digestMarker": true, "sourceItemIds": true, "schemaVersion": true,
		"reasoningExcluded": true, "reasoningExclusionProof": true,
		"assistantProseExcluded": true, "toolPayloadsExcluded": true,
		"caseFactsExcluded": true, "caseHistoryProjectionVersion": true,
	}
	for key := range item {
		if !allowed[key] {
			return false
		}
	}
	return true
}

// ValidateCaseCompactionAuthorityTurnV1 validates the private, authority-bound
// compaction target before final-authority preflight classifies it as a
// metadata-only terminal. The target is not an accepted-final record: every
// identity, context, epoch snapshot, continuation digest, and operation
// binding must agree with the durable thread and its exact prior authority
// inventory before that narrow classification is allowed.
func ValidateCaseCompactionAuthorityTurnV1(
	threadID string,
	thread map[string]any,
	turn map[string]any,
	targetState domaincontextepoch.State,
	activeInherited ...[]domainsecurity.ActiveInheritedTurnV1,
) error {
	if len(activeInherited) > 1 {
		return errors.New("case compaction inherited inventory is ambiguous")
	}
	var inherited []domainsecurity.ActiveInheritedTurnV1
	if len(activeInherited) == 1 {
		inherited = activeInherited[0]
	}
	threadID = strings.TrimSpace(threadID)
	turnID := strings.TrimSpace(stringField(turn, "id"))
	if thread == nil || turn == nil || threadID == "" || turnID == "" ||
		strings.TrimSpace(stringField(thread, "id")) != threadID ||
		strings.TrimSpace(stringField(turn, "threadId")) != threadID ||
		strings.TrimSpace(stringField(turn, "caseHistoryProjection")) != "compaction_authority_v1" ||
		strings.TrimSpace(stringField(turn, "status")) != "completed" || turn["acceptedFinal"] != nil {
		return errors.New("case compaction authority turn identity or projection is invalid")
	}
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(frozen) != nil ||
		!domainsecurity.TurnSecurityContextIsCaseSensitive(frozen) || frozen.ThreadID != threadID || frozen.TurnID != turnID {
		return errors.New("case compaction authority turn security context is invalid")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || current.ThreadID != threadID || current.ContextEpoch < frozen.ContextEpoch {
		return errors.New("case compaction authority turn is detached from current security authority")
	}
	snapshot, err := parseCaseCompactionEpochSnapshotV1(turn["contextEpochSnapshot"])
	if err != nil || snapshot.ThreadID != threadID || snapshot.Epoch != frozen.ContextEpoch ||
		!domainsecurity.IsSHA256Hex(snapshot.RecoveryDigest) {
		return errors.New("case compaction authority epoch snapshot is invalid")
	}
	currentState, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	if err != nil || currentState.ThreadID != threadID || currentState.AcceptedSnapshot.Epoch != current.ContextEpoch {
		return errors.New("case compaction authority epoch state is detached")
	}
	if domaincontextepoch.ValidateState(targetState) != nil || targetState.ThreadID != threadID ||
		!reflect.DeepEqual(targetState.AcceptedSnapshot, snapshot) {
		return errors.New("case compaction authority epoch state is detached")
	}
	auto, err := contextepochapp.CompactionAutoMode(targetState)
	if err != nil {
		return errors.New("case compaction authority operation mode is invalid")
	}
	items, ok := turn["items"].([]any)
	if !ok || len(items) != 1 {
		return errors.New("case compaction authority turn item manifest is invalid")
	}
	item, ok := items[0].(map[string]any)
	if !ok || !validPrivateCaseCompactionMarkerSource(item) {
		return errors.New("case compaction authority marker source is invalid")
	}
	continuation, continuationErr := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
	binding, bindingErr := ParseCaseCompactionOperationBindingV1(item["caseCompactionBinding"])
	digest, digestErr := CaseCompactionOperationDigestV1(binding)
	issuedAt, issuedAtErr := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
	if continuationErr != nil || bindingErr != nil || digestErr != nil || issuedAtErr != nil {
		return errors.Join(errors.New("case compaction authority marker binding is invalid"), continuationErr, bindingErr, digestErr)
	}
	itemAuto, itemAutoOK := item["auto"].(bool)
	safeThreadID := contracts.SafeRecordID(threadID)
	expectedTurnID := fmt.Sprintf("turn_%s_compaction_%s", safeThreadID, binding.OperationStamp)
	expectedItemID := fmt.Sprintf("compaction_%s_%s", safeThreadID, binding.OperationStamp)
	expectedStamp := strconv.FormatInt(issuedAt.UnixNano(), 10)
	if !itemAutoOK || itemAuto != auto || binding.OperationStamp != expectedStamp ||
		binding.ThreadIDHash != domainsecurity.SHA256Hex([]byte(threadID)) ||
		binding.SourceContextDigest != strings.TrimSpace(stringField(item, "sourceContextDigest")) ||
		binding.ContinuationDigest != continuation.StateDigest || digest != strings.TrimSpace(stringField(item, "sourceDigest")) ||
		digest != snapshot.RecoveryDigest || stringField(item, "threadId") != threadID ||
		stringField(item, "turnId") != turnID || stringField(item, "id") != expectedItemID ||
		turnID != expectedTurnID || stringField(item, "createdAt") != frozen.IssuedAt ||
		stringField(item, "finishedAt") != frozen.IssuedAt {
		return errors.New("case compaction authority marker binding is detached")
	}
	if err := validateCaseCompactionAuthorityInventoryV1(
		threadID, turnID, frozen.ContextEpoch, issuedAt,
		binding.AuthorityTurnIDs, binding.SourceContextDigest, thread, inherited,
	); err != nil {
		return err
	}
	return nil
}

func validateCaseCompactionAuthorityInventoryV1(
	threadID,
	targetTurnID string,
	targetEpoch uint64,
	targetIssuedAt time.Time,
	authorityTurnIDs []string,
	sourceContextDigest string,
	thread map[string]any,
	inherited []domainsecurity.ActiveInheritedTurnV1,
) error {
	wanted := make(map[string]bool, len(authorityTurnIDs))
	for _, turnID := range authorityTurnIDs {
		turnID = strings.TrimSpace(turnID)
		if turnID == "" || turnID == targetTurnID || wanted[turnID] {
			return errors.New("case compaction authority inventory is invalid")
		}
		wanted[turnID] = true
	}
	if len(wanted) == 0 {
		return errors.New("case compaction authority inventory is unavailable")
	}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return errors.New("case compaction authority inventory is unavailable")
	}
	if inherited != nil && !validActiveInheritedCompactionPrefixV1(turns, inherited, append(append([]string(nil), authorityTurnIDs...), targetTurnID)) {
		return errors.New("case compaction inherited prefix is invalid")
	}
	generalAuthorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if err != nil {
		return errors.Join(errors.New("case compaction ordinary terminal authority inventory is invalid"), err)
	}
	seen := make(map[string]bool, len(wanted))
	seenTurnIDs := make(map[string]bool, len(turns))
	targetCount := 0
	sourceMatches := 0
	for index, raw := range turns {
		candidate, candidateOK := raw.(map[string]any)
		candidateID := strings.TrimSpace(stringField(candidate, "id"))
		if !candidateOK || candidateID == "" || seenTurnIDs[candidateID] {
			return errors.New("case compaction authority inventory turn is invalid")
		}
		seenTurnIDs[candidateID] = true
		if index < len(inherited) {
			continue
		}
		if candidateID == targetTurnID {
			targetCount++
			continue
		}
		context, contextErr := domainsecurity.ParseTurnSecurityContext(candidate["securityContext"])
		contextIssuedAt, issuedAtErr := time.Parse(time.RFC3339Nano, context.IssuedAt)
		if contextErr != nil || issuedAtErr != nil || context.ThreadID != threadID || context.TurnID != candidateID {
			if wanted[candidateID] {
				return errors.New("case compaction authority inventory context is invalid")
			}
			return errors.New("case compaction authority inventory contains an unexpected turn")
		}
		if context.ContextEpoch > targetEpoch ||
			(context.ContextEpoch == targetEpoch && contextIssuedAt.After(targetIssuedAt)) {
			// Later turns are outside this compaction's frozen source inventory.
			// Final-authority reconciliation validates each of them through its
			// own accepted-final, general-terminal, or compaction authority.
			continue
		}
		if !wanted[candidateID] {
			authority, found := generalAuthorities[candidateID]
			if !found || !authority.Governed || !authority.Terminal ||
				!domainsecurity.TurnSecurityContextIsGeneral(context) || context.ContextEpoch >= targetEpoch {
				return errors.New("case compaction authority inventory contains an unexpected turn")
			}
			continue
		}
		if !domainsecurity.TurnSecurityContextIsCaseSensitive(context) {
			continue
		}
		if context.ContextEpoch >= targetEpoch {
			if wanted[candidateID] {
				return errors.New("case compaction authority inventory contains a non-prior context")
			}
			continue
		}
		if seen[candidateID] {
			return errors.New("case compaction authority inventory is incomplete")
		}
		seen[candidateID] = true
		if context.ContextDigest == sourceContextDigest {
			sourceMatches++
		}
	}
	for turnID := range wanted {
		if !seen[turnID] {
			return errors.New("case compaction authority inventory is incomplete")
		}
	}
	if targetCount != 1 || sourceMatches != 1 {
		return errors.New("case compaction authority source context is unavailable")
	}
	return nil
}

func parseCaseCompactionEpochSnapshotV1(value any) (domaincontextepoch.Snapshot, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return domaincontextepoch.Snapshot{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var snapshot domaincontextepoch.Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return domaincontextepoch.Snapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domaincontextepoch.Snapshot{}, errors.New("case compaction epoch snapshot has trailing content")
	}
	if err := domaincontextepoch.ValidateSnapshot(snapshot); err != nil {
		return domaincontextepoch.Snapshot{}, err
	}
	return snapshot, nil
}

func validPrivateCaseCompactionMarkerSource(item map[string]any) bool {
	sourceDigest := strings.TrimSpace(stringField(item, "sourceDigest"))
	replacedTokens, replacedTokensOK := positiveCaseCompactionInteger(item["replacedTokens"])
	pinnedConstraints, pinnedConstraintsOK := item["pinnedConstraints"].([]any)
	sourceItemIDs, sourceItemIDsOK := item["sourceItemIds"].([]any)
	_, autoOK := item["auto"].(bool)
	if item == nil || stringField(item, "kind") != "compaction" ||
		stringField(item, "role") != "system" || stringField(item, "status") != "completed" ||
		stringField(item, "summary") != CaseCompactionSummaryTextV1 ||
		stringField(item, "id") == "" || stringField(item, "threadId") == "" ||
		stringField(item, "turnId") == "" || stringField(item, "createdAt") == "" ||
		stringField(item, "finishedAt") != stringField(item, "createdAt") ||
		caseCompactionNumeric(item["schemaVersion"]) != 3 ||
		caseCompactionNumeric(item["caseHistoryProjectionVersion"]) != 2 ||
		item["reasoningExcluded"] != true || item["caseFactsExcluded"] != true ||
		!replacedTokensOK || replacedTokens <= 0 || !autoOK || !pinnedConstraintsOK ||
		len(pinnedConstraints) != 1 || pinnedConstraints[0] != "user: preserve recent turns" ||
		!sourceItemIDsOK || len(sourceItemIDs) != 0 || !domainsecurity.IsSHA256Hex(sourceDigest) ||
		stringField(item, "digestMarker") != "sha256:"+sourceDigest[:12] ||
		!domainsecurity.IsSHA256Hex(stringField(item, "sourceContextDigest")) ||
		item["taskContinuation"] == nil || item["caseCompactionBinding"] == nil ||
		stringField(item, "reasoningExclusionProof") != domainevent.ReasoningExclusionProof(item) {
		return false
	}
	allowed := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true,
		"createdAt": true, "finishedAt": true, "kind": true, "summary": true,
		"replacedTokens": true, "auto": true, "pinnedConstraints": true, "sourceDigest": true,
		"digestMarker": true, "sourceItemIds": true, "schemaVersion": true,
		"reasoningExcluded": true, "reasoningExclusionProof": true, "caseFactsExcluded": true,
		"caseHistoryProjectionVersion": true, "taskContinuation": true,
		"sourceContextDigest": true, "caseCompactionBinding": true,
	}
	for key := range item {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func selectCaseCompactionPublicFields(record map[string]any, fields []string) map[string]any {
	selected := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := record[field]; ok {
			selected[field] = contracts.CloneValue(value)
		}
	}
	return selected
}

func positiveCaseCompactionInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), typed > 0
	case int64:
		return typed, typed > 0
	case float64:
		if typed <= 0 || typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		return parsed, err == nil && parsed > 0
	default:
		return 0, false
	}
}

func caseCompactionNumeric(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		if typed == float64(int(typed)) {
			return int(typed)
		}
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err == nil && int64(int(parsed)) == parsed {
			return int(parsed)
		}
	}
	return -1
}
