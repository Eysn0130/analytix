package contracts

type ProductBoundary struct {
	RendererPreloadMainBridgeUnchanged bool `json:"rendererPreloadMainBridgeUnchanged"`
	AnalytixServeContractUnchanged     bool `json:"analytixServeContractUnchanged"`
	ReasonixPublicProtocolAllowed      bool `json:"reasonixPublicProtocolAllowed"`
	DefaultGoBackendAllowed            bool `json:"defaultGoBackendAllowed"`
	RendererVisibleGoRoutesAllowed     bool `json:"rendererVisibleGoRoutesAllowed"`
}

func G1ShadowProductBoundary() ProductBoundary {
	return ProductBoundary{
		RendererPreloadMainBridgeUnchanged: true,
		AnalytixServeContractUnchanged:     true,
		ReasonixPublicProtocolAllowed:      false,
		DefaultGoBackendAllowed:            false,
		RendererVisibleGoRoutesAllowed:     false,
	}
}
