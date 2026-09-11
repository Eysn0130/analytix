//go:build linux

package finalauthority

import (
	"os/exec"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSecurePrivateCASRejectsPOSIXACLAtEveryAuthorityLevel(t *testing.T) {
	setfacl, err := exec.LookPath("setfacl")
	if err != nil {
		t.Skip("setfacl is unavailable on this Linux host")
	}
	for _, target := range []string{"root", "shard", "record"} {
		t.Run(target, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private-cas")
			store, err := openTestSecurePrivateCAS(t, root, 4096)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("cas-posix-acl:" + target))
			if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
				t.Fatal(err)
			}
			path := root
			switch target {
			case "shard":
				path = filepath.Join(root, digest[:2])
			case "record":
				path = filepath.Join(root, digest[:2], digest+".json")
			}
			if output, err := exec.Command(setfacl, "-m", "u:65534:rwx", path).CombinedOutput(); err != nil {
				t.Fatalf("private CAS POSIX ACL fixture: %v: %s", err, output)
			}
			if _, err := store.Read(nil, digest); err == nil {
				t.Fatalf("POSIX ACL on %s was accepted", target)
			}
		})
	}
}
