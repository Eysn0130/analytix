//go:build darwin || linux || windows

package finalauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestPrivateCASRecoveryNeverRollsBackAfterCommitMarkerRename(t *testing.T) {
	for _, phase := range []string{"after_commit_marker_rename", "after_commit_marker_sync"} {
		t.Run(phase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private-cas")
			authority, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
			if err != nil {
				t.Fatal(err)
			}
			digest := "a7" + strings.Repeat("b", 62)
			body := []byte(`{"record":"commit-marker-rename"}`)
			if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			ordinary := filepath.Join(root, digest[:2], "."+digest+".json-0123456789abcdef01234567.tmp")
			if err := os.WriteFile(ordinary, []byte("rename-outcome-residue"), 0o600); err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
			if err != nil {
				t.Fatal(err)
			}
			setPrivateCASRecoveryV3TestHook(t, func(gotPhase string, _ int) error {
				if gotPhase == phase {
					return errors.New("commit-marker-outcome-cut")
				}
				return nil
			})
			if err := prepared.Apply(context.Background()); err == nil {
				t.Fatal("commit-marker outcome cut did not stop recovery")
			}
			if _, err := os.Lstat(ordinary); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("post-marker failure rolled the committed name back: %v", err)
			}
			if !privateCASRecoveryV3TestCommitMarkerPresent(t, filepath.Join(root, digest[:2])) {
				t.Fatal("post-marker failure lost the committed recovery marker")
			}

			setPrivateCASRecoveryV3TestHook(t, nil)
			resumed, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
			if err != nil {
				t.Fatal(err)
			}
			if err := resumed.Apply(context.Background()); err != nil {
				t.Fatalf("resume committed recovery: %v", err)
			}
			if privateCASRecoveryV3TestCommitMarkerPresent(t, filepath.Join(root, digest[:2])) {
				t.Fatal("resumed recovery retained the commit marker")
			}
			reopened, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			got, err := reopened.Read(context.Background(), digest)
			if err != nil || string(got) != string(body) {
				t.Fatalf("committed record changed across recovery resume: body=%q err=%v", got, err)
			}
		})
	}
}

func TestPrivateCASRecoveryV3RejectsCaseAliasedCrossAuthorityRoots(t *testing.T) {
	parent := t.TempDir()
	upper := &PreparedSecurePrivateCASRecoveryV1{rootPath: filepath.Join(parent, "Owner")}
	lower := &PreparedSecurePrivateCASRecoveryV1{rootPath: filepath.Join(parent, "owner")}
	authorities := []PreparedSecurePrivateCASRecoveryAuthorityV3{
		privateCASRecoveryV3TestAuthority{plans: []*PreparedSecurePrivateCASRecoveryV1{upper}},
		privateCASRecoveryV3TestAuthority{plans: []*PreparedSecurePrivateCASRecoveryV1{lower}},
	}
	if _, err := prepareSecurePrivateCASRecoveryAuthoritySetV3(context.Background(), authorities); err == nil {
		t.Fatal("V3 accepted case-aliased roots from separate recovery authorities")
	}
}

type privateCASRecoveryV3TestAuthority struct {
	plans []*PreparedSecurePrivateCASRecoveryV1
}

func (authority privateCASRecoveryV3TestAuthority) Revalidate(context.Context) error {
	return nil
}

func (authority privateCASRecoveryV3TestAuthority) SecurePrivateCASRecoveryPlansV2() []*PreparedSecurePrivateCASRecoveryV1 {
	return authority.plans
}

func (privateCASRecoveryV3TestAuthority) PrivateCASRecoveryTopologiesV3() []SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return nil
}

func privateCASRecoveryV3TestCommitMarkerPresent(t *testing.T, shard string) bool {
	t.Helper()
	entries, err := os.ReadDir(shard)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), privateCASRecoveryCommitPrefix) {
			return true
		}
	}
	return false
}

func setPrivateCASRecoveryV3TestHook(t *testing.T, hook func(string, int) error) {
	t.Helper()
	privateCASRecoveryTransactionTestHooks.Lock()
	privateCASRecoveryTransactionTestHooks.hook = hook
	privateCASRecoveryTransactionTestHooks.Unlock()
	if hook != nil {
		t.Cleanup(func() {
			privateCASRecoveryTransactionTestHooks.Lock()
			privateCASRecoveryTransactionTestHooks.hook = nil
			privateCASRecoveryTransactionTestHooks.Unlock()
		})
	}
}
