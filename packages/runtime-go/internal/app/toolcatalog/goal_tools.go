package toolcatalog

import (
	"encoding/json"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func GoalAndTodoToolSchemas() []domainmodel.ToolSchema {
	todoOpsParameters := json.RawMessage(`{"type":"object","properties":{"ops":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"object","properties":{"op":{"type":"string","enum":["append","start","done","fail","cancel","retry","drop","note"]},"id":{"type":"string"},"content":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]},"statusReasonCode":{"type":"string","enum":["execution_failed","dependency_failed","verification_failed","timeout","tool_failed","subagent_failed","runtime_failed","user_canceled","parent_canceled","runtime_canceled","superseded"]},"note":{"type":"string"}},"required":["op"],"additionalProperties":false}},"operations":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"object","properties":{"op":{"type":"string","enum":["append","start","done","fail","cancel","retry","drop","note"]},"id":{"type":"string"},"content":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]},"statusReasonCode":{"type":"string","enum":["execution_failed","dependency_failed","verification_failed","timeout","tool_failed","subagent_failed","runtime_failed","user_canceled","parent_canceled","runtime_canceled","superseded"]},"note":{"type":"string"}},"required":["op"],"additionalProperties":false}}},"additionalProperties":false}`)
	return []domainmodel.ToolSchema{
		{
			Name:        "get_goal",
			Description: "Get the current goal for this thread, including status, budgets, usage, and remaining token budget.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Source:      "goal",
		},
		{
			Name:        "create_goal",
			Description: "Create a goal only when explicitly requested by the user or system/developer instructions; fails if an active goal already exists.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"objective":{"type":"string"},"token_budget":{"type":"integer"},"tokenBudget":{"type":"integer"},"strict_completion":{"type":"boolean"},"strictCompletion":{"type":"boolean"}},"required":["objective"],"additionalProperties":false}`),
			Source:      "goal",
		},
		{
			Name:        "complete_step",
			Description: "Record an evidence-backed completion of one active-goal step. A step cannot be signed off without concrete evidence; structured evidence may cite verification commands, diff/files paths, or manual checks.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"step":{"type":"string"},"step_index":{"type":"integer","minimum":1},"stepIndex":{"type":"integer","minimum":1},"result":{"type":"string"},"evidence":{"type":"array","minItems":1,"maxItems":20,"items":{"anyOf":[{"type":"string"},{"type":"object","properties":{"kind":{"type":"string","enum":["verification","diff","files","manual"]},"summary":{"type":"string"},"command":{"type":"string"},"paths":{"type":"array","items":{"type":"string"}}},"required":["kind","summary"],"additionalProperties":false}]}},"requirement_id":{"type":"string"},"requirementId":{"type":"string"},"summary":{"type":"string"},"notes":{"type":"string"},"self_check":{"type":"boolean"},"selfCheck":{"type":"boolean"}},"required":["evidence"],"additionalProperties":false}`),
			Source:      "goal",
		},
		{
			Name:        "update_goal",
			Description: "Update the existing goal only to mark it complete or blocked. Completion requires complete_step evidence and no incomplete todos; blocked requires the same reason across three goal turns.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["complete","blocked"]},"reason":{"type":"string"}},"required":["status"],"additionalProperties":false}`),
			Source:      "goal",
		},
		{
			Name:        "todo_list",
			Description: "Return the current thread todo list. Use this to inspect structured progress state.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Source:      "todo",
		},
		{
			Name:        "todo_write",
			Description: "Replace the current thread todo list after strict lifecycle validation. At most one item may be in_progress; failed/canceled audit records cannot be created, retried, or deleted through replacement.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"todos":{"type":"array","maxItems":200,"items":{"type":"object","properties":{"id":{"type":"string"},"content":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed","failed","canceled"]},"statusReasonCode":{"type":"string","enum":["execution_failed","dependency_failed","verification_failed","timeout","tool_failed","subagent_failed","runtime_failed","user_canceled","parent_canceled","runtime_canceled","superseded"]},"note":{"type":"string"}},"required":["content","status"],"additionalProperties":false}}},"required":["todos"],"additionalProperties":false}`),
			Source:      "todo",
		},
		{
			Name:        "todo_ops",
			Description: "Apply ordered todo operations to the current thread todo list. Supports append/start/done/fail/cancel/retry/drop/note; fail/cancel require a fixed statusReasonCode and retry is the only way to reopen failed/canceled work.",
			Parameters:  todoOpsParameters,
			Source:      "todo",
		},
		{
			Name:        "todo_patch",
			Description: "Alias of todo_ops for applying ordered append/start/done/fail/cancel/retry/drop/note operations to the current thread todo list.",
			Parameters:  todoOpsParameters,
			Source:      "todo",
		},
	}
}
