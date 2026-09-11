//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	piistore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	publicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	piiapp "analytix.local/runtime-go/internal/app/piiauthorization"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestRuntimeOriginalControlledAccessRejectsSignedFinalLostClosedInventory(t *testing.T) {
	ctx := context.Background()
	fixture := newRuntimeOriginalReportHistoryVariantFixtureV1(t, false, true)
	core := fixture.core
	history, err := prepareRuntimeOriginalReportHistoryV1(ctx, core, fixture.publication, fixture.advance, fixture.scope)
	if err != nil {
		t.Fatal(err)
	}
	closed := history.controlled.owners["controlled-artifact-access-v2"]
	if len(closed) != 2 {
		t.Fatal("expected actual closed receipt and disposition")
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("signed Final removes complete closed V2 history"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	root, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	marker, unapplied := filepath.Join("private", "a-closed-access-final-marker.bin"), filepath.Join("private", "z-closed-access-final-marker.bin")
	// An unguarded isolated fixture creates a valid signed interrupted program.
	// Production recovery must reject its Final even though both open plans are empty.
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, runtimeReportRestartPreservationV1{})
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		for name := range closed {
			if err := os.Remove(filepath.Join(stage.DataDir, "private", "controlled-artifact-access-v2", filepath.FromSlash(name))); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("signed prefix"), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, unapplied), []byte("unapplied Final"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: filepath.Join(core.roots.DataDir, marker), intentPath: filepath.Join(core.roots.DataDir, unapplied)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("signed Final fixture did not interrupt: %v", err)
	}
	for name, entry := range closed {
		body, err := os.ReadFile(filepath.Join(core.roots.DataDir, "private", "controlled-artifact-access-v2", filepath.FromSlash(name)))
		if err != nil || !reflect.DeepEqual(body, entry.Body) {
			t.Fatal("signed Final fixture lost physical Original before audit")
		}
	}
	fixture.refresh(t)
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	history, err = prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
	if err == nil || history != nil {
		t.Fatal("empty Final open plan erased complete closed Original history")
	}
	if !strings.Contains(err.Error(), "completed controlled access history changed in signed Final") {
		t.Fatalf("negative did not reach exact closed inventory comparison: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("signed Final rejection changed managed state")
	}
}

func TestRuntimeOriginalClosedControlledAccessAllowsRuntimeActivation(t *testing.T) {
	fixture := newRuntimeOriginalReportHistoryVariantFixtureV1(t, false, true)
	roots := []string{}
	for _, owner := range runtimePublicationOwnersV1 {
		roots = append(roots, filepath.Join(fixture.core.roots.DataDir, "private", owner))
	}
	before := startupWholeTreeRecordMapForTest(t, roots...)
	for restart := 0; restart < 2; restart++ {
		handler, err := NewRuntimeServerHandlerE(fixture.config)
		if err != nil {
			t.Fatalf("closed verified V2 history blocked runtime activation %d: %v", restart, err)
		}
		shutdownOwnedRuntimeHandler(t, handler)
		if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots...)) {
			t.Fatal("runtime changed original controlled publication/access records")
		}
	}
}

func TestRuntimeOriginalControlledAccessHistoryUsesActualReportAuthority(t *testing.T) {
	ctx := context.Background()
	fixture := newRuntimeOriginalReportHistoryVariantFixtureV1(t, false, true)
	core, publication := fixture.core, fixture.publication
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	history, err := prepareRuntimeOriginalReportHistoryV1(ctx, core, publication, fixture.advance, fixture.scope)
	if err != nil {
		t.Fatal(err)
	}
	if !history.matchesCompletedPlanV1(history.plan) {
		t.Fatal("actual controlled terminal is not complete")
	}
	original, _, observation, err := publication.readV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	check := func(id, key string) error {
		if id != core.verification.KeyID() || key != base64.RawURLEncoding.EncodeToString(core.verification.PublicKey()) {
			return errors.New("foreign current key")
		}
		return nil
	}
	reports, err := publicationstore.ParseOriginalReadersV1(ctx, original["report-publication"], publication.installation, check, publication.creationProofV1("report-publication"))
	if err != nil {
		t.Fatal(err)
	}
	grants, err := piistore.ParseOriginalGrantReaderV1(ctx, original["pii-authorization"], publication.installation, check, publication.creationProofV1("pii-authorization"))
	if err != nil {
		t.Fatal(err)
	}
	access, err := piistore.ParseOriginalAccessReaderV2(ctx, original["controlled-artifact-access-v2"], publication.installation, check, publication.creationProofV1("controlled-artifact-access-v2"))
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := publicationapp.NewHistoricalArtifactDeliveryBridgeV1(history.authority, reports.Commits, reports.Receipts, core.verification)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := piiapp.VerifyControlledAccessInventoryV2(ctx, piiapp.ControlledAccessInventoryDependenciesV2{Access: access, Historical: bridge, Grants: grants, Artifacts: reports.Artifacts, Authority: core.verification})
	if err != nil {
		t.Fatal(err)
	}
	if plan.OpenReceiptCount() != 0 {
		t.Fatal("closed history became a restart writer")
	}
	receipts, dispositions := 0, 0
	if err := access.VisitAccessReceiptsV2(ctx, func(domainpii.ControlledArtifactAccessReceiptV2) error { receipts++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := access.VisitAccessDispositionsV2(ctx, func(domainpii.ControlledArtifactAccessDispositionV2) error { dispositions++; return nil }); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || dispositions != 1 {
		t.Fatal("closed history lost the complete access denominator")
	}
	if err := errors.Join(history.revalidate(ctx), observation.Revalidate(ctx)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("historical audit changed native state")
	}
}

func TestRuntimeOriginalControlledAccessRejectsIncompleteCutsBeforeWrites(t *testing.T) {
	for _, fault := range []string{"open-access-with-missing-grant", "missing-grant", "orphan-disposition", "missing-witness"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{controlled: true, openAccess: fault == "open-access-with-missing-grant"})
			history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
			if err != nil {
				t.Fatal(err)
			}
			owner, prefix := "controlled-artifact-access-v2", "access-dispositions/"
			switch fault {
			case "missing-grant", "open-access-with-missing-grant":
				owner, prefix = "pii-authorization", "grants/"
			case "orphan-disposition":
				prefix = "access-receipts/"
			}
			removed := 0
			if fault == "missing-witness" {
				digest := history.plan.Attempts[0].Selection.ObservationDigest
				if err := os.Remove(filepath.Join(fixture.core.roots.DataDir, "private", "evidence-authority", "observations", digest[:2], digest+".json")); err != nil {
					t.Fatal(err)
				}
				removed++
			} else {
				for name := range history.controlled.owners[owner] {
					if strings.HasPrefix(name, prefix) {
						if err := os.Remove(filepath.Join(fixture.core.roots.DataDir, "private", owner, filepath.FromSlash(name))); err != nil {
							t.Fatal(err)
						}
						removed++
					}
				}
			}
			if removed != 1 {
				t.Fatal("fault did not remove its exact native record")
			}
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			handler, err := NewRuntimeServerHandlerE(fixture.config)
			if handler != nil {
				shutdownOwnedRuntimeHandler(t, handler)
			}
			if err == nil {
				t.Fatal("incomplete controlled history passed runtime gates")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
				t.Fatal("incomplete controlled history changed managed state")
			}
		})
	}
}
