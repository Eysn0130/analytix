//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
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

func TestRuntimeOriginalOpenControlledAccessRecoversIndeterminate(t *testing.T) {
	testRuntimeOriginalOpenControlledAccessRecoveryV2(t, false, false)
}

func TestRuntimeMixedControlledAccessPreservesClosedAndRecoversOpen(t *testing.T) {
	testRuntimeOriginalOpenControlledAccessRecoveryV2(t, true, false)
}

func TestRuntimeOpenControlledAccessResumesAppliedSignedPrefix(t *testing.T) {
	testRuntimeOriginalOpenControlledAccessRecoveryV2(t, false, true)
}

// Use the actual stage-only V2 consumer and guarded signed semantic Apply,
// placing the cancellation probe inside the owner's derived observation context.
// Full runtime startup must then recover this authentic applied prefix.
func interruptRuntimeControlledAccessSemanticPlanForTestV2(t *testing.T, fixture *runtimeOriginalReservedReportHistoryFixtureV1, dispositionPath string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	core, publication := fixture.core, fixture.publication
	history, err := prepareRuntimeOriginalReportHistoryV1(ctx, core, publication, fixture.advance, fixture.scope)
	if err != nil {
		t.Fatal(err)
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	check := func(id, public string) error {
		if id != core.verification.KeyID() || public != base64.RawURLEncoding.EncodeToString(core.verification.PublicKey()) {
			return errors.New("foreign current key")
		}
		return nil
	}
	reports, err := publicationstore.ParseOriginalReadersV1(ctx, publication.original["report-publication"], publication.installation, check, publication.creationProofV1("report-publication"))
	if err != nil {
		t.Fatal(err)
	}
	grants, err := piistore.ParseOriginalGrantReaderV1(ctx, publication.original["pii-authorization"], publication.installation, check, publication.creationProofV1("pii-authorization"))
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := publicationapp.NewHistoricalArtifactDeliveryBridgeV1(history.authority, reports.Commits, reports.Receipts, core.verification)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("native V2 indeterminate interrupted semantic fixture"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	rootAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journalAuthority, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	preserved := runtimeReportRestartPreservationV1{report: fixture.scope, core: core, publication: publication, history: history}
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, rootAuthority, journalAuthority, preserved)
	prepared, err := builder.Prepare(ctx, baseline, digest, func(ctx context.Context, stage startupport.PersistenceRootsV1) (resultErr error) {
		roots, err := persistencefs.ResolveRootSet(stage.DataDir, stage.DurableDir)
		if err != nil {
			return err
		}
		authority, err := persistencefs.FreezeRootAuthority(roots)
		if err != nil {
			return err
		}
		access, err := persistencefs.NewSemanticStagePrivateCASAccessAuthority(roots, authority)
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, access.Close()) }()
		store, err := piistore.NewAccessStoreV2(filepath.Join(roots.DataDir, "private", "controlled-artifact-access-v2"), access)
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, store.Close()) }()
		plan, err := piiapp.VerifyControlledAccessInventoryV2(ctx, piiapp.ControlledAccessInventoryDependenciesV2{Access: store, Historical: bridge, Grants: grants, Artifacts: reports.Artifacts, Authority: core.verification})
		if err != nil {
			return err
		}
		if plan.OpenReceiptCount() != 2 {
			return errors.New("actual stage did not retain both Original open receipts")
		}
		return piiapp.ApplyControlledAccessRestartPlanV2(ctx, plan, store, key, time.Now().UTC())
	})
	if err != nil {
		t.Fatal(err)
	}
	applyErr := withRetiredRuntimePrivateCASOwnerGenerationsForSemanticApply(ctx, core.roots.DataDir, core.access, publication.installation, prepared.Plan(), func(observationContext context.Context) error {
		cut := cancelAfterTerminalDispositionContextV1{Context: observationContext, cancel: cancel, dispositionPath: dispositionPath, intentPath: filepath.Join(core.roots.DataDir, "never-issued-interruption-marker")}
		return prepared.Apply(cut)
	}, preserved)
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(applyErr, context.Canceled) {
		t.Fatalf("guarded semantic Apply did not interrupt at applied V2 prefix: %v", applyErr)
	}
}

func testRuntimeOriginalOpenControlledAccessRecoveryV2(t *testing.T, mixed, interrupted bool) {
	t.Helper()
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{controlled: true, openAccess: !mixed, additionalOpenAccess: mixed || interrupted})
	var open []domainpii.ControlledArtifactAccessReceiptV2
	original := fixture.publication.original["controlled-artifact-access-v2"]
	for name, entry := range fixture.publication.original["controlled-artifact-access-v2"] {
		if entry.Directory || !strings.HasPrefix(name, "access-receipts/") {
			continue
		}
		receipt, err := domainpii.ParseControlledArtifactAccessReceiptV2(entry.Body)
		if err != nil {
			t.Fatal(err)
		}
		if _, closed := original["access-dispositions/"+receipt.AccessID[:2]+"/"+receipt.AccessID+".json"]; !closed {
			open = append(open, receipt)
		}
	}
	if len(open) == 0 {
		t.Fatal("fixture has no actual Original open receipt")
	}
	sort.Slice(open, func(i, j int) bool { return open[i].AccessID < open[j].AccessID })
	receipt := open[0]
	root := filepath.Join(fixture.core.roots.DataDir, "private", "controlled-artifact-access-v2")
	dispositionPath := filepath.Join(root, "access-dispositions", receipt.AccessID[:2], receipt.AccessID+".json")
	if _, err := os.Stat(dispositionPath); !os.IsNotExist(err) {
		t.Fatal("fixture already had a terminal disposition")
	}
	protected := []string{filepath.Join(root, "access-receipts")}
	for _, owner := range []string{"report-publication", "pii-authorization", "controlled-artifact-access"} {
		protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", owner))
	}
	before := startupWholeTreeRecordMapForTest(t, protected...)
	var appliedPrefix []byte
	var appliedMode os.FileMode
	if interrupted {
		if len(open) != 2 {
			t.Fatal("interruption needs two actual Original open receipts")
		}
		interruptRuntimeControlledAccessSemanticPlanForTestV2(t, fixture, dispositionPath)
		var err error
		appliedPrefix, err = os.ReadFile(dispositionPath)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(dispositionPath)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("applied disposition is not a regular file: %v", err)
		}
		appliedMode = info.Mode()
		other := open[1]
		if _, err := os.Stat(filepath.Join(root, "access-dispositions", other.AccessID[:2], other.AccessID+".json")); !os.IsNotExist(err) {
			t.Fatal("second open receipt was already disposed before cut")
		}
		journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(context.Background(), fixture.core.roots, nil)
		if err != nil || journal == nil {
			t.Fatalf("applied prefix lost its actual signed journal: %v", err)
		}
		if err := journal.Revalidate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := NewRuntimeServerHandlerE(fixture.config)
	if err != nil {
		t.Fatalf("verified Original open access failed semantic indeterminate recovery: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	for _, receipt := range open {
		path := filepath.Join(root, "access-dispositions", receipt.AccessID[:2], receipt.AccessID+".json")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode() != 0o600 || info.Size() != int64(len(body)) {
			t.Fatalf("recovered disposition lost its native file mode or size: %v", err)
		}
		disposition, err := domainpii.ParseControlledArtifactAccessDispositionV2(body)
		if err != nil || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
			t.Fatalf("recovered disposition lost exact receipt authority: %v", err)
		}
		if disposition.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 || disposition.ReasonCode != domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1 || disposition.ReleasedByteLength != receipt.ArtifactByteLength {
			t.Fatal("restart claimed a release result instead of full-length uncertainty")
		}
		if interrupted && receipt.AccessID == open[0].AccessID && (!reflect.DeepEqual(body, appliedPrefix) || info.Mode() != appliedMode) {
			t.Fatal("resume replaced an already applied terminal")
		}
	}
	for name, entry := range original {
		if entry.Directory || !strings.HasPrefix(name, "access-dispositions/") {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		body, err := os.ReadFile(path)
		if err != nil || !reflect.DeepEqual(body, entry.Body) {
			t.Fatal("recovery replaced an Original closed disposition")
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode() != os.FileMode(entry.Mode) || info.Size() != int64(len(entry.Body)) {
			t.Fatalf("recovery changed an Original closed disposition mode or size: %v", err)
		}
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, protected...)) {
		t.Fatal("access recovery changed immutable Original materials")
	}
	closed := startupWholeTreeRecordMapForTest(t, append(protected, root)...)
	handler, err = NewRuntimeServerHandlerE(fixture.config)
	if err != nil {
		t.Fatalf("recovered access failed fresh restart: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	if !reflect.DeepEqual(closed, startupWholeTreeRecordMapForTest(t, append(protected, root)...)) {
		t.Fatal("fresh restart changed the exact recovered terminal")
	}
}
