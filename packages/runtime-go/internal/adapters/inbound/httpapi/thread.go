package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type ThreadService interface {
	List(threadapp.ListInput) ([]map[string]any, error)
	Create(map[string]any) (map[string]any, error)
	Get(threadID string) (map[string]any, error)
	Patch(context.Context, string, map[string]any) (map[string]any, error)
	Delete(context.Context, string) (bool, error)
	Fork(threadID string, request map[string]any) (map[string]any, error)
	Rewind(context.Context, string, string) (map[string]any, error)
	GetGoal(threadID string) (map[string]any, error)
	SetGoal(threadID string, patch map[string]any) (map[string]any, error)
	ClearGoal(threadID string) (bool, error)
	GetTodos(threadID string) (map[string]any, error)
	SetTodos(threadID string, items []any) (map[string]any, error)
	ClearTodos(threadID string) (bool, error)
	Compact(context.Context, string, string) (map[string]any, error)
}

type ThreadHandlers struct {
	Service                      ThreadService
	HydrateAcceptedFinalDelivery func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error)
}

func (h ThreadHandlers) HandleThreads(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_service_missing", "message": "thread service missing"})
		return
	}
	if r.Method == http.MethodPost {
		h.handleCreate(w, r)
		return
	}
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	threads, err := h.Service.List(threadapp.ListInput{
		ArchivedOnly:    r.URL.Query().Get("archived_only") == "true",
		IncludeArchived: r.URL.Query().Get("include_archived") == "true",
		IncludeSide:     threadapp.ListIncludesSide(r.URL.Query().Get("include")),
		Search:          r.URL.Query().Get("search"),
		Limit:           IntQuery(r.URL.Query(), "limit"),
	})
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"threads": threads})
}

func (h ThreadHandlers) handleCreate(w http.ResponseWriter, r *http.Request) {
	body, ok := RequestMapBody(w, r, "invalid thread create body")
	if !ok {
		return
	}
	thread, err := h.Service.Create(body)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusCreated, thread)
}

func (h ThreadHandlers) HandleRecord(w http.ResponseWriter, r *http.Request, threadID string) {
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_service_missing", "message": "thread service missing"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		thread, err := h.Service.Get(threadID)
		if err != nil {
			if errors.Is(err, threadapp.ErrPublicProjectionPending) {
				WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
					"code":    "public_projection_pending",
					"message": "The thread public projection is finalizing.",
				})
				return
			}
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		if thread == nil {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		// Delivery batches are response-time authority owned by this adapter.
		// Never let a stale field already present on the service projection become
		// a second alias or survive a failed exact hydration.
		thread = contracts.CloneMap(thread)
		delete(thread, "acceptedFinalDelivery")
		delete(thread, "acceptedFinalDeliveries")
		acceptedFinalReferenceCount := threadAcceptedFinalReferenceCount(thread)
		hasAcceptedFinalReference := acceptedFinalReferenceCount != 0
		if h.HydrateAcceptedFinalDelivery == nil && hasAcceptedFinalReference {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
				"code":    "accepted_final_hydration_unavailable",
				"message": "accepted-final snapshot authority is unavailable",
			})
			return
		}
		if h.HydrateAcceptedFinalDelivery != nil {
			delivery, hydrationErr := h.HydrateAcceptedFinalDelivery(r.Context(), threadID, thread)
			if hydrationErr != nil {
				if errors.Is(hydrationErr, threadapp.ErrPublicProjectionPending) {
					WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
						"code":    "public_projection_pending",
						"message": "The thread public projection is finalizing.",
					})
					return
				}
				if os.Getenv("ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK") == "1" {
					w.Header().Set("X-Analytix-QA-Hydration-Class", acceptedFinalHydrationQAClass(hydrationErr))
				}
				WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
					"code":    "accepted_final_hydration_unavailable",
					"message": "accepted-final snapshot authority is unavailable",
				})
				return
			}
			if len(delivery.Deliveries) > domainevent.AcceptedFinalDeliveryGroupLimitV1 ||
				len(delivery.Deliveries) != acceptedFinalReferenceCount ||
				delivery.Latest == nil && acceptedFinalReferenceCount != 0 ||
				delivery.Latest != nil && (acceptedFinalReferenceCount == 0 || len(delivery.Deliveries) == 0 ||
					!reflect.DeepEqual(delivery.Latest, delivery.Deliveries[len(delivery.Deliveries)-1])) {
				WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
					"code":    "accepted_final_hydration_unavailable",
					"message": "accepted-final snapshot authority is unavailable",
				})
				return
			}
			if delivery.Latest != nil {
				thread["acceptedFinalDeliveries"] = delivery.Deliveries
				if len(delivery.Deliveries) == 1 {
					thread["acceptedFinalDelivery"] = delivery.Latest
				}
			}
		}
		WriteJSON(w, http.StatusOK, thread)
	case http.MethodPatch:
		body, ok := RequestBody(w, r)
		if !ok {
			return
		}
		var patch map[string]any
		if err := json.Unmarshal(body, &patch); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid thread patch"})
			return
		}
		if err := threadapp.ValidatePublicPatch(patch); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		thread, err := h.Service.Patch(r.Context(), threadID, patch)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if isThreadMutationConflict(err) {
			WriteJSON(w, http.StatusConflict, threadMutationError(err))
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, thread)
	case http.MethodDelete:
		deleted, err := h.Service.Delete(r.Context(), threadID)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if isThreadMutationConflict(err) {
			WriteJSON(w, http.StatusConflict, threadMutationError(err))
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
	default:
		MethodNotAllowed(w)
	}
}

// Only fixed categories enter a private packaged-QA response header. The
// underlying error can contain host details and is never sent to the desktop.
func acceptedFinalHydrationQAClass(err error) string {
	if err == nil {
		return "none"
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "snapshot and event frontier are torn"):
		return "frontier_torn"
	case strings.Contains(message, "snapshot frontier is invalid"):
		return "frontier_invalid"
	case strings.Contains(message, "durable replay is unavailable"):
		return "durable_replay"
	case strings.Contains(message, "thread readback is unavailable"):
		return "thread_readback"
	case strings.Contains(message, "manifest"):
		return "manifest_mismatch"
	case strings.Contains(message, "public projection was rejected"):
		return "projection_rejected"
	case strings.Contains(message, "public item") || strings.Contains(message, "public turn"):
		return "public_slot_mismatch"
	case strings.Contains(message, "seal was rejected"):
		return "seal_rejected"
	case strings.Contains(message, "batch is invalid"):
		return "batch_invalid"
	case strings.Contains(message, "retained"):
		return "retained_authority"
	case strings.Contains(message, "authority"):
		return "authority_mismatch"
	case strings.Contains(message, "bound"):
		return "delivery_bound"
	default:
		return "other"
	}
}

func threadAcceptedFinalReferenceCount(thread map[string]any) int {
	turns, _ := thread["turns"].([]any)
	count := 0
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		hasReference := false
		if _, present := turn["acceptedFinal"]; present {
			hasReference = true
		}
		if _, present := turn["acceptedFinalView"]; present {
			hasReference = true
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if _, present := item["acceptedFinal"]; present {
				hasReference = true
			}
			if _, present := item["acceptedFinalView"]; present {
				hasReference = true
			}
		}
		if hasReference {
			count++
		}
	}
	return count
}

func (h ThreadHandlers) HandleFork(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if !domainthread.IsCanonicalRecordID(threadID) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_service_missing", "message": "thread service missing"})
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	request := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &request); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid fork body"})
			return
		}
	}
	thread, err := h.Service.Fork(threadID, request)
	if errors.Is(err, os.ErrNotExist) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if errors.Is(err, threadapp.ErrTurnNotFound) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": err.Error()})
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusCreated, thread)
}

func (h ThreadHandlers) HandleRewind(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_service_missing", "message": "thread service missing"})
		return
	}
	body, ok := RequestMapBody(w, r, "invalid rewind body")
	if !ok {
		return
	}
	turnID := stringField(body, "turnId")
	if turnID == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "turnId is required"})
		return
	}
	response, err := h.Service.Rewind(r.Context(), threadID, turnID)
	if errors.Is(err, os.ErrNotExist) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if errors.Is(err, threadapp.ErrTurnNotFound) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "turn not found"})
		return
	}
	if errors.Is(err, threadapp.ErrThreadRunning) {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": err.Error()})
		return
	}
	if errors.Is(err, threadapp.ErrAcceptedFinalRewind) {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "message": err.Error()})
		return
	}
	if errors.Is(err, threadapp.ErrCurrentSecurityContextRewind) || isThreadMutationConflict(err) {
		WriteJSON(w, http.StatusConflict, threadMutationError(err))
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	// The committed mutation also carries Core-owned authority metadata. Only
	// publish RewindThreadResponse fields so a successful durable cut remains
	// consumable by the desktop's exact public-contract validator.
	WriteJSON(w, http.StatusOK, map[string]any{
		"threadId": response["threadId"], "turnId": response["turnId"],
		"removedTurns": response["removedTurns"], "remainingTurns": response["remainingTurns"],
		"removedTurnIds": response["removedTurnIds"],
	})
}

func (h ThreadHandlers) HandleGoal(w http.ResponseWriter, r *http.Request, threadID string) {
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_service_missing", "message": "thread service missing"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		goal, err := h.Service.GetGoal(threadID)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"goal": goal})
	case http.MethodPost:
		body, ok := RequestMapBody(w, r, "invalid thread goal body")
		if !ok {
			return
		}
		goal, err := h.Service.SetGoal(threadID, body)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"goal": goal})
	case http.MethodDelete:
		cleared, err := h.Service.ClearGoal(threadID)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"cleared": cleared})
	default:
		MethodNotAllowed(w)
	}
}

func (h ThreadHandlers) HandleTodos(w http.ResponseWriter, r *http.Request, threadID string) {
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_service_missing", "message": "thread service missing"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		todos, err := h.Service.GetTodos(threadID)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"todos": todos})
	case http.MethodPost:
		items, ok := strictThreadTodoItems(w, r, threadID)
		if !ok {
			return
		}
		todos, err := h.Service.SetTodos(threadID, items)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"todos": todos})
	case http.MethodDelete:
		cleared, err := h.Service.ClearTodos(threadID)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if errors.Is(err, threadapp.ErrTodoTerminalAuditRetention) {
			WriteJSON(w, http.StatusConflict, map[string]any{"code": "todo_terminal_audit_retained", "message": err.Error()})
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"cleared": cleared})
	default:
		MethodNotAllowed(w)
	}
}

func strictThreadTodoItems(w http.ResponseWriter, r *http.Request, threadID string) ([]any, bool) {
	raw, ok := RequestBody(w, r)
	if !ok {
		return nil, false
	}
	if len(raw) == 0 {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid thread todos body"})
		return nil, false
	}
	body, err := domainjsonstrict.DecodeObject(raw, domainjsonstrict.Options{MaxBytes: 1 << 20, MaxDepth: 16, MaxTokens: 5000, MaxStringBytes: 4000})
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid thread todos body"})
		return nil, false
	}
	for key := range body {
		if key != "todos" && key != "turnId" {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "unknown thread todos property"})
			return nil, false
		}
	}
	if rawTurnID, exists := body["turnId"]; exists {
		turnID, ok := rawTurnID.(string)
		if !ok || strings.TrimSpace(turnID) == "" {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "turnId must be a non-empty string"})
			return nil, false
		}
	}
	items, ok := body["todos"].([]any)
	if !ok {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "todos must be an array"})
		return nil, false
	}
	allowed := map[string]bool{
		"id": true, "content": true, "status": true, "statusReasonCode": true, "source": true, "note": true,
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "todo items must be objects"})
			return nil, false
		}
		if _, exists := item["content"]; !exists {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "todo content is required"})
			return nil, false
		}
		if _, exists := item["status"]; !exists {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "todo status is required"})
			return nil, false
		}
		for key := range item {
			if !allowed[key] {
				WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "unknown todo property at item " + strconv.Itoa(index+1)})
				return nil, false
			}
		}
	}
	if _, err := threadapp.NormalizeTodos(threadID, items, "validation"); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return nil, false
	}
	return items, true
}

func (h ThreadHandlers) HandleCompact(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "thread_service_missing", "message": "thread service missing"})
		return
	}
	body, ok := RequestMapBody(w, r, "invalid compact body")
	if !ok {
		return
	}
	reason := stringField(body, "reason")
	if reason == "" {
		reason = "manual"
	}
	response, err := h.Service.Compact(r.Context(), threadID, reason)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, threadapp.ErrThreadNotFound) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if errors.Is(err, threadapp.ErrThreadRunning) {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "thread_running", "message": err.Error()})
		return
	}
	if errors.Is(err, threadapp.ErrCaseCompactionRequiresTrustedArchive) {
		WriteJSON(w, http.StatusConflict, map[string]any{
			"code": "case_compaction_archive_required", "message": err.Error(),
		})
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, response)
}

func listAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return []any{}
}
