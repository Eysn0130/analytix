package pluginpackage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const DevelopmentSourceOriginV1 = "development-source"

// DevelopmentSourceRegistrationV1 describes inspected source bytes, not a
// release admission, resource seal, runtime grant, or proof of adapter readiness.
// Its digest is authenticated by the existing materialization receipt.
type DevelopmentSourceRegistrationV1 struct {
	SchemaVersion              int               `json:"schemaVersion"`
	Origin                     string            `json:"origin"`
	ExecutionMode              string            `json:"executionMode"`
	Identity                   PackageIdentityV1 `json:"identity"`
	DeclarationRawSHA256       string            `json:"declarationRawSha256"`
	DeclarationCanonicalSHA256 string            `json:"declarationCanonicalSha256"`
	DeclarationCanonicalJSON   string            `json:"declarationCanonicalJson"`
	SourceTreeSHA256           string            `json:"sourceTreeSha256"`
	SourceTreeFileCount        uint64            `json:"sourceTreeFileCount"`
	ManifestSHA256             string            `json:"manifestSha256"`
	PublicUISHA256             string            `json:"publicUiSha256"`
	AdapterSHA256              string            `json:"adapterSha256"`
	ContributionsSHA256        string            `json:"contributionsSha256"`
	Publishable                bool              `json:"publishable"`
	FactToolsEnabled           bool              `json:"factToolsEnabled"`
}

// NewDevelopmentSourceRegistrationV1 accepts observations from a bounded tree
// inspector. Materialization independently re-inspects all bytes before signing.
func NewDevelopmentSourceRegistrationV1(observed DevelopmentSourceRegistrationV1) (DevelopmentSourceRegistrationV1, error) {
	observed.SchemaVersion = 1
	observed.Origin = DevelopmentSourceOriginV1
	observed.ExecutionMode = "source-experiment"
	// Never turn caller-supplied publishable/fact-tools assertions into approval.
	observed.ContributionsSHA256 = developmentContributionsSHA256V1(observed)
	if err := ValidateDevelopmentSourceRegistrationV1(observed); err != nil {
		return DevelopmentSourceRegistrationV1{}, err
	}
	return observed, nil
}

func ValidDevelopmentSourcePackageIDV1(id string) bool {
	switch id {
	case "analytix-documents", "analytix-spreadsheets", "analytix-presentations":
		return true
	}
	return false
}

func ValidateDevelopmentSourceDeclarationV1(declaration DeclarationV1) error {
	invalid := errors.New("development source declaration is not an admitted first-party static editor")
	if ValidateDeclarationV1(declaration) != nil || !ValidDevelopmentSourcePackageIDV1(declaration.PackageID) ||
		declaration.Lifecycle.ProtocolVersion != 1 || declaration.Lifecycle.EntryPolicy != "host-static-first-party" ||
		len(declaration.Contributions.Skills) != 0 || len(declaration.Contributions.MCPServers) != 0 || len(declaration.Contributions.Hooks) != 0 ||
		len(declaration.Contributions.PublicUI) != 1 || len(declaration.Contributions.Assets) != 1 ||
		declaration.Contributions.PublicUI[0] != (PathContributionV1{ID: "workspace-editor", Path: "ui/editor.json"}) ||
		declaration.Contributions.Assets[0] != (PathContributionV1{ID: "editor-adapter", Path: "assets/adapter.json"}) ||
		len(declaration.RequestedCapabilities) != 1 {
		return invalid
	}
	capability := declaration.RequestedCapabilities[0]
	if capability.ID != "office.local-edit" || capability.ProtocolVersion != 1 || len(capability.ScopeConstraints) != 2 {
		return invalid
	}
	scopes := map[string]bool{}
	for _, scope := range capability.ScopeConstraints {
		scopes[scope] = true
	}
	if !scopes["user-selected-object"] || !scopes["explicit-save"] {
		return invalid
	}
	return nil
}

func ValidateDevelopmentSourceRegistrationV1(registration DevelopmentSourceRegistrationV1) error {
	invalid := errors.New("development source registration is invalid")
	if registration.SchemaVersion != 1 || registration.Origin != DevelopmentSourceOriginV1 || registration.ExecutionMode != "source-experiment" ||
		registration.Publishable || registration.FactToolsEnabled || !ValidPackageIdentityV1(registration.Identity) ||
		!canonicalSHA256V1(registration.DeclarationRawSHA256) || !canonicalSHA256V1(registration.DeclarationCanonicalSHA256) ||
		!canonicalSHA256V1(registration.SourceTreeSHA256) || registration.SourceTreeFileCount == 0 || registration.SourceTreeFileCount > 100000 ||
		!canonicalSHA256V1(registration.ManifestSHA256) || !canonicalSHA256V1(registration.PublicUISHA256) || !canonicalSHA256V1(registration.AdapterSHA256) ||
		registration.ContributionsSHA256 != developmentContributionsSHA256V1(registration) {
		return invalid
	}
	declaration, err := ParseDeclarationV1([]byte(registration.DeclarationCanonicalJSON))
	if err != nil || ValidateDevelopmentSourceDeclarationV1(declaration) != nil || declaration.IdentityV1() != registration.Identity {
		return invalid
	}
	canonical, err := CanonicalDeclarationV1Bytes(declaration)
	if err != nil || !bytes.Equal(canonical, []byte(registration.DeclarationCanonicalJSON)) || developmentSHA256V1(canonical) != registration.DeclarationCanonicalSHA256 {
		return invalid
	}
	return nil
}

func DevelopmentSourceRegistrationV1Bytes(registration DevelopmentSourceRegistrationV1) ([]byte, error) {
	if err := ValidateDevelopmentSourceRegistrationV1(registration); err != nil {
		return nil, err
	}
	return json.Marshal(registration)
}

func ParseDevelopmentSourceRegistrationV1(body []byte) (DevelopmentSourceRegistrationV1, error) {
	var registration DevelopmentSourceRegistrationV1
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{MaxBytes: 512 << 10, MaxDepth: 8, MaxTokens: 4096, MaxStringBytes: MaxDeclarationBytesV1})
	if err != nil || len(fields) != 15 {
		return registration, errors.New("development source registration JSON is invalid")
	}
	if err := json.Unmarshal(body, &registration); err != nil {
		return registration, err
	}
	canonical, err := DevelopmentSourceRegistrationV1Bytes(registration)
	if err != nil || !bytes.Equal(canonical, body) {
		return DevelopmentSourceRegistrationV1{}, errors.New("development source registration is not canonical")
	}
	return registration, nil
}

func DevelopmentSourceRegistrationSHA256V1(registration DevelopmentSourceRegistrationV1) string {
	body, err := DevelopmentSourceRegistrationV1Bytes(registration)
	if err != nil {
		return ""
	}
	return developmentSHA256V1(append([]byte("analytix.development-source-registration/v1\x00"), body...))
}

func developmentContributionsSHA256V1(registration DevelopmentSourceRegistrationV1) string {
	type contribution struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	body, _ := json.Marshal([]contribution{
		{"publicUi", "workspace-editor", "ui/editor.json", registration.PublicUISHA256},
		{"assets", "editor-adapter", "assets/adapter.json", registration.AdapterSHA256},
	})
	return developmentSHA256V1(append([]byte("analytix.development-source-contributions/v1\x00"), body...))
}

func developmentSHA256V1(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
