package turn

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

const CaseCompactionSummaryTextV1 = "Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch."

const GeneralCompactionSummaryTextV3 = domainevent.GeneralCompactionSummaryTextV3

type CompactionInput struct {
	ThreadID                   string
	Model                      string
	Turns                      []any
	SecurityContext            any
	CaseMarker                 bool
	Auto                       bool
	ContinuationThread         map[string]any
	Reason                     string
	Stamp                      int64
	Now                        string
	GeneralTerminalAuthorities map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1
	// CaseContinuation is present only after the application service has
	// revalidated the exact current committed TSCV2/DSV2 and sealed the
	// existing typed task continuation. It authorizes a metadata-only rewrite;
	// its evidence references remain unverified for case facts.
	CaseContinuation *threaddomain.TaskContinuationSnapshotV1
	// CaseAuthorityTurnIDs is the exact pre-compaction CaseThreadAuthority
	// inventory. Every signed context remains present after the rewrite so
	// restart validation never has to trust a pruned authority record.
	CaseAuthorityTurnIDs []string
	// ActiveInheritedTurns is supplied only after the caller authenticates the
	// existing lineage. Digests enforce exact replay; they grant no authority.
	ActiveInheritedTurns []domainsecurity.ActiveInheritedTurnV1
}

type CompactionPlan struct {
	Result    threaddomain.CompactionResult
	NextTurns []any
	Changed   bool
	Error     error
}

func BuildThreadCompaction(thread map[string]any, threadID, reason string, stamp int64, now string) CompactionPlan {
	return BuildThreadCompactionWithMode(thread, threadID, reason, stamp, now, false)
}

func BuildThreadCompactionWithMode(thread map[string]any, threadID, reason string, stamp int64, now string, auto bool) CompactionPlan {
	return buildThreadCompactionWithModeAndCaseAuthority(thread, threadID, reason, stamp, now, auto, nil, nil, nil)
}

func BuildThreadCompactionWithCaseAuthority(
	thread map[string]any,
	threadID, reason string,
	stamp int64,
	now string,
	auto bool,
	continuation threaddomain.TaskContinuationSnapshotV1,
	authorityTurnIDs []string,
	activeInherited ...[]domainsecurity.ActiveInheritedTurnV1,
) CompactionPlan {
	if len(activeInherited) > 1 {
		return CompactionPlan{Error: errors.New("case compaction inherited inventory is ambiguous")}
	}
	var inherited []domainsecurity.ActiveInheritedTurnV1
	if len(activeInherited) == 1 {
		inherited = activeInherited[0]
	}
	return buildThreadCompactionWithModeAndCaseAuthority(
		thread, threadID, reason, stamp, now, auto, &continuation, authorityTurnIDs, inherited,
	)
}

func buildThreadCompactionWithModeAndCaseAuthority(
	thread map[string]any,
	threadID, reason string,
	stamp int64,
	now string,
	auto bool,
	continuation *threaddomain.TaskContinuationSnapshotV1,
	authorityTurnIDs []string,
	activeInherited []domainsecurity.ActiveInheritedTurnV1,
) CompactionPlan {
	turns, _ := thread["turns"].([]any)
	authorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if err != nil {
		return CompactionPlan{Result: threaddomain.CompactionResult{ThreadID: strings.TrimSpace(threadID), RemainingTurns: len(turns)}, Error: err}
	}
	return BuildCompaction(CompactionInput{
		ThreadID: threadID, Model: stringField(thread, "model"), Turns: turns, SecurityContext: thread["securityState"],
		CaseMarker: hasCompactionCaseMarker(thread), Auto: auto, ContinuationThread: thread, Reason: reason, Stamp: stamp, Now: now,
		GeneralTerminalAuthorities: authorities, CaseContinuation: continuation,
		CaseAuthorityTurnIDs: append([]string(nil), authorityTurnIDs...),
		ActiveInheritedTurns: activeInherited,
	})
}

func BuildCompaction(input CompactionInput) CompactionPlan {
	threadID := strings.TrimSpace(input.ThreadID)
	turns := append([]any(nil), input.Turns...)
	caseBound, err := compactionCaseBound(input.SecurityContext, turns, threadID, input.CaseMarker)
	if err != nil {
		return CompactionPlan{Result: threaddomain.CompactionResult{ThreadID: threadID, RemainingTurns: len(turns)}, Error: err}
	}
	authorizedCase := caseBound && validCaseCompactionContinuation(input.SecurityContext, input.CaseContinuation) &&
		validCaseCompactionAuthorityTurnIDs(turns, input.CaseAuthorityTurnIDs, threadID)
	if input.ActiveInheritedTurns != nil && (!authorizedCase ||
		!validActiveInheritedCompactionPrefixV1(turns, input.ActiveInheritedTurns, input.CaseAuthorityTurnIDs)) {
		return CompactionPlan{Error: errors.New("case compaction inherited prefix is invalid")}
	}
	if caseBound && !authorizedCase && compactionContainsAcceptedFinal(turns) {
		return CompactionPlan{
			Result: threaddomain.CompactionResult{ThreadID: threadID, RemainingTurns: len(turns)},
			Error:  errors.New("case compaction requires a trusted authority archive before accepted finals can be removed"),
		}
	}
	if compactionContainsEvidenceSettlement(turns) && !authorizedCase {
		return CompactionPlan{
			Result: threaddomain.CompactionResult{ThreadID: threadID, RemainingTurns: len(turns)},
			Error:  errors.New("compaction requires a trusted authority archive before evidence settlement markers can be removed"),
		}
	}
	if len(turns) <= 2 {
		return CompactionPlan{
			Result: threaddomain.CompactionResult{
				ThreadID:          threadID,
				PinnedConstraints: []string{"user: preserve recent turns"},
				RemainingTurns:    len(turns),
			},
		}
	}
	compactedTurns := turns[:len(turns)-2]
	tailTurns := append([]any(nil), turns[len(turns)-2:]...)
	if caseBound && !authorizedCase && caseCompactionAlreadyReduced(compactedTurns) {
		return CompactionPlan{Result: threaddomain.CompactionResult{ThreadID: threadID, PinnedConstraints: []string{"user: preserve recent turns"}, RemainingTurns: len(turns)}}
	}
	sourceItems := compactionSourceItems(compactedTurns, input.GeneralTerminalAuthorities)
	if caseBound {
		sourceItems = []map[string]any{}
	}
	if len(sourceItems) == 0 && !caseBound {
		return CompactionPlan{
			Result: threaddomain.CompactionResult{
				ThreadID:          threadID,
				PinnedConstraints: []string{"user: preserve recent turns"},
				RemainingTurns:    len(turns),
			},
		}
	}
	if len(compactedTurns) == 1 && onlyCompactionSourceItems(sourceItems) {
		return CompactionPlan{
			Result: threaddomain.CompactionResult{
				ThreadID:          threadID,
				PinnedConstraints: []string{"user: preserve recent turns"},
				RemainingTurns:    len(turns),
			},
		}
	}
	stamp := input.Stamp
	if stamp == 0 {
		stamp = time.Now().UnixNano()
	}
	now := strings.TrimSpace(input.Now)
	if now == "" {
		now = time.Unix(0, stamp).UTC().Format(time.RFC3339Nano)
	}
	safeThreadID := contracts.SafeRecordID(threadID)
	turnID := fmt.Sprintf("turn_%s_compaction_%d", safeThreadID, stamp)
	itemID := fmt.Sprintf("compaction_%s_%d", safeThreadID, stamp)
	sourceDigest := compactionSourceDigest(sourceItems)
	var caseOperationBinding *CaseCompactionOperationBindingV1
	if caseBound {
		if authorizedCase {
			binding, bindingErr := NewCaseCompactionOperationBindingV1(
				threadID, compactedTurns, turns, input.SecurityContext, stamp,
				*input.CaseContinuation, input.CaseAuthorityTurnIDs,
			)
			if bindingErr != nil {
				return CompactionPlan{
					Result: threaddomain.CompactionResult{ThreadID: threadID, RemainingTurns: len(turns)},
					Error:  bindingErr,
				}
			}
			sourceDigest, bindingErr = CaseCompactionOperationDigestV1(binding)
			if bindingErr != nil {
				return CompactionPlan{
					Result: threaddomain.CompactionResult{ThreadID: threadID, RemainingTurns: len(turns)},
					Error:  bindingErr,
				}
			}
			caseOperationBinding = &binding
		} else {
			sourceDigest = legacyCaseCompactionOperationDigest(
				threadID, compactedTurns, input.SecurityContext, stamp, input.CaseContinuation, input.CaseAuthorityTurnIDs,
			)
		}
	}
	sourceItemIDs := compactionSourceItemIDs(sourceItems)
	summary := GeneralCompactionSummaryTextV3
	if caseBound {
		summary = CaseCompactionSummaryTextV1
	}
	replacedTokens := estimateCompactedTokens(sourceItems)
	if caseBound {
		replacedTokens = estimateCompactedTokensFromTurns(compactedTurns)
	}
	pinnedConstraints := []string{"user: preserve recent turns"}
	summaryItem := map[string]any{
		"id":                itemID,
		"turnId":            turnID,
		"threadId":          threadID,
		"role":              "system",
		"status":            "completed",
		"createdAt":         now,
		"finishedAt":        now,
		"kind":              "compaction",
		"summary":           summary,
		"replacedTokens":    float64(replacedTokens),
		"auto":              input.Auto,
		"pinnedConstraints": stringListAny(pinnedConstraints),
		"sourceDigest":      sourceDigest,
		"digestMarker":      "sha256:" + sourceDigest[:12],
		"sourceItemIds":     stringListAny(sourceItemIDs),
		"schemaVersion":     float64(3),
		"reasoningExcluded": true,
	}
	if caseBound {
		summaryItem["schemaVersion"] = float64(2)
		summaryItem["caseFactsExcluded"] = true
		summaryItem["caseHistoryProjectionVersion"] = float64(1)
		if authorizedCase {
			summaryItem["schemaVersion"] = float64(3)
			summaryItem["caseHistoryProjectionVersion"] = float64(2)
			summaryItem["taskContinuation"] = threaddomain.TaskContinuationSnapshotMapV1(*input.CaseContinuation)
			current, _ := domainsecurity.ParseTurnSecurityContext(input.SecurityContext)
			summaryItem["sourceContextDigest"] = current.ContextDigest
			summaryItem["caseCompactionBinding"] = CaseCompactionOperationBindingMapV1(*caseOperationBinding)
		}
	} else {
		summaryItem["assistantProseExcluded"] = true
		summaryItem["toolPayloadsExcluded"] = true
		summaryItem["caseFactsExcluded"] = true
		summaryItem["providerHistoryProjectionVersion"] = float64(1)
		if input.Auto {
			continuation, continuationErr := BuildTaskContinuationSnapshotV1(input.ContinuationThread)
			if continuationErr != nil {
				return CompactionPlan{Result: threaddomain.CompactionResult{ThreadID: threadID, RemainingTurns: len(turns)}, Error: continuationErr}
			}
			summaryItem["schemaVersion"] = float64(4)
			summaryItem["providerHistoryProjectionVersion"] = float64(2)
			summaryItem["taskContinuation"] = threaddomain.TaskContinuationSnapshotMapV1(continuation)
		}
	}
	summaryItem["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(summaryItem)
	compactTurn := map[string]any{
		"id":         turnID,
		"threadId":   threadID,
		"status":     "completed",
		"prompt":     "/compact",
		"model":      strings.TrimSpace(input.Model),
		"createdAt":  now,
		"startedAt":  now,
		"finishedAt": now,
		"items":      []any{summaryItem},
	}
	nextTurns := make([]any, 0, 1+len(tailTurns))
	if authorizedCase {
		nextTurns = authorizedCaseCompactionTurns(
			turns, input.CaseAuthorityTurnIDs, input.GeneralTerminalAuthorities, len(input.ActiveInheritedTurns),
		)
		compactTurn["caseHistoryProjection"] = "compaction_authority_v1"
		nextTurns = append(nextTurns, compactTurn)
	} else {
		nextTurns = append(nextTurns, compactTurn)
		for _, turn := range tailTurns {
			publicTurn, ok := sanitizeCompactionTailTurn(turn, input.GeneralTerminalAuthorities)
			if caseBound {
				publicTurn, ok = sanitizeCaseCompactionTailTurn(turn)
			}
			if ok {
				nextTurns = append(nextTurns, publicTurn)
			}
		}
	}
	return CompactionPlan{
		Result: threaddomain.CompactionResult{
			ThreadID:                threadID,
			TurnID:                  turnID,
			ItemID:                  itemID,
			Summary:                 summary,
			ReplacedTokens:          replacedTokens,
			PinnedConstraints:       pinnedConstraints,
			SourceDigest:            sourceDigest,
			DigestMarker:            "sha256:" + sourceDigest[:12],
			SourceItemIDs:           sourceItemIDs,
			ReasoningExclusionProof: stringField(summaryItem, "reasoningExclusionProof"),
			CompactedTurns:          len(compactedTurns),
			RemainingTurns:          len(nextTurns),
		},
		NextTurns: nextTurns,
		Changed:   true,
	}
}

func compactionContainsAcceptedFinal(turns []any) bool {
	for _, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		if turn == nil {
			continue
		}
		if turn["acceptedFinal"] != nil {
			return true
		}
		for _, itemValue := range listAny(turn["items"]) {
			item, _ := itemValue.(map[string]any)
			if item != nil && item["acceptedFinal"] != nil {
				return true
			}
		}
	}
	return false
}

func compactionContainsEvidenceSettlement(turns []any) bool {
	for _, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		for _, itemValue := range listAny(turn["items"]) {
			item, _ := itemValue.(map[string]any)
			if item != nil {
				if _, present := item["hostEvidenceSettlement"]; present {
					return true
				}
			}
		}
	}
	return false
}

func compactionCaseBound(securityContext any, turns []any, threadID string, caseMarker bool) (bool, error) {
	values := []any{}
	if securityContext != nil {
		values = append(values, securityContext)
	}
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if turn != nil && turn["securityContext"] != nil {
			values = append(values, turn["securityContext"])
		}
	}
	caseBound := caseMarker
	for _, value := range values {
		parsed, err := domainsecurity.ParseTurnSecurityContext(value)
		if err != nil || parsed.ThreadID != strings.TrimSpace(threadID) {
			return false, errors.New("compaction contains an invalid frozen security context")
		}
		if domainsecurity.TurnSecurityContextIsCaseSensitive(parsed) {
			caseBound = true
		}
	}
	for _, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		if turn == nil {
			continue
		}
		if turn["acceptedFinal"] != nil {
			caseBound = true
		}
		for _, itemValue := range listAny(turn["items"]) {
			item, _ := itemValue.(map[string]any)
			if item != nil && item["acceptedFinal"] != nil {
				caseBound = true
			}
		}
	}
	return caseBound, nil
}

func hasCompactionCaseMarker(thread map[string]any) bool {
	for _, key := range []string{"caseId", "caseProjectId", "caseBindingHash"} {
		if strings.TrimSpace(stringField(thread, key)) != "" {
			return true
		}
	}
	return false
}

func caseCompactionAlreadyReduced(turns []any) bool {
	if len(turns) != 1 {
		return false
	}
	turn, _ := turns[0].(map[string]any)
	for _, value := range listAny(turn["items"]) {
		item, _ := value.(map[string]any)
		factsExcluded, _ := item["caseFactsExcluded"].(bool)
		if stringField(item, "kind") == "compaction" && factsExcluded && canonicalCaseCompactionSummaryItem(item) {
			return true
		}
	}
	return false
}

func canonicalCaseCompactionSummaryItem(item map[string]any) bool {
	if domainevent.ValidatePublicRecord(item) != nil || stringField(item, "summary") != CaseCompactionSummaryTextV1 {
		return false
	}
	allowed := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true, "createdAt": true, "finishedAt": true,
		"kind": true, "summary": true, "replacedTokens": true, "pinnedConstraints": true, "sourceDigest": true,
		"digestMarker": true, "sourceItemIds": true, "schemaVersion": true, "reasoningExcluded": true,
		"reasoningExclusionProof": true, "caseFactsExcluded": true, "caseHistoryProjectionVersion": true,
	}
	for key := range item {
		if !allowed[key] {
			return false
		}
	}
	if values := listAny(item["sourceItemIds"]); len(values) != 0 {
		return false
	}
	return true
}

func sanitizeCaseCompactionTailTurn(value any) (any, bool) {
	turn, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	projected := map[string]any{
		"id": stringField(turn, "id"), "threadId": stringField(turn, "threadId"), "status": stringField(turn, "status"),
		"createdAt": stringField(turn, "createdAt"), "startedAt": stringField(turn, "startedAt"),
		"finishedAt": stringField(turn, "finishedAt"), "caseHistoryProjection": "user_only_untrusted_v1",
	}
	items := []any{}
	for _, itemValue := range listAny(turn["items"]) {
		item, _ := itemValue.(map[string]any)
		if stringField(item, "kind") != "user_message" {
			continue
		}
		userItem := map[string]any{}
		for _, key := range []string{"id", "threadId", "turnId", "kind", "role", "text", "delivery", "status", "createdAt", "finishedAt"} {
			if child, present := item[key]; present {
				userItem[key] = contracts.CloneValue(child)
			}
		}
		if attachmentIDs, ok := safeCompactionStringList(item["attachmentIds"]); ok {
			userItem["attachmentIds"] = attachmentIDs
		}
		if domainevent.ValidatePublicRecord(userItem) == nil {
			items = append(items, userItem)
		}
	}
	projected["items"] = items
	for key, child := range projected {
		if text, isString := child.(string); isString && text == "" {
			delete(projected, key)
		}
	}
	if domainevent.ValidatePublicRecord(projected) != nil {
		return nil, false
	}
	return projected, true
}

func safeCompactionStringList(value any) ([]any, bool) {
	values, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]any, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, false
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, true
}

func legacyCaseCompactionOperationDigest(
	threadID string,
	turns []any,
	securityContext any,
	stamp int64,
	continuation *threaddomain.TaskContinuationSnapshotV1,
	authorityTurnIDs []string,
) string {
	turnBody, _ := json.Marshal(turns)
	current, _ := domainsecurity.ParseTurnSecurityContext(securityContext)
	continuationDigest := ""
	if continuation != nil {
		continuationDigest = continuation.StateDigest
	}
	body, _ := json.Marshal(map[string]any{
		"domain": "analytix.case-compaction/v2", "threadIdHash": domainsecurity.SHA256Hex([]byte(threadID)),
		"sourceContextDigest":  current.ContextDigest,
		"compactedTurnsDigest": domainsecurity.CanonicalJSONHash(turnBody),
		"continuationDigest":   continuationDigest,
		"authorityTurnIds":     canonicalCaseCompactionTurnIDs(authorityTurnIDs), "stamp": stamp,
	})
	return domainsecurity.SHA256Hex(body)
}

func validCaseCompactionContinuation(
	securityContext any,
	continuation *threaddomain.TaskContinuationSnapshotV1,
) bool {
	if continuation == nil {
		return false
	}
	body := threaddomain.TaskContinuationSnapshotMapV1(*continuation)
	parsed, err := threaddomain.ParseTaskContinuationSnapshotV1(body)
	current, contextErr := domainsecurity.ParseTurnSecurityContext(securityContext)
	return err == nil && contextErr == nil && parsed.StateDigest == continuation.StateDigest &&
		domainsecurity.ValidateTurnSecurityContextForCasePublication(current) == nil
}

func validCaseCompactionAuthorityTurnIDs(turns []any, authorityTurnIDs []string, threadID string) bool {
	ids := canonicalCaseCompactionTurnIDs(authorityTurnIDs)
	if len(ids) == 0 || len(ids) != len(authorityTurnIDs) {
		return false
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	seen := map[string]bool{}
	for _, raw := range turns {
		turn, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		turnID := strings.TrimSpace(stringField(turn, "id"))
		if !wanted[turnID] {
			continue
		}
		frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if err != nil || frozen.ThreadID != threadID || frozen.TurnID != turnID || seen[turnID] {
			return false
		}
		seen[turnID] = true
	}
	return len(seen) == len(wanted)
}

func canonicalCaseCompactionTurnIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func authorizedCaseCompactionTurns(
	turns []any,
	authorityTurnIDs []string,
	generalAuthorities map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1,
	inheritedCount int,
) []any {
	wanted := map[string]bool{}
	for _, id := range authorityTurnIDs {
		wanted[strings.TrimSpace(id)] = true
	}
	tailStart := len(turns) - 2
	if tailStart < 0 {
		tailStart = 0
	}
	out := make([]any, 0, len(authorityTurnIDs)+3)
	for index, raw := range turns {
		turn, _ := raw.(map[string]any)
		if index < inheritedCount {
			out = append(out, cloneActiveInheritedCompactionValueV1(turn))
			continue
		}
		turnID := strings.TrimSpace(stringField(turn, "id"))
		generalAuthority, generalGoverned := generalAuthorities[turnID]
		protected := compactionTurnContainsAcceptedFinal(turn) || compactionTurnContainsEvidenceSettlement(turn) ||
			(generalGoverned && generalAuthority.Governed && generalAuthority.Terminal)
		if protected {
			out = append(out, contracts.CloneMap(turn))
			continue
		}
		if wanted[turnID] {
			// Keep the private, metadata-only compaction witness so the next
			// compaction and restart compiler can verify an unbroken signed
			// recovery-digest ancestry. Public projection still emits an empty
			// authority boundary for this turn.
			if strings.TrimSpace(stringField(turn, "caseHistoryProjection")) == "compaction_authority_v1" {
				out = append(out, contracts.CloneMap(turn))
				continue
			}
			if index >= tailStart {
				if projected, ok := sanitizeCaseCompactionTailTurn(turn); ok {
					projectedTurn, _ := projected.(map[string]any)
					projectedTurn["securityContext"] = contracts.CloneValue(turn["securityContext"])
					if snapshot, present := turn["contextEpochSnapshot"]; present {
						projectedTurn["contextEpochSnapshot"] = contracts.CloneValue(snapshot)
					}
					out = append(out, projectedTurn)
					continue
				}
			}
			out = append(out, caseCompactionAuthoritySkeleton(turn))
			continue
		}
		if index >= tailStart {
			if projected, ok := sanitizeCompactionTailTurn(turn, generalAuthorities); ok {
				out = append(out, projected)
			}
		}
	}
	return out
}

func validActiveInheritedCompactionPrefixV1(turns []any, inherited []domainsecurity.ActiveInheritedTurnV1, authorityTurnIDs []string) bool {
	if len(inherited) > len(turns) {
		return false
	}
	wanted := make(map[string]bool, len(inherited))
	for index, entry := range inherited {
		turn, ok := turns[index].(map[string]any)
		body, err := json.Marshal(turn)
		if !ok || !threaddomain.IsCanonicalRecordID(entry.TurnID) || wanted[entry.TurnID] ||
			stringField(turn, "id") != entry.TurnID || err != nil || domainsecurity.SHA256Hex(body) != entry.ContentSHA256 {
			return false
		}
		wanted[entry.TurnID] = true
	}
	for _, id := range authorityTurnIDs {
		if wanted[strings.TrimSpace(id)] {
			return false
		}
	}
	for _, raw := range turns[len(inherited):] {
		turn, _ := raw.(map[string]any)
		if wanted[stringField(turn, "id")] {
			return false
		}
	}
	return true
}

// Preserve JSON number values and every private field while detaching the
// mutable maps/slices. A JSON round trip through float64 can change the digest.
func cloneActiveInheritedCompactionValueV1(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if typed == nil {
			return typed
		}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneActiveInheritedCompactionValueV1(item)
		}
		return out
	case []any:
		if typed == nil {
			return typed
		}
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneActiveInheritedCompactionValueV1(item)
		}
		return out
	default:
		return value
	}
}

func caseCompactionAuthoritySkeleton(turn map[string]any) map[string]any {
	out := map[string]any{
		"id": stringField(turn, "id"), "threadId": stringField(turn, "threadId"),
		"status": stringField(turn, "status"), "items": []any{},
		"securityContext":       contracts.CloneValue(turn["securityContext"]),
		"caseHistoryProjection": "authority_only_v1",
	}
	for _, key := range []string{"model", "createdAt", "startedAt", "finishedAt"} {
		if value, present := turn[key]; present {
			out[key] = contracts.CloneValue(value)
		}
	}
	if snapshot, present := turn["contextEpochSnapshot"]; present {
		out["contextEpochSnapshot"] = contracts.CloneValue(snapshot)
	}
	return out
}

func compactionTurnContainsAcceptedFinal(turn map[string]any) bool {
	if turn == nil {
		return false
	}
	if turn["acceptedFinal"] != nil {
		return true
	}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if item != nil && item["acceptedFinal"] != nil {
			return true
		}
	}
	return false
}

func compactionTurnContainsEvidenceSettlement(turn map[string]any) bool {
	if turn == nil {
		return false
	}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if item != nil {
			if _, present := item["hostEvidenceSettlement"]; present {
				return true
			}
		}
	}
	return false
}

func estimateCompactedTokensFromTurns(turns []any) int {
	body, _ := json.Marshal(turns)
	if len(body) == 0 {
		return 0
	}
	tokens := len(body) / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

func onlyCompactionSourceItems(items []map[string]any) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if stringField(item, "kind") != "compaction" {
			return false
		}
	}
	return true
}

func sanitizeCompactionTailTurn(value any, authoritySets ...map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1) (any, bool) {
	var authorities map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1
	if len(authoritySets) > 0 {
		authorities = authoritySets[0]
	}
	original, originalOK := value.(map[string]any)
	if !originalOK {
		return nil, false
	}
	privateProtocolCallIDs := compactionPrivateProtocolObservedCallIDsV1(original)
	public, ok := domainevent.SanitizePublicValue(contracts.CloneValue(value), false)
	if !ok {
		return nil, false
	}
	turn, ok := public.(map[string]any)
	if !ok {
		return nil, false
	}
	items := make([]any, 0, len(listAny(turn["items"])))
	authority, governed := authorities[stringField(original, "id")]
	for _, item := range listAny(turn["items"]) {
		if record, _ := item.(map[string]any); record != nil {
			if domainevent.IsLegacyAssistantDraftItem(record) {
				continue
			}
			kind := stringField(record, "kind")
			if privateProtocolCallIDs[stringField(record, "callId")] &&
				(kind == "tool_call" || kind == "tool_result") {
				// The retained tail crosses a provider-attempt boundary after
				// public sanitation has removed the source grant. Withhold
				// only host-marked private protocol pairs; ordinary pairs keep
				// their existing native history representation.
				continue
			}
			if kind == "assistant_text" || kind == "error" {
				if !governed || !authority.Terminal || stringField(record, "id") != authority.Commit.TerminalItemID {
					continue
				}
			}
			switch kind {
			case "tool_call":
				item = projectCompactionToolCall(record)
			case "tool_result":
				item = projectCompactionToolResult(record)
			}
		}
		if domainevent.ValidatePublicRecord(item) == nil {
			items = append(items, item)
		}
	}
	turn["items"] = items
	if authority.Terminal {
		if !restoreValidatedGeneralTerminalCompactionAuthority(original, turn) {
			return nil, false
		}
	} else if original["generalTerminalPublication"] != nil || original["generalTerminalCASBinding"] != nil {
		return nil, false
	}
	if domainevent.ValidatePublicRecord(turn) != nil {
		return nil, false
	}
	return turn, true
}

func compactionPrivateProtocolObservedCallIDsV1(turn map[string]any) map[string]bool {
	callIDs := map[string]bool{}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if item == nil || stringField(item, "kind") != "tool_result" {
			continue
		}
		if _, present := item["privateProtocolObserved"]; !present {
			continue
		}
		if callID := stringField(item, "callId"); callID != "" {
			// Durable normalization admits only true, but malformed in-memory
			// presence also fails closed instead of becoming native replay.
			callIDs[callID] = true
		}
	}
	return callIDs
}

func restoreValidatedGeneralTerminalCompactionAuthority(original, projected map[string]any) bool {
	commitValue, hasCommit := original["generalTerminalPublication"]
	bindingValue, hasBinding := original["generalTerminalCASBinding"]
	if !hasCommit && !hasBinding {
		return true
	}
	if !hasCommit || !hasBinding {
		return false
	}
	commit, commitErr := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(commitValue)
	binding, bindingErr := domainturnterminal.ParseGeneralTerminalCASBindingV1(bindingValue)
	minimalThread := map[string]any{"id": stringField(original, "threadId"), "turns": []any{contracts.CloneMap(original)}}
	if commitErr != nil || bindingErr != nil || binding.BindingDigest != commit.AuthorityDigest ||
		ValidateGeneralTerminalPublicationCommitForThreadV1(minimalThread, stringField(original, "id"), commit) != nil {
		return false
	}
	projected["generalTerminalPublication"] = domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit)
	projected["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	if commit.TerminalItemID == "" {
		return true
	}
	projectedItems := listAny(projected["items"])
	for _, rawItem := range projectedItems {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "id") != commit.TerminalItemID {
			continue
		}
		for _, rawOriginalItem := range listAny(original["items"]) {
			originalItem, _ := rawOriginalItem.(map[string]any)
			if stringField(originalItem, "id") != commit.TerminalItemID {
				continue
			}
			itemBinding, err := domainturnterminal.ParseGeneralTerminalCASBindingV1(originalItem["generalTerminalCASBinding"])
			if err != nil || itemBinding.BindingDigest != binding.BindingDigest {
				return false
			}
			item["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(itemBinding)
			return true
		}
		return false
	}
	return false
}

func compactionSourceItems(turns []any, authoritySets ...map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1) []map[string]any {
	var authorities map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1
	if len(authoritySets) > 0 {
		authorities = authoritySets[0]
	}
	items := []map[string]any{}
	for _, turnValue := range turns {
		turn, _ := turnValue.(map[string]any)
		authority, governed := authorities[stringField(turn, "id")]
		for _, itemValue := range listAny(turn["items"]) {
			item, _ := itemValue.(map[string]any)
			kind := stringField(item, "kind")
			if item == nil || kind == "error" || kind == "assistant_reasoning" || kind == "compaction" || domainevent.IsLegacyAssistantDraftItem(item) {
				continue
			}
			if kind == "assistant_text" && (!governed || !authority.Terminal || stringField(item, "id") != authority.Commit.TerminalItemID) {
				continue
			}
			publicItem := contracts.CloneMap(item)
			delete(publicItem, "reasoningContent")
			delete(publicItem, "reasoning_content")
			if kind == "assistant_text" {
				text, err := domainevent.FilterPublicText(rawStringField(publicItem, "text"))
				if err != nil || text == "" {
					continue
				}
				publicItem["text"] = text
			}
			if kind == "tool_call" {
				publicItem = projectCompactionToolCall(publicItem)
			}
			if kind == "tool_result" {
				publicItem = projectCompactionToolResult(publicItem)
			}
			if domainevent.ValidatePublicRecord(publicItem) != nil {
				continue
			}
			items = append(items, publicItem)
		}
	}
	return items
}

func projectCompactionToolCall(item map[string]any) map[string]any {
	return domaintoolcall.PublicToolCallItemRecordV1(item)
}

func projectCompactionToolResult(item map[string]any) map[string]any {
	return domaintoolresult.PublicToolResultItemRecordV1(item)
}

func compactionSourceDigest(items []map[string]any) string {
	data, _ := json.Marshal(items)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

func compactionSourceItemIDs(items []map[string]any) []string {
	ids := []string{}
	for _, item := range items {
		if id := strings.TrimSpace(stringField(item, "id")); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func compactionSummary(items []map[string]any, reason string) string {
	lines := []string{"Compacted conversation history."}
	if reason != "" {
		lines = append(lines, "Reason: "+reason+".")
	}
	for _, item := range items {
		text := compactionItemSummaryText(item)
		if text == "" {
			continue
		}
		lines = append(lines, "- "+text)
		if len(lines) >= 8 {
			break
		}
	}
	return strings.Join(lines, "\n")
}

func compactionItemSummaryText(item map[string]any) string {
	kind := stringField(item, "kind")
	switch kind {
	case "user_message", "assistant_text":
		if text := truncateCompactionText(stringField(item, "text"), 220); text != "" {
			return kind + ": " + text
		}
	case "tool_call":
		return "tool_call: " + firstNonEmptyCompactionString(stringField(item, "toolName"), "unknown_tool")
	case "tool_result":
		return "tool_result: " + firstNonEmptyCompactionString(stringField(item, "toolName"), "unknown_tool")
	case "compaction":
		if summary := truncateCompactionText(stringField(item, "summary"), 220); summary != "" {
			return "compaction: " + summary
		}
	}
	return ""
}

func truncateCompactionText(text string, limit int) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}

func firstNonEmptyCompactionString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func estimateCompactedTokens(items []map[string]any) int {
	bytes := 0
	for _, item := range items {
		data, _ := json.Marshal(item)
		bytes += len(data)
	}
	if bytes == 0 {
		return 0
	}
	tokens := bytes / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

func listAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}
