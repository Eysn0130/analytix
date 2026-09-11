package job

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const DelegatedToolManifestVersionV1 = 1

// DelegatedToolManifestV1 freezes the child tool capability contract across
// queueing, background admission, restart, and the first provider dispatch.
// MCPAuthorityHash additionally binds host policy and live server authority
// that are intentionally absent from the provider-facing schema hash.
type DelegatedToolManifestV1 struct {
	Version          int    `json:"version"`
	ScopeHash        string `json:"scopeHash"`
	ToolSchemaHash   string `json:"toolSchemaHash"`
	MCPAuthorityHash string `json:"mcpAuthorityHash"`
	ManifestHash     string `json:"manifestHash"`
}

func NewDelegatedToolManifestV1(toolScope []string, toolSchemaHash string, mcpAuthorityHash string) (*DelegatedToolManifestV1, error) {
	scopeHash, err := DelegatedToolScopeHashV1(toolScope)
	if err != nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(toolSchemaHash)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(mcpAuthorityHash)) {
		return nil, errors.New("delegated tool manifest authority is invalid")
	}
	manifest := &DelegatedToolManifestV1{
		Version: DelegatedToolManifestVersionV1, ScopeHash: scopeHash,
		ToolSchemaHash: strings.TrimSpace(toolSchemaHash), MCPAuthorityHash: strings.TrimSpace(mcpAuthorityHash),
	}
	manifest.ManifestHash = delegatedToolManifestHashV1(*manifest)
	return manifest, nil
}

func ValidateDelegatedToolManifestV1(manifest *DelegatedToolManifestV1, toolScope []string, toolSchemaHash string) error {
	if manifest == nil || manifest.Version != DelegatedToolManifestVersionV1 ||
		manifest.ScopeHash != strings.TrimSpace(manifest.ScopeHash) || manifest.ToolSchemaHash != strings.TrimSpace(manifest.ToolSchemaHash) ||
		manifest.MCPAuthorityHash != strings.TrimSpace(manifest.MCPAuthorityHash) || manifest.ManifestHash != strings.TrimSpace(manifest.ManifestHash) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(manifest.ScopeHash)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(manifest.ToolSchemaHash)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(manifest.MCPAuthorityHash)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(manifest.ManifestHash)) ||
		manifest.ManifestHash != delegatedToolManifestHashV1(*manifest) {
		return errors.New("delegated tool manifest integrity is invalid")
	}
	scopeHash, err := DelegatedToolScopeHashV1(toolScope)
	if err != nil || manifest.ScopeHash != scopeHash || manifest.ToolSchemaHash != strings.TrimSpace(toolSchemaHash) {
		return errors.New("delegated tool manifest does not match the frozen tool scope")
	}
	return nil
}

func DelegatedToolScopeHashV1(toolScope []string) (string, error) {
	values := make([]string, 0, len(toolScope))
	seen := make(map[string]bool, len(toolScope))
	for _, value := range toolScope {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return "", errors.New("delegated tool scope is invalid")
		}
		seen[value] = true
		values = append(values, value)
	}
	sort.Strings(values)
	body, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(body), nil
}

func CloneDelegatedToolManifestV1(manifest *DelegatedToolManifestV1) *DelegatedToolManifestV1 {
	if manifest == nil {
		return nil
	}
	cloned := *manifest
	return &cloned
}

func delegatedToolManifestHashV1(manifest DelegatedToolManifestV1) string {
	return domainsecurity.SHA256Hex([]byte(
		"analytix.delegated-tool-manifest/v1\x00" + strconv.Itoa(manifest.Version) + "\x00" +
			strings.TrimSpace(manifest.ScopeHash) + "\x00" + strings.TrimSpace(manifest.ToolSchemaHash) + "\x00" +
			strings.TrimSpace(manifest.MCPAuthorityHash),
	))
}
