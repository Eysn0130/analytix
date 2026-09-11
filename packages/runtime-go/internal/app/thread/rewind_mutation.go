package thread

import (
	"errors"
	"strings"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type PreparedRewindMutation struct {
	ThreadID        string
	RewindTurnID    string
	Stamp           int64
	BaselineDigest  string
	SecurityContext domainsecurity.TurnSecurityContext
	EpochState      domaincontextepoch.State
	TransitionTurn  map[string]any
	Response        map[string]any
	Thread          map[string]any
}

type RewindMutationCommitRequest struct {
	ThreadID               string
	RewindTurnID           string
	Stamp                  int64
	ExpectedBaselineDigest string
	ExpectedContextDigest  string
	CurrentContext         domainsecurity.TurnSecurityContext
}

type RewindMutationCommitResult struct {
	Response        map[string]any
	SecurityContext domainsecurity.TurnSecurityContext
	EpochState      domaincontextepoch.State
	Committed       bool
}

func PrepareRewindMutation(thread map[string]any, threadID, rewindTurnID string, current domainsecurity.TurnSecurityContext, at time.Time) (PreparedRewindMutation, error) {
	threadID = strings.TrimSpace(threadID)
	rewindTurnID = strings.TrimSpace(rewindTurnID)
	if thread == nil || threadID == "" || rewindTurnID == "" || strings.TrimSpace(stringField(thread, "id")) != threadID ||
		domainsecurity.ValidateTurnSecurityContext(current) != nil || current.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		current.ThreadID != threadID || !domainsecurity.TurnSecurityContextIsGeneral(current) {
		return PreparedRewindMutation{}, errors.New("rewind mutation authority is invalid")
	}
	if persisted, found, err := turnsecurityapp.LatestContext(thread); err != nil || found && persisted != current || !found && thread["contextEpochState"] != nil {
		return PreparedRewindMutation{}, errors.Join(errors.New("rewind current security authority is inconsistent"), err)
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	at = at.UTC()
	baselineDigest, err := MutationBaselineDigest(thread)
	if err != nil {
		return PreparedRewindMutation{}, err
	}
	plan, err := BuildRewind(RewindInput{
		Thread: thread, ThreadID: threadID, TurnID: rewindTurnID, Now: at.Format(time.RFC3339Nano), AllowCurrentSecurityReplacement: true,
	})
	if err != nil {
		return PreparedRewindMutation{}, err
	}
	target := rewindMutationTarget(current, rewindTurnID, at)
	authority, err := contextepochapp.PrepareHistoryMutation(thread, current, target, at)
	if err != nil {
		return PreparedRewindMutation{}, err
	}
	securityRecord := turnsecurityapp.PublicRecord(authority.SecurityContext)
	transitionTurn := map[string]any{
		"id": authority.SecurityContext.TurnID, "threadId": threadID, "kind": "rewind_transition", "status": "completed",
		"createdAt": at.Format(time.RFC3339Nano), "completedAt": at.Format(time.RFC3339Nano), "securityContext": securityRecord,
		"contextEpochSnapshot": contextepochapp.PublicSnapshot(authority.State.AcceptedSnapshot),
		"removedTurnIds":       contracts.CloneValue(plan.Response["removedTurnIds"]), "items": []any{},
	}
	next := contracts.CloneMap(plan.Thread)
	turns, _ := next["turns"].([]any)
	next["turns"] = append(turns, contracts.CloneMap(transitionTurn))
	next["securityState"] = contracts.CloneMap(securityRecord)
	next["contextEpochState"] = contextepochapp.PublicState(authority.State)
	next["status"] = "idle"
	next["updatedAt"] = at.Format(time.RFC3339Nano)
	response := contracts.CloneMap(plan.Response)
	response["authorityTurnId"] = authority.SecurityContext.TurnID
	return PreparedRewindMutation{
		ThreadID: threadID, RewindTurnID: rewindTurnID, Stamp: at.UnixNano(), BaselineDigest: baselineDigest,
		SecurityContext: authority.SecurityContext, EpochState: authority.State, TransitionTurn: transitionTurn,
		Response: response, Thread: next,
	}, nil
}

func ApplyRewindMutationCommit(thread map[string]any, request RewindMutationCommitRequest) (PreparedRewindMutation, error) {
	if request.Stamp <= 0 || !domainsecurity.IsSHA256Hex(strings.TrimSpace(request.ExpectedBaselineDigest)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(request.ExpectedContextDigest)) {
		return PreparedRewindMutation{}, errors.New("rewind mutation commit authority is invalid")
	}
	if err := ValidateMutationBaseline(thread, request.ThreadID, request.ExpectedBaselineDigest); err != nil {
		return PreparedRewindMutation{}, err
	}
	prepared, err := PrepareRewindMutation(
		thread, request.ThreadID, request.RewindTurnID, request.CurrentContext, time.Unix(0, request.Stamp).UTC(),
	)
	if err != nil {
		return PreparedRewindMutation{}, err
	}
	if prepared.SecurityContext.ContextDigest != strings.TrimSpace(request.ExpectedContextDigest) {
		return PreparedRewindMutation{}, ErrThreadMutationBaselineConflict
	}
	return prepared, nil
}

func CommitRewindMutationWithReadWrite(
	request RewindMutationCommitRequest,
	read func(string) (map[string]any, error),
	write func(map[string]any) error,
) (RewindMutationCommitResult, error) {
	if read == nil || write == nil {
		return RewindMutationCommitResult{}, errors.New("rewind mutation repository adapter is unavailable")
	}
	thread, err := read(request.ThreadID)
	if err != nil || thread == nil {
		if err == nil {
			err = ErrThreadNotFound
		}
		return RewindMutationCommitResult{}, err
	}
	prepared, err := ApplyRewindMutationCommit(thread, request)
	if err != nil {
		return RewindMutationCommitResult{}, err
	}
	targetDigest, err := MutationBaselineDigest(prepared.Thread)
	if err != nil {
		return RewindMutationCommitResult{}, err
	}
	result := RewindMutationCommitResult{
		Response: prepared.Response, SecurityContext: prepared.SecurityContext, EpochState: prepared.EpochState,
	}
	writeErr := write(prepared.Thread)
	reloaded, readErr := read(request.ThreadID)
	readbackErr := validateRewindMutationReadback(reloaded, prepared)
	readbackDigest, digestErr := MutationBaselineDigest(reloaded)
	result.Committed = readErr == nil && readbackErr == nil && digestErr == nil && readbackDigest == targetDigest
	if writeErr != nil || readErr != nil || readbackErr != nil || digestErr != nil {
		return result, errors.Join(writeErr, readErr, readbackErr, digestErr)
	}
	if !result.Committed {
		return result, errors.New("durable rewind mutation readback diverged")
	}
	return result, nil
}

func CommitRewindMutationUnlessCaseBound(
	request RewindMutationCommitRequest,
	caseBound bool,
	read func(string) (map[string]any, error),
	write func(map[string]any) error,
) (RewindMutationCommitResult, error) {
	if caseBound {
		return RewindMutationCommitResult{}, ErrCaseRewindSignedArchive
	}
	return CommitRewindMutationWithReadWrite(request, read, write)
}

func validateRewindMutationReadback(thread map[string]any, prepared PreparedRewindMutation) error {
	current, found, err := turnsecurityapp.LatestContext(thread)
	if err != nil || !found || current != prepared.SecurityContext {
		return errors.Join(errors.New("durable rewind current context diverged"), err)
	}
	state, found, err := contextepochapp.StateFromThread(thread)
	if err != nil || !found || state.StateDigest != prepared.EpochState.StateDigest {
		return errors.Join(errors.New("durable rewind epoch state diverged"), err)
	}
	turns, _ := thread["turns"].([]any)
	var highWater map[string]any
	if len(turns) > 0 {
		highWater, _ = turns[len(turns)-1].(map[string]any)
	}
	if strings.TrimSpace(stringField(highWater, "id")) != prepared.SecurityContext.TurnID {
		return errors.New("durable rewind transition is not the high-water")
	}
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if strings.TrimSpace(stringField(turn, "id")) == prepared.RewindTurnID {
			return errors.New("durable rewind retained the removed target")
		}
	}
	return nil
}

func rewindMutationTarget(current domainsecurity.TurnSecurityContext, rewindTurnID string, at time.Time) domainsecurity.TurnSecurityContext {
	digest := domainsecurity.SHA256Hex([]byte(current.ThreadID + "\x00" + rewindTurnID + "\x00" + at.Format(time.RFC3339Nano)))
	target, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: "turn_rewind_" + digest[:24], WorkspaceRealPath: current.WorkspaceRealPath,
		TenantID: current.TenantID, UserID: current.UserID, CaseID: current.CaseID, CaseBindingHash: current.CaseBindingHash,
		DatasetSnapshotID: current.DatasetSnapshotID, SourceManifestHash: current.SourceManifestHash,
		ContextEpoch: current.ContextEpoch, IssuedAt: at, PublicationPolicy: current.PublicationPolicy,
		RiskAuthorityBinding: current.RiskAuthorityBinding,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}
	}
	return target
}
