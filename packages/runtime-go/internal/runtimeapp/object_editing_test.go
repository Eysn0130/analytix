package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
)

type editingTestIdentity struct{ p identitydomain.PrincipalV1 }

func (i editingTestIdentity) ResolveCurrent(context.Context) (identitydomain.PrincipalV1, error) {
	return i.p, nil
}
func (i editingTestIdentity) ValidateCurrent(_ context.Context, p identitydomain.PrincipalV1) error {
	if i.p != p {
		return errors.New("mismatch")
	}
	return nil
}
func TestObjectEditingAssemblyUsesPrivateManagedRoot(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported object persistence platform")
	}
	data := t.TempDir()
	p, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	h := newObjectEditingHandler(Config{DataDir: data}, editingTestIdentity{p}, []string{data})
	if h == nil {
		t.Fatal("handler not assembled")
	}
	info, err := os.Stat(filepath.Join(data, "object-editing"))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("receipt root is not private")
	}
	if newObjectEditingHandler(Config{DataDir: data}, editingTestIdentity{p}, []string{data}) == nil {
		t.Fatal("existing receipt root cannot reopen")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/local-display/object-editing", strings.NewReader(`{"action":"open","workspace":"`+data+`","path":"object-editing/anything.json"}`)))
	if w.Code != http.StatusForbidden {
		t.Fatal("managed receipt storage was editable")
	}
}
