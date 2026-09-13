//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	"analytix.local/runtime-go/internal/contracts"
	"analytix.local/runtime-go/internal/mcp"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// One production-composition driver crosses the same ordinary effects for each
// optional fault. Synthetic model/doc peers use loopback and MCP uses local stdio; no package or
// live-provider claim is made by this test.
func TestRuntimeOptionalPluginOrdinaryLifecycle(t *testing.T) {
	for _, fault := range []string{"missing", "disabled", "incompatible", "unauthorized", "domain-semantic"} {
		t.Run(fault, func(t *testing.T) { runRuntimeOptionalPluginOrdinaryLifecycleV1(t, fault) })
	}
}

func runRuntimeOptionalPluginOrdinaryLifecycleV1(t *testing.T, fault string, collectors ...*runtimeOptionalPublicCollectorV1) {
	runRuntimeOptionalPluginOrdinaryLifecycleModeV1(t, fault, nil, collectors...)
}

func runRuntimeOptionalPluginOrdinaryLifecycleModeV1(t *testing.T, fault string, diagnostic *runtimeOptionalTerminalDiagnosticV1, collectors ...*runtimeOptionalPublicCollectorV1) {
	t.Helper()
	if diagnostic != nil && (fault != "unauthorized" || len(collectors) != 0) {
		t.Fatal("terminal diagnostic requires its dedicated unauthorized-only entry")
	}
	var collector *runtimeOptionalPublicCollectorV1
	if len(collectors) != 0 {
		collector = collectors[0]
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("the ordinary Coding fixture requires the configured local Node test runner")
	}
	root := ""
	if collector != nil {
		var cleanup func() error
		root, cleanup, err = runtimeOptionalPublicFixtureRootV1()
		if err != nil {
			t.Fatal("isolated public fixture root is unavailable")
		}
		t.Cleanup(func() {
			if err := cleanup(); err != nil {
				t.Error("isolated public fixture cleanup failed")
			}
		})
	} else {
		root = workspacetest.New(t)
	}
	workspace := filepath.Join(root, "ordinary-workspace")
	skills := filepath.Join(root, "skills")
	for _, dir := range []string{workspace, filepath.Join(skills, "ordinary-writing")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(workspace, "normalize.cjs"):             "module.exports = name => name === 'q' ? 'q' : name;\n",
		filepath.Join(workspace, "normalize.test.cjs"):        "const test = require('node:test');\nconst assert = require('node:assert/strict');\nconst normalize = require('./normalize.cjs');\ntest('quality name is case insensitive and other names survive', () => { assert.equal(normalize('Q'), 'q'); assert.equal(normalize('q'), 'q'); assert.equal(normalize('Charset'), 'Charset'); require('node:fs').writeFileSync('coding-test-ok.txt', 'R131_CODING_TEST_OK\\n'); console.log('R131_CODING_TEST_OK'); });\n",
		filepath.Join(skills, "ordinary-writing", "SKILL.md"): "---\nname: ordinary-writing\ndescription: Draft a concise ordinary note\n---\nUse the heading R131_SKILL_CONSUMED and state that the local coding regression passed.\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var docCalls atomic.Int64
	doc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/ordinary-source" {
			t.Error("unexpected loopback research request")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		docCalls.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body><h1>Ordinary source</h1><p>R131_LOOPBACK_RESEARCH: quality parameter names are case insensitive.</p></body></html>")
	}))
	defer doc.Close()
	entrypoint := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_SCHEDULE_MCP_ENTRYPOINT_V1"))
	if entrypoint == "" {
		t.Skip("BLOCKED: current-source schedule MCP entrypoint is not configured")
	}
	if info, err := os.Stat(entrypoint); err != nil || !info.Mode().IsRegular() || !filepath.IsAbs(entrypoint) {
		t.Fatal("current-source schedule MCP entrypoint is unavailable")
	}
	const scheduleSecret = "r131-synthetic-schedule-secret"
	var scheduleCalls atomic.Int64
	scheduleBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil || r.Method != http.MethodPost || r.URL.Path != "/schedule/internal/list" || r.Header.Get("Authorization") != "Bearer "+scheduleSecret || string(body) != "{}" {
			t.Error("unexpected synthetic schedule backend request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		scheduleCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tasks": []any{}})
	}))
	defer scheduleBackend.Close()
	scheduleSpec := mcp.ServerSpec{ID: "gui_schedule", Transport: "stdio", Command: node,
		Args: []string{entrypoint, "--gui-schedule-mcp-server", "--base-url", scheduleBackend.URL, "--secret", scheduleSecret},
		Env:  map[string]string{"ELECTRON_RUN_AS_NODE": "1"}, TrustScope: "user", TimeoutMS: 5000}
	mcpCalls := func() int { return int(scheduleCalls.Load()) }
	model := &runtimeOptionalLifecycleModelV1{t: t, steps: map[string]int{}, lastToolResults: map[string]int{}, docURL: doc.URL + "/ordinary-source"}
	provider := httptest.NewServer(http.HandlerFunc(model.serve))
	defer provider.Close()
	document, err := json.Marshal(map[string]any{
		"mcpServers":   map[string]any{"gui_schedule": map[string]any{"transport": scheduleSpec.Transport, "command": scheduleSpec.Command, "args": scheduleSpec.Args, "env": scheduleSpec.Env, "trustScope": scheduleSpec.TrustScope, "timeoutMs": scheduleSpec.TimeoutMS}},
		"skills":       map[string]any{"enabled": true, "roots": []string{skills}},
		"subagents":    map[string]any{"enabled": true, "default_tool_policy": "readOnly"},
		"capabilities": map[string]any{"web": map[string]any{"enabled": true, "fetchEnabled": true, "allowDomains": []string{"127.0.0.1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := newRuntimeOptionalPluginLifecycleFixtureV1(t, fault, Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: filepath.Join(root, "runtime-data"),
		ProductionDurableRoot: filepath.Join(root, "durable"), UserDataDir: filepath.Join(root, "user-data"),
		ProviderID: "ordinary-lifecycle", Model: "ordinary-lifecycle-model", APIKey: "test-only",
		BaseURL: provider.URL + "/v1", EndpointFormat: "chat_completions", MCPConfigJSON: string(document), HostScheduleMCPServer: &scheduleSpec,
	})
	config := fixture.config
	if diagnostic != nil {
		diagnostic.guard = fixture.async
	}
	if collector != nil {
		defer collector.finish(t, fixture.async)
	}
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
	handler, err := fixture.start()
	if err != nil {
		t.Fatal(err)
	}
	collector.bindHandler(t, handler)
	server := httptest.NewServer(handler)
	defer func() {
		if server != nil {
			server.Close()
			shutdownOwnedRuntimeHandler(t, handler)
			fixture.async.assertDrained(t)
		}
	}()
	fixture.assertFault(1)
	client := &http.Client{Timeout: 30 * time.Second, Transport: runtimeOptionalPrivacyTransport{t: t}}
	if collector != nil {
		collector.nextPhase("startup")
		collector.transport = client.Transport
		client.Transport = collector
	}
	request := func(method, path string, body map[string]any, expected int) map[string]any {
		t.Helper()
		status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, method, path, body)
		if status != expected {
			t.Fatalf("ordinary lifecycle %s %s: status=%d want=%d code=%s message=%.200s", method, path, status, expected, contracts.StringField(result, "code"), contracts.StringField(result, "message"))
		}
		return result
	}
	diagnostics := request(http.MethodGet, "/v1/runtime/tools", nil, http.StatusOK)
	servers, _ := diagnostics["mcpServers"].([]any)
	ordinaryConnected := false
	for _, raw := range servers {
		server, _ := raw.(map[string]any)
		if server["id"] == "gui_schedule" {
			ordinaryConnected = server["connected"] == true
			if !ordinaryConnected {
				t.Fatalf("ordinary stdio MCP not connected: status=%v failureCode=%v", server["status"], server["failureCode"])
			}
		}
	}
	if !ordinaryConnected {
		t.Fatal("ordinary stdio MCP configuration was not composed")
	}
	created := request(http.MethodPost, "/v1/threads", map[string]any{"title": "ordinary lifecycle", "workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
	threadID := contracts.StringField(created, "id")
	if threadID == "" {
		t.Fatal("missing ordinary thread identity")
	}
	turns := map[string]string{}
	run := func(thread, phase string, tools ...string) map[string]any {
		t.Helper()
		t.Logf("ordinary lifecycle phase=%s", phase)
		collector.nextPhase(phase)
		model.setPhase(phase)
		sandboxMode := "workspace-write"
		if phase == "coding" {
			sandboxMode = "danger-full-access"
		}
		prompt := "Complete R131_" + strings.ToUpper(phase) + " using the ordinary tools."
		if phase == "jobs" {
			prompt = "Use the task tool to start one background subagent for the completed ordinary work."
		}
		started := request(http.MethodPost, "/v1/threads/"+thread+"/turns", map[string]any{
			"prompt": prompt, "mode": "agent", "async": true,
			"approvalPolicy": "auto", "sandboxMode": sandboxMode,
		}, http.StatusAccepted)
		id := contracts.StringField(started, "turnId")
		if id == "" {
			t.Fatal("ordinary lifecycle start omitted turn identity")
		}
		fixture.async.expect(thread, id, phase)
		turn := runtimeOptionalLifecycleWaitTurnV1(t, client, server.URL, thread, id, phase)
		runtimeOptionalLifecycleAssertTurnV1(t, turn, "R131_"+strings.ToUpper(phase)+"_COMPLETE", tools)
		turns[phase] = id
		if collector != nil {
			request(http.MethodGet, "/v1/threads/"+thread, nil, http.StatusOK)
			request(http.MethodGet, "/v1/threads?limit=50", nil, http.StatusOK)
		}
		return turn
	}
	run(threadID, "mcp", mcp.CanonicalToolName("gui_schedule", "gui_schedule_list"))
	if mcpCalls() != 1 {
		t.Fatalf("ordinary MCP real call count=%d", mcpCalls())
	}
	if diagnostic != nil {
		return // Dedicated diagnostic only; normal five-fault sequence is unchanged.
	}
	run(threadID, "coding", "read", "edit", "bash")
	if body, err := os.ReadFile(filepath.Join(workspace, "normalize.cjs")); err != nil || !strings.Contains(string(body), "name.toLowerCase() === 'q'") {
		t.Fatal("Coding did not change the real workspace source")
	}
	runtimeOptionalLifecycleAssertFileV1(t, workspace, "coding-test-ok.txt", "R131_CODING_TEST_OK\n")
	run(threadID, "writing", "write")
	runtimeOptionalLifecycleAssertFileV1(t, workspace, "brief.md", "# Ordinary work\nThe local coding regression passed.\n")
	run(threadID, "research", "web_fetch")
	if docCalls.Load() != 1 {
		t.Fatalf("Research real loopback request count=%d", docCalls.Load())
	}
	run(threadID, "skills", "run_skill", "write")
	runtimeOptionalLifecycleAssertFileV1(t, workspace, "skill-applied.md", "# R131_SKILL_CONSUMED\nThe local coding regression passed.\n")
	run(threadID, "jobs", "task")
	wait := request(http.MethodPost, "/v1/runtime/task-jobs/wait", map[string]any{"threadId": threadID, "timeoutMs": 5000}, http.StatusOK)
	jobs, _ := wait["jobs"].([]any)
	if len(jobs) != 1 {
		t.Fatalf("expected exactly one bounded background job, got %d", len(jobs))
	}
	job, _ := jobs[0].(map[string]any)
	jobID := contracts.StringField(job, "id")
	if jobID == "" || job["status"] != "completed" || job["background"] != true || model.childCalls.Load() != 1 {
		t.Fatal("the real bounded subagent did not complete")
	}
	output := request(http.MethodPost, "/v1/runtime/task-jobs/output", map[string]any{"threadId": threadID, "jobId": jobID}, http.StatusOK)
	if output["status"] != "completed" || output["availability"] != "withheld" || output["outputWithheld"] != true || output["factAnswerAllowed"] != false || output["evidenceAuthority"] != false || output["canReadOutput"] != false || output["canContinueParent"] != false {
		t.Fatal("job output did not preserve the existing security-bound metadata contract")
	}
	if collector != nil {
		runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, 0, "", turns["jobs"], false, false)
	}

	collector.nextPhase("protected")
	model.setPhase("protected")
	beforeProtected := model.calls.Load()
	protected := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": packagedSourceUnavailableHydrationPromptV1, "mode": "agent", "async": true}, http.StatusAccepted)
	protectedID := contracts.StringField(protected, "turnId")
	packagedPlanThenProtectedWaitSSETerminalV1(t, client, server.URL, threadID, protectedID)
	protectedReadback := packagedPlanThenProtectedReadV1(t, client, server.URL, threadID, protectedID)
	packagedPlanThenProtectedValidateZeroFactArtifactsV1(t, protectedReadback.turn)
	if model.calls.Load() != beforeProtected || mcpCalls() != 1 {
		t.Fatal("protected refusal dispatched model or ordinary MCP work")
	}
	// Validate each boundary at its complete snapshot cursor. Starting an ordinary
	// turn may also change the current security context of this case-bound thread.
	runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, 0, protectedID, turns["jobs"], false, false)
	run(threadID, "post-denial")
	runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, protectedReadback.hydration.latestSeq, "", turns["post-denial"], false, false)
	preCompact := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
	preCompactSeq, ok := contracts.NumericSeq(preCompact["latestSeq"])
	if !ok || preCompactSeq <= 0 {
		t.Fatal("complete pre-compaction history omitted its replay cursor")
	}
	t.Log("ordinary lifecycle phase=manual-compaction")
	collector.nextPhase("manual-compaction")
	model.setPhase("compact")
	compacted := request(http.MethodPost, "/v1/threads/"+threadID+"/compact", map[string]any{"reason": "manual"}, http.StatusOK)
	if compacted["ok"] != true || compacted["auto"] == true || contracts.StringField(compacted, "sourceDigest") == "" {
		t.Fatal("manual compaction did not produce a committed source binding")
	}
	beforeRestart := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
	beforeMarker := runtimeProductionCaseCompactionMarkerV1(t, beforeRestart)
	if beforeMarker["auto"] != false || beforeMarker["sourceDigest"] != compacted["sourceDigest"] {
		t.Fatal("manual compaction marker did not bind the committed source")
	}
	// Old-epoch protected authority must remain revoked; a complete snapshot
	// cursor can replay the new compaction without reviving that authority.
	runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, 0, "", "", false, true)
	runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, preCompactSeq, "", "", true, false)

	t.Log("ordinary lifecycle phase=fresh-composition")
	collector.nextPhase("fresh-composition")
	server.Close()
	shutdownOwnedRuntimeHandler(t, handler)
	fixture.async.assertDrained(t)
	server = nil
	handler, err = fixture.start()
	if err != nil {
		t.Fatal(err)
	}
	collector.bindHandler(t, handler)
	server = httptest.NewServer(handler)
	fixture.assertFault(2)
	afterRestart := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
	for _, id := range append([]string{protectedID}, runtimeOptionalLifecycleTurnIDsV1(turns)...) {
		before := packagedSourceUnavailableHydrationTurnV1(beforeRestart, id)
		after := packagedSourceUnavailableHydrationTurnV1(afterRestart, id)
		if before == nil || after == nil || !packagedSourceUnavailableHydrationSameJSONV1(before, after) {
			t.Fatal("public completed turn changed across fresh production composition")
		}
	}
	marker := runtimeProductionCaseCompactionMarkerV1(t, afterRestart)
	if marker["sourceDigest"] != compacted["sourceDigest"] || marker["auto"] != false {
		t.Fatal("manual compaction binding did not survive fresh startup")
	}
	// Old-epoch protected authority must remain revoked; a complete snapshot
	// cursor can replay the new compaction without reviving that authority.
	runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, 0, "", "", false, true)
	runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, preCompactSeq, "", "", true, false)
	run(threadID, "restart")
	runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, threadID, preCompactSeq, "", turns["restart"], true, false)
	resumed := request(http.MethodPost, "/v1/sessions/"+threadID+"/resume-thread", map[string]any{"workspace": workspace, "model": config.Model}, http.StatusCreated)
	resumedID := contracts.StringField(resumed, "thread_id")
	if resumedID == "" {
		t.Fatal("public resume did not create a continuation identity")
	}
	run(resumedID, "resume")
	request(http.MethodGet, "/v1/threads/"+resumedID, nil, http.StatusOK)
	if fault == "missing" {
		// One full cell additionally proves both producers across the target's
		// first accepted final and a second fresh composition. The fork cutoff
		// deliberately excludes later source turns.
		fork := request(http.MethodPost, "/v1/threads/"+threadID+"/fork", map[string]any{"turnId": turns["coding"]}, http.StatusCreated)
		forkID := contracts.StringField(fork, "id")
		if forkID == "" {
			t.Fatal("public fork omitted identity")
		}
		forkRead := request(http.MethodGet, "/v1/threads/"+forkID, nil, http.StatusOK)
		inherited, _ := forkRead["turns"].([]any)
		if len(inherited) != 2 || contracts.StringField(inherited[1].(map[string]any), "id") != turns["coding"] {
			t.Fatal("fork did not apply the exact meaningful cutoff")
		}
		run(forkID, "fork")
		beforeFork := request(http.MethodGet, "/v1/threads/"+forkID, nil, http.StatusOK)
		beforeResume := request(http.MethodGet, "/v1/threads/"+resumedID, nil, http.StatusOK)
		// Changing the admitted parent must not recapture the immutable old source.
		run(threadID, "source-after-derive")
		server.Close()
		shutdownOwnedRuntimeHandler(t, handler)
		fixture.async.assertDrained(t)
		server = nil
		handler, err = fixture.start()
		if err != nil {
			t.Fatal(err)
		}
		collector.bindHandler(t, handler)
		server = httptest.NewServer(handler)
		fixture.assertFault(3)
		for _, pair := range []struct {
			id     string
			before map[string]any
			phase  string
		}{
			{forkID, beforeFork, "fork-restart"}, {resumedID, beforeResume, "resume-restart"},
		} {
			after := request(http.MethodGet, "/v1/threads/"+pair.id, nil, http.StatusOK)
			if !packagedSourceUnavailableHydrationSameJSONV1(pair.before["turns"], after["turns"]) {
				t.Fatal("derived history changed across fresh composition")
			}
			sinceSeq, ok := contracts.NumericSeq(after["latestSeq"])
			if !ok || sinceSeq <= 0 {
				t.Fatal("complete derived snapshot omitted its replay cursor")
			}
			run(pair.id, pair.phase)
			request(http.MethodGet, "/v1/threads/"+pair.id, nil, http.StatusOK)
			// The next target context revokes its earlier epoch just as in
			// the original thread. Resume from the complete snapshot cursor.
			runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, pair.id, sinceSeq, "", turns[pair.phase], false, false)
		}
		reader, err := finalauthorityadapter.NewAcceptedFinalCASReader(config.ProductionDurableRoot)
		if err != nil {
			t.Fatal(err)
		}
		privatePrefix := func() (string, []byte) {
			t.Helper()
			snapshot, err := reader.ReadPrimaryThreadSnapshotV1(context.Background(), forkID)
			if err != nil {
				t.Fatal(err)
			}
			privateTurns, ok := snapshot.Thread["turns"].([]any)
			if !ok || len(privateTurns) < len(inherited) {
				t.Fatal("derived compaction lost the inherited prefix")
			}
			body, err := json.Marshal(privateTurns[:len(inherited)])
			if err != nil {
				t.Fatal(err)
			}
			receipt := contracts.StringField(snapshot.Thread, "activeInheritedHistoryReceipt")
			if receipt == "" {
				t.Fatal("derived compaction lost its private lineage receipt")
			}
			return receipt, body
		}
		receipt, prefix := privatePrefix()
		assertPrefix := func() {
			t.Helper()
			afterReceipt, afterPrefix := privatePrefix()
			if afterReceipt != receipt || !bytes.Equal(prefix, afterPrefix) {
				t.Fatal("derived compaction changed authenticated inherited history")
			}
		}
		t.Log("ordinary lifecycle phase=derived-manual-compaction")
		collector.nextPhase("derived-manual-compaction")
		model.setPhase("compact")
		derivedCompact := request(http.MethodPost, "/v1/threads/"+forkID+"/compact", map[string]any{"reason": "manual"}, http.StatusOK)
		if derivedCompact["ok"] != true || derivedCompact["auto"] == true || contracts.StringField(derivedCompact, "sourceDigest") == "" {
			t.Fatal("derived manual compaction did not commit")
		}
		request(http.MethodGet, "/v1/threads/"+forkID, nil, http.StatusOK)
		assertPrefix()
		server.Close()
		shutdownOwnedRuntimeHandler(t, handler)
		fixture.async.assertDrained(t)
		server = nil
		handler, err = fixture.start()
		if err != nil {
			t.Fatal(err)
		}
		collector.bindHandler(t, handler)
		server = httptest.NewServer(handler)
		fixture.assertFault(4)
		afterCompact := request(http.MethodGet, "/v1/threads/"+forkID, nil, http.StatusOK)
		derivedMarker := runtimeProductionCaseCompactionMarkerV1(t, afterCompact)
		if derivedMarker["sourceDigest"] != derivedCompact["sourceDigest"] || derivedMarker["auto"] != false {
			t.Fatal("derived compaction binding did not survive fresh startup")
		}
		assertPrefix()
		sinceSeq, ok := contracts.NumericSeq(afterCompact["latestSeq"])
		if !ok || sinceSeq <= 0 {
			t.Fatal("complete compacted derived snapshot omitted its replay cursor")
		}
		run(forkID, "fork-after-compact")
		request(http.MethodGet, "/v1/threads/"+forkID, nil, http.StatusOK)
		assertPrefix()
		runtimeOptionalLifecycleAssertReplayV1(t, client, server.URL, forkID, sinceSeq, "", turns["fork-after-compact"], false, false)
	} else {
		fixture.assertFault(2)
	}
	t.Logf("R131 coverage fault=%s: Coding read/edit/local-test; Writing file; loopback Research; consumed Skill; ordinary MCP; real subagent/job wait+withheld-output; protected zero-effect refusal; manual compaction; fresh composition; public history/replay/resume+ordinary continuation", fault)
}

type runtimeOptionalLifecycleModelV1 struct {
	t               *testing.T
	mu              sync.Mutex
	phase           string
	steps           map[string]int
	lastToolResults map[string]int
	docURL          string
	calls           atomic.Int64
	childCalls      atomic.Int64
}

func (model *runtimeOptionalLifecycleModelV1) setPhase(phase string) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.phase = phase
}

func (model *runtimeOptionalLifecycleModelV1) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		model.t.Error(err)
		return
	}
	if bytes.Contains(body, []byte(runtimeOptionalDomainPrivateCanary)) {
		model.t.Error("private domain canary reached model input")
		return
	}
	model.calls.Add(1)
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil {
		model.t.Error("invalid synthetic provider request")
		return
	}
	lastUser, results := "", ""
	toolResultCount := 0
	lastToolResult := ""
	var resultCodes []string
	var collectCodes func(any)
	collectCodes = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "code" {
					if code, ok := child.(string); ok && len(code) < 100 {
						resultCodes = append(resultCodes, code)
					}
				}
				collectCodes(child)
			}
		case []any:
			for _, child := range value {
				collectCodes(child)
			}
		}
	}
	for _, message := range request.Messages {
		text, _ := json.Marshal(message.Content)
		if message.Role == "user" {
			lastUser = string(text)
		}
		if message.Role == "tool" {
			results += string(text)
			toolResultCount++
			lastToolResult = string(text)
			if content, ok := message.Content.(string); ok {
				var value any
				if json.Unmarshal([]byte(content), &value) == nil {
					collectCodes(value)
				}
			}
		}
	}
	model.mu.Lock()
	phase := model.phase
	child := strings.Contains(lastUser, "R131_BACKGROUND_CHILD")
	step := model.steps[phase]
	if !child {
		if step > 0 && (phase == "post-denial" || phase == "restart" || phase == "resume" || phase == "fork" || phase == "fork-restart" || phase == "resume-restart" || phase == "source-after-derive" || phase == "fork-after-compact") && toolResultCount <= model.lastToolResults[phase] {
			model.t.Errorf("phase %s did not supply a new current-turn tool result", phase)
		}
		model.lastToolResults[phase] = toolResultCount
		model.steps[phase]++
	}
	model.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	if child {
		model.childCalls.Add(1)
		runtimeOptionalLifecycleModelResponseV1(w, "", nil, "R131_BACKGROUND_CHILD_COMPLETE")
		return
	}
	check := func(marker string) {
		if !strings.Contains(results, marker) {
			model.t.Errorf("phase %s missing actual tool result marker %s; result_codes=%v", phase, marker, resultCodes)
		}
	}
	name, args, final := "", map[string]any{}, "R131_"+strings.ToUpper(phase)+"_COMPLETE"
	switch phase {
	case "coding":
		switch step {
		case 0:
			name, args = "read", map[string]any{"path": "normalize.cjs"}
		case 1:
			check("module.exports")
			name, args = "edit", map[string]any{"path": "normalize.cjs", "oldText": "name === 'q'", "newText": "name.toLowerCase() === 'q'"}
		case 2:
			name, args = "bash", map[string]any{"command": "node --test normalize.test.cjs", "timeout": 10}
		default:
			check("R131_CODING_TEST_OK")
		}
	case "writing":
		if step == 0 {
			name, args = "write", map[string]any{"path": "brief.md", "content": "# Ordinary work\nThe local coding regression passed.\n"}
		}
	case "research":
		if step == 0 {
			name, args = "web_fetch", map[string]any{"url": model.docURL}
		} else {
			check("R131_LOOPBACK_RESEARCH")
		}
	case "skills":
		if step == 0 {
			name, args = "run_skill", map[string]any{"name": "ordinary-writing", "arguments": "Apply the writing instruction to a note."}
		} else if step == 1 {
			if !bytes.Contains(body, []byte("R131_SKILL_CONSUMED")) {
				model.t.Error("skill entry was not actually consumed by the model")
			}
			name, args = "write", map[string]any{"path": "skill-applied.md", "content": "# R131_SKILL_CONSUMED\nThe local coding regression passed.\n"}
		}
	case "mcp":
		if step == 0 {
			name, args = mcp.CanonicalToolName("gui_schedule", "gui_schedule_list"), map[string]any{}
		} else {
			check("No scheduled tasks are configured.")
		}
	case "jobs":
		if step == 0 {
			name, args = "task", map[string]any{"prompt": "R131_BACKGROUND_CHILD: summarize the completed local coding task in one sentence.", "run_in_background": true}
		}
	case "post-denial", "restart", "resume", "fork", "fork-restart", "resume-restart", "source-after-derive", "fork-after-compact":
		if step == 0 {
			name, args = "read", map[string]any{"path": "brief.md"}
		} else {
			if !strings.Contains(lastToolResult, "The local coding regression passed.") {
				model.t.Errorf("phase %s did not read the real ordinary note", phase)
			}
		}
	case "compact":
		final = "The ordinary code regression passed; writing, research, skill and MCP work completed. A background child completed with withheld output. Protected work was refused. Continue ordinary work safely."
	default:
		model.t.Errorf("unexpected model call in phase %s", phase)
	}
	runtimeOptionalLifecycleModelResponseV1(w, name, args, final)
}

func runtimeOptionalLifecycleModelResponseV1(w http.ResponseWriter, name string, args map[string]any, final string) {
	delta := map[string]any{"content": final}
	finish := "stop"
	if name != "" {
		arguments, _ := json.Marshal(args)
		delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_" + strings.ReplaceAll(name, "-", "_"), "type": "function", "function": map[string]any{"name": name, "arguments": string(arguments)}}}}
		finish = "tool_calls"
	}
	chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
}

func runtimeOptionalLifecycleAssertTurnV1(t *testing.T, turn map[string]any, marker string, tools []string) {
	t.Helper()
	packagedPlanThenProtectedValidateOrdinaryReadV1(t, turn, marker)
	items, _ := turn["items"].([]any)
	for _, tool := range tools {
		call, result := false, false
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if item["toolName"] == tool {
				call = call || item["kind"] == "tool_call"
				if item["kind"] == "tool_result" {
					if item["isError"] == true || item["status"] != "completed" {
						output, _ := item["output"].(map[string]any)
						t.Fatalf("ordinary tool %s failed: status=%v code=%v", tool, item["status"], output["code"])
					}
					result = true
				}
			}
		}
		if !call || !result {
			t.Fatalf("ordinary %s did not traverse public tool-call/result pair", tool)
		}
	}
}

func runtimeOptionalLifecycleAssertFileV1(t *testing.T, workspace, name, expected string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(workspace, name))
	if err != nil || string(body) != expected {
		t.Fatalf("ordinary output %s was not written exactly", name)
	}
}

func runtimeOptionalLifecycleTurnIDsV1(turns map[string]string) []string {
	ids := make([]string, 0, len(turns))
	for _, id := range turns {
		ids = append(ids, id)
	}
	return ids
}

func runtimeOptionalLifecycleAssertReplayV1(t *testing.T, client *http.Client, baseURL, threadID string, sinceSeq int, protectedID, ordinaryID string, wantCompaction, wantRevoked bool) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/threads/%s/events?since_seq=%d", baseURL, threadID, sinceSeq), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal("public replay failed")
	}
	protected, ordinary, compaction, revoked := false, false, false, false
	var observed []string
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
			t.Fatal("invalid public replay event")
		}
		kind := contracts.StringField(event, "kind")
		observed = append(observed, kind+":"+contracts.StringField(event, "code")+":"+contracts.StringField(event, "phase"))
		protected = protected || kind == "accepted_final_batch" && event["turnId"] == protectedID
		// Case-lineage ordinary work is carried by the existing accepted-final
		// envelope; pre-case ordinary work uses a general terminal batch.
		if ordinaryID != "" && event["turnId"] == ordinaryID && (kind == "general_terminal_batch" || kind == "accepted_final_batch") {
			nested, _ := event["events"].([]any)
			for _, raw := range nested {
				member, _ := raw.(map[string]any)
				item, _ := member["item"].(map[string]any)
				ordinary = ordinary || member["kind"] == "item_completed" && item["kind"] == "assistant_text" &&
					item["threadId"] == threadID && item["turnId"] == ordinaryID && item["status"] == "completed" &&
					strings.Contains(contracts.StringField(item, "text"), "R131_")
			}
		}
		revoked = revoked || kind == "public_projection_revoked" && event["code"] == "case_public_authority_rejected"
		compaction = compaction || kind == "context_compacted" || kind == "compaction_completed"
	}
	if wantRevoked {
		if !revoked || len(observed) != 1 {
			t.Fatalf("old-epoch replay must only revoke current authority: events=%v", observed)
		}
		return
	}
	if revoked || (protectedID != "" && !protected) || (ordinaryID != "" && !ordinary) || (wantCompaction && !compaction) {
		t.Fatalf("public replay since=%d lost protected=%t ordinary=%t compaction=%t events=%v", sinceSeq, protected, ordinary, compaction, observed)
	}
}

func runtimeOptionalLifecycleWaitTurnV1(t *testing.T, client *http.Client, baseURL, threadID, turnID, phase string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, detail := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, baseURL, http.MethodGet, "/v1/threads/"+threadID, nil)
		if status != http.StatusOK && status != http.StatusServiceUnavailable {
			t.Fatalf("phase=%s history HTTP status=%d code=%s message=%.200s", phase, status, contracts.StringField(detail, "code"), contracts.StringField(detail, "message"))
		}
		turn := packagedSourceUnavailableHydrationTurnV1(detail, turnID)
		if turn != nil {
			state := contracts.StringField(turn, "status")
			if state == "completed" {
				return turn
			}
			if state == "failed" || state == "interrupted" || state == "cancelled" {
				codes := []string{}
				items, _ := turn["items"].([]any)
				for _, raw := range items {
					item, _ := raw.(map[string]any)
					if item["kind"] == "error" {
						codes = append(codes, contracts.StringField(item, "code"))
					}
				}
				t.Fatalf("phase=%s ended status=%s codes=%v", phase, state, codes)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("phase=%s did not publish completed history within 30s", phase)
	return nil
}

// This task-only corpus fixture must use an actual path that survives the
// unchanged desktop PII guard. t.TempDir adds numeric ancestors before /001.
// No environment variable may redirect these public fixture writes.
const runtimeOptionalPublicFixtureScopeV1 = "/Volumes/AnalytixCache/development-v3/tmp/own2-shared-public-consumer-20260911"

var runtimeOptionalPublicWorkspacePatternV1 = regexp.MustCompile("^" + regexp.QuoteMeta(runtimeOptionalPublicFixtureScopeV1) + `/fixture-[a-p]{32}/ordinary-workspace$`)

func runtimeOptionalPublicFixtureRootV1() (string, func() error, error) {
	parent, err := os.Lstat(runtimeOptionalPublicFixtureScopeV1)
	realParent, realErr := filepath.EvalSymlinks(runtimeOptionalPublicFixtureScopeV1)
	if err != nil || realErr != nil || !parent.IsDir() || parent.Mode().Perm() != 0o700 || realParent != runtimeOptionalPublicFixtureScopeV1 {
		return "", nil, errors.New("public fixture parent is not the exact task-owned directory")
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", nil, err
	}
	var suffix strings.Builder
	for _, value := range entropy {
		suffix.WriteByte('a' + value>>4)
		suffix.WriteByte('a' + value&15)
	}
	root := filepath.Join(runtimeOptionalPublicFixtureScopeV1, "fixture-"+suffix.String())
	if !runtimeOptionalPublicWorkspacePatternV1.MatchString(filepath.Join(root, "ordinary-workspace")) {
		return "", nil, errors.New("public fixture complete workspace path is invalid")
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return "", nil, err
	}
	identity, err := os.Lstat(root)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() error {
		current, err := os.Lstat(root)
		if err != nil {
			return err
		}
		currentParent, err := os.Lstat(runtimeOptionalPublicFixtureScopeV1)
		if err != nil || !os.SameFile(parent, currentParent) || !os.SameFile(identity, current) || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 {
			return errors.New("public fixture cleanup identity changed")
		}
		return os.RemoveAll(root)
	}
	return root, cleanup, nil
}

func TestRuntimeOptionalPublicFixturePathV1(t *testing.T) {
	if os.Getenv("ANALYTIX_TEST_SHARED_PUBLIC_CONSUMER_V1") == "" {
		t.Skip("NOT_CONFIGURED: task-owned public fixture validation is inactive")
	}
	if os.Getenv("ANALYTIX_TEST_SHARED_PUBLIC_CONSUMER_V1") != "1" {
		t.Fatal("invalid public fixture validation activation")
	}
	root, cleanup, err := runtimeOptionalPublicFixtureRootV1()
	if err != nil {
		t.Fatal("public fixture allocation failed")
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			if err := cleanup(); err != nil {
				t.Error("public fixture cleanup failed")
			}
		}
	})
	workspace := filepath.Join(root, "ordinary-workspace")
	if !runtimeOptionalPublicWorkspacePatternV1.MatchString(workspace) {
		t.Fatal("complete public workspace failed its closed path contract")
	}
	if runtimeOptionalPublicWorkspacePatternV1.MatchString(filepath.Join(runtimeOptionalPublicFixtureScopeV1, "fixture-1234567890", "001", "ordinary-workspace")) {
		t.Fatal("unsafe numeric ancestor was admitted")
	}
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal("workspace creation failed")
	}
	file := filepath.Join(workspace, "restart-reuse.txt")
	if err := os.WriteFile(file, []byte("R131_FIXTURE_REUSE"), 0o600); err != nil {
		t.Fatal("fixture write failed")
	}
	// The actual lifecycle reuses this path for restart/resume; reopening must
	// address the same physical workspace, not a redacted or substituted path.
	reopened, err := filepath.EvalSymlinks(workspace)
	body, readErr := os.ReadFile(filepath.Join(reopened, "restart-reuse.txt"))
	if err != nil || readErr != nil || reopened != workspace || string(body) != "R131_FIXTURE_REUSE" {
		t.Fatal("fixture restart reuse changed identity or content")
	}
	if err := cleanup(); err != nil {
		t.Fatal("exact fixture cleanup failed")
	}
	cleaned = true
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture root survived cleanup")
	}
}
