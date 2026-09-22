//go:build darwin

package packagedbuildauthorityfs

import (
	"context"
	"os"
	"testing"
)

// This opt-in check reads a real normal-lifecycle package. It never starts
// Electron, accesses credentials, changes resources or grants release status.
func TestConfiguredCorePackageClosure(t *testing.T) {
	runtimePath := os.Getenv("ANALYTIX_CORE_PACKAGE_RUNTIME_SERVER")
	if runtimePath == "" {
		t.Skip("exact Core package input not configured")
	}
	if !CoreOnlyBuild() {
		t.Fatal("Core inspection requires the compiled Core profile")
	}
	expected := os.Getenv("ANALYTIX_CORE_PACKAGE_SOURCE_COMMIT")
	if len(expected) != 40 {
		t.Fatal("exact Core source commit not configured")
	}
	inspection, err := InspectPackageV2(context.Background(), runtimePath, "darwin", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	authority := inspection.Authority
	if authority.Core == nil || authority.Controlled != nil || authority.Development != nil ||
		authority.Authority.SourceCommit != expected || authority.Authority.WorktreeSnapshot.Dirty ||
		authority.Authority.Classification != "development_clean_non_publishable" ||
		inspection.Publishable || inspection.FactToolsEnabled || inspection.PackageAnchor != "macos_nonpublishable_resource_seal" {
		t.Fatal("Core package does not match its exact private candidate contract")
	}
	if err := verifyCoreResourcesAbsentV2(inspection.ResourcesRoot); err != nil {
		t.Fatal(err)
	}
	t.Log("exact clean Core package: sealed runner, runtime, app.asar and professional-resource absence verified")
}
