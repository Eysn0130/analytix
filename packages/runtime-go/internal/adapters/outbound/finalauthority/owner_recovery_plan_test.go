package finalauthority

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type countingPrivateCASRecoveryAccessAuthority struct {
	inner *privatecastest.AccessAuthority
	calls int
}

func (authority *countingPrivateCASRecoveryAccessAuthority) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	authority.calls++
	return authority.inner.WithPrivateCASAccess(ctx, requestedRoot, access)
}

func (authority *countingPrivateCASRecoveryAccessAuthority) WithExistingPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	authority.calls++
	return authority.inner.WithExistingPrivateCASAccess(ctx, requestedRoot, access)
}

func TestMissingPrivateCASOwnerRecoveryAccessIsIndependentOfLeafCount(t *testing.T) {
	prepare := func(t *testing.T, leafCount int) (*PreparedSecurePrivateCASOwnerRecoveryV1, *countingPrivateCASRecoveryAccessAuthority) {
		t.Helper()
		root := filepath.Join(t.TempDir(), "missing-owner")
		inner, err := privatecastest.NewAccessAuthority(root)
		if err != nil {
			t.Fatal(err)
		}
		authority := &countingPrivateCASRecoveryAccessAuthority{inner: inner}
		leaves := make([]SecurePrivateCASOwnerLeafV1, leafCount)
		for index := range leaves {
			leaves[index] = SecurePrivateCASOwnerLeafV1{
				Name:     fmt.Sprintf("leaf-%02d", index),
				MaxBytes: 4096,
			}
		}
		prepared, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), root, leaves, authority)
		if err != nil {
			t.Fatal(err)
		}
		if prepared.Present() {
			t.Fatal("missing private CAS owner was reported present")
		}
		return prepared, authority
	}

	one, oneAuthority := prepare(t, 1)
	many, manyAuthority := prepare(t, 8)
	if oneAuthority.calls != manyAuthority.calls {
		t.Fatalf("missing owner preparation scaled with leaf count: one=%d many=%d", oneAuthority.calls, manyAuthority.calls)
	}
	if oneAuthority.calls != 2 {
		t.Fatalf("missing owner preparation access calls = %d, want 2", oneAuthority.calls)
	}
	if err := one.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := many.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if oneAuthority.calls != manyAuthority.calls {
		t.Fatalf("missing owner revalidation scaled with leaf count: one=%d many=%d", oneAuthority.calls, manyAuthority.calls)
	}
	if oneAuthority.calls != 3 {
		t.Fatalf("missing owner revalidation access calls = %d, want 3", oneAuthority.calls)
	}
}

func TestMissingPrivateCASOwnerRecoveryRejectsLaterOwnerCreation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-owner")
	inner, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASOwnerRecoveryV1(
		context.Background(),
		root,
		[]SecurePrivateCASOwnerLeafV1{{Name: "leaf", MaxBytes: 4096}},
		inner,
	)
	if err != nil {
		t.Fatal(err)
	}
	plans := prepared.SecurePrivateCASRecoveryPlansV2()
	if len(plans) != 1 {
		t.Fatalf("missing owner leaf plans = %d, want 1", len(plans))
	}
	if err := os.MkdirAll(filepath.Join(root, "leaf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err == nil {
		t.Fatal("missing owner recovery accepted a later owner creation")
	}
	if err := plans[0].Revalidate(context.Background()); err == nil {
		t.Fatal("missing owner leaf plan accepted a later leaf creation")
	}
}

func TestPrivateCASOwnerMixedTopologyRetainsAndBindsEmptyRegularSibling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owner")
	if err := os.MkdirAll(filepath.Join(root, "leaf"), 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, ".registry.lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASOwnerMixedTopologyV2(
		context.Background(), root, []string{"leaf"},
		[]SecurePrivateCASOwnerRegularSiblingV2{{Name: ".registry.lock", ByteLength: 0}}, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(lockPath)
	if err != nil {
		t.Fatalf("bound regular sibling was removed: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("bound regular sibling identity changed during preparation")
	}
	if err := prepared.RevalidatePrivateCASRecoveryTopologyV3(context.Background()); err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(filepath.Dir(root), "displaced-registry-lock")
	if err := os.Rename(lockPath, oldPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepared.RevalidatePrivateCASRecoveryTopologyV3(context.Background()); err == nil {
		t.Fatal("owner topology accepted a same-shape regular sibling identity swap")
	}
}

func TestPrivateCASOwnerMixedTopologyRejectsHardlinkedOrNonEmptyRegularSibling(t *testing.T) {
	for _, target := range []string{"hardlink", "non-empty"} {
		t.Run(target, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "owner")
			if err := os.MkdirAll(filepath.Join(root, "leaf"), 0o700); err != nil {
				t.Fatal(err)
			}
			lockPath := filepath.Join(root, ".registry.lock")
			body := []byte(nil)
			if target == "non-empty" {
				body = []byte("not-disposable")
			}
			if err := os.WriteFile(lockPath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if target == "hardlink" {
				if err := os.Link(lockPath, filepath.Join(filepath.Dir(root), "registry-lock-link")); err != nil {
					t.Skipf("hardlink unavailable: %v", err)
				}
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareSecurePrivateCASOwnerMixedTopologyV2(
				context.Background(), root, []string{"leaf"},
				[]SecurePrivateCASOwnerRegularSiblingV2{{Name: ".registry.lock", ByteLength: 0}}, access,
			); err == nil {
				t.Fatalf("%s regular sibling passed handle-bound owner topology", target)
			}
		})
	}
}

func TestDiscoveredOwnerRecursiveInventoryRejectsSameSizeRewriteOnUnixAndWindows(t *testing.T) {
	for _, testCase := range []struct {
		name string
		path func(string) string
	}{
		{name: "direct", path: func(root string) string { return filepath.Join(root, "legacy-direct") }},
		{name: "nested", path: func(root string) string { return filepath.Join(root, "thread-legacy", "turn-record") }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "owner")
			for _, leaf := range []string{"indexes", "capsules"} {
				if err := os.MkdirAll(filepath.Join(root, leaf), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			target := testCase.path(root)
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			prepared, inventory, err := PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2(
				context.Background(), root, []string{"indexes", "capsules"}, 16, 4096, access,
			)
			if err != nil || len(inventory) != 3 {
				t.Fatalf("prepare recursive owner inventory: inventory=%#v err=%v", inventory, err)
			}
			if err := os.WriteFile(target, []byte("rewritte"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := prepared.RevalidatePrivateCASRecoveryTopologyV3(context.Background()); err == nil {
				t.Fatal("same-size direct/nested rewrite survived recursive owner revalidation")
			}
		})
	}
}

func TestDiscoveredOwnerRecursiveInventoryEnforcesAggregateBounds(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		maxEntries int
		maxBytes   int64
	}{
		{name: "entry-count", maxEntries: 3, maxBytes: 4096},
		{name: "regular-bytes", maxEntries: 16, maxBytes: 7},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "owner")
			for _, path := range []string{
				filepath.Join(root, "indexes"), filepath.Join(root, "capsules"), filepath.Join(root, "legacy"),
			} {
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "legacy", "record"), []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2(
				context.Background(), root, []string{"indexes", "capsules"},
				testCase.maxEntries, testCase.maxBytes, access,
			); err == nil {
				t.Fatal("recursive owner inventory exceeded a fixed count/byte bound")
			}
		})
	}
}
