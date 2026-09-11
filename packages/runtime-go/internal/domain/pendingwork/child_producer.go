package pendingwork

import (
	"errors"
	"strconv"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// ChildProducerV1 is part of the existing private side-effect receipt. It
// records the complete immediate host allocation, not child completion or
// permission to retry. ParentBindingDigest binds the full frozen parent job
// security binding without persisting principal or workspace values here.
type ChildProducerV1 struct {
	ParentBindingDigest string                  `json:"parentBindingDigest"`
	Children            []ChildProducerTargetV1 `json:"children"`
}

type ChildProducerTargetV1 struct {
	Ordinal       uint32 `json:"ordinal"`
	JobID         string `json:"jobId"`
	ChildThreadID string `json:"childThreadId"`
	ChildTurnID   string `json:"childTurnId"`
}

func ValidateChildProducerV1(producer *ChildProducerV1) error {
	if producer == nil || !domainsecurity.IsSHA256Hex(producer.ParentBindingDigest) || len(producer.Children) == 0 {
		return errors.New("child producer allocation is incomplete")
	}
	jobs, turns := make(map[string]bool), make(map[string]bool)
	for index, child := range producer.Children {
		if uint64(child.Ordinal) != uint64(index)+1 || !canonicalChildCounterID(child.JobID, "job-") ||
			!canonicalChildCounterID(child.ChildTurnID, "turn_") || !domainthread.IsCanonicalRecordID(child.ChildThreadID) ||
			jobs[child.JobID] || turns[child.ChildTurnID] {
			return errors.New("child producer ordered allocation is invalid")
		}
		jobs[child.JobID], turns[child.ChildTurnID] = true, true
	}
	return nil
}

func CloneChildProducerV1(producer *ChildProducerV1) *ChildProducerV1 {
	if producer == nil {
		return nil
	}
	cloned := *producer
	cloned.Children = append([]ChildProducerTargetV1(nil), producer.Children...)
	return &cloned
}

func validateReceiptChildProducer(kind string, producer *ChildProducerV1) error {
	if producer == nil {
		// Historical records retain their original signature and audit validity.
		// Live child admission separately requires a complete host allocation.
		return nil
	}
	if kind != KindSideEffectIntent {
		return errors.New("child producer allocation requires a side-effect intent")
	}
	return ValidateChildProducerV1(producer)
}

func canonicalChildCounterID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	digits := strings.TrimPrefix(value, prefix)
	number, err := strconv.ParseInt(digits, 10, strconv.IntSize)
	return err == nil && number > 0 && digits == strconv.FormatInt(number, 10)
}
