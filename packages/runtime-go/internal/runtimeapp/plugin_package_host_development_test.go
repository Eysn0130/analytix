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
	if absent, count := newDevelopmentPackageHost(ctx, Config{DataDir: data}, identity, nil); absent != nil || count != 0 {
		t.Fatal("ambient source capability")
	}
	host, count := newDevelopmentPackageHost(ctx, config, identity, nil)
	if host == nil || count != 0 {
		t.Fatal("actual source host was not composed")
	}
	views, err := host.List(ctx)
	if err != nil || len(views) != 4 {
		t.Fatalf("actual four source packages: %d %v", len(views), err)
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
	if len(skills) != 4 {
		t.Fatal("actual four enabled productivity skills were not admitted")
	}
	for _, skill := range skills {
		if skill.Binding.PackageID != "analytix-documents" && skill.Binding.PackageID != "analytix-spreadsheets" && skill.Binding.PackageID != "analytix-presentations" && skill.Binding.PackageID != "analytix-canvas" {
			t.Fatal("unexpected skill contribution")
		}
		instructions, ok := skill.Snapshot.File("SKILL.md")
		tool := "generate_office_document"
		if skill.Binding.PackageID == "analytix-canvas" {
			tool = "generate_canvas_object"
		}
		if !ok || !strings.Contains(string(instructions), tool) {
			t.Fatal("installed skill has no generation workflow")
		}
	}
	reopened, count := newDevelopmentPackageHost(ctx, config, identity, nil)
	if reopened == nil || count != 0 {
		t.Fatal("source host did not reopen")
	}
	restored, err := reopened.List(ctx)
	if err != nil || len(restored) != 4 {
		t.Fatal("source inventory disappeared")
	}
	for index, view := range restored {
		if view.GenerationID != views[index].GenerationID || view.ActivationRevision != 1 || view.DesiredState != domainplugin.DesiredEnabledV1 {
			t.Fatal("restart replaced generation or activation")
		}
	}
	restoredSkills := reopened.Skills(ctx)
	if len(restoredSkills) != 4 {
		t.Fatal("restart lost productivity skills")
	}
	for i, skill := range restoredSkills {
		if skill.Binding != skills[i].Binding || skill.Snapshot.Digest() != skills[i].Snapshot.Digest() {
			t.Fatal("restart changed the admitted skill snapshot")
		}
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

func TestDevelopmentPackageHostReportsIncompleteStartupDiscovery(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	principal, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	identity := editingTestIdentity{principal}
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "all-failed", true: "partial"}[partial], func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			data := filepath.Join(home, "data")
			if err := os.Mkdir(data, 0700); err != nil {
				t.Fatal(err)
			}
			descriptors := map[string]staticEditorPackageDescriptor{"analytix-canvas": {root: t.TempDir()}}
			if partial {
				descriptors["analytix-documents"] = staticEditorPackageDescriptor{root: root}
			}
			host, count := composeStaticEditorPackageHost(ctx, Config{DataDir: data}, identity, nil, descriptors)
			if count != 1 {
				t.Fatalf("lost failed registration: %d", count)
			}
			if !partial {
				if host != nil {
					t.Fatal("failed sources created host")
				}
				return
			}
			views, err := host.List(ctx)
			if err != nil || len(views) != 1 || views[0].PackageID != "analytix-documents" {
				t.Fatal(views, err)
			}
			if _, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: views[0].PackageID, GenerationID: views[0].GenerationID, DesiredState: domainplugin.DesiredEnabledV1}); err != nil {
				t.Fatal(err)
			}
			if discovery := host.DiscoverSkills(ctx); len(discovery.Skills) != 1 || discovery.ValidationErrorCount != 0 {
				t.Fatal("admitted source lost")
			}
		})
	}
}
