//go:build !analytix_prod

package livelocal

import (
	"net/http"
	"strings"
	"sync"

	contracts "analytix.local/runtime-go/internal/contracts"
)

const LiveLocalKernelPrefix = "/v1/conformance/kernel"

type GoKernelLiveScaffoldContract struct {
	ID                 string                       `json:"id"`
	RuntimeToken       string                       `json:"runtimeToken"`
	SourceContractIDs  []string                     `json:"sourceContractIds"`
	ProductBoundary    map[string]any               `json:"productBoundary"`
	Components         []GoKernelComponent          `json:"components"`
	ReasonixAbsorption []GoKernelAbsorptionDecision `json:"reasonixAbsorption"`
	JobOrchestration   GoKernelJobOrchestration     `json:"jobOrchestration"`
	RetirementCleanup  GoKernelRetirementCleanup    `json:"retirementCleanup"`
	Expected           GoKernelLiveScaffoldExpected `json:"expected"`
}

type GoKernelComponent struct {
	ID                 string `json:"id"`
	Source             string `json:"source"`
	Class              string `json:"class"`
	Route              string `json:"route"`
	ExpectedLive       bool   `json:"expectedLive"`
	SideEffectsAllowed bool   `json:"sideEffectsAllowed"`
}

type GoKernelAbsorptionDecision struct {
	Capability      string `json:"capability"`
	ReasonixSource  string `json:"reasonixSource"`
	Class           string `json:"class"`
	AnalytixLanding string `json:"analytixLanding"`
}

type GoKernelJobOrchestration struct {
	FixtureOnly                   bool            `json:"fixtureOnly"`
	ParentGoalRequired            bool            `json:"parentGoalRequired"`
	TopLevelRoutesExposed         bool            `json:"topLevelRoutesExposed"`
	PublicSubagentProtocolAllowed bool            `json:"publicSubagentProtocolAllowed"`
	JobManagerLive                bool            `json:"jobManagerLive"`
	BackgroundExecutionAllowed    bool            `json:"backgroundExecutionAllowed"`
	LineageContract               map[string]bool `json:"lineageContract"`
	ExpectedStatuses              []string        `json:"expectedStatuses"`
	ExpectedTools                 []string        `json:"expectedTools"`
}

type GoKernelRetirementCleanup struct {
	G6Ready                     bool                          `json:"g6Ready"`
	NoImmediateProductionDelete bool                          `json:"noImmediateProductionDelete"`
	CurrentRetainedPaths        []GoKernelRetirementPath      `json:"currentRetainedPaths"`
	DeleteAfterG6               []GoKernelRetirementPath      `json:"deleteAfterG6"`
	ForbiddenRedundancy         []GoKernelForbiddenRedundancy `json:"forbiddenRedundancy"`
	G6Blockers                  []string                      `json:"g6Blockers"`
}

type GoKernelRetirementPath struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type GoKernelForbiddenRedundancy struct {
	ID      string `json:"id"`
	Token   string `json:"token"`
	Present bool   `json:"present"`
}

type GoKernelLiveScaffoldExpected struct {
	ComponentCount                  int            `json:"componentCount"`
	LiveComponentCount              int            `json:"liveComponentCount"`
	ReplaceOrPortCount              int            `json:"replaceOrPortCount"`
	ContractReimplementCount        int            `json:"contractReimplementCount"`
	DeferCount                      int            `json:"deferCount"`
	RejectCount                     int            `json:"rejectCount"`
	ForbiddenSurfaceCount           int            `json:"forbiddenSurfaceCount"`
	CurrentRetainedCount            int            `json:"currentRetainedCount"`
	DeleteAfterG6Count              int            `json:"deleteAfterG6Count"`
	ForbiddenRedundancyCount        int            `json:"forbiddenRedundancyCount"`
	PresentForbiddenRedundancyCount int            `json:"presentForbiddenRedundancyCount"`
	G6BlockerCount                  int            `json:"g6BlockerCount"`
	SideEffects                     map[string]int `json:"sideEffects"`
}

type LiveLocalKernelHandler struct {
	runtimeToken string
	contract     GoKernelLiveScaffoldContract
	g3           liveLocalKernelAvailability
	g4           liveLocalKernelAvailability
	durable      *LiveLocalDurableHandler
	loop         *LiveLocalLoopHandler
	store        *LiveLocalG2Store

	mu      sync.Mutex
	replays int
}

type liveLocalKernelAvailability interface {
	Enabled() bool
}

func NewLiveLocalKernelHandler(
	runtimeToken string,
	contract GoKernelLiveScaffoldContract,
	g3 liveLocalKernelAvailability,
	g4 liveLocalKernelAvailability,
	durable *LiveLocalDurableHandler,
	loop *LiveLocalLoopHandler,
	store *LiveLocalG2Store,
) *LiveLocalKernelHandler {
	if strings.TrimSpace(runtimeToken) == "" {
		runtimeToken = DefaultRuntimeToken
	}
	return &LiveLocalKernelHandler{
		runtimeToken: runtimeToken,
		contract:     contract,
		g3:           g3,
		g4:           g4,
		durable:      durable,
		loop:         loop,
		store:        store,
	}
}

func (h *LiveLocalKernelHandler) Handles(path string) bool {
	return h != nil && strings.HasPrefix(path, LiveLocalKernelPrefix)
}

func (h *LiveLocalKernelHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !Authorized(r, h.runtimeToken) {
		WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}

	switch r.URL.Path {
	case LiveLocalKernelPrefix + "/boundary":
		h.handleBoundary(w, r)
	case LiveLocalKernelPrefix + "/capabilities":
		h.handleCapabilities(w, r)
	case LiveLocalKernelPrefix + "/absorption-contract":
		h.handleAbsorptionContract(w, r)
	case LiveLocalKernelPrefix + "/job-orchestration":
		h.handleJobOrchestration(w, r)
	case LiveLocalKernelPrefix + "/g6-retirement-cleanup":
		h.handleG6RetirementCleanup(w, r)
	case LiveLocalKernelPrefix + "/snapshot":
		h.handleSnapshot(w, r)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h *LiveLocalKernelHandler) ReplayAttempts() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.replays
}

func (h *LiveLocalKernelHandler) noteReplay() {
	h.mu.Lock()
	h.replays += 1
	h.mu.Unlock()
}

func (h *LiveLocalKernelHandler) handleBoundary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.noteReplay()
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                    true,
		"testConformanceOnly":            true,
		"internalGateOnly":               true,
		"routePrefix":                    LiveLocalKernelPrefix,
		"kernelLiveScaffoldPrototype":    true,
		"fixtureBackedKernelOnly":        true,
		"durableStoreAvailable":          h.durable != nil,
		"minimalLoopAvailable":           h.loop != nil,
		"providerCacheReplayAvailable":   h.g3 != nil && h.g3.Enabled(),
		"approvalUserInputGateAvailable": h.g4 != nil && h.g4.Enabled(),
		"mcpCatalogRecoveryAvailable":    h.g4 != nil && h.g4.Enabled(),
		"jobSubagentFixtureAvailable":    h.contract.JobOrchestration.FixtureOnly,
		"externalNetworkAllowed":         false,
		"providerLiveCallsAllowed":       false,
		"toolExecutionAllowed":           false,
		"approvalExecutionAllowed":       false,
		"mcpConnectionAllowed":           false,
		"credentialReadAllowed":          false,
		"fileMutationAllowed":            false,
		"realWorkspaceMutationAllowed":   false,
		"electronMainConnected":          false,
		"defaultGoBackendEnabled":        false,
		"rendererVisibleGoRoutesAllowed": false,
		"reasonixPublicProtocolAllowed":  false,
		"productBoundary":                contracts.LiveLocalSidecarKernelProductBoundary(),
	})
}

func (h *LiveLocalKernelHandler) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.noteReplay()
	components := make([]map[string]any, 0, len(h.contract.Components))
	liveCount := 0
	for _, component := range h.contract.Components {
		live := h.componentLive(component.ID)
		if live {
			liveCount += 1
		}
		components = append(components, map[string]any{
			"id":                 component.ID,
			"source":             component.Source,
			"class":              component.Class,
			"route":              component.Route,
			"expectedLive":       component.ExpectedLive,
			"live":               live,
			"matchesExpected":    live == component.ExpectedLive,
			"sideEffectsAllowed": component.SideEffectsAllowed,
		})
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":        true,
		"componentCount":     len(components),
		"liveComponentCount": liveCount,
		"components":         components,
		"allExpectedLive":    liveCount == h.contract.Expected.LiveComponentCount,
		"sideEffects":        h.sideEffects(),
	})
}

func (h *LiveLocalKernelHandler) handleAbsorptionContract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.noteReplay()
	counts := map[string]int{}
	for _, item := range h.contract.ReasonixAbsorption {
		counts[item.Class] += 1
	}
	replaceOrPort := counts["code-port-and-adapt"] + counts["replace"]
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":              true,
		"sourceContractIds":        h.contract.SourceContractIDs,
		"decisions":                h.contract.ReasonixAbsorption,
		"replaceOrPortCount":       replaceOrPort,
		"contractReimplementCount": counts["contract-reimplement"],
		"deferCount":               counts["defer"],
		"rejectCount":              counts["reject"],
		"matchesExpected": replaceOrPort == h.contract.Expected.ReplaceOrPortCount &&
			counts["contract-reimplement"] == h.contract.Expected.ContractReimplementCount &&
			counts["defer"] == h.contract.Expected.DeferCount &&
			counts["reject"] == h.contract.Expected.RejectCount,
	})
}

func (h *LiveLocalKernelHandler) handleJobOrchestration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.noteReplay()
	job := h.contract.JobOrchestration
	forbiddenSurfaceCount := 0
	if job.TopLevelRoutesExposed {
		forbiddenSurfaceCount += 1
	}
	if job.PublicSubagentProtocolAllowed {
		forbiddenSurfaceCount += 1
	}
	if job.BackgroundExecutionAllowed {
		forbiddenSurfaceCount += 1
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                   job.FixtureOnly,
		"parentGoalRequired":            job.ParentGoalRequired,
		"topLevelRoutesExposed":         job.TopLevelRoutesExposed,
		"publicSubagentProtocolAllowed": job.PublicSubagentProtocolAllowed,
		"jobManagerLive":                job.JobManagerLive,
		"backgroundExecutionAllowed":    job.BackgroundExecutionAllowed,
		"lineageContract":               job.LineageContract,
		"expectedStatuses":              job.ExpectedStatuses,
		"expectedTools":                 job.ExpectedTools,
		"forbiddenSurfaceCount":         forbiddenSurfaceCount,
		"matchesExpected":               forbiddenSurfaceCount == h.contract.Expected.ForbiddenSurfaceCount,
	})
}

func (h *LiveLocalKernelHandler) handleG6RetirementCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.noteReplay()
	cleanup := h.contract.RetirementCleanup
	presentForbidden := 0
	for _, item := range cleanup.ForbiddenRedundancy {
		if item.Present {
			presentForbidden += 1
		}
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                     true,
		"g6Ready":                         cleanup.G6Ready,
		"noImmediateProductionDelete":     cleanup.NoImmediateProductionDelete,
		"currentRetainedPaths":            cleanup.CurrentRetainedPaths,
		"deleteAfterG6":                   cleanup.DeleteAfterG6,
		"forbiddenRedundancy":             cleanup.ForbiddenRedundancy,
		"g6Blockers":                      cleanup.G6Blockers,
		"currentRetainedCount":            len(cleanup.CurrentRetainedPaths),
		"deleteAfterG6Count":              len(cleanup.DeleteAfterG6),
		"forbiddenRedundancyCount":        len(cleanup.ForbiddenRedundancy),
		"presentForbiddenRedundancyCount": presentForbidden,
		"g6BlockerCount":                  len(cleanup.G6Blockers),
		"tsRuntimeRetained":               cleanup.NoImmediateProductionDelete && len(cleanup.CurrentRetainedPaths) > 0,
		"matchesExpected": len(cleanup.CurrentRetainedPaths) == h.contract.Expected.CurrentRetainedCount &&
			len(cleanup.DeleteAfterG6) == h.contract.Expected.DeleteAfterG6Count &&
			len(cleanup.ForbiddenRedundancy) == h.contract.Expected.ForbiddenRedundancyCount &&
			presentForbidden == h.contract.Expected.PresentForbiddenRedundancyCount &&
			len(cleanup.G6Blockers) == h.contract.Expected.G6BlockerCount &&
			!cleanup.G6Ready,
	})
}

func (h *LiveLocalKernelHandler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.noteReplay()
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                  true,
		"kernelReplayAttempts":         h.ReplayAttempts(),
		"durableStoreAvailable":        h.durable != nil,
		"minimalLoopAvailable":         h.loop != nil,
		"providerCacheReplayAvailable": h.g3 != nil && h.g3.Enabled(),
		"g4ManagerAvailable":           h.g4 != nil && h.g4.Enabled(),
		"sideEffects":                  h.sideEffects(),
		"productBoundary":              contracts.LiveLocalSidecarKernelProductBoundary(),
	})
}

func (h *LiveLocalKernelHandler) componentLive(id string) bool {
	switch id {
	case "loop-controller":
		return h.loop != nil
	case "event-sink":
		return h.durable != nil
	case "cache-accounting":
		return h.g3 != nil && h.g3.Enabled()
	case "approval-user-input-gate", "mcp-catalog-recovery":
		return h.g4 != nil && h.g4.Enabled()
	case "job-subagent-orchestration":
		return h.contract.JobOrchestration.FixtureOnly &&
			h.contract.JobOrchestration.ParentGoalRequired &&
			!h.contract.JobOrchestration.TopLevelRoutesExposed &&
			!h.contract.JobOrchestration.PublicSubagentProtocolAllowed
	default:
		return false
	}
}

func (h *LiveLocalKernelHandler) sideEffects() map[string]int {
	return map[string]int{
		"providerCallAttempts":       h.store.Snapshot().ProviderCallAttempts,
		"approvalExecutionAttempts":  h.store.Snapshot().ApprovalExecutionAttempts,
		"toolExecutionAttempts":      h.store.Snapshot().ToolExecutionAttempts,
		"mcpConnectionAttempts":      h.store.Snapshot().MCPConnectionAttempts,
		"mcpCredentialAttempts":      h.store.Snapshot().MCPCredentialAttempts,
		"credentialReadAttempts":     h.store.Snapshot().CredentialReadAttempts,
		"fileMutationAttempts":       h.store.Snapshot().FileMutationAttempts,
		"realWorkspaceWriteAttempts": h.store.Snapshot().RealWorkspaceWriteAttempts,
	}
}
