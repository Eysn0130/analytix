package pluginmaterialization

import (
	"errors"
	"time"
)

const ReadyPurposeV1 = "analytix.bundled-funds-materialization-ready/v1"

type PackageAuthorityBindingV1 struct {
	FileSHA256      string `json:"fileSha256"`
	AuthorityDigest string `json:"authorityDigest"`
	Classification  string `json:"classification"`
	DispositionKind string `json:"dispositionKind"`
	PlatformAnchor  string `json:"platformAnchor"`
}

type RuntimeIdentityV1 struct {
	PayloadSHA256 string `json:"payloadSha256"`
	PayloadBytes  int64  `json:"payloadBytes"`
	Format        string `json:"format"`
	Arch          string `json:"arch"`
}

type ReadyV1 struct {
	SchemaVersion              int                       `json:"schemaVersion"`
	Purpose                    string                    `json:"purpose"`
	InvocationID               string                    `json:"invocationId"`
	ConfigurationBindingDigest string                    `json:"configurationBindingDigest"`
	PackageAuthority           PackageAuthorityBindingV1 `json:"packageAuthority"`
	RuntimeIdentity            RuntimeIdentityV1         `json:"runtimeIdentity"`
	PluginName                 string                    `json:"pluginName"`
	PluginVersion              string                    `json:"pluginVersion"`
	ActivePluginRoot           string                    `json:"activePluginRoot"`
	Receipt                    ReceiptV1                 `json:"receipt"`
	Index                      IndexV1                   `json:"index"`
	Publishable                bool                      `json:"publishable"`
	FactToolsEnabled           bool                      `json:"factToolsEnabled"`
	CompletedAt                string                    `json:"completedAt"`
}

type ReadyInputV1 struct {
	InvocationID     string
	PackageAuthority PackageAuthorityBindingV1
	RuntimeIdentity  RuntimeIdentityV1
	ActivePluginRoot string
	Receipt          ReceiptV1
	Index            IndexV1
	CompletedAt      time.Time
}

func NewReadyV1(input ReadyInputV1) (ReadyV1, error) {
	ready := ReadyV1{
		SchemaVersion: SchemaVersionV1, Purpose: ReadyPurposeV1,
		InvocationID: input.InvocationID, PackageAuthority: input.PackageAuthority,
		RuntimeIdentity: input.RuntimeIdentity, PluginName: input.Receipt.PluginName, PluginVersion: input.Receipt.PluginVersion,
		ActivePluginRoot: input.ActivePluginRoot, Receipt: input.Receipt, Index: input.Index,
		Publishable: false, FactToolsEnabled: false,
		CompletedAt: input.CompletedAt.UTC().Format(time.RFC3339Nano),
	}
	ready.ConfigurationBindingDigest = deriveReadyBindingDigestV1(ready)
	if err := ValidateReadyV1(ready); err != nil {
		return ReadyV1{}, err
	}
	return ready, nil
}

func ValidateReadyV1(ready ReadyV1) error {
	if ready.SchemaVersion != SchemaVersionV1 || ready.Purpose != ReadyPurposeV1 ||
		!canonicalDigest(ready.InvocationID) || !canonicalDigest(ready.ConfigurationBindingDigest) ||
		ready.ConfigurationBindingDigest != deriveReadyBindingDigestV1(ready) ||
		!validPackageAuthorityBindingV1(ready.PackageAuthority) || !validRuntimeIdentityV1(ready.RuntimeIdentity) ||
		!validPluginIdentityV1(ready.PluginName, ready.PluginVersion) ||
		!canonicalAbsolutePath(ready.ActivePluginRoot) || ValidateReceiptV1(ready.Receipt) != nil ||
		ValidateIndexForReceiptV1(ready.Index, ready.Receipt) != nil ||
		ready.Receipt.PackageAuthoritySHA256 != ready.PackageAuthority.FileSHA256 ||
		ready.Receipt.PluginName != ready.PluginName || ready.Receipt.PluginVersion != ready.PluginVersion ||
		ready.Publishable || ready.FactToolsEnabled || ready.Receipt.FactToolsEnabled || ready.Index.FactToolsEnabled ||
		!canonicalTime(ready.CompletedAt) {
		return errors.New("bundled funds materialization ready result is invalid")
	}
	return nil
}

func ReadyV1Bytes(ready ReadyV1) ([]byte, error) {
	return canonicalBytes(ready, func() error { return ValidateReadyV1(ready) })
}

func ParseReadyV1(body []byte) (ReadyV1, error) {
	var ready ReadyV1
	err := strictParse(body, &ready, func() error { return ValidateReadyV1(ready) })
	return ready, err
}

func deriveReadyBindingDigestV1(ready ReadyV1) string {
	ready.ConfigurationBindingDigest = ""
	return digestWithDomain("analytix.bundled-funds-materialization-ready/binding/v1", ready)
}

func validPackageAuthorityBindingV1(binding PackageAuthorityBindingV1) bool {
	if !canonicalDigest(binding.FileSHA256) || !canonicalDigest(binding.AuthorityDigest) {
		return false
	}
	switch binding.Classification {
	case "development_clean_non_publishable", "development_dirty_non_publishable",
		"controlled_release_clean_candidate_non_publishable", "controlled_release_dirty_non_publishable":
	default:
		return false
	}
	if binding.DispositionKind != "development_non_publishable" && binding.DispositionKind != "controlled_release_receipt" {
		return false
	}
	return binding.PlatformAnchor == "macos_developer_id_resource_seal" || binding.PlatformAnchor == "macos_nonpublishable_resource_seal"
}

func validRuntimeIdentityV1(identity RuntimeIdentityV1) bool {
	return canonicalDigest(identity.PayloadSHA256) && identity.PayloadBytes > 0 &&
		(identity.Format == "mach-o" || identity.Format == "pe" || identity.Format == "elf") &&
		(identity.Arch == "arm64" || identity.Arch == "x64")
}
