package filestore

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeSkillTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillSnapshotIsImmutableAfterDiscovery(t *testing.T) {
	root := NormalizeSkillRoot(t.TempDir())
	pkg := filepath.Join(root, "group", "review")
	writeSkillTestFile(t, filepath.Join(pkg, "SKILL.md"), "---\ndescription: Review changes.\n---\n# Skill\n\nFrozen body")
	writeSkillTestFile(t, filepath.Join(pkg, "references", "guide.md"), "Use carefully.")
	writeSkillTestFile(t, filepath.Join(pkg, "scripts", "doctor.mjs"), "export {}")
	writeSkillTestFile(t, filepath.Join(root, "flat.md"), "---\nname: Flat Skill\ndescription: Flat description.\n---\nBody")

	candidates, err := SkillPackageCandidates(root)
	if err != nil || len(candidates) != 2 || candidates[0] != filepath.Join(root, "flat.md") || candidates[1] != pkg {
		t.Fatalf("skill candidates mismatch: candidates=%#v err=%v", candidates, err)
	}
	loaded, err := LoadSkillPackage(pkg, root, root)
	if err != nil {
		t.Fatalf("load skill package: %v", err)
	}
	if loaded.Record["id"] != "review" || loaded.Record["scope"] != "project" || loaded.Record["legacy"] != true {
		t.Fatalf("skill summary mismatch: %#v", loaded.Record)
	}
	entryBefore, ok := loaded.Snapshot.File("SKILL.md")
	if !ok || !strings.Contains(string(entryBefore), "Frozen body") {
		t.Fatalf("snapshot entry missing: %q", entryBefore)
	}
	referenceBefore, ok := loaded.Snapshot.File("references/guide.md")
	if !ok || string(referenceBefore) != "Use carefully." {
		t.Fatalf("snapshot reference missing: %q", referenceBefore)
	}
	writeSkillTestFile(t, filepath.Join(pkg, "SKILL.md"), "replacement sentinel")
	writeSkillTestFile(t, filepath.Join(pkg, "references", "guide.md"), "replacement reference sentinel")
	entryAfter, _ := loaded.Snapshot.File("SKILL.md")
	referenceAfter, _ := loaded.Snapshot.File("references/guide.md")
	if string(entryAfter) != string(entryBefore) || string(referenceAfter) != string(referenceBefore) || strings.Contains(string(entryAfter), "replacement sentinel") {
		t.Fatalf("discovered snapshot changed with live files: entry=%q reference=%q", entryAfter, referenceAfter)
	}

	flat, err := LoadSkillSummary(filepath.Join(root, "flat.md"), root, root)
	if err != nil || flat["id"] != "flat-skill" || flat["description"] != "Flat description." {
		t.Fatalf("flat skill summary mismatch: record=%#v err=%v", flat, err)
	}
	if !SkillNestedCandidate(pkg, root) {
		t.Fatal("nested skill candidate should be detected")
	}
}

func TestSkillManifestEntryMustRemainInsidePackageSnapshot(t *testing.T) {
	root := NormalizeSkillRoot(t.TempDir())
	outside := filepath.Join(root, "outside.md")
	writeSkillTestFile(t, outside, "outside sentinel")
	for index, entry := range []string{"../outside.md", "a/../../outside.md", "/tmp/outside.md", `a\\outside.md`, "./SKILL.md"} {
		pkg := filepath.Join(root, "manifest-"+string(rune('a'+index)))
		writeSkillTestFile(t, filepath.Join(pkg, "skill.json"), `{"name":"Manifest Skill","description":"From manifest.","entry":`+strconv.Quote(entry)+`}`)
		if _, err := LoadSkillPackage(pkg, root, filepath.Join(root, "data")); err == nil {
			t.Fatalf("manifest entry %q escaped or was accepted", entry)
		}
	}
}

func TestSkillManifestSummaryRequiresSnapshotEntry(t *testing.T) {
	root := NormalizeSkillRoot(t.TempDir())
	pkg := filepath.Join(root, "manifest")
	writeSkillTestFile(t, filepath.Join(pkg, "skill.json"), `{"name":"Manifest Skill","description":"From manifest.","entry":"PLAYBOOK.md","runAs":"inline"}`)
	if _, err := LoadSkillSummary(pkg, root, filepath.Join(root, "data")); err == nil {
		t.Fatal("manifest with a missing entry must fail closed")
	}
	writeSkillTestFile(t, filepath.Join(pkg, "PLAYBOOK.md"), "Playbook body")
	record, err := LoadSkillSummary(pkg, root, filepath.Join(root, "data"))
	if err != nil {
		t.Fatalf("load manifest summary: %v", err)
	}
	if record["id"] != "manifest-skill" || record["entry"] != "PLAYBOOK.md" || record["scope"] != "global" || record["runAs"] != "inline" {
		t.Fatalf("manifest skill summary mismatch: %#v", record)
	}
	if digest, _ := record["packageDigest"].(string); len(digest) != 64 {
		t.Fatalf("manifest skill missing full package digest: %#v", record)
	}
}

func TestSkillSnapshotRejectsSymlinkEntryAndReference(t *testing.T) {
	root := NormalizeSkillRoot(t.TempDir())
	outside := filepath.Join(root, "outside.md")
	writeSkillTestFile(t, outside, "outside sentinel")
	outsidePackage := filepath.Join(root, "outside-package")
	writeSkillTestFile(t, filepath.Join(outsidePackage, "SKILL.md"), "---\ndescription: outside\n---\noutside package sentinel")
	packageLink := filepath.Join(root, "package-link")
	if err := os.Symlink(outsidePackage, packageLink); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSkillPackage(packageLink, root, root); err == nil {
		t.Fatal("symlink skill package must be rejected")
	}

	entryPackage := filepath.Join(root, "entry-link")
	if err := os.MkdirAll(entryPackage, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(entryPackage, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSkillPackage(entryPackage, root, root); err == nil {
		t.Fatal("symlink skill entry must be rejected")
	}

	referencePackage := filepath.Join(root, "reference-link")
	writeSkillTestFile(t, filepath.Join(referencePackage, "SKILL.md"), "---\ndescription: safe\n---\nbody")
	if err := os.MkdirAll(filepath.Join(referencePackage, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(referencePackage, "references", "escape.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSkillPackage(referencePackage, root, root); err == nil {
		t.Fatal("symlink skill reference must be rejected")
	}

	referenceDirectoryPackage := filepath.Join(root, "reference-directory-link")
	writeSkillTestFile(t, filepath.Join(referenceDirectoryPackage, "SKILL.md"), "---\ndescription: safe\n---\nbody")
	outsideReferences := filepath.Join(root, "outside-references")
	writeSkillTestFile(t, filepath.Join(outsideReferences, "escape.md"), "outside reference directory sentinel")
	if err := os.Symlink(outsideReferences, filepath.Join(referenceDirectoryPackage, "references")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSkillPackage(referenceDirectoryPackage, root, root); err == nil {
		t.Fatal("symlink references directory must be rejected")
	}

	scriptPackage := filepath.Join(root, "script-link")
	writeSkillTestFile(t, filepath.Join(scriptPackage, "SKILL.md"), "---\ndescription: safe\n---\nbody")
	if err := os.MkdirAll(filepath.Join(scriptPackage, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(scriptPackage, "scripts", "escape.py")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSkillPackage(scriptPackage, root, root); err == nil {
		t.Fatal("symlink helper script must be rejected")
	}
}

func TestSkillSnapshotSwapRaceFailsClosed(t *testing.T) {
	root := NormalizeSkillRoot(t.TempDir())
	pkg := filepath.Join(root, "race")
	entry := filepath.Join(pkg, "SKILL.md")
	outside := filepath.Join(root, "outside.md")
	writeSkillTestFile(t, entry, "safe")
	writeSkillTestFile(t, outside, "outside sentinel")
	_, err := readSkillSnapshotFileWithHook(pkg, "SKILL.md", func() {
		if removeErr := os.Remove(entry); removeErr != nil {
			t.Fatal(removeErr)
		}
		if linkErr := os.Symlink(outside, entry); linkErr != nil {
			t.Fatal(linkErr)
		}
	})
	if err == nil {
		t.Fatal("entry swapped to a symlink during snapshot must fail closed")
	}
}

func TestManagedHubSkillProjectionCannotBecomeExecutableAuthority(t *testing.T) {
	root := NormalizeSkillRoot(t.TempDir())
	for _, test := range []struct {
		name   string
		marker func(*testing.T, string)
	}{
		{
			name: "stale funds projection",
			marker: func(t *testing.T, path string) {
				writeSkillTestFile(t, path, `{"managedBy":"analytix-hub","skillName":"quick-fact","pluginName":"analytix-fund-analysis","version":"0.16.15","platform":"mac-x64","skillPath":"skills/quick-fact/SKILL.md","sourceKind":"plugin"}`)
			},
		},
		{
			name: "forged current projection",
			marker: func(t *testing.T, path string) {
				writeSkillTestFile(t, path, `{"managedBy":"analytix-hub","pluginName":"analytix-fund-analysis","version":"0.16.16"}`)
			},
		},
		{
			name: "symlink marker",
			marker: func(t *testing.T, path string) {
				outside := filepath.Join(root, "outside-marker.json")
				writeSkillTestFile(t, outside, `{}`)
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := filepath.Join(root, strings.ReplaceAll(test.name, " ", "-"))
			writeSkillTestFile(t, filepath.Join(pkg, "SKILL.md"), "---\ndescription: must stay inert\n---\nbody")
			test.marker(t, filepath.Join(pkg, managedHubSkillProjectionMarkerV1))
			loaded, err := LoadSkillPackage(pkg, root, root)
			if err == nil || len(loaded.Record) != 0 || loaded.Snapshot.Valid() {
				t.Fatalf("managed projection became executable: loaded=%#v err=%v", loaded, err)
			}
		})
	}

	authoritative := filepath.Join(root, "plugin-cache", "analytix-fund-analysis", "0.16.16", "skills", "quick-fact")
	writeSkillTestFile(t, filepath.Join(authoritative, "SKILL.md"), "---\ndescription: current verified root candidate\n---\nbody")
	loaded, err := LoadSkillPackage(authoritative, root, root)
	if err != nil || len(loaded.Record) == 0 || !loaded.Snapshot.Valid() {
		t.Fatalf("unprojected plugin package was rejected: loaded=%#v err=%v", loaded, err)
	}

	otherPlugin := filepath.Join(root, "other-plugin-skill")
	writeSkillTestFile(t, filepath.Join(otherPlugin, "SKILL.md"), "---\ndescription: other managed plugin\n---\nbody")
	writeSkillTestFile(t, filepath.Join(otherPlugin, managedHubSkillProjectionMarkerV1), `{"managedBy":"analytix-hub","platform":"mac-arm64","pluginName":"other-plugin","skillName":"other-plugin-skill","skillPath":"skills/other-plugin-skill/SKILL.md","sourceKind":"plugin","version":"1.2.3"}`)
	loaded, err = LoadSkillPackage(otherPlugin, root, root)
	if err != nil || len(loaded.Record) == 0 || !loaded.Snapshot.Valid() {
		t.Fatalf("valid projection for another plugin was rejected: loaded=%#v err=%v", loaded, err)
	}
}

func TestSkillScopeUsesRelativeContainment(t *testing.T) {
	dataDir := NormalizeSkillRoot(filepath.Join(t.TempDir(), "data"))
	if SkillScope(dataDir, dataDir) != "project" || SkillScope(filepath.Join(dataDir, "skills"), dataDir) != "project" {
		t.Fatal("equal and child roots must be project scoped")
	}
	if SkillScope(dataDir+"-evil", dataDir) != "global" || SkillScope(filepath.Join(filepath.Dir(dataDir), "sibling"), dataDir) != "global" {
		t.Fatal("prefix collision and sibling roots must not be project scoped")
	}
	if SkillScriptExtensionAllowed(".exe") {
		t.Fatal("unexpected skill script extension allowed")
	}
}
