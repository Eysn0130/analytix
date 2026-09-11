package subagent

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestParentTodosDigestUsesFullStableSHA256(t *testing.T) {
	todos := map[string]any{
		"updatedAt": "2026-07-18T00:00:00Z",
		"items": []any{
			map[string]any{"id": "todo-1", "content": "verify receipt", "status": "pending"},
		},
	}

	first := ParentTodosDigest(todos)
	second := ParentTodosDigest(todos)
	if first != second {
		t.Fatalf("expected stable digest, got %q and %q", first, second)
	}
	encoded := strings.TrimPrefix(first, "sha256:")
	if encoded == first || len(encoded) != 64 {
		t.Fatalf("expected full sha256 digest, got %q", first)
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		t.Fatalf("expected hexadecimal sha256 digest, got %q: %v", first, err)
	}
}

func TestParentTodosDigestChangesWithTodoState(t *testing.T) {
	pending := map[string]any{
		"items": []any{
			map[string]any{"id": "todo-1", "content": "verify receipt", "status": "pending"},
		},
	}
	completed := map[string]any{
		"items": []any{
			map[string]any{"id": "todo-1", "content": "verify receipt", "status": "completed"},
		},
	}
	if ParentTodosDigest(pending) == ParentTodosDigest(completed) {
		t.Fatal("expected todo state change to change parent digest")
	}
}
