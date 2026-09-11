//go:build !analytix_prod

package livelocal

import (
	"net/http"
	"strings"

	agent "analytix.local/runtime-go/internal/agent"
	contracts "analytix.local/runtime-go/internal/contracts"
	mcp "analytix.local/runtime-go/internal/mcp"
	provider "analytix.local/runtime-go/internal/provider"
)

type LiveLocalSidecarConfig struct {
	RuntimeToken              string
	StartedAt                 string
	Routes                    []G2RouteReplayCase
	ProviderContract          provider.G3ProviderConformanceContract
	G4Contract                mcp.G4ToolsConformanceContract
	ApprovalUserInputContract agent.ApprovalUserInputRouteContract
	MCPToolLifecycleContract  mcp.MCPToolLifecycleContract
	DurableTempDir            string
	LoopContract              agent.GoMinimalAgentLoopContract
	KernelContract            GoKernelLiveScaffoldContract
}

type LiveLocalSidecarBoundary = contracts.LiveLocalSidecarBoundary

func LiveLocalSidecarProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarProductBoundary()
}

func LiveLocalSidecarDurableProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarDurableProductBoundary()
}

func LiveLocalSidecarLoopProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarLoopProductBoundary()
}

func LiveLocalSidecarKernelProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarKernelProductBoundary()
}

func NewLiveLocalSidecarHandler(config LiveLocalSidecarConfig) http.Handler {
	return NewLiveLocalSidecarHarness(config)
}

type LiveLocalSidecarHarness struct {
	g1      http.Handler
	g2      *LiveLocalG2MutableHandler
	g3      *provider.LiveLocalG3ProviderHandler
	g4      *mcp.LiveLocalG4ManagerHandler
	durable *LiveLocalDurableHandler
	loop    *LiveLocalLoopHandler
	kernel  *LiveLocalKernelHandler
	live    *LiveProductionCandidateHandler
	store   *LiveLocalG2Store
}

func NewLiveLocalSidecarHarness(config LiveLocalSidecarConfig) *LiveLocalSidecarHarness {
	g1 := NewG1ShadowHandler(G1ShadowConfig{
		RuntimeToken: config.RuntimeToken,
		StartedAt:    config.StartedAt,
	})
	g2 := NewLiveLocalG2MutableHandler(config.RuntimeToken, config.Routes)
	g3 := provider.NewLiveLocalG3ProviderHandler(config.RuntimeToken, config.ProviderContract, g2.Store())
	g4 := mcp.NewLiveLocalG4ManagerHandler(
		config.RuntimeToken,
		config.G4Contract,
		config.ApprovalUserInputContract,
		config.MCPToolLifecycleContract,
		g2.Store(),
	)
	var durable *LiveLocalDurableHandler
	if strings.TrimSpace(config.DurableTempDir) != "" {
		var err error
		durable, err = NewLiveLocalDurableHandler(config.RuntimeToken, config.DurableTempDir, config.Routes)
		if err != nil {
			panic(err)
		}
	}
	var loop *LiveLocalLoopHandler
	if durable != nil && strings.TrimSpace(config.LoopContract.ID) != "" {
		loop = NewLiveLocalLoopHandler(config.RuntimeToken, config.LoopContract, durable.Store())
	}
	var kernel *LiveLocalKernelHandler
	if strings.TrimSpace(config.KernelContract.ID) != "" {
		kernel = NewLiveLocalKernelHandler(config.RuntimeToken, config.KernelContract, g3, g4, durable, loop, g2.Store())
	}
	var live *LiveProductionCandidateHandler
	if durable != nil {
		live = NewLiveProductionCandidateHandler(config.RuntimeToken, durable.Store())
	}

	return &LiveLocalSidecarHarness{
		g1:      g1,
		g2:      g2,
		g3:      g3,
		g4:      g4,
		durable: durable,
		loop:    loop,
		kernel:  kernel,
		live:    live,
		store:   g2.Store(),
	}
}

func (h *LiveLocalSidecarHarness) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.live != nil && h.live.Handles(r.URL.Path) {
		h.live.ServeHTTP(w, r)
		return
	}
	if h.kernel != nil && h.kernel.Handles(r.URL.Path) {
		h.kernel.ServeHTTP(w, r)
		return
	}
	if h.loop != nil && h.loop.Handles(r.URL.Path) {
		h.loop.ServeHTTP(w, r)
		return
	}
	if h.durable != nil && h.durable.Handles(r.URL.Path) {
		h.durable.ServeHTTP(w, r)
		return
	}
	if h.g3 != nil && h.g3.Handles(r.URL.Path) {
		h.g3.ServeHTTP(w, r)
		return
	}
	if h.g4 != nil && h.g4.Handles(r.URL.Path) {
		h.g4.ServeHTTP(w, r)
		return
	}
	switch r.URL.Path {
	case "/health", "/v1/runtime/info", "/v1/runtime/tools":
		h.g1.ServeHTTP(w, r)
	default:
		h.g2.ServeHTTP(w, r)
	}
}

func (h *LiveLocalSidecarHarness) Snapshot() LiveLocalSidecarSnapshot {
	snapshot := h.store.Snapshot()
	if h.durable != nil {
		snapshot.TempDurableStoreEnabled = true
		snapshot.EventsJSONLWriteAttempts = h.durable.Store().WriteAttempts()
	}
	if h.kernel != nil {
		snapshot.StubKernelScaffoldReplays = h.kernel.ReplayAttempts()
	}
	if h.live != nil {
		snapshot.ProductionCandidate = h.live.Snapshot()
	}
	return snapshot
}

func readOnlyG2Routes(routes []G2RouteReplayCase) []G2RouteReplayCase {
	out := make([]G2RouteReplayCase, 0, len(routes))
	for _, route := range routes {
		if strings.EqualFold(route.Method, http.MethodGet) {
			out = append(out, route)
		}
	}
	return out
}
