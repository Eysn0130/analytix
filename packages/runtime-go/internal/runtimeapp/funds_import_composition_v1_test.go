package runtimeapp

import (
	"context"
	"os"
	"strings"
	"testing"

	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
)

func TestLocalNonPublishablePackageInspectionWhenExplicitlyProvided(t *testing.T) {
	runtimeServer := os.Getenv("ANALYTIX_AB_R2_PACKAGE_RUNTIME_SERVER")
	if runtimeServer == "" {
		t.Skip("set ANALYTIX_AB_R2_PACKAGE_RUNTIME_SERVER to inspect an exact packaged runtime")
	}
	inspection, err := packagedauthorityfs.InspectPackageV2(context.Background(), runtimeServer, "darwin", "arm64")
	if err != nil {
		t.Fatalf("local non-publishable package inspection failed: %v", err)
	}
	if inspection.Publishable || inspection.FactToolsEnabled {
		t.Fatalf("local package minted release authority: publishable=%t factToolsEnabled=%t", inspection.Publishable, inspection.FactToolsEnabled)
	}
	dataDir := os.Getenv("ANALYTIX_AB_R2_PACKAGE_DATA_DIR")
	if dataDir == "" {
		t.Skip("set ANALYTIX_AB_R2_PACKAGE_DATA_DIR to inspect exact local Funds admission")
	}
	dependencies := defaultBundledFundsHostValidationDependenciesV1()
	dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
		return inspection, nil
	}
	host, err := validateBundledFundsHostMaterializationWithDependenciesV1(
		context.Background(),
		Config{DataDir: dataDir},
		dependencies,
	)
	if err != nil {
		t.Fatalf("local non-publishable Funds admission failed: %v", err)
	}
	if host == nil {
		t.Fatal("local non-publishable Funds admission returned no Host-owned spec")
	}
}

// This guard is intentionally structural: the runtime startup integration
// tests exercise New in production composition, while this test fixes the
// narrow Slice 5 dependency boundary that must remain visible in review.
func TestFundsImportAndCleaningProductionCompositionReuseOneDSV2DataPlane(t *testing.T) {
	body, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"ReadImportSource: fundscsvsourceadapter.ReadImportExactV1",
		"localdisplayapp.ImportMappingPreviewDependenciesV1{",
		"Identity: identityAuthority, Reader: fundsCSVAdmission",
		"fundscleaningapp.NewServiceV1",
		"Native: nativeOwner, Admission: fundsCSVAdmission",
		"Identity: identityAuthority, Reader: fundsCleaning",
		"if strings.TrimSpace(config.UserDataDir) == \"\" {",
		"openErr = nativecomponenthost.ErrUnavailable",
		"nativecomponenthost.OpenProduction(config.UserDataDir)",
		"fundsquerysourceadapter.NewHostExactSource(config.UserDataDir)",
		"var hostFundsNativeOwnerCurrent func(context.Context) error",
		"hostFundsNativeOwnerCurrent = nativeOwner.ValidateCurrentDataEngine",
		"HostFundsNativeOwnerCurrent: hostFundsNativeOwnerCurrent",
		"authorityDataDir := config.DataDir",
		"filepath.Join(config.DataDir, \"private\", \"dataset-snapshot-authority\")",
		"nativeAdmissionMayDisableCaseCapability(openErr)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("production import composition lost %q", required)
		}
	}
	for _, forbidden := range []string{
		"cleaning.clean_all",
		"CleaningDiffPreviewDependenciesV1{}",
		"sharedEvidenceDatasetSnapshotV2.snapshot != nil && sharedEvidenceDatasetSnapshotV2.registry != nil",
		"nativecomponenthost.OpenProduction(config.DataDir)",
		"HostFundsNativeOwnerCurrent: func(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("production cleaning composition retained forbidden path %q", forbidden)
		}
	}
	for _, required := range []string{
		"if sharedEvidenceDatasetSnapshotV2.snapshot != nil {",
		"sharedEvidenceConfiguredV2 && sharedEvidenceDatasetSnapshotV2.registry == nil",
		"evidenceStore = runtimeUnavailableEvidenceRegistryV1{}",
		"if nativeOwner != nil && sharedEvidenceDatasetSnapshotV2.evidence != nil &&\n\t\tsharedEvidenceDatasetSnapshotV2.snapshot != nil {",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("cold-registry Funds composition lost %q", required)
		}
	}
}

func TestNativeHealthProductionCompositionUsesMetadataCurrentnessOnly(t *testing.T) {
	body, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	start := strings.Index(source, "HealthOnlyAuthority: nativecomponentapp.NewHealthOnlyCurrentnessV1(")
	if start < 0 {
		t.Fatal("production native health composition lost its health-only currentness owner")
	}
	endOffset := strings.Index(source[start:], "\n\t\t\t\tOwner: nativeOwner,")
	if endOffset < 0 {
		t.Fatal("production native health composition is not bounded before the native owner")
	}
	healthBlock := source[start : start+endOffset]
	for _, required := range []string{
		"turnsecurityapp.ValidateCurrentOrdinaryEffect(",
		"Identity:         identityAuthority",
		"Observer:         filestore.CaseBindingReader{}",
		"RiskAuthority:    threadRiskAuthority",
		"Workspace:        securityContext.WorkspaceRealPath",
		"nativeOwner.ValidateCurrentDataEngine",
	} {
		if !strings.Contains(healthBlock, required) {
			t.Fatalf("production health-only currentness lost %q", required)
		}
	}
	for _, forbidden := range []string{
		"turnsecurityapp.ValidateCurrent(",
		"turnsecurityapp.ValidateCurrentInsideExactDatasetCapability(",
		"SnapshotAuthority:",
		"SnapshotAuthorityV2:",
	} {
		if strings.Contains(healthBlock, forbidden) {
			t.Fatalf("production health-only currentness acquired forbidden DSV2 owner %q", forbidden)
		}
	}

	generalBlock := source[:start]
	if !strings.Contains(generalBlock, "LiveAuthority: nativecomponentapp.LiveAuthorityFunc(func(") ||
		!strings.Contains(generalBlock, "turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{") ||
		!strings.Contains(generalBlock, "SnapshotAuthorityV2: datasetSnapshotAuthorityV2") {
		t.Fatal("general native LiveAuthority no longer retains full DSV2 currentness")
	}
}
