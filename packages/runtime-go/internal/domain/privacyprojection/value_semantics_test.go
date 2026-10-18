package privacyprojection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNumericMeasureProjectionDoesNotDependOnMapRepresentation(t *testing.T) {
	for _, amount := range []string{"9007199254740993.01", "-9007199254740993.01", "6222020000000000000", "0.00"} {
		t.Run(amount, func(t *testing.T) {
			stringMap := map[string]string{"amount": amount, "balanceMinor": amount, "金额": amount, "direction": "out"}
			anyMap := map[string]any{}
			for key, value := range stringMap {
				anyMap[key] = value
			}
			for name, input := range map[string]any{
				"string-map": stringMap, "any-map": anyMap,
				"string-map-list": []map[string]string{stringMap}, "any-map-list": []map[string]any{anyMap},
			} {
				t.Run(name, func(t *testing.T) {
					public := map[string]any{"output": input}
					if err := ValidatePublicValue(public); err != nil {
						t.Fatal("valid typed measure depended on Go map representation")
					}
					for mode, project := range map[string]func(any) (any, bool){
						"public": ProjectPublicValue, "untrusted": ProjectUntrustedValue,
					} {
						got, changed := project(public)
						if changed || !reflect.DeepEqual(got, public) {
							t.Fatalf("%s projection changed exact measures: %#v", mode, got)
						}
					}
				})
			}
		})
	}
}

func TestStringMapMeasureExceptionDoesNotExemptPIIOrMutateSource(t *testing.T) {
	input := map[string]string{
		"amount": "9007199254740993.01", "amountMinor": "900719925474099301",
		"account": "6222020000000000000", "phone": "13800138000",
		"note": "电话 13800138000", "opaque": "6222020000000000000",
		"balance": "电话 13800138000",
	}
	original := map[string]string{}
	for key, value := range input {
		original[key] = value
	}
	projected, changed := ProjectUntrustedValue(input)
	got := projected.(map[string]string)
	if !changed || got["amount"] != input["amount"] || got["amountMinor"] != input["amountMinor"] {
		t.Fatal("projection lost exact decimal measures")
	}
	if got["account"] != "[ACCOUNT]" || got["phone"] != "[PHONE]" || got["opaque"] != "[ACCOUNT]" ||
		strings.Contains(got["note"], input["phone"]) || strings.Contains(got["balance"], input["phone"]) {
		t.Fatal("numeric exception expanded into PII or nonnumeric content")
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("projection modified the private source map")
	}
	again, changedAgain := ProjectUntrustedValue(projected)
	if changedAgain || !reflect.DeepEqual(again, projected) || ValidatePublicValue(map[string]any{"output": projected}) != nil {
		t.Fatal("projection is not a validated fixed point")
	}
}

func TestUntrustedQuestionShapeCannotGrantHostCorrelationException(t *testing.T) {
	const raw = "6222020000000000000"
	for _, kind := range []string{"user_input", "user_input_requested"} {
		for _, mode := range []string{"untrusted-root", "public-content"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				record := map[string]any{
					"kind": kind, "inputId": "账号 " + raw,
					"questions": []any{map[string]any{"id": "账号 " + raw + "_1", "question": "Continue?"}},
				}
				var input any = record
				project := ProjectUntrustedValue
				if mode == "public-content" {
					input = map[string]any{"output": record}
					project = ProjectPublicValue
					if ValidatePublicValue(input) == nil {
						t.Fatal("untrusted content gained host authority during validation")
					}
				}
				before, _ := json.Marshal(input)
				got, changed := project(input)
				encoded, err := json.Marshal(got)
				if err != nil || !changed || strings.Contains(string(encoded), raw) {
					t.Fatal("untrusted host-shaped question retained PII after projection")
				}
				after, _ := json.Marshal(input)
				if string(before) != string(after) {
					t.Fatal("untrusted projection mutated its source")
				}
				again, changedAgain := project(got)
				if changedAgain || !reflect.DeepEqual(got, again) || ValidatePublicValue(got) != nil {
					t.Fatal("untrusted projection requires another pass to close PII")
				}
			})
		}
	}
}

const goalHistoryStampV1 = "2026-10-18T09:11:13.123456789Z"
const goalHistoryIDV1 = "abcdefab-cdef-4abc-8def-131234567890"

func goalHistoryFixtureV1(counted bool) map[string]any {
	snapshot := map[string]any{
		"id": goalHistoryIDV1, "threadId": "thread-goal", "objective": "Verify the deliverable", "status": "active",
		"tokenBudget": nil, "tokensUsed": float64(0), "timeUsedSeconds": float64(0),
		"createdAt": goalHistoryStampV1, "updatedAt": goalHistoryStampV1,
	}
	entry := map[string]any{"id": goalHistoryIDV1, "operation": "legacy_update", "goal": snapshot, "createdAt": goalHistoryStampV1}
	if counted {
		snapshot["revision"], snapshot["maxGoalRounds"], snapshot["roundsStarted"] = 1, 3, 0
		entry["operation"] = "create"
		entry["ref"] = map[string]any{"id": goalHistoryIDV1, "revision": 1}
	} else {
		snapshot["id"] = "goal_thread-goal"
	}
	return entry
}

func goalHistoryThreadFixtureV1(entry map[string]any) map[string]any {
	return map[string]any{
		"id": "thread-goal", "status": "idle", "createdAt": goalHistoryStampV1, "updatedAt": goalHistoryStampV1,
		"goalHistory": []any{entry}, "turns": []any{},
	}
}

func goalHistoryMetadataFixtureV1(entry map[string]any) map[string]any {
	return map[string]any{
		"kind": "thread_metadata", "version": float64(1), "timestamp": goalHistoryStampV1,
		"thread":  goalHistoryThreadFixtureV1(entry),
		"summary": map[string]any{"schemaVersion": float64(1), "preview": "", "messageCount": float64(0), "turnCount": float64(0)},
	}
}

func TestGoalHistorySnapshotPreservesMetadata(t *testing.T) {
	if len(ProjectText(goalHistoryStampV1).Findings) == 0 || len(ProjectText(goalHistoryIDV1).Findings) == 0 {
		t.Fatal("fixture must exercise metadata that the prose detector would project")
	}
	for _, counted := range []bool{false, true} {
		for _, representation := range []string{"host", "json"} {
			t.Run(representation+"/counted="+map[bool]string{false: "false", true: "true"}[counted], func(t *testing.T) {
				var input any = goalHistoryThreadFixtureV1(goalHistoryFixtureV1(counted))
				if representation == "json" {
					encoded, _ := json.Marshal(input)
					decoder := json.NewDecoder(strings.NewReader(string(encoded)))
					decoder.UseNumber()
					if err := decoder.Decode(&input); err != nil {
						t.Fatal(err)
					}
				}
				got, changed := ProjectPublicValue(input)
				if changed || !reflect.DeepEqual(got, input) || ValidatePublicValue(input) != nil {
					t.Fatal("valid history metadata was treated as prose", got)
				}
			})
		}
	}
}

func TestGoalHistorySnapshotProjectsContent(t *testing.T) {
	for _, field := range []string{"objective", "blockedReason"} {
		t.Run(field, func(t *testing.T) {
			entry := goalHistoryFixtureV1(true)
			entry["goal"].(map[string]any)[field] = "Call 13800138000"
			input := goalHistoryThreadFixtureV1(entry)
			before, _ := json.Marshal(input)
			if ValidatePublicValue(input) == nil {
				t.Fatal("history content bypassed publication validation")
			}
			got, changed := ProjectPublicValue(input)
			snapshot := got.(map[string]any)["goalHistory"].([]any)[0].(map[string]any)["goal"].(map[string]any)
			if !changed || strings.Contains(snapshot[field].(string), "13800138000") ||
				snapshot["id"] != goalHistoryIDV1 || snapshot["createdAt"] != goalHistoryStampV1 || snapshot["updatedAt"] != goalHistoryStampV1 {
				t.Fatal("history projection did not separate content from metadata", snapshot)
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("history projection mutated the source")
			}
			again, changedAgain := ProjectPublicValue(got)
			if changedAgain || !reflect.DeepEqual(got, again) || ValidatePublicValue(got) != nil {
				t.Fatal("projected history is not a validated fixed point")
			}
		})
	}
}

func TestGoalHistorySnapshotLookalikesRemainProse(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any, map[string]any)
	}{
		{"unknown-snapshot-field", func(_ map[string]any, s map[string]any) { s["opaque"] = "13800138000" }},
		{"unknown-entry-field", func(e, _ map[string]any) { e["opaque"] = "extension" }},
		{"missing-usage", func(_ map[string]any, s map[string]any) { delete(s, "tokensUsed") }},
		{"wrong-usage", func(_ map[string]any, s map[string]any) { s["tokensUsed"] = "13800138000" }},
		{"wrong-objective", func(_ map[string]any, s map[string]any) { s["objective"] = map[string]any{"opaque": "13800138000"} }},
		{"wrong-status", func(_ map[string]any, s map[string]any) { s["status"] = "13800138000" }},
		{"wrong-timestamp", func(_ map[string]any, s map[string]any) { s["createdAt"] = "13800138000" }},
		{"wrong-optional-budget", func(_ map[string]any, s map[string]any) { s["tokenBudget"] = false }},
		{"incomplete-counters", func(_ map[string]any, s map[string]any) { delete(s, "roundsStarted") }},
		{"over-budget-counters", func(_ map[string]any, s map[string]any) { s["roundsStarted"] = 4 }},
		{"mismatched-ref", func(e, _ map[string]any) { e["ref"].(map[string]any)["revision"] = 2 }},
		{"wrong-operation", func(e, _ map[string]any) { e["operation"] = "invented" }},
		{"wrong-thread-binding", func(_ map[string]any, s map[string]any) { s["threadId"] = "other-thread" }},
		{"pii-thread-id", func(_ map[string]any, s map[string]any) { s["threadId"] = "13800138000" }},
		{"pii-entry-id", func(e, _ map[string]any) { e["id"] = "13800138000" }},
		{"pii-snapshot-id", func(_ map[string]any, s map[string]any) { s["id"] = "13800138000" }},
		{"email-snapshot-id", func(_ map[string]any, s map[string]any) { s["id"] = "person@example.com" }},
		{"wrong-uuid-version", func(e, _ map[string]any) { e["id"] = "abcdefab-cdef-1abc-8def-131234567890" }},
		{"wrong-reason-shape", func(_ map[string]any, s map[string]any) { s["blockedReason"] = map[string]any{"opaque": "13800138000"} }},
		{"fractional-usage", func(_ map[string]any, s map[string]any) { s["timeUsedSeconds"] = 0.5 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := goalHistoryFixtureV1(true)
			test.mutate(entry, entry["goal"].(map[string]any))
			input := goalHistoryThreadFixtureV1(entry)
			if ValidatePublicValue(input) == nil {
				t.Fatal("lookalike gained history metadata routing")
			}
			got, changed := ProjectPublicValue(input)
			snapshot := got.(map[string]any)["goalHistory"].([]any)[0].(map[string]any)["goal"].(map[string]any)
			encoded, _ := json.Marshal(got)
			if !changed || snapshot["updatedAt"] == goalHistoryStampV1 || strings.Contains(string(encoded), "13800138000") || strings.Contains(string(encoded), "person@example.com") {
				t.Fatal("lookalike metadata escaped conservative projection", snapshot)
			}
		})
	}
	for _, test := range []struct {
		name string
		wrap func(map[string]any) any
	}{
		{"entry-root", func(e map[string]any) any { return e }},
		{"arbitrary-map", func(e map[string]any) any { return map[string]any{"opaque": e} }},
		{"arbitrary-array", func(e map[string]any) any { return []any{e} }},
		{"nested-thread", func(e map[string]any) any { return map[string]any{"opaque": goalHistoryThreadFixtureV1(e)} }},
		{"content", func(e map[string]any) any { return map[string]any{"output": goalHistoryThreadFixtureV1(e)} }},
		{"bare-goal", func(e map[string]any) any { return map[string]any{"goal": e["goal"]} }},
		{"nested-history-array", func(e map[string]any) any {
			r := goalHistoryThreadFixtureV1(e)
			r["goalHistory"] = []any{[]any{e}}
			return r
		}},
		{"raw-history-entry", func(e map[string]any) any {
			r := goalHistoryThreadFixtureV1(e)
			b, _ := json.Marshal(e)
			r["goalHistory"] = []any{json.RawMessage(b)}
			return r
		}},
		{"history-key-alias", func(e map[string]any) any {
			r := goalHistoryThreadFixtureV1(e)
			r["GoalHistory"] = r["goalHistory"]
			delete(r, "goalHistory")
			return r
		}},
		{"unknown-metadata-field", func(e map[string]any) any { r := goalHistoryMetadataFixtureV1(e); r["opaque"] = true; return r }},
		{"nested-metadata", func(e map[string]any) any { return map[string]any{"opaque": goalHistoryMetadataFixtureV1(e)} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := goalHistoryFixtureV1(true)
			input := test.wrap(entry)
			before, _ := json.Marshal(input)
			if ValidatePublicValue(input) == nil {
				t.Fatal("history-shaped value at an unrelated position gained metadata routing")
			}
			got, changed := ProjectPublicValue(input)
			encoded, _ := json.Marshal(got)
			if !changed || strings.Count(string(encoded), goalHistoryStampV1) >= strings.Count(string(before), goalHistoryStampV1) {
				t.Fatal("history-shaped prose gained trusted metadata routing", got)
			}
		})
	}
}

func TestGoalHistoryMetadataEnvelopePreservesMetadataAndScansContent(t *testing.T) {
	for _, content := range []string{"Verified", "Call 13800138000"} {
		entry := goalHistoryFixtureV1(true)
		entry["goal"].(map[string]any)["blockedReason"] = content
		input := goalHistoryMetadataFixtureV1(entry)
		containsPII := strings.Contains(content, "13800138000")
		if (ValidatePublicValue(input) != nil) != containsPII {
			t.Fatal("metadata envelope validation disagreed with content")
		}
		got, changed := ProjectPublicValue(input)
		if changed != containsPII || ValidatePublicValue(got) != nil {
			t.Fatal("metadata envelope did not close content projection", got)
		}
		snapshot := got.(map[string]any)["thread"].(map[string]any)["goalHistory"].([]any)[0].(map[string]any)["goal"].(map[string]any)
		if snapshot["id"] != goalHistoryIDV1 || snapshot["updatedAt"] != goalHistoryStampV1 || strings.Contains(snapshot["blockedReason"].(string), "13800138000") {
			t.Fatal("metadata envelope changed metadata or retained PII", snapshot)
		}
		if !containsPII && !reflect.DeepEqual(got, input) {
			t.Fatal("safe metadata envelope changed")
		}
		untrusted, untrustedChanged := ProjectUntrustedValue(input)
		encoded, _ := json.Marshal(untrusted)
		if !untrustedChanged || strings.Contains(string(encoded), goalHistoryStampV1) {
			t.Fatal("untrusted metadata envelope granted history routing")
		}
	}
}

func TestGoalHistorySnapshotLeavesContentLengthsToExistingOwner(t *testing.T) {
	for _, reason := range []string{"", strings.Repeat("verified ", 300)} {
		entry := goalHistoryFixtureV1(false)
		snapshot := entry["goal"].(map[string]any)
		snapshot["blockedReason"] = reason
		snapshot["tokenBudget"], snapshot["tokensUsed"] = int64(1<<53), int64(1<<53)
		input := goalHistoryThreadFixtureV1(entry)
		got, changed := ProjectPublicValue(input)
		if changed || !reflect.DeepEqual(got, input) || ValidatePublicValue(input) != nil {
			t.Fatal("privacy classification imposed a new content length or safe-integer measure bound")
		}
	}
}

func TestGoalHistoryClearMetadataRemainsExact(t *testing.T) {
	for _, counted := range []bool{false, true} {
		entry := goalHistoryFixtureV1(counted)
		entry["goal"], entry["operation"] = nil, "legacy_clear"
		if counted {
			entry["operation"] = "clear"
		}
		input := goalHistoryThreadFixtureV1(entry)
		got, changed := ProjectPublicValue(input)
		if changed || !reflect.DeepEqual(got, input) || ValidatePublicValue(input) != nil {
			t.Fatal("clear history metadata became prose")
		}
	}
	entry := goalHistoryFixtureV1(false)
	entry["operation"] = "legacy_clear"
	delete(entry, "goal")
	input := goalHistoryThreadFixtureV1(entry)
	if ValidatePublicValue(input) == nil {
		t.Fatal("clear entry without the required goal field gained structural routing")
	}
}

func TestGoalHistoryMetadataRejectsPIIWithoutTimestampCollision(t *testing.T) {
	for _, location := range []string{"entry-id", "goal-id", "thread-id", "ref-id"} {
		t.Run(location, func(t *testing.T) {
			entry := goalHistoryFixtureV1(true)
			snapshot := entry["goal"].(map[string]any)
			const safeTime = "2026-10-18T00:00:00Z"
			const safeID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
			entry["id"], entry["createdAt"] = safeID, safeTime
			snapshot["id"], snapshot["createdAt"], snapshot["updatedAt"] = safeID, safeTime, safeTime
			entry["ref"].(map[string]any)["id"] = safeID
			switch location {
			case "entry-id":
				entry["id"] = "13800138000"
			case "goal-id":
				snapshot["id"] = "13800138000"
			case "thread-id":
				snapshot["threadId"] = "13800138000"
			case "ref-id":
				entry["ref"].(map[string]any)["id"] = "13800138000"
			}
			input := goalHistoryThreadFixtureV1(entry)
			input["createdAt"], input["updatedAt"] = safeTime, safeTime
			if ValidatePublicValue(input) == nil {
				t.Fatal("PII identifier gained metadata routing without a timestamp collision")
			}
			got, changed := ProjectPublicValue(input)
			encoded, _ := json.Marshal(got)
			if !changed || strings.Contains(string(encoded), "13800138000") || ValidatePublicValue(got) != nil {
				t.Fatal("PII metadata was not conservatively projected", got)
			}
		})
	}
}
