//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestRuntimeCompletedReportSemanticCandidatePreservesHistory(t *testing.T) {
	testRuntimeCompletedReportSemanticCandidateV1(t, "", []string{"harmless-primary", "missing-primary-result", "missing-disposition", "missing-witness", "repair-missing-original-witness", "malformed-added-witness", "orphan-attempt", "duplicate-primary-family", "controlled-remove-access", "controlled-forget-access", "controlled-repair-access", "controlled-remove-grant", "controlled-open-committed", "controlled-open-cancelled"})
}

func TestRuntimeClosedCompletedReportSemanticCandidatePreservesHistory(t *testing.T) {
	for _, cut := range []publicationapp.RestartAttemptStateV1{publicationapp.RestartAttemptStageDispositionV1, publicationapp.RestartAttemptStageCompletionV1} {
		t.Run(string(cut), func(t *testing.T) {
			testRuntimeCompletedReportSemanticCandidateV1(t, cut, []string{"harmless-primary", "missing-primary-result", "missing-disposition"})
		})
	}
}

func testRuntimeCompletedReportSemanticCandidateV1(t *testing.T, cut publicationapp.RestartAttemptStateV1, faults []string) {
	t.Helper()
	for _, fault := range faults {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: cut, controlled: strings.HasPrefix(fault, "controlled-"), openAccess: strings.HasPrefix(fault, "controlled-open-")})
			if fault == "duplicate-primary-family" {
				// The legacy container is outside managed anchors; establish it
				// before baseline so the negative reaches the identity guard.
				if err := os.MkdirAll(filepath.Join(fixture.core.roots.DurableDir, "runtime-go"), 0700); err != nil {
					t.Fatal(err)
				}
				fixture.refresh(t)
			}
			core := fixture.core
			key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			history, err := prepareRuntimeOriginalReportHistoryV1(ctx, core, fixture.publication, fixture.advance, fixture.scope)
			if err != nil {
				t.Fatal(err)
			}
			if history.semantic == nil || len(fixture.scope.ThreadIDs()) != 0 {
				t.Fatal("completed history did not have an independent semantic guard")
			}
			preserved := runtimeReportRestartPreservationV1{report: fixture.scope, core: core, publication: fixture.publication, history: history}
			if cut != "" {
				preserved, err = prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, core, core.access, fixture.publication.installation, fixture.advance)
				if err != nil || preserved.history == nil || !preserved.history.closedCompletedPrefix {
					t.Fatal("closed prefix requires the full native guard", err)
				}
				if err := preserved.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
					t.Fatal(err)
				}
			}
			var missingWitnessBody []byte
			missingWitnessRelative := ""
			controlledFiles := runtimeOriginalSemanticFilesV1{}
			if strings.HasPrefix(fault, "controlled-") {
				owner := "controlled-artifact-access-v2"
				if fault == "controlled-remove-grant" {
					owner = "pii-authorization"
				}
				for name, entry := range history.controlled.owners[owner] {
					relative := filepath.Join("private", owner, filepath.FromSlash(name))
					controlledFiles[relative] = entry
					if fault == "controlled-forget-access" || fault == "controlled-repair-access" {
						if err := os.Remove(filepath.Join(core.roots.DataDir, relative)); err != nil {
							t.Fatal(err)
						}
					}
				}
				if len(controlledFiles) == 0 {
					t.Fatal("controlled attack has no actual Original records")
				}
			}
			if fault == "repair-missing-original-witness" {
				id := history.plan.Attempts[0].Selection.ObservationDigest
				missingWitnessRelative = filepath.Join("private", "evidence-authority", "observations", id[:2], id+".json")
				target := filepath.Join(core.roots.DataDir, missingWitnessRelative)
				missingWitnessBody, err = os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("completed report semantic " + fault))
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
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "a-report-semantic-marker.bin"), []byte("independent marker"), 0600); err != nil {
					return err
				}
				switch fault {
				case "controlled-open-committed", "controlled-open-cancelled":
					if len(history.controlled.openReceipts) != 1 {
						return errors.New("wrong-terminal fixture needs one original open receipt")
					}
					for _, receipt := range history.controlled.openReceipts {
						status, reason, length := domainpii.ControlledArtifactAccessDispositionHostReleaseCommittedV1, domainpii.ControlledArtifactAccessReasonHostReleaseCommittedV1, receipt.ArtifactByteLength
						if fault == "controlled-open-cancelled" {
							status, reason, length = domainpii.ControlledArtifactAccessDispositionCancelledV1, domainpii.ControlledArtifactAccessReasonAccessCancelledV1, 0
						}
						requested, err := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
						if err != nil {
							return err
						}
						disposition, err := domainpii.NewControlledArtifactAccessDispositionV2(receipt, status, reason, length, requested.Add(time.Second), key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
						if err != nil {
							return err
						}
						body, err := domainpii.ControlledArtifactAccessDispositionV2Bytes(disposition)
						if err != nil {
							return err
						}
						target := filepath.Join(stage.DataDir, "private", "controlled-artifact-access-v2", "access-dispositions", receipt.AccessID[:2], receipt.AccessID+".json")
						if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
							return err
						}
						if err := os.WriteFile(target, body, 0600); err != nil {
							return err
						}
					}
					return nil
				case "controlled-forget-access":
					return nil
				case "controlled-remove-access", "controlled-remove-grant", "controlled-repair-access":
					for relative, entry := range controlledFiles {
						target := filepath.Join(stage.DataDir, relative)
						if fault == "controlled-repair-access" {
							if err := os.WriteFile(target, entry.Body, 0600); err != nil {
								return err
							}
						} else if err := os.Remove(target); err != nil {
							return err
						}
					}
					return nil
				case "repair-missing-original-witness":
					return os.WriteFile(filepath.Join(stage.DataDir, missingWitnessRelative), missingWitnessBody, 0600)
				case "malformed-added-witness":
					id := domainsecurity.SHA256Hex([]byte("invalid added witness"))
					target := filepath.Join(stage.DataDir, "private", "evidence-authority", "observations", id[:2], id+".json")
					if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
						return err
					}
					return os.WriteFile(target, []byte("{}"), 0600)
				case "duplicate-primary-family":
					body, err := os.ReadFile(fixture.primary)
					if err != nil {
						return err
					}
					id := history.plan.Attempts[0].Stage.Context.ThreadID
					target := filepath.Join(stage.DurableDir, "runtime-go", "threads", id, "thread.json")
					if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
						return err
					}
					return os.WriteFile(target, body, 0600)
				case "harmless-primary", "missing-primary-result":
					relative, err := filepath.Rel(core.roots.DurableDir, fixture.primary)
					if err != nil {
						return err
					}
					target := filepath.Join(stage.DurableDir, relative)
					body, err := os.ReadFile(target)
					if err != nil {
						return err
					}
					var primary map[string]any
					if err := json.Unmarshal(body, &primary); err != nil {
						return err
					}
					primary["title"] = "preserved report title"
					if fault == "missing-primary-result" {
						removeRuntimeOriginalReportResultForTestV1(t, primary, history.plan.Attempts[0].GrantSettlement.ResultItemID)
					}
					body, err = json.Marshal(primary)
					if err != nil {
						return err
					}
					return os.WriteFile(target, body, 0600)
				case "missing-disposition":
					id := history.plan.Attempts[0].Stage.WorkID
					return os.Remove(filepath.Join(stage.DataDir, "private", "pending-work", "dispositions", id[:2], id+".json"))
				case "missing-witness":
					id := history.plan.Attempts[0].Selection.ObservationDigest
					return os.Remove(filepath.Join(stage.DataDir, "private", "evidence-authority", "observations", id[:2], id+".json"))
				case "orphan-attempt":
					_, attempt := runtimeReservedReportAttemptFixtureV1(t, key)
					body, err := domainpublication.PublicationAttemptV1Bytes(attempt)
					if err != nil {
						return err
					}
					target := filepath.Join(stage.DataDir, "private", "report-publication", "attempts", attempt.AttemptID[:2], attempt.AttemptID+".json")
					if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
						return err
					}
					return os.WriteFile(target, body, 0600)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if fault != "harmless-primary" {
				applyErr := prepared.Apply(ctx)
				if err := prepared.Close(); err != nil {
					t.Fatal(err)
				}
				if applyErr == nil {
					t.Fatal("invalid completed-history candidate applied its signed program")
				}
				t.Logf("candidate refused before writes: %v", applyErr)
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
					t.Fatal("rejected program changed managed files")
				}
				return
			}
			defer func() {
				if err := prepared.Close(); err != nil {
					t.Error(err)
				}
			}()
			if err := prepared.Apply(ctx); err != nil {
				t.Fatal(err)
			}
			fixture.refresh(t)
			if _, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope); err != nil {
				t.Fatalf("harmless primary rewrite lost historical audit: %v", err)
			}
		})
	}
}
