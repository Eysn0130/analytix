package sideeffectidentity

import (
	"encoding/json"
	"path"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func testPathResolver(workspaceRealPath string, requestedPath string) (string, bool) {
	requestedPath = strings.TrimSpace(requestedPath)
	if path.IsAbs(requestedPath) {
		return path.Clean(requestedPath), true
	}
	return path.Clean(path.Join(workspaceRealPath, requestedPath)), true
}

func identityForTest(toolName string, arguments string) (IdentityV1, error) {
	input := Input{
		ToolName: toolName, Arguments: json.RawMessage(arguments),
		WorkspaceRealPath: "/workspace", ResolvePath: testPathResolver,
		ResolveMCP: func(_ string, arguments json.RawMessage) (any, error) {
			return domainsecurity.DecodeCanonicalJSONValue(arguments)
		},
		ResolveCompleteStep: completeStepProjection,
		ResolveUpdateGoal:   updateGoalProjection,
		ResolveTodoOps:      todoOpsProjection,
	}
	input.ResolveNotebook = func(arguments map[string]any) (any, error) { return notebookProjection(input, arguments) }
	input.ResolveDeleteSymbol = func(arguments map[string]any) (any, error) { return deleteSymbolProjection(input, arguments) }
	return ResolveV1(input)
}

func TestGenerationIdentityBindsContentAndRejectsUnownedOptions(t *testing.T) {
	first, err := identityForTest("generate_office_document", `{"path":"report.docx","kind":"docx","markdown":"Synthetic"}`)
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := identityForTest("generate_office_document", `{"path":"/workspace/report.docx","kind":"docx","markdown":"Synthetic","title":"","images":[]}`)
	if err != nil || first.ArgsHash != equivalent.ArgsHash {
		t.Fatal("canonical generation identity drift")
	}
	changed, err := identityForTest("generate_office_document", `{"path":"report.docx","kind":"docx","markdown":"Different"}`)
	if err != nil || first.ArgsHash == changed.ArgsHash {
		t.Fatal("generation identity omitted content")
	}
	if _, err := identityForTest("generate_office_document", `{"path":"report.docx","kind":"docx","markdown":"Synthetic","overwrite":true}`); err == nil {
		t.Fatal("unowned overwrite option accepted")
	}
}

func TestGenerationIdentityBindsPluginActivationAndSource(t *testing.T) {
	input := Input{ToolName: "generate_office_document", Arguments: json.RawMessage(`{"path":"report.docx","kind":"docx","markdown":"Synthetic"}`), WorkspaceRealPath: "/workspace", ResolvePath: testPathResolver}
	baseline, err := ResolveV1(input)
	if err != nil {
		t.Fatal(err)
	}
	input.HostBinding = map[string]any{"generationId": "first", "activationRevision": 1, "sourceRegistrationSha256": strings.Repeat("a", 64)}
	bound, err := ResolveV1(input)
	if err != nil || bound.ArgsHash == baseline.ArgsHash {
		t.Fatal("plugin authority absent from identity", err)
	}
	input.HostBinding = map[string]any{"generationId": "first", "activationRevision": 2, "sourceRegistrationSha256": strings.Repeat("a", 64)}
	reenabled, err := ResolveV1(input)
	if err != nil || bound.ArgsHash == reenabled.ArgsHash {
		t.Fatal("reenable inherited an old identity", err)
	}
	input.HostBinding = map[string]any{"generationId": "second", "activationRevision": 1, "sourceRegistrationSha256": strings.Repeat("b", 64)}
	upgraded, err := ResolveV1(input)
	if err != nil || bound.ArgsHash == upgraded.ArgsHash {
		t.Fatal("upgrade inherited an old identity", err)
	}
}

func TestResolveV1CanonicalizesOwnerParserSemantics(t *testing.T) {
	tests := []struct {
		name       string
		firstTool  string
		firstArgs  string
		secondTool string
		secondArgs string
	}{
		{
			name: "relative and absolute write target", firstTool: "write", firstArgs: `{"path":"dir/../result.txt","content":"x"}`,
			secondTool: "write_file", secondArgs: `{"content":"x","path":"/workspace/result.txt"}`,
		},
		{
			name: "edit list overrides ignored root edit", firstTool: "edit", firstArgs: `{"path":"a.txt","oldText":"ignored-a","newText":"ignored-b","edits":[{"oldText":"a","newText":"b"}]}`,
			secondTool: "multi_edit", secondArgs: `{"path":"/workspace/a.txt","edits":[{"old_string":"a","new_string":"b","replace_all":false}]}`,
		},
		{
			name: "bash fractional timeout follows typed parser", firstTool: "bash", firstArgs: `{"command":"  printf x  ","timeout":31}`,
			secondTool: "bash", secondArgs: `{"command":"printf x","timeout":31.5,"runInBackground":false}`,
		},
		{
			name: "bash omitted timeout uses product default", firstTool: "bash", firstArgs: `{"command":"printf x"}`,
			secondTool: "bash", secondArgs: `{"command":"printf x","timeout":120}`,
		},
		{
			name: "task integer lexical form and implicit profile policy", firstTool: "task", firstArgs: `{"prompt":" inspect ","profile":"inherit","max_steps":31}`,
			secondTool: "delegate_task", secondArgs: `{"prompt":"inspect","profile":"inherit","toolPolicy":"inherit","maxSteps":31.0,"returnFormat":"summary"}`,
		},
		{
			name: "notebook cell id overrides cell number", firstTool: "notebook_edit", firstArgs: `{"path":"a.ipynb","cellId":"cell-a","cell_number":1,"new_source":"x"}`,
			secondTool: "notebook_edit", secondArgs: `{"path":"/workspace/a.ipynb","cell_id":"cell-a","cell_number":99.0,"newString":"x","edit_mode":"replace"}`,
		},
		{
			name: "kill job id and default reason", firstTool: "kill_shell", firstArgs: `{"job_id":" job-a "}`,
			secondTool: "kill_shell", secondArgs: `{"jobId":"job-a","reason":"killed by parent"}`,
		},
		{
			name: "goal trim and integer lexical form", firstTool: "create_goal", firstArgs: `{"objective":" ship ","token_budget":31}`,
			secondTool: "create_goal", secondArgs: `{"objective":"ship","tokenBudget":31.0,"strictCompletion":false}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, err := identityForTest(test.firstTool, test.firstArgs)
			if err != nil {
				t.Fatal(err)
			}
			second, err := identityForTest(test.secondTool, test.secondArgs)
			if err != nil || first != second {
				t.Fatalf("owner-equivalent requests diverged: first=%#v second=%#v err=%v", first, second, err)
			}
		})
	}
}

func TestResolveV1BlockedReasonUsesOwnerPrivacyAndComparisonNormalization(t *testing.T) {
	first, err := identityForTest("update_goal", `{"status":"blocked","reason":"Waiting--on account 6222020000000000000!!"}`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := identityForTest("update_goal", `{"status":"blocked","reason":"waiting on account 6217000000000000000"}`)
	if err != nil || first != second {
		t.Fatalf("owner-equivalent private blocker reasons diverged: first=%#v second=%#v err=%v", first, second, err)
	}
}

func TestResolveV1PreservesMeaningfulDifferences(t *testing.T) {
	first, err := identityForTest("bash", `{"command":"printf x","timeout":30}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []string{
		`{"command":"printf y","timeout":30}`,
		`{"command":"printf x","timeout":31}`,
		`{"command":"printf x","timeout":30,"run_in_background":true}`,
	} {
		second, err := identityForTest("bash", changed)
		if err != nil || first == second {
			t.Fatalf("meaningful bash change retained identity: first=%#v second=%#v err=%v", first, second, err)
		}
	}
}

func TestResolveV1MCPKeepsExactCanonicalNumberSemantics(t *testing.T) {
	integer, err := identityForTest("mcp__funds__mutate", `{"amount":1,"memo":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	decimal, err := identityForTest("mcp__funds__mutate", `{"memo":"x","amount":1.0}`)
	if err != nil || integer == decimal {
		t.Fatalf("remote MCP number semantics were guessed: integer=%#v decimal=%#v err=%v", integer, decimal, err)
	}
	reordered, err := identityForTest("mcp__funds__mutate", " { \n \"memo\" : \"x\", \"amount\" : 1 } ")
	if err != nil || integer != reordered {
		t.Fatalf("canonical MCP key/whitespace equivalence diverged: integer=%#v reordered=%#v err=%v", integer, reordered, err)
	}
}

func TestResolveV1FailsClosedWithoutHostAuthoritativeMutatingMCPContract(t *testing.T) {
	if _, err := ResolveV1(Input{
		ToolName: "mcp__funds__mutate", Arguments: json.RawMessage(`{"amount":1}`),
	}); err == nil {
		t.Fatal("mutating MCP received a guessed remote semantic identity")
	}
}

func TestResolveV1FailsClosedWhenHostPathIdentityIsUnavailable(t *testing.T) {
	for _, resolver := range []PathResolver{
		nil,
		func(string, string) (string, bool) { return "", true },
		func(string, string) (string, bool) { return "/workspace", false },
	} {
		if _, err := ResolveV1(Input{
			ToolName: "bash", Arguments: json.RawMessage(`{"command":"printf ok"}`),
			WorkspaceRealPath: "/workspace", ResolvePath: resolver,
		}); err == nil {
			t.Fatal("bash received a semantic identity without a host-verified workspace path")
		}
	}
}

func TestResolveV1UsesExecutionGrantArgumentLimits(t *testing.T) {
	content := strings.Repeat("x", 768*1024)
	arguments, err := json.Marshal(map[string]any{"path": "result.txt", "content": content})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveV1(Input{
		ToolName: "write_file", Arguments: arguments, WorkspaceRealPath: "/workspace", ResolvePath: testPathResolver,
	}); err != nil {
		t.Fatalf("execution-grant-compatible arguments were rejected: %v", err)
	}
}

func TestResolveV1FailsClosedForUnknownHostTool(t *testing.T) {
	if _, err := identityForTest("future_host_writer", `{}`); err == nil {
		t.Fatal("unknown host writer received a fallback semantic identity")
	}
}

func TestResolveV1UsesResolvedSkillOwnerSemantics(t *testing.T) {
	resolveSkill := func(name string) (map[string]any, bool) {
		normalized := strings.ToLower(strings.Trim(strings.TrimSpace(name), "$@"))
		normalized = strings.ReplaceAll(normalized, " ", "-")
		if normalized != "deep-review" {
			return nil, false
		}
		return map[string]any{
			"id": "deep-review", "name": "Deep Review", "runAs": "inline",
			"packageDigest": strings.Repeat("a", 64), "entry": "SKILL.md",
		}, true
	}
	resolveBody := func(map[string]any) (string, error) { return "Inspect carefully.", nil }
	first, err := ResolveV1(Input{
		ToolName: "run_skill", Arguments: json.RawMessage(`{"name":"$Deep Review","arguments":" inspect "}`),
		WorkspaceRealPath: "/workspace", ResolvePath: testPathResolver, ResolveSkill: resolveSkill, ResolveSkillBody: resolveBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveV1(Input{
		ToolName: "run_skill", Arguments: json.RawMessage(`{"skill_id":"deep-review","task":"inspect","providerId":"ignored","model":"ignored"}`),
		WorkspaceRealPath: "/workspace", ResolvePath: testPathResolver, ResolveSkill: resolveSkill, ResolveSkillBody: resolveBody,
	})
	if err != nil || first != second {
		t.Fatalf("aliases for the same inline skill diverged: first=%#v second=%#v err=%v", first, second, err)
	}
}

func TestResolveV1UsesTodoOwnerPrivacyDefaultsAndProvenance(t *testing.T) {
	first, err := identityForTest("todo_ops", `{"ops":[{"op":"append","content":"contact 13800138000","note":"call 13800138000","source":{"kind":"manual"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := identityForTest("todo_patch", `{"operations":[{"op":"append","content":"contact 13900139000","note":"call 13900139000","status":"pending","source":{"kind":"manual"}}]}`)
	if err != nil || first != second {
		t.Fatalf("execution-equivalent todo ops diverged: first=%#v second=%#v err=%v", first, second, err)
	}
	changed, err := identityForTest("todo_ops", `{"ops":[{"op":"append","content":"contact 13800138000","note":"call 13800138000","source":{"kind":"child","childRunId":"run-1"}}]}`)
	if err != nil || first == changed {
		t.Fatalf("different todo provenance collapsed: first=%#v changed=%#v err=%v", first, changed, err)
	}
}

func TestResolveV1CompleteStepUsesExactGoalOwnerEvidenceSemantics(t *testing.T) {
	shorthand, err := identityForTest("complete_step", `{"step":"verify","evidence":["x"]}`)
	if err != nil {
		t.Fatal(err)
	}
	object, err := identityForTest("complete_step", `{"step":"verify","evidence":[{"kind":"manual","summary":"x"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if shorthand == object {
		t.Fatalf("different persisted goal evidence effects shared identity: shorthand=%#v object=%#v", shorthand, object)
	}

	alias, err := identityForTest("complete_step", `{"step":" verify ","evidence":[{"summary":" x ","kind":"manual"}]}`)
	if err != nil || alias != object {
		t.Fatalf("owner-equivalent object evidence diverged: object=%#v alias=%#v err=%v", object, alias, err)
	}
}

func TestSupportsHostToolV1CoversExecutedNonReadOnlyBuiltins(t *testing.T) {
	for _, name := range []string{
		"bash", "write", "write_file", "edit", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol",
		"task", "delegate_task", "parallel_tasks", "run_skill", "kill_shell", "restart_job", "create_goal", "complete_step",
		"update_goal", "todo_write", "todo_ops", "todo_patch", "create_plan", "native_selection_propose",
	} {
		if !SupportsHostToolV1(name) {
			t.Fatalf("executed non-read-only host tool %q lacks a V1 semantic canonicalizer", name)
		}
	}
	for _, name := range []string{"read", "wait", "list_jobs", "bash_output", "stage_case_report", "future_host_writer", "native_selection_read", "native_selection_propose_extra"} {
		if SupportsHostToolV1(name) {
			t.Fatalf("tool %q was incorrectly classified as a generic V1 side-effect canonicalizer", name)
		}
	}
}

func nativeProposalIdentityArgumentsForTest(payload string) json.RawMessage {
	arguments := `{"scopeId":"abdccbadecddaaebaebadfecefeeeedfbecabfdeadebdfad","operationId":"desktop178_b7e49c1032ad865f"`
	if payload != "" {
		arguments += "," + payload
	}
	return json.RawMessage(arguments + `}`)
}

func TestNativeSelectionProposeIdentityBindsScopedTypedPayloads(t *testing.T) {
	protectedA, protectedB := "protected_"+strings.Repeat("a", 48), "protected_"+strings.Repeat("b", 48)
	canvasID := "canvas_" + strings.Repeat("a", 48)
	tests := []struct{ name, payload, changed string }{
		{"ordered parts", `"parts":[{"kind":"literal","text":"First"},{"kind":"literal","text":"Second"}]`, `"parts":[{"kind":"literal","text":"Second"},{"kind":"literal","text":"First"}]`},
		{"protected reference", `"parts":[{"kind":"literal","text":"Edit "},{"kind":"protected","protectedRef":"` + protectedA + `"}]`, `"parts":[{"kind":"literal","text":"Edit "},{"kind":"protected","protectedRef":"` + protectedB + `"}]`},
		{"workbook exact integer", `"workbook":{"kind":"number","value":9007199254740992}`, `"workbook":{"kind":"number","value":9007199254740993}`},
		{"workbook exact decimal", `"workbook":{"kind":"number","value":1.00000000000000001}`, `"workbook":{"kind":"number","value":1.00000000000000002}`},
		{"workbook formula", `"workbook":{"kind":"formula","formula":"=A1+1"}`, `"workbook":{"kind":"formula","formula":"=A1+2"}`},
		{"workbook range", `"workbook":{"kind":"range","cells":[{"rowOffset":0,"columnOffset":0,"type":"text","text":"Before"}]}`, `"workbook":{"kind":"range","cells":[{"rowOffset":0,"columnOffset":0,"type":"text","text":"After"}]}`},
		{"canvas layout", `"canvas":[{"kind":"set-node-layout","id":"` + canvasID + `","layout":{"x":0,"y":0,"width":1,"height":1}}]`, `"canvas":[{"kind":"set-node-layout","id":"` + canvasID + `","layout":{"x":0,"y":0,"width":2,"height":1}}]`},
		{"canvas route", `"canvas":[{"kind":"set-edge-route","id":"` + canvasID + `","points":[]}]`, `"canvas":[{"kind":"set-edge-route","id":"` + canvasID + `","points":[{"x":0,"y":0},{"x":1,"y":1}]}]`},
		{"canvas label", `"canvas":[{"kind":"set-display-label","id":"` + canvasID + `","target":"node","displayLabel":"Before"}]`, `"canvas":[{"kind":"set-display-label","id":"` + canvasID + `","target":"node","displayLabel":"After"}]`},
		{"canvas style", `"canvas":[{"kind":"set-style","id":"` + canvasID + `","target":"node","style":{"fill":"#aabbcc","stroke":"#112233","strokeWidth":1,"dash":"solid","shape":"ellipse"}}]`, `"canvas":[{"kind":"set-style","id":"` + canvasID + `","target":"node","style":{"fill":"#aabbcc","stroke":"#112233","strokeWidth":1,"dash":"solid","shape":"rectangle"}}]`},
		{"presentation geometry", `"presentation":{"kind":"shape-geometry","x100thMm":0,"y100thMm":0,"width100thMm":1,"height100thMm":1}`, `"presentation":{"kind":"shape-geometry","x100thMm":1,"y100thMm":0,"width100thMm":1,"height100thMm":1}`},
		{"presentation fill", `"presentation":{"kind":"shape-fill","rgb":"#123456"}`, `"presentation":{"kind":"shape-fill","rgb":"#123457"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := nativeProposalIdentityArgumentsForTest(test.payload)
			baseline, err := identityForTest("native_selection_propose", string(arguments))
			if err != nil || baseline.ArgsHash == "" {
				t.Fatalf("registered typed request rejected: %v", err)
			}
			value, err := domainsecurity.DecodeCanonicalJSONValue(arguments)
			if err != nil {
				t.Fatal(err)
			}
			reordered, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			equivalent, err := identityForTest("native_selection_propose", string(reordered))
			if err != nil || equivalent != baseline {
				t.Fatal("object key order changed semantic identity", err)
			}
			// Different well-formed references are different requests, not proof of
			// selection authority. The execution owner validates the referenced scope.
			for name, changed := range map[string]string{
				"scope":     strings.Replace(string(arguments), "abdccbadecddaaebaebadfecefeeeedfbecabfdeadebdfad", strings.Repeat("b", 48), 1),
				"operation": strings.Replace(string(arguments), "desktop178_b7e49c1032ad865f", "desktop178_b7e49c1032ad865f_retry", 1),
				"payload":   string(nativeProposalIdentityArgumentsForTest(test.changed)),
			} {
				identity, err := identityForTest("native_selection_propose", changed)
				if err != nil || identity.ArgsHash == baseline.ArgsHash {
					t.Fatalf("%s change vanished from identity: %v", name, err)
				}
			}
		})
	}
}

func TestNativeSelectionProposeIdentityRetainsTrustedHostBindingWithoutResourceReads(t *testing.T) {
	input := Input{
		ToolName: "native_selection_propose", Arguments: nativeProposalIdentityArgumentsForTest(`"parts":[{"kind":"literal","text":"Scoped edit."}]`),
		ResolvePath: func(string, string) (string, bool) {
			t.Fatal("native request identity must not read or resolve a document")
			return "", false
		},
	}
	baseline, err := ResolveV1(input)
	if err != nil {
		t.Fatal(err)
	}
	input.HostBinding = map[string]any{"generationId": "synthetic-generation", "activationRevision": 1}
	bound, err := ResolveV1(input)
	if err != nil || baseline == bound {
		t.Fatal("existing trusted HostBinding was omitted", err)
	}
	input.HostBinding = map[string]any{"generationId": "synthetic-generation", "activationRevision": 2}
	changed, err := ResolveV1(input)
	if err != nil || changed == bound {
		t.Fatal("changed trusted HostBinding was omitted", err)
	}
}

func TestNativeSelectionProposeIdentityRejectsInvalidRequests(t *testing.T) {
	canvasID := "canvas_" + strings.Repeat("a", 48)
	tests := []struct{ name, payload string }{
		{"missing payload", ``},
		{"mixed payloads", `"parts":[],"workbook":{"kind":"number","value":1}`},
		{"unknown root field", `"parts":[],"unexpected":true`},
		{"null parts", `"parts":null`},
		{"null workbook", `"workbook":null`},
		{"null canvas", `"canvas":null`},
		{"null presentation", `"presentation":null`},
		{"literal type", `"parts":[{"kind":"literal","text":1}]`},
		{"protected reference", `"parts":[{"kind":"protected","protectedRef":"protected_invalid"}]`},
		{"protected extra field", `"parts":[{"kind":"protected","protectedRef":"protected_` + strings.Repeat("a", 48) + `","text":"forged"}]`},
		{"parts limit", `"parts":[` + strings.TrimSuffix(strings.Repeat(`{"kind":"literal","text":"x"},`, 257), ",") + `]`},
		{"literal limit", `"parts":[{"kind":"literal","text":"` + strings.Repeat("x", 65537) + `"}]`},
		{"workbook number type", `"workbook":{"kind":"number","value":"1"}`},
		{"workbook formula", `"workbook":{"kind":"formula","formula":""}`},
		{"workbook range offset", `"workbook":{"kind":"range","cells":[{"rowOffset":256,"columnOffset":0,"type":"number","value":1}]}`},
		{"workbook extra field", `"workbook":{"kind":"number","value":1,"path":"other.xlsx"}`},
		{"canvas reference", `"canvas":[{"kind":"set-display-label","id":"canvas_invalid","target":"node","displayLabel":"x"}]`},
		{"canvas route length", `"canvas":[{"kind":"set-edge-route","id":"` + canvasID + `","points":[{"x":0,"y":0}]}]`},
		{"canvas size", `"canvas":[{"kind":"set-node-layout","id":"` + canvasID + `","layout":{"x":0,"y":0,"width":0,"height":1}}]`},
		{"presentation integer", `"presentation":{"kind":"shape-geometry","x100thMm":0,"y100thMm":0,"width100thMm":1.5,"height100thMm":1}`},
		{"presentation extra selector", `"presentation":{"kind":"shape-fill","rgb":"#123456","shapeIndex":0}`},
		{"presentation color", `"presentation":{"kind":"shape-fill","rgb":"#ABCDEF"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := nativeProposalIdentityArgumentsForTest(test.payload)
			if _, err := identityForTest("native_selection_propose", string(arguments)); err == nil {
				t.Fatal("request outside the actual closed native schema received an identity")
			}
		})
	}
	valid := string(nativeProposalIdentityArgumentsForTest(`"parts":[]`))
	for name, arguments := range map[string]string{
		"invalid scope":     strings.Replace(valid, "abdccbadecddaaebaebadfecefeeeedfbecabfdeadebdfad", "invalid", 1),
		"invalid operation": strings.Replace(valid, "desktop178_b7e49c1032ad865f", "short", 1),
		"duplicate key":     strings.Replace(valid, `"parts":[]`, `"parts":[],"parts":[]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := identityForTest("native_selection_propose", arguments); err == nil {
				t.Fatal("invalid request received an identity")
			}
		})
	}
}
