package persistencefs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestSemanticAuthorityPathIgnoresEnvironmentRetargetWithoutCreatingAlternateNamespace(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	configA := filepath.Join(base, "config-a")
	configB := filepath.Join(base, "config-b")
	for _, path := range []string{home, configA, configB} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	rootAuthority, err := FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	journalAuthority, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	expectedNamespace := journalAuthority.path()

	t.Setenv("XDG_CONFIG_HOME", configB)
	journalRoot, err := semanticJournalRootForAuthority(roots, journalAuthority)
	if err != nil || filepath.Dir(journalRoot) != expectedNamespace {
		t.Fatalf("journal path escaped frozen authority: path=%s err=%v", journalRoot, err)
	}
	planningRoot, err := semanticPlanningRootForAuthority(roots, journalAuthority)
	if err != nil || filepath.Dir(planningRoot) != expectedNamespace {
		t.Fatalf("planning path escaped frozen authority: path=%s err=%v", planningRoot, err)
	}
	if err := NewSemanticPlanBuilderWithAuthorities(
		roots, rootAuthority, journalAuthority,
	).RecoverAuthenticatedExisting(context.Background()); err != nil {
		t.Fatalf("no-journal recovery after environment retarget: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(configB, "analytix")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("environment retarget created alternate authority state: %v", err)
	}
}

func TestStartupJournalForgedPublicHashesCannotAuthenticateRecovery(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	builder.fault = stopSemanticJournalAfterPrepared
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop at the prepared journal")
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := loadStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := readSemanticJournal(journalRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	if journal.SchemaVersion != 4 {
		t.Fatalf("current semantic journal schema = %d", journal.SchemaVersion)
	}
	for _, identity := range []string{journal.JournalRootIdentity, journal.JournalStageIdentity} {
		if _, err := parseStrongDirectoryIdentity(identity); err != nil {
			t.Fatalf("current semantic journal used a truncated identity %q: %v", identity, err)
		}
	}
	for _, capability := range journal.RootCapabilities {
		if _, err := parseStrongDirectoryIdentity(capability.AnchorIdentity); err != nil {
			t.Fatalf("current root anchor identity is truncated: %v", err)
		}
		if capability.RootIdentity != "" {
			if _, err := parseStrongDirectoryIdentity(capability.RootIdentity); err != nil {
				t.Fatalf("current root identity is truncated: %v", err)
			}
		}
	}
	forgedConfiguration := domainsecurity.SHA256Hex([]byte("forged-current-config"))
	forgedPlan, err := domainstartup.NewSemanticStartupPlanV1(
		journal.Plan.BaselineDigest, forgedConfiguration, journal.Plan.FinalStateDigest, journal.Plan.Operations,
	)
	if err != nil {
		t.Fatal(err)
	}
	journal.Plan = forgedPlan
	journal.RootBindingDigest = domainsecurity.SHA256Hex([]byte("forged-root-binding"))
	journal.RootCapabilityDigest = domainsecurity.SHA256Hex([]byte("forged-root-capability"))
	journal.JournalDigest = semanticJournalDigest(journal)
	body, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalRoot, "journal.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	planningRoot, err := semanticPlanningRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	planningBefore, err := os.ReadDir(planningRoot)
	if err != nil || len(planningBefore) == 0 {
		t.Fatalf("forged-journal fixture omitted its planning residue: entries=%d err=%v", len(planningBefore), err)
	}
	if err := NewSemanticPlanBuilder(roots).RecoverAuthenticatedExisting(context.Background()); err == nil {
		t.Fatal("authenticated recovery trusted attacker-recomputed public hashes")
	}
	planningAfter, err := os.ReadDir(planningRoot)
	if err != nil || len(planningAfter) != len(planningBefore) {
		t.Fatalf("forged active journal allowed planning cleanup: before=%d after=%d err=%v", len(planningBefore), len(planningAfter), err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), forgedConfiguration); err == nil {
		t.Fatal("journal with attacker-recomputed public hashes retained installation authority")
	}
	after, err := CaptureStrict(roots)
	if err != nil || after.SHA256 != before.SHA256 {
		t.Fatalf("forged journal changed managed state: before=%s after=%s err=%v", before.SHA256, after.SHA256, err)
	}
}

func TestStartupJournalAuthenticatedRecoveryUsesSignedOriginalConfiguration(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	builder.fault = stopSemanticJournalAfterPrepared
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop at the prepared journal")
	}

	if err := NewSemanticPlanBuilder(roots).RecoverAuthenticatedExisting(context.Background()); err != nil {
		t.Fatalf("recover signed old-configuration journal: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "fixture.bin"))
	if err != nil || string(body) != "after" {
		t.Fatalf("authenticated recovery did not apply the signed plan: body=%q err=%v", body, err)
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(journalRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authenticated recovery left the old journal active: %v", err)
	}
}

func TestAuthenticatedRecoveryPreflightsLaterResiduesBeforePlanningCleanup(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		poison func(*testing.T, RootSet, string)
	}{
		{
			name: "forged-retired-journal",
			poison: func(t *testing.T, roots RootSet, journalRoot string) {
				t.Helper()
				namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := openOrCreateStartupJournalAuthority(namespace, journalRoot); err != nil {
					t.Fatal(err)
				}
				retired := filepath.Join(filepath.Dir(journalRoot), ".retired-journal-0123456789abcdef0123456789abcdef")
				if err := os.Mkdir(retired, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(retired, semanticJournalRetirementFile), []byte(`{"forged":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "invalid-authority-create-temp",
			poison: func(t *testing.T, _ RootSet, journalRoot string) {
				t.Helper()
				body := []byte(`{"partial":`)
				target := filepath.Base(startupJournalAuthorityPath(journalRoot))
				name, err := startupAuthorityCreateTempName(target, body, "0123456789abcdef01234567")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(filepath.Dir(journalRoot), name), body, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			writeSemanticFixture(t, roots, "before")
			baseline, configDigest := semanticBaselineForTest(t, roots)
			builder := NewSemanticPlanBuilder(roots)
			prepared, err := builder.Prepare(
				context.Background(), baseline, configDigest, semanticSimulationForTest("after"),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			planningRoot, err := semanticPlanningRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			before := semanticDirectoryEntryNamesForTest(t, planningRoot)
			if before == "" {
				t.Fatal("fixture omitted its semantic planning stage")
			}
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			fixture.poison(t, roots, journalRoot)

			if err := NewSemanticPlanBuilder(roots).RecoverAuthenticatedExisting(context.Background()); err == nil {
				t.Fatal("unsafe later recovery residue was accepted")
			}
			if after := semanticDirectoryEntryNamesForTest(t, planningRoot); after != before {
				t.Fatalf("later recovery failure changed planning residue: before=%q after=%q", before, after)
			}
		})
	}
}

func semanticDirectoryEntryNamesForTest(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return strings.Join(names, "\n")
}

func TestStartupJournalRejectsOldConfigurationBeforeManagedMutation(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	builder.fault = stopSemanticJournalAfterPrepared
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop at the prepared journal")
	}
	before, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	newConfiguration := domainsecurity.SHA256Hex([]byte("different-current-config"))
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), newConfiguration); err == nil {
		t.Fatal("old-configuration startup journal was applied")
	}
	after, err := CaptureStrict(roots)
	if err != nil || after.SHA256 != before.SHA256 {
		t.Fatalf("old-config recovery changed managed state: before=%s after=%s err=%v", before.SHA256, after.SHA256, err)
	}
}

func TestStartupJournalMissingOrWrongInstallationAuthorityFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		poison func(*testing.T, string)
	}{
		{name: "missing", poison: func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong", poison: func(t *testing.T, path string) {
			body, err := newStartupJournalAuthorityBody()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			writeSemanticFixture(t, roots, "before")
			baseline, configDigest := semanticBaselineForTest(t, roots)
			builder := NewSemanticPlanBuilder(roots)
			builder.fault = stopSemanticJournalAfterPrepared
			prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			if err := prepared.Apply(context.Background()); err == nil {
				t.Fatal("semantic apply did not stop at the prepared journal")
			}
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			test.poison(t, startupJournalAuthorityPath(journalRoot))
			before, err := CaptureStrict(roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
				t.Fatal("pending journal recovered without its fixed installation authority")
			}
			after, err := CaptureStrict(roots)
			if err != nil || after.SHA256 != before.SHA256 {
				t.Fatalf("authority failure changed managed state: before=%s after=%s err=%v", before.SHA256, after.SHA256, err)
			}
		})
	}
}

func TestStartupAuthorityCreateResidueIsBenignOnlyWithoutJournal(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	authorityBody, err := newStartupJournalAuthorityBody()
	if err != nil {
		t.Fatal(err)
	}
	discardedAuthority, err := parseStartupJournalAuthorityBody(authorityBody)
	if err != nil {
		t.Fatal(err)
	}
	authorityName := filepath.Base(startupJournalAuthorityPath(journalRoot))
	residueName, err := startupAuthorityCreateTempName(authorityName, authorityBody, "0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(filepath.Dir(journalRoot), residueName)
	if err := os.WriteFile(residue, authorityBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err != nil {
		t.Fatalf("no-journal restart rejected isolated key-create residue: %v", err)
	}
	if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("safe key-create residue survived no-journal recovery: %v", err)
	}
	baseline, configDigest := semanticBaselineForTest(t, roots)
	prepared, err := NewSemanticPlanBuilder(roots).Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatalf("fresh bootstrap did not replace benign uncommitted key residue: %v", err)
	}
	committedBody, err := os.ReadFile(startupJournalAuthorityPath(journalRoot))
	if err != nil {
		t.Fatal(err)
	}
	committedAuthority, err := parseStartupJournalAuthorityBody(committedBody)
	if err != nil {
		t.Fatal(err)
	}
	if committedAuthority.KeyID == discardedAuthority.KeyID {
		t.Fatal("discarded uncommitted authority temp was promoted or reused as the installation key")
	}
}

func TestStartupAuthorityCreateResiduePreflightsExactNamesBeforeCleanup(t *testing.T) {
	roots := semanticRootsForTest(t)
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	prefix := startupJournalAuthorityTempPrefix(journalRoot)
	authorityBody, err := newStartupJournalAuthorityBody()
	if err != nil {
		t.Fatal(err)
	}
	authorityName := filepath.Base(startupJournalAuthorityPath(journalRoot))
	valid := make([]string, 0, 2)
	for _, nonce := range []string{"000000000000000000000001", "000000000000000000000002"} {
		name, err := startupAuthorityCreateTempName(authorityName, authorityBody, nonce)
		if err != nil {
			t.Fatal(err)
		}
		valid = append(valid, name)
	}
	for _, path := range valid {
		if err := os.WriteFile(filepath.Join(filepath.Dir(journalRoot), path), authorityBody, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	malformed := prefix + "later-malformed.tmp"
	if err := os.WriteFile(filepath.Join(filepath.Dir(journalRoot), malformed), []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
		t.Fatal("malformed startup authority residue was accepted")
	}
	for _, name := range append(valid, malformed) {
		if _, err := os.Lstat(filepath.Join(filepath.Dir(journalRoot), name)); err != nil {
			t.Fatalf("startup authority preflight partially cleaned %s: %v", name, err)
		}
	}
}

func TestStartupAuthorityBootstrapsOnlyAfterCompleteSimulation(t *testing.T) {
	for _, test := range []struct {
		name     string
		simulate startupport.SemanticSimulationV1
		wantErr  bool
	}{
		{name: "prepared-not-applied", simulate: semanticSimulationForTest("after")},
		{name: "simulation-failed", wantErr: true, simulate: func(context.Context, startupport.PersistenceRootsV1) error {
			return errors.New("late semantic validation failed")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			writeSemanticFixture(t, roots, "before")
			baseline, configDigest := semanticBaselineForTest(t, roots)
			prepared, err := NewSemanticPlanBuilder(roots).Prepare(context.Background(), baseline, configDigest, test.simulate)
			if test.wantErr {
				if err == nil {
					t.Fatal("failed simulation returned a prepared plan")
				}
			} else if err != nil {
				t.Fatal(err)
			} else {
				defer prepared.Close()
			}
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(startupJournalAuthorityPath(journalRoot)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("startup authority existed before the first journal prepare: %v", err)
			}
		})
	}
}

func TestStartupJournalNamespaceAndKeyIdentitySwapsFailClosed(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private-namespace")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := canonicalPathWithoutCreate(directory)
	if err != nil {
		t.Fatal(err)
	}
	root, err := captureStartupAuthorityRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	namespace := &JournalNamespaceAuthority{root: root}
	journalRoot := filepath.Join(directory, "startup-test")
	authority, err := openOrCreateStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := startupJournalAuthorityPath(journalRoot)
	keyBody, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	heldKey := keyPath + ".held"
	if err := os.Rename(keyPath, heldKey); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.sign([]byte("same bytes, different key inode")); err == nil {
		t.Fatal("same-content key file replacement retained signing authority")
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(heldKey, keyPath); err != nil {
		t.Fatal(err)
	}

	heldNamespace := directory + ".held"
	if err := os.Rename(directory, heldNamespace); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, filepath.Base(keyPath)), keyBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStartupJournalAuthority(namespace, journalRoot); err == nil {
		t.Fatal("same-content lease namespace replacement retained startup authority")
	}
}

func TestSemanticPlanningAndJournalStageSameContentSwapsAreRejected(t *testing.T) {
	t.Run("planning-stage", func(t *testing.T) {
		roots := semanticRootsForTest(t)
		writeSemanticFixture(t, roots, "before")
		baseline, configDigest := semanticBaselineForTest(t, roots)
		preparedInterface, err := NewSemanticPlanBuilder(roots).Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
		if err != nil {
			t.Fatal(err)
		}
		prepared := preparedInterface.(*preparedSemanticPlan)
		defer prepared.Close()
		held := prepared.stageRoot + ".held"
		if err := os.Rename(prepared.stageRoot, held); err != nil {
			t.Fatal(err)
		}
		copySemanticTestTree(t, held, prepared.stageRoot)
		if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
			t.Fatal("same-content planning stage replacement was removed as trusted residue")
		}
		if _, err := os.Lstat(prepared.stageRoot); err != nil {
			t.Fatalf("untrusted replacement planning stage was mutated: %v", err)
		}
	})

	t.Run("journal-stage", func(t *testing.T) {
		roots := semanticRootsForTest(t)
		writeSemanticFixture(t, roots, "before")
		baseline, configDigest := semanticBaselineForTest(t, roots)
		builder := NewSemanticPlanBuilder(roots)
		builder.fault = stopSemanticJournalAfterPrepared
		prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		if err := prepared.Apply(context.Background()); err == nil {
			t.Fatal("semantic apply did not stop at the prepared journal")
		}
		journalRoot, err := semanticJournalRoot(roots)
		if err != nil {
			t.Fatal(err)
		}
		stage := filepath.Join(journalRoot, "stage")
		held := stage + ".held"
		if err := os.Rename(stage, held); err != nil {
			t.Fatal(err)
		}
		copySemanticTestTree(t, held, stage)
		before, err := CaptureStrict(roots)
		if err != nil {
			t.Fatal(err)
		}
		if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
			t.Fatal("same-content journal stage replacement was accepted")
		}
		after, err := CaptureStrict(roots)
		if err != nil || after.SHA256 != before.SHA256 {
			t.Fatalf("journal stage replacement changed managed state: before=%s after=%s err=%v", before.SHA256, after.SHA256, err)
		}
	})
}

func TestSemanticStartupRejectsSameContentRootAndAncestorSwaps(t *testing.T) {
	t.Run("existing-root", func(t *testing.T) {
		roots := semanticRootsForTest(t)
		writeSemanticFixture(t, roots, "before")
		if err := os.MkdirAll(roots.DurableDir, 0o700); err != nil {
			t.Fatal(err)
		}
		baseline, configDigest := semanticBaselineForTest(t, roots)
		builder := NewSemanticPlanBuilder(roots)
		builder.fault = stopSemanticJournalAfterPrepared
		prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		if err := prepared.Apply(context.Background()); err == nil {
			t.Fatal("semantic apply did not stop at the prepared journal")
		}
		held := roots.DataDir + ".held"
		if err := os.Rename(roots.DataDir, held); err != nil {
			t.Fatal(err)
		}
		copySemanticTestTree(t, held, roots.DataDir)
		if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
			t.Fatal("same-content managed root replacement retained journal authority")
		}
		if got := semanticFixtureValue(t, roots); got != "before" {
			t.Fatalf("replacement managed root was mutated: %q", got)
		}
	})

	t.Run("missing-root-ancestor", func(t *testing.T) {
		outer := t.TempDir()
		ancestor := filepath.Join(outer, "ancestor")
		if err := os.Mkdir(ancestor, 0o700); err != nil {
			t.Fatal(err)
		}
		roots, err := ResolveRootSet(filepath.Join(ancestor, "data"), filepath.Join(ancestor, "durable"))
		if err != nil {
			t.Fatal(err)
		}
		baseline, configDigest := semanticBaselineForTest(t, roots)
		builder := NewSemanticPlanBuilder(roots)
		prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
			if err := os.MkdirAll(filepath.Join(stage.DataDir, "private"), 0o700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(stage.DataDir, "private", "new.bin"), []byte("new"), 0o600)
		})
		if err != nil {
			t.Fatal(err)
		}
		defer prepared.Close()
		held := ancestor + ".held"
		if err := os.Rename(ancestor, held); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(ancestor, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := prepared.Apply(context.Background()); err == nil {
			t.Fatal("same-content nearest ancestor replacement retained cold-root authority")
		}
		if _, err := os.Lstat(roots.DataDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ancestor replacement received managed data: %v", err)
		}
	})
}

func TestSemanticStartupColdRootFirstCreateSwapFailsBeforeJournalApply(t *testing.T) {
	roots := semanticRootsForTest(t)
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.MkdirAll(filepath.Join(stage.DataDir, "private"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, "private", "new.bin"), []byte("new"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := os.Mkdir(roots.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("root created outside the frozen cold-root capability was accepted")
	}
	entries, err := os.ReadDir(roots.DataDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("attacker-created cold root received semantic data: entries=%#v err=%v", entries, err)
	}
}

func TestSemanticStartupCreatesColdManagedRootOnlyThroughFrozenCapability(t *testing.T) {
	roots := semanticRootsForTest(t)
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.MkdirAll(filepath.Join(stage.DataDir, "private"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, "private", "new.bin"), []byte("new"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatalf("frozen cold-root creation failed: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "new.bin"))
	if err != nil || string(body) != "new" {
		t.Fatalf("cold-root semantic result mismatch: body=%q err=%v", body, err)
	}
	if _, held := builder.rootAuthority.Roots(); !held || builder.rootAuthority.Validate() != nil {
		t.Fatal("cold root was not pinned into the live root authority")
	}
}

func TestSemanticStartupStagesAndPublishesEmptyFileWithoutInventingBytes(t *testing.T) {
	roots := semanticRootsForTest(t)
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		private := filepath.Join(stage.DataDir, "private")
		if err := os.MkdirAll(private, 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(private, "empty.bin"), nil, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatalf("empty managed file did not pass exact staging: %v", err)
	}
	info, err := os.Stat(filepath.Join(roots.DataDir, "private", "empty.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("empty managed file changed during publication: size=%d", info.Size())
	}
}

func TestColdRootPromotionCrashFailsClosedWithoutSignedInode(t *testing.T) {
	roots := semanticRootsForTest(t)
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.MkdirAll(filepath.Join(stage.DataDir, "private"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, "private", "signed-intent.bin"), []byte("verified"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	builder.fault = func(stage string, _ int) error {
		if stage == "after_cold_root_create_before_promotion" {
			return errors.New("crash after cold root creation")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("cold-root promotion crash cut did not fire")
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := readSemanticJournalForTest(t, journalRoot)
	if err != nil || len(journal.RootPromotionIntents) == 0 {
		t.Fatalf("cold-root creation was not preceded by a signed intent: intents=%#v err=%v", journal.RootPromotionIntents, err)
	}
	entries, err := os.ReadDir(roots.DataDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("pre-promotion cold root was not empty: entries=%#v err=%v", entries, err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
		t.Fatal("unsigned cold-root inode was promoted after restart")
	}
	if entries, err := os.ReadDir(roots.DataDir); err != nil || len(entries) != 0 {
		t.Fatalf("failed-closed cold root was mutated: entries=%#v err=%v", entries, err)
	}
}

func TestColdRootPromotionCrashRejectsReplacedEmptyWorldWritableDirectory(t *testing.T) {
	roots := semanticRootsForTest(t)
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.MkdirAll(filepath.Join(stage.DataDir, "private"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, "private", "replacement.bin"), []byte("must-not-publish"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	builder.fault = func(stage string, _ int) error {
		if stage == "after_cold_root_create_before_promotion" {
			return errors.New("crash before signed inode promotion")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("cold-root crash cut did not fire")
	}
	hostCreated := roots.DataDir + ".host-created"
	if err := os.Rename(roots.DataDir, hostCreated); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(hostCreated)
	if err := os.Mkdir(roots.DataDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roots.DataDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
		t.Fatal("replacement empty world-writable directory inherited cold-root authority")
	}
	entries, err := os.ReadDir(roots.DataDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("replacement cold root was mutated: entries=%#v err=%v", entries, err)
	}
}

func TestEmptyJournalRootCrashRollsBack(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	builder.fault = func(stage string, _ int) error {
		if stage == "after_journal_root_create" {
			return errors.New("crash after empty journal root creation")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("empty journal-root crash cut did not fire")
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(journalRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("journal root was not empty at crash cut: entries=%#v err=%v", entries, err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err != nil {
		t.Fatalf("empty journal root did not roll back: %v", err)
	}
	if _, err := os.Lstat(journalRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty journal root survived atomic retirement: %v", err)
	}
	assertSemanticFixture(t, roots, "before")
}

func TestPlanningStageSwapDuringSimulationRejected(t *testing.T) {
	roots := semanticRootsForTest(t)
	baseline, configDigest := semanticBaselineForTest(t, roots)
	var originalStage, heldStage string
	builder := NewSemanticPlanBuilder(roots)
	_, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		originalStage = filepath.Dir(stage.DataDir)
		heldStage = originalStage + "-held"
		if err := os.Rename(originalStage, heldStage); err != nil {
			return err
		}
		if err := os.MkdirAll(stage.DataDir, 0o700); err != nil {
			return err
		}
		if canonicalPathKey(stage.DataDir) != canonicalPathKey(stage.DurableDir) {
			if err := os.MkdirAll(stage.DurableDir, 0o700); err != nil {
				return err
			}
		}
		return nil
	})
	if heldStage != "" {
		defer os.RemoveAll(heldStage)
	}
	if originalStage != "" {
		defer os.RemoveAll(originalStage)
	}
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("planning stage replacement was signed into a plan: %v", err)
	}
}

func TestPlanningCleanupRetiresBeforePropagatingFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires host privilege")
	}
	roots := semanticRootsForTest(t)
	baseline, configDigest := semanticBaselineForTest(t, roots)
	preparedInterface, err := NewSemanticPlanBuilder(roots).Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		return os.MkdirAll(filepath.Join(stage.DataDir, "private"), 0o700)
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := preparedInterface.(*preparedSemanticPlan)
	if err := os.Symlink(t.TempDir(), filepath.Join(prepared.stageRoot, "cleanup-attack")); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err == nil {
		t.Fatal("planning cleanup failure was swallowed")
	}
	if _, err := os.Lstat(prepared.stageRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe planning stage was not atomically retired first: %v", err)
	}
	planningRoot, err := semanticPlanningRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(planningRoot)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		found = found || strings.HasPrefix(entry.Name(), ".retired-stage-")
	}
	if !found {
		t.Fatal("cleanup error did not preserve a reentrant retired-stage residue")
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
		t.Fatal("unsafe retired-stage cleanup error was not propagated on recovery")
	}
}

func TestPlanningRecoveryLaterUnknownResidueCausesZeroCleanup(t *testing.T) {
	roots := semanticRootsForTest(t)
	authority, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	planningRoot, err := semanticPlanningRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	planning, err := secureStartupCreateDirectory(authority.planningRoot, filepath.Base(planningRoot))
	if err != nil {
		t.Fatal(err)
	}
	stageNames := []string{
		"stage-00000000000000000000000000000001",
		"stage-00000000000000000000000000000002",
	}
	for _, name := range stageNames {
		stage, err := planning.CreateDirectory(name)
		if err != nil {
			_ = planning.Close()
			t.Fatal(err)
		}
		if err := stage.Close(); err != nil {
			_ = planning.Close()
			t.Fatal(err)
		}
	}
	if err := planning.WriteExclusive("zz-unknown.bin", []byte("unsafe"), 64); err != nil {
		_ = planning.Close()
		t.Fatal(err)
	}
	if err := planning.Close(); err != nil {
		t.Fatal(err)
	}
	if err := recoverSemanticPlanningStage(roots, authority); err == nil {
		t.Fatal("later unknown planning residue was accepted")
	}
	for _, name := range stageNames {
		if _, err := os.Lstat(filepath.Join(planningRoot, name)); err != nil {
			t.Fatalf("planning recovery partially cleaned %s: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(planningRoot, "zz-unknown.bin")); err != nil {
		t.Fatalf("planning recovery mutated unknown residue: %v", err)
	}
}

func TestSemanticStartupPlanAppliesOneSnapshotBoundFixedPoint(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatalf("prepare semantic startup plan: %v", err)
	}
	defer prepared.Close()
	if len(prepared.Plan().Operations) == 0 || domainstartup.ValidateSemanticStartupPlanV1(prepared.Plan()) != nil {
		t.Fatalf("semantic startup plan did not bind mutations: %#v", prepared.Plan())
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatalf("apply semantic startup plan: %v", err)
	}
	assertSemanticFixture(t, roots, "after")
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(journalRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed semantic journal was not retired: %v", err)
	}
}

func TestSemanticStartupStageResidueRecoveredWithoutPrivateLeak(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	if err := os.WriteFile(filepath.Join(roots.DataDir, "private", "secret.bin"), []byte("PRIVATE_STAGE_SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	preparedInterface, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	prepared, ok := preparedInterface.(*preparedSemanticPlan)
	if !ok {
		t.Fatal("semantic prepared plan implementation is unavailable")
	}
	stageRoot := prepared.stageRoot
	expectedPlanningRoot, err := semanticPlanningRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(stageRoot) != expectedPlanningRoot || !strings.HasPrefix(filepath.Base(stageRoot), "stage-") ||
		!strings.Contains(filepath.Base(expectedPlanningRoot), rootBindingDigest(roots)) {
		t.Fatalf("semantic stage is not root-bound to the private startup namespace: %s", stageRoot)
	}
	staged, err := os.ReadFile(filepath.Join(prepared.stageRoots.DataDir, "private", "secret.bin"))
	if err != nil || string(staged) != "PRIVATE_STAGE_SENTINEL" {
		t.Fatalf("semantic stage fixture mismatch: body=%q err=%v", staged, err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err != nil {
		t.Fatalf("recover crashed semantic planning stage: %v", err)
	}
	if _, err := os.Lstat(stageRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private semantic planning stage survived recovery: %v", err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatalf("closing already-recovered planning stage was not idempotent: %v", err)
	}
}

func TestStartupJournalEveryInstallCrashCutReachesOneFixedPoint(t *testing.T) {
	for _, cut := range []string{
		"after_journal_prepare",
		"after_stage_sync",
		"after_journal_prepared",
		"after_operation",
		"after_operation_journal",
		"before_commit",
		"after_commit",
	} {
		t.Run(cut, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			writeSemanticFixture(t, roots, "before")
			baseline, configDigest := semanticBaselineForTest(t, roots)
			builder := NewSemanticPlanBuilder(roots)
			fired := false
			builder.fault = func(stage string, _ int) error {
				if !fired && stage == cut {
					fired = true
					return errors.New("simulated abrupt startup cut")
				}
				return nil
			}
			prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
			if err != nil {
				t.Fatalf("prepare semantic plan: %v", err)
			}
			defer prepared.Close()
			if err := prepared.Apply(context.Background()); err == nil || !fired {
				t.Fatalf("fault cut did not interrupt apply: fired=%v err=%v", fired, err)
			}

			restarted := NewSemanticPlanBuilder(roots)
			if err := restarted.Recover(context.Background(), semanticConfigurationDigestForTest()); err != nil {
				t.Fatalf("recover semantic journal after %s: %v", cut, err)
			}
			if cut == "after_journal_prepare" || cut == "after_stage_sync" {
				assertSemanticFixture(t, roots, "before")
				baseline, configDigest = semanticBaselineForTest(t, roots)
				retry, err := restarted.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
				if err != nil {
					t.Fatalf("replan rolled-back semantic journal: %v", err)
				}
				defer retry.Close()
				if err := retry.Apply(context.Background()); err != nil {
					t.Fatalf("apply replanned semantic journal: %v", err)
				}
			}
			assertSemanticFixture(t, roots, "after")
		})
	}
}

func TestStartupJournalNonzeroCursorRemainsMonotonicAcrossSecondCrash(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticMultiOperationSimulationForTest)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if len(prepared.Plan().Operations) < 3 {
		t.Fatalf("multi-operation fixture is too small: %#v", prepared.Plan().Operations)
	}
	firstCursor := 2
	builder.fault = func(stage string, operation int) error {
		if stage == "after_operation_journal" && operation == firstCursor-1 {
			return errors.New("first crash after durable nonzero cursor")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("first crash did not interrupt semantic apply")
	}
	jRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	j, err := readSemanticJournalForTest(t, jRoot)
	if err != nil || j.NextOperation != firstCursor || j.State != semanticJournalApplying {
		t.Fatalf("first crash did not preserve the nonzero cursor: journal=%#v err=%v", j, err)
	}

	restarted := NewSemanticPlanBuilder(roots)
	secondCrashOperation := -1
	restarted.fault = func(stage string, operation int) error {
		if stage == "after_operation_journal" && secondCrashOperation < 0 {
			secondCrashOperation = operation
			return errors.New("second crash during journal recovery")
		}
		return nil
	}
	if err := restarted.Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
		t.Fatal("second crash did not interrupt journal recovery")
	}
	if secondCrashOperation != firstCursor {
		t.Fatalf("journal recovery replayed an already committed operation: cursor=%d operation=%d", firstCursor, secondCrashOperation)
	}
	j, err = readSemanticJournalForTest(t, jRoot)
	if err != nil || j.NextOperation != firstCursor+1 {
		t.Fatalf("second crash regressed the durable cursor: journal=%#v err=%v", j, err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err != nil {
		t.Fatalf("third restart did not converge after two crashes: %v", err)
	}
	assertSemanticMultiOperationFixture(t, roots)
}

func TestStartupJournalEveryAtomicReplaceCrashCutConverges(t *testing.T) {
	for _, cut := range []string{"after_journal_temp_sync", "after_journal_replace"} {
		t.Run(cut, func(t *testing.T) {
			occurrences := countSemanticJournalCutOccurrences(t, cut)
			if occurrences < 4 {
				t.Fatalf("journal fixture did not exercise enough replacements: cut=%s occurrences=%d", cut, occurrences)
			}
			for occurrence := 1; occurrence <= occurrences; occurrence++ {
				t.Run(fmt.Sprintf("%02d", occurrence), func(t *testing.T) {
					roots := semanticRootsForTest(t)
					writeSemanticFixture(t, roots, "before")
					baseline, configDigest := semanticBaselineForTest(t, roots)
					builder := NewSemanticPlanBuilder(roots)
					seen := 0
					builder.fault = func(stage string, _ int) error {
						if stage != cut {
							return nil
						}
						seen++
						if seen == occurrence {
							return errors.New("atomic journal crash cut")
						}
						return nil
					}
					prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticMultiOperationSimulationForTest)
					if err != nil {
						t.Fatal(err)
					}
					defer prepared.Close()
					if err := prepared.Apply(context.Background()); err == nil || seen != occurrence {
						t.Fatalf("journal crash cut was not reached: cut=%s occurrence=%d seen=%d err=%v", cut, occurrence, seen, err)
					}
					restarted := NewSemanticPlanBuilder(roots)
					if err := restarted.Recover(context.Background(), semanticConfigurationDigestForTest()); err != nil {
						t.Fatalf("recover journal cut %s/%d: %v", cut, occurrence, err)
					}
					if semanticFixtureValue(t, roots) == "before" {
						baseline, configDigest = semanticBaselineForTest(t, roots)
						retry, err := restarted.Prepare(context.Background(), baseline, configDigest, semanticMultiOperationSimulationForTest)
						if err != nil {
							t.Fatal(err)
						}
						defer retry.Close()
						if err := retry.Apply(context.Background()); err != nil {
							t.Fatalf("reapply rolled-back journal cut %s/%d: %v", cut, occurrence, err)
						}
					}
					assertSemanticMultiOperationFixture(t, roots)
				})
			}
		})
	}
}

func TestStartupJournalLossCannotAdmitPartialGeneration(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticMultiOperationSimulationForTest)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	fired := false
	builder.fault = func(stage string, operation int) error {
		if stage == "after_operation" && operation == 0 {
			fired = true
			return errors.New("crash with one live operation and stale cursor")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil || !fired {
		t.Fatalf("partial-generation crash cut did not fire: fired=%v err=%v", fired, err)
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	userConfigRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if relative, relErr := filepath.Rel(userConfigRoot, journalRoot); relErr != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("startup recovery authority escaped the isolated persistent user-config namespace: %s", journalRoot)
	}
	legacyLeaseRoot, err := defaultLeaseDirectory()
	if err != nil {
		t.Fatal(err)
	}
	legacyJournal := filepath.Join(legacyLeaseRoot, "startup-"+rootBindingDigest(roots))
	if relative, relErr := filepath.Rel(legacyJournal, journalRoot); relErr == nil &&
		relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("persistent startup journal remained inside the legacy volatile lease target: %s", journalRoot)
	}
	if err := os.RemoveAll(legacyJournal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(journalRoot); err != nil {
		t.Fatalf("temp cleanup removed persistent startup journal: %v", err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err != nil {
		t.Fatalf("persistent journal did not recover partial generation: %v", err)
	}
	assertSemanticMultiOperationFixture(t, roots)
}

func TestStartupRetiredJournalCrashCleanupConverges(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	builder.fault = func(stage string, _ int) error {
		if stage == "after_journal_retired" {
			return errors.New("crash after atomic journal retirement")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("retired-journal crash cut did not fire")
	}
	namespace, err := persistentStartupNamespacePath(roots, false)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(namespace)
	if err != nil {
		t.Fatal(err)
	}
	foundRetired := false
	for _, entry := range entries {
		foundRetired = foundRetired || strings.HasPrefix(entry.Name(), ".retired-journal-")
	}
	if !foundRetired {
		t.Fatal("atomic retirement crash did not leave a recoverable retired directory")
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err != nil {
		t.Fatalf("retired journal cleanup did not converge: %v", err)
	}
	entries, err = os.ReadDir(namespace)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".retired-journal-") {
			t.Fatalf("retired journal survived recovery: %s", entry.Name())
		}
	}
	assertSemanticFixture(t, roots, "after")
}

func TestRetiredRecoveryPreflightsAllBeforeDeletingAny(t *testing.T) {
	roots := semanticRootsForTest(t)
	_, configurationDigest := semanticBaselineForTest(t, roots)
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openOrCreateStartupJournalAuthority(namespace, journalRoot); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		directory, err := secureStartupCreateDirectory(namespace.root, "journal")
		if err != nil {
			t.Fatal(err)
		}
		identity := directory.Identity()
		if err := directory.Close(); err != nil {
			t.Fatal(err)
		}
		fired := false
		err = retireSemanticJournal(namespace, identity, roots, configurationDigest, func(stage string, _ int) error {
			if stage == "after_journal_retired" {
				fired = true
				return errors.New("retain retired journal fixture")
			}
			return nil
		})
		if err == nil || !fired {
			t.Fatalf("retired fixture %d did not stop: fired=%v err=%v", index, fired, err)
		}
	}
	entries, err := os.ReadDir(namespace.path())
	if err != nil {
		t.Fatal(err)
	}
	retired := make([]string, 0, 2)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".retired-journal-") {
			retired = append(retired, entry.Name())
		}
	}
	sort.Strings(retired)
	if len(retired) != 2 {
		t.Fatalf("retired fixture count = %d", len(retired))
	}
	if err := os.WriteFile(filepath.Join(namespace.path(), retired[1], "unknown.bin"), []byte("unsafe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recoverRetiredSemanticJournals(namespace, roots); err == nil {
		t.Fatal("later unsafe retired journal was accepted")
	}
	for _, name := range retired {
		if _, err := os.Lstat(filepath.Join(namespace.path(), name)); err != nil {
			t.Fatalf("retired preflight partially cleaned %s: %v", name, err)
		}
	}
}

func TestSemanticJournalTempNameRequiresExact32LowerHex(t *testing.T) {
	root := t.TempDir()
	names := []string{
		".startup-replace-0123456789abcdef0123456789abcdef.tmp",
		".startup-replace-short.tmp",
		".startup-replace-0123456789ABCDEF0123456789ABCDEF.tmp",
		".startup-replace-0123456789abcdef0123456789abcdef0.tmp",
		".startup-replace-0123456789abcdef0123456789abcdeg.tmp",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		valid := validSemanticJournalTempName(entry)
		expected := entry.Name() == names[0]
		if valid != expected {
			t.Fatalf("temp name %q validity = %v, want %v", entry.Name(), valid, expected)
		}
	}
}

func TestRenamedApplyingJournalIsNotCleanupAuthority(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticMultiOperationSimulationForTest)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	builder.fault = func(stage string, operation int) error {
		if stage == "after_operation_journal" && operation == 0 {
			return errors.New("crash with an applying frontier")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("applying journal crash cut did not fire")
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := readSemanticJournalForTest(t, journalRoot)
	if err != nil || journal.State != semanticJournalApplying || journal.NextOperation != 1 {
		t.Fatalf("fixture did not preserve an applying frontier: journal=%#v err=%v", journal, err)
	}
	retiredRoot := filepath.Join(filepath.Dir(journalRoot), ".retired-journal-0123456789abcdef0123456789abcdef")
	if err := os.Rename(journalRoot, retiredRoot); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
		t.Fatal("retired-looking directory name deleted an active applying journal")
	}
	if _, err := os.Stat(filepath.Join(retiredRoot, "journal.json")); err != nil {
		t.Fatalf("unproven applying journal was not preserved for recovery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(retiredRoot, semanticJournalRetirementFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery minted deletion authority for an attacker-renamed journal: %v", err)
	}
}

func TestJournalFileRejectsSymlinkHardlinkAndParentSwap(t *testing.T) {
	for _, mode := range []string{"symlink", "hardlink", "parent-swap"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "symlink" && runtime.GOOS == "windows" {
				t.Skip("Windows symlink creation requires host privilege")
			}
			roots := semanticRootsForTest(t)
			writeSemanticFixture(t, roots, "before")
			baseline, configDigest := semanticBaselineForTest(t, roots)
			builder := NewSemanticPlanBuilder(roots)
			builder.fault = stopSemanticJournalAfterPrepared
			prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			if err := prepared.Apply(context.Background()); err == nil {
				t.Fatal("prepared-journal fixture did not stop")
			}
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			journalPath := filepath.Join(journalRoot, "journal.json")
			switch mode {
			case "symlink":
				body, err := os.ReadFile(journalPath)
				if err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "journal.json")
				if err := os.WriteFile(target, body, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(journalPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, journalPath); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(journalPath, filepath.Join(journalRoot, "journal-alias.json")); err != nil {
					t.Fatal(err)
				}
			case "parent-swap":
				held := journalRoot + "-held"
				if err := os.Rename(journalRoot, held); err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(held)
				if err := os.Mkdir(journalRoot, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := copyTreeForJournalSwap(held, journalRoot); err != nil {
					t.Fatal(err)
				}
			}
			before, err := CaptureStrict(roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configDigest); err == nil {
				t.Fatalf("%s journal attack was accepted", mode)
			}
			after, err := CaptureStrict(roots)
			if err != nil || after.SHA256 != before.SHA256 {
				t.Fatalf("%s journal attack mutated managed state: before=%s after=%s err=%v", mode, before.SHA256, after.SHA256, err)
			}
		})
	}
}

func copyTreeForJournalSwap(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.Mkdir(destination, 0o700)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, body, 0o600)
	})
}

func TestStartupJournalTargetNeitherPreNorPostPoisonsWithoutMutation(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	builder.fault = func(stage string, _ int) error {
		if stage == "after_journal_prepared" {
			return errors.New("stop before first target")
		}
		return nil
	}
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("prepared journal did not stop before the first target")
	}
	foreign := []byte("foreign-state")
	target := filepath.Join(roots.DataDir, "private", "fixture.bin")
	if err := os.WriteFile(target, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
		t.Fatal("foreign state was accepted as a journal pre/post state")
	}
	after, err := os.ReadFile(target)
	if err != nil || string(after) != string(before) {
		t.Fatalf("poisoned recovery mutated the foreign target: before=%q after=%q err=%v", before, after, err)
	}
}

func TestSemanticStartupInitialJournalTempWithoutAuthorityFailsClosed(t *testing.T) {
	roots := semanticRootsForTest(t)
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journalRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalRoot, ".startup-replace-crash.tmp"), []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
		t.Fatal("unsigned initial journal temp was accepted without installation authority")
	}
	if _, err := os.Lstat(journalRoot); err != nil {
		t.Fatalf("unsigned initial journal temp was mutated: %v", err)
	}
}

func TestSemanticStartupMissingJournalRejectsUnknownOrAmbiguousResidue(t *testing.T) {
	for _, test := range []struct {
		name   string
		poison func(*testing.T, string)
	}{
		{name: "unknown", poison: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "ambiguous", poison: func(t *testing.T, root string) {
			for _, name := range []string{".startup-replace-one.tmp", ".startup-replace-two.tmp"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}},
		{name: "hardlink", poison: func(t *testing.T, root string) {
			path := filepath.Join(root, ".startup-replace-linked.tmp")
			if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(path, filepath.Join(t.TempDir(), "alias")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(journalRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			test.poison(t, journalRoot)
			if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
				t.Fatal("unsafe missing-journal residue was removed")
			}
			if _, err := os.Lstat(journalRoot); err != nil {
				t.Fatalf("unsafe journal residue was mutated: %v", err)
			}
		})
	}
}

func TestSemanticStartupPartialInstallTempRebuildsFromJournalBlob(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	var install domainstartup.SemanticStartupOperationV1
	installIndex := -1
	for index, operation := range prepared.Plan().Operations {
		if operation.Kind == domainstartup.SemanticOperationInstallFile && operation.Path == "data/private/fixture.bin" {
			install = operation
			installIndex = index
		}
	}
	if install.OperationID == "" {
		t.Fatalf("fixture install operation is missing: %#v", prepared.Plan().Operations)
	}
	builder.fault = func(stage string, operation int) error {
		if stage == "before_operation" && operation == installIndex {
			return errors.New("stop during install")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop during install")
	}
	target, err := managedPath(roots, install.Path)
	if err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".startup-"+install.OperationID+".tmp")
	if err := os.WriteFile(temporary, []byte("pa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err != nil {
		t.Fatalf("recover partial deterministic install temp: %v", err)
	}
	assertSemanticFixture(t, roots, "after")
	if _, err := os.Lstat(temporary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial deterministic install temp survived recovery: %v", err)
	}
}

func TestLegacySignedSemanticJournalRemovesCaseThreadCASResidueBeforeOwnerRecovery(t *testing.T) {
	roots := semanticRootsForTest(t)
	digest := "ab" + strings.Repeat("c", 62)
	label := "data/private/case-thread-authority/" + digest[:2] + "/." + digest + ".json-000000000000000000000001.tmp"
	temp, err := managedPath(roots, label)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(temp), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temp, []byte("legacy-owner-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureStrict(roots); err == nil {
		t.Fatal("ordinary startup snapshot accepted case-thread CAS residue")
	}
	before, err := captureStrictWithAllowedPrivateResidues(
		context.Background(), roots, nil, map[string]struct{}{label: {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := managedSnapshotFromRaw(before)
	if err != nil {
		t.Fatal(err)
	}
	configurationDigest := domainsecurity.SHA256Hex([]byte("legacy-case-thread-cas-journal"))
	rootAuthority, err := FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	journalAuthority, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	stageRoot, stageRoots, stageIdentity, planningIdentity, err := createSemanticStage(roots, journalAuthority)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = removeSemanticPlanningStage(roots, journalAuthority, stageRoot, stageIdentity) }()
	if err := copyManagedSnapshotToStage(context.Background(), before, stageRoots); err != nil {
		t.Fatal(err)
	}
	stageTemp, err := managedPath(stageRoots, label)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stageTemp); err != nil {
		t.Fatal(err)
	}
	after, err := CaptureStrict(stageRoots)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := semanticOperations(before, after)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domainstartup.NewSemanticStartupPlanV1(
		baseline.SnapshotDigest, configurationDigest, semanticStateDigest(after.Entries), operations,
	)
	if err != nil || len(plan.Operations) == 0 {
		t.Fatalf("build legacy owner-residue plan: operations=%#v err=%v", plan.Operations, err)
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareSemanticJournal(
		context.Background(), journalRoot, roots, rootAuthority, journalAuthority, plan,
		stageRoot, stageRoots, stageIdentity, planningIdentity, nil,
	); err != nil {
		t.Fatal(err)
	}

	builder := NewSemanticPlanBuilderWithAuthorities(roots, rootAuthority, journalAuthority)
	if err := builder.Recover(context.Background(), configurationDigest); err != nil {
		t.Fatalf("recover signed legacy owner-residue journal: %v", err)
	}
	if _, err := os.Lstat(temp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("signed legacy owner-residue delete was not applied: %v", err)
	}
	if _, err := CaptureStrict(roots); err != nil {
		t.Fatalf("post-recovery managed state is not canonical: %v", err)
	}
}

func TestSemanticStartupRecoveryRejectsUnchangedEntryDriftBeforeFirstWrite(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	unchanged := filepath.Join(roots.DataDir, "private", "unchanged.bin")
	if err := os.WriteFile(unchanged, []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	builder.fault = func(stage string, _ int) error {
		if stage == "after_journal_prepared" {
			return errors.New("stop before first mutation")
		}
		return nil
	}
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop before first mutation")
	}
	if err := os.WriteFile(unchanged, []byte("foreign-drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
		t.Fatal("unchanged-entry drift was accepted during journal recovery")
	}
	fixture, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "fixture.bin"))
	if err != nil || string(fixture) != "before" {
		t.Fatalf("recovery mutated a planned target before rejecting baseline drift: body=%q err=%v", fixture, err)
	}
	if _, err := os.Lstat(filepath.Join(roots.DataDir, "private", "planned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery created a planned directory before rejecting baseline drift: %v", err)
	}
	drift, err := os.ReadFile(unchanged)
	if err != nil || string(drift) != "foreign-drift" {
		t.Fatalf("recovery rewrote the foreign drift: body=%q err=%v", drift, err)
	}
}

func TestSemanticStartupLateStageBlobCorruptionCausesZeroManagedMutation(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	builder.fault = func(stage string, _ int) error {
		if stage == "after_journal_prepared" {
			return errors.New("stop with prepared stage")
		}
		return nil
	}
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop with prepared stage")
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	var late domainstartup.SemanticStartupOperationV1
	for _, operation := range prepared.Plan().Operations {
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			late = operation
		}
	}
	if late.OperationID == "" {
		t.Fatal("semantic fixture has no staged install")
	}
	corrupt := make([]byte, late.After.Size)
	for index := range corrupt {
		corrupt[index] = 'x'
	}
	if err := os.WriteFile(filepath.Join(journalRoot, "stage", late.OperationID), corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
		t.Fatal("late stage corruption was accepted")
	}
	after, err := CaptureStrict(roots)
	if err != nil || after.SHA256 != before.SHA256 {
		t.Fatalf("late stage corruption caused a managed mutation: before=%s after=%s err=%v", before.SHA256, after.SHA256, err)
	}
	journal, err := readSemanticJournalForTest(t, journalRoot)
	if err != nil || journal.State != semanticJournalPrepared || journal.NextOperation != 0 {
		t.Fatalf("late stage corruption advanced the journal: journal=%#v err=%v", journal, err)
	}
}

func TestSemanticStartupInstallTempRequiresBeforeState(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	installIndex := -1
	var install domainstartup.SemanticStartupOperationV1
	for index, operation := range prepared.Plan().Operations {
		if operation.Kind == domainstartup.SemanticOperationInstallFile && operation.Path == "data/private/fixture.bin" {
			installIndex, install = index, operation
		}
	}
	builder.fault = func(stage string, operation int) error {
		if stage == "after_operation" && operation == installIndex {
			return errors.New("stop after install before cursor")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop after install")
	}
	target, err := managedPath(roots, install.Path)
	if err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".startup-"+install.OperationID+".tmp")
	if err := os.WriteFile(temporary, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeTarget, _ := os.ReadFile(target)
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err == nil {
		t.Fatal("after-state target plus install temp was accepted")
	}
	afterTarget, err := os.ReadFile(target)
	if err != nil || string(afterTarget) != string(beforeTarget) {
		t.Fatalf("impossible install-temp recovery changed target: before=%q after=%q err=%v", beforeTarget, afterTarget, err)
	}
	if _, err := os.Lstat(temporary); err != nil {
		t.Fatalf("impossible install temp was mutated: %v", err)
	}
}

func TestLegacyMigrationEveryCrashCutPreservesSourceUntilRetirementAndConverges(t *testing.T) {
	for cut := 0; ; cut++ {
		roots := semanticRootsForTest(t)
		legacy := filepath.Join(roots.DurableDir, "runtime-go", "threads", "thread-a", "fixture.bin")
		if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
			t.Fatal(err)
		}
		baseline, configDigest := semanticBaselineForTest(t, roots)
		builder := NewSemanticPlanBuilder(roots)
		prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
			source := filepath.Join(stage.DurableDir, "runtime-go", "threads", "thread-a", "fixture.bin")
			body, err := os.ReadFile(source)
			if err != nil {
				return err
			}
			target := filepath.Join(stage.DurableDir, "threads", "thread-a", "fixture.bin")
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(target, body, 0o600); err != nil {
				return err
			}
			return os.RemoveAll(filepath.Join(stage.DurableDir, "runtime-go", "threads"))
		})
		if err != nil {
			t.Fatal(err)
		}
		operations := prepared.Plan().Operations
		if cut >= len(operations) {
			_ = prepared.Close()
			break
		}
		builder.fault = func(stage string, operation int) error {
			if stage == "after_operation" && operation == cut {
				return errors.New("legacy migration crash cut")
			}
			return nil
		}
		if err := prepared.Apply(context.Background()); err == nil {
			_ = prepared.Close()
			t.Fatalf("legacy migration cut %d did not interrupt apply", cut)
		}
		if operations[cut].Kind != domainstartup.SemanticOperationRemoveFile && operations[cut].Kind != domainstartup.SemanticOperationRemoveDirectory {
			if body, err := os.ReadFile(legacy); err != nil || string(body) != "legacy" {
				_ = prepared.Close()
				t.Fatalf("legacy source retired before target install completed at cut %d: body=%q err=%v", cut, body, err)
			}
		}
		if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), semanticConfigurationDigestForTest()); err != nil {
			_ = prepared.Close()
			t.Fatalf("recover legacy migration cut %d: %v", cut, err)
		}
		target := filepath.Join(roots.DurableDir, "threads", "thread-a", "fixture.bin")
		if body, err := os.ReadFile(target); err != nil || string(body) != "legacy" {
			_ = prepared.Close()
			t.Fatalf("legacy target did not converge at cut %d: body=%q err=%v", cut, body, err)
		}
		if _, err := os.Lstat(legacy); !errors.Is(err, os.ErrNotExist) {
			_ = prepared.Close()
			t.Fatalf("legacy source remained after committed recovery at cut %d: %v", cut, err)
		}
		_ = prepared.Close()
	}
}

func semanticRootsForTest(t *testing.T) RootSet {
	t.Helper()
	base := t.TempDir()
	roots, err := ResolveRootSet(filepath.Join(base, "data"), filepath.Join(base, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	if namespace, err := persistentStartupNamespacePath(roots, false); err == nil {
		t.Cleanup(func() { _ = os.RemoveAll(namespace) })
	}
	if planningRoot, err := semanticPlanningRoot(roots); err == nil {
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(planningRoot)) })
	}
	return roots
}

func writeSemanticFixture(t *testing.T, roots RootSet, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(roots.DataDir, "private"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roots.DataDir, "private", "fixture.bin"), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func semanticBaselineForTest(t *testing.T, roots RootSet) (domainstartup.ReadOnlyStartupBaselineV1, string) {
	t.Helper()
	snapshot, err := NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	configurationDigest := semanticConfigurationDigestForTest()
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, configurationDigest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return baseline, configurationDigest
}

func semanticConfigurationDigestForTest() string {
	return domainsecurity.SHA256Hex([]byte("semantic-config"))
}

func stopSemanticJournalAfterPrepared(stage string, _ int) error {
	if stage == "after_journal_prepared" {
		return errors.New("stop after prepared journal")
	}
	return nil
}

func readSemanticJournalForTest(t *testing.T, root string) (semanticJournalV1, error) {
	t.Helper()
	namespace, err := freezeJournalNamespaceAuthorityAt(filepath.Dir(root))
	if err != nil {
		return semanticJournalV1{}, err
	}
	authority, err := loadStartupJournalAuthority(namespace, root)
	if err != nil {
		return semanticJournalV1{}, err
	}
	return readSemanticJournal(root, authority)
}

func semanticSimulationForTest(value string) startupport.SemanticSimulationV1 {
	return func(_ context.Context, roots startupport.PersistenceRootsV1) error {
		path := filepath.Join(roots.DataDir, "private", "fixture.bin")
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			return err
		}
		directory := filepath.Join(roots.DataDir, "private", "planned")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(directory, "receipt.bin"), []byte("planned"), 0o600)
	}
}

func semanticMultiOperationSimulationForTest(_ context.Context, roots startupport.PersistenceRootsV1) error {
	private := filepath.Join(roots.DataDir, "private")
	for _, name := range []string{"planned-a", "planned-b"} {
		if err := os.Mkdir(filepath.Join(private, name), 0o700); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(private, "fixture.bin"), []byte("after"), 0o600)
}

func assertSemanticMultiOperationFixture(t *testing.T, roots RootSet) {
	t.Helper()
	if got := semanticFixtureValue(t, roots); got != "after" {
		t.Fatalf("semantic multi-operation fixture mismatch: %q", got)
	}
	for _, name := range []string{"planned-a", "planned-b"} {
		info, err := os.Stat(filepath.Join(roots.DataDir, "private", name))
		if err != nil || !info.IsDir() {
			t.Fatalf("semantic planned directory %s is missing: info=%v err=%v", name, info, err)
		}
	}
}

func semanticFixtureValue(t *testing.T, roots RootSet) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "fixture.bin"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func countSemanticJournalCutOccurrences(t *testing.T, cut string) int {
	t.Helper()
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	count := 0
	builder.fault = func(stage string, _ int) error {
		if stage == cut {
			count++
		}
		return nil
	}
	prepared, err := builder.Prepare(context.Background(), baseline, configDigest, semanticMultiOperationSimulationForTest)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertSemanticFixture(t *testing.T, roots RootSet, value string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "fixture.bin"))
	if err != nil || string(body) != value {
		t.Fatalf("semantic fixture mismatch: body=%q err=%v", body, err)
	}
	planned, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "planned", "receipt.bin"))
	if value == "after" {
		if err != nil || string(planned) != "planned" {
			t.Fatalf("semantic planned output mismatch: body=%q err=%v", planned, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-state unexpectedly contains planned output: body=%q err=%v", planned, err)
	}
}

func copySemanticTestTree(t *testing.T, source, target string) {
	t.Helper()
	type directoryTime struct {
		path string
		mode os.FileMode
		time time.Time
	}
	directories := []directoryTime{}
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("semantic test tree contains a symlink")
		}
		if info.IsDir() {
			if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
				return err
			}
			directories = append(directories, directoryTime{path: destination, mode: info.Mode().Perm(), time: info.ModTime()})
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("semantic test tree contains a special file")
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := errors.Join(input.Close(), output.Sync(), output.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		return os.Chtimes(destination, info.ModTime(), info.ModTime())
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := len(directories) - 1; index >= 0; index-- {
		directory := directories[index]
		if err := os.Chmod(directory.path, directory.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(directory.path, directory.time, directory.time); err != nil {
			t.Fatal(err)
		}
	}
}
