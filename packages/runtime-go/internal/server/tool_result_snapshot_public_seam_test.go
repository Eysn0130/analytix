package server

import (
	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	snapshotstore "analytix.local/runtime-go/internal/adapters/outbound/toolresultsnapshot"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	jobs "analytix.local/runtime-go/internal/jobs"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	provider "analytix.local/runtime-go/internal/provider"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Actual host grant, terminal/file adapter, private CAS, atomic primary store,
// and typed HTTP boundary. No model, live credentials or user state participates.
func protectedSeamFixtureV1(t *testing.T, tool string, arguments map[string]any, caseBound ...bool) (*runtimeServerHandler, runtimePendingToolCall, string, string) {
	t.Helper()
	durable := t.TempDir()
	h := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: durable, DataDir: t.TempDir(), ProviderID: "synthetic", BaseURL: "http://127.0.0.1:18997/v1", APIKey: "test-placeholder", EndpointFormat: "chat_completions", Model: "synthetic"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, h)
	root := filepath.Join(t.TempDir(), "tool-result-snapshots-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := snapshotstore.NewStore(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	h.toolSnapshots = store
	t.Cleanup(func() { store.Close() })
	workspace := workspacetest.New(t)
	if len(caseBound) > 0 && caseBound[0] {
		workspace = writeThreadMutationCaseBinding(t)
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := newServerTestCaseThreadStore(t, filepath.Join(t.TempDir(), "case-contexts"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := casethreadapp.NewRegistry(context.Background(), key, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	h.caseThreads = registry
	h.store.SetCaseThreadAuthority(registry)
	// Real signed-prefix admission for this fixture's ordinary-only history.
	registry.SetActiveInheritedHistoryIdentityValidatorV1(func(ctx context.Context, id string, turnIDs []string) error {
		wanted := map[string]bool{}
		for _, turnID := range turnIDs {
			wanted[turnID] = true
		}
		inventory, err := h.pendingWork.TrustedInventoryV1(ctx)
		if err != nil {
			return err
		}
		for _, receipt := range inventory.Receipts {
			if receipt.Context.ThreadID == id && wanted[receipt.Context.TurnID] {
				return snapshotport.ErrUnavailable
			}
			if receipt.ChildProducer != nil {
				for _, child := range receipt.ChildProducer.Children {
					if child.ChildThreadID == id && wanted[child.ChildTurnID] {
						return snapshotport.ErrUnavailable
					}
				}
			}
		}
		runs, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(h.dataDir, "child-runs"))
		if err != nil || runs.HasLegacyTypeScript {
			return snapshotport.ErrUnavailable
		}
		for _, run := range runs.Records {
			if (run.ParentThreadID == id && (wanted[run.ParentTurnID] || wanted[run.AutoContinueTurnID])) || (run.ChildThreadID == id && wanted[run.ChildTurnID]) {
				return snapshotport.ErrUnavailable
			}
		}
		return ctx.Err()
	})
	h.store.SetActiveHistorySourceAdmissionV1(func(source map[string]any) error {
		current, found, err := turnsecurityapp.LatestContext(source)
		if err != nil || !found {
			return snapshotport.ErrUnavailable
		}
		_, err = protectedSeamCurrentOriginV1(h)(context.Background(), source, current)
		return err
	})
	h.toolSnapshotHistoryValidate = func(ctx context.Context, thread map[string]any, original domainsecurity.TurnSecurityContext) error {
		return snapshotapp.ValidateHistoricalScopeV1(ctx, thread, original, snapshotapp.HistoricalScopeDependenciesV1{Primary: h.store, ValidateCurrentOrigin: protectedSeamCurrentOriginV1(h),
			ObserveCaseContexts: func(ctx context.Context) (*casethreadapp.VerifiedCommittedContextInventoryV1, error) {
				records, err := caseStore.List(ctx)
				if err != nil {
					return nil, err
				}
				return casethreadapp.VerifyCommittedContextInventoryV1(ctx, records, key)
			},
			ObserveChildren: func(ctx context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error) {
				inventory, err := h.pendingWork.TrustedInventoryV1(ctx)
				if err != nil {
					return inventory, nil, false, err
				}
				runs, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(h.dataDir, "child-runs"))
				return inventory, runs.Records, runs.HasLegacyTypeScript, err
			},
		})
	}
	thread, err := h.store.CreateThread(map[string]any{"title": "synthetic result", "workspace": workspace, "providerId": "synthetic", "model": "synthetic"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	pending := protectedSeamPendingV1(t, h, thread, tool, arguments, "turn_synthetic_result")
	return h, pending, durable, root
}

func protectedSeamPendingV1(t *testing.T, h *runtimeServerHandler, thread map[string]any, tool string, arguments map[string]any, turnID string) runtimePendingToolCall {
	t.Helper()
	threadID := stringField(thread, "id")
	workspace := stringField(thread, "workspace")
	now := time.Now().UTC().Truncate(time.Second)
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(), Authority: h.turnSecurity, Thread: thread, ThreadID: threadID, TurnID: turnID, Workspace: workspace, Principal: testIdentityPrincipal(), IssuedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(arguments)
	call := provider.ToolCall{ID: serverTestHostToolCallID("synthetic_" + tool), Name: tool, Arguments: raw}
	prompt := "synthetic ordinary tool result"
	if tool == "task" {
		prompt = "delegate one child agent to inspect synthetic code"
	}
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, nil, false, false, prompt, false, nil)
	scope := []string{}
	for _, schema := range schemas {
		scope = append(scope, schema.Name)
	}
	grant, err := executiongrantapp.IssueProvider(frozen, "synthetic", call, schemas, scope, tool == "read" || tool == "read_file", "not_required", 0, "", now)
	if err != nil {
		t.Fatal(err)
	}
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	epoch, err := contextepochapp.BootstrapState(threadID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, now)
	if err != nil {
		t.Fatal(err)
	}
	callItemID := domaintoolcall.ToolCallItemIDV1(turnID, call.ID)
	frozenRecord := turnsecurityapp.PublicRecord(frozen)
	if err := casethreadapp.RegisterRequired(context.Background(), h.caseThreads, frozen); err != nil {
		t.Fatal(err)
	}
	err = h.store.AppendTurnToThread(threadID, map[string]any{"id": turnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano), "securityContext": frozenRecord,
		"items": []any{map[string]any{"id": callItemID, "threadId": threadID, "turnId": turnID, "kind": "tool_call", "toolName": tool, "callId": call.ID, "status": "running", "createdAt": now.Format(time.RFC3339Nano), "arguments": arguments, "contextDigest": frozen.ContextDigest, "contextEpoch": float64(frozen.ContextEpoch), "executionGrantId": grant.GrantID, "executionGrant": grantRecord}}}, "synthetic", map[string]any{"securityState": frozenRecord, "contextEpochState": contextepochapp.PublicState(epoch)})
	if err != nil {
		t.Fatal(err)
	}
	if err := casethreadapp.CommitRequired(context.Background(), h.caseThreads, frozen, epoch, now); err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{ThreadID: threadID, TurnID: turnID, ProviderID: "synthetic", Model: "synthetic", Workspace: workspace, Prompt: prompt, Call: call, ToolCallItemID: callItemID, ToolScope: scope, SandboxMode: "workspace-write", ApprovalPolicy: "never", SecurityContext: frozen, ExecutionGrant: grant}
	if err := h.authorizeRuntimePending(context.Background(), pending, "", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if tool == "bash" {
		pending.SandboxMode = "danger-full-access"
	}
	if tool == "task" {
		pending.ApprovalPolicy = "auto"
	}
	return pending
}
func readProtectedSeamV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall) (int, snapshotapp.DisplayV1, string) {
	t.Helper()
	selector := snapshotport.SelectorV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, CallID: pending.Call.ID, ResultItemID: domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID)}
	body, _ := json.Marshal(selector)
	r := httptest.NewRequest(http.MethodPost, httpapi.ToolResultLocalDisplayPathV1, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, ToolResults: h.ToolResultLocalDisplayV1()}.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response permits caching")
	}
	var display snapshotapp.DisplayV1
	if w.Code == http.StatusOK && json.Unmarshal(w.Body.Bytes(), &display) != nil {
		t.Fatal("typed result is malformed")
	}
	return w.Code, display, w.Body.String()
}
func assertPublicToolSeamV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall, body string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/v1/threads/"+pending.ThreadID, nil)
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("public thread unavailable: status=%d", w.Code)
	}
	replay, err := h.store.LoadEventsSince(pending.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventBody, _ := json.Marshal(replay.Events)
	for _, public := range []string{w.Body.String(), string(eventBody)} {
		for _, forbidden := range []string{domaintoolresult.ProtectedSnapshotBindingFieldV1, "snapshotDigest", "bodyDigest", "installationIdHash", body} {
			if forbidden != "" && strings.Contains(public, forbidden) {
				t.Fatal("private result entered public snapshot/replay")
			}
		}
	}
	if len(replay.Events) == 0 {
		t.Fatal("actual settlement did not publish public metadata")
	}
}
func TestProtectedToolResultActualReadPublicSeamAndRestart(t *testing.T) {
	for _, original := range []string{"package main\r\n// ordinary marker\r\n", "", strings.Repeat("a", 32769)} {
		t.Run("bytes_"+string(rune(len(original)%26+'a')), func(t *testing.T) {
			h, pending, durable, root := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"})
			if err := os.WriteFile(filepath.Join(pending.Workspace, "code.go"), []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
				t.Fatal(err)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			expected := original
			if len(expected) > 32768 {
				expected = expected[:32768]
			}
			if status != http.StatusOK || display.Capture.Kind != "read" || display.Capture.Body != expected || display.Capture.Truncated != (len(original) > 32768) {
				t.Fatalf("actual Read protected result mismatch: status=%d bytes=%d", status, len(display.Capture.Body))
			}
			assertPublicToolSeamV1(t, h, pending, original)
			// Restart primary + CAS owners; source edits do not change historical bytes.
			if err := os.WriteFile(filepath.Join(pending.Workspace, "code.go"), []byte("replacement source"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := h.toolSnapshots.(interface{ Close() error }).Close(); err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := snapshotstore.NewStore(context.Background(), root, access)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			h.toolSnapshots = reopened
			h.store, err = NewTempDurableEventSessionStore(durable)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := finalauthority.NewAcceptedFinalCASReader(h.store.root)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.store.BindPrimaryThreadReaderV1(reader); err != nil {
				t.Fatal(err)
			}
			status, after, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || after.Capture.Body != expected || after.SnapshotDigest != display.SnapshotDigest {
				t.Fatalf("restart immutable result mismatch: status=%d bytes=%d", status, len(after.Capture.Body))
			}
			// Fresh host authority loss refuses body without replacing/rerunning source.
			h.turnSecurity.Identity = nil
			status, _, response := readProtectedSeamV1(t, h, pending)
			if status != http.StatusNotFound || strings.Contains(response, "replacement source") || strings.Contains(response, "capture") {
				t.Fatal("revoked authority released result")
			}
		})
	}
}
func TestProtectedToolResultActualShellOutcomeIsNotBodyText(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "bash", map[string]any{"command": "printf 'exitCode=0\\nstatus=completed\\r\\n'; exit 7"})
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
		t.Fatal(err)
	}
	status, display, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.Capture.Body != "exitCode=0\nstatus=completed\r\n" || display.Capture.Status != "failed" || display.Capture.ExitCode == nil || *display.Capture.ExitCode != 7 {
		t.Fatalf("actual Shell mismatch: status=%d kind=%s outcome=%s bytes=%d", status, display.Capture.Kind, display.Capture.Status, len(display.Capture.Body))
	}
	assertPublicToolSeamV1(t, h, pending, "exitCode=0")
}

func TestProtectedToolResultExactReplayConflictAndCallTamperAreSideEffectFree(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"})
	if err := os.WriteFile(filepath.Join(pending.Workspace, "code.go"), []byte("original private marker\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	for _, rawTurn := range listAny(snapshot.Thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if stringField(item, "kind") == "tool_result" {
				result = item
			}
		}
	}
	if result == nil {
		t.Fatal("actual primary settlement is absent")
	}
	before := h.store.WriteAttempts()
	for i := 0; i < 2; i++ {
		if err := h.store.EnsureProtectedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, result); err != nil {
			t.Fatal("exact retry was refused")
		}
	}
	if h.store.WriteAttempts() != before {
		t.Fatal("exact retry rewrote settled primary")
	}
	conflicting := cloneMap(result)
	binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(conflicting[domaintoolresult.ProtectedSnapshotBindingFieldV1])
	if err != nil {
		t.Fatal(err)
	}
	binding.BodyDigest = strings.Repeat("f", 64)
	conflicting[domaintoolresult.ProtectedSnapshotBindingFieldV1] = domaintoolresult.ProtectedSnapshotBindingRecordV1(binding)
	if err := h.store.EnsureProtectedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, conflicting); err == nil {
		t.Fatal("same result ID admitted different immutable bytes")
	}
	if h.store.WriteAttempts() != before {
		t.Fatal("conflicting retry produced a write")
	}
	if status, display, _ := readProtectedSeamV1(t, h, pending); status != 200 || display.Capture.Body != "original private marker\r\n" {
		t.Fatal("retry altered original body")
	}
	if err := h.store.PatchTurnItemStatus(pending.ThreadID, pending.TurnID, pending.ToolCallItemID, "failed"); err != nil {
		t.Fatal(err)
	}
	if status, _, response := readProtectedSeamV1(t, h, pending); status != 404 || strings.Contains(response, "capture") {
		t.Fatal("mismatched durable call/result released body")
	}
}

func TestProtectedToolResultCaseOrdinaryCodeVisibleUntilCurrentCaseBindingChanges(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"}, true)
	if pending.SecurityContext.CaseID == "" {
		t.Fatal("case fixture was not actually case bound")
	}
	const body = "package main\r\n// synthetic private account 6222020202020202020\r\n"
	if err := os.WriteFile(filepath.Join(pending.Workspace, "code.go"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
		t.Fatal(err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, pending); status != 200 || display.Capture.Body != body {
		t.Fatalf("ordinary code in actual case workspace was overfiltered: status=%d", status)
	}
	assertPublicToolSeamV1(t, h, pending, "6222020202020202020")
	path := filepath.Join(pending.Workspace, ".analytix", "case-project.json")
	binding, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(binding, []byte("case_writer_barrier"), []byte("case_successor"), 1)
	if bytes.Equal(changed, binding) {
		t.Fatal("case fixture did not change actual binding")
	}
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if status, _, response := readProtectedSeamV1(t, h, pending); status != 404 || strings.Contains(response, "capture") {
		t.Fatal("successor case authority released historical private body")
	}
}

func TestProtectedToolResultActualShellEmptyAndTimeout(t *testing.T) {
	for _, test := range []struct {
		name, command, body, status string
		timeout                     int
	}{
		{"empty", ":", "", "completed", 5},
		{"timeout", "printf 'partial\\r\\n'; sleep 3", "partial\r\n", "timeout", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, pending, _, _ := protectedSeamFixtureV1(t, "bash", map[string]any{"command": test.command, "timeout": test.timeout})
			if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
				t.Fatal(err)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != 200 || display.Capture.Body != test.body || display.Capture.Status != test.status {
				t.Fatalf("actual Shell empty/timeout mismatch: http=%d status=%s bytes=%d", status, display.Capture.Status, len(display.Capture.Body))
			}
		})
	}
}

// The ordinary fork is made by the real durable fork owner; its inherited
// records cannot provide the new side turn's frozen/grant/snapshot authority.
func TestProtectedToolResultOrdinarySideOwnResultAndInheritedRefusal(t *testing.T) {
	h, parent, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"})
	if err := os.WriteFile(filepath.Join(parent.Workspace, "code.go"), []byte("parent ordinary\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil {
		t.Fatal(err)
	}
	side, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "side"})
	if err != nil {
		t.Fatal(err)
	}
	inherited := parent
	inherited.ThreadID = stringField(side, "id")
	if status, display, _ := readProtectedSeamV1(t, h, inherited); status != http.StatusNotFound || display.Capture.Body != "" {
		t.Fatal("inherited parent result gained private reading authority")
	}
	own := protectedSeamPendingV1(t, h, side, "read", map[string]any{"path": "code.go"}, "turn_ordinary_side")
	if err := os.WriteFile(filepath.Join(own.Workspace, "code.go"), []byte("side own\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), own, nil, nil); err != nil {
		t.Fatal(err)
	}
	verified, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), own.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotapp.CurrentOwnContextV1(verified.Thread, own.SecurityContext); err != nil {
		t.Fatalf("side own current frozen mismatch: %v", err)
	}
	if _, err := protectedSeamCurrentOriginV1(h)(context.Background(), verified.Thread, own.SecurityContext); err != nil {
		t.Fatalf("side current origin rejected: %v", err)
	}
	if _, err := h.pendingWork.TrustedInventoryV1(context.Background()); err != nil {
		t.Fatalf("side trusted pending observation: %v", err)
	}
	if _, err := jobs.ReadChildRunIdentitySnapshotV1(context.Background(), filepath.Join(h.dataDir, "child-runs")); err != nil {
		t.Fatalf("side child identity observation: %v", err)
	}
	if err := h.toolSnapshotHistoryValidate(context.Background(), verified.Thread, own.SecurityContext); err != nil {
		t.Fatalf("side historical origin rejected: %v", err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, own); status != http.StatusOK || display.Capture.Body != "side own\r\n" {
		t.Fatalf("own ordinary side result unavailable: status=%d bytes=%d", status, len(display.Capture.Body))
	}
	if status, display, _ := readProtectedSeamV1(t, h, parent); status != http.StatusOK || display.Capture.Body != "parent ordinary\r\n" {
		t.Fatal("side own result replaced parent immutable bytes")
	}
	assertPublicToolSeamV1(t, h, own, "side own\r\n")
}

func protectedSeamCurrentOriginV1(h *runtimeServerHandler) func(context.Context, map[string]any, domainsecurity.TurnSecurityContext) (domainsecurity.TurnSecurityContext, error) {
	return func(ctx context.Context, node map[string]any, original domainsecurity.TurnSecurityContext) (domainsecurity.TurnSecurityContext, error) {
		current, err := snapshotapp.CurrentOwnContextV1(node, original)
		if err != nil {
			return domainsecurity.TurnSecurityContext{}, err
		}
		if err := turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: current, Workspace: stringField(node, "workspace")}); err != nil {
			return domainsecurity.TurnSecurityContext{}, err
		}
		if h.caseThreads != nil {
			if h.caseThreads.IsCaseThread(current.ThreadID) {
				exact, err := threadapp.NewCurrentCaseThreadAuthorityValidator(h.caseThreads, filestore.CaseBindingReader{}, nil).ValidateCurrent(current.ThreadID, node)
				if err != nil || exact != current {
					return domainsecurity.TurnSecurityContext{}, snapshotport.ErrUnavailable
				}
			}
			reader, ok := h.caseThreads.(casethreadapp.ActiveInheritedHistoryReaderV1)
			if !ok {
				return domainsecurity.TurnSecurityContext{}, snapshotport.ErrUnavailable
			}
			inherited, err := threadapp.ValidateActiveInheritedHistoryV1(ctx, node, reader, h.store)
			if err != nil || inherited[original.TurnID] {
				return domainsecurity.TurnSecurityContext{}, snapshotport.ErrUnavailable
			}
		}
		return current, nil
	}
}

func protectedSeamResultV1(t *testing.T, thread map[string]any, pending runtimePendingToolCall) map[string]any {
	t.Helper()
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != pending.TurnID {
			continue
		}
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == pending.Call.ID {
				return item
			}
		}
	}
	t.Fatal("actual settled result absent")
	return nil
}

func TestProtectedToolResultForkForgedParentBindingAndCurrentOriginRefused(t *testing.T) {
	h, parent, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"})
	if err := os.WriteFile(filepath.Join(parent.Workspace, "code.go"), []byte("parent-only bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil {
		t.Fatal(err)
	}
	fork, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "fork"})
	if err != nil {
		t.Fatal(err)
	}
	own := protectedSeamPendingV1(t, h, fork, "read", map[string]any{"path": "code.go"}, "turn_fork_own")
	if err := os.WriteFile(filepath.Join(own.Workspace, "code.go"), []byte("fork-only bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), own, nil, nil); err != nil {
		t.Fatal(err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, own); status != 200 || display.Capture.Body != "fork-only bytes\n" {
		t.Fatal("real ordinary fork own result was overfiltered")
	}
	parentSnapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), parent.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	ownSnapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), own.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"parent_private_binding", "latest_without_own_frozen_turn", "workspace_drift"} {
		t.Run(mode, func(t *testing.T) {
			altered := cloneMap(ownSnapshot.Thread)
			switch mode {
			case "parent_private_binding":
				// Keep the legitimate side call/result IDs and grant; only borrow the
				// parent's immutable CAS binding, as a copied/renamed result would do.
				protectedSeamResultV1(t, altered, own)[domaintoolresult.ProtectedSnapshotBindingFieldV1] = protectedSeamResultV1(t, parentSnapshot.Thread, parent)[domaintoolresult.ProtectedSnapshotBindingFieldV1]
			case "latest_without_own_frozen_turn":
				future, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(), Authority: h.turnSecurity, Thread: ownSnapshot.Thread, ThreadID: own.ThreadID, TurnID: "turn_not_committed", Workspace: own.Workspace, Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC()})
				if err != nil {
					t.Fatal(err)
				}
				altered["securityState"] = turnsecurityapp.PublicRecord(future)
			case "workspace_drift":
				altered["workspace"] = workspacetest.New(t)
			}
			if err := h.store.ReplaceThreadForAuthorityRepair(own.ThreadID, altered); err != nil {
				t.Fatal(err)
			}
			before := h.store.WriteAttempts()
			if status, display, response := readProtectedSeamV1(t, h, own); status != 404 || display.Capture.Body != "" || strings.Contains(response, "capture") {
				t.Fatal("forged origin released private body")
			}
			if h.store.WriteAttempts() != before {
				t.Fatal("refused read repaired or wrote primary")
			}
			if err := h.store.ReplaceThreadForAuthorityRepair(own.ThreadID, cloneMap(ownSnapshot.Thread)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProtectedToolResultCaseForkOwnAndSignedInheritedOrigin(t *testing.T) {
	h, parent, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"}, true)
	if err := os.WriteFile(filepath.Join(parent.Workspace, "code.go"), []byte("case-parent ordinary code\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil {
		t.Fatal(err)
	}
	fork, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "fork"})
	if err != nil {
		t.Fatal(err)
	}
	if stringField(fork, "activeInheritedHistoryReceipt") == "" {
		t.Fatal("case fork lacks actual signed prefix receipt")
	}
	own := protectedSeamPendingV1(t, h, fork, "read", map[string]any{"path": "code.go"}, "turn_case_fork_own")
	if err := os.WriteFile(filepath.Join(own.Workspace, "code.go"), []byte("case-fork own code\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), own, nil, nil); err != nil {
		t.Fatal(err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, own); status != 200 || display.Capture.Body != "case-fork own code\r\n" {
		t.Fatalf("signed case fork own result unavailable: http=%d", status)
	}
	inherited := parent
	inherited.ThreadID = own.ThreadID
	if status, display, _ := readProtectedSeamV1(t, h, inherited); status != 404 || display.Capture.Body != "" {
		t.Fatal("signed inherited history granted parent private result")
	}
	exact, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), own.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	altered := cloneMap(exact.Thread)
	altered["parentThreadId"] = "thr_durable_999"
	if err := h.store.ReplaceThreadForAuthorityRepair(own.ThreadID, altered); err != nil {
		t.Fatal(err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, own); status != 404 || display.Capture.Body != "" {
		t.Fatal("case fork signed parent origin drift released body")
	}
}

func TestProtectedToolResultSignedChildOfOrdinarySideAndIncompleteChainRefusal(t *testing.T) {
	h, parent, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"})
	side, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "side"})
	if err != nil {
		t.Fatal(err)
	}
	h.subagents = subagentapp.ProfileSettings{Enabled: true, DefaultToolPolicy: "readOnly", MaxParallel: 2, MaxChildRuns: 8, Profiles: map[string]subagentapp.ProfileConfig{"reviewer": {Name: "reviewer", ToolPolicy: "readOnly", MaxSteps: 4, MaxStepsSet: true}}}
	task := protectedSeamPendingV1(t, h, side, "task", map[string]any{"prompt": "inspect synthetic only", "profile": "reviewer", "run_in_background": false}, "turn_side_task")
	// Existing synthetic provider supplies only a completed text response. Real
	// task execution, receipt allocation, job, and first frozen turn run in Core.
	recorder := &providerStepRecordingProvider{}
	h.provider = recorder
	if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), task.SecurityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), task, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if settled.IsError {
		out, _ := settled.Output.(map[string]any)
		t.Fatalf("synthetic task failed: projection=%s code=%s status=%s", stringField(out, "projectionKind"), stringField(out, "code"), stringField(out, "status"))
	}
	inventory, err := h.pendingWork.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var childID, firstTurnID string
	for _, receipt := range inventory.Receipts {
		if receipt.ChildProducer != nil {
			for _, target := range receipt.ChildProducer.Children {
				childID, firstTurnID = target.ChildThreadID, target.ChildTurnID
			}
		}
	}
	if childID == "" || len(recorder.Requests()) != 1 {
		t.Fatal("real task did not create and execute exactly one signed child")
	}
	child, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), childID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := snapshotapp.CurrentOwnContextV1(child.Thread, mustProtectedFrozenV1(t, child.Thread, firstTurnID))
	if err != nil || first.ThreadID != childID {
		t.Fatal("real child first frozen origin absent")
	}
	own := protectedSeamPendingV1(t, h, child.Thread, "read", map[string]any{"path": "code.go"}, "turn_child_read")
	if err := os.WriteFile(filepath.Join(own.Workspace, "code.go"), []byte("signed-child own bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), own, nil, nil); err != nil {
		t.Fatal(err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, own); status != 200 || display.Capture.Body != "signed-child own bytes\n" {
		t.Fatalf("signed child with ordinary side parent unavailable: http=%d", status)
	}
	originalHistory := h.toolSnapshotHistoryValidate
	for _, mode := range []string{"missing_receipt", "missing_job", "wrong_parent_grant", "legacy_observation", "observation_error"} {
		t.Run(mode, func(t *testing.T) {
			h.toolSnapshotHistoryValidate = func(ctx context.Context, node map[string]any, origin domainsecurity.TurnSecurityContext) error {
				return snapshotapp.ValidateHistoricalScopeV1(ctx, node, origin, snapshotapp.HistoricalScopeDependenciesV1{Primary: h.store, ValidateCurrentOrigin: protectedSeamCurrentOriginV1(h), ObserveCaseContexts: func(context.Context) (*casethreadapp.VerifiedCommittedContextInventoryV1, error) {
					return nil, snapshotport.ErrUnavailable
				}, ObserveChildren: func(ctx context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error) {
					inv, err := h.pendingWork.TrustedInventoryV1(ctx)
					if err != nil {
						return inv, nil, false, err
					}
					runs, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(h.dataDir, "child-runs"))
					if err != nil {
						return inv, nil, false, err
					}
					switch mode {
					case "missing_receipt":
						inv.Receipts = nil
					case "missing_job":
						runs.Records = nil
					case "wrong_parent_grant":
						if len(runs.Records) != 1 {
							t.Fatal("unexpected actual job inventory")
						}
						runs.Records[0].SecurityBinding.ParentExecutionGrantID = strings.Repeat("f", 64)
					case "legacy_observation":
						return inv, runs.Records, true, nil
					case "observation_error":
						return inv, runs.Records, false, snapshotport.ErrUnavailable
					}
					return inv, runs.Records, runs.HasLegacyTypeScript, nil
				}})
			}
			before := h.store.WriteAttempts()
			if status, display, response := readProtectedSeamV1(t, h, own); status != 404 || display.Capture.Body != "" || strings.Contains(response, "capture") {
				t.Fatal("incomplete child chain fell back to ordinary origin")
			}
			if before != h.store.WriteAttempts() {
				t.Fatal("refused child read wrote primary")
			}
			h.toolSnapshotHistoryValidate = originalHistory
		})
	}
	exact, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), childID)
	if err != nil {
		t.Fatal(err)
	}
	altered := cloneMap(exact.Thread)
	altered["relation"] = "primary"
	delete(altered, "parentThreadId")
	if err := h.store.ReplaceThreadForAuthorityRepair(childID, altered); err != nil {
		t.Fatal(err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, own); status != 404 || display.Capture.Body != "" {
		t.Fatal("signed child relabelled primary bypassed chain")
	}
}

func mustProtectedFrozenV1(t *testing.T, thread map[string]any, turnID string) domainsecurity.TurnSecurityContext {
	t.Helper()
	frozen, err := turnapp.FrozenSecurityContextForTurn(thread, turnID)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}
