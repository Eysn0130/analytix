//go:build darwin || linux

package finalauthority

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"golang.org/x/sys/unix"
)

func TestSecurePrivateCASAdditionReceiptRejectsMetadataAndIdentityReplacement(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string, string)
	}{
		{
			name: "record mode",
			mutate: func(t *testing.T, root, digest string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(root, digest[:2], digest+".json"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "same body new record identity",
			mutate: func(t *testing.T, root, digest string) {
				t.Helper()
				record := filepath.Join(root, digest[:2], digest+".json")
				body, err := os.ReadFile(record)
				if err != nil {
					t.Fatal(err)
				}
				replacement := record + ".replacement"
				if err := os.WriteFile(replacement, body, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, record); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "shard mode",
			mutate: func(t *testing.T, root, digest string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(root, digest[:2]), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private-cas")
			store, err := openTestSecurePrivateCAS(t, root, 4096)
			if err != nil {
				t.Fatal(err)
			}
			body := []byte(`{"schemaVersion":1,"purpose":"receipt-test"}`)
			digest := domainsecurity.SHA256Hex([]byte(test.name))
			token, err := store.PutIfAbsentWithAdditionReceipt(context.Background(), digest, body)
			if err != nil {
				t.Fatalf("commit with addition receipt: %v", err)
			}
			if !token.CreatedByThisCall || token.Finalized {
				t.Fatal("addition receipt did not bind current-call creation")
			}
			finalized, err := store.FinalizeCommittedAdditions(context.Background(), []SecurePrivateCASAdditionReceiptV2{token})
			if err != nil || len(finalized) != 1 {
				t.Fatalf("finalize committed addition: %v", err)
			}
			receipt := finalized[0]
			if err := store.VerifyCommittedAddition(context.Background(), receipt); err != nil {
				t.Fatalf("fresh addition receipt failed verification: %v", err)
			}
			test.mutate(t, root, digest)
			if err := store.VerifyCommittedAddition(context.Background(), receipt); err == nil {
				t.Fatal("metadata or object identity replacement preserved addition authority")
			}
		})
	}
}

func TestSecurePrivateCASAdditionBatchFinalizesOnlyCurrentCallCommits(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digests := []string{
		"aa" + strings.Repeat("0", 61) + "1",
		"aa" + strings.Repeat("0", 61) + "2",
		"bb" + strings.Repeat("0", 61) + "3",
	}
	tokens := make([]SecurePrivateCASAdditionReceiptV2, 0, len(digests))
	for index, digest := range digests {
		token, err := store.PutIfAbsentWithAdditionReceipt(
			context.Background(), digest, []byte(fmt.Sprintf(`{"record":%d}`, index+1)),
		)
		if err != nil {
			t.Fatalf("commit %d: %v", index, err)
		}
		tokens = append(tokens, token)
	}
	if !tokens[0].ShardCreatedByThisCall || tokens[1].ShardCreatedByThisCall || !tokens[2].ShardCreatedByThisCall {
		t.Fatalf("unexpected shard creation provenance: %#v", tokens)
	}
	finalized, err := store.FinalizeCommittedAdditions(context.Background(), tokens)
	if err != nil || len(finalized) != len(tokens) {
		t.Fatalf("finalize addition batch: %v", err)
	}
	for _, receipt := range finalized {
		if !receipt.Finalized {
			t.Fatal("batch receipt was not finalized")
		}
		if err := store.VerifyCommittedAddition(context.Background(), receipt); err != nil {
			t.Fatalf("verify finalized batch receipt: %v", err)
		}
	}
	if finalized[0].ShardMetadata != finalized[1].ShardMetadata || finalized[0].RootMetadata != finalized[2].RootMetadata {
		t.Fatal("batch finalization did not freeze one final directory snapshot")
	}
}

func TestSecurePrivateCASAdditionReceiptV2SpansBatchesAndPreopenedSiblingStores(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	sibling, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sibling.Close() })

	digests := []string{
		"ab" + strings.Repeat("0", 61) + "1",
		"ab" + strings.Repeat("0", 61) + "2",
		"ab" + strings.Repeat("0", 61) + "3",
	}
	commit := func(store *SecurePrivateCAS, digest string) SecurePrivateCASAdditionReceiptV2 {
		t.Helper()
		token, err := store.PutIfAbsentWithAdditionReceipt(context.Background(), digest, []byte(`{"record":"`+digest+`"}`))
		if err != nil {
			t.Fatalf("commit %s: %v", digest, err)
		}
		if token.SchemaVersion != 2 {
			t.Fatalf("receipt schema version = %d", token.SchemaVersion)
		}
		return token
	}

	firstToken := commit(first, digests[0])
	siblingToken := commit(sibling, digests[1])
	firstBatch, err := sibling.FinalizeCommittedAdditions(
		context.Background(), []SecurePrivateCASAdditionReceiptV2{firstToken, siblingToken},
	)
	if err != nil || len(firstBatch) != 2 {
		t.Fatalf("preopened sibling could not finalize shared receipt batch: %v", err)
	}
	for _, receipt := range firstBatch {
		if err := first.VerifyCommittedAddition(context.Background(), receipt); err != nil {
			t.Fatalf("first store could not verify sibling-finalized receipt: %v", err)
		}
	}
	secondBatchToken := commit(first, digests[2])
	secondBatch, err := first.FinalizeCommittedAdditions(
		context.Background(), []SecurePrivateCASAdditionReceiptV2{secondBatchToken},
	)
	if err != nil || len(secondBatch) != 1 {
		t.Fatalf("finalize later receipt batch: %v", err)
	}
	if err := sibling.VerifyCommittedAddition(context.Background(), secondBatch[0]); err != nil {
		t.Fatalf("preopened sibling could not verify later receipt batch: %v", err)
	}
	firstReceipt, siblingReceipt, secondBatchReceipt := firstBatch[0], firstBatch[1], secondBatch[0]
	if !firstReceipt.ShardCreatedByThisCall || firstReceipt.ShardExistedBeforeCommit {
		t.Fatalf("first shard commit provenance is wrong: %#v", firstReceipt)
	}
	for _, receipt := range []SecurePrivateCASAdditionReceiptV2{secondBatchReceipt, siblingReceipt} {
		if receipt.ShardCreatedByThisCall || !receipt.ShardExistedBeforeCommit {
			t.Fatalf("existing shard commit provenance is wrong: %#v", receipt)
		}
	}
}

func TestSecurePrivateCASSharedGenerationRejectsInjectedRecordAndAncestorReplacement(t *testing.T) {
	t.Run("host-created shard record injection", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "private-cas")
		authority, err := privatecastest.NewAccessAuthority(root)
		if err != nil {
			t.Fatal(err)
		}
		first, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = first.Close() })
		sibling, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sibling.Close() })
		digest := "ac" + strings.Repeat("1", 62)
		if err := first.PutIfAbsent(context.Background(), digest, []byte(`{"record":"host"}`)); err != nil {
			t.Fatal(err)
		}
		injected := "ac" + strings.Repeat("2", 62)
		if err := os.WriteFile(filepath.Join(root, "ac", injected+".json"), []byte(`{"record":"injected"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := sibling.Read(context.Background(), digest); err == nil {
			t.Fatal("canonical record injection entered the shared host authority")
		}
		if err := first.PutIfAbsent(context.Background(), "ac"+strings.Repeat("3", 62), []byte(`{"record":"later"}`)); err == nil {
			t.Fatal("record injection was absorbed by a later host commit")
		}
	})

	t.Run("shard replacement", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "private-cas")
		store, err := openTestSecurePrivateCAS(t, root, 4096)
		if err != nil {
			t.Fatal(err)
		}
		digest := "ad" + strings.Repeat("4", 62)
		body := []byte(`{"record":"anchored"}`)
		if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
			t.Fatal(err)
		}
		shard := filepath.Join(root, "ad")
		moved := shard + ".old"
		if err := os.Rename(shard, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(shard, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(shard, digest+".json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Read(context.Background(), digest); err == nil {
			t.Fatal("replacement shard reused the anchored authority")
		}
	})

	t.Run("root replacement", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "private-cas")
		store, err := openTestSecurePrivateCAS(t, root, 4096)
		if err != nil {
			t.Fatal(err)
		}
		digest := "ae" + strings.Repeat("5", 62)
		if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"record":"root"}`)); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(root, root+".old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := store.List(context.Background()); err == nil {
			t.Fatal("replacement root reused the anchored authority")
		}
	})
}

func TestSecurePrivateCASRecoveryRevokesEveryLiveSiblingGeneration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	sibling, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sibling.Close() })
	digest := "af" + strings.Repeat("6", 62)
	body := []byte(`{"record":"before-recovery"}`)
	if err := first.PutIfAbsent(context.Background(), digest, body); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []*SecurePrivateCAS{first, sibling} {
		if _, err := stale.Read(context.Background(), digest); err == nil {
			t.Fatal("pre-recovery live store retained root-generation authority")
		}
	}
	reopened, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got, err := reopened.Read(context.Background(), digest); err != nil || !equalPrivateCASBytes(got, body) {
		t.Fatalf("new root generation did not preserve committed inventory: body=%q err=%v", got, err)
	}
}

func TestPrivateCASRecoveryTransactionRollsBackBeforeDurableCommit(t *testing.T) {
	t.Run("owner one before owner two", func(t *testing.T) {
		first, firstTemp := preparedRecoveryTransactionFixture(t, "31", "owner-one")
		second, secondTemp := preparedRecoveryTransactionFixture(t, "32", "owner-two")
		setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
			if phase == "after_plan_stage" && index == 0 {
				return errors.New("owner-two-stage-cut")
			}
			return nil
		})
		if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV2(
			context.Background(), []*PreparedSecurePrivateCASRecoveryV1{first, second},
		); err == nil {
			t.Fatal("owner-stage fault did not stop the transaction")
		}
		for _, path := range []string{firstTemp, secondTemp} {
			if body, err := os.ReadFile(path); err != nil || string(body) != "owner-residue" {
				t.Fatalf("pre-commit owner fault deleted or changed residue %s: body=%q err=%v", path, body, err)
			}
		}
	})

	t.Run("shard one before shard two", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "private-cas")
		authority, err := privatecastest.NewAccessAuthority(root)
		if err != nil {
			t.Fatal(err)
		}
		store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		digests := []string{"41" + strings.Repeat("a", 62), "42" + strings.Repeat("b", 62)}
		paths := make([]string, 0, len(digests))
		for _, digest := range digests {
			if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"record":"committed"}`)); err != nil {
				t.Fatal(err)
			}
		}
		for _, digest := range digests {
			path := filepath.Join(root, digest[:2], "."+digest+".json-0123456789abcdef01234567.tmp")
			if err := os.WriteFile(path, []byte("shard-residue"), 0o600); err != nil {
				t.Fatal(err)
			}
			paths = append(paths, path)
		}
		prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
		if err != nil {
			t.Fatal(err)
		}
		setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
			if phase == "after_shard_stage" && index == 0 {
				return errors.New("shard-two-stage-cut")
			}
			return nil
		})
		if err := prepared.Apply(context.Background()); err == nil {
			t.Fatal("shard-stage fault did not stop the transaction")
		}
		for _, path := range paths {
			if body, err := os.ReadFile(path); err != nil || string(body) != "shard-residue" {
				t.Fatalf("pre-commit shard fault deleted or changed residue %s: body=%q err=%v", path, body, err)
			}
		}
	})

	t.Run("revalidate to stage drift", func(t *testing.T) {
		prepared, original := preparedRecoveryTransactionFixture(t, "51", "revalidate-stage")
		late := filepath.Join(filepath.Dir(original), "."+"51"+strings.Repeat("c", 62)+".json-89abcdef0123456701234567.tmp")
		mutated := false
		setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
			if phase == "before_plan_stage" && index == 0 && !mutated {
				mutated = true
				return os.WriteFile(late, []byte("late-residue"), 0o600)
			}
			return nil
		})
		if err := prepared.Apply(context.Background()); err == nil {
			t.Fatal("revalidate-to-stage drift passed the transaction")
		}
		for _, path := range []string{original, late} {
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("drift failure cleaned residue %s: %v", path, err)
			}
		}
	})
}

func TestPrivateCASOwnerRecoveryUsesOneCrossLeafTransaction(t *testing.T) {
	ownerRoot := filepath.Join(t.TempDir(), "private-owner")
	authority, err := privatecastest.NewAccessAuthority(ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		leaf   string
		shard  string
		marker string
	}{
		{leaf: "first", shard: "81", marker: "a"},
		{leaf: "second", shard: "82", marker: "b"},
	}
	residues := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		root := filepath.Join(ownerRoot, fixture.leaf)
		store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		digest := fixture.shard + strings.Repeat(fixture.marker, 62)
		if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"record":"owner"}`)); err != nil {
			t.Fatal(err)
		}
		residue := filepath.Join(root, fixture.shard, "."+digest+".json-0123456789abcdef01234567.tmp")
		if err := os.WriteFile(residue, []byte("owner-cross-leaf-residue"), 0o600); err != nil {
			t.Fatal(err)
		}
		residues = append(residues, residue)
	}
	prepared, err := PrepareSecurePrivateCASOwnerRecoveryV1(
		context.Background(), ownerRoot,
		[]SecurePrivateCASOwnerLeafV1{{Name: "first", MaxBytes: 4096}, {Name: "second", MaxBytes: 4096}},
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
		if phase == "after_plan_stage" && index == 0 {
			return errors.New("owner-cross-leaf-cut")
		}
		return nil
	})
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("cross-leaf transaction cut did not stop owner recovery")
	}
	for _, residue := range residues {
		if body, err := os.ReadFile(residue); err != nil || string(body) != "owner-cross-leaf-residue" {
			t.Fatalf("owner recovery partially cleaned leaf %s: body=%q err=%v", residue, body, err)
		}
	}
}

func TestPrivateCASRecoveryV3RejectsParentSwapWithOriginalLeafIdentities(t *testing.T) {
	authorityRoot := t.TempDir()
	ownerRoot := filepath.Join(authorityRoot, "private-owner")
	authority, err := privatecastest.NewAccessAuthority(authorityRoot)
	if err != nil {
		t.Fatal(err)
	}
	leaves := []SecurePrivateCASOwnerLeafV1{
		{Name: "first", MaxBytes: 4096},
		{Name: "second", MaxBytes: 4096},
	}
	residuePaths := make([]string, 0, len(leaves))
	for index, leaf := range leaves {
		root := filepath.Join(ownerRoot, leaf.Name)
		store, err := OpenSecurePrivateCASWithAccessAuthority(root, leaf.MaxBytes, authority)
		if err != nil {
			t.Fatal(err)
		}
		digest := fmt.Sprintf("%02x", 0xb1+index) + strings.Repeat("d", 62)
		if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"record":"parent-swap"}`)); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		residue := filepath.Join(root, digest[:2], "."+digest+".json-0123456789abcdef01234567.tmp")
		if err := os.WriteFile(residue, []byte("parent-swap-residue"), 0o600); err != nil {
			t.Fatal(err)
		}
		residuePaths = append(residuePaths, residue)
	}
	prepared, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), ownerRoot, leaves, authority)
	if err != nil {
		t.Fatal(err)
	}
	originalTopologyDigest := prepared.PrivateCASRecoveryTopologyDigestV4()
	if !validPrivateDigest(originalTopologyDigest) {
		t.Fatal("prepared owner omitted its durable topology digest")
	}
	backup := ownerRoot + ".original"
	setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
		if phase != "after_plan_stage" || index != 0 {
			return nil
		}
		if err := os.Rename(ownerRoot, backup); err != nil {
			return err
		}
		if err := os.Mkdir(ownerRoot, 0o700); err != nil {
			return err
		}
		for _, leaf := range leaves {
			if err := os.Rename(filepath.Join(backup, leaf.Name), filepath.Join(ownerRoot, leaf.Name)); err != nil {
				return err
			}
		}
		return errors.New("parent-owner-swap-cut")
	})
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("parent owner swap with original leaf identities passed V3 recovery")
	}
	reprepared, err := PrepareSecurePrivateCASOwnerRecoveryV1(context.Background(), ownerRoot, leaves, authority)
	if err != nil {
		t.Fatal(err)
	}
	if reparsed := reprepared.PrivateCASRecoveryTopologyDigestV4(); !validPrivateDigest(reparsed) || reparsed == originalTopologyDigest {
		t.Fatalf("replacement owner retained the original durable topology digest: before=%s after=%s", originalTopologyDigest, reparsed)
	}
	for index, ordinary := range residuePaths {
		entries, err := os.ReadDir(filepath.Dir(ordinary))
		if err != nil {
			t.Fatal(err)
		}
		staged := false
		committed := false
		for _, entry := range entries {
			staged = staged || strings.HasPrefix(entry.Name(), privateCASRecoveryStagePrefix)
			committed = committed || strings.HasPrefix(entry.Name(), privateCASRecoveryCommitPrefix)
		}
		if committed {
			t.Fatal("pre-marker topology drift created a committed marker")
		}
		if index == 0 {
			if _, err := os.Lstat(ordinary); !errors.Is(err, os.ErrNotExist) || !staged {
				t.Fatalf("topology drift incorrectly rolled a staged residue back: path=%s staged=%v err=%v", ordinary, staged, err)
			}
			continue
		}
		if body, err := os.ReadFile(ordinary); err != nil || string(body) != "parent-swap-residue" || staged {
			t.Fatalf("topology drift changed a not-yet-staged residue: path=%s staged=%v body=%q err=%v", ordinary, staged, body, err)
		}
	}
}

func TestPrivateCASRecoveryExcludesPreopenedSiblingOperationsAcrossTransaction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	sibling, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sibling.Close() })
	digest := "91" + strings.Repeat("a", 62)
	if err := first.PutIfAbsent(context.Background(), digest, []byte(`{"record":"recovery-exclusion"}`)); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(root, digest[:2], "."+digest+".json-0123456789abcdef01234567.tmp")
	if err := os.WriteFile(residue, []byte("recovery-exclusion-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	cutEntered := make(chan struct{})
	releaseCut := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case releaseCut <- struct{}{}:
		default:
		}
	})
	setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
		if phase == "after_plan_stage" && index == 0 {
			close(cutEntered)
			<-releaseCut
			return errors.New("recovery-exclusion-cut")
		}
		return nil
	})
	applyDone := make(chan error, 1)
	go func() { applyDone <- prepared.Apply(context.Background()) }()
	select {
	case <-cutEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery transaction did not reach the staged cut")
	}
	cancelled, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := sibling.Read(cancelled, digest); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recovery exclusion did not preserve sibling cancellation: %v", err)
	}
	readStarted := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		close(readStarted)
		_, readErr := sibling.Read(context.Background(), digest)
		readDone <- readErr
	}()
	<-readStarted
	select {
	case err := <-readDone:
		t.Fatalf("preopened sibling entered a staged recovery transaction: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	releaseCut <- struct{}{}
	if err := <-applyDone; err == nil {
		t.Fatal("recovery exclusion cut did not stop the transaction")
	}
	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
		t.Fatal("preopened sibling did not resume after recovery released exclusion")
	}
}

func TestPrivateCASRecoveryTransactionResumesAfterDurableCommitCuts(t *testing.T) {
	for _, cut := range []struct {
		name  string
		phase string
		index int
	}{
		{name: "after commit marker", phase: "after_commit_marker", index: 0},
		{name: "after first owner commit", phase: "after_plan_commit", index: 0},
		{name: "before marker finalize", phase: "before_commit_finalize", index: 0},
	} {
		t.Run(cut.name, func(t *testing.T) {
			first, firstTemp := preparedRecoveryTransactionFixture(t, "61", "commit-cut-one")
			second, secondTemp := preparedRecoveryTransactionFixture(t, "62", "commit-cut-two")
			setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
				if phase == cut.phase && index == cut.index {
					return errors.New("committed-transaction-crash-cut")
				}
				return nil
			})
			if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV2(
				context.Background(), []*PreparedSecurePrivateCASRecoveryV1{first, second},
			); err == nil {
				t.Fatal("commit crash cut did not stop the transaction")
			}
			setPrivateCASRecoveryTransactionHook(t, nil)
			first = reprepareRecoveryTransactionFixture(t, first)
			second = reprepareRecoveryTransactionFixture(t, second)
			if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV2(
				context.Background(), []*PreparedSecurePrivateCASRecoveryV1{first, second},
			); err != nil {
				t.Fatalf("resume committed transaction: %v", err)
			}
			for _, path := range []string{firstTemp, secondTemp} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("resumed committed transaction retained original residue %s: %v", path, err)
				}
			}
		})
	}
}

func preparedRecoveryTransactionFixture(
	t *testing.T,
	shard string,
	label string,
) (*PreparedSecurePrivateCASRecoveryV1, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	digest := shard + strings.Repeat("d", 62)
	if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"record":"`+label+`"}`)); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, shard, "."+digest+".json-0123456789abcdef01234567.tmp")
	if err := os.WriteFile(temp, []byte("owner-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, temp
}

func reprepareRecoveryTransactionFixture(
	t *testing.T,
	previous *PreparedSecurePrivateCASRecoveryV1,
) *PreparedSecurePrivateCASRecoveryV1 {
	t.Helper()
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(
		context.Background(), previous.rootPath, previous.maxBytes, previous.access,
	)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func setPrivateCASRecoveryTransactionHook(t *testing.T, hook func(string, int) error) {
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

func TestSecurePrivateCASAdditionReceiptCannotBeIssuedForExistingEqualRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"record":"existing"}`)
	digest := domainsecurity.SHA256Hex(body)
	if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.PutIfAbsentWithAdditionReceipt(context.Background(), digest, body)
	if !errors.Is(err, os.ErrExist) || receipt != (SecurePrivateCASAdditionReceiptV2{}) {
		t.Fatalf("existing equal record yielded creation authority: receipt=%#v err=%v", receipt, err)
	}
}

func TestSecurePrivateCASAdditionCommitRejectsUnpreparedShardWithoutWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := "cc" + strings.Repeat("0", 62)
	shard := filepath.Join(root, digest[:2])
	if err := os.Mkdir(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutIfAbsentWithAdditionReceipt(context.Background(), digest, []byte(`{"record":"blocked"}`)); err == nil {
		t.Fatal("unprepared shard received a host-authorized commit")
	}
	entries, err := os.ReadDir(shard)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected unprepared shard was mutated: entries=%v err=%v", entries, err)
	}
}

func TestSecurePrivateCASAdditionFinalizationRejectsSameBodyObjectReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"record":"created-by-host"}`)
	digest := domainsecurity.SHA256Hex(body)
	token, err := store.PutIfAbsentWithAdditionReceipt(context.Background(), digest, body)
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, digest[:2], digest+".json")
	replacement := record + ".replacement"
	if err := os.WriteFile(replacement, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, record); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinalizeCommittedAdditions(context.Background(), []SecurePrivateCASAdditionReceiptV2{token}); err == nil {
		t.Fatal("same-body replacement retained host commit provenance")
	}
}

func TestPreparedSecurePrivateCASRecoveryAppliesOnlyFrozenResidue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"record":"committed"}`)
	digest := "dd" + strings.Repeat("0", 62)
	if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, digest[:2], "."+digest+".json-0123456789abcdef01234567.tmp")
	if err := os.WriteFile(temp, []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	committed := 0
	if err == nil && prepared != nil {
		err = prepared.VisitCommittedFiles(context.Background(), func(SecurePrivateCASFile) error {
			committed++
			return nil
		})
	}
	if err != nil || !prepared.Present() || committed != 1 {
		t.Fatalf("prepare recovery: present=%v committed=%d err=%v", prepared != nil && prepared.Present(), committed, err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatalf("revalidate prepared recovery: %v", err)
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatalf("apply prepared recovery: %v", err)
	}
	if _, err := os.Lstat(temp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prepared residue survived exact apply: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(root, digest[:2], digest+".json"))
	if err != nil || !equalPrivateCASBytes(written, body) {
		t.Fatalf("prepared recovery changed committed record: body=%q err=%v", written, err)
	}
}

func TestPreparedSecurePrivateCASRecoveryRejectsLateCandidateBeforeAnyCleanup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	digests := []string{"ee" + strings.Repeat("0", 62), "ff" + strings.Repeat("0", 62)}
	temps := make([]string, 0, len(digests))
	for index, digest := range digests {
		if err := store.PutIfAbsent(context.Background(), digest, []byte(fmt.Sprintf(`{"record":%d}`, index))); err != nil {
			t.Fatal(err)
		}
	}
	for _, digest := range digests {
		temp := filepath.Join(root, digest[:2], "."+digest+".json-0123456789abcdef01234567.tmp")
		if err := os.WriteFile(temp, []byte(`{"partial":`), 0o600); err != nil {
			t.Fatal(err)
		}
		temps = append(temps, temp)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	lateDigest := digests[1]
	late := filepath.Join(root, lateDigest[:2], "."+lateDigest+".json-89abcdef0123456701234567.tmp")
	if err := os.WriteFile(late, []byte(`{"late":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("prepared recovery absorbed a late residue")
	}
	for _, temp := range temps {
		if _, err := os.Lstat(temp); err != nil {
			t.Fatalf("late-owner drift allowed earlier cleanup of %s: %v", temp, err)
		}
	}
}

func TestPreparedSecurePrivateCASRecoveryFingerprintIncludesSafeMetadataChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	digest := "ab" + strings.Repeat("0", 62)
	if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"record":1}`)); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, digest[:2], "."+digest+".json-0123456789abcdef01234567.tmp")
	if err := os.WriteFile(temp, []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	changed := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(temp, changed, changed); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err == nil {
		t.Fatal("prepared recovery fingerprint ignored a safe metadata change")
	}
	if _, err := os.Lstat(temp); err != nil {
		t.Fatalf("metadata revalidation mutated residue: %v", err)
	}
}

type beforePrivateCASAccessAuthority struct {
	inner  SecurePrivateCASAccessAuthority
	before func()
}

func (authority *beforePrivateCASAccessAuthority) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	return authority.inner.WithPrivateCASAccess(ctx, requestedRoot, func(binding privatecasport.RootBinding) error {
		if authority.before != nil {
			authority.before()
			authority.before = nil
		}
		return access(binding)
	})
}

func TestSecurePrivateCASBindingRejectsPersistenceRootReplacementBeforeAccess(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	frozen, held := lease.FrozenRoots()
	if !held {
		_ = lease.Close()
		t.Fatal("persistence lease did not expose frozen roots")
	}
	moved := frozen.DataDir + ".moved"
	replacementInstalled := false
	defer func() {
		if replacementInstalled {
			_ = os.RemoveAll(frozen.DataDir)
			_ = os.Rename(moved, frozen.DataDir)
		}
		_ = lease.Close()
	}()
	authority := &beforePrivateCASAccessAuthority{inner: lease, before: func() {
		if err := os.Rename(frozen.DataDir, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(frozen.DataDir, 0o700); err != nil {
			t.Fatal(err)
		}
		replacementInstalled = true
	}}
	root := filepath.Join(frozen.DataDir, "private", "authority")
	if _, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority); err == nil {
		t.Fatal("replacement persistence root received private CAS authority")
	}
	if _, err := os.Lstat(filepath.Join(frozen.DataDir, "private")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement persistence tree was mutated: %v", err)
	}
}

func TestSecurePrivateCASBindingRejectsIntermediateSymlink(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	outside := filepath.Join(base, "outside")
	for _, root := range []string{roots.DataDir, roots.DurableDir, outside} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	frozen, held := lease.FrozenRoots()
	if !held {
		t.Fatal("persistence lease did not expose frozen roots")
	}
	if err := os.Symlink(outside, filepath.Join(frozen.DataDir, "private")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(frozen.DataDir, "private", "authority")
	if _, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, lease); err == nil {
		t.Fatal("intermediate symlink received private CAS authority")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("private CAS escaped through intermediate symlink: entries=%v err=%v", entries, err)
	}
}

func TestSecurePrivateCASBoundDirectoryNeverConsumesPreExistingCreateResidue(t *testing.T) {
	for _, nonEmpty := range []bool{false, true} {
		name := "empty"
		if nonEmpty {
			name = "non-empty"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			component := "private-cas"
			residue := filepath.Join(parent, domainprivatecas.CreateDirectoryResidueNameV1(component))
			if err := os.Mkdir(residue, 0o700); err != nil {
				t.Fatal(err)
			}
			if nonEmpty {
				if err := os.WriteFile(filepath.Join(residue, "unexpected"), []byte("unsafe"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root := filepath.Join(parent, component)
			_, err := openTestSecurePrivateCAS(t, root, 4096)
			if err == nil {
				t.Fatal("ordinary CAS creation consumed a pre-existing create residue")
			}
			if _, statErr := os.Lstat(root); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("pre-existing residue still created the CAS root: %v", statErr)
			}
			if info, statErr := os.Lstat(residue); statErr != nil || !info.IsDir() {
				t.Fatalf("ordinary creation mutated the pre-existing residue: info=%v err=%v", info, statErr)
			}
		})
	}
}

func TestSecurePrivateCASRejectsShardSwapBetweenCommitAndReadback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("write-readback-shard-swap"))
	body := []byte(`{"schemaVersion":1,"authority":"original"}`)
	err = securePrivateCASWriteWithHook(store.root, store.generation.shardPins, digest, body, store.maxBytes, func() {
		shard := filepath.Join(root, digest[:2])
		if renameErr := os.Rename(shard, shard+".moved"); renameErr != nil {
			t.Fatal(renameErr)
		}
		if mkdirErr := os.Mkdir(shard, 0o700); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
		if writeErr := os.WriteFile(filepath.Join(shard, digest+".json"), body, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if err == nil {
		t.Fatal("byte-identical shard replacement survived commit readback verification")
	}
}

func TestSecurePrivateCASStableReadRejectsRecordNameSwap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("stable-record-name-swap"))
	body := []byte(`{"schemaVersion":1,"authority":"stable"}`)
	if err := store.PutIfAbsent(nil, digest, body); err != nil {
		t.Fatal(err)
	}
	rootHandle, err := store.root.open()
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootHandle)
	shard, _, err := privateCASUnixOpenPinnedShard(rootHandle, store.generation.shardPins, digest[:2], false)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(shard)
	record := filepath.Join(root, digest[:2], digest+".json")
	moved := record + ".moved"
	var hookErr error
	_, _, err = privateCASUnixReadStableAtWithHook(shard, digest+".json", store.maxBytes, func() {
		if renameErr := os.Rename(record, moved); renameErr != nil {
			hookErr = renameErr
			return
		}
		hookErr = os.WriteFile(record, body, 0o600)
	})
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if err == nil {
		t.Fatal("byte-identical replacement inode survived stable record read")
	}
}

func TestSecurePrivateCASStableReadRejectsSameSizeInPlaceMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("stable-record-in-place"))
	body := []byte("aaaaaaaa")
	if err := store.PutIfAbsent(nil, digest, body); err != nil {
		t.Fatal(err)
	}
	rootHandle, err := store.root.open()
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootHandle)
	shard, _, err := privateCASUnixOpenPinnedShard(rootHandle, store.generation.shardPins, digest[:2], false)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(shard)
	record := filepath.Join(root, digest[:2], digest+".json")
	var hookErr error
	_, _, err = privateCASUnixReadStableAtWithHook(shard, digest+".json", store.maxBytes, func() {
		hookErr = os.WriteFile(record, []byte("bbbbbbbb"), 0o600)
	})
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if err == nil {
		t.Fatal("same-size in-place mutation survived stable record read")
	}
}

func TestSecurePrivateCASRejectsHardLinkedRecordAndUnsafeModes(t *testing.T) {
	t.Run("hardlink", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "private-cas")
		store, err := openTestSecurePrivateCAS(t, root, 4096)
		if err != nil {
			t.Fatal(err)
		}
		digest := domainsecurity.SHA256Hex([]byte("hard-linked-record"))
		if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(root, digest[:2], digest+".json")
		if err := os.Link(record, record+".link"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Read(nil, digest); err == nil {
			t.Fatal("hard-linked private CAS record was accepted")
		}
	})
	for _, target := range []string{"root", "shard", "record"} {
		t.Run(target, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private-cas")
			store, err := openTestSecurePrivateCAS(t, root, 4096)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("unsafe-mode:" + target))
			if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
				t.Fatal(err)
			}
			path := root
			switch target {
			case "shard":
				path = filepath.Join(root, digest[:2])
			case "record":
				path = filepath.Join(root, digest[:2], digest+".json")
			}
			if err := os.Chmod(path, 0o770); err != nil {
				t.Fatal(err)
			}
			_, readErr := store.Read(nil, digest)
			if readErr == nil || errors.Is(readErr, os.ErrNotExist) {
				t.Fatalf("unsafe %s mode classification = %v", target, readErr)
			}
		})
	}
}

func TestSecurePrivateCASInventoryHasDeterministicRecordAndAggregateBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	first := domainsecurity.SHA256Hex([]byte("inventory-bound-first"))
	second := domainsecurity.SHA256Hex([]byte("inventory-bound-second"))
	if err := store.PutIfAbsent(nil, first, []byte("123456789")); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(nil, second, []byte("abcdefghi")); err != nil {
		t.Fatal(err)
	}
	if _, err := privateCASUnixScanWithBounds(
		nil, store.root, store.generation.shardPins, store.maxBytes, false, 1, maxSecurePrivateCASListAggregateBytes,
	); !errors.Is(err, ErrSecurePrivateCASMaterializationLimit) {
		t.Fatalf("private CAS record materialization classification = %v", err)
	}
	if _, err := privateCASUnixScanWithBounds(
		nil, store.root, store.generation.shardPins, store.maxBytes, true, maxSecurePrivateCASListRecords, 8,
	); !errors.Is(err, ErrSecurePrivateCASMaterializationLimit) {
		t.Fatalf("private CAS aggregate materialization classification = %v", err)
	}
	if err := securePrivateCASValidateInventory(store.root, store.generation.shardPins, store.maxBytes); err != nil {
		t.Fatalf("eager List bounds poisoned streaming inventory validation: %v", err)
	}
	if _, err := openTestSecurePrivateCAS(t, root, 4096); err != nil {
		t.Fatalf("eager List bounds made the valid CAS unreopenable: %v", err)
	}
}

func TestSecurePrivateCASRecoveryUsesBoundedTempBatches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("bounded-recovery-temp-batches"))
	if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(root, digest[:2])
	for index := 0; index < privateCASScanPageEntries+17; index++ {
		name := fmt.Sprintf(".%s.json-%024x.tmp", digest, index+1)
		if err := os.WriteFile(filepath.Join(shard, name), []byte("residue"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recoverTestSecurePrivateCAS(t, root, 4096)
	if _, err := openTestSecurePrivateCAS(t, root, 4096); err != nil {
		t.Fatalf("bounded recovery failed: %v", err)
	}
	entries, err := os.ReadDir(shard)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != digest+".json" {
		t.Fatalf("bounded recovery left non-canonical residue: %#v", entries)
	}
}

func TestSecurePrivateCASRecoveryPreflightsAllPagesBeforeCleanup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("recovery-preflight-all-pages"))
	if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(root, digest[:2])
	for index := 0; index < privateCASScanPageEntries+17; index++ {
		name := fmt.Sprintf(".%s.json-%024x.tmp", digest, index+1)
		if err := os.WriteFile(filepath.Join(shard, name), []byte("residue"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(shard, "unknown.bin"), []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(shard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openTestSecurePrivateCAS(t, root, 4096); err == nil {
		t.Fatal("later-page unknown residue passed recovery preflight")
	}
	after, err := os.ReadDir(shard)
	if err != nil || len(after) != len(before) {
		t.Fatalf("failed recovery preflight partially cleaned state: before=%d after=%d err=%v", len(before), len(after), err)
	}
}

func TestSecurePrivateCASRecoveryPreflightsEveryShardBeforeCleanup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest := "00" + strings.Repeat("0", 62)
	lastDigest := "ff" + strings.Repeat("f", 62)
	for _, digest := range []string{firstDigest, lastDigest} {
		if err := store.PutIfAbsent(context.Background(), digest, []byte("authority")); err != nil {
			t.Fatal(err)
		}
	}
	firstTemp := filepath.Join(root, firstDigest[:2], fmt.Sprintf(".%s.json-%024x.tmp", firstDigest, 1))
	if err := os.WriteFile(firstTemp, []byte("residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, lastDigest[:2], "unknown.bin"), []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openTestSecurePrivateCAS(t, root, 4096); err == nil {
		t.Fatal("later-shard corruption passed private CAS recovery preflight")
	}
	if _, err := os.Lstat(firstTemp); err != nil {
		t.Fatalf("later-shard corruption caused earlier cleanup: %v", err)
	}
}

func TestSecurePrivateCASRecoveryRejectsUntrackedRecordHardlinkBeforeCleanup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("recovery-untracked-hardlink"))
	if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(root, digest[:2])
	temp := filepath.Join(shard, fmt.Sprintf(".%s.json-%024x.tmp", digest, 1))
	if err := os.WriteFile(temp, []byte("residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "untracked.json")
	if err := os.Link(filepath.Join(shard, digest+".json"), outside); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, err := openTestSecurePrivateCAS(t, root, 4096); err == nil {
		t.Fatal("untracked record hardlink passed recovery preflight")
	}
	if _, err := os.Stat(temp); err != nil {
		t.Fatalf("failed recovery preflight removed a valid temp: %v", err)
	}
}

func TestSecurePrivateCASAdmissionWaitHonorsContext(t *testing.T) {
	store, err := openTestSecurePrivateCAS(t, filepath.Join(t.TempDir(), "private-cas"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	<-store.gate
	defer store.release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled CAS admission wait = %v", err)
	}
}

func TestSecurePrivateCASLeaseSerializesOpenAgainstFsyncedTemp(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	if err := os.MkdirAll(roots.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(roots.DurableDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	roots, held := lease.FrozenRoots()
	if !held {
		t.Fatal("persistence lease did not expose frozen roots")
	}
	root := filepath.Join(roots.DataDir, "private", "leased-cas")
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, lease)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("leased-staged-write"))
	staged := make(chan struct{})
	release := make(chan struct{})
	store.beforeCommit = func() {
		close(staged)
		<-release
	}
	putDone := make(chan error, 1)
	go func() { putDone <- store.PutIfAbsent(context.Background(), digest, []byte("authority")) }()
	select {
	case <-staged:
	case err := <-putDone:
		t.Fatalf("write failed before the staged cut: %v", err)
	}
	openDone := make(chan error, 1)
	go func() {
		_, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, lease)
		openDone <- err
	}()
	select {
	case err := <-openDone:
		t.Fatalf("concurrent Open reached recovery while a staged write was live: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-putDone; err != nil {
		t.Fatalf("staged write was damaged by concurrent Open: %v", err)
	}
	if err := <-openDone; err != nil {
		t.Fatalf("Open failed after the live writer committed: %v", err)
	}
	store.beforeCommit = nil
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	next := domainsecurity.SHA256Hex([]byte("closed-lease-write"))
	if err := store.PutIfAbsent(context.Background(), next, []byte("forbidden")); err == nil {
		t.Fatal("closed persistence lease authorized a CAS write")
	}
	if _, err := os.Lstat(filepath.Join(root, next[:2], next+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed-lease write changed the CAS: %v", err)
	}
}
