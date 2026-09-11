package runtimeapp

import (
	"testing"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

func TestTerminalCompleteFinalAuthorityAdmitsOnlyCompleteChain(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	inventory := evidenceapp.FinalAuthorityInventory{
		Committed:             []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
		CommittedDispositions: []domainevidence.AcceptedFinalDispositionRecord{fixture.AcceptedDisposition},
	}
	recovery := turnterminalapp.RestartRecoveryResultV1{Complete: []turnterminalapp.CommitResultV1{{
		Persistence: appturn.PersistAcceptedFinalResult{AcceptedFinal: fixture.PrivateFinal.AcceptedFinal},
		Intent:      fixture.Intent, ProviderClosure: fixture.Closure,
		AcceptedFinalDisposition: fixture.AcceptedDisposition, TerminalDisposition: fixture.Disposition,
	}}}
	records, dispositions, err := terminalCompleteFinalAuthorityV1(inventory, recovery)
	if err != nil || len(records) != 1 || len(dispositions) != 1 ||
		records[0].AcceptedFinal.RecordDigest != fixture.PrivateFinal.AcceptedFinal.RecordDigest ||
		dispositions[0].RecordDigest != fixture.AcceptedDisposition.RecordDigest {
		t.Fatalf("complete terminal chain was not admitted: records=%#v dispositions=%#v err=%v", records, dispositions, err)
	}
}

func TestTerminalCompleteFinalAuthorityQuarantinesLegacyAndRejectsUnclassifiedCommit(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	inventory := evidenceapp.FinalAuthorityInventory{
		Committed:             []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
		CommittedDispositions: []domainevidence.AcceptedFinalDispositionRecord{fixture.AcceptedDisposition},
	}
	records, dispositions, err := terminalCompleteFinalAuthorityV1(inventory, turnterminalapp.RestartRecoveryResultV1{
		LegacyQuarantined: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	})
	if err != nil || len(records) != 0 || len(dispositions) != 0 {
		t.Fatalf("legacy public final entered trusted projection: records=%#v dispositions=%#v err=%v", records, dispositions, err)
	}
	if _, _, err := terminalCompleteFinalAuthorityV1(inventory, turnterminalapp.RestartRecoveryResultV1{}); err == nil {
		t.Fatal("unclassified committed final entered trusted projection")
	}
}

func TestTerminalCompleteFinalAuthorityClosesNonExecutableAuditInventory(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	inventory := evidenceapp.FinalAuthorityInventory{
		AuditOnlyPublicWinners: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	}
	recovery := turnterminalapp.RestartRecoveryResultV1{
		NonExecutableAuditOnly: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	}
	records, dispositions, err := terminalCompleteFinalAuthorityV1(inventory, recovery)
	if err != nil || len(records) != 0 || len(dispositions) != 0 {
		t.Fatalf("non-executable audit final escaped quarantine: records=%#v dispositions=%#v err=%v", records, dispositions, err)
	}
	if _, _, err := terminalCompleteFinalAuthorityV1(inventory, turnterminalapp.RestartRecoveryResultV1{}); err == nil {
		t.Fatal("unclassified non-executable audit final passed terminal closure")
	}
}

func TestTerminalCompleteFinalAuthorityKeepsExactPreservedInventoryNonExecutable(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	for _, classification := range []string{"committed", "not_committed", "audit_winner", "audit_not_committed", "public_repair"} {
		t.Run(classification, func(t *testing.T) {
			inventory := evidenceapp.FinalAuthorityInventory{}
			switch classification {
			case "committed":
				inventory.Committed = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
				inventory.CommittedDispositions = []domainevidence.AcceptedFinalDispositionRecord{fixture.AcceptedDisposition}
			case "not_committed":
				inventory.NotCommitted = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
			case "audit_winner":
				inventory.AuditOnlyPublicWinners = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
			case "audit_not_committed":
				inventory.AuditOnlyNotCommitted = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
			case "public_repair":
				inventory.PublicCommitRepairs = []evidenceapp.AcceptedFinalPublicCommitRepair{{PrivateRecord: fixture.PrivateFinal}}
			}
			recovery := turnterminalapp.RestartRecoveryResultV1{Preserved: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}}
			records, dispositions, err := terminalCompleteFinalAuthorityV1(inventory, recovery)
			if err != nil || len(records) != 0 || len(dispositions) != 0 {
				t.Fatalf("original held %s was rejected or became executable: records=%d dispositions=%d err=%v", classification, len(records), len(dispositions), err)
			}
		})
	}
}

func TestTerminalCompleteFinalAuthorityRejectsInvalidPreservedInventory(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"unknown", "duplicate", "changed", "overlap_complete", "overlap_audit", "overlap_legacy", "overlap_non_executable", "duplicate_source"} {
		t.Run(fault, func(t *testing.T) {
			inventory := evidenceapp.FinalAuthorityInventory{Committed: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}, CommittedDispositions: []domainevidence.AcceptedFinalDispositionRecord{fixture.AcceptedDisposition}}
			recovery := turnterminalapp.RestartRecoveryResultV1{Preserved: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}}
			switch fault {
			case "unknown":
				inventory = evidenceapp.FinalAuthorityInventory{}
			case "duplicate":
				recovery.Preserved = append(recovery.Preserved, fixture.PrivateFinal)
			case "changed":
				recovery.Preserved[0].SecurityContext.ThreadID = "thread-foreign"
			case "overlap_complete":
				recovery.Complete = []turnterminalapp.CommitResultV1{{Persistence: appturn.PersistAcceptedFinalResult{AcceptedFinal: fixture.PrivateFinal.AcceptedFinal}, Intent: fixture.Intent, ProviderClosure: fixture.Closure, AcceptedFinalDisposition: fixture.AcceptedDisposition, TerminalDisposition: fixture.Disposition}}
			case "overlap_audit":
				inventory.Committed = nil
				inventory.NotCommitted = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
				recovery.AuditOnly = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
			case "overlap_legacy":
				recovery.LegacyQuarantined = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
			case "overlap_non_executable":
				inventory.Committed = nil
				inventory.AuditOnlyPublicWinners = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
				recovery.NonExecutableAuditOnly = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
			case "duplicate_source":
				inventory.NotCommitted = []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal}
			}
			if _, _, err := terminalCompleteFinalAuthorityV1(inventory, recovery); err == nil {
				t.Fatalf("invalid preserved classification was admitted: %s", fault)
			}
		})
	}
}
