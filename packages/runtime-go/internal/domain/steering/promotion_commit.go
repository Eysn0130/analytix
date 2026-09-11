package steering

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const promotionCommitDomainV1 = "analytix/steering-promotion-commit/v1\x00"

// PromotionCommitV1 is the canonical local admission result for one steering
// message. It proves only that the host committed the exact entry/item pair to
// the frozen turn; it does not prove provider transport or model execution.
type PromotionCommitV1 struct {
	Version             int
	CommitID            string
	ThreadID            string
	TurnID              string
	ContextDigest       string
	EntryID             string
	ClientUserMessageID string
	PromotedAt          string
	Origin              string
	JobID               string
	SteerMessageID      string
	Entry               map[string]any
	Item                map[string]any
}

func NewPromotionCommitV1(
	threadID, turnID, contextDigest string,
	entry, item map[string]any,
	verify AuthorityVerifierV1,
) (PromotionCommitV1, error) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	contextDigest = strings.TrimSpace(contextDigest)
	if threadID == "" || turnID == "" || !domainsecurity.IsSHA256Hex(contextDigest) || verify == nil ||
		ValidatePromotedItemForContextV1(entry, item, threadID, turnID, contextDigest) != nil ||
		verify(entry, contextDigest) != nil {
		return PromotionCommitV1{}, errors.New("steering promotion commit authority is invalid")
	}
	content, err := parseEntryContentV1(entry, promotedFieldsV1)
	if err != nil {
		return PromotionCommitV1{}, errors.New("steering promotion commit content is invalid")
	}
	canonical := map[string]any{
		"version": ProjectionVersionV1, "threadId": threadID, "turnId": turnID,
		"contextDigest": contextDigest, "entry": cloneSteeringMapV1(entry), "item": cloneSteeringMapV1(item),
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return PromotionCommitV1{}, errors.New("steering promotion commit payload is invalid")
	}
	digest := sha256.Sum256(append([]byte(promotionCommitDomainV1), payload...))
	origin := "ordinary"
	if content.JobID != "" {
		origin = "task_job"
	}
	return PromotionCommitV1{
		Version: ProjectionVersionV1, CommitID: "steer_commit_" + hex.EncodeToString(digest[:]),
		ThreadID: threadID, TurnID: turnID, ContextDigest: contextDigest,
		EntryID: content.ID, ClientUserMessageID: content.ClientUserMessageID,
		PromotedAt: stringField(entry, "promotedAt"), Origin: origin,
		JobID: content.JobID, SteerMessageID: content.SteerMessageID,
		Entry: cloneSteeringMapV1(entry), Item: cloneSteeringMapV1(item),
	}, nil
}
