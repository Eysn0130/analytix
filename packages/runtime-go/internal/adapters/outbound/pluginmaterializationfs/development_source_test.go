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
	}, RequestedCapabilities: []domainpackage.CapabilityRequestV1{{ID: "office.local-preview", ProtocolVersion: 1, ScopeConstraints: []string{"user-selected-object", "read-only"}}}, Lifecycle: domainpackage.LifecycleV1{ProtocolVersion: 1, EntryPolicy: "host-static-first-party"}}
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

// An optional cross-version fixture is created only in an explicitly supplied
// synthetic directory by the prior implementation, then upgraded by this one.
func TestDevelopmentSourceCrossVersionPreviewUpgradeV1(t *testing.T) {
	root := os.Getenv("ANALYTIX_TEST_OFFICE_MIGRATION_ROOT")
	if root == "" {
		t.Skip("cross-version synthetic fixture not supplied")
	}
	ctx := context.Background()
	id := "analytix-documents"
	authority := newTestAuthority()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	home := filepath.Join(root, "runtime")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := NewPackageStoreV1(home, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ANALYTIX_TEST_OFFICE_MIGRATION_PHASE") == "seed" {
		source := writeDevelopmentSourceV1(t, id)
		setDevelopmentFixtureCapabilityV1(t, source, "office.local-edit", "explicit-save")
		persistentSource := filepath.Join(root, "legacy-source")
		if err := copySourceTree(ctx, source, persistentSource); err != nil {
			t.Fatal(err)
		}
		binding := developmentBindingV1(t, persistentSource)
		intent, err := binding.NewIntentV1(now)
		if err != nil {
			t.Fatal(err)
		}
		service, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, binding, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		old, err := service.Materialize(ctx, intent)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: old.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1}, authority, now); err != nil {
			t.Fatal(err)
		}
		t.Log("prior implementation materialized and enabled signed legacy source")
		return
	}
	assertDevelopmentPreviewUpgradeV1(t, store, authority, now)
}

func setDevelopmentFixtureCapabilityV1(t *testing.T, source, capability, scope string) {
	t.Helper()
	path := filepath.Join(source, domainpackage.DeclarationRelativePathV1)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := domainpackage.ParseDeclarationV1(body)
	if err != nil {
		t.Fatal(err)
	}
	declaration.PackageVersion = "0.1.0"
	declaration.RequestedCapabilities[0].ID = capability
	declaration.RequestedCapabilities[0].ScopeConstraints = []string{"user-selected-object", scope}
	body, err = domainpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]string{"name": declaration.PackageID, "version": declaration.PackageVersion})
	if err := os.WriteFile(filepath.Join(source, domainplugin.ManifestRelativePathV1), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertDevelopmentPreviewUpgradeV1(t *testing.T, store *Store, authority testInstallationAuthority, now time.Time) {
	t.Helper()
	ctx := context.Background()
	old, err := store.ResolveActive(ctx, authority)
	if err != nil {
		t.Fatalf("signed historical source could not be resolved: %v", err)
	}
	state, err := store.ReadActivation(ctx, authority)
	if err != nil || state.DesiredState != domainplugin.DesiredEnabledV1 {
		t.Fatalf("legacy enabled receipt not authentic: %v", err)
	}
	if domainplugin.ValidateTrustedActivationForReceiptV1(state, old.Receipt, authority.KeyID(), authority.PublicKey()) != nil {
		t.Fatal("old activation signature invalid")
	}
	source := writeDevelopmentSourceV1(t, old.Receipt.PluginName)
	setDevelopmentFixtureCapabilityV1(t, source, "office.local-preview", "read-only")
	binding := developmentBindingV1(t, source)
	intent, err := binding.NewIntentV1(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	service, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, binding, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	next, err := service.Materialize(ctx, intent)
	if err != nil {
		t.Fatalf("legacy-to-preview materialization blocked: %v", err)
	}
	if next.Receipt.GenerationID == old.Receipt.GenerationID || next.Receipt.SourceRegistrationSHA256 == old.Receipt.SourceRegistrationSHA256 || next.Receipt.PluginVersion != old.Receipt.PluginVersion {
		t.Fatal("same-version upgrade failed to rotate registration/generation")
	}
	if _, err := service.ResolveActive(ctx); err != nil {
		t.Fatal("new preview unavailable", err)
	}
	if _, err := store.ReadActivation(ctx, authority); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new preview inherited enabled state", err)
	}
	if _, err := store.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: old.Receipt.GenerationID, ExpectedRevision: state.Revision, DesiredState: domainplugin.DesiredEnabledV1}, authority, now); !errors.Is(err, pluginport.ErrConflict) {
		t.Fatal("stale generation enabled", err)
	}
	previewState, err := store.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: next.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1}, authority, now.Add(2*time.Second))
	if err != nil {
		t.Fatal("explicit preview enable failed", err)
	}
	reopened, err := OpenExistingPackageStoreV1(store.runtimeHome, old.Receipt.PluginName)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.ReadActivation(ctx, authority)
	if err != nil || got != previewState {
		t.Fatal("preview activation not durable", err)
	}
}

// Seed a portable signed historical installation with the existing receipt,
// marker, index and activation owners. The cross-version test above separately
// proves prior Store.Materialize produced an equivalent upgradeable state.
func historicalDevelopmentStoreFixtureV1(t *testing.T) (*Store, testInstallationAuthority, time.Time) {
	t.Helper()
	ctx := context.Background()
	id := "analytix-documents"
	source := writeDevelopmentSourceV1(t, id)
	setDevelopmentFixtureCapabilityV1(t, source, "office.local-edit", "explicit-save")
	identity, err := inspectInstalledForOriginV1(ctx, source, domainplugin.DevelopmentSourceOriginV1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDevelopmentSourceTreeV1(ctx, source); err == nil {
		t.Fatal("retired edit source newly admitted")
	}
	var registration domainpackage.DevelopmentSourceRegistrationV1
	if err := json.Unmarshal([]byte(identity.SourceRegistrationJSON), &registration); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginapp.NewDevelopmentSourceBindingV1(registration, source, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}); !errors.Is(err, pluginapp.ErrPackageAuthority) {
		t.Fatal("retired edit source acquired service binding", err)
	}
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		Origin: domainplugin.DevelopmentSourceOriginV1, SourceRegistrationSHA256: identity.SourceRegistrationSHA256,
		Target: domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}, PluginName: id, PluginVersion: "0.1.0",
		SourceRoot: source, SourceTreeSHA256: identity.TreeSHA256, SourceTreeFileCount: identity.FileCount,
		ManifestSHA256: identity.ManifestSHA256, RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPackageStoreV1(realTempDir(t), id, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority := newTestAuthority()
	if _, err := store.Materialize(ctx, intent, authority, now); !errors.Is(err, pluginport.ErrInvalid) {
		t.Fatal("retired intent materialized as new source", err)
	}
	if _, err := store.ensurePrivateDirectory(pluginParentRelativeV1(id)); err != nil {
		t.Fatal(err)
	}
	active := store.absolute(activeRelativeV1(id, "0.1.0"))
	if err := copySourceTree(ctx, source, active); err != nil {
		t.Fatal(err)
	}
	if err := store.writeInstallMarker(active, intent); err != nil {
		t.Fatal(err)
	}
	generation := generationIDV1(intent, authority.KeyID())
	receipt, err := store.loadOrCreateReceipt(ctx, filepath.Join(store.absolute(controlRelativeV1+"/receipts"), generation+".json"), intent, generation, authority.KeyID(), authority.PublicKey(), authority, now)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainplugin.NewIndexV1(receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainplugin.IndexV1Bytes(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishActiveIndex(body); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: generation, DesiredState: domainplugin.DesiredEnabledV1}, authority, now); err != nil {
		t.Fatal(err)
	}
	return store, authority, now
}

func TestDevelopmentSourceHistoricalPreviewUpgradeV1(t *testing.T) {
	store, authority, now := historicalDevelopmentStoreFixtureV1(t)
	assertDevelopmentPreviewUpgradeV1(t, store, authority, now)
}

func TestDevelopmentSourceHistoricalIntegrityStillRequiredV1(t *testing.T) {
	for _, damage := range []string{"tree", "signature"} {
		t.Run(damage, func(t *testing.T) {
			store, authority, now := historicalDevelopmentStoreFixtureV1(t)
			old, err := store.ResolveActive(context.Background(), authority)
			if err != nil {
				t.Fatal(err)
			}
			if damage == "tree" {
				if err := os.WriteFile(filepath.Join(store.absolute(old.Receipt.ActiveRelativePath), "assets/adapter.json"), []byte(`{"changed":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				receipt := old.Receipt
				receipt.AuthoritySignature = strings.Repeat("A", len(receipt.AuthoritySignature))
				body, _ := json.Marshal(receipt)
				if err := os.WriteFile(filepath.Join(store.absolute(controlRelativeV1+"/receipts"), receipt.GenerationID+".json"), body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.ResolveActive(context.Background(), authority); !errors.Is(err, pluginport.ErrCorrupt) {
				t.Fatal("damaged historical installation resolved", err)
			}
			if _, err := store.ReadActivation(context.Background(), authority); !errors.Is(err, pluginport.ErrCorrupt) {
				t.Fatal("damaged historical installation remained enabled", err)
			}
			source := writeDevelopmentSourceV1(t, old.Receipt.PluginName)
			setDevelopmentFixtureCapabilityV1(t, source, "office.local-preview", "read-only")
			binding := developmentBindingV1(t, source)
			intent, err := binding.NewIntentV1(now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Materialize(context.Background(), intent, authority, now.Add(time.Second)); !errors.Is(err, pluginport.ErrCorrupt) {
				t.Fatal("upgrade concealed corrupt historical state", err)
			}
		})
	}
}
