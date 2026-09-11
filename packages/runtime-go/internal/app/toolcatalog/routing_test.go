package toolcatalog

import (
	"reflect"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestPromptRouteForPromptClassifiesDirectWorkSubagentAndSkill(t *testing.T) {
	const diagnosticsKey = "\x00diagnostics"
	cases := []struct {
		name  string
		input PromptRouteInput
		want  string
	}{
		{
			name: "diagnostics keeps complete tool surface",
			input: PromptRouteInput{
				Prompt:         diagnosticsKey,
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteSubagent,
		},
		{
			name: "short question is direct answer",
			input: PromptRouteInput{
				Prompt:         "what model are you?",
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteDirectAnswer,
		},
		{
			name: "work prompt keeps tools",
			input: PromptRouteInput{
				Prompt:         "请分析项目代码",
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteToolAgent,
		},
		{
			name: "subagent cue routes to child capable agent",
			input: PromptRouteInput{
				Prompt:         "请用子代理分解任务",
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteSubagent,
		},
		{
			name: "child intelligent agent cue routes to child capable agent",
			input: PromptRouteInput{
				Prompt:         "请创建 1 个后台子智能体执行只读检查",
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteSubagent,
		},
		{
			name: "explicit task tool arguments route to child capable agent",
			input: PromptRouteInput{
				Prompt:         `Call the task tool with {"run_in_background":true,"auto_continue_parent":true}.`,
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteSubagent,
		},
		{
			name: "natural child request routes to child capable agent",
			input: PromptRouteInput{
				Prompt:         "Run a child with a bounded model.",
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteSubagent,
		},
		{
			name: "natural parallel request routes to child capable agent",
			input: PromptRouteInput{
				Prompt:         "Run the ready wave concurrently.",
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteSubagent,
		},
		{
			name: "natural background request routes to child capable agent",
			input: PromptRouteInput{
				Prompt:         "Start background.",
				DiagnosticsKey: diagnosticsKey,
			},
			want: RouteSubagent,
		},
		{
			name: "skill title cue routes to tool agent",
			input: PromptRouteInput{
				Prompt:         "run deep review",
				DiagnosticsKey: diagnosticsKey,
				Skills:         []map[string]any{{"title": "Deep Review"}},
			},
			want: RouteToolAgent,
		},
	}
	for _, tc := range cases {
		if got := PromptRouteForPrompt(tc.input); got != tc.want {
			t.Fatalf("%s route got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestPromptRouteForAdvertisedToolsPromotesRoute(t *testing.T) {
	if got := PromptRouteForAdvertisedTools(RouteLightAgent, []domainmodel.ToolSchema{{Name: "task", Source: "subagent"}}); got != RouteSubagent {
		t.Fatalf("subagent advertised tools route got %q", got)
	}
	if got := PromptRouteForAdvertisedTools(RouteLightAgent, []domainmodel.ToolSchema{{Name: "web_fetch", Source: "builtin"}}); got != RouteToolAgent {
		t.Fatalf("web fetch advertised tools route got %q", got)
	}
	if got := PromptRouteForAdvertisedTools(RouteDirectAnswer, []domainmodel.ToolSchema{{Name: "bash", Source: "builtin"}}); got != RouteDirectAnswer {
		t.Fatalf("direct answer route should not be promoted, got %q", got)
	}
}

func TestGoalToolsPromptActiveRequiresExplicitCue(t *testing.T) {
	const diagnosticsKey = "\x00diagnostics"
	if !GoalToolsPromptActive(diagnosticsKey, nil, diagnosticsKey) {
		t.Fatalf("diagnostics should activate goal tools")
	}
	if !GoalToolsPromptActive("please create_goal", nil, diagnosticsKey) {
		t.Fatalf("explicit create_goal cue should activate goal tools")
	}
	if !GoalToolsPromptActive("ordinary", []string{"todo_write"}, diagnosticsKey) {
		t.Fatalf("explicit tool scope should activate goal tools")
	}
	if !GoalToolsPromptActive("ordinary", []string{"todo_ops"}, diagnosticsKey) {
		t.Fatalf("explicit todo_ops scope should activate goal tools")
	}
	if !GoalToolsPromptActive("please todo_patch this list", nil, diagnosticsKey) {
		t.Fatalf("explicit todo_patch cue should activate goal tools")
	}
	if !GoalToolsPromptActive("请查看待办列表", nil, diagnosticsKey) {
		t.Fatalf("Chinese todo cue should activate goal tools")
	}
	if GoalToolsPromptActive("general goal of this paragraph", nil, diagnosticsKey) {
		t.Fatalf("generic goal wording should stay inactive")
	}
}

func TestMCPPromptPinnedToolNamesPinsServerAndToolAliases(t *testing.T) {
	tools := []string{
		"mcp__computer_use__screenshot",
		"mcp__docs_server__lookup",
		"mcp__docs_server__search",
	}
	pinned := MCPPromptPinnedToolNames("Use computer use and docs server lookup", tools)
	want := []string{
		"mcp__computer_use__screenshot",
		"mcp__docs_server__lookup",
		"mcp__docs_server__search",
	}
	if !reflect.DeepEqual(pinned, want) {
		t.Fatalf("pinned tools got %#v want %#v", pinned, want)
	}
}

func TestMCPToolNamesForPromptScopesSearchAndKeepsPinnedTools(t *testing.T) {
	tools := []string{
		"mcp__docs_server__lookup",
		"mcp__docs_server__search",
		"mcp__jira__issue",
	}
	got := MCPToolNamesForPrompt(MCPToolNamesForPromptInput{
		Prompt:        "Use docs server for this lookup",
		ToolNames:     tools,
		SearchEnabled: true,
		ResultLimit:   1,
		Search: func(string) []string {
			return []string{"mcp__jira__issue", "mcp__unknown__stale"}
		},
	})
	want := []string{"mcp__docs_server__lookup", "mcp__docs_server__search"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pinned tools should extend search limit, got %#v want %#v", got, want)
	}

	got = MCPToolNamesForPrompt(MCPToolNamesForPromptInput{
		Prompt:        "find linked ticket",
		ToolNames:     append(tools, "mcp__jira__issue"),
		SearchEnabled: true,
		ResultLimit:   2,
		Search: func(string) []string {
			return []string{"mcp__jira__issue", "mcp__unknown__stale", "mcp__docs_server__lookup", "mcp__jira__issue"}
		},
	})
	want = []string{"mcp__jira__issue", "mcp__docs_server__lookup"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("search-scoped tools got %#v want %#v", got, want)
	}
}

func TestMCPToolNamesForPromptKeepsFullCatalogWhenScopeOrSearchDisabled(t *testing.T) {
	tools := []string{"mcp__a__read", "mcp__b__search", "mcp__a__read"}
	got := MCPToolNamesForPrompt(MCPToolNamesForPromptInput{
		Prompt:        "anything",
		ToolNames:     tools,
		ToolScope:     []string{"mcp__a__read"},
		SearchEnabled: true,
		ResultLimit:   1,
		Search: func(string) []string {
			t.Fatalf("explicit tool scope should bypass MCP search")
			return nil
		},
	})
	want := []string{"mcp__a__read", "mcp__b__search"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool scope should return unique full catalog for later schema filtering, got %#v want %#v", got, want)
	}
	if got := MCPToolNamesForPrompt(MCPToolNamesForPromptInput{ToolNames: tools}); !reflect.DeepEqual(got, want) {
		t.Fatalf("disabled search should return unique full catalog, got %#v want %#v", got, want)
	}
}

func TestMCPToolServerIDRejectsAmbiguousName(t *testing.T) {
	if got := MCPToolServerID("mcp__docs__lookup"); got != "docs" {
		t.Fatalf("valid MCP server ID mismatch: %q", got)
	}
	if got := MCPToolServerID("mcp__foo__bar__lookup"); got != "foo" {
		t.Fatalf("valid repeated-underscore MCP name failed: %q", got)
	}
	for _, name := range []string{"mcp__Docs__lookup", "mcp____lookup", "mcp__docs__"} {
		if got := MCPToolServerID(name); got != "" {
			t.Fatalf("ambiguous MCP name was parsed: name=%q server=%q", name, got)
		}
	}
}

func TestUserInputQuestionsNormalizesStructuredAndFallbackQuestions(t *testing.T) {
	questions := UserInputQuestions(map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Pick one",
				"options":  []any{"A", map[string]any{"label": "B", "description": "Bee"}, 3},
			},
			map[string]any{"id": "skip"},
		},
	}, "input_1", "fallback")
	if len(questions) != 1 || questions[0]["id"] != "input_1_1" || questions[0]["header"] != "Question 1" {
		t.Fatalf("structured questions mismatch: %#v", questions)
	}
	options := questions[0]["options"].([]map[string]string)
	if len(options) != 2 || options[0]["label"] != "A" || options[1]["description"] != "Bee" {
		t.Fatalf("question options mismatch: %#v", options)
	}
	fallback := UserInputQuestions(map[string]any{
		"id":      "custom",
		"options": []any{map[string]any{"label": "OK"}},
	}, "input_2", "Continue?")
	if len(fallback) != 1 || fallback[0]["id"] != "input_2_1" || fallback[0]["question"] != "Continue?" {
		t.Fatalf("fallback question mismatch: %#v", fallback)
	}
}

func TestUserInputQuestionsIgnoreModelOriginatedAuthorityIDs(t *testing.T) {
	questions := UserInputQuestions(map[string]any{
		"questions": []any{map[string]any{
			"id":       "6222020000000000000",
			"question": "Continue?",
		}},
	}, "input_host", "fallback")
	if len(questions) != 1 || questions[0]["id"] != "input_host_1" {
		t.Fatalf("model-originated question ID became authority: %#v", questions)
	}
}
