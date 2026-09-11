//go:build darwin || linux

// Package formalauthority provides a real-filesystem, real-mTLS
// local authority service shared by production-composition tests and the
// explicit macOS formal-acceptance operator command. The service implements
// the production manifest, credential, and monotonic-head protocols; starting
// it is provisioning, not by itself delivery evidence.
package formalauthority

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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	authoritycredentialsfs "analytix.local/runtime-go/internal/adapters/outbound/authoritycredentialsfs"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	monotonicheadhttp "analytix.local/runtime-go/internal/adapters/outbound/monotonicheadhttp"
	domaincredentials "analytix.local/runtime-go/internal/domain/authoritycredentials"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const protocolContentType = "application/json"

type Service struct {
	DataDir               string
	AuthorityPath         string
	ManifestRoot          string
	CredentialProfileRoot string
	CredentialBundleRoot  string
	AnchorEnvelope        string
	InstallationID        string
	Manifest              domainenrollment.ManifestV2
	Profile               domaincredentials.CredentialProfileV1
	Authority             *finalauthority.FileAuthority

	witnesses map[string]*witnessServer
}

type WitnessSnapshot struct {
	Checkpoint   domainsecurity.MonotonicHeadCheckpointV1
	AttemptCalls int
	ObserveCalls int
	AdvanceCalls int
	ResolveCalls int
}

type anchorEnvelopeV1 struct {
	SchemaVersion         int    `json:"schemaVersion"`
	InstallationID        string `json:"installationId"`
	AuthorityKeyID        string `json:"authorityKeyId"`
	AuthorityPublicKey    string `json:"authorityPublicKey"`
	CurrentManifestDigest string `json:"currentManifestDigest"`
}

type namespaceMaterial struct {
	rootDER       []byte
	clientLeafDER []byte
	clientChain   []byte
	clientKeyDER  []byte
	clientKeySPKI []byte
	endpoint      string
	serverName    string
}

type committedMutation struct {
	request domainsecurity.MonotonicHeadAdvanceRequestV1
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1
}

type witnessServer struct {
	mu sync.Mutex

	server            *localWitnessHTTPServer
	installationID    string
	authorityKeyID    string
	authorityPublic   ed25519.PublicKey
	enrollmentID      string
	namespace         string
	witnessPrivate    ed25519.PrivateKey
	witnessPublic     ed25519.PublicKey
	expectedClientDER []byte
	checkpoint        domainsecurity.MonotonicHeadCheckpointV1
	committed         map[string]committedMutation
	attemptCalls      int
	observeCalls      int
	advanceCalls      int
	resolveCalls      int
	unavailable       bool
}

type localWitnessHTTPServer struct {
	URL      string
	server   *http.Server
	listener net.Listener
}

func (server *localWitnessHTTPServer) Close() {
	if server == nil {
		return
	}
	if server.server != nil {
		_ = server.server.Close()
	}
	if server.listener != nil {
		_ = server.listener.Close()
	}
}

func New(root string) (_ *Service, resultErr error) {
	canonicalRoot, err := canonicalPrivateRoot(root)
	if err != nil {
		return nil, err
	}
	service := &Service{
		DataDir:   filepath.Join(canonicalRoot, "data"),
		witnesses: make(map[string]*witnessServer, 2),
	}
	defer func() {
		if resultErr != nil {
			service.Close()
		}
	}()
	if err := makePrivateDirectory(service.DataDir); err != nil {
		return nil, err
	}
	service.AuthorityPath = filepath.Join(
		service.DataDir,
		"private",
		"authority",
		"final-answer-ed25519-v1.json",
	)
	authority, err := finalauthority.OpenOrCreateFileAuthority(service.AuthorityPath, false)
	if err != nil {
		return nil, fmt.Errorf("create installation authority: %w", err)
	}
	service.Authority = authority
	service.InstallationID = domainsecurity.SHA256Hex(
		append([]byte("analytix-local-authority-composition:\x00"), authority.PublicKey()...),
	)

	now := time.Now().UTC().Truncate(time.Second)
	namespaces := []string{
		domainenrollment.ThreadRiskNamespaceV1,
		domainenrollment.SharedEvidenceNamespaceV1,
	}
	materials := make(map[string]namespaceMaterial, len(namespaces))
	defer func() {
		for _, material := range materials {
			clear(material.clientKeyDER)
		}
	}()
	enrollmentIDs := make(map[string]string, len(namespaces))
	witnessKeys := make(map[string]ed25519.PrivateKey, len(namespaces))
	checkpoints := make(map[string]domainsecurity.MonotonicHeadCheckpointV1, len(namespaces))
	for index, namespace := range namespaces {
		enrollmentID := domainsecurity.SHA256Hex([]byte("analytix-local-enrollment:\x00" + service.InstallationID + "\x00" + namespace))
		_, witnessPrivate, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate witness key: %w", err)
		}
		witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
		checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(
			domainsecurity.MonotonicHeadCheckpointInputV1{
				InstallationID:     service.InstallationID,
				EnrollmentID:       enrollmentID,
				Namespace:          namespace,
				Generation:         0,
				CurrentStateDigest: domainsecurity.SHA256Hex([]byte("analytix-local-genesis-state:\x00" + namespace)),
				FenceNonce:         domainsecurity.SHA256Hex([]byte("analytix-local-genesis-fence:\x00" + namespace)),
				WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublic),
				WitnessPublicKey:   witnessPublic,
			},
			func(message []byte) ([]byte, error) {
				return ed25519.Sign(witnessPrivate, message), nil
			},
		)
		if err != nil {
			return nil, fmt.Errorf("create witness checkpoint: %w", err)
		}
		material, server, err := newNamespaceMaterial(
			service.InstallationID,
			authority.KeyID(),
			authority.PublicKey(),
			enrollmentID,
			namespace,
			witnessPrivate,
			checkpoint,
			now,
			index,
		)
		if err != nil {
			return nil, err
		}
		service.witnesses[namespace] = server
		materials[namespace] = material
		enrollmentIDs[namespace] = enrollmentID
		witnessKeys[namespace] = witnessPrivate
		checkpoints[namespace] = checkpoint
	}

	descriptorInputs := make([]domaincredentials.FileDescriptorInputV1, 0, 6)
	bundleBodies := make(map[string][]byte, 6)
	for _, namespace := range namespaces {
		material := materials[namespace]
		prefix := "thread-risk"
		if namespace == domainenrollment.SharedEvidenceNamespaceV1 {
			prefix = "shared-evidence"
		}
		descriptorInputs = append(descriptorInputs,
			domaincredentials.FileDescriptorInputV1{
				Role: domaincredentials.RoleWitnessRootCA, Namespace: namespace,
				EnrollmentID: enrollmentIDs[namespace], SizeBytes: uint64(len(material.rootDER)),
				FileSHA256: domainsecurity.SHA256Hex(material.rootDER), SemanticSHA256: domainsecurity.SHA256Hex(material.rootDER),
			},
			domaincredentials.FileDescriptorInputV1{
				Role: domaincredentials.RoleWitnessMTLSClientChain, Namespace: namespace,
				EnrollmentID: enrollmentIDs[namespace], SizeBytes: uint64(len(material.clientChain)),
				FileSHA256: domainsecurity.SHA256Hex(material.clientChain), SemanticSHA256: domainsecurity.SHA256Hex(material.clientLeafDER),
			},
			domaincredentials.FileDescriptorInputV1{
				Role: domaincredentials.RoleWitnessMTLSClientPrivateKey, Namespace: namespace,
				EnrollmentID: enrollmentIDs[namespace], SizeBytes: uint64(len(material.clientKeyDER)),
				FileSHA256: domainsecurity.SHA256Hex(material.clientKeyDER), SemanticSHA256: domainsecurity.SHA256Hex(material.clientKeySPKI),
			},
		)
		bundleBodies[prefix+"-root-ca.der"] = material.rootDER
		bundleBodies[prefix+"-client-chain-v1.bin"] = material.clientChain
		bundleBodies[prefix+"-client-key.pk8"] = material.clientKeyDER
	}
	profile, err := domaincredentials.NewCredentialProfileV1(domaincredentials.CredentialProfileInputV1{
		InstallationID:    service.InstallationID,
		AuthorityKeyID:    authority.KeyID(),
		ProfileGeneration: 1,
		Files:             descriptorInputs,
	})
	if err != nil {
		return nil, fmt.Errorf("create credential profile: %w", err)
	}
	service.Profile = profile
	manifest, err := domainenrollment.NewManifestV2(domainenrollment.ManifestInputV2{
		InstallationID:                 service.InstallationID,
		InstallationAuthorityKeyID:     authority.KeyID(),
		InstallationAuthorityPublicKey: authority.PublicKey(),
		CredentialProfileGeneration:    profile.ProfileGeneration,
		CredentialProfileDigest:        profile.ProfileDigest,
		IssuedAt:                       now,
		ThreadRisk: enrollmentInput(
			checkpoints[domainenrollment.ThreadRiskNamespaceV1],
			witnessKeys[domainenrollment.ThreadRiskNamespaceV1],
			materials[domainenrollment.ThreadRiskNamespaceV1],
		),
		SharedEvidence: enrollmentInput(
			checkpoints[domainenrollment.SharedEvidenceNamespaceV1],
			witnessKeys[domainenrollment.SharedEvidenceNamespaceV1],
			materials[domainenrollment.SharedEvidenceNamespaceV1],
		),
	}, func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		return nil, fmt.Errorf("create manifest: %w", err)
	}
	service.Manifest = manifest

	service.ManifestRoot = filepath.Join(canonicalRoot, "manifest")
	service.CredentialProfileRoot = filepath.Join(canonicalRoot, "credential-profile")
	service.CredentialBundleRoot = filepath.Join(canonicalRoot, "credential-bundle")
	for _, directory := range []string{
		service.ManifestRoot,
		service.CredentialProfileRoot,
		service.CredentialBundleRoot,
	} {
		if err := makePrivateDirectory(directory); err != nil {
			return nil, err
		}
	}
	manifestBody, err := domainenrollment.ManifestV2Bytes(manifest)
	if err != nil {
		return nil, err
	}
	if err := writePrivateExclusive(filepath.Join(service.ManifestRoot, "manifest.json"), manifestBody); err != nil {
		return nil, err
	}
	profileBody, err := domaincredentials.CredentialProfileV1Bytes(profile)
	if err != nil {
		return nil, err
	}
	if err := writePrivateExclusive(
		filepath.Join(service.CredentialProfileRoot, authoritycredentialsfs.ProfileFileNameV1),
		profileBody,
	); err != nil {
		return nil, err
	}
	for _, descriptor := range profile.Files {
		body := bundleBodies[descriptor.FixedName]
		if len(body) == 0 {
			return nil, errors.New("credential bundle body is missing")
		}
		if err := writePrivateExclusive(filepath.Join(service.CredentialBundleRoot, descriptor.FixedName), body); err != nil {
			return nil, err
		}
	}
	envelopeBody, err := json.Marshal(anchorEnvelopeV1{
		SchemaVersion:         1,
		InstallationID:        service.InstallationID,
		AuthorityKeyID:        authority.KeyID(),
		AuthorityPublicKey:    base64.RawURLEncoding.EncodeToString(authority.PublicKey()),
		CurrentManifestDigest: manifest.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	service.AnchorEnvelope = string(envelopeBody)
	return service, nil
}

func (fixture *Service) Close() {
	if fixture == nil {
		return
	}
	for _, witness := range fixture.witnesses {
		if witness != nil && witness.server != nil {
			witness.server.Close()
		}
	}
}

// Snapshot exposes the local witness activity without issuing a protocol request.
func (fixture *Service) Snapshot(namespace string) (WitnessSnapshot, bool) {
	if fixture == nil {
		return WitnessSnapshot{}, false
	}
	witness, ok := fixture.witnesses[namespace]
	if !ok {
		return WitnessSnapshot{}, false
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	return WitnessSnapshot{
		Checkpoint:   witness.checkpoint,
		AttemptCalls: witness.attemptCalls,
		ObserveCalls: witness.observeCalls,
		AdvanceCalls: witness.advanceCalls,
		ResolveCalls: witness.resolveCalls,
	}, true
}

func (fixture *Service) TotalAttempts() int {
	if fixture == nil {
		return 0
	}
	total := 0
	for namespace := range fixture.witnesses {
		snapshot, _ := fixture.Snapshot(namespace)
		total += snapshot.AttemptCalls
	}
	return total
}

func (fixture *Service) SetWitnessAvailable(namespace string, available bool) bool {
	if fixture == nil {
		return false
	}
	witness, ok := fixture.witnesses[namespace]
	if !ok {
		return false
	}
	witness.mu.Lock()
	witness.unavailable = !available
	witness.mu.Unlock()
	return true
}

func newNamespaceMaterial(
	installationID string,
	authorityKeyID string,
	authorityPublic []byte,
	enrollmentID string,
	namespace string,
	witnessPrivate ed25519.PrivateKey,
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
	now time.Time,
	index int,
) (namespaceMaterial, *witnessServer, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	serialBase := int64(index*10 + 1)
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(serialBase),
		Subject:               pkix.Name{CommonName: "Analytix Local Authority Root " + namespace},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(serialBase + 1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, root, &serverKey.PublicKey, rootKey)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	serverLeaf, err := x509.ParseCertificate(serverDER)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	clientTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(serialBase + 2),
		Subject:               pkix.Name{CommonName: "Analytix Local Authority Client " + namespace},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, root, &clientKey.PublicKey, rootKey)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	clientLeaf, err := x509.ParseCertificate(clientDER)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	clientKeyDER, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	clientKeySPKI, err := x509.MarshalPKIXPublicKey(&clientKey.PublicKey)
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	clientChain, err := domaincredentials.ClientChainV1Bytes([][]byte{clientLeaf.Raw})
	if err != nil {
		return namespaceMaterial{}, nil, err
	}

	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	witness := &witnessServer{
		installationID:    installationID,
		authorityKeyID:    authorityKeyID,
		authorityPublic:   append(ed25519.PublicKey(nil), authorityPublic...),
		enrollmentID:      enrollmentID,
		namespace:         namespace,
		witnessPrivate:    append(ed25519.PrivateKey(nil), witnessPrivate...),
		witnessPublic:     append(ed25519.PublicKey(nil), witnessPublic...),
		expectedClientDER: append([]byte(nil), clientLeaf.Raw...),
		checkpoint:        checkpoint,
		committed:         make(map[string]committedMutation),
	}
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(root)
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverLeaf.Raw},
			PrivateKey:  serverKey,
			Leaf:        serverLeaf,
		}},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  clientRoots,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return namespaceMaterial{}, nil, err
	}
	server := &localWitnessHTTPServer{
		URL:      "https://" + listener.Addr().String(),
		server:   &http.Server{Handler: witness},
		listener: listener,
	}
	go func() {
		_ = server.server.Serve(tls.NewListener(listener, tlsConfig))
	}()
	witness.server = server
	parsed, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		return namespaceMaterial{}, nil, err
	}
	return namespaceMaterial{
		rootDER:       append([]byte(nil), root.Raw...),
		clientLeafDER: append([]byte(nil), clientLeaf.Raw...),
		clientChain:   append([]byte(nil), clientChain...),
		clientKeyDER:  append([]byte(nil), clientKeyDER...),
		clientKeySPKI: append([]byte(nil), clientKeySPKI...),
		endpoint:      server.URL,
		serverName:    parsed.Hostname(),
	}, witness, nil
}

func enrollmentInput(
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
	witnessPrivate ed25519.PrivateKey,
	material namespaceMaterial,
) domainenrollment.WitnessEnrollmentInputV1 {
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	return domainenrollment.WitnessEnrollmentInputV1{
		EnrollmentID:                        checkpoint.EnrollmentID,
		EndpointOrigin:                      material.endpoint,
		WitnessKeyID:                        domainsecurity.SHA256Hex(witnessPublic),
		WitnessPublicKey:                    witnessPublic,
		RootCASHA256:                        domainsecurity.SHA256Hex(material.rootDER),
		MTLSClientIdentityCertificateSHA256: domainsecurity.SHA256Hex(material.clientLeafDER),
		ServerName:                          material.serverName,
		TimeoutMS:                           5_000,
		InitialCheckpoint:                   checkpoint,
	}
}

func (witness *witnessServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.RawQuery != "" ||
		request.Header.Get("Accept") != protocolContentType ||
		request.Header.Get("Content-Type") != protocolContentType ||
		request.TLS == nil || len(request.TLS.PeerCertificates) != 1 ||
		!bytes.Equal(request.TLS.PeerCertificates[0].Raw, witness.expectedClientDER) {
		writeProtocolError(writer, http.StatusForbidden, "invalid_receipt")
		return
	}
	witness.mu.Lock()
	witness.attemptCalls++
	unavailable := witness.unavailable
	witness.mu.Unlock()
	if unavailable {
		writeProtocolError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 64<<10+1))
	if err != nil || len(body) == 0 || len(body) > 64<<10 {
		writeProtocolError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	switch request.URL.Path {
	case monotonicheadhttp.ObservePath:
		witness.serveObserve(writer, body)
	case monotonicheadhttp.AdvancePath:
		witness.serveAdvance(writer, body)
	case monotonicheadhttp.MutationResolvePath:
		witness.serveResolve(writer, body)
	default:
		writeProtocolError(writer, http.StatusNotFound, "not_enrolled")
	}
}

func (witness *witnessServer) serveObserve(writer http.ResponseWriter, body []byte) {
	request, err := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
	if err != nil || domainsecurity.ValidateMonotonicHeadObserveRequestForInstallationV1(
		request,
		witness.installationID,
		witness.authorityKeyID,
		witness.authorityPublic,
	) != nil || request.EnrollmentID != witness.enrollmentID || request.Namespace != witness.namespace {
		writeProtocolError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	witness.observeCalls++
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		request,
		witness.checkpoint,
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(witness.witnessPrivate, message), nil
		},
	)
	if err != nil {
		writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
		return
	}
	response, err := domainsecurity.MonotonicHeadObservationV1Bytes(observation)
	if err != nil {
		writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
		return
	}
	writeProtocolBody(writer, response)
}

func (witness *witnessServer) serveAdvance(writer http.ResponseWriter, body []byte) {
	request, err := domainsecurity.ParseMonotonicHeadAdvanceRequestV1(body)
	if err != nil || domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(
		request,
		witness.installationID,
		witness.authorityKeyID,
		witness.authorityPublic,
	) != nil || request.EnrollmentID != witness.enrollmentID || request.Namespace != witness.namespace {
		writeProtocolError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	witness.advanceCalls++
	if committed, ok := witness.committed[request.MutationID]; ok {
		if committed.request != request {
			writeProtocolError(writer, http.StatusConflict, "mutation_conflict")
			return
		}
		body, err := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(committed.receipt)
		if err != nil {
			writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
			return
		}
		writeProtocolBody(writer, body)
		return
	}
	current := witness.checkpoint
	if request.ExpectedGeneration != current.Generation ||
		request.ExpectedCheckpointDigest != current.CheckpointDigest ||
		request.ExpectedStateDigest != current.CurrentStateDigest ||
		request.ExpectedFenceNonce != current.FenceNonce ||
		request.NextGeneration != current.Generation+1 {
		writeProtocolError(writer, http.StatusConflict, "cas_conflict")
		return
	}
	fence := domainsecurity.SHA256Hex([]byte("analytix-local-next-fence:\x00" + witness.namespace + "\x00" + request.MutationID))
	if fence == request.ExpectedFenceNonce {
		fence = domainsecurity.SHA256Hex([]byte("analytix-local-alternative-fence:\x00" + request.MutationID))
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(
		domainsecurity.MonotonicHeadCheckpointInputV1{
			InstallationID:           witness.installationID,
			EnrollmentID:             witness.enrollmentID,
			Namespace:                witness.namespace,
			Generation:               request.NextGeneration,
			CurrentStateDigest:       request.NextStateDigest,
			PreviousStateDigest:      request.ExpectedStateDigest,
			PreviousCheckpointDigest: request.ExpectedCheckpointDigest,
			FenceNonce:               fence,
			MutationID:               request.MutationID,
			WitnessKeyID:             domainsecurity.SHA256Hex(witness.witnessPublic),
			WitnessPublicKey:         witness.witnessPublic,
		},
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(witness.witnessPrivate, message), nil
		},
	)
	if err != nil {
		writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
		return
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(
		request,
		checkpoint,
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(witness.witnessPrivate, message), nil
		},
	)
	if err != nil {
		writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
		return
	}
	witness.checkpoint = checkpoint
	witness.committed[request.MutationID] = committedMutation{request: request, receipt: receipt}
	response, err := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(receipt)
	if err != nil {
		writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
		return
	}
	writeProtocolBody(writer, response)
}

func (witness *witnessServer) serveResolve(writer http.ResponseWriter, body []byte) {
	request, err := domainsecurity.ParseMonotonicHeadMutationResolveRequestV1(body)
	if err != nil || domainsecurity.ValidateMonotonicHeadMutationResolveRequestForInstallationV1(
		request,
		witness.installationID,
		witness.authorityKeyID,
		witness.authorityPublic,
	) != nil || request.AdvanceRequest.EnrollmentID != witness.enrollmentID ||
		request.AdvanceRequest.Namespace != witness.namespace {
		writeProtocolError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	witness.resolveCalls++
	var committed *domainsecurity.MonotonicHeadCommittedMutationV1
	if stored, ok := witness.committed[request.AdvanceRequest.MutationID]; ok {
		if stored.request != request.AdvanceRequest {
			writeProtocolError(writer, http.StatusConflict, "mutation_conflict")
			return
		}
		committed = &domainsecurity.MonotonicHeadCommittedMutationV1{
			AdvanceRequest: stored.request,
			AdvanceReceipt: stored.receipt,
		}
	}
	resolution, err := domainsecurity.NewMonotonicHeadMutationResolutionV1(
		request,
		witness.checkpoint,
		committed,
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(witness.witnessPrivate, message), nil
		},
	)
	if err != nil {
		writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
		return
	}
	response, err := domainsecurity.MonotonicHeadMutationResolutionV1Bytes(resolution)
	if err != nil {
		writeProtocolError(writer, http.StatusInternalServerError, "unavailable")
		return
	}
	writeProtocolBody(writer, response)
}

func writeProtocolBody(writer http.ResponseWriter, body []byte) {
	writer.Header().Set("Content-Type", protocolContentType)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(body)
}

func writeProtocolError(writer http.ResponseWriter, status int, code string) {
	body, _ := json.Marshal(struct {
		SchemaVersion int    `json:"schemaVersion"`
		Purpose       string `json:"purpose"`
		Code          string `json:"code"`
	}{
		SchemaVersion: 1,
		Purpose:       "analytix.monotonic-head-error/v1",
		Code:          code,
	})
	writer.Header().Set("Content-Type", protocolContentType)
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
}

func canonicalPrivateRoot(root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", errors.New("local authority composition root is invalid")
	}
	state, err := os.Lstat(root)
	if err != nil || !state.IsDir() || state.Mode()&os.ModeSymlink != 0 ||
		state.Mode().Perm() != 0o700 || state.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return "", errors.Join(errors.New("local authority composition root is not private"), err)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil || !filepath.IsAbs(real) || filepath.Clean(real) != real {
		return "", errors.Join(errors.New("local authority composition root is not canonical"), err)
	}
	return real, nil
}

func makePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func writePrivateExclusive(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(append([]byte(nil), body...))
	if writeErr == nil && written != len(body) {
		writeErr = io.ErrShortWrite
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}
