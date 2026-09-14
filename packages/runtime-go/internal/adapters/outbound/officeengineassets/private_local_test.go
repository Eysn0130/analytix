package officeengineassets

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

// Metadata only: these fixed asset hashes are not replaced by tiny fixtures.
// OpenPrivateLocal must never accept the synthetic on-disk tree below.
func privateFixture(t *testing.T) privateQualification {
	t.Helper()
	layout, err := loadPrivateLayout()
	if err != nil {
		t.Fatal(err)
	}
	q := privateQualification{privateBody: privateBody{SchemaVersion: 1, Contract: privateContract, Usage: "private-local", TargetKey: "darwin-arm64",
		SourceCommit: strings.Repeat("a", 40), WorktreeSnapshotDigest: strings.Repeat("b", 64), EngineBuildID: layout.BuildID, EngineManifestSHA256: layout.ManifestSHA256}}
	for _, f := range layout.Files {
		entry := privateFile{Path: f.Path, ByteLength: 9, SHA256: privateHash([]byte("synthetic"))}
		if f.ByteLength != nil {
			entry.ByteLength = *f.ByteLength
			entry.SHA256 = *f.SHA256
		}
		q.Files = append(q.Files, entry)
	}
	q.QualificationDigest = privateDigest(q.privateBody)
	return q
}
func privateFixtureBytes(q privateQualification) []byte {
	q.QualificationDigest = privateDigest(q.privateBody)
	return append(privateJSON(q), '\n')
}
func privateTestRoot(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("private-local held-fd implementation is unavailable")
	}
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func privateWrite(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
}
func privateInspection(root string, q privateQualification) packagedbuildauthorityfs.InspectionV2 {
	return packagedbuildauthorityfs.InspectionV2{ResourcesRoot: root, PackageAnchor: "macos_nonpublishable_resource_seal",
		Authority: domainauthority.ParsedAuthorityV2{Authority: domainauthority.AuthorityV2{SourceCommit: q.SourceCommit, TargetKey: "darwin-arm64",
			WorktreeSnapshot: domainauthority.WorktreeSnapshotV1{SourceCommit: q.SourceCommit, SnapshotDigest: q.WorktreeSnapshotDigest}},
			Development: &domainauthority.DevelopmentDispositionV2{Kind: domainauthority.DevelopmentDispositionKindV2, TargetKey: "darwin-arm64"}}}
}
func privateTinyTree(t *testing.T) (string, privateQualification) {
	t.Helper()
	root, q := privateTestRoot(t), privateFixture(t)
	for _, f := range q.Files {
		privateWrite(t, filepath.Join(root, "office-private", filepath.FromSlash(f.Path)), []byte("synthetic"))
	}
	privateWrite(t, filepath.Join(root, "office-private", "qualification.json"), privateFixtureBytes(q))
	return root, q
}
func TestPrivateLocalCrossLanguageDigestAndFixedLayout(t *testing.T) {
	q := privateFixture(t)
	if q.QualificationDigest != "9cd95af9b21eb4efb244895e913231e00bdece39aa4b01d53bca6e44d5d3bb0e" {
		t.Fatal("JS domain/order digest drift", q.QualificationDigest)
	}
	if q.EngineBuildID != "efaf0670b4d055f838a2849becb10f08aa06a257" || q.EngineManifestSHA256 != "87f31a749b2cd71f60da5f96c8e2a6323bcb6a53d9fa8339bca383e66bcdf2c5" || len(q.Files) != 35 {
		t.Fatal("fixed manifest/layout drift")
	}
	raw := privateFixtureBytes(q)
	parsed, err := parsePrivateQualification(raw)
	if err != nil || !bytes.Equal(raw, privateFixtureBytes(parsed)) {
		t.Fatal("canonical metadata rejected", err)
	}
}
func TestPrivateLocalRejectsAmbiguousOrNoncanonicalQualification(t *testing.T) {
	raw := privateFixtureBytes(privateFixture(t))
	for name, body := range map[string][]byte{
		"duplicate":     bytes.Replace(raw, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1),
		"unknown":       bytes.Replace(raw, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"unknown":false`), 1),
		"missing_false": bytes.Replace(raw, []byte(`"publishable":false,`), nil, 1),
		"reordered":     bytes.Replace(raw, []byte(`"usage":"private-local","publishable":false`), []byte(`"publishable":false,"usage":"private-local"`), 1),
		"fraction":      bytes.Replace(raw, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1.0`), 1),
		"no_lf":         bytes.TrimSuffix(raw, []byte{'\n'}), "extra_lf": append(bytes.Clone(raw), '\n'),
		"bom": append([]byte{0xef, 0xbb, 0xbf}, raw...), "invalid_utf8": {0xff},
		"digest": bytes.Replace(raw, []byte(`"qualificationDigest":"9`), []byte(`"qualificationDigest":"0`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parsePrivateQualification(body); err == nil {
				t.Fatal("invalid qualification accepted")
			}
		})
	}
}
func TestPrivateLocalRehashingCannotReplaceFixedAssetsOrNotices(t *testing.T) {
	for _, expected := range privateFixture(t).Files {
		if !strings.HasPrefix(expected.Path, "assets/") && !strings.HasPrefix(expected.Path, "notices/") && expected.Path != "manifest.json" {
			continue
		}
		if expected.ByteLength == 9 {
			continue
		}
		t.Run(expected.Path, func(t *testing.T) {
			q := privateFixture(t)
			for i := range q.Files {
				if q.Files[i].Path == expected.Path {
					q.Files[i].SHA256 = strings.Repeat("0", 64)
				}
			}
			if _, err := parsePrivateQualification(privateFixtureBytes(q)); err == nil {
				t.Fatal("self-rehashed replacement admitted")
			}
		})
	}
	for name, change := range map[string]func(*privateQualification){
		"publishable": func(q *privateQualification) { q.Publishable = true }, "release": func(q *privateQualification) { q.ReleaseEligible = true },
		"target": func(q *privateQualification) { q.TargetKey = "linux-amd64" }, "source": func(q *privateQualification) { q.SourceCommit = "bad" },
		"snapshot": func(q *privateQualification) { q.WorktreeSnapshotDigest = "bad" }, "build": func(q *privateQualification) { q.EngineBuildID = strings.Repeat("c", 40) },
		"omit": func(q *privateQualification) { q.Files = q.Files[1:] }, "order": func(q *privateQualification) { q.Files[0], q.Files[1] = q.Files[1], q.Files[0] },
		"escape": func(q *privateQualification) { q.Files[0].Path = "../outside" }, "size": func(q *privateQualification) { q.Files[0].ByteLength-- },
	} {
		t.Run(name, func(t *testing.T) {
			q := privateFixture(t)
			change(&q)
			if _, err := parsePrivateQualification(privateFixtureBytes(q)); err == nil {
				t.Fatal("invalid rehashed metadata accepted")
			}
		})
	}
}
func TestPrivateLocalRequiresVerifiedDevelopmentInspection(t *testing.T) {
	q := privateFixture(t)
	for name, change := range map[string]func(*packagedbuildauthorityfs.InspectionV2){
		"anchor":      func(i *packagedbuildauthorityfs.InspectionV2) { i.PackageAnchor = "macos_developer_id_resource_seal" },
		"development": func(i *packagedbuildauthorityfs.InspectionV2) { i.Authority.Development = nil },
		"controlled": func(i *packagedbuildauthorityfs.InspectionV2) {
			i.Authority.Controlled = &domainauthority.ControlledReleaseDispositionV2{}
		},
		"target":                func(i *packagedbuildauthorityfs.InspectionV2) { i.Authority.Authority.TargetKey = "linux-amd64" },
		"development_target":    func(i *packagedbuildauthorityfs.InspectionV2) { i.Authority.Development.TargetKey = "darwin-x64" },
		"publishable":           func(i *packagedbuildauthorityfs.InspectionV2) { i.Publishable = true },
		"fact":                  func(i *packagedbuildauthorityfs.InspectionV2) { i.FactToolsEnabled = true },
		"authority_publishable": func(i *packagedbuildauthorityfs.InspectionV2) { i.Authority.Authority.Publishable = true },
		"authority_release":     func(i *packagedbuildauthorityfs.InspectionV2) { i.Authority.Authority.ReleaseEligible = true },
		"publication":           func(i *packagedbuildauthorityfs.InspectionV2) { i.Authority.Authority.PublicationReceiptIssued = true },
		"snapshot_source": func(i *packagedbuildauthorityfs.InspectionV2) {
			i.Authority.Authority.WorktreeSnapshot.SourceCommit = strings.Repeat("c", 40)
		},
	} {
		t.Run(name, func(t *testing.T) {
			i := privateInspection("/unused", q)
			change(&i)
			if privateInspectionAllowed(i) {
				t.Fatal("invalid inspection accepted")
			}
			if _, err := OpenPrivateLocal(context.Background(), i); err == nil {
				t.Fatal("invalid inspection opened")
			}
		})
	}
}
func TestPrivateLocalRejectsTinyTreeAndSealedIdentityMismatch(t *testing.T) {
	root, q := privateTinyTree(t)
	if _, err := OpenPrivateLocal(context.Background(), privateInspection(root, q)); err == nil {
		t.Fatal("synthetic bytes impersonated fixed engine")
	}
	for _, change := range []func(*privateQualification){func(q *privateQualification) { q.SourceCommit = strings.Repeat("c", 40) }, func(q *privateQualification) { q.WorktreeSnapshotDigest = strings.Repeat("c", 64) }} {
		changed := q
		change(&changed)
		privateWrite(t, filepath.Join(root, "office-private", "qualification.json"), privateFixtureBytes(changed))
		if _, err := OpenPrivateLocal(context.Background(), privateInspection(root, q)); err == nil {
			t.Fatal("sealed identity mismatch accepted")
		}
	}
}
func TestPrivateLocalTreeRejectsAliasesAndUnrecognizedPayload(t *testing.T) {
	for name, change := range map[string]func(*testing.T, string){
		"extra_file": func(t *testing.T, r string) { privateWrite(t, filepath.Join(r, "extra"), []byte("x")) },
		"extra_dir": func(t *testing.T, r string) {
			if err := os.Mkdir(filepath.Join(r, "extra"), 0755); err != nil {
				t.Fatal(err)
			}
		},
		"hardlink": func(t *testing.T, r string) {
			if err := os.Link(filepath.Join(r, "manifest.json"), filepath.Join(filepath.Dir(r), "alias")); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(t *testing.T, r string) {
			path := filepath.Join(r, "manifest.json")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("qualification.json", path); err != nil {
				t.Fatal(err)
			}
		},
		"writable": func(t *testing.T, r string) {
			if err := os.Chmod(filepath.Join(r, "manifest.json"), 0666); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root, q := privateTinyTree(t)
			root = filepath.Join(root, "office-private")
			change(t, root)
			if _, err := privateTree(context.Background(), root, q.Files); err == nil {
				t.Fatal("unsafe tree accepted")
			}
		})
	}
	root, q := privateTinyTree(t)
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "office-private"), alias); err != nil {
		t.Fatal(err)
	}
	if _, err := privateTree(context.Background(), alias, q.Files); err == nil {
		t.Fatal("symlink root accepted")
	}
}

func TestPrivateLocalPayloadDirectoryPermissions(t *testing.T) {
	for _, relative := range []string{".", "assets/fonts", "plugins/analytix-documents/skills/documents"} {
		for _, mode := range []os.FileMode{0775, 0777} {
			t.Run(relative+"/"+mode.String(), func(t *testing.T) {
				resources, q := privateTinyTree(t)
				root := filepath.Join(resources, "office-private")
				if err := os.Chmod(filepath.Join(root, relative), mode); err != nil {
					t.Fatal(err)
				}
				if _, err := privateTree(context.Background(), root, q.Files); err == nil {
					t.Fatal("initial writable payload directory accepted")
				}
			})
		}
	}
	resources, q := privateTinyTree(t)
	if err := os.Chmod(resources, 0777); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(resources, "office-private")
	if _, err := privateTree(context.Background(), root, q.Files); err != nil {
		t.Fatal("legitimate host ancestor rejected", err)
	}
	if _, _, err := privateRead(context.Background(), filepath.Join(root, "qualification.json"), privateMaxQualification, nil); err != nil {
		t.Fatal("host ancestor rejected during held read", err)
	}
}

type privateMutatingWriter struct{ mutate func() }

func (w *privateMutatingWriter) Write(b []byte) (int, error) {
	if w.mutate != nil {
		f := w.mutate
		w.mutate = nil
		f()
	}
	return len(b), nil
}
func TestPrivateLocalHeldDescriptorDetectsMidReadMutation(t *testing.T) {
	for _, kind := range []string{"hardlink", "replacement", "write_restored_mtime", "parent_replacement"} {
		t.Run(kind, func(t *testing.T) {
			root := privateTestRoot(t)
			path := filepath.Join(root, "input", "file")
			privateWrite(t, path, []byte("synthetic"))
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			sink := &privateMutatingWriter{mutate: func() {
				switch kind {
				case "hardlink":
					err = os.Link(path, filepath.Join(root, "alias"))
				case "replacement":
					err = os.Rename(path, filepath.Join(root, "old"))
					if err == nil {
						privateWrite(t, path, []byte("synthetic"))
					}
				case "write_restored_mtime":
					privateWrite(t, path, []byte("different"))
					err = os.Chtimes(path, time.Now(), before.ModTime())
				case "parent_replacement":
					err = os.Rename(filepath.Dir(path), filepath.Join(root, "old"))
					if err == nil {
						privateWrite(t, path, []byte("synthetic"))
					}
				}
				if err != nil {
					t.Fatal(err)
				}
			}}
			if _, _, err := privateRead(context.Background(), path, 100, sink); err == nil {
				t.Fatal("concurrent mutation accepted")
			}
		})
	}
}

// Construct a private stat-witness directly to exercise Current without
// claiming admission of tiny engine bytes; the public Open still rejects it.
func privateStatFixture(t *testing.T) *PrivateLocal {
	t.Helper()
	root, q := privateTinyTree(t)
	root = filepath.Join(root, "office-private")
	dirs, err := privateTree(context.Background(), root, q.Files)
	if err != nil {
		t.Fatal(err)
	}
	p := &PrivateLocal{root: root, qualification: q, directories: dirs}
	paths := []string{"qualification.json"}
	for _, f := range q.Files {
		paths = append(paths, f.Path)
	}
	for _, path := range paths {
		o, _, err := privateRead(context.Background(), filepath.Join(root, filepath.FromSlash(path)), privateMaxQualification, nil)
		if err != nil {
			t.Fatal(err)
		}
		p.files = append(p.files, o)
	}
	return p
}
func TestPrivateLocalCurrentBindsAllFilesAndDirectories(t *testing.T) {
	for name, change := range map[string]func(*testing.T, *PrivateLocal){
		"root_chmod": func(t *testing.T, p *PrivateLocal) {
			if err := os.Chmod(p.root, 0775); err != nil {
				t.Fatal(err)
			}
		},
		"subdir_chmod": func(t *testing.T, p *PrivateLocal) {
			if err := os.Chmod(filepath.Join(p.root, "assets/fonts"), 0777); err != nil {
				t.Fatal(err)
			}
		},
		"subdir_rename": func(t *testing.T, p *PrivateLocal) {
			path := filepath.Join(p.root, "assets/fonts")
			if err := os.Rename(path, filepath.Join(filepath.Dir(p.root), "old-fonts")); err != nil {
				t.Fatal(err)
			}
			privateWrite(t, filepath.Join(path, "NotoSansCJKsc-Regular.otf"), []byte("synthetic"))
		},
		"plugin_write": func(t *testing.T, p *PrivateLocal) {
			privateWrite(t, filepath.Join(p.root, "plugins/analytix-documents/assets/adapter.json"), []byte("different"))
		},
		"asset_replace": func(t *testing.T, p *PrivateLocal) {
			path := filepath.Join(p.root, "assets/soffice.js")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			privateWrite(t, path, []byte("synthetic"))
		},
		"qualification": func(t *testing.T, p *PrivateLocal) {
			privateWrite(t, filepath.Join(p.root, "qualification.json"), []byte("different"))
		},
		"extra": func(t *testing.T, p *PrivateLocal) { privateWrite(t, filepath.Join(p.root, "extra"), []byte("x")) },
		"root": func(t *testing.T, p *PrivateLocal) {
			if err := os.Rename(p.root, p.root+"-old"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(p.root, 0755); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := privateStatFixture(t)
			if !p.Current(context.Background()) {
				t.Fatal("initial stat witness rejected")
			}
			if p.Root() == "" || p.AssetRoot() != filepath.Join(p.Root(), "assets") || p.QualificationDigest() == "" {
				t.Fatal("getter mismatch")
			}
			change(t, p)
			if p.Current(context.Background()) {
				t.Fatal("changed witness accepted")
			}
		})
	}
	var empty *PrivateLocal
	if empty.Current(context.Background()) || empty.Root() != "" || empty.AssetRoot() != "" || empty.QualificationDigest() != "" {
		t.Fatal("nil witness usable")
	}
	p := privateStatFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p.Current(ctx) || p.Current(nil) {
		t.Fatal("canceled witness usable")
	}
}
