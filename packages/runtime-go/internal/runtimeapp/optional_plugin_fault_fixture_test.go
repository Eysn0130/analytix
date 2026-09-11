//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	caseentitystore "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	mcp "analytix.local/runtime-go/internal/mcp"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type runtimeOptionalPluginLifecycleFixtureV1 struct {
	config      Config
	start       func() (http.Handler, error)
	assertFault func(startups int)
	async       *runtimeAsyncTurnGuardV1
}

func newRuntimeOptionalPluginLifecycleFixtureV1(t *testing.T, fault string, base Config) runtimeOptionalPluginLifecycleFixtureV1 {
	t.Helper()
	_, enrolled := runtimeWitnessedRegistryConfigV2(t)
	config := base
	config.DataDir = enrolled.DataDir
	config.AuthorityAnchorV1 = enrolled.AuthorityAnchorV1
	config.AuthorityManifestRoot = enrolled.AuthorityManifestRoot
	config.AuthorityCredentialProfileRoot = enrolled.AuthorityCredentialProfileRoot
	config.AuthorityCredentialBundleRoot = enrolled.AuthorityCredentialBundleRoot
	if config.UserDataDir == "" {
		t.Fatal("optional plugin lifecycle requires an isolated canonical user-data root")
	}
	dependencies := defaultBundledFundsHostValidationDependenciesV1()
	async := newRuntimeAsyncTurnGuardV1(fault)
	var inspectionCalls, admissionCalls, hostSpecCalls, starts int
	var hostSourceReadObservers []func() []domainplugincapability.FundsSourceReadDecisionEventV1
	var retainedRoots []string
	var retainedDigest string
	var assertSpecific = func() {}

	switch fault {
	case "missing":
		missingRoot := filepath.Join(t.TempDir(), "absent-funds-plugin")
		// This is the package reader's synthetic missing-root outcome. The
		// real packaged seal/native-artifact reader is a separate evidence seam.
		dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
			if _, err := os.Lstat(missingRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing package root fixture is no longer absent: %v", err)
			}
			return packagedauthorityfs.InspectionV2{}, packagedauthorityfs.ErrPackagedFundsPluginRootUnavailable
		}
	case "disabled":
		var transportCalls atomic.Int32
		forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			transportCalls.Add(1)
			http.Error(w, "disabled transport must not execute", http.StatusForbidden)
		}))
		t.Cleanup(forbidden.Close)
		document, present, err := loadRuntimeConfigDocument(config)
		if err != nil || !present {
			t.Fatalf("ordinary MCP configuration is unavailable: present=%t err=%v", present, err)
		}
		servers, ok := document["mcpServers"].(map[string]any)
		if !ok {
			servers, ok = document["servers"].(map[string]any)
		}
		if !ok {
			capabilities, _ := document["capabilities"].(map[string]any)
			mcpConfig, _ := capabilities["mcp"].(map[string]any)
			servers, ok = mcpConfig["servers"].(map[string]any)
		}
		if !ok {
			t.Fatal("ordinary MCP configuration has no server map")
		}
		servers["analytix_funds"] = map[string]any{"disabled": true, "url": forbidden.URL, "trustScope": "user"}
		body, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		config.MCPConfigJSON, config.MCPConfigPath = string(body), ""
		assertSpecific = func() {
			specs, err := mcp.LoadMCPJSONDocument(body, config.DataDir)
			if err != nil {
				t.Fatal(err)
			}
			ordinary := false
			for _, spec := range specs {
				if spec.ID == "analytix_funds" {
					t.Fatal("disabled professional endpoint escaped the real MCP configuration filter")
				}
				ordinary = ordinary || spec.ID == "gui_schedule"
			}
			if !ordinary || transportCalls.Load() != 0 {
				t.Fatalf("disabled endpoint affected ordinary MCP or executed: ordinary=%t forbidden_calls=%d", ordinary, transportCalls.Load())
			}
		}
		fallthrough
	case "incompatible", "unauthorized":
		materialized := newBundledFundsHostValidationFixtureAtRuntimeHomeV1(t, t.TempDir(), "0.16.16", domainplugin.EntrypointRelativePathV1,
			func(declaration *domainpluginpackage.DeclarationV1) {
				if fault == "disabled" {
					requests := declaration.RequestedCapabilities[:0]
					for _, request := range declaration.RequestedCapabilities {
						if request.ID != "funds.source.read" {
							requests = append(requests, request)
						}
					}
					declaration.RequestedCapabilities = requests
					return
				}
				if fault == "incompatible" {
					declaration.Lifecycle.ProtocolVersion = 2
					return
				}
				declaration.RequestedCapabilities = append(declaration.RequestedCapabilities,
					domainpluginpackage.CapabilityRequestV1{ID: "funds.admin", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound"}})
			})
		dependencies = materialized.dependencies
		// Materialized synthetic package storage stays on the cache volume. Only
		// enrolled configuration and its helper-owned DataDir use the approved host root.
		dependencies.resolveRuntimeRoots = func(string) (string, string, error) {
			return bundledFundsRuntimeRootsV1(materialized.config.DataDir)
		}
		dependencies.observeHostSourceRead = func(snapshot func() []domainplugincapability.FundsSourceReadDecisionEventV1) {
			hostSourceReadObservers = append(hostSourceReadObservers, snapshot)
		}
		retainedRoots = []string{materialized.activeRoot, materialized.inspection.PluginSourceRoot}
		dependencies.observeStaticAdmission = func(input domainpluginpackage.StaticAdmissionInputV1, err error) {
			admissionCalls++
			if (fault == "disabled" && err != nil) || (fault != "disabled" && !errors.Is(err, domainpluginpackage.ErrStaticAdmissionDeniedV1)) || !input.Evidence.ArtifactIntegrityVerified {
				t.Fatalf("signed generation did not reach real static admission denial: %v", err)
			}
			declaration, parseErr := domainpluginpackage.ParseDeclarationV1(input.CanonicalDeclaration)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if fault == "incompatible" && declaration.Lifecycle.ProtocolVersion != 2 {
				t.Fatal("static admission did not receive incompatible lifecycle protocol")
			}
			if fault == "unauthorized" {
				found := false
				for _, request := range declaration.RequestedCapabilities {
					found = found || request.ID == "funds.admin"
				}
				if !found {
					t.Fatal("static admission did not receive unauthorized capability")
				}
			}
		}
	case "domain-semantic":
		// Initialize the real authenticated CAS inventory before adding the
		// domain-invalid record. This activation is fixture setup, not lifecycle evidence.
		initial, err := NewRuntimeServerHandlerE(config)
		if err != nil {
			t.Fatal(err)
		}
		shutdownOwnedRuntimeHandler(t, initial)
		access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		ownerRoot := filepath.Join(config.DataDir, "private", "case-entity")
		cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(ownerRoot, "bindings-v1"), 16<<10, access)
		if err != nil {
			t.Fatal(err)
		}
		address := domainsecurity.SHA256Hex([]byte("optional-domain-semantics-case-entity"))
		if err := errors.Join(cas.PutIfAbsent(context.Background(), address, []byte(`{"domainCanary":"`+runtimeOptionalDomainPrivateCanary+`"}`)), cas.Close()); err != nil {
			t.Fatal(err)
		}
		prepared, err := caseentitystore.PrepareRecoveryV1(context.Background(), ownerRoot, access)
		if err != nil {
			t.Fatal(err)
		}
		var domainErr *finalauthority.DomainRecordUnavailableError
		if err := prepared.ValidateSemantics(context.Background()); !errors.As(err, &domainErr) {
			t.Fatalf("authenticated case-entity fault lost its domain semantic owner: %v", err)
		}
		retainedRoots = []string{ownerRoot}
	default:
		t.Fatalf("unknown optional plugin lifecycle fault %q", fault)
	}
	if len(retainedRoots) != 0 {
		retainedDigest = startupWholeTreeDigest(t, retainedRoots...)
	}
	inspectPackage := dependencies.inspectPackage
	dependencies.inspectPackage = func(ctx context.Context) (packagedauthorityfs.InspectionV2, error) {
		inspectionCalls++
		return inspectPackage(ctx)
	}
	newHostSpec := dependencies.newHostSpec
	dependencies.newHostSpec = func(spec mcp.ServerSpec, input domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error) {
		hostSpecCalls++
		return newHostSpec(spec, input)
	}
	return runtimeOptionalPluginLifecycleFixtureV1{
		config: config,
		async:  async,
		start: func() (http.Handler, error) {
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				return nil, err
			}
			ctx := context.WithValue(context.Background(), bundledFundsHostValidationContextKeyV1{}, dependencies)
			ctx = context.WithValue(ctx, asyncTurnObservationContextKeyV1{}, async.observe)
			handler, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				return nil, errors.Join(err, lease.Close())
			}
			starts++
			return &ownedPersistenceLeaseHandler{Handler: handler, lease: lease}, nil
		},
		assertFault: func(startups int) {
			wantHostSpecs := 0
			if fault == "disabled" {
				wantHostSpecs = startups
			}
			if starts != startups || hostSpecCalls != wantHostSpecs {
				t.Fatalf("optional fault activation mismatch: starts=%d want=%d host_specs=%d inspections=%d admissions=%d", starts, startups, hostSpecCalls, inspectionCalls, admissionCalls)
			}
			wantInspections, wantAdmissions := startups, 0
			if fault == "domain-semantic" {
				wantInspections = 0
			}
			if fault == "disabled" || fault == "incompatible" || fault == "unauthorized" {
				wantAdmissions = startups
			}
			if inspectionCalls != wantInspections || admissionCalls != wantAdmissions {
				t.Fatalf("fault did not reach its production owner: inspection=%d want=%d admission=%d want=%d", inspectionCalls, wantInspections, admissionCalls, wantAdmissions)
			}
			if len(retainedRoots) != 0 && startupWholeTreeDigest(t, retainedRoots...) != retainedDigest {
				t.Fatal("optional fault owner bytes changed during ordinary work or restart")
			}
			if fault == "disabled" {
				if len(hostSourceReadObservers) != startups {
					t.Fatal("missing production Host lifecycle observer")
				}
				for _, snapshot := range hostSourceReadObservers {
					events := snapshot()
					if len(events) == 0 {
						t.Fatal("Host source-read lifecycle emitted no decision")
					}
					event := events[len(events)-1]
					if event.State != domainplugincapability.FundsSourceReadStateDisabledV1 || event.Health != domainplugincapability.FundsSourceReadHealthDisabledV1 || event.Decision != domainplugincapability.FundsSourceReadDecisionDeniedV1 || event.ReasonCode != domainplugincapability.FundsSourceReadReasonRequestMissingV1 || event.GrantDigest != "" || event.AuthorityDigest != "" {
						t.Fatalf("Host source-read capability was not disabled by its request owner: state=%s health=%s decision=%s reason=%s", event.State, event.Health, event.Decision, event.ReasonCode)
					}
				}
			}
			assertSpecific()
		},
	}
}
