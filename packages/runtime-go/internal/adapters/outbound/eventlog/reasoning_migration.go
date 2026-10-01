package eventlog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

const PublicContentMigrationVersion = 3

type PrivateReasoningMigrationInput struct {
	Root                   string
	ThreadSummaryIndexPath string
	RestartPreservation    *SemanticRestartPreservationV1
}

func MigratePrivateReasoning(input PrivateReasoningMigrationInput) error {
	if err := input.RestartPreservation.revalidate(context.Background(), input.Root, input.ThreadSummaryIndexPath); err != nil {
		return err
	}
	threadsDir := filepath.Join(input.Root, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Preflight every authority-bearing event stream before writing any migrated
	// file. Private-content cleanup must never erase a malformed current marker
	// and thereby make a later ordering pass reinterpret the stream as legacy.
	for ordinal, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(threadsDir, entry.Name(), "events.jsonl")
		if err := preflightReasoningEventAuthorityV1(path); err != nil {
			return fmt.Errorf("private-reasoning event authority ordinal %d: %w", ordinal, err)
		}
	}
	for ordinal, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadDir := filepath.Join(threadsDir, entry.Name())
		if input.RestartPreservation.ownsThread(entry.Name()) {
			continue
		}
		if err := migrateReasoningThreadJSON(filepath.Join(threadDir, "thread.json")); err != nil {
			return fmt.Errorf("private-reasoning thread snapshot ordinal %d: %w", ordinal, err)
		}
		if err := migrateReasoningMessagesJSONL(filepath.Join(threadDir, "messages.jsonl")); err != nil {
			return fmt.Errorf("private-reasoning message sidecar ordinal %d: %w", ordinal, err)
		}
		if err := migrateReasoningMetadataJSONL(filepath.Join(threadDir, "metadata.jsonl")); err != nil {
			return fmt.Errorf("private-reasoning metadata sidecar ordinal %d: %w", ordinal, err)
		}
		if err := migrateReasoningEventsJSONL(filepath.Join(threadDir, "events.jsonl")); err != nil {
			return fmt.Errorf("private-reasoning event sidecar ordinal %d: %w", ordinal, err)
		}
	}
	if input.RestartPreservation != nil {
		return input.RestartPreservation.summaries.Transform(context.Background(), preservedReasoningSummaryTransform)
	}
	return migrateReasoningThreadSummariesJSONL(input.ThreadSummaryIndexPath)
}

func preflightReasoningEventAuthorityV1(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	records := []map[string]any{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		record, strictErr := strictMigrationObject([]byte(raw), 16*1024*1024)
		if strictErr != nil {
			if malformedCurrentAuthorityHintV1(raw) {
				return fmt.Errorf("malformed current authority record %d", lineNumber)
			}
			continue
		}
		if domainstartup.ContainsCurrentEventOrderAuthorityV1(record) && domainevent.ValidatePublicRecord(record) != nil {
			return fmt.Errorf("current authority record %d is not public-safe", lineNumber)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return validateCurrentTerminalEventGroupsV1(records)
}

func malformedCurrentAuthorityHintV1(raw string) bool {
	for _, key := range domainstartup.CurrentEventOrderAuthorityKeysV1() {
		if strings.Contains(raw, `"`+key+`"`) {
			return true
		}
	}
	for _, marker := range []string{
		"acceptedFinal", "acceptedFinalView", "acceptedFinalDigest", "publicationCommitId", "publicationEventId",
		"publicationSlot", "publicationPayloadDigest", "generalTerminalCASBinding", "generalTerminalCASBindingDigest",
		"generalTerminalPublication", "generalTerminalPublicationArchive", "generalTerminalCommitId", "generalTerminalEventId",
		"generalTerminalSlot", "generalTerminalPayloadDigest", "generalTerminalAuthorityKind", "generalTerminalAuthorityDigest",
		"accepted_final_batch", "general_terminal_batch", "analytix.accepted-final-delivery-batch/v1",
		"analytix.accepted-final-delivery-batch/v2",
		"analytix.general-terminal-delivery-batch/v1",
	} {
		if strings.Contains(raw, marker) {
			return true
		}
	}
	return false
}

func migrateReasoningThreadJSON(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	thread, err := strictMigrationObject(data, 64*1024*1024)
	if err != nil {
		return fmt.Errorf("decode thread for private-reasoning migration: %w", err)
	}
	// A current authority snapshot is already governed by the current public
	// contract. Validate its original bytes before any legacy sanitizer can
	// rewrite adjacent content and thereby make an invalid snapshot appear
	// eligible for a later startup migration.
	if domainstartup.ContainsCurrentEventOrderAuthorityV1(thread) {
		if err := validateCurrentAuthorityThreadPrivacyV1(thread); err != nil {
			return fmt.Errorf("preflight current authority thread: %w", err)
		}
		return nil
	}
	sanitized, err := threadapp.SanitizeDurableHistory(thread)
	if err != nil {
		return fmt.Errorf("sanitize legacy durable history: %w", err)
	}
	projected, err := projectLegacyOrdinaryThreadPrivacyV1(sanitized)
	if err != nil {
		return fmt.Errorf("project legacy ordinary privacy: %w", err)
	}
	if err := threadapp.ValidatePublicHistory(projected); err != nil {
		return fmt.Errorf("validate migrated public history: %w", err)
	}
	next, err := json.Marshal(projected)
	if err != nil {
		return err
	}
	if string(next) == string(data) {
		return nil
	}
	if _, err := filestore.WriteFileAtomic(path, next, nil); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func projectLegacyOrdinaryThreadPrivacyV1(thread map[string]any) (map[string]any, error) {
	if thread == nil {
		return nil, errors.New("legacy ordinary thread privacy projection is unavailable")
	}
	if domainstartup.ContainsCurrentEventOrderAuthorityV1(thread) {
		if err := validateCurrentAuthorityThreadPrivacyV1(thread); err != nil {
			return nil, errors.Join(errors.New("current authority thread is not already public-safe"), err)
		}
		projected, _ := cloneLegacyAuthorityValueV1(thread).(map[string]any)
		return projected, nil
	}
	workspace, hasWorkspace := thread["workspace"]
	failureSafeValue, _ := projectLegacyPrivateContentValueV1(thread)
	failureSafe, _ := failureSafeValue.(map[string]any)
	if failureSafe == nil {
		return nil, errors.New("legacy ordinary thread failure projection failed")
	}
	ordinaryValue := domainordinary.ProjectValueV1(failureSafe)
	ordinary, _ := ordinaryValue.(map[string]any)
	if ordinary == nil {
		return nil, errors.New("legacy ordinary thread fixed-point projection failed")
	}
	publicSafeValue, _ := projectLegacyPrivateContentValueV1(ordinary)
	publicSafe, _ := publicSafeValue.(map[string]any)
	if publicSafe == nil {
		return nil, errors.New("legacy ordinary thread final public projection failed")
	}
	finalValue := domainordinary.ProjectValueV1(publicSafe)
	publicSafe, _ = finalValue.(map[string]any)
	if publicSafe == nil {
		return nil, errors.New("legacy ordinary thread final fixed-point projection failed")
	}
	if hasWorkspace {
		publicSafe["workspace"] = contracts.CloneValue(workspace)
	}
	if !domainstartup.FrozenEventOrderAuthorityStableV1(thread, publicSafe) {
		return nil, errors.New("legacy ordinary thread projection changed frozen authority")
	}
	return publicSafe, nil
}

func validateCurrentAuthorityThreadPrivacyV1(thread map[string]any, inherited ...func(map[string]any, map[string]any) error) error {
	if err := threadapp.ValidatePublicHistory(thread); err != nil {
		return err
	}
	threadID := strings.TrimSpace(contracts.StringField(thread, "id"))
	if threadID == "" {
		return errors.New("current authority thread identity is missing")
	}
	if value, present := thread["securityState"]; present {
		securityContext, err := domainsecurity.ParseTurnSecurityContext(value)
		if err != nil || securityContext.ThreadID != threadID {
			return errors.New("current authority root security context is invalid")
		}
	}
	if value, present := thread["contextEpochState"]; present {
		state, err := domaincontextepoch.ParseState(value)
		if err != nil || state.ThreadID != threadID {
			return errors.New("current authority context epoch state is invalid")
		}
	}
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		if value, present := turn["securityContext"]; present {
			securityContext, err := domainsecurity.ParseTurnSecurityContext(value)
			if err != nil || securityContext.ThreadID != threadID || securityContext.TurnID != turnID {
				return errors.New("current authority turn security context is invalid")
			}
			if _, err := executiongrantapp.RegistryFromThread(threadID, thread, turnID); err != nil {
				return errors.New("current authority execution grant registry is invalid")
			}
		} else if domainstartup.ContainsCurrentEventOrderAuthorityV1(turn) {
			if len(inherited) != 1 || inherited[0] == nil {
				return errors.New("current authority turn security context is missing")
			}
			if err := inherited[0](thread, turn); err != nil {
				return err
			}
		}
		if value, present := turn["acceptedFinal"]; present {
			acceptedFinal, err := domainevidence.ParseAcceptedFinalRecord(value)
			if err != nil || acceptedFinal.ThreadID != threadID || acceptedFinal.TurnID != turnID {
				return errors.New("current authority accepted final is invalid")
			}
		}
	}
	return nil
}

func cloneLegacyAuthorityValueV1(value any) any {
	switch current := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, child := range current {
			out[key] = cloneLegacyAuthorityValueV1(child)
		}
		return out
	case []any:
		out := make([]any, len(current))
		for index, child := range current {
			out[index] = cloneLegacyAuthorityValueV1(child)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(current))
		for index, child := range current {
			out[index], _ = cloneLegacyAuthorityValueV1(child).(map[string]any)
		}
		return out
	default:
		return value
	}
}

type legacyPrivateContentRemovalV1 uint8

const (
	legacyFailureContentRemovedV1 legacyPrivateContentRemovalV1 = 1 << iota
	legacyAssistantDraftRemovedV1
	legacyReasoningContentRemovedV1
)

func projectLegacyPrivateContentValueV1(value any) (any, legacyPrivateContentRemovalV1) {
	switch typed := value.(type) {
	case map[string]any:
		// Digest-bound general terminal commits and archives deliberately retain
		// an exact host failure `error` mirror. Preserve only values accepted by
		// their closed parsers; generic public-looking maps do not establish host
		// identity or integrity and still take the legacy migration path below.
		if preserveDigestBoundGeneralTerminalRecordV1(typed) {
			return contracts.CloneValue(typed), 0
		}
		out := make(map[string]any, len(typed))
		var removed legacyPrivateContentRemovalV1
		originalKind := strings.ToLower(strings.TrimSpace(stringValue(typed["kind"])))
		for key, child := range typed {
			if legacyRawFailureKeyV1(key) && legacyRawFailureValuePresentV1(child) {
				removed |= legacyFailureContentRemovedV1
				continue
			}
			if text, ok := child.(string); ok && !(originalKind == "user_message" && normalizeLegacyFailureKeyV1(key) == "text") {
				public, err := domainevent.FilterPublicText(text)
				if err != nil {
					removed |= legacyReasoningContentRemovedV1
					continue
				}
				if public != text {
					removed |= legacyReasoningContentRemovedV1
				}
				out[key] = public
				continue
			}
			projected, childRemoved := projectLegacyPrivateContentValueV1(child)
			removed |= childRemoved
			// JSON null is a real, safe value. Keep it distinct from a child that
			// this migration deliberately removed; dropping an authority-field
			// null on a later startup changes the canonical bytes and can
			// invalidate a digest computed over the original record.
			if projected != nil || child == nil && childRemoved == 0 {
				out[key] = projected
			}
		}
		kind := strings.ToLower(strings.TrimSpace(stringValue(out["kind"])))
		if kind == "assistant_reasoning" || kind == "assistant_reasoning_delta" || kind == "agent_reasoning" {
			return nil, removed | legacyReasoningContentRemovedV1
		}
		if originalKind == "assistant_text" &&
			removed&legacyReasoningContentRemovedV1 != 0 &&
			strings.TrimSpace(stringValue(out["text"])) == "" {
			return nil, removed | legacyReasoningContentRemovedV1
		}
		if errors.Is(domainevent.ValidatePublicRecord(out), domainevent.ErrRawFailurePersistence) &&
			(kind == "error" || strings.HasSuffix(kind, "_error")) {
			return nil, removed | legacyFailureContentRemovedV1
		}
		if removed&legacyFailureContentRemovedV1 != 0 {
			out["failureContentRedacted"] = true
		}
		if removed&legacyAssistantDraftRemovedV1 != 0 {
			out["assistantDraftRedacted"] = true
		}
		if removed&legacyReasoningContentRemovedV1 != 0 {
			out["providerPrivateContentRemoved"] = true
		}
		if errors.Is(domainevent.ValidatePublicRecord(out), domainevent.ErrRawFailurePersistence) {
			return nil, removed | legacyFailureContentRemovedV1
		}
		if errors.Is(domainevent.ValidatePublicRecord(out), domainevent.ErrAssistantDraftPersistence) {
			return nil, removed | legacyAssistantDraftRemovedV1
		}
		if errors.Is(domainevent.ValidatePublicRecord(out), domainevent.ErrPrivateReasoningPersistence) {
			return nil, removed | legacyReasoningContentRemovedV1
		}
		return out, removed
	case []any:
		out := make([]any, 0, len(typed))
		var removed legacyPrivateContentRemovalV1
		for _, child := range typed {
			projected, childRemoved := projectLegacyPrivateContentValueV1(child)
			removed |= childRemoved
			if projected != nil || child == nil && childRemoved == 0 {
				out = append(out, projected)
			}
		}
		return out, removed
	default:
		return contracts.CloneValue(value), 0
	}
}

func preserveDigestBoundGeneralTerminalRecordV1(value map[string]any) bool {
	if _, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(value); err == nil {
		return true
	}
	_, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(value)
	return err == nil
}

func legacyRawFailureKeyV1(key string) bool {
	normalized := normalizeLegacyFailureKeyV1(key)
	switch normalized {
	case "iserror", "errorcode":
		return false
	case "error", "stderr", "stack", "stacktrace", "traceback", "exception", "exceptionmessage", "providererror":
		return true
	default:
		return strings.HasSuffix(normalized, "error") || strings.HasSuffix(normalized, "errormessage")
	}
}

func normalizeLegacyFailureKeyV1(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	var normalized strings.Builder
	for index := 0; index < len(key); index++ {
		character := key[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			normalized.WriteByte(character)
		}
	}
	return normalized.String()
}

func legacyRawFailureValuePresentV1(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func migrateReasoningMessagesJSONL(path string) error {
	records, changed, err := readReasoningMigrationRecords(path, func(record map[string]any, lineNumber int, _ string) (map[string]any, bool) {
		failureSafeValue, _ := projectLegacyPrivateContentValueV1(record)
		failureSafe, _ := failureSafeValue.(map[string]any)
		if failureSafe == nil {
			return legacyMessageContentTombstoneV1(record, lineNumber), true
		}
		ordinaryValue := domainordinary.ProjectValueV1(failureSafe)
		ordinary, _ := ordinaryValue.(map[string]any)
		public, ok := threadapp.SanitizePublicItem(ordinary)
		if !ok {
			return legacyMessageContentTombstoneV1(record, lineNumber), true
		}
		before, _ := json.Marshal(record)
		after, _ := json.Marshal(public)
		return public, string(before) != string(after)
	})
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, ".messages-reasoning-migration-*.tmp", records)
}

func legacyMessageContentTombstoneV1(record map[string]any, lineNumber int) map[string]any {
	out := map[string]any{
		"kind":             "content_redacted",
		"code":             "private_message_content_removed",
		"migrationVersion": float64(PublicContentMigrationVersion),
		"sourceLine":       float64(lineNumber),
	}
	for _, key := range []string{"id", "threadId", "turnId"} {
		if value := safeMigrationRecordID(record[key]); value != "" {
			out[key] = value
		}
	}
	for _, key := range []string{"timestamp", "createdAt", "finishedAt"} {
		if value := safeMigrationTimestamp(record[key]); value != "" {
			out[key] = value
		}
	}
	return out
}

func migrateReasoningEventsJSONL(path string) error {
	records, changed, err := readReasoningMigrationRecords(path, func(record map[string]any, lineNumber int, raw string) (map[string]any, bool) {
		if err := domainevent.ValidatePublicRecord(record); err == nil {
			return record, false
		}
		return validatedPrivateContentMigrationProjection(record, lineNumber, raw), true
	})
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, ".events-reasoning-migration-*.tmp", records)
}

func migrateReasoningMetadataJSONL(path string) error {
	records, changed, err := readReasoningMigrationRecords(path, func(record map[string]any, lineNumber int, raw string) (map[string]any, bool) {
		if err := domainevent.ValidatePublicRecord(record); err == nil {
			return record, false
		}
		thread, _ := record["thread"].(map[string]any)
		if thread == nil {
			return validatedPrivateContentMigrationProjection(record, lineNumber, raw), true
		}
		sanitizedThread := threadapp.SanitizePublicHistory(thread)
		next := threadapp.MetadataSidecarEntry(sanitizedThread, stringValue(record["timestamp"]))
		if next == nil || domainevent.ValidatePublicRecord(next) != nil {
			return validatedPrivateContentMigrationProjection(record, lineNumber, raw), true
		}
		before, _ := json.Marshal(record)
		after, _ := json.Marshal(next)
		return next, string(before) != string(after)
	})
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, ".metadata-reasoning-migration-*.tmp", records)
}

func migrateReasoningThreadSummariesJSONL(path string) error {
	records, changed, err := readReasoningMigrationRecords(path, reasoningSummaryTransform)
	if err != nil || !changed {
		return err
	}
	return filestore.WriteJSONLFileAtomic(path, ".thread-summaries-reasoning-migration-*.tmp", records)
}

func reasoningSummaryTransform(record map[string]any, lineNumber int, raw string) (map[string]any, bool) {
	if err := threadapp.ValidateSummaryIndexRecordV1(record); err == nil {
		return record, false
	}
	return validatedPrivateContentMigrationProjection(record, lineNumber, raw), true
}

func preservedReasoningSummaryTransform(record map[string]any, _ int, _ string) (map[string]any, bool, error) {
	if err := threadapp.ValidateSummaryIndexRecordV1(record); err == nil {
		return record, false, nil
	}
	// The raw-row owner has already validated the complete index identity
	// inventory. Retain the independent row's lookup identity and deletion
	// state, discarding all unsafe display hints. Normal listing rehydrates
	// hints from the canonical primary; this projection creates no authority.
	id := migrationStringField(record, "threadId")
	if id == "" {
		summary, _ := record["summary"].(map[string]any)
		id = migrationStringField(summary, "id")
	}
	if safeMigrationRecordID(id) != id || id == "" {
		return nil, false, errors.New("summary projection identity is unavailable")
	}
	next := map[string]any{"schemaVersion": float64(1), "threadId": id, "summary": map[string]any{"id": id}}
	if deleted, _ := record["deleted"].(bool); deleted {
		next["deleted"] = true
	}
	return next, true, nil
}

func validatedPrivateContentMigrationProjection(record map[string]any, lineNumber int, raw string) map[string]any {
	projected := privateContentMigrationProjection(record, lineNumber, raw)
	if domainevent.ValidatePublicRecord(projected) == nil {
		return projected
	}
	return reasoningRejectedLineRecord(lineNumber, raw)
}

type reasoningMigrationTransform func(record map[string]any, lineNumber int, raw string) (map[string]any, bool)

func readReasoningMigrationRecords(path string, transform reasoningMigrationTransform) ([]map[string]any, bool, error) {
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
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 16*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(raw), &record); err != nil {
			records = append(records, reasoningRejectedLineRecord(lineNumber, raw))
			changed = true
			continue
		}
		next, recordChanged := transform(record, lineNumber, raw)
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

func reasoningRedactionTombstone(record map[string]any, lineNumber int, _ string) map[string]any {
	out := map[string]any{
		"kind":             "content_redacted",
		"code":             "private_content_removed",
		"migrationVersion": float64(PublicContentMigrationVersion),
		"sourceLine":       float64(lineNumber),
	}
	if seq, ok := contracts.NumericSeq(record["seq"]); ok {
		out["seq"] = float64(seq)
	}
	for _, key := range []string{"threadId", "turnId"} {
		if value := safeMigrationRecordID(record[key]); value != "" {
			out[key] = value
		}
	}
	if value := safeMigrationTimestamp(record["timestamp"]); value != "" {
		out["timestamp"] = value
	}
	return out
}

func privateContentMigrationProjection(record map[string]any, lineNumber int, raw string) map[string]any {
	if strings.TrimSpace(stringValue(record["kind"])) == "usage" {
		if usage := numericUsageProjectionV1(record); len(usage) > 0 {
			out := map[string]any{
				"kind":             "usage",
				"code":             "private_content_removed",
				"migrationVersion": float64(PublicContentMigrationVersion),
				"sourceLine":       float64(lineNumber),
				"usage":            usage,
			}
			if seq, ok := contracts.NumericSeq(record["seq"]); ok {
				out["seq"] = float64(seq)
			}
			for _, key := range []string{"threadId", "turnId"} {
				if value := safeMigrationRecordID(record[key]); value != "" {
					out[key] = value
				}
			}
			if value := safeMigrationTimestamp(record["timestamp"]); value != "" {
				out["timestamp"] = value
			}
			return out
		}
	}
	if errors.Is(domainevent.ValidatePublicRecord(record), domainevent.ErrRawFailurePersistence) {
		return privateFailureMigrationTombstoneV1(record, lineNumber)
	}
	return reasoningRedactionTombstone(record, lineNumber, raw)
}

func privateFailureMigrationTombstoneV1(record map[string]any, lineNumber int) map[string]any {
	out := map[string]any{
		"kind":             "content_redacted",
		"code":             "private_failure_removed",
		"migrationVersion": float64(PublicContentMigrationVersion),
		"sourceLine":       float64(lineNumber),
	}
	if seq, ok := contracts.NumericSeq(record["seq"]); ok {
		out["seq"] = float64(seq)
	}
	for _, key := range []string{"threadId", "turnId"} {
		if value := safeMigrationRecordID(record[key]); value != "" {
			out[key] = value
		}
	}
	if value := safeMigrationTimestamp(record["timestamp"]); value != "" {
		out["timestamp"] = value
	}
	return out
}

var numericUsageTokenFieldsV1 = []string{
	"inputTokens", "promptTokens", "outputTokens", "completionTokens", "reasoningTokens",
	"totalTokens", "cachedTokens", "cacheHitTokens", "cacheMissTokens",
}

func numericUsageProjectionV1(record map[string]any) map[string]any {
	out := map[string]any{}
	usage, _ := record["usage"].(map[string]any)
	for _, key := range numericUsageTokenFieldsV1 {
		value, found := usage[key]
		if !found {
			value, found = record[key]
		}
		if !found {
			continue
		}
		if number, ok := nonNegativeSafeIntegerV1(value); ok {
			out[key] = number
		}
	}
	return out
}

func nonNegativeSafeIntegerV1(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0 && number <= 1<<53-1 && math.Trunc(number) == number
}

func reasoningRejectedLineRecord(lineNumber int, _ string) map[string]any {
	return map[string]any{
		"kind":             "migration_rejected_line",
		"code":             "invalid_json_removed",
		"migrationVersion": float64(PublicContentMigrationVersion),
		"sourceLine":       float64(lineNumber),
	}
}

func safeMigrationRecordID(value any) string {
	text := safeMigrationText(value)
	if text == "" || contracts.SafeRecordID(text) != text {
		return ""
	}
	return text
}

func safeMigrationText(value any) string {
	text, _ := value.(string)
	text = strings.TrimSpace(text)
	if text == "" || domainsecret.ValidateValueV1(text) != nil || domainprivacy.ValidateOrdinaryText(text) != nil {
		return ""
	}
	public, err := domainevent.FilterPublicText(text)
	if err != nil || public != text {
		return ""
	}
	return text
}

func safeMigrationTimestamp(value any) string {
	text := safeMigrationText(value)
	if text == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
		return ""
	}
	return text
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
