package pluginmaterializationfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const (
	globalSkillsRelativeV1                    = "skills"
	managedHubSkillProjectionMarkerFileNameV1 = ".analytix-hub-skill.json"
	maxManagedHubSkillProjectionMarkerBytesV1 = 64 << 10
)

type managedHubSkillProjectionMarkerV1 struct {
	ManagedBy  string `json:"managedBy"`
	Platform   string `json:"platform"`
	PluginName string `json:"pluginName"`
	SkillName  string `json:"skillName"`
	SkillPath  string `json:"skillPath"`
	SourceKind string `json:"sourceKind"`
	Version    string `json:"version"`
}

func (store *Store) quarantineLegacyFundsSkillProjections(intent domainplugin.IntentV1) error {
	if store == nil || intent.PluginName != domainplugin.PluginNameV1 {
		return pluginport.ErrInvalid
	}
	skillsRoot := store.absolute(globalSkillsRelativeV1)
	rootInfo, err := os.Lstat(skillsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.Join(pluginport.ErrCorrupt, errors.New("global skill root is not a regular directory"), err)
	}
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	authorizedNames := make(map[string]struct{}, len(domainplugin.FundsSkillNamesV1()))
	for _, name := range domainplugin.FundsSkillNamesV1() {
		authorizedNames[name] = struct{}{}
	}
	quarantineRoot, err := store.ensurePrivateDirectory(controlRelativeV1 + "/quarantine/" + intent.IntentID)
	if err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	moved := false
	for _, entry := range entries {
		if _, expected := authorizedNames[entry.Name()]; !expected {
			continue
		}
		source := filepath.Join(skillsRoot, entry.Name())
		info, infoErr := os.Lstat(source)
		if infoErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		marker, ownedLegacy := readOwnedLegacyFundsSkillProjectionMarkerV1(
			source, entry.Name(), intent.PluginName, intent.PluginVersion,
		)
		if !ownedLegacy {
			continue
		}
		digest := sha256.Sum256([]byte("analytix.bundled-plugin-global-skill-quarantine/v1\x00" + entry.Name() + "\x00" + marker.Version))
		target := filepath.Join(quarantineRoot, "global-skill-"+hex.EncodeToString(digest[:]))
		if _, targetErr := os.Lstat(target); targetErr == nil {
			return errors.Join(pluginport.ErrConflict, errors.New("legacy funds skill quarantine target already exists"))
		} else if !errors.Is(targetErr, os.ErrNotExist) {
			return errors.Join(pluginport.ErrUnavailable, targetErr)
		}
		if err := os.Rename(source, target); err != nil {
			return errors.Join(pluginport.ErrUnavailable, err)
		}
		moved = true
	}
	if !moved {
		return nil
	}
	if err := syncDirectory(skillsRoot); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := syncDirectory(quarantineRoot); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	return nil
}

func readOwnedLegacyFundsSkillProjectionMarkerV1(
	root, skillName, currentPluginName, currentPluginVersion string,
) (managedHubSkillProjectionMarkerV1, bool) {
	markerPath := filepath.Join(root, managedHubSkillProjectionMarkerFileNameV1)
	body, err := stableReadFile(markerPath, maxManagedHubSkillProjectionMarkerBytesV1)
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxManagedHubSkillProjectionMarkerBytesV1,
		MaxDepth:       4,
		MaxTokens:      32,
		MaxStringBytes: 4096,
	}) != nil {
		return managedHubSkillProjectionMarkerV1{}, false
	}
	var marker managedHubSkillProjectionMarkerV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&marker) != nil {
		return managedHubSkillProjectionMarkerV1{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return managedHubSkillProjectionMarkerV1{}, false
	}
	if strings.TrimSpace(marker.ManagedBy) != "analytix-hub" ||
		strings.TrimSpace(marker.Platform) == "" ||
		strings.TrimSpace(marker.PluginName) != currentPluginName ||
		strings.TrimSpace(marker.SkillName) != skillName ||
		strings.TrimSpace(marker.SkillPath) != "skills/"+skillName+"/SKILL.md" ||
		strings.TrimSpace(marker.SourceKind) != "plugin" ||
		strings.TrimSpace(marker.Version) == "" ||
		strings.TrimSpace(marker.Version) == currentPluginVersion {
		return managedHubSkillProjectionMarkerV1{}, false
	}
	return marker, true
}
