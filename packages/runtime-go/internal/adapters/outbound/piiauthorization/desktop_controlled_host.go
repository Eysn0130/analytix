package piiauthorization

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

const (
	desktopControlledHostAdmissionPathV1 = "/v1/controlled-artifacts/admission"
	desktopControlledHostReleasePathV1   = "/v1/controlled-artifacts/release"

	desktopControlledHostAdmissionPurposeV1  = "analytix.controlled-artifact-admission/v1"
	desktopControlledHostReleasePurposeV1    = "analytix.controlled-artifact-release/v1"
	desktopControlledHostReleaseAckPurposeV1 = "analytix.controlled-artifact-release-ack/v1"

	desktopControlledHostJSONMediaTypeV1    = "application/vnd.analytix.controlled-artifact-host+json"
	desktopControlledHostReleaseMediaTypeV1 = "application/vnd.analytix.controlled-artifact-release-v1"

	desktopControlledHostProtocolVersionV1  = 1
	desktopControlledHostSecretBytesV1      = 32
	desktopControlledHostMaxJSONBytesV1     = 64 << 10
	desktopControlledHostMaxMetadataBytesV1 = domainpii.MaxControlledArtifactAccessRecordBytesV1 + 4096
)

var (
	desktopControlledHostReleaseMagicV1 = [8]byte{'A', 'N', 'X', 'C', 'R', 'V', '1', '\n'}
	desktopControlledHostAckDomainV1    = []byte("analytix.controlled-artifact-release-ack/hmac/v1\x00")

	ErrDesktopControlledHostUnavailable = errors.New("desktop controlled artifact host is unavailable")
	ErrDesktopControlledHostRejected    = errors.New("desktop controlled artifact host rejected the request")
	ErrDesktopControlledHostProtocol    = errors.New("desktop controlled artifact host protocol is invalid")
)

type DesktopControlledHostConfigV1 struct {
	Origin           string
	Secret           string
	AdmissionTimeout time.Duration
	ReleaseTimeout   time.Duration
}

// DesktopControlledHostClientV1 is both the idempotent desktop admission
// authority and the synchronous trusted release sink. It is deliberately
// separate from the ordinary runtime HTTP client so protected bytes cannot
// inherit redirects, proxies, response sanitizers, or renderer routing.
type DesktopControlledHostClientV1 struct {
	origin           string
	targetAddress    string
	secret           string
	secretBytes      []byte
	client           *http.Client
	admissionTimeout time.Duration
	releaseTimeout   time.Duration
}

var _ piiauthorizationport.ControlledAccessAdmissionAuthority = (*DesktopControlledHostClientV1)(nil)
var _ piiauthorizationport.ControlledReleaseSink = (*DesktopControlledHostClientV1)(nil)

func NewDesktopControlledHostClientV1(config DesktopControlledHostConfigV1) (*DesktopControlledHostClientV1, error) {
	origin, targetAddress, err := validateDesktopControlledHostOriginV1(config.Origin)
	secretBytes, secretErr := decodeDesktopControlledHostSecretV1(config.Secret)
	if err != nil || secretErr != nil {
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
		Proxy:                 nil,
		ForceAttemptHTTP2:     false,
		DisableCompression:    true,
		DisableKeepAlives:     true,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   0,
		ResponseHeaderTimeout: releaseTimeout,
		ExpectContinueTimeout: time.Second,
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			if network != "tcp" || address != targetAddress {
				return nil, ErrDesktopControlledHostUnavailable
			}
			return dialer.DialContext(ctx, network, address)
		},
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return ErrDesktopControlledHostRejected
		},
	}
	return &DesktopControlledHostClientV1{
		origin: origin, targetAddress: targetAddress, secret: config.Secret,
		secretBytes: append([]byte(nil), secretBytes...), client: client,
		admissionTimeout: admissionTimeout, releaseTimeout: releaseTimeout,
	}, nil
}

func (client *DesktopControlledHostClientV1) Close() {
	if client == nil {
		return
	}
	if client.client != nil {
		client.client.CloseIdleConnections()
	}
	clearDesktopControlledHostBytesV1(client.secretBytes)
	client.secretBytes = nil
	client.secret = ""
}

type desktopControlledHostAdmissionRequestV1 struct {
	SchemaVersion      int    `json:"schemaVersion"`
	Purpose            string `json:"purpose"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	CaseID             string `json:"caseId"`
	CaseBindingHash    string `json:"caseBindingHash"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	ContextDigest      string `json:"contextDigest"`
	AccessAction       string `json:"accessAction"`
	ControlledHandle   string `json:"controlledHandle"`
	UseSlot            string `json:"useSlot"`
	RendererPrincipal  string `json:"rendererPrincipal"`
	RendererGeneration uint64 `json:"rendererGeneration"`
	BackendGeneration  uint64 `json:"backendGeneration"`
}

type desktopControlledHostAdmissionResponseV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	ControlledHandleDigest  string `json:"controlledHandleDigest"`
	UseSlotDigest           string `json:"useSlotDigest"`
	RendererPrincipalDigest string `json:"rendererPrincipalDigest"`
	RendererGeneration      uint64 `json:"rendererGeneration"`
	BackendGeneration       uint64 `json:"backendGeneration"`
	PublicationCommitDigest string `json:"publicationCommitDigest"`
	AuthorizedUntil         string `json:"authorizedUntil"`
}

func (client *DesktopControlledHostClientV1) ResolveCurrent(
	ctx context.Context,
	request piiauthorizationport.ControlledAccessAdmissionRequestV1,
) (piiauthorizationport.ControlledAccessAdmissionV1, error) {
	if client == nil || client.client == nil || ctx == nil || ctx.Err() != nil || len(client.secretBytes) != desktopControlledHostSecretBytesV1 ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(request.SecurityContext) != nil {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, ErrDesktopControlledHostUnavailable
	}
	handleDigest, handleErr := domainpii.ControlledAccessHandleDigestV1(request.ControlledHandle)
	useSlotDigest, slotErr := domainpii.ControlledAccessUseSlotDigestV1(request.UseSlot)
	principalDigest, principalErr := domainpii.ControlledAccessRendererPrincipalDigestV1(request.RendererPrincipal)
	if handleErr != nil || slotErr != nil || principalErr != nil || request.RendererGeneration == 0 || request.BackendGeneration == 0 ||
		(request.AccessAction != domainpii.ControlledArtifactAccessActionDisplayV1 && request.AccessAction != domainpii.ControlledArtifactAccessActionExportV1) {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, ErrDesktopControlledHostRejected
	}
	wireRequest := desktopControlledHostAdmissionRequestV1{
		SchemaVersion: desktopControlledHostProtocolVersionV1, Purpose: desktopControlledHostAdmissionPurposeV1,
		ThreadID: request.SecurityContext.ThreadID, TurnID: request.SecurityContext.TurnID, CaseID: request.SecurityContext.CaseID,
		CaseBindingHash: request.SecurityContext.CaseBindingHash, DatasetSnapshotID: request.SecurityContext.DatasetSnapshotID,
		SourceManifestHash: request.SecurityContext.SourceManifestHash, ContextEpoch: request.SecurityContext.ContextEpoch,
		ContextDigest: request.SecurityContext.ContextDigest, AccessAction: request.AccessAction,
		ControlledHandle: request.ControlledHandle, UseSlot: request.UseSlot, RendererPrincipal: request.RendererPrincipal,
		RendererGeneration: request.RendererGeneration, BackendGeneration: request.BackendGeneration,
	}
	requestBody, err := json.Marshal(wireRequest)
	if err != nil {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, ErrDesktopControlledHostProtocol
	}
	requestCtx, cancel := context.WithTimeout(ctx, client.admissionTimeout)
	defer cancel()
	responseBody, err := client.post(requestCtx, desktopControlledHostAdmissionPathV1, desktopControlledHostJSONMediaTypeV1, requestBody)
	if err != nil {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, err
	}
	var response desktopControlledHostAdmissionResponseV1
	if err := decodeDesktopControlledHostJSONV1(responseBody, &response); err != nil {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, ErrDesktopControlledHostProtocol
	}
	authorizedUntil, timeErr := time.Parse(time.RFC3339Nano, response.AuthorizedUntil)
	if response.SchemaVersion != desktopControlledHostProtocolVersionV1 || response.Purpose != desktopControlledHostAdmissionPurposeV1 ||
		response.ControlledHandleDigest != handleDigest || response.UseSlotDigest != useSlotDigest ||
		response.RendererPrincipalDigest != principalDigest || response.RendererGeneration != request.RendererGeneration ||
		response.BackendGeneration != request.BackendGeneration || !domainsecurity.IsSHA256Hex(response.PublicationCommitDigest) ||
		timeErr != nil || authorizedUntil.IsZero() || authorizedUntil.UTC().Format(time.RFC3339Nano) != response.AuthorizedUntil {
		return piiauthorizationport.ControlledAccessAdmissionV1{}, ErrDesktopControlledHostRejected
	}
	return piiauthorizationport.ControlledAccessAdmissionV1{
		SecurityContext: request.SecurityContext, AccessAction: request.AccessAction,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest, RendererPrincipalDigest: principalDigest,
		RendererGeneration: request.RendererGeneration, BackendGeneration: request.BackendGeneration,
		PublicationCommitDigest: response.PublicationCommitDigest, AuthorizedUntil: authorizedUntil,
	}, nil
}

type desktopControlledHostReleaseMetadataV1 struct {
	SchemaVersion int                                         `json:"schemaVersion"`
	Purpose       string                                      `json:"purpose"`
	Receipt       domainpii.ControlledArtifactAccessReceiptV1 `json:"receipt"`
}

type desktopControlledHostReleaseAckV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	Committed               bool   `json:"committed"`
	AccessID                string `json:"accessId"`
	AccessReceiptDigest     string `json:"accessReceiptDigest"`
	ArtifactSHA256          string `json:"artifactSha256"`
	ReleasedByteLength      uint64 `json:"releasedByteLength"`
	AccessAction            string `json:"accessAction"`
	ControlledHandleDigest  string `json:"controlledHandleDigest"`
	UseSlotDigest           string `json:"useSlotDigest"`
	RendererPrincipalDigest string `json:"rendererPrincipalDigest"`
	RendererGeneration      uint64 `json:"rendererGeneration"`
	BackendGeneration       uint64 `json:"backendGeneration"`
	CommittedAt             string `json:"committedAt"`
	MAC                     string `json:"mac"`
}

func (client *DesktopControlledHostClientV1) Release(
	ctx context.Context,
	request piiauthorizationport.ControlledReleaseRequestV1,
) (piiauthorizationport.ControlledReleaseResultV1, error) {
	if client == nil || client.client == nil || ctx == nil || ctx.Err() != nil || request.Body == nil ||
		len(client.secretBytes) != desktopControlledHostSecretBytesV1 ||
		domainpii.ValidateControlledArtifactAccessReceiptV1(request.Receipt) != nil ||
		request.ArtifactByteLength == 0 || request.ArtifactByteLength != request.Receipt.ArtifactByteLength ||
		request.ArtifactByteLength > domainpii.MaxControlledPIIArtifactBytesV1 {
		return piiauthorizationport.ControlledReleaseResultV1{}, ErrDesktopControlledHostRejected
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, int64(request.ArtifactByteLength)+1))
	if err != nil || uint64(len(body)) != request.ArtifactByteLength || domainsecurity.SHA256Hex(body) != request.Receipt.ArtifactSHA256 {
		clearDesktopControlledHostBytesV1(body)
		return piiauthorizationport.ControlledReleaseResultV1{}, ErrDesktopControlledHostRejected
	}
	defer clearDesktopControlledHostBytesV1(body)
	metadata, err := json.Marshal(desktopControlledHostReleaseMetadataV1{
		SchemaVersion: desktopControlledHostProtocolVersionV1,
		Purpose:       desktopControlledHostReleasePurposeV1,
		Receipt:       request.Receipt,
	})
	if err != nil || len(metadata) == 0 || len(metadata) > desktopControlledHostMaxMetadataBytesV1 {
		return piiauthorizationport.ControlledReleaseResultV1{}, ErrDesktopControlledHostProtocol
	}
	frame := make([]byte, len(desktopControlledHostReleaseMagicV1)+4+len(metadata)+len(body))
	copy(frame, desktopControlledHostReleaseMagicV1[:])
	binary.BigEndian.PutUint32(frame[len(desktopControlledHostReleaseMagicV1):], uint32(len(metadata)))
	copy(frame[len(desktopControlledHostReleaseMagicV1)+4:], metadata)
	copy(frame[len(desktopControlledHostReleaseMagicV1)+4+len(metadata):], body)
	defer clearDesktopControlledHostBytesV1(frame)
	releaseCtx, cancel := context.WithTimeout(ctx, client.releaseTimeout)
	defer cancel()
	responseBody, err := client.post(releaseCtx, desktopControlledHostReleasePathV1, desktopControlledHostReleaseMediaTypeV1, frame)
	if err != nil {
		return piiauthorizationport.ControlledReleaseResultV1{}, err
	}
	var ack desktopControlledHostReleaseAckV1
	if err := decodeDesktopControlledHostJSONV1(responseBody, &ack); err != nil ||
		validateDesktopControlledHostReleaseAckV1(client.secretBytes, request.Receipt, ack) != nil {
		return piiauthorizationport.ControlledReleaseResultV1{}, ErrDesktopControlledHostProtocol
	}
	return piiauthorizationport.ControlledReleaseResultV1{
		Committed: true, ReleasedByteLength: ack.ReleasedByteLength,
	}, nil
}

func (client *DesktopControlledHostClientV1) post(
	ctx context.Context,
	path string,
	contentType string,
	body []byte,
) ([]byte, error) {
	if client == nil || client.client == nil || ctx == nil || ctx.Err() != nil || len(body) == 0 {
		return nil, ErrDesktopControlledHostUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.origin+path, bytes.NewReader(body))
	if err != nil {
		return nil, ErrDesktopControlledHostUnavailable
	}
	request.ContentLength = int64(len(body))
	request.Header.Set("Authorization", "Bearer "+client.secret)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", desktopControlledHostJSONMediaTypeV1)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, errors.Join(ErrDesktopControlledHostUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != desktopControlledHostJSONMediaTypeV1 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, desktopControlledHostMaxJSONBytesV1+1))
		return nil, ErrDesktopControlledHostRejected
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, desktopControlledHostMaxJSONBytesV1+1))
	if err != nil || len(responseBody) == 0 || len(responseBody) > desktopControlledHostMaxJSONBytesV1 {
		return nil, ErrDesktopControlledHostProtocol
	}
	return responseBody, nil
}

func validateDesktopControlledHostOriginV1(raw string) (string, string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return "", "", ErrDesktopControlledHostUnavailable
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", ErrDesktopControlledHostUnavailable
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", "", ErrDesktopControlledHostUnavailable
	}
	target := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if parsed.Host != target {
		return "", "", ErrDesktopControlledHostUnavailable
	}
	return "http://" + target, target, nil
}

func decodeDesktopControlledHostSecretV1(raw string) ([]byte, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || strings.Contains(raw, "=") {
		return nil, ErrDesktopControlledHostUnavailable
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(decoded) != desktopControlledHostSecretBytesV1 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		clearDesktopControlledHostBytesV1(decoded)
		return nil, ErrDesktopControlledHostUnavailable
	}
	return decoded, nil
}

func decodeDesktopControlledHostJSONV1(body []byte, target any) error {
	if target == nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: desktopControlledHostMaxJSONBytesV1,
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
	return nil
}

func validateDesktopControlledHostReleaseAckV1(
	secret []byte,
	receipt domainpii.ControlledArtifactAccessReceiptV1,
	ack desktopControlledHostReleaseAckV1,
) error {
	committedAt, timeErr := time.Parse(time.RFC3339Nano, ack.CommittedAt)
	requestedAt, requestTimeErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, expiryErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	providedMAC, macErr := base64.RawURLEncoding.Strict().DecodeString(ack.MAC)
	expectedMAC := desktopControlledHostReleaseAckMACV1(secret, ack)
	defer clearDesktopControlledHostBytesV1(providedMAC)
	defer clearDesktopControlledHostBytesV1(expectedMAC)
	if ack.SchemaVersion != desktopControlledHostProtocolVersionV1 || ack.Purpose != desktopControlledHostReleaseAckPurposeV1 || !ack.Committed ||
		ack.AccessID != receipt.AccessID || ack.AccessReceiptDigest != receipt.RecordDigest ||
		ack.ArtifactSHA256 != receipt.ArtifactSHA256 || ack.ReleasedByteLength != receipt.ArtifactByteLength ||
		ack.AccessAction != receipt.AccessAction || ack.ControlledHandleDigest != receipt.ControlledHandleDigest ||
		ack.UseSlotDigest != receipt.UseSlotDigest || ack.RendererPrincipalDigest != receipt.RendererPrincipalDigest ||
		ack.RendererGeneration != receipt.RendererGeneration || ack.BackendGeneration != receipt.BackendGeneration ||
		timeErr != nil || requestTimeErr != nil || expiryErr != nil || committedAt.Before(requestedAt) || committedAt.After(authorizedUntil) ||
		committedAt.UTC().Format(time.RFC3339Nano) != ack.CommittedAt || macErr != nil || len(providedMAC) != sha256.Size ||
		!hmac.Equal(providedMAC, expectedMAC) {
		return ErrDesktopControlledHostRejected
	}
	return nil
}

func desktopControlledHostReleaseAckMACV1(secret []byte, ack desktopControlledHostReleaseAckV1) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(desktopControlledHostAckDomainV1)
	for _, value := range []string{
		strconv.Itoa(ack.SchemaVersion), ack.Purpose, strconv.FormatBool(ack.Committed), ack.AccessID,
		ack.AccessReceiptDigest, ack.ArtifactSHA256, strconv.FormatUint(ack.ReleasedByteLength, 10),
		ack.AccessAction, ack.ControlledHandleDigest, ack.UseSlotDigest, ack.RendererPrincipalDigest,
		strconv.FormatUint(ack.RendererGeneration, 10), strconv.FormatUint(ack.BackendGeneration, 10), ack.CommittedAt,
	} {
		_, _ = fmt.Fprintf(mac, "%d:", len(value))
		_, _ = mac.Write([]byte(value))
	}
	return mac.Sum(nil)
}

func clearDesktopControlledHostBytesV1(body []byte) {
	for index := range body {
		body[index] = 0
	}
}
