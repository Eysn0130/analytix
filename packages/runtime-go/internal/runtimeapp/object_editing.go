package runtimeapp

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

// Receipt metadata belongs to the managed data inventory, not the encrypted
// original-content CAS. It contains no document bytes or original path. The
// qualified runtime data root is established before this additive capability.
func newObjectEditingHandler(config Config, identity identityport.Authority, protectedRoots []string) http.Handler {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return httpapi.ObjectEditingHandler{UnsupportedPlatform: true}
	}
	if !filepath.IsAbs(config.DataDir) || identity == nil {
		return nil
	}
	root := filepath.Join(config.DataDir, "object-editing")
	if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil
	}
	files, err := filestore.NewObjectEditingFiles(root, protectedRoots)
	if err != nil {
		return nil
	}
	return httpapi.ObjectEditingHandler{Service: editingapp.New(identity, files)}
}
