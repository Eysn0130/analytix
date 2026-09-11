//go:build darwin || linux || windows

package finalauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestPrivateCASOriginalOrphanPreservationKeepsExactCandidateSet(t *testing.T) {
	for _, scenario := range []string{"complete owner", "empty partial owner", "late proof failure", "candidate drift", "unknown owner"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			data := t.TempDir()
			owner := filepath.Join(data, "private", "attachment-authority")
			leaves := []string{"owners"}
			if scenario != "empty partial owner" {
				leaves = append(leaves, "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions")
			}
			for _, leaf := range leaves {
				if err := os.MkdirAll(filepath.Join(owner, leaf), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			held := filepath.Join(owner, "owners", "ab")
			independent := filepath.Join(data, "private", "case-thread-authority", "cd")
			for _, name := range []string{held, independent} {
				if err := os.MkdirAll(name, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			authority, err := privatecastest.NewAccessAuthority(filepath.Join(data, "private"))
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareSecurePrivateCASOrphanTopologyRecoveryV1(ctx, data, authority)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.CandidateCount() < 2 {
				t.Fatal("fixture lacks both original and independent candidates")
			}
			before, err := os.Stat(held)
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New("synthetic original proof unavailable")
			calls := 0
			revalidate := func(context.Context) error {
				calls++
				if scenario == "late proof failure" {
					return cause
				}
				return nil
			}
			roots := []string{"private/attachment-authority", "private/attachment-authority/owners", "private/attachment-authority/owners/ab"}
			if scenario == "unknown owner" {
				roots = []string{"private/not-a-known-owner"}
			}
			if scenario == "candidate drift" {
				if err := os.WriteFile(filepath.Join(held, "unexpected"), []byte("unobserved"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err = prepared.ApplyPreservingOriginalDirectoriesV1(ctx, roots, revalidate)
			wantPass := scenario == "complete owner" || scenario == "empty partial owner"
			if (err == nil) != wantPass {
				t.Fatalf("original orphan preservation: %v", err)
			}
			if scenario == "late proof failure" && !errors.Is(err, cause) {
				t.Fatalf("lost pre-effect proof cause: %v", err)
			}
			if wantPass && calls != 1 {
				t.Fatalf("preservation proof was not checked before effects: %d", calls)
			}
			after, statErr := os.Stat(held)
			if statErr != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("original candidate changed: %v", statErr)
			}
			_, independentErr := os.Stat(independent)
			if wantPass && !os.IsNotExist(independentErr) {
				t.Fatalf("independent candidate remained: %v", independentErr)
			}
			if !wantPass && independentErr != nil {
				t.Fatalf("rejected proof reached independent deletion: %v", independentErr)
			}
		})
	}
}
