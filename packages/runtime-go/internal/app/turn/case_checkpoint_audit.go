package turn

import (
	"errors"
	"strings"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var errInvalidCaseCheckpointAudit = errors.New("case checkpoint audit event is invalid")

// ProjectCaseCheckpointAuditEvent validates and returns the closed
// metadata-only form accepted by case SSE/history projection. It accepts a
// host checkpoint summary or the already-sanitized durable form, and never
// retains a runtime checkpoint identifier, path, hash, content, workspace, or
// execution authority.
func ProjectCaseCheckpointAuditEvent(threadID string, event map[string]any) (map[string]any, bool) {
	projected, handled, err := sanitizeCaseCheckpointAuditEvent(threadID, event)
	if !handled || err != nil {
		return projected, false
	}
	payloadKey := caseCheckpointAuditPayloadKey(stringField(projected, "kind"))
	payload, _ := projected[payloadKey].(map[string]any)
	if payload != nil {
		publicPayload := contracts.CloneMap(payload)
		delete(publicPayload, "captureEventId")
		delete(publicPayload, "capturePayloadDigest")
		projected = contracts.CloneMap(projected)
		projected[payloadKey] = publicPayload
	}
	return projected, true
}

func sanitizeCaseCheckpointAuditEvent(expectedThreadID string, event map[string]any) (map[string]any, bool, error) {
	kind, kindOK := caseCheckpointAuditExactString(event, "kind")
	if !kindOK || !caseCheckpointAuditKind(kind) {
		return nil, false, nil
	}
	threadID, threadOK := caseCheckpointAuditExactString(event, "threadId")
	if !threadOK || expectedThreadID == "" || expectedThreadID != strings.TrimSpace(expectedThreadID) || threadID != expectedThreadID ||
		!caseCheckpointAuditOuterFieldsValid(event, kind) {
		return nil, true, errInvalidCaseCheckpointAudit
	}
	payloadKey := caseCheckpointAuditPayloadKey(kind)
	payload, payloadOK := event[payloadKey].(map[string]any)
	if !payloadOK || payload == nil {
		return nil, true, errInvalidCaseCheckpointAudit
	}
	if projectionKind, _ := payload["projectionKind"].(string); projectionKind != "" {
		metadata, ok := validateProjectedCaseCheckpointMetadata(kind, event, payload)
		if !ok {
			return nil, true, errInvalidCaseCheckpointAudit
		}
		return buildCaseCheckpointAuditEvent(event, kind, payloadKey, metadata), true, nil
	}
	metadata, ok := projectRawCaseCheckpointMetadata(kind, threadID, event, payload)
	if !ok {
		return nil, true, errInvalidCaseCheckpointAudit
	}
	return buildCaseCheckpointAuditEvent(event, kind, payloadKey, metadata), true, nil
}

func projectRawCaseCheckpointMetadata(kind, threadID string, event, payload map[string]any) (map[string]any, bool) {
	checkpointID, checkpointOK := caseCheckpointAuditExactString(payload, "checkpointId")
	nestedThreadID, nestedThreadOK := caseCheckpointAuditExactString(payload, "threadId")
	if !checkpointOK || !domaincheckpointref.IsCanonicalRuntimeIDV2(checkpointID) || !nestedThreadOK || nestedThreadID != threadID ||
		!caseCheckpointAuditSchemaV1(payload) || !caseCheckpointAuditRFC3339(payload, "createdAt") {
		return nil, false
	}
	turnID := ""
	status := ""
	metadata := map[string]any{}
	switch kind {
	case "checkpoint_captured":
		captureEventID, hasCaptureEventID := payload["captureEventId"].(string)
		_, hasCapturePayloadDigest := payload["capturePayloadDigest"]
		hasCaptureAuthority := hasCaptureEventID || hasCapturePayloadDigest
		legacyKeys := caseCheckpointAuditExactKeys(payload,
			"schemaVersion", "checkpointId", "threadId", "turnId", "createdAt", "status", "changedFileCount", "snapshotStorage")
		authorityKeys := caseCheckpointAuditExactKeys(payload,
			"schemaVersion", "checkpointId", "threadId", "turnId", "createdAt", "status", "changedFileCount", "snapshotStorage", "captureEventId", "capturePayloadDigest")
		if hasCaptureAuthority && (!authorityKeys || !hasCaptureEventID || !hasCapturePayloadDigest ||
			!domaincheckpointref.IsCaptureEventIDV2(captureEventID) || !domaincheckpointref.CapturedPayloadDigestMatches(payload)) ||
			!hasCaptureAuthority && !legacyKeys {
			return nil, false
		}
		var turnOK bool
		turnID, turnOK = caseCheckpointAuditExactString(event, "turnId")
		nestedTurnID, nestedTurnOK := caseCheckpointAuditExactString(payload, "turnId")
		status, _ = caseCheckpointAuditExactString(payload, "status")
		changedFileCount, countOK := caseCheckpointAuditNonNegativeInteger(payload["changedFileCount"])
		storage, storageOK := caseCheckpointAuditExactString(payload, "snapshotStorage")
		if !turnOK || !nestedTurnOK || nestedTurnID != turnID || status != "captured" || !countOK || storage != "runtime_private_cas" || !storageOK {
			return nil, false
		}
		metadata["changedFileCount"] = float64(changedFileCount)
		metadata["snapshotStorage"] = storage
		if hasCaptureAuthority {
			metadata["captureEventId"] = captureEventID
		}
	case "checkpoint_rewind_rescue_created":
		if !caseCheckpointAuditExactKeys(payload,
			"schemaVersion", "rescueId", "planId", "checkpointId", "threadId", "createdAt", "fileCount", "storage") {
			return nil, false
		}
		rescueID, rescueOK := caseCheckpointAuditExactString(payload, "rescueId")
		planID, planOK := caseCheckpointAuditExactString(payload, "planId")
		fileCount, countOK := caseCheckpointAuditNonNegativeInteger(payload["fileCount"])
		storage, storageOK := caseCheckpointAuditExactString(payload, "storage")
		if !rescueOK || !planOK || !countOK || !storageOK ||
			!caseCheckpointAuditOneOf(storage, "runtime_private_sidecar", "runtime_private_cas") ||
			!domaincheckpointref.LinkedIDMatches(domaincheckpointref.RescueLink, checkpointID, rescueID) ||
			!domaincheckpointref.LinkedIDMatches(domaincheckpointref.PlanLink, checkpointID, planID) {
			return nil, false
		}
		status = "created"
		metadata["fileCount"] = float64(fileCount)
	case "checkpoint_rewind_applied":
		if !caseCheckpointAuditExactKeys(payload,
			"schemaVersion", "applyId", "planId", "checkpointId", "threadId", "createdAt", "scope", "status", "destructive", "fileCount", "conversationStatus", "summary") {
			return nil, false
		}
		applyID, applyOK := caseCheckpointAuditExactString(payload, "applyId")
		planID, planOK := caseCheckpointAuditExactString(payload, "planId")
		status, _ = caseCheckpointAuditExactString(payload, "status")
		scope, scopeOK := caseCheckpointAuditExactString(payload, "scope")
		destructive, destructiveOK := payload["destructive"].(bool)
		fileCount, countOK := caseCheckpointAuditNonNegativeInteger(payload["fileCount"])
		conversationStatus, conversationOK := caseCheckpointAuditExactString(payload, "conversationStatus")
		summary, summaryOK := caseCheckpointAuditSummary(payload["summary"], fileCount)
		if !applyOK || !planOK || !scopeOK || !destructiveOK || !destructive || !countOK || !conversationOK || !summaryOK ||
			!domaincheckpointref.LinkedIDMatches(domaincheckpointref.ApplyLink, checkpointID, applyID) ||
			!domaincheckpointref.LinkedIDMatches(domaincheckpointref.PlanLink, checkpointID, planID) ||
			!caseCheckpointAuditOneOf(status, "applied", "blocked", "failed", "already_applied") ||
			!caseCheckpointAuditOneOf(scope, "code", "conversation", "combined") ||
			!caseCheckpointAuditOneOf(conversationStatus, "not_requested", "audit_recorded", "blocked", "already_applied") {
			return nil, false
		}
		metadata["scope"] = scope
		metadata["destructive"] = true
		metadata["fileCount"] = float64(fileCount)
		metadata["conversationStatus"] = conversationStatus
		metadata["summary"] = summary
	default:
		return nil, false
	}
	metadata["schemaVersion"] = float64(1)
	metadata["projectionKind"] = "checkpoint_status"
	metadata["disclosure"] = "metadata_only"
	metadata["status"] = status
	metadata["checkpointRefDigest"] = domaincheckpointref.PublicReferenceDigest(threadID, turnID, checkpointID)
	metadata["privatePayloadWithheld"] = true
	metadata["factAnswerAllowed"] = false
	metadata["evidenceAuthority"] = false
	if captureEventID, ok := metadata["captureEventId"].(string); ok && captureEventID != "" {
		metadata["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(metadata)
	}
	return metadata, true
}

func validateProjectedCaseCheckpointMetadata(kind string, event, metadata map[string]any) (map[string]any, bool) {
	status, statusOK := caseCheckpointAuditExactString(metadata, "status")
	disclosure, disclosureOK := caseCheckpointAuditExactString(metadata, "disclosure")
	projectionKind, projectionOK := caseCheckpointAuditExactString(metadata, "projectionKind")
	refDigest, digestOK := caseCheckpointAuditExactString(metadata, "checkpointRefDigest")
	privateWithheld, privateOK := metadata["privatePayloadWithheld"].(bool)
	factAllowed, factOK := metadata["factAnswerAllowed"].(bool)
	evidenceAuthority, evidenceOK := metadata["evidenceAuthority"].(bool)
	if !caseCheckpointAuditSchemaV1(metadata) || !statusOK || !disclosureOK || disclosure != "metadata_only" ||
		!projectionOK || projectionKind != "checkpoint_status" || !digestOK || !domainsecurity.IsSHA256Hex(refDigest) ||
		!privateOK || !privateWithheld || !factOK || factAllowed || !evidenceOK || evidenceAuthority {
		return nil, false
	}
	out := map[string]any{}
	switch kind {
	case "checkpoint_captured":
		captureEventID, hasCaptureEventID := metadata["captureEventId"].(string)
		_, hasCapturePayloadDigest := metadata["capturePayloadDigest"]
		hasCaptureAuthority := hasCaptureEventID || hasCapturePayloadDigest
		legacyKeys := caseCheckpointAuditExactKeys(metadata,
			"schemaVersion", "projectionKind", "disclosure", "status", "checkpointRefDigest", "privatePayloadWithheld", "factAnswerAllowed", "evidenceAuthority", "changedFileCount", "snapshotStorage")
		authorityKeys := caseCheckpointAuditExactKeys(metadata,
			"schemaVersion", "projectionKind", "disclosure", "status", "checkpointRefDigest", "privatePayloadWithheld", "factAnswerAllowed", "evidenceAuthority", "changedFileCount", "snapshotStorage", "captureEventId", "capturePayloadDigest")
		if hasCaptureAuthority && (!authorityKeys || !hasCaptureEventID || !hasCapturePayloadDigest ||
			!domaincheckpointref.IsCaptureEventIDV2(captureEventID) || !domaincheckpointref.CapturedPayloadDigestMatches(metadata)) ||
			!hasCaptureAuthority && !legacyKeys {
			return nil, false
		}
		_, turnOK := caseCheckpointAuditExactString(event, "turnId")
		count, countOK := caseCheckpointAuditNonNegativeInteger(metadata["changedFileCount"])
		storage, storageOK := caseCheckpointAuditExactString(metadata, "snapshotStorage")
		if !turnOK || !countOK || !storageOK || storage != "runtime_private_cas" || status != "captured" {
			return nil, false
		}
		out["changedFileCount"] = float64(count)
		out["snapshotStorage"] = storage
		if hasCaptureAuthority {
			out["captureEventId"] = captureEventID
		}
	case "checkpoint_rewind_rescue_created":
		if !caseCheckpointAuditExactKeys(metadata,
			"schemaVersion", "projectionKind", "disclosure", "status", "checkpointRefDigest", "privatePayloadWithheld", "factAnswerAllowed", "evidenceAuthority", "fileCount") {
			return nil, false
		}
		count, countOK := caseCheckpointAuditNonNegativeInteger(metadata["fileCount"])
		if !countOK || status != "created" {
			return nil, false
		}
		out["fileCount"] = float64(count)
	case "checkpoint_rewind_applied":
		if !caseCheckpointAuditExactKeys(metadata,
			"schemaVersion", "projectionKind", "disclosure", "status", "checkpointRefDigest", "privatePayloadWithheld", "factAnswerAllowed", "evidenceAuthority", "scope", "destructive", "fileCount", "conversationStatus", "summary") {
			return nil, false
		}
		scope, scopeOK := caseCheckpointAuditExactString(metadata, "scope")
		destructive, destructiveOK := metadata["destructive"].(bool)
		fileCount, countOK := caseCheckpointAuditNonNegativeInteger(metadata["fileCount"])
		conversationStatus, conversationOK := caseCheckpointAuditExactString(metadata, "conversationStatus")
		summary, summaryOK := caseCheckpointAuditSummary(metadata["summary"], fileCount)
		if !scopeOK || !destructiveOK || !destructive || !countOK || !conversationOK || !summaryOK ||
			!caseCheckpointAuditOneOf(status, "applied", "blocked", "failed", "already_applied") ||
			!caseCheckpointAuditOneOf(scope, "code", "conversation", "combined") ||
			!caseCheckpointAuditOneOf(conversationStatus, "not_requested", "audit_recorded", "blocked", "already_applied") {
			return nil, false
		}
		out["scope"] = scope
		out["destructive"] = true
		out["fileCount"] = float64(fileCount)
		out["conversationStatus"] = conversationStatus
		out["summary"] = summary
	default:
		return nil, false
	}
	out["schemaVersion"] = float64(1)
	out["projectionKind"] = "checkpoint_status"
	out["disclosure"] = "metadata_only"
	out["status"] = status
	out["checkpointRefDigest"] = refDigest
	out["privatePayloadWithheld"] = true
	out["factAnswerAllowed"] = false
	out["evidenceAuthority"] = false
	if captureEventID, ok := out["captureEventId"].(string); ok && captureEventID != "" {
		out["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(out)
		if out["capturePayloadDigest"] != metadata["capturePayloadDigest"] {
			return nil, false
		}
	}
	return out, true
}

func buildCaseCheckpointAuditEvent(event map[string]any, kind, payloadKey string, metadata map[string]any) map[string]any {
	out := map[string]any{"kind": kind, "threadId": event["threadId"], payloadKey: metadata}
	if kind == "checkpoint_captured" {
		out["turnId"] = event["turnId"]
	}
	if seq, present := event["seq"]; present {
		out["seq"] = seq
	}
	if timestamp, present := event["timestamp"]; present {
		out["timestamp"] = timestamp
	}
	return out
}

func caseCheckpointAuditOuterFieldsValid(event map[string]any, kind string) bool {
	allowed := []string{"kind", "threadId", caseCheckpointAuditPayloadKey(kind)}
	if kind == "checkpoint_captured" {
		allowed = append(allowed, "turnId")
	}
	if _, present := event["seq"]; present {
		if _, ok := caseCheckpointAuditNonNegativeInteger(event["seq"]); !ok {
			return false
		}
		allowed = append(allowed, "seq")
	}
	if _, present := event["timestamp"]; present {
		timestamp, ok := caseCheckpointAuditExactString(event, "timestamp")
		if !ok {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			return false
		}
		allowed = append(allowed, "timestamp")
	}
	return caseCheckpointAuditExactKeys(event, allowed...)
}

func caseCheckpointAuditSummary(value any, expectedTotal int) (map[string]any, bool) {
	summary, ok := value.(map[string]any)
	if !ok || !caseCheckpointAuditExactKeys(summary,
		"fileAppliedCount", "fileNoopCount", "fileManualReviewCount", "fileBlockedCount", "fileFailedCount") {
		return nil, false
	}
	out := map[string]any{}
	total := 0
	for _, key := range []string{"fileAppliedCount", "fileNoopCount", "fileManualReviewCount", "fileBlockedCount", "fileFailedCount"} {
		count, countOK := caseCheckpointAuditNonNegativeInteger(summary[key])
		if !countOK {
			return nil, false
		}
		out[key] = float64(count)
		total += count
	}
	return out, total == expectedTotal
}

func caseCheckpointAuditExactKeys(record map[string]any, keys ...string) bool {
	if record == nil || len(record) != len(keys) {
		return false
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for key := range record {
		if _, ok := allowed[key]; !ok {
			return false
		}
	}
	return true
}

func caseCheckpointAuditExactString(record map[string]any, key string) (string, bool) {
	if record == nil {
		return "", false
	}
	value, ok := record[key].(string)
	return value, ok && value != "" && value == strings.TrimSpace(value)
}

func caseCheckpointAuditSchemaV1(record map[string]any) bool {
	value, ok := caseCheckpointAuditNonNegativeInteger(record["schemaVersion"])
	return ok && value == 1
}

func caseCheckpointAuditNonNegativeInteger(value any) (int, bool) {
	parsed, ok := contracts.NumericSeq(value)
	return parsed, ok && parsed >= 0
}

func caseCheckpointAuditRFC3339(record map[string]any, key string) bool {
	value, ok := caseCheckpointAuditExactString(record, key)
	if !ok {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func caseCheckpointAuditOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func caseCheckpointAuditKind(kind string) bool {
	return kind == "checkpoint_captured" || kind == "checkpoint_rewind_rescue_created" || kind == "checkpoint_rewind_applied"
}

func caseCheckpointAuditPayloadKey(kind string) string {
	switch kind {
	case "checkpoint_captured":
		return "checkpoint"
	case "checkpoint_rewind_rescue_created":
		return "rescue"
	case "checkpoint_rewind_applied":
		return "apply"
	default:
		return ""
	}
}
