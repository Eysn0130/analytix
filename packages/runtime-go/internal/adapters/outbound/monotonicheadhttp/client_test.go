package monotonicheadhttp

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports/monotonichead"
)

type witnessFixture struct {
	installationID string
	enrollmentID   string
	namespace      string
	authorityKeyID string
	authorityPub   ed25519.PublicKey
	authorityPriv  ed25519.PrivateKey
	witnessKeyID   string
	witnessPub     ed25519.PublicKey
	witnessPriv    ed25519.PrivateKey
	checkpoint     domainsecurity.MonotonicHeadCheckpointV1
}

func TestNewRejectsNonHTTPSAndNonOriginEndpoints(t *testing.T) {
	fixture := newWitnessFixture(t)
	for _, endpoint := range []string{
		"http://example.test", "https://user@example.test", "https://example.test/base",
		"https://example.test?token=secret", "https://example.test/#fragment", " https://example.test",
	} {
		config := fixture.config(endpoint, x509.NewCertPool())
		if _, err := New(config); err == nil {
			t.Fatalf("accepted unsafe endpoint %q", endpoint)
		}
	}
}

func TestNewRejectsMissingRootsAndMismatchedAuthorities(t *testing.T) {
	fixture := newWitnessFixture(t)
	config := fixture.config("https://example.test", x509.NewCertPool())
	config.RootCAs = nil
	if _, err := New(config); err == nil {
		t.Fatal("accepted a missing witness root pool")
	}
	config = fixture.config("https://example.test", x509.NewCertPool())
	config.AuthorityKeyID = digestText("wrong-authority")
	if _, err := New(config); err == nil {
		t.Fatal("accepted a mismatched installation authority")
	}
	config = fixture.config("https://example.test", x509.NewCertPool())
	config.WitnessKeyID = digestText("wrong-witness")
	if _, err := New(config); err == nil {
		t.Fatal("accepted a mismatched witness authority")
	}
}

func TestObserveUsesPinnedTLS13AndCanonicalProtocol(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "observe-valid")
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		assertProtocolRequest(t, incoming, ObservePath)
		body, err := io.ReadAll(incoming.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		parsed, err := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
		if err != nil || parsed != request {
			t.Errorf("request was not canonical or exact: %v", err)
			return
		}
		observation := fixture.observation(t, parsed, fixture.checkpoint)
		writeObservation(t, writer, observation)
	}))
	client := fixture.client(t, server.URL, roots)
	observation, err := client.Observe(context.Background(), request)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if observation.Checkpoint.CheckpointDigest != fixture.checkpoint.CheckpointDigest {
		t.Fatal("observe returned the wrong checkpoint")
	}
}

func TestEachWitnessOperationUsesFreshTLSConnection(t *testing.T) {
	fixture := newWitnessFixture(t)
	var mu sync.Mutex
	remoteAddresses := make([]string, 0, 2)
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		mu.Lock()
		remoteAddresses = append(remoteAddresses, incoming.RemoteAddr)
		mu.Unlock()
		body, err := io.ReadAll(incoming.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		request, err := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
		if err != nil {
			t.Errorf("parse request: %v", err)
			return
		}
		writeObservation(t, writer, fixture.observation(t, request, fixture.checkpoint))
	}))
	client := fixture.client(t, server.URL, roots)
	for _, nonce := range []string{"fresh-tls-one", "fresh-tls-two"} {
		if _, err := client.Observe(context.Background(), fixture.observeRequest(t, nonce)); err != nil {
			t.Fatalf("observe %s: %v", nonce, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(remoteAddresses) != 2 || remoteAddresses[0] == remoteAddresses[1] {
		t.Fatalf("witness operations reused a TLS connection: %v", remoteAddresses)
	}
}

func TestAdvanceUsesPinnedTLS13AndCanonicalProtocol(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, receipt := fixture.advance(t, fixture.checkpoint, "advance-valid", "next-valid")
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		assertProtocolRequest(t, incoming, AdvancePath)
		body, err := io.ReadAll(incoming.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		parsed, err := domainsecurity.ParseMonotonicHeadAdvanceRequestV1(body)
		if err != nil || parsed != request {
			t.Errorf("request was not canonical or exact: %v", err)
			return
		}
		writeReceipt(t, writer, receipt)
	}))
	client := fixture.client(t, server.URL, roots)
	got, err := client.Advance(context.Background(), request)
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if got.ReceiptDigest != receipt.ReceiptDigest {
		t.Fatal("advance returned the wrong receipt")
	}
}

func TestResolveMutationUsesPinnedTLS13AndCanonicalReadOnlyProtocol(t *testing.T) {
	fixture := newWitnessFixture(t)
	advance, receipt := fixture.advance(t, fixture.checkpoint, "resolve-valid", "resolve-next")
	request := fixture.resolveRequest(t, advance, "resolve-challenge")
	resolution := fixture.mutationResolution(t, request, receipt.Checkpoint, &domainsecurity.MonotonicHeadCommittedMutationV1{
		AdvanceRequest: advance, AdvanceReceipt: receipt,
	})
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		assertProtocolRequest(t, incoming, MutationResolvePath)
		body, err := io.ReadAll(incoming.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		parsed, err := domainsecurity.ParseMonotonicHeadMutationResolveRequestV1(body)
		if err != nil || parsed != request {
			t.Errorf("mutation resolve request was not canonical or exact: parsed=%#v err=%v", parsed, err)
			return
		}
		writeMutationResolution(t, writer, resolution)
	}))
	client := fixture.client(t, server.URL, roots)
	got, err := client.ResolveMutation(context.Background(), request)
	if err != nil {
		t.Fatalf("resolve mutation: %v", err)
	}
	if got.Status != domainsecurity.MonotonicHeadMutationResolutionCommittedV1 || got.Committed == nil ||
		got.Committed.AdvanceReceipt.ReceiptDigest != receipt.ReceiptDigest {
		t.Fatalf("resolve mutation returned the wrong committed receipt: %#v", got)
	}
}

func TestResolveMutationRejectsReplayedChallengeAndUnsupportedEndpoint(t *testing.T) {
	fixture := newWitnessFixture(t)
	advance, receipt := fixture.advance(t, fixture.checkpoint, "resolve-replay", "resolve-replay-next")
	request := fixture.resolveRequest(t, advance, "resolve-current-challenge")
	oldRequest := fixture.resolveRequest(t, advance, "resolve-old-challenge")
	oldResolution := fixture.mutationResolution(t, oldRequest, receipt.Checkpoint, &domainsecurity.MonotonicHeadCommittedMutationV1{
		AdvanceRequest: advance, AdvanceReceipt: receipt,
	})
	t.Run("replayed challenge", func(t *testing.T) {
		server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writeMutationResolution(t, writer, oldResolution)
		}))
		client := fixture.client(t, server.URL, roots)
		if _, err := client.ResolveMutation(context.Background(), request); !errors.Is(err, monotonichead.ErrInvalidReceipt) {
			t.Fatalf("replayed recovery challenge classification = %v", err)
		}
	})
	t.Run("unsupported endpoint", func(t *testing.T) {
		server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", protocolContentType)
			writer.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(writer, `{"schemaVersion":1,"purpose":"analytix.monotonic-head-error/v1","code":"unsupported"}`)
		}))
		client := fixture.client(t, server.URL, roots)
		if _, err := client.ResolveMutation(context.Background(), request); !errors.Is(err, monotonichead.ErrInvalidReceipt) || errors.Is(err, monotonichead.ErrIndeterminate) {
			t.Fatalf("unsupported recovery endpoint did not fail closed as a read: %v", err)
		}
	})
}

func TestResolveMutationAcceptsSignedAbsentOnlyAsAbsent(t *testing.T) {
	fixture := newWitnessFixture(t)
	advance, _ := fixture.advance(t, fixture.checkpoint, "resolve-absent", "resolve-absent-next")
	request := fixture.resolveRequest(t, advance, "resolve-absent-challenge")
	resolution := fixture.mutationResolution(t, request, fixture.checkpoint, nil)
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeMutationResolution(t, writer, resolution)
	}))
	client := fixture.client(t, server.URL, roots)
	got, err := client.ResolveMutation(context.Background(), request)
	if err != nil {
		t.Fatalf("resolve absent mutation: %v", err)
	}
	if got.Status != domainsecurity.MonotonicHeadMutationResolutionAbsentV1 || got.Committed != nil {
		t.Fatalf("signed absent response was upgraded to committed: %#v", got)
	}
}

func TestTLS12OnlyWitnessIsUnavailableBeforeAdvanceDispatch(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, _ := fixture.advance(t, fixture.checkpoint, "tls12", "tls12-next")
	var calls atomic.Int32
	server, roots := newTLS12Server(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	client := fixture.client(t, server.URL, roots)
	_, err := client.Advance(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrUnavailable) || errors.Is(err, monotonichead.ErrIndeterminate) {
		t.Fatalf("TLS negotiation failure classification = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("TLS 1.2 server received %d application requests", calls.Load())
	}
}

func TestUntrustedWitnessCertificateIsUnavailable(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "untrusted")
	server, _ := newTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := fixture.client(t, server.URL, x509.NewCertPool())
	_, err := client.Observe(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrUnavailable) {
		t.Fatalf("untrusted certificate classification = %v", err)
	}
}

func TestRedirectIsRejectedWithoutFollowing(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "redirect")
	var redirected atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		if incoming.URL.Path == "/redirected" {
			redirected.Add(1)
			return
		}
		writer.Header().Set("Location", "/redirected")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	client := fixture.client(t, server.URL, roots)
	_, err := client.Observe(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrInvalidReceipt) {
		t.Fatalf("redirect classification = %v", err)
	}
	if redirected.Load() != 0 {
		t.Fatal("client followed a witness redirect")
	}
}

func TestOversizedResponseIsRejected(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "oversize")
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", protocolContentType)
		_, _ = io.WriteString(writer, strings.Repeat("x", responseBodyLimit+1))
	}))
	client := fixture.client(t, server.URL, roots)
	_, err := client.Observe(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrInvalidReceipt) {
		t.Fatalf("oversized body classification = %v", err)
	}
}

func TestOversizedResponseHeadersPreserveMutationUncertainty(t *testing.T) {
	fixture := newWitnessFixture(t)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", protocolContentType)
		writer.Header().Set("X-Oversized", strings.Repeat("a", responseHeaderLimit+1))
		writer.WriteHeader(http.StatusOK)
	})
	t.Run("observe unavailable", func(t *testing.T) {
		server, roots := newTLSServer(t, handler)
		client := fixture.client(t, server.URL, roots)
		_, err := client.Observe(context.Background(), fixture.observeRequest(t, "oversized-observe-header"))
		if !errors.Is(err, monotonichead.ErrUnavailable) || errors.Is(err, monotonichead.ErrIndeterminate) {
			t.Fatalf("oversized observe header classification = %v", err)
		}
	})
	t.Run("advance indeterminate", func(t *testing.T) {
		server, roots := newTLSServer(t, handler)
		client := fixture.client(t, server.URL, roots)
		request, _ := fixture.advance(t, fixture.checkpoint, "oversized-advance-header", "state-one")
		_, err := client.Advance(context.Background(), request)
		if !errors.Is(err, monotonichead.ErrIndeterminate) {
			t.Fatalf("oversized advance header classification = %v", err)
		}
	})
}

func TestMissingOrParameterizedContentTypeIsRejected(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "content-type")
	observation := fixture.observation(t, request, fixture.checkpoint)
	body := mustObservationBytes(t, observation)
	for _, contentType := range []string{"", "application/json; charset=utf-8", "text/plain"} {
		server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			if contentType != "" {
				writer.Header().Set("Content-Type", contentType)
			}
			_, _ = writer.Write(body)
		}))
		client := fixture.client(t, server.URL, roots)
		if _, err := client.Observe(context.Background(), request); !errors.Is(err, monotonichead.ErrInvalidReceipt) {
			t.Fatalf("content type %q classification = %v", contentType, err)
		}
		server.Close()
	}
}

func TestUnknownResponseFieldIsRejected(t *testing.T) {
	fixture, request, canonical := canonicalObservationFixture(t, "unknown")
	body := append([]byte(`{"unknown":true,`), canonical[1:]...)
	assertObserveBodyRejected(t, fixture, request, body)
}

func TestDuplicateResponseFieldIsRejected(t *testing.T) {
	fixture, request, canonical := canonicalObservationFixture(t, "duplicate")
	body := append([]byte(`{"schemaVersion":1,`), canonical[1:]...)
	assertObserveBodyRejected(t, fixture, request, body)
}

func TestTrailingResponseJSONIsRejected(t *testing.T) {
	fixture, request, canonical := canonicalObservationFixture(t, "trailing")
	body := append(append([]byte(nil), canonical...), []byte(`{}`)...)
	assertObserveBodyRejected(t, fixture, request, body)
}

func TestNonCanonicalResponseJSONIsRejected(t *testing.T) {
	fixture, request, canonical := canonicalObservationFixture(t, "noncanonical")
	body := append(append([]byte(nil), canonical...), '\n')
	assertObserveBodyRejected(t, fixture, request, body)
}

func TestCanonicalErrorMappings(t *testing.T) {
	fixture := newWitnessFixture(t)
	tests := []struct {
		name     string
		status   int
		code     string
		mutation bool
		want     error
	}{
		{"not-enrolled", http.StatusNotFound, "not_enrolled", false, monotonichead.ErrNotEnrolled},
		{"cas", http.StatusConflict, "cas_conflict", true, monotonichead.ErrCASConflict},
		{"mutation", http.StatusConflict, "mutation_conflict", true, monotonichead.ErrMutationConflict},
		{"unavailable", http.StatusServiceUnavailable, "unavailable", false, monotonichead.ErrUnavailable},
		{"advance-unavailable", http.StatusServiceUnavailable, "unavailable", true, monotonichead.ErrIndeterminate},
		{"invalid", http.StatusUnprocessableEntity, "invalid_receipt", false, monotonichead.ErrInvalidReceipt},
		{"advance-invalid", http.StatusUnprocessableEntity, "invalid_receipt", true, monotonichead.ErrIndeterminate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeErrorEnvelope(t, writer, test.status, test.code)
			}))
			client := fixture.client(t, server.URL, roots)
			var err error
			if test.mutation {
				request, _ := fixture.advance(t, fixture.checkpoint, "error-"+test.name, "next-"+test.name)
				_, err = client.Advance(context.Background(), request)
			} else {
				_, err = client.Observe(context.Background(), fixture.observeRequest(t, "error-"+test.name))
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("classification = %v, want %v", err, test.want)
			}
			if test.name == "advance-unavailable" && !errors.Is(err, monotonichead.ErrUnavailable) {
				t.Fatalf("advance unavailable lost diagnostic classification: %v", err)
			}
			if test.name == "advance-invalid" && !errors.Is(err, monotonichead.ErrInvalidReceipt) {
				t.Fatalf("advance invalid lost diagnostic classification: %v", err)
			}
		})
	}
}

func TestErrorEnvelopeRejectsUnknownDuplicateTrailingAndNonCanonicalJSON(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "bad-errors")
	canonical, _ := json.Marshal(errorEnvelope{SchemaVersion: 1, Purpose: errorPurpose, Code: "unavailable"})
	bodies := [][]byte{
		append([]byte(`{"unknown":true,`), canonical[1:]...),
		append([]byte(`{"code":"unavailable",`), canonical[1:]...),
		append(append([]byte(nil), canonical...), []byte(`{}`)...),
		append(append([]byte(nil), canonical...), '\n'),
	}
	for _, body := range bodies {
		server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", protocolContentType)
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write(body)
		}))
		client := fixture.client(t, server.URL, roots)
		if _, err := client.Observe(context.Background(), request); !errors.Is(err, monotonichead.ErrInvalidReceipt) {
			t.Fatalf("bad error envelope classification = %v", err)
		}
		server.Close()
	}
}

func TestOutgoingWrongEnrollmentIsRejectedBeforeNetwork(t *testing.T) {
	fixture := newWitnessFixture(t)
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	client := fixture.client(t, server.URL, roots)
	request := fixture.observeRequestForEnrollment(t, digestText("other-enrollment"), "wrong-outgoing-enrollment")
	_, err := client.Observe(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrInvalidReceipt) || calls.Load() != 0 {
		t.Fatalf("outgoing enrollment result = %v, calls = %d", err, calls.Load())
	}
}

func TestObservationWrongWitnessKeyIsRejected(t *testing.T) {
	fixture := newWitnessFixture(t)
	other := newWitnessFixture(t)
	request := fixture.observeRequest(t, "wrong-witness")
	checkpoint := other.checkpointFor(t, fixture.installationID, fixture.enrollmentID, fixture.namespace, 0,
		digestText("state-wrong-witness"), "", "", digestText("fence-wrong-witness"), "")
	observation := other.observation(t, request, checkpoint)
	assertObserveBodyRejected(t, fixture, request, mustObservationBytes(t, observation))
}

func TestObservationWrongEnrollmentIsRejected(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "expected-enrollment")
	otherEnrollment := digestText("other-observation-enrollment")
	otherRequest := fixture.observeRequestForEnrollment(t, otherEnrollment, "other-enrollment-request")
	checkpoint := fixture.checkpointFor(t, fixture.installationID, otherEnrollment, fixture.namespace, 0,
		digestText("other-enrollment-state"), "", "", digestText("other-enrollment-fence"), "")
	observation := fixture.observation(t, otherRequest, checkpoint)
	assertObserveBodyRejected(t, fixture, request, mustObservationBytes(t, observation))
}

func TestObservationWrongRequestBindingIsRejected(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "request-one")
	otherRequest := fixture.observeRequest(t, "request-two")
	observation := fixture.observation(t, otherRequest, fixture.checkpoint)
	assertObserveBodyRejected(t, fixture, request, mustObservationBytes(t, observation))
}

func TestAdvanceReceiptWrongRequestBindingIsRejected(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, _ := fixture.advance(t, fixture.checkpoint, "binding-one", "binding-next-one")
	_, otherReceipt := fixture.advance(t, fixture.checkpoint, "binding-two", "binding-next-two")
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeReceipt(t, writer, otherReceipt)
	}))
	client := fixture.client(t, server.URL, roots)
	_, err := client.Advance(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrInvalidReceipt) || !errors.Is(err, monotonichead.ErrIndeterminate) {
		t.Fatalf("wrong receipt binding classification = %v", err)
	}
}

func TestAdvanceDoesNotRetryAfterConnectionLoss(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, _ := fixture.advance(t, fixture.checkpoint, "no-retry", "no-retry-next")
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Error("test server does not support hijacking")
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = connection.Close()
	}))
	client := fixture.client(t, server.URL, roots)
	_, err := client.Advance(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrIndeterminate) {
		t.Fatalf("post-dispatch connection loss classification = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("advance was sent %d times", calls.Load())
	}
}

func TestAdvancePreCanceledContextDoesNotDispatch(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, _ := fixture.advance(t, fixture.checkpoint, "pre-cancel", "pre-cancel-next")
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	client := fixture.client(t, server.URL, roots)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Advance(ctx, request)
	if !errors.Is(err, context.Canceled) || errors.Is(err, monotonichead.ErrIndeterminate) || calls.Load() != 0 {
		t.Fatalf("pre-cancel result = %v, calls = %d", err, calls.Load())
	}
}

func TestAdvancePostDispatchCancellationIsIndeterminate(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, _ := fixture.advance(t, fixture.checkpoint, "post-cancel", "post-cancel-next")
	entered := make(chan struct{})
	release := make(chan struct{})
	server, roots := newTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
	}))
	client := fixture.client(t, server.URL, roots)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Advance(ctx, request)
		result <- err
	}()
	<-entered
	cancel()
	err := <-result
	close(release)
	if !errors.Is(err, monotonichead.ErrIndeterminate) || !errors.Is(err, context.Canceled) {
		t.Fatalf("post-dispatch cancellation classification = %v", err)
	}
}

func TestObservePostDispatchCancellationReturnsContextError(t *testing.T) {
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, "observe-cancel")
	entered := make(chan struct{})
	release := make(chan struct{})
	server, roots := newTLSServer(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
	}))
	client := fixture.client(t, server.URL, roots)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Observe(ctx, request)
		result <- err
	}()
	<-entered
	cancel()
	err := <-result
	close(release)
	if !errors.Is(err, context.Canceled) || errors.Is(err, monotonichead.ErrIndeterminate) {
		t.Fatalf("observe cancellation classification = %v", err)
	}
}

func TestAdvanceExactIdempotentReceiptIsAccepted(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, receipt := fixture.advance(t, fixture.checkpoint, "idempotent", "idempotent-next")
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeReceipt(t, writer, receipt)
	}))
	client := fixture.client(t, server.URL, roots)
	first, firstErr := client.Advance(context.Background(), request)
	second, secondErr := client.Advance(context.Background(), request)
	if firstErr != nil || secondErr != nil || first.ReceiptDigest != second.ReceiptDigest {
		t.Fatalf("idempotent results = (%v, %v)", firstErr, secondErr)
	}
	if calls.Load() != 2 {
		t.Fatalf("explicit replay did not consult witness exactly once: %d calls", calls.Load())
	}
}

func TestAdvanceMutationIDReuseWithDifferentRequestIsLocallyRejected(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, receipt := fixture.advance(t, fixture.checkpoint, "mutation-reuse", "mutation-next-one")
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeReceipt(t, writer, receipt)
	}))
	client := fixture.client(t, server.URL, roots)
	if _, err := client.Advance(context.Background(), request); err != nil {
		t.Fatalf("first advance: %v", err)
	}
	reused := fixture.advanceRequest(t, fixture.checkpoint, request.MutationID, digestText("mutation-next-two"))
	_, err := client.Advance(context.Background(), reused)
	if !errors.Is(err, monotonichead.ErrMutationConflict) || calls.Load() != 1 {
		t.Fatalf("mutation reuse result = %v, calls = %d", err, calls.Load())
	}
}

func TestReplayDefenseMemoryIsBounded(t *testing.T) {
	client := &Client{
		heads: make(map[headKey][]byte), mutations: make(map[string]mutationRecord),
		headOrder: make([]headKey, 0, headReplayWindow), mutationOrder: make([]string, 0, mutationReplayWindow),
	}
	for index := 0; index < mutationReplayWindow+257; index++ {
		mutationID := digestText(fmt.Sprintf("bounded-mutation-%d", index))
		request := []byte(fmt.Sprintf("request-%d", index))
		if err := client.reserveMutation(mutationID, request); err != nil {
			t.Fatal(err)
		}
	}
	if len(client.mutations) != mutationReplayWindow || len(client.mutationOrder) != mutationReplayWindow {
		t.Fatalf("mutation replay memory is unbounded: records=%d order=%d", len(client.mutations), len(client.mutationOrder))
	}
	client.mu.Lock()
	for index := 0; index < headReplayWindow+257; index++ {
		checkpoint := domainsecurity.MonotonicHeadCheckpointV1{Namespace: "bounded-head", Generation: uint64(index * 2)}
		canonical := []byte(fmt.Sprintf("checkpoint-%d", index))
		if err := client.recordHeadLocked(checkpoint, canonical); err != nil {
			client.mu.Unlock()
			t.Fatal(err)
		}
	}
	client.mu.Unlock()
	if len(client.heads) != headReplayWindow || len(client.headOrder) != headReplayWindow {
		t.Fatalf("head replay memory is unbounded: records=%d order=%d", len(client.heads), len(client.headOrder))
	}
}

func TestAdvanceIndeterminateMutationIDCannotBeRebound(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, _ := fixture.advance(t, fixture.checkpoint, "indeterminate-reservation", "indeterminate-next-one")
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		hijacker := writer.(http.Hijacker)
		connection, _, err := hijacker.Hijack()
		if err == nil {
			_ = connection.Close()
		}
	}))
	client := fixture.client(t, server.URL, roots)
	if _, err := client.Advance(context.Background(), request); !errors.Is(err, monotonichead.ErrIndeterminate) {
		t.Fatalf("first advance was not indeterminate: %v", err)
	}
	rebound := fixture.advanceRequest(t, fixture.checkpoint, request.MutationID, digestText("indeterminate-next-two"))
	if _, err := client.Advance(context.Background(), rebound); !errors.Is(err, monotonichead.ErrMutationConflict) {
		t.Fatalf("indeterminate mutation id was rebound: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("rebound mutation reached witness: calls=%d", calls.Load())
	}
}

func TestAdvanceMalformedSuccessIsIndeterminate(t *testing.T) {
	fixture := newWitnessFixture(t)
	request, _ := fixture.advance(t, fixture.checkpoint, "malformed-success", "malformed-success-next")
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", protocolContentType)
		_, _ = writer.Write([]byte(`{"not":"a receipt"}`))
	}))
	client := fixture.client(t, server.URL, roots)
	_, err := client.Advance(context.Background(), request)
	if !errors.Is(err, monotonichead.ErrIndeterminate) || !errors.Is(err, monotonichead.ErrInvalidReceipt) {
		t.Fatalf("malformed committed response classification = %v", err)
	}
}

func TestSameGenerationDifferentSignedCheckpointIsEquivocation(t *testing.T) {
	fixture := newWitnessFixture(t)
	alternate := fixture.checkpointFor(t, fixture.installationID, fixture.enrollmentID, fixture.namespace, 0,
		digestText("alternate-state"), "", "", digestText("alternate-fence"), "")
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		body, _ := io.ReadAll(incoming.Body)
		request, _ := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
		checkpoint := fixture.checkpoint
		if calls.Add(1) == 2 {
			checkpoint = alternate
		}
		writeObservation(t, writer, fixture.observation(t, request, checkpoint))
	}))
	client := fixture.client(t, server.URL, roots)
	if _, err := client.Observe(context.Background(), fixture.observeRequest(t, "equivocation-one")); err != nil {
		t.Fatalf("first observation: %v", err)
	}
	_, err := client.Observe(context.Background(), fixture.observeRequest(t, "equivocation-two"))
	if !errors.Is(err, monotonichead.ErrEquivocation) {
		t.Fatalf("equivocation classification = %v", err)
	}
}

func TestAdjacentIncompatibleSignedHeadsAreEquivocation(t *testing.T) {
	fixture := newWitnessFixture(t)
	incompatible := fixture.checkpointFor(t, fixture.installationID, fixture.enrollmentID, fixture.namespace, 1,
		digestText("bad-next-state"), digestText("unrelated-state"), digestText("unrelated-checkpoint"),
		digestText("bad-next-fence"), digestText("bad-next-mutation"))
	var calls atomic.Int32
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		body, _ := io.ReadAll(incoming.Body)
		request, _ := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
		checkpoint := fixture.checkpoint
		if calls.Add(1) == 2 {
			checkpoint = incompatible
		}
		writeObservation(t, writer, fixture.observation(t, request, checkpoint))
	}))
	client := fixture.client(t, server.URL, roots)
	if _, err := client.Observe(context.Background(), fixture.observeRequest(t, "adjacent-one")); err != nil {
		t.Fatalf("first observation: %v", err)
	}
	_, err := client.Observe(context.Background(), fixture.observeRequest(t, "adjacent-two"))
	if !errors.Is(err, monotonichead.ErrEquivocation) {
		t.Fatalf("adjacent incompatibility classification = %v", err)
	}
}

func TestConcurrentObservationsAreRaceSafe(t *testing.T) {
	fixture := newWitnessFixture(t)
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		body, err := io.ReadAll(incoming.Body)
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		request, err := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
		if err != nil {
			t.Errorf("parse: %v", err)
			return
		}
		writeObservation(t, writer, fixture.observation(t, request, fixture.checkpoint))
	}))
	client := fixture.client(t, server.URL, roots)
	var wait sync.WaitGroup
	errorsFound := make(chan error, 32)
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := client.Observe(context.Background(), fixture.observeRequest(t, fmt.Sprintf("concurrent-%d", index)))
			if err != nil {
				errorsFound <- err
			}
		}(index)
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent observe: %v", err)
	}
}

func newWitnessFixture(t *testing.T) witnessFixture {
	t.Helper()
	authorityPub, authorityPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	witnessPub, witnessPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fixture := witnessFixture{
		installationID: digestText("installation"),
		enrollmentID:   digestText("enrollment"),
		namespace:      domainsecurity.ThreadRiskAuthorityNamespaceV1,
		authorityKeyID: sha256Hex(authorityPub),
		authorityPub:   authorityPub,
		authorityPriv:  authorityPriv,
		witnessKeyID:   sha256Hex(witnessPub),
		witnessPub:     witnessPub,
		witnessPriv:    witnessPriv,
	}
	fixture.checkpoint = fixture.checkpointFor(t, fixture.installationID, fixture.enrollmentID, fixture.namespace, 0,
		digestText("state-zero"), "", "", digestText("fence-zero"), "")
	return fixture
}

func (fixture witnessFixture) config(endpoint string, roots *x509.CertPool) Config {
	return Config{
		Endpoint: endpoint, InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID,
		Namespace: fixture.namespace, AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPub,
		WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPub, RootCAs: roots, Timeout: 2 * time.Second,
	}
}

func (fixture witnessFixture) client(t *testing.T, endpoint string, roots *x509.CertPool) *Client {
	t.Helper()
	client, err := New(fixture.config(endpoint, roots))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func (fixture witnessFixture) observeRequest(t *testing.T, nonce string) domainsecurity.MonotonicHeadObserveRequestV1 {
	t.Helper()
	return fixture.observeRequestForEnrollment(t, fixture.enrollmentID, nonce)
}

func (fixture witnessFixture) observeRequestForEnrollment(t *testing.T, enrollmentID, nonce string) domainsecurity.MonotonicHeadObserveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: fixture.installationID, EnrollmentID: enrollmentID, Namespace: fixture.namespace,
		ChallengeNonce: digestText(nonce), AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPub,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authorityPriv, message), nil })
	if err != nil {
		t.Fatalf("new observe request: %v", err)
	}
	return request
}

func (fixture witnessFixture) observation(t *testing.T, request domainsecurity.MonotonicHeadObserveRequestV1, checkpoint domainsecurity.MonotonicHeadCheckpointV1) domainsecurity.MonotonicHeadObservationV1 {
	t.Helper()
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.witnessPriv, message), nil
	})
	if err != nil {
		t.Fatalf("new observation: %v", err)
	}
	return observation
}

func (fixture witnessFixture) advance(t *testing.T, previous domainsecurity.MonotonicHeadCheckpointV1, mutation, nextState string) (domainsecurity.MonotonicHeadAdvanceRequestV1, domainsecurity.MonotonicHeadAdvanceReceiptV1) {
	t.Helper()
	request := fixture.advanceRequest(t, previous, digestText(mutation), digestText(nextState))
	checkpoint := fixture.checkpointFor(t, fixture.installationID, fixture.enrollmentID, fixture.namespace, request.NextGeneration,
		request.NextStateDigest, request.ExpectedStateDigest, request.ExpectedCheckpointDigest,
		digestText("fence-"+mutation), request.MutationID)
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.witnessPriv, message), nil
	})
	if err != nil {
		t.Fatalf("new receipt: %v", err)
	}
	return request, receipt
}

func (fixture witnessFixture) advanceRequest(t *testing.T, previous domainsecurity.MonotonicHeadCheckpointV1, mutationID, nextStateDigest string) domainsecurity.MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.installationID, EnrollmentID: fixture.enrollmentID, Namespace: fixture.namespace,
		ExpectedGeneration: previous.Generation, ExpectedCheckpointDigest: previous.CheckpointDigest,
		ExpectedStateDigest: previous.CurrentStateDigest, NextGeneration: previous.Generation + 1,
		NextStateDigest: nextStateDigest, ExpectedFenceNonce: previous.FenceNonce, MutationID: mutationID,
		AuthorityKeyID: fixture.authorityKeyID, AuthorityPublicKey: fixture.authorityPub,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authorityPriv, message), nil })
	if err != nil {
		t.Fatalf("new advance request: %v", err)
	}
	return request
}

func (fixture witnessFixture) resolveRequest(t *testing.T, advance domainsecurity.MonotonicHeadAdvanceRequestV1, nonce string) domainsecurity.MonotonicHeadMutationResolveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadMutationResolveRequestV1(
		advance,
		digestText(nonce),
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authorityPriv, message), nil },
	)
	if err != nil {
		t.Fatalf("new mutation resolve request: %v", err)
	}
	return request
}

func (fixture witnessFixture) mutationResolution(
	t *testing.T,
	request domainsecurity.MonotonicHeadMutationResolveRequestV1,
	current domainsecurity.MonotonicHeadCheckpointV1,
	committed *domainsecurity.MonotonicHeadCommittedMutationV1,
) domainsecurity.MonotonicHeadMutationResolutionV1 {
	t.Helper()
	resolution, err := domainsecurity.NewMonotonicHeadMutationResolutionV1(
		request,
		current,
		committed,
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessPriv, message), nil },
	)
	if err != nil {
		t.Fatalf("new mutation resolution: %v", err)
	}
	return resolution
}

func (fixture witnessFixture) checkpointFor(
	t *testing.T, installationID, enrollmentID, namespace string, generation uint64,
	state, previousState, previousCheckpoint, fence, mutation string,
) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: namespace, Generation: generation,
		CurrentStateDigest: state, PreviousStateDigest: previousState, PreviousCheckpointDigest: previousCheckpoint,
		FenceNonce: fence, MutationID: mutation, WitnessKeyID: fixture.witnessKeyID, WitnessPublicKey: fixture.witnessPub,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.witnessPriv, message), nil })
	if err != nil {
		t.Fatalf("new checkpoint: %v", err)
	}
	return checkpoint
}

func newTLSServer(t *testing.T, handler http.Handler) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	return newServerWithTLS(t, handler, tls.VersionTLS13, 0)
}

func newTLS12Server(t *testing.T, handler http.Handler) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	return newServerWithTLS(t, handler, tls.VersionTLS12, tls.VersionTLS12)
}

func newServerWithTLS(t *testing.T, handler http.Handler, minVersion, maxVersion uint16) (*httptest.Server, *x509.CertPool) {
	return newServerWithTLSLeafMutation(t, handler, minVersion, maxVersion, nil)
}

func newServerWithTLSLeafMutation(
	t *testing.T,
	handler http.Handler,
	minVersion, maxVersion uint16,
	mutateLeaf func(*x509.Certificate),
) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Analytix Monotonic Head Test Root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	if mutateLeaf != nil {
		mutateLeaf(serverTemplate)
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, root, &serverKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	serverLeaf, err := x509.ParseCertificate(serverDER)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		MinVersion: minVersion, MaxVersion: maxVersion,
		Certificates: []tls.Certificate{{Certificate: [][]byte{serverDER}, PrivateKey: serverKey, Leaf: serverLeaf}},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	return server, roots
}

func assertProtocolRequest(t *testing.T, request *http.Request, wantPath string) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Path != wantPath || request.URL.RawQuery != "" ||
		request.Header.Get("Accept") != protocolContentType || request.Header.Get("Content-Type") != protocolContentType {
		t.Errorf("unexpected protocol request: %s %s headers=%v", request.Method, request.URL.String(), request.Header)
	}
}

func writeObservation(t *testing.T, writer http.ResponseWriter, observation domainsecurity.MonotonicHeadObservationV1) {
	t.Helper()
	body := mustObservationBytes(t, observation)
	writer.Header().Set("Content-Type", protocolContentType)
	_, _ = writer.Write(body)
}

func writeReceipt(t *testing.T, writer http.ResponseWriter, receipt domainsecurity.MonotonicHeadAdvanceReceiptV1) {
	t.Helper()
	body, err := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatalf("receipt bytes: %v", err)
	}
	writer.Header().Set("Content-Type", protocolContentType)
	_, _ = writer.Write(body)
}

func writeMutationResolution(t *testing.T, writer http.ResponseWriter, resolution domainsecurity.MonotonicHeadMutationResolutionV1) {
	t.Helper()
	body, err := domainsecurity.MonotonicHeadMutationResolutionV1Bytes(resolution)
	if err != nil {
		t.Fatalf("mutation resolution bytes: %v", err)
	}
	writer.Header().Set("Content-Type", protocolContentType)
	_, _ = writer.Write(body)
}

func writeErrorEnvelope(t *testing.T, writer http.ResponseWriter, status int, code string) {
	t.Helper()
	body, err := json.Marshal(errorEnvelope{SchemaVersion: errorSchemaVersion, Purpose: errorPurpose, Code: code})
	if err != nil {
		t.Fatal(err)
	}
	writer.Header().Set("Content-Type", protocolContentType)
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
}

func mustObservationBytes(t *testing.T, observation domainsecurity.MonotonicHeadObservationV1) []byte {
	t.Helper()
	body, err := domainsecurity.MonotonicHeadObservationV1Bytes(observation)
	if err != nil {
		t.Fatalf("observation bytes: %v", err)
	}
	return body
}

func canonicalObservationFixture(t *testing.T, nonce string) (witnessFixture, domainsecurity.MonotonicHeadObserveRequestV1, []byte) {
	t.Helper()
	fixture := newWitnessFixture(t)
	request := fixture.observeRequest(t, nonce)
	return fixture, request, mustObservationBytes(t, fixture.observation(t, request, fixture.checkpoint))
}

func assertObserveBodyRejected(t *testing.T, fixture witnessFixture, request domainsecurity.MonotonicHeadObserveRequestV1, body []byte) {
	t.Helper()
	server, roots := newTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", protocolContentType)
		_, _ = writer.Write(body)
	}))
	client := fixture.client(t, server.URL, roots)
	if _, err := client.Observe(context.Background(), request); !errors.Is(err, monotonichead.ErrInvalidReceipt) {
		t.Fatalf("malformed observation classification = %v", err)
	}
}

func digestText(value string) string {
	return sha256Hex([]byte(value))
}
