//go:build darwin

package finalauthority

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenExistingFileAuthorityRejectsExtendedACLWithoutMutation(t *testing.T) {
	t.Run("key", func(t *testing.T) {
		path := existingEnrolledAuthorityPath(t)
		if output, err := exec.Command("chmod", "+a", "everyone allow read,write", path).CombinedOutput(); err != nil {
			t.Fatalf("install key ACL fixture: %v: %s", err, output)
		}
		assertExistingAuthorityRejected(t, path)
	})

	t.Run("root", func(t *testing.T) {
		path := existingEnrolledAuthorityPath(t)
		root := filepath.Dir(path)
		if output, err := exec.Command("chmod", "+a", "everyone allow list,search,add_file,delete_child", root).CombinedOutput(); err != nil {
			t.Fatalf("install root ACL fixture: %v: %s", err, output)
		}
		assertExistingAuthorityRejected(t, path)
	})
}
