package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/jobs"
)

type legacyTypeScriptThreadReaderStubV1 struct {
	thread map[string]any
	err    error
}

func TestLegacyTypeScriptRawThreadReaderFreezesSidecarLineageBeforeClosedHydration(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_parent"
	threadDir := filepath.Join(root, "threads", threadID)
	childRoot := filepath.Join(root, "child-runs")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(childRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := []byte(`{"kind":"thread_metadata","thread":{"id":"thr_parent","turns":[{"id":"turn_parent","threadId":"thr_parent","status":"completed","items":[]}]}}` + "\n")
	if err := os.WriteFile(filepath.Join(threadDir, "metadata.jsonl"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	messages := []byte(
		`{"id":"redacted","threadId":"thr_parent","turnId":"turn_parent","kind":"content_redacted","code":"private_message_content_removed"}` + "\n" +
			`{"id":"item_parent","threadId":"thr_parent","turnId":"turn_parent","kind":"tool_call","status":"completed","toolName":"parallel_tasks","callId":"call_parent","arguments":{"private":"must-not-enter-witness"}}` + "\n",
	)
	if err := os.WriteFile(filepath.Join(threadDir, "messages.jsonl"), messages, 0o600); err != nil {
		t.Fatal(err)
	}
	child := []byte(`{"id":"child_mr1b6yo0_abc123","parentThreadId":"thr_parent","parentTurnId":"turn_parent","parentToolCallId":"call_parent","childThreadId":"child_mr1b6yo0_abc123","childTurnId":"turn_child","prompt":"private","status":"completed","usage":{},"toolInvocations":1,"durationMs":1,"queuedMs":1,"createdAt":"2026-07-01T00:00:00.000Z","startedAt":"2026-07-01T00:00:00.001Z","updatedAt":"2026-07-01T00:00:00.002Z"}`)
	if err := os.WriteFile(filepath.Join(childRoot, "child_mr1b6yo0_abc123.json"), child, 0o600); err != nil {
		t.Fatal(err)
	}

	reader := legacyLineageReaderForTestV1(t, root, threadID)
	thread, err := reader.GetThread(threadID)
	if err != nil {
		t.Fatalf("read raw staged lineage: %v", err)
	}
	items := runtimeappListAnyV1(runtimeappListAnyV1(thread["turns"])[0].(map[string]any)["items"])
	if len(items) != 1 || runtimeappStringFieldV1(items[0].(map[string]any), "callId") != "call_parent" {
		t.Fatalf("raw lineage reader did not retain only the exact tool call: %#v", items)
	}
	roots, _ := reader.authority.Roots()
	lineage, err := prepareRuntimeRetiredTypeScriptAuditLineageV1(context.Background(), roots, reader.authority, legacyLineageEntriesForTestV1(reader))
	if err != nil {
		t.Fatalf("freeze sidecar-only raw lineage: %v", err)
	}
	if _, exists := lineage.parentIDs[threadID]; !exists {
		t.Fatal("prepared lineage did not retain the witnessed parent identity")
	}
	witness := lineage.witness
	observation, err := jobs.BuildLegacyTypeScriptLineageObservationV1(childRoot)
	if err != nil || len(observation.Entries) != 1 {
		t.Fatalf("observe child source: entries=%d err=%v", len(observation.Entries), err)
	}
	if err := witness.VerifyLegacyTypeScriptLineageV1(observation.Entries[0].Lineage); err != nil {
		t.Fatalf("frozen raw lineage witness rejected exact source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "metadata.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lineage.witness.VerifyLegacyTypeScriptLineageV1(observation.Entries[0].Lineage); err != nil {
		t.Fatalf("prepared witness reread parent content after repair: %v", err)
	}
}

func TestLegacyTypeScriptRawThreadReaderRejectsAmbiguousSidecarJSON(t *testing.T) {
	root := t.TempDir()
	threadDir := filepath.Join(root, "threads", "thr_parent")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := []byte(`{"kind":"thread_metadata","thread":{"id":"thr_parent","turns":[{"id":"turn_parent","items":[]}]}}` + "\n")
	if err := os.WriteFile(filepath.Join(threadDir, "metadata.jsonl"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	ambiguous := []byte(`{"id":"item_parent","id":"item_attacker","threadId":"thr_parent","turnId":"turn_parent","kind":"tool_call","toolName":"delegate_task","callId":"call_parent"}` + "\n")
	if err := os.WriteFile(filepath.Join(threadDir, "messages.jsonl"), ambiguous, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyLineageReaderForTestV1(t, root, "thr_parent").GetThread("thr_parent"); err == nil {
		t.Fatal("ambiguous raw sidecar lineage was accepted")
	}
}

// Build expected states once from the fixture, before each test mutates it.
// Production callers instead pass their already-captured startup snapshot.
func legacyLineageReaderForTestV1(t *testing.T, root, threadID string) legacyTypeScriptRawThreadReaderV1 {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("secure managed reader is unsupported")
	}
	reader := legacyTypeScriptRawThreadReaderV1{entries: map[string]persistencefs.EntryRecord{}}
	for _, name := range []string{"", "thread.json", "metadata.jsonl", "messages.jsonl"} {
		path := filepath.Join(root, "threads", threadID, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		label := "durable/threads/" + threadID
		entry := persistencefs.EntryRecord{Path: label, Type: "directory", Mode: uint32(info.Mode()), Size: info.Size()}
		if name != "" {
			entry.Path += "/" + name
			entry.Type = "file"
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			entry.SHA256 = domainsecurity.SHA256Hex(body)
		}
		reader.entries[entry.Path] = entry
	}
	authority, err := persistencefs.FreezeRootAuthority(persistencefs.RootSet{DataDir: root, DurableDir: root})
	if err != nil {
		t.Fatal(err)
	}
	return newLegacyTypeScriptRawThreadReaderV1(context.Background(), authority, legacyLineageEntriesForTestV1(reader))
}

func legacyLineageEntriesForTestV1(reader legacyTypeScriptRawThreadReaderV1) []persistencefs.EntryRecord {
	entries := make([]persistencefs.EntryRecord, 0, len(reader.entries))
	for _, entry := range reader.entries {
		entries = append(entries, entry)
	}
	return entries
}

func TestLegacyTypeScriptRawThreadReaderSnapshotDriftDoesNotFallback(t *testing.T) {
	for _, source := range []string{"thread.json", "metadata.jsonl", "messages.jsonl"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "threads", "thr_parent")
			if err := os.MkdirAll(directory, 0o755); err != nil {
				t.Fatal(err)
			}
			thread := `{"id":"thr_parent","turns":[]}`
			files := map[string]string{"metadata.jsonl": `{"kind":"thread_metadata","thread":` + thread + "}\n"}
			if source == "thread.json" {
				files[source] = thread
			}
			if source == "messages.jsonl" {
				files[source] = ""
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			reader := legacyLineageReaderForTestV1(t, root, "thr_parent")
			if _, err := reader.GetThread("thr_parent"); err != nil {
				t.Fatalf("ordinary managed source modes were rejected: %v", err)
			}
			if err := os.Remove(filepath.Join(directory, source)); err != nil {
				t.Fatal(err)
			}
			if _, err := reader.GetThread("thr_parent"); err == nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("snapshot source disappearance became a fallback: %v", err)
			}
		})
	}
}

func TestLegacyTypeScriptRawThreadReaderRejectsThreadsAncestorSymlink(t *testing.T) {
	root := t.TempDir()
	threads := filepath.Join(root, "threads")
	directory := filepath.Join(threads, "thr_parent")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "thread.json"), []byte(`{"id":"thr_parent","turns":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := legacyLineageReaderForTestV1(t, root, "thr_parent")
	retired := filepath.Join(t.TempDir(), "original-threads")
	if err := os.Rename(threads, retired); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(retired, threads); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := reader.GetThread("thr_parent"); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("threads ancestor redirection was accepted or treated as absent: %v", err)
	}
}

func TestLegacyLineageJSONLRetainsSixteenMiBLineBound(t *testing.T) {
	// A record above the ordinary snapshot scanner's 4 MiB line size remains
	// legal in this private raw parser, subject to its existing string budget.
	if _, err := readLegacyLineageJSONLV1([]byte(`{"value":"` + strings.Repeat("x", 5<<20) + "\"}\n")); err != nil {
		t.Fatalf("legacy JSONL line below 16 MiB was rejected: %v", err)
	}
	if _, err := readLegacyLineageJSONLV1([]byte(strings.Repeat(" ", 16<<20) + "\n")); err == nil {
		t.Fatal("legacy JSONL line at the scanner bound was accepted")
	}
}

func TestFrozenLegacyTypeScriptChildLineageWitnessSurvivesParentRedactionAndRejectsSourceDrift(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "child_mr1b6yo0_abc123.json")
	body := []byte(`{
  "id":"child_mr1b6yo0_abc123",
  "parentThreadId":"thr_parent",
  "parentTurnId":"turn_parent",
  "parentToolCallId":"call_parent",
  "childThreadId":"child_mr1b6yo0_abc123",
  "childTurnId":"turn_child",
  "prompt":"PRIVATE_PROMPT_MUST_NOT_ENTER_WITNESS",
  "status":"completed",
  "usage":{},
  "toolInvocations":1,
  "durationMs":1,
  "queuedMs":1,
  "createdAt":"2026-07-01T00:00:00.000Z",
  "startedAt":"2026-07-01T00:00:00.001Z",
  "updatedAt":"2026-07-01T00:00:00.002Z"
}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{
		"id": "thr_parent",
		"turns": []any{map[string]any{
			"id": "turn_parent",
			"items": []any{map[string]any{
				"kind": "tool_call", "callId": "call_parent", "turnId": "turn_parent", "toolName": "delegate_task",
			}},
		}},
	}
	observation, err := jobs.BuildLegacyTypeScriptLineageObservationV1(root)
	if err != nil || len(observation.Entries) != 1 {
		t.Fatalf("observe legacy source: entries=%d err=%v", len(observation.Entries), err)
	}
	witness, err := newFrozenLegacyTypeScriptChildLineageWitnessV1(
		root, legacyTypeScriptThreadReaderStubV1{thread: valid},
	)
	if err != nil {
		t.Fatalf("freeze pre-redaction witness: %v", err)
	}
	// A post-redaction reader can no longer supply the retired tool call. The
	// witness is self-contained and hash-only, so exact source-bound lineage
	// remains verifiable without rereading the removed payload.
	if err := witness.VerifyLegacyTypeScriptLineageV1(observation.Entries[0].Lineage); err != nil {
		t.Fatalf("exact frozen lineage was rejected after redaction: %v", err)
	}
	unknown := observation.Entries[0].Lineage
	unknown.ParentToolCallID = "call_fabricated"
	if err := witness.VerifyLegacyTypeScriptLineageV1(unknown); err == nil {
		t.Fatal("lineage absent from the pre-redaction witness was accepted")
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.NewManagerForSemanticStartupWithWitness(root, root, nil, witness); err == nil {
		t.Fatal("changed child-run source inventory was accepted by the frozen witness")
	}
}

func TestFrozenLegacyTypeScriptChildLineageWitnessRejectsRemovedSourceRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "child-runs")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "child_mr1b6yo0_abc123.json")
	body := []byte(`{"id":"child_mr1b6yo0_abc123","parentThreadId":"thr_parent","parentTurnId":"turn_parent","parentToolCallId":"call_parent","childThreadId":"child_mr1b6yo0_abc123","prompt":"private","status":"completed","usage":{},"toolInvocations":1,"durationMs":1,"queuedMs":1,"createdAt":"2026-07-01T00:00:00.000Z","startedAt":"2026-07-01T00:00:00.001Z","updatedAt":"2026-07-01T00:00:00.002Z"}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{"id": "thr_parent", "turns": []any{map[string]any{
		"id": "turn_parent", "items": []any{map[string]any{
			"kind": "tool_call", "callId": "call_parent", "turnId": "turn_parent", "toolName": "parallel_tasks",
		}},
	}}}
	witness, err := newFrozenLegacyTypeScriptChildLineageWitnessV1(root, legacyTypeScriptThreadReaderStubV1{thread: thread})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.NewManagerForSemanticStartupWithWitness(root, root, nil, witness); err == nil {
		t.Fatal("removed child-run source root bypassed the frozen inventory witness")
	}
}

func (stub legacyTypeScriptThreadReaderStubV1) GetThread(string) (map[string]any, error) {
	return stub.thread, stub.err
}

func TestLegacyTypeScriptChildLineageVerifierRequiresExactParentTurnAndToolCall(t *testing.T) {
	lineage := jobs.LegacyTypeScriptLineageV1{
		ParentThreadID: "thr_parent", ParentTurnID: "turn_parent", ParentToolCallID: "call_parent",
	}
	valid := map[string]any{
		"id": "thr_parent",
		"turns": []any{map[string]any{
			"id": "turn_parent",
			"items": []any{map[string]any{
				"kind": "tool_call", "callId": "call_parent", "turnId": "turn_parent", "toolName": "delegate_task",
			}},
		}},
	}
	if err := newLegacyTypeScriptChildLineageVerifierV1(legacyTypeScriptThreadReaderStubV1{thread: valid}).VerifyLegacyTypeScriptLineageV1(lineage); err != nil {
		t.Fatalf("exact staged lineage was rejected: %v", err)
	}
	fixtures := map[string]legacyTypeScriptThreadReaderStubV1{
		"reader-error": {err: errors.New("unavailable")},
		"wrong-thread": {thread: map[string]any{"id": "thr_other", "turns": valid["turns"]}},
		"missing-turn": {thread: map[string]any{"id": "thr_parent", "turns": []any{}}},
		"missing-call": {thread: map[string]any{"id": "thr_parent", "turns": []any{map[string]any{
			"id": "turn_parent", "items": []any{map[string]any{"kind": "tool_call", "callId": "call_other", "toolName": "delegate_task"}},
		}}}},
		"duplicate-call": {thread: map[string]any{"id": "thr_parent", "turns": []any{map[string]any{
			"id": "turn_parent", "items": []any{
				map[string]any{"kind": "tool_call", "callId": "call_parent", "toolName": "delegate_task"},
				map[string]any{"kind": "tool_call", "callId": "call_parent", "toolName": "task"},
			},
		}}}},
		"cross-turn-item": {thread: map[string]any{"id": "thr_parent", "turns": []any{map[string]any{
			"id": "turn_parent", "items": []any{map[string]any{
				"kind": "tool_call", "callId": "call_parent", "turnId": "turn_other", "toolName": "delegate_task",
			}},
		}}}},
		"wrong-tool": {thread: map[string]any{"id": "thr_parent", "turns": []any{map[string]any{
			"id": "turn_parent", "items": []any{map[string]any{
				"kind": "tool_call", "callId": "call_parent", "turnId": "turn_parent", "toolName": "read_file",
			}},
		}}}},
		"duplicate-cross-turn": {thread: map[string]any{"id": "thr_parent", "turns": []any{
			map[string]any{"id": "turn_parent", "items": []any{map[string]any{
				"kind": "tool_call", "callId": "call_parent", "toolName": "delegate_task",
			}}},
			map[string]any{"id": "turn_other", "items": []any{map[string]any{
				"kind": "tool_call", "callId": "call_parent", "toolName": "parallel_tasks",
			}}},
		}}},
		"duplicate-wrong-tool": {thread: map[string]any{"id": "thr_parent", "turns": []any{map[string]any{
			"id": "turn_parent", "items": []any{
				map[string]any{"kind": "tool_call", "callId": "call_parent", "toolName": "delegate_task"},
				map[string]any{"kind": "tool_call", "callId": "call_parent", "toolName": "read_file"},
			},
		}}}},
	}
	for name, reader := range fixtures {
		t.Run(name, func(t *testing.T) {
			if err := newLegacyTypeScriptChildLineageVerifierV1(reader).VerifyLegacyTypeScriptLineageV1(lineage); err == nil {
				t.Fatal("invalid staged lineage was accepted")
			}
		})
	}
	if err := newLegacyTypeScriptChildLineageVerifierV1(legacyTypeScriptThreadReaderStubV1{}).VerifyLegacyTypeScriptLineageV1(lineage); err == nil {
		t.Fatal("missing staged parent was accepted")
	}
	if err := newLegacyTypeScriptChildLineageVerifierV1(legacyTypeScriptThreadReaderStubV1{err: errors.New("storage failed")}).VerifyLegacyTypeScriptLineageV1(lineage); err == nil {
		t.Fatal("storage failure was accepted")
	}
}
