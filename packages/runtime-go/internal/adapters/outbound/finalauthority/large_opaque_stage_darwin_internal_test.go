//go:build darwin

package finalauthority

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLargeOpaqueDarwinPinnedSourceSurvivesCallerCloseAndDescriptorReuse(t *testing.T) {
	root := t.TempDir()
	body := []byte("darwin-pinned-exact-source")
	sourcePath := filepath.Join(root, "source.duckdb")
	if err := os.WriteFile(sourcePath, body, 0o400); err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	originalFD := int(source.Fd())
	var original unix.Stat_t
	if err := unix.Fstat(originalFD, &original); err != nil {
		t.Fatal(err)
	}

	pinnedFD, err := largeOpaqueDarwinPinSource(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(pinnedFD) })
	if pinnedFD < largeOpaqueDarwinMinimumPinnedFD {
		t.Fatalf("pinned descriptor=%d, want >=%d", pinnedFD, largeOpaqueDarwinMinimumPinnedFD)
	}
	flags, err := unix.FcntlInt(uintptr(pinnedFD), unix.F_GETFD, 0)
	if err != nil {
		t.Fatal(err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("pinned descriptor is not close-on-exec")
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	reusedFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(reusedFD) })
	if reusedFD != originalFD {
		t.Fatalf("closed caller descriptor=%d was not reused by directory descriptor=%d", originalFD, reusedFD)
	}
	var reused unix.Stat_t
	if err := unix.Fstat(reusedFD, &reused); err != nil {
		t.Fatal(err)
	}
	if reused.Mode&unix.S_IFMT != unix.S_IFDIR {
		t.Fatal("reused caller descriptor is not a directory")
	}
	var pinned unix.Stat_t
	if err := unix.Fstat(pinnedFD, &pinned); err != nil {
		t.Fatal(err)
	}
	if pinned.Mode&unix.S_IFMT != unix.S_IFREG || pinned.Dev != original.Dev || pinned.Ino != original.Ino {
		t.Fatal("pinned descriptor no longer identifies the exact regular source")
	}

	destinationPath := filepath.Join(root, "destination")
	if err := os.Mkdir(destinationPath, 0o700); err != nil {
		t.Fatal(err)
	}
	destinationFD, err := unix.Open(destinationPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(destinationFD) })
	if err := unix.Fclonefileat(pinnedFD, destinationFD, "clone.duckdb", unix.CLONE_NOOWNERCOPY); err != nil {
		t.Fatal(err)
	}
	cloned, err := os.ReadFile(filepath.Join(destinationPath, "clone.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cloned, body) {
		t.Fatal("clone did not use the pinned exact source")
	}
}
