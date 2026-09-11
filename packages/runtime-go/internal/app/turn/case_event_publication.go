package turn

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

var caseEventMetadataFields = []string{
	"kind", "threadId", "turnId", "itemId", "status", "model", "providerId", "approvalPolicy", "sandboxMode",
	"attachmentIds", "workspaceCheckpointId", "mode", "disableUserInput", "maxModelSteps", "toolName", "callId",
	"toolKind", "approvalId", "inputId", "childRunId", "usageSource", "usageFinalStatus", "terminalReason", "code",
	"severity", "discard", "cancelled", "cancelledPendingGates", "stage", "state", "seq", "timestamp",
	"continuationReceiptId",
}

func ValidateCaseEventPublication(thread map[string]any, event map[string]any) error {
	turnID := strings.TrimSpace(stringField(event, "turnId"))
	turn, found := securityTurnByID(thread, turnID)
	if !found || turn["securityContext"] == nil {
		caseBound, err := threadHasCaseSecurityContext(thread)
		if err != nil {
			return err
		}
		if caseBound && caseDraftEventKind(event) {
			return errors.New("unknown-turn case assistant drafts cannot enter the durable event stream")
		}
		return nil
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		return errors.New("case event has an invalid frozen security context")
	}
	if !domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext) {
		return nil
	}
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil {
		return errors.New("case event lacks current V2 publication authority")
	}
	switch stringField(event, "kind") {
	case "assistant_text_delta", "assistant_reasoning_delta":
		return errors.New("case assistant drafts cannot enter the durable event stream")
	case "item_completed":
		item, _ := event["item"].(map[string]any)
		if stringField(item, "kind") == "assistant_text" {
			return validateAcceptedFinalEventItem(turn, item)
		}
	case "turn_completed", "turn_failed", "turn_aborted":
		return validateAcceptedFinalTerminalEvent(turn, event)
	}
	return nil
}

// SanitizeGenericCaseEventPublication is the only projection for generic
// single-event sinks. Accepted-final and ordinary-terminal publication
// markers belong to their dedicated atomic bundle paths and are rejected
// before case/privacy projection.
func SanitizeGenericCaseEventPublication(thread map[string]any, event map[string]any) (map[string]any, error) {
	if err := ValidateGenericTerminalEventPathForThreadV1(thread, event); err != nil {
		return nil, err
	}
	return SanitizeCaseEventPublication(thread, event)
}

// SanitizeCaseEventPublication applies a positive public-event projection to
// every case-sensitive thread event before it is persisted or published.
// Manifest-owned final events remain byte-stable for deterministic replay;
// all pre-final tool/control events are reduced to non-factual metadata.
func SanitizeCaseEventPublication(thread map[string]any, event map[string]any) (map[string]any, error) {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil {
		return nil, err
	}
	if !caseSensitive {
		if projected, handled, providerErr := ProjectProviderErrorPipelineStageV1(event); handled {
			if providerErr != nil {
				return nil, providerErr
			}
			return validateCaseEventOrdinaryProjection(projected)
		}
		projected, _ := domainevent.ProjectPublicValuePreservingClosedPlanDigestV1(event)
		record, ok := projected.(map[string]any)
		if !ok || record == nil {
			return nil, errors.New("ordinary event projection is unavailable")
		}
		return validateCaseEventOrdinaryProjection(record)
	}
	if err := ValidateCaseEventPublication(thread, event); err != nil {
		return nil, err
	}
	turnID := strings.TrimSpace(stringField(event, "turnId"))
	turn, _ := securityTurnByID(thread, turnID)
	if strings.TrimSpace(stringField(event, "publicationCommitId")) != "" {
		if err := validateManifestOwnedCaseEvent(turn, event); err != nil {
			return nil, err
		}
		return validateCaseEventOrdinaryProjection(contracts.CloneMap(event))
	}
	kind := strings.TrimSpace(stringField(event, "kind"))
	if kind == "assistant_text_delta" || kind == "assistant_reasoning_delta" || kind == "agent_reasoning" {
		return nil, errors.New("case assistant drafts cannot enter the durable event stream")
	}
	if kind == "turn_completed" || kind == "turn_failed" || kind == "turn_aborted" {
		return nil, errors.New("case terminal event is not owned by an accepted-final publication manifest")
	}
	if compaction, handled, compactionErr := sanitizeCaseCompactionEvent(thread, turn, event); handled {
		if compactionErr != nil {
			return nil, compactionErr
		}
		return validateCaseEventOrdinaryProjection(compaction)
	}
	if kind == "checkpoint_captured" {
		checkpointTurnID, turnOK := caseCheckpointAuditExactString(event, "turnId")
		if !turnOK {
			return nil, errInvalidCaseCheckpointAudit
		}
		if _, found := securityTurnByID(thread, checkpointTurnID); !found {
			return nil, errInvalidCaseCheckpointAudit
		}
	}
	threadID, threadOK := caseCheckpointAuditExactString(thread, "id")
	if checkpointAudit, handled, checkpointErr := sanitizeCaseCheckpointAuditEvent(threadID, event); handled {
		if !threadOK {
			return nil, errInvalidCaseCheckpointAudit
		}
		if checkpointErr != nil {
			return nil, checkpointErr
		}
		return validateCaseEventOrdinaryProjection(checkpointAudit)
	}
	if providerError, handled, providerErr := ProjectProviderErrorPipelineStageV1(event); handled {
		if providerErr != nil {
			return nil, providerErr
		}
		return validateCaseEventOrdinaryProjection(providerError)
	}
	projected := selectCaseEventFields(event, caseEventMetadataFields)
	if kind == "item_created" || kind == "item_completed" || kind == "tool_call_finished" {
		item, _ := event["item"].(map[string]any)
		switch strings.TrimSpace(stringField(item, "kind")) {
		case "user_message":
			if strings.TrimSpace(stringField(item, "role")) != "user" || stringField(item, "threadId") != stringField(event, "threadId") ||
				stringField(item, "turnId") != turnID || !caseTurnContainsExactItem(turn, item) {
				return nil, errors.New("case user-message event is detached from the host-created turn item")
			}
			projected["item"] = selectCaseEventFields(item, []string{
				"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "text", "displayText",
				"delivery", "clientUserMessageId", "attachmentIds", "fileReferences", "workspaceCheckpointId",
			})
		case "assistant_text":
			return nil, errors.New("case assistant item is not owned by an accepted-final publication manifest")
		default:
			projected["item"] = selectCaseEventFields(item, []string{
				"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "toolName", "callId",
				"toolKind", "isError", "approvalId", "inputId", "code", "severity", "continuationReceiptId",
			})
		}
	}
	return validateCaseEventOrdinaryProjection(projected)
}

func sanitizeCaseCompactionEvent(
	thread map[string]any,
	turn map[string]any,
	event map[string]any,
) (map[string]any, bool, error) {
	kind := strings.TrimSpace(stringField(event, "kind"))
	if kind != "compaction_started" && kind != "compaction_completed" {
		return nil, false, nil
	}
	threadID := strings.TrimSpace(stringField(thread, "id"))
	turnID := strings.TrimSpace(stringField(event, "turnId"))
	itemID := strings.TrimSpace(stringField(event, "itemId"))
	items, itemsOK := turn["items"].([]any)
	if threadID == "" || stringField(event, "threadId") != threadID ||
		turnID == "" || stringField(turn, "id") != turnID || itemID == "" ||
		strings.TrimSpace(stringField(turn, "caseHistoryProjection")) != "compaction_authority_v1" ||
		!itemsOK || len(items) != 1 {
		return nil, true, errors.New("case compaction event is detached from its durable target")
	}
	item, _ := items[0].(map[string]any)
	marker, markerErr := ProjectCaseCompactionPublicMarker(item)
	continuation, continuationErr := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
	binding, bindingErr := ParseCaseCompactionOperationBindingV1(item["caseCompactionBinding"])
	digest, digestErr := CaseCompactionOperationDigestV1(binding)
	frozen, frozenErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	snapshot, _ := turn["contextEpochSnapshot"].(map[string]any)
	issuedAt, issuedAtErr := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
	expectedStamp := ""
	if issuedAtErr == nil {
		expectedStamp = strconv.FormatInt(issuedAt.UnixNano(), 10)
	}
	safeThreadID := contracts.SafeRecordID(threadID)
	expectedTurnID := fmt.Sprintf("turn_%s_compaction_%s", safeThreadID, binding.OperationStamp)
	expectedItemID := fmt.Sprintf("compaction_%s_%s", safeThreadID, binding.OperationStamp)
	if markerErr != nil || continuationErr != nil || bindingErr != nil || digestErr != nil || frozenErr != nil ||
		issuedAtErr != nil || binding.OperationStamp != expectedStamp ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(frozen) != nil ||
		frozen.ThreadID != threadID || frozen.TurnID != turnID || turnID != expectedTurnID || itemID != expectedItemID ||
		stringField(item, "threadId") != threadID || stringField(item, "turnId") != turnID ||
		stringField(item, "id") != itemID || stringField(item, "createdAt") != frozen.IssuedAt ||
		stringField(item, "finishedAt") != frozen.IssuedAt ||
		binding.ThreadIDHash != domainsecurity.SHA256Hex([]byte(threadID)) ||
		binding.SourceContextDigest != strings.TrimSpace(stringField(item, "sourceContextDigest")) ||
		binding.ContinuationDigest != continuation.StateDigest || digest != stringField(item, "sourceDigest") ||
		snapshot == nil || stringField(snapshot, "threadId") != threadID ||
		caseCompactionNumeric(snapshot["epoch"]) != int(frozen.ContextEpoch) ||
		stringField(snapshot, "recoveryDigest") != digest {
		return nil, true, errors.Join(
			errors.New("case compaction event authority is invalid"),
			markerErr, continuationErr, bindingErr, digestErr, frozenErr,
		)
	}
	projected := selectCaseEventFields(event, []string{
		"kind", "threadId", "turnId", "itemId", "seq", "timestamp",
	})
	projected["auto"] = contracts.CloneValue(marker["auto"])
	if kind == "compaction_started" {
		if eventAuto, present := event["auto"]; present && !reflect.DeepEqual(eventAuto, marker["auto"]) {
			return nil, true, errors.New("case compaction start event mode is invalid")
		}
		return projected, true, nil
	}
	for _, field := range []string{
		"summary", "auto", "pinnedConstraints", "sourceDigest", "digestMarker", "sourceItemIds",
	} {
		matches := reflect.DeepEqual(event[field], marker[field])
		if !matches {
			return nil, true, fmt.Errorf(
				"case compaction completed event field is detached from its public marker: %s (%T/%T)",
				field, event[field], marker[field],
			)
		}
		projected[field] = contracts.CloneValue(marker[field])
	}
	proof := strings.TrimSpace(stringField(event, "reasoningExclusionProof"))
	if caseCompactionNumeric(event["schemaVersion"]) != 2 || event["reasoningExcluded"] != true ||
		(proof != stringField(item, "reasoningExclusionProof") &&
			proof != stringField(marker, "reasoningExclusionProof")) {
		return nil, true, errors.New("case compaction completed event reasoning authority is invalid")
	}
	projected["schemaVersion"] = float64(2)
	projected["reasoningExcluded"] = true
	projected["reasoningExclusionProof"] = marker["reasoningExclusionProof"]
	return projected, true, nil
}

// ValidateExactCaseEventProjection protects raw/conformance append paths that
// cannot safely rewrite a signed or caller-supplied line. Ordinary events use
// the regular sanitizer and do not require byte-shape equality here.
func ValidateExactCaseEventProjection(thread, event map[string]any) error {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil || !caseSensitive {
		return err
	}
	projected, err := SanitizeCaseEventPublication(thread, event)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(projected, event) {
		return errors.New("raw case event is not an exact ordinary projection")
	}
	return nil
}

func validateCaseEventOrdinaryProjection(event map[string]any) (map[string]any, error) {
	if err := domainevent.ValidatePublicRecord(event); err != nil {
		return nil, errors.Join(errors.New("case event ordinary projection contains restricted runtime content"), err)
	}
	projected, changed := domainevent.ProjectPublicValuePreservingClosedPlanDigestV1(event)
	if changed || !reflect.DeepEqual(projected, event) {
		return nil, errors.New("case event ordinary projection contains restricted PII")
	}
	return event, nil
}

func validateManifestOwnedCaseEvent(turn, event map[string]any) error {
	record, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	commitID := strings.TrimSpace(stringField(event, "publicationCommitId"))
	slot := strings.TrimSpace(stringField(event, "publicationSlot"))
	expectedEventID := domainsecurity.SHA256Hex([]byte(acceptedFinalPublicationEventDomain + commitID + "\x00" + slot))
	if err != nil || validateAcceptedFinalTurnContext(turn, record) != nil || commitID != record.RecordDigest || stringField(event, "acceptedFinalDigest") != record.RecordDigest ||
		stringField(event, "publicationEventId") != expectedEventID || stringField(event, "publicationPayloadDigest") == "" ||
		AcceptedFinalPublicationPayloadDigest(event) != stringField(event, "publicationPayloadDigest") {
		return errors.New("case event is detached from the accepted-final publication manifest")
	}
	switch slot {
	case "assistant-final":
		item, _ := event["item"].(map[string]any)
		return validateAcceptedFinalEventItem(turn, item)
	case "terminal-error-item":
		item, _ := event["item"].(map[string]any)
		if stringField(event, "kind") != "item_completed" || stringField(item, "kind") != "error" ||
			stringField(item, "acceptedFinalDigest") != record.RecordDigest || !caseTurnContainsExactItem(turn, item) {
			return errors.New("case terminal item is detached from committed public authority")
		}
		return nil
	case "usage":
		if stringField(event, "kind") != "usage" {
			return errors.New("case accepted-final usage event kind is invalid")
		}
		return nil
	case "terminal":
		return validateAcceptedFinalTerminalEvent(turn, event)
	default:
		return errors.New("case accepted-final publication slot is unknown")
	}
}

func caseTurnContainsExactItem(turn, expected map[string]any) bool {
	expectedID := strings.TrimSpace(stringField(expected, "id"))
	expectedBody, _ := json.Marshal(expected)
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(stringField(item, "id")) != expectedID {
			continue
		}
		body, _ := json.Marshal(item)
		return string(body) == string(expectedBody)
	}
	return false
}

func selectCaseEventFields(record map[string]any, fields []string) map[string]any {
	selected := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := record[field]; ok {
			selected[field] = contracts.CloneValue(value)
		}
	}
	return selected
}

// ProjectProviderErrorPipelineStageV1 is the closed projection for a provider
// failure pipeline event. It intentionally recognizes only the exact
// pipeline_stage/provider_error pair and retains an allowlisted reason code
// plus optional closed provider or bounded recovery metadata. Provider
// messages, bodies, endpoints, credentials, and request/response payloads
// never enter the returned record.
func ProjectProviderErrorPipelineStageV1(event map[string]any) (map[string]any, bool, error) {
	rawKind, kindOK := event["kind"].(string)
	rawStage, stageOK := event["stage"].(string)
	kind := strings.TrimSpace(rawKind)
	stage := strings.TrimSpace(rawStage)
	if !kindOK || !stageOK || kind != "pipeline_stage" || stage != "provider_error" {
		return nil, false, nil
	}
	if rawKind != kind || rawStage != stage {
		return nil, true, errors.New("provider error pipeline event kind or stage is not canonical")
	}
	rawThreadID, threadOK := event["threadId"].(string)
	rawTurnID, turnOK := event["turnId"].(string)
	threadID := strings.TrimSpace(rawThreadID)
	turnID := strings.TrimSpace(rawTurnID)
	if !threadOK || !turnOK || rawThreadID != threadID || rawTurnID != turnID ||
		threadID == "" || contracts.SafeRecordID(threadID) != threadID ||
		turnID == "" || contracts.SafeRecordID(turnID) != turnID {
		return nil, true, errors.New("provider error pipeline event identity is invalid")
	}
	rawDetails, detailsPresent := event["details"]
	if !detailsPresent {
		return nil, true, errors.New("provider error pipeline event details are missing")
	}
	details, ok := rawDetails.(map[string]any)
	if !ok || details == nil {
		return nil, true, errors.New("provider error pipeline event details are invalid")
	}
	rawReasonCode, reasonPresent := details["reasonCode"]
	if !reasonPresent {
		return nil, true, errors.New("provider error pipeline event reason code is missing")
	}
	reasonCode, ok := rawReasonCode.(string)
	canonicalReasonCode := domainfailure.New(reasonCode, nil).Code()
	if !ok || reasonCode == "" || strings.TrimSpace(reasonCode) != reasonCode || canonicalReasonCode != reasonCode {
		return nil, true, errors.New("provider error pipeline event reason code is not canonical")
	}
	projectedDetails, detailsErr := projectProviderErrorPipelineDetailsV1(details, reasonCode)
	if detailsErr != nil {
		return nil, true, detailsErr
	}
	projected := map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
		"stage": "provider_error", "label": "Provider stream failed",
		"details": projectedDetails,
	}
	rawSeq, seqPresent := event["seq"]
	rawTimestamp, timestampPresent := event["timestamp"]
	if seqPresent != timestampPresent {
		return nil, true, errors.New("provider error pipeline transport identity is incomplete")
	}
	if seqPresent {
		seq, seqOK := contracts.NumericSeq(rawSeq)
		if !seqOK || seq <= 0 {
			return nil, true, errors.New("provider error pipeline event sequence is invalid")
		}
		projected["seq"] = float64(seq)
	}
	if timestampPresent {
		timestamp, timestampOK := rawTimestamp.(string)
		if !timestampOK || strings.TrimSpace(timestamp) != timestamp || timestamp == "" {
			return nil, true, errors.New("provider error pipeline event timestamp is invalid")
		}
		if _, parseErr := time.Parse(time.RFC3339Nano, timestamp); parseErr != nil {
			return nil, true, errors.New("provider error pipeline event timestamp is invalid")
		}
		projected["timestamp"] = timestamp
	}
	return projected, true, nil
}

func projectProviderErrorPipelineDetailsV1(details map[string]any, reasonCode string) (map[string]any, error) {
	projected := map[string]any{"reasonCode": reasonCode}
	providerValue, hasProvider := details["providerError"]
	recoveryKeys := []string{
		"visibleRecovery", "recoveryKind", "recoveryAttempt", "maxRecoveryAttempts", "recoveryExhausted",
	}
	hasRecovery := false
	for _, key := range recoveryKeys {
		if _, present := details[key]; present {
			hasRecovery = true
			break
		}
	}
	if hasProvider && hasRecovery {
		return nil, errors.New("provider error pipeline details mix provider and recovery diagnostics")
	}
	if hasProvider {
		provider, ok := providerValue.(map[string]any)
		if !ok || provider == nil || !domainfailure.ValidatePublicDetails(provider) {
			return nil, errors.New("provider error pipeline provider diagnostic is invalid")
		}
		allowed := map[string]bool{
			"kind": true, "status": true, "retryAfterMs": true, "retryable": true,
			"hasApiKey": true, "authStatus": true, "endpointFormat": true, "attempt": true,
			"failureStage": true, "dispatchState": true,
		}
		for key := range provider {
			if !allowed[key] {
				return nil, errors.New("provider error pipeline provider diagnostic is open")
			}
		}
		_, hasKind := provider["kind"]
		_, hasFailureStage := provider["failureStage"]
		_, hasDispatchState := provider["dispatchState"]
		if !hasKind && (!hasFailureStage || !hasDispatchState) {
			return nil, errors.New("provider error pipeline provider diagnostic attribution is missing")
		}
		projected["providerError"] = contracts.CloneMap(provider)
	}
	if hasRecovery {
		attempt, attemptOK := contracts.NumericSeq(details["recoveryAttempt"])
		maxAttempts, maxAttemptsOK := contracts.NumericSeq(details["maxRecoveryAttempts"])
		if reasonCode != domainfailure.CodeProviderEmptyFinal ||
			details["visibleRecovery"] != true || details["recoveryKind"] != "empty_final" ||
			details["recoveryExhausted"] != true || !attemptOK || !maxAttemptsOK ||
			attempt < 0 || maxAttempts < 0 || attempt > maxAttempts || maxAttempts > 1_000_000 {
			return nil, errors.New("provider error pipeline recovery diagnostic is invalid")
		}
		projected["visibleRecovery"] = true
		projected["recoveryKind"] = "empty_final"
		projected["recoveryAttempt"] = float64(attempt)
		projected["maxRecoveryAttempts"] = float64(maxAttempts)
		projected["recoveryExhausted"] = true
	}
	return projected, nil
}

func caseDraftEventKind(event map[string]any) bool {
	switch stringField(event, "kind") {
	case "assistant_text_delta", "assistant_reasoning_delta", "agent_reasoning":
		return true
	case "item_completed":
		item, _ := event["item"].(map[string]any)
		return stringField(item, "kind") == "assistant_text" || stringField(item, "kind") == "assistant_reasoning"
	default:
		return false
	}
}

func threadHasCaseSecurityContext(thread map[string]any) (bool, error) {
	return domainsecurity.ClassifyCaseSensitiveThread(thread)
}

func validateAcceptedFinalEventItem(turn, item map[string]any) error {
	turnRecord, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	if err != nil || validateAcceptedFinalTurnContext(turn, turnRecord) != nil {
		return errors.New("case assistant event has no committed accepted final")
	}
	allowed := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true,
		"createdAt": true, "finishedAt": true, "kind": true, "text": true, "acceptedFinalView": true,
	}
	for key := range item {
		if !allowed[key] {
			return errors.New("case assistant event contains non-public authority")
		}
	}
	view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(item["acceptedFinalView"])
	expectedView, expectedErr := domainevidence.NewAcceptedFinalPublicViewFromRecordV3(turnRecord)
	text, _ := item["text"].(string)
	if err != nil || expectedErr != nil || item["acceptedFinal"] != nil || !reflect.DeepEqual(view, expectedView) ||
		domainsecurity.SHA256Hex([]byte(text)) != turnRecord.RenderedTextSHA256 {
		return errors.New("case assistant event does not match committed authority")
	}
	return nil
}

func validateAcceptedFinalPrivateItem(turn, item map[string]any) error {
	turnRecord, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	if err != nil || validateAcceptedFinalTurnContext(turn, turnRecord) != nil {
		return errors.New("case assistant item has no committed accepted final")
	}
	itemRecord, err := domainevidence.ParseAcceptedFinalRecord(item["acceptedFinal"])
	text, _ := item["text"].(string)
	if err != nil || itemRecord.RecordDigest != turnRecord.RecordDigest ||
		domainsecurity.SHA256Hex([]byte(text)) != turnRecord.RenderedTextSHA256 {
		return errors.New("case assistant item does not match committed authority")
	}
	return nil
}

func validateAcceptedFinalTerminalEvent(turn, event map[string]any) error {
	record, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
	status, ok := domainevidence.FinalAnswerTerminalStatus(record.TerminalReason)
	if err != nil || validateAcceptedFinalTurnContext(turn, record) != nil || !ok || stringField(event, "kind") != "turn_"+status ||
		strings.TrimSpace(stringField(event, "acceptedFinalDigest")) != record.RecordDigest {
		return errors.New("case terminal event does not match committed authority")
	}
	return nil
}

func validateAcceptedFinalTurnContext(turn map[string]any, record domainevidence.AcceptedFinalRecord) error {
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil ||
		securityContext.ThreadID != record.ThreadID || securityContext.TurnID != record.TurnID ||
		securityContext.ContextDigest != record.ContextDigest || securityContext.ContextEpoch != record.ContextEpoch ||
		securityContext.DatasetSnapshotID != record.DatasetSnapshotID {
		return errors.New("accepted final event lacks current V2 case authority")
	}
	if domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Variant) &&
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return errors.New("fact-bearing accepted final event lacks current V2 case fact authority")
	}
	return nil
}
