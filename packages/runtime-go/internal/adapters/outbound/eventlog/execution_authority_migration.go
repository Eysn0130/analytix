package eventlog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	historymigrationapp "analytix.local/runtime-go/internal/app/historymigration"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const DerivedExecutionAuthorityMigrationVersion = 1

type DerivedExecutionAuthorityMigrationInput struct {
	Root                string
	RestartPreservation *SemanticRestartPreservationV1
}

// MigrateDerivedExecutionAuthority runs only inside the semantic startup
// stage. The outer signed semantic journal owns live-root atomicity and crash
// recovery; this function only transforms the isolated stage.
func MigrateDerivedExecutionAuthority(input DerivedExecutionAuthorityMigrationInput) error {
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
	for _, entry := range entries {
		if !entry.IsDir() || contracts.SafeRecordID(entry.Name()) != entry.Name() {
			return errors.New("execution-authority migration found an invalid thread directory")
		}
		if input.RestartPreservation.ownsThread(entry.Name()) {
			continue
		}
		threadDir := filepath.Join(threadsDir, entry.Name())
		thread, err := migrateExecutionAuthorityThreadJSON(filepath.Join(threadDir, "thread.json"), entry.Name())
		if err != nil {
			return err
		}
		if thread == nil {
			return errors.New("execution-authority migration thread snapshot is missing")
		}
		if err := migrateExecutionAuthorityMessages(filepath.Join(threadDir, "messages.jsonl"), entry.Name(), thread); err != nil {
			return err
		}
		if err := migrateExecutionAuthorityMetadata(filepath.Join(threadDir, "metadata.jsonl"), entry.Name()); err != nil {
			return err
		}
		if err := migrateExecutionAuthorityEvents(filepath.Join(threadDir, "events.jsonl"), thread); err != nil {
			return err
		}
	}
	return nil
}

func migrateExecutionAuthorityThreadJSON(path, threadID string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return loadExecutionAuthoritySidecarOnlyThread(filepath.Join(filepath.Dir(path), "metadata.jsonl"), threadID)
	}
	if err != nil {
		return nil, err
	}
	thread, err := strictMigrationObject(data, 64*1024*1024)
	if err != nil {
		return nil, fmt.Errorf("decode thread for execution-authority migration: %w", err)
	}
	if strings.TrimSpace(migrationStringField(thread, "id")) != threadID {
		return nil, errors.New("execution-authority migration thread identity mismatch")
	}
	projected, changed, err := historymigrationapp.StripUntrustedExecutionAuthority(thread)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := writeMigrationJSON(path, projected); err != nil {
			return nil, err
		}
	}
	return projected, nil
}

// loadExecutionAuthoritySidecarOnlyThread supports the exact historical
// rendering-only layout that predates a durable thread.json snapshot. It does
// not create primary thread authority. The returned projection is used only to
// strip or tombstone untrusted authority from sibling sidecars in the isolated
// semantic-startup stage; normal store recovery remains responsible for the
// read-only public view.
func loadExecutionAuthoritySidecarOnlyThread(path, threadID string) (map[string]any, error) {
	var latest map[string]any
	_, _, err := readStrictMigrationRecords(path, func(record map[string]any, line int, _ string) (map[string]any, bool, error) {
		thread, _ := record["thread"].(map[string]any)
		if thread == nil || strings.TrimSpace(migrationStringField(thread, "id")) != threadID {
			return nil, false, fmt.Errorf("execution-authority metadata identity is invalid at line %d", line)
		}
		projected, _, err := historymigrationapp.StripUntrustedExecutionAuthority(thread)
		if err != nil {
			return nil, false, err
		}
		latest = projected
		return record, false, nil
	})
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, errors.New("execution-authority migration thread snapshot is missing")
	}
	return latest, nil
}

func migrateExecutionAuthorityMessages(path, threadID string, thread map[string]any) error {
	at := migrationStableTime(thread)
	records, changed, err := readStrictMigrationRecords(path, func(record map[string]any, _ int, _ string) (map[string]any, bool, error) {
		projected, keep := historymigrationapp.ProjectAuthorityFreeSidecarItem(threadID, record, at)
		if !keep {
			return nil, true, nil
		}
		return projected, !sameMigrationJSON(record, projected), nil
	})
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, ".messages-execution-authority-migration-*.tmp", records)
}

func migrateExecutionAuthorityMetadata(path, threadID string) error {
	records, changed, err := readStrictMigrationRecords(path, func(record map[string]any, line int, _ string) (map[string]any, bool, error) {
		thread, _ := record["thread"].(map[string]any)
		if thread == nil || strings.TrimSpace(migrationStringField(thread, "id")) != threadID {
			return nil, false, fmt.Errorf("execution-authority metadata identity is invalid at line %d", line)
		}
		projected, _, err := historymigrationapp.StripUntrustedExecutionAuthority(thread)
		if err != nil {
			return nil, false, err
		}
		next := contracts.CloneMap(record)
		next["thread"] = threadapp.StripItemsForSidecar(projected)
		return next, !sameMigrationJSON(record, next), nil
	})
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, ".metadata-execution-authority-migration-*.tmp", records)
}

func migrateExecutionAuthorityEvents(path string, thread map[string]any) error {
	records, changed, err := readStrictMigrationRecords(path, func(record map[string]any, line int, raw string) (map[string]any, bool, error) {
		if historymigrationapp.EventAuthorityBelongsToThread(thread, record) {
			return record, false, nil
		}
		return executionAuthorityTombstone(record, line, raw), true, nil
	})
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, ".events-execution-authority-migration-*.tmp", records)
}

type strictMigrationTransform func(map[string]any, int, string) (map[string]any, bool, error)

func readStrictMigrationRecords(path string, transform strictMigrationTransform) ([]map[string]any, bool, error) {
	return readStrictMigrationRecordsWithDecoderV1(path, transform, strictMigrationObject)
}

func readStrictMigrationRecordsWithDecoderV1(path string, transform strictMigrationTransform, decode func([]byte, int) (map[string]any, error)) ([]map[string]any, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	records := []map[string]any{}
	changed := false
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			return nil, false, fmt.Errorf("execution-authority migration contains an empty JSONL record at line %d", line)
		}
		record, err := decode([]byte(raw), 16*1024*1024)
		if err != nil {
			return nil, false, fmt.Errorf("execution-authority migration JSONL record %d is invalid: %w", line, err)
		}
		next, recordChanged, err := transform(record, line, raw)
		if err != nil {
			return nil, false, err
		}
		changed = changed || recordChanged
		if next != nil {
			records = append(records, next)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return records, changed, nil
}

func strictMigrationObject(data []byte, maxBytes int) (map[string]any, error) {
	if err := jsonstrict.Validate(data, strictMigrationJSONOptionsV1(maxBytes)); err != nil {
		return nil, err
	}
	record := map[string]any{}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return record, nil
}

func strictMigrationJSONOptionsV1(maxBytes int) jsonstrict.Options {
	return jsonstrict.Options{
		RequireObject: true, MaxBytes: maxBytes, MaxDepth: 256, MaxTokens: 2_000_000,
		MaxStringBytes: 16 * 1024 * 1024, MaxNumberBytes: 256, MaxAbsExponent: 10000,
	}
}

func executionAuthorityTombstone(record map[string]any, line int, _ string) map[string]any {
	out := map[string]any{
		"kind":             "execution_authority_redacted",
		"code":             "foreign_execution_authority_removed",
		"migrationVersion": float64(DerivedExecutionAuthorityMigrationVersion),
		"sourceLine":       float64(line),
	}
	for _, key := range []string{"threadId", "turnId", "seq", "timestamp"} {
		if value, ok := record[key]; ok {
			out[key] = value
		}
	}
	return out
}

func writeMigrationJSON(path string, value map[string]any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := filestore.WriteFileAtomic(path, data, nil); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func sameMigrationJSON(left, right map[string]any) bool {
	leftBody, _ := json.Marshal(left)
	rightBody, _ := json.Marshal(right)
	return string(leftBody) == string(rightBody)
}

func migrationStableTime(thread map[string]any) string {
	for _, key := range []string{"forkedAt", "updatedAt", "createdAt"} {
		if value := strings.TrimSpace(migrationStringField(thread, key)); value != "" {
			return value
		}
	}
	return "1970-01-01T00:00:00Z"
}

func migrationStringField(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	return migrationStringValue(record[key])
}

func migrationStringValue(value any) string {
	text, _ := value.(string)
	return text
}
