package subagent

import (
	"errors"

	modelapp "analytix.local/runtime-go/internal/app/model"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type PreparedTaskRunStartInput struct {
	Preparation   TaskRunPreparation
	Pending       modelapp.PendingToolCall
	Security      *domainjob.SecurityBinding
	ParentGoal    map[string]any
	Settings      ProfileSettings
	StartChildRun func(domainjob.StartRequest) (domainjob.Record, error)
}

// StartPreparedTaskRun turns an admitted preparation into its durable queued
// child record. The caller still owns the preparation's source lock.
func StartPreparedTaskRun(input PreparedTaskRunStartInput) (domainjob.Record, error) {
	if input.StartChildRun == nil {
		return domainjob.Record{}, errors.New("subagent child run store is unavailable")
	}
	preparation, pending := input.Preparation, input.Pending
	parentGoalID, parentGoalObjective := ParentGoalIdentity(input.ParentGoal, pending.ThreadID)
	startRequest, err := BuildChildRunStartRequest(ChildRunStartInput{
		ParentGoalID: parentGoalID, ParentGoalObjective: parentGoalObjective,
		ParentThreadID: pending.ThreadID, ParentTurnID: pending.TurnID, ParentToolItemID: pending.ToolCallItemID,
		ParentToolCallID: pending.Call.ID, SecurityBinding: input.Security, FallbackName: pending.Call.Name,
		SourceRef: preparation.Source.ID, Request: preparation.Request, Execution: preparation.Execution,
		ParentMaxModelSteps: ParentMaxModelSteps(pending.MaxModelSteps, pending.EffectiveMaxModelSteps), ToolScope: preparation.ToolScope,
		ToolSchemaHash: preparation.ToolSchemaHash, DelegatedToolManifest: preparation.DelegatedToolManifest,
		SystemPromptHash: preparation.SystemPromptHash, MaxChildRuns: MaxChildRuns(input.Settings),
	})
	if err != nil {
		return domainjob.Record{}, err
	}
	return input.StartChildRun(startRequest)
}
