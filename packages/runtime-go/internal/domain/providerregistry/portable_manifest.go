package providerregistry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	PortableManifestSchemaV1       = "analytix.provider-portable-manifest/v1"
	PortableManifestMaxBytes       = 1 << 20
	portableManifestMaxTokens      = 1 << 16
	portableManifestMaxStringBytes = 4096
)

// PortableManifestV1 is the ordinary, key-free cross-device projection. The
// correlation fields are local to this manifest and are never Registry IDs or
// authority on import.
type PortableManifestV1 struct {
	Schema    string                         `json:"schema"`
	Providers []PortableProviderDescriptorV1 `json:"providers"`
	Accounts  []PortableAccountDescriptorV1  `json:"accounts"`
}

// PortableProviderDescriptorV1 contains only metadata that can be safely
// re-entered on a destination. It deliberately has no source Provider ID,
// credential reference, selection authority, or Registry state.
type PortableProviderDescriptorV1 struct {
	Correlation        string                     `json:"correlation"`
	Kind               string                     `json:"kind"`
	Endpoint           string                     `json:"endpoint"`
	Proxy              string                     `json:"proxy,omitempty"`
	Models             []string                   `json:"models"`
	MediaModels        []string                   `json:"mediaModels"`
	SelectedModel      string                     `json:"selectedModel,omitempty"`
	SelectedMedia      string                     `json:"selectedMediaModel,omitempty"`
	OAuthBinding       *OAuthBindingMetadata      `json:"oauthBinding,omitempty"`
	AccountObservation *AccountObservationBinding `json:"accountObservation,omitempty"`
	Routes             []string                   `json:"routes"`
	Intent             string                     `json:"intent"`
}

// PortableAccountDescriptorV1 is the key-free portion of an existing private
// account entry. Account IDs and channels are intentionally omitted: the
// destination mints them during import.
type PortableAccountDescriptorV1 struct {
	Correlation string `json:"correlation"`
	Owner       string `json:"owner"`
	// Provider is a portable owner descriptor only. It is a manifest-local
	// correlation aid and is never copied into destination authority.
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Proxy    string `json:"proxy,omitempty"`
	Purpose  string `json:"purpose"`
	Intent   string `json:"intent"`
}

const (
	PortableManifestIntentReentryRequired        = "reentry_required"
	PortableManifestIntentProtectedRecoveryAvail = "protected_recovery_available"
)

// ParsePortableManifestV1 performs duplicate-key, trailing-data, unknown
// field, bounds, and canonical-byte checks before returning typed data.
func ParsePortableManifestV1(data []byte) (PortableManifestV1, error) {
	if err := jsonstrict.Validate(data, jsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       PortableManifestMaxBytes,
		MaxDepth:       16,
		MaxTokens:      portableManifestMaxTokens,
		MaxStringBytes: portableManifestMaxStringBytes,
	}); err != nil {
		return PortableManifestV1{}, ErrInvalidRegistry
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest PortableManifestV1
	if err := decoder.Decode(&manifest); err != nil {
		return PortableManifestV1{}, ErrInvalidRegistry
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PortableManifestV1{}, ErrInvalidRegistry
	}
	if err := manifest.Validate(); err != nil {
		return PortableManifestV1{}, err
	}
	canonical, err := marshalPortableManifestCanonical(manifest)
	if err != nil || !bytes.Equal(data, canonical) {
		return PortableManifestV1{}, ErrInvalidRegistry
	}
	return manifest, nil
}

// MarshalPortableManifestV1 emits the one canonical JSON byte sequence for a
// valid ordinary manifest.
func MarshalPortableManifestV1(manifest PortableManifestV1) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	data, err := marshalPortableManifestCanonical(manifest)
	if err != nil {
		return nil, ErrInvalidRegistry
	}
	if len(data) > PortableManifestMaxBytes {
		return nil, ErrInvalidRegistry
	}
	return data, nil
}

func marshalPortableManifestCanonical(manifest PortableManifestV1) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	// Match the public TypeScript JSON.stringify contract. The default Go
	// encoder escapes HTML punctuation, which would make an otherwise safe
	// metadata value non-canonical at the public boundary.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(manifest); err != nil {
		return nil, err
	}
	data := buffer.Bytes()
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, errors.New("portable manifest canonical encoding is invalid")
	}
	return append([]byte(nil), data[:len(data)-1]...), nil
}

func (manifest PortableManifestV1) Validate() error {
	if manifest.Schema != PortableManifestSchemaV1 || manifest.Providers == nil || manifest.Accounts == nil ||
		len(manifest.Providers) > MaxProviders || len(manifest.Accounts) > MaxPrivateAccounts ||
		len(manifest.Providers)+len(manifest.Accounts) > MaxProviders {
		return ErrInvalidRegistry
	}
	seenCorrelations := make(map[string]struct{}, len(manifest.Providers)+len(manifest.Accounts))
	seenProviders := make(map[string]struct{}, len(manifest.Providers))
	seenProviderCorrelations := make(map[string]struct{}, len(manifest.Providers))
	seenAccounts := make(map[string]struct{}, len(manifest.Accounts))
	previousProviderIdentity := ""
	for index, provider := range manifest.Providers {
		if provider.Correlation != "provider-"+strconv.Itoa(index) {
			return ErrInvalidRegistry
		}
		if err := provider.Validate(); err != nil {
			return err
		}
		if _, duplicate := seenCorrelations[provider.Correlation]; duplicate {
			return ErrInvalidRegistry
		}
		seenCorrelations[provider.Correlation] = struct{}{}
		seenProviderCorrelations[provider.Correlation] = struct{}{}
		identity, err := provider.CanonicalIdentity()
		if err != nil {
			return err
		}
		if index > 0 && identity <= previousProviderIdentity {
			return ErrInvalidRegistry
		}
		previousProviderIdentity = identity
		if _, duplicate := seenProviders[identity]; duplicate {
			return ErrInvalidRegistry
		}
		seenProviders[identity] = struct{}{}
	}
	previousAccountIdentity := ""
	for index, account := range manifest.Accounts {
		if account.Correlation != "account-"+strconv.Itoa(index) {
			return ErrInvalidRegistry
		}
		if err := account.Validate(); err != nil {
			return err
		}
		if _, duplicate := seenCorrelations[account.Correlation]; duplicate {
			return ErrInvalidRegistry
		}
		seenCorrelations[account.Correlation] = struct{}{}
		identity, err := account.CanonicalIdentity()
		if err != nil {
			return err
		}
		if index > 0 && identity <= previousAccountIdentity {
			return ErrInvalidRegistry
		}
		previousAccountIdentity = identity
		if _, duplicate := seenAccounts[identity]; duplicate {
			return ErrInvalidRegistry
		}
		seenAccounts[identity] = struct{}{}
	}
	for _, provider := range manifest.Providers {
		for _, route := range provider.Routes {
			if _, exists := seenProviderCorrelations[route]; !exists {
				return ErrInvalidRegistry
			}
		}
	}
	return nil
}

func (descriptor PortableProviderDescriptorV1) Validate() error {
	normalized, err := descriptor.Normalize()
	if err != nil || normalized.Endpoint != descriptor.Endpoint || normalized.Proxy != descriptor.Proxy ||
		!slices.Equal(normalized.Models, descriptor.Models) || !slices.Equal(normalized.MediaModels, descriptor.MediaModels) ||
		!slices.Equal(normalized.Routes, descriptor.Routes) ||
		!reflect.DeepEqual(normalized.OAuthBinding, descriptor.OAuthBinding) ||
		!reflect.DeepEqual(normalized.AccountObservation, descriptor.AccountObservation) {
		return ErrInvalidRegistry
	}
	if !validIdentifier(descriptor.Correlation, 96) ||
		!validIdentifier(descriptor.Kind, 96) || descriptor.Kind == PrivateAccountKind ||
		descriptor.Intent != PortableManifestIntentReentryRequired &&
			descriptor.Intent != PortableManifestIntentProtectedRecoveryAvail ||
		!validPortableMetadataString(descriptor.Correlation) ||
		!validPortableMetadataString(descriptor.Kind) ||
		!validPortableMetadataString(descriptor.Endpoint) ||
		!validPortableMetadataString(descriptor.Proxy) ||
		!validPortableMetadataString(descriptor.SelectedModel) ||
		!validPortableMetadataString(descriptor.SelectedMedia) {
		return ErrInvalidRegistry
	}
	if descriptor.Models == nil || descriptor.MediaModels == nil || descriptor.Routes == nil ||
		len(descriptor.Models) > MaxModels || len(descriptor.MediaModels) > MaxModels || len(descriptor.Routes) > MaxRoutes ||
		!validUniqueValues(descriptor.Models, 256) || !validUniqueValues(descriptor.MediaModels, 256) {
		return ErrInvalidRegistry
	}
	for _, value := range append(slices.Clone(descriptor.Models), descriptor.MediaModels...) {
		if !validPortableMetadataString(value) {
			return ErrInvalidRegistry
		}
	}
	seenRoutes := make(map[string]struct{}, len(descriptor.Routes))
	for _, route := range descriptor.Routes {
		if !validIdentifier(route, 96) || route == PrimaryRouteAlias || strings.HasPrefix(route, ExactProviderRoutePrefix) ||
			!validPortableMetadataString(route) {
			return ErrInvalidRegistry
		}
		if _, duplicate := seenRoutes[route]; duplicate {
			return ErrInvalidRegistry
		}
		seenRoutes[route] = struct{}{}
	}
	if descriptor.SelectedModel != "" && !slices.Contains(descriptor.Models, descriptor.SelectedModel) {
		return ErrInvalidRegistry
	}
	if descriptor.SelectedMedia != "" && !slices.Contains(descriptor.MediaModels, descriptor.SelectedMedia) {
		return ErrInvalidRegistry
	}
	return nil
}

func (descriptor PortableProviderDescriptorV1) Normalize() (PortableProviderDescriptorV1, error) {
	normalized := descriptor
	var err error
	if normalized.Endpoint, err = NormalizePortableEndpoint(descriptor.Endpoint, false); err != nil {
		return PortableProviderDescriptorV1{}, err
	}
	if normalized.Proxy, err = NormalizePortableEndpoint(descriptor.Proxy, true); err != nil {
		return PortableProviderDescriptorV1{}, err
	}
	normalized.Models = slices.Clone(descriptor.Models)
	normalized.MediaModels = slices.Clone(descriptor.MediaModels)
	normalized.Routes = slices.Clone(descriptor.Routes)
	if descriptor.OAuthBinding != nil {
		binding, err := normalizePortableOAuthBinding(*descriptor.OAuthBinding)
		if err != nil {
			return PortableProviderDescriptorV1{}, err
		}
		normalized.OAuthBinding = &binding
	}
	if descriptor.AccountObservation != nil {
		observation, err := normalizePortableAccountObservation(*descriptor.AccountObservation)
		if err != nil {
			return PortableProviderDescriptorV1{}, err
		}
		normalized.AccountObservation = &observation
	}
	sort.Strings(normalized.Models)
	sort.Strings(normalized.MediaModels)
	return normalized, nil
}

// CanonicalIdentity is the stable v1 Provider identity: kind plus normalized
// endpoint. Mutable proxy, model, selection, OAuth, observation, route, and
// intent fields deliberately do not make a duplicate safe.
func (descriptor PortableProviderDescriptorV1) CanonicalIdentity() (string, error) {
	normalized, err := descriptor.Normalize()
	if err != nil {
		return "", ErrInvalidRegistry
	}
	if !validIdentifier(normalized.Kind, 96) || normalized.Kind == PrivateAccountKind ||
		!validPortableMetadataString(normalized.Kind) || !validPortableMetadataString(normalized.Endpoint) ||
		!validPortableMetadataString(normalized.Proxy) || !validPortableMetadataString(normalized.SelectedModel) ||
		!validPortableMetadataString(normalized.SelectedMedia) ||
		!validUniqueValues(normalized.Models, 256) || !validUniqueValues(normalized.MediaModels, 256) ||
		(normalized.SelectedModel != "" && !slices.Contains(normalized.Models, normalized.SelectedModel)) ||
		(normalized.SelectedMedia != "" && !slices.Contains(normalized.MediaModels, normalized.SelectedMedia)) {
		return "", ErrInvalidRegistry
	}
	identity := struct {
		Kind     string `json:"kind"`
		Endpoint string `json:"endpoint"`
	}{
		Kind: normalized.Kind, Endpoint: normalized.Endpoint,
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", ErrInvalidRegistry
	}
	return string(data), nil
}

func (descriptor PortableAccountDescriptorV1) Validate() error {
	normalized, err := descriptor.Normalize()
	if err != nil || normalized.Endpoint != descriptor.Endpoint || normalized.Proxy != descriptor.Proxy ||
		!validIdentifier(descriptor.Correlation, 96) ||
		!validPortableMetadataString(descriptor.Correlation) ||
		(descriptor.Owner != "mcp" && descriptor.Owner != "extension") ||
		!validScopeComponent(descriptor.Provider, 128) || strings.ContainsAny(descriptor.Provider, "/\\") ||
		!validPortableMetadataString(descriptor.Provider) || !validPurpose(descriptor.Purpose) ||
		!validPortableMetadataString(descriptor.Endpoint) || !validPortableMetadataString(descriptor.Proxy) ||
		descriptor.Proxy != "" ||
		(descriptor.Intent != PortableManifestIntentReentryRequired && descriptor.Intent != PortableManifestIntentProtectedRecoveryAvail) {
		return ErrInvalidRegistry
	}
	if (descriptor.Owner == "mcp" && descriptor.Purpose != "mcp-oauth-access-token") ||
		(descriptor.Owner == "extension" && descriptor.Purpose != "extension-provider-account-token") {
		return ErrInvalidRegistry
	}
	accountID, channelID := "portable-account", "portable-channel"
	if (PrivateAccountScope{
		SchemaVersion: 1, Owner: descriptor.Owner, Provider: descriptor.Provider,
		AccountID: accountID, ChannelID: channelID, Purpose: descriptor.Purpose,
	}).Validate() != nil {
		return ErrInvalidRegistry
	}
	return nil
}

func (descriptor PortableAccountDescriptorV1) Normalize() (PortableAccountDescriptorV1, error) {
	normalized := descriptor
	var err error
	if normalized.Endpoint, err = NormalizePortableEndpoint(descriptor.Endpoint, false); err != nil {
		return PortableAccountDescriptorV1{}, err
	}
	if normalized.Proxy, err = NormalizePortableEndpoint(descriptor.Proxy, true); err != nil {
		return PortableAccountDescriptorV1{}, err
	}
	return normalized, nil
}

func (descriptor PortableAccountDescriptorV1) CanonicalIdentity() (string, error) {
	// Endpoint and proxy are mutable transport metadata. The stable v1 account
	// identity is owner plus the portable owner descriptor and purpose.
	normalized, err := descriptor.Normalize()
	if err != nil || descriptor.Validate() != nil || !validPurpose(normalized.Purpose) {
		return "", ErrInvalidRegistry
	}
	identity := struct {
		Owner    string `json:"owner"`
		Provider string `json:"provider"`
		Purpose  string `json:"purpose"`
	}{
		Owner: normalized.Owner, Provider: normalized.Provider, Purpose: normalized.Purpose,
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", ErrInvalidRegistry
	}
	return string(data), nil
}

// NormalizePortableEndpoint rejects userinfo, query/fragment material, and
// alternate escaped paths, then canonicalizes scheme and host casing.
func NormalizePortableEndpoint(value string, optional bool) (string, error) {
	if value == "" {
		if optional {
			return "", nil
		}
		return "", ErrInvalidRegistry
	}
	if !validEndpoint(value, optional) || strings.ContainsAny(value, "\u2028\u2029") {
		return "", ErrInvalidRegistry
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		parsed.RawFragment != "" || parsed.RawPath != "" || strings.ContainsAny(value, "\\#%") ||
		strings.ContainsAny(parsed.Path, "\\\x00\r\n\t") {
		return "", ErrInvalidRegistry
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	normalized := parsed.String()
	if strings.Contains(normalized, "%") || !validEndpoint(normalized, optional) {
		return "", ErrInvalidRegistry
	}
	return normalized, nil
}

func validPortableMetadataString(value string) bool {
	if value == "" {
		return true
	}
	if strings.ContainsAny(value, "\\") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~/") ||
		strings.Contains(value, "../") || strings.Contains(value, "..\\") {
		return false
	}
	return utf8SafePortableString(value)
}

func utf8SafePortableString(value string) bool {
	return utf8.ValidString(value) && value == strings.TrimSpace(value) &&
		!strings.ContainsAny(value, "\x00\r\n\t\u2028\u2029")
}

func normalizePortableOAuthBinding(binding OAuthBindingMetadata) (OAuthBindingMetadata, error) {
	if binding.SchemaVersion != 1 || binding.RedirectModeVersion != 1 ||
		!validPortableOAuthMetadataString(binding.ClientID, 256) || len(binding.Scopes) == 0 || len(binding.Scopes) > 32 {
		return OAuthBindingMetadata{}, ErrInvalidRegistry
	}
	normalized := binding
	var err error
	if normalized.Issuer, err = normalizePortableOAuthEndpoint(binding.Issuer); err != nil {
		return OAuthBindingMetadata{}, err
	}
	if normalized.AuthorizationEndpoint, err = normalizePortableOAuthEndpoint(binding.AuthorizationEndpoint); err != nil {
		return OAuthBindingMetadata{}, err
	}
	if normalized.TokenEndpoint, err = normalizePortableOAuthEndpoint(binding.TokenEndpoint); err != nil {
		return OAuthBindingMetadata{}, err
	}
	if binding.RevocationEndpoint != "" {
		if normalized.RevocationEndpoint, err = normalizePortableOAuthEndpoint(binding.RevocationEndpoint); err != nil {
			return OAuthBindingMetadata{}, err
		}
	}
	normalized.Scopes = slices.Clone(binding.Scopes)
	seenScopes := make(map[string]struct{}, len(binding.Scopes))
	for _, scope := range binding.Scopes {
		if !validPortableOAuthMetadataString(scope, 256) {
			return OAuthBindingMetadata{}, ErrInvalidRegistry
		}
		if _, duplicate := seenScopes[scope]; duplicate {
			return OAuthBindingMetadata{}, ErrInvalidRegistry
		}
		seenScopes[scope] = struct{}{}
	}
	return normalized, nil
}

func normalizePortableAccountObservation(observation AccountObservationBinding) (AccountObservationBinding, error) {
	if observation.SchemaVersion != 1 || observation.Method != "GET" || observation.Projection != "normalized-quota-v1" {
		return AccountObservationBinding{}, ErrInvalidRegistry
	}
	normalized := observation
	var err error
	if normalized.Endpoint, err = normalizePortableOAuthEndpoint(observation.Endpoint); err != nil {
		return AccountObservationBinding{}, err
	}
	return normalized, nil
}

func normalizePortableOAuthEndpoint(value string) (string, error) {
	if len(value) > 2048 || !validOAuthEndpoint(value) || strings.ContainsAny(value, "\u2028\u2029") {
		return "", ErrInvalidRegistry
	}
	normalized, err := NormalizePortableEndpoint(value, false)
	if err != nil || len(normalized) > 2048 || !validOAuthEndpoint(normalized) {
		return "", ErrInvalidRegistry
	}
	return normalized, nil
}

func validPortableOAuthMetadataString(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && validPortableMetadataString(value)
}
