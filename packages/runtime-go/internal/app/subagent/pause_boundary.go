package subagent

import (
	"context"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type PauseBoundaryJobStore interface {
	LoadChildRun(id string) (domainjob.Record, error)
	UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error)
	MarkPauseRequestPaused(id string, requestID string, pausedAt string, token domainjob.ResumeToken) (domainjob.Record, domainjob.PauseRequest, error)
	MarkPauseRequestResumed(id string, requestID string, resumedAt string) (domainjob.Record, domainjob.PauseRequest, error)
	CompletePauseResume(id string, requestID string) (domainjob.Record, domainjob.PauseRequest, error)
	ExpirePauseRequest(id string, requestID string, expiredAt string, reason string) (domainjob.Record, domainjob.PauseRequest, error)
}

type PauseBoundaryInput struct {
	ChildRunID  string
	State       *RuntimeState
	Jobs        PauseBoundaryJobStore
	Now         func() time.Time
	RecordEvent ChildPauseEventRecorder
}

func WaitAtBackgroundPauseBoundary(ctx context.Context, input PauseBoundaryInput) error {
	childRunID := strings.TrimSpace(input.ChildRunID)
	if childRunID == "" || input.State == nil || input.Jobs == nil {
		return nil
	}
	snapshot, ok := input.State.BackgroundJobPauseSnapshot(childRunID)
	if !ok {
		return nil
	}
	now := pauseBoundaryNow(input.Now)
	if snapshot.Status == "pause_requested" && !snapshot.RequestedAt.IsZero() && now.Sub(snapshot.RequestedAt) > DefaultTaskJobPauseRequestTimeout {
		reason := "pause request expired before a safe boundary"
		record, pauseRequest, err := input.Jobs.ExpirePauseRequest(childRunID, snapshot.PauseRequestID, now.Format(time.RFC3339Nano), reason)
		if err == nil && input.RecordEvent != nil {
			input.RecordEvent(record, pauseRequest, "expired", reason)
		}
		input.State.ClearBackgroundJobPauseRequest(childRunID, snapshot.PauseRequestID)
		return nil
	}
	_, err := input.State.WaitIfBackgroundJobPauseRequested(ctx, childRunID, BackgroundJobPauseCallbacks{
		OnPaused: func(snapshot BackgroundJobPauseSnapshot) error {
			record, err := input.Jobs.LoadChildRun(childRunID)
			if err != nil {
				return err
			}
			pauseRequest := LatestRequestedPause(record)
			if strings.TrimSpace(pauseRequest.ID) == "" {
				pauseRequest = LatestPauseRequest(record)
			}
			if strings.TrimSpace(pauseRequest.ID) == "" {
				return nil
			}
			token := BuildResumeToken(record, pauseRequest, snapshot.PausedAt)
			updated, pausedRequest, err := input.Jobs.MarkPauseRequestPaused(childRunID, pauseRequest.ID, snapshot.PausedAt.Format(time.RFC3339Nano), token)
			if err != nil {
				return err
			}
			if input.RecordEvent != nil {
				input.RecordEvent(updated, pausedRequest, "paused", "")
			}
			return nil
		},
		OnResumed: func(snapshot BackgroundJobPauseSnapshot) error {
			record, pauseRequest, err := input.Jobs.MarkPauseRequestResumed(childRunID, snapshot.PauseRequestID, pauseBoundaryNow(input.Now).Format(time.RFC3339Nano))
			if err != nil {
				return err
			}
			if strings.TrimSpace(pauseRequest.ID) != "" && input.RecordEvent != nil {
				input.RecordEvent(record, pauseRequest, "resumed", "")
			}
			_, _, err = input.Jobs.CompletePauseResume(childRunID, pauseRequest.ID)
			return err
		},
	})
	return err
}

func pauseBoundaryNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}
	value := now()
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}
