package server

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	readapp "analytix.local/runtime-go/internal/app/workspaceread"
)

func TestWorkspaceReadUsesCurrentCoreAuthorityWithoutHistory(t *testing.T) {
	p, before, scope := newSelectionProjectorFixture(t)
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal(err)
	}
	p.handler.turnSecurity.RiskAuthority = risk
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(scope.Workspace, "reference.md"), []byte("SYNTHETIC_READ_CANARY"), 0600); err != nil {
		t.Fatal(err)
	}
	service := &readapp.Service{Files: filestore.NewWorkspaceReadFiles(nil)}
	authority := runtimeWorkspaceReadAuthority{p.handler}
	first, err := authority.Current(ctx, scope.ThreadID)
	if err != nil {
		t.Fatalf("initial authority: %v", err)
	}
	second, err := authority.Current(ctx, scope.ThreadID)
	if err != nil || second != first {
		t.Fatalf("unstable authority: %v first=%+v second=%+v", err, first, second)
	}
	if _, err := service.Files.InspectRoot(ctx, first.Workspace); err != nil {
		t.Fatalf("root: %v", err)
	}
	if err := service.BindAuthority(runtimeWorkspaceReadAuthority{p.handler}); err != nil {
		t.Fatal(err)
	}
	authorized, err := service.Read(ctx, readapp.Request{Action: "authorize", ThreadID: scope.ThreadID})
	if err != nil {
		t.Fatal(err)
	}
	request := readapp.Request{Action: "scan", ThreadID: scope.ThreadID, Binding: authorized.Binding}
	snapshot, err := service.Read(ctx, request)
	if err != nil || len(snapshot.Files) != 1 || string(snapshot.Files[0].Content) != "SYNTHETIC_READ_CANARY" {
		t.Fatalf("authorized snapshot missing: %v", err)
	}
	after, err := p.handler.store.GetThread(scope.ThreadID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("read changed thread/history")
	}
	other, err := p.handler.store.CreateThread(map[string]any{"workspace": scope.Workspace}, scope.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	request.ThreadID = stringField(other, "id")
	if value, err := service.Read(ctx, request); err == nil || len(value.Files) != 0 {
		t.Fatal("binding crossed threads")
	}
	request.ThreadID = scope.ThreadID
	p.handler.turnSecurity.Identity = nil
	if value, err := service.Read(ctx, request); err == nil || len(value.Files) != 0 {
		t.Fatal("revoked authority returned bytes")
	}
}
