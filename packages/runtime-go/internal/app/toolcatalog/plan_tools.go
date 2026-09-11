package toolcatalog

import (
	"encoding/json"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func CreatePlanToolSchema() domainmodel.ToolSchema {
	return domainmodel.ToolSchema{
		Name:        ToolCreatePlanName,
		Description: "Create or replace a GUI-owned implementation plan. Available only during Plan-mode turns or an active GUI plan context; writes Markdown under .analytixsdd/plan and returns structured metadata.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"markdown":{"type":"string","description":"Complete Markdown plan content to save."},"source_request":{"type":"string","description":"Original user request that this plan answers."},"title":{"type":"string","description":"Short display title for the plan."},"operation":{"type":"string","enum":["draft","refine"],"description":"Use draft for a new plan, refine when revising an existing one."},"plan_id":{"type":"string","description":"Optional reserved plan id; when supplied, must match the GUI plan context."},"plan_relative_path":{"type":"string","description":"Optional reserved relative path; must live directly under .analytixsdd/plan."}},"required":["markdown","operation"],"additionalProperties":false}`),
		Source:      "plan",
	}
}
