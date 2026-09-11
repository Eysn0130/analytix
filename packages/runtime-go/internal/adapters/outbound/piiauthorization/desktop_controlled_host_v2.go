package piiauthorization

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

const (
	desktopControlledHostAdmissionPathV2 = "/v2/controlled-artifacts/admission"
	desktopControlledHostReleasePathV2   = "/v2/controlled-artifacts/release"
	desktopControlledHostProbePathV2     = "/v2/controlled-artifacts/probe"

	desktopControlledHostAdmissionPurposeV2     = "analytix.controlled-artifact-admission/v2"
	desktopControlledHostProbeRequestPurposeV2  = "analytix.controlled-artifact-host-probe-request/v2"
	desktopControlledHostProbeResponsePurposeV2 = "analytix.controlled-artifact-host-probe-response/v2"
	desktopControlledHostReleasePurposeV2       = "analytix.controlled-artifact-release/v2"
	desktopControlledHostReleaseAckPurposeV2    = "analytix.controlled-artifact-release-ack/v2"

	desktopControlledHostJSONMediaTypeV2    = "application/vnd.analytix.controlled-artifact-host-v2+json"
	desktopControlledHostReleaseMediaTypeV2 = "application/vnd.analytix.controlled-artifact-release-v2"

	desktopControlledHostProtocolVersionV2  = 2
	desktopControlledHostMaxJSONBytesV2     = 64 << 10
	desktopControlledHostMaxMetadataBytesV2 = domainpii.MaxControlledArtifactAccessRecordBytesV2 + 4096
	desktopControlledHostMaxSafeIntegerV2   = uint64(1<<53 - 1)
)

type desktopControlledHostProbeRequestV2 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	ProbeNonce        string `json:"probeNonce"`
	BackendGeneration uint64 `json:"backendGeneration"`
}

type desktopControlledHostProbeResponseV2 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	ProbeNonce        string `json:"probeNonce"`
	BackendGeneration uint64 `json:"backendGeneration"`
	TransportReady    bool   `json:"transportReady"`
	InvocationsActive bool   `json:"invocationsActive"`
}

var (
	desktopControlledHostReleaseMagicV2 = [8]byte{'A', 'N', 'X', 'C', 'R', 'V', '2', '\n'}
	desktopControlledHostAckDomainV2    = []byte("analytix.controlled-artifact-release-ack/hmac/v2\x00")
)

type DesktopControlledHostConfigV2 struct {
	Origin                string
	Secret                string
	BackendGeneration     uint64
	TLSRootCertificateDER []byte
	TLSLeafSPKISHA256     string
	AdmissionTimeout      time.Duration
	ReleaseTimeout        time.Duration
}

// DesktopControlledHostClientV2 is a private, generation-pinned transport.
// It implements only the V2 projected-delivery protocol and never retries or
// falls back to a legacy path after protected bytes may have reached the host.
type DesktopControlledHostClientV2 struct {
	mu                sync.Mutex
	active            sync.WaitGroup
	closed            bool
	origin            string
	targetAddress     string
	secretBytes       []byte
	backendGeneration uint64
	client            *http.Client
	admissionTimeout  time.Duration
	releaseTimeout    time.Duration
}

var _ piiauthorizationport.ControlledAccessAdmissionAuthorityV2 = (*DesktopControlledHostClientV2)(nil)
var _ piiauthorizationport.ControlledReleaseSinkV2 = (*DesktopControlledHostClientV2)(nil)

func NewDesktopControlledHostClientV2(config DesktopControlledHostConfigV2) (*DesktopControlledHostClientV2, error) {
	origin, targetAddress, originErr := validateDesktopControlledHostOriginV2(config.Origin)
	secretBytes, secretErr := decodeDesktopControlledHostSecretV1(config.Secret)
	tlsConfig, tlsErr := newDesktopControlledHostTLSConfigV2(config.TLSRootCertificateDER, config.TLSLeafSPKISHA256)
	if originErr != nil || secretErr != nil || tlsErr != nil || !desktopControlledHostSafePositiveV2(config.BackendGeneration) {
		clearDesktopControlledHostBytesV1(secretBytes)
		return nil, ErrDesktopControlledHostUnavailable
	}
	admissionTimeout := config.AdmissionTimeout
	if admissionTimeout <= 0 {
		admissionTimeout = 3 * time.Second
	}
	releaseTimeout := config.ReleaseTimeout
	if releaseTimeout <= 0 {
		releaseTimeout = 30 * time.Second
	}
	if admissionTimeout > 30*time.Second || releaseTimeout > 2*time.Minute {
		clearDesktopControlledHostBytesV1(secretBytes)
		return nil, ErrDesktopControlledHostUnavailable
	}
	dialer := &net.Dialer{Timeout: admissionTimeout, KeepAlive: -1}
	transport := &http.Transport{
		Proxy:                  nil,
		ForceAttemptHTTP2:      false,
		DisableCompression:     true,
		DisableKeepAlives:      true,
		MaxIdleConns:           0,
		MaxIdleConnsPerHost:    0,
		ResponseHeaderTimeout:  releaseTimeout,
		TLSHandshakeTimeout:    admissionTimeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 16 << 10,
		TLSClientConfig:        tlsConfig,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != targetAddress {
				return nil, ErrDesktopControlledHostUnavailable
			}
			return dialer.DialContext(ctx, network, address)
		},
	}
	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return ErrDesktopControlledHostRejected
		},
	}
	return &DesktopControlledHostClientV2{
		origin: origin, targetAddress: targetAddress,
		secretBytes: secretBytes, backendGeneration: config.BackendGeneration,
		client: httpClient, admissionTimeout: admissionTimeout, releaseTimeout: releaseTimeout,
	}, nil
}

func (client *DesktopControlledHostClientV2) Close() {
	if client == nil {
		return
	}
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		client.active.Wait()
		return
	}
	client.closed = true
	httpClient := client.client
	client.client = nil
	client.mu.Unlock()
	if httpClient != nil {
		httpClient.CloseIdleConnections()
	}
	client.active.Wait()
	client.mu.Lock()
	clearDesktopControlledHostBytesV1(client.secretBytes)
	client.secretBytes = nil
	client.backendGeneration = 0
	client.origin = ""
	client.targetAddress = ""
	client.mu.Unlock()
}

type desktopControlledHostCallV2 struct {
	origin            string
	secretBytes       []byte
	backendGeneration uint64
	client            *http.Client
	admissionTimeout  time.Duration
	releaseTimeout    time.Duration
}

func (client *DesktopControlledHostClientV2) beginCallV2() (desktopControlledHostCallV2, func(), bool) {
	if client == nil {
		return desktopControlledHostCallV2{}, nil, false
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed || client.client == nil || len(client.secretBytes) != desktopControlledHostSecretBytesV1 ||
		!desktopControlledHostSafePositiveV2(client.backendGeneration) {
		return desktopControlledHostCallV2{}, nil, false
	}
	client.active.Add(1)
	return desktopControlledHostCallV2{
		origin: client.origin, secretBytes: client.secretBytes, backendGeneration: client.backendGeneration,
		client: client.client, admissionTimeout: client.admissionTimeout, releaseTimeout: client.releaseTimeout,
	}, client.active.Done, true
}

// ProbeV2 proves the current TLS/token/generation transport is reachable and
// still quiescent. It never grants access and must run before Electron enables
// invocation creation for this backend generation.
func (client *DesktopControlledHostClientV2) ProbeV2(ctx context.Context) error {
	call, done, ok := client.beginCallV2()
	if !ok {
		return ErrDesktopControlledHostUnavailable
	}
	defer done()
	if ctx == nil || ctx.Err() != nil {
		return ErrDesktopControlledHostUnavailable
	}
	nonce := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		clearDesktopControlledHostBytesV1(nonce)
		return ErrDesktopControlledHostUnavailable
	}
	defer clearDesktopControlledHostBytesV1(nonce)
	nonceText := base64.RawURLEncoding.EncodeToString(nonce)
	requestBody, err := json.Marshal(desktopControlledHostProbeRequestV2{
		SchemaVersion: desktopControlledHostProtocolVersionV2,
		Purpose:       desktopControlledHostProbeRequestPurposeV2,
		ProbeNonce:    nonceText, BackendGeneration: call.backendGeneration,
	})
	if err != nil || len(requestBody) == 0 || len(requestBody) > desktopControlledHostMaxJSONBytesV2 {
		return ErrDesktopControlledHostProtocol
	}
	probeCtx, cancel := context.WithTimeout(ctx, call.admissionTimeout)
	defer cancel()
	responseBody, err := call.postV2(
		probeCtx, desktopControlledHostProbePathV2, desktopControlledHostJSONMediaTypeV2, requestBody,
	)
	if err != nil {
		return err
	}
	var response desktopControlledHostProbeResponseV2
	if err := decodeCanonicalDesktopControlledHostJSONV2(responseBody, &response); err != nil ||
		response.SchemaVersion != desktopControlledHostProtocolVersionV2 ||
		response.Purpose != desktopControlledHostProbeResponsePurposeV2 ||
		response.ProbeNonce != nonceText || response.BackendGeneration != call.backendGeneration ||
		!response.TransportReady || response.InvocationsActive {
		return ErrDesktopControlledHostProtocol
	}
	return nil
}

type desktopControlledHostAdmissionRequestV2 struct {
	SchemaVersion      int                        `json:"schemaVersion"`
	Purpose            string                     `json:"purpose"`
	Context            domainpii.ContextBindingV1 `json:"context"`
	AccessAction       string                     `json:"accessAction"`
	ControlledHandle   string                     `json:"controlledHandle"`
	UseSlot            string                     `json:"useSlot"`
	RendererPrincipal  string                     `json:"rendererPrincipal"`
	RendererGeneration uint64                     `json:"rendererGeneration"`
	BackendGeneration  uint64                     `json:"backendGeneration"`
}

type desktopControlledHostAdmissionResponseV2 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	Purpose                     string `json:"purpose"`
	ContextDigest               string `json:"contextDigest"`
	AccessAction                string `json:"accessAction"`
	ControlledHandleDigest      string `json:"controlledHandleDigest"`
	UseSlotDigest               string `json:"useSlotDigest"`
	RendererPrincipalDigest     string `json:"rendererPrincipalDigest"`
	RendererGeneration          uint64 `json:"rendererGeneration"`
	BackendGeneration           uint64 `json:"backendGeneration"`
	DeliveryID                  string `json:"deliveryId"`
	DeliveryOutcomeRecordDigest string `json:"deliveryOutcomeRecordDigest"`
	PublicationCommitDigest     string `json:"publicationCommitDigest"`
	ReleaseTargetIdentityDigest string `json:"releaseTargetIdentityDigest"`
	AuthorizedUntil             string `json:"authorizedUntil"`
}

func (client *DesktopControlledHostClientV2) ResolveCurrentV2(
	ctx context.Context,
	request piiauthorizationport.ControlledAccessAdmissionRequestV2,
) (piiauthorizationport.ControlledAccessAdmissionV2, error) {
	call, done, ok := client.beginCallV2()
	if !ok {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrDesktopControlledHostUnavailable
	}
	defer done()
	if ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(request.SecurityContext) != nil {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrDesktopControlledHostUnavailable
	}
	handleDigest, handleErr := domainpii.ControlledAccessHandleDigestV2(request.ControlledHandle)
	useSlotDigest, slotErr := domainpii.ControlledAccessUseSlotDigestV2(request.UseSlot)
	principalDigest, principalErr := domainpii.ControlledAccessRendererPrincipalDigestV2(request.RendererPrincipal)
	if handleErr != nil || slotErr != nil || principalErr != nil ||
		!desktopControlledHostSafePositiveV2(request.SecurityContext.ContextEpoch) ||
		!desktopControlledHostSafePositiveV2(request.RendererGeneration) ||
		request.BackendGeneration != call.backendGeneration ||
		(request.AccessAction != domainpii.ControlledArtifactAccessActionDisplayV1 &&
			request.AccessAction != domainpii.ControlledArtifactAccessActionExportV1) {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrDesktopControlledHostRejected
	}
	wireRequest := desktopControlledHostAdmissionRequestV2{
		SchemaVersion: desktopControlledHostProtocolVersionV2, Purpose: desktopControlledHostAdmissionPurposeV2,
		Context: controlledHostContextBindingV2(request.SecurityContext), AccessAction: request.AccessAction,
		ControlledHandle: request.ControlledHandle, UseSlot: request.UseSlot, RendererPrincipal: request.RendererPrincipal,
		RendererGeneration: request.RendererGeneration, BackendGeneration: request.BackendGeneration,
	}
	requestBody, err := json.Marshal(wireRequest)
	if err != nil || len(requestBody) == 0 || len(requestBody) > desktopControlledHostMaxJSONBytesV2 {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrDesktopControlledHostProtocol
	}
	requestCtx, cancel := context.WithTimeout(ctx, call.admissionTimeout)
	defer cancel()
	responseBody, err := call.postV2(requestCtx, desktopControlledHostAdmissionPathV2, desktopControlledHostJSONMediaTypeV2, requestBody)
	if err != nil {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, err
	}
	var response desktopControlledHostAdmissionResponseV2
	if err := decodeCanonicalDesktopControlledHostJSONV2(responseBody, &response); err != nil {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrDesktopControlledHostProtocol
	}
	authorizedUntil, timeErr := time.Parse(time.RFC3339Nano, response.AuthorizedUntil)
	if response.SchemaVersion != desktopControlledHostProtocolVersionV2 || response.Purpose != desktopControlledHostAdmissionPurposeV2 ||
		response.ContextDigest != request.SecurityContext.ContextDigest || response.AccessAction != request.AccessAction ||
		response.ControlledHandleDigest != handleDigest || response.UseSlotDigest != useSlotDigest ||
		response.RendererPrincipalDigest != principalDigest || response.RendererGeneration != request.RendererGeneration ||
		response.BackendGeneration != request.BackendGeneration || !domainsecurity.IsSHA256Hex(response.DeliveryID) ||
		!domainsecurity.IsSHA256Hex(response.DeliveryOutcomeRecordDigest) || !domainsecurity.IsSHA256Hex(response.PublicationCommitDigest) ||
		!domainsecurity.IsSHA256Hex(response.ReleaseTargetIdentityDigest) ||
		timeErr != nil || authorizedUntil.IsZero() || authorizedUntil.UTC().Format(time.RFC3339Nano) != response.AuthorizedUntil {
		return piiauthorizationport.ControlledAccessAdmissionV2{}, ErrDesktopControlledHostRejected
	}
	return piiauthorizationport.ControlledAccessAdmissionV2{
		SecurityContext: request.SecurityContext, AccessAction: request.AccessAction,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest, RendererPrincipalDigest: principalDigest,
		RendererGeneration: request.RendererGeneration, BackendGeneration: request.BackendGeneration,
		DeliveryID: response.DeliveryID, DeliveryOutcomeRecordDigest: response.DeliveryOutcomeRecordDigest,
		PublicationCommitDigest:     response.PublicationCommitDigest,
		ReleaseTargetIdentityDigest: response.ReleaseTargetIdentityDigest,
		AuthorizedUntil:             authorizedUntil,
	}, nil
}

type desktopControlledHostReleaseMetadataV2 struct {
	SchemaVersion int                                         `json:"schemaVersion"`
	Purpose       string                                      `json:"purpose"`
	Receipt       domainpii.ControlledArtifactAccessReceiptV2 `json:"receipt"`
}

type desktopControlledHostReleaseAckV2 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	Purpose                     string `json:"purpose"`
	Committed                   bool   `json:"committed"`
	AccessID                    string `json:"accessId"`
	AccessReceiptDigest         string `json:"accessReceiptDigest"`
	ContextDigest               string `json:"contextDigest"`
	DeliveryID                  string `json:"deliveryId"`
	DeliveryOutcomeRecordDigest string `json:"deliveryOutcomeRecordDigest"`
	PublicationCommitDigest     string `json:"publicationCommitDigest"`
	ReleaseTargetIdentityDigest string `json:"releaseTargetIdentityDigest"`
	ArtifactSHA256              string `json:"artifactSha256"`
	ReleasedByteLength          uint64 `json:"releasedByteLength"`
	AccessAction                string `json:"accessAction"`
	ControlledHandleDigest      string `json:"controlledHandleDigest"`
	UseSlotDigest               string `json:"useSlotDigest"`
	RendererPrincipalDigest     string `json:"rendererPrincipalDigest"`
	RendererGeneration          uint64 `json:"rendererGeneration"`
	BackendGeneration           uint64 `json:"backendGeneration"`
	CommittedAt                 string `json:"committedAt"`
	MAC                         string `json:"mac"`
}

func (client *DesktopControlledHostClientV2) ReleaseV2(
	ctx context.Context,
	request piiauthorizationport.ControlledReleaseRequestV2,
) (piiauthorizationport.ControlledReleaseResultV2, error) {
	call, done, ok := client.beginCallV2()
	if !ok {
		return piiauthorizationport.ControlledReleaseResultV2{}, ErrDesktopControlledHostUnavailable
	}
	defer done()
	if ctx == nil || ctx.Err() != nil || request.Body == nil ||
		domainpii.ValidateControlledArtifactAccessReceiptV2(request.Receipt) != nil ||
		!desktopControlledHostSafePositiveV2(request.Receipt.Context.ContextEpoch) ||
		!desktopControlledHostSafePositiveV2(request.Receipt.RendererGeneration) ||
		request.Receipt.BackendGeneration != call.backendGeneration ||
		request.ArtifactByteLength == 0 || request.ArtifactByteLength != request.Receipt.ArtifactByteLength ||
		request.ArtifactByteLength > domainpii.MaxControlledPIIArtifactBytesV1 {
		return piiauthorizationport.ControlledReleaseResultV2{}, ErrDesktopControlledHostRejected
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, int64(request.ArtifactByteLength)+1))
	if err != nil || uint64(len(body)) != request.ArtifactByteLength || domainsecurity.SHA256Hex(body) != request.Receipt.ArtifactSHA256 {
		clearDesktopControlledHostBytesV1(body)
		return piiauthorizationport.ControlledReleaseResultV2{}, ErrDesktopControlledHostRejected
	}
	defer clearDesktopControlledHostBytesV1(body)
	metadata, err := json.Marshal(desktopControlledHostReleaseMetadataV2{
		SchemaVersion: desktopControlledHostProtocolVersionV2,
		Purpose:       desktopControlledHostReleasePurposeV2,
		Receipt:       request.Receipt,
	})
	if err != nil || len(metadata) == 0 || len(metadata) > desktopControlledHostMaxMetadataBytesV2 {
		return piiauthorizationport.ControlledReleaseResultV2{}, ErrDesktopControlledHostProtocol
	}
	frame := make([]byte, len(desktopControlledHostReleaseMagicV2)+4+len(metadata)+len(body))
	copy(frame, desktopControlledHostReleaseMagicV2[:])
	binary.BigEndian.PutUint32(frame[len(desktopControlledHostReleaseMagicV2):], uint32(len(metadata)))
	copy(frame[len(desktopControlledHostReleaseMagicV2)+4:], metadata)
	copy(frame[len(desktopControlledHostReleaseMagicV2)+4+len(metadata):], body)
	defer clearDesktopControlledHostBytesV1(frame)
	releaseCtx, cancel := context.WithTimeout(ctx, call.releaseTimeout)
	defer cancel()
	responseBody, err := call.postV2(releaseCtx, desktopControlledHostReleasePathV2, desktopControlledHostReleaseMediaTypeV2, frame)
	if err != nil {
		return piiauthorizationport.ControlledReleaseResultV2{}, err
	}
	var ack desktopControlledHostReleaseAckV2
	if err := decodeCanonicalDesktopControlledHostJSONV2(responseBody, &ack); err != nil ||
		validateDesktopControlledHostReleaseAckV2(call.secretBytes, request.Receipt, ack) != nil {
		return piiauthorizationport.ControlledReleaseResultV2{}, ErrDesktopControlledHostProtocol
	}
	return piiauthorizationport.ControlledReleaseResultV2{
		Committed: true, ReleasedByteLength: ack.ReleasedByteLength,
	}, nil
}

func (call desktopControlledHostCallV2) postV2(
	ctx context.Context,
	path string,
	contentType string,
	body []byte,
) ([]byte, error) {
	if call.client == nil || ctx == nil || ctx.Err() != nil || len(body) == 0 ||
		len(call.secretBytes) != desktopControlledHostSecretBytesV1 {
		return nil, ErrDesktopControlledHostUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, call.origin+path, bytes.NewReader(body))
	if err != nil {
		return nil, ErrDesktopControlledHostUnavailable
	}
	request.ContentLength = int64(len(body))
	request.GetBody = nil
	request.Header.Set("Authorization", "Bearer "+base64.RawURLEncoding.EncodeToString(call.secretBytes))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", desktopControlledHostJSONMediaTypeV2)
	response, err := call.client.Do(request)
	if err != nil {
		return nil, errors.Join(ErrDesktopControlledHostUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != desktopControlledHostJSONMediaTypeV2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, desktopControlledHostMaxJSONBytesV2+1))
		return nil, ErrDesktopControlledHostRejected
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, desktopControlledHostMaxJSONBytesV2+1))
	if err != nil || len(responseBody) == 0 || len(responseBody) > desktopControlledHostMaxJSONBytesV2 {
		return nil, ErrDesktopControlledHostProtocol
	}
	return responseBody, nil
}

func controlledHostContextBindingV2(value domainsecurity.TurnSecurityContext) domainpii.ContextBindingV1 {
	return domainpii.ContextBindingV1{
		Version: value.Version, ThreadID: value.ThreadID, TurnID: value.TurnID, WorkspaceRealPath: value.WorkspaceRealPath,
		TenantID: value.TenantID, UserID: value.UserID, CaseID: value.CaseID, CaseBindingHash: value.CaseBindingHash,
		DatasetSnapshotID: value.DatasetSnapshotID, SourceManifestHash: value.SourceManifestHash,
		ContextEpoch: value.ContextEpoch, ContextIssuedAt: value.IssuedAt, ContextDigest: value.ContextDigest,
	}
}

func decodeCanonicalDesktopControlledHostJSONV2(body []byte, target any) error {
	if target == nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: desktopControlledHostMaxJSONBytesV2,
		MaxDepth: 4, MaxTokens: 128, MaxStringBytes: 4096,
	}) != nil {
		return ErrDesktopControlledHostProtocol
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrDesktopControlledHostProtocol
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrDesktopControlledHostProtocol
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, body) {
		return ErrDesktopControlledHostProtocol
	}
	return nil
}

func validateDesktopControlledHostReleaseAckV2(
	secret []byte,
	receipt domainpii.ControlledArtifactAccessReceiptV2,
	ack desktopControlledHostReleaseAckV2,
) error {
	committedAt, timeErr := time.Parse(time.RFC3339Nano, ack.CommittedAt)
	requestedAt, requestTimeErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, expiryErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	providedMAC, macErr := base64.RawURLEncoding.Strict().DecodeString(ack.MAC)
	expectedMAC := desktopControlledHostReleaseAckMACV2(secret, ack)
	defer clearDesktopControlledHostBytesV1(providedMAC)
	defer clearDesktopControlledHostBytesV1(expectedMAC)
	if ack.SchemaVersion != desktopControlledHostProtocolVersionV2 || ack.Purpose != desktopControlledHostReleaseAckPurposeV2 || !ack.Committed ||
		ack.AccessID != receipt.AccessID || ack.AccessReceiptDigest != receipt.RecordDigest || ack.ContextDigest != receipt.Context.ContextDigest ||
		ack.DeliveryID != receipt.DeliveryID || ack.DeliveryOutcomeRecordDigest != receipt.DeliveryOutcomeRecordDigest ||
		ack.PublicationCommitDigest != receipt.PublicationCommitDigest ||
		ack.ReleaseTargetIdentityDigest != receipt.ReleaseTargetIdentityDigest ||
		ack.ArtifactSHA256 != receipt.ArtifactSHA256 ||
		ack.ReleasedByteLength != receipt.ArtifactByteLength || ack.AccessAction != receipt.AccessAction ||
		ack.ControlledHandleDigest != receipt.ControlledHandleDigest || ack.UseSlotDigest != receipt.UseSlotDigest ||
		ack.RendererPrincipalDigest != receipt.RendererPrincipalDigest || ack.RendererGeneration != receipt.RendererGeneration ||
		ack.BackendGeneration != receipt.BackendGeneration || timeErr != nil || requestTimeErr != nil || expiryErr != nil ||
		committedAt.Before(requestedAt) || !committedAt.Before(authorizedUntil) ||
		committedAt.UTC().Format(time.RFC3339Nano) != ack.CommittedAt || macErr != nil || len(providedMAC) != sha256.Size ||
		!hmac.Equal(providedMAC, expectedMAC) {
		return ErrDesktopControlledHostRejected
	}
	return nil
}

func desktopControlledHostReleaseAckMACV2(secret []byte, ack desktopControlledHostReleaseAckV2) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(desktopControlledHostAckDomainV2)
	for _, value := range []string{
		strconv.Itoa(ack.SchemaVersion), ack.Purpose, strconv.FormatBool(ack.Committed), ack.AccessID,
		ack.AccessReceiptDigest, ack.ContextDigest, ack.DeliveryID, ack.DeliveryOutcomeRecordDigest,
		ack.PublicationCommitDigest, ack.ReleaseTargetIdentityDigest, ack.ArtifactSHA256,
		strconv.FormatUint(ack.ReleasedByteLength, 10),
		ack.AccessAction, ack.ControlledHandleDigest, ack.UseSlotDigest, ack.RendererPrincipalDigest,
		strconv.FormatUint(ack.RendererGeneration, 10), strconv.FormatUint(ack.BackendGeneration, 10), ack.CommittedAt,
	} {
		_, _ = fmt.Fprintf(mac, "%d:", len(value))
		_, _ = mac.Write([]byte(value))
	}
	return mac.Sum(nil)
}

func desktopControlledHostSafePositiveV2(value uint64) bool {
	return value > 0 && value <= desktopControlledHostMaxSafeIntegerV2
}
