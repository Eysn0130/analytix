package fundscsvsource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadExactAcceptsCanonicalContainedRegularCSV(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "transactions.csv")
	body := []byte("a,b\r\n1,2\r\n3,4\r\n")
	if err := os.WriteFile(source, body, 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	err = ReadExactV1(context.Background(), workspace, source, func(
		_ context.Context,
		resolvedWorkspace string,
		read []byte,
		digest string,
		rows uint64,
	) error {
		called = true
		if resolvedWorkspace != workspace || string(read) != string(body) || len(digest) != 64 || rows != 2 {
			t.Fatal("contained CSV identity changed")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("contained CSV read failed: called=%t err=%v", called, err)
	}
}

func TestReadImportExactFreezesOwnerRegularIdentityAndRejectsReplacement(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "transactions.csv")
	body := []byte("a,b\r\n1,2\r\n")
	if err := os.WriteFile(source, body, 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	err = ReadImportExactV1(context.Background(), workspace, source, func(
		ctx context.Context,
		resolvedWorkspace string,
		format string,
		read []byte,
		digest string,
		revalidate func(context.Context, func(context.Context, []byte, string) error) error,
	) error {
		called = true
		if resolvedWorkspace != workspace || format != "csv" || string(read) != string(body) || len(digest) != 64 {
			t.Fatal("frozen source identity changed")
		}
		replacement := filepath.Join(workspace, "replacement.csv")
		if err := os.WriteFile(replacement, []byte("a,b\r\n3,4\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, source); err != nil {
			t.Fatal(err)
		}
		if err := revalidate(ctx, func(context.Context, []byte, string) error {
			t.Fatal("replacement reached confirmation callback")
			return nil
		}); err == nil {
			t.Fatal("source replacement was accepted")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("import source freeze failed: called=%t err=%v", called, err)
	}
}

func TestReadImportExactRejectsUnsupportedModeHardlinkAndFormat(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	use := func(
		context.Context,
		string,
		string,
		[]byte,
		string,
		func(context.Context, func(context.Context, []byte, string) error) error,
	) error {
		t.Fatal("unsafe source reached callback")
		return nil
	}
	worldReadable := filepath.Join(workspace, "world.csv")
	if err := os.WriteFile(worldReadable, []byte("a\r\n1\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(worldReadable, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReadImportExactV1(context.Background(), workspace, worldReadable, use); err == nil {
		t.Fatal("non-owner-only mode was accepted")
	}
	hardlinkSource := filepath.Join(workspace, "hardlink.csv")
	if err := os.WriteFile(hardlinkSource, []byte("a\r\n1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(hardlinkSource, filepath.Join(workspace, "alias.csv")); err != nil {
		t.Fatal(err)
	}
	if err := ReadImportExactV1(context.Background(), workspace, hardlinkSource, use); err == nil {
		t.Fatal("hardlinked source was accepted")
	}
	unsupported := filepath.Join(workspace, "transactions.xlsx")
	if err := os.WriteFile(unsupported, []byte(strings.Repeat("x", 8)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReadImportExactV1(context.Background(), workspace, unsupported, use); err == nil {
		t.Fatal("unsupported source format was accepted")
	}
}

func TestReadImportExactRejectsWorkspaceEscapeSymlinkDirectoryAndCancellation(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outsideRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideRoot, "outside.csv")
	if err := os.WriteFile(outside, []byte("a\r\n1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "linked.csv")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(workspace, "directory.csv")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	use := func(
		context.Context,
		string,
		string,
		[]byte,
		string,
		func(context.Context, func(context.Context, []byte, string) error) error,
	) error {
		t.Fatal("unsafe import source reached callback")
		return nil
	}
	for _, source := range []string{outside, link, directory} {
		if err := ReadImportExactV1(context.Background(), workspace, source, use); err == nil {
			t.Fatalf("unsafe import source was accepted: %s", source)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ReadImportExactV1(canceled, workspace, outside, use); err == nil {
		t.Fatal("canceled source acquisition continued")
	}
}

func TestReadImportExactRejectsCompressedSourceByteBudget(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "oversized.zip")
	file, err := os.OpenFile(source, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maximumSourceBytesV1 + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ReadImportExactV1(context.Background(), workspace, source, func(
		context.Context,
		string,
		string,
		[]byte,
		string,
		func(context.Context, func(context.Context, []byte, string) error) error,
	) error {
		t.Fatal("oversized source reached callback")
		return nil
	}); err == nil {
		t.Fatal("compressed source byte budget was not enforced")
	}
}

func TestReadImportExactRevalidationDetectsSameFileHashChange(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "transactions.csv")
	body := []byte("a,b\r\n1,2\r\n")
	if err := os.WriteFile(source, body, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	err = ReadImportExactV1(context.Background(), workspace, source, func(
		ctx context.Context,
		_ string,
		_ string,
		_ []byte,
		_ string,
		revalidate func(context.Context, func(context.Context, []byte, string) error) error,
	) error {
		if err := os.WriteFile(source, []byte("a,b\r\n3,4\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(source, info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
		if err := revalidate(ctx, func(context.Context, []byte, string) error {
			t.Fatal("same-file hash replacement reached confirmation callback")
			return nil
		}); err == nil {
			t.Fatal("same-file hash replacement was accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("initial frozen source failed: %v", err)
	}
}

func TestReadExactRejectsOutsideSymlinkAndMalformedRows(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outsideRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideRoot, "outside.csv")
	if err := os.WriteFile(outside, []byte("a\r\n1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "linked.csv")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	use := func(context.Context, string, []byte, string, uint64) error {
		t.Fatal("invalid source reached callback")
		return nil
	}
	if err := ReadExactV1(context.Background(), workspace, outside, use); err == nil {
		t.Fatal("outside CSV was accepted")
	}
	if err := ReadExactV1(context.Background(), workspace, link, use); err == nil {
		t.Fatal("symlinked CSV was accepted")
	}
	malformed := filepath.Join(workspace, "malformed.csv")
	if err := os.WriteFile(malformed, []byte("a,b\r\n1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReadExactV1(context.Background(), workspace, malformed, use); err == nil {
		t.Fatal("malformed CSV was accepted")
	}
}
