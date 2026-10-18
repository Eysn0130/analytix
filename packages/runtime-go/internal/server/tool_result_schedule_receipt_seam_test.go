package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	snapshotstore "analytix.local/runtime-go/internal/adapters/outbound/toolresultsnapshot"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	mcp "analytix.local/runtime-go/internal/mcp"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	provider "analytix.local/runtime-go/internal/provider"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

const scheduleReceiptFixtureEntryV1 = "schedule-receipt-mcp-fixture-v1.json"

// The child is a task-owned synthetic stdio server. The exact native binding
// argv is recognized before testing parses flags; it never runs TestMain,
// ScheduleRuntime, a provider, or an HTTP scheduling endpoint.
func init() {
	if len(os.Args) != 5 || filepath.Base(os.Args[1]) != scheduleReceiptFixtureEntryV1 ||
		os.Args[2] != "--gui-schedule-mcp-server" || os.Args[3] != "--base-url" || os.Args[4] != "http://127.0.0.1:9787" {
		return
	}
	body, err := os.ReadFile(os.Args[1])
	if err != nil {
		os.Exit(2)
	}
	var fixture map[string]json.RawMessage
	if json.Unmarshal(body, &fixture) != nil {
		os.Exit(2)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 16*1024), 1024*1024)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ID) == 0 {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "initialize":
			response["result"] = map[string]any{"protocolVersion": "2025-11-25", "serverInfo": map[string]any{"name": "analytix-schedule", "version": "0.1.0"}, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			tools := []any{}
			for _, operation := range []string{"create", "update", "delete"} {
				tools = append(tools, map[string]any{"name": "gui_schedule_" + operation, "description": "Synthetic native Schedule receipt", "inputSchema": scheduleReceiptInputSchemaV1(operation), "outputSchema": scheduleReceiptOutputSchemaV1(operation)})
			}
			response["result"] = map[string]any{"tools": tools}
		case "tools/call":
			name, _ := request.Params["name"].(string)
			result, present := fixture[name]
			if !present {
				response["error"] = map[string]any{"code": -32602, "message": "Unknown fixture tool"}
			} else {
				response["result"] = result
			}
		default:
			response["error"] = map[string]any{"code": -32601, "message": "Unknown fixture method"}
		}
		if encoder.Encode(response) != nil {
			os.Exit(2)
		}
	}
	if scanner.Err() != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func scheduleReceiptInputSchemaV1(operation string) map[string]any {
	text := map[string]any{"type": "string"}
	if operation == "delete" {
		return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"task_id"}, "properties": map[string]any{"task_id": map[string]any{"type": "string", "minLength": 1}}}
	}
	schedules := []any{
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "atTime"}, "properties": map[string]any{"kind": map[string]any{"const": "at"}, "atTime": map[string]any{"type": "string", "minLength": 1}}},
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "timeOfDay"}, "properties": map[string]any{"kind": map[string]any{"const": "daily"}, "timeOfDay": map[string]any{"type": "string", "pattern": `^([01]\d|2[0-3]):[0-5]\d$`}}},
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "everyMinutes"}, "properties": map[string]any{"kind": map[string]any{"const": "interval"}, "everyMinutes": map[string]any{"type": "integer", "minimum": 1, "maximum": 10080}}},
	}
	properties := map[string]any{"title": text, "prompt": text, "enabled": map[string]any{"type": "boolean"}, "workspace_root": text, "claw_channel_id": text, "provider_id": text, "model": text,
		"reasoning_effort": map[string]any{"type": "string", "enum": []string{"auto", "off", "low", "medium", "high", "max"}}, "mode": map[string]any{"type": "string", "enum": []string{"agent", "plan"}}}
	required := []string{"title", "prompt", "schedule"}
	if operation == "update" {
		required = []string{"task_id"}
		properties["task_id"] = map[string]any{"type": "string", "minLength": 1}
		schedules = append(schedules, map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind"}, "properties": map[string]any{"kind": map[string]any{"const": "manual"}}})
	} else {
		properties["title"] = map[string]any{"type": "string", "minLength": 1}
		properties["prompt"] = map[string]any{"type": "string", "minLength": 1}
	}
	properties["schedule"] = map[string]any{"anyOf": schedules}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}

func scheduleReceiptOutputSchemaV1(operation string) map[string]any {
	text := map[string]any{"type": "string"}
	properties := map[string]any{"status": map[string]any{"type": "string", "enum": []string{"success", "failure"}}, "message": text}
	required := []string{"status", "message"}
	if operation == "delete" {
		properties["taskId"] = text
		required = append(required, "taskId")
	} else {
		taskProperties := map[string]any{}
		taskRequired := []string{"id", "title", "enabled", "prompt", "workspaceRoot", "clawChannelId", "model", "reasoningEffort", "mode", "schedule", "createdAt", "updatedAt", "lastRunAt", "nextRunAt", "lastStatus", "lastMessage", "lastThreadId"}
		for _, key := range taskRequired {
			taskProperties[key] = text
		}
		taskProperties["providerId"] = text
		taskProperties["enabled"] = map[string]any{"type": "boolean"}
		taskProperties["reasoningEffort"] = map[string]any{"type": "string", "enum": []string{"auto", "off", "low", "medium", "high", "max"}}
		taskProperties["mode"] = map[string]any{"type": "string", "enum": []string{"agent", "plan"}}
		taskProperties["lastStatus"] = map[string]any{"type": "string", "enum": []string{"idle", "running", "success", "error"}}
		taskProperties["schedule"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "everyMinutes", "timeOfDay", "atTime"}, "properties": map[string]any{
			"kind": map[string]any{"type": "string", "enum": []string{"manual", "at", "daily", "interval"}}, "everyMinutes": map[string]any{"type": "number"}, "timeOfDay": text, "atTime": text,
		}}
		task := map[string]any{"type": "object", "additionalProperties": false, "properties": taskProperties, "required": taskRequired}
		properties["task"] = map[string]any{"anyOf": []any{task, map[string]any{"type": "null"}}}
		required = append(required, "task")
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}

func scheduleReceiptTransportResultV1(operation, status string, isError bool, omitIsError bool) map[string]any {
	structured := map[string]any{"status": status, "message": "Actual committed " + operation}
	if operation == "delete" {
		structured["taskId"] = "actual-native-task"
	} else {
		var task any
		if status == "success" {
			task = map[string]any{"id": "actual-native-task", "title": "Actual committed title", "enabled": true, "prompt": "PRIVATE_NATIVE_PROMPT", "workspaceRoot": "/synthetic/native-target", "clawChannelId": "", "providerId": "actual-native-provider", "model": "actual-native-model", "reasoningEffort": "auto", "mode": "agent",
				"schedule": map[string]any{"kind": "daily", "everyMinutes": 60, "timeOfDay": "09:00", "atTime": ""}, "createdAt": "actual-created", "updatedAt": "actual-updated", "lastRunAt": "", "nextRunAt": "", "lastStatus": "idle", "lastMessage": "schedule_task_idle", "lastThreadId": ""}
		}
		structured["task"] = task
	}
	result := map[string]any{"content": []any{}, "structuredContent": structured}
	if !omitIsError {
		result["isError"] = isError
	}
	return result
}

func scheduleReceiptProductionManagerV1(t *testing.T, result map[string]any, protectedRoots []string) *mcp.ProductionManager {
	t.Helper()
	entry := filepath.Join(t.TempDir(), scheduleReceiptFixtureEntryV1)
	fixture := map[string]any{}
	for _, operation := range []string{"create", "update", "delete"} {
		fixture["gui_schedule_"+operation] = result
	}
	body, err := json.Marshal(fixture)
	if err != nil || os.WriteFile(entry, body, 0600) != nil {
		t.Fatal("write isolated Schedule transport fixture")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := mcp.ServerSpec{ID: "gui_schedule", Transport: "stdio", Command: executable, Args: []string{entry, "--gui-schedule-mcp-server", "--base-url", "http://127.0.0.1:9787"}, Env: map[string]string{"ELECTRON_RUN_AS_NODE": "1"}, TrustScope: "user", TimeoutMS: 5000}
	roots := make([]string, len(protectedRoots))
	for index, root := range protectedRoots {
		roots[index], err = filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatal("canonicalize isolated protected root", err)
		}
	}
	manager := mcp.NewProductionManagerWithOptions(mcp.BindHostScheduleMCPServerV1([]mcp.ServerSpec{spec}, &spec), mcp.ProductionManagerOptions{ProtectedReadDirs: roots})
	t.Cleanup(manager.Disconnect)
	manager.RefreshCatalog()
	if _, live := manager.ToolConnectionEpoch("mcp__gui_schedule__gui_schedule_create"); !live {
		t.Fatalf("synthetic native stdio transport did not connect: %v", manager.ServerDiagnostics())
	}
	return manager
}

func scheduleReceiptPendingV1(t *testing.T, h *runtimeServerHandler, manager *mcp.ProductionManager, parent runtimePendingToolCall, operation string) runtimePendingToolCall {
	t.Helper()
	pending := parent
	tool := "mcp__gui_schedule__gui_schedule_" + operation
	arguments := map[string]any{"task_id": "actual-native-task"}
	if operation == "create" {
		arguments = map[string]any{"title": "Actual committed title", "prompt": "PRIVATE_NATIVE_PROMPT", "schedule": map[string]any{"kind": "daily", "timeOfDay": "09:00"}}
	} else if operation == "update" {
		arguments["title"] = "Actual committed title"
	}
	argumentJSON, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	call := provider.ToolCall{ID: serverTestHostToolCallID("synthetic_schedule_" + operation), Name: tool, Arguments: argumentJSON}
	advertisements := toolcatalogapp.MCPToolAdvertisementsByNameV1(toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, parent.SecurityContext), []string{tool})
	if len(advertisements) != 1 {
		t.Fatal("native fixture lacks one verified current advertisement")
	}
	advertisement := advertisements[0]
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPAdvertisements(false, nil, false, false, "synthetic native schedule", false, advertisements)
	scope := []string{}
	for _, schema := range schemas {
		scope = append(scope, schema.Name)
	}
	grant, err := executiongrantapp.IssueProvider(parent.SecurityContext, "synthetic", call, schemas, scope, advertisement.ReadOnly, "not_required", advertisement.ConnectionEpoch, advertisement.ServerIdentity, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	pending.Call, pending.ExecutionGrant, pending.ToolScope = call, grant, scope
	pending.Prompt, pending.ApprovalPolicy = "synthetic native schedule", "auto"
	pending.ToolCallItemID, err = h.persistToolCallReady(context.Background(), parent.ThreadID, parent.TurnID, call, 1, parent.SecurityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	pending.ModelVisibleToolDefinition = domainmodel.FreezeModelVisibleToolDefinitionsV1(schemas)[tool]
	if err := h.authorizeRuntimePending(context.Background(), pending, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return pending
}

func TestNativeScheduleActualTransportSettlementPrivateHTTPAndRestartV1(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			h, initial, durable, root := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.go"})
			manager := scheduleReceiptProductionManagerV1(t, scheduleReceiptTransportResultV1(operation, "success", false, true), []string{root, durable})
			h.mcp = manager
			pending := scheduleReceiptPendingV1(t, h, manager, initial, operation)
			var actual map[string]any
			settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, func(output any, isError bool) (any, bool, error) {
				actual, _ = output.(map[string]any)
				return output, isError, nil
			})
			if err != nil || settled.IsError || actual["executed"] != true || actual["transportStatus"] != "success" {
				t.Fatalf("native Schedule settlement failed: %v; code=%v executed=%v transport=%v", err, actual["code"], actual["executed"], actual["transportStatus"])
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			receipt := display.Capture.Schedule
			if status != http.StatusOK || display.Capture.Kind != "schedule" || receipt == nil || receipt.Operation != operation || receipt.Status != "success" || display.Inspect == nil {
				t.Fatal("actual native receipt did not traverse private HTTP")
			}
			if operation != "delete" && (receipt.Task == nil || receipt.Task.ID != "actual-native-task" || receipt.Task.WorkspaceRoot != "/synthetic/native-target" || receipt.Task.LastThreadID != "" || receipt.Task.ProviderID == nil || *receipt.Task.ProviderID != "actual-native-provider") {
				t.Fatal("native identities were replaced by source chat or guessed")
			}
			if operation == "delete" && (receipt.TaskID == nil || *receipt.TaskID != "actual-native-task" || receipt.Task != nil) {
				t.Fatal("delete receipt lost actual identity")
			}
			assertPublicToolSeamV1(t, h, pending, "PRIVATE_NATIVE_PROMPT")
			assertPublicToolSeamV1(t, h, pending, "/synthetic/native-target")
			assertPublicToolSeamV1(t, h, pending, "Actual committed title")
			primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			result := protectedSeamResultV1(t, primary.Thread, pending)
			before := h.store.WriteAttempts()
			if err := h.store.EnsureProtectedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, result); err != nil {
				t.Fatal("exact native retry refused", err)
			}
			conflict := cloneMap(result)
			binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(conflict[domaintoolresult.ProtectedSnapshotBindingFieldV1])
			if err != nil {
				t.Fatal(err)
			}
			binding.BodyDigest = strings.Repeat("f", 64)
			conflict[domaintoolresult.ProtectedSnapshotBindingFieldV1] = domaintoolresult.ProtectedSnapshotBindingRecordV1(binding)
			if h.store.EnsureProtectedToolResultSettlementExact(context.Background(), pending.ThreadID, pending.TurnID, pending.ToolCallItemID, conflict) == nil || h.store.WriteAttempts() != before {
				t.Fatal("native conflicting retry altered immutable primary")
			}
			// Disconnect the producer: historical CAS remains available unchanged.
			manager.Disconnect()
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
			if status != http.StatusOK || after.SnapshotDigest != display.SnapshotDigest || !reflect.DeepEqual(after.Capture.Schedule, receipt) {
				t.Fatal("restart changed or re-fetched committed Schedule receipt")
			}
			foreign := pending
			foreign.ThreadID = "other-source-thread"
			if code, _, _ := readProtectedSeamV1(t, h, foreign); code != http.StatusNotFound {
				t.Fatal("cross-thread selector opened native receipt")
			}
			h.turnSecurity.Identity = nil
			status, _, response := readProtectedSeamV1(t, h, pending)
			if status != http.StatusNotFound || strings.Contains(response, "capture") || strings.Contains(response, "PRIVATE_NATIVE_PROMPT") {
				t.Fatal("revoked authority opened native receipt")
			}
		})
	}
}

type scheduleReceiptCancelAfterCASV1 struct {
	snapshotport.Store
	after   func() error
	binding domaintoolresult.ProtectedSnapshotBindingV1
	puts    int
}

func (store *scheduleReceiptCancelAfterCASV1) Put(ctx context.Context, snapshot domaintoolresult.ProtectedSnapshotV1) (domaintoolresult.ProtectedSnapshotBindingV1, error) {
	binding, err := store.Store.Put(ctx, snapshot)
	if err != nil || snapshot.Capture.Kind != "schedule" {
		return binding, err
	}
	store.puts++
	store.binding = binding
	if err := store.after(); err != nil {
		return binding, err
	}
	return binding, nil
}

func TestNativeScheduleCancellationAfterCASLeavesUnreadableOrphanV1(t *testing.T) {
	h, initial, durable, root := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.go"})
	manager := scheduleReceiptProductionManagerV1(t, scheduleReceiptTransportResultV1("create", "success", false, true), []string{root, durable})
	h.mcp = manager
	pending := scheduleReceiptPendingV1(t, h, manager, initial, "create")
	realStore := h.toolSnapshots
	fault := &scheduleReceiptCancelAfterCASV1{Store: realStore, after: func() error {
		return turnapp.PersistFailure(turnapp.PersistFailureInput{Store: h.store, SecurityContext: pending.SecurityContext, TerminalReason: "cancel", ThreadID: pending.ThreadID, TurnID: pending.TurnID,
			FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Failure: domainfailure.New(domainfailure.CodeTurnCancelled, nil), Interrupt: &turnapp.GeneralTerminalInterruptMetadata{Cancelled: true}})
	}}
	h.toolSnapshots = fault
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil); err == nil {
		t.Fatal("terminal winner admitted a late native result")
	}
	orphan, err := realStore.Read(context.Background(), fault.binding)
	if err != nil || fault.puts != 1 || orphan.Capture.Kind != "schedule" || orphan.Capture.Schedule == nil || orphan.Capture.Schedule.Task == nil || orphan.Capture.Schedule.Task.Prompt != "PRIVATE_NATIVE_PROMPT" {
		t.Fatal("fixture did not retain one actual CAS orphan", err)
	}
	h.toolSnapshots = realStore
	if code, _, response := readProtectedSeamV1(t, h, pending); code != http.StatusNotFound || strings.Contains(response, "capture") {
		t.Fatal("cancellation orphan opened before restart")
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range listAny(primary.Thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") != pending.TurnID {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == pending.Call.ID {
				t.Fatal("cancelled native result entered primary")
			}
		}
	}
	assertPublicToolSeamV1(t, h, pending, "PRIVATE_NATIVE_PROMPT")
	reopenProtectedJobSnapshotOwnersV1(t, h, root, durable)
	if code, _, response := readProtectedSeamV1(t, h, pending); code != http.StatusNotFound || strings.Contains(response, "capture") {
		t.Fatal("cancellation orphan opened after restart")
	}
}

func TestNativeScheduleActualFailureAndInconsistentTransportV1(t *testing.T) {
	for _, fixture := range []struct {
		name, status     string
		isError, capture bool
	}{
		{"failure", "failure", true, true}, {"success-is-error", "success", true, false}, {"failure-is-success", "failure", false, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			h, initial, durable, root := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.go"})
			manager := scheduleReceiptProductionManagerV1(t, scheduleReceiptTransportResultV1("create", fixture.status, fixture.isError, false), []string{root, durable})
			h.mcp = manager
			pending := scheduleReceiptPendingV1(t, h, manager, initial, "create")
			var actual map[string]any
			_, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, func(output any, isError bool) (any, bool, error) {
				actual, _ = output.(map[string]any)
				return output, isError, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if actual["executed"] != true || actual["transportStatus"] != "success" {
				t.Fatalf("negative fixture did not reach native transport: code=%v executed=%v", actual["code"], actual["executed"])
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if fixture.capture {
				if status != http.StatusOK || display.Capture.Kind != "schedule" || display.Capture.Status != "failed" || display.Capture.Schedule.Status != "failure" || display.Capture.Schedule.Task != nil {
					t.Fatal("actual failure receipt was changed")
				}
			} else if status == http.StatusOK && display.Capture.Kind == "schedule" {
				t.Fatal("inconsistent output acquired a native receipt")
			}
			assertPublicToolSeamV1(t, h, pending, "PRIVATE_NATIVE_PROMPT")
		})
	}
}

func TestNativeScheduleFinalSettlementFailureCannotSealSuccessfulCaptureV1(t *testing.T) {
	h, initial, durable, root := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.go"})
	manager := scheduleReceiptProductionManagerV1(t, scheduleReceiptTransportResultV1("create", "success", false, false), []string{root, durable})
	h.mcp = manager
	pending := scheduleReceiptPendingV1(t, h, manager, initial, "create")
	transformed := false
	_, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, func(output any, isError bool) (any, bool, error) {
		actual, _ := output.(map[string]any)
		if isError || actual["executed"] != true || actual["transportStatus"] != "success" {
			t.Fatalf("final downgrade did not receive native success: code=%v executed=%v", actual["code"], actual["executed"])
		}
		transformed = true
		return map[string]any{"code": "synthetic_final_refusal", "error": "Synthetic final settlement refusal", "executed": true}, true, nil
	})
	if err != nil || !transformed {
		t.Fatal("native success did not enter final transform", err)
	}
	status, display, response := readProtectedSeamV1(t, h, pending)
	if status == http.StatusOK && display.Capture.Kind == "schedule" || strings.Contains(response, "PRIVATE_NATIVE_PROMPT") {
		t.Fatal("successful producer capture survived failed final settlement")
	}
	assertPublicToolSeamV1(t, h, pending, "PRIVATE_NATIVE_PROMPT")
}
