package server

import (
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	processapp "analytix.local/runtime-go/internal/app/processdisplay"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	processport "analytix.local/runtime-go/internal/ports/processdisplay"
	"analytix.local/runtime-go/internal/provider"
)

const processSeamViewerV1 = "0c85059c-540e-4bdc-a1de-5bfb26a6168f"
const processSeamDocumentV1 = "bf82dc58-764d-4845-a2f4-9e9823618ba9"

// These requests use the real typed mux, runtime bearer and native document
// lease. The current-context and history validators are the existing fixture's
// real signed origin validators, rather than an allow-all display authority.
func processSeamRequestV1(t *testing.T, h *runtimeServerHandler, path string, payload map[string]any) (processapp.ResponseV1, string) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, Processes: h.ProcessLocalDisplayV1()}.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("typed process response status=%d", w.Code)
	}
	var result processapp.ResponseV1
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result, w.Body.String()
}

func processSeamAuthorizeV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall, token string) processapp.ResponseV1 {
	t.Helper()
	response, _ := processSeamRequestV1(t, h, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "authorize", "viewerToken": token, "documentNonce": processSeamDocumentV1, "threadId": pending.ThreadID, "turnId": pending.TurnID})
	return response
}

func processSeamReadV1(t *testing.T, h *runtimeServerHandler, token string, expanded ...string) (processapp.ResponseV1, string) {
	t.Helper()
	if expanded == nil {
		expanded = []string{}
	}
	return processSeamRequestV1(t, h, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "read", "viewerToken": token, "expandedBlockIds": expanded})
}

func processSeamOwnBlockV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall, targets ...domainprocess.CallAnchorV2) domainprocess.BlockV1 {
	t.Helper()
	if response := processSeamAuthorizeV1(t, h, pending, processSeamViewerV1); response.Status != "authorized" {
		t.Fatalf("own process authorization=%s", response.Status)
	}
	collapsed, _ := processSeamReadV1(t, h, processSeamViewerV1)
	if collapsed.Status != "snapshot" || len(targets) > 1 || len(targets) == 0 && len(collapsed.Blocks) != 1 {
		t.Fatalf("collapsed process response disposition=%s blocks=%d", collapsed.Status, len(collapsed.Blocks))
	}
	var block domainprocess.BlockV1
	matches := 0
	for _, candidate := range collapsed.Blocks {
		if candidate.Body != nil {
			t.Fatal("collapsed snapshot released a body")
		}
		if len(targets) == 1 {
			target := targets[0]
			if candidate.Kind != "shell" || candidate.State != "completed" || candidate.Source.Kind != "tool" || candidate.Source.CallID != target.CallID || candidate.Source.ToolCallItemID != target.ToolCallItemID || candidate.Source.ToolResultItemID != target.ToolResultItemID {
				continue
			}
		}
		block, matches = candidate, matches+1
	}
	if matches != 1 {
		t.Fatal("actual durable capture identity was absent or ambiguous")
	}
	expanded, _ := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
	if expanded.Status != "snapshot" || len(expanded.Blocks) != len(collapsed.Blocks) {
		t.Fatal("expansion changed the complete snapshot")
	}
	var selected domainprocess.BlockV1
	for _, candidate := range expanded.Blocks {
		if candidate.BlockID == block.BlockID {
			if candidate.Body == nil || !reflect.DeepEqual(candidate.Source, block.Source) || candidate.CaptureDigest != block.CaptureDigest {
				t.Fatal("actual frozen capture could not expand")
			}
			selected = candidate
		} else if candidate.Body != nil {
			t.Fatal("expansion released another block body")
		}
	}
	if selected.Body == nil {
		t.Fatal("exact expanded target missing")
	}
	opened, _ := processSeamRequestV1(t, h, httpapi.ProcessLocalDisplayTargetPathV1, map[string]any{"schemaVersion": 2, "action": "open_capture", "viewerToken": processSeamViewerV1, "blockId": block.BlockID, "captureDigest": block.CaptureDigest})
	if opened.Status != "capture" || opened.Block == nil || opened.Block.Body == nil || opened.Block.Body.Text != selected.Body.Text {
		t.Fatal("capture target did not use the frozen body digest")
	}
	return selected
}

func assertProcessSeamPublicIsolationV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall, block domainprocess.BlockV1, privateText ...string) {
	t.Helper()
	for _, path := range []string{"/v1/threads/" + pending.ThreadID, "/v1/threads/" + pending.ThreadID + "/events?since_seq=0"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("public process seam status=%d", w.Code)
		}
		for _, forbidden := range append([]string{"captureDigest", "expandedBlockIds", "processDisplay", "displayCapture", block.BlockID, block.CaptureDigest}, privateText...) {
			if forbidden != "" && strings.Contains(w.Body.String(), forbidden) {
				t.Fatal("private process copy entered public history or actual SSE adapter")
			}
		}
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot.Thread)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"processDisplay", "displayCapture", block.BlockID, block.CaptureDigest} {
		if forbidden != "" && strings.Contains(string(encoded), forbidden) {
			t.Fatal("volatile process copy entered durable primary")
		}
	}
}

func TestProcessDisplayNativeSSEAdapterUsesCurrentOwnHistoryAndProtectedHTTP(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	const thinking = "PRIVATE_NATIVE_PROCESS_SEAM_3712"
	const signature = "PRIVATE_SIGNATURE_PROCESS_SEAM_7312"
	const redacted = "PRIVATE_REDACTED_PROCESS_SEAM_1732"
	payload := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_process_seam","model":"claude-test","usage":{"input_tokens":4,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":3,"content_block":{"type":"thinking","thinking":""}}`,
		`data: {"type":"content_block_stop","index":3}`,
		`data: {"type":"content_block_start","index":8,"content_block":{"type":"thinking","thinking":""}}`,
		`data: {"type":"content_block_delta","index":8,"delta":{"type":"thinking_delta","thinking":"` + thinking + `"}}`,
		`data: {"type":"content_block_delta","index":8,"delta":{"type":"signature_delta","signature":"` + signature + `"}}`,
		`data: {"type":"content_block_stop","index":8}`,
		`data: {"type":"content_block_start","index":11,"content_block":{"type":"redacted_thinking","data":"` + redacted + `"}}`,
		`data: {"type":"content_block_stop","index":11}`,
		`data: {"type":"content_block_start","index":14,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":14,"delta":{"type":"text_delta","text":"visible answer"}}`,
		`data: {"type":"content_block_stop","index":14}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"
	requests := 0
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(payload))
	}))
	defer fake.Close()
	producer := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext, "test-placeholder")
	if producer == nil {
		t.Fatal("real current binding did not admit producer")
	}
	_, err := provider.NewHTTPProviderClient(fake.Client()).Stream(context.Background(), provider.Request{
		ProviderID: "anthropic", Family: "anthropic", EndpointFormat: "messages", BaseURL: fake.URL, APIKey: "test-placeholder", Model: "claude-test",
		Messages: []provider.Message{{Role: "user", Content: "synthetic seam prompt"}},
		OnChunk: func(chunk provider.Chunk) error {
			if chunk.ProcessFrame != nil {
				producer.FrameV1(*chunk.ProcessFrame)
			}
			return nil
		},
	})
	producer.FinishV1()
	if err != nil || requests != 1 {
		t.Fatalf("native SSE adapter failed: %v requests=%d", err, requests)
	}
	if authorized := processSeamAuthorizeV1(t, h, pending, processSeamViewerV1); authorized.Status != "authorized" {
		t.Fatalf("native current-own authorization=%s", authorized.Status)
	}
	collapsed, _ := processSeamReadV1(t, h, processSeamViewerV1)
	if collapsed.Status != "snapshot" || len(collapsed.Blocks) != 2 {
		t.Fatal("actual empty/source native boundaries were replaced or invented")
	}
	ids := []string{collapsed.Blocks[0].BlockID, collapsed.Blocks[1].BlockID}
	expanded, raw := processSeamReadV1(t, h, processSeamViewerV1, ids...)
	if expanded.Status != "snapshot" || len(expanded.Blocks) != 2 {
		t.Fatal("native expansion unavailable")
	}
	for index, want := range []struct {
		native int
		text   string
	}{{3, ""}, {8, thinking}} {
		block := expanded.Blocks[index]
		if block.Kind != "native_reasoning" || block.State != "completed" || block.NativeIndex == nil || *block.NativeIndex != want.native || block.Body == nil || block.Body.Text != want.text {
			t.Fatal("native plaintext/source index did not survive true SSE producer")
		}
		assertProcessSeamPublicIsolationV1(t, h, pending, block, thinking, signature, redacted)
	}
	if strings.Contains(raw, signature) || strings.Contains(raw, redacted) {
		t.Fatal("opaque native protocol became display text")
	}
	// No tool result exists here. A native reasoning read must not invent a tool
	// grant to obtain source/current viewer authority.
	h.turnSecurity.Identity = nil
	denied, denial := processSeamReadV1(t, h, processSeamViewerV1, ids...)
	if denied.Status != "unavailable" || strings.Contains(denial, "blocks") || strings.Contains(denial, thinking) {
		t.Fatal("revoked current principal released native body")
	}
}

func TestProcessDisplayActualToolSettlementShellAndMutationProtectedHTTP(t *testing.T) {
	for _, tool := range []string{"bash", "write", "edit"} {
		t.Run(tool, func(t *testing.T) {
			args := map[string]any{"path": "code.txt", "content": "after private canary\r\n"}
			if tool == "edit" {
				args = map[string]any{"path": "code.txt", "oldText": "before", "newText": "after private canary"}
			}
			if tool == "bash" {
				args = map[string]any{"command": "printf 'process shell private canary\\r\\nexitCode=0\\n'; exit 7"}
			}
			h, pending, _, _ := protectedSeamFixtureV1(t, tool, args)
			pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
			if tool != "bash" {
				if err := os.WriteFile(filepath.Join(pending.Workspace, "code.txt"), []byte("before\r\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			pending.ApprovalPolicy = "auto"
			if tool == "edit" {
				output, isError, blocked := h.runtimeToolPreflight(pending)
				outputMap, _ := output.(map[string]any)
				if !blocked || !isError || stringField(outputMap, "code") != "read_before_edit_required" {
					t.Fatal("Edit fixture did not require its actual current-turn read")
				}
				processSeamActualReadBeforeEditV1(t, h, pending)
			}
			if _, isError, blocked := h.runtimeToolPreflight(pending); blocked || isError {
				t.Fatal("actual tool preflight rejected the prepared fixture")
			}
			settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			block := processSeamOwnBlockV1(t, h, pending)
			if block.DurationMs < 0 || block.Classification != "source_exact_local" {
				t.Fatal("actual tool capture lost host timing or classification")
			}
			if tool == "bash" {
				if !settled.IsError || block.Kind != "shell" || block.State != "failed" || block.ExitCode == nil || *block.ExitCode != 7 || block.Body.Text != "process shell private canary\r\nexitCode=0\n" {
					t.Fatal("shell outcome was inferred from body or source rerun")
				}
				assertProcessSeamPublicIsolationV1(t, h, pending, block, "process shell private canary")
			} else {
				actual, err := os.ReadFile(filepath.Join(pending.Workspace, "code.txt"))
				if err != nil || settled.IsError || block.Kind != tool || block.State != "completed" || block.Path != filepath.Join(pending.Workspace, "code.txt") || block.EffectApplied == nil || !*block.EffectApplied || block.Body.Before != "before\r\n" || block.Body.After != string(actual) || block.Body.AddedLines != 1 || block.Body.RemovedLines != 1 || block.Body.Approximate || !strings.Contains(block.Body.Text, "-before\r") || !strings.Contains(block.Body.Text, "+after private canary\r") {
					t.Fatal("mutation display did not freeze actual path/effect/diff")
				}
				assertProcessSeamPublicIsolationV1(t, h, pending, block, "after private canary", "before\\r\\n")
				if err := os.WriteFile(filepath.Join(pending.Workspace, "code.txt"), []byte("later replacement"), 0600); err != nil {
					t.Fatal(err)
				}
				expanded, _ := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
				if expanded.Status != "snapshot" || expanded.Blocks[0].Body.After != string(actual) {
					t.Fatal("later source edit replaced frozen actual after image")
				}
			}
			message, err := json.Marshal(settled.Message)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"captureDigest", "displayCapture", "processDisplay", block.BlockID, block.CaptureDigest} {
				if forbidden != "" && strings.Contains(string(message), forbidden) {
					t.Fatal("display copy entered continuation model message")
				}
			}
		})
	}
}

func TestProcessDisplaySettledGrantLossAndCurrentCaseDriftDenyExpandedCopy(t *testing.T) {
	for _, loss := range []string{"settled_grant", "case_binding"} {
		t.Run(loss, func(t *testing.T) {
			h, pending, _, _ := protectedSeamFixtureV1(t, "write", map[string]any{"path": "code.txt", "content": "private settled canary\n"}, loss == "case_binding")
			pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
			if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err != nil {
				t.Fatal(err)
			}
			block := processSeamOwnBlockV1(t, h, pending)
			if loss == "settled_grant" {
				snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
				if err != nil {
					t.Fatal(err)
				}
				altered := cloneMap(snapshot.Thread)
				delete(protectedSeamResultV1(t, altered, pending), "executionGrantId")
				if err := h.store.ReplaceThreadForAuthorityRepair(pending.ThreadID, altered); err != nil {
					t.Fatal(err)
				}
			} else {
				path := filepath.Join(pending.Workspace, ".analytix", "case-project.json")
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				after := bytes.Replace(before, []byte("case_writer_barrier"), []byte("case_successor"), 1)
				if bytes.Equal(before, after) {
					t.Fatal("fixture did not contain actual case binding")
				}
				if err := os.WriteFile(path, after, 0600); err != nil {
					t.Fatal(err)
				}
			}
			writes := h.store.WriteAttempts()
			denied, raw := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
			if denied.Status != "unavailable" || strings.Contains(raw, "block") || strings.Contains(raw, "private settled canary") || h.store.WriteAttempts() != writes {
				t.Fatal("fresh authority loss disclosed copy or mutated primary")
			}
		})
	}
}

func TestProcessDisplayOwnSideAndSignedChildKeepRealHistoryAuthority(t *testing.T) {
	h, parent, _, _ := protectedSeamFixtureV1(t, "write", map[string]any{"path": "code.txt", "content": "parent process private\n"})
	parent.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil {
		t.Fatal(err)
	}
	side, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "side"})
	if err != nil {
		t.Fatal(err)
	}
	inherited := parent
	inherited.ThreadID = stringField(side, "id")
	if response := processSeamAuthorizeV1(t, h, inherited, "f52bf6cd-fb55-4132-a571-d759527a4bbc"); response.Status != "unavailable" {
		t.Fatal("fork inherited parent display granted side source authority")
	}
	own := protectedSeamPendingV1(t, h, side, "write", map[string]any{"path": "code.txt", "content": "side process own\n"}, "turn_process_side_own")
	own.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), own, nil, nil); err != nil {
		t.Fatal(err)
	}
	block := processSeamOwnBlockV1(t, h, own)
	if block.Body.After != "side process own\n" {
		t.Fatal("own side capture unavailable under real inherited-history validation")
	}
	processSeamRequestV1(t, h, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "revoke", "viewerToken": processSeamViewerV1})
	h.subagents = subagentapp.ProfileSettings{Enabled: true, DefaultToolPolicy: "inherit", MaxParallel: 2, MaxChildRuns: 8, Profiles: map[string]subagentapp.ProfileConfig{"reviewer": {Name: "reviewer", ToolPolicy: "inherit", MaxSteps: 4, MaxStepsSet: true}}}
	current, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), own.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	task := protectedSeamPendingV1(t, h, current.Thread, "task", map[string]any{"prompt": "inspect synthetic only", "profile": "reviewer", "run_in_background": false}, "turn_process_side_task")
	task.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	recorder := &providerStepRecordingProvider{}
	h.provider = recorder
	if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), task.SecurityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), task, nil, nil)
	if err != nil || settled.IsError {
		t.Fatal("real task did not settle signed child")
	}
	inventory, err := h.pendingWork.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	childID := ""
	for _, receipt := range inventory.Receipts {
		if receipt.ChildProducer != nil {
			for _, child := range receipt.ChildProducer.Children {
				childID = child.ChildThreadID
			}
		}
	}
	if childID == "" || len(recorder.Requests()) != 1 {
		t.Fatal("task fixture did not produce exactly one real signed child")
	}
	child, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), childID)
	if err != nil {
		t.Fatal(err)
	}
	// The actual signed child inherits its parent's ordinary tool capability.
	// Its own settled Shell is an isolated inspection with its own host grant.
	childOwn := protectedSeamPendingV1(t, h, child.Thread, "bash", map[string]any{"command": "printf 'signed-child process own\\n'"}, "turn_process_child_own")
	childOwn.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), childOwn, nil, nil); err != nil {
		t.Fatal(err)
	}
	childToken := "4e3d4658-5f1f-4f1d-b019-5fd6e5bf0251"
	if response := processSeamAuthorizeV1(t, h, childOwn, childToken); response.Status != "authorized" {
		t.Fatalf("signed child own process authority=%s", response.Status)
	}
	collapsed, _ := processSeamReadV1(t, h, childToken)
	if collapsed.Status != "snapshot" || len(collapsed.Blocks) != 1 {
		t.Fatal("signed child actual tool capture absent")
	}
	expanded, _ := processSeamReadV1(t, h, childToken, collapsed.Blocks[0].BlockID)
	if expanded.Status != "snapshot" || expanded.Blocks[0].Body == nil || expanded.Blocks[0].Body.Text != "signed-child process own\n" {
		t.Fatal("signed child own capture was overfiltered")
	}
}

// Issue, persist, authorize, execute and settle a distinct real Read in the
// same frozen turn. This never fabricates a read-registry record or output.
func processSeamActualReadBeforeEditV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall) {
	t.Helper()
	arguments := map[string]any{"path": "code.txt"}
	raw, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	read := pending
	read.Call = provider.ToolCall{ID: serverTestHostToolCallID("process_actual_read_before_edit"), Name: "read", Arguments: raw}
	read.ToolCallItemID = domaintoolcall.ToolCallItemIDV1(read.TurnID, read.Call.ID)
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, read.ToolScope, false, false, read.Prompt, false, nil)
	read.ExecutionGrant, err = executiongrantapp.IssueProvider(read.SecurityContext, read.ProviderID, read.Call, schemas, read.ToolScope, true, "not_required", 0, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.persistToolCallReady(context.Background(), read.ThreadID, read.TurnID, read.Call, 1, read.SecurityContext, read.ExecutionGrant); err != nil {
		t.Fatal(err)
	}
	if err := h.authorizeRuntimePending(context.Background(), read, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), read, nil, nil)
	if err != nil || settled.IsError {
		t.Fatal("actual same-turn Read did not settle")
	}
	if _, isError, blocked := h.runtimeToolPreflight(pending); blocked || isError {
		t.Fatal("same-turn real Read failed the existing Edit guard")
	}
}

// Restore happens on the same loop goroutine after actual HTTP/SSE parsing.
// A RoundTripper hook cannot observe this path: current proxy authority creates
// its own transport. The real client's durable pipeline/currentness stay intact.
type processRestoreDisplayProviderV1 struct {
	base     *provider.HTTPProviderClient
	onResult func()
	once     sync.Once
}

func (*processRestoreDisplayProviderV1) RequiresDurablePipelineStagesV1() {}
func (client *processRestoreDisplayProviderV1) Stream(ctx context.Context, request provider.Request) (provider.Result, error) {
	result, err := client.base.Stream(ctx, request)
	if err == nil && client.onResult != nil {
		client.once.Do(client.onResult)
	}
	return result, err
}
func (client *processRestoreDisplayProviderV1) Close() error { return client.base.Close() }

// Actual PrepareProviderAttempt, HTTP/SSE tool parser, signed host tool grant,
// Bash execution, real settlement, and protected read. The existing seam's
// static test resolver provides the key; production Registry is covered by the
// runtimeapp regression. There are deliberately no native process frames.
func TestProcessDisplayPreparedCredentialWithoutNativeFramesAndLateToolProducer(t *testing.T) {
	for _, initiallyUnavailable := range []bool{false, true} {
		name := "display_available"
		if initiallyUnavailable {
			name = "initial_producer_unavailable"
		}
		t.Run(name, func(t *testing.T) {
			const preparedKey = "p5Af8M0n4Q6r9T2v7W1x3Y5z8C0d2F4g6H9j1K3m"
			const initialKey = "b3Cd6F8g0H2j4K7m9N1p5Q8r0T2v4W6x9Y1z3A5c"
			var sends atomic.Int64
			var mismatchedKey atomic.Bool
			arguments := map[string]any{"command": "printf '%s\\n' '" + preparedKey + "'"}
			commandArgs, _ := json.Marshal(arguments)
			h, pending, _, _ := protectedSeamFixtureV1(t, "bash", arguments)
			pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
			pending.ApprovalPolicy = "auto"
			if _, isError, blocked := h.runtimeToolPreflight(pending); blocked || isError {
				t.Fatal("legitimate host Shell policy rejected fixture")
			}
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := sends.Add(1)
				if r.Header.Get("Authorization") != "Bearer "+preparedKey {
					mismatchedKey.Store(true)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				var chunk map[string]any
				if call == 1 {
					chunk = map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "wire_prepared_only", "type": "function", "function": map[string]any{"name": "bash", "arguments": string(commandArgs)}}}}, "finish_reason": "tool_calls"}}}
				} else {
					chunk = map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "synthetic complete"}, "finish_reason": "stop"}}}
				}
				encoded, _ := json.Marshal(chunk)
				_, _ = w.Write(append(append([]byte("data: "), encoded...), []byte("\n\ndata: [DONE]\n\n")...))
			}))
			defer fake.Close()
			h.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{DefaultProviderID: "synthetic", DefaultBaseURL: fake.URL + "/v1", DefaultAPIKey: preparedKey, DefaultEndpointFormat: "chat_completions", DefaultModel: "synthetic"})
			installHandlerProviderExecutionResolverForTest(h)
			owner := h.processDisplay
			defer func() { h.processDisplay = owner }()
			if initiallyUnavailable {
				h.processDisplay = nil
			}
			restored := false
			client := &processRestoreDisplayProviderV1{base: provider.NewHTTPProviderClient(fake.Client())}
			defer client.Close()
			if initiallyUnavailable {
				client.onResult = func() { h.processDisplay = owner; restored = true }
			}
			h.provider = client
			thread, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			initial, err := h.resolveRuntimeTurnIntent(context.Background(), provider.TurnExecutionInput{RequestProviderID: "synthetic", RequestModel: "synthetic", ThreadProviderID: "synthetic", ThreadModel: "synthetic"})
			if err != nil {
				t.Fatal(err)
			}
			initial.Config.APIKey = initialKey
			limit := 3
			input := runtimeAgentLoopInput{ThreadID: pending.ThreadID, TurnID: pending.TurnID, Thread: thread.Thread, ProviderConfig: initial.Config, ProviderID: "synthetic", Model: "synthetic", Workspace: pending.Workspace,
				ApprovalPolicy: pending.ApprovalPolicy, SandboxMode: pending.SandboxMode, ToolScope: pending.ToolScope, SecurityContext: pending.SecurityContext,
				Request: startRuntimeTurnRequest{Prompt: "Run a synthetic shell command", DisableUserInput: true, MaxModelSteps: &limit}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !h.runtimeControl().RegisterTurnCancel(pending.ThreadID, pending.TurnID, cancel) {
				t.Fatal("loop ownership registration failed")
			}
			defer h.runtimeControl().UnregisterTurnCancel(pending.ThreadID, pending.TurnID)
			if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(ctx, pending.SecurityContext, time.Second); err != nil {
				t.Fatal(err)
			}
			result, err := h.runRuntimeAgentLoopWithMessages(ctx, input, []provider.Message{{Role: "system", Content: "synthetic ordinary shell check"}, {Role: "user", Content: input.Request.Prompt}})
			if result.ReleaseCandidateTerminal != nil {
				result.ReleaseCandidateTerminal()
			}
			if err != nil || sends.Load() != 2 || mismatchedKey.Load() {
				t.Fatal("prepared-key real HTTP/tool sequence failed")
			}
			if initiallyUnavailable && (!restored || h.processDisplay != owner) {
				t.Fatal("initially unavailable process owner was never restored after the real send")
			}
			target := processSeamAssertActualSettledShellV1(t, h, pending, preparedKey)
			block := processSeamOwnBlockV1(t, h, pending, target)
			if block.Kind != "shell" || block.State != "completed" || block.Classification != "credential_redacted" || block.Body.Text != "<redacted>\n" {
				t.Fatal("bare prepared key survived Shell display without native frames")
			}
			expanded, raw := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
			if expanded.Status != "snapshot" || strings.Contains(raw, preparedKey) || strings.Contains(raw, initialKey) {
				t.Fatal("prepared credential entered protected display JSON")
			}
			assertProcessSeamPublicIsolationV1(t, h, pending, block, preparedKey, initialKey)
		})
	}
}

// Verify physical Shell capture and exact signed settlement before reading the
// process sidecar. Successful Provider sends alone do not prove tool execution.
// All raw body comparisons remain inside this isolated fixture; errors contain
// fixed labels only, never command/body/credential data.
func processSeamAssertActualSettledShellV1(t *testing.T, h *runtimeServerHandler, scope runtimePendingToolCall, expectedBody string) domainprocess.CallAnchorV2 {
	t.Helper()
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), scope.ThreadID)
	if err != nil {
		t.Fatal("actual Shell primary unavailable")
	}
	var call, result map[string]any
	for _, rawTurn := range listAny(snapshot.Thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != scope.TurnID {
			continue
		}
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if stringField(item, "kind") == "tool_result" && stringField(item, "toolName") == "bash" {
				if result != nil {
					t.Fatal("actual Shell result was ambiguous")
				}
				result = item
			}
		}
		if result == nil {
			continue
		}
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if stringField(item, "kind") == "tool_call" && stringField(item, "callId") == stringField(result, "callId") {
				if call != nil {
					t.Fatal("actual Shell call was ambiguous")
				}
				call = item
			}
		}
	}
	if call == nil || result == nil {
		t.Fatal("Provider returned without actual settled Shell")
	}
	isError, isErrorOK := result["isError"].(bool)
	_, closed := domaintoolresult.PrivateDurableToolResultItemRecordV1(result)
	if !closed || !isErrorOK || isError || stringField(result, "status") != "completed" || stringField(call, "status") != "completed" || stringField(call, "finishedAt") == "" || stringField(call, "finishedAt") != stringField(result, "finishedAt") {
		t.Fatal("actual Shell did not produce a closed successful exact lifecycle")
	}
	registry, err := executiongrantapp.RegistryFromThread(scope.ThreadID, snapshot.Thread, scope.TurnID)
	if err != nil {
		t.Fatal("actual Shell signed grant registry unavailable")
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, stringField(result, "executionGrantId"))
	if !found || entry.Status != domainsecurity.GrantRegistrySettled || entry.Grant.ToolName != "bash" || entry.Grant.ToolCallID != stringField(result, "callId") || executiongrantapp.VerifyThreadGrantMembership(scope.ThreadID, snapshot.Thread, scope.TurnID, entry.Grant, domainsecurity.GrantRegistrySettled) != nil {
		t.Fatal("actual Shell result had no settled signed host grant")
	}
	binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(result[domaintoolresult.ProtectedSnapshotBindingFieldV1])
	if err != nil {
		t.Fatal("actual Shell producer did not seal physical capture")
	}
	captured, err := h.toolSnapshots.Read(context.Background(), binding)
	if err != nil || captured.ThreadID != scope.ThreadID || captured.TurnID != scope.TurnID || captured.CallID != stringField(result, "callId") || captured.ToolName != "bash" || captured.Capture.Kind != "shell" || captured.Capture.Status != "completed" || captured.Capture.ExitCode == nil || *captured.Capture.ExitCode != 0 || captured.Capture.Body != expectedBody+"\n" {
		t.Fatal("actual Shell physical status or complete output differed")
	}
	return domainprocess.CallAnchorV2{CallID: stringField(result, "callId"), ToolCallItemID: stringField(call, "id"), ToolResultItemID: stringField(result, "id")}
}

// This fixture uses the real component constructor and current signed scope.
// It neither installs a replacement service nor substitutes read authority.
func processSeamCurrentServiceFixtureV1(t *testing.T, tool string, arguments map[string]any) (*runtimeServerHandler, runtimePendingToolCall) {
	t.Helper()
	h, pending, _, _ := protectedSeamFixtureV1(t, tool, arguments)
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	owner := h.processDisplay
	if owner == nil {
		t.Fatal("real component fixture has no process display owner")
	}
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error("process fixture shutdown failed")
		}
	})
	return h, pending
}

// All principal, context, history and exact settlement checks stay on the real
// access authority. The observer signals only after its actual lease release.
type processSeamPendingReadAuthorityV1 struct {
	runtimeProcessDisplayAccessV1
	armed    atomic.Bool
	released chan struct{}
	once     sync.Once
}

func (a *processSeamPendingReadAuthorityV1) AcquireCurrent(ctx context.Context, binding processport.BindingV1) (processport.AuthorizedV1, func(), error) {
	authorized, release, err := a.runtimeProcessDisplayAccessV1.AcquireCurrent(ctx, binding)
	if err != nil || release == nil {
		return authorized, release, err
	}
	return authorized, func() {
		release()
		if a.armed.Load() {
			a.once.Do(func() { close(a.released) })
		}
	}, nil
}

func TestProcessDisplayDirectCoreShutdownRejectsFreshReadsAndCapture(t *testing.T) {
	h, pending := processSeamCurrentServiceFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	if err := h.processDisplay.Close(); err != nil {
		t.Fatal("empty component display owner could not retire")
	}
	limits := processapp.DefaultLimitsV1()
	limits.AuthorityCheckInterval = time.Second
	authority := &processSeamPendingReadAuthorityV1{
		runtimeProcessDisplayAccessV1: runtimeProcessDisplayAccessV1{handler: h},
		released:                      make(chan struct{}),
	}
	owner := processapp.NewServiceV1(authority, limits)
	h.processDisplay = owner
	t.Cleanup(func() { _ = owner.Close() })
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(context.Background(), h.turnSecurity.Identity)
	if err != nil {
		t.Fatal("current fixture principal unavailable")
	}
	binding := processport.BindingV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, Principal: principal, Original: pending.SecurityContext}
	probe := owner.BeginV1(binding)
	if probe == nil {
		t.Fatal("actual owner rejected its valid current binding before shutdown")
	}
	probe.FinishV1()
	const canary = "private-native-shutdown-canary"
	producer := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if producer == nil {
		t.Fatal("current native producer could not acquire its own scope")
	}
	for _, frame := range []domainmodel.ProcessFrameV1{
		{Kind: "start", Attempt: 1, NativeIndex: 0, NativeID: "synthetic_shutdown", ProviderSequence: 1},
		{Kind: "delta", Attempt: 1, NativeIndex: 0, NativeID: "synthetic_shutdown", Text: canary, ProviderSequence: 1},
		{Kind: "end", Attempt: 1, NativeIndex: 0, NativeID: "synthetic_shutdown", ProviderSequence: 1},
	} {
		producer.FrameV1(frame)
	}
	producer.FinishV1()
	block := processSeamOwnBlockV1(t, h, pending)
	if block.Kind != "native_reasoning" || block.Body == nil || block.Body.Text != canary {
		t.Fatal("completed native capture was not readable before shutdown")
	}
	before := owner.ReadV1(context.Background(), processSeamViewerV1, "", 0, nil)
	if before.Status != "snapshot" || before.Revision == "" {
		t.Fatal("current viewer has no stable snapshot revision")
	}
	pollContext, cancelPoll := context.WithCancel(context.Background())
	defer cancelPoll()
	polled := make(chan processapp.ResponseV1, 1)
	// Register the listener and arm the observer before starting the read.
	// ReadV1 captures both broadcast channels under its owner mutex, unlocks,
	// then calls this actual release. A close before select still wakes it.
	authority.armed.Store(true)
	go func() {
		polled <- owner.ReadV1(pollContext, processSeamViewerV1, before.Revision, 25*time.Second, []string{block.BlockID})
	}()
	select {
	case <-authority.released:
	case <-polled:
		t.Fatal("pending display read returned before waiting")
	case <-time.After(2 * time.Second):
		t.Fatal("pending display read never released current authority")
	}
	select {
	case <-polled:
		t.Fatal("display read was not pending before shutdown")
	default:
	}
	if owner.ReadV1(context.Background(), processSeamViewerV1, before.Revision, 0, nil).Status != "busy" {
		t.Fatal("viewer did not retain its sole pending poll")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	shutdown := make(chan error, 1)
	go func() { shutdown <- h.Shutdown(ctx) }()
	select {
	case response := <-polled:
		raw, marshalErr := json.Marshal(response)
		if marshalErr != nil || string(raw) != `{"status":"unavailable"}` {
			t.Fatal("closed owner released private data to its pending poll")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("owner close did not wake pending read before authority recheck")
	}
	select {
	case err := <-shutdown:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("direct core shutdown did not finish its safety cleanup")
	}
	if !h.runtimeControl().ShuttingDown() {
		t.Fatal("direct core shutdown did not close runtime admission")
	}
	// BeginV1 has no host currentness callback. This independently proves the
	// captured service closed, rather than merely a fresh host check denying it.
	if unexpected := owner.BeginV1(binding); unexpected != nil {
		unexpected.FinishV1()
		t.Fatal("direct core shutdown left its actual display owner open")
	}
	assertUnavailable := func(response processapp.ResponseV1, raw string) {
		t.Helper()
		if response.Status != "unavailable" || strings.TrimSpace(raw) != `{"status":"unavailable"}` || strings.Contains(raw, canary) {
			t.Fatal("direct core shutdown left private process read authority available")
		}
	}
	response, raw := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
	assertUnavailable(response, raw)
	response, raw = processSeamRequestV1(t, h, httpapi.ProcessLocalDisplayTargetPathV1, map[string]any{"schemaVersion": 2, "action": "open_capture", "viewerToken": processSeamViewerV1, "blockId": block.BlockID, "captureDigest": block.CaptureDigest})
	assertUnavailable(response, raw)
	response, raw = processSeamRequestV1(t, h, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "authorize", "viewerToken": "22222222-2222-4222-8222-222222222222", "documentNonce": processSeamDocumentV1, "threadId": pending.ThreadID, "turnId": pending.TurnID})
	assertUnavailable(response, raw)
	if h.beginProcessDisplayV1(context.Background(), pending.SecurityContext) != nil {
		t.Fatal("direct core shutdown accepted a new private process producer")
	}
}

// Only the atomic primary callback is faulted. The actual tool runner, signed
// grant/effect lease, side-effect intent and settlement preparation stay real.
// Storage replay reuses one frozen result and cannot re-enter the file runner.
func TestProcessDisplayOrdinaryOwnerCommitACKLossReplayAndStrictClosureV1(t *testing.T) {
	h, pending, durable, _ := protectedSeamFixtureV1(t, "write", map[string]any{"path": "exact-owner.txt", "content": "effect once\n"})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	pending.ApprovalPolicy = "auto"
	if _, isError, blocked := h.runtimeToolPreflight(pending); blocked || isError {
		t.Fatal("actual Write preflight rejected the signed fixture")
	}
	ctx := domainprocess.WithCollectorV1(context.Background(), &domainprocess.CollectorV1{})
	var frozen map[string]any
	var captured domainprocess.RawCaptureV1
	effects, primaryCommits, atomicCallbacks := 0, 0, 0
	atomicWrite := func(write func() error) error {
		atomicCallbacks++
		if err := write(); err != nil {
			return err
		}
		primaryCommits++
		return nil
	}
	settled, err := apploop.ExecuteAndSettleWithSideEffectIntent(apploop.SideEffectIntentExecutionInput{
		Context: ctx, Pending: pending, PendingWork: h.pendingWork,
		Acquire: func(effectCtx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return h.acquireRuntimePendingToolEffect(effectCtx, pending)
		},
		Execute: func(effectCtx context.Context, call runtimePendingToolCall, override any) (any, bool) {
			effects++
			return h.executeRuntimeToolWithEffectAuthority(effectCtx, call, override)
		},
		Settle: func(effectCtx context.Context, call runtimePendingToolCall, output any, isError bool) (apploop.SettledToolExecution, error) {
			attempt := apploop.CaptureToolResultProviderAttemptV1(apploop.ToolResultProviderAttemptInputV1{
				Pending: call, Output: output, IsError: isError, Config: h.visionBridge, Source: h.mcp,
				ProjectExact:       subagentapp.ProjectForegroundParentToolOutputV1,
				ProjectExactPublic: subagentapp.ProjectForegroundParentPublicToolOutputV1,
			})
			defer attempt.DiscardPrivate()
			settlement, err := evidenceapp.PrepareToolSettlement(effectCtx, h.caseFinalizer, h.turnSecurity, filestore.CaseBindingReader{}, h.store, h.mcp, call, output, isError)
			if err != nil {
				return apploop.SettledToolExecution{}, err
			}
			if !settlement.SettlementAuthorized || settlement.IsError || !domainprocess.MutationCaptureReadyV1(effectCtx, call.Call.Name) {
				return apploop.SettledToolExecution{}, errors.New("actual Write settlement was not authorized with its producer capture")
			}
			prepared, err := attempt.PrepareSettlementV1(settlement.Output, settlement.IsError)
			if err != nil {
				return apploop.SettledToolExecution{}, err
			}
			stamp := time.Now().UTC().Format(time.RFC3339Nano)
			records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
				ThreadID: call.ThreadID, TurnID: call.TurnID, CreatedAt: stamp, FinishedAt: stamp,
				Call: call.Call, Projection: prepared.Projection, IsError: settlement.IsError,
				ContextDigest: call.SecurityContext.ContextDigest, ContextEpoch: call.SecurityContext.ContextEpoch,
				ExecutionGrantID: call.ExecutionGrant.GrantID, CaseID: call.SecurityContext.CaseID,
				DatasetSnapshotID: call.SecurityContext.DatasetSnapshotID, ServerIdentity: call.ExecutionGrant.ServerIdentity,
				PrivateCaseOutcome: privateCaseToolOutcomeForSettlement(prepared.Projection, settlement.Output), EvidenceSettlement: settlement.Marker,
			})
			if err != nil {
				return apploop.SettledToolExecution{}, err
			}
			// One prepared result map; neither readback nor replay obtains a new timestamp.
			frozen = cloneMap(records.ResultItem)
			for _, test := range []struct {
				name   string
				mutate func(map[string]any)
			}{
				{"unknown_root", func(item map[string]any) { item["unrecognized"] = true }},
				{"raw_arguments", func(item map[string]any) { item["arguments"] = map[string]any{"path": "raw canary"} }},
				{"raw_body", func(item map[string]any) { item["body"] = "raw canary" }},
				{"raw_nested_output", func(item map[string]any) { item["output"].(map[string]any)["content"] = "raw canary" }},
				{"json_encoding_failure", func(item map[string]any) { item["createdAt"] = make(chan struct{}) }},
				{"invalid_utf8_created_at", func(item map[string]any) { item["createdAt"] = string([]byte{0xff}) }},
				{"invalid_utf8_known_nested_string", func(item map[string]any) {
					// Otherwise valid closed nested metadata. Marshal repairs this byte;
					// the owner must reject the original, before its first primary write.
					item["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.PublicToolResultProjectionV1{
						SchemaVersion: 1, ProjectionKind: domaintoolresult.ProjectionPlanStatus, Disclosure: domaintoolresult.MetadataOnlyDisclosure,
						MessageKey: "plan_updated", Status: "completed", PrivatePayloadWithheld: true,
						Plan: &domaintoolresult.PlanStatusV1{PlanID: "plan_synthetic", RelativePath: "plan.md", Operation: "draft", ContentHash: strings.Repeat("a", 64), SavedAt: stamp},
					})
					item["output"].(map[string]any)["plan"].(map[string]any)["planId"] = string([]byte{0xff})
				}},
			} {
				t.Run(test.name, func(t *testing.T) {
					candidate := cloneMap(frozen)
					test.mutate(candidate)
					original := processSeamCopyOriginalV1(candidate)
					before, info := processSeamPrimaryFileV1(t, h.store, call.ThreadID)
					callbacksBefore, commitsBefore := atomicCallbacks, primaryCommits
					err := h.store.ensureClosedToolResultSettlementExact(effectCtx, call.ThreadID, call.TurnID, call.ToolCallItemID, candidate, atomicWrite)
					if err == nil || atomicCallbacks != callbacksBefore || primaryCommits != commitsBefore || !reflect.DeepEqual(candidate, original) {
						t.Fatal("invalid original record was repaired, accepted or reached the physical primary writer")
					}
					processSeamAssertPrimaryFileV1(t, h.store, call.ThreadID, before, info)
				})
			}
			before, info := processSeamPrimaryFileV1(t, h.store, call.ThreadID)
			bound := cloneMap(frozen)
			bound[domaintoolresult.ProtectedSnapshotBindingFieldV1] = nil
			if err := h.store.EnsureOrdinaryToolResultSettlementExact(effectCtx, call.ThreadID, call.TurnID, call.ToolCallItemID, bound); err == nil {
				return apploop.SettledToolExecution{}, errors.New("ordinary owner accepted explicit null snapshot binding")
			}
			if err := h.store.EnsureProtectedToolResultSettlementExact(effectCtx, call.ThreadID, call.TurnID, call.ToolCallItemID, frozen); err == nil {
				return apploop.SettledToolExecution{}, errors.New("protected owner accepted an ordinary result without its mandatory binding")
			}
			processSeamAssertPrimaryFileV1(t, h.store, call.ThreadID, before, info)
			err = h.store.ensureClosedToolResultSettlementExact(effectCtx, call.ThreadID, call.TurnID, call.ToolCallItemID, frozen, func(write func() error) error {
				if err := atomicWrite(write); err != nil {
					return err
				}
				return errors.New("synthetic acknowledgement loss after actual primary commit")
			})
			if err != nil {
				return apploop.SettledToolExecution{}, err
			}
			var present bool
			captured, present = domainprocess.ConsumeV1(effectCtx)
			if !present {
				return apploop.SettledToolExecution{}, errors.New("persisted actual Write capture was lost")
			}
			return apploop.SettledToolExecution{Output: prepared.PublicOutput, IsError: settlement.IsError, Message: prepared.Message}, nil
		},
		Prepare: h.prepareRuntimeSideEffect, ValidateApproved: h.validateRuntimeApprovedTransition,
		ResolvedEffectFree: h.runtimeRunSkillIsInline, Now: func() time.Time { return time.Now().UTC() }, AccountFlowSource: h.mcp,
	})
	if err != nil || settled.IsError || effects != 1 || primaryCommits != 1 || atomicCallbacks != 1 {
		t.Fatalf("actual commit/ACK-loss readback failed: error=%v effects=%d primaryCommits=%d atomicCallbacks=%d", err, effects, primaryCommits, atomicCallbacks)
	}
	if captured.Kind != "write" || !captured.Complete || captured.Status != "completed" || captured.EffectApplied == nil || !*captured.EffectApplied || captured.After != "effect once\n" {
		t.Fatal("the actual file producer did not prove its completed effect")
	}
	path := filepath.Join(pending.Workspace, "exact-owner.txt")
	if body, err := os.ReadFile(path); err != nil || string(body) != "effect once\n" {
		t.Fatal("actual Write effect is absent")
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil || !bytes.Equal(processSeamResultJSONBytesV1(t, protectedSeamResultV1(t, snapshot.Thread, pending)), processSeamResultJSONBytesV1(t, frozen)) {
		t.Fatal("ACK-loss readback differs from the one frozen result")
	}
	before, info := processSeamPrimaryFileV1(t, h.store, pending.ThreadID)
	for replay := 0; replay < 2; replay++ {
		if err := h.store.ensureClosedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, frozen, atomicWrite); err != nil {
			t.Fatal(err)
		}
	}
	// A new store must have its actual composition-owned anchored primary reader.
	// This establishes owner/primary reopen, not a full native app restart.
	reopened, err := NewTempDurableEventSessionStore(durable)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetCaseThreadAuthority(h.caseThreads)
	if err := reopened.BindPrimaryThreadReaderV1(h.store.primaryReader); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ensureClosedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, frozen, atomicWrite); err != nil {
		t.Fatal(err)
	}
	readback, err := reopened.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil || readback.ThreadFileSHA256 != snapshot.ThreadFileSHA256 || !bytes.Equal(processSeamResultJSONBytesV1(t, protectedSeamResultV1(t, readback.Thread, pending)), processSeamResultJSONBytesV1(t, frozen)) {
		t.Fatal("bound primary reopen did not read the original committed record")
	}
	if primaryCommits != 1 || atomicCallbacks != 1 {
		t.Fatal("exact replay invoked the primary writer")
	}
	processSeamAssertPrimaryFileV1(t, h.store, pending.ThreadID, before, info)
	// Storage replay must leave a later external file edit intact.
	if err := os.WriteFile(path, []byte("later external edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.store.ensureClosedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, frozen, atomicWrite); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "later external edit\n" || effects != 1 || primaryCommits != 1 || atomicCallbacks != 1 {
		t.Fatal("storage replay repeated or replaced the completed file effect")
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"conflicting_timestamp", func(item map[string]any) { item["finishedAt"] = "2099-01-01T00:00:00Z" }},
		{"conflicting_status", func(item map[string]any) {
			item["status"] = "failed"
			item["isError"] = true
			item["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("failed", "tool_projection_invalid"))
		}},
		{"conflicting_output", func(item map[string]any) {
			item["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneMap(frozen)
			test.mutate(candidate)
			original := processSeamCopyOriginalV1(candidate)
			if err := h.store.ensureClosedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, candidate, atomicWrite); err == nil || !reflect.DeepEqual(candidate, original) {
				t.Fatal("same-ID conflict was accepted or repaired")
			}
			if primaryCommits != 1 || atomicCallbacks != 1 {
				t.Fatal("conflict reached the primary writer")
			}
			processSeamAssertPrimaryFileV1(t, h.store, pending.ThreadID, before, info)
		})
	}
	// Reproduce the old independent call timestamp owner; the exact owner must
	// reject these already-drifted bytes instead of silently repairing them.
	if err := h.store.PatchTurnItemStatus(pending.ThreadID, pending.TurnID, pending.ToolCallItemID, "completed"); err != nil {
		t.Fatal(err)
	}
	drifted, driftInfo := processSeamPrimaryFileV1(t, h.store, pending.ThreadID)
	if bytes.Equal(drifted, before) {
		t.Fatal("old timestamp drift was not actually established")
	}
	if err := h.store.ensureClosedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, frozen, atomicWrite); err == nil {
		t.Fatal("old lifecycle timestamp drift was repaired")
	}
	if primaryCommits != 1 || atomicCallbacks != 1 || effects != 1 {
		t.Fatal("drift denial wrote or reran the tool")
	}
	processSeamAssertPrimaryFileV1(t, h.store, pending.ThreadID, drifted, driftInfo)
}

func TestProcessDisplayOrdinaryOwnerRejectsActualProtectedBindingV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "bash", map[string]any{"command": "printf 'synthetic protected binding'"})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	pending.ApprovalPolicy = "auto"
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
	if err != nil || settled.IsError {
		t.Fatal("actual protected Shell settlement failed")
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	result := protectedSeamResultV1(t, snapshot.Thread, pending)
	if _, err := domaintoolresult.ParseProtectedSnapshotBindingV1(result[domaintoolresult.ProtectedSnapshotBindingFieldV1]); err != nil {
		t.Fatal("actual protected binding is absent")
	}
	before, info := processSeamPrimaryFileV1(t, h.store, pending.ThreadID)
	if err := h.store.EnsureOrdinaryToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, result); err == nil {
		t.Fatal("ordinary owner accepted the actual protected result")
	}
	if err := h.store.EnsureProtectedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, result); err != nil {
		t.Fatal("protected exact replay rejected its own mandatory binding")
	}
	processSeamAssertPrimaryFileV1(t, h.store, pending.ThreadID, before, info)
}

// JSON cloning would repair invalid UTF-8 or discard unencodable values, so
// retain their original Go bytes/types when checking that rejection is inert.
func processSeamCopyOriginalV1(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(typed))
		for key, entry := range typed {
			copy[key] = processSeamCopyOriginalV1(entry)
		}
		return copy
	case []any:
		copy := make([]any, len(typed))
		for index, entry := range typed {
			copy[index] = processSeamCopyOriginalV1(entry)
		}
		return copy
	default:
		return value
	}
}

func processSeamPrimaryFileV1(t *testing.T, store *DurableEventSessionStore, threadID string) ([]byte, os.FileInfo) {
	t.Helper()
	body, err := os.ReadFile(store.threadPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.threadPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	return body, info
}

func processSeamAssertPrimaryFileV1(t *testing.T, store *DurableEventSessionStore, threadID string, want []byte, info os.FileInfo) {
	t.Helper()
	got, current := processSeamPrimaryFileV1(t, store, threadID)
	if !bytes.Equal(got, want) || !current.ModTime().Equal(info.ModTime()) || !os.SameFile(current, info) {
		t.Fatal("rejected or replayed settlement physically replaced the primary file")
	}
}

// A frozen record must preserve the original canonical bytes before asking any
// primary authority. The unbound store gives a distinct error if reached.
func TestProcessDisplayOrdinaryOwnerRejectsLossyFreezeBeforePrimaryV1(t *testing.T) {
	item := map[string]any{"id": "result-synthetic", "kind": "tool_result", "role": "tool", "threadId": "thread-synthetic", "turnId": "turn-synthetic", "callId": "call-synthetic", "toolName": "write", "status": "completed", "createdAt": uint64(9007199254740993), "finishedAt": "2026-01-01T00:00:00Z", "isError": false, "output": domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"))}
	projected, closed := domaintoolresult.PrivateDurableToolResultItemRecordV1(item)
	originalBytes, originalErr := json.Marshal(item)
	canonicalBytes, canonicalErr := json.Marshal(projected)
	frozenBytes, frozenErr := json.Marshal(cloneMap(projected))
	if !closed || originalErr != nil || canonicalErr != nil || frozenErr != nil || !bytes.Equal(originalBytes, canonicalBytes) || bytes.Equal(originalBytes, frozenBytes) {
		t.Fatal("lossy freeze fixture did not reach the exact pre-freeze closed shape")
	}
	var store DurableEventSessionStore
	err := store.EnsureOrdinaryToolResultSettlementExact(context.Background(), "thread-synthetic", "turn-synthetic", "call-item-synthetic", item)
	if err == nil || err.Error() != "tool settlement is not canonically closed" {
		t.Fatal("lossy freeze reached primary authority or was silently accepted")
	}
}

// The anchored primary reader preserves json.Number; the prepared host record
// uses float64 after CloneMap. Compare their entire canonical JSON, including
// all binding/timestamp/output fields, without changing either map.
func processSeamResultJSONBytesV1(t *testing.T, value map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal("actual frozen/readback result could not be encoded")
	}
	return encoded
}

func TestProcessDisplayActualReadSettlementAnchorsLaterNativeUnitWithoutCaptureV2(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "anchor-read.txt"})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	const content = "synthetic protected Read content\n"
	if err := os.WriteFile(filepath.Join(pending.Workspace, "anchor-read.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	producer := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if producer == nil {
		t.Fatal("current host producer denied")
	}
	defer producer.FinishV1()
	if !producer.UnitV2(pending.ProviderNamespace, true) || !producer.ToolReadyV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, pending.ToolScope...) {
		t.Fatal("actual persisted Read call anchor rejected")
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
	if err != nil || settled.IsError {
		t.Fatal("actual Read settlement failed")
	}
	if !producer.UnitV2(pending.ProviderNamespace, false) {
		t.Fatal("real Read unit completion rejected")
	}
	next := pending.ProviderNamespace
	next.ProviderCallSequence++
	if !producer.UnitV2(next, true) {
		t.Fatal("later unit rejected")
	}
	producer.FrameV1(domainmodel.ProcessFrameV1{Kind: "start", Attempt: 1, NativeIndex: 0, ProviderSequence: next.ProviderCallSequence})
	producer.FrameV1(domainmodel.ProcessFrameV1{Kind: "end", Attempt: 1, NativeIndex: 0, ProviderSequence: next.ProviderCallSequence})
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	var actualResultID string
	turns, _ := primary.Thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != pending.TurnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == pending.Call.ID {
				if actualResultID != "" {
					t.Fatal("duplicate actual Read result")
				}
				actualResultID = stringField(item, "id")
			}
		}
	}
	if actualResultID == "" {
		t.Fatal("actual Read result not persisted")
	}
	if processSeamAuthorizeV1(t, h, pending, processSeamViewerV1).Status != "authorized" {
		t.Fatal("current own pane denied")
	}
	snapshot, _ := processSeamReadV1(t, h, processSeamViewerV1)
	if snapshot.Status != "snapshot" || len(snapshot.Units) != 2 || len(snapshot.Blocks) != 1 || snapshot.Blocks[0].Source.Kind != "native" || snapshot.Blocks[0].Body != nil {
		t.Fatal("Read invented process body or later native unit missing")
	}
	for _, anchor := range []domainprocess.CallAnchorV2{snapshot.Units[0].Calls[0], snapshot.Units[1].PrecedingCalls[0]} {
		if anchor.CallID != pending.Call.ID || anchor.ToolCallItemID != pending.ToolCallItemID || anchor.ToolResultItemID != actualResultID || anchor.StartedEventSeq == "" {
			t.Fatal("capture-less actual Read result missing from current/preceding anchor")
		}
	}
	replay, err := h.store.LoadEventsSince(pending.ThreadID, 0)
	if err != nil {
		t.Fatal("actual started event readback unavailable")
	}
	var actualStartedSeq uint64
	for _, event := range replay.Events {
		item, _ := event["item"].(map[string]any)
		if stringField(event, "kind") == "tool_call_started" && stringField(item, "callId") == pending.Call.ID {
			seq, ok := numericSeq(event["seq"])
			if !ok || seq <= 0 || actualStartedSeq != 0 {
				t.Fatal("ambiguous actual started event")
			}
			actualStartedSeq = uint64(seq)
		}
	}
	if snapshot.Units[0].Calls[0].StartedEventSeq != strconv.FormatUint(actualStartedSeq, 10) {
		t.Fatal("started identity was not the real committed RecordEvent sequence")
	}
	for _, mutate := range []func(*runtimePendingToolCall){
		func(p *runtimePendingToolCall) { p.ToolCallItemID = "wrong-call-item" },
		func(p *runtimePendingToolCall) { p.ExecutionGrant.GrantID = strings.Repeat("0", 64) },
		func(p *runtimePendingToolCall) { p.SecurityContext.ContextEpoch++ },
		func(p *runtimePendingToolCall) { p.Call.Name = "write" },
	} {
		wrong := pending
		mutate(&wrong)
		h.registerProcessToolSettlementV2(context.Background(), wrong)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	h.registerProcessToolSettlementV2(canceled, pending)
	h.registerProcessToolSettlementV2(context.Background(), pending)
	h.registerProcessToolSettlementV2(context.Background(), pending)
	afterReplay, _ := processSeamReadV1(t, h, processSeamViewerV1)
	if !reflect.DeepEqual(snapshot, afterReplay) {
		t.Fatal("settlement notification replay/conflict/cancellation changed revision or anchors")
	}
	status, display, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.Capture.Body != content {
		t.Fatal("Read protected result authority changed")
	}
	assertProcessSeamPublicIsolationV1(t, h, pending, snapshot.Blocks[0], content)
}

func TestProcessDisplayClosedReadAnchorSurvivesPublicEventFailureV2(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "event-failure.txt"})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	if err := os.WriteFile(filepath.Join(pending.Workspace, "event-failure.txt"), []byte("synthetic closed Read\n"), 0600); err != nil {
		t.Fatal(err)
	}
	producer := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if producer == nil {
		t.Fatal("current host producer denied")
	}
	defer producer.FinishV1()
	if !producer.ToolReadyV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, pending.ToolScope...) {
		t.Fatal("actual Read ready rejected")
	}
	if processSeamAuthorizeV1(t, h, pending, processSeamViewerV1).Status != "authorized" {
		t.Fatal("current own pane denied")
	}
	before, _ := processSeamReadV1(t, h, processSeamViewerV1)
	h.registerProcessToolSettlementV2(context.Background(), pending)
	unclosed, _ := processSeamReadV1(t, h, processSeamViewerV1)
	if !reflect.DeepEqual(before, unclosed) {
		t.Fatal("unsettled grant created a result anchor")
	}
	h.recordRuntimeToolStarted(pending)
	// Fault only this fresh fixture's public event append, after a real started
	// event. The primary/CAS settlement and execution authority stay untouched.
	eventPath := h.store.eventsPath(pending.ThreadID)
	backup := eventPath + ".anchor-test-backup"
	if err := os.Rename(eventPath, backup); err != nil {
		t.Fatal("isolated event fixture backup failed")
	}
	if err := os.Mkdir(eventPath, 0700); err != nil {
		_ = os.Rename(backup, eventPath)
		t.Fatal("isolated event failure fixture unavailable")
	}
	restore := func() {
		if err := os.Remove(eventPath); err != nil {
			t.Error("isolated event failure directory cleanup failed")
			return
		}
		if err := os.Rename(backup, eventPath); err != nil {
			t.Error("isolated event fixture restoration failed")
		}
	}
	defer restore()
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err == nil {
		t.Fatal("real public event append was not faulted")
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal("closed primary readback unavailable")
	}
	actual := protectedSeamResultV1(t, primary.Thread, pending)
	snapshot, _ := processSeamReadV1(t, h, processSeamViewerV1)
	if snapshot.Status != "snapshot" || len(snapshot.Units) != 1 || len(snapshot.Units[0].Calls) != 1 || snapshot.Units[0].Calls[0].ToolResultItemID != stringField(actual, "id") || snapshot.Units[0].Calls[0].StartedEventSeq == "" || len(snapshot.Blocks) != 0 {
		t.Fatal("already closed Read identity lost with later public event failure")
	}
	for replay := 0; replay < 2; replay++ {
		h.registerProcessToolSettlementV2(context.Background(), pending)
	}
	after, _ := processSeamReadV1(t, h, processSeamViewerV1)
	readback, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil || readback.ThreadFileSHA256 != primary.ThreadFileSHA256 || !reflect.DeepEqual(snapshot, after) {
		t.Fatal("exact settlement notification replay rewrote primary or volatile identity")
	}
}

func TestAssistantDraftRealProviderStreamProtectedBeforeTerminalV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	const key = "synthetic-draft-provider-key-746839"
	const private = "PRIVATE_INLINE_DRAFT_SEAM_6478"
	producer := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext, key)
	if producer == nil || !producer.UnitV2(appmodel.ProviderNamespace("turn", "", 7, nil), true) {
		t.Fatal("real source unit not authorized")
	}
	defer producer.FinishV1()
	if got := processSeamAuthorizeV1(t, h, pending, processSeamViewerV1); got.Status != "authorized" {
		t.Fatal("protected viewer denied")
	}
	finish := make(chan struct{})
	advance := make(chan struct{})
	var finishOnce sync.Once
	release := func() { finishOnce.Do(func() { close(finish) }) }
	defer release()
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Inspect only synthetic request data: the observer is not another
		// producer and must not send prior draft bytes back to a model.
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("synthetic request invalid")
			return
		}
		encoded, _ := json.Marshal(request)
		if strings.Contains(string(encoded), private) || strings.Contains(string(encoded), "assistant_draft") {
			t.Error("private draft leaked to model request")
			return
		}
		for _, text := range []string{"<think>" + private + " first words ", "second words " + key[:16], key[16:] + " ", "more safe words "} {
			encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": text}}}})
			_, _ = w.Write(append(append([]byte("data: "), encoded...), []byte("\n\n")...))
			w.(http.Flusher).Flush()
			select {
			case <-advance:
			case <-finish:
				return
			}
		}
		<-finish
		for _, line := range []string{`data: {"choices":[{"delta":{"content":"</think>accepted answer"}}]}`, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`, `data: [DONE]`} {
			_, _ = w.Write([]byte(line + "\n\n"))
		}
	}))
	defer func() { release(); fake.Close() }()
	updates := make(chan struct{}, 8)
	type outcome struct {
		output apploop.ProviderStreamOutput
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := apploop.StreamProviderWithRetry(context.Background(), apploop.ProviderStreamInput{
			Provider: provider.NewHTTPProviderClient(fake.Client()), SecurityContext: pending.SecurityContext, OrdinaryEffect: true,
			Request:     domainmodel.Request{ProviderID: "fixture", Family: "openai", EndpointFormat: "chat_completions", BaseURL: fake.URL, APIKey: key, Model: "fixture", Messages: []domainmodel.Message{{Role: "user", Content: "synthetic prompt"}}},
			MaxAttempts: 1, Callbacks: apploop.ProviderStreamCallbacks{
				OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
					frame.ProviderSequence = 7
					frame.ProjectionSecrets = []string{key}
					producer.FrameV1(frame)
					if frame.Source == "assistant_draft" {
						updates <- struct{}{}
					}
				},
			},
		})
		done <- outcome{out, err}
	}()
	previous := ""
	var block domainprocess.BlockV1
	for i := 0; i < 4; i++ {
		select {
		case <-updates:
		case <-time.After(5 * time.Second):
			release()
			t.Fatal("provider did not observe a preterminal draft delta")
		}
		select {
		case <-done:
			t.Fatal("synthetic provider finished before blocked terminal")
		default:
		}
		metadata, _ := processSeamReadV1(t, h, processSeamViewerV1)
		if metadata.Status != "snapshot" || len(metadata.Blocks) != 1 {
			t.Fatal("private source not visible before terminal")
		}
		expanded, raw := processSeamReadV1(t, h, processSeamViewerV1, metadata.Blocks[0].BlockID)
		block = expanded.Blocks[0]
		if expanded.Status != "snapshot" || block.Kind != "assistant_draft" || block.State != "running" || block.Source.ProviderSequence != "7" || block.Source.Attempt != 1 || block.ContentClassification != "unclassified_inline" || len(block.DraftSegments) == 0 {
			t.Fatal("running protected draft missing real identity/classification")
		}
		if strings.Contains(raw, key) || strings.Contains(raw, key[:16]) {
			t.Fatal("split host-known credential escaped draft")
		}
		if i == 0 && !strings.Contains(raw, private) {
			t.Fatal("private source draft missing")
		}
		if previous == expanded.Revision {
			t.Fatal("distinct blocked provider delta did not advance draft revision")
		}
		previous = expanded.Revision
		assertProcessSeamPublicIsolationV1(t, h, pending, block, private, key)
		advance <- struct{}{}
	}
	if previous == "" {
		t.Fatal("no draft revisions")
	}
	release()
	select {
	case result := <-done:
		if result.err != nil || result.output.Text != "accepted answer" {
			t.Fatalf("terminal parser changed: %v", result.err)
		}
		for _, chunk := range result.output.Result.Chunks {
			if chunk.ProcessFrame != nil || strings.Contains(chunk.Text, private) {
				t.Fatal("raw draft entered result/model continuation")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("synthetic terminal did not finish")
	}
	retired, raw := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
	if retired.Status != "snapshot" || len(retired.Blocks[0].DraftSegments) != 0 || strings.Contains(raw, private) {
		t.Fatal("terminal kept private draft alive")
	}
	producer.FrameV1(domainmodel.ProcessFrameV1{Source: "assistant_draft", Kind: "delta", ProviderSequence: 7, Attempt: 1, Text: "LATE_PRIVATE_DRAFT ", ContentClassification: "unclassified_inline"})
	_, raw = processSeamReadV1(t, h, processSeamViewerV1, block.BlockID)
	if strings.Contains(raw, "LATE_PRIVATE_DRAFT") {
		t.Fatal("late callback revived terminal draft")
	}
	h.turnSecurity.Identity = nil
	if denied, _ := processSeamReadV1(t, h, processSeamViewerV1, block.BlockID); denied.Status != "unavailable" {
		t.Fatal("current principal revocation did not retire viewer")
	}
}

func TestAssistantDraftActualEpochDriftRevokesAndRetiresV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	producer := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if producer == nil || !producer.UnitV2(appmodel.ProviderNamespace("turn", "", 7, nil), true) {
		t.Fatal("actual draft producer denied")
	}
	defer producer.FinishV1()
	producer.FrameV1(domainmodel.ProcessFrameV1{Source: "assistant_draft", Kind: "delta", ProviderSequence: 7, Attempt: 1, Text: "CURRENT_EPOCH_PRIVATE_DRAFT ", ContentClassification: "provider_text"})
	if got := processSeamAuthorizeV1(t, h, pending, processSeamViewerV1); got.Status != "authorized" {
		t.Fatal("actual viewer denied")
	}
	before, _ := processSeamReadV1(t, h, processSeamViewerV1)
	if len(before.Blocks) != 1 {
		t.Fatal("no actual draft source")
	}
	expanded, raw := processSeamReadV1(t, h, processSeamViewerV1, before.Blocks[0].BlockID)
	if expanded.Status != "snapshot" || !strings.Contains(raw, "CURRENT_EPOCH_PRIVATE_DRAFT") {
		t.Fatal("no positive draft before epoch drift")
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	freezeThread := cloneMap(snapshot.Thread)
	epoch, err := contextepochapp.DefaultState(pending.ThreadID, pending.SecurityContext.ContextEpoch+1, now)
	if err != nil {
		t.Fatal(err)
	}
	freezeThread["contextEpochState"] = contextepochapp.PublicState(epoch)
	successor, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(), Authority: h.turnSecurity, Thread: freezeThread, ThreadID: pending.ThreadID, TurnID: "turn_draft_successor", Workspace: pending.Workspace, Principal: testIdentityPrincipal(), IssuedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	epoch, err = contextepochapp.BootstrapState(pending.ThreadID, successor.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(successor)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.AppendTurnToThread(pending.ThreadID, map[string]any{"id": successor.TurnID, "threadId": pending.ThreadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano), "securityContext": turnsecurityapp.PublicRecord(successor), "items": []any{}}, "synthetic", map[string]any{"securityState": turnsecurityapp.PublicRecord(successor), "contextEpochState": contextepochapp.PublicState(epoch)}); err != nil {
		t.Fatal(err)
	}
	// This is a valid matching successor own turn. Completed own native
	// history still passes the base lease, while the exact draft epoch fails.
	authority := runtimeProcessDisplayAccessV1{handler: h}
	binding := producer.BindingV2()
	acquired, release, err := authority.AcquireCurrent(context.Background(), binding)
	if err != nil || release == nil {
		t.Fatalf("valid successor unexpectedly denied base authority: %v", err)
	}
	if err := authority.ValidateCurrent(context.Background(), acquired); err != nil {
		release()
		t.Fatal("valid successor failed base current authority")
	}
	if authority.ValidateAssistantDraftCurrent(context.Background(), binding) == nil {
		release()
		t.Fatal("old draft epoch accepted valid successor")
	}
	release()
	if got, raw := processSeamReadV1(t, h, processSeamViewerV1, before.Blocks[0].BlockID); got.Status != "unavailable" || strings.Contains(raw, "CURRENT_EPOCH_PRIVATE_DRAFT") {
		t.Fatal("real epoch drift released old draft")
	}
	// The valid successor still permits the original own native-history lease.
	// A fresh viewer may see the retired block, but no later attempt may revive
	// this old logical draft source. Authority restoration/ABA is independently
	// covered at the service owner without rolling back signed thread history.
	producer.FrameV1(domainmodel.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
	producer.FrameV1(domainmodel.ProcessFrameV1{Source: "assistant_draft", Kind: "delta", ProviderSequence: 7, Attempt: 2, Text: "LATE_EPOCH_PRIVATE_DRAFT ", ContentClassification: "provider_text"})
	const freshViewer = "77777777-7777-4777-8777-777777777777"
	if got := processSeamAuthorizeV1(t, h, pending, freshViewer); got.Status != "authorized" {
		t.Fatal("fresh viewer under valid successor denied")
	}
	got, raw := processSeamReadV1(t, h, freshViewer, before.Blocks[0].BlockID)
	if got.Status != "snapshot" || len(got.Blocks[0].DraftSegments) != 0 || strings.Contains(raw, "LATE_EPOCH_PRIVATE_DRAFT") || strings.Contains(raw, "CURRENT_EPOCH_PRIVATE_DRAFT") {
		t.Fatal("valid successor revived retired logical source")
	}
}

func TestWebActiveInputRealGrantAndHTTPSealedTargetV1(t *testing.T) {
	const requestURL = "https://example.invalid/actual/request?case=alice"
	h, pending, _, _ := protectedSeamFixtureV1(t, "web_fetch", map[string]any{"url": requestURL})
	pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
	h.recordRuntimeToolStarted(pending)
	h.registerProcessWebInputV1(context.Background(), pending)
	if response := processSeamAuthorizeV1(t, h, pending, processSeamViewerV1); response.Status != "authorized" {
		t.Fatal("real native pane denied")
	}
	snapshot, wire := processSeamReadV1(t, h, processSeamViewerV1)
	if snapshot.Status != "snapshot" || len(snapshot.Blocks) != 1 {
		t.Fatal("real active input missing")
	}
	block := snapshot.Blocks[0]
	if block.Kind != "web_fetch" || block.RequestURL != requestURL || block.InputDigest == "" || block.Body != nil || block.Source.ToolResultItemID != "" {
		t.Fatal("active input became fake output")
	}
	if strings.Contains(wire, "targetUrl") || strings.Contains(wire, pending.ExecutionGrant.GrantID) || strings.Contains(wire, pending.ExecutionGrant.ArgsHash) {
		t.Fatal("host target/grant leaked into pane wire")
	}
	payload := map[string]any{"schemaVersion": 2, "action": "open_web_link", "viewerToken": processSeamViewerV1, "blockId": block.BlockID, "inputDigest": block.InputDigest, "role": "request"}
	body, _ := json.Marshal(payload)
	r := httptest.NewRequest(http.MethodPost, httpapi.ProcessLocalDisplayTargetPathV1, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, Processes: h.ProcessLocalDisplayV1()}.ServeHTTP(w, r)
	var target processapp.WebLinkResponseV1
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &target) != nil || target.Status != "web_link" || target.TargetURL != requestURL || target.InputDigest != block.InputDigest || target.Scope == nil {
		t.Fatal("real signed source could not resolve sealed target")
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	altered := cloneMap(primary.Thread)
	turns, _ := altered["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != pending.TurnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") == pending.ToolCallItemID {
				item["status"] = "completed"
			}
		}
	}
	if err := h.store.ReplaceThreadForAuthorityRepair(pending.ThreadID, altered); err != nil {
		t.Fatal(err)
	}
	if got := h.processDisplay.OpenWebLinkV1(context.Background(), processSeamViewerV1, block.BlockID, block.InputDigest, "request"); got.Status != "unavailable" || got.TargetURL != "" {
		t.Fatal("non-running source remained clickable")
	}
}

func TestWebActiveInputRejectsWrongKindHashAndGrantV1(t *testing.T) {
	for _, failure := range []string{"kind", "args_hash", "grant", "name", "call", "result"} {
		t.Run(failure, func(t *testing.T) {
			h, pending, _, _ := protectedSeamFixtureV1(t, "web_fetch", map[string]any{"url": "https://example.invalid/actual"})
			pending.ProviderNamespace = appmodel.ProviderNamespace("turn", "", 1, nil)
			h.recordRuntimeToolStarted(pending)
			principal, err := turnsecurityapp.ResolveCurrentPrincipal(context.Background(), h.turnSecurity.Identity)
			if err != nil {
				t.Fatal(err)
			}
			binding := processport.BindingV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, Principal: principal, Original: pending.SecurityContext}
			tool := domainprocess.ToolBindingV1{CallID: pending.Call.ID, ToolName: "web_fetch", GrantID: pending.ExecutionGrant.GrantID, InputArgsHash: pending.ExecutionGrant.ArgsHash}
			access := runtimeProcessDisplayAccessV1{handler: h}
			if access.ValidateToolCurrent(context.Background(), binding, tool) != nil {
				t.Fatal("positive active grant fixture invalid")
			}
			switch failure {
			case "args_hash":
				tool.InputArgsHash = strings.Repeat("0", 64)
			case "grant":
				tool.GrantID = strings.Repeat("0", 64)
			case "name":
				tool.ToolName = "bash"
			case "call":
				tool.CallID = serverTestHostToolCallID("other_active_call")
			case "result":
				tool.ResultItemID = domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID)
			case "kind":
				primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
				if err != nil {
					t.Fatal(err)
				}
				altered := cloneMap(primary.Thread)
				turns, _ := altered["turns"].([]any)
				for _, rawTurn := range turns {
					turn, _ := rawTurn.(map[string]any)
					if stringField(turn, "id") != pending.TurnID {
						continue
					}
					items, _ := turn["items"].([]any)
					for _, rawItem := range items {
						item, _ := rawItem.(map[string]any)
						if stringField(item, "id") == pending.ToolCallItemID {
							item["toolKind"] = "skill"
						}
					}
				}
				if err := h.store.ReplaceThreadForAuthorityRepair(pending.ThreadID, altered); err != nil {
					t.Fatal(err)
				}
			}
			if access.ValidateToolCurrent(context.Background(), binding, tool) == nil {
				t.Fatal("forged active Web source gained authority")
			}
		})
	}
}
