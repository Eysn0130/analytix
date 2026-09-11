package nativecomponentregistry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	FrozenManifestSHA256V4 = "7265c3508f16731b5f8dbb1e08f7d645a2e8ab6d7bf4a35cf64c887e0c129aa4"
	MaxManifestBytesV4     = 256 * 1024
)

type FrozenManifestV4 struct {
	SchemaVersion        int                 `json:"schemaVersion"`
	ReceiptSchemaVersion int                 `json:"receiptSchemaVersion"`
	Components           []FrozenComponentV4 `json:"components"`
}

type FrozenComponentV4 struct {
	ID               string                 `json:"id"`
	SourceRoot       string                 `json:"sourceRoot"`
	CargoManifest    string                 `json:"cargoManifest"`
	BinaryName       string                 `json:"binaryName"`
	Role             string                 `json:"role"`
	AgentCore        bool                   `json:"agentCore"`
	Consumers        []string               `json:"consumers"`
	PackagePath      string                 `json:"packagePath"`
	SupportedTargets []string               `json:"supportedTargets"`
	ExecutionProbe   FrozenExecutionProbeV4 `json:"executionProbe"`
	Authorization    string                 `json:"authorization"`
}

type FrozenExecutionProbeV4 struct {
	SchemaVersion     int      `json:"schemaVersion"`
	AuthorityProtocol string   `json:"authorityProtocol"`
	ComponentProtocol string   `json:"componentProtocol"`
	PolicySHA256      string   `json:"policySha256"`
	AuthorityTargets  []string `json:"authorityTargets"`
}

type frozenManifestIdentityV4 struct {
	id, sourceRoot, cargoManifest, binaryName, role, packagePath, policySHA256, authorization string
	consumerCount                                                                             int
}

var frozenManifestIdentitiesV4 = [...]frozenManifestIdentityV4{
	{"import-accelerator", "tools/import_accelerator", "tools/import_accelerator/Cargo.toml", "analytix-import-accelerator", "immutable-data-import", "runtime/analytix-import-accelerator", "0cc3872164fdadfb687a8642846913bf0f2394cd21c26799a5d483dee1ea8d2a", "existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d", 3},
	{"cleaning-ops", "tools/cleaning_ops", "tools/cleaning_ops/Cargo.toml", "analytix-cleaning-ops", "case-data-cleaning", "runtime/analytix-cleaning-ops", "59c4287d312b4972eb6890883865e3c37d226650caf78f65168363413ac85e35", "existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d", 3},
	{"analysis-compute", "tools/analysis_compute", "tools/analysis_compute/Cargo.toml", "analytix-analysis-compute", "bounded-analysis-compute", "runtime/analytix-analysis-compute", "fd2227234c61de1505e50aa33e1e24c2f58c2545fe4d07e41b8064709c18f4d8", "existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d", 3},
	{"data-engine", "tools/data_engine", "tools/data_engine/Cargo.toml", "analytix-data-engine", "single-owner-case-database", "runtime/analytix-data-engine", "97ce13396fb6f5ba7e9d851ab4abc2f23ac8abdd13acaa3e983a326d6bf4a1e0", "existing-data-plane-boundary@af0ea1967c76c7b771aaff1c0d5606ca4d5b832c", 5},
}

func ParseFrozenManifestV4(body []byte) (FrozenManifestV4, error) {
	if len(body) == 0 || len(body) > MaxManifestBytesV4 || digestManifestV4(body) != FrozenManifestSHA256V4 {
		return FrozenManifestV4{}, ErrReceiptInvalid
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxManifestBytesV4, MaxDepth: 16, MaxTokens: 8192,
		MaxStringBytes: 64 * 1024, MaxNumberBytes: 32, MaxAbsExponent: 8,
	}); err != nil {
		return FrozenManifestV4{}, ErrReceiptInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest FrozenManifestV4
	if err := decoder.Decode(&manifest); err != nil {
		return FrozenManifestV4{}, ErrReceiptInvalid
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return FrozenManifestV4{}, ErrReceiptInvalid
	}
	canonical, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil || !bytes.Equal(append(canonical, '\n'), body) || !validFrozenManifestV4(manifest) {
		return FrozenManifestV4{}, ErrReceiptInvalid
	}
	return manifest, nil
}

func validFrozenManifestV4(manifest FrozenManifestV4) bool {
	if manifest.SchemaVersion != 4 || manifest.ReceiptSchemaVersion != ReceiptSchemaVersion ||
		len(manifest.Components) != len(frozenManifestIdentitiesV4) {
		return false
	}
	expectedTargets := [...]string{"darwin-arm64", "darwin-x64", "linux-x64", "win32-x64"}
	expectedAuthorityTargets := [...]string{"darwin-arm64", "darwin-x64"}
	for index, expected := range frozenManifestIdentitiesV4 {
		component := manifest.Components[index]
		if component.ID != expected.id || component.SourceRoot != expected.sourceRoot || component.CargoManifest != expected.cargoManifest ||
			component.BinaryName != expected.binaryName || component.Role != expected.role || component.AgentCore ||
			len(component.Consumers) != expected.consumerCount || component.PackagePath != expected.packagePath ||
			component.Authorization != expected.authorization || len(component.SupportedTargets) != len(expectedTargets) ||
			component.ExecutionProbe.SchemaVersion != 1 || component.ExecutionProbe.AuthorityProtocol != "analytix-native-build-probe-v1" ||
			component.ExecutionProbe.ComponentProtocol != "analytix-native-v1" || component.ExecutionProbe.PolicySHA256 != expected.policySHA256 ||
			len(component.ExecutionProbe.AuthorityTargets) != len(expectedAuthorityTargets) {
			return false
		}
		for targetIndex := range expectedTargets {
			if component.SupportedTargets[targetIndex] != expectedTargets[targetIndex] {
				return false
			}
		}
		for targetIndex := range expectedAuthorityTargets {
			if component.ExecutionProbe.AuthorityTargets[targetIndex] != expectedAuthorityTargets[targetIndex] {
				return false
			}
		}
	}
	return true
}

func digestManifestV4(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
