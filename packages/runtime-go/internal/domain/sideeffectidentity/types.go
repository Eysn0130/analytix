package sideeffectidentity

import (
	"encoding/hex"
	"errors"
	"strings"
)

const SchemaVersionV1 = 1

// IdentityV1 is a host-derived, argument-free semantic side-effect identity.
// Raw arguments, paths, PII, and tool output never cross this domain value.
type IdentityV1 struct {
	SchemaVersion int
	ToolName      string
	ArgsHash      string
}

func ValidateV1(identity IdentityV1) error {
	if identity.SchemaVersion != SchemaVersionV1 || strings.TrimSpace(identity.ToolName) == "" || identity.ToolName != strings.TrimSpace(identity.ToolName) {
		return errors.New("side-effect semantic identity is invalid")
	}
	if len(identity.ArgsHash) != 64 || strings.ToLower(identity.ArgsHash) != identity.ArgsHash {
		return errors.New("side-effect semantic identity hash is invalid")
	}
	decoded, err := hex.DecodeString(identity.ArgsHash)
	if err != nil || len(decoded) != 32 {
		return errors.New("side-effect semantic identity hash is invalid")
	}
	return nil
}

func CanonicalToolNameV1(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "write", "write_file":
		return "write_file"
	case "edit", "edit_file", "multi_edit":
		return "edit_file"
	case "delegate_task", "task":
		return "task"
	case "todo_patch", "todo_ops":
		return "todo_ops"
	default:
		return strings.TrimSpace(toolName)
	}
}
