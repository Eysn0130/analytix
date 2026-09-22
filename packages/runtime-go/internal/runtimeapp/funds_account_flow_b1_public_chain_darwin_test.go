//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	packagedbuildauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	"analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityfixture "analytix.local/runtime-go/internal/formalauthority"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// This test binds current Go composition to an explicitly selected historical
// native generation. It does not execute the retained runtime-server or claim
// that the package contains the current Go source.
func TestFundsAccountFlowB1FixedNativeInputAdmission(t *testing.T) {
	path := b1SelectedNativeInput(t)
	inspection, err := packagedbuildauthorityfs.InspectPackageV2(context.Background(), path, "darwin", "arm64")
	if err != nil {
		t.Fatalf("B1 fixed native package inspection: %v", err)
	}
	if inspection.Authority.Development == nil || inspection.Authority.Controlled != nil ||
		inspection.Publishable || inspection.FactToolsEnabled ||
		inspection.PackageAnchor != "macos_nonpublishable_resource_seal" {
		t.Fatal("B1 fixed native input exceeded development authority")
	}
	owner, err := openRev9DarwinHostCandidateForExecutable(t.TempDir(), path)
	if err != nil {
		t.Fatalf("B1 fixed native owner admission: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("B1 fixed native owner closure: %v", err)
	}
	t.Log("current Go package inspection and normal local-build Owner admission passed; historical native input only")
}

// A byte-identical installation copy can remove removable-volume I/O from the
// diagnostic. This does not replace production signature or Owner admission.
func b1SelectedNativeInput(t *testing.T) string {
	t.Helper()
	original := os.Getenv("ANALYTIX_AB_R3_PACKAGE_RUNTIME_SERVER")
	if original != rev9RetainedRuntimeServer {
		t.Fatal("B1 fixed native input is unavailable")
	}
	selected := os.Getenv("ANALYTIX_FUNDS_RELOCATED_NATIVE_RUNTIME_SERVER")
	if selected == "" {
		return original
	}
	const suffix = "/Contents/Resources/runtime-go/bin/runtime-server"
	if !filepath.IsAbs(selected) || filepath.Clean(selected) != selected || !strings.HasSuffix(selected, ".app"+suffix) {
		t.Fatal("B1 relocated input must be an absolute canonical app runtime path")
	}
	resolved, err := filepath.EvalSymlinks(selected)
	if err != nil || resolved != selected {
		t.Fatal("B1 relocated input must not traverse symlinks")
	}
	originalRoot, selectedRoot := strings.TrimSuffix(original, suffix), strings.TrimSuffix(selected, suffix)
	anchors := []string{
		"Contents/MacOS/analytix", "Contents/Info.plist", "Contents/_CodeSignature/CodeResources",
		"Contents/Resources/app.asar", "Contents/Resources/runtime-go/bin/runtime-server",
		"Contents/Resources/runtime/analytix-packaged-build-authority.json",
		"Contents/Resources/runtime/analytix-native-development-build.json",
		"Contents/Resources/runtime/analytix-import-accelerator",
		"Contents/Resources/runtime/analytix-cleaning-ops",
		"Contents/Resources/runtime/analytix-analysis-compute",
		"Contents/Resources/runtime/analytix-data-engine",
	}
	for _, anchor := range anchors {
		if b1NativeInputDigest(t, filepath.Join(originalRoot, anchor)) != b1NativeInputDigest(t, filepath.Join(selectedRoot, anchor)) {
			t.Fatalf("B1 relocated historical input changed: %s", anchor)
		}
	}
	t.Log("B1 historical native input relocation: 11 anchors byte-identical; normal signature admission still required")
	return selected
}

func b1NativeInputDigest(t *testing.T, path string) [32]byte {
	t.Helper()
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		t.Fatal("B1 native input anchor is unavailable or not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	after, err := f.Stat()
	current, currentErr := os.Lstat(path)
	if err != nil || currentErr != nil || !os.SameFile(before, after) || !os.SameFile(before, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("B1 native input anchor changed during comparison")
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

// The model supplies only a tool request and ordinary text. All evidence,
// claims, final authority and display bindings must be produced by the live
// production handler from the real native result.
func TestFundsAccountFlowB1ProductionPublicChain(t *testing.T) {
	model := &b1PublicChainModel{t: t, expectedCredential: "test-only"}
	defer model.diagnostics()
	runB1ProductionPublicChain(t, rev14AccountFlowCSV(), http.HandlerFunc(model.serve), func(config Config, root, workspace, sourcePath, authorityKeyID string, sourceEvents func() []domainplugincapability.FundsSourceReadDecisionEventV1, driver b1PublicChainDriver) {
		b1AssertPublicChain(t, config, root, workspace, sourcePath, authorityKeyID, model, sourceEvents, driver)
	})
}

// Reuse the same production composition for additional immutable synthetic CSV
// vectors; callers supply only source/model responses and public-seam assertions.
func runB1ProductionPublicChain(t *testing.T, source []byte, model http.Handler, verify func(Config, string, string, string, string, func() []domainplugincapability.FundsSourceReadDecisionEventV1, b1PublicChainDriver)) {
	t.Helper()
	trust := nativecomponentregistry.EmbeddedTrust()
	if trust.SigningMode != "ad-hoc" || trust.SigningPolicySHA256 != "7625f1de94282690dc72ce92f7e42a293437c24fb9f4a725ff82bfec157ecaf3" || trust.ReceiptSHA256 != "" || trust.ManifestSHA256 != "" || trust.TargetKey != "" || trust.AppleTeamIdentifier != "" {
		t.Fatal("B1 requires the existing compiled development signing policy; no package or receipt trust override")
	}
	runtimePath := b1SelectedNativeInput(t)
	root := workspacetest.New(t)
	authorityRoot := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_PROFILE_ROOT"))
	if authorityRoot == "" {
		authorityRoot = root
	}
	if blocker := authorityfixture.SecureConfigurationFilesystemBlocker(authorityRoot); blocker != "" {
		t.Fatalf("B1 authority configuration prerequisite: %s", blocker)
	}
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	defer func() {
		if !t.Failed() {
			return
		}
		preparedCount := len(b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared")))
		capsules := b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "evidence-registry", "capsules"))
		t.Logf("B1 failure persistence prepared=%d capsule_objects=%d", preparedCount, len(capsules))
		for _, body := range capsules {
			capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(body)
			if err == nil {
				t.Logf("B1 failure capsule sequence=%d entries=%d", capsule.Registry.Sequence, len(capsule.Registry.Entries))
			}
		}
	}()
	config.UserDataDir = filepath.Join(root, "user-data")
	if err := os.Mkdir(config.UserDataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "synthetic-case")
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	sourcePath := filepath.Join(workspace, "baseline.csv")
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(model)
	defer provider.Close()
	config.BaseURL, config.ProviderID, config.Model = provider.URL+"/v1", "b1-synthetic", "b1-synthetic-model"
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
	host := newBundledFundsHostValidationFixtureV1(t)
	dependencies := host.dependencies
	dependencies.resolveRuntimeRoots = func(string) (string, string, error) {
		return bundledFundsRuntimeRootsV1(host.config.DataDir)
	}
	dependencies.openNativeOwnerForTest = func(dataDir string) (*nativecomponenthost.Owner, error) {
		owner, err := openRev9DarwinHostCandidateForExecutable(dataDir, runtimePath)
		if err != nil || owner == nil {
			t.Fatalf("B1 selected native owner prerequisite: %v", err)
		}
		return owner, nil
	}
	if spec, err := validateBundledFundsHostMaterializationWithDependenciesV1(context.Background(), config, dependencies); err != nil || spec == nil {
		t.Fatalf("B1 synthetic Host installation admission failed: %v", err)
	}
	var sourceEvents func() []domainplugincapability.FundsSourceReadDecisionEventV1
	dependencies.observeHostSourceRead = func(read func() []domainplugincapability.FundsSourceReadDecisionEventV1) { sourceEvents = read }
	async := newRuntimeAsyncTurnGuardV1("b1-public-chain")
	ctx := context.WithValue(context.Background(), bundledFundsHostValidationContextKeyV1{}, dependencies)
	ctx = context.WithValue(ctx, asyncTurnObservationContextKeyV1{}, async.observe)
	ctx = context.WithValue(ctx, factRecoveryObservationKeyV1{}, func(report factRecoveryObservationV1) {
		t.Logf("fact recovery candidates=%d admitted=%d held=%d phase=%s", report.Candidates, report.Admitted, report.Held, report.LastRejectedPhase)
	})
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatalf("B1 production assembly: %v", err)
	}
	owned := &ownedPersistenceLeaseHandler{Handler: handler, lease: lease}
	runtime := httptest.NewServer(owned)
	defer func() {
		runtime.Close()
		shutdownOwnedRuntimeHandler(t, owned)
		async.assertDrained(t)
	}()
	verify(config, root, workspace, sourcePath, fixture.Authority.KeyID(), sourceEvents, b1PublicChainDriver{
		baseURL: func() string { return runtime.URL },
		waitTurn: func(threadID, turnID, phase string) {
			async.expect(threadID, turnID, phase)
			if phase != "ordinary-after-failure" {
				b1WaitAsyncCompletion(t, async, threadID, turnID)
			}
		},
		reopen: func() {
			runtime.Close()
			shutdownOwnedRuntimeHandler(t, owned)
			async.assertDrained(t)
			lease, err = AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			handler, err = newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatalf("B1 durable runtime reopen failed: %v", err)
			}
			owned = &ownedPersistenceLeaseHandler{Handler: handler, lease: lease}
			runtime = httptest.NewServer(owned)
		},
	})
}

// The assertions below are shared with the black-box process test. The driver
// owns transport/lifecycle only; it cannot fabricate product evidence or finals.
type b1PublicChainDriver struct {
	baseURL  func() string
	waitTurn func(threadID, turnID, phase string)
	reopen   func()
}

func b1AssertPublicChain(t *testing.T, config Config, root, workspace, sourcePath, authorityKeyID string, model *b1PublicChainModel, sourceEvents func() []domainplugincapability.FundsSourceReadDecisionEventV1, driver b1PublicChainDriver) {
	t.Helper()
	client := &http.Client{Timeout: 45 * time.Second}
	request := func(method, path string, body map[string]any, expected int) map[string]any {
		t.Helper()
		return b1PublicChainRequest(t, client, driver.baseURL(), method, path, body, expected)
	}
	staged := request(http.MethodPost, "/v1/local-display/funds-import/stage", map[string]any{
		"workspaceRoot": workspace, "sourcePath": sourcePath,
	}, http.StatusOK)
	items, _ := staged["items"].([]any)
	if len(items) != 1 {
		t.Fatal("B1 synthetic import did not select one generation")
	}
	item, _ := items[0].(map[string]any)
	selector := contracts.StringField(item, "selector")
	confirmed := request(http.MethodPost, "/v1/local-display/funds-import/confirm", map[string]any{"selector": selector}, http.StatusOK)
	if confirmed["sourceRowCount"] != float64(1) {
		t.Fatalf("B1 synthetic snapshot row count: %v", confirmed["sourceRowCount"])
	}
	t.Log("B1 actual native import and immutable snapshot admitted")
	thread := request(http.MethodPost, "/v1/threads", map[string]any{
		"title": "B1 synthetic flow", "workspace": workspace, "providerId": config.ProviderID, "model": config.Model,
	}, http.StatusCreated)
	threadID := contracts.StringField(thread, "id")
	started := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
		"prompt": "分析银行账号 " + rev14PrivateAccount + " 在二〇二六年八月二十七日的收支。B1_BASELINE", "mode": "agent", "async": true,
		"approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}, http.StatusAccepted)
	turnID := contracts.StringField(started, "turnId")
	driver.waitTurn(threadID, turnID, "baseline")
	turn := b1WaitTerminal(t, client, driver.baseURL(), threadID, turnID)
	preparedBodies := b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared"))
	var preparedRecords []domainevidence.PreparedEvidenceSettlement
	for _, body := range preparedBodies {
		record, err := domainevidence.ParsePreparedEvidenceSettlement(body)
		if err != nil || record.HostAuthority == nil || record.HostAuthority.SelectionContentDigest == "" || record.AuthorityKeyID != authorityKeyID {
			t.Fatal("B1 actual prepared record lost signed content or installation authority")
		}
		b1AssertPreparedFlow(t, record, "1250", "0", "1")
		preparedRecords = append(preparedRecords, record)
	}
	capsuleBodies := b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "evidence-registry", "capsules"))
	for _, body := range capsuleBodies {
		capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(body)
		if err != nil {
			t.Fatal("B1 actual registry capsule is invalid")
		}
		if len(preparedRecords) != 1 {
			t.Fatal("B1 registry capsule lacks its one actual native preparation")
		}
		if receipt, found, err := domainevidence.ResolvePreparedSettlementIssue(capsule.Registry, preparedRecords[0]); err != nil || !found || receipt.ReceiptID != preparedRecords[0].ReceiptID || capsule.Registry.Sequence != 1 {
			t.Fatal("B1 actual receipt lost its exact native preparation and canonical material")
		}
	}
	t.Logf("B1 actual private persistence prepared=%d registry_capsules=%d", len(preparedBodies), len(capsuleBodies))
	body, _ := json.Marshal(turn)
	b1AssertGenericPrivacy(t, body)
	if turn["status"] != "completed" {
		ordinaryWorkspace := filepath.Join(root, "ordinary-workspace")
		if err := os.Mkdir(ordinaryWorkspace, 0o700); err != nil {
			t.Fatal(err)
		}
		ordinaryThread := request(http.MethodPost, "/v1/threads", map[string]any{"title": "B1 ordinary recovery", "workspace": ordinaryWorkspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
		ordinaryID := contracts.StringField(ordinaryThread, "id")
		ordinaryStart := request(http.MethodPost, "/v1/threads/"+ordinaryID+"/turns", map[string]any{"prompt": "Write one sentence about clear writing. B1_ORDINARY", "mode": "agent", "async": true, "approvalPolicy": "auto", "sandboxMode": "workspace-write"}, http.StatusAccepted)
		ordinaryTurnID := contracts.StringField(ordinaryStart, "turnId")
		driver.waitTurn(ordinaryID, ordinaryTurnID, "ordinary-after-failure")
		ordinary := runtimeOptionalLifecycleWaitTurnV1(t, client, driver.baseURL(), ordinaryID, ordinaryTurnID, "B1 ordinary after capability failure")
		ordinaryBody, _ := json.Marshal(ordinary)
		if !bytes.Contains(ordinaryBody, []byte("B1_ORDINARY_COMPLETE")) {
			t.Fatal("B1 ordinary recovery omitted actual Provider response")
		}
		t.Log("B1 actual ordinary synthetic Provider turn completed after capability failure")
		t.Fatalf("B1 terminal status=%v reason=%v", turn["status"], turn["terminalReason"])
	}
	final, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
	if err != nil || turn["acceptedFinal"] != nil {
		t.Fatal("B1 production final omitted its typed public view or leaked private authority")
	}
	t.Logf("B1 public final variant=%s coverage=%s blocker=%s claims=%d receipts=%d", final.Variant, final.CoverageStatus, final.BlockerCode, final.ClaimCount, final.ReceiptMetadata.Count)
	if final.Variant != domainevidence.EvidenceBackedAnswer {
		model.mu.Lock()
		t.Logf("B1 Provider requests=%d", len(model.requests))
		model.mu.Unlock()
		if sourceEvents != nil {
			for _, event := range sourceEvents() {
				t.Logf("B1 source lifecycle operation=%s state=%s decision=%s reason=%s", event.Operation, event.State, event.Decision, event.ReasonCode)
			}
		}
		diagnostics := request(http.MethodGet, "/v1/runtime/tools", nil, http.StatusOK)
		servers, _ := diagnostics["mcpServers"].([]any)
		for _, raw := range servers {
			server, _ := raw.(map[string]any)
			if server["id"] == "analytix_funds" {
				t.Logf("B1 Host status=%v connected=%v failureCode=%v", server["status"], server["connected"], server["failureCode"])
			}
		}
		t.Fatal("B1 completed turn did not acquire current native evidence")
	}
	digest := final.AcceptedFinalDigest
	model.mu.Lock()
	providerCalls := len(model.requests)
	model.mu.Unlock()
	before := startupWholeTreeDigest(t,
		filepath.Join(config.DataDir, "private", "evidence-registry"),
		filepath.Join(config.DataDir, "private", "evidence-settlements"),
		filepath.Join(config.DataDir, "private", "accepted-finals"))
	full := request(http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{
		"kind": "accepted_slot_display", "threadId": threadID, "turnId": turnID,
		"acceptedFinalDigest": digest, "displayMode": "full",
	}, http.StatusOK)
	slots, _ := full["slots"].([]any)
	if len(slots) != 1 {
		t.Fatalf("B1 actual final has %d local slots", len(slots))
	}
	slot, _ := slots[0].(map[string]any)
	if slot["displayValue"] != b1OriginalSourceAccount {
		t.Fatal("B1 accepted slot did not resolve its original exact source field")
	}
	masked := request(http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{
		"kind": "accepted_slot_display", "threadId": threadID, "turnId": turnID,
		"acceptedFinalDigest": digest, "displayMode": "masked",
	}, http.StatusOK)
	maskedSlots, _ := masked["slots"].([]any)
	if len(maskedSlots) != 1 {
		t.Fatal("B1 masked display changed the accepted slot count")
	}
	maskedSlot, _ := maskedSlots[0].(map[string]any)
	if maskedSlot["displayValue"] != "****"+rev14PrivateAccount[len(rev14PrivateAccount)-4:] {
		t.Fatal("B1 masked display was not produced by the Host")
	}
	for _, field := range []string{"slotId", "field", "claimIds", "receiptIds"} {
		left, _ := json.Marshal(slot[field])
		right, _ := json.Marshal(maskedSlot[field])
		if !bytes.Equal(left, right) {
			t.Fatalf("B1 display mode changed %s", field)
		}
	}
	for _, mode := range []string{"full", "masked"} {
		preview := request(http.MethodPost, "/v1/local-display/direct-source-preview", map[string]any{
			"kind": "direct_source_preview", "workspaceRoot": workspace, "view": "transactions",
			"fields": []string{"account"}, "rowOffset": 0, "rowLimit": 1, "displayMode": mode,
		}, http.StatusOK)
		previewBody, _ := json.Marshal(preview)
		if bytes.Contains(previewBody, []byte(b1OriginalSourceAccount)) != (mode == "full") {
			t.Fatal("B1 Direct Preview did not enforce the final display mode")
		}
	}
	after := startupWholeTreeDigest(t,
		filepath.Join(config.DataDir, "private", "evidence-registry"),
		filepath.Join(config.DataDir, "private", "evidence-settlements"),
		filepath.Join(config.DataDir, "private", "accepted-finals"))
	model.mu.Lock()
	unchangedCalls := len(model.requests) == providerCalls
	model.mu.Unlock()
	if before != after || !unchangedCalls {
		t.Fatal("B1 typed display changed evidence/final bindings or invoked Provider")
	}
	t.Log("B1 same-execution native result -> production settlement -> receipt/claims/Final Gate -> accepted local slot")
	if len(preparedRecords) != 1 || len(capsuleBodies) != 1 {
		t.Fatal("B1 baseline did not issue exactly one preparation and receipt")
	}
	baseline := preparedRecords[0]
	b1AssertActualFinal(t, config.DataDir, baseline, digest, true)
	baselineMaterial, _ := domainevidence.ParseCanonicalEvidenceMaterial(baseline.CanonicalEvidence)
	baselineBinding := baselineMaterial.AcceptedSlotSourceBindings[0]
	originalPage := b1RetainedParsedPage(t, config.DataDir, baselineBinding.SourceRecordID)
	originalPageBytes, err := os.ReadFile(originalPage)
	if err != nil {
		t.Fatal(err)
	}

	// A real second native import replaces the current analytical snapshot.
	// The same case account keeps its alias, but gains a second transaction.
	evolvedSource := filepath.Join(workspace, "evolved.csv")
	if err := os.WriteFile(evolvedSource, b1EvolvedCSV(t), 0600); err != nil {
		t.Fatal(err)
	}
	evolvedStage := request(http.MethodPost, "/v1/local-display/funds-import/stage", map[string]any{"workspaceRoot": workspace, "sourcePath": evolvedSource}, http.StatusOK)
	evolvedItems, _ := evolvedStage["items"].([]any)
	if len(evolvedItems) != 1 {
		t.Fatal("B1 evolved import selector count changed")
	}
	evolvedItem, _ := evolvedItems[0].(map[string]any)
	evolvedImport := request(http.MethodPost, "/v1/local-display/funds-import/confirm", map[string]any{"selector": contracts.StringField(evolvedItem, "selector")}, http.StatusOK)
	if evolvedImport["sourceRowCount"] != float64(2) {
		t.Fatal("B1 evolved native import did not contain two rows")
	}
	evolvedStart := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
		"prompt": "分析银行账号 " + rev14PrivateAccount + " 在二〇二六年八月二十七日的收支。B1_EVOLVED", "mode": "agent", "async": true, "approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}, http.StatusAccepted)
	evolvedTurnID := contracts.StringField(evolvedStart, "turnId")
	driver.waitTurn(threadID, evolvedTurnID, "evolved")
	evolvedTurn := b1WaitTerminal(t, client, driver.baseURL(), threadID, evolvedTurnID)
	evolvedFinal, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(evolvedTurn["acceptedFinalView"])
	if err != nil || evolvedTurn["status"] != "completed" || evolvedFinal.Variant != domainevidence.PartialEvidenceAnswer || evolvedFinal.CoverageStatus != "partial" || evolvedFinal.ClaimCount != 3 || evolvedFinal.ReceiptMetadata.Count != 1 {
		t.Fatal("B1 evolved native analysis did not retain partial evidence coverage in its actual final")
	}
	var evolved domainevidence.PreparedEvidenceSettlement
	for _, body := range b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared")) {
		candidate, err := domainevidence.ParsePreparedEvidenceSettlement(body)
		if err != nil {
			t.Fatal("B1 evolved preparation is invalid")
		}
		if candidate.SecurityContext.TurnID == evolvedTurnID {
			evolved = candidate
		}
	}
	b1AssertPreparedFlow(t, evolved, "1250", "225", "2")
	b1AssertActualFinal(t, config.DataDir, evolved, evolvedFinal.AcceptedFinalDigest, false)
	evolvedMaterial, _ := domainevidence.ParseCanonicalEvidenceMaterial(evolved.CanonicalEvidence)
	evolvedBinding := evolvedMaterial.AcceptedSlotSourceBindings[0]
	if baseline.SecurityContext.DatasetSnapshotID == evolved.SecurityContext.DatasetSnapshotID || baseline.SecurityContext.SourceManifestHash == evolved.SecurityContext.SourceManifestHash ||
		baselineBinding.EntityReference != evolvedBinding.EntityReference || baselineBinding.SourceRecordID == evolvedBinding.SourceRecordID || baselineBinding.SourceFileID == evolvedBinding.SourceFileID {
		t.Fatal("B1 snapshot evolution lost stable entity identity or exact new source lineage")
	}
	t.Log("B1 evolved actual native aggregate 1250/225/count2 with one evidence row; stable alias and distinct immutable lineage")

	readOriginal := func() {
		t.Helper()
		for _, mode := range []string{"full", "masked"} {
			got := request(http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{"kind": "accepted_slot_display", "threadId": threadID, "turnId": turnID, "acceptedFinalDigest": digest, "displayMode": mode}, http.StatusOK)
			want := full
			if mode == "masked" {
				want = masked
			}
			if !reflect.DeepEqual(got["slots"], want["slots"]) {
				t.Fatal("B1 historical source slot changed after snapshot evolution or reopen")
			}
		}
	}
	previewCurrent := func() {
		t.Helper()
		for _, mode := range []string{"full", "masked"} {
			got := request(http.MethodPost, "/v1/local-display/direct-source-preview", map[string]any{"kind": "direct_source_preview", "workspaceRoot": workspace, "view": "transactions", "fields": []string{"account", "amountText"}, "rowOffset": 1, "rowLimit": 1, "displayMode": mode}, http.StatusOK)
			if got["datasetSnapshotId"] != evolved.SecurityContext.DatasetSnapshotID || got["rowOffset"] != float64(1) || got["rowLimit"] != float64(1) || got["hasMore"] != false {
				t.Fatal("B1 current preview selected another snapshot or row window")
			}
			rows, _ := got["rows"].([]any)
			if len(rows) != 1 {
				t.Fatal("B1 current preview row count changed")
			}
			row, _ := rows[0].(map[string]any)
			cells, _ := row["cells"].([]any)
			if row["rowIndex"] != float64(1) || len(cells) != 2 {
				t.Fatal("B1 current preview returned unrelated rows or fields")
			}
			acct, _ := cells[0].(map[string]any)
			amount, _ := cells[1].(map[string]any)
			want := rev14PrivateAccount
			if mode == "masked" {
				want = "****7890"
			}
			if acct["field"] != "account" || acct["displayValue"] != want || amount["field"] != "amountText" || amount["displayValue"] != "2.25" {
				t.Fatal("B1 current preview did not return exact selected source cells")
			}
		}
	}
	model.mu.Lock()
	providerCalls = len(model.requests)
	model.mu.Unlock()
	before = startupWholeTreeDigest(t, filepath.Join(config.DataDir, "private", "evidence-registry"), filepath.Join(config.DataDir, "private", "evidence-settlements"), filepath.Join(config.DataDir, "private", "accepted-finals"))
	readOriginal()
	previewCurrent()
	driver.reopen()
	readOriginal()
	previewCurrent()
	t.Log("B1 real durable reopen preserved original slots and current exact preview")
	for _, fault := range []string{"missing", "corrupt"} {
		func() {
			if fault == "missing" {
				if err := os.Rename(originalPage, originalPage+".b1-held"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Rename(originalPage+".b1-held", originalPage); err != nil {
						t.Error(err)
					}
				}()
			} else {
				if err := os.WriteFile(originalPage, []byte("invalid synthetic retained parsed page"), 0600); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.WriteFile(originalPage, originalPageBytes, 0600); err != nil {
						t.Error(err)
					}
				}()
			}
			failed := request(http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{"kind": "accepted_slot_display", "threadId": threadID, "turnId": turnID, "acceptedFinalDigest": digest, "displayMode": "full"}, http.StatusConflict)
			failedBytes, _ := json.Marshal(failed)
			b1AssertGenericPrivacy(t, failedBytes)
			if failed["slots"] != nil {
				t.Fatal("B1 unavailable retained material yielded a display slot")
			}
			// The reopened material CAS validates its complete original inventory.
			// Corrupting that shared authority also blocks current local preview.
			for _, mode := range []string{"full", "masked"} {
				unavailable := request(http.MethodPost, "/v1/local-display/direct-source-preview", map[string]any{
					"kind": "direct_source_preview", "workspaceRoot": workspace, "view": "transactions",
					"fields": []string{"account", "amountText"}, "rowOffset": 1, "rowLimit": 1, "displayMode": mode,
				}, http.StatusConflict)
				body, _ := json.Marshal(unavailable)
				b1AssertGenericPrivacy(t, body)
				if unavailable["rows"] != nil {
					t.Fatal("B1 invalid shared material authority returned current preview rows")
				}
			}
		}()
		readOriginal()
		previewCurrent()
		t.Logf("B1 retained %s failed closed and exact-byte restoration recovered both local display paths", fault)
	}
	after = startupWholeTreeDigest(t, filepath.Join(config.DataDir, "private", "evidence-registry"), filepath.Join(config.DataDir, "private", "evidence-settlements"), filepath.Join(config.DataDir, "private", "accepted-finals"))
	model.mu.Lock()
	unchangedCalls = len(model.requests) == providerCalls
	model.mu.Unlock()
	if before != after || !unchangedCalls {
		t.Fatal("B1 readback/reopen/fault path mutated receipt/final authority or invoked Provider")
	}
	// The ordinary turn runs while the original retained source is still
	// unavailable, proving independence from recovery of that capability.
	func() {
		if err := os.WriteFile(originalPage, []byte("invalid synthetic retained parsed page"), 0600); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.WriteFile(originalPage, originalPageBytes, 0600); err != nil {
				t.Error(err)
			}
		}()
		failed := request(http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{"kind": "accepted_slot_display", "threadId": threadID, "turnId": turnID, "acceptedFinalDigest": digest, "displayMode": "full"}, http.StatusConflict)
		if failed["slots"] != nil {
			t.Fatal("B1 retained fault yielded a slot before ordinary work")
		}
		ordinaryWorkspace := filepath.Join(root, "ordinary-after-retained-failure")
		if err := os.Mkdir(ordinaryWorkspace, 0700); err != nil {
			t.Fatal(err)
		}
		ordinaryThread := request(http.MethodPost, "/v1/threads", map[string]any{"title": "B1 ordinary recovery", "workspace": ordinaryWorkspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
		ordinaryID := contracts.StringField(ordinaryThread, "id")
		ordinaryStart := request(http.MethodPost, "/v1/threads/"+ordinaryID+"/turns", map[string]any{"prompt": "Write one sentence about clear writing. B1_ORDINARY", "mode": "agent", "async": true, "approvalPolicy": "auto", "sandboxMode": "workspace-write"}, http.StatusAccepted)
		ordinaryTurnID := contracts.StringField(ordinaryStart, "turnId")
		driver.waitTurn(ordinaryID, ordinaryTurnID, "ordinary-after-retained-failure")
		ordinaryTurn := b1WaitTerminal(t, client, driver.baseURL(), ordinaryID, ordinaryTurnID)
		ordinaryBytes, _ := json.Marshal(ordinaryTurn)
		if ordinaryTurn["status"] != "completed" || !bytes.Contains(ordinaryBytes, []byte("B1_ORDINARY_COMPLETE")) {
			t.Fatal("B1 ordinary Provider turn did not complete after retained-source failure")
		}
	}()
	readOriginal()
	for _, name := range []string{"thread.json", "events.jsonl", "messages.jsonl"} {
		body, err := os.ReadFile(filepath.Join(config.ProductionDurableRoot, "threads", threadID, name))
		if os.IsNotExist(err) && name == "messages.jsonl" {
			continue
		}
		if err != nil {
			t.Fatal("B1 durable forbidden-channel inventory unavailable")
		}
		b1AssertGenericPrivacy(t, body)
	}
	t.Log("B1 actual durable reopen, current preview, retained missing/corrupt fail-closed, and ordinary Provider continuity passed")
}

// This test uses an unformatted account admitted by canonical CSV. Exact retained
// reads are distinguished from value fallback by later retained-material faults.
const b1OriginalSourceAccount = rev14PrivateAccount

type b1PublicChainModel struct {
	expectedCredential string
	t                  *testing.T
	mu                 sync.Mutex
	requests           [][]byte
}

func b1WaitAsyncCompletion(t *testing.T, guard *runtimeAsyncTurnGuardV1, threadID, turnID string) {
	t.Helper()
	// Wait on the in-process completion observation before the public HTTP read.
	// Hydrating historical finals on every poll would add concurrent witness work
	// to the settlement being measured. Keep the original 90-second test budget.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		guard.mu.Lock()
		record := guard.records[threadID+"\x00"+turnID]
		done := record != nil && record.ends == 1
		guard.mu.Unlock()
		if done {
			guard.assertDrained(t)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("B1 asynchronous terminal finalization did not drain")
}

func (model *b1PublicChainModel) serve(w http.ResponseWriter, r *http.Request) {
	if model.expectedCredential == "" || r.Header.Get("Authorization") != "Bearer "+model.expectedCredential {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil || r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		model.t.Error("B1 unexpected synthetic Provider request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	b1AssertGenericPrivacy(model.t, body)
	model.mu.Lock()
	model.requests = append(model.requests, append([]byte(nil), body...))
	model.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	if bytes.Contains(body, []byte("B1_ORDINARY")) {
		runtimeOptionalLifecycleModelResponseV1(w, "", nil, "B1_ORDINARY_COMPLETE: Clear writing makes one point at a time.")
		return
	}
	if b1HasCurrentSemantic(model.t, body) {
		runtimeOptionalLifecycleModelResponseV1(w, "", nil, "B1 synthetic analysis complete")
		return
	}
	runtimeOptionalLifecycleModelResponseV1(w, rev14AccountFlowToolName, map[string]any{
		"subject_alias": "acct:1", "start_inclusive": "2026-08-27T00:00:00.000000Z",
		"end_inclusive": "2026-08-27T23:59:59.999999Z", "evidence_row_limit": 1,
	}, "")
}

func (model *b1PublicChainModel) diagnostics() {
	model.mu.Lock()
	defer model.mu.Unlock()
	semantics, toolMessages := 0, 0
	for _, body := range model.requests {
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &request)
		for _, message := range request.Messages {
			if message.Role == "tool" {
				toolMessages++
			}
		}
		if bytes.Contains(body, []byte("analytix.account-flow-provider-semantics/v3")) {
			semantics++
		}
	}
	model.t.Logf("B1 Provider diagnostics requests=%d tool_messages=%d native_semantic_messages=%d", len(model.requests), toolMessages, semantics)
}

func b1WaitTerminal(t *testing.T, client *http.Client, base, threadID, turnID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		status, detail := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, base, http.MethodGet, "/v1/threads/"+threadID, nil)
		if status != http.StatusOK && status != http.StatusServiceUnavailable {
			t.Fatalf("B1 terminal observation status=%d", status)
		}
		turn := packagedSourceUnavailableHydrationTurnV1(detail, turnID)
		if turn != nil {
			switch contracts.StringField(turn, "status") {
			case "completed", "failed", "cancelled", "interrupted":
				return turn
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("B1 terminal observation timed out")
	return nil
}

func b1PublicChainRequest(t *testing.T, client *http.Client, base, method, path string, body map[string]any, expected int) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal("B1 request JSON encoding failed")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatal("B1 request construction failed")
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(path, "/v1/local-display/") {
		request.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("B1 request transport failed")
	}
	defer response.Body.Close()
	decoded := map[string]any{}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&decoded); err != nil {
		t.Fatal("B1 response JSON decoding failed")
	}
	if response.StatusCode != expected {
		t.Fatal(b1PublicChainRequestFailure(method, path, response.StatusCode, expected, decoded))
	}
	if !strings.HasPrefix(path, "/v1/local-display/") {
		encoded, _ := json.Marshal(decoded)
		b1AssertGenericPrivacy(t, encoded)
	}
	return decoded
}

func b1PublicChainRequestFailure(method, path string, status, expected int, decoded map[string]any) string {
	// Only known method tokens and the versioned Registry error-code enum are
	// diagnostics. Never format response messages, arbitrary codes or bodies.
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		method = "UNKNOWN"
	}
	phase := "OTHER"
	switch path {
	case "/v1/local-display/funds-import/stage":
		phase = "IMPORT_STAGE"
	case "/v1/local-display/funds-import/confirm":
		phase = "IMPORT_CONFIRM"
	}
	code := "unavailable"
	// Keep finite thread-detail failures distinguishable without echoing bodies,
	// messages, arbitrary codes or thread identities into diagnostics.
	switch candidate := contracts.StringField(decoded, "code"); candidate {
	case "public_projection_pending", "accepted_final_hydration_unavailable":
		code = candidate
	}
	if decoded["schemaVersion"] == float64(1) {
		nested, _ := decoded["error"].(map[string]any)
		switch candidate := contracts.StringField(nested, "code"); candidate {
		case "invalid_request", "unauthorized", "method_not_allowed", "not_found", "conflict",
			"persistence_failure", "verification_failure", "request_too_large":
			code = candidate
		}
	}
	return fmt.Sprintf("B1 request %s: phase=%s status=%d want=%d code=%s", method, phase, status, expected, code)
}

func b1AssertGenericPrivacy(t *testing.T, body []byte) {
	t.Helper()
	for _, private := range []string{rev14PrivateAccount, b1OriginalSourceAccount, rev14PrivateCard, rev14PrivateCounterparty, rev14PrivateSourceSentinel, "cer1_", ".duckdb"} {
		if bytes.Contains(body, []byte(private)) {
			t.Fatal("B1 private value reached a forbidden generic projection")
		}
	}
}

func b1PrivateCASRecords(t *testing.T, root string) [][]byte {
	t.Helper()
	var bodies [][]byte
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		if len(entry.Name()) != 69 {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		bodies = append(bodies, body)
		return nil
	})
	if err != nil {
		t.Fatal("B1 private synthetic inventory is unavailable")
	}
	return bodies
}

func b1AssertPreparedFlow(t *testing.T, record domainevidence.PreparedEvidenceSettlement, inflow, outflow, count string) {
	t.Helper()
	material, err := domainevidence.ParseCanonicalEvidenceMaterial(record.CanonicalEvidence)
	if err != nil || len(material.Facts) != 3 || len(material.AcceptedSlotSourceBindings) != 1 {
		t.Fatal("B1 actual native canonical fact/lineage cardinality is invalid")
	}
	values := map[string]string{}
	for _, fact := range material.Facts {
		if fact.ClaimType == domainevidence.ClaimAmount {
			if fact.NormalizedPayload.Currency != "CNY" {
				t.Fatal("B1 native aggregate changed currency")
			}
			values[fact.NormalizedPayload.Direction] = fact.NormalizedPayload.AmountMinor
		} else if fact.ClaimType == domainevidence.ClaimCount {
			values["count"] = fact.NormalizedPayload.Count
		}
	}
	if values["in"] != inflow || values["out"] != outflow || values["count"] != count {
		t.Fatal("B1 native exact minor units or transaction count changed")
	}
	binding := material.AcceptedSlotSourceBindings[0]
	if binding.Field != domainevidence.AcceptedSlotSourceFieldAccountV1 || binding.SourceRowNumber != 1 || len(binding.FactIDs) != 3 || len(record.ReceiptDraft.SourceRecordIDs) != 1 || record.ReceiptDraft.SourceRecordIDs[0] != binding.SourceRecordID {
		t.Fatal("B1 native account aggregate lost exact original row/field lineage")
	}
}

func b1EvolvedCSV(t *testing.T) []byte {
	t.Helper()
	rows, err := csv.NewReader(bytes.NewReader(rev14AccountFlowCSV())).ReadAll()
	if err != nil || len(rows) != 2 {
		t.Fatal("B1 baseline CSV fixture is invalid")
	}
	second := append([]string(nil), rows[1]...)
	second[4], second[5], second[6], second[7] = "2026-08-27 11:00:00", "2.25", "997.75", "出"
	rows = append(rows, second)
	var body bytes.Buffer
	writer := csv.NewWriter(&body)
	writer.UseCRLF = true
	if err := writer.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func b1HasCurrentSemantic(t *testing.T, body []byte) bool {
	t.Helper()
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Error("B1 Provider JSON is invalid")
		return false
	}
	start := 0
	evolved := false
	for index, message := range request.Messages {
		content, _ := json.Marshal(message.Content)
		if message.Role == "user" && bytes.Contains(content, []byte("B1_")) {
			start = index
			evolved = bytes.Contains(content, []byte("B1_EVOLVED"))
		}
	}
	var found []domainnative.AccountFlowProviderSemanticResultV1
	var inspect func(any)
	inspect = func(value any) {
		switch value := value.(type) {
		case string:
			var decoded any
			if json.Unmarshal([]byte(value), &decoded) == nil {
				inspect(decoded)
			}
		case []any:
			for _, child := range value {
				inspect(child)
			}
		case map[string]any:
			if value["purpose"] == domainnative.AccountFlowProviderModelPurposeV1 {
				canonical, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(value)
				if err != nil {
					t.Error("B1 current native Provider envelope is invalid")
					return
				}
				var output domainnative.AccountFlowProviderModelOutputV1
				if json.Unmarshal(canonical, &output) != nil {
					t.Error("B1 current native semantic parse failed")
					return
				}
				found = append(found, output.Data)
				return
			}
			for _, child := range value {
				inspect(child)
			}
		}
	}
	for _, message := range request.Messages[start:] {
		inspect(message.Content)
	}
	if len(found) == 0 {
		return false
	}
	if len(found) != 1 {
		t.Error("B1 current turn has duplicate native semantic envelopes")
		return true
	}
	data := found[0]
	count := uint64(1)
	outflow := "0"
	if evolved {
		count = 2
		outflow = "225"
	}
	if data.SubjectAlias != "acct:1" || data.InflowMinor != "1250" || data.OutflowMinor != outflow || data.TransactionCount != count ||
		!data.AggregateComplete || data.EvidenceRowsComplete == evolved || data.EvidenceTransactionCount != 1 || data.EvidenceRowLimit != 1 || len(data.Transactions) != 1 {
		t.Error("B1 actual native semantics lost stable alias, exact aggregate, or independent row coverage")
	}
	if evolved && (data.Outcome.AggregateCompleteness != domainevidence.AccountFlowOutcomeCompletenessCompleteV1 ||
		data.Outcome.EvidenceRowsCompleteness != domainevidence.AccountFlowOutcomeCompletenessIncompleteV1 || data.Outcome.FactAnswerAllowed) {
		t.Error("B1 bounded native evidence rows were upgraded to complete fact-answer eligibility")
	}
	return true
}

func b1AssertActualFinal(t *testing.T, dataDir string, prepared domainevidence.PreparedEvidenceSettlement, digest string, rowsComplete bool) {
	t.Helper()
	var matches []domainevidence.PrivateAcceptedFinalRecord
	for _, body := range b1PrivateCASRecords(t, filepath.Join(dataDir, "private", "accepted-finals", "records")) {
		record, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
		if err != nil {
			t.Fatal("B1 actual private final is invalid")
		}
		if record.SecurityContext.TurnID == prepared.SecurityContext.TurnID {
			matches = append(matches, record)
		}
	}
	if len(matches) != 1 {
		t.Fatal("B1 actual preparation did not yield exactly one private final")
	}
	record := matches[0]
	outcome := record.Envelope.AccountFlowOutcome
	if record.AcceptedFinal.RecordDigest != digest || record.SecurityContext != prepared.SecurityContext ||
		len(record.Envelope.Claims) != 3 || len(record.Envelope.EvidenceReceiptIDs) != 1 || record.Envelope.EvidenceReceiptIDs[0] != prepared.ReceiptID {
		t.Fatal("B1 final authority does not bind its actual native preparation")
	}
	if rowsComplete {
		if record.Envelope.Variant != domainevidence.EvidenceBackedAnswer || outcome == nil || len(outcome.Groups) != 1 ||
			!outcome.FactAnswerAllowed || outcome.AggregateCompleteness != "complete" || outcome.EvidenceRowsCompleteness != "complete" || outcome.LocalDisplayCompletion != "not_requested" {
			t.Fatal("B1 complete final lost its native account-flow eligibility")
		}
		group := outcome.Groups[0]
		if group.QueryHash != prepared.QueryHash || len(prepared.ReceiptDraft.TransformationLineage) != 2 ||
			group.ResultHash != prepared.ReceiptDraft.TransformationLineage[0].OutputHash || group.EvidenceReceiptID != prepared.ReceiptID || len(group.ClaimIDs) != 3 || group.SourceFieldReference.Field != "account" ||
			domainevidence.ValidateAccountFlowTypedSourceFieldReferenceV1(group.SourceFieldReference, group.QueryHash, group.ResultHash) != nil {
			t.Fatal("B1 Final Gate changed native query/result/source lineage")
		}
	} else if record.Envelope.Variant != domainevidence.PartialEvidenceAnswer || outcome != nil || len(record.Envelope.MissingScope) == 0 ||
		prepared.ReceiptDraft.PaginationCompleteness != domainevidence.PaginationPartial {
		t.Fatal("B1 partial final lost its explicit missing scope or gained complete fact-answer eligibility")
	}
	material, err := domainevidence.ParseCanonicalEvidenceMaterial(prepared.CanonicalEvidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, claim := range record.Envelope.Claims {
		if !rowsComplete && claim.SupportState != domainevidence.ClaimPartial {
			t.Fatal("B1 partial native receipt was upgraded to a verified aggregate claim")
		}
		matched := 0
		for _, fact := range material.Facts {
			if claim.ClaimType == fact.ClaimType && reflect.DeepEqual(claim.NormalizedPayload, fact.NormalizedPayload) {
				matched++
			}
		}
		if matched != 1 || len(claim.EvidenceIDs) != 1 || claim.EvidenceIDs[0] != prepared.ReceiptID {
			t.Fatal("B1 actual claim did not match one exact canonical native fact")
		}
	}
}

func b1RetainedParsedPage(t *testing.T, dataDir, sourceRecordID string) string {
	t.Helper()
	root := filepath.Join(dataDir, "private", "dataset-snapshot-authority", "materials")
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		defer clear(body)
		page, err := domainevidence.ParseParsedPageV1(body)
		if err != nil {
			return nil
		}
		for _, outcome := range page.Outcomes {
			if outcome.SourceRecordID == sourceRecordID {
				paths = append(paths, path)
			}
		}
		return nil
	}); err != nil || len(paths) != 1 {
		t.Fatal("B1 exact original parsed-page inventory is unavailable or ambiguous")
	}
	body, err := os.ReadFile(paths[0])
	if err != nil || filepath.Base(paths[0]) != domainsecurity.SHA256Hex(body)+".json" {
		t.Fatal("B1 original parsed page does not match its content address")
	}
	return paths[0]
}
