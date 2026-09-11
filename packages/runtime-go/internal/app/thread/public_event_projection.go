package thread

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	"analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// ProjectPublicThreadEvent applies the same boundary-only case projection to
// SSE replay and live events as ProjectPublicThread applies to snapshots.
func ProjectPublicThreadEvent(routeThreadID string, thread, event map[string]any) (map[string]any, bool, error) {
	return NewTrustedPublicProjector(nil).ProjectEvent(routeThreadID, thread, event)
}

func projectPublicThreadEvent(routeThreadID string, thread, event map[string]any, trusted *gateprojection.TrustedFinalProjectionIndex, caseThreads CaseThreadAuthority, currentAuthority CurrentCaseThreadAuthorityValidator, primaryCAS finalauthorityport.AcceptedFinalCASReader) (map[string]any, bool, error) {
	routeThreadID = strings.TrimSpace(routeThreadID)
	threadID := strings.TrimSpace(contracts.StringField(thread, "id"))
	eventThreadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	if routeThreadID == "" || contracts.SafeRecordID(routeThreadID) != routeThreadID || threadID != routeThreadID || eventThreadID != routeThreadID {
		return nil, false, nil
	}
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil {
		return nil, false, err
	}
	indexedCase := trusted.ContainsThread(routeThreadID) || caseThreadKnown(caseThreads, routeThreadID)
	classifiedCase := caseSensitive
	caseSensitive = caseSensitive || indexedCase
	if !caseSensitive {
		governed, valid, validationErr := domainturnterminal.ValidateGeneralTerminalPublicEventV1(thread, event)
		if validationErr != nil {
			return nil, false, validationErr
		}
		if governed && !valid {
			return nil, false, nil
		}
		if !validOrdinaryCompactionCompletedEventV1(thread, event) {
			return nil, false, nil
		}
		projected, visible := projectOrdinaryPublicThreadEvent(event)
		if governed && visible && !projectGeneralTerminalOrdinaryResultV1(event, projected) {
			return nil, false, nil
		}
		if governed && visible && strings.TrimSpace(contracts.StringField(event, "terminalReason")) == "cancel" {
			metadata, metadataErr := appturn.ResolveCommittedGeneralInterruptMetadataV1(
				thread, contracts.StringField(event, "turnId"),
			)
			if metadataErr != nil {
				return nil, false, metadataErr
			}
			projected["discard"] = metadata.Discard
			projected["cancelled"] = metadata.Cancelled
			projected["cancelledPendingGates"] = metadata.CancelledPendingGates
		}
		return projected, visible, nil
	}
	if providerError, handled, providerErr := appturn.ProjectProviderErrorPipelineStageV1(event); handled {
		if providerErr != nil || !validProviderErrorPipelineStagePublicEventV1(providerError, true) {
			return nil, false, nil
		}
		projectedThread, projectionErr := projectPublicThreadWithAuthority(
			thread, trusted, caseThreads, currentAuthority, primaryCAS,
		)
		if projectionErr != nil {
			return nil, false, projectionErr
		}
		if !projectedThreadHasTurn(projectedThread, contracts.StringField(providerError, "turnId")) {
			return nil, false, nil
		}
		return providerError, true, nil
	}
	generalAuthorities, generalAuthorityErr := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if generalAuthorityErr != nil {
		return nil, false, generalAuthorityErr
	}
	if generalAuthority, generalTurn := generalAuthorities[strings.TrimSpace(contracts.StringField(event, "turnId"))]; generalTurn {
		governed, valid, validationErr := domainturnterminal.ValidateGeneralTerminalPublicEventV1(thread, event)
		if validationErr != nil {
			return nil, false, validationErr
		}
		if !generalAuthority.Terminal || !governed || !valid {
			return nil, false, nil
		}
		if _, projectionErr := projectPublicThreadWithAuthority(
			thread, trusted, caseThreads, currentAuthority, primaryCAS,
		); projectionErr != nil {
			return nil, false, projectionErr
		}
		projected, visible := projectOrdinaryPublicThreadEvent(event)
		if !visible || !projectGeneralTerminalOrdinaryResultV1(event, projected) {
			return nil, false, nil
		}
		if strings.TrimSpace(contracts.StringField(event, "terminalReason")) == "cancel" {
			metadata, metadataErr := appturn.ResolveCommittedGeneralInterruptMetadataV1(
				thread, contracts.StringField(event, "turnId"),
			)
			if metadataErr != nil {
				return nil, false, metadataErr
			}
			projected["discard"] = metadata.Discard
			projected["cancelled"] = metadata.Cancelled
			projected["cancelledPendingGates"] = metadata.CancelledPendingGates
		}
		return projected, true, nil
	}
	sanitizerThread := thread
	if indexedCase && !classifiedCase {
		sanitizerThread = contracts.CloneMap(thread)
		sanitizerThread["historyAuthority"] = CaseBoundaryOnlyHistoryAuthority
	}
	projectedEvent, err := appturn.SanitizeCaseEventPublication(sanitizerThread, event)
	if err != nil {
		return nil, false, nil
	}
	if trusted != nil && strings.TrimSpace(contracts.StringField(projectedEvent, "publicationCommitId")) != "" &&
		!trustedPublicationEvent(routeThreadID, projectedEvent, trusted) {
		return nil, false, nil
	}
	projectedThread, err := projectPublicThreadWithAuthority(thread, trusted, caseThreads, currentAuthority, primaryCAS)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(contracts.StringField(projectedEvent, "publicationCommitId")) != "" {
		slot := strings.TrimSpace(contracts.StringField(projectedEvent, "publicationSlot"))
		switch slot {
		case "assistant-final", "terminal-error-item":
			item, _ := projectedEvent["item"].(map[string]any)
			if _, ok := projectedThreadItem(projectedThread, contracts.StringField(projectedEvent, "turnId"), contracts.StringField(item, "id")); !ok {
				return nil, false, nil
			}
		case "usage", "terminal":
			if !projectedThreadHasAcceptedFinal(projectedThread, contracts.StringField(projectedEvent, "turnId")) {
				return nil, false, nil
			}
		default:
			return nil, false, nil
		}
		// The event was reconstructed from the exact committed plan and its
		// payload digest was recomputed below. Returning the full host event
		// keeps the transport batch byte-equivalent to the durable manifest.
		return projectedEvent, true, nil
	}
	kind := strings.TrimSpace(contracts.StringField(projectedEvent, "kind"))
	switch kind {
	case "turn_started":
		if !projectedThreadHasTurn(projectedThread, contracts.StringField(projectedEvent, "turnId")) {
			return nil, false, nil
		}
		return selectPublicFields(projectedEvent, []string{"kind", "threadId", "turnId", "status", "seq", "timestamp"}), true, nil
	case "item_created", "item_completed":
		item, _ := projectedEvent["item"].(map[string]any)
		visibleItem, ok := projectedThreadItem(projectedThread, contracts.StringField(projectedEvent, "turnId"), contracts.StringField(item, "id"))
		if !ok {
			return nil, false, nil
		}
		out := selectPublicFields(projectedEvent, []string{
			"kind", "threadId", "turnId", "itemId", "seq", "timestamp", "acceptedFinalDigest", "publicationCommitId",
			"publicationEventId", "publicationSlot", "publicationPayloadDigest",
		})
		out["item"] = visibleItem
		if strings.TrimSpace(contracts.StringField(out, "publicationCommitId")) != "" &&
			contracts.StringField(out, "publicationPayloadDigest") != appturn.AcceptedFinalPublicationPayloadDigest(out) {
			return nil, false, nil
		}
		return out, true, nil
	case "turn_completed", "turn_failed", "turn_aborted":
		if trusted != nil && !projectedThreadHasAcceptedFinal(projectedThread, contracts.StringField(projectedEvent, "turnId")) {
			return nil, false, nil
		}
		return selectPublicFields(projectedEvent, []string{
			"kind", "threadId", "turnId", "status", "seq", "timestamp", "acceptedFinalDigest", "terminalReason", "discard", "cancelled",
		}), true, nil
	case "thread_created", "thread_updated", "thread_archived":
		return selectPublicFields(projectedEvent, []string{"kind", "threadId", "status", "seq", "timestamp"}), true, nil
	case "checkpoint_captured", "checkpoint_rewind_rescue_created", "checkpoint_rewind_applied":
		checkpointAudit, ok := appturn.ProjectCaseCheckpointAuditEvent(routeThreadID, projectedEvent)
		if !ok {
			return nil, false, nil
		}
		return checkpointAudit, true, nil
	case "compaction_started":
		turnID := contracts.StringField(projectedEvent, "turnId")
		if _, ok := projectedCaseCompactionMarkerV1(projectedThread, turnID, contracts.StringField(projectedEvent, "itemId")); !ok {
			return nil, false, nil
		}
		return selectPublicFields(projectedEvent, []string{
			"kind", "threadId", "turnId", "seq", "timestamp",
		}), true, nil
	case "compaction_completed":
		turnID := contracts.StringField(projectedEvent, "turnId")
		itemID := contracts.StringField(projectedEvent, "itemId")
		marker, ok := projectedCaseCompactionMarkerV1(projectedThread, turnID, itemID)
		if !ok {
			return nil, false, nil
		}
		out := selectPublicFields(projectedEvent, []string{
			"kind", "threadId", "turnId", "seq", "timestamp",
		})
		for _, key := range []string{
			"summary", "auto", "pinnedConstraints", "sourceDigest", "digestMarker",
			"sourceItemIds", "reasoningExclusionProof",
		} {
			out[key] = contracts.CloneValue(marker[key])
		}
		out["schemaVersion"] = float64(2)
		out["reasoningExcluded"] = true
		return out, true, nil
	default:
		return nil, false, nil
	}
}

func projectedCaseCompactionMarkerV1(thread map[string]any, turnID, itemID string) (map[string]any, bool) {
	item, ok := projectedThreadItem(thread, strings.TrimSpace(turnID), strings.TrimSpace(itemID))
	return item, ok && validProjectedCaseCompactionItemV1(item)
}

func projectOrdinaryPublicThreadEvent(event map[string]any) (map[string]any, bool) {
	if domainevent.ContainsPrivateAcceptedFinalAuthority(event) {
		return nil, false
	}
	if providerError, handled, providerErr := appturn.ProjectProviderErrorPipelineStageV1(event); handled {
		if providerErr != nil || !validProviderErrorPipelineStagePublicEventV1(providerError, false) {
			return nil, false
		}
		return providerError, true
	}
	rawItem, _ := event["item"].(map[string]any)
	sanitizedValue, safe := domainevent.SanitizePublicValue(event, false)
	if !safe {
		return nil, false
	}
	sanitized, _ := sanitizedValue.(map[string]any)
	if sanitized == nil {
		return nil, false
	}
	event = sanitized
	kind := strings.TrimSpace(contracts.StringField(event, "kind"))
	itemEvent := ordinaryItemEventKind(kind)
	if itemEvent {
		item, _ := event["item"].(map[string]any)
		if !ordinaryItemEventIdentityValid(event, item, kind) || ordinaryEventCarriesPublicationAuthority(event, item) {
			return nil, false
		}
	}
	if item, _ := event["item"].(map[string]any); itemEvent && strings.TrimSpace(contracts.StringField(item, "kind")) == "tool_call" {
		projectedItem := domaintoolcall.PublicToolCallItemRecordV1(item)
		projectedID := strings.TrimSpace(contracts.StringField(projectedItem, "id"))
		if projectedID == "" || !ordinaryItemEventLifecycleValidV1(kind, projectedItem) {
			return nil, false
		}
		out := ordinaryEventFields(event)
		out["itemId"] = projectedID
		out["item"] = projectedItem
		return out, true
	}
	if item, _ := event["item"].(map[string]any); itemEvent && strings.TrimSpace(contracts.StringField(item, "kind")) == "tool_result" {
		if rawItem == nil {
			return nil, false
		}
		// The recursive sanitizer has already removed the private binding proof.
		// Re-project the raw durable item exactly once so case-source status cannot
		// survive a missing or rebound proof.
		projectedItem := projectOrdinaryPublicToolResultItem(rawItem)
		projectedID := strings.TrimSpace(contracts.StringField(projectedItem, "id"))
		if projectedID == "" || !ordinaryItemEventLifecycleValidV1(kind, projectedItem) || !ordinaryToolResultLifecycleConsistentV1(projectedItem) {
			return nil, false
		}
		out := ordinaryEventFields(event, "acceptedFinalDigest", "publicationCommitId", "publicationEventId", "publicationSlot", "publicationPayloadDigest")
		out["itemId"] = projectedID
		out["item"] = projectedItem
		return out, true
	}
	if item, _ := event["item"].(map[string]any); itemEvent && ordinaryChildLifecycleItem(item) {
		out := ordinaryEventFields(event)
		out["item"] = projectOrdinaryChildLifecycleItem(item)
		return out, true
	}
	child, _ := event["child"].(map[string]any)
	details, _ := event["details"].(map[string]any)
	if (kind == "pipeline_stage" || kind == "tool_progress" || itemEvent) && (child != nil || ordinaryChildLifecycleMetadata(details)) {
		out := ordinaryEventFields(event, "stage", "callId", "toolName", "status")
		if kind == "tool_progress" {
			out["status"] = publicToolProgressStatusV1(contracts.StringField(event, "status"))
		}
		if kind == "pipeline_stage" {
			projectedStage := publicChildLifecycleStageV1(contracts.StringField(event, "stage"))
			out["stage"] = projectedStage
			if publicBasePipelineStageV1(projectedStage) {
				delete(out, "status")
			} else if _, present := event["status"]; present {
				out["status"] = publicChildStageStatusV1(projectedStage, contracts.StringField(event, "status"))
			}
		}
		out["message"] = "child output withheld"
		if child != nil {
			out["child"] = projectOrdinaryChildMetadata(child)
		}
		if ordinaryChildLifecycleMetadata(details) {
			out["details"] = projectOrdinaryChildMetadata(details)
		}
		return out, true
	}

	switch kind {
	case "thread_created", "thread_updated":
		status, valid := publicThreadLifecycleEventStatusV1(kind, contracts.StringField(event, "status"))
		if !valid {
			return nil, false
		}
		out := ordinaryEventFields(event, "status")
		out["status"] = status
		return out, true
	case "thread_rewound":
		return ordinaryEventFields(event, "rewindTurnId", "removedTurns", "remainingTurns", "removedTurnIds"), true
	case "turn_started", "turn_completed", "turn_failed", "turn_aborted", "turn_steered":
		status, valid := publicTurnLifecycleEventStatusV1(kind, event)
		if !valid {
			return nil, false
		}
		out := ordinaryEventFields(event,
			"status", "text", "clientUserMessageId", "admittedSeq", "message", "code", "details", "severity",
			"acceptedFinalDigest", "terminalReason", "discard", "cancelled", "cancelledPendingGates", "model", "providerId", "approvalPolicy",
			"sandboxMode", "mode", "disableUserInput", "maxModelSteps", "workspaceCheckpointId",
		)
		if status == "" {
			delete(out, "status")
		} else {
			out["status"] = status
		}
		retainClosedFailureDetailsV1(out)
		return out, true
	case "item_created", "item_updated", "item_completed", "tool_call_started", "tool_call_finished":
		item, _ := event["item"].(map[string]any)
		if domainevent.IsLegacyAssistantDraftItem(item) {
			return nil, false
		}
		projectedItem, ok := projectOrdinaryPublicEventItem(item)
		if !ok || !ordinaryItemEventLifecycleValidV1(kind, projectedItem) {
			return nil, false
		}
		out := ordinaryEventFields(event, "acceptedFinalDigest", "publicationCommitId", "publicationEventId", "publicationSlot", "publicationPayloadDigest")
		if itemKind := strings.TrimSpace(contracts.StringField(item, "kind")); itemKind == "tool_call" || itemKind == "tool_result" {
			projectedID := strings.TrimSpace(contracts.StringField(projectedItem, "id"))
			if projectedID == "" {
				return nil, false
			}
			out["itemId"] = projectedID
		}
		out["item"] = projectedItem
		return out, true
	case "tool_call_ready":
		return ordinaryEventFields(event, "toolName", "callId", "readyCount", "partial"), true
	case "tool_progress":
		out := ordinaryEventFields(event, "toolName", "callId", "status")
		out["status"] = publicToolProgressStatusV1(contracts.StringField(event, "status"))
		return out, true
	case "tool_result_upload_wait":
		if strings.TrimSpace(contracts.StringField(event, "status")) != "waiting" {
			return nil, false
		}
		out := ordinaryEventFields(event, "status", "toolResultCount")
		out["status"] = "waiting"
		return out, true
	case "tool_storm_suppressed":
		return ordinaryEventFields(event, "toolName", "callId", "message"), true
	case "tool_catalog_changed":
		return ordinaryEventFields(event, "fingerprint", "toolCount", "changeKind", "toolNames", "message"), true
	case "child_steer_queued", "child_steer_admitted", "child_steer_rejected":
		status, valid := publicSteerEventStatusV1(kind, contracts.StringField(event, "status"))
		if !valid {
			return nil, false
		}
		out := ordinaryEventFields(event,
			"status", "jobId", "childRunId", "childThreadId", "childTurnId", "steerMessageId", "parentThreadId",
			"sourceTurnId", "sourceToolCallId", "createdAt", "admittedAt", "reason",
		)
		out["status"] = status
		return out, true
	case "child_pause_requested", "child_paused", "child_resume_requested", "child_resumed", "child_pause_rejected":
		status, valid := publicPauseEventStatusV1(kind, contracts.StringField(event, "status"))
		if !valid {
			return nil, false
		}
		out := ordinaryEventFields(event,
			"status", "jobId", "childRunId", "childThreadId", "childTurnId", "pauseRequestId", "parentThreadId",
			"sourceTurnId", "createdAt", "pausedAt", "resumedAt", "reason",
		)
		out["status"] = status
		return out, true
	case "approval_requested", "approval_resolved":
		status, valid := publicGateEventStatusV1(kind, contracts.StringField(event, "status"))
		if !valid {
			return nil, false
		}
		out := ordinaryEventFields(event, "approvalId", "toolName", "status", "approvalPolicy", "sandboxMode", "summary")
		out["status"] = status
		return out, true
	case "user_input_requested", "user_input_resolved":
		status, valid := publicGateEventStatusV1(kind, contracts.StringField(event, "status"))
		if !valid {
			return nil, false
		}
		out := ordinaryEventFields(event, "inputId", "status", "prompt", "questions")
		out["status"] = status
		return out, true
	case "compaction_started":
		out := ordinaryEventFields(event,
			"summary", "replacedTokens", "auto", "pinnedConstraints", "sourceDigest", "digestMarker", "sourceItemIds",
		)
		delete(out, "itemId")
		return out, true
	case "compaction_completed":
		out := ordinaryEventFields(event,
			"summary", "replacedTokens", "auto", "pinnedConstraints", "sourceDigest", "digestMarker", "sourceItemIds",
			"schemaVersion", "reasoningExcluded", "reasoningExclusionProof",
		)
		delete(out, "itemId")
		return out, true
	case "goal_updated":
		return nil, false
	case "goal_cleared":
		return ordinaryEventFields(event, "cleared"), event["cleared"] == true && event["goal"] == nil
	case "goal_evidence_audit":
		return ordinaryEventFields(event,
			"schemaVersion", "changeId", "runtimeContract", "upstreamSource", "goalId", "result", "recovered",
			"missingProjectChecks", "incompleteTodos", "commandMismatchMissing", "latestWriterReceiptIndex",
			"blockedStateKey", "reason", "missingCheckIds", "usesReasonixPublicProtocol", "usesReasonixConfigRoot",
			"changesRendererContract", "changesProductIdentity",
		), true
	case "autoresearch_state_audit":
		if !validAutoResearchStateAuditEventV1(event) {
			return nil, false
		}
		return ordinaryEventFields(event,
			"schemaVersion", "changeId", "runtimeContract", "upstreamSource", "goalMode",
			"fileCount", "requirementCount", "completedRequirementCount", "staleRequirementCount", "staleDirectionCount",
			"complete", "pivotRequired", "result", "unknownRequirementAccepted", "findingsWrittenForUnknownRequirement",
			"writesReasonixFile", "writesAgentsFile", "stablePrefixContainsState", "toolSchemaContainsState",
			"topLevelAutoResearchRouteExposed", "usesReasonixPublicProtocol", "usesReasonixConfigRoot",
			"changesRendererContract", "changesProductIdentity",
		), true
	case "todos_updated":
		return nil, false
	case "todos_cleared":
		return ordinaryEventFields(event, "cleared"), event["cleared"] == true && event["todos"] == nil
	case "checkpoint_captured":
		return projectOrdinaryPublicCheckpointCapturedEvent(event)
	case "checkpoint_rewind_rescue_created":
		return projectOrdinaryPublicCheckpointRescueEvent(event)
	case "checkpoint_rewind_applied":
		return projectOrdinaryPublicCheckpointApplyEvent(event)
	case "pipeline_stage":
		stage, label := publicOrdinaryPipelineStageV1(contracts.StringField(event, "stage"))
		out := ordinaryEventFields(event, "stage", "label", "attempt", "maxAttempt")
		out["stage"] = stage
		out["label"] = label
		projectedDetails := projectOrdinaryPipelineDetails(stage, details)
		if stage == "provider_admission_rejected" && len(projectedDetails) == 0 {
			return nil, false
		}
		if len(projectedDetails) > 0 {
			out["details"] = projectedDetails
		}
		return out, true
	case "usage":
		usage, usageOK := event["usage"].(map[string]any)
		diagnostics, diagnosticsOK := event["cacheDiagnostics"].(map[string]any)
		if !usageOK || !diagnosticsOK || !appusage.ValidateTerminalTelemetryPublicMapsV1(usage, diagnostics) {
			return nil, false
		}
		projected := ordinaryEventFields(event,
			"model", "providerId", "endpointFormat", "effort", "usageSource", "childRunId", "usage", "cacheDiagnostics",
		)
		if raw, exists := event["effort"]; exists {
			effort, stringOK := raw.(string)
			canonical, valid := domainmodel.ProjectReasoningEffortV1(effort)
			if !stringOK || !valid {
				return nil, false
			}
			if canonical == "" {
				delete(projected, "effort")
			} else {
				projected["effort"] = canonical
			}
		}
		return projected, true
	case "error":
		out := ordinaryEventFields(event, "message", "code", "details", "severity", "terminal", "fatal", "recoverable")
		retainClosedFailureDetailsV1(out)
		return out, true
	case "heartbeat":
		return ordinaryEventFields(event), true
	case "snapshot_required":
		return ordinaryEventFields(event, "sinceSeq", "highestSeq", "replayEventCount", "reason"), true
	default:
		return nil, false
	}
}

func validProviderErrorPipelineStagePublicEventV1(event map[string]any, requireSequenceAndTimestamp bool) bool {
	if event == nil || len(event) < 6 || len(event) > 8 {
		return false
	}
	allowedKeys := map[string]bool{
		"kind": true, "threadId": true, "turnId": true, "stage": true, "label": true, "details": true,
		"seq": true, "timestamp": true,
	}
	for key := range event {
		if !allowedKeys[key] {
			return false
		}
	}
	for _, key := range []string{"kind", "threadId", "turnId", "stage", "label", "details"} {
		if _, present := event[key]; !present {
			return false
		}
	}
	_, hasSeq := event["seq"]
	_, hasTimestamp := event["timestamp"]
	if hasSeq != hasTimestamp || (requireSequenceAndTimestamp && !hasSeq) {
		return false
	}
	if contracts.StringField(event, "kind") != "pipeline_stage" ||
		contracts.StringField(event, "stage") != "provider_error" ||
		contracts.StringField(event, "label") != "Provider stream failed" ||
		contracts.SafeRecordID(contracts.StringField(event, "threadId")) != contracts.StringField(event, "threadId") ||
		contracts.SafeRecordID(contracts.StringField(event, "turnId")) != contracts.StringField(event, "turnId") {
		return false
	}
	if hasSeq {
		seq, seqOK := contracts.NumericSeq(event["seq"])
		if !seqOK || seq <= 0 {
			return false
		}
	}
	if hasTimestamp {
		timestamp, timestampOK := event["timestamp"].(string)
		if !timestampOK || strings.TrimSpace(timestamp) != timestamp || timestamp == "" {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			return false
		}
	}
	details, detailsOK := event["details"].(map[string]any)
	if !detailsOK || len(details) < 1 || len(details) > 6 {
		return false
	}
	projected, handled, err := appturn.ProjectProviderErrorPipelineStageV1(event)
	return err == nil && handled && reflect.DeepEqual(projected, event)
}

// projectGeneralTerminalOrdinaryResultV1 restores the typed ordinary result
// only on a terminal item that already passed the exact general-terminal CAS
// and outbox validation. Ordinary thread snapshots intentionally retain their
// existing assistant-text contract; trusted provider-history reconstruction
// reads the raw turn through that same CAS instead of relying on this transport
// projection.
func projectGeneralTerminalOrdinaryResultV1(raw, projected map[string]any) bool {
	if strings.TrimSpace(contracts.StringField(raw, "kind")) != "item_completed" {
		return true
	}
	rawItem, _ := raw["item"].(map[string]any)
	value, present := rawItem["ordinaryResult"]
	if !present {
		return true
	}
	slot, err := domainordinaryresult.ParseResultSlotV1(value)
	projectedItem, _ := projected["item"].(map[string]any)
	if err != nil || strings.TrimSpace(contracts.StringField(rawItem, "kind")) != "assistant_text" ||
		contracts.StringField(rawItem, "text") != slot.Text ||
		strings.TrimSpace(contracts.StringField(projectedItem, "kind")) != "assistant_text" ||
		contracts.StringField(projectedItem, "text") != slot.Text {
		return false
	}
	projectedItem["ordinaryResult"] = domainordinaryresult.ResultSlotV1Map(slot)
	return true
}

func ordinaryItemEventKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "item_created", "item_updated", "item_completed", "assistant_text_delta", "tool_call_started", "tool_call_finished":
		return true
	default:
		return false
	}
}

func ordinaryEventFields(event map[string]any, extra ...string) map[string]any {
	fields := append([]string{"kind", "seq", "timestamp", "threadId", "turnId", "itemId"}, extra...)
	projected := selectPublicFields(event, fields)
	removeUnsafeToolIdentityFieldsV1(projected)
	return projected
}

func removeUnsafeToolIdentityFieldsV1(projected map[string]any) {
	for _, key := range []string{"callId", "sourceToolCallId", "parentToolCallId"} {
		if value := contracts.StringField(projected, key); value != "" && !domainmodel.IsHostToolCallIDV1(value) {
			delete(projected, key)
		}
	}
	if itemID := contracts.StringField(projected, "itemId"); itemID != "" &&
		!domaintoolcall.IsToolCallItemIDV1(itemID) && domainprivacy.ContainsRestrictedPII(itemID) {
		delete(projected, "itemId")
	}
	if sourceIDs, ok := projected["sourceItemIds"].([]string); ok {
		projected["sourceItemIds"] = publicSafeIdentityListV1(sourceIDs)
	} else if sourceIDs, ok := projected["sourceItemIds"].([]any); ok {
		values := make([]string, 0, len(sourceIDs))
		for _, sourceID := range sourceIDs {
			if value, ok := sourceID.(string); ok {
				values = append(values, value)
			}
		}
		projected["sourceItemIds"] = publicSafeIdentityListV1(values)
	}
}

func publicSafeIdentityListV1(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !domainprivacy.ContainsRestrictedPII(value) {
			out = append(out, value)
		}
	}
	return out
}

func carriesUnsafeToolIdentityV1(record map[string]any) bool {
	for _, key := range []string{"callId", "sourceToolCallId", "parentToolCallId"} {
		if value := contracts.StringField(record, key); value != "" && !domainmodel.IsHostToolCallIDV1(value) {
			return true
		}
	}
	return false
}

func projectOrdinaryPublicEventItem(item map[string]any) (map[string]any, bool) {
	if item == nil {
		return nil, false
	}
	kind := strings.TrimSpace(contracts.StringField(item, "kind"))
	status, valid := publicOrdinaryItemStatusV1(kind, contracts.StringField(item, "status"))
	if !valid {
		return nil, false
	}
	var projected map[string]any
	switch kind {
	case "tool_call":
		return domaintoolcall.PublicToolCallItemRecordV1(item), true
	case "tool_result":
		return projectOrdinaryPublicToolResultItem(item), true
	case "user_message":
		projected = selectPublicFields(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "text", "displayText",
			"delivery", "clientUserMessageId", "attachmentIds", "fileReferences", "workspaceCheckpointId",
		})
	case "assistant_text":
		projected = selectPublicFields(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "text",
		})
	case "approval":
		projected = selectPublicFields(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "approvalId", "toolName", "summary",
		})
	case "user_input":
		projected = selectPublicFields(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "inputId", "prompt", "questions",
		})
	case "compaction":
		projected = selectPublicFields(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "summary", "replacedTokens",
			"auto", "pinnedConstraints", "sourceDigest", "digestMarker", "sourceItemIds", "schemaVersion", "reasoningExcluded",
			"reasoningExclusionProof", "assistantProseExcluded", "toolPayloadsExcluded", "caseFactsExcluded",
			"providerHistoryProjectionVersion", "caseHistoryProjectionVersion", "taskContinuation",
		})
	case "review":
		projected = selectPublicFields(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "target", "title", "reviewText", "output",
		})
	case "error":
		projected = selectPublicFields(item, []string{
			"id", "turnId", "threadId", "role", "status", "createdAt", "finishedAt", "kind", "message", "code", "details", "severity",
		})
	default:
		return nil, false
	}
	projected["kind"] = kind
	projected["role"] = publicOrdinaryItemRoleV1(kind)
	projected["status"] = status
	retainClosedFailureDetailsV1(projected)
	return projected, true
}

func retainClosedFailureDetailsV1(record map[string]any) {
	raw, present := record["details"]
	if !present {
		return
	}
	details, ok := raw.(map[string]any)
	if !ok || strings.TrimSpace(contracts.StringField(record, "code")) != "tool_not_advertised" ||
		!domainfailure.ValidateToolNotAdvertisedDetails(details) {
		delete(record, "details")
		return
	}
	record["details"] = contracts.CloneMap(details)
}

func publicThreadLifecycleEventStatusV1(kind string, status string) (string, bool) {
	kind, status = strings.TrimSpace(kind), strings.TrimSpace(status)
	switch kind {
	case "thread_created":
		return status, status == "idle"
	case "thread_updated":
		return status, oneOfPublicValue(status, "idle", "running", "archived", "deleted")
	default:
		return "", false
	}
}

func publicTurnLifecycleEventStatusV1(kind string, event map[string]any) (string, bool) {
	kind = strings.TrimSpace(kind)
	status, present := event["status"]
	if kind == "turn_steered" {
		return "", !present
	}
	value, ok := status.(string)
	if !present || !ok || value != strings.TrimSpace(value) {
		return "", false
	}
	expected := map[string]string{
		"turn_started":   "running",
		"turn_completed": "completed",
		"turn_failed":    "failed",
		"turn_aborted":   "aborted",
	}[kind]
	return value, expected != "" && value == expected
}

func publicGateEventStatusV1(kind string, status string) (string, bool) {
	kind, status = strings.TrimSpace(kind), strings.TrimSpace(status)
	switch kind {
	case "approval_requested", "user_input_requested":
		return status, status == "pending"
	case "approval_resolved":
		return status, oneOfPublicValue(status, "allowed", "denied", "expired")
	case "user_input_resolved":
		return status, oneOfPublicValue(status, "submitted", "cancelled")
	default:
		return "", false
	}
}

func publicOrdinaryItemStatusV1(kind string, status string) (string, bool) {
	kind, status = strings.TrimSpace(kind), strings.TrimSpace(status)
	var valid bool
	switch kind {
	case "user_message":
		valid = status == "completed"
	case "assistant_text":
		valid = oneOfPublicValue(status, "running", "completed", "failed", "aborted")
	case "approval":
		valid = oneOfPublicValue(status, "pending", "allowed", "denied", "expired")
	case "user_input":
		valid = oneOfPublicValue(status, "pending", "submitted", "cancelled")
	case "compaction", "review":
		valid = status == "completed"
	case "error":
		valid = oneOfPublicValue(status, "completed", "failed", "aborted")
	case "tool_call", "tool_result":
		return status, true
	default:
		return "", false
	}
	return status, valid
}

func publicOrdinaryItemRoleV1(kind string) string {
	switch strings.TrimSpace(kind) {
	case "user_message":
		return "user"
	case "assistant_text", "review":
		return "assistant"
	case "approval", "tool_call", "tool_result":
		return "tool"
	default:
		return "system"
	}
}

func ordinaryItemEventLifecycleValidV1(eventKind string, item map[string]any) bool {
	eventKind = strings.TrimSpace(eventKind)
	if withheld, _ := item["lifecycleStatusWithheld"].(bool); withheld {
		return false
	}
	kind := strings.TrimSpace(contracts.StringField(item, "kind"))
	status := strings.TrimSpace(contracts.StringField(item, "status"))
	if projectedStatus, valid := publicOrdinaryItemStatusV1(kind, status); !valid || projectedStatus != status {
		return false
	}
	switch eventKind {
	case "tool_call_started":
		return kind == "tool_call" && status == "running"
	case "tool_call_finished":
		return kind == "tool_result" && oneOfPublicValue(status, "completed", "failed")
	case "item_completed":
		switch kind {
		case "user_message", "assistant_text", "compaction", "review":
			return status == "completed"
		case "approval":
			return oneOfPublicValue(status, "allowed", "denied", "expired")
		case "user_input":
			return oneOfPublicValue(status, "submitted", "cancelled")
		case "error":
			return oneOfPublicValue(status, "completed", "failed", "aborted")
		case "tool_call", "tool_result":
			return oneOfPublicValue(status, "completed", "failed", "aborted", "cancelled")
		default:
			return false
		}
	case "item_created", "item_updated":
		return true
	default:
		return false
	}
}

func ordinaryToolResultLifecycleConsistentV1(item map[string]any) bool {
	isError, ok := item["isError"].(bool)
	if !ok {
		return false
	}
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
	if err != nil {
		return false
	}
	rootStatus, err := domaintoolresult.SettlementLifecycleStatusV1(projection, isError)
	if err != nil && projection.ProjectionKind == domaintoolresult.ProjectionWithheld &&
		projection.MessageKey == "legacy_output_withheld" && projection.Code == "legacy_output_withheld" &&
		((projection.Status == "completed" && !isError) || (projection.Status == "failed" && isError)) {
		// Read-time case-source proof failures use a lifecycle-compatible legacy
		// placeholder. It is never valid first-settlement input.
		rootStatus, err = projection.Status, nil
	}
	return err == nil && strings.TrimSpace(contracts.StringField(item, "status")) == rootStatus
}

func publicOrdinaryPipelineStageV1(value string) (string, string) {
	value = strings.TrimSpace(value)
	labels := map[string]string{
		"setup": "Setup", "pre_start": "Pre-Start", "post_start": "Post-Start",
		"input_received": "Input Received", "input_cached": "Input Cached", "input_routed": "Input Routed",
		"input_compressed": "Input Compressed", "input_remembered": "Input Remembered",
		"pre_send": "Pre-Send", "post_send": "Post-Send", "response_received": "Response Received",
		"provider_admission_rejected": "Provider admission rejected",
		"provider_retrying":           "Retrying provider stream", "step_limit_finalizing": "Model step budget reached",
		"provider_error": "Provider stream failed", "empty_final_recovered": "Provider returned an empty final response",
		"loop_guard": "Loop guard",
	}
	if label := labels[value]; label != "" {
		return value, label
	}
	if strings.HasPrefix(value, "subagent_") {
		status := strings.TrimPrefix(value, "subagent_")
		if domainjob.ValidStatusV1(status) {
			return value, "Subagent lifecycle update"
		}
	}
	if strings.HasPrefix(value, "background_job_delivery_") {
		status := strings.TrimPrefix(value, "background_job_delivery_")
		if domainjob.ValidCompletionDeliveryStatusV1(status) {
			return value, "Background job delivery update"
		}
	}
	if strings.HasPrefix(value, "background_job_auto_continue_") {
		status := strings.TrimPrefix(value, "background_job_auto_continue_")
		if domainjob.ValidAutoContinueStatusV1(status) {
			return value, "Background job continuation update"
		}
	}
	if strings.HasPrefix(value, "background_job_") {
		status := strings.TrimPrefix(value, "background_job_")
		if domainjob.TerminalStatusV1(status) {
			return value, "Background job lifecycle update"
		}
	}
	return "unknown", "Pipeline stage unavailable"
}

func projectOrdinaryPipelineDetails(stage string, details map[string]any) map[string]any {
	if details == nil {
		return nil
	}
	if stage == "provider_admission_rejected" {
		if len(details) != 4 || contracts.StringField(details, "reasonCode") != "context_window_hard_limit" {
			return nil
		}
		out := map[string]any{"reasonCode": "context_window_hard_limit"}
		for _, key := range []string{"projectedRequestTokens", "hardThresholdTokens", "providerAttemptCount"} {
			value, ok := nonNegativePublicInteger(details[key])
			if !ok {
				return nil
			}
			out[key] = float64(value)
		}
		return out
	}
	out := selectPublicFields(details, []string{
		"visibleRecovery", "recoveryKind", "recoveryAttempt", "maxRecoveryAttempt", "maxRecoveryAttempts",
		"recoveryExhausted", "partialToolStarted", "maxModelSteps", "toolName", "guardKind", "stormCount",
	})
	if diagnostic, _ := details["providerError"].(map[string]any); diagnostic != nil {
		if projected := projectOrdinaryProviderDiagnostic(diagnostic); len(projected) > 0 {
			out["providerError"] = projected
		}
	}
	return out
}

func projectOrdinaryProviderDiagnostic(diagnostic map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"kind", "code", "providerId", "family", "endpointFormat", "model", "authStatus"} {
		if value, ok := diagnostic[key].(string); ok {
			out[key] = value
		}
	}
	for _, key := range []string{"hasApiKey", "retryable"} {
		if value, ok := diagnostic[key].(bool); ok {
			out[key] = value
		}
	}
	for _, key := range []string{"status", "retryAfterMs"} {
		if value, ok := nonNegativePublicInteger(diagnostic[key]); ok {
			out[key] = float64(value)
		}
	}
	if value, ok := nonNegativePublicInteger(diagnostic["attempt"]); ok && value <= 1_000_000 {
		out["attempt"] = float64(value)
	}
	if value, ok := diagnostic["failureStage"].(string); ok {
		switch value {
		case "request_validation", "request_build", "pre_send_body_audit", "telemetry_begin",
			"transport_before_observed_send", "transport_after_observed_send", "local_admission_config",
			"callback_projection", "unclassified":
			out["failureStage"] = value
		}
	}
	if value, ok := diagnostic["dispatchState"].(string); ok {
		switch value {
		case "not_sent", "sent", "indeterminate":
			out["dispatchState"] = value
		}
	}
	return out
}

func ordinaryItemEventIdentityValid(event, item map[string]any, eventKind string) bool {
	if item == nil {
		return false
	}
	eventThreadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	eventTurnID := strings.TrimSpace(contracts.StringField(event, "turnId"))
	eventItemID := strings.TrimSpace(contracts.StringField(event, "itemId"))
	itemKind := strings.TrimSpace(contracts.StringField(item, "kind"))
	if eventThreadID == "" || eventTurnID == "" || eventItemID == "" ||
		strings.TrimSpace(contracts.StringField(item, "threadId")) != eventThreadID ||
		strings.TrimSpace(contracts.StringField(item, "turnId")) != eventTurnID ||
		strings.TrimSpace(contracts.StringField(item, "id")) != eventItemID {
		return false
	}
	switch eventKind {
	case "tool_call_started":
		return itemKind == "tool_call"
	case "tool_call_finished":
		return itemKind == "tool_result"
	case "item_created", "item_updated", "item_completed":
		return itemKind != ""
	default:
		return false
	}
}

func ordinaryEventCarriesPublicationAuthority(event, item map[string]any) bool {
	for _, record := range []map[string]any{event, item} {
		for _, key := range []string{
			"acceptedFinal", "acceptedFinalView", "acceptedFinalDigest", "publicationCommitId", "publicationEventId",
			"publicationSlot", "publicationPayloadDigest", "publicationReceipt", "publicationReceiptId",
		} {
			if _, present := record[key]; present {
				return true
			}
		}
	}
	return false
}

func projectOrdinaryPublicCheckpointCapturedEvent(event map[string]any) (map[string]any, bool) {
	checkpoint, _ := event["checkpoint"].(map[string]any)
	checkpointID, ok := validCheckpointReference(checkpoint, "checkpointId")
	status, statusOK := exactPublicString(checkpoint, "status")
	if !ok || !sameCheckpointEventAuthority(event, checkpoint, true) || !statusOK || status != "captured" ||
		!validCheckpointCaptureIntegrity(checkpoint) {
		return nil, false
	}
	changedFileCount, ok := validCapturedCheckpointFileCount(checkpoint)
	if !ok || !recordSchemaVersionOne(checkpoint) {
		return nil, false
	}
	threadID, _ := exactPublicString(event, "threadId")
	turnID, _ := exactPublicString(event, "turnId")
	metadata := checkpointPublicMetadata(checkpointID, "captured", threadID, turnID)
	metadata["changedFileCount"] = float64(changedFileCount)
	if storage, ok := checkpoint["snapshotStorage"].(string); ok && storage == "runtime_private_cas" {
		metadata["snapshotStorage"] = storage
	}
	out, ok := ordinaryCheckpointEventBase(event, "checkpoint_captured", true)
	if !ok {
		return nil, false
	}
	out["checkpoint"] = metadata
	return out, true
}

func validCheckpointCaptureIntegrity(checkpoint map[string]any) bool {
	_, hasEventID := checkpoint["captureEventId"]
	_, hasPayloadDigest := checkpoint["capturePayloadDigest"]
	if !hasEventID && !hasPayloadDigest {
		return true
	}
	eventID, eventIDOK := checkpoint["captureEventId"].(string)
	return hasEventID && hasPayloadDigest && eventIDOK && domaincheckpointref.IsCaptureEventIDV2(eventID) &&
		domaincheckpointref.CapturedPayloadDigestMatches(checkpoint)
}

func projectOrdinaryPublicCheckpointRescueEvent(event map[string]any) (map[string]any, bool) {
	rescue, _ := event["rescue"].(map[string]any)
	checkpointID, ok := validCheckpointReference(rescue, "checkpointId")
	fileCount, filesOK := validCheckpointRescueFileCount(rescue)
	if !ok || !filesOK || !recordSchemaVersionOne(rescue) || !sameCheckpointEventAuthority(event, rescue, false) ||
		!validLinkedCheckpointIdentifier(rescue, "rescueId", domaincheckpointref.RescueLink, checkpointID) ||
		!validLinkedCheckpointIdentifier(rescue, "planId", domaincheckpointref.PlanLink, checkpointID) {
		return nil, false
	}
	threadID, _ := exactPublicString(event, "threadId")
	metadata := checkpointPublicMetadata(checkpointID, "created", threadID, "")
	metadata["fileCount"] = float64(fileCount)
	out, ok := ordinaryCheckpointEventBase(event, "checkpoint_rewind_rescue_created", false)
	if !ok {
		return nil, false
	}
	out["rescue"] = metadata
	return out, true
}

func projectOrdinaryPublicCheckpointApplyEvent(event map[string]any) (map[string]any, bool) {
	apply, _ := event["apply"].(map[string]any)
	checkpointID, ok := validCheckpointReference(apply, "checkpointId")
	status, statusOK := exactPublicString(apply, "status")
	scope, scopeOK := exactPublicString(apply, "scope")
	destructive, destructiveOK := apply["destructive"].(bool)
	fileCount, projectedSummary, filesOK := validCheckpointApplyFilesAndSummary(apply)
	if !ok || !filesOK || !statusOK || !scopeOK || !recordSchemaVersionOne(apply) || !sameCheckpointEventAuthority(event, apply, false) ||
		!validLinkedCheckpointIdentifier(apply, "applyId", domaincheckpointref.ApplyLink, checkpointID) ||
		!validLinkedCheckpointIdentifier(apply, "planId", domaincheckpointref.PlanLink, checkpointID) ||
		!oneOfPublicValue(status, "applied", "blocked", "failed", "already_applied") ||
		!oneOfPublicValue(scope, "code", "conversation", "combined") || !destructiveOK || !destructive {
		return nil, false
	}
	threadID, _ := exactPublicString(event, "threadId")
	metadata := checkpointPublicMetadata(checkpointID, status, threadID, "")
	metadata["scope"] = scope
	metadata["destructive"] = true
	metadata["fileCount"] = float64(fileCount)
	conversationStatus, conversationOK := checkpointApplyConversationStatus(apply)
	if !conversationOK || !oneOfPublicValue(conversationStatus, "not_requested", "audit_recorded", "blocked", "already_applied") {
		return nil, false
	}
	metadata["conversationStatus"] = conversationStatus
	metadata["summary"] = projectedSummary
	out, ok := ordinaryCheckpointEventBase(event, "checkpoint_rewind_applied", false)
	if !ok {
		return nil, false
	}
	out["apply"] = metadata
	return out, true
}

func ordinaryCheckpointEventBase(event map[string]any, expectedKind string, requireTurn bool) (map[string]any, bool) {
	kind, kindOK := event["kind"].(string)
	threadID, threadOK := event["threadId"].(string)
	timestamp, timestampOK := event["timestamp"].(string)
	seq, seqOK := nonNegativePublicInteger(event["seq"])
	if !kindOK || kind != expectedKind || !threadOK || strings.TrimSpace(threadID) != threadID || threadID == "" ||
		!timestampOK || strings.TrimSpace(timestamp) != timestamp || timestamp == "" || !seqOK {
		return nil, false
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		return nil, false
	}
	out := map[string]any{
		"kind": kind, "threadId": threadID, "seq": float64(seq), "timestamp": timestamp,
	}
	if requireTurn {
		turnID, ok := event["turnId"].(string)
		if !ok || strings.TrimSpace(turnID) != turnID || turnID == "" {
			return nil, false
		}
		out["turnId"] = turnID
	}
	return out, true
}

func checkpointPublicMetadata(checkpointID, status, threadID, turnID string) map[string]any {
	return map[string]any{
		"schemaVersion":          float64(1),
		"projectionKind":         "checkpoint_status",
		"disclosure":             "metadata_only",
		"status":                 status,
		"checkpointRefDigest":    domaincheckpointref.PublicReferenceDigest(threadID, turnID, checkpointID),
		"privatePayloadWithheld": true,
		"factAnswerAllowed":      false,
		"evidenceAuthority":      false,
	}
}

func validCheckpointReference(record map[string]any, key string) (string, bool) {
	value, ok := exactPublicString(record, key)
	return value, ok && domaincheckpointref.IsPublicReplayRuntimeID(value)
}

func recordSchemaVersionOne(record map[string]any) bool {
	value, ok := nonNegativePublicInteger(record["schemaVersion"])
	return ok && value == 1
}

func sameCheckpointEventAuthority(event, record map[string]any, requireTurn bool) bool {
	recordThreadID, recordThreadOK := exactPublicString(record, "threadId")
	eventThreadID, eventThreadOK := exactPublicString(event, "threadId")
	if !recordThreadOK || !eventThreadOK || recordThreadID != eventThreadID {
		return false
	}
	if requireTurn {
		recordTurnID, recordTurnOK := exactPublicString(record, "turnId")
		eventTurnID, eventTurnOK := exactPublicString(event, "turnId")
		if !recordTurnOK || !eventTurnOK || recordTurnID != eventTurnID {
			return false
		}
	}
	return true
}

func exactPublicString(record map[string]any, key string) (string, bool) {
	if record == nil {
		return "", false
	}
	value, ok := record[key].(string)
	return value, ok && value != "" && strings.TrimSpace(value) == value
}

func validPublicPrefixedIdentifier(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validLinkedCheckpointIdentifier(record map[string]any, key string, kind domaincheckpointref.LinkKind, checkpointID string) bool {
	value, ok := exactPublicString(record, key)
	return ok && domaincheckpointref.LinkedIDMatches(kind, checkpointID, value)
}

func validCapturedCheckpointFileCount(checkpoint map[string]any) (int, bool) {
	if files, ok := checkpoint["changedFiles"].([]any); ok {
		for _, raw := range files {
			file, ok := raw.(map[string]any)
			if !ok {
				return 0, false
			}
			changeKind, changeKindOK := exactPublicString(file, "changeKind")
			if _, pathOK := exactPublicString(file, "relativePath"); !pathOK || !changeKindOK ||
				!oneOfPublicValue(changeKind, "created", "modified", "deleted", "unknown") {
				return 0, false
			}
		}
		return len(files), true
	}
	count, ok := nonNegativePublicInteger(checkpoint["changedFileCount"])
	storage, storageOK := exactPublicString(checkpoint, "snapshotStorage")
	return count, ok && storageOK && storage == "runtime_private_cas"
}

func validCheckpointRescueFileCount(rescue map[string]any) (int, bool) {
	if files, ok := rescue["files"].([]any); ok {
		for _, raw := range files {
			file, ok := raw.(map[string]any)
			if !ok {
				return 0, false
			}
			if _, pathOK := exactPublicString(file, "relativePath"); !pathOK {
				return 0, false
			}
			if _, existedOK := file["existed"].(bool); !existedOK {
				return 0, false
			}
		}
		return len(files), true
	}
	count, ok := nonNegativePublicInteger(rescue["fileCount"])
	storage, storageOK := exactPublicString(rescue, "storage")
	return count, ok && storageOK && storage == "runtime_private_sidecar"
}

func validCheckpointApplyFilesAndSummary(apply map[string]any) (int, map[string]any, bool) {
	keys := map[string]string{
		"applied": "fileAppliedCount", "noop": "fileNoopCount", "manual_review": "fileManualReviewCount",
		"blocked": "fileBlockedCount", "failed": "fileFailedCount",
	}
	projected := map[string]any{
		"fileAppliedCount": float64(0), "fileNoopCount": float64(0), "fileManualReviewCount": float64(0),
		"fileBlockedCount": float64(0), "fileFailedCount": float64(0),
	}
	if files, ok := apply["files"].([]any); ok {
		for _, raw := range files {
			file, ok := raw.(map[string]any)
			status, statusOK := exactPublicString(file, "status")
			_, pathOK := exactPublicString(file, "relativePath")
			_, reasonOK := exactPublicString(file, "reason")
			countKey := keys[status]
			if !ok || !statusOK || !pathOK || !reasonOK || countKey == "" {
				return 0, nil, false
			}
			projected[countKey] = projected[countKey].(float64) + 1
		}
		return len(files), projected, true
	}
	fileCount, countOK := nonNegativePublicInteger(apply["fileCount"])
	summary, summaryOK := apply["summary"].(map[string]any)
	if !countOK || !summaryOK {
		return 0, nil, false
	}
	total := 0
	for _, key := range keys {
		count, ok := nonNegativePublicInteger(summary[key])
		if !ok {
			return 0, nil, false
		}
		projected[key] = float64(count)
		total += count
	}
	return fileCount, projected, total == fileCount
}

func checkpointApplyConversationStatus(apply map[string]any) (string, bool) {
	if value, ok := exactPublicString(apply, "conversationStatus"); ok {
		return value, true
	}
	conversation, ok := apply["conversation"].(map[string]any)
	if !ok {
		return "", false
	}
	return exactPublicString(conversation, "status")
}

func nonNegativePublicInteger(value any) (int, bool) {
	parsed, ok := contracts.NumericSeq(value)
	return parsed, ok && parsed >= 0
}

func oneOfPublicValue(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func ordinaryChildLifecycleItem(item map[string]any) bool {
	if item == nil || strings.TrimSpace(contracts.StringField(item, "kind")) != "tool_progress" {
		return false
	}
	if child, _ := item["child"].(map[string]any); child != nil {
		return true
	}
	arguments, _ := item["arguments"].(map[string]any)
	diagnostics, _ := arguments["diagnostics"].(map[string]any)
	return ordinaryChildLifecycleMetadata(diagnostics)
}

func ordinaryChildLifecycleMetadata(value map[string]any) bool {
	if value == nil {
		return false
	}
	for _, key := range []string{"jobId", "childId", "childRunId", "childThreadId", "childTurnId"} {
		if strings.TrimSpace(contracts.StringField(value, key)) != "" {
			return true
		}
	}
	return false
}

func projectOrdinaryChildLifecycleItem(item map[string]any) map[string]any {
	out := selectPublicFields(item, []string{
		"id", "threadId", "turnId", "role", "status", "createdAt", "finishedAt", "kind", "toolName", "callId",
	})
	out["status"] = publicChildLifecycleItemStatusV1(contracts.StringField(item, "status"))
	out["summary"] = "child output withheld"
	out["message"] = "child output withheld"
	removeUnsafeToolIdentityFieldsV1(out)
	arguments, _ := item["arguments"].(map[string]any)
	diagnostics, _ := arguments["diagnostics"].(map[string]any)
	safeArguments := selectPublicFields(arguments, []string{"runtimeStatus", "stage", "status"})
	if strings.TrimSpace(contracts.StringField(arguments, "runtimeStatus")) == "tool_progress" {
		safeArguments["runtimeStatus"] = "tool_progress"
	} else {
		safeArguments["runtimeStatus"] = "unknown"
	}
	projectedStage := publicChildLifecycleStageV1(contracts.StringField(arguments, "stage"))
	safeArguments["stage"] = projectedStage
	safeArguments["status"] = publicChildStageStatusV1(projectedStage, contracts.StringField(arguments, "status"))
	if ordinaryChildLifecycleMetadata(diagnostics) {
		safeArguments["diagnostics"] = projectOrdinaryChildMetadata(diagnostics)
	}
	out["arguments"] = safeArguments
	return out
}

func projectOrdinaryChildMetadata(value map[string]any) map[string]any {
	out := selectPublicFields(value, []string{
		"notificationKind", "jobId", "childId", "childRunId", "childThreadId", "childTurnId",
		"parentThreadId", "parentTurnId", "parentToolCallId", "status", "childStatus", "background", "terminal",
		"autoContinueStatus", "lateCompletionSuppressed", "deliveryStatus",
	})
	out["kind"] = "subagent_task"
	status := domainjob.PublicStatusV1(firstNonEmptyProjectionStringV1(
		contracts.StringField(value, "childStatus"),
		contracts.StringField(value, "status"),
	))
	out["status"] = status
	out["childStatus"] = status
	out["terminal"] = domainjob.TerminalStatusV1(status)
	if contracts.StringField(value, "notificationKind") == "background_job_completion" {
		out["notificationKind"] = "background_job_completion"
	} else {
		delete(out, "notificationKind")
	}
	if raw := strings.TrimSpace(contracts.StringField(value, "autoContinueStatus")); raw != "" {
		out["autoContinueStatus"] = domainjob.PublicAutoContinueStatusV1(raw)
	}
	if raw := strings.TrimSpace(contracts.StringField(value, "deliveryStatus")); raw != "" {
		out["deliveryStatus"] = domainjob.PublicCompletionDeliveryStatusV1(raw)
	}
	removeUnsafeToolIdentityFieldsV1(out)
	for _, key := range []string{"jobId", "childRunId", "childId"} {
		if id := strings.TrimSpace(contracts.StringField(out, key)); id != "" {
			out["id"] = id
			break
		}
	}
	out["outputWithheld"] = true
	out["outputTrustStatus"] = "untrusted_child_output"
	out["factAnswerAllowed"] = false
	out["evidenceAuthority"] = false
	out["canReadOutput"] = false
	out["canContinueParent"] = false
	return out
}

func firstNonEmptyProjectionStringV1(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func publicToolProgressStatusV1(value string) string {
	switch strings.TrimSpace(value) {
	case "running", "success", "error":
		return strings.TrimSpace(value)
	default:
		return "unknown"
	}
}

func publicChildLifecycleItemStatusV1(value string) string {
	switch strings.TrimSpace(value) {
	case "running", "completed", "failed":
		return strings.TrimSpace(value)
	default:
		return "unknown"
	}
}

func publicChildLifecycleStageV1(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case publicBasePipelineStageV1(value):
		return value
	case strings.HasPrefix(value, "subagent_"):
		return "subagent_" + domainjob.PublicStatusV1(strings.TrimPrefix(value, "subagent_"))
	case strings.HasPrefix(value, "background_job_delivery_"):
		return "background_job_delivery_" + nonEmptyOperationalStatusV1(
			domainjob.PublicCompletionDeliveryStatusV1(strings.TrimPrefix(value, "background_job_delivery_")),
		)
	case strings.HasPrefix(value, "background_job_auto_continue_"):
		return "background_job_auto_continue_" + nonEmptyOperationalStatusV1(
			domainjob.PublicAutoContinueStatusV1(strings.TrimPrefix(value, "background_job_auto_continue_")),
		)
	case strings.HasPrefix(value, "background_job_"):
		status := domainjob.PublicStatusV1(strings.TrimPrefix(value, "background_job_"))
		if !domainjob.TerminalStatusV1(status) {
			status = domainjob.PublicStatusUnknownV1
		}
		return "background_job_" + status
	default:
		return "child_lifecycle_unknown"
	}
}

func publicBasePipelineStageV1(value string) bool {
	switch strings.TrimSpace(value) {
	case "setup", "pre_start", "post_start", "input_received", "input_cached", "input_routed",
		"input_compressed", "input_remembered", "pre_send", "post_send", "response_received",
		"provider_admission_rejected":
		return true
	default:
		return false
	}
}

func publicChildStageStatusV1(stage string, status string) string {
	stage, status = strings.TrimSpace(stage), strings.TrimSpace(status)
	if publicBasePipelineStageV1(stage) {
		return domainjob.PublicStatusV1(status)
	}
	var expected string
	var projected string
	switch {
	case strings.HasPrefix(stage, "background_job_delivery_"):
		expected = strings.TrimPrefix(stage, "background_job_delivery_")
		projected = nonEmptyOperationalStatusV1(domainjob.PublicCompletionDeliveryStatusV1(status))
	case strings.HasPrefix(stage, "background_job_auto_continue_"):
		expected = strings.TrimPrefix(stage, "background_job_auto_continue_")
		projected = nonEmptyOperationalStatusV1(domainjob.PublicAutoContinueStatusV1(status))
	case strings.HasPrefix(stage, "subagent_"):
		expected = strings.TrimPrefix(stage, "subagent_")
		projected = domainjob.PublicStatusV1(status)
	case strings.HasPrefix(stage, "background_job_"):
		expected = strings.TrimPrefix(stage, "background_job_")
		projected = domainjob.PublicStatusV1(status)
	default:
		return domainjob.PublicStatusUnknownV1
	}
	if expected == "" || expected == domainjob.PublicStatusUnknownV1 || projected != expected {
		return domainjob.PublicStatusUnknownV1
	}
	return projected
}

func nonEmptyOperationalStatusV1(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return domainjob.PublicStatusUnknownV1
	}
	return status
}

func publicSteerEventStatusV1(kind string, status string) (string, bool) {
	kind, status = strings.TrimSpace(kind), strings.TrimSpace(status)
	expected := map[string]string{
		"child_steer_queued":   "queued",
		"child_steer_admitted": "admitted",
		"child_steer_rejected": "rejected",
	}[kind]
	return status, expected != "" && status == expected
}

func publicPauseEventStatusV1(kind string, status string) (string, bool) {
	kind, status = strings.TrimSpace(kind), strings.TrimSpace(status)
	switch kind {
	case "child_pause_requested":
		return status, status == "requested"
	case "child_paused":
		return status, status == "paused"
	case "child_resume_requested":
		return status, status == "resume_requested"
	case "child_resumed":
		return status, status == "resumed"
	case "child_pause_rejected":
		return status, status == "rejected" || status == "expired"
	default:
		return "", false
	}
}

func trustedPublicationEvent(threadID string, event map[string]any, trusted *gateprojection.TrustedFinalProjectionIndex) bool {
	record, disposition, ok := trusted.ResolveForEvent(threadID, contracts.StringField(event, "turnId"))
	if !ok {
		return false
	}
	plan, err := appturn.BuildAcceptedFinalPublicationPlan(record.AcceptedFinal, record.RenderedText, record.PublicationIntent)
	if err != nil || plan.EventManifestDigest != disposition.EventManifestDigest {
		return false
	}
	slot := contracts.StringField(event, "publicationSlot")
	for _, expected := range plan.Events {
		if expected.Slot == slot && expected.EventID == contracts.StringField(event, "publicationEventId") &&
			expected.PayloadDigest == contracts.StringField(event, "publicationPayloadDigest") &&
			expected.PayloadDigest == appturn.AcceptedFinalPublicationPayloadDigest(event) {
			return true
		}
	}
	return false
}

func projectedThreadHasTurn(thread map[string]any, turnID string) bool {
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if contracts.StringField(turn, "id") == strings.TrimSpace(turnID) {
			return true
		}
	}
	return false
}

func projectedThreadHasAcceptedFinal(thread map[string]any, turnID string) bool {
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if contracts.StringField(turn, "id") == strings.TrimSpace(turnID) {
			view, ok := turn["acceptedFinalView"].(map[string]any)
			return ok && numericEqualsV1(view["schemaVersion"], domainevidence.AcceptedFinalPublicViewV3Version) &&
				strings.TrimSpace(contracts.StringField(view, "acceptedFinalDigest")) != ""
		}
	}
	return false
}

func projectedThreadItem(thread map[string]any, turnID, itemID string) (map[string]any, bool) {
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if contracts.StringField(turn, "id") != strings.TrimSpace(turnID) {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if contracts.StringField(item, "id") == strings.TrimSpace(itemID) {
				return contracts.CloneMap(item), true
			}
		}
	}
	return nil, false
}

func validAutoResearchStateAuditEventV1(event map[string]any) bool {
	allowed := map[string]bool{
		"kind": true, "seq": true, "timestamp": true, "threadId": true, "turnId": true,
		"schemaVersion": true, "changeId": true, "runtimeContract": true, "upstreamSource": true,
		"goalMode": true, "fileCount": true, "requirementCount": true,
		"completedRequirementCount": true, "staleRequirementCount": true, "staleDirectionCount": true,
		"complete": true, "pivotRequired": true, "result": true, "unknownRequirementAccepted": true,
		"findingsWrittenForUnknownRequirement": true, "writesReasonixFile": true, "writesAgentsFile": true,
		"stablePrefixContainsState": true, "toolSchemaContainsState": true,
		"topLevelAutoResearchRouteExposed": true, "usesReasonixPublicProtocol": true,
		"usesReasonixConfigRoot": true, "changesRendererContract": true, "changesProductIdentity": true,
	}
	for key := range event {
		if !allowed[key] {
			return false
		}
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	turnID := strings.TrimSpace(contracts.StringField(event, "turnId"))
	if contracts.StringField(event, "kind") != "autoresearch_state_audit" ||
		threadID == "" || contracts.SafeRecordID(threadID) != threadID ||
		turnID == "" || contracts.SafeRecordID(turnID) != turnID ||
		!numericEqualsV1(event["schemaVersion"], 1) ||
		contracts.StringField(event, "changeId") != "autoresearch-state-audit" ||
		contracts.StringField(event, "runtimeContract") != "analytix-go-runtime" ||
		contracts.StringField(event, "upstreamSource") != "reasonix-absorbed" ||
		contracts.StringField(event, "goalMode") != "research" {
		return false
	}
	if seq, present := event["seq"]; present {
		value, ok := contracts.NumericSeq(seq)
		if !ok || value <= 0 {
			return false
		}
	}
	if rawTimestamp, present := event["timestamp"]; present {
		timestamp, ok := rawTimestamp.(string)
		if !ok || strings.TrimSpace(timestamp) != timestamp {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			return false
		}
	}
	counts := make(map[string]int, 5)
	for _, key := range []string{
		"fileCount", "requirementCount", "completedRequirementCount", "staleRequirementCount", "staleDirectionCount",
	} {
		value, ok := contracts.NumericSeq(event[key])
		if !ok || value < 0 {
			return false
		}
		counts[key] = value
	}
	if counts["fileCount"] != 5 ||
		counts["completedRequirementCount"]+counts["staleRequirementCount"] != counts["requirementCount"] {
		return false
	}
	for _, key := range []string{
		"complete", "pivotRequired", "unknownRequirementAccepted", "findingsWrittenForUnknownRequirement",
		"writesReasonixFile", "writesAgentsFile", "stablePrefixContainsState", "toolSchemaContainsState",
		"topLevelAutoResearchRouteExposed", "usesReasonixPublicProtocol", "usesReasonixConfigRoot",
		"changesRendererContract", "changesProductIdentity",
	} {
		if _, ok := event[key].(bool); !ok {
			return false
		}
	}
	for _, key := range []string{
		"unknownRequirementAccepted", "findingsWrittenForUnknownRequirement", "writesReasonixFile", "writesAgentsFile",
		"stablePrefixContainsState", "toolSchemaContainsState", "topLevelAutoResearchRouteExposed",
		"usesReasonixPublicProtocol", "usesReasonixConfigRoot", "changesRendererContract", "changesProductIdentity",
	} {
		if event[key] != false {
			return false
		}
	}
	result := contracts.StringField(event, "result")
	complete := event["complete"] == true
	pivotRequired := event["pivotRequired"] == true
	if complete != (result == "complete") || pivotRequired != (result == "pivot_required") {
		return false
	}
	switch result {
	case "created", "resumed", "errored":
		return !complete && !pivotRequired
	case "pivot_required":
		return counts["staleRequirementCount"] > 0 && counts["staleDirectionCount"] > 0
	case "complete":
		return counts["requirementCount"] > 0 &&
			counts["completedRequirementCount"] == counts["requirementCount"] &&
			counts["staleRequirementCount"] == 0
	default:
		return false
	}
}

func validOrdinaryCompactionCompletedEventV1(thread, event map[string]any) bool {
	if strings.TrimSpace(contracts.StringField(event, "kind")) != "compaction_completed" {
		return true
	}
	itemID := strings.TrimSpace(contracts.StringField(event, "itemId"))
	item, ok := projectedThreadItem(thread, contracts.StringField(event, "turnId"), itemID)
	if !ok || itemID == "" || !domainevent.ValidGeneralCompactionProviderHistoryItem(item) {
		return false
	}
	if contracts.StringField(item, "threadId") != strings.TrimSpace(contracts.StringField(event, "threadId")) ||
		contracts.StringField(item, "turnId") != strings.TrimSpace(contracts.StringField(event, "turnId")) ||
		contracts.StringField(item, "id") != itemID ||
		event["reasoningExcluded"] != true ||
		!numericEqualsV1(event["schemaVersion"], 2) ||
		contracts.StringField(event, "reasoningExclusionProof") != contracts.StringField(item, "reasoningExclusionProof") {
		return false
	}
	for _, key := range []string{
		"summary", "replacedTokens", "auto", "pinnedConstraints", "sourceDigest", "digestMarker", "sourceItemIds",
	} {
		if !reflect.DeepEqual(event[key], item[key]) {
			return false
		}
	}
	return true
}

func numericEqualsV1(value any, expected int) bool {
	switch typed := value.(type) {
	case int:
		return typed == expected
	case int64:
		return typed == int64(expected)
	case float64:
		return typed == float64(expected)
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed == int64(expected)
	default:
		return false
	}
}
