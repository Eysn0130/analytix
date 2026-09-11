package runtimeapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginauthority "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationauthority"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	mcp "analytix.local/runtime-go/internal/mcp"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

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

type unavailableBundledFundsDatasetAuthorityV2 struct{}

func (unavailableBundledFundsDatasetAuthorityV2) WithCurrentSelectionV2(
	context.Context,
	datasetsnapshotport.ResolveInputV2,
	domainsecurity.TurnSecurityContext,
	func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	return errors.New("unavailable")
}

func TestBundledFundsHostMissingSourceReadRequestCannotReachProductionCountAdvertisement(t *testing.T) {
	fixture := newBundledFundsHostValidationFixtureWithDeclarationV1(
		t,
		"0.16.16",
		domainplugin.EntrypointRelativePathV1,
		func(declaration *domainpluginpackage.DeclarationV1) {
			requests := declaration.RequestedCapabilities[:0]
			for _, request := range declaration.RequestedCapabilities {
				if request.ID != "funds.source.read" {
					requests = append(requests, request)
				}
			}
			declaration.RequestedCapabilities = requests
		},
	)
	hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
		context.Background(), fixture.config, fixture.dependencies,
	)
	if err != nil {
		t.Fatalf("missing source-read request escaped the Funds-only startup lane: %v", err)
	}
	manager := mcp.NewProductionManagerWithOptions(nil, mcp.ProductionManagerOptions{
		DatasetAuthority: unavailableBundledFundsDatasetAuthorityV2{},
		HostFundsServer:  hostSpec,
	})
	manager.Connect()
	defer manager.Disconnect()
	for _, toolName := range manager.LiveTools() {
		if toolName == "mcp__analytix_funds__count_case_rows" {
			t.Fatal("package without the exact funds.source.read request reached production count advertisement")
		}
	}
}

func TestBundledFundsHostRevalidatesCurrentPackageAndInstalledGeneration(t *testing.T) {
	fixture := newBundledFundsHostValidationFixtureV1(t)
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, fixture.dependencies,
	); err != nil {
		t.Fatalf("exact installed generation rejected: %v", err)
	}
	entrypoint := filepath.Join(
		fixture.activeRoot,
		filepath.FromSlash(domainplugin.EntrypointRelativePathV1),
	)
	if err := os.WriteFile(entrypoint, []byte("export const tampered = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, fixture.dependencies,
	); !errors.Is(err, errBundledFundsHostUnavailableV1) {
		t.Fatalf("installed tree drift was not capability-local: %v", err)
	}
}

func TestBundledFundsHostStartupConsumesAdmittedReceiptVersion(t *testing.T) {
	fixture := newBundledFundsHostValidationFixtureForVersionV1(t, "0.16.17")
	var consumed mcp.ServerSpec
	dependencies := fixture.dependencies
	dependencies.newHostSpec = func(spec mcp.ServerSpec, input domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error) {
		consumed = spec
		return mcp.NewHostFundsServerSpecV1(spec, input)
	}
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, dependencies,
	); err != nil {
		t.Fatalf("newly admitted installed generation rejected at startup: %v", err)
	}
	if consumed.ExpectedServerVersion != "0.16.17" {
		t.Fatalf("MCP startup did not consume admitted receipt version: %#v", consumed)
	}
}

func TestBundledFundsHostStartupConsumesCanonicalDeclarationEntrypoint(t *testing.T) {
	const futureEntrypoint = "mcp/future-server.mjs"
	fixture := newBundledFundsHostValidationFixtureForVersionAndEntrypointV1(t, "0.16.17", futureEntrypoint)
	var consumed mcp.ServerSpec
	dependencies := fixture.dependencies
	dependencies.newHostSpec = func(spec mcp.ServerSpec, input domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error) {
		consumed = spec
		return mcp.NewHostFundsServerSpecV1(spec, input)
	}
	hostSpec, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, dependencies,
	)
	if err != nil || hostSpec == nil {
		t.Fatalf("canonical declaration entrypoint did not reach Host startup: spec=%#v err=%v", hostSpec, err)
	}
	expected := filepath.Join(fixture.activeRoot, filepath.FromSlash(futureEntrypoint))
	if consumed.EntrypointPath != expected || len(consumed.Args) != 1 || consumed.Args[0] != expected {
		t.Fatalf("Host startup did not consume the canonical declaration entrypoint: %#v", consumed)
	}
}

func TestBundledFundsHostReappliesStaticAdmissionToCanonicalSignedGeneration(t *testing.T) {
	fixture := newBundledFundsHostValidationFixtureWithDeclarationV1(
		t,
		"0.16.16",
		domainplugin.EntrypointRelativePathV1,
		func(declaration *domainpluginpackage.DeclarationV1) {
			declaration.RequestedCapabilities = append(
				declaration.RequestedCapabilities,
				domainpluginpackage.CapabilityRequestV1{
					ID: "funds.admin", ProtocolVersion: 1,
					ScopeConstraints: []string{"case:bound"},
				},
			)
		},
	)
	hostSpecCreated := false
	dependencies := fixture.dependencies
	dependencies.newHostSpec = func(mcp.ServerSpec, domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error) {
		hostSpecCreated = true
		return nil, errors.New("over-ceiling canonical generation reached Host spec creation")
	}
	hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
		context.Background(), fixture.config, dependencies,
	)
	if err != nil || hostSpec != nil || hostSpecCreated {
		t.Fatalf(
			"signed canonical generation bypassed current static admission: spec=%#v created=%t err=%v",
			hostSpec, hostSpecCreated, err,
		)
	}
}

func TestBundledFundsHostMigratesVerifiedLegacyV0AtProductionConsumer(t *testing.T) {
	fixture := newBundledFundsLegacyV0HostFixture(t)
	var consumed mcp.ServerSpec
	dependencies := fixture.dependencies
	dependencies.newHostSpec = func(spec mcp.ServerSpec, input domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error) {
		consumed = spec
		return mcp.NewHostFundsServerSpecV1(spec, input)
	}
	hostSpec, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, dependencies,
	)
	if err != nil {
		t.Fatalf("verified legacy-v0 generation was not migrated at the production consumer: %v", err)
	}
	if hostSpec == nil || consumed.ExpectedServerName != "analytix_funds" ||
		consumed.ExpectedServerVersion != "0.16.16" {
		t.Fatalf("legacy-v0 migration did not reach the exact Host Funds projection: %#v", consumed)
	}
	if len(consumed.ReadOnlyToolNames) != 2 || !consumed.ReadOnlyToolNames["count_case_rows"] ||
		!consumed.ReadOnlyToolNames["analyze_account_flows"] {
		t.Fatalf("legacy-v0 package metadata widened the Host-owned tool ceiling: %#v", consumed.ReadOnlyToolNames)
	}
	for _, root := range []string{fixture.inspection.PluginSourceRoot, fixture.activeRoot} {
		if _, statErr := os.Lstat(filepath.Join(root, ".analytix-plugin", "package.json")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("legacy-v0 migration wrote a canonical companion into %s: %v", root, statErr)
		}
	}
}

func TestBundledFundsHostLegacyV0FailuresRemainFundsOnlyAtStartup(t *testing.T) {
	tests := map[string]func(*testing.T, bundledFundsHostValidationFixtureV1){
		"present directory companion": func(t *testing.T, fixture bundledFundsHostValidationFixtureV1) {
			path := filepath.Join(fixture.inspection.PluginSourceRoot, ".analytix-plugin", "package.json")
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"present invalid companion": func(t *testing.T, fixture bundledFundsHostValidationFixtureV1) {
			path := filepath.Join(fixture.inspection.PluginSourceRoot, ".analytix-plugin", "package.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"unknown":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"tampered artifact": func(t *testing.T, fixture bundledFundsHostValidationFixtureV1) {
			if err := os.WriteFile(
				filepath.Join(fixture.inspection.PluginSourceRoot, filepath.FromSlash(domainplugin.EntrypointRelativePathV1)),
				[]byte("export const tampered = true\n"),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
		},
		"missing installation authority": func(t *testing.T, fixture bundledFundsHostValidationFixtureV1) {
			if err := os.Remove(pluginauthority.KeyPathV1(fixture.config.DataDir)); err != nil {
				t.Fatal(err)
			}
		},
		"spoofed install marker": func(t *testing.T, fixture bundledFundsHostValidationFixtureV1) {
			if err := os.WriteFile(
				filepath.Join(fixture.activeRoot, domainplugin.InstallMarkerFileNameV1),
				[]byte(`{"managedBy":"analytix-hub","requestedCapabilities":["funds.admin"]}`),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newBundledFundsLegacyV0HostFixture(t)
			mutate(t, fixture)
			hostSpecCreated := false
			dependencies := fixture.dependencies
			dependencies.newHostSpec = func(mcp.ServerSpec, domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error) {
				hostSpecCreated = true
				return nil, errors.New("legacy-v0 failure reached Host spec creation")
			}
			hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
				context.Background(), fixture.config, dependencies,
			)
			if err != nil || hostSpec != nil || hostSpecCreated {
				t.Fatalf(
					"legacy-v0 failure escaped the additive Funds lane: spec=%#v created=%t err=%v",
					hostSpec, hostSpecCreated, err,
				)
			}
		})
	}
}

func TestBundledFundsHostRejectsPackagedSourceDrift(t *testing.T) {
	fixture := newBundledFundsHostValidationFixtureV1(t)
	manifest := filepath.Join(fixture.inspection.PluginSourceRoot, ".codex-plugin", "plugin.json")
	if err := os.WriteFile(manifest, []byte(`{"name":"analytix-fund-analysis","version":"0.16.15"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, fixture.dependencies,
	); !errors.Is(err, errBundledFundsHostUnavailableV1) {
		t.Fatalf("packaged source drift was not rejected locally: %v", err)
	}
}

func TestBundledFundsHostClassifiesOnlyExpectedLocalFailures(t *testing.T) {
	fixture := newBundledFundsHostValidationFixtureV1(t)
	missing := fixture.dependencies
	missing.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
		return packagedauthorityfs.InspectionV2{}, packagedauthorityfs.ErrPackagedFundsPluginRootUnavailable
	}
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, missing,
	); !errors.Is(err, errBundledFundsHostUnavailableV1) {
		t.Fatalf("missing packaged source was not capability-local: %v", err)
	}
	postAnchorDrift := fixture.dependencies
	postAnchorDrift.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
		return packagedauthorityfs.InspectionV2{}, errors.Join(
			packagedauthorityfs.ErrPackagedFundsPluginRootUnavailable,
			errors.New("funds plugin source changed after anchor verification"),
		)
	}
	if hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
		context.Background(), fixture.config, postAnchorDrift,
	); err != nil || hostSpec != nil {
		t.Fatalf("post-anchor Funds drift escaped its startup lane: spec=%#v err=%v", hostSpec, err)
	}

	unknown := fixture.dependencies
	unknown.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
		return packagedauthorityfs.InspectionV2{}, errors.New("private package detail")
	}
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, unknown,
	); err == nil || errors.Is(err, errBundledFundsHostUnavailableV1) ||
		strings.Contains(err.Error(), "private package detail") {
		t.Fatalf("unexpected package failure was downgraded or leaked: %v", err)
	}
	if hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
		context.Background(), fixture.config, unknown,
	); err == nil || hostSpec != nil || errors.Is(err, errBundledFundsHostUnavailableV1) {
		t.Fatalf("non-Funds package failure was downgraded at startup: spec=%#v err=%v", hostSpec, err)
	}
	for name, cause := range map[string]error{
		"canceled package inspection": context.Canceled,
		"deadline package inspection": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			contextFailure := fixture.dependencies
			contextFailure.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
				return packagedauthorityfs.InspectionV2{}, cause
			}
			hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
				context.Background(), fixture.config, contextFailure,
			)
			if hostSpec != nil || !errors.Is(err, cause) || errors.Is(err, errBundledFundsHostUnavailableV1) {
				t.Fatalf("package inspection context failure was swallowed: spec=%#v err=%v", hostSpec, err)
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		cancelled, fixture.config, fixture.dependencies,
	); !errors.Is(err, context.Canceled) || errors.Is(err, errBundledFundsHostUnavailableV1) {
		t.Fatalf("cancellation was downgraded: %v", err)
	}
}

func TestBundledFundsHostSpecFailureDisablesOnlyFunds(t *testing.T) {
	fixture := newBundledFundsHostValidationFixtureV1(t)
	fixture.dependencies.newHostSpec = func(mcp.ServerSpec, domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error) {
		return nil, errors.New("private host spec detail")
	}
	if _, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(), fixture.config, fixture.dependencies,
	); !errors.Is(err, errBundledFundsHostUnavailableV1) ||
		strings.Contains(err.Error(), "private host spec detail") {
		t.Fatalf("host-spec failure escaped its capability boundary: %v", err)
	}
}

type bundledFundsHostValidationFixtureV1 struct {
	config       Config
	inspection   packagedauthorityfs.InspectionV2
	activeRoot   string
	dependencies bundledFundsHostValidationDependenciesV1
}

type legacyFundsTreeRecordV0 struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

const legacyFundsMCPEntrypointFixtureV0 = "mcp/server.mjs"

func newBundledFundsLegacyV0HostFixture(t *testing.T) bundledFundsHostValidationFixtureV1 {
	t.Helper()
	runtimeHome, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(runtimeHome, "data")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.Join(runtimeHome, "package", "plugins", "analytix-fund-analysis")
	activeRoot := filepath.Join(
		runtimeHome, "plugins", "cache", "analytix-hub", "analytix-fund-analysis", "0.16.16",
	)
	files := map[string]string{
		filepath.Join(".codex-plugin", "plugin.json"): `{"name":"analytix-fund-analysis","version":"0.16.16","interface":{"capabilities":["Interactive","Read"]}}`,
		".mcp.json": `{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs"]}}}`,
		filepath.FromSlash(legacyFundsMCPEntrypointFixtureV0): "export const server = 'analytix_funds'\n",
		filepath.Join("assets", "icon.png"):                   "legacy-icon\n",
		filepath.Join("assets", "logo.png"):                   "legacy-logo\n",
		filepath.Join("agents", "openai.yaml"):                "interface:\n  display_name: Analytix Funds\n",
	}
	for _, skill := range legacyFundsSkillNamesFixtureV0 {
		files[filepath.Join("skills", skill, "SKILL.md")] = "---\nname: " + skill + "\n---\n"
	}
	for _, root := range []string{sourceRoot, activeRoot} {
		for relative, body := range files {
			target := filepath.Join(root, relative)
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	treeSHA256, fileCount, manifestSHA256, entrypointSHA256 := legacyFundsTreeIdentityV0(t, sourceRoot)
	sourceIdentity, err := pluginstore.InspectPackagedFundsSourceTreeV1(context.Background(), sourceRoot)
	if err != nil {
		t.Fatalf("legacy-v0 RED harness source is not a recognized historical artifact: %v", err)
	}
	if sourceIdentity.TreeSHA256 != treeSHA256 || sourceIdentity.FileCount != fileCount ||
		sourceIdentity.ManifestSHA256 != manifestSHA256 || sourceIdentity.EntrypointSHA256 != entrypointSHA256 {
		t.Fatalf("legacy-v0 RED harness identity drifted: inspected=%#v", sourceIdentity)
	}
	authority, err := pluginauthority.OpenOrCreateV1(dataDir, false)
	if err != nil {
		t.Fatal(err)
	}
	requestedAt := time.Date(2026, 8, 27, 3, 4, 5, 0, time.UTC)
	authorityFileSHA256 := strings.Repeat("a", 64)
	target := domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}
	intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		PackageAuthoritySHA256: authorityFileSHA256,
		Target:                 target,
		PluginName:             domainplugin.PluginNameV1,
		PluginVersion:          "0.16.16",
		SourceRoot:             sourceRoot,
		SourceTreeSHA256:       treeSHA256,
		SourceTreeFileCount:    fileCount,
		ManifestSHA256:         manifestSHA256,
		EntrypointSHA256:       entrypointSHA256,
		RequestedAt:            requestedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	generationID := strings.Repeat("c", 64)
	activeRelative := filepath.ToSlash(filepath.Join(
		"plugins", "cache", "analytix-hub", domainplugin.PluginNameV1, "0.16.16",
	))
	receipt, err := domainplugin.NewReceiptV1(
		intent, generationID, activeRelative, requestedAt, authority.KeyID(), authority.PublicKey(),
		func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainplugin.NewIndexV1(receipt, requestedAt)
	if err != nil {
		t.Fatal(err)
	}
	controlRoot := filepath.Join(runtimeHome, ".state", "bundled-plugin-materialization", "v1")
	if err := os.MkdirAll(filepath.Join(controlRoot, "receipts"), 0o700); err != nil {
		t.Fatal(err)
	}
	receiptBody, err := domainplugin.ReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, err := domainplugin.IndexV1Bytes(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(controlRoot, "receipts", generationID+".json"), receiptBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(controlRoot, "active-index.v1.json"), indexBody, 0o600); err != nil {
		t.Fatal(err)
	}
	markerBody, err := json.Marshal(struct {
		ManagedBy           string `json:"managedBy"`
		MarketplaceName     string `json:"marketplaceName"`
		PluginName          string `json:"pluginName"`
		Version             string `json:"version"`
		PackageSHA256       string `json:"packageSha256"`
		SourcePath          string `json:"sourcePath"`
		SourceTreeSHA256    string `json:"sourceTreeSha256"`
		SourceTreeFileCount uint64 `json:"sourceTreeFileCount"`
		InstallType         string `json:"installType"`
	}{
		ManagedBy: "analytix-hub", MarketplaceName: "analytix-hub",
		PluginName: domainplugin.PluginNameV1, Version: "0.16.16",
		PackageSHA256: authorityFileSHA256, SourcePath: sourceRoot,
		SourceTreeSHA256: treeSHA256, SourceTreeFileCount: fileCount, InstallType: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeRoot, domainplugin.InstallMarkerFileNameV1), markerBody, 0o600); err != nil {
		t.Fatal(err)
	}
	inspection := packagedauthorityfs.InspectionV2{
		ApplicationRunnerPath: filepath.Join(runtimeHome, "analytix.app", "Contents", "MacOS", "analytix"),
		PluginSourceRoot:      sourceRoot,
		PluginSourceIdentity:  sourceIdentity,
		AuthorityFileSHA256:   authorityFileSHA256,
		Authority: domainauthority.ParsedAuthorityV2{
			Authority: domainauthority.AuthorityV2{
				TargetKey:       "darwin-arm64",
				AuthorityDigest: strings.Repeat("b", 64),
				Classification:  "controlled_release_clean_candidate_non_publishable",
			},
			Controlled: &domainauthority.ControlledReleaseDispositionV2{
				Kind: domainauthority.ControlledDispositionKindV2,
			},
		},
		PackageAnchor: "macos_developer_id_resource_seal",
	}
	dependencies := bundledFundsHostValidationDependenciesV1{
		inspectPackage: func(context.Context) (packagedauthorityfs.InspectionV2, error) {
			return inspection, nil
		},
		inspectSource: pluginstore.InspectPackagedFundsSourceTreeV1,
		openAuthority: func(dataDir string, stateExists bool) (pluginport.InstallationAuthority, error) {
			return pluginauthority.OpenOrCreateV1(dataDir, stateExists)
		},
		openStore: func(runtimeHome string) (pluginport.Store, error) {
			return pluginstore.OpenExistingStoreV1(runtimeHome)
		},
		newService:  pluginapp.NewService,
		newHostSpec: mcp.NewHostFundsServerSpecV1,
	}
	return bundledFundsHostValidationFixtureV1{
		config: Config{DataDir: dataDir}, inspection: inspection,
		activeRoot: activeRoot, dependencies: dependencies,
	}
}

func legacyFundsTreeIdentityV0(t *testing.T, root string) (string, uint64, string, string) {
	t.Helper()
	records := make([]legacyFundsTreeRecordV0, 0, 32)
	manifestSHA256 := ""
	entrypointSHA256 := ""
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == domainplugin.InstallMarkerFileNameV1 {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		digestHex := hex.EncodeToString(digest[:])
		records = append(records, legacyFundsTreeRecordV0{Path: relative, Size: int64(len(body)), SHA256: digestHex})
		switch relative {
		case domainplugin.ManifestRelativePathV1:
			manifestSHA256 = digestHex
		case legacyFundsMCPEntrypointFixtureV0:
			entrypointSHA256 = digestHex
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(records, func(left, right int) bool { return records[left].Path < records[right].Path })
	body, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), uint64(len(records)), manifestSHA256, entrypointSHA256
}

func newBundledFundsHostValidationFixtureV1(t *testing.T) bundledFundsHostValidationFixtureV1 {
	return newBundledFundsHostValidationFixtureForVersionV1(t, "0.16.16")
}

func newBundledFundsHostValidationFixtureForVersionV1(
	t *testing.T,
	packageVersion string,
) bundledFundsHostValidationFixtureV1 {
	return newBundledFundsHostValidationFixtureForVersionAndEntrypointV1(
		t,
		packageVersion,
		domainplugin.EntrypointRelativePathV1,
	)
}

func newBundledFundsHostValidationFixtureForVersionAndEntrypointV1(
	t *testing.T,
	packageVersion string,
	entrypointRelativePath string,
) bundledFundsHostValidationFixtureV1 {
	return newBundledFundsHostValidationFixtureWithDeclarationV1(
		t,
		packageVersion,
		entrypointRelativePath,
		nil,
	)
}

func newBundledFundsHostValidationFixtureWithDeclarationV1(
	t *testing.T,
	packageVersion string,
	entrypointRelativePath string,
	mutateDeclaration func(*domainpluginpackage.DeclarationV1),
) bundledFundsHostValidationFixtureV1 {
	t.Helper()
	runtimeHome, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return newBundledFundsHostValidationFixtureAtRuntimeHomeV1(t, runtimeHome, packageVersion, entrypointRelativePath, mutateDeclaration)
}

func newBundledFundsHostValidationFixtureAtRuntimeHomeV1(
	t *testing.T,
	runtimeHome string,
	packageVersion string,
	entrypointRelativePath string,
	mutateDeclaration func(*domainpluginpackage.DeclarationV1),
) bundledFundsHostValidationFixtureV1 {
	t.Helper()
	dataDir := filepath.Join(runtimeHome, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.Join(runtimeHome, "package", "plugins", "analytix-fund-analysis")
	if err := os.MkdirAll(filepath.Join(sourceRoot, ".codex-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "mcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		filepath.Join(".analytix-plugin", "package.json"): `{"schemaVersion":1,"packageId":"analytix-fund-analysis","packageVersion":"0.16.16","contributions":{"skills":[],"mcpServers":[{"id":"analytix_funds","entrypoint":"mcp/server.mjs"}],"hooks":[],"assets":[],"publicUi":[]},"requestedCapabilities":[{"id":"funds.case.read","protocolVersion":1,"scopeConstraints":["case:bound","source:verified"]},{"id":"funds.source.read","protocolVersion":1,"scopeConstraints":["case:bound","source:verified"]}],"lifecycle":{"protocolVersion":1,"entryPolicy":"host-static-first-party"}}`,
		filepath.Join(".codex-plugin", "plugin.json"):     `{"name":"analytix-fund-analysis","version":"0.16.16"}`,
		".mcp.json": `{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs"]}}}`,
		filepath.FromSlash(entrypointRelativePath): "export const server = 'analytix_funds'\n",
	} {
		body = strings.ReplaceAll(body, "0.16.16", packageVersion)
		body = strings.ReplaceAll(body, "mcp/server.mjs", entrypointRelativePath)
		if name == filepath.Join(".analytix-plugin", "package.json") && mutateDeclaration != nil {
			declaration, err := domainpluginpackage.ParseDeclarationV1([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			mutateDeclaration(&declaration)
			canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
			if err != nil {
				t.Fatal(err)
			}
			body = string(canonical)
		}
		target := filepath.Join(sourceRoot, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source, err := pluginstore.InspectSourceTreeV1(context.Background(), sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := pluginauthority.OpenOrCreateV1(dataDir, false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := pluginstore.NewStore(runtimeHome, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 2, 3, 4, 0, time.UTC)
	authorityFileSHA256 := strings.Repeat("a", 64)
	target := domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}
	intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		PackageAuthoritySHA256: authorityFileSHA256, Target: target,
		PluginName: source.Declaration.PackageID, PluginVersion: source.Declaration.PackageVersion,
		SourceRoot: source.RootRealPath, SourceTreeSHA256: source.TreeSHA256,
		SourceTreeFileCount: source.FileCount, ManifestSHA256: source.ManifestSHA256,
		EntrypointSHA256: source.EntrypointSHA256, RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := pluginapp.NewService(store, authority, pluginapp.FormalPackageBindingV1{
		AuthoritySHA256: authorityFileSHA256, Target: target,
		PackageIdentity: domainpluginpackage.PackageIdentityV1{
			PackageID: source.Declaration.PackageID, PackageVersion: source.Declaration.PackageVersion,
		},
		DeclarationRawSHA256:       source.Declaration.RawSHA256,
		DeclarationCanonicalSHA256: source.Declaration.CanonicalSHA256,
		SourceTreeSHA256:           source.TreeSHA256, SourceTreeFileCount: source.FileCount,
		ManifestSHA256: source.ManifestSHA256, EntrypointSHA256: source.EntrypointSHA256,
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Materialize(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	inspection := packagedauthorityfs.InspectionV2{
		ApplicationRunnerPath: filepath.Join(runtimeHome, "analytix.app", "Contents", "MacOS", "analytix"),
		PluginSourceRoot:      sourceRoot,
		PluginSourceIdentity:  source,
		AuthorityFileSHA256:   authorityFileSHA256,
		Authority: domainauthority.ParsedAuthorityV2{
			Authority: domainauthority.AuthorityV2{
				TargetKey:       "darwin-arm64",
				AuthorityDigest: strings.Repeat("b", 64),
				Classification:  "controlled_release_clean_candidate_non_publishable",
			},
			Controlled: &domainauthority.ControlledReleaseDispositionV2{
				Kind: domainauthority.ControlledDispositionKindV2,
			},
		},
		PackageAnchor: "macos_developer_id_resource_seal",
	}
	dependencies := bundledFundsHostValidationDependenciesV1{
		inspectPackage: func(context.Context) (packagedauthorityfs.InspectionV2, error) {
			return inspection, nil
		},
		inspectSource: pluginstore.InspectSourceTreeV1,
		openAuthority: func(dataDir string, stateExists bool) (pluginport.InstallationAuthority, error) {
			return pluginauthority.OpenOrCreateV1(dataDir, stateExists)
		},
		openStore: func(runtimeHome string) (pluginport.Store, error) {
			return pluginstore.OpenExistingStoreV1(runtimeHome)
		},
		newService:  pluginapp.NewService,
		newHostSpec: mcp.NewHostFundsServerSpecV1,
	}
	return bundledFundsHostValidationFixtureV1{
		config:       Config{DataDir: dataDir},
		inspection:   inspection,
		activeRoot:   filepath.Join(runtimeHome, filepath.FromSlash(result.Index.ActiveRelativePath)),
		dependencies: dependencies,
	}
}

func TestBundledFundsUnpackagedRuntimeKeepsCapabilityUnavailable(t *testing.T) {
	spec, err := admitBundledFundsHostForStartupV1(context.Background(), Config{})
	if err != nil || spec != nil {
		t.Fatalf("unpackaged runtime blocked ordinary startup: specPresent=%t err=%v", spec != nil, err)
	}
}

func TestBundledFundsNonpackageClassificationDoesNotMaskMixedFailure(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		dependencies := defaultBundledFundsHostValidationDependenciesV1()
		dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
			err := packagedauthorityfs.ErrNotPackagedRuntimeV2
			if mixed {
				err = errors.Join(err, errors.New("synthetic unexpected I/O"))
			}
			return packagedauthorityfs.InspectionV2{}, err
		}
		dependencies.inspectSource = func(context.Context, string) (pluginstore.SourceTreeIdentityV1, error) {
			t.Fatal("nonpackage classification reached source inspection")
			return pluginstore.SourceTreeIdentityV1{}, nil
		}
		spec, err := admitBundledFundsHostForStartupWithDependenciesV1(context.Background(), Config{}, dependencies)
		if spec != nil || (err != nil) != mixed {
			t.Fatalf("mixed=%t specPresent=%t err=%v", mixed, spec != nil, err)
		}
	}
}
