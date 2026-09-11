package reportpublication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestGrantSettlementWriterPersistsDecisionBoundStageOnceAndReplays(t *testing.T) {
	fixture, decision, expected, writer := newGrantSettlementWriterFixture(t)
	if fixture.stages.receipt.GrantMembers[0].RegistrySequence >= decision.GrantRegistrySequence {
		t.Fatalf("fixture does not exercise a non-tail report grant: member=%d head=%d",
			fixture.stages.receipt.GrantMembers[0].RegistrySequence, decision.GrantRegistrySequence)
	}

	first, err := writer.SettleDecisionV1(context.Background(), decision)
	if err != nil || !reflect.DeepEqual(first, expected) {
		t.Fatalf("decision-bound settlement was not persisted: got=%#v want=%#v err=%v", first, expected, err)
	}
	second, err := writer.SettleDecisionV1(context.Background(), decision)
	if err != nil || !reflect.DeepEqual(second, expected) {
		t.Fatalf("decision-bound settlement replay was not idempotent: got=%#v want=%#v err=%v", second, expected, err)
	}
	fixture.grantSettlements.mu.Lock()
	recordCount := len(fixture.grantSettlements.records)
	createCalls := fixture.grantSettlements.createCalls
	fixture.grantSettlements.mu.Unlock()
	if recordCount != 1 || createCalls != 2 {
		t.Fatalf("settlement CAS inventory = %d records/%d attempts, want one record/two idempotent attempts", recordCount, createCalls)
	}
	body, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if account := fixture.input.ClaimLedger.Claims[0].NormalizedPayload.AccountID; bytes.Contains(body, []byte(account)) {
		t.Fatalf("grant settlement leaked the exact controlled account: %s", body)
	}
}

func TestGrantSettlementWriterResolvesLostCreateAcknowledgement(t *testing.T) {
	fixture, decision, expected, writer := newGrantSettlementWriterFixture(t)
	ackLost := errors.New("grant settlement acknowledgement lost")
	fixture.grantSettlements.mu.Lock()
	fixture.grantSettlements.createErr = ackLost
	fixture.grantSettlements.commitOnErr = true
	fixture.grantSettlements.mu.Unlock()

	stored, err := writer.SettleDecisionV1(context.Background(), decision)
	if err != nil || !reflect.DeepEqual(stored, expected) {
		t.Fatalf("durable settlement was lost with its acknowledgement: got=%#v want=%#v err=%v", stored, expected, err)
	}
}

func TestGrantSettlementWriterFailsClosedWhenCreateDidNotCommit(t *testing.T) {
	fixture, decision, _, writer := newGrantSettlementWriterFixture(t)
	fixture.grantSettlements.mu.Lock()
	fixture.grantSettlements.createErr = errors.New("grant settlement write failed")
	fixture.grantSettlements.commitOnErr = false
	fixture.grantSettlements.mu.Unlock()

	settlement, err := writer.SettleDecisionV1(context.Background(), decision)
	if !errors.Is(err, ErrPublicationRestartUnresolved) || settlement.RecordDigest != "" {
		t.Fatalf("uncommitted settlement was treated as durable: settlement=%#v err=%v", settlement, err)
	}
}

func TestGrantSettlementWriterRejectsFabricatedOrMismatchedStageAuthority(t *testing.T) {
	t.Run("fabricated host admission", func(t *testing.T) {
		fixture, decision, _, writer := newGrantSettlementWriterFixture(t)
		fixture.stages.mu.Lock()
		fixture.stages.trustedResultItem["hostReportAdmission"] = domaintoolresult.HostReportAdmissionRecordV1(
			domaintoolresult.NewHostReportAdmissionV1(
				domainsecurity.SHA256Hex([]byte("fabricated-report-decision")),
				decision.RecordDigest,
			),
		)
		fixture.stages.mu.Unlock()
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("fabricated host admission created settlement authority: settlement=%#v err=%v", settlement, err)
		}
	})

	t.Run("work already has terminal disposition", func(t *testing.T) {
		fixture, decision, expected, writer := newGrantSettlementWriterFixture(t)
		fixture.installDurableReportStageCompletion(t, decision, expected)
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("closed work created a pre-disposition settlement: settlement=%#v err=%v", settlement, err)
		}
	})

	t.Run("decision signature graph mismatch", func(t *testing.T) {
		_, decision, _, writer := newGrantSettlementWriterFixture(t)
		decision.DecisionID = domainsecurity.SHA256Hex([]byte("mismatched-report-decision"))
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("mutated decision created settlement authority: settlement=%#v err=%v", settlement, err)
		}
	})
}

func TestGrantSettlementWriterRejectsConflictingCASReadback(t *testing.T) {
	fixture, decision, expected, writer := newGrantSettlementWriterFixture(t)
	conflict := expected
	conflict.ResultItemDigest = domainsecurity.SHA256Hex([]byte("conflicting-result-item"))
	fixture.grantSettlements.mu.Lock()
	fixture.grantSettlements.records[conflict.SettlementID] = conflict
	fixture.grantSettlements.mu.Unlock()

	settlement, err := writer.SettleDecisionV1(context.Background(), decision)
	if !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
		t.Fatalf("conflicting settlement readback was accepted: settlement=%#v err=%v", settlement, err)
	}
}

func TestGrantSettlementWriterRequiresExactDecisionCASMembership(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		fixture, decision, _, writer := newGrantSettlementWriterFixture(t)
		fixture.decisions.mu.Lock()
		fixture.decisions.records = map[string]domainpublication.ReportDeliveryDecisionV1{}
		fixture.decisions.mu.Unlock()
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("signer-valid unregistered decision created settlement authority: settlement=%#v err=%v", settlement, err)
		}
	})

	t.Run("conflicting", func(t *testing.T) {
		fixture, decision, _, writer := newGrantSettlementWriterFixture(t)
		conflict := decision
		conflict.Status = "conflicting"
		fixture.decisions.mu.Lock()
		fixture.decisions.records[decision.DecisionID] = conflict
		fixture.decisions.mu.Unlock()
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("conflicting decision CAS member created settlement authority: settlement=%#v err=%v", settlement, err)
		}
	})
}

func TestGrantSettlementWriterIndependentlyRevalidatesPendingAuthority(t *testing.T) {
	t.Run("result bytes changed without digest", func(t *testing.T) {
		fixture, decision, _, writer := newGrantSettlementWriterFixture(t)
		settled := trustedSettledStageForWriterTest(t, fixture, decision)
		settled.ResultItem["callId"] = "call_fabricated_after_pending_validation"
		writer.pending = grantSettlementPendingBypassStub{settled: settled}
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("changed result bytes retained stale digest authority: settlement=%#v err=%v", settlement, err)
		}
	})

	t.Run("wrong call with recomputed digest", func(t *testing.T) {
		fixture, decision, _, writer := newGrantSettlementWriterFixture(t)
		settled := trustedSettledStageForWriterTest(t, fixture, decision)
		settled.ResultItem["callId"] = "call_fabricated_after_pending_validation"
		resultBody, err := json.Marshal(settled.ResultItem)
		if err != nil {
			t.Fatal(err)
		}
		settled.ResultItemDigest = domainsecurity.CanonicalJSONHash(resultBody)
		writer.pending = grantSettlementPendingBypassStub{settled: settled}
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("wrong result call binding created settlement authority: settlement=%#v err=%v", settlement, err)
		}
	})

	t.Run("receipt signature changed", func(t *testing.T) {
		fixture, decision, _, writer := newGrantSettlementWriterFixture(t)
		settled := trustedSettledStageForWriterTest(t, fixture, decision)
		settled.Receipt.AuthoritySignature = "tampered"
		writer.pending = grantSettlementPendingBypassStub{settled: settled}
		if settlement, err := writer.SettleDecisionV1(context.Background(), decision); !errors.Is(err, ErrPublicationIntegrity) || settlement.RecordDigest != "" {
			t.Fatalf("untrusted stage receipt created settlement authority: settlement=%#v err=%v", settlement, err)
		}
	})
}

func TestGrantSettlementWriterConcurrentReplayCreatesOneExactRecord(t *testing.T) {
	fixture, decision, expected, writer := newGrantSettlementWriterFixture(t)
	const callers = 16
	results := make(chan domainpublication.ReportGrantSettlementV1, callers)
	errorsFound := make(chan error, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			settlement, err := writer.SettleDecisionV1(context.Background(), decision)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- settlement
		}()
	}
	group.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent settlement replay failed: %v", err)
	}
	count := 0
	for result := range results {
		count++
		if !reflect.DeepEqual(result, expected) {
			t.Fatalf("concurrent settlement changed authority: got=%#v want=%#v", result, expected)
		}
	}
	fixture.grantSettlements.mu.Lock()
	recordCount := len(fixture.grantSettlements.records)
	fixture.grantSettlements.mu.Unlock()
	if count != callers || recordCount != 1 {
		t.Fatalf("concurrent replay returned %d results and %d records, want %d/1", count, recordCount, callers)
	}
}

func newGrantSettlementWriterFixture(
	t *testing.T,
) (*reportPublicationFixture, domainpublication.ReportDeliveryDecisionV1, domainpublication.ReportGrantSettlementV1, *GrantSettlementWriterV1) {
	t.Helper()
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	expected := fixture.installDurableReportGrantSettlement(t, published.Decision)
	fixture.grantSettlements.mu.Lock()
	fixture.grantSettlements.records = map[string]domainpublication.ReportGrantSettlementV1{}
	fixture.grantSettlements.createCalls = 0
	fixture.grantSettlements.resolveCalls = 0
	fixture.grantSettlements.createErr = nil
	fixture.grantSettlements.commitOnErr = false
	fixture.grantSettlements.mu.Unlock()
	writer, err := NewGrantSettlementWriterV1(GrantSettlementWriterConfigV1{
		InstallationID: published.Decision.InstallationID,
		EnrollmentID:   published.Decision.EnrollmentID,
		Authority:      fixture.authority,
		Pending:        fixture.stages,
		Decisions:      fixture.decisions,
		Settlements:    fixture.grantSettlements,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture, published.Decision, expected, writer
}

type grantSettlementPendingBypassStub struct {
	settled pendingworkapp.TrustedSettledReportStageV1
}

func (stub grantSettlementPendingBypassStub) WithTrustedSettledReportStageForDecisionV1(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	callback func(pendingworkapp.TrustedSettledReportStageV1) error,
) error {
	return callback(stub.settled)
}

func trustedSettledStageForWriterTest(
	t *testing.T,
	fixture *reportPublicationFixture,
	decision domainpublication.ReportDeliveryDecisionV1,
) pendingworkapp.TrustedSettledReportStageV1 {
	t.Helper()
	settled, err := fixture.stages.ResolveTrustedSettledReportStageForDecisionV1(
		context.Background(), decision.ReportStageWorkID, decision.DecisionID, decision.RecordDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	return settled
}
