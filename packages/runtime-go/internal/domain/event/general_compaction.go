package event

import (
	"math"
	"strconv"
	"strings"

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
	return ValidGeneralCompactionProviderHistoryItemV3(item) || ValidGeneralCompactionProviderHistoryItemV4(item)
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
