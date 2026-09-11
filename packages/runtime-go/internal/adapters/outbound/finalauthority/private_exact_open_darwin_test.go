//go:build darwin

package finalauthority

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinPrivateCASExactBoundDirectoryFastPathRejectsCaseAlias(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := unix.Open(parentPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parent)
	var stat unix.Stat_t
	if err := unix.Fstat(parent, &stat); err != nil {
		t.Fatal(err)
	}
	caseSensitive, err := unix.Fpathconf(parent, privateDarwinPathconfCaseSensitive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parentPath, "Canonical"), 0o700); err != nil {
		t.Fatal(err)
	}
	opened, present, handled, err := platformPrivateCASUnixOpenExactBoundDirectory(parent, "Canonical", uint64(stat.Dev))
	if caseSensitive == 0 {
		if err != nil || !present || !handled {
			t.Fatalf("case-insensitive fast path rejected exact component: present=%t handled=%t err=%v", present, handled, err)
		}
		if err := unix.Close(opened); err != nil {
			t.Fatal(err)
		}
		if alias, present, handled, err := platformPrivateCASUnixOpenExactBoundDirectory(parent, "canonical", uint64(stat.Dev)); err == nil || present || !handled {
			if alias >= 0 {
				_ = unix.Close(alias)
			}
			t.Fatalf("case-insensitive fast path accepted case alias: present=%t handled=%t err=%v", present, handled, err)
		}
	} else if err != nil || present || handled || opened >= 0 {
		if opened >= 0 {
			_ = unix.Close(opened)
		}
		t.Fatalf("case-sensitive volume did not defer to bounded scan: fd=%d present=%t handled=%t err=%v", opened, present, handled, err)
	}

	portable, present, err := privateCASUnixOpenExactBoundDirectory(parent, "Canonical", uint64(stat.Dev))
	if err != nil || !present {
		t.Fatalf("bounded exact-component scan rejected canonical name: present=%t err=%v", present, err)
	}
	if err := unix.Close(portable); err != nil {
		t.Fatal(err)
	}
	if alias, present, err := privateCASUnixOpenExactBoundDirectory(parent, "canonical", uint64(stat.Dev)); err == nil || present {
		if alias >= 0 {
			_ = unix.Close(alias)
		}
		t.Fatalf("bounded exact-component scan accepted case alias: present=%t err=%v", present, err)
	}
}
