package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupSemanticValidationAttemptReusesOnlyExactValidatedDigest(t *testing.T) {
	roots := testRootSet(t)
	privateRoot := filepath.Join(roots.DataDir, "private")
	mustMkdirAll(t, privateRoot)
	path := filepath.Join(privateRoot, "record.json")
	valid := `{"value":"one","other":"two"}`
	invalidSameSize := `{"value":"one","value":"two"}`
	if len(valid) != len(invalidSameSize) {
		t.Fatal("same-size validation fixture drifted")
	}
	mustWrite(t, path, valid)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	attempt := NewStartupSemanticValidationAttemptV1()
	ctx := attempt.WithContext(context.Background())
	first, err := CaptureStrictContext(ctx, roots)
	if err != nil {
		t.Fatalf("initial semantic snapshot: %v", err)
	}
	if stats := attempt.statsV1(); stats.Entries != 1 || stats.Decodes != 1 || stats.Misses != 0 {
		t.Fatalf("initial semantic validation stats = %#v, want one entry/decode", stats)
	}
	second, err := CaptureStrictContext(ctx, roots)
	if err != nil || second.SHA256 != first.SHA256 {
		t.Fatalf("exact repeated semantic snapshot: digest=%s err=%v", second.SHA256, err)
	}
	if stats := attempt.statsV1(); stats.Entries != 1 || stats.Decodes != 1 || stats.Hits < 3 {
		t.Fatalf("exact digest was not hash-verified memo reuse: stats=%#v", stats)
	}
	if err := os.WriteFile(path, []byte(invalidSameSize), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	_, err = CaptureStrictContext(ctx, roots)
	assertIntegrityCode(t, err, "invalid_json")
	if stats := attempt.statsV1(); stats.Entries != 1 || stats.Decodes != 2 {
		t.Fatalf("failed semantic result memo/decode stats = %#v", stats)
	}
	mustWrite(t, path, valid)
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureStrictContext(ctx, roots); err != nil {
		t.Fatalf("restored exact digest did not remain valid: %v", err)
	}
	if stats := attempt.statsV1(); stats.Entries != 1 || stats.Decodes != 2 {
		t.Fatalf("restored exact digest was decoded again: stats=%#v", stats)
	}
}

func TestStartupSemanticValidationAttemptSeparatesValidationProfiles(t *testing.T) {
	attempt := NewStartupSemanticValidationAttemptV1()
	memo := attempt.memo
	record := FileRecord{
		Path:   "durable/threads/thr_one/events.jsonl",
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Size:   42, RecordCount: 2,
	}
	legacy := startupSemanticValidationProfileV1{
		ValidatorSchemaVersion: startupSemanticValidatorSchemaVersionV1,
		RootBindingDigest:      "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		Extension:              ".jsonl", TargetLabel: "durable/threads", RelativePath: "thr_one/events.jsonl",
		ThreadTree: true, StrictJSONLFraming: true, EventSequenceMode: startupEventSequenceModeStrictV1,
	}
	current := legacy
	current.EventSequenceMode = startupEventSequenceModeNoneV1
	memo.remember(record.Path, legacy, record)
	if !memo.hasCandidate(record.Path, legacy) || memo.hasCandidate(record.Path, current) {
		t.Fatal("semantic validation candidate crossed its validation profile")
	}
	if _, ok := memo.reuse(record.Path, current, record); ok {
		t.Fatal("legacy event-order validation was reused for the current profile")
	}
	if reused, ok := memo.reuse(record.Path, legacy, record); !ok || reused.RecordCount != record.RecordCount {
		t.Fatalf("exact validation profile was not reusable: record=%#v ok=%v", reused, ok)
	}
	otherRoot := legacy
	otherRoot.RootBindingDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if memo.hasCandidate(record.Path, otherRoot) {
		t.Fatal("semantic validation candidate crossed a live/stage root binding")
	}
	if _, ok := memo.reuse(record.Path, otherRoot, record); ok {
		t.Fatal("semantic validation crossed a live/stage root binding")
	}
}

func TestStartupSemanticValidationAttemptMemoizesStrictPhysicalEventOrder(t *testing.T) {
	roots := testRootSet(t)
	threadRoot := filepath.Join(roots.DurableDir, "threads", "thr_event_memo")
	mustMkdirAll(t, threadRoot)
	mustWrite(t, filepath.Join(threadRoot, "thread.json"), `{"id":"thr_event_memo","turns":[]}`)
	mustWrite(t, filepath.Join(threadRoot, "events.jsonl"),
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_event_memo\",\"seq\":1}\n")
	attempt := NewStartupSemanticValidationAttemptV1()
	if _, err := CaptureStrictContext(attempt.WithContext(context.Background()), roots); err != nil {
		t.Fatalf("event-order authority snapshot: %v", err)
	}
	if stats := attempt.statsV1(); stats.Entries != 2 || stats.Decodes != 2 || stats.Hits < 2 {
		t.Fatalf("strict events were not memoized after exact hash validation: stats=%#v", stats)
	}
}

func TestStartupSemanticValidationAttemptNeverMemoizesLegacyEventFallback(t *testing.T) {
	roots := testRootSet(t)
	threadRoot := filepath.Join(roots.DurableDir, "threads", "thr_legacy_event_memo")
	mustMkdirAll(t, threadRoot)
	mustWrite(t, filepath.Join(threadRoot, "metadata.jsonl"),
		"{\"kind\":\"thread_metadata\",\"thread\":{\"id\":\"thr_legacy_event_memo\",\"turns\":[]}}\n")
	mustWrite(t, filepath.Join(threadRoot, "events.jsonl"),
		"{\"kind\":\"heartbeat\",\"threadId\":\"thr_legacy_event_memo\",\"seq\":7}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_legacy_event_memo\",\"seq\":9}\n"+
			"{\"kind\":\"heartbeat\",\"threadId\":\"thr_legacy_event_memo\",\"seq\":8}\n")
	attempt := NewStartupSemanticValidationAttemptV1()
	ctx := attempt.WithContext(context.Background())
	if _, err := CaptureStrictContext(ctx, roots); err != nil {
		t.Fatalf("legacy event fallback snapshot: %v", err)
	}
	first := attempt.statsV1()
	if first.Entries != 1 {
		t.Fatalf("legacy event fallback entered memo: stats=%#v", first)
	}
	if _, err := CaptureStrictContext(ctx, roots); err != nil {
		t.Fatalf("repeated legacy event fallback snapshot: %v", err)
	}
	second := attempt.statsV1()
	if second.Decodes <= first.Decodes {
		t.Fatalf("legacy event fallback was reused instead of revalidated: first=%#v second=%#v", first, second)
	}
}

func TestStartupSemanticValidationAttemptHonorsCancellationWithoutMemoizing(t *testing.T) {
	roots := testRootSet(t)
	privateRoot := filepath.Join(roots.DataDir, "private")
	mustMkdirAll(t, privateRoot)
	mustWrite(t, filepath.Join(privateRoot, "record.json"), `{"value":"valid"}`)
	attempt := NewStartupSemanticValidationAttemptV1()
	ctx, cancel := context.WithCancel(attempt.WithContext(context.Background()))
	cancel()
	if _, err := CaptureStrictContext(ctx, roots); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled semantic snapshot error = %v", err)
	}
	if stats := attempt.statsV1(); stats.Entries != 0 || stats.Decodes != 0 {
		t.Fatalf("cancelled semantic snapshot entered memo: stats=%#v", stats)
	}
}

func TestStartupSemanticValidationAttemptBudgetSaturationFallsBackToValidation(t *testing.T) {
	attempt := NewStartupSemanticValidationAttemptV1()
	attempt.memo.limit = 1
	profile := startupSemanticValidationProfileV1{
		ValidatorSchemaVersion: startupSemanticValidatorSchemaVersionV1,
		RootBindingDigest:      "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		Extension:              ".json",
		TargetLabel:            "data/private",
		RelativePath:           "record.json",
	}
	first := FileRecord{
		Path: "data/private/one.json", SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Size: 1, RecordCount: 1,
	}
	second := FileRecord{
		Path: "data/private/two.json", SHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		Size: 1, RecordCount: 1,
	}
	attempt.memo.remember(first.Path, profile, first)
	attempt.memo.remember(second.Path, profile, second)
	if stats := attempt.statsV1(); stats.Entries != 1 {
		t.Fatalf("memo exceeded fail-safe budget: %#v", stats)
	}
	if _, ok := attempt.memo.reuse(second.Path, profile, second); ok {
		t.Fatal("budget saturation fabricated a validation result")
	}
}
