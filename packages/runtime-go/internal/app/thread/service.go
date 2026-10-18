package thread

import (
	"context"
	"errors"
	"strings"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	controlapp "analytix.local/runtime-go/internal/app/control"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrCaseControlProjectionRestricted = errors.New("case goal and todo content is unavailable until trusted evidence projection succeeds")

type Repository interface {
	ListThreads(archivedOnly bool, includeArchived bool, includeSide bool, search string) ([]map[string]any, error)
	CreateThread(request map[string]any, fallbackWorkspace string) (map[string]any, error)
	GetThread(threadID string) (map[string]any, error)
	ReadThreadMutationBaseline(threadID string) (map[string]any, string, error)
	PatchThreadIfBaseline(threadID string, patch map[string]any, expectedDigest string) (map[string]any, error)
	CommitWorkspaceMutation(WorkspaceMutationCommitRequest) (map[string]any, error)
	DeleteThreadIfBaseline(threadID string, expectedDigest string) (bool, error)
	ForkThread(threadID string, request map[string]any) (map[string]any, error)
	CommitRewindMutation(RewindMutationCommitRequest) (RewindMutationCommitResult, error)
	CommitCompaction(CompactionCommitRequest) (CompactionCommitResult, error)
	GetGoal(threadID string) (map[string]any, error)
	SetGoal(threadID string, patch map[string]any) (map[string]any, error)
	ClearGoal(threadID string) (bool, error)
	GetTodos(threadID string) (map[string]any, error)
	SetTodos(threadID string, items []any) (map[string]any, error)
	ClearTodos(threadID string) (bool, error)
	HighestSeq(threadID string) (int, error)
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

// The durable owner keeps projection, prefix replay, identity and creation seed
// under its existing mutation mutex. Other repositories retain the legacy seam.
type ForkProjectionRepositoryV1 interface {
	ForkThreadWithProjectionV1(string, map[string]any, PublicProjector) (map[string]any, error)
}

type LimitedRepository interface {
	ListThreadsLimited(archivedOnly bool, includeArchived bool, includeSide bool, search string, limit int) ([]map[string]any, error)
}

type UsageSnapshotFunc func(threadID string) (map[string]any, error)
type PendingGateSnapshotFunc func(threadID string) ([]string, []string, error)
type QuiesceThreadTurnsFunc func(context.Context, string) error
type BeginMetadataReadFunc func(context.Context, string, string, string, string) (func(), error)
type ObservePromotionChildrenFunc func(context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error)

type Service struct {
	goalMutation             func(string, map[string]any, func() (map[string]any, error)) (map[string]any, error)
	goalActivation           func(string, map[string]any) *domaingoal.ActivationV1
	summarizeCompaction      SummarizeCompactionFunc
	projectCompactionHistory ProjectCompactionHistoryFunc
	repository               Repository
	dataDir                  string
	defaultApprovalPolicy    string
	defaultSandboxMode       string
	usageSnapshot            UsageSnapshotFunc
	pendingGateSnapshot      PendingGateSnapshotFunc
	publicProjector          PublicProjector
	caseThreads              CaseThreadAuthority
	caseCompactionAuthority  casethreadapp.CommittedAuthority
	workspaceReader          turnsecurityapp.WorkspaceReader
	workspaceSecurity        turnsecurityapp.WorkspaceSecurityAuthority
	beginTransition          BeginCompactionTransitionFunc
	beginScopeTransition     BeginScopeTransitionFunc
	beginWorkspaceTransition BeginWorkspaceScopeTransitionFunc
	beginMetadataRead        BeginMetadataReadFunc
	beginArchiveRead         BeginMetadataReadFunc
	observePromotionChildren ObservePromotionChildrenFunc
	quiesceThreadTurns       QuiesceThreadTurnsFunc
	transitionTimeout        time.Duration
}

type Dependencies struct {
	GoalMutation             func(string, map[string]any, func() (map[string]any, error)) (map[string]any, error)
	GoalActivation           func(string, map[string]any) *domaingoal.ActivationV1
	SummarizeCompaction      SummarizeCompactionFunc
	ProjectCompactionHistory ProjectCompactionHistoryFunc
	Repository               Repository
	DataDir                  string
	DefaultApprovalPolicy    string
	DefaultSandboxMode       string
	UsageSnapshot            UsageSnapshotFunc
	PendingGateSnapshot      PendingGateSnapshotFunc
	PublicProjector          PublicProjector
	CaseThreads              CaseThreadAuthority
	WorkspaceReader          turnsecurityapp.WorkspaceReader
	WorkspaceSecurity        turnsecurityapp.WorkspaceSecurityAuthority
	BeginTransition          BeginCompactionTransitionFunc
	BeginScopeTransition     BeginScopeTransitionFunc
	BeginWorkspaceTransition BeginWorkspaceScopeTransitionFunc
	BeginMetadataRead        BeginMetadataReadFunc
	BeginArchiveRead         BeginMetadataReadFunc
	ObservePromotionChildren ObservePromotionChildrenFunc
	QuiesceThreadTurns       QuiesceThreadTurnsFunc
	TransitionTimeout        time.Duration
}

type ListInput struct {
	ArchivedOnly    bool
	IncludeArchived bool
	IncludeSide     bool
	Search          string
	Limit           int
}

func NewService(deps Dependencies) *Service {
	projector := deps.PublicProjector
	if projector == nil {
		projector = NewTrustedPublicProjector(nil)
	}
	var caseCompactionAuthority casethreadapp.CommittedAuthority
	if authority, ok := deps.CaseThreads.(casethreadapp.CommittedAuthority); ok {
		caseCompactionAuthority = authority
	}
	return &Service{
		goalMutation: deps.GoalMutation, goalActivation: deps.GoalActivation,
		summarizeCompaction:      deps.SummarizeCompaction,
		projectCompactionHistory: deps.ProjectCompactionHistory,
		repository:               deps.Repository,
		dataDir:                  deps.DataDir,
		defaultApprovalPolicy:    NormalizeApprovalPolicy(deps.DefaultApprovalPolicy),
		defaultSandboxMode:       NormalizeSandboxMode(deps.DefaultSandboxMode),
		usageSnapshot:            deps.UsageSnapshot,
		pendingGateSnapshot:      deps.PendingGateSnapshot,
		publicProjector:          projector,
		caseThreads:              deps.CaseThreads,
		caseCompactionAuthority:  caseCompactionAuthority,
		workspaceReader:          deps.WorkspaceReader,
		workspaceSecurity:        deps.WorkspaceSecurity,
		beginTransition:          deps.BeginTransition,
		beginScopeTransition:     deps.BeginScopeTransition,
		beginWorkspaceTransition: deps.BeginWorkspaceTransition,
		beginMetadataRead:        deps.BeginMetadataRead,
		beginArchiveRead:         deps.BeginArchiveRead,
		observePromotionChildren: deps.ObservePromotionChildren,
		quiesceThreadTurns:       deps.QuiesceThreadTurns,
		transitionTimeout:        deps.TransitionTimeout,
	}
}

func (s *Service) List(input ListInput) ([]map[string]any, error) {
	indexed, err := s.repository.ListThreads(input.ArchivedOnly, input.IncludeArchived, input.IncludeSide, "")
	if err != nil {
		return nil, err
	}
	threads := make([]map[string]any, 0, len(indexed))
	for _, candidate := range indexed {
		threadID := strings.TrimSpace(contracts.StringField(candidate, "id"))
		// A qualified restart hold has no public snapshot/cursor authority.
		// Omit only that explicit scope before reads; failures for independent
		// threads still propagate instead of turning corrupt state into success.
		if s.caseThreads != nil && s.caseThreads.RestartPreservesThreadV1(threadID) {
			continue
		}
		projected, _, err := s.readPublicThreadSnapshotV1(threadID)
		if err != nil || projected == nil || contracts.StringField(projected, "id") != threadID {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("thread list projection authority is unavailable")
		}
		summary := SummaryIndexProjection(projected)
		if IncludeThreadInList(projected, summary, ListProjectionFilter{
			ArchivedOnly: input.ArchivedOnly, IncludeArchived: input.IncludeArchived, IncludeSide: input.IncludeSide, Search: input.Search,
		}) {
			threads = append(threads, summary)
		}
	}
	SortThreadSummaries(threads)
	if input.Limit > 0 && len(threads) > input.Limit {
		threads = threads[:input.Limit]
	}
	return threads, nil
}

func (s *Service) Create(request map[string]any) (map[string]any, error) {
	body := contracts.CloneMap(request)
	if NormalizeApprovalPolicy(stringField(body, "approvalPolicy")) == "" {
		body["approvalPolicy"] = s.defaultApprovalPolicyOrFallback()
	}
	if NormalizeSandboxMode(stringField(body, "sandboxMode")) == "" {
		body["sandboxMode"] = s.defaultSandboxModeOrFallback()
	}
	thread, err := s.repository.CreateThread(body, s.dataDir)
	if err != nil {
		return nil, err
	}
	_, _, _ = s.repository.RecordEvent(map[string]any{
		"kind":     "thread_created",
		"threadId": stringField(thread, "id"),
		"status":   stringField(thread, "status"),
	})
	return s.projectThread(thread)
}

func (s *Service) Get(threadID string) (map[string]any, error) {
	thread, highestSeq, err := s.readPublicThreadSnapshotV1(threadID)
	if err != nil || thread == nil {
		return thread, err
	}
	thread["latestSeq"] = float64(highestSeq)
	if s.usageSnapshot != nil {
		usage, err := s.usageSnapshot(threadID)
		if err != nil {
			return nil, err
		}
		thread["usage"] = contracts.CloneMap(usage)
	}
	for key, value := range contracts.ThreadSummary(thread) {
		if key == "turns" {
			continue
		}
		thread[key] = value
	}
	pendingApprovals, pendingInputs := PendingGateIDs(thread)
	if s.pendingGateSnapshot != nil {
		approvals, inputs, err := s.pendingGateSnapshot(threadID)
		if err != nil {
			return nil, err
		}
		pendingApprovals = approvals
		pendingInputs = inputs
	}
	thread["pendingApprovalIds"] = pendingApprovals
	thread["pendingUserInputIds"] = pendingInputs
	return thread, nil
}

func (s *Service) Patch(ctx context.Context, threadID string, patch map[string]any) (map[string]any, error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		if patch["promoteSide"] == true {
			return nil, ErrSidePromotionUnavailable
		}
		return nil, err
	}
	if err := ValidatePublicPatch(patch); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, ErrThreadMutationTransition
	}
	initial, _, err := s.repository.ReadThreadMutationBaseline(threadID)
	if err != nil {
		if patch["promoteSide"] == true {
			return nil, ErrSidePromotionUnavailable
		}
		return nil, err
	}
	if initial == nil {
		return nil, ErrThreadNotFound
	}
	_, workspacePresent := patch["workspace"]
	requestedWorkspace := strings.TrimSpace(stringField(map[string]any{"workspace": patch["workspace"]}, "workspace"))
	changesWorkspace := workspacePresent && requestedWorkspace != strings.TrimSpace(stringField(initial, "workspace"))
	var thread map[string]any
	if patch["promoteSide"] == true {
		thread, err = s.promoteSideUnderWriter(ctx, threadID, initial)
	} else if changesWorkspace {
		thread, err = s.patchWorkspace(ctx, threadID, requestedWorkspace, patch, initial)
	} else {
		thread, err = s.patchUnderWriter(ctx, threadID, patch, initial)
	}
	if err != nil {
		return nil, err
	}
	_, _, _ = s.repository.RecordEvent(map[string]any{
		"kind":     "thread_updated",
		"threadId": threadID,
		"status":   stringField(thread, "status"),
	})
	projected, err := s.projectThread(thread)
	if err != nil && patch["promoteSide"] == true {
		return nil, ErrSidePromotionUnavailable
	}
	return projected, err
}

func (s *Service) Delete(ctx context.Context, threadID string) (bool, error) {
	transition, thread, digest, principal, err := s.beginMutationWriter(ctx, threadID, nil, true)
	if err != nil {
		return false, err
	}
	defer transition.Abort()
	if ThreadIsCaseBound(s.caseThreads, threadID, thread) {
		return false, ErrCaseDeleteSignedTombstone
	}
	timeout := s.mutationTimeout()
	current, err := FreezeThreadMutationScope(thread, threadID, s.workspaceReader, principal, time.Now().UTC(), s.workspaceSecurity)
	if err != nil {
		return false, err
	}
	if err := transition.Prepare(ctx, MutationInvalidationTarget(current, "delete", time.Now().UTC()), timeout); err != nil {
		return false, err
	}
	deleted, err := s.repository.DeleteThreadIfBaseline(threadID, digest)
	if err != nil {
		return false, err
	}
	if deleted {
		_, _, _ = s.repository.RecordEvent(map[string]any{
			"kind":     "thread_updated",
			"threadId": threadID,
			"status":   "deleted",
		})
	}
	return deleted, nil
}

func (s *Service) Fork(threadID string, request map[string]any) (map[string]any, error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		return nil, err
	}
	if _, err := ParseForkBoundaryV1(request); err != nil {
		return nil, err
	}
	var thread map[string]any
	var err error
	if owner, ok := s.repository.(ForkProjectionRepositoryV1); ok {
		thread, err = owner.ForkThreadWithProjectionV1(threadID, contracts.CloneMap(request), s.publicProjector)
	} else {
		if stringField(request, "turnId") != "" {
			return nil, ErrForkBoundaryUnavailable
		}
		thread, err = s.repository.ForkThread(threadID, contracts.CloneMap(request))
		if err == nil {
			_, _, err = s.repository.RecordEvent(map[string]any{"kind": "thread_created", "threadId": stringField(thread, "id"), "status": stringField(thread, "status")})
		}
	}
	if err != nil {
		return nil, err
	}
	return s.projectThread(thread)
}

func (s *Service) Rewind(ctx context.Context, threadID string, turnID string) (map[string]any, error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		return nil, err
	}
	preflight, err := s.repository.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if preflight == nil {
		return nil, ErrThreadNotFound
	}
	if err := ValidateRewindAcceptedFinalProtection(preflight, turnID); err != nil {
		return nil, err
	}
	transition, thread, digest, principal, err := s.beginMutationWriter(ctx, threadID, nil, true)
	if err != nil {
		return nil, err
	}
	defer transition.Abort()
	if ThreadIsCaseBound(s.caseThreads, threadID, thread) {
		return nil, ErrCaseRewindSignedArchive
	}
	current, err := FreezeThreadMutationScope(thread, threadID, s.workspaceReader, principal, time.Now().UTC(), s.workspaceSecurity)
	if err != nil {
		return nil, err
	}
	prepared, err := PrepareRewindMutation(thread, threadID, strings.TrimSpace(turnID), current, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err := transition.Prepare(ctx, prepared.SecurityContext, s.mutationTimeout()); err != nil {
		return nil, err
	}
	committed, err := s.repository.CommitRewindMutation(RewindMutationCommitRequest{
		ThreadID: threadID, RewindTurnID: strings.TrimSpace(turnID), Stamp: prepared.Stamp,
		ExpectedBaselineDigest: digest, ExpectedContextDigest: prepared.SecurityContext.ContextDigest, CurrentContext: current,
	})
	if err != nil {
		if committed.Committed {
			if commitErr := transition.Commit(); commitErr != nil {
				return nil, errors.Join(err, commitErr)
			}
		}
		return nil, err
	}
	if !committed.Committed || committed.SecurityContext != prepared.SecurityContext || committed.EpochState.StateDigest != prepared.EpochState.StateDigest {
		return nil, errors.New("durable rewind readback does not match the prepared authority")
	}
	if err := transition.Commit(); err != nil {
		return nil, err
	}
	response := contracts.CloneMap(committed.Response)
	_, _, _ = s.repository.RecordEvent(map[string]any{
		"kind":           "thread_rewound",
		"threadId":       threadID,
		"rewindTurnId":   turnID,
		"removedTurns":   response["removedTurns"],
		"remainingTurns": response["remainingTurns"],
		"removedTurnIds": response["removedTurnIds"],
	})
	return response, nil
}

func (s *Service) GetGoal(threadID string) (map[string]any, error) {
	if restricted, err := s.caseControlRestricted(threadID); err != nil || restricted {
		return nil, err
	}
	goal, err := s.repository.GetGoal(threadID)
	if err != nil || goal == nil {
		return goal, err
	}
	return contracts.CloneMap(goal), nil
}

func (s *Service) SetGoal(threadID string, patch map[string]any) (map[string]any, error) {
	if restricted, err := s.caseControlRestricted(threadID); err != nil || restricted {
		if err != nil {
			return nil, err
		}
		return nil, ErrCaseControlProjectionRestricted
	}
	commit := func() (map[string]any, error) { return s.repository.SetGoal(threadID, contracts.CloneMap(patch)) }
	var goal map[string]any
	var err error
	if s.goalMutation != nil {
		goal, err = s.goalMutation(threadID, patch, commit)
	} else {
		goal, err = commit()
	}
	if err != nil {
		return nil, err
	}
	_, _, _ = s.repository.RecordEvent(map[string]any{
		"kind":     "goal_updated",
		"threadId": threadID,
		"goal":     goal,
	})
	return contracts.CloneMap(goal), nil
}

func (s *Service) GetGoalStateV1(threadID string) (map[string]any, error) {
	thread, _, err := s.readPublicThreadSnapshotV1(threadID)
	if err != nil {
		return nil, err
	}
	goal, _ := thread["goal"].(map[string]any)
	history, _ := thread["goalHistory"].([]any)
	if history == nil {
		history = []any{}
	}
	var activation *domaingoal.ActivationV1
	if s.goalActivation != nil {
		activation = s.goalActivation(threadID, goal)
	}
	return map[string]any{"goal": goal, "activation": activation, "history": history}, nil
}

func (s *Service) ClearGoal(threadID string) (bool, error) {
	if restricted, err := s.caseControlRestricted(threadID); err != nil || restricted {
		if err != nil {
			return false, err
		}
		return false, ErrCaseControlProjectionRestricted
	}
	cleared, err := s.repository.ClearGoal(threadID)
	if err != nil {
		return false, err
	}
	if cleared {
		_, _, _ = s.repository.RecordEvent(map[string]any{
			"kind":     "goal_cleared",
			"threadId": threadID,
			"cleared":  true,
		})
	}
	return cleared, nil
}

func (s *Service) GetTodos(threadID string) (map[string]any, error) {
	restricted, err := s.caseControlRestricted(threadID)
	if err != nil {
		return nil, err
	}
	todos, err := s.repository.GetTodos(threadID)
	if err != nil || todos == nil {
		return todos, err
	}
	if restricted {
		thread, threadErr := s.repository.GetThread(strings.TrimSpace(threadID))
		if threadErr != nil || thread == nil {
			return nil, threadErr
		}
		return CaseTerminalTodoAuditProjectionV1(
			todos,
			strings.TrimSpace(threadID),
			strings.TrimSpace(stringField(thread, "updatedAt")),
		)
	}
	return contracts.CloneMap(todos), nil
}

func (s *Service) SetTodos(threadID string, items []any) (map[string]any, error) {
	if restricted, err := s.caseControlRestricted(threadID); err != nil || restricted {
		if err != nil {
			return nil, err
		}
		return nil, ErrCaseControlProjectionRestricted
	}
	current, err := s.repository.GetTodos(threadID)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeTodos(threadID, cloneList(items), "validation")
	if err != nil {
		return nil, err
	}
	if err := ValidateTodoReplacement(current, normalized); err != nil {
		return nil, err
	}
	todos, err := s.repository.SetTodos(threadID, cloneList(items))
	if err != nil {
		return nil, err
	}
	_, _, _ = s.repository.RecordEvent(map[string]any{
		"kind":     "todos_updated",
		"threadId": threadID,
		"todos":    todos,
	})
	return contracts.CloneMap(todos), nil
}

func (s *Service) ClearTodos(threadID string) (bool, error) {
	if restricted, err := s.caseControlRestricted(threadID); err != nil || restricted {
		if err != nil {
			return false, err
		}
		return false, ErrCaseControlProjectionRestricted
	}
	current, err := s.repository.GetTodos(threadID)
	if err != nil {
		return false, err
	}
	if HasRetainedTerminalTodos(current) {
		return false, ErrTodoTerminalAuditRetention
	}
	cleared, err := s.repository.ClearTodos(threadID)
	if err != nil {
		return false, err
	}
	if cleared {
		_, _, _ = s.repository.RecordEvent(map[string]any{
			"kind":     "todos_cleared",
			"threadId": threadID,
			"cleared":  true,
		})
	}
	return cleared, nil
}

func (s *Service) caseControlRestricted(threadID string) (bool, error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		return true, err
	}
	thread, err := s.repository.GetThread(strings.TrimSpace(threadID))
	if err != nil || thread == nil {
		return false, err
	}
	projected, err := s.projectThread(thread)
	if err != nil {
		return true, err
	}
	return stringField(projected, "historyAuthority") == CaseBoundaryOnlyHistoryAuthority, nil
}

func (s *Service) projectThread(thread map[string]any) (map[string]any, error) {
	return s.publicProjector.ProjectThread(thread)
}

func (s *Service) readPublicThreadSnapshotV1(threadID string) (map[string]any, int, error) {
	project := func(thread map[string]any) (map[string]any, error) {
		if strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) {
			return nil, errors.New("thread route and body identity mismatch")
		}
		return s.projectThread(thread)
	}
	if reader, ok := s.repository.(interface {
		ReadPublicThreadSnapshotV1(string, func(map[string]any) (map[string]any, error)) (map[string]any, int, error)
	}); ok {
		return reader.ReadPublicThreadSnapshotV1(threadID, project)
	}
	thread, err := s.repository.GetThread(threadID)
	if err != nil || thread == nil {
		return thread, 0, err
	}
	thread, err = project(thread)
	if err != nil {
		return nil, 0, err
	}
	seq, err := s.repository.HighestSeq(threadID)
	if err != nil {
		return nil, 0, err
	}
	return thread, seq, nil
}

func (s *Service) requireRestartWritableV1(threadID string) error {
	if s != nil && s.caseThreads != nil && s.caseThreads.RestartPreservesThreadV1(strings.TrimSpace(threadID)) {
		return casethreadapp.ErrRestartPreserved
	}
	return nil
}

func (s *Service) Compact(ctx context.Context, threadID string, reason string) (map[string]any, error) {
	return s.compact(ctx, threadID, reason, false)
}

func (s *Service) compact(ctx context.Context, threadID string, reason string, auto bool, reservations ...*controlapp.ForegroundReservation) (map[string]any, error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual"
	}
	if ctx == nil {
		return nil, errors.New("compaction request context is unavailable")
	}
	thread, err := s.repository.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, ErrThreadNotFound
	}
	caseSensitive, classificationErr := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if classificationErr != nil {
		return nil, classificationErr
	}
	if !caseSensitive && !caseThreadKnown(s.caseThreads, threadID) {
		if len(reservations) > 1 {
			return nil, errors.New("compaction reservation is ambiguous")
		}
		var reservation *controlapp.ForegroundReservation
		if len(reservations) == 1 {
			reservation = reservations[0]
		}
		return s.compactOrdinarySummary(ctx, thread, threadID, reason, auto, reservation)
	}
	prepared, caseAuthorization, err := s.prepareCompactionForCurrentThread(
		ctx, thread, threadID, reason, time.Now().UTC(), auto, nil,
	)
	if err != nil {
		return nil, err
	}
	result := prepared.Result
	if result.ReplacedTokens <= 0 {
		return map[string]any{
			"ok":                true,
			"threadId":          threadID,
			"replacedTokens":    float64(0),
			"pinnedConstraints": stringListAny(result.PinnedConstraints),
		}, nil
	}
	if err := s.validateCompactionWorkspaceAuthority(thread); err != nil {
		return nil, err
	}
	if s.beginTransition == nil {
		return nil, errors.New("compaction security transition is unavailable")
	}
	transition, err := s.beginTransition(ctx, prepared.SecurityContext)
	if err != nil || transition == nil {
		return nil, errors.Join(errors.New("compaction security transition could not start"), err)
	}
	defer transition.Abort()
	thread, err = s.repository.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, ErrThreadNotFound
	}
	if err := s.validateCompactionWorkspaceAuthority(thread); err != nil {
		return nil, err
	}
	prepared, caseAuthorization, err = s.prepareCompactionForCurrentThread(
		ctx, thread, threadID, reason, time.Unix(0, prepared.Stamp).UTC(), auto, caseAuthorization,
	)
	if err != nil {
		return nil, err
	}
	result = prepared.Result
	if result.ReplacedTokens <= 0 {
		return nil, ErrCompactionBaselineConflict
	}
	timeout := s.transitionTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if err := transition.Prepare(ctx, prepared.SecurityContext, timeout); err != nil {
		return nil, err
	}
	if err := s.revalidateCaseCompactionImmediatelyBeforeCommit(ctx, thread, prepared); err != nil {
		return nil, errors.Join(ErrCaseCompactionRequiresTrustedArchive, err)
	}
	committed, err := s.repository.CommitCompaction(prepared.CommitRequest())
	if err != nil {
		exactAuthorityCommit := committed.AuthorityCommitted && prepared.CaseBound &&
			committed.SecurityContext == prepared.SecurityContext &&
			committed.EpochState.StateDigest == prepared.EpochState.StateDigest
		if committed.AuthorityCommitted && !exactAuthorityCommit {
			return nil, errors.Join(err, errors.New("case compaction authority commit readback does not match the prepared transition"))
		}
		if committed.Committed || exactAuthorityCommit {
			if commitErr := transition.Commit(); commitErr != nil {
				return nil, errors.Join(err, commitErr)
			}
		}
		return nil, err
	}
	if !committed.Committed || (prepared.CaseBound && !committed.AuthorityCommitted) ||
		committed.SecurityContext != prepared.SecurityContext || committed.EpochState.StateDigest != prepared.EpochState.StateDigest ||
		committed.Result.ThreadID != result.ThreadID || committed.Result.TurnID != result.TurnID || committed.Result.SourceDigest != result.SourceDigest {
		return nil, errors.New("durable compaction readback does not match the prepared authority")
	}
	if err := transition.Commit(); err != nil {
		return nil, err
	}
	if _, _, err := s.repository.RecordEvent(map[string]any{
		"kind":     "compaction_started",
		"threadId": threadID,
		"turnId":   result.TurnID,
		"itemId":   result.ItemID,
		"auto":     auto,
		"reason":   reason,
	}); err != nil {
		return nil, err
	}
	completedEvent := map[string]any{
		"kind":                    "compaction_completed",
		"threadId":                threadID,
		"turnId":                  result.TurnID,
		"itemId":                  result.ItemID,
		"reason":                  reason,
		"summary":                 result.Summary,
		"replacedTokens":          float64(result.ReplacedTokens),
		"auto":                    auto,
		"pinnedConstraints":       stringListAny(result.PinnedConstraints),
		"sourceDigest":            result.SourceDigest,
		"digestMarker":            result.DigestMarker,
		"sourceItemIds":           stringListAny(result.SourceItemIDs),
		"schemaVersion":           float64(2),
		"reasoningExcluded":       true,
		"reasoningExclusionProof": result.ReasoningExclusionProof,
	}
	if !prepared.CaseBound {
		// The existing completion producer records the committed compaction
		// turn, including its closed item proof and original timestamps.
		for _, raw := range listAny(prepared.Thread["turns"]) {
			turn, _ := raw.(map[string]any)
			if stringField(turn, "id") != result.TurnID {
				continue
			}
			public, projectionErr := ProjectPublicThread(map[string]any{"id": threadID, "status": "idle", "turns": []any{turn}})
			if projectionErr != nil {
				return nil, projectionErr
			}
			completedEvent["turn"] = contracts.CloneValue(public["turns"].([]any)[0])
		}

		if completedEvent["turn"] == nil {
			return nil, ErrForkBoundaryUnavailable
		}
	}
	completed, _, err := s.repository.RecordEvent(completedEvent)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":                      true,
		"threadId":                result.ThreadID,
		"turnId":                  result.TurnID,
		"itemId":                  result.ItemID,
		"eventSeq":                completed["seq"],
		"summary":                 result.Summary,
		"replacedTokens":          float64(result.ReplacedTokens),
		"pinnedConstraints":       stringListAny(result.PinnedConstraints),
		"sourceDigest":            result.SourceDigest,
		"digestMarker":            result.DigestMarker,
		"sourceItemIds":           stringListAny(result.SourceItemIDs),
		"schemaVersion":           float64(2),
		"reasoningExcluded":       true,
		"reasoningExclusionProof": result.ReasoningExclusionProof,
	}, nil
}

func (s *Service) validateCompactionWorkspaceAuthority(thread map[string]any) error {
	if s.workspaceReader == nil {
		return errors.New("compaction workspace authority is unavailable")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil {
		return errors.New("compaction requires an exact current security context")
	}
	realWorkspace, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(thread, "workspace")))
	if err != nil || realWorkspace != current.WorkspaceRealPath {
		return errors.Join(errors.New("compaction workspace does not match the signed current security context"), err)
	}
	return nil
}

func NormalizeApprovalPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case "always", "auto", "on-request", "untrusted", "suggest", "never":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func NormalizeSandboxMode(value string) string {
	switch strings.TrimSpace(value) {
	case "read-only", "workspace-write", "danger-full-access", "external-sandbox":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func (s *Service) defaultApprovalPolicyOrFallback() string {
	if s.defaultApprovalPolicy != "" {
		return s.defaultApprovalPolicy
	}
	return defaultApprovalPolicy
}

func (s *Service) defaultSandboxModeOrFallback() string {
	if s.defaultSandboxMode != "" {
		return s.defaultSandboxMode
	}
	return defaultSandboxMode
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func cloneMaps(items []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, contracts.CloneMap(item))
	}
	return out
}

func cloneList(items []any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, contracts.CloneValue(item))
	}
	return out
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func PendingGateIDs(thread map[string]any) ([]string, []string) {
	return gateprojection.PendingGateIDs(thread)
}
