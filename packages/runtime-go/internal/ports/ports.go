package ports

import (
	"context"
	"encoding/json"
	"io"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type Clock interface {
	NowRFC3339Nano() string
}

type EventRecorder interface {
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

type EventReplayStore interface {
	EventsAfter(threadID string, afterSeq domainevent.Seq) ([]domainevent.RuntimeEvent, error)
	HighestSeq(threadID string) (domainevent.Seq, error)
}

type ProviderClient interface {
	Stream(context.Context, domainmodel.Request) (domainmodel.Result, error)
}

// DurablePipelineProviderClient marks a provider transport that emits one
// durable pre_send/post_send pair for every physical transport attempt. The
// runtime Agent loop requires this stronger contract so an implementation
// cannot return a publishable result after silently bypassing the durable
// indeterminate-send frontier.
type DurablePipelineProviderClient interface {
	ProviderClient
	RequiresDurablePipelineStagesV1()
}

type MCPManager interface {
	Tools() []domainmcp.ToolSpec
	CallToolContext(context.Context, string, map[string]any) (any, error)
}

type MCPToolAdvertisementSource interface {
	MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []domainmcp.ToolAdvertisementV1
}

type RuntimeMCPManager interface {
	MCPToolAdvertisementSource
	Connect()
	Diagnostics() map[string]any
	ServerDiagnostics() []any
	Search(query string) []string
	CallTool(toolName string, approved bool, arguments ...map[string]any) map[string]any
	RefreshCatalog() map[string]any
	RestartReconnect() map[string]any
	Disconnect()
	Tools() []string
	Prompts() []any
	Resources() []any
	ToolReadOnlyHint(toolName string) bool
	ToolInputSchema(toolName string) (json.RawMessage, bool)
	ToolOutputSchema(toolName string) (json.RawMessage, bool)
	ToolDescription(toolName string) (string, bool)
}

type JobRepository interface {
	StartChildRun(domainjob.StartRequest) (domainjob.Record, error)
	LoadChildRun(id string) (domainjob.Record, error)
	UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error)
	List(parentThreadID string) ([]domainjob.Record, error)
}

type ProcessRunner interface {
	Run(context.Context, ProcessRequest) (ProcessResult, error)
}

type ProcessRequest struct {
	Command string
	Args    []string
	Dir     string
	Env     map[string]string
}

type ProcessResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type ShellRunner interface {
	RunShell(context.Context, ShellRequest) ShellResult
}

// ProcessStartBarrier linearizes cancellation with the final operating-system
// process start. Implementations must run start while holding the same
// authority that cancellation closes, so a cancellation that wins first
// guarantees that start is never called.
type ProcessStartBarrier interface {
	StartIfActive(context.Context, func() error) error
}

type ShellRequest struct {
	Command           string
	Dir               string
	Output            io.Writer
	KillGrace         time.Duration
	StartBarrier      ProcessStartBarrier
	ProtectedReadDirs []string
}

type ShellResult struct {
	ExitCode    int
	Error       string
	StartFailed bool
	Canceled    bool
}

type CommandProbe interface {
	ProbeCommand(context.Context, CommandProbeRequest) CommandProbeResult
}

type CommandProbeRequest struct {
	Binary string
	Args   []string
}

type CommandProbeResult struct {
	Found    bool
	Stdout   string
	Stderr   string
	Error    string
	TimedOut bool
}

type WorkspaceStatusProbe interface {
	WorkspaceStatus(context.Context, string) WorkspaceStatusResult
}

type WorkspaceStatusResult struct {
	Path            string
	Exists          bool
	IsGitRepository bool
	Branch          *string
	HeadSHA         *string
	IsDirty         *bool
	FileChangeCount *int
	CheckedAt       string
}

type GitStatusProbe interface {
	PathHasStagedChanges(workspace string, relativePath string) bool
}

type WorktreeCreateRequest struct {
	ParentWorkspace string
	ParentThreadID  string
	JobID           string
}

type WorktreeAcceptRequest struct {
	ParentWorkspace string
	ParentThreadID  string
	JobID           string
	MergeRequestID  string
	ApprovalID      string
	Isolation       domainjob.WorktreeIsolation
}

type WorktreeConflictReportRequest struct {
	ParentWorkspace string
	ParentThreadID  string
	JobID           string
	ClientRequestID string
	Isolation       domainjob.WorktreeIsolation
}

type WorktreeRepairCheckRequest struct {
	ParentWorkspace    string
	ParentThreadID     string
	JobID              string
	ClientRequestID    string
	ConflictReportID   string
	RepairPatch        string
	ExpectedParentHead string
	Isolation          domainjob.WorktreeIsolation
}

type WorktreeRepairAcceptRequest struct {
	ParentWorkspace    string
	ParentThreadID     string
	JobID              string
	ClientRequestID    string
	RepairReviewID     string
	ApprovalID         string
	ExpectedParentHead string
	RepairPatch        string
	Isolation          domainjob.WorktreeIsolation
}

type WorktreeManager interface {
	CreateSubagentWorktree(context.Context, WorktreeCreateRequest) (domainjob.WorktreeIsolation, error)
	SummarizeSubagentWorktree(context.Context, domainjob.WorktreeIsolation) (domainjob.WorktreeIsolation, error)
	CleanupSubagentWorktree(context.Context, domainjob.WorktreeIsolation) (domainjob.CleanupReceipt, error)
	AcceptSubagentWorktree(context.Context, WorktreeAcceptRequest) (domainjob.WorktreeIsolation, domainjob.AcceptDecision, error)
	ReportSubagentWorktreeConflict(context.Context, WorktreeConflictReportRequest) (domainjob.WorktreeIsolation, domainjob.ConflictReport, error)
	CheckSubagentRepairPatch(context.Context, WorktreeRepairCheckRequest) (domainjob.WorktreeIsolation, domainjob.RepairPatchReview, error)
	AcceptSubagentRepairPatch(context.Context, WorktreeRepairAcceptRequest) (domainjob.WorktreeIsolation, domainjob.RepairDecision, error)
}
