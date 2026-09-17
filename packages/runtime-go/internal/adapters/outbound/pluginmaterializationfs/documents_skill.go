package pluginmaterializationfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	hostport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// StaticEditorSkillReader is composed only from the inspected source registration
// and the host's installation authority. Neither value comes from a tool call.
type StaticEditorSkillReader struct {
	Store     *Store
	Authority pluginport.InstallationAuthority
	SHA256    string
}

// OfficeSkillReader preserves the existing Office composition API.
type OfficeSkillReader = StaticEditorSkillReader

func (reader StaticEditorSkillReader) ReadSkill(ctx context.Context, expected hostport.Binding) (domainskill.PackageSnapshot, error) {
	store := reader.Store
	if store == nil || ctx == nil || ctx.Err() != nil || !domainpackage.ValidDevelopmentSourcePackageIDV1(expected.PackageID) || !domainplugin.IsCanonicalSHA256V1(reader.SHA256) {
		return domainskill.PackageSnapshot{}, pluginport.ErrUnavailable
	}
	keyID, key, err := validateAuthority(reader.Authority)
	if err != nil {
		return domainskill.PackageSnapshot{}, pluginport.ErrUnavailable
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	current, err := store.resolveActive(ctx, keyID, key)
	if err != nil || current.Receipt.PluginName != expected.PackageID || current.Receipt.PluginVersion != expected.PackageVersion ||
		current.Receipt.GenerationID != expected.GenerationID || current.Receipt.SourceRegistrationSHA256 != expected.SourceRegistrationSHA256 {
		return domainskill.PackageSnapshot{}, pluginport.ErrConflict
	}
	contribution, ok := domainpackage.StaticEditorSkillContributionV1(expected.PackageID)
	if !ok {
		return domainskill.PackageSnapshot{}, pluginport.ErrUnavailable
	}
	body, err := stableReadFile(filepath.Join(store.absolute(current.Receipt.ActiveRelativePath), filepath.FromSlash(contribution.Path)), 128<<10)
	if err != nil || len(body) == 0 {
		return domainskill.PackageSnapshot{}, pluginport.ErrCorrupt
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != reader.SHA256 {
		return domainskill.PackageSnapshot{}, pluginport.ErrCorrupt
	}
	after, err := store.resolveActive(ctx, keyID, key)
	if err != nil || after != current {
		return domainskill.PackageSnapshot{}, pluginport.ErrConflict
	}
	return domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: body}})
}
