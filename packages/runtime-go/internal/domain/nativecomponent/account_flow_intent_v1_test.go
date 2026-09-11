package nativecomponent

import (
	"encoding/json"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAnalyzeAccountFlowsProviderIntentHashMatchesExactPublicToolJSON(t *testing.T) {
	intent := AnalyzeAccountFlowsProviderIntentV1{
		SubjectAlias:     "acct:1",
		StartInclusive:   "2026-01-01T00:00:00.000000Z",
		EndInclusive:     "2026-01-01T00:02:00.000000Z",
		EvidenceRowLimit: 10,
	}
	body, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := AnalyzeAccountFlowsProviderIntentHashV1(intent), domainsecurity.CanonicalJSONHash(body); got == "" || got != want {
		t.Fatalf("provider intent hash=%q want=%q", got, want)
	}
	for name, changed := range map[string]AnalyzeAccountFlowsProviderIntentV1{
		"subject": {SubjectAlias: "acct:2", StartInclusive: intent.StartInclusive, EndInclusive: intent.EndInclusive, EvidenceRowLimit: intent.EvidenceRowLimit},
		"start":   {SubjectAlias: intent.SubjectAlias, StartInclusive: "2026-01-01T00:00:01.000000Z", EndInclusive: intent.EndInclusive, EvidenceRowLimit: intent.EvidenceRowLimit},
		"end":     {SubjectAlias: intent.SubjectAlias, StartInclusive: intent.StartInclusive, EndInclusive: "2026-01-01T00:03:00.000000Z", EvidenceRowLimit: intent.EvidenceRowLimit},
		"limit":   {SubjectAlias: intent.SubjectAlias, StartInclusive: intent.StartInclusive, EndInclusive: intent.EndInclusive, EvidenceRowLimit: 11},
	} {
		if hash := AnalyzeAccountFlowsProviderIntentHashV1(changed); hash == "" || hash == AnalyzeAccountFlowsProviderIntentHashV1(intent) {
			t.Fatalf("%s intent did not refine the outer effect hash: %q", name, hash)
		}
	}
}

func TestAnalyzeAccountFlowsProviderIntentRejectsNoncanonicalOrUnboundedValues(t *testing.T) {
	base := AnalyzeAccountFlowsProviderIntentV1{
		SubjectAlias: "acct:1", StartInclusive: "2026-01-01T00:00:00.000000Z",
		EndInclusive: "2026-01-01T00:02:00.000000Z", EvidenceRowLimit: 10,
	}
	for name, mutate := range map[string]func(*AnalyzeAccountFlowsProviderIntentV1){
		"raw account": func(value *AnalyzeAccountFlowsProviderIntentV1) { value.SubjectAlias = "6222021234567890123" },
		"authority ref": func(value *AnalyzeAccountFlowsProviderIntentV1) {
			value.SubjectAlias = "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"leading zero": func(value *AnalyzeAccountFlowsProviderIntentV1) { value.SubjectAlias = "acct:01" },
		"overflow":     func(value *AnalyzeAccountFlowsProviderIntentV1) { value.SubjectAlias = "card:4294967296" },
		"unsupported":  func(value *AnalyzeAccountFlowsProviderIntentV1) { value.SubjectAlias = "person:1" },
		"offset time":  func(value *AnalyzeAccountFlowsProviderIntentV1) { value.StartInclusive = "2026-01-01T08:00:00+08:00" },
		"reversed": func(value *AnalyzeAccountFlowsProviderIntentV1) {
			value.StartInclusive, value.EndInclusive = value.EndInclusive, value.StartInclusive
		},
		"zero limit": func(value *AnalyzeAccountFlowsProviderIntentV1) { value.EvidenceRowLimit = 0 },
		"large limit": func(value *AnalyzeAccountFlowsProviderIntentV1) {
			value.EvidenceRowLimit = AccountFlowMaximumEvidenceRowsV1 + 1
		},
	} {
		value := base
		mutate(&value)
		if ValidateAnalyzeAccountFlowsProviderIntentV1(value) == nil || AnalyzeAccountFlowsProviderIntentHashV1(value) != "" {
			t.Fatalf("%s provider intent was accepted", name)
		}
	}
}
