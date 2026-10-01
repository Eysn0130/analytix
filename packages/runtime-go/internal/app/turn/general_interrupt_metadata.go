package turn

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

// CommittedInterruptMetadataV1 is the host-authored interrupt projection
// bound to a validated ordinary terminal CAS/outbox/archive winner.
type CommittedInterruptMetadataV1 struct {
	Discard               bool
	Cancelled             bool
	CancelledPendingGates int
}

// ResolveCommittedGeneralInterruptMetadataV1 resolves interrupt response
// metadata only from the canonical ordinary terminal outbox. The duplicated
// turn fields are projections and must match the outbox exactly.
func ResolveCommittedGeneralInterruptMetadataV1(
	thread map[string]any,
	turnID string,
) (CommittedInterruptMetadataV1, error) {
	turnID = strings.TrimSpace(turnID)
	authorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if err != nil {
		return CommittedInterruptMetadataV1{}, err
	}
	authority, ok := authorities[turnID]
	if !ok || !authority.Governed || !authority.Terminal ||
		authority.Commit.TerminalReason != "cancel" || authority.Commit.TerminalStatus != "aborted" {
		return CommittedInterruptMetadataV1{}, errors.New("general interrupt terminal authority is unavailable")
	}
	metadata, err := interruptMetadataFromRecordV1(authority.Commit.TerminalEvent)
	if err != nil {
		return CommittedInterruptMetadataV1{}, err
	}
	turn, found := securityTurnByID(thread, turnID)
	if !found {
		return CommittedInterruptMetadataV1{}, errors.New("general interrupt terminal turn is unavailable")
	}
	projection, err := interruptMetadataFromRecordV1(turn)
	if err != nil || projection != metadata {
		return CommittedInterruptMetadataV1{}, errors.New("general interrupt metadata projection is inconsistent")
	}
	return metadata, nil
}

func interruptMetadataFromRecordV1(record map[string]any) (CommittedInterruptMetadataV1, error) {
	if record == nil {
		return CommittedInterruptMetadataV1{}, errors.New("interrupt metadata is unavailable")
	}
	discard, discardOK := record["discard"].(bool)
	cancelled, cancelledOK := record["cancelled"].(bool)
	count, countOK := exactNonNegativeInterruptCountV1(record["cancelledPendingGates"])
	if !discardOK || !cancelledOK || !countOK || count > 0 && !cancelled {
		return CommittedInterruptMetadataV1{}, errors.New("interrupt metadata is invalid")
	}
	return CommittedInterruptMetadataV1{
		Discard: discard, Cancelled: cancelled, CancelledPendingGates: count,
	}, nil
}

func exactNonNegativeInterruptCountV1(value any) (int, bool) {
	var count int64
	switch typed := value.(type) {
	case int:
		return typed, typed >= 0
	case int64:
		count = typed
	case float64:
		if math.IsNaN(typed) || typed < 0 || typed >= -float64(math.MinInt) || math.Trunc(typed) != typed {
			return 0, false
		}
		count = int64(typed)
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		count = parsed
	default:
		return 0, false
	}
	if count < 0 || count > math.MaxInt {
		return 0, false
	}
	return int(count), true
}
