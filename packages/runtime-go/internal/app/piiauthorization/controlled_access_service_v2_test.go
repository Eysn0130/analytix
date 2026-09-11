package piiauthorization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	domainartifactdelivery "analytix.local/runtime-go/internal/domain/artifactdelivery"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	artifactdeliveryport "analytix.local/runtime-go/internal/ports/artifactdelivery"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

func TestControlledAccessServiceV2ReleasesExactAccountOnlyThroughLinearizedProjectedWinner(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domainpii.ControlledArtifactAccessDispositionHostReleaseCommittedV1 ||
		result.DeliveryID != fixture.delivery.DeliveryID ||
		result.DeliveryOutcomeRecordDigest != fixture.delivery.OutcomeRecordDigest ||
		result.ReleasedByteLength != uint64(len(fixture.base.artifactBytes)) ||
		fixture.sink.calls != 1 || !bytes.Equal(fixture.sink.body, fixture.base.artifactBytes) ||
		!bytes.Contains(fixture.sink.body, []byte(`"exactValue":"`+serviceTestAccountExact+`"`)) {
		t.Fatalf("V2 trusted sink did not receive the exact account once: result=%#v sink=%#v", result, fixture.sink)
	}
	if fixture.deliveryAuthority.calls != 3 || fixture.deliveryAuthority.linearizedCalls != 1 || fixture.admissions.calls != 4 ||
		fixture.effectCalls != 1 {
		t.Fatalf("V2 release did not cross all authority boundaries: projected=%d linearized=%d admissions=%d effects=%d",
			fixture.deliveryAuthority.calls, fixture.deliveryAuthority.linearizedCalls, fixture.admissions.calls, fixture.effectCalls)
	}
	receipt, receiptErr := fixture.access.ResolveAccessReceiptV2(context.Background(), result.AccessID)
	disposition, dispositionErr := fixture.access.ResolveAccessDispositionV2(context.Background(), result.AccessID)
	if receiptErr != nil || dispositionErr != nil || receipt.DeliveryID != fixture.delivery.DeliveryID ||
		receipt.DeliveryOutcomeRecordDigest != fixture.delivery.OutcomeRecordDigest ||
		receipt.ReleaseTargetIdentityDigest != fixture.admissions.admission.ReleaseTargetIdentityDigest ||
		disposition.ReleaseTargetIdentityDigest != receipt.ReleaseTargetIdentityDigest ||
		fixture.sink.receipt.ReleaseTargetIdentityDigest != receipt.ReleaseTargetIdentityDigest ||
		domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
		t.Fatalf("V2 controlled access audit chain is incomplete: receipt=%#v disposition=%#v errors=%v/%v",
			receipt, disposition, receiptErr, dispositionErr)
	}
	metadata, err := json.Marshal(struct {
		Result      ControlledAccessResultV2
		Receipt     domainpii.ControlledArtifactAccessReceiptV2
		Disposition domainpii.ControlledArtifactAccessDispositionV2
	}{result, receipt, disposition})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{
		[]byte(serviceTestAccountExact), []byte(fixture.input.ControlledHandle),
		[]byte(fixture.input.UseSlot), []byte(fixture.input.RendererPrincipal),
	} {
		if bytes.Contains(metadata, forbidden) {
			t.Fatalf("V2 audit metadata leaked a raw or bearer value %q: %s", forbidden, metadata)
		}
	}
}

func TestControlledAccessServiceV2RejectsWinnerBeforeReservation(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	fixture.deliveryAuthority.failAt = 1
	fixture.deliveryAuthority.failErr = artifactdeliveryport.ErrRejected
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessRejected) || result.AccessID != "" ||
		fixture.sink.calls != 0 || len(fixture.access.receipts) != 0 || len(fixture.access.dispositions) != 0 {
		t.Fatalf("rejected delivery reached V2 reservation: result=%#v sink=%d receipts=%d dispositions=%d err=%v",
			result, fixture.sink.calls, len(fixture.access.receipts), len(fixture.access.dispositions), err)
	}
}

func TestControlledAccessServiceV2RejectsPrivateArtifactIdentityAsReleaseTarget(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	fixture.admissions.admission.ReleaseTargetIdentityDigest = fixture.delivery.TargetIdentityDigest
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessRejected) || result.AccessID != "" ||
		fixture.sink.calls != 0 || len(fixture.access.receipts) != 0 || len(fixture.access.dispositions) != 0 {
		t.Fatalf("private artifact identity was accepted as the trusted-sink target: result=%#v sink=%d receipts=%d dispositions=%d err=%v",
			result, fixture.sink.calls, len(fixture.access.receipts), len(fixture.access.dispositions), err)
	}
}

func TestControlledAccessServiceV2ClosesPreSinkRevocationWithZeroBytes(t *testing.T) {
	tests := []struct {
		name       string
		failure    error
		wantStatus string
		wantReason string
		wantError  error
	}{
		{"rejected", artifactdeliveryport.ErrRejected,
			domainpii.ControlledArtifactAccessDispositionRejectedV1,
			domainpii.ControlledArtifactAccessReasonAccessRejectedV1, ErrControlledAccessRejected},
		{"stale", artifactdeliveryport.ErrStale,
			domainpii.ControlledArtifactAccessDispositionStaleContextV1,
			domainpii.ControlledArtifactAccessReasonStaleContextV1, ErrControlledAccessStale},
		{"unavailable", artifactdeliveryport.ErrUnavailable,
			domainpii.ControlledArtifactAccessDispositionFailedV1,
			domainpii.ControlledArtifactAccessReasonAuthorityUnavailableV2, ErrControlledAccessUnavailable},
		{"integrity", artifactdeliveryport.ErrIntegrity,
			domainpii.ControlledArtifactAccessDispositionFailedV1,
			domainpii.ControlledArtifactAccessReasonAuthorityIntegrityV2, ErrControlledAccessIntegrity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveControlledAccessFixtureV2(t)
			fixture.deliveryAuthority.failAt = 2
			fixture.deliveryAuthority.failErr = test.failure
			result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
			if !errors.Is(err, test.wantError) || result.Status != test.wantStatus ||
				result.ReleasedByteLength != 0 || fixture.sink.calls != 0 || len(fixture.access.dispositions) != 1 {
				t.Fatalf("pre-sink authority failure escaped terminalization: result=%#v sink=%d err=%v",
					result, fixture.sink.calls, err)
			}
			disposition, resolveErr := fixture.access.ResolveAccessDispositionV2(context.Background(), result.AccessID)
			if resolveErr != nil || disposition.ReasonCode != test.wantReason || disposition.ReleasedByteLength != 0 {
				t.Fatalf("pre-sink failure terminal is inaccurate: disposition=%#v err=%v", disposition, resolveErr)
			}
		})
	}
}

func TestControlledAccessServiceV2PostSinkRevocationIsFullLengthIndeterminate(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	fixture.deliveryAuthority.failAt = 3
	fixture.deliveryAuthority.failErr = artifactdeliveryport.ErrIntegrity
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessIndeterminate) || fixture.sink.calls != 1 ||
		result.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
		result.ReleasedByteLength != uint64(len(fixture.base.artifactBytes)) {
		t.Fatalf("post-sink revocation was reported as committed: result=%#v sink=%d err=%v", result, fixture.sink.calls, err)
	}
}

func TestControlledAccessServiceV2PartialOrFailedSinkIsFullLengthIndeterminate(t *testing.T) {
	tests := []struct {
		name       string
		readBytes  int
		committed  bool
		releaseErr error
	}{
		{"partial", 13, false, nil},
		{"error after exact read", -1, false, errors.New("V2 host commit failed")},
		{"false commit with partial read", 7, true, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveControlledAccessFixtureV2(t)
			fixture.sink.readBytes = test.readBytes
			fixture.sink.committed = test.committed
			fixture.sink.err = test.releaseErr
			result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
			if !errors.Is(err, ErrControlledAccessIndeterminate) || fixture.sink.calls != 1 ||
				result.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
				result.ReleasedByteLength != uint64(len(fixture.base.artifactBytes)) {
				t.Fatalf("ambiguous V2 sink outcome was understated: result=%#v sink=%#v err=%v", result, fixture.sink, err)
			}
		})
	}
}

func TestControlledAccessServiceV2UseSlotCannotRebindToReplacementOutcome(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	if _, err := fixture.service.ReleaseV2(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	fixture.delivery.OutcomeRecordDigest = domainsecurity.SHA256Hex([]byte("replacement projected outcome record"))
	fixture.deliveryAuthority.delivery = fixture.delivery
	fixture.admissions.admission.DeliveryOutcomeRecordDigest = fixture.delivery.OutcomeRecordDigest
	second, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessDuplicate) || second.AccessID != "" || fixture.sink.calls != 1 ||
		len(fixture.access.receipts) != 1 || len(fixture.access.dispositions) != 1 {
		t.Fatalf("replacement outcome rebound an already-used V2 slot: result=%#v sink=%d receipts=%d dispositions=%d err=%v",
			second, fixture.sink.calls, len(fixture.access.receipts), len(fixture.access.dispositions), err)
	}
}

func TestControlledAccessServiceV2UseSlotCannotRebindToAnotherReleaseTarget(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	if _, err := fixture.service.ReleaseV2(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	fixture.admissions.admission.ReleaseTargetIdentityDigest = domainsecurity.SHA256Hex([]byte("replacement trusted sink target"))
	second, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessDuplicate) || second.AccessID != "" || fixture.sink.calls != 1 ||
		len(fixture.access.receipts) != 1 || len(fixture.access.dispositions) != 1 {
		t.Fatalf("replacement trusted sink rebound an already-used V2 slot: result=%#v sink=%d receipts=%d dispositions=%d err=%v",
			second, fixture.sink.calls, len(fixture.access.receipts), len(fixture.access.dispositions), err)
	}
}

func TestControlledAccessServiceV2RejectsClockRollbackBeforeReservation(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	fixture.nowSequence = []time.Time{
		fixture.now,
		fixture.now.Add(-time.Second),
	}
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessStale) || result.AccessID != "" ||
		fixture.sink.calls != 0 || len(fixture.access.receipts) != 0 ||
		len(fixture.access.dispositions) != 0 {
		t.Fatalf("clock rollback retained release authority: result=%#v sink=%d receipts=%d dispositions=%d err=%v",
			result, fixture.sink.calls, len(fixture.access.receipts), len(fixture.access.dispositions), err)
	}
}

func TestControlledAccessServiceV2ClockRollbackAfterSinkIsDurablyIndeterminate(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV2(t)
	fixture.nowSequence = []time.Time{
		fixture.now,
		fixture.now,
		fixture.now,
		fixture.now.Add(-time.Second),
	}
	result, err := fixture.service.ReleaseV2(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessIndeterminate) || fixture.sink.calls != 1 ||
		result.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
		result.ReleasedByteLength != uint64(len(fixture.base.artifactBytes)) {
		t.Fatalf("post-sink clock rollback was understated: result=%#v sink=%d err=%v",
			result, fixture.sink.calls, err)
	}
	receipt, receiptErr := fixture.access.ResolveAccessReceiptV2(context.Background(), result.AccessID)
	disposition, dispositionErr := fixture.access.ResolveAccessDispositionV2(context.Background(), result.AccessID)
	if receiptErr != nil || dispositionErr != nil ||
		disposition.DisposedAt != receipt.RequestedAt ||
		domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
		t.Fatalf("clock rollback did not retain a conservative terminal audit: receipt=%#v disposition=%#v errors=%v/%v",
			receipt, disposition, receiptErr, dispositionErr)
	}
}

type controlledAccessLiveFixtureV2 struct {
	base              *controlledAccessInventoryFixtureV1
	service           *ControlledAccessServiceV2
	input             ControlledAccessInputV2
	delivery          domainartifactdelivery.VerifiedArtifactDeliveryV1
	access            *memoryControlledAccessStoreV2
	admissions        *controlledAccessAdmissionStubV2
	deliveryAuthority *controlledArtifactDeliveryStubV2
	sink              *controlledReleaseSinkStubV2
	now               time.Time
	nowSequence       []time.Time
	nowCalls          int
	effectCalls       int
	currentCalls      int
	currentFailAt     int
}

type controlledAccessEffectContextKeyV2 struct{}

func newLiveControlledAccessFixtureV2(t *testing.T) *controlledAccessLiveFixtureV2 {
	t.Helper()
	base := newControlledAccessInventoryFixtureV1(t)
	input := ControlledAccessInputV2{
		SecurityContext: base.accessInput.SecurityContext, AccessAction: domainpii.ControlledArtifactAccessActionExportV1,
		ControlledHandle: "host-controlled-publication-handle-v2", UseSlot: "host-controlled-single-use-slot-v2",
		RendererPrincipal: "main-frame-renderer-principal-v2", RendererGeneration: 3, BackendGeneration: 5,
	}
	handleDigest, err := domainpii.ControlledAccessHandleDigestV2(input.ControlledHandle)
	if err != nil {
		t.Fatal(err)
	}
	useSlotDigest, err := domainpii.ControlledAccessUseSlotDigestV2(input.UseSlot)
	if err != nil {
		t.Fatal(err)
	}
	principalDigest, err := domainpii.ControlledAccessRendererPrincipalDigestV2(input.RendererPrincipal)
	if err != nil {
		t.Fatal(err)
	}
	publicationReceipt := base.receipts.record
	contextBinding, err := domainartifactdelivery.ContextBindingFromTurnSecurityContextV1(input.SecurityContext)
	if err != nil {
		t.Fatal(err)
	}
	delivery := domainartifactdelivery.VerifiedArtifactDeliveryV1{
		Context:                  contextBinding,
		DeliveryID:               domainsecurity.SHA256Hex([]byte("controlled V2 delivery slot")),
		OutcomeRecordDigest:      domainsecurity.SHA256Hex([]byte("controlled V2 delivery outcome")),
		PublicationCommitDigest:  base.commits.record.RecordDigest,
		PublicationReceiptDigest: publicationReceipt.RecordDigest,
		ClaimLedgerDigest:        publicationReceipt.ClaimLedgerDigest,
		ContentProjectionDigest:  publicationReceipt.PIIProjectionDigest,
		AuthorizationAuditDigest: publicationReceipt.AuthorizationAuditDigest,
		ArtifactSHA256:           publicationReceipt.ReportSHA256, ArtifactByteLength: publicationReceipt.ReportByteLength,
		MediaType: publicationReceipt.MediaType, TargetIdentityDigest: publicationReceipt.TargetIdentityDigest,
		ExposureClass:       domainartifactdelivery.ExposureClassRestrictedExactV1,
		PublicationIssuedAt: publicationReceipt.IssuedAt,
	}
	if err := domainartifactdelivery.ValidateVerifiedArtifactDeliveryV1(delivery); err != nil {
		t.Fatalf("build controlled artifact delivery fixture: %v", err)
	}
	admission := piiauthorizationport.ControlledAccessAdmissionV2{
		SecurityContext: input.SecurityContext, AccessAction: input.AccessAction,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest, RendererPrincipalDigest: principalDigest,
		RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
		DeliveryID: delivery.DeliveryID, DeliveryOutcomeRecordDigest: delivery.OutcomeRecordDigest,
		PublicationCommitDigest:     delivery.PublicationCommitDigest,
		ReleaseTargetIdentityDigest: domainsecurity.SHA256Hex([]byte("controlled V2 trusted sink target")),
		AuthorizedUntil:             base.authorizedUntil,
	}
	fixture := &controlledAccessLiveFixtureV2{
		base: base, input: input, delivery: delivery, now: base.requestedAt,
		access: &memoryControlledAccessStoreV2{
			receipts:     map[string]domainpii.ControlledArtifactAccessReceiptV2{},
			dispositions: map[string]domainpii.ControlledArtifactAccessDispositionV2{},
		},
		admissions: &controlledAccessAdmissionStubV2{
			expected: piiauthorizationport.ControlledAccessAdmissionRequestV2{
				SecurityContext: input.SecurityContext, AccessAction: input.AccessAction,
				ControlledHandle: input.ControlledHandle, UseSlot: input.UseSlot, RendererPrincipal: input.RendererPrincipal,
				RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
			},
			admission: admission,
		},
		deliveryAuthority: &controlledArtifactDeliveryStubV2{delivery: delivery},
		sink:              &controlledReleaseSinkStubV2{readBytes: -1, committed: true},
	}
	service, err := NewControlledAccessServiceV2(ControlledAccessConfigV2{
		Authority: base.authority, Access: fixture.access, Admissions: fixture.admissions, Sink: fixture.sink,
		Delivery: fixture.deliveryAuthority, Grants: base.grants,
		Artifacts: base.artifacts,
		ValidateCurrent: func(ctx context.Context, current domainsecurity.TurnSecurityContext) error {
			fixture.currentCalls++
			if current != fixture.input.SecurityContext ||
				(fixture.currentFailAt > 0 && fixture.currentCalls >= fixture.currentFailAt) {
				return errors.New("current V2 case context changed")
			}
			return nil
		},
		AcquireEffect: func(ctx context.Context, current domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			if current != fixture.input.SecurityContext {
				return nil, nil, errors.New("V2 effect context changed")
			}
			fixture.effectCalls++
			effectCtx := context.WithValue(ctx, controlledAccessEffectContextKeyV2{}, true)
			fixture.deliveryAuthority.requireEffect = true
			return effectCtx, func() {}, nil
		},
		Now: func() time.Time {
			if fixture.nowCalls < len(fixture.nowSequence) {
				current := fixture.nowSequence[fixture.nowCalls]
				fixture.nowCalls++
				return current
			}
			fixture.nowCalls++
			return fixture.now
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.service = service
	return fixture
}

type memoryControlledAccessStoreV2 struct {
	receipts      map[string]domainpii.ControlledArtifactAccessReceiptV2
	dispositions  map[string]domainpii.ControlledArtifactAccessDispositionV2
	semanticStage bool
	afterReserve  func()
}

func (store *memoryControlledAccessStoreV2) ReserveAccessReceiptV2(
	_ context.Context,
	receipt domainpii.ControlledArtifactAccessReceiptV2,
) error {
	if current, found := store.receipts[receipt.AccessID]; found {
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

func (store *memoryControlledAccessStoreV2) ResolveAccessReceiptV2(
	_ context.Context,
	accessID string,
) (domainpii.ControlledArtifactAccessReceiptV2, error) {
	receipt, found := store.receipts[accessID]
	if !found {
		return domainpii.ControlledArtifactAccessReceiptV2{}, piiauthorizationport.ErrNotFound
	}
	return receipt, nil
}

func (store *memoryControlledAccessStoreV2) PutAccessDispositionIfAbsentV2(
	_ context.Context,
	disposition domainpii.ControlledArtifactAccessDispositionV2,
) error {
	receipt, found := store.receipts[disposition.AccessID]
	if !found || domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil {
		return piiauthorizationport.ErrCorrupt
	}
	if current, found := store.dispositions[disposition.AccessID]; found {
		if reflect.DeepEqual(current, disposition) {
			return nil
		}
		return piiauthorizationport.ErrConflict
	}
	store.dispositions[disposition.AccessID] = disposition
	return nil
}

func (store *memoryControlledAccessStoreV2) ResolveAccessDispositionV2(
	_ context.Context,
	accessID string,
) (domainpii.ControlledArtifactAccessDispositionV2, error) {
	disposition, found := store.dispositions[accessID]
	if !found {
		return domainpii.ControlledArtifactAccessDispositionV2{}, piiauthorizationport.ErrNotFound
	}
	return disposition, nil
}

type controlledAccessAdmissionStubV2 struct {
	expected  piiauthorizationport.ControlledAccessAdmissionRequestV2
	admission piiauthorizationport.ControlledAccessAdmissionV2
	calls     int
}

func (stub *controlledAccessAdmissionStubV2) ResolveCurrentV2(
	_ context.Context,
	request piiauthorizationport.ControlledAccessAdmissionRequestV2,
) (piiauthorizationport.ControlledAccessAdmissionV2, error) {
	stub.calls++
	if !reflect.DeepEqual(request, stub.expected) {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, errors.New("opaque V2 desktop access authority mismatch")
	}
	return stub.admission, nil
}

type controlledArtifactDeliveryStubV2 struct {
	delivery        domainartifactdelivery.VerifiedArtifactDeliveryV1
	calls           int
	linearizedCalls int
	failAt          int
	failErr         error
	requireEffect   bool
}

func (stub *controlledArtifactDeliveryStubV2) ResolveCurrent(
	ctx context.Context,
	selector artifactdeliveryport.SelectorV1,
) (domainartifactdelivery.VerifiedArtifactDeliveryV1, error) {
	stub.calls++
	if stub.requireEffect && ctx.Value(controlledAccessEffectContextKeyV2{}) != true {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, artifactdeliveryport.ErrStale
	}
	if err := stub.validateSelector(selector); err != nil {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, err
	}
	if stub.failAt > 0 && stub.calls >= stub.failAt {
		return domainartifactdelivery.VerifiedArtifactDeliveryV1{}, stub.failErr
	}
	return stub.delivery, nil
}

func (stub *controlledArtifactDeliveryStubV2) WithCurrent(
	ctx context.Context,
	selector artifactdeliveryport.SelectorV1,
	use func(domainartifactdelivery.VerifiedArtifactDeliveryV1) error,
) error {
	stub.calls++
	stub.linearizedCalls++
	if stub.requireEffect && ctx.Value(controlledAccessEffectContextKeyV2{}) != true {
		return artifactdeliveryport.ErrStale
	}
	if err := stub.validateSelector(selector); err != nil {
		return err
	}
	if stub.failAt > 0 && stub.calls >= stub.failAt {
		return stub.failErr
	}
	if use == nil {
		return artifactdeliveryport.ErrInvalid
	}
	return use(stub.delivery)
}

func (stub *controlledArtifactDeliveryStubV2) validateSelector(
	selector artifactdeliveryport.SelectorV1,
) error {
	if selector.DeliveryID != stub.delivery.DeliveryID || selector.OutcomeRecordDigest != stub.delivery.OutcomeRecordDigest ||
		selector.PublicationCommitDigest != stub.delivery.PublicationCommitDigest ||
		selector.SecurityContext.ContextDigest != stub.delivery.Context.ContextDigest {
		return artifactdeliveryport.ErrIntegrity
	}
	return nil
}

type controlledReleaseSinkStubV2 struct {
	calls     int
	readBytes int
	committed bool
	err       error
	body      []byte
	receipt   domainpii.ControlledArtifactAccessReceiptV2
}

func (stub *controlledReleaseSinkStubV2) ReleaseV2(
	_ context.Context,
	request piiauthorizationport.ControlledReleaseRequestV2,
) (piiauthorizationport.ControlledReleaseResultV2, error) {
	stub.calls++
	stub.receipt = request.Receipt
	reader := request.Body
	if stub.readBytes >= 0 {
		reader = io.LimitReader(reader, int64(stub.readBytes))
	}
	body, readErr := io.ReadAll(reader)
	stub.body = append([]byte(nil), body...)
	if readErr != nil {
		return piiauthorizationport.ControlledReleaseResultV2{ReleasedByteLength: uint64(len(body))}, readErr
	}
	return piiauthorizationport.ControlledReleaseResultV2{
		Committed: stub.committed, ReleasedByteLength: uint64(len(body)),
	}, stub.err
}
