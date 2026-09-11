package eventlog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const OrdinaryToolPayloadMigrationVersion = 1

type OrdinaryToolPayloadMigrationInput struct {
	Root                   string
	ThreadSummaryIndexPath string
	RestartPreservation    *SemanticRestartPreservationV1
}

// MigrateOrdinaryToolPayloads runs only inside the semantic-startup stage.
// The signed outer journal owns live-root atomicity. This transform removes
// legacy raw tool arguments/results from every ordinary durable projection;
// legacy bytes cannot become evidence authority merely because they existed.
func MigrateOrdinaryToolPayloads(input OrdinaryToolPayloadMigrationInput) error {
	if err := input.RestartPreservation.revalidate(context.Background(), input.Root, input.ThreadSummaryIndexPath); err != nil {
		return err
	}
	threadsDir := filepath.Join(strings.TrimSpace(input.Root), "threads")
	entries, err := os.ReadDir(threadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return migrateOrdinaryToolPayloadSummaries(input)
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || contracts.SafeRecordID(entry.Name()) != entry.Name() {
			return errors.New("ordinary tool-payload migration found an invalid thread directory")
		}
		if input.RestartPreservation.ownsThread(entry.Name()) {
			continue
		}
		threadDir := filepath.Join(threadsDir, entry.Name())
		if err := migrateOrdinaryToolPayloadThread(filepath.Join(threadDir, "thread.json"), entry.Name()); err != nil {
			return err
		}
		if err := migrateOrdinaryToolPayloadRecords(filepath.Join(threadDir, "messages.jsonl"), ".messages-tool-payload-migration-*.tmp", false); err != nil {
			return err
		}
		if err := migrateOrdinaryToolPayloadRecords(filepath.Join(threadDir, "metadata.jsonl"), ".metadata-tool-payload-migration-*.tmp", false); err != nil {
			return err
		}
		if err := migrateOrdinaryToolPayloadRecords(filepath.Join(threadDir, "events.jsonl"), ".events-tool-payload-migration-*.tmp", true); err != nil {
			return err
		}
	}
	return migrateOrdinaryToolPayloadSummaries(input)
}

func migrateOrdinaryToolPayloadSummaries(input OrdinaryToolPayloadMigrationInput) error {
	if input.RestartPreservation != nil {
		return input.RestartPreservation.summaries.Transform(context.Background(), ordinaryToolPayloadTransform(false))
	}
	return migrateOrdinaryToolPayloadRecords(input.ThreadSummaryIndexPath, ".thread-summaries-tool-payload-migration-*.tmp", false)
}

func migrateOrdinaryToolPayloadThread(path, threadID string) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	thread, err := strictMigrationObject(body, 64*1024*1024)
	if err != nil {
		return fmt.Errorf("decode thread for ordinary tool-payload migration: %w", err)
	}
	if strings.TrimSpace(migrationStringField(thread, "id")) != threadID {
		return errors.New("ordinary tool-payload migration thread identity mismatch")
	}
	if !containsStructuredToolPayload(thread) {
		return nil
	}
	projected, err := threadapp.SanitizeDurableHistory(thread)
	if err != nil || projected == nil || containsUnclosedToolPayload(projected) {
		return errors.New("ordinary tool-payload migration could not close the thread projection")
	}
	if sameMigrationJSON(thread, projected) {
		return nil
	}
	return writeMigrationJSON(path, projected)
}

func migrateOrdinaryToolPayloadRecords(path, tempPattern string, durableEvent bool) error {
	records, changed, err := readStrictMigrationRecords(path, ordinaryToolPayloadTransform(durableEvent))
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, tempPattern, records)
}

func ordinaryToolPayloadTransform(durableEvent bool) strictMigrationTransform {
	return func(record map[string]any, line int, raw string) (map[string]any, bool, error) {
		if !containsStructuredToolPayload(record) {
			return record, false, nil
		}
		kind := strings.ToLower(strings.TrimSpace(migrationStringField(record, "kind")))
		if durableEvent && (kind == "tool_call" || kind == "tool_result") {
			return toolPayloadRedactionTombstone(record, line, raw), true, nil
		}
		var projectedValue any
		var ok bool
		if durableEvent {
			projectedValue, ok = domainevent.SanitizeDurableValue(record, false)
		} else {
			projectedValue, ok = domainevent.SanitizePublicValue(record, false)
		}
		projected, _ := projectedValue.(map[string]any)
		if !ok || projected == nil || containsUnclosedToolPayload(projected) {
			return nil, false, fmt.Errorf("ordinary tool-payload migration could not close record at line %d", line)
		}
		return projected, !sameMigrationJSON(record, projected), nil
	}
}

func containsStructuredToolPayload(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		kind := strings.ToLower(strings.TrimSpace(migrationStringField(typed, "kind")))
		if kind == "tool_call" || kind == "tool_result" {
			return true
		}
		for _, child := range typed {
			if containsStructuredToolPayload(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsStructuredToolPayload(child) {
				return true
			}
		}
	}
	return false
}

func containsUnclosedToolPayload(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		kind := strings.ToLower(strings.TrimSpace(migrationStringField(typed, "kind")))
		switch kind {
		case "tool_call":
			if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(typed["arguments"]); err != nil {
				return true
			}
		case "tool_result":
			if _, err := domaintoolresult.ParsePublicToolResultProjectionV1(typed["output"]); err != nil {
				return true
			}
		}
		for _, child := range typed {
			if containsUnclosedToolPayload(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsUnclosedToolPayload(child) {
				return true
			}
		}
	}
	return false
}

func toolPayloadRedactionTombstone(record map[string]any, line int, _ string) map[string]any {
	out := map[string]any{
		"kind":             "tool_payload_redacted",
		"code":             "legacy_tool_payload_removed",
		"migrationVersion": float64(OrdinaryToolPayloadMigrationVersion),
		"sourceLine":       float64(line),
	}
	for _, key := range []string{"threadId", "turnId", "seq", "timestamp"} {
		if value, ok := record[key]; ok {
			out[key] = value
		}
	}
	return out
}

type SemanticStartupContentMigrationInput struct {
	Root                   string
	ThreadSummaryIndexPath string
	RestartPreservation    *SemanticRestartPreservationV1
}

func MigrateSemanticStartupContent(input SemanticStartupContentMigrationInput) error {
	if err := input.RestartPreservation.revalidate(context.Background(), input.Root, input.ThreadSummaryIndexPath); err != nil {
		return err
	}
	if err := preflightSemanticStartupCurrentAuthorityV1(input.Root); err != nil {
		return err
	}
	if err := MigrateDerivedExecutionAuthority(DerivedExecutionAuthorityMigrationInput{Root: input.Root, RestartPreservation: input.RestartPreservation}); err != nil {
		return err
	}
	if err := MigratePrivateReasoning(PrivateReasoningMigrationInput{Root: input.Root, ThreadSummaryIndexPath: input.ThreadSummaryIndexPath, RestartPreservation: input.RestartPreservation}); err != nil {
		return err
	}
	if err := MigrateOrdinaryToolPayloads(OrdinaryToolPayloadMigrationInput{Root: input.Root, ThreadSummaryIndexPath: input.ThreadSummaryIndexPath, RestartPreservation: input.RestartPreservation}); err != nil {
		return err
	}
	if err := MigrateLegacyOrdinaryProjectionClosure(LegacyOrdinaryProjectionClosureInput{Root: input.Root, RestartPreservation: input.RestartPreservation}); err != nil {
		return err
	}
	if err := migrateLegacyEventSequenceOrderV1(legacyEventSequenceMigrationInput{Root: input.Root, RestartPreservation: input.RestartPreservation}); err != nil {
		return err
	}
	return validatePreservedLegacyOrdinaryProjectionFixedPointV1(input.Root, input.RestartPreservation)
}
