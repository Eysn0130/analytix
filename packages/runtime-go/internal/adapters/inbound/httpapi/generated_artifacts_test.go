package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
)

func TestGeneratedArtifactRequiresTypedLocalTransport(t *testing.T) {
	for _, test := range []struct {
		name, method, token, header, body string
		want                              int
		resolves                          bool
	}{
		{"valid", "POST", "synthetic", LocalDisplayHeaderValueV1, `{"threadId":"thread-1","artifactId":"` + strings.Repeat("a", 64) + `"}`, 200, true},
		{"missing header", "POST", "synthetic", "", `{}`, 403, false},
		{"missing runtime auth", "POST", "", LocalDisplayHeaderValueV1, `{}`, 401, false},
		{"get", "GET", "synthetic", LocalDisplayHeaderValueV1, `{}`, 405, false},
		{"untrusted path", "POST", "synthetic", LocalDisplayHeaderValueV1, `{"threadId":"thread-1","artifactId":"a","path":"/private/file"}`, 400, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			handler := LocalDisplayMuxV1{RuntimeToken: "synthetic", LocalDisplay: LocalDisplayHandlerV1{GeneratedArtifacts: GeneratedArtifactHandler{Resolve: func(_ context.Context, thread, id string) (generationapp.Resolved, error) {
				calls++
				return generationapp.Resolved{ThreadID: thread, ArtifactID: id}, nil
			}}}}
			request := httptest.NewRequest(test.method, GeneratedArtifactPath, strings.NewReader(test.body))
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			if test.header != "" {
				request.Header.Set(LocalDisplayHeaderV1, test.header)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want || (calls > 0) != test.resolves || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected private route behavior: status=%d calls=%d", response.Code, calls)
			}
		})
	}
}

func TestGeneratedArtifactDoesNotDiscloseResolverErrors(t *testing.T) {
	handler := GeneratedArtifactHandler{Resolve: func(context.Context, string, string) (generationapp.Resolved, error) {
		return generationapp.Resolved{}, errors.New("private-path-and-identity")
	}}
	request := httptest.NewRequest(http.MethodPost, GeneratedArtifactPath, strings.NewReader(`{"threadId":"thread-1","artifactId":"`+strings.Repeat("a", 64)+`"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "private-path") {
		t.Fatal("resolver error crossed the private response boundary")
	}
}
