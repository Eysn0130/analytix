package pluginmaterialization

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

const (
	SchemaVersionV1 = 1

	PluginNameV1             = domainpluginpackage.FirstPartyFundsPackageIDV1
	ManifestRelativePathV1   = ".codex-plugin/plugin.json"
	EntrypointRelativePathV1 = "mcp/server.mjs"
	InstallMarkerFileNameV1  = ".analytix-hub-installed-plugin.json"
	AuthorityAlgorithmV1     = "Ed25519"
	IntentPurposeV1          = "analytix.bundled-plugin-materialization-intent/v1"
	ReceiptPurposeV1         = "analytix.bundled-plugin-materialization-receipt/v1"
	IndexPurposeV1           = "analytix.bundled-plugin-materialization-index/v1"
	JournalPurposeV1         = "analytix.bundled-plugin-materialization-journal/v1"
	MaxContractBytesV1       = 256 << 10
	MaxSourceTreeFilesV1     = 100_000
	DiscoverableGenerationV1 = 1
)

func validPluginIdentityV1(name, version string) bool {
	return domainpluginpackage.ValidPackageIdentityV1(domainpluginpackage.PackageIdentityV1{
		PackageID: name, PackageVersion: version,
	})
}

var fundsSkillNamesV1 = [...]string{
	"analytix-fund-analysis",
	"index",
	"case-context",
	"data-quality",
	"quick-fact",
	"pair-amount-investigation",
	"account-dossier",
	"subject-dossier",
	"counterparty-analysis",
	"fund-tracing",
	"investigation-lab",
	"full-case-analysis",
	"report-builder",
	"evidence-request",
	"analysis-critique",
	"claim-review",
	"delivery-qc",
	"graph-visualization",
	"visual-evidence",
	"case-workbench",
}

func FundsSkillNamesV1() []string {
	return append([]string(nil), fundsSkillNamesV1[:]...)
}

type TargetV1 struct {
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

func ValidateTargetV1(target TargetV1) error {
	switch target.Platform {
	case "darwin":
		if target.Arch == "arm64" || target.Arch == "amd64" {
			return nil
		}
	case "windows", "linux":
		if target.Arch == "arm64" || target.Arch == "amd64" {
			return nil
		}
	}
	return errors.New("bundled plugin materialization target is unsupported")
}

func canonicalDigest(value string) bool {
	if value != strings.TrimSpace(value) || len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func IsCanonicalSHA256V1(value string) bool {
	return canonicalDigest(value)
}

func canonicalTime(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasSuffix(value, "Z") {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.UTC().Format(time.RFC3339Nano) == value
}

func canonicalAbsolutePath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	if strings.HasPrefix(value, "/") {
		return !strings.Contains(value, `\`) && path.Clean(value) == value
	}
	if len(value) < 4 || value[1] != ':' || (value[0] < 'A' || value[0] > 'Z') || value[2] != '\\' || strings.Contains(value, "/") {
		return false
	}
	parts := strings.Split(value[3:], `\`)
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func canonicalRelativePath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || path.IsAbs(value) || strings.Contains(value, `\`) {
		return false
	}
	if path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func digestWithDomain(domain string, value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(append(append([]byte(domain), 0), body...))
	return hex.EncodeToString(digest[:])
}

func strictParse(body []byte, target any, validate func() error) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       MaxContractBytesV1,
		MaxDepth:       8,
		MaxTokens:      2048,
		MaxStringBytes: 64 << 10,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("bundled plugin materialization contract contains trailing JSON")
	}
	if err := validate(); err != nil {
		return err
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(body, canonical) {
		return errors.New("bundled plugin materialization contract is not canonically encoded")
	}
	return nil
}

func canonicalBytes(value any, validate func() error) ([]byte, error) {
	if err := validate(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) > MaxContractBytesV1 {
		return nil, errors.New("bundled plugin materialization contract exceeds its bound")
	}
	return body, nil
}

// An empty origin retains the original packaged V1 wire representation.
const DevelopmentSourceOriginV1 = "development-source"

func validDevelopmentPackageIDV1(id string) bool {
	switch id {
	case "analytix-documents", "analytix-spreadsheets", "analytix-presentations":
		return true
	}
	return false
}

func validOriginProjectionV1(origin, registration string) bool {
	return (origin == "" && registration == "") || (origin == DevelopmentSourceOriginV1 && canonicalDigest(registration))
}

func validMaterializationOriginV1(origin, packaged, registration, packageID string) bool {
	if origin == "" {
		return canonicalDigest(packaged) && registration == ""
	}
	return origin == DevelopmentSourceOriginV1 && packaged == "" && canonicalDigest(registration) && validDevelopmentPackageIDV1(packageID)
}

func validEntrypointForOriginV1(origin, entrypoint string) bool {
	if origin == DevelopmentSourceOriginV1 {
		return entrypoint == ""
	}
	return canonicalDigest(entrypoint)
}
