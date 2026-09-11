package pendingwork

import (
	"errors"
	"strconv"
	"strings"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// ChildIdentityFloorsV1 contains only process-local monotone allocator bounds.
// It is neither a signed record nor permission to execute or recreate a child.
type ChildIdentityFloorsV1 struct {
	JobSequence    int
	ThreadSequence int
	ForkSequence   int
	ResumeSequence int
	TurnSequence   int
}

func (floors ChildIdentityFloorsV1) Validate() error {
	if floors.JobSequence < 0 || floors.ThreadSequence < 0 || floors.ForkSequence < 0 || floors.ResumeSequence < 0 || floors.TurnSequence < 0 {
		return errors.New("child identity allocator floor is invalid")
	}
	return nil
}

func MergeChildIdentityFloorsV1(values ...ChildIdentityFloorsV1) (ChildIdentityFloorsV1, error) {
	var merged ChildIdentityFloorsV1
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return ChildIdentityFloorsV1{}, err
		}
		merged.JobSequence = max(merged.JobSequence, value.JobSequence)
		merged.ThreadSequence = max(merged.ThreadSequence, value.ThreadSequence)
		merged.ForkSequence = max(merged.ForkSequence, value.ForkSequence)
		merged.ResumeSequence = max(merged.ResumeSequence, value.ResumeSequence)
		merged.TurnSequence = max(merged.TurnSequence, value.TurnSequence)
	}
	return merged, nil
}

func (floors *ChildIdentityFloorsV1) Observe(jobID, threadID, turnID string) error {
	if floors == nil || floors.Validate() != nil {
		return errors.New("child identity allocator floor is unavailable")
	}
	for _, pair := range []struct {
		id, prefix string
		floor      *int
	}{
		{jobID, "job-", &floors.JobSequence},
		// The retired runtime shares the current turn allocator's sequence.
		{turnID, "turn_" + string([]rune{100, 48, 50, 52, 50}) + "_", &floors.TurnSequence},
		{turnID, "turn_", &floors.TurnSequence},
	} {
		if err := observeIdentityCounterV1(pair.id, pair.prefix, pair.floor); err != nil {
			return err
		}
	}
	if threadID == "" {
		return nil
	}
	if !domainthread.IsCanonicalRecordID(threadID) {
		return errors.New("child thread floor identity is invalid")
	}
	for _, family := range []struct {
		prefix string
		floor  *int
	}{
		{"thr_durable_fork_", &floors.ForkSequence},
		{"thr_durable_resume_", &floors.ResumeSequence},
		{"thr_durable_", &floors.ThreadSequence},
	} {
		if strings.HasPrefix(threadID, family.prefix) {
			return observeIdentityCounterV1(threadID, family.prefix, family.floor)
		}
	}
	return nil
}

func observeIdentityCounterV1(id, prefix string, floor *int) error {
	if id == "" {
		return nil
	}
	if !domainthread.IsCanonicalRecordID(id) {
		return errors.New("child allocator identity is invalid")
	}
	if !strings.HasPrefix(id, prefix) {
		return nil
	}
	digits := strings.TrimPrefix(id, prefix)
	if digits == "" {
		return errors.New("child allocator identity counter is absent")
	}
	for _, digit := range digits {
		// Non-counter legacy identities remain separate names; they cannot
		// select or wrap an integer from the current allocator family.
		if digit < '0' || digit > '9' {
			return nil
		}
	}
	value, err := strconv.ParseUint(digits, 10, strconv.IntSize-1)
	if err != nil {
		return errors.New("child allocator identity counter exceeds host range")
	}
	if int(value) > *floor {
		*floor = int(value)
	}
	return nil
}
