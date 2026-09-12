//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authoritycompositionfixture "analytix.local/runtime-go/internal/formalauthority"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	"analytix.local/runtime-go/internal/server"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestRuntimeSharedEvidenceDatasetSnapshotV2OutageKeepsHTTPHealthyAndRecoversSameProcess(t *testing.T) {
	ctx := context.Background()
	fixture, err := authoritycompositionfixture.New(runtimeWitnessedRegistryHostTempV2(t, "outage"))
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if blocker := authoritycompositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
	}
	config := Config{
		RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: t.TempDir(), DataDir: fixture.DataDir,
		ProviderID: "shared-evidence-http", BaseURL: "https://provider.invalid", APIKey: "test-only",
		Model: "shared-evidence-http-model", EndpointFormat: "chat_completions",
		AuthorityAnchorV1: fixture.AnchorEnvelope, AuthorityManifestRoot: fixture.ManifestRoot,
		AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot,
		AuthorityCredentialBundleRoot:  fixture.CredentialBundleRoot,
	}

	privateRoot := filepath.Join(fixture.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotStores, err := datasetsnapshotstore.OpenStoresV2(
		filepath.Join(privateRoot, "dataset-snapshot-authority"), access,
	)
	if err != nil {
		t.Fatal(err)
	}
	evidenceStores, err := openRuntimeSharedEvidenceStoresV2(fixture.DataDir, access)
	if err != nil {
		t.Fatal(err)
	}
	composition, configured, err := newRuntimeSharedEvidenceDatasetSnapshotV2(
		ctx, config, fixture.Authority, snapshotStores, evidenceStores,
		access, filestore.CaseBindingReader{}, nil,
	)
	if err != nil || !configured || composition.evidence == nil || composition.snapshot == nil || composition.registry != nil {
		t.Fatalf("local shared evidence composition = %#v configured=%t err=%v", composition, configured, err)
	}
	if attempts := fixture.TotalAttempts(); attempts != 0 {
		t.Fatalf("local composition reached a witness: attempts=%d", attempts)
	}

	// This explicit initialization and exact material seed are test-only
	// provisioning. Production HEAD has no supported host admission ingress;
	// the public seam below verifies only already-admitted resolution behavior.
	if _, err := composition.evidence.Initialize(ctx); err != nil {
		t.Fatalf("test-only initialize shared EvidenceAuthority: %v", err)
	}
	workspace := t.TempDir()
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	manifestReference, producerReference, materials := runtimeSharedEvidenceBoundMaterialsV2(
		t, observation.WorkspaceRealPath, observation, fixture.InstallationID, fixture.Authority,
	)
	materialCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(privateRoot, "dataset-snapshot-authority", "materials"), 16*1024*1024, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, records := range materials {
		for address, body := range records {
			if err := materialCAS.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
				t.Fatalf("seed exact test material %s: %v", address, err)
			}
		}
	}
	resolved, err := composition.snapshot.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation,
		ManifestReference: manifestReference, FundsProducerReference: producerReference,
		AcceptedAt: time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("test-only exact admission: %v", err)
	}
	baselineAttempts := fixture.TotalAttempts()
	if baselineAttempts == 0 || !fixture.SetWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatalf("shared witness outage setup failed: attempts=%d", baselineAttempts)
	}

	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("witness outage blocked ordinary runtime startup: %v", err)
	}
	if attempts := fixture.TotalAttempts(); attempts != baselineAttempts {
		t.Fatalf("runtime startup reached a witness: before=%d after=%d", baselineAttempts, attempts)
	}
	server := httptest.NewServer(handler)
	defer func() {
		server.Close()
		shutdownOwnedRuntimeHandler(t, handler)
	}()

	status, health := runtimeSharedEvidenceHTTPJSON(t, server.URL, http.MethodGet, "/health", nil)
	if status != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("ordinary health during witness outage: status=%d body=%#v", status, health)
	}
	resolveInput := datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, ExpectedDatasetSnapshotID: resolved.Record.DatasetSnapshotID,
	}
	if selected, resolveErr := composition.snapshot.ResolveWitnessedV2(ctx, resolveInput); resolveErr == nil {
		t.Fatalf("witness outage returned protected DSV2 authority: %#v", selected)
	}
	if attempts := fixture.TotalAttempts(); attempts <= baselineAttempts {
		t.Fatalf("protected DSV2 resolution did not reach the unavailable witness: before=%d after=%d", baselineAttempts, attempts)
	}

	beforeRecoveryAttempts := fixture.TotalAttempts()
	if !fixture.SetWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, true) {
		t.Fatal("shared witness recovery setup failed")
	}
	selected, err := composition.snapshot.ResolveWitnessedV2(ctx, resolveInput)
	if err != nil || selected.Record.RecordDigest != resolved.Record.RecordDigest {
		t.Fatalf("same-process witness recovery did not restore exact DSV2 authority: selected=%#v err=%v", selected, err)
	}
	if attempts := fixture.TotalAttempts(); attempts <= beforeRecoveryAttempts {
		t.Fatalf("recovered protected effect did not freshly challenge witness: before=%d after=%d", beforeRecoveryAttempts, attempts)
	}
}

func TestRuntimeWitnessedEvidenceRegistryV2RestartAndOptionalCapabilityRecovery(t *testing.T) {
	t.Run("cold absent direct composition keeps registry disabled", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		runtimeWitnessedRegistryAssertAbsentV2(t, root)

		composition := runtimeWitnessedRegistryDirectCompositionV2(t, fixture, config)
		if composition.evidence == nil || composition.snapshot == nil || composition.registry != nil {
			t.Fatalf("cold absent registry was constructed as a writable service: %#v", composition)
		}
		runtimeWitnessedRegistryAssertAbsentV2(t, root)
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("cold absent direct composition reached witness authority: attempts=%d", attempts)
		}
	})

	t.Run("complete generation zero direct composition keeps registry disabled", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		for _, leaf := range []string{"indexes", "capsules"} {
			if err := os.MkdirAll(filepath.Join(root, leaf), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		before := runtimeWitnessedRegistryEmptyRootIdentityV2(t, root)
		beforeTree := startupWholeTreeDigest(t, root)

		composition := runtimeWitnessedRegistryDirectCompositionV2(t, fixture, config)
		if composition.evidence == nil || composition.snapshot == nil || composition.registry != nil {
			t.Fatalf("generation-zero registry was constructed as a writable service: %#v", composition)
		}
		after := runtimeWitnessedRegistryEmptyRootIdentityV2(t, root)
		for name, first := range before {
			if !os.SameFile(first, after[name]) {
				t.Fatalf("generation-zero direct composition changed %s identity", name)
			}
		}
		if afterTree := startupWholeTreeDigest(t, root); afterTree != beforeTree {
			t.Fatalf("generation-zero direct composition changed registry tree: before=%s after=%s", beforeTree, afterTree)
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("generation-zero direct composition reached witness authority: attempts=%d", attempts)
		}
	})

	t.Run("ordinary durable turn restarts without registry authority", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		var providerCalls atomic.Int64
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			providerCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "isolated ordinary provider failure"}})
		}))
		defer provider.Close()
		config.BaseURL = provider.URL + "/v1"
		seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "synthetic-registry-restart-credential")
		registryRoot := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		runtimeWitnessedRegistryAssertAbsentV2(t, registryRoot)
		initial, err := NewRuntimeServerHandlerE(config)
		if err != nil {
			t.Fatal(err)
		}
		if providerCalls.Load() != 0 || fixture.TotalAttempts() != 0 {
			t.Fatalf("cold absent startup reached provider or witness: provider=%d witness=%d", providerCalls.Load(), fixture.TotalAttempts())
		}
		server := httptest.NewServer(initial)
		status, thread := runtimeSharedEvidenceHTTPJSON(t, server.URL, http.MethodPost, "/v1/threads", map[string]any{
			"title": "ordinary witnessed registry restart", "workspace": t.TempDir(),
			"providerId": config.ProviderID, "model": config.Model,
		})
		threadID, _ := thread["id"].(string)
		if status != http.StatusCreated || threadID == "" {
			t.Fatalf("create ordinary restart thread status=%d body=%#v", status, thread)
		}
		status, turn := runtimeSharedEvidenceHTTPJSON(
			t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
			map[string]any{"prompt": "只说明当前代码结构"},
		)
		if status != http.StatusInternalServerError || turn["reasonCode"] != "provider_unavailable" || providerCalls.Load() == 0 {
			t.Fatalf("ordinary turn did not reach the isolated unavailable provider: status=%d calls=%d body=%#v", status, providerCalls.Load(), turn)
		}
		server.Close()
		shutdownOwnedRuntimeHandler(t, initial)
		runtimeWitnessedRegistryAssertAbsentV2(t, registryRoot)
		turnID, before := runtimeWitnessedRegistryLatestFrozenContextV2(t, config.ProductionDurableRoot, threadID)
		if !domainsecurity.TurnSecurityContextIsGeneral(before) {
			t.Fatalf("ordinary persisted turn acquired case authority: %#v", before)
		}
		providerBeforeRestart := providerCalls.Load()

		restarted, err := NewRuntimeServerHandlerE(config)
		if err != nil {
			t.Fatalf("configured V2 restart forced ordinary turn through registry authority: %v", err)
		}
		restartedServer := httptest.NewServer(restarted)
		defer func() {
			restartedServer.Close()
			shutdownOwnedRuntimeHandler(t, restarted)
		}()
		status, health := runtimeSharedEvidenceHTTPJSON(t, restartedServer.URL, http.MethodGet, "/health", nil)
		if status != http.StatusOK || health["status"] != "ok" {
			t.Fatalf("ordinary restart health status=%d body=%#v", status, health)
		}
		status, detail := runtimeSharedEvidenceHTTPJSON(t, restartedServer.URL, http.MethodGet, "/v1/threads/"+threadID, nil)
		if status != http.StatusOK || detail["id"] != threadID {
			t.Fatalf("ordinary persisted thread was unavailable after restart: status=%d body=%#v", status, detail)
		}
		if providerCalls.Load() != providerBeforeRestart {
			t.Fatalf("cold absent restart replayed the ordinary provider: before=%d after=%d", providerBeforeRestart, providerCalls.Load())
		}
		after := runtimeWitnessedRegistryFrozenContextV2(t, config.ProductionDurableRoot, threadID, turnID)
		if !domainsecurity.TurnSecurityContextIsGeneral(after) || after.ContextDigest != before.ContextDigest {
			t.Fatalf("configured V2 restart changed ordinary turn authority: before=%#v after=%#v", before, after)
		}
		runtimeWitnessedRegistryAssertAbsentV2(t, registryRoot)
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("cold absent registry startup reached witness authority: attempts=%d", attempts)
		}
	})

	seedLegacyV1 := func(t *testing.T) (
		*authoritycompositionfixture.Service,
		Config,
		Config,
		string,
		domainsecurity.TurnSecurityContext,
		domainevidence.EvidenceReceipt,
	) {
		t.Helper()
		fixture, witnessedConfig := runtimeWitnessedRegistryConfigV2(t)
		legacyConfig := witnessedConfig
		legacyConfig.AuthorityAnchorV1 = ""
		legacyConfig.AuthorityManifestRoot = ""
		legacyConfig.AuthorityCredentialProfileRoot = ""
		legacyConfig.AuthorityCredentialBundleRoot = ""

		initial, err := NewRuntimeServerHandlerE(legacyConfig)
		if err != nil {
			t.Fatalf("create no-enrollment legacy runtime: %v", err)
		}
		shutdownOwnedRuntimeHandler(t, initial)

		securityContext := seedRuntimeDurableLegacyRegistryContextV1(
			t, legacyConfig.ProductionDurableRoot, fixture.DataDir, fixture.Authority,
		)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		legacy, err := evidenceregistrystore.NewStore(root, fixture.Authority)
		if err != nil {
			t.Fatal(err)
		}
		input := runtimeLegacyRegistryPreparedInputForContextV1(t, securityContext)
		receipt, err := legacy.CommitPrepared(context.Background(), input)
		if err != nil {
			t.Fatalf("commit legal V1 receipt before restart: %v", err)
		}
		return fixture, legacyConfig, witnessedConfig, root, securityContext, receipt
	}

	t.Run("legacy V1 receipt remains usable without enrollment restart", func(t *testing.T) {
		fixture, legacyConfig, _, root, securityContext, receipt := seedLegacyV1(t)
		restarted, err := NewRuntimeServerHandlerE(legacyConfig)
		if err != nil {
			t.Fatalf("no-enrollment restart rejected legal V1 registry: %v", err)
		}
		shutdownOwnedRuntimeHandler(t, restarted)
		reopened, err := evidenceregistrystore.NewStore(root, fixture.Authority)
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := reopened.Resolve(context.Background(), registryport.MembershipQuery{
			Context: securityContext, ReceiptID: receipt.ReceiptID,
		})
		if err != nil || resolved.Receipt.ReceiptID != receipt.ReceiptID {
			t.Fatalf("fresh legacy adapter lost current V1 receipt: resolved=%#v err=%v", resolved, err)
		}
		replayed, err := reopened.Replay(context.Background(), securityContext)
		if err != nil || replayed.Sequence != 1 || len(replayed.Entries) != 1 || replayed.Entries[0].ReceiptID != receipt.ReceiptID {
			t.Fatalf("no-enrollment restart lost V1 replay: replay=%#v err=%v", replayed, err)
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("no-enrollment V1 restart reached shared witness authority: attempts=%d", attempts)
		}
	})

	t.Run("legacy V1 receipt cannot enter configured V2", func(t *testing.T) {
		fixture, witnessedConfig := runtimeWitnessedRegistryConfigV2(t)
		legacyConfig := witnessedConfig
		legacyConfig.AuthorityAnchorV1 = ""
		legacyConfig.AuthorityManifestRoot = ""
		legacyConfig.AuthorityCredentialProfileRoot = ""
		legacyConfig.AuthorityCredentialBundleRoot = ""

		initial, err := NewRuntimeServerHandlerE(legacyConfig)
		if err != nil {
			t.Fatalf("create no-enrollment legacy runtime: %v", err)
		}
		shutdownOwnedRuntimeHandler(t, initial)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		legacy, err := evidenceregistrystore.NewStore(root, fixture.Authority)
		if err != nil {
			t.Fatal(err)
		}
		_, input := runtimeLegacyRegistryPreparedInputV1(t)
		if _, err := legacy.CommitPrepared(context.Background(), input); err != nil {
			t.Fatalf("commit legal standalone V1 receipt before V2 switch: %v", err)
		}
		beforeV2 := startupWholeTreeDigest(t, root)

		runtimeWitnessedRegistryStartAndStopHealthyV2(t, witnessedConfig)
		if afterV2 := startupWholeTreeDigest(t, root); afterV2 != beforeV2 {
			t.Fatalf("configured V2 degradation changed frozen V1 bytes: before=%s after=%s", beforeV2, afterV2)
		}
		for _, leaf := range []string{"indexes", "capsules"} {
			if _, err := os.Lstat(filepath.Join(root, leaf)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("configured V2 degradation manufactured %s over V1 lineage: %v", leaf, err)
			}
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("frozen V1 degradation reached shared witness authority: attempts=%d", attempts)
		}
	})

	t.Run("complete generation zero pair remains uninitialized", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		for _, leaf := range []string{"indexes", "capsules"} {
			if err := os.MkdirAll(filepath.Join(root, leaf), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		before := runtimeWitnessedRegistryEmptyRootIdentityV2(t, root)
		beforeTree := startupWholeTreeDigest(t, root)
		authorityBody, err := os.ReadFile(fixture.AuthorityPath)
		if err != nil {
			t.Fatal(err)
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("production startup reached shared witness authority: attempts=%d", attempts)
		}

		runtimeWitnessedRegistryAssertCanonicalOpaqueBlockerV2(t, fixture, config)
		after := runtimeWitnessedRegistryEmptyRootIdentityV2(t, root)
		for name, first := range before {
			if !os.SameFile(first, after[name]) {
				t.Fatalf("witnessed registry %s authority identity changed across restart", name)
			}
		}
		if afterTree := startupWholeTreeDigest(t, root); afterTree != beforeTree {
			t.Fatalf("generation-zero V2 pair changed during degraded startup: before=%s after=%s", beforeTree, afterTree)
		}
		afterAuthorityBody, err := os.ReadFile(fixture.AuthorityPath)
		if err != nil || !bytes.Equal(authorityBody, afterAuthorityBody) {
			t.Fatalf("installation authority changed across witnessed registry restart: %v", err)
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("generation-zero pair selected authority by a witness side effect: attempts=%d", attempts)
		}
	})

	t.Run("frozen legacy V1 lineage is not promoted", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		legacyCapsules := filepath.Join(root, ".registry-capsules")
		legacyProjections := filepath.Join(root, ".registry-projections")
		if err := os.MkdirAll(legacyCapsules, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(legacyProjections, 0o700); err != nil {
			t.Fatal(err)
		}
		legacyIndex := filepath.Join(root, ".registry-authority-index.json")
		legacyBody := []byte(`{"schemaVersion":1,"purpose":"analytix.evidence-registry-index/v1","frozen":true}`)
		if err := os.WriteFile(legacyIndex, legacyBody, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".registry.lock"), nil, 0o600); err != nil {
			t.Fatal(err)
		}

		runtimeWitnessedRegistryStartAndStopHealthyV2(t, config)
		retained, err := os.ReadFile(legacyIndex)
		if err != nil || !bytes.Equal(retained, legacyBody) {
			t.Fatalf("degraded startup rewrote frozen V1 lineage: %v", err)
		}
		for _, leaf := range []string{"indexes", "capsules"} {
			if _, err := os.Lstat(filepath.Join(root, leaf)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("legacy V1 lineage was promoted into V2 %s: %v", leaf, err)
			}
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("legacy V1 rejection reached shared witness authority: attempts=%d", attempts)
		}
	})

	t.Run("legacy authority-index crash residue degrades only case authority", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		residue := filepath.Join(root, ".registry-authority-index-deadbeef.tmp")
		body := []byte("frozen-unknown-v1-crash-residue")
		if err := os.WriteFile(residue, body, 0o600); err != nil {
			t.Fatal(err)
		}

		before := startupWholeTreeDigest(t, root)
		runtimeWitnessedRegistryAssertBlockedCaseAndOrdinaryRestartV2(t, fixture, config)
		retained, err := os.ReadFile(residue)
		if err != nil || !bytes.Equal(retained, body) {
			t.Fatalf("degraded restart deleted or changed unknown V1 crash residue: %v", err)
		}
		if after := startupWholeTreeDigest(t, root); after != before {
			t.Fatalf("degraded restart changed unknown V1 crash residue topology: before=%s after=%s", before, after)
		}
		for _, leaf := range []string{"indexes", "capsules"} {
			if _, err := os.Lstat(filepath.Join(root, leaf)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unknown V1 crash residue was promoted into %s: %v", leaf, err)
			}
		}
	})

	t.Run("legacy thread directory is frozen without semantic use", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		legacyThread := filepath.Join(root, "thread-frozen-v1")
		if err := os.MkdirAll(legacyThread, 0o700); err != nil {
			t.Fatal(err)
		}
		body := []byte("opaque-legacy-thread-record")
		if err := os.WriteFile(filepath.Join(legacyThread, "turn-opaque.bin"), body, 0o600); err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(legacyThread)
		if err != nil {
			t.Fatal(err)
		}
		beforeDigest := startupWholeTreeDigest(t, root)

		runtimeWitnessedRegistryStartAndStopHealthyV2(t, config)
		after, err := os.Stat(legacyThread)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("degraded startup changed the frozen legacy thread identity: %v", err)
		}
		if afterDigest := startupWholeTreeDigest(t, root); afterDigest != beforeDigest {
			t.Fatalf("degraded startup changed the frozen legacy thread bytes: before=%s after=%s", beforeDigest, afterDigest)
		}
		for _, leaf := range []string{"indexes", "capsules"} {
			if _, err := os.Lstat(filepath.Join(root, leaf)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("legacy thread residue manufactured %s: %v", leaf, err)
			}
		}
	})

	for _, presentLeaf := range []string{"indexes", "capsules"} {
		presentLeaf := presentLeaf
		t.Run("partial V2 "+presentLeaf+" leaf is frozen and unavailable", func(t *testing.T) {
			fixture, config := runtimeWitnessedRegistryConfigV2(t)
			root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			maxBytes := 16 << 20
			if presentLeaf == "indexes" {
				maxBytes = 256 << 10
			}
			leaf, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
				filepath.Join(root, presentLeaf), maxBytes, access,
			)
			if err != nil {
				t.Fatal(err)
			}
			body := []byte("canonical opaque partial " + presentLeaf + " bytes are not JSON")
			digest := domainsecurity.SHA256Hex(body)
			if err := leaf.PutIfAbsent(context.Background(), digest, body); err != nil {
				t.Fatal(err)
			}
			if err := leaf.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(filepath.Join(root, presentLeaf))
			if err != nil {
				t.Fatal(err)
			}
			beforeTree := startupWholeTreeDigest(t, root)

			runtimeWitnessedRegistryAssertCanonicalOpaqueBlockerV2(t, fixture, config)
			after, err := os.Stat(filepath.Join(root, presentLeaf))
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("degraded startup changed the partial %s identity: %v", presentLeaf, err)
			}
			if afterTree := startupWholeTreeDigest(t, root); afterTree != beforeTree {
				t.Fatalf("degraded startup changed partial %s whole tree: before=%s after=%s", presentLeaf, beforeTree, afterTree)
			}
			reopened, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
				filepath.Join(root, presentLeaf), maxBytes, access,
			)
			if err != nil {
				t.Fatal(err)
			}
			retained, readErr := reopened.Read(context.Background(), digest)
			closeErr := reopened.Close()
			if readErr != nil || closeErr != nil || !bytes.Equal(retained, body) {
				t.Fatalf("degraded startup changed partial %s bytes: read=%v close=%v", presentLeaf, readErr, closeErr)
			}
			missingLeaf := "capsules"
			if presentLeaf == missingLeaf {
				missingLeaf = "indexes"
			}
			if _, err := os.Lstat(filepath.Join(root, missingLeaf)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("degraded startup manufactured missing %s leaf: %v", missingLeaf, err)
			}
			if attempts := fixture.TotalAttempts(); attempts != 0 {
				t.Fatalf("partial V2 degradation reached shared witness authority: attempts=%d", attempts)
			}
		})
	}

	t.Run("complete non-JSON V2 pair is frozen and unavailable", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		access, err := privatecastest.NewAccessAuthority(root)
		if err != nil {
			t.Fatal(err)
		}
		bodies := map[string][]byte{
			"indexes":  []byte("canonical opaque index body is not JSON"),
			"capsules": []byte("canonical opaque capsule body is not JSON"),
		}
		identities := make(map[string]os.FileInfo, 5)
		for _, leafName := range []string{"indexes", "capsules"} {
			maxBytes := 16 << 20
			if leafName == "indexes" {
				maxBytes = 256 << 10
			}
			leaf, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
				filepath.Join(root, leafName), maxBytes, access,
			)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex(bodies[leafName])
			if err := leaf.PutIfAbsent(context.Background(), digest, bodies[leafName]); err != nil {
				t.Fatal(err)
			}
			if err := leaf.Close(); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{
				filepath.Join(root, leafName),
				filepath.Join(root, leafName, digest[:2], digest+".json"),
			} {
				identities[path], err = os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		identities[root], err = os.Stat(root)
		if err != nil {
			t.Fatal(err)
		}
		beforeTree := startupWholeTreeDigest(t, root)

		runtimeWitnessedRegistryAssertCanonicalOpaqueBlockerV2(t, fixture, config)
		if afterTree := startupWholeTreeDigest(t, root); afterTree != beforeTree {
			t.Fatalf("degraded startup changed complete non-JSON V2 tree: before=%s after=%s", beforeTree, afterTree)
		}
		for path, before := range identities {
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("degraded startup changed complete non-JSON V2 identity %s: %v", path, err)
			}
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 2 || entries[0].Name() != "capsules" || entries[1].Name() != "indexes" {
			t.Fatalf("degraded startup manufactured evidence-registry siblings: entries=%v err=%v", entries, err)
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("complete non-JSON V2 degradation reached shared witness authority: attempts=%d", attempts)
		}
	})

	t.Run("corrupt V2 leaf record is frozen and unavailable", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		access, err := privatecastest.NewAccessAuthority(root)
		if err != nil {
			t.Fatal(err)
		}
		indexes, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
			filepath.Join(root, "indexes"), 256<<10, access,
		)
		if err != nil {
			t.Fatal(err)
		}
		capsules, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
			filepath.Join(root, "capsules"), 16<<20, access,
		)
		if err != nil {
			t.Fatal(err)
		}
		corrupt := []byte(`{"not":"an evidence registry V2 index"}`)
		digest := domainsecurity.SHA256Hex(corrupt)
		if err := indexes.PutIfAbsent(context.Background(), digest, corrupt); err != nil {
			t.Fatal(err)
		}
		if err := indexes.Close(); err != nil {
			t.Fatal(err)
		}
		if err := capsules.Close(); err != nil {
			t.Fatal(err)
		}
		corruptPath := filepath.Join(root, "indexes", digest[:2], digest+".json")
		corruptBefore, err := os.Stat(corruptPath)
		if err != nil {
			t.Fatal(err)
		}
		residuePrefix := "fe"
		if digest[:2] == residuePrefix {
			residuePrefix = "fd"
		}
		residuePath := runtimePrivateCASRecoveryTemp(
			t, fixture.DataDir, "evidence-registry", "indexes", residuePrefix+strings.Repeat("a", 62),
		)
		residueBefore, err := os.Stat(residuePath)
		if err != nil {
			t.Fatal(err)
		}
		residueBody, err := os.ReadFile(residuePath)
		if err != nil {
			t.Fatal(err)
		}
		privateAccess, err := privatecastest.NewAccessAuthority(filepath.Join(fixture.DataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		for _, leaf := range []string{"dispositions", "records"} {
			store, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
				filepath.Join(fixture.DataDir, "private", "accepted-finals", leaf), 16<<20, privateAccess,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
		}
		siblingResiduePath := runtimePrivateCASRecoveryTemp(
			t, fixture.DataDir, "accepted-finals", "records", "fc"+strings.Repeat("b", 62),
		)
		beforeTree := startupWholeTreeDigest(t, root)

		runtimeWitnessedRegistryAssertBlockedCaseAndOrdinaryRestartV2(t, fixture, config)
		if reopened, reopenErr := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
			filepath.Join(root, "indexes"), 256<<10, access,
		); reopenErr == nil {
			_ = reopened.Close()
			t.Fatal("blocked V2 CAS with a frozen residue reopened as usable authority")
		}
		corruptAfter, err := os.Stat(corruptPath)
		retained, readCorruptErr := os.ReadFile(corruptPath)
		if err != nil || readCorruptErr != nil || !os.SameFile(corruptBefore, corruptAfter) ||
			!bytes.Equal(retained, corrupt) {
			t.Fatalf("degraded V2 recovery rewrote the corrupt record: stat=%v read=%v", err, readCorruptErr)
		}
		residueAfter, err := os.Stat(residuePath)
		retainedResidue, readErr := os.ReadFile(residuePath)
		if err != nil || readErr != nil || !os.SameFile(residueBefore, residueAfter) ||
			!bytes.Equal(retainedResidue, residueBody) {
			t.Fatalf("blocked V2 recovery consumed or rewrote plain residue: stat=%v read=%v", err, readErr)
		}
		if afterTree := startupWholeTreeDigest(t, root); afterTree != beforeTree {
			t.Fatalf("cross-participant recovery changed frozen registry tree: before=%s after=%s", beforeTree, afterTree)
		}
		if _, err := os.Lstat(siblingResiduePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("non-frozen accepted-final sibling residue did not converge: %v", err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 2 || entries[0].Name() != "capsules" || entries[1].Name() != "indexes" {
			t.Fatalf("cross-participant recovery manufactured evidence-registry siblings: entries=%v err=%v", entries, err)
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("corrupt V2 degradation reached shared witness authority: attempts=%d", attempts)
		}
	})

	t.Run("nonempty bound sibling is frozen and unavailable", func(t *testing.T) {
		fixture, config := runtimeWitnessedRegistryConfigV2(t)
		root := filepath.Join(fixture.DataDir, "private", "evidence-registry")
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		lock := filepath.Join(root, ".registry.lock")
		body := []byte("not-an-empty-bound-lock")
		if err := os.WriteFile(lock, body, 0o600); err != nil {
			t.Fatal(err)
		}

		runtimeWitnessedRegistryStartAndStopHealthyV2(t, config)
		retained, err := os.ReadFile(lock)
		if err != nil || !bytes.Equal(retained, body) {
			t.Fatalf("degraded sibling preflight changed the bound lock: %v", err)
		}
		if attempts := fixture.TotalAttempts(); attempts != 0 {
			t.Fatalf("sibling binding degradation reached shared witness authority: attempts=%d", attempts)
		}
	})
}

func runtimeWitnessedRegistryDirectCompositionV2(
	t *testing.T,
	fixture *authoritycompositionfixture.Service,
	config Config,
) runtimeSharedEvidenceDatasetSnapshotV2 {
	t.Helper()
	privateRoot := filepath.Join(fixture.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	snapshotStores, err := datasetsnapshotstore.OpenStoresV2(
		filepath.Join(privateRoot, "dataset-snapshot-authority"), access,
	)
	if err != nil {
		t.Fatal(err)
	}
	evidenceStores, err := openRuntimeSharedEvidenceStoresV2(fixture.DataDir, access)
	if err != nil {
		t.Fatal(err)
	}
	composition, configured, err := newRuntimeSharedEvidenceDatasetSnapshotV2(
		context.Background(), config, fixture.Authority, snapshotStores, evidenceStores,
		access, filestore.CaseBindingReader{}, nil,
	)
	if err != nil || !configured {
		t.Fatalf("direct shared evidence composition configured=%t err=%v", configured, err)
	}
	return composition
}

func runtimeWitnessedRegistryStartAndStopHealthyV2(t *testing.T, config Config) {
	t.Helper()
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("optional evidence-registry blocker stopped ordinary runtime startup: %v", err)
	}
	server := httptest.NewServer(handler)
	status, health := runtimeSharedEvidenceHTTPJSON(t, server.URL, http.MethodGet, "/health", nil)
	if status != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("optional evidence-registry blocker stopped ordinary health: status=%d body=%#v", status, health)
	}
	status, tools := runtimeSharedEvidenceHTTPJSON(t, server.URL, http.MethodGet, "/v1/runtime/tools?refresh=1", nil)
	toolBody, _ := json.Marshal(tools)
	if status != http.StatusOK || bytes.Contains(toolBody, []byte("analytix_funds")) ||
		bytes.Contains(toolBody, []byte("analyze_account_flows")) || bytes.Contains(toolBody, []byte("count_case_rows")) {
		t.Fatalf("blocked evidence-registry advertised case evidence tools: status=%d body=%s", status, toolBody)
	}
	server.Close()
	shutdownOwnedRuntimeHandler(t, handler)
}

func runtimeWitnessedRegistryAssertCanonicalOpaqueBlockerV2(
	t *testing.T,
	fixture *authoritycompositionfixture.Service,
	config Config,
) {
	t.Helper()
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "provider must not be reached"}})
	}))
	defer provider.Close()
	config.BaseURL = provider.URL + "/v1"

	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("canonical opaque registry blocker stopped ordinary runtime startup: %v", err)
	}
	runtimeServer := httptest.NewServer(handler)
	defer func() {
		runtimeServer.Close()
		shutdownOwnedRuntimeHandler(t, handler)
	}()
	status, health := runtimeSharedEvidenceHTTPJSON(t, runtimeServer.URL, http.MethodGet, "/health", nil)
	if status != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("canonical opaque registry blocker stopped ordinary health: status=%d body=%#v", status, health)
	}
	status, tools := runtimeSharedEvidenceHTTPJSON(t, runtimeServer.URL, http.MethodGet, "/v1/runtime/tools?refresh=1", nil)
	toolBody, _ := json.Marshal(tools)
	if status != http.StatusOK || bytes.Contains(toolBody, []byte("analytix_funds")) ||
		bytes.Contains(toolBody, []byte("analyze_account_flows")) || bytes.Contains(toolBody, []byte("count_case_rows")) {
		t.Fatalf("canonical opaque registry blocker advertised case evidence tools: status=%d body=%s", status, toolBody)
	}

	workspace := t.TempDir()
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	status, thread := runtimeSharedEvidenceHTTPJSON(t, runtimeServer.URL, http.MethodPost, "/v1/threads", map[string]any{
		"title": "canonical opaque registry blocker", "workspace": workspace,
		"providerId": config.ProviderID, "model": config.Model,
	})
	threadID, _ := thread["id"].(string)
	if status != http.StatusCreated || threadID == "" {
		t.Fatalf("create canonical opaque blocker thread status=%d body=%#v", status, thread)
	}
	status, turn := runtimeSharedEvidenceHTTPJSON(
		t, runtimeServer.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		map[string]any{"prompt": packagedSourceUnavailableHydrationPromptV1, "riskIntent": "case", "mode": "agent"},
	)
	turnID, _ := turn["turnId"].(string)
	if status != http.StatusAccepted || turnID == "" {
		t.Fatalf("canonical opaque blocker case turn status=%d body=%#v", status, turn)
	}
	blocked := packagedSourceUnavailableHydrationReadV1(
		t, &http.Client{Timeout: 5 * time.Second}, runtimeServer.URL, threadID, turnID,
	)
	blockedViewBody, _ := json.Marshal(blocked.accepted)
	if blocked.accepted.ClaimCount != 0 || blocked.accepted.ReceiptMetadata.Count != 0 ||
		bytes.Contains(blockedViewBody, []byte(`"factFinalWitnessAdmission":`)) ||
		bytes.Contains(blockedViewBody, []byte(`"registryHead":`)) ||
		bytes.Contains(blockedViewBody, []byte(`"publicationSnapshotProof":`)) ||
		bytes.Contains(blockedViewBody, []byte(`"securityContext":`)) ||
		bytes.Contains(blockedViewBody, []byte(`"publicationIntent":`)) ||
		bytes.Contains(blockedViewBody, []byte(`"storeDigest":`)) {
		t.Fatalf("canonical opaque blocker escaped the fixed zero-fact boundary: %#v", blocked.accepted)
	}
	if providerCalls.Load() != 0 || fixture.TotalAttempts() != 0 {
		for _, namespace := range []string{domainenrollment.SharedEvidenceNamespaceV1, domainenrollment.ThreadRiskNamespaceV1} {
			snapshot, _ := fixture.Snapshot(namespace)
			t.Logf("witness namespace=%s attempts=%d observe=%d advance=%d resolve=%d", namespace, snapshot.AttemptCalls, snapshot.ObserveCalls, snapshot.AdvanceCalls, snapshot.ResolveCalls)
		}
		t.Fatalf("canonical opaque blocker reached provider or witness: provider=%d witness=%d", providerCalls.Load(), fixture.TotalAttempts())
	}
}

func runtimeWitnessedRegistryAssertBlockedCaseAndOrdinaryRestartV2(
	t *testing.T,
	fixture *authoritycompositionfixture.Service,
	config Config,
) {
	t.Helper()
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "isolated provider failure"}})
	}))
	defer provider.Close()
	config.BaseURL = provider.URL + "/v1"
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "synthetic-registry-restart-credential")

	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("blocked case registry stopped ordinary runtime startup: %v", err)
	}
	runtimeServer := httptest.NewServer(handler)
	client := &http.Client{Timeout: 5 * time.Second}
	status, health := runtimeSharedEvidenceHTTPJSON(t, runtimeServer.URL, http.MethodGet, "/health", nil)
	if status != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("blocked case registry stopped ordinary health: status=%d body=%#v", status, health)
	}

	ordinaryWorkspace := t.TempDir()
	status, ordinaryThread := runtimeSharedEvidenceHTTPJSON(t, runtimeServer.URL, http.MethodPost, "/v1/threads", map[string]any{
		"title": "ordinary work with blocked case registry", "workspace": ordinaryWorkspace,
		"providerId": config.ProviderID, "model": config.Model,
	})
	ordinaryThreadID, _ := ordinaryThread["id"].(string)
	if status != http.StatusCreated || ordinaryThreadID == "" {
		t.Fatalf("create ordinary blocked-registry thread status=%d body=%#v", status, ordinaryThread)
	}
	status, ordinaryTurn := runtimeSharedEvidenceHTTPJSON(
		t, runtimeServer.URL, http.MethodPost, "/v1/threads/"+ordinaryThreadID+"/turns",
		map[string]any{"prompt": "只解释当前代码结构"},
	)
	if status != http.StatusInternalServerError || providerCalls.Load() == 0 {
		t.Fatalf("ordinary blocked-registry turn did not reach provider: status=%d calls=%d body=%#v", status, providerCalls.Load(), ordinaryTurn)
	}

	caseWorkspace := t.TempDir()
	runtimeSharedEvidenceWriteCaseBindingV2(t, caseWorkspace)
	status, caseThread := runtimeSharedEvidenceHTTPJSON(t, runtimeServer.URL, http.MethodPost, "/v1/threads", map[string]any{
		"title": "case work with blocked registry", "workspace": caseWorkspace,
		"providerId": config.ProviderID, "model": config.Model,
	})
	caseThreadID, _ := caseThread["id"].(string)
	if status != http.StatusCreated || caseThreadID == "" {
		t.Fatalf("create case blocked-registry thread status=%d body=%#v", status, caseThread)
	}
	providerBeforeCase := providerCalls.Load()
	status, caseTurn := runtimeSharedEvidenceHTTPJSON(
		t, runtimeServer.URL, http.MethodPost, "/v1/threads/"+caseThreadID+"/turns",
		map[string]any{"prompt": packagedSourceUnavailableHydrationPromptV1, "riskIntent": "case", "mode": "agent"},
	)
	caseTurnID, _ := caseTurn["turnId"].(string)
	if status != http.StatusAccepted || caseTurnID == "" {
		t.Fatalf("blocked-registry case turn status=%d body=%#v", status, caseTurn)
	}
	firstCaseFinal := packagedSourceUnavailableHydrationReadV1(t, client, runtimeServer.URL, caseThreadID, caseTurnID)
	if providerCalls.Load() != providerBeforeCase || fixture.TotalAttempts() != 0 {
		t.Fatalf("blocked-registry case turn reached provider or witness: provider_before=%d provider_after=%d witness=%d", providerBeforeCase, providerCalls.Load(), fixture.TotalAttempts())
	}
	runtimeServer.Close()
	shutdownOwnedRuntimeHandler(t, handler)
	ordinaryTurnID, ordinaryBefore := runtimeWitnessedRegistryLatestFrozenContextV2(
		t, config.ProductionDurableRoot, ordinaryThreadID,
	)
	if !domainsecurity.TurnSecurityContextIsGeneral(ordinaryBefore) {
		t.Fatalf("ordinary blocked-registry turn acquired case authority: %#v", ordinaryBefore)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("blocked case registry stopped ordinary persisted restart: %v", err)
	}
	restartedServer := httptest.NewServer(restarted)
	status, health = runtimeSharedEvidenceHTTPJSON(t, restartedServer.URL, http.MethodGet, "/health", nil)
	if status != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("blocked-registry restart health status=%d body=%#v", status, health)
	}
	status, ordinaryDetail := runtimeSharedEvidenceHTTPJSON(t, restartedServer.URL, http.MethodGet, "/v1/threads/"+ordinaryThreadID, nil)
	if status != http.StatusOK || ordinaryDetail["id"] != ordinaryThreadID {
		t.Fatalf("ordinary blocked-registry thread did not survive restart: status=%d body=%#v", status, ordinaryDetail)
	}
	secondCaseFinal := packagedSourceUnavailableHydrationReadV1(
		t, &http.Client{Timeout: 5 * time.Second}, restartedServer.URL, caseThreadID, caseTurnID,
	)
	if firstCaseFinal.accepted.AcceptedFinalDigest != secondCaseFinal.accepted.AcceptedFinalDigest ||
		firstCaseFinal.latestSeq != secondCaseFinal.latestSeq ||
		!reflect.DeepEqual(firstCaseFinal.delivery, secondCaseFinal.delivery) ||
		providerCalls.Load() != providerBeforeCase || fixture.TotalAttempts() != 0 {
		t.Fatalf("blocked-registry restart changed the zero-fact case hydration or used unavailable authority: first=%#v second=%#v provider=%d witness=%d", firstCaseFinal, secondCaseFinal, providerCalls.Load(), fixture.TotalAttempts())
	}
	restartedServer.Close()
	shutdownOwnedRuntimeHandler(t, restarted)
	ordinaryAfter := runtimeWitnessedRegistryFrozenContextV2(
		t, config.ProductionDurableRoot, ordinaryThreadID, ordinaryTurnID,
	)
	if !domainsecurity.TurnSecurityContextIsGeneral(ordinaryAfter) || ordinaryAfter.ContextDigest != ordinaryBefore.ContextDigest {
		t.Fatalf("blocked-registry restart changed ordinary authority: before=%#v after=%#v", ordinaryBefore, ordinaryAfter)
	}
}

func runtimeWitnessedRegistryFrozenContextV2(
	t *testing.T,
	durableRoot string,
	threadID string,
	turnID string,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	store, err := server.NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.GetThread(strings.TrimSpace(threadID))
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := appturn.FrozenSecurityContextForTurn(thread, strings.TrimSpace(turnID))
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func runtimeWitnessedRegistryLatestFrozenContextV2(
	t *testing.T,
	durableRoot string,
	threadID string,
) (string, domainsecurity.TurnSecurityContext) {
	t.Helper()
	store, err := server.NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.GetThread(strings.TrimSpace(threadID))
	if err != nil {
		t.Fatal(err)
	}
	turns, _ := thread["turns"].([]any)
	if len(turns) == 0 {
		t.Fatalf("durable thread has no turn: %#v", thread)
	}
	latest, _ := turns[len(turns)-1].(map[string]any)
	turnID, _ := latest["id"].(string)
	if turnID == "" {
		t.Fatalf("durable turn has no id: %#v", latest)
	}
	return turnID, runtimeWitnessedRegistryFrozenContextV2(t, durableRoot, threadID, turnID)
}

func runtimeWitnessedRegistryConfigV2(t *testing.T) (*authoritycompositionfixture.Service, Config) {
	t.Helper()
	fixture, err := authoritycompositionfixture.New(runtimeWitnessedRegistryHostTempV2(t, "authority"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	if blocker := authoritycompositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
	}
	return fixture, Config{
		RuntimeToken:          DefaultRuntimeToken,
		ProductionDurableRoot: t.TempDir(), DataDir: fixture.DataDir,
		ProviderID: "witnessed-registry-restart", BaseURL: "https://provider.invalid", APIKey: "test-only",
		Model: "witnessed-registry-restart-model", EndpointFormat: "chat_completions",
		AuthorityAnchorV1: fixture.AnchorEnvelope, AuthorityManifestRoot: fixture.ManifestRoot,
		AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot,
		AuthorityCredentialBundleRoot:  fixture.CredentialBundleRoot,
	}
}

func runtimeWitnessedRegistryHostTempV2(t *testing.T, label string) string {
	t.Helper()
	root := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_PROFILE_ROOT"))
	if root == "" {
		return t.TempDir()
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		t.Fatalf("isolated witnessed-registry test profile root is invalid: %v", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("isolated witnessed-registry test profile root is unsafe: info=%#v err=%v", info, err)
	}
	created, err := os.MkdirTemp(absolute, "slice22-"+strings.TrimSpace(label)+"-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(created, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(created); err != nil {
			t.Errorf("cleanup isolated witnessed-registry test profile: %v", err)
		}
	})
	return created
}

func runtimeWitnessedRegistryEmptyRootIdentityV2(t *testing.T, root string) map[string]os.FileInfo {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "capsules" || entries[1].Name() != "indexes" {
		t.Fatalf("witnessed registry does not exclusively own the empty V2 leaf pair: entries=%v", entries)
	}
	result := make(map[string]os.FileInfo, len(entries)+1)
	result["root"], err = os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		result[entry.Name()], err = os.Stat(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func runtimeWitnessedRegistryAssertAbsentV2(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cold evidence-registry owner was materialized: %v", err)
	}
}

func runtimeSharedEvidenceWriteCaseBindingV2(t *testing.T, workspace string) {
	t.Helper()
	metadata := filepath.Join(workspace, ".analytix")
	if err := os.MkdirAll(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_shared_evidence_http_001",
		"source": "analytix-data-analysis", "updatedAt": "2026-07-30T08:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "case-project.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runtimeSharedEvidenceBoundMaterialsV2(
	t *testing.T,
	workspace string,
	observation domainsecurity.CaseBindingObservationV1,
	installationID string,
	authority *finalauthority.FileAuthority,
) (
	datasetsnapshotport.ExactMaterialReferenceV2,
	datasetsnapshotport.ExactMaterialReferenceV2,
	map[datasetsnapshotport.MaterialKindV2]map[string][]byte,
) {
	t.Helper()
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath: workspace, CaseID: observation.CaseID,
			CaseBindingHash:          observation.CaseBindingHash,
			BindingObservationDigest: observation.ObservationDigest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := fixturev2.NewAcceptedSlotRetainedAdmissionV1(fixturev2.AcceptedSlotRetainedAdmissionInputV1{
		Binding: binding, OriginalExact: "synthetic-account-001", Canonical: "synthetic-account-001",
		Field: domainevidence.AcceptedSlotSourceFieldAccountV1, InstallationID: installationID,
		AcceptedAt:     time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
		Sign: func(message []byte) ([]byte, error) {
			return authority.Sign(context.Background(), message)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	referenceFor := func(kind datasetsnapshotport.MaterialKindV2) datasetsnapshotport.ExactMaterialReferenceV2 {
		values := admission.Materials[kind]
		if len(values) != 1 {
			t.Fatalf("bound dataset material %s is ambiguous: %d", kind, len(values))
		}
		for address, body := range values {
			return datasetsnapshotport.ExactMaterialReferenceV2{
				Address: address, SHA256: domainsecurity.SHA256Hex(body), ByteLength: uint64(len(body)),
			}
		}
		return datasetsnapshotport.ExactMaterialReferenceV2{}
	}
	return referenceFor(datasetsnapshotport.MaterialSnapshotManifestV2),
		referenceFor(datasetsnapshotport.MaterialFundsProducerContentV1), admission.Materials
}

func runtimeSharedEvidenceHTTPJSON(
	t *testing.T,
	serverURL string,
	method string,
	path string,
	body map[string]any,
) (int, map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, serverURL+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoded := map[string]any{}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, decoded
}
