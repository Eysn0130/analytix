package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	controlapp "analytix.local/runtime-go/internal/app/control"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	admission "analytix.local/runtime-go/internal/domain/scheduleadmission"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// This carrier is constructed only after the private Host request and current
// thread authority pass. It never broadens public start/steer request schemas.
type scheduleAdmissionContextKeyV1 struct{}
type scheduleAdmissionCarrierV1 struct {
	host            *runtimeServerHandler
	request         admission.RequestV1
	baselineDigest  string
	targetTurnID    string
	at              time.Time
	inheritedEffort string
	inheritedMode   string
	writeAttempted  bool
}

func (c *scheduleAdmissionCarrierV1) startRequestV1() controlapp.StartTurnRequest {
	r := controlapp.StartTurnRequest{ThreadID: c.request.Origin.ThreadID, Prompt: c.request.Prompt, Async: true, ReasoningEffort: c.inheritedEffort, Mode: c.inheritedMode}
	if e := c.request.Execution; e.ProviderID != nil {
		r.ProviderID = *e.ProviderID
	}
	if e := c.request.Execution; e.Model != nil {
		r.Model = *e.Model
	}
	if e := c.request.Execution; e.ReasoningEffort != nil {
		r.ReasoningEffort = *e.ReasoningEffort
	}
	if e := c.request.Execution; e.Mode != nil {
		r.Mode = *e.Mode
	}
	return controlapp.NormalizeStartTurnRequest(r)
}
func (c *scheduleAdmissionCarrierV1) steerRequestV1() controlapp.SteerTurnRequest {
	return controlapp.NormalizeSteerTurnRequest(controlapp.SteerTurnRequest{ThreadID: c.request.Origin.ThreadID, TurnID: c.targetTurnID, ExpectedTurnID: c.targetTurnID, Text: c.request.Prompt, ClientUserMessageID: c.request.AttemptID})
}
func scheduleCarrierV1(ctx context.Context, h *runtimeServerHandler) (*scheduleAdmissionCarrierV1, error) {
	if ctx == nil || ctx.Value(scheduleAdmissionContextKeyV1{}) == nil {
		return nil, nil
	}
	c, ok := ctx.Value(scheduleAdmissionContextKeyV1{}).(*scheduleAdmissionCarrierV1)
	if !ok || c == nil || c.host != h || admission.ValidateRequestV1(c.request) != nil || c.request.Kind != "submit" {
		return nil, admission.ErrInvalidV1
	}
	return c, nil
}
func scheduleIdleV1(thread map[string]any) bool {
	if thread == nil || stringField(thread, "status") == "deleted" {
		return false
	}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return false
	}
	if len(turns) == 0 {
		return true
	}
	t, _ := turns[len(turns)-1].(map[string]any)
	switch stringField(t, "status") {
	case "completed", "failed", "aborted", "interrupted":
		return true
	default:
		return false
	}
}

// No speculative provider preference fallback for a running turn: absent
// frozen facts reject explicit overrides instead of silently ignoring them.
func scheduleExecutionMatchesV1(e admission.ExecutionV1, t map[string]any) bool {
	for k, v := range map[string]*string{"providerId": e.ProviderID, "model": e.Model, "reasoningEffort": e.ReasoningEffort, "mode": e.Mode} {
		if v != nil && stringField(t, k) != *v {
			return false
		}
	}
	return true
}
func (h *runtimeServerHandler) ScheduleAdmissionV1() http.Handler {
	leaf := httpapi.ScheduleAdmissionHandlerV1{Admit: h.admitScheduleV1}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.serveWithRuntimeMaintenanceAdmission(w, r, leaf) })
}
func (h *runtimeServerHandler) readAuthorizedScheduleV1(ctx context.Context, r admission.RequestV1) (map[string]any, *admission.ReceiptV1, error) {
	if h == nil || h.store == nil || ctx == nil || ctx.Err() != nil || h.runtimeControl().ShuttingDown() || h.turnSecurity.Observer == nil {
		return nil, nil, errors.New("schedule authority unavailable")
	}
	principal, e := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if e != nil {
		return nil, nil, e
	}
	first, e := h.store.ReadPrimaryThreadSnapshotV1(ctx, r.Origin.ThreadID)
	if e != nil {
		return nil, nil, e
	}
	original, e := turnapp.FrozenSecurityContextForTurn(first.Thread, r.Origin.TurnID)
	if e != nil || original.TenantID != principal.TenantID || original.UserID != principal.UserID {
		return nil, nil, errors.New("schedule origin unavailable")
	}
	observation, e := h.turnSecurity.Observer.Observe(stringField(first.Thread, "workspace"))
	if e != nil {
		return nil, nil, e
	}
	release, e := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, r.Origin.ThreadID, observation.WorkspaceRealPath, principal.TenantID, principal.UserID)
	if e != nil || release == nil {
		return nil, nil, errors.New("schedule current scope unavailable")
	}
	defer release()
	h.store.mu.Lock()
	thread, receipt, e := h.store.readScheduleAdmissionNoLockV1(ctx, r)
	h.store.mu.Unlock()
	if e != nil {
		return nil, nil, e
	}
	current, e := snapshotapp.CurrentOwnContextV1(thread, original)
	if e != nil || current.WorkspaceRealPath != observation.WorkspaceRealPath || stringField(thread, "status") == "deleted" {
		return nil, nil, errors.New("schedule current context unavailable")
	}
	if e = turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: current, Workspace: current.WorkspaceRealPath}); e != nil {
		return nil, nil, e
	}
	if ctx.Err() != nil || h.runtimeControl().ShuttingDown() || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return nil, nil, errors.New("schedule authority changed")
	}
	return thread, receipt, nil
}
func (h *runtimeServerHandler) admitScheduleV1(ctx context.Context, r admission.RequestV1) admission.ResponseV1 {
	if admission.ValidateRequestV1(r) != nil {
		return admission.ResultV1("invalid", nil)
	}
	thread, receipt, e := h.readAuthorizedScheduleV1(ctx, r)
	if errors.Is(e, admission.ErrConflictV1) {
		return admission.ResultV1("conflict", nil)
	}
	if e != nil {
		return admission.ResultV1("unknown", nil)
	}
	if receipt != nil {
		return admission.ResultV1("accepted", receipt)
	}
	if r.Kind == "query" {
		return admission.ResultV1("not_found", nil)
	}
	c := &scheduleAdmissionCarrierV1{host: h, request: r, at: time.Now().UTC(), inheritedEffort: stringField(thread, "reasoningEffort"), inheritedMode: stringField(thread, "mode")}
	carrierCtx := context.WithValue(ctx, scheduleAdmissionContextKeyV1{}, c)
	turns, _ := thread["turns"].([]any)
	var latest map[string]any
	if len(turns) > 0 {
		latest, _ = turns[len(turns)-1].(map[string]any)
	}
	if stringField(latest, "status") == "running" {
		if !scheduleExecutionMatchesV1(r.Execution, latest) {
			return admission.ResultV1("rejected", nil)
		}
		c.targetTurnID = stringField(latest, "id")
		_, e = h.runtimeControl().SteerTurn(carrierCtx, c.steerRequestV1())
	} else {
		if !scheduleIdleV1(thread) {
			return admission.ResultV1("rejected", nil)
		}
		c.baselineDigest, e = turnstartapp.BaselineDigest(thread)
		if e == nil {
			_, e = h.runtimeControl().SendTurn(carrierCtx, c.startRequestV1())
		}
	}
	// A post-primary sidecar/event/transport error is not a failed delivery.
	// Only strict primary readback can decide; never retry this dispatch here.
	_, receipt, readErr := h.readAuthorizedScheduleV1(ctx, r)
	if errors.Is(e, admission.ErrConflictV1) || errors.Is(readErr, admission.ErrConflictV1) {
		return admission.ResultV1("conflict", nil)
	}
	if readErr == nil && receipt != nil {
		return admission.ResultV1("accepted", receipt)
	}
	if readErr != nil || c.writeAttempted {
		return admission.ResultV1("unknown", nil)
	}
	return admission.ResultV1("rejected", nil)
}

// Called under the same mutex as every primary admission mutation. It never
// hydrates a sidecar or treats an SSE event as a receipt.
func (s *DurableEventSessionStore) readScheduleAdmissionNoLockV1(ctx context.Context, r admission.RequestV1) (map[string]any, *admission.ReceiptV1, error) {
	snap, e := s.ReadPrimaryThreadSnapshotV1(ctx, r.Origin.ThreadID)
	if e != nil {
		return nil, nil, e
	}
	thread := snap.Thread
	// Strict primary decoding keeps json.Number. Only closed V1
	// steering version fields need the existing native codec's integer shape.
	// Their exact representation is checked; message content and other values
	// are never normalized, and native schema/signature validation still runs.
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		entries, _ := turn["steering"].([]any)
		for i, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				_, legacy := rawEntry.(string)
				_, marked := turn[admission.MarkerFieldV1]
				if legacy && !marked && turnapp.IsTerminalStatus(stringField(turn, "status")) {
					continue
				}
				return nil, nil, admission.ErrInvalidV1
			}
			for _, key := range []string{"projectionVersion", "jobProjectionVersion"} {
				if number, ok := entry[key].(json.Number); ok && number.String() != "1" {
					return nil, nil, admission.ErrInvalidV1
				}
			}
			// A JSON round trip would round json.Number before the exact check.
			// Copy fields without changing any nested values or numeric types.
			adapted := make(map[string]any, len(entry))
			for key, value := range entry {
				adapted[key] = value
			}
			for _, key := range []string{"projectionVersion", "jobProjectionVersion"} {
				if _, ok := entry[key].(json.Number); ok {
					adapted[key] = 1
				}
			}
			entries[i] = adapted
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") != "user_message" || stringField(item, "delivery") != "steer" {
				continue
			}
			if number, ok := item["steeringProjectionVersion"].(json.Number); ok {
				if number.String() != "1" {
					return nil, nil, admission.ErrInvalidV1
				}
				item["steeringProjectionVersion"] = 1
			}
		}
	}
	if domainthread.ValidatePrimaryIdentityV1(r.Origin.ThreadID, thread) != nil {
		return nil, nil, admission.ErrInvalidV1
	}
	receipt, e := s.scheduleReceiptFromPrimaryV1(thread, r)
	return thread, receipt, e
}
func (s *DurableEventSessionStore) scheduleReceiptFromPrimaryV1(thread map[string]any, r admission.RequestV1) (*admission.ReceiptV1, error) {
	turns, _ := thread["turns"].([]any)
	seen := map[string]bool{}
	seenMessages := map[string]bool{}
	var found *admission.ReceiptV1
	for _, raw := range turns {
		t, _ := raw.(map[string]any)
		value, present := t[admission.MarkerFieldV1]
		if !present {
			continue
		}
		markers, ok := value.([]any)
		if !ok || len(markers) == 0 {
			return nil, admission.ErrInvalidV1
		}
		for _, rawMarker := range markers {
			m, e := admission.DecodeMarkerV1(rawMarker)
			if e != nil {
				return nil, errors.Join(e, errors.New("schedule primary marker codec"))
			}
			receipt := m.Receipt
			if seen[receipt.AttemptID] {
				return nil, admission.ErrConflictV1
			}
			seen[receipt.AttemptID] = true
			if seenMessages[receipt.MessageID] {
				return nil, admission.ErrConflictV1
			}
			seenMessages[receipt.MessageID] = true
			frozen, e := turnapp.FrozenSecurityContextForTurn(thread, receipt.TurnID)
			if e != nil || frozen.ContextDigest != m.ContextDigest || receipt.ThreadID != stringField(thread, "id") || receipt.TurnID != stringField(t, "id") {
				return nil, errors.Join(admission.ErrInvalidV1, errors.New("schedule primary marker context"))
			}
			count := 0
			if receipt.Delivery == "start" {
				items, _ := t["items"].([]any)
				for i, rawItem := range items {
					item, _ := rawItem.(map[string]any)
					if stringField(item, "id") != receipt.MessageID {
						continue
					}
					count++
					if i != 0 || receipt.MessageID != "item_"+receipt.TurnID+"_user" || stringField(item, "threadId") != receipt.ThreadID || stringField(item, "kind") != "user_message" || stringField(item, "role") != "user" || stringField(item, "turnId") != receipt.TurnID || admission.TextDigestV1(stringField(item, "text")) != m.ContentDigest || stringField(item, "createdAt") != receipt.DeliveredAt {
						return nil, errors.Join(admission.ErrInvalidV1, errors.New("schedule primary start item binding"))
					}
				}
			} else {
				entries, _ := t["steering"].([]any)
				for _, rawEntry := range entries {
					entry, _ := rawEntry.(map[string]any)
					if stringField(entry, "id") != receipt.MessageID {
						continue
					}
					count++
					if stringField(entry, "clientUserMessageId") != receipt.AttemptID || receipt.MessageID != domainsteering.EntryIDV1(receipt.TurnID, receipt.AttemptID) || admission.TextDigestV1(stringField(entry, "text")) != m.ContentDigest || stringField(entry, "admittedAt") != receipt.DeliveredAt {
						return nil, errors.Join(admission.ErrInvalidV1, errors.New("schedule primary steering identity"))
					}
					switch stringField(entry, "status") {
					case "pending":
						if domainsteering.ValidatePendingEntryForContextV1(entry, m.ContextDigest) != nil || s.verifySteeringEntryAuthorityV1(entry, m.ContextDigest) != nil {
							return nil, errors.Join(admission.ErrInvalidV1, errors.New("schedule primary pending steering signature"))
						}
					case "promoted":
						matches := 0
						items, _ := t["items"].([]any)
						for _, rawItem := range items {
							item, _ := rawItem.(map[string]any)
							if stringField(item, "id") == stringField(entry, "promotedItemId") {
								matches++
								if _, e := domainsteering.NewPromotionCommitV1(receipt.ThreadID, receipt.TurnID, m.ContextDigest, entry, item, s.verifySteeringEntryAuthorityV1); e != nil {
									return nil, admission.ErrInvalidV1
								}
							}
						}
						if matches != 1 {
							return nil, admission.ErrInvalidV1
						}
					case "cancelled":
						pending := cloneMap(entry)
						pending["status"] = "pending"
						delete(pending, "cancelledAt")
						delete(pending, "cancelReason")
						if !turnapp.IsTerminalStatus(stringField(t, "status")) || stringField(entry, "cancelledAt") != stringField(t, "finishedAt") || stringField(entry, "cancelReason") != stringField(t, "status") || domainsteering.ValidatePendingEntryForContextV1(pending, m.ContextDigest) != nil || s.verifySteeringEntryAuthorityV1(pending, m.ContextDigest) != nil {
							return nil, admission.ErrInvalidV1
						}
					default:
						return nil, admission.ErrInvalidV1
					}
				}
			}
			if count != 1 {
				return nil, admission.ErrInvalidV1
			}
			if receipt.AttemptID == r.AttemptID {
				if receipt.TaskID != r.TaskID || !reflect.DeepEqual(m.Origin, r.Origin) || m.InputDigest != admission.InputDigestV1(r) {
					return nil, admission.ErrConflictV1
				}
				copy := receipt
				found = &copy
			}
		}
	}
	return found, nil
}

// The wrapper reuses the ordinary steering orchestration and event owner;
// only the private atomic primary write gains the Schedule marker.
type scheduleSteeringStoreV1 struct {
	*DurableEventSessionStore
	carrier *scheduleAdmissionCarrierV1
}

func (s scheduleSteeringStoreV1) AdmitSteeringEntryForContext(threadID, turnID, expectedTurnID, contextDigest string, entry map[string]any) (map[string]any, error) {
	return s.DurableEventSessionStore.admitScheduleSteeringEntryV1(s.carrier, threadID, turnID, expectedTurnID, contextDigest, entry)
}

// Schedule writes still use the existing primary append CAS.
type scheduleTurnStoreV1 struct {
	*DurableEventSessionStore
	carrier *scheduleAdmissionCarrierV1
}

func (s scheduleTurnStoreV1) AppendTurnToThreadIfBaseline(threadID string, turn map[string]any, providerID string, patch map[string]any, workspace, digest string) error {
	return s.DurableEventSessionStore.appendScheduleTurnV1(s.carrier, threadID, turn, providerID, patch, workspace, digest)
}
