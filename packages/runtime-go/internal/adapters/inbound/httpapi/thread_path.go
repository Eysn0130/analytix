package httpapi

import (
	"net/http"
	"strings"
)

type ThreadPathHandler func(http.ResponseWriter, *http.Request, string)

type ThreadPathHandlers struct {
	Summary     ThreadPathHandler
	Events      ThreadPathHandler
	Fork        ThreadPathHandler
	Goal        ThreadPathHandler
	Todos       ThreadPathHandler
	Compact     ThreadPathHandler
	Review      ThreadPathHandler
	Checkpoints ThreadPathHandler
	TurnAction  ThreadPathHandler
	Turns       ThreadPathHandler
	Rewind      ThreadPathHandler
	Record      ThreadPathHandler
}

func (h ThreadPathHandlers) Handle(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/threads/")
	switch {
	case strings.HasSuffix(rest, "/summary") || strings.Contains(rest, "/summary/tasks/"):
		h.dispatch(w, r, rest, h.Summary)
	case strings.HasSuffix(rest, "/events"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/events"), h.Events)
	case strings.HasSuffix(rest, "/fork"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/fork"), h.Fork)
	case strings.HasSuffix(rest, "/goal"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/goal"), h.Goal)
	case strings.HasSuffix(rest, "/todos"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/todos"), h.Todos)
	case strings.HasSuffix(rest, "/compact"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/compact"), h.Compact)
	case strings.HasSuffix(rest, "/review"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/review"), h.Review)
	case strings.Contains(rest, "/checkpoints/"):
		h.dispatch(w, r, rest, h.Checkpoints)
	case strings.Contains(rest, "/turns/"):
		h.dispatch(w, r, rest, h.TurnAction)
	case strings.HasSuffix(rest, "/turns"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/turns"), h.Turns)
	case strings.HasSuffix(rest, "/rewind"):
		h.dispatch(w, r, strings.TrimSuffix(rest, "/rewind"), h.Rewind)
	case !strings.Contains(rest, "/"):
		h.dispatch(w, r, rest, h.Record)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h ThreadPathHandlers) dispatch(w http.ResponseWriter, r *http.Request, value string, handler ThreadPathHandler) {
	if handler == nil {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	handler(w, r, value)
}
