//go:build !analytix_prod

package contracts

type LiveLocalSidecarBoundary struct {
	Mode                                string `json:"mode"`
	TestConformanceOnly                 bool   `json:"testConformanceOnly"`
	ReadOnlyRouteReplayOnly             bool   `json:"readOnlyRouteReplayOnly"`
	IsolatedMutableG2LifecyclePrototype bool   `json:"isolatedMutableG2LifecyclePrototype"`
	IsolatedInMemoryStoreOnly           bool   `json:"isolatedInMemoryStoreOnly"`
	FixtureBackedProviderG3Prototype    bool   `json:"fixtureBackedProviderG3Prototype"`
	FixtureBackedProviderOnly           bool   `json:"fixtureBackedProviderOnly"`
	IsolatedG4ManagerPrototype          bool   `json:"isolatedG4ManagerPrototype"`
	FixtureBackedG4ManagerOnly          bool   `json:"fixtureBackedG4ManagerOnly"`
	TempDurableStorePrototype           bool   `json:"tempDurableStorePrototype"`
	TempDurableStoreOnly                bool   `json:"tempDurableStoreOnly"`
	MinimalAgentLoopPrototype           bool   `json:"minimalAgentLoopPrototype"`
	FixtureBackedLoopOnly               bool   `json:"fixtureBackedLoopOnly"`
	KernelLiveScaffoldPrototype         bool   `json:"kernelLiveScaffoldPrototype"`
	FixtureBackedKernelOnly             bool   `json:"fixtureBackedKernelOnly"`
	InternalGateOnly                    bool   `json:"internalGateOnly"`
	ExternalNetworkAllowed              bool   `json:"externalNetworkAllowed"`
	ProviderCredentialsAllowed          bool   `json:"providerCredentialsAllowed"`
	APIKeyReadAllowed                   bool   `json:"apiKeyReadAllowed"`
	ProviderLiveCallsAllowed            bool   `json:"providerLiveCallsAllowed"`
	ToolExecutionAllowed                bool   `json:"toolExecutionAllowed"`
	ApprovalExecutionAllowed            bool   `json:"approvalExecutionAllowed"`
	MCPConnectionAllowed                bool   `json:"mcpConnectionAllowed"`
	MCPCredentialsAllowed               bool   `json:"mcpCredentialsAllowed"`
	CredentialReadAllowed               bool   `json:"credentialReadAllowed"`
	FileMutationAllowed                 bool   `json:"fileMutationAllowed"`
	EventsJSONLMutationAllowed          bool   `json:"eventsJsonlMutationAllowed"`
	RealWorkspaceMutationAllowed        bool   `json:"realWorkspaceMutationAllowed"`
	ElectronMainConnected               bool   `json:"electronMainConnected"`
	DefaultGoBackendEnabled             bool   `json:"defaultGoBackendEnabled"`
	RendererVisibleGoRoutesAllowed      bool   `json:"rendererVisibleGoRoutesAllowed"`
	ReasonixPublicProtocolAllowed       bool   `json:"reasonixPublicProtocolAllowed"`
	RendererPreloadMainBridgeUnchanged  bool   `json:"rendererPreloadMainBridgeUnchanged"`
	AnalytixServeContractUnchanged      bool   `json:"analytixServeContractUnchanged"`
}

func LiveLocalSidecarProductBoundary() LiveLocalSidecarBoundary {
	return LiveLocalSidecarBoundary{
		Mode:                                "live-local-isolated-g2-g3-g4-provider-cache-manager-prototype",
		TestConformanceOnly:                 true,
		ReadOnlyRouteReplayOnly:             false,
		IsolatedMutableG2LifecyclePrototype: true,
		IsolatedInMemoryStoreOnly:           true,
		FixtureBackedProviderG3Prototype:    true,
		FixtureBackedProviderOnly:           true,
		IsolatedG4ManagerPrototype:          true,
		FixtureBackedG4ManagerOnly:          true,
		TempDurableStorePrototype:           false,
		TempDurableStoreOnly:                false,
		MinimalAgentLoopPrototype:           false,
		FixtureBackedLoopOnly:               false,
		KernelLiveScaffoldPrototype:         false,
		FixtureBackedKernelOnly:             false,
		InternalGateOnly:                    false,
		ExternalNetworkAllowed:              false,
		ProviderCredentialsAllowed:          false,
		APIKeyReadAllowed:                   false,
		ProviderLiveCallsAllowed:            false,
		ToolExecutionAllowed:                false,
		ApprovalExecutionAllowed:            false,
		MCPConnectionAllowed:                false,
		MCPCredentialsAllowed:               false,
		CredentialReadAllowed:               false,
		FileMutationAllowed:                 false,
		EventsJSONLMutationAllowed:          false,
		RealWorkspaceMutationAllowed:        false,
		ElectronMainConnected:               false,
		DefaultGoBackendEnabled:             false,
		RendererVisibleGoRoutesAllowed:      false,
		ReasonixPublicProtocolAllowed:       false,
		RendererPreloadMainBridgeUnchanged:  true,
		AnalytixServeContractUnchanged:      true,
	}
}

func LiveLocalSidecarDurableProductBoundary() LiveLocalSidecarBoundary {
	boundary := LiveLocalSidecarProductBoundary()
	boundary.Mode = "temp-durable-event-session-store-prototype"
	boundary.TempDurableStorePrototype = true
	boundary.TempDurableStoreOnly = true
	return boundary
}

func LiveLocalSidecarLoopProductBoundary() LiveLocalSidecarBoundary {
	boundary := LiveLocalSidecarDurableProductBoundary()
	boundary.Mode = "minimal-agent-loop-conformance-prototype"
	boundary.MinimalAgentLoopPrototype = true
	boundary.FixtureBackedLoopOnly = true
	return boundary
}

func LiveLocalSidecarKernelProductBoundary() LiveLocalSidecarBoundary {
	boundary := LiveLocalSidecarLoopProductBoundary()
	boundary.Mode = "kernel-live-scaffold-conformance-prototype"
	boundary.KernelLiveScaffoldPrototype = true
	boundary.FixtureBackedKernelOnly = true
	boundary.InternalGateOnly = true
	return boundary
}

func LiveProductionCandidateProductBoundary() LiveLocalSidecarBoundary {
	boundary := LiveLocalSidecarKernelProductBoundary()
	boundary.Mode = "production-candidate-go-runtime-parity-slice"
	boundary.TestConformanceOnly = false
	boundary.FixtureBackedProviderOnly = false
	boundary.FixtureBackedG4ManagerOnly = false
	boundary.FixtureBackedLoopOnly = false
	boundary.FixtureBackedKernelOnly = false
	boundary.ExternalNetworkAllowed = false
	boundary.ProviderCredentialsAllowed = false
	boundary.APIKeyReadAllowed = false
	boundary.ProviderLiveCallsAllowed = true
	boundary.MCPConnectionAllowed = true
	boundary.InternalGateOnly = true
	boundary.DefaultGoBackendEnabled = false
	boundary.RendererVisibleGoRoutesAllowed = false
	boundary.ReasonixPublicProtocolAllowed = false
	return boundary
}

func RuntimeServerContractProductBoundary() LiveLocalSidecarBoundary {
	boundary := LiveProductionCandidateProductBoundary()
	boundary.Mode = "go-runtime-server-contract-candidate"
	boundary.TestConformanceOnly = false
	boundary.FixtureBackedProviderOnly = false
	boundary.TempDurableStoreOnly = false
	boundary.FixtureBackedLoopOnly = false
	boundary.FixtureBackedKernelOnly = false
	boundary.ProviderLiveCallsAllowed = true
	boundary.MCPConnectionAllowed = true
	boundary.InternalGateOnly = true
	boundary.DefaultGoBackendEnabled = false
	boundary.RendererVisibleGoRoutesAllowed = false
	boundary.ReasonixPublicProtocolAllowed = false
	boundary.RendererPreloadMainBridgeUnchanged = true
	boundary.AnalytixServeContractUnchanged = true
	return boundary
}
