package httpapi

import (
	"net/http"
	"strings"

	usageapp "analytix.local/runtime-go/internal/app/usage"
)

type UsageHandlers struct {
	Records         func(threadID string) ([]usageapp.Record, error)
	RuntimeResponse func(records []usageapp.Record) (map[string]any, error)
}

func (h UsageHandlers) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Records == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "usage_store_missing", "message": "usage store missing"})
		return
	}
	groupBy := strings.TrimSpace(r.URL.Query().Get("group_by"))
	if groupBy == "" {
		groupBy = "runtime"
	}
	threadID := strings.TrimSpace(r.URL.Query().Get("thread_id"))
	records, err := h.Records(threadID)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	switch groupBy {
	case "runtime":
		if h.RuntimeResponse == nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "usage_store_missing", "message": "runtime usage response missing"})
			return
		}
		response, err := h.RuntimeResponse(records)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, response)
	case "thread":
		WriteJSON(w, http.StatusOK, usageapp.ThreadResponse(records))
	case "day":
		window, ok := parseUsageWindow(r, "daily usage", w)
		if !ok {
			return
		}
		WriteJSON(w, http.StatusOK, usageapp.DailyResponse(records, window))
	case "model":
		window, ok := parseUsageWindow(r, "model usage", w)
		if !ok {
			return
		}
		WriteJSON(w, http.StatusOK, usageapp.ModelResponse(records, window))
	default:
		WriteJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "validation_error",
			"message": "unsupported usage grouping: " + groupBy,
		})
	}
}

func parseUsageWindow(r *http.Request, label string, w http.ResponseWriter) (usageapp.Window, bool) {
	window, err := usageapp.ParseWindow(usageapp.WindowParams{
		From:     r.URL.Query().Get("from"),
		To:       r.URL.Query().Get("to"),
		Window:   r.URL.Query().Get("window"),
		Timezone: r.URL.Query().Get("timezone"),
		Label:    label,
	})
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return usageapp.Window{}, false
	}
	return window, true
}
