package thread

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

var (
	ErrCompactionBaselineConflict           = errors.New("durable thread changed before compaction commit")
	ErrCaseCompactionRequiresTrustedArchive = errors.New("case compaction requires a trusted signed authority archive")
	ErrCaseCompactionPendingRecovery        = errors.New("signed case compaction requires durable thread recovery")
)

// CompactionTransition is implemented by the shared RuntimeState transition.
// The app service keeps this narrow interface so thread use cases do not own a
// second epoch/effect gate.
type CompactionTransition interface {
	Prepare(context.Context, domainsecurity.TurnSecurityContext, time.Duration) error
	Commit() error
	Abort()
}

type BeginCompactionTransitionFunc func(context.Context, domainsecurity.TurnSecurityContext) (CompactionTransition, error)

func AdaptBeginTransition[T CompactionTransition](begin func(context.Context, domainsecurity.TurnSecurityContext) (T, error)) BeginCompactionTransitionFunc {
	return func(ctx context.Context, target domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
		return begin(ctx, target)
	}
}

type CompactionCommitRequest struct {
	ThreadID                 string
	Reason                   string
	Auto                     bool
	Stamp                    int64
	ExpectedBaselineDigest   string
	ExpectedContextDigest    string
	ExpectedTurnID           string
	ExpectedEpochStateDigest string
	ExpectedSourceDigest     string
	CaseBound                bool
	CaseContinuation         threaddomain.TaskContinuationSnapshotV1
	CaseSourceContextDigest  string
	CaseAuthorityTurnIDs     []string
	ActiveInheritedHistory   *domainsecurity.CaseThreadAuthorityRecord
}

type CompactionCommitResult struct {
	Result          threaddomain.CompactionResult
	SecurityContext domainsecurity.TurnSecurityContext
	EpochState      domaincontextepoch.State
	// AuthorityCommitted is set only after an exact installation-signed case
	// target and epoch state pass durable authority readback. The canonical
	// thread write can still fail after this point; callers must then advance
	// the prepared security transition so old case effects cannot resume.
	AuthorityCommitted bool
	Committed          bool
}

type PreparedCompaction struct {
	Result                  threaddomain.CompactionResult
	SecurityContext         domainsecurity.TurnSecurityContext
	EpochState              domaincontextepoch.State
	BaselineDigest          string
	Reason                  string
	Auto                    bool
	Stamp                   int64
	Thread                  map[string]any
	CaseBound               bool
	CaseContinuation        threaddomain.TaskContinuationSnapshotV1
	CaseSourceContextDigest string
	CaseAuthorityTurnIDs    []string
	ActiveInheritedHistory  *domainsecurity.CaseThreadAuthorityRecord
}

func (prepared PreparedCompaction) CommitRequest() CompactionCommitRequest {
	return CompactionCommitRequest{
		ThreadID: prepared.Result.ThreadID, Reason: prepared.Reason, Auto: prepared.Auto, Stamp: prepared.Stamp,
		ExpectedBaselineDigest: prepared.BaselineDigest, ExpectedContextDigest: prepared.SecurityContext.ContextDigest,
		ExpectedTurnID:           prepared.Result.TurnID,
		ExpectedEpochStateDigest: prepared.EpochState.StateDigest,
		ExpectedSourceDigest:     prepared.Result.SourceDigest,
		CaseBound:                prepared.CaseBound, CaseContinuation: cloneCaseCompactionContinuation(prepared.CaseContinuation),
		CaseSourceContextDigest: prepared.CaseSourceContextDigest,
		CaseAuthorityTurnIDs:    append([]string(nil), prepared.CaseAuthorityTurnIDs...),
		ActiveInheritedHistory:  cloneActiveInheritedHistoryRecordV1(prepared.ActiveInheritedHistory),
	}
}

// PrepareCompaction is a pure preparation over a cloned durable snapshot.
// Case-bound history is rejected until a signed archive/prune contract exists:
// deleting a turn while retaining its CaseThreadAuthority record would make
// the durable high-water unverifiable after restart.
func PrepareCompaction(thread map[string]any, threadID string, reason string, at time.Time) (PreparedCompaction, error) {
	return prepareCompaction(thread, threadID, reason, at, false, nil)
}

func PrepareAutomaticCompaction(thread map[string]any, threadID string, reason string, at time.Time) (PreparedCompaction, error) {
	return prepareCompaction(thread, threadID, reason, at, true, nil)
}

type CaseCompactionAuthorization struct {
	Continuation           threaddomain.TaskContinuationSnapshotV1
	SourceContextDigest    string
	AuthorityTurnIDs       []string
	ActiveInheritedHistory *domainsecurity.CaseThreadAuthorityRecord
}

func PrepareCaseCompaction(
	thread map[string]any,
	threadID string,
	reason string,
	at time.Time,
	auto bool,
	authorization CaseCompactionAuthorization,
) (PreparedCompaction, error) {
	return prepareCompaction(thread, threadID, reason, at, auto, &authorization)
}

func prepareCompaction(thread map[string]any, threadID string, reason string, at time.Time, auto bool, caseAuthorization *CaseCompactionAuthorization) (PreparedCompaction, error) {
	threadID = strings.TrimSpace(threadID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual"
	}
	if thread == nil || threadID == "" || strings.TrimSpace(stringField(thread, "id")) != threadID {
		return PreparedCompaction{}, errors.New("compaction thread authority is invalid")
	}
	if strings.EqualFold(stringField(thread, "status"), "running") {
		return PreparedCompaction{}, ErrThreadRunning
	}
	baselineDigest, err := CompactionBaselineDigest(thread)
	if err != nil {
		return PreparedCompaction{}, err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	at = at.UTC()
	stamp := at.UnixNano()
	now := at.Format(time.RFC3339Nano)
	var inherited []domainsecurity.ActiveInheritedTurnV1
	if caseAuthorization != nil && caseAuthorization.ActiveInheritedHistory != nil {
		if _, err := validateActiveInheritedHistoryViewsV1(*caseAuthorization.ActiveInheritedHistory, thread); err != nil {
			return PreparedCompaction{}, err
		}
		inherited = caseAuthorization.ActiveInheritedHistory.ActiveInheritedHistory.Turns
	} else if _, present := thread["activeInheritedHistoryReceipt"]; present {
		return PreparedCompaction{}, errors.New("active inherited compaction provenance is required")
	}
	candidate := contracts.CloneMap(thread)
	var plan turnapp.CompactionPlan
	if caseAuthorization != nil {
		plan = turnapp.BuildThreadCompactionWithCaseAuthority(
			candidate, threadID, reason, stamp, now, auto,
			caseAuthorization.Continuation, caseAuthorization.AuthorityTurnIDs, inherited,
		)
	} else {
		plan = turnapp.BuildThreadCompactionWithMode(candidate, threadID, reason, stamp, now, auto)
	}
	if plan.Error != nil {
		return PreparedCompaction{}, plan.Error
	}
	prepared := PreparedCompaction{
		Result: plan.Result, BaselineDigest: baselineDigest, Reason: reason, Auto: auto, Stamp: stamp, Thread: candidate,
	}
	if !plan.Changed {
		return prepared, nil
	}
	current, err := domainsecurity.ParseTurnSecurityContext(candidate["securityState"])
	if err != nil || current.ThreadID != threadID {
		return PreparedCompaction{}, errors.New("compaction requires an exact current security context")
	}
	if err := validateCompactionCurrentTurn(candidate, current); err != nil {
		return PreparedCompaction{}, err
	}
	caseBound := domainsecurity.TurnSecurityContextIsCaseSensitive(current)
	if caseBound && caseAuthorization == nil {
		return PreparedCompaction{}, ErrCaseCompactionRequiresTrustedArchive
	}
	if !caseBound && caseAuthorization != nil {
		return PreparedCompaction{}, errors.New("case compaction authority cannot rewrite an ordinary thread")
	}
	if caseBound {
		continuation, authorityTurnIDs, authorityErr := validateCaseCompactionAuthorization(
			current, *caseAuthorization,
		)
		if authorityErr != nil {
			return PreparedCompaction{}, authorityErr
		}
		prepared.CaseBound = true
		prepared.CaseContinuation = continuation
		prepared.CaseSourceContextDigest = current.ContextDigest
		prepared.CaseAuthorityTurnIDs = authorityTurnIDs
		prepared.ActiveInheritedHistory = cloneActiveInheritedHistoryRecordV1(caseAuthorization.ActiveInheritedHistory)
	}
	authority, err := contextepochapp.PrepareAndAttachCompactionAuthority(
		candidate, plan.NextTurns, current, threadID, plan.Result.TurnID, plan.Result.SourceDigest, at,
		caseAuthorization != nil,
	)
	if err != nil {
		return PreparedCompaction{}, err
	}
	candidate["updatedAt"] = now
	prepared.SecurityContext = authority.SecurityContext
	prepared.EpochState = authority.State
	prepared.Thread = candidate
	return prepared, nil
}

// ApplyCompactionCommit repeats preparation under the repository lock and
// accepts only the exact snapshot/context prepared by the app service.
func ApplyCompactionCommit(thread map[string]any, request CompactionCommitRequest) (PreparedCompaction, error) {
	if request.Stamp <= 0 || !domainsecurity.IsSHA256Hex(strings.TrimSpace(request.ExpectedBaselineDigest)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(request.ExpectedContextDigest)) ||
		strings.TrimSpace(request.ExpectedTurnID) == "" ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(request.ExpectedEpochStateDigest)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(request.ExpectedSourceDigest)) ||
		(request.CaseBound && !domainsecurity.IsSHA256Hex(strings.TrimSpace(request.CaseSourceContextDigest))) {
		return PreparedCompaction{}, errors.New("compaction commit authority is invalid")
	}
	currentDigest, err := CompactionBaselineDigest(thread)
	if err != nil || currentDigest != strings.TrimSpace(request.ExpectedBaselineDigest) {
		return PreparedCompaction{}, ErrCompactionBaselineConflict
	}
	var caseAuthorization *CaseCompactionAuthorization
	if request.CaseBound {
		caseAuthorization = &CaseCompactionAuthorization{
			Continuation:           cloneCaseCompactionContinuation(request.CaseContinuation),
			SourceContextDigest:    strings.TrimSpace(request.CaseSourceContextDigest),
			AuthorityTurnIDs:       append([]string(nil), request.CaseAuthorityTurnIDs...),
			ActiveInheritedHistory: cloneActiveInheritedHistoryRecordV1(request.ActiveInheritedHistory),
		}
	}
	prepared, err := prepareCompaction(
		thread, strings.TrimSpace(request.ThreadID), strings.TrimSpace(request.Reason), time.Unix(0, request.Stamp).UTC(), request.Auto,
		caseAuthorization,
	)
	if err != nil {
		return PreparedCompaction{}, err
	}
	if prepared.Result.ReplacedTokens <= 0 || prepared.SecurityContext.ContextDigest != strings.TrimSpace(request.ExpectedContextDigest) ||
		prepared.Result.TurnID != strings.TrimSpace(request.ExpectedTurnID) ||
		prepared.EpochState.StateDigest != strings.TrimSpace(request.ExpectedEpochStateDigest) ||
		prepared.Result.SourceDigest != strings.TrimSpace(request.ExpectedSourceDigest) ||
		prepared.CaseSourceContextDigest != strings.TrimSpace(request.CaseSourceContextDigest) {
		return PreparedCompaction{}, ErrCompactionBaselineConflict
	}
	return prepared, nil
}

func validateCaseCompactionAuthorization(
	current domainsecurity.TurnSecurityContext,
	authorization CaseCompactionAuthorization,
) (threaddomain.TaskContinuationSnapshotV1, []string, error) {
	continuation := cloneCaseCompactionContinuation(authorization.Continuation)
	parsed, err := threaddomain.ParseTaskContinuationSnapshotV1(
		threaddomain.TaskContinuationSnapshotMapV1(continuation),
	)
	if err != nil || strings.TrimSpace(authorization.SourceContextDigest) != current.ContextDigest {
		return threaddomain.TaskContinuationSnapshotV1{}, nil,
			errors.New("case compaction continuation does not match current authority")
	}
	turnIDs := canonicalCompactionAuthorityTurnIDs(authorization.AuthorityTurnIDs)
	if len(turnIDs) == 0 || len(turnIDs) != len(authorization.AuthorityTurnIDs) {
		return threaddomain.TaskContinuationSnapshotV1{}, nil,
			errors.New("case compaction turn authority inventory is invalid")
	}
	return parsed, turnIDs, nil
}

func cloneCaseCompactionContinuation(
	continuation threaddomain.TaskContinuationSnapshotV1,
) threaddomain.TaskContinuationSnapshotV1 {
	parsed, err := threaddomain.ParseTaskContinuationSnapshotV1(
		threaddomain.TaskContinuationSnapshotMapV1(continuation),
	)
	if err != nil {
		return continuation
	}
	return parsed
}

func canonicalCompactionAuthorityTurnIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// CommitCompactionWithReadWrite owns the repository transaction algorithm;
// the outbound adapter supplies lock-scoped durable read/write operations.
// Readback distinguishes a write that committed its canonical thread before a
// later sidecar error so the shared in-memory authority can still advance to
// the durable truth.
func CommitCompactionWithReadWrite(
	request CompactionCommitRequest,
	read func(string) (map[string]any, error),
	write func(map[string]any) error,
) (CompactionCommitResult, error) {
	if read == nil || write == nil {
		return CompactionCommitResult{}, errors.New("compaction repository adapter is unavailable")
	}
	thread, err := read(request.ThreadID)
	if err != nil {
		return CompactionCommitResult{}, err
	}
	if thread == nil {
		return CompactionCommitResult{}, ErrThreadNotFound
	}
	prepared, err := ApplyCompactionCommit(thread, request)
	if err != nil {
		return CompactionCommitResult{}, err
	}
	targetDigest, err := CompactionBaselineDigest(prepared.Thread)
	if err != nil {
		return CompactionCommitResult{}, err
	}
	result := CompactionCommitResult{Result: prepared.Result, SecurityContext: prepared.SecurityContext, EpochState: prepared.EpochState}
	writeErr := write(prepared.Thread)
	reloaded, readErr := read(prepared.Result.ThreadID)
	readbackDigest, digestErr := CompactionBaselineDigest(reloaded)
	result.Committed = readErr == nil && digestErr == nil && readbackDigest == targetDigest
	if writeErr != nil || readErr != nil || digestErr != nil {
		return result, errors.Join(writeErr, readErr, digestErr)
	}
	if !result.Committed {
		return result, errors.New("durable compaction readback diverged")
	}
	return result, nil
}

func CompactionBaselineDigest(thread map[string]any) (string, error) {
	body, err := json.Marshal(thread)
	if err != nil || len(body) == 0 {
		return "", errors.New("durable compaction baseline is not canonical")
	}
	return domainsecurity.CanonicalJSONHash(body), nil
}

func validateCompactionCurrentTurn(thread map[string]any, current domainsecurity.TurnSecurityContext) error {
	turns, ok := thread["turns"].([]any)
	if !ok || len(turns) == 0 {
		return errors.New("compaction current turn authority is unavailable")
	}
	latest, ok := turns[len(turns)-1].(map[string]any)
	if !ok || strings.TrimSpace(stringField(latest, "id")) != current.TurnID {
		return errors.New("compaction current turn is not the durable high-water")
	}
	frozen, err := domainsecurity.ParseTurnSecurityContext(latest["securityContext"])
	if err != nil || frozen != current {
		return errors.New("compaction current turn security context is inconsistent")
	}
	return nil
}
