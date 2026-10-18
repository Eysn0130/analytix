package runtimeapp

import (
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentsStorageDoesNotRequireLibreOfficeAssets(t *testing.T) {
	ctx := context.Background()
	principal, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{DataDir: t.TempDir(), DevelopmentPluginSourceRoot: root}
	adapters := newOfficeEditingAdapters(ctx, config, editingTestIdentity{principal}, nil)
	documents := adapters["analytix-documents"]
	if documents == nil {
		t.Fatal("DOCX storage was gated on missing LibreOffice assets")
	}
	if adapters["analytix-spreadsheets"] != nil || adapters["analytix-presentations"] != nil {
		t.Fatal("native Office became available without its assets")
	}
	ready, err := documents.Readiness(ctx, adapterport.Binding{PackageID: "analytix-documents"})
	if err != nil || !ready.Available {
		t.Fatalf("storage not ready: %v", err)
	}
	for _, op := range ready.Operations {
		if op == "create-object" {
			t.Fatal("create exposed without codec/projector")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	ready, _ = documents.Readiness(cancelled, adapterport.Binding{PackageID: "analytix-documents"})
	if ready.Available {
		t.Fatal("cancelled authority accepted")
	}
}
