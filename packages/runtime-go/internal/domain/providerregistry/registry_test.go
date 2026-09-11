package providerregistry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRegistryStrictVersionedKeyFreeRoundTrip(t *testing.T) {
	t.Parallel()

	registry := validRegistryFixture()
	content, err := Marshal(registry)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, forbidden := range []string{
		"synthetic-provider-secret-marker", "apiKey", "accessToken", "refreshToken", "rawBody",
	} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Fatalf("committed Registry contains forbidden material %q", forbidden)
		}
	}
	loaded, err := Unmarshal(content)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	second, err := Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal(round trip) error = %v", err)
	}
	if !bytes.Equal(content, second) {
		t.Fatal("strict Registry round trip is not deterministic")
	}
	if loaded.Version != FormatVersion || loaded.Transactions[fixtureTransactionID()].Phase != PhaseCandidateDurable {
		t.Fatalf("loaded Registry = %#v", loaded)
	}
}

func TestRegistryProtectedRecoverySessionRoundTrip(t *testing.T) {
	t.Parallel()

	registry := validRegistryFixture()
	registry.LegacyMigrationRecoveries = make(map[string]LegacyMigrationRecovery)
	manifest, err := MarshalPortableManifestV1(PortableManifestV1{
		Schema: PortableManifestSchemaV1,
		Providers: []PortableProviderDescriptorV1{{
			Correlation: "provider-0", Kind: "openai-compatible", Endpoint: "https://provider.invalid",
			Models: []string{"model-alpha"}, MediaModels: []string{}, Routes: []string{},
			Intent: PortableManifestIntentReentryRequired,
		}},
		Accounts: []PortableAccountDescriptorV1{},
	})
	if err != nil {
		t.Fatalf("MarshalPortableManifestV1() error = %v", err)
	}
	manifestDigest := ProtectedRecoveryManifestDigest(manifest)
	request := ProtectedRecoveryRequestV1{
		Schema: ProtectedRecoveryRequestSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
		ManifestDigest: manifestDigest, OperationID: "operation-synthetic-roundtrip",
		SessionNonce: strings.Repeat("b", 32), ExpiresAt: "2099-01-02T03:04:05Z",
		DestinationEphemeralPublicKey: bytes.Repeat([]byte{1}, 32),
		Entries: []ProtectedRecoveryEntryV1{{
			Correlation: "provider-0", DestinationProviderID: "provider-destination",
			DestinationProviderRevision: 1, DestinationProviderGeneration: 1,
			DestinationProviderIncarnation: fixtureIncarnation('c'),
		}},
	}
	digestPayload, err := MarshalProtectedRecoveryRequestV1ForDigest(request)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryRequestV1ForDigest() error = %v", err)
	}
	digest := sha256.Sum256(digestPayload)
	request.RequestDigest = hex.EncodeToString(digest[:])
	request.VerificationFingerprint = ProtectedRecoveryFingerprint(request.RequestDigest)
	requestBytes, err := MarshalProtectedRecoveryRequestV1(request)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryRequestV1() error = %v", err)
	}
	registry.ProtectedRecoverySessions = map[string]ProtectedRecoverySessionV1{
		request.OperationID: {
			Version: ProtectedRecoveryProtocolVersion, OperationID: request.OperationID, Phase: ProtectedRecoveryPhasePending,
			Request: requestBytes, ManifestJSON: manifest, RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest,
			ItemSetDigest: ProtectedRecoveryItemSetDigest(request.Entries), SessionNonce: request.SessionNonce,
			ExpiresAt: request.ExpiresAt, DestinationKeyRef: fixtureProtectedRecoveryKeyRef(),
			DestinationKeyPurpose: "protected-recovery-pending-key", Entries: request.Entries,
			LocalBinding: ProtectedRecoveryLocalBindingV1{
				BrowserWindowID: "synthetic-window", MainFrameID: "synthetic-frame",
				ProfileBinding: "synthetic-profile", DataDirectoryBinding: "/isolated/synthetic-data",
			},
		},
	}
	content, err := Marshal(registry)
	if err != nil {
		t.Fatalf("Marshal(protected recovery session) error = %v", err)
	}
	loaded, err := Unmarshal(content)
	if err != nil {
		t.Fatalf("Unmarshal(protected recovery session) error = %v", err)
	}
	if !reflect.DeepEqual(loaded, registry) {
		t.Fatalf("protected recovery session round trip = %#v, want %#v", loaded.ProtectedRecoverySessions, registry.ProtectedRecoverySessions)
	}
	second, err := Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal(round trip protected recovery session) error = %v", err)
	}
	if !bytes.Equal(content, second) {
		t.Fatal("protected recovery session Registry round trip is not deterministic")
	}
	for _, forbidden := range []string{"synthetic-secret", "privateKey", "profileBindingOutsideLocalRecord"} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Fatalf("protected recovery Registry contains forbidden material %q", forbidden)
		}
	}
}

func TestProtectedRecoveryWireRecordsRejectAmbiguousJSON(t *testing.T) {
	t.Parallel()

	request, requestBytes := protectedRecoveryRequestFixture(t)
	bundleBytes, err := MarshalProtectedRecoveryBundleV1(ProtectedRecoveryBundleV1{
		Schema: ProtectedRecoveryBundleSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
		RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest,
		OperationID: request.OperationID, SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt,
		SourceEphemeralPublicKey: bytes.Repeat([]byte{2}, 32), Nonce: bytes.Repeat([]byte{3}, 12),
		Ciphertext: []byte("synthetic-ciphertext"), Entries: []ProtectedRecoveryBundleEntryV1{{
			Correlation: "provider-0", DestinationProviderID: "provider-destination", Purpose: "provider-api-key",
		}},
	})
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryBundleV1() error = %v", err)
	}
	receiptBytes, err := MarshalProtectedRecoveryReceiptV1(ProtectedRecoveryReceiptV1{
		Schema: ProtectedRecoveryReceiptSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
		RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest,
		BundleDigest: sha256Hex(bundleBytes), OperationID: request.OperationID,
		SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt,
		Entries: []ProtectedRecoveryReceiptEntryV1{{
			Correlation: "provider-0", DestinationProviderID: "provider-destination", Status: "applied",
		}}, AuthenticationTag: bytes.Repeat([]byte{4}, 32),
	})
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryReceiptV1() error = %v", err)
	}

	for _, testCase := range []struct {
		name  string
		body  []byte
		parse func([]byte) error
	}{
		{name: "request duplicate key", body: duplicateProtectedRecoverySchemaKey(requestBytes), parse: func(body []byte) error {
			_, err := ParseProtectedRecoveryRequestV1(body)
			return err
		}},
		{name: "bundle unknown field", body: addProtectedRecoveryUnknownField(bundleBytes), parse: func(body []byte) error {
			_, err := ParseProtectedRecoveryBundleV1(body)
			return err
		}},
		{name: "receipt trailing document", body: append(append([]byte(nil), receiptBytes...), []byte("{}")...), parse: func(body []byte) error {
			_, err := ParseProtectedRecoveryReceiptV1(body)
			return err
		}},
		{name: "request noncanonical whitespace", body: bytes.Replace(requestBytes, []byte(`"schema":"`), []byte(`"schema": "`), 1), parse: func(body []byte) error {
			_, err := ParseProtectedRecoveryRequestV1(body)
			return err
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.parse(testCase.body); err != ErrInvalidRegistry {
				t.Fatalf("protected recovery parser error = %v, want invalid Registry", err)
			}
		})
	}
}

func TestProtectedRecoveryWireBoundsAndStrictProducers(t *testing.T) {
	t.Parallel()
	request, requestBytes := protectedRecoveryRequestFixture(t)
	for _, size := range []int{31, 33} {
		hostile := replaceProtectedRecoveryJSONBytesField(t, requestBytes, "destinationEphemeralPublicKey", bytes.Repeat([]byte{7}, size))
		if _, err := ParseProtectedRecoveryRequestV1(hostile); err != ErrInvalidRegistry {
			t.Fatalf("request key length %d parser error = %v, want invalid Registry", size, err)
		}
		if _, err := MarshalProtectedRecoveryRequestV1(ProtectedRecoveryRequestV1{
			Schema: ProtectedRecoveryRequestSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
			ManifestDigest: request.ManifestDigest, OperationID: request.OperationID, SessionNonce: request.SessionNonce,
			ExpiresAt: request.ExpiresAt, DestinationEphemeralPublicKey: bytes.Repeat([]byte{8}, size),
			VerificationFingerprint: request.VerificationFingerprint, Entries: request.Entries,
		}); err == nil {
			t.Fatalf("request key length %d producer unexpectedly succeeded", size)
		}
		if _, err := MarshalProtectedRecoveryRequestV1ForDigest(ProtectedRecoveryRequestV1{
			Schema: ProtectedRecoveryRequestSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
			ManifestDigest: request.ManifestDigest, OperationID: request.OperationID, SessionNonce: request.SessionNonce,
			ExpiresAt: request.ExpiresAt, DestinationEphemeralPublicKey: bytes.Repeat([]byte{8}, size), Entries: request.Entries,
		}); err == nil {
			t.Fatalf("request digest key length %d producer unexpectedly succeeded", size)
		}
	}
	bundle := ProtectedRecoveryBundleV1{
		Schema: ProtectedRecoveryBundleSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
		RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest, OperationID: request.OperationID,
		SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt, SourceEphemeralPublicKey: bytes.Repeat([]byte{2}, 32),
		Nonce: bytes.Repeat([]byte{3}, 12), Ciphertext: []byte("synthetic-ciphertext"), Entries: []ProtectedRecoveryBundleEntryV1{{
			Correlation: "provider-0", DestinationProviderID: "provider-destination", Purpose: "provider-api-key",
		}},
	}
	bundleBytes, err := MarshalProtectedRecoveryBundleV1(bundle)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryBundleV1() error = %v", err)
	}
	for _, size := range []int{31, 33} {
		hostile := replaceProtectedRecoveryJSONBytesField(t, bundleBytes, "sourceEphemeralPublicKey", bytes.Repeat([]byte{9}, size))
		if _, err := ParseProtectedRecoveryBundleV1(hostile); err != ErrInvalidRegistry {
			t.Fatalf("bundle key length %d parser error = %v, want invalid Registry", size, err)
		}
	}
	if _, err := MarshalProtectedRecoveryBundleV1(ProtectedRecoveryBundleV1{
		Schema: ProtectedRecoveryBundleSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
		RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest, OperationID: request.OperationID,
		SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt, SourceEphemeralPublicKey: bytes.Repeat([]byte{2}, 32),
		Nonce: bytes.Repeat([]byte{3}, 12), Ciphertext: []byte("synthetic-ciphertext"), Entries: nil,
	}); err == nil {
		t.Fatal("empty bundle producer unexpectedly succeeded")
	}
	if _, err := MarshalProtectedRecoveryRequestV1ForDigest(ProtectedRecoveryRequestV1{
		Schema: ProtectedRecoveryRequestSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
		ManifestDigest: request.ManifestDigest, OperationID: request.OperationID, SessionNonce: request.SessionNonce,
		ExpiresAt: request.ExpiresAt, DestinationEphemeralPublicKey: request.DestinationEphemeralPublicKey,
	}); err == nil {
		t.Fatal("empty request digest entry producer unexpectedly succeeded")
	}
	bundle.ExpiresAt = "2099-01-02T03:04:05.000Z"
	if _, err := MarshalProtectedRecoveryBundleV1(bundle); err == nil {
		t.Fatal("noncanonical bundle expiry producer unexpectedly succeeded")
	}
}

func replaceProtectedRecoveryJSONBytesField(t *testing.T, body []byte, field string, value []byte) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", field, err)
	}
	old, ok := object[field]
	if !ok {
		t.Fatalf("JSON field %q missing", field)
	}
	replacement, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%s) error = %v", field, err)
	}
	oldField := append(append([]byte(nil), []byte(`"`+field+`":`)...), old...)
	newField := append(append([]byte(nil), []byte(`"`+field+`":`)...), replacement...)
	mutated := bytes.Replace(body, oldField, newField, 1)
	if bytes.Equal(mutated, body) {
		t.Fatalf("JSON field %q replacement did not change body", field)
	}
	return mutated
}

func protectedRecoveryRequestFixture(t *testing.T) (ProtectedRecoveryRequestV1, []byte) {
	t.Helper()
	request := ProtectedRecoveryRequestV1{
		Schema: ProtectedRecoveryRequestSchemaV1, ProtocolVersion: ProtectedRecoveryProtocolVersion,
		ManifestDigest: strings.Repeat("a", 64), OperationID: "operation-synthetic-wire",
		SessionNonce: strings.Repeat("b", 32), ExpiresAt: "2099-01-02T03:04:05Z",
		DestinationEphemeralPublicKey: bytes.Repeat([]byte{1}, 32), Entries: []ProtectedRecoveryEntryV1{{
			Correlation: "provider-0", DestinationProviderID: "provider-destination",
			DestinationProviderRevision: 1, DestinationProviderGeneration: 1,
			DestinationProviderIncarnation: fixtureIncarnation('c'),
		}},
	}
	digestPayload, err := MarshalProtectedRecoveryRequestV1ForDigest(request)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryRequestV1ForDigest() error = %v", err)
	}
	digest := sha256.Sum256(digestPayload)
	request.RequestDigest = hex.EncodeToString(digest[:])
	request.VerificationFingerprint = ProtectedRecoveryFingerprint(request.RequestDigest)
	requestBytes, err := MarshalProtectedRecoveryRequestV1(request)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryRequestV1() error = %v", err)
	}
	return request, requestBytes
}

func duplicateProtectedRecoverySchemaKey(body []byte) []byte {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return append([]byte(nil), body...)
	}
	schema, ok := object["schema"]
	if !ok {
		return append([]byte(nil), body...)
	}
	prefix := append([]byte(`{"schema":`), schema...)
	if !bytes.HasPrefix(body, prefix) {
		return append([]byte(nil), body...)
	}
	result := append([]byte(nil), prefix...)
	result = append(result, []byte(`,"schema":`)...)
	result = append(result, schema...)
	result = append(result, body[len(prefix):]...)
	return result
}

func addProtectedRecoveryUnknownField(body []byte) []byte {
	return bytes.Replace(body, []byte(`{"schema":`), []byte(`{"unknown":"synthetic","schema":`), 1)
}

func TestOAuthBindingRejectsBackslashOutsideTheAuthority(t *testing.T) {
	t.Parallel()
	binding := OAuthBindingMetadata{
		SchemaVersion: 1, Issuer: `https://issuer.invalid/path%5Csmuggled`,
		AuthorizationEndpoint: `https://login.invalid/authorize`,
		TokenEndpoint:         `https://tokens.invalid/token`, ClientID: "client-a",
		Scopes: []string{"openid"}, RedirectModeVersion: 1,
	}
	if err := binding.Validate(); err != ErrInvalidRegistry {
		t.Fatalf("OAuthBindingMetadata.Validate() error = %v, want invalid Registry", err)
	}
}

func TestRegistryRejectsMalformedUnknownAndCredentialBearingDocuments(t *testing.T) {
	t.Parallel()

	content, err := Marshal(validRegistryFixture())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	base := strings.TrimSpace(string(content))
	for _, testCase := range []struct {
		name    string
		content string
	}{
		{name: "truncated", content: base[:len(base)-1]},
		{name: "trailing document", content: base + ` {}`},
		{name: "unknown top-level field", content: strings.Replace(base, `"version":1`, `"version":1,"apiKey":"synthetic-provider-secret-marker"`, 1)},
		{name: "unknown version", content: strings.Replace(base, `"version":1`, `"version":2`, 1)},
		{name: "unknown transaction phase", content: strings.Replace(base, `"phase":"candidate-durable"`, `"phase":"guessed-winner"`, 1)},
		{name: "unknown transaction field", content: strings.Replace(base, `"providerId":"provider-alpha"`, `"providerId":"provider-alpha","rawBody":"synthetic-provider-secret-marker"`, 1)},
		{name: "credential bytes field", content: strings.Replace(base, `"credentialRef":"`+fixtureCredentialRef()+`"`, `"credentialRef":"`+fixtureCredentialRef()+`","secret":"synthetic-provider-secret-marker"`, 1)},
		{name: "null providers", content: strings.Replace(base, `"providers":{`, `"providers":null,"discarded":{`, 1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Unmarshal([]byte(testCase.content)); err != ErrInvalidRegistry {
				t.Fatalf("Unmarshal() error = %v, want invalid Registry", err)
			}
		})
	}
}

func TestRegistryValidationRejectsUnsafeIDsEndpointsLimitsAndFences(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(*Registry)
	}{
		{name: "unsafe provider id", mutate: func(value *Registry) {
			provider := value.Providers["provider-alpha"]
			delete(value.Providers, provider.ID)
			provider.ID = "../provider-alpha"
			value.Providers[provider.ID] = provider
		}},
		{name: "endpoint credentials", mutate: func(value *Registry) {
			provider := value.Providers["provider-alpha"]
			provider.Endpoint = "https://user:secret@provider.invalid/v1"
			value.Providers[provider.ID] = provider
		}},
		{name: "endpoint newline", mutate: func(value *Registry) {
			provider := value.Providers["provider-alpha"]
			provider.Endpoint = "https://provider.invalid/v1\nsecond"
			value.Providers[provider.ID] = provider
		}},
		{name: "credential query", mutate: func(value *Registry) {
			provider := value.Providers["provider-alpha"]
			provider.Endpoint = "https://provider.invalid/v1?api_key=synthetic-provider-secret-marker"
			value.Providers[provider.ID] = provider
		}},
		{name: "unknown candidate reference", mutate: func(value *Registry) {
			transaction := value.Transactions[fixtureTransactionID()]
			transaction.CandidateCredentialRef = "credential-raw-value"
			value.Transactions[transaction.ID] = transaction
		}},
		{name: "candidate does not match next Provider", mutate: func(value *Registry) {
			transaction := value.Transactions[fixtureTransactionID()]
			transaction.NextProvider.CredentialRef = "cred_" + strings.Repeat("Z", 43)
			value.Transactions[transaction.ID] = transaction
		}},
		{name: "candidate purpose does not match next Provider", mutate: func(value *Registry) {
			transaction := value.Transactions[fixtureTransactionID()]
			transaction.NextProvider.CredentialPurpose = "provider-other-purpose"
			value.Transactions[transaction.ID] = transaction
		}},
		{name: "operation shape", mutate: func(value *Registry) {
			transaction := value.Transactions[fixtureTransactionID()]
			transaction.Operation = OperationSelect
			value.Transactions[transaction.ID] = transaction
		}},
		{name: "phase resolution shape", mutate: func(value *Registry) {
			transaction := value.Transactions[fixtureTransactionID()]
			transaction.Phase = PhaseVerified
			transaction.Recovery.LastObserved = PhaseVerified
			value.Transactions[transaction.ID] = transaction
		}},
		{name: "fence incarnation mismatch", mutate: func(value *Registry) {
			transaction := value.Transactions[fixtureTransactionID()]
			transaction.Fence.ProviderIncarnation = "inc_invalid"
			value.Transactions[transaction.ID] = transaction
		}},
		{name: "fence purpose mismatch", mutate: func(value *Registry) {
			transaction := value.Transactions[fixtureTransactionID()]
			transaction.Fence.CurrentCredentialPurpose = "provider-other-purpose"
			value.Transactions[transaction.ID] = transaction
		}},
		{name: "credential ref without purpose", mutate: func(value *Registry) {
			provider := value.Providers["provider-alpha"]
			provider.CredentialPurpose = ""
			value.Providers[provider.ID] = provider
		}},
		{name: "selected tombstone", mutate: func(value *Registry) {
			provider := value.Providers["provider-alpha"]
			provider.CredentialRef = ""
			provider.SelectedRoutes = nil
			provider.Tombstone = true
			value.Providers[provider.ID] = provider
		}},
		{name: "provider limit", mutate: func(value *Registry) {
			for index := 0; index <= MaxProviders; index++ {
				id := "provider-" + strings.Repeat("a", index%80+1) + string(rune('a'+index%26))
				provider := value.Providers["provider-alpha"].Clone()
				provider.ID = id
				value.Providers[id] = provider
			}
		}},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			value := validRegistryFixture()
			testCase.mutate(&value)
			if err := value.Validate(); err != ErrInvalidRegistry {
				t.Fatalf("Validate() error = %v, want invalid Registry", err)
			}
		})
	}
}

func TestRegistryRejectsAllEndpointAndProxyQueriesAndFragments(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		endpoint string
		proxy    string
	}{
		{name: "client secret", endpoint: "https://provider.invalid/v1?client_secret=synthetic-marker"},
		{name: "password", endpoint: "https://provider.invalid/v1?password=synthetic-marker"},
		{name: "secret", endpoint: "https://provider.invalid/v1?secret=synthetic-marker"},
		{name: "x api key", endpoint: "https://provider.invalid/v1?x-api-key=synthetic-marker"},
		{name: "arbitrary query", endpoint: "https://provider.invalid/v1?custom=synthetic-marker"},
		{name: "fragment", endpoint: "https://provider.invalid/v1#synthetic-marker"},
		{name: "proxy query", endpoint: "https://provider.invalid/v1", proxy: "https://proxy.invalid?custom=synthetic-marker"},
		{name: "proxy fragment", endpoint: "https://provider.invalid/v1", proxy: "https://proxy.invalid#synthetic-marker"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := validRegistryFixture()
			provider := registry.Providers["provider-alpha"]
			provider.Endpoint = testCase.endpoint
			provider.Proxy = testCase.proxy
			registry.Providers[provider.ID] = provider
			if err := registry.Validate(); err != ErrInvalidRegistry {
				t.Fatalf("Validate() error = %v, want invalid Registry", err)
			}
		})
	}
}

func TestRegistryLegacyMigrationRecoveriesAreVersionedBoundedKeyFreeAndAliasSafe(t *testing.T) {
	t.Parallel()

	registry := validRegistryFixture()
	recovery := validLegacyMigrationRecoveryFixture()
	registry.LegacyMigrationRecoveries = map[string]LegacyMigrationRecovery{recovery.ID: recovery}
	content, err := Marshal(registry)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if bytes.Contains(content, []byte("synthetic-legacy-migration-canary-not-a-real-key")) {
		t.Fatal("Registry contains the synthetic legacy migration credential canary")
	}
	if bytes.Contains(content, []byte(`"sourceSha256"`)) || bytes.Contains(content, []byte(`"candidateSha256"`)) ||
		bytes.Contains(content, []byte(strings.Repeat("a", 64))) {
		t.Fatal("Registry contains source-derived or credential-derived migration binding metadata")
	}
	loaded, err := Unmarshal(content)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if loaded.LegacyMigrationRecoveries[recovery.ID].Phase != LegacyMigrationRecoveryPhaseVerified {
		t.Fatalf("loaded legacy migration recovery = %#v", loaded.LegacyMigrationRecoveries[recovery.ID])
	}

	mutations := []struct {
		name   string
		mutate func(*Registry)
	}{
		{name: "unknown version", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.Version++
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "unknown phase", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.Phase = "guessed-verified"
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "malformed id", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			delete(value.LegacyMigrationRecoveries, recovery.ID)
			current.ID = "../migration"
			value.LegacyMigrationRecoveries[current.ID] = current
		}},
		{name: "wrong recovery purpose", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.RecoveryPurpose = "provider-api-key"
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "provider id mismatch", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.Provider.ID = "provider-other"
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "aliases Provider winner", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.RecoveryCredentialRef = fixtureCredentialRef()
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "aliases K2 candidate", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.RecoveryCredentialRef = value.Transactions[fixtureTransactionID()].CandidateCredentialRef
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "duplicate recovery owner", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.ID = "migration-settings-beta"
			current.ProviderID = "provider-migration-beta"
			current.Provider.ID = current.ProviderID
			value.LegacyMigrationRecoveries[current.ID] = current
		}},
		{name: "duplicate Provider migration", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.ID = "migration-settings-beta"
			current.RecoveryCredentialRef = "cred_" + strings.Repeat("S", 43)
			value.LegacyMigrationRecoveries[current.ID] = current
		}},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			value := registry.Clone()
			testCase.mutate(&value)
			if err := value.Validate(); err != ErrInvalidRegistry {
				t.Fatalf("Validate() error = %v, want invalid Registry", err)
			}
		})
	}

	overLimit := registry.Clone()
	overLimit.LegacyMigrationRecoveries = make(map[string]LegacyMigrationRecovery, MaxLegacyMigrationRecoveries+1)
	for index := 0; index <= MaxLegacyMigrationRecoveries; index++ {
		current := recovery.Clone()
		current.ID = fmt.Sprintf("migration-settings-%03d", index)
		current.ProviderID = fmt.Sprintf("provider-migration-%03d", index)
		current.Provider.ID = current.ProviderID
		current.RecoveryCredentialRef = fmt.Sprintf("cred_%043d", index)
		overLimit.LegacyMigrationRecoveries[current.ID] = current
	}
	if err := overLimit.Validate(); err != ErrInvalidRegistry {
		t.Fatalf("over-limit Validate() error = %v, want invalid Registry", err)
	}
}

func TestRegistryCommittedRetainedRecoveryAliasesOnlyItsExactProviderWinner(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fixtureIncarnation('a'))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	recovery := validLegacyMigrationRecoveryFixture()
	recovery.Fence = LegacyMigrationRecoveryFence{
		RegistryIncarnation: registry.Incarnation,
	}
	recovery.Phase = LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained
	winner := recovery.Provider.Provider(
		fixtureIncarnation('m'),
		"cred_"+strings.Repeat("W", 43),
		recovery.CredentialPurpose,
		1,
		1,
	)
	recovery.CommittedProviderCredentialRef = winner.CredentialRef
	recovery.CommittedProviderCredentialPurpose = winner.CredentialPurpose
	recovery.CommittedProviderRevision = winner.Revision
	recovery.CommittedProviderGeneration = winner.Generation
	recovery.CommittedProviderIncarnation = winner.Incarnation
	registry.Revision = 1
	registry.SelectedProviderID = winner.ID
	registry.Providers[winner.ID] = winner
	registry.LegacyMigrationRecoveries[recovery.ID] = recovery
	content, err := Marshal(registry)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, forbidden := range []string{
		"synthetic-legacy-migration-canary-not-a-real-key",
		`"sourceSha256"`, `"candidateSha256"`, strings.Repeat("a", 64),
	} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Fatalf("committed-retained Registry contains forbidden material %q", forbidden)
		}
	}
	loaded, err := Unmarshal(content)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if loaded.LegacyMigrationRecoveries[recovery.ID].CommittedProviderCredentialRef != winner.CredentialRef {
		t.Fatalf("loaded committed-retained recovery = %#v", loaded.LegacyMigrationRecoveries[recovery.ID])
	}

	mutations := []struct {
		name   string
		mutate func(*Registry)
	}{
		{name: "recovery ref aliases winner", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.RecoveryCredentialRef = winner.CredentialRef
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "bound ref does not own Provider", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.CommittedProviderCredentialRef = "cred_" + strings.Repeat("Z", 43)
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "bound purpose diverges", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.CommittedProviderCredentialPurpose = "provider-other-purpose"
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "bound revision diverges", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.CommittedProviderRevision++
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "bound generation diverges", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.CommittedProviderGeneration++
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "bound incarnation diverges", mutate: func(value *Registry) {
			current := value.LegacyMigrationRecoveries[recovery.ID]
			current.CommittedProviderIncarnation = fixtureIncarnation('z')
			value.LegacyMigrationRecoveries[recovery.ID] = current
		}},
		{name: "winner metadata diverges", mutate: func(value *Registry) {
			provider := value.Providers[winner.ID]
			provider.Endpoint = "https://divergent.invalid/v1"
			value.Providers[winner.ID] = provider
		}},
		{name: "another recovery owns winner alias", mutate: func(value *Registry) {
			other := validLegacyMigrationRecoveryFixture()
			other.ID = "migration-settings-other"
			other.ProviderID = "provider-other"
			other.Provider.ID = other.ProviderID
			other.RecoveryCredentialRef = winner.CredentialRef
			other.Fence = LegacyMigrationRecoveryFence{RegistryRevision: 1, RegistryIncarnation: value.Incarnation}
			value.LegacyMigrationRecoveries[other.ID] = other
		}},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			value := registry.Clone()
			testCase.mutate(&value)
			if err := value.Validate(); err != ErrInvalidRegistry {
				t.Fatalf("Validate() error = %v, want invalid Registry", err)
			}
		})
	}
}

func TestRegistryAllowsMultipleDistinctCommittedRetainedLegacyMigrationRecoveries(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fixtureIncarnation('a'))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	alphaProvider, alphaRecovery := committedLegacyMigrationFixture(
		"migration-settings-alpha", "provider-migration-alpha", 'R', 'W', 'm',
		0, "",
	)
	betaProvider, betaRecovery := committedLegacyMigrationFixture(
		"migration-settings-beta", "provider-migration-beta", 'S', 'X', 'n',
		1, alphaProvider.ID,
	)
	registry.Revision = 2
	registry.SelectedProviderID = alphaProvider.ID
	registry.Providers[alphaProvider.ID] = alphaProvider
	registry.Providers[betaProvider.ID] = betaProvider
	registry.LegacyMigrationRecoveries[alphaRecovery.ID] = alphaRecovery
	registry.LegacyMigrationRecoveries[betaRecovery.ID] = betaRecovery
	content, err := Marshal(registry)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	loaded, err := Unmarshal(content)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, registry) {
		t.Fatalf("multiple committed-retained round trip = %#v, want %#v", loaded, registry)
	}
	for _, forbidden := range []string{
		"synthetic-legacy-migration-canary-not-a-real-key",
		`"sourceSha256"`, `"candidateSha256"`, strings.Repeat("a", 64),
	} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Fatalf("multiple committed-retained Registry contains forbidden material %q", forbidden)
		}
	}

	mutations := []struct {
		name   string
		mutate func(*Registry)
	}{
		{name: "duplicate provider winner ref", mutate: func(value *Registry) {
			provider := value.Providers[betaProvider.ID]
			provider.CredentialRef = alphaProvider.CredentialRef
			value.Providers[provider.ID] = provider
			recovery := value.LegacyMigrationRecoveries[betaRecovery.ID]
			recovery.CommittedProviderCredentialRef = provider.CredentialRef
			value.LegacyMigrationRecoveries[recovery.ID] = recovery
		}},
		{name: "recovery aliases other winner", mutate: func(value *Registry) {
			recovery := value.LegacyMigrationRecoveries[betaRecovery.ID]
			recovery.RecoveryCredentialRef = alphaProvider.CredentialRef
			value.LegacyMigrationRecoveries[recovery.ID] = recovery
		}},
		{name: "winner aliases other recovery", mutate: func(value *Registry) {
			provider := value.Providers[betaProvider.ID]
			provider.CredentialRef = alphaRecovery.RecoveryCredentialRef
			value.Providers[provider.ID] = provider
			recovery := value.LegacyMigrationRecoveries[betaRecovery.ID]
			recovery.CommittedProviderCredentialRef = provider.CredentialRef
			value.LegacyMigrationRecoveries[recovery.ID] = recovery
		}},
		{name: "duplicate migration provider", mutate: func(value *Registry) {
			recovery := value.LegacyMigrationRecoveries[betaRecovery.ID]
			recovery.ProviderID = alphaProvider.ID
			recovery.Provider.ID = alphaProvider.ID
			value.LegacyMigrationRecoveries[recovery.ID] = recovery
		}},
		{name: "incomplete committed recovery", mutate: func(value *Registry) {
			recovery := value.LegacyMigrationRecoveries[betaRecovery.ID]
			recovery.CommittedProviderCredentialRef = ""
			value.LegacyMigrationRecoveries[recovery.ID] = recovery
		}},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			value := registry.Clone()
			testCase.mutate(&value)
			if err := value.Validate(); err != ErrInvalidRegistry {
				t.Fatalf("Validate() error = %v, want invalid Registry", err)
			}
		})
	}
}

func TestRegistryRejectsUnknownLegacyMigrationRecoveryJSON(t *testing.T) {
	t.Parallel()

	registry := validRegistryFixture()
	recovery := validLegacyMigrationRecoveryFixture()
	registry.LegacyMigrationRecoveries = map[string]LegacyMigrationRecovery{recovery.ID: recovery}
	content, err := Marshal(registry)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	base := strings.TrimSpace(string(content))
	for _, testCase := range []struct {
		name    string
		content string
	}{
		{name: "unknown recovery field", content: strings.Replace(base, `"recoveryPurpose":"`, `"rawBody":"synthetic-marker","recoveryPurpose":"`, 1)},
		{name: "unknown recovery version", content: strings.Replace(base,
			fmt.Sprintf(`"legacyMigrationRecoveries":{"migration-settings-alpha":{"version":%d`, LegacyMigrationRecoveryVersion),
			fmt.Sprintf(`"legacyMigrationRecoveries":{"migration-settings-alpha":{"version":%d`, LegacyMigrationRecoveryVersion+1), 1)},
		{name: "unknown recovery phase", content: strings.Replace(base, `"phase":"verified"`, `"phase":"guessed"`, 1)},
		{name: "null recovery map", content: strings.Replace(base, `"legacyMigrationRecoveries":{`, `"legacyMigrationRecoveries":null,"discarded":{`, 1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Unmarshal([]byte(testCase.content)); err != ErrInvalidRegistry {
				t.Fatalf("Unmarshal() error = %v, want invalid Registry", err)
			}
		})
	}
}

func TestLegacyMigrationFinalizationMarkersRejectSourceIdentityDrift(t *testing.T) {
	t.Parallel()

	identity := strings.Repeat("a", 64)
	finalizing := validLegacyMigrationRecoveryFixture()
	finalizing.Phase = LegacyMigrationRecoveryPhaseFinalizingPreCommit
	finalizing.SourceIdentitySHA256 = identity
	finalizing.FinalizationOutcome = LegacyMigrationFinalizationOutcomeRolledBack
	finalizing.FinalizationPreCommit = true
	finalizing.FinalizedSourceIdentitySHA256 = identity
	if err := finalizing.Validate(); err != nil {
		t.Fatalf("valid finalizing recovery error = %v", err)
	}
	drifted := finalizing
	drifted.FinalizedSourceIdentitySHA256 = strings.Repeat("b", 64)
	if err := drifted.Validate(); err != ErrInvalidRegistry {
		t.Fatalf("finalizing source identity drift error = %v, want invalid Registry", err)
	}

	finalized := finalizing
	finalized.Phase = LegacyMigrationRecoveryPhaseFinalized
	finalized.RecoveryCredentialRef = ""
	if err := finalized.Validate(); err != nil {
		t.Fatalf("valid finalized recovery error = %v", err)
	}
	drifted = finalized
	drifted.SourceIdentitySHA256 = strings.Repeat("c", 64)
	if err := drifted.Validate(); err != ErrInvalidRegistry {
		t.Fatalf("finalized source identity drift error = %v, want invalid Registry", err)
	}
}

func validLegacyMigrationRecoveryFixture() LegacyMigrationRecovery {
	return LegacyMigrationRecovery{
		Version: LegacyMigrationRecoveryVersion, ID: "migration-settings-alpha",
		Phase: LegacyMigrationRecoveryPhaseVerified, ProviderID: "provider-migration",
		Provider: ProviderInput{
			ID: "provider-migration", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
			Models: []string{"model-alpha"}, MediaModels: []string{}, SelectedModel: "model-alpha",
			SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: "provider-api-key", RecoveryCredentialRef: "cred_" + strings.Repeat("R", 43),
		RecoveryPurpose: LegacyMigrationRecoveryPurpose,
		Fence: LegacyMigrationRecoveryFence{
			RegistryRevision: 9, RegistryIncarnation: fixtureIncarnation('a'),
			SelectedProviderID: "provider-alpha",
		},
	}
}

func committedLegacyMigrationFixture(
	migrationID, providerID string,
	recoveryRefByte, winnerRefByte, providerIncarnationByte byte,
	fenceRevision uint64,
	fenceSelectedProviderID string,
) (Provider, LegacyMigrationRecovery) {
	recovery := validLegacyMigrationRecoveryFixture()
	recovery.ID = migrationID
	recovery.ProviderID = providerID
	recovery.Provider.ID = providerID
	recovery.Phase = LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained
	recovery.RecoveryCredentialRef = "cred_" + strings.Repeat(string(recoveryRefByte), 43)
	recovery.Fence = LegacyMigrationRecoveryFence{
		RegistryRevision:    fenceRevision,
		RegistryIncarnation: fixtureIncarnation('a'),
		SelectedProviderID:  fenceSelectedProviderID,
	}
	provider := recovery.Provider.Provider(
		fixtureIncarnation(providerIncarnationByte),
		"cred_"+strings.Repeat(string(winnerRefByte), 43),
		recovery.CredentialPurpose,
		1,
		1,
	)
	recovery.CommittedProviderCredentialRef = provider.CredentialRef
	recovery.CommittedProviderCredentialPurpose = provider.CredentialPurpose
	recovery.CommittedProviderRevision = provider.Revision
	recovery.CommittedProviderGeneration = provider.Generation
	recovery.CommittedProviderIncarnation = provider.Incarnation
	return provider, recovery
}

func validRegistryFixture() Registry {
	provider := Provider{
		ID: "provider-alpha", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
		Models: []string{"model-alpha"}, MediaModels: []string{"media-alpha"},
		SelectedModel: "model-alpha", SelectedMedia: "media-alpha", SelectedRoutes: []string{"primary"},
		CredentialRef: fixtureCredentialRef(), CredentialPurpose: "provider-api-key", Revision: 3, Generation: 2,
		Incarnation: fixtureIncarnation('b'),
	}
	next := provider.Clone()
	next.CredentialRef = "cred_" + strings.Repeat("C", 43)
	next.CredentialPurpose = "provider-bearer-token"
	next.Revision++
	next.Generation++
	transaction := Transaction{
		Version: TransactionVersion, ID: fixtureTransactionID(), Operation: OperationCredentialReplace,
		Phase: PhaseCandidateDurable, ProviderID: provider.ID,
		Resolution:                 ResolutionPending,
		CandidateCredentialPurpose: "provider-bearer-token", SupersededCredentialPurpose: "provider-api-key",
		Fence: Fence{
			RegistryRevision: 9, RegistryIncarnation: fixtureIncarnation('a'),
			ProviderRevision: provider.Revision, ProviderGeneration: provider.Generation,
			ProviderIncarnation: provider.Incarnation, CurrentCredentialRef: provider.CredentialRef,
			CurrentCredentialPurpose: provider.CredentialPurpose,
			SelectedProviderID:       provider.ID,
		},
		CandidateCredentialRef: next.CredentialRef, SupersededCredentialRef: provider.CredentialRef,
		PriorProvider: &provider, NextProvider: &next, NextSelectedProviderID: provider.ID,
		Recovery: RecoveryRecord{Version: RecoveryRecordVersion, Attempts: 1, LastObserved: PhaseCandidateDurable},
	}
	return Registry{
		Version: FormatVersion, Revision: 9, Incarnation: fixtureIncarnation('a'),
		SelectedProviderID: provider.ID,
		Providers:          map[string]Provider{provider.ID: provider},
		Transactions:       map[string]Transaction{transaction.ID: transaction},
	}
}

func fixtureIncarnation(value byte) string { return "inc_" + strings.Repeat(string(value), 43) }

func fixtureCredentialRef() string { return "cred_" + strings.Repeat("B", 43) }

func fixtureProtectedRecoveryKeyRef() string { return "cred_" + strings.Repeat("E", 43) }

func fixtureTransactionID() string { return "txn_" + strings.Repeat("D", 43) }
