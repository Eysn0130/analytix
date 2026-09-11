package job

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const SteerMessageProjectionVersionV1 = 1

const SteerQueueAuthorityVersionV1 = 1

const SteerPromotionSettlementVersionV1 = 1

type SteerPromotionSettlementV1 struct {
	Version           int    `json:"version"`
	PromotionCommitID string `json:"promotionCommitId"`
	PromotionEntryID  string `json:"promotionEntryId"`
	ContextDigest     string `json:"contextDigest"`
	PromotedAt        string `json:"promotedAt"`
}

type SteerQueueAuthorityV1 struct {
	Version               int    `json:"version"`
	JobID                 string `json:"jobId"`
	RecordUpdatedAt       string `json:"recordUpdatedAt"`
	Status                string `json:"status"`
	Background            bool   `json:"background"`
	ChildThreadID         string `json:"childThreadId"`
	ChildTurnID           string `json:"childTurnId"`
	SecurityBindingDigest string `json:"securityBindingDigest,omitempty"`
	SteerSetDigest        string `json:"steerSetDigest"`
	ContextDigest         string `json:"contextDigest,omitempty"`
	PendingUntilChildTurn bool   `json:"pendingUntilChildTurn"`
}

type steerMessageContentV1 struct {
	Version          int                          `json:"version"`
	ID               string                       `json:"id"`
	ParentThreadID   string                       `json:"parentThreadId"`
	ChildRunID       string                       `json:"childRunId"`
	JobID            string                       `json:"jobId"`
	Text             string                       `json:"text"`
	SourceTurnID     string                       `json:"sourceTurnId"`
	SourceToolCallID string                       `json:"sourceToolCallId"`
	LogicalEffect    domainsecurity.LogicalEffect `json:"logicalEffect,omitempty"`
	OrdinaryWork     bool                         `json:"ordinaryWork,omitempty"`
}

func SteerMessageContentDigestV1(message SteerMessage) string {
	payload, _ := json.Marshal(steerMessageContentV1{
		Version:          SteerMessageProjectionVersionV1,
		ID:               strings.TrimSpace(message.ID),
		ParentThreadID:   strings.TrimSpace(message.ParentThreadID),
		ChildRunID:       strings.TrimSpace(message.ChildRunID),
		JobID:            strings.TrimSpace(message.JobID),
		Text:             strings.TrimSpace(message.Text),
		SourceTurnID:     strings.TrimSpace(message.SourceTurnID),
		SourceToolCallID: strings.TrimSpace(message.SourceToolCallID),
		LogicalEffect:    message.LogicalEffect,
		OrdinaryWork:     message.OrdinaryWork,
	})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// ValidateSteerMessageLogicalEffectBindingV1 validates intent metadata only;
// it never grants provider, tool, case, or dataset authority. An absent
// binding remains readable solely for legacy V1 records.
func ValidateSteerMessageLogicalEffectBindingV1(message SteerMessage) error {
	effect := message.LogicalEffect
	if effect == "" {
		if message.OrdinaryWork {
			return errors.New("task-job steering logical effect binding is partial")
		}
		return nil
	}
	if domainsecurity.ValidateLogicalEffect(effect) != nil ||
		(effect == domainsecurity.LogicalEffectOrdinary && !message.OrdinaryWork) {
		return errors.New("task-job steering logical effect binding is invalid")
	}
	return nil
}

// ValidateSteerTextProjectionV1 proves that the caller supplied the exact
// host-projected ordinary text. The job store never projects and then accepts
// a raw steer because doing so would detach the persisted bytes from the
// context/queue authority that covered the caller's original message.
func ValidateSteerTextProjectionV1(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || value != ProjectPersistableUntrustedOutputV1(value) {
		return errors.New("steer message text lacks a closed host projection")
	}
	return nil
}

func SteerMessagesHaveSameContentV1(left, right SteerMessage) bool {
	leftDigest := strings.TrimSpace(left.ContentDigest)
	rightDigest := strings.TrimSpace(right.ContentDigest)
	return left.ProjectionVersion == SteerMessageProjectionVersionV1 &&
		right.ProjectionVersion == SteerMessageProjectionVersionV1 &&
		leftDigest != "" && leftDigest == rightDigest &&
		leftDigest == SteerMessageContentDigestV1(left) &&
		rightDigest == SteerMessageContentDigestV1(right) &&
		strings.TrimSpace(left.ContextDigest) == strings.TrimSpace(right.ContextDigest) &&
		strings.TrimSpace(left.AuthorityDigest) == strings.TrimSpace(right.AuthorityDigest)
}

// RejectedSteerMatchesQueuedContentV1 verifies an exact retry after the
// durable tombstone has intentionally erased private text and source fields.
// The original content digest remains authoritative; erased data is never
// reconstructed or written back into the tombstone.
func RejectedSteerMatchesQueuedContentV1(tombstone, queued SteerMessage) bool {
	if strings.TrimSpace(tombstone.Status) != "rejected" || strings.TrimSpace(queued.Status) != "queued" ||
		tombstone.Text != "" || tombstone.SourceTurnID != "" || tombstone.SourceToolCallID != "" ||
		queued.ProjectionVersion != SteerMessageProjectionVersionV1 ||
		strings.TrimSpace(queued.ContentDigest) == "" || queued.ContentDigest != SteerMessageContentDigestV1(queued) {
		return false
	}
	return strings.TrimSpace(tombstone.ID) == strings.TrimSpace(queued.ID) &&
		strings.TrimSpace(tombstone.ParentThreadID) == strings.TrimSpace(queued.ParentThreadID) &&
		strings.TrimSpace(tombstone.ChildRunID) == strings.TrimSpace(queued.ChildRunID) &&
		strings.TrimSpace(tombstone.JobID) == strings.TrimSpace(queued.JobID) &&
		tombstone.ProjectionVersion == queued.ProjectionVersion &&
		strings.TrimSpace(tombstone.ContentDigest) == strings.TrimSpace(queued.ContentDigest) &&
		strings.TrimSpace(tombstone.ContextDigest) == strings.TrimSpace(queued.ContextDigest) &&
		strings.TrimSpace(tombstone.AuthorityDigest) == strings.TrimSpace(queued.AuthorityDigest) &&
		strings.TrimSpace(tombstone.QueueAuthorityDigest) == strings.TrimSpace(queued.QueueAuthorityDigest) &&
		tombstone.LogicalEffect == queued.LogicalEffect && tombstone.OrdinaryWork == queued.OrdinaryWork &&
		strings.TrimSpace(tombstone.CreatedAt) == strings.TrimSpace(queued.CreatedAt) && tombstone.AdmittedAt == ""
}

func ValidateSteerPromotionSettlementV1(settlement SteerPromotionSettlementV1, queued SteerMessage) error {
	commitDigest := strings.TrimPrefix(strings.TrimSpace(settlement.PromotionCommitID), "steer_commit_")
	entryDigest := strings.TrimPrefix(strings.TrimSpace(settlement.PromotionEntryID), "item_steer_")
	promotedAt := strings.TrimSpace(settlement.PromotedAt)
	parsed, timeErr := time.Parse(time.RFC3339Nano, promotedAt)
	if settlement.Version != SteerPromotionSettlementVersionV1 ||
		!strings.HasPrefix(strings.TrimSpace(settlement.PromotionCommitID), "steer_commit_") || !domainsecurity.IsSHA256Hex(commitDigest) ||
		!strings.HasPrefix(strings.TrimSpace(settlement.PromotionEntryID), "item_steer_") || !domainsecurity.IsSHA256Hex(entryDigest) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(settlement.ContextDigest)) ||
		timeErr != nil || parsed.Location() != time.UTC || parsed.UTC().Format(time.RFC3339Nano) != promotedAt {
		return errors.New("steer promotion settlement is invalid")
	}
	if strings.TrimSpace(queued.Status) != "queued" || queued.ProjectionVersion != SteerMessageProjectionVersionV1 ||
		queued.ContentDigest != SteerMessageContentDigestV1(queued) ||
		strings.TrimSpace(queued.ContextDigest) != strings.TrimSpace(settlement.ContextDigest) {
		return errors.New("steer promotion settlement does not match queued authority")
	}
	return nil
}

func SteerMessageMatchesPromotionAuthorityV1(current, expected SteerMessage) bool {
	return (strings.TrimSpace(current.Status) == "queued" || strings.TrimSpace(current.Status) == "admitted") &&
		strings.TrimSpace(expected.Status) == "queued" && SteerMessagesHaveSameContentV1(current, expected) &&
		strings.TrimSpace(current.QueueAuthorityDigest) == strings.TrimSpace(expected.QueueAuthorityDigest) &&
		strings.TrimSpace(current.CreatedAt) == strings.TrimSpace(expected.CreatedAt)
}

func NewSteerQueueAuthorityV1(record Record, contextDigest string, pendingUntilChildTurn bool) (SteerQueueAuthorityV1, error) {
	authority := SteerQueueAuthorityV1{
		Version:               SteerQueueAuthorityVersionV1,
		JobID:                 strings.TrimSpace(record.ID),
		RecordUpdatedAt:       strings.TrimSpace(record.UpdatedAt),
		Status:                strings.TrimSpace(record.Status),
		Background:            record.Background,
		ChildThreadID:         strings.TrimSpace(record.ChildThreadID),
		ChildTurnID:           strings.TrimSpace(record.ChildTurnID),
		SteerSetDigest:        SteerSetDigestV1(record.Steers),
		ContextDigest:         strings.TrimSpace(contextDigest),
		PendingUntilChildTurn: pendingUntilChildTurn,
	}
	if record.SecurityBinding != nil {
		authority.SecurityBindingDigest = strings.TrimSpace(record.SecurityBinding.BindingDigest)
	}
	if err := ValidateSteerQueueAuthorityForRecordV1(authority, record); err != nil {
		return SteerQueueAuthorityV1{}, err
	}
	return authority, nil
}

func ValidateSteerQueueAuthorityForRecordV1(authority SteerQueueAuthorityV1, record Record) error {
	bindingDigest := ""
	if record.SecurityBinding != nil {
		if ValidateSecurityBinding(record.SecurityBinding) != nil {
			return errors.New("steer queue security binding is invalid")
		}
		bindingDigest = strings.TrimSpace(record.SecurityBinding.BindingDigest)
	}
	if authority.Version != SteerQueueAuthorityVersionV1 || strings.TrimSpace(authority.JobID) == "" ||
		strings.TrimSpace(authority.RecordUpdatedAt) == "" || !authority.Background ||
		strings.TrimSpace(authority.ChildThreadID) == "" || !domainsecurity.IsSHA256Hex(strings.TrimSpace(authority.SteerSetDigest)) {
		return errors.New("steer queue authority is invalid")
	}
	switch strings.TrimSpace(authority.Status) {
	case string(StatusQueued), string(StatusRunning), string(StatusPauseRequested), string(StatusPaused), string(StatusResumeRequested):
	default:
		return errors.New("steer queue status is not active")
	}
	if authority.PendingUntilChildTurn {
		// The host may have reserved the exact first-turn identity before its
		// primary record is committed. Pending authority never gains a context.
		if strings.TrimSpace(authority.ContextDigest) != "" {
			return errors.New("pending steer queue authority is invalid")
		}
	} else if strings.TrimSpace(authority.ChildTurnID) == "" || !domainsecurity.IsSHA256Hex(strings.TrimSpace(authority.ContextDigest)) {
		return errors.New("bound steer queue authority is invalid")
	}
	if strings.TrimSpace(authority.JobID) != strings.TrimSpace(record.ID) ||
		strings.TrimSpace(authority.RecordUpdatedAt) != strings.TrimSpace(record.UpdatedAt) ||
		strings.TrimSpace(authority.Status) != strings.TrimSpace(record.Status) || authority.Background != record.Background ||
		strings.TrimSpace(authority.ChildThreadID) != strings.TrimSpace(record.ChildThreadID) ||
		strings.TrimSpace(authority.ChildTurnID) != strings.TrimSpace(record.ChildTurnID) ||
		strings.TrimSpace(authority.SecurityBindingDigest) != bindingDigest ||
		strings.TrimSpace(authority.SteerSetDigest) != SteerSetDigestV1(record.Steers) {
		return errors.New("steer queue authority does not match current job state")
	}
	return nil
}

func SteerSetDigestV1(messages []SteerMessage) string {
	payload, _ := json.Marshal(struct {
		Version  int            `json:"version"`
		Messages []SteerMessage `json:"messages"`
	}{Version: SteerQueueAuthorityVersionV1, Messages: messages})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func SteerQueueAuthorityDigestV1(authority SteerQueueAuthorityV1) string {
	payload, _ := json.Marshal(authority)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
