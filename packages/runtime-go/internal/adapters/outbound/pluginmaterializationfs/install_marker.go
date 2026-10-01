package pluginmaterializationfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
)

type InstallMarkerIdentityV1 struct {
	SHA256     string
	SourcePath string
}

// InspectInstallMarkerV1 re-reads the installed marker and binds every
// authority-bearing field to the signed receipt. SourcePath is provenance, not
// an execution selector, but remains canonical and part of the marker digest.
func InspectInstallMarkerV1(
	root string,
	receipt domainplugin.ReceiptV1,
) (InstallMarkerIdentityV1, error) {
	if domainplugin.ValidateReceiptV1(receipt) != nil {
		return InstallMarkerIdentityV1{}, errors.New("bundled plugin installation receipt is invalid")
	}
	realRoot, err := canonicalDirectory(root)
	if err != nil {
		return InstallMarkerIdentityV1{}, errors.New("bundled plugin installation root is invalid")
	}
	body, err := stableReadFile(
		filepath.Join(realRoot, domainplugin.InstallMarkerFileNameV1),
		domainplugin.MaxContractBytesV1,
	)
	if err != nil {
		return InstallMarkerIdentityV1{}, errors.New("bundled plugin installation marker is unavailable")
	}
	defer clear(body)
	var marker struct {
		ManagedBy                string `json:"managedBy"`
		MarketplaceName          string `json:"marketplaceName"`
		PluginName               string `json:"pluginName"`
		Version                  string `json:"version"`
		PackageSHA256            string `json:"packageSha256,omitempty"`
		SourcePath               string `json:"sourcePath"`
		SourceTreeSHA256         string `json:"sourceTreeSha256"`
		SourceTreeFileCount      uint64 `json:"sourceTreeFileCount"`
		InstallType              string `json:"installType"`
		Origin                   string `json:"origin,omitempty"`
		SourceRegistrationSHA256 string `json:"sourceRegistrationSha256,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&marker) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) ||
		marker.ManagedBy != "analytix-hub" || marker.MarketplaceName != "analytix-hub" ||
		marker.PluginName != receipt.PluginName || marker.Version != receipt.PluginVersion ||
		marker.Origin != receipt.Origin || marker.SourceRegistrationSHA256 != receipt.SourceRegistrationSHA256 ||
		marker.PackageSHA256 != receipt.PackageAuthoritySHA256 ||
		marker.SourceTreeSHA256 != receipt.SourceTreeSHA256 ||
		marker.SourceTreeFileCount != receipt.SourceTreeFileCount ||
		marker.SourcePath == "" || marker.SourcePath != strings.TrimSpace(marker.SourcePath) ||
		!filepath.IsAbs(marker.SourcePath) || filepath.Clean(marker.SourcePath) != marker.SourcePath ||
		marker.InstallType != "user" {
		return InstallMarkerIdentityV1{}, errors.New("bundled plugin installation marker does not bind the signed receipt")
	}
	canonical, err := json.Marshal(marker)
	if err != nil || !bytes.Equal(canonical, body) {
		return InstallMarkerIdentityV1{}, errors.New("bundled plugin installation marker is not canonical")
	}
	digest := sha256.Sum256(body)
	return InstallMarkerIdentityV1{
		SHA256:     hex.EncodeToString(digest[:]),
		SourcePath: marker.SourcePath,
	}, nil
}
