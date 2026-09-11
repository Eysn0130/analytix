package httpapi

import "testing"

func TestMatchRuntimeRoutePreservesAnalytixProductionSurface(t *testing.T) {
	tests := []struct {
		path            string
		route           RuntimeRoute
		userInputPrefix string
	}{
		{path: "/health", route: RouteHealth},
		{path: "/v1/runtime/info", route: RouteRuntimeInfo},
		{path: "/v1/runtime/tools", route: RouteRuntimeTools},
		{path: "/v1/runtime/task-jobs/job_1/wait", route: RouteRuntimeTaskJobs},
		{path: "/v1/usage", route: RouteUsage},
		{path: "/v1/skills", route: RouteSkills},
		{path: "/v1/attachments", route: RouteAttachments},
		{path: "/v1/attachments/diagnostics", route: RouteAttachmentDiagnostics},
		{path: "/v1/attachments/att_1", route: RouteAttachmentPath},
		{path: "/v1/memory", route: RouteMemory},
		{path: "/v1/memory/diagnostics", route: RouteMemoryDiagnostics},
		{path: "/v1/memory/mem_1", route: RouteMemoryRecordPath},
		{path: "/v1/workspace/status", route: RouteWorkspaceStatus},
		{path: LocalDisplayImportMappingPathV1, route: RouteLocalDisplay},
		{path: LocalDisplayCleaningDiffPathV1, route: RouteLocalDisplay},
		{path: LocalDisplayDirectPreviewPathV1, route: RouteLocalDisplay},
		{path: LocalDisplayAcceptedSlotsPathV1, route: RouteLocalDisplay},
		{path: HostFundsImportStagePathV1, route: RouteLocalDisplay},
		{path: HostFundsImportConfirmPathV1, route: RouteLocalDisplay},
		{path: HostFundsImportCancelPathV1, route: RouteLocalDisplay},
		{path: HostFundsImportStatusPathV1, route: RouteLocalDisplay},
		{path: HostFundsDeterministicCleaningPathV1, route: RouteLocalDisplay},
		{path: HostFundsCleaningRevokePathV1, route: RouteLocalDisplay},
		{path: "/v1/case-projects", route: RouteCaseProjects},
		{path: "/v1/case-projects/case_1/threads", route: RouteCaseProjects},
		{path: "/v1/threads", route: RouteThreads},
		{path: "/v1/threads/thread_1", route: RouteThreadPath},
		{path: "/v1/sessions/session_1/resume-thread", route: RouteSessionPath},
		{path: "/v1/approvals/approval_1", route: RouteApprovalPath},
		{path: "/v1/user-inputs/input_1", route: RouteUserInputsPath, userInputPrefix: "/v1/user-inputs/"},
		{path: "/v1/user-input/input_1", route: RouteUserInputPath, userInputPrefix: "/v1/user-input/"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			match := MatchRuntimeRoute(tt.path)
			if !match.Found() || match.Route != tt.route || match.UserInputPrefix != tt.userInputPrefix {
				t.Fatalf("unexpected match: %#v", match)
			}
		})
	}
}

func TestMatchRuntimeRouteRejectsUpstreamProductRoutes(t *testing.T) {
	for _, path := range []string{
		"/v1/reasonix",
		"/v1/subagents",
		"/v1/workflow",
		"/v1/conformance",
		"/v1/local-display/funds-csv-admission",
	} {
		if match := MatchRuntimeRoute(path); match.Found() {
			t.Fatalf("upstream route %q should not match production surface: %#v", path, match)
		}
	}
}
