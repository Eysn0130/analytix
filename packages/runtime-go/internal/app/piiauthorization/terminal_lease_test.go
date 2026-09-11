package piiauthorization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestControlledPIITerminalLeaseConsumesMeasuredBytesExactlyOnce(t *testing.T) {
	lease, expected, alias := terminalLeaseFixture(t, "00123456789012345678")
	disposition, err := lease.Consume(context.Background(), func(
		_ context.Context,
		reader io.Reader,
		_ ControlledPIITerminalLeaseMetadataV1,
	) error {
		observed, readErr := io.ReadAll(reader)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(observed, expected) || !strings.Contains(string(observed), `"exactValue":"00123456789012345678"`) {
			t.Fatalf("trusted publication did not receive the byte-exact leading-zero account: %s", observed)
		}
		return nil
	})
	if err != nil || disposition.Status != ControlledPIITerminalHandoffConsumedV1 ||
		disposition.ReasonCode != ControlledPIITerminalReasonHandoffCompleteV1 ||
		disposition.ObservedByteLength != uint64(len(expected)) || disposition.ArtifactByteLength != uint64(len(expected)) {
		t.Fatalf("terminal handoff did not complete exactly: disposition=%#v err=%v", disposition, err)
	}
	assertControlledBytesZeroed(t, alias)
	if _, err := lease.Consume(context.Background(), func(context.Context, io.Reader, ControlledPIITerminalLeaseMetadataV1) error {
		return nil
	}); !errors.Is(err, ErrControlledPIITerminalLeaseConsumed) {
		t.Fatalf("terminal lease was consumed twice: %v", err)
	}
}

func TestControlledPIITerminalLeaseRejectsZeroReadFalseSuccess(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	disposition, err := lease.Consume(context.Background(), func(context.Context, io.Reader, ControlledPIITerminalLeaseMetadataV1) error {
		return nil
	})
	if !errors.Is(err, ErrControlledPIITerminalLeaseIndeterminate) ||
		disposition.Status != ControlledPIITerminalHandoffIndeterminateV1 ||
		disposition.ReasonCode != ControlledPIITerminalReasonPartialReadV1 || disposition.ObservedByteLength != 0 {
		t.Fatalf("zero-read consumer self-certified success: disposition=%#v err=%v", disposition, err)
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeasePartialReadIsIndeterminateAndCannotRetry(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	disposition, err := lease.Consume(context.Background(), func(_ context.Context, reader io.Reader, _ ControlledPIITerminalLeaseMetadataV1) error {
		buffer := make([]byte, 7)
		_, readErr := io.ReadFull(reader, buffer)
		return readErr
	})
	if !errors.Is(err, ErrControlledPIITerminalLeaseIndeterminate) ||
		disposition.Status != ControlledPIITerminalHandoffIndeterminateV1 ||
		disposition.ReasonCode != ControlledPIITerminalReasonPartialReadV1 || disposition.ObservedByteLength != 7 {
		t.Fatalf("partial read was not terminally indeterminate: disposition=%#v err=%v", disposition, err)
	}
	assertControlledBytesZeroed(t, alias)
	if _, err := lease.Consume(context.Background(), func(context.Context, io.Reader, ControlledPIITerminalLeaseMetadataV1) error { return nil }); !errors.Is(err, ErrControlledPIITerminalLeaseConsumed) {
		t.Fatalf("partial handoff was retried: %v", err)
	}
}

func TestControlledPIITerminalLeaseCallbackRunsOutsideLeaseLock(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	done := make(chan error, 1)
	go func() {
		_, err := lease.Consume(context.Background(), func(_ context.Context, reader io.Reader, _ ControlledPIITerminalLeaseMetadataV1) error {
			if _, metadataErr := lease.Metadata(); metadataErr != nil {
				return metadataErr
			}
			if _, terminal := lease.Disposition(); terminal {
				return errors.New("lease became terminal before callback returned")
			}
			_, readErr := io.Copy(io.Discard, reader)
			return readErr
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal callback deadlocked by re-entering lease metadata")
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeaseCloseAbandonsAndZeros(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("terminal close was not idempotent: %v", err)
	}
	disposition, terminal := lease.Disposition()
	if !terminal || disposition.Status != ControlledPIITerminalHandoffDiscardedV1 ||
		disposition.ReasonCode != ControlledPIITerminalReasonAbandonedV1 || disposition.ObservedByteLength != 0 {
		t.Fatalf("abandoned lease disposition is invalid: %#v terminal=%v", disposition, terminal)
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeaseCancelledBeforeCallbackDiscardsWithoutRead(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	disposition, err := lease.Consume(ctx, func(context.Context, io.Reader, ControlledPIITerminalLeaseMetadataV1) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || called || disposition.Status != ControlledPIITerminalHandoffDiscardedV1 ||
		disposition.ReasonCode != ControlledPIITerminalReasonContextCancelledV1 || disposition.ObservedByteLength != 0 {
		t.Fatalf("pre-cancelled lease reached consumer: disposition=%#v called=%v err=%v", disposition, called, err)
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeaseErrorPanicAndIntegrityPathsZeroBackingBytes(t *testing.T) {
	tests := []struct {
		name       string
		mutate     bool
		consumer   ControlledPIITerminalConsumerV1
		wantStatus string
		wantReason string
		wantErr    error
	}{
		{
			name: "consumer error",
			consumer: func(context.Context, io.Reader, ControlledPIITerminalLeaseMetadataV1) error {
				return errors.New("sink failed")
			},
			wantStatus: ControlledPIITerminalHandoffIndeterminateV1,
			wantReason: ControlledPIITerminalReasonConsumerErrorV1,
			wantErr:    ErrControlledPIITerminalLeaseIndeterminate,
		},
		{
			name: "consumer panic",
			consumer: func(context.Context, io.Reader, ControlledPIITerminalLeaseMetadataV1) error {
				panic("sink panic")
			},
			wantStatus: ControlledPIITerminalHandoffIndeterminateV1,
			wantReason: ControlledPIITerminalReasonConsumerPanicV1,
			wantErr:    ErrControlledPIITerminalLeaseConsumerPanic,
		},
		{
			name:       "integrity mutation",
			mutate:     true,
			consumer:   func(context.Context, io.Reader, ControlledPIITerminalLeaseMetadataV1) error { return nil },
			wantStatus: ControlledPIITerminalHandoffDiscardedV1,
			wantReason: ControlledPIITerminalReasonIntegrityFailureV1,
			wantErr:    ErrControlledPIITerminalLeaseInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
			if test.mutate {
				alias[0] ^= 0xff
			}
			disposition, err := lease.Consume(context.Background(), test.consumer)
			if !errors.Is(err, test.wantErr) || disposition.Status != test.wantStatus || disposition.ReasonCode != test.wantReason {
				t.Fatalf("terminal path mismatch: disposition=%#v err=%v", disposition, err)
			}
			assertControlledBytesZeroed(t, alias)
		})
	}
}

func TestControlledPIITerminalLeaseCloseCancelsInFlightConsumer(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	started := make(chan struct{})
	result := make(chan struct {
		disposition ControlledPIITerminalDispositionV1
		err         error
	}, 1)
	go func() {
		disposition, err := lease.Consume(context.Background(), func(ctx context.Context, _ io.Reader, _ ControlledPIITerminalLeaseMetadataV1) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
		result <- struct {
			disposition ControlledPIITerminalDispositionV1
			err         error
		}{disposition: disposition, err: err}
	}()
	<-started
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-result:
		if !errors.Is(outcome.err, ErrControlledPIITerminalLeaseIndeterminate) ||
			outcome.disposition.Status != ControlledPIITerminalHandoffIndeterminateV1 ||
			outcome.disposition.ReasonCode != ControlledPIITerminalReasonContextCancelledV1 {
			t.Fatalf("in-flight close was not indeterminate: disposition=%#v err=%v", outcome.disposition, outcome.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close did not cancel the in-flight terminal consumer")
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeaseConcurrentConsumersHaveOneWinner(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan error, 2)
	var once sync.Once
	consumer := func(_ context.Context, reader io.Reader, _ ControlledPIITerminalLeaseMetadataV1) error {
		once.Do(func() { close(started) })
		<-release
		_, err := io.Copy(io.Discard, reader)
		return err
	}
	go func() {
		_, err := lease.Consume(context.Background(), consumer)
		results <- err
	}()
	<-started
	go func() {
		_, err := lease.Consume(context.Background(), consumer)
		results <- err
	}()
	close(release)
	first, second := <-results, <-results
	successes, unavailable := 0, 0
	for _, err := range []error{first, second} {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrControlledPIITerminalLeaseConsumed):
			unavailable++
		default:
			t.Fatalf("unexpected concurrent terminal result: %v", err)
		}
	}
	if successes != 1 || unavailable != 1 {
		t.Fatalf("terminal lease consumption was not linearizable: success=%d unavailable=%d", successes, unavailable)
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeaseCannotSerializeOrFormat(t *testing.T) {
	account := "00123456789012345678"
	lease, _, alias := terminalLeaseFixture(t, account)
	if body, err := json.Marshal(lease); err == nil || len(body) != 0 {
		t.Fatalf("terminal lease became JSON serializable: body=%s err=%v", body, err)
	}
	for name, marshal := range map[string]func() ([]byte, error){
		"text":   lease.MarshalText,
		"binary": lease.MarshalBinary,
		"gob":    lease.GobEncode,
	} {
		if body, err := marshal(); err == nil || len(body) != 0 {
			t.Fatalf("terminal lease became %s serializable: body=%x err=%v", name, body, err)
		}
	}
	formatted := fmt.Sprintf("%v|%+v|%#v|%s|%q", lease, lease, lease, lease, lease)
	parts := strings.Split(formatted, "|")
	if strings.Contains(formatted, account) || strings.Contains(formatted, "[]uint8") || len(parts) != 5 {
		t.Fatalf("terminal lease formatting exposed implementation state: %q", formatted)
	}
	for _, part := range parts {
		if part != "<controlled-pii-terminal-lease>" {
			t.Fatalf("terminal lease formatting exposed implementation state: %q", formatted)
		}
	}
	metadata, err := lease.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(metadata)
	if err != nil || strings.Contains(string(body), account) {
		t.Fatalf("lease metadata exposed raw controlled PII: %s err=%v", body, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeaseConstructorFailureZerosOwnedBackingBytes(t *testing.T) {
	body := []byte(`{"exactValue":"00123456789012345678"}`)
	owned := append([]byte(nil), body...)
	alias := owned[:]
	metadata := terminalLeaseMetadataFixture(body)
	metadata.ArtifactSHA256 = domainsecurity.SHA256Hex([]byte("different"))
	lease, err := newControlledPIITerminalLeaseV1(metadata, owned)
	if !errors.Is(err, ErrControlledPIITerminalLeaseInvalid) || lease != nil {
		t.Fatalf("invalid lease constructor succeeded: lease=%v err=%v", lease, err)
	}
	assertControlledBytesZeroed(t, alias)
}

func TestControlledPIITerminalLeaseReaderCannotEscapeCallback(t *testing.T) {
	lease, _, alias := terminalLeaseFixture(t, "00123456789012345678")
	var escaped io.Reader
	if _, err := lease.Consume(context.Background(), func(_ context.Context, reader io.Reader, _ ControlledPIITerminalLeaseMetadataV1) error {
		escaped = reader
		_, readErr := io.Copy(io.Discard, reader)
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if _, err := escaped.Read(buffer); !errors.Is(err, ErrControlledPIITerminalReaderClosed) {
		t.Fatalf("terminal reader remained usable after callback: %v", err)
	}
	assertControlledBytesZeroed(t, alias)
}

func terminalLeaseFixture(t *testing.T, account string) (*ControlledPIITerminalLeaseV1, []byte, []byte) {
	t.Helper()
	expected := []byte(`{"schemaVersion":1,"exactValue":"` + account + `"}`)
	owned := append([]byte(nil), expected...)
	alias := owned[:]
	lease, err := newControlledPIITerminalLeaseV1(terminalLeaseMetadataFixture(expected), owned)
	if err != nil {
		t.Fatal(err)
	}
	return lease, expected, alias
}

func terminalLeaseMetadataFixture(body []byte) ControlledPIITerminalLeaseMetadataV1 {
	metadata := ControlledPIITerminalLeaseMetadataV1{
		ContextDigest: domainsecurity.SHA256Hex([]byte("terminal-context")), ContextEpoch: 7,
		DatasetSnapshotID:    securitycontexttest.DatasetSnapshotID("terminal-lease"),
		ClaimLedgerDigest:    domainsecurity.SHA256Hex([]byte("terminal-ledger")),
		TargetIdentityDigest: domainsecurity.SHA256Hex([]byte("terminal-target")),
		ArtifactSHA256:       domainsecurity.SHA256Hex(body), ArtifactByteLength: uint64(len(body)),
		MediaType:              domainpii.ControlledPIIArtifactMediaTypeV1,
		PIIAuthorizationDigest: domainsecurity.SHA256Hex([]byte("terminal-grant")),
		PIIProjectionDigest:    domainsecurity.SHA256Hex([]byte("terminal-projection")),
	}
	metadata.ArtifactMetadata = domainpii.ControlledPIIArtifactMetadataV1{
		SHA256: metadata.ArtifactSHA256, ByteLength: metadata.ArtifactByteLength, MediaType: metadata.MediaType,
		ClaimLedgerDigest: metadata.ClaimLedgerDigest, TargetIdentityDigest: metadata.TargetIdentityDigest,
	}
	return metadata
}

func assertControlledBytesZeroed(t *testing.T, body []byte) {
	t.Helper()
	for index, value := range body {
		if value != 0 {
			t.Fatalf("controlled backing byte %d was not zeroed: %d", index, value)
		}
	}
}
