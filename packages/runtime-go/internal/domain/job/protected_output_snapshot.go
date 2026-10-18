package job

import (
	"errors"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// This private reference contains no output text and grants no read authority.
// The existing protected tool-result CAS owns the producer's immutable bytes.
// Readers must validate the envelope against the original Job and current scope.
type ProtectedOutputSnapshotV1 struct {
	Binding       domaintoolresult.ProtectedSnapshotBindingV1 `json:"binding"`
	ChildThreadID string                                      `json:"childThreadId,omitempty"`
	ChildTurnID   string                                      `json:"childTurnId,omitempty"`
}

func CloneProtectedOutputSnapshotV1(value *ProtectedOutputSnapshotV1) *ProtectedOutputSnapshotV1 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func ValidateProtectedOutputSnapshotV1(record Record) error {
	value := record.ProtectedOutputSnapshot
	if value == nil {
		return nil // Older or unavailable producers have no historical body.
	}
	if ValidateSecurityBinding(record.SecurityBinding) != nil || !TerminalStatusV1(record.Status) ||
		record.Output != "" || value.ChildThreadID != record.ChildThreadID || value.ChildTurnID != record.ChildTurnID {
		return errors.New("protected Job output reference does not match its producer")
	}
	if _, err := domaintoolresult.ParseProtectedSnapshotBindingV1(value.Binding); err != nil {
		return errors.New("protected Job output snapshot binding is invalid")
	}
	switch record.Kind {
	case "subagent":
		if record.Status != string(StatusCompleted) || value.ChildThreadID == "" || value.ChildTurnID == "" {
			return errors.New("protected child output requires an exact completed turn")
		}
	case "background-shell":
		if value.ChildThreadID != "" || value.ChildTurnID != "" {
			return errors.New("protected shell output cannot carry a child turn")
		}
	default:
		return errors.New("protected Job output producer kind is invalid")
	}
	return nil
}
