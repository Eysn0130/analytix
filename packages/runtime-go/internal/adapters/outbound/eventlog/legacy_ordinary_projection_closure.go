package eventlog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	historymigrationapp "analytix.local/runtime-go/internal/app/historymigration"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

const LegacyOrdinaryProjectionClosureVersion = 1

type LegacyOrdinaryProjectionClosureInput struct {
	Root                string
	RestartPreservation *SemanticRestartPreservationV1
}

// preflightSemanticStartupCurrentAuthorityV1 runs before any semantic content
// transform. It prevents an invalid current V2 container from being stripped
// into a legacy-looking record and validates malformed authority-bearing event
// bytes before the execution-authority migration can tombstone them.
func preflightSemanticStartupCurrentAuthorityV1(root string) error {
	threadsDir := filepath.Join(strings.TrimSpace(root), "threads")
	entries, err := os.ReadDir(threadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for ordinal, entry := range entries {
		if !entry.IsDir() || contracts.SafeRecordID(entry.Name()) != entry.Name() {
			return errors.New("semantic authority preflight found an invalid thread directory")
		}
		threadDir := filepath.Join(threadsDir, entry.Name())
		if err := preflightReasoningEventAuthorityV1(filepath.Join(threadDir, "events.jsonl")); err != nil {
			return fmt.Errorf("semantic authority event preflight ordinal %d: %w", ordinal, err)
		}
		body, readErr := os.ReadFile(filepath.Join(threadDir, "thread.json"))
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		thread, decodeErr := strictMigrationObject(body, 64*1024*1024)
		if decodeErr != nil {
			return fmt.Errorf("semantic authority thread preflight ordinal %d: %w", ordinal, decodeErr)
		}
		threadID := strings.TrimSpace(migrationStringField(thread, "id"))
		if threadID != entry.Name() {
			return errors.New("semantic authority preflight thread identity mismatch")
		}
		if !semanticTypedCurrentAuthorityContainerV1(thread) || semanticDerivedThreadV1(thread, threadID) {
			continue
		}
		if err := validateCurrentAuthorityThreadPrivacyV1(thread); err != nil {
			return fmt.Errorf("semantic authority thread preflight ordinal %d: %w", ordinal, err)
		}
		if err := preflightCurrentExecutionEventsV1(filepath.Join(threadDir, "events.jsonl"), thread); err != nil {
			return fmt.Errorf("semantic authority event binding preflight ordinal %d: %w", ordinal, err)
		}
	}
	return nil
}

func preflightCurrentExecutionEventsV1(path string, thread map[string]any) error {
	_, _, err := readStrictMigrationRecords(path, func(record map[string]any, line int, _ string) (map[string]any, bool, error) {
		if !historymigrationapp.EventAuthorityBelongsToThread(thread, record) {
			return nil, false, fmt.Errorf("current execution event authority is invalid at line %d", line)
		}
		return record, false, nil
	})
	return err
}

func semanticDerivedThreadV1(thread map[string]any, threadID string) bool {
	source := strings.TrimSpace(migrationStringField(thread, "forkedFromThreadId"))
	return source != "" && source != strings.TrimSpace(threadID)
}

func semanticTypedCurrentAuthorityContainerV1(thread map[string]any) bool {
	for _, key := range []string{"securityState", "contextEpochState", "acceptedFinal", "acceptedFinalView"} {
		if _, present := thread[key]; present {
			return true
		}
	}
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		for _, key := range []string{"securityContext", "contextEpochSnapshot", "acceptedFinal", "acceptedFinalView"} {
			if _, present := turn[key]; present {
				return true
			}
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if item == nil {
				continue
			}
			for _, key := range []string{"executionGrant", "hostEvidenceSettlement", "approvalTransition", "acceptedFinal", "acceptedFinalView"} {
				if _, present := item[key]; present {
					return true
				}
			}
		}
	}
	return false
}

// MigrateLegacyOrdinaryProjectionClosure is the last content transform in the
// staged semantic-startup transaction. The preceding lineage freeze already
// captured any legitimate legacy parent-call tuple. This pass can therefore
// retire provider-originated tool history that has no host authority, remove
// non-hydratable sidecar tombstones, and prove a closed ordinary projection
// without minting a tool identity, grant, receipt, citation, or final answer.
//
// Current execution/publication authority is never rewritten here. It must
// already satisfy the current privacy contract or startup fails closed.
func MigrateLegacyOrdinaryProjectionClosure(input LegacyOrdinaryProjectionClosureInput) error {
	if err := input.RestartPreservation.revalidate(context.Background(), input.Root, ""); err != nil {
		return err
	}
	threadsDir := filepath.Join(strings.TrimSpace(input.Root), "threads")
	entries, err := os.ReadDir(threadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for ordinal, entry := range entries {
		if !entry.IsDir() || contracts.SafeRecordID(entry.Name()) != entry.Name() {
			return errors.New("legacy ordinary projection closure found an invalid thread directory")
		}
		if input.RestartPreservation.ownsThread(entry.Name()) {
			continue
		}
		threadDir := filepath.Join(threadsDir, entry.Name())
		if err := closeLegacyOrdinaryProjectionDirectoryV1(threadDir, entry.Name()); err != nil {
			return fmt.Errorf("legacy ordinary projection closure ordinal %d: %w", ordinal, err)
		}
	}
	return nil
}

func validateLegacyOrdinaryProjectionFixedPointV1(root string) error {
	return validatePreservedLegacyOrdinaryProjectionFixedPointV1(root, nil)
}

func validatePreservedLegacyOrdinaryProjectionFixedPointV1(root string, preserved *SemanticRestartPreservationV1) error {
	// Held original bytes must remain valid and unchanged. They cannot satisfy
	// a fixed point that would require projecting away their private authority.
	if err := preserved.revalidate(context.Background(), root, ""); err != nil {
		return err
	}
	threadsDir := filepath.Join(strings.TrimSpace(root), "threads")
	entries, err := os.ReadDir(threadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for ordinal, entry := range entries {
		if !entry.IsDir() || contracts.SafeRecordID(entry.Name()) != entry.Name() {
			return errors.New("closed ordinary fixed-point found an invalid thread directory")
		}
		if preserved.ownsThread(entry.Name()) {
			continue
		}
		if err := validateClosedLegacyOrdinaryDirectoryV1(filepath.Join(threadsDir, entry.Name()), entry.Name()); err != nil {
			return fmt.Errorf("closed ordinary fixed-point ordinal %d: %w", ordinal, err)
		}
	}
	return nil
}

func closeLegacyOrdinaryProjectionDirectoryV1(threadDir, threadID string) error {
	threadPath := filepath.Join(threadDir, "thread.json")
	thread, threadChanged, err := prepareLegacyOrdinaryPrimaryClosureV1(threadPath, threadID)
	if err != nil {
		return err
	}
	messagesPath := filepath.Join(threadDir, "messages.jsonl")
	messages, messagesChanged, err := prepareLegacyOrdinaryMessagesClosureV1(messagesPath)
	if err != nil {
		return err
	}

	// Both projections are fully prepared before either file is replaced. The
	// outer signed semantic-startup journal owns cross-directory rollback.
	if threadChanged {
		if err := writeMigrationJSON(threadPath, thread); err != nil {
			return err
		}
	}
	if messagesChanged {
		if err := filestore.WriteJSONLFileAtomic(messagesPath, ".messages-ordinary-projection-closure-*.tmp", messages); err != nil {
			return err
		}
	}
	return validateClosedLegacyOrdinaryDirectoryV1(threadDir, threadID)
}

func prepareLegacyOrdinaryPrimaryClosureV1(path, threadID string) (map[string]any, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	thread, err := strictMigrationObject(body, 64*1024*1024)
	if err != nil {
		return nil, false, fmt.Errorf("decode primary thread: %w", err)
	}
	if strings.TrimSpace(migrationStringField(thread, "id")) != threadID {
		return nil, false, errors.New("primary thread identity mismatch")
	}
	if domainstartup.ContainsCurrentEventOrderAuthorityV1(thread) {
		if err := validateCurrentAuthorityThreadPrivacyV1(thread); err != nil {
			return nil, false, fmt.Errorf("current authority thread is invalid: %w", err)
		}
		return thread, false, nil
	}
	projected, changed, err := closeLegacyOrdinaryThreadItemsV1(thread, threadID)
	if err != nil {
		return nil, false, err
	}
	if _, err := threadapp.ProjectPublicThread(projected); err != nil {
		return nil, false, fmt.Errorf("closed primary projection is invalid: %w", err)
	}
	return projected, changed || !sameMigrationJSON(thread, projected), nil
}

func closeLegacyOrdinaryThreadItemsV1(thread map[string]any, threadID string) (map[string]any, bool, error) {
	projected := contracts.CloneMap(thread)
	// A legacy ordinary snapshot has no frozen TurnSecurityContext and cannot
	// establish continuation or execution authority after restart. Close its
	// public lifecycle in the isolated migration stage instead of asking the
	// runtime restore path to invent authority for an old in-flight turn.
	// Current authority containers never reach this function.
	changed := false
	if strings.TrimSpace(migrationStringField(projected, "status")) == "running" {
		projected["status"] = "idle"
		changed = true
	}
	rawTurns, present := projected["turns"]
	if !present {
		return nil, false, errors.New("legacy ordinary thread turn inventory is missing")
	}
	turns, ok := rawTurns.([]any)
	if !ok {
		return nil, false, errors.New("legacy ordinary thread turn inventory is invalid")
	}
	seenTurns := map[string]bool{}
	for turnIndex, rawTurn := range turns {
		turn, ok := rawTurn.(map[string]any)
		if !ok || turn == nil {
			return nil, false, errors.New("legacy ordinary thread contains an invalid turn")
		}
		turnID := strings.TrimSpace(migrationStringField(turn, "id"))
		if turnID == "" || contracts.SafeRecordID(turnID) != turnID || seenTurns[turnID] {
			return nil, false, errors.New("legacy ordinary thread turn identity is invalid")
		}
		seenTurns[turnID] = true
		switch strings.TrimSpace(migrationStringField(turn, "status")) {
		case "running", "queued", "waiting":
			turn["status"] = "aborted"
			changed = true
		}
		rawItems, present := turn["items"]
		if !present {
			continue
		}
		items, ok := rawItems.([]any)
		if !ok {
			return nil, false, errors.New("legacy ordinary thread item inventory is invalid")
		}
		closed := make([]any, 0, len(items))
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok || item == nil {
				return nil, false, errors.New("legacy ordinary thread contains an invalid item")
			}
			kind := strings.TrimSpace(migrationStringField(item, "kind"))
			if kind == "tool_call" || kind == "tool_result" || kind == "execution_grant_transition" {
				changed = true
				continue
			}
			closedCandidate, lifecycleChanged := closeLegacyOrdinaryPendingItemV1(item)
			closedItem, accepted := threadapp.ProjectLegacyOrdinaryHistoryItemForMigrationV1(closedCandidate)
			if !accepted {
				if legacyOrdinaryRetirableItemKindV1(kind) {
					changed = true
					continue
				}
				return nil, false, fmt.Errorf("legacy ordinary item kind %q has no closed projection", kind)
			}
			closed = append(closed, closedItem)
			changed = changed || lifecycleChanged || !sameMigrationJSON(item, closedItem)
		}
		if len(closed) != len(items) {
			changed = true
		}
		turn["threadId"] = threadID
		turn["items"] = closed
		turns[turnIndex] = turn
	}
	projected["turns"] = turns
	return projected, changed, nil
}

func closeLegacyOrdinaryPendingItemV1(item map[string]any) (map[string]any, bool) {
	if item == nil {
		return nil, false
	}
	kind := strings.TrimSpace(migrationStringField(item, "kind"))
	status := strings.TrimSpace(migrationStringField(item, "status"))
	closedStatus := ""
	switch {
	case kind == "approval" && status == "pending":
		closedStatus = "expired"
	case kind == "user_input" && status == "pending":
		closedStatus = "cancelled"
	}
	if closedStatus == "" {
		return item, false
	}
	closed := contracts.CloneMap(item)
	closed["status"] = closedStatus
	return closed, true
}

func legacyOrdinaryRetirableItemKindV1(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "tool_progress", "content_redacted", "tool_payload_redacted", "execution_authority_redacted":
		return true
	default:
		return false
	}
}

func prepareLegacyOrdinaryMessagesClosureV1(path string) ([]map[string]any, bool, error) {
	return readStrictMigrationRecords(path, func(record map[string]any, line int, _ string) (map[string]any, bool, error) {
		if domainstartup.ContainsCurrentEventOrderAuthorityV1(record) {
			return nil, false, fmt.Errorf("messages sidecar carries current authority at line %d", line)
		}
		kind := strings.TrimSpace(migrationStringField(record, "kind"))
		if kind == "tool_call" || kind == "tool_result" || kind == "execution_grant_transition" ||
			legacyOrdinaryRetirableItemKindV1(kind) || legacyOrdinaryRetirableSidecarKindV1(kind) {
			return nil, true, nil
		}
		closedCandidate, lifecycleChanged := closeLegacyOrdinaryPendingItemV1(record)
		projected, accepted := threadapp.ProjectLegacyOrdinarySidecarItemForMigrationV1(closedCandidate)
		if !accepted {
			return nil, false, fmt.Errorf("messages sidecar item %d has no closed projection", line)
		}
		return projected, lifecycleChanged || !sameMigrationJSON(record, projected), nil
	})
}

func legacyOrdinaryRetirableSidecarKindV1(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "assistant_text", "assistant_reasoning", "assistant_reasoning_delta", "agent_reasoning", "error":
		return true
	default:
		return false
	}
}

func validateClosedLegacyOrdinaryDirectoryV1(threadDir, threadID string) error {
	thread, err := readClosedLegacyOrdinaryThreadV1(threadDir, threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return errors.New("closed ordinary thread recovery is unavailable")
	}
	if domainstartup.ContainsCurrentEventOrderAuthorityV1(thread) {
		return validateCurrentAuthorityThreadPrivacyV1(thread)
	}
	items, _, err := prepareLegacyOrdinaryMessagesClosureV1(filepath.Join(threadDir, "messages.jsonl"))
	if err != nil {
		return err
	}
	thread = threadapp.HydrateSidecarItems(threadapp.HydrateSidecarInput{
		ThreadID: threadID,
		Thread:   thread,
		Items:    items,
	})
	if _, err := threadapp.ProjectPublicThread(thread); err != nil {
		return fmt.Errorf("closed recovered projection is invalid: %w", err)
	}
	return nil
}

func readClosedLegacyOrdinaryThreadV1(threadDir, threadID string) (map[string]any, error) {
	body, err := os.ReadFile(filepath.Join(threadDir, "thread.json"))
	if err == nil {
		thread, decodeErr := strictMigrationObject(body, 64*1024*1024)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if strings.TrimSpace(migrationStringField(thread, "id")) != threadID {
			return nil, errors.New("closed ordinary primary identity mismatch")
		}
		return thread, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var latest map[string]any
	_, _, err = readStrictMigrationRecords(filepath.Join(threadDir, "metadata.jsonl"), func(record map[string]any, line int, _ string) (map[string]any, bool, error) {
		if strings.TrimSpace(migrationStringField(record, "kind")) != "thread_metadata" {
			return record, false, nil
		}
		thread, _ := record["thread"].(map[string]any)
		if thread == nil || strings.TrimSpace(migrationStringField(thread, "id")) != threadID {
			return nil, false, fmt.Errorf("closed ordinary metadata identity is invalid at line %d", line)
		}
		if domainstartup.ContainsCurrentEventOrderAuthorityV1(thread) {
			return nil, false, errors.New("sidecar-only metadata cannot establish current authority")
		}
		candidate, _, closeErr := closeLegacyOrdinaryThreadItemsV1(thread, threadID)
		if closeErr != nil {
			return nil, false, closeErr
		}
		latest = candidate
		return record, false, nil
	})
	return latest, err
}
