//go:build darwin || linux

package authoritycredentialsfs

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	secureconfigfs "analytix.local/runtime-go/internal/adapters/outbound/secureconfigfs"
	domaincredentials "analytix.local/runtime-go/internal/domain/authoritycredentials"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
	authoritycredentialsport "analytix.local/runtime-go/internal/ports/authoritycredentials"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

type credentialAnchorStub struct {
	mu      sync.Mutex
	anchors []authorityanchorport.AnchorV1
	calls   int
}

func (stub *credentialAnchorStub) Load(context.Context) (authorityanchorport.AnchorV1, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	index := stub.calls
	if index >= len(stub.anchors) {
		index = len(stub.anchors) - 1
	}
	stub.calls++
	anchor := stub.anchors[index]
	anchor.AuthorityPublicKey = append([]byte(nil), anchor.AuthorityPublicKey...)
	return anchor, nil
}

func (stub *credentialAnchorStub) setAnchors(anchors ...authorityanchorport.AnchorV1) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.anchors = append([]authorityanchorport.AnchorV1(nil), anchors...)
	stub.calls = 0
}

type countingListenerV1 struct {
	net.Listener
	accepts      *atomic.Int32
	totalAccepts *atomic.Int32
}

func (listener countingListenerV1) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err == nil {
		listener.accepts.Add(1)
		if listener.totalAccepts != nil {
			listener.totalAccepts.Add(1)
		}
	}
	return connection, err
}

type credentialFixtureOptions struct {
	keySemanticOverride string
	mismatchedKey       bool
	keyEncoding         privateKeyEncodingV1
	clientUsage         []x509.ExtKeyUsage
	expiredClient       bool
	rootLifetime        time.Duration
	clientLifetime      time.Duration
	crossNamespaceRole  domaincredentials.FileRoleV1
	crossNamespaceAll   bool
}

type privateKeyEncodingV1 uint8

const (
	privateKeyCanonicalV1 privateKeyEncodingV1 = iota
	privateKeyTrailingByteV1
	privateKeyConcatenatedObjectV1
	privateKeyWithAttributesV1
)

type credentialServerCountersV1 struct {
	calls   atomic.Int32
	accepts atomic.Int32
}

type credentialClockStubV1 struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *credentialClockStubV1) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *credentialClockStubV1) Advance(duration time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(duration)
}

type credentialNamespaceMaterialV1 struct {
	rootDER       []byte
	clientLeafDER []byte
	chainBody     []byte
	keyDER        []byte
	keySemantic   string
	endpoint      string
	serverName    string
}

type credentialLoaderFixture struct {
	loader              Loader
	anchored            domainenrollment.AnchoredManifestV2
	anchor              authorityanchorport.AnchorV1
	anchorStub          *credentialAnchorStub
	profile             domaincredentials.CredentialProfileV1
	bundleRoot          string
	installationPrivate ed25519.PrivateKey
	installationPublic  ed25519.PublicKey
	witnessPrivate      map[string]ed25519.PrivateKey
	checkpoints         map[string]domainsecurity.MonotonicHeadCheckpointV1
	clientLeafRaw       map[string][]byte
	servers             map[string]*credentialServerCountersV1
	clock               *credentialClockStubV1
	serverCalls         atomic.Int32
	serverAccepts       atomic.Int32
}

func (fixture *credentialLoaderFixture) assertNoNetworkV1(t *testing.T) {
	t.Helper()
	if fixture.serverCalls.Load() != 0 || fixture.serverAccepts.Load() != 0 {
		t.Fatalf("invalid credential path caused network I/O: calls=%d accepts=%d", fixture.serverCalls.Load(), fixture.serverAccepts.Load())
	}
	for namespace, counters := range fixture.servers {
		if counters.calls.Load() != 0 || counters.accepts.Load() != 0 {
			t.Fatalf("invalid credential path reached %s: calls=%d accepts=%d", namespace, counters.calls.Load(), counters.accepts.Load())
		}
	}
}

func (fixture *credentialLoaderFixture) loadCurrent(
	ctx context.Context,
) (authoritycredentialsport.EnrolledWitnessesV1, error) {
	return fixture.loader.loadCurrentWithReads(
		ctx,
		fixture.anchored,
		readCredentialProfileFixtureV1,
		readCredentialBundleFixtureV1,
	)
}

func TestLoaderBuildsExactMTLSWitnessesAndPresentsEnrolledIdentity(t *testing.T) {
	fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{})
	witnesses, err := fixture.loadCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixture.assertNoNetworkV1(t)
	if witnesses.ManifestDigest != fixture.anchor.CurrentManifestDigest ||
		witnesses.ProfileDigest != fixture.profile.ProfileDigest || witnesses.ProfileGeneration != fixture.profile.ProfileGeneration ||
		witnesses.ThreadRisk == nil || witnesses.SharedEvidence == nil {
		t.Fatalf("loaded witness identity mismatch: %#v", witnesses)
	}
	threadProjection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(
		fixture.anchored, domainenrollment.ThreadRiskNamespaceV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	sharedProjection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(
		fixture.anchored, domainenrollment.SharedEvidenceNamespaceV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	threadKey := descriptorV1(
		t, fixture.profile, domainenrollment.ThreadRiskNamespaceV1, domaincredentials.RoleWitnessMTLSClientPrivateKey,
	)
	sharedKey := descriptorV1(
		t, fixture.profile, domainenrollment.SharedEvidenceNamespaceV1, domaincredentials.RoleWitnessMTLSClientPrivateKey,
	)
	if threadProjection.Enrollment.EndpointOrigin == sharedProjection.Enrollment.EndpointOrigin ||
		threadProjection.Enrollment.RootCASHA256 == sharedProjection.Enrollment.RootCASHA256 ||
		threadProjection.Enrollment.MTLSClientIdentityCertificateSHA256 == sharedProjection.Enrollment.MTLSClientIdentityCertificateSHA256 ||
		threadProjection.Enrollment.WitnessKeyID == sharedProjection.Enrollment.WitnessKeyID ||
		threadKey.SemanticSHA256 == sharedKey.SemanticSHA256 {
		t.Fatal("authority namespaces reused an endpoint, root, client identity, private key, or witness key")
	}
	for _, test := range []struct {
		namespace string
		witness   interface {
			Observe(context.Context, domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error)
		}
	}{
		{domainenrollment.ThreadRiskNamespaceV1, witnesses.ThreadRisk},
		{domainenrollment.SharedEvidenceNamespaceV1, witnesses.SharedEvidence},
	} {
		request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(
			domainsecurity.MonotonicHeadObserveRequestInputV1{
				InstallationID: fixture.anchor.InstallationID,
				EnrollmentID:   fixture.checkpoints[test.namespace].EnrollmentID, Namespace: test.namespace,
				ChallengeNonce: domainsecurity.SHA256Hex([]byte("loader-observe:" + test.namespace)),
				AuthorityKeyID: fixture.anchor.AuthorityKeyID, AuthorityPublicKey: fixture.installationPublic,
			},
			func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil },
		)
		if err != nil {
			t.Fatal(err)
		}
		observation, err := test.witness.Observe(context.Background(), request)
		if err != nil || observation.Checkpoint != fixture.checkpoints[test.namespace] {
			t.Fatalf("mTLS witness observe failed for %s: %#v err=%v", test.namespace, observation, err)
		}
	}
	if fixture.serverCalls.Load() != 2 {
		t.Fatalf("mTLS witness call count = %d", fixture.serverCalls.Load())
	}
	if fixture.serverAccepts.Load() == 0 {
		t.Fatal("mTLS witness never established a network connection")
	}
	if bytes.Equal(fixture.clientLeafRaw[domainenrollment.ThreadRiskNamespaceV1], fixture.clientLeafRaw[domainenrollment.SharedEvidenceNamespaceV1]) {
		t.Fatal("fixture reused a client identity across authority namespaces")
	}
	for namespace, counters := range fixture.servers {
		if counters.calls.Load() != 1 || counters.accepts.Load() == 0 {
			t.Fatalf("namespace server was not independently exercised for %s: calls=%d accepts=%d", namespace, counters.calls.Load(), counters.accepts.Load())
		}
	}
}

func TestNamespaceWitnessRejectsOtherNamespaceRequestBeforeDial(t *testing.T) {
	fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{})
	witnesses, err := fixture.loadCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.observeRequestV1(
		t, domainenrollment.SharedEvidenceNamespaceV1, "shared-request-to-thread-witness",
	)
	if _, err := witnesses.ThreadRisk.Observe(context.Background(), request); !errors.Is(err, monotonicheadport.ErrInvalidReceipt) {
		t.Fatalf("cross-namespace request classification = %v", err)
	}
	fixture.assertNoNetworkV1(t)
}

func TestLoaderRejectsProfileFileOrKeySemanticMismatchBeforeNetwork(t *testing.T) {
	t.Run("file hash mismatch", func(t *testing.T) {
		fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{})
		name := descriptorNameV1(t, fixture.profile, domainenrollment.ThreadRiskNamespaceV1, domaincredentials.RoleWitnessRootCA)
		if err := os.WriteFile(filepath.Join(fixture.bundleRoot, name), []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.loadCurrent(context.Background()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("file hash mismatch classification = %v", err)
		}
		fixture.assertNoNetworkV1(t)
	})
	t.Run("key semantic mismatch", func(t *testing.T) {
		fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{
			keySemanticOverride: domainsecurity.SHA256Hex([]byte("wrong-key-spki")),
		})
		if _, err := fixture.loadCurrent(context.Background()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("key semantic mismatch classification = %v", err)
		}
		fixture.assertNoNetworkV1(t)
	})
	t.Run("private key leaf mismatch", func(t *testing.T) {
		fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{mismatchedKey: true})
		if _, err := fixture.loadCurrent(context.Background()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("key/leaf mismatch classification = %v", err)
		}
		fixture.assertNoNetworkV1(t)
	})
	for name, encoding := range map[string]privateKeyEncodingV1{
		"private key trailing byte":       privateKeyTrailingByteV1,
		"private key concatenated object": privateKeyConcatenatedObjectV1,
		"private key attributes":          privateKeyWithAttributesV1,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{keyEncoding: encoding})
			if _, err := fixture.loadCurrent(context.Background()); !errors.Is(err, ErrInvalid) {
				t.Fatalf("non-canonical PKCS#8 classification = %v", err)
			}
			fixture.assertNoNetworkV1(t)
		})
	}
	for name, options := range map[string]credentialFixtureOptions{
		"cross namespace root":   {crossNamespaceRole: domaincredentials.RoleWitnessRootCA},
		"cross namespace chain":  {crossNamespaceRole: domaincredentials.RoleWitnessMTLSClientChain},
		"cross namespace key":    {crossNamespaceRole: domaincredentials.RoleWitnessMTLSClientPrivateKey},
		"cross namespace bundle": {crossNamespaceAll: true},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCredentialLoaderFixture(t, options)
			if _, err := fixture.loadCurrent(context.Background()); !errors.Is(err, ErrInvalid) {
				t.Fatalf("cross-namespace credential classification = %v", err)
			}
			fixture.assertNoNetworkV1(t)
		})
	}
}

func TestLoaderRejectsClientUsageValidityAndAnchorRotation(t *testing.T) {
	for name, options := range map[string]credentialFixtureOptions{
		"missing client auth": {clientUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}},
		"expired client":      {expiredClient: true},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCredentialLoaderFixture(t, options)
			if _, err := fixture.loadCurrent(context.Background()); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid client certificate classification = %v", err)
			}
			fixture.assertNoNetworkV1(t)
		})
	}
	t.Run("anchor rotation", func(t *testing.T) {
		fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{})
		changed := fixture.anchor
		changed.CurrentManifestDigest = domainsecurity.SHA256Hex([]byte("replacement-manifest"))
		fixture.anchorStub.setAnchors(fixture.anchor, changed)
		if _, err := fixture.loadCurrent(context.Background()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("anchor rotation classification = %v", err)
		}
		fixture.assertNoNetworkV1(t)
	})
	t.Run("loaded witness rejects rotated anchor before dial", func(t *testing.T) {
		fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{})
		witnesses, err := fixture.loadCurrent(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		changed := fixture.anchor
		changed.CurrentManifestDigest = domainsecurity.SHA256Hex([]byte("replacement-manifest"))
		fixture.anchorStub.setAnchors(changed)
		for _, test := range []struct {
			namespace string
			witness   interface {
				Observe(context.Context, domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error)
			}
		}{
			{domainenrollment.ThreadRiskNamespaceV1, witnesses.ThreadRisk},
			{domainenrollment.SharedEvidenceNamespaceV1, witnesses.SharedEvidence},
		} {
			request := fixture.observeRequestV1(t, test.namespace, "rotated-anchor-observe:"+test.namespace)
			if _, err := test.witness.Observe(context.Background(), request); !errors.Is(err, monotonicheadport.ErrUnavailable) {
				t.Fatalf("rotated-anchor witness classification for %s = %v", test.namespace, err)
			}
		}
		fixture.assertNoNetworkV1(t)
	})
	t.Run("loaded witness rejects expired credential before dial", func(t *testing.T) {
		fixture := newCredentialLoaderFixture(t, credentialFixtureOptions{})
		witnesses, err := fixture.loadCurrent(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		fixture.clock.Advance(25 * time.Hour)
		for _, test := range []struct {
			namespace string
			witness   interface {
				Observe(context.Context, domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error)
			}
		}{
			{domainenrollment.ThreadRiskNamespaceV1, witnesses.ThreadRisk},
			{domainenrollment.SharedEvidenceNamespaceV1, witnesses.SharedEvidence},
		} {
			request := fixture.observeRequestV1(t, test.namespace, "expired-credential:"+test.namespace)
			if _, err := test.witness.Observe(context.Background(), request); !errors.Is(err, monotonicheadport.ErrUnavailable) {
				t.Fatalf("expired credential classification for %s = %v", test.namespace, err)
			}
		}
		fixture.assertNoNetworkV1(t)
	})
	for name, options := range map[string]credentialFixtureOptions{
		"root expires first": {rootLifetime: 2 * time.Hour, clientLifetime: 24 * time.Hour},
		"leaf expires first": {rootLifetime: 24 * time.Hour, clientLifetime: 2 * time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCredentialLoaderFixture(t, options)
			witnesses, err := fixture.loadCurrent(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			fixture.clock.Advance(3 * time.Hour)
			request := fixture.observeRequestV1(t, domainenrollment.ThreadRiskNamespaceV1, name)
			if _, err := witnesses.ThreadRisk.Observe(context.Background(), request); !errors.Is(err, monotonicheadport.ErrUnavailable) {
				t.Fatalf("earliest validity boundary classification = %v", err)
			}
			fixture.assertNoNetworkV1(t)
		})
	}
}

func (fixture *credentialLoaderFixture) observeRequestV1(
	t *testing.T,
	namespace string,
	nonce string,
) domainsecurity.MonotonicHeadObserveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(
		domainsecurity.MonotonicHeadObserveRequestInputV1{
			InstallationID: fixture.anchor.InstallationID,
			EnrollmentID:   fixture.checkpoints[namespace].EnrollmentID,
			Namespace:      namespace,
			ChallengeNonce: domainsecurity.SHA256Hex([]byte(nonce)),
			AuthorityKeyID: fixture.anchor.AuthorityKeyID, AuthorityPublicKey: fixture.installationPublic,
		},
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func newCredentialLoaderFixture(t *testing.T, options credentialFixtureOptions) *credentialLoaderFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	installationPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x71}, ed25519.SeedSize))
	installationPublic := installationPrivate.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("credential-loader-installation"))
	namespaces := []string{domainenrollment.ThreadRiskNamespaceV1, domainenrollment.SharedEvidenceNamespaceV1}
	witnessPrivate := make(map[string]ed25519.PrivateKey, 2)
	checkpoints := make(map[string]domainsecurity.MonotonicHeadCheckpointV1, 2)
	for index, namespace := range namespaces {
		seed := bytes.Repeat([]byte{byte(0x31 + index)}, ed25519.SeedSize)
		private := ed25519.NewKeyFromSeed(seed)
		enrollmentID := domainsecurity.SHA256Hex([]byte("enrollment:" + namespace))
		checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
			InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: namespace, Generation: 0,
			CurrentStateDigest: domainsecurity.SHA256Hex([]byte("state:" + namespace)),
			FenceNonce:         domainsecurity.SHA256Hex([]byte("fence:" + namespace)),
			WitnessKeyID:       domainsecurity.SHA256Hex(private.Public().(ed25519.PublicKey)),
			WitnessPublicKey:   private.Public().(ed25519.PublicKey),
		}, func(message []byte) ([]byte, error) { return ed25519.Sign(private, message), nil })
		if err != nil {
			t.Fatal(err)
		}
		witnessPrivate[namespace] = private
		checkpoints[namespace] = checkpoint
	}

	fixture := &credentialLoaderFixture{
		installationPrivate: installationPrivate, installationPublic: installationPublic,
		witnessPrivate: witnessPrivate, checkpoints: checkpoints,
		clientLeafRaw: make(map[string][]byte, 2), servers: make(map[string]*credentialServerCountersV1, 2),
	}
	clock := &credentialClockStubV1{now: now}
	fixture.clock = clock
	materials := make(map[string]credentialNamespaceMaterialV1, 2)
	for index, namespace := range namespaces {
		namespaceOptions := credentialFixtureOptions{}
		if namespace == domainenrollment.ThreadRiskNamespaceV1 {
			namespaceOptions = options
		}
		materials[namespace] = newCredentialNamespaceMaterialV1(t, fixture, namespace, index, now, namespaceOptions)
	}

	inputs := make([]domaincredentials.FileDescriptorInputV1, 0, 6)
	bodies := make(map[string][]byte, 6)
	for _, namespace := range namespaces {
		material := materials[namespace]
		if namespace == domainenrollment.SharedEvidenceNamespaceV1 &&
			(options.crossNamespaceAll || options.crossNamespaceRole != "") {
			threadMaterial := materials[domainenrollment.ThreadRiskNamespaceV1]
			switch options.crossNamespaceRole {
			case domaincredentials.RoleWitnessRootCA:
				material.rootDER = threadMaterial.rootDER
			case domaincredentials.RoleWitnessMTLSClientChain:
				material.chainBody = threadMaterial.chainBody
				material.clientLeafDER = threadMaterial.clientLeafDER
			case domaincredentials.RoleWitnessMTLSClientPrivateKey:
				material.keyDER = threadMaterial.keyDER
				material.keySemantic = threadMaterial.keySemantic
			}
			if options.crossNamespaceAll {
				material = threadMaterial
			}
		}
		enrollmentID := checkpoints[namespace].EnrollmentID
		prefix := "thread-risk"
		if namespace == domainenrollment.SharedEvidenceNamespaceV1 {
			prefix = "shared-evidence"
		}
		inputs = append(inputs,
			domaincredentials.FileDescriptorInputV1{Role: domaincredentials.RoleWitnessRootCA, Namespace: namespace, EnrollmentID: enrollmentID, SizeBytes: uint64(len(material.rootDER)), FileSHA256: domainsecurity.SHA256Hex(material.rootDER), SemanticSHA256: domainsecurity.SHA256Hex(material.rootDER)},
			domaincredentials.FileDescriptorInputV1{Role: domaincredentials.RoleWitnessMTLSClientChain, Namespace: namespace, EnrollmentID: enrollmentID, SizeBytes: uint64(len(material.chainBody)), FileSHA256: domainsecurity.SHA256Hex(material.chainBody), SemanticSHA256: domainsecurity.SHA256Hex(material.clientLeafDER)},
			domaincredentials.FileDescriptorInputV1{Role: domaincredentials.RoleWitnessMTLSClientPrivateKey, Namespace: namespace, EnrollmentID: enrollmentID, SizeBytes: uint64(len(material.keyDER)), FileSHA256: domainsecurity.SHA256Hex(material.keyDER), SemanticSHA256: material.keySemantic},
		)
		bodies[prefix+"-root-ca.der"] = material.rootDER
		bodies[prefix+"-client-chain-v1.bin"] = material.chainBody
		bodies[prefix+"-client-key.pk8"] = material.keyDER
	}
	profile, err := domaincredentials.NewCredentialProfileV1(domaincredentials.CredentialProfileInputV1{
		InstallationID: installationID, AuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		ProfileGeneration: 1, Files: inputs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.profile = profile
	manifest, err := domainenrollment.NewManifestV2(domainenrollment.ManifestInputV2{
		InstallationID: installationID, InstallationAuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		InstallationAuthorityPublicKey: installationPublic, CredentialProfileGeneration: profile.ProfileGeneration,
		CredentialProfileDigest: profile.ProfileDigest, IssuedAt: now,
		ThreadRisk: credentialEnrollmentInputV1(checkpoints[domainenrollment.ThreadRiskNamespaceV1], witnessPrivate[domainenrollment.ThreadRiskNamespaceV1],
			materials[domainenrollment.ThreadRiskNamespaceV1].endpoint, materials[domainenrollment.ThreadRiskNamespaceV1].serverName,
			materials[domainenrollment.ThreadRiskNamespaceV1].rootDER, materials[domainenrollment.ThreadRiskNamespaceV1].clientLeafDER),
		SharedEvidence: credentialEnrollmentInputV1(checkpoints[domainenrollment.SharedEvidenceNamespaceV1], witnessPrivate[domainenrollment.SharedEvidenceNamespaceV1],
			materials[domainenrollment.SharedEvidenceNamespaceV1].endpoint, materials[domainenrollment.SharedEvidenceNamespaceV1].serverName,
			materials[domainenrollment.SharedEvidenceNamespaceV1].rootDER, materials[domainenrollment.SharedEvidenceNamespaceV1].clientLeafDER),
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	anchor := authorityanchorport.AnchorV1{
		InstallationID: installationID, AuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		AuthorityPublicKey: append([]byte(nil), installationPublic...), CurrentManifestDigest: manifest.ManifestDigest,
	}
	anchored, err := domainenrollment.AnchorManifestForInstallationV2(
		manifest, installationID, anchor.AuthorityKeyID, installationPublic, manifest.ManifestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	profileRoot := privateDirectoryV1(t, "profile")
	bundleRoot := privateDirectoryV1(t, "bundle")
	profileBody, err := domaincredentials.CredentialProfileV1Bytes(profile)
	if err != nil {
		t.Fatal(err)
	}
	writePrivateFileV1(t, filepath.Join(profileRoot, ProfileFileNameV1), profileBody)
	for _, descriptor := range profile.Files {
		writePrivateFileV1(t, filepath.Join(bundleRoot, descriptor.FixedName), bodies[descriptor.FixedName])
	}
	anchorStub := &credentialAnchorStub{anchors: []authorityanchorport.AnchorV1{anchor}}
	fixture.loader = Loader{ProfileRoot: profileRoot, BundleRoot: bundleRoot, Anchor: anchorStub, Now: clock.Now}
	fixture.anchored = anchored
	fixture.anchor = anchor
	fixture.anchorStub = anchorStub
	fixture.bundleRoot = bundleRoot
	return fixture
}

func newCredentialNamespaceMaterialV1(
	t *testing.T,
	fixture *credentialLoaderFixture,
	namespace string,
	index int,
	now time.Time,
	options credentialFixtureOptions,
) credentialNamespaceMaterialV1 {
	t.Helper()
	serialBase := int64(index*10 + 1)
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootLifetime := options.rootLifetime
	if rootLifetime == 0 {
		rootLifetime = 24 * time.Hour
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(serialBase), Subject: pkix.Name{CommonName: "Analytix Test Root " + namespace},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(rootLifetime),
		BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER := createCertificateV1(t, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(serialBase + 1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER := createCertificateV1(t, serverTemplate, root, &serverKey.PublicKey, rootKey)
	serverCertificate := tls.Certificate{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientLifetime := options.clientLifetime
	if clientLifetime == 0 {
		clientLifetime = 24 * time.Hour
	}
	clientNotAfter := now.Add(clientLifetime)
	if options.expiredClient {
		clientNotAfter = now.Add(-time.Minute)
	}
	clientUsage := options.clientUsage
	if clientUsage == nil {
		clientUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(serialBase + 2), Subject: pkix.Name{CommonName: "Analytix Test Client " + namespace},
		NotBefore: now.Add(-time.Hour), NotAfter: clientNotAfter, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: clientUsage,
	}
	clientDER := createCertificateV1(t, clientTemplate, root, &clientKey.PublicKey, rootKey)
	clientLeaf, err := x509.ParseCertificate(clientDER)
	if err != nil {
		t.Fatal(err)
	}
	keyForFile := clientKey
	if options.mismatchedKey {
		keyForFile, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(keyForFile)
	if err != nil {
		t.Fatal(err)
	}
	switch options.keyEncoding {
	case privateKeyCanonicalV1:
	case privateKeyTrailingByteV1:
		keyDER = append(keyDER, 0)
	case privateKeyConcatenatedObjectV1:
		keyDER = append(append([]byte(nil), keyDER...), keyDER...)
	case privateKeyWithAttributesV1:
		keyDER = pkcs8WithAttributeV1(t, keyDER)
	default:
		t.Fatal("unsupported private-key fixture encoding")
	}
	keySPKI, err := x509.MarshalPKIXPublicKey(&keyForFile.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	keySemantic := domainsecurity.SHA256Hex(keySPKI)
	if options.keySemanticOverride != "" {
		keySemantic = options.keySemanticOverride
	}
	chainBody, err := domaincredentials.ClientChainV1Bytes([][]byte{clientLeaf.Raw})
	if err != nil {
		t.Fatal(err)
	}

	fixture.clientLeafRaw[namespace] = append([]byte(nil), clientLeaf.Raw...)
	counters := &credentialServerCountersV1{}
	fixture.servers[namespace] = counters
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(root)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fixture.serverCalls.Add(1)
		counters.calls.Add(1)
		if request.TLS == nil || len(request.TLS.PeerCertificates) == 0 ||
			!bytes.Equal(request.TLS.PeerCertificates[0].Raw, fixture.clientLeafRaw[namespace]) {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		body, readErr := io.ReadAll(request.Body)
		observeRequest, parseErr := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
		checkpoint := fixture.checkpoints[namespace]
		private := fixture.witnessPrivate[namespace]
		if readErr != nil || parseErr != nil || observeRequest.Namespace != namespace ||
			observeRequest.EnrollmentID != checkpoint.EnrollmentID {
			writer.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		observation, newErr := domainsecurity.NewMonotonicHeadObservationV1(
			observeRequest, checkpoint, func(message []byte) ([]byte, error) { return ed25519.Sign(private, message), nil },
		)
		response, encodeErr := domainsecurity.MonotonicHeadObservationV1Bytes(observation)
		if newErr != nil || encodeErr != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(response)
	}))
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCertificate},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots,
	}
	server.Listener = countingListenerV1{
		Listener: server.Listener, accepts: &counters.accepts, totalAccepts: &fixture.serverAccepts,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return credentialNamespaceMaterialV1{
		rootDER: append([]byte(nil), root.Raw...), clientLeafDER: append([]byte(nil), clientLeaf.Raw...),
		chainBody: append([]byte(nil), chainBody...), keyDER: append([]byte(nil), keyDER...), keySemantic: keySemantic,
		endpoint: server.URL, serverName: parsedURL.Hostname(),
	}
}

func pkcs8WithAttributeV1(t *testing.T, keyDER []byte) []byte {
	t.Helper()
	var outer asn1.RawValue
	rest, err := asn1.Unmarshal(keyDER, &outer)
	if err != nil || len(rest) != 0 || outer.Class != asn1.ClassUniversal || outer.Tag != asn1.TagSequence || !outer.IsCompound {
		t.Fatal("canonical fixture key is not one PKCS#8 sequence")
	}
	attributeDER := []byte{
		0x30, 0x17,
		0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x14,
		0x31, 0x0a, 0x0c, 0x08, 'a', 'n', 'a', 'l', 'y', 't', 'i', 'x',
	}
	attributes, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: attributeDER,
	})
	if err != nil {
		t.Fatal(err)
	}
	withAttributes, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true,
		Bytes: append(append([]byte(nil), outer.Bytes...), attributes...),
	})
	if err != nil {
		t.Fatal(err)
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(withAttributes)
	if err != nil {
		t.Fatalf("attribute fixture must remain parseable PKCS#8: %v", err)
	}
	canonical, err := x509.MarshalPKCS8PrivateKey(parsedKey)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(canonical)
	if bytes.Equal(canonical, withAttributes) {
		t.Fatal("attribute fixture did not produce a non-canonical PKCS#8 encoding")
	}
	return withAttributes
}

func credentialEnrollmentInputV1(
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
	witnessPrivate ed25519.PrivateKey,
	endpoint, serverName string,
	rootDER, clientLeafDER []byte,
) domainenrollment.WitnessEnrollmentInputV1 {
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	return domainenrollment.WitnessEnrollmentInputV1{
		EnrollmentID: checkpoint.EnrollmentID, EndpointOrigin: endpoint,
		WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
		RootCASHA256:                        domainsecurity.SHA256Hex(rootDER),
		MTLSClientIdentityCertificateSHA256: domainsecurity.SHA256Hex(clientLeafDER),
		ServerName:                          serverName, TimeoutMS: 5_000, InitialCheckpoint: checkpoint,
	}
}

func createCertificateV1(
	t *testing.T,
	template, parent *x509.Certificate,
	publicKey any,
	parentKey any,
) []byte {
	t.Helper()
	body, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func privateDirectoryV1(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func writePrivateFileV1(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, append([]byte(nil), body...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readCredentialProfileFixtureV1(input secureconfigfs.ReadExactInput) ([]byte, error) {
	if input.Target != ProfileFileNameV1 || len(input.AllowedNames) != 1 ||
		input.AllowedNames[0] != input.Target || input.MaxBytes <= 0 {
		return nil, errors.New("credential profile test reader received an invalid exact inventory")
	}
	entries, err := os.ReadDir(input.Root)
	if err != nil || len(entries) != 1 || entries[0].Name() != input.Target || !entries[0].Type().IsRegular() {
		return nil, errors.Join(errors.New("credential profile test inventory is invalid"), err)
	}
	body, err := os.ReadFile(filepath.Join(input.Root, input.Target))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) == 0 || int64(len(body)) > input.MaxBytes {
		return nil, errors.New("credential profile test body exceeds its bound")
	}
	return body, nil
}

func readCredentialBundleFixtureV1(input secureconfigfs.ReadBundleInput) (map[string][]byte, error) {
	if len(input.Files) == 0 || input.MaxTotalBytes <= 0 {
		return nil, errors.New("credential bundle test reader received invalid bounds")
	}
	entries, err := os.ReadDir(input.Root)
	if err != nil || len(entries) != len(input.Files) {
		return nil, errors.Join(errors.New("credential bundle test inventory is invalid"), err)
	}
	declared := make(map[string]secureconfigfs.BundleFile, len(input.Files))
	for _, file := range input.Files {
		if file.Name == "" || file.MaxBytes <= 0 {
			return nil, errors.New("credential bundle test declaration is invalid")
		}
		if _, duplicate := declared[file.Name]; duplicate {
			return nil, errors.New("credential bundle test declaration contains duplicates")
		}
		if strings.HasSuffix(file.Name, "-client-key.pk8") != file.Sensitive {
			return nil, errors.New("credential bundle test private-key sensitivity is invalid")
		}
		declared[file.Name] = file
	}
	for _, entry := range entries {
		if _, ok := declared[entry.Name()]; !ok || !entry.Type().IsRegular() {
			return nil, errors.New("credential bundle test inventory is invalid")
		}
	}
	bodies := make(map[string][]byte, len(input.Files))
	var total int64
	for _, file := range input.Files {
		body, readErr := os.ReadFile(filepath.Join(input.Root, file.Name))
		if readErr != nil {
			return nil, readErr
		}
		if int64(len(body)) == 0 || int64(len(body)) > file.MaxBytes ||
			int64(len(body)) > input.MaxTotalBytes-total {
			return nil, errors.New("credential bundle test body exceeds its bound")
		}
		total += int64(len(body))
		bodies[file.Name] = body
	}
	return bodies, nil
}

func descriptorNameV1(
	t *testing.T,
	profile domaincredentials.CredentialProfileV1,
	namespace string,
	role domaincredentials.FileRoleV1,
) string {
	t.Helper()
	for _, descriptor := range profile.Files {
		if descriptor.Namespace == namespace && descriptor.Role == role {
			return descriptor.FixedName
		}
	}
	t.Fatal("credential descriptor is missing")
	return ""
}

func descriptorV1(
	t *testing.T,
	profile domaincredentials.CredentialProfileV1,
	namespace string,
	role domaincredentials.FileRoleV1,
) domaincredentials.FileDescriptorV1 {
	t.Helper()
	for _, descriptor := range profile.Files {
		if descriptor.Namespace == namespace && descriptor.Role == role {
			return descriptor
		}
	}
	t.Fatal("credential descriptor is missing")
	return domaincredentials.FileDescriptorV1{}
}
