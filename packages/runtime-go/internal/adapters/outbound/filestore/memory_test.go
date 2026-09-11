package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmemory "analytix.local/runtime-go/internal/domain/memory"
)

func TestPersistentMemoryStorePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{
		"content":    "Remember provider/model identity",
		"workspace":  "/tmp/analytix",
		"scope":      "project",
		"confidence": 0.7,
		"tags":       []any{"runtime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id != "mem_go_1" {
		t.Fatalf("unexpected memory id: %#v", created)
	}

	restarted, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := restarted.List(false, "/tmp/analytix")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected persisted memory after restart, got %#v", listed)
	}
	updated, found, err := restarted.Patch(id, map[string]any{
		"content":    "Remember updated runtime state",
		"confidence": 0.9,
		"disabled":   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found || updated["content"] != "Remember updated runtime state" || updated["disabledAt"] == "" {
		t.Fatalf("memory patch mismatch: found=%v updated=%#v", found, updated)
	}

	next, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := next.Create(map[string]any{"content": "Second memory"})
	if err != nil {
		t.Fatal(err)
	}
	if second["id"] != "mem_go_2" {
		t.Fatalf("memory sequence should resume after restart, got %#v", second)
	}
}

func TestPersistentMemoryStoreRejectsUnknownOrMalformedScope(t *testing.T) {
	store, err := NewPersistentMemoryStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []any{"unknown", "", float64(7)} {
		if memory, err := store.Create(map[string]any{"content": "must fail", "scope": scope}); err == nil || memory != nil {
			t.Fatalf("invalid create scope was accepted: scope=%#v memory=%#v err=%v", scope, memory, err)
		}
	}
	created, err := store.Create(map[string]any{"content": "valid"})
	if err != nil || created["scope"] != "workspace" {
		t.Fatalf("missing create scope must use the explicit contract default: memory=%#v err=%v", created, err)
	}
	if updated, found, err := store.Patch(created["id"].(string), map[string]any{"scope": "unknown"}); err == nil || found || updated != nil {
		t.Fatalf("invalid patch scope was accepted: memory=%#v found=%v err=%v", updated, found, err)
	}
}

func TestPersistentMemoryStoreDeleteAndDiagnostics(t *testing.T) {
	store, err := NewPersistentMemoryStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Create(map[string]any{
		"content":    "private-memory-content",
		"workspace":  "/private/workspace",
		"project":    "private-project",
		"tags":       []any{"private-tag"},
		"confidence": 0.25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(map[string]any{"content": "Second"}); err != nil {
		t.Fatal(err)
	}
	deleted, found, err := store.Delete(first["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !found || deleted["deletedAt"] == "" {
		t.Fatalf("delete should mark tombstone: found=%v deleted=%#v", found, deleted)
	}
	assertPrivacyPreservingMemoryTombstone(t, deleted)
	data, err := os.ReadFile(store.recordPath(first["id"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-") || strings.Contains(string(data), "content") {
		t.Fatalf("persisted tombstone retained deleted payload: %s", data)
	}
	active, err := store.List(false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active list should hide tombstones: %#v", active)
	}
	all, err := store.List(true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("include_deleted should return tombstones: %#v", all)
	}
	for _, item := range all {
		memory := item.(map[string]any)
		if memory["id"] == first["id"] {
			assertPrivacyPreservingMemoryTombstone(t, memory)
		}
	}
	if _, found, err := store.Patch(first["id"].(string), map[string]any{"content": "restored-secret"}); err != nil || found {
		t.Fatalf("deleted memory must not be writable: found=%v err=%v", found, err)
	}
	diagnostics := store.Diagnostics()
	if diagnostics["activeCount"] != float64(1) || diagnostics["tombstoneCount"] != float64(1) {
		t.Fatalf("diagnostics mismatch: %#v", diagnostics)
	}
}

func TestPersistentMemoryStoreIsManualOnlyAndRejectsCaseChannels(t *testing.T) {
	store, err := NewPersistentMemoryStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []map[string]any{
		{"content": "continue with acct:1"},
		{"content": "authority cer1_" + strings.Repeat("a", 64)},
		{"content": "source /private/case.duckdb"},
		{"content": `{"structuredContent":{"subjectRef":"private"}}`},
		{"content": "safe", "sourceThreadId": "thread-case"},
		{"content": "safe", "automaticCapture": true},
		{"content": "safe", "modelInjection": true},
	} {
		if memory, err := store.Create(body); err == nil || memory != nil {
			t.Fatalf("forbidden memory channel was accepted: body=%#v memory=%#v err=%v", body, memory, err)
		}
	}
	created, err := store.Create(map[string]any{"content": "Prefer deterministic focused tests"})
	if err != nil || created["provenance"] != "manual-general" || created["captureMode"] != "manual" || created["modelInjection"] != false {
		t.Fatalf("manual memory contract mismatch: memory=%#v err=%v", created, err)
	}
	if updated, found, err := store.Patch(created["id"].(string), map[string]any{
		"content": "continue with card:1",
	}); err == nil || found || updated != nil {
		t.Fatalf("case alias patch was accepted: updated=%#v found=%v err=%v", updated, found, err)
	}
	diagnostics := store.Diagnostics()
	if diagnostics["admittedProvenance"] != "manual-general" || diagnostics["captureMode"] != "manual" || diagnostics["modelInjection"] != false ||
		len(diagnostics["lastInjectedIds"].([]any)) != 0 {
		t.Fatalf("memory diagnostics advertised capture or injection: %#v", diagnostics)
	}
}

func TestPersistentMemoryStoreScrubsLegacyTombstonesOnStartup(t *testing.T) {
	dir := t.TempDir()
	recordsDir := filepath.Join(dir, "memory", "records")
	path := filepath.Join(recordsDir, "mem_go_7.json")
	if err := WriteJSONMapFile(path, map[string]any{
		"id":             "mem_go_7",
		"content":        "legacy-private-content",
		"scope":          "project",
		"workspace":      "/legacy/private",
		"project":        "legacy-project",
		"sourceThreadId": "legacy-thread",
		"sourceTurnId":   "legacy-turn",
		"tags":           []any{"legacy-tag"},
		"confidence":     0.4,
		"disabledAt":     "2026-01-01T00:00:00Z",
		"createdAt":      "2025-01-01T00:00:00Z",
		"updatedAt":      "2026-01-01T00:00:00Z",
		"deletedAt":      "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	store, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	all, err := store.List(true, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("legacy tombstone list mismatch: memories=%#v err=%v", all, err)
	}
	assertPrivacyPreservingMemoryTombstone(t, all[0].(map[string]any))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "legacy-") || strings.Contains(string(data), "content") {
		t.Fatalf("legacy tombstone payload remained on disk: %s", data)
	}
	next, err := store.Create(map[string]any{"content": "next"})
	if err != nil || next["id"] != "mem_go_8" {
		t.Fatalf("sequence after tombstone migration mismatch: next=%#v err=%v", next, err)
	}
}

func TestPersistentMemoryStoreWithholdsLegacyAutomaticRecords(t *testing.T) {
	dir := t.TempDir()
	recordsDir := filepath.Join(dir, "memory", "records")
	if err := WriteJSONMapFile(filepath.Join(recordsDir, "mem_go_9.json"), map[string]any{
		"id":             "mem_go_9",
		"content":        "legacy automatic content",
		"scope":          "workspace",
		"sourceThreadId": "legacy-thread",
		"createdAt":      "2026-01-01T00:00:00Z",
		"updatedAt":      "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	store, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(true, "")
	if err != nil || len(listed) != 0 {
		t.Fatalf("legacy automatic record was admitted: listed=%#v err=%v", listed, err)
	}
	if _, err := os.Stat(filepath.Join(recordsDir, "mem_go_9.json")); err != nil {
		t.Fatalf("withheld legacy record must remain untouched: %v", err)
	}
}

func TestPersistentMemoryStoreWithholdsTamperedCaseMetadata(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{"content": "general manual preference"})
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{}
	for key, value := range created {
		record[key] = value
	}
	record["tags"] = []any{"acct:1"}
	if err := WriteJSONMapFile(store.recordPath(created["id"].(string)), record); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(true, "")
	if err != nil || len(listed) != 0 {
		t.Fatalf("tampered case metadata was admitted: listed=%#v err=%v", listed, err)
	}
}

func TestPersistentMemoryStorePatchRejectsExistingTamperedRecordWithoutEchoOrRewrite(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{"content": "general manual preference"})
	if err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	tampered := contracts.CloneMap(created)
	tampered["sourceThreadId"] = "private-thread"
	tampered["authorityRef"] = "cer1_" + strings.Repeat("a", 64)
	tampered["content"] = "raw case content"
	if err := WriteJSONMapFile(store.recordPath(id), tampered); err != nil {
		t.Fatal(err)
	}

	updated, found, err := store.Patch(id, map[string]any{"disabled": true})
	if !errors.Is(err, domainmemory.ErrMutationNotAdmissibleV1) || found || updated != nil {
		t.Fatalf("tampered existing record was decorated by patch: updated=%#v found=%v err=%v", updated, found, err)
	}
	after, readErr := ReadJSONMapFile(store.recordPath(id))
	if readErr != nil || !reflect.DeepEqual(after, tampered) {
		t.Fatalf("rejected patch rewrote tampered record: after=%#v err=%v", after, readErr)
	}
}

func TestPersistentMemoryStoreWithholdsNonMinimalTombstoneCreatedAfterStartup(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := store.recordPath("mem_go_7")
	if err := WriteJSONMapFile(path, map[string]any{
		"id": "mem_go_7", "content": "runtime-private-content", "scope": "workspace",
		"tags": []any{}, "confidence": float64(1), "provenance": "manual-general",
		"captureMode": "manual", "modelInjection": false,
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
		"deletedAt": "2026-01-02T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List(true, "")
	if err != nil || len(listed) != 0 {
		t.Fatalf("runtime non-minimal tombstone was returned: listed=%#v err=%v", listed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "runtime-private-content") {
		t.Fatalf("withheld runtime tombstone should remain untouched: data=%s err=%v", data, err)
	}
}

func TestPersistentMemoryStoreRejectsAliasPatchDeleteAndFilenameBodyMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{"content": "canonical-content"})
	if err != nil || created["id"] != "mem_go_1" {
		t.Fatalf("create mismatch: memory=%#v err=%v", created, err)
	}
	canonicalPath := store.recordPath("mem_go_1")
	before, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"mem:go_1", "mem/go_1", " mem_go_1", "mem_go_01"} {
		if _, found, err := store.Patch(alias, map[string]any{"content": "attacker"}); err != nil || found {
			t.Fatalf("alias patch accepted: id=%q found=%v err=%v", alias, found, err)
		}
		if _, found, err := store.Delete(alias); err != nil || found {
			t.Fatalf("alias delete accepted: id=%q found=%v err=%v", alias, found, err)
		}
	}
	after, err := os.ReadFile(canonicalPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("alias mutated canonical record: before=%s after=%s err=%v", before, after, err)
	}

	mismatchedPath := filepath.Join(dir, "memory", "records", "mem_go_2.json")
	if err := WriteJSONMapFile(mismatchedPath, map[string]any{
		"id": "mem_go_3", "content": "mismatched", "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewPersistentMemoryStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := restarted.List(true, "")
	if err == nil || listed != nil {
		t.Fatalf("filename/body mismatch must make memory authority unavailable: listed=%#v err=%v", listed, err)
	}
	diagnostics := restarted.Diagnostics()
	if diagnostics["status"] != "unavailable" || diagnostics["activeCount"] != nil || diagnostics["tombstoneCount"] != nil {
		t.Fatalf("unavailable diagnostics must not collapse unknown counts to zero: %#v", diagnostics)
	}
}

func assertPrivacyPreservingMemoryTombstone(t *testing.T, memory map[string]any) {
	t.Helper()
	if !isPrivacyPreservingMemoryTombstone(memory) {
		t.Fatalf("expected minimal privacy-preserving tombstone, got %#v", memory)
	}
	for _, key := range []string{
		"content", "scope", "tags", "workspace", "project", "sourceThreadId", "sourceTurnId", "confidence", "disabledAt",
	} {
		if _, ok := memory[key]; ok {
			t.Fatalf("tombstone retained %q: %#v", key, memory)
		}
	}
}
