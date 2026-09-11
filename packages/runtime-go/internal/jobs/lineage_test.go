package jobs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestReserveBackgroundAutoContinueV1LinearizesSiblingCallbacks(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const siblings = 32
	records := make([]Record, 0, siblings)
	for index := 0; index < siblings; index++ {
		record, startErr := manager.StartChildRun(StartRequest{
			ParentGoalID: "goal-auto", ParentThreadID: "thread-auto", ParentTurnID: "turn-parent",
			ChildThreadID: fmt.Sprintf("child-thread-%d", index), ChildTurnID: fmt.Sprintf("child-turn-%d", index),
			Kind: "subagent", Status: string(domainjob.StatusCompleted), Background: true, AutoContinueParent: true,
		})
		if startErr != nil {
			t.Fatal(startErr)
		}
		record, err = manager.UpdateChildRun(record.ID, UpdateRequest{
			CompletionDeliveryID: "delivery-" + record.ID, CompletionDeliveryItemID: "item-" + record.ID,
			CompletionDeliveryStatus: "delivered",
		})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	var group sync.WaitGroup
	errs := make(chan error, siblings)
	winners := make(chan string, siblings)
	for index, record := range records {
		group.Add(1)
		go func(index int, record Record) {
			defer group.Done()
			reserved, won, reserveErr := manager.ReserveBackgroundAutoContinueV1(
				record.ID, fmt.Sprintf("turn_%d", index+1),
				func(current Record, all []Record) string {
					for _, other := range all {
						if other.ID == current.ID || other.ParentThreadID != current.ParentThreadID ||
							other.ParentTurnID != current.ParentTurnID {
							continue
						}
						if other.AutoContinueStatus == "starting" || other.AutoContinueStatus == "started" {
							return "auto_continue_already_starting"
						}
					}
					return ""
				},
			)
			if reserveErr != nil {
				errs <- reserveErr
				return
			}
			if won {
				winners <- reserved.ID
			}
		}(index, record)
	}
	group.Wait()
	close(errs)
	close(winners)
	for err := range errs {
		t.Fatal(err)
	}
	winnerCount := 0
	for range winners {
		winnerCount++
	}
	if winnerCount != 1 {
		t.Fatalf("durable auto-continue reservations=%d want=1", winnerCount)
	}
	starting, skipped := 0, 0
	for _, record := range manager.AllRecords() {
		switch record.AutoContinueStatus {
		case "starting":
			starting++
			if record.AutoContinueTurnID == "" || record.AutoContinueReason != "" {
				t.Fatalf("winner reservation mismatch: %#v", record)
			}
		case "skipped":
			skipped++
			if record.AutoContinueTurnID != "" || record.AutoContinueReason != "auto_continue_already_starting" {
				t.Fatalf("loser reservation mismatch: %#v", record)
			}
		default:
			t.Fatalf("unclosed reservation state: %#v", record)
		}
	}
	if starting != 1 || skipped != siblings-1 {
		t.Fatalf("reservation states starting=%d skipped=%d", starting, skipped)
	}
}

func TestReserveBackgroundAutoContinueV1UnknownGateReasonFailsClosedWithoutReflection(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-auto", ParentThreadID: "thread-auto", ParentTurnID: "turn-parent",
		ChildThreadID: "child-thread", ChildTurnID: "child-turn", Kind: "subagent",
		Status: string(domainjob.StatusCompleted), Background: true, AutoContinueParent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err = manager.UpdateChildRun(record.ID, UpdateRequest{
		CompletionDeliveryID: "delivery-unknown", CompletionDeliveryItemID: "item-unknown",
		CompletionDeliveryStatus: "delivered",
	})
	if err != nil {
		t.Fatal(err)
	}
	const privateReason = "PRIVATE_UNKNOWN_GATE_REASON_/Users/private_13800138000"
	reserved, won, err := manager.ReserveBackgroundAutoContinueV1(
		record.ID, "turn_8", func(Record, []Record) string { return privateReason },
	)
	if err != nil || won || reserved.AutoContinueStatus != "skipped" ||
		reserved.AutoContinueTurnID != "" || reserved.AutoContinueReason != "job_auto_continue_state_changed" {
		t.Fatalf("unknown gate reason did not fail closed: record=%#v won=%t err=%v", reserved, won, err)
	}
	body, err := os.ReadFile(filepath.Join(root, record.ID+".json"))
	if err != nil || bytes.Contains(body, []byte(privateReason)) || bytes.Contains(body, []byte("13800138000")) {
		t.Fatalf("unknown gate reason was reflected durably: err=%v body=%s", err, body)
	}
}

func validJobModelExecutionFixtureV1() map[string]any {
	return map[string]any{
		"providerId": "deepseek", "modelId": "deepseek-chat", "source": "runtime-default",
		"resolvedAt": "2026-07-18T12:34:56.123456789Z", "endpointFormat": "chat_completions",
		"baseUrlFingerprint": strings.Repeat("a", 64), "capabilityFingerprint": strings.Repeat("b", 64),
	}
}

func validJobShellUsageFixtureV1() map[string]any {
	return map[string]any{
		"exitCode": float64(0), "durationMs": float64(5), "outputBytes": float64(3),
		"maxOutputBytes": float64(1024), "truncated": false, "timedOut": false,
	}
}

func TestReasoningEffortRejectsBeforeIdentityOrDurableMutation(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	const privateEffort = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	if _, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "child-run", Effort: privateEffort,
	}); err == nil || strings.Contains(err.Error(), privateEffort) {
		t.Fatalf("malformed reasoning effort was accepted or reflected: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 || len(manager.AllRecords()) != 0 {
		t.Fatalf("rejected reasoning effort mutated job authority: entries=%#v records=%#v err=%v", entries, manager.AllRecords(), err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "child-run", Effort: "auto",
	})
	if err != nil || record.ID != "job-1" || record.Effort != "auto" {
		t.Fatalf("rejected start consumed identity or valid effort was changed: record=%#v err=%v", record, err)
	}
	path := filepath.Join(root, record.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.jobs[0].Effort = privateEffort
	manager.mu.Unlock()
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: "completed"}); err == nil || strings.Contains(err.Error(), privateEffort) {
		t.Fatalf("poisoned in-memory reasoning effort was repaired or reflected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected poisoned effort changed durable bytes: err=%v before=%s after=%s", err, before, after)
	}
}

func TestSemanticStartupRemovesInvalidReasoningEffortAndPreservesAuto(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "child-run", Effort: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "child-run", Effort: "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, first.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	persisted["effort"] = "<think>PRIVATE_REASONING_SENTINEL</think>"
	poisoned, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, poisoned, 0o600); err != nil {
		t.Fatalf("write legacy fixture: %v", err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("live startup accepted an invalid persisted reasoning effort")
	}
	migrated, err := NewManagerForSemanticStartup(root, root)
	if err != nil {
		t.Fatal(err)
	}
	records := migrated.AllRecords()
	if len(records) != 2 || records[0].ID != first.ID || records[0].Effort != "" || records[1].ID != second.ID || records[1].Effort != "auto" {
		t.Fatalf("semantic reasoning-effort migration mismatch: %#v", records)
	}
	firstPass, err := os.ReadFile(path)
	if err != nil || bytes.Contains(firstPass, []byte("PRIVATE_REASONING")) || bytes.Contains(firstPass, []byte(`"effort"`)) {
		t.Fatalf("semantic migration retained invalid effort: err=%v body=%s", err, firstPass)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatal(err)
	}
	secondPass, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(firstPass, secondPass) {
		t.Fatalf("semantic reasoning-effort migration was not byte-stable: err=%v first=%s second=%s", err, firstPass, secondPass)
	}
}

func TestLiveJobMetadataRejectsUnknownContentWithoutDurableMutation(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	invalidModel := validJobModelExecutionFixtureV1()
	invalidModel["reasoningContent"] = "PRIVATE_REASONING_SENTINEL"
	if _, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: "running", ModelExecution: invalidModel,
	}); !errors.Is(err, domainjob.ErrPersistedModelExecutionInvalid) {
		t.Fatalf("unknown model execution bytes were accepted: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected model execution mutated storage: entries=%#v err=%v", entries, err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: "running", ModelExecution: validJobModelExecutionFixtureV1(),
	})
	if err != nil || record.ID != "job-1" {
		t.Fatalf("rejected start consumed identity or blocked valid metadata: record=%#v err=%v", record, err)
	}
	path := filepath.Join(root, record.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	poisonedUsage := validJobShellUsageFixtureV1()
	poisonedUsage["rawProviderResponse"] = "PRIVATE_PROVIDER_BODY_SENTINEL"
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Usage: poisonedUsage}); !errors.Is(err, domainjob.ErrPersistedUsageInvalid) {
		t.Fatalf("unknown job usage bytes were accepted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected job usage changed durable bytes: err=%v before=%s after=%s", err, before, after)
	}
}

func TestReturnedJobRecordsCannotMutateManagerAuthority(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: "running", Usage: validJobShellUsageFixtureV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	record.Usage["rawProviderResponse"] = "PRIVATE_PROVIDER_BODY_SENTINEL"

	updated, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	updated.Usage["reasoningContent"] = "PRIVATE_REASONING_SENTINEL"
	updated.ToolScope = append(updated.ToolScope, "caller-mutated")

	records := manager.AllRecords()
	if len(records) != 1 || records[0].Status != "completed" {
		t.Fatalf("unexpected manager authority: %#v", records)
	}
	if _, found := records[0].Usage["rawProviderResponse"]; found {
		t.Fatalf("start return shared usage authority with manager: %#v", records[0].Usage)
	}
	if _, found := records[0].Usage["reasoningContent"]; found {
		t.Fatalf("update return shared usage authority with manager: %#v", records[0].Usage)
	}
	if len(records[0].ToolScope) != 0 {
		t.Fatalf("update return shared slice authority with manager: %#v", records[0].ToolScope)
	}
	after, err := os.ReadFile(path)
	if err != nil || bytes.Contains(after, []byte("PRIVATE_")) || bytes.Contains(after, []byte("caller-mutated")) {
		t.Fatalf("caller-mutated return reached durable bytes: err=%v before=%s after=%s", err, before, after)
	}
}

func TestFinalJobWriteBackstopRejectsPoisonedClosedMetadata(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: "running", Usage: validJobShellUsageFixtureV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.jobs[0].Usage["unknownPrivateField"] = "PRIVATE_USAGE_SENTINEL"
	manager.mu.Unlock()
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: "completed"}); !errors.Is(err, domainjob.ErrPersistedUsageInvalid) {
		t.Fatalf("poisoned in-memory usage was repaired instead of rejected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected poisoned metadata changed durable bytes: err=%v before=%s after=%s", err, before, after)
	}
}

func TestSemanticStartupProjectsLegacyJobMetadataOnce(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: "running", ModelExecution: validJobModelExecutionFixtureV1(), Usage: validJobShellUsageFixtureV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{}
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	persisted["modelExecution"].(map[string]any)["rawProviderResponse"] = "PRIVATE_PROVIDER_BODY_SENTINEL"
	persisted["usage"].(map[string]any)["reasoning"] = "PRIVATE_REASONING_SENTINEL"
	poisoned, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, poisoned, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); !errors.Is(err, domainjob.ErrPersistedModelExecutionInvalid) {
		t.Fatalf("live restart accepted an unprojected legacy job: %v", err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatalf("semantic startup did not project legacy job metadata: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(first), "PRIVATE_") || strings.Contains(string(first), "rawProviderResponse") ||
		strings.Contains(string(first), "reasoning") {
		t.Fatalf("semantic startup retained legacy poison: %s", first)
	}
	if _, err := NewManager(root); err != nil {
		t.Fatalf("projected job did not pass live restart: %v", err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatalf("second semantic startup failed: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("semantic startup projection was not byte-stable: err=%v first=%s second=%s", err, first, second)
	}
}

func TestManagerConstructorDoesNotCreateStorage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "child-runs")
	manager, err := NewManager(root)
	if err != nil || manager == nil {
		t.Fatalf("new manager: manager=%#v err=%v", manager, err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("job manager constructor mutated storage: %v", err)
	}
}

func TestJobInventoryRejectsExternalArtifactPathBeforeAnyMutation(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "outside.log")
	sentinel := []byte("unchanged")
	if err := os.WriteFile(external, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"id":"job-1","parentGoalId":"goal","parentThreadId":"thread","kind":"subagent","status":"running","artifactPath":%q,"output":"attacker-controlled"}`, filepath.ToSlash(external))
	if err := os.WriteFile(filepath.Join(root, "job-1.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("external persisted artifact path was accepted")
	}
	after, err := os.ReadFile(external)
	if err != nil || !bytes.Equal(after, sentinel) {
		t.Fatalf("rejected child-run inventory mutated external target: body=%q err=%v", after, err)
	}
}

func TestSemanticStartupRetiresRelocatedLegacyArtifactWithoutExternalAccess(t *testing.T) {
	for _, stageArtifact := range []bool{false, true} {
		t.Run(fmt.Sprintf("stage-artifact-%t", stageArtifact), func(t *testing.T) {
			root := t.TempDir()
			external := filepath.Join(t.TempDir(), "job-1.log")
			sentinel := []byte("external bytes must remain unchanged")
			if err := os.WriteFile(external, sentinel, 0o600); err != nil {
				t.Fatal(err)
			}
			body := fmt.Sprintf(`{"id":"job-1","parentGoalId":"goal","parentThreadId":"thread","kind":"background-shell","status":"completed","artifactPath":%q,"output":"legacy output"}`, filepath.ToSlash(external))
			path := filepath.Join(root, "job-1.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			stagePath := expectedJobArtifactPath(root, "job-1")
			if stageArtifact {
				if err := os.WriteFile(stagePath, []byte("retired stage artifact"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			manager, err := NewManagerForSemanticStartup(root, root)
			if err != nil {
				t.Fatalf("semantic startup rejected a relocated legacy pointer: %v", err)
			}
			records := manager.AllRecords()
			if len(records) != 1 || records[0].ArtifactPath != "" {
				t.Fatalf("semantic startup retained a retired artifact pointer: %#v", records)
			}
			after, err := os.ReadFile(external)
			if err != nil || !bytes.Equal(after, sentinel) {
				t.Fatalf("semantic startup accessed the external legacy target: body=%q err=%v", after, err)
			}
			if _, err := os.Lstat(stagePath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("authenticated stage artifact was not retired: %v", err)
			}
			if _, err := NewManager(root); err != nil {
				t.Fatalf("projected record did not pass live restart: %v", err)
			}
		})
	}
}

func TestJobInventoryRejectsFilenameIdentityMismatchAndDuplicateJSONKeys(t *testing.T) {
	for name, body := range map[string]string{
		"filename":  `{"id":"job-2","status":"completed"}`,
		"duplicate": `{"id":"job-1","id":"job-2","status":"completed"}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "job-1.json"), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewManager(root); err == nil {
				t.Fatal("invalid persisted child-run record was accepted")
			}
		})
	}
}

func TestManagerPersistsProjectedChildRunRecordsAndReloads(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:          "goal_1",
		ParentGoalObjective:   "Ship durable child runs",
		ParentThreadID:        "thr_1",
		ParentTurnID:          "turn_1",
		Kind:                  "child-run",
		Model:                 "deepseek-chat",
		ProfileSource:         "parent-default",
		DefaultModelInherited: true,
		Output:                "child result",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	if record.ArtifactPath != "" || !record.DefaultModelInherited || record.Model != "deepseek-chat" {
		t.Fatalf("record should persist projected state without a duplicate output artifact: %#v", record)
	}

	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	records := reloaded.Records()
	if len(records) != 1 {
		t.Fatalf("expected one reloaded record, got %#v", records)
	}
	reloadedRecord, _ := records[0].(Record)
	if reloadedRecord.ID != record.ID ||
		reloadedRecord.ParentGoalID != "goal_1" ||
		reloadedRecord.ParentThreadID != "thr_1" ||
		reloadedRecord.ArtifactPath != "" {
		t.Fatalf("reloaded record mismatch: %#v", reloadedRecord)
	}
}

func TestManagerNeverPersistsReasoningOrRestrictedPIIInOrdinaryJobOutput(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_1", ParentThreadID: "thread_1", Kind: "background-shell", Status: "running",
		Output: "public<think>PRIVATE_REASONING</think> 账号 6222020202020202020",
		Error:  `{"reasoning_content":"PRIVATE_ERROR"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record.Output, "PRIVATE_REASONING") || strings.Contains(record.Output, "6222020202020202020") ||
		!strings.Contains(record.Output, "public") || !strings.Contains(record.Output, "[ACCOUNT]") || record.Error != "" {
		t.Fatalf("unsafe ordinary job record was retained: %#v", record)
	}
	updated, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: "completed", Output: "public<thi", Error: "failure<think>PRIVATE_ERROR",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Output != "" || updated.Error != "" {
		t.Fatalf("malformed reasoning output was not withheld: %#v", updated)
	}
	for _, path := range []string{filepath.Join(root, record.ID+".json")} {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(body)
		if strings.Contains(text, "PRIVATE_") || strings.Contains(text, "6222020202020202020") ||
			strings.Contains(strings.ToLower(text), "reasoning_content") || strings.Contains(strings.ToLower(text), "<think") {
			t.Fatalf("ordinary job durable bytes leaked private output at %s: %q", path, text)
		}
	}
	if _, statErr := os.Lstat(expectedJobArtifactPath(root, record.ID)); !os.IsNotExist(statErr) {
		t.Fatalf("ordinary job output artifact must not be created: %v", statErr)
	}
}

func TestManagerRejectsUnknownStatusOnStartUpdateAndReload(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	privateStatus := "running<think>PRIVATE_STATUS</think>"
	if _, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal", ParentThreadID: "thread", Kind: "background-shell", Status: privateStatus,
	}); err == nil {
		t.Fatal("unknown start status was accepted")
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal", ParentThreadID: "thread", Kind: "background-shell", Status: string(domainjob.StatusRunning),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: privateStatus}); err == nil {
		t.Fatal("unknown update status was accepted")
	}
	for name, request := range map[string]UpdateRequest{
		"auto continue":       {AutoContinueStatus: privateStatus},
		"completion delivery": {CompletionDeliveryStatus: privateStatus},
		"recovery":            {RecoveryStatus: privateStatus},
	} {
		if _, err := manager.UpdateChildRun(record.ID, request); err == nil {
			t.Fatalf("unknown %s status was accepted", name)
		}
	}
	loaded := manager.AllRecords()
	if len(loaded) != 1 || loaded[0].Status != string(domainjob.StatusRunning) {
		t.Fatalf("rejected update changed durable state: %#v", loaded)
	}

	path := filepath.Join(root, record.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(body), `"status": "running"`, `"status": "running PRIVATE_STATUS"`, 1)
	if text == string(body) {
		t.Fatal("status fixture replacement did not apply")
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("unknown reloaded status was accepted")
	}
}

func TestManagerOperationalReplayIsNoOpAndSettledIdentityIsImmutable(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal", ParentThreadID: "thread", Kind: "background-shell",
		Status: string(domainjob.StatusCompleted), Background: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		CompletionDeliveryID: "delivery-1", CompletionDeliveryItemID: "item-1",
		CompletionDeliveryStatus: "pending", CompletionDeliveryReason: "runtime_startup_recovery",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		CompletionDeliveryID: "delivery-1", CompletionDeliveryItemID: "item-1",
		CompletionDeliveryStatus: "pending", CompletionDeliveryReason: "<think>PRIVATE_REPLAY</think>",
	})
	if err != nil {
		t.Fatal(err)
	}
	afterReplay, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.CompletionDeliveryAttempts != 1 || replayed.UpdatedAt != pending.UpdatedAt || !bytes.Equal(before, afterReplay) {
		t.Fatalf("exact operational replay mutated durable state: before=%#v after=%#v", pending, replayed)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		CompletionDeliveryID: "delivery-replaced", CompletionDeliveryStatus: "pending",
	}); err == nil {
		t.Fatal("completion delivery identity replacement was accepted")
	}
	afterIdentityReject, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, afterIdentityReject) {
		t.Fatalf("rejected identity replacement mutated durable bytes: err=%v", err)
	}
	delivered, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		CompletionDeliveryID: "delivery-1", CompletionDeliveryItemID: "item-1", CompletionDeliveryStatus: "delivered",
	})
	if err != nil {
		t.Fatal(err)
	}
	settledBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		CompletionDeliveryID: "delivery-1", CompletionDeliveryItemID: "item-1", CompletionDeliveryStatus: "pending",
	}); err == nil {
		t.Fatal("settled delivery was reopened")
	}
	afterReopenReject, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(settledBytes, afterReopenReject) {
		t.Fatalf("rejected settled-state transition mutated durable bytes: err=%v", err)
	}
	if loaded, err := manager.LoadChildRun(record.ID); err != nil || loaded.CompletionDeliveryStatus != "delivered" || loaded.CompletionDeliveryAttempts != delivered.CompletionDeliveryAttempts {
		t.Fatalf("rejected transition changed in-memory state: loaded=%#v err=%v", loaded, err)
	}
}

func TestSemanticStartupMigratesLegacyChildRunReasoningBeforeActivation(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := canonicalJobRootWithoutCreate(root)
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := expectedJobArtifactPath(canonicalRoot, "job-1")
	legacy := fmt.Sprintf(`{
  "id": "job-1",
  "parentGoalId": "goal",
  "parentThreadId": "thread",
  "kind": "background-shell",
  "name": "worker<think>PRIVATE_NAME</think>",
  "label": "ordinary diagnostic",
  "prompt": "{\"reasoning_content\":\"PRIVATE_PROMPT\"}",
  "profileDescription": "reviewer<think>PRIVATE_PROFILE</think>",
  "diffSummary": "2 files changed<think>PRIVATE_DIFF</think>",
  "status": "running",
  "output": "ordinary output<think>PRIVATE_OUTPUT</think> account 6222020202020202020",
  "error": "ordinary error<think>PRIVATE_ERROR</think>",
  "autoContinueError": "PRIVATE_AUTO",
  "completionDeliveryError": "PRIVATE_DELIVERY",
	"artifactPath": %q,
  "toolInvocations": 0
}`, artifactPath)
	path := filepath.Join(root, "job-1.json")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.FromSlash(artifactPath), []byte("PRIVATE_CHILD_LOG_REASONING"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("live manager accepted an unmigrated legacy child-run record")
	}
	afterRejected, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(afterRejected, before) {
		t.Fatalf("live rejection mutated legacy bytes: err=%v", err)
	}

	manager, err := NewManagerForSemanticStartup(root, root)
	if err != nil {
		t.Fatal(err)
	}
	records := manager.AllRecords()
	if len(records) != 1 {
		t.Fatalf("migrated records=%#v", records)
	}
	record := records[0]
	if record.Name != "bash" || record.Label != "background shell" || record.Prompt != "" ||
		record.ProfileDescription != "reviewer" || record.DiffSummary != "2 files changed" ||
		record.Error != "" || record.FailureCode != domainjob.FailureChildUnknown ||
		record.ArtifactPath != "" ||
		record.AutoContinueError != "" || record.CompletionDeliveryError != "" ||
		!strings.Contains(record.Output, "ordinary output") || !strings.Contains(record.Output, "[ACCOUNT]") ||
		strings.Contains(record.Output, "6222020202020202020") || record.ChildSeq != 1 {
		t.Fatalf("migrated record=%#v", record)
	}
	durable, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(durable))
	for _, forbidden := range []string{"private_", "reasoning_content", "<think", "6222020202020202020"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("semantic migration retained %q: %s", forbidden, durable)
		}
	}
	if _, err := os.Lstat(filepath.FromSlash(artifactPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("semantic migration retained legacy reasoning artifact: %v", err)
	}
	if _, err := NewManager(root); err != nil {
		t.Fatalf("migrated live record did not reload: %v", err)
	}
}

func TestLiveSteerRequiresExactHostProjectionWithoutDurableMutation(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-steer-projection", ParentThreadID: "thread-steer-projection", ParentTurnID: "turn-parent",
		Kind: "background-shell", Status: string(domainjob.StatusRunning), Background: true, ChildThreadID: "thread-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{
		"inspect account 6222020202020202020",
		"continue<think>PRIVATE_STEER_REASONING</think>",
	} {
		_, _, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), domainjob.SteerMessage{
			ID: "steer-private", ParentThreadID: record.ParentThreadID, ChildRunID: record.ID, JobID: record.ID,
			Text: text, Status: "queued", CreatedAt: "2026-07-19T00:00:00Z",
		})
		if err == nil {
			t.Fatalf("unprojected steer was accepted: %q", text)
		}
		after, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(before, after) {
			t.Fatalf("rejected steer mutated durable bytes: err=%v", readErr)
		}
	}
}

func TestSemanticStartupTombstonesLegacyPrivateSteerWithoutReflection(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-legacy-steer", ParentThreadID: "thread-legacy-steer", ParentTurnID: "turn-parent",
		Kind: "background-shell", Status: string(domainjob.StatusRunning), Background: true, ChildThreadID: "thread-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, _, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), domainjob.SteerMessage{
		ID: "steer-legacy-private", ParentThreadID: record.ParentThreadID, ChildRunID: record.ID, JobID: record.ID,
		Text: "safe guidance", Status: "queued", CreatedAt: "2026-07-19T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	const sentinel = "PRIVATE_LEGACY_STEER_6222020202020202020"
	queued.Steers[0].Text = sentinel
	queued.Steers[0].ContentDigest = domainjob.SteerMessageContentDigestV1(queued.Steers[0])
	queued.SteerState = childRunState(queued)
	path := filepath.Join(root, record.ID+".json")
	body, err := json.MarshalIndent(queued, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), body...)
	if _, err := NewManager(root); err == nil {
		t.Fatal("live startup accepted a legacy private steer")
	}
	afterRejected, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, afterRejected) {
		t.Fatalf("live rejection mutated legacy bytes: err=%v", err)
	}
	migrated, err := NewManagerForSemanticStartup(root, root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := migrated.LoadChildRun(record.ID)
	if err != nil || len(loaded.Steers) != 1 {
		t.Fatalf("migrated steer missing: record=%#v err=%v", loaded, err)
	}
	steer := loaded.Steers[0]
	if steer.Status != "expired" || steer.Text != "" || steer.SourceTurnID != "" || steer.SourceToolCallID != "" ||
		steer.RejectedReason != "legacy_private_steer_removed" || loaded.SteerState.PendingSteers != 0 || loaded.SteerState.CanAcceptSteer != true {
		t.Fatalf("legacy private steer was not reduced to a closed tombstone: record=%#v", loaded)
	}
	durable, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(durable), sentinel) {
		t.Fatalf("semantic migration retained private steer bytes: err=%v body=%s", err, durable)
	}
	if _, err := NewManager(root); err != nil {
		t.Fatalf("migrated steer did not reload idempotently: %v", err)
	}
}

func TestSemanticStartupTombstonesUnboundLegacySteerV0WithoutAuthorityUpgrade(t *testing.T) {
	root := t.TempDir()
	const sentinel = "PRIVATE_UNBOUND_V0_STEER"
	legacy := Record{
		ID: "job-1", ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: string(domainjob.StatusCompleted),
		Steers: []domainjob.SteerMessage{{
			ID: "steer-1", ParentThreadID: "thread-1", ChildRunID: "job-1", JobID: "job-1",
			Text: sentinel, SourceTurnID: "turn-private", Status: "admitted", CreatedAt: "2026-07-01T00:00:00Z",
			AdmittedAt: "2026-07-01T00:00:01Z",
		}},
	}
	legacy.SteerState = childRunState(legacy)
	body, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "job-1.json")
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("live startup accepted an unauthoritative legacy-v0 steer")
	}
	afterRejected, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, afterRejected) {
		t.Fatalf("live rejection mutated legacy-v0 bytes: err=%v", err)
	}

	migrated, err := NewManagerForSemanticStartup(root, root)
	if err != nil {
		t.Fatalf("semantic migration rejected the exact unbound legacy-v0 shape: %v", err)
	}
	loaded, err := migrated.LoadChildRun("job-1")
	if err != nil || len(loaded.Steers) != 1 {
		t.Fatalf("migrated legacy-v0 steer missing: record=%#v err=%v", loaded, err)
	}
	steer := loaded.Steers[0]
	if steer.Status != "expired" || steer.Text != "" || steer.SourceTurnID != "" || steer.AdmittedAt != "" ||
		steer.ProjectionVersion != domainjob.SteerMessageProjectionVersionV1 ||
		!domainsecurity.IsSHA256Hex(steer.ContentDigest) || !domainsecurity.IsSHA256Hex(steer.QueueAuthorityDigest) ||
		steer.ContextDigest != "" || steer.AuthorityDigest != "" || steer.RejectedReason != "legacy_private_steer_removed" ||
		loaded.SteerState.PendingSteers != 0 || loaded.SteerState.AdmittedSteers != 0 {
		t.Fatalf("legacy-v0 steer gained authority during migration: record=%#v", loaded)
	}
	durable, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(durable), sentinel) || strings.Contains(string(durable), "turn-private") {
		t.Fatalf("semantic migration retained legacy-v0 private bytes: err=%v body=%s", err, durable)
	}
	if _, err := NewManager(root); err != nil {
		t.Fatalf("migrated legacy-v0 tombstone did not reload: %v", err)
	}
}

func TestSemanticStartupRemovesConsumedLegacyResumeToken(t *testing.T) {
	root := t.TempDir()
	legacy := Record{
		ID: "job-1", ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: string(domainjob.StatusCompleted),
		PauseRequests: []domainjob.PauseRequest{{
			ID: "pause-1", ParentThreadID: "thread-1", ChildRunID: "job-1", JobID: "job-1", Status: "resumed",
			RequestedAt: "2026-07-01T00:00:00Z", PausedAt: "2026-07-01T00:00:01Z", ResumedAt: "2026-07-01T00:00:02Z",
			ResumeToken: "PRIVATE_CONSUMED_RESUME_TOKEN", ResumeTokenIssuedAt: "2026-07-01T00:00:01Z",
			ResumeTokenExpiresAt: "2026-07-02T00:00:01Z",
		}},
	}
	legacy.PauseState = childRunPauseState(legacy)
	body, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "job-1.json")
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("live startup accepted a consumed legacy resume capability")
	}
	afterRejected, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, afterRejected) {
		t.Fatalf("live rejection mutated consumed legacy resume bytes: err=%v", err)
	}

	migrated, err := NewManagerForSemanticStartup(root, root)
	if err != nil {
		t.Fatalf("semantic migration rejected a complete settled pause request: %v", err)
	}
	loaded, err := migrated.LoadChildRun("job-1")
	if err != nil || len(loaded.PauseRequests) != 1 {
		t.Fatalf("migrated pause request missing: record=%#v err=%v", loaded, err)
	}
	request := loaded.PauseRequests[0]
	if request.ResumeToken != "" || request.ResumeTokenIssuedAt != "2026-07-01T00:00:01Z" ||
		request.ResumeTokenExpiresAt != "2026-07-02T00:00:01Z" || request.ResumedAt != "2026-07-01T00:00:02Z" {
		t.Fatalf("semantic migration changed pause audit authority or retained the token: %#v", request)
	}
	durable, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(durable), "PRIVATE_CONSUMED_RESUME_TOKEN") {
		t.Fatalf("semantic migration retained the consumed resume capability: err=%v body=%s", err, durable)
	}
	if _, err := NewManager(root); err != nil {
		t.Fatalf("migrated pause request did not reload: %v", err)
	}
}

func TestSemanticStartupRejectsUnknownOperationalStatusWithoutMutation(t *testing.T) {
	root := t.TempDir()
	legacy := `{
  "id": "job-1",
  "parentGoalId": "goal",
  "parentThreadId": "thread",
  "kind": "background-shell",
  "status": "running",
  "recoveryStatus": "PRIVATE_RECOVERY_STATUS",
  "toolInvocations": 0
}`
	path := filepath.Join(root, "job-1.json")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err == nil {
		t.Fatal("semantic startup accepted an unknown durable recovery status")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("failed semantic validation mutated unknown status evidence: err=%v", err)
	}
}

func TestUnboundReloadRejectsUnknownFailureCodeWithoutMutation(t *testing.T) {
	root := t.TempDir()
	body := []byte(`{
  "id": "job-1",
  "parentGoalId": "goal",
  "parentThreadId": "thread",
  "kind": "background-shell",
  "status": "failed",
  "failureCode": "PRIVATE_FAILURE_CODE",
  "toolInvocations": 0
}`)
	path := filepath.Join(root, "job-1.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("ordinary reload accepted an unknown failure code")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, body) {
		t.Fatalf("rejected failure code mutated durable evidence: err=%v", err)
	}
}

func TestSemanticStartupMigratesUnknownFailureCodeToClosedStatusCode(t *testing.T) {
	root := t.TempDir()
	const sentinel = "PRIVATE_FAILURE_CODE_6E1B"
	body := []byte(`{
  "id": "job-1",
  "parentGoalId": "goal",
  "parentThreadId": "thread",
  "kind": "background-shell",
  "status": "timeout",
  "failureCode": "` + sentinel + `",
  "toolInvocations": 0
}`)
	path := filepath.Join(root, "job-1.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManagerForSemanticStartup(root, root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.LoadChildRun("job-1")
	if err != nil || record.FailureCode != domainjob.FailureChildTimeout || record.Error != "" {
		t.Fatalf("unknown legacy failure was not migrated to the status-derived closed code: record=%#v err=%v", record, err)
	}
	durable, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(durable), sentinel) {
		t.Fatalf("semantic failure-code migration retained unknown bytes: err=%v body=%s", err, durable)
	}
	if _, err := NewManager(root); err != nil {
		t.Fatalf("migrated failure code did not reload: %v", err)
	}
}

func TestManagerProjectsProviderOriginatedMetadataBeforeFirstWrite(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal", ParentThreadID: "thread", Kind: "background-shell", Status: string(domainjob.StatusRunning),
		Name: "worker<think>PRIVATE_NAME</think>", Label: "review 6222020202020202020",
		Prompt:             `{"thinking_content":"PRIVATE_PROMPT"}`,
		ProfileDescription: "profile<think>PRIVATE_PROFILE</think>",
		DiffSummary:        "1 file<think>PRIVATE_DIFF</think>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Name != "bash" || record.Label != "background shell" || record.Prompt != "" || record.ProfileDescription != "profile" ||
		record.DiffSummary != "1 file" {
		t.Fatalf("projected metadata=%#v", record)
	}
	body, err := os.ReadFile(filepath.Join(root, record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(body)), "private_") || strings.Contains(string(body), "6222020202020202020") {
		t.Fatalf("first durable write leaked provider metadata: %s", body)
	}
}

func TestBackgroundShellCredentialNeverEntersJobJSON(t *testing.T) {
	const secret = "sk-jobjsonsecret123456"
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal", ParentThreadID: "thread", Kind: "background-shell", Status: "running",
		Name: "curl", Label: "Authorization: Bearer " + secret,
		Output: "stdout=" + secret, Error: "postgres://analyst:" + secret + "@db.example/case",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secret) || strings.Contains(string(body), "Authorization: Bearer") ||
		strings.Contains(string(body), "postgres://analyst:"+secret) || record.Label != "background shell" {
		t.Fatalf("background credential entered durable job state: record=%#v body=%s", record, body)
	}
}

func TestSemanticStartupScrubsLegacyBackgroundShellCredentialsIdempotently(t *testing.T) {
	const secret = "sk-legacyjobsecret123456"
	root := t.TempDir()
	path := filepath.Join(root, "job-1.json")
	legacy := []byte(`{"id":"job-1","parentGoalId":"goal","parentThreadId":"thread","kind":"background-shell","name":"curl","label":"Authorization: Bearer ` + secret + `","status":"completed","output":"stdout=` + secret + `","error":"postgres://analyst:` + secret + `@db.example/case","toolInvocations":0}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("live manager accepted legacy background credential bytes")
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(first), secret) || strings.Contains(string(first), "Authorization: Bearer") {
		t.Fatalf("semantic startup retained legacy credential: err=%v body=%s", err, first)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("credential migration was not byte-stable: err=%v first=%s second=%s", err, first, second)
	}
}

func TestManagerPreservesJobOutputBytes(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	initialOutput := " first line\nsecond line\n" + privateSentinel + "\n"
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_output",
		ParentThreadID: "thr_output",
		Kind:           "background-shell",
		Status:         "running",
		Output:         initialOutput,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	if record.Output != initialOutput {
		t.Fatalf("initial output should preserve bytes: got %q want %q", record.Output, initialOutput)
	}
	if record.ArtifactPath != "" {
		t.Fatalf("ordinary output must not create a duplicate artifact: %#v", record)
	}

	updatedOutput := initialOutput + " third line with spaces  \n"
	updated, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Output: updatedOutput,
	})
	if err != nil {
		t.Fatalf("update child run: %v", err)
	}
	if updated.Output != updatedOutput {
		t.Fatalf("updated output should preserve bytes: got %q want %q", updated.Output, updatedOutput)
	}
	if _, statErr := os.Lstat(expectedJobArtifactPath(root, record.ID)); !os.IsNotExist(statErr) {
		t.Fatalf("updated output must not create a duplicate artifact: %v", statErr)
	}
	durable, err := os.ReadFile(filepath.Join(root, record.ID+".json"))
	if err != nil || !bytes.Contains(durable, []byte(privateSentinel)) {
		t.Fatalf("private task output was not retained inside the durable job authority: err=%v body=%s", err, durable)
	}
	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	loaded, err := reloaded.LoadChildRun(record.ID)
	if err != nil || loaded.Output != updatedOutput || !strings.Contains(loaded.Output, privateSentinel) {
		t.Fatalf("durable task output did not reload exactly: record=%#v err=%v", loaded, err)
	}
}

func TestManagerPersistsWorktreeIsolationMetadata(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_iso",
		ParentThreadID: "thr_iso",
		Kind:           "subagent",
		Status:         "queued",
		Background:     true,
		ToolPolicy:     "inherit",
		IsolationMode:  "worktree",
		MergeStatus:    "not_requested",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	isolation := domainjob.WorktreeIsolation{
		IsolationMode:  "worktree",
		WorktreePath:   "/tmp/analytix/subagent-worktrees/thr_iso/job-1",
		WorktreeBranch: "codex/subagent/thr_iso/job-1",
		BaseCommit:     "base",
		CurrentCommit:  "current",
		ChangedFiles:   []domainjob.ChangedFile{{Path: "src/app.go", Status: "M"}, {Path: "notes.md", Status: "??"}},
		DiffSummary:    "2 files changed",
		MergeStatus:    "not_requested",
	}
	updated, err := manager.UpdateChildRun(record.ID, UpdateRequest{Workspace: isolation.WorktreePath, Isolation: &isolation})
	if err != nil {
		t.Fatalf("update child run: %v", err)
	}
	decision := domainjob.MergeDecision{
		ID:             "decision-1",
		ParentThreadID: "thr_iso",
		ChildRunID:     record.ID,
		JobID:          record.ID,
		Decision:       "reject",
		CreatedAt:      "2026-07-07T00:00:00Z",
		Reason:         "not needed",
	}
	receipt := domainjob.CleanupReceipt{
		ID:               "cleanup-1",
		AcceptDecisionID: "accept-1",
		WorktreePath:     isolation.WorktreePath,
		Branch:           isolation.WorktreeBranch,
		Removed:          true,
		CreatedAt:        "2026-07-07T00:01:00Z",
	}
	accept := domainjob.AcceptDecision{
		ID:                 "accept-1",
		ParentThreadID:     "thr_iso",
		ChildRunID:         record.ID,
		JobID:              record.ID,
		MergeRequestID:     "merge-1",
		ApprovalID:         "approval-1",
		BaseCommit:         "base",
		ParentHeadBefore:   "base",
		ParentHeadAfter:    "base",
		ChangedFiles:       isolation.ChangedFiles,
		AppliedPatchDigest: "sha256:abc",
		CreatedAt:          "2026-07-07T00:02:00Z",
	}
	conflict := domainjob.ConflictReport{
		ID:                     "conflict-1",
		ParentThreadID:         "thr_iso",
		ChildRunID:             record.ID,
		JobID:                  record.ID,
		ParentHeadAtConflict:   "base",
		ChildHeadAtConflict:    "current",
		ConflictFiles:          []domainjob.ChangedFile{{Path: "src/app.go", Status: "UU"}},
		ConflictSummary:        "1 conflict",
		CreatedAt:              "2026-07-07T00:03:00Z",
		TouchedParentWorkspace: false,
	}
	repairReview := domainjob.RepairPatchReview{
		ID:                 "repair-review-1",
		ConflictReportID:   conflict.ID,
		ParentThreadID:     "thr_iso",
		ChildRunID:         record.ID,
		JobID:              record.ID,
		RepairPatch:        "patch",
		RepairPatchDigest:  "sha256:repair",
		ExpectedParentHead: "base",
		ChangedFiles:       []domainjob.ChangedFile{{Path: "src/app.go", Status: "M"}},
		DryRunStatus:       "clean",
		CreatedAt:          "2026-07-07T00:04:00Z",
	}
	repairDecision := domainjob.RepairDecision{
		ID:                 "repair-decision-1",
		RepairReviewID:     repairReview.ID,
		ApprovalID:         "approval-repair-1",
		ParentHeadBefore:   "base",
		ParentHeadAfter:    "base",
		AppliedPatchDigest: "sha256:repair",
		CreatedAt:          "2026-07-07T00:05:00Z",
	}
	childTodos := domainjob.ChildTodoList{
		ID:             "child-todos-1",
		ParentThreadID: "thr_iso",
		ChildThreadID:  "child-thread",
		ChildRunID:     record.ID,
		JobID:          record.ID,
		Scope:          "child",
		Items:          []domainjob.ChildTodoItem{{ID: "child-todo-1", Content: "Review output", Status: "completed", EvidenceIDs: []string{"ev-1"}}},
		CreatedAt:      "2026-07-07T00:06:00Z",
		UpdatedAt:      "2026-07-07T00:06:00Z",
	}
	projection := domainjob.ChildTodoProjection{
		ID:              "projection-1",
		ParentThreadID:  "thr_iso",
		ChildThreadID:   "child-thread",
		ChildRunID:      record.ID,
		JobID:           record.ID,
		ChildTodoListID: childTodos.ID,
		Status:          "proposed",
		ProjectedItems:  []domainjob.ChildTodoProjectionItem{{ID: "child-todo-1", Content: "Review output", Status: "completed", EvidenceIDs: []string{"ev-1"}}},
		EvidenceIDs:     []string{"ev-1"},
		CreatedAt:       "2026-07-07T00:06:01Z",
	}
	projectionDecision := domainjob.ProjectionDecision{
		ID:                           "projection-decision-1",
		ParentThreadID:               "thr_iso",
		ChildThreadID:                "child-thread",
		ChildRunID:                   record.ID,
		JobID:                        record.ID,
		ProjectionID:                 projection.ID,
		Decision:                     "accepted",
		ApprovalID:                   "approval-child-todo",
		AcceptedItems:                []domainjob.AcceptedProjectionItem{{ChildTodoID: "child-todo-1", ParentTodoID: "parent-todo-1", Action: "complete_existing", PreviousParentStatus: "pending", NextParentStatus: "completed", EvidenceIDs: []string{"ev-1"}}},
		SkippedItems:                 []domainjob.SkippedProjectionItem{{ChildTodoID: "child-todo-2", Reason: "missing_parent_todo_ref"}},
		ParentTodosBeforeDigest:      "sha256:before",
		ParentTodosAfterDigest:       "sha256:after",
		ExpectedParentTodosUpdatedAt: "2026-07-07T00:06:00Z",
		CreatedAt:                    "2026-07-07T00:06:02Z",
	}
	updated, err = manager.UpdateChildRun(record.ID, UpdateRequest{
		MergeDecision:       &decision,
		CleanupReceipt:      &receipt,
		AcceptDecision:      &accept,
		ConflictReport:      &conflict,
		RepairPatchReview:   &repairReview,
		RepairDecision:      &repairDecision,
		ChildTodoList:       &childTodos,
		ChildTodoProjection: &projection,
		ProjectionDecision:  &projectionDecision,
	})
	if err != nil {
		t.Fatalf("update child run decision receipt: %v", err)
	}
	if updated.Workspace != isolation.WorktreePath || updated.WorktreeBranch != isolation.WorktreeBranch || len(updated.ChangedFiles) != 2 {
		t.Fatalf("isolation metadata not updated: %#v", updated)
	}
	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	loaded, err := reloaded.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load child run: %v", err)
	}
	if loaded.IsolationMode != "worktree" || loaded.MergeStatus != "not_requested" || loaded.ChangedFiles[1].Status != "??" {
		t.Fatalf("isolation metadata not persisted: %#v", loaded)
	}
	if len(loaded.MergeDecisions) != 1 || loaded.MergeDecisions[0].Decision != "reject" ||
		len(loaded.CleanupReceipts) != 1 || !loaded.CleanupReceipts[0].Removed || loaded.CleanupReceipts[0].AcceptDecisionID != "accept-1" ||
		len(loaded.AcceptDecisions) != 1 || loaded.AcceptDecisions[0].ApprovalID != "approval-1" ||
		len(loaded.ConflictReports) != 1 || loaded.ConflictReports[0].TouchedParentWorkspace ||
		len(loaded.RepairPatchReviews) != 1 || loaded.RepairPatchReviews[0].RepairPatch != "" || loaded.RepairPatchReviews[0].RepairPatchDigest != "sha256:repair" ||
		len(loaded.RepairDecisions) != 1 || loaded.RepairDecisions[0].ApprovalID != "approval-repair-1" ||
		len(loaded.ChildTodoLists) != 1 || loaded.ChildTodoLists[0].Items[0].EvidenceIDs[0] != "ev-1" ||
		len(loaded.ChildTodoProjections) != 1 || loaded.ChildTodoProjections[0].Status != "proposed" ||
		len(loaded.ProjectionDecisions) != 1 || loaded.ProjectionDecisions[0].AcceptedItems[0].ParentTodoID != "parent-todo-1" {
		t.Fatalf("isolation decision metadata not persisted: %#v", loaded)
	}
}

func TestNestedChildRunPrivateTextNeverPersistsOrSurvivesSemanticStartup(t *testing.T) {
	const sentinel = "PRIVATE_NESTED_REASONING"
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal-nested", ParentThreadID: "thread-nested",
		Kind: "parallel-child-run", Status: string(domainjob.StatusRunning),
	})
	if err != nil {
		t.Fatal(err)
	}
	list := domainjob.ChildTodoList{
		ID: "child-list-1", ParentThreadID: record.ParentThreadID,
		ChildThreadID: "child-thread", ChildRunID: record.ID, JobID: record.ID,
		Scope: "scope<think>" + sentinel + "</think>",
		Items: []domainjob.ChildTodoItem{{
			ID: "child-todo-1", Content: "todo<think>" + sentinel + "</think> account 6222020202020202020", Status: "pending",
		}},
		CreatedAt: "2026-07-20T00:00:00Z", UpdatedAt: "2026-07-20T00:00:00Z",
	}
	updated, err := manager.UpdateChildRun(record.ID, UpdateRequest{ChildTodoList: &list})
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.ChildTodoLists[0].Items[0].Content; got != "todo account [ACCOUNT]" {
		t.Fatalf("live nested projection mismatch: %q", got)
	}
	path := filepath.Join(root, record.ID+".json")
	durable, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(durable), sentinel) || strings.Contains(string(durable), "6222020202020202020") {
		t.Fatalf("live child-run write retained nested private bytes: %s", durable)
	}

	var tampered domainjob.Record
	if err := json.Unmarshal(durable, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered.ChildTodoLists[0].Items[0].Content = "legacy<think>" + sentinel + "</think>"
	tampered.RepairPatchReviews = []domainjob.RepairPatchReview{{
		ID: "repair-1", RepairPatch: "diff --git a/x b/x\n+" + sentinel,
	}}
	tamperedBytes, err := json.MarshalIndent(tampered, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(tamperedBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("live startup accepted an unprojected nested child-run record")
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatal(err)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(migrated), sentinel) || strings.Contains(string(migrated), "diff --git") {
		t.Fatalf("semantic startup retained nested private bytes: %s", migrated)
	}
	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatalf("nested semantic migration did not produce a reloadable record: %v", err)
	}
	loaded, err := reloaded.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ChildTodoLists[0].Items[0].Content != "legacy" || loaded.RepairPatchReviews[0].RepairPatch != "" {
		t.Fatalf("nested semantic migration projection mismatch: %#v", loaded)
	}
}

func TestManagerSemanticStartupProjectsRetiredTypeScriptChildRunWithoutPrivateBytes(t *testing.T) {
	root := t.TempDir()
	const legacyID = "child_mr1b6yo0_abc123"
	const privateSentinel = "PRIVATE_LEGACY_TYPESCRIPT_CHILD_REASONING"
	legacyPath := filepath.Join(root, legacyID+".json")
	legacyBody := []byte(`{
  "id":"child_mr1b6yo0_abc123",
  "parentThreadId":"thr_legacy",
  "parentTurnId":"turn_parent",
  "parentToolCallId":"call_parent",
  "childThreadId":"child_mr1b6yo0_abc123",
  "childTurnId":"turn_child",
  "label":"private label ` + privateSentinel + `",
  "prompt":"private prompt ` + privateSentinel + `",
  "workspace":"/private/workspace/` + privateSentinel + `",
  "model":"private-model-` + privateSentinel + `",
  "profile":"private-profile-` + privateSentinel + `",
  "toolPolicy":"readOnly",
  "toolScope":["read_file"],
  "status":"completed",
  "summary":"private summary ` + privateSentinel + `",
  "usage":{"promptTokens":1,"completionTokens":1,"totalTokens":2},
  "prefixReused":true,
  "inheritedHistoryItems":0,
  "toolInvocations":2,
  "transcriptItems":[{"type":"assistant_message","content":"` + privateSentinel + `"}],
  "evidenceLedgered":true,
  "durationMs":5,
  "queuedMs":1,
  "createdAt":"2026-07-01T00:00:00.000Z",
  "startedAt":"2026-07-01T00:00:00.001Z",
  "updatedAt":"2026-07-01T00:00:00.006Z"
}`)
	if err := os.WriteFile(legacyPath, legacyBody, 0o600); err != nil {
		t.Fatal(err)
	}
	before := childRunDirectoryBytesForTest(t, root)
	if _, err := NewManager(root); err == nil || !strings.Contains(err.Error(), "retired TypeScript record") {
		t.Fatalf("live manager accepted a retired TypeScript record: %v", err)
	}
	if after := childRunDirectoryBytesForTest(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("live rejection mutated retired TypeScript bytes: before=%#v after=%#v", before, after)
	}
	lineageWitness := newLegacyTypeScriptLineageWitnessForTestV1(t, root, func(lineage LegacyTypeScriptLineageV1) error {
		if lineage.ParentThreadID != "thr_legacy" || lineage.ParentTurnID != "turn_parent" ||
			lineage.ParentToolCallID != "call_parent" {
			return errors.New("unexpected legacy lineage")
		}
		return nil
	})
	if _, err := NewManagerForSemanticStartupWithWitness(root, root, nil, lineageWitness); err != nil {
		t.Fatalf("semantic startup did not project the retired TypeScript record: %v", err)
	}
	if _, err := os.Lstat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired TypeScript source remains after staged projection: %v", err)
	}
	projectedPath := filepath.Join(root, "job-1.json")
	projectedBytes, err := os.ReadFile(projectedPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(projectedBytes, []byte(privateSentinel)) || bytes.Contains(projectedBytes, []byte("transcriptItems")) ||
		bytes.Contains(projectedBytes, []byte("evidenceLedgered")) {
		t.Fatalf("semantic projection retained private legacy fields: %s", projectedBytes)
	}
	digest := sha256.Sum256(legacyBody)
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("reload projected child-run store: %v", err)
	}
	record, err := manager.LoadChildRun("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "job-1" || record.ChildSeq != 1 || record.ParentGoalID != legacyTypeScriptParentGoalIDV1 ||
		record.ParentThreadID != "thr_legacy" || record.ParentTurnID != "turn_parent" ||
		record.ParentToolCallID != "call_parent" || record.ChildThreadID != "" || record.ChildTurnID != "" ||
		record.Kind != "child-run" || record.Status != "completed" || record.Label != "legacy child run" ||
		record.SourceRef != legacyTypeScriptSourceRefV1+hex.EncodeToString(digest[:]) || record.Prompt != "" ||
		record.Output != "" || record.Error != "" || record.Usage != nil || record.ToolInvocations != 0 {
		t.Fatalf("unexpected retired TypeScript projection: %#v", record)
	}
	beforeImmutableUpdate, err := os.ReadFile(projectedPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{ChildThreadID: "thr_fabricated"}); err == nil ||
		!strings.Contains(err.Error(), "tombstone is immutable") {
		t.Fatalf("retired TypeScript tombstone accepted a live update: %v", err)
	}
	if afterImmutableUpdate, err := os.ReadFile(projectedPath); err != nil || !bytes.Equal(afterImmutableUpdate, beforeImmutableUpdate) {
		t.Fatalf("rejected tombstone update changed durable bytes: err=%v", err)
	}
	for name, mutate := range map[string]func(*Record){
		"child-thread":  func(value *Record) { value.ChildThreadID = "thr_fabricated" },
		"output":        func(value *Record) { value.Output = "fabricated output" },
		"usage":         func(value *Record) { value.Usage = map[string]any{"totalTokens": 1} },
		"background":    func(value *Record) { value.Background = true },
		"auto-continue": func(value *Record) { value.AutoContinueParent = true },
		"delivery": func(value *Record) {
			value.CompletionDeliveryID = "delivery_fabricated"
			value.CompletionDeliveryStatus = "delivered"
		},
		"tool-count": func(value *Record) { value.ToolInvocations = 1 },
	} {
		t.Run("tampered-tombstone-"+name, func(t *testing.T) {
			tamperedRoot := t.TempDir()
			tampered := cloneRecord(record)
			mutate(&tampered)
			body, err := json.MarshalIndent(&tampered, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tamperedRoot, tampered.ID+".json"), append(body, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewManager(tamperedRoot); err == nil || !strings.Contains(err.Error(), "tombstone") {
				t.Fatalf("live manager accepted a tampered retired tombstone: %v", err)
			}
		})
	}
	if _, err := manager.StartChildRun(StartRequest{
		ParentGoalID: legacyTypeScriptParentGoalIDV1, ParentThreadID: "thr_parent", Kind: "child-run",
	}); err == nil || !strings.Contains(err.Error(), "migration-only") {
		t.Fatalf("live manager created a record in the retired namespace: %v", err)
	}
	settled := childRunDirectoryBytesForTest(t, root)
	if _, err := NewManagerForSemanticStartupWithWitness(root, root, nil, nil); err != nil {
		t.Fatalf("repeat semantic startup: %v", err)
	}
	if repeated := childRunDirectoryBytesForTest(t, root); !reflect.DeepEqual(settled, repeated) {
		t.Fatalf("repeat semantic startup was not byte stable: before=%#v after=%#v", settled, repeated)
	}
}

func TestManagerSemanticStartupRejectsMissingParentWithoutMutation(t *testing.T) {
	root := t.TempDir()
	legacy := validLegacyTypeScriptChildRunFixtureV1()
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(root, "child_mr1b6yo0_abc123.json")
	if err := os.WriteFile(legacyPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	before := childRunDirectoryBytesForTest(t, root)
	if _, err := FreezeLegacyTypeScriptLineageWitnessV1(root, LegacyTypeScriptLineageVerifyFuncV1(func(LegacyTypeScriptLineageV1) error {
		return errors.New("parent thread is unavailable")
	})); err == nil {
		t.Fatal("missing parent lineage produced a migration witness")
	}
	if after := childRunDirectoryBytesForTest(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected missing-parent lineage mutated the child-run tree: before=%#v after=%#v", before, after)
	}
}

func TestManagerSemanticStartupDoesNotTreatLineageFailureAsMissingParent(t *testing.T) {
	root := t.TempDir()
	body, err := json.Marshal(validLegacyTypeScriptChildRunFixtureV1())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "child_mr1b6yo0_abc123.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	before := childRunDirectoryBytesForTest(t, root)
	if _, err := FreezeLegacyTypeScriptLineageWitnessV1(root, LegacyTypeScriptLineageVerifyFuncV1(func(LegacyTypeScriptLineageV1) error {
		return errors.New("staged thread is corrupt")
	})); err == nil || !strings.Contains(err.Error(), "could not be proven") {
		t.Fatalf("non-orphan lineage failure was accepted: %v", err)
	}
	if after := childRunDirectoryBytesForTest(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected lineage failure mutated the child-run tree: before=%#v after=%#v", before, after)
	}
}

func TestProjectLegacyTypeScriptChildRunRejectsInventoryBindingDrift(t *testing.T) {
	root := t.TempDir()
	body, err := json.Marshal(validLegacyTypeScriptChildRunFixtureV1())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "child_mr1b6yo0_abc123.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil || len(inventory.Entries) != 1 {
		t.Fatalf("inventory baseline: entries=%d err=%v", len(inventory.Entries), err)
	}
	entry := inventory.Entries[0]
	verifier := LegacyTypeScriptLineageVerifyFuncV1(func(LegacyTypeScriptLineageV1) error { return nil })
	fixtures := map[string]struct {
		raw      []byte
		entry    ChildRunInventoryEntryV1
		expected string
	}{
		"invalid-name-before-id-slice": {
			raw: body, entry: func() ChildRunInventoryEntryV1 {
				changed := entry
				changed.Name = "child_bad.json"
				return changed
			}(), expected: "migration binding is invalid",
		},
		"raw-digest-mismatch": {
			raw: append(append([]byte(nil), body...), ' '), entry: entry, expected: "source digest is invalid",
		},
		"group-writable": {
			raw: body, entry: func() ChildRunInventoryEntryV1 {
				changed := entry
				changed.Mode |= uint32(0o020)
				return changed
			}(), expected: "source permissions are unsafe",
		},
		"owner-unreadable": {
			raw: body, entry: func() ChildRunInventoryEntryV1 {
				changed := entry
				changed.Mode &^= uint32(0o400)
				return changed
			}(), expected: "source permissions are unsafe",
		},
		"setuid": {
			raw: body, entry: func() ChildRunInventoryEntryV1 {
				changed := entry
				changed.Mode |= uint32(os.ModeSetuid)
				return changed
			}(), expected: "source permissions are unsafe",
		},
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, err := projectLegacyTypeScriptChildRunV1(fixture.raw, fixture.entry, "job-1", verifier); err == nil ||
				!strings.Contains(err.Error(), fixture.expected) {
				t.Fatalf("binding drift error = %v, expected class %q", err, fixture.expected)
			}
		})
	}
}

func TestManagerSemanticStartupRejectsInvalidRetiredTypeScriptRecordWithoutMutation(t *testing.T) {
	type invalidFixture struct {
		expected string
		mutate   func(map[string]any)
		raw      func([]byte) []byte
	}
	fixtures := map[string]invalidFixture{
		"identity-mismatch": {
			expected: "identity is invalid",
			mutate:   func(record map[string]any) { record["id"] = "child_mr1b6yo0_abc124" },
		},
		"child-thread-mismatch": {
			expected: "producer identity is invalid",
			mutate:   func(record map[string]any) { record["childThreadId"] = "child_mr1b6yo0_abc124" },
		},
		"non-terminal": {
			expected: "is not terminal",
			mutate:   func(record map[string]any) { record["status"] = "running" },
		},
		"unknown-field": {
			expected: "shape is invalid",
			mutate:   func(record map[string]any) { record["authority"] = true },
		},
		"wrong-type": {
			expected: "shape is invalid",
			mutate:   func(record map[string]any) { record["toolInvocations"] = "1" },
		},
		"wrong-container": {
			expected: "private container is invalid",
			mutate:   func(record map[string]any) { record["usage"] = []any{} },
		},
		"missing-required": {
			expected: "required fields are missing",
			mutate:   func(record map[string]any) { record["prompt"] = "" },
		},
		"noncanonical-time-offset": {
			expected: "time is invalid",
			mutate:   func(record map[string]any) { record["createdAt"] = "2026-07-01T08:00:00.000+08:00" },
		},
		"noncanonical-time-nanos": {
			expected: "time is invalid",
			mutate:   func(record map[string]any) { record["createdAt"] = "2026-07-01T00:00:00.000000Z" },
		},
		"reverse-time": {
			expected: "update time is invalid",
			mutate:   func(record map[string]any) { record["updatedAt"] = "2025-07-01T00:00:00.002Z" },
		},
		"queued-duration-mismatch": {
			expected: "start time is invalid",
			mutate:   func(record map[string]any) { record["queuedMs"] = 2 },
		},
		"duplicate-key": {
			expected: "JSON is invalid",
			raw: func(body []byte) []byte {
				return bytes.Replace(body, []byte(`"status":"completed"`), []byte(`"status":"completed","status":"failed"`), 1)
			},
		},
		"invalid-utf8": {
			expected: "JSON is invalid",
			raw: func(body []byte) []byte {
				return bytes.Replace(body, []byte(`"prompt":"p"`), []byte{'"', 'p', 'r', 'o', 'm', 'p', 't', '"', ':', '"', 0xff, '"'}, 1)
			},
		},
		"trailing-json": {
			expected: "JSON is invalid",
			raw:      func(body []byte) []byte { return append(body, []byte("\n{}")...) },
		},
		"exponent-limit": {
			expected: "JSON is invalid",
			raw: func(body []byte) []byte {
				return bytes.Replace(body, []byte(`"toolInvocations":1`), []byte(`"toolInvocations":1e10001`), 1)
			},
		},
	}
	for name, body := range fixtures {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			record := validLegacyTypeScriptChildRunFixtureV1()
			if body.mutate != nil {
				body.mutate(record)
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if body.raw != nil {
				encoded = body.raw(encoded)
			}
			path := filepath.Join(root, "child_mr1b6yo0_abc123.json")
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			before := childRunDirectoryBytesForTest(t, root)
			witness, freezeErr := FreezeLegacyTypeScriptLineageWitnessV1(root, LegacyTypeScriptLineageVerifyFuncV1(func(LegacyTypeScriptLineageV1) error { return nil }))
			migrationErr := freezeErr
			if migrationErr == nil {
				_, migrationErr = NewManagerForSemanticStartupWithWitness(root, root, nil, witness)
			}
			if migrationErr == nil ||
				(freezeErr != nil && !strings.Contains(migrationErr.Error(), "lineage observation failed")) ||
				(freezeErr == nil && !strings.Contains(migrationErr.Error(), body.expected)) {
				t.Fatalf("semantic startup invalid fixture error = %v, expected class %q", migrationErr, body.expected)
			}
			if after := childRunDirectoryBytesForTest(t, root); !reflect.DeepEqual(before, after) {
				t.Fatalf("semantic rejection mutated the tree: before=%#v after=%#v", before, after)
			}
		})
	}
}

func validLegacyTypeScriptChildRunFixtureV1() map[string]any {
	return map[string]any{
		"id": "child_mr1b6yo0_abc123", "parentThreadId": "thr_legacy", "parentTurnId": "turn_parent",
		"parentToolCallId": "call_parent", "childThreadId": "child_mr1b6yo0_abc123", "childTurnId": "turn_child",
		"prompt": "p", "status": "completed", "usage": map[string]any{}, "toolInvocations": 1,
		"durationMs": 1, "queuedMs": 1, "createdAt": "2026-07-01T00:00:00.000Z",
		"startedAt": "2026-07-01T00:00:00.001Z", "updatedAt": "2026-07-01T00:00:00.002Z",
	}
}

func newLegacyTypeScriptLineageWitnessForTestV1(
	t *testing.T,
	root string,
	verify LegacyTypeScriptLineageVerifyFuncV1,
) *FrozenLegacyTypeScriptLineageWitnessV1 {
	t.Helper()
	witness, err := FreezeLegacyTypeScriptLineageWitnessV1(root, verify)
	if err != nil {
		t.Fatalf("freeze legacy TypeScript lineage witness: %v", err)
	}
	return witness
}

func TestManagerSemanticStartupDeletesExactTemporaryRecordsAndAtomicWritesLeaveNoResidue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "job-1.json"), []byte(`{
  "id": "job-1",
  "parentGoalId": "goal_atomic",
  "parentThreadId": "thr_atomic",
  "kind": "subagent",
  "status": "completed"
}`), 0o600); err != nil {
		t.Fatalf("write committed record: %v", err)
	}
	legacyTemporaryPath := filepath.Join(root, ".job-2.json.tmp-17")
	if err := os.WriteFile(legacyTemporaryPath, []byte(`{
  "id": "job-2",
  "parentGoalId": "goal_atomic",
  "parentThreadId": "thr_atomic",
  "kind": "subagent",
  "status": "completed"
}`), 0o600); err != nil {
		t.Fatalf("write temporary record: %v", err)
	}
	if _, err := NewManager(root); err == nil || !strings.Contains(err.Error(), "requiring semantic migration") {
		t.Fatalf("live manager ignored a legacy private temporary record: %v", err)
	}
	if _, err := NewManagerForSemanticStartup(root, root); err != nil {
		t.Fatalf("semantic startup did not remove exact legacy temporary residue: %v", err)
	}
	if _, err := os.Lstat(legacyTemporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("semantic startup retained legacy temporary residue: %v", err)
	}
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager after semantic migration: %v", err)
	}
	if records := manager.RecordsByParentThread("thr_atomic"); len(records) != 1 {
		t.Fatalf("semantic migration changed committed records: %#v", records)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_atomic",
		ParentThreadID: "thr_atomic",
		Kind:           "background-shell",
		Status:         "running",
		Output:         "first output",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	updatedOutput := strings.Repeat("replacement output\n", 64)
	updated, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: "completed",
		Output: updatedOutput,
	})
	if err != nil {
		t.Fatalf("update child run: %v", err)
	}
	jsonData, err := os.ReadFile(filepath.Join(root, updated.ID+".json"))
	if err != nil {
		t.Fatalf("read record json: %v", err)
	}
	if !bytes.Contains(jsonData, []byte(`"status": "completed"`)) || !bytes.Contains(jsonData, []byte(`"output": "`)) {
		t.Fatalf("record json should contain the updated state, got %s", string(jsonData))
	}
	matches, err := filepath.Glob(filepath.Join(root, ".*.tmp-*"))
	if err != nil {
		t.Fatalf("glob temporary files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("atomic writes should not leave new temp files, got %#v", matches)
	}
}

func TestManagerRunsParallelChildrenWithLineage(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	records, err := manager.StartParallel("goal_parallel", "thr_parallel", "turn_parallel", "deepseek-chat", "high", []string{"left", "right", "center"})
	if err != nil {
		t.Fatalf("start parallel: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %#v", records)
	}
	groupID := records[0].ParallelGroupID
	if groupID == "" {
		t.Fatalf("parallel group id missing: %#v", records)
	}
	for index, record := range records {
		if record.ParentGoalID != "goal_parallel" ||
			record.ParentThreadID != "thr_parallel" ||
			record.ParentTurnID != "turn_parallel" ||
			record.Kind != "parallel-child-run" ||
			record.Model != "deepseek-chat" ||
			record.Effort != "high" ||
			!record.DefaultModelInherited ||
			record.ParallelGroupID != groupID ||
			record.ParallelIndex != index+1 {
			t.Fatalf("parallel record %d mismatch: %#v", index, record)
		}
	}
}

func TestManagerLockChildRunSerializesSourceReferences(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	release, err := manager.LockChildRun("job-source")
	if err != nil {
		t.Fatalf("first source lock: %v", err)
	}
	if _, err := manager.LockChildRun("job-source"); err == nil {
		t.Fatal("second source lock should fail while the reference is locked")
	} else if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("unexpected source lock error: %v", err)
	}
	release()
	reacquired, err := manager.LockChildRun("job-source")
	if err != nil {
		t.Fatalf("source lock should be reusable after release: %v", err)
	}
	reacquired()
}

func TestManagerTerminalChildRunIgnoresLateConflictingStatus(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_terminal",
		ParentThreadID: "thr_terminal",
		Kind:           "subagent",
		Status:         "running",
		Output:         "partial output",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	killed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: "killed",
		Error:  "stopped by user",
	})
	if err != nil {
		t.Fatalf("kill child run: %v", err)
	}
	late, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: "completed",
		Output: "late successful output",
	})
	if err != nil {
		t.Fatalf("late complete child run: %v", err)
	}
	if late.Status != "killed" || late.Output != "partial output" || late.Error != "" ||
		late.FailureCode != domainjob.FailureChildKilled || late.FinishedAt != killed.FinishedAt {
		t.Fatalf("late terminal update should not overwrite killed record: killed=%#v late=%#v", killed, late)
	}
	if !late.LateCompletionSuppressed || late.LateCompletionReason != "late_completion_suppressed" {
		t.Fatalf("late conflicting terminal update should be marked suppressed: %#v", late)
	}
	lateKilled, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: "killed",
		Error:  "provider returned context canceled after kill",
		Output: "late killed output",
	})
	if err != nil {
		t.Fatalf("late killed child run: %v", err)
	}
	if lateKilled.Status != "killed" || lateKilled.Output != "partial output" || lateKilled.Error != "" ||
		lateKilled.FailureCode != domainjob.FailureChildKilled || lateKilled.FinishedAt != killed.FinishedAt {
		t.Fatalf("late same-status killed update should not overwrite explicit stop reason: killed=%#v late=%#v", killed, lateKilled)
	}
	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	persisted, err := reloaded.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load child run: %v", err)
	}
	if persisted.Status != "killed" || persisted.Output != "partial output" || persisted.Error != "" ||
		persisted.FailureCode != domainjob.FailureChildKilled {
		t.Fatalf("persisted terminal record mismatch: %#v", persisted)
	}
	if !persisted.LateCompletionSuppressed {
		t.Fatalf("persisted terminal record should retain suppression marker: %#v", persisted)
	}
}

func TestManagerPersistsPauseRequestsAndExpiresOnTerminal(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_pause",
		ParentThreadID: "thr_pause",
		Kind:           "subagent",
		Status:         "running",
		ChildThreadID:  "child_pause",
		Background:     true,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	record, pauseRequest, err := manager.AddPauseRequest(record.ID, domainjob.PauseRequest{
		ID:             "pause_1",
		ParentThreadID: "thr_pause",
		ChildRunID:     record.ID,
		JobID:          record.ID,
		Status:         "requested",
		RequestedAt:    "2026-07-06T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("add pause request: %v", err)
	}
	if record.Status != string(domainjob.StatusPauseRequested) ||
		pauseRequest.ID != "pause_1" ||
		record.PauseState.PauseRequestID != "pause_1" {
		t.Fatalf("pause request state mismatch: record=%#v request=%#v", record, pauseRequest)
	}
	token := domainjob.ResumeToken{
		ResumeToken:    "resume_1",
		IssuedAt:       "2026-07-06T00:00:01Z",
		ExpiresAt:      "2026-07-07T00:00:01Z",
		ChildRunID:     record.ID,
		ParentThreadID: "thr_pause",
	}
	record, pauseRequest, err = manager.MarkPauseRequestPaused(record.ID, "pause_1", "2026-07-06T00:00:01Z", token)
	if err != nil {
		t.Fatalf("mark paused: %v", err)
	}
	if record.Status != string(domainjob.StatusPaused) ||
		!record.PauseState.CanResume ||
		pauseRequest.ResumeToken != "resume_1" {
		t.Fatalf("paused state mismatch: record=%#v request=%#v", record, pauseRequest)
	}
	record, _, err = manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), domainjob.SteerMessage{
		ID:             "steer_queued",
		ParentThreadID: "thr_pause",
		ChildRunID:     record.ID,
		JobID:          record.ID,
		Status:         "queued",
		CreatedAt:      "2026-07-06T00:00:02Z",
		Text:           "continue later",
	})
	if err != nil {
		t.Fatalf("queue steer while paused: %v", err)
	}
	record, _, err = manager.MarkPauseResumeRequested(record.ID, "pause_1")
	if err != nil {
		t.Fatalf("mark resume requested: %v", err)
	}
	record, pauseRequest, err = manager.MarkPauseRequestResumed(record.ID, "pause_1", "2026-07-06T00:00:03Z")
	if err != nil {
		t.Fatalf("mark resumed: %v", err)
	}
	if pauseRequest.Status != "resumed" || len(record.Steers) != 1 || record.Steers[0].Status != "queued" {
		t.Fatalf("resume should not admit queued steer directly: record=%#v request=%#v", record, pauseRequest)
	}
	record, _, err = manager.CompletePauseResume(record.ID, "pause_1")
	if err != nil {
		t.Fatalf("complete resume: %v", err)
	}
	record, _, err = manager.AddPauseRequest(record.ID, domainjob.PauseRequest{
		ID:             "pause_2",
		ParentThreadID: "thr_pause",
		ChildRunID:     record.ID,
		JobID:          record.ID,
		Status:         "requested",
		RequestedAt:    "2026-07-06T00:00:04Z",
	})
	if err != nil {
		t.Fatalf("add second pause request: %v", err)
	}
	secondToken := token
	secondToken.ResumeToken = "resume_2"
	secondToken.IssuedAt = "2026-07-06T00:00:05Z"
	secondToken.ExpiresAt = "2026-07-07T00:00:05Z"
	record, _, err = manager.MarkPauseRequestPaused(record.ID, "pause_2", "2026-07-06T00:00:05Z", secondToken)
	if err != nil {
		t.Fatalf("mark second pause request paused: %v", err)
	}
	record, err = manager.UpdateChildRun(record.ID, UpdateRequest{Status: "killed", Error: "stop"})
	if err != nil {
		t.Fatalf("kill paused child: %v", err)
	}
	if record.Status != "killed" || len(record.PauseRequests) != 2 || record.PauseRequests[1].Status != "expired" {
		t.Fatalf("terminal update should expire pause request: %#v", record)
	}
	if record.PauseRequests[1].ResumeToken != "" || record.PauseRequests[1].ResumedAt != "" || record.PauseState.Status != "expired" || record.PauseState.CanResume {
		t.Fatalf("terminal expiration retained resume authority or stale state: %#v", record)
	}
}

func TestPauseSettlementReplayIsExactAndImmutable(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_pause_replay", ParentThreadID: "thr_pause_replay", Kind: "subagent",
		Status: string(domainjob.StatusRunning), ChildThreadID: "child_pause_replay", Background: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	record, request, err := manager.AddPauseRequest(record.ID, domainjob.PauseRequest{
		ID: "pause_replay", ParentThreadID: record.ParentThreadID, ChildRunID: record.ID, JobID: record.ID,
		Status: "requested", RequestedAt: "2026-07-06T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	token := domainjob.ResumeToken{
		ResumeToken: "resume_replay", IssuedAt: "2026-07-06T00:00:01Z", ExpiresAt: "2026-07-07T00:00:01Z",
		ChildRunID: record.ID, ParentThreadID: record.ParentThreadID,
	}
	record, request, err = manager.MarkPauseRequestPaused(record.ID, request.ID, token.IssuedAt, token)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.ID+".json")
	pausedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.MarkPauseRequestPaused(record.ID, request.ID, token.IssuedAt, token); err != nil {
		t.Fatalf("exact paused replay failed: %v", err)
	}
	afterExact, _ := os.ReadFile(path)
	if !bytes.Equal(pausedBytes, afterExact) {
		t.Fatal("exact paused replay rewrote durable bytes")
	}
	changedToken := token
	changedToken.ResumeToken = "resume_replaced"
	if _, _, err := manager.MarkPauseRequestPaused(record.ID, request.ID, token.IssuedAt, changedToken); err == nil {
		t.Fatal("paused replay replaced the durable token")
	}
	afterRejected, _ := os.ReadFile(path)
	if !bytes.Equal(pausedBytes, afterRejected) {
		t.Fatal("rejected paused replay changed durable bytes")
	}
	if _, _, err := manager.MarkPauseResumeRequested(record.ID, request.ID); err != nil {
		t.Fatal(err)
	}
	record, request, err = manager.MarkPauseRequestResumed(record.ID, request.ID, "2026-07-06T00:00:02Z")
	if err != nil {
		t.Fatal(err)
	}
	if request.ResumeToken != "" || record.Status != string(domainjob.StatusResuming) {
		t.Fatalf("resume settlement retained usable authority: record=%#v request=%#v", record, request)
	}
	resumedBytes, _ := os.ReadFile(path)
	if _, _, err := manager.MarkPauseRequestResumed(record.ID, request.ID, "2026-07-06T00:00:02Z"); err != nil {
		t.Fatalf("exact resumed replay failed: %v", err)
	}
	afterResumedReplay, _ := os.ReadFile(path)
	if !bytes.Equal(resumedBytes, afterResumedReplay) {
		t.Fatal("exact resumed replay rewrote durable bytes")
	}
	if _, _, err := manager.MarkPauseRequestResumed(record.ID, request.ID, "2026-07-06T00:00:03Z"); err == nil {
		t.Fatal("resumed replay changed the durable settlement time")
	}
	afterResumedReject, _ := os.ReadFile(path)
	if !bytes.Equal(resumedBytes, afterResumedReject) {
		t.Fatal("rejected resumed replay changed durable bytes")
	}
}

func TestManagerReloadsChildRunsInNumericSequenceOrder(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	for i := 0; i < 12; i++ {
		if _, err := manager.StartChildRun(StartRequest{
			ParentGoalID:   "goal_order",
			ParentThreadID: "thr_order",
			Kind:           "subagent",
			Status:         "completed",
			Output:         "child",
		}); err != nil {
			t.Fatalf("start child run %d: %v", i+1, err)
		}
	}
	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	records := reloaded.Records()
	if len(records) != 12 {
		t.Fatalf("expected 12 records, got %#v", records)
	}
	for index, raw := range records {
		record, ok := raw.(Record)
		if !ok {
			t.Fatalf("record %d has unexpected type %#v", index, raw)
		}
		expectedID := fmt.Sprintf("job-%d", index+1)
		if record.ID != expectedID {
			t.Fatalf("records should reload in numeric id order at index %d: got %s, want %s; all=%#v", index, record.ID, expectedID, records)
		}
	}
}

func TestManagerChildSeqIsScopedToParentThread(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	firstParentA, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_seq",
		ParentThreadID: "thr_parent_a",
		Kind:           "subagent",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("start first parent A child: %v", err)
	}
	firstParentB, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_seq",
		ParentThreadID: "thr_parent_b",
		Kind:           "subagent",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("start first parent B child: %v", err)
	}
	secondParentA, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_seq",
		ParentThreadID: "thr_parent_a",
		Kind:           "subagent",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("start second parent A child: %v", err)
	}
	if firstParentA.ChildSeq != 1 || firstParentB.ChildSeq != 1 || secondParentA.ChildSeq != 2 {
		t.Fatalf("childSeq should be parent-scoped, got A1=%d B1=%d A2=%d", firstParentA.ChildSeq, firstParentB.ChildSeq, secondParentA.ChildSeq)
	}
	reloaded, err := NewManager(root)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	secondParentB, err := reloaded.StartChildRun(StartRequest{
		ParentGoalID:   "goal_seq",
		ParentThreadID: "thr_parent_b",
		Kind:           "subagent",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("start second parent B child after reload: %v", err)
	}
	if secondParentB.ID != "job-4" || secondParentB.ChildSeq != 2 {
		t.Fatalf("reloaded manager should keep global ids but parent-scoped childSeq, got %#v", secondParentB)
	}
}

func TestManagerPersistsSteerQueueAndAdmissionState(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_steer",
		ParentThreadID: "thr_parent",
		ParentTurnID:   "turn_parent",
		Kind:           "subagent",
		Status:         "running",
		Background:     true,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	record, err = manager.UpdateChildRun(record.ID, UpdateRequest{
		ChildThreadID: "thr_child",
		ChildTurnID:   "turn_child",
	})
	if err != nil {
		t.Fatalf("bind child turn lineage: %v", err)
	}
	queued, message, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), domainjob.SteerMessage{
		ID:             "steer_1",
		JobID:          record.ID,
		ChildRunID:     record.ID,
		ParentThreadID: "thr_parent",
		Text:           "focus",
		Status:         "queued",
		CreatedAt:      "2026-07-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("queue steer: %v", err)
	}
	if message.Status != "queued" || queued.SteerState.PendingSteers != 1 || queued.SteerState.SteerCount != 1 || !queued.SteerState.CanAcceptSteer {
		t.Fatalf("queued steer state mismatch: message=%#v record=%#v", message, queued)
	}
	admitted, admittedMessage, err := manager.AdmitSteerMessage(record.ID, "steer_1", "2026-07-01T00:00:01Z")
	if err != nil {
		t.Fatalf("admit steer: %v", err)
	}
	if admittedMessage.Status != "admitted" ||
		admittedMessage.AdmittedAt != "2026-07-01T00:00:01Z" ||
		admitted.SteerState.PendingSteers != 0 ||
		admitted.SteerState.AdmittedSteers != 1 ||
		admitted.SteerState.SteerCount != 1 {
		t.Fatalf("admitted steer state mismatch: message=%#v record=%#v", admittedMessage, admitted)
	}
	replayed, replayedMessage, err := manager.AdmitSteerMessage(record.ID, "steer_1", "2026-07-01T00:00:01Z")
	if err != nil || replayedMessage.Status != "admitted" || replayedMessage.AdmittedAt != admittedMessage.AdmittedAt || replayed.ID != admitted.ID {
		t.Fatalf("exact admitted replay was not idempotent: record=%#v message=%#v err=%v", replayed, replayedMessage, err)
	}
	if _, _, err := manager.AdmitSteerMessage(record.ID, "steer_1", "2026-07-01T00:00:02Z"); err == nil {
		t.Fatal("conflicting admitted replay changed committed time")
	}
	if _, _, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, admitted), domainjob.SteerMessage{
		ID:             "steer_2",
		JobID:          record.ID,
		ChildRunID:     record.ID,
		ParentThreadID: "thr_parent",
		Text:           "late focus",
		Status:         "queued",
		CreatedAt:      "2026-07-01T00:00:02Z",
	}); err != nil {
		t.Fatalf("queue second steer: %v", err)
	}
	killed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: "killed",
		Error:  "stopped by parent",
	})
	if err != nil {
		t.Fatalf("kill child run: %v", err)
	}
	if killed.SteerState.PendingSteers != 0 || killed.SteerState.CanAcceptSteer {
		t.Fatalf("terminal child should not keep pending steer state: %#v", killed)
	}
	foundExpired := false
	for _, steer := range killed.Steers {
		if steer.ID == "steer_2" {
			foundExpired = steer.Status == "expired" && steer.RejectedReason == "child_run_terminal"
		}
	}
	if !foundExpired {
		t.Fatalf("queued steer should expire on terminal child state: %#v", killed.Steers)
	}
}

func TestRejectSteerMessageTransitionsExistingQueuedRecord(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_reject_steer", ParentThreadID: "thr_parent", ParentTurnID: "turn_parent",
		Kind: "subagent", Status: "running", Background: true, ChildThreadID: "thr_child", ChildTurnID: "turn_child",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	message := domainjob.SteerMessage{
		ID: "steer_reject", JobID: record.ID, ChildRunID: record.ID, ParentThreadID: record.ParentThreadID,
		Text: "projected guidance", Status: "queued", CreatedAt: "2026-07-01T00:00:00Z",
	}
	_, queuedMessage, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), message)
	if err != nil {
		t.Fatalf("queue steer: %v", err)
	}
	updated, rejected, err := manager.RejectSteerMessage(record.ID, queuedMessage, "append_parent_turn_failed")
	if err != nil {
		t.Fatalf("reject steer: %v", err)
	}
	if rejected.Status != "rejected" || updated.SteerState.PendingSteers != 0 || len(updated.Steers) != 1 || updated.Steers[0].Status != "rejected" {
		t.Fatalf("existing queued steer was not durably rejected: message=%#v record=%#v", rejected, updated)
	}
	replayed, replayedMessage, err := manager.RejectSteerMessage(record.ID, queuedMessage, "append_parent_turn_failed")
	if err != nil || replayed.UpdatedAt != updated.UpdatedAt || replayedMessage.Status != "rejected" || replayedMessage.Text != "" {
		t.Fatalf("exact rejected replay was not idempotent: record=%#v message=%#v err=%v", replayed, replayedMessage, err)
	}
	if _, _, err := manager.RejectSteerMessage(record.ID, queuedMessage, "job_not_completed"); err == nil {
		t.Fatal("rejected replay changed committed reason")
	}
	tampered := queuedMessage
	tampered.Text = "different projected guidance"
	tampered.ContentDigest = domainjob.SteerMessageContentDigestV1(tampered)
	if _, _, err := manager.RejectSteerMessage(record.ID, tampered, "append_parent_turn_failed"); err == nil {
		t.Fatal("rejected replay accepted different private content")
	}
}

func TestSettleSteerPromotionExactBindsCommitAndIsIdempotent(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_promote_steer", ParentThreadID: "thr_parent", ParentTurnID: "turn_parent",
		Kind: "subagent", Status: "running", Background: true, ChildThreadID: "thr_child", ChildTurnID: "turn_child",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	_, queued, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), domainjob.SteerMessage{
		ID: "steer_promoted", JobID: record.ID, ChildRunID: record.ID, ParentThreadID: record.ParentThreadID,
		Text: "projected guidance", Status: "queued", CreatedAt: "2026-07-18T01:02:03Z",
	})
	if err != nil {
		t.Fatalf("queue steer: %v", err)
	}
	settlement := domainjob.SteerPromotionSettlementV1{
		Version:           domainjob.SteerPromotionSettlementVersionV1,
		PromotionCommitID: "steer_commit_" + strings.Repeat("b", 64),
		PromotionEntryID:  "item_steer_" + strings.Repeat("c", 64),
		ContextDigest:     queued.ContextDigest,
		PromotedAt:        "2026-07-18T01:02:04Z",
	}
	updated, admitted, err := manager.SettleSteerPromotionExact(record.ID, queued, settlement)
	if err != nil || admitted.Status != "admitted" || admitted.AdmittedAt != settlement.PromotedAt ||
		admitted.PromotionCommitID != settlement.PromotionCommitID || admitted.PromotionEntryID != settlement.PromotionEntryID {
		t.Fatalf("settle steer promotion: record=%#v message=%#v err=%v", updated, admitted, err)
	}
	replayed, replayedMessage, err := manager.SettleSteerPromotionExact(record.ID, queued, settlement)
	if err != nil || replayed.UpdatedAt != updated.UpdatedAt || replayedMessage.PromotionCommitID != settlement.PromotionCommitID {
		t.Fatalf("exact promotion replay was not idempotent: record=%#v message=%#v err=%v", replayed, replayedMessage, err)
	}
	conflict := settlement
	conflict.PromotionCommitID = "steer_commit_" + strings.Repeat("d", 64)
	if _, _, err := manager.SettleSteerPromotionExact(record.ID, queued, conflict); err == nil {
		t.Fatal("conflicting promotion commit replaced durable settlement")
	}
	tampered := queued
	tampered.Text = "different guidance"
	tampered.ContentDigest = domainjob.SteerMessageContentDigestV1(tampered)
	if _, _, err := manager.SettleSteerPromotionExact(record.ID, tampered, settlement); err == nil {
		t.Fatal("promotion settlement accepted different queued content")
	}
}

func TestQueueSteerMessageRejectsMismatchedDuplicatePayload(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_replay_steer", ParentThreadID: "thr_parent", ParentTurnID: "turn_parent",
		Kind: "subagent", Status: "running", Background: true, ChildThreadID: "thr_child", ChildTurnID: "turn_child",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	message := domainjob.SteerMessage{
		ID: "steer_replay", JobID: record.ID, ChildRunID: record.ID, ParentThreadID: record.ParentThreadID,
		Text: "case-risk payload B", Status: "queued", CreatedAt: "2026-07-01T00:00:00Z",
	}
	queued, queuedMessage, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), message)
	if err != nil {
		t.Fatalf("queue initial steer: %v", err)
	}
	queuedMessage.Text = "safe payload A"
	queuedMessage.ContentDigest = ""
	if _, _, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, queued), queuedMessage); err == nil {
		t.Fatal("same steer id accepted a different payload")
	}
}

func TestTerminalTransitionWinsSteerQueueCAS(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_terminal_steer", ParentThreadID: "thr_parent", ParentTurnID: "turn_parent",
		Kind: "subagent", Status: "running", Background: true, ChildThreadID: "thr_child", ChildTurnID: "turn_child",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	authority := testSteerQueueAuthority(t, record)
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: "killed", Error: "stopped"}); err != nil {
		t.Fatalf("kill child run: %v", err)
	}
	if _, _, err := manager.QueueSteerMessage(record.ID, authority, domainjob.SteerMessage{
		ID: "steer_after_kill", JobID: record.ID, ChildRunID: record.ID, ParentThreadID: record.ParentThreadID,
		Text: "must not queue", Status: "queued", CreatedAt: "2026-07-01T00:00:00Z",
	}); err == nil {
		t.Fatal("stale running authority queued a steer after terminal transition")
	}
	current, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("reload child run: %v", err)
	}
	if len(current.Steers) != 0 || current.SteerState.PendingSteers != 0 {
		t.Fatalf("terminal queue CAS left steer state: %#v", current)
	}
}

func TestTamperedPersistedSteerFailsClosedOnRestart(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID: "goal_tamper_steer", ParentThreadID: "thr_parent", ParentTurnID: "turn_parent",
		Kind: "subagent", Status: "running", Background: true, ChildThreadID: "thr_child", ChildTurnID: "turn_child",
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	if _, _, err := manager.QueueSteerMessage(record.ID, testSteerQueueAuthority(t, record), domainjob.SteerMessage{
		ID: "steer_tamper", JobID: record.ID, ChildRunID: record.ID, ParentThreadID: record.ParentThreadID,
		Text: "safe projected text", Status: "queued", CreatedAt: "2026-07-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("queue steer: %v", err)
	}
	path := filepath.Join(root, record.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read child record: %v", err)
	}
	data = bytes.Replace(data, []byte("safe projected text"), []byte("case-risk payload BB"), 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("tamper child record: %v", err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("restart accepted a steer whose content no longer matched its digest")
	}
}

func testSteerQueueAuthority(t *testing.T, record domainjob.Record) domainjob.SteerQueueAuthorityV1 {
	t.Helper()
	pending := strings.TrimSpace(record.ChildTurnID) == ""
	contextDigest := ""
	if !pending {
		contextDigest = strings.Repeat("a", 64)
	}
	authority, err := domainjob.NewSteerQueueAuthorityV1(record, contextDigest, pending)
	if err != nil {
		t.Fatalf("build steer queue authority: %v", err)
	}
	return authority
}

func TestManagerBackfillsLegacyChildSeqWithoutDuplicates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "job-1.json"), []byte(`{
  "id": "job-1",
  "parentGoalId": "goal_legacy",
  "parentThreadId": "thr_legacy",
  "kind": "subagent",
  "status": "completed"
}`), 0o600); err != nil {
		t.Fatalf("write legacy child without childSeq: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "job-2.json"), []byte(`{
  "id": "job-2",
  "childSeq": 1,
  "parentGoalId": "goal_legacy",
  "parentThreadId": "thr_legacy",
  "kind": "subagent",
  "status": "completed"
}`), 0o600); err != nil {
		t.Fatalf("write legacy child with childSeq: %v", err)
	}
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	records := manager.RecordsByParentThread("thr_legacy")
	if len(records) != 2 {
		t.Fatalf("expected two legacy records, got %#v", records)
	}
	seen := map[int]bool{}
	for _, record := range records {
		if seen[record.ChildSeq] {
			t.Fatalf("legacy childSeq backfill duplicated sequence %d in %#v", record.ChildSeq, records)
		}
		seen[record.ChildSeq] = true
	}
	next, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_legacy",
		ParentThreadID: "thr_legacy",
		Kind:           "subagent",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("start child after legacy reload: %v", err)
	}
	if next.ID != "job-3" || next.ChildSeq != 3 {
		t.Fatalf("new child should continue legacy parent-scoped sequence, got %#v", next)
	}
}

func TestManagerPersistsHeartbeatLeaseRecoveryAndDeadLetterState(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	record, err := manager.StartChildRun(StartRequest{
		ParentGoalID:   "goal_health",
		ParentThreadID: "thr_health",
		Kind:           "subagent",
		Status:         "running",
		Background:     true,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	if record.LastHeartbeatAt == "" || !strings.HasPrefix(record.LeaseOwner, defaultTaskJobLeaseOwner+":") ||
		record.LeaseExpiresAt == "" || record.StaleAfterMs != defaultTaskJobStaleAfter.Milliseconds() {
		t.Fatalf("running job should persist heartbeat lease defaults: %#v", record)
	}
	initialHeartbeat := record.LastHeartbeatAt
	initialLeaseOwner := record.LeaseOwner
	updated, err := manager.UpdateChildRun(record.ID, UpdateRequest{Output: "still working"})
	if err != nil {
		t.Fatalf("update running child run: %v", err)
	}
	if updated.LastHeartbeatAt == "" || updated.LeaseExpiresAt == "" || updated.LastHeartbeatAt < initialHeartbeat {
		t.Fatalf("running update should refresh heartbeat/lease: before=%q after=%#v", initialHeartbeat, updated)
	}
	if updated.LeaseOwner != initialLeaseOwner {
		t.Fatalf("running update should keep the same runtime lease owner: before=%q after=%q", initialLeaseOwner, updated.LeaseOwner)
	}

	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{Status: "completed", Output: "done"}); err != nil {
		t.Fatalf("complete child run: %v", err)
	}
	deadLettered, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		CompletionDeliveryStatus: "dead_letter",
		CompletionDeliveryReason: "parent_missing",
		CompletionDeliveryError:  "<think>PRIVATE_DELIVERY_REASONING</think> account=6222020202020202020",
	})
	if err != nil {
		t.Fatalf("dead-letter update: %v", err)
	}
	if deadLettered.DeadLetterReason != "parent_missing" ||
		deadLettered.RecoveryStatus != "dead_lettered" ||
		deadLettered.CompletionDeliveryError != "" ||
		deadLettered.CompletionDeadLetterAt == "" ||
		deadLettered.CompletionDeliveryAttempts != 1 {
		t.Fatalf("dead-letter state should be durable: %#v", deadLettered)
	}
	durable, readErr := os.ReadFile(filepath.Join(root, deadLettered.ID+".json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(durable), "PRIVATE_DELIVERY_REASONING") || strings.Contains(string(durable), "6222020202020202020") {
		t.Fatalf("delivery error leaked to child-run storage: %s", durable)
	}

	recoverable := map[string]Record{}
	for _, status := range []string{
		string(domainjob.StatusQueued),
		string(domainjob.StatusPauseRequested),
		string(domainjob.StatusPaused),
		string(domainjob.StatusResumeRequested),
		string(domainjob.StatusResuming),
	} {
		started := startClaimFixture(t, manager, status)
		recoverable[started.ID] = started
	}
	cleaned, err := manager.CleanupStaleRunningRecords()
	if err != nil {
		t.Fatalf("cleanup stale running: %v", err)
	}
	found := map[string]bool{}
	for _, item := range cleaned {
		if _, ok := recoverable[item.ID]; !ok {
			continue
		}
		found[item.ID] = item.Status == "interrupted" &&
			item.Orphaned &&
			item.RecoveryStatus == "" &&
			item.RecoveryAttempt == 0 &&
			item.RecoveryReason == "runtime_restart_orphaned_running_job"
	}
	if len(found) != len(recoverable) {
		t.Fatalf("cleanup should record orphan recovery, cleaned=%#v", cleaned)
	}
	for id := range recoverable {
		if !found[id] {
			t.Fatalf("cleanup should close %s without claiming recovery authority, cleaned=%#v", id, cleaned)
		}
	}
}
