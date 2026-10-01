package server

import (
	"context"
	"testing"

	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	packagehostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	"analytix.local/runtime-go/internal/app/workspacemutation"
	filesport "analytix.local/runtime-go/internal/ports/managedediting"
)

type catalogFileIdentity struct{}

func (catalogFileIdentity) Regular() bool    { return true }
func (catalogFileIdentity) SingleLink() bool { return true }

type catalogFiles struct{}

func (catalogFiles) Inspect(string, bool) (filesport.Identity, []filesport.Identity, error) {
	return catalogFileIdentity{}, nil, nil
}
func (catalogFiles) Contains(parent, path string) bool     { return parent == path }
func (catalogFiles) SameFile(a, b filesport.Identity) bool { return a == b }

func TestNativeToolCatalogFollowsCoreCaptureLifetime(t *testing.T) {
	h := &runtimeServerHandler{officePackageHost: &packagehostapp.Service{},
		managedEditing: managededitingapp.New(workspacemutation.NewCoordinator(), catalogFiles{})}
	check := func(want bool) {
		t.Helper()
		tools := h.runtimeToolSchemasForPrompt(true, nil, false, false, "Update the selected document text")
		for _, name := range []string{"native_selection_read", "native_selection_propose"} {
			found := false
			for _, tool := range tools {
				found = found || tool.Name == name
			}
			if found != want {
				t.Fatalf("%s availability = %v, want %v", name, found, want)
			}
		}
	}
	check(false)
	release, err := h.managedEditing.Capture(context.Background(), "test-session", "/synthetic.docx")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	check(true)
	h.officePackageHost = nil
	check(false)
	h.officePackageHost = &packagehostapp.Service{}
	release()
	check(false)
}
