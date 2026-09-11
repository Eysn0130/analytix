//go:build darwin || linux

package secureconfigfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadExactAcceptsOnlyStablePrivateInventory(t *testing.T) {
	root := privateRoot(t)
	writePrivate(t, filepath.Join(root, "manifest.json"), []byte(`{"schemaVersion":1}`))
	body, err := readExactOnManagedTestFilesystem(ReadExactInput{
		Root: root, Target: "manifest.json", AllowedNames: []string{"manifest.json"}, MaxBytes: 1024,
	})
	if err != nil || string(body) != `{"schemaVersion":1}` {
		t.Fatalf("secure exact read failed: body=%q err=%v", body, err)
	}
}

func TestReadBundleAcceptsOnlyStablePrivateInventory(t *testing.T) {
	root := privateRoot(t)
	writePrivate(t, filepath.Join(root, "authority.json"), []byte(`{"schemaVersion":1}`))
	writePrivate(t, filepath.Join(root, "client.crt"), []byte("certificate"))
	writePrivate(t, filepath.Join(root, "client.key"), []byte("private-key"))
	files, err := readBundleOnManagedTestFilesystem(ReadBundleInput{Root: root, Files: []BundleFile{
		{Name: "client.key", MaxBytes: 1024},
		{Name: "authority.json", MaxBytes: 1024},
		{Name: "client.crt", MaxBytes: 1024},
	}, MaxTotalBytes: 4096})
	if err != nil {
		t.Fatalf("secure bundle read failed: %v", err)
	}
	if string(files["authority.json"]) != `{"schemaVersion":1}` ||
		string(files["client.crt"]) != "certificate" || string(files["client.key"]) != "private-key" {
		t.Fatalf("secure bundle returned unexpected bodies: %#v", files)
	}
}

func TestReadBundleRejectsInvalidOrInexactInventory(t *testing.T) {
	root := privateRoot(t)
	writePrivate(t, filepath.Join(root, "authority.json"), []byte("authority"))
	writePrivate(t, filepath.Join(root, "client.crt"), []byte("certificate"))
	tests := map[string]ReadBundleInput{
		"missing file declaration": {
			Root: root, Files: []BundleFile{{Name: "authority.json", MaxBytes: 1024}}, MaxTotalBytes: 1024,
		},
		"duplicate file declaration": {
			Root: root, Files: []BundleFile{{Name: "authority.json", MaxBytes: 1024}, {Name: "authority.json", MaxBytes: 1024}}, MaxTotalBytes: 2048,
		},
		"unsafe name": {
			Root: root, Files: []BundleFile{{Name: "../authority.json", MaxBytes: 1024}}, MaxTotalBytes: 1024,
		},
		"invalid bound": {
			Root: root, Files: []BundleFile{{Name: "authority.json", MaxBytes: 0}}, MaxTotalBytes: 1024,
		},
		"file bound exceeds total": {
			Root: root, Files: []BundleFile{{Name: "authority.json", MaxBytes: 2048}}, MaxTotalBytes: 1024,
		},
		"missing total bound": {
			Root: root, Files: []BundleFile{{Name: "authority.json", MaxBytes: 1024}},
		},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := readBundleOnManagedTestFilesystem(input); err == nil {
				t.Fatal("invalid secure bundle inventory was accepted")
			}
		})
	}
}

func TestReadBundleEnforcesAggregateSizeBound(t *testing.T) {
	root := privateRoot(t)
	writePrivate(t, filepath.Join(root, "a"), []byte("1234"))
	writePrivate(t, filepath.Join(root, "b"), []byte("5678"))
	_, err := readBundleOnManagedTestFilesystem(ReadBundleInput{
		Root:          root,
		Files:         []BundleFile{{Name: "a", MaxBytes: 7}, {Name: "b", MaxBytes: 7}},
		MaxTotalBytes: 7,
	})
	if err == nil {
		t.Fatal("aggregate secure bundle size limit was not enforced")
	}
}

func TestReadExactRejectsUnvalidatedCompanionInventory(t *testing.T) {
	root := privateRoot(t)
	writePrivate(t, filepath.Join(root, "manifest.json"), []byte("manifest"))
	target := filepath.Join(t.TempDir(), "ca.pem")
	writePrivate(t, target, []byte("ca"))
	if err := os.Symlink(target, filepath.Join(root, "ca.pem")); err != nil {
		t.Fatalf("create companion symlink: %v", err)
	}
	_, err := readExactOnManagedTestFilesystem(ReadExactInput{
		Root: root, Target: "manifest.json", AllowedNames: []string{"manifest.json", "ca.pem"}, MaxBytes: 1024,
	})
	if err == nil {
		t.Fatal("exact read accepted an unvalidated companion inventory entry")
	}
}

func TestReadExactRejectsUnsafeFilesystemShapes(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"unknown inventory": func(t *testing.T, root string) {
			writePrivate(t, filepath.Join(root, "extra.json"), []byte("extra"))
		},
		"wide file mode": func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "manifest.json"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"hardlink": func(t *testing.T, root string) {
			if err := os.Link(filepath.Join(root, "manifest.json"), filepath.Join(t.TempDir(), "manifest-link.json")); err != nil {
				t.Fatalf("create hardlink: %v", err)
			}
		},
		"target symlink": func(t *testing.T, root string) {
			target := filepath.Join(t.TempDir(), "real.json")
			writePrivate(t, target, []byte("real"))
			if err := os.Remove(filepath.Join(root, "manifest.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(root, "manifest.json")); err != nil {
				t.Fatalf("create target symlink: %v", err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := privateRoot(t)
			writePrivate(t, filepath.Join(root, "manifest.json"), []byte("manifest"))
			mutate(t, root)
			if _, err := readExactOnManagedTestFilesystem(ReadExactInput{
				Root: root, Target: "manifest.json", AllowedNames: []string{"manifest.json"}, MaxBytes: 1024,
			}); err == nil {
				t.Fatal("unsafe configuration filesystem shape was accepted")
			}
		})
	}
}

func TestReadExactRejectsSymlinkedRoot(t *testing.T) {
	realRoot := privateRoot(t)
	writePrivate(t, filepath.Join(realRoot, "manifest.json"), []byte("manifest"))
	link := filepath.Join(t.TempDir(), "config-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatalf("create root symlink: %v", err)
	}
	_, err := readExactOnManagedTestFilesystem(ReadExactInput{
		Root: link, Target: "manifest.json", AllowedNames: []string{"manifest.json"}, MaxBytes: 1024,
	})
	if err == nil || errors.Is(err, ErrUnsupported) {
		t.Fatalf("symlinked root was not rejected securely: %v", err)
	}
}

func TestReadExactFailsClosedWhenFilesystemAuthorityIsUnavailableOrChanges(t *testing.T) {
	root := privateRoot(t)
	writePrivate(t, filepath.Join(root, "manifest.json"), []byte("manifest"))
	input := ReadExactInput{
		Root: root, Target: "manifest.json", AllowedNames: []string{"manifest.json"}, MaxBytes: 1024,
	}
	t.Run("probe error", func(t *testing.T) {
		reader := func(normalized normalizedBundle) (map[string][]byte, error) {
			return readBundleWithFilesystemProbe(normalized, func(int) (secureFilesystemIdentity, error) {
				return secureFilesystemIdentity{}, errors.New("filesystem attestation unavailable")
			})
		}
		if _, err := readExactWith(input, reader); err == nil {
			t.Fatal("missing filesystem authority was accepted")
		}
	})
	t.Run("root and file identity drift", func(t *testing.T) {
		calls := 0
		reader := func(normalized normalizedBundle) (map[string][]byte, error) {
			return readBundleWithFilesystemProbe(normalized, func(int) (secureFilesystemIdentity, error) {
				calls++
				return secureFilesystemIdentity{kind: "test-managed", fsid: [2]int64{1, int64(calls)}}, nil
			})
		}
		if _, err := readExactWith(input, reader); err == nil {
			t.Fatal("filesystem identity drift was accepted")
		}
	})
	t.Run("nil reader", func(t *testing.T) {
		if _, err := readExactWith(input, nil); err == nil {
			t.Fatal("nil filesystem reader was accepted")
		}
	})
}

func privateRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return realRoot
}

func writePrivate(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readExactOnManagedTestFilesystem(input ReadExactInput) ([]byte, error) {
	return readExactWith(input, managedTestBundleReader)
}

func readBundleOnManagedTestFilesystem(input ReadBundleInput) (map[string][]byte, error) {
	return readBundleWith(input, managedTestBundleReader)
}

func managedTestBundleReader(input normalizedBundle) (map[string][]byte, error) {
	return readBundleWithFilesystemProbe(input, managedTestFilesystemProbe)
}

func managedTestFilesystemProbe(int) (secureFilesystemIdentity, error) {
	return secureFilesystemIdentity{kind: "test-managed", fsid: [2]int64{1, 1}}, nil
}
