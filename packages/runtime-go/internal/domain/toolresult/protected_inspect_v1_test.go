package toolresult

import (
	"strings"
	"testing"
)

func TestProtectedErrorPreservesSourceAndCredentialOnlyFloorV1(t *testing.T) {
	message := "  Alice alice@example.invalid\r\nnumber=9007199254740993\n"
	error_ := ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: message}})
	if ValidateProtectedOutputErrorV1(error_) != nil || error_.Entries[0].Message != message || error_.Entries[0].Classification != "source_exact_local" {
		t.Fatal("private source error lost whitespace or case truth")
	}
	error_ = ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: message + "api_key=synthetic-private-key"}})
	if error_.Availability != "available" || error_.Entries[0].Classification != "credential_redacted" || strings.Contains(error_.Entries[0].Message, "synthetic-private-key") || !strings.Contains(error_.Entries[0].Message, "Alice") {
		t.Fatal("error bypassed the production credential floor")
	}
	for _, message := range []string{"", " \r\n", "invalid\x00source", string([]byte{255}), strings.Repeat("x", ProtectedSnapshotBodyLimitV1+1)} {
		if value := ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: message}}); value.Availability != "unavailable" || len(value.Entries) != 0 {
			t.Fatal("invalid error was fabricated or prefix-truncated")
		}
	}
	if value := ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "actual"}}, make([]string, 65)...); value.Availability != "unavailable" {
		t.Fatal("closed secret scope delivered an error")
	}
}

func TestProtectedParallelErrorKeepsActualOneBasedIndicesAndOwnsDisplayCopyV1(t *testing.T) {
	first, third := 1, 3
	value := ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "first actual failure", ParallelIndex: &first}, {Message: "third actual failure", ParallelIndex: &third}})
	if ValidateProtectedOutputErrorV1(value) != nil || *value.Entries[1].ParallelIndex != 3 {
		t.Fatal("missing middle child shifted the actual protocol index")
	}
	request := &ProtectedInspectV1{OutputError: value}
	display := InspectWithEventTimesV1(request, "", "")
	display.OutputError.Entries[0].Message = "mutated"
	*display.OutputError.Entries[1].ParallelIndex = 2
	if request.OutputError.Entries[0].Message == "mutated" || *request.OutputError.Entries[1].ParallelIndex != 3 {
		t.Fatal("display timing borrowed sealed source errors")
	}
	zero := 0
	for _, entries := range [][]ProtectedOutputErrorEntryV1{{{Message: "bad", Classification: "source_exact_local", ParallelIndex: &zero}}, {{Message: "a", Classification: "source_exact_local", ParallelIndex: &first}, {Message: "b", Classification: "source_exact_local", ParallelIndex: &first}}, {{Message: "a", Classification: "source_exact_local"}, {Message: "b", Classification: "source_exact_local", ParallelIndex: &third}}} {
		if ValidateProtectedOutputErrorV1(&ProtectedOutputErrorV1{Availability: "available", Entries: entries}) == nil {
			t.Fatal("ambiguous parallel ownership admitted")
		}
	}
}

func TestInspectDurationUsesEventPairAndRetainsUnknownFactsV1(t *testing.T) {
	request := &ProtectedInspectV1{InputJSON: `{"actual":"input"}`, SchemaJSON: `{"name":"read","description":"actual request","parameters":{"type":"object"}}`}
	display := InspectWithEventTimesV1(request, "2026-10-10T00:00:00.000999999Z", "2026-10-10T00:00:02.000000001Z")
	if ValidateProtectedInspectV1(display) != nil || display.DurationMs == nil || *display.DurationMs != 1999 {
		t.Fatal("duration did not use the exact event time difference")
	}
	if request.DurationMs != nil || request.CallEventAt != nil || request.ResultEventAt != nil {
		t.Fatal("display timing mutated the sealed request")
	}
	running := InspectWithEventTimesV1(request, "2026-10-10T00:00:00Z", "")
	if running.CallEventAt == nil || running.ResultEventAt != nil || running.DurationMs != nil || ValidateProtectedInspectV1(running) != nil {
		t.Fatal("running call received a duration")
	}
	unknown := InspectWithEventTimesV1(&ProtectedInspectV1{}, "", "")
	if unknown.InputJSON != "" || unknown.SchemaJSON != "" || unknown.DurationMs != nil || InspectWithEventTimesV1(nil, "2026-10-10T00:00:00Z", "2026-10-10T00:00:01Z") != nil {
		t.Fatal("historical missing facts were backfilled")
	}
	wrong := int64(2000)
	display.DurationMs = &wrong
	if ValidateProtectedInspectV1(display) == nil {
		t.Fatal("forged duration passed")
	}
}

func TestInspectRejectsPartialJSONAndMixedTruncationV1(t *testing.T) {
	for _, value := range []ProtectedInspectV1{{InputJSON: `[]`}, {InputJSON: `{"broken":`}, {InputJSON: `{}`, InputTruncated: true}, {SchemaJSON: `{}`, SchemaTruncated: true}} {
		if ValidateProtectedInspectV1(&value) == nil {
			t.Fatal("invalid inspect request passed")
		}
	}
}

func TestProtectedSuccessTextProjectsBeforeUTF8PrefixAndRemainsLiteralV1(t *testing.T) {
	if ProjectProtectedOutputTextV1(" \r\n") != nil {
		t.Fatal("empty text acquired OUT")
	}
	original := "  {\"n\":9007199254740993}\r\n中\n"
	value := ProjectProtectedOutputTextV1(original)
	if ValidateProtectedOutputTextV1(value) != nil || value.Body != original || value.Truncated || value.Classification != "source_exact_local" {
		t.Fatal("literal text was parsed or normalized")
	}
	secret := "synthetic-opaque-request-value-9264"
	boundary := strings.Repeat("x", ProtectedSnapshotBodyLimitV1-6) + secret + strings.Repeat("中", 8)
	value = ProjectProtectedOutputTextV1(boundary, secret)
	if value.Availability != "available" || !value.Truncated || value.Classification != "credential_redacted" || strings.Contains(value.Body, "synthetic") || strings.Contains(value.Body, secret) {
		t.Fatal("split credential entered a prefix")
	}
	value = ProjectProtectedOutputTextV1(strings.Repeat("中", ProtectedSnapshotBodyLimitV1))
	if value.Availability != "available" || len(value.Body) > ProtectedSnapshotBodyLimitV1 || len(value.Body)%3 != 0 || !value.Truncated {
		t.Fatal("UTF-8 display byte cap failed")
	}
	for _, test := range []struct{ text, reason string }{{strings.Repeat("x", ProtectedOutputTextScanLimitV1+1), "limit"}, {"bad\x00text", "invalid"}, {string([]byte{255}), "invalid"}} {
		got := ProjectProtectedOutputTextV1(test.text)
		if got.Availability != "unavailable" || got.Reason != test.reason || got.Body != "" {
			t.Fatal("scan failure became readable body")
		}
	}
	if got := ProjectProtectedOutputTextV1("actual", make([]string, 65)...); got.Availability != "unavailable" {
		t.Fatal("closed credential registry was ignored")
	}
	sealed := &ProtectedInspectV1{OutputText: ProjectProtectedOutputTextV1(original)}
	display := InspectWithEventTimesV1(sealed, "", "")
	display.OutputText.Body = "changed"
	if sealed.OutputText.Body != original {
		t.Fatal("timing display borrowed immutable text")
	}
	sealed.OutputError = ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "actual error"}})
	if ValidateProtectedInspectV1(sealed) == nil {
		t.Fatal("successful OUT and error were merged")
	}
}
