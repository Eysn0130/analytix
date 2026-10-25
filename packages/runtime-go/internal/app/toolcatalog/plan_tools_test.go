package toolcatalog

import (
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	"encoding/json"
	"strings"
	"testing"
)

func TestCreatePlanToolSchemaPreservesPlanContract(t *testing.T) {
	schema := CreatePlanToolSchema()
	if schema.Name != ToolCreatePlanName || schema.Source != "plan" {
		t.Fatalf("plan tool identity drifted: %#v", schema)
	}
	var decoded struct {
		Required []string       `json:"required"`
		Props    map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema.Parameters, &decoded); err != nil {
		t.Fatalf("decode plan schema: %v", err)
	}
	if len(decoded.Required) != 2 || decoded.Required[0] != "markdown" || decoded.Required[1] != "operation" {
		t.Fatalf("plan schema required fields drifted: %#v", decoded.Required)
	}
	if decoded.Props["plan_relative_path"] == nil || decoded.Props["plan_id"] == nil {
		t.Fatalf("plan schema lost GUI-owned path/id guards: %#v", decoded.Props)
	}
}

func TestExitPlanReviewUsesOriginalArgumentsAndOnlyNativeApproval(t *testing.T) {
	plan := "# Complete synthetic plan\n\nKeep the full original detail.\n"
	args := map[string]any{"plan": plan}
	derived, err := UserInputArgumentsForTool(ToolExitPlanModeName, args)
	if err != nil || len(args) != 1 || args["plan"] != plan {
		t.Fatal("review changed immutable original arguments", err)
	}
	inputID := "input_" + strings.Repeat("a", 64)
	questions := UserInputQuestionsForLocalDisplay(derived, inputID, "fallback")
	if len(questions) != 1 || questions[0]["detail"] != plan || questions[0]["multiSelect"] != false {
		t.Fatal("review lost complete signed detail")
	}
	options := questions[0]["options"].([]map[string]string)
	if len(options) != 2 || options[0]["id"] != inputID+"_1_option_1" {
		t.Fatal("review invented option association")
	}
	for _, args := range []map[string]any{{"plan": "plain"}, {"plan": plan, "timeout": 1}, {"plan": nil}} {
		if _, err := UserInputArgumentsForTool(ToolExitPlanModeName, args); err == nil {
			t.Fatal("invalid plan admitted")
		}
	}
	for _, test := range []struct {
		body string
		want bool
	}{
		{`[{"id":"` + inputID + `_1","kind":"answer","optionIds":["` + inputID + `_1_option_1"]}]`, true},
		{`[{"id":"` + inputID + `_1","kind":"answer","optionIds":["` + inputID + `_1_option_2"]}]`, false},
		{`[{"id":"` + inputID + `_1","kind":"answer","optionIds":[],"customText":"Approve"}]`, false},
		{`[{"id":"` + inputID + `_1","kind":"skip"}]`, false},
	} {
		submission := &domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: json.RawMessage(test.body)}
		if PlanReviewApprovedSubmissionV1(ToolExitPlanModeName, inputID, submission) != test.want || PlanReviewApprovedSubmissionV1("request_user_input", inputID, submission) {
			t.Fatal("approval was inferred from prose, mode or wrong option")
		}
	}
	schema := ExitPlanModeToolSchema()
	if schema.Name != ToolExitPlanModeName || !IsUserInputTool(schema.Name) || ToolContractPolicy(schema.Name) != "auto" {
		t.Fatal("native review catalog path is unavailable")
	}
	var parameters map[string]any
	if json.Unmarshal(schema.Parameters, &parameters) != nil || parameters["additionalProperties"] != false || parameters["properties"].(map[string]any)["timeout"] != nil {
		t.Fatal("review changed legacy blocking contract")
	}
}
