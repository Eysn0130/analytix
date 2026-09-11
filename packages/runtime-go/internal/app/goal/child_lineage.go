package goal

import (
	"strings"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type ChildRunPipelineStageEventInput struct {
	ThreadID             string
	TurnID               string
	ParentGoalObjective  string
	Record               domainjob.Record
	ChildSeq             int
	TotalTokens          int
	CacheHitRate         float64
	TopLevelRouteExposed bool
}

func ParentIdentity(goal map[string]any, threadID string) (string, string) {
	parentGoalID := strings.TrimSpace(stringField(goal, "id"))
	if parentGoalID == "" {
		parentGoalID = "goal_" + strings.TrimSpace(threadID)
	}
	return parentGoalID, strings.TrimSpace(stringField(goal, "objective"))
}

func BuildChildRunPipelineStageEvent(input ChildRunPipelineStageEventInput) map[string]any {
	record := input.Record
	if subagentapp.SecurityBoundChildOutput(record) {
		return map[string]any{
			"kind": "pipeline_stage", "threadId": strings.TrimSpace(input.ThreadID), "turnId": strings.TrimSpace(input.TurnID),
			"stage": "response_received", "label": "internal goal child-run lineage", "child": subagentapp.SecurityBoundChildOutputProjection(record),
		}
	}
	effort, _ := domainmodel.ProjectReasoningEffortV1(record.Effort)
	return map[string]any{
		"kind":     "pipeline_stage",
		"threadId": strings.TrimSpace(input.ThreadID),
		"turnId":   strings.TrimSpace(input.TurnID),
		"stage":    "response_received",
		"label":    "internal goal child-run lineage",
		"child": map[string]any{
			"parentGoalId":                 record.ParentGoalID,
			"parentGoalObjective":          strings.TrimSpace(input.ParentGoalObjective),
			"parentThreadId":               strings.TrimSpace(input.ThreadID),
			"parentTurnId":                 strings.TrimSpace(input.TurnID),
			"childId":                      record.ID,
			"childLabel":                   record.Kind,
			"childStatus":                  record.Status,
			"lineageKey":                   record.LineageKey,
			"model":                        record.Model,
			"effort":                       effort,
			"profileSource":                record.ProfileSource,
			"defaultModelInherited":        record.DefaultModelInherited,
			"artifactPath":                 record.ArtifactPath,
			"childSeq":                     input.ChildSeq,
			"prefixReused":                 true,
			"evidenceLedgered":             true,
			"totalTokens":                  input.TotalTokens,
			"cacheHitRate":                 input.CacheHitRate,
			"contract":                     "analytix-goal-child-run",
			"topLevelSubagentRouteExposed": input.TopLevelRouteExposed,
		},
	}
}
