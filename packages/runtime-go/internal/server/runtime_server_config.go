//go:build !analytix_prod

package server

import (
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"strings"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	"analytix.local/runtime-go/internal/protocol"
	readiness "analytix.local/runtime-go/internal/readiness"
)

type runtimeReadinessStatus = readiness.RuntimeReadinessStatus

type RuntimeServerConfig struct {
	DevelopmentPluginSourceRoot               string
	DevelopmentOfficeAssetRoot                string
	RuntimeToken                              string
	Insecure                                  bool
	StartedAt                                 string
	Routes                                    []protocol.G2RouteReplayCase
	DurableTempDir                            string
	CandidateDurableRoot                      string
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

type RuntimeServerContractConfig = RuntimeServerConfig

func defaultRuntimeReadinessStatus() runtimeReadinessStatus {
	return readiness.RuntimeReadinessStatusFromEnv(nil)
}

func NewRuntimeEventSessionStore(config RuntimeServerConfig, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newRuntimeEventSessionStore(config, floors...)
}

func NewRuntimeEventSessionStoreForSemanticStartup(config RuntimeServerConfig, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newRuntimeEventSessionStoreForSemanticStartup(config, floors...)
}

func newRuntimeEventSessionStore(config RuntimeServerConfig, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		return NewProductionDurableEventSessionStore(config.ProductionDurableRoot, floors...)
	}
	if strings.TrimSpace(config.CandidateDurableRoot) != "" {
		return NewCandidateDurableEventSessionStore(config.CandidateDurableRoot, floors...)
	}
	return NewTempDurableEventSessionStore(config.DurableTempDir, floors...)
}

func newRuntimeEventSessionStoreForSemanticStartup(config RuntimeServerConfig, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		return newSemanticStartupDurableEventSessionStore(config.ProductionDurableRoot, floors...)
	}
	if strings.TrimSpace(config.CandidateDurableRoot) != "" {
		return newSemanticStartupDurableEventSessionStore(config.CandidateDurableRoot, floors...)
	}
	return newSemanticStartupDurableEventSessionStore(config.DurableTempDir, floors...)
}

func runtimeDurableStoreLocationV1(config RuntimeServerConfig) (string, durableStoreMode) {
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		return config.ProductionDurableRoot, durableStoreModeProduction
	}
	if strings.TrimSpace(config.CandidateDurableRoot) != "" {
		return config.CandidateDurableRoot, durableStoreModeCandidate
	}
	return config.DurableTempDir, durableStoreModeTemp
}
