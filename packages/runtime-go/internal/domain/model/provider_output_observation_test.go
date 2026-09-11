package model

import "testing"

func TestProviderOutputObservationV1PreservesAllClassesAndFailsClosedOnUnknown(t *testing.T) {
	observation := ProviderOutputObservationPrivateReasoningV1.
		MergeV1(ProviderOutputObservationPublicTextV1).
		MergeV1(ProviderOutputObservationToolStartedV1)
	if !observation.IncludesV1(ProviderOutputObservationPrivateReasoningV1) ||
		!observation.IncludesV1(ProviderOutputObservationPublicTextV1) ||
		!observation.IncludesV1(ProviderOutputObservationToolStartedV1) ||
		!observation.RetryUnsafeV1() {
		t.Fatalf("provider output observation lost a class: %08b", observation)
	}
	unknown := ProviderOutputObservationV1(1 << 7)
	if !unknown.HasUnknownClassV1() || !unknown.RetryUnsafeV1() {
		t.Fatalf("unknown provider output observation failed open: %08b", unknown)
	}
	if ProviderOutputObservationPrivateReasoningV1.RetryUnsafeV1() ||
		ProviderOutputObservationControlV1.RetryUnsafeV1() {
		t.Fatal("private reasoning or control state was treated as public output")
	}
}
