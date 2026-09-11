package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

func TestControlledAccessTrustedInventoryClosesOpenCrashCutAsIndeterminate(t *testing.T) {
	fixture := newControlledAccessInventoryFixtureV1(t)
	plan, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies())
	if err != nil || plan.OpenReceiptCount() != 1 {
		t.Fatalf("trusted open access was not planned: open=%d err=%v", plan.OpenReceiptCount(), err)
	}
	disposedAt := fixture.requestedAt.Add(time.Minute)
	if err := ApplyControlledAccessRestartPlanV1(
		context.Background(), plan, fixture.access, fixture.authority, disposedAt,
	); err != nil {
		t.Fatal(err)
	}
	disposition, err := fixture.access.ResolveAccessDisposition(context.Background(), fixture.accessReceipt.AccessID)
	if err != nil || disposition.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
		disposition.ReasonCode != domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1 ||
		disposition.ReleasedByteLength != fixture.accessReceipt.ArtifactByteLength ||
		disposition.DisposedAt != disposedAt.Format(time.RFC3339Nano) {
		t.Fatalf("restart did not preserve an indeterminate release: disposition=%#v err=%v", disposition, err)
	}
	closed, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies())
	if err != nil || closed.OpenReceiptCount() != 0 {
		t.Fatalf("terminalized access did not pass full re-verification: open=%d err=%v", closed.OpenReceiptCount(), err)
	}
}

func TestControlledAccessTrustedInventoryRejectsUntrustedAuthorityAndArtifactMismatch(t *testing.T) {
	t.Run("access receipt signed by another installation", func(t *testing.T) {
		fixture := newControlledAccessInventoryFixtureV1(t)
		otherKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x79}, ed25519.SeedSize))
		otherPublic := otherKey.Public().(ed25519.PublicKey)
		input := fixture.accessInput
		input.AuthorityKeyID = domainsecurity.SHA256Hex(otherPublic)
		input.AuthorityPublicKey = otherPublic
		untrusted, err := domainpii.NewControlledArtifactAccessReceiptV1(
			input, func(message []byte) ([]byte, error) { return ed25519.Sign(otherKey, message), nil },
		)
		if err != nil {
			t.Fatal(err)
		}
		fixture.access.receipts = map[string]domainpii.ControlledArtifactAccessReceiptV1{untrusted.AccessID: untrusted}
		if _, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies()); err == nil {
			t.Fatal("self-signed controlled access receipt passed installation trust")
		}
	})

	t.Run("protected artifact metadata changed", func(t *testing.T) {
		fixture := newControlledAccessInventoryFixtureV1(t)
		fixture.artifacts.metadata.SHA256 = domainsecurity.SHA256Hex([]byte("different-controlled-bytes"))
		if _, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies()); err == nil {
			t.Fatal("controlled access receipt accepted different artifact bytes")
		}
	})
}

func TestControlledAccessRestartPlanRejectsClockRollbackAndStaleClosure(t *testing.T) {
	t.Run("clock before reservation", func(t *testing.T) {
		fixture := newControlledAccessInventoryFixtureV1(t)
		plan, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies())
		if err != nil {
			t.Fatal(err)
		}
		if err := ApplyControlledAccessRestartPlanV1(
			context.Background(), plan, fixture.access, fixture.authority, fixture.requestedAt.Add(-time.Nanosecond),
		); err == nil {
			t.Fatal("restart terminalization accepted a clock before the reservation")
		}
	})

	t.Run("another terminal already exists", func(t *testing.T) {
		fixture := newControlledAccessInventoryFixtureV1(t)
		plan, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies())
		if err != nil {
			t.Fatal(err)
		}
		disposition, err := domainpii.NewControlledArtifactAccessDispositionV1(
			fixture.accessReceipt,
			domainpii.ControlledArtifactAccessDispositionRejectedV1,
			domainpii.ControlledArtifactAccessReasonAccessRejectedV1,
			0,
			fixture.requestedAt.Add(time.Minute),
			fixture.authority.KeyID(),
			fixture.authority.PublicKey(),
			func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
		)
		if err != nil {
			t.Fatal(err)
		}
		fixture.access.dispositions[disposition.AccessID] = disposition
		if err := ApplyControlledAccessRestartPlanV1(
			context.Background(), plan, fixture.access, fixture.authority, fixture.requestedAt.Add(2*time.Minute),
		); err == nil {
			t.Fatal("stale restart plan overwrote an existing terminal")
		}
	})
}

func TestControlledAccessRestartPlanRejectsOpenReceiptWritesOutsideSemanticStage(t *testing.T) {
	fixture := newControlledAccessInventoryFixtureV1(t)
	fixture.access.semanticStage = false
	plan, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies())
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyControlledAccessRestartPlanV1(
		context.Background(),
		plan,
		fixture.access,
		fixture.authority,
		fixture.requestedAt.Add(time.Minute),
	); err == nil {
		t.Fatal("live controlled access store terminalized an open restart receipt")
	}
	if len(fixture.access.dispositions) != 0 {
		t.Fatalf("live controlled access store changed during rejected recovery: %d", len(fixture.access.dispositions))
	}
}

func TestControlledAccessRestartPlanPreflightsEveryReceiptBeforeFirstWrite(t *testing.T) {
	fixture := newControlledAccessInventoryFixtureV1(t)
	var second domainpii.ControlledArtifactAccessReceiptV1
	for candidate := 0; candidate < 4096; candidate++ {
		input := fixture.accessInput
		input.UseSlotDigest = domainsecurity.SHA256Hex([]byte(
			"controlled-access-second-use-slot-" + time.Unix(int64(candidate), 0).UTC().Format(time.RFC3339Nano),
		))
		receipt, err := domainpii.NewControlledArtifactAccessReceiptV1(
			input,
			func(message []byte) ([]byte, error) {
				return fixture.authority.Sign(context.Background(), message)
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.AccessID > fixture.accessReceipt.AccessID {
			second = receipt
			break
		}
	}
	if second.AccessID == "" {
		t.Fatal("could not construct a second receipt ordered after the first")
	}
	fixture.access.receipts[second.AccessID] = second
	plan, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies())
	if err != nil || plan.OpenReceiptCount() != 2 {
		t.Fatalf("two trusted open accesses were not planned: open=%d err=%v", plan.OpenReceiptCount(), err)
	}
	secondTerminal, err := domainpii.NewControlledArtifactAccessDispositionV1(
		second,
		domainpii.ControlledArtifactAccessDispositionRejectedV1,
		domainpii.ControlledArtifactAccessReasonAccessRejectedV1,
		0,
		fixture.requestedAt.Add(time.Minute),
		fixture.authority.KeyID(),
		fixture.authority.PublicKey(),
		func(message []byte) ([]byte, error) {
			return fixture.authority.Sign(context.Background(), message)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.access.dispositions[second.AccessID] = secondTerminal

	if err := ApplyControlledAccessRestartPlanV1(
		context.Background(),
		plan,
		fixture.access,
		fixture.authority,
		fixture.requestedAt.Add(2*time.Minute),
	); err == nil {
		t.Fatal("restart plan accepted a stale second receipt")
	}
	if _, err := fixture.access.ResolveAccessDisposition(
		context.Background(),
		fixture.accessReceipt.AccessID,
	); !errors.Is(err, piiauthorizationport.ErrNotFound) {
		t.Fatalf("restart plan wrote the first disposition before preflighting the second: %v", err)
	}
	if len(fixture.access.dispositions) != 1 {
		t.Fatalf("restart plan mutated access dispositions during failed preflight: %d", len(fixture.access.dispositions))
	}
}

func TestControlledAccessRestartSecondWriteFailureProducesZeroLiveMutation(t *testing.T) {
	fixture := newControlledAccessInventoryFixtureV1(t)
	second := controlledAccessSecondReceiptAfterFirstV1(t, fixture)
	fixture.access.receipts[second.AccessID] = second
	live := fixture.access
	stage := &memoryControlledAccessStoreV1{
		receipts:      cloneControlledAccessReceiptsV1(live.receipts),
		dispositions:  cloneControlledAccessDispositionsV1(live.dispositions),
		semanticStage: true,
		failPutAt:     2,
	}
	fixture.access = stage
	plan, err := VerifyControlledAccessInventoryV1(context.Background(), fixture.dependencies())
	if err != nil || plan.OpenReceiptCount() != 2 {
		t.Fatalf("two staged open accesses were not planned: open=%d err=%v", plan.OpenReceiptCount(), err)
	}
	if err := ApplyControlledAccessRestartPlanV1(
		context.Background(),
		plan,
		stage,
		fixture.authority,
		fixture.requestedAt.Add(2*time.Minute),
	); err == nil {
		t.Fatal("staged restart did not surface the second disposition write failure")
	}
	if len(stage.dispositions) != 1 {
		t.Fatalf("fault seam did not cut after one staged disposition: %d", len(stage.dispositions))
	}
	if len(live.dispositions) != 0 {
		t.Fatalf("failed staged recovery changed the live access journal: %d", len(live.dispositions))
	}
}

type controlledAccessInventoryFixtureV1 struct {
	authority       *testAuthority
	access          *memoryControlledAccessStoreV1
	grants          *memoryGrantStore
	receipts        *memoryControlledPublicationReceiptStoreV1
	commits         *memoryControlledPublicationCommitStoreV1
	indexes         *memoryControlledPublicationIndexStoreV1
	ledgers         *memoryLedgerStore
	projections     *memoryControlledProjectionStoreV1
	inspections     *memoryControlledInspectionStoreV1
	artifacts       *memoryControlledArtifactMetadataStoreV1
	accessReceipt   domainpii.ControlledArtifactAccessReceiptV1
	accessInput     domainpii.ControlledArtifactAccessReceiptInputV1
	artifactBytes   []byte
	requestedAt     time.Time
	authorizedUntil time.Time
}

func newControlledAccessInventoryFixtureV1(t *testing.T) *controlledAccessInventoryFixtureV1 {
	t.Helper()
	service := newServiceFixture(t)
	prepared, err := service.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(service))
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := service.service.AuthorizePreparedControlledArtifact(
		context.Background(),
		AuthorizePreparedControlledArtifactInputV1{
			SecurityContext:      service.context,
			ClaimLedger:          service.input.ClaimLedger,
			Prepared:             prepared,
			ApprovalID:           service.input.ApprovalID,
			ApprovalRecordDigest: service.input.ApprovalRecordDigest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	artifactBytes, disposition, err := consumeAuthorizedControlledArtifactForTest(context.Background(), authorized)
	if err != nil {
		t.Fatal(err)
	}
	if disposition.Status != ControlledPIITerminalHandoffConsumedV1 {
		t.Fatalf("controlled artifact test sink did not commit: %#v", disposition)
	}
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer:         "analytix-host",
		RendererVersion:  "1.0.0",
		ReportSHA256:     authorized.ProtectedSHA256,
		ReportByteLength: authorized.ProtectedByteLength,
		MediaType:        domainpii.ControlledPIIArtifactMediaTypeV1,
		Passed:           true,
		IssueCodes:       []string{},
		InspectedAt:      service.clock.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	installationID := domainsecurity.SHA256Hex([]byte("controlled-access-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("controlled-access-enrollment"))
	previousBundle := controlledAccessPreviousBundleV1(t, service.authority, installationID, enrollmentID)
	receipt, err := domainpublication.NewPublicationReceiptV1(domainpublication.PublicationReceiptInputV1{
		InstallationID:                installationID,
		EnrollmentID:                  enrollmentID,
		Context:                       service.context,
		ReportVariant:                 domainpublication.EvidenceBackedReport,
		EvidenceAuthorityBundleDigest: previousBundle.RecordDigest,
		EvidenceRegistryIndexDigest:   previousBundle.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:         previousBundle.EvidenceRegistryCount,
		EvidenceRegistrySequence:      1,
		EvidenceRegistryStateDigest:   domainsecurity.SHA256Hex([]byte("controlled-access-registry-state")),
		ClaimLedger:                   service.input.ClaimLedger,
		ReportSHA256:                  authorized.ProtectedSHA256,
		ReportByteLength:              authorized.ProtectedByteLength,
		MediaType:                     domainpii.ControlledPIIArtifactMediaTypeV1,
		PIIProjection:                 authorized.PIIProjection,
		RenderInspection:              inspection,
		Publisher:                     "analytix-host",
		PublisherVersion:              "1.0.0",
		TargetIdentityDigest:          authorized.Grant.TargetIdentityDigest,
		IssuedAt:                      service.clock.Add(2 * time.Minute),
		AuthorityKeyID:                service.authority.KeyID(),
		AuthorityPublicKey:            service.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainpublication.NewPublicationIndexV1(domainpublication.PublicationIndexInputV1{
		InstallationID:       installationID,
		EnrollmentID:         enrollmentID,
		Generation:           1,
		PreviousIndexDigest:  domainpublication.PublicationIndexGenesisDigestV1(),
		MutationID:           domainsecurity.SHA256Hex([]byte("controlled-access-publication-mutation")),
		ReceiptID:            receipt.ReceiptID,
		ReceiptRecordDigest:  receipt.RecordDigest,
		TargetIdentityDigest: receipt.TargetIdentityDigest,
		AuthorityKeyID:       service.authority.KeyID(),
		AuthorityPublicKey:   service.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	committedBundle, request, observation, witnessPublic := controlledAccessWitnessFixtureV1(
		t, service.authority, installationID, enrollmentID, previousBundle, index,
	)
	if err := domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previousBundle, committedBundle); err != nil {
		t.Fatalf("controlled access publication bundle transition: %v", err)
	}
	commit, err := domainpublication.NewPublicationCommitReceiptV1(domainpublication.PublicationCommitReceiptInputV1{
		PreviousBundle: previousBundle, CommittedBundle: committedBundle,
		ObserveRequest: request, Observation: observation, Candidate: receipt, Index: index,
		InstallationID: installationID, EnrollmentID: enrollmentID,
		AuthorityKeyID: service.authority.KeyID(), AuthorityPublicKey: service.authority.PublicKey(),
		WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	requestedAt := service.clock.Add(3 * time.Minute)
	authorizedUntil, err := time.Parse(time.RFC3339Nano, authorized.Grant.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	accessInput := domainpii.ControlledArtifactAccessReceiptInputV1{
		SecurityContext:          service.context,
		AccessAction:             domainpii.ControlledArtifactAccessActionExportV1,
		ControlledHandleDigest:   domainsecurity.SHA256Hex([]byte("controlled-access-handle")),
		UseSlotDigest:            domainsecurity.SHA256Hex([]byte("controlled-access-use-slot")),
		RendererPrincipalDigest:  domainsecurity.SHA256Hex([]byte("controlled-access-renderer")),
		RendererGeneration:       2,
		BackendGeneration:        4,
		AccessPolicyDigest:       authorized.Grant.AccessPolicyDigest,
		RetentionPolicyDigest:    authorized.Grant.RetentionPolicyDigest,
		PublicationCommitDigest:  commit.RecordDigest,
		PublicationReceiptDigest: receipt.RecordDigest,
		PIIProjectionDigest:      authorized.PIIProjection.ProjectionDigest,
		PIIAuthorizationDigest:   authorized.Grant.RecordDigest,
		ClaimLedgerDigest:        service.input.ClaimLedger.LedgerDigest,
		TargetIdentityDigest:     authorized.Grant.TargetIdentityDigest,
		ArtifactSHA256:           authorized.ProtectedSHA256,
		ArtifactByteLength:       authorized.ProtectedByteLength,
		MediaType:                domainpii.ControlledPIIArtifactMediaTypeV1,
		RequestedAt:              requestedAt,
		AuthorizedUntil:          authorizedUntil,
		AuthorityKeyID:           service.authority.KeyID(),
		AuthorityPublicKey:       service.authority.PublicKey(),
	}
	accessReceipt, err := domainpii.NewControlledArtifactAccessReceiptV1(
		accessInput, func(message []byte) ([]byte, error) { return service.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	access := &memoryControlledAccessStoreV1{
		receipts:      map[string]domainpii.ControlledArtifactAccessReceiptV1{accessReceipt.AccessID: accessReceipt},
		dispositions:  map[string]domainpii.ControlledArtifactAccessDispositionV1{},
		semanticStage: true,
	}
	return &controlledAccessInventoryFixtureV1{
		authority: service.authority, access: access, grants: service.grants,
		receipts:    &memoryControlledPublicationReceiptStoreV1{record: receipt},
		commits:     &memoryControlledPublicationCommitStoreV1{record: commit},
		indexes:     &memoryControlledPublicationIndexStoreV1{record: index},
		ledgers:     service.ledgers,
		projections: &memoryControlledProjectionStoreV1{record: authorized.PIIProjection},
		inspections: &memoryControlledInspectionStoreV1{record: inspection},
		artifacts: &memoryControlledArtifactMetadataStoreV1{
			metadata: authorized.Metadata, body: append([]byte(nil), artifactBytes...),
		},
		accessReceipt: accessReceipt, accessInput: accessInput, requestedAt: requestedAt, authorizedUntil: authorizedUntil,
		artifactBytes: append([]byte(nil), artifactBytes...),
	}
}

func (fixture *controlledAccessInventoryFixtureV1) dependencies() ControlledAccessInventoryDependenciesV1 {
	return ControlledAccessInventoryDependenciesV1{
		Access: fixture.access, Grants: fixture.grants, Receipts: fixture.receipts, Commits: fixture.commits,
		Indexes: fixture.indexes, Ledgers: fixture.ledgers, Projections: fixture.projections,
		Inspections: fixture.inspections, Artifacts: fixture.artifacts, Authority: fixture.authority,
	}
}

func controlledAccessWitnessFixtureV1(
	t *testing.T,
	authority *testAuthority,
	installationID string,
	enrollmentID string,
	previous domainevidence.EvidenceAuthorityBundleV1,
	index domainpublication.PublicationIndexV1,
) (
	domainevidence.EvidenceAuthorityBundleV1,
	domainsecurity.MonotonicHeadObserveRequestV1,
	domainsecurity.MonotonicHeadObservationV1,
	ed25519.PublicKey,
) {
	t.Helper()
	sign := func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) }
	committed, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 2,
		PreviousBundleDigest:       previous.RecordDigest,
		MutationID:                 domainsecurity.SHA256Hex([]byte("controlled-access-committed-bundle")),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount,
		PublicationIndexDigest: index.IndexDigest, PublicationCount: 1,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x68}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1, Generation: committed.Generation,
		CurrentStateDigest: committed.RecordDigest, PreviousStateDigest: previous.RecordDigest,
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("controlled-access-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("controlled-access-fence")), MutationID: committed.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace:      domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte("controlled-access-observe-challenge")),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		request, checkpoint,
		func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return committed, request, observation, witnessPublic
}

func controlledAccessPreviousBundleV1(
	t *testing.T,
	authority *testAuthority,
	installationID string,
	enrollmentID string,
) domainevidence.EvidenceAuthorityBundleV1 {
	t.Helper()
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                 domainsecurity.SHA256Hex([]byte("controlled-access-previous-bundle")),
		DatasetSnapshotIndexDigest: domainsecurity.SHA256Hex([]byte("controlled-access-dataset-index")), DatasetSnapshotCount: 1,
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte("controlled-access-evidence-index")), EvidenceRegistryCount: 1,
		PublicationIndexDigest: domainpublication.PublicationIndexGenesisDigestV1(), PublicationCount: 0,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

type memoryControlledAccessStoreV1 struct {
	receipts      map[string]domainpii.ControlledArtifactAccessReceiptV1
	dispositions  map[string]domainpii.ControlledArtifactAccessDispositionV1
	semanticStage bool
	putCalls      int
	failPutAt     int
	afterReserve  func()
}

func (store *memoryControlledAccessStoreV1) IsSemanticStageControlledAccessStore() bool {
	return store != nil && store.semanticStage
}

func (store *memoryControlledAccessStoreV1) ReserveAccessReceipt(
	_ context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV1,
) error {
	if current, exists := store.receipts[receipt.AccessID]; exists {
		if reflect.DeepEqual(current, receipt) {
			return piiauthorizationport.ErrAlreadyReserved
		}
		return piiauthorizationport.ErrConflict
	}
	store.receipts[receipt.AccessID] = receipt
	if store.afterReserve != nil {
		store.afterReserve()
	}
	return nil
}

func (store *memoryControlledAccessStoreV1) ResolveAccessReceipt(
	_ context.Context,
	accessID string,
) (domainpii.ControlledArtifactAccessReceiptV1, error) {
	receipt, found := store.receipts[accessID]
	if !found {
		return domainpii.ControlledArtifactAccessReceiptV1{}, piiauthorizationport.ErrNotFound
	}
	return receipt, nil
}

func (store *memoryControlledAccessStoreV1) PutAccessDispositionIfAbsent(
	_ context.Context,
	disposition domainpii.ControlledArtifactAccessDispositionV1,
) error {
	store.putCalls++
	if store.failPutAt > 0 && store.putCalls == store.failPutAt {
		return errors.New("controlled access disposition write fault")
	}
	receipt, found := store.receipts[disposition.AccessID]
	if !found || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt) != nil {
		return piiauthorizationport.ErrCorrupt
	}
	if current, exists := store.dispositions[disposition.AccessID]; exists {
		if reflect.DeepEqual(current, disposition) {
			return nil
		}
		return piiauthorizationport.ErrConflict
	}
	store.dispositions[disposition.AccessID] = disposition
	return nil
}

func (store *memoryControlledAccessStoreV1) ResolveAccessDisposition(
	_ context.Context,
	accessID string,
) (domainpii.ControlledArtifactAccessDispositionV1, error) {
	disposition, found := store.dispositions[accessID]
	if !found {
		return domainpii.ControlledArtifactAccessDispositionV1{}, piiauthorizationport.ErrNotFound
	}
	return disposition, nil
}

func (store *memoryControlledAccessStoreV1) VisitAccessReceipts(
	_ context.Context,
	visit func(domainpii.ControlledArtifactAccessReceiptV1) error,
) error {
	keys := make([]string, 0, len(store.receipts))
	for key := range store.receipts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := visit(store.receipts[key]); err != nil {
			return err
		}
	}
	return nil
}

func controlledAccessSecondReceiptAfterFirstV1(
	t *testing.T,
	fixture *controlledAccessInventoryFixtureV1,
) domainpii.ControlledArtifactAccessReceiptV1 {
	t.Helper()
	for candidate := 0; candidate < 4096; candidate++ {
		input := fixture.accessInput
		input.UseSlotDigest = domainsecurity.SHA256Hex([]byte(
			"controlled-access-second-use-slot-" + time.Unix(int64(candidate), 0).UTC().Format(time.RFC3339Nano),
		))
		receipt, err := domainpii.NewControlledArtifactAccessReceiptV1(
			input,
			func(message []byte) ([]byte, error) {
				return fixture.authority.Sign(context.Background(), message)
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.AccessID > fixture.accessReceipt.AccessID {
			return receipt
		}
	}
	t.Fatal("could not construct a second receipt ordered after the first")
	return domainpii.ControlledArtifactAccessReceiptV1{}
}

func cloneControlledAccessReceiptsV1(
	source map[string]domainpii.ControlledArtifactAccessReceiptV1,
) map[string]domainpii.ControlledArtifactAccessReceiptV1 {
	cloned := make(map[string]domainpii.ControlledArtifactAccessReceiptV1, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneControlledAccessDispositionsV1(
	source map[string]domainpii.ControlledArtifactAccessDispositionV1,
) map[string]domainpii.ControlledArtifactAccessDispositionV1 {
	cloned := make(map[string]domainpii.ControlledArtifactAccessDispositionV1, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (store *memoryControlledAccessStoreV1) VisitAccessDispositions(
	_ context.Context,
	visit func(domainpii.ControlledArtifactAccessDispositionV1) error,
) error {
	keys := make([]string, 0, len(store.dispositions))
	for key := range store.dispositions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := visit(store.dispositions[key]); err != nil {
			return err
		}
	}
	return nil
}

type memoryControlledPublicationReceiptStoreV1 struct {
	record domainpublication.PublicationReceiptV1
}

func (*memoryControlledPublicationReceiptStoreV1) PutIfAbsent(context.Context, domainpublication.PublicationReceiptV1) error {
	return errors.New("not implemented")
}
func (store *memoryControlledPublicationReceiptStoreV1) Resolve(
	_ context.Context,
	digest string,
) (domainpublication.PublicationReceiptV1, error) {
	if digest != store.record.RecordDigest {
		return domainpublication.PublicationReceiptV1{}, errors.New("receipt missing")
	}
	return store.record, nil
}

type memoryControlledPublicationCommitStoreV1 struct {
	record domainpublication.PublicationCommitReceiptV1
}

func (*memoryControlledPublicationCommitStoreV1) PutIfAbsent(context.Context, domainpublication.PublicationCommitReceiptV1) error {
	return errors.New("not implemented")
}
func (store *memoryControlledPublicationCommitStoreV1) Resolve(
	_ context.Context,
	digest string,
) (domainpublication.PublicationCommitReceiptV1, error) {
	if digest != store.record.RecordDigest {
		return domainpublication.PublicationCommitReceiptV1{}, errors.New("commit missing")
	}
	return store.record, nil
}

type memoryControlledPublicationIndexStoreV1 struct {
	record domainpublication.PublicationIndexV1
}

func (*memoryControlledPublicationIndexStoreV1) PutIfAbsent(context.Context, domainpublication.PublicationIndexV1) error {
	return errors.New("not implemented")
}
func (store *memoryControlledPublicationIndexStoreV1) Resolve(
	_ context.Context,
	digest string,
) (domainpublication.PublicationIndexV1, error) {
	if digest != store.record.IndexDigest {
		return domainpublication.PublicationIndexV1{}, errors.New("index missing")
	}
	return store.record, nil
}

type memoryControlledProjectionStoreV1 struct {
	record domainpublication.PIIProjectionV1
}

func (*memoryControlledProjectionStoreV1) PutIfAbsent(context.Context, domainpublication.PIIProjectionV1) error {
	return errors.New("not implemented")
}
func (store *memoryControlledProjectionStoreV1) Resolve(
	_ context.Context,
	digest string,
) (domainpublication.PIIProjectionV1, error) {
	if digest != store.record.ProjectionDigest {
		return domainpublication.PIIProjectionV1{}, errors.New("projection missing")
	}
	return store.record, nil
}

type memoryControlledInspectionStoreV1 struct {
	record domainpublication.RenderInspectionV1
}

func (*memoryControlledInspectionStoreV1) PutIfAbsent(context.Context, domainpublication.RenderInspectionV1) error {
	return errors.New("not implemented")
}
func (store *memoryControlledInspectionStoreV1) Resolve(
	_ context.Context,
	digest string,
) (domainpublication.RenderInspectionV1, error) {
	if digest != store.record.InspectionDigest {
		return domainpublication.RenderInspectionV1{}, errors.New("inspection missing")
	}
	return store.record, nil
}

type memoryControlledArtifactMetadataStoreV1 struct {
	metadata domainpii.ControlledPIIArtifactMetadataV1
	body     []byte
}

func (store *memoryControlledArtifactMetadataStoreV1) InstallNoReplace(
	_ context.Context,
	_ string,
	body []byte,
) error {
	if store.body != nil && !bytes.Equal(store.body, body) {
		return errors.New("controlled artifact conflict")
	}
	store.body = append([]byte(nil), body...)
	return nil
}

func (store *memoryControlledArtifactMetadataStoreV1) ResolveExact(
	_ context.Context,
	_ string,
) ([]byte, error) {
	if len(store.body) == 0 {
		return nil, errors.New("controlled artifact missing")
	}
	return append([]byte(nil), store.body...), nil
}

func (store *memoryControlledArtifactMetadataStoreV1) ResolveControlledMetadata(
	context.Context,
	string,
) (domainpii.ControlledPIIArtifactMetadataV1, error) {
	return store.metadata, nil
}
