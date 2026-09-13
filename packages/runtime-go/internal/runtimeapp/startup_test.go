package runtimeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	attachmentauthority "analytix.local/runtime-go/internal/adapters/outbound/attachmentauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	piiauthorizationstore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	"analytix.local/runtime-go/internal/protocol"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRuntimePrivateCASOwnersRecoverBeforeBaseline(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir()}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("initialize runtime CAS owners: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)

	fixtures := []struct {
		name       string
		components []string
	}{
		{name: "accepted-final-records", components: []string{"accepted-finals", "records"}},
		{name: "accepted-final-dispositions", components: []string{"accepted-finals", "dispositions"}},
		{name: "case-thread-authority", components: []string{"case-thread-authority"}},
		{name: "continuation-receipts", components: []string{"gate-continuations", "receipts-v2"}},
		{name: "continuation-dispositions", components: []string{"gate-continuations", "dispositions-v2"}},
		{name: "pending-work-receipts", components: []string{"pending-work", "receipts"}},
		{name: "pending-work-dispositions", components: []string{"pending-work", "dispositions"}},
		{name: "provider-cache-attempts", components: []string{"provider-cache-telemetry", "attempts"}},
		{name: "provider-cache-settlements", components: []string{"provider-cache-telemetry", "settlements"}},
		{name: "provider-cache-turn-closures", components: []string{"provider-cache-telemetry", "turn-closures"}},
		{name: "turn-terminal-intents", components: []string{"turn-terminal-authority", "intents"}},
		{name: "turn-terminal-dispositions", components: []string{"turn-terminal-authority", "dispositions"}},
		{name: "attachment-owners", components: []string{"attachment-authority", "owners"}},
		{name: "attachment-use-receipts", components: []string{"attachment-authority", "use-receipts"}},
		{name: "attachment-use-dispositions", components: []string{"attachment-authority", "use-dispositions"}},
		{name: "attachment-upload-intents", components: []string{"attachment-authority", "upload-intents"}},
		{name: "attachment-upload-dispositions", components: []string{"attachment-authority", "upload-dispositions"}},
		{name: "thread-risk-policy", components: []string{"thread-risk-policy"}},
		{name: "pii-authorization-grants", components: []string{"pii-authorization", "grants"}},
		{name: "report-publication-attempts", components: []string{"report-publication", "attempts"}},
		{name: "report-publication-receipts", components: []string{"report-publication", "receipts"}},
		{name: "report-publication-commit-receipts", components: []string{"report-publication", "commit-receipts"}},
		{name: "report-publication-commit-selections", components: []string{"report-publication", "commit-selections"}},
		{name: "report-publication-delivery-decisions", components: []string{"report-publication", "delivery-decisions"}},
		{name: "report-publication-grant-settlements", components: []string{"report-publication", "grant-settlements"}},
		{name: "report-publication-stage-completions", components: []string{"report-publication", "stage-completions"}},
		{name: "report-publication-delivery-projections", components: []string{"report-publication", "delivery-projections"}},
		{name: "report-publication-indexes", components: []string{"report-publication", "indexes"}},
		{name: "report-publication-claim-ledgers", components: []string{"report-publication", "claim-ledgers"}},
		{name: "report-publication-pii-projections", components: []string{"report-publication", "pii-projections"}},
		{name: "report-publication-render-inspections", components: []string{"report-publication", "render-inspections"}},
		{name: "report-publication-artifacts", components: []string{"report-publication", "artifacts"}},
		{name: "controlled-artifact-access-receipts", components: []string{"controlled-artifact-access", "access-receipts"}},
		{name: "controlled-artifact-access-dispositions", components: []string{"controlled-artifact-access", "access-dispositions"}},
		{name: "controlled-artifact-access-v2-receipts", components: []string{"controlled-artifact-access-v2", "access-receipts"}},
		{name: "controlled-artifact-access-v2-dispositions", components: []string{"controlled-artifact-access-v2", "access-dispositions"}},
		{name: "checkpoint-snapshot-intents", components: []string{"checkpoint-authority", "snapshot-intents"}},
		{name: "checkpoint-snapshot-completions", components: []string{"checkpoint-authority", "snapshot-completions"}},
		{name: "checkpoint-snapshot-dispositions", components: []string{"checkpoint-authority", "snapshot-dispositions"}},
		{name: "checkpoint-operation-group-intents", components: []string{"checkpoint-authority", "operation-group-intents-v2"}},
		{name: "checkpoint-operation-group-terminals", components: []string{"checkpoint-authority", "operation-group-terminals-v2"}},
	}
	residues := make([]string, 0, len(fixtures))
	for index, fixture := range fixtures {
		digest := fmt.Sprintf("%02x%s", index+1, strings.Repeat("a", 62))
		root := filepath.Join(append([]string{dataDir, "private"}, fixture.components...)...)
		shard := filepath.Join(root, digest[:2])
		if err := os.MkdirAll(shard, 0o700); err != nil {
			t.Fatal(err)
		}
		temp := filepath.Join(shard, fmt.Sprintf(".%s.json-%024x.tmp", digest, 1))
		if err := os.WriteFile(temp, []byte("recoverable-residue"), 0o600); err != nil {
			t.Fatal(err)
		}
		residues = append(residues, temp)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("recover every runtime private CAS root before baseline: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	for index, residue := range residues {
		if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s residue entered or survived baseline: %v", fixtures[index].name, err)
		}
	}
}

func TestRuntimePrivateCASThirteenOwnerCrashResidueRestartsThroughProductionComposition(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir()}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("initialize production runtime composition: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)

	fixtures := []struct {
		digest     string
		components []string
	}{
		{digest: "71" + strings.Repeat("a", 62), components: []string{"accepted-finals", "records"}},
		{digest: "72" + strings.Repeat("b", 62), components: []string{"case-thread-authority"}},
		{digest: "73" + strings.Repeat("c", 62), components: []string{"gate-continuations", "receipts-v2"}},
		{digest: "74" + strings.Repeat("d", 62), components: []string{"pending-work", "receipts"}},
		{digest: "75" + strings.Repeat("e", 62), components: []string{"provider-cache-telemetry", "attempts"}},
		{digest: "76" + strings.Repeat("f", 62), components: []string{"turn-terminal-authority", "intents"}},
		{digest: "77" + strings.Repeat("1", 62), components: []string{"attachment-authority", "owners"}},
		{digest: "78" + strings.Repeat("2", 62), components: []string{"authority-advance", "v2", "intents"}},
		{digest: "79" + strings.Repeat("3", 62), components: []string{"pii-authorization", "grants"}},
		{digest: "7a" + strings.Repeat("4", 62), components: []string{"report-publication", "receipts"}},
		{digest: "7b" + strings.Repeat("5", 62), components: []string{"controlled-artifact-access", "access-receipts"}},
		{digest: "7c" + strings.Repeat("6", 62), components: []string{"controlled-artifact-access-v2", "access-receipts"}},
		{digest: "7d" + strings.Repeat("7", 62), components: []string{"checkpoint-authority", "snapshot-intents"}},
	}
	residues := make([]string, 0, len(fixtures))
	for index, fixture := range fixtures {
		root := filepath.Join(append([]string{dataDir, "private"}, fixture.components...)...)
		shard := filepath.Join(root, fixture.digest[:2])
		if err := os.Mkdir(shard, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			t.Fatalf("create owner %d crash shard: %v", index, err)
		}
		residue := filepath.Join(
			shard, fmt.Sprintf(".%s.json-%024x.tmp", fixture.digest, index+1),
		)
		if err := os.WriteFile(residue, []byte(fmt.Sprintf("owner-%d-crash-residue", index)), 0o600); err != nil {
			t.Fatalf("create owner %d crash residue: %v", index, err)
		}
		residues = append(residues, residue)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("restart production runtime through thirteen-owner recovery transaction: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	for index, residue := range residues {
		if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owner %d crash residue survived production startup: %v", index, err)
		}
	}
}

func initializeControlledArtifactAccessRecoveryOwner(t *testing.T, dataDir string) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := piiauthorizationstore.NewAccessStore(
		filepath.Join(dataDir, "private", "controlled-artifact-access"),
		access,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	v2Store, err := piiauthorizationstore.NewAccessStoreV2(
		filepath.Join(dataDir, "private", "controlled-artifact-access-v2"),
		access,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := v2Store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePrivateCASGlobalPreflightPreventsPartialCleanup(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir()}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)

	digest := "01" + strings.Repeat("b", 62)
	earlyShard := filepath.Join(dataDir, "private", "accepted-finals", "records", digest[:2])
	if err := os.MkdirAll(earlyShard, 0o700); err != nil {
		t.Fatal(err)
	}
	earlyTemp := filepath.Join(earlyShard, fmt.Sprintf(".%s.json-%024x.tmp", digest, 1))
	if err := os.WriteFile(earlyTemp, []byte("recoverable-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	lateShard := filepath.Join(dataDir, "private", "attachment-authority", "use-dispositions", "ff")
	if err := os.MkdirAll(lateShard, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lateShard, "unknown.bin"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}

	if handler, err := NewRuntimeServerHandlerE(config); err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("later corrupt CAS owner passed startup preflight")
	}
	if _, err := os.Lstat(earlyTemp); err != nil {
		t.Fatalf("later owner failure partially cleaned an earlier owner: %v", err)
	}
}

func TestRuntimeStartupQuarantinesOwnerlessAttachmentWithoutDeletingIt(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir()}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)

	store, err := filestore.NewPersistentAttachmentStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{
		"name": "legacy.txt", "mimeType": "text/plain",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("legacy ownerless bytes")),
		"threadId":   "thread_legacy", "workspace": t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("ownerless attachment was not quarantined during restart: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	if _, _, found, err := store.Content(id); err != nil || !found {
		t.Fatalf("restart deleted quarantined ownerless data: found=%v err=%v", found, err)
	}
}

func TestRuntimeStartupRejectsCorruptOwnerBackedAttachment(t *testing.T) {
	dataDir := t.TempDir()
	workspace := workspacetest.New(t)
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir()}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	thread := runtimeStartupJSON(t, initial, http.MethodPost, "/v1/threads", map[string]any{
		"title": "Attachment recovery", "workspace": workspace,
	}, http.StatusCreated)
	threadID, _ := thread["id"].(string)
	upload := runtimeStartupJSON(t, initial, http.MethodPost, "/v1/attachments", map[string]any{
		"name": "evidence.txt", "mimeType": "text/plain",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("owner-backed evidence")),
		"threadId":   threadID, "workspace": workspace,
	}, http.StatusCreated)
	attachment, _ := upload["attachment"].(map[string]any)
	id, _ := attachment["id"].(string)
	if id == "" {
		t.Fatalf("upload did not return attachment id: %#v", upload)
	}
	shutdownOwnedRuntimeHandler(t, initial)

	contentPath := filepath.Join(dataDir, "attachments", "content", id+".bin")
	if err := os.WriteFile(contentPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if handler, err := NewRuntimeServerHandlerE(config); err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("corrupt owner-backed attachment passed production startup reconciliation")
	}
}

func TestRuntimeAttachmentUploadCommitsPrivateDispositionBeforeCreated(t *testing.T) {
	dataDir := t.TempDir()
	workspace := workspacetest.New(t)
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir()}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	thread := runtimeStartupJSON(t, handler, http.MethodPost, "/v1/threads", map[string]any{
		"title": "Attachment transaction", "workspace": workspace,
	}, http.StatusCreated)
	threadID, _ := thread["id"].(string)
	upload := runtimeStartupJSON(t, handler, http.MethodPost, "/v1/attachments", map[string]any{
		"name": "evidence.txt", "mimeType": "text/plain",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("transactional evidence")),
		"threadId":   threadID, "workspace": workspace,
	}, http.StatusCreated)
	attachment, _ := upload["attachment"].(map[string]any)
	attachmentID, _ := attachment["id"].(string)
	if attachmentID == "" {
		t.Fatalf("created response lacked attachment id: %#v", upload)
	}
	shutdownOwnedRuntimeHandler(t, handler)

	authorityRoot := filepath.Join(dataDir, "private", "attachment-authority")
	access, err := privatecastest.NewAccessAuthority(authorityRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := attachmentauthority.NewStore(authorityRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	owners, intents, dispositions := 0, 0, 0
	if err := authority.VisitOwners(context.Background(), func(owner domainattachment.OwnerRecordV1) error {
		if owner.AttachmentID == attachmentID {
			owners++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := authority.VisitUploadIntents(context.Background(), func(intent domainattachment.UploadIntentV1) error {
		if intent.Owner.AttachmentID == attachmentID {
			intents++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := authority.VisitUploadDispositions(context.Background(), func(disposition domainattachment.UploadDispositionV1) error {
		if disposition.AttachmentID == attachmentID {
			dispositions++
			if disposition.Status != domainattachment.UploadDispositionCommittedV1 {
				t.Fatalf("HTTP 201 preceded committed disposition: %#v", disposition)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if owners != 1 || intents != 1 || dispositions != 1 {
		t.Fatalf("HTTP 201 did not bind one exact upload transaction: owners=%d intents=%d dispositions=%d", owners, intents, dispositions)
	}
}

func runtimeStartupJSON(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body map[string]any,
	wantStatus int,
) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status=%d body=%s", method, path, response.Code, response.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestPreparedRuntimeStartupActivatesOnlyBoundConfigurationOnce(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir(), Host: "127.0.0.1"}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
	if err != nil {
		t.Fatal(err)
	}
	changed := config
	changed.ApprovalPolicy = "never"
	if _, err := prepared.Activate(changed); err == nil || !strings.Contains(err.Error(), "startup security configuration changed") {
		t.Fatalf("security configuration drift was activated: %v", err)
	}
	config.Port = 43123
	handler, err := prepared.Activate(config)
	if err != nil || handler == nil {
		t.Fatalf("ephemeral listener address did not activate prepared startup: handler=%T err=%v", handler, err)
	}
	if _, err := prepared.Activate(config); err == nil {
		t.Fatal("prepared runtime startup activated twice")
	}
}

func TestSamePersistenceLeaseCannotActivateSecondRuntimeHandler(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir(), Host: "127.0.0.1"}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	handler, err := NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease)
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle, ok := handler.(interface{ Shutdown(context.Context) error }); ok {
		defer lifecycle.Shutdown(context.Background())
	}
	if _, err := NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease); !errors.Is(err, persistencefs.ErrPersistenceActivationInUse) {
		t.Fatalf("same persistence lease activated a second runtime handler: %v", err)
	}
}

func TestPreparedRuntimeStartupRejectsManagedGenerationDriftBeforeActivation(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir(), Host: "127.0.0.1"}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.managedSnapshotDigest == "" || prepared.generationDigest == "" {
		t.Fatalf("prepared startup omitted its final generation authority: %#v", prepared)
	}
	records := filepath.Join(dataDir, "memory", "records")
	if err := os.MkdirAll(records, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(records, "mem_go_999.json"), []byte(`{"id":"mem_go_999","content":"post-plan drift"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Activate(config); err == nil || !strings.Contains(err.Error(), "managed persistence changed after startup preparation") {
		t.Fatalf("post-plan managed drift was activated: %v", err)
	}
}

func TestStartupSecurityConfigurationExcludesEphemeralAndSecretValues(t *testing.T) {
	base := Config{
		RuntimeToken: "runtime-secret-a", StartedAt: "2026-07-11T00:00:00Z", Host: "127.0.0.1", Port: 4242,
		DataDir: "/data", DurableTempDir: "/durable", ProviderID: "deepseek", BaseURL: "https://api.example/v1",
		APIKey: "provider-secret-a", Model: "deepseek-chat", EndpointFormat: "openai-chat-completions",
		ModelProvidersJSON: `{"providers":[]}`, ModelProxyURL: "http://proxy-a", MCPProxyURL: "http://mcp-proxy-a",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		AuthorityAnchorV1: "authority-anchor-private-a", AuthorityManifestRoot: "/private/authority-manifest-a",
		AuthorityCredentialProfileRoot: "/private/authority-profile-a",
		AuthorityCredentialBundleRoot:  "/private/authority-bundle-a",
		Routes:                         []protocol.G2RouteReplayCase{{ID: "route-a"}},
	}
	startupBody, err := json.Marshal(runtimeStartupSecurityConfiguration(base))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(startupBody, []byte("darwinKeychainBindingDigest")) {
		t.Fatal("default startup security configuration changed without an explicit task Keychain binding")
	}
	for _, raw := range []string{
		base.AuthorityAnchorV1, base.AuthorityManifestRoot,
		base.AuthorityCredentialProfileRoot, base.AuthorityCredentialBundleRoot,
	} {
		if bytes.Contains(startupBody, []byte(raw)) {
			t.Fatalf("durable startup configuration exposed protected authority input %q", raw)
		}
	}
	changedEphemeral := base
	changedEphemeral.RuntimeToken = "runtime-secret-b"
	changedEphemeral.APIKey = "provider-secret-b"
	changedEphemeral.StartedAt = "2026-07-12T00:00:00Z"
	changedEphemeral.Host = "localhost"
	changedEphemeral.Port = 5252
	changedEphemeral.ModelProxyURL = "http://proxy-b"
	changedEphemeral.MCPProxyURL = "http://mcp-proxy-b"
	if left, right := runtimeStartupSecurityConfiguration(base), runtimeStartupSecurityConfiguration(changedEphemeral); !reflect.DeepEqual(left, right) {
		t.Fatalf("ephemeral or raw secret values changed durable startup semantics: left=%#v right=%#v", left, right)
	}

	changedPolicy := base
	changedPolicy.ApprovalPolicy = "never"
	if reflect.DeepEqual(runtimeStartupSecurityConfiguration(base), runtimeStartupSecurityConfiguration(changedPolicy)) {
		t.Fatal("persistent execution policy change was omitted from startup semantics")
	}
	changedRoute := base
	changedRoute.Routes = []protocol.G2RouteReplayCase{{ID: "route-b"}}
	if reflect.DeepEqual(runtimeStartupSecurityConfiguration(base), runtimeStartupSecurityConfiguration(changedRoute)) {
		t.Fatal("persistent conformance route change was omitted from startup semantics")
	}
	changedAuthority := base
	changedAuthority.AuthorityAnchorV1 = "authority-anchor-private-b"
	if reflect.DeepEqual(runtimeStartupSecurityConfiguration(base), runtimeStartupSecurityConfiguration(changedAuthority)) {
		t.Fatal("protected authority configuration change was omitted from startup semantics")
	}
	changedKeychain := base
	changedKeychain.DarwinSecretStoreKeychainDBPath = "/private/task/login.keychain-db"
	changedKeychain.DarwinSecretStoreKeychainSecurityDigest = strings.Repeat("a", 64)
	changedKeychain.DarwinSecretStoreKeychainBindingDigest = strings.Repeat("b", 64)
	keychainSecurity := runtimeStartupSecurityConfiguration(changedKeychain)
	if reflect.DeepEqual(runtimeStartupSecurityConfiguration(base), keychainSecurity) {
		t.Fatal("explicit task Keychain binding was omitted from startup semantics")
	}
	keychainBody, err := json.Marshal(keychainSecurity)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(keychainBody, []byte(changedKeychain.DarwinSecretStoreKeychainDBPath)) ||
		bytes.Contains(keychainBody, []byte(changedKeychain.DarwinSecretStoreKeychainSecurityDigest)) {
		t.Fatal("startup security configuration retained raw task Keychain path or identity")
	}
}
