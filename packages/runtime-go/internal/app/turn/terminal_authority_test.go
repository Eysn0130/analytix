package turn

import (
	"testing"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

func TestInspectTerminalAuthorityV1AcceptsOnlyCanonicalExistingWinner(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	status, found, terminal, err := InspectTerminalAuthorityV1(thread, commit.TurnID)
	if err != nil || !found || !terminal || status != "completed" {
		t.Fatalf("canonical terminal winner inspection mismatch: status=%q found=%t terminal=%t err=%v", status, found, terminal, err)
	}

	pending, pendingCommit, _, _ := pendingGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	status, found, terminal, err = InspectTerminalAuthorityV1(pending, pendingCommit.TurnID)
	if err != nil || !found || terminal || status != "running" {
		t.Fatalf("active turn inspection mismatch: status=%q found=%t terminal=%t err=%v", status, found, terminal, err)
	}

	tampered := contracts.CloneMap(thread)
	publicationFixtureAssistant(tampered)["text"] = "tampered after publication"
	if _, _, terminal, err = InspectTerminalAuthorityV1(tampered, commit.TurnID); !terminal || err == nil {
		t.Fatal("tampered existing terminal winner was accepted")
	}

	if _, found, terminal, err = InspectTerminalAuthorityV1(thread, "turn-missing"); err != nil || found || terminal {
		t.Fatalf("missing turn inspection mismatch: found=%t terminal=%t err=%v", found, terminal, err)
	}
}

func TestInspectTerminalAuthorityV1ValidatesAcceptedFinalWinner(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-case-terminal", "turn-case-terminal", 2)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: securityContext, TerminalReason: "success",
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := signedAcceptedFinalForTest(t, securityContext, envelope, time.Unix(2, 0))
	plan, err := BuildAcceptedFinalPublicationPlan(record, domainevidence.CaseUnverifiedText, acceptedFinalIntentForTest(t, "success", time.Unix(2, 0)))
	if err != nil {
		t.Fatal(err)
	}
	items := make([]any, len(plan.TurnItems))
	for index := range plan.TurnItems {
		items[index] = contracts.CloneMap(plan.TurnItems[index])
	}
	turn := map[string]any{
		"id": securityContext.TurnID, "status": "completed", "finishedAt": record.AcceptedAt,
		"securityContext": turnSecurityContextRecord(securityContext), "acceptedFinal": domainevidence.AcceptedFinalRecordMap(record), "items": items,
	}
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{turn}}
	status, found, terminal, err := InspectTerminalAuthorityV1(thread, securityContext.TurnID)
	if err != nil || !found || !terminal || status != "completed" {
		t.Fatalf("accepted-final terminal winner inspection mismatch: status=%q found=%t terminal=%t err=%v", status, found, terminal, err)
	}

	tampered := contracts.CloneMap(thread)
	tamperedTurn := tampered["turns"].([]any)[0].(map[string]any)
	tamperedTurn["items"].([]any)[0].(map[string]any)["text"] = "unsupported case fact"
	if _, _, terminal, err = InspectTerminalAuthorityV1(tampered, securityContext.TurnID); !terminal || err == nil {
		t.Fatal("tampered accepted-final terminal winner was accepted")
	}
}
