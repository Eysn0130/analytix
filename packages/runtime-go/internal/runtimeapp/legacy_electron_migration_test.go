package runtimeapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	electronlegacytask "analytix.local/runtime-go/internal/adapters/outbound/electronlegacytask"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/jobs"
)

func TestDesktopPrivateHistoryMigrationV2RetiresElectronAndChildRunBeforeReady(t *testing.T) {
	base := t.TempDir()
	isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	for _, root := range []string{data, durable, userData, filepath.Join(data, "child-runs")} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	const legacyParentThreadID = "thr_legacy_parent"
	const legacyParentCallID = "call_00_legacy_parallel_parent"
	legacyParentThread := map[string]any{
		"id": legacyParentThreadID, "title": "Legacy parent", "workspace": "/workspace", "mode": "agent", "status": "idle",
		"approvalPolicy": "on-request", "sandboxMode": "workspace-write", "relation": "primary",
		"createdAt": "2026-07-01T00:00:00Z", "updatedAt": "2026-07-01T00:00:01Z", "turns": []any{map[string]any{
			"id": "turn-parent", "threadId": legacyParentThreadID, "status": "completed", "createdAt": "2026-07-01T00:00:00Z",
			"finishedAt": "2026-07-01T00:00:01Z", "items": []any{map[string]any{
				"id": "item_legacy_parallel_parent", "kind": "tool_call", "role": "assistant",
				"callId": legacyParentCallID, "threadId": legacyParentThreadID, "turnId": "turn-parent", "toolName": "parallel_tasks", "status": "completed",
				"createdAt": "2026-07-01T00:00:00Z", "finishedAt": "2026-07-01T00:00:01Z", "arguments": map[string]any{"task": "legacy"},
			}},
		}},
	}
	legacyParentRoot := filepath.Join(durable, "threads", legacyParentThreadID)
	if err := os.MkdirAll(legacyParentRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyParentMetadata := threadapp.MetadataSidecarEntry(
		threadapp.StripItemsForSidecar(legacyParentThread), "2026-07-01T00:00:01Z",
	)
	legacyParentToolItem := legacyParentThread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	if legacyParentMetadata == nil {
		t.Fatal("legacy sidecar-only parent fixture did not form metadata")
	}
	legacyParentMetadataBody, err := json.Marshal(legacyParentMetadata)
	if err != nil {
		t.Fatal(err)
	}
	legacyParentToolBody, err := json.Marshal(legacyParentToolItem)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyParentRoot, "metadata.jsonl"), append(legacyParentMetadataBody, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyParentRoot, "messages.jsonl"), append(legacyParentToolBody, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(userData, electronlegacytask.BackgroundTaskFileV1)
	opaque := []byte("reasoning: must never survive migration\naccount=6222020202020202020\x00")
	if err := os.WriteFile(target, opaque, 0o600); err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(data, "child-runs", "job-1.json")
	legacyChild := `{
  "id":"job-1",
  "parentGoalId":"goal",
  "parentThreadId":"thread",
  "kind":"background-shell",
  "name":"worker<think>PRIVATE_CHILD_NAME</think>",
  "label":"ordinary child diagnostic",
  "prompt":"{\"reasoning_content\":\"PRIVATE_CHILD_PROMPT\"}",
  "profileDescription":"reviewer<think>PRIVATE_CHILD_PROFILE</think>",
  "diffSummary":"1 file<think>PRIVATE_CHILD_DIFF</think>",
  "status":"completed",
  "output":"ordinary child output<think>PRIVATE_CHILD_OUTPUT</think> account 6222020202020202020",
  "error":"ordinary child error<think>PRIVATE_CHILD_ERROR</think>",
  "autoContinueError":"PRIVATE_CHILD_AUTO",
  "completionDeliveryError":"PRIVATE_CHILD_DELIVERY",
  "toolInvocations":0
}`
	if err := os.WriteFile(childPath, []byte(legacyChild), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyTypeScriptPath := filepath.Join(data, "child-runs", "child_mr1b6yo0_abc123.json")
	legacyTypeScriptChild := fmt.Sprintf(`{
  "id":"child_mr1b6yo0_abc123",
  "parentThreadId":%q,
  "parentTurnId":"turn-parent",
  "parentToolCallId":%q,
  "childThreadId":"child_mr1b6yo0_abc123",
  "childTurnId":"turn-child",
  "prompt":"PRIVATE_TYPESCRIPT_CHILD_PROMPT",
  "status":"completed",
  "summary":"PRIVATE_TYPESCRIPT_CHILD_SUMMARY",
  "usage":{"promptTokens":1,"completionTokens":1,"totalTokens":2},
  "transcriptItems":[{"type":"assistant_message","content":"PRIVATE_TYPESCRIPT_CHILD_TRANSCRIPT"}],
  "toolInvocations":1,
  "durationMs":1,
  "queuedMs":1,
  "createdAt":"2026-07-01T00:00:00.000Z",
  "startedAt":"2026-07-01T00:00:00.001Z",
  "updatedAt":"2026-07-01T00:00:00.002Z"
}`, legacyParentThreadID, legacyParentCallID)
	if err := os.WriteFile(legacyTypeScriptPath, []byte(legacyTypeScriptChild), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyTypeScriptSiblingPath := filepath.Join(data, "child-runs", "child_mr1b6yo0_abc124.json")
	legacyTypeScriptSibling := strings.ReplaceAll(legacyTypeScriptChild, "child_mr1b6yo0_abc123", "child_mr1b6yo0_abc124")
	if err := os.WriteFile(legacyTypeScriptSiblingPath, []byte(legacyTypeScriptSibling), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyTypeScriptThirdPath := filepath.Join(data, "child-runs", "child_mr1b6yo0_abc125.json")
	legacyTypeScriptThird := strings.ReplaceAll(legacyTypeScriptChild, "child_mr1b6yo0_abc123", "child_mr1b6yo0_abc125")
	if err := os.WriteFile(legacyTypeScriptThirdPath, []byte(legacyTypeScriptThird), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyTypeScriptFourthPath := filepath.Join(data, "child-runs", "child_mr1b6yo0_abc126.json")
	legacyTypeScriptFourth := strings.ReplaceAll(legacyTypeScriptChild, "child_mr1b6yo0_abc123", "child_mr1b6yo0_abc126")
	if err := os.WriteFile(legacyTypeScriptFourthPath, []byte(legacyTypeScriptFourth), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy task remains: %v", err)
	}
	journal := filepath.Join(userData, journalDirectoryV1ForTest, "committed.json")
	if _, err := os.Lstat(journal); err != nil {
		t.Fatalf("owner commit is missing: %v", err)
	}
	childBody, err := os.ReadFile(childPath)
	if err != nil {
		t.Fatal(err)
	}
	lowerChild := strings.ToLower(string(childBody))
	for _, forbidden := range []string{"private_child", "reasoning_content", "<think", "6222020202020202020"} {
		if strings.Contains(lowerChild, forbidden) {
			t.Fatalf("desktop activation migration retained %q: %s", forbidden, childBody)
		}
	}
	if !strings.Contains(string(childBody), "background shell") ||
		!strings.Contains(string(childBody), "ordinary child output") ||
		!strings.Contains(string(childBody), "[ACCOUNT]") {
		t.Fatalf("desktop activation migration removed public child diagnostics: %s", childBody)
	}
	if _, err := os.Lstat(legacyTypeScriptPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired TypeScript child source remains after migration: %v", err)
	}
	if _, err := os.Lstat(legacyTypeScriptSiblingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired TypeScript sibling source remains after migration: %v", err)
	}
	if _, err := os.Lstat(legacyTypeScriptThirdPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired TypeScript third source remains after migration: %v", err)
	}
	if _, err := os.Lstat(legacyTypeScriptFourthPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired TypeScript fourth source remains after migration: %v", err)
	}
	for _, jobID := range []string{"job-2", "job-3", "job-4", "job-5"} {
		legacyProjection, err := os.ReadFile(filepath.Join(data, "child-runs", jobID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"PRIVATE_TYPESCRIPT", "transcriptItems", "evidenceLedgered"} {
			if bytes.Contains(legacyProjection, []byte(forbidden)) {
				t.Fatalf("retired TypeScript child projection retained %q: %s", forbidden, legacyProjection)
			}
		}
		if !bytes.Contains(legacyProjection, []byte(`"sourceRef": "analytix-legacy-typescript-child-run/v1/sha256/`)) ||
			!bytes.Contains(legacyProjection, []byte(`"parentThreadId": "`+legacyParentThreadID+`"`)) ||
			bytes.Contains(legacyProjection, []byte(`"childThreadId"`)) || bytes.Contains(legacyProjection, []byte(`"childTurnId"`)) {
			t.Fatalf("retired TypeScript child projection lost verified parent provenance or retained unverified child lineage: %s", legacyProjection)
		}
	}
	if _, err := jobs.NewManager(filepath.Join(data, "child-runs")); err != nil {
		t.Fatalf("migrated child-run storage does not reload: %v", err)
	}
	if bytes.Contains(readAllTreeForTest(t, userData), opaque) {
		t.Fatal("retired reasoning bytes remain in the Electron owner tree")
	}
	settledDigest := startupWholeTreeDigest(t, data, durable, userData)
	settledRecords := startupWholeTreeRecordMapForTest(t, data, durable, userData)
	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err != nil {
		t.Fatalf("repeat desktop migration: %v", err)
	}
	if repeatedDigest := startupWholeTreeDigest(t, data, durable, userData); repeatedDigest != settledDigest {
		t.Fatalf("repeat desktop migration was not byte-stable: before=%s after=%s changed=%v", settledDigest, repeatedDigest,
			changedWholeTreeRecordKeysForTest(settledRecords, startupWholeTreeRecordMapForTest(t, data, durable, userData)))
	}
	handler, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        data,
		DurableTempDir: durable,
		UserDataDir:    userData,
		ProviderID:     "openai",
		BaseURL:        "https://provider.example.invalid/v1",
		Model:          "gpt-test",
		EndpointFormat: "chat-completions",
		ApprovalPolicy: "on-request",
		SandboxMode:    "workspace-write",
	})
	if err != nil {
		t.Fatalf("ordinary runtime startup after desktop migration: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
}

func TestDesktopPrivateHistoryMigrationV2OwnerPreflightFailureMutatesNothing(t *testing.T) {
	base := t.TempDir()
	configRoot := isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	for _, root := range []string{data, durable, userData} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	external := filepath.Join(base, "external")
	if err := os.WriteFile(external, []byte("reasoning witness"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(userData, electronlegacytask.BackgroundTaskFileV1)
	if err := os.Symlink(external, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	childRoot := filepath.Join(data, "child-runs")
	if err := os.MkdirAll(childRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(childRoot, "job-1.json")
	childBody := []byte(`{"id":"job-1","parentGoalId":"goal","parentThreadId":"thread","kind":"background-shell","status":"completed","output":"public<think>PRIVATE_PREFLIGHT_CHILD</think>","toolInvocations":0}`)
	if err := os.WriteFile(childPath, childBody, 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	before, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err == nil {
		t.Fatal("unsafe Electron owner target passed preflight")
	}
	after, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if before.SHA256 != after.SHA256 {
		t.Fatal("failed owner preflight changed Go-owned persistence")
	}
	childAfter, err := os.ReadFile(childPath)
	if err != nil || !bytes.Equal(childAfter, childBody) {
		t.Fatalf("failed Electron preflight ran child semantic migration: body=%q err=%v", childAfter, err)
	}
	if _, err := os.Lstat(filepath.Join(userData, journalDirectoryV1ForTest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed preflight created a new owner transaction: %v", err)
	}
	assertPathAbsentForTest(t, filepath.Join(configRoot, "analytix", "startup-authority-v1"))
	body, err := os.ReadFile(external)
	if err != nil || !bytes.Equal(body, []byte("reasoning witness")) {
		t.Fatalf("external witness changed: %q, %v", body, err)
	}
}

func TestDesktopPrivateHistoryMigrationV2SemanticFailurePreservesElectronOwner(t *testing.T) {
	base := t.TempDir()
	configRoot := isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	childRoot := filepath.Join(data, "child-runs")
	for _, root := range []string{data, durable, userData, childRoot} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(userData, electronlegacytask.BackgroundTaskFileV1)
	opaque := []byte("opaque reasoning and account 6222020202020202020\x00")
	if err := os.WriteFile(target, opaque, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(childRoot, "job-1.json")
	invalidChild := []byte(`{"id":"wrong-job-id"}`)
	if err := os.WriteFile(childPath, invalidChild, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeGoTree := startupWholeTreeDigest(t, data, durable)

	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err == nil {
		t.Fatal("invalid child semantic state reached desktop ready")
	}
	afterInfo, err := os.Lstat(target)
	if err != nil || !os.SameFile(beforeInfo, afterInfo) || afterInfo.Mode() != beforeInfo.Mode() {
		t.Fatalf("semantic failure replaced the Electron target: before=%v after=%v err=%v", beforeInfo, afterInfo, err)
	}
	afterOpaque, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(afterOpaque, opaque) {
		t.Fatalf("semantic failure changed Electron bytes: body=%q err=%v", afterOpaque, err)
	}
	afterChild, err := os.ReadFile(childPath)
	if err != nil || !bytes.Equal(afterChild, invalidChild) {
		t.Fatalf("semantic failure changed invalid child bytes: body=%q err=%v", afterChild, err)
	}
	if afterGoTree := startupWholeTreeDigest(t, data, durable); afterGoTree != beforeGoTree {
		t.Fatalf("semantic failure mutated Go-owned roots: before=%s after=%s", beforeGoTree, afterGoTree)
	}
	if _, err := os.Lstat(filepath.Join(userData, journalDirectoryV1ForTest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("semantic failure created an Electron retirement journal: %v", err)
	}
	assertPathAbsentForTest(t, filepath.Join(configRoot, "analytix", "startup-authority-v1"))
}

func TestDesktopPrivateHistoryMigrationV2SharedLeaseBlocksSecondActivation(t *testing.T) {
	base := t.TempDir()
	isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	for _, root := range []string{data, durable, userData} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(userData, electronlegacytask.BackgroundTaskFileV1)
	if err := os.WriteFile(target, []byte("opaque"), 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	held, err := persistencefs.AcquireCompositeLeaseWithSeparateOwnerRoots(roots, userData)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); !errors.Is(err, persistencefs.ErrPersistenceInUse) {
		t.Fatalf("second activation = %v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(body, []byte("opaque")) {
		t.Fatalf("blocked activation changed target: %q, %v", body, err)
	}
	if _, err := os.Lstat(filepath.Join(userData, journalDirectoryV1ForTest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blocked activation created a journal: %v", err)
	}
}

func TestDesktopPrivateHistoryMigrationV2AbsentRootsRemainAbsent(t *testing.T) {
	base := t.TempDir()
	configRoot := isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{data, durable, userData} {
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("absent root was created: %s: %v", root, err)
		}
	}
	assertPathAbsentForTest(t, filepath.Join(configRoot, "analytix", "startup-authority-v1"))
}

func TestDesktopPrivateHistoryMigrationV2DefersManagedOnlyReasoningToRuntimeStartup(t *testing.T) {
	base := t.TempDir()
	isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	initial, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        data,
		DurableTempDir: durable,
		UserDataDir:    userData,
	})
	if err != nil {
		t.Fatalf("initialize authenticated managed persistence: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	threadRoot := filepath.Join(durable, "threads", "thr_managed_only_reasoning")
	if err := os.MkdirAll(threadRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"id":"thr_managed_only_reasoning","title":"managed history","workspace":"","mode":"agent","status":"idle","approvalPolicy":"on-request","sandboxMode":"workspace-write","relation":"primary","createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-20T00:00:00Z","turns":[{"id":"turn_1","items":[{"id":"reasoning","turnId":"turn_1","kind":"assistant_reasoning","text":"PRIVATE_MANAGED_ONLY_REASONING"},{"id":"answer","turnId":"turn_1","kind":"assistant_text","text":"public answer"}]}]}`)
	threadPath := filepath.Join(threadRoot, "thread.json")
	if err := os.WriteFile(threadPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err != nil {
		t.Fatalf("managed-only Electron preflight: %v", err)
	}
	body, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, legacy) {
		t.Fatalf("Electron-only preflight changed managed bytes: %s", body)
	}
	runtime, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        data,
		DurableTempDir: durable,
		UserDataDir:    userData,
	})
	if err != nil {
		t.Fatalf("managed semantic runtime startup: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, runtime)
	body, err = os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("PRIVATE_MANAGED_ONLY_REASONING")) || bytes.Contains(body, []byte("assistant_reasoning")) {
		t.Fatalf("managed-only reasoning reached runtime ready: %s", body)
	}
	if !bytes.Contains(body, []byte("public answer")) {
		t.Fatalf("managed runtime migration removed public answer: %s", body)
	}
	if _, err := os.Lstat(filepath.Join(userData, electronlegacytask.BackgroundTaskFileV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed-only migration fabricated an Electron task: %v", err)
	}
}

func TestDesktopPrivateHistoryMigrationV2DefersManagedOnlyAuthorityBootstrapToRuntimeStartup(t *testing.T) {
	base := t.TempDir()
	isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	if err := os.Mkdir(userData, 0o700); err != nil {
		t.Fatal(err)
	}
	threadRoot := filepath.Join(durable, "threads", "thr_managed_only_bootstrap")
	if err := os.MkdirAll(threadRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	threadPath := filepath.Join(threadRoot, "thread.json")
	legacy := []byte(`{"id":"thr_managed_only_bootstrap","title":"managed history","workspace":"","mode":"agent","status":"idle","approvalPolicy":"on-request","sandboxMode":"workspace-write","relation":"primary","createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-20T00:00:00Z","turns":[{"id":"turn_1","items":[{"id":"reasoning","turnId":"turn_1","kind":"assistant_reasoning","text":"PRIVATE_MANAGED_BOOTSTRAP_REASONING"},{"id":"answer","turnId":"turn_1","kind":"assistant_text","text":"public bootstrap answer"}]}]}`)
	if err := os.WriteFile(threadPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err != nil {
		t.Fatalf("bootstrap managed-only Electron preflight: %v", err)
	}
	body, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, legacy) {
		t.Fatalf("Electron-only preflight changed bootstrap managed bytes: %s", body)
	}
	assertPathAbsentForTest(t, filepath.Join(userData, "startup-authority-v1"))
	runtime, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        data,
		DurableTempDir: durable,
		UserDataDir:    userData,
	})
	if err != nil {
		t.Fatalf("bootstrap managed semantic runtime startup: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, runtime)
	body, err = os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("PRIVATE_MANAGED_BOOTSTRAP_REASONING")) || bytes.Contains(body, []byte("assistant_reasoning")) {
		t.Fatalf("bootstrap managed-only reasoning reached runtime ready: %s", body)
	}
	if !bytes.Contains(body, []byte("public bootstrap answer")) {
		t.Fatalf("bootstrap managed runtime migration removed public answer: %s", body)
	}
}

func TestDesktopPrivateHistoryMigrationV2SettledOwnerDefersManagedFailureToRuntime(t *testing.T) {
	base := t.TempDir()
	isolateDesktopMigrationAuthorityConfig(t, base)
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	userData := filepath.Join(base, "electron")
	for _, root := range []string{data, durable, userData} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(userData, electronlegacytask.BackgroundTaskFileV1)
	if err := os.WriteFile(target, []byte("opaque legacy task"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err != nil {
		t.Fatalf("initial Electron retirement: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy Electron target remains: %v", err)
	}
	invalidRoot := filepath.Join(data, "child-runs")
	if err := os.MkdirAll(invalidRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(invalidRoot, "job-invalid.json")
	invalidBody := []byte(`{"id":"wrong-job-id"}`)
	if err := os.WriteFile(invalidPath, invalidBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunDesktopPrivateHistoryMigrationV2(context.Background(), data, durable, userData); err != nil {
		t.Fatalf("settled Electron owner repeated preflight: %v", err)
	}
	if body, err := os.ReadFile(invalidPath); err != nil || !bytes.Equal(body, invalidBody) {
		t.Fatalf("settled Electron preflight changed managed failure: body=%q err=%v", body, err)
	}
	if runtime, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        data,
		DurableTempDir: durable,
		UserDataDir:    userData,
	}); err == nil {
		shutdownOwnedRuntimeHandler(t, runtime)
		t.Fatal("managed failure reached runtime ready")
	}
}

func isolateDesktopMigrationAuthorityConfig(t *testing.T, base string) string {
	t.Helper()
	home := filepath.Join(base, "home")
	config := filepath.Join(base, "config")
	for _, path := range []string{home, config} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	return config
}

func assertPathAbsentForTest(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected path %s: %v", path, err)
	}
}

const journalDirectoryV1ForTest = ".analytix-electron-task-retirement-v1"

func readAllTreeForTest(t *testing.T, root string) []byte {
	t.Helper()
	var result []byte
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result = append(result, body...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func startupWholeTreeRecordMapForTest(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	records := map[string]string{}
	for rootIndex, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("%d:%s", rootIndex, filepath.ToSlash(relative))
			value := fmt.Sprintf("%d:%d", uint32(info.Mode()), info.Size())
			if info.Mode().IsRegular() {
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				value += fmt.Sprintf(":%x", sha256.Sum256(body))
			}
			records[key] = value
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return records
}

func changedWholeTreeRecordKeysForTest(before, after map[string]string) []string {
	changed := []string{}
	for key, value := range before {
		if after[key] != value {
			changed = append(changed, key)
		}
	}
	for key := range after {
		if _, exists := before[key]; !exists {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return changed
}
