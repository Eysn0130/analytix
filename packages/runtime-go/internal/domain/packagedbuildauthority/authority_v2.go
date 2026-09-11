package packagedbuildauthority

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	ContractV2                     = "analytix.packaged-build-authority/v2"
	DevelopmentDispositionKindV2   = "development_non_publishable"
	ControlledDispositionKindV2    = "controlled_release_receipt"
	MaxAuthorityBytesV2            = 256 << 10
	maxJavaScriptSafeIntegerV2     = int64(9_007_199_254_740_991)
	authorityDigestDomainV2        = "AnalytixPackagedBuildAuthorityV2\x00"
	worktreeDigestDomainV1         = "AnalytixPackagedWorktreeSnapshotV1\x00"
	worktreeSnapshotContractV1     = "analytix.packaged-worktree-snapshot/v1"
	effectiveBuilderContextV1      = "analytix.electron-builder-effective-context/v1"
	effectiveBuilderTargetDomain   = "AnalytixElectronBuilderEffectiveTargetV1\x00"
	effectiveBuilderContextDomain  = "AnalytixElectronBuilderEffectiveContextV1\x00"
	electronFusePolicyContractV1   = "analytix.electron-fuse-policy/v1"
	electronFusePolicySHA256V1     = "a11a3d69fb77157f56af8fd3332ae08059dd66a967d52facc373c36573b2b62c"
	stagedPayloadContractV1        = "analytix.packaged-staged-payload/v1"
	stagedPayloadExclusionSHA256V1 = "21d491819a899a7c12f4366e5c62858c97def101cc69ba92e11a59049baee5cd"
	worktreeExclusionSHA256V1      = "3f4a6441cb53c9b916c6e3f81eaf7b29a26136ce0700b1b44d3bd5053e1a515b"
)

var developmentComponentsV2 = [...]struct {
	ID         string
	BinaryName string
}{
	{ID: "import-accelerator", BinaryName: "analytix-import-accelerator"},
	{ID: "cleaning-ops", BinaryName: "analytix-cleaning-ops"},
	{ID: "analysis-compute", BinaryName: "analytix-analysis-compute"},
	{ID: "data-engine", BinaryName: "analytix-data-engine"},
}

type ManifestSummaryV1 struct {
	Count      int64  `json:"count"`
	ByteLength int64  `json:"byteLength"`
	SHA256     string `json:"sha256"`
}

type WorktreeSnapshotV1 struct {
	SchemaVersion         int               `json:"schemaVersion"`
	Contract              string            `json:"contract"`
	SourceCommit          string            `json:"sourceCommit"`
	State                 string            `json:"state"`
	Dirty                 bool              `json:"dirty"`
	ExclusionPolicySHA256 string            `json:"exclusionPolicySha256"`
	StagedPatch           ManifestSummaryV1 `json:"stagedPatch"`
	UnstagedPatch         ManifestSummaryV1 `json:"unstagedPatch"`
	UntrackedSource       ManifestSummaryV1 `json:"untrackedSource"`
	SnapshotDigest        string            `json:"snapshotDigest"`
}

type ContentArtifactBindingV2 struct {
	SHA256     string `json:"sha256"`
	ByteLength int64  `json:"byteLength"`
}

type NativeArtifactBindingV2 struct {
	PreSignSHA256     string `json:"preSignSha256"`
	PreSignByteLength int64  `json:"preSignByteLength"`
	PayloadSHA256     string `json:"payloadSha256"`
	PayloadByteLength int64  `json:"payloadByteLength"`
	Format            string `json:"format"`
	Arch              string `json:"arch"`
}

type FundsPluginArtifactBindingV2 struct {
	TreeSHA256       string `json:"treeSha256"`
	FileCount        int64  `json:"fileCount"`
	ManifestSHA256   string `json:"manifestSha256"`
	EntrypointSHA256 string `json:"entrypointSha256"`
	TotalBytes       int64  `json:"totalBytes"`
}

type ArtifactBindingsV2 struct {
	Executable    NativeArtifactBindingV2      `json:"executable"`
	AppASAR       ContentArtifactBindingV2     `json:"appAsar"`
	RuntimeServer NativeArtifactBindingV2      `json:"runtimeServer"`
	FundsPlugin   FundsPluginArtifactBindingV2 `json:"fundsPlugin"`
}

type EffectiveBuilderTargetV1 struct {
	Key      string   `json:"key"`
	Platform string   `json:"platform"`
	Arch     string   `json:"arch"`
	Targets  []string `json:"targets"`
	Digest   string   `json:"digest"`
}

type EffectiveBuilderContextV1 struct {
	SchemaVersion         int                      `json:"schemaVersion"`
	Contract              string                   `json:"contract"`
	EffectiveConfigSHA256 string                   `json:"effectiveConfigSha256"`
	Target                EffectiveBuilderTargetV1 `json:"target"`
	FusePolicyContract    string                   `json:"fusePolicyContract"`
	FusePolicySHA256      string                   `json:"fusePolicySha256"`
	ContextDigest         string                   `json:"contextDigest"`
}

type StagedPayloadClosureV1 struct {
	SchemaVersion         int    `json:"schemaVersion"`
	Contract              string `json:"contract"`
	ExclusionPolicySHA256 string `json:"exclusionPolicySha256"`
	EntryCount            int64  `json:"entryCount"`
	DirectoryCount        int64  `json:"directoryCount"`
	RegularFileCount      int64  `json:"regularFileCount"`
	NativeFileCount       int64  `json:"nativeFileCount"`
	SymlinkCount          int64  `json:"symlinkCount"`
	ContentByteLength     int64  `json:"contentByteLength"`
	ManifestSHA256        string `json:"manifestSha256"`
}

type ControlledReleaseDispositionV2 struct {
	Kind                string `json:"kind"`
	TargetKey           string `json:"targetKey"`
	ReceiptSHA256       string `json:"receiptSha256"`
	ManifestSHA256      string `json:"manifestSha256"`
	SigningPolicySHA256 string `json:"signingPolicySha256"`
	SigningMode         string `json:"signingMode"`
	AppleTeamIdentifier string `json:"appleTeamIdentifier"`
}

type DevelopmentComponentBindingV2 struct {
	ID                     string `json:"id"`
	BinaryName             string `json:"binaryName"`
	MarkerBinarySHA256     string `json:"markerBinarySha256"`
	MarkerBinaryByteLength int64  `json:"markerBinaryByteLength"`
	PayloadSHA256          string `json:"payloadSha256"`
	PayloadByteLength      int64  `json:"payloadByteLength"`
	Format                 string `json:"format"`
	Arch                   string `json:"arch"`
}

type DevelopmentDispositionV2 struct {
	Kind       string                          `json:"kind"`
	TargetKey  string                          `json:"targetKey"`
	Marker     ContentArtifactBindingV2        `json:"marker"`
	Components []DevelopmentComponentBindingV2 `json:"components"`
}

type AuthorityV2 struct {
	SchemaVersion            int                       `json:"schemaVersion"`
	Contract                 string                    `json:"contract"`
	SourceCommit             string                    `json:"sourceCommit"`
	WorktreeSnapshot         WorktreeSnapshotV1        `json:"worktreeSnapshot"`
	Classification           string                    `json:"classification"`
	Publishable              bool                      `json:"publishable"`
	ReleaseEligible          bool                      `json:"releaseEligible"`
	PublicationReceiptIssued bool                      `json:"publicationReceiptIssued"`
	TargetKey                string                    `json:"targetKey"`
	BuildContext             EffectiveBuilderContextV1 `json:"buildContext"`
	NativeDisposition        json.RawMessage           `json:"nativeDisposition"`
	Artifacts                ArtifactBindingsV2        `json:"artifacts"`
	StagedPayload            StagedPayloadClosureV1    `json:"stagedPayload"`
	AuthorityDigest          string                    `json:"authorityDigest"`
}

type ParsedAuthorityV2 struct {
	Authority   AuthorityV2
	Controlled  *ControlledReleaseDispositionV2
	Development *DevelopmentDispositionV2
}

func ParseV2(body []byte) (ParsedAuthorityV2, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxAuthorityBytesV2, MaxDepth: 16,
		MaxTokens: 16_384, MaxStringBytes: 16_384,
	}); err != nil {
		return ParsedAuthorityV2{}, errors.New("packaged build authority JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var authority AuthorityV2
	if err := decoder.Decode(&authority); err != nil {
		return ParsedAuthorityV2{}, errors.New("packaged build authority shape is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ParsedAuthorityV2{}, errors.New("packaged build authority contains trailing JSON")
	}
	canonical, err := json.Marshal(authority)
	if err != nil || !bytes.Equal(body, canonical) {
		return ParsedAuthorityV2{}, errors.New("packaged build authority is not canonical")
	}
	parsed := ParsedAuthorityV2{Authority: authority}
	if err := parsed.validate(); err != nil {
		return ParsedAuthorityV2{}, err
	}
	return parsed, nil
}

func (parsed *ParsedAuthorityV2) validate() error {
	authority := parsed.Authority
	if authority.SchemaVersion != 2 || authority.Contract != ContractV2 ||
		!gitCommit(authority.SourceCommit) || authority.SourceCommit != authority.WorktreeSnapshot.SourceCommit ||
		authority.Publishable || authority.ReleaseEligible || authority.PublicationReceiptIssued ||
		authority.TargetKey == "" || authority.TargetKey != strings.TrimSpace(authority.TargetKey) ||
		validateEffectiveBuilderContextV1(authority.BuildContext, authority.TargetKey) != nil ||
		!validArtifacts(authority.Artifacts) || !sha256Digest(authority.AuthorityDigest) ||
		validateStagedPayloadClosureV1(authority.StagedPayload) != nil ||
		authority.AuthorityDigest != authorityDigest(authority) || validateWorktreeSnapshot(authority.WorktreeSnapshot) != nil {
		return errors.New("packaged build authority is invalid")
	}

	kind, err := dispositionKind(authority.NativeDisposition)
	if err != nil {
		return err
	}
	switch kind {
	case ControlledDispositionKindV2:
		var disposition ControlledReleaseDispositionV2
		if err := strictNested(authority.NativeDisposition, &disposition); err != nil ||
			validateControlled(disposition, authority.TargetKey) != nil {
			return errors.New("packaged controlled release disposition is invalid")
		}
		parsed.Controlled = &disposition
	case DevelopmentDispositionKindV2:
		var disposition DevelopmentDispositionV2
		if err := strictNested(authority.NativeDisposition, &disposition); err != nil ||
			validateDevelopment(disposition, authority.TargetKey) != nil {
			return errors.New("packaged development disposition is invalid")
		}
		parsed.Development = &disposition
	default:
		return errors.New("packaged build authority disposition is unsupported")
	}

	wantClassification := classification(authority.WorktreeSnapshot.Dirty, kind)
	if authority.Classification != wantClassification {
		return errors.New("packaged build authority classification is invalid")
	}
	return nil
}

func (parsed ParsedAuthorityV2) DispositionKind() string {
	if parsed.Controlled != nil {
		return ControlledDispositionKindV2
	}
	if parsed.Development != nil {
		return DevelopmentDispositionKindV2
	}
	return ""
}

func (parsed ParsedAuthorityV2) Target() (platform, arch string, ok bool) {
	switch parsed.Authority.TargetKey {
	case "darwin-arm64":
		return "darwin", "arm64", true
	case "darwin-x64":
		return "darwin", "amd64", true
	case "win32-arm64":
		return "windows", "arm64", true
	case "win32-x64":
		return "windows", "amd64", true
	case "linux-arm64":
		return "linux", "arm64", true
	case "linux-x64":
		return "linux", "amd64", true
	default:
		return "", "", false
	}
}

func AuthorityFileSHA256V2(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func authorityDigest(authority AuthorityV2) string {
	without := struct {
		SchemaVersion            int                       `json:"schemaVersion"`
		Contract                 string                    `json:"contract"`
		SourceCommit             string                    `json:"sourceCommit"`
		WorktreeSnapshot         WorktreeSnapshotV1        `json:"worktreeSnapshot"`
		Classification           string                    `json:"classification"`
		Publishable              bool                      `json:"publishable"`
		ReleaseEligible          bool                      `json:"releaseEligible"`
		PublicationReceiptIssued bool                      `json:"publicationReceiptIssued"`
		TargetKey                string                    `json:"targetKey"`
		BuildContext             EffectiveBuilderContextV1 `json:"buildContext"`
		NativeDisposition        json.RawMessage           `json:"nativeDisposition"`
		Artifacts                ArtifactBindingsV2        `json:"artifacts"`
		StagedPayload            StagedPayloadClosureV1    `json:"stagedPayload"`
	}{
		SchemaVersion:            authority.SchemaVersion,
		Contract:                 authority.Contract,
		SourceCommit:             authority.SourceCommit,
		WorktreeSnapshot:         authority.WorktreeSnapshot,
		Classification:           authority.Classification,
		Publishable:              authority.Publishable,
		ReleaseEligible:          authority.ReleaseEligible,
		PublicationReceiptIssued: authority.PublicationReceiptIssued,
		TargetKey:                authority.TargetKey,
		BuildContext:             authority.BuildContext,
		NativeDisposition:        authority.NativeDisposition,
		Artifacts:                authority.Artifacts,
		StagedPayload:            authority.StagedPayload,
	}
	body, _ := json.Marshal(without)
	return domainDigest(authorityDigestDomainV2, body)
}

func validateEffectiveBuilderContextV1(value EffectiveBuilderContextV1, targetKey string) error {
	if value.SchemaVersion != 1 || value.Contract != effectiveBuilderContextV1 ||
		!sha256Digest(value.EffectiveConfigSHA256) ||
		value.FusePolicyContract != electronFusePolicyContractV1 ||
		value.FusePolicySHA256 != electronFusePolicySHA256V1 || !sha256Digest(value.ContextDigest) ||
		validateEffectiveBuilderTargetV1(value.Target, targetKey) != nil {
		return errors.New("packaged effective builder context is invalid")
	}
	without := struct {
		SchemaVersion         int                      `json:"schemaVersion"`
		Contract              string                   `json:"contract"`
		EffectiveConfigSHA256 string                   `json:"effectiveConfigSha256"`
		Target                EffectiveBuilderTargetV1 `json:"target"`
		FusePolicyContract    string                   `json:"fusePolicyContract"`
		FusePolicySHA256      string                   `json:"fusePolicySha256"`
	}{
		SchemaVersion: value.SchemaVersion, Contract: value.Contract,
		EffectiveConfigSHA256: value.EffectiveConfigSHA256, Target: value.Target,
		FusePolicyContract: value.FusePolicyContract, FusePolicySHA256: value.FusePolicySHA256,
	}
	body, err := json.Marshal(without)
	if err != nil || value.ContextDigest != domainDigest(effectiveBuilderContextDomain, body) {
		return errors.New("packaged effective builder context digest is invalid")
	}
	return nil
}

func validateEffectiveBuilderTargetV1(value EffectiveBuilderTargetV1, targetKey string) error {
	if value.Key != targetKey || value.Key != value.Platform+"-"+value.Arch ||
		(value.Platform != "darwin" && value.Platform != "win32" && value.Platform != "linux") ||
		(value.Arch != "arm64" && value.Arch != "x64") || !sha256Digest(value.Digest) || value.Targets == nil {
		return errors.New("packaged effective builder target is invalid")
	}
	previous := ""
	for index, target := range value.Targets {
		if target == "" || target != strings.TrimSpace(target) || strings.ContainsRune(target, 0) ||
			(index > 0 && target <= previous) {
			return errors.New("packaged effective builder target inventory is invalid")
		}
		previous = target
	}
	without := struct {
		Key      string   `json:"key"`
		Platform string   `json:"platform"`
		Arch     string   `json:"arch"`
		Targets  []string `json:"targets"`
	}{value.Key, value.Platform, value.Arch, value.Targets}
	body, err := json.Marshal(without)
	if err != nil || value.Digest != domainDigest(effectiveBuilderTargetDomain, body) {
		return errors.New("packaged effective builder target digest is invalid")
	}
	return nil
}

func validateStagedPayloadClosureV1(value StagedPayloadClosureV1) error {
	if value.SchemaVersion != 1 || value.Contract != stagedPayloadContractV1 ||
		value.ExclusionPolicySHA256 != stagedPayloadExclusionSHA256V1 || !sha256Digest(value.ManifestSHA256) ||
		!nonNegativeSafe(value.EntryCount) || !nonNegativeSafe(value.DirectoryCount) ||
		!nonNegativeSafe(value.RegularFileCount) || !nonNegativeSafe(value.NativeFileCount) ||
		!nonNegativeSafe(value.SymlinkCount) || !nonNegativeSafe(value.ContentByteLength) ||
		value.DirectoryCount > maxJavaScriptSafeIntegerV2-value.RegularFileCount ||
		value.DirectoryCount+value.RegularFileCount > maxJavaScriptSafeIntegerV2-value.NativeFileCount ||
		value.DirectoryCount+value.RegularFileCount+value.NativeFileCount > maxJavaScriptSafeIntegerV2-value.SymlinkCount ||
		value.EntryCount != value.DirectoryCount+value.RegularFileCount+value.NativeFileCount+value.SymlinkCount {
		return errors.New("packaged staged payload closure is invalid")
	}
	return nil
}

func validateWorktreeSnapshot(snapshot WorktreeSnapshotV1) error {
	if snapshot.SchemaVersion != 1 || snapshot.Contract != worktreeSnapshotContractV1 ||
		!gitCommit(snapshot.SourceCommit) || (snapshot.State != "clean" && snapshot.State != "dirty") ||
		snapshot.Dirty != (snapshot.State == "dirty") || snapshot.ExclusionPolicySHA256 != worktreeExclusionSHA256V1 ||
		!validSummary(snapshot.StagedPatch) || !validSummary(snapshot.UnstagedPatch) ||
		!validSummary(snapshot.UntrackedSource) || !sha256Digest(snapshot.SnapshotDigest) {
		return errors.New("packaged worktree snapshot is invalid")
	}
	dirty := snapshot.StagedPatch.Count > 0 || snapshot.UnstagedPatch.Count > 0 || snapshot.UntrackedSource.Count > 0
	if snapshot.Dirty != dirty {
		return errors.New("packaged worktree dirty state is inconsistent")
	}
	without := struct {
		SchemaVersion         int               `json:"schemaVersion"`
		Contract              string            `json:"contract"`
		SourceCommit          string            `json:"sourceCommit"`
		State                 string            `json:"state"`
		Dirty                 bool              `json:"dirty"`
		ExclusionPolicySHA256 string            `json:"exclusionPolicySha256"`
		StagedPatch           ManifestSummaryV1 `json:"stagedPatch"`
		UnstagedPatch         ManifestSummaryV1 `json:"unstagedPatch"`
		UntrackedSource       ManifestSummaryV1 `json:"untrackedSource"`
	}{
		SchemaVersion:         snapshot.SchemaVersion,
		Contract:              snapshot.Contract,
		SourceCommit:          snapshot.SourceCommit,
		State:                 snapshot.State,
		Dirty:                 snapshot.Dirty,
		ExclusionPolicySHA256: snapshot.ExclusionPolicySHA256,
		StagedPatch:           snapshot.StagedPatch,
		UnstagedPatch:         snapshot.UnstagedPatch,
		UntrackedSource:       snapshot.UntrackedSource,
	}
	body, _ := json.Marshal(without)
	if snapshot.SnapshotDigest != domainDigest(worktreeDigestDomainV1, body) {
		return errors.New("packaged worktree snapshot digest is invalid")
	}
	return nil
}

func dispositionKind(body json.RawMessage) (string, error) {
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		MaxBytes: MaxAuthorityBytesV2, MaxDepth: 8, MaxTokens: 4096, MaxStringBytes: 16_384,
	})
	if err != nil {
		return "", errors.New("packaged build authority disposition is invalid")
	}
	var kind string
	if json.Unmarshal(fields["kind"], &kind) != nil || kind == "" {
		return "", errors.New("packaged build authority disposition kind is invalid")
	}
	return kind, nil
}

func strictNested(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("nested authority value contains trailing JSON")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(body, canonical) {
		return errors.New("nested authority value is not canonical")
	}
	return nil
}

func validateControlled(value ControlledReleaseDispositionV2, targetKey string) error {
	if value.Kind != ControlledDispositionKindV2 || value.TargetKey != targetKey ||
		!sha256Digest(value.ReceiptSHA256) || !sha256Digest(value.ManifestSHA256) {
		return errors.New("controlled release disposition is invalid")
	}
	if strings.HasPrefix(targetKey, "darwin-") {
		if !sha256Digest(value.SigningPolicySHA256) || (value.SigningMode != "ad-hoc" && value.SigningMode != "developer-id") {
			return errors.New("Darwin signing disposition is invalid")
		}
		if value.SigningMode == "ad-hoc" && value.AppleTeamIdentifier != "" {
			return errors.New("ad-hoc signing disposition cannot claim a team")
		}
		if value.SigningMode == "developer-id" && !teamIdentifier(value.AppleTeamIdentifier) {
			return errors.New("Developer ID signing disposition requires a team")
		}
		return nil
	}
	if value.SigningPolicySHA256 != "" || value.SigningMode != "" || value.AppleTeamIdentifier != "" {
		return errors.New("non-Darwin controlled disposition has unexpected signing fields")
	}
	return nil
}

func validateDevelopment(value DevelopmentDispositionV2, targetKey string) error {
	format, arch, validTarget := developmentNativeTargetV2(targetKey)
	if value.Kind != DevelopmentDispositionKindV2 || value.TargetKey != targetKey || !validContent(value.Marker) ||
		!validTarget || len(value.Components) != len(developmentComponentsV2) {
		return errors.New("development disposition is invalid")
	}
	for index, component := range value.Components {
		expected := developmentComponentsV2[index]
		if component.ID != expected.ID || component.BinaryName != expected.BinaryName ||
			!sha256Digest(component.MarkerBinarySHA256) ||
			!sha256Digest(component.PayloadSHA256) || !positiveSafe(component.MarkerBinaryByteLength) ||
			!positiveSafe(component.PayloadByteLength) || component.PayloadByteLength > component.MarkerBinaryByteLength ||
			component.Format != format || component.Arch != arch {
			return errors.New("development component binding is invalid")
		}
	}
	return nil
}

func developmentNativeTargetV2(targetKey string) (format, arch string, ok bool) {
	switch targetKey {
	case "darwin-arm64":
		return "mach-o", "arm64", true
	case "darwin-x64":
		return "mach-o", "x64", true
	case "linux-x64":
		return "elf", "x64", true
	case "win32-x64":
		return "pe", "x64", true
	default:
		return "", "", false
	}
}

func validArtifacts(value ArtifactBindingsV2) bool {
	return validNative(value.Executable) && validContent(value.AppASAR) && validNative(value.RuntimeServer) &&
		validFundsPlugin(value.FundsPlugin)
}

func validFundsPlugin(value FundsPluginArtifactBindingV2) bool {
	return sha256Digest(value.TreeSHA256) && sha256Digest(value.ManifestSHA256) &&
		sha256Digest(value.EntrypointSHA256) && positiveSafe(value.FileCount) &&
		value.FileCount <= 100_000 && positiveSafe(value.TotalBytes) && value.TotalBytes <= 512<<20
}

func validNative(value NativeArtifactBindingV2) bool {
	return sha256Digest(value.PreSignSHA256) && sha256Digest(value.PayloadSHA256) &&
		positiveSafe(value.PreSignByteLength) && positiveSafe(value.PayloadByteLength) &&
		value.PayloadByteLength <= value.PreSignByteLength && validNativeFormatArch(value.Format, value.Arch)
}

func validContent(value ContentArtifactBindingV2) bool {
	return sha256Digest(value.SHA256) && positiveSafe(value.ByteLength)
}

func validSummary(value ManifestSummaryV1) bool {
	return nonNegativeSafe(value.Count) && nonNegativeSafe(value.ByteLength) && sha256Digest(value.SHA256)
}

func validNativeFormatArch(format, arch string) bool {
	return (format == "mach-o" || format == "pe" || format == "elf") && (arch == "arm64" || arch == "x64")
}

func positiveSafe(value int64) bool    { return value > 0 && value <= maxJavaScriptSafeIntegerV2 }
func nonNegativeSafe(value int64) bool { return value >= 0 && value <= maxJavaScriptSafeIntegerV2 }

func sha256Digest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.TrimSpace(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func gitCommit(value string) bool {
	if len(value) != 40 || value != strings.TrimSpace(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func teamIdentifier(value string) bool {
	if len(value) != 10 {
		return false
	}
	for _, char := range value {
		if !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func classification(dirty bool, kind string) string {
	if kind == DevelopmentDispositionKindV2 {
		if dirty {
			return "development_dirty_non_publishable"
		}
		return "development_clean_non_publishable"
	}
	if dirty {
		return "controlled_release_dirty_non_publishable"
	}
	return "controlled_release_clean_candidate_non_publishable"
}

func domainDigest(domain string, body []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}
