//go:build darwin

package finalauthority

import (
	"os/exec"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSecurePrivateCASRejectsExtendedACLAtEveryAuthorityLevel(t *testing.T) {
	for _, target := range []string{"root", "shard", "record"} {
		t.Run(target, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private-cas")
			store, err := openTestSecurePrivateCAS(t, root, 4096)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("cas-acl:" + target))
			if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
				t.Fatal(err)
			}
			path := root
			permission := "everyone allow list,search,add_file,delete_child"
			switch target {
			case "shard":
				path = filepath.Join(root, digest[:2])
			case "record":
				path = filepath.Join(root, digest[:2], digest+".json")
				permission = "everyone allow read,write"
			}
			if output, err := exec.Command("chmod", "+a", permission, path).CombinedOutput(); err != nil {
				t.Fatalf("private CAS ACL fixture: %v: %s", err, output)
			}
			if _, err := store.Read(nil, digest); err == nil {
				t.Fatalf("extended ACL on %s was accepted", target)
			}
		})
	}
}
