package subagent

import (
	"context"
	"encoding/json"
	"strings"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type ParentContinuationRecordReaderV1 interface {
	AllRecords() []domainjob.Record
}

type ParentContinuationRehydratorV1 interface {
	Rehydrate(context.Context, domainjob.Record) (VerifiedChildCompletion, error)
}

type ParentContinuationSkillResolverV1 func(string) (map[string]any, bool)

// ResolveParentContinuationAuthorityV1 keeps continuation authority in the
// application owner. A foreground case lane can consume only the process-local
// capability in this exact settlement output; ordinary durable child receipts
// retain their existing rehydration path.
func ResolveParentContinuationAuthorityV1(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	output any,
	records ParentContinuationRecordReaderV1,
	completions ParentContinuationRehydratorV1,
	resolveSkill ParentContinuationSkillResolverV1,
) apploop.ParentContinuationAuthority {
	if IsForegroundHandoffPendingToolCallV1(pending) {
		projection, _ := output.(map[string]any)
		capability, _ := projection[ForegroundParentCapabilityFieldV1].(*ForegroundParentResultCapabilityV1)
		if capability == nil {
			return apploop.NewParentContinuationAuthority(1, nil)
		}
		return apploop.NewParentContinuationAuthority(1, []apploop.ParentContinuationCapability{capability})
	}
	expected, required := childContinuationRequirementV1(pending, resolveSkill)
	if !required {
		return apploop.NoParentContinuationAuthority()
	}
	capabilities := []apploop.ParentContinuationCapability{}
	if records != nil && completions != nil {
		for _, record := range records.AllRecords() {
			if strings.TrimSpace(record.Kind) != "subagent" || record.ParentThreadID != pending.ThreadID ||
				record.ParentTurnID != pending.TurnID || record.ParentToolCallID != pending.Call.ID ||
				record.Status != string(domainjob.StatusCompleted) || record.ChildCompletionReceipt == nil || record.SecurityBinding == nil ||
				record.SecurityBinding.ParentExecutionGrantID != pending.ExecutionGrant.GrantID ||
				!domainjob.SecurityBindingMatchesContext(record.SecurityBinding, pending.SecurityContext) {
				continue
			}
			verified, err := completions.Rehydrate(ctx, record)
			if err == nil {
				capabilities = append(capabilities, verified)
			}
		}
	}
	return apploop.NewParentContinuationAuthority(expected, capabilities)
}

func childContinuationRequirementV1(
	pending appmodel.PendingToolCall,
	resolveSkill ParentContinuationSkillResolverV1,
) (int, bool) {
	switch strings.TrimSpace(pending.Call.Name) {
	case "task", "delegate_task":
		return 1, true
	case "parallel_tasks":
		var args map[string]any
		if json.Unmarshal(pending.Call.Arguments, &args) != nil {
			return 0, true
		}
		tasks, err := ParallelTaskRequestsFromArgs(args)
		if err != nil {
			return 0, true
		}
		return len(tasks), true
	case "run_skill":
		var args map[string]any
		if json.Unmarshal(pending.Call.Arguments, &args) != nil || resolveSkill == nil {
			return 0, true
		}
		skill, ok := resolveSkill(SkillNameFromArgs(args))
		return 1, !ok || SkillRunAs(skill) == "subagent"
	default:
		return 0, false
	}
}
