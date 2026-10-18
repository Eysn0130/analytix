package toolresult

import (
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestProtectedErrorSharesOneCollectorWithTypedCaptureAndCannotReviveV1(t *testing.T) {
	capture, ok := NewProtectedTodoCaptureV1(map[string]any{"threadId": "thread-synthetic", "updatedAt": "2026-10-10T00:00:00Z", "items": []any{}}, nil, true)
	if !ok {
		t.Fatal("typed fixture rejected")
	}
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, capture)
	value := ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "actual source failure"}})
	CaptureProtectedToolErrorV1(ctx, value)
	value.Entries[0].Message = "replacement"
	actual, present, error_ := ConsumeProtectedToolObservationV1(ctx)
	if !present || actual.Todo == nil || error_ == nil || error_.Entries[0].Message != "actual source failure" {
		t.Fatal("error capture overwrote Todo or borrowed its message")
	}
	CaptureProtectedToolErrorV1(ctx, ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "late child failure"}}))
	CaptureProtectedToolResultV1(ctx, capture)
	if _, present, error_ := ConsumeProtectedToolObservationV1(ctx); present || error_ != nil {
		t.Fatal("late background error rewrote a sealed parent acceptance")
	}
	empty := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	ConsumeProtectedToolObservationV1(empty)
	CaptureProtectedToolErrorV1(empty, ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "late"}}))
	if _, _, error_ := ConsumeProtectedToolObservationV1(empty); error_ != nil {
		t.Fatal("empty consumed collector reopened")
	}
}

func TestProtectedErrorSharesOneCollectorWithActualJobRecordsV1(t *testing.T) {
	capture := ProtectedCaptureV1{Kind: "job", Status: "failed", Job: &ProtectedJobCaptureV1{
		Operation: "spawn", Jobs: []ProtectedJobRecordV1{{JobID: "actual-child-3", Kind: "subagent", Status: "failed", Output: ProtectedJobOutputV1{Availability: "unavailable"}}}, Total: 1,
	}}
	if err := ValidateProtectedCaptureV1(capture); err != nil {
		t.Fatal(err)
	}
	index := 3
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, capture)
	CaptureProtectedToolErrorV1(ctx, ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "actual child failure", ParallelIndex: &index, JobID: "actual-child-3"}}))
	capture.Job.Jobs[0].JobID = "replacement"
	index = 1
	actual, present, error_ := ConsumeProtectedToolObservationV1(ctx)
	if !present || actual.Job == nil || actual.Job.Jobs[0].JobID != "actual-child-3" || error_ == nil || len(error_.Entries) != 1 || error_.Entries[0].JobID != "actual-child-3" || *error_.Entries[0].ParallelIndex != 3 {
		t.Fatal("error attachment overwrote the Job capture or lost actual parallel ownership")
	}
	if _, present, error_ := ConsumeProtectedToolObservationV1(ctx); present || error_ != nil {
		t.Fatal("Job observation was consumed twice")
	}
}

func TestProtectedDomainCaptureV1SelectedRecordsAndOwnership(t *testing.T) {
	goal := map[string]any{"id": "goal-synthetic", "threadId": "thread-synthetic", "objective": "  committed projected objective  ", "status": "blocked", "tokenBudget": 100, "tokensUsed": 12, "timeUsedSeconds": 3,
		"blockedReason": "  committed blocker  ", "createdAt": "2026-10-10T00:00:00Z", "updatedAt": "2026-10-10T00:00:01Z", "evidenceLedger": []any{"PRIVATE_LEDGER"}}
	goalCapture, ok := NewProtectedGoalCaptureV1(goal)
	if !ok || *goalCapture.Goal.RemainingTokens != 88 || goalCapture.Goal.Goal.Objective != goal["objective"] {
		t.Fatal("actual goal not captured")
	}
	todo := map[string]any{"threadId": "thread-synthetic", "updatedAt": "2026-10-10T00:00:00Z", "items": []any{map[string]any{
		"id": "todo-1", "content": "committed projected task", "status": "failed", "statusReasonCode": "tool_failed", "note": "PRIVATE_NOTE", "createdAt": "2026-10-10T00:00:00Z", "updatedAt": "2026-10-10T00:00:00Z"}}}
	todoCapture, ok := NewProtectedTodoCaptureV1(todo, nil, true)
	if !ok || todoCapture.Todo.Baseline.Availability != "known" || todoCapture.Todo.Baseline.Todos != nil {
		t.Fatal("absent committed baseline not retained")
	}
	for _, capture := range []ProtectedCaptureV1{goalCapture, todoCapture} {
		body, _ := json.Marshal(capture)
		if bytes.Contains(body, []byte("PRIVATE_")) {
			t.Fatal("unselected metadata entered capture")
		}
		var parsed ProtectedCaptureV1
		if json.Unmarshal(body, &parsed) != nil || ValidateProtectedCaptureV1(parsed) != nil {
			t.Fatal("selected capture did not roundtrip")
		}
		ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
		CaptureProtectedToolResultV1(ctx, capture)
		if capture.Goal != nil {
			*capture.Goal.RemainingTokens = 1
			capture.Goal.Goal.Objective = "replaced"
		} else {
			capture.Todo.Todos.Items[0].Content = "replaced"
		}
		owned, present := ConsumeProtectedToolResultV1(ctx)
		if !present || owned.Goal != nil && (*owned.Goal.RemainingTokens != 88 || owned.Goal.Goal.Objective == "replaced") || owned.Todo != nil && owned.Todo.Todos.Items[0].Content == "replaced" {
			t.Fatal("collector borrowed mutable domain records")
		}
	}
	if empty, ok := NewProtectedGoalCaptureV1(nil); !ok || empty.Goal.Goal != nil || empty.Goal.RemainingTokens != nil {
		t.Fatal("get_goal null observation invalid")
	}
	if _, ok := NewProtectedTodoCaptureV1(nil, nil, true); ok {
		t.Fatal("missing after became a successful observation")
	}
	for _, mutation := range []map[string]any{{"objective": "a\x00b"}, {"tokensUsed": -1}, {"createdAt": ""}, {"threadId": "../thread"}} {
		changed := map[string]any{}
		for key, value := range goal {
			changed[key] = value
		}
		for key, value := range mutation {
			changed[key] = value
		}
		if _, ok := NewProtectedGoalCaptureV1(changed); ok {
			t.Fatal("malformed goal observation accepted")
		}
	}
	for _, body := range []string{`{"availability":"known"}`, `{"availability":"unavailable","todos":null}`, `{"availability":"known","todos":null,"historyComplete":true}`} {
		var baseline ProtectedTodoBaselineV1
		if json.Unmarshal([]byte(body), &baseline) == nil {
			t.Fatal("open baseline accepted")
		}
	}
}

func TestProtectedToolSnapshotBodyPrefixAndStructuredStatus(t *testing.T) {
	for _, test := range []struct {
		body     string
		limit    int
		expected string
		cut      bool
	}{
		{"", 32768, "", false}, {"status=completed\nexitCode=0\n", 32768, "status=completed\nexitCode=0\n", false},
		{"A中B", 3, "A", true}, {"A中B", 4, "A中", true}, {"a\r\nb\n", 6, "a\r\nb\n", false},
		{strings.Repeat("x", 32769), 32768, strings.Repeat("x", 32768), true},
	} {
		got, cut := BoundProtectedTextV1(test.body, test.limit)
		if got != test.expected || cut != test.cut {
			t.Fatalf("prefix mismatch: bytes=%d cut=%v", len(got), cut)
		}
	}
	code := 7
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "shell", Status: "timeout", Body: "exitCode=0\nstatus=completed\n", ExitCode: &code})
	got, ok := ConsumeProtectedToolResultV1(ctx)
	if !ok || got.Status != "timeout" || *got.ExitCode != 7 {
		t.Fatal("body must not determine host status")
	}
	if _, ok := ConsumeProtectedToolResultV1(ctx); ok {
		t.Fatal("capture consumed twice")
	}
}

func TestProtectedToolSnapshotCaptureDuplicateAndUnsupportedFailClosed(t *testing.T) {
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "original\n"})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "replacement\n"})
	if _, ok := ConsumeProtectedToolResultV1(ctx); ok {
		t.Fatal("duplicate producer must not pick a body")
	}
	for _, kind := range []string{"unknown", "web", "background"} {
		if ValidateProtectedCaptureV1(ProtectedCaptureV1{Kind: kind, Status: "completed"}) == nil {
			t.Fatal("unsupported kind")
		}
	}
}

func TestProtectedToolSnapshotStrictEnvelopeAndPublicPrivateSplit(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	snapshot := ProtectedSnapshotV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, Principal: principal,
		Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID,
		ResultItemID: ToolResultItemIDV1("turn-a", callID), ToolName: "read", ContextDigest: strings.Repeat("c", 64),
		ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64),
		Capture: ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "package main\r\n", Label: "code.go", StartLine: 1, EndLine: 1, TotalLines: 1}}
	legacy, err := ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseProtectedSnapshotV1(legacy); err != nil || parsed.Capture.ReadTarget != nil {
		t.Fatal("legacy targetless bytes changed")
	}
	snapshot.Capture.ReadTarget = &ProtectedReadTargetV1{Path: "/synthetic/workspace/actual.go", StartLine: 1, ContentDigest: domainsecurity.SHA256Hex([]byte("package main\r\n"))}
	body, err := ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProtectedSnapshotV1(body)
	if err != nil || parsed.Capture.Body != "package main\r\n" {
		t.Fatal("strict ordinary envelope roundtrip failed")
	}
	for _, invalid := range [][]byte{
		append([]byte(" "), body...), bytes.Replace(body, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		bytes.Replace(body, []byte(`"purpose":`), []byte(`"unknown":true,"purpose":`), 1),
	} {
		if _, err := ParseProtectedSnapshotV1(invalid); err == nil {
			t.Fatal("noncanonical, duplicate or extra envelope accepted")
		}
	}
	binding, err := ProtectedSnapshotBindingForV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": snapshot.ResultItemID, "threadId": "thread-a", "turnId": "turn-a", "kind": "tool_result", "role": "tool",
		"callId": callID, "toolName": "read", "toolKind": "tool_call", "status": "completed", "isError": false,
		"createdAt": "2026-01-01T00:00:00Z", "finishedAt": "2026-01-01T00:00:00Z", "contextDigest": snapshot.ContextDigest,
		"contextEpoch": uint64(1), "executionGrantId": snapshot.ExecutionGrantID,
		"output":                        PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
		ProtectedSnapshotBindingFieldV1: ProtectedSnapshotBindingRecordV1(binding)}
	if _, ok := PrivateDurableToolResultItemRecordV1(item); !ok {
		t.Fatal("closed private binding was refused")
	}
	public := PublicToolResultItemRecordV1(item)
	publicBytes, _ := json.Marshal(public)
	if bytes.Contains(publicBytes, []byte("actual.go")) || bytes.Contains(publicBytes, []byte("readTarget")) {
		t.Fatal("target entered public projection")
	}
	if public[ProtectedSnapshotBindingFieldV1] != nil {
		t.Fatal("private snapshot binding entered public record")
	}
}

func TestProtectedScheduleSnapshotOperationAndWholeEnvelopeBudget(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	receipt := &ProtectedScheduleReceiptV1{Operation: "create", Status: "success", Message: "Actual", Task: protectedScheduleTaskFixtureV1()}
	snapshot := ProtectedSnapshotV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, Principal: principal,
		Workspace: "/synthetic/source-chat", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: ToolResultItemIDV1("turn-a", callID),
		ToolName: "mcp__gui_schedule__gui_schedule_create", ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64),
		Capture: ProtectedCaptureV1{Kind: "schedule", Status: "completed", Schedule: receipt}}
	body, err := ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProtectedSnapshotV1(body)
	if err != nil || parsed.Capture.Schedule.Task.WorkspaceRoot != "/synthetic/original" || parsed.Capture.Schedule.Task.LastThreadID != "" {
		t.Fatal("source identity substituted into created task", err)
	}
	for _, name := range []string{"schedule_create", "mcp__other__gui_schedule_create", "mcp__gui_schedule__gui_schedule_update", "mcp__gui_schedule__GUI_schedule_create"} {
		changed := snapshot
		changed.ToolName = name
		if ValidateProtectedSnapshotV1(changed) == nil {
			t.Fatal("different protocol/operation acquired native receipt")
		}
	}
	receipt.Task.Prompt = strings.Repeat("a", 200000)
	if ValidateProtectedScheduleReceiptV1(receipt) != nil {
		t.Fatal("receipt fixture alone exceeds budget")
	}
	snapshot.Inspect = &ProtectedInspectV1{InputJSON: `{"original":"` + strings.Repeat("b", 62000) + `"}`}
	if _, err := ProtectedSnapshotBytesV1(snapshot); err == nil {
		t.Fatal("receipt and Inspect bypassed whole canonical envelope limit")
	}
}

func TestProtectedReadTargetValidationAndCollectorOwnership(t *testing.T) {
	target := ProtectedReadTargetV1{Path: "/synthetic/actual.go", StartLine: 9, ContentDigest: strings.Repeat("f", 64)}
	capture := ProtectedCaptureV1{Kind: "read", Status: "completed", StartLine: 9, EndLine: 9, TotalLines: 9, ReadTarget: &target}
	for _, invalid := range []ProtectedReadTargetV1{
		{Path: "relative.go", StartLine: 9, ContentDigest: target.ContentDigest},
		{Path: target.Path, StartLine: 8, ContentDigest: target.ContentDigest},
		{Path: target.Path, StartLine: 9, ContentDigest: "forged"},
	} {
		value := capture
		value.ReadTarget = &invalid
		if ValidateProtectedCaptureV1(value) == nil {
			t.Fatal("invalid target accepted")
		}
	}
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, capture)
	target.Path = "/synthetic/forged.go"
	actual, ok := ConsumeProtectedToolResultV1(ctx)
	if !ok || actual.ReadTarget.Path != "/synthetic/actual.go" {
		t.Fatal("collector retained mutable producer target")
	}
}

func protectedWebFixtureV1(t *testing.T, body string) ProtectedSnapshotV1 {
	t.Helper()
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	status := 404
	truncated := false
	callID := "call_host_" + strings.Repeat("b", 64)
	return ProtectedSnapshotV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, Principal: principal, Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: ToolResultItemIDV1("turn-a", callID), ToolName: "web_fetch", ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64), Capture: ProtectedCaptureV1{Kind: "web_fetch", Status: "completed", Body: body, Label: "Fetch", DurationMs: 12, Web: &ProtectedWebCaptureV1{RequestURL: "https://example.test/request", FinalURL: "https://example.test/final", InputJSON: `{"url":"https://example.test/request"}`, SchemaJSON: `{"type":"object"}`, Title: "Actual title", StatusCode: &status, FetchTruncated: &truncated, ResponseComplete: true, ToolIsError: true, Error: &ProtectedWebErrorV1{Code: "fetch_failed", Message: "HTTP 404"}, StartedAt: "2026-10-08T10:11:12.123Z"}, WebTargets: &ProtectedWebTargetsV1{RequestURL: "https://example.test/request", ResultURL: "https://example.test/final"}}}
}

func TestProtectedWebCaptureFullBudgetStrictShapeAndLegacyLimitsV1(t *testing.T) {
	fixture := protectedWebFixtureV1(t, strings.Repeat("\x01", ProtectedWebSnapshotBodyLimitV1))
	encoded, err := ProtectedSnapshotBytesV1(fixture)
	if err != nil || len(encoded) <= ProtectedSnapshotEnvelopeLimitV1 || len(encoded) > ProtectedWebSnapshotEnvelopeLimitV1 {
		t.Fatal("full Web body did not fit its bounded JSON envelope")
	}
	parsed, err := ParseProtectedSnapshotV1(encoded)
	if err != nil || parsed.Capture.Body != fixture.Capture.Body || parsed.Capture.Web.Title != fixture.Capture.Web.Title {
		t.Fatal("full body lost during canonical parse")
	}
	for _, kind := range []string{"shell", "read"} {
		if ValidateProtectedCaptureV1(ProtectedCaptureV1{Kind: kind, Status: "completed", Body: strings.Repeat("a", ProtectedSnapshotBodyLimitV1+1)}) == nil {
			t.Fatal("Web budget widened old capture budget")
		}
	}
	for _, mutation := range []func(*ProtectedSnapshotV1){
		func(s *ProtectedSnapshotV1) { s.ToolName = "read" },
		func(s *ProtectedSnapshotV1) { s.Capture.Body = strings.Repeat("a", ProtectedWebSnapshotBodyLimitV1+1) },
		func(s *ProtectedSnapshotV1) { s.Capture.WebTargets.ResultURL = "https://forged.test/target" },
		func(s *ProtectedSnapshotV1) { s.Capture.Web.ResponseComplete = false },
		func(s *ProtectedSnapshotV1) { s.Capture.Web.InputJSON = `{"url":1,"url":2}` },
		func(s *ProtectedSnapshotV1) { s.Capture.Web.InputTruncated = true },
		func(s *ProtectedSnapshotV1) { s.Capture.Web.RequestURLTruncated = true },
	} {
		value := protectedWebFixtureV1(t, "body")
		mutation(&value)
		if ValidateProtectedSnapshotV1(value) == nil {
			t.Fatal("invalid web binding or closed capture accepted")
		}
	}
	fixture = protectedWebFixtureV1(t, "body")
	fixture.Capture.Web.InputJSON = ""
	fixture.Capture.Web.InputTruncated = true
	if _, err := ProtectedSnapshotBytesV1(fixture); err != nil {
		t.Fatal("explicit input budget refusal rejected the actual result")
	}
}

func TestProtectedWebCaptureCollectorOwnsDeepObservationsV1(t *testing.T) {
	fixture := protectedWebFixtureV1(t, "original")
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, fixture.Capture)
	fixture.Capture.Web.RequestURL = "https://forged.test/"
	*fixture.Capture.Web.StatusCode = 200
	fixture.Capture.Web.Error.Message = "forged"
	fixture.Capture.WebTargets.ResultURL = "https://forged.test/"
	capture, ok := ConsumeProtectedToolResultV1(ctx)
	if !ok || capture.Web.RequestURL != "https://example.test/request" || *capture.Web.StatusCode != 404 || capture.Web.Error.Message != "HTTP 404" || capture.WebTargets.ResultURL != "https://example.test/final" {
		t.Fatal("collector retained mutable web metadata or target")
	}
}

func TestProtectedWebBindingRemainsPrivateV1(t *testing.T) {
	fixture := protectedWebFixtureV1(t, "actual private body")
	binding, err := ProtectedSnapshotBindingForV1(fixture)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": fixture.ResultItemID, "threadId": fixture.ThreadID, "turnId": fixture.TurnID, "kind": "tool_result", "role": "tool", "callId": fixture.CallID, "toolName": "web_fetch", "toolKind": "tool_call", "status": "completed", "isError": false, "createdAt": "2026-10-08T10:11:13Z", "finishedAt": "2026-10-08T10:11:13Z", "contextDigest": fixture.ContextDigest, "contextEpoch": fixture.ContextEpoch, "executionGrantId": fixture.ExecutionGrantID, "output": PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")), ProtectedSnapshotBindingFieldV1: ProtectedSnapshotBindingRecordV1(binding)}
	if _, ok := PrivateDurableToolResultItemRecordV1(item); !ok {
		t.Fatal("exact web private binding refused")
	}
	for _, kind := range []string{"skill", "web", ""} {
		invalid := make(map[string]any, len(item))
		for key, value := range item {
			invalid[key] = value
		}
		invalid["toolKind"] = kind
		if _, ok := PrivateDurableToolResultItemRecordV1(invalid); ok {
			t.Fatal("non-tool-call web binding accepted")
		}
	}
	publicBytes, _ := json.Marshal(PublicToolResultItemRecordV1(item))
	for _, private := range []string{"actual private body", "example.test", "inputJSON", "schemaJSON", "webTargets", ProtectedSnapshotBindingFieldV1} {
		if bytes.Contains(publicBytes, []byte(private)) {
			t.Fatal("private Web content entered public item")
		}
	}
}

func TestProtectedLegacyMarshalParseBudgetsAndWebExclusionV1(t *testing.T) {
	web := protectedWebFixtureV1(t, "body")
	for _, name := range []string{"bash", "read", "read_file"} {
		old := web
		old.ToolName = name
		old.Capture = ProtectedCaptureV1{Kind: "read", Status: "completed", Body: strings.Repeat("\x01", ProtectedSnapshotBodyLimitV1)}
		if name == "bash" {
			old.Capture.Kind = "shell"
		}
		encoded, err := ProtectedSnapshotBytesV1(old)
		if err != nil || len(encoded) > ProtectedSnapshotEnvelopeLimitV1 {
			t.Fatal("old legal capture changed envelope budget")
		}
		if _, err := ParseProtectedSnapshotV1(encoded); err != nil {
			t.Fatal("old legal canonical capture failed parse")
		}
		oversized := append(append([]byte(nil), encoded...), bytes.Repeat([]byte(" "), ProtectedSnapshotEnvelopeLimitV1)...)
		if _, err := ParseProtectedSnapshotV1(oversized); err == nil {
			t.Fatal("old kind accepted an over-budget envelope")
		}
		withWeb := old
		withWeb.Capture.Web = web.Capture.Web
		if _, err := ProtectedSnapshotBytesV1(withWeb); err == nil {
			t.Fatal("old kind accepted Web observation")
		}
		withTargets := old
		withTargets.Capture.WebTargets = web.Capture.WebTargets
		if _, err := ProtectedSnapshotBytesV1(withTargets); err == nil {
			t.Fatal("old kind accepted Web target")
		}
	}
}

func TestProtectedWebTargetParserKeepsRealPathSpaceAndRejectsControlsV1(t *testing.T) {
	if !ValidProtectedWebTargetV1("https://example.test/actual path?q=actual value") {
		t.Fatal("valid HTTP path spaces lost actual target")
	}
	for _, target := range []string{"https://user@example.test/", "https://example.test/\t", "https://example.test/\u0085", "javascript:alert(1)", "https://exa mple.test/"} {
		if ValidProtectedWebTargetV1(target) {
			t.Fatal("invalid Host target accepted")
		}
	}
}

func TestProtectedTodoParallelBeforeAndAfterCaptureRoundtrip(t *testing.T) {
	list := func(content string) map[string]any {
		return map[string]any{"threadId": "thread-parallel", "updatedAt": "now", "items": []any{
			map[string]any{"id": "first", "content": content, "status": "in_progress", "createdAt": "then", "updatedAt": "now"},
			map[string]any{"id": "second", "content": "Second", "status": "in_progress", "createdAt": "then", "updatedAt": "now"},
		}}
	}
	before, after := list("Before"), list("After")
	capture, ok := NewProtectedTodoCaptureV1(after, before, true)
	if !ok || ValidateProtectedCaptureV1(capture) != nil {
		t.Fatal("parallel committed capture refused")
	}
	body, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	var restored ProtectedCaptureV1
	if err := json.Unmarshal(body, &restored); err != nil || ValidateProtectedCaptureV1(restored) != nil {
		t.Fatalf("parallel replay refused: %v", err)
	}
	if restored.Todo.Baseline.Todos.Items[0].Content != "Before" || restored.Todo.Todos.Items[0].Content != "After" {
		t.Fatal("before/after identity lost")
	}
	for _, list := range []ProtectedTodoListV1{*restored.Todo.Baseline.Todos, restored.Todo.Todos} {
		if len(list.Items) != 2 || list.Items[0].Status != "in_progress" || list.Items[1].Status != "in_progress" {
			t.Fatal("capture downgraded parallel state")
		}
	}
	restored.Todo.Todos.Items[1].ID = restored.Todo.Todos.Items[0].ID
	if ValidateProtectedCaptureV1(restored) == nil {
		t.Fatal("parallel policy weakened unique identity validation")
	}
}

func TestProtectedSuccessTextSharesSingleConsumeAndRejectsLateReplayV1(t *testing.T) {
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	source := ProjectProtectedOutputTextV1("actual first\nactual second")
	CaptureProtectedToolTextV1(ctx, source)
	source.Body = "caller mutation"
	_, present, error_, text := ConsumeProtectedToolObservationWithTextV1(ctx)
	if present || error_ != nil || text == nil || text.Body != "actual first\nactual second" {
		t.Fatal("text acquired a fake typed result or borrowed caller storage")
	}
	CaptureProtectedToolTextV1(ctx, ProjectProtectedOutputTextV1("late"))
	if _, _, _, late := ConsumeProtectedToolObservationWithTextV1(ctx); late != nil {
		t.Fatal("retired collector replayed text")
	}
	legacy := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolTextV1(legacy, ProjectProtectedOutputTextV1("private"))
	ConsumeProtectedToolObservationV1(legacy)
	if _, _, _, late := ConsumeProtectedToolObservationWithTextV1(legacy); late != nil {
		t.Fatal("legacy consume left private text available")
	}
	duplicate := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolTextV1(duplicate, ProjectProtectedOutputTextV1("first"))
	CaptureProtectedToolTextV1(duplicate, ProjectProtectedOutputTextV1("second"))
	if _, _, _, text := ConsumeProtectedToolObservationWithTextV1(duplicate); text == nil || text.Availability != "unavailable" || text.Body != "" {
		t.Fatal("duplicate producer selected a replacement body")
	}
}

func TestGenericOutputTextDigestAndStrictSuccessBindingV1(t *testing.T) {
	original := protectedWebFixtureV1(t, "unused typed body")
	original.ToolName = "mcp__docs__lookup"
	original.Capture = ProtectedCaptureV1{Kind: "inspect", Status: "completed"}
	original.Inspect = &ProtectedInspectV1{InputJSON: `{}`, OutputText: ProjectProtectedOutputTextV1("first\n{\"n\":9007199254740993}")}
	binding, err := ProtectedSnapshotBindingForV1(original)
	if err != nil {
		t.Fatal(err)
	}
	bytes_, err := ProtectedSnapshotBytesV1(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseProtectedSnapshotV1(bytes_)
	if err != nil || decoded.Inspect.OutputText.Body != original.Inspect.OutputText.Body {
		t.Fatal("canonical private text failed round trip")
	}
	changed := original
	changed.Inspect = &ProtectedInspectV1{InputJSON: `{}`, OutputText: ProjectProtectedOutputTextV1("different actual text")}
	changedBinding, err := ProtectedSnapshotBindingForV1(changed)
	if err != nil || changedBinding.SnapshotDigest == binding.SnapshotDigest || changedBinding.BodyDigest != binding.BodyDigest {
		t.Fatal("Inspect text was not part of full snapshot identity")
	}
	for _, status := range []string{"failed", "canceled", "unknown"} {
		changed := original
		changed.Capture.Status = status
		if ValidateProtectedSnapshotV1(changed) == nil {
			t.Fatal("non-success acquired generic success text")
		}
	}
	changed = original
	changed.ToolName = "read"
	if ValidateProtectedSnapshotV1(changed) == nil {
		t.Fatal("ordinary typed producer was reclassified by text")
	}
}

func TestInvalidCaptureCannotReappearAsGenericSuccessTextV1(t *testing.T) {
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "unsupported", Status: "completed"})
	CaptureProtectedToolTextV1(ctx, ProjectProtectedOutputTextV1("ambiguous source"))
	if _, _, _, text := ConsumeProtectedToolObservationWithTextV1(ctx); text != nil {
		t.Fatal("invalid observation reappeared through generic fallback")
	}
}
