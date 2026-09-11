package toolcatalog

import (
	"errors"
	"testing"

	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

func testLoadedSkillPackage(t *testing.T, id string) LoadedSkillPackage {
	t.Helper()
	snapshot, err := domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: []byte("body")}})
	if err != nil {
		t.Fatal(err)
	}
	return LoadedSkillPackage{
		Record:   map[string]any{"id": id, "name": "Review", "entry": "SKILL.md", "packageDigest": snapshot.Digest()},
		Snapshot: snapshot,
	}
}

func TestLoadSkillCatalogDiscoversConfiguredRoots(t *testing.T) {
	catalog, err := LoadSkillCatalog(SkillCatalogLoadInput{
		Document: map[string]any{
			"skills": map[string]any{"roots": []any{"/skills"}, "enabled": true},
		},
		DataDir: "/data",
		Files: SkillCatalogFileSource{
			NormalizeRoot: func(root string) string { return root },
			RootExists:    func(root string) bool { return root == "/skills" },
			PackageCandidates: func(root string) ([]string, error) {
				if root != "/skills" {
					t.Fatalf("unexpected root: %s", root)
				}
				return []string{"/skills/review/SKILL.md"}, nil
			},
			LoadPackage: func(path string, root string, dataDir string) (LoadedSkillPackage, error) {
				if dataDir != "/data" {
					t.Fatalf("dataDir mismatch: %s", dataDir)
				}
				return testLoadedSkillPackage(t, "review"), nil
			},
		},
	})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if !catalog.Enabled || catalog.Reason != "Skills are discovered from configured roots" || len(catalog.Skills) != 1 {
		t.Fatalf("catalog mismatch: %#v", catalog)
	}
	if len(catalog.RootDiagnostics) != 1 || catalog.RootDiagnostics[0]["skillCount"] != float64(1) {
		t.Fatalf("root diagnostics mismatch: %#v", catalog.RootDiagnostics)
	}
	if catalog.PackageSnapshotCount() != 1 {
		t.Fatalf("catalog must retain one private package snapshot: %#v", catalog)
	}
}

func TestLoadSkillCatalogRecordsMissingAndErroredRoots(t *testing.T) {
	catalog, err := LoadSkillCatalog(SkillCatalogLoadInput{
		Document: map[string]any{
			"skills": map[string]any{"roots": []any{"/missing", "/broken"}, "enabled": true},
		},
		Files: SkillCatalogFileSource{
			NormalizeRoot: func(root string) string { return root },
			RootExists:    func(root string) bool { return root == "/broken" },
			PackageCandidates: func(root string) ([]string, error) {
				return nil, errors.New("cannot inspect root")
			},
		},
	})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if catalog.Reason != "Skills are enabled but no valid skill packages were found" || len(catalog.ValidationErrors) != 2 {
		t.Fatalf("catalog diagnostics mismatch: %#v", catalog)
	}
	if catalog.ValidationErrors[0]["status"] != "error" && catalog.ValidationErrors[1]["status"] != "error" {
		t.Fatalf("expected package candidate error diagnostic: %#v", catalog.ValidationErrors)
	}
}

func TestLoadSkillCatalogDisabledWithoutDocument(t *testing.T) {
	catalog, err := LoadSkillCatalog(SkillCatalogLoadInput{})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if catalog.Enabled || catalog.Reason != "Skills are disabled by config" {
		t.Fatalf("disabled catalog mismatch: %#v", catalog)
	}
}

func TestLoadSkillCatalogFromDocumentHonorsMissingRuntimeConfig(t *testing.T) {
	catalog, err := LoadSkillCatalogFromDocument(map[string]any{
		"skills": map[string]any{"roots": []any{"/skills"}, "enabled": true},
	}, false, "/data", SkillCatalogFileSource{
		RootExists: func(root string) bool {
			t.Fatalf("skill roots should not be inspected without runtime config")
			return false
		},
	})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if catalog.Enabled || catalog.Reason != "Skills are disabled by config" {
		t.Fatalf("disabled catalog mismatch: %#v", catalog)
	}
}
