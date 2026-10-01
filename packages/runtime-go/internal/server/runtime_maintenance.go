package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const runtimeMaintenanceLeaseLifetime = 30 * time.Second
const runtimeMaintenancePrepareTimeout = 8 * time.Second

type runtimeMaintenanceAdmittedContextKey struct{}

// RuntimeMaintenanceAdmissionHandlerV1 covers the production LocalDisplayMux,
// whose private routes do not pass through the Core runtime handler.
type RuntimeMaintenanceAdmissionHandlerV1 struct {
	owner *runtimeServerHandler
	next  http.Handler
}

func WrapRuntimeMaintenanceAdmissionV1(owner http.Handler, next http.Handler) (http.Handler, error) {
	h, ok := owner.(*runtimeServerHandler)
	if !ok || next == nil {
		return nil, errors.New("runtime maintenance owner is unavailable")
	}
	return &RuntimeMaintenanceAdmissionHandlerV1{owner: h, next: next}, nil
}

func (h *RuntimeMaintenanceAdmissionHandlerV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.owner.serveWithRuntimeMaintenanceAdmission(w, r, h.next)
}

func (h *RuntimeMaintenanceAdmissionHandlerV1) Shutdown(ctx context.Context) error {
	if lifecycle, ok := h.next.(interface{ Shutdown(context.Context) error }); ok {
		return lifecycle.Shutdown(ctx)
	}
	return nil
}

// The HTTP fence closes before any owner is inspected. Existing mutations
// finish first; later mutations cannot enter until the lease is released.
func (h *runtimeServerHandler) serveWithRuntimeMaintenanceAdmission(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.Context().Value(runtimeMaintenanceAdmittedContextKey{}) != nil {
		next.ServeHTTP(w, r)
		return
	}
	admitted, request := h.admitRuntimeMaintenanceRequest(w, r)
	if !admitted {
		return
	}
	if !runtimeMaintenanceUntrackedRead(r) && r.URL.Path != "/v1/runtime/quiescence" {
		defer h.finishRuntimeMaintenanceRequest(request)
	}
	next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), runtimeMaintenanceAdmittedContextKey{}, true)))
}

func (h *runtimeServerHandler) admitRuntimeMaintenanceRequest(w http.ResponseWriter, r *http.Request) (bool, *http.Request) {
	if runtimeMaintenanceUntrackedRead(r) || r.URL.Path == "/v1/runtime/quiescence" {
		return true, r
	}
	h.maintenanceMu.Lock()
	defer h.maintenanceMu.Unlock()
	if h.maintenanceClosed {
		httpapi.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"code": "runtime_maintenance"})
		return false, r
	}
	if h.mutatingRequests == 0 {
		h.maintenanceDrained = make(chan struct{})
	}
	h.mutatingRequests++
	if runtimeMaintenanceSSE(r) {
		requestContext, cancel := context.WithCancel(r.Context())
		h.maintenanceNextID++
		if h.maintenanceSSE == nil {
			h.maintenanceSSE = make(map[uint64]context.CancelFunc)
		}
		h.maintenanceSSE[h.maintenanceNextID] = cancel
		return true, r.WithContext(context.WithValue(requestContext, runtimeMaintenanceSSEContextKey{}, h.maintenanceNextID))
	}
	return true, r
}

func runtimeMaintenanceUntrackedRead(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == "/health"
}

func runtimeMaintenanceSSE(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		strings.HasPrefix(r.URL.Path, "/v1/threads/") &&
		strings.HasSuffix(r.URL.Path, "/events")
}

type runtimeMaintenanceSSEContextKey struct{}

func (h *runtimeServerHandler) finishRuntimeMaintenanceRequest(r *http.Request) {
	h.maintenanceMu.Lock()
	if id, ok := r.Context().Value(runtimeMaintenanceSSEContextKey{}).(uint64); ok {
		if cancel := h.maintenanceSSE[id]; cancel != nil {
			cancel()
			delete(h.maintenanceSSE, id)
		}
	}
	if h.mutatingRequests > 0 {
		h.mutatingRequests--
		if h.mutatingRequests == 0 && h.maintenanceDrained != nil {
			close(h.maintenanceDrained)
			h.maintenanceDrained = nil
		}
	}
	h.maintenanceMu.Unlock()
}

func (h *runtimeServerHandler) handleRuntimeQuiescence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpapi.MethodNotAllowed(w)
		return
	}
	if h.insecure || h.runtimeToken == "" || r.URL.RawQuery != "" {
		httpapi.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"code": "runtime_maintenance_unavailable"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1025))
	if err != nil || len(raw) > 1024 {
		httpapi.WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "invalid_maintenance_request"})
		return
	}
	decoded, err := jsonstrict.DecodeValue(raw, jsonstrict.Options{
		RequireObject: true, MaxBytes: 1024, MaxDepth: 2, MaxTokens: 8, MaxStringBytes: 128,
	})
	request, ok := decoded.(map[string]any)
	if err != nil || !ok {
		httpapi.WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "invalid_maintenance_request"})
		return
	}
	switch request["operation"] {
	case "prepare":
		if len(request) != 1 {
			httpapi.WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "invalid_maintenance_request"})
			return
		}
		h.prepareRuntimeMaintenance(w, r)
	case "release":
		lease, valid := request["lease"].(string)
		if len(request) != 2 || !valid || len(lease) != 43 {
			httpapi.WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "invalid_maintenance_request"})
			return
		}
		if !h.releaseRuntimeMaintenance(lease) {
			httpapi.WriteJSON(w, http.StatusConflict, map[string]any{"code": "maintenance_lease_mismatch"})
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"schemaVersion": 1, "released": true, "runtimePid": os.Getpid()})
	case "commit-stop":
		lease, valid := request["lease"].(string)
		if len(request) != 2 || !valid || len(lease) != 43 {
			httpapi.WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "invalid_maintenance_request"})
			return
		}
		if !h.commitRuntimeMaintenanceStop(lease) {
			httpapi.WriteJSON(w, http.StatusConflict, map[string]any{"code": "maintenance_lease_mismatch"})
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"schemaVersion": 1, "committed": true, "runtimePid": os.Getpid()})
	default:
		httpapi.WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "invalid_maintenance_request"})
	}
}

func (h *runtimeServerHandler) prepareRuntimeMaintenance(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), runtimeMaintenancePrepareTimeout)
	defer cancel()
	h.maintenanceMu.Lock()
	if h.maintenanceClosed || h.mutatingRequests != len(h.maintenanceSSE) {
		h.maintenanceMu.Unlock()
		httpapi.WriteJSON(w, http.StatusConflict, map[string]any{"code": "runtime_busy"})
		return
	}
	h.maintenanceClosed = true
	h.maintenanceLastReleasedLease = ""
	h.maintenanceAttempt++
	attempt := h.maintenanceAttempt
	h.maintenancePrepareDeadline = time.Now().Add(runtimeMaintenancePrepareTimeout)
	h.maintenanceTimer = time.AfterFunc(runtimeMaintenancePrepareTimeout, func() {
		h.expireRuntimeMaintenanceAttempt(attempt)
	})
	for _, cancelSSE := range h.maintenanceSSE {
		cancelSSE()
	}
	drained := h.maintenanceDrained
	h.maintenanceMu.Unlock()
	stopOnCancel := context.AfterFunc(ctx, func() { h.expireRuntimeMaintenanceAttempt(attempt) })
	defer stopOnCancel()
	if drained != nil {
		select {
		case <-drained:
		case <-ctx.Done():
			h.releaseRuntimeMaintenanceAttempt(attempt)
			httpapi.WriteJSON(w, http.StatusConflict, map[string]any{"code": "runtime_busy"})
			return
		}
	}

	if !h.beginRuntimeOwnerMaintenance(attempt) {
		h.releaseRuntimeMaintenanceAttempt(attempt)
		httpapi.WriteJSON(w, http.StatusConflict, map[string]any{"code": "runtime_busy"})
		return
	}
	if h.store == nil {
		h.releaseRuntimeMaintenanceAttempt(attempt)
		httpapi.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"code": "runtime_maintenance_unavailable"})
		return
	}
	idle, err := h.store.RuntimeTurnsIdleForMaintenance(ctx)
	if err != nil || !idle {
		h.releaseRuntimeMaintenanceAttempt(attempt)
		code, status := "runtime_busy", http.StatusConflict
		if err != nil {
			code, status = "runtime_maintenance_unavailable", http.StatusServiceUnavailable
		}
		httpapi.WriteJSON(w, status, map[string]any{"code": code})
		return
	}
	leaseBytes := make([]byte, 32)
	if _, err = rand.Read(leaseBytes); err != nil {
		h.releaseRuntimeMaintenanceAttempt(attempt)
		httpapi.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"code": "runtime_maintenance_unavailable"})
		return
	}
	lease := base64.RawURLEncoding.EncodeToString(leaseBytes)
	expiresAt := time.Now().Add(runtimeMaintenanceLeaseLifetime)
	for index := range leaseBytes {
		leaseBytes[index] = 0
	}
	h.maintenanceMu.Lock()
	if h.maintenanceAttempt != attempt || !h.maintenanceClosed || h.maintenanceLease != "" ||
		ctx.Err() != nil || !time.Now().Before(h.maintenancePrepareDeadline) {
		if h.maintenanceAttempt == attempt && h.maintenanceClosed && h.maintenanceLease == "" {
			h.releaseRuntimeMaintenanceLocked()
		}
		h.maintenanceMu.Unlock()
		httpapi.WriteJSON(w, http.StatusConflict, map[string]any{"code": "runtime_busy"})
		return
	}
	h.maintenanceLease = lease
	h.maintenanceCommitted = false
	h.maintenanceExpiresAt = expiresAt
	h.maintenancePrepareDeadline = time.Time{}
	if h.maintenanceTimer != nil {
		h.maintenanceTimer.Stop()
	}
	h.maintenanceTimer = time.AfterFunc(time.Until(expiresAt), func() {
		h.expireRuntimeMaintenance(lease)
	})
	h.maintenanceMu.Unlock()
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"schemaVersion": 1, "runtimePid": os.Getpid(), "state": "idle", "lease": lease,
		"expiresAtUnixMs": expiresAt.UnixMilli(),
	})
}

func (h *runtimeServerHandler) beginRuntimeOwnerMaintenance(attempt uint64) bool {
	h.maintenanceMu.Lock()
	defer h.maintenanceMu.Unlock()
	if h.maintenanceAttempt != attempt || !h.maintenanceClosed || h.maintenanceLease != "" ||
		!time.Now().Before(h.maintenancePrepareDeadline) {
		return false
	}
	if !h.runtimeControl().BeginMaintenanceIfIdle() {
		return false
	}
	if !h.runtimeSubagentState().BeginMaintenanceIfIdle() {
		h.runtimeControl().EndMaintenance()
		return false
	}
	if h.jobs != nil && !h.jobs.RuntimeIdleForMaintenance() {
		h.runtimeSubagentState().EndMaintenance()
		h.runtimeControl().EndMaintenance()
		return false
	}
	return true
}

func (h *runtimeServerHandler) releaseRuntimeMaintenanceAttempt(attempt uint64) {
	h.maintenanceMu.Lock()
	defer h.maintenanceMu.Unlock()
	if h.maintenanceAttempt == attempt && h.maintenanceClosed && h.maintenanceLease == "" {
		h.releaseRuntimeMaintenanceLocked()
	}
}

func (h *runtimeServerHandler) expireRuntimeMaintenanceAttempt(attempt uint64) {
	h.releaseRuntimeMaintenanceAttempt(attempt)
}

// CommitStop converts the expiring preparation into a non-expiring process
// fence. The exact caller must then stop this process or explicitly release.
func (h *runtimeServerHandler) commitRuntimeMaintenanceStop(lease string) bool {
	h.maintenanceMu.Lock()
	defer h.maintenanceMu.Unlock()
	if !h.maintenanceClosed || h.maintenanceCommitted || h.maintenanceLease != lease {
		return false
	}
	if !time.Now().Before(h.maintenanceExpiresAt) {
		h.releaseRuntimeMaintenanceLocked()
		return false
	}
	if h.maintenanceTimer != nil {
		h.maintenanceTimer.Stop()
		h.maintenanceTimer = nil
	}
	h.maintenanceCommitted = true
	return true
}

func (h *runtimeServerHandler) releaseRuntimeMaintenance(lease string) bool {
	h.maintenanceMu.Lock()
	defer h.maintenanceMu.Unlock()
	if !h.maintenanceClosed {
		return lease != "" && lease == h.maintenanceLastReleasedLease
	}
	if lease != h.maintenanceLease {
		return false
	}
	h.releaseRuntimeMaintenanceLocked()
	return true
}

func (h *runtimeServerHandler) expireRuntimeMaintenance(lease string) {
	h.maintenanceMu.Lock()
	defer h.maintenanceMu.Unlock()
	if !h.maintenanceClosed || h.maintenanceCommitted || lease != h.maintenanceLease {
		return
	}
	h.releaseRuntimeMaintenanceLocked()
}

func (h *runtimeServerHandler) releaseRuntimeMaintenanceLocked() {
	if h.maintenanceTimer != nil {
		h.maintenanceTimer.Stop()
		h.maintenanceTimer = nil
	}
	h.runtimeSubagentState().EndMaintenance()
	h.runtimeControl().EndMaintenance()
	if h.maintenanceLease != "" {
		h.maintenanceLastReleasedLease = h.maintenanceLease
	}
	h.maintenanceLease = ""
	h.maintenanceCommitted = false
	h.maintenanceExpiresAt = time.Time{}
	h.maintenancePrepareDeadline = time.Time{}
	h.maintenanceAttempt++
	h.maintenanceClosed = false
}
