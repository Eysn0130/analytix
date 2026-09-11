package thread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

var ErrPublicProjectionPending = errors.New("public thread projection is pending committed authority")

type caseCompactionCommittedContextReaderV1 interface {
	CommittedContext(string, string) (casethreadapp.CommittedContext, bool)
}

const (
	CaseBoundaryOnlyHistoryAuthority = "case_boundary_only_v1"
	CasePublicThreadTitle            = "案件分析"
)

// ProjectPublicThread exposes only user-authored messages and signed,
// deterministic AcceptedFinal assistant items. Raw drafts and tool history
// never become public evidence authority.
func ProjectPublicThread(thread map[string]any) (map[string]any, error) {
	return NewTrustedPublicProjector(nil).ProjectThread(thread)
}

func projectPublicThreadWithAuthority(thread map[string]any, trusted *gateprojection.TrustedFinalProjectionIndex, caseThreads CaseThreadAuthority, currentAuthority CurrentCaseThreadAuthorityValidator, primaryCAS finalauthorityport.AcceptedFinalCASReader, preservedHistory ...PreservedDerivedHistoryV1) (map[string]any, error) {
	threadID := strings.TrimSpace(contracts.StringField(thread, "id"))
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil {
		return nil, err
	}
	caseSensitive = caseSensitive || (threadID != "" && (trusted.ContainsThread(threadID) || caseThreadKnown(caseThreads, threadID)))
	if !caseSensitive {
		return projectOrdinaryPublicThread(thread)
	}
	if threadID == "" {
		return nil, errors.New("case thread identity is required for public projection")
	}
	var inheritedTurns map[string]bool
	if caseThreads != nil && !caseThreads.RestartPreservesThreadV1(threadID) {
		if reader, ok := caseThreads.(casethreadapp.ActiveInheritedHistoryReaderV1); ok {
			primary, _ := primaryCAS.(recoveryport.PrimaryThreadReaderV1)
			inheritedTurns, err = ValidateActiveInheritedHistoryV1(context.Background(), thread, reader, primary)
			if err != nil {
				return nil, err
			}
		} else if _, present := thread["activeInheritedHistoryReceipt"]; present {
			return nil, errors.New("active inherited history reader is unavailable")
		}
	}
	generalAuthorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if err != nil {
		return nil, errors.Join(errors.New("case thread general terminal authority is invalid"), err)
	}
	primaryObservations, primaryCurrentContext, err := trustedPrimaryCASObservationsV1(
		threadID, trusted, primaryCAS,
	)
	if err != nil {
		return nil, err
	}
	currentContext, currentContextErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if primaryCurrentContext != nil {
		if currentContextErr != nil || !reflect.DeepEqual(currentContext, *primaryCurrentContext) {
			return nil, errors.New("case thread normalized current context differs from primary CAS authority")
		}
		currentContext = *primaryCurrentContext
		currentContextErr = nil
	}
	currentContextTrusted := currentContextErr == nil && (!domainsecurity.TurnSecurityContextIsCaseSensitive(currentContext) || caseThreads == nil || caseThreads.ContainsContext(currentContext))
	currentWorkspaceMatches := currentContextTrusted && strings.TrimSpace(contracts.StringField(thread, "workspace")) == currentContext.WorkspaceRealPath
	if caseThreadKnown(caseThreads, threadID) {
		currentContext, currentContextErr = validateCurrentCaseThreadAuthority(currentAuthority, threadID, thread)
		currentWorkspaceMatches = currentContextErr == nil
	}
	projected := contracts.ThreadSummary(thread)
	projected["historyAuthority"] = CaseBoundaryOnlyHistoryAuthority
	projected["title"] = CasePublicThreadTitle
	delete(projected, "workspace")
	delete(projected, "forkedFromTitle")
	delete(projected, "preview")
	delete(projected, "goal")
	delete(projected, "todos")
	if todos, _ := thread["todos"].(map[string]any); todos != nil {
		projectedAt := strings.TrimSpace(contracts.StringField(thread, "updatedAt"))
		audit, auditErr := CaseTerminalTodoAuditProjectionV1(todos, threadID, projectedAt)
		if auditErr != nil {
			return nil, auditErr
		}
		if audit != nil {
			projected["todos"] = audit
		}
	}
	turns, _ := thread["turns"].([]any)
	projectedTurns := make([]any, 0, len(turns))
	expectedCurrentTurns := map[string]bool{}
	if currentWorkspaceMatches {
		for _, record := range trusted.RecordsForThread(threadID) {
			if samePublicEpoch(currentContext, record.SecurityContext) {
				expectedCurrentTurns[record.SecurityContext.TurnID] = true
			}
		}
	}
	seenTurnIDs := map[string]bool{}
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		if turnID == "" {
			return nil, errors.New("case thread contains a turn without identity")
		}
		if seenTurnIDs[turnID] {
			return nil, errors.New("case thread contains a duplicate turn identity")
		}
		seenTurnIDs[turnID] = true
		publicTurn := selectPublicFields(turn, []string{
			"id", "threadId", "status", "model", "createdAt", "startedAt", "finishedAt",
		})
		publicTurn["id"] = turnID
		publicTurn["threadId"] = threadID
		if err := projectCasePublicTurnReasoningEffortV1(publicTurn, turn); err != nil {
			return nil, err
		}
		items, _ := turn["items"].([]any)
		trustedRecord, disposition, indexedFinal := trusted.ResolveCommitted(threadID, turnID)
		if !indexedFinal && trusted.TerminalProjectionPending(threadID, turnID, turn["acceptedFinal"]) {
			return nil, ErrPublicProjectionPending
		}
		generalAuthority, generalGoverned := generalAuthorities[turnID]
		if inheritedTurns[turnID] {
			if indexedFinal || generalGoverned {
				return nil, errors.New("inherited turn collides with target final authority")
			}
			publicTurn["items"] = []any{}
			projectedTurns = append(projectedTurns, publicTurn)
			continue
		}
		if indexedFinal && generalGoverned {
			return nil, errors.New("case turn carries conflicting general terminal authority")
		}
		if generalGoverned && generalAuthority.Terminal {
			generalTurn, projectionErr := projectCaseHistoryGeneralTerminalV1(threadID, turn, generalAuthority)
			if projectionErr != nil {
				return nil, projectionErr
			}
			projectedTurns = append(projectedTurns, generalTurn)
			continue
		}
		if !indexedFinal {
			_, contextPresent := turn["securityContext"]
			_, projectionPresent := turn["caseHistoryProjection"]
			if !contextPresent && projectionPresent && len(preservedHistory) == 1 && preservedHistory[0] != nil &&
				caseThreads != nil && caseThreads.RestartPreservesThreadV1(threadID) {
				if err := ValidateCaseDerivedHistoryTurnV1(threadID, turn); err != nil {
					return nil, err
				}
				if err := preservedHistory[0].ValidateInheritedTurnV1(context.Background(), thread, turn); err != nil {
					return nil, err
				}
				// A held inherited marker is neither a current compaction nor
				// public content authority. Keep only the usual case turn metadata.
				publicTurn["items"] = []any{}
				projectedTurns = append(projectedTurns, publicTurn)
				continue
			}
			retainedTurn, retained, retentionErr := projectCaseCompactionRetentionTurnV1(
				threadID, turn, caseThreads,
			)
			if retentionErr != nil {
				return nil, retentionErr
			}
			if retained {
				projectedTurns = append(projectedTurns, retainedTurn)
				continue
			}
		}
		if (len(primaryObservations) > 0 || inheritedTurns != nil) && !indexedFinal {
			turnContext, turnContextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			currentInFlight := currentContextErr == nil && turnContextErr == nil &&
				turnID == currentContext.TurnID && reflect.DeepEqual(turnContext, currentContext) &&
				!publicTerminalStatus(contracts.StringField(turn, "status"))
			if !currentInFlight {
				return nil, errors.New("case thread contains a turn without current public admission authority")
			}
		}
		var trustedPlan appturn.AcceptedFinalPublicationPlan
		if indexedFinal {
			trustedPlan, err = appturn.BuildAcceptedFinalPublicationPlan(trustedRecord.AcceptedFinal, trustedRecord.RenderedText, trustedRecord.PublicationIntent)
			if err != nil || trustedPlan.EventManifestDigest != disposition.EventManifestDigest {
				return nil, errors.New("case turn publication manifest is not trusted")
			}
			observation, found := primaryObservations[turnID]
			if !found || observation.TurnProjectionSHA256 != disposition.TurnCASDigest {
				return nil, errors.New("case turn lacks its strict primary CAS observation")
			}
		}
		trustedFinal := indexedFinal && currentWorkspaceMatches && samePublicEpoch(currentContext, trustedRecord.SecurityContext)
		var trustedPublicView map[string]any
		if trustedFinal {
			delete(expectedCurrentTurns, turnID)
			view, viewErr := domainevidence.NewAcceptedFinalPublicViewFromRecordV3(trustedRecord.AcceptedFinal)
			if viewErr != nil {
				return nil, errors.New("case turn accepted-final public view is not trusted")
			}
			trustedPublicView = domainevidence.AcceptedFinalPublicViewRecordV3(view)
			if trustedPublicView == nil {
				return nil, errors.New("case turn accepted-final public view is unavailable")
			}
		}
		if trusted != nil {
			hasAuthority := turn["acceptedFinal"] != nil
			for _, rawItem := range items {
				item, _ := rawItem.(map[string]any)
				hasAuthority = hasAuthority || (contracts.StringField(item, "kind") == "assistant_text" && item["acceptedFinal"] != nil)
			}
			terminal := publicTerminalStatus(contracts.StringField(turn, "status"))
			turnContext, turnContextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			hostCaseTerminal := terminal && turnContextErr == nil && domainsecurity.TurnSecurityContextIsCaseSensitive(turnContext) &&
				caseThreads != nil && caseThreads.ContainsContext(turnContext)
			if hasAuthority != indexedFinal || (indexedFinal && !terminal) || (hostCaseTerminal && !indexedFinal) {
				return nil, errors.New("case turn public authority is not in the trusted committed index")
			}
		}
		publicItems := make([]any, 0, len(items))
		acceptedAssistantCount := 0
		acceptedTerminalErrorCount := 0
		var trustedAssistantItem map[string]any
		var trustedTerminalErrorItem map[string]any
		trustedTerminalErrorExpected := false
		if trustedFinal {
			trustedTerminalErrorItem, trustedTerminalErrorExpected = trustedAcceptedFinalTerminalErrorItemV1(
				trustedRecord,
				trustedPlan,
			)
			requiresTerminalError, knownTerminalReason := domainterminal.AcceptedFinalDeliveryRequiresErrorItemV1(
				trustedRecord.AcceptedFinal.TerminalReason,
			)
			if !knownTerminalReason || requiresTerminalError != trustedTerminalErrorExpected {
				return nil, errors.New("trusted case terminal item does not match its closed publication plan")
			}
		}
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			var trustedAuthority *domainevidence.PrivateAcceptedFinalRecord
			var trustedItem map[string]any
			if trustedFinal {
				trustedAuthority = &trustedRecord
				trustedItem = trustedPlan.TurnItems[0]
			}
			if trusted == nil || trustedFinal {
				if boundaryItem, ok := projectSafeBoundaryAssistant(threadID, turnID, turn, item, trustedAuthority, trustedItem, trustedPublicView); ok {
					publicItems = append(publicItems, boundaryItem)
					if trustedAuthority != nil {
						publicTurn["acceptedFinalView"] = contracts.CloneValue(trustedPublicView)
						publicTurn["status"] = trustedRecord.PublicationIntent.TerminalStatus
						publicTurn["finishedAt"] = trustedRecord.AcceptedFinal.AcceptedAt
					}
					acceptedAssistantCount++
					if trustedAuthority != nil {
						trustedAssistantItem = boundaryItem
					}
					continue
				}
			}
			if trustedTerminalErrorExpected && samePublicJSON(item, trustedTerminalErrorItem) {
				publicItems = append(publicItems, contracts.CloneMap(trustedTerminalErrorItem))
				acceptedTerminalErrorCount++
				continue
			}
			// Persisted case-user content has no host-issued admission receipt yet.
			// Withhold it from every ordinary public surface until the versioned
			// user-admission authority is implemented; the current request remains
			// available only inside the private execution boundary.
		}
		if trusted != nil && trustedFinal && acceptedAssistantCount != 1 {
			return nil, errors.New("trusted case final requires exactly one public assistant item")
		}
		if trusted != nil && trustedFinal {
			expectedTerminalErrors := 0
			if trustedTerminalErrorExpected {
				expectedTerminalErrors = 1
			}
			if acceptedTerminalErrorCount != expectedTerminalErrors {
				return nil, errors.New("trusted case final terminal item is detached from its publication plan")
			}
			publicItems = []any{trustedAssistantItem}
			if trustedTerminalErrorExpected {
				publicItems = append(publicItems, contracts.CloneMap(trustedTerminalErrorItem))
			}
		}
		publicTurn["items"] = publicItems
		projectedTurns = append(projectedTurns, publicTurn)
	}
	if len(expectedCurrentTurns) != 0 {
		return nil, errors.New("case thread omits a trusted committed turn")
	}
	projected["turns"] = projectedTurns
	projected["turnCount"] = float64(len(projectedTurns))
	projected["messageCount"] = float64(0)
	return projected, nil
}

func projectCaseCompactionRetentionTurnV1(
	threadID string,
	turn map[string]any,
	caseThreads CaseThreadAuthority,
) (map[string]any, bool, error) {
	projection := strings.TrimSpace(contracts.StringField(turn, "caseHistoryProjection"))
	if projection != "authority_only_v1" && projection != "user_only_untrusted_v1" &&
		projection != "compaction_authority_v1" {
		return nil, false, nil
	}
	turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || caseThreads == nil || !caseThreads.ContainsContext(frozen) ||
		frozen.ThreadID != threadID || frozen.TurnID != turnID ||
		!domainsecurity.TurnSecurityContextIsCaseSensitive(frozen) || turn["acceptedFinal"] != nil {
		return nil, false, errors.New("case compaction retention turn lacks committed host authority")
	}
	items, ok := turn["items"].([]any)
	if !ok {
		return nil, false, errors.New("case compaction retention turn items are invalid")
	}
	switch projection {
	case "authority_only_v1":
		if len(items) != 0 {
			return nil, false, errors.New("case compaction authority-only turn carries content")
		}
	case "user_only_untrusted_v1":
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if strings.TrimSpace(contracts.StringField(item, "kind")) != "user_message" || item["acceptedFinal"] != nil ||
				domainevent.ValidatePublicRecord(item) != nil {
				return nil, false, errors.New("case compaction user tail carries non-user authority")
			}
		}
	case "compaction_authority_v1":
		if len(items) != 1 {
			return nil, false, errors.New("case compaction authority turn is incomplete")
		}
		item, _ := items[0].(map[string]any)
		factsExcluded, _ := item["caseFactsExcluded"].(bool)
		continuation, continuationErr := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
		if item == nil || strings.TrimSpace(contracts.StringField(item, "kind")) != "compaction" ||
			strings.TrimSpace(contracts.StringField(item, "summary")) != appturn.CaseCompactionSummaryTextV1 ||
			!factsExcluded || continuationErr != nil ||
			!domainsecurity.IsSHA256Hex(strings.TrimSpace(contracts.StringField(item, "sourceDigest"))) ||
			!domainsecurity.IsSHA256Hex(strings.TrimSpace(contracts.StringField(item, "sourceContextDigest"))) ||
			strings.TrimSpace(contracts.StringField(item, "reasoningExclusionProof")) != domainevent.ReasoningExclusionProof(item) {
			return nil, false, errors.New("case compaction continuation authority is invalid")
		}
		rawBinding, bindingPresent := item["caseCompactionBinding"]
		if !bindingPresent {
			break
		}
		binding, bindingErr := appturn.ParseCaseCompactionOperationBindingV1(rawBinding)
		digest, digestErr := appturn.CaseCompactionOperationDigestV1(binding)
		snapshot, _ := turn["contextEpochSnapshot"].(map[string]any)
		committedReader, committedReaderOK := caseThreads.(caseCompactionCommittedContextReaderV1)
		committed, committedFound := casethreadapp.CommittedContext{}, false
		if committedReaderOK {
			committed, committedFound = committedReader.CommittedContext(threadID, turnID)
		}
		signedAuto, signedAutoErr := contextepochapp.CompactionAutoMode(committed.EpochState)
		itemAuto, itemAutoOK := item["auto"].(bool)
		issuedAt, issuedAtErr := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
		expectedStamp := ""
		if issuedAtErr == nil {
			expectedStamp = strconv.FormatInt(issuedAt.UnixNano(), 10)
		}
		safeThreadID := contracts.SafeRecordID(threadID)
		expectedTurnID := fmt.Sprintf("turn_%s_compaction_%s", safeThreadID, binding.OperationStamp)
		expectedItemID := fmt.Sprintf("compaction_%s_%s", safeThreadID, binding.OperationStamp)
		if bindingErr != nil || digestErr != nil ||
			!committedReaderOK || !committedFound || committed.SecurityContext != frozen ||
			domaincontextepoch.ValidateState(committed.EpochState) != nil || signedAutoErr != nil || issuedAtErr != nil ||
			!itemAutoOK || itemAuto != signedAuto || binding.OperationStamp != expectedStamp ||
			binding.ThreadIDHash != domainsecurity.SHA256Hex([]byte(threadID)) ||
			binding.SourceContextDigest != strings.TrimSpace(contracts.StringField(item, "sourceContextDigest")) ||
			binding.ContinuationDigest != continuation.StateDigest ||
			digest != strings.TrimSpace(contracts.StringField(item, "sourceDigest")) ||
			contracts.StringField(item, "turnId") != turnID ||
			contracts.StringField(item, "threadId") != threadID ||
			turnID != expectedTurnID || contracts.StringField(item, "id") != expectedItemID ||
			contracts.StringField(item, "createdAt") != frozen.IssuedAt ||
			contracts.StringField(item, "finishedAt") != frozen.IssuedAt ||
			snapshot == nil || contracts.StringField(snapshot, "threadId") != threadID ||
			!numericEqualsV1(snapshot["epoch"], int(frozen.ContextEpoch)) ||
			contracts.StringField(snapshot, "recoveryDigest") != digest ||
			!sameContextEpochSnapshotV1(snapshot, committed.EpochState.AcceptedSnapshot) ||
			committed.EpochState.AcceptedSnapshot.RecoveryDigest != digest {
			return nil, false, errors.New("case compaction operation binding is invalid")
		}
		marker, markerErr := projectCaseCompactionPublicMarkerV1(item)
		if markerErr != nil {
			return nil, false, markerErr
		}
		items = []any{marker}
	}
	publicTurn := selectPublicFields(turn, []string{
		"id", "threadId", "status", "model", "createdAt", "startedAt", "finishedAt",
	})
	publicTurn["id"] = turnID
	publicTurn["threadId"] = threadID
	if err := projectCasePublicTurnReasoningEffortV1(publicTurn, turn); err != nil {
		return nil, false, err
	}
	if projection == "compaction_authority_v1" && len(items) == 1 {
		if marker, _ := items[0].(map[string]any); validProjectedCaseCompactionItemV1(marker) {
			publicTurn["items"] = []any{marker}
			return publicTurn, true, nil
		}
	}
	publicTurn["items"] = []any{}
	return publicTurn, true, nil
}

func projectCasePublicTurnReasoningEffortV1(publicTurn, turn map[string]any) error {
	raw, present := turn["reasoningEffort"]
	if !present {
		return nil
	}
	effort, ok := raw.(string)
	if !ok {
		return errors.New("case public turn reasoning effort is invalid")
	}
	canonical, valid := domainmodel.ProjectReasoningEffortV1(effort)
	if !valid {
		return errors.New("case public turn reasoning effort is invalid")
	}
	if canonical == "" {
		delete(publicTurn, "reasoningEffort")
		return nil
	}
	publicTurn["reasoningEffort"] = canonical
	return nil
}

func projectCaseCompactionPublicMarkerV1(item map[string]any) (map[string]any, error) {
	return appturn.ProjectCaseCompactionPublicMarker(item)
}

func validProjectedCaseCompactionItemV1(item map[string]any) bool {
	return appturn.ValidateCaseCompactionPublicMarker(item)
}

// projectCaseHistoryGeneralTerminalV1 admits only the exact ordinary terminal
// item already bound to the turn's atomic general-terminal CAS/outbox. Earlier
// ordinary drafts and user prose remain private after the thread gains case
// authority, while the completed ordinary result remains available in the
// same public history.
func projectCaseHistoryGeneralTerminalV1(
	threadID string,
	turn map[string]any,
	authority domainturnterminal.GeneralTerminalProjectionAuthorityV1,
) (map[string]any, error) {
	turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
	if !authority.Governed || !authority.Terminal || authority.Commit.ThreadID != threadID ||
		authority.Commit.TurnID != turnID || authority.Commit.TerminalStatus != contracts.StringField(turn, "status") {
		return nil, errors.New("case history general terminal authority is detached")
	}
	publicTurn := selectPublicFields(turn, []string{
		"id", "threadId", "status", "model", "createdAt", "startedAt", "finishedAt",
	})
	publicTurn["id"] = turnID
	publicTurn["threadId"] = threadID
	if err := projectCasePublicTurnReasoningEffortV1(publicTurn, turn); err != nil {
		return nil, err
	}
	publicItems := []any{}
	matchedTerminalItemCount := 0
	items, _ := turn["items"].([]any)
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if authority.Commit.TerminalItemID == "" ||
			strings.TrimSpace(contracts.StringField(item, "id")) != authority.Commit.TerminalItemID {
			continue
		}
		matchedTerminalItemCount++
		projected, ok := projectOrdinaryPublicHistoryItemV1(item)
		if !ok || !oneOfPublicValue(contracts.StringField(projected, "kind"), "assistant_text", "error") {
			return nil, errors.New("case history general terminal item is invalid")
		}
		if contracts.StringField(projected, "kind") == "assistant_text" {
			slot, slotErr := domainordinaryresult.ParseResultSlotV1(item["ordinaryResult"])
			if slotErr != nil || contracts.StringField(projected, "text") != slot.Text {
				// Legacy or tampered ordinary prose remains withheld after the
				// thread becomes case-bound. Never synthesize authority from text.
				continue
			}
			projected["ordinaryResult"] = domainordinaryresult.ResultSlotV1Map(slot)
		}
		publicItems = append(publicItems, projected)
	}
	expected := 0
	if authority.Commit.TerminalItemID != "" {
		expected = 1
	}
	if matchedTerminalItemCount != expected {
		return nil, errors.New("case history general terminal item does not match its canonical outbox")
	}
	publicTurn["items"] = publicItems
	return publicTurn, nil
}

func trustedPrimaryCASObservationsV1(
	threadID string,
	trusted *gateprojection.TrustedFinalProjectionIndex,
	primaryCAS finalauthorityport.AcceptedFinalCASReader,
) (map[string]domainevidence.AcceptedFinalCASObservationV1, *domainsecurity.TurnSecurityContext, error) {
	observations := map[string]domainevidence.AcceptedFinalCASObservationV1{}
	if trusted == nil {
		return observations, nil, nil
	}
	records := trusted.RecordsForThread(threadID)
	if len(records) == 0 {
		return observations, nil, nil
	}
	if primaryCAS == nil {
		return nil, nil, errors.New("trusted final projection requires a strict primary CAS reader")
	}
	turnIDs := make([]string, 0, len(records))
	expectedTurnIDs := make(map[string]bool, len(records))
	for _, record := range records {
		turnID := record.SecurityContext.TurnID
		if expectedTurnIDs[turnID] {
			return nil, nil, errors.New("trusted final index duplicates a primary CAS turn")
		}
		expectedTurnIDs[turnID] = true
		turnIDs = append(turnIDs, turnID)
	}
	if batch, ok := primaryCAS.(finalauthorityport.AcceptedFinalCASProjectionReader); ok {
		var err error
		observations, err = batch.ReadAcceptedFinalCASObservations(context.Background(), threadID, turnIDs)
		if err != nil || len(observations) != len(records) {
			return nil, nil, errors.Join(errors.New("strict primary CAS snapshot is unavailable"), err)
		}
		for turnID := range observations {
			if !expectedTurnIDs[turnID] {
				return nil, nil, errors.New("strict primary CAS snapshot contains an unrequested turn")
			}
		}
	}
	var current *domainsecurity.TurnSecurityContext
	threadFileSHA256 := ""
	for _, record := range records {
		turnID := record.SecurityContext.TurnID
		dispositionRecord, disposition, found := trusted.ResolveCommitted(threadID, turnID)
		if !found || !reflect.DeepEqual(dispositionRecord, record) {
			return nil, nil, errors.New("trusted final index changed during primary CAS projection")
		}
		observation, observed := observations[turnID]
		var err error
		if !observed {
			observation, err = primaryCAS.ReadAcceptedFinalCASObservation(context.Background(), threadID, turnID)
		}
		if err != nil || domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil ||
			!observation.HasWinner || !reflect.DeepEqual(observation.Winner, record.AcceptedFinal) ||
			!reflect.DeepEqual(observation.FrozenContext, record.SecurityContext) ||
			observation.Status != record.PublicationIntent.TerminalStatus ||
			observation.TurnProjectionSHA256 != disposition.TurnCASDigest {
			return nil, nil, errors.Join(errors.New("trusted final no longer matches strict primary CAS authority"), err)
		}
		if threadFileSHA256 == "" {
			threadFileSHA256 = observation.ThreadFileSHA256
		} else if threadFileSHA256 != observation.ThreadFileSHA256 {
			return nil, nil, errors.New("strict primary CAS observations span different thread snapshots")
		}
		if current == nil {
			value := observation.CurrentContext
			current = &value
		} else if !reflect.DeepEqual(*current, observation.CurrentContext) {
			return nil, nil, errors.New("strict primary CAS observations disagree on current context")
		}
		observations[turnID] = observation
	}
	return observations, current, nil
}

func projectOrdinaryPublicThread(thread map[string]any) (map[string]any, error) {
	authorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if err != nil {
		return nil, err
	}
	public, ok := domainevent.SanitizePublicValue(thread, false)
	if !ok {
		return nil, errors.New("ordinary public thread contains an unsafe value")
	}
	projected, _ := public.(map[string]any)
	if projected == nil {
		return nil, errors.New("ordinary public thread projection is unavailable")
	}
	projected = selectPublicFields(projected, []string{
		"id", "title", "autoTitle", "workspace", "model", "providerId", "mode", "status",
		"executionPolicyVersion", "approvalPolicy", "sandboxMode", "costBudgetUsd", "costBudgetWarningSent",
		"relation", "parentThreadId", "forkedFromThreadId", "forkedFromTitle", "forkedAt",
		"forkedFromMessageCount", "forkedFromTurnCount", "runtimeStepLimits", "goal", "todos",
		"createdAt", "updatedAt", "turns",
	})
	if rawGoal, present := projected["goal"]; present {
		goal, ok := projectOrdinaryPublicGoalV1(rawGoal)
		if !ok {
			return nil, errors.New("ordinary public goal is outside the closed projection")
		}
		projected["goal"] = goal
	}
	if status := strings.TrimSpace(contracts.StringField(projected, "status")); status != "" {
		if !oneOfPublicValue(status, "idle", "running", "archived", "deleted") {
			return nil, errors.New("ordinary public thread status is outside the closed lifecycle")
		}
		projected["status"] = status
	}
	turns, _ := projected["turns"].([]any)
	rawTurns, _ := thread["turns"].([]any)
	if len(turns) != len(rawTurns) {
		return nil, errors.New("ordinary public thread turn projection is incomplete")
	}
	seenAuthorities := map[string]bool{}
	for index, rawTurn := range turns {
		sanitizedTurn, _ := rawTurn.(map[string]any)
		sourceTurn, _ := rawTurns[index].(map[string]any)
		if sanitizedTurn == nil || sourceTurn == nil {
			return nil, errors.New("ordinary public thread contains an invalid turn")
		}
		turn := selectPublicFields(sanitizedTurn, []string{
			"id", "threadId", "status", "prompt", "model", "reasoningEffort", "steering",
			"createdAt", "startedAt", "finishedAt", "items", "acceptedFinal", "acceptedFinalView",
			"attachmentIds", "activeSkillIds", "injectedMemoryIds", "skillInjectionBytes",
			"workspaceCheckpointId", "toolCatalogFingerprint", "toolCatalogToolCount", "toolCatalogDrift",
			"maxModelSteps", "guiPlan", "mode", "disableUserInput", "error",
		})
		turns[index] = turn
		legacyInterruptedHistory := false
		if status := strings.TrimSpace(contracts.StringField(turn, "status")); status != "" {
			switch status {
			case "running", "completed", "failed", "aborted":
				turn["status"] = status
			case "interrupted", "killed", "canceled", "cancelled", "timeout":
				// The public TurnStatus contract intentionally has one aborted
				// state. Preserve richer legacy settlement metadata privately,
				// while mapping its terminal snapshot to that closed status.
				turn["status"] = "aborted"
				legacyInterruptedHistory = true
			default:
				return nil, errors.New("ordinary public turn status is outside the closed lifecycle")
			}
		}
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		authority, governed := authorities[turnID]
		if governed {
			seenAuthorities[turnID] = true
		}
		items, _ := turn["items"].([]any)
		sourceItems, _ := sourceTurn["items"].([]any)
		if len(items) != len(sourceItems) {
			return nil, errors.New("ordinary public turn item projection is incomplete")
		}
		publicItems := make([]any, 0, len(items))
		terminalItemCount := 0
		for itemIndex, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			sourceItem, _ := sourceItems[itemIndex].(map[string]any)
			if item == nil || sourceItem == nil {
				return nil, errors.New("ordinary public turn contains an invalid item")
			}
			// SanitizePublicValue has already stripped private durable bindings
			// from its recursive copies. Only a tool result is re-projected from
			// the raw durable item so case-source status must pass its host-only
			// proof exactly once; every other kind stays on the sanitized copy.
			if strings.TrimSpace(contracts.StringField(item, "kind")) == "tool_result" {
				if strings.TrimSpace(contracts.StringField(sourceItem, "kind")) != "tool_result" {
					return nil, errors.New("ordinary public tool-result source identity changed")
				}
				item = sourceItem
			}
			if domainevent.IsLegacyAssistantDraftItem(item) {
				continue
			}
			itemKind := strings.TrimSpace(contracts.StringField(item, "kind"))
			// Approval-derived grants remain in the private durable registry so
			// execution can revalidate their exact transition. They are control
			// authority, not public history, and must never enter UI/sidecars.
			if itemKind == executiongrantapp.GrantTransitionItemKind {
				continue
			}
			if legacyInterruptedHistory && itemKind == "tool_call" {
				legacyProjection := domaintoolcall.PublicToolCallItemRecordV1(item)
				if withheld, _ := legacyProjection["legacyIdentityWithheld"].(bool); withheld {
					continue
				}
			}
			if carriesUnsafeToolIdentityV1(item) {
				if legacyInterruptedHistory || (itemKind != "tool_call" && itemKind != "tool_result") {
					continue
				}
			}
			if itemKind == "assistant_text" || itemKind == "error" {
				if !governed || !authority.Terminal || strings.TrimSpace(contracts.StringField(item, "id")) != authority.Commit.TerminalItemID {
					continue
				}
				terminalItemCount++
			}
			projectedItem, ok := projectOrdinaryPublicHistoryItemV1(item)
			if !ok {
				return nil, fmt.Errorf(
					"ordinary public item lifecycle is outside the closed projection: %s",
					ordinaryPublicHistoryItemBlockerV1(item),
				)
			}
			publicItems = append(publicItems, projectedItem)
		}
		if governed && authority.Terminal {
			expected := 0
			if authority.Commit.TerminalItemID != "" {
				expected = 1
			}
			if terminalItemCount != expected {
				return nil, errors.New("ordinary terminal public item does not match its canonical outbox")
			}
		}
		turn["items"] = publicItems
	}
	if len(seenAuthorities) != len(authorities) {
		return nil, errors.New("ordinary public projection omitted a context-bound turn")
	}
	projected["turns"] = turns
	return projected, nil
}

func projectOrdinaryPublicGoalV1(value any) (map[string]any, bool) {
	goal, _ := value.(map[string]any)
	if goal == nil {
		return nil, false
	}
	// Durable research state carries only a private opaque reference. Do not
	// synthesize the retired operational paths expected by legacy clients.
	projected := selectPublicFields(goal, []string{
		"id", "threadId", "objective", "status", "tokenBudget", "tokensUsed", "timeUsedSeconds",
		"evidenceLedger", "blockedReason", "blockedCount", "blockedTurnId", "strictCompletion",
		"selfCheckRequired", "selfCheckCompleted", "selfCheckTurnId", "createdAt", "updatedAt",
	})
	if !validNonemptyPublicStringV1(projected["threadId"]) ||
		!validTrimmedPublicStringV1(projected, "objective", 1, 4_000, true) ||
		!oneOfPublicValue(contracts.StringField(projected, "status"), "active", "paused", "blocked", "usageLimited", "budgetLimited", "complete") ||
		!validPublicIntegerV1(projected["tokensUsed"], false) ||
		!validPublicIntegerV1(projected["timeUsedSeconds"], false) ||
		!validPublicStringV1(projected, "createdAt", true) ||
		!validPublicStringV1(projected, "updatedAt", true) {
		return nil, false
	}
	if rawID, present := projected["id"]; present && !validNonemptyPublicStringV1(rawID) {
		return nil, false
	}
	if rawBudget, present := projected["tokenBudget"]; present && rawBudget != nil && !validPublicIntegerV1(rawBudget, true) {
		return nil, false
	}
	if _, present := projected["blockedReason"]; present && !validTrimmedPublicStringV1(projected, "blockedReason", 1, 1_000, true) {
		return nil, false
	}
	if rawCount, present := projected["blockedCount"]; present && !validPublicIntegerV1(rawCount, true) {
		return nil, false
	}
	for _, field := range []string{"blockedTurnId", "selfCheckTurnId"} {
		if raw, present := projected[field]; present && !validNonemptyPublicStringV1(raw) {
			return nil, false
		}
	}
	for _, field := range []string{"strictCompletion", "selfCheckRequired", "selfCheckCompleted"} {
		if raw, present := projected[field]; present {
			if _, ok := raw.(bool); !ok {
				return nil, false
			}
		}
	}
	if rawLedger, present := projected["evidenceLedger"]; present {
		ledger, ok := rawLedger.([]any)
		if !ok || len(ledger) > 500 {
			return nil, false
		}
		publicLedger := make([]any, 0, len(ledger))
		for _, rawEntry := range ledger {
			entry, _ := rawEntry.(map[string]any)
			if entry == nil {
				return nil, false
			}
			publicEntry := selectPublicFields(entry, []string{
				"id", "turnId", "toolCallId", "requirementId", "step", "evidence", "summary", "createdAt",
			})
			if !validNonemptyPublicStringV1(publicEntry["id"]) ||
				!validTrimmedPublicStringV1(publicEntry, "step", 1, 1_000, true) ||
				!validPublicStringV1(publicEntry, "createdAt", true) {
				return nil, false
			}
			for _, field := range []string{"turnId", "toolCallId", "requirementId"} {
				if raw, present := publicEntry[field]; present && !validNonemptyPublicStringV1(raw) {
					return nil, false
				}
			}
			if _, present := publicEntry["summary"]; present && !validTrimmedPublicStringV1(publicEntry, "summary", 1, 2_000, true) {
				return nil, false
			}
			evidence, ok := publicEntry["evidence"].([]any)
			if !ok || len(evidence) < 1 || len(evidence) > 20 {
				return nil, false
			}
			for index, rawEvidence := range evidence {
				text, ok := trimmedPublicStringV1(rawEvidence, 1, 2_000)
				if !ok {
					return nil, false
				}
				evidence[index] = text
			}
			publicEntry["evidence"] = evidence
			publicLedger = append(publicLedger, publicEntry)
		}
		projected["evidenceLedger"] = publicLedger
	}
	return projected, true
}

func validPublicStringV1(record map[string]any, field string, required bool) bool {
	value, present := record[field]
	if !present {
		return !required
	}
	_, ok := value.(string)
	return ok
}

func validNonemptyPublicStringV1(value any) bool {
	text, ok := value.(string)
	return ok && text != ""
}

func validTrimmedPublicStringV1(record map[string]any, field string, minimum, maximum int, required bool) bool {
	value, present := record[field]
	if !present {
		return !required
	}
	text, ok := trimmedPublicStringV1(value, minimum, maximum)
	if !ok {
		return false
	}
	record[field] = text
	return true
}

func trimmedPublicStringV1(value any, minimum, maximum int) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	length := len(utf16.Encode([]rune(text)))
	return text, length >= minimum && length <= maximum
}

func validPublicIntegerV1(value any, positive bool) bool {
	number, ok := contracts.NumericSeq(value)
	if !ok {
		return false
	}
	if positive {
		return number > 0
	}
	return number >= 0
}

func ordinaryPublicHistoryItemBlockerV1(item map[string]any) string {
	if item == nil {
		return "item_invalid"
	}
	if ordinaryChildLifecycleItem(item) {
		return "child_lifecycle_projection_invalid"
	}
	kind := strings.TrimSpace(contracts.StringField(item, "kind"))
	switch kind {
	case "tool_call":
		projected := domaintoolcall.PublicToolCallItemRecordV1(item)
		if strings.TrimSpace(contracts.StringField(projected, "id")) == "" {
			return "tool_call_identity_invalid"
		}
		return "tool_call_lifecycle_invalid"
	case "tool_result":
		projected := projectOrdinaryPublicToolResultItem(item)
		if strings.TrimSpace(contracts.StringField(projected, "id")) == "" {
			return "tool_result_identity_invalid"
		}
		if !ordinaryToolResultLifecycleConsistentV1(projected) {
			return "tool_result_lifecycle_inconsistent"
		}
		return "tool_result_lifecycle_invalid"
	default:
		if kind == "" {
			return "item_kind_missing"
		}
		return "item_kind_or_status_unsupported"
	}
}

func projectOrdinaryPublicHistoryItemV1(item map[string]any) (map[string]any, bool) {
	if item == nil {
		return nil, false
	}
	if ordinaryChildLifecycleItem(item) {
		return projectOrdinaryChildLifecycleItem(item), true
	}
	kind := strings.TrimSpace(contracts.StringField(item, "kind"))
	switch kind {
	case "tool_call":
		projected := domaintoolcall.PublicToolCallItemRecordV1(item)
		if strings.TrimSpace(contracts.StringField(projected, "id")) == "" {
			return nil, false
		}
		if withheld, _ := projected["lifecycleStatusWithheld"].(bool); withheld {
			return nil, false
		}
		return projected, true
	case "tool_result":
		projected := projectOrdinaryPublicToolResultItem(item)
		if strings.TrimSpace(contracts.StringField(projected, "id")) == "" ||
			!ordinaryToolResultLifecycleConsistentV1(projected) {
			return nil, false
		}
		if withheld, _ := projected["lifecycleStatusWithheld"].(bool); withheld {
			return nil, false
		}
		return projected, true
	}
	status := strings.TrimSpace(contracts.StringField(item, "status"))
	if status != "" {
		projected, ok := projectOrdinaryPublicEventItem(item)
		if ok && strings.TrimSpace(contracts.StringField(item, "role")) == "" {
			delete(projected, "role")
		}
		return projected, ok
	}
	placeholder := map[string]string{
		"user_message": "completed", "assistant_text": "completed", "approval": "pending", "user_input": "pending",
		"compaction": "completed", "review": "completed", "error": "failed",
	}[kind]
	if placeholder == "" {
		return nil, false
	}
	legacy := contracts.CloneMap(item)
	legacy["status"] = placeholder
	projected, ok := projectOrdinaryPublicEventItem(legacy)
	if !ok {
		return nil, false
	}
	delete(projected, "status")
	if strings.TrimSpace(contracts.StringField(item, "role")) == "" {
		delete(projected, "role")
	}
	return projected, true
}

func projectOrdinaryPublicToolResultItem(item map[string]any) map[string]any {
	return domaintoolresult.PublicToolResultItemRecordV1(item)
}

func caseThreadKnown(authority CaseThreadAuthority, threadID string) bool {
	return authority != nil && authority.IsCaseThread(strings.TrimSpace(threadID))
}

func samePublicEpoch(current, accepted domainsecurity.TurnSecurityContext) bool {
	return current.ThreadID == accepted.ThreadID && current.WorkspaceRealPath == accepted.WorkspaceRealPath &&
		current.TenantID == accepted.TenantID && current.UserID == accepted.UserID && current.CaseID == accepted.CaseID &&
		current.CaseBindingHash == accepted.CaseBindingHash && current.DatasetSnapshotID == accepted.DatasetSnapshotID &&
		current.SourceManifestHash == accepted.SourceManifestHash && current.ContextEpoch == accepted.ContextEpoch
}

func projectSafeBoundaryAssistant(
	threadID, turnID string,
	turn, item map[string]any,
	trusted *domainevidence.PrivateAcceptedFinalRecord,
	trustedItem map[string]any,
	trustedPublicView map[string]any,
) (map[string]any, bool) {
	if contracts.StringField(item, "kind") != "assistant_text" {
		return nil, false
	}
	turnRecord, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	itemRecord, itemErr := domainevidence.ParseAcceptedFinalRecord(item["acceptedFinal"])
	securityContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	text, textOK := item["text"].(string)
	expectedStatus, statusOK := domainevidence.FinalAnswerTerminalStatus(turnRecord.TerminalReason)
	safeText := safeBoundaryText(turnRecord.Variant, text)
	if trusted != nil {
		rendered, renderErr := domainevidence.RenderFinalAnswer(trusted.Envelope)
		safeText = renderErr == nil && rendered == text && rendered == trusted.RenderedText
	}
	if err != nil || itemErr != nil || contextErr != nil || !textOK || !statusOK ||
		turnRecord.RecordDigest != itemRecord.RecordDigest || turnRecord.ThreadID != threadID || turnRecord.TurnID != turnID ||
		securityContext.ThreadID != threadID || securityContext.TurnID != turnID || securityContext.ContextDigest != turnRecord.ContextDigest ||
		securityContext.ContextEpoch != turnRecord.ContextEpoch || securityContext.DatasetSnapshotID != turnRecord.DatasetSnapshotID ||
		contracts.StringField(turn, "status") != expectedStatus || domainsecurity.SHA256Hex([]byte(text)) != turnRecord.RenderedTextSHA256 ||
		contracts.StringField(item, "threadId") != threadID || contracts.StringField(item, "turnId") != turnID || !safeText {
		return nil, false
	}
	if trusted != nil && (!reflect.DeepEqual(turnRecord, trusted.AcceptedFinal) || !reflect.DeepEqual(itemRecord, trusted.AcceptedFinal) ||
		!reflect.DeepEqual(securityContext, trusted.SecurityContext) || text != trusted.RenderedText || !samePublicJSON(item, trustedItem)) {
		return nil, false
	}
	projectedSource := item
	if trustedItem != nil {
		projectedSource = trustedItem
	}
	projected := selectPublicFields(projectedSource, []string{
		"id", "turnId", "threadId", "status", "createdAt", "finishedAt", "kind", "text",
	})
	projected["kind"] = "assistant_text"
	projected["role"] = "assistant"
	projected["threadId"] = threadID
	projected["turnId"] = turnID
	if trusted != nil && trustedPublicView != nil {
		projected["acceptedFinalView"] = contracts.CloneValue(trustedPublicView)
	}
	return projected, true
}

func trustedAcceptedFinalTerminalErrorItemV1(
	trusted domainevidence.PrivateAcceptedFinalRecord,
	plan appturn.AcceptedFinalPublicationPlan,
) (map[string]any, bool) {
	if len(plan.TurnItems) != 2 {
		return nil, false
	}
	item := plan.TurnItems[1]
	projection, ok := domainterminal.FailureProjectionV1(trusted.AcceptedFinal.TerminalReason)
	if !ok ||
		contracts.StringField(item, "kind") != "error" ||
		contracts.StringField(item, "role") != "system" ||
		contracts.StringField(item, "threadId") != trusted.AcceptedFinal.ThreadID ||
		contracts.StringField(item, "turnId") != trusted.AcceptedFinal.TurnID ||
		contracts.StringField(item, "status") != projection.Status ||
		contracts.StringField(item, "code") != projection.Code ||
		contracts.StringField(item, "message") != projection.Message ||
		contracts.StringField(item, "severity") != projection.Severity ||
		contracts.StringField(item, "acceptedFinalDigest") != trusted.AcceptedFinal.RecordDigest ||
		strings.TrimSpace(contracts.StringField(item, "id")) == "" ||
		domainevent.ValidatePublicRecord(item) != nil {
		return nil, false
	}
	return contracts.CloneMap(item), true
}

func samePublicJSON(left, right any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftBody) == string(rightBody)
}

func sameContextEpochSnapshotV1(value map[string]any, expected domaincontextepoch.Snapshot) bool {
	body, err := json.Marshal(value)
	if err != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var parsed domaincontextepoch.Snapshot
	if err := decoder.Decode(&parsed); err != nil || domaincontextepoch.ValidateSnapshot(parsed) != nil ||
		!reflect.DeepEqual(parsed, expected) {
		return false
	}
	var normalized any
	if err := json.Unmarshal(body, &normalized); err != nil {
		return false
	}
	return samePublicJSON(normalized, contextepochapp.PublicSnapshot(expected))
}

func publicTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "aborted":
		return true
	default:
		return false
	}
}

func safeBoundaryText(variant domainevidence.FinalAnswerVariant, text string) bool {
	switch variant {
	case domainevidence.SourceUnavailableAnswer:
		return text == domainevidence.CaseSourceUnavailableText
	case domainevidence.NeedsEvidenceAnswer:
		return text == domainevidence.CaseUnverifiedText || text == domainevidence.CaseReportUnavailableText
	case domainevidence.EvidenceBackedAnswer, domainevidence.PartialEvidenceAnswer, domainevidence.VerifiedNoHitAnswer, domainevidence.GeneralGuidanceAnswer:
		return strings.TrimSpace(text) != ""
	default:
		return false
	}
}

func selectPublicFields(record map[string]any, fields []string) map[string]any {
	selected := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := record[field]; ok {
			selected[field] = contracts.CloneValue(value)
		}
	}
	return selected
}
