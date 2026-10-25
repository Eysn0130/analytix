package toolcatalog

import (
	"encoding/json"
	"testing"
)

func TestUserInputQuestionsForLocalDisplayPreservesOriginalProviderValues(t *testing.T) {
	args := map[string]any{"questions": []any{
		map[string]any{"question": "  ", "id": "untrusted"},
		map[string]any{"id": "model-q", "question": " Original question ", "header": " Original header ", "detail": " **Original detail**\n", "multi_select": true,
			"options": []any{" ", map[string]any{"id": "model-option", "label": " Same label ", "description": " Original description "}, " Same label "}},
	}}
	questions := UserInputQuestionsForLocalDisplay(args, "input-core", "fallback")
	if len(questions) != 1 || questions[0]["id"] != "input-core_1" || questions[0]["question"] != " Original question " || questions[0]["header"] != " Original header " || questions[0]["detail"] != " **Original detail**\n" {
		t.Fatalf("original Provider fields or Core question identity changed")
	}
	options := questions[0]["options"].([]map[string]string)
	if len(options) != 2 || options[0]["id"] != "input-core_1_option_1" || options[1]["id"] != "input-core_1_option_2" || options[0]["label"] != " Same label " || options[0]["description"] != " Original description " {
		t.Fatal("original options or effective Core identities changed")
	}
	public := UserInputQuestions(args, "input-core", "fallback")
	if public[0]["question"] != "Original question" || public[0]["detail"] != nil {
		t.Fatal("private values entered the public question producer")
	}
}

func TestUserInputQuestionsForLocalDisplaySingletonAndDetailProducer(t *testing.T) {
	args := map[string]any{"prompt": " Original prompt ", "detail": "# Detail\n", "options": []any{" Original option "}}
	question := UserInputQuestionsForLocalDisplay(args, "input-core", "projected fallback")[0]
	if question["question"] != " Original prompt " || question["detail"] != "# Detail\n" || question["id"] != "input-core_1" {
		t.Fatal("singleton original fields were not retained")
	}
	for _, schema := range UserInputToolSchemas() {
		var parameters map[string]any
		if json.Unmarshal(schema.Parameters, &parameters) != nil {
			t.Fatal("invalid user input tool schema")
		}
		properties := parameters["properties"].(map[string]any)
		if properties["detail"] == nil || properties["questions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["detail"] == nil {
			t.Fatal("detail has no real tool-schema producer")
		}
	}
}
