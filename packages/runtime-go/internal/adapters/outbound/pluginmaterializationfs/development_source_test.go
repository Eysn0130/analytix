package pluginmaterializationfs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

func writeDevelopmentSourceV1(t *testing.T, id string) string {
	t.Helper()
	root := realTempDir(t)
	declaration := domainpackage.DeclarationV1{SchemaVersion: 1, PackageID: id, PackageVersion: "1.0.0", Contributions: domainpackage.ContributionsV1{
		Skills: []domainpackage.PathContributionV1{}, MCPServers: []domainpackage.MCPServerContributionV1{}, Hooks: []domainpackage.PathContributionV1{},
		PublicUI: []domainpackage.PathContributionV1{{ID: "workspace-editor", Path: "ui/editor.json"}}, Assets: []domainpackage.PathContributionV1{{ID: "editor-adapter", Path: "assets/adapter.json"}},
	}, RequestedCapabilities: []domainpackage.CapabilityRequestV1{{ID: "office.local-edit", ProtocolVersion: 1, ScopeConstraints: []string{"user-selected-object", "explicit-save"}}}, Lifecycle: domainpackage.LifecycleV1{ProtocolVersion: 1, EntryPolicy: "host-static-first-party"}}
	body, err := domainpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]string{"name": id, "version": "1.0.0"})
	for path, body := range map[string][]byte{domainpackage.DeclarationRelativePathV1: body, domainplugin.ManifestRelativePathV1: manifest, "ui/editor.json": []byte(`{"kind":"synthetic-static-editor"}`), "assets/adapter.json": []byte(`{"kind":"synthetic-static-adapter"}`)} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func developmentBindingV1(t *testing.T, root string) pluginapp.DevelopmentSourceBindingV1 {
	t.Helper()
	identity, err := InspectDevelopmentSourceTreeV1(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(identity.SourceRegistrationJSON))
	if err != nil {
		t.Fatal(err)
	}
	if identity.SourceRegistrationSHA256 != domainpackage.DevelopmentSourceRegistrationSHA256V1(registration) {
		t.Fatal("registration identity drift")
	}
	binding, err := pluginapp.NewDevelopmentSourceBindingV1(registration, root, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func TestDevelopmentSourceThreePackagesMaterializeAndRestoreSignedActivationV1(t *testing.T) {
	ctx := context.Background()
	home := realTempDir(t)
	authority := newTestAuthority()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	results := map[string]pluginport.ResultV1{}
	for _, id := range []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations"} {
		source := writeDevelopmentSourceV1(t, id)
		if _, err := InspectSourceTreeV1(ctx, source); err == nil {
			t.Fatal("formal Funds source inspector accepted static editor")
		}
		binding := developmentBindingV1(t, source)
		intent, err := binding.NewIntentV1(now)
		if err != nil {
			t.Fatal(err)
		}
		store, err := NewPackageStoreV1(home, id, nil)
		if err != nil {
			t.Fatal(err)
		}
		service, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, binding, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Materialize(ctx, intent)
		if err != nil {
			t.Fatal(id, err)
		}
		results[id] = result
		if result.Receipt.Origin != domainplugin.DevelopmentSourceOriginV1 || result.Receipt.PackageAuthoritySHA256 != "" || result.Receipt.SourceRegistrationSHA256 != intent.SourceRegistrationSHA256 {
			t.Fatal("receipt lost explicit source registration")
		}
		if _, err := InspectInstallMarkerV1(store.absolute(result.Receipt.ActiveRelativePath), result.Receipt); err != nil {
			t.Fatal(err)
		}
		state, err := store.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: result.Receipt.GenerationID, DesiredState: domainplugin.DesiredDisabledV1}, authority, now)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenExistingPackageStoreV1(home, id)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := reopened.ReadActivation(ctx, authority)
		if err != nil || restored != state {
			t.Fatal("source disabled state did not survive reopening", err)
		}
		// An altered source cannot acquire a receipt under the frozen registration.
		if id == "analytix-documents" {
			ui := filepath.Join(source, "ui/editor.json")
			if err := os.WriteFile(ui, []byte(`{"kind":"synthetic-static-editor","revision":2}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Materialize(ctx, intent); err == nil {
				t.Fatal("changed source materialized under old registration")
			}
			nextBinding := developmentBindingV1(t, source)
			nextIntent, err := nextBinding.NewIntentV1(now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			nextService, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, nextBinding, func() time.Time { return now.Add(time.Second) })
			if err != nil {
				t.Fatal(err)
			}
			next, err := nextService.Materialize(ctx, nextIntent)
			if err != nil {
				t.Fatal("new source registration did not rotate generation", err)
			}
			results[id] = next
			if next.Receipt.GenerationID == result.Receipt.GenerationID {
				t.Fatal("changed registration reused generation")
			}
			if _, err := service.ResolveActive(ctx); err == nil {
				t.Fatal("old binding resolved new source generation")
			}
			if _, err := reopened.ReadActivation(ctx, authority); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("new generation inherited activation", err)
			}
			if _, err := reopened.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: result.Receipt.GenerationID, ExpectedRevision: state.Revision, DesiredState: domainplugin.DesiredEnabledV1}, authority, now); !errors.Is(err, pluginport.ErrConflict) {
				t.Fatal("old generation reactivated", err)
			}
		}
	}
	for id, want := range results {
		store, err := OpenExistingPackageStoreV1(home, id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.ResolveActive(ctx, authority)
		if err != nil || got != want {
			t.Fatal("another package replaced the active generation", id, err)
		}
	}
	// A formal envelope with a coincidentally equal digest cannot read source state.
	result := results["analytix-presentations"]
	store, err := OpenExistingPackageStoreV1(home, result.Receipt.PluginName)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := pluginapp.NewService(store, authority, pluginapp.FormalPackageBindingV1{AuthoritySHA256: result.Receipt.SourceRegistrationSHA256, Target: result.Receipt.Target, SourceTreeSHA256: result.Receipt.SourceTreeSHA256, SourceTreeFileCount: result.Receipt.SourceTreeFileCount, ManifestSHA256: result.Receipt.ManifestSHA256, EntrypointSHA256: strings.Repeat("a", 64)}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := formal.ResolveActive(ctx); !errors.Is(err, pluginapp.ErrPackageAuthority) {
		t.Fatal("formal service accepted source receipt", err)
	}
	active := store.absolute(result.Receipt.ActiveRelativePath)
	adapter := filepath.Join(active, "assets/adapter.json")
	if err := os.WriteFile(adapter, []byte(`{"kind":"changed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveActive(ctx, authority); !errors.Is(err, pluginport.ErrCorrupt) {
		t.Fatal("changed active contribution accepted", err)
	}
	if _, err := store.ReadActivation(ctx, authority); !errors.Is(err, pluginport.ErrCorrupt) {
		t.Fatal("activation survived changed contribution", err)
	}
}

func TestDevelopmentSourceInspectorRejectsMissingContributionAndSymlinkV1(t *testing.T) {
	root := writeDevelopmentSourceV1(t, "analytix-documents")
	path := filepath.Join(root, "ui/editor.json")
	if err := os.Rename(path, path+".original"); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDevelopmentSourceTreeV1(context.Background(), root); err == nil {
		t.Fatal("missing declared contribution accepted")
	}
	if err := os.Symlink(path+".original", path); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDevelopmentSourceTreeV1(context.Background(), root); err == nil {
		t.Fatal("symlinked contribution accepted")
	}
}
