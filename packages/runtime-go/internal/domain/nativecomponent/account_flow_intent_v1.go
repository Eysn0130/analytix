package nativecomponent

import (
	"encoding/json"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// AnalyzeAccountFlowsProviderIntentV1 is the complete provider-visible effect
// intent for the fixed funds tool. It contains only a closed case-scoped model
// alias and bounded query controls; authority refs and exact account bytes
// remain host-only.
type AnalyzeAccountFlowsProviderIntentV1 struct {
	SubjectAlias     string `json:"subject_alias"`
	StartInclusive   string `json:"start_inclusive"`
	EndInclusive     string `json:"end_inclusive"`
	EvidenceRowLimit uint32 `json:"evidence_row_limit"`
}

func ValidateAnalyzeAccountFlowsProviderIntentV1(intent AnalyzeAccountFlowsProviderIntentV1) error {
	start, startOK := canonicalAccountFlowTimestampV1(intent.StartInclusive)
	end, endOK := canonicalAccountFlowTimestampV1(intent.EndInclusive)
	if domaincaseentity.ValidateModelEntityAliasV1(intent.SubjectAlias) != nil ||
		!startOK || !endOK || start != intent.StartInclusive || end != intent.EndInclusive || start > end ||
		intent.EvidenceRowLimit == 0 || intent.EvidenceRowLimit > AccountFlowMaximumEvidenceRowsV1 {
		return ErrRequestInvalid
	}
	body, err := json.Marshal(intent)
	if err != nil || len(body) == 0 || len(body) > AccountFlowMaximumRequestBytesV1 ||
		domainsecurity.CanonicalJSONHash(body) == "" {
		return ErrRequestInvalid
	}
	return nil
}

// AnalyzeAccountFlowsProviderIntentHashV1 equals the canonical hash recorded
// on the outer MCP execution grant for this exact provider-safe JSON value.
func AnalyzeAccountFlowsProviderIntentHashV1(intent AnalyzeAccountFlowsProviderIntentV1) string {
	if ValidateAnalyzeAccountFlowsProviderIntentV1(intent) != nil {
		return ""
	}
	body, err := json.Marshal(intent)
	if err != nil {
		return ""
	}
	return domainsecurity.CanonicalJSONHash(body)
}
