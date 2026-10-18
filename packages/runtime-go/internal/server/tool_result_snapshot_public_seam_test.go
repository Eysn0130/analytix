package server

import (
	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	snapshotstore "analytix.local/runtime-go/internal/adapters/outbound/toolresultsnapshot"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
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
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
	if tool == "run_skill" {
		h.skills = syntheticSpecializedSkillCatalogV1(t, "Actual verified synthetic instruction.")
	}
	if tool == "web_fetch" {
		h.web.Enabled = true
		h.web.FetchEnabled = true
		h.web.MaxFetchBytes = runtimeinfoapp.DefaultWebMaxFetchBytes
	}
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
	if tool == "task" || tool == "delegate_task" || tool == "parallel_tasks" {
		prompt = "delegate child agents to inspect synthetic code"
	} else if tool == "run_skill" {
		prompt = "run skill synthetic-skill and inspect its child job"
	} else if tool == "bash" && (boolField(arguments, "run_in_background") || boolField(arguments, "runInBackground")) {
		prompt = "start a background job and inspect its output"
	} else if tool == "list_jobs" || tool == "bash_output" || tool == "kill_shell" || tool == "wait" {
		prompt = "inspect background job status using " + tool
	}
	goalTools := tool == "get_goal" || tool == "create_goal" || tool == "update_goal" || tool == "todo_write"
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, nil, false, false, prompt, goalTools, nil)
	scope := []string{}
	for _, schema := range schemas {
		scope = append(scope, schema.Name)
	}
	readOnly, _ := h.runtimeToolPolicy(tool)
	grant, err := executiongrantapp.IssueProvider(frozen, "synthetic", call, schemas, scope, readOnly, "not_required", 0, "", now)
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
		"items": []any{map[string]any{"id": callItemID, "threadId": threadID, "turnId": turnID, "kind": "tool_call", "toolKind": toolcatalogapp.ToolKind(tool), "toolName": tool, "callId": call.ID, "status": "running", "createdAt": now.Format(time.RFC3339Nano), "arguments": arguments, "contextDigest": frozen.ContextDigest, "contextEpoch": float64(frozen.ContextEpoch), "executionGrantId": grant.GrantID, "executionGrant": grantRecord}}}, "synthetic", map[string]any{"securityState": frozenRecord, "contextEpochState": contextepochapp.PublicState(epoch)})
	if err != nil {
		t.Fatal(err)
	}
	if err := casethreadapp.CommitRequired(context.Background(), h.caseThreads, frozen, epoch, now); err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{ThreadID: threadID, TurnID: turnID, ProviderID: "synthetic", Model: "synthetic", Workspace: workspace, Prompt: prompt, Call: call, ToolCallItemID: callItemID, ToolScope: scope, SandboxMode: "workspace-write", ApprovalPolicy: "never", SecurityContext: frozen, ExecutionGrant: grant}
	pending.ModelVisibleToolDefinition = domainmodel.FreezeModelVisibleToolDefinitionsV1(schemas)[tool]
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

func syntheticSpecializedSkillCatalogV1(t *testing.T, body string, runAs ...string) toolcatalogapp.SkillCatalog {
	t.Helper()
	snapshot, err := domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: []byte(body)}})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := toolcatalogapp.LoadSkillCatalog(toolcatalogapp.SkillCatalogLoadInput{
		Document: map[string]any{"skills": map[string]any{"enabled": true, "roots": []any{"/synthetic-skills"}}},
		Files: toolcatalogapp.SkillCatalogFileSource{
			NormalizeRoot: func(root string) string { return root }, RootExists: func(string) bool { return true },
			PackageCandidates: func(string) ([]string, error) { return []string{"/synthetic-skills/SKILL.md"}, nil },
			LoadPackage: func(string, string, string) (toolcatalogapp.LoadedSkillPackage, error) {
				record := map[string]any{"id": "synthetic-skill", "name": "Synthetic Skill", "entry": "SKILL.md", "packageDigest": snapshot.Digest()}
				if len(runAs) == 1 {
					record["runAs"], record["allowedTools"] = runAs[0], []string{"read"}
				}
				return toolcatalogapp.LoadedSkillPackage{Snapshot: snapshot,
					Record: record}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

// These assertions use only existing public fixture APIs and JSON identities,
// so the same test can falsify the missing producer on the exact base archive.
func TestSpecializedActualGoalTodoSkillProductionSeamV1(t *testing.T) {
	for _, tool := range []string{"get_goal", "create_goal", "update_goal", "todo_write", "run_skill"} {
		t.Run(tool, func(t *testing.T) {
			args := map[string]any{}
			switch tool {
			case "create_goal":
				args = map[string]any{"objective": "Actual goal", "token_budget": 100}
			case "update_goal":
				args = map[string]any{"status": "blocked", "reason": "Actual blocker"}
			case "todo_write":
				args = map[string]any{"todos": []any{
					map[string]any{"id": "todo-1", "content": "Actual after", "status": "in_progress"},
					map[string]any{"id": "todo-2", "content": "Parallel second", "status": "in_progress"},
					map[string]any{"id": "todo-3", "content": "Parallel third", "status": "in_progress"},
				}}
			case "run_skill":
				args = map[string]any{"name": "synthetic-skill"}
			}
			h, pending, durable, root := protectedSeamFixtureV1(t, tool, args)
			if tool == "get_goal" || tool == "update_goal" {
				if _, err := h.store.SetGoal(pending.ThreadID, map[string]any{"objective": "Actual goal", "tokenBudget": 100, "tokensUsed": 12}); err != nil {
					t.Fatal(err)
				}
			}
			if tool == "todo_write" {
				if _, err := h.store.SetTodos(pending.ThreadID, []any{
					map[string]any{"id": "todo-1", "content": "Actual before", "status": "in_progress"},
					map[string]any{"id": "todo-2", "content": "Parallel second", "status": "in_progress"},
				}); err != nil {
					t.Fatal(err)
				}
			}
			if result, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil || result.IsError {
				t.Fatal("actual production execution failed", err)
			}
			status, display, response := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK {
				t.Fatalf("actual observation unavailable: http=%d", status)
			}
			wire := map[string]any{}
			if json.Unmarshal([]byte(response), &wire) != nil {
				t.Fatal("malformed typed display")
			}
			capture := wire["capture"].(map[string]any)
			switch tool {
			case "get_goal", "create_goal", "update_goal":
				goal := capture["goal"].(map[string]any)
				record := goal["goal"].(map[string]any)
				if capture["kind"] != "goal" || record["threadId"] != pending.ThreadID || record["objective"] != "Actual goal" || goal["remainingTokens"] == nil {
					t.Fatal("actual returned goal missing")
				}
				if tool == "update_goal" && record["status"] != "active" {
					t.Fatal("requested blocked state substituted for actual first blocker observation")
				}
				patch := map[string]any{"objective": "Changed current goal"}
				if tool == "create_goal" {
					current, err := h.store.GetGoal(pending.ThreadID)
					if err != nil {
						t.Fatal(err)
					}
					patch["operation"] = "edit"
					patch["ref"] = map[string]any{"id": current["id"], "revision": current["revision"]}
				}
				if _, err := h.store.SetGoal(pending.ThreadID, patch); err != nil {
					t.Fatal(err)
				}
			case "todo_write":
				todo := capture["todo"].(map[string]any)
				baseline := todo["baseline"].(map[string]any)
				if baseline["availability"] != "known" {
					t.Fatal("actual same-mutation baseline unavailable")
				}
				beforeItems := baseline["todos"].(map[string]any)["items"].([]any)
				afterItems := todo["todos"].(map[string]any)["items"].([]any)
				if len(beforeItems) != 2 || len(afterItems) != 3 || beforeItems[0].(map[string]any)["content"] != "Actual before" || afterItems[0].(map[string]any)["content"] != "Actual after" {
					t.Fatal("actual same-mutation before/after missing")
				}
				for _, items := range [][]any{beforeItems, afterItems} {
					for index, raw := range items {
						item := raw.(map[string]any)
						if item["id"] != []string{"todo-1", "todo-2", "todo-3"}[index] || item["status"] != "in_progress" {
							t.Fatal("actual private HTTP capture lost parallel status or identity")
						}
					}
				}
				if _, err := h.store.SetTodos(pending.ThreadID, []any{map[string]any{"id": "todo-1", "content": "Changed current todo", "status": "in_progress"}}); err != nil {
					t.Fatal(err)
				}
			case "run_skill":
				skill := capture["skill"].(map[string]any)
				resolved := skill["resolved"].(map[string]any)
				if capture["body"] != "Actual verified synthetic instruction." || resolved["id"] != "synthetic-skill" || resolved["runAs"] != "inline" || skill["child"] != nil || skill["error"] != nil {
					t.Fatal("verified instruction/identity missing")
				}
				h.skills = syntheticSpecializedSkillCatalogV1(t, "Changed current package")
			}
			assertPublicToolSeamV1(t, h, pending, "")
			// Reopen both existing CAS and durable primary authorities; the old
			// call must retain its original observation after current state changes.
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
			if status != http.StatusOK || after.SnapshotDigest != display.SnapshotDigest || !reflect.DeepEqual(after.Capture, display.Capture) {
				t.Fatal("reopened original observation changed")
			}
		})
	}
}

func TestSpecializedActualJobProductionSeamV1(t *testing.T) {
	for _, tool := range []string{"list_jobs", "bash_output", "kill_shell", "wait"} {
		t.Run(tool, func(t *testing.T) {
			h, parent, durable, root := protectedSeamFixtureV1(t, "task", map[string]any{"prompt": "PRIVATE_JOB_PROMPT", "name": "PRIVATE_JOB_NAME", "label": "PRIVATE_JOB_LABEL"})
			h.provider = &providerStepRecordingProvider{}
			if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent.SecurityContext, time.Second); err != nil {
				t.Fatal(err)
			}
			if settled, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil || settled.IsError {
				t.Fatal("real parent task did not produce the signed child association", err)
			}
			records, err := h.jobs.List(parent.ThreadID)
			if err != nil || len(records) != 1 {
				t.Fatal("real task did not produce exactly one native Job", err)
			}
			record := records[0]
			expectedOutput, _, err := subagentapp.TurnCompletionFromThreadStoreV1(h.store.GetThread, record.ChildThreadID, record.ChildTurnID)
			if err != nil || expectedOutput == "" || record.Output != "" || record.ProtectedOutputSnapshot == nil {
				t.Fatal("real child did not retain its exact completion through private CAS", err)
			}
			if tool == "bash_output" {
				assertProtectedJobSourceIdentityV1(t, h, parent, record)
				if _, err := h.jobs.UpdateChildRun(record.ID, jobs.UpdateRequest{Output: "PRIVATE_JOB_OUTPUT"}); err == nil {
					t.Fatal("native bound child output persistence guard was bypassed")
				}
			}
			args := map[string]any{"job_id": record.ID}
			if tool == "list_jobs" {
				args = map[string]any{}
			} else if tool == "wait" {
				args = map[string]any{"job_ids": []any{record.ID}}
			}
			// Job queries retain the original parent turn's security authority.
			// A new turn would revoke that job before the query could observe it.
			pending := parent
			raw, _ := json.Marshal(args)
			pending.Call = provider.ToolCall{ID: serverTestHostToolCallID("synthetic_query_" + tool), Name: tool, Arguments: raw}
			schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, parent.ToolScope, false, false, parent.Prompt, false, nil)
			pending.ModelVisibleToolDefinition = domainmodel.FreezeModelVisibleToolDefinitionsV1(schemas)[tool]
			readOnly, _ := h.runtimeToolPolicy(tool)
			pending.ExecutionGrant, err = executiongrantapp.IssueProvider(parent.SecurityContext, "synthetic", pending.Call, schemas, parent.ToolScope, readOnly, "not_required", 0, "", time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			pending.ToolCallItemID, err = h.persistToolCallReady(context.Background(), parent.ThreadID, parent.TurnID, pending.Call, 1, parent.SecurityContext, pending.ExecutionGrant)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.authorizeRuntimePending(context.Background(), pending, "", time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			if result, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil || result.IsError {
				t.Fatal("actual job query failed", err)
			}
			localStatus, originalDisplay, _ := readProtectedSeamV1(t, h, pending)
			if localStatus != http.StatusOK || originalDisplay.Capture.Kind != "job" || originalDisplay.Capture.Job == nil || len(originalDisplay.Capture.Job.Jobs) != 1 {
				t.Fatalf("actual Job record did not cross protected CAS/local display: http=%d kind=%s jobPresent=%t", localStatus, originalDisplay.Capture.Kind, originalDisplay.Capture.Job != nil)
			}
			observed := originalDisplay.Capture.Job.Jobs[0]
			if observed.Name == nil || *observed.Name != record.Name || observed.Label == nil || *observed.Label != record.Label || observed.Status != domainjob.PublicStatusV1(record.Status) {
				t.Fatal("Job display replaced actual producer facts")
			}
			if tool == "bash_output" {
				if observed.Output.Availability != "available" || observed.Output.Body == nil || *observed.Output.Body != expectedOutput || observed.Output.Truncated == nil || *observed.Output.Truncated {
					t.Fatal("native signed child completion did not reach protected Job output")
				}
				assertPublicToolSeamV1(t, h, pending, expectedOutput)
			} else if observed.Output.Availability != "unavailable" || observed.Output.Body != nil {
				t.Fatal("non-output operation supplied a body")
			}
			if tool == "kill_shell" && (originalDisplay.Capture.Job.Receipt == nil || originalDisplay.Capture.Job.Receipt.Status != "already-finished") {
				t.Fatal("finished job was presented as a new cancellation request")
			}
			if tool == "wait" && (originalDisplay.Capture.Job.Receipt == nil || originalDisplay.Capture.Job.Receipt.Operation != "wait" || originalDisplay.Capture.Job.Receipt.Status != "completed") {
				t.Fatal("actual completed wait receipt was lost at the private HTTP boundary")
			}
			operations := map[string]string{"list_jobs": "list", "bash_output": "output", "kill_shell": "kill", "wait": "wait"}
			readPublishedJob := func() map[string]any {
				t.Helper()
				r := httptest.NewRequest(http.MethodGet, "/v1/threads/"+pending.ThreadID, nil)
				r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				thread := map[string]any{}
				if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &thread) != nil {
					t.Fatalf("persisted job public thread unavailable: status=%d", w.Code)
				}
				calls, results := 0, 0
				var item map[string]any
				for _, rawTurn := range listAny(thread["turns"]) {
					turn, _ := rawTurn.(map[string]any)
					if stringField(turn, "id") != pending.TurnID {
						continue
					}
					for _, raw := range listAny(turn["items"]) {
						candidate, _ := raw.(map[string]any)
						if stringField(candidate, "callId") != pending.Call.ID {
							continue
						}
						if stringField(candidate, "kind") == "tool_call" {
							calls++
							if stringField(candidate, "id") != pending.ToolCallItemID || stringField(candidate, "toolName") != tool {
								t.Fatal("reopened query lost its exact admitted call")
							}
						} else if stringField(candidate, "kind") == "tool_result" {
							results++
							item = candidate
						}
					}
				}
				if calls != 1 || results != 1 || stringField(item, "id") != domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID) ||
					stringField(item, "threadId") != pending.ThreadID || stringField(item, "turnId") != pending.TurnID ||
					stringField(item, "toolName") != tool || stringField(item, "toolKind") != "command_execution" || item["status"] != "completed" || item["isError"] != false {
					t.Fatal("persisted job result lost its unique original call/result pair")
				}
				output, _ := item["output"].(map[string]any)
				payload, _ := output["taskJob"].(map[string]any)
				if output["projectionKind"] != "task_job_result" || payload["operation"] != operations[tool] ||
					output["privatePayloadWithheld"] != true || output["factAnswerAllowed"] != false || output["evidenceAuthority"] != false {
					t.Fatal("real query lost typed withheld projection")
				}
				body, _ := json.Marshal(item)
				if strings.Contains(string(body), "PRIVATE_JOB_") || strings.Contains(string(body), "\"active\"") {
					t.Fatal("job private data or transient activity entered history")
				}
				return item
			}
			before := readPublishedJob()
			// CAS owns live descriptors and is really closed. The primary store
			// has no Close API: its I/O closes each file; discard its old owner and
			// cached thread service before reconstructing from the same disk root.
			if err := h.toolSnapshots.(interface{ Close() error }).Close(); err != nil {
				t.Fatal(err)
			}
			h.toolSnapshots, h.store, h.threads = nil, nil, nil
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
			h.store.SetCaseThreadAuthority(h.caseThreads)
			reader, err := finalauthority.NewAcceptedFinalCASReader(h.store.root)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.store.BindPrimaryThreadReaderV1(reader); err != nil {
				t.Fatal(err)
			}
			if after := readPublishedJob(); !reflect.DeepEqual(before, after) {
				t.Fatal("reopened primary changed the recorded typed Job result")
			}
			if status, after, _ := readProtectedSeamV1(t, h, pending); status != http.StatusOK || after.SnapshotDigest != originalDisplay.SnapshotDigest || !reflect.DeepEqual(after.Capture, originalDisplay.Capture) {
				t.Fatal("restart lost immutable Job observation")
			}
			// Raw completion still never enters the durable Job's ordinary Output.
			assertPublicToolSeamV1(t, h, pending, "PRIVATE_JOB_")
		})
	}
}

func TestSpecializedActualSubagentSkillProductionSeamV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "run_skill", map[string]any{"name": "synthetic-skill", "arguments": "inspect synthetic code"})
	h.skills = syntheticSpecializedSkillCatalogV1(t, "Actual verified subagent instruction.", "subagent")
	pending.ApprovalPolicy = "auto"
	providerClient := &providerStepRecordingProvider{}
	h.provider = providerClient
	if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), pending.SecurityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if settled.IsError {
		output, _ := settled.Output.(map[string]any)
		t.Fatalf("synthetic skill child failed: projection=%s code=%s status=%s", stringField(output, "projectionKind"), stringField(output, "code"), stringField(output, "status"))
	}
	status, display, response := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK {
		t.Fatalf("real subagent Skill observation unavailable: http=%d", status)
	}
	wire := map[string]any{}
	if json.Unmarshal([]byte(response), &wire) != nil {
		t.Fatal("malformed display")
	}
	capture := wire["capture"].(map[string]any)
	skill := capture["skill"].(map[string]any)
	child, ok := skill["child"].(map[string]any)
	if !ok || capture["body"] != "" || display.Capture.Truncated || skill["resolved"].(map[string]any)["runAs"] != "subagent" || child["outputWithheld"] != true {
		t.Fatal("actual child association was replaced by inline/result text")
	}
	records, err := h.jobs.List(pending.ThreadID)
	if err != nil || len(records) != 1 || child["jobId"] != records[0].ID || child["childThreadId"] != records[0].ChildThreadID || child["childTurnId"] != records[0].ChildTurnID || records[0].SecurityBinding.ParentToolCallID != pending.Call.ID {
		t.Fatal("capture not associated with real admitted child", err)
	}
	if len(providerClient.Requests()) != 1 {
		t.Fatalf("fixture did not exercise the real child production path with synthetic in-memory Provider: requests=%d status=%s error=%q", len(providerClient.Requests()), records[0].Status, records[0].Error)
	}
	if strings.Contains(response, "Actual verified subagent instruction.") || strings.Contains(response, "provider-step-candidate") {
		t.Fatal("subagent prompt/output leaked into parent display")
	}
	assertPublicToolSeamV1(t, h, pending, "Actual verified subagent instruction.")
	expected, _, err := subagentapp.TurnCompletionFromThreadStoreV1(h.store.GetThread, records[0].ChildThreadID, records[0].ChildTurnID)
	if err != nil {
		t.Fatal(err)
	}
	assertProtectedJobOutputQueryBindingV1(t, h, pending, records[0], expected, "provider-step-candidate-", "subagent-skill")
	if len(providerClient.Requests()) != 1 {
		t.Fatal("skill output query reexecuted its child")
	}
}

func TestProtectedJobSpawnActualProducerSettlementAndPrivateHTTPV1(t *testing.T) {
	for _, tool := range []string{"task", "delegate_task", "parallel_tasks"} {
		t.Run(tool, func(t *testing.T) {
			arguments := map[string]any{"prompt": "inspect synthetic code", "name": "PRIVATE_SPAWN_NAME", "label": "PRIVATE_SPAWN_LABEL"}
			wantCount := 1
			if tool == "parallel_tasks" {
				arguments = map[string]any{"tasks": []any{
					map[string]any{"id": "first", "prompt": "inspect synthetic first", "name": "PRIVATE_SPAWN_NAME_1", "label": "PRIVATE_SPAWN_LABEL_1"},
					map[string]any{"id": "second", "prompt": "inspect synthetic second", "name": "PRIVATE_SPAWN_NAME_2", "label": "PRIVATE_SPAWN_LABEL_2"},
				}}
				wantCount = 2
			}
			h, pending, _, _ := protectedSeamFixtureV1(t, tool, arguments)
			pending.ApprovalPolicy = "auto"
			providerClient := &providerStepRecordingProvider{}
			h.provider = providerClient
			if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), pending.SecurityContext, time.Second); err != nil {
				t.Fatal(err)
			}
			settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
			if err != nil || settled.IsError {
				output, _ := settled.Output.(map[string]any)
				t.Fatalf("real synthetic child spawn did not settle: err=%v code=%s status=%s", err, stringField(output, "code"), stringField(output, "status"))
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || display.Capture.Kind != "job" || display.Capture.Job == nil || display.Capture.Job.Operation != "spawn" || len(display.Capture.Job.Jobs) != wantCount || display.Capture.Job.Total != wantCount || display.Capture.Job.Truncated {
				t.Fatal("actual spawn lost its one protected Job observation")
			}
			records, err := h.jobs.List(pending.ThreadID)
			if err != nil || len(records) != wantCount || len(providerClient.Requests()) != wantCount {
				t.Fatal("fixture did not exercise actual admitted child production", err)
			}
			byID := map[string]jobs.Record{}
			for _, record := range records {
				byID[record.ID] = record
			}
			if tool == "parallel_tasks" {
				if records[0].ProtectedOutputSnapshot == nil || records[1].ProtectedOutputSnapshot == nil {
					t.Fatal("parallel producers did not retain independent sources")
				}
				candidate := records[0]
				candidate.ProtectedOutputSnapshot = domainjob.CloneProtectedOutputSnapshotV1(candidate.ProtectedOutputSnapshot)
				candidate.ProtectedOutputSnapshot.Binding = records[1].ProtectedOutputSnapshot.Binding
				binding := candidate.SecurityBinding
				release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(context.Background(), candidate.ParentThreadID, binding.ParentWorkspaceRealPath, binding.TenantID, binding.UserID)
				if err != nil || release == nil {
					t.Fatal(err)
				}
				_, readable := h.resolveRuntimeProtectedJobOutputV1(context.Background(), candidate)
				release()
				if readable {
					t.Fatal("parallel query accepted its sibling's real source binding")
				}
			}
			for i, observed := range display.Capture.Job.Jobs {
				record, found := byID[observed.JobID]
				if !found || observed.Name == nil || *observed.Name != record.Name || observed.Label == nil || *observed.Label != record.Label || !strings.HasPrefix(*observed.Label, "PRIVATE_SPAWN_LABEL") || observed.Status != record.Status || observed.Output.Availability != "unavailable" || display.Capture.Job.Receipt != nil {
					t.Fatal("spawn display fabricated or replaced returned producer facts")
				}
				if tool == "parallel_tasks" && record.ParallelIndex != i+1 {
					t.Fatal("parallel capture replaced producer order")
				}
				expected, _, err := subagentapp.TurnCompletionFromThreadStoreV1(h.store.GetThread, record.ChildThreadID, record.ChildTurnID)
				if err != nil {
					t.Fatal(err)
				}
				assertProtectedJobOutputQueryBindingV1(t, h, pending, record, expected, "provider-step-candidate-", tool+"-"+record.ID)
			}
			if len(providerClient.Requests()) != wantCount {
				t.Fatal("spawn output query reexecuted a child")
			}
			assertPublicToolSeamV1(t, h, pending, "PRIVATE_SPAWN_")
		})
	}
}
func TestSharedActualReadTodoAndSubagentErrorsSealOnceWithoutPublicRawV1(t *testing.T) {
	for _, test := range []struct {
		tool string
		args map[string]any
	}{{"read", map[string]any{"path": "actual-missing-file.txt"}}, {"todo_write", map[string]any{"todos": []any{map[string]any{"id": "todo-1", "content": "Actual todo", "status": "failed", "statusReasonCode": "tool_failed"}}}}, {"task", map[string]any{"prompt": "bounded"}}} {
		t.Run(test.tool, func(t *testing.T) {
			h, pending, _, _ := protectedSeamFixtureV1(t, test.tool, test.args)
			var childProvider *providerStepRecordingProvider
			if test.tool == "task" {
				pending.ApprovalPolicy = "auto"
				childProvider = &providerStepRecordingProvider{onCall: func(int, domainmodel.Request) error { return errors.New("bounded synthetic child error") }}
				h.provider = childProvider
				if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), pending.SecurityContext, time.Second); err != nil {
					t.Fatal(err)
				}
			}
			result, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
			if err != nil || !result.IsError {
				t.Fatal("actual error producer did not fail", err)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || display.Inspect == nil || display.Inspect.OutputError == nil || display.Inspect.OutputError.Availability != "available" || len(display.Inspect.OutputError.Entries) != 1 {
				t.Fatal("shared producer error was not sealed with its Inspect")
			}
			message := display.Inspect.OutputError.Entries[0].Message
			if test.tool == "task" {
				records, err := h.jobs.List(pending.ThreadID)
				if err != nil || len(records) != 1 || len(childProvider.Requests()) == 0 || display.Capture.Job == nil || len(display.Capture.Job.Jobs) != 1 || display.Capture.Job.Jobs[0].JobID != records[0].ID || display.Inspect.OutputError.Entries[0].JobID != records[0].ID {
					t.Fatal("completion error did not belong to its actual dispatched child and typed Job capture")
				}
			}
			assertPublicToolSeamV1(t, h, pending, message)
			_, replay, _ := readProtectedSeamV1(t, h, pending)
			if replay.SnapshotDigest != display.SnapshotDigest || replay.Inspect.OutputError.Entries[0].Message != message {
				t.Fatal("private read created a second observation")
			}
		})
	}
}

func TestActualParallelErrorIndicesIgnoreMissingMiddleRecordAndLateErrorV1(t *testing.T) {
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	captureRuntimeSubagentErrorsV1(ctx, []runtimeSubagentErrorObservationV1{{Err: errors.New("first actual failure"), ParallelIndex: 1}, {}, {Err: errors.New("third actual failure"), ParallelIndex: 3}})
	_, _, error_ := domaintoolresult.ConsumeProtectedToolObservationV1(ctx)
	if error_ == nil || error_.Availability != "available" || len(error_.Entries) != 2 || *error_.Entries[0].ParallelIndex != 1 || *error_.Entries[1].ParallelIndex != 3 {
		t.Fatal("filtered Job records shifted real child error indices")
	}
	captureRuntimeSubagentErrorsV1(ctx, []runtimeSubagentErrorObservationV1{{Err: errors.New("late background failure")}})
	if _, _, late := domaintoolresult.ConsumeProtectedToolObservationV1(ctx); late != nil {
		t.Fatal("late child overwrote parent acceptance")
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
			if display.ReadTarget == nil || display.ReadTarget.Path != filepath.Join(pending.Workspace, "code.go") || display.ReadTarget.WorkspaceRoot != pending.Workspace ||
				display.ReadTarget.StartLine != max(1, display.Capture.StartLine) || display.ReadTarget.ContentDigest != domainsecurity.SHA256Hex([]byte(original)) || display.Capture.ReadTarget != nil {
				t.Fatal("actual host target did not traverse sealed CAS / current authority / typed HTTP")
			}
			assertPublicToolSeamV1(t, h, pending, original)
			assertPublicToolSeamV1(t, h, pending, display.ReadTarget.Path)
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
			if status != http.StatusOK || after.Capture.Body != expected || after.SnapshotDigest != display.SnapshotDigest || !reflect.DeepEqual(after.ReadTarget, display.ReadTarget) {
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
	finishSidePromotionTurnV1(t, h, own)
	endedChild, err := h.store.GetThreadForAuthorityRepair(childID)
	if err != nil {
		t.Fatal(err)
	}
	// Mutable fork-looking metadata cannot erase the actual signed allocation.
	endedChild["forkedFromThreadId"] = endedChild["parentThreadId"]
	if err := h.store.ReplaceThreadForAuthorityRepair(childID, endedChild); err != nil {
		t.Fatal(err)
	}
	_, beforePromotion, _ := h.store.ReadThreadMutationBaseline(childID)
	if _, err := h.runtimeThreadService().Patch(context.Background(), childID, map[string]any{"promoteSide": true}); !errors.Is(err, threadapp.ErrSidePromotionUnavailable) {
		t.Fatal("ended signed child was promoted from mutable side metadata", err)
	}
	_, afterPromotion, _ := h.store.ReadThreadMutationBaseline(childID)
	if beforePromotion != afterPromotion {
		t.Fatal("refused signed child promotion changed its history")
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

func TestProtectedWebActualProducerSettlementAndPrivateHTTPV1(t *testing.T) {
	const text = "ordinary actual Web content "
	body := strings.Repeat(text, 3000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/request" {
			http.Redirect(w, r, "/final?observed=actual", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	requestURL := server.URL + "/request"
	for _, test := range []struct {
		name           string
		maxBytes       int
		wantBody       string
		fetchTruncated bool
	}{
		{name: "full_response", maxBytes: runtimeinfoapp.DefaultWebMaxFetchBytes, wantBody: strings.TrimSpace(body)},
		{name: "fetch_truncated", maxBytes: runtimeinfoapp.MinWebFetchBytes, wantBody: strings.TrimSpace(body[:runtimeinfoapp.MinWebFetchBytes]), fetchTruncated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, pending, _, _ := protectedSeamFixtureV1(t, "web_fetch", map[string]any{"url": requestURL, "max_bytes": float64(test.maxBytes), "timeout_ms": 3000.0})
			if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
				t.Fatal(err)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || display.Capture.Kind != "web_fetch" || display.Capture.Body != test.wantBody || display.Capture.Truncated || display.Capture.Status != "completed" || display.Capture.Web == nil || !display.Capture.Web.ResponseComplete || !display.Capture.Web.ToolIsError || display.Capture.Web.StatusCode == nil || *display.Capture.Web.StatusCode != 503 || display.Capture.Web.FetchTruncated == nil || *display.Capture.Web.FetchTruncated != test.fetchTruncated || display.Capture.Web.Error == nil || display.Capture.Web.Error.Message != "HTTP 503" {
				t.Fatal("actual non-2xx producer did not cross existing settlement/CAS/private HTTP")
			}
			if display.Inspect == nil || display.Inspect.InputJSON != string(pending.Call.Arguments) || display.Inspect.InputTruncated || display.Inspect.SchemaJSON != pending.ModelVisibleToolDefinition.SchemaJSON || display.Inspect.SchemaTruncated || display.Capture.Web.InputJSON != "" || display.Capture.Web.SchemaJSON != "" {
				t.Fatal("real pending payload/schema missing from Inspect capture")
			}
			var schema provider.ToolSchema
			if json.Unmarshal([]byte(display.Inspect.SchemaJSON), &schema) != nil || schema.Name != "web_fetch" {
				t.Fatal("Inspect schema is not the actual model-visible definition")
			}
			if display.Capture.Web.RequestURL != requestURL || display.Capture.Web.FinalURL != server.URL+"/final?observed=actual" || display.WebTargets == nil || display.WebTargets.RequestURL != requestURL || display.WebTargets.ResultURL != display.Capture.Web.FinalURL || display.Capture.WebTargets != nil {
				t.Fatal("private HTTP did not separate exact Host targets")
			}
			assertPublicToolSeamV1(t, h, pending, test.wantBody)
			assertPublicToolSeamV1(t, h, pending, requestURL)
			assertPublicToolSeamV1(t, h, pending, "inputJSON")
			assertPublicToolSeamV1(t, h, pending, "schemaJSON")
		})
	}
}

func TestProtectedWebSealingBindsPendingPayloadAndNeverInventsSchemaV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "web_fetch", map[string]any{"url": "https://example.test/actual"})
	status := 201
	truncated := false
	capture := domaintoolresult.ProtectedCaptureV1{Kind: "web_fetch", Status: "completed", Body: "actual captured text", Label: "Fetch", Web: &domaintoolresult.ProtectedWebCaptureV1{RequestURL: "https://example.test/actual", FinalURL: "https://example.test/result", StatusCode: &status, FetchTruncated: &truncated, ResponseComplete: true, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	sealed, ok := h.sealProtectedToolSnapshotV1(context.Background(), pending, capture)
	if !ok {
		t.Fatal("actual pending call failed sealing")
	}
	actual, err := h.toolSnapshots.Read(context.Background(), sealed.Binding)
	if err != nil || actual.Inspect == nil || actual.Inspect.InputJSON != string(pending.Call.Arguments) || actual.Inspect.SchemaJSON != pending.ModelVisibleToolDefinition.SchemaJSON {
		t.Fatal("sealed Web inspect does not bind actual pending input/schema")
	}
	forged := pending
	forged.Call.Arguments = json.RawMessage(`{"url":"https://forged.test/"}`)
	if _, ok := h.sealProtectedToolSnapshotV1(context.Background(), forged, capture); ok {
		t.Fatal("changed args reused an execution grant")
	}
	h.web.FetchEnabled = false
	sealed, ok = h.sealProtectedToolSnapshotV1(context.Background(), pending, capture)
	if !ok {
		t.Fatal("unknown schema should not discard actual already-captured result")
	}
	actual, err = h.toolSnapshots.Read(context.Background(), sealed.Binding)
	if err != nil || actual.Inspect == nil || actual.Inspect.SchemaJSON != pending.ModelVisibleToolDefinition.SchemaJSON {
		t.Fatal("changed catalog replaced the historical model-visible definition")
	}
	pending.ModelVisibleToolDefinition = nil
	sealed, ok = h.sealProtectedToolSnapshotV1(context.Background(), pending, capture)
	if !ok {
		t.Fatal("missing historical schema discarded actual capture")
	}
	actual, err = h.toolSnapshots.Read(context.Background(), sealed.Binding)
	if err != nil || actual.Inspect == nil || actual.Inspect.SchemaJSON != "" || actual.Inspect.SchemaTruncated {
		t.Fatal("missing actual schema was reconstructed or called budget truncation")
	}
}

func TestProtectedOrdinaryInspectActualSettlementAndEventTimesV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "ls", map[string]any{"path": "."})
	ready, _, err := h.store.RecordEvent(map[string]any{"kind": "tool_call_ready", "threadId": pending.ThreadID, "turnId": pending.TurnID, "itemId": pending.ToolCallItemID, "callId": pending.Call.ID, "toolName": pending.Call.Name})
	if err != nil {
		t.Fatal(err)
	}
	h.store.beforeRecordEventHook = func(event map[string]any) error {
		if stringField(event, "kind") == "tool_call_finished" {
			time.Sleep(20 * time.Millisecond)
		}
		return nil
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
		t.Fatal(err)
	}
	status, display, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.Capture.Kind != "file_result" || display.Capture.FileResult == nil || display.Capture.FileResult.Operation != "ls" || display.Capture.Body != "" || display.Inspect == nil || display.Inspect.InputJSON != string(pending.Call.Arguments) || display.Inspect.SchemaJSON != pending.ModelVisibleToolDefinition.SchemaJSON {
		t.Fatal("ordinary actual request did not cross settlement/CAS/private HTTP")
	}
	if display.Inspect.CallEventAt == nil || *display.Inspect.CallEventAt != stringField(ready, "timestamp") || display.Inspect.ResultEventAt == nil || display.Inspect.DurationMs == nil || *display.Inspect.DurationMs < 20 {
		t.Fatal("inspect omitted durable settlement delay or invented event timing")
	}
	loaded, err := h.store.LoadEventsSince(pending.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, event := range loaded.Events {
		if stringField(event, "kind") == "tool_call_finished" && stringField(event, "itemId") == display.ResultItemID {
			matched = *display.Inspect.ResultEventAt == stringField(event, "timestamp")
		}
	}
	if !matched {
		t.Fatal("inspect result time was not the actual recorded result event")
	}
	assertPublicToolSeamV1(t, h, pending, "inputJSON")
	assertPublicToolSeamV1(t, h, pending, "schemaJSON")
}

// Execute the real grep/glob adapter under the existing signed grant, then
// cross protected CAS, exact primary settlement, public replay and private HTTP.
// Neither the producer output nor its snapshot/settlement is overridden.
func TestProtectedSearchActualProducerSettlementAndPrivateHTTPV1(t *testing.T) {
	const fileName = "SYNTHETIC_PRIVATE_PATH_source.txt"
	const matchedLine = "SYNTHETIC_PRIVATE_MATCH needle"
	const before = "SYNTHETIC_PRIVATE_CONTEXT_BEFORE"
	const after = "SYNTHETIC_PRIVATE_CONTEXT_AFTER"
	for _, tool := range []string{"grep", "glob"} {
		t.Run(tool, func(t *testing.T) {
			arguments := map[string]any{"pattern": "SYNTHETIC_PRIVATE_PATH_*.txt"}
			if tool == "grep" {
				arguments = map[string]any{"pattern": "needle", "path": fileName, "literal": true, "context": float64(1)}
			}
			h, pending, _, _ := protectedSeamFixtureV1(t, tool, arguments)
			sourcePath := filepath.Join(pending.Workspace, fileName)
			if err := os.WriteFile(sourcePath, []byte(before+"\n"+matchedLine+"\n"+after+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
			if err != nil {
				t.Fatalf("actual %s settlement failed: %v", tool, err)
			}
			if settled.IsError {
				t.Fatal("actual Search producer returned failure")
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || display.ToolName != tool || display.Capture.Kind != "search" ||
				display.Capture.Status != "completed" || display.Capture.Truncated || display.Capture.Search == nil {
				t.Fatalf("actual Search did not cross settlement/CAS/private HTTP: status=%d", status)
			}
			search := display.Capture.Search
			if search.ToolIsError || search.Truncated || search.Total == nil || *search.Total != 1 || search.InputJSON != "" || search.InputTruncated || display.Inspect == nil || display.Inspect.InputJSON != string(pending.Call.Arguments) || display.Inspect.InputTruncated {
				t.Fatal("actual Search capture lost producer count or grant-bound input")
			}
			if tool == "grep" {
				if search.Shape != "matches" || len(search.Files) != 1 || filepath.Base(search.Files[0].Path) != fileName || len(search.Files[0].Matches) != 1 {
					t.Fatal("actual grep producer file/match shape was lost")
				}
				match := search.Files[0].Matches[0]
				if match.LineNumber != 2 || match.Line != matchedLine || match.Column != strings.Index(matchedLine, "needle")+1 ||
					!reflect.DeepEqual(match.ContextBefore, []string{before}) || !reflect.DeepEqual(match.ContextAfter, []string{after}) {
					t.Fatal("actual grep line/column/context was lost")
				}
			} else if search.Shape != "paths" || len(search.Paths) != 1 || filepath.Base(search.Paths[0]) != fileName {
				t.Fatal("actual glob path shape was lost")
			}
			thread, err := h.store.GetThread(pending.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			item := protectedSeamResultV1(t, thread, pending)
			binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(item[domaintoolresult.ProtectedSnapshotBindingFieldV1])
			if err != nil || binding.SnapshotDigest != display.SnapshotDigest || stringField(item, "status") != "completed" ||
				stringField(item, "toolKind") != "tool_call" || item["isError"] != false {
				t.Fatal("real primary result did not retain the exact private Search binding")
			}
			publicOutput, err := json.Marshal(settled.Output)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{fileName, sourcePath, matchedLine, before, after, "inputJSON"} {
				if strings.Contains(string(publicOutput), private) {
					t.Fatal("private Search result entered settled public output")
				}
				assertPublicToolSeamV1(t, h, pending, private)
			}
			// Existing local authority still controls the admitted sealed result.
			h.turnSecurity.Identity = nil
			status, _, response := readProtectedSeamV1(t, h, pending)
			if status != http.StatusNotFound || strings.Contains(response, fileName) || strings.Contains(response, matchedLine) || strings.Contains(response, "capture") {
				t.Fatal("Search admission bypassed revoked local authority")
			}
		})
	}
}

func TestForkEventBoundaryCaseInheritedPrefixKeepsPrivateWorkspace(t *testing.T) {
	h, parent, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"}, true)
	if err := os.WriteFile(filepath.Join(parent.Workspace, "code.go"), []byte("synthetic CASE fork workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil {
		t.Fatal(err)
	}
	first, err := h.store.ForkThread(parent.ThreadID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if stringField(first, "activeInheritedHistoryReceipt") == "" {
		t.Fatal("missing real signed inherited chain")
	}
	own := protectedSeamPendingV1(t, h, first, "read", map[string]any{"path": "code.go"}, "turn_case_boundary_owner")
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), own, nil, nil); err != nil {
		t.Fatal(err)
	}
	reader, err := finalauthority.NewAcceptedFinalCASReader(h.store.root)
	if err != nil {
		t.Fatal(err)
	}
	current := threadapp.NewCurrentCaseThreadAuthorityValidator(h.caseThreads, filestore.CaseBindingReader{}, nil)
	h.publicProjector = threadapp.NewTrustedPublicProjectorWithPrimaryCAS(nil, h.caseThreads, current, reader)
	h.threads = nil
	derived := forkBoundaryHTTPV1(t, h, http.MethodPost, stringField(first, "id"), map[string]any{"turnId": parent.TurnID, "endSeq": 1}, http.StatusCreated)
	exact, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), stringField(derived, "id"))
	if err != nil {
		t.Fatal(err)
	}
	if stringField(exact.Thread, "workspace") != parent.Workspace || stringField(exact.Thread, "activeInheritedHistoryReceipt") == "" {
		t.Fatal("bounded CASE derivation lost real Host workspace or signed chain")
	}
	if derived["workspace"] != nil || derived["forkCreationPendingV1"] != nil {
		t.Fatal("private CASE metadata escaped public response")
	}
}

func TestProtectedPresentActualProducerSettlementPrivateHTTPAndRestartV1(t *testing.T) {
	description := "PRIVATE_PRESENT_DESCRIPTION"
	args := map[string]any{"files": []any{map[string]any{"path": "PRIVATE_PRESENT_RESULT.pdf", "description": description}, map[string]any{"path": "second.docx", "description": ""}, map[string]any{"path": "PRIVATE_PRESENT_RESULT.pdf"}}}
	h, pending, durable, root := protectedSeamFixtureV1(t, "present", args)
	for _, name := range []string{"PRIVATE_PRESENT_RESULT.pdf", "second.docx"} {
		if err := os.WriteFile(filepath.Join(pending.Workspace, name), []byte("PRIVATE_PRESENT_CONTENT"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
	if err != nil || settled.IsError {
		t.Fatal("actual present producer/settlement failed", err)
	}
	status, display, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.ToolName != "present" || display.ResultItemID != domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID) || display.Capture.Kind != "present" || display.Capture.Present == nil || len(display.Capture.Present.Files) != 3 || display.PresentTargets == nil || display.Capture.PresentTargets != nil || display.Capture.Body != "" || display.ReadTarget != nil {
		t.Fatal("real private receipt missing or acquired frozen preview")
	}
	for index, file := range display.Capture.Present.Files {
		if file.Index != index || display.PresentTargets.Files[index].Index != index || display.PresentTargets.Files[index].Path != filepath.Join(pending.Workspace, file.Path) {
			t.Fatal("declaration order/target identity changed")
		}
	}
	if *display.Capture.Present.Files[0].Description != description || display.Capture.Present.Files[1].Description == nil || *display.Capture.Present.Files[1].Description != "" || display.Capture.Present.Files[2].Description != nil || display.Inspect == nil || display.Inspect.InputJSON != string(pending.Call.Arguments) {
		t.Fatal("exact declaration/request observation changed")
	}
	for _, private := range []string{description, "PRIVATE_PRESENT_RESULT.pdf", "PRIVATE_PRESENT_CONTENT", display.PresentTargets.Files[0].Path, "presentTargets", "inputJSON"} {
		assertPublicToolSeamV1(t, h, pending, private)
	}
	thread, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	result := protectedSeamResultV1(t, thread.Thread, pending)
	before := h.store.WriteAttempts()
	for i := 0; i < 2; i++ {
		if err := h.store.EnsureProtectedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, result); err != nil {
			t.Fatal("exact replay refused", err)
		}
	}
	conflict := cloneMap(result)
	binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(conflict[domaintoolresult.ProtectedSnapshotBindingFieldV1])
	if err != nil {
		t.Fatal(err)
	}
	binding.BodyDigest = strings.Repeat("f", 64)
	conflict[domaintoolresult.ProtectedSnapshotBindingFieldV1] = domaintoolresult.ProtectedSnapshotBindingRecordV1(binding)
	if err := h.store.EnsureProtectedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, conflict); err == nil || h.store.WriteAttempts() != before {
		t.Fatal("conflicting receipt replay wrote primary state")
	}
	// Later edits are allowed: only declaration bytes, not file contents, are frozen.
	if err := os.WriteFile(filepath.Join(pending.Workspace, "PRIVATE_PRESENT_RESULT.pdf"), []byte("edited current file"), 0600); err != nil {
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
	status, replay, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || replay.SnapshotDigest != display.SnapshotDigest || !reflect.DeepEqual(replay.Capture.Present, display.Capture.Present) || !reflect.DeepEqual(replay.PresentTargets, display.PresentTargets) {
		t.Fatal("restart changed original declaration or selected target")
	}
	h.turnSecurity.Identity = nil
	if status, _, response := readProtectedSeamV1(t, h, pending); status != http.StatusNotFound || strings.Contains(response, "capture") {
		t.Fatal("revoked identity released present receipt")
	}
}

func TestProtectedPresentFailureNeverSettlesSuccessReceiptV1(t *testing.T) {
	args := map[string]any{"files": []any{map[string]any{"path": "missing.pdf"}}}
	h, pending, _, _ := protectedSeamFixtureV1(t, "present", args)
	result, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
	if err != nil || !result.IsError {
		t.Fatal("missing file did not fail", err)
	}
	status, display, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.Capture.Kind != "inspect" || display.Capture.Present != nil || display.PresentTargets != nil || display.Inspect == nil || display.Inspect.OutputError == nil || display.Inspect.OutputError.Availability != "available" {
		t.Fatal("failed present produced success receipt or lost error observation")
	}
	assertPublicToolSeamV1(t, h, pending, display.Inspect.OutputError.Entries[0].Message)
	// An actual successful producer cannot survive a later rejected transform.
	for _, test := range []struct{ code, captureStatus string }{
		{"tool_failed", "failed"}, {"approval_policy_blocked", "failed"}, {"tool_cancelled", "canceled"},
	} {
		t.Run(test.code, func(t *testing.T) {
			h2, pending2, _, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "result.pdf"}}})
			if err := os.WriteFile(filepath.Join(pending2.Workspace, "result.pdf"), []byte("source"), 0600); err != nil {
				t.Fatal(err)
			}
			settled, err := h2.executeAndSettleRuntimeTool(context.Background(), pending2, nil, func(output any, isError bool) (any, bool, error) {
				if isError {
					t.Fatal("actual declaration failed before transform")
				}
				return map[string]any{"code": test.code}, true, nil
			})
			if err != nil || !settled.IsError {
				t.Fatal("transformed rejection failed to settle", err)
			}
			status, failedDisplay, _ := readProtectedSeamV1(t, h2, pending2)
			if status != http.StatusOK || failedDisplay.Capture.Kind != "inspect" || failedDisplay.Capture.Status != test.captureStatus || failedDisplay.Capture.Present != nil || failedDisplay.PresentTargets != nil || failedDisplay.Inspect == nil || failedDisplay.Inspect.InputJSON != string(pending2.Call.Arguments) || failedDisplay.Inspect.OutputError == nil {
				t.Fatal("successful declaration survived rejection or private request/error facts were lost")
			}
		})
	}
}

func TestProtectedPresentCanceledDispatchDoesNotDeclareOrWriteV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "result.pdf"}}})
	if err := os.WriteFile(filepath.Join(pending.Workspace, "result.pdf"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := h.store.WriteAttempts()
	if settled, err := h.executeAndSettleRuntimeTool(ctx, pending, nil, nil); err == nil && !settled.IsError {
		t.Fatal("canceled dispatch succeeded")
	}
	if h.store.WriteAttempts() != before {
		t.Fatal("canceled dispatch wrote a tool result")
	}
	status, display, response := readProtectedSeamV1(t, h, pending)
	if status != http.StatusNotFound || display.Capture.Present != nil || display.PresentTargets != nil || strings.Contains(response, "capture") {
		t.Fatal("canceled dispatch released a successful receipt")
	}
}

func TestProtectedPresentWorkspaceAndCurrentEpochWitnessDriftRefusedV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "result.pdf", "description": "PRIVATE_PRESENT_AUTHORITY"}}})
	if err := os.WriteFile(filepath.Join(pending.Workspace, "result.pdf"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil || settled.IsError {
		t.Fatal("actual declaration failed", err)
	}
	status, originalDisplay, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || originalDisplay.Capture.Present == nil || originalDisplay.PresentTargets == nil {
		t.Fatal("original receipt unavailable before authority drift")
	}
	original, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"workspace_drift", "current_epoch_without_matching_frozen_turn"} {
		t.Run(mode, func(t *testing.T) {
			altered := cloneMap(original.Thread)
			var successor domainsecurity.TurnSecurityContext
			if mode == "workspace_drift" {
				altered["workspace"] = workspacetest.New(t)
			} else {
				// A structurally valid new epoch cannot replace the original own
				// frozen turn merely by changing the current top-level pointer.
				state, err := contextepochapp.BootstrapState(pending.ThreadID, pending.SecurityContext.ContextEpoch+1, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(pending.SecurityContext)}, time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				altered["contextEpochState"] = contextepochapp.PublicState(state)
				future, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(), Authority: h.turnSecurity, Thread: altered, ThreadID: pending.ThreadID, TurnID: "turn_present_epoch_successor", Workspace: pending.Workspace, Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC()})
				if err != nil || future.ContextEpoch != pending.SecurityContext.ContextEpoch+1 || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(future) != nil {
					t.Fatal("new epoch fixture was not valid", err)
				}
				successor = future
				altered["securityState"] = turnsecurityapp.PublicRecord(future)
			}
			if err := h.store.ReplaceThreadForAuthorityRepair(pending.ThreadID, altered); err != nil {
				t.Fatal(err)
			}
			before := h.store.WriteAttempts()
			status, display, response := readProtectedSeamV1(t, h, pending)
			if status != http.StatusNotFound || display.Capture.Present != nil || display.PresentTargets != nil || strings.Contains(response, "capture") || strings.Contains(response, "PRIVATE_PRESENT_AUTHORITY") {
				t.Fatal("drifted authority released declarations or targets")
			}
			if h.store.WriteAttempts() != before {
				t.Fatal("refused read repaired or wrote primary state")
			}
			if err := h.store.ReplaceThreadForAuthorityRepair(pending.ThreadID, cloneMap(original.Thread)); err != nil {
				t.Fatal(err)
			}
			if mode == "current_epoch_without_matching_frozen_turn" {
				// Restore the primary structure, then commit the actual successor
				// own turn. Rewriting an older primary alone cannot establish
				// recovery of a current risk/context authority.
				successorAt, err := time.Parse(time.RFC3339Nano, successor.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				epoch, err := contextepochapp.BootstrapState(pending.ThreadID, successor.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(successor)}, successorAt)
				if err != nil {
					t.Fatal(err)
				}
				if err := h.store.AppendTurnToThread(pending.ThreadID, map[string]any{"id": successor.TurnID, "threadId": pending.ThreadID, "status": "running", "createdAt": successor.IssuedAt, "securityContext": turnsecurityapp.PublicRecord(successor), "items": []any{}}, "synthetic", map[string]any{"securityState": turnsecurityapp.PublicRecord(successor), "contextEpochState": contextepochapp.PublicState(epoch)}); err != nil {
					t.Fatal(err)
				}
			}
			if status, display, _ := readProtectedSeamV1(t, h, pending); status != http.StatusOK || display.SnapshotDigest != originalDisplay.SnapshotDigest || !reflect.DeepEqual(display.Capture.Present, originalDisplay.Capture.Present) || !reflect.DeepEqual(display.PresentTargets, originalDisplay.PresentTargets) {
				t.Fatal("valid current own authority lost or changed original immutable receipt")
			}
		})
	}
}

func readPresentHistorySeamV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall, authority ...snapshotport.AccessAuthority) (int, snapshotapp.PresentHistoryDisplayV1, string) {
	t.Helper()
	selector := snapshotport.PresentHistorySelectorV1{Kind: snapshotport.PresentHistoryKindV1, ThreadID: pending.ThreadID, TurnID: pending.TurnID}
	body, err := json.Marshal(selector)
	if err != nil {
		t.Fatal(err)
	}
	var leaf http.Handler = h.ToolResultLocalDisplayV1()
	if len(authority) == 1 {
		leaf = httpapi.ToolResultLocalDisplayHandlerV1{Service: &snapshotapp.Service{Store: h.toolSnapshots, Authority: authority[0]}}
	}
	r := httptest.NewRequest(http.MethodPost, httpapi.ToolResultLocalDisplayPathV1, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, ToolResults: leaf}.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private history caching enabled")
	}
	var display snapshotapp.PresentHistoryDisplayV1
	if w.Code == http.StatusOK && json.Unmarshal(w.Body.Bytes(), &display) != nil {
		t.Fatal("malformed history wire")
	}
	return w.Code, display, w.Body.String()
}

func sameTurnPresentSeamV1(t *testing.T, h *runtimeServerHandler, previous runtimePendingToolCall, name, file string) runtimePendingToolCall {
	t.Helper()
	present := previous
	raw, err := json.Marshal(map[string]any{"files": []any{map[string]any{"path": file}}})
	if err != nil {
		t.Fatal(err)
	}
	present.Call = provider.ToolCall{ID: serverTestHostToolCallID(name), Name: "present", Arguments: raw}
	present.ToolCallItemID = domaintoolcall.ToolCallItemIDV1(present.TurnID, present.Call.ID)
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, present.ToolScope, false, false, present.Prompt, false, nil)
	present.ExecutionGrant, err = executiongrantapp.IssueProvider(present.SecurityContext, present.ProviderID, present.Call, schemas, present.ToolScope, true, "not_required", 0, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	present.ModelVisibleToolDefinition = domainmodel.FreezeModelVisibleToolDefinitionsV1(schemas)["present"]
	if _, err := h.persistToolCallReady(context.Background(), present.ThreadID, present.TurnID, present.Call, 1, present.SecurityContext, present.ExecutionGrant); err != nil {
		t.Fatal(err)
	}
	if err := h.authorizeRuntimePending(context.Background(), present, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return present
}

func TestProtectedPresentHistoryActualPrimaryOrderClosingAndRestartV1(t *testing.T) {
	h, first, durable, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "PRIVATE_HISTORY_RESULT.pdf", "description": "PRIVATE_HISTORY_DESCRIPTION"}}})
	for _, name := range []string{"PRIVATE_HISTORY_RESULT.pdf", "second.pdf"} {
		if err := os.WriteFile(filepath.Join(first.Workspace, name), []byte("PRIVATE_HISTORY_BODY"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if result, err := h.executeAndSettleRuntimeTool(context.Background(), first, nil, nil); err != nil || result.IsError {
		t.Fatal("first actual Present failed", err)
	}
	second := sameTurnPresentSeamV1(t, h, first, "history_second_present", "second.pdf")
	if result, err := h.executeAndSettleRuntimeTool(context.Background(), second, nil, nil); err != nil || result.IsError {
		t.Fatal("second actual Present failed", err)
	}
	status, live, _ := readPresentHistorySeamV1(t, h, first)
	if status != http.StatusOK || len(live.OrderedItems) != 2 {
		t.Fatal("live Present identities unavailable", status)
	}
	finish := time.Now().UTC().Format(time.RFC3339Nano)
	committed, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: h.store, SecurityContext: first.SecurityContext, ThreadID: first.ThreadID, TurnID: first.TurnID, Model: first.Model, CreatedAt: finish, FinishedAt: finish, Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{}, nil)})
	if err != nil || !committed.Changed {
		t.Fatal("real ordinary closing did not commit", err)
	}
	status, history, response := readPresentHistorySeamV1(t, h, first)
	if status != http.StatusOK || history.SchemaVersion != 1 || history.Kind != "present_history" || history.ThreadID != first.ThreadID || history.TurnID != first.TurnID || len(history.OrderedItems) != 3 {
		t.Fatal("closing history unavailable", status)
	}
	wanted := []snapshotport.PresentHistoryItemV1{
		{Kind: "tool_result", ItemID: domaintoolresult.ToolResultItemIDV1(first.TurnID, first.Call.ID), CallID: first.Call.ID, ToolName: "present"},
		{Kind: "tool_result", ItemID: domaintoolresult.ToolResultItemIDV1(second.TurnID, second.Call.ID), CallID: second.Call.ID, ToolName: "present"},
		{Kind: "assistant_text", ItemID: "item_" + first.TurnID + "_assistant"},
	}
	if !reflect.DeepEqual(history.OrderedItems, wanted) {
		t.Fatal("actual primary declaration/closing order changed")
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), first.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	var fromPrimary []string
	for _, rawTurn := range primary.Thread["turns"].([]any) {
		turn := rawTurn.(map[string]any)
		if stringField(turn, "id") != first.TurnID {
			continue
		}
		for _, rawItem := range turn["items"].([]any) {
			item := rawItem.(map[string]any)
			if stringField(item, "kind") == "assistant_text" || stringField(item, "kind") == "tool_result" && stringField(item, "toolName") == "present" {
				fromPrimary = append(fromPrimary, stringField(item, "id"))
			}
		}
	}
	for index, item := range history.OrderedItems {
		if item.ItemID != fromPrimary[index] {
			t.Fatal("history was not drawn from the same primary ordering")
		}
	}
	for _, private := range []string{"PRIVATE_HISTORY", "body", "path", "description", "seq", "ThreadFileSHA256", "contextDigest", "snapshotDigest", first.Workspace} {
		if strings.Contains(response, private) {
			t.Fatal("history released contents or a fabricated coordinate", private)
		}
	}
	if _, display, _ := readProtectedSeamV1(t, h, first); display.Capture.Present == nil {
		t.Fatal("history replaced the protected receipt reader")
	}
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
	h.store.SetCaseThreadAuthority(h.caseThreads)
	status, reopened, _ := readPresentHistorySeamV1(t, h, first)
	if status != http.StatusOK || !reflect.DeepEqual(reopened, history) {
		t.Fatal("strict primary reopen changed ordered identities", status)
	}
	h.turnSecurity.Identity = nil
	if status, denied, response := readPresentHistorySeamV1(t, h, first); status != http.StatusNotFound || denied.OrderedItems != nil || strings.Contains(response, "orderedItems") {
		t.Fatal("revoked owner released history")
	}
}

func TestProtectedPresentHistoryCasePublicFilterKeepsPrivateAnchorsV1(t *testing.T) {
	h, pending, durable, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "PRIVATE_CASE_HISTORY.pdf"}}}, true)
	if err := os.WriteFile(filepath.Join(pending.Workspace, "PRIVATE_CASE_HISTORY.pdf"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil || result.IsError {
		t.Fatal("actual case Present failed", err)
	}
	index, reader := configureWorkspaceReadFinalAuthority(t, h, t.TempDir(), durable, false)
	if _, err := h.caseFinalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{Store: h.store, Context: pending.SecurityContext, TerminalReason: evidenceapp.TerminalProviderFailure, ThreadID: pending.ThreadID, TurnID: pending.TurnID, AcceptedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	h.publicProjector = threadapp.NewTrustedPublicProjectorWithPrimaryCAS(index, h.caseThreads, threadapp.NewCurrentCaseThreadAuthorityValidator(h.caseThreads, filestore.CaseBindingReader{}, nil), reader)
	h.threads = nil
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	public, err := h.publicProjector.ProjectThread(primary.Thread)
	if err != nil {
		t.Fatal(err)
	}
	var closing string
	for _, rawTurn := range public["turns"].([]any) {
		turn := rawTurn.(map[string]any)
		for _, raw := range turn["items"].([]any) {
			item := raw.(map[string]any)
			if stringField(item, "kind") == "tool_result" {
				t.Fatal("case public history acquired hidden Present result")
			}
			if stringField(item, "kind") == "assistant_text" {
				closing = stringField(item, "id")
			}
		}
	}
	if closing == "" {
		t.Fatal("actual case boundary has no admitted closing")
	}
	status, history, response := readPresentHistorySeamV1(t, h, pending)
	if status != http.StatusOK || len(history.OrderedItems) != 2 || history.OrderedItems[0].ItemID != domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID) || history.OrderedItems[1].ItemID != closing {
		t.Fatal("public case filter lost private actual identities", status)
	}
	if strings.Contains(response, "PRIVATE_CASE_HISTORY") || strings.Contains(response, pending.Workspace) {
		t.Fatal("case history leaked private file metadata")
	}
	assertPublicToolSeamV1(t, h, pending, "PRIVATE_CASE_HISTORY.pdf")
	if status, display, _ := readProtectedSeamV1(t, h, pending); status != http.StatusOK || display.Capture.Present == nil {
		t.Fatal("failed case turn erased successful Present")
	}
}

func TestProtectedPresentHistoryAbortedWithoutClosingAndFailedReceiptV1(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "success_then_stopped", true: "failed_present"}[missing], func(t *testing.T) {
			h, pending, _, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "result.pdf"}}})
			if !missing {
				if err := os.WriteFile(filepath.Join(pending.Workspace, "result.pdf"), []byte("source"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
			if err != nil || settled.IsError != missing {
				t.Fatal("Present outcome changed", err)
			}
			stopped, err := turnapp.CommitGeneralFailureTerminal(turnapp.PersistFailureInput{Store: h.store, SecurityContext: pending.SecurityContext, TerminalReason: "cancel", ThreadID: pending.ThreadID, TurnID: pending.TurnID, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Failure: domainfailure.New(domainfailure.CodeTurnCancelled, nil), Model: pending.Model, Interrupt: &turnapp.GeneralTerminalInterruptMetadata{Cancelled: true}})
			if err != nil || !stopped.Changed || stopped.Status != "aborted" {
				t.Fatal("actual turn stop failed", err)
			}
			status, history, _ := readPresentHistorySeamV1(t, h, pending)
			if status != http.StatusOK || len(history.OrderedItems) != 1 || history.OrderedItems[0].Kind != "tool_result" {
				t.Fatal("stopped history lost result or fabricated assistant closing", status)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || (display.Capture.Present != nil) == missing {
				t.Fatal("turn stop confused successful and failed Present receipts", status)
			}
		})
	}
}

type presentHistoryChangingAccessV1 struct {
	runtimeToolSnapshotAccessV1
	checks        int
	afterValidate func()
}

func (a *presentHistoryChangingAccessV1) ValidatePresentHistoryCurrentV1(ctx context.Context, expected snapshotport.PresentHistoryAuthorizedV1) error {
	if err := a.runtimeToolSnapshotAccessV1.ValidatePresentHistoryCurrentV1(ctx, expected); err != nil {
		return err
	}
	a.checks++
	if a.checks == 1 {
		a.afterValidate()
	}
	return nil
}
func TestProtectedPresentHistoryFinalPrimaryChangeAndOwnerMismatchRefusedV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "result.pdf"}}})
	if err := os.WriteFile(filepath.Join(pending.Workspace, "result.pdf"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil || result.IsError {
		t.Fatal(err)
	}
	if status, _, _ := readPresentHistorySeamV1(t, h, pending); status != http.StatusOK {
		t.Fatal("history unavailable before interleaving", status)
	}
	before := h.store.WriteAttempts()
	originalPrimary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	var changedPrimarySHA256 string
	access := &presentHistoryChangingAccessV1{runtimeToolSnapshotAccessV1: runtimeToolSnapshotAccessV1{h}, afterValidate: func() {
		primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		primary.Thread["title"] = "concurrent task-owned primary change"
		if err := h.store.ReplaceThreadForAuthorityRepair(pending.ThreadID, primary.Thread); err != nil {
			t.Fatal(err)
		}
		changed, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		changedPrimarySHA256 = changed.ThreadFileSHA256
	}}
	if status, denied, response := readPresentHistorySeamV1(t, h, pending, access); status != http.StatusNotFound || denied.OrderedItems != nil || strings.Contains(response, "orderedItems") {
		t.Fatal("final primary freshness released stale order")
	}
	currentPrimary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	// WriteAttempts counts event-log writes; the primary mutation is witnessed
	// by its exact changed hash. Denial must preserve only that injected state.
	if access.checks != 1 || changedPrimarySHA256 == originalPrimary.ThreadFileSHA256 || currentPrimary.ThreadFileSHA256 != changedPrimarySHA256 || h.store.WriteAttempts() != before {
		t.Fatal("fresh denial wrote repair state or missed controlled concurrent change")
	}
	if status, _, _ := readPresentHistorySeamV1(t, h, pending); status != http.StatusOK {
		t.Fatal("fresh legal primary cannot be read")
	}
	if err := h.store.PatchTurnItemStatus(pending.ThreadID, pending.TurnID, pending.ToolCallItemID, "failed"); err != nil {
		t.Fatal(err)
	}
	before = h.store.WriteAttempts()
	if status, denied, response := readPresentHistorySeamV1(t, h, pending); status != http.StatusNotFound || denied.OrderedItems != nil || strings.Contains(response, "orderedItems") {
		t.Fatal("mismatched actual call/result owner released history")
	}
	if h.store.WriteAttempts() != before {
		t.Fatal("denied history repaired primary")
	}
}

func TestGenericSuccessTextCannotOverrideNativeOutcomeOrGrantV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "source.go"})
	fact := domaintoolresult.ProtectedInspectV1{OutputText: domaintoolresult.ProjectProtectedOutputTextV1("actual literal text")}
	for _, status := range []string{"completed", "failed", "canceled", "unknown"} {
		if _, sealed := h.sealProtectedToolSnapshotV1(context.Background(), pending, domaintoolresult.ProtectedCaptureV1{Kind: "inspect", Status: status}, fact); sealed {
			t.Fatal("non-MCP/native outcome acquired generic text")
		}
	}
	changed := pending
	changed.Call.Arguments = json.RawMessage(`{"path":"different.go"}`)
	if _, sealed := h.sealProtectedToolSnapshotV1(context.Background(), changed, domaintoolresult.ProtectedCaptureV1{Kind: "inspect", Status: "completed"}); sealed {
		t.Fatal("changed arguments reused exact original grant")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, sealed := h.sealProtectedToolSnapshotV1(canceled, pending, domaintoolresult.ProtectedCaptureV1{Kind: "inspect", Status: "completed"}); sealed {
		t.Fatal("canceled owner sealed a readable snapshot")
	}
}
