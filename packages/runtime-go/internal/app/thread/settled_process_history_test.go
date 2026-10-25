package thread

import (
	"encoding/json"
	"strings"
	"testing"

	display "analytix.local/runtime-go/internal/domain/processdisplay"
	security "analytix.local/runtime-go/internal/domain/security"
)

func TestPrimarySettledHistoryAnchorIsStrictAndForkRetiresItV1(t *testing.T) {
	original := security.NewTurnSecurityContext(security.TurnSecurityContextInput{ThreadID: "thread-history", TurnID: "turn-history", WorkspaceRealPath: "/synthetic/workspace", TenantID: "local", UserID: "local"})
	a := display.SettledHistoryAnchorV1{SchemaVersion: display.SettledHistoryAnchorSchemaV1, ThreadID: original.ThreadID, TurnID: original.TurnID, ContextDigest: original.ContextDigest, Terminal: display.TerminalWitnessV1{Status: "completed", FinishedAt: "2026-10-14T01:00:00Z", AuthorityDigest: strings.Repeat("b", 64), SourceDigest: strings.Repeat("c", 64)}, Availability: "unavailable", Reason: "capacity"}
	ref, err := display.SettledHistoryAnchorMapV1(a)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var contextRecord map[string]any
	if err := json.Unmarshal(body, &contextRecord); err != nil {
		t.Fatal(err)
	}
	turn := map[string]any{"id": original.TurnID, "threadId": original.ThreadID, "status": "completed", "finishedAt": a.Terminal.FinishedAt, "securityContext": contextRecord, "items": []any{}, display.SettledHistoryFieldV1: ref}
	node := map[string]any{"id": original.ThreadID, "workspace": "/synthetic/workspace", "turns": []any{turn}}
	private, err := SanitizeDurableHistory(node)
	if err != nil {
		t.Fatal(err)
	}
	if private["turns"].([]any)[0].(map[string]any)[display.SettledHistoryFieldV1] == nil {
		t.Fatal("capacity fact lost from Primary")
	}
	stripDerivedExecutionAuthorityFields(turn)
	if turn[display.SettledHistoryFieldV1] != nil {
		t.Fatal("fork/resume copied source turn authority anchor")
	}
}
