package event

import (
	"strings"
	"testing"

	display "analytix.local/runtime-go/internal/domain/processdisplay"
)

func privateHistoryAnchorFixtureV1(t *testing.T) map[string]any {
	t.Helper()
	a := display.SettledHistoryAnchorV1{SchemaVersion: display.SettledHistoryAnchorSchemaV1, ThreadID: "thread-history", TurnID: "turn-history", ContextDigest: strings.Repeat("a", 64), Terminal: display.TerminalWitnessV1{Status: "completed", FinishedAt: "2026-10-14T01:00:00Z", AuthorityDigest: strings.Repeat("b", 64), SourceDigest: strings.Repeat("c", 64)}, Availability: "available", RecordDigest: strings.Repeat("d", 64)}
	v, err := display.SettledHistoryAnchorMapV1(a)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestPrivateSettledHistoryAnchorSurvivesOnlyStrictPrimaryMetadataV1(t *testing.T) {
	a := privateHistoryAnchorFixtureV1(t)
	v := map[string]any{"id": "turn-history", display.SettledHistoryFieldV1: a}
	private, ok := SanitizeDurableValue(v, false)
	if !ok || private.(map[string]any)[display.SettledHistoryFieldV1] == nil {
		t.Fatal("strict Primary anchor dropped")
	}
	public, ok := SanitizePublicValue(v, false)
	if !ok || public.(map[string]any)[display.SettledHistoryFieldV1] != nil {
		t.Fatal("private anchor reached public/model copy")
	}
	for _, raw := range []any{v, map[string]any{"neutral": a}, map[string]any{"neutral": map[string]any{"schemaVersion": display.SettledHistorySchemaV1, "blocks": []any{}}}} {
		if ValidatePublicRecord(raw) == nil || !ContainsPrivateTerminalAuthority(raw) {
			t.Fatal("private purpose admitted under ordinary/neutral wrapper")
		}
	}
	a["body"] = "private source"
	if _, ok := SanitizeDurableValue(v, false); ok {
		t.Fatal("malformed private anchor normalized into admission")
	}
}
