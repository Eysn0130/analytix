//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

// Only synthetic configuration, fixture root paths and readback
// expectations cross this boundary. No process capability, private final, signing
// key or source handle is serialized. The child reopens the existing real stores
// and contacts the still-running independent witness service with a fresh client.
// Like its parent, this is test composition, not installed-app acceptance.
type b1RecoveryProcessInput struct {
	Config          Config
	HostDataDir     string
	NativePath      string
	ParentPID       int
	Account         string
	ExpectedFinals  []b1RecoveryExpectedFinal
	StagedRoot      string
	ProtectedDigest string
}

const b1StagedFixtureEnv = "ANALYTIX_FUNDS_STAGED_FIXTURE_ROOT"

// The caller owns these resources across both test processes. Readiness binds
// the original sidecar's bootstrap and key bytes; the normal signed, nonce-bound
// witness challenge still decides whether that authority is actually available.
func b1StagedAuthorityConfig(root string) (Config, string, error) {
	fail := func() (Config, string, error) {
		return Config{}, "", errors.New("staged original authority binding is invalid")
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fail()
	}
	resolved, err := filepath.EvalSymlinks(root)
	info, statErr := os.Lstat(root)
	if err != nil || resolved != root || statErr != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fail()
	}
	read := func(path string) ([]byte, error) {
		state, err := os.Lstat(path)
		if err != nil || !state.Mode().IsRegular() || state.Mode().Perm() != 0o600 || state.Size() > 1<<20 {
			return nil, errors.New("private staged authority file is invalid")
		}
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil || realPath != path {
			return nil, errors.New("staged authority file is redirected")
		}
		return os.ReadFile(path)
	}
	decode := func(body []byte, value any) bool {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		return decoder.Decode(value) == nil && decoder.Decode(new(any)) == io.EOF
	}
	var ready struct {
		SchemaVersion                  int    `json:"schemaVersion"`
		BootstrapSHA256                string `json:"bootstrapSha256"`
		InstallationAuthorityKeySHA256 string `json:"installationAuthorityKeySha256"`
	}
	readyBody, err := read(filepath.Join(root, "authority-ready.json"))
	if err != nil || !decode(readyBody, &ready) || ready.SchemaVersion != 1 {
		return fail()
	}
	ownerRoot := filepath.Join(root, "authority")
	bootstrapBody, err := read(filepath.Join(ownerRoot, "authority-bootstrap-v1.json"))
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(bootstrapBody)) != ready.BootstrapSHA256 {
		return fail()
	}
	var bootstrap struct {
		SchemaVersion                  int    `json:"schemaVersion"`
		Purpose                        string `json:"purpose"`
		AuthorityAnchorV1              string `json:"authorityAnchorV1"`
		AuthorityManifestRoot          string `json:"authorityManifestRoot"`
		AuthorityCredentialProfileRoot string `json:"authorityCredentialProfileRoot"`
		AuthorityCredentialBundleRoot  string `json:"authorityCredentialBundleRoot"`
	}
	if !decode(bootstrapBody, &bootstrap) || bootstrap.SchemaVersion != 1 || bootstrap.Purpose != "analytix.runtime-main-owned-authority/v1" ||
		bootstrap.AuthorityAnchorV1 == "" || bootstrap.AuthorityManifestRoot != filepath.Join(ownerRoot, "manifest") ||
		bootstrap.AuthorityCredentialProfileRoot != filepath.Join(ownerRoot, "credential-profile") || bootstrap.AuthorityCredentialBundleRoot != filepath.Join(ownerRoot, "credential-bundle") {
		return fail()
	}
	dataDir := filepath.Join(ownerRoot, "data")
	keyPath := filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json")
	keyBody, err := read(keyPath)
	if err != nil {
		return fail()
	}
	keyDigest := fmt.Sprintf("%x", sha256.Sum256(keyBody))
	clear(keyBody)
	if keyDigest != ready.InstallationAuthorityKeySHA256 {
		return fail()
	}
	authority, err := finalauthority.OpenExistingFileAuthority(keyPath)
	if err != nil {
		return fail()
	}
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, ProductionDurableRoot: filepath.Join(root, "durable"),
		ProviderID: "witnessed-registry-restart", BaseURL: "https://provider.invalid", Model: "witnessed-registry-restart-model", EndpointFormat: "chat_completions",
		AuthorityAnchorV1: bootstrap.AuthorityAnchorV1, AuthorityManifestRoot: bootstrap.AuthorityManifestRoot,
		AuthorityCredentialProfileRoot: bootstrap.AuthorityCredentialProfileRoot, AuthorityCredentialBundleRoot: bootstrap.AuthorityCredentialBundleRoot}
	return config, authority.KeyID(), nil
}

func b1WriteRecoveryInput(path string, input b1RecoveryProcessInput) error {
	if input.ParentPID <= 0 || input.Config.APIKey != "" || b1ValidateRecoveryExpectedFinals(input.ExpectedFinals) != nil {
		return errors.New("synthetic recovery input is invalid")
	}
	body, err := json.Marshal(input)
	if err != nil || len(body) > 1<<20 {
		return errors.New("synthetic recovery input exceeds its bound")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(body)
	if writeErr == nil && written != len(body) {
		writeErr = io.ErrShortWrite
	}
	return errors.Join(writeErr, file.Sync(), file.Close())
}

func b1StagedProtectedDigest(t *testing.T, input b1RecoveryProcessInput) string {
	return startupWholeTreeDigest(t,
		filepath.Join(input.Config.DataDir, "private", "evidence-registry"),
		filepath.Join(input.Config.DataDir, "private", "evidence-settlements"),
		filepath.Join(input.Config.DataDir, "private", "accepted-finals"),
		filepath.Join(input.Config.DataDir, "private", "dataset-snapshot-authority", "materials"),
		filepath.Join(input.Config.DataDir, "private", "dataset-snapshot-authority", "authority-bundles-v2"),
		filepath.Join(input.Config.DataDir, "private", "dataset-snapshot-authority", "indexes"),
		filepath.Join(input.Config.DataDir, "private", "dataset-snapshot-authority", "legacy-records"),
		filepath.Join(input.StagedRoot, "fixture", "synthetic-case"))
}

type b1RecoveryExpectedFinal struct {
	ThreadID     string
	TurnID       string
	FinalDigest  string
	HistoryState string // empty is current; retained_snapshot is historical
}

func b1ValidateRecoveryExpectedFinals(expected []b1RecoveryExpectedFinal) error {
	if len(expected) == 0 || len(expected) > 8 {
		return errors.New("fresh process final expectations are outside their bound")
	}
	seen := make(map[[2]string]bool)
	for _, final := range expected {
		key := [2]string{final.ThreadID, final.TurnID}
		if final.ThreadID == "" || final.TurnID == "" || len(final.FinalDigest) != 64 || strings.Trim(final.FinalDigest, "0123456789abcdef") != "" ||
			(final.HistoryState != "" && final.HistoryState != "retained_snapshot") || seen[key] {
			return errors.New("fresh process final expectations are incomplete or repeated")
		}
		seen[key] = true
	}
	return nil
}

func b1RecoveryAdmissionMatches(report factRecoveryObservationV1, expected []b1RecoveryExpectedFinal) bool {
	return b1ValidateRecoveryExpectedFinals(expected) == nil && report.Candidates == len(expected) && report.Admitted == len(expected) && report.Held == 0
}

// Each final is checked against its own thread. Reusing a GET is safe only for
// finals in that same thread; every tuple still gets an independent readback.
func b1VisitRecoveryFinals(expected []b1RecoveryExpectedFinal, read func(string) (map[string]any, error), check func(b1RecoveryExpectedFinal, map[string]any) error) error {
	if err := b1ValidateRecoveryExpectedFinals(expected); err != nil {
		return err
	}
	threads := make(map[string]map[string]any)
	for _, final := range expected {
		thread, ok := threads[final.ThreadID]
		if !ok {
			var err error
			thread, err = read(final.ThreadID)
			if err != nil {
				return err
			}
			threads[final.ThreadID] = thread
		}
		if err := check(final, thread); err != nil {
			return err
		}
	}
	return nil
}

// Closed test-only diagnostics preserve the strict identity/history checks.
// No response body, final digest or source identity enters the phase journal.
func b1RecoveryFinalReadbackFailure(expected b1RecoveryExpectedFinal, thread map[string]any) string {
	turn := packagedSourceUnavailableHydrationTurnV1(thread, expected.TurnID)
	if turn == nil {
		return "final-turn-missing"
	}
	if turn["acceptedFinalView"] == nil {
		return "final-view-missing"
	}
	final, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
	if err != nil {
		return "final-view-invalid"
	}
	if final.AcceptedFinalDigest != expected.FinalDigest {
		return "final-identity-mismatch"
	}
	if (expected.HistoryState == "" && turn["factHistoryState"] != nil) || (expected.HistoryState != "" && turn["factHistoryState"] != expected.HistoryState) {
		return "final-history-label-mismatch"
	}
	return ""
}

const b1RecoveryPhasePrefix = "ANALYTIX_TEST_RECOVERY_PHASE "

type b1RecoveryPhase struct {
	Phase      string `json:"phase"`
	Ordinal    int    `json:"ordinal"`
	Candidates int    `json:"candidates"`
	Admitted   int    `json:"admitted"`
	Held       int    `json:"held"`
}

func b1RecoveryPhaseAllowed(record b1RecoveryPhase) bool {
	switch record.Phase {
	case "input-ready", "lease-begin", "lease-end", "assembly-begin", "assembly-end", "native-owner-begin", "native-owner-end",
		"witness-begin", "witness-end", "witness-credentials-unavailable", "witness-request-invalid", "witness-unavailable", "witness-deadline", "witness-invalid",
		"fact-admission-report", "fact-admission-pass", "fact-admission-fail", "thread-get-begin", "thread-get-end",
		"final-turn-missing", "final-view-missing", "final-view-invalid", "final-identity-mismatch", "final-history-label-mismatch", "public-privacy-fail",
		"display-begin", "display-end", "readback-pass", "runtime-close-begin", "runtime-close-end", "owner-shutdown-begin", "owner-shutdown-end":
	default:
		return false
	}
	return record.Ordinal >= 0 && record.Ordinal <= 8 && record.Candidates >= 0 && record.Candidates <= 1_000_000 &&
		record.Admitted >= 0 && record.Admitted <= record.Candidates && record.Held >= 0 && record.Held <= record.Candidates
}

// Raw child stdout, stderr and panic stacks never enter the journal. Lines are
// streamed, bounded, schema checked and persisted before the child returns.
type b1RecoveryPhaseWriter struct {
	mu       sync.Mutex
	line     []byte
	dropping bool
	records  []b1RecoveryPhase
	persist  func([]byte) error
}

func (w *b1RecoveryPhaseWriter) Write(body []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range body {
		if b != '\n' {
			if !w.dropping && len(w.line) < 256 {
				w.line = append(w.line, b)
			} else {
				clear(w.line)
				w.line = w.line[:0]
				w.dropping = true
			}
			continue
		}
		if !w.dropping && len(w.records) < 64 && strings.HasPrefix(string(w.line), b1RecoveryPhasePrefix) {
			decoder := json.NewDecoder(strings.NewReader(string(w.line[len(b1RecoveryPhasePrefix):])))
			decoder.DisallowUnknownFields()
			var record b1RecoveryPhase
			if decoder.Decode(&record) == nil && decoder.Decode(new(any)) == io.EOF && b1RecoveryPhaseAllowed(record) {
				encoded, _ := json.Marshal(record)
				if err := w.persist(append(encoded, '\n')); err != nil {
					return 0, err
				}
				w.records = append(w.records, record)
			}
		}
		clear(w.line)
		w.line = w.line[:0]
		w.dropping = false
	}
	return len(body), nil
}

func b1EmitRecoveryPhase(record b1RecoveryPhase) {
	if !b1RecoveryPhaseAllowed(record) {
		return
	}
	body, _ := json.Marshal(record)
	_, _ = fmt.Fprintf(os.Stdout, "%s%s\n", b1RecoveryPhasePrefix, body)
}

// A retained fixture needs its original live authority, not just saved client
// credentials. This read-only fresh challenge fails before assembly when that
// prerequisite is absent; it never creates an identity or advances a head.
func b1RecoveryWitnessPhase(ctx context.Context, config Config) string {
	authority, err := finalauthority.OpenExistingFileAuthority(filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"))
	if err != nil {
		return "witness-credentials-unavailable"
	}
	enrolled, configured, err := loadRuntimeSharedEvidenceEnrollmentV2(ctx, config, authority)
	if ctx.Err() != nil {
		return "witness-deadline"
	}
	if err != nil || !configured || enrolled.credentials.SharedEvidence == nil {
		return "witness-credentials-unavailable"
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "witness-request-invalid"
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: enrolled.projection.InstallationID, EnrollmentID: enrolled.projection.Enrollment.EnrollmentID,
		Namespace: domainenrollment.SharedEvidenceNamespaceV1, ChallengeNonce: hex.EncodeToString(nonce[:]),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		if ctx.Err() != nil {
			return "witness-deadline"
		}
		return "witness-request-invalid"
	}
	_, err = enrolled.credentials.SharedEvidence.Observe(ctx, request)
	if ctx.Err() != nil {
		return "witness-deadline"
	}
	if errors.Is(err, monotonicheadport.ErrUnavailable) {
		return "witness-unavailable"
	}
	if err != nil {
		return "witness-invalid"
	}
	return "witness-end"
}

func b1AssertFreshProcessRecovery(t *testing.T, input b1RecoveryProcessInput) {
	t.Helper()
	input.ParentPID = os.Getpid()
	input.Config.APIKey = "" // recovery must not need a copied Provider credential
	if err := b1ValidateRecoveryExpectedFinals(input.ExpectedFinals); err != nil {
		t.Fatal(err)
	}
	root := runtimeWitnessedRegistryHostTempV2(t, "recovery-process")
	path := filepath.Join(root, "input.json")
	if err := b1WriteRecoveryInput(path, input); err != nil {
		t.Fatal("write private synthetic process input")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("resolve current test executable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestFundsRecoveryFreshProcessHelper$", "-test.v", "-test.timeout=11m")
	command.Env = append(os.Environ(), "ANALYTIX_FUNDS_RECOVERY_PROCESS_INPUT="+path)
	journal, err := os.OpenFile(filepath.Join(root, "phases.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal("create private fresh process phase journal")
	}
	defer journal.Close()
	phases := &b1RecoveryPhaseWriter{persist: func(body []byte) error {
		if _, err := journal.Write(body); err != nil {
			return err
		}
		return journal.Sync()
	}}
	command.Stdout, command.Stderr = phases, io.Discard
	err = command.Run()
	for _, record := range phases.records {
		t.Logf("fresh process phase=%s ordinal=%d candidates=%d admitted=%d held=%d", record.Phase, record.Ordinal, record.Candidates, record.Admitted, record.Held)
	}
	if err != nil {
		t.Fatal("fresh process fact recovery did not pass")
	}
	readbackPassed := false
	for _, record := range phases.records {
		readbackPassed = readbackPassed || record.Phase == "readback-pass"
	}
	if !readbackPassed || len(phases.records) == 0 || phases.records[len(phases.records)-1].Phase != "owner-shutdown-end" {
		t.Fatal("fresh process phase journal did not cover readback and shutdown")
	}
	t.Log("new OS process: fresh witness, every expected thread/final digest and protected local display passed")
}

func TestFundsRecoveryFreshProcessHelper(t *testing.T) {
	path := os.Getenv("ANALYTIX_FUNDS_RECOVERY_PROCESS_INPUT")
	if path == "" {
		t.Skip("synthetic subprocess helper is invoked by the owning chain")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > 1<<20 {
		t.Fatal("private synthetic process input is invalid")
	}
	body, err := os.ReadFile(path)
	var input b1RecoveryProcessInput
	if err != nil || json.Unmarshal(body, &input) != nil || input.ParentPID == os.Getpid() || input.ParentPID <= 0 || input.Config.APIKey != "" {
		t.Fatal("synthetic process identity or input is invalid")
	}
	if err := b1ValidateRecoveryExpectedFinals(input.ExpectedFinals); err != nil {
		t.Fatal(err)
	}
	if input.StagedRoot != "" {
		bound, _, err := b1StagedAuthorityConfig(input.StagedRoot)
		if err != nil || bound.DataDir != input.Config.DataDir || bound.ProductionDurableRoot != input.Config.ProductionDurableRoot ||
			bound.AuthorityAnchorV1 != input.Config.AuthorityAnchorV1 || bound.AuthorityManifestRoot != input.Config.AuthorityManifestRoot ||
			bound.AuthorityCredentialProfileRoot != input.Config.AuthorityCredentialProfileRoot || bound.AuthorityCredentialBundleRoot != input.Config.AuthorityCredentialBundleRoot ||
			input.HostDataDir != filepath.Join(input.StagedRoot, "host", "data") || len(input.ProtectedDigest) != 64 {
			t.Fatal("staged fresh process did not retain its original fixture binding")
		}
		if b1StagedProtectedDigest(t, input) != input.ProtectedDigest {
			t.Fatal("staged protected stores changed before fresh process")
		}
		defer func() {
			if b1StagedProtectedDigest(t, input) != input.ProtectedDigest {
				t.Error("fresh process changed original source, DSV2 or evidence/final stores")
			}
		}()
	} else if input.ProtectedDigest != "" {
		t.Fatal("protected staged digest has no fixture binding")
	}
	phase := func(name string) { b1EmitRecoveryPhase(b1RecoveryPhase{Phase: name}) }
	phase("input-ready")
	phase("witness-begin")
	witnessContext, cancelWitness := context.WithTimeout(context.Background(), 10*time.Second)
	witnessPhase := b1RecoveryWitnessPhase(witnessContext, input.Config)
	cancelWitness()
	phase(witnessPhase)
	if witnessPhase != "witness-end" {
		t.Fatal("fresh process original witness prerequisite failed: " + witnessPhase)
	}
	dependencies := defaultBundledFundsHostValidationDependenciesV1()
	dependencies.inspectPackage = func(ctx context.Context) (packagedauthorityfs.InspectionV2, error) {
		// Reconstruct the same declared synthetic package fixture from a fresh
		// source read, not a serialized inspection or process permission.
		runtimeHome := filepath.Dir(input.HostDataDir)
		sourceRoot := filepath.Join(runtimeHome, "package", "plugins", "analytix-fund-analysis")
		source, err := pluginstore.InspectSourceTreeV1(ctx, sourceRoot)
		if err != nil {
			return packagedauthorityfs.InspectionV2{}, err
		}
		return packagedauthorityfs.InspectionV2{
			ApplicationRunnerPath: filepath.Join(runtimeHome, "analytix.app", "Contents", "MacOS", "analytix"),
			PluginSourceRoot:      sourceRoot, PluginSourceIdentity: source, AuthorityFileSHA256: strings.Repeat("a", 64),
			Authority: domainauthority.ParsedAuthorityV2{
				Authority:  domainauthority.AuthorityV2{TargetKey: "darwin-arm64", AuthorityDigest: strings.Repeat("b", 64), Classification: "controlled_release_clean_candidate_non_publishable"},
				Controlled: &domainauthority.ControlledReleaseDispositionV2{Kind: domainauthority.ControlledDispositionKindV2},
			}, PackageAnchor: "macos_developer_id_resource_seal",
		}, nil
	}
	dependencies.inspectSource = pluginstore.InspectSourceTreeV1
	dependencies.resolveRuntimeRoots = func(string) (string, string, error) { return bundledFundsRuntimeRootsV1(input.HostDataDir) }
	dependencies.openNativeOwnerForTest = func(dataDir string) (*nativecomponenthost.Owner, error) {
		phase("native-owner-begin")
		owner, err := openRev9DarwinHostCandidateForExecutable(dataDir, input.NativePath)
		if err == nil && owner != nil {
			phase("native-owner-end")
		}
		return owner, err
	}
	ctx := context.WithValue(context.Background(), bundledFundsHostValidationContextKeyV1{}, dependencies)
	admitted := false
	ctx = context.WithValue(ctx, factRecoveryObservationKeyV1{}, func(report factRecoveryObservationV1) {
		b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "fact-admission-report", Candidates: report.Candidates, Admitted: report.Admitted, Held: report.Held})
		admitted = b1RecoveryAdmissionMatches(report, input.ExpectedFinals)
	})
	phase("lease-begin")
	lease, err := AcquireRuntimePersistenceLease(input.Config)
	if err != nil {
		t.Fatal("fresh process persistence lease unavailable")
	}
	phase("lease-end")
	phase("assembly-begin")
	handler, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, input.Config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal("fresh process production assembly unavailable")
	}
	phase("assembly-end")
	owned := &ownedPersistenceLeaseHandler{Handler: handler, lease: lease}
	runtime := httptest.NewServer(owned)
	defer func() {
		phase("runtime-close-begin")
		runtime.Close()
		phase("runtime-close-end")
		phase("owner-shutdown-begin")
		shutdownOwnedRuntimeHandler(t, owned)
		phase("owner-shutdown-end")
	}()
	if !admitted {
		phase("fact-admission-fail")
		t.Fatal("fresh process did not obtain fresh fact admission")
	}
	phase("fact-admission-pass")
	client := &http.Client{Timeout: 45 * time.Second}
	readOrdinal, finalOrdinal, displayOrdinal := 0, 0, 0
	err = b1VisitRecoveryFinals(input.ExpectedFinals, func(threadID string) (map[string]any, error) {
		readOrdinal++
		b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "thread-get-begin", Ordinal: readOrdinal})
		thread := b1PublicChainRequest(t, client, runtime.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
		b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "thread-get-end", Ordinal: readOrdinal})
		public, err := json.Marshal(thread)
		if err != nil {
			return nil, errors.New("fresh process public thread could not be encoded")
		}
		if !deliveryAssertPublic(t, public) {
			b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "public-privacy-fail", Ordinal: readOrdinal})
		}
		return thread, nil
	}, func(expected b1RecoveryExpectedFinal, thread map[string]any) error {
		finalOrdinal++
		if failure := b1RecoveryFinalReadbackFailure(expected, thread); failure != "" {
			b1EmitRecoveryPhase(b1RecoveryPhase{Phase: failure, Ordinal: finalOrdinal})
			return errors.New("fresh process expected final readback failed: " + failure)
		}

		displayOrdinal++
		b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "display-begin", Ordinal: displayOrdinal})
		display := b1PublicChainRequest(t, client, runtime.URL, http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{
			"kind": "accepted_slot_display", "threadId": expected.ThreadID, "turnId": expected.TurnID, "acceptedFinalDigest": expected.FinalDigest, "displayMode": "full",
		}, http.StatusOK)
		slots, _ := display["slots"].([]any)
		if len(slots) != 1 {
			return errors.New("fresh process protected local source cardinality differs")
		}
		slot, ok := slots[0].(map[string]any)
		if !ok || slot["displayValue"] != input.Account {
			return errors.New("fresh process protected local source value differs")
		}
		b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "display-end", Ordinal: displayOrdinal})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	phase("readback-pass")
}
