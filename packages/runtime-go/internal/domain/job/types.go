package job

import domainsecurity "analytix.local/runtime-go/internal/domain/security"

type Status string

const (
	StatusQueued          Status = "queued"
	StatusRunning         Status = "running"
	StatusPauseRequested  Status = "pause_requested"
	StatusPaused          Status = "paused"
	StatusResumeRequested Status = "resume_requested"
	StatusResuming        Status = "resuming"
	StatusCompleted       Status = "completed"
	StatusFailed          Status = "failed"
	StatusAborted         Status = "aborted"
	StatusInterrupted     Status = "interrupted"
	StatusKilled          Status = "killed"
)

type IsolationMode string

const (
	IsolationNone     IsolationMode = ""
	IsolationWorktree IsolationMode = "worktree"
)

type ChangedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type WorktreeIsolation struct {
	IsolationMode  string        `json:"isolationMode,omitempty"`
	WorktreePath   string        `json:"worktreePath,omitempty"`
	WorktreeBranch string        `json:"worktreeBranch,omitempty"`
	BaseCommit     string        `json:"baseCommit,omitempty"`
	CurrentCommit  string        `json:"currentCommit,omitempty"`
	ChangedFiles   []ChangedFile `json:"changedFiles,omitempty"`
	DiffSummary    string        `json:"diffSummary,omitempty"`
	MergeStatus    string        `json:"mergeStatus,omitempty"`
}

type MergeDecision struct {
	ID             string `json:"id"`
	ParentThreadID string `json:"parentThreadId,omitempty"`
	ChildRunID     string `json:"childRunId,omitempty"`
	JobID          string `json:"jobId,omitempty"`
	Decision       string `json:"decision"`
	CreatedAt      string `json:"createdAt"`
	Reason         string `json:"reason,omitempty"`
	ApprovalID     string `json:"approvalId,omitempty"`
}

type CleanupReceipt struct {
	ID               string `json:"id"`
	AcceptDecisionID string `json:"acceptDecisionId,omitempty"`
	WorktreePath     string `json:"worktreePath,omitempty"`
	Branch           string `json:"branch,omitempty"`
	Removed          bool   `json:"removed"`
	RetainedReason   string `json:"retainedReason,omitempty"`
	CreatedAt        string `json:"createdAt"`
}

type AcceptDecision struct {
	ID                 string        `json:"id"`
	ParentThreadID     string        `json:"parentThreadId,omitempty"`
	ChildRunID         string        `json:"childRunId,omitempty"`
	JobID              string        `json:"jobId,omitempty"`
	MergeRequestID     string        `json:"mergeRequestId,omitempty"`
	ApprovalID         string        `json:"approvalId"`
	BaseCommit         string        `json:"baseCommit,omitempty"`
	ParentHeadBefore   string        `json:"parentHeadBefore,omitempty"`
	ParentHeadAfter    string        `json:"parentHeadAfter,omitempty"`
	ChangedFiles       []ChangedFile `json:"changedFiles,omitempty"`
	AppliedPatchDigest string        `json:"appliedPatchDigest,omitempty"`
	CreatedAt          string        `json:"createdAt"`
}

type ConflictReport struct {
	ID                     string        `json:"id"`
	ParentThreadID         string        `json:"parentThreadId,omitempty"`
	ChildRunID             string        `json:"childRunId,omitempty"`
	JobID                  string        `json:"jobId,omitempty"`
	BaseCommit             string        `json:"baseCommit,omitempty"`
	ParentHeadAtConflict   string        `json:"parentHeadAtConflict,omitempty"`
	ChildHeadAtConflict    string        `json:"childHeadAtConflict,omitempty"`
	SourcePatchDigest      string        `json:"sourcePatchDigest,omitempty"`
	ConflictFiles          []ChangedFile `json:"conflictFiles,omitempty"`
	ConflictSummary        string        `json:"conflictSummary,omitempty"`
	CreatedAt              string        `json:"createdAt"`
	TouchedParentWorkspace bool          `json:"touchedParentWorkspace"`
}

type RepairPatchReview struct {
	ID                 string        `json:"id"`
	ConflictReportID   string        `json:"conflictReportId,omitempty"`
	ParentThreadID     string        `json:"parentThreadId,omitempty"`
	ChildRunID         string        `json:"childRunId,omitempty"`
	JobID              string        `json:"jobId,omitempty"`
	RepairPatchDigest  string        `json:"repairPatchDigest,omitempty"`
	ExpectedParentHead string        `json:"expectedParentHead,omitempty"`
	ChangedFiles       []ChangedFile `json:"changedFiles,omitempty"`
	DryRunStatus       string        `json:"dryRunStatus,omitempty"`
	RejectedReason     string        `json:"rejectedReason,omitempty"`
	CreatedAt          string        `json:"createdAt"`
	RepairPatch        string        `json:"repairPatch,omitempty"`
}

type RepairDecision struct {
	ID                 string `json:"id"`
	RepairReviewID     string `json:"repairReviewId,omitempty"`
	ApprovalID         string `json:"approvalId"`
	ParentHeadBefore   string `json:"parentHeadBefore,omitempty"`
	ParentHeadAfter    string `json:"parentHeadAfter,omitempty"`
	AppliedPatchDigest string `json:"appliedPatchDigest,omitempty"`
	CreatedAt          string `json:"createdAt"`
}

type ChildTodoItem struct {
	ID               string   `json:"id"`
	Content          string   `json:"content"`
	Status           string   `json:"status"`
	StatusReasonCode string   `json:"statusReasonCode,omitempty"`
	EvidenceIDs      []string `json:"evidenceIds,omitempty"`
	ParentTodoRef    string   `json:"parentTodoRef,omitempty"`
	CreatedAt        string   `json:"createdAt,omitempty"`
	UpdatedAt        string   `json:"updatedAt,omitempty"`
}

type ChildTodoList struct {
	ID             string          `json:"id"`
	ParentThreadID string          `json:"parentThreadId"`
	ChildThreadID  string          `json:"childThreadId"`
	ChildRunID     string          `json:"childRunId"`
	JobID          string          `json:"jobId"`
	Scope          string          `json:"scope"`
	Items          []ChildTodoItem `json:"items,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
	SourceTurnID   string          `json:"sourceTurnId,omitempty"`
}

type ChildTodoProjectionItem struct {
	ID               string   `json:"id"`
	Content          string   `json:"content"`
	Status           string   `json:"status"`
	StatusReasonCode string   `json:"statusReasonCode,omitempty"`
	EvidenceIDs      []string `json:"evidenceIds,omitempty"`
	ParentTodoRef    string   `json:"parentTodoRef,omitempty"`
}

type ChildTodoProjection struct {
	ID              string                    `json:"id"`
	ParentThreadID  string                    `json:"parentThreadId"`
	ChildThreadID   string                    `json:"childThreadId"`
	ChildRunID      string                    `json:"childRunId"`
	JobID           string                    `json:"jobId"`
	ChildTodoListID string                    `json:"childTodoListId"`
	ProjectedItems  []ChildTodoProjectionItem `json:"projectedItems,omitempty"`
	Summary         string                    `json:"summary,omitempty"`
	EvidenceIDs     []string                  `json:"evidenceIds,omitempty"`
	Status          string                    `json:"status"`
	CreatedAt       string                    `json:"createdAt"`
	AcceptedAt      string                    `json:"acceptedAt,omitempty"`
	RejectedAt      string                    `json:"rejectedAt,omitempty"`
}

type AcceptedProjectionItem struct {
	ChildTodoID          string   `json:"childTodoId"`
	ParentTodoID         string   `json:"parentTodoId"`
	Action               string   `json:"action"`
	PreviousParentStatus string   `json:"previousParentStatus,omitempty"`
	NextParentStatus     string   `json:"nextParentStatus"`
	EvidenceIDs          []string `json:"evidenceIds,omitempty"`
}

type SkippedProjectionItem struct {
	ChildTodoID  string   `json:"childTodoId"`
	ParentTodoID string   `json:"parentTodoId,omitempty"`
	Reason       string   `json:"reason"`
	EvidenceIDs  []string `json:"evidenceIds,omitempty"`
}

type ProjectionDecision struct {
	ID                           string                   `json:"id"`
	ParentThreadID               string                   `json:"parentThreadId"`
	ChildThreadID                string                   `json:"childThreadId"`
	ChildRunID                   string                   `json:"childRunId"`
	JobID                        string                   `json:"jobId"`
	ProjectionID                 string                   `json:"projectionId"`
	Decision                     string                   `json:"decision"`
	ApprovalID                   string                   `json:"approvalId,omitempty"`
	AcceptedItems                []AcceptedProjectionItem `json:"acceptedItems,omitempty"`
	SkippedItems                 []SkippedProjectionItem  `json:"skippedItems,omitempty"`
	ParentTodosBeforeDigest      string                   `json:"parentTodosBeforeDigest,omitempty"`
	ParentTodosAfterDigest       string                   `json:"parentTodosAfterDigest,omitempty"`
	ExpectedParentTodosUpdatedAt string                   `json:"expectedParentTodosUpdatedAt,omitempty"`
	CreatedAt                    string                   `json:"createdAt"`
}

type Record struct {
	ID                         string                    `json:"id"`
	ChildSeq                   int                       `json:"childSeq"`
	ParentGoalID               string                    `json:"parentGoalId"`
	ParentGoalObjective        string                    `json:"parentGoalObjective,omitempty"`
	ParentThreadID             string                    `json:"parentThreadId"`
	ParentTurnID               string                    `json:"parentTurnId,omitempty"`
	ParentToolItemID           string                    `json:"parentToolItemId,omitempty"`
	ParentToolCallID           string                    `json:"parentToolCallId,omitempty"`
	SecurityBinding            *SecurityBinding          `json:"securityBinding,omitempty"`
	CaseDelegation             *CaseDelegationContextV1  `json:"caseDelegation,omitempty"`
	ChildCompletionReceipt     *ChildCompletionReceiptV1 `json:"childCompletionReceipt,omitempty"`
	ChildThreadID              string                    `json:"childThreadId,omitempty"`
	ChildTurnID                string                    `json:"childTurnId,omitempty"`
	Kind                       string                    `json:"kind"`
	Name                       string                    `json:"name,omitempty"`
	Label                      string                    `json:"label,omitempty"`
	Prompt                     string                    `json:"prompt,omitempty"`
	Status                     string                    `json:"status"`
	LineageKey                 string                    `json:"lineageKey"`
	Workspace                  string                    `json:"workspace,omitempty"`
	Model                      string                    `json:"model,omitempty"`
	ProviderID                 string                    `json:"providerId,omitempty"`
	EndpointFormat             string                    `json:"endpointFormat,omitempty"`
	Variant                    string                    `json:"variant,omitempty"`
	ModelSource                string                    `json:"modelSource,omitempty"`
	ModelExecution             map[string]any            `json:"modelExecution,omitempty"`
	Effort                     string                    `json:"effort,omitempty"`
	MaxModelSteps              *int                      `json:"maxModelSteps,omitempty"`
	ProfileName                string                    `json:"profileName,omitempty"`
	ProfileSource              string                    `json:"profileSource,omitempty"`
	ToolPolicy                 string                    `json:"toolPolicy,omitempty"`
	ToolScope                  []string                  `json:"toolScope,omitempty"`
	ToolSchemaHash             string                    `json:"toolSchemaHash,omitempty"`
	DelegatedToolManifest      *DelegatedToolManifestV1  `json:"delegatedToolManifest,omitempty"`
	SystemPromptHash           string                    `json:"systemPromptHash,omitempty"`
	SkillPackageDigest         string                    `json:"skillPackageDigest,omitempty"`
	ProfileMode                string                    `json:"profileMode,omitempty"`
	ProfileDescription         string                    `json:"profileDescription,omitempty"`
	ProfileColor               string                    `json:"profileColor,omitempty"`
	ProfileIcon                string                    `json:"profileIcon,omitempty"`
	ReturnFormat               string                    `json:"returnFormat,omitempty"`
	TokenBudget                int                       `json:"tokenBudget,omitempty"`
	TimeBudgetMs               int                       `json:"timeBudgetMs,omitempty"`
	DefaultModelInherited      bool                      `json:"defaultModelInherited"`
	ParallelGroupID            string                    `json:"parallelGroupId,omitempty"`
	ParallelIndex              int                       `json:"parallelIndex,omitempty"`
	ContinueFrom               string                    `json:"continueFrom,omitempty"`
	ForkFrom                   string                    `json:"forkFrom,omitempty"`
	SourceRef                  string                    `json:"sourceRef,omitempty"`
	Background                 bool                      `json:"background,omitempty"`
	AutoContinueParent         bool                      `json:"autoContinueParent,omitempty"`
	AutoContinueStatus         string                    `json:"autoContinueStatus,omitempty"`
	AutoContinueTurnID         string                    `json:"autoContinueTurnId,omitempty"`
	AutoContinueReason         string                    `json:"autoContinueReason,omitempty"`
	AutoContinueError          string                    `json:"autoContinueError,omitempty"`
	AutoContinueUpdatedAt      string                    `json:"autoContinueUpdatedAt,omitempty"`
	LateCompletionSuppressed   bool                      `json:"lateCompletionSuppressed,omitempty"`
	LateCompletionReason       string                    `json:"lateCompletionReason,omitempty"`
	LateCompletionUpdatedAt    string                    `json:"lateCompletionUpdatedAt,omitempty"`
	CompletionDeliveryID       string                    `json:"completionDeliveryId,omitempty"`
	CompletionDeliveryStatus   string                    `json:"completionDeliveryStatus,omitempty"`
	CompletionDeliveryItemID   string                    `json:"completionDeliveryItemId,omitempty"`
	CompletionDeliveryReason   string                    `json:"completionDeliveryReason,omitempty"`
	CompletionDeliveryError    string                    `json:"completionDeliveryError,omitempty"`
	CompletionDeliveryAt       string                    `json:"completionDeliveryAt,omitempty"`
	CompletionDeadLetterAt     string                    `json:"completionDeadLetterAt,omitempty"`
	CompletionDeliveryAttempts int                       `json:"completionDeliveryAttempts,omitempty"`
	LastHeartbeatAt            string                    `json:"lastHeartbeatAt,omitempty"`
	LeaseOwner                 string                    `json:"leaseOwner,omitempty"`
	LeaseExpiresAt             string                    `json:"leaseExpiresAt,omitempty"`
	StaleAfterMs               int64                     `json:"staleAfterMs,omitempty"`
	Orphaned                   bool                      `json:"orphaned,omitempty"`
	RecoveryStatus             string                    `json:"recoveryStatus,omitempty"`
	RecoveryAttempt            int                       `json:"recoveryAttempt,omitempty"`
	RecoveryReason             string                    `json:"recoveryReason,omitempty"`
	RecoveryUpdatedAt          string                    `json:"recoveryUpdatedAt,omitempty"`
	DeadLetterReason           string                    `json:"deadLetterReason,omitempty"`
	ArtifactPath               string                    `json:"artifactPath,omitempty"`
	IsolationMode              string                    `json:"isolationMode,omitempty"`
	WorktreePath               string                    `json:"worktreePath,omitempty"`
	WorktreeBranch             string                    `json:"worktreeBranch,omitempty"`
	BaseCommit                 string                    `json:"baseCommit,omitempty"`
	CurrentCommit              string                    `json:"currentCommit,omitempty"`
	ChangedFiles               []ChangedFile             `json:"changedFiles,omitempty"`
	DiffSummary                string                    `json:"diffSummary,omitempty"`
	MergeStatus                string                    `json:"mergeStatus,omitempty"`
	MergeDecisions             []MergeDecision           `json:"mergeDecisions,omitempty"`
	CleanupReceipts            []CleanupReceipt          `json:"cleanupReceipts,omitempty"`
	AcceptDecisions            []AcceptDecision          `json:"acceptDecisions,omitempty"`
	ConflictReports            []ConflictReport          `json:"conflictReports,omitempty"`
	RepairPatchReviews         []RepairPatchReview       `json:"repairPatchReviews,omitempty"`
	RepairDecisions            []RepairDecision          `json:"repairDecisions,omitempty"`
	ChildTodoLists             []ChildTodoList           `json:"childTodoLists,omitempty"`
	ChildTodoProjections       []ChildTodoProjection     `json:"childTodoProjections,omitempty"`
	ProjectionDecisions        []ProjectionDecision      `json:"projectionDecisions,omitempty"`
	QueuedAt                   string                    `json:"queuedAt,omitempty"`
	QueuedMs                   int                       `json:"queuedMs,omitempty"`
	StartedAt                  string                    `json:"startedAt,omitempty"`
	FinishedAt                 string                    `json:"finishedAt,omitempty"`
	UpdatedAt                  string                    `json:"updatedAt,omitempty"`
	Output                     string                    `json:"output,omitempty"`
	Error                      string                    `json:"error,omitempty"`
	FailureCode                string                    `json:"failureCode,omitempty"`
	Usage                      map[string]any            `json:"usage,omitempty"`
	ToolInvocations            int                       `json:"toolInvocations"`
	Steers                     []SteerMessage            `json:"steers,omitempty"`
	SteerState                 ChildRunState             `json:"steerState,omitempty"`
	PauseRequests              []PauseRequest            `json:"pauseRequests,omitempty"`
	PauseState                 ChildRunPauseState        `json:"pauseState,omitempty"`

	ForegroundChildHandoffReceipt *ForegroundChildHandoffReceiptV1 `json:"foregroundChildHandoffReceipt,omitempty"`
}

type StartRequest struct {
	ParentGoalID          string
	ParentGoalObjective   string
	ParentThreadID        string
	ParentTurnID          string
	ParentToolItemID      string
	ParentToolCallID      string
	SecurityBinding       *SecurityBinding
	CaseDelegation        *CaseDelegationContextV1
	ChildThreadID         string
	ChildTurnID           string
	Kind                  string
	Name                  string
	Label                 string
	Prompt                string
	Status                string
	Workspace             string
	Model                 string
	ProviderID            string
	EndpointFormat        string
	Variant               string
	ModelSource           string
	ModelExecution        map[string]any
	Effort                string
	MaxModelSteps         *int
	ProfileName           string
	ProfileSource         string
	ToolPolicy            string
	ToolScope             []string
	ToolSchemaHash        string
	DelegatedToolManifest *DelegatedToolManifestV1
	SystemPromptHash      string
	SkillPackageDigest    string
	ProfileMode           string
	ProfileDescription    string
	ProfileColor          string
	ProfileIcon           string
	ReturnFormat          string
	TokenBudget           int
	TimeBudgetMs          int
	DefaultModelInherited bool
	ParallelGroupID       string
	ParallelIndex         int
	ContinueFrom          string
	ForkFrom              string
	SourceRef             string
	Background            bool
	AutoContinueParent    bool
	IsolationMode         string
	WorktreePath          string
	WorktreeBranch        string
	BaseCommit            string
	CurrentCommit         string
	ChangedFiles          []ChangedFile
	DiffSummary           string
	MergeStatus           string
	QueuedAt              string
	LastHeartbeatAt       string
	LeaseOwner            string
	LeaseExpiresAt        string
	StaleAfterMs          int64
	Output                string
	Error                 string
	FailureCode           string
	Usage                 map[string]any
	MaxChildRuns          int
	MaxChildRunsSet       bool
	ToolInvocations       int
}

type UpdateRequest struct {
	Status                   string
	ChildThreadID            string
	ChildTurnID              string
	ChildCompletionReceipt   *ChildCompletionReceiptV1
	Workspace                string
	Output                   string
	Error                    string
	FailureCode              string
	Usage                    map[string]any
	ToolInvocations          *int
	Isolation                *WorktreeIsolation
	MergeDecision            *MergeDecision
	CleanupReceipt           *CleanupReceipt
	AcceptDecision           *AcceptDecision
	ConflictReport           *ConflictReport
	RepairPatchReview        *RepairPatchReview
	RepairDecision           *RepairDecision
	ChildTodoList            *ChildTodoList
	ChildTodoProjection      *ChildTodoProjection
	ProjectionDecision       *ProjectionDecision
	AutoContinueStatus       string
	AutoContinueTurnID       string
	AutoContinueReason       string
	AutoContinueError        string
	LateCompletionSuppressed *bool
	LateCompletionReason     string
	CompletionDeliveryID     string
	CompletionDeliveryStatus string
	CompletionDeliveryItemID string
	CompletionDeliveryReason string
	CompletionDeliveryError  string
	LastHeartbeatAt          string
	LeaseOwner               string
	LeaseExpiresAt           string
	StaleAfterMs             int64
	Orphaned                 *bool
	RecoveryStatus           string
	RecoveryReason           string
	DeadLetterReason         string

	ForegroundChildHandoffReceipt *ForegroundChildHandoffReceiptV1
}

type PauseRequest struct {
	ID                   string `json:"id"`
	ParentThreadID       string `json:"parentThreadId"`
	ChildRunID           string `json:"childRunId"`
	JobID                string `json:"jobId"`
	Status               string `json:"status"`
	RequestedAt          string `json:"requestedAt"`
	PausedAt             string `json:"pausedAt,omitempty"`
	ResumedAt            string `json:"resumedAt,omitempty"`
	RejectedReason       string `json:"rejectedReason,omitempty"`
	SourceTurnID         string `json:"sourceTurnId,omitempty"`
	ResumeToken          string `json:"resumeToken,omitempty"`
	ResumeTokenIssuedAt  string `json:"resumeTokenIssuedAt,omitempty"`
	ResumeTokenExpiresAt string `json:"resumeTokenExpiresAt,omitempty"`
}

type ResumeToken struct {
	ResumeToken    string `json:"resumeToken"`
	IssuedAt       string `json:"issuedAt"`
	ExpiresAt      string `json:"expiresAt"`
	ChildRunID     string `json:"childRunId"`
	ParentThreadID string `json:"parentThreadId"`
}

type ChildRunPauseState struct {
	PauseRequestID       string `json:"pauseRequestId,omitempty"`
	Status               string `json:"status,omitempty"`
	Paused               bool   `json:"paused,omitempty"`
	PauseCount           int    `json:"pauseCount,omitempty"`
	CanPause             bool   `json:"canPause,omitempty"`
	CanResume            bool   `json:"canResume,omitempty"`
	RequestedAt          string `json:"requestedAt,omitempty"`
	LastPausedAt         string `json:"lastPausedAt,omitempty"`
	LastResumedAt        string `json:"lastResumedAt,omitempty"`
	ResumeTokenIssuedAt  string `json:"resumeTokenIssuedAt,omitempty"`
	ResumeTokenExpiresAt string `json:"resumeTokenExpiresAt,omitempty"`
}

type SteerMessage struct {
	ID                   string                       `json:"id"`
	ParentThreadID       string                       `json:"parentThreadId"`
	ChildRunID           string                       `json:"childRunId"`
	JobID                string                       `json:"jobId"`
	Text                 string                       `json:"text,omitempty"`
	ProjectionVersion    int                          `json:"projectionVersion,omitempty"`
	ContentDigest        string                       `json:"contentDigest,omitempty"`
	ContextDigest        string                       `json:"contextDigest,omitempty"`
	AuthorityDigest      string                       `json:"authorityDigest,omitempty"`
	QueueAuthorityDigest string                       `json:"queueAuthorityDigest,omitempty"`
	Status               string                       `json:"status"`
	CreatedAt            string                       `json:"createdAt"`
	AdmittedAt           string                       `json:"admittedAt,omitempty"`
	PromotionCommitID    string                       `json:"promotionCommitId,omitempty"`
	PromotionEntryID     string                       `json:"promotionEntryId,omitempty"`
	RejectedReason       string                       `json:"rejectedReason,omitempty"`
	SourceTurnID         string                       `json:"sourceTurnId,omitempty"`
	SourceToolCallID     string                       `json:"sourceToolCallId,omitempty"`
	LogicalEffect        domainsecurity.LogicalEffect `json:"logicalEffect,omitempty"`
	OrdinaryWork         bool                         `json:"ordinaryWork,omitempty"`
}

type ChildRunState struct {
	PendingSteers  int    `json:"pendingSteers,omitempty"`
	AdmittedSteers int    `json:"admittedSteers,omitempty"`
	LastSteerAt    string `json:"lastSteerAt,omitempty"`
	SteerCount     int    `json:"steerCount,omitempty"`
	CanAcceptSteer bool   `json:"canAcceptSteer,omitempty"`
}
