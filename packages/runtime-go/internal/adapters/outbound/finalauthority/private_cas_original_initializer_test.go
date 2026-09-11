//go:build darwin || linux || windows

package finalauthority

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestOriginalLeafInitializationReopensEveryInterruptedDirectoryPrefix(t *testing.T) {
	for cut := 1; cut <= 6; cut++ {
		t.Run(fmt.Sprintf("created-%d", cut), func(t *testing.T) {
			ctx := context.Background()
			data, names, proof := originalCreateResidueFixtureV1(t, false)
			access := proof.prepared.access
			root := filepath.Join(data, "private", "attachment-authority")
			before, err := os.Stat(names[0])
			if err != nil {
				t.Fatal(err)
			}
			leaves := []string{"owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"}
			if store, err := OpenSecurePrivateCASWithAccessAuthorityContext(ctx, filepath.Join(root, "owners"), 4096, access); err == nil {
				_ = store.Close()
				t.Fatal("ordinary opening consumed original owner creation residue")
			}
			cause := errors.New("synthetic interruption after canonical directory sync")
			created := 0
			var interrupted error
			for _, leaf := range leaves {
				interrupted = proof.initializeLeafDirectoriesV1(ctx, filepath.Join(root, leaf), access, func(string) error {
					created++
					if created == cut {
						return cause
					}
					return nil
				})
				if interrupted != nil {
					break
				}
			}
			if !errors.Is(interrupted, cause) || created != cut {
				t.Fatalf("directory prefix did not reach exact interruption: created=%d err=%v", created, interrupted)
			}
			for i, leaf := range leaves {
				_, err := os.Stat(filepath.Join(root, leaf))
				if i < cut-1 && err != nil || i >= cut-1 && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected directory prefix at %s: %v", leaf, err)
				}
			}
			physical, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, data, access)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := physical.OriginalCreateResiduesV1(ctx, "private/attachment-authority")
			if err != nil {
				t.Fatal(err)
			}
			for _, leaf := range leaves {
				if err := fresh.InitializeLeafDirectoriesV1(ctx, filepath.Join(root, leaf), access); err != nil {
					t.Fatalf("fresh original partial observation blocked independent continuation: %v", err)
				}
				entries, err := os.ReadDir(filepath.Join(root, leaf))
				if err != nil || len(entries) != 0 {
					t.Fatalf("directory initializer created record or residue: %v", err)
				}
			}
			after, err := os.Stat(names[0])
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("interrupted initialization changed original create residue: %v", err)
			}
			if err := proof.Revalidate(ctx); err != nil {
				t.Fatalf("initial immutable proof was replaced by fresh initialization: %v", err)
			}
		})
	}
}

func TestOriginalLeafInitializationRetainsExactAuthorityAndDriftRefusals(t *testing.T) {
	for _, kind := range []string{"denied write", "canceled", "foreign leaf", "nonempty original", "replaced original"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			data, names, proof := originalCreateResidueFixtureV1(t, false)
			access := proof.prepared.access
			root := filepath.Join(data, "private", "attachment-authority", "owners")
			var want error
			switch kind {
			case "denied write":
				want = errors.New("synthetic exact leaf write authority denied")
				access = originalCreateDeniedWriteAuthorityV1{SecurePrivateCASRecoveryAccessAuthority: access, cause: want}
			case "canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx, want = canceled, context.Canceled
			case "foreign leaf":
				root = filepath.Join(data, "private", "pending-work", "receipts")
			case "nonempty original":
				if err := os.WriteFile(filepath.Join(names[0], "unexpected"), []byte("synthetic"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "replaced original":
				if err := os.Rename(names[0], filepath.Join(data, "saved-original")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(names[0], 0o700); err != nil {
					t.Fatal(err)
				}
			}
			err := proof.InitializeLeafDirectoriesV1(ctx, root, access)
			if err == nil || want != nil && !errors.Is(err, want) {
				t.Fatalf("initialization lost required refusal/cause: %v", err)
			}
			for _, owner := range []string{"attachment-authority", "pending-work"} {
				if _, err := os.Stat(filepath.Join(data, "private", owner)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("rejected initialization created canonical owner %s: %v", owner, err)
				}
			}
		})
	}
}

func TestOriginalLeafInitializationRejectsReboundCreatedOwnerBeforeLeafWrite(t *testing.T) {
	ctx := context.Background()
	data, names, proof := originalCreateResidueFixtureV1(t, false)
	owner := filepath.Join(data, "private", "attachment-authority")
	moved := filepath.Join(data, "saved-owner")
	before, err := os.Stat(names[0])
	if err != nil {
		t.Fatal(err)
	}
	rebound := false
	err = proof.initializeLeafDirectoriesV1(ctx, filepath.Join(owner, "owners"), proof.prepared.access, func(name string) error {
		if name != "private/attachment-authority" {
			return nil
		}
		if err := os.Rename(owner, moved); err != nil {
			return err
		}
		if err := os.Mkdir(owner, 0o700); err != nil {
			return err
		}
		rebound = true
		return nil
	})
	if !rebound {
		t.Fatalf("fixture did not rebind newly created owner: %v", err)
	}
	if err == nil {
		t.Error("initialization accepted a newly created owner that no longer names the opened directory")
	}
	for _, root := range []string{owner, moved} {
		if _, err := os.Stat(filepath.Join(root, "owners")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("initialization wrote a leaf after owner identity changed: %v", err)
		}
	}
	after, statErr := os.Stat(names[0])
	if statErr != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("rebound owner changed original create residue: %v", statErr)
	}
}
