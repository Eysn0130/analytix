package event

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

const GeneralCompactionSummaryTextV3 = "Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded."

// ValidGeneralCompactionProviderHistoryItemV3 accepts only the fixed host
// boundary summary. Arbitrary caller-authored compaction prose is rejected even
// when a caller can recompute an unkeyed reasoning-exclusion hash.
func ValidGeneralCompactionProviderHistoryItemV3(item map[string]any) bool {
	sourceDigest := generalCompactionString(item, "sourceDigest")
	if item == nil || generalCompactionString(item, "kind") != "compaction" ||
		generalCompactionString(item, "role") != "system" || generalCompactionString(item, "status") != "completed" ||
		generalCompactionString(item, "summary") != GeneralCompactionSummaryTextV3 ||
		generalCompactionString(item, "id") == "" || generalCompactionString(item, "threadId") == "" ||
		generalCompactionString(item, "turnId") == "" || generalCompactionString(item, "createdAt") == "" ||
		generalCompactionString(item, "finishedAt") == "" || generalCompactionNumeric(item["schemaVersion"]) != 3 ||
		generalCompactionNumeric(item["providerHistoryProjectionVersion"]) != 1 || item["reasoningExcluded"] != true ||
		item["assistantProseExcluded"] != true || item["toolPayloadsExcluded"] != true || item["caseFactsExcluded"] != true ||
		!domainsecurity.IsSHA256Hex(sourceDigest) || generalCompactionString(item, "digestMarker") != "sha256:"+sourceDigest[:12] ||
		ValidatePublicRecord(item) != nil {
		return false
	}
	allowed := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true, "createdAt": true, "finishedAt": true,
		"kind": true, "summary": true, "replacedTokens": true, "auto": true, "pinnedConstraints": true, "sourceDigest": true,
		"digestMarker": true, "sourceItemIds": true, "schemaVersion": true, "reasoningExcluded": true,
		"reasoningExclusionProof": true, "assistantProseExcluded": true, "toolPayloadsExcluded": true,
		"caseFactsExcluded": true, "providerHistoryProjectionVersion": true,
	}
	for key := range item {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func ValidGeneralCompactionProviderHistoryItem(item map[string]any) bool {
	return ValidGeneralCompactionProviderHistoryItemV3(item) || ValidGeneralCompactionProviderHistoryItemV4(item) || ValidGeneralCompactionProviderHistoryItemV5(item)
}

func CompactionSummaryFactsFromItemV1(item map[string]any) threaddomain.CompactionSummaryFactsV1 {
	var ids []string
	switch values := item["sourceItemIds"].(type) {
	case []any:
		for _, raw := range values {
			value, ok := raw.(string)
			if !ok {
				return threaddomain.CompactionSummaryFactsV1{}
			}
			ids = append(ids, value)
		}
	case []string:
		ids = append(ids, values...)
	default:
		return threaddomain.CompactionSummaryFactsV1{}
	}
	return threaddomain.CompactionSummaryFactsV1{
		CompactionID: generalCompactionString(item, "compactionId"), Summary: rawGeneralCompactionString(item, "summary"),
		SummaryDigest: generalCompactionString(item, "summaryDigest"), SourceDigest: generalCompactionString(item, "sourceDigest"),
		CheckpointDigest: generalCompactionString(item, "checkpointDigest"), SourceItemIDs: ids,
		ShadowedItemCount: generalCompactionNumeric(item["shadowedItemCount"]), ReplacedTokens: generalCompactionNumeric(item["replacedTokens"]),
		FinishedAt: generalCompactionString(item, "finishedAt"),
	}
}

// V5 is a complete ordinary conversation summary bound to its selection and
// checkpoint. It is historical data; it grants no evidence or tool authority.
func ValidGeneralCompactionProviderHistoryItemV5(item map[string]any) bool {
	facts := CompactionSummaryFactsFromItemV1(item)
	created, err := time.Parse(time.RFC3339Nano, generalCompactionString(item, "createdAt"))
	finished, finishErr := time.Parse(time.RFC3339Nano, facts.FinishedAt)
	if item == nil || threaddomain.ValidateCompactionSummaryFactsV1(facts) != nil ||
		generalCompactionString(item, "id") != facts.CompactionID || !threaddomain.IsCanonicalRecordID(generalCompactionString(item, "threadId")) ||
		!threaddomain.IsCanonicalRecordID(generalCompactionString(item, "turnId")) || err != nil || finishErr != nil || finished.Before(created) ||
		generalCompactionString(item, "kind") != "compaction" || generalCompactionString(item, "role") != "system" ||
		generalCompactionString(item, "status") != "completed" || generalCompactionNumeric(item["schemaVersion"]) != 5 ||
		generalCompactionNumeric(item["providerHistoryProjectionVersion"]) != 3 || generalCompactionString(item, "summaryKind") != "conversation" ||
		generalCompactionString(item, "tokenCountKind") != "estimated" || generalCompactionNumeric(item["tokenEstimatorVersion"]) != 1 ||
		item["reasoningExcluded"] != true || item["caseFactsExcluded"] != true || item["toolPayloadsExcluded"] != true ||
		!domainsecurity.IsSHA256Hex(facts.SourceDigest) || generalCompactionString(item, "digestMarker") != "sha256:"+facts.SourceDigest[:12] ||
		ValidatePublicRecord(item) != nil {
		return false
	}
	auto, autoOK := item["auto"].(bool)
	_, continuationPresent := item["taskContinuation"]
	if !autoOK || auto != continuationPresent {
		return false
	}
	if auto {
		if _, err := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"]); err != nil {
			return false
		}
	}
	constraints, ok := item["pinnedConstraints"].([]any)
	if !ok || len(constraints) != 1 || constraints[0] != "user: preserve recent turns" {
		return false
	}
	allowed := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true, "createdAt": true, "finishedAt": true,
		"kind": true, "summary": true, "replacedTokens": true, "auto": true, "pinnedConstraints": true, "sourceDigest": true,
		"digestMarker": true, "sourceItemIds": true, "schemaVersion": true, "reasoningExcluded": true, "reasoningExclusionProof": true,
		"toolPayloadsExcluded": true, "caseFactsExcluded": true, "providerHistoryProjectionVersion": true, "taskContinuation": true,
		"compactionId": true, "summaryKind": true, "summaryDigest": true, "checkpointDigest": true, "shadowedItemCount": true,
		"tokenCountKind": true, "tokenEstimatorVersion": true,
	}
	for key := range item {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func rawGeneralCompactionString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

// ValidGeneralCompactionProviderHistoryItemV4 admits only host-created
// automatic compactions with a digest-bound, non-authoritative continuation.
func ValidGeneralCompactionProviderHistoryItemV4(item map[string]any) bool {
	sourceDigest := generalCompactionString(item, "sourceDigest")
	continuation, continuationErr := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
	if item == nil || generalCompactionString(item, "kind") != "compaction" ||
		generalCompactionString(item, "role") != "system" || generalCompactionString(item, "status") != "completed" ||
		generalCompactionString(item, "summary") != GeneralCompactionSummaryTextV3 || item["auto"] != true ||
		generalCompactionString(item, "id") == "" || generalCompactionString(item, "threadId") == "" ||
		generalCompactionString(item, "turnId") == "" || generalCompactionString(item, "createdAt") == "" ||
		generalCompactionString(item, "finishedAt") == "" || generalCompactionNumeric(item["schemaVersion"]) != 4 ||
		generalCompactionNumeric(item["providerHistoryProjectionVersion"]) != 2 || item["reasoningExcluded"] != true ||
		item["assistantProseExcluded"] != true || item["toolPayloadsExcluded"] != true || item["caseFactsExcluded"] != true ||
		!domainsecurity.IsSHA256Hex(sourceDigest) || generalCompactionString(item, "digestMarker") != "sha256:"+sourceDigest[:12] ||
		continuationErr != nil || continuation.EvidenceAuthority != threaddomain.TaskContinuationEvidenceStateV1 ||
		ValidatePublicRecord(item) != nil {
		return false
	}
	allowed := map[string]bool{
		"id": true, "turnId": true, "threadId": true, "role": true, "status": true, "createdAt": true, "finishedAt": true,
		"kind": true, "summary": true, "replacedTokens": true, "auto": true, "pinnedConstraints": true, "sourceDigest": true,
		"digestMarker": true, "sourceItemIds": true, "schemaVersion": true, "reasoningExcluded": true,
		"reasoningExclusionProof": true, "assistantProseExcluded": true, "toolPayloadsExcluded": true,
		"caseFactsExcluded": true, "providerHistoryProjectionVersion": true, "taskContinuation": true,
	}
	for key := range item {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func generalCompactionString(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func generalCompactionNumeric(value any) int {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil && parsed >= 0 && parsed <= math.MaxInt && !strings.HasPrefix(typed.String(), "-") {
			return int(parsed)
		}
	case int:
		return typed
	case int64:
		if typed >= math.MinInt && typed <= math.MaxInt {
			return int(typed)
		}
	case float64:
		if !math.IsNaN(typed) && typed >= float64(math.MinInt) && typed < -float64(math.MinInt) && math.Trunc(typed) == typed {
			parsed, err := strconv.Atoi(strconv.FormatFloat(typed, 'f', 0, 64))
			if err == nil {
				return parsed
			}
		}
	}
	return -1
}
