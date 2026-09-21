package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
)

func TestImageRequestClosedTypedGeometryBeforeAuthority(t *testing.T) {
	handler := ObjectEditingHandler{Service: editingapp.New(nil, nil)}
	baseline := map[string]any{"action": "image-annotation-write", "sessionId": strings.Repeat("a", 48), "threadId": "primary", "sourceRevision": strings.Repeat("b", 64), "expectedAnnotationRevision": "", "regions": []any{map[string]any{"regionId": strings.Repeat("c", 48), "region": map[string]any{"x": 0, "y": 0, "width": 1, "height": 1}, "note": "remark"}}}
	for _, name := range []string{"fraction", "negative", "empty", "extra geometry", "missing geometry", "string geometry", "null coordinate", "extra authority", "bad revision", "large note", "duplicate id", "null collection", "extra item", "null region", "null note", "legacy payload"} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(baseline)
			var value map[string]any
			_ = json.Unmarshal(raw, &value)
			item := value["regions"].([]any)[0].(map[string]any)
			region := item["region"].(map[string]any)
			switch name {
			case "fraction":
				region["x"] = 0.5
			case "negative":
				region["x"] = -1
			case "empty":
				region["width"] = 0
			case "extra geometry":
				region["units"] = "percent"
			case "missing geometry":
				delete(region, "y")
			case "string geometry":
				item["region"] = "0,0,1,1"
			case "null coordinate":
				region["x"] = nil
			case "extra authority":
				value["workspace"] = "/tmp"
			case "bad revision":
				value["sourceRevision"] = "caller"
			case "duplicate id":
				value["regions"] = []any{item, item}
			case "null collection":
				value["regions"] = nil
			case "extra item":
				item["extra"] = true
			case "null region":
				item["region"] = nil
			case "null note":
				item["note"] = nil
			case "legacy payload":
				delete(value, "regions")
				value["region"] = region
				value["note"] = "remark"
			case "large note":
				item["note"] = strings.Repeat("x", 4097)
			}
			raw, _ = json.Marshal(value)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ObjectEditingPath, strings.NewReader(string(raw))))
			if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_request") {
				t.Fatalf("malformed geometry reached authority: %d %s", w.Code, w.Body.String())
			}
		})
	}
	for _, action := range []string{"image-open", "image-annotation-read", "image-scope-capture", "image-scope-read", "image-scope-revoke"} {
		t.Run(action, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ObjectEditingPath, strings.NewReader(`{"action":"`+action+`","threadId":"primary"}`)))
			if w.Code != 400 {
				t.Fatal("missing exact fields")
			}
		})
	}
}
