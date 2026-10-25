package processdisplay

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	identity "analytix.local/runtime-go/internal/domain/identity"
	"analytix.local/runtime-go/internal/domain/secretprojection"
	security "analytix.local/runtime-go/internal/domain/security"
)

func settledRecordFixtureV1(t *testing.T) SettledHistoryV1 {
	t.Helper()
	p, err := identity.NewPrincipalV1(strings.Repeat("1", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	original := security.NewTurnSecurityContext(security.TurnSecurityContextInput{ThreadID: "thread-history", TurnID: "turn-history", WorkspaceRealPath: "/synthetic/workspace", TenantID: "local", UserID: "local"})
	v := SettledHistoryV1{SchemaVersion: SettledHistorySchemaV1, ThreadID: original.ThreadID, TurnID: original.TurnID, Principal: p, Original: original, Terminal: TerminalWitnessV1{Status: "completed", FinishedAt: "2026-10-14T01:00:00Z", AuthorityDigest: strings.Repeat("a", 64), SourceDigest: security.SHA256Hex([]byte("[]"))}, Units: []UnitV2{{UnitID: "unit-history", ProducerID: "producer-history", ProviderSequence: "1", Placement: "unit_only", Calls: []CallAnchorV2{}, PrecedingCalls: []CallAnchorV2{}}}, Blocks: []SettledBlockV1{{Display: BlockV1{BlockID: strings.Repeat("2", 32), Kind: "native_reasoning", State: "completed", Attempt: 1, Source: SourceV2{Kind: "provider_reasoning", UnitID: "unit-history", ProducerID: "producer-history", ProviderSequence: "1", Attempt: 1, SourceOrdinal: "1", ReasoningForm: "summary"}, Classification: "source_exact_local", Body: &BodyV1{Text: "Alice\r\nsummary"}}}}}
	v.Blocks[0].Display.CaptureDigest = CaptureDigestV1(v.Blocks[0].Display)
	return v
}

func TestSettledHistoryStrictCodecBindsOriginalPrincipalAndSummaryV1(t *testing.T) {
	v := settledRecordFixtureV1(t)
	policy, err := security.NewTurnPublicationPolicyV1(security.TurnPublicationPolicyInputV1{ThreadRiskPolicyDigest: strings.Repeat("b", 64), RiskClass: security.RiskClassCase, Disposition: security.PublicationDispositionCaseBoundaryOnly, CaseBindingState: security.CaseBindingStateInvalid, BindingObservationDigest: strings.Repeat("c", 64), BlockerCode: security.PublicationBlockerCaseBindingInvalid})
	if err != nil {
		t.Fatal(err)
	}
	v.Original, err = security.NewTurnSecurityContextV2(security.TurnSecurityContextInput{ThreadID: v.ThreadID, TurnID: v.TurnID, WorkspaceRealPath: "/synthetic/workspace", TenantID: "local", UserID: "local", CaseID: security.UnboundCaseID, CaseBindingHash: security.UnboundCaseBindingHash("/synthetic/workspace"), DatasetSnapshotID: security.NoDatasetSnapshotID, SourceManifestHash: security.EmptySourceManifestHash, ContextEpoch: 3, IssuedAt: time.Date(2026, 10, 14, 1, 0, 0, 0, time.UTC), PublicationPolicy: policy, RiskAuthorityBinding: security.NewQuarantinedRiskAuthorityBindingV1()})
	if err != nil {
		t.Fatal(err)
	}
	body, err := SettledHistoryBytesV1(v)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSettledHistoryV1(body)
	if err != nil || !reflect.DeepEqual(got, v) || got.Original.Version != 2 || got.Blocks[0].Display.Source.ReasoningForm != "summary" {
		t.Fatal("strict original authority/summary roundtrip failed", err)
	}
	a, err := SettledHistoryAnchorForV1(v)
	if err != nil {
		t.Fatal(err)
	}
	v.Blocks[0].Display.Body.Text += " changed"
	v.Blocks[0].Display.CaptureDigest = CaptureDigestV1(v.Blocks[0].Display)
	b, err := SettledHistoryAnchorForV1(v)
	if err != nil || a.RecordDigest == b.RecordDigest {
		t.Fatal("body not bound by record digest")
	}
	for _, malformed := range [][]byte{append([]byte(" "), body...), bytes.Replace(body, []byte(`"schemaVersion":`), []byte(`"unknown":true,"schemaVersion":`), 1), bytes.Replace(body, []byte(`"threadId":"thread-history"`), []byte(`"threadId":"thread-history","threadId":"thread-history"`), 1)} {
		if _, err := ParseSettledHistoryV1(malformed); err == nil {
			t.Fatal("noncanonical/extra/duplicate record admitted")
		}
	}
}

func TestSettledHistoryRejectsLiveOrMisboundSourceV1(t *testing.T) {
	// Local synthetic test fixture; never sourced from an account.
	const syntheticSettledCredentialV1 = "sk-0123456789abcdef0123456789abcdef"
	credentialInput := "Authorization: Bearer " + syntheticSettledCredentialV1
	if !secretprojection.ContainsCredentialV1(credentialInput) {
		t.Fatal("synthetic credential fixture no longer exercises API-token detection")
	}
	for name, mutate := range map[string]func(*SettledHistoryV1){
		"running":     func(v *SettledHistoryV1) { v.Blocks[0].Display.State = "running" },
		"draft bytes": func(v *SettledHistoryV1) { v.Blocks[0].Display.DraftBytes = "4" },
		"input": func(v *SettledHistoryV1) {
			v.Blocks[0].Display.Input = &InputV2{Availability: "available", InputJSON: `{}`}
		},
		"input count":        func(v *SettledHistoryV1) { n := int64(-1); v.Blocks[0].Display.InputUtf16CodeUnits = &n },
		"source index":       func(v *SettledHistoryV1) { v.Blocks[0].Display.Source.NativeIndex = -1 },
		"zero preceding ref": func(v *SettledHistoryV1) { v.Units[0].PrecedingCalls = []CallAnchorV2{{}} },
		"wrong unit":         func(v *SettledHistoryV1) { v.Blocks[0].Display.Source.UnitID = "other" },
		"wrong principal":    func(v *SettledHistoryV1) { v.Principal.PrincipalDigest = strings.Repeat("f", 64) },
		"body limit": func(v *SettledHistoryV1) {
			v.Blocks[0].Display.Body.Text = strings.Repeat("x", CaptureBodyByteLimitV1+1)
		},
		"credentials": func(v *SettledHistoryV1) {
			v.Blocks[0].Display.Body.Text = credentialInput
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := settledRecordFixtureV1(t)
			mutate(&v)
			if _, err := SettledHistoryBytesV1(v); err == nil {
				t.Fatal("invalid settled source admitted")
			}
		})
	}
	anchor, err := SettledHistoryAnchorForV1(settledRecordFixtureV1(t))
	if err != nil {
		t.Fatal(err)
	}
	value, _ := SettledHistoryAnchorMapV1(anchor)
	value["body"] = "private"
	if _, err := ParseSettledHistoryAnchorV1(value); err == nil {
		t.Fatal("anchor accepted private body")
	}
}
