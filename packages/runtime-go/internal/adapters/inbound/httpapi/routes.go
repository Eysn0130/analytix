package httpapi

import "strings"

type RuntimeRoute string

const (
	toolExecutionObservationPathV1       = "/v1/runtime/tool-executions/observe"
	LocalDisplayImportMappingPathV1      = "/v1/local-display/import-mapping-preview"
	LocalDisplayCleaningDiffPathV1       = "/v1/local-display/cleaning-diff-preview"
	LocalDisplayDirectPreviewPathV1      = "/v1/local-display/direct-source-preview"
	LocalDisplayAcceptedSlotsPathV1      = "/v1/local-display/accepted-slot-display"
	HostFundsImportStagePathV1           = "/v1/local-display/funds-import/stage"
	HostFundsImportConfirmPathV1         = "/v1/local-display/funds-import/confirm"
	HostFundsImportCancelPathV1          = "/v1/local-display/funds-import/cancel"
	HostFundsImportStatusPathV1          = "/v1/local-display/funds-import/status"
	HostFundsDeterministicCleaningPathV1 = "/v1/local-display/funds-cleaning/run"
	HostFundsCleaningRevokePathV1        = "/v1/local-display/funds-cleaning/revoke"
	LocalDisplayHeaderV1                 = "X-Analytix-Local-Display"
	LocalDisplayHeaderValueV1            = "typed-v1"

	RouteUnknown               RuntimeRoute = ""
	RouteHealth                RuntimeRoute = "health"
	RouteRuntimeInfo           RuntimeRoute = "runtime_info"
	RouteRuntimeTools          RuntimeRoute = "runtime_tools"
	RouteToolExecutionObserve  RuntimeRoute = "tool_execution_observe"
	RouteRuntimeTaskJobs       RuntimeRoute = "runtime_task_jobs"
	RouteUsage                 RuntimeRoute = "usage"
	RouteSkills                RuntimeRoute = "skills"
	RouteAttachments           RuntimeRoute = "attachments"
	RouteAttachmentDiagnostics RuntimeRoute = "attachment_diagnostics"
	RouteAttachmentPath        RuntimeRoute = "attachment_path"
	RouteMemory                RuntimeRoute = "memory"
	RouteMemoryDiagnostics     RuntimeRoute = "memory_diagnostics"
	RouteMemoryRecordPath      RuntimeRoute = "memory_record_path"
	RouteWorkspaceStatus       RuntimeRoute = "workspace_status"
	RouteProviderRegistry      RuntimeRoute = "provider_registry"
	RouteMediaExecution        RuntimeRoute = "media_execution"
	RouteLocalDisplay          RuntimeRoute = "local_display"
	RouteCaseProjects          RuntimeRoute = "case_projects"
	RouteThreads               RuntimeRoute = "threads"
	RouteThreadPath            RuntimeRoute = "thread_path"
	RouteSessionPath           RuntimeRoute = "session_path"
	RouteApprovalPath          RuntimeRoute = "approval_path"
	RouteUserInputsPath        RuntimeRoute = "user_inputs_path"
	RouteUserInputPath         RuntimeRoute = "user_input_path"
)

type RuntimeRouteMatch struct {
	Route           RuntimeRoute
	UserInputPrefix string
}

func (m RuntimeRouteMatch) Found() bool {
	return m.Route != RouteUnknown
}

func MatchRuntimeRoute(path string) RuntimeRouteMatch {
	switch {
	case path == "/health":
		return RuntimeRouteMatch{Route: RouteHealth}
	case path == "/v1/runtime/info":
		return RuntimeRouteMatch{Route: RouteRuntimeInfo}
	case path == "/v1/runtime/tools":
		return RuntimeRouteMatch{Route: RouteRuntimeTools}
	case path == toolExecutionObservationPathV1:
		return RuntimeRouteMatch{Route: RouteToolExecutionObserve}
	case strings.HasPrefix(path, "/v1/runtime/task-jobs/"):
		return RuntimeRouteMatch{Route: RouteRuntimeTaskJobs}
	case path == "/v1/usage":
		return RuntimeRouteMatch{Route: RouteUsage}
	case path == "/v1/skills":
		return RuntimeRouteMatch{Route: RouteSkills}
	case path == "/v1/attachments":
		return RuntimeRouteMatch{Route: RouteAttachments}
	case path == "/v1/attachments/diagnostics":
		return RuntimeRouteMatch{Route: RouteAttachmentDiagnostics}
	case strings.HasPrefix(path, "/v1/attachments/"):
		return RuntimeRouteMatch{Route: RouteAttachmentPath}
	case path == "/v1/memory":
		return RuntimeRouteMatch{Route: RouteMemory}
	case path == "/v1/memory/diagnostics":
		return RuntimeRouteMatch{Route: RouteMemoryDiagnostics}
	case strings.HasPrefix(path, "/v1/memory/"):
		return RuntimeRouteMatch{Route: RouteMemoryRecordPath}
	case path == "/v1/workspace/status":
		return RuntimeRouteMatch{Route: RouteWorkspaceStatus}
	case path == ProviderRegistryPathV1 || strings.HasPrefix(path, ProviderRegistryPathV1+"/"):
		return RuntimeRouteMatch{Route: RouteProviderRegistry}
	case path == MediaExecutionPathV1:
		return RuntimeRouteMatch{Route: RouteMediaExecution}
	case path == LocalDisplayImportMappingPathV1 || path == LocalDisplayCleaningDiffPathV1 ||
		path == LocalDisplayDirectPreviewPathV1 || path == LocalDisplayAcceptedSlotsPathV1 ||
		path == HostFundsImportStagePathV1 || path == HostFundsImportConfirmPathV1 ||
		path == HostFundsImportCancelPathV1 || path == HostFundsImportStatusPathV1 ||
		path == HostFundsDeterministicCleaningPathV1 || path == HostFundsCleaningRevokePathV1 || path == ObjectEditingPath || path == BrowserSelectionPath || path == WorkspaceReadPath || path == InlineCompletionPath || path == PluginPackageHostPath || path == GeneratedArtifactPath || path == OfficePrivateAdmissionPath:
		return RuntimeRouteMatch{Route: RouteLocalDisplay}
	case path == "/v1/case-projects" || strings.HasPrefix(path, "/v1/case-projects/"):
		return RuntimeRouteMatch{Route: RouteCaseProjects}
	case path == "/v1/threads":
		return RuntimeRouteMatch{Route: RouteThreads}
	case strings.HasPrefix(path, "/v1/threads/"):
		return RuntimeRouteMatch{Route: RouteThreadPath}
	case strings.HasPrefix(path, "/v1/sessions/"):
		return RuntimeRouteMatch{Route: RouteSessionPath}
	case strings.HasPrefix(path, "/v1/approvals/"):
		return RuntimeRouteMatch{Route: RouteApprovalPath}
	case strings.HasPrefix(path, "/v1/user-inputs/"):
		return RuntimeRouteMatch{Route: RouteUserInputsPath, UserInputPrefix: "/v1/user-inputs/"}
	case strings.HasPrefix(path, "/v1/user-input/"):
		return RuntimeRouteMatch{Route: RouteUserInputPath, UserInputPrefix: "/v1/user-input/"}
	default:
		return RuntimeRouteMatch{}
	}
}
