package runtimeapp

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceregistryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestRuntimeHostLocalProfileClassificationRequiresExactEmptyOrCommittedPair(t *testing.T) {
	tests := []struct {
		name                                              string
		witness, protected, selector, history, simulation bool
		want                                              runtimeHostLocalProfileDecisionV1
	}{
		{"fresh empty", false, false, false, false, false, runtimeHostLocalProfileAdmitV1},
		{"existing pair", false, true, true, true, false, runtimeHostLocalProfileResumeV1},
		{"selector only", false, false, true, false, false, runtimeHostLocalProfileUnavailableV1},
		{"head only", false, false, false, true, false, runtimeHostLocalProfileUnavailableV1},
		{"protected old state", false, true, false, false, false, runtimeHostLocalProfileUnavailableV1},
		{"witnessed empty", true, false, false, false, false, runtimeHostLocalProfileUnavailableV1},
		{"mixed witnessed and host", true, true, true, true, false, runtimeHostLocalProfileUnavailableV1},
		{"semantic simulation", false, false, false, false, true, runtimeHostLocalProfileUnavailableV1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyRuntimeHostLocalProfileV1(test.witness, test.protected,
				test.selector, test.history, test.simulation); got != test.want {
				t.Fatalf("host-local mode decision mismatch: got=%d want=%d", got, test.want)
			}
		})
	}
}

func TestRuntimeHostLocalProfileReadOnlyRootInventoryRejectsSymlink(t *testing.T) {
	dataDir := t.TempDir()
	selector, history, err := runtimeHostLocalProfileRootsPresentV1(dataDir)
	if err != nil || selector || history {
		t.Fatalf("absent roots were not classified read-only: %v", err)
	}
	privateRoot := filepath.Join(dataDir, "private")
	if err := os.Mkdir(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(privateRoot, "evidence-authority-host-local"), 0o700); err != nil {
		t.Fatal(err)
	}
	selector, history, err = runtimeHostLocalProfileRootsPresentV1(dataDir)
	if err != nil || selector || !history {
		t.Fatalf("single-sided head residue was not visible: %v", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(privateRoot, "evidence-authority-host-local-projection")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtimeHostLocalProfileRootsPresentV1(dataDir); err == nil {
		t.Fatal("selector symlink was accepted as a host-local mode commitment")
	}
}

func TestRuntimeHostLocalOrdinaryStartupDoesNotCommitModeHead(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(),
		DurableTempDir: t.TempDir(), UserDataDir: t.TempDir()}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownOwnedRuntimeHandler(t, handler)
	selector, history, err := runtimeHostLocalProfileRootsPresentV1(config.DataDir)
	if err != nil || selector || history {
		t.Fatalf("ordinary Core startup committed host-local mode: selector=%t history=%t err=%v", selector, history, err)
	}
}

func TestRuntimeHostLocalOwnerNestedBorrowAndClose(t *testing.T) {
	owner := &runtimeHostLocalEvidenceOwnersV1{
		snapshot: &datasetsnapshotapp.HostLocalSealedServiceV2{},
		registry: &evidenceregistryapp.HostLocalServiceV3{},
	}
	owner.idle = sync.NewCond(&owner.mu)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, releaseOuter, err := owner.borrow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, _, releaseInner, err := owner.borrow(ctx)
	if err != nil {
		releaseOuter()
		t.Fatalf("nested snapshot-to-registry borrow deadlocked: %v", err)
	}
	closed := make(chan error, 1)
	go func() { closed <- owner.Close() }()
	select {
	case err := <-closed:
		releaseInner()
		releaseOuter()
		t.Fatalf("owner closed active CAS borrows: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	releaseInner()
	releaseOuter()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("owner Close did not drain the nested borrow")
	}
}

func TestRuntimeHostLocalFundsEffectAloneCommitsAndResumesEmptyMode(t *testing.T) {
	ctx := context.Background()
	config := Config{DataDir: t.TempDir(), DurableTempDir: t.TempDir(), UserDataDir: t.TempDir()}
	privateRoot := filepath.Join(config.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	root, err := persistencefs.FreezeRootAuthority(persistencefs.RootSet{
		DataDir: config.DataDir, DurableDir: config.DurableTempDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	stores, err := datasetsnapshotstore.OpenStoresV2(filepath.Join(privateRoot, "dataset-snapshot-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer stores.Close()
	checked := 0
	owner, err := newRuntimeHostLocalEvidenceOwnersV1(config, root, access, key, stores,
		runtimeHostLocalProfileAdmitV1, func(context.Context) error { checked++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	selector, history, err := runtimeHostLocalProfileRootsPresentV1(config.DataDir)
	if err != nil || selector || history {
		t.Fatal("composition created a mode marker before Funds effect")
	}
	if _, err := owner.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{}); err == nil {
		t.Fatal("an empty profile exposed a snapshot before admission")
	}
	selector, history, err = runtimeHostLocalProfileRootsPresentV1(config.DataDir)
	if err != nil || selector || history {
		t.Fatal("read-only snapshot lookup created a mode marker")
	}
	if err := owner.EnsureForFundsImport(ctx); err != nil {
		t.Fatal(err)
	}
	if checked != 1 || owner.CaseEvidenceAuthorityUnavailableV1() {
		t.Fatal("Funds effect did not activate exact local owners")
	}
	selector, history, err = runtimeHostLocalProfileRootsPresentV1(config.DataDir)
	if err != nil || !selector || !history {
		t.Fatal("Funds effect did not commit both mode roots")
	}
	before, found, err := owner.heads.Current(ctx)
	if err != nil || !found || before.Generation != 0 {
		t.Fatalf("signed empty mode head unavailable: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	root, err = persistencefs.FreezeRootAuthority(persistencefs.RootSet{
		DataDir: config.DataDir, DurableDir: config.DurableTempDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := newRuntimeHostLocalEvidenceOwnersV1(config, root, access, key, stores,
		runtimeHostLocalProfileResumeV1, func(context.Context) error { t.Fatal("resume rechecked fresh admission"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if err := resumed.ensureRead(ctx); err != nil {
		t.Fatal(err)
	}
	after, found, err := resumed.heads.Current(ctx)
	if err != nil || !found || after != before {
		t.Fatalf("host-local mode restart changed signed head: %v", err)
	}
}
