package mediaexecution

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/mediaexecutiontransport"
)

func TestMediaPrivacyProjectsActualGenerationBytesAndClosesUninspectableEffects(t *testing.T) {
	var calls atomic.Int32
	var body string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"iVBORwAAAAAAAAAA"}]}`))
	}))
	defer upstream.Close()
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-model"), credential: "synthetic-media-secret"}
	executor := New(registry, mediaexecutiontransport.New)
	const canary = "13800138000"
	result, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate,
		Prompt: "Draw a chart for phone " + canary})
	if err != nil || len(result.Image) == 0 || calls.Load() != 1 {
		t.Fatalf("ordinary projected generation failed: err=%v calls=%d", err, calls.Load())
	}
	for _, forbidden := range []string{canary, "/Users/private-owner/case.csv"} {
		if strings.Contains(body, forbidden) {
			t.Fatal("serialized request retained private canary")
		}
	}
	binary := base64.StdEncoding.EncodeToString([]byte("raw-media-" + canary))
	for _, request := range []Request{
		{Operation: OperationImageGenerate, Prompt: "Draw a chart at /Users/private-owner/case.csv"},
		{Operation: OperationImageGenerate, Prompt: "Draw ~/private-case.csv"},
		{Operation: OperationImageGenerate, Prompt: "路径：/Users/private-owner/case.csv"},
		{Operation: OperationImageGenerate, Prompt: "请分析/Users/private-owner/case.csv"},
		{Operation: OperationImageGenerate, Prompt: "see/Users/private-owner/case.csv"},
		{Operation: OperationImageGenerate, Prompt: `请分析C:\private-owner\case.csv`},
		{Operation: OperationImageEdit, Prompt: "edit", Images: []ReferenceImage{{Name: "reference.png", MIMEType: "image/png", DataBase64: binary}}},
		{Operation: OperationSpeechTranscribe, MIMEType: "audio/wav", AudioBase64: binary},
		{Operation: OperationImageGenerate, Prompt: "data:image/png;base64," + binary},
		{Operation: OperationImageGenerate, Prompt: "ordinary chart", Size: canary},
	} {
		before := calls.Load()
		result, err := executor.Execute(context.Background(), request)
		if !errors.Is(err, ErrPrivacyUnavailable) || len(result.Image) != 0 || result.Transcript != "" || calls.Load() != before {
			t.Fatalf("uninspectable effect reached provider or returned success: operation=%s calls=%d", request.Operation, calls.Load()-before)
		}
	}
	registry.provider.SelectedMedia = canary
	before := calls.Load()
	if _, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "ordinary chart"}); err == nil || calls.Load() != before {
		t.Fatal("private model metadata reached provider")
	}
}

func TestMediaDownloadRejectsPrivateURLWithoutMakingDownloadRequest(t *testing.T) {
	var downloads atomic.Int32
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	}))
	defer download.Close()
	var destination string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"url":"` + destination + `"}]}`))
	}))
	defer upstream.Close()
	executor := New(&mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-model"), credential: "synthetic"}, mediaexecutiontransport.New)
	for _, suffix := range []string{"/13800138000.png", "/Users/private-owner/case.png", "/asset/%2FUsers%2Fprivate-owner%2Fcase.png", "/asset/Users/private-owner/case.png", "/asset/%252FUsers%252Fprivate-owner%252Fcase.png", "/image?" + url.QueryEscape("账号") + "=00123456", "/image?phone=%25313800138000"} {
		destination = download.URL + suffix
		if _, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "ordinary chart"}); err == nil || downloads.Load() != 0 {
			t.Fatal("private download URL was sent or accepted")
		}
	}
}
