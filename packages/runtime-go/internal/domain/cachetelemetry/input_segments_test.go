package cachetelemetry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestFinalWireInputSegmentsPreserveOrderAndExcludeMetadata(t *testing.T) {
	digest := func(label string, body []byte) string {
		h := hmac.New(sha256.New, []byte("synthetic-installation-authority"))
		h.Write([]byte(label))
		h.Write(body)
		return hex.EncodeToString(h.Sum(nil))
	}
	body := `{"messages":[{"role":"system","content":"rules"},{"role":"user","content":"first"},{"role":"assistant","content":"second"},{"role":"user","content":"current"}],"tools":[],"metadata":{"user_id":"one"}}`
	base := CaptureModelInputSegmentsV1([]byte(body), digest)
	if base.Validate() != nil || base.System.Items != 1 || base.History.Items != 2 || base.Current.Items != 1 {
		t.Fatal("missing final wire segments")
	}
	metadata := CaptureModelInputSegmentsV1([]byte(strings.Replace(body, `"one"`, `"two"`, 1)), digest)
	if base != metadata {
		t.Fatal("off-model metadata invalidated model input")
	}
	reordered := CaptureModelInputSegmentsV1([]byte(strings.ReplaceAll(strings.ReplaceAll(body, "first", "tmp"), "second", "first")), digest)
	if base.History == reordered.History || base.System != reordered.System || base.Tools != reordered.Tools || base.Current != reordered.Current {
		t.Fatal("ordered history identity was lost")
	}
	encoded, err := json.Marshal(base)
	if err != nil || strings.Contains(string(encoded), "rules") || strings.Contains(string(encoded), "first") {
		t.Fatal("segments exposed input")
	}
	if got := CaptureModelInputSegmentsV1([]byte(`{"messages":"unsupported"}`), digest); got != (ModelInputSegmentsV1{}) {
		t.Fatal("unsupported input must remain unavailable")
	}
}

func TestOldCacheShapeCanonicalJSONOmitsNewSegments(t *testing.T) {
	// In particular, omitzero must preserve old sealed intent/signature bytes.
	body, err := json.Marshal(CacheVisibleShapeV1{})
	if err != nil || strings.Contains(string(body), "modelInput") {
		t.Fatal("old shape canonical bytes changed")
	}
}

func TestOldCacheUsageCanonicalJSONOmitsOptionalMessagesAndCost(t *testing.T) {
	body, err := json.Marshal(ProviderUsageV1{})
	if err != nil || strings.Contains(string(body), "messagesInput") || strings.Contains(string(body), "estimatedCost") {
		t.Fatal("old sealed usage gained optional fields")
	}
	var reopened ProviderUsageV1
	if json.Unmarshal(body, &reopened) != nil {
		t.Fatal("old usage readback failed")
	}
	again, err := json.Marshal(reopened)
	if err != nil || string(again) != string(body) {
		t.Fatal("old canonical usage changed on reopen")
	}
}
