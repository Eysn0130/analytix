package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestSemanticRestartPreservationRejectsWholeProgramBeforeEffects(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, restart := range []bool{false, true} {
			for _, scenario := range []string{"held_primary", "held_index", "independent_index", "independent_family"} {
				name := map[bool]string{false: "current", true: "legacy"}[legacy] + "/" + map[bool]string{false: "apply", true: "authenticated_restart"}[restart] + "/" + scenario
				t.Run(name, func(t *testing.T) {
					ctx := context.Background()
					roots := semanticRootsForTest(t)
					writeSemanticFixture(t, roots, "before")
					if !legacy && scenario == "independent_family" {
						// runtime-go itself is not a managed snapshot anchor. Keep
						// that container present; the managed threads anchor is absent.
						if err := os.MkdirAll(filepath.Join(roots.DurableDir, "runtime-go"), 0o700); err != nil {
							t.Fatal(err)
						}
					}
					family := "threads"
					if legacy {
						family = filepath.Join("runtime-go", "threads")
					}
					relativePrimary := filepath.Join(family, "thread-held", "thread.json")
					if err := os.MkdirAll(filepath.Dir(filepath.Join(roots.DurableDir, relativePrimary)), 0o700); err != nil {
						t.Fatal(err)
					}
					write := func(path, body string) {
						t.Helper()
						if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					write(filepath.Join(roots.DurableDir, relativePrimary), `{"id":"thread-held","turns":[]}`)
					heldRows := `{"threadId":"thread-held","summary":{"id":"thread-held","title":"historical"}}` + "\r\n" + `{"threadId":"thread-held","deleted":true,"summary":{"id":"thread-held"}}`
					write(filepath.Join(roots.DurableDir, "thread_summaries.jsonl"), `{"threadId":"thread-independent","summary":{"id":"thread-independent","title":"before"}}`+"\n"+heldRows)
					preserved, err := eventlog.PrepareSemanticRestartPreservationV1(ctx, roots.DurableDir, filepath.Join(roots.DurableDir, "thread_summaries.jsonl"), []string{"thread-held"})
					if err != nil {
						t.Fatal(err)
					}
					baseline, digest := semanticBaselineForTest(t, roots)
					builder := NewSemanticPlanBuilder(roots)
					if !restart {
						builder = NewSemanticPlanBuilderWithRestartPreservationV1(roots, builder.rootAuthority, builder.journalAuthority, preserved)
					}
					prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
						// A valid independent change shares the same signed transaction.
						if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "fixture.bin"), []byte("after"), 0o600); err != nil {
							return err
						}
						if scenario == "held_primary" {
							return os.WriteFile(filepath.Join(stage.DurableDir, relativePrimary), []byte(`{"id":"thread-held","turns":[],"title":"changed"}`), 0o600)
						}
						if scenario == "independent_family" {
							otherFamily := filepath.Join("runtime-go", "threads")
							if legacy {
								otherFamily = "threads"
							}
							path := filepath.Join(stage.DurableDir, otherFamily, "thread-independent")
							if err := os.MkdirAll(path, 0o700); err != nil {
								return err
							}
							return os.WriteFile(filepath.Join(path, "thread.json"), []byte(`{"id":"thread-independent","turns":[]}`), 0o600)
						}
						rows := `{"threadId":"thread-independent","summary":{"id":"thread-independent","title":"after"}}` + "\n" + heldRows
						if scenario == "held_index" {
							rows = `{"threadId":"thread-independent","summary":{"id":"thread-independent","title":"after"}}` + "\n" + `{"threadId":"thread-held","deleted":true,"summary":{"id":"thread-held"}}` + "\n"
						}
						return os.WriteFile(filepath.Join(stage.DurableDir, "thread_summaries.jsonl"), []byte(rows), 0o600)
					})
					if err != nil {
						t.Fatal(err)
					}
					defer prepared.Close()
					if len(prepared.Plan().Operations) < 2 || (scenario != "independent_family" && len(prepared.Plan().Operations) != 2) {
						t.Fatalf("fixture is not the complete two-operation transaction: %v", prepared.Plan().Operations)
					}
					if restart {
						builder.fault = stopSemanticJournalAfterPrepared
						if err := prepared.Apply(ctx); err == nil {
							t.Fatal("fixture failed to stop at authenticated prepared journal")
						}
					}
					beforeData, err := treeDigestForStartupUserDataTest(roots.DataDir)
					if err != nil {
						t.Fatal(err)
					}
					beforeDurable, err := treeDigestForStartupUserDataTest(roots.DurableDir)
					if err != nil {
						t.Fatal(err)
					}
					beforeNamespace, err := treeDigestForStartupUserDataTest(builder.journalAuthority.path())
					if err != nil {
						t.Fatal(err)
					}
					if restart {
						fresh := NewSemanticPlanBuilder(roots)
						err = NewSemanticPlanBuilderWithRestartPreservationV1(roots, fresh.rootAuthority, fresh.journalAuthority, preserved).RecoverAuthenticatedExisting(ctx)
					} else {
						err = prepared.Apply(ctx)
					}
					if scenario == "independent_index" || scenario == "independent_family" {
						if err != nil {
							t.Fatalf("independent rows with exact held framing were denied: %v", err)
						}
						if err := preserved.Revalidate(ctx, roots.DurableDir); err != nil {
							t.Fatal(err)
						}
						return
					}
					if err == nil {
						t.Error("held conflict did not refuse the complete semantic transaction")
					}
					afterData, dataErr := treeDigestForStartupUserDataTest(roots.DataDir)
					afterDurable, durableErr := treeDigestForStartupUserDataTest(roots.DurableDir)
					afterNamespace, namespaceErr := treeDigestForStartupUserDataTest(builder.journalAuthority.path())
					if dataErr != nil || durableErr != nil || namespaceErr != nil || beforeData != afterData || beforeDurable != afterDurable || beforeNamespace != afterNamespace {
						t.Fatal("held conflict crossed the first managed write boundary")
					}
				})
			}
		}
	}
}

func TestSemanticRestartPreservationAllowsPhysicallyCompletedCursorWithoutRewritingHeldFile(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(map[bool]string{false: "already_applied", true: "after_state_drift"}[drift], func(t *testing.T) {
			testSemanticRestartPreservedCompletedCursorV1(t, drift)
		})
	}
}

func testSemanticRestartPreservedCompletedCursorV1(t *testing.T, drift bool) {
	ctx := context.Background()
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	threadDir := filepath.Join(roots.DurableDir, "threads", "thread-held")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thread-held","turns":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline, digest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "fixture.bin"), []byte("after"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DurableDir, "threads", "thread-held", "thread.json"), []byte(`{"id":"thread-held","turns":[],"title":"already-applied"}`), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	plan := prepared.Plan()
	if len(plan.Operations) != 2 || plan.Operations[1].Path != "durable/threads/thread-held/thread.json" {
		t.Fatal("fixture operation order is invalid")
	}
	builder.fault = func(stage string, index int) error {
		if stage == "after_operation" && index == 1 {
			return errors.New("synthetic crash before cursor commit")
		}
		return nil
	}
	if err := prepared.Apply(ctx); err == nil {
		t.Fatal("physical completion crash cut was not reached")
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := readSemanticJournalForTest(t, journalRoot)
	if err != nil || journal.State != semanticJournalApplying || journal.NextOperation != 1 || journal.Plan.Operations[1].Kind != domainstartup.SemanticOperationInstallFile {
		t.Fatalf("physical completion cursor fixture is invalid: %v", err)
	}
	// This restart observes the already-applied primary as its original bytes.
	preserved, err := eventlog.PrepareSemanticRestartPreservationV1(ctx, roots.DurableDir, filepath.Join(roots.DurableDir, "thread_summaries.jsonl"), []string{"thread-held"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := treeDigestForStartupUserDataTest(roots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	fresh := NewSemanticPlanBuilder(roots)
	recovery := NewSemanticPlanBuilderWithRestartPreservationV1(roots, fresh.rootAuthority, fresh.journalAuthority, preserved)
	injected := false
	if drift {
		recovery.fault = func(stage string, index int) error {
			if stage == "before_operation" && index == 1 {
				injected = true
				return os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"thread-held","turns":[]}`), 0o600)
			}
			return nil
		}
	}
	err = recovery.RecoverAuthenticatedExisting(ctx)
	if drift {
		if !injected || err == nil || !strings.Contains(err.Error(), "no-write cursor changed") {
			t.Fatalf("changed no-write cursor reached a write fallback: injected=%v err=%v", injected, err)
		}
		body, err := os.ReadFile(filepath.Join(threadDir, "thread.json"))
		if err != nil || string(body) != `{"id":"thread-held","turns":[]}` {
			t.Fatal("executor rewrote the held cursor after its no-write proof changed")
		}
		return
	}
	if err != nil {
		t.Fatalf("already-applied held operation was treated as a new write: %v", err)
	}
	after, err := treeDigestForStartupUserDataTest(roots.DurableDir)
	if err != nil || before != after {
		t.Fatal("physically completed held primary was rewritten")
	}
	if err := preserved.Revalidate(ctx, roots.DurableDir); err != nil {
		t.Fatal(err)
	}
}
