//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	evidencestore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	evidenceport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

type runtimeOriginalFinalLiveFailureV1 struct{ calls int }

func (live *runtimeOriginalFinalLiveFailureV1) ReplayAt(context.Context, domainsecurity.TurnSecurityContext, uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	live.calls++
	return domainevidence.EvidenceReceiptRegistry{}, errors.New("synthetic live historical witness unavailable")
}

type runtimeOriginalFinalPrimaryReaderV1 struct {
	core *runtimeChildIdentityStartupV1
}

func (reader runtimeOriginalFinalPrimaryReaderV1) AllThreadIDs() ([]string, error) {
	ids := []string{}
	for id := range reader.core.primaries.entries {
		ids = append(ids, id)
	}
	return ids, nil
}

func (reader runtimeOriginalFinalPrimaryReaderV1) GetThread(id string) (map[string]any, error) {
	snapshot, err := reader.core.primaries.ReadPrimaryThreadSnapshotV1(context.Background(), id)
	return snapshot.Thread, err
}

func newRuntimeOriginalNonzeroBoundaryFixtureV1(t *testing.T) (*runtimeOriginalReservedReportHistoryFixtureV1, domainevidence.PrivateAcceptedFinalRecord) {
	t.Helper()
	ctx := context.Background()
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{
		preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, preWitnessDisposition: domainpending.StatusFailed, omitFailedResult: true,
	})
	lease, err := AcquireRuntimePersistenceLease(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	preparedStartup, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(fixture.config, lease)
	if err != nil {
		lease.Close()
		t.Fatal(err)
	}
	handler, err := preparedStartup.Activate(fixture.config)
	if err != nil {
		lease.Close()
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.refresh(t)
	finals, err := finalauthority.NewPrivateStore(filepath.Join(fixture.core.roots.DataDir, "private", "accepted-finals"), fixture.core.access)
	if err != nil {
		t.Fatal(err)
	}
	var record domainevidence.PrivateAcceptedFinalRecord
	if err := finals.VisitAcceptedFinals(ctx, func(candidate domainevidence.PrivateAcceptedFinalRecord) error {
		if candidate.SecurityContext.ThreadID == fixture.attempt.ThreadID && candidate.RegistryHead.Sequence > 0 {
			record = candidate
		}
		return nil
	}); err != nil || record.RegistryHead.Sequence == 0 {
		t.Fatalf("actual Core restart did not produce a signed nonzero boundary: %v", err)
	}
	if domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Envelope.Variant) || record.AcceptedFinal.FactFinalWitnessAdmission != nil {
		t.Fatal("fixture unexpectedly produced a fact final")
	}
	return fixture, record
}

func TestRuntimeOriginalNonzeroBoundaryFinalOfflineHistory(t *testing.T) {
	ctx := context.Background()
	fixture, record := newRuntimeOriginalNonzeroBoundaryFixtureV1(t)
	if !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("fixture witness was not available to disable")
	}
	startupCalls := fixture.witnessAttempts()
	handler, err := NewRuntimeServerHandlerE(fixture.config)
	if err != nil {
		t.Fatalf("existing nonzero boundary could not restart with witness offline: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	if fixture.witnessAttempts() != startupCalls {
		t.Fatal("existing nonzero boundary restart contacted unavailable witness")
	}
	fixture.refresh(t)
	finals, err := finalauthority.NewPrivateStore(filepath.Join(fixture.core.roots.DataDir, "private", "accepted-finals"), fixture.core.access)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := finalauthority.NewAcceptedFinalCASReader(fixture.core.roots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	live := &runtimeOriginalFinalLiveFailureV1{}
	preserved := runtimeReportRestartPreservationV1{report: fixture.scope}
	preserved.finalHistory, err = prepareRuntimeOriginalFinalHistoryV1(ctx, fixture.core, preserved)
	if err != nil || preserved.finalHistory == nil {
		t.Fatal("original history was not frozen before the reader", err)
	}
	reader := runtimeFinalHistoricalReaderV1(live, fixture.core, preserved)
	unqualified := context.WithValue(ctx, runtimeFinalHistoryQualificationKeyV1{}, &runtimeFinalHistoryQualificationV1{})
	if _, inherited, err := bindRuntimeOriginalFinalHistoryV1(unqualified, fixture.core, preserved); err != nil || inherited != nil {
		t.Fatal("same-startup recursion acquired previously absent Original qualification", err)
	}
	before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
	if !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("fixture witness was not available to disable")
	}
	attempts := fixture.witnessAttempts()
	for pass := 0; pass < 2; pass++ {
		inventory, err := evidenceapp.PreflightFinalAuthorityInventory(ctx, runtimeOriginalFinalPrimaryReaderV1{fixture.core}, cas, reader, fixture.publication.installation, finals)
		if err != nil || len(inventory.Committed) != 1 || inventory.Committed[0].AcceptedFinal.RecordDigest != record.AcceptedFinal.RecordDigest {
			t.Fatalf("native nonzero boundary history depended on live witness or changed classification: %v", err)
		}
	}
	if live.calls != 0 || fixture.witnessAttempts() != attempts || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
		t.Fatal("offline historical preflight wrote native records or contacted live witness")
	}

	// The native readers establish the full Original denominator before these
	// focused selection controls remove support from detached copies.
	files, _, contexts, registryObservation, err := readRuntimeRegistrySemanticInventoryV1(ctx, fixture.core, fixture.scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := parseRuntimeOriginalRegistryInventoryV1(ctx, fixture.core, files, contexts)
	if err != nil || registry.v2 == nil {
		t.Fatal("native registry graph unavailable", err)
	}
	originals, _, _, associatedObservation, err := readRuntimeAssociatedSemanticInventoryV1(ctx, fixture.core, fixture.scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	trust := fixture.core.originalRegistryTrust
	graph, err := evidencestore.ParseOriginalGraphV1(ctx, originals["evidence-authority"], trust.projection.InstallationID, trust.projection.Enrollment.EnrollmentID, trust.projection.Enrollment.WitnessKeyID, trust.witnessKey, fixture.core.verification)
	if err != nil || errors.Join(registryObservation.Revalidate(ctx), associatedObservation.Revalidate(ctx)) != nil {
		t.Fatal("native evidence graph unavailable", err)
	}
	for _, name := range []string{"witnessed-prefix", "duplicate-prefix", "unwitnessed-local-capsules", "missing-ancestor", "missing-capsule", "wrong-full-head", "wrong-full-context", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(registry.v2)
			if err != nil {
				t.Fatal(err)
			}
			var copied = *registry.v2
			if err := json.Unmarshal(body, &copied); err != nil {
				t.Fatal(err)
			}
			evidenceBody, _ := json.Marshal(graph)
			var evidenceGraph evidencestore.OriginalGraphV1
			if err := json.Unmarshal(evidenceBody, &evidenceGraph); err != nil {
				t.Fatal(err)
			}
			candidate, callCtx := record, ctx
			switch name {
			case "duplicate-prefix":
				// Selection consumes already-authenticated exchanges; repeat the
				// same exact exchange to exercise prefix deduplication, not native
				// canonical observation addressing.
				for _, stored := range evidenceGraph.Observations {
					evidenceGraph.Observations["detached-duplicate"] = stored
					break
				}
			case "unwitnessed-local-capsules":
				evidenceGraph.Observations = map[string]evidenceport.ObservationBundle{}
			case "missing-ancestor":
				for digest := range copied.Indexes {
					delete(copied.Indexes, digest)
				}
			case "missing-capsule":
				for digest := range copied.Capsules {
					delete(copied.Capsules, digest)
				}
			case "wrong-full-head":
				candidate.RegistryHead.StateDigest = domainsecurity.SHA256Hex([]byte("different exact historical head"))
			case "wrong-full-context":
				candidate.SecurityContext.UserID += "-different"
			case "cancelled":
				var cancel context.CancelFunc
				callCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			snapshot, err := replayRuntimeOriginalBoundaryPrefixV1(callCtx, candidate, &copied, evidenceGraph)
			if name == "witnessed-prefix" || name == "duplicate-prefix" {
				head, headErr := domainevidence.NewEvidenceRegistryHead(snapshot)
				if err != nil || headErr != nil || head != record.RegistryHead {
					t.Fatalf("exact witnessed prefix unavailable: %v", err)
				}
			} else if err == nil || !reflect.DeepEqual(snapshot, domainevidence.EvidenceReceiptRegistry{}) {
				t.Fatal("unsupported historical prefix acquired authority")
			}
		})
	}
	if live.calls != 0 || fixture.witnessAttempts() != attempts || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
		t.Fatal("historical negative controls had effects")
	}
	testRuntimeOriginalFinalSemanticControlsV1(t, fixture, record, preserved)
}

type cancelAfterOriginalFinalMarkerV1 struct {
	context.Context
	cancel context.CancelFunc
	marker string
}

func TestRuntimeOriginalNonzeroFinalRejectsFinalOnlyWitnessExchange(t *testing.T) {
	ctx := context.Background()
	fixture, _ := newRuntimeOriginalNonzeroBoundaryFixtureV1(t)
	originals, _, _, observation, err := readRuntimeAssociatedSemanticInventoryV1(ctx, fixture.core, fixture.scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	exchanges := runtimeOriginalSemanticFilesV1{}
	for name, entry := range runtimeReportCommittedFilesV1(originals["evidence-authority"]) {
		if strings.HasPrefix(name, "observations/") {
			exchanges[name] = entry
		}
	}
	if len(exchanges) == 0 || observation.Revalidate(ctx) != nil {
		t.Fatal("native stored exchanges were not established")
	}
	// This isolated negative retains the genuine signed final, bundle and
	// registry graph but lacks its original stored exchange. The signed Final
	// program may not repair that missing historical proof.
	for name := range exchanges {
		if err := os.Remove(filepath.Join(fixture.core.roots.DataDir, "private", "evidence-authority", name)); err != nil {
			t.Fatal(err)
		}
	}
	fixture.refresh(t)
	root, err := persistencefs.FreezeRootAuthority(fixture.core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(fixture.core.roots)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(fixture.core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("Final-only stored witness exchange"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join("private", "a-final-only-exchange-marker.bin")
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(fixture.core.roots, root, journal, runtimeReportRestartPreservationV1{})
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		for name, entry := range exchanges {
			if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "evidence-authority", name), entry.Body, os.FileMode(entry.Mode)); err != nil {
				return err
			}
		}
		return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("synthetic Final-only exchange prefix"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterOriginalFinalMarkerV1{Context: cancelled, cancel: cancel, marker: filepath.Join(fixture.core.roots.DataDir, marker)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("Final-only exchange signed cut not reached: %v", err)
	}
	for name := range exchanges {
		if _, err := os.Stat(filepath.Join(fixture.core.roots.DataDir, "private", "evidence-authority", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("exchange already entered physical Original")
		}
	}
	if !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("native witness unavailable to disable")
	}
	before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
	calls := fixture.witnessAttempts()
	handler, err := NewRuntimeServerHandlerE(fixture.config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("Final-only witness material acquired Original qualification")
	}
	if !strings.Contains(err.Error(), "no witnessed historical prefix") {
		t.Fatalf("Final-only negative did not reach Original selector: %v", err)
	}
	if calls != fixture.witnessAttempts() || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
		t.Fatal("Final-only refusal wrote or contacted live witness")
	}
}

func (ctx cancelAfterOriginalFinalMarkerV1) Err() error {
	if _, err := os.Stat(ctx.marker); err == nil {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func testRuntimeOriginalFinalSemanticControlsV1(t *testing.T, fixture *runtimeOriginalReservedReportHistoryFixtureV1, record domainevidence.PrivateAcceptedFinalRecord, preserved runtimeReportRestartPreservationV1) {
	t.Helper()
	ctx := context.Background()
	root, err := persistencefs.FreezeRootAuthority(fixture.core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(fixture.core.roots)
	if err != nil {
		t.Fatal(err)
	}
	var indexName string
	for name := range preserved.finalHistory.registryRecords {
		if strings.HasPrefix(name, "indexes/") {
			indexName = name
			break
		}
	}
	if indexName == "" {
		t.Fatal("native history has no immutable index")
	}
	prepare := func(name string, guard runtimeReportRestartPreservationV1, mutate func(startupport.PersistenceRootsV1) error) (startupport.PreparedSemanticPlanV1, error) {
		snapshot, err := persistencefs.NewStartupSnapshotReader(fixture.core.roots).CaptureManagedSnapshotV1(ctx)
		if err != nil {
			return nil, err
		}
		digest := domainsecurity.SHA256Hex([]byte("nonzero Original final " + name))
		baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
		if err != nil {
			return nil, err
		}
		builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(fixture.core.roots, root, journal, guard)
		return builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error { return mutate(stage) })
	}
	for _, name := range []string{"harmless-title", "duplicate-primary-family", "incomplete-new-primary", "erase-prepared", "erase-settlement-marker", "erase-index", "erase-observations", "foreign-final-addition"} {
		t.Run(name, func(t *testing.T) {
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			calls := fixture.witnessAttempts()
			constructed := false
			prepared, err := prepare(name, preserved, func(stage startupport.PersistenceRootsV1) error {
				switch name {
				case "harmless-title":
					file := filepath.Join(stage.DurableDir, "threads", fixture.attempt.ThreadID, "thread.json")
					body, err := os.ReadFile(file)
					if err != nil {
						return err
					}
					var primary map[string]any
					if err := json.Unmarshal(body, &primary); err != nil {
						return err
					}
					primary["title"] = "ordinary nonzero history metadata"
					body, err = json.Marshal(primary)
					if err != nil {
						return err
					}
					err = os.WriteFile(file, body, 0600)
					constructed = err == nil
					return err
				case "erase-index":
					return os.Remove(filepath.Join(stage.DataDir, "private", "evidence-registry", indexName))
				case "duplicate-primary-family":
					return os.MkdirAll(filepath.Join(stage.DurableDir, "runtime-go", "threads", fixture.attempt.ThreadID), 0700)
				case "incomplete-new-primary":
					return os.MkdirAll(filepath.Join(stage.DurableDir, "threads", "incomplete-history-thread"), 0700)
				case "erase-prepared":
					for name := range preserved.finalHistory.settlementHistory.prepared {
						if strings.HasPrefix(name, "prepared/") {
							return os.Remove(filepath.Join(stage.DataDir, "private", "evidence-settlements", name))
						}
					}
					return errors.New("fixture has no Original prepared record")
				case "erase-settlement-marker":
					file := filepath.Join(stage.DurableDir, "threads", fixture.attempt.ThreadID, "thread.json")
					body, err := os.ReadFile(file)
					if err != nil {
						return err
					}
					var primary map[string]any
					if err := json.Unmarshal(body, &primary); err != nil {
						return err
					}
					for _, turnValue := range primary["turns"].([]any) {
						for _, itemValue := range turnValue.(map[string]any)["items"].([]any) {
							item := itemValue.(map[string]any)
							if _, found := item["hostEvidenceSettlement"]; found {
								delete(item, "hostEvidenceSettlement")
								constructed = true
							}
						}
					}
					if !constructed {
						return errors.New("fixture has no Original settlement marker")
					}
					body, err = json.Marshal(primary)
					if err != nil {
						return err
					}
					return os.WriteFile(file, body, 0600)
				case "erase-observations":
					for name := range preserved.finalHistory.evidenceRecords {
						if strings.HasPrefix(name, "observations/") {
							if err := os.Remove(filepath.Join(stage.DataDir, "private", "evidence-authority", name)); err != nil {
								return err
							}
						}
					}
					return nil
				case "foreign-final-addition":
					key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{81}, ed25519.SeedSize))
					public := key.Public().(ed25519.PublicKey)
					acceptedAt, err := time.Parse(time.RFC3339Nano, record.AcceptedFinal.AcceptedAt)
					if err != nil {
						return err
					}
					accepted, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
						Context: record.SecurityContext, Envelope: record.Envelope, RenderedText: record.RenderedText,
						RegistryHead: record.RegistryHead, PrivateRecordDigest: record.PrivateRecordDigest, AcceptedAt: acceptedAt,
						AuthorityKeyID: domainsecurity.SHA256Hex(public), AuthorityPublicKey: public,
					}, func(body []byte) ([]byte, error) { return ed25519.Sign(key, body), nil })
					if err != nil {
						return err
					}
					foreign, err := domainevidence.NewPrivateAcceptedFinalRecord(record.SecurityContext, record.Envelope, record.RenderedText, record.RegistryHead, record.PublicationIntent, accepted)
					if err != nil {
						return err
					}
					body, err := domainevidence.PrivateAcceptedFinalRecordBytes(foreign)
					if err != nil {
						return err
					}
					digest := accepted.RecordDigest
					file := filepath.Join(stage.DataDir, "private", "accepted-finals", "records", digest[:2], digest+".json")
					if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
						return err
					}
					err = os.WriteFile(file, body, 0600)
					constructed = err == nil
					return err
				}
				return errors.New("unknown nonzero final semantic fixture")
			})
			if prepared != nil {
				defer prepared.Close()
			}
			if name == "foreign-final-addition" && !constructed {
				t.Fatalf("foreign final candidate was not constructed: %v", err)
			}
			if err != nil {
				t.Fatalf("semantic fixture did not prepare: %v", err)
			}
			err = prepared.Apply(ctx)
			if name == "harmless-title" {
				if err != nil {
					t.Fatalf("ordinary metadata was rejected: %v", err)
				}
				fixture.refresh(t)
				qualification := context.WithValue(ctx, runtimeFinalHistoryQualificationKeyV1{}, &runtimeFinalHistoryQualificationV1{history: preserved.finalHistory})
				rebound := runtimeReportRestartPreservationV1{report: fixture.scope}
				_, rebound.finalHistory, err = bindRuntimeOriginalFinalHistoryV1(qualification, fixture.core, rebound)
				if err != nil {
					t.Fatalf("ordinary metadata changed original final history: %v", err)
				}
				preserved = rebound
			} else if err == nil {
				t.Fatal("unsafe semantic candidate passed the actual Apply guard")
			}
			if name != "harmless-title" && !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || calls != fixture.witnessAttempts() {
				t.Fatal("candidate validation wrote Original or contacted witness")
			}
		})
	}
	// Leave one real signed program before its destructive operation, then
	// exercise both configured root and configuration-free maintenance entry.
	markerRelative := filepath.Join("private", "a-nonzero-history-marker.bin")
	var preparedName string
	for name := range preserved.finalHistory.settlementHistory.prepared {
		if strings.HasPrefix(name, "prepared/") {
			preparedName = name
			break
		}
	}
	if preparedName == "" {
		t.Fatal("native history has no prepared record")
	}
	prepared, err := prepare("signed-prepared-removal", runtimeReportRestartPreservationV1{}, func(stage startupport.PersistenceRootsV1) error {
		if err := os.Remove(filepath.Join(stage.DataDir, "private", "evidence-settlements", preparedName)); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, markerRelative), []byte("synthetic signed prefix"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterOriginalFinalMarkerV1{Context: cancelled, cancel: cancel, marker: filepath.Join(fixture.core.roots.DataDir, markerRelative)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("real signed prefix not established: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.core.roots.DataDir, "private", "evidence-settlements", preparedName)); err != nil {
		t.Fatal("signed removal passed its intended interruption", err)
	}
	before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
	calls := fixture.witnessAttempts()
	lease, err := AcquireRuntimePersistenceLease(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	err = RecoverAuthenticatedExistingSemanticStartupWithPersistenceLeaseContextE(ctx, lease)
	closeErr := lease.Close()
	if err == nil || !strings.Contains(err.Error(), "requires configured Original final authority") || closeErr != nil {
		t.Fatalf("unqualified maintenance did not refuse before writing: %v / %v", err, closeErr)
	}
	handler, err := NewRuntimeServerHandlerE(fixture.config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("signed Final erased original nonzero support")
	}
	if !strings.Contains(err.Error(), "durable marker is orphaned") {
		t.Fatalf("configured root refused at another boundary: %v", err)
	}
	if calls != fixture.witnessAttempts() || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
		t.Fatal("signed program refusal wrote state or contacted witness")
	}
}
