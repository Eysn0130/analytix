package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const desktopControlledHostTestAccountV2 = "0006222020202020202020"

type desktopControlledHostFixtureV2 struct {
	secret     string
	secretBody []byte
	handle     string
	useSlot    string
	principal  string
	context    domainsecurity.TurnSecurityContext
	receipt    domainpii.ControlledArtifactAccessReceiptV2
	body       []byte
	requested  time.Time
	expires    time.Time
}

func TestDesktopControlledHostClientV2ProbesExactQuiescentGeneration(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV2(t)
	var calls atomic.Int32
	server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != desktopControlledHostProbePathV2 || request.Method != http.MethodPost ||
			request.Header.Get("Authorization") != "Bearer "+fixture.secret ||
			request.Header.Get("Accept") != desktopControlledHostJSONMediaTypeV2 ||
			request.Header.Get("Content-Type") != desktopControlledHostJSONMediaTypeV2 || request.ContentLength <= 0 {
			http.Error(w, "bad probe", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(request.Body)
		var probe desktopControlledHostProbeRequestV2
		if err != nil || int64(len(body)) != request.ContentLength || json.Unmarshal(body, &probe) != nil ||
			probe.SchemaVersion != desktopControlledHostProtocolVersionV2 ||
			probe.Purpose != desktopControlledHostProbeRequestPurposeV2 ||
			probe.BackendGeneration != fixture.receipt.BackendGeneration {
			http.Error(w, "bad probe", http.StatusBadRequest)
			return
		}
		canonical, _ := json.Marshal(probe)
		nonce, nonceErr := base64.RawURLEncoding.Strict().DecodeString(probe.ProbeNonce)
		defer clearDesktopControlledHostBytesV1(nonce)
		if nonceErr != nil || len(nonce) != 32 || !bytes.Equal(body, canonical) {
			http.Error(w, "bad probe", http.StatusBadRequest)
			return
		}
		writeCanonicalDesktopControlledHostJSONV2(t, w, desktopControlledHostProbeResponseV2{
			SchemaVersion: desktopControlledHostProtocolVersionV2,
			Purpose:       desktopControlledHostProbeResponsePurposeV2,
			ProbeNonce:    probe.ProbeNonce, BackendGeneration: probe.BackendGeneration,
			TransportReady: true, InvocationsActive: false,
		})
	}))
	defer server.Close()
	client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.ProbeV2(context.Background()); err != nil || calls.Load() != 1 {
		t.Fatalf("exact V2 host probe failed: calls=%d err=%v", calls.Load(), err)
	}
}

func TestDesktopControlledHostClientV2RejectsUnreadyProbeResponse(t *testing.T) {
	for name, mutate := range map[string]func(*desktopControlledHostProbeResponseV2){
		"active":     func(value *desktopControlledHostProbeResponseV2) { value.InvocationsActive = true },
		"not-ready":  func(value *desktopControlledHostProbeResponseV2) { value.TransportReady = false },
		"generation": func(value *desktopControlledHostProbeResponseV2) { value.BackendGeneration++ },
		"nonce": func(value *desktopControlledHostProbeResponseV2) {
			value.ProbeNonce = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDesktopControlledHostFixtureV2(t)
			server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				body, _ := io.ReadAll(request.Body)
				var probe desktopControlledHostProbeRequestV2
				_ = json.Unmarshal(body, &probe)
				response := desktopControlledHostProbeResponseV2{
					SchemaVersion: desktopControlledHostProtocolVersionV2,
					Purpose:       desktopControlledHostProbeResponsePurposeV2,
					ProbeNonce:    probe.ProbeNonce, BackendGeneration: probe.BackendGeneration,
					TransportReady: true, InvocationsActive: false,
				}
				mutate(&response)
				writeCanonicalDesktopControlledHostJSONV2(t, w, response)
			}))
			defer server.Close()
			client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.ProbeV2(context.Background()); !errors.Is(err, ErrDesktopControlledHostProtocol) {
				t.Fatalf("unready probe response was accepted: %v", err)
			}
		})
	}
}

func TestDesktopControlledHostClientV2ReleasesExactProjectedDeliveryWithAuthenticatedAck(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV2(t)
	var admissionCalls atomic.Int32
	var releaseCalls atomic.Int32
	server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+fixture.secret ||
			request.Header.Get("Accept") != desktopControlledHostJSONMediaTypeV2 || len(request.TransferEncoding) != 0 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case desktopControlledHostAdmissionPathV2:
			admissionCalls.Add(1)
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != desktopControlledHostJSONMediaTypeV2 ||
				request.ContentLength <= 0 {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			body, err := io.ReadAll(request.Body)
			var admission desktopControlledHostAdmissionRequestV2
			if err != nil || int64(len(body)) != request.ContentLength || json.Unmarshal(body, &admission) != nil ||
				admission.SchemaVersion != desktopControlledHostProtocolVersionV2 || admission.Purpose != desktopControlledHostAdmissionPurposeV2 ||
				!reflect.DeepEqual(admission.Context, controlledHostContextBindingV2(fixture.context)) ||
				admission.ControlledHandle != fixture.handle || admission.UseSlot != fixture.useSlot ||
				admission.RendererPrincipal != fixture.principal || admission.RendererGeneration != fixture.receipt.RendererGeneration ||
				admission.BackendGeneration != fixture.receipt.BackendGeneration {
				http.Error(w, "bad admission", http.StatusBadRequest)
				return
			}
			writeCanonicalDesktopControlledHostJSONV2(t, w, fixture.admissionResponse())
		case desktopControlledHostReleasePathV2:
			releaseCalls.Add(1)
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != desktopControlledHostReleaseMediaTypeV2 ||
				request.ContentLength <= 0 {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			frame, err := io.ReadAll(request.Body)
			if err != nil || int64(len(frame)) != request.ContentLength || len(frame) < len(desktopControlledHostReleaseMagicV2)+4 ||
				!bytes.Equal(frame[:len(desktopControlledHostReleaseMagicV2)], desktopControlledHostReleaseMagicV2[:]) {
				http.Error(w, "bad frame", http.StatusBadRequest)
				return
			}
			metadataLength := int(binary.BigEndian.Uint32(frame[len(desktopControlledHostReleaseMagicV2):]))
			metadataStart := len(desktopControlledHostReleaseMagicV2) + 4
			bodyStart := metadataStart + metadataLength
			var metadata desktopControlledHostReleaseMetadataV2
			if metadataLength < 1 || bodyStart > len(frame) || json.Unmarshal(frame[metadataStart:bodyStart], &metadata) != nil ||
				metadata.SchemaVersion != desktopControlledHostProtocolVersionV2 || metadata.Purpose != desktopControlledHostReleasePurposeV2 ||
				!reflect.DeepEqual(metadata.Receipt, fixture.receipt) || !bytes.Equal(frame[bodyStart:], fixture.body) ||
				!bytes.Contains(frame[bodyStart:], []byte(desktopControlledHostTestAccountV2)) {
				http.Error(w, "wrong artifact", http.StatusBadRequest)
				return
			}
			ack := fixture.releaseAck()
			ack.MAC = base64.RawURLEncoding.EncodeToString(desktopControlledHostReleaseAckMACV2(fixture.secretBody, ack))
			writeCanonicalDesktopControlledHostJSONV2(t, w, ack)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	admission, err := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest())
	if err != nil || admission.SecurityContext != fixture.context || admission.DeliveryID != fixture.receipt.DeliveryID ||
		admission.DeliveryOutcomeRecordDigest != fixture.receipt.DeliveryOutcomeRecordDigest ||
		admission.PublicationCommitDigest != fixture.receipt.PublicationCommitDigest ||
		admission.ReleaseTargetIdentityDigest != fixture.receipt.ReleaseTargetIdentityDigest ||
		!admission.AuthorizedUntil.Equal(fixture.expires) {
		t.Fatalf("V2 desktop admission lost projected-delivery authority: admission=%#v err=%v", admission, err)
	}
	result, err := client.ReleaseV2(context.Background(), piiauthorizationport.ControlledReleaseRequestV2{
		Receipt: fixture.receipt, Body: bytes.NewReader(fixture.body), ArtifactByteLength: uint64(len(fixture.body)),
	})
	if err != nil || !result.Committed || result.ReleasedByteLength != uint64(len(fixture.body)) ||
		admissionCalls.Load() != 1 || releaseCalls.Load() != 1 {
		t.Fatalf("V2 exact controlled release failed: result=%#v calls=%d/%d err=%v",
			result, admissionCalls.Load(), releaseCalls.Load(), err)
	}
}

func TestDesktopControlledHostClientV2RejectsMalformedAdmissionAuthority(t *testing.T) {
	for name, mutate := range map[string]func(*desktopControlledHostAdmissionResponseV2){
		"context": func(value *desktopControlledHostAdmissionResponseV2) {
			value.ContextDigest = domainsecurity.SHA256Hex([]byte("other-context"))
		},
		"delivery": func(value *desktopControlledHostAdmissionResponseV2) {
			value.DeliveryID = "not-a-digest"
		},
		"outcome": func(value *desktopControlledHostAdmissionResponseV2) {
			value.DeliveryOutcomeRecordDigest = "not-a-digest"
		},
		"commit": func(value *desktopControlledHostAdmissionResponseV2) {
			value.PublicationCommitDigest = "not-a-digest"
		},
		"release target": func(value *desktopControlledHostAdmissionResponseV2) {
			value.ReleaseTargetIdentityDigest = "not-a-digest"
		},
		"backend": func(value *desktopControlledHostAdmissionResponseV2) { value.BackendGeneration++ },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDesktopControlledHostFixtureV2(t)
			server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				response := fixture.admissionResponse()
				mutate(&response)
				writeCanonicalDesktopControlledHostJSONV2(t, w, response)
			}))
			defer server.Close()
			client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest()); !errors.Is(err, ErrDesktopControlledHostRejected) {
				t.Fatalf("malformed admission authority was accepted: %v", err)
			}
		})
	}
}

func TestDesktopControlledHostClientV2RejectsReleaseAckMutationEvenWithValidMAC(t *testing.T) {
	for name, mutate := range map[string]func(*desktopControlledHostReleaseAckV2){
		"context": func(value *desktopControlledHostReleaseAckV2) {
			value.ContextDigest = domainsecurity.SHA256Hex([]byte("other-context"))
		},
		"delivery": func(value *desktopControlledHostReleaseAckV2) {
			value.DeliveryID = domainsecurity.SHA256Hex([]byte("other-delivery"))
		},
		"outcome": func(value *desktopControlledHostReleaseAckV2) {
			value.DeliveryOutcomeRecordDigest = domainsecurity.SHA256Hex([]byte("other-outcome"))
		},
		"commit": func(value *desktopControlledHostReleaseAckV2) {
			value.PublicationCommitDigest = domainsecurity.SHA256Hex([]byte("other-commit"))
		},
		"release target": func(value *desktopControlledHostReleaseAckV2) {
			value.ReleaseTargetIdentityDigest = domainsecurity.SHA256Hex([]byte("other-release-target"))
		},
		"length": func(value *desktopControlledHostReleaseAckV2) { value.ReleasedByteLength-- },
		"v1": func(value *desktopControlledHostReleaseAckV2) {
			value.SchemaVersion = 1
			value.Purpose = desktopControlledHostReleaseAckPurposeV1
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDesktopControlledHostFixtureV2(t)
			server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				ack := fixture.releaseAck()
				mutate(&ack)
				ack.MAC = base64.RawURLEncoding.EncodeToString(desktopControlledHostReleaseAckMACV2(fixture.secretBody, ack))
				writeCanonicalDesktopControlledHostJSONV2(t, w, ack)
			}))
			defer server.Close()
			client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err := client.ReleaseV2(context.Background(), piiauthorizationport.ControlledReleaseRequestV2{
				Receipt: fixture.receipt, Body: bytes.NewReader(fixture.body), ArtifactByteLength: uint64(len(fixture.body)),
			}); !errors.Is(err, ErrDesktopControlledHostProtocol) {
				t.Fatalf("mutated V2 release acknowledgement was accepted: %v", err)
			}
		})
	}
}

func TestDesktopControlledHostClientV2RejectsLegacyFallbackAndStaleGeneration(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV2(t)
	var legacyCalls atomic.Int32
	var v2Calls atomic.Int32
	server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == desktopControlledHostAdmissionPathV1 || request.URL.Path == desktopControlledHostReleasePathV1 {
			legacyCalls.Add(1)
			writeCanonicalDesktopControlledHostJSONV2(t, w, fixture.admissionResponse())
			return
		}
		v2Calls.Add(1)
		http.Error(w, "V2 unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest()); err == nil ||
		legacyCalls.Load() != 0 || v2Calls.Load() != 1 {
		t.Fatalf("V2 client fell back to a legacy path: legacy=%d v2=%d err=%v", legacyCalls.Load(), v2Calls.Load(), err)
	}
	if _, err := client.ReleaseV2(context.Background(), piiauthorizationport.ControlledReleaseRequestV2{
		Receipt: fixture.receipt, Body: bytes.NewReader(fixture.body), ArtifactByteLength: uint64(len(fixture.body)),
	}); err == nil || legacyCalls.Load() != 0 || v2Calls.Load() != 2 {
		t.Fatalf("V2 release fell back to a legacy path: legacy=%d v2=%d err=%v", legacyCalls.Load(), v2Calls.Load(), err)
	}
	stale := fixture.admissionRequest()
	stale.BackendGeneration++
	if _, err := client.ResolveCurrentV2(context.Background(), stale); !errors.Is(err, ErrDesktopControlledHostRejected) || v2Calls.Load() != 2 {
		t.Fatalf("stale backend generation reached the host: calls=%d err=%v", v2Calls.Load(), err)
	}
}

func TestDesktopControlledHostClientV2RequiresCanonicalStrictJSON(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV2(t)
	server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body, err := json.Marshal(fixture.admissionResponse())
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", desktopControlledHostJSONMediaTypeV2)
		_, _ = w.Write(append(body, '\n'))
	}))
	defer server.Close()
	client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest()); !errors.Is(err, ErrDesktopControlledHostProtocol) {
		t.Fatalf("noncanonical V2 JSON was accepted: %v", err)
	}
}

func TestDesktopControlledHostClientV2PortRebindCannotObserveControlledBytes(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV2(t)
	legitimate := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != desktopControlledHostAdmissionPathV2 {
			http.NotFound(w, request)
			return
		}
		writeCanonicalDesktopControlledHostJSONV2(t, w, fixture.admissionResponse())
	}))
	client, err := NewDesktopControlledHostClientV2(legitimate.clientConfig(fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest()); err != nil {
		t.Fatalf("legitimate admission failed before rebind: %v", err)
	}
	address := legitimate.Listener.Addr().String()
	legitimate.Close()

	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("rebind controlled host address: %v", err)
	}
	attackerIdentity := newDesktopControlledHostTLSTestIdentityV2(t)
	var handlerCalls atomic.Int32
	var observedBytes atomic.Int64
	attacker := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			handlerCalls.Add(1)
			body, _ := io.ReadAll(request.Body)
			observedBytes.Add(int64(len(body)))
			http.Error(w, "attacker", http.StatusOK)
		}),
		ErrorLog: log.New(io.Discard, "", 0),
	}
	tlsListener := tls.NewListener(listener, &tls.Config{
		Certificates: []tls.Certificate{attackerIdentity.server}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		NextProtos: []string{desktopControlledHostTLSALPNV2},
	})
	serveDone := make(chan error, 1)
	go func() { serveDone <- attacker.Serve(tlsListener) }()

	_, releaseErr := client.ReleaseV2(context.Background(), piiauthorizationport.ControlledReleaseRequestV2{
		Receipt: fixture.receipt, Body: bytes.NewReader(fixture.body), ArtifactByteLength: uint64(len(fixture.body)),
	})
	_ = attacker.Close()
	<-serveDone
	if releaseErr == nil || handlerCalls.Load() != 0 || observedBytes.Load() != 0 {
		t.Fatalf("rebound host observed protected HTTP bytes: calls=%d bytes=%d err=%v",
			handlerCalls.Load(), observedBytes.Load(), releaseErr)
	}
}

func TestDesktopControlledHostClientV2RejectsUnsafeJSONIntegersBeforeNetwork(t *testing.T) {
	for name, fixture := range map[string]*desktopControlledHostFixtureV2{
		"context epoch":       newDesktopControlledHostFixtureWithGenerationsV2(t, desktopControlledHostMaxSafeIntegerV2+1, 7, 9),
		"renderer generation": newDesktopControlledHostFixtureWithGenerationsV2(t, 4, desktopControlledHostMaxSafeIntegerV2+1, 9),
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				calls.Add(1)
			}))
			defer server.Close()
			client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest()); !errors.Is(err, ErrDesktopControlledHostRejected) {
				t.Fatalf("unsafe admission integer was not rejected: %v", err)
			}
			if _, err := client.ReleaseV2(context.Background(), piiauthorizationport.ControlledReleaseRequestV2{
				Receipt: fixture.receipt, Body: bytes.NewReader(fixture.body), ArtifactByteLength: uint64(len(fixture.body)),
			}); !errors.Is(err, ErrDesktopControlledHostRejected) {
				t.Fatalf("unsafe release integer was not rejected: %v", err)
			}
			if calls.Load() != 0 {
				t.Fatalf("unsafe integer reached the desktop host: calls=%d", calls.Load())
			}
		})
	}
	fixture := newDesktopControlledHostFixtureWithGenerationsV2(t, 4, 7, desktopControlledHostMaxSafeIntegerV2+1)
	server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	if _, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture)); !errors.Is(err, ErrDesktopControlledHostUnavailable) {
		t.Fatalf("unsafe backend generation constructed a client: %v", err)
	}
}

func TestDesktopControlledHostClientV2ReleaseCommitMustPrecedeExpiry(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV2(t)
	for name, testCase := range map[string]struct {
		at    time.Time
		valid bool
	}{
		"one nanosecond before": {at: fixture.expires.Add(-time.Nanosecond), valid: true},
		"equal":                 {at: fixture.expires, valid: false},
		"one nanosecond after":  {at: fixture.expires.Add(time.Nanosecond), valid: false},
	} {
		t.Run(name, func(t *testing.T) {
			ack := fixture.releaseAck()
			ack.CommittedAt = testCase.at.UTC().Format(time.RFC3339Nano)
			ack.MAC = base64.RawURLEncoding.EncodeToString(desktopControlledHostReleaseAckMACV2(fixture.secretBody, ack))
			err := validateDesktopControlledHostReleaseAckV2(fixture.secretBody, fixture.receipt, ack)
			if (err == nil) != testCase.valid {
				t.Fatalf("expiry boundary validity = %v, want %v: %v", err == nil, testCase.valid, err)
			}
		})
	}
}

func TestDesktopControlledHostClientV2CloseDrainsActiveCallBeforeClearingAuthority(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV2(t)
	requestEntered := make(chan struct{})
	releaseResponse := make(chan struct{})
	server := newDesktopControlledHostTLSTestServerV2(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestEntered)
		<-releaseResponse
		writeCanonicalDesktopControlledHostJSONV2(t, w, fixture.admissionResponse())
	}))
	defer server.Close()
	client, err := NewDesktopControlledHostClientV2(server.clientConfig(fixture))
	if err != nil {
		t.Fatal(err)
	}
	callDone := make(chan error, 1)
	go func() {
		_, callErr := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest())
		callDone <- callErr
	}()
	<-requestEntered
	closeDone := make(chan struct{})
	go func() {
		client.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close cleared V2 authority while a call was active")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseResponse)
	if err := <-callDone; err != nil {
		t.Fatalf("active admission failed during close drain: %v", err)
	}
	<-closeDone
	if _, err := client.ResolveCurrentV2(context.Background(), fixture.admissionRequest()); !errors.Is(err, ErrDesktopControlledHostUnavailable) {
		t.Fatalf("closed client accepted a new call: %v", err)
	}
}

func newDesktopControlledHostFixtureV2(t *testing.T) *desktopControlledHostFixtureV2 {
	return newDesktopControlledHostFixtureWithGenerationsV2(t, 4, 7, 9)
}

func newDesktopControlledHostFixtureWithGenerationsV2(
	t *testing.T,
	contextEpoch uint64,
	rendererGeneration uint64,
	backendGeneration uint64,
) *desktopControlledHostFixtureV2 {
	t.Helper()
	requested := time.Date(2026, 7, 18, 12, 0, 0, 123456789, time.UTC)
	expires := requested.Add(10 * time.Minute)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-controlled-host-v2", TurnID: "turn-controlled-host-v2", WorkspaceRealPath: "/workspace/controlled-host-v2",
		CaseID: "case-controlled-host-v2", CaseBindingHash: domainsecurity.SHA256Hex([]byte("controlled-host-v2-binding")),
		ContextEpoch: contextEpoch, IssuedAt: requested.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1,"exactValue":"` + desktopControlledHostTestAccountV2 + `"}`)
	handle := "host-controlled-publication-handle-v2"
	useSlot := "host-controlled-single-use-slot-v2"
	principal := "main-frame-renderer-principal-v2"
	handleDigest, _ := domainpii.ControlledAccessHandleDigestV2(handle)
	useSlotDigest, _ := domainpii.ControlledAccessUseSlotDigestV2(useSlot)
	principalDigest, _ := domainpii.ControlledAccessRendererPrincipalDigestV2(principal)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x74}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := domainpii.NewControlledArtifactAccessReceiptV2(domainpii.ControlledArtifactAccessReceiptInputV2{
		SecurityContext: securityContext, AccessAction: domainpii.ControlledArtifactAccessActionExportV1,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest, RendererPrincipalDigest: principalDigest,
		RendererGeneration: rendererGeneration, BackendGeneration: backendGeneration,
		AccessPolicyDigest:          domainsecurity.SHA256Hex([]byte("controlled-host-v2-access-policy")),
		RetentionPolicyDigest:       domainsecurity.SHA256Hex([]byte("controlled-host-v2-retention-policy")),
		DeliveryID:                  domainsecurity.SHA256Hex([]byte("controlled-host-v2-delivery")),
		DeliveryOutcomeRecordDigest: domainsecurity.SHA256Hex([]byte("controlled-host-v2-delivery-outcome")),
		PublicationCommitDigest:     domainsecurity.SHA256Hex([]byte("controlled-host-v2-publication-commit")),
		PublicationReceiptDigest:    domainsecurity.SHA256Hex([]byte("controlled-host-v2-publication-receipt")),
		PIIProjectionDigest:         domainsecurity.SHA256Hex([]byte("controlled-host-v2-pii-projection")),
		PIIAuthorizationDigest:      domainsecurity.SHA256Hex([]byte("controlled-host-v2-pii-authorization")),
		ClaimLedgerDigest:           domainsecurity.SHA256Hex([]byte("controlled-host-v2-claim-ledger")),
		TargetIdentityDigest:        domainsecurity.SHA256Hex([]byte("controlled-host-v2-target")),
		ReleaseTargetIdentityDigest: domainsecurity.SHA256Hex([]byte("controlled-host-v2-release-target")),
		ArtifactSHA256:              domainsecurity.SHA256Hex(body), ArtifactByteLength: uint64(len(body)),
		MediaType: domainpii.ControlledPIIArtifactMediaTypeV1, RequestedAt: requested, AuthorizedUntil: expires,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	secretBody := bytes.Repeat([]byte{0x53}, desktopControlledHostSecretBytesV1)
	return &desktopControlledHostFixtureV2{
		secret: base64.RawURLEncoding.EncodeToString(secretBody), secretBody: secretBody,
		handle: handle, useSlot: useSlot, principal: principal, context: securityContext,
		receipt: receipt, body: body, requested: requested, expires: expires,
	}
}

func (fixture *desktopControlledHostFixtureV2) admissionRequest() piiauthorizationport.ControlledAccessAdmissionRequestV2 {
	return piiauthorizationport.ControlledAccessAdmissionRequestV2{
		SecurityContext: fixture.context, AccessAction: fixture.receipt.AccessAction,
		ControlledHandle: fixture.handle, UseSlot: fixture.useSlot, RendererPrincipal: fixture.principal,
		RendererGeneration: fixture.receipt.RendererGeneration, BackendGeneration: fixture.receipt.BackendGeneration,
	}
}

func (fixture *desktopControlledHostFixtureV2) admissionResponse() desktopControlledHostAdmissionResponseV2 {
	return desktopControlledHostAdmissionResponseV2{
		SchemaVersion: desktopControlledHostProtocolVersionV2, Purpose: desktopControlledHostAdmissionPurposeV2,
		ContextDigest: fixture.context.ContextDigest, AccessAction: fixture.receipt.AccessAction,
		ControlledHandleDigest: fixture.receipt.ControlledHandleDigest, UseSlotDigest: fixture.receipt.UseSlotDigest,
		RendererPrincipalDigest: fixture.receipt.RendererPrincipalDigest,
		RendererGeneration:      fixture.receipt.RendererGeneration, BackendGeneration: fixture.receipt.BackendGeneration,
		DeliveryID: fixture.receipt.DeliveryID, DeliveryOutcomeRecordDigest: fixture.receipt.DeliveryOutcomeRecordDigest,
		PublicationCommitDigest:     fixture.receipt.PublicationCommitDigest,
		ReleaseTargetIdentityDigest: fixture.receipt.ReleaseTargetIdentityDigest,
		AuthorizedUntil:             fixture.receipt.AuthorizedUntil,
	}
}

func (fixture *desktopControlledHostFixtureV2) releaseAck() desktopControlledHostReleaseAckV2 {
	return desktopControlledHostReleaseAckV2{
		SchemaVersion: desktopControlledHostProtocolVersionV2, Purpose: desktopControlledHostReleaseAckPurposeV2,
		Committed: true, AccessID: fixture.receipt.AccessID, AccessReceiptDigest: fixture.receipt.RecordDigest,
		ContextDigest: fixture.context.ContextDigest, DeliveryID: fixture.receipt.DeliveryID,
		DeliveryOutcomeRecordDigest: fixture.receipt.DeliveryOutcomeRecordDigest,
		PublicationCommitDigest:     fixture.receipt.PublicationCommitDigest,
		ReleaseTargetIdentityDigest: fixture.receipt.ReleaseTargetIdentityDigest,
		ArtifactSHA256:              fixture.receipt.ArtifactSHA256, ReleasedByteLength: fixture.receipt.ArtifactByteLength,
		AccessAction: fixture.receipt.AccessAction, ControlledHandleDigest: fixture.receipt.ControlledHandleDigest,
		UseSlotDigest: fixture.receipt.UseSlotDigest, RendererPrincipalDigest: fixture.receipt.RendererPrincipalDigest,
		RendererGeneration: fixture.receipt.RendererGeneration, BackendGeneration: fixture.receipt.BackendGeneration,
		CommittedAt: fixture.requested.Add(time.Minute).Format(time.RFC3339Nano),
	}
}

func writeCanonicalDesktopControlledHostJSONV2(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writer.Header().Set("Content-Type", desktopControlledHostJSONMediaTypeV2)
	if _, err := writer.Write(body); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("write V2 controlled host response: %v", err)
	}
}
