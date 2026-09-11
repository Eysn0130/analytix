package goal

import (
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ChildRunStarter interface {
	StartChildRun(domainjob.StartRequest) (domainjob.Record, error)
}

type EventRecorder interface {
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

type CompletedTurnChildRunInput struct {
	Starter                 ChildRunStarter
	Events                  EventRecorder
	Goal                    map[string]any
	ThreadID                string
	TurnID                  string
	TurnNumber              int
	Model                   string
	Effort                  string
	RequestedModel          string
	TerminalReferenceKind   string
	TerminalReferenceDigest string
	SecurityContext         domainsecurity.TurnSecurityContext
	TotalTokens             int
	CacheHitRate            float64
	TopLevelRouteExposed    bool
}

const (
	TerminalReferenceAcceptedFinal = "accepted_final"
	TerminalReferenceGeneralCAS    = "general_terminal_cas"
)

func RecordCompletedTurnChildRun(input CompletedTurnChildRunInput) (domainjob.Record, bool, error) {
	if input.Goal == nil {
		return domainjob.Record{}, false, nil
	}
	if err := domainmodel.ValidateReasoningEffortV1(input.Effort); err != nil {
		return domainjob.Record{}, false, err
	}
	parentGoalID, parentGoalObjective := ParentIdentity(input.Goal, input.ThreadID)
	if domainsecurity.ValidateTurnSecurityContext(input.SecurityContext) != nil || input.SecurityContext.ThreadID != strings.TrimSpace(input.ThreadID) ||
		input.SecurityContext.TurnID != strings.TrimSpace(input.TurnID) {
		return domainjob.Record{}, false, errors.New("goal child-run security context is invalid")
	}
	toolCallEntropy := sha256.Sum256([]byte("analytix.goal-completion-tool-call/v1\x00" + input.SecurityContext.ContextDigest))
	parentToolCallID, _ := domainmodel.NewHostToolCallIDV1(toolCallEntropy[:])
	referenceKind := strings.TrimSpace(input.TerminalReferenceKind)
	if referenceKind != TerminalReferenceAcceptedFinal && referenceKind != TerminalReferenceGeneralCAS {
		return domainjob.Record{}, false, errors.New("goal child-run terminal reference kind is invalid")
	}
	referenceDigest := strings.TrimSpace(input.TerminalReferenceDigest)
	if len(referenceDigest) != 64 || referenceDigest != strings.ToLower(referenceDigest) {
		return domainjob.Record{}, false, errors.New("goal child-run terminal reference digest is invalid")
	}
	for _, char := range referenceDigest {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return domainjob.Record{}, false, errors.New("goal child-run terminal reference digest is invalid")
		}
	}
	terminalSourceRef := "analytix-terminal-authority-ref/v1/" + referenceKind + "/sha256/" + referenceDigest
	issuedAt, _ := time.Parse(time.RFC3339Nano, input.SecurityContext.IssuedAt)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: input.SecurityContext, Provider: "analytix-host", ServerIdentity: "host:builtin",
		ToolName: "record_goal_completion", ToolCallID: parentToolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte(input.SecurityContext.ContextDigest)), SchemaHash: domainsecurity.SHA256Hex([]byte("goal-child-run/v1")),
		ScopeHash: domainsecurity.SHA256Hex([]byte(input.SecurityContext.ThreadID + "\x00" + input.SecurityContext.TurnID)), ReadOnly: true,
		ApprovalState: "not_required", IssuedAt: issuedAt,
	})
	securityBinding, err := domainjob.NewSecurityBinding(input.SecurityContext, grant, parentToolCallID)
	if err != nil {
		return domainjob.Record{}, false, err
	}
	job, err := input.Starter.StartChildRun(domainjob.StartRequest{
		ParentGoalID:          parentGoalID,
		ParentGoalObjective:   parentGoalObjective,
		ParentThreadID:        strings.TrimSpace(input.ThreadID),
		ParentTurnID:          strings.TrimSpace(input.TurnID),
		ParentToolCallID:      parentToolCallID,
		SecurityBinding:       securityBinding,
		Kind:                  "child-run",
		Model:                 strings.TrimSpace(input.Model),
		Effort:                input.Effort,
		ProfileSource:         "parent-default",
		DefaultModelInherited: strings.TrimSpace(input.RequestedModel) == "",
		SourceRef:             terminalSourceRef,
	})
	if err != nil {
		return domainjob.Record{}, false, err
	}
	if _, _, err := input.Events.RecordEvent(BuildChildRunPipelineStageEvent(ChildRunPipelineStageEventInput{
		ThreadID:             input.ThreadID,
		TurnID:               input.TurnID,
		ParentGoalObjective:  parentGoalObjective,
		Record:               job,
		ChildSeq:             input.TurnNumber,
		TotalTokens:          input.TotalTokens,
		CacheHitRate:         input.CacheHitRate,
		TopLevelRouteExposed: input.TopLevelRouteExposed,
	})); err != nil {
		return domainjob.Record{}, false, err
	}
	return job, true, nil
}
