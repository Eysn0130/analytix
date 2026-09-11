package eventlog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func semanticPreservationFixtureV1(t *testing.T) (SemanticStartupContentMigrationInput, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"a-active", "z-held"} {
		dir := filepath.Join(root, "threads", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		item := map[string]any{"id": "result-1", "threadId": id, "turnId": "turn-1", "kind": "tool_result", "role": "tool", "status": "completed", "toolName": "read_file", "callId": "provider-call", "output": map[string]any{"secret": "RAW_" + id}}
		thread := map[string]any{"id": id, "status": "running", "turns": []any{map[string]any{"id": "turn-1", "threadId": id, "status": "running", "items": []any{item}}}}
		writeMigrationFixtureJSON(t, filepath.Join(dir, "thread.json"), thread)
		writeMigrationFixtureJSONL(t, filepath.Join(dir, "messages.jsonl"), []map[string]any{item})
		writeMigrationFixtureJSONL(t, filepath.Join(dir, "metadata.jsonl"), []map[string]any{{"kind": "thread_metadata", "thread": thread}})
		writeMigrationFixtureJSONL(t, filepath.Join(dir, "events.jsonl"), []map[string]any{
			{"kind": "tool_result", "threadId": id, "turnId": "turn-1", "seq": float64(2), "output": "RAW_" + id},
			{"kind": "item_completed", "threadId": id, "turnId": "turn-1", "seq": float64(1), "item": item},
		})
	}
	indexPath := filepath.Join(root, "thread_summaries.jsonl")
	heldFirst := " {\"schemaVersion\":1,\"threadId\":\"z-held\",\"summary\":{\"id\":\"z-held\",\"title\":\"HISTORY\"}}\r\n"
	heldLast := "\t{\"schemaVersion\":1,\"threadId\":\"z-held\",\"deleted\":true,\"summary\":{\"id\":\"z-held\",\"title\":\"DELETED\"}}"
	active := `{"schemaVersion":1,"threadId":"a-active","summary":{"id":"a-active","lastItem":{"kind":"tool_result","output":{"secret":"RAW_INDEX"}}}}`
	if err := os.WriteFile(indexPath, []byte(heldFirst+active+"\n"+heldLast), 0o600); err != nil {
		t.Fatal(err)
	}
	preserved, err := PrepareSemanticRestartPreservationV1(context.Background(), root, indexPath, []string{"z-held"})
	if err != nil {
		t.Fatal(err)
	}
	input := SemanticStartupContentMigrationInput{Root: root, ThreadSummaryIndexPath: indexPath, RestartPreservation: preserved}
	return input, semanticPreservationBytesV1(t, root)
}

func TestSemanticRestartPreservationReadsCompleteOriginalEventInventory(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "legacy"}[legacy], func(t *testing.T) {
			input, _ := semanticPreservationFixtureV1(t)
			if legacy {
				parent := filepath.Join(input.Root, "runtime-go", "threads")
				if err := os.MkdirAll(parent, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Join(input.Root, "threads", "z-held"), filepath.Join(parent, "z-held")); err != nil {
					t.Fatal(err)
				}
			}
			scope, err := PrepareSemanticRestartPreservationV1(context.Background(), input.Root, input.ThreadSummaryIndexPath, []string{"z-held"})
			if err != nil {
				t.Fatal(err)
			}
			before := semanticPreservationBytesV1(t, input.Root)
			for range 2 {
				rows, err := scope.ReadOriginalEventInventoryV1(context.Background(), "z-held")
				if err != nil || len(rows) != 2 || rows[0]["output"] != "RAW_z-held" || migrationStringField(rows[1], "kind") != "item_completed" {
					t.Fatalf("original event observation normalized or omitted rows: count=%d err=%v", len(rows), err)
				}
				if first, ok := exactEventSequenceV1(rows[0]["seq"]); !ok || first != 2 {
					t.Fatal("original observation reordered the preserved historical sequence")
				}
				rows[0]["output"] = "visitor mutation"
				assertSemanticOriginalBytesV1(t, input.Root, before)
			}
			if _, err := scope.ReadOriginalEventInventoryV1(context.Background(), "a-active"); err == nil {
				t.Fatal("preserved observation accepted an independent thread")
			}
			path := filepath.Join(scope.threadPaths["z-held"], "events.jsonl")
			if err := os.WriteFile(path, append(append([]byte(nil), before[path]...), ' '), 0o600); err != nil {
				t.Fatal(err)
			}
			if rows, err := scope.ReadOriginalEventInventoryV1(context.Background(), "z-held"); err == nil || rows != nil {
				t.Fatal("stale original event inventory remained observable")
			}
		})
	}
}

func semanticPreservationBytesV1(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err == nil {
			files[path] = body
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func assertSemanticOriginalBytesV1(t *testing.T, root string, want map[string][]byte) {
	t.Helper()
	got := semanticPreservationBytesV1(t, root)
	if len(want) != len(got) {
		t.Fatal("refusal changed file inventory")
	}
	for path, body := range want {
		if !bytes.Equal(body, got[path]) {
			t.Fatalf("refusal changed %s", filepath.Base(path))
		}
	}
}

func TestSemanticRestartPreservationAcrossCompleteMigrationAndRepeat(t *testing.T) {
	input, before := semanticPreservationFixtureV1(t)
	var stable map[string][]byte
	for iteration := 0; iteration < 2; iteration++ {
		if err := MigrateSemanticStartupContent(input); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"thread.json", "messages.jsonl", "metadata.jsonl", "events.jsonl"} {
			path := filepath.Join(input.Root, "threads", "z-held", file)
			body, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before[path], body) {
				t.Errorf("migration changed held %s", file)
			}
		}
		index, err := os.ReadFile(input.ThreadSummaryIndexPath)
		if err != nil {
			t.Fatal(err)
		}
		oldLines := bytes.Split(before[input.ThreadSummaryIndexPath], []byte("\n"))
		if !bytes.HasPrefix(index, append(append([]byte(nil), oldLines[0]...), '\n')) || !bytes.HasSuffix(index, oldLines[len(oldLines)-1]) || bytes.Contains(index, []byte("RAW_INDEX")) {
			t.Error("shared index failed held framing or independent cleanup")
		}
		if err := input.RestartPreservation.revalidate(context.Background(), input.Root, input.ThreadSummaryIndexPath); err != nil {
			t.Errorf("held scope did not survive chain: %v", err)
		}
		for path, body := range semanticPreservationBytesV1(t, filepath.Join(input.Root, "threads", "a-active")) {
			if bytes.Contains(body, []byte("RAW_a-active")) {
				t.Errorf("independent migration did not close %s", filepath.Base(path))
			}
		}
		thread := readMigrationFixtureObject(t, filepath.Join(input.Root, "threads", "a-active", "thread.json"))
		if thread["status"] != "idle" || thread["turns"].([]any)[0].(map[string]any)["status"] != "aborted" {
			t.Error("independent lifecycle closure did not run")
		}
		events := readMigrationFixtureLines(t, filepath.Join(input.Root, "threads", "a-active", "events.jsonl"))
		if len(events) != 2 || events[0]["seq"] != float64(1) || events[1]["seq"] != float64(2) {
			t.Error("independent event sequence migration did not run")
		}
		if iteration == 0 {
			stable = semanticPreservationBytesV1(t, input.Root)
		} else {
			assertSemanticOriginalBytesV1(t, input.Root, stable)
		}
	}
}

func TestSemanticRestartPreservationRefusesDriftAndWrongBindingBeforeWrites(t *testing.T) {
	for _, mode := range []string{"primary", "sidecar", "missing_sidecar", "new_file", "index", "root"} {
		t.Run(mode, func(t *testing.T) {
			input, _ := semanticPreservationFixtureV1(t)
			dir := filepath.Join(input.Root, "threads", "z-held")
			switch mode {
			case "primary", "sidecar":
				name := "thread.json"
				if mode == "sidecar" {
					name = "messages.jsonl"
				}
				path := filepath.Join(dir, name)
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append([]byte(" "), body...), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing_sidecar":
				if err := os.Remove(filepath.Join(dir, "metadata.jsonl")); err != nil {
					t.Fatal(err)
				}
			case "new_file":
				if err := os.WriteFile(filepath.Join(dir, "added.json"), []byte(`{}`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "index":
				body, err := os.ReadFile(input.ThreadSummaryIndexPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(input.ThreadSummaryIndexPath, append([]byte(" "), body...), 0o600); err != nil {
					t.Fatal(err)
				}
			case "root":
				input.Root = filepath.Join(input.Root, ".") + string(filepath.Separator)
			}
			before := semanticPreservationBytesV1(t, input.Root)
			if err := MigrateSemanticStartupContent(input); err == nil {
				t.Error("migration accepted stale or differently bound preservation")
			}
			assertSemanticOriginalBytesV1(t, input.Root, before)
		})
	}
}

func TestSemanticRestartPreservationDoesNotExcludeHeldCoreCorruption(t *testing.T) {
	for _, mode := range []string{"current_primary", "derived_current_primary", "derived_turn_authority", "event_duplicate", "event_terminal", "malformed_sidecar", "foreign_metadata", "missing_metadata_thread"} {
		t.Run(mode, func(t *testing.T) {
			input, _ := semanticPreservationFixtureV1(t)
			dir := filepath.Join(input.Root, "threads", "z-held")
			switch mode {
			case "current_primary", "derived_current_primary", "derived_turn_authority":
				thread := readMigrationFixtureObject(t, filepath.Join(dir, "thread.json"))
				thread["securityState"] = nil
				if mode != "current_primary" {
					thread["forkedFromThreadId"] = "source"
					if mode == "derived_turn_authority" {
						delete(thread, "securityState")
						thread["turns"].([]any)[0].(map[string]any)["securityContext"] = map[string]any{}
					}
					events := readMigrationFixtureLines(t, filepath.Join(dir, "events.jsonl"))
					events[0], events[1] = events[1], events[0]
					writeMigrationFixtureJSONL(t, filepath.Join(dir, "events.jsonl"), events)
				}
				writeMigrationFixtureJSON(t, filepath.Join(dir, "thread.json"), thread)
			case "event_duplicate", "event_terminal":
				records := readMigrationFixtureLines(t, filepath.Join(dir, "events.jsonl"))
				if mode == "event_duplicate" {
					records[1]["seq"] = records[0]["seq"]
				} else {
					records[0]["publicationSlot"] = "assistant-final"
					records[0]["publicationCommitId"] = strings.Repeat("a", 64)
				}
				writeMigrationFixtureJSONL(t, filepath.Join(dir, "events.jsonl"), records)
			case "malformed_sidecar":
				if err := os.WriteFile(filepath.Join(dir, "metadata.jsonl"), []byte(`{"thread":{},"thread":{}}`+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "foreign_metadata", "missing_metadata_thread":
				records := readMigrationFixtureLines(t, filepath.Join(dir, "metadata.jsonl"))
				if mode == "foreign_metadata" {
					records[0]["thread"].(map[string]any)["id"] = "foreign"
				} else {
					records[0]["thread"] = nil
				}
				writeMigrationFixtureJSONL(t, filepath.Join(dir, "metadata.jsonl"), records)
			}
			before := semanticPreservationBytesV1(t, input.Root)
			preserved, err := PrepareSemanticRestartPreservationV1(context.Background(), input.Root, input.ThreadSummaryIndexPath, []string{"z-held"})
			if err == nil {
				input.RestartPreservation = preserved
				err = MigrateSemanticStartupContent(input)
			}
			if err == nil {
				t.Error("held Core damage became an isolation exception")
			}
			assertSemanticOriginalBytesV1(t, input.Root, before)
		})
	}
}

func TestSemanticRestartPreservationRequiresOriginalPrimaryAndStrictScope(t *testing.T) {
	input, _ := semanticPreservationFixtureV1(t)
	for _, ids := range [][]string{{"z-held", "z-held"}, {" z-held"}, {"missing"}} {
		if scope, err := PrepareSemanticRestartPreservationV1(context.Background(), input.Root, input.ThreadSummaryIndexPath, ids); err == nil || scope != nil {
			t.Fatal("invalid scope acquired preservation")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareSemanticRestartPreservationV1(ctx, input.Root, input.ThreadSummaryIndexPath, []string{"z-held"}); err == nil {
		t.Fatal("cancelled preparation succeeded")
	}
	// The fixture is deliberately a local consumer test; it does not mint or
	// establish a trusted pending-report receipt, case identity, or capability.
	var decoded map[string]any
	body, err := os.ReadFile(filepath.Join(input.Root, "threads", "z-held", "thread.json"))
	if err != nil || json.Unmarshal(body, &decoded) != nil {
		t.Fatal("invalid fixture")
	}
}

func TestSemanticRestartPreservationRejectsDirectoryInKnownFileSlot(t *testing.T) {
	for _, name := range []string{"thread.json", "messages.jsonl", "metadata.jsonl", "events.jsonl"} {
		t.Run(name, func(t *testing.T) {
			input, _ := semanticPreservationFixtureV1(t)
			path := filepath.Join(input.Root, "threads", "z-held", name)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			before := semanticPreservationBytesV1(t, input.Root)
			preserved, err := PrepareSemanticRestartPreservationV1(context.Background(), input.Root, input.ThreadSummaryIndexPath, []string{"z-held"})
			if err == nil {
				input.RestartPreservation = preserved
				err = MigrateSemanticStartupContent(input)
			}
			if err == nil {
				t.Error("directory in known file slot accepted")
			}
			assertSemanticOriginalBytesV1(t, input.Root, before)
		})
	}
}

func TestSemanticRestartPreservationKeepsValidCurrentGrantAndRejectsChangedBinding(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "foreign_event_grant"}[corrupt], func(t *testing.T) {
			input, _ := semanticPreservationFixtureV1(t)
			id := "z-held"
			dir := filepath.Join(input.Root, "threads", id)
			now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
			securityContext := migrationExecutionContext(t, id, "turn-1", now)
			grant := migrationGrant(securityContext, migrationHostToolCallID("preserved-current"), "SYNTHETIC_ARGUMENT", now)
			call := migrationToolCallItem(id, securityContext, grant, "SYNTHETIC_ARGUMENT")
			thread := map[string]any{"id": id, "status": "running", "securityState": securityContext, "turns": []any{map[string]any{"id": "turn-1", "threadId": id, "status": "running", "securityContext": securityContext, "items": []any{call}}}}
			writeMigrationFixtureJSON(t, filepath.Join(dir, "thread.json"), thread)
			writeMigrationFixtureJSONL(t, filepath.Join(dir, "messages.jsonl"), []map[string]any{call})
			writeMigrationFixtureJSONL(t, filepath.Join(dir, "metadata.jsonl"), []map[string]any{{"kind": "thread_metadata", "thread": thread}})
			event := migrationReadyEvent(id, securityContext, grant, 1)
			if corrupt {
				event["executionGrantId"] = strings.Repeat("f", 64)
			}
			writeMigrationFixtureJSONL(t, filepath.Join(dir, "events.jsonl"), []map[string]any{event})
			before := semanticPreservationBytesV1(t, input.Root)
			preserved, err := PrepareSemanticRestartPreservationV1(context.Background(), input.Root, input.ThreadSummaryIndexPath, []string{id})
			if corrupt {
				if err == nil {
					t.Fatal("changed current event binding acquired scope")
				}
				assertSemanticOriginalBytesV1(t, input.Root, before)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			input.RestartPreservation = preserved
			for i := 0; i < 2; i++ {
				if err := MigrateSemanticStartupContent(input); err != nil {
					t.Fatal(err)
				}
				for path, body := range semanticPreservationBytesV1(t, dir) {
					if !bytes.Equal(body, before[path]) {
						t.Fatalf("valid current authority changed in %s", filepath.Base(path))
					}
				}
			}
		})
	}
}

func TestSemanticRestartPreservationBindsOriginalLegacyFamily(t *testing.T) {
	input, _ := semanticPreservationFixtureV1(t)
	current := filepath.Join(input.Root, "threads", "z-held")
	legacy := filepath.Join(input.Root, "runtime-go", "threads", "z-held")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(current, legacy); err != nil {
		t.Fatal(err)
	}
	preserved, err := PrepareSemanticRestartPreservationV1(context.Background(), input.Root, input.ThreadSummaryIndexPath, []string{"z-held"})
	if err != nil {
		t.Fatal(err)
	}
	input.RestartPreservation = preserved
	before := semanticPreservationBytesV1(t, legacy)
	for i := 0; i < 2; i++ {
		if err := MigrateSemanticStartupContent(input); err != nil {
			t.Fatal(err)
		}
		assertSemanticOriginalBytesV1(t, legacy, before)
		if _, err := os.Lstat(current); !os.IsNotExist(err) {
			t.Fatal("legacy preservation created a current primary")
		}
	}
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := preserved.Revalidate(context.Background(), input.Root); err == nil {
		t.Fatal("new current-family shadow accepted")
	}
	if _, err := PrepareSemanticRestartPreservationV1(context.Background(), input.Root, input.ThreadSummaryIndexPath, []string{"z-held"}); err == nil {
		t.Fatal("ambiguous two-family scope accepted")
	}
}
