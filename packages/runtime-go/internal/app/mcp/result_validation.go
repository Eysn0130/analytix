package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
)

type ToolResultObservation = domainmcp.ToolResultObservation

const maxDecodedMCPContentBytes = 768 * 1024

// ValidateToolResultOutput validates the exact MCP tools/call result bytes
// against the live catalog's host-private output schema. It never validates a
// lossy provider-facing projection and never treats remote self-report as
// authority.
func ValidateToolResultOutput(result domainmcp.LosslessToolResult, outputSchema json.RawMessage) error {
	_, err := InspectToolResultOutput(result, outputSchema)
	return err
}

// InspectToolResultOutput classifies the exact transport bytes. Conflicting
// remote assertions are combined monotonically: metadata can downgrade a
// result but can never hide an error, blocker, unsafe marker, or incomplete
// coverage observed in another channel.
func InspectToolResultOutput(result domainmcp.LosslessToolResult, outputSchema json.RawMessage) (ToolResultObservation, error) {
	if !domainmcp.ValidLosslessToolResult(result) {
		return ToolResultObservation{}, errors.New("lossless MCP tool result integrity is invalid")
	}
	if err := domainjsonstrict.Validate(result.RawResult, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	}); err != nil {
		return ToolResultObservation{}, errors.New("MCP tool result is not strict JSON")
	}
	envelope, err := domainjsonstrict.DecodeRawObject(result.RawResult, domainjsonstrict.Options{
		MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	if err != nil {
		return ToolResultObservation{}, errors.New("MCP tool result must be one strict JSON object")
	}
	envelopeRecord, err := decodeToolResultRecord(result.RawResult)
	if err != nil {
		return ToolResultObservation{}, err
	}
	observation := observedToolResultObservation(result, envelopeRecord)
	reject := func(cause error) (ToolResultObservation, error) {
		observation.SemanticStatus = "failure"
		observation.IsError = true
		if observation.Blocker == "" {
			observation.Blocker = "mcp_output_schema_invalid"
		}
		return observation, cause
	}
	for key := range envelope {
		if !allowedToolResultEnvelopeField(key) {
			return reject(fmt.Errorf("MCP tool result contains unsupported field %q", key))
		}
	}
	if raw, ok := envelope["content"]; !ok || !validToolResultContent(raw) {
		return reject(errors.New("MCP tool result content is invalid"))
	}
	for _, key := range []string{"isError", "safeToAnswer", "safe_to_answer_current_task"} {
		if raw, ok := envelope[key]; ok && !rawJSONBool(raw) {
			return reject(fmt.Errorf("MCP tool result %s must be boolean", key))
		}
	}
	remoteIsError := false
	if raw, ok := envelope["isError"]; ok {
		_ = json.Unmarshal(raw, &remoteIsError)
	}
	if raw, ok := envelope["_meta"]; ok && !validToolResultMeta(raw) {
		return reject(errors.New("MCP tool result _meta must be an object"))
	}
	for _, key := range []string{"semanticStatus", "semantic_status"} {
		if raw, ok := envelope[key]; ok && !rawJSONString(raw) {
			return reject(fmt.Errorf("MCP tool result %s must be a string", key))
		}
	}
	for _, key := range []string{"blocker", "partialCoverage", "partial_coverage"} {
		if raw, ok := envelope[key]; ok && ((key == "blocker" && !rawJSONObjectOrString(raw)) || (key != "blocker" && !rawJSONObjectOrBoolOrString(raw))) {
			return reject(fmt.Errorf("MCP tool result %s has invalid type", key))
		}
	}
	for _, key := range []string{"evidenceReceipts", "evidence_receipts"} {
		if raw, ok := envelope[key]; ok && !rawJSONArrayOfObjects(raw) {
			return reject(fmt.Errorf("MCP tool result %s must be an array", key))
		}
	}
	structured, hasStructured := envelope["structuredContent"]
	if remoteIsError {
		// MCP output schemas describe successful results. A tool-reported error
		// may omit structuredContent; when present it is still required to be a
		// bounded JSON object, but it cannot become evidence authority.
		if hasStructured && !rawJSONObject(structured) {
			return reject(errors.New("MCP error structuredContent must be an object"))
		}
	} else {
		if !hasStructured || len(structured) == 0 || bytes.Equal(bytes.TrimSpace(structured), []byte("null")) {
			return reject(errors.New("MCP tool result is missing structuredContent"))
		}
		if err := toolcatalogapp.ValidateJSONSchemaRawValue(structured, outputSchema, "structuredContent"); err != nil {
			return reject(err)
		}
		if _, ok := envelopeRecord["structuredContent"].(map[string]any); !ok {
			return reject(errors.New("MCP structuredContent must be an object"))
		}
	}
	if meta, ok := envelopeRecord["_meta"].(map[string]any); ok {
		if metaOutcome, ok := meta["analytix_tool_outcome"].(map[string]any); ok {
			if !validAnalytixToolOutcomeMeta(metaOutcome) {
				return reject(errors.New("MCP analytix_tool_outcome metadata is invalid"))
			}
		} else if _, exists := meta["analytix_tool_outcome"]; exists {
			return reject(errors.New("MCP analytix_tool_outcome metadata must be an object"))
		}
	}
	return observation, nil
}

func observedToolResultObservation(result domainmcp.LosslessToolResult, envelopeRecord map[string]any) ToolResultObservation {
	channels := []map[string]any{envelopeRecord}
	if structured, ok := envelopeRecord["structuredContent"].(map[string]any); ok {
		channels = append(channels, structured)
	}
	untrustedMeta := map[string]any{}
	if meta, ok := envelopeRecord["_meta"].(map[string]any); ok {
		untrustedMeta = cloneToolResultMap(meta)
		if metaOutcome, ok := meta["analytix_tool_outcome"].(map[string]any); ok {
			channels = append(channels, metaOutcome)
		}
	}
	observation := conservativeToolResultObservation(channels)
	observation.RawSHA256 = result.RawSHA256
	observation.CandidateEvidenceReceipts = collectToolResultReceiptCandidates(channels)
	observation.ReportedSemanticStatus = observedReportedSemanticStatus(channels, observation.SemanticStatus)
	observation.ReportedSafeToAnswer = observedSafeToAnswer(channels)
	observation.ReportedCaseID = observedText(channels, "caseId", "case_id")
	observation.ReportedContextEpoch = observedUint64(channels, "contextEpoch", "context_epoch")
	observation.ReportedDatasetSnapshotID = observedText(channels, "datasetSnapshotId", "dataset_snapshot_id")
	observation.ReportedServerIdentity = observedText(channels, "serverIdentity", "server_identity")
	observation.UntrustedMeta = untrustedMeta
	return observation
}

func decodeToolResultRecord(raw json.RawMessage) (map[string]any, error) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("MCP tool result must decode as an object")
	}
	return value, nil
}

func conservativeToolResultObservation(channels []map[string]any) ToolResultObservation {
	observation := ToolResultObservation{
		SemanticStatus: "success", PartialCoverage: map[string]any{},
		CandidateEvidenceReceipts: []map[string]any{}, UntrustedMeta: map[string]any{},
	}
	statusRank := 0
	setStatus := func(status string, rank int) {
		if rank > statusRank {
			observation.SemanticStatus = status
			statusRank = rank
		}
	}
	for _, record := range channels {
		for _, key := range []string{"transportStatus", "transport_status"} {
			if value, exists := record[key]; exists {
				status, ok := value.(string)
				if !ok {
					observation.IsError = true
					setStatus("failure", 70)
					continue
				}
				switch strings.ToLower(strings.TrimSpace(status)) {
				case "success":
				case "failure", "failed", "error":
					observation.IsError = true
					setStatus("failure", 70)
				case "timeout", "timed_out":
					observation.IsError = true
					setStatus("timeout", 65)
				case "cancelled", "canceled":
					observation.IsError = true
					setStatus("cancelled", 60)
				default:
					observation.IsError = true
					setStatus("failure", 70)
				}
			}
		}
		for _, key := range []string{"semanticStatus", "semantic_status"} {
			if value, exists := record[key]; exists {
				status, ok := value.(string)
				if !ok {
					observation.IsError = true
					setStatus("failure", 70)
					continue
				}
				switch strings.ToLower(strings.TrimSpace(status)) {
				case "failure", "failed", "error":
					observation.IsError = true
					setStatus("failure", 70)
				case "timeout", "timed_out":
					observation.IsError = true
					setStatus("timeout", 65)
				case "cancelled", "canceled":
					observation.IsError = true
					setStatus("cancelled", 60)
				case "unavailable", "source_unavailable":
					observation.IsError = true
					setStatus("unavailable", 55)
				case "blocked", "capability_gap":
					observation.IsError = true
					setStatus("blocked", 50)
				case "partial", "incomplete":
					setStatus("partial", 20)
				case "success", "succeeded", "ok", "complete", "completed":
					// Positive source assertions never lower a prior status.
				default:
					observation.IsError = true
					setStatus("failure", 70)
				}
			}
		}
		if value, exists := record["isError"]; exists {
			isError, ok := value.(bool)
			if !ok {
				observation.IsError = true
				setStatus("failure", 70)
			} else if isError {
				observation.IsError = true
				setStatus("failure", 45)
			}
		}
		for _, key := range []string{"safeToAnswer", "safe_to_answer_current_task"} {
			if value, exists := record[key]; exists {
				safe, ok := value.(bool)
				if !ok {
					observation.IsError = true
					setStatus("failure", 70)
				} else if !safe {
					// True never authorizes publication, while false is a monotonic
					// source-side downgrade that the host must preserve.
					observation.IsError = true
					setStatus("blocked", 50)
				}
			}
		}
		if blockerValue, exists := record["blocker"]; exists {
			if !validDecodedBlocker(blockerValue) {
				observation.IsError = true
				setStatus("failure", 70)
			} else if blocker := toolResultBlocker(blockerValue); blocker != "" {
				if observation.Blocker == "" {
					observation.Blocker = blocker
				}
				observation.IsError = true
				setStatus("blocked", 50)
			}
		}
		for _, key := range []string{"partialCoverage", "partial_coverage"} {
			if value, exists := record[key]; exists {
				if !validDecodedCoverage(value) {
					observation.IsError = true
					setStatus("failure", 70)
				} else if mergeCoverageObservation(&observation, value) {
					setStatus("partial", 20)
				}
			}
		}
		if toolResultRecordCoverageIncomplete(record) {
			setStatus("partial", 20)
		}
	}
	if observation.SemanticStatus == "partial" && len(observation.PartialCoverage) == 0 {
		observation.PartialCoverage = map[string]any{"coverageStatus": "partial"}
	}
	if observation.SemanticStatus != "success" && observation.SemanticStatus != "partial" {
		observation.IsError = true
	}
	return observation
}

func collectToolResultReceiptCandidates(channels []map[string]any) []map[string]any {
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, record := range channels {
		for _, key := range []string{"candidateEvidenceReceipts", "evidenceReceipts", "evidence_receipts"} {
			items, ok := record[key].([]any)
			if !ok {
				continue
			}
			for _, item := range items {
				candidate, ok := item.(map[string]any)
				if !ok {
					continue
				}
				body, err := json.Marshal(candidate)
				if err != nil || seen[string(body)] {
					continue
				}
				seen[string(body)] = true
				out = append(out, cloneToolResultMap(candidate))
			}
		}
	}
	return out
}

func observedReportedSemanticStatus(channels []map[string]any, fallback string) string {
	if value := observedText(channels, "reportedSemanticStatus", "reported_semantic_status"); value != "" {
		return value
	}
	if value := observedText(channels, "semanticStatus", "semantic_status"); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func observedSafeToAnswer(channels []map[string]any) *bool {
	foundTrue := false
	for _, record := range channels {
		for _, key := range []string{"safeToAnswer", "safe_to_answer_current_task"} {
			value, ok := record[key].(bool)
			if !ok {
				continue
			}
			if !value {
				result := false
				return &result
			}
			foundTrue = true
		}
	}
	if foundTrue {
		result := true
		return &result
	}
	return nil
}

func observedText(channels []map[string]any, keys ...string) string {
	for _, record := range channels {
		for _, key := range keys {
			if value, ok := record[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func observedUint64(channels []map[string]any, keys ...string) uint64 {
	for _, record := range channels {
		for _, key := range keys {
			switch value := record[key].(type) {
			case json.Number:
				if parsed, err := value.Int64(); err == nil && parsed > 0 {
					return uint64(parsed)
				}
			case uint64:
				if value > 0 {
					return value
				}
			case int:
				if value > 0 {
					return uint64(value)
				}
			}
		}
	}
	return 0
}

func cloneToolResultMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	body, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&out) != nil {
		return map[string]any{}
	}
	return out
}

func validDecodedBlocker(value any) bool {
	switch value.(type) {
	case string, map[string]any:
		return true
	default:
		return false
	}
}

func validDecodedCoverage(value any) bool {
	switch typed := value.(type) {
	case bool:
		return true
	case string:
		return domainevidence.ValidateCoverageDescriptor(map[string]any{"coverageStatus": typed}) == nil
	case map[string]any:
		return domainevidence.ValidateCoverageDescriptor(typed) == nil
	default:
		return false
	}
}

func mergeCoverageObservation(observation *ToolResultObservation, value any) bool {
	switch typed := value.(type) {
	case bool:
		if typed {
			observation.PartialCoverage["partial"] = true
			return true
		}
	case string:
		status := strings.ToLower(strings.TrimSpace(typed))
		if status == "partial" || status == "incomplete" || status == "unknown" {
			observation.PartialCoverage["coverageStatus"] = status
			return true
		}
	case map[string]any:
		observation.PartialCoverage = domainevidence.MergeConservativeCoverage(observation.PartialCoverage, typed)
		return len(typed) > 0 && toolResultCoverageIncomplete(typed, 0)
	}
	return false
}

func toolResultBlocker(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	if record, ok := value.(map[string]any); ok {
		for _, key := range []string{"code", "reason", "message"} {
			if text, ok := record[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
		if len(record) > 0 {
			return "source_blocker_present"
		}
	}
	return ""
}

func toolResultRecordCoverageIncomplete(record map[string]any) bool {
	for _, key := range []string{"partialCoverage", "partial_coverage"} {
		if value, exists := record[key]; exists {
			switch typed := value.(type) {
			case bool:
				if typed {
					return true
				}
			case string:
				if toolResultCoverageIncomplete(map[string]any{"coverageStatus": typed}, 0) {
					return true
				}
			case map[string]any:
				if toolResultCoverageIncomplete(typed, 0) {
					return true
				}
			}
		}
	}
	for _, candidate := range []map[string]any{record, mapValueForCoverage(record["data"])} {
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
			if value, ok := candidate[key].(string); ok && toolResultCoverageIncomplete(map[string]any{"coverageStatus": value}, 0) {
				return true
			}
		}
		for _, key := range []string{"resultCompleteness", "result_completeness"} {
			if value, ok := candidate[key].(map[string]any); ok && toolResultCoverageIncomplete(value, 0) {
				return true
			}
		}
	}
	return false
}

func mapValueForCoverage(value any) map[string]any {
	record, _ := value.(map[string]any)
	return record
}

func toolResultCoverageIncomplete(value any, depth int) bool {
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
				if status, ok := child.(string); ok {
					switch strings.ToLower(strings.TrimSpace(status)) {
					case "partial", "incomplete", "unknown", "truncated":
						return true
					}
				}
			}
			if toolResultCoverageIncomplete(child, depth+1) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if toolResultCoverageIncomplete(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func validAnalytixToolOutcomeMeta(record map[string]any) bool {
	for key, value := range record {
		switch key {
		case "transportStatus", "transport_status", "semanticStatus", "semantic_status", "reportedSemanticStatus", "reported_semantic_status",
			"caseId", "case_id", "datasetSnapshotId", "dataset_snapshot_id", "serverIdentity", "server_identity":
			if _, ok := value.(string); !ok {
				return false
			}
		case "safeToAnswer", "safe_to_answer_current_task", "isError":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "blocker":
			if !validDecodedBlocker(value) {
				return false
			}
		case "partialCoverage", "partial_coverage":
			if !validDecodedCoverage(value) {
				return false
			}
		case "data":
			if _, ok := value.(map[string]any); !ok {
				return false
			}
		case "candidateEvidenceReceipts", "evidenceReceipts", "evidence_receipts":
			items, ok := value.([]any)
			if !ok {
				return false
			}
			for _, item := range items {
				if _, ok := item.(map[string]any); !ok {
					return false
				}
			}
		case "contextEpoch", "context_epoch":
			number, ok := value.(json.Number)
			if !ok {
				return false
			}
			if parsed, err := number.Int64(); err != nil || parsed <= 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func allowedToolResultEnvelopeField(key string) bool {
	switch key {
	case "content", "structuredContent", "isError", "_meta",
		"safeToAnswer", "safe_to_answer_current_task", "blocker",
		"partialCoverage", "partial_coverage", "evidenceReceipts", "evidence_receipts",
		"semanticStatus", "semantic_status":
		return true
	default:
		return false
	}
}

func validToolResultContent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return false
	}
	var items []json.RawMessage
	if json.Unmarshal(trimmed, &items) != nil {
		return false
	}
	for _, item := range items {
		part, err := domainjsonstrict.DecodeRawObject(item, domainjsonstrict.Options{
			MaxBytes: 1024 * 1024, MaxTokens: 50_000, MaxStringBytes: 1024 * 1024,
		})
		if err != nil {
			return false
		}
		var contentType string
		if json.Unmarshal(part["type"], &contentType) != nil || !validContentAnnotations(part["annotations"]) {
			return false
		}
		if !optionalRawObject(part["_meta"]) {
			return false
		}
		switch contentType {
		case "text":
			if !rawObjectHasOnly(part, "type", "text", "annotations", "_meta") || !rawJSONString(part["text"]) {
				return false
			}
		case "image", "audio":
			if !rawObjectHasOnly(part, "type", "data", "mimeType", "annotations", "_meta") || !rawBoundedBase64(part["data"]) || !rawContentMIMEType(part["mimeType"], contentType) {
				return false
			}
		case "resource":
			if !rawObjectHasOnly(part, "type", "resource", "annotations", "_meta") || !validEmbeddedResource(part["resource"]) {
				return false
			}
		case "resource_link":
			if !rawObjectHasOnly(part, "type", "uri", "name", "title", "description", "mimeType", "size", "icons", "annotations", "_meta") ||
				!rawNonEmptyString(part["uri"]) || !rawNonEmptyString(part["name"]) || !optionalRawString(part["title"]) ||
				!optionalRawString(part["description"]) || !optionalRawString(part["mimeType"]) || !optionalRawNonnegativeInteger(part["size"]) ||
				!optionalRawIcons(part["icons"]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func rawObjectHasOnly(object map[string]json.RawMessage, allowed ...string) bool {
	allowlist := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowlist[key] = true
	}
	for key := range object {
		if !allowlist[key] {
			return false
		}
	}
	return true
}

func rawNonEmptyString(raw json.RawMessage) bool {
	var value string
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != ""
}

func optionalRawString(raw json.RawMessage) bool {
	return len(raw) == 0 || rawJSONString(raw)
}

func optionalRawObject(raw json.RawMessage) bool {
	return len(raw) == 0 || rawJSONObject(raw)
}

func optionalRawNonnegativeInteger(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var value json.Number
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	parsed, err := value.Int64()
	return err == nil && parsed >= 0
}

func optionalRawIcons(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var icons []json.RawMessage
	if json.Unmarshal(raw, &icons) != nil {
		return false
	}
	for _, item := range icons {
		icon, err := domainjsonstrict.DecodeRawObject(item, domainjsonstrict.Options{MaxBytes: 64 * 1024, MaxTokens: 1000, MaxStringBytes: 16 * 1024})
		if err != nil || !rawObjectHasOnly(icon, "src", "mimeType", "sizes", "theme") ||
			!rawJSONString(icon["src"]) || !optionalRawString(icon["mimeType"]) {
			return false
		}
		if sizes := icon["sizes"]; len(sizes) > 0 {
			var values []string
			if json.Unmarshal(sizes, &values) != nil {
				return false
			}
		}
		if theme := icon["theme"]; len(theme) > 0 {
			var value string
			if json.Unmarshal(theme, &value) != nil || (value != "light" && value != "dark") {
				return false
			}
		}
	}
	return true
}

func validContentAnnotations(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	annotations, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{MaxBytes: 64 * 1024, MaxTokens: 1000, MaxStringBytes: 4096})
	if err != nil || !rawObjectHasOnly(annotations, "audience", "priority", "lastModified") {
		return false
	}
	if audience := annotations["audience"]; len(audience) > 0 {
		var values []string
		if json.Unmarshal(audience, &values) != nil {
			return false
		}
		seen := map[string]bool{}
		for _, value := range values {
			if value != "user" && value != "assistant" || seen[value] {
				return false
			}
			seen[value] = true
		}
	}
	if priority := annotations["priority"]; len(priority) > 0 {
		var value float64
		if json.Unmarshal(priority, &value) != nil || value < 0 || value > 1 {
			return false
		}
	}
	if lastModified := annotations["lastModified"]; len(lastModified) > 0 {
		var value string
		if json.Unmarshal(lastModified, &value) != nil || strings.TrimSpace(value) != value {
			return false
		}
		if !validMCPDateTime(value) {
			return false
		}
	}
	return true
}

func validMCPDateTime(value string) bool {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04Z07:00"} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func validToolResultMeta(raw json.RawMessage) bool {
	meta, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024})
	if err != nil {
		return false
	}
	if token := meta["progressToken"]; len(token) > 0 && !rawJSONString(token) && !rawJSONInteger(token) {
		return false
	}
	if related := meta["io.modelcontextprotocol/related-task"]; len(related) > 0 {
		record, err := domainjsonstrict.DecodeRawObject(related, domainjsonstrict.Options{MaxBytes: 64 * 1024, MaxTokens: 1000, MaxStringBytes: 4096})
		if err != nil || !rawObjectHasOnly(record, "taskId") || !rawJSONString(record["taskId"]) {
			return false
		}
	}
	return true
}

func validEmbeddedResource(raw json.RawMessage) bool {
	resource, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{MaxBytes: 1024 * 1024, MaxTokens: 50_000, MaxStringBytes: 1024 * 1024})
	if err != nil || !rawObjectHasOnly(resource, "uri", "mimeType", "text", "blob", "_meta") || !rawNonEmptyString(resource["uri"]) || !optionalRawString(resource["mimeType"]) || !optionalRawObject(resource["_meta"]) {
		return false
	}
	textPresent := len(resource["text"]) > 0
	blobPresent := len(resource["blob"]) > 0
	return textPresent != blobPresent && (!textPresent || rawJSONString(resource["text"])) && (!blobPresent || rawBoundedBase64(resource["blob"]))
}

func rawBoundedBase64(raw json.RawMessage) bool {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == "" || len(value) > base64.StdEncoding.EncodedLen(maxDecodedMCPContentBytes) {
		return false
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) > 0 && len(decoded) <= maxDecodedMCPContentBytes
}

func rawContentMIMEType(raw json.RawMessage, expectedPrefix string) bool {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.HasPrefix(strings.ToLower(mediaType), expectedPrefix+"/")
}

func rawJSONBool(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false"))
}

func rawJSONInteger(raw json.RawMessage) bool {
	var value json.Number
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return false
	}
	_, err := value.Int64()
	return err == nil
}

func rawJSONString(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '"' {
		return false
	}
	var value string
	return json.Unmarshal(trimmed, &value) == nil
}

func rawJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	_, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{
		MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	return err == nil
}

func rawJSONArrayOfObjects(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return false
	}
	var value []json.RawMessage
	if json.Unmarshal(trimmed, &value) != nil {
		return false
	}
	for _, item := range value {
		if !rawJSONObject(item) {
			return false
		}
	}
	return true
}

func rawJSONObjectOrBoolOrString(raw json.RawMessage) bool {
	return rawJSONObject(raw) || rawJSONBool(raw) || rawJSONString(raw)
}

func rawJSONObjectOrString(raw json.RawMessage) bool {
	return rawJSONObject(raw) || rawJSONString(raw)
}
