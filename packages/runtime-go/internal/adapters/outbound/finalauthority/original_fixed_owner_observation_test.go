//go:build darwin || linux || windows

package finalauthority

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOriginalFixedOwnerObservationRetainsPartialPhysicalLeaves(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	leaves := []SecurePrivateCASOwnerLeafV1{{Name: "a", MaxBytes: 4096}, {Name: "b", MaxBytes: 4096}}
	for count := -1; count <= 2; count++ {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(fixture.authorityRoot, fmt.Sprintf("original-prefix-%d", count))
			if count >= 0 {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < count; i++ {
				if err := os.Mkdir(filepath.Join(root, leaves[i].Name), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			observed, err := PrepareOriginalFixedOwnerObservationV1(ctx, root, leaves, fixture.access)
			if err != nil {
				t.Fatal(err)
			}
			files, err := observed.SnapshotOriginalFilesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]SecurePrivateCASOriginalEntryV1{}
			if count >= 0 {
				want = originalSnapshotExpectedTreeV1(t, root)
			}
			if !reflect.DeepEqual(files, want) {
				t.Fatal("partial original files were invented or lost")
			}
			_, grammarErr := ValidateOriginalFixedOwnerEntriesV1(ctx, files, leaves)
			if count >= 0 && count < 2 && grammarErr == nil {
				t.Fatal("physical prefix was accepted as a complete endpoint")
			}
			if (count < 0 || count == 2) && grammarErr != nil {
				t.Fatal(grammarErr)
			}
			if count >= 0 && count < 2 {
				if _, err := PrepareSecurePrivateCASOwnerRecoveryV1(ctx, root, leaves, fixture.access); err == nil {
					t.Fatal("normal fixed recovery was weakened")
				}
			}
			if count < 0 {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(filepath.Join(root, "unknown"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if changed, err := observed.SnapshotOriginalFilesV1(ctx); err == nil || changed != nil {
				t.Fatal("changed native topology returned an original map")
			}
		})
	}
}

type originalOwnerBatchHookV1 struct {
	SecurePrivateCASRecoveryAccessAuthority
	leaf  string
	after func() error
}

func (access *originalOwnerBatchHookV1) WithExistingPrivateCASAccess(ctx context.Context, root string, read func(privatecasport.RootBinding) error) error {
	err := access.SecurePrivateCASRecoveryAccessAuthority.WithExistingPrivateCASAccess(ctx, root, read)
	if err == nil && root == access.leaf && access.after != nil {
		hook := access.after
		access.after = nil
		err = hook()
	}
	return err
}

func TestOriginalFixedOwnerBatchRechecksCreationProofAfterLeafObservation(t *testing.T) {
	ctx := context.Background()
	data, residues, proof := originalCreateResidueFixtureV1(t, true)
	root := filepath.Join(data, "private", "attachment-authority")
	leaves := []SecurePrivateCASOwnerLeafV1{}
	for _, name := range []string{"owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"} {
		leaves = append(leaves, SecurePrivateCASOwnerLeafV1{Name: name, MaxBytes: 4096})
	}
	access := &originalOwnerBatchHookV1{SecurePrivateCASRecoveryAccessAuthority: proof.prepared.access, leaf: filepath.Join(root, "owners")}
	original, err := PrepareOriginalFixedOwnerObservationV1(ctx, root, leaves, access, proof)
	if err != nil {
		t.Fatal(err)
	}
	if files, err := original.SnapshotOriginalFilesV1(ctx); err != nil || len(files) == 0 {
		t.Fatalf("healthy original batch: %v", err)
	}
	access.after = func() error { return os.Remove(residues[0]) }
	if files, err := original.SnapshotOriginalFilesV1(ctx); err == nil || files != nil {
		t.Fatal("batch returned original bytes after owner-root creation residue changed during leaf observation")
	}
	if access.after != nil {
		t.Fatal("fixture did not reach the selected leaf")
	}
}

func TestOriginalFixedOwnerBatchOwnsWholeSemanticObservationScope(t *testing.T) {
	for _, mode := range []string{"constructor", "frozen-revalidate"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			data, _, original := originalCreateResidueFixtureV1(t, true)
			private := filepath.Join(data, "private")
			probeRoot := filepath.Join(private, "accepted-finals", "records")
			if err := os.MkdirAll(probeRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			access := &originalOwnerBatchHookV1{SecurePrivateCASRecoveryAccessAuthority: original.prepared.access, leaf: private}
			plan, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, data, access)
			if err != nil {
				t.Fatal(err)
			}
			proof, err := plan.OriginalCreateResiduesV1(ctx, "private/attachment-authority")
			if err != nil {
				t.Fatal(err)
			}
			root := proof.OwnerRootV1()
			leaves := []SecurePrivateCASOwnerLeafV1{}
			for _, name := range []string{"owners", "use-receipts", "use-dispositions", "upload-intents", "upload-dispositions"} {
				leaves = append(leaves, SecurePrivateCASOwnerLeafV1{Name: name, MaxBytes: 4096})
			}
			observed, err := PrepareOriginalFixedOwnerObservationV1(ctx, root, leaves, access, proof)
			if err != nil {
				t.Fatal(err)
			}
			frozen, err := observed.FreezeRecoveryV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			call := frozen.Revalidate
			if mode == "constructor" {
				call = func(ctx context.Context) error {
					_, err := PrepareOriginalFixedOwnerObservationV1(ctx, root, leaves, access, proof)
					return err
				}
			}
			entered, finish, callbackReturned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			observerDone, ownerDone := make(chan error, 1), make(chan error, 1)
			access.after = func() error {
				close(entered)
				select {
				case <-finish:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			go func() {
				ownerDone <- WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(ctx, nil, nil, func(observation context.Context) error {
					go func() { observerDone <- call(observation) }()
					select {
					case <-entered:
					case <-ctx.Done():
						return ctx.Err()
					}
					close(callbackReturned)
					return nil
				})
			}()
			select {
			case <-callbackReturned:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			probe, stop := context.WithTimeout(ctx, 30*time.Millisecond)
			store, openErr := OpenSecurePrivateCASWithAccessAuthorityContext(probe, probeRoot, 4096, original.prepared.access)
			stop()
			if store != nil {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			}
			if !errors.Is(openErr, context.DeadlineExceeded) {
				t.Errorf("whole original batch escaped recovery exclusion before root read drained: %v", openErr)
			}
			ownerReturned := false
			select {
			case err := <-ownerDone:
				ownerReturned = true
				t.Errorf("recovery owner returned while original root read was active: %v", err)
			default:
			}
			close(finish)
			if err := <-observerDone; !errors.Is(err, context.Canceled) {
				t.Errorf("original batch lost scope closing cause: %v", err)
			}
			if !ownerReturned {
				if err := <-ownerDone; err != nil {
					t.Fatal(err)
				}
			}
			var escaped context.Context
			if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(ctx, nil, nil, func(observation context.Context) error { escaped = context.WithoutCancel(observation); return nil }); err != nil {
				t.Fatal(err)
			}
			reads := 0
			access.after = func() error { reads++; return nil }
			if err := call(escaped); err == nil || reads != 0 {
				t.Errorf("expired batch token reached original root access: reads=%d error=%v", reads, err)
			}
		})
	}
}

func TestOriginalFixedOwnerObservationHonorsRetiredScope(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(fixture.authorityRoot, "original-scope-absent")
	observed, err := PrepareOriginalFixedOwnerObservationV1(context.Background(), root, []SecurePrivateCASOwnerLeafV1{{Name: "records", MaxBytes: 4096}}, fixture.access)
	if err != nil {
		t.Fatal(err)
	}
	var escaped context.Context
	err = WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(ctx context.Context) error {
		escaped = context.WithoutCancel(ctx)
		if files, err := observed.SnapshotOriginalFilesV1(ctx); err != nil || files == nil || len(files) != 0 {
			return errors.Join(errors.New("scoped original absence failed"), err)
		}
		outside, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		if files, err := observed.SnapshotOriginalFilesV1(outside); !errors.Is(err, context.DeadlineExceeded) || files != nil {
			return errors.Join(errors.New("unscoped original observation entered retired barrier"), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files, err := observed.SnapshotOriginalFilesV1(escaped); err == nil || files != nil {
		t.Fatal("escaped original observation scope succeeded")
	}
}
