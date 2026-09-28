//go:build darwin || linux

// Package sharedwitnesshttp serves the enrolled SharedEvidence monotonic-head
// protocol from an existing, separately leased filesystem witness owner. It
// does not enroll an installation, create owner state, or select credentials.
package sharedwitnesshttp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/monotonicheadhttp"
	"analytix.local/runtime-go/internal/adapters/outbound/sharedwitnessownerfs"
	domaincredentials "analytix.local/runtime-go/internal/domain/authoritycredentials"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
	"analytix.local/runtime-go/internal/ports/monotonichead"
)

const (
	protocolContentType = "application/json"
	errorPurpose        = "analytix.monotonic-head-error/v1"
	bodyLimit           = 64 << 10
	headerLimit         = 16 << 10
)

// Config contains only explicit, protected inputs. Manifest must already be
// anchored to the current installation and credential profile. RootCADER,
// ClientChainDER (leaf first), and ServerCertificate must come from that
// profile's protected material; Start verifies their enrolled identities
// before binding a socket.
// UserDataDir is the canonical Electron userData path containing an existing
// shared witness owner. Start takes ownership of that opened Owner until Close.
type Config struct {
	UserDataDir       string
	Manifest          domainenrollment.AnchoredManifestV2
	AnchorSource      authorityanchorport.Source
	RootCADER         []byte
	ClientChainDER    [][]byte
	ServerCertificate tls.Certificate
}

// Server owns one fixed loopback listener and one persistent owner lifetime
// lease. Close drains requests before releasing the owner.
type Server struct {
	server   *http.Server
	owner    *sharedwitnessownerfs.Owner
	done     chan struct{}
	serveMu  sync.Mutex
	serveErr error
	closeMu  sync.Mutex
	closed   bool
}

// Start binds exactly the enrolled HTTPS origin's 127.0.0.1 port. It fails
// closed when either the existing owner or pinned TLS material is unavailable.
func Start(ctx context.Context, config Config) (*Server, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("shared witness start context is unavailable")
	}
	projection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(
		config.Manifest, domainenrollment.SharedEvidenceNamespaceV1,
	)
	if err != nil {
		return nil, fmt.Errorf("shared witness enrollment is unavailable: %w", err)
	}
	if config.AnchorSource == nil {
		return nil, errors.New("shared witness protected anchor source is unavailable")
	}
	expectedAnchor := authorityanchorport.AnchorV1{
		InstallationID:        projection.InstallationID,
		AuthorityKeyID:        projection.InstallationAuthorityKeyID,
		AuthorityPublicKey:    append([]byte(nil), projection.InstallationAuthorityPublicKey...),
		CurrentManifestDigest: projection.ManifestDigest,
	}
	if !currentAnchorMatches(ctx, config.AnchorSource, expectedAnchor) {
		return nil, errors.New("shared witness current manifest anchor is unavailable")
	}
	enrollment := projection.Enrollment
	address, err := fixedLoopbackAddress(enrollment.EndpointOrigin)
	if err != nil {
		return nil, err
	}
	if enrollment.ServerName != "127.0.0.1" ||
		enrollment.MTLSClientIdentityCertificateSHA256 == "" ||
		enrollment.InitialCheckpoint.Namespace != domainenrollment.SharedEvidenceNamespaceV1 {
		return nil, errors.New("shared witness enrollment is not bound to fixed mTLS loopback")
	}
	root, rootPool, err := pinnedRoot(config.RootCADER, enrollment.RootCASHA256)
	if err != nil {
		return nil, err
	}
	clientChainDER, err := pinnedClientChain(config.ClientChainDER,
		enrollment.MTLSClientIdentityCertificateSHA256, root, rootPool)
	if err != nil {
		return nil, err
	}
	serverCertificate, serverChain, err := pinnedServerCertificate(config.ServerCertificate, rootPool, enrollment.ServerName)
	if err != nil {
		return nil, err
	}
	// Root must be a self-signed enrolled CA, not an arbitrary leaf or a
	// caller-selected intermediate that could widen client authentication.
	if !bytes.Equal(root.RawIssuer, root.RawSubject) || root.CheckSignatureFrom(root) != nil {
		return nil, errors.New("shared witness root CA is invalid")
	}
	witnessPublic, err := base64.RawURLEncoding.DecodeString(enrollment.WitnessPublicKey)
	if err != nil || len(witnessPublic) != ed25519.PublicKeySize {
		return nil, errors.New("shared witness enrolled key is invalid")
	}
	anchor := sharedwitnessownerfs.Anchor{
		InstallationID:     projection.InstallationID,
		AuthorityKeyID:     projection.InstallationAuthorityKeyID,
		AuthorityPublicKey: append(ed25519.PublicKey(nil), projection.InstallationAuthorityPublicKey...),
		EnrollmentID:       enrollment.EnrollmentID,
		GenesisCheckpoint:  enrollment.InitialCheckpoint,
	}
	owner, err := sharedwitnessownerfs.OpenExisting(ctx, config.UserDataDir, anchor)
	if err != nil {
		return nil, fmt.Errorf("shared witness existing owner is unavailable: %w", err)
	}
	if !currentAnchorMatches(ctx, config.AnchorSource, expectedAnchor) {
		return nil, errors.Join(errors.New("shared witness current manifest anchor changed"), owner.Close())
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, errors.Join(errors.New("shared witness fixed listener is unavailable"), err, owner.Close())
	}
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    rootPool,
		NextProtos:   []string{"http/1.1"},
		VerifyConnection: func(state tls.ConnectionState) error {
			if !validEnrolledClientConnection(state, clientChainDER, root.Raw) {
				return errors.New("shared witness client certificate is not enrolled")
			}
			return nil
		},
	}
	timeout := time.Duration(enrollment.TimeoutMS) * time.Millisecond
	handler := &protocolHandler{owner: owner, endpointHost: address,
		clientChainDER: clientChainDER, rootDER: append([]byte(nil), root.Raw...),
		serverChain: serverChain, anchorSource: config.AnchorSource,
		expectedAnchor: expectedAnchor}
	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: minDuration(timeout, 5*time.Second),
		ReadTimeout:       timeout,
		WriteTimeout:      timeout,
		IdleTimeout:       minDuration(timeout, 5*time.Second),
		MaxHeaderBytes:    headerLimit,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	server := &Server{server: httpServer, owner: owner, done: make(chan struct{})}
	go func() {
		serveErr := httpServer.Serve(tls.NewListener(listener, tlsConfig))
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		server.serveMu.Lock()
		server.serveErr = serveErr
		server.serveMu.Unlock()
		close(server.done)
	}()
	return server, nil
}

// Wait reports an unexpected listener failure. It does not release the owner;
// callers must Close even after Wait returns an error.
func (server *Server) Wait() error {
	if server == nil {
		return nil
	}
	<-server.done
	server.serveMu.Lock()
	defer server.serveMu.Unlock()
	return server.serveErr
}

// Close stops accepting requests, drains active handlers, then releases the
// owner lease. If the context expires while draining, the owner stays leased
// so an in-flight mutation cannot overlap a successor server.
func (server *Server) Close(ctx context.Context) error {
	if server == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("shared witness close context is unavailable")
	}
	server.closeMu.Lock()
	defer server.closeMu.Unlock()
	if server.closed {
		return nil
	}
	if err := server.server.Shutdown(ctx); err != nil {
		return err
	}
	serveErr := server.Wait()
	if err := server.owner.Close(); err != nil {
		return errors.Join(serveErr, err)
	}
	server.closed = true
	return serveErr
}

type protocolHandler struct {
	owner          *sharedwitnessownerfs.Owner
	endpointHost   string
	clientChainDER [][]byte
	rootDER        []byte
	serverChain    []*x509.Certificate
	anchorSource   authorityanchorport.Source
	expectedAnchor authorityanchorport.AnchorV1
}

func (handler *protocolHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// HTTP/1.1 can reuse a TLS connection after the handshake's certificate
	// validity window. Recheck every accepted request before the owner signs or
	// commits anything, including Advance on a previously authenticated socket.
	now := time.Now()
	if request.Method != http.MethodPost || request.Host != handler.endpointHost ||
		request.URL.RawQuery != "" || request.URL.ForceQuery || request.URL.RawPath != "" ||
		request.Header.Get("Content-Encoding") != "" ||
		!exactHeader(request.Header, "Accept", protocolContentType) ||
		!exactHeader(request.Header, "Content-Type", protocolContentType) ||
		request.TLS == nil || !validEnrolledClientConnection(*request.TLS, handler.clientChainDER, handler.rootDER) ||
		!validCertificateChainTime(handler.serverChain, now) || !validPeerCertificateTime(*request.TLS, now) {
		writeError(writer, http.StatusForbidden, "invalid_receipt")
		return
	}
	path := request.URL.Path
	if path != monotonicheadhttp.ObservePath && path != monotonicheadhttp.AdvancePath &&
		path != monotonicheadhttp.MutationResolvePath {
		writeError(writer, http.StatusNotFound, "not_enrolled")
		return
	}
	if request.ContentLength > bodyLimit {
		writeError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, bodyLimit+1))
	if err != nil || len(body) == 0 || len(body) > bodyLimit {
		writeError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	if !handler.stillAuthorized(request.Context(), request.TLS) {
		writeError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	switch path {
	case monotonicheadhttp.ObservePath:
		handler.observe(writer, request.Context(), request.TLS, body)
	case monotonicheadhttp.AdvancePath:
		handler.advance(writer, request.Context(), request.TLS, body)
	case monotonicheadhttp.MutationResolvePath:
		handler.resolveMutation(writer, request.Context(), request.TLS, body)
	}
}

func (handler *protocolHandler) observe(writer http.ResponseWriter, ctx context.Context,
	connection *tls.ConnectionState, body []byte) {
	request, err := domainsecurity.ParseMonotonicHeadObserveRequestV1(body)
	if err != nil || !canonicalObserveRequest(body, request) {
		writeError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	observation, err := handler.owner.Observe(ctx, request)
	if err != nil {
		writeOwnerError(writer, err)
		return
	}
	if !handler.stillAuthorized(ctx, connection) {
		writeError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	response, err := domainsecurity.MonotonicHeadObservationV1Bytes(observation)
	writeResult(writer, response, err)
}

func (handler *protocolHandler) advance(writer http.ResponseWriter, ctx context.Context,
	connection *tls.ConnectionState, body []byte) {
	request, err := domainsecurity.ParseMonotonicHeadAdvanceRequestV1(body)
	if err != nil || !canonicalAdvanceRequest(body, request) {
		writeError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	receipt, err := handler.owner.Advance(ctx, request)
	if err != nil {
		writeOwnerError(writer, err)
		return
	}
	if !handler.stillAuthorized(ctx, connection) {
		writeError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	response, err := domainsecurity.MonotonicHeadAdvanceReceiptV1Bytes(receipt)
	writeResult(writer, response, err)
}

func (handler *protocolHandler) resolveMutation(writer http.ResponseWriter, ctx context.Context,
	connection *tls.ConnectionState, body []byte) {
	request, err := domainsecurity.ParseMonotonicHeadMutationResolveRequestV1(body)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
		return
	}
	resolution, err := handler.owner.ResolveMutation(ctx, request)
	if err != nil {
		writeOwnerError(writer, err)
		return
	}
	if !handler.stillAuthorized(ctx, connection) {
		writeError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	response, err := domainsecurity.MonotonicHeadMutationResolutionV1Bytes(resolution)
	writeResult(writer, response, err)
}

func (handler *protocolHandler) stillAuthorized(ctx context.Context, connection *tls.ConnectionState) bool {
	if connection == nil {
		return false
	}
	now := time.Now()
	return validCertificateChainTime(handler.serverChain, now) && validPeerCertificateTime(*connection, now) &&
		currentAnchorMatches(ctx, handler.anchorSource, handler.expectedAnchor)
}

func writeResult(writer http.ResponseWriter, body []byte, err error) {
	if err != nil || len(body) == 0 || len(body) > bodyLimit {
		writeError(writer, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writer.Header().Set("Content-Type", protocolContentType)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(body)
}

func writeOwnerError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, monotonichead.ErrNotEnrolled):
		writeError(writer, http.StatusNotFound, "not_enrolled")
	case errors.Is(err, monotonichead.ErrCASConflict):
		writeError(writer, http.StatusConflict, "cas_conflict")
	case errors.Is(err, monotonichead.ErrMutationConflict):
		writeError(writer, http.StatusConflict, "mutation_conflict")
	case errors.Is(err, monotonichead.ErrInvalidReceipt):
		writeError(writer, http.StatusUnprocessableEntity, "invalid_receipt")
	default:
		// Unknown and indeterminate storage failures are unavailable on the
		// wire. Advance callers treat this as indeterminate and must resolve.
		writeError(writer, http.StatusServiceUnavailable, "unavailable")
	}
}

func writeError(writer http.ResponseWriter, status int, code string) {
	body, _ := json.Marshal(struct {
		SchemaVersion int    `json:"schemaVersion"`
		Purpose       string `json:"purpose"`
		Code          string `json:"code"`
	}{1, errorPurpose, code})
	writer.Header().Set("Content-Type", protocolContentType)
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
}

func exactHeader(header http.Header, name, value string) bool {
	values := header.Values(name)
	return len(values) == 1 && values[0] == value
}

func canonicalObserveRequest(body []byte, request domainsecurity.MonotonicHeadObserveRequestV1) bool {
	canonical, err := domainsecurity.MonotonicHeadObserveRequestV1Bytes(request)
	return err == nil && bytes.Equal(body, canonical)
}

func canonicalAdvanceRequest(body []byte, request domainsecurity.MonotonicHeadAdvanceRequestV1) bool {
	canonical, err := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(request)
	return err == nil && bytes.Equal(body, canonical)
}

func currentAnchorMatches(ctx context.Context, source authorityanchorport.Source,
	expected authorityanchorport.AnchorV1) bool {
	if ctx == nil || ctx.Err() != nil || source == nil {
		return false
	}
	current, err := source.Load(ctx)
	return err == nil && current.InstallationID == expected.InstallationID &&
		current.AuthorityKeyID == expected.AuthorityKeyID &&
		current.CurrentManifestDigest == expected.CurrentManifestDigest &&
		bytes.Equal(current.AuthorityPublicKey, expected.AuthorityPublicKey)
}

func fixedLoopbackAddress(origin string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "127.0.0.1" ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.String() != origin || parsed.Port() == "" ||
		!strings.HasPrefix(origin, "https://127.0.0.1:") {
		return "", errors.New("shared witness endpoint is not a fixed loopback HTTPS origin")
	}
	port, err := strconv.ParseUint(parsed.Port(), 10, 16)
	if err != nil || port == 0 || strconv.FormatUint(port, 10) != parsed.Port() {
		return "", errors.New("shared witness loopback port is invalid")
	}
	return parsed.Host, nil
}

func pinnedRoot(der []byte, enrolledDigest string) (*x509.Certificate, *x509.CertPool, error) {
	if len(der) == 0 || uint64(len(der)) > domaincredentials.MaxRootCABytesV1 {
		return nil, nil, errors.New("shared witness root CA size is invalid")
	}
	der = append([]byte(nil), der...)
	root, err := x509.ParseCertificate(der)
	if err != nil || root == nil || domainsecurity.SHA256Hex(der) != enrolledDigest ||
		!root.BasicConstraintsValid || !root.IsCA || root.KeyUsage&x509.KeyUsageCertSign == 0 ||
		root.KeyUsage & ^(x509.KeyUsageCertSign|x509.KeyUsageCRLSign) != 0 ||
		len(root.ExtKeyUsage) != 0 || len(root.UnknownExtKeyUsage) != 0 ||
		!validCertificateSignatureAlgorithm(root.SignatureAlgorithm) || !validCertificatePublicKey(root.PublicKey) {
		return nil, nil, errors.New("shared witness enrolled root CA is invalid")
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return root, pool, nil
}

func pinnedClientChain(chainDER [][]byte, enrolledLeafDigest string, root *x509.Certificate,
	roots *x509.CertPool) ([][]byte, error) {
	if len(chainDER) == 0 || len(chainDER) > domaincredentials.MaxClientChainCertificatesV1 {
		return nil, errors.New("shared witness client chain length is invalid")
	}
	var totalBytes uint64
	certificates := make([]*x509.Certificate, 0, len(chainDER))
	seenSubjects := map[string]struct{}{string(root.RawSubject): {}}
	seenKeys := map[string]struct{}{string(root.RawSubjectPublicKeyInfo): {}}
	for _, supplied := range chainDER {
		if len(supplied) == 0 || uint64(len(supplied)) > domaincredentials.MaxClientChainBytesV1-totalBytes {
			return nil, errors.New("shared witness client chain size is invalid")
		}
		totalBytes += uint64(len(supplied))
		der := append([]byte(nil), supplied...)
		certificate, err := x509.ParseCertificate(der)
		if err != nil || certificate == nil || !bytes.Equal(certificate.Raw, der) ||
			!validCertificateSignatureAlgorithm(certificate.SignatureAlgorithm) ||
			!validCertificatePublicKey(certificate.PublicKey) {
			return nil, errors.New("shared witness client chain contains an invalid certificate")
		}
		if _, duplicate := seenSubjects[string(certificate.RawSubject)]; duplicate {
			return nil, errors.New("shared witness client chain repeats a subject")
		}
		if _, duplicate := seenKeys[string(certificate.RawSubjectPublicKeyInfo)]; duplicate {
			return nil, errors.New("shared witness client chain repeats a public key")
		}
		seenSubjects[string(certificate.RawSubject)] = struct{}{}
		seenKeys[string(certificate.RawSubjectPublicKeyInfo)] = struct{}{}
		certificates = append(certificates, certificate)
	}
	leaf := certificates[0]
	if domainsecurity.SHA256Hex(leaf.Raw) != enrolledLeafDigest || !leaf.BasicConstraintsValid || leaf.IsCA ||
		leaf.KeyUsage != x509.KeyUsageDigitalSignature ||
		len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth ||
		len(leaf.UnknownExtKeyUsage) != 0 {
		return nil, errors.New("shared witness enrolled client certificate is invalid")
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range certificates[1:] {
		if !certificate.BasicConstraintsValid || !certificate.IsCA || certificate.KeyUsage&x509.KeyUsageCertSign == 0 ||
			certificate.KeyUsage & ^(x509.KeyUsageCertSign|x509.KeyUsageCRLSign) != 0 ||
			len(certificate.UnknownExtKeyUsage) != 0 ||
			(len(certificate.ExtKeyUsage) != 0 &&
				(len(certificate.ExtKeyUsage) != 1 || certificate.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth)) {
			return nil, errors.New("shared witness client intermediate policy is invalid")
		}
		intermediates.AddCert(certificate)
	}
	for index, certificate := range certificates {
		issuer := root
		if index+1 < len(certificates) {
			issuer = certificates[index+1]
		}
		if !bytes.Equal(certificate.RawIssuer, issuer.RawSubject) || certificate.CheckSignatureFrom(issuer) != nil {
			return nil, errors.New("shared witness client chain signature is invalid")
		}
	}
	verified, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}},
	)
	if err != nil || len(verified) != 1 || len(verified[0]) != len(certificates)+1 {
		return nil, errors.New("shared witness enrolled client chain cannot be verified")
	}
	for index, certificate := range certificates {
		if !bytes.Equal(verified[0][index].Raw, certificate.Raw) {
			return nil, errors.New("shared witness verified client chain differs from supplied chain")
		}
	}
	if !bytes.Equal(verified[0][len(certificates)].Raw, root.Raw) {
		return nil, errors.New("shared witness verified client root differs from enrollment")
	}
	pinned := make([][]byte, len(certificates))
	for index, certificate := range certificates {
		pinned[index] = append([]byte(nil), certificate.Raw...)
	}
	return pinned, nil
}

func validEnrolledClientConnection(state tls.ConnectionState, chainDER [][]byte, rootDER []byte) bool {
	if len(chainDER) == 0 || len(state.PeerCertificates) != len(chainDER) || len(state.VerifiedChains) != 1 ||
		len(state.VerifiedChains[0]) != len(chainDER)+1 ||
		!bytes.Equal(state.VerifiedChains[0][len(chainDER)].Raw, rootDER) {
		return false
	}
	for index, certificate := range state.PeerCertificates {
		if !bytes.Equal(certificate.Raw, chainDER[index]) ||
			!bytes.Equal(state.VerifiedChains[0][index].Raw, certificate.Raw) {
			return false
		}
	}
	return true
}

func validPeerCertificateTime(state tls.ConnectionState, now time.Time) bool {
	if len(state.VerifiedChains) != 1 {
		return false
	}
	for _, certificate := range state.VerifiedChains[0] {
		if !validCertificateTime(certificate, now) {
			return false
		}
	}
	return true
}

func validCertificateTime(certificate *x509.Certificate, now time.Time) bool {
	return certificate != nil && !now.Before(certificate.NotBefore) && !now.After(certificate.NotAfter)
}

func validCertificateChainTime(chain []*x509.Certificate, now time.Time) bool {
	if len(chain) < 2 {
		return false
	}
	for _, certificate := range chain {
		if !validCertificateTime(certificate, now) {
			return false
		}
	}
	return true
}

func pinnedServerCertificate(certificate tls.Certificate, roots *x509.CertPool,
	serverName string) (tls.Certificate, []*x509.Certificate, error) {
	signer, signerOK := certificate.PrivateKey.(crypto.Signer)
	if len(certificate.Certificate) == 0 || len(certificate.Certificate) > domaincredentials.MaxClientChainCertificatesV1 ||
		!signerOK || signer == nil {
		return tls.Certificate{}, nil, errors.New("shared witness server certificate is incomplete")
	}
	chainDER := make([][]byte, 0, len(certificate.Certificate))
	certificates := make([]*x509.Certificate, 0, len(certificate.Certificate))
	var totalBytes uint64
	for _, supplied := range certificate.Certificate {
		if len(supplied) == 0 || uint64(len(supplied)) > domaincredentials.MaxClientChainBytesV1-totalBytes {
			return tls.Certificate{}, nil, errors.New("shared witness server chain size is invalid")
		}
		totalBytes += uint64(len(supplied))
		der := append([]byte(nil), supplied...)
		parsed, err := x509.ParseCertificate(der)
		if err != nil || parsed == nil || !bytes.Equal(parsed.Raw, der) ||
			!validCertificateSignatureAlgorithm(parsed.SignatureAlgorithm) ||
			!validCertificatePublicKey(parsed.PublicKey) {
			return tls.Certificate{}, nil, errors.New("shared witness server chain contains an invalid certificate")
		}
		chainDER = append(chainDER, der)
		certificates = append(certificates, parsed)
	}
	leaf := certificates[0]
	if !leaf.BasicConstraintsValid || leaf.IsCA || leaf.KeyUsage != x509.KeyUsageDigitalSignature ||
		len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth ||
		len(leaf.UnknownExtKeyUsage) != 0 {
		return tls.Certificate{}, nil, errors.New("shared witness server certificate policy is invalid")
	}
	intermediates := x509.NewCertPool()
	for _, intermediate := range certificates[1:] {
		if !intermediate.BasicConstraintsValid || !intermediate.IsCA ||
			intermediate.KeyUsage&x509.KeyUsageCertSign == 0 ||
			intermediate.KeyUsage & ^(x509.KeyUsageCertSign|x509.KeyUsageCRLSign) != 0 ||
			len(intermediate.UnknownExtKeyUsage) != 0 ||
			(len(intermediate.ExtKeyUsage) != 0 &&
				(len(intermediate.ExtKeyUsage) != 1 || intermediate.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth)) {
			return tls.Certificate{}, nil, errors.New("shared witness server intermediate policy is invalid")
		}
		intermediates.AddCert(intermediate)
	}
	verified, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates,
		DNSName: serverName, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil || len(verified) != 1 || len(verified[0]) != len(certificates)+1 {
		return tls.Certificate{}, nil, errors.New("shared witness server certificate cannot be verified")
	}
	for index, parsed := range certificates {
		if !bytes.Equal(verified[0][index].Raw, parsed.Raw) {
			return tls.Certificate{}, nil, errors.New("shared witness verified server chain differs from supplied chain")
		}
	}
	leafKey, leafKeyErr := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	signerKey, signerKeyErr := x509.MarshalPKIXPublicKey(signer.Public())
	if leafKeyErr != nil || signerKeyErr != nil || !bytes.Equal(leafKey, signerKey) {
		return tls.Certificate{}, nil, errors.New("shared witness server private key does not match certificate")
	}
	// tls.Certificate carries a signer interface, which cannot be deep-copied.
	// Copy the public DER and retain the explicitly provided signer for this
	// server lifetime; TLS itself proves that it matches the leaf at handshake.
	return tls.Certificate{Certificate: chainDER,
		PrivateKey: certificate.PrivateKey, Leaf: leaf}, verified[0], nil
}

func validCertificateSignatureAlgorithm(algorithm x509.SignatureAlgorithm) bool {
	switch algorithm {
	case x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA,
		x509.SHA256WithRSAPSS, x509.SHA384WithRSAPSS, x509.SHA512WithRSAPSS,
		x509.ECDSAWithSHA256, x509.ECDSAWithSHA384, x509.ECDSAWithSHA512,
		x509.PureEd25519:
		return true
	default:
		return false
	}
}

func validCertificatePublicKey(publicKey any) bool {
	switch key := publicKey.(type) {
	case *rsa.PublicKey:
		return key != nil && key.N != nil &&
			(key.N.BitLen() == 2048 || key.N.BitLen() == 3072 || key.N.BitLen() == 4096) && key.E == 65537
	case *ecdsa.PublicKey:
		return key != nil && (key.Curve == elliptic.P256() || key.Curve == elliptic.P384() || key.Curve == elliptic.P521()) &&
			key.X != nil && key.Y != nil && key.Curve.IsOnCurve(key.X, key.Y)
	case ed25519.PublicKey:
		return len(key) == ed25519.PublicKeySize
	default:
		return false
	}
}

func minDuration(first, second time.Duration) time.Duration {
	if first < second {
		return first
	}
	return second
}
