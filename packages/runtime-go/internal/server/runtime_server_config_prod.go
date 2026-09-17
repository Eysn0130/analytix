//go:build analytix_prod

package server

import (
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"strings"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	"analytix.local/runtime-go/internal/protocol"
	readiness "analytix.local/runtime-go/internal/readiness"
)

type runtimeReadinessStatus = readiness.RuntimeReadinessProjection

type RuntimeServerConfig struct {
	DocumentCodecExecutable                   string
	DocumentCodecEntry                        string
	DevelopmentPluginSourceRoot               string
	DevelopmentOfficeAssetRoot                string
	RuntimeToken                              string
	Insecure                                  bool
	StartedAt                                 string
	Routes                                    []protocol.G2RouteReplayCase
	DurableTempDir                            string
	ProductionDurableRoot                     string
	Host                                      string
	Port                                      int
	DataDir                                   string
	UserDataDir                               string
	ProviderID                                string
	BaseURL                                   string
	APIKey                                    string
	Model                                     string
	EndpointFormat                            string
	ModelProvidersJSON                        string
	ModelProxyURL                             string
	ProviderAuditSocketPath                   string
	MCPProxyURL                               string
	ApprovalPolicy                            string
	SandboxMode                               string
	MCPConfigPath                             string
	MCPConfigJSON                             string
	HostScheduleMCPServer                     *domainmcp.ServerSpec
	AllowWriteRoots                           []string
	ProtectedReadDirs                         []string
	AuthorityAnchorV1                         string
	AuthorityManifestRoot                     string
	AuthorityCredentialProfileRoot            string
	AuthorityCredentialBundleRoot             string
	DarwinSecretStoreKeychainDBPath           string
	DarwinSecretStoreKeychainBindingDigest    string
	DarwinSecretStoreKeychainSecurityDigest   string
	ControlledArtifactHostV2URL               string
	ControlledArtifactHostV2Token             string
	ControlledArtifactHostV2BackendGeneration string
	ControlledArtifactHostV2AllocationDigest  string
	ControlledArtifactHostV2TLSRootCertDER    string
	ControlledArtifactHostV2TLSLeafSPKISHA256 string
	G6Readiness                               runtimeReadinessStatus
}

func defaultRuntimeReadinessStatus() runtimeReadinessStatus {
	return runtimeReadinessStatus{
		SchemaVersion: 1,
		Ready:         false,
	}
}

func newRuntimeEventSessionStore(config RuntimeServerConfig, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		return NewProductionDurableEventSessionStore(config.ProductionDurableRoot, floors...)
	}
	return NewTempDurableEventSessionStore(config.DurableTempDir, floors...)
}

func NewRuntimeEventSessionStore(config RuntimeServerConfig, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newRuntimeEventSessionStore(config, floors...)
}

func NewRuntimeEventSessionStoreForSemanticStartup(config RuntimeServerConfig, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		return newSemanticStartupDurableEventSessionStore(config.ProductionDurableRoot, floors...)
	}
	return newSemanticStartupDurableEventSessionStore(config.DurableTempDir, floors...)
}

func runtimeDurableStoreLocationV1(config RuntimeServerConfig) (string, durableStoreMode) {
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		return config.ProductionDurableRoot, durableStoreModeProduction
	}
	return config.DurableTempDir, durableStoreModeTemp
}
