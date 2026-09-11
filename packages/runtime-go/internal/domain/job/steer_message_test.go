package job

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSteerMessageLogicalEffectBindingIsSignedWithoutChangingLegacyDigest(t *testing.T) {
	legacy := SteerMessage{
		ID: "steer-1", ParentThreadID: "thread-parent", ChildRunID: "job-1", JobID: "job-1",
		Text: "continue", SourceTurnID: "turn-1", SourceToolCallID: "call-1",
	}
	const legacyDigest = "f5dc373f489bc77e7221d5415ecb3a2f81846b7cd68b1be02fdf87d84d97bf3e"
	if got := SteerMessageContentDigestV1(legacy); got != legacyDigest {
		t.Fatalf("legacy V1 digest changed: got %s want %s", got, legacyDigest)
	}
	if err := ValidateSteerMessageLogicalEffectBindingV1(legacy); err != nil {
		t.Fatalf("legacy absent binding should remain readable: %v", err)
	}

	funds := legacy
	funds.LogicalEffect = domainsecurity.LogicalEffectFundsData
	if err := ValidateSteerMessageLogicalEffectBindingV1(funds); err != nil {
		t.Fatalf("funds binding rejected: %v", err)
	}
	if SteerMessageContentDigestV1(funds) == legacyDigest {
		t.Fatal("logical effect was not covered by the content digest")
	}
	funds.OrdinaryWork = true
	if SteerMessageContentDigestV1(funds) == SteerMessageContentDigestV1(legacy) {
		t.Fatal("ordinary-work bit was not covered by the content digest")
	}
}

func TestSteerMessageLogicalEffectBindingFailsClosedOnPartialOrInvalidValues(t *testing.T) {
	for _, message := range []SteerMessage{
		{OrdinaryWork: true},
		{LogicalEffect: domainsecurity.LogicalEffectOrdinary},
		{LogicalEffect: domainsecurity.LogicalEffect("unknown")},
	} {
		if err := ValidateSteerMessageLogicalEffectBindingV1(message); err == nil {
			t.Fatalf("invalid binding was accepted: %#v", message)
		}
	}
	for _, message := range []SteerMessage{
		{LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
		{LogicalEffect: domainsecurity.LogicalEffectCaseData},
		{LogicalEffect: domainsecurity.LogicalEffectFundsData, OrdinaryWork: true},
	} {
		if err := ValidateSteerMessageLogicalEffectBindingV1(message); err != nil {
			t.Fatalf("valid binding rejected: %#v err=%v", message, err)
		}
	}
}
