package toolresult

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func searchSnapshotFixtureV1(t *testing.T) ProtectedSnapshotV1 {
	snapshot := protectedWebFixtureV1(t, "")
	snapshot.ToolName = "grep"
	total := 35
	search := &ProtectedSearchCaptureV1{Shape: "matches", Total: &total, InputJSON: `{"pattern":"SOURCE"}`, StartedAt: "2026-10-08T00:00:00Z"}
	file := ProtectedSearchFileV1{Path: "/synthetic/source.go"}
	for i := 1; i <= total; i++ {
		file.Matches = append(file.Matches, ProtectedSearchMatchV1{LineNumber: i, Line: fmt.Sprintf("SOURCE_%d", i), Column: 1, ContextBefore: []string{}, ContextAfter: []string{}})
	}
	search.Files = []ProtectedSearchFileV1{file}
	snapshot.Capture = ProtectedCaptureV1{Kind: "search", Status: "completed", Label: "grep", Search: search, Body: ProtectedSearchTextV1(*search)}
	return snapshot
}
func TestProtectedSearchSnapshotCanonicalManyMatchesAndCompatibilityV1(t *testing.T) {
	snapshot := searchSnapshotFixtureV1(t)
	body, err := ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProtectedSnapshotV1(body)
	if err != nil || len(parsed.Capture.Search.Files[0].Matches) != 35 {
		t.Fatal("real typed array rejected", err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(body, []byte(`"total":35`), []byte(`"total":35,"total":35`), 1),
		bytes.Replace(body, []byte(`"shape":"matches"`), []byte(`"shape":"matches","spill":"invented"`), 1),
	} {
		if _, err := ParseProtectedSnapshotV1(invalid); err == nil {
			t.Fatal("malformed immutable snapshot accepted")
		}
	}
	old := protectedWebFixtureV1(t, "original")
	oldBytes, err := ProtectedSnapshotBytesV1(old)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(oldBytes, []byte(`"search"`)) {
		t.Fatal("old canonical bytes gained search data")
	}
	if _, err := ParseProtectedSnapshotV1(oldBytes); err != nil {
		t.Fatal("old Web snapshot no longer reads", err)
	}
	binding, err := ProtectedSnapshotBindingForV1(snapshot)
	if err != nil || len(binding.SnapshotDigest) != 64 {
		t.Fatal("search has no real immutable locator", err)
	}
}
func TestProtectedSearchCaptureRejectsCountBodyCrossKindAndOwnsArraysV1(t *testing.T) {
	capture := searchSnapshotFixtureV1(t).Capture
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, capture)
	capture.Search.Files[0].Matches[0].Line = "MUTATED"
	*capture.Search.Total = 999
	got, ok := ConsumeProtectedToolResultV1(ctx)
	if !ok || strings.Contains(got.Body, "MUTATED") || got.Search.Files[0].Matches[0].Line != "SOURCE_1" || *got.Search.Total != 35 {
		t.Fatal("collector kept producer-owned mutable arrays")
	}
	for _, mutate := range []func(*ProtectedCaptureV1){
		func(c *ProtectedCaptureV1) { c.Body = "forged body" },
		func(c *ProtectedCaptureV1) { c.Search.Truncated = true },
		func(c *ProtectedCaptureV1) { c.Search.Files[0].Matches[0].Column = 0 },
		func(c *ProtectedCaptureV1) { c.Kind = "read" },
		func(c *ProtectedCaptureV1) { c.Search.Total = nil },
		func(c *ProtectedCaptureV1) { *c.Search.Total = 36 },
		func(c *ProtectedCaptureV1) { c.Search.Files[0].Matches[0].ContextBefore = nil },
	} {
		value := searchSnapshotFixtureV1(t).Capture
		mutate(&value)
		if ValidateProtectedCaptureV1(value) == nil {
			t.Fatal("invalid search accepted")
		}
	}
	unknown := searchSnapshotFixtureV1(t)
	unknown.Capture.Search.Total = nil
	unknown.Capture.Search.Truncated = true
	encoded, err := ProtectedSnapshotBytesV1(unknown)
	if err != nil || !bytes.Contains(encoded, []byte(`"total":null`)) {
		t.Fatal("unknown total must survive canonical serialization", err)
	}
	unknown.ToolName = "glob"
	if ValidateProtectedSnapshotV1(unknown) == nil {
		t.Fatal("mismatched tool/shape accepted")
	}
}

func TestProtectedSearchRejectsMoreThanProducerGrepBudgetV1(t *testing.T) {
	capture := searchSnapshotFixtureV1(t).Capture
	matches := []ProtectedSearchMatchV1{}
	for i := 1; i <= 501; i++ {
		matches = append(matches, ProtectedSearchMatchV1{LineNumber: i, Line: "x", Column: 1, ContextBefore: []string{}, ContextAfter: []string{}})
	}
	capture.Search.Files[0].Matches = matches
	total := 501
	capture.Search.Total = &total
	capture.Body = ProtectedSearchTextV1(*capture.Search)
	if ValidateProtectedCaptureV1(capture) == nil {
		t.Fatal("domain widened the grep producer budget")
	}
}

func TestProtectedSearchRequestFactsHaveOneOwnerAndLegacyMissingStaysInvalidV1(t *testing.T) {
	legacy := searchSnapshotFixtureV1(t)
	legacyBytes, err := ProtectedSnapshotBytesV1(legacy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProtectedSnapshotV1(legacyBytes)
	if err != nil || parsed.Inspect != nil || parsed.Capture.Search.InputJSON != legacy.Capture.Search.InputJSON {
		t.Fatal("legacy request facts were changed", err)
	}
	current := searchSnapshotFixtureV1(t)
	current.Inspect = &ProtectedInspectV1{InputJSON: current.Capture.Search.InputJSON}
	current.Capture.Search.InputJSON = ""
	currentBytes, err := ProtectedSnapshotBytesV1(current)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = ParseProtectedSnapshotV1(currentBytes)
	if err != nil || parsed.Inspect == nil || parsed.Capture.Search.InputJSON != "" || bytes.Count(currentBytes, []byte(`"inputJSON"`)) != 1 {
		t.Fatal("new request facts have duplicate owners", err)
	}
	current.Inspect.InputJSON = ""
	if ValidateProtectedSnapshotV1(current) == nil {
		t.Fatal("empty Inspect relaxed missing legacy input")
	}
	current.Inspect.InputJSON = legacy.Capture.Search.InputJSON
	current.Capture.Search.InputJSON = legacy.Capture.Search.InputJSON
	if ValidateProtectedSnapshotV1(current) == nil {
		t.Fatal("mixed historical request owners accepted")
	}
	legacy.Capture.Search.InputJSON = ""
	if ValidateProtectedSnapshotV1(legacy) == nil {
		t.Fatal("missing legacy input was relaxed")
	}
	legacy.Capture.Search.InputTruncated = true
	if ValidateProtectedSnapshotV1(legacy) != nil {
		t.Fatal("legacy honest truncation no longer reads")
	}
}
