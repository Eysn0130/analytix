package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	mcpidentity "analytix.local/runtime-go/internal/adapters/outbound/mcp/identity"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

type unavailableHostFundsDatasetAuthorityV2 struct{}

const hostFundsTestPackageVersionV1 = "0.16.16"

func (unavailableHostFundsDatasetAuthorityV2) WithCurrentSelectionV2(
	context.Context,
	datasetsnapshotport.ResolveInputV2,
	domainsecurity.TurnSecurityContext,
	func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	return errors.New("unavailable")
}

func TestOrdinaryConfigurationCannotEnableReservedFundsWithDatasetAuthority(t *testing.T) {
	spec := hostFundsServerSpecFixtureForVersionV1(t, "0.16.17")
	manager := NewProductionManagerWithOptions(
		[]ServerSpec{spec},
		ProductionManagerOptions{DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}},
	)
	if len(manager.specs) != 1 || manager.specs[0].ID != "analytix_funds" ||
		manager.specs[0].Transport != caseFactHostQuarantineTransport ||
		manager.specs[0].Command != "" || manager.hostFundsFingerprint != "" {
		t.Fatalf("ordinary config escaped reserved funds quarantine: %#v", manager.specs)
	}
}

func TestOrdinaryJSONCannotConstructHostFundsMaterializationAuthority(t *testing.T) {
	for name, document := range map[string]string{
		"host identity source":  `{"mcpServers":{"analytix_funds":{"transport":"stdio","command":"/Applications/analytix.app/Contents/MacOS/analytix","trustScope":"user","identitySource":"host-installed-generation-v1"}}}`,
		"private marker digest": `{"mcpServers":{"analytix_funds":{"transport":"stdio","command":"/Applications/analytix.app/Contents/MacOS/analytix","trustScope":"user","hostInstallMarkerSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadMCPJSONDocument([]byte(document), "/workspace"); err == nil {
				t.Fatal("ordinary MCP JSON constructed host funds materialization authority")
			}
		})
	}
}

func TestOpaqueHostFundsSpecIsSingleAndExactCatalogOnly(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatalf("mint exact host funds spec: %v", err)
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{},
		HostFundsServer:  host,
	})
	if len(manager.specs) != 1 || manager.specs[0].ID != "analytix_funds" ||
		manager.hostFundsServer == nil ||
		manager.hostFundsFingerprint != SpecFingerprint(spec) {
		t.Fatalf("host funds capability did not inject exactly one spec: %#v", manager.specs)
	}
	if !manager.hostFundsServer.matches(spec, SpecFingerprint(spec)) {
		t.Fatal("manager did not retain an exact private host funds binding")
	}
	changedMarker := spec
	changedMarker.HostInstallMarkerSHA256 = strings.Repeat("b", 64)
	if SpecFingerprint(spec) == SpecFingerprint(changedMarker) {
		t.Fatal("host installation marker authority is absent from the server fingerprint")
	}
	count := ToolSpec{
		Name: hostFundsCountToolNameV1, Description: hostFundsCountToolDescriptionV1, ReadOnlyHint: true,
		TaskSupport:  domainmcp.ToolTaskSupportForbidden,
		InputSchema:  json.RawMessage(hostFundsInputSchemaV1),
		OutputSchema: json.RawMessage(hostFundsOutputSchemaV1),
	}
	accountFlow := ToolSpec{
		Name: hostFundsAccountFlowToolNameV1, Description: hostFundsAccountFlowDescriptionV1, ReadOnlyHint: true,
		TaskSupport:  domainmcp.ToolTaskSupportForbidden,
		InputSchema:  json.RawMessage(hostFundsAccountFlowInputSchemaV1),
		OutputSchema: json.RawMessage(hostFundsAccountFlowOutputSchemaV1),
	}
	if bytes.Contains(bytes.ToLower(accountFlow.OutputSchema), []byte(`"institution"`)) {
		t.Fatal("host funds output schema advertised a source-derived institution field")
	}
	if !exactHostFundsToolCatalogV1([]ToolSpec{count, accountFlow}, 0) ||
		!exactHostFundsToolCatalogV1([]ToolSpec{accountFlow, count}, 0) {
		t.Fatal("exact host funds evidence tool contract was rejected")
	}
	if exactHostFundsToolCatalogV1([]ToolSpec{count}, 0) ||
		exactHostFundsToolCatalogV1([]ToolSpec{count, accountFlow, {Name: "extra"}}, 0) ||
		exactHostFundsToolCatalogV1([]ToolSpec{count, accountFlow}, 1) {
		t.Fatal("extra or quarantined host funds tool contract was accepted")
	}
	changed := count
	changed.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)
	if exactHostFundsToolCatalogV1([]ToolSpec{changed, accountFlow}, 0) {
		t.Fatal("drifted host funds count schema was accepted")
	}
	changed = accountFlow
	changed.OutputSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)
	if exactHostFundsToolCatalogV1([]ToolSpec{count, changed}, 0) {
		t.Fatal("drifted host funds account-flow schema was accepted")
	}
	changed = accountFlow
	changed.Description = "drifted"
	if exactHostFundsToolCatalogV1([]ToolSpec{count, changed}, 0) {
		t.Fatal("drifted host funds account-flow description was accepted")
	}
}

func TestOpaqueHostFundsSpecPrivatelyRevalidatesStaticAdmissionInput(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	input := hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion)
	host, err := NewHostFundsServerSpecV1(spec, input)
	if err != nil || !host.sourceReadRequestedV1() {
		t.Fatalf("exact static request did not reach the opaque Host binding: host=%#v err=%v", host, err)
	}
	input.CanonicalDeclaration[0] = 'x'
	if !host.valid() {
		t.Fatal("Host binding retained caller-owned mutable admission bytes")
	}
	host.admissionInput.CanonicalDeclaration[0] = 'x'
	if host.valid() {
		t.Fatal("opaque Host binding did not re-run static admission over its private input")
	}
	if _, err := NewHostFundsServerSpecV1(spec, domainpluginpackage.StaticAdmissionInputV1{}); err == nil {
		t.Fatal("empty or caller-fabricated admission decision minted an opaque Host binding")
	}
}

func TestHostFundsSourceReadRequestDoesNotAuthorizeOrDisableAccountFlow(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	input := hostFundsStaticAdmissionInputForRequestsFixtureV1(t, spec.ExpectedServerVersion, []domainpluginpackage.CapabilityRequestV1{{
		ID: "funds.case.read", ProtocolVersion: 1,
		ScopeConstraints: []string{"case:bound", "source:verified"},
	}})
	host, err := NewHostFundsServerSpecV1(spec, input)
	if err != nil || host.sourceReadRequestedV1() {
		t.Fatalf("missing source-read request did not remain an independent Host lane: host=%#v err=%v", host, err)
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{},
		HostFundsServer:  host,
		AccountFlowExecutor: func(
			context.Context,
			AccountFlowExecutionInput,
			domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
			domainnative.AccountFlowHostEvidenceRowConsumerV1,
		) (domainnative.AccountFlowProviderSemanticResultV1, error) {
			return domainnative.AccountFlowProviderSemanticResultV1{}, nil
		},
	})
	manager.Connect()
	defer manager.Disconnect()
	accountFlowName := CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1)
	if tools := manager.LiveTools(); !reflect.DeepEqual(tools, []string{accountFlowName}) {
		t.Fatalf("missing source-read request disabled or authorized the wrong lane: %#v", tools)
	}
	events := manager.HostFundsSourceReadCapabilityEventsV1()
	if len(events) == 0 || events[len(events)-1].Decision != domainplugincapability.FundsSourceReadDecisionDeniedV1 ||
		events[len(events)-1].ReasonCode != domainplugincapability.FundsSourceReadReasonRequestMissingV1 {
		t.Fatalf("missing request did not record a typed deny decision: %#v", events)
	}
}

func TestHostFundsWrongSourceReadScopeOrProtocolCannotMintGrant(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	wrongScope := hostFundsStaticAdmissionInputForRequestsFixtureV1(t, spec.ExpectedServerVersion, []domainpluginpackage.CapabilityRequestV1{{
		ID: "funds.source.read", ProtocolVersion: 1,
		ScopeConstraints: []string{"case:bound"},
	}})
	host, err := NewHostFundsServerSpecV1(spec, wrongScope)
	if err != nil || host.sourceReadRequestedV1() {
		t.Fatalf("narrowed source-read scope minted or broke an independent Host binding: host=%#v err=%v", host, err)
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host,
	})
	manager.Connect()
	defer manager.Disconnect()
	if tools := manager.LiveTools(); len(tools) != 0 {
		t.Fatalf("narrowed source-read scope reached production advertisement: %#v", tools)
	}

	wrongProtocol := hostFundsStaticAdmissionInputForRequestsFixtureV1(t, spec.ExpectedServerVersion, []domainpluginpackage.CapabilityRequestV1{{
		ID: "funds.source.read", ProtocolVersion: 2,
		ScopeConstraints: []string{"case:bound", "source:verified"},
	}})
	if _, err := NewHostFundsServerSpecV1(spec, wrongProtocol); err == nil {
		t.Fatal("wrong source-read protocol minted an opaque Host binding")
	}
}

func TestHostFundsMCPStartupConsumesAdmittedPackageVersion(t *testing.T) {
	spec := hostFundsServerSpecFixtureForVersionV1(t, "0.16.17")
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatalf("mint dynamic admitted host funds spec: %v", err)
	}
	client, err := newHostFundsTransportClientV1(host, spec, SpecFingerprint(spec))
	if err != nil {
		t.Fatalf("start dynamic admitted host funds transport: %v", err)
	}
	t.Cleanup(client.Close)
	identity := client.ObservedServerIdentity()
	if identity.Name != "analytix_funds" || identity.Version != "0.16.17" {
		t.Fatalf("MCP startup did not consume admitted package identity: %#v", identity)
	}
}

func TestHostFundsMCPStartupConsumesAdmittedEntrypoint(t *testing.T) {
	spec := hostFundsServerSpecFixtureForVersionAndEntrypointV1(
		t, "0.16.17", "mcp/future-server.mjs",
	)
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatalf("mint host funds spec with admitted entrypoint: %v", err)
	}
	if !host.matches(spec, SpecFingerprint(spec)) {
		t.Fatal("opaque Host capability did not retain the admitted entrypoint binding")
	}
}

func TestHostFundsSpecRejectsEntrypointAuthorityDrift(t *testing.T) {
	t.Run("outside plugin root", func(t *testing.T) {
		spec := hostFundsServerSpecFixtureV1(t)
		outsideBody := []byte("outside\n")
		outside := filepath.Join(t.TempDir(), "server.mjs")
		if err := os.WriteFile(outside, outsideBody, 0o600); err != nil {
			t.Fatal(err)
		}
		spec.EntrypointPath = outside
		spec.Args = []string{outside}
		spec.EntrypointSHA256 = domainsecurity.SHA256Hex(outsideBody)
		if _, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion)); err == nil {
			t.Fatal("outside-root entrypoint minted Host authority")
		}
	})

	t.Run("symbolic link", func(t *testing.T) {
		spec := hostFundsServerSpecFixtureV1(t)
		link := filepath.Join(spec.PluginRootPath, "mcp", "linked-server.mjs")
		if err := os.Symlink(spec.EntrypointPath, link); err != nil {
			t.Fatal(err)
		}
		spec.EntrypointPath = link
		spec.Args = []string{link}
		if _, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion)); err == nil {
			t.Fatal("symbolic-link entrypoint minted Host authority")
		}
	})

	t.Run("wrong args", func(t *testing.T) {
		spec := hostFundsServerSpecFixtureV1(t)
		spec.Args = []string{filepath.Join(spec.PluginRootPath, "mcp", "other.mjs")}
		if _, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion)); err == nil {
			t.Fatal("arguments detached from the admitted entrypoint minted Host authority")
		}
	})

	t.Run("entrypoint digest mismatch", func(t *testing.T) {
		spec := hostFundsServerSpecFixtureV1(t)
		spec.EntrypointSHA256 = strings.Repeat("b", 64)
		if _, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion)); err == nil {
			t.Fatal("entrypoint digest drift minted Host authority")
		}
	})

	t.Run("source tree mismatch", func(t *testing.T) {
		spec := hostFundsServerSpecFixtureV1(t)
		spec.SourceTreeSHA256 = strings.Repeat("b", 64)
		if _, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion)); err == nil {
			t.Fatal("source-tree drift minted Host authority")
		}
	})
}

func TestHostFundsSpecCannotBeDeniedByExternalReservedNamespace(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatalf("mint exact host funds spec: %v", err)
	}
	external := ServerSpec{
		ID: "analytix_funds", Transport: "stdio", Command: "/bin/sh",
		Args: []string{"-c", "exit 99"}, CWD: t.TempDir(), TrustScope: "user",
	}
	manager := NewProductionManagerWithOptions([]ServerSpec{external}, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host,
	})
	if len(manager.specs) != 1 || manager.specs[0].ID != "analytix_funds" ||
		manager.specs[0].Command != spec.Command || manager.hostFundsFingerprint != SpecFingerprint(spec) {
		t.Fatalf("external reserved namespace denied exact host funds capability: %#v", manager.specs)
	}
}

func TestHostFundsConfigConflictPrefersExactHostAuthority(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithOptions([]ServerSpec{spec}, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{},
		HostFundsServer:  host,
	})
	if len(manager.specs) != 1 || manager.specs[0].ID != "analytix_funds" ||
		manager.hostFundsFingerprint != SpecFingerprint(spec) || len(productionNamespaceFailures(manager.specs)) != 0 {
		t.Fatalf("ordinary reserved config denied exact host authority: specs=%#v", manager.specs)
	}
}

func TestHostFundsApplicationRunnerPathSupportsExactPackagedMacOSAndWindowsShapes(t *testing.T) {
	for name, command := range map[string]string{
		"macOS":   filepath.Join(string(filepath.Separator), "Applications", "analytix.app", "Contents", "MacOS", "analytix"),
		"Windows": filepath.Join(string(filepath.Separator), "Program Files", "Analytix", "analytix.exe"),
	} {
		t.Run(name, func(t *testing.T) {
			if !validHostFundsApplicationRunnerPathV1(command) {
				t.Fatalf("supported packaged application runner was rejected: %s", command)
			}
		})
	}
	for name, command := range map[string]string{
		"renamed executable": filepath.Join(string(filepath.Separator), "Applications", "analytix.app", "Contents", "MacOS", "other"),
		"wrong mac parent":   filepath.Join(string(filepath.Separator), "Applications", "analytix.app", "MacOS", "analytix"),
		"wrong windows name": filepath.Join(string(filepath.Separator), "Program Files", "Analytix", "Analytix.exe"),
	} {
		t.Run(name, func(t *testing.T) {
			if validHostFundsApplicationRunnerPathV1(command) {
				t.Fatalf("non-canonical packaged application runner was accepted: %s", command)
			}
		})
	}
}

func hostFundsServerSpecFixtureV1(t *testing.T) ServerSpec {
	return hostFundsServerSpecFixtureForVersionV1(t, hostFundsTestPackageVersionV1)
}

func hostFundsServerSpecFixtureForVersionV1(t *testing.T, packageVersion string) ServerSpec {
	t.Helper()
	return hostFundsServerSpecFixtureForVersionAndEntrypointV1(t, packageVersion, "mcp/server.mjs")
}

func hostFundsServerSpecFixtureForVersionAndEntrypointV1(
	t *testing.T,
	packageVersion string,
	entrypointRelativePath string,
) ServerSpec {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(root, filepath.FromSlash(entrypointRelativePath))
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o700); err != nil {
		t.Fatal(err)
	}
	entrypointBody := []byte("export const server = 'analytix_funds'\n")
	if err := os.WriteFile(entrypoint, entrypointBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestBody := []byte(`{"name":"analytix-fund-analysis","version":"` + packageVersion + `"}`)
	if err := os.MkdirAll(filepath.Join(root, ".codex-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex-plugin", "plugin.json"), manifestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	sourceTreeSHA256, err := mcpidentity.ComputeSourceTreeSHA256(root)
	if err != nil {
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
		PluginName: "analytix-fund-analysis", Version: packageVersion,
		PackageSHA256: strings.Repeat("a", 64), SourcePath: root,
		SourceTreeSHA256: sourceTreeSHA256, SourceTreeFileCount: 2, InstallType: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".analytix-hub-installed-plugin.json"), markerBody, 0o600); err != nil {
		t.Fatal(err)
	}
	markerDigest := domainsecurity.SHA256Hex(markerBody)
	runner := filepath.Join(t.TempDir(), "analytix.app", "Contents", "MacOS", "analytix")
	return ServerSpec{
		ID: "analytix_funds", Transport: "stdio", Command: runner, Args: []string{entrypoint},
		Env: map[string]string{"ELECTRON_RUN_AS_NODE": "1"}, CWD: root,
		ExpectedServerName: "analytix_funds", ExpectedServerVersion: packageVersion,
		IdentitySource: mcpidentity.HostInstalledGenerationSourceV1,
		ManifestSHA256: domainsecurity.SHA256Hex(manifestBody),
		EntrypointPath: entrypoint, EntrypointSHA256: domainsecurity.SHA256Hex(entrypointBody),
		PluginRootPath: root, SourceTreeSHA256: sourceTreeSHA256,
		HostInstallMarkerSHA256: markerDigest, TrustScope: "user",
		TimeoutMS: hostFundsServerTimeoutMSV1,
		ReadOnlyToolNames: map[string]bool{
			hostFundsCountToolNameV1:       true,
			hostFundsAccountFlowToolNameV1: true,
		},
	}
}

func hostFundsStaticAdmissionInputFixtureV1(
	t *testing.T,
	packageVersion string,
) domainpluginpackage.StaticAdmissionInputV1 {
	t.Helper()
	return hostFundsStaticAdmissionInputForRequestsFixtureV1(t, packageVersion, []domainpluginpackage.CapabilityRequestV1{
		{
			ID: "funds.case.read", ProtocolVersion: 1,
			ScopeConstraints: []string{"case:bound", "source:verified"},
		},
		{
			ID: "funds.source.read", ProtocolVersion: 1,
			ScopeConstraints: []string{"case:bound", "source:verified"},
		},
	})
}

func hostFundsStaticAdmissionInputForRequestsFixtureV1(
	t *testing.T,
	packageVersion string,
	requests []domainpluginpackage.CapabilityRequestV1,
) domainpluginpackage.StaticAdmissionInputV1 {
	t.Helper()
	declaration := domainpluginpackage.DeclarationV1{
		SchemaVersion: 1, PackageID: domainpluginpackage.FirstPartyFundsPackageIDV1,
		PackageVersion: packageVersion,
		Contributions: domainpluginpackage.ContributionsV1{
			Skills:     []domainpluginpackage.PathContributionV1{},
			MCPServers: []domainpluginpackage.MCPServerContributionV1{{ID: "analytix_funds", Entrypoint: "mcp/server.mjs"}},
			Hooks:      []domainpluginpackage.PathContributionV1{}, Assets: []domainpluginpackage.PathContributionV1{},
			PublicUI: []domainpluginpackage.PathContributionV1{},
		},
		RequestedCapabilities: requests,
		Lifecycle: domainpluginpackage.LifecycleV1{
			ProtocolVersion: 1, EntryPolicy: domainpluginpackage.StaticFirstPartyEntryPolicyV1,
		},
	}
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex(canonical)
	return domainpluginpackage.StaticAdmissionInputV1{
		CanonicalDeclaration: canonical,
		DeclarationRawSHA256: digest, DeclarationCanonicalSHA256: digest,
		Evidence: domainpluginpackage.StaticAdmissionEvidenceV1{
			ArtifactIntegrityVerified: true,
			PackageAuthoritySHA256:    strings.Repeat("a", 64),
			ProvenanceAuthorityDigest: strings.Repeat("b", 64),
			ProvenanceClassification:  "controlled_release_clean_candidate_non_publishable",
			ProvenanceDispositionKind: "controlled_release_receipt",
			PlatformAnchor:            "macos_developer_id_resource_seal",
			SigningAlgorithm:          domainpluginpackage.StaticAdmissionSigningAlgorithmV1,
		},
	}
}
