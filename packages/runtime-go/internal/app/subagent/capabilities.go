package subagent

import (
	"strings"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func CapabilityState(settings ProfileSettings, durableChildRunStore bool, toolNames []string) map[string]any {
	state := runtimeinfoapp.SubagentCapabilityState(durableChildRunStore, toolNames)
	if !settings.Enabled {
		state["status"] = "disabled"
		state["enabled"] = false
		state["available"] = false
		state["reason"] = "subagents are disabled in Analytix runtime settings"
		state["parallelExecutionAvailable"] = false
		state["taskToolAvailable"] = false
		state["parallelTasksToolAvailable"] = false
		state["backgroundTaskJobsAvailable"] = false
		state["backgroundSubagentJobsAvailable"] = false
		state["taskJobThreadScopeSupported"] = false
		state["modelJobToolsAvailable"] = false
		state["backgroundShellAvailable"] = false
	}
	state["defaultToolPolicy"] = firstNonEmptyAnyString(settings.DefaultToolPolicy, "readOnly")
	state["maxParallel"] = float64(MaxParallel(settings))
	state["maxChildRuns"] = float64(MaxChildRuns(settings))
	if strings.TrimSpace(settings.DefaultProfile) != "" {
		state["defaultProfile"] = settings.DefaultProfile
	}
	state["profiles"] = ProfileDiagnostics(settings)
	state["profilesAvailable"] = durableChildRunStore && len(settings.Profiles) > 0
	return state
}

func ToolDiagnostics(settings ProfileSettings, durableChildRunStore bool, toolNames []string, active int, queued int, childRuns []any) map[string]any {
	state := CapabilityState(settings, durableChildRunStore, toolNames)
	state["active"] = float64(active)
	state["queued"] = float64(queued)
	if childRuns == nil {
		childRuns = []any{}
	}
	state["childRuns"] = childRuns
	return state
}

func ScopedToolDiagnostics(settings ProfileSettings, durableChildRunStore bool, toolNames []string, records []domainjob.Record) map[string]any {
	active := 0
	queued := 0
	for _, record := range records {
		switch normalizedChildStatus(record.Status) {
		case string(domainjob.StatusQueued):
			queued++
		case string(domainjob.StatusRunning), string(domainjob.StatusPauseRequested), string(domainjob.StatusPaused),
			string(domainjob.StatusResumeRequested), string(domainjob.StatusResuming):
			active++
		}
	}
	return ToolDiagnostics(settings, durableChildRunStore, toolNames, active, queued, ChildOutputMetadataRecordsAny(records))
}

func MaxParallel(settings ProfileSettings) int {
	if settings.MaxParallel < 1 {
		return 1
	}
	return settings.MaxParallel
}

func MaxChildRuns(settings ProfileSettings) int {
	if settings.MaxChildRuns < 0 {
		return 0
	}
	return settings.MaxChildRuns
}
