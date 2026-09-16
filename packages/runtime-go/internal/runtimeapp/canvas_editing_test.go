package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	canvasapp "analytix.local/runtime-go/internal/app/canvasediting"
	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	identity "analytix.local/runtime-go/internal/domain/identity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type canvasAssemblyProjector struct{ principal identity.PrincipalV1 }

func (p canvasAssemblyProjector) ValidateCurrent(_ context.Context, in objectapp.ScopeAuthority) error {
	if in.ThreadID != "canvas-test-thread" || !identity.SamePrincipalV1(p.principal, in.Principal) {
		return objectapp.ErrProjection
	}
	return nil
}
func (p canvasAssemblyProjector) AuthorizeAndProject(ctx context.Context, in objectapp.ProjectionInput) ([]objectapp.ProtectedRange, error) {
	return nil, p.ValidateCurrent(ctx, in.ScopeAuthority)
}

func TestCanvasRealHostReviewSaveAndActivationRevocation(t *testing.T) {
	ctx := context.Background()
	repository, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, workspace := filepath.Join(home, "data"), filepath.Join(home, "workspace")
	for _, p := range []string{data, workspace} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(workspace, "chart.canvas")
	before := []byte(`{"schemaVersion":1,"facts":{"nodes":[{"id":"n1","label":"Original fact","attributes":{},"sources":[],"assumption":true}],"edges":[]},"presentation":{"nodes":[{"id":"n1","layout":{"x":0,"y":0,"width":120,"height":60},"displayLabel":"Original view","style":{"fill":"#ffffff","stroke":"#000000","strokeWidth":1,"dash":"solid","shape":"rounded"}}],"edges":[]}}`)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	principal, _ := identity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	config := Config{DataDir: data, DevelopmentPluginSourceRoot: repository}
	adapter := newDevelopmentCanvasAdapter(ctx, config, editingTestIdentity{principal}, []string{data})
	concrete, ok := adapter.(*canvasapp.Adapter)
	if !ok {
		t.Fatal("Canvas adapter absent")
	}
	if err := concrete.BindHost(canvasAssemblyProjector{principal}, func(_ context.Context, _, _ string, validate func() error) (func(), error) {
		return func() {}, validate()
	}); err != nil {
		t.Fatal(err)
	}
	host := composeStaticEditorPackageHost(ctx, config, editingTestIdentity{principal}, map[string]adapterport.Adapter{"analytix-canvas": adapter}, map[string]staticEditorPackageDescriptor{"analytix-canvas": {root: repository}})
	if host == nil {
		t.Fatal("real materialized Host absent")
	}
	views, err := host.List(ctx)
	if err != nil || len(views) != 1 {
		t.Fatal("inventory", err)
	}
	view, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: "analytix-canvas", GenerationID: views[0].GenerationID, DesiredState: domainplugin.DesiredEnabledV1})
	if err != nil || !view.Available {
		t.Fatal("Canvas did not activate", err, view)
	}
	invoke := func(op string, input any) (map[string]json.RawMessage, error) {
		raw, e := json.Marshal(input)
		if e != nil {
			return nil, e
		}
		result, e := host.Invoke(ctx, hostapp.InvokeRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, ContributionID: "workspace-editor", Operation: op, Input: raw})
		if e != nil {
			return nil, e
		}
		var output map[string]json.RawMessage
		e = json.Unmarshal(result.Output, &output)
		return output, e
	}
	if _, err := invoke("open-object", map[string]any{"threadId": "canvas-test-thread", "kind": "canvas", "workspace": workspace, "path": path}); !errors.Is(err, hostapp.ErrInvalid) {
		t.Fatal("Host path selector guard changed", err)
	}
	out, err := invoke("open-object", map[string]any{"threadId": "canvas-test-thread", "kind": "canvas", "object": map[string]any{"workspace": workspace, "path": path}})
	var document canvasapp.Document
	if err != nil || string(out["ok"]) != "true" || json.Unmarshal(out["document"], &document) != nil || document.SessionID == "" {
		t.Fatal("real Host open failed", err)
	}
	out, err = invoke("propose-scene", map[string]any{"sessionId": document.SessionID, "threadId": document.ThreadID, "baseRevision": document.Revision, "selectedIds": []string{"n1"}, "operations": []any{map[string]any{"kind": "set-display-label", "id": "n1", "target": "node", "displayLabel": "Reviewed display"}}})
	var proposal canvasapp.Proposal
	if err != nil || string(out["ok"]) != "true" || json.Unmarshal(out["proposal"], &proposal) != nil || proposal.ID == "" {
		t.Fatal("real Host propose failed", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(before) {
		t.Fatal("proposal mutated original", err)
	}
	out, err = invoke("proposal-apply", map[string]any{"sessionId": document.SessionID, "threadId": document.ThreadID, "proposalId": proposal.ID})
	if err != nil || string(out["ok"]) != "true" {
		t.Fatal("real Host apply failed", err)
	}
	var receipt map[string]any
	if json.Unmarshal(out["receipt"], &receipt) != nil || receipt["status"] != "committed" || receipt["operationId"] == "" || len(receipt) != 4 {
		t.Fatal("public receipt shape", receipt)
	}
	actual, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(actual), "Original fact") || !strings.Contains(string(actual), "Reviewed display") {
		t.Fatal("saved facts/display", err)
	}
	_, err = host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, DesiredState: domainplugin.DesiredDisabledV1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = invoke("read-object", map[string]any{"sessionId": document.SessionID, "threadId": document.ThreadID}); err == nil {
		t.Fatal("disabled generation remained executable")
	}
}
