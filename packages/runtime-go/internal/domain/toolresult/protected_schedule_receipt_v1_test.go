package toolresult

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func protectedScheduleTaskFixtureV1() *ProtectedScheduleTaskV1 {
	provider := "unknown-provider"
	return &ProtectedScheduleTaskV1{ID: "task-1", Title: "  Committed title  ", Enabled: true, Prompt: "PRIVATE_ORIGINAL_PROMPT", WorkspaceRoot: "/synthetic/original", ClawChannelID: "", ProviderID: &provider,
		Model: "actual-model", ReasoningEffort: "auto", Mode: "agent", Schedule: ProtectedScheduleRuleV1{Kind: "daily", EveryMinutes: 60, TimeOfDay: "09:00", AtTime: ""},
		CreatedAt: "actual-created", UpdatedAt: "actual-updated", LastRunAt: "", NextRunAt: "", LastStatus: "idle", LastMessage: "schedule_task_idle", LastThreadID: ""}
}

func TestProtectedScheduleReceiptStrictNativeOutputAndNulls(t *testing.T) {
	task := protectedScheduleTaskFixtureV1()
	for _, operation := range []string{"create", "update"} {
		body, _ := json.Marshal(map[string]any{"status": "success", "message": "Actual outcome", "task": task})
		receipt, err := ParseProtectedScheduleToolOutputV1(operation, body, false)
		if err != nil || !reflect.DeepEqual(receipt.Task, task) || receipt.Operation != operation {
			t.Fatal("actual task receipt changed", err)
		}
		encoded, _ := json.Marshal(receipt)
		var decoded ProtectedScheduleReceiptV1
		if json.Unmarshal(encoded, &decoded) != nil || !reflect.DeepEqual(receipt, &decoded) {
			t.Fatal("receipt did not roundtrip")
		}
		failure := []byte(`{"status":"failure","message":"Actual failure","task":null}`)
		if value, err := ParseProtectedScheduleToolOutputV1(operation, failure, true); err != nil || value.Task != nil {
			t.Fatal("failure null changed", err)
		}
		for _, invalid := range []struct {
			body    []byte
			isError bool
		}{{body, true}, {failure, false}, {[]byte(`{"status":"success","message":"Actual","task":null}`), false}} {
			if _, err := ParseProtectedScheduleToolOutputV1(operation, invalid.body, invalid.isError); err == nil {
				t.Fatal("contradictory native outcome accepted")
			}
		}
	}
	deleted, err := ParseProtectedScheduleToolOutputV1("delete", []byte(`{"status":"failure","message":"Actual delete failure","taskId":" exact returned ID "}`), true)
	if err != nil || deleted.TaskID == nil || *deleted.TaskID != " exact returned ID " {
		t.Fatal("delete receipt identity normalized", err)
	}
	for _, body := range []string{
		`{"status":"failure","message":"Actual"}`, `{"status":"failure","message":null,"task":null}`,
		`{"status":"failure","message":"Actual","task":null,"taskId":"injected"}`,
		`{"status":"failure","status":"success","message":"Actual","task":null}`,
		`{"status":"failure","message":"\ud800","task":null}`,
	} {
		if _, err := ParseProtectedScheduleToolOutputV1("create", []byte(body), true); err == nil {
			t.Fatal("open native receipt accepted")
		}
	}
	taskBytes, _ := json.Marshal(task)
	for _, mutate := range []func(map[string]any){
		func(value map[string]any) { delete(value, "enabled") }, func(value map[string]any) { delete(value, "lastThreadId") },
		func(value map[string]any) { value["providerId"] = nil }, func(value map[string]any) { value["timezone"] = "UTC" },
		func(value map[string]any) { value["prompt"] = nil }, func(value map[string]any) { value["schedule"].(map[string]any)["kind"] = "cron" },
	} {
		value, _ := scheduleObjectV1(taskBytes)
		mutate(value)
		body, _ := json.Marshal(map[string]any{"status": "success", "message": "Actual", "task": value})
		if _, err := ParseProtectedScheduleToolOutputV1("create", body, false); err == nil {
			t.Fatal("incomplete or unknown task fields accepted")
		}
	}
}

func TestProtectedScheduleCaptureOwnsDeepCopyAndClosedKind(t *testing.T) {
	receipt := &ProtectedScheduleReceiptV1{Operation: "create", Status: "success", Message: "Actual", Task: protectedScheduleTaskFixtureV1()}
	capture := ProtectedCaptureV1{Kind: "schedule", Status: "completed", Schedule: receipt}
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, capture)
	receipt.Task.Title = "Later title"
	*receipt.Task.ProviderID = "Later provider"
	receipt.Task.Schedule.TimeOfDay = "18:00"
	owned, present := ConsumeProtectedToolResultV1(ctx)
	if !present || owned.Schedule.Task.Title != "  Committed title  " || *owned.Schedule.Task.ProviderID != "unknown-provider" || owned.Schedule.Task.Schedule.TimeOfDay != "09:00" {
		t.Fatal("collector borrowed mutable task pointers")
	}
	if _, present := ConsumeProtectedToolResultV1(ctx); present {
		t.Fatal("capture consumed twice")
	}
	for _, changed := range []ProtectedCaptureV1{
		{Kind: "inspect", Status: "completed", Schedule: receipt}, {Kind: "schedule", Status: "failed", Schedule: receipt},
		{Kind: "schedule", Status: "completed", Schedule: receipt, Body: "injected"}, {Kind: "schedule", Status: "completed", Schedule: receipt, Truncated: true},
		{Kind: "schedule", Status: "completed", Schedule: receipt, Job: &ProtectedJobCaptureV1{}},
	} {
		if ValidateProtectedCaptureV1(changed) == nil {
			t.Fatal("open or contradictory schedule capture accepted")
		}
	}
	ctx = WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, capture)
	CaptureProtectedToolResultV1(ctx, capture)
	if _, present := ConsumeProtectedToolResultV1(ctx); present {
		t.Fatal("duplicate capture retained authority")
	}
	receipt.Task.Prompt = strings.Repeat("中", 87382)
	if ValidateProtectedScheduleReceiptV1(receipt) == nil {
		t.Fatal("oversize receipt truncated or accepted")
	}
}

func TestProtectedScheduleReceiptCalendarVariantsV1(t *testing.T) {
	zone, expression := "America/New_York", "0 9 */2 * 1"
	for _, rule := range []ProtectedScheduleRuleV1{
		{Kind: "manual", EveryMinutes: 60, TimeOfDay: "unchanged legacy", AtTime: ""},
		{Kind: "at", EveryMinutes: 60, TimeOfDay: "", AtTime: "unchanged legacy"},
		{Kind: "interval", EveryMinutes: -1, TimeOfDay: "", AtTime: ""},
		{Kind: "daily", EveryMinutes: 60, TimeOfDay: "unchanged legacy", AtTime: ""},
		{Kind: "daily", EveryMinutes: 60, TimeOfDay: "09:00:01.123", AtTime: "", TimeZone: &zone},
		{Kind: "weekly", EveryMinutes: 60, TimeOfDay: "09:00", AtTime: "", TimeZone: &zone, Weekdays: []int{7, 1, 5}},
		{Kind: "cron", EveryMinutes: 60, TimeOfDay: "unchanged unused", AtTime: "", TimeZone: &zone, Expression: &expression},
	} {
		t.Run(rule.Kind+"/"+rule.TimeOfDay, func(t *testing.T) {
			task := protectedScheduleTaskFixtureV1()
			task.Schedule = rule
			body, _ := json.Marshal(map[string]any{"status": "success", "message": "Actual", "task": task})
			for _, operation := range []string{"create", "update"} {
				receipt, err := ParseProtectedScheduleToolOutputV1(operation, body, false)
				if err != nil || !reflect.DeepEqual(receipt.Task, task) {
					t.Fatal("committed calendar rule changed", err)
				}
				encoded, _ := json.Marshal(receipt)
				var decoded ProtectedScheduleReceiptV1
				if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(receipt, &decoded) {
					t.Fatal("calendar receipt did not roundtrip", err)
				}
				if rule.TimeZone == nil && (strings.Contains(string(encoded), `"timeZone"`) || strings.Contains(string(encoded), `"weekdays"`) || strings.Contains(string(encoded), `"expression"`)) {
					t.Fatal("legacy four-field rule gained calendar fields")
				}
			}
		})
	}
}

func TestProtectedScheduleReceiptCalendarRejectsOpenFieldsV1(t *testing.T) {
	base := protectedScheduleTaskFixtureV1()
	baseBytes, _ := json.Marshal(base)
	rules := []string{
		`{"kind":"daily","timeZone":null}`, `{"kind":"daily","timeZone":1}`,
		`{"kind":"daily","timeZone":""}`, `{"kind":"daily","timeZone":" UTC "}`,
		`{"kind":"daily","timeZone":"EST"}`, `{"kind":"daily","timeZone":"+08:00"}`,
		`{"kind":"daily","timeZone":"UTC","timeOfDay":"24:00"}`,
		`{"kind":"daily","weekdays":[1]}`, `{"kind":"daily","expression":"0 9 * * *"}`,
		`{"kind":"manual","timeZone":"UTC"}`, `{"kind":"at","weekdays":[1]}`,
		`{"kind":"interval","expression":"0 9 * * *"}`,
		`{"kind":"weekly","weekdays":[1]}`, `{"kind":"weekly","timeZone":"UTC"}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":null}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":[]}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":[0]}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":[8]}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":[1,1]}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":["1"]}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":[1.0000000000000000001]}`,
		`{"kind":"weekly","timeZone":"UTC","weekdays":[1],"expression":"0 9 * * *"}`,
		`{"kind":"cron","expression":"0 9 * * *"}`, `{"kind":"cron","timeZone":"UTC"}`,
		`{"kind":"cron","timeZone":"UTC","expression":null}`,
		`{"kind":"cron","timeZone":"UTC","expression":1}`,
		`{"kind":"cron","timeZone":"UTC","expression":""}`,
		`{"kind":"cron","timeZone":"UTC","expression":" 0 9 * * *"}`,
		`{"kind":"cron","timeZone":"UTC","expression":"@daily"}`,
		`{"kind":"cron","timeZone":"UTC","expression":"0 9 * * MON"}`,
		`{"kind":"cron","timeZone":"UTC","expression":"0 9 * * * *"}`,
		`{"kind":"cron","timeZone":"UTC","expression":"0,,1 9 * * *"}`,
		`{"kind":"cron","timeZone":"UTC","expression":"0 9 * * *","weekdays":[1]}`,
		`{"kind":"daily","timezone":"UTC"}`,
	}
	for _, patch := range rules {
		t.Run(patch, func(t *testing.T) {
			task, _ := scheduleObjectV1(baseBytes)
			rule := task["schedule"].(map[string]any)
			changes, err := scheduleObjectV1([]byte(patch))
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range changes {
				rule[key] = value
			}
			body, _ := json.Marshal(map[string]any{"status": "success", "message": "Actual", "task": task})
			if _, err := ParseProtectedScheduleToolOutputV1("create", body, false); err == nil {
				t.Fatal("open or inconsistent calendar rule accepted")
			}
		})
	}
	longZone, utc := "Area/"+strings.Repeat("A", 251), "UTC"
	longExpression := strings.Repeat("0", 505) + " 0 * * *"
	for _, rule := range []ProtectedScheduleRuleV1{
		{Kind: "daily", TimeOfDay: "09:00", TimeZone: &longZone},
		{Kind: "cron", TimeZone: &utc, Expression: &longExpression},
	} {
		base.Schedule = rule
		if ValidateProtectedScheduleReceiptV1(&ProtectedScheduleReceiptV1{Operation: "create", Status: "success", Task: base}) == nil {
			t.Fatal("oversize calendar text accepted")
		}
	}
}

func TestProtectedScheduleCaptureCalendarDeepCopyV1(t *testing.T) {
	zone := "UTC"
	task := protectedScheduleTaskFixtureV1()
	task.Schedule = ProtectedScheduleRuleV1{Kind: "weekly", EveryMinutes: 60, TimeOfDay: "09:00", TimeZone: &zone, Weekdays: []int{1, 7}}
	receipt := &ProtectedScheduleReceiptV1{Operation: "update", Status: "success", Message: "Actual", Task: task}
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "schedule", Status: "completed", Schedule: receipt})
	*task.Schedule.TimeZone = "America/New_York"
	task.Schedule.Weekdays[0] = 5
	owned, present := ConsumeProtectedToolResultV1(ctx)
	if !present || *owned.Schedule.Task.Schedule.TimeZone != "UTC" || !reflect.DeepEqual(owned.Schedule.Task.Schedule.Weekdays, []int{1, 7}) {
		t.Fatal("collector borrowed calendar fields")
	}
	expression := "0 9 * * *"
	task.Schedule = ProtectedScheduleRuleV1{Kind: "cron", EveryMinutes: 60, TimeOfDay: "09:00", TimeZone: &zone, Expression: &expression}
	ctx = WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "schedule", Status: "completed", Schedule: receipt})
	*task.Schedule.Expression = "0 18 * * *"
	owned, present = ConsumeProtectedToolResultV1(ctx)
	if !present || *owned.Schedule.Task.Schedule.Expression != "0 9 * * *" {
		t.Fatal("collector borrowed cron expression")
	}
}
