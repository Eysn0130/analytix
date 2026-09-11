package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestAuthenticatedSemanticJournalObservationDoesNotCreateMissingState(t *testing.T) {
	ctx := context.Background()
	roots := semanticRootsForTest(t)
	before, err := treeDigestForStartupUserDataTest(filepath.Dir(roots.DataDir))
	if err != nil {
		t.Fatal(err)
	}
	observed, err := ObserveAuthenticatedSemanticJournalV1(ctx, roots)
	if err != nil || observed != nil {
		t.Fatalf("absent journal observation: present=%v err=%v", observed != nil, err)
	}
	after, err := treeDigestForStartupUserDataTest(filepath.Dir(roots.DataDir))
	if err != nil || after != before {
		t.Fatal("read-only observation created startup state")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if observed, err := ObserveAuthenticatedSemanticJournalV1(cancelled, roots); !errors.Is(err, context.Canceled) || observed != nil {
		t.Fatal("cancelled journal observation returned authority")
	}
}

func TestAuthenticatedSemanticJournalObservationBindsCompleteInterruptedProgramWithoutWrites(t *testing.T) {
	for _, fault := range []string{"none", "stage_drift", "managed_drift", "forged_journal"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			roots := semanticRootsForTest(t)
			writeSemanticFixture(t, roots, "before")
			baseline, digest := semanticBaselineForTest(t, roots)
			builder := NewSemanticPlanBuilder(roots)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "fixture.bin"), []byte("after"), 0o600); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(stage.DataDir, "private", "z-independent.bin"), []byte("independent-after"), 0o600)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			plan := prepared.Plan()
			if len(plan.Operations) != 2 {
				t.Fatal("observation fixture needs two operations")
			}
			builder.fault = func(stage string, index int) error {
				if stage == "after_operation" && index == 0 {
					return errors.New("synthetic physical cursor cut")
				}
				return nil
			}
			if err := prepared.Apply(ctx); err == nil {
				t.Fatal("signed interrupted fixture did not stop")
			}
			journalRoot := filepath.Join(builder.journalAuthority.path(), "journal")
			before := map[string][32]byte{}
			for _, path := range []string{filepath.Dir(roots.DataDir), journalRoot} {
				digest, err := treeDigestForStartupUserDataTest(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = digest
			}
			observed, err := ObserveAuthenticatedSemanticJournalV1(ctx, roots)
			if err != nil || observed == nil {
				t.Fatalf("signed current-state observation failed: %v", err)
			}
			if observed.NextOperationV1() != 0 || !reflect.DeepEqual(observed.OperationsV1(), plan.Operations) {
				t.Fatal("observation filtered or changed the signed program/cursor")
			}
			for index, operation := range plan.Operations {
				after, err := observed.PhysicallyAfterV1(ctx, operation)
				if err != nil || after != (index == 0) {
					t.Fatal("physical After observation did not distinguish completed cursor and unwritten suffix")
				}
			}
			operations := observed.OperationsV1()
			operations[0].Path = "data/private/forged.bin"
			if _, err := observed.ReadAfterV1(ctx, operations[0]); err == nil {
				t.Fatal("altered operation obtained authenticated After bytes")
			}
			if _, err := observed.PhysicallyAfterV1(ctx, operations[0]); err == nil {
				t.Fatal("altered operation obtained a physical-state observation")
			}
			if !reflect.DeepEqual(observed.OperationsV1(), plan.Operations) {
				t.Fatal("returned operation slice changed private observation")
			}
			for _, operation := range plan.Operations {
				body, err := observed.ReadAfterV1(ctx, operation)
				if err != nil || len(body) == 0 {
					t.Fatalf("exact signed After unreadable: %v", err)
				}
				body[0] ^= 1
				if _, err := observed.ReadAfterV1(ctx, operation); err != nil {
					t.Fatal("mutating returned bytes changed journal stage")
				}
			}
			if err := observed.Revalidate(ctx); err != nil {
				t.Fatal(err)
			}
			for path, expected := range before {
				actual, err := treeDigestForStartupUserDataTest(path)
				if err != nil || actual != expected {
					t.Fatal("authenticated observation wrote or cleaned state")
				}
			}
			switch fault {
			case "none":
				return
			case "stage_drift":
				if err := os.WriteFile(filepath.Join(journalRoot, "stage", plan.Operations[1].OperationID), []byte("stage-corrupt"), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := observed.ReadAfterV1(ctx, plan.Operations[1]); err == nil {
					t.Fatal("corrupt After bytes returned")
				}
			case "managed_drift":
				if err := os.WriteFile(filepath.Join(roots.DataDir, "private", "fixture.bin"), []byte("unknown-current"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "forged_journal":
				path := filepath.Join(journalRoot, "journal.json")
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				body[len(body)/2] ^= 1
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := observed.Revalidate(ctx); err == nil {
				t.Fatal("changed signed program/stage/current inventory was accepted")
			}
			if next, err := ObserveAuthenticatedSemanticJournalV1(ctx, roots); err == nil || next != nil {
				t.Fatal("fresh observation accepted invalid interrupted state")
			}
		})
	}
}
