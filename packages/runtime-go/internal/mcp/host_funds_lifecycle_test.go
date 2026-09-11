package mcp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

func TestProductionManagerHostFundsSourceReadLifecycleStopsAndFreshlyRecovers(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	input := hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion)
	host, err := NewHostFundsServerSpecV1(spec, input)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host,
	})
	if _, ok := manager.hostFundsSourceReadLifecycle.CurrentGrant(); ok || len(manager.Tools()) != 0 {
		t.Fatal("declaration, spec, and manager construction minted source-read authority before Host setup")
	}
	manager.Connect()
	lifecycle := manager.hostFundsSourceReadLifecycle
	grant1, ok := lifecycle.CurrentGrant()
	if !ok || !managerHostFundsSourceReadCurrentForTestV1(manager) {
		t.Fatalf("production Connect did not mint source-read authority: events=%#v", manager.HostFundsSourceReadCapabilityEventsV1())
	}
	authority1 := managerHostFundsSourceReadAuthorityForTestV1(manager)
	if !lifecycle.Authorizes(grant1, domainplugincapability.FundsSourceReadOperationV1, authority1) {
		t.Fatal("fresh production authority did not authorize count_case_rows")
	}
	manager.Disconnect()
	if lifecycle.Authorizes(grant1, domainplugincapability.FundsSourceReadOperationV1, authority1) ||
		len(manager.LiveTools()) != 0 {
		t.Fatal("Disconnect retained a source-read grant or live advertisement")
	}

	manager.RestartReconnect()
	grant2, ok := lifecycle.CurrentGrant()
	authority2 := managerHostFundsSourceReadAuthorityForTestV1(manager)
	if !ok || !managerHostFundsSourceReadCurrentForTestV1(manager) ||
		!lifecycle.Authorizes(grant2, domainplugincapability.FundsSourceReadOperationV1, authority2) {
		t.Fatalf("RestartReconnect did not perform fresh full revalidation: events=%#v", manager.HostFundsSourceReadCapabilityEventsV1())
	}
	if grant2.BindingDigest() != grant1.BindingDigest() || grant2.GrantDigest() == grant1.GrantDigest() ||
		grant2.Generation() <= grant1.Generation() || grant2.ConnectionEpoch() <= grant1.ConnectionEpoch() ||
		lifecycle.Authorizes(grant1, domainplugincapability.FundsSourceReadOperationV1, authority2) {
		t.Fatalf("recovery reused stale authority or changed the stable binding: old=%#v new=%#v", grant1, grant2)
	}

	host2, err := NewHostFundsServerSpecV1(spec, input)
	if err != nil {
		t.Fatal(err)
	}
	manager2 := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host2,
	})
	manager2.Connect()
	defer manager2.Disconnect()
	grant3, ok := manager2.hostFundsSourceReadLifecycle.CurrentGrant()
	if !ok || grant3.BindingDigest() != grant1.BindingDigest() ||
		host2.sourceReadBinding.AdmissionDigest() != host.sourceReadBinding.AdmissionDigest() {
		t.Fatal("fresh manager did not deterministically rebuild the admitted binding")
	}
	manager.Disconnect()

	events := manager.HostFundsSourceReadCapabilityEventsV1()
	wantStates := []domainplugincapability.FundsSourceReadStateV1{
		domainplugincapability.FundsSourceReadStateSetupV1,
		domainplugincapability.FundsSourceReadStateReadyV1,
		domainplugincapability.FundsSourceReadStateStoppedV1,
		domainplugincapability.FundsSourceReadStateSetupV1,
		domainplugincapability.FundsSourceReadStateReadyV1,
		domainplugincapability.FundsSourceReadStateStoppedV1,
	}
	gotStates := make([]domainplugincapability.FundsSourceReadStateV1, 0, len(events))
	for _, event := range events {
		gotStates = append(gotStates, event.State)
	}
	if !reflect.DeepEqual(gotStates, wantStates) {
		t.Fatalf("Host-owned lifecycle events are incomplete: got=%#v want=%#v", gotStates, wantStates)
	}
}

func TestProductionManagerFundsOnlyDenialPreservesOrdinaryMCP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var rpc struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&rpc); err != nil {
			t.Errorf("decode ordinary MCP request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch rpc.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(rpc.ID, map[string]any{
				"serverInfo": map[string]any{"name": "ordinary-docs"},
			}))
		case "tools/list":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(rpc.ID, map[string]any{
				"tools": []map[string]any{{
					"name": "lookup", "description": "ordinary lookup",
					"inputSchema": map[string]any{"type": "object", "additionalProperties": false},
				}},
			}))
		case "tools/call":
			_ = json.NewEncoder(w).Encode(jsonRPCResult(rpc.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "ordinary result"}},
			}))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": rpc.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
		}
	}))
	defer server.Close()

	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(
		spec,
		hostFundsStaticAdmissionInputForRequestsFixtureV1(
			t,
			spec.ExpectedServerVersion,
			[]domainpluginpackage.CapabilityRequestV1{{
				ID: "funds.case.read", ProtocolVersion: 1,
				ScopeConstraints: []string{"case:bound", "source:verified"},
			}},
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithOptions([]ServerSpec{{
		ID: "ordinary-docs", Transport: "http", URL: server.URL, TrustScope: "user",
	}}, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host,
	})
	manager.Connect()
	defer manager.Disconnect()
	ordinaryTool := CanonicalToolName("ordinary-docs", "lookup")
	if tools := manager.LiveTools(); !reflect.DeepEqual(tools, []string{ordinaryTool}) {
		t.Fatalf("Funds-only denial blocked ordinary MCP or exposed count: tools=%#v failures=%#v", tools, manager.failures)
	}
	if result := manager.CallTool(ordinaryTool, true, map[string]any{}); result["executed"] != true {
		t.Fatalf("Funds-only denial blocked ordinary MCP execution: %#v", result)
	}
}

func TestProductionManagerHostFundsFatalFailureRevokesCountOnlyUntilFreshReconnect(t *testing.T) {
	manager := newConnectedHostFundsLifecycleManagerV1(t)
	defer manager.Disconnect()
	manager.mu.Lock()
	client := manager.clients["analytix_funds"]
	beforeCalls := manager.calls
	manager.mu.Unlock()
	grant, ok := manager.hostFundsSourceReadLifecycle.CurrentGrant()
	if !ok {
		t.Fatal("fixture did not mint source-read grant")
	}
	authority := managerHostFundsSourceReadAuthorityForTestV1(manager)
	manager.revokeMCPAuthorityAfterNativeFailure("analytix_funds", client, errors.New("private native failure"))
	if tools := manager.Tools(); len(tools) != 0 || len(manager.LiveTools()) != 0 {
		t.Fatalf("revoked count lane remained advertised: %#v", tools)
	}
	result := manager.CallTool(fundsCountEvidenceToolName, true, map[string]any{
		"table_name": "analysis_txn_detail_idx",
	})
	if result["executed"] != false || result["code"] != "mcp_capability_grant_unavailable" ||
		manager.calls != beforeCalls ||
		manager.hostFundsSourceReadLifecycle.Authorizes(
			grant, domainplugincapability.FundsSourceReadOperationV1, authority,
		) {
		t.Fatalf("revoked grant reached the count effect: result=%#v calls=%d", result, manager.calls)
	}
	manager.RestartReconnect()
	if !managerHostFundsSourceReadCurrentForTestV1(manager) ||
		!reflect.DeepEqual(manager.LiveTools(), []string{fundsCountEvidenceToolName}) {
		t.Fatalf("fresh reconnect did not restore only the current count lane: tools=%#v events=%#v",
			manager.LiveTools(), manager.HostFundsSourceReadCapabilityEventsV1())
	}
}

func TestProductionManagerHostFundsCurrentProvenanceDriftRevokesAndRequiresFullRecovery(t *testing.T) {
	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(
		spec,
		hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion),
	)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host,
	})
	manager.Connect()
	defer manager.Disconnect()
	grant, ok := manager.hostFundsSourceReadLifecycle.CurrentGrant()
	authority := managerHostFundsSourceReadAuthorityForTestV1(manager)
	if !ok {
		t.Fatal("fixture did not mint source-read grant")
	}
	original := []byte("export const server = 'analytix_funds'\n")
	if err := os.WriteFile(spec.EntrypointPath, []byte("export const drifted = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if tools := manager.Tools(); len(tools) != 0 ||
		manager.hostFundsSourceReadLifecycle.Authorizes(
			grant, domainplugincapability.FundsSourceReadOperationV1, authority,
		) {
		t.Fatalf("current provenance drift retained count authority: %#v", tools)
	}
	events := manager.HostFundsSourceReadCapabilityEventsV1()
	if last := events[len(events)-1]; last.State != domainplugincapability.FundsSourceReadStateFailedV1 ||
		last.ReasonCode != domainplugincapability.FundsSourceReadReasonProvenanceFailedV1 {
		t.Fatalf("provenance drift did not record a typed value-free failure: %#v", last)
	}
	if err := os.WriteFile(spec.EntrypointPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	manager.RestartReconnect()
	grant2, ok := manager.hostFundsSourceReadLifecycle.CurrentGrant()
	if !ok || !managerHostFundsSourceReadCurrentForTestV1(manager) ||
		grant2.BindingDigest() != grant.BindingDigest() || grant2.GrantDigest() == grant.GrantDigest() {
		t.Fatalf("restored provenance did not require a fresh grant: old=%#v new=%#v", grant, grant2)
	}
}

func newConnectedHostFundsLifecycleManagerV1(t *testing.T) *ProductionManager {
	t.Helper()
	spec := hostFundsServerSpecFixtureV1(t)
	host, err := NewHostFundsServerSpecV1(
		spec,
		hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion),
	)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: unavailableHostFundsDatasetAuthorityV2{}, HostFundsServer: host,
	})
	manager.Connect()
	if !managerHostFundsSourceReadCurrentForTestV1(manager) {
		t.Fatalf("production manager did not start with current source-read authority: %#v", manager.failures)
	}
	return manager
}

func managerHostFundsSourceReadAuthorityForTestV1(
	manager *ProductionManager,
) domainplugincapability.FundsSourceReadHostAuthorityV1 {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return hostFundsSourceReadAuthorityV1(
		manager.hostFundsFingerprint,
		manager.serverIdentities["analytix_funds"],
		manager.sourceCatalogs["analytix_funds"],
		manager.connectionEpochs["analytix_funds"],
	)
}

func managerHostFundsSourceReadCurrentForTestV1(manager *ProductionManager) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.hostFundsSourceReadGrantedNoLock()
}
