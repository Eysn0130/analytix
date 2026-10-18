package toolcatalog

import (
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"encoding/json"
	"testing"
)

func TestUserInputToolSchemasKeepGateAliasPair(t *testing.T) {
	schemas := UserInputToolSchemas()
	if len(schemas) != 2 {
		t.Fatalf("user input schema count drifted: %d", len(schemas))
	}
	for _, schema := range schemas {
		if schema.Source != "user_input" {
			t.Fatalf("schema %s source drifted: %#v", schema.Name, schema)
		}
	}
	if schemas[0].Name != "user_input" || schemas[1].Name != "request_user_input" {
		t.Fatalf("user input schema alias order drifted: %#v", schemas)
	}
}

func TestUserInputSchemasAcceptMultiSelectButRejectProviderIDs(t *testing.T) {
	for _, raw := range []string{
		`{"prompt":"Choose","questions":[{"question":"Choose","multi_select":true,"options":[{"label":"One"},{"label":"Two"}]}]}`,
		`{"prompt":"Choose","multi_select":true,"options":["One","Two"]}`,
		`{"prompt":"Choose","questions":[{"question":" "},{"question":"First"},{"question":"Second"}]}`,
	} {
		if details, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(raw)}, UserInputToolSchemas()); blocked {
			t.Fatalf("multi select schema rejected: %#v", details)
		}
	}
	for _, raw := range []string{
		`{"prompt":"Choose","questions":[{"id":"forged","question":"Choose"}]}`,
		`{"prompt":"Choose","questions":[{"question":"Choose","options":[{"id":"forged","label":"One"}]}]}`,
		`{"prompt":"Choose","multi_select":"true"}`,
		`{"prompt":"Choose","questions":[{"question":"Choose","multi_select":1}]}`,
		`{"prompt":"Choose","questions":[{"question":"a"},{"question":"b"},{"question":"c"},{"question":"d"}]}`,
		`{"prompt":"Choose","options":["a","b","c","d"]}`,
	} {
		if _, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(raw)}, UserInputToolSchemas()); !blocked {
			t.Fatalf("invalid provider carrier accepted: %s", raw)
		}
	}
}

func TestTimedQuestionPolicyIsOptInAndCallTimeoutTakesPrecedence(t *testing.T) {
	wait, err := UserInputTimedWait(UserInputConfig{}, json.RawMessage(`{"prompt":"choose"}`))
	if err != nil || wait != nil {
		t.Fatal("legacy acquired a deadline", err)
	}
	configured := 45
	for _, test := range []struct {
		args string
		want int
	}{{`{"prompt":"choose"}`, 45}, {`{"timeout":1}`, 1}, {`{"timeout":-1}`, -1}, {`{"timeout":2147483}`, 2147483}} {
		wait, err := UserInputTimedWait(UserInputConfig{Mode: "timed", TimeoutSeconds: &configured}, json.RawMessage(test.args))
		if err != nil || wait == nil || wait.TimeoutSeconds != test.want {
			t.Fatal("timed precedence", test.args, err)
		}
	}
	wait, err = UserInputTimedWait(UserInputConfig{Mode: "timed"}, json.RawMessage(`{}`))
	if err != nil || wait.TimeoutSeconds != 120 {
		t.Fatal("timed default", err)
	}
	for _, raw := range []string{`{"timeout":null}`, `{"timeout":0}`, `{"timeout":1.5}`, `{"timeout":-2}`, `{"timeout":2147484}`, `{"timeout":"1"}`} {
		if _, err := UserInputTimedWait(UserInputConfig{Mode: "timed"}, json.RawMessage(raw)); err == nil {
			t.Fatal("invalid timeout", raw)
		}
	}
	for _, schema := range UserInputToolSchemas(UserInputConfig{Mode: "timed"}) {
		if _, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: schema.Name, Arguments: json.RawMessage(`{"prompt":"choose","timeout":-1}`)}, []domainmodel.ToolSchema{schema}); blocked {
			t.Fatal("timed schema rejected unlimited")
		}
	}
	if _, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: "user_input", Arguments: json.RawMessage(`{"prompt":"choose","timeout":120}`)}, UserInputToolSchemas()); !blocked {
		t.Fatal("legacy schema silently opted into timed")
	}
}
