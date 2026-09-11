package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func RequestBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	if r.Body == nil {
		return nil, true
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid request body"})
		return nil, false
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, true
	}
	return json.RawMessage(data), true
}

func RequestMapBody(w http.ResponseWriter, r *http.Request, message string) (map[string]any, bool) {
	body, ok := RequestBody(w, r)
	if !ok {
		return nil, false
	}
	if len(body) == 0 {
		return map[string]any{}, true
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": message})
		return nil, false
	}
	if request == nil {
		request = map[string]any{}
	}
	return request, true
}

func RouteKey(method string, path string, body json.RawMessage) string {
	return strings.ToUpper(method) + " " + path + " " + CanonicalJSON(body)
}

func CanonicalJSON(body json.RawMessage) string {
	if len(body) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return strings.TrimSpace(string(body))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(body))
	}
	return string(data)
}

func WriteRawJSON(w http.ResponseWriter, status int, body json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if len(body) == 0 {
		_, _ = w.Write([]byte("{}\n"))
		return
	}
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}

func WriteSSE(w http.ResponseWriter, status int, frames []string) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(status)
	for _, frame := range frames {
		_, _ = w.Write([]byte(frame))
		_, _ = w.Write([]byte("\n\n"))
	}
}

func WriteDurableSSE(w http.ResponseWriter, status int, events []map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(status)
	for _, event := range events {
		WriteDurableSSEEvent(w, event)
	}
}

func WriteDurableSSEEvent(w http.ResponseWriter, event map[string]any) {
	seq, _ := DurableNumericSeq(event["seq"])
	kind, _ := event["kind"].(string)
	data, _ := json.Marshal(event)
	_, _ = w.Write([]byte("id: " + strconv.Itoa(seq) + "\n"))
	_, _ = w.Write([]byte("event: " + kind + "\n"))
	_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
}

func DurableNumericSeq(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), typed == float64(int(typed))
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
