//go:build darwin || linux || windows

package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type originalResidueFixtureV1 struct {
	root, residue, digest string
	access                SecurePrivateCASRecoveryAccessAuthority
	prepared              *PreparedSecurePrivateCASRecoveryV1
}

func newOriginalResidueFixtureV1(t *testing.T) originalResidueFixtureV1 {
	return newOriginalResidueVariantV1(t, false)
}

func newOriginalResidueVariantV1(t *testing.T, linked bool) originalResidueFixtureV1 {
	t.Helper()
	root := filepath.Join(t.TempDir(), "private-cas")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, access)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"original":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(root, digest[:2], "."+digest+".json-held.tmp")
	if linked {
		err = os.Link(filepath.Join(root, digest[:2], digest+".json"), residue)
	} else {
		err = os.WriteFile(residue, []byte("opaque original residue"), 0o600)
	}
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, access)
	if err != nil {
		t.Fatal(err)
	}
	return originalResidueFixtureV1{root: root, residue: residue, digest: digest, access: access, prepared: prepared}
}

func TestSecurePrivateCASOriginalResiduesAllowsIndependentRecords(t *testing.T) {
	for _, kind := range []string{"plain", "linked"} {
		t.Run(kind, func(t *testing.T) {
			testOriginalResidueIndependentRecordsV1(t, newOriginalResidueVariantV1(t, kind == "linked"))
		})
	}
}

func testOriginalResidueIndependentRecordsV1(t *testing.T, fixture originalResidueFixtureV1) {
	t.Helper()
	ctx := context.Background()
	originalBody, err := os.ReadFile(fixture.residue)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(fixture.residue)
	if err != nil {
		t.Fatal(err)
	}
	store, err := fixture.prepared.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if ordinary, _, err := OpenExistingSecurePrivateCASWithAccessAuthorityContext(ctx, fixture.root, 4096, fixture.access); err == nil {
		_ = ordinary.Close()
		t.Fatal("ordinary opening borrowed original-residue authority")
	}
	expected := map[string][]byte{fixture.digest: []byte(`{"original":true}`)}
	for _, digest := range []string{strings.Repeat("a", 63) + "b", strings.Repeat("b", 64)} {
		body := []byte(`{"independent":"` + digest + `"}`)
		if err := store.PutIfAbsent(ctx, digest, body); err != nil {
			t.Fatal(err)
		}
		expected[digest] = body
	}
	additionDigest := strings.Repeat("c", 64)
	additionBody := []byte(`{"receiptedAddition":true}`)
	receipt, err := store.PutIfAbsentWithAdditionReceipt(ctx, additionDigest, additionBody)
	if err != nil {
		t.Fatal(err)
	}
	finalized, err := store.FinalizeCommittedAdditions(ctx, []SecurePrivateCASAdditionReceiptV2{receipt})
	if err != nil || len(finalized) != 1 {
		t.Fatalf("finalize independent addition: %v", err)
	}
	if err := store.VerifyCommittedAddition(ctx, finalized[0]); err != nil {
		t.Fatal(err)
	}
	expected[additionDigest] = additionBody
	files, err := store.List(ctx)
	if err != nil || len(files) != len(expected) {
		t.Fatalf("complete list: count=%d err=%v", len(files), err)
	}
	seen := make(map[string]bool)
	if err := store.Visit(ctx, func(file SecurePrivateCASFile) error {
		if !bytes.Equal(expected[file.Digest], file.Body) || seen[file.Digest] {
			t.Fatal("visitor changed or repeated committed inventory")
		}
		seen[file.Digest] = true
		return nil
	}); err != nil || len(seen) != len(expected) {
		t.Fatalf("complete visit: count=%d err=%v", len(seen), err)
	}
	for digest, body := range expected {
		if got, err := store.Read(ctx, digest); err != nil || !bytes.Equal(got, body) {
			t.Fatalf("readback: %v", err)
		}
	}
	if body, err := store.Read(ctx, strings.Repeat("f", 64)); !errors.Is(err, os.ErrNotExist) || body != nil {
		t.Fatalf("proven absent address: %v", err)
	}
	after, err := os.Stat(fixture.residue)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("independent additions changed held residue metadata: %v", err)
	}
	if body, err := os.ReadFile(fixture.residue); err != nil || !bytes.Equal(body, originalBody) {
		t.Fatalf("independent additions changed held residue body: %v", err)
	}
	if stale, err := fixture.prepared.OpenPreservingOriginalResiduesV1(ctx); err == nil {
		_ = stale.Close()
		t.Fatal("stale opening silently adopted new records")
	}
	current, err := PrepareSecurePrivateCASRecoveryIfPresent(ctx, fixture.root, 4096, fixture.access)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := current.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sibling.Close()
	if sibling.generation != store.generation {
		t.Fatal("compatible sibling reset current generation authority")
	}
}

func TestSecurePrivateCASOriginalLinkedResiduesRejectsDriftBeforeEffects(t *testing.T) {
	for _, kind := range []string{"missing partner", "renamed partner", "third link", "replaced pair", "body", "extra same-address temp"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			fixture := newOriginalResidueVariantV1(t, true)
			store, err := fixture.prepared.OpenPreservingOriginalResiduesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			committed := filepath.Join(fixture.root, fixture.digest[:2], fixture.digest+".json")
			switch kind {
			case "missing partner":
				err = os.Remove(fixture.residue)
			case "renamed partner":
				err = os.Rename(fixture.residue, fixture.residue+"-renamed.tmp")
			case "third link":
				err = os.Link(committed, filepath.Join(filepath.Dir(fixture.root), "foreign-alias"))
			case "replaced pair":
				replacement := filepath.Join(filepath.Dir(fixture.root), "replacement-pair")
				if err = os.WriteFile(replacement, []byte(`{"original":true}`), 0o600); err == nil {
					err = os.Remove(fixture.residue)
				}
				if err == nil {
					err = os.Rename(replacement, committed)
				}
				if err == nil {
					err = os.Link(committed, fixture.residue)
				}
			case "body":
				err = os.WriteFile(committed, []byte(`{"changed":true}`), 0o600)
			case "extra same-address temp":
				err = os.WriteFile(fixture.residue+"-extra.tmp", []byte("unbound"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if body, err := store.Read(ctx, fixture.digest); err == nil || body != nil {
				t.Fatalf("read accepted linked drift: %v", err)
			}
			if files, err := store.List(ctx); err == nil || files != nil {
				t.Fatalf("list accepted linked drift: %v", err)
			}
			called := false
			if err := store.Visit(ctx, func(SecurePrivateCASFile) error { called = true; return nil }); err == nil || called {
				t.Fatalf("visitor reached drifted linked inventory: %v", err)
			}
			digest := strings.Repeat("d", 64)
			if _, err := store.PutIfAbsentWithAdditionReceipt(ctx, digest, []byte(`{"forbidden":true}`)); err == nil {
				t.Fatal("drifted linked inventory issued a new addition receipt")
			}
			if _, err := os.Lstat(filepath.Join(fixture.root, "dd", digest+".json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("drifted linked inventory wrote a record: %v", err)
			}
		})
	}
}

func TestSecurePrivateCASOriginalResiduesRejectsDriftBeforeEffects(t *testing.T) {
	for _, kind := range []string{"body", "missing", "extra residue", "replaced identity", "permissions", "foreign record", "foreign shard", "committed hardlink"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && kind == "permissions" {
				t.Skip("Windows permissions use native DACL validation")
			}
			fixture := newOriginalResidueFixtureV1(t)
			store, err := fixture.prepared.OpenPreservingOriginalResiduesV1(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			switch kind {
			case "body":
				err = os.WriteFile(fixture.residue, []byte("changed original residue"), 0o600)
			case "missing":
				err = os.Remove(fixture.residue)
			case "extra residue":
				err = os.WriteFile(fixture.residue+"-second.tmp", []byte("unobserved"), 0o600)
			case "replaced identity":
				err = os.WriteFile(fixture.residue+".replacement", []byte("opaque original residue"), 0o600)
				if err == nil {
					err = os.Rename(fixture.residue+".replacement", fixture.residue)
				}
			case "permissions":
				err = os.Chmod(fixture.residue, 0o400)
			case "foreign record":
				err = os.WriteFile(filepath.Join(fixture.root, "aa", strings.Repeat("a", 63)+"c.json"), []byte(`{"foreign":true}`), 0o600)
			case "foreign shard":
				err = os.Mkdir(filepath.Join(fixture.root, "cc"), 0o700)
			case "committed hardlink":
				err = os.Link(filepath.Join(fixture.root, "aa", fixture.digest+".json"), filepath.Join(filepath.Dir(fixture.root), "foreign-link"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if body, err := store.Read(context.Background(), fixture.digest); err == nil || body != nil {
				t.Fatalf("read accepted %s: %v", kind, err)
			}
			if files, err := store.List(context.Background()); err == nil || files != nil {
				t.Fatalf("list accepted %s: %v", kind, err)
			}
			called := false
			if err := store.Visit(context.Background(), func(SecurePrivateCASFile) error { called = true; return nil }); err == nil || called {
				t.Fatalf("visitor accepted %s: %v", kind, err)
			}
			digest := strings.Repeat("d", 64)
			if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"mustNotWrite":true}`)); err == nil {
				t.Fatalf("write accepted %s", kind)
			}
			if _, err := os.Lstat(filepath.Join(fixture.root, "dd", digest+".json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected write made a record: %v", err)
			}
			if candidate, err := fixture.prepared.OpenPreservingOriginalResiduesV1(context.Background()); err == nil {
				_ = candidate.Close()
				t.Fatalf("stale prepared opening accepted %s", kind)
			}
		})
	}
}

func TestSecurePrivateCASOriginalResiduesRevalidatesAfterVisitor(t *testing.T) {
	for _, cancelOnly := range []bool{false, true} {
		fixture := newOriginalResidueFixtureV1(t)
		ctx, cancel := context.WithCancel(context.Background())
		store, err := fixture.prepared.OpenPreservingOriginalResiduesV1(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = store.Visit(ctx, func(SecurePrivateCASFile) error {
			if cancelOnly {
				cancel()
				return nil
			}
			return os.WriteFile(fixture.residue, []byte("visitor changed residue"), 0o600)
		})
		cancel()
		_ = store.Close()
		if err == nil || cancelOnly && !errors.Is(err, context.Canceled) {
			t.Fatalf("visitor result lost final proof/cancel: %v", err)
		}
	}
}

func TestSecurePrivateCASOriginalResiduesMissingRequiresFinalProof(t *testing.T) {
	for _, kind := range []string{"cancel", "residue drift"} {
		t.Run(kind, func(t *testing.T) {
			fixture := newOriginalResidueFixtureV1(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store, err := fixture.prepared.OpenPreservingOriginalResiduesV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			store.generation.beforeOriginalReadRevalidation = func() {
				if kind == "cancel" {
					cancel()
					return
				}
				if err := os.WriteFile(fixture.residue, []byte("changed during absent lookup"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			body, err := store.Read(ctx, strings.Repeat("f", 64))
			if err == nil || body != nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed final proof still classified absence: %v", err)
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation cause: %v", err)
			}
			if kind == "residue drift" && !errors.Is(err, ErrSecurePrivateCASIntegrity) {
				t.Fatalf("lost integrity cause: %v", err)
			}
		})
	}
}

func TestSecurePrivateCASOriginalEmptyShardAllowsOnlyAuthorizedAdditions(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "private-cas")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(root, "ab")
	if err := os.Mkdir(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(shard)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary, _, err := OpenExistingSecurePrivateCASWithAccessAuthorityContext(ctx, root, 4096, access); err == nil {
		_ = ordinary.Close()
		t.Fatal("ordinary opening adopted an empty shard")
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(ctx, root, 4096, access)
	if err != nil {
		t.Fatal(err)
	}
	store, err := prepared.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if files, err := store.List(ctx); err != nil || len(files) != 0 {
		t.Fatalf("original empty shard inventory: %v", err)
	}
	for _, digest := range []string{"ab" + strings.Repeat("1", 62), "cd" + strings.Repeat("2", 62)} {
		if err := store.PutIfAbsent(ctx, digest, []byte(`{"independent":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	if files, err := store.List(ctx); err != nil || len(files) != 2 {
		t.Fatalf("empty-shard additions lost the full denominator: %v", err)
	}
	after, err := os.Stat(shard)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
		t.Fatalf("exact addition changed original directory: %v", err)
	}
	current, err := PrepareSecurePrivateCASRecoveryIfPresent(ctx, root, 4096, access)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := current.OpenPreservingOriginalResiduesV1(ctx)
	if err != nil {
		t.Fatalf("fresh explicit opening lost authorized filled-shard history: %v", err)
	}
	defer sibling.Close()
	if sibling.generation != store.generation {
		t.Fatal("filled-shard sibling reset authority")
	}
	if ordinary, _, err := OpenExistingSecurePrivateCASWithAccessAuthorityContext(ctx, root, 4096, access); err == nil {
		_ = ordinary.Close()
		t.Fatal("ordinary opening borrowed original directory preservation")
	}
	if err := os.Mkdir(filepath.Join(root, "ef"), 0o700); err != nil {
		t.Fatal(err)
	}
	if files, err := store.List(ctx); err == nil || files != nil {
		t.Fatalf("original proof adopted a new unowned empty shard: %v", err)
	}
	foreign, err := PrepareSecurePrivateCASRecoveryIfPresent(ctx, root, 4096, access)
	if err != nil {
		t.Fatal(err)
	}
	if candidate, err := foreign.OpenPreservingOriginalResiduesV1(ctx); err == nil {
		_ = candidate.Close()
		t.Fatal("fresh preparation upgraded the live generation with an unowned shard")
	}
}
