package server

import (
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func loadRuntimeSubagentConfig(config RuntimeServerConfig) (runtimeSubagentConfig, error) {
	document, ok, err := runtimeConfigDocument(config)
	if err != nil {
		settings, _ := subagentapp.LoadProfileSettings(nil, false)
		return settings, err
	}
	return subagentapp.LoadProfileSettings(document, ok)
}

func (h *runtimeServerHandler) runtimeSubagentState() *subagentapp.RuntimeState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subagentState == nil {
		h.subagentState = subagentapp.NewRuntimeState()
	}
	return h.subagentState
}

func (h *runtimeServerHandler) runtimeSubagentService() *subagentapp.Service {
	authorizer := h.runtimeJobSecurityAuthorizer()
	return subagentapp.NewService(subagentapp.Dependencies{
		Jobs:            h.jobs,
		Events:          h.store,
		State:           h.runtimeSubagentState(),
		WorktreeManager: h.worktreeManager,
		ChildTodos:      h.store,
		AuthorizeRecord: func(parentThreadID string, record domainjob.Record) bool {
			return authorizer.Allows(parentThreadID, record)
		},
	})
}

func (h *runtimeServerHandler) runtimeJobSecurityAuthorizer() subagentapp.JobSecurityAuthorizer {
	authorizer := subagentapp.JobSecurityAuthorizer{WorkspaceHasCaseBinding: filestore.WorkspaceHasAnalytixCaseBinding}
	if h != nil && h.store != nil {
		authorizer.Threads = h.store
	}
	if h != nil && h.jobs != nil {
		authorizer.Jobs = h.jobs
	}
	return authorizer
}
