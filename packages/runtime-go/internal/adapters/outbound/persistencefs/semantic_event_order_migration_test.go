package persistencefs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	eventlogadapter "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestLegacyEventPhysicalOrderMigrationIsSignedAppliedAndRestartStable(t *testing.T) {
	roots := semanticRootsForTest(t)
	threadID := "thr_signed_legacy_order"
	threadDir := filepath.Join(roots.DurableDir, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thr_signed_legacy_order","turns":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	line1041 := []byte(`{"kind":"tool_call_started","threadId":"thr_signed_legacy_order","turnId":"turn_redacted","seq":1041}` + "\n")
	line1042 := []byte(`{"kind":"tool_progress","threadId":"thr_signed_legacy_order","turnId":"turn_redacted","seq":1042}` + "\n")
	line1043 := []byte(`{"kind":"tool_progress","threadId":"thr_signed_legacy_order","turnId":"turn_redacted","seq":1043}` + "\n")
	line1044 := []byte(`{"kind":"tool_progress","threadId":"thr_signed_legacy_order","turnId":"turn_redacted","seq":1044}` + "\n")
	before := bytes.Join([][]byte{line1041, line1043, line1042, line1044}, nil)
	after := bytes.Join([][]byte{line1041, line1042, line1043, line1044}, nil)
	eventsPath := filepath.Join(threadDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, before, 0o600); err != nil {
		t.Fatal(err)
	}

	baseline, configurationDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	preparedInterface, err := builder.Prepare(context.Background(), baseline, configurationDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		return eventlogadapter.MigrateSemanticStartupContent(eventlogadapter.SemanticStartupContentMigrationInput{
			Root: stage.DurableDir, ThreadSummaryIndexPath: filepath.Join(stage.DurableDir, "thread_summaries.jsonl"),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, ok := preparedInterface.(*preparedSemanticPlan)
	if !ok {
		t.Fatal("semantic prepared plan implementation is unavailable")
	}
	defer prepared.Close()
	plan := prepared.Plan()
	if domainstartup.ValidateSemanticStartupPlanV1(plan) != nil || plan.BaselineDigest != baseline.ManagedSnapshotDigest ||
		plan.ConfigurationDigest != configurationDigest || len(plan.Operations) != 1 {
		t.Fatalf("event migration plan is not exact: %#v", plan)
	}
	operation := plan.Operations[0]
	if operation.Kind != domainstartup.SemanticOperationInstallFile ||
		operation.Path != filepath.ToSlash(filepath.Join("durable", "threads", threadID, "events.jsonl")) ||
		operation.Before.SHA256 != domainsecurity.SHA256Hex(before) || operation.After.SHA256 != domainsecurity.SHA256Hex(after) ||
		operation.Before.Size != int64(len(before)) || operation.After.Size != int64(len(after)) {
		t.Fatalf("event migration operation is not bound to exact bytes: %#v", operation)
	}
	staged, err := os.ReadFile(filepath.Join(prepared.stageRoots.DurableDir, "threads", threadID, "events.jsonl"))
	if err != nil || !bytes.Equal(staged, after) {
		t.Fatalf("semantic stage is not the exact sorted fixed point: err=%v", err)
	}

	builder.fault = stopSemanticJournalAfterPrepared
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("prepared-journal crash cut did not interrupt event migration")
	}
	live, err := os.ReadFile(eventsPath)
	if err != nil || !bytes.Equal(live, before) {
		t.Fatalf("signed journal preparation mutated live events: err=%v", err)
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := readSemanticJournalForTest(t, journalRoot)
	if err != nil || journal.State != semanticJournalPrepared || journal.NextOperation != 0 ||
		journal.AuthoritySignature == "" || !domainsecurity.IsSHA256Hex(journal.AuthorityKeyID) ||
		journal.Plan.PlanDigest != plan.PlanDigest || journal.JournalDigest != semanticJournalDigest(journal) {
		t.Fatalf("prepared event journal is not signed and plan-bound: journal=%#v err=%v", journal, err)
	}

	if err := NewSemanticPlanBuilder(roots).RecoverAuthenticatedExisting(context.Background()); err != nil {
		t.Fatalf("authenticated restart did not converge signed event migration: %v", err)
	}
	live, err = os.ReadFile(eventsPath)
	if err != nil || !bytes.Equal(live, after) {
		t.Fatalf("authenticated restart did not publish exact sorted events: err=%v", err)
	}
	if _, err := CaptureStrict(roots); err != nil {
		t.Fatalf("strict readback rejected committed sorted events: %v", err)
	}
	if _, err := os.Lstat(journalRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed event migration journal was not retired: %v", err)
	}

	restartBaseline, restartConfiguration := semanticBaselineForTest(t, roots)
	restartPrepared, err := NewSemanticPlanBuilder(roots).Prepare(context.Background(), restartBaseline, restartConfiguration, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		return eventlogadapter.MigrateSemanticStartupContent(eventlogadapter.SemanticStartupContentMigrationInput{
			Root: stage.DurableDir, ThreadSummaryIndexPath: filepath.Join(stage.DurableDir, "thread_summaries.jsonl"),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restartPrepared.Close()
	if len(restartPrepared.Plan().Operations) != 0 {
		t.Fatalf("second startup was not a zero-operation fixed point: %#v", restartPrepared.Plan().Operations)
	}
	if err := restartPrepared.Apply(context.Background()); err != nil {
		t.Fatalf("second zero-operation startup failed: %v", err)
	}
}
