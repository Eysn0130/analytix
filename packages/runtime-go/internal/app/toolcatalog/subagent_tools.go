package toolcatalog

import (
	"encoding/json"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ForegroundTaskToolName       = "task"
	ForegroundSubmitToolName     = "submit_child_result"
	ForegroundMaxSteps           = 4
	ForegroundMaxTokenBudget     = 4096
	ForegroundMaxTimeBudgetMS    = 60000
	ForegroundMaxSubmissionBytes = 8192
)

// ForegroundSubmitToolSchema is the only tool delegated to a foreground child.
// Its payload is consumed in process and never enters durable tool output.
func ForegroundSubmitToolSchema() domainmodel.ToolSchema {
	return domainmodel.ToolSchema{
		Name:        ForegroundSubmitToolName,
		Description: "Required final tool for this foreground child. General children submit result; host case-typed children submit only the exact closed delegation and answer-slot commitment tokens. The host rechecks these selectors against the current delegation; assistant free text and self-issued case facts are never accepted.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"result":{"type":"string","minLength":1,"maxLength":8192},"caseResult":{"type":"object","properties":{"schemaVersion":{"const":1},"purpose":{"const":"analytix.case-foreground-child-selection/v1"},"delegationDigest":{"type":"string","pattern":"^cmt1_[a-p]{64}$"},"answerSlotDigest":{"type":"string","pattern":"^cmt1_[a-p]{64}$"}},"required":["schemaVersion","purpose","delegationDigest","answerSlotDigest"],"additionalProperties":false}},"minProperties":1,"maxProperties":1,"additionalProperties":false}`),
		Source:      "subagent",
	}
}

func IsForegroundSubmitOnlyScope(scope []string) bool {
	return len(scope) == 1 && strings.TrimSpace(scope[0]) == ForegroundSubmitToolName
}

func SecurityScopedToolSchemas(
	securityContext domainsecurity.TurnSecurityContext,
	subagentDepth int,
	schemas []domainmodel.ToolSchema,
) []domainmodel.ToolSchema {
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		return nil
	}
	out := make([]domainmodel.ToolSchema, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Name)
		if MCPToolRequiresHostArtifactAuthority(name) ||
			(MCPToolNeedsAnalytixCaseContext(name) &&
				!domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext)) {
			continue
		}
		if name == ReportDeliveryToolName &&
			(subagentDepth != 0 ||
				domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil) {
			continue
		}
		out = append(out, schema)
	}
	return out
}

func SubagentToolSchemas(profileDescription string) []domainmodel.ToolSchema {
	profileGuidance := " Omit profile unless the user explicitly asks for a named sub-agent profile or the task exactly matches that profile; use no profile for general file, directory, workspace, or desktop analysis."
	return []domainmodel.ToolSchema{
		{
			Name:        "task",
			Description: "Recommended Analytix tool for exactly one child-agent task. Always provide prompt. Use toolPolicy=readOnly for inspection; use toolPolicy=inherit only when the child must run shell commands or write/edit files. Set max_steps/maxSteps for broad scans or multi-step work. Set run_in_background=true for long-running children. Set auto_continue_parent=true only when the user explicitly wants the parent thread to resume automatically after that background child completes; it is gated and never applies to killed, interrupted, restarted, or superseded jobs. Set isolationMode=worktree only for explicit writable background children; this creates an isolated Git worktree and does not merge automatically." + profileGuidance + profileDescription,
			Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"label":{"type":"string"},"prompt":{"type":"string"},"description":{"type":"string"},"profile":{"type":"string"},"toolPolicy":{"type":"string","enum":["readOnly","inherit"]},"tool_policy":{"type":"string","enum":["readOnly","inherit"]},"tools":{"type":"array","items":{"type":"string"}},"blockedTools":{"type":"array","items":{"type":"string"}},"blockedMcpServers":{"type":"array","items":{"type":"string"}},"blockedSkills":{"type":"array","items":{"type":"string"}},"max_steps":{"type":"integer"},"maxSteps":{"type":"integer"},"tokenBudget":{"type":"integer"},"token_budget":{"type":"integer"},"timeBudgetMs":{"type":"integer"},"time_budget_ms":{"type":"integer"},"returnFormat":{"type":"string","enum":["summary","evidence","transcriptRef"]},"return_format":{"type":"string","enum":["summary","evidence","transcriptRef","transcript_ref"]},"run_in_background":{"type":"boolean"},"runInBackground":{"type":"boolean"},"auto_continue_parent":{"type":"boolean"},"autoContinueParent":{"type":"boolean"},"isolationMode":{"type":"string","enum":["worktree"]},"isolation_mode":{"type":"string","enum":["worktree"]},"providerId":{"type":"string"},"provider_id":{"type":"string"},"model":{"type":"string"},"endpointFormat":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"endpoint_format":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"variant":{"type":"string"},"modelVariant":{"type":"string"},"model_variant":{"type":"string"},"effort":{"type":"string","enum":["off","low","medium","high","max"]},"task_id":{"type":"string"},"taskId":{"type":"string"},"continue_from":{"type":"string"},"continueFrom":{"type":"string"},"fork_from":{"type":"string"},"forkFrom":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`),
			Source:      "subagent",
		},
		{
			Name:        "parallel_tasks",
			Description: "Recommended Analytix tool for two or more independent child-agent tasks. Do not use it for a single task; use task instead. Each task must include prompt. Use per-task toolPolicy=readOnly for inspection and toolPolicy=inherit only when that child must run shell commands or write/edit files. Set per-task max_steps/maxSteps for broad scans or multi-step work. Prefer wait/list_jobs for aggregating parallel background children; if multiple tasks request auto_continue_parent, the runtime admits at most one continuation for the parent turn." + profileGuidance + profileDescription,
			Parameters:  json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},"label":{"type":"string"},"depends_on":{"type":"array","items":{"type":"string"}},"dependsOn":{"type":"array","items":{"type":"string"}},"prompt":{"type":"string"},"description":{"type":"string"},"profile":{"type":"string"},"toolPolicy":{"type":"string","enum":["readOnly","inherit"]},"tool_policy":{"type":"string","enum":["readOnly","inherit"]},"tools":{"type":"array","items":{"type":"string"}},"blockedTools":{"type":"array","items":{"type":"string"}},"blockedMcpServers":{"type":"array","items":{"type":"string"}},"blockedSkills":{"type":"array","items":{"type":"string"}},"max_steps":{"type":"integer"},"maxSteps":{"type":"integer"},"tokenBudget":{"type":"integer"},"token_budget":{"type":"integer"},"timeBudgetMs":{"type":"integer"},"time_budget_ms":{"type":"integer"},"returnFormat":{"type":"string","enum":["summary","evidence","transcriptRef"]},"return_format":{"type":"string","enum":["summary","evidence","transcriptRef","transcript_ref"]},"run_in_background":{"type":"boolean"},"runInBackground":{"type":"boolean"},"auto_continue_parent":{"type":"boolean"},"autoContinueParent":{"type":"boolean"},"providerId":{"type":"string"},"provider_id":{"type":"string"},"model":{"type":"string"},"endpointFormat":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"endpoint_format":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"variant":{"type":"string"},"modelVariant":{"type":"string"},"model_variant":{"type":"string"},"effort":{"type":"string","enum":["off","low","medium","high","max"]},"continue_from":{"type":"string"},"continueFrom":{"type":"string"},"fork_from":{"type":"string"},"forkFrom":{"type":"string"}},"required":["prompt"],"additionalProperties":false}}},"required":["tasks"],"additionalProperties":false}`),
			Source:      "subagent",
		},
		{
			Name:        "delegate_task",
			Description: "Legacy compatibility alias for task. Prefer task for one child-agent task and parallel_tasks for multiple child-agent tasks. Use this only when continuing older transcripts that already used delegate_task. Always provide prompt; set toolPolicy=inherit only when the child must run shell commands or write/edit files." + profileGuidance + profileDescription,
			Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"label":{"type":"string"},"prompt":{"type":"string"},"workspace":{"type":"string"},"providerId":{"type":"string"},"provider_id":{"type":"string"},"model":{"type":"string"},"endpointFormat":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"endpoint_format":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"variant":{"type":"string"},"modelVariant":{"type":"string"},"model_variant":{"type":"string"},"effort":{"type":"string","enum":["off","low","medium","high","max"]},"profile":{"type":"string"},"toolPolicy":{"type":"string","enum":["readOnly","inherit"]},"tool_policy":{"type":"string","enum":["readOnly","inherit"]},"tools":{"type":"array","items":{"type":"string"}},"blockedTools":{"type":"array","items":{"type":"string"}},"blockedMcpServers":{"type":"array","items":{"type":"string"}},"blockedSkills":{"type":"array","items":{"type":"string"}},"max_steps":{"type":"integer"},"maxSteps":{"type":"integer"},"tokenBudget":{"type":"integer"},"token_budget":{"type":"integer"},"timeBudgetMs":{"type":"integer"},"time_budget_ms":{"type":"integer"},"returnFormat":{"type":"string","enum":["summary","evidence","transcriptRef"]},"return_format":{"type":"string","enum":["summary","evidence","transcriptRef","transcript_ref"]},"run_in_background":{"type":"boolean"},"runInBackground":{"type":"boolean"},"auto_continue_parent":{"type":"boolean"},"autoContinueParent":{"type":"boolean"},"task_id":{"type":"string"},"taskId":{"type":"string"},"continue_from":{"type":"string"},"continueFrom":{"type":"string"},"fork_from":{"type":"string"},"forkFrom":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`),
			Source:      "subagent",
		},
	}
}

func JobToolSchemas() []domainmodel.ToolSchema {
	return []domainmodel.ToolSchema{
		{
			Name:        "wait",
			Description: "Wait for one or more background sub-agent jobs owned by the current parent thread.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string"},"jobId":{"type":"string"},"job_ids":{"type":"array","items":{"type":"string"}},"jobIds":{"type":"array","items":{"type":"string"}},"timeout_seconds":{"type":"integer"},"timeoutSeconds":{"type":"integer"},"timeout_ms":{"type":"integer"},"timeoutMs":{"type":"integer"}},"additionalProperties":false}`),
			Source:      "job",
		},
		{
			Name:        "bash_output",
			Description: "Read new output from a background job owned by the current parent thread. Without offset/cursor it returns output produced since the last bash_output call for that job.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string"},"jobId":{"type":"string"},"offset":{"type":"integer"},"cursor":{"type":"integer"},"limit":{"type":"integer"},"filter":{"type":"string"},"tail":{"type":"boolean"},"since":{"type":"string"}},"additionalProperties":false}`),
			Source:      "job",
		},
		{
			Name:        "list_jobs",
			Description: "List background sub-agent and background shell jobs owned by the current parent thread, including heartbeat and stale diagnostics.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["queued","running","pause_requested","paused","resume_requested","resuming","completed","failed","aborted","killed","interrupted","canceled","timeout"]},"background":{"type":"boolean"},"limit":{"type":"integer"}},"additionalProperties":false}`),
			Source:      "job",
		},
		{
			Name:        "kill_shell",
			Description: "Cancel a background sub-agent job owned by the current parent thread.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string"},"jobId":{"type":"string"},"reason":{"type":"string"}},"additionalProperties":false}`),
			Source:      "job",
		},
		{
			Name:        "restart_job",
			Description: "Restart a terminal sub-agent job owned by the current parent thread using its persisted prompt, model, tool policy, and workspace. Use this after a failed, killed, canceled, or timed-out child job when the same task should be retried.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string"},"jobId":{"type":"string"},"id":{"type":"string"}},"additionalProperties":false}`),
			Source:      "job",
		},
	}
}

func RunSkillToolSchema() domainmodel.ToolSchema {
	return domainmodel.ToolSchema{
		Name:        "run_skill",
		Description: "Invoke a configured Analytix skill. Skills marked runAs=subagent run through the durable child-agent runtime; inline skills return their instructions as a tool result.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"skill_id":{"type":"string"},"arguments":{"type":"string"},"task":{"type":"string"},"continue_from":{"type":"string"},"fork_from":{"type":"string"},"providerId":{"type":"string"},"provider_id":{"type":"string"},"model":{"type":"string"},"endpointFormat":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"endpoint_format":{"type":"string","enum":["chat_completions","responses","messages","custom_endpoint"]},"variant":{"type":"string"},"modelVariant":{"type":"string"},"model_variant":{"type":"string"},"effort":{"type":"string","enum":["off","low","medium","high","max"]}},"required":["name"],"additionalProperties":false}`),
		Source:      "skill",
	}
}
