package toolcatalog

import (
	"regexp"
	"strings"
	"unicode/utf8"

	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const (
	RouteDirectAnswer = "direct_answer"
	RouteLightAgent   = "light_agent"
	RouteToolAgent    = "tool_agent"
	RouteSubagent     = "subagent_agent"
)

var promptAliasPattern = regexp.MustCompile(`[^a-z0-9]+`)

type PromptRouteInput struct {
	Prompt         string
	ToolScope      []string
	Subagent       bool
	PlanActive     bool
	DiagnosticsKey string
	Skills         []map[string]any
}

func GoalToolsPromptActive(prompt string, toolScope []string, diagnosticsKey string) bool {
	if prompt == diagnosticsKey {
		return true
	}
	if ToolScopeIncludesAny(toolScope, "get_goal", "create_goal", "complete_step", "update_goal", "todo_list", "todo_write", "todo_ops", "todo_patch") {
		return true
	}
	raw := strings.ToLower(strings.TrimSpace(prompt))
	compact := strings.ToLower(MCPPromptAliasText(prompt))
	for _, cue := range []string{
		"/goal",
		"/todo",
		"get_goal",
		"complete_step",
		"update_goal",
		"create_goal",
		"todo_write",
		"todo_list",
		"todo_ops",
		"todo_patch",
		"current goal",
		"create goal",
		"create a goal",
		"set goal",
		"mark goal",
		"complete goal",
		"update goal",
		"todo list",
		"todo ops",
		"todo patch",
		"创建目标",
		"设置目标",
		"当前目标",
		"更新目标",
		"完成目标",
		"待办列表",
		"写入待办",
	} {
		if strings.Contains(compact, cue) || strings.Contains(raw, cue) {
			return true
		}
	}
	return false
}

func PromptRouteForPrompt(input PromptRouteInput) string {
	if input.Prompt == input.DiagnosticsKey {
		return RouteSubagent
	}
	if PromptUsesDirectAnswerToolProfile(input.Prompt, input.ToolScope, input.Subagent, input.PlanActive) {
		return RouteDirectAnswer
	}
	if PromptRequestsSubagentTools(input.Prompt, input.ToolScope) {
		return RouteSubagent
	}
	if PromptMentionsSkillTool(input.Prompt, input.Skills, input.ToolScope) {
		return RouteToolAgent
	}
	if PromptContainsAgentWorkCue(strings.ToLower(strings.TrimSpace(input.Prompt))) {
		return RouteToolAgent
	}
	return RouteLightAgent
}

func PromptRouteAllowsSubagentTools(route string, toolScope []string) bool {
	return route == RouteSubagent || ToolScopeIncludesAny(toolScope, "delegate_task", "task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job", "list_jobs")
}

func PromptRouteForAdvertisedTools(route string, tools []domainmodel.ToolSchema) string {
	if route == RouteDirectAnswer {
		return route
	}
	for _, tool := range tools {
		switch tool.Source {
		case "subagent", "job":
			return RouteSubagent
		case "mcp", "skill", "plan", "goal", "todo":
			return RouteToolAgent
		}
		switch tool.Name {
		case "bash", "generate_office_document", "write", "write_file", "edit", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol", "web_fetch":
			return RouteToolAgent
		}
	}
	return route
}

func PromptRequestsSubagentTools(prompt string, toolScope []string) bool {
	if ToolScopeIncludesAny(toolScope, "delegate_task", "task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job", "list_jobs") {
		return true
	}
	compact := MCPPromptAliasText(prompt)
	for _, token := range strings.Fields(compact) {
		switch token {
		case "child", "children", "subtask", "subtasks":
			return true
		}
	}
	for _, cue := range []string{
		"subagent",
		"sub agent",
		"child agent",
		"child task",
		"task tool",
		"delegate",
		"delegate task",
		"task_tool",
		"tool task",
		"call task",
		"parallel task",
		"parallel tasks",
		"parallel_tasks",
		"run in parallel",
		"concurrently",
		"background job",
		"background task",
		"start background",
		"cancellable background",
		"background via task",
		"background via tools",
		"run in background",
		"run_in_background",
		"runinbackground",
		"auto continue parent",
		"auto_continue_parent",
		"autocontinueparent",
		"inspect background",
		"background status",
		"background output",
		"list jobs",
		"list background",
		"job status",
		"active subagents",
		"bash output",
		"bash_output",
		"kill shell",
		"kill_shell",
	} {
		if cue != "" && strings.Contains(compact, cue) {
			return true
		}
	}
	raw := strings.ToLower(strings.TrimSpace(prompt))
	for _, cue := range []string{
		"子代理", "子智能体", "子任务", "并行任务", "分解任务", "后台任务", "后台子智能体",
		"创建智能体", "创建子智能体", "等待子任务", "等待后台", "列出后台", "后台状态", "活动子代理", "活动子智能体",
	} {
		if strings.Contains(raw, cue) {
			return true
		}
	}
	return false
}

func PromptMentionsSkillTool(prompt string, skills []map[string]any, toolScope []string) bool {
	if ToolScopeIncludesAny(toolScope, "run_skill", "load_skill") {
		return true
	}
	compact := MCPPromptAliasText(prompt)
	raw := strings.ToLower(strings.TrimSpace(prompt))
	if strings.Contains(compact, "skill") || strings.Contains(compact, "run skill") || strings.Contains(compact, "load skill") ||
		strings.Contains(raw, "技能") {
		return true
	}
	for _, skill := range skills {
		for _, value := range []string{
			firstString(skill["id"]),
			firstString(skill["name"]),
			firstString(skill["title"]),
		} {
			alias := MCPPromptAliasText(value)
			if alias != "" && strings.Contains(compact, alias) {
				return true
			}
		}
	}
	return false
}

func ToolScopeIncludesAny(toolScope []string, names ...string) bool {
	if len(toolScope) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, name := range names {
		allowed[name] = true
	}
	for _, name := range toolScope {
		if allowed[strings.TrimSpace(name)] {
			return true
		}
	}
	return false
}

func PromptUsesDirectAnswerToolProfile(prompt string, toolScope []string, subagent bool, planActive bool) bool {
	if len(toolScope) > 0 || subagent || planActive {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	if normalized == "" || strings.Contains(normalized, "\n") || utf8.RuneCountInString(normalized) > 80 {
		return false
	}
	if PromptContainsAgentWorkCue(normalized) {
		return false
	}
	compact := strings.Join(strings.Fields(normalized), " ")
	identityPatterns := []string{
		"你是什么大模型",
		"你是什么模型",
		"你是什么 ai",
		"你是什么ai",
		"你是谁",
		"介绍一下你自己",
		"what model are you",
		"which model are you",
		"who are you",
	}
	for _, pattern := range identityPatterns {
		if strings.Contains(compact, pattern) {
			return true
		}
	}
	greetingPatterns := []string{"你好", "您好", "谢谢"}
	for _, pattern := range greetingPatterns {
		if compact == pattern {
			return true
		}
	}
	lightweightPatterns := []string{
		"这个问题怎么理解",
		"这是什么意思",
		"简单总结一下",
		"简单解释一下",
		"帮我理解一下",
	}
	for _, pattern := range lightweightPatterns {
		if compact == pattern || strings.HasPrefix(compact, pattern+" ") || strings.HasPrefix(compact, pattern+"，") || strings.HasPrefix(compact, pattern+",") {
			return true
		}
	}
	if utf8.RuneCountInString(compact) > 48 {
		return false
	}
	if strings.Contains(compact, "?") || strings.Contains(compact, "？") {
		return true
	}
	chineseQuestionPrefixes := []string{"什么", "为什么", "怎么", "如何", "是否", "是不是", "能不能", "可以吗", "请问"}
	for _, prefix := range chineseQuestionPrefixes {
		if strings.HasPrefix(compact, prefix) {
			return true
		}
	}
	englishQuestionPrefixes := []string{"what ", "why ", "how ", "is ", "are ", "can ", "could ", "should ", "do ", "does "}
	for _, prefix := range englishQuestionPrefixes {
		if strings.HasPrefix(compact, prefix) {
			return true
		}
	}
	return false
}

func PromptContainsAgentWorkCue(normalizedPrompt string) bool {
	cues := []string{
		"analytix",
		"opencode",
		"codex",
		"mcp",
		"plugin",
		"skill",
		"subagent",
		"agent",
		"provider",
		"workspace",
		"thread",
		"deeplink",
		"git",
		"npm",
		"go test",
		"源码",
		"代码",
		"工作区",
		"文件",
		"目录",
		"插件",
		"技能",
		"子代理",
		"工具",
		"线程",
		"深链",
		"读取",
		"打开",
		"搜索",
		"运行",
		"执行",
		"调用",
		"操作",
		"修改",
		"修复",
		"创建",
		"生成",
		"构建",
		"启动",
		"拉取",
		"提交",
		"测试",
		"排查",
		"研究",
		"复核",
		"审核",
		"报告",
		"文档",
		"案件",
		"资金",
		"数据库",
		"截图",
		"图片",
		"附件",
		"错误",
	}
	for _, cue := range cues {
		if strings.Contains(normalizedPrompt, cue) {
			return true
		}
	}
	return false
}

func MCPPromptPinnedToolNames(prompt string, toolNames []string) []string {
	normalizedPrompt := MCPPromptAliasText(prompt)
	if normalizedPrompt == "" {
		return nil
	}
	pinnedServers := map[string]bool{}
	for _, toolName := range toolNames {
		serverID := MCPToolServerID(toolName)
		if serverID == "" {
			continue
		}
		if MCPPromptMentionsServer(normalizedPrompt, serverID) {
			pinnedServers[serverID] = true
		}
	}
	pinned := []string{}
	for _, toolName := range toolNames {
		if strings.Contains(normalizedPrompt, MCPPromptAliasText(toolName)) {
			pinned = append(pinned, toolName)
			continue
		}
		if serverID := MCPToolServerID(toolName); serverID != "" && pinnedServers[serverID] {
			pinned = append(pinned, toolName)
		}
	}
	return UniqueStringsInOrder(pinned)
}

type MCPToolNamesForPromptInput struct {
	Prompt        string
	ToolNames     []string
	ToolScope     []string
	SearchEnabled bool
	ResultLimit   int
	Search        func(string) []string
}

func MCPToolNamesForPrompt(input MCPToolNamesForPromptInput) []string {
	toolNames := UniqueStringsInOrder(input.ToolNames)
	if len(toolNames) == 0 {
		return nil
	}
	if len(input.ToolScope) > 0 || !input.SearchEnabled || input.Search == nil {
		return toolNames
	}
	pinned := MCPPromptPinnedToolNames(input.Prompt, toolNames)
	matches := UniqueStringsInOrder(append(pinned, input.Search(input.Prompt)...))
	if len(matches) == 0 {
		return nil
	}
	available := map[string]bool{}
	for _, name := range toolNames {
		available[name] = true
	}
	limit := input.ResultLimit
	if limit <= 0 {
		limit = 1
	}
	if len(pinned) > limit {
		limit = len(pinned)
	}
	filtered := make([]string, 0, minInt(limit, len(matches)))
	for _, name := range matches {
		if !available[name] {
			continue
		}
		filtered = append(filtered, name)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func MCPToolServerID(toolName string) string {
	serverID, _, ok := domainmcpname.Parse(toolName)
	if !ok {
		return ""
	}
	return serverID
}

func MCPPromptMentionsServer(normalizedPrompt string, serverID string) bool {
	alias := MCPPromptAliasText(serverID)
	if alias == "" {
		return false
	}
	if !strings.Contains(alias, " ") {
		return false
	}
	if strings.Contains(normalizedPrompt, alias) {
		return true
	}
	return strings.Contains(alias, "computer use") && strings.Contains(normalizedPrompt, "computer use")
}

func MCPPromptAliasText(value string) string {
	normalized := promptAliasPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), " ")
	return strings.Join(strings.Fields(normalized), " ")
}

func UniqueStringsInOrder(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func firstString(value any) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}
