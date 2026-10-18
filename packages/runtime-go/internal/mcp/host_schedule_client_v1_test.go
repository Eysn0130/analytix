package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	appmcp "analytix.local/runtime-go/internal/app/mcp"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func hostScheduleClientFixtureV1(t *testing.T, handler http.HandlerFunc, admission func(context.Context) error) (*ProductionManager, ServerSpec) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	private := hostScheduleMCPBindingFixtureV1()
	private.Args[3] = server.URL
	private.Args = append(private.Args, "--secret", "synthetic-schedule-secret")
	specs := BindHostScheduleMCPServerV1([]ServerSpec{private}, &private)
	m := NewProductionManagerWithOptions(specs, ProductionManagerOptions{HostScheduleServer: &private})
	if err := m.BindEffectAdmission(admission); err != nil {
		t.Fatal(err)
	}
	m.Connect()
	t.Cleanup(m.Disconnect)
	if _, ok := m.clients[private.ID].(*hostScheduleClientV1); !ok {
		t.Fatalf("private typed client missing: %#v", m.failures)
	}
	return m, private
}

func TestHostScheduleTypedListCoexistsWithEditingCapture(t *testing.T) {
	registry, path := editingAdmissionFixture(t)
	var calls atomic.Int32
	m, _ := hostScheduleClientFixtureV1(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/schedule/internal/list" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-schedule-secret" {
			t.Error("unexpected Host request")
		}
		_, _ = w.Write([]byte(`{"ok":true,"tasks":[]}`))
	}, registry.BeginOpaque)
	if calls.Load() != 0 {
		t.Fatal("typed constructor/catalog caused Host request")
	}
	release, err := registry.Capture(context.Background(), "office-capture", path)
	if err != nil {
		t.Fatal(err)
	}
	result := m.CallTool("mcp__gui_schedule__gui_schedule_list", true, map[string]any{})
	if result["executed"] != true || result["isError"] == true || calls.Load() != 1 {
		t.Fatalf("controlled list failed: %#v", result)
	}
	for _, operation := range []string{"create", "update", "delete"} {
		args := map[string]any{"task_id": "task"}
		if operation == "create" {
			args = map[string]any{"title": "task", "prompt": "synthetic", "schedule": map[string]any{"kind": "interval", "everyMinutes": 1}}
		}
		output := m.CallTool("mcp__gui_schedule__gui_schedule_"+operation, true, args)
		if output["executed"] != false || calls.Load() != 1 {
			t.Fatalf("%s crossed capture barrier: %#v", operation, output)
		}
	}
	release()
	release, err = registry.Capture(context.Background(), "after-list", path)
	if err != nil {
		t.Fatal("list tainted registry", err)
	}
	release()
}

func TestHostSchedulePrivateClientIdentityCannotTransferToReplacement(t *testing.T) {
	registry, path := editingAdmissionFixture(t)
	m, private := hostScheduleClientFixtureV1(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"ok":true,"tasks":[]}`)) }, registry.BeginOpaque)
	var admissions atomic.Int32
	other := NewProductionManager(nil)
	if err := other.BindEffectAdmission(func(context.Context) error { admissions.Add(1); return errors.New("denied") }); err != nil {
		t.Fatal(err)
	}
	client := m.clients[private.ID]
	if err := other.admitMCPEffect(context.Background(), client, "gui_schedule_list"); err == nil || admissions.Load() != 1 {
		t.Fatal("private capability transferred between owners")
	}
	oldEpoch := m.connectionEpochs[private.ID]
	m.RestartReconnect()
	if output := m.CallToolBoundContext(context.Background(), "mcp__gui_schedule__gui_schedule_list", true, oldEpoch, map[string]any{}); output["executed"] != false {
		t.Fatal("stale epoch retained controlled authority")
	}
	release, err := registry.Capture(context.Background(), "replacement-capture", path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	fake := &connectionEpochClient{}
	m.clients[private.ID] = fake
	if output := m.CallToolBoundContext(context.Background(), "mcp__gui_schedule__gui_schedule_list", true, m.connectionEpochs[private.ID], map[string]any{}); output["executed"] != false || fake.calls != 0 {
		t.Fatal("same-name generic replacement bypassed admission")
	}
	// Same metadata/name/readonly policy cannot turn the replacement into the
	// concrete private client, even when a caller presents a current epoch.
	if _, ok := m.clients[private.ID].(*hostScheduleClientV1); ok {
		t.Fatal("fake acquired private type")
	}
	if err := other.admitMCPEffect(context.Background(), fake, "gui_schedule_list"); err == nil || fake.calls != 0 {
		t.Fatal("generic same-name replacement bypassed admission")
	}
	for _, mutate := range []func(*ServerSpec){func(s *ServerSpec) { s.Args[5] = "different" }, func(s *ServerSpec) { s.AccountCredential = &domainmcp.AccountCredentialScope{Provider: "unknown"} }, func(s *ServerSpec) { s.BackgroundStart = true }} {
		changed := cloneServerSpecForAuthority(private)
		mutate(&changed)
		if exactHostScheduleClientBindingV1(m.specs, &changed) != nil {
			t.Fatal("changed private projection retained capability")
		}
	}
}

func TestHostScheduleTypedSchemasAndMalformedBackend(t *testing.T) {
	tools := hostScheduleToolsV1()
	schemas := make([]domainmodel.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		schemas = append(schemas, domainmodel.ToolSchema{Name: tool.Name, Parameters: tool.InputSchema})
	}
	if len(tools) != 8 || toolcatalogapp.ValidateProviderVisibleToolSchemas(schemas) != nil {
		t.Fatal("invalid fixed catalog")
	}
	m, _ := hostScheduleClientFixtureV1(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"tasks":[],"injected":true}`))
	}, allowMCPTestEffect)
	result := m.CallTool("mcp__gui_schedule__gui_schedule_list", true, map[string]any{})
	if result["executed"] != true || result["isError"] != true {
		t.Fatalf("malformed backend accepted: %#v", result)
	}
	for i := 0; i < 10; i++ {
		before, _ := json.Marshal(tools)
		after, _ := json.Marshal(hostScheduleToolsV1())
		if string(before) != string(after) {
			t.Fatal("catalog schema hash is unstable")
		}
	}
}

func TestHostScheduleTypedCreatePreservesOriginAndProtectedReceipt(t *testing.T) {
	var requests atomic.Int32
	var rawResult map[string]any
	if json.Unmarshal(scheduleReceiptRawV1("success", false), &rawResult) != nil {
		t.Fatal("invalid task fixture")
	}
	task := rawResult["structuredContent"].(map[string]any)["task"]
	m, _ := hostScheduleClientFixtureV1(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid create body")
		}
		origin, ok := request["origin"].(map[string]any)
		input, inputOK := request["input"].(map[string]any)
		if !ok || !inputOK || len(request) != 2 || len(origin) != 4 || origin["threadId"] != "thread-schedule" || origin["toolCallId"] == "" || input["prompt"] != "synthetic prompt" || r.Header.Get("X-Analytix-Schedule-Origin") != "typed-v1" || r.URL.Path != "/schedule/internal/create" {
			t.Error("typed origin/input changed")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "task": task})
	}, allowMCPTestEffect)
	_, _, input := scheduleReceiptManagerFixtureV1(t, scheduleReceiptRawV1("success", false))
	input.Arguments = map[string]any{"title": "task", "prompt": "synthetic prompt", "schedule": map[string]any{"kind": "interval", "everyMinutes": 1}}
	input.Call.Arguments, _ = json.Marshal(input.Arguments)
	input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: input.SecurityContext, Provider: "synthetic", ServerIdentity: m.serverIdentities[hostScheduleMCPServerIDV1], ToolName: input.ToolName, ToolCallID: input.Call.ID, ConnectionEpoch: m.connectionEpochs[hostScheduleMCPServerIDV1], ArgsHash: domainsecurity.CanonicalJSONHash(input.Call.Arguments), SchemaHash: managedToolSchemaHash(input.ToolName, m.tools[input.ToolName]), ScopeHash: input.ExecutionGrant.ScopeHash, ApprovalState: "approved", IssuedAt: time.Now().UTC()})
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	output, failed := appmcp.ExecuteTool(ctx, m, input)
	capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if failed || !ok || capture.Schedule == nil || capture.Schedule.Task == nil || capture.Schedule.Task.ID != "task-1" || requests.Load() != 1 {
		t.Fatalf("typed create receipt missing: %#v", output)
	}
}
