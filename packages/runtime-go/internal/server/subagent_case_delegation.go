package server

import (
	"context"
	"errors"
	"strings"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
)

func (h *runtimeServerHandler) prepareRuntimeChildCaseDelegationV1(
	ctx context.Context,
	input runtimeAgentLoopInput,
) (runtimeAgentLoopInput, error) {
	if strings.TrimSpace(input.ChildRunID) == "" {
		return input, nil
	}
	resolved, err := subagentapp.ResolveCurrentChildCaseDelegationV1(ctx, subagentapp.ResolveCurrentChildCaseDelegationInputV1{
		ChildRunID: input.ChildRunID, ChildThreadID: input.ThreadID, Prompt: input.Request.Prompt,
		SecurityContext: input.SecurityContext, Jobs: h.jobs, Blocker: h.runtimeJobSecurityAuthorizer().Blocker,
		CaseEntities: h.caseEntities,
	})
	if err != nil {
		return runtimeAgentLoopInput{}, err
	}
	if resolved.CurrentRecord.CaseDelegation != nil && input.HostChildCasePromptWitness == nil {
		return runtimeAgentLoopInput{}, errors.New("current child case prompt witness is unavailable")
	}
	input.Request.Prompt, input.HostEntitySelection = resolved.Prompt, resolved.HostSelection
	input.initialProviderAliases = resolved.Aliases
	input.CurrentChildCaseRecord = resolved.CurrentRecord
	return input, nil
}
