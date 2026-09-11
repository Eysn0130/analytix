package monotonicheadhttp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports/monotonichead"
)

const (
	ObservePath         = "/v1/monotonic-head/observe"
	AdvancePath         = "/v1/monotonic-head/advance"
	MutationResolvePath = "/v1/monotonic-head/mutation/resolve"

	protocolContentType  = "application/json"
	errorPurpose         = "analytix.monotonic-head-error/v1"
	errorSchemaVersion   = 1
	defaultTimeout       = 10 * time.Second
	requestBodyLimit     = 64 << 10
	responseBodyLimit    = 64 << 10
	responseHeaderLimit  = 16 << 10
	headReplayWindow     = 4_096
	mutationReplayWindow = 4_096
)

var (
	errRedirectRejected = errors.New("monotonic head redirect rejected")
	errBodyTooLarge     = errors.New("monotonic head response body is too large")
)

// Config pins one enrolled installation namespace to one HTTPS witness.
// RootCAs is mandatory: this adapter never falls back to the host trust store
// and never reads proxy configuration from the process environment.
type Config struct {
	Endpoint           string
	InstallationID     string
	EnrollmentID       string
	Namespace          string
	AuthorityKeyID     string
	AuthorityPublicKey ed25519.PublicKey
	WitnessKeyID       string
	WitnessPublicKey   ed25519.PublicKey
	RootCAs            *x509.CertPool
	ClientCertificates []tls.Certificate
	ServerName         string
	Timeout            time.Duration
}

type Client struct {
	baseURL            url.URL
	installationID     string
	enrollmentID       string
	namespace          string
	authorityKeyID     string
	authorityPublicKey ed25519.PublicKey
	witnessKeyID       string
	witnessPublicKey   ed25519.PublicKey
	httpClient         *http.Client

	mu            sync.Mutex
	heads         map[headKey][]byte
	headOrder     []headKey
	mutations     map[string]mutationRecord
	mutationOrder []string
}

type headKey struct {
	namespace  string
	generation uint64
}

type mutationRecord struct {
	request []byte
	receipt []byte
}

type errorEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	Code          string `json:"code"`
}

var _ monotonichead.Witness = (*Client)(nil)
var _ monotonichead.MutationRecoveryWitness = (*Client)(nil)

func New(config Config) (*Client, error) {
	baseURL, err := validateEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if !canonicalSHA256(config.InstallationID) || !canonicalSHA256(config.EnrollmentID) ||
		!validNamespace(config.Namespace) {
		return nil, errors.New("monotonic head enrollment identity is invalid")
	}
	authorityKey := append(ed25519.PublicKey(nil), config.AuthorityPublicKey...)
	witnessKey := append(ed25519.PublicKey(nil), config.WitnessPublicKey...)
	if len(authorityKey) != ed25519.PublicKeySize || config.AuthorityKeyID != sha256Hex(authorityKey) {
		return nil, errors.New("monotonic head installation authority is invalid")
	}
	if len(witnessKey) != ed25519.PublicKeySize || config.WitnessKeyID != sha256Hex(witnessKey) {
		return nil, errors.New("monotonic head witness authority is invalid")
	}
	if config.RootCAs == nil {
		return nil, errors.New("monotonic head witness roots are required")
	}
	serverName := strings.TrimSpace(config.ServerName)
	if serverName != config.ServerName || strings.ContainsAny(serverName, "\x00/\\") {
		return nil, errors.New("monotonic head TLS server name is invalid")
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 || timeout > time.Minute {
		return nil, errors.New("monotonic head request timeout is invalid")
	}
	certificates, err := cloneCertificates(config.ClientCertificates)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		MinVersion:       tls.VersionTLS13,
		RootCAs:          config.RootCAs.Clone(),
		Certificates:     certificates,
		ServerName:       serverName,
		VerifyConnection: verifyWitnessServerConnectionV1,
	}
	transport := &http.Transport{
		Proxy:       nil,
		DialContext: (&net.Dialer{Timeout: minDuration(timeout, 5*time.Second), KeepAlive: 30 * time.Second}).DialContext,
		// Witness traffic is low-volume authority traffic. A fresh TLS 1.3
		// handshake per operation prevents a pooled connection from surviving
		// past the enrolled server certificate's validity window.
		ForceAttemptHTTP2:      false,
		DisableKeepAlives:      true,
		MaxIdleConns:           8,
		MaxIdleConnsPerHost:    4,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    minDuration(timeout, 5*time.Second),
		ExpectContinueTimeout:  time.Second,
		ResponseHeaderTimeout:  timeout,
		DisableCompression:     true,
		MaxResponseHeaderBytes: responseHeaderLimit,
		TLSClientConfig:        tlsConfig,
	}
	return &Client{
		baseURL:            baseURL,
		installationID:     config.InstallationID,
		enrollmentID:       config.EnrollmentID,
		namespace:          config.Namespace,
		authorityKeyID:     config.AuthorityKeyID,
		authorityPublicKey: authorityKey,
		witnessKeyID:       config.WitnessKeyID,
		witnessPublicKey:   witnessKey,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errRedirectRejected
			},
		},
		heads:         make(map[headKey][]byte),
		headOrder:     make([]headKey, 0, headReplayWindow),
		mutations:     make(map[string]mutationRecord),
		mutationOrder: make([]string, 0, mutationReplayWindow),
	}, nil
}

func (client *Client) Observe(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error) {
	if ctx == nil {
		return domainsecurity.MonotonicHeadObservationV1{}, monotonichead.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	if err := client.validateObserveRequest(request); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, fmt.Errorf("%w: outgoing observe request", monotonichead.ErrInvalidReceipt)
	}
	body, err := domainsecurity.MonotonicHeadObserveRequestV1Bytes(request)
	if err != nil || len(body) > requestBodyLimit {
		return domainsecurity.MonotonicHeadObservationV1{}, fmt.Errorf("%w: outgoing observe request", monotonichead.ErrInvalidReceipt)
	}
	responseBody, status, requestErr := client.post(ctx, ObservePath, body, false)
	if requestErr != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, requestErr
	}
	if status != http.StatusOK {
		return domainsecurity.MonotonicHeadObservationV1{}, mapErrorResponse(status, responseBody, false)
	}
	observation, err := parseCanonicalObservation(responseBody)
	if err != nil || domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
		observation, request,
		client.installationID, client.authorityKeyID, client.authorityPublicKey,
		client.enrollmentID, client.witnessKeyID, client.witnessPublicKey,
	) != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, monotonichead.ErrInvalidReceipt
	}
	if err := client.recordHead(observation.Checkpoint); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	return observation, nil
}

func (client *Client) Advance(ctx context.Context, request domainsecurity.MonotonicHeadAdvanceRequestV1) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	if ctx == nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	if err := client.validateAdvanceRequest(request); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, fmt.Errorf("%w: outgoing advance request", monotonichead.ErrInvalidReceipt)
	}
	body, err := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(request)
	if err != nil || len(body) > requestBodyLimit {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, fmt.Errorf("%w: outgoing advance request", monotonichead.ErrInvalidReceipt)
	}
	if err := client.reserveMutation(request.MutationID, body); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	responseBody, status, requestErr := client.post(ctx, AdvancePath, body, true)
	if requestErr != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, requestErr
	}
	if status != http.StatusOK {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, mapErrorResponse(status, responseBody, true)
	}
	receipt, err := parseCanonicalReceipt(responseBody)
	if err != nil || client.validateReceiptForRequest(receipt, request) != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, indeterminateInvalidReceipt()
	}
	if err := client.recordAdvance(request.MutationID, body, responseBody, receipt.Checkpoint); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	return receipt, nil
}

func (client *Client) ResolveMutation(
	ctx context.Context,
	request domainsecurity.MonotonicHeadMutationResolveRequestV1,
) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
	if ctx == nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonichead.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, err
	}
	if err := client.validateMutationResolveRequest(request); err != nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, fmt.Errorf("%w: outgoing mutation resolve request", monotonichead.ErrInvalidReceipt)
	}
	body, err := domainsecurity.MonotonicHeadMutationResolveRequestV1Bytes(request)
	if err != nil || len(body) > requestBodyLimit {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, fmt.Errorf("%w: outgoing mutation resolve request", monotonichead.ErrInvalidReceipt)
	}
	responseBody, status, requestErr := client.post(ctx, MutationResolvePath, body, false)
	if requestErr != nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, requestErr
	}
	if status != http.StatusOK {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, mapErrorResponse(status, responseBody, false)
	}
	resolution, err := domainsecurity.ParseMonotonicHeadMutationResolutionV1(responseBody)
	if err != nil || domainsecurity.ValidateMonotonicHeadMutationResolutionForRequestV1(
		resolution,
		request,
		client.installationID,
		client.authorityKeyID,
		client.authorityPublicKey,
		client.enrollmentID,
		client.witnessKeyID,
		client.witnessPublicKey,
	) != nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonichead.ErrInvalidReceipt
	}
	if err := client.recordHead(resolution.CurrentCheckpoint); err != nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, err
	}
	if resolution.Status == domainsecurity.MonotonicHeadMutationResolutionCommittedV1 {
		committed := resolution.Committed
		requestBytes, requestBytesErr := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(committed.AdvanceRequest)
		receiptBytes, receiptBytesErr := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(committed.AdvanceReceipt)
		if requestBytesErr != nil || receiptBytesErr != nil {
			return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonichead.ErrInvalidReceipt
		}
		if err := client.recordAdvance(
			committed.AdvanceRequest.MutationID,
			requestBytes,
			receiptBytes,
			committed.AdvanceReceipt.Checkpoint,
		); err != nil {
			return domainsecurity.MonotonicHeadMutationResolutionV1{}, err
		}
	}
	return resolution, nil
}

func (client *Client) validateObserveRequest(request domainsecurity.MonotonicHeadObserveRequestV1) error {
	if err := domainsecurity.ValidateMonotonicHeadObserveRequestForInstallationV1(
		request, client.installationID, client.authorityKeyID, client.authorityPublicKey,
	); err != nil {
		return err
	}
	if request.EnrollmentID != client.enrollmentID || request.Namespace != client.namespace {
		return errors.New("observe request enrollment mismatch")
	}
	return nil
}

func (client *Client) validateAdvanceRequest(request domainsecurity.MonotonicHeadAdvanceRequestV1) error {
	if err := domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(
		request, client.installationID, client.authorityKeyID, client.authorityPublicKey,
	); err != nil {
		return err
	}
	if request.EnrollmentID != client.enrollmentID || request.Namespace != client.namespace {
		return errors.New("advance request enrollment mismatch")
	}
	return nil
}

func (client *Client) validateMutationResolveRequest(request domainsecurity.MonotonicHeadMutationResolveRequestV1) error {
	if err := domainsecurity.ValidateMonotonicHeadMutationResolveRequestForInstallationV1(
		request,
		client.installationID,
		client.authorityKeyID,
		client.authorityPublicKey,
	); err != nil {
		return err
	}
	if request.AdvanceRequest.EnrollmentID != client.enrollmentID || request.AdvanceRequest.Namespace != client.namespace {
		return errors.New("mutation resolve request enrollment mismatch")
	}
	return nil
}

func (client *Client) validateReceiptForRequest(receipt domainsecurity.MonotonicHeadAdvanceReceiptV1, request domainsecurity.MonotonicHeadAdvanceRequestV1) error {
	if err := domainsecurity.ValidateMonotonicHeadAdvanceReceiptV1(receipt); err != nil {
		return err
	}
	checkpoint := receipt.Checkpoint
	if err := domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(
		checkpoint, client.installationID, client.enrollmentID, client.witnessKeyID, client.witnessPublicKey,
	); err != nil {
		return err
	}
	if receipt.RequestDigest != request.RequestDigest || receipt.MutationID != request.MutationID ||
		checkpoint.Namespace != client.namespace || checkpoint.Namespace != request.Namespace ||
		checkpoint.Generation != request.NextGeneration || checkpoint.CurrentStateDigest != request.NextStateDigest ||
		checkpoint.PreviousStateDigest != request.ExpectedStateDigest ||
		checkpoint.PreviousCheckpointDigest != request.ExpectedCheckpointDigest ||
		checkpoint.MutationID != request.MutationID || checkpoint.FenceNonce == request.ExpectedFenceNonce {
		return errors.New("advance receipt request binding mismatch")
	}
	return nil
}

func (client *Client) post(ctx context.Context, path string, body []byte, mutation bool) ([]byte, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	target := client.baseURL
	target.Path = path
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, mapTransportError(ctx, false, mutation, err)
	}
	// Prevent net/http from replaying this POST even if transport policy changes.
	request.GetBody = nil
	request.Header.Set("Accept", protocolContentType)
	request.Header.Set("Content-Type", protocolContentType)
	request.Header.Set("User-Agent", "analytix-monotonic-head/1")

	var mayHaveDispatched atomic.Bool
	trace := &httptrace.ClientTrace{
		WroteHeaders:         func() { mayHaveDispatched.Store(true) },
		WroteRequest:         func(httptrace.WroteRequestInfo) { mayHaveDispatched.Store(true) },
		GotFirstResponseByte: func() { mayHaveDispatched.Store(true) },
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	response, err := client.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, errRedirectRejected) {
			if mutation && mayHaveDispatched.Load() {
				return nil, 0, indeterminateInvalidReceipt()
			}
			return nil, 0, monotonichead.ErrInvalidReceipt
		}
		return nil, 0, mapTransportError(ctx, mayHaveDispatched.Load(), mutation, err)
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil || response.Request.URL.String() != target.String() ||
		response.Header.Get("Content-Encoding") != "" || !exactJSONContentType(response.Header.Get("Content-Type")) {
		if mutation {
			return nil, response.StatusCode, indeterminateInvalidReceipt()
		}
		return nil, response.StatusCode, monotonichead.ErrInvalidReceipt
	}
	responseBody, readErr := readBounded(response.Body, responseBodyLimit)
	if readErr != nil {
		if errors.Is(readErr, errBodyTooLarge) {
			if mutation {
				return nil, response.StatusCode, indeterminateInvalidReceipt()
			}
			return nil, response.StatusCode, monotonichead.ErrInvalidReceipt
		}
		return nil, response.StatusCode, mapTransportError(ctx, true, mutation, readErr)
	}
	return responseBody, response.StatusCode, nil
}

func mapTransportError(ctx context.Context, dispatched, mutation bool, cause error) error {
	if mutation && dispatched {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(monotonichead.ErrIndeterminate, ctxErr)
		}
		return fmt.Errorf("%w: witness transport ended after dispatch", monotonichead.ErrIndeterminate)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if mutation && errors.Is(cause, context.Canceled) {
		return context.Canceled
	}
	return monotonichead.ErrUnavailable
}

func mapErrorResponse(status int, body []byte, mutation bool) error {
	envelope, err := parseCanonicalError(body)
	if err != nil {
		if mutation {
			return indeterminateInvalidReceipt()
		}
		return monotonichead.ErrInvalidReceipt
	}
	switch envelope.Code {
	case "not_enrolled":
		if status == http.StatusNotFound {
			return monotonichead.ErrNotEnrolled
		}
	case "cas_conflict":
		if mutation && status == http.StatusConflict {
			return monotonichead.ErrCASConflict
		}
	case "mutation_conflict":
		if mutation && status == http.StatusConflict {
			return monotonichead.ErrMutationConflict
		}
	case "unavailable":
		if status == http.StatusServiceUnavailable {
			if mutation {
				return errors.Join(monotonichead.ErrIndeterminate, monotonichead.ErrUnavailable)
			}
			return monotonichead.ErrUnavailable
		}
	case "invalid_receipt":
		if status == http.StatusUnprocessableEntity {
			if mutation {
				return indeterminateInvalidReceipt()
			}
			return monotonichead.ErrInvalidReceipt
		}
	}
	if mutation {
		return indeterminateInvalidReceipt()
	}
	return monotonichead.ErrInvalidReceipt
}

func indeterminateInvalidReceipt() error {
	return errors.Join(monotonichead.ErrIndeterminate, monotonichead.ErrInvalidReceipt)
}

func parseCanonicalObservation(body []byte) (domainsecurity.MonotonicHeadObservationV1, error) {
	observation, err := domainsecurity.ParseMonotonicHeadObservationV1(body)
	if err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	canonical, err := domainsecurity.MonotonicHeadObservationV1Bytes(observation)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainsecurity.MonotonicHeadObservationV1{}, errors.New("observation JSON is not canonical")
	}
	return observation, nil
}

func parseCanonicalReceipt(body []byte) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	receipt, err := domainsecurity.ParseMonotonicHeadAdvanceReceiptV1(body)
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	canonical, err := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(receipt)
	if err != nil || !bytes.Equal(canonical, body) {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.New("receipt JSON is not canonical")
	}
	return receipt, nil
}

func parseCanonicalError(body []byte) (errorEnvelope, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: responseBodyLimit, MaxDepth: 2, MaxTokens: 16, MaxStringBytes: 128,
	}); err != nil {
		return errorEnvelope{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || len(raw) != 3 {
		return errorEnvelope{}, errors.New("witness error envelope shape is invalid")
	}
	for _, name := range []string{"schemaVersion", "purpose", "code"} {
		if _, ok := raw[name]; !ok {
			return errorEnvelope{}, errors.New("witness error envelope is incomplete")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var envelope errorEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return errorEnvelope{}, err
	}
	if envelope.SchemaVersion != errorSchemaVersion || envelope.Purpose != errorPurpose {
		return errorEnvelope{}, errors.New("witness error envelope identity is invalid")
	}
	canonical, err := json.Marshal(envelope)
	if err != nil || !bytes.Equal(canonical, body) {
		return errorEnvelope{}, errors.New("witness error envelope is not canonical")
	}
	return envelope, nil
}

func (client *Client) reserveMutation(mutationID string, request []byte) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	known, ok := client.mutations[mutationID]
	if ok && !bytes.Equal(known.request, request) {
		return monotonichead.ErrMutationConflict
	}
	if !ok {
		client.rememberMutationLocked(mutationID, mutationRecord{request: append([]byte(nil), request...)})
	}
	return nil
}

func (client *Client) recordHead(checkpoint domainsecurity.MonotonicHeadCheckpointV1) error {
	canonical, err := domainsecurity.MonotonicHeadCheckpointV1Bytes(checkpoint)
	if err != nil {
		return monotonichead.ErrInvalidReceipt
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.recordHeadLocked(checkpoint, canonical)
}

func (client *Client) recordAdvance(mutationID string, request, receipt []byte, checkpoint domainsecurity.MonotonicHeadCheckpointV1) error {
	canonicalHead, err := domainsecurity.MonotonicHeadCheckpointV1Bytes(checkpoint)
	if err != nil {
		return monotonichead.ErrInvalidReceipt
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if err := client.recordHeadLocked(checkpoint, canonicalHead); err != nil {
		return err
	}
	if known, ok := client.mutations[mutationID]; ok {
		if !bytes.Equal(known.request, request) {
			return monotonichead.ErrMutationConflict
		}
		if len(known.receipt) != 0 && !bytes.Equal(known.receipt, receipt) {
			return monotonichead.ErrInvalidReceipt
		}
		if len(known.receipt) == 0 {
			known.receipt = append([]byte(nil), receipt...)
			client.mutations[mutationID] = known
		}
		return nil
	}
	client.rememberMutationLocked(mutationID, mutationRecord{
		request: append([]byte(nil), request...),
		receipt: append([]byte(nil), receipt...),
	})
	return nil
}

func (client *Client) recordHeadLocked(checkpoint domainsecurity.MonotonicHeadCheckpointV1, canonical []byte) error {
	key := headKey{namespace: checkpoint.Namespace, generation: checkpoint.Generation}
	if known, ok := client.heads[key]; ok && !bytes.Equal(known, canonical) {
		return monotonichead.ErrEquivocation
	}
	if checkpoint.Generation > 0 {
		if previousBytes, ok := client.heads[headKey{namespace: checkpoint.Namespace, generation: checkpoint.Generation - 1}]; ok {
			previous, parseErr := domainsecurity.ParseMonotonicHeadCheckpointV1(previousBytes)
			if parseErr != nil || checkpoint.PreviousCheckpointDigest != previous.CheckpointDigest ||
				checkpoint.PreviousStateDigest != previous.CurrentStateDigest || checkpoint.WitnessKeyID != previous.WitnessKeyID {
				return monotonichead.ErrEquivocation
			}
		}
	}
	if checkpoint.Generation != ^uint64(0) {
		if nextBytes, ok := client.heads[headKey{namespace: checkpoint.Namespace, generation: checkpoint.Generation + 1}]; ok {
			next, parseErr := domainsecurity.ParseMonotonicHeadCheckpointV1(nextBytes)
			if parseErr != nil || next.PreviousCheckpointDigest != checkpoint.CheckpointDigest ||
				next.PreviousStateDigest != checkpoint.CurrentStateDigest || next.WitnessKeyID != checkpoint.WitnessKeyID {
				return monotonichead.ErrEquivocation
			}
		}
	}
	if _, ok := client.heads[key]; !ok {
		client.heads[key] = append([]byte(nil), canonical...)
		client.headOrder = append(client.headOrder, key)
		if len(client.headOrder) > headReplayWindow {
			evicted := client.headOrder[0]
			client.headOrder = client.headOrder[1:]
			delete(client.heads, evicted)
		}
	}
	return nil
}

func (client *Client) rememberMutationLocked(mutationID string, record mutationRecord) {
	if _, exists := client.mutations[mutationID]; !exists {
		client.mutationOrder = append(client.mutationOrder, mutationID)
	}
	client.mutations[mutationID] = record
	if len(client.mutationOrder) <= mutationReplayWindow {
		return
	}
	evicted := client.mutationOrder[0]
	client.mutationOrder = client.mutationOrder[1:]
	delete(client.mutations, evicted)
}

func validateEndpoint(endpoint string) (url.URL, error) {
	if strings.TrimSpace(endpoint) != endpoint || endpoint == "" {
		return url.URL{}, errors.New("monotonic head endpoint is invalid")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" {
		return url.URL{}, errors.New("monotonic head endpoint must be an HTTPS origin")
	}
	parsed.Path = ""
	return *parsed, nil
}

func cloneCertificates(certificates []tls.Certificate) ([]tls.Certificate, error) {
	cloned := make([]tls.Certificate, len(certificates))
	for index := range certificates {
		certificate := certificates[index]
		cloned[index] = certificate
		cloned[index].Certificate = make([][]byte, len(certificate.Certificate))
		for certIndex := range certificate.Certificate {
			cloned[index].Certificate[certIndex] = append([]byte(nil), certificate.Certificate[certIndex]...)
		}
		cloned[index].OCSPStaple = append([]byte(nil), certificate.OCSPStaple...)
		cloned[index].SignedCertificateTimestamps = make([][]byte, len(certificate.SignedCertificateTimestamps))
		for timestampIndex := range certificate.SignedCertificateTimestamps {
			cloned[index].SignedCertificateTimestamps[timestampIndex] = append([]byte(nil), certificate.SignedCertificateTimestamps[timestampIndex]...)
		}
		if len(certificate.Certificate) == 0 || certificate.PrivateKey == nil {
			return nil, errors.New("monotonic head client certificate is incomplete")
		}
	}
	return cloned, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errBodyTooLarge
	}
	return body, nil
}

func exactJSONContentType(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), protocolContentType)
}

func canonicalSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func validNamespace(namespace string) bool {
	return namespace == domainsecurity.ThreadRiskAuthorityNamespaceV1 ||
		namespace == domainsecurity.EvidenceRegistryAuthorityNamespaceV1
}

func minDuration(first, second time.Duration) time.Duration {
	if first < second {
		return first
	}
	return second
}
