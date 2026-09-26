package thread

import (
	"strings"
	"testing"
)

func TestContinuationProviderReferenceGrammarAndProjection(t *testing.T) {
	for _, digit := range []string{"0", "9", "f"} {
		digest := strings.Repeat(digit, 64)
		ref := ContinuationProviderReferenceV1(digest)
		if !ValidContinuationProviderReferenceV1(ref) || ref != ContinuationProviderReferenceV1(digest) {
			t.Fatal("unstable or invalid provider reference")
		}
		snapshot := TaskContinuationSnapshotV1{StateDigest: digest, UserHistory: &ContinuationUserHistoryV1{Version: "continuation-user-history.v1", ScopeDigest: digest, Sources: []ContinuationUserSourceV1{{Reference: digest, Digest: digest, Text: "Keep this original request."}}}}
		view := ProviderContinuationMapV1(snapshot)
		history := view["userHistory"].(map[string]any)
		source := history["sources"].([]map[string]any)[0]
		if view["sourceSnapshotReference"] != ref || source["reference"] != ref || source["text"] != snapshot.UserHistory.Sources[0].Text || snapshot.UserHistory.Sources[0].Reference != digest {
			t.Fatal("inline projection lost text/reference or changed sealed snapshot")
		}
	}
	if ContinuationProviderReferenceV1(strings.Repeat("0", 64)) == ContinuationProviderReferenceV1(strings.Repeat("9", 64)) {
		t.Fatal("different sources collapsed")
	}
	for _, invalid := range []string{"", "hist1_" + strings.Repeat("a", 63), "hist1_" + strings.Repeat("a", 65), "hist1_" + strings.Repeat("q", 64), "hist1_" + strings.Repeat("1", 64), strings.Repeat("a", 64)} {
		if ValidContinuationProviderReferenceV1(invalid) {
			t.Fatal("invalid wire reference admitted")
		}
	}
	if ContinuationProviderReferenceV1("invalid") != "" {
		t.Fatal("invalid local digest encoded")
	}
}
