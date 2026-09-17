package pluginmaterialization

import (
	"encoding/json"
	"errors"
	"time"
)

type IntentV1 struct {
	SchemaVersion            int      `json:"schemaVersion"`
	Purpose                  string   `json:"purpose"`
	IntentID                 string   `json:"intentId"`
	PackageAuthoritySHA256   string   `json:"packageAuthoritySha256,omitempty"`
	Target                   TargetV1 `json:"target"`
	PluginName               string   `json:"pluginName"`
	PluginVersion            string   `json:"pluginVersion"`
	SourceRoot               string   `json:"sourceRoot"`
	SourceTreeSHA256         string   `json:"sourceTreeSha256"`
	SourceTreeFileCount      uint64   `json:"sourceTreeFileCount"`
	ManifestSHA256           string   `json:"manifestSha256"`
	EntrypointSHA256         string   `json:"entrypointSha256"`
	RequestedAt              string   `json:"requestedAt"`
	Origin                   string   `json:"origin,omitempty"`
	SourceRegistrationSHA256 string   `json:"sourceRegistrationSha256,omitempty"`
}

type IntentInputV1 struct {
	Origin                   string
	SourceRegistrationSHA256 string
	PackageAuthoritySHA256   string
	Target                   TargetV1
	PluginName               string
	PluginVersion            string
	SourceRoot               string
	SourceTreeSHA256         string
	SourceTreeFileCount      uint64
	ManifestSHA256           string
	EntrypointSHA256         string
	RequestedAt              time.Time
}

func NewIntentV1(input IntentInputV1) (IntentV1, error) {
	intent := IntentV1{
		SchemaVersion: SchemaVersionV1, Purpose: IntentPurposeV1,
		Origin: input.Origin, SourceRegistrationSHA256: input.SourceRegistrationSHA256,
		PackageAuthoritySHA256: input.PackageAuthoritySHA256, Target: input.Target,
		PluginName: input.PluginName, PluginVersion: input.PluginVersion, SourceRoot: input.SourceRoot,
		SourceTreeSHA256: input.SourceTreeSHA256, SourceTreeFileCount: input.SourceTreeFileCount,
		ManifestSHA256: input.ManifestSHA256, EntrypointSHA256: input.EntrypointSHA256,
		RequestedAt: input.RequestedAt.UTC().Format(time.RFC3339Nano),
	}
	intent.IntentID = deriveIntentID(intent)
	if err := ValidateIntentV1(intent); err != nil {
		return IntentV1{}, err
	}
	return intent, nil
}

func ValidateIntentV1(intent IntentV1) error {
	if intent.SchemaVersion != SchemaVersionV1 || intent.Purpose != IntentPurposeV1 ||
		!validPluginIdentityV1(intent.PluginName, intent.PluginVersion) ||
		!validMaterializationOriginV1(intent.Origin, intent.PackageAuthoritySHA256, intent.SourceRegistrationSHA256, intent.PluginName) || ValidateTargetV1(intent.Target) != nil ||
		!canonicalAbsolutePath(intent.SourceRoot) || !canonicalDigest(intent.SourceTreeSHA256) ||
		intent.SourceTreeFileCount == 0 || intent.SourceTreeFileCount > MaxSourceTreeFilesV1 ||
		!canonicalDigest(intent.ManifestSHA256) || !validEntrypointForOriginV1(intent.Origin, intent.EntrypointSHA256) ||
		!canonicalTime(intent.RequestedAt) || !canonicalDigest(intent.IntentID) || intent.IntentID != deriveIntentID(intent) {
		return errors.New("bundled plugin materialization intent is invalid")
	}
	return nil
}

func IntentV1Bytes(intent IntentV1) ([]byte, error) {
	return canonicalBytes(intent, func() error { return ValidateIntentV1(intent) })
}

func ParseIntentV1(body []byte) (IntentV1, error) {
	var intent IntentV1
	err := strictParse(body, &intent, func() error { return ValidateIntentV1(intent) })
	return intent, err
}

func deriveIntentID(intent IntentV1) string {
	intent.IntentID = ""
	return digestWithDomain("analytix.bundled-plugin-materialization-intent/id/v1", intent)
}

func IntentSHA256V1(intent IntentV1) string {
	body, err := IntentV1Bytes(intent)
	if err != nil {
		return ""
	}
	return digestWithDomain("analytix.bundled-plugin-materialization-intent/body/v1", json.RawMessage(body))
}
