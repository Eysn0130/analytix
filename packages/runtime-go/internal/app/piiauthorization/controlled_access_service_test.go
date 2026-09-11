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

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

func TestControlledAccessServiceReleasesExactBankAccountOnlyThroughTrustedSink(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV1(t)
	result, err := fixture.service.Release(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domainpii.ControlledArtifactAccessDispositionHostReleaseCommittedV1 ||
		result.ReleasedByteLength != uint64(len(fixture.base.artifactBytes)) || fixture.sink.calls != 1 ||
		!bytes.Equal(fixture.sink.body, fixture.base.artifactBytes) ||
		!bytes.Contains(fixture.sink.body, []byte(`"exactValue":"`+serviceTestAccountExact+`"`)) {
		t.Fatalf("trusted sink did not receive the exact controlled account once: result=%#v sink=%#v", result, fixture.sink)
	}
	receipt, receiptErr := fixture.base.access.ResolveAccessReceipt(context.Background(), result.AccessID)
	disposition, dispositionErr := fixture.base.access.ResolveAccessDisposition(context.Background(), result.AccessID)
	if receiptErr != nil || dispositionErr != nil || receipt.RecordDigest != result.AccessReceiptDigest ||
		disposition.RecordDigest != result.DispositionDigest ||
		domainpii.ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt) != nil {
		t.Fatalf("controlled access audit chain is incomplete: receipt=%#v disposition=%#v errors=%v/%v", receipt, disposition, receiptErr, dispositionErr)
	}
	metadata, err := json.Marshal(struct {
		Result      ControlledAccessResultV1
		Receipt     domainpii.ControlledArtifactAccessReceiptV1
		Disposition domainpii.ControlledArtifactAccessDispositionV1
	}{result, receipt, disposition})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{
		[]byte(serviceTestAccountExact), []byte(fixture.input.ControlledHandle),
		[]byte(fixture.input.UseSlot), []byte(fixture.input.RendererPrincipal),
	} {
		if bytes.Contains(metadata, forbidden) {
			t.Fatalf("controlled access metadata leaked an ephemeral/raw value %q: %s", forbidden, metadata)
		}
	}
	if fixture.admissions.calls != 4 || fixture.pii.calls != 3 || fixture.effectCalls != 1 {
		t.Fatalf("release was not revalidated at every authority boundary: admissions=%d pii=%d effects=%d",
			fixture.admissions.calls, fixture.pii.calls, fixture.effectCalls)
	}
}

func TestControlledAccessServiceUseSlotCannotReleaseTwice(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV1(t)
	first, err := fixture.service.Release(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.service.Release(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessDuplicate) || second.AccessID != "" || fixture.sink.calls != 1 ||
		len(fixture.base.access.receipts) != 1 || len(fixture.base.access.dispositions) != 1 {
		t.Fatalf("one-use slot released twice: first=%#v second=%#v sink=%d receipts=%d dispositions=%d err=%v",
			first, second, fixture.sink.calls, len(fixture.base.access.receipts), len(fixture.base.access.dispositions), err)
	}
}

func TestControlledAccessServiceRejectsFakeHandlePrincipalGenerationAndCommitBeforeReservation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*controlledAccessLiveFixtureV1)
	}{
		{"fake handle", func(f *controlledAccessLiveFixtureV1) { f.input.ControlledHandle = "fake-controlled-handle-token-v1" }},
		{"fake principal", func(f *controlledAccessLiveFixtureV1) { f.input.RendererPrincipal = "fake-renderer-principal-token-v1" }},
		{"stale renderer generation", func(f *controlledAccessLiveFixtureV1) { f.input.RendererGeneration++ }},
		{"stale backend generation", func(f *controlledAccessLiveFixtureV1) { f.input.BackendGeneration++ }},
		{"missing publication commit", func(f *controlledAccessLiveFixtureV1) {
			f.admissions.admission.PublicationCommitDigest = domainsecurity.SHA256Hex([]byte("missing-controlled-publication-commit"))
		}},
		{"missing publication receipt", func(f *controlledAccessLiveFixtureV1) {
			f.base.receipts.record = domainpublication.PublicationReceiptV1{}
		}},
		{"changed protected artifact", func(f *controlledAccessLiveFixtureV1) {
			f.base.artifacts.body = append([]byte(nil), f.base.artifacts.body...)
			f.base.artifacts.body[len(f.base.artifacts.body)-2] ^= 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveControlledAccessFixtureV1(t)
			test.mutate(fixture)
			result, err := fixture.service.Release(context.Background(), fixture.input)
			if err == nil || result.AccessID != "" || fixture.sink.calls != 0 || len(fixture.base.access.receipts) != 0 {
				t.Fatalf("invalid desktop/publication authority reached reservation: result=%#v sink=%d receipts=%d err=%v",
					result, fixture.sink.calls, len(fixture.base.access.receipts), err)
			}
		})
	}
}

func TestControlledAccessServiceClosesPostReservationRevocationWithoutReleasingBytes(t *testing.T) {
	t.Run("renderer principal changes", func(t *testing.T) {
		fixture := newLiveControlledAccessFixtureV1(t)
		fixture.admissions.mutateAt = 3
		result, err := fixture.service.Release(context.Background(), fixture.input)
		if !errors.Is(err, ErrControlledAccessStale) ||
			result.Status != domainpii.ControlledArtifactAccessDispositionStaleContextV1 ||
			result.ReleasedByteLength != 0 || fixture.sink.calls != 0 {
			t.Fatalf("stale principal released controlled bytes: result=%#v sink=%d err=%v", result, fixture.sink.calls, err)
		}
	})

	t.Run("PII approval is revoked", func(t *testing.T) {
		fixture := newLiveControlledAccessFixtureV1(t)
		fixture.pii.failAt = 2
		result, err := fixture.service.Release(context.Background(), fixture.input)
		if !errors.Is(err, ErrControlledAccessRejected) ||
			result.Status != domainpii.ControlledArtifactAccessDispositionRejectedV1 ||
			result.ReleasedByteLength != 0 || fixture.sink.calls != 0 {
			t.Fatalf("revoked PII grant released controlled bytes: result=%#v sink=%d err=%v", result, fixture.sink.calls, err)
		}
	})

	t.Run("case context changes", func(t *testing.T) {
		fixture := newLiveControlledAccessFixtureV1(t)
		fixture.currentFailAt = 3
		result, err := fixture.service.Release(context.Background(), fixture.input)
		if !errors.Is(err, ErrControlledAccessStale) ||
			result.Status != domainpii.ControlledArtifactAccessDispositionStaleContextV1 ||
			result.ReleasedByteLength != 0 || fixture.sink.calls != 0 {
			t.Fatalf("stale case context released controlled bytes: result=%#v sink=%d err=%v", result, fixture.sink.calls, err)
		}
	})
}

func TestControlledAccessServiceCancellationAfterReservationClosesZeroByteAttempt(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV1(t)
	fixture.base.access.afterReserve = func() {
		if fixture.effectCancel != nil {
			fixture.effectCancel()
		}
	}
	result, err := fixture.service.Release(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessCancelled) ||
		result.Status != domainpii.ControlledArtifactAccessDispositionCancelledV1 ||
		result.ReleasedByteLength != 0 || fixture.sink.calls != 0 || len(fixture.base.access.dispositions) != 1 {
		t.Fatalf("post-reservation cancellation left an open or released access: result=%#v sink=%d dispositions=%d err=%v",
			result, fixture.sink.calls, len(fixture.base.access.dispositions), err)
	}
}

func TestControlledAccessServicePartialOrFailedSinkIsConservativelyIndeterminate(t *testing.T) {
	tests := []struct {
		name       string
		readBytes  int
		committed  bool
		releaseErr error
	}{
		{"partial", 17, false, nil},
		{"failed after exact read", -1, false, errors.New("trusted sink commit failed")},
		{"claims commit with partial read", 9, true, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveControlledAccessFixtureV1(t)
			fixture.sink.readBytes = test.readBytes
			fixture.sink.committed = test.committed
			fixture.sink.err = test.releaseErr
			result, err := fixture.service.Release(context.Background(), fixture.input)
			if !errors.Is(err, ErrControlledAccessIndeterminate) ||
				result.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
				result.ReleasedByteLength != uint64(len(fixture.base.artifactBytes)) || fixture.sink.calls != 1 {
				t.Fatalf("ambiguous sink outcome was understated: result=%#v sink=%#v err=%v", result, fixture.sink, err)
			}
			disposition, resolveErr := fixture.base.access.ResolveAccessDisposition(context.Background(), result.AccessID)
			if resolveErr != nil || disposition.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
				disposition.ReleasedByteLength != disposition.ArtifactByteLength {
				t.Fatalf("indeterminate release was not durably conservative: disposition=%#v err=%v", disposition, resolveErr)
			}
		})
	}
}

func TestControlledAccessServicePostSinkRevocationIsIndeterminateNotCommitted(t *testing.T) {
	fixture := newLiveControlledAccessFixtureV1(t)
	fixture.pii.failAt = 3
	result, err := fixture.service.Release(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledAccessIndeterminate) || fixture.sink.calls != 1 ||
		result.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
		result.ReleasedByteLength != uint64(len(fixture.base.artifactBytes)) {
		t.Fatalf("post-sink PII revocation was reported as a safe commit: result=%#v sink=%d err=%v", result, fixture.sink.calls, err)
	}
}

type controlledAccessLiveFixtureV1 struct {
	base          *controlledAccessInventoryFixtureV1
	service       *ControlledAccessServiceV1
	input         ControlledAccessInputV1
	admissions    *controlledAccessAdmissionStubV1
	sink          *controlledReleaseSinkStubV1
	pii           *controlledPIIAuthorityStubV1
	now           time.Time
	effectCalls   int
	effectCancel  context.CancelFunc
	currentCalls  int
	currentFailAt int
}

func newLiveControlledAccessFixtureV1(t *testing.T) *controlledAccessLiveFixtureV1 {
	t.Helper()
	base := newControlledAccessInventoryFixtureV1(t)
	base.access.receipts = map[string]domainpii.ControlledArtifactAccessReceiptV1{}
	base.access.dispositions = map[string]domainpii.ControlledArtifactAccessDispositionV1{}
	base.access.semanticStage = false
	input := ControlledAccessInputV1{
		SecurityContext: base.accessInput.SecurityContext, AccessAction: domainpii.ControlledArtifactAccessActionExportV1,
		ControlledHandle: "host-controlled-publication-handle-v1", UseSlot: "host-controlled-single-use-slot-v1",
		RendererPrincipal: "main-frame-renderer-principal-v1", RendererGeneration: 2, BackendGeneration: 4,
	}
	handleDigest, err := domainpii.ControlledAccessHandleDigestV1(input.ControlledHandle)
	if err != nil {
		t.Fatal(err)
	}
	useSlotDigest, err := domainpii.ControlledAccessUseSlotDigestV1(input.UseSlot)
	if err != nil {
		t.Fatal(err)
	}
	principalDigest, err := domainpii.ControlledAccessRendererPrincipalDigestV1(input.RendererPrincipal)
	if err != nil {
		t.Fatal(err)
	}
	admission := piiauthorizationport.ControlledAccessAdmissionV1{
		SecurityContext: input.SecurityContext, AccessAction: input.AccessAction,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest, RendererPrincipalDigest: principalDigest,
		RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
		PublicationCommitDigest: base.commits.record.RecordDigest, AuthorizedUntil: base.authorizedUntil,
	}
	fixture := &controlledAccessLiveFixtureV1{
		base: base, input: input, now: base.requestedAt,
		admissions: &controlledAccessAdmissionStubV1{expected: piiauthorizationport.ControlledAccessAdmissionRequestV1{
			SecurityContext: input.SecurityContext, AccessAction: input.AccessAction,
			ControlledHandle: input.ControlledHandle, UseSlot: input.UseSlot, RendererPrincipal: input.RendererPrincipal,
			RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
		}, admission: admission},
		sink: &controlledReleaseSinkStubV1{readBytes: -1, committed: true},
		pii:  &controlledPIIAuthorityStubV1{},
	}
	service, err := NewControlledAccessServiceV1(ControlledAccessConfigV1{
		Authority: base.authority, Access: base.access, Admissions: fixture.admissions, Sink: fixture.sink,
		Grants: base.grants, Receipts: base.receipts, Commits: base.commits, Indexes: base.indexes,
		Ledgers: base.ledgers, Projections: base.projections, Inspections: base.inspections,
		Artifacts: base.artifacts, ArtifactMetadata: base.artifacts, PIIAuthority: fixture.pii,
		ValidateCurrent: func(_ context.Context, current domainsecurity.TurnSecurityContext) error {
			fixture.currentCalls++
			if current != fixture.input.SecurityContext ||
				(fixture.currentFailAt > 0 && fixture.currentCalls >= fixture.currentFailAt) {
				return errors.New("current case context changed")
			}
			return nil
		},
		AcquireEffect: func(ctx context.Context, current domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			if current != fixture.input.SecurityContext {
				return nil, nil, errors.New("effect context changed")
			}
			fixture.effectCalls++
			effectCtx, cancel := context.WithCancel(ctx)
			fixture.effectCancel = cancel
			return effectCtx, func() { cancel() }, nil
		},
		Now: func() time.Time { return fixture.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.service = service
	return fixture
}

type controlledAccessAdmissionStubV1 struct {
	expected  piiauthorizationport.ControlledAccessAdmissionRequestV1
	admission piiauthorizationport.ControlledAccessAdmissionV1
	calls     int
	mutateAt  int
}

func (stub *controlledAccessAdmissionStubV1) ResolveCurrent(
	_ context.Context,
	request piiauthorizationport.ControlledAccessAdmissionRequestV1,
) (piiauthorizationport.ControlledAccessAdmissionV1, error) {
	stub.calls++
	if !reflect.DeepEqual(request, stub.expected) {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, errors.New("opaque desktop access authority mismatch")
	}
	admission := stub.admission
	if stub.mutateAt > 0 && stub.calls >= stub.mutateAt {
		admission.BackendGeneration++
	}
	return admission, nil
}

type controlledReleaseSinkStubV1 struct {
	calls     int
	readBytes int
	committed bool
	err       error
	body      []byte
}

func (stub *controlledReleaseSinkStubV1) Release(
	_ context.Context,
	request piiauthorizationport.ControlledReleaseRequestV1,
) (piiauthorizationport.ControlledReleaseResultV1, error) {
	stub.calls++
	reader := request.Body
	if stub.readBytes >= 0 {
		reader = io.LimitReader(reader, int64(stub.readBytes))
	}
	body, readErr := io.ReadAll(reader)
	stub.body = append([]byte(nil), body...)
	if readErr != nil {
		return piiauthorizationport.ControlledReleaseResultV1{ReleasedByteLength: uint64(len(body))}, readErr
	}
	return piiauthorizationport.ControlledReleaseResultV1{
		Committed: stub.committed, ReleasedByteLength: uint64(len(body)),
	}, stub.err
}

type controlledPIIAuthorityStubV1 struct {
	calls  int
	failAt int
}

func (stub *controlledPIIAuthorityStubV1) ValidateCurrent(
	_ context.Context,
	_ domainsecurity.TurnSecurityContext,
	_ domainpublication.PIIProjectionV1,
	_ string,
) error {
	stub.calls++
	if stub.failAt > 0 && stub.calls >= stub.failAt {
		return errors.New("controlled PII approval revoked")
	}
	return nil
}
