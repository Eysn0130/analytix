package toolcatalog

import (
	contracts "analytix.local/runtime-go/internal/contracts"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

type SkillCatalogFileSource struct {
	NormalizeRoot     func(string) string
	RootExists        func(string) bool
	PackageCandidates func(string) ([]string, error)
	LoadPackage       func(path string, root string, dataDir string) (LoadedSkillPackage, error)
}

type SkillCatalogLoadInput struct {
	Document map[string]any
	DataDir  string
	Files    SkillCatalogFileSource
}

func LoadSkillCatalogFromDocument(document map[string]any, ok bool, dataDir string, files SkillCatalogFileSource) (SkillCatalog, error) {
	if !ok {
		document = nil
	}
	return LoadSkillCatalog(SkillCatalogLoadInput{
		Document: document,
		DataDir:  dataDir,
		Files:    files,
	})
}

func LoadSkillCatalog(input SkillCatalogLoadInput) (SkillCatalog, error) {
	if input.Document == nil {
		return SkillCatalog{Reason: "Skills are disabled by config"}, nil
	}

	topSkills, _ := input.Document["skills"].(map[string]any)
	capabilities, _ := input.Document["capabilities"].(map[string]any)
	capabilitySkills, _ := capabilities["skills"].(map[string]any)

	roots := []string{}
	roots = append(roots, stringList(topSkills["roots"])...)
	roots = append(roots, stringList(capabilitySkills["roots"])...)
	roots = UniqueStringsSorted(roots)

	enabled := len(roots) > 0
	if value, ok := topSkills["enabled"].(bool); ok {
		enabled = value
	}
	if value, ok := capabilitySkills["enabled"].(bool); ok {
		enabled = value
	}

	catalog := SkillCatalog{
		Enabled:   enabled,
		Roots:     []string{},
		Reason:    "Skills are disabled by config",
		snapshots: map[string]domainskill.PackageSnapshot{},
	}
	if !enabled {
		return catalog, nil
	}
	if len(roots) == 0 {
		catalog.Reason = "Skills are enabled but no skill roots are configured"
		return catalog, nil
	}

	for _, rawRoot := range roots {
		root := normalizeSkillRoot(input.Files, rawRoot)
		if root == "" {
			continue
		}
		catalog.Roots = append(catalog.Roots, root)
		if !skillRootExists(input.Files, root) {
			diagnostic := map[string]any{
				"root":   root,
				"status": "missing",
				"error":  "skill root does not exist or is not a directory",
			}
			catalog.RootDiagnostics = append(catalog.RootDiagnostics, diagnostic)
			catalog.ValidationErrors = append(catalog.ValidationErrors, diagnostic)
			continue
		}

		candidates, candidateErr := skillPackageCandidates(input.Files, root)
		if candidateErr != nil {
			diagnostic := map[string]any{
				"root":   root,
				"status": "error",
				"error":  candidateErr.Error(),
			}
			catalog.RootDiagnostics = append(catalog.RootDiagnostics, diagnostic)
			catalog.ValidationErrors = append(catalog.ValidationErrors, diagnostic)
			continue
		}

		discovered := 0
		for _, candidate := range candidates {
			loaded, skillErr := skillPackage(input.Files, candidate, root, input.DataDir)
			if skillErr != nil {
				catalog.ValidationErrors = append(catalog.ValidationErrors, map[string]any{
					"root":  root,
					"path":  candidate,
					"error": skillErr.Error(),
				})
				continue
			}
			if len(loaded.Record) == 0 {
				continue
			}
			if !loaded.Snapshot.Valid() || loaded.Snapshot.Digest() != contracts.StringField(loaded.Record, "packageDigest") {
				catalog.ValidationErrors = append(catalog.ValidationErrors, map[string]any{
					"root":  root,
					"path":  candidate,
					"error": "skill package snapshot authority is invalid",
				})
				continue
			}
			discovered++
			catalog.Skills = append(catalog.Skills, loaded.Record)
			catalog.snapshots[loaded.Snapshot.Digest()] = loaded.Snapshot
		}
		catalog.RootDiagnostics = append(catalog.RootDiagnostics, map[string]any{
			"root":       root,
			"status":     "available",
			"skillCount": float64(discovered),
		})
	}

	catalog.Roots = UniqueStringsSorted(catalog.Roots)
	catalog.Skills = DedupeSkills(catalog.Skills)
	referencedSnapshots := map[string]domainskill.PackageSnapshot{}
	for _, skill := range catalog.Skills {
		digest := contracts.StringField(skill, "packageDigest")
		if snapshot, ok := catalog.snapshots[digest]; ok {
			referencedSnapshots[digest] = snapshot
		}
	}
	catalog.snapshots = referencedSnapshots
	if len(catalog.Skills) > 0 {
		catalog.Reason = "Skills are discovered from configured roots"
	} else {
		catalog.Reason = "Skills are enabled but no valid skill packages were found"
	}
	return catalog, nil
}

func normalizeSkillRoot(files SkillCatalogFileSource, root string) string {
	if files.NormalizeRoot == nil {
		return root
	}
	return files.NormalizeRoot(root)
}

func skillRootExists(files SkillCatalogFileSource, root string) bool {
	return files.RootExists != nil && files.RootExists(root)
}

func skillPackageCandidates(files SkillCatalogFileSource, root string) ([]string, error) {
	if files.PackageCandidates == nil {
		return nil, nil
	}
	return files.PackageCandidates(root)
}

func skillPackage(files SkillCatalogFileSource, path string, root string, dataDir string) (LoadedSkillPackage, error) {
	if files.LoadPackage == nil {
		return LoadedSkillPackage{}, nil
	}
	return files.LoadPackage(path, root, dataDir)
}
