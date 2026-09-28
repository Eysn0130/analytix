//go:build darwin || linux

package sharedwitnesshttp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/monotonicheadhttp"
	"analytix.local/runtime-go/internal/adapters/outbound/sharedwitnessownerfs"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
	"analytix.local/runtime-go/internal/ports/monotonichead"
)

type witnessFixture struct {
	config              Config
	anchor              sharedwitnessownerfs.Anchor
	installationPrivate ed25519.PrivateKey
	witnessPrivate      ed25519.PrivateKey
	root                *x509.Certificate
	rootPrivate         ed25519.PrivateKey
	clientPrivate       ed25519.PrivateKey
	endpoint            string
	anchorSource        *testAnchorSource
}

type testAnchorSource struct {
	mu            sync.Mutex
	value         authorityanchorport.AnchorV1
	loads         int
	rotateAtLoad  int
	rotatedDigest string
}

func (source *testAnchorSource) Load(ctx context.Context) (authorityanchorport.AnchorV1, error) {
	if err := ctx.Err(); err != nil {
		return authorityanchorport.AnchorV1{}, err
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	source.loads++
	if source.rotateAtLoad != 0 && source.loads == source.rotateAtLoad {
		source.value.CurrentManifestDigest = source.rotatedDigest
	}
	value := source.value
	value.AuthorityPublicKey = append([]byte(nil), value.AuthorityPublicKey...)
	return value, nil
}

func (source *testAnchorSource) setDigest(digest string) {
	source.mu.Lock()
	source.value.CurrentManifestDigest = digest
	source.mu.Unlock()
}

func (source *testAnchorSource) rotateAt(load int, digest string) {
	source.mu.Lock()
	source.rotateAtLoad = load
	source.rotatedDigest = digest
	source.mu.Unlock()
}

func newWitnessFixture(t *testing.T) witnessFixture {
	return newWitnessFixtureWithOptions(t, time.Time{}, false, time.Time{})
}

func newWitnessFixtureWithClientExpiry(t *testing.T, clientExpiry time.Time) witnessFixture {
	return newWitnessFixtureWithOptions(t, clientExpiry, false, time.Time{})
}

func newWitnessFixtureWithServerIntermediateExpiry(t *testing.T, expiry time.Time) witnessFixture {
	return newWitnessFixtureWithOptions(t, time.Time{}, false, expiry)
}

func newWitnessFixtureWithOptions(t *testing.T, clientExpiry time.Time,
	clientIntermediate bool, serverIntermediateExpiry time.Time) witnessFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	userData := filepath.Join(base, "electron-user-data")
	if err := os.Mkdir(userData, 0o700); err != nil {
		t.Fatal(err)
	}
	installationPublic, installationPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	witnessPublic, witnessPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	installationID := domainsecurity.SHA256Hex([]byte("isolated installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("isolated shared enrollment"))
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace: domainenrollment.SharedEvidenceNamespaceV1, Generation: 0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("genesis state")),
		FenceNonce:         domainsecurity.SHA256Hex([]byte("genesis fence")),
		WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	anchor := sharedwitnessownerfs.Anchor{
		InstallationID: installationID, AuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		AuthorityPublicKey: installationPublic, EnrollmentID: enrollmentID, GenesisCheckpoint: checkpoint,
	}
	root, rootPrivate, rootDER := makeTestRoot(t)
	serverIssuer, serverIssuerPrivate := root, rootPrivate
	var serverIntermediateDER []byte
	if !serverIntermediateExpiry.IsZero() {
		serverIssuer, serverIssuerPrivate, serverIntermediateDER = makeTestIntermediateWithExpiry(
			t, root, rootPrivate, x509.ExtKeyUsageServerAuth, serverIntermediateExpiry)
	}
	serverCertificate, _, _ := makeTestLeaf(t, serverIssuer, serverIssuerPrivate, x509.ExtKeyUsageServerAuth)
	if len(serverIntermediateDER) != 0 {
		serverCertificate.Certificate = append(serverCertificate.Certificate, serverIntermediateDER)
	}
	clientIssuer, clientIssuerPrivate := root, rootPrivate
	var clientIntermediateDER []byte
	if clientIntermediate {
		clientIssuer, clientIssuerPrivate, clientIntermediateDER = makeTestIntermediate(t, root, rootPrivate)
	}
	_, clientPrivate, clientDER := makeTestLeafWithExpiry(t, clientIssuer, clientIssuerPrivate, x509.ExtKeyUsageClientAuth, clientExpiry)
	clientChainDER := [][]byte{clientDER}
	if clientIntermediate {
		clientChainDER = append(clientChainDER, clientIntermediateDER)
	}
	endpoint := "https://127.0.0.1:" + reserveTestPort(t)
	manifest, err := domainenrollment.NewSharedEvidenceOnlyManifestV2(
		domainenrollment.SharedEvidenceOnlyManifestInputV2{
			InstallationID: installationID, InstallationAuthorityKeyID: anchor.AuthorityKeyID,
			InstallationAuthorityPublicKey: installationPublic,
			CredentialProfileGeneration:    1,
			CredentialProfileDigest:        domainsecurity.SHA256Hex([]byte("isolated credential profile")),
			IssuedAt:                       time.Now().UTC(),
			SharedEvidence: domainenrollment.WitnessEnrollmentInputV1{
				EnrollmentID: enrollmentID, EndpointOrigin: endpoint,
				WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
				RootCASHA256:                        domainsecurity.SHA256Hex(rootDER),
				MTLSClientIdentityCertificateSHA256: domainsecurity.SHA256Hex(clientDER),
				ServerName:                          "127.0.0.1", TimeoutMS: 5_000, InitialCheckpoint: checkpoint,
			},
		}, func(message []byte) ([]byte, error) { return ed25519.Sign(installationPrivate, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	anchored, err := domainenrollment.AnchorManifestForInstallationV2(
		manifest, installationID, anchor.AuthorityKeyID, installationPublic, manifest.ManifestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	anchorSource := &testAnchorSource{value: authorityanchorport.AnchorV1{
		InstallationID: installationID, AuthorityKeyID: anchor.AuthorityKeyID,
		AuthorityPublicKey:    append([]byte(nil), installationPublic...),
		CurrentManifestDigest: manifest.ManifestDigest,
	}}
	return witnessFixture{
		config: Config{UserDataDir: userData, Manifest: anchored,
			AnchorSource: anchorSource, RootCADER: rootDER,
			ClientChainDER: clientChainDER, ServerCertificate: serverCertificate},
		anchor: anchor, installationPrivate: installationPrivate, witnessPrivate: witnessPrivate,
		root: root, rootPrivate: rootPrivate, clientPrivate: clientPrivate,
		endpoint: endpoint, anchorSource: anchorSource,
	}
}

// This writes an isolated owner fixture only in the test's private temporary
// directory. Production Start has no genesis or seed creation path.
func (fixture witnessFixture) createExistingOwner(t *testing.T) {
	t.Helper()
	root := filepath.Join(fixture.config.UserDataDir, sharedwitnessownerfs.OwnerRootName)
	state := filepath.Join(root, "state-v1")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	record := struct {
		FormatVersion      int                                      `json:"formatVersion"`
		Purpose            string                                   `json:"purpose"`
		InstallationID     string                                   `json:"installationId"`
		AuthorityKeyID     string                                   `json:"authorityKeyId"`
		AuthorityPublicKey string                                   `json:"authorityPublicKey"`
		EnrollmentID       string                                   `json:"enrollmentId"`
		WitnessSeed        []byte                                   `json:"witnessSeed"`
		GenesisCheckpoint  domainsecurity.MonotonicHeadCheckpointV1 `json:"genesisCheckpoint"`
	}{1, "analytix.shared-evidence-witness-owner-genesis/v1", fixture.anchor.InstallationID,
		fixture.anchor.AuthorityKeyID, base64.RawURLEncoding.EncodeToString(fixture.anchor.AuthorityPublicKey),
		fixture.anchor.EnrollmentID, append([]byte(nil), fixture.witnessPrivate.Seed()...),
		fixture.anchor.GenesisCheckpoint}
	body, err := json.Marshal(record)
	clear(record.WitnessSeed)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(body)
	if err := os.WriteFile(filepath.Join(state, "genesis.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (fixture witnessFixture) start(t *testing.T) *Server {
	t.Helper()
	server, err := Start(context.Background(), fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return server
}

func (fixture witnessFixture) client(t *testing.T) *monotonicheadhttp.Client {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(fixture.root)
	witnessPublic := fixture.witnessPrivate.Public().(ed25519.PublicKey)
	client, err := monotonicheadhttp.New(monotonicheadhttp.Config{
		Endpoint:           fixture.endpoint,
		InstallationID:     fixture.anchor.InstallationID,
		EnrollmentID:       fixture.anchor.EnrollmentID,
		Namespace:          domainenrollment.SharedEvidenceNamespaceV1,
		AuthorityKeyID:     fixture.anchor.AuthorityKeyID,
		AuthorityPublicKey: fixture.anchor.AuthorityPublicKey,
		WitnessKeyID:       fixture.anchor.GenesisCheckpoint.WitnessKeyID,
		WitnessPublicKey:   witnessPublic,
		RootCAs:            roots,
		ClientCertificates: []tls.Certificate{{Certificate: fixture.config.ClientChainDER,
			PrivateKey: fixture.clientPrivate}},
		ServerName: "127.0.0.1", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (fixture witnessFixture) observeRequest(t *testing.T) domainsecurity.MonotonicHeadObserveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: fixture.anchor.InstallationID, EnrollmentID: fixture.anchor.EnrollmentID,
		Namespace:      domainenrollment.SharedEvidenceNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte("fresh observe challenge")),
		AuthorityKeyID: fixture.anchor.AuthorityKeyID, AuthorityPublicKey: fixture.anchor.AuthorityPublicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func (fixture witnessFixture) advanceRequest(t *testing.T, checkpoint domainsecurity.MonotonicHeadCheckpointV1,
	mutation, nextState string) domainsecurity.MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: fixture.anchor.InstallationID, EnrollmentID: fixture.anchor.EnrollmentID,
		Namespace:          domainenrollment.SharedEvidenceNamespaceV1,
		ExpectedGeneration: checkpoint.Generation, ExpectedCheckpointDigest: checkpoint.CheckpointDigest,
		ExpectedStateDigest: checkpoint.CurrentStateDigest, ExpectedFenceNonce: checkpoint.FenceNonce,
		NextGeneration: checkpoint.Generation + 1, NextStateDigest: domainsecurity.SHA256Hex([]byte(nextState)),
		MutationID:     domainsecurity.SHA256Hex([]byte(mutation)),
		AuthorityKeyID: fixture.anchor.AuthorityKeyID, AuthorityPublicKey: fixture.anchor.AuthorityPublicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func (fixture witnessFixture) resolveRequest(t *testing.T,
	advance domainsecurity.MonotonicHeadAdvanceRequestV1) domainsecurity.MonotonicHeadMutationResolveRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadMutationResolveRequestV1(advance,
		domainsecurity.SHA256Hex([]byte("fresh resolve challenge")),
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestServerUsesExistingOwnerAcrossRestartAndExactReplay(t *testing.T) {
	fixture := newWitnessFixture(t)
	fixture.createExistingOwner(t)
	server := fixture.start(t)
	if duplicate, err := Start(context.Background(), fixture.config); err == nil {
		_ = duplicate.Close(context.Background())
		t.Fatal("second server acquired the live owner lease")
	}
	client := fixture.client(t)
	observe := fixture.observeRequest(t)
	initial, err := client.Observe(context.Background(), observe)
	if err != nil || initial.Checkpoint != fixture.anchor.GenesisCheckpoint {
		t.Fatalf("initial observe = %+v, %v", initial, err)
	}
	advance := fixture.advanceRequest(t, initial.Checkpoint, "first mutation", "next state")
	receipt, err := client.Advance(context.Background(), advance)
	if err != nil || receipt.Checkpoint.Generation != 1 {
		t.Fatalf("advance = %+v, %v", receipt, err)
	}
	replay, err := client.Advance(context.Background(), advance)
	if err != nil || replay != receipt {
		t.Fatalf("exact replay changed receipt: %v", err)
	}
	stale := fixture.advanceRequest(t, initial.Checkpoint, "stale mutation", "other state")
	if _, err := client.Advance(context.Background(), stale); !errors.Is(err, monotonichead.ErrCASConflict) {
		t.Fatalf("stale CAS error = %v", err)
	}
	resolve := fixture.resolveRequest(t, advance)
	resolution, err := client.ResolveMutation(context.Background(), resolve)
	if err != nil || resolution.Status != domainsecurity.MonotonicHeadMutationResolutionCommittedV1 ||
		resolution.Committed == nil || resolution.Committed.AdvanceReceipt != receipt {
		t.Fatalf("committed resolution = %+v, %v", resolution, err)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixture.start(t)
	reopenedClient := fixture.client(t)
	afterRestart, err := reopenedClient.Observe(context.Background(), observe)
	if err != nil || afterRestart.Checkpoint != receipt.Checkpoint {
		t.Fatalf("restart head = %+v, %v", afterRestart, err)
	}
	afterRestartResolution, err := reopenedClient.ResolveMutation(context.Background(), resolve)
	if err != nil || afterRestartResolution.Status != domainsecurity.MonotonicHeadMutationResolutionCommittedV1 ||
		afterRestartResolution.Committed == nil || afterRestartResolution.Committed.AdvanceReceipt != receipt {
		t.Fatalf("restart resolution = %+v, %v", afterRestartResolution, err)
	}
	conflicting := fixture.advanceRequest(t, initial.Checkpoint, "first mutation", "different state")
	if _, err := reopenedClient.Advance(context.Background(), conflicting); !errors.Is(err, monotonichead.ErrMutationConflict) {
		t.Fatalf("reused mutation ID error = %v", err)
	}
}

func TestFixedLoopbackAddressRejectsDynamicAndRemoteOrigins(t *testing.T) {
	if address, err := fixedLoopbackAddress("https://127.0.0.1:8443"); err != nil || address != "127.0.0.1:8443" {
		t.Fatalf("fixed loopback = %q, %v", address, err)
	}
	for _, origin := range []string{
		"http://127.0.0.1:8443", "https://127.0.0.1:0", "https://127.0.0.1:08443",
		"https://127.0.0.1", "https://0.0.0.0:8443", "https://localhost:8443",
		"https://[::1]:8443", "https://127.0.0.1:8443/path",
	} {
		if _, err := fixedLoopbackAddress(origin); err == nil {
			t.Fatalf("unsafe origin %q was accepted", origin)
		}
	}
}

func TestStartRequiresExistingOwnerAndEnrolledTLSMaterial(t *testing.T) {
	fixture := newWitnessFixture(t)
	wrongAnchor := fixture.config
	wrongAnchor.AnchorSource = &testAnchorSource{value: authorityanchorport.AnchorV1{
		InstallationID: fixture.anchor.InstallationID, AuthorityKeyID: fixture.anchor.AuthorityKeyID,
		AuthorityPublicKey:    append([]byte(nil), fixture.anchor.AuthorityPublicKey...),
		CurrentManifestDigest: domainsecurity.SHA256Hex([]byte("wrong manifest")),
	}}
	if _, err := Start(context.Background(), wrongAnchor); err == nil {
		t.Fatal("wrong current manifest anchor was accepted")
	}
	if _, err := Start(context.Background(), fixture.config); err == nil {
		t.Fatal("missing owner was created or accepted")
	}
	if _, err := os.Stat(filepath.Join(fixture.config.UserDataDir, sharedwitnessownerfs.OwnerRootName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing owner root changed: %v", err)
	}
	fixture.createExistingOwner(t)
	wrongRoot := fixture.config
	wrongRoot.RootCADER = append([]byte(nil), fixture.config.RootCADER...)
	wrongRoot.RootCADER[len(wrongRoot.RootCADER)-1] ^= 1
	if _, err := Start(context.Background(), wrongRoot); err == nil {
		t.Fatal("changed root CA was accepted")
	}
	wrongClient := fixture.config
	_, _, wrongClientDER := makeTestLeaf(t, fixture.root, fixture.rootPrivate, x509.ExtKeyUsageClientAuth)
	wrongClient.ClientChainDER = [][]byte{wrongClientDER}
	if _, err := Start(context.Background(), wrongClient); err == nil {
		t.Fatal("unenrolled client leaf was accepted")
	}
	wrongServer := fixture.config
	_, _, wrongServerDER := makeTestLeaf(t, fixture.root, fixture.rootPrivate, x509.ExtKeyUsageClientAuth)
	wrongServer.ServerCertificate.Certificate = [][]byte{wrongServerDER}
	if _, err := Start(context.Background(), wrongServer); err == nil {
		t.Fatal("non-server TLS leaf was accepted")
	}
	fixture.start(t)
}

func TestRotatedProtectedAnchorRefusesFurtherMutation(t *testing.T) {
	fixture := newWitnessFixture(t)
	fixture.createExistingOwner(t)
	server := fixture.start(t)
	client := fixture.client(t)
	initial, err := client.Observe(context.Background(), fixture.observeRequest(t))
	if err != nil || initial.Checkpoint.Generation != 0 {
		t.Fatalf("initial head = %+v, %v", initial, err)
	}
	fixture.anchorSource.setDigest(domainsecurity.SHA256Hex([]byte("rotated manifest")))
	advance := fixture.advanceRequest(t, initial.Checkpoint, "refused after rotation", "forbidden state")
	if _, err := client.Advance(context.Background(), advance); !errors.Is(err, monotonichead.ErrIndeterminate) ||
		!errors.Is(err, monotonichead.ErrUnavailable) {
		t.Fatalf("rotation error = %v", err)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, err := sharedwitnessownerfs.OpenExisting(context.Background(), fixture.config.UserDataDir, fixture.anchor)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	observation, err := owner.Observe(context.Background(), fixture.observeRequest(t))
	if err != nil || observation.Checkpoint.Generation != 0 {
		t.Fatalf("rotation changed durable head: %+v, %v", observation, err)
	}
}

func TestAnchorRotationAfterCommitReturnsIndeterminateForResolution(t *testing.T) {
	fixture := newWitnessFixture(t)
	fixture.createExistingOwner(t)
	// Start reads the anchor twice. Advance reads it before and after the
	// persistent commit; rotate on that fourth read.
	fixture.anchorSource.rotateAt(4, domainsecurity.SHA256Hex([]byte("rotated after commit")))
	server := fixture.start(t)
	advance := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "committed before rotation", "next state")
	if _, err := fixture.client(t).Advance(context.Background(), advance); !errors.Is(err, monotonichead.ErrIndeterminate) ||
		!errors.Is(err, monotonichead.ErrUnavailable) {
		t.Fatalf("post-commit rotation error = %v", err)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, err := sharedwitnessownerfs.OpenExisting(context.Background(), fixture.config.UserDataDir, fixture.anchor)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	resolution, err := owner.ResolveMutation(context.Background(), fixture.resolveRequest(t, advance))
	if err != nil || resolution.Status != domainsecurity.MonotonicHeadMutationResolutionCommittedV1 ||
		resolution.Committed == nil || resolution.Committed.AdvanceRequest != advance {
		t.Fatalf("durable post-rotation resolution = %+v, %v", resolution, err)
	}
}

func TestServerAcceptsLoaderStyleClientIntermediateChain(t *testing.T) {
	fixture := newWitnessFixtureWithOptions(t, time.Time{}, true, time.Time{})
	fixture.createExistingOwner(t)
	fixture.start(t)
	observation, err := fixture.client(t).Observe(context.Background(), fixture.observeRequest(t))
	if err != nil || observation.Checkpoint != fixture.anchor.GenesisCheckpoint {
		t.Fatalf("enrolled intermediate client chain = %+v, %v", observation, err)
	}
}

func TestServerRejectsAlternateCrossSignedClientIntermediate(t *testing.T) {
	fixture := newWitnessFixtureWithOptions(t, time.Time{}, true, time.Time{})
	fixture.createExistingOwner(t)
	fixture.start(t)
	enrolledIntermediate, err := x509.ParseCertificate(fixture.config.ClientChainDER[1])
	if err != nil {
		t.Fatal(err)
	}
	alternateDER := makeTestAlternateIntermediate(t, fixture.root, fixture.rootPrivate, enrolledIntermediate)
	alternateIntermediate, err := x509.ParseCertificate(alternateDER)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(alternateDER, enrolledIntermediate.Raw) ||
		!bytes.Equal(alternateIntermediate.RawSubject, enrolledIntermediate.RawSubject) ||
		!bytes.Equal(alternateIntermediate.RawSubjectPublicKeyInfo, enrolledIntermediate.RawSubjectPublicKeyInfo) {
		t.Fatal("alternate intermediate is not the intended same-subject same-key replacement")
	}
	leaf, err := x509.ParseCertificate(fixture.config.ClientChainDER[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(fixture.root)
	intermediates := x509.NewCertPool()
	intermediates.AddCert(alternateIntermediate)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("alternate chain is not otherwise valid: %v", err)
	}
	observeBody, err := domainsecurity.MonotonicHeadObserveRequestV1Bytes(fixture.observeRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	alternateClient := fixture.rawClientChain(t, [][]byte{fixture.config.ClientChainDER[0], alternateDER},
		fixture.clientPrivate, tls.VersionTLS13)
	if response, err := fixture.uncheckedPost(alternateClient, monotonicheadhttp.ObservePath, observeBody); err == nil {
		_ = response.Body.Close()
		t.Fatal("alternate cross-signed client chain completed mTLS handshake")
	}
	if observation, err := fixture.client(t).Observe(context.Background(), fixture.observeRequest(t)); err != nil ||
		observation.Checkpoint.Generation != 0 {
		t.Fatalf("enrolled chain was rejected after alternate: %+v, %v", observation, err)
	}
}

func TestServerRejectsUnenrolledClientAndMalformedHTTP(t *testing.T) {
	fixture := newWitnessFixture(t)
	fixture.createExistingOwner(t)
	fixture.start(t)
	observe, err := domainsecurity.MonotonicHeadObserveRequestV1Bytes(fixture.observeRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	requester := fixture.rawClient(t, fixture.config.ClientChainDER[0], fixture.clientPrivate, tls.VersionTLS13)
	for _, test := range []struct {
		name, method, path, contentType string
		body                            []byte
		status                          int
	}{
		{"wrong method", http.MethodGet, monotonicheadhttp.ObservePath, protocolContentType, observe, http.StatusForbidden},
		{"query", http.MethodPost, monotonicheadhttp.ObservePath + "?x=1", protocolContentType, observe, http.StatusForbidden},
		{"wrong path", http.MethodPost, "/v1/monotonic-head/unknown", protocolContentType, observe, http.StatusNotFound},
		{"wrong content type", http.MethodPost, monotonicheadhttp.ObservePath, "application/json; charset=utf-8", observe, http.StatusForbidden},
		{"noncanonical", http.MethodPost, monotonicheadhttp.ObservePath, protocolContentType, append(append([]byte(nil), observe...), '\n'), http.StatusUnprocessableEntity},
		{"oversized", http.MethodPost, monotonicheadhttp.ObservePath, protocolContentType, bytes.Repeat([]byte("x"), bodyLimit+1), http.StatusUnprocessableEntity},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := fixture.rawPost(t, requester, test.method, test.path, test.contentType, test.body)
			if response.StatusCode != test.status || response.Header.Get("Content-Type") != protocolContentType {
				t.Fatalf("status/content type = %d/%q", response.StatusCode, response.Header.Get("Content-Type"))
			}
			_ = response.Body.Close()
		})
	}
	_, otherPrivate, otherDER := makeTestLeaf(t, fixture.root, fixture.rootPrivate, x509.ExtKeyUsageClientAuth)
	otherClient := fixture.rawClient(t, otherDER, otherPrivate, tls.VersionTLS13)
	if response, err := fixture.uncheckedPost(otherClient, monotonicheadhttp.ObservePath, observe); err == nil {
		_ = response.Body.Close()
		t.Fatal("unenrolled client completed mTLS handshake")
	}
	oldTLS := fixture.rawClient(t, fixture.config.ClientChainDER[0], fixture.clientPrivate, tls.VersionTLS12)
	if response, err := fixture.uncheckedPost(oldTLS, monotonicheadhttp.ObservePath, observe); err == nil {
		_ = response.Body.Close()
		t.Fatal("TLS 1.2 client completed handshake")
	}
	if observation, err := fixture.client(t).Observe(context.Background(), fixture.observeRequest(t)); err != nil ||
		observation.Checkpoint.Generation != 0 {
		t.Fatalf("rejected requests changed owner head: %+v, %v", observation, err)
	}
}

func TestExpiredClientCannotAdvanceOnReusedTLSConnection(t *testing.T) {
	fixture := newWitnessFixtureWithClientExpiry(t, time.Now().Add(4*time.Second))
	assertExpiredChainRejectsAdvance(t, fixture, fixture.config.ClientChainDER[0])
}

func TestExpiredServerIntermediateCannotAdvanceOnReusedTLSConnection(t *testing.T) {
	fixture := newWitnessFixtureWithServerIntermediateExpiry(t, time.Now().Add(4*time.Second))
	assertExpiredChainRejectsAdvance(t, fixture, fixture.config.ServerCertificate.Certificate[1])
}

func assertExpiredChainRejectsAdvance(t *testing.T, fixture witnessFixture, expiringDER []byte) {
	t.Helper()
	fixture.createExistingOwner(t)
	server := fixture.start(t)
	roots := x509.NewCertPool()
	roots.AddCert(fixture.root)
	transport := &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 1, IdleConnTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots,
			ServerName: "127.0.0.1", Certificates: []tls.Certificate{{
				Certificate: fixture.config.ClientChainDER, PrivateKey: fixture.clientPrivate,
			}}}}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	t.Cleanup(transport.CloseIdleConnections)
	observe := fixture.observeRequest(t)
	observeBody, err := domainsecurity.MonotonicHeadObserveRequestV1Bytes(observe)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.uncheckedPost(client, monotonicheadhttp.ObservePath, observeBody)
	if err != nil {
		t.Fatal(err)
	}
	if first.StatusCode != http.StatusOK {
		t.Fatalf("pre-expiry observe status = %d", first.StatusCode)
	}
	if _, err := io.Copy(io.Discard, first.Body); err != nil {
		t.Fatal(err)
	}
	if err := first.Body.Close(); err != nil {
		t.Fatal(err)
	}
	expiringCertificate, err := x509.ParseCertificate(expiringDER)
	if err != nil {
		t.Fatal(err)
	}
	if wait := time.Until(expiringCertificate.NotAfter.Add(150 * time.Millisecond)); wait > 0 {
		time.Sleep(wait)
	}
	advance := fixture.advanceRequest(t, fixture.anchor.GenesisCheckpoint, "expired client mutation", "forbidden state")
	advanceBody, err := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(advance)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, fixture.endpoint+monotonicheadhttp.AdvancePath, bytes.NewReader(advanceBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", protocolContentType)
	request.Header.Set("Content-Type", protocolContentType)
	reused := false
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
	}))
	second, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if !reused || second.StatusCode != http.StatusForbidden {
		t.Fatalf("expired client reused=%v status=%d", reused, second.StatusCode)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, err := sharedwitnessownerfs.OpenExisting(context.Background(), fixture.config.UserDataDir, fixture.anchor)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	observation, err := owner.Observe(context.Background(), fixture.observeRequest(t))
	if err != nil || observation.Checkpoint.Generation != 0 {
		t.Fatalf("expired client changed durable head: %+v, %v", observation, err)
	}
}

func (fixture witnessFixture) rawClient(t *testing.T, leafDER []byte, private ed25519.PrivateKey, maxTLS uint16) *http.Client {
	return fixture.rawClientChain(t, [][]byte{leafDER}, private, maxTLS)
}

func (fixture witnessFixture) rawClientChain(t *testing.T, chainDER [][]byte,
	private ed25519.PrivateKey, maxTLS uint16) *http.Client {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(fixture.root)
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		Proxy: nil, DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: maxTLS,
			RootCAs: roots, ServerName: "127.0.0.1",
			Certificates: []tls.Certificate{{Certificate: chainDER, PrivateKey: private}}},
	}}
}

func (fixture witnessFixture) uncheckedPost(client *http.Client, path string, body []byte) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodPost, fixture.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", protocolContentType)
	request.Header.Set("Content-Type", protocolContentType)
	return client.Do(request)
}

func (fixture witnessFixture) rawPost(t *testing.T, client *http.Client,
	method, path, contentType string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, fixture.endpoint+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", protocolContentType)
	request.Header.Set("Content-Type", contentType)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestOwnerErrorWireCodes(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{monotonichead.ErrNotEnrolled, http.StatusNotFound, "not_enrolled"},
		{monotonichead.ErrCASConflict, http.StatusConflict, "cas_conflict"},
		{monotonichead.ErrMutationConflict, http.StatusConflict, "mutation_conflict"},
		{monotonichead.ErrInvalidReceipt, http.StatusUnprocessableEntity, "invalid_receipt"},
		{monotonichead.ErrIndeterminate, http.StatusServiceUnavailable, "unavailable"},
		{monotonichead.ErrUnavailable, http.StatusServiceUnavailable, "unavailable"},
	} {
		recorder := &wireRecorder{header: make(http.Header)}
		writeOwnerError(recorder, test.err)
		expected := `{"schemaVersion":1,"purpose":"analytix.monotonic-head-error/v1","code":"` + test.code + `"}`
		if recorder.status != test.status || recorder.header.Get("Content-Type") != protocolContentType ||
			recorder.body.String() != expected {
			t.Fatalf("error %v produced %d/%q", test.err, recorder.status, recorder.body.String())
		}
	}
}

type wireRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (recorder *wireRecorder) Header() http.Header            { return recorder.header }
func (recorder *wireRecorder) WriteHeader(status int)         { recorder.status = status }
func (recorder *wireRecorder) Write(body []byte) (int, error) { return recorder.body.Write(body) }

func reserveTestPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:")
}

func makeTestRoot(t *testing.T) (*x509.Certificate, ed25519.PrivateKey, []byte) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated shared witness root"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true, IsCA: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return root, private, der
}

func makeTestIntermediate(t *testing.T, root *x509.Certificate,
	rootPrivate ed25519.PrivateKey) (*x509.Certificate, ed25519.PrivateKey, []byte) {
	return makeTestIntermediateWithExpiry(t, root, rootPrivate, x509.ExtKeyUsageClientAuth, time.Time{})
}

func makeTestIntermediateWithExpiry(t *testing.T, root *x509.Certificate,
	rootPrivate ed25519.PrivateKey, usage x509.ExtKeyUsage,
	expiry time.Time) (*x509.Certificate, ed25519.PrivateKey, []byte) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if expiry.IsZero() {
		expiry = time.Now().Add(24 * time.Hour)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "isolated shared witness client CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: expiry,
		BasicConstraintsValid: true, IsCA: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, root, public, rootPrivate)
	if err != nil {
		t.Fatal(err)
	}
	intermediate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return intermediate, private, der
}

func makeTestAlternateIntermediate(t *testing.T, root *x509.Certificate,
	rootPrivate ed25519.PrivateKey, enrolled *x509.Certificate) []byte {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(901), Subject: enrolled.Subject,
		NotBefore: enrolled.NotBefore, NotAfter: enrolled.NotAfter.Add(-time.Hour),
		BasicConstraintsValid: true, IsCA: true,
		KeyUsage: enrolled.KeyUsage, ExtKeyUsage: append([]x509.ExtKeyUsage(nil), enrolled.ExtKeyUsage...),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, root, enrolled.PublicKey, rootPrivate)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func makeTestLeaf(t *testing.T, root *x509.Certificate, rootPrivate ed25519.PrivateKey,
	usage x509.ExtKeyUsage) (tls.Certificate, ed25519.PrivateKey, []byte) {
	return makeTestLeafWithExpiry(t, root, rootPrivate, usage, time.Time{})
}

func makeTestLeafWithExpiry(t *testing.T, root *x509.Certificate, rootPrivate ed25519.PrivateKey,
	usage x509.ExtKeyUsage, expiry time.Time) (tls.Certificate, ed25519.PrivateKey, []byte) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	if expiry.IsZero() {
		expiry = time.Now().Add(24 * time.Hour)
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "isolated shared witness leaf"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: expiry,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	if usage == x509.ExtKeyUsageServerAuth {
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, root, public, rootPrivate)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: private}, private, der
}
