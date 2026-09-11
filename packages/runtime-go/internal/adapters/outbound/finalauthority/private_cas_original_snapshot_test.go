//go:build darwin || linux || windows

package finalauthority

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func originalSnapshotExpectedTreeV1(t *testing.T, root string) map[string]SecurePrivateCASOriginalEntryV1 {
	t.Helper()
	entries := map[string]SecurePrivateCASOriginalEntryV1{}
	if err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		value := SecurePrivateCASOriginalEntryV1{Directory: info.IsDir(), Mode: uint32(info.Mode().Perm())}
		if !value.Directory {
			value.Body, err = os.ReadFile(name)
			if err != nil {
				return err
			}
		}
		entries[filepath.ToSlash(rel)] = value
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestPrivateCASOriginalSnapshotRetainsCompleteOwnerAndNativePair(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain residue", true: "linked residue"}[linked], func(t *testing.T) {
			fixture := newPrivateCASRecoveryV4Fixture(t)
			leaf := filepath.Dir(filepath.Dir(fixture.ordinaryResidue))
			if linked {
				if err := os.Remove(fixture.ordinaryResidue); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(filepath.Join(leaf, privateCASRecoveryV4Digest[:2], privateCASRecoveryV4Digest+".json"), fixture.ordinaryResidue); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(leaf, "fe"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(leaf, "fe", ".fe00000000000000000000000000000000000000000000000000000000000000.json-0123456789abcdef01234567.tmp"), []byte{}, 0o600); err != nil {
				t.Fatal(err)
			}
			owner := fixture.owner(t)
			before := originalSnapshotExpectedTreeV1(t, fixture.ownerRoot)
			entries, err := owner.SnapshotOriginalEntriesV1(context.Background())
			if err != nil || !reflect.DeepEqual(entries, before) {
				t.Fatalf("complete original entries changed: actual=%d expected=%d err=%v", len(entries), len(before), err)
			}
			if !reflect.DeepEqual(originalSnapshotExpectedTreeV1(t, fixture.ownerRoot), before) {
				t.Fatal("original snapshot wrote owner state")
			}
			// Add a new physical path after the frozen observation. Neither a
			// provisional map nor an old empty-inventory answer may escape.
			if err := os.WriteFile(filepath.Join(fixture.ownerRoot, "unexpected"), []byte("drift"), 0o600); err != nil {
				t.Fatal(err)
			}
			if entries, err := owner.SnapshotOriginalEntriesV1(context.Background()); err == nil || entries != nil {
				t.Fatalf("late physical entry returned original map: %v", err)
			}
		})
	}
}

func TestPrivateCASOriginalSnapshotHonorsScopedObservationAndCancellation(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	owner := fixture.owner(t)
	before := originalSnapshotExpectedTreeV1(t, fixture.ownerRoot)
	var escaped context.Context
	err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(ctx context.Context) error {
		escaped = context.WithoutCancel(ctx)
		bounded, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		entries, err := owner.SnapshotOriginalEntriesV1(bounded)
		if err != nil || !reflect.DeepEqual(entries, before) {
			return errors.Join(errors.New("original snapshot lost scoped native inventory"), err)
		}
		outside, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer stop()
		if entries, err := owner.SnapshotOriginalEntriesV1(outside); !errors.Is(err, context.DeadlineExceeded) || entries != nil {
			return errors.Join(errors.New("unscoped original snapshot entered recovery barrier"), err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if entries, err := owner.SnapshotOriginalEntriesV1(escaped); err == nil || entries != nil {
		t.Fatalf("escaped original snapshot retained authority: %v", err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("original snapshot cancelled")
	cancel(cause)
	if entries, err := owner.SnapshotOriginalEntriesV1(ctx); !errors.Is(err, cause) || entries != nil {
		t.Fatalf("original snapshot lost cancellation cause: %v", err)
	}
}

func TestPrivateCASOriginalSnapshotAbsentOwnerDoesNotCreate(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	root := filepath.Join(fixture.authorityRoot, "absent-original-owner")
	owner, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), root,
		[]SecurePrivateCASOwnerLeafV1{{Name: "bundles", MaxBytes: 4096}, {Name: "observations", MaxBytes: 4096}}, fixture.access)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := owner.SnapshotOriginalEntriesV1(context.Background())
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatalf("absent original owner acquired entries: %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original snapshot created missing owner: %v", err)
	}
}

func TestPrivateCASOriginalSnapshotAbsentOwnerHonorsObservationScope(t *testing.T) {
	for _, boundary := range []string{"unscoped", "escaped"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := newPrivateCASRecoveryV4Fixture(t)
			if err := os.Remove(fixture.ordinaryResidue); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(fixture.authorityRoot, "absent-scoped-original")
			owner, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), root, []SecurePrivateCASOwnerLeafV1{{Name: "records", MaxBytes: 4096}}, fixture.access)
			if err != nil {
				t.Fatal(err)
			}
			var escaped context.Context
			var entries map[string]SecurePrivateCASOriginalEntryV1
			var observedErr error
			err = WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(ctx context.Context) error {
				escaped = context.WithoutCancel(ctx)
				if entries, err := owner.SnapshotOriginalEntriesV1(ctx); err != nil || entries == nil || len(entries) != 0 {
					return errors.Join(errors.New("scoped absent original inventory failed"), err)
				}
				if boundary == "unscoped" {
					outside, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
					defer cancel()
					entries, observedErr = owner.SnapshotOriginalEntriesV1(outside)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "escaped" {
				entries, observedErr = owner.SnapshotOriginalEntriesV1(escaped)
			}
			if observedErr == nil || entries != nil {
				t.Fatalf("absent original inventory bypassed %s scope: entries=%d err=%v", boundary, len(entries), observedErr)
			}
			if boundary == "unscoped" && !errors.Is(observedErr, context.DeadlineExceeded) {
				t.Fatalf("unscoped absent observation lost deadline: %v", observedErr)
			}
		})
	}
}
