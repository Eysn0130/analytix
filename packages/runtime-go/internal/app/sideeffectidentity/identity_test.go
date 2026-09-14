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
		"update_goal", "todo_write", "todo_ops", "todo_patch", "create_plan",
	} {
		if !SupportsHostToolV1(name) {
			t.Fatalf("executed non-read-only host tool %q lacks a V1 semantic canonicalizer", name)
		}
	}
	for _, name := range []string{"read", "wait", "list_jobs", "bash_output", "stage_case_report", "future_host_writer"} {
		if SupportsHostToolV1(name) {
			t.Fatalf("tool %q was incorrectly classified as a generic V1 side-effect canonicalizer", name)
		}
	}
}
