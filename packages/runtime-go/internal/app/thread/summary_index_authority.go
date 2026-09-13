package thread

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// ValidateSummaryIndexRecordV1 owns the complete index envelope. An index is
// only a lookup hint, but its typed metadata must not become prose merely by
// being nested under a field named summary. The validation view is detached;
// neither persisted bytes nor the canonical thread are rewritten here.
func ValidateSummaryIndexRecordV1(record map[string]any) error {
	invalid := errors.New("thread summary index record is outside the closed projection")
	version, ok := contracts.NumericSeq(record["schemaVersion"])
	id, idOK := record["threadId"].(string)
	summary, summaryOK := record["summary"].(map[string]any)
	if !ok || version != 1 || !idOK || !domainthread.IsCanonicalRecordID(id) || !summaryOK || summary["id"] != id {
		return invalid
	}
	for key, value := range record {
		switch key {
		case "schemaVersion", "threadId", "summary":
		case "updatedAt", "writtenAt":
			if !summaryIndexTimestampV1(value) {
				return invalid
			}
		case "deleted":
			if _, ok := value.(bool); !ok {
				return invalid
			}
		default:
			return invalid
		}
	}
	if err := domainsecret.ValidateValueV1(record); err != nil {
		return err
	}
	view := contracts.CloneMap(record)
	content, ok := view["summary"].(map[string]any)
	if !ok {
		return invalid
	}
	for key, value := range summary {
		switch key {
		case "id", "parentThreadId", "forkedFromThreadId", "caseProjectId", "caseId", "latestTurnId":
			text, ok := value.(string)
			if !ok || (text != "" && !domainthread.IsCanonicalRecordID(text)) {
				return invalid
			}
			delete(content, key)
		case "createdAt", "updatedAt", "forkedAt":
			if !summaryIndexTimestampV1(value) {
				return invalid
			}
			delete(content, key)
		case "turnCount", "messageCount", "forkedFromMessageCount", "forkedFromTurnCount", "executionPolicyVersion":
			if count, ok := contracts.NumericSeq(value); !ok || count < 0 {
				return invalid
			}
		case "archived", "pinned", "hasRunningTurn":
			if _, ok := value.(bool); !ok {
				return invalid
			}
		case "title", "workspace", "model", "providerId", "mode", "status", "approvalPolicy", "sandboxMode", "relation", "forkedFromTitle", "preview", "historyAuthority":
			if _, ok := value.(string); !ok {
				return invalid
			}
		case "goal", "todos":
			if err := summaryIndexStateMetadataViewV1(content, key, id); err != nil {
				return err
			}
		default:
			return invalid
		}
	}
	return domainevent.ValidatePublicRecord(view)
}

func summaryIndexStateMetadataViewV1(summary map[string]any, key, threadID string) error {
	if summary[key] == nil {
		return nil
	}
	state, ok := summary[key].(map[string]any)
	if !ok {
		return errors.New("thread summary state is invalid")
	}
	if key == "goal" {
		closed, ok := projectOrdinaryPublicGoalV1(contracts.CloneMap(state))
		if !ok || !reflect.DeepEqual(state, closed) {
			return errors.New("thread summary goal is outside the closed projection")
		}
	} else {
		if err := validateAllowedKeys(state, map[string]bool{"threadId": true, "items": true, "updatedAt": true}); err != nil {
			return errors.New("thread summary todos are outside the closed projection")
		}
	}
	if state["threadId"] != threadID {
		return errors.New("thread summary state belongs to another thread")
	}
	if err := summaryIndexMetadataViewV1(state, []string{"id", "threadId", "blockedTurnId", "selfCheckTurnId"}); err != nil {
		return err
	}
	if key == "goal" {
		ledger, _ := state["evidenceLedger"].([]any)
		for _, raw := range ledger {
			entry := raw.(map[string]any) // The closed goal projection proved each entry.
			if err := summaryIndexMetadataViewV1(entry, []string{"id", "turnId", "toolCallId", "requirementId"}); err != nil {
				return err
			}
		}
		return nil
	}
	items, ok := state["items"].([]any)
	if !ok || len(items) > MaxTodoItems {
		return errors.New("thread summary todo items are invalid")
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || validateAllowedTodoKeys(item) != nil {
			return errors.New("thread summary todo item is outside the closed projection")
		}
		if err := summaryIndexMetadataViewV1(item, []string{"id", "parentTodoRef"}); err != nil {
			return err
		}
		if raw, present := item["evidenceIds"]; present {
			ids, err := normalizeTodoEvidenceIDs(raw)
			if err != nil || !reflect.DeepEqual(raw, ids) {
				return errors.New("thread summary todo evidence identities are invalid")
			}
			canonical := true
			for _, id := range ids {
				canonical = canonical && domainthread.IsCanonicalRecordID(id.(string))
			}
			if canonical {
				delete(item, "evidenceIds")
			}
		}
		if raw, present := item["source"]; present {
			source, err := normalizeTodoSource(raw)
			if err != nil || !reflect.DeepEqual(raw, source) {
				return errors.New("thread summary todo source is outside the closed projection")
			}
			item["source"] = source
			if err := summaryIndexMetadataViewV1(source, []string{"planId", "parentThreadId", "childThreadId", "childRunId", "jobId", "projectionId"}); err != nil {
				return err
			}
			if digest, ok := source["contentHash"].(string); ok && strings.TrimSpace(digest) == digest && domainsecurity.IsSHA256Hex(digest) {
				delete(source, "contentHash")
			}
		}
	}
	return nil
}

func summaryIndexMetadataViewV1(record map[string]any, ids []string) error {
	for _, key := range []string{"createdAt", "updatedAt"} {
		if value, present := record[key]; present {
			if !summaryIndexTimestampV1(value) {
				return errors.New("thread summary timestamp is invalid")
			}
			delete(record, key)
		}
	}
	for _, key := range ids {
		if text, ok := record[key].(string); ok && domainthread.IsCanonicalRecordID(text) {
			delete(record, key)
		}
	}
	return nil
}

func summaryIndexTimestampV1(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	// Older index rows may carry an absent timestamp as an empty string.
	if text == "" {
		return true
	}
	_, err := time.Parse(time.RFC3339Nano, text)
	return err == nil
}

// RehydrateSummaryIndex treats an index as an ID/order hint only. Every
// returned projection is rebuilt from the current durable thread so stale or
// tampered summary text cannot become read authority.
func RehydrateSummaryIndex(
	indexed []map[string]any,
	readThread func(string) (map[string]any, error),
) ([]map[string]any, error) {
	if readThread == nil {
		return nil, errors.New("summary index authority dependencies are required")
	}
	summaries := make([]map[string]any, 0, len(indexed))
	for _, indexedSummary := range indexed {
		threadID := strings.TrimSpace(contracts.StringField(indexedSummary, "id"))
		if threadID == "" || contracts.SafeRecordID(threadID) != threadID {
			return nil, errors.New("thread summary index contains an invalid thread id")
		}
		thread, err := readThread(threadID)
		if err != nil {
			return nil, fmt.Errorf("read indexed thread %s: %w", threadID, err)
		}
		if thread == nil || strings.TrimSpace(contracts.StringField(thread, "id")) != threadID {
			return nil, fmt.Errorf("indexed thread %s is missing or has mismatched identity", threadID)
		}
		summaries = append(summaries, SummaryIndexProjection(thread))
	}
	return summaries, nil
}

func SummaryIndexProjection(thread map[string]any) map[string]any {
	summary := contracts.ThreadSummary(thread)
	// A durable goal may own private research/evidence details that the public
	// goal contract deliberately omits. Index hints use that existing public
	// projection; the canonical goal stays intact. Invalid goals remain for the
	// complete row validator to reject rather than silently losing state.
	if goal, ok := projectOrdinaryPublicGoalV1(summary["goal"]); ok {
		summary["goal"] = goal
	}
	for _, key := range []string{"archived", "pinned", "caseProjectId", "caseId"} {
		if value, ok := thread[key]; ok {
			summary[key] = contracts.CloneValue(value)
		}
	}
	turns, _ := thread["turns"].([]any)
	for index := len(turns) - 1; index >= 0; index-- {
		turn, _ := turns[index].(map[string]any)
		if turnID := strings.TrimSpace(contracts.StringField(turn, "id")); turnID != "" {
			summary["latestTurnId"] = turnID
			break
		}
	}
	if strings.EqualFold(contracts.StringField(thread, "status"), "running") || threadHasRunningTurn(turns) {
		summary["hasRunningTurn"] = true
	}
	status := strings.TrimSpace(contracts.StringField(summary, "status"))
	if strings.EqualFold(status, "archived") {
		summary["archived"] = true
	}
	return summary
}

func threadHasRunningTurn(turns []any) bool {
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		status := strings.TrimSpace(contracts.StringField(turn, "status"))
		if status == "running" || status == "queued" {
			return true
		}
	}
	return false
}
