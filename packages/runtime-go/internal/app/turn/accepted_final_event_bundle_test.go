package turn

import (
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

func TestPrepareAcceptedFinalEventBundleValidatesWholeManifestBeforeSequencing(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-bundle", "turn-bundle", 3)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: securityContext, TerminalReason: "success",
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: time.Unix(3, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := signedAcceptedFinalForTest(t, securityContext, envelope, time.Unix(3, 0))
	intent := acceptedFinalIntentForTest(t, envelope.TerminalReason, time.Unix(3, 0))
	plan, err := BuildAcceptedFinalPublicationPlan(record, domainevidence.CaseUnverifiedText, intent)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]any, 0, len(plan.TurnItems))
	for _, item := range plan.TurnItems {
		items = append(items, item)
	}
	turn := map[string]any{
		"id": record.TurnID, "status": "completed", "securityContext": turnSecurityContextRecord(securityContext),
		"acceptedFinal": domainevidence.AcceptedFinalRecordMap(record), "items": items,
	}
	thread := map[string]any{
		"id": record.ThreadID, "securityState": turnSecurityContextRecord(securityContext), "turns": []any{turn},
	}
	drafts := make([]map[string]any, 0, len(plan.Events))
	for _, event := range plan.Events {
		drafts = append(drafts, event.Draft)
	}
	prepared, err := PrepareAcceptedFinalEventBundle(thread, drafts, 11)
	if err != nil || len(prepared) != len(drafts) {
		t.Fatalf("prepare accepted final bundle: events=%#v err=%v", prepared, err)
	}
	for index, event := range prepared {
		if event["seq"] != float64(11+index) {
			t.Fatalf("bundle sequence mismatch at %d: %#v", index, event)
		}
	}

	tampered := make([]map[string]any, len(drafts))
	for index, event := range drafts {
		tampered[index] = cloneMapForBundleTest(event)
	}
	tampered[len(tampered)-1]["publicationCommitId"] = "forged"
	if projected, err := PrepareAcceptedFinalEventBundle(thread, tampered, 11); err == nil || len(projected) != 0 {
		t.Fatalf("whole-manifest validation should fail before returning a partial bundle: projected=%#v err=%v", projected, err)
	}
}

func cloneMapForBundleTest(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
