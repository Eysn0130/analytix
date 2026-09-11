package runtimeapp

import (
	"encoding/json"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

const runtimeStartupSecurityConfigurationVersion = 1

type runtimeStartupSecurityConfigurationV1 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	PersistenceMode             string `json:"persistenceMode"`
	ProviderID                  string `json:"providerId"`
	ProviderBaseURLDigest       string `json:"providerBaseUrlDigest"`
	Model                       string `json:"model"`
	EndpointFormat              string `json:"endpointFormat"`
	ModelProvidersDigest        string `json:"modelProvidersDigest"`
	MCPConfigPathDigest         string `json:"mcpConfigPathDigest"`
	MCPConfigDocumentDigest     string `json:"mcpConfigDocumentDigest"`
	ApprovalPolicy              string `json:"approvalPolicy"`
	SandboxMode                 string `json:"sandboxMode"`
	AllowWriteRootsDigest       string `json:"allowWriteRootsDigest"`
	ProtectedReadRootsDigest    string `json:"protectedReadRootsDigest"`
	AuthorityAnchorDigest       string `json:"authorityAnchorDigest"`
	AuthorityManifestDigest     string `json:"authorityManifestDigest"`
	AuthorityProfileDigest      string `json:"authorityProfileDigest"`
	AuthorityBundleDigest       string `json:"authorityBundleDigest"`
	DarwinKeychainBindingDigest string `json:"darwinKeychainBindingDigest,omitempty"`
	ConformanceRoutesDigest     string `json:"conformanceRoutesDigest"`
}

// runtimeStartupSecurityConfiguration deliberately excludes process time,
// listener host/port, readiness, bearer/provider secrets, and proxy URLs.
// Those values are either ephemeral or external-connect inputs and must not
// make a durable migration journal impossible to resume after restart.
func runtimeStartupSecurityConfiguration(config Config) runtimeStartupSecurityConfigurationV1 {
	return runtimeStartupSecurityConfigurationV1{
		SchemaVersion:               runtimeStartupSecurityConfigurationVersion,
		PersistenceMode:             runtimeStartupPersistenceMode(config),
		ProviderID:                  strings.TrimSpace(config.ProviderID),
		ProviderBaseURLDigest:       startupValueDigest(strings.TrimSpace(config.BaseURL)),
		Model:                       strings.TrimSpace(config.Model),
		EndpointFormat:              strings.TrimSpace(config.EndpointFormat),
		ModelProvidersDigest:        startupValueDigest(config.ModelProvidersJSON),
		MCPConfigPathDigest:         startupValueDigest(strings.TrimSpace(config.MCPConfigPath)),
		MCPConfigDocumentDigest:     startupValueDigest(config.MCPConfigJSON),
		ApprovalPolicy:              strings.TrimSpace(config.ApprovalPolicy),
		SandboxMode:                 strings.TrimSpace(config.SandboxMode),
		AllowWriteRootsDigest:       startupValueDigest(config.AllowWriteRoots),
		ProtectedReadRootsDigest:    startupValueDigest(config.ProtectedReadDirs),
		AuthorityAnchorDigest:       startupValueDigest(config.AuthorityAnchorV1),
		AuthorityManifestDigest:     startupValueDigest(strings.TrimSpace(config.AuthorityManifestRoot)),
		AuthorityProfileDigest:      startupValueDigest(strings.TrimSpace(config.AuthorityCredentialProfileRoot)),
		AuthorityBundleDigest:       startupValueDigest(strings.TrimSpace(config.AuthorityCredentialBundleRoot)),
		DarwinKeychainBindingDigest: strings.TrimSpace(config.DarwinSecretStoreKeychainBindingDigest),
		ConformanceRoutesDigest:     startupValueDigest(config.Routes),
	}
}

func startupValueDigest(value any) string {
	encoded, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(encoded)
}

// runtimeStartupConfigurationDigest binds validated current-run inputs without
// retaining credentials, case text, or raw configuration in the startup plan.
func runtimeStartupConfigurationDigest(values ...any) (string, error) {
	if len(values) == 0 {
		return "", errors.New("startup configuration binding is empty")
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", errors.New("startup configuration binding is not serializable")
		}
		parts = append(parts, domainsecurity.SHA256Hex(encoded))
	}
	body, _ := json.Marshal(struct {
		SchemaVersion int      `json:"schemaVersion"`
		Parts         []string `json:"parts"`
	}{SchemaVersion: 1, Parts: parts})
	return domainsecurity.SHA256Hex(body), nil
}

func runtimeStartupGenerationDigest(configurationDigest string, snapshot domainstartup.ManagedSnapshotV1) (string, error) {
	configurationDigest = strings.TrimSpace(configurationDigest)
	if !domainsecurity.IsSHA256Hex(configurationDigest) || domainstartup.ValidateManagedSnapshotV1(snapshot) != nil {
		return "", errors.New("runtime startup generation input is invalid")
	}
	body, err := json.Marshal(struct {
		SchemaVersion         int    `json:"schemaVersion"`
		ConfigurationDigest   string `json:"configurationDigest"`
		ManagedSnapshotDigest string `json:"managedSnapshotDigest"`
		RootBindingDigest     string `json:"rootBindingDigest"`
		RawCaptureDigest      string `json:"rawCaptureDigest"`
	}{
		SchemaVersion:         1,
		ConfigurationDigest:   configurationDigest,
		ManagedSnapshotDigest: snapshot.SnapshotDigest,
		RootBindingDigest:     snapshot.RootBindingDigest,
		RawCaptureDigest:      snapshot.RawCaptureDigest,
	})
	if err != nil {
		return "", errors.New("runtime startup generation is not serializable")
	}
	return domainsecurity.SHA256Hex(body), nil
}
