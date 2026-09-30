//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	fundsquerysourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundsquerysource"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	fundsquerysourceapp "analytix.local/runtime-go/internal/app/fundsquerysource"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRuntimeHostLocalPreparedReceiptUsesActualRegistryCASAndRestarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
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
	freeze := func() *persistencefs.RootAuthority {
		t.Helper()
		root, err := persistencefs.FreezeRootAuthority(persistencefs.RootSet{
			DataDir: config.DataDir, DurableDir: config.DurableTempDir,
		})
		if err != nil {
			t.Fatal(err)
		}
		return root
	}
	stores, err := datasetsnapshotstore.OpenStoresV2(filepath.Join(privateRoot, "dataset-snapshot-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer stores.Close()
	owner, err := newRuntimeHostLocalEvidenceOwnersV1(config, freeze(), access, key, stores,
		runtimeHostLocalProfileAdmitV1, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.EnsureForFundsImport(ctx); err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	manifest, producer, materials := runtimeSharedEvidenceBoundMaterialsV2(t, workspace, observation, key.KeyID(), key)
	materialCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(privateRoot, "dataset-snapshot-authority", "materials"), 16*1024*1024, access)
	if err != nil {
		t.Fatal(err)
	}
	for _, records := range materials {
		for address, body := range records {
			if err := materialCAS.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
				t.Fatal(err)
			}
		}
	}
	if err := materialCAS.Close(); err != nil {
		t.Fatal(err)
	}
	selected, err := owner.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, ManifestReference: manifest, FundsProducerReference: producer,
		AcceptedAt: time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	hostSource, err := fundsquerysourceadapter.NewHostExactSource(config.UserDataDir)
	if err != nil {
		t.Fatal(err)
	}
	query, err := fundsquerysourceapp.NewService(owner, filestore.CaseBindingReader{}, stores.Materials, hostSource)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := query.ResolveCurrentLocalDisplay(ctx, workspace,
		domainsecurity.LocalTenantID, domainsecurity.LocalUserID)
	if err != nil || descriptor.DatasetSnapshotID != selected.Record.DatasetSnapshotID {
		t.Fatalf("host-local current source descriptor is unavailable: %v", err)
	}
	resolve := datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, ExpectedDatasetSnapshotID: selected.Record.DatasetSnapshotID,
	}
	securityContext, durable := runtimeHostLocalPrepareFinalTurnV6(t, config, workspace, observation, selected)
	var prepared domainevidence.PreparedEvidenceSettlement
	var receipt domainevidence.EvidenceReceipt
	err = owner.WithCurrentSelectionV2(ctx, resolve, securityContext,
		func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			if selection.HostLocalHead == nil || selection.Head.HasBundle {
				t.Fatal("host-local selection borrowed a witnessed head")
			}
			prepared = runtimeHostLocalPreparedSettlementV1(t, securityContext, resolve, key, selection)
			marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
			if err != nil {
				return err
			}
			effect, ok := capability.(datasetsnapshotport.RegistryCommitCapabilityV1)
			if !ok {
				t.Fatal("host-local selection has no actual registry effect")
			}
			return effect.UseExactRegistryCommit(selection, securityContext, prepared, marker,
				func(lease context.Context) error {
					var err error
					receipt, err = owner.CommitPrepared(lease, preparedRegistryCommitInputV1(prepared))
					return err
				})
		})
	if err != nil || receipt.ReceiptID != prepared.ReceiptID {
		t.Fatalf("host-local actual prepared registry commit failed: receipt=%q err=%v", receipt.ReceiptID, err)
	}
	registered, err := owner.Resolve(ctx, registryport.MembershipQuery{
		Context: securityContext, ReceiptID: receipt.ReceiptID,
	})
	if err != nil || registered.Revoked || registered.Receipt.ReceiptID != receipt.ReceiptID {
		t.Fatalf("host-local committed receipt was not selected: %v", err)
	}
	before, found, err := owner.heads.Current(ctx)
	if err != nil || !found || before.DatasetSnapshotCount != 1 || before.EvidenceRegistryCount != 1 {
		t.Fatalf("host-local exact child counts are wrong: %v", err)
	}
	originalFinal := runtimeHostLocalIssueAndPersistFinalV6(t, ctx, owner, access, privateRoot, securityContext, resolve, prepared, receipt, config, key, durable)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stores.Close(); err != nil {
		t.Fatal(err)
	}
	access, err = privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	key, err = finalauthority.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	stores, err = datasetsnapshotstore.OpenStoresV2(filepath.Join(privateRoot, "dataset-snapshot-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer stores.Close()
	resumed, err := newRuntimeHostLocalEvidenceOwnersV1(config, freeze(), access, key, stores,
		runtimeHostLocalProfileResumeV1, func(context.Context) error { t.Fatal("resume re-entered fresh admission"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	registered, err = resumed.Resolve(ctx, registryport.MembershipQuery{
		Context: securityContext, ReceiptID: receipt.ReceiptID,
	})
	if err != nil || registered.Revoked || registered.Receipt.ReceiptID != receipt.ReceiptID {
		t.Fatalf("host-local restart lost selected receipt: %v", err)
	}
	after, found, err := resumed.heads.Current(ctx)
	if err != nil || !found || after != before {
		t.Fatalf("host-local restart changed signed head: %v", err)
	}
	runtimeHostLocalRecoverFinalAfterSuccessorV6(t, ctx, resumed, access, privateRoot, selected, resolve, originalFinal, func() {
		runtimeHostLocalRestoreFinalDeliveryV6(t, ctx, config, resumed, key, access, originalFinal)
	})
}

func runtimeHostLocalFactContextV1(t *testing.T, workspace string,
	observation domainsecurity.CaseBindingObservationV1,
	snapshot datasetsnapshotport.ResolvedSnapshotV2, threadID, turnID string) domainsecurity.TurnSecurityContext {
	t.Helper()
	input := domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
		DatasetSnapshotID:  snapshot.Record.DatasetSnapshotID,
		SourceManifestHash: snapshot.Record.SourceManifestHash, ContextEpoch: 1,
		IssuedAt: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC),
	}
	frozen, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	input.RiskAuthorityBinding = frozen.RiskAuthorityBinding
	input.PublicationPolicy, err = domainsecurity.NewTurnPublicationPolicyV1(
		domainsecurity.TurnPublicationPolicyInputV1{
			ThreadRiskPolicyDigest:   frozen.PublicationPolicy.ThreadRiskPolicyDigest,
			RiskClass:                domainsecurity.RiskClassCase,
			Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
			CaseBindingState:         domainsecurity.CaseBindingStateValid,
			BindingObservationDigest: observation.ObservationDigest,
			BlockerCode:              domainsecurity.PublicationBlockerNone,
		})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err = domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func runtimeHostLocalPreparedSettlementV1(t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	resolve datasetsnapshotport.ResolveInputV2,
	authority finalauthorityport.Authority,
	selection datasetsnapshotport.CurrentSelectionV2) domainevidence.PreparedEvidenceSettlement {
	t.Helper()
	var draftInput domainevidence.EvidenceReceiptInput
	base := runtimePreparedSettlementWithDraftForSemanticTestV1(t, securityContext, authority,
		func(draft *domainevidence.EvidenceReceiptInput) ([]byte, []byte) {
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
	input := domainevidence.PreparedEvidenceSettlementInput{
		Context: base.SecurityContext, Grant: base.ExecutionGrant,
		ActiveGrantRegistrySequence: base.ActiveGrantRegistrySequence,
		ActiveGrantRegistryDigest:   base.ActiveGrantRegistryDigest,
		SourceProbe:                 base.SourceProbe, ToolOutcome: base.ToolOutcome,
		RawResult: raw, CanonicalEvidence: base.CanonicalEvidence,
		ReceiptDraft: base.ReceiptDraft, QueryHash: base.QueryHash,
		ResultItemID: base.ResultItemID, PreparedAt: preparedAt,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
		HostAuthority: &domainevidence.PreparedEvidenceHostAuthorityV1{
			Binding: resolve.Observation, SelectionDigest: selection.SelectionDigest,
			SelectionContentDigest: content,
		},
	}
	draftInput.ReceiptID = domainevidence.EvidenceSettlementReceiptID(domainevidence.ComputeEvidenceSettlementID(input))
	draftInput.RawSHA256, draftInput.ResultHash = base.RawSHA256, base.ReceiptDraft.ResultHash
	input.ReceiptDraft, err = domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := domainevidence.NewPreparedEvidenceSettlement(input,
		func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	if err := domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(
		prepared, securityContext, prepared.SourceProbe, selection.SelectionDigest, content); err != nil {
		t.Fatal(err)
	}
	return prepared
}
