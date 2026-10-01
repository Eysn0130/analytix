//go:build darwin && analytix_prod

package runtimeapp

import (
	"context"
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

	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

// Only synthetic configuration, fixture root paths and readback
// expectations cross this boundary. No process capability, private final, signing
// key or source handle is serialized. The child reopens the existing real stores
// and contacts the still-running independent witness service with a fresh client.
// Like its parent, this is test composition, not installed-app acceptance.
type b1RecoveryProcessInput struct {
	Config         Config
	HostDataDir    string
	NativePath     string
	ParentPID      int
	Account        string
	ExpectedFinals []b1RecoveryExpectedFinal
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
		"fact-admission-report", "fact-admission-pass", "fact-admission-fail", "thread-get-begin", "thread-get-end",
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

func b1AssertFreshProcessRecovery(t *testing.T, input b1RecoveryProcessInput) {
	t.Helper()
	input.ParentPID = os.Getpid()
	input.Config.APIKey = "" // recovery must not need a copied Provider credential
	if err := b1ValidateRecoveryExpectedFinals(input.ExpectedFinals); err != nil {
		t.Fatal(err)
	}
	root := runtimeWitnessedRegistryHostTempV2(t, "recovery-process")
	path := filepath.Join(root, "input.json")
	body, err := json.Marshal(input)
	if err != nil || len(body) > 1<<20 {
		t.Fatal("synthetic process input cannot be encoded within its bound")
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
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
	phase := func(name string) { b1EmitRecoveryPhase(b1RecoveryPhase{Phase: name}) }
	phase("input-ready")
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
	readOrdinal, displayOrdinal := 0, 0
	err = b1VisitRecoveryFinals(input.ExpectedFinals, func(threadID string) (map[string]any, error) {
		readOrdinal++
		b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "thread-get-begin", Ordinal: readOrdinal})
		thread := b1PublicChainRequest(t, client, runtime.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
		b1EmitRecoveryPhase(b1RecoveryPhase{Phase: "thread-get-end", Ordinal: readOrdinal})
		public, err := json.Marshal(thread)
		if err != nil {
			return nil, errors.New("fresh process public thread could not be encoded")
		}
		deliveryAssertPublic(t, public)
		return thread, nil
	}, func(expected b1RecoveryExpectedFinal, thread map[string]any) error {
		turn := packagedSourceUnavailableHydrationTurnV1(thread, expected.TurnID)
		final, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
		if err != nil || final.AcceptedFinalDigest != expected.FinalDigest {
			return errors.New("fresh process changed an expected final identity")
		}
		if (expected.HistoryState == "" && turn["factHistoryState"] != nil) || (expected.HistoryState != "" && turn["factHistoryState"] != expected.HistoryState) {
			return errors.New("fresh process changed an expected snapshot history label")
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
