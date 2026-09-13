//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	formalauthority "analytix.local/runtime-go/internal/formalauthority"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// These use the actual enrolled witness, Authority, DSV2 service, secure CAS
// stores and shared registry owner. The prepared facts are synthetic fixture
// inputs, not native-analysis or Final Gate acceptance evidence.
func TestPreparedRegistryEffectUsesOnlyOwnActualCAS(t *testing.T) {
	for _, fault := range []string{"none", "readonly", "empty callback", "wrong marker", "wrong context", "tampered prepared", "wrong receipt", "pre-head drift", "extra advance", "sibling advance", "cancel before", "cancel after", "post-witness unavailable", "swallowed error", "repeat operation", "commit outside operation"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newPreparedRegistryEffectFixtureV1(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var prepared domainevidence.PreparedEvidenceSettlement
			var input registryport.CommitPreparedInput
			var retained datasetsnapshotport.RegistryCommitCapabilityV1
			var selection datasetsnapshotport.CurrentSelectionV2
			var leaked context.Context
			committed := false
			err := fixture.composition.snapshot.WithCurrentSelectionV2(ctx, fixture.resolve, fixture.securityContext,
				func(selected datasetsnapshotport.CurrentSelectionV2, cap datasetsnapshotport.CurrentSelectionCapabilityV2) error {
					selection = selected
					prepared = fixture.prepare(t, selected)
					marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
					if err != nil {
						t.Fatal(err)
					}
					input = preparedRegistryCommitInputV1(prepared)
					effect, ok := cap.(datasetsnapshotport.RegistryCommitCapabilityV1)
					if !ok {
						t.Fatal("actual DSV2 capability has no registry effect")
					}
					retained = effect
					if fault == "wrong marker" {
						marker.ReceiptID += "-wrong"
					}
					securityContext := fixture.securityContext
					if fault == "wrong context" {
						securityContext.ContextEpoch++
					}
					if fault == "tampered prepared" {
						prepared.HostAuthority.SelectionContentDigest = domainsecurity.SHA256Hex([]byte("forged"))
					}
					if fault == "pre-head drift" {
						fixture.advanceSibling(t, ctx)
					}
					if fault == "cancel before" {
						cancel()
					}
					use := func(lease context.Context) error {
						leaked = lease
						if fault == "empty callback" {
							return nil
						}
						if fault == "wrong receipt" {
							input.Draft.ReceiptID += "-wrong"
						}
						commitContext := lease
						if fault == "commit outside operation" {
							commitContext = ctx
						}
						receipt, err := fixture.composition.registryOwner.CommitPrepared(commitContext, input)
						if err != nil {
							return err
						}
						if receipt.ReceiptID != prepared.ReceiptID {
							t.Fatal("actual committed receipt is not the prepared receipt")
						}
						committed = true
						switch fault {
						case "extra advance":
							if err := fixture.composition.registryOwner.Revoke(lease, registryport.RevokeInput{Context: fixture.securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: input.RegisteredAt.Add(time.Minute)}); err != nil {
								t.Fatal(err)
							}
						case "sibling advance":
							fixture.advanceSibling(t, lease)
						case "cancel after":
							cancel()
						case "post-witness unavailable":
							fixture.witness.SetWitnessAvailable(domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1, false)
						case "swallowed error":
							_, duplicateErr := fixture.composition.registryOwner.CommitPrepared(lease, input)
							if duplicateErr == nil {
								t.Fatal("second operation in same effect was accepted")
							}
						}
						return nil
					}
					if fault == "readonly" {
						return cap.UseExact(selected, securityContext, use)
					}
					err = effect.UseExactRegistryCommit(selected, securityContext, prepared, marker, use)
					if fault == "repeat operation" && err == nil {
						err = effect.UseExactRegistryCommit(selected, securityContext, prepared, marker, use)
					}
					if fault == "none" && err == nil {
						if err := cap.UseExact(selected, securityContext, func(context.Context) error { return nil }); err == nil {
							t.Fatal("old read capability was silently advanced")
						}
					}
					return err
				})
			fixture.witness.SetWitnessAvailable(domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1, true)
			if (fault == "none") != (err == nil) {
				t.Fatalf("effect result fault=%s committed=%t err=%v", fault, committed, err)
			}
			marker, _ := domainevidence.NewHostEvidenceSettlementMarker(prepared)
			if retained != nil && retained.UseExactRegistryCommit(selection, fixture.securityContext, prepared, marker, func(context.Context) error { return nil }) == nil {
				t.Fatal("expired effect capability remained usable")
			}
			if leaked != nil {
				if _, err := fixture.composition.registryOwner.CommitPrepared(leaked, input); err == nil {
					t.Fatal("expired operation context minted a receipt")
				}
			}
			head, err := fixture.composition.evidence.ObserveFresh(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			expected := uint64(0)
			if committed {
				expected = 1
			}
			if fault == "extra advance" {
				expected = 2
			}
			if head.Bundle.EvidenceRegistryCount != expected {
				t.Fatal("registry mutation count does not match actual commit")
			}
			if !committed {
				return
			}
			registry, err := fixture.composition.registryOwner.Replay(context.Background(), fixture.securityContext)
			if err != nil {
				t.Fatal(err)
			}
			if _, found, err := domainevidence.ResolvePreparedSettlementIssue(registry, prepared); err != nil || !found {
				t.Fatal("reported error erased the actual committed issue")
			}
			if fault != "post-witness unavailable" {
				return
			}
			// Reopen actual durable owners against the existing enrolled witness.
			// The same signed marker/receipt resolves once; an exact retry adds no
			// new issue. Capsule presence alone was never used as commit proof.
			before := startupWholeTreeDigest(t, filepath.Join(fixture.config.DataDir, "private", "evidence-registry"))
			if err := fixture.composition.registryOwner.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := runtimeWitnessedRegistryDirectCompositionV2(t, fixture.witness, fixture.config)
			if reopened.registryOwner == nil || reopened.registry == nil {
				t.Fatal("actual committed registry did not recover")
			}
			defer reopened.registryOwner.Close()
			registry, err = reopened.registryOwner.Replay(context.Background(), fixture.securityContext)
			if err != nil {
				t.Fatal(err)
			}
			resolved, found, err := domainevidence.ResolvePreparedSettlementIssue(registry, prepared)
			if err != nil || !found || registry.Sequence != 1 {
				t.Fatal("restart did not resolve one exact committed issue")
			}
			retry, err := reopened.registryOwner.CommitPrepared(context.Background(), input)
			if err != nil || retry.ReceiptID != resolved.ReceiptID {
				t.Fatal("idempotent exact retry failed")
			}
			after, err := reopened.evidence.ObserveFresh(context.Background())
			if err != nil || after.Bundle != head.Bundle || before != startupWholeTreeDigest(t, filepath.Join(fixture.config.DataDir, "private", "evidence-registry")) {
				t.Fatal("recovery duplicated or changed the committed issue")
			}
		})
	}
}

type preparedRegistryEffectFixtureV1 struct {
	witness         *formalauthority.Service
	config          Config
	composition     runtimeSharedEvidenceDatasetSnapshotV2
	resolve         datasetsnapshotport.ResolveInputV2
	securityContext domainsecurity.TurnSecurityContext
}

func newPreparedRegistryEffectFixtureV1(t *testing.T) *preparedRegistryEffectFixtureV1 {
	t.Helper()
	ctx := context.Background()
	witness, config := runtimeWitnessedRegistryConfigV2(t)
	composition := runtimeWitnessedRegistryDirectCompositionV2(t, witness, config)
	if composition.registryOwner == nil {
		t.Fatal("cold explicit-import registry owner missing")
	}
	t.Cleanup(func() { _ = composition.registryOwner.Close() })
	if _, err := composition.evidence.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	manifest, producer, materials := runtimeSharedEvidenceBoundMaterialsV2(t, workspace, observation, witness.InstallationID, witness.Authority)
	access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", "dataset-snapshot-authority", "materials"), 16*1024*1024, access)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, records := range materials {
		for address, body := range records {
			if err := store.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
				t.Fatal(err)
			}
		}
	}
	selected, err := composition.snapshot.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation, ManifestReference: manifest, FundsProducerReference: producer, AcceptedAt: time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.registryOwner.ActivateAfterImport(ctx, observation, selected.Record.DatasetSnapshotID); err != nil {
		t.Fatal(err)
	}
	input := domainsecurity.TurnSecurityContextInput{ThreadID: "effect-thread", TurnID: "effect-turn", WorkspaceRealPath: workspace, TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash, DatasetSnapshotID: selected.Record.DatasetSnapshotID, SourceManifestHash: selected.Record.SourceManifestHash, ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)}
	frozen, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	input.RiskAuthorityBinding = frozen.RiskAuthorityBinding
	input.PublicationPolicy, err = domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{ThreadRiskPolicyDigest: frozen.PublicationPolicy.ThreadRiskPolicyDigest, RiskClass: domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid, BindingObservationDigest: observation.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err = domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return &preparedRegistryEffectFixtureV1{witness: witness, config: config, composition: composition, securityContext: frozen, resolve: datasetsnapshotport.ResolveInputV2{TenantID: input.TenantID, UserID: input.UserID, Observation: observation, ExpectedDatasetSnapshotID: input.DatasetSnapshotID}}
}

func (fixture *preparedRegistryEffectFixtureV1) prepare(t *testing.T, selection datasetsnapshotport.CurrentSelectionV2) domainevidence.PreparedEvidenceSettlement {
	t.Helper()
	var draftInput domainevidence.EvidenceReceiptInput
	base := runtimePreparedSettlementWithDraftForSemanticTestV1(t, fixture.securityContext, fixture.witness.Authority, func(draft *domainevidence.EvidenceReceiptInput) ([]byte, []byte) {
		draftInput = *draft
		body := []byte(`{"schemaVersion":1,"facts":[{"factId":"fact-count","claimType":"count","normalizedPayload":{"subjectId":"dataset:analysis_txn_detail_idx","entityId":"dataset:analysis_txn_detail_idx","count":"3","granularity":"dataset_table_rows"}}]}`)
		canonical, err := domainevidence.CanonicalEvidenceBytes(body)
		if err != nil {
			t.Fatal(err)
		}
		return canonical, canonical
	})
	raw, err := base64.RawStdEncoding.DecodeString(base.RawResultBase64)
	if err != nil {
		t.Fatal(err)
	}
	content, err := datasetsnapshotport.CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	preparedAt, _ := time.Parse(time.RFC3339Nano, base.PreparedAt)
	input := domainevidence.PreparedEvidenceSettlementInput{Context: base.SecurityContext, Grant: base.ExecutionGrant, ActiveGrantRegistrySequence: base.ActiveGrantRegistrySequence, ActiveGrantRegistryDigest: base.ActiveGrantRegistryDigest, SourceProbe: base.SourceProbe, ToolOutcome: base.ToolOutcome, RawResult: raw, CanonicalEvidence: base.CanonicalEvidence, ReceiptDraft: base.ReceiptDraft, QueryHash: base.QueryHash, ResultItemID: base.ResultItemID, PreparedAt: preparedAt, AuthorityKeyID: fixture.witness.Authority.KeyID(), AuthorityPublicKey: fixture.witness.Authority.PublicKey(), HostAuthority: &domainevidence.PreparedEvidenceHostAuthorityV1{Binding: fixture.resolve.Observation, SelectionDigest: selection.SelectionDigest, SelectionContentDigest: content}}
	draftInput.ReceiptID = domainevidence.EvidenceSettlementReceiptID(domainevidence.ComputeEvidenceSettlementID(input))
	draftInput.RawSHA256, draftInput.ResultHash = base.RawSHA256, base.ReceiptDraft.ResultHash
	input.ReceiptDraft, err = domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := domainevidence.NewPreparedEvidenceSettlement(input, func(body []byte) ([]byte, error) { return fixture.witness.Authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	if err := domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(prepared, fixture.securityContext, prepared.SourceProbe, selection.SelectionDigest, content); err != nil {
		t.Fatal(err)
	}
	return prepared
}

func preparedRegistryCommitInputV1(prepared domainevidence.PreparedEvidenceSettlement) registryport.CommitPreparedInput {
	settled, _ := time.Parse(time.RFC3339Nano, prepared.PreparedAt)
	return registryport.CommitPreparedInput{Context: prepared.SecurityContext, Draft: prepared.ReceiptDraft, CanonicalEvidence: append(json.RawMessage(nil), prepared.CanonicalEvidence...), SettlementProof: domainevidence.EvidenceSettlementProof{SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest}, RegisteredAt: settled.Add(time.Minute)}
}

func (fixture *preparedRegistryEffectFixtureV1) advanceSibling(t *testing.T, ctx context.Context) {
	t.Helper()
	head, err := fixture.composition.evidence.ObserveFresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.composition.evidence.AdvancePublication(ctx, evidenceauthorityport.PublicationAdvanceInput{ExpectedBundleDigest: head.Bundle.RecordDigest, NextIndexDigest: domainsecurity.SHA256Hex([]byte("effect-test-publication-child"))})
	if err != nil {
		t.Fatal(err)
	}
}
