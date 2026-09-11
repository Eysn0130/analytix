//go:build darwin || linux || windows

package finalauthority

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func emptyOwnerProofTreeV1(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[path] = "directory"
			return nil
		}
		body, err := os.ReadFile(path)
		result[path] = "file:" + string(body)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEmptyPartialOwnerProofRequiresExactRecursiveEmptyOwner(t *testing.T) {
	for _, mode := range []string{"partial", "empty_owner", "nonempty", "unknown_leaf", "aliased_leaf", "leaf_file", "complete", "absent", "other_owner_only", "wrong_group"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ancestor_mode" && runtime.GOOS == "windows" {
				t.Skip("Unix mode metadata")
			}
			data := t.TempDir()
			owner := filepath.Join(data, "private", "pending-work")
			mkdir := func(path string) {
				t.Helper()
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			mkdir(filepath.Join(data, "private"))
			switch mode {
			case "absent", "other_owner_only":
			case "empty_owner":
				mkdir(owner)
			case "leaf_file":
				mkdir(owner)
				write(filepath.Join(owner, "receipts"))
			default:
				mkdir(filepath.Join(owner, "receipts", "ab"))
			}
			switch mode {
			case "nonempty":
				write(filepath.Join(owner, "receipts", "ab", "record.json"))
			case "unknown_leaf":
				mkdir(filepath.Join(owner, "unknown"))
			case "aliased_leaf":
				mkdir(filepath.Join(owner, "Dispositions"))
			case "complete":
				mkdir(filepath.Join(owner, "dispositions"))
			case "other_owner_only":
				mkdir(filepath.Join(data, "private", "accepted-finals", "records", "ab"))
			}
			// A legitimate create residue in another owner is outside this proof.
			mkdir(filepath.Join(data, "private", "report-publication", domainprivatecas.CreateDirectoryResidueNameV1("artifacts")))
			access, err := privatecastest.NewAccessAuthority(filepath.Join(data, "private"))
			if err != nil {
				t.Fatal(err)
			}
			before := emptyOwnerProofTreeV1(t, data)
			group := "pending-work"
			if mode == "wrong_group" {
				group = "../pending-work"
			}
			proof, err := PrepareEmptyPartialOwnerProofV1(context.Background(), data, group, access)
			valid := mode == "partial" || mode == "empty_owner"
			ineligible := mode == "complete" || mode == "absent" || mode == "other_owner_only"
			if !valid && !ineligible && err == nil {
				t.Fatal("invalid owner probe did not return an error")
			}
			if ineligible && err != nil {
				t.Fatalf("ordinary owner did not continue to its normal preparation: %v", err)
			}
			if valid != (err == nil && proof != nil) || (!valid && proof != nil) {
				t.Fatalf("empty partial owner proof result: %v", err)
			}
			if valid {
				if err := proof.Revalidate(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err := proof.prepared.Apply(context.Background()); err == nil {
					t.Fatal("proof acquired mutation authority")
				}
			}
			if !reflect.DeepEqual(before, emptyOwnerProofTreeV1(t, data)) {
				t.Fatal("empty proof changed owner or unrelated recovery material")
			}
		})
	}
}

func TestEmptyPartialOwnerProofRevalidatesShardIdentityAndCompleteDenominator(t *testing.T) {
	for _, mode := range []string{"new_record", "replace_empty_shard", "complete_owner", "ancestor_mode", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ancestor_mode" && runtime.GOOS == "windows" {
				t.Skip("Unix mode metadata")
			}
			data := t.TempDir()
			owner := filepath.Join(data, "private", "pending-work")
			shard := filepath.Join(owner, "receipts", "ab")
			if err := os.MkdirAll(shard, 0o700); err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Join(data, "private"))
			if err != nil {
				t.Fatal(err)
			}
			proof, err := PrepareEmptyPartialOwnerProofV1(context.Background(), data, "pending-work", access)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch mode {
			case "new_record":
				err = os.WriteFile(filepath.Join(shard, "record.json"), []byte("synthetic"), 0o600)
			case "replace_empty_shard":
				err = os.Rename(shard, filepath.Join(t.TempDir(), "preserved-shard"))
				if err == nil {
					err = os.Mkdir(shard, 0o700)
				}
			case "ancestor_mode":
				err = os.Chmod(filepath.Join(data, "private"), 0o500)
				t.Cleanup(func() { _ = os.Chmod(filepath.Join(data, "private"), 0o700) })
			case "complete_owner":
				err = os.Mkdir(filepath.Join(owner, "dispositions"), 0o700)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			before := emptyOwnerProofTreeV1(t, data)
			if err := proof.Revalidate(ctx); err == nil {
				t.Fatal("stale or cancelled empty proof remained usable")
			}
			if !reflect.DeepEqual(before, emptyOwnerProofTreeV1(t, data)) {
				t.Fatal("failed proof revalidation changed state")
			}
		})
	}
}
