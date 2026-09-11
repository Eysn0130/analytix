package evidence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	caseentityadapter "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	evidenceregistryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	evidenceregistryv2fixture "analytix.local/runtime-go/internal/testsupport/evidenceregistryv2"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestConcreteWitnessedRegistryFinalizerIssuesFactFinalAndFailsClosedOnDrift(t *testing.T) {
	t.Run("typed case reference produces closed longitudinal slot authority", func(t *testing.T) {
		typedReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("6", 64))
		if err != nil {
			t.Fatal(err)
		}
		reference := string(typedReference)
		payload := amountClaimPayload()
		payload.SubjectID = reference
		payload.EntityID = reference
		payload.AccountID = reference
		fixture := newConcreteRegistryFinalizerFixture(t, payload)
		caseEntityRoot := filepath.Join(t.TempDir(), "case-entities")
		access, err := privatecastest.NewAccessAuthority(caseEntityRoot)
		if err != nil {
			t.Fatal(err)
		}
		caseStore, err := caseentityadapter.NewStore(caseEntityRoot, access)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := caseStore.Close(); err != nil {
				t.Errorf("close concrete longitudinal case store: %v", err)
			}
		}()
		caseEntities := caseentityapp.NewPersistentService(
			concreteRegistryCaseEntityDigesterV1{}, caseStore, fixture.datasetAuthority, fixture.observer,
			func(_ context.Context, candidate domainsecurity.TurnSecurityContext) error {
				if candidate != fixture.securityContext {
					return errors.New("concrete longitudinal current context changed")
				}
				return nil
			},
		)
		boundReference, err := caseEntities.BindReferenceV1(
			context.Background(), caseentityapp.NewDeriveReferenceInputV1(
				fixture.securityContext,
				domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				"6222021234567890",
			),
		)
		if err != nil || boundReference != typedReference {
			t.Fatalf("concrete longitudinal entity binding changed: reference=%s err=%v", boundReference, err)
		}
		if err := caseEntities.AppendCaseLongitudinalIngressV1(
			context.Background(), caseentityapp.AppendCaseLongitudinalIngressInputV1{
				SecurityContext: fixture.securityContext,
				References:      []domaincaseentity.ReferenceV1{boundReference},
			},
		); err != nil {
			t.Fatal(err)
		}
		if err := caseentityapp.AppendPersistedEvidenceReceiptV1(
			context.Background(), caseEntities, caseentityapp.AppendPersistedEvidenceReceiptInputV1{
				SecurityContext: fixture.securityContext, Receipt: fixture.receipt,
			},
		); err != nil {
			t.Fatal(err)
		}
		result, err := fixture.persist(context.Background())
		if err != nil || result.Boundary.Envelope.Variant != domainevidence.EvidenceBackedAnswer {
			t.Fatalf("typed case-reference finalization failed: result=%#v err=%v", result, err)
		}
		calls := 0
		available, err := result.UseCaseLongitudinalAcceptedSlotsV1(func(
			securityContext domainsecurity.TurnSecurityContext,
			acceptedFinalDigest string,
			dispositionDigest string,
			finalGateVersion string,
			slot domainevidence.AcceptedEntitySlotBindingV1,
		) error {
			calls++
			referenceCalls := 0
			return slot.UseReferenceV1(func(candidate domaincaseentity.ReferenceV1) error {
				referenceCalls++
				if securityContext != fixture.securityContext || candidate != domaincaseentity.ReferenceV1(reference) ||
					acceptedFinalDigest != result.Persistence.AcceptedFinal.RecordDigest ||
					!domainsecurity.IsSHA256Hex(dispositionDigest) ||
					finalGateVersion != domainevidence.FinalEvidenceGateVersion ||
					slot.SlotID != "account-slot-1" || len(slot.ClaimIDs) != 1 ||
					len(slot.ReceiptIDs) != 1 || slot.ReceiptIDs[0] != fixture.receipt.ReceiptID || referenceCalls != 1 {
					return errors.New("closed longitudinal accepted slot changed")
				}
				return nil
			})
		})
		if err != nil || !available || calls != 1 {
			slots, slotsErr := domainevidence.BuildAcceptedEntitySlotBindingsV1(result.Boundary.Envelope)
			t.Fatalf("production finalization omitted its exact eligible slot: available=%t calls=%d err=%v slots=%#v slotsErr=%v envelope=%#v", available, calls, err, slots, slotsErr, result.Boundary.Envelope)
		}
		if err := caseentityapp.AppendFinalizedCaseLongitudinalStateV1(
			context.Background(), caseEntities, concreteRegistryFinalizedThreadReaderV1{}, fixture.securityContext.ThreadID,
			fixture.securityContext, &result,
		); err != nil {
			t.Fatalf("production accepted slot did not enter the longitudinal owner: %v", err)
		}
		index, err := caseStore.ResolveLatestCaseLongitudinalContext(context.Background(), fixture.securityContext)
		if err != nil || len(index.DisplayBindings) != 1 ||
			index.DisplayBindings[0].AcceptedFinalDigest != result.Persistence.AcceptedFinal.RecordDigest ||
			index.DisplayBindings[0].EntityReference != typedReference ||
			index.DisplayBindings[0].SlotID != "account-slot-1" ||
			len(index.DisplayBindings[0].ClaimBindings) != 1 ||
			len(index.DisplayBindings[0].EvidenceReceiptBindings) != 1 ||
			index.DisplayBindings[0].EvidenceReceiptBindings[0].EvidenceReference != fixture.receipt.ReceiptID {
			t.Fatalf("production finalization did not persist one exact display binding: index=%#v err=%v", index, err)
		}
		body, err := json.Marshal(result)
		if err != nil || bytes.Contains(body, []byte("useCaseLongitudinalAcceptedSlotsV1")) ||
			bytes.Contains(body, []byte("authorityEntityRef")) {
			t.Fatalf("closed longitudinal authority entered ordinary result serialization: body=%s err=%v", body, err)
		}
	})

	t.Run("current exact authority issues witness and advances terminal", func(t *testing.T) {
		fixture := newConcreteRegistryFinalizerFixture(t)
		result, err := fixture.persist(context.Background())
		if err != nil || !result.Persistence.Changed ||
			result.Boundary.Envelope.Variant != domainevidence.EvidenceBackedAnswer ||
			len(result.Boundary.Envelope.EvidenceReceiptIDs) != 1 ||
			result.Boundary.Envelope.EvidenceReceiptIDs[0] != fixture.receipt.ReceiptID ||
			result.Persistence.AcceptedFinal.FactFinalWitnessAdmission == nil ||
			result.Persistence.AcceptedFinal.PublicationSnapshotProofDigest == "" ||
			result.PublicationIntent.TerminalStatus != "completed" || fixture.store.status != "completed" {
			t.Fatalf("concrete witnessed finalization did not enter the next legal terminal state: variant=%q blocker=%q status=%q persistErr=%v authorityErr=%v", result.Boundary.Envelope.Variant, result.Boundary.Envelope.Blocker, fixture.store.status, err, fixture.host.lastErr)
		}
		concrete, ok := fixture.finalizer.(*casePublicationFinalizer)
		if !ok || concrete.registry != fixture.registry || concrete.lockedSnapshots != fixture.registry ||
			concrete.factFinalWitnesses != fixture.registry {
			t.Fatal("real finalizer does not share the exact concrete witnessed registry/issuer instance")
		}
		publicBytes, marshalErr := json.Marshal(result.Persistence.AcceptedFinal.PublicView)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, forbidden := range []string{
			"factFinalWitnessAdmission", "authorityRef", "publicationSnapshotProof", "sourceExactValue",
			fixture.binding.BindingSHA256, fixture.receipt.RawSHA256,
		} {
			if strings.Contains(string(publicBytes), forbidden) {
				t.Fatalf("private fact-final authority escaped the public terminal projection: forbidden=%q body=%s", forbidden, publicBytes)
			}
		}

		recovery, err := fixture.registryCAS.PrepareRecovery(context.Background())
		if err != nil {
			t.Fatalf("prepare non-empty witnessed registry recovery: %v", err)
		}
		if err := recovery.ValidateSemantics(context.Background()); err != nil {
			t.Fatalf("validate non-empty witnessed registry recovery: %v", err)
		}
		if err := recovery.Revalidate(context.Background()); err != nil {
			t.Fatalf("revalidate non-empty witnessed registry recovery: %v", err)
		}

		freshRegistry := fixture.freshRegistryInstance(t)
		member, err := freshRegistry.Resolve(context.Background(), registryport.MembershipQuery{
			Context: fixture.securityContext, ReceiptID: fixture.receipt.ReceiptID,
		})
		if err != nil || member.Revoked || !reflect.DeepEqual(member.Receipt, fixture.receipt) {
			t.Fatalf("fresh witnessed registry instance lost exact receipt membership: member=%#v err=%v", member, err)
		}
		var freshSnapshot registryport.WitnessedSnapshot
		if err := freshRegistry.WithWitnessedSnapshot(
			context.Background(), fixture.securityContext,
			func(snapshot registryport.WitnessedSnapshot) error {
				freshSnapshot = snapshot
				return nil
			},
		); err != nil || freshSnapshot.SelectedIndex.IndexDigest == "" ||
			freshSnapshot.SelectedCapsule.RecordDigest == "" ||
			!domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(
				freshSnapshot.SelectedIndex, freshSnapshot.SelectedCapsule,
			) {
			t.Fatalf("fresh witnessed registry instance lost its current index/capsule chain: snapshot=%#v err=%v", freshSnapshot, err)
		}
		records, err := fixture.privateStore.List(context.Background())
		if err != nil || len(records) != 1 || records[0].AcceptedFinal.FactFinalWitnessAdmission == nil {
			t.Fatalf("fact-final private record is unavailable for fresh-instance verification: records=%#v err=%v", records, err)
		}
		beforeAdvance := fixture.coordinator.advanceCalls
		beforeAdmission := *records[0].AcceptedFinal.FactFinalWitnessAdmission
		beforeSignature := records[0].AcceptedFinal.AuthoritySignature
		if err := freshRegistry.VerifyFactFinalWitnessCurrent(context.Background(), records[0]); err != nil {
			t.Fatalf("fresh concrete service instance rejected the existing current witness: %v", err)
		}
		if fixture.coordinator.advanceCalls != beforeAdvance ||
			!reflect.DeepEqual(*records[0].AcceptedFinal.FactFinalWitnessAdmission, beforeAdmission) ||
			records[0].AcceptedFinal.AuthoritySignature != beforeSignature {
			t.Fatal("fresh-instance verification re-signed or mutated the existing fact-final witness")
		}
	})

	t.Run("concurrent registry revocation is stale and boundary only", func(t *testing.T) {
		fixture := newConcreteRegistryFinalizerFixture(t)
		fixture.host.beforeCallback = func() error {
			return fixture.registry.Revoke(context.Background(), registryport.RevokeInput{
				Context: fixture.securityContext, ReceiptID: fixture.receipt.ReceiptID,
				ReasonCode: "source_retracted", RevokedAt: fixture.now.Add(7 * time.Minute),
			})
		}
		fixture.assertBoundaryOnly(t)
	})

	t.Run("already revoked membership cannot request a witness", func(t *testing.T) {
		fixture := newConcreteRegistryFinalizerFixture(t)
		if err := fixture.registry.Revoke(context.Background(), registryport.RevokeInput{
			Context: fixture.securityContext, ReceiptID: fixture.receipt.ReceiptID,
			ReasonCode: "source_retracted", RevokedAt: fixture.now.Add(7 * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
		fixture.assertBoundaryOnly(t)
		if fixture.host.calls != 0 {
			t.Fatalf("revoked registry membership reached host evidence authority: calls=%d", fixture.host.calls)
		}
	})

	t.Run("cross snapshot host probe cannot issue a witness", func(t *testing.T) {
		fixture := newConcreteRegistryFinalizerFixture(t)
		fixture.host.crossSnapshot = true
		fixture.assertBoundaryOnly(t)
	})

	t.Run("cross case binding observation cannot issue a witness", func(t *testing.T) {
		fixture := newConcreteRegistryFinalizerFixture(t)
		cross, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
			WorkspaceRealPath: fixture.binding.WorkspaceRealPath, State: domainsecurity.CaseBindingStateValid,
			CaseID: "case-concrete-registry-cross", BindingSHA256: domainsecurity.SHA256Hex([]byte("cross-binding-file")),
			CaseBindingHash: domainsecurity.SHA256Hex([]byte("cross-binding")),
		})
		if err != nil {
			t.Fatal(err)
		}
		fixture.observer.observation = cross
		fixture.assertBoundaryOnly(t)
		if fixture.host.calls != 0 {
			t.Fatalf("cross-case binding reached host evidence authority: calls=%d", fixture.host.calls)
		}
	})
}

type concreteRegistryFinalizerFixture struct {
	now              time.Time
	securityContext  domainsecurity.TurnSecurityContext
	binding          domainsecurity.CaseBindingObservationV1
	registry         *evidenceregistryapp.Service
	registryCAS      *evidenceregistryv2fixture.Fixture
	coordinator      *concreteRegistryCoordinator
	datasetAuthority *concreteRegistryDatasetAuthority
	receipt          domainevidence.EvidenceReceipt
	finalizer        CasePublicationFinalizer
	privateStore     *memoryPrivateFinalStore
	store            *caseTerminalStoreStub
	host             *concreteRegistryHostAuthority
	observer         *concreteRegistryBindingObserver
}

type concreteRegistryCaseEntityDigesterV1 struct{}

type concreteRegistryFinalizedThreadReaderV1 struct{}

func (concreteRegistryFinalizedThreadReaderV1) GetThread(string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (concreteRegistryCaseEntityDigesterV1) KeyedPayloadHash(
	context.Context,
	string,
	[]byte,
) (string, error) {
	return strings.Repeat("6", 64), nil
}

func newConcreteRegistryFinalizerFixture(
	t *testing.T,
	payloads ...domainevidence.NormalizedClaimPayload,
) *concreteRegistryFinalizerFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	authority := newMemoryFinalAuthority(0x73)
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x74}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("concrete-registry-finalizer-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("concrete-registry-finalizer-enrollment"))
	const (
		workspace = "/workspace/concrete-registry-finalizer"
		threadID  = "thread-concrete-registry-finalizer"
		turnID    = "turn-concrete-registry-finalizer"
		caseID    = "case-concrete-registry-finalizer"
	)
	binding, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateValid, CaseID: caseID,
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("concrete-registry-binding-file")),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("concrete-registry-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: binding, Material: "concrete-registry-finalizer", InstallationID: installationID,
		AcceptedAt: now.Add(-2 * time.Minute), AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.publicKey,
		Sign: func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) },
	})
	if err != nil {
		t.Fatal(err)
	}
	datasetIndex, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:          domainsecurity.SHA256Hex([]byte("concrete-registry-dataset-index")),
		Binding:             resolved.Record.Binding, SnapshotRecordDigest: resolved.Record.RecordDigest,
		AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                 domainsecurity.SHA256Hex([]byte("concrete-registry-bundle-genesis")),
		DatasetSnapshotIndexDigest: datasetIndex.IndexDigest, DatasetSnapshotCount: 1,
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.keyID, AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &concreteRegistryCoordinator{
		bundle: bundle, authority: authority, witnessPrivate: witnessPrivate, witnessPublic: witnessPublic,
		history: map[string]evidenceauthorityport.ObservationBundle{},
	}
	policyDigest := domainsecurity.SHA256Hex([]byte("concrete-registry-risk-policy"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: binding.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: caseID, CaseBindingHash: binding.CaseBindingHash,
		DatasetSnapshotID: resolved.Record.DatasetSnapshotID, SourceManifestHash: resolved.Record.SourceManifestHash,
		ContextEpoch: 4, IssuedAt: now, PublicationPolicy: policy, RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		Head:             evidenceauthorityport.FreshHead{HasBundle: true, Bundle: bundle},
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{datasetIndex}, SelectedIndex: datasetIndex, Snapshot: resolved,
	}
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	registryCAS, err := evidenceregistryv2fixture.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registryStores := registryCAS.Stores()
	observer := &concreteRegistryBindingObserver{workspace: workspace, observation: binding}
	datasetAuthority := &concreteRegistryDatasetAuthority{
		coordinator: coordinator, binding: binding, selection: selection,
	}
	registry, err := evidenceregistryapp.New(evidenceregistryapp.Config{
		InstallationID: installationID, EnrollmentID: enrollmentID, Authority: authority,
		WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessKey: witnessPublic,
		Coordinator: coordinator, WitnessChain: coordinator,
		Indexes: registryStores.Indexes, Capsules: registryStores.Capsules,
		DatasetAuthority: datasetAuthority, BindingObserver: observer,
		Random: bytes.NewReader(bytes.Repeat([]byte{0x75}, 4096)), Now: func() time.Time { return now.Add(6 * time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt := commitConcreteRegistryEvidence(t, registry, securityContext, now, payloads...)
	host := &concreteRegistryHostAuthority{
		coordinator: coordinator, binding: binding, selection: selection,
	}
	privateStore := &memoryPrivateFinalStore{
		records:      map[string]domainevidence.PrivateAcceptedFinalRecord{},
		dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
	}
	finalizer := NewCasePublicationFinalizerWithHostEvidenceAuthority(
		registry, registry, authority, privateStore, newTestFinalPublicationEventIO(),
		newTestTurnTerminalCoordinator(authority, privateStore), host, observer,
	)
	return &concreteRegistryFinalizerFixture{
		now: now, securityContext: securityContext, binding: binding, registry: registry,
		registryCAS: registryCAS, coordinator: coordinator,
		datasetAuthority: datasetAuthority, receipt: receipt,
		finalizer: finalizer, privateStore: privateStore, store: &caseTerminalStoreStub{}, host: host, observer: observer,
	}
}

func commitConcreteRegistryEvidence(
	t *testing.T,
	registry *evidenceregistryapp.Service,
	securityContext domainsecurity.TurnSecurityContext,
	now time.Time,
	payloads ...domainevidence.NormalizedClaimPayload,
) domainevidence.EvidenceReceipt {
	t.Helper()
	_, input := evidenceIssuerFixtureForContext(t, now, securityContext)
	payload := amountClaimPayload()
	if len(payloads) > 1 {
		t.Fatal("concrete registry fixture received multiple payloads")
	}
	if len(payloads) == 1 {
		payload = payloads[0]
	}
	material := domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts:         []domainevidence.CanonicalEvidenceFact{{FactID: "fact-concrete-registry", ClaimType: domainevidence.ClaimAmount, NormalizedPayload: payload}},
	}
	canonicalBody, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	input.Material.CanonicalEvidence = canonicalBody
	input.Material.SourceType = "transactions"
	input.Material.QueryRange.EntityIDs = []string{payload.EntityID}
	input.Material.QueryRange.AccountIDs = []string{payload.AccountID}
	input.Material.QueryRange.Directions = []string{payload.Direction}
	input.Material.QueryRange.StartAt = payload.StartAt
	input.Material.QueryRange.EndAt = payload.EndAt
	input.Material.TransformationLineage[0].OutputHash = domainsecurity.CanonicalJSONHash(canonicalBody)
	canonical, err := domainevidence.CanonicalEvidenceBytes(canonicalBody)
	if err != nil {
		t.Fatal(err)
	}
	settlement := domainevidence.EvidenceSettlementProof{
		SettlementID:         domainsecurity.SHA256Hex([]byte("concrete-registry-settlement")),
		PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("concrete-registry-prepared")),
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(evidenceReceiptDraftInput(
		input, canonical, domainevidence.EvidenceSettlementReceiptID(settlement.SettlementID), now.Add(time.Minute),
	))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registry.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: draft, CanonicalEvidence: canonical,
		SettlementProof: settlement, RegisteredAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func (fixture *concreteRegistryFinalizerFixture) freshRegistryInstance(t *testing.T) *evidenceregistryapp.Service {
	t.Helper()
	stores, err := fixture.registryCAS.FreshStores()
	if err != nil {
		t.Fatal(err)
	}
	service, err := evidenceregistryapp.New(evidenceregistryapp.Config{
		InstallationID:   fixture.coordinator.bundle.InstallationID,
		EnrollmentID:     fixture.coordinator.bundle.EnrollmentID,
		Authority:        fixture.coordinator.authority,
		WitnessKeyID:     domainsecurity.SHA256Hex(fixture.coordinator.witnessPublic),
		WitnessKey:       fixture.coordinator.witnessPublic,
		Coordinator:      fixture.coordinator,
		WitnessChain:     fixture.coordinator,
		Indexes:          stores.Indexes,
		Capsules:         stores.Capsules,
		DatasetAuthority: fixture.datasetAuthority,
		BindingObserver:  fixture.observer,
		Random:           bytes.NewReader(bytes.Repeat([]byte{0x76}, 4096)),
		Now:              func() time.Time { return fixture.now.Add(7 * time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func (fixture *concreteRegistryFinalizerFixture) persist(ctx context.Context) (PersistCaseBoundaryResult, error) {
	return fixture.finalizer.PersistBoundary(ctx, PersistCaseBoundaryInput{
		Store: fixture.store, Context: fixture.securityContext, TerminalReason: TerminalSuccess,
		CaseSlotIntent: CaseSlotRequestedV1, ThreadID: fixture.securityContext.ThreadID,
		TurnID: fixture.securityContext.TurnID, AcceptedAt: fixture.now.Add(5 * time.Minute),
	})
}

func (fixture *concreteRegistryFinalizerFixture) assertBoundaryOnly(t *testing.T) {
	t.Helper()
	result, err := fixture.persist(context.Background())
	if err != nil || !result.Persistence.Changed ||
		result.Persistence.AcceptedFinal.FactFinalWitnessAdmission != nil ||
		result.Persistence.AcceptedFinal.PublicationSnapshotProofDigest != "" ||
		result.Boundary.Envelope.Variant == domainevidence.EvidenceBackedAnswer {
		t.Fatalf("hostile concrete registry finalization did not fail closed: result=%#v err=%v", result, err)
	}
}

type concreteRegistryBindingObserver struct {
	workspace   string
	observation domainsecurity.CaseBindingObservationV1
}

var _ casecontextport.Observer = (*concreteRegistryBindingObserver)(nil)

func (observer *concreteRegistryBindingObserver) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if observer == nil || workspace != observer.workspace {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("concrete registry binding is unavailable")
	}
	return observer.observation, nil
}

type concreteRegistryDatasetAuthority struct {
	coordinator *concreteRegistryCoordinator
	binding     domainsecurity.CaseBindingObservationV1
	selection   datasetsnapshotport.CurrentSelectionV2
}

func (authority *concreteRegistryDatasetAuthority) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if authority == nil || authority.coordinator == nil || ctx == nil || ctx.Err() != nil || callback == nil ||
		input.TenantID != securityContext.TenantID || input.UserID != securityContext.UserID ||
		input.ExpectedDatasetSnapshotID != securityContext.DatasetSnapshotID ||
		!reflect.DeepEqual(input.Observation, authority.binding) {
		return errors.New("concrete registry current dataset authority is unavailable")
	}
	head, err := authority.coordinator.ObserveFresh(ctx)
	if err != nil {
		return err
	}
	selection := authority.selection
	selection.Head = head
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		return err
	}
	capability := &concreteRegistryDatasetCapability{
		active: true, ctx: ctx, coordinator: authority.coordinator, selection: selection, securityContext: securityContext,
	}
	defer func() { capability.active = false }()
	return callback(selection, capability)
}

type concreteRegistryDatasetCapability struct {
	active          bool
	ctx             context.Context
	coordinator     *concreteRegistryCoordinator
	selection       datasetsnapshotport.CurrentSelectionV2
	securityContext domainsecurity.TurnSecurityContext
}

func (capability *concreteRegistryDatasetCapability) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	if capability == nil || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil ||
		capability.coordinator == nil || use == nil ||
		!reflect.DeepEqual(selection, capability.selection) ||
		!reflect.DeepEqual(securityContext, capability.securityContext) {
		return errors.New("concrete registry dataset capability changed")
	}
	before, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !before.HasBundle || before.Bundle.RecordDigest != selection.Head.Bundle.RecordDigest {
		return errors.Join(errors.New("concrete registry dataset authority changed before use"), err)
	}
	if err := use(capability.ctx); err != nil {
		return err
	}
	after, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !after.HasBundle || after.Bundle.RecordDigest != selection.Head.Bundle.RecordDigest {
		return errors.Join(errors.New("concrete registry dataset authority changed after use"), err)
	}
	return nil
}

type concreteRegistryCoordinator struct {
	mu             sync.Mutex
	bundle         domainevidence.EvidenceAuthorityBundleV1
	authority      *memoryFinalAuthority
	witnessPrivate ed25519.PrivateKey
	witnessPublic  ed25519.PublicKey
	history        map[string]evidenceauthorityport.ObservationBundle
	observeCalls   int
	advanceCalls   int
}

func (coordinator *concreteRegistryCoordinator) ObserveFresh(context.Context) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.freshHeadLocked("observe")
}

func (coordinator *concreteRegistryCoordinator) AdvanceEvidenceRegistry(
	_ context.Context,
	input evidenceauthorityport.RegistryAdvanceInput,
) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	previous := coordinator.bundle
	if input.ExpectedBundleDigest != previous.RecordDigest {
		return evidenceauthorityport.FreshHead{}, errors.New("concrete registry witness CAS conflict")
	}
	coordinator.advanceCalls++
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID,
		Generation: previous.Generation + 1, PreviousBundleDigest: previous.RecordDigest,
		MutationID:                 domainsecurity.SHA256Hex([]byte("concrete-registry-mutation:" + input.NextIndexDigest)),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: input.NextIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount + 1,
		PublicationIndexDigest: previous.PublicationIndexDigest, PublicationCount: previous.PublicationCount,
		AuthorityKeyID: coordinator.authority.keyID, AuthorityPublicKey: coordinator.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.bundle = next
	return coordinator.freshHeadLocked("advance")
}

func (coordinator *concreteRegistryCoordinator) freshHeadLocked(label string) (evidenceauthorityport.FreshHead, error) {
	coordinator.observeCalls++
	bundle := coordinator.bundle
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		Generation: bundle.Generation, CurrentStateDigest: bundle.RecordDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("concrete-registry-previous-state")),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("concrete-registry-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("concrete-registry-fence")), MutationID: bundle.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(coordinator.witnessPublic), WitnessPublicKey: coordinator.witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(coordinator.witnessPrivate, message), nil })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte(label + ":" + strconv.Itoa(coordinator.observeCalls) + ":" + bundle.RecordDigest)),
		AuthorityKeyID: coordinator.authority.keyID, AuthorityPublicKey: coordinator.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(coordinator.witnessPrivate, message), nil
	})
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	head := evidenceauthorityport.FreshHead{HasBundle: true, Bundle: bundle, Request: request, Observation: observation}
	if coordinator.history == nil {
		coordinator.history = map[string]evidenceauthorityport.ObservationBundle{}
	}
	coordinator.history[observation.ObservationDigest] = evidenceauthorityport.ObservationBundle{
		Bundle: bundle, Request: request, Observation: observation,
	}
	return head, nil
}

func (coordinator *concreteRegistryCoordinator) ResolveWitnessBindingOnFreshChain(
	_ context.Context,
	binding domainevidence.EvidenceAuthorityWitnessBindingV1,
) (evidenceauthorityport.WitnessBindingChainResolution, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if domainevidence.ValidateEvidenceAuthorityWitnessBindingV1(binding) != nil {
		return evidenceauthorityport.WitnessBindingChainResolution{}, errors.New("concrete registry witness binding is invalid")
	}
	historical, found := coordinator.history[binding.ObservationDigest]
	if !found || historical.Bundle.RecordDigest != binding.BundleRecordDigest {
		return evidenceauthorityport.WitnessBindingChainResolution{}, errors.New("concrete registry historical witness is unavailable")
	}
	current, err := coordinator.freshHeadLocked("resolve-witness-binding")
	if err != nil || !current.HasBundle || current.Bundle.RecordDigest != historical.Bundle.RecordDigest {
		return evidenceauthorityport.WitnessBindingChainResolution{}, errors.Join(
			errors.New("concrete registry witness binding is no longer current"), err,
		)
	}
	return evidenceauthorityport.WitnessBindingChainResolution{Current: current, Historical: historical}, nil
}

type concreteRegistryHostAuthority struct {
	coordinator    *concreteRegistryCoordinator
	binding        domainsecurity.CaseBindingObservationV1
	selection      datasetsnapshotport.CurrentSelectionV2
	beforeCallback func() error
	crossSnapshot  bool
	calls          int
	lastErr        error
}

func (authority *concreteRegistryHostAuthority) WithFreshPublicationSnapshotAuthority(
	ctx context.Context,
	input sourceprobeport.PublicationInput,
	callback func([]domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	authority.calls++
	if authority == nil || ctx == nil || ctx.Err() != nil || callback == nil || len(input.Requirements) != 1 ||
		!reflect.DeepEqual(input.Binding, authority.binding) {
		return errors.New("concrete registry host publication authority is unavailable")
	}
	if authority.beforeCallback != nil {
		if err := authority.beforeCallback(); err != nil {
			return err
		}
	}
	requirement := input.Requirements[0]
	identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(requirement.ServerIdentity)
	if err != nil {
		return err
	}
	snapshotID := input.Context.DatasetSnapshotID
	if authority.crossSnapshot {
		snapshotID = domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("cross-snapshot"))
	}
	checkedAt := evidenceIssuerTime().Add(5 * time.Minute)
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: requirement.ServerID, ServerIdentity: requirement.ServerIdentity,
		ConnectionEpoch:    requirement.ConnectionEpoch,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("concrete-registry-catalog")),
		SpecFingerprint:    domainsecurity.SHA256Hex([]byte("concrete-registry-spec")),
		ThreadID:           input.Context.ThreadID, TurnID: input.Context.TurnID,
		ContextEpoch: input.Context.ContextEpoch, ContextDigest: input.Context.ContextDigest,
		DatasetSnapshotID: snapshotID, CheckedAt: checkedAt,
		Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: identity.ObservedName,
			ServerVersion: identity.ObservedVersion, CaseID: input.Context.CaseID,
			CaseBindingHash: input.Context.CaseBindingHash, DatasetSnapshotID: snapshotID,
			Ready: true, ReadOnly: true, CheckedAt: checkedAt.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return err
	}
	head, err := authority.coordinator.ObserveFresh(ctx)
	if err != nil {
		return err
	}
	selection := authority.selection
	selection.Head = head
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		return err
	}
	capability := &concreteRegistryHostCapability{
		active: true, ctx: ctx, coordinator: authority.coordinator,
		securityContext: input.Context, probe: probe, selection: selection,
	}
	defer func() { capability.active = false }()
	authority.lastErr = callback([]domainsecurity.VerifiedSourceProbe{probe}, capability)
	return authority.lastErr
}

type concreteRegistryHostCapability struct {
	active          bool
	ctx             context.Context
	coordinator     *concreteRegistryCoordinator
	securityContext domainsecurity.TurnSecurityContext
	probe           domainsecurity.VerifiedSourceProbe
	selection       datasetsnapshotport.CurrentSelectionV2
}

func (capability *concreteRegistryHostCapability) DatasetSelection() (datasetsnapshotport.CurrentSelectionV2, error) {
	if capability == nil || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("concrete registry host capability is inactive")
	}
	return capability.selection, nil
}

func (capability *concreteRegistryHostCapability) UseExact(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	use func(context.Context) error,
) error {
	if capability == nil || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || use == nil ||
		!reflect.DeepEqual(securityContext, capability.securityContext) || !reflect.DeepEqual(probe, capability.probe) ||
		!reflect.DeepEqual(selection, capability.selection) {
		return errors.New("concrete registry host capability exact binding changed")
	}
	current, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !current.HasBundle || current.Bundle != selection.Head.Bundle {
		return errors.New("concrete registry host capability is stale")
	}
	return use(capability.ctx)
}
