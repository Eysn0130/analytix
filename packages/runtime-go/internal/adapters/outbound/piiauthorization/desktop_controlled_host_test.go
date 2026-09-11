package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const desktopControlledHostTestAccount = "6222020202020202020"

type desktopControlledHostFixtureV1 struct {
	secret     string
	secretBody []byte
	handle     string
	useSlot    string
	principal  string
	context    domainsecurity.TurnSecurityContext
	receipt    domainpii.ControlledArtifactAccessReceiptV1
	body       []byte
	requested  time.Time
	expires    time.Time
}

func TestDesktopControlledHostClientReleasesExactArtifactWithAuthenticatedAck(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV1(t)
	var admissionCalls atomic.Int32
	var releaseCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fixture.secret ||
			r.Header.Get("Accept") != desktopControlledHostJSONMediaTypeV1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case desktopControlledHostAdmissionPathV1:
			admissionCalls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != desktopControlledHostJSONMediaTypeV1 {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			var request desktopControlledHostAdmissionRequestV1
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.ThreadID != fixture.context.ThreadID ||
				request.TurnID != fixture.context.TurnID || request.ContextDigest != fixture.context.ContextDigest ||
				request.ControlledHandle != fixture.handle || request.UseSlot != fixture.useSlot ||
				request.RendererPrincipal != fixture.principal || request.RendererGeneration != fixture.receipt.RendererGeneration ||
				request.BackendGeneration != fixture.receipt.BackendGeneration {
				http.Error(w, "bad admission", http.StatusBadRequest)
				return
			}
			writeDesktopControlledHostJSONV1(t, w, desktopControlledHostAdmissionResponseV1{
				SchemaVersion: desktopControlledHostProtocolVersionV1, Purpose: desktopControlledHostAdmissionPurposeV1,
				ControlledHandleDigest: fixture.receipt.ControlledHandleDigest, UseSlotDigest: fixture.receipt.UseSlotDigest,
				RendererPrincipalDigest: fixture.receipt.RendererPrincipalDigest,
				RendererGeneration:      fixture.receipt.RendererGeneration, BackendGeneration: fixture.receipt.BackendGeneration,
				PublicationCommitDigest: fixture.receipt.PublicationCommitDigest,
				AuthorizedUntil:         fixture.receipt.AuthorizedUntil,
			})
		case desktopControlledHostReleasePathV1:
			releaseCalls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != desktopControlledHostReleaseMediaTypeV1 {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			frame, err := io.ReadAll(r.Body)
			if err != nil || int64(len(frame)) != r.ContentLength || len(frame) < len(desktopControlledHostReleaseMagicV1)+4 ||
				!bytes.Equal(frame[:len(desktopControlledHostReleaseMagicV1)], desktopControlledHostReleaseMagicV1[:]) {
				http.Error(w, "bad frame", http.StatusBadRequest)
				return
			}
			metadataLength := int(binary.BigEndian.Uint32(frame[len(desktopControlledHostReleaseMagicV1):]))
			metadataStart := len(desktopControlledHostReleaseMagicV1) + 4
			bodyStart := metadataStart + metadataLength
			if metadataLength < 1 || bodyStart > len(frame) {
				http.Error(w, "bad metadata", http.StatusBadRequest)
				return
			}
			var metadata desktopControlledHostReleaseMetadataV1
			if err := json.Unmarshal(frame[metadataStart:bodyStart], &metadata); err != nil ||
				metadata.SchemaVersion != desktopControlledHostProtocolVersionV1 ||
				metadata.Purpose != desktopControlledHostReleasePurposeV1 ||
				metadata.Receipt.RecordDigest != fixture.receipt.RecordDigest ||
				!bytes.Equal(frame[bodyStart:], fixture.body) ||
				!bytes.Contains(frame[bodyStart:], []byte(desktopControlledHostTestAccount)) {
				http.Error(w, "wrong artifact", http.StatusBadRequest)
				return
			}
			ack := fixture.releaseAck()
			ack.MAC = base64.RawURLEncoding.EncodeToString(desktopControlledHostReleaseAckMACV1(fixture.secretBody, ack))
			writeDesktopControlledHostJSONV1(t, w, ack)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewDesktopControlledHostClientV1(DesktopControlledHostConfigV1{Origin: server.URL, Secret: fixture.secret})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	admission, err := client.ResolveCurrent(context.Background(), fixture.admissionRequest())
	if err != nil || admission.SecurityContext != fixture.context ||
		admission.PublicationCommitDigest != fixture.receipt.PublicationCommitDigest ||
		!admission.AuthorizedUntil.Equal(fixture.expires) {
		t.Fatalf("desktop host admission did not bind the frozen context: admission=%#v err=%v", admission, err)
	}
	result, err := client.Release(context.Background(), piiauthorizationport.ControlledReleaseRequestV1{
		Receipt: fixture.receipt, Body: bytes.NewReader(fixture.body), ArtifactByteLength: uint64(len(fixture.body)),
	})
	if err != nil || !result.Committed || result.ReleasedByteLength != uint64(len(fixture.body)) ||
		admissionCalls.Load() != 1 || releaseCalls.Load() != 1 {
		t.Fatalf("exact controlled artifact release failed: result=%#v calls=%d/%d err=%v",
			result, admissionCalls.Load(), releaseCalls.Load(), err)
	}
}

func TestDesktopControlledHostClientRejectsNonLoopbackOriginsAndWeakSecrets(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, desktopControlledHostSecretBytesV1))
	for _, test := range []struct {
		name   string
		origin string
		secret string
	}{
		{name: "localhost alias", origin: "http://localhost:1234", secret: secret},
		{name: "ipv6 loopback", origin: "http://[::1]:1234", secret: secret},
		{name: "remote", origin: "http://192.0.2.1:1234", secret: secret},
		{name: "tls or proxyable origin", origin: "https://127.0.0.1:1234", secret: secret},
		{name: "userinfo", origin: "http://user@127.0.0.1:1234", secret: secret},
		{name: "path", origin: "http://127.0.0.1:1234/private", secret: secret},
		{name: "noncanonical port", origin: "http://127.0.0.1:01234", secret: secret},
		{name: "short secret", origin: "http://127.0.0.1:1234", secret: "short-secret"},
		{name: "padded secret", origin: "http://127.0.0.1:1234", secret: secret + "="},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewDesktopControlledHostClientV1(DesktopControlledHostConfigV1{Origin: test.origin, Secret: test.secret})
			if !errors.Is(err, ErrDesktopControlledHostUnavailable) || client != nil {
				t.Fatalf("invalid controlled host configuration was accepted: client=%#v err=%v", client, err)
			}
		})
	}
}

func TestDesktopControlledHostClientRejectsAmbiguousAdmissionAndReleaseResponses(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler func(*desktopControlledHostFixtureV1, http.ResponseWriter, *http.Request)
		phase   string
	}{
		{
			name: "unknown admission field", phase: "admission",
			handler: func(f *desktopControlledHostFixtureV1, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", desktopControlledHostJSONMediaTypeV1)
				_, _ = io.WriteString(w, `{"schemaVersion":1,"purpose":"`+desktopControlledHostAdmissionPurposeV1+`","controlledHandleDigest":"`+f.receipt.ControlledHandleDigest+`","useSlotDigest":"`+f.receipt.UseSlotDigest+`","rendererPrincipalDigest":"`+f.receipt.RendererPrincipalDigest+`","rendererGeneration":7,"backendGeneration":9,"publicationCommitDigest":"`+f.receipt.PublicationCommitDigest+`","authorizedUntil":"`+f.receipt.AuthorizedUntil+`","extra":true}`)
			},
		},
		{
			name: "duplicate admission field", phase: "admission",
			handler: func(f *desktopControlledHostFixtureV1, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", desktopControlledHostJSONMediaTypeV1)
				_, _ = io.WriteString(w, `{"schemaVersion":1,"schemaVersion":1,"purpose":"`+desktopControlledHostAdmissionPurposeV1+`","controlledHandleDigest":"`+f.receipt.ControlledHandleDigest+`","useSlotDigest":"`+f.receipt.UseSlotDigest+`","rendererPrincipalDigest":"`+f.receipt.RendererPrincipalDigest+`","rendererGeneration":7,"backendGeneration":9,"publicationCommitDigest":"`+f.receipt.PublicationCommitDigest+`","authorizedUntil":"`+f.receipt.AuthorizedUntil+`"}`)
			},
		},
		{
			name: "wrong acknowledgement MAC", phase: "release",
			handler: func(f *desktopControlledHostFixtureV1, w http.ResponseWriter, _ *http.Request) {
				ack := f.releaseAck()
				ack.MAC = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x99}, 32))
				writeDesktopControlledHostJSONV1(t, w, ack)
			},
		},
		{
			name: "mismatched acknowledgement length", phase: "release",
			handler: func(f *desktopControlledHostFixtureV1, w http.ResponseWriter, _ *http.Request) {
				ack := f.releaseAck()
				ack.ReleasedByteLength--
				ack.MAC = base64.RawURLEncoding.EncodeToString(desktopControlledHostReleaseAckMACV1(f.secretBody, ack))
				writeDesktopControlledHostJSONV1(t, w, ack)
			},
		},
		{
			name: "redirect", phase: "release",
			handler: func(_ *desktopControlledHostFixtureV1, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1:1/steal")
				w.WriteHeader(http.StatusTemporaryRedirect)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDesktopControlledHostFixtureV1(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == desktopControlledHostAdmissionPathV1 && test.phase != "admission" {
					writeDesktopControlledHostJSONV1(t, w, fixture.admissionResponse())
					return
				}
				test.handler(fixture, w, r)
			}))
			defer server.Close()
			client, err := NewDesktopControlledHostClientV1(DesktopControlledHostConfigV1{Origin: server.URL, Secret: fixture.secret})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if test.phase == "admission" {
				if _, err := client.ResolveCurrent(context.Background(), fixture.admissionRequest()); err == nil {
					t.Fatal("ambiguous desktop admission response was accepted")
				}
				return
			}
			if _, err := client.Release(context.Background(), piiauthorizationport.ControlledReleaseRequestV1{
				Receipt: fixture.receipt, Body: bytes.NewReader(fixture.body), ArtifactByteLength: uint64(len(fixture.body)),
			}); err == nil {
				t.Fatal("ambiguous desktop release acknowledgement was accepted")
			}
		})
	}
}

func TestDesktopControlledHostClientRejectsWrongOrExtraArtifactBytesBeforeNetwork(t *testing.T) {
	fixture := newDesktopControlledHostFixtureV1(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := NewDesktopControlledHostClientV1(DesktopControlledHostConfigV1{Origin: server.URL, Secret: fixture.secret})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	wrong := bytes.Repeat([]byte{'x'}, len(fixture.body))
	extra := append(append([]byte(nil), fixture.body...), 'x')
	for _, body := range [][]byte{wrong, fixture.body[:len(fixture.body)-1], extra} {
		if _, err := client.Release(context.Background(), piiauthorizationport.ControlledReleaseRequestV1{
			Receipt: fixture.receipt, Body: bytes.NewReader(body), ArtifactByteLength: fixture.receipt.ArtifactByteLength,
		}); !errors.Is(err, ErrDesktopControlledHostRejected) {
			t.Fatalf("wrong artifact was not rejected before network: len=%d err=%v", len(body), err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid artifact reached desktop host: calls=%d", calls.Load())
	}
}

func newDesktopControlledHostFixtureV1(t *testing.T) *desktopControlledHostFixtureV1 {
	t.Helper()
	requested := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	expires := requested.Add(10 * time.Minute)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-controlled-host", TurnID: "turn-controlled-host", WorkspaceRealPath: "/workspace/controlled-host",
		CaseID: "case-controlled-host", CaseBindingHash: domainsecurity.SHA256Hex([]byte("controlled-host-binding")),
		ContextEpoch: 4, IssuedAt: requested.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1,"exactValue":"` + desktopControlledHostTestAccount + `"}`)
	handle := "host-controlled-publication-handle-v1"
	useSlot := "host-controlled-single-use-slot-v1"
	principal := "main-frame-renderer-principal-v1"
	handleDigest, _ := domainpii.ControlledAccessHandleDigestV1(handle)
	useSlotDigest, _ := domainpii.ControlledAccessUseSlotDigestV1(useSlot)
	principalDigest, _ := domainpii.ControlledAccessRendererPrincipalDigestV1(principal)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x73}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := domainpii.NewControlledArtifactAccessReceiptV1(domainpii.ControlledArtifactAccessReceiptInputV1{
		SecurityContext: securityContext, AccessAction: domainpii.ControlledArtifactAccessActionExportV1,
		ControlledHandleDigest: handleDigest, UseSlotDigest: useSlotDigest, RendererPrincipalDigest: principalDigest,
		RendererGeneration: 7, BackendGeneration: 9,
		AccessPolicyDigest:       domainsecurity.SHA256Hex([]byte("controlled-host-access-policy")),
		RetentionPolicyDigest:    domainsecurity.SHA256Hex([]byte("controlled-host-retention-policy")),
		PublicationCommitDigest:  domainsecurity.SHA256Hex([]byte("controlled-host-publication-commit")),
		PublicationReceiptDigest: domainsecurity.SHA256Hex([]byte("controlled-host-publication-receipt")),
		PIIProjectionDigest:      domainsecurity.SHA256Hex([]byte("controlled-host-pii-projection")),
		PIIAuthorizationDigest:   domainsecurity.SHA256Hex([]byte("controlled-host-pii-authorization")),
		ClaimLedgerDigest:        domainsecurity.SHA256Hex([]byte("controlled-host-claim-ledger")),
		TargetIdentityDigest:     domainsecurity.SHA256Hex([]byte("controlled-host-target")),
		ArtifactSHA256:           domainsecurity.SHA256Hex(body), ArtifactByteLength: uint64(len(body)),
		MediaType: domainpii.ControlledPIIArtifactMediaTypeV1, RequestedAt: requested, AuthorizedUntil: expires,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	secretBody := bytes.Repeat([]byte{0x52}, desktopControlledHostSecretBytesV1)
	return &desktopControlledHostFixtureV1{
		secret: base64.RawURLEncoding.EncodeToString(secretBody), secretBody: secretBody,
		handle: handle, useSlot: useSlot, principal: principal, context: securityContext,
		receipt: receipt, body: body, requested: requested, expires: expires,
	}
}

func (fixture *desktopControlledHostFixtureV1) admissionRequest() piiauthorizationport.ControlledAccessAdmissionRequestV1 {
	return piiauthorizationport.ControlledAccessAdmissionRequestV1{
		SecurityContext: fixture.context, AccessAction: fixture.receipt.AccessAction,
		ControlledHandle: fixture.handle, UseSlot: fixture.useSlot, RendererPrincipal: fixture.principal,
		RendererGeneration: fixture.receipt.RendererGeneration, BackendGeneration: fixture.receipt.BackendGeneration,
	}
}

func (fixture *desktopControlledHostFixtureV1) admissionResponse() desktopControlledHostAdmissionResponseV1 {
	return desktopControlledHostAdmissionResponseV1{
		SchemaVersion: desktopControlledHostProtocolVersionV1, Purpose: desktopControlledHostAdmissionPurposeV1,
		ControlledHandleDigest: fixture.receipt.ControlledHandleDigest, UseSlotDigest: fixture.receipt.UseSlotDigest,
		RendererPrincipalDigest: fixture.receipt.RendererPrincipalDigest,
		RendererGeneration:      fixture.receipt.RendererGeneration, BackendGeneration: fixture.receipt.BackendGeneration,
		PublicationCommitDigest: fixture.receipt.PublicationCommitDigest, AuthorizedUntil: fixture.receipt.AuthorizedUntil,
	}
}

func (fixture *desktopControlledHostFixtureV1) releaseAck() desktopControlledHostReleaseAckV1 {
	return desktopControlledHostReleaseAckV1{
		SchemaVersion: desktopControlledHostProtocolVersionV1, Purpose: desktopControlledHostReleaseAckPurposeV1,
		Committed: true, AccessID: fixture.receipt.AccessID, AccessReceiptDigest: fixture.receipt.RecordDigest,
		ArtifactSHA256: fixture.receipt.ArtifactSHA256, ReleasedByteLength: fixture.receipt.ArtifactByteLength,
		AccessAction: fixture.receipt.AccessAction, ControlledHandleDigest: fixture.receipt.ControlledHandleDigest,
		UseSlotDigest: fixture.receipt.UseSlotDigest, RendererPrincipalDigest: fixture.receipt.RendererPrincipalDigest,
		RendererGeneration: fixture.receipt.RendererGeneration, BackendGeneration: fixture.receipt.BackendGeneration,
		CommittedAt: fixture.requested.Add(time.Minute).Format(time.RFC3339Nano),
	}
}

func writeDesktopControlledHostJSONV1(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", desktopControlledHostJSONMediaTypeV1)
	if err := json.NewEncoder(w).Encode(value); err != nil && !strings.Contains(err.Error(), "closed") {
		t.Fatalf("write desktop controlled host response: %v", err)
	}
}
