package runtimeapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
)

func TestDevelopmentPackageHostActualSourceActivationRestartAndProtectedRoute(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	principal, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	identity := editingTestIdentity{principal}
	config := Config{DataDir: data, DevelopmentPluginSourceRoot: root}
	if newDevelopmentPackageHost(ctx, Config{DataDir: data}, identity, nil) != nil {
		t.Fatal("ambient source capability")
	}
	host := newDevelopmentPackageHost(ctx, config, identity, nil)
	if host == nil {
		t.Fatal("actual source host was not composed")
	}
	views, err := host.List(ctx)
	if err != nil || len(views) != 3 {
		t.Fatalf("actual three source packages: %d %v", len(views), err)
	}
	for _, view := range views {
		if !view.Materialized || view.Publishable || view.Available || view.ActivationState != "unset" {
			t.Fatal("fabricated source readiness")
		}
		updated, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, DesiredState: domainplugin.DesiredEnabledV1})
		if err != nil || updated.ActivationRevision != 1 || updated.Available || updated.UnavailableReason != "adapter_unavailable" {
			t.Fatal("activation was not committed or fabricated an engine", err)
		}
	}
	skills := host.Skills(ctx)
	if len(skills) != 1 || skills[0].Binding.PackageID != "analytix-documents" {
		t.Fatal("actual enabled Documents skill was not admitted")
	}
	instructions, ok := skills[0].Snapshot.File("SKILL.md")
	if !ok || !strings.Contains(string(instructions), "generate_office_document") {
		t.Fatal("installed skill did not contain its real generation workflow")
	}
	reopened := newDevelopmentPackageHost(ctx, config, identity, nil)
	if reopened == nil {
		t.Fatal("source host did not reopen")
	}
	restored, err := reopened.List(ctx)
	if err != nil || len(restored) != 3 {
		t.Fatal("source inventory disappeared")
	}
	for index, view := range restored {
		if view.GenerationID != views[index].GenerationID || view.ActivationRevision != 1 || view.DesiredState != domainplugin.DesiredEnabledV1 {
			t.Fatal("restart replaced generation or activation")
		}
	}
	restoredSkills := reopened.Skills(ctx)
	if len(restoredSkills) != 1 || restoredSkills[0].Binding != skills[0].Binding || restoredSkills[0].Snapshot.Digest() != skills[0].Snapshot.Digest() {
		t.Fatal("restart changed the admitted skill snapshot")
	}
	mux := httpapi.LocalDisplayMuxV1{RuntimeToken: "synthetic-token", LocalDisplay: httpapi.LocalDisplayHandlerV1{PackageHost: httpapi.PluginPackageHostHandler{Service: reopened}}}
	for _, tc := range []struct {
		token, typed bool
		code         int
	}{{false, false, 401}, {true, false, 403}, {true, true, 200}} {
		r := httptest.NewRequest(http.MethodPost, httpapi.PluginPackageHostPath, strings.NewReader(`{"action":"list"}`))
		if tc.token {
			r.Header.Set("Authorization", "Bearer synthetic-token")
		}
		if tc.typed {
			r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.code || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("exact protected path status %d", w.Code)
		}
		if strings.Contains(w.Body.String(), root) || strings.Contains(w.Body.String(), home) {
			t.Fatal("private source path projected")
		}
	}
}
