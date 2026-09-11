package subagent

import "testing"

func TestCapabilityStateAppliesSubagentRuntimeSettings(t *testing.T) {
	settings := DefaultProfileSettings()
	settings.DefaultToolPolicy = "inherit"
	settings.MaxParallel = 0
	settings.MaxChildRuns = -1
	settings.DefaultProfile = "design-reviewer"

	state := CapabilityState(settings, true, []string{"task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job", "steer_job", "pause_job", "resume_job"})
	if state["status"] != "available" || state["defaultToolPolicy"] != "inherit" {
		t.Fatalf("capability state mismatch: %#v", state)
	}
	if state["maxParallel"] != float64(1) || state["maxChildRuns"] != float64(0) {
		t.Fatalf("subagent bounds mismatch: %#v", state)
	}
	if state["defaultProfile"] != "design-reviewer" || state["profilesAvailable"] != true {
		t.Fatalf("profile diagnostics mismatch: %#v", state)
	}
}

func TestCapabilityStateMarksDisabledSettingsUnavailable(t *testing.T) {
	settings := DefaultProfileSettings()
	settings.Enabled = false

	state := CapabilityState(settings, true, []string{"task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job", "steer_job", "pause_job", "resume_job"})
	if state["status"] != "disabled" || state["available"] != false || state["backgroundSubagentJobsAvailable"] != false {
		t.Fatalf("disabled capability state mismatch: %#v", state)
	}
	if state["reason"] != "subagents are disabled in Analytix runtime settings" {
		t.Fatalf("disabled reason mismatch: %#v", state)
	}
}

func TestToolDiagnosticsIncludesRuntimeQueuesAndChildRuns(t *testing.T) {
	childRuns := []any{map[string]any{"id": "job-1"}}
	state := ToolDiagnostics(DefaultProfileSettings(), true, []string{"task", "parallel_tasks", "wait", "bash_output", "kill_shell", "restart_job", "steer_job", "pause_job", "resume_job"}, 2, 1, childRuns)
	if state["active"] != float64(2) || state["queued"] != float64(1) {
		t.Fatalf("runtime counters mismatch: %#v", state)
	}
	if got, _ := state["childRuns"].([]any); len(got) != 1 {
		t.Fatalf("child runs mismatch: %#v", state)
	}

	state = ToolDiagnostics(DefaultProfileSettings(), false, nil, 0, 0, nil)
	if got, _ := state["childRuns"].([]any); len(got) != 0 {
		t.Fatalf("nil child runs should be exposed as empty list: %#v", state)
	}
}
