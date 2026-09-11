package subagent

import (
	"errors"
	"fmt"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type JobStartRecordClaimer interface {
	LoadChildRun(string) (domainjob.Record, error)
	ClaimChildRunStart(domainjob.Record) (domainjob.Record, error)
}

// JobStartClaimError means the durable job record could not be atomically
// claimed from the exact status and runtime lease observed by the caller. A
// caller must not rewrite the returned record: another lifecycle operation may
// already own it.
type JobStartClaimError struct {
	cause error
}

func (err *JobStartClaimError) Error() string {
	if err == nil || err.cause == nil {
		return "task job start claim rejected"
	}
	return "task job start claim rejected: " + err.cause.Error()
}

func (err *JobStartClaimError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func IsJobStartClaimError(err error) bool {
	var claimErr *JobStartClaimError
	return errors.As(err, &claimErr)
}

// AuthorizeJobStart atomically claims the exact durable status/runtime lease at
// the last boundary before a child provider or background process can start.
// The caller supplies the current-context blocker and dead-letter transition,
// but cannot turn a stale, stopped, paused, replaced, or dead-lettered record
// into an executable record.
func AuthorizeJobStart(record domainjob.Record, authorityAvailable bool, jobs JobStartRecordClaimer, blocker func(domainjob.Record) string, markDeadLetter func(domainjob.Record, string) error) (domainjob.Record, error) {
	if !authorityAvailable || jobs == nil || blocker == nil {
		return record, errors.New("task job authority unavailable before execution")
	}
	latest, err := jobs.LoadChildRun(record.ID)
	if err != nil {
		return record, fmt.Errorf("task job record unavailable before execution: %w", err)
	}
	if reason := strings.TrimSpace(blocker(latest)); reason != "" {
		if markDeadLetter != nil {
			if markErr := markDeadLetter(latest, reason); markErr != nil {
				return latest, errors.Join(
					fmt.Errorf("task job authority rejected before execution: %s", reason),
					fmt.Errorf("persist task job dead letter: %w", markErr),
				)
			}
		}
		return latest, fmt.Errorf("task job authority rejected before execution: %s", reason)
	}
	claimed, err := jobs.ClaimChildRunStart(record)
	if err != nil {
		if strings.TrimSpace(claimed.ID) == "" {
			claimed = latest
		}
		return claimed, &JobStartClaimError{cause: fmt.Errorf("durable status/lease CAS failed: %w", err)}
	}
	return claimed, nil
}
