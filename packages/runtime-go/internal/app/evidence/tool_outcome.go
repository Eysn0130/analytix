package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type NormalizeMCPToolOutcomeInput struct {
	Context domainsecurity.TurnSecurityContext
	Grant   domainsecurity.ExecutionGrant
	Call    domainmodel.ToolCall
	Raw     map[string]any
	At      time.Time
}

func NormalizeMCPToolOutcome(input NormalizeMCPToolOutcomeInput) (domainevidence.ToolOutcome, error) {
	if err := domainsecurity.ValidateExecutionGrantForContext(input.Grant, input.Context); err != nil ||
		!domainmodel.IsHostToolCallIDV1(input.Call.ID) ||
		!domainmodel.IsHostToolCallIDV1(input.Grant.ToolCallID) ||
		input.Grant.ContextDigest != input.Context.ContextDigest ||
		input.Grant.TurnID != input.Context.TurnID || input.Grant.ToolName != strings.TrimSpace(input.Call.Name) ||
		input.Grant.ToolCallID != strings.TrimSpace(input.Call.ID) || input.Grant.ArgsHash != domainsecurity.CanonicalJSONHash(input.Call.Arguments) {
		return domainevidence.ToolOutcome{}, errors.New("tool outcome execution grant is invalid")
	}
	raw := input.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	result := mapValue(raw["result"])
	structured := mapValue(result["structuredContent"])
	meta := mapValue(result["_meta"])
	metaOutcome := mapValue(meta["analytix_tool_outcome"])
	observationChannel := map[string]any{}
	observationMeta := map[string]any{}
	observationCandidates := []map[string]any{}
	if lossless, ok := raw[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult); ok && domainmcp.ValidToolResultObservation(lossless) {
		observation := lossless.Observation
		observationChannel["semanticStatus"] = observation.SemanticStatus
		observationChannel["isError"] = observation.IsError
		if observation.Blocker != "" {
			observationChannel["blocker"] = observation.Blocker
		}
		if len(observation.PartialCoverage) > 0 {
			observationChannel["partialCoverage"] = observation.PartialCoverage
		}
		if observation.ReportedSemanticStatus != "" {
			observationChannel["reportedSemanticStatus"] = observation.ReportedSemanticStatus
		}
		if observation.ReportedSafeToAnswer != nil {
			observationChannel["safeToAnswer"] = *observation.ReportedSafeToAnswer
		}
		if observation.ReportedCaseID != "" {
			observationChannel["caseId"] = observation.ReportedCaseID
		}
		if observation.ReportedContextEpoch > 0 {
			observationChannel["contextEpoch"] = observation.ReportedContextEpoch
		}
		if observation.ReportedDatasetSnapshotID != "" {
			observationChannel["datasetSnapshotId"] = observation.ReportedDatasetSnapshotID
		}
		if observation.ReportedServerIdentity != "" {
			observationChannel["serverIdentity"] = observation.ReportedServerIdentity
		}
		observationMeta = observation.UntrustedMeta
		observationCandidates = observation.CandidateEvidenceReceipts
	}
	transport := normalizedTransportStatus(raw)
	channels := []map[string]any{raw, result, structured, metaOutcome, observationChannel}
	reportedStatus := conservativeReportedStatus(channels)
	semantic := normalizedSemanticStatus(transport, channels...)
	reportedSafe := conservativeBoolPointer(channels, "safeToAnswer", "safe_to_answer_current_task")
	isError := transport != domainevidence.TransportSuccess || anyTrue(channels, "isError")
	if semantic != domainevidence.SemanticSuccess && semantic != domainevidence.SemanticPartial {
		isError = true
	}
	coverage := mergedPartialCoverage(structured, result, observationChannel)
	if semantic == domainevidence.SemanticPartial && len(coverage) == 0 {
		coverage = map[string]any{"coverageStatus": "partial"}
	}
	data := mapValue(structured["data"])
	if len(data) == 0 && len(structured) > 0 && !looksLikeOutcomeContract(structured) {
		data = structured
	}
	candidates := firstMapList(structured["candidateEvidenceReceipts"], structured["evidenceReceipts"], structured["evidence_receipts"], result["evidenceReceipts"], result["evidence_receipts"])
	if len(observationCandidates) > 0 {
		candidates = observationCandidates
	}
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: input.Grant.ToolName, ToolCallID: input.Grant.ToolCallID, ContextDigest: input.Context.ContextDigest,
		ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
		TransportStatus: transport, SemanticStatus: semantic, IsError: isError,
		Blocker:         normalizedBlocker(raw["blocker"], structured["blocker"], result["blocker"], metaOutcome["blocker"], semanticFailureCode(raw, semantic)),
		PartialCoverage: coverage, Data: data, CandidateEvidenceReceipts: candidates,
		ReportedSemanticStatus: reportedStatus, ReportedSafeToAnswer: reportedSafe,
		ReportedCaseID:            firstText(observationChannel["caseId"], structured["caseId"], structured["case_id"], result["caseId"], result["case_id"], data["caseId"], data["case_id"]),
		ReportedContextEpoch:      firstUint64(observationChannel["contextEpoch"], structured["contextEpoch"], structured["context_epoch"], result["contextEpoch"], result["context_epoch"], data["contextEpoch"], data["context_epoch"]),
		ReportedDatasetSnapshotID: firstText(observationChannel["datasetSnapshotId"], structured["datasetSnapshotId"], structured["dataset_snapshot_id"], result["datasetSnapshotId"], result["dataset_snapshot_id"], data["datasetSnapshotId"], data["dataset_snapshot_id"]),
		ReportedServerIdentity:    firstText(observationChannel["serverIdentity"], structured["serverIdentity"], structured["server_identity"], result["serverIdentity"], result["server_identity"], data["serverIdentity"], data["server_identity"]),
		UntrustedMeta:             normalizedOutcomeMeta(mergeOutcomeMeta(meta, observationMeta), raw["rpcError"]), IssuedAt: input.At,
	})
	if err := domainevidence.ValidateToolOutcome(outcome); err != nil {
		return domainevidence.ToolOutcome{}, err
	}
	return outcome, nil
}

func mergeOutcomeMeta(values ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, value := range values {
		for key, item := range value {
			out[key] = item
		}
	}
	return out
}

func normalizedOutcomeMeta(meta map[string]any, rpcValue any) map[string]any {
	out := make(map[string]any, len(meta)+1)
	for key, value := range meta {
		out[key] = value
	}
	if bounded, ok := domainmcp.BoundedJSONRPCDiagnostic(rpcValue); ok {
		out["jsonRpcError"] = bounded
	}
	return out
}

func normalizedTransportStatus(raw map[string]any) domainevidence.TransportStatus {
	var explicit domainevidence.TransportStatus
	for _, key := range []string{"transportStatus", "transport_status"} {
		value, exists := raw[key]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return domainevidence.TransportFailure
		}
		var candidate domainevidence.TransportStatus
		switch strings.ToLower(strings.TrimSpace(text)) {
		case "success":
			candidate = domainevidence.TransportSuccess
		case "timeout":
			candidate = domainevidence.TransportTimeout
		case "cancelled", "canceled":
			candidate = domainevidence.TransportCancelled
		case "failure", "failed", "error":
			candidate = domainevidence.TransportFailure
		default:
			return domainevidence.TransportFailure
		}
		if explicit != "" && explicit != candidate {
			return domainevidence.TransportFailure
		}
		explicit = candidate
	}
	if explicit != "" {
		return explicit
	}
	if value, exists := raw["executed"]; exists {
		if executed, ok := value.(bool); ok && executed {
			return domainevidence.TransportSuccess
		}
		return domainevidence.TransportFailure
	}
	code := strings.ToLower(firstText(raw["code"]))
	if strings.Contains(code, "timeout") || strings.Contains(code, "deadline") {
		return domainevidence.TransportTimeout
	}
	if strings.Contains(code, "cancel") || strings.Contains(code, "abort") || strings.Contains(code, "interrupt") {
		return domainevidence.TransportCancelled
	}
	return domainevidence.TransportFailure
}

func normalizedSemanticStatus(transport domainevidence.TransportStatus, records ...map[string]any) domainevidence.SemanticStatus {
	if transport == domainevidence.TransportTimeout {
		return domainevidence.SemanticTimeout
	}
	if transport == domainevidence.TransportCancelled {
		return domainevidence.SemanticCancelled
	}
	if transport != domainevidence.TransportSuccess {
		code := ""
		if len(records) > 0 {
			code = strings.ToLower(firstText(records[0]["code"]))
		}
		if strings.Contains(code, "unavailable") || strings.Contains(code, "not_connected") || strings.Contains(code, "missing") {
			return domainevidence.SemanticUnavailable
		}
		return domainevidence.SemanticFailure
	}
	status := domainevidence.SemanticSuccess
	rank := 0
	set := func(candidate domainevidence.SemanticStatus, candidateRank int) {
		if candidateRank > rank {
			status = candidate
			rank = candidateRank
		}
	}
	for _, record := range records {
		if value, exists := record["executed"]; exists {
			if executed, ok := value.(bool); !ok || !executed {
				set(domainevidence.SemanticFailure, 70)
			}
		}
		for _, key := range []string{"transportStatus", "transport_status"} {
			if value, exists := record[key]; exists {
				text, ok := value.(string)
				if !ok {
					set(domainevidence.SemanticFailure, 70)
					continue
				}
				switch strings.ToLower(strings.TrimSpace(text)) {
				case "success":
				case "failure", "failed", "error":
					set(domainevidence.SemanticFailure, 70)
				case "timeout", "timed_out":
					set(domainevidence.SemanticTimeout, 65)
				case "cancelled", "canceled":
					set(domainevidence.SemanticCancelled, 60)
				default:
					set(domainevidence.SemanticFailure, 70)
				}
			}
		}
		for _, key := range []string{"semanticStatus", "semantic_status"} {
			if value, exists := record[key]; exists {
				text, ok := value.(string)
				if !ok {
					set(domainevidence.SemanticFailure, 70)
					continue
				}
				switch strings.ToLower(strings.TrimSpace(text)) {
				case "success", "succeeded", "ok", "complete", "completed":
				case "partial", "incomplete":
					set(domainevidence.SemanticPartial, 20)
				case "blocked", "capability_gap":
					set(domainevidence.SemanticBlocked, 50)
				case "unavailable", "source_unavailable":
					set(domainevidence.SemanticUnavailable, 55)
				case "cancelled", "canceled":
					set(domainevidence.SemanticCancelled, 60)
				case "timeout", "timed_out":
					set(domainevidence.SemanticTimeout, 65)
				case "failure", "failed", "error":
					set(domainevidence.SemanticFailure, 70)
				default:
					set(domainevidence.SemanticFailure, 70)
				}
			}
		}
		if value, exists := record["isError"]; exists {
			if isError, ok := value.(bool); !ok || isError {
				set(domainevidence.SemanticFailure, 45)
			}
		}
		for _, key := range []string{"safeToAnswer", "safe_to_answer_current_task"} {
			if value, exists := record[key]; exists {
				if safe, ok := value.(bool); !ok {
					set(domainevidence.SemanticFailure, 70)
				} else if !safe {
					set(domainevidence.SemanticBlocked, 50)
				}
			}
		}
		if blocker, exists := record["blocker"]; exists {
			switch blocker.(type) {
			case string, map[string]any:
				if normalizedBlocker(blocker) != "" || len(mapValue(blocker)) > 0 {
					set(domainevidence.SemanticBlocked, 50)
				}
			default:
				set(domainevidence.SemanticFailure, 70)
			}
		}
		for _, key := range []string{"partialCoverage", "partial_coverage"} {
			if coverage, exists := record[key]; exists {
				switch typed := coverage.(type) {
				case bool:
				case string:
					if domainevidence.ValidateCoverageDescriptor(map[string]any{"coverageStatus": typed}) != nil {
						set(domainevidence.SemanticFailure, 70)
					}
				case map[string]any:
					if domainevidence.ValidateCoverageDescriptor(typed) != nil {
						set(domainevidence.SemanticFailure, 70)
					}
				default:
					set(domainevidence.SemanticFailure, 70)
				}
			}
		}
		if outcomeRecordCoverageIncomplete(record) {
			set(domainevidence.SemanticPartial, 20)
		}
	}
	return status
}

func conservativeReportedStatus(records []map[string]any) string {
	for _, record := range records {
		if reported := firstText(record["reportedSemanticStatus"], record["reported_semantic_status"]); reported != "" {
			return reported
		}
	}
	status := normalizedSemanticStatus(domainevidence.TransportSuccess, records...)
	return string(status)
}

func conservativeBoolPointer(records []map[string]any, keys ...string) *bool {
	foundTrue := false
	for _, record := range records {
		for _, key := range keys {
			if value, ok := record[key].(bool); ok {
				if !value {
					result := false
					return &result
				}
				foundTrue = true
			}
		}
	}
	if foundTrue {
		result := true
		return &result
	}
	return nil
}

func anyTrue(records []map[string]any, keys ...string) bool {
	for _, record := range records {
		for _, key := range keys {
			if value, ok := record[key].(bool); ok && value {
				return true
			}
		}
	}
	return false
}

func mergedPartialCoverage(records ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, record := range records {
		for _, key := range []string{"partialCoverage", "partial_coverage"} {
			switch value := record[key].(type) {
			case map[string]any:
				out = domainevidence.MergeConservativeCoverage(out, value)
			case bool:
				if value {
					out = domainevidence.MergeConservativeCoverage(out, map[string]any{"partial": true})
				}
			case string:
				if text := strings.TrimSpace(value); text != "" {
					out = domainevidence.MergeConservativeCoverage(out, map[string]any{"coverageStatus": text})
				}
			}
		}
	}
	return out
}

func outcomeRecordCoverageIncomplete(record map[string]any) bool {
	for _, key := range []string{"partialCoverage", "partial_coverage"} {
		if value, exists := record[key]; exists {
			switch typed := value.(type) {
			case bool:
				if typed {
					return true
				}
			case string:
				if outcomeCoverageIncomplete(map[string]any{"coverageStatus": typed}, 0) {
					return true
				}
			case map[string]any:
				if outcomeCoverageIncomplete(typed, 0) {
					return true
				}
			}
		}
	}
	for _, candidate := range []map[string]any{record, mapValue(record["data"])} {
		for _, key := range []string{"truncated", "hasMore", "has_more", "support_payload_truncated"} {
			if value, ok := candidate[key].(bool); ok && value {
				return true
			}
		}
		for _, key := range []string{"paginationComplete", "pagination_complete", "source_coverage_complete"} {
			if value, ok := candidate[key].(bool); ok && !value {
				return true
			}
		}
		for _, key := range []string{"coverageStatus", "coverage_status", "paginationCompleteness", "pagination_completeness"} {
			if value, ok := candidate[key].(string); ok && outcomeCoverageIncomplete(map[string]any{"coverageStatus": value}, 0) {
				return true
			}
		}
		for _, key := range []string{"resultCompleteness", "result_completeness"} {
			if value, ok := candidate[key].(map[string]any); ok && outcomeCoverageIncomplete(value, 0) {
				return true
			}
		}
	}
	return false
}

func outcomeCoverageIncomplete(value any, depth int) bool {
	if depth > 12 {
		return true
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "partial", "partialCoverage", "partial_coverage", "truncated", "hasMore", "has_more", "support_payload_truncated", "coverageConflict":
				if flag, ok := child.(bool); ok && flag {
					return true
				}
			case "complete", "coverageComplete", "coverage_complete", "paginationComplete", "pagination_complete", "source_coverage_complete":
				if flag, ok := child.(bool); ok && !flag {
					return true
				}
			case "coverageStatus", "coverage_status", "paginationCompleteness", "pagination_completeness", "status":
				if text, ok := child.(string); ok {
					switch strings.ToLower(strings.TrimSpace(text)) {
					case "partial", "incomplete", "unknown", "truncated":
						return true
					}
				}
			}
			if outcomeCoverageIncomplete(child, depth+1) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if outcomeCoverageIncomplete(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func semanticFailureCode(raw map[string]any, semantic domainevidence.SemanticStatus) any {
	if semantic == domainevidence.SemanticSuccess || semantic == domainevidence.SemanticPartial {
		return nil
	}
	return raw["code"]
}

func looksLikeOutcomeContract(record map[string]any) bool {
	for _, key := range []string{
		"transportStatus", "transport_status", "semanticStatus", "semantic_status", "safeToAnswer", "safe_to_answer_current_task",
		"isError", "blocker", "partialCoverage", "partial_coverage", "candidateEvidenceReceipts", "evidenceReceipts", "evidence_receipts",
	} {
		if _, exists := record[key]; exists {
			return true
		}
	}
	return false
}

func normalizedBlocker(values ...any) string {
	for _, value := range values {
		if record := mapValue(value); len(record) > 0 {
			if text := firstText(record["code"], record["reason"], record["message"]); text != "" {
				return text
			}
			return "source_blocker_present"
		}
		if text := firstText(value); text != "" {
			return text
		}
	}
	return ""
}

func mapValue(value any) map[string]any {
	record, _ := value.(map[string]any)
	if record == nil {
		return map[string]any{}
	}
	return record
}

func firstMap(values ...any) map[string]any {
	for _, value := range values {
		if record := mapValue(value); len(record) > 0 {
			return record
		}
	}
	return map[string]any{}
}

func firstMapList(values ...any) []map[string]any {
	for _, value := range values {
		items, ok := value.([]any)
		if !ok {
			if typed, typedOK := value.([]map[string]any); typedOK {
				return typed
			}
			continue
		}
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if record := mapValue(item); len(record) > 0 {
				out = append(out, record)
			}
		}
		return out
	}
	return []map[string]any{}
}

func firstBoolPointer(values ...any) *bool {
	for _, value := range values {
		if typed, ok := value.(bool); ok {
			return &typed
		}
	}
	return nil
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func firstText(values ...any) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func firstUint64(values ...any) uint64 {
	for _, value := range values {
		switch typed := value.(type) {
		case uint64:
			if typed > 0 {
				return typed
			}
		case int:
			if typed > 0 {
				return uint64(typed)
			}
		case float64:
			if typed > 0 && typed == float64(uint64(typed)) {
				return uint64(typed)
			}
		case json.Number:
			if parsed, err := typed.Int64(); err == nil && parsed > 0 {
				return uint64(parsed)
			}
		}
	}
	return 0
}
