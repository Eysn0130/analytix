package runtimeapp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestRuntimeCaseEntitySemanticFailureKeepsGeneralAgentAvailable(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        dataDir,
		DurableTempDir: t.TempDir(),
	}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)

	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dataDir, "private", "case-entity", "bindings-v1")
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		root,
		domaincaseentity.MaxCaseEntityBindingRecordBytesV1,
		access,
	)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("case-entity-semantic-corruption"))
	if err := cas.PutIfAbsent(context.Background(), digest, []byte(`{}`)); err != nil {
		_ = cas.Close()
		t.Fatal(err)
	}
	if err := cas.Close(); err != nil {
		t.Fatal(err)
	}

	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("case-only semantic failure blocked general startup: %v", err)
	}
	health := runtimeStartupJSON(t, handler, http.MethodGet, "/health", nil, http.StatusOK)
	if health["status"] != "ok" {
		t.Fatalf("general runtime health unavailable: %#v", health)
	}
	tools := runtimeStartupJSON(t, handler, http.MethodGet, "/v1/runtime/tools?refresh=1", nil, http.StatusOK)
	contracts, _ := tools["toolContracts"].(map[string]any)
	if count, _ := contracts["count"].(float64); count <= 0 {
		t.Fatalf("case-only failure zeroed general tools: %#v", tools)
	}
	body := strings.ToLower(runtimeStartupJSONBodyV1(t, tools))
	if strings.Contains(body, "analyze_account_flows") {
		t.Fatalf("invalid case state advertised funds analysis: %s", body)
	}
	shutdownOwnedRuntimeHandler(t, handler)
}

func runtimeStartupJSONBodyV1(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
