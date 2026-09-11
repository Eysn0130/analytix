package pluginpackage

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	SchemaVersionV1           = 1
	DeclarationRelativePathV1 = ".analytix-plugin/package.json"
	MaxDeclarationBytesV1     = 256 << 10
	maxIdentifierBytesV1      = 128
	maxSemanticVersionBytesV1 = 128
)

type PackageIdentityV1 struct {
	PackageID      string `json:"packageId"`
	PackageVersion string `json:"packageVersion"`
}

func ValidPackageIdentityV1(identity PackageIdentityV1) bool {
	return validPackageIDV1(identity.PackageID) && validSemanticVersionV1(identity.PackageVersion)
}

type DeclarationV1 struct {
	SchemaVersion         int                   `json:"schemaVersion"`
	PackageID             string                `json:"packageId"`
	PackageVersion        string                `json:"packageVersion"`
	Contributions         ContributionsV1       `json:"contributions"`
	RequestedCapabilities []CapabilityRequestV1 `json:"requestedCapabilities"`
	Lifecycle             LifecycleV1           `json:"lifecycle"`
}

type ContributionsV1 struct {
	Skills     []PathContributionV1      `json:"skills"`
	MCPServers []MCPServerContributionV1 `json:"mcpServers"`
	Hooks      []PathContributionV1      `json:"hooks"`
	Assets     []PathContributionV1      `json:"assets"`
	PublicUI   []PathContributionV1      `json:"publicUi"`
}

type PathContributionV1 struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type MCPServerContributionV1 struct {
	ID         string `json:"id"`
	Entrypoint string `json:"entrypoint"`
}

type CapabilityRequestV1 struct {
	ID               string   `json:"id"`
	ProtocolVersion  int      `json:"protocolVersion"`
	ScopeConstraints []string `json:"scopeConstraints"`
}

type LifecycleV1 struct {
	ProtocolVersion int    `json:"protocolVersion"`
	EntryPolicy     string `json:"entryPolicy"`
}

func (declaration DeclarationV1) IdentityV1() PackageIdentityV1 {
	return PackageIdentityV1{
		PackageID:      declaration.PackageID,
		PackageVersion: declaration.PackageVersion,
	}
}

func ParseDeclarationV1(body []byte) (DeclarationV1, error) {
	object, err := domainjsonstrict.DecodeObject(body, declarationJSONOptionsV1())
	if err != nil {
		return DeclarationV1{}, fmt.Errorf("plugin package declaration v1 strict JSON: %w", err)
	}
	if err := validateDeclarationShapeV1(object); err != nil {
		return DeclarationV1{}, err
	}
	var declaration DeclarationV1
	if err := json.Unmarshal(body, &declaration); err != nil {
		return DeclarationV1{}, fmt.Errorf("plugin package declaration v1 typed decode: %w", err)
	}
	if err := ValidateDeclarationV1(declaration); err != nil {
		return DeclarationV1{}, err
	}
	return declaration, nil
}

func ValidateDeclarationV1(declaration DeclarationV1) error {
	if declaration.SchemaVersion != SchemaVersionV1 {
		return invalidDeclarationV1("schema version")
	}
	if !validPackageIDV1(declaration.PackageID) {
		return invalidDeclarationV1("package id")
	}
	if !validSemanticVersionV1(declaration.PackageVersion) {
		return invalidDeclarationV1("package version")
	}
	if err := validateContributionsV1(declaration.Contributions); err != nil {
		return err
	}
	if declaration.RequestedCapabilities == nil {
		return invalidDeclarationV1("requested capability collection")
	}
	seenCapabilities := make(map[string]struct{}, len(declaration.RequestedCapabilities))
	for _, request := range declaration.RequestedCapabilities {
		if !validContractIdentifierV1(request.ID) || request.ProtocolVersion <= 0 || request.ScopeConstraints == nil || len(request.ScopeConstraints) == 0 {
			return invalidDeclarationV1("capability request")
		}
		if _, exists := seenCapabilities[request.ID]; exists {
			return invalidDeclarationV1("duplicate capability request")
		}
		seenCapabilities[request.ID] = struct{}{}
		seenScopes := make(map[string]struct{}, len(request.ScopeConstraints))
		for _, scope := range request.ScopeConstraints {
			if !validContractIdentifierV1(scope) {
				return invalidDeclarationV1("capability scope constraint")
			}
			if _, exists := seenScopes[scope]; exists {
				return invalidDeclarationV1("duplicate capability scope constraint")
			}
			seenScopes[scope] = struct{}{}
		}
	}
	if declaration.Lifecycle.ProtocolVersion <= 0 || !validContractIdentifierV1(declaration.Lifecycle.EntryPolicy) {
		return invalidDeclarationV1("lifecycle declaration")
	}
	return nil
}

func CanonicalDeclarationV1Bytes(declaration DeclarationV1) ([]byte, error) {
	if err := ValidateDeclarationV1(declaration); err != nil {
		return nil, err
	}
	canonical := cloneDeclarationV1(declaration)
	sortPathContributionsV1(canonical.Contributions.Skills)
	sortMCPServerContributionsV1(canonical.Contributions.MCPServers)
	sortPathContributionsV1(canonical.Contributions.Hooks)
	sortPathContributionsV1(canonical.Contributions.Assets)
	sortPathContributionsV1(canonical.Contributions.PublicUI)
	sort.Slice(canonical.RequestedCapabilities, func(left, right int) bool {
		return canonical.RequestedCapabilities[left].ID < canonical.RequestedCapabilities[right].ID
	})
	for index := range canonical.RequestedCapabilities {
		sort.Strings(canonical.RequestedCapabilities[index].ScopeConstraints)
	}
	body, err := json.Marshal(canonical)
	if err != nil || len(body) > MaxDeclarationBytesV1 {
		return nil, errors.New("plugin package declaration v1 canonical encoding failed")
	}
	return body, nil
}

// ValidPackageRelativePathV1 validates only the portable package-relative
// syntax. Filesystem admission remains responsible for real-root and symlink
// confinement when it opens an artifact.
func ValidPackageRelativePathV1(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || path.IsAbs(value) || strings.Contains(value, `\`) || strings.HasPrefix(value, "/") {
		return false
	}
	if len(value) >= 2 && value[1] == ':' {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return false
		}
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

func declarationJSONOptionsV1() domainjsonstrict.Options {
	return domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       MaxDeclarationBytesV1,
		MaxDepth:       8,
		MaxTokens:      8192,
		MaxStringBytes: 64 << 10,
		MaxNumberBytes: 32,
		MaxAbsExponent: 8,
	}
}

func validateDeclarationShapeV1(object map[string]any) error {
	if !hasExactKeysV1(object, "schemaVersion", "packageId", "packageVersion", "contributions", "requestedCapabilities", "lifecycle") {
		return invalidDeclarationV1("top-level fields")
	}
	contributions, ok := object["contributions"].(map[string]any)
	if !ok || !hasExactKeysV1(contributions, "skills", "mcpServers", "hooks", "assets", "publicUi") {
		return invalidDeclarationV1("contribution fields")
	}
	for _, name := range []string{"skills", "hooks", "assets", "publicUi"} {
		if err := validateObjectArrayShapeV1(contributions[name], "id", "path"); err != nil {
			return invalidDeclarationV1(name + " contribution fields")
		}
	}
	if err := validateObjectArrayShapeV1(contributions["mcpServers"], "id", "entrypoint"); err != nil {
		return invalidDeclarationV1("MCP server contribution fields")
	}
	requests, ok := object["requestedCapabilities"].([]any)
	if !ok {
		return invalidDeclarationV1("requested capability fields")
	}
	for _, raw := range requests {
		request, ok := raw.(map[string]any)
		if !ok || !hasExactKeysV1(request, "id", "protocolVersion", "scopeConstraints") {
			return invalidDeclarationV1("requested capability fields")
		}
		scopes, ok := request["scopeConstraints"].([]any)
		if !ok {
			return invalidDeclarationV1("scope constraint fields")
		}
		for _, scope := range scopes {
			if _, ok := scope.(string); !ok {
				return invalidDeclarationV1("scope constraint type")
			}
		}
	}
	lifecycle, ok := object["lifecycle"].(map[string]any)
	if !ok || !hasExactKeysV1(lifecycle, "protocolVersion", "entryPolicy") {
		return invalidDeclarationV1("lifecycle fields")
	}
	return nil
}

func validateObjectArrayShapeV1(value any, keys ...string) error {
	items, ok := value.([]any)
	if !ok {
		return errors.New("plugin package declaration v1 collection is not an array")
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok || !hasExactKeysV1(object, keys...) {
			return errors.New("plugin package declaration v1 collection item is not closed")
		}
	}
	return nil
}

func hasExactKeysV1(object map[string]any, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, exists := object[key]; !exists {
			return false
		}
	}
	return true
}

func validateContributionsV1(contributions ContributionsV1) error {
	if contributions.Skills == nil || contributions.MCPServers == nil || contributions.Hooks == nil || contributions.Assets == nil || contributions.PublicUI == nil {
		return invalidDeclarationV1("contribution collection")
	}
	if len(contributions.Skills)+len(contributions.MCPServers)+len(contributions.Hooks)+len(contributions.Assets)+len(contributions.PublicUI) == 0 {
		return invalidDeclarationV1("empty contributions")
	}
	seenIDs := map[string]struct{}{}
	seenPaths := map[string]struct{}{}
	for _, collection := range [][]PathContributionV1{contributions.Skills, contributions.Hooks, contributions.Assets, contributions.PublicUI} {
		for _, contribution := range collection {
			if err := validateContributionIdentityAndPathV1(contribution.ID, contribution.Path, seenIDs, seenPaths); err != nil {
				return err
			}
		}
	}
	for _, server := range contributions.MCPServers {
		if err := validateContributionIdentityAndPathV1(server.ID, server.Entrypoint, seenIDs, seenPaths); err != nil {
			return err
		}
	}
	return nil
}

func validateContributionIdentityAndPathV1(id, relativePath string, seenIDs, seenPaths map[string]struct{}) error {
	if !validContractIdentifierV1(id) || !ValidPackageRelativePathV1(relativePath) {
		return invalidDeclarationV1("contribution identity or path")
	}
	if _, exists := seenIDs[id]; exists {
		return invalidDeclarationV1("duplicate contribution id")
	}
	seenIDs[id] = struct{}{}
	if _, exists := seenPaths[relativePath]; exists {
		return invalidDeclarationV1("duplicate contribution path")
	}
	seenPaths[relativePath] = struct{}{}
	return nil
}

func validPackageIDV1(value string) bool {
	if len(value) == 0 || len(value) > maxIdentifierBytesV1 || value != strings.TrimSpace(value) || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	previousSeparator := false
	for index, character := range []byte(value) {
		letter := character >= 'a' && character <= 'z'
		digit := character >= '0' && character <= '9'
		separator := character == '-'
		if !letter && !digit && !separator || separator && (index == 0 || index == len(value)-1 || previousSeparator) {
			return false
		}
		previousSeparator = separator
	}
	return true
}

func validContractIdentifierV1(value string) bool {
	if len(value) == 0 || len(value) > maxIdentifierBytesV1 || value != strings.TrimSpace(value) || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	previousSeparator := false
	for index, character := range []byte(value) {
		letter := character >= 'a' && character <= 'z'
		digit := character >= '0' && character <= '9'
		separator := character == '-' || character == '_' || character == '.' || character == ':'
		if !letter && !digit && !separator || separator && (index == 0 || index == len(value)-1 || previousSeparator) {
			return false
		}
		previousSeparator = separator
	}
	return true
}

func validSemanticVersionV1(value string) bool {
	if len(value) == 0 || len(value) > maxSemanticVersionBytesV1 || value != strings.TrimSpace(value) {
		return false
	}
	if strings.Count(value, "+") > 1 {
		return false
	}
	versionWithoutBuild, build, hasBuild := strings.Cut(value, "+")
	if hasBuild && !validSemanticIdentifiersV1(build, false) {
		return false
	}
	coreText, preRelease, hasPreRelease := strings.Cut(versionWithoutBuild, "-")
	if hasPreRelease && !validSemanticIdentifiersV1(preRelease, true) {
		return false
	}
	core := strings.Split(coreText, ".")
	if len(core) != 3 {
		return false
	}
	for _, part := range core {
		if !canonicalUnsignedDecimalV1(part) {
			return false
		}
	}
	return true
}

func validSemanticIdentifiersV1(value string, rejectNumericLeadingZero bool) bool {
	parts := strings.Split(value, ".")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		numeric := true
		for _, character := range []byte(part) {
			digit := character >= '0' && character <= '9'
			upper := character >= 'A' && character <= 'Z'
			lower := character >= 'a' && character <= 'z'
			if !digit {
				numeric = false
			}
			if !digit && !upper && !lower && character != '-' {
				return false
			}
		}
		if rejectNumericLeadingZero && numeric && len(part) > 1 && part[0] == '0' {
			return false
		}
	}
	return true
}

func canonicalUnsignedDecimalV1(value string) bool {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return false
	}
	for _, character := range []byte(value) {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func cloneDeclarationV1(declaration DeclarationV1) DeclarationV1 {
	clone := declaration
	clone.Contributions.Skills = append([]PathContributionV1{}, declaration.Contributions.Skills...)
	clone.Contributions.MCPServers = append([]MCPServerContributionV1{}, declaration.Contributions.MCPServers...)
	clone.Contributions.Hooks = append([]PathContributionV1{}, declaration.Contributions.Hooks...)
	clone.Contributions.Assets = append([]PathContributionV1{}, declaration.Contributions.Assets...)
	clone.Contributions.PublicUI = append([]PathContributionV1{}, declaration.Contributions.PublicUI...)
	clone.RequestedCapabilities = append([]CapabilityRequestV1{}, declaration.RequestedCapabilities...)
	for index := range clone.RequestedCapabilities {
		clone.RequestedCapabilities[index].ScopeConstraints = append([]string{}, declaration.RequestedCapabilities[index].ScopeConstraints...)
	}
	return clone
}

func sortPathContributionsV1(values []PathContributionV1) {
	sort.Slice(values, func(left, right int) bool {
		if values[left].ID != values[right].ID {
			return values[left].ID < values[right].ID
		}
		return values[left].Path < values[right].Path
	})
}

func sortMCPServerContributionsV1(values []MCPServerContributionV1) {
	sort.Slice(values, func(left, right int) bool {
		if values[left].ID != values[right].ID {
			return values[left].ID < values[right].ID
		}
		return values[left].Entrypoint < values[right].Entrypoint
	})
}

func invalidDeclarationV1(part string) error {
	return fmt.Errorf("plugin package declaration v1 is invalid: %s", part)
}
