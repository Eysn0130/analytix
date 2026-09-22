//go:build darwin && analytix_prod

package runtimeapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	Config           Config
	HostDataDir      string
	NativePath       string
	ParentPID        int
	ThreadID         string
	TurnID           string
	FinalDigest      string
	Account          string
	AdditionalFinals []b1RecoveryExpectedFinal
}

type b1RecoveryExpectedFinal struct {
	TurnID      string
	FinalDigest string
}

func b1AssertFreshProcessRecovery(t *testing.T, input b1RecoveryProcessInput) {
	t.Helper()
	input.ParentPID = os.Getpid()
	input.Config.APIKey = "" // recovery must not need a copied Provider credential
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
	output, err := command.CombinedOutput()
	if err != nil {
		// Helper failures use static labels; raw application responses stay private.
		t.Logf("fresh process diagnostic: %s", output)
		t.Fatal("fresh process fact recovery did not pass")
	}
	t.Log("new OS process: fresh witness, same thread GET200/final digest and protected local display passed")
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
		return openRev9DarwinHostCandidateForExecutable(dataDir, input.NativePath)
	}
	ctx := context.WithValue(context.Background(), bundledFundsHostValidationContextKeyV1{}, dependencies)
	admitted := false
	ctx = context.WithValue(ctx, factRecoveryObservationKeyV1{}, func(report factRecoveryObservationV1) {
		expected := 1 + len(input.AdditionalFinals)
		admitted = report.Candidates == expected && report.Admitted == expected && report.Held == 0
	})
	lease, err := AcquireRuntimePersistenceLease(input.Config)
	if err != nil {
		t.Fatal("fresh process persistence lease unavailable")
	}
	handler, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, input.Config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal("fresh process production assembly unavailable")
	}
	owned := &ownedPersistenceLeaseHandler{Handler: handler, lease: lease}
	runtime := httptest.NewServer(owned)
	defer func() { runtime.Close(); shutdownOwnedRuntimeHandler(t, owned) }()
	if !admitted {
		t.Fatal("fresh process did not obtain fresh fact admission")
	}
	client := &http.Client{Timeout: 45 * time.Second}
	thread := b1PublicChainRequest(t, client, runtime.URL, http.MethodGet, "/v1/threads/"+input.ThreadID, nil, http.StatusOK)
	turn := packagedSourceUnavailableHydrationTurnV1(thread, input.TurnID)
	final, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
	if err != nil || final.AcceptedFinalDigest != input.FinalDigest {
		t.Fatal("fresh process changed original final identity")
	}
	if len(input.AdditionalFinals) != 0 && turn["factHistoryState"] != "retained_snapshot" {
		t.Fatal("fresh process did not preserve the original snapshot history label")
	}
	for _, expected := range input.AdditionalFinals {
		currentTurn := packagedSourceUnavailableHydrationTurnV1(thread, expected.TurnID)
		currentFinal, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(currentTurn["acceptedFinalView"])
		if err != nil || currentFinal.AcceptedFinalDigest != expected.FinalDigest || currentTurn["factHistoryState"] != nil {
			t.Fatal("fresh process changed current snapshot final identity")
		}
	}
	public, _ := json.Marshal(thread)
	deliveryAssertPublic(t, public)
	display := b1PublicChainRequest(t, client, runtime.URL, http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{
		"kind": "accepted_slot_display", "threadId": input.ThreadID, "turnId": input.TurnID, "acceptedFinalDigest": input.FinalDigest, "displayMode": "full",
	}, http.StatusOK)
	slots, _ := display["slots"].([]any)
	if len(slots) != 1 || slots[0].(map[string]any)["displayValue"] != input.Account {
		t.Fatal("fresh process protected local source value differs")
	}
}
