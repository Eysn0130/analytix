//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestFreshImportRegistryActivationIsExplicitSharedAndFailClosed(t *testing.T) {
	for _, fault := range []string{"none", "nonzero missing", "changed owner", "partial", "unknown"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			fixture, config := runtimeWitnessedRegistryConfigV2(t)
			root := filepath.Join(config.DataDir, "private", "evidence-registry")
			if fault == "changed owner" {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			composition := runtimeWitnessedRegistryDirectCompositionV2(t, fixture, config)
			owner := composition.registryOwner
			if owner == nil || composition.registry != nil || !owner.CaseEvidenceAuthorityUnavailableV1() || fixture.TotalAttempts() != 0 {
				t.Fatal("cold observation activated registry or reached witness")
			}
			defer owner.Close()
			if _, err := owner.HasRecords(ctx); err == nil {
				t.Fatal("cold read activated registry")
			}
			if fixture.TotalAttempts() != 0 {
				t.Fatal("cold read reached witness")
			}
			if _, err := composition.evidence.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			workspace := t.TempDir()
			runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
			observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
			if err != nil {
				t.Fatal(err)
			}
			manifest, producer, materials := runtimeSharedEvidenceBoundMaterialsV2(t, workspace, observation, fixture.InstallationID, fixture.Authority)
			access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			materialCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", "dataset-snapshot-authority", "materials"), 16*1024*1024, access)
			if err != nil {
				t.Fatal(err)
			}
			defer materialCAS.Close()
			for _, records := range materials {
				for address, body := range records {
					if err := materialCAS.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
						t.Fatal(err)
					}
				}
			}
			selected, err := composition.snapshot.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{
				TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation,
				ManifestReference: manifest, FundsProducerReference: producer, AcceptedAt: time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "nonzero missing":
				head, err := composition.evidence.ObserveFresh(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := composition.evidence.AdvanceEvidenceRegistry(ctx, evidenceauthorityport.RegistryAdvanceInput{ExpectedBundleDigest: head.Bundle.RecordDigest, NextIndexDigest: strings.Repeat("c", 64)}); err != nil {
					t.Fatal(err)
				}
			case "changed owner":
				if err := os.Rename(root, root+"-retained"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			case "partial":
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(root, "indexes"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "unknown":
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("preserved"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			inventoryDigest := func() string {
				if _, err := os.Lstat(root); os.IsNotExist(err) {
					return "absent"
				}
				return startupWholeTreeDigest(t, root)
			}
			before := inventoryDigest()
			if fault != "none" {
				if err := owner.ActivateAfterImport(ctx, observation, selected.Record.DatasetSnapshotID); err == nil {
					t.Fatal("blocked inventory/head activated registry")
				}
				if !owner.CaseEvidenceAuthorityUnavailableV1() || before != inventoryDigest() {
					t.Fatal("failed activation changed registry inventory or authority")
				}
				return
			}
			var wg sync.WaitGroup
			failures := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					failures <- owner.ActivateAfterImport(ctx, observation, selected.Record.DatasetSnapshotID)
				}()
			}
			wg.Wait()
			close(failures)
			for err := range failures {
				if err != nil {
					t.Fatal(err)
				}
			}
			if owner.CaseEvidenceAuthorityUnavailableV1() {
				t.Fatal("confirmed activation not visible through shared owner")
			}
			active := owner.registry
			if err := owner.ActivateAfterImport(ctx, observation, selected.Record.DatasetSnapshotID); err != nil || owner.registry != active {
				t.Fatal("repeat activation replaced shared owner")
			}
			if found, err := owner.HasRecords(ctx); err != nil || found {
				t.Fatal("activation fabricated a receipt")
			}
			_, release, err := owner.current()
			if err != nil {
				t.Fatal(err)
			}
			closed := make(chan error, 1)
			go func() { closed <- owner.Close() }()
			select {
			case <-closed:
				t.Fatal("close did not drain active owner call")
			case <-time.After(25 * time.Millisecond):
			}
			release()
			if err := <-closed; err != nil {
				t.Fatal(err)
			}
			if err := owner.ActivateAfterImport(ctx, observation, selected.Record.DatasetSnapshotID); err == nil {
				t.Fatal("closed owner reactivated")
			}
		})
	}
}
