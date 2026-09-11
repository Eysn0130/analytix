//go:build !analytix_prod

package livelocal

import (
	"context"
	"net/http"
	"strings"
	"sync"

	agent "analytix.local/runtime-go/internal/agent"
	contracts "analytix.local/runtime-go/internal/contracts"
	jobs "analytix.local/runtime-go/internal/jobs"
	mcp "analytix.local/runtime-go/internal/mcp"
	providerscript "analytix.local/runtime-go/internal/testsupport/providerscript"
	upstreamaudit "analytix.local/runtime-go/internal/upstreamaudit"
)

const LiveProductionCandidatePrefix = "/v1/internal/go-production-candidate"

type liveProductionCandidateCounters struct {
	providerLiveRuns      int
	durableReplayRuns     int
	approvalGateRuns      int
	mcpManagerRuns        int
	jobLineageRuns        int
	checklistRuns         int
	canaryRuns            int
	externalCredentialUse int
}

type LiveProductionCandidateHandler struct {
	runtimeToken string
	store        *DurableEventSessionStore

	mu       sync.Mutex
	counters liveProductionCandidateCounters
}

func NewLiveProductionCandidateHandler(runtimeToken string, store *DurableEventSessionStore) *LiveProductionCandidateHandler {
	if strings.TrimSpace(runtimeToken) == "" {
		runtimeToken = DefaultRuntimeToken
	}
	return &LiveProductionCandidateHandler{
		runtimeToken: runtimeToken,
		store:        store,
	}
}

func (h *LiveProductionCandidateHandler) Handles(path string) bool {
	return h != nil && strings.HasPrefix(path, LiveProductionCandidatePrefix)
}

func (h *LiveProductionCandidateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !Authorized(r, h.runtimeToken) {
		WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}

	switch r.URL.Path {
	case LiveProductionCandidatePrefix + "/boundary":
		h.handleBoundary(w, r)
	case LiveProductionCandidatePrefix + "/provider-live":
		h.handleProviderLive(w, r)
	case LiveProductionCandidatePrefix + "/durable-replay":
		h.handleDurableReplay(w, r)
	case LiveProductionCandidatePrefix + "/approval-user-input":
		h.handleApprovalUserInput(w, r)
	case LiveProductionCandidatePrefix + "/mcp-manager":
		h.handleMCPManager(w, r)
	case LiveProductionCandidatePrefix + "/job-lineage":
		h.handleJobLineage(w, r)
	case LiveProductionCandidatePrefix + "/single-baseline-checklist":
		h.handleSingleBaselineChecklist(w, r)
	case LiveProductionCandidatePrefix + "/canary":
		h.handleCanary(w, r)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h *LiveProductionCandidateHandler) Snapshot() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return map[string]any{
		"runtimeGoContractParitySlice": true,
		"providerLiveRuns":             h.counters.providerLiveRuns,
		"durableReplayRuns":            h.counters.durableReplayRuns,
		"approvalGateRuns":             h.counters.approvalGateRuns,
		"mcpManagerRuns":               h.counters.mcpManagerRuns,
		"jobLineageRuns":               h.counters.jobLineageRuns,
		"checklistRuns":                h.counters.checklistRuns,
		"canaryRuns":                   h.counters.canaryRuns,
		"externalCredentialUse":        h.counters.externalCredentialUse,
	}
}

func (h *LiveProductionCandidateHandler) bump(fn func(*liveProductionCandidateCounters)) {
	h.mu.Lock()
	fn(&h.counters)
	h.mu.Unlock()
}

func (h *LiveProductionCandidateHandler) handleBoundary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"runtimeGoContractParitySlice":       true,
		"internalGateOnly":                   true,
		"defaultGoBackendEnabled":            false,
		"rendererVisibleGoRoutesAllowed":     false,
		"reasonixPublicProtocolAllowed":      false,
		"rendererPreloadMainBridgeUnchanged": true,
		"analytixServeContractUnchanged":     true,
		"usesContractReplayProviderServer":   true,
		"usesContractReplayMCPTransport":     true,
		"usesRealDurableEventSink":           h.store != nil,
		"contractReplayOnly":                 false,
		"testConformanceOnly":                false,
		"readsRealAPIKeys":                   false,
		"externalNetworkAllowed":             false,
		"credentialReadAllowed":              false,
		"productBoundary":                    contracts.LiveProductionCandidateProductBoundary(),
	})
}

func (h *LiveProductionCandidateHandler) handleProviderLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	result, err := providerscript.RunLocalProviderContract(context.Background())
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "provider_live_failed", "message": err.Error()})
		return
	}
	h.bump(func(c *liveProductionCandidateCounters) { c.providerLiveRuns++ })
	WriteJSON(w, http.StatusOK, result)
}

func (h *LiveProductionCandidateHandler) handleDurableReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.store == nil {
		WriteJSON(w, http.StatusPreconditionFailed, map[string]any{"code": "durable_store_unavailable", "message": "durable store is required"})
		return
	}
	providerContract, err := providerscript.RunLocalProviderContract(context.Background())
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "durable_replay_failed", "message": err.Error()})
		return
	}
	result, err := RunDurableReplayContract(providerContract, h.store)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "durable_replay_failed", "message": err.Error()})
		return
	}
	h.bump(func(c *liveProductionCandidateCounters) { c.durableReplayRuns++ })
	WriteJSON(w, http.StatusOK, result)
}

func (h *LiveProductionCandidateHandler) handleApprovalUserInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	result := agent.RunApprovalUserInputContractExercise()
	h.bump(func(c *liveProductionCandidateCounters) { c.approvalGateRuns++ })
	WriteJSON(w, http.StatusOK, result)
}

func (h *LiveProductionCandidateHandler) handleMCPManager(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	result := mcp.RunManagerContractExercise()
	h.bump(func(c *liveProductionCandidateCounters) { c.mcpManagerRuns++ })
	WriteJSON(w, http.StatusOK, result)
}

func (h *LiveProductionCandidateHandler) handleJobLineage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	result := jobs.RunLineageContractExercise()
	h.bump(func(c *liveProductionCandidateCounters) { c.jobLineageRuns++ })
	WriteJSON(w, http.StatusOK, result)
}

func (h *LiveProductionCandidateHandler) handleSingleBaselineChecklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.bump(func(c *liveProductionCandidateCounters) { c.checklistRuns++ })
	WriteJSON(w, http.StatusOK, upstreamaudit.SingleBaselineChecklist())
}

func (h *LiveProductionCandidateHandler) handleCanary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	h.bump(func(c *liveProductionCandidateCounters) { c.canaryRuns++ })

	providerContract, providerErr := providerscript.RunLocalProviderContract(context.Background())
	durable := map[string]any{"ok": false}
	var durableErr error
	if h.store != nil {
		durableProvider, err := providerscript.RunLocalProviderContract(context.Background())
		if err != nil {
			durableErr = err
		} else {
			durable, durableErr = RunDurableReplayContract(durableProvider, h.store)
		}
	}
	approval := agent.RunApprovalUserInputContractExercise()
	mcpContract := mcp.RunManagerContractExercise()
	jobContract := jobs.RunLineageContractExercise()
	checklist := upstreamaudit.SingleBaselineChecklist()

	checks := []string{}
	ok := true
	if providerErr != nil || providerContract["providerFamiliesCovered"] != true {
		ok = false
	} else {
		checks = append(checks, "contract-provider-server")
	}
	if durableErr != nil || durable["durableReplayFromEventSink"] != true {
		ok = false
	} else {
		checks = append(checks, "durable-replay")
	}
	if approval["deniedToolExecuted"] != false || approval["submittedAnswersPersistedInEvents"] != false {
		ok = false
	} else {
		checks = append(checks, "approval-user-input-manager")
	}
	if mcpContract["contractReplayMCPTransportUsed"] != true || mcpContract["productionFallbackMCPTransportUsed"] != false || mcpContract["deniedMCPToolExecuted"] != false {
		ok = false
	} else {
		checks = append(checks, "contract-mcp-manager")
	}
	if jobContract["parentGoalLineageRequired"] != true || jobContract["topLevelRouteExposed"] != false {
		ok = false
	} else {
		checks = append(checks, "job-lineage")
	}
	if checklist["presentForbiddenRedundancyCount"] != 0 {
		ok = false
	} else {
		checks = append(checks, "single-baseline-checklist")
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"ok":                                 ok,
		"checks":                             checks,
		"provider":                           providerContract,
		"providerError":                      errorString(providerErr),
		"durable":                            durable,
		"durableError":                       errorString(durableErr),
		"approvalUserInput":                  approval,
		"mcp":                                mcpContract,
		"jobLineage":                         jobContract,
		"singleBaselineChecklist":            checklist,
		"defaultGoBackendEnabled":            false,
		"rendererVisibleGoRoutesAllowed":     false,
		"reasonixPublicProtocolAllowed":      false,
		"rendererPreloadMainBridgeUnchanged": true,
		"runtimeGoContractParitySlice":       true,
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
