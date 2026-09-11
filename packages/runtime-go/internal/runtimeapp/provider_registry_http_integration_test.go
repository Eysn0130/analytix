package runtimeapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestProviderRegistryProductionCompositionListsRecoversAndClosesWithoutCredentialState(t *testing.T) {
	t.Parallel()

	authority, err := openProviderRegistryAuthorityV1(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1() error = %v", err)
	}
	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}

	list := httptest.NewRecorder()
	handler.Handle(list, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"providers":[]`) ||
		strings.Contains(list.Body.String(), "credentialRef") {
		t.Fatalf("list response = (%d, %s)", list.Code, list.Body.String())
	}

	recoverRecorder := httptest.NewRecorder()
	handler.Handle(recoverRecorder, httptest.NewRequest(http.MethodPost, httpapi.ProviderRegistryPathV1+"/recover", strings.NewReader(`{"schemaVersion":1}`)))
	if recoverRecorder.Code != http.StatusOK || !strings.Contains(recoverRecorder.Body.String(), `"recovered":true`) {
		t.Fatalf("recover response = (%d, %s)", recoverRecorder.Code, recoverRecorder.Body.String())
	}

	if err := authority.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := authority.Manager().Snapshot(context.Background()); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("Snapshot() after Close error = %v", err)
	}
	if _, err := authority.secrets.PreparePut(context.Background(), secretstoreport.Purpose("provider-api-key"), []byte("synthetic-closed-store-marker")); !errors.Is(err, secretstoreport.ErrClosed) {
		t.Fatalf("PreparePut() after Close error = %v", err)
	}
}

func TestProviderRegistryProductionCompositionPreparesCommitsAndRetainsLegacyMigrationRecoveryThroughPrivateHTTP(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}

	dataDir := t.TempDir()
	installSyntheticProviderRegistryFallbackMasterKey(t, dataDir)
	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1() error = %v", err)
	}
	t.Cleanup(func() {
		if err := authority.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	snapshot, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	secretMarker := []byte("synthetic-runtimeapp-legacy-migration-marker")
	shadowRuntimeMarker := []byte("synthetic-runtimeapp-shadow-runtime-marker")
	shadowLegacyMarker := []byte("synthetic-runtimeapp-shadow-legacy-marker")
	sourceSnapshot := []byte(`{"agents":{"kun":{"apiKey":"` + string(secretMarker) + `"}},"deepseek":{"apiKey":"` + string(shadowLegacyMarker) + `"},"provider":{"apiKey":"` + string(shadowRuntimeMarker) + `","providers":[{"apiKey":"` + string(secretMarker) + `"}]},"runtime":{"apiKey":"` + string(shadowRuntimeMarker) + `"}}`)
	cleanedSource := []byte(`{"agents":{"kun":{}},"deepseek":{},"provider":{"providers":[{}]},"runtime":{}}`)
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	encodedShadowRuntime := base64.StdEncoding.EncodeToString(shadowRuntimeMarker)
	encodedShadowLegacy := base64.StdEncoding.EncodeToString(shadowLegacyMarker)
	encodedSourceSnapshot := base64.StdEncoding.EncodeToString(sourceSnapshot)
	activeLocators := []string{
		"current:analytix-settings.json:agents.kun.apiKey",
		"current:analytix-settings.json:provider.providers[0].apiKey",
	}
	shadowRuntimeLocators := []string{
		"current:analytix-settings.json:provider.apiKey",
		"current:analytix-settings.json:runtime.apiKey",
	}
	shadowLegacyLocator := "current:analytix-settings.json:deepseek.apiKey"
	sourceLocator := "current:analytix-settings.json"
	sourceDigest := sha256.Sum256(sourceSnapshot)
	sourceSHA256 := hex.EncodeToString(sourceDigest[:])
	cleanedSourceDigest := sha256.Sum256(cleanedSource)
	expectedCleanedSourceSHA256 := hex.EncodeToString(cleanedSourceDigest[:])
	physicalIdentityDigest := sha256.Sum256([]byte("synthetic-runtimeapp-source-physical-identity"))
	sourcePhysicalIdentitySHA256 := hex.EncodeToString(physicalIdentityDigest[:])
	migrationID := "migration-settings-alpha"
	body := `{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"` + snapshot.Incarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"migrationId":"` + migrationID + `","sourceLocator":"` + sourceLocator + `","sourceSHA256":"` + sourceSHA256 + `","expectedCleanedSourceSHA256":"` + expectedCleanedSourceSHA256 + `","sourcePhysicalIdentitySHA256":"` + sourcePhysicalIdentitySHA256 + `","provider":{"id":"provider-alpha","kind":"openai-compatible","endpoint":"https://provider.invalid/v1","proxy":"","models":["model-alpha"],"mediaModels":[],"selectedModel":"model-alpha","selectedMediaModel":"","selectedRoutes":["primary"]},"credential":{"purpose":"provider-api-key","valueBase64":"` + encodedMarker + `"},"rollbackSource":{"valueBase64":"` + encodedSourceSnapshot + `"},"activeCredentialLocators":["` + strings.Join(activeLocators, `","`) + `"],"rollbackCredentialArtifacts":[{"schemaVersion":1,"credentialLocators":["` + strings.Join(shadowRuntimeLocators, `","`) + `"],"credential":{"valueBase64":"` + encodedShadowRuntime + `"}},{"schemaVersion":1,"credentialLocators":["` + shadowLegacyLocator + `"],"credential":{"valueBase64":"` + encodedShadowLegacy + `"}}]}`
	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	prepared := httptest.NewRecorder()
	handler.Handle(prepared, httptest.NewRequest(
		http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/_private/legacy-migration-recovery/prepare",
		strings.NewReader(body),
	))
	if prepared.Code != http.StatusOK || !strings.Contains(prepared.Body.String(), `"status":"VERIFIED_RECOVERY"`) ||
		!strings.Contains(prepared.Body.String(), `"migrationId":"`+migrationID+`"`) ||
		!strings.Contains(prepared.Body.String(), `"safeToProceedWithProviderMigration":true`) {
		t.Fatalf("prepare response = (%d, %s)", prepared.Code, prepared.Body.String())
	}
	for _, forbidden := range []string{
		string(secretMarker), string(shadowRuntimeMarker), string(shadowLegacyMarker),
		encodedMarker, encodedShadowRuntime, encodedShadowLegacy, encodedSourceSnapshot,
		sourceSHA256, activeLocators[0], activeLocators[1], shadowRuntimeLocators[0],
		shadowRuntimeLocators[1], shadowLegacyLocator, "provider.invalid", "raw-provider-body",
	} {
		if strings.Contains(prepared.Body.String(), forbidden) {
			t.Fatalf("prepare response leaks %q: %s", forbidden, prepared.Body.String())
		}
	}

	preparedState, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() after prepare error = %v", err)
	}
	recovery, ok := preparedState.LegacyMigrationRecoveries[migrationID]
	if !ok || len(preparedState.LegacyMigrationRecoveries) != 1 ||
		recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified ||
		recovery.ProviderID != "provider-alpha" || recovery.RecoveryCredentialRef == "" ||
		len(preparedState.Providers) != 0 {
		t.Fatalf("prepared recovery state = %#v; Providers = %d", recovery, len(preparedState.Providers))
	}
	if !strings.Contains(prepared.Body.String(), `"recoveryCredentialRef":"`+recovery.RecoveryCredentialRef+`"`) {
		t.Fatalf("prepare response did not correlate the opaque recovery reference: %s", prepared.Body.String())
	}

	recovered := httptest.NewRecorder()
	handler.Handle(recovered, httptest.NewRequest(
		http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/recover",
		strings.NewReader(`{"schemaVersion":1}`),
	))
	if recovered.Code != http.StatusOK || !strings.Contains(recovered.Body.String(), `"recovered":true`) ||
		!strings.Contains(recovered.Body.String(), `"providers":[]`) {
		t.Fatalf("recover response = (%d, %s)", recovered.Code, recovered.Body.String())
	}
	for _, forbidden := range []string{migrationID, recovery.RecoveryCredentialRef, "credentialRef", sourceSHA256, encodedMarker, string(secretMarker)} {
		if strings.Contains(recovered.Body.String(), forbidden) {
			t.Fatalf("public recover response leaks %q: %s", forbidden, recovered.Body.String())
		}
	}
	afterRecover, err := authority.Manager().Snapshot(context.Background())
	if err != nil || afterRecover.LegacyMigrationRecoveries[migrationID].Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified {
		t.Fatalf("recovery did not preserve the verified protected record: error=%v state=%#v", err, afterRecover.LegacyMigrationRecoveries[migrationID])
	}

	commitBody := `{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"` + snapshot.Incarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"expectedSelectedProviderID":"","migrationId":"` + migrationID + `","sourceLocator":"` + sourceLocator + `","sourceSHA256":"` + sourceSHA256 + `","recoveryCredentialRef":"` + recovery.RecoveryCredentialRef + `","confirmation":"COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY"}`
	committed := httptest.NewRecorder()
	handler.Handle(committed, httptest.NewRequest(
		http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/_private/legacy-migration-recovery/commit",
		strings.NewReader(commitBody),
	))
	if committed.Code != http.StatusOK ||
		!strings.Contains(committed.Body.String(), `"status":"PROVIDER_COMMITTED_RECOVERY_RETAINED"`) ||
		!strings.Contains(committed.Body.String(), `"migrationId":"`+migrationID+`"`) ||
		!strings.Contains(committed.Body.String(), `"safeToRemoveLegacyPlaintext":true`) ||
		!strings.Contains(committed.Body.String(), `"provider":{"id":"provider-alpha"`) {
		t.Fatalf("commit response = (%d, %s)", committed.Code, committed.Body.String())
	}
	for _, forbidden := range []string{
		string(secretMarker), encodedMarker, sourceSHA256, recovery.RecoveryCredentialRef,
		"credentialRef", "valueBase64", "raw-provider-body", "masterKey", "ciphertext",
	} {
		if strings.Contains(committed.Body.String(), forbidden) {
			t.Fatalf("commit response leaks %q: %s", forbidden, committed.Body.String())
		}
	}

	committedState, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() after commit error = %v", err)
	}
	committedRecovery, ok := committedState.LegacyMigrationRecoveries[migrationID]
	committedProvider, providerOK := committedState.Providers["provider-alpha"]
	if !ok || len(committedState.LegacyMigrationRecoveries) != 1 ||
		committedRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		!providerOK || committedState.SelectedProviderID != committedProvider.ID ||
		committedProvider.CredentialRef == "" ||
		committedProvider.CredentialRef == committedRecovery.RecoveryCredentialRef {
		t.Fatalf("committed-retained state = recovery=%#v provider=%#v selected=%q", committedRecovery, committedProvider, committedState.SelectedProviderID)
	}

	winnerCredential, err := authority.secrets.GetForAuthorizedConsumer(context.Background(), secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(committedProvider.CredentialRef),
		Purpose:       secretstoreport.Purpose(committedProvider.CredentialPurpose),
		Consumer:      providerregistryapp.RegistryReadbackConsumer,
	})
	if err != nil {
		clearRuntimeappTestBytes(winnerCredential)
		t.Fatalf("GetForAuthorizedConsumer(provider winner) error = %v", err)
	}
	if !bytes.Equal(winnerCredential, secretMarker) || bytes.Equal(winnerCredential, shadowRuntimeMarker) ||
		bytes.Equal(winnerCredential, shadowLegacyMarker) {
		clearRuntimeappTestBytes(winnerCredential)
		t.Fatal("committed Provider winner was not exactly the active credential")
	}
	clearRuntimeappTestBytes(winnerCredential)

	protectedRecovery, err := authority.secrets.GetForAuthorizedConsumer(context.Background(), secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(committedRecovery.RecoveryCredentialRef),
		Purpose:       providerregistryapp.LegacyMigrationRecoveryPurpose,
		Consumer:      providerregistryapp.RegistryReadbackConsumer,
	})
	if err != nil {
		clearRuntimeappTestBytes(protectedRecovery)
		t.Fatalf("GetForAuthorizedConsumer(protected recovery) error = %v", err)
	}
	defer clearRuntimeappTestBytes(protectedRecovery)
	if len(protectedRecovery) < 8 || !bytes.Equal(protectedRecovery[:4], []byte("ALMR")) ||
		binary.BigEndian.Uint32(protectedRecovery[4:8]) != 6 {
		t.Fatal("retained protected recovery is not canonical v6")
	}
	for _, required := range [][]byte{
		secretMarker, shadowRuntimeMarker, shadowLegacyMarker, sourceSnapshot,
		[]byte(activeLocators[0]), []byte(activeLocators[1]),
		[]byte(shadowRuntimeLocators[0]), []byte(shadowRuntimeLocators[1]), []byte(shadowLegacyLocator),
	} {
		if !bytes.Contains(protectedRecovery, required) {
			t.Fatalf("retained protected v6 recovery is missing bounded canary length %d", len(required))
		}
	}

	publicList := httptest.NewRecorder()
	handler.Handle(publicList, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil))
	if publicList.Code != http.StatusOK ||
		!strings.Contains(publicList.Body.String(), `"selectedProviderId":"provider-alpha"`) ||
		!strings.Contains(publicList.Body.String(), `"credentialConfigured":true`) {
		t.Fatalf("public list response = (%d, %s)", publicList.Code, publicList.Body.String())
	}
	for _, forbidden := range []string{
		string(secretMarker), encodedMarker, sourceSHA256, committedRecovery.RecoveryCredentialRef,
		committedProvider.CredentialRef, "credentialRef", "valueBase64", "raw-provider-body",
	} {
		if strings.Contains(publicList.Body.String(), forbidden) {
			t.Fatalf("public list response leaks %q: %s", forbidden, publicList.Body.String())
		}
	}

	secretStoreBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-secrets", providerRegistrySecretStoreFileV1))
	if err != nil {
		t.Fatalf("ReadFile(secret store) error = %v", err)
	}
	defer clearRuntimeappTestBytes(secretStoreBytes)
	for _, forbidden := range [][]byte{
		secretMarker, shadowRuntimeMarker, shadowLegacyMarker,
		[]byte(encodedMarker), []byte(encodedShadowRuntime), []byte(encodedShadowLegacy), []byte(encodedSourceSnapshot),
		[]byte(sourceSHA256), []byte(activeLocators[0]), []byte(activeLocators[1]),
		[]byte(shadowRuntimeLocators[0]), []byte(shadowRuntimeLocators[1]), []byte(shadowLegacyLocator),
	} {
		if bytes.Contains(secretStoreBytes, forbidden) {
			t.Fatalf("encrypted Secret Store contains plaintext marker %q", forbidden)
		}
	}
	registryBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-registry", "registry.v1.json"))
	if err != nil {
		t.Fatalf("ReadFile(Registry) error = %v", err)
	}
	defer clearRuntimeappTestBytes(registryBytes)
	for _, forbidden := range [][]byte{
		secretMarker, shadowRuntimeMarker, shadowLegacyMarker,
		[]byte(encodedMarker), []byte(encodedShadowRuntime), []byte(encodedShadowLegacy), []byte(encodedSourceSnapshot),
		[]byte(activeLocators[0]), []byte(activeLocators[1]),
		[]byte(shadowRuntimeLocators[0]), []byte(shadowRuntimeLocators[1]), []byte(shadowLegacyLocator),
	} {
		if bytes.Contains(registryBytes, forbidden) {
			t.Fatalf("Registry contains plaintext/source marker %q", forbidden)
		}
	}
	if committedState.LegacyMigrationRecoveries[migrationID].Phase !=
		domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
		t.Fatal("private commit cleaned the retained recovery source authority")
	}
	initialExpected := `"registryRevision":"0","registryIncarnation":"` + snapshot.Incarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""`
	currentExpected := `"registryRevision":"` + strconv.FormatUint(committedState.Revision, 10) + `","registryIncarnation":"` + committedState.Incarnation + `","providerRevision":"` + strconv.FormatUint(committedProvider.Revision, 10) + `","providerGeneration":"` + strconv.FormatUint(committedProvider.Generation, 10) + `","providerIncarnation":"` + committedProvider.Incarnation + `","providerCredentialPurpose":"` + committedProvider.CredentialPurpose + `"`
	replayBody := strings.Replace(body, initialExpected, currentExpected, 1)
	if replayBody == body {
		t.Fatal("retained replay body did not advance to the current Registry/Provider CAS")
	}

	assertRetainedReplay := func(label string) {
		t.Helper()
		replayed := httptest.NewRecorder()
		handler.Handle(replayed, httptest.NewRequest(
			http.MethodPost,
			httpapi.ProviderRegistryPathV1+"/_private/legacy-migration-recovery/prepare",
			strings.NewReader(replayBody),
		))
		wantBody := `{"schemaVersion":1,"status":"PROVIDER_COMMITTED_RECOVERY_RETAINED","migrationId":"` + migrationID + `","recoveryCredentialRef":"` + committedRecovery.RecoveryCredentialRef + `","safeToProceedWithProviderMigration":false}` + "\n"
		if replayed.Code != http.StatusOK || replayed.Body.String() != wantBody {
			t.Fatalf("%s retained replay response = (%d, %s), want %q", label, replayed.Code, replayed.Body.String(), wantBody)
		}
		for _, forbidden := range []string{
			string(secretMarker), string(shadowRuntimeMarker), string(shadowLegacyMarker),
			encodedMarker, encodedShadowRuntime, encodedShadowLegacy, sourceSHA256,
			activeLocators[0], activeLocators[1], shadowRuntimeLocators[0],
			shadowRuntimeLocators[1], shadowLegacyLocator, "provider.invalid", "raw-provider-body",
		} {
			if strings.Contains(replayed.Body.String(), forbidden) {
				t.Fatalf("%s retained replay response leaks %q: %s", label, forbidden, replayed.Body.String())
			}
		}
	}
	assertUnchanged := func(label string) {
		t.Helper()
		afterReplay, err := authority.Manager().Snapshot(context.Background())
		if err != nil || !reflect.DeepEqual(afterReplay, committedState) {
			t.Fatalf("%s retained replay changed Registry state: error=%v\nbefore=%#v\nafter=%#v", label, err, committedState, afterReplay)
		}
		afterSecretStoreBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-secrets", providerRegistrySecretStoreFileV1))
		if err != nil {
			t.Fatalf("%s ReadFile(secret store) error = %v", label, err)
		}
		defer clearRuntimeappTestBytes(afterSecretStoreBytes)
		if !bytes.Equal(afterSecretStoreBytes, secretStoreBytes) {
			t.Fatalf("%s retained replay changed encrypted Secret Store bytes", label)
		}
		afterRegistryBytes, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-registry", "registry.v1.json"))
		if err != nil {
			t.Fatalf("%s ReadFile(Registry) error = %v", label, err)
		}
		defer clearRuntimeappTestBytes(afterRegistryBytes)
		if !bytes.Equal(afterRegistryBytes, registryBytes) {
			t.Fatalf("%s retained replay changed Registry bytes", label)
		}
	}

	assertRetainedReplay("same process")
	assertUnchanged("same process")
	if err := authority.Close(); err != nil {
		t.Fatalf("Close() before fresh runtimeapp restart error = %v", err)
	}
	authority, err = openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1() after restart error = %v", err)
	}
	handler = httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	assertRetainedReplay("fresh runtimeapp restart")
	assertUnchanged("fresh runtimeapp restart")
}

func installSyntheticProviderRegistryFallbackMasterKey(t *testing.T, dataDir string) {
	t.Helper()
	directory := filepath.Join(dataDir, "private", "provider-secrets", "master-key")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("MkdirAll(master-key) error = %v", err)
	}
	key := bytes.Repeat([]byte{0x6a}, 32)
	defer clearRuntimeappTestBytes(key)
	if err := os.WriteFile(filepath.Join(directory, "master.key"), key, 0o600); err != nil {
		t.Fatalf("WriteFile(master.key) error = %v", err)
	}
	if runtime.GOOS == "darwin" {
		if err := os.WriteFile(
			filepath.Join(directory, "authority.v1"),
			[]byte("analytix-master-key-authority:v1:fallback\n"),
			0o600,
		); err != nil {
			t.Fatalf("WriteFile(authority.v1) error = %v", err)
		}
	}
}

func clearRuntimeappTestBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
