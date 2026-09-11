package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginauthority "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationauthority"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const fixturePluginVersionV1 = "0.16.16"

var legacyFundsSkillNamesFixtureV0 = [...]string{
	"analytix-fund-analysis",
	"index",
	"case-context",
	"data-quality",
	"quick-fact",
	"pair-amount-investigation",
	"account-dossier",
	"subject-dossier",
	"counterparty-analysis",
	"fund-tracing",
	"investigation-lab",
	"full-case-analysis",
	"report-builder",
	"evidence-request",
	"analysis-critique",
	"claim-review",
	"delivery-qc",
	"graph-visualization",
	"visual-evidence",
	"case-workbench",
}

func TestBundledFundsMaterializationCommandIssuesCurrentRunBindingAndRestartsIdempotently(t *testing.T) {
	for _, mode := range []struct {
		name           string
		classification string
		disposition    string
		anchor         string
	}{
		{
			name: "Developer ID candidate", classification: "controlled_release_clean_candidate_non_publishable",
			disposition: domainauthority.ControlledDispositionKindV2, anchor: "macos_developer_id_resource_seal",
		},
		{
			name: "ad-hoc development", classification: "development_dirty_non_publishable",
			disposition: domainauthority.DevelopmentDispositionKindV2, anchor: "macos_nonpublishable_resource_seal",
		},
	} {
		t.Run(mode.name, func(t *testing.T) {
			fixture := newBundledFundsCommandFixtureV1(t, mode.classification, mode.disposition, mode.anchor)
			first := runBundledFundsCommandFixtureV1(t, fixture, strings.Repeat("1", 64))
			second := runBundledFundsCommandFixtureV1(t, fixture, strings.Repeat("2", 64))
			if first.InvocationID == second.InvocationID || first.Receipt.ReceiptID != second.Receipt.ReceiptID ||
				first.Index.IndexDigest != second.Index.IndexDigest {
				t.Fatalf("restart did not reuse one authorized generation: first=%#v second=%#v", first, second)
			}
			for _, ready := range []domainplugin.ReadyV1{first, second} {
				if ready.Publishable || ready.FactToolsEnabled || ready.Receipt.FactToolsEnabled || ready.Index.FactToolsEnabled {
					t.Fatal("materialization current-run result enabled facts or publication")
				}
				if ready.PackageAuthority.Classification != mode.classification ||
					ready.PackageAuthority.DispositionKind != mode.disposition || ready.PackageAuthority.PlatformAnchor != mode.anchor {
					t.Fatalf("package authority mode drifted: %#v", ready.PackageAuthority)
				}
			}
		})
	}
}

func TestBundledFundsMaterializationCommandRotatesVerifiedActiveGenerationForRebuiltPackage(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "development_dirty_non_publishable", domainauthority.DevelopmentDispositionKindV2,
		"macos_nonpublishable_resource_seal",
	)
	first := runBundledFundsCommandFixtureV1(t, fixture, strings.Repeat("6", 64))
	inspectPackage := fixture.dependencies.inspectPackage
	fixture.dependencies.inspectPackage = func(ctx context.Context) (packagedauthorityfs.InspectionV2, error) {
		inspection, err := inspectPackage(ctx)
		if err == nil {
			inspection.AuthorityFileSHA256 = strings.Repeat("d", 64)
			inspection.Authority.Authority.AuthorityDigest = strings.Repeat("e", 64)
		}
		return inspection, err
	}
	second := runBundledFundsCommandFixtureV1(t, fixture, strings.Repeat("7", 64))
	if first.Receipt.ReceiptID == second.Receipt.ReceiptID || first.Receipt.GenerationID == second.Receipt.GenerationID ||
		second.Receipt.PackageAuthoritySHA256 != strings.Repeat("d", 64) || second.PackageAuthority.AuthorityDigest != strings.Repeat("e", 64) ||
		second.FactToolsEnabled || second.Publishable || second.Receipt.FactToolsEnabled || second.Index.FactToolsEnabled {
		t.Fatalf("rebuilt package did not rotate the current generation safely: first=%#v second=%#v", first, second)
	}
	quarantineRoot := filepath.Join(
		fixture.runtimeHome, ".state", "bundled-plugin-materialization", "v1", "quarantine", second.Receipt.IntentID,
	)
	if _, err := os.Stat(filepath.Join(quarantineRoot, "prior-active-index.v1.json")); err != nil {
		t.Fatalf("prior signed active index was not preserved in quarantine: %v", err)
	}
}

func TestBundledFundsMaterializationCommandBindsAdmittedPublisherVersionAcrossSignedChain(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "development_dirty_non_publishable", domainauthority.DevelopmentDispositionKindV2,
		"macos_nonpublishable_resource_seal",
	)
	rewriteBundledFundsFixtureDeclarationV1(t, &fixture, func(declaration *domainpluginpackage.DeclarationV1) {
		declaration.PackageVersion = "0.16.17"
	})
	ready := runBundledFundsCommandFixtureV1(t, fixture, strings.Repeat("e", 64))
	if ready.PluginName != domainplugin.PluginNameV1 || ready.PluginVersion != "0.16.17" ||
		ready.Receipt.PluginName != ready.PluginName || ready.Receipt.PluginVersion != ready.PluginVersion ||
		ready.Index.PluginName != ready.PluginName || ready.Index.PluginVersion != ready.PluginVersion ||
		filepath.Base(ready.ActivePluginRoot) != ready.PluginVersion {
		t.Fatalf("admitted publisher identity/version did not bind ready->receipt->index: %#v", ready)
	}
}

func TestBundledFundsMaterializationCommandRejectsCorruptPriorGenerationDuringPackageRotation(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "development_dirty_non_publishable", domainauthority.DevelopmentDispositionKindV2,
		"macos_nonpublishable_resource_seal",
	)
	first := runBundledFundsCommandFixtureV1(t, fixture, strings.Repeat("a", 64))
	receiptPath := filepath.Join(
		fixture.runtimeHome, ".state", "bundled-plugin-materialization", "v1", "receipts", first.Receipt.GenerationID+".json",
	)
	if err := os.WriteFile(receiptPath, []byte(`{"corrupt":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	activeIndexPath := filepath.Join(
		fixture.runtimeHome, ".state", "bundled-plugin-materialization", "v1", "active-index.v1.json",
	)
	activeIndexBefore, err := os.ReadFile(activeIndexPath)
	if err != nil {
		t.Fatal(err)
	}
	inspectPackage := fixture.dependencies.inspectPackage
	fixture.dependencies.inspectPackage = func(ctx context.Context) (packagedauthorityfs.InspectionV2, error) {
		inspection, err := inspectPackage(ctx)
		if err == nil {
			inspection.AuthorityFileSHA256 = strings.Repeat("f", 64)
		}
		return inspection, err
	}
	var output bytes.Buffer
	err = runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", strings.Repeat("b", 64),
	}, &output, fixture.dependencies)
	if err == nil || output.Len() != 0 {
		t.Fatalf("corrupt prior generation was replaced during package rotation: err=%v output=%q", err, output.String())
	}
	activeIndexAfter, readErr := os.ReadFile(activeIndexPath)
	if readErr != nil || !bytes.Equal(activeIndexBefore, activeIndexAfter) {
		t.Fatalf("failed rotation changed the signed active index: err=%v", readErr)
	}
}

func TestBundledFundsMaterializationCommandRejectsUnanchoredPackageBeforeInstallationWrites(t *testing.T) {
	root := canonicalTempDirV1(t)
	dataDir := filepath.Join(root, "runtime-home", "data")
	dependencies := defaultBundledFundsMaterializationDependenciesV1()
	dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
		return packagedauthorityfs.InspectionV2{}, errors.New("independent package anchor unavailable")
	}
	var output bytes.Buffer
	err := runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", dataDir, "--invocation-id", strings.Repeat("3", 64),
	}, &output, dependencies)
	if err == nil || output.Len() != 0 {
		t.Fatalf("unanchored package was accepted: err=%v output=%q", err, output.String())
	}
	if _, statErr := os.Stat(dataDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unanchored package created installation state: %v", statErr)
	}
}

func TestBundledFundsMaterializationCommandRejectsCapabilityAboveHostCeilingBeforeInstallationEffects(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "development_dirty_non_publishable", domainauthority.DevelopmentDispositionKindV2,
		"macos_nonpublishable_resource_seal",
	)
	rewriteBundledFundsFixtureDeclarationV1(t, &fixture, func(declaration *domainpluginpackage.DeclarationV1) {
		declaration.RequestedCapabilities = append(declaration.RequestedCapabilities, domainpluginpackage.CapabilityRequestV1{
			ID: "funds.admin", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"},
		})
	})
	authorityOpened := false
	originalOpenAuthority := fixture.dependencies.openAuthority
	fixture.dependencies.openAuthority = func(dataDir string, stateExists bool) (pluginport.InstallationAuthority, error) {
		authorityOpened = true
		return originalOpenAuthority(dataDir, stateExists)
	}
	var output bytes.Buffer
	err := runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", strings.Repeat("0", 64),
	}, &output, fixture.dependencies)
	if err == nil || output.Len() != 0 {
		t.Fatalf("over-ceiling capability was admitted: err=%v output=%q", err, output.String())
	}
	if authorityOpened {
		t.Fatal("over-ceiling capability reached installation authority effects")
	}
	if _, statErr := os.Stat(pluginauthority.KeyPathV1(fixture.dataDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("over-ceiling capability created installation authority state: %v", statErr)
	}
}

func TestBundledFundsMaterializationCommandRejectsUnsupportedStaticPolicyBeforeInstallationEffects(t *testing.T) {
	tests := map[string]func(*testing.T, *bundledFundsCommandFixtureV1){
		"schema version": func(t *testing.T, fixture *bundledFundsCommandFixtureV1) {
			inspection, err := fixture.dependencies.inspectPackage(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			declarationPath := filepath.Join(
				inspection.PluginSourceRoot,
				filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1),
			)
			body, err := os.ReadFile(declarationPath)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":2`), 1)
			if err := os.WriteFile(declarationPath, body, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"lifecycle protocol": func(t *testing.T, fixture *bundledFundsCommandFixtureV1) {
			rewriteBundledFundsFixtureDeclarationV1(t, fixture, func(declaration *domainpluginpackage.DeclarationV1) {
				declaration.Lifecycle.ProtocolVersion = 2
			})
		},
		"entry policy": func(t *testing.T, fixture *bundledFundsCommandFixtureV1) {
			rewriteBundledFundsFixtureDeclarationV1(t, fixture, func(declaration *domainpluginpackage.DeclarationV1) {
				declaration.Lifecycle.EntryPolicy = "publisher-managed"
			})
		},
		"first-party identity": func(t *testing.T, fixture *bundledFundsCommandFixtureV1) {
			rewriteBundledFundsFixtureDeclarationV1(t, fixture, func(declaration *domainpluginpackage.DeclarationV1) {
				declaration.PackageID = "another-first-party"
			})
		},
		"trusted provenance": func(t *testing.T, fixture *bundledFundsCommandFixtureV1) {
			inspection, err := fixture.dependencies.inspectPackage(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			inspection.Authority.Authority.AuthorityDigest = ""
			fixture.dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
				return inspection, nil
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newBundledFundsCommandFixtureV1(
				t, "development_dirty_non_publishable", domainauthority.DevelopmentDispositionKindV2,
				"macos_nonpublishable_resource_seal",
			)
			mutate(t, &fixture)
			authorityOpened := false
			storeOpened := false
			originalOpenAuthority := fixture.dependencies.openAuthority
			fixture.dependencies.openAuthority = func(dataDir string, stateExists bool) (pluginport.InstallationAuthority, error) {
				authorityOpened = true
				return originalOpenAuthority(dataDir, stateExists)
			}
			originalNewStore := fixture.dependencies.newStore
			fixture.dependencies.newStore = func(runtimeHome string) (pluginport.Store, error) {
				storeOpened = true
				return originalNewStore(runtimeHome)
			}
			var output bytes.Buffer
			err := runBundledPluginCommandV1(context.Background(), []string{
				"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", strings.Repeat("1", 64),
			}, &output, fixture.dependencies)
			if err == nil || output.Len() != 0 || authorityOpened || storeOpened {
				t.Fatalf(
					"unsupported static policy reached installation effects: err=%v output=%q authority=%t store=%t",
					err, output.String(), authorityOpened, storeOpened,
				)
			}
			if _, statErr := os.Stat(pluginauthority.KeyPathV1(fixture.dataDir)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unsupported static policy created installation authority state: %v", statErr)
			}
		})
	}
}

func TestBundledFundsMaterializationCommandMarkerCannotMintMissingAuthority(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "development_dirty_non_publishable", domainauthority.DevelopmentDispositionKindV2,
		"macos_nonpublishable_resource_seal",
	)
	markerRoot := filepath.Join(fixture.runtimeHome, "plugins", "cache", "analytix-hub", domainplugin.PluginNameV1, fixturePluginVersionV1)
	if err := os.MkdirAll(markerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(markerRoot, domainplugin.InstallMarkerFileNameV1), []byte(`{"managedBy":"analytix-hub"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", strings.Repeat("4", 64),
	}, &output, fixture.dependencies)
	if err == nil || output.Len() != 0 {
		t.Fatalf("ordinary marker minted an authority: err=%v output=%q", err, output.String())
	}
	authorityPath := pluginauthority.KeyPathV1(fixture.dataDir)
	if _, statErr := os.Stat(authorityPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing installation authority was replaced: %v", statErr)
	}
}

func TestBundledFundsMaterializationCommandBootstrapsBesideRealLegacyLayoutAndQuarantines01615(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "controlled_release_clean_candidate_non_publishable", domainauthority.ControlledDispositionKindV2,
		"macos_developer_id_resource_seal",
	)
	// Mirror the relevant real default-data condition: unrelated runtime data
	// and a legacy 0.16.15 cache exist, while neither the Final Evidence key nor
	// the new dedicated plugin-materialization key exists.
	if err := os.WriteFile(filepath.Join(fixture.dataDir, "legacy-runtime-state.json"), []byte(`{"schemaVersion":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyRoot := filepath.Join(
		fixture.runtimeHome, "plugins", "cache", "analytix-hub", domainplugin.PluginNameV1, "0.16.15",
	)
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "legacy.txt"), []byte("legacy plugin bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ready := runBundledFundsCommandFixtureV1(t, fixture, strings.Repeat("8", 64))
	if ready.PluginVersion != "0.16.16" || ready.FactToolsEnabled || ready.Publishable {
		t.Fatalf("legacy migration widened authority: %#v", ready)
	}
	if _, err := os.Stat(legacyRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy 0.16.15 remained discoverable: %v", err)
	}
	quarantineRoot := filepath.Join(fixture.runtimeHome, ".state", "bundled-plugin-materialization", "v1", "quarantine")
	foundLegacy := false
	_ = filepath.WalkDir(quarantineRoot, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() == "legacy.txt" {
			foundLegacy = true
		}
		return nil
	})
	if !foundLegacy {
		t.Fatal("legacy 0.16.15 bytes were not retained in non-discoverable quarantine")
	}
	if _, err := os.Stat(filepath.Join(fixture.dataDir, "private", "authority", "final-answer-ed25519-v1.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plugin bootstrap created or reused Final Evidence authority: %v", err)
	}
	if _, err := os.Stat(pluginauthority.KeyPathV1(fixture.dataDir)); err != nil {
		t.Fatalf("dedicated plugin materialization authority was not enrolled: %v", err)
	}
}

func TestBundledFundsMaterializationCommandMissingDedicatedKeyWithV1StateFailsClosed(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "controlled_release_clean_candidate_non_publishable", domainauthority.ControlledDispositionKindV2,
		"macos_developer_id_resource_seal",
	)
	stateRoot := filepath.Join(fixture.runtimeHome, ".state", "bundled-plugin-materialization", "v1")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateRoot, "active-index.v1.json"), []byte(`{"forged":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", strings.Repeat("9", 64),
	}, &output, fixture.dependencies)
	if err == nil || output.Len() != 0 {
		t.Fatalf("missing dedicated key was recreated over v1 state: err=%v output=%q", err, output.String())
	}
	if _, statErr := os.Stat(pluginauthority.KeyPathV1(fixture.dataDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing dedicated authority was recreated: %v", statErr)
	}
}

func TestBundledFundsMaterializationCommandRejectsSourceDriftAfterCurrentRunInspection(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "controlled_release_clean_candidate_non_publishable", domainauthority.ControlledDispositionKindV2,
		"macos_developer_id_resource_seal",
	)
	originalInspect := fixture.dependencies.inspectSource
	fixture.dependencies.inspectSource = func(ctx context.Context, root string) (pluginstore.SourceTreeIdentityV1, error) {
		identity, err := originalInspect(ctx, root)
		if err == nil {
			err = os.WriteFile(filepath.Join(root, "README.md"), []byte("source changed after inspection\n"), 0o600)
		}
		return identity, err
	}
	var output bytes.Buffer
	err := runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", strings.Repeat("5", 64),
	}, &output, fixture.dependencies)
	if err == nil || output.Len() != 0 {
		t.Fatalf("source drift was accepted: err=%v output=%q", err, output.String())
	}
}

func TestBundledFundsMaterializationCommandNeverMintsAuthorityFromLegacyV0(t *testing.T) {
	fixture := newBundledFundsCommandFixtureV1(
		t, "development_dirty_non_publishable", domainauthority.DevelopmentDispositionKindV2,
		"macos_nonpublishable_resource_seal",
	)
	inspection, err := fixture.dependencies.inspectPackage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(
		inspection.PluginSourceRoot,
		filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1),
	)); err != nil {
		t.Fatal(err)
	}
	for relative, body := range map[string]string{
		"assets/icon.png":    "legacy-icon\n",
		"assets/logo.png":    "legacy-logo\n",
		"agents/openai.yaml": "interface:\n  display_name: Analytix Funds\n",
	} {
		path := filepath.Join(inspection.PluginSourceRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, skill := range legacyFundsSkillNamesFixtureV0 {
		path := filepath.Join(inspection.PluginSourceRoot, "skills", skill, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: "+skill+"\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inspection.PluginSourceIdentity, err = pluginstore.InspectPackagedFundsSourceTreeV1(
		context.Background(), inspection.PluginSourceRoot,
	)
	if err != nil || !inspection.PluginSourceIdentity.LegacyV0 {
		t.Fatalf("legacy-v0 command fixture is not a recognized historical artifact: identity=%#v err=%v", inspection.PluginSourceIdentity, err)
	}
	fixture.dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
		return inspection, nil
	}
	authorityOpened := false
	storeOpened := false
	fixture.dependencies.openAuthority = func(string, bool) (pluginport.InstallationAuthority, error) {
		authorityOpened = true
		return nil, errors.New("legacy-v0 reached authority enrollment")
	}
	fixture.dependencies.newStore = func(string) (pluginport.Store, error) {
		storeOpened = true
		return nil, errors.New("legacy-v0 reached materialization store")
	}
	var output bytes.Buffer
	err = runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", strings.Repeat("d", 64),
	}, &output, fixture.dependencies)
	if err == nil || output.Len() != 0 || authorityOpened || storeOpened {
		t.Fatalf(
			"legacy-v0 minted or reached installation effects: err=%v output=%q authority=%t store=%t",
			err, output.String(), authorityOpened, storeOpened,
		)
	}
	if _, statErr := os.Stat(pluginauthority.KeyPathV1(fixture.dataDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("legacy-v0 created an installation authority: %v", statErr)
	}
}

type bundledFundsCommandFixtureV1 struct {
	runtimeHome  string
	dataDir      string
	dependencies bundledFundsMaterializationDependenciesV1
}

func newBundledFundsCommandFixtureV1(t *testing.T, classification, disposition, anchor string) bundledFundsCommandFixtureV1 {
	t.Helper()
	root := canonicalTempDirV1(t)
	runtimeHome := filepath.Join(root, "runtime-home")
	dataDir := filepath.Join(runtimeHome, "data")
	source := filepath.Join(root, "packaged-source")
	for _, directory := range []string{
		runtimeHome, dataDir, filepath.Join(source, ".analytix-plugin"),
		filepath.Join(source, ".codex-plugin"), filepath.Join(source, "mcp"),
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for relative, body := range map[string]string{
		".analytix-plugin/package.json": `{"schemaVersion":1,"packageId":"analytix-fund-analysis","packageVersion":"0.16.16","contributions":{"skills":[],"mcpServers":[{"id":"analytix_funds","entrypoint":"mcp/server.mjs"}],"hooks":[],"assets":[],"publicUi":[]},"requestedCapabilities":[{"id":"funds.case.read","protocolVersion":1,"scopeConstraints":["case:bound","source:verified"]}],"lifecycle":{"protocolVersion":1,"entryPolicy":"host-static-first-party"}}`,
		".codex-plugin/plugin.json":     `{"name":"analytix-fund-analysis","version":"0.16.16"}`,
		".mcp.json":                     `{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs"]}}}`,
		"mcp/server.mjs":                "export const factsEnabled = false\n",
		"README.md":                     "packaged fixture\n",
	} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(relative)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sourceIdentity, err := pluginstore.InspectSourceTreeV1(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	parsed := domainauthority.ParsedAuthorityV2{Authority: domainauthority.AuthorityV2{
		TargetKey: "darwin-arm64", AuthorityDigest: strings.Repeat("a", 64), Classification: classification,
	}}
	if disposition == domainauthority.ControlledDispositionKindV2 {
		parsed.Controlled = &domainauthority.ControlledReleaseDispositionV2{Kind: disposition}
	} else {
		parsed.Development = &domainauthority.DevelopmentDispositionV2{Kind: disposition}
	}
	inspection := packagedauthorityfs.InspectionV2{
		PluginSourceRoot: source, PluginSourceIdentity: sourceIdentity,
		AuthorityPath:       filepath.Join(root, "authority.json"),
		AuthorityFileSHA256: strings.Repeat("b", 64), Authority: parsed,
		RuntimeIdentity: packagedauthorityfs.RuntimeIdentityV2{
			PayloadSHA256: strings.Repeat("c", 64), PayloadBytes: 4096, Format: "mach-o", Arch: "arm64",
		},
		PackageAnchor: anchor, Publishable: false, FactToolsEnabled: false,
	}
	dependencies := defaultBundledFundsMaterializationDependenciesV1()
	dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) { return inspection, nil }
	dependencies.now = func() time.Time { return time.Date(2026, 7, 23, 18, 30, 0, 0, time.UTC) }
	return bundledFundsCommandFixtureV1{runtimeHome: runtimeHome, dataDir: dataDir, dependencies: dependencies}
}

func rewriteBundledFundsFixtureDeclarationV1(
	t *testing.T,
	fixture *bundledFundsCommandFixtureV1,
	mutate func(*domainpluginpackage.DeclarationV1),
) {
	t.Helper()
	inspection, err := fixture.dependencies.inspectPackage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	declarationPath := filepath.Join(
		inspection.PluginSourceRoot,
		filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1),
	)
	declarationBody, err := os.ReadFile(declarationPath)
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := domainpluginpackage.ParseDeclarationV1(declarationBody)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&declaration)
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(declarationPath, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(inspection.PluginSourceRoot, filepath.FromSlash(domainplugin.ManifestRelativePathV1))
	if err := os.WriteFile(
		manifestPath,
		[]byte(`{"name":"`+declaration.PackageID+`","version":"`+declaration.PackageVersion+`"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	inspection.PluginSourceIdentity, err = pluginstore.InspectSourceTreeV1(context.Background(), inspection.PluginSourceRoot)
	if err != nil {
		t.Fatalf("mutated declaration did not reach the production Host policy seam: %v", err)
	}
	fixture.dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
		return inspection, nil
	}
}

func runBundledFundsCommandFixtureV1(t *testing.T, fixture bundledFundsCommandFixtureV1, invocationID string) domainplugin.ReadyV1 {
	t.Helper()
	var output bytes.Buffer
	if err := runBundledPluginCommandV1(context.Background(), []string{
		"materialize-funds-v1", "--data-dir", fixture.dataDir, "--invocation-id", invocationID,
	}, &output, fixture.dependencies); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSuffix(output.String(), "\n")
	if !strings.HasPrefix(line, bundledFundsMaterializationReadyMarkerV1) {
		t.Fatalf("current-run marker is missing: %q", line)
	}
	ready, err := domainplugin.ParseReadyV1([]byte(strings.TrimPrefix(line, bundledFundsMaterializationReadyMarkerV1)))
	if err != nil {
		t.Fatal(err)
	}
	authority, err := pluginauthority.OpenOrCreateV1(fixture.dataDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainplugin.ValidateTrustedReceiptV1(ready.Receipt, authority.KeyID(), authority.PublicKey()); err != nil {
		t.Fatal(err)
	}
	return ready
}

func canonicalTempDirV1(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
