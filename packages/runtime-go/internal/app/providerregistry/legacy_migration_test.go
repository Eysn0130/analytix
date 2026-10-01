package providerregistry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

var legacyMigrationTestPhysicalSources = struct {
	sync.Mutex
	root    string
	paths   map[string]string
	counter uint64
}{paths: make(map[string]string)}

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "analytix-provider-migration-source-")
	if err != nil {
		panic(err)
	}
	legacyMigrationTestPhysicalSources.Lock()
	legacyMigrationTestPhysicalSources.root = root
	legacyMigrationTestPhysicalSources.Unlock()
	code := m.Run()
	if err := os.RemoveAll(root); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

func TestManagerPreparesVerifiedLegacyMigrationRecoveryBeforeSourceRemoval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	recoveryRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43))
	credentialCanary := []byte("synthetic-legacy-migration-canary-not-a-real-key")
	sourceSnapshot := []byte(`{"provider":{"apiKey":"synthetic-legacy-migration-canary-not-a-real-key"}}`)
	sourceDigestBytes := sha256.Sum256(sourceSnapshot)
	sourceDigest := hex.EncodeToString(sourceDigestBytes[:])
	cleanedDigestBytes := sha256.Sum256([]byte(`{"provider":{}}`))
	cleanedDigest := hex.EncodeToString(cleanedDigestBytes[:])
	physicalIdentitySHA256 := strings.Repeat("e", 64)
	events := make([]string, 0, 12)
	sourceMutationCalls := 0

	registry := &legacyMigrationRecordingRegistryStore{
		state: domainregistry.Registry{
			Version:                   domainregistry.FormatVersion,
			Incarnation:               registryIncarnation,
			Providers:                 map[string]domainregistry.Provider{},
			Transactions:              map[string]domainregistry.Transaction{},
			LegacyMigrationRecoveries: map[string]domainregistry.LegacyMigrationRecovery{},
		},
		events: &events,
	}
	secrets := &legacyMigrationRecordingSecretStore{
		candidateRef:         recoveryRef,
		expectedCredential:   bytes.Clone(credentialCanary),
		expectedSourceSHA256: sourceDigest,
		events:               &events,
	}
	manager, err := NewManager(registry, secrets)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	result, err := manager.PrepareLegacyMigrationRecovery(ctx, LegacyMigrationCandidate{
		Expected: domainregistry.ExpectedState{
			RegistryRevision:    0,
			RegistryIncarnation: registryIncarnation,
		},
		MigrationID:                  "migration-settings-alpha",
		SourceLocator:                "current:analytix-settings.json",
		SourceSHA256:                 sourceDigest,
		ExpectedCleanedSourceSHA256:  cleanedDigest,
		SourcePhysicalIdentitySHA256: physicalIdentitySHA256,
		SourceSnapshot:               sourceSnapshot,
		ActiveCredentialLocators: []string{
			"current:analytix-settings.json:provider.apiKey",
		},
		Provider: domainregistry.ProviderInput{
			ID:             "provider-alpha",
			Kind:           "openai-compatible",
			Endpoint:       "https://provider.invalid/v1",
			Models:         []string{"model-alpha"},
			MediaModels:    []string{},
			SelectedModel:  "model-alpha",
			SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
		Credential:        credentialCanary,
	})
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	if result.Status != LegacyMigrationRecoveryStatusVerified || !result.SafeToProceedWithProviderMigration {
		t.Fatalf("PrepareLegacyMigrationRecovery() result = %#v, want VERIFIED_RECOVERY", result)
	}
	if result.MigrationID != "migration-settings-alpha" || result.RecoveryCredentialRef != recoveryRef {
		t.Fatalf("PrepareLegacyMigrationRecovery() identity = %#v", result)
	}
	if sourceMutationCalls != 0 {
		t.Fatalf("legacy settings source mutation calls = %d, want 0", sourceMutationCalls)
	}
	for _, event := range events {
		if strings.HasPrefix(event, "source:") {
			t.Fatalf("legacy settings source callback was exposed or called: %v", events)
		}
	}
	if bytes.Contains(registry.committedBytes, credentialCanary) {
		t.Fatal("durable Registry bytes contain the synthetic credential canary")
	}

	wantOrder := []string{
		"secret:recovery-candidate-prepared",
		"registry:migration-recovery-prepared",
		"secret:recovery-candidate-durable",
		"registry:migration-recovery-secret-durable",
		"secret:authorized-recovery-readback-verified",
		"registry:migration-recovery-verified",
	}
	last := -1
	for _, want := range wantOrder {
		index := indexOfEvent(events, want)
		if index < 0 {
			t.Fatalf("events are missing %q: %v", want, events)
		}
		if index <= last {
			t.Fatalf("events are out of order for %q: %v", want, events)
		}
		last = index
	}
	if !secrets.readbackAuthorized {
		t.Fatal("Manager did not use the authorized internal consumer recovery readback seam")
	}
	stored := registry.state.LegacyMigrationRecoveries["migration-settings-alpha"]
	if stored.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified ||
		stored.ProviderID != "provider-alpha" ||
		stored.RecoveryCredentialRef != string(recoveryRef) {
		t.Fatalf("durable legacy migration recovery = %#v", stored)
	}
}

func TestManagerProtectsShadowCredentialsInLegacyMigrationRecoveryBeforeProviderCommit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(),
		"migration-shadow-credentials",
		"provider-shadow-credentials",
		"source-shadow-credentials",
		"synthetic-active-credential-not-a-real-key",
	)
	candidate.ActiveCredentialLocators = []string{
		"current:analytix-settings.json:agents.kun.apiKey",
		"current:analytix-settings.json:provider.providers[0].apiKey",
	}
	candidate.RollbackCredentialArtifacts = []LegacyMigrationRollbackCredentialArtifact{
		{
			Locators: []string{
				"current:analytix-settings.json:provider.apiKey",
				"current:analytix-settings.json:runtime.apiKey",
			},
			Credential: []byte("synthetic-shadow-runtime-credential-not-a-real-key"),
		},
		{
			Locators:   []string{"current:analytix-settings.json:deepseek.apiKey"},
			Credential: []byte("synthetic-shadow-legacy-credential-not-a-real-key"),
		},
	}
	activeBefore := bytes.Clone(candidate.Credential)
	locatorsBefore := append([]string(nil), candidate.ActiveCredentialLocators...)
	artifactsBefore := []LegacyMigrationRollbackCredentialArtifact{
		{
			Locators:   append([]string(nil), candidate.RollbackCredentialArtifacts[0].Locators...),
			Credential: bytes.Clone(candidate.RollbackCredentialArtifacts[0].Credential),
		},
		{
			Locators:   append([]string(nil), candidate.RollbackCredentialArtifacts[1].Locators...),
			Credential: bytes.Clone(candidate.RollbackCredentialArtifacts[1].Credential),
		},
	}

	result, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	if result.Status != LegacyMigrationRecoveryStatusVerified ||
		!result.SafeToProceedWithProviderMigration {
		t.Fatalf("PrepareLegacyMigrationRecovery() result = %#v", result)
	}

	state := registry.snapshot()
	recovery, exists := state.LegacyMigrationRecoveries[candidate.MigrationID]
	if !exists || recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified ||
		len(state.Providers) != 0 || len(state.Transactions) != 0 {
		t.Fatalf("pre-commit migration recovery state = %#v", state)
	}
	secrets.mu.Lock()
	record, recordExists := secrets.records[recovery.RecoveryCredentialRef]
	protectedPayload := bytes.Clone(record.secret)
	secrets.mu.Unlock()
	defer clear(protectedPayload)
	if !recordExists || record.purpose != LegacyMigrationRecoveryPurpose || record.tombstoned || record.tampered {
		t.Fatalf("protected recovery record = %#v, exists = %t", record, recordExists)
	}
	if len(protectedPayload) < len(legacyMigrationRecoveryPayloadMagic)+4 ||
		binary.BigEndian.Uint32(protectedPayload[len(legacyMigrationRecoveryPayloadMagic):]) !=
			legacyMigrationRecoveryPayloadVersionV6 {
		t.Fatal("shadow-bearing recovery was not written with protected payload v6")
	}
	protectedCanaries := [][]byte{
		candidate.Credential,
		candidate.RollbackCredentialArtifacts[0].Credential,
		candidate.RollbackCredentialArtifacts[1].Credential,
		[]byte(candidate.ActiveCredentialLocators[0]),
		[]byte(candidate.ActiveCredentialLocators[1]),
		[]byte(candidate.RollbackCredentialArtifacts[0].Locators[0]),
		[]byte(candidate.RollbackCredentialArtifacts[0].Locators[1]),
		[]byte(candidate.RollbackCredentialArtifacts[1].Locators[0]),
	}
	for _, canary := range protectedCanaries {
		if !bytes.Contains(protectedPayload, canary) {
			t.Fatalf("protected recovery payload is missing bounded canary of length %d", len(canary))
		}
	}

	registryBytes, err := domainregistry.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal(Registry) error = %v", err)
	}
	for _, canary := range protectedCanaries {
		if bytes.Contains(registryBytes, canary) {
			t.Fatalf("key-free Registry bytes contain protected canary of length %d", len(canary))
		}
	}
	if !bytes.Equal(candidate.Credential, activeBefore) ||
		!reflect.DeepEqual(candidate.ActiveCredentialLocators, locatorsBefore) ||
		!reflect.DeepEqual(candidate.RollbackCredentialArtifacts, artifactsBefore) {
		t.Fatal("Manager mutated caller-owned active or shadow recovery input")
	}
}

func TestLegacyMigrationRecoveryV2AcceptsCanonicalSettingsInventoryLocatorsWithoutPathWidening(t *testing.T) {
	t.Parallel()

	registry := newMemoryRegistryStore()
	candidate := legacyMigrationV1CandidateFor(
		registry.snapshot(),
		"migration-v2-canonical-locators",
		"provider-v2-canonical-locators",
		"source-v2-canonical-locators",
		"synthetic-active-canonical-locators-not-a-real-key",
	)
	candidate.ActiveCredentialLocators = []string{
		"current:analytix-settings.json:runtime.apiKey",
		"current:analytix-settings.json:deepseek.apiKey",
		"current:analytix-settings.json:agents.reasonix.apiKey",
		"current:analytix-settings.json:agents.codewhale.apiKey",
		"current:analytix-settings.json:agents.kun.apiKey",
		"current:analytix-settings.json:provider.apiKey",
		"current:analytix-settings.json:provider.providers[0].apiKey",
		"current:analytix-settings.json:provider.providers[63].apiKey",
	}
	candidate.RollbackCredentialArtifacts = []LegacyMigrationRollbackCredentialArtifact{
		{
			Locators: []string{
				"compatibility:00:kun-settings.json:provider.apiKey",
				"compatibility:01:analytix-settings.json:runtime.apiKey",
			},
			Credential: []byte("synthetic-shadow-canonical-zero-not-a-real-key"),
		},
		{
			Locators: []string{
				"compatibility:02:kun-settings.json:provider.providers[1].apiKey",
			},
			Credential: []byte("synthetic-shadow-canonical-one-not-a-real-key"),
		},
	}

	_, payload, err := normalizeLegacyMigrationCandidate(candidate)
	if err != nil {
		t.Fatalf("normalizeLegacyMigrationCandidate(canonical settings locators) error = %v", err)
	}
	defer clear(payload)
	parsed, err := parseLegacyMigrationRecoveryPayload(payload)
	if err != nil {
		t.Fatalf("parseLegacyMigrationRecoveryPayload(canonical settings locators) error = %v", err)
	}
	if !equalLegacyMigrationLocatorView(parsed.activeCredentialLocators, candidate.ActiveCredentialLocators) ||
		len(parsed.rollbackCredentialArtifacts) != len(candidate.RollbackCredentialArtifacts) {
		t.Fatalf("parsed canonical locator view = %#v", parsed)
	}
	for index, artifact := range parsed.rollbackCredentialArtifacts {
		if !equalLegacyMigrationLocatorView(artifact.locators, candidate.RollbackCredentialArtifacts[index].Locators) {
			t.Fatalf("parsed canonical artifact %d locators = %#v", index, artifact.locators)
		}
	}

	invalidLocators := []struct {
		name    string
		locator string
	}{
		{name: "absolute path", locator: "/tmp/analytix-settings.json"},
		{name: "URL", locator: "https://provider.invalid/settings"},
		{name: "forward slash", locator: "current:analytix-settings.json:provider/providers[0].apiKey"},
		{name: "backslash", locator: `current:analytix-settings.json:provider\providers[0].apiKey`},
		{name: "traversal", locator: "current:../analytix-settings.json:provider.apiKey"},
		{name: "control", locator: "current:analytix-settings.json:provider.apiKey\n"},
		{name: "whitespace", locator: "current:analytix-settings.json:provider .apiKey"},
		{name: "unknown current filename", locator: "current:kun-settings.json:provider.apiKey"},
		{name: "empty current suffix", locator: "current:analytix-settings.json:"},
		{name: "extra current segment", locator: "current:analytix-settings.json:provider.apiKey:extra"},
		{name: "unknown suffix", locator: "current:analytix-settings.json:agents.unknown.apiKey"},
		{name: "profile leading zero", locator: "current:analytix-settings.json:provider.providers[00].apiKey"},
		{name: "profile nondecimal", locator: "current:analytix-settings.json:provider.providers[1a].apiKey"},
		{name: "profile negative", locator: "current:analytix-settings.json:provider.providers[-1].apiKey"},
		{name: "profile missing bracket", locator: "current:analytix-settings.json:provider.providers[1.apiKey"},
		{name: "profile out of range", locator: "current:analytix-settings.json:provider.providers[64].apiKey"},
		{name: "compatibility short index", locator: "compatibility:0:kun-settings.json:provider.apiKey"},
		{name: "compatibility long index", locator: "compatibility:000:kun-settings.json:provider.apiKey"},
		{name: "compatibility nondecimal index", locator: "compatibility:0a:kun-settings.json:provider.apiKey"},
		{name: "compatibility negative index", locator: "compatibility:-1:kun-settings.json:provider.apiKey"},
		{name: "compatibility index out of range", locator: "compatibility:100:kun-settings.json:provider.apiKey"},
		{name: "compatibility missing index", locator: "compatibility::kun-settings.json:provider.apiKey"},
		{name: "unknown compatibility filename", locator: "compatibility:00:settings.json:provider.apiKey"},
		{name: "empty compatibility suffix", locator: "compatibility:00:kun-settings.json:"},
		{name: "extra compatibility segment", locator: "compatibility:00:kun-settings.json:provider.apiKey:extra"},
	}
	for _, testCase := range invalidLocators {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			invalid := cloneLegacyMigrationCandidate(candidate)
			invalid.ActiveCredentialLocators = []string{testCase.locator}
			invalid.RollbackCredentialArtifacts = nil
			_, invalidPayload, err := normalizeLegacyMigrationCandidate(invalid)
			clear(invalidPayload)
			if !errors.Is(err, registryport.ErrInvalidRequest) {
				t.Fatalf("normalizeLegacyMigrationCandidate(%q) error = %v, want invalid request", testCase.locator, err)
			}
			invalidPayload = legacyMigrationRecoveryPayloadFixture(
				t,
				legacyMigrationRecoveryPayloadVersionV2,
				invalid,
			)
			_, err = parseLegacyMigrationRecoveryPayload(invalidPayload)
			clear(invalidPayload)
			if !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("parseLegacyMigrationRecoveryPayload(%q) error = %v, want verification", testCase.locator, err)
			}
		})
	}
}

func TestLegacyMigrationRecoveryPayloadV2IsStrictAndV1FixtureRemainsReadable(t *testing.T) {
	t.Parallel()

	registry := newMemoryRegistryStore()
	v1Candidate := legacyMigrationV1CandidateFor(
		registry.snapshot(),
		"migration-v1-fixture",
		"provider-v1-fixture",
		"source-v1-fixture",
		"synthetic-v1-fixture-not-a-real-key",
	)
	v1Fixture := legacyMigrationRecoveryPayloadFixture(t, legacyMigrationRecoveryPayloadVersion, v1Candidate)
	_, v1Written, err := normalizeLegacyMigrationCandidate(v1Candidate)
	if err != nil {
		t.Fatalf("normalizeLegacyMigrationCandidate(v1) error = %v", err)
	}
	defer clear(v1Written)
	if !bytes.Equal(v1Written, v1Fixture) {
		t.Fatal("legacy candidate without locators or artifacts did not retain exact v1 encoding")
	}
	v1, err := parseLegacyMigrationRecoveryPayload(v1Fixture)
	if err != nil {
		t.Fatalf("parseLegacyMigrationRecoveryPayload(v1 fixture) error = %v", err)
	}
	if v1.version != legacyMigrationRecoveryPayloadVersion ||
		!bytes.Equal(v1.migrationID, []byte(v1Candidate.MigrationID)) ||
		!bytes.Equal(v1.sourceSHA256, []byte(v1Candidate.SourceSHA256)) ||
		!reflect.DeepEqual(v1.provider, v1Candidate.Provider) ||
		!bytes.Equal(v1.credentialPurpose, []byte(v1Candidate.CredentialPurpose)) ||
		!bytes.Equal(v1.credential, v1Candidate.Credential) ||
		len(v1.activeCredentialLocators) != 0 || len(v1.rollbackCredentialArtifacts) != 0 {
		t.Fatalf("parsed v1 fixture = %#v", v1)
	}

	v2Candidate := legacyMigrationV2CandidateWithArtifactsFor(
		registry.snapshot(),
		"migration-v2-strict",
		"provider-v2-strict",
		"source-v2-strict",
	)
	_, v2Payload, err := normalizeLegacyMigrationCandidate(v2Candidate)
	if err != nil {
		t.Fatalf("normalizeLegacyMigrationCandidate(v2) error = %v", err)
	}
	defer clear(v2Payload)
	v2, err := parseLegacyMigrationRecoveryPayload(v2Payload)
	if err != nil {
		t.Fatalf("parseLegacyMigrationRecoveryPayload(v2) error = %v", err)
	}
	if v2.version != legacyMigrationRecoveryPayloadVersionV2 ||
		!equalLegacyMigrationLocatorView(v2.activeCredentialLocators, v2Candidate.ActiveCredentialLocators) ||
		len(v2.rollbackCredentialArtifacts) != len(v2Candidate.RollbackCredentialArtifacts) {
		t.Fatalf("parsed v2 payload = %#v", v2)
	}
	for index, artifact := range v2.rollbackCredentialArtifacts {
		if !equalLegacyMigrationLocatorView(artifact.locators, v2Candidate.RollbackCredentialArtifacts[index].Locators) ||
			!bytes.Equal(artifact.credential, v2Candidate.RollbackCredentialArtifacts[index].Credential) {
			t.Fatalf("parsed v2 artifact %d = %#v", index, artifact)
		}
	}

	activeOnly := cloneLegacyMigrationCandidate(v2Candidate)
	activeOnly.RollbackCredentialArtifacts = []LegacyMigrationRollbackCredentialArtifact{}
	_, activeOnlyPayload, err := normalizeLegacyMigrationCandidate(activeOnly)
	if err != nil {
		t.Fatalf("normalizeLegacyMigrationCandidate(active-only v2) error = %v", err)
	}
	activeOnlyView, parseErr := parseLegacyMigrationRecoveryPayload(activeOnlyPayload)
	clear(activeOnlyPayload)
	if parseErr != nil || activeOnlyView.version != legacyMigrationRecoveryPayloadVersionV2 ||
		len(activeOnlyView.rollbackCredentialArtifacts) != 0 {
		t.Fatalf("active-only v2 parse = %#v error = %v", activeOnlyView, parseErr)
	}

	invalidInputs := []struct {
		name   string
		mutate func(*LegacyMigrationCandidate)
	}{
		{name: "artifacts without active locators", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators = nil
		}},
		{name: "empty active locator", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators[0] = ""
		}},
		{name: "absolute locator", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators[0] = "/private/settings.json"
		}},
		{name: "URL locator", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators[0] = "https://provider.invalid/key"
		}},
		{name: "control locator", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators[0] = "settings.provider\nkey"
		}},
		{name: "overlong locator", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators[0] = strings.Repeat("a", maxLegacyMigrationLocatorBytes+1)
		}},
		{name: "duplicate locator", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[0].Locators[0] = candidate.ActiveCredentialLocators[0]
		}},
		{name: "empty artifact locators", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[0].Locators = nil
		}},
		{name: "empty artifact credential", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[0].Credential = nil
		}},
		{name: "artifact equals active", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[0].Credential = bytes.Clone(candidate.Credential)
		}},
		{name: "artifacts equal each other", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[1].Credential = bytes.Clone(candidate.RollbackCredentialArtifacts[0].Credential)
		}},
		{name: "locator count over limit", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators = make([]string, maxLegacyMigrationLocatorCount+1)
			for index := range candidate.ActiveCredentialLocators {
				candidate.ActiveCredentialLocators[index] = fmt.Sprintf("settings.provider.key%d", index)
			}
		}},
		{name: "locator total bytes over limit", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators = make([]string, maxLegacyMigrationLocatorCount)
			for index := range candidate.ActiveCredentialLocators {
				candidate.ActiveCredentialLocators[index] = strings.Repeat("a", 240) + fmt.Sprintf("%03d", index)
			}
		}},
		{name: "artifact count over limit", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts = make([]LegacyMigrationRollbackCredentialArtifact, maxLegacyMigrationRollbackArtifactCount+1)
			for index := range candidate.RollbackCredentialArtifacts {
				candidate.RollbackCredentialArtifacts[index] = LegacyMigrationRollbackCredentialArtifact{
					Locators:   []string{fmt.Sprintf("settings.shadow.key%d", index)},
					Credential: []byte(fmt.Sprintf("synthetic-shadow-%d-not-a-real-key", index)),
				}
			}
		}},
		{name: "credential total over limit", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.Credential = bytes.Repeat([]byte{'a'}, maxLegacyMigrationRecoveryCredentialTotalBytes/2+1)
			candidate.RollbackCredentialArtifacts[0].Credential = bytes.Repeat(
				[]byte{'b'},
				maxLegacyMigrationRecoveryCredentialTotalBytes/2+1,
			)
		}},
		{name: "serialized payload over K1 limit", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.Credential = bytes.Repeat([]byte{'a'}, maxLegacyMigrationCredentialBytes)
			candidate.RollbackCredentialArtifacts = nil
		}},
	}
	for _, testCase := range invalidInputs {
		testCase := testCase
		t.Run("input/"+testCase.name, func(t *testing.T) {
			candidate := cloneLegacyMigrationCandidate(v2Candidate)
			testCase.mutate(&candidate)
			_, payload, err := normalizeLegacyMigrationCandidate(candidate)
			clear(payload)
			if !errors.Is(err, registryport.ErrInvalidRequest) {
				t.Fatalf("normalizeLegacyMigrationCandidate() error = %v, want invalid request", err)
			}
		})
	}

	invalidPayloads := []struct {
		name    string
		payload func() []byte
	}{
		{name: "unknown version", payload: func() []byte {
			payload := bytes.Clone(v2Payload)
			binary.BigEndian.PutUint32(payload[len(legacyMigrationRecoveryPayloadMagic):], legacyMigrationRecoveryPayloadVersionV2+1)
			return payload
		}},
		{name: "truncated", payload: func() []byte { return bytes.Clone(v2Payload[:len(v2Payload)-1]) }},
		{name: "trailing", payload: func() []byte { return append(bytes.Clone(v2Payload), 0) }},
		{name: "over K1 limit", payload: func() []byte { return make([]byte, maxLegacyMigrationRecoveryPayloadBytes+1) }},
		{name: "zero active locator count", payload: func() []byte {
			payload := bytes.Clone(v2Payload)
			binary.BigEndian.PutUint32(payload[legacyMigrationV2ActiveLocatorCountOffset(t, payload):], 0)
			return payload
		}},
		{name: "over active locator count", payload: func() []byte {
			payload := bytes.Clone(v2Payload)
			binary.BigEndian.PutUint32(
				payload[legacyMigrationV2ActiveLocatorCountOffset(t, payload):],
				uint32(maxLegacyMigrationLocatorCount+1),
			)
			return payload
		}},
		{name: "over global locator count", payload: func() []byte {
			candidate := cloneLegacyMigrationCandidate(v2Candidate)
			candidate.ActiveCredentialLocators = make([]string, maxLegacyMigrationLocatorCount)
			for index := range candidate.ActiveCredentialLocators {
				candidate.ActiveCredentialLocators[index] = fmt.Sprintf("settings.active.key%d", index)
			}
			return legacyMigrationRecoveryPayloadFixture(t, legacyMigrationRecoveryPayloadVersionV2, candidate)
		}},
		{name: "ambiguous duplicate locator", payload: func() []byte {
			candidate := cloneLegacyMigrationCandidate(v2Candidate)
			candidate.RollbackCredentialArtifacts[0].Locators[0] = candidate.ActiveCredentialLocators[0]
			return legacyMigrationRecoveryPayloadFixture(t, legacyMigrationRecoveryPayloadVersionV2, candidate)
		}},
		{name: "ambiguous duplicate credential", payload: func() []byte {
			candidate := cloneLegacyMigrationCandidate(v2Candidate)
			candidate.RollbackCredentialArtifacts[0].Credential = bytes.Clone(candidate.Credential)
			return legacyMigrationRecoveryPayloadFixture(t, legacyMigrationRecoveryPayloadVersionV2, candidate)
		}},
		{name: "artifact without locator", payload: func() []byte {
			candidate := cloneLegacyMigrationCandidate(v2Candidate)
			candidate.RollbackCredentialArtifacts[0].Locators = nil
			return legacyMigrationRecoveryPayloadFixture(t, legacyMigrationRecoveryPayloadVersionV2, candidate)
		}},
		{name: "artifact without credential", payload: func() []byte {
			candidate := cloneLegacyMigrationCandidate(v2Candidate)
			candidate.RollbackCredentialArtifacts[0].Credential = nil
			return legacyMigrationRecoveryPayloadFixture(t, legacyMigrationRecoveryPayloadVersionV2, candidate)
		}},
	}
	for _, testCase := range invalidPayloads {
		testCase := testCase
		t.Run("parser/"+testCase.name, func(t *testing.T) {
			payload := testCase.payload()
			defer clear(payload)
			if _, err := parseLegacyMigrationRecoveryPayload(payload); !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("parseLegacyMigrationRecoveryPayload() error = %v, want verification", err)
			}
		})
	}
}

func TestLegacyMigrationPayloadAppendFailureRetainsBufferForZeroization(t *testing.T) {
	t.Parallel()

	payload := []byte("synthetic-manager-owned-payload-not-a-real-key")
	oversized := make([]byte, maxLegacyMigrationRecoveryPayloadBytes+1)
	defer clear(oversized)
	retained, err := appendLegacyMigrationPayloadField(payload, oversized)
	if !errors.Is(err, registryport.ErrInvalidRequest) {
		t.Fatalf("appendLegacyMigrationPayloadField() error = %v, want invalid request", err)
	}
	clear(retained)
	if !allZero(payload) {
		t.Fatal("append failure discarded the Manager-owned payload reference before zeroization")
	}
}

func TestLegacyMigrationRecoveryV4ReplayComparesFullProtectedPayloadWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithArtifactsFor(
		registry.snapshot(),
		"migration-v4-replay",
		"provider-v4-replay",
		"source-v4-replay",
	)
	result, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	before := registry.snapshot()
	recovery := before.LegacyMigrationRecoveries[candidate.MigrationID]
	secrets.mu.Lock()
	protectedBefore := bytes.Clone(secrets.records[recovery.RecoveryCredentialRef].secret)
	secrets.mu.Unlock()
	defer clear(protectedBefore)

	replayed, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil || replayed != result || secrets.preparedCount() != 1 ||
		!reflect.DeepEqual(before, registry.snapshot()) {
		t.Fatalf("exact v4 replay result = %#v error = %v prepared = %d", replayed, err, secrets.preparedCount())
	}
	mutations := []struct {
		name   string
		mutate func(*LegacyMigrationCandidate)
	}{
		{name: "source", mutate: func(value *LegacyMigrationCandidate) {
			value.SourceSnapshot = []byte(`{"provider":{"apiKey":"synthetic-v4-different-source"}}`)
			digest := sha256.Sum256(value.SourceSnapshot)
			value.SourceSHA256 = hex.EncodeToString(digest[:])
		}},
		{name: "provider", mutate: func(value *LegacyMigrationCandidate) { value.Provider.SelectedRoutes = []string{"secondary"} }},
		{name: "purpose", mutate: func(value *LegacyMigrationCandidate) { value.CredentialPurpose = "provider-bearer-token" }},
		{name: "active credential", mutate: func(value *LegacyMigrationCandidate) { value.Credential[0] ^= 1 }},
		{name: "active locator value", mutate: func(value *LegacyMigrationCandidate) {
			value.ActiveCredentialLocators[0] = "current:analytix-settings.json:agents.reasonix.apiKey"
		}},
		{name: "active locator order", mutate: func(value *LegacyMigrationCandidate) {
			value.ActiveCredentialLocators[0], value.ActiveCredentialLocators[1] = value.ActiveCredentialLocators[1], value.ActiveCredentialLocators[0]
		}},
		{name: "artifact locator value", mutate: func(value *LegacyMigrationCandidate) {
			value.RollbackCredentialArtifacts[0].Locators[0] = "current:analytix-settings.json:agents.codewhale.apiKey"
		}},
		{name: "artifact locator order", mutate: func(value *LegacyMigrationCandidate) {
			locators := value.RollbackCredentialArtifacts[0].Locators
			locators[0], locators[1] = locators[1], locators[0]
		}},
		{name: "artifact credential", mutate: func(value *LegacyMigrationCandidate) { value.RollbackCredentialArtifacts[0].Credential[0] ^= 1 }},
		{name: "artifact order", mutate: func(value *LegacyMigrationCandidate) {
			value.RollbackCredentialArtifacts[0], value.RollbackCredentialArtifacts[1] =
				value.RollbackCredentialArtifacts[1], value.RollbackCredentialArtifacts[0]
		}},
	}
	for _, testCase := range mutations {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			changed := cloneLegacyMigrationCandidate(candidate)
			testCase.mutate(&changed)
			_, err := manager.PrepareLegacyMigrationRecovery(ctx, changed)
			if !errors.Is(err, registryport.ErrConflict) || err.Error() != registryport.ErrConflict.Error() {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want stable redacted conflict", err)
			}
			secrets.mu.Lock()
			protectedAfter := bytes.Clone(secrets.records[recovery.RecoveryCredentialRef].secret)
			secrets.mu.Unlock()
			if !reflect.DeepEqual(before, registry.snapshot()) || secrets.preparedCount() != 1 ||
				!bytes.Equal(protectedBefore, protectedAfter) {
				clear(protectedAfter)
				t.Fatal("inexact v4 replay replaced or mutated protected recovery authority")
			}
			clear(protectedAfter)
		})
	}
}

func TestLegacyMigrationRecoveryV2RegistryMetadataCannotVerifyCredentialOrArtifactGuesses(t *testing.T) {
	t.Parallel()

	variants := []struct {
		active  string
		shadowA string
		shadowB string
	}{
		{
			active:  "synthetic-active-guess-alpha-not-a-real-key",
			shadowA: "synthetic-shadow-guess-alpha-a-not-a-real-key",
			shadowB: "synthetic-shadow-guess-alpha-b-not-a-real-key",
		},
		{
			active:  "synthetic-active-guess-bravo-not-a-real-key",
			shadowA: "synthetic-shadow-guess-bravo-a-not-a-real-key",
			shadowB: "synthetic-shadow-guess-bravo-b-not-a-real-key",
		},
	}
	registryBytes := make([][]byte, 0, len(variants))
	for _, variant := range variants {
		registry := newMemoryRegistryStore()
		candidate := legacyMigrationCandidateWithArtifactsFor(
			registry.snapshot(),
			"migration-v2-guess",
			"provider-v2-guess",
			"source-v2-guess",
		)
		candidate.Credential = []byte(variant.active)
		candidate.RollbackCredentialArtifacts[0].Credential = []byte(variant.shadowA)
		candidate.RollbackCredentialArtifacts[1].Credential = []byte(variant.shadowB)
		if _, err := mustManager(t, registry, newMemorySecretStore(), nil).PrepareLegacyMigrationRecovery(
			context.Background(),
			candidate,
		); err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
		}
		serialized, err := domainregistry.Marshal(registry.snapshot())
		if err != nil {
			t.Fatalf("Marshal(Registry) error = %v", err)
		}
		for _, protected := range [][]byte{
			candidate.Credential,
			candidate.RollbackCredentialArtifacts[0].Credential,
			candidate.RollbackCredentialArtifacts[1].Credential,
		} {
			if bytes.Contains(serialized, protected) {
				t.Fatalf("Registry contains guessable protected value of length %d", len(protected))
			}
		}
		registryBytes = append(registryBytes, serialized)
	}
	if len(registryBytes) != 2 || !bytes.Equal(registryBytes[0], registryBytes[1]) {
		t.Fatal("Registry metadata changes solely with active or artifact credential bytes")
	}
}

func TestLegacyMigrationRecoveryV2CommitUsesOnlyActiveCredentialAndRetainsArtifacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithArtifactsFor(
		registry.snapshot(),
		"migration-v2-commit",
		"provider-v2-commit",
		"source-v2-commit",
	)
	recoveryResult, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recoveryResult)
	beforeCommit := registry.snapshot()
	recoveryBefore := beforeCommit.LegacyMigrationRecoveries[candidate.MigrationID]
	secrets.mu.Lock()
	protectedBefore := bytes.Clone(secrets.records[recoveryBefore.RecoveryCredentialRef].secret)
	secrets.mu.Unlock()
	defer clear(protectedBefore)

	result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	state := registry.snapshot()
	winner := state.Providers[candidate.Provider.ID]
	recovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
	secrets.mu.Lock()
	winnerPlaintext := bytes.Clone(secrets.records[winner.CredentialRef].secret)
	retainedPlaintext := bytes.Clone(secrets.records[recovery.RecoveryCredentialRef].secret)
	secrets.mu.Unlock()
	defer clear(winnerPlaintext)
	defer clear(retainedPlaintext)
	retained, parseErr := parseLegacyMigrationRecoveryPayload(retainedPlaintext)
	if parseErr != nil {
		t.Fatalf("parse retained v2 recovery error = %v", parseErr)
	}
	if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
		!result.SafeToRemoveLegacyPlaintext ||
		recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		winner.CredentialRef == recovery.RecoveryCredentialRef ||
		recovery.CommittedProviderCredentialRef != winner.CredentialRef ||
		!bytes.Equal(winnerPlaintext, candidate.Credential) ||
		!bytes.Equal(retainedPlaintext, protectedBefore) ||
		len(retained.rollbackCredentialArtifacts) != len(candidate.RollbackCredentialArtifacts) ||
		secrets.recordCount() != 2 {
		t.Fatalf("committed retained v2 state = %#v", state)
	}
	for _, artifact := range candidate.RollbackCredentialArtifacts {
		if bytes.Equal(winnerPlaintext, artifact.Credential) || !bytes.Contains(retainedPlaintext, artifact.Credential) {
			t.Fatal("rollback artifact became Provider winner or was lost from retained recovery")
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
			t.Fatalf("Recover(%d) error = %v", attempt, err)
		}
	}
	repeated, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command)
	if err != nil || !reflect.DeepEqual(repeated, result) {
		t.Fatalf("repeated CommitVerifiedLegacyMigrationRecovery() result = %#v error = %v", repeated, err)
	}
	if err := manager.AbandonLegacyMigrationRecovery(ctx, AbandonLegacyMigrationRecoveryCommand{
		MigrationID: candidate.MigrationID, SourceSHA256: candidate.SourceSHA256,
		RecoveryCredentialRef: recoveryResult.RecoveryCredentialRef,
		Confirmation:          LegacyMigrationAbandonConfirmation,
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("AbandonLegacyMigrationRecovery(committed retained) error = %v, want conflict", err)
	}
	after := registry.snapshot()
	_, tombstones, deletes := secrets.callCounts()
	secrets.mu.Lock()
	retainedAfter := bytes.Clone(secrets.records[recovery.RecoveryCredentialRef].secret)
	secrets.mu.Unlock()
	defer clear(retainedAfter)
	if !reflect.DeepEqual(state, after) || !bytes.Equal(protectedBefore, retainedAfter) ||
		!secrets.hasActive(winner.CredentialRef) || !secrets.hasActive(recovery.RecoveryCredentialRef) ||
		tombstones != 0 || deletes != 0 {
		t.Fatalf("repeated recovery changed retained artifacts: tombstones=%d deletes=%d", tombstones, deletes)
	}

	registryBytes, marshalErr := domainregistry.Marshal(after)
	if marshalErr != nil {
		t.Fatalf("Marshal(Registry) error = %v", marshalErr)
	}
	protectedValues := [][]byte{candidate.Credential}
	for _, locator := range candidate.ActiveCredentialLocators {
		protectedValues = append(protectedValues, []byte(locator))
	}
	for _, artifact := range candidate.RollbackCredentialArtifacts {
		protectedValues = append(protectedValues, artifact.Credential)
		for _, locator := range artifact.Locators {
			protectedValues = append(protectedValues, []byte(locator))
		}
	}
	for _, protected := range protectedValues {
		if bytes.Contains(registryBytes, protected) {
			t.Fatalf("Registry contains protected v2 value of length %d", len(protected))
		}
	}
}

func TestManagerCommitsVerifiedLegacyMigrationRecoveryBeforeSourceCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(),
		"migration-commit-alpha",
		"provider-migration-alpha",
		"source-commit-alpha",
		"synthetic-commit-canary-not-a-real-key",
	)
	recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}

	result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, CommitVerifiedLegacyMigrationRecoveryCommand{
		Expected:                   expectedFor(registry.snapshot(), candidate.Provider.ID),
		ExpectedSelectedProviderID: registry.snapshot().SelectedProviderID,
		MigrationID:                candidate.MigrationID,
		SourceLocator:              candidate.SourceLocator,
		SourceSHA256:               candidate.SourceSHA256,
		RecoveryCredentialRef:      recovery.RecoveryCredentialRef,
		Confirmation:               LegacyMigrationCommitConfirmation,
	})
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
		!result.SafeToRemoveLegacyPlaintext {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() result = %#v", result)
	}

	state := registry.snapshot()
	winner, exists := state.Providers[candidate.Provider.ID]
	committedRecovery, recoveryExists := state.LegacyMigrationRecoveries[candidate.MigrationID]
	if !exists || !recoveryExists ||
		committedRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		committedRecovery.CommittedProviderCredentialRef != winner.CredentialRef ||
		winner.CredentialRef == string(recovery.RecoveryCredentialRef) ||
		!secrets.hasActive(winner.CredentialRef) ||
		!secrets.hasActive(string(recovery.RecoveryCredentialRef)) {
		t.Fatalf("committed retained migration state = %#v", state)
	}
}

func TestManagerCommitsMultipleVerifiedLegacyMigrationRecoveriesBeforeSharedSourceCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	sharedSourceSnapshot := []byte(`{"provider":{"providers":[{"id":"provider-shared-source-alpha","apiKey":"synthetic-alpha"},{"id":"provider-shared-source-beta","apiKey":"synthetic-beta"},{"id":"provider-shared-source-gamma","apiKey":"synthetic-gamma"}]}}`)
	sharedSourceDigest := sha256.Sum256(sharedSourceSnapshot)
	sharedSourceSHA256 := hex.EncodeToString(sharedSourceDigest[:])
	sharedCleanedDigest := sha256.Sum256([]byte(`{"provider":{"providers":[{"id":"provider-shared-source-alpha"},{"id":"provider-shared-source-beta"},{"id":"provider-shared-source-gamma"}]}}`))
	sharedExpectedCleanedSourceSHA256 := hex.EncodeToString(sharedCleanedDigest[:])
	selectedProviderID := ""
	credentialRefs := make(map[string]string)
	recoveryRefs := make(map[string]string)
	for index, migration := range []struct {
		migrationID string
		providerID  string
		credential  string
	}{
		{migrationID: "migration-shared-source-alpha", providerID: "provider-shared-source-alpha", credential: "synthetic-shared-source-alpha-not-a-real-key"},
		{migrationID: "migration-shared-source-beta", providerID: "provider-shared-source-beta", credential: "synthetic-shared-source-beta-not-a-real-key"},
		{migrationID: "migration-shared-source-gamma", providerID: "provider-shared-source-gamma", credential: "synthetic-shared-source-gamma-not-a-real-key"},
	} {
		before := registry.snapshot()
		candidate := legacyMigrationCandidateFor(
			before,
			migration.migrationID,
			migration.providerID,
			"shared-settings-source",
			migration.credential,
		)
		candidate.SourceSnapshot = bytes.Clone(sharedSourceSnapshot)
		candidate.SourceSHA256 = sharedSourceSHA256
		candidate.ExpectedCleanedSourceSHA256 = sharedExpectedCleanedSourceSHA256
		candidate.ActiveCredentialLocators = []string{
			fmt.Sprintf("current:analytix-settings.json:provider.providers[%d].apiKey", index),
		}
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery(%d) error = %v", index+1, err)
		}

		result, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx,
			legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
		)
		if err != nil {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery(%d) error = %v", index+1, err)
		}
		if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			!result.SafeToRemoveLegacyPlaintext {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery(%d) result = %#v", index+1, result)
		}

		state := registry.snapshot()
		winner := state.Providers[candidate.Provider.ID]
		committedRecovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
		if index == 0 {
			selectedProviderID = candidate.Provider.ID
		}
		if state.SelectedProviderID != selectedProviderID ||
			winner.CredentialRef == "" || winner.CredentialRef == committedRecovery.RecoveryCredentialRef ||
			committedRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
			committedRecovery.CommittedProviderCredentialRef != winner.CredentialRef ||
			!secrets.hasActive(winner.CredentialRef) || !secrets.hasActive(committedRecovery.RecoveryCredentialRef) {
			t.Fatalf("committed retained migration %d state = %#v", index+1, state)
		}
		credentialRefs[candidate.Provider.ID] = winner.CredentialRef
		recoveryRefs[candidate.Provider.ID] = committedRecovery.RecoveryCredentialRef
		for providerID, credentialRef := range credentialRefs {
			if providerID != candidate.Provider.ID && credentialRef == winner.CredentialRef {
				t.Fatalf("provider winner refs alias: %q", winner.CredentialRef)
			}
			if credentialRef == committedRecovery.RecoveryCredentialRef {
				t.Fatalf("provider winner ref aliases recovery ref: %q", credentialRef)
			}
		}
		for providerID, recoveryRef := range recoveryRefs {
			if providerID != candidate.Provider.ID && recoveryRef == committedRecovery.RecoveryCredentialRef {
				t.Fatalf("migration recovery refs alias: %q", recoveryRef)
			}
			if recoveryRef == winner.CredentialRef {
				t.Fatalf("migration recovery ref aliases winner ref: %q", recoveryRef)
			}
		}
	}

	state := registry.snapshot()
	if state.Revision != 3 || len(state.Providers) != 3 || len(state.Transactions) != 0 ||
		len(state.LegacyMigrationRecoveries) != 3 || state.SelectedProviderID != selectedProviderID ||
		secrets.preparedCount() != 6 || secrets.recordCount() != 6 {
		t.Fatalf("multiple committed-retained migrations = %#v prepared=%d records=%d",
			state, secrets.preparedCount(), secrets.recordCount())
	}
}

func TestManagerReplaysCommittedRetainedLegacyMigrationRecoveryWithoutSourceCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithArtifactsFor(
		registry.snapshot(),
		"migration-committed-retained-prepare-replay",
		"provider-committed-retained-prepare-replay",
		"source-committed-retained-prepare-replay",
	)
	recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx,
		legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	if committed.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
		!committed.SafeToRemoveLegacyPlaintext {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() result = %#v", committed)
	}

	before := registry.snapshot()
	beforeBytes, err := domainregistry.Marshal(before)
	if err != nil {
		t.Fatalf("Marshal(before replay) error = %v", err)
	}
	beforeReads, beforeTombstones, beforeDeletes := secrets.callCounts()
	beforePrepared := secrets.preparedCount()
	beforeRecords := secrets.recordCount()
	replay := cloneLegacyMigrationCandidate(candidate)
	replay.Expected = expectedFor(before, replay.Provider.ID)

	for name, replayManager := range map[string]*Manager{
		"same-process":  manager,
		"fresh-manager": mustManager(t, registry, secrets, nil),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := replayManager.PrepareLegacyMigrationRecovery(ctx, cloneLegacyMigrationCandidate(replay))
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(replay) error = %v", err)
			}
			if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
				result.MigrationID != committed.MigrationID ||
				result.RecoveryCredentialRef != committed.RecoveryCredentialRef ||
				result.SafeToProceedWithProviderMigration {
				t.Fatalf("PrepareLegacyMigrationRecovery(replay) result = %#v", result)
			}
		})
	}

	after := registry.snapshot()
	afterBytes, err := domainregistry.Marshal(after)
	if err != nil {
		t.Fatalf("Marshal(after replay) error = %v", err)
	}
	afterReads, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(before, after) || !bytes.Equal(beforeBytes, afterBytes) ||
		beforePrepared != secrets.preparedCount() || beforeRecords != secrets.recordCount() ||
		beforeTombstones != afterTombstones || beforeDeletes != afterDeletes ||
		afterReads != beforeReads+4 {
		t.Fatalf("committed-retained Prepare replay changed authority: state=%#v reads=%d->%d tombstones=%d->%d deletes=%d->%d prepared=%d->%d records=%d->%d",
			after, beforeReads, afterReads, beforeTombstones, afterTombstones, beforeDeletes, afterDeletes,
			beforePrepared, secrets.preparedCount(), beforeRecords, secrets.recordCount())
	}
}

func TestManagerRejectsDivergentCommittedRetainedLegacyMigrationPrepareReplayWithoutMutation(t *testing.T) {
	t.Parallel()

	type fixture struct {
		ctx       context.Context
		registry  *memoryRegistryStore
		secrets   *memorySecretStore
		manager   *Manager
		candidate LegacyMigrationCandidate
		committed CommitVerifiedLegacyMigrationRecoveryResult
	}
	newFixture := func(t *testing.T) fixture {
		t.Helper()
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateWithArtifactsFor(
			registry.snapshot(),
			"migration-committed-retained-divergence",
			"provider-committed-retained-divergence",
			"source-committed-retained-divergence",
		)
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
		}
		committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx,
			legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
		)
		if err != nil {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
		}
		candidate.Expected = expectedFor(registry.snapshot(), candidate.Provider.ID)
		return fixture{
			ctx: ctx, registry: registry, secrets: secrets, manager: manager,
			candidate: candidate, committed: committed,
		}
	}
	assertRejectedWithoutMutation := func(t *testing.T, current fixture, replay LegacyMigrationCandidate) {
		t.Helper()
		before := current.registry.snapshot()
		beforeCommits := current.registry.commits
		beforePrepared := current.secrets.preparedCount()
		beforeRecords := current.secrets.recordCount()
		_, beforeTombstones, beforeDeletes := current.secrets.callCounts()
		if _, err := current.manager.PrepareLegacyMigrationRecovery(current.ctx, replay); err == nil {
			t.Fatal("PrepareLegacyMigrationRecovery(divergent replay) error = nil")
		}
		after := current.registry.snapshot()
		_, afterTombstones, afterDeletes := current.secrets.callCounts()
		if !reflect.DeepEqual(before, after) || current.registry.commits != beforeCommits ||
			current.secrets.preparedCount() != beforePrepared || current.secrets.recordCount() != beforeRecords ||
			afterTombstones != beforeTombstones || afterDeletes != beforeDeletes {
			t.Fatalf("divergent replay changed authority: before=%#v after=%#v commits=%d->%d prepared=%d->%d records=%d->%d tombstones=%d->%d deletes=%d->%d",
				before, after, beforeCommits, current.registry.commits,
				beforePrepared, current.secrets.preparedCount(), beforeRecords, current.secrets.recordCount(),
				beforeTombstones, afterTombstones, beforeDeletes, afterDeletes)
		}
	}
	mutateRegistryUnchecked := func(current fixture, mutate func(*domainregistry.Registry)) {
		current.registry.stateMu.Lock()
		defer current.registry.stateMu.Unlock()
		state := current.registry.state.Clone()
		mutate(&state)
		current.registry.state = state
	}

	for _, testCase := range []struct {
		name   string
		mutate func(*LegacyMigrationCandidate)
	}{
		{name: "source", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.SourceSHA256 = strings.Repeat("b", 64)
		}},
		{name: "secret", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.Credential = []byte("synthetic-divergent-active-not-a-real-key")
		}},
		{name: "locator order", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators[0], candidate.ActiveCredentialLocators[1] =
				candidate.ActiveCredentialLocators[1], candidate.ActiveCredentialLocators[0]
		}},
		{name: "locator value", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.ActiveCredentialLocators[0] = "current:analytix-settings.json:runtime.apiKey"
		}},
		{name: "artifact order", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[0], candidate.RollbackCredentialArtifacts[1] =
				candidate.RollbackCredentialArtifacts[1], candidate.RollbackCredentialArtifacts[0]
		}},
		{name: "artifact locator order", mutate: func(candidate *LegacyMigrationCandidate) {
			artifact := &candidate.RollbackCredentialArtifacts[0]
			artifact.Locators[0], artifact.Locators[1] = artifact.Locators[1], artifact.Locators[0]
		}},
		{name: "artifact value", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[0].Credential =
				[]byte("synthetic-divergent-shadow-not-a-real-key")
		}},
		{name: "Provider", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.Provider.Endpoint = "https://other-provider.invalid/v1"
		}},
		{name: "purpose", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.CredentialPurpose = "provider-oauth-token"
		}},
		{name: "stale Registry CAS", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.Expected.RegistryRevision--
		}},
		{name: "stale Provider CAS", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.Expected.ProviderGeneration++
		}},
		{name: "locator alias", mutate: func(candidate *LegacyMigrationCandidate) {
			candidate.RollbackCredentialArtifacts[0].Locators[0] = candidate.ActiveCredentialLocators[0]
		}},
	} {
		testCase := testCase
		t.Run("candidate/"+testCase.name, func(t *testing.T) {
			current := newFixture(t)
			replay := cloneLegacyMigrationCandidate(current.candidate)
			testCase.mutate(&replay)
			assertRejectedWithoutMutation(t, current, replay)
		})
	}

	for _, testCase := range []struct {
		name   string
		mutate func(fixture)
	}{
		{name: "selection", mutate: func(current fixture) {
			prepared, err := current.secrets.PreparePut(current.ctx, "provider-api-key", []byte("synthetic-other-winner"))
			if err != nil {
				panic(err)
			}
			if err := prepared.Commit(current.ctx); err != nil {
				panic(err)
			}
			mutateRegistryUnchecked(current, func(state *domainregistry.Registry) {
				state.Revision++
				state.Providers["provider-other-selection"] = domainregistry.ProviderInput{
					ID: "provider-other-selection", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
					Models: []string{"model-alpha"}, SelectedModel: "model-alpha", SelectedRoutes: []string{"primary"},
				}.Provider("inc_"+strings.Repeat("S", 43), string(prepared.CredentialRef()), "provider-api-key", 1, 1)
				state.SelectedProviderID = "provider-other-selection"
			})
		}},
		{name: "winner Provider", mutate: func(current fixture) {
			mutateRegistryUnchecked(current, func(state *domainregistry.Registry) {
				provider := state.Providers[current.candidate.Provider.ID]
				provider.Endpoint = "https://divergent-winner.invalid/v1"
				state.Providers[provider.ID] = provider
			})
		}},
		{name: "winner purpose", mutate: func(current fixture) {
			mutateRegistryUnchecked(current, func(state *domainregistry.Registry) {
				provider := state.Providers[current.candidate.Provider.ID]
				provider.CredentialPurpose = "provider-oauth-token"
				state.Providers[provider.ID] = provider
			})
		}},
		{name: "winner revision", mutate: func(current fixture) {
			mutateRegistryUnchecked(current, func(state *domainregistry.Registry) {
				provider := state.Providers[current.candidate.Provider.ID]
				provider.Revision++
				state.Providers[provider.ID] = provider
			})
		}},
		{name: "winner generation", mutate: func(current fixture) {
			mutateRegistryUnchecked(current, func(state *domainregistry.Registry) {
				provider := state.Providers[current.candidate.Provider.ID]
				provider.Generation++
				state.Providers[provider.ID] = provider
			})
		}},
		{name: "winner incarnation", mutate: func(current fixture) {
			mutateRegistryUnchecked(current, func(state *domainregistry.Registry) {
				provider := state.Providers[current.candidate.Provider.ID]
				provider.Incarnation = "inc_" + strings.Repeat("I", 43)
				state.Providers[provider.ID] = provider
			})
		}},
		{name: "missing recovery payload", mutate: func(current fixture) {
			current.secrets.remove(string(current.committed.RecoveryCredentialRef))
		}},
		{name: "tampered recovery payload", mutate: func(current fixture) {
			current.secrets.tamper(string(current.committed.RecoveryCredentialRef))
		}},
		{name: "tombstoned recovery payload", mutate: func(current fixture) {
			current.secrets.mu.Lock()
			record := current.secrets.records[string(current.committed.RecoveryCredentialRef)]
			record.tombstoned = true
			current.secrets.records[string(current.committed.RecoveryCredentialRef)] = record
			current.secrets.mu.Unlock()
		}},
		{name: "unauthorized recovery read", mutate: func(current fixture) {
			current.secrets.mu.Lock()
			current.secrets.denyRead = true
			current.secrets.mu.Unlock()
		}},
		{name: "missing winner", mutate: func(current fixture) {
			current.secrets.remove(current.registry.snapshot().Providers[current.candidate.Provider.ID].CredentialRef)
		}},
		{name: "tampered winner", mutate: func(current fixture) {
			current.secrets.tamper(current.registry.snapshot().Providers[current.candidate.Provider.ID].CredentialRef)
		}},
		{name: "divergent recovery metadata", mutate: func(current fixture) {
			mutateRegistryUnchecked(current, func(state *domainregistry.Registry) {
				recovery := state.LegacyMigrationRecoveries[current.candidate.MigrationID]
				recovery.CommittedProviderGeneration++
				state.LegacyMigrationRecoveries[recovery.ID] = recovery
			})
		}},
	} {
		testCase := testCase
		t.Run("authority/"+testCase.name, func(t *testing.T) {
			current := newFixture(t)
			testCase.mutate(current)
			replay := cloneLegacyMigrationCandidate(current.candidate)
			replay.Expected = expectedFor(current.registry.snapshot(), replay.Provider.ID)
			assertRejectedWithoutMutation(t, current, replay)
		})
	}

	t.Run("authority/credential reference alias", func(t *testing.T) {
		current := newFixture(t)
		state := current.registry.snapshot()
		provider := state.Providers[current.candidate.Provider.ID]
		recovery := state.LegacyMigrationRecoveries[current.candidate.MigrationID]
		provider.CredentialRef = recovery.RecoveryCredentialRef
		recovery.CommittedProviderCredentialRef = recovery.RecoveryCredentialRef
		state.Providers[provider.ID] = provider
		state.LegacyMigrationRecoveries[recovery.ID] = recovery
		mutateRegistryUnchecked(current, func(currentState *domainregistry.Registry) {
			*currentState = state
		})
		replay := cloneLegacyMigrationCandidate(current.candidate)
		replay.Expected = expectedFor(state, replay.Provider.ID)
		assertRejectedWithoutMutation(t, current, replay)
	})
}

func TestCompetingLegacyMigrationPrepareCannotInterleaveWithProviderCommitIntent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidateA := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-commit-intent-a", "provider-commit-intent-a",
		"source-commit-intent-a", "synthetic-commit-intent-a-not-a-real-key",
	)
	recoveryA, err := manager.PrepareLegacyMigrationRecovery(ctx, candidateA)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery(A) error = %v", err)
	}
	commandA := legacyMigrationCommitCommandFor(registry.snapshot(), candidateA, recoveryA)
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultBeforeMigrationProviderTransactionPrepared))
	if _, err := interrupted.CommitVerifiedLegacyMigrationRecovery(ctx, commandA); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery(A) error = %v, want interrupted", err)
	}
	replayedA, err := manager.PrepareLegacyMigrationRecovery(ctx, cloneLegacyMigrationCandidate(candidateA))
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery(A replay) error = %v", err)
	}
	if replayedA.Status != LegacyMigrationRecoveryStatusVerified ||
		replayedA.RecoveryCredentialRef != recoveryA.RecoveryCredentialRef {
		t.Fatalf("PrepareLegacyMigrationRecovery(A replay) result = %#v", replayedA)
	}

	beforeState := registry.snapshot()
	beforeBytes, err := domainregistry.Marshal(beforeState)
	if err != nil {
		t.Fatalf("Marshal(before competing prepare) error = %v", err)
	}
	beforeReads, beforeTombstones, beforeDeletes := secrets.callCounts()
	beforePrepared := secrets.preparedCount()
	beforeRecords := secrets.recordCount()
	candidateB := legacyMigrationCandidateFor(
		beforeState, "migration-commit-intent-b", "provider-commit-intent-b",
		"source-commit-intent-b", "synthetic-commit-intent-b-not-a-real-key",
	)
	if _, err := manager.PrepareLegacyMigrationRecovery(ctx, candidateB); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("PrepareLegacyMigrationRecovery(B) error = %v, want conflict", err)
	}

	afterState := registry.snapshot()
	afterBytes, err := domainregistry.Marshal(afterState)
	if err != nil {
		t.Fatalf("Marshal(after competing prepare) error = %v", err)
	}
	afterReads, afterTombstones, afterDeletes := secrets.callCounts()
	storedA, exists := afterState.LegacyMigrationRecoveries[candidateA.MigrationID]
	if !bytes.Equal(beforeBytes, afterBytes) || !reflect.DeepEqual(beforeState, afterState) ||
		len(afterState.LegacyMigrationRecoveries) != 1 || !exists ||
		storedA.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
		!secrets.hasActive(storedA.RecoveryCredentialRef) ||
		afterReads != beforeReads || afterTombstones != beforeTombstones || afterDeletes != beforeDeletes ||
		secrets.preparedCount() != beforePrepared || secrets.recordCount() != beforeRecords {
		t.Fatalf("competing prepare changed protected state: before=%#v after=%#v calls=%d/%d/%d->%d/%d/%d prepared=%d->%d records=%d->%d",
			beforeState, afterState,
			beforeReads, beforeTombstones, beforeDeletes,
			afterReads, afterTombstones, afterDeletes,
			beforePrepared, secrets.preparedCount(), beforeRecords, secrets.recordCount())
	}

	for attempt := 0; attempt < 2; attempt++ {
		beforeRecover := registry.snapshot()
		if err := manager.Recover(ctx); err != nil {
			t.Fatalf("Recover(%d) error = %v", attempt+1, err)
		}
		if !reflect.DeepEqual(beforeRecover, registry.snapshot()) {
			t.Fatalf("Recover(%d) changed provider-commit intent", attempt+1)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, commandA)
		if err != nil {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery(A retry %d) error = %v", attempt+1, err)
		}
		if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			!result.SafeToRemoveLegacyPlaintext {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery(A retry %d) result = %#v", attempt+1, result)
		}
	}
	finalState := registry.snapshot()
	winner, winnerExists := finalState.Providers[candidateA.Provider.ID]
	committedA := finalState.LegacyMigrationRecoveries[candidateA.MigrationID]
	if !winnerExists || len(finalState.Providers) != 1 || len(finalState.Transactions) != 0 ||
		len(finalState.LegacyMigrationRecoveries) != 1 ||
		committedA.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		committedA.CommittedProviderCredentialRef != winner.CredentialRef ||
		winner.CredentialRef == committedA.RecoveryCredentialRef ||
		!secrets.hasActive(winner.CredentialRef) || !secrets.hasActive(committedA.RecoveryCredentialRef) ||
		secrets.preparedCount() != 2 || secrets.recordCount() != 2 {
		t.Fatalf("final committed-retained state = %#v prepared=%d records=%d",
			finalState, secrets.preparedCount(), secrets.recordCount())
	}
}

func TestVerifiedLegacyMigrationProviderCommitCrashPointsReconcileSameProcessAndRestart(t *testing.T) {
	t.Parallel()

	points := []FaultPoint{
		FaultBeforeMigrationProviderTransactionPrepared,
		FaultAfterTransactionPrepared,
		FaultAfterCandidateDurableRecorded,
		FaultAfterMetadataCommitted,
		FaultAfterReadbackVerified,
		FaultBeforeMigrationRecoveryCommittedRetainedRecord,
		FaultAfterMigrationRecoveryCommittedRetainedRecorded,
	}
	for _, point := range points {
		point := point
		for _, restart := range []bool{false, true} {
			restart := restart
			name := "same-process/" + string(point)
			if restart {
				name = "fresh-manager/" + string(point)
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				ctx := context.Background()
				registry := newMemoryRegistryStore()
				secrets := newMemorySecretStore()
				base := mustManager(t, registry, secrets, nil)
				candidate := legacyMigrationCandidateWithArtifactsFor(
					registry.snapshot(), "migration-commit-crash", "provider-commit-crash",
					"source-commit-crash",
				)
				recovery, err := base.PrepareLegacyMigrationRecovery(ctx, candidate)
				if err != nil {
					t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
				}
				command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery)
				interrupted := mustManager(t, registry, secrets, faultOnce(point))
				if _, err := interrupted.CommitVerifiedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
					t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v, want interrupted", err)
				}
				interruptedState := registry.snapshot()
				storedRecovery, exists := interruptedState.LegacyMigrationRecoveries[candidate.MigrationID]
				if !exists || !secrets.hasActive(storedRecovery.RecoveryCredentialRef) {
					t.Fatalf("interrupted commit lost protected recovery: %#v", interruptedState)
				}

				reconciler := interrupted
				if restart {
					reconciler = mustManager(t, registry, secrets, nil)
				}
				if err := reconciler.Recover(ctx); err != nil {
					t.Fatalf("Recover() error = %v", err)
				}
				result, err := reconciler.CommitVerifiedLegacyMigrationRecovery(ctx, command)
				if err != nil {
					t.Fatalf("repeated CommitVerifiedLegacyMigrationRecovery() error = %v", err)
				}
				if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
					!result.SafeToRemoveLegacyPlaintext {
					t.Fatalf("repeated commit result = %#v", result)
				}
				if err := reconciler.Recover(ctx); err != nil {
					t.Fatalf("repeat Recover() error = %v", err)
				}
				state := registry.snapshot()
				winner := state.Providers[candidate.Provider.ID]
				storedRecovery = state.LegacyMigrationRecoveries[candidate.MigrationID]
				_, tombstones, deletes := secrets.callCounts()
				if len(state.Transactions) != 0 ||
					storedRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
					winner.CredentialRef == "" || winner.CredentialRef == storedRecovery.RecoveryCredentialRef ||
					!secrets.hasActive(winner.CredentialRef) ||
					!secrets.hasActive(storedRecovery.RecoveryCredentialRef) || secrets.recordCount() != 2 ||
					tombstones != 0 || deletes != 0 {
					t.Fatalf("reconciled commit state = %#v tombstones=%d deletes=%d", state, tombstones, deletes)
				}
			})
		}
	}
}

func TestSubsequentLegacyMigrationProviderCommitCrashPointsPreserveExistingRetainedWinners(t *testing.T) {
	t.Parallel()

	points := []FaultPoint{
		FaultBeforeMigrationProviderTransactionPrepared,
		FaultAfterTransactionPrepared,
		FaultAfterCandidateDurable,
		FaultAfterCandidateDurableRecorded,
		FaultAfterMetadataCommitted,
		FaultAfterReadbackVerified,
		FaultBeforeMigrationRecoveryCommittedRetainedRecord,
		FaultAfterMigrationRecoveryCommittedRetainedRecorded,
	}
	for _, point := range points {
		point := point
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			base := mustManager(t, registry, secrets, nil)
			alpha := legacyMigrationCandidateFor(
				registry.snapshot(), "migration-shared-crash-alpha", "provider-shared-crash-alpha",
				"shared-crash-source", "synthetic-shared-crash-alpha-not-a-real-key",
			)
			alphaRecovery, err := base.PrepareLegacyMigrationRecovery(ctx, alpha)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(alpha) error = %v", err)
			}
			if _, err := base.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), alpha, alphaRecovery),
			); err != nil {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery(alpha) error = %v", err)
			}
			retainedState := registry.snapshot()
			retainedProvider := retainedState.Providers[alpha.Provider.ID]
			retainedRecovery := retainedState.LegacyMigrationRecoveries[alpha.MigrationID]

			beta := legacyMigrationCandidateFor(
				retainedState, "migration-shared-crash-beta", "provider-shared-crash-beta",
				"shared-crash-source", "synthetic-shared-crash-beta-not-a-real-key",
			)
			betaRecovery, err := base.PrepareLegacyMigrationRecovery(ctx, beta)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(beta) error = %v", err)
			}
			command := legacyMigrationCommitCommandFor(registry.snapshot(), beta, betaRecovery)
			interrupted := mustManager(t, registry, secrets, faultOnce(point))
			if _, err := interrupted.CommitVerifiedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery(beta) error = %v, want interrupted", err)
			}

			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("fresh Manager Recover() error = %v", err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				result, err := restarted.CommitVerifiedLegacyMigrationRecovery(ctx, command)
				if err != nil {
					t.Fatalf("repeated beta commit %d error = %v", attempt+1, err)
				}
				if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
					!result.SafeToRemoveLegacyPlaintext {
					t.Fatalf("repeated beta commit %d result = %#v", attempt+1, result)
				}
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("repeat fresh Manager Recover() error = %v", err)
			}

			state := registry.snapshot()
			betaProvider := state.Providers[beta.Provider.ID]
			storedBetaRecovery := state.LegacyMigrationRecoveries[beta.MigrationID]
			_, tombstones, deletes := secrets.callCounts()
			expectedPrepared := 4
			if point == FaultAfterTransactionPrepared {
				expectedPrepared = 5
			}
			if state.Revision != 2 || len(state.Providers) != 2 || len(state.Transactions) != 0 ||
				len(state.LegacyMigrationRecoveries) != 2 || state.SelectedProviderID != alpha.Provider.ID ||
				!reflect.DeepEqual(state.Providers[alpha.Provider.ID], retainedProvider) ||
				!reflect.DeepEqual(state.LegacyMigrationRecoveries[alpha.MigrationID], retainedRecovery) ||
				storedBetaRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
				betaProvider.CredentialRef != storedBetaRecovery.CommittedProviderCredentialRef ||
				betaProvider.CredentialRef == retainedProvider.CredentialRef ||
				betaProvider.CredentialRef == storedBetaRecovery.RecoveryCredentialRef ||
				storedBetaRecovery.RecoveryCredentialRef == retainedRecovery.RecoveryCredentialRef ||
				!secrets.hasActive(retainedProvider.CredentialRef) ||
				!secrets.hasActive(retainedRecovery.RecoveryCredentialRef) ||
				!secrets.hasActive(betaProvider.CredentialRef) ||
				!secrets.hasActive(storedBetaRecovery.RecoveryCredentialRef) ||
				secrets.preparedCount() != expectedPrepared || secrets.recordCount() != 4 ||
				tombstones != 0 || deletes != 0 {
				t.Fatalf("subsequent crash recovery state = %#v prepared=%d records=%d tombstones=%d deletes=%d",
					state, secrets.preparedCount(), secrets.recordCount(), tombstones, deletes)
			}
		})
	}
}

func TestConcurrentManagersCommitOneExactLegacyMigrationProviderWinnerIdempotently(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-commit-concurrent", "provider-commit-concurrent",
		"source-commit-concurrent", "synthetic-commit-concurrent-not-a-real-key",
	)
	recovery, err := base.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery)
	managers := []*Manager{
		mustManager(t, registry, secrets, nil),
		mustManager(t, registry, secrets, nil),
	}
	start := make(chan struct{})
	results := make(chan CommitVerifiedLegacyMigrationRecoveryResult, len(managers))
	errorsSeen := make(chan error, len(managers))
	var waitGroup sync.WaitGroup
	for _, manager := range managers {
		manager := manager
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command)
			results <- result
			errorsSeen <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent CommitVerifiedLegacyMigrationRecovery() error = %v", err)
		}
	}
	for result := range results {
		if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			!result.SafeToRemoveLegacyPlaintext {
			t.Fatalf("concurrent commit result = %#v", result)
		}
	}
	state := registry.snapshot()
	winner := state.Providers[candidate.Provider.ID]
	storedRecovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
	if len(state.Providers) != 1 || len(state.Transactions) != 0 ||
		storedRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		winner.CredentialRef != storedRecovery.CommittedProviderCredentialRef ||
		secrets.preparedCount() != 2 || secrets.recordCount() != 2 {
		t.Fatalf("concurrent exact winner state = %#v prepared=%d records=%d",
			state, secrets.preparedCount(), secrets.recordCount())
	}
}

func TestConcurrentManagersCommitOneExactSubsequentLegacyMigrationProviderWinnerIdempotently(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	alpha := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-concurrent-existing", "provider-concurrent-existing",
		"shared-concurrent-source", "synthetic-concurrent-existing-not-a-real-key",
	)
	alphaRecovery, err := base.PrepareLegacyMigrationRecovery(ctx, alpha)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery(alpha) error = %v", err)
	}
	if _, err := base.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), alpha, alphaRecovery),
	); err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery(alpha) error = %v", err)
	}
	retainedState := registry.snapshot()
	retainedProvider := retainedState.Providers[alpha.Provider.ID]
	retainedRecovery := retainedState.LegacyMigrationRecoveries[alpha.MigrationID]

	beta := legacyMigrationCandidateFor(
		retainedState, "migration-concurrent-next", "provider-concurrent-next",
		"shared-concurrent-source", "synthetic-concurrent-next-not-a-real-key",
	)
	betaRecovery, err := base.PrepareLegacyMigrationRecovery(ctx, beta)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery(beta) error = %v", err)
	}
	command := legacyMigrationCommitCommandFor(registry.snapshot(), beta, betaRecovery)
	managers := []*Manager{
		mustManager(t, registry, secrets, nil),
		mustManager(t, registry, secrets, nil),
	}
	start := make(chan struct{})
	results := make(chan CommitVerifiedLegacyMigrationRecoveryResult, len(managers))
	errorsSeen := make(chan error, len(managers))
	var waitGroup sync.WaitGroup
	for _, manager := range managers {
		manager := manager
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command)
			results <- result
			errorsSeen <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent subsequent commit error = %v", err)
		}
	}
	for result := range results {
		if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			!result.SafeToRemoveLegacyPlaintext {
			t.Fatalf("concurrent subsequent commit result = %#v", result)
		}
	}
	state := registry.snapshot()
	winner := state.Providers[beta.Provider.ID]
	storedRecovery := state.LegacyMigrationRecoveries[beta.MigrationID]
	if state.Revision != 2 || len(state.Providers) != 2 || len(state.Transactions) != 0 ||
		len(state.LegacyMigrationRecoveries) != 2 || state.SelectedProviderID != alpha.Provider.ID ||
		!reflect.DeepEqual(state.Providers[alpha.Provider.ID], retainedProvider) ||
		!reflect.DeepEqual(state.LegacyMigrationRecoveries[alpha.MigrationID], retainedRecovery) ||
		winner.CredentialRef != storedRecovery.CommittedProviderCredentialRef ||
		winner.CredentialRef == retainedProvider.CredentialRef ||
		winner.CredentialRef == storedRecovery.RecoveryCredentialRef ||
		secrets.preparedCount() != 4 || secrets.recordCount() != 4 {
		t.Fatalf("concurrent subsequent winner state = %#v prepared=%d records=%d",
			state, secrets.preparedCount(), secrets.recordCount())
	}
}

func TestCommitVerifiedLegacyMigrationRecoveryRequiresExactProtectedAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*CommitVerifiedLegacyMigrationRecoveryCommand)
	}{
		{name: "wrong source", mutate: func(command *CommitVerifiedLegacyMigrationRecoveryCommand) {
			command.SourceSHA256 = strings.Repeat("b", 64)
		}},
		{name: "wrong recovery ref", mutate: func(command *CommitVerifiedLegacyMigrationRecoveryCommand) {
			command.RecoveryCredentialRef = "cred_" + secretstoreport.CredentialRef(strings.Repeat("Z", 43))
		}},
		{name: "wrong confirmation", mutate: func(command *CommitVerifiedLegacyMigrationRecoveryCommand) {
			command.Confirmation = "CONFIRM"
		}},
		{name: "stale Registry revision", mutate: func(command *CommitVerifiedLegacyMigrationRecoveryCommand) {
			command.Expected.RegistryRevision++
		}},
		{name: "stale Registry incarnation", mutate: func(command *CommitVerifiedLegacyMigrationRecoveryCommand) {
			command.Expected.RegistryIncarnation = "inc_" + strings.Repeat("Z", 43)
		}},
		{name: "stale selection fence", mutate: func(command *CommitVerifiedLegacyMigrationRecoveryCommand) {
			command.ExpectedSelectedProviderID = "provider-other"
		}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			retainedCandidate := legacyMigrationCandidateFor(
				registry.snapshot(), "migration-commit-exact-retained", "provider-commit-exact-retained",
				"source-commit-exact", "synthetic-commit-exact-retained-not-a-real-key",
			)
			retainedRecoveryResult, err := manager.PrepareLegacyMigrationRecovery(ctx, retainedCandidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(retained) error = %v", err)
			}
			if _, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), retainedCandidate, retainedRecoveryResult),
			); err != nil {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery(retained) error = %v", err)
			}
			retainedState := registry.snapshot()
			retainedProvider := retainedState.Providers[retainedCandidate.Provider.ID]
			retainedRecovery := retainedState.LegacyMigrationRecoveries[retainedCandidate.MigrationID]
			candidate := legacyMigrationCandidateFor(
				retainedState, "migration-commit-exact", "provider-commit-exact",
				"source-commit-exact", "synthetic-commit-exact-not-a-real-key",
			)
			recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
			}
			command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery)
			testCase.mutate(&command)
			before := registry.snapshot()
			preparedBefore := secrets.preparedCount()
			if _, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command); err == nil {
				t.Fatal("inexact CommitVerifiedLegacyMigrationRecovery() unexpectedly succeeded")
			}
			if !reflect.DeepEqual(before, registry.snapshot()) || secrets.preparedCount() != preparedBefore ||
				!reflect.DeepEqual(before.Providers[retainedProvider.ID], retainedProvider) ||
				!reflect.DeepEqual(before.LegacyMigrationRecoveries[retainedRecovery.ID], retainedRecovery) ||
				!secrets.hasActive(retainedProvider.CredentialRef) ||
				!secrets.hasActive(retainedRecovery.RecoveryCredentialRef) ||
				!secrets.hasActive(string(recovery.RecoveryCredentialRef)) {
				t.Fatal("inexact commit changed protected authority")
			}
		})
	}
}

func TestCommitVerifiedLegacyMigrationRecoveryRejectsPreexistingAndCompetingRegistryState(t *testing.T) {
	t.Parallel()

	providerShapes := []struct {
		name string
		seed func(*testing.T, context.Context, *memoryRegistryStore, *memorySecretStore)
	}{
		{name: "active Provider", seed: func(t *testing.T, ctx context.Context, registry *memoryRegistryStore, secrets *memorySecretStore) {
			_ = connectMemoryProvider(t, ctx, mustManager(t, registry, secrets, nil), registry,
				"provider-commit-existing", "synthetic-existing-active")
		}},
		{name: "credentialless Provider", seed: func(_ *testing.T, _ context.Context, registry *memoryRegistryStore, _ *memorySecretStore) {
			registry.mutate(func(state *domainregistry.Registry) {
				input := legacyMigrationCandidateFor(*state, "seed", "provider-commit-existing", "seed", "seed").Provider
				provider := input.Provider("inc_"+strings.Repeat("C", 43), "", "", 1, 1)
				state.Revision = 1
				state.Providers[provider.ID] = provider
				state.SelectedProviderID = provider.ID
			})
		}},
		{name: "tombstoned Provider", seed: func(_ *testing.T, _ context.Context, registry *memoryRegistryStore, _ *memorySecretStore) {
			registry.mutate(func(state *domainregistry.Registry) {
				input := legacyMigrationCandidateFor(*state, "seed", "provider-commit-existing", "seed", "seed").Provider
				provider := input.Provider("inc_"+strings.Repeat("T", 43), "", "", 1, 2)
				provider.Tombstone = true
				provider.SelectedRoutes = nil
				state.Revision = 1
				state.Providers[provider.ID] = provider
			})
		}},
		{name: "conflicting Provider metadata", seed: func(_ *testing.T, _ context.Context, registry *memoryRegistryStore, _ *memorySecretStore) {
			registry.mutate(func(state *domainregistry.Registry) {
				input := legacyMigrationCandidateFor(*state, "seed", "provider-commit-existing", "seed", "seed").Provider
				input.Endpoint = "https://conflict.invalid/v1"
				provider := input.Provider("inc_"+strings.Repeat("D", 43), "", "", 3, 4)
				state.Revision = 3
				state.Providers[provider.ID] = provider
			})
		}},
	}
	for _, testCase := range providerShapes {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			testCase.seed(t, ctx, registry, secrets)
			manager := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationCandidateFor(
				registry.snapshot(), "migration-commit-existing", "provider-commit-existing",
				"source-commit-existing", "synthetic-commit-existing-not-a-real-key",
			)
			recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
			}
			before := registry.snapshot()
			if _, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(before, candidate, recovery),
			); !errors.Is(err, registryport.ErrConflict) {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v, want conflict", err)
			}
			if !reflect.DeepEqual(before, registry.snapshot()) ||
				!secrets.hasActive(string(recovery.RecoveryCredentialRef)) {
				t.Fatal("pre-existing Provider conflict changed recovery authority")
			}
		})
	}

	t.Run("selected unrelated Provider is preserved", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		_ = connectMemoryProvider(t, ctx, manager, registry, "provider-unrelated", "synthetic-unrelated")
		before := registry.snapshot()
		selected := before.Providers[before.SelectedProviderID]
		candidate := legacyMigrationCandidateFor(
			before, "migration-commit-selected", "provider-commit-selected",
			"source-commit-selected", "synthetic-commit-selected-not-a-real-key",
		)
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
		}
		result, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
		)
		if err != nil {
			t.Fatalf("selected-state commit error = %v", err)
		}
		state := registry.snapshot()
		winner := state.Providers[candidate.Provider.ID]
		storedRecovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
		if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
			!result.SafeToRemoveLegacyPlaintext || state.Revision != before.Revision+1 ||
			state.SelectedProviderID != selected.ID || !reflect.DeepEqual(state.Providers[selected.ID], selected) ||
			winner.CredentialRef != storedRecovery.CommittedProviderCredentialRef ||
			winner.CredentialRef == storedRecovery.RecoveryCredentialRef ||
			!secrets.hasActive(selected.CredentialRef) || !secrets.hasActive(winner.CredentialRef) ||
			!secrets.hasActive(storedRecovery.RecoveryCredentialRef) {
			t.Fatalf("selected-state committed result = %#v state = %#v", result, state)
		}
		if _, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256: candidate.SourceSHA256, SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef: result.RecoveryCredentialRef, Confirmation: LegacyMigrationRollbackConfirmation,
		}); err != nil {
			t.Fatalf("BeginLegacyMigrationRollback(selected prior) error = %v", err)
		}
		if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
			t, ctx, manager, registry.snapshot(), candidate, result.RecoveryCredentialRef,
		); err != nil {
			t.Fatalf("CommitLegacyMigrationRollback(selected prior) error = %v", err)
		}
		rolledBack := registry.snapshot()
		if rolledBack.SelectedProviderID != selected.ID ||
			!reflect.DeepEqual(rolledBack.Providers[selected.ID], selected) ||
			!secrets.hasActive(selected.CredentialRef) ||
			!secrets.hasActive(string(result.RecoveryCredentialRef)) {
			t.Fatalf("selected prior rollback state = %#v", rolledBack)
		}
		if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
			t, ctx, manager, candidate, result.RecoveryCredentialRef,
			candidate.ExpectedCleanedSourceSHA256,
		); err != nil {
			t.Fatalf("FinalizeLegacyMigrationRecovery(selected prior) error = %v", err)
		}
		if !reflect.DeepEqual(registry.snapshot().Providers[selected.ID], selected) ||
			!secrets.hasActive(selected.CredentialRef) ||
			!secrets.hasActive(string(result.RecoveryCredentialRef)) ||
			secrets.exists(result.Provider.CredentialRef) {
			t.Fatal("rollback finalization changed the exact protected prior winner")
		}
		later := connectMemoryProvider(
			t, ctx, manager, registry, "provider-selected-prior-later", "synthetic-selected-prior-later",
		)
		if _, err := manager.Select(ctx, SelectCommand{
			Expected: expectedFor(registry.snapshot(), later.ID), ProviderID: later.ID,
		}); err != nil {
			t.Fatalf("Select(after prior-winner retained terminal) error = %v", err)
		}
		if _, err := manager.Snapshot(ctx); err != nil {
			t.Fatalf("Snapshot(after prior-winner retained terminal mutation) error = %v", err)
		}
		if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
			t.Fatalf("Recover(after prior-winner retained terminal mutation) error = %v", err)
		}
		terminal := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
		if terminal.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained ||
			terminal.RecoveryCredentialRef != string(result.RecoveryCredentialRef) ||
			!secrets.hasActive(string(result.RecoveryCredentialRef)) {
			t.Fatalf("ordinary mutation changed prior-winner retained terminal phase = %s", terminal.Phase)
		}
	})

	t.Run("competing recovery", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), "migration-commit-primary", "provider-commit-primary",
			"source-commit-primary", "synthetic-commit-primary",
		)
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery(primary) error = %v", err)
		}
		competing := legacyMigrationCandidateFor(
			registry.snapshot(), "migration-commit-competing", "provider-commit-competing",
			"source-commit-competing", "synthetic-commit-competing",
		)
		if _, err := manager.PrepareLegacyMigrationRecovery(ctx, competing); err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery(competing) error = %v", err)
		}
		before := registry.snapshot()
		if _, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(before, candidate, recovery),
		); !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("competing recovery commit error = %v, want conflict", err)
		}
		if !reflect.DeepEqual(before, registry.snapshot()) {
			t.Fatal("competing recovery conflict changed Registry")
		}
	})

	t.Run("competing K2 transaction", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), "migration-commit-transaction", "provider-commit-transaction",
			"source-commit-transaction", "synthetic-commit-transaction",
		)
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
		}
		interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterTransactionPrepared))
		if _, err := interrupted.Connect(
			ctx, connectCommand(registry.snapshot(), "provider-competing-transaction", "synthetic-competing-transaction"),
		); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("competing Connect() error = %v, want interrupted", err)
		}
		before := registry.snapshot()
		if _, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(before, candidate, recovery),
		); !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("competing transaction commit error = %v, want conflict", err)
		}
		if !reflect.DeepEqual(before, registry.snapshot()) ||
			!secrets.hasActive(string(recovery.RecoveryCredentialRef)) {
			t.Fatal("competing transaction conflict changed recovery authority")
		}
	})
}

func TestCommitVerifiedLegacyMigrationRecoveryRejectsUnreadableRecoveryAndDivergentWinner(t *testing.T) {
	t.Parallel()

	recoveryFailures := []struct {
		name  string
		apply func(*memorySecretStore, string)
	}{
		{name: "missing", apply: func(store *memorySecretStore, ref string) { store.remove(ref) }},
		{name: "malformed", apply: func(store *memorySecretStore, ref string) {
			store.mu.Lock()
			record := store.records[ref]
			clear(record.secret)
			record.secret = []byte("malformed-protected-payload")
			store.records[ref] = record
			store.mu.Unlock()
		}},
		{name: "tampered", apply: func(store *memorySecretStore, ref string) { store.tamper(ref) }},
		{name: "tombstoned", apply: func(store *memorySecretStore, ref string) {
			_ = store.Tombstone(context.Background(), secretstoreport.CredentialRef(ref), LegacyMigrationRecoveryPurpose)
		}},
		{name: "unauthorized", apply: func(store *memorySecretStore, _ string) {
			store.mu.Lock()
			store.denyRead = true
			store.mu.Unlock()
		}},
		{name: "wrong purpose", apply: func(store *memorySecretStore, ref string) {
			store.mu.Lock()
			record := store.records[ref]
			record.purpose = "wrong-purpose"
			store.records[ref] = record
			store.mu.Unlock()
		}},
	}
	for _, failure := range recoveryFailures {
		failure := failure
		t.Run("recovery/"+failure.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationCandidateFor(
				registry.snapshot(), "migration-commit-unreadable", "provider-commit-unreadable",
				"source-commit-unreadable", "synthetic-commit-unreadable",
			)
			recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
			}
			failure.apply(secrets, string(recovery.RecoveryCredentialRef))
			before := registry.snapshot()
			if _, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(before, candidate, recovery),
			); !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v, want verification", err)
			}
			if !reflect.DeepEqual(before, registry.snapshot()) || len(registry.snapshot().Providers) != 0 {
				t.Fatal("unreadable recovery changed Registry")
			}
		})
	}

	for _, testCase := range []struct {
		name        string
		winnerBytes string
		mutate      func(*domainregistry.Provider)
	}{
		{name: "wrong winner secret", winnerBytes: "synthetic-divergent-winner"},
		{name: "wrong winner metadata", winnerBytes: "synthetic-commit-divergent", mutate: func(provider *domainregistry.Provider) {
			provider.Endpoint = "https://divergent.invalid/v1"
		}},
	} {
		testCase := testCase
		t.Run("winner/"+testCase.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationCandidateFor(
				registry.snapshot(), "migration-commit-divergent", "provider-commit-divergent",
				"source-commit-divergent", "synthetic-commit-divergent",
			)
			recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
			}
			command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery)
			interrupted := mustManager(t, registry, secrets, faultOnce(FaultBeforeMigrationProviderTransactionPrepared))
			if _, err := interrupted.CommitVerifiedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v, want interrupted", err)
			}
			prepared, err := secrets.PreparePut(ctx, candidate.CredentialPurpose, []byte(testCase.winnerBytes))
			if err != nil {
				t.Fatalf("PreparePut(divergent winner) error = %v", err)
			}
			if err := prepared.Commit(ctx); err != nil {
				t.Fatalf("Commit(divergent winner) error = %v", err)
			}
			registry.mutate(func(state *domainregistry.Registry) {
				provider := candidate.Provider.Provider(
					"inc_"+strings.Repeat("W", 43), string(prepared.CredentialRef()),
					string(candidate.CredentialPurpose), 1, 1,
				)
				if testCase.mutate != nil {
					testCase.mutate(&provider)
				}
				state.Revision++
				state.Providers[provider.ID] = provider
				state.SelectedProviderID = provider.ID
			})
			before := registry.snapshot()
			if err := mustManager(t, registry, secrets, nil).Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("Recover(divergent winner) error = %v, want verification", err)
			}
			if !reflect.DeepEqual(before, registry.snapshot()) ||
				!secrets.hasActive(string(recovery.RecoveryCredentialRef)) ||
				!secrets.hasActive(string(prepared.CredentialRef())) {
				t.Fatal("divergent winner recovery changed retained authorities")
			}
		})
	}
}

func TestStaleLegacyMigrationProviderCASCleansOnlyCandidateAndRetainsRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	unrelated, err := secrets.PreparePut(ctx, "unrelated-purpose", []byte("synthetic-unrelated-secret"))
	if err != nil {
		t.Fatalf("PreparePut(unrelated) error = %v", err)
	}
	if err := unrelated.Commit(ctx); err != nil {
		t.Fatalf("Commit(unrelated) error = %v", err)
	}
	base := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-commit-stale-cas", "provider-commit-stale-cas",
		"source-commit-stale-cas", "synthetic-commit-stale-cas",
	)
	recovery, err := base.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery)
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterCandidateDurableRecorded))
	if _, err := interrupted.CommitVerifiedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v, want interrupted", err)
	}
	pendingState := registry.snapshot()
	pending := onlyTransaction(pendingState)
	if !secrets.hasActive(pending.CandidateCredentialRef) {
		t.Fatal("test did not leave a durable K2 candidate")
	}
	registry.mutate(func(state *domainregistry.Registry) { state.Revision++ })
	if err := mustManager(t, registry, secrets, nil).Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("Recover(stale CAS) error = %v, want verification", err)
	}
	state := registry.snapshot()
	storedRecovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
	_, tombstones, deletes := secrets.callCounts()
	if len(state.Providers) != 0 || len(state.Transactions) != 0 ||
		storedRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
		!secrets.hasActive(storedRecovery.RecoveryCredentialRef) ||
		!secrets.hasActive(string(unrelated.CredentialRef())) || secrets.exists(pending.CandidateCredentialRef) ||
		tombstones != 1 || deletes != 1 {
		t.Fatalf("stale CAS recovery state = %#v tombstones=%d deletes=%d", state, tombstones, deletes)
	}
}

func TestLegacyMigrationProviderCommitClearsManagerOwnedPlaintextBuffers(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		faults FaultRecorder
	}{
		{name: "success"},
		{name: "before K2", faults: faultOnce(FaultBeforeMigrationProviderTransactionPrepared)},
		{name: "after K2 readback", faults: faultOnce(FaultAfterReadbackVerified)},
		{name: "before retained record", faults: faultOnce(FaultBeforeMigrationRecoveryCommittedRetainedRecord)},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			registry := newMemoryRegistryStore()
			baseSecrets := newMemorySecretStore()
			secrets := &legacyMigrationPlaintextObservingStore{memorySecretStore: baseSecrets}
			base := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationCandidateWithArtifactsFor(
				registry.snapshot(), "migration-commit-buffer", "provider-commit-buffer",
				"source-commit-buffer",
			)
			callerCandidate := cloneLegacyMigrationCandidate(candidate)
			recovery, err := base.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
			}
			manager := mustManager(t, registry, secrets, testCase.faults)
			_, err = manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
			)
			if testCase.faults == nil {
				if err != nil {
					t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
				}
			} else if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v, want interrupted", err)
			}
			if !reflect.DeepEqual(candidate, callerCandidate) {
				t.Fatal("Manager mutated caller-owned migration credential, locators, or rollback artifacts")
			}
			if len(secrets.prepareBuffer) == 0 || !allZero(secrets.prepareBuffer) {
				t.Fatal("Manager-owned K1 candidate plaintext buffer was not cleared")
			}
			if len(secrets.readBuffers) == 0 {
				t.Fatal("test did not observe authorized readback plaintext")
			}
			for _, buffer := range secrets.readBuffers {
				if !allZero(buffer) {
					t.Fatal("Manager-owned authorized readback buffer was not cleared")
				}
			}
		})
	}
}

func TestCommittedRetainedLegacyMigrationRecoveryIsNotResidueOrCleanupAuthority(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-commit-retained", "provider-commit-retained",
		"source-commit-retained", "synthetic-commit-retained",
	)
	recoveryResult, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recoveryResult)
	result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	before := registry.snapshot()
	winner := before.Providers[candidate.Provider.ID]
	for attempt := 0; attempt < 2; attempt++ {
		if err := manager.Recover(ctx); err != nil {
			t.Fatalf("Recover(%d) error = %v", attempt, err)
		}
	}
	replacement, _ := secretstoreport.SetCredential([]byte("synthetic-replacement-blocked"))
	if _, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected:          expectedFor(before, winner.ID),
		ProviderID:        winner.ID,
		CredentialPurpose: "provider-api-key",
		Credential:        replacement,
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("ReplaceCredential() error = %v, want conflict", err)
	}
	if _, err := manager.Disconnect(ctx, DisconnectCommand{
		Expected: expectedFor(before, winner.ID), ProviderID: winner.ID,
		CredentialPurpose: "provider-api-key",
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("Disconnect() error = %v, want conflict", err)
	}
	if err := manager.ExplicitDelete(ctx, ExplicitDeleteCommand{
		Expected: expectedFor(before, winner.ID), ProviderID: winner.ID,
		CredentialPurpose: "provider-api-key",
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("ExplicitDelete() error = %v, want conflict", err)
	}
	if err := manager.AbandonLegacyMigrationRecovery(ctx, AbandonLegacyMigrationRecoveryCommand{
		MigrationID: candidate.MigrationID, SourceSHA256: candidate.SourceSHA256,
		RecoveryCredentialRef: recoveryResult.RecoveryCredentialRef,
		Confirmation:          LegacyMigrationAbandonConfirmation,
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("AbandonLegacyMigrationRecovery() error = %v, want conflict", err)
	}
	after := registry.snapshot()
	storedRecovery := after.LegacyMigrationRecoveries[candidate.MigrationID]
	_, tombstones, deletes := secrets.callCounts()
	if result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
		!reflect.DeepEqual(before, after) ||
		storedRecovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		!secrets.hasActive(winner.CredentialRef) ||
		!secrets.hasActive(storedRecovery.RecoveryCredentialRef) ||
		tombstones != 0 || deletes != 0 {
		t.Fatalf("committed-retained authority changed: before=%#v after=%#v tombstones=%d deletes=%d",
			before, after, tombstones, deletes)
	}
}

func TestLegacyMigrationProviderCommitErrorsAreStableAndRedacted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-commit-redacted", "provider-commit-redacted",
		"source-commit-redacted", "synthetic-unsafe-commit-canary-not-a-real-key",
	)
	recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	command := legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery)
	unsafeError := errors.New(strings.Join([]string{
		candidate.MigrationID,
		candidate.SourceSHA256,
		string(recovery.RecoveryCredentialRef),
		string(candidate.Credential),
	}, ":"))
	secrets.mu.Lock()
	secrets.prepareErr = unsafeError
	secrets.mu.Unlock()
	if _, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command); !errors.Is(err, registryport.ErrVerification) || err.Error() != registryport.ErrVerification.Error() ||
		strings.Contains(err.Error(), candidate.MigrationID) ||
		strings.Contains(err.Error(), candidate.SourceSHA256) ||
		strings.Contains(err.Error(), string(recovery.RecoveryCredentialRef)) ||
		strings.Contains(err.Error(), string(candidate.Credential)) {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %q, want stable redacted verification", err)
	}
	secrets.clearFailures()
	result, err := manager.CommitVerifiedLegacyMigrationRecovery(ctx, command)
	if err != nil || result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained {
		t.Fatalf("repeated CommitVerifiedLegacyMigrationRecovery() result=%#v error=%v", result, err)
	}
}

func TestLegacyMigrationRecoveryRegistryMetadataCannotVerifyCredentialGuesses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	recoveryRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43))
	sourceDigestBytes := sha256.Sum256([]byte("synthetic normalized legacy provider source v1"))
	sourceDigest := hex.EncodeToString(sourceDigestBytes[:])
	credentials := [][]byte{
		[]byte("synthetic-credential-guess-alpha-not-a-real-key"),
		[]byte("synthetic-credential-guess-beta-not-a-real-key"),
	}
	registryBytes := make([][]byte, 0, len(credentials))

	for _, credential := range credentials {
		events := make([]string, 0, 6)
		registry := &legacyMigrationRecordingRegistryStore{
			state: domainregistry.Registry{
				Version:                   domainregistry.FormatVersion,
				Incarnation:               registryIncarnation,
				Providers:                 map[string]domainregistry.Provider{},
				Transactions:              map[string]domainregistry.Transaction{},
				LegacyMigrationRecoveries: map[string]domainregistry.LegacyMigrationRecovery{},
			},
			events: &events,
		}
		secrets := &legacyMigrationRecordingSecretStore{
			candidateRef:         recoveryRef,
			expectedCredential:   bytes.Clone(credential),
			expectedSourceSHA256: sourceDigest,
			events:               &events,
		}
		manager, err := NewManager(registry, secrets)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		result, err := manager.PrepareLegacyMigrationRecovery(ctx, LegacyMigrationCandidate{
			Expected: domainregistry.ExpectedState{
				RegistryRevision:    0,
				RegistryIncarnation: registryIncarnation,
			},
			MigrationID:  "migration-settings-credential-guess",
			SourceSHA256: sourceDigest,
			Provider: domainregistry.ProviderInput{
				ID:             "provider-credential-guess",
				Kind:           "openai-compatible",
				Endpoint:       "https://provider.invalid/v1",
				Models:         []string{"model-alpha"},
				MediaModels:    []string{},
				SelectedModel:  "model-alpha",
				SelectedRoutes: []string{"primary"},
			},
			CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
			Credential:        credential,
		})
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
		}
		if result.Status != LegacyMigrationRecoveryStatusVerified {
			t.Fatalf("PrepareLegacyMigrationRecovery() result = %#v, want VERIFIED_RECOVERY", result)
		}
		registryBytes = append(registryBytes, bytes.Clone(registry.committedBytes))
	}

	if len(registryBytes[0]) == 0 || len(registryBytes[1]) == 0 {
		t.Fatal("test did not capture durable Registry bytes")
	}
	if !bytes.Equal(registryBytes[0], registryBytes[1]) {
		t.Fatal("Registry metadata changes solely with credential bytes and can verify credential guesses offline")
	}
}

func TestLegacyMigrationRecoveryReplayConflictAndCompetingPrepareAreDeterministic(t *testing.T) {
	t.Parallel()

	t.Run("exact replay and conflicts", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(),
			"migration-settings-alpha",
			"provider-migration-alpha",
			"source-alpha",
			"synthetic-replay-canary-not-a-real-key",
		)

		first, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("first PrepareLegacyMigrationRecovery() error = %v", err)
		}
		before := registry.snapshot()
		second, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("replay PrepareLegacyMigrationRecovery() error = %v", err)
		}
		if first != second || secrets.preparedCount() != 1 || !reflect.DeepEqual(before, registry.snapshot()) {
			t.Fatalf("exact replay changed authority: first=%#v second=%#v prepared=%d", first, second, secrets.preparedCount())
		}

		changedSource := cloneLegacyMigrationCandidate(candidate)
		changedSource.SourceSnapshot = []byte(`{"provider":{"apiKey":"synthetic-different-source"}}`)
		changedSourceDigest := sha256.Sum256(changedSource.SourceSnapshot)
		changedSource.SourceSHA256 = hex.EncodeToString(changedSourceDigest[:])
		changedCredential := cloneLegacyMigrationCandidate(candidate)
		changedCredential.Credential = []byte("synthetic-different-canary-not-a-real-key")
		competingID := cloneLegacyMigrationCandidate(candidate)
		competingID.MigrationID = "migration-settings-beta"
		for _, testCase := range []struct {
			name      string
			candidate LegacyMigrationCandidate
		}{
			{name: "same id different source", candidate: changedSource},
			{name: "same id different candidate", candidate: changedCredential},
			{name: "different id same Provider", candidate: competingID},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				_, err := manager.PrepareLegacyMigrationRecovery(ctx, testCase.candidate)
				if !errors.Is(err, registryport.ErrConflict) {
					t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want conflict", err)
				}
				if secrets.preparedCount() != 1 || !reflect.DeepEqual(before, registry.snapshot()) {
					t.Fatal("conflicting replay changed durable recovery authority")
				}
			})
		}

		_ = connectMemoryProvider(t, ctx, manager, registry, "provider-unrelated", "synthetic-unrelated-secret")
		_, err = manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("stale replay error = %v, want conflict", err)
		}
	})

	t.Run("competing Managers", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		state := registry.snapshot()
		firstCandidate := legacyMigrationCandidateFor(
			state, "migration-competing-alpha", "provider-competing", "source-alpha", "synthetic-competing-alpha",
		)
		secondCandidate := legacyMigrationCandidateFor(
			state, "migration-competing-beta", "provider-competing", "source-beta", "synthetic-competing-beta",
		)
		managers := []*Manager{
			mustManager(t, registry, secrets, nil),
			mustManager(t, registry, secrets, nil),
		}
		candidates := []LegacyMigrationCandidate{firstCandidate, secondCandidate}
		start := make(chan struct{})
		errorsSeen := make(chan error, 2)
		var waitGroup sync.WaitGroup
		for index := range managers {
			waitGroup.Add(1)
			go func(index int) {
				defer waitGroup.Done()
				<-start
				_, err := managers[index].PrepareLegacyMigrationRecovery(ctx, candidates[index])
				errorsSeen <- err
			}(index)
		}
		close(start)
		waitGroup.Wait()
		close(errorsSeen)
		var successes, conflicts int
		for err := range errorsSeen {
			if err == nil {
				successes++
			} else if errors.Is(err, registryport.ErrConflict) {
				conflicts++
			} else {
				t.Fatalf("competing PrepareLegacyMigrationRecovery() error = %v", err)
			}
		}
		state = registry.snapshot()
		if successes != 1 || conflicts != 1 || len(state.LegacyMigrationRecoveries) != 1 ||
			secrets.recordCount() != 1 || secrets.preparedCount() != 1 {
			t.Fatalf("competing outcome successes=%d conflicts=%d state=%#v records=%d prepared=%d",
				successes, conflicts, state, secrets.recordCount(), secrets.preparedCount())
		}
	})
}

func TestLegacyMigrationRecoveryCrashPointsRestartDeterministically(t *testing.T) {
	t.Parallel()

	points := []FaultPoint{
		FaultAfterMigrationRecoveryPrepared,
		FaultAfterMigrationRecoverySecretDurable,
		FaultAfterMigrationRecoverySecretDurableRecorded,
		FaultAfterMigrationRecoveryReadbackVerified,
		FaultAfterMigrationRecoveryVerifiedRecorded,
	}
	for _, point := range points {
		point := point
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			candidate := legacyMigrationCandidateWithArtifactsFor(
				registry.snapshot(), "migration-crash-alpha", "provider-crash-alpha", "source-crash",
			)
			interrupted := mustManager(t, registry, secrets, faultOnce(point))
			_, err := interrupted.PrepareLegacyMigrationRecovery(ctx, candidate)
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want interrupted", err)
			}

			restarted := mustManager(t, registry, secrets, nil)
			if point == FaultAfterMigrationRecoveryPrepared {
				for attempt := 0; attempt < 2; attempt++ {
					if err := restarted.Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
						t.Fatalf("Recover() missing prepared secret error = %v, want verification", err)
					}
				}
				before := registry.snapshot()
				for attempt := 0; attempt < 2; attempt++ {
					if _, err := restarted.PrepareLegacyMigrationRecovery(ctx, candidate); !errors.Is(err, registryport.ErrVerification) {
						t.Fatalf("exact replay without protected candidate error = %v, want verification", err)
					}
				}
				after := registry.snapshot()
				if !reflect.DeepEqual(before, after) ||
					after.LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
						domainregistry.LegacyMigrationRecoveryPhasePrepared ||
					secrets.preparedCount() != 1 || secrets.recordCount() != 0 || len(after.Providers) != 0 {
					t.Fatalf("missing protected candidate replay changed source-preserving state: %#v", after)
				}
				return
			} else if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("fresh Recover() error = %v", err)
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("repeat Recover() error = %v", err)
			}
			state := registry.snapshot()
			recovery := state.LegacyMigrationRecoveries[candidate.MigrationID]
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified ||
				len(state.Providers) != 0 || state.Revision != 0 || !secrets.hasActive(recovery.RecoveryCredentialRef) {
				t.Fatalf("restarted recovery state = %#v", state)
			}
		})
	}
}

func TestPreparedMissingLegacyMigrationRecoveryCanRollbackAndRetry(t *testing.T) {
	t.Parallel()

	t.Run("exact explicit rollback survives interruption and permits retry", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		base := mustManager(t, registry, secrets, nil)
		winner := connectMemoryProvider(t, ctx, base, registry, "provider-existing", "synthetic-existing-winner")
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), "migration-prepared-missing", "provider-prepared-missing",
			"source-prepared-missing", "synthetic-prepared-missing-not-a-real-key",
		)
		interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationRecoveryPrepared))
		if _, err := interrupted.PrepareLegacyMigrationRecovery(ctx, candidate); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want interrupted", err)
		}
		before := registry.snapshot()
		missing := before.LegacyMigrationRecoveries[candidate.MigrationID]
		if missing.Phase != domainregistry.LegacyMigrationRecoveryPhasePrepared ||
			secrets.exists(missing.RecoveryCredentialRef) || !secrets.hasActive(winner.CredentialRef) {
			t.Fatalf("prepared missing setup = %#v", before)
		}

		invalid := AbandonLegacyMigrationRecoveryCommand{
			MigrationID: candidate.MigrationID, SourceSHA256: candidate.SourceSHA256,
			RecoveryCredentialRef: "cred_" + secretstoreport.CredentialRef(strings.Repeat("Z", 43)),
			Confirmation:          LegacyMigrationAbandonConfirmation,
		}
		if err := mustManager(t, registry, secrets, nil).AbandonLegacyMigrationRecovery(ctx, invalid); !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("non-exact prepared rollback error = %v, want conflict", err)
		}
		if !reflect.DeepEqual(before, registry.snapshot()) || !secrets.hasActive(winner.CredentialRef) {
			t.Fatal("non-exact prepared rollback changed recovery authority")
		}

		command := AbandonLegacyMigrationRecoveryCommand{
			MigrationID: candidate.MigrationID, SourceSHA256: candidate.SourceSHA256,
			RecoveryCredentialRef: secretstoreport.CredentialRef(missing.RecoveryCredentialRef),
			Confirmation:          LegacyMigrationAbandonConfirmation,
		}
		rollback := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationRecoveryAbandonRecorded))
		if err := rollback.AbandonLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("AbandonLegacyMigrationRecovery() error = %v, want interrupted", err)
		}
		abandoning := registry.snapshot()
		if abandoning.LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
			domainregistry.LegacyMigrationRecoveryPhaseAbandoning || !secrets.hasActive(winner.CredentialRef) {
			t.Fatalf("durable prepared rollback intent = %#v", abandoning)
		}

		restarted := mustManager(t, registry, secrets, nil)
		if err := restarted.Recover(ctx); err != nil {
			t.Fatalf("fresh Recover() prepared rollback error = %v", err)
		}
		if err := restarted.Recover(ctx); err != nil {
			t.Fatalf("repeat Recover() prepared rollback error = %v", err)
		}
		afterRollback := registry.snapshot()
		if _, exists := afterRollback.LegacyMigrationRecoveries[candidate.MigrationID]; exists ||
			!reflect.DeepEqual(afterRollback.Providers[winner.ID], before.Providers[winner.ID]) ||
			!secrets.hasActive(winner.CredentialRef) {
			t.Fatalf("prepared rollback damaged Provider winner: %#v", afterRollback)
		}

		result, err := restarted.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("fresh PrepareLegacyMigrationRecovery() after rollback error = %v", err)
		}
		final := registry.snapshot()
		recovery := final.LegacyMigrationRecoveries[candidate.MigrationID]
		if result.Status != LegacyMigrationRecoveryStatusVerified ||
			recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified ||
			recovery.RecoveryCredentialRef == missing.RecoveryCredentialRef ||
			!secrets.hasActive(recovery.RecoveryCredentialRef) || !secrets.hasActive(winner.CredentialRef) ||
			!reflect.DeepEqual(final.Providers[winner.ID], before.Providers[winner.ID]) {
			t.Fatalf("fresh verified retry = %#v result=%#v", final, result)
		}
	})

	t.Run("stale prepared intent cannot be discarded", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), "migration-prepared-stale", "provider-prepared-stale",
			"source-prepared-stale", "synthetic-prepared-stale-not-a-real-key",
		)
		interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationRecoveryPrepared))
		if _, err := interrupted.PrepareLegacyMigrationRecovery(ctx, candidate); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want interrupted", err)
		}
		before := registry.snapshot()
		missing := before.LegacyMigrationRecoveries[candidate.MigrationID]
		registry.mutate(func(state *domainregistry.Registry) { state.Revision++ })
		stale := registry.snapshot()
		command := AbandonLegacyMigrationRecoveryCommand{
			MigrationID: candidate.MigrationID, SourceSHA256: candidate.SourceSHA256,
			RecoveryCredentialRef: secretstoreport.CredentialRef(missing.RecoveryCredentialRef),
			Confirmation:          LegacyMigrationAbandonConfirmation,
		}
		if err := mustManager(t, registry, secrets, nil).AbandonLegacyMigrationRecovery(ctx, command); !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("stale prepared rollback error = %v, want conflict", err)
		}
		if !reflect.DeepEqual(stale, registry.snapshot()) || secrets.recordCount() != 0 {
			t.Fatal("stale prepared rollback changed recovery authority")
		}
	})
}

func TestLegacyMigrationRecoveryAbandonOnlyDiscardsPreparedNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		point FaultPoint
		apply func(*memorySecretStore, string)
	}{
		{name: "prepared unauthorized", point: FaultAfterMigrationRecoveryPrepared, apply: func(store *memorySecretStore, _ string) {
			store.mu.Lock()
			store.denyRead = true
			store.mu.Unlock()
		}},
		{name: "prepared tombstoned", point: FaultAfterMigrationRecoverySecretDurable, apply: func(store *memorySecretStore, ref string) {
			_ = store.Tombstone(context.Background(), secretstoreport.CredentialRef(ref), LegacyMigrationRecoveryPurpose)
		}},
		{name: "prepared wrong purpose", point: FaultAfterMigrationRecoverySecretDurable, apply: func(store *memorySecretStore, ref string) {
			store.mu.Lock()
			record := store.records[ref]
			record.purpose = "wrong-purpose"
			store.records[ref] = record
			store.mu.Unlock()
		}},
		{name: "prepared tampered", point: FaultAfterMigrationRecoverySecretDurable, apply: func(store *memorySecretStore, ref string) {
			store.tamper(ref)
		}},
		{name: "secret durable missing", point: FaultAfterMigrationRecoveryReadbackVerified, apply: func(store *memorySecretStore, ref string) {
			store.remove(ref)
		}},
		{name: "verified missing", apply: func(store *memorySecretStore, ref string) { store.remove(ref) }},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			candidate := legacyMigrationCandidateFor(
				registry.snapshot(), "migration-abandon-unreadable", "provider-abandon-unreadable",
				"source-abandon-unreadable", "synthetic-abandon-unreadable-not-a-real-key",
			)
			manager := mustManager(t, registry, secrets, nil)
			if testCase.point != "" {
				manager = mustManager(t, registry, secrets, faultOnce(testCase.point))
			}
			_, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if testCase.point == "" {
				if err != nil {
					t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
				}
			} else if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want interrupted", err)
			}
			before := registry.snapshot()
			recovery := before.LegacyMigrationRecoveries[candidate.MigrationID]
			testCase.apply(secrets, recovery.RecoveryCredentialRef)
			command := AbandonLegacyMigrationRecoveryCommand{
				MigrationID: candidate.MigrationID, SourceSHA256: candidate.SourceSHA256,
				RecoveryCredentialRef: secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
				Confirmation:          LegacyMigrationAbandonConfirmation,
			}
			if err := mustManager(t, registry, secrets, nil).AbandonLegacyMigrationRecovery(ctx, command); !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("AbandonLegacyMigrationRecovery() error = %v, want verification", err)
			}
			if !reflect.DeepEqual(before, registry.snapshot()) {
				t.Fatal("unreadable protected candidate was discarded or mutated")
			}
		})
	}
}

func TestLegacyMigrationRecoveryReadFailuresNeverPromoteOrDeleteRecord(t *testing.T) {
	t.Parallel()

	failures := []struct {
		name  string
		apply func(*memorySecretStore, string)
	}{
		{name: "missing", apply: func(store *memorySecretStore, ref string) { store.remove(ref) }},
		{name: "tombstoned", apply: func(store *memorySecretStore, ref string) {
			_ = store.Tombstone(context.Background(), secretstoreport.CredentialRef(ref), LegacyMigrationRecoveryPurpose)
		}},
		{name: "tampered", apply: func(store *memorySecretStore, ref string) { store.tamper(ref) }},
		{name: "unauthorized", apply: func(store *memorySecretStore, _ string) {
			store.mu.Lock()
			store.denyRead = true
			store.mu.Unlock()
		}},
		{name: "wrong purpose", apply: func(store *memorySecretStore, ref string) {
			store.mu.Lock()
			record := store.records[ref]
			record.purpose = "wrong-purpose"
			store.records[ref] = record
			store.mu.Unlock()
		}},
		{name: "wrong plaintext", apply: func(store *memorySecretStore, _ string) {
			store.mu.Lock()
			store.wrongNextRead = true
			store.mu.Unlock()
		}},
	}
	for _, phase := range []domainregistry.LegacyMigrationRecoveryPhase{
		domainregistry.LegacyMigrationRecoveryPhaseSecretDurable,
		domainregistry.LegacyMigrationRecoveryPhaseVerified,
	} {
		phase := phase
		for _, failure := range failures {
			failure := failure
			t.Run(string(phase)+"/"+failure.name, func(t *testing.T) {
				t.Parallel()

				ctx := context.Background()
				registry := newMemoryRegistryStore()
				secrets := newMemorySecretStore()
				candidate := legacyMigrationCandidateWithArtifactsFor(
					registry.snapshot(), "migration-failure-alpha", "provider-failure-alpha", "source-failure",
				)
				var manager *Manager
				if phase == domainregistry.LegacyMigrationRecoveryPhaseSecretDurable {
					manager = mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationRecoveryReadbackVerified))
					if _, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate); !errors.Is(err, ErrInterrupted) {
						t.Fatalf("PrepareLegacyMigrationRecovery() error = %v, want interrupted", err)
					}
				} else {
					manager = mustManager(t, registry, secrets, nil)
					if _, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate); err != nil {
						t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
					}
				}
				before := registry.snapshot()
				recovery := before.LegacyMigrationRecoveries[candidate.MigrationID]
				if recovery.Phase != phase {
					t.Fatalf("setup phase = %q, want %q", recovery.Phase, phase)
				}
				failure.apply(secrets, recovery.RecoveryCredentialRef)
				if err := mustManager(t, registry, secrets, nil).Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
					t.Fatalf("Recover() error = %v, want verification", err)
				}
				after := registry.snapshot()
				current, exists := after.LegacyMigrationRecoveries[candidate.MigrationID]
				if !exists || current.Phase != phase || len(after.Providers) != 0 || after.Revision != before.Revision {
					t.Fatalf("failed readback changed recovery authority: %#v", after)
				}
			})
		}
	}
}

func TestLegacyMigrationRecoveryProtectedPayloadFailsClosed(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(t *testing.T, candidate LegacyMigrationCandidate, payload []byte) []byte
	}{
		{name: "malformed", mutate: func(_ *testing.T, _ LegacyMigrationCandidate, payload []byte) []byte {
			mutated := bytes.Clone(payload)
			mutated[0] ^= 0xff
			return mutated
		}},
		{name: "truncated", mutate: func(_ *testing.T, _ LegacyMigrationCandidate, payload []byte) []byte {
			return bytes.Clone(payload[:len(payload)-1])
		}},
		{name: "unknown version", mutate: func(_ *testing.T, _ LegacyMigrationCandidate, payload []byte) []byte {
			mutated := bytes.Clone(payload)
			binary.BigEndian.PutUint32(
				mutated[len(legacyMigrationRecoveryPayloadMagic):],
				legacyMigrationRecoveryPayloadVersionV2+1,
			)
			return mutated
		}},
		{name: "tampered protected binding", mutate: func(t *testing.T, candidate LegacyMigrationCandidate, _ []byte) []byte {
			candidate.Provider.SelectedRoutes = []string{"secondary"}
			payload, err := marshalLegacyMigrationRecoveryPayloadV2(
				candidate.MigrationID,
				candidate.SourceSHA256,
				candidate.Provider,
				candidate.CredentialPurpose,
				candidate.Credential,
				candidate.ActiveCredentialLocators,
				candidate.RollbackCredentialArtifacts,
			)
			if err != nil {
				t.Fatalf("marshalLegacyMigrationRecoveryPayload() error = %v", err)
			}
			return payload
		}},
	}
	for _, testCase := range mutations {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			candidate := legacyMigrationCandidateWithArtifactsFor(
				registry.snapshot(), "migration-protected-payload", "provider-protected-payload",
				"source-protected-payload",
			)
			if _, err := mustManager(t, registry, secrets, nil).PrepareLegacyMigrationRecovery(ctx, candidate); err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
			}
			before := registry.snapshot()
			recovery := before.LegacyMigrationRecoveries[candidate.MigrationID]
			secrets.mu.Lock()
			record := secrets.records[recovery.RecoveryCredentialRef]
			original := bytes.Clone(record.secret)
			secrets.mu.Unlock()
			mutated := testCase.mutate(t, candidate, original)
			clear(original)
			secrets.mu.Lock()
			record = secrets.records[recovery.RecoveryCredentialRef]
			clear(record.secret)
			record.secret = mutated
			secrets.records[recovery.RecoveryCredentialRef] = record
			secrets.mu.Unlock()

			if err := mustManager(t, registry, secrets, nil).Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("Recover() error = %v, want verification", err)
			}
			after := registry.snapshot()
			if !reflect.DeepEqual(before, after) ||
				after.LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
					domainregistry.LegacyMigrationRecoveryPhaseVerified || len(after.Providers) != 0 {
				t.Fatalf("invalid protected payload changed recovery authority: %#v", after)
			}
		})
	}
}

func TestVerifiedLegacyMigrationRecoveryCoexistsWithK2AndPreservesProviderWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	winner := connectMemoryProvider(t, ctx, manager, registry, "provider-existing", "synthetic-existing-winner")
	before := registry.snapshot()
	candidate := legacyMigrationCandidateFor(
		before, "migration-existing-provider", winner.ID, "source-existing", "synthetic-recovery-for-existing-provider",
	)
	recoveryResult, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	afterRecovery := registry.snapshot()
	if afterRecovery.Revision != before.Revision ||
		!reflect.DeepEqual(afterRecovery.Providers[winner.ID], before.Providers[winner.ID]) ||
		recoveryResult.RecoveryCredentialRef == secretstoreport.CredentialRef(winner.CredentialRef) {
		t.Fatalf("recovery preparation rewrote Provider winner: before=%#v after=%#v", before, afterRecovery)
	}

	replacement, _ := secretstoreport.SetCredential([]byte("synthetic-new-k2-winner"))
	updated, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected:          expectedFor(afterRecovery, winner.ID),
		ProviderID:        winner.ID,
		CredentialPurpose: "provider-api-key",
		Credential:        replacement,
	})
	if err != nil {
		t.Fatalf("ReplaceCredential() error = %v", err)
	}
	final := registry.snapshot()
	recovery := final.LegacyMigrationRecoveries[candidate.MigrationID]
	if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified ||
		updated.CredentialRef == winner.CredentialRef || updated.CredentialRef == recovery.RecoveryCredentialRef ||
		!secrets.hasActive(updated.CredentialRef) || !secrets.hasActive(recovery.RecoveryCredentialRef) {
		t.Fatalf("K2 and migration recovery authority = %#v", final)
	}
	if err := manager.Recover(ctx); err != nil {
		t.Fatalf("Recover() with verified migration and K2 winner error = %v", err)
	}
	secrets.remove(updated.CredentialRef)
	if err := manager.Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("Recover() unreadable K2 winner error = %v, want verification", err)
	}
	if registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
		domainregistry.LegacyMigrationRecoveryPhaseVerified {
		t.Fatal("K2 committed-credential verification failure damaged verified migration recovery")
	}
}

func TestManagerRecoverCompletesK2ResidueWithoutConsumingVerifiedLegacyMigrationRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-k2-residue", "provider-migration-residue", "source-k2-residue",
		"synthetic-k2-residue-recovery",
	)
	result, err := base.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterCandidateDurableRecorded))
	_, err = interrupted.Connect(ctx, connectCommand(registry.snapshot(), "provider-k2-residue", "synthetic-k2-candidate"))
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Connect() error = %v, want interrupted", err)
	}
	if len(registry.snapshot().Transactions) != 1 {
		t.Fatal("test did not leave one K2 transaction residue")
	}
	if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
		t.Fatalf("Recover() mixed K2 and migration state error = %v", err)
	}
	state := registry.snapshot()
	recovery, exists := state.LegacyMigrationRecoveries[candidate.MigrationID]
	provider, providerExists := state.Providers["provider-k2-residue"]
	if len(state.Transactions) != 0 || !exists || recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified ||
		recovery.RecoveryCredentialRef != string(result.RecoveryCredentialRef) || !providerExists ||
		!secrets.hasActive(provider.CredentialRef) || !secrets.hasActive(recovery.RecoveryCredentialRef) {
		t.Fatalf("mixed recovery result = %#v", state)
	}
}

func TestLegacyMigrationRecoveryClearsManagerOwnedPlaintextBuffers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := &legacyMigrationPlaintextObservingStore{memorySecretStore: newMemorySecretStore()}
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithArtifactsFor(
		registry.snapshot(), "migration-buffer-zero", "provider-buffer-zero", "source-buffer-zero",
	)
	original := cloneLegacyMigrationCandidate(candidate)
	if _, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate); err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	if !reflect.DeepEqual(candidate, original) {
		t.Fatal("Manager mutated the caller-owned bounded migration candidate")
	}
	if len(secrets.prepareBuffer) == 0 || !allZero(secrets.prepareBuffer) {
		t.Fatal("Manager-owned K1 prepare plaintext buffer was not cleared")
	}
	if len(secrets.readBuffers) == 0 {
		t.Fatal("test did not observe an authorized readback buffer")
	}
	for _, buffer := range secrets.readBuffers {
		if !allZero(buffer) {
			t.Fatal("Manager-owned authorized readback plaintext buffer was not cleared")
		}
	}
}

func TestLegacyMigrationRecoveryAbandonRequiresExactExplicitIntentAndRestarts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	baseSecrets := newMemorySecretStore()
	secrets := &legacyMigrationCleanupFailureStore{memorySecretStore: baseSecrets}
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithArtifactsFor(
		registry.snapshot(), "migration-abandon-alpha", "provider-abandon-alpha", "source-abandon",
	)
	result, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	command := AbandonLegacyMigrationRecoveryCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256:          candidate.SourceSHA256,
		RecoveryCredentialRef: result.RecoveryCredentialRef, Confirmation: LegacyMigrationAbandonConfirmation,
	}
	before := registry.snapshot()
	_, tombstonesBefore, deletesBefore := baseSecrets.callCounts()
	invalidCommands := []AbandonLegacyMigrationRecoveryCommand{
		{},
		{MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator, SourceSHA256: candidate.SourceSHA256, RecoveryCredentialRef: result.RecoveryCredentialRef},
		{MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator, SourceSHA256: "****", RecoveryCredentialRef: result.RecoveryCredentialRef, Confirmation: LegacyMigrationAbandonConfirmation},
		{MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator, SourceSHA256: strings.Repeat("b", 64), RecoveryCredentialRef: result.RecoveryCredentialRef, Confirmation: LegacyMigrationAbandonConfirmation},
		{MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator, SourceSHA256: candidate.SourceSHA256, RecoveryCredentialRef: "cred_" + secretstoreport.CredentialRef(strings.Repeat("Z", 43)), Confirmation: LegacyMigrationAbandonConfirmation},
	}
	for _, invalid := range invalidCommands {
		if err := manager.AbandonLegacyMigrationRecovery(ctx, invalid); err == nil {
			t.Fatalf("AbandonLegacyMigrationRecovery(%#v) unexpectedly succeeded", invalid)
		}
	}
	_, tombstonesAfter, deletesAfter := baseSecrets.callCounts()
	if tombstonesAfter != tombstonesBefore || deletesAfter != deletesBefore ||
		!reflect.DeepEqual(before, registry.snapshot()) || !baseSecrets.hasActive(string(result.RecoveryCredentialRef)) {
		t.Fatal("empty, omitted, redacted, or conflicting abandon input changed verified recovery")
	}

	secrets.tombstoneErr = secretstoreport.ErrPersistence
	if err := manager.AbandonLegacyMigrationRecovery(ctx, command); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("AbandonLegacyMigrationRecovery() error = %v, want persistence", err)
	}
	current := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
	if current.Phase != domainregistry.LegacyMigrationRecoveryPhaseAbandoning ||
		!baseSecrets.hasActive(current.RecoveryCredentialRef) {
		t.Fatalf("failed explicit abandon damaged recovery bytes: %#v", current)
	}
	secrets.tombstoneErr = nil
	if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
		t.Fatalf("fresh Recover() abandon error = %v", err)
	}
	if _, exists := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]; exists ||
		baseSecrets.exists(string(result.RecoveryCredentialRef)) {
		t.Fatal("restart did not finish exact task-owned recovery abandon")
	}
	if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
		t.Fatalf("repeat Recover() after abandon error = %v", err)
	}
}

func TestLegacyMigrationRecoveryRejectsMalformedStaleOverLimitAndAliasedInputWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseRegistry := newMemoryRegistryStore()
	valid := legacyMigrationCandidateFor(
		baseRegistry.snapshot(), "migration-validation-alpha", "provider-validation-alpha", "source-validation",
		"synthetic-validation-canary-not-a-real-key",
	)
	mutations := []struct {
		name   string
		mutate func(*LegacyMigrationCandidate)
	}{
		{name: "empty id", mutate: func(value *LegacyMigrationCandidate) { value.MigrationID = "" }},
		{name: "unsafe id", mutate: func(value *LegacyMigrationCandidate) { value.MigrationID = "../migration" }},
		{name: "unknown digest form", mutate: func(value *LegacyMigrationCandidate) { value.SourceSHA256 = strings.Repeat("A", 64) }},
		{name: "empty credential", mutate: func(value *LegacyMigrationCandidate) { value.Credential = nil }},
		{name: "over limit credential", mutate: func(value *LegacyMigrationCandidate) {
			value.Credential = make([]byte, maxLegacyMigrationCredentialBytes+1)
		}},
		{name: "wrong credential purpose", mutate: func(value *LegacyMigrationCandidate) { value.CredentialPurpose = "Provider API Key" }},
		{name: "source descriptor without credential locator inventory", mutate: func(value *LegacyMigrationCandidate) {
			value.ActiveCredentialLocators = nil
			value.RollbackCredentialArtifacts = nil
		}},
		{name: "malformed Provider", mutate: func(value *LegacyMigrationCandidate) {
			value.Provider.Endpoint = "https://user:secret@provider.invalid/v1"
		}},
		{name: "stale revision", mutate: func(value *LegacyMigrationCandidate) { value.Expected.RegistryRevision++ }},
		{name: "stale incarnation", mutate: func(value *LegacyMigrationCandidate) {
			value.Expected.RegistryIncarnation = "inc_" + strings.Repeat("Z", 43)
		}},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			candidate := cloneLegacyMigrationCandidate(valid)
			candidate.Expected = legacyMigrationCandidateFor(
				registry.snapshot(), candidate.MigrationID, candidate.Provider.ID, "source-validation",
				string(candidate.Credential),
			).Expected
			testCase.mutate(&candidate)
			_, err := mustManager(t, registry, secrets, nil).PrepareLegacyMigrationRecovery(ctx, candidate)
			if err == nil {
				t.Fatal("PrepareLegacyMigrationRecovery() unexpectedly succeeded")
			}
			if len(registry.snapshot().LegacyMigrationRecoveries) != 0 || secrets.preparedCount() != 0 || secrets.recordCount() != 0 {
				t.Fatal("rejected legacy migration candidate mutated Registry or Secret Store")
			}
		})
	}

	t.Run("recovery ref cannot alias Provider winner", func(t *testing.T) {
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		winner := connectMemoryProvider(t, ctx, manager, registry, "provider-alias-winner", "synthetic-alias-winner")
		secrets.mu.Lock()
		secrets.next = 0
		secrets.mu.Unlock()
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), "migration-alias-alpha", "provider-alias-migration", "source-alias",
			"synthetic-alias-recovery",
		)
		if _, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate); !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("PrepareLegacyMigrationRecovery() alias error = %v, want conflict", err)
		}
		if !secrets.hasActive(winner.CredentialRef) || len(registry.snapshot().LegacyMigrationRecoveries) != 0 {
			t.Fatal("recovery alias attempt damaged Provider winner")
		}
	})

	t.Run("K2 candidate cannot alias recovery ref", func(t *testing.T) {
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), "migration-alias-k2", "provider-alias-recovery", "source-alias-k2",
			"synthetic-alias-k2-recovery",
		)
		result, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
		}
		secrets.mu.Lock()
		secrets.next = 0
		secrets.mu.Unlock()
		_, err = manager.Connect(ctx, connectCommand(registry.snapshot(), "provider-k2-alias", "synthetic-k2-alias"))
		if !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("Connect() recovery alias error = %v, want conflict", err)
		}
		if !secrets.hasActive(string(result.RecoveryCredentialRef)) || len(registry.snapshot().Providers) != 0 {
			t.Fatal("K2 alias attempt damaged verified migration recovery")
		}
	})
}

func TestLegacyMigrationRecoveryErrorsAreStableAndRedacted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	unsafeCanary := "synthetic-unsafe-provider-body-canary-not-a-real-key"
	registry := newMemoryRegistryStore()
	candidate := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-redacted-error", "provider-redacted-error", "source-redacted-error",
		"synthetic-redacted-error-credential",
	)
	secrets := newMemorySecretStore()
	secrets.prepareErr = errors.New(unsafeCanary)
	_, err := mustManager(t, registry, secrets, nil).PrepareLegacyMigrationRecovery(ctx, candidate)
	if !errors.Is(err, registryport.ErrVerification) || strings.Contains(err.Error(), unsafeCanary) {
		t.Fatalf("Secret Store error projection = %q, want stable redacted verification", err)
	}

	manager, newErr := NewManager(errorRegistryStore{err: errors.New(unsafeCanary)}, newMemorySecretStore())
	if newErr != nil {
		t.Fatalf("NewManager() error = %v", newErr)
	}
	_, err = manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if !errors.Is(err, registryport.ErrPersistence) || strings.Contains(err.Error(), unsafeCanary) {
		t.Fatalf("Registry error projection = %q, want stable redacted persistence", err)
	}
}

func TestManagerRollsBackCommittedLegacyMigrationOnlyFromCleanedSourceAndFinalizesSeparately(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(),
		"migration-explicit-rollback",
		"provider-explicit-rollback",
		"source-explicit-rollback",
	)
	candidate.SourceSnapshot = []byte(`{"provider":{"apiKey":"synthetic-explicit-rollback"}}`)
	candidate.SourceLocator = "current:analytix-settings.json"
	sourceDigest := sha256.Sum256(candidate.SourceSnapshot)
	candidate.SourceSHA256 = hex.EncodeToString(sourceDigest[:])
	recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx,
		legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}

	begin, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256:                 candidate.SourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef:        committed.RecoveryCredentialRef,
		Confirmation:                 LegacyMigrationRollbackConfirmation,
	})
	if err != nil {
		t.Fatalf("BeginLegacyMigrationRollback() error = %v", err)
	}
	if begin.Status != LegacyMigrationRollbackStatusCleanedSourceRequired ||
		begin.MigrationID != candidate.MigrationID {
		t.Fatalf("BeginLegacyMigrationRollback() result = %#v", begin)
	}
	stateBeforeRestore := registry.snapshot()
	winnerBeforeRestore, exists := stateBeforeRestore.Providers[candidate.Provider.ID]
	if !exists || winnerBeforeRestore.CredentialRef == "" ||
		!secrets.hasActive(winnerBeforeRestore.CredentialRef) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatalf("rollback begin did not retain exact winner and recovery: %#v", stateBeforeRestore)
	}

	rollback, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
		t, ctx, manager, stateBeforeRestore, candidate, committed.RecoveryCredentialRef,
	)
	if err != nil {
		t.Fatalf("CommitLegacyMigrationRollback() error = %v", err)
	}
	if rollback.Status != LegacyMigrationRollbackStatusCommittedRecoveryRetained {
		t.Fatalf("CommitLegacyMigrationRollback() result = %#v", rollback)
	}
	repeatedBegin, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256:                 candidate.SourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef:        committed.RecoveryCredentialRef,
		Confirmation:                 LegacyMigrationRollbackConfirmation,
	})
	if err != nil {
		t.Fatalf("BeginLegacyMigrationRollback(repeat) error = %v", err)
	}
	if repeatedBegin.Status != LegacyMigrationRollbackStatusCommittedRecoveryRetained ||
		repeatedBegin.MigrationID != candidate.MigrationID {
		t.Fatalf("BeginLegacyMigrationRollback(repeat) result = %#v", repeatedBegin)
	}
	rolledBackState := registry.snapshot()
	if _, exists := rolledBackState.Providers[candidate.Provider.ID]; exists ||
		rolledBackState.SelectedProviderID != "" ||
		!secrets.hasActive(winnerBeforeRestore.CredentialRef) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatalf("rollback commit discarded protected material before finalization: %#v", rolledBackState)
	}

	finalized, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, manager, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	)
	if err != nil {
		t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v", err)
	}
	if finalized.Status != LegacyMigrationFinalizationStatusCompleted ||
		finalized.Outcome != LegacyMigrationFinalizationOutcomeRolledBack {
		t.Fatalf("FinalizeLegacyMigrationRecovery() result = %#v", finalized)
	}
	if secrets.exists(winnerBeforeRestore.CredentialRef) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatal("rollback finalization did not delete only the inactive migrated winner")
	}

	repeated, err := manager.FinalizeLegacyMigrationRecovery(
		ctx,
		legacyMigrationTestFinalizeCommand(
			t, candidate, "", candidate.ExpectedCleanedSourceSHA256,
		),
	)
	if err != nil {
		t.Fatalf("FinalizeLegacyMigrationRecovery(repeat) error = %v", err)
	}
	if repeated.Status != LegacyMigrationFinalizationStatusAlreadyFinalized ||
		repeated.Outcome != LegacyMigrationFinalizationOutcomeRolledBack {
		t.Fatalf("FinalizeLegacyMigrationRecovery(repeat) result = %#v", repeated)
	}
}

func TestManagerRollbackKeepsCleanedSourceAndRetainsProtectedRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(),
		"migration-cleaned-source-rollback",
		"provider-cleaned-source-rollback",
		"cleaned-source-rollback",
	)
	prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx,
		legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	if _, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256:                 candidate.SourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef:        committed.RecoveryCredentialRef,
		Confirmation:                 LegacyMigrationRollbackConfirmation,
	}); err != nil {
		t.Fatalf("BeginLegacyMigrationRollback() error = %v", err)
	}

	cleanedSource := bytes.Clone(legacyMigrationTestCleanedSource)
	defer clear(cleanedSource)
	command := CommitLegacyMigrationRollbackCommand{
		Expected:    expectedFor(registry.snapshot(), candidate.Provider.ID),
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256:                 candidate.SourceSHA256,
		VerifiedCleanedSourceSHA256:  candidate.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		VerifiedCleanedSource:        bytes.Clone(cleanedSource),
		RecoveryCredentialRef:        committed.RecoveryCredentialRef,
		Confirmation:                 LegacyMigrationRollbackCommitConfirmation,
	}
	proof, cleanup := legacyMigrationTestSourceAuthorityProof(
		t, ctx, manager, LegacyMigrationSourceAuthorityOperationRollbackCommit,
		candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
	)
	command.SourceAuthority = proof
	rolledBack, err := manager.CommitLegacyMigrationRollback(ctx, command)
	cleanup()
	if err != nil {
		t.Fatalf("CommitLegacyMigrationRollback(cleaned source) error = %v", err)
	}
	if rolledBack.Status != LegacyMigrationRollbackStatusCommittedRecoveryRetained {
		t.Fatalf("CommitLegacyMigrationRollback(cleaned source) result = %#v", rolledBack)
	}
	state := registry.snapshot()
	if _, exists := state.Providers[candidate.Provider.ID]; exists {
		t.Fatal("rollback retained migrated Provider as a Registry winner")
	}
	if !secrets.hasActive(committed.Provider.CredentialRef) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatal("rollback removed protected material before terminal Registry verification")
	}

	finalized, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, manager, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	)
	if err != nil {
		t.Fatalf("FinalizeLegacyMigrationRecovery(cleaned source) error = %v", err)
	}
	if finalized.Status != LegacyMigrationFinalizationStatusCompleted ||
		finalized.Outcome != LegacyMigrationFinalizationOutcomeRolledBack {
		t.Fatalf("FinalizeLegacyMigrationRecovery(cleaned source) result = %#v", finalized)
	}
	terminal := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
	if terminal.RecoveryCredentialRef != string(committed.RecoveryCredentialRef) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) ||
		secrets.exists(committed.Provider.CredentialRef) {
		t.Fatalf("rollback finalization did not retain only the protected recovery: %#v", terminal)
	}
	restarted := mustManager(t, registry, secrets, nil)
	repeated, err := restarted.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: candidate.SourceSHA256, SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef: committed.RecoveryCredentialRef, Confirmation: LegacyMigrationRollbackConfirmation,
	})
	if err != nil || repeated.Status != LegacyMigrationRollbackStatusRecoveryRetainedTerminal {
		t.Fatalf("BeginLegacyMigrationRollback(retained terminal restart) status = %s, error = %v", repeated.Status, err)
	}
	beforeMismatch := registry.snapshot()
	if _, err := restarted.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: strings.Repeat("f", 64), SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef: committed.RecoveryCredentialRef, Confirmation: LegacyMigrationRollbackConfirmation,
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("BeginLegacyMigrationRollback(retained terminal source mismatch) error = %v, want conflict", err)
	}
	if !reflect.DeepEqual(beforeMismatch, registry.snapshot()) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatal("retained terminal source mismatch changed protected recovery authority")
	}
	physicalSource, err := os.ReadFile(legacyMigrationTestPhysicalSourcePath(t, candidate))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(physicalSource)
	if !bytes.Equal(physicalSource, cleanedSource) {
		t.Fatal("rollback changed the cleaned ordinary settings source")
	}
}

func TestManagerExplicitlyRemigratesRetainedRecoveryAndDeletesItOnlyBySeparateProtectedAction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(), "migration-retained-remigration", "provider-retained-remigration",
		"source-retained-remigration",
	)
	prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: candidate.SourceSHA256, SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef: committed.RecoveryCredentialRef, Confirmation: LegacyMigrationRollbackConfirmation,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
		t, ctx, manager, registry.snapshot(), candidate, committed.RecoveryCredentialRef,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, manager, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); err != nil {
		t.Fatal(err)
	}
	ordinaryAlpha := connectMemoryProvider(
		t, ctx, manager, registry, "provider-retained-ordinary-alpha", "synthetic-retained-ordinary-alpha",
	)
	ordinaryBeta := connectMemoryProvider(
		t, ctx, manager, registry, "provider-retained-ordinary-beta", "synthetic-retained-ordinary-beta",
	)
	if _, err := manager.Select(ctx, SelectCommand{
		Expected: expectedFor(registry.snapshot(), ordinaryBeta.ID), ProviderID: ordinaryBeta.ID,
	}); err != nil {
		t.Fatalf("Select(after retained terminal) error = %v", err)
	}
	if _, err := manager.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot(after retained terminal mutation) error = %v", err)
	}
	if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
		t.Fatalf("Recover(after retained terminal mutation) error = %v", err)
	}
	afterOrdinaryMutation := registry.snapshot()
	retainedAfterOrdinaryMutation := afterOrdinaryMutation.LegacyMigrationRecoveries[candidate.MigrationID]
	if retainedAfterOrdinaryMutation.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained ||
		retainedAfterOrdinaryMutation.RecoveryCredentialRef != string(committed.RecoveryCredentialRef) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) ||
		afterOrdinaryMutation.SelectedProviderID != ordinaryBeta.ID {
		t.Fatalf("ordinary mutation changed retained recovery marker phase = %s", retainedAfterOrdinaryMutation.Phase)
	}
	if _, exists := afterOrdinaryMutation.Providers[ordinaryAlpha.ID]; !exists {
		t.Fatal("ordinary mutation lost the unrelated Provider after retained-terminal recovery")
	}
	for _, provider := range afterOrdinaryMutation.Providers {
		if provider.CredentialRef == string(committed.RecoveryCredentialRef) {
			t.Fatal("ordinary mutation activated retained recovery as Provider credential")
		}
	}
	cleanedBefore, err := os.ReadFile(legacyMigrationTestPhysicalSourcePath(t, candidate))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(cleanedBefore)
	remigrationCommand := RemigrateRetainedLegacyMigrationRecoveryCommand{
		Expected:    expectedFor(registry.snapshot(), candidate.Provider.ID),
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: candidate.SourceSHA256, VerifiedCleanedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		VerifiedCleanedSource:        bytes.Clone(legacyMigrationTestCleanedSource),
		RecoveryCredentialRef:        committed.RecoveryCredentialRef,
		Confirmation:                 LegacyMigrationRemigrationConfirmation,
	}
	beforeUnprovenRemigration := registry.snapshot()
	if _, err := manager.RemigrateRetainedLegacyMigrationRecovery(ctx, remigrationCommand); err == nil {
		t.Fatal("RemigrateRetainedLegacyMigrationRecovery(without live source authority) error = nil")
	}
	if !reflect.DeepEqual(beforeUnprovenRemigration, registry.snapshot()) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatal("unproven remigration changed protected authority")
	}

	remigrationProof, cleanupRemigration := legacyMigrationTestSourceAuthorityProof(
		t, ctx, manager, LegacyMigrationSourceAuthorityOperationRemigrate,
		candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
	)
	remigrationCommand.SourceAuthority = remigrationProof
	remigrated, err := manager.RemigrateRetainedLegacyMigrationRecovery(ctx, remigrationCommand)
	cleanupRemigration()
	if err != nil || remigrated.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
		remigrated.Provider.CredentialRef == "" ||
		remigrated.Provider.CredentialRef == string(committed.RecoveryCredentialRef) {
		t.Fatalf("RemigrateRetainedLegacyMigrationRecovery() = %#v, %v", remigrated, err)
	}
	if !secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatal("remigration removed retained protected recovery")
	}
	cleanedAfterRemigration, err := os.ReadFile(legacyMigrationTestPhysicalSourcePath(t, candidate))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(cleanedAfterRemigration)
	if !bytes.Equal(cleanedBefore, cleanedAfterRemigration) {
		t.Fatal("remigration changed the cleaned ordinary source")
	}
	beforeOrdinaryFinalize := registry.snapshot()
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, manager, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); err == nil {
		t.Fatal("FinalizeLegacyMigrationRecovery(remigrated retained recovery) error = nil")
	}
	if !reflect.DeepEqual(beforeOrdinaryFinalize, registry.snapshot()) ||
		!secrets.hasActive(string(committed.RecoveryCredentialRef)) {
		t.Fatal("ordinary finalization deleted or advanced remigrated protected recovery")
	}

	if _, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: candidate.SourceSHA256, SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef: committed.RecoveryCredentialRef, Confirmation: LegacyMigrationRollbackConfirmation,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
		t, ctx, manager, registry.snapshot(), candidate, committed.RecoveryCredentialRef,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, manager, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); err != nil {
		t.Fatal(err)
	}

	deleteProof, cleanupDelete := legacyMigrationTestSourceAuthorityProof(
		t, ctx, manager, LegacyMigrationSourceAuthorityOperationProtectedDelete,
		candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
	)
	deleted, err := manager.DeleteRetainedLegacyMigrationRecovery(ctx, DeleteRetainedLegacyMigrationRecoveryCommand{
		Expected:    expectedFor(registry.snapshot(), candidate.Provider.ID),
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: candidate.SourceSHA256, VerifiedCleanedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		VerifiedCleanedSource:        bytes.Clone(legacyMigrationTestCleanedSource), SourceAuthority: deleteProof,
		RecoveryCredentialRef: committed.RecoveryCredentialRef,
		Confirmation:          LegacyMigrationProtectedDeleteConfirmation,
	})
	cleanupDelete()
	if err != nil || deleted.Status != LegacyMigrationProtectedDeleteStatusCompleted {
		t.Fatalf("DeleteRetainedLegacyMigrationRecovery() = %#v, %v", deleted, err)
	}
	if secrets.exists(string(committed.RecoveryCredentialRef)) {
		t.Fatal("separate protected delete retained the recovery secret")
	}
	marker := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
	if marker.Phase != domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted ||
		marker.RecoveryCredentialRef != "" {
		t.Fatalf("protected delete terminal marker = %#v", marker)
	}
	postDelete := connectMemoryProvider(
		t, ctx, manager, registry, "provider-post-protected-delete", "synthetic-post-protected-delete",
	)
	if _, err := manager.Select(ctx, SelectCommand{
		Expected: expectedFor(registry.snapshot(), postDelete.ID), ProviderID: postDelete.ID,
	}); err != nil {
		t.Fatalf("Select(after protected recovery deletion) error = %v", err)
	}
	if _, err := manager.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot(after protected recovery deletion mutation) error = %v", err)
	}
	if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
		t.Fatalf("Recover(after protected recovery deletion mutation) error = %v", err)
	}
	deletedAfterOrdinaryMutation := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
	if deletedAfterOrdinaryMutation.Phase != domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted ||
		deletedAfterOrdinaryMutation.RecoveryCredentialRef != "" {
		t.Fatalf("ordinary mutation changed protected-delete marker phase = %s", deletedAfterOrdinaryMutation.Phase)
	}
	cleanedAfterDelete, err := os.ReadFile(legacyMigrationTestPhysicalSourcePath(t, candidate))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(cleanedAfterDelete)
	if !bytes.Equal(cleanedBefore, cleanedAfterDelete) {
		t.Fatal("protected delete changed the cleaned ordinary source")
	}
}

func TestManagerResumesExplicitRemigrationAndProtectedDeleteOnlyWithFreshSourceAuthority(t *testing.T) {
	t.Parallel()

	setupTerminal := func(t *testing.T, suffix string) (
		context.Context,
		*memoryRegistryStore,
		*memorySecretStore,
		LegacyMigrationCandidate,
		CommitVerifiedLegacyMigrationRecoveryResult,
	) {
		t.Helper()
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateWithSourceSnapshotFor(
			registry.snapshot(), "migration-explicit-resume-"+suffix,
			"provider-explicit-resume-"+suffix, "source-explicit-resume-"+suffix,
		)
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256: candidate.SourceSHA256, SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef: committed.RecoveryCredentialRef, Confirmation: LegacyMigrationRollbackConfirmation,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
			t, ctx, manager, registry.snapshot(), candidate, committed.RecoveryCredentialRef,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
			t, ctx, manager, candidate, committed.RecoveryCredentialRef,
			candidate.ExpectedCleanedSourceSHA256,
		); err != nil {
			t.Fatal(err)
		}
		return ctx, registry, secrets, candidate, committed
	}

	t.Run("remigration intent before provider transaction", func(t *testing.T) {
		ctx, registry, secrets, candidate, committed := setupTerminal(t, "remigration")
		interrupted := mustManager(t, registry, secrets, faultOnce(FaultBeforeMigrationProviderTransactionPrepared))
		proof, cleanup := legacyMigrationTestSourceAuthorityProof(
			t, ctx, interrupted, LegacyMigrationSourceAuthorityOperationRemigrate,
			candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
		)
		command := RemigrateRetainedLegacyMigrationRecoveryCommand{
			Expected:    expectedFor(registry.snapshot(), candidate.Provider.ID),
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256: candidate.SourceSHA256, VerifiedCleanedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
			SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
			VerifiedCleanedSource:        bytes.Clone(legacyMigrationTestCleanedSource), SourceAuthority: proof,
			RecoveryCredentialRef: committed.RecoveryCredentialRef, Confirmation: LegacyMigrationRemigrationConfirmation,
		}
		if _, err := interrupted.RemigrateRetainedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("RemigrateRetainedLegacyMigrationRecovery(interrupted) error = %v", err)
		}
		cleanup()
		pending := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
		if pending.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
			!pending.RemigrationPending || !secrets.hasActive(string(committed.RecoveryCredentialRef)) {
			t.Fatalf("interrupted remigration state = %#v", pending)
		}
		restarted := mustManager(t, registry, secrets, nil)
		if err := restarted.Recover(ctx); err != nil {
			t.Fatal(err)
		}
		if _, exists := registry.snapshot().Providers[candidate.Provider.ID]; exists {
			t.Fatal("Go-only restart advanced remigration without fresh source authority")
		}
		proof, cleanup = legacyMigrationTestSourceAuthorityProof(
			t, ctx, restarted, LegacyMigrationSourceAuthorityOperationRemigrate,
			candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
		)
		command.Expected = expectedFor(registry.snapshot(), candidate.Provider.ID)
		command.SourceAuthority = proof
		result, err := restarted.RemigrateRetainedLegacyMigrationRecovery(ctx, command)
		cleanup()
		if err != nil || result.Status != LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained {
			t.Fatalf("RemigrateRetainedLegacyMigrationRecovery(resume) = %#v, %v", result, err)
		}
	})

	for _, point := range []FaultPoint{
		FaultAfterMigrationProtectedDeleteIntent,
		FaultAfterMigrationProtectedDeleteTombstoned,
		FaultAfterMigrationProtectedDeleteDeleted,
		FaultAfterMigrationProtectedDeleteRecorded,
	} {
		point := point
		t.Run(string(point), func(t *testing.T) {
			ctx, registry, secrets, candidate, committed := setupTerminal(t, string(point))
			interrupted := mustManager(t, registry, secrets, faultOnce(point))
			proof, cleanup := legacyMigrationTestSourceAuthorityProof(
				t, ctx, interrupted, LegacyMigrationSourceAuthorityOperationProtectedDelete,
				candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
			)
			command := DeleteRetainedLegacyMigrationRecoveryCommand{
				Expected:    expectedFor(registry.snapshot(), candidate.Provider.ID),
				MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
				SourceSHA256: candidate.SourceSHA256, VerifiedCleanedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
				SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
				VerifiedCleanedSource:        bytes.Clone(legacyMigrationTestCleanedSource), SourceAuthority: proof,
				RecoveryCredentialRef: committed.RecoveryCredentialRef,
				Confirmation:          LegacyMigrationProtectedDeleteConfirmation,
			}
			if _, err := interrupted.DeleteRetainedLegacyMigrationRecovery(ctx, command); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("DeleteRetainedLegacyMigrationRecovery(interrupted) error = %v", err)
			}
			cleanup()
			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			if registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID].Phase !=
				domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
				proof, cleanup = legacyMigrationTestSourceAuthorityProof(
					t, ctx, restarted, LegacyMigrationSourceAuthorityOperationProtectedDelete,
					candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
				)
				command.Expected = expectedFor(registry.snapshot(), candidate.Provider.ID)
				command.SourceAuthority = proof
				result, err := restarted.DeleteRetainedLegacyMigrationRecovery(ctx, command)
				cleanup()
				if err != nil || result.Status != LegacyMigrationProtectedDeleteStatusCompleted {
					t.Fatalf("DeleteRetainedLegacyMigrationRecovery(resume) = %#v, %v", result, err)
				}
			}
			command.Expected = expectedFor(registry.snapshot(), candidate.Provider.ID)
			command.SourceAuthority = LegacyMigrationSourceAuthorityProof{}
			repeated, err := restarted.DeleteRetainedLegacyMigrationRecovery(ctx, command)
			if err != nil || repeated.Status != LegacyMigrationProtectedDeleteStatusAlreadyDeleted ||
				secrets.exists(string(committed.RecoveryCredentialRef)) {
				t.Fatalf("DeleteRetainedLegacyMigrationRecovery(repeat) = %#v, %v", repeated, err)
			}
		})
	}
}

func TestManagerRollsBackAllCommittedProvidersFromSharedLegacySourceInReverseOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	sharedSource := []byte(`{"provider":{"providers":[{"id":"provider-shared-alpha","apiKey":"synthetic-shared-alpha"},{"id":"provider-shared-beta","apiKey":"synthetic-shared-beta"}]}}`)
	sharedDigest := sha256.Sum256(sharedSource)
	sharedSourceSHA256 := hex.EncodeToString(sharedDigest[:])

	alpha := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-shared-alpha", "provider-shared-alpha", "shared-source",
		"synthetic-shared-alpha",
	)
	alpha.SourceSHA256 = sharedSourceSHA256
	alpha.SourceSnapshot = bytes.Clone(sharedSource)
	alpha.SourceLocator = "current:analytix-settings.json"
	alpha.ActiveCredentialLocators = []string{
		"current:analytix-settings.json:provider.providers[0].apiKey",
	}
	alphaRecovery, err := manager.PrepareLegacyMigrationRecovery(ctx, alpha)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery(alpha) error = %v", err)
	}
	alphaCommitted, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), alpha, alphaRecovery),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery(alpha) error = %v", err)
	}

	beta := legacyMigrationCandidateFor(
		registry.snapshot(), "migration-shared-beta", "provider-shared-beta", "shared-source",
		"synthetic-shared-beta",
	)
	beta.SourceSHA256 = sharedSourceSHA256
	beta.SourceSnapshot = bytes.Clone(sharedSource)
	beta.SourceLocator = "current:analytix-settings.json"
	beta.ActiveCredentialLocators = []string{
		"current:analytix-settings.json:provider.providers[1].apiKey",
	}
	legacyMigrationTestSharePhysicalSource(t, alpha, &beta)
	betaRecovery, err := manager.PrepareLegacyMigrationRecovery(ctx, beta)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery(beta) error = %v", err)
	}
	betaCommitted, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), beta, betaRecovery),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery(beta) error = %v", err)
	}
	committedState := registry.snapshot()
	if committedState.SelectedProviderID != alpha.Provider.ID || len(committedState.Providers) != 2 {
		t.Fatalf("shared committed state = %#v", committedState)
	}

	for _, current := range []struct {
		candidate LegacyMigrationCandidate
		committed CommitVerifiedLegacyMigrationRecoveryResult
	}{
		{candidate: alpha, committed: alphaCommitted},
		{candidate: beta, committed: betaCommitted},
	} {
		begin, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: current.candidate.MigrationID, SourceLocator: current.candidate.SourceLocator,
			SourceSHA256:                 sharedSourceSHA256,
			SourcePhysicalIdentitySHA256: current.candidate.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef:        current.committed.RecoveryCredentialRef,
			Confirmation:                 LegacyMigrationRollbackConfirmation,
		})
		if err != nil {
			t.Fatalf("BeginLegacyMigrationRollback(%s) error = %v", current.candidate.Provider.ID, err)
		}
		if begin.Status != LegacyMigrationRollbackStatusCleanedSourceRequired ||
			begin.MigrationID != current.candidate.MigrationID {
			t.Fatalf("BeginLegacyMigrationRollback(%s) result = %#v", current.candidate.Provider.ID, begin)
		}
	}
	outOfOrderState := registry.snapshot()
	if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
		t, ctx, manager, outOfOrderState, alpha, alphaCommitted.RecoveryCredentialRef,
	); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("CommitLegacyMigrationRollback(out of order) error = %v, want conflict", err)
	}
	if !reflect.DeepEqual(outOfOrderState, registry.snapshot()) {
		t.Fatal("out-of-order shared rollback changed Registry authority")
	}

	for _, current := range []struct {
		candidate LegacyMigrationCandidate
		committed CommitVerifiedLegacyMigrationRecoveryResult
	}{
		{candidate: beta, committed: betaCommitted},
		{candidate: alpha, committed: alphaCommitted},
	} {
		before := registry.snapshot()
		for attempt := 0; attempt < 2; attempt++ {
			rollback, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
				t, ctx, manager, before, current.candidate, current.committed.RecoveryCredentialRef,
			)
			if err != nil || rollback.Status != LegacyMigrationRollbackStatusCommittedRecoveryRetained {
				t.Fatalf("CommitLegacyMigrationRollback(%s, %d) = %#v, %v",
					current.candidate.Provider.ID, attempt+1, rollback, err)
			}
		}
	}
	rolledBackState := registry.snapshot()
	if rolledBackState.Revision != 4 || rolledBackState.SelectedProviderID != "" ||
		len(rolledBackState.Providers) != 0 {
		t.Fatalf("shared rolled-back state = %#v", rolledBackState)
	}
	for _, migrationID := range []string{alpha.MigrationID, beta.MigrationID} {
		recovery := rolledBackState.LegacyMigrationRecoveries[migrationID]
		if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained ||
			recovery.RollbackRegistryRevision != rolledBackState.Revision ||
			recovery.RollbackSelectedProviderID != rolledBackState.SelectedProviderID {
			t.Fatalf("shared rollback recovery %q = %#v", migrationID, recovery)
		}
	}

	for index, current := range []struct {
		candidate LegacyMigrationCandidate
		committed CommitVerifiedLegacyMigrationRecoveryResult
	}{
		{candidate: beta, committed: betaCommitted},
		{candidate: alpha, committed: alphaCommitted},
	} {
		var finalized FinalizeLegacyMigrationRecoveryResult
		var err error
		if index == 0 {
			finalized, err = legacyMigrationTestFinalizeWithLiveSourceAuthority(
				t, ctx, manager, current.candidate, current.committed.RecoveryCredentialRef,
				current.candidate.ExpectedCleanedSourceSHA256,
			)
		} else {
			finalized, err = manager.FinalizeLegacyMigrationRecovery(
				ctx,
				legacyMigrationTestFinalizeCommand(
					t, current.candidate, "", current.candidate.ExpectedCleanedSourceSHA256,
				),
			)
		}
		expectedStatus := LegacyMigrationFinalizationStatusCompleted
		if index > 0 {
			expectedStatus = LegacyMigrationFinalizationStatusAlreadyFinalized
		}
		if err != nil || finalized.Status != expectedStatus ||
			finalized.Outcome != LegacyMigrationFinalizationOutcomeRolledBack {
			t.Fatalf("FinalizeLegacyMigrationRecovery(%s) = %#v, %v",
				current.candidate.Provider.ID, finalized, err)
		}
	}
	for _, ref := range []string{alphaCommitted.Provider.CredentialRef, betaCommitted.Provider.CredentialRef} {
		if secrets.exists(ref) {
			t.Fatal("finalized shared rollback retained an inactive migrated winner")
		}
	}
	for _, ref := range []string{
		string(alphaCommitted.RecoveryCredentialRef), string(betaCommitted.RecoveryCredentialRef),
	} {
		if !secrets.hasActive(ref) {
			t.Fatal("finalized shared rollback removed retained protected recovery")
		}
	}
}

func TestManagerFinalizesSharedSourceRecoveriesAsOneCrashRecoverableDecision(t *testing.T) {
	t.Parallel()

	for _, point := range []FaultPoint{
		FaultAfterMigrationFinalizingRecorded,
		FaultAfterMigrationFinalizationRecoveryDeleteIntent,
		FaultAfterMigrationFinalizationRecoveryTombstoned,
		FaultAfterMigrationFinalizationRecoveryDeleted,
	} {
		point := point
		t.Run(string(point), func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			sharedSource := []byte(`{"provider":{"providers":[{"id":"provider-finalize-alpha","apiKey":"synthetic-finalize-alpha"},{"id":"provider-finalize-beta","apiKey":"synthetic-finalize-beta"}]}}`)
			digest := sha256.Sum256(sharedSource)
			sourceSHA256 := hex.EncodeToString(digest[:])
			candidates := []LegacyMigrationCandidate{
				legacyMigrationCandidateFor(
					registry.snapshot(), "migration-finalize-alpha", "provider-finalize-alpha", "shared-finalize",
					"synthetic-finalize-alpha",
				),
			}
			candidates[0].SourceSHA256 = sourceSHA256
			candidates[0].SourceSnapshot = bytes.Clone(sharedSource)
			candidates[0].SourceLocator = "current:analytix-settings.json"
			candidates[0].ActiveCredentialLocators = []string{
				"current:analytix-settings.json:provider.providers[0].apiKey",
			}
			recoveries := make([]CommitVerifiedLegacyMigrationRecoveryResult, 0, 2)
			prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidates[0])
			if err != nil {
				t.Fatal(err)
			}
			committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidates[0], prepared),
			)
			if err != nil {
				t.Fatal(err)
			}
			recoveries = append(recoveries, committed)

			beta := legacyMigrationCandidateFor(
				registry.snapshot(), "migration-finalize-beta", "provider-finalize-beta", "shared-finalize",
				"synthetic-finalize-beta",
			)
			beta.SourceSHA256 = sourceSHA256
			beta.SourceSnapshot = bytes.Clone(sharedSource)
			beta.SourceLocator = "current:analytix-settings.json"
			beta.ActiveCredentialLocators = []string{
				"current:analytix-settings.json:provider.providers[1].apiKey",
			}
			legacyMigrationTestSharePhysicalSource(t, candidates[0], &beta)
			candidates = append(candidates, beta)
			prepared, err = manager.PrepareLegacyMigrationRecovery(ctx, beta)
			if err != nil {
				t.Fatal(err)
			}
			committed, err = manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), beta, prepared),
			)
			if err != nil {
				t.Fatal(err)
			}
			recoveries = append(recoveries, committed)

			interrupted := mustManager(t, registry, secrets, faultOnce(point))
			if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
				t, ctx, interrupted, candidates[0], recoveries[0].RecoveryCredentialRef,
				candidates[0].ExpectedCleanedSourceSHA256,
			); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v, want interrupted", err)
			}
			decision := registry.snapshot()
			for _, candidate := range candidates {
				recovery := decision.LegacyMigrationRecoveries[candidate.MigrationID]
				if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted ||
					recovery.FinalizationOutcome != domainregistry.LegacyMigrationFinalizationOutcomeCommitted {
					t.Fatalf("shared finalization decision for %q = %#v", candidate.MigrationID, recovery)
				}
			}

			restarted := mustManager(t, registry, secrets, nil)
			_ = restarted.Recover(ctx)
			if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
				t, ctx, restarted, candidates[0], recoveries[0].RecoveryCredentialRef,
				candidates[0].ExpectedCleanedSourceSHA256,
			); err != nil {
				t.Fatalf("FinalizeLegacyMigrationRecovery(fresh live authority) error = %v", err)
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover(repeat) error = %v", err)
			}
			finalState := registry.snapshot()
			if len(finalState.Providers) != 2 {
				t.Fatalf("shared committed finalization removed Providers: %#v", finalState)
			}
			for index, candidate := range candidates {
				stored := finalState.LegacyMigrationRecoveries[candidate.MigrationID]
				if stored.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
					stored.FinalizationOutcome != domainregistry.LegacyMigrationFinalizationOutcomeCommitted ||
					!secrets.hasActive(recoveries[index].Provider.CredentialRef) ||
					secrets.exists(string(recoveries[index].RecoveryCredentialRef)) {
					t.Fatalf("shared finalized state for %q = %#v", candidate.MigrationID, finalState)
				}
				repeated, err := restarted.FinalizeLegacyMigrationRecovery(ctx, FinalizeLegacyMigrationRecoveryCommand{
					MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
					SourceSHA256: sourceSHA256, VerifiedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
					SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
					VerifiedSource: legacyMigrationTestSourceRepresentation(
						t, candidate, candidate.ExpectedCleanedSourceSHA256,
					),
					Confirmation: LegacyMigrationFinalizeConfirmation,
				})
				if err != nil || repeated.Status != LegacyMigrationFinalizationStatusAlreadyFinalized ||
					repeated.Outcome != LegacyMigrationFinalizationOutcomeCommitted {
					t.Fatalf("FinalizeLegacyMigrationRecovery(repeat %q) = %#v, %v",
						candidate.MigrationID, repeated, err)
				}
			}
		})
	}
}

type countingLegacySourceReader struct {
	next  registryport.LegacySourceReader
	reads int
}

func (reader *countingLegacySourceReader) ReadLegacySource(request registryport.LegacySourceRequest) (registryport.LegacySourceSnapshot, error) {
	reader.reads++
	return reader.next.ReadLegacySource(request)
}

func TestManagerRequiresProductionOwnerSourceReceiptBeforeMigrationEffects(t *testing.T) {
	t.Parallel()

	type finalizationFixture struct {
		ctx       context.Context
		registry  *memoryRegistryStore
		secrets   *memorySecretStore
		manager   *Manager
		candidate LegacyMigrationCandidate
		committed CommitVerifiedLegacyMigrationRecoveryResult
	}
	newFinalizationFixture := func(t *testing.T, suffix string) finalizationFixture {
		t.Helper()
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateWithSourceSnapshotFor(
			registry.snapshot(), "migration-live-source-authority-"+suffix,
			"provider-live-source-authority-"+suffix, "live-source-authority-"+suffix,
		)
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		return finalizationFixture{
			ctx: ctx, registry: registry, secrets: secrets, manager: manager,
			candidate: candidate, committed: committed,
		}
	}
	t.Run("unissued challenge rejects before read and issued finalization reads once", func(t *testing.T) {
		current := newFinalizationFixture(t, "unissued-read-count")
		reader := &countingLegacySourceReader{next: current.manager.legacySource}
		current.manager.legacySource = reader
		proof, cleanup := legacyMigrationTestSourceAuthorityProof(t, current.ctx, current.manager,
			LegacyMigrationSourceAuthorityOperationFinalize, current.candidate, current.committed.RecoveryCredentialRef,
			current.candidate.ExpectedCleanedSourceSHA256)
		defer cleanup()
		command := legacyMigrationTestFinalizeCommand(t, current.candidate, current.committed.RecoveryCredentialRef,
			current.candidate.ExpectedCleanedSourceSHA256)
		command.SourceAuthority = proof
		command.SourceAuthority.Challenge = legacyMigrationSourceAuthorityPrefix + strings.Repeat("X", 43)
		if _, err := current.manager.FinalizeLegacyMigrationRecovery(current.ctx, command); !errors.Is(err, registryport.ErrVerification) || reader.reads != 0 {
			t.Fatalf("unissued finalization challenge must fail before source read: err=%v reads=%d", err, reader.reads)
		}
		command.SourceAuthority = proof
		if _, err := current.manager.FinalizeLegacyMigrationRecovery(current.ctx, command); err != nil || reader.reads != 1 {
			t.Fatalf("issued finalization must perform one physical read and succeed: err=%v reads=%d", err, reader.reads)
		}
	})
	t.Run("finalization challenge cannot authorize rollback source read", func(t *testing.T) {
		current := newFinalizationFixture(t, "cross-operation-read-count")
		reader := &countingLegacySourceReader{next: current.manager.legacySource}
		current.manager.legacySource = reader
		proof, cleanup := legacyMigrationTestSourceAuthorityProof(t, current.ctx, current.manager,
			LegacyMigrationSourceAuthorityOperationFinalize, current.candidate, current.committed.RecoveryCredentialRef,
			current.candidate.ExpectedCleanedSourceSHA256)
		defer cleanup()
		if _, err := current.manager.BeginLegacyMigrationRollback(current.ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: current.candidate.MigrationID, SourceLocator: current.candidate.SourceLocator,
			SourceSHA256: current.candidate.SourceSHA256, RecoveryCredentialRef: current.committed.RecoveryCredentialRef,
			Confirmation: LegacyMigrationRollbackConfirmation,
		}); err != nil {
			t.Fatal(err)
		}
		command := legacyMigrationTestRollbackCommitCommand(current.registry.snapshot(), current.candidate, current.committed.RecoveryCredentialRef)
		command.SourceAuthority = proof
		if _, err := current.manager.CommitLegacyMigrationRollback(current.ctx, command); !errors.Is(err, registryport.ErrVerification) || reader.reads != 0 {
			t.Fatalf("cross-operation challenge must fail before source read: err=%v reads=%d", err, reader.reads)
		}
		if _, exists := current.manager.sourceAuthorityChallenges[proof.Challenge]; exists {
			t.Fatal("cross-operation request did not reach challenge consumption")
		}
		cleanup()
		proof, cleanupRollback := legacyMigrationTestSourceAuthorityProof(t, current.ctx, current.manager,
			LegacyMigrationSourceAuthorityOperationRollbackCommit, current.candidate, current.committed.RecoveryCredentialRef,
			current.candidate.ExpectedCleanedSourceSHA256)
		defer cleanupRollback()
		command.SourceAuthority = proof
		if _, err := current.manager.CommitLegacyMigrationRollback(current.ctx, command); err != nil || reader.reads != 1 {
			t.Fatalf("issued rollback must perform one physical read and succeed: err=%v reads=%d", err, reader.reads)
		}
	})

	t.Run("finalization rejects a complete but stale caller receipt without live source authority", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateWithSourceSnapshotFor(
			registry.snapshot(), "migration-stale-source-authority-finalize",
			"provider-stale-source-authority-finalize", "stale-source-authority-finalize",
		)
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		before := registry.snapshot()
		_, beforeTombstones, beforeDeletes := secrets.callCounts()
		command := legacyMigrationTestFinalizeCommand(
			t, candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
		)
		if _, err := manager.FinalizeLegacyMigrationRecovery(ctx, command); err == nil {
			t.Fatal("FinalizeLegacyMigrationRecovery(stale caller receipt) error = nil")
		}
		_, afterTombstones, afterDeletes := secrets.callCounts()
		if !reflect.DeepEqual(before, registry.snapshot()) || beforeTombstones != afterTombstones ||
			beforeDeletes != afterDeletes || !secrets.exists(string(committed.RecoveryCredentialRef)) {
			t.Fatal("stale caller receipt advanced or deleted protected recovery")
		}
	})

	t.Run("rollback commit rejects a complete but stale caller receipt without live source authority", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateWithSourceSnapshotFor(
			registry.snapshot(), "migration-stale-source-authority-rollback",
			"provider-stale-source-authority-rollback", "stale-source-authority-rollback",
		)
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256: candidate.SourceSHA256, RecoveryCredentialRef: committed.RecoveryCredentialRef,
			Confirmation: LegacyMigrationRollbackConfirmation,
		}); err != nil {
			t.Fatal(err)
		}
		before := registry.snapshot()
		command := legacyMigrationTestRollbackCommitCommand(before, candidate, committed.RecoveryCredentialRef)
		if _, err := manager.CommitLegacyMigrationRollback(ctx, command); err == nil {
			t.Fatal("CommitLegacyMigrationRollback(stale caller receipt) error = nil")
		}
		if !reflect.DeepEqual(before, registry.snapshot()) ||
			!secrets.hasActive(committed.Provider.CredentialRef) ||
			!secrets.exists(string(committed.RecoveryCredentialRef)) {
			t.Fatal("stale caller receipt advanced Registry or protected material")
		}
	})

	t.Run("finalization rejects digest-only authority", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateWithSourceSnapshotFor(
			registry.snapshot(), "migration-source-receipt-finalize",
			"provider-source-receipt-finalize", "source-receipt-finalize",
		)
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		before := registry.snapshot()
		_, beforeTombstones, beforeDeletes := secrets.callCounts()
		if _, err := manager.FinalizeLegacyMigrationRecovery(ctx, FinalizeLegacyMigrationRecoveryCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256: candidate.SourceSHA256, VerifiedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
			RecoveryCredentialRef: committed.RecoveryCredentialRef,
			Confirmation:          LegacyMigrationFinalizeConfirmation,
		}); err == nil {
			t.Fatal("FinalizeLegacyMigrationRecovery(digest-only source authority) error = nil")
		}
		_, afterTombstones, afterDeletes := secrets.callCounts()
		if !reflect.DeepEqual(before, registry.snapshot()) || beforeTombstones != afterTombstones ||
			beforeDeletes != afterDeletes || !secrets.exists(string(committed.RecoveryCredentialRef)) {
			t.Fatal("digest-only finalization advanced or deleted protected recovery")
		}
	})

	t.Run("rollback commit rejects digest-only authority", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		candidate := legacyMigrationCandidateWithSourceSnapshotFor(
			registry.snapshot(), "migration-source-receipt-rollback",
			"provider-source-receipt-rollback", "source-receipt-rollback",
		)
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256: candidate.SourceSHA256, RecoveryCredentialRef: committed.RecoveryCredentialRef,
			Confirmation: LegacyMigrationRollbackConfirmation,
		})
		if err != nil {
			t.Fatal(err)
		}
		before := registry.snapshot()
		if _, err := manager.CommitLegacyMigrationRollback(ctx, CommitLegacyMigrationRollbackCommand{
			Expected: expectedFor(before, candidate.Provider.ID), MigrationID: candidate.MigrationID,
			SourceLocator: candidate.SourceLocator, SourceSHA256: candidate.SourceSHA256,
			VerifiedCleanedSourceSHA256: candidate.SourceSHA256,
			RecoveryCredentialRef:       committed.RecoveryCredentialRef,
			Confirmation:                LegacyMigrationRollbackCommitConfirmation,
		}); err == nil {
			t.Fatal("CommitLegacyMigrationRollback(digest-only source authority) error = nil")
		}
		if !reflect.DeepEqual(before, registry.snapshot()) ||
			!secrets.hasActive(committed.Provider.CredentialRef) ||
			!secrets.exists(string(committed.RecoveryCredentialRef)) {
			t.Fatal("digest-only rollback commit advanced Registry or protected material")
		}
	})

	for _, testCase := range []struct {
		name   string
		id     string
		mutate func(*testing.T, string, []byte) func()
	}{
		{
			name: "same representation atomic replacement changes the physical generation",
			id:   "inode-replacement",
			mutate: func(t *testing.T, sourcePath string, currentSource []byte) func() {
				t.Helper()
				replacementPath := sourcePath + ".replacement"
				if err := os.WriteFile(replacementPath, currentSource, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacementPath, sourcePath); err != nil {
					t.Fatal(err)
				}
				return func() {}
			},
		},
		{
			name: "same inode source digest drift",
			id:   "digest-drift",
			mutate: func(t *testing.T, sourcePath string, _ []byte) func() {
				t.Helper()
				if err := os.WriteFile(sourcePath, []byte(`{"provider":{"drift":true}}`), 0o600); err != nil {
					t.Fatal(err)
				}
				return func() {}
			},
		},
		{
			name: "symlink source alias",
			id:   "symlink-alias",
			mutate: func(t *testing.T, sourcePath string, _ []byte) func() {
				t.Helper()
				peerPath := sourcePath + ".peer"
				if err := os.Rename(sourcePath, peerPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(peerPath, sourcePath); err != nil {
					t.Fatal(err)
				}
				return func() {
					_ = os.Remove(sourcePath)
				}
			},
		},
		{
			name: "hardlink source peer",
			id:   "hardlink-peer",
			mutate: func(t *testing.T, sourcePath string, _ []byte) func() {
				t.Helper()
				peerPath := sourcePath + ".peer"
				if err := os.Link(sourcePath, peerPath); err != nil {
					t.Fatal(err)
				}
				return func() { _ = os.Remove(peerPath) }
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			current := newFinalizationFixture(t, testCase.id)
			command := legacyMigrationTestFinalizeCommand(
				t, current.candidate, current.committed.RecoveryCredentialRef,
				current.candidate.ExpectedCleanedSourceSHA256,
			)
			proof, cleanupAuthority := legacyMigrationTestSourceAuthorityProof(
				t, current.ctx, current.manager, LegacyMigrationSourceAuthorityOperationFinalize,
				current.candidate, current.committed.RecoveryCredentialRef,
				current.candidate.ExpectedCleanedSourceSHA256,
			)
			defer cleanupAuthority()
			command.SourceAuthority = proof
			currentSource := legacyMigrationTestSourceRepresentation(
				t, current.candidate, current.candidate.ExpectedCleanedSourceSHA256,
			)
			defer clear(currentSource)
			cleanupMutation := testCase.mutate(
				t, legacyMigrationTestPhysicalSourcePath(t, current.candidate), currentSource,
			)
			defer cleanupMutation()
			before := current.registry.snapshot()
			_, beforeTombstones, beforeDeletes := current.secrets.callCounts()
			if _, err := current.manager.FinalizeLegacyMigrationRecovery(current.ctx, command); err == nil {
				t.Fatal("FinalizeLegacyMigrationRecovery(invalid physical source authority) error = nil")
			}
			_, afterTombstones, afterDeletes := current.secrets.callCounts()
			if !reflect.DeepEqual(before, current.registry.snapshot()) ||
				beforeTombstones != afterTombstones || beforeDeletes != afterDeletes ||
				!current.secrets.exists(string(current.committed.RecoveryCredentialRef)) {
				t.Fatal("invalid physical source authority advanced or deleted protected recovery")
			}
		})
	}

	t.Run("consumed challenge cannot be replayed after the exact source returns", func(t *testing.T) {
		current := newFinalizationFixture(t, "consumed-challenge-replay")
		reader := &countingLegacySourceReader{next: current.manager.legacySource}
		current.manager.legacySource = reader
		command := legacyMigrationTestFinalizeCommand(
			t, current.candidate, current.committed.RecoveryCredentialRef,
			current.candidate.ExpectedCleanedSourceSHA256,
		)
		proof, cleanupAuthority := legacyMigrationTestSourceAuthorityProof(
			t, current.ctx, current.manager, LegacyMigrationSourceAuthorityOperationFinalize,
			current.candidate, current.committed.RecoveryCredentialRef,
			current.candidate.ExpectedCleanedSourceSHA256,
		)
		defer cleanupAuthority()
		command.SourceAuthority = proof
		sourcePath := legacyMigrationTestPhysicalSourcePath(t, current.candidate)
		heldPath := sourcePath + ".held"
		if err := os.Rename(sourcePath, heldPath); err != nil {
			t.Fatal(err)
		}
		if _, err := current.manager.FinalizeLegacyMigrationRecovery(current.ctx, command); err == nil {
			t.Fatal("FinalizeLegacyMigrationRecovery(missing source) error = nil")
		}
		if reader.reads != 1 {
			t.Fatalf("issued challenge must reach the physical reader once: reads=%d", reader.reads)
		}
		if err := os.Rename(heldPath, sourcePath); err != nil {
			t.Fatal(err)
		}
		before := current.registry.snapshot()
		_, beforeTombstones, beforeDeletes := current.secrets.callCounts()
		if _, err := current.manager.FinalizeLegacyMigrationRecovery(current.ctx, command); err == nil {
			t.Fatal("FinalizeLegacyMigrationRecovery(replayed challenge) error = nil")
		}
		if reader.reads != 1 {
			t.Fatalf("replayed challenge performed another source read: reads=%d", reader.reads)
		}
		_, afterTombstones, afterDeletes := current.secrets.callCounts()
		if !reflect.DeepEqual(before, current.registry.snapshot()) ||
			beforeTombstones != afterTombstones || beforeDeletes != afterDeletes ||
			!current.secrets.exists(string(current.committed.RecoveryCredentialRef)) {
			t.Fatal("replayed challenge advanced or deleted protected recovery")
		}
	})
}

func TestManagerFreshRestartLeavesFinalizingMigrationPendingWithoutLiveSourceAuthority(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(), "migration-restart-live-source-authority",
		"provider-restart-live-source-authority", "restart-live-source-authority",
	)
	prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationFinalizingRecorded))
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, interrupted, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v, want interrupted", err)
	}
	before := registry.snapshot()
	_, beforeTombstones, beforeDeletes := secrets.callCounts()
	restarted := mustManager(t, registry, secrets, nil)
	if err := restarted.Recover(ctx); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	_, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(before, registry.snapshot()) || beforeTombstones != afterTombstones ||
		beforeDeletes != afterDeletes || !secrets.exists(string(committed.RecoveryCredentialRef)) {
		t.Fatal("fresh Go restart performed a destructive finalization effect without live source authority")
	}
	inventory, err := restarted.InventoryLegacyMigrationRecoveries(ctx)
	if err != nil {
		t.Fatalf("InventoryLegacyMigrationRecoveries() error = %v", err)
	}
	if len(inventory.Recoveries) != 1 || inventory.Recoveries[0].MigrationID != candidate.MigrationID ||
		inventory.Recoveries[0].Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted {
		t.Fatalf("pending inventory = %#v", inventory.Recoveries)
	}
}

func TestManagerRecoveryRejectsIncoherentFinalizedSiblingBeforeFurtherGroupDeletion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	sourceLocator := "current:analytix-settings.json"
	sourceSnapshot := []byte(`{"provider":{"providers":[{"id":"provider-finalized-sibling-a","apiKey":"synthetic-finalized-sibling-a"},{"id":"provider-finalized-sibling-b","apiKey":"synthetic-finalized-sibling-b"}]}}`)
	digest := sha256.Sum256(sourceSnapshot)
	sourceSHA256 := hex.EncodeToString(digest[:])
	candidates := make([]LegacyMigrationCandidate, 0, 2)
	committed := make([]CommitVerifiedLegacyMigrationRecoveryResult, 0, 2)
	for index := 0; index < 2; index++ {
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), fmt.Sprintf("migration-finalized-sibling-%d", index),
			fmt.Sprintf("provider-finalized-sibling-%c", 'a'+rune(index)), "finalized-sibling",
			fmt.Sprintf("synthetic-finalized-sibling-%d", index),
		)
		candidate.SourceLocator = sourceLocator
		candidate.SourceSHA256 = sourceSHA256
		candidate.SourceSnapshot = bytes.Clone(sourceSnapshot)
		candidate.ActiveCredentialLocators = []string{
			fmt.Sprintf("%s:provider.providers[%d].apiKey", sourceLocator, index),
		}
		if index > 0 {
			legacyMigrationTestSharePhysicalSource(t, candidates[0], &candidate)
		}
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, candidate)
		committed = append(committed, result)
	}
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationFinalizedRecorded))
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, interrupted, candidates[0], committed[0].RecoveryCredentialRef,
		candidates[0].ExpectedCleanedSourceSHA256,
	); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v, want interrupted", err)
	}
	var finalizedID string
	for id, recovery := range registry.snapshot().LegacyMigrationRecoveries {
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized {
			finalizedID = id
		}
	}
	if finalizedID == "" {
		t.Fatal("fault did not leave one finalized sibling")
	}
	registry.mutate(func(state *domainregistry.Registry) {
		recovery := state.LegacyMigrationRecoveries[finalizedID]
		recovery.FinalizedSourceStateIdentitySHA256 = strings.Repeat("f", 64)
		state.LegacyMigrationRecoveries[finalizedID] = recovery
	})
	before := registry.snapshot()
	_, beforeTombstones, beforeDeletes := secrets.callCounts()
	restarted := mustManager(t, registry, secrets, nil)
	_ = restarted.Recover(ctx)
	pendingIndex := 0
	if candidates[0].MigrationID == finalizedID {
		pendingIndex = 1
	}
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, restarted, candidates[pendingIndex], committed[pendingIndex].RecoveryCredentialRef,
		candidates[pendingIndex].ExpectedCleanedSourceSHA256,
	); err == nil {
		t.Fatal("FinalizeLegacyMigrationRecovery(incoherent finalized sibling) error = nil")
	}
	_, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(before, registry.snapshot()) || beforeTombstones != afterTombstones ||
		beforeDeletes != afterDeletes {
		t.Fatalf("restart advanced after finalized sibling drift: cleanup=%d/%d->%d/%d",
			beforeTombstones, beforeDeletes, afterTombstones, afterDeletes)
	}
}

func TestManagerKeepsIdenticalLegacyMigrationBytesDistinctByLogicalSourceLocator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	sharedSource := []byte(`{"provider":{"apiKey":"synthetic-identical-source"}}`)
	digest := sha256.Sum256(sharedSource)
	sourceSHA256 := hex.EncodeToString(digest[:])
	type committedSource struct {
		candidate LegacyMigrationCandidate
		result    CommitVerifiedLegacyMigrationRecoveryResult
	}
	committed := make([]committedSource, 0, 2)
	for index, sourceLocator := range []string{
		"current:analytix-settings.json",
		"compatibility:00:analytix-settings.json",
	} {
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), fmt.Sprintf("migration-identical-source-%d", index),
			fmt.Sprintf("provider-identical-source-%d", index), "identical-source",
			fmt.Sprintf("synthetic-identical-source-%d", index),
		)
		candidate.SourceLocator = sourceLocator
		candidate.SourceSHA256 = sourceSHA256
		candidate.SourceSnapshot = bytes.Clone(sharedSource)
		candidate.ActiveCredentialLocators = []string{fmt.Sprintf("%s:provider.apiKey", sourceLocator)}
		legacyMigrationTestBindPhysicalSource(&candidate)
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatalf("PrepareLegacyMigrationRecovery(%d) error = %v", index, err)
		}
		result, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
		)
		if err != nil {
			t.Fatalf("CommitVerifiedLegacyMigrationRecovery(%d) error = %v", index, err)
		}
		committed = append(committed, committedSource{candidate: candidate, result: result})
	}

	finalized, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, manager, committed[0].candidate, committed[0].result.RecoveryCredentialRef,
		committed[0].candidate.ExpectedCleanedSourceSHA256,
	)
	if err != nil || finalized.Status != LegacyMigrationFinalizationStatusCompleted {
		t.Fatalf("FinalizeLegacyMigrationRecovery(current source) = %#v, %v", finalized, err)
	}
	state := registry.snapshot()
	if state.LegacyMigrationRecoveries[committed[0].candidate.MigrationID].Phase !=
		domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
		state.LegacyMigrationRecoveries[committed[1].candidate.MigrationID].Phase !=
			domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		!secrets.hasActive(string(committed[1].result.RecoveryCredentialRef)) {
		t.Fatalf("finalizing one logical source changed identical-byte peer: %#v", state)
	}

	begin, err := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID:           committed[1].candidate.MigrationID,
		SourceLocator:         committed[1].candidate.SourceLocator,
		SourceSHA256:          committed[1].candidate.SourceSHA256,
		RecoveryCredentialRef: committed[1].result.RecoveryCredentialRef,
		Confirmation:          LegacyMigrationRollbackConfirmation,
	})
	if err != nil || begin.Status != LegacyMigrationRollbackStatusCleanedSourceRequired {
		t.Fatalf("BeginLegacyMigrationRollback(compatibility source) = %#v, %v", begin, err)
	}
	beforeRollback := registry.snapshot()
	if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
		t, ctx, manager, beforeRollback, committed[1].candidate,
		committed[1].result.RecoveryCredentialRef,
	); err != nil {
		t.Fatalf("CommitLegacyMigrationRollback(compatibility source) error = %v", err)
	}
	state = registry.snapshot()
	if state.LegacyMigrationRecoveries[committed[0].candidate.MigrationID].Phase !=
		domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
		state.LegacyMigrationRecoveries[committed[1].candidate.MigrationID].Phase !=
			domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained {
		t.Fatalf("rolling back compatibility source changed current-source marker: %#v", state)
	}
}

func TestManagerFinalizesCommittedLegacyMigrationOnlyAfterExactWinnerReadback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(), "migration-finalize-committed", "provider-finalize-committed", "source-finalize-committed",
	)
	recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
	)
	if err != nil {
		t.Fatalf("CommitVerifiedLegacyMigrationRecovery() error = %v", err)
	}
	winnerRef := committed.Provider.CredentialRef
	finalized, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, manager, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	)
	if err != nil {
		t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v", err)
	}
	if finalized.Status != LegacyMigrationFinalizationStatusCompleted ||
		finalized.Outcome != LegacyMigrationFinalizationOutcomeCommitted ||
		!secrets.hasActive(winnerRef) || secrets.exists(string(committed.RecoveryCredentialRef)) {
		t.Fatalf("committed finalization = %#v winnerActive=%t recoveryExists=%t",
			finalized, secrets.hasActive(winnerRef), secrets.exists(string(committed.RecoveryCredentialRef)))
	}
	state := registry.snapshot()
	provider, exists := state.Providers[candidate.Provider.ID]
	marker := state.LegacyMigrationRecoveries[candidate.MigrationID]
	if !exists || provider.CredentialRef != winnerRef ||
		marker.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
		marker.RecoveryCredentialRef != "" || marker.CommittedProviderCredentialRef != "" ||
		marker.FinalizationOutcome != domainregistry.LegacyMigrationFinalizationOutcomeCommitted {
		t.Fatalf("committed finalized state = %#v", state)
	}
	repeated, err := manager.FinalizeLegacyMigrationRecovery(ctx, FinalizeLegacyMigrationRecoveryCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: candidate.SourceSHA256, VerifiedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		VerifiedSource: legacyMigrationTestSourceRepresentation(
			t, candidate, candidate.ExpectedCleanedSourceSHA256,
		),
		Confirmation: LegacyMigrationFinalizeConfirmation,
	})
	if err != nil || repeated.Status != LegacyMigrationFinalizationStatusAlreadyFinalized ||
		repeated.Outcome != LegacyMigrationFinalizationOutcomeCommitted {
		t.Fatalf("FinalizeLegacyMigrationRecovery(repeat) = %#v, %v", repeated, err)
	}
}

func TestManagerLegacyMigrationRollbackAndFinalizationFailClosedOnIdentityDrift(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		mutate func(*memorySecretStore, domainregistry.Registry, CommitVerifiedLegacyMigrationRecoveryResult)
	}{
		{
			name: "recovery payload",
			mutate: func(secrets *memorySecretStore, _ domainregistry.Registry, committed CommitVerifiedLegacyMigrationRecoveryResult) {
				secrets.tamper(string(committed.RecoveryCredentialRef))
			},
		},
		{
			name: "winner readback",
			mutate: func(secrets *memorySecretStore, _ domainregistry.Registry, committed CommitVerifiedLegacyMigrationRecoveryResult) {
				secrets.tamper(committed.Provider.CredentialRef)
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationCandidateWithSourceSnapshotFor(
				registry.snapshot(), "migration-drift-"+strings.ReplaceAll(testCase.name, " ", "-"),
				"provider-drift-"+strings.ReplaceAll(testCase.name, " ", "-"), "source-drift",
			)
			recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatal(err)
			}
			committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
			)
			if err != nil {
				t.Fatal(err)
			}
			before := registry.snapshot()
			_, beforeTombstones, beforeDeletes := secrets.callCounts()
			testCase.mutate(secrets, before, committed)
			_, err = manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
				MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
				SourceSHA256:          candidate.SourceSHA256,
				RecoveryCredentialRef: committed.RecoveryCredentialRef,
				Confirmation:          LegacyMigrationRollbackConfirmation,
			})
			if err == nil {
				t.Fatal("BeginLegacyMigrationRollback(identity drift) error = nil")
			}
			_, afterTombstones, afterDeletes := secrets.callCounts()
			if !reflect.DeepEqual(before, registry.snapshot()) || afterTombstones != beforeTombstones ||
				afterDeletes != beforeDeletes {
				t.Fatalf("identity drift changed authority: before=%#v after=%#v cleanup=%d/%d->%d/%d",
					before, registry.snapshot(), beforeTombstones, beforeDeletes, afterTombstones, afterDeletes)
			}
		})
	}
}

func TestManagerRecoveryReverifiesProtectedAuthorityBeforeResumedLegacyMigrationFinalization(t *testing.T) {
	t.Parallel()

	for _, outcome := range []string{"committed", "rolled-back"} {
		outcome := outcome
		for _, target := range []string{"recovery", "winner"} {
			target := target
			for _, mutation := range []string{"missing", "tampered"} {
				mutation := mutation
				t.Run(outcome+"/"+target+"/"+mutation, func(t *testing.T) {
					t.Parallel()
					ctx := context.Background()
					registry := newMemoryRegistryStore()
					secrets := newMemorySecretStore()
					manager := mustManager(t, registry, secrets, nil)
					candidate := legacyMigrationCandidateWithSourceSnapshotFor(
						registry.snapshot(),
						"migration-finalize-reverify-"+outcome+"-"+target+"-"+mutation,
						"provider-finalize-reverify-"+outcome+"-"+target+"-"+mutation,
						"source-finalize-reverify",
					)
					recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
					if err != nil {
						t.Fatal(err)
					}
					committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
						ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
					)
					if err != nil {
						t.Fatal(err)
					}
					if outcome == "rolled-back" {
						_, err = manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
							MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
							SourceSHA256: candidate.SourceSHA256, RecoveryCredentialRef: committed.RecoveryCredentialRef,
							Confirmation: LegacyMigrationRollbackConfirmation,
						})
						if err != nil {
							t.Fatal(err)
						}
						before := registry.snapshot()
						if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
							t, ctx, manager, before, candidate, committed.RecoveryCredentialRef,
						); err != nil {
							t.Fatal(err)
						}
					}

					verifiedSourceSHA256 := candidate.ExpectedCleanedSourceSHA256
					interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationFinalizingRecorded))
					if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
						t, ctx, interrupted, candidate, committed.RecoveryCredentialRef,
						verifiedSourceSHA256,
					); !errors.Is(err, ErrInterrupted) {
						t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v, want interrupted", err)
					}
					before := registry.snapshot()
					_, beforeTombstones, beforeDeletes := secrets.callCounts()
					ref := string(committed.RecoveryCredentialRef)
					if target == "winner" {
						ref = committed.Provider.CredentialRef
					}
					if mutation == "missing" {
						secrets.remove(ref)
					} else {
						secrets.tamper(ref)
					}

					restarted := mustManager(t, registry, secrets, nil)
					_ = restarted.Recover(ctx)
					if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
						t, ctx, restarted, candidate, committed.RecoveryCredentialRef,
						verifiedSourceSHA256,
					); err == nil {
						t.Fatal("FinalizeLegacyMigrationRecovery(authority drift) error = nil")
					}
					_, afterTombstones, afterDeletes := secrets.callCounts()
					if !reflect.DeepEqual(before, registry.snapshot()) ||
						afterTombstones != beforeTombstones || afterDeletes != beforeDeletes {
						t.Fatalf("resumed finalization changed authority after %s %s drift: before=%#v after=%#v cleanup=%d/%d->%d/%d",
							target, mutation, before, registry.snapshot(), beforeTombstones, beforeDeletes,
							afterTombstones, afterDeletes)
					}
				})
			}
		}
	}
}

func TestManagerRecoveryPreflightsWholeLegacyMigrationSourceGroupBeforeDeletingAnyRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	sourceLocator := "current:analytix-settings.json"
	sourceSnapshot := []byte(`{"provider":{"apiKey":"synthetic-group-finalization-reverify"}}`)
	digest := sha256.Sum256(sourceSnapshot)
	sourceSHA256 := hex.EncodeToString(digest[:])
	committed := make([]CommitVerifiedLegacyMigrationRecoveryResult, 0, 2)
	candidates := make([]LegacyMigrationCandidate, 0, 2)
	expectedCleanedSourceSHA256 := ""
	for index := 0; index < 2; index++ {
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), fmt.Sprintf("migration-group-finalization-reverify-%d", index),
			fmt.Sprintf("provider-group-finalization-reverify-%d", index), "group-finalization-reverify",
			fmt.Sprintf("synthetic-group-finalization-reverify-%d", index),
		)
		candidate.SourceLocator = sourceLocator
		candidate.SourceSnapshot = bytes.Clone(sourceSnapshot)
		candidate.SourceSHA256 = sourceSHA256
		candidate.ActiveCredentialLocators = []string{fmt.Sprintf("%s:provider.providers[%d].apiKey", sourceLocator, index)}
		if index > 0 {
			legacyMigrationTestSharePhysicalSource(t, candidates[0], &candidate)
		}
		if expectedCleanedSourceSHA256 == "" {
			expectedCleanedSourceSHA256 = candidate.ExpectedCleanedSourceSHA256
		}
		recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
		)
		if err != nil {
			t.Fatal(err)
		}
		committed = append(committed, result)
		candidates = append(candidates, candidate)
	}
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationFinalizingRecorded))
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, interrupted, candidates[0], committed[0].RecoveryCredentialRef,
		expectedCleanedSourceSHA256,
	); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("FinalizeLegacyMigrationRecovery(group) error = %v, want interrupted", err)
	}
	before := registry.snapshot()
	_, beforeTombstones, beforeDeletes := secrets.callCounts()
	secrets.tamper(string(committed[1].RecoveryCredentialRef))
	restarted := mustManager(t, registry, secrets, nil)
	if err := restarted.Recover(ctx); err != nil {
		t.Fatalf("Recover(without live source authority) error = %v", err)
	}
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, restarted, candidates[0], committed[0].RecoveryCredentialRef,
		expectedCleanedSourceSHA256,
	); err == nil {
		t.Fatal("FinalizeLegacyMigrationRecovery(group with tampered member) error = nil")
	}
	_, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(before, registry.snapshot()) || afterTombstones != beforeTombstones ||
		afterDeletes != beforeDeletes || !secrets.exists(string(committed[0].RecoveryCredentialRef)) {
		t.Fatalf("group preflight failure partially deleted recovery: before=%#v after=%#v cleanup=%d/%d->%d/%d",
			before, registry.snapshot(), beforeTombstones, beforeDeletes, afterTombstones, afterDeletes)
	}
}

func TestManagerRecoveryRepreflightsAuthorizedLegacyMigrationSourceGroupBeforeAnyDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	sourceLocator := "current:analytix-settings.json"
	sourceSnapshot := []byte(`{"provider":{"apiKey":"synthetic-authorized-group-repreflight"}}`)
	digest := sha256.Sum256(sourceSnapshot)
	sourceSHA256 := hex.EncodeToString(digest[:])
	committed := make([]CommitVerifiedLegacyMigrationRecoveryResult, 0, 2)
	candidates := make([]LegacyMigrationCandidate, 0, 2)
	expectedCleanedSourceSHA256 := ""
	for index := 0; index < 2; index++ {
		candidate := legacyMigrationCandidateFor(
			registry.snapshot(), fmt.Sprintf("migration-authorized-group-repreflight-%d", index),
			fmt.Sprintf("provider-authorized-group-repreflight-%d", index), "authorized-group-repreflight",
			fmt.Sprintf("synthetic-authorized-group-repreflight-%d", index),
		)
		candidate.SourceLocator = sourceLocator
		candidate.SourceSnapshot = bytes.Clone(sourceSnapshot)
		candidate.SourceSHA256 = sourceSHA256
		candidate.ActiveCredentialLocators = []string{
			fmt.Sprintf("%s:provider.providers[%d].apiKey", sourceLocator, index),
		}
		if index > 0 {
			legacyMigrationTestSharePhysicalSource(t, candidates[0], &candidate)
		}
		if expectedCleanedSourceSHA256 == "" {
			expectedCleanedSourceSHA256 = candidate.ExpectedCleanedSourceSHA256
		}
		prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.CommitVerifiedLegacyMigrationRecovery(
			ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
		)
		if err != nil {
			t.Fatal(err)
		}
		committed = append(committed, result)
		candidates = append(candidates, candidate)
	}
	interrupted := mustManager(
		t, registry, secrets, faultOnce(FaultAfterMigrationFinalizationRecoveryDeleteIntent),
	)
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, interrupted, candidates[0], committed[0].RecoveryCredentialRef,
		expectedCleanedSourceSHA256,
	); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("FinalizeLegacyMigrationRecovery(group intent) error = %v, want interrupted", err)
	}
	before := registry.snapshot()
	_, beforeTombstones, beforeDeletes := secrets.callCounts()
	secrets.tamper(string(committed[1].RecoveryCredentialRef))
	restarted := mustManager(t, registry, secrets, nil)
	if err := restarted.Recover(ctx); err != nil {
		t.Fatalf("Recover(without live source authority) error = %v", err)
	}
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, restarted, candidates[0], committed[0].RecoveryCredentialRef,
		expectedCleanedSourceSHA256,
	); err == nil {
		t.Fatal("FinalizeLegacyMigrationRecovery(authorized group with tampered member) error = nil")
	}
	_, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(before, registry.snapshot()) || afterTombstones != beforeTombstones ||
		afterDeletes != beforeDeletes || !secrets.exists(string(committed[0].RecoveryCredentialRef)) {
		t.Fatalf("authorized group repreflight partially deleted recovery: before=%#v after=%#v cleanup=%d/%d->%d/%d",
			before, registry.snapshot(), beforeTombstones, beforeDeletes, afterTombstones, afterDeletes)
	}
}

func TestManagerRecoveryRejectsMissingPayloadAfterAuthorizationBeforeTombstone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(), "migration-finalization-authorization-window",
		"provider-finalization-authorization-window", "source-finalization-authorization-window",
	)
	prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationFinalizingRecorded))
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, interrupted, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v, want interrupted", err)
	}
	secrets.remove(string(committed.RecoveryCredentialRef))
	before := registry.snapshot()
	_, beforeTombstones, beforeDeletes := secrets.callCounts()
	restarted := mustManager(t, registry, secrets, nil)
	if err := restarted.Recover(ctx); err != nil {
		t.Fatalf("Recover(without live source authority) error = %v", err)
	}
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, restarted, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); err == nil {
		t.Fatal("FinalizeLegacyMigrationRecovery(missing payload before tombstone) error = nil")
	}
	_, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(before, registry.snapshot()) || afterTombstones != beforeTombstones ||
		afterDeletes != beforeDeletes || !secrets.hasActive(committed.Provider.CredentialRef) {
		t.Fatalf("authorization window promoted missing payload: before=%#v after=%#v cleanup=%d/%d->%d/%d",
			before, registry.snapshot(), beforeTombstones, beforeDeletes, afterTombstones, afterDeletes)
	}
}

func TestManagerRecoveryRetainsProtectedPayloadUntilRollbackWinnerDeletionIsDurable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(), "migration-finalize-order", "provider-finalize-order", "source-finalize-order",
	)
	prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256: candidate.SourceSHA256, RecoveryCredentialRef: committed.RecoveryCredentialRef,
		Confirmation: LegacyMigrationRollbackConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := registry.snapshot()
	if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
		t, ctx, manager, before, candidate, committed.RecoveryCredentialRef,
	); err != nil {
		t.Fatal(err)
	}
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationFinalizationWinnerDeleted))
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, interrupted, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v, want interrupted", err)
	}
	if !secrets.exists(string(committed.RecoveryCredentialRef)) {
		t.Fatal("rollback finalization deleted protected recovery before winner deletion became durable")
	}
	secrets.tamper(string(committed.RecoveryCredentialRef))
	beforeRecovery := registry.snapshot()
	_, beforeTombstones, beforeDeletes := secrets.callCounts()
	restarted := mustManager(t, registry, secrets, nil)
	if err := restarted.Recover(ctx); err != nil {
		t.Fatalf("Recover(without live source authority) error = %v", err)
	}
	if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
		t, ctx, restarted, candidate, committed.RecoveryCredentialRef,
		candidate.ExpectedCleanedSourceSHA256,
	); err == nil {
		t.Fatal("FinalizeLegacyMigrationRecovery(tampered recovery after winner deletion) error = nil")
	}
	_, afterTombstones, afterDeletes := secrets.callCounts()
	if !reflect.DeepEqual(beforeRecovery, registry.snapshot()) ||
		beforeTombstones != afterTombstones || beforeDeletes != afterDeletes ||
		!secrets.exists(string(committed.RecoveryCredentialRef)) {
		t.Fatal("restarted rollback finalization advanced after protected recovery tamper")
	}
}

type interruptAfterTombstoneEffectStore struct {
	*memorySecretStore
	targetRef   string
	interrupted bool
}

func (store *interruptAfterTombstoneEffectStore) Tombstone(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
) error {
	err := store.memorySecretStore.Tombstone(ctx, ref, purpose)
	if err == nil && string(ref) == store.targetRef && !store.interrupted {
		store.interrupted = true
		return errors.New("synthetic interruption after tombstone effect")
	}
	return err
}

func TestManagerFinalizationRecoversTombstoneEffectAfterDurableDeleteIntent(t *testing.T) {
	t.Parallel()

	for _, outcome := range []string{"committed", "rolled-back"} {
		outcome := outcome
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationCandidateWithSourceSnapshotFor(
				registry.snapshot(), "migration-finalize-effect-window-"+outcome,
				"provider-finalize-effect-window-"+outcome, "source-finalize-effect-window-"+outcome,
			)
			prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatal(err)
			}
			committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, prepared),
			)
			if err != nil {
				t.Fatal(err)
			}
			targetRef := string(committed.RecoveryCredentialRef)
			if outcome == "rolled-back" {
				_, beginErr := manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
					MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
					SourceSHA256: candidate.SourceSHA256, RecoveryCredentialRef: committed.RecoveryCredentialRef,
					Confirmation: LegacyMigrationRollbackConfirmation,
				})
				if beginErr != nil {
					t.Fatal(beginErr)
				}
				before := registry.snapshot()
				if _, commitErr := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
					t, ctx, manager, before, candidate, committed.RecoveryCredentialRef,
				); commitErr != nil {
					t.Fatal(commitErr)
				}
				targetRef = committed.Provider.CredentialRef
			}

			interruptedSecrets := &interruptAfterTombstoneEffectStore{
				memorySecretStore: secrets,
				targetRef:         targetRef,
			}
			interrupted := mustManager(t, registry, interruptedSecrets, nil)
			verifiedSourceSHA256 := candidate.ExpectedCleanedSourceSHA256
			if _, finalizeErr := legacyMigrationTestFinalizeWithLiveSourceAuthority(
				t, ctx, interrupted, candidate, committed.RecoveryCredentialRef,
				verifiedSourceSHA256,
			); finalizeErr == nil {
				t.Fatal("FinalizeLegacyMigrationRecovery(interrupted tombstone effect) error = nil")
			}
			if !interruptedSecrets.interrupted {
				t.Fatal("finalization did not reach the synthetic tombstone effect window")
			}
			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover(without live source authority) error = %v", err)
			}
			if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
				t, ctx, restarted, candidate, committed.RecoveryCredentialRef,
				verifiedSourceSHA256,
			); err != nil {
				t.Fatalf("FinalizeLegacyMigrationRecovery(fresh live authority) error = %v", err)
			}
			marker := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
			expectedPhase := domainregistry.LegacyMigrationRecoveryPhaseFinalized
			if outcome == "rolled-back" {
				expectedPhase = domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained
			}
			if marker.Phase != expectedPhase {
				t.Fatalf("recovered finalization marker = %#v", marker)
			}
		})
	}
}

func TestLegacyMigrationV1V2RecoveryCannotAuthorizePlaintextCleanup(t *testing.T) {
	t.Parallel()

	for _, version := range []string{"v1", "v2"} {
		version := version
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationV1CandidateFor(
				registry.snapshot(), "migration-no-cleanup-"+version,
				"provider-no-cleanup-"+version, "source-no-cleanup-"+version,
				"synthetic-no-cleanup-"+version,
			)
			if version == "v2" {
				candidate.ActiveCredentialLocators = []string{
					"current:analytix-settings.json:provider.apiKey",
				}
			}
			prepared, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatalf("PrepareLegacyMigrationRecovery(%s) error = %v", version, err)
			}
			if prepared.SafeToProceedWithProviderMigration {
				t.Fatalf("PrepareLegacyMigrationRecovery(%s) authorized Provider migration: %#v", version, prepared)
			}
			replayed, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil || replayed.SafeToProceedWithProviderMigration {
				t.Fatalf("PrepareLegacyMigrationRecovery(%s repeat) = %#v, %v", version, replayed, err)
			}
			before := registry.snapshot()
			if committed, commitErr := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(before, candidate, prepared),
			); commitErr == nil || committed.SafeToRemoveLegacyPlaintext {
				t.Fatalf("CommitVerifiedLegacyMigrationRecovery(%s) = %#v, %v", version, committed, commitErr)
			}
			if len(registry.snapshot().Providers) != 0 {
				t.Fatalf("pre-V3 recovery committed Provider metadata: %#v", registry.snapshot())
			}
		})
	}
}

func TestManagerLegacyMigrationRollbackRejectsMismatchedSourceReferenceAndFencesWithoutCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	candidate := legacyMigrationCandidateWithSourceSnapshotFor(
		registry.snapshot(), "migration-rollback-fence", "provider-rollback-fence", "source-rollback-fence",
	)
	recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
		ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("X", 43))
	for _, command := range []BeginLegacyMigrationRollbackCommand{
		{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256:          strings.Repeat("b", 64),
			RecoveryCredentialRef: committed.RecoveryCredentialRef,
			Confirmation:          LegacyMigrationRollbackConfirmation,
		},
		{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256:          candidate.SourceSHA256,
			RecoveryCredentialRef: wrongRef,
			Confirmation:          LegacyMigrationRollbackConfirmation,
		},
		{
			MigrationID: candidate.MigrationID, SourceLocator: "compatibility:00:analytix-settings.json",
			SourceSHA256:          candidate.SourceSHA256,
			RecoveryCredentialRef: committed.RecoveryCredentialRef,
			Confirmation:          LegacyMigrationRollbackConfirmation,
		},
	} {
		before := registry.snapshot()
		_, beforeTombstones, beforeDeletes := secrets.callCounts()
		if _, err := manager.BeginLegacyMigrationRollback(ctx, command); !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("BeginLegacyMigrationRollback(mismatch) error = %v, want conflict", err)
		}
		_, afterTombstones, afterDeletes := secrets.callCounts()
		if !reflect.DeepEqual(before, registry.snapshot()) || beforeTombstones != afterTombstones ||
			beforeDeletes != afterDeletes {
			t.Fatal("mismatched rollback begin changed Registry or protected material")
		}
	}
	_, err = manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
		MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
		SourceSHA256:          candidate.SourceSHA256,
		RecoveryCredentialRef: committed.RecoveryCredentialRef,
		Confirmation:          LegacyMigrationRollbackConfirmation,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := registry.snapshot()
	validExpected := expectedFor(pending, candidate.Provider.ID)
	staleExpected := validExpected
	staleExpected.RegistryRevision--
	rollbackCommands := make([]CommitLegacyMigrationRollbackCommand, 0, 5)
	locatorMismatch := legacyMigrationTestRollbackCommitCommand(pending, candidate, committed.RecoveryCredentialRef)
	locatorMismatch.SourceLocator = "compatibility:00:analytix-settings.json"
	rollbackCommands = append(rollbackCommands, locatorMismatch)
	representationMismatch := legacyMigrationTestRollbackCommitCommand(pending, candidate, committed.RecoveryCredentialRef)
	representationMismatch.VerifiedCleanedSource = []byte(`{"provider":{"unexpected":true}}`)
	representationDigest := sha256.Sum256(representationMismatch.VerifiedCleanedSource)
	representationMismatch.VerifiedCleanedSourceSHA256 = hex.EncodeToString(representationDigest[:])
	rollbackCommands = append(rollbackCommands, representationMismatch)
	staleFence := legacyMigrationTestRollbackCommitCommand(pending, candidate, committed.RecoveryCredentialRef)
	staleFence.Expected = staleExpected
	rollbackCommands = append(rollbackCommands, staleFence)
	providerMismatch := legacyMigrationTestRollbackCommitCommand(pending, candidate, committed.RecoveryCredentialRef)
	providerMismatch.Expected = expectedFor(pending, "provider-other")
	rollbackCommands = append(rollbackCommands, providerMismatch)
	physicalMismatch := legacyMigrationTestRollbackCommitCommand(pending, candidate, committed.RecoveryCredentialRef)
	physicalMismatch.SourcePhysicalIdentitySHA256 = strings.Repeat("f", 64)
	rollbackCommands = append(rollbackCommands, physicalMismatch)
	for _, command := range rollbackCommands {
		_, beforeTombstones, beforeDeletes := secrets.callCounts()
		proof, cleanup := legacyMigrationTestSourceAuthorityProof(
			t, ctx, manager, LegacyMigrationSourceAuthorityOperationRollbackCommit,
			candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
		)
		command.SourceAuthority = proof
		_, err := manager.CommitLegacyMigrationRollback(ctx, command)
		cleanup()
		if err == nil {
			t.Fatal("CommitLegacyMigrationRollback(mismatch) error = nil")
		}
		_, afterTombstones, afterDeletes := secrets.callCounts()
		if !reflect.DeepEqual(pending, registry.snapshot()) || beforeTombstones != afterTombstones ||
			beforeDeletes != afterDeletes {
			t.Fatal("mismatched rollback commit changed Registry or protected material")
		}
	}
	finalizeCommands := make([]FinalizeLegacyMigrationRecoveryCommand, 0, 4)
	finalizeLocatorMismatch := legacyMigrationTestFinalizeCommand(
		t, candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
	)
	finalizeLocatorMismatch.SourceLocator = "compatibility:00:analytix-settings.json"
	finalizeCommands = append(finalizeCommands, finalizeLocatorMismatch)
	finalizeDigestMismatch := legacyMigrationTestFinalizeCommand(
		t, candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
	)
	finalizeDigestMismatch.SourceSHA256 = strings.Repeat("d", 64)
	finalizeCommands = append(finalizeCommands, finalizeDigestMismatch)
	finalizeRefMismatch := legacyMigrationTestFinalizeCommand(
		t, candidate, wrongRef, candidate.ExpectedCleanedSourceSHA256,
	)
	finalizeCommands = append(finalizeCommands, finalizeRefMismatch)
	finalizePhysicalMismatch := legacyMigrationTestFinalizeCommand(
		t, candidate, committed.RecoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
	)
	finalizePhysicalMismatch.SourcePhysicalIdentitySHA256 = strings.Repeat("f", 64)
	finalizeCommands = append(finalizeCommands, finalizePhysicalMismatch)
	for _, command := range finalizeCommands {
		_, beforeTombstones, beforeDeletes := secrets.callCounts()
		if _, err := manager.FinalizeLegacyMigrationRecovery(ctx, command); err == nil {
			t.Fatal("FinalizeLegacyMigrationRecovery(mismatch) error = nil")
		}
		_, afterTombstones, afterDeletes := secrets.callCounts()
		if !reflect.DeepEqual(pending, registry.snapshot()) || beforeTombstones != afterTombstones ||
			beforeDeletes != afterDeletes {
			t.Fatal("mismatched finalization changed Registry or protected material")
		}
	}
}

func TestManagerRecoversExplicitLegacyMigrationRollbackLifecycleCrashPoints(t *testing.T) {
	t.Parallel()

	t.Run("prepared candidate", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		candidate := legacyMigrationCandidateWithSourceSnapshotFor(
			registry.snapshot(), "migration-crash-prepared", "provider-crash-prepared", "source-crash-prepared",
		)
		interruptedPrepare := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationRecoveryPrepared))
		if _, err := interruptedPrepare.PrepareLegacyMigrationRecovery(ctx, candidate); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
		}
		record := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
		interruptedRollback := mustManager(t, registry, secrets, faultOnce(FaultAfterMigrationFinalizingRecorded))
		if _, err := interruptedRollback.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256:          candidate.SourceSHA256,
			RecoveryCredentialRef: secretstoreport.CredentialRef(record.RecoveryCredentialRef),
			Confirmation:          LegacyMigrationRollbackConfirmation,
		}); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("BeginLegacyMigrationRollback() error = %v", err)
		}
		restarted := mustManager(t, registry, secrets, nil)
		if err := restarted.Recover(ctx); err != nil {
			t.Fatalf("Recover() error = %v", err)
		}
		finalized := registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID]
		if finalized.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
			len(registry.snapshot().Providers) != 0 {
			t.Fatalf("prepared rollback restart state = %#v", registry.snapshot())
		}
	})

	for _, point := range []FaultPoint{
		FaultAfterMigrationRollbackSourceRestoreRecorded,
		FaultAfterMigrationRollbackCommittedRecorded,
		FaultAfterMigrationFinalizingRecorded,
		FaultAfterMigrationFinalizationWinnerDeleteIntent,
		FaultAfterMigrationFinalizationWinnerTombstoned,
		FaultAfterMigrationFinalizationWinnerDeleted,
		FaultAfterMigrationFinalizedRecorded,
	} {
		point := point
		t.Run(string(point), func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			candidate := legacyMigrationCandidateWithSourceSnapshotFor(
				registry.snapshot(), "migration-crash-"+string(point), "provider-crash-"+string(point), "source-crash",
			)
			recovery, err := manager.PrepareLegacyMigrationRecovery(ctx, candidate)
			if err != nil {
				t.Fatal(err)
			}
			committed, err := manager.CommitVerifiedLegacyMigrationRecovery(
				ctx, legacyMigrationCommitCommandFor(registry.snapshot(), candidate, recovery),
			)
			if err != nil {
				t.Fatal(err)
			}
			if point == FaultAfterMigrationRollbackSourceRestoreRecorded {
				interrupted := mustManager(t, registry, secrets, faultOnce(point))
				if _, err := interrupted.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
					MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
					SourceSHA256:          candidate.SourceSHA256,
					RecoveryCredentialRef: committed.RecoveryCredentialRef,
					Confirmation:          LegacyMigrationRollbackConfirmation,
				}); !errors.Is(err, ErrInterrupted) {
					t.Fatalf("BeginLegacyMigrationRollback() error = %v", err)
				}
				restarted := mustManager(t, registry, secrets, nil)
				if err := restarted.Recover(ctx); err != nil {
					t.Fatal(err)
				}
				replayed, err := restarted.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
					MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
					SourceSHA256:          candidate.SourceSHA256,
					RecoveryCredentialRef: committed.RecoveryCredentialRef,
					Confirmation:          LegacyMigrationRollbackConfirmation,
				})
				if err != nil || replayed.Status != LegacyMigrationRollbackStatusCleanedSourceRequired {
					t.Fatalf("BeginLegacyMigrationRollback(replay) = %#v, %v", replayed, err)
				}
				return
			}

			_, err = manager.BeginLegacyMigrationRollback(ctx, BeginLegacyMigrationRollbackCommand{
				MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
				SourceSHA256:          candidate.SourceSHA256,
				RecoveryCredentialRef: committed.RecoveryCredentialRef,
				Confirmation:          LegacyMigrationRollbackConfirmation,
			})
			if err != nil {
				t.Fatal(err)
			}
			if point == FaultAfterMigrationRollbackCommittedRecorded {
				interrupted := mustManager(t, registry, secrets, faultOnce(point))
				before := registry.snapshot()
				if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
					t, ctx, interrupted, before, candidate, committed.RecoveryCredentialRef,
				); !errors.Is(err, ErrInterrupted) {
					t.Fatalf("CommitLegacyMigrationRollback() error = %v", err)
				}
				restarted := mustManager(t, registry, secrets, nil)
				if err := restarted.Recover(ctx); err != nil {
					t.Fatal(err)
				}
				replayed, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
					t, ctx, restarted, before, candidate, committed.RecoveryCredentialRef,
				)
				if err != nil || replayed.Status != LegacyMigrationRollbackStatusCommittedRecoveryRetained {
					t.Fatalf("CommitLegacyMigrationRollback(replay) = %#v, %v", replayed, err)
				}
				return
			}

			before := registry.snapshot()
			if _, err := legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
				t, ctx, manager, before, candidate, committed.RecoveryCredentialRef,
			); err != nil {
				t.Fatal(err)
			}
			interrupted := mustManager(t, registry, secrets, faultOnce(point))
			if _, err := legacyMigrationTestFinalizeWithLiveSourceAuthority(
				t, ctx, interrupted, candidate, committed.RecoveryCredentialRef,
				candidate.ExpectedCleanedSourceSHA256,
			); !errors.Is(err, ErrInterrupted) {
				t.Fatalf("FinalizeLegacyMigrationRecovery() error = %v", err)
			}
			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
			var replayed FinalizeLegacyMigrationRecoveryResult
			expectedStatus := LegacyMigrationFinalizationStatusCompleted
			if registry.snapshot().LegacyMigrationRecoveries[candidate.MigrationID].Phase ==
				domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
				expectedStatus = LegacyMigrationFinalizationStatusAlreadyFinalized
				replayed, err = restarted.FinalizeLegacyMigrationRecovery(
					ctx,
					legacyMigrationTestFinalizeCommand(
						t, candidate, "", candidate.ExpectedCleanedSourceSHA256,
					),
				)
			} else {
				replayed, err = legacyMigrationTestFinalizeWithLiveSourceAuthority(
					t, ctx, restarted, candidate, committed.RecoveryCredentialRef,
					candidate.ExpectedCleanedSourceSHA256,
				)
			}
			if err != nil || replayed.Status != expectedStatus ||
				replayed.Outcome != LegacyMigrationFinalizationOutcomeRolledBack {
				t.Fatalf("FinalizeLegacyMigrationRecovery(replay) = %#v, %v", replayed, err)
			}
		})
	}
}

func legacyMigrationCandidateFor(
	state domainregistry.Registry,
	migrationID, providerID, sourceSeed, credential string,
) LegacyMigrationCandidate {
	candidate := legacyMigrationV1CandidateFor(state, migrationID, providerID, sourceSeed, credential)
	candidate.SourceLocator = "current:analytix-settings.json"
	candidate.SourcePhysicalIdentitySHA256 = strings.Repeat("e", 64)
	candidate.SourceSnapshot = []byte(fmt.Sprintf(
		`{"provider":{"apiKey":"synthetic-%s"}}`, migrationID,
	))
	candidate.ActiveCredentialLocators = []string{
		"current:analytix-settings.json:provider.apiKey",
	}
	sourceDigest := sha256.Sum256(candidate.SourceSnapshot)
	candidate.SourceSHA256 = hex.EncodeToString(sourceDigest[:])
	cleanedDigest := sha256.Sum256(legacyMigrationTestCleanedSource)
	candidate.ExpectedCleanedSourceSHA256 = hex.EncodeToString(cleanedDigest[:])
	legacyMigrationTestBindPhysicalSource(&candidate)
	return candidate
}

func legacyMigrationV1CandidateFor(
	state domainregistry.Registry,
	migrationID, providerID, sourceSeed, credential string,
) LegacyMigrationCandidate {
	sourceDigest := sha256.Sum256([]byte("synthetic legacy Provider source: " + sourceSeed))
	return LegacyMigrationCandidate{
		Expected:     expectedFor(state, providerID),
		MigrationID:  migrationID,
		SourceSHA256: hex.EncodeToString(sourceDigest[:]),
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
			Models: []string{"model-alpha"}, MediaModels: []string{"media-alpha"},
			SelectedModel: "model-alpha", SelectedMedia: "media-alpha", SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: "provider-api-key",
		Credential:        []byte(credential),
	}
}

func legacyMigrationCandidateWithArtifactsFor(
	state domainregistry.Registry,
	migrationID, providerID, sourceSeed string,
) LegacyMigrationCandidate {
	candidate := legacyMigrationCandidateFor(
		state,
		migrationID,
		providerID,
		sourceSeed,
		"synthetic-active-"+migrationID+"-not-a-real-key",
	)
	candidate.ActiveCredentialLocators = []string{
		"current:analytix-settings.json:provider.apiKey",
		"current:analytix-settings.json:provider.providers[0].apiKey",
	}
	candidate.RollbackCredentialArtifacts = []LegacyMigrationRollbackCredentialArtifact{
		{
			Locators: []string{
				"current:analytix-settings.json:runtime.apiKey",
				"current:analytix-settings.json:agents.kun.apiKey",
			},
			Credential: []byte("synthetic-shadow-runtime-" + migrationID + "-not-a-real-key"),
		},
		{
			Locators:   []string{"current:analytix-settings.json:deepseek.apiKey"},
			Credential: []byte("synthetic-shadow-legacy-" + migrationID + "-not-a-real-key"),
		},
	}
	return candidate
}

func legacyMigrationV2CandidateWithArtifactsFor(
	state domainregistry.Registry,
	migrationID, providerID, sourceSeed string,
) LegacyMigrationCandidate {
	candidate := legacyMigrationCandidateWithArtifactsFor(state, migrationID, providerID, sourceSeed)
	candidate.SourceLocator = ""
	candidate.SourceSnapshot = nil
	candidate.ExpectedCleanedSourceSHA256 = ""
	candidate.SourcePhysicalIdentitySHA256 = ""
	sourceDigest := sha256.Sum256([]byte("synthetic legacy Provider source: " + sourceSeed))
	candidate.SourceSHA256 = hex.EncodeToString(sourceDigest[:])
	return candidate
}

func legacyMigrationCandidateWithSourceSnapshotFor(
	state domainregistry.Registry,
	migrationID, providerID, sourceSeed string,
) LegacyMigrationCandidate {
	candidate := legacyMigrationCandidateWithArtifactsFor(state, migrationID, providerID, sourceSeed)
	candidate.SourceSnapshot = []byte(fmt.Sprintf(
		`{"provider":{"apiKey":"synthetic-%s","providers":[{"id":"%s","apiKey":"synthetic-%s"}]}}`,
		migrationID,
		providerID,
		migrationID,
	))
	candidate.SourceLocator = "current:analytix-settings.json"
	candidate.RollbackCredentialArtifacts[0].Locators = []string{
		"current:analytix-settings.json:runtime.apiKey",
		"current:analytix-settings.json:agents.kun.apiKey",
	}
	candidate.RollbackCredentialArtifacts[1].Locators = []string{
		"current:analytix-settings.json:deepseek.apiKey",
	}
	digest := sha256.Sum256(candidate.SourceSnapshot)
	candidate.SourceSHA256 = hex.EncodeToString(digest[:])
	cleanedDigest := sha256.Sum256(legacyMigrationTestCleanedSource)
	candidate.ExpectedCleanedSourceSHA256 = hex.EncodeToString(cleanedDigest[:])
	return candidate
}

var legacyMigrationTestCleanedSource = []byte(`{"provider":{}}`)

func legacyMigrationTestBindPhysicalSource(candidate *LegacyMigrationCandidate) {
	if candidate == nil || candidate.MigrationID == "" || candidate.SourceLocator == "" ||
		len(candidate.SourceSnapshot) == 0 {
		panic("invalid legacy migration physical source fixture")
	}
	legacyMigrationTestPhysicalSources.Lock()
	defer legacyMigrationTestPhysicalSources.Unlock()
	physicalPath := legacyMigrationTestPhysicalSources.paths[candidate.MigrationID]
	if physicalPath == "" {
		if legacyMigrationTestPhysicalSources.root == "" {
			panic("legacy migration physical source fixture root is unavailable")
		}
		legacyMigrationTestPhysicalSources.counter++
		directory := filepath.Join(
			legacyMigrationTestPhysicalSources.root,
			fmt.Sprintf("source-%06d", legacyMigrationTestPhysicalSources.counter),
		)
		if err := os.Mkdir(directory, 0o700); err != nil {
			panic(err)
		}
		physicalPath = filepath.Join(directory, "analytix-settings.json")
		if err := os.WriteFile(physicalPath, candidate.SourceSnapshot, 0o600); err != nil {
			panic(err)
		}
		legacyMigrationTestPhysicalSources.paths[candidate.MigrationID] = physicalPath
	}
	resolved, err := filepath.EvalSymlinks(physicalPath)
	if err != nil {
		panic(err)
	}
	candidate.SourcePhysicalIdentitySHA256 = legacyMigrationSourcePhysicalIdentity(
		candidate.SourceLocator, resolved,
	)
}

func legacyMigrationTestSharePhysicalSource(
	t *testing.T,
	anchor LegacyMigrationCandidate,
	candidate *LegacyMigrationCandidate,
) {
	t.Helper()
	if candidate == nil || anchor.SourceLocator != candidate.SourceLocator {
		t.Fatal("legacy migration shared source locator mismatch")
	}
	legacyMigrationTestPhysicalSources.Lock()
	physicalPath := legacyMigrationTestPhysicalSources.paths[anchor.MigrationID]
	if physicalPath == "" {
		legacyMigrationTestPhysicalSources.Unlock()
		t.Fatal("legacy migration shared source fixture is unavailable")
	}
	legacyMigrationTestPhysicalSources.paths[candidate.MigrationID] = physicalPath
	legacyMigrationTestPhysicalSources.Unlock()
	resolved, err := filepath.EvalSymlinks(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	candidate.SourcePhysicalIdentitySHA256 = legacyMigrationSourcePhysicalIdentity(
		candidate.SourceLocator, resolved,
	)
}

func legacyMigrationTestPhysicalSourcePath(
	t *testing.T,
	candidate LegacyMigrationCandidate,
) string {
	t.Helper()
	legacyMigrationTestPhysicalSources.Lock()
	physicalPath := legacyMigrationTestPhysicalSources.paths[candidate.MigrationID]
	legacyMigrationTestPhysicalSources.Unlock()
	if physicalPath == "" {
		t.Fatal("legacy migration physical source fixture is unavailable")
	}
	resolved, err := filepath.EvalSymlinks(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if legacyMigrationSourcePhysicalIdentity(candidate.SourceLocator, resolved) !=
		candidate.SourcePhysicalIdentitySHA256 {
		t.Fatal("legacy migration physical source fixture identity mismatch")
	}
	return physicalPath
}

func legacyMigrationTestSourceAuthorityProof(
	t *testing.T,
	ctx context.Context,
	manager *Manager,
	operation string,
	candidate LegacyMigrationCandidate,
	recoveryCredentialRef secretstoreport.CredentialRef,
	currentSourceSHA256 string,
) (LegacyMigrationSourceAuthorityProof, func()) {
	t.Helper()
	currentSource := legacyMigrationTestSourceRepresentation(t, candidate, currentSourceSHA256)
	defer clear(currentSource)
	physicalPath := legacyMigrationTestPhysicalSourcePath(t, candidate)
	if err := os.WriteFile(physicalPath, currentSource, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(physicalPath)
	if err != nil || !legacyMigrationRegularSingleLinkFileInfo(info) {
		t.Fatal("legacy migration physical source fixture is not an exact regular single-link file")
	}
	device, deviceOK := legacyMigrationFileInfoUintField(info, "Dev")
	inode, inodeOK := legacyMigrationFileInfoUintField(info, "Ino")
	if !deviceOK || !inodeOK || inode == 0 {
		t.Fatal("legacy migration physical source fixture generation is unavailable")
	}
	issued, err := manager.IssueLegacyMigrationSourceAuthorityChallenge(
		ctx,
		IssueLegacyMigrationSourceAuthorityChallengeCommand{
			Operation: operation, MigrationID: candidate.MigrationID,
			SourceLocator: candidate.SourceLocator, SourceSHA256: candidate.SourceSHA256,
			CurrentSourceSHA256:          currentSourceSHA256,
			SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef:        recoveryCredentialRef,
			Confirmation:                 LegacyMigrationSourceAuthorityChallengeConfirmation,
		},
	)
	if err != nil {
		t.Fatalf("IssueLegacyMigrationSourceAuthorityChallenge() error = %v", err)
	}
	legacyMigrationTestPhysicalSources.Lock()
	legacyMigrationTestPhysicalSources.counter++
	counter := legacyMigrationTestPhysicalSources.counter
	legacyMigrationTestPhysicalSources.Unlock()
	token := fmt.Sprintf("%08x-%04x-4000-8000-%012x", counter>>16, counter&0xffff, counter)
	resolved, err := filepath.EvalSymlinks(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := resolved + legacyMigrationSourceLockSuffix
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatalf("create legacy migration source lock: %v", err)
	}
	owner := legacyMigrationSourceLockOwner{
		SchemaVersion: 1,
		Purpose:       "analytix-provider-credential-migration-source-lock",
		PID:           os.Getpid(),
		Token:         token,
		CreatedAtMS:   1,
		Authority: legacyMigrationSourceLockAuthority{
			SchemaVersion: 1, Challenge: issued.Challenge, Operation: operation,
			MigrationID: candidate.MigrationID, SourceLocator: candidate.SourceLocator,
			SourceSHA256: candidate.SourceSHA256, CurrentSourceSHA256: currentSourceSHA256,
			SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
			SourceDevice:                 strconv.FormatUint(device, 10), SourceInode: strconv.FormatUint(inode, 10),
		},
	}
	ownerBytes, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	ownerPath := filepath.Join(lockPath, "owner-"+token+".json")
	if err := os.WriteFile(ownerPath, ownerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		if err := os.Remove(ownerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove legacy migration source lock owner: %v", err)
		}
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove legacy migration source lock: %v", err)
		}
	}
	return LegacyMigrationSourceAuthorityProof{
		Challenge: issued.Challenge, SourcePath: physicalPath, LockOwnerToken: token,
		SourceDevice: strconv.FormatUint(device, 10), SourceInode: strconv.FormatUint(inode, 10),
	}, cleanup
}

func legacyMigrationTestSourceRepresentation(
	t *testing.T,
	candidate LegacyMigrationCandidate,
	verifiedSourceSHA256 string,
) []byte {
	t.Helper()
	if verifiedSourceSHA256 == candidate.SourceSHA256 {
		return bytes.Clone(candidate.SourceSnapshot)
	}
	cleanedDigest := sha256.Sum256(legacyMigrationTestCleanedSource)
	if verifiedSourceSHA256 == hex.EncodeToString(cleanedDigest[:]) {
		return bytes.Clone(legacyMigrationTestCleanedSource)
	}
	t.Fatalf("test source representation unavailable for digest")
	return nil
}

func legacyMigrationTestRollbackCommitCommand(
	state domainregistry.Registry,
	candidate LegacyMigrationCandidate,
	recoveryCredentialRef secretstoreport.CredentialRef,
) CommitLegacyMigrationRollbackCommand {
	return CommitLegacyMigrationRollbackCommand{
		Expected:                     expectedFor(state, candidate.Provider.ID),
		MigrationID:                  candidate.MigrationID,
		SourceLocator:                candidate.SourceLocator,
		SourceSHA256:                 candidate.SourceSHA256,
		VerifiedCleanedSourceSHA256:  candidate.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		VerifiedCleanedSource:        bytes.Clone(legacyMigrationTestCleanedSource),
		RecoveryCredentialRef:        recoveryCredentialRef,
		Confirmation:                 LegacyMigrationRollbackCommitConfirmation,
	}
}

func legacyMigrationTestCommitRollbackWithLiveSourceAuthority(
	t *testing.T,
	ctx context.Context,
	manager *Manager,
	state domainregistry.Registry,
	candidate LegacyMigrationCandidate,
	recoveryCredentialRef secretstoreport.CredentialRef,
) (CommitLegacyMigrationRollbackResult, error) {
	t.Helper()
	command := legacyMigrationTestRollbackCommitCommand(state, candidate, recoveryCredentialRef)
	proof, cleanup := legacyMigrationTestSourceAuthorityProof(
		t, ctx, manager, LegacyMigrationSourceAuthorityOperationRollbackCommit,
		candidate, recoveryCredentialRef, candidate.ExpectedCleanedSourceSHA256,
	)
	defer cleanup()
	command.SourceAuthority = proof
	return manager.CommitLegacyMigrationRollback(ctx, command)
}

func legacyMigrationTestFinalizeCommand(
	t *testing.T,
	candidate LegacyMigrationCandidate,
	recoveryCredentialRef secretstoreport.CredentialRef,
	verifiedSourceSHA256 string,
) FinalizeLegacyMigrationRecoveryCommand {
	t.Helper()
	return FinalizeLegacyMigrationRecoveryCommand{
		MigrationID:                  candidate.MigrationID,
		SourceLocator:                candidate.SourceLocator,
		SourceSHA256:                 candidate.SourceSHA256,
		VerifiedSourceSHA256:         verifiedSourceSHA256,
		SourcePhysicalIdentitySHA256: candidate.SourcePhysicalIdentitySHA256,
		VerifiedSource: legacyMigrationTestSourceRepresentation(
			t, candidate, verifiedSourceSHA256,
		),
		RecoveryCredentialRef: recoveryCredentialRef,
		Confirmation:          LegacyMigrationFinalizeConfirmation,
	}
}

func legacyMigrationTestFinalizeWithLiveSourceAuthority(
	t *testing.T,
	ctx context.Context,
	manager *Manager,
	candidate LegacyMigrationCandidate,
	recoveryCredentialRef secretstoreport.CredentialRef,
	verifiedSourceSHA256 string,
) (FinalizeLegacyMigrationRecoveryResult, error) {
	t.Helper()
	command := legacyMigrationTestFinalizeCommand(
		t, candidate, recoveryCredentialRef, verifiedSourceSHA256,
	)
	proof, cleanup := legacyMigrationTestSourceAuthorityProof(
		t, ctx, manager, LegacyMigrationSourceAuthorityOperationFinalize,
		candidate, recoveryCredentialRef, verifiedSourceSHA256,
	)
	defer cleanup()
	command.SourceAuthority = proof
	return manager.FinalizeLegacyMigrationRecovery(ctx, command)
}

func equalLegacyMigrationLocatorView(actual [][]byte, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if !bytes.Equal(actual[index], []byte(expected[index])) {
			return false
		}
	}
	return true
}

func legacyMigrationRecoveryPayloadFixture(
	t *testing.T,
	version uint32,
	candidate LegacyMigrationCandidate,
) []byte {
	t.Helper()
	providerBytes, err := json.Marshal(candidate.Provider)
	if err != nil {
		t.Fatalf("json.Marshal(ProviderInput) error = %v", err)
	}
	payload := make([]byte, 0, 512)
	payload = append(payload, legacyMigrationRecoveryPayloadMagic...)
	payload = binary.BigEndian.AppendUint32(payload, version)
	appendField := func(field []byte) {
		payload = binary.BigEndian.AppendUint32(payload, uint32(len(field)))
		payload = append(payload, field...)
	}
	for _, field := range [][]byte{
		[]byte(candidate.MigrationID),
		[]byte(candidate.SourceSHA256),
		providerBytes,
		[]byte(candidate.CredentialPurpose),
		candidate.Credential,
	} {
		appendField(field)
	}
	if version == legacyMigrationRecoveryPayloadVersionV2 {
		payload = binary.BigEndian.AppendUint32(payload, uint32(len(candidate.ActiveCredentialLocators)))
		for _, locator := range candidate.ActiveCredentialLocators {
			appendField([]byte(locator))
		}
		payload = binary.BigEndian.AppendUint32(payload, uint32(len(candidate.RollbackCredentialArtifacts)))
		for _, artifact := range candidate.RollbackCredentialArtifacts {
			payload = binary.BigEndian.AppendUint32(payload, uint32(len(artifact.Locators)))
			for _, locator := range artifact.Locators {
				appendField([]byte(locator))
			}
			appendField(artifact.Credential)
		}
	}
	return payload
}

func legacyMigrationV2ActiveLocatorCountOffset(t *testing.T, payload []byte) int {
	t.Helper()
	offset := len(legacyMigrationRecoveryPayloadMagic) + 4
	for field := 0; field < 5; field++ {
		if offset > len(payload)-4 {
			t.Fatal("v2 fixture ended before base field length")
		}
		length := int(binary.BigEndian.Uint32(payload[offset : offset+4]))
		offset += 4
		if length < 0 || offset > len(payload)-length {
			t.Fatal("v2 fixture has invalid base field length")
		}
		offset += length
	}
	if offset > len(payload)-4 {
		t.Fatal("v2 fixture ended before active locator count")
	}
	return offset
}

func cloneLegacyMigrationCandidate(candidate LegacyMigrationCandidate) LegacyMigrationCandidate {
	clone := candidate
	clone.Provider = cloneProviderInput(candidate.Provider)
	clone.Credential = bytes.Clone(candidate.Credential)
	clone.SourceSnapshot = bytes.Clone(candidate.SourceSnapshot)
	clone.ActiveCredentialLocators = append([]string(nil), candidate.ActiveCredentialLocators...)
	clone.RollbackCredentialArtifacts = make(
		[]LegacyMigrationRollbackCredentialArtifact,
		len(candidate.RollbackCredentialArtifacts),
	)
	for index, artifact := range candidate.RollbackCredentialArtifacts {
		clone.RollbackCredentialArtifacts[index] = LegacyMigrationRollbackCredentialArtifact{
			Locators:   append([]string(nil), artifact.Locators...),
			Credential: bytes.Clone(artifact.Credential),
		}
	}
	return clone
}

func legacyMigrationCommitCommandFor(
	state domainregistry.Registry,
	candidate LegacyMigrationCandidate,
	recovery LegacyMigrationRecoveryResult,
) CommitVerifiedLegacyMigrationRecoveryCommand {
	sourceLocator := ""
	if len(candidate.SourceSnapshot) != 0 {
		sourceLocator = candidate.SourceLocator
	}
	return CommitVerifiedLegacyMigrationRecoveryCommand{
		Expected:                   candidate.Expected,
		ExpectedSelectedProviderID: state.SelectedProviderID,
		MigrationID:                candidate.MigrationID,
		SourceLocator:              sourceLocator,
		SourceSHA256:               candidate.SourceSHA256,
		RecoveryCredentialRef:      recovery.RecoveryCredentialRef,
		Confirmation:               LegacyMigrationCommitConfirmation,
	}
}

type legacyMigrationCleanupFailureStore struct {
	*memorySecretStore
	tombstoneErr error
	deleteErr    error
}

func (store *legacyMigrationCleanupFailureStore) Tombstone(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
) error {
	if store.tombstoneErr != nil {
		return store.tombstoneErr
	}
	return store.memorySecretStore.Tombstone(ctx, ref, purpose)
}

func (store *legacyMigrationCleanupFailureStore) ExplicitDelete(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
	intent secretstoreport.CredentialMutation,
) error {
	if store.deleteErr != nil {
		return store.deleteErr
	}
	return store.memorySecretStore.ExplicitDelete(ctx, ref, purpose, intent)
}

type legacyMigrationPlaintextObservingStore struct {
	*memorySecretStore
	prepareBuffer []byte
	readBuffers   [][]byte
}

func (store *legacyMigrationPlaintextObservingStore) PreparePut(
	ctx context.Context,
	purpose secretstoreport.Purpose,
	secret []byte,
) (secretstoreport.PreparedCandidate, error) {
	store.prepareBuffer = secret
	return store.memorySecretStore.PreparePut(ctx, purpose, secret)
}

func (store *legacyMigrationPlaintextObservingStore) GetForAuthorizedConsumer(
	ctx context.Context,
	request secretstoreport.AccessRequest,
) ([]byte, error) {
	plaintext, err := store.memorySecretStore.GetForAuthorizedConsumer(ctx, request)
	if plaintext != nil {
		store.readBuffers = append(store.readBuffers, plaintext)
	}
	return plaintext, err
}

func allZero(value []byte) bool {
	for _, current := range value {
		if current != 0 {
			return false
		}
	}
	return true
}

type legacyMigrationRecordingRegistryStore struct {
	state          domainregistry.Registry
	events         *[]string
	committedBytes []byte
	lastPhase      domainregistry.LegacyMigrationRecoveryPhase
}

func (store *legacyMigrationRecordingRegistryStore) WithExclusive(
	ctx context.Context,
	use func(registryport.Transaction) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return use(&legacyMigrationRecordingRegistryTransaction{store: store})
}

type legacyMigrationRecordingRegistryTransaction struct {
	store *legacyMigrationRecordingRegistryStore
}

func (transaction *legacyMigrationRecordingRegistryTransaction) Load(context.Context) (domainregistry.Registry, error) {
	return transaction.store.state.Clone(), nil
}

func (transaction *legacyMigrationRecordingRegistryTransaction) Commit(
	_ context.Context,
	state domainregistry.Registry,
) error {
	if err := state.Validate(); err != nil {
		return err
	}
	for _, recovery := range state.LegacyMigrationRecoveries {
		if recovery.Phase == transaction.store.lastPhase {
			continue
		}
		transaction.store.lastPhase = recovery.Phase
		switch recovery.Phase {
		case domainregistry.LegacyMigrationRecoveryPhasePrepared:
			*transaction.store.events = append(*transaction.store.events, "registry:migration-recovery-prepared")
		case domainregistry.LegacyMigrationRecoveryPhaseSecretDurable:
			*transaction.store.events = append(*transaction.store.events, "registry:migration-recovery-secret-durable")
		case domainregistry.LegacyMigrationRecoveryPhaseVerified:
			*transaction.store.events = append(*transaction.store.events, "registry:migration-recovery-verified")
		}
	}
	committed, err := domainregistry.Marshal(state)
	if err != nil {
		return err
	}
	transaction.store.committedBytes = committed
	transaction.store.state = state.Clone()
	return nil
}

type legacyMigrationRecordingSecretStore struct {
	candidateRef         secretstoreport.CredentialRef
	expectedCredential   []byte
	expectedSourceSHA256 string
	protectedPayload     []byte
	events               *[]string
	candidateDurable     bool
	readbackAuthorized   bool
}

func (store *legacyMigrationRecordingSecretStore) PreparePut(
	_ context.Context,
	purpose secretstoreport.Purpose,
	secret []byte,
) (secretstoreport.PreparedCandidate, error) {
	payload, err := parseLegacyMigrationRecoveryPayload(secret)
	if purpose != LegacyMigrationRecoveryPurpose || err != nil ||
		!bytes.Equal(payload.sourceSHA256, []byte(store.expectedSourceSHA256)) ||
		!bytes.Equal(payload.credential, store.expectedCredential) {
		return nil, secretstoreport.ErrInvalidRequest
	}
	store.protectedPayload = bytes.Clone(secret)
	*store.events = append(*store.events, "secret:recovery-candidate-prepared")
	return &legacyMigrationRecordingPreparedCandidate{store: store}, nil
}

func (store *legacyMigrationRecordingSecretStore) GetForAuthorizedConsumer(
	_ context.Context,
	request secretstoreport.AccessRequest,
) ([]byte, error) {
	if !store.candidateDurable || request.CredentialRef != store.candidateRef ||
		request.Purpose != LegacyMigrationRecoveryPurpose || request.Consumer != RegistryReadbackConsumer {
		return nil, secretstoreport.ErrUnauthorized
	}
	store.readbackAuthorized = true
	*store.events = append(*store.events, "secret:authorized-recovery-readback-verified")
	return bytes.Clone(store.protectedPayload), nil
}

func (*legacyMigrationRecordingSecretStore) Tombstone(
	context.Context,
	secretstoreport.CredentialRef,
	secretstoreport.Purpose,
) error {
	return secretstoreport.ErrConflict
}

func (*legacyMigrationRecordingSecretStore) ExplicitDelete(
	context.Context,
	secretstoreport.CredentialRef,
	secretstoreport.Purpose,
	secretstoreport.CredentialMutation,
) error {
	return secretstoreport.ErrConflict
}

type legacyMigrationRecordingPreparedCandidate struct {
	store *legacyMigrationRecordingSecretStore
}

func (candidate *legacyMigrationRecordingPreparedCandidate) CredentialRef() secretstoreport.CredentialRef {
	return candidate.store.candidateRef
}

func (candidate *legacyMigrationRecordingPreparedCandidate) Commit(context.Context) error {
	if candidate.store.candidateDurable {
		return secretstoreport.ErrConflict
	}
	candidate.store.candidateDurable = true
	*candidate.store.events = append(*candidate.store.events, "secret:recovery-candidate-durable")
	return nil
}

func (candidate *legacyMigrationRecordingPreparedCandidate) Abort() {
	if !candidate.store.candidateDurable {
		clear(candidate.store.protectedPayload)
	}
}
