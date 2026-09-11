package security

import (
	"encoding/json"
	"errors"
	"time"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

const ActiveInheritedHistoryPurposeV1 = "analytix.active-inherited-history/v1"

// ActiveInheritedHistoryBindingV1 authenticates the origin and complete ordered
// prefix of inert derived history. It grants no execution or publication authority.
type ActiveInheritedHistoryBindingV1 struct {
	SchemaVersion               int                     `json:"schemaVersion"`
	Purpose                     string                  `json:"purpose"`
	SourceThreadID              string                  `json:"sourceThreadId"`
	SourcePrimarySHA256         string                  `json:"sourcePrimarySha256"`
	SourceAuthorityRecordDigest string                  `json:"sourceAuthorityRecordDigest"`
	TargetThreadID              string                  `json:"targetThreadId"`
	Derivation                  string                  `json:"derivation"`
	CutoffTurnID                string                  `json:"cutoffTurnId"`
	SourceTurnCount             int                     `json:"sourceTurnCount"`
	TargetCreatedAt             string                  `json:"targetCreatedAt"`
	TargetRelation              string                  `json:"targetRelation"`
	Turns                       []ActiveInheritedTurnV1 `json:"turns"`
	InventoryDigest             string                  `json:"inventoryDigest"`
}

type ActiveInheritedTurnV1 struct {
	TurnID        string `json:"turnId"`
	ContentSHA256 string `json:"contentSha256"`
}

func ValidateActiveInheritedHistoryBindingV1(binding ActiveInheritedHistoryBindingV1) error {
	invalid := func() error { return errors.New("active inherited history binding is invalid") }
	if binding.SchemaVersion != 1 || binding.Purpose != ActiveInheritedHistoryPurposeV1 ||
		!domainthread.IsCanonicalRecordID(binding.SourceThreadID) || !domainthread.IsCanonicalRecordID(binding.TargetThreadID) ||
		binding.SourceThreadID == binding.TargetThreadID || !IsSHA256Hex(binding.SourcePrimarySHA256) ||
		!IsSHA256Hex(binding.SourceAuthorityRecordDigest) || binding.Turns == nil || binding.SourceTurnCount < len(binding.Turns) ||
		!IsSHA256Hex(binding.InventoryDigest) || binding.InventoryDigest != ActiveInheritedHistoryInventoryDigestV1(binding.Turns) {
		return invalid()
	}
	createdAt, err := time.Parse(time.RFC3339Nano, binding.TargetCreatedAt)
	if err != nil || createdAt.IsZero() || createdAt.UTC().Format(time.RFC3339Nano) != binding.TargetCreatedAt {
		return invalid()
	}
	switch binding.Derivation {
	case "fork":
		if binding.TargetRelation != "fork" && binding.TargetRelation != "side" {
			return invalid()
		}
	case "resume":
		if binding.TargetRelation != "primary" || binding.SourceTurnCount != len(binding.Turns) {
			return invalid()
		}
	default:
		return invalid()
	}
	seen := make(map[string]bool, len(binding.Turns))
	for _, turn := range binding.Turns {
		if !domainthread.IsCanonicalRecordID(turn.TurnID) || !IsSHA256Hex(turn.ContentSHA256) || seen[turn.TurnID] {
			return invalid()
		}
		seen[turn.TurnID] = true
	}
	if len(binding.Turns) == 0 {
		if binding.CutoffTurnID != "" {
			return invalid()
		}
	} else if binding.CutoffTurnID != binding.Turns[len(binding.Turns)-1].TurnID {
		return invalid()
	}
	return nil
}

// ActiveInheritedHistoryInventoryDigestV1 preserves order and binds every exact
// turn digest. A nil inventory is unavailable, distinct from an empty prefix.
func ActiveInheritedHistoryInventoryDigestV1(turns []ActiveInheritedTurnV1) string {
	if turns == nil {
		return ""
	}
	body, err := json.Marshal(turns)
	if err != nil {
		return ""
	}
	return SHA256Hex(append([]byte(ActiveInheritedHistoryPurposeV1+"\x00"), body...))
}

func CloneActiveInheritedHistoryBindingV1(binding *ActiveInheritedHistoryBindingV1) *ActiveInheritedHistoryBindingV1 {
	if binding == nil {
		return nil
	}
	cloned := *binding
	if binding.Turns != nil {
		cloned.Turns = append([]ActiveInheritedTurnV1{}, binding.Turns...)
	}
	return &cloned
}
