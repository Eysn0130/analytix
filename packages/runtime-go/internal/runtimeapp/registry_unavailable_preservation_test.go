//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func TestRuntimeUnavailableOriginalRegistryPreservesWholeOwner(t *testing.T) {
	ctx := context.Background()
	_, config := runtimeWitnessedRegistryConfigV2(t)
	roots, err := persistencefs.ResolveRootSet(config.DataDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core, _ := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, true, false)
	core.originalRegistryTrust, err = prepareRuntimeOriginalRegistryTrustV2(ctx, config, core.verification)
	if err != nil || core.originalRegistryTrust == nil {
		t.Fatalf("independent original installation is unavailable: %v", err)
	}
	root := filepath.Join(core.roots.DataDir, "private", "evidence-registry")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"domainCanary":"` + runtimeOptionalDomainPrivateCanary + `"}`)
	name := ".registry-authority-index.json"
	if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatalf("physically proven unavailable original registry blocked UNKNOWN preservation: %v", err)
	}
	if preserved.registry == nil || !preserved.registry.unavailable || len(preserved.registry.original) == 0 {
		t.Fatal("unavailable original registry was reported as an empty healthy inventory")
	}
	original, err := preserved.ObserveOriginalEvidenceSettlementInventoryV1(ctx)
	if err != nil || original.RegistryUnavailable == nil || original.RegistryHistoryV2 != nil || original.HistoricalRegistryV1 || len(original.Registries) != 0 {
		t.Fatalf("settlement observer lost typed unavailable raw inventory: %v", err)
	}
	if err := preserved.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		t.Fatalf("unchanged unavailable original owner blocked independent semantic work: %v", err)
	}
	digest := domainsecurity.SHA256Hex([]byte("unavailable-registry-preservation"))
	plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, []domainstartup.SemanticStartupOperationV1{{
		Kind: domainstartup.SemanticOperationRemoveFile, Path: runtimeRegistrySemanticRootV1 + "/" + name,
		Before: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)},
		After:  domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, nil, ""); err == nil {
		t.Fatal("unavailable original owner allowed semantic deletion")
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("unavailable original observation changed original state")
	}
	// Domain failure may not hide missing independent authority or control
	// errors, even though the raw body and the local key remain unchanged.
	trust := core.originalRegistryTrust
	core.originalRegistryTrust = nil
	if err := preserved.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err == nil {
		t.Fatal("unavailable registry accepted missing independent enrollment")
	}
	core.originalRegistryTrust = trust
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := preserved.ValidateSemanticOperationsV1(cancelled, nil, nil, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("registry swallowed cancellation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".registry-authority-index-late.tmp"), []byte("opaque late residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := preserved.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err == nil {
		t.Fatal("unavailable original owner accepted newly appeared inventory")
	}
}
