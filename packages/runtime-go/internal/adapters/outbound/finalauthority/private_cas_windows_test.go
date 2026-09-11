//go:build windows

package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"golang.org/x/sys/windows"
)

func TestSecurePrivateCASWindowsCreatesProtectedObjectsAndRejectsADS(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("windows-protected-cas"))
	body := []byte("authority")
	if err := store.PutIfAbsent(nil, digest, body); err != nil {
		t.Fatal(err)
	}
	readback, err := store.Read(nil, digest)
	if err != nil || !bytes.Equal(readback, body) {
		t.Fatalf("protected Windows CAS round trip failed: %v", err)
	}
	record := filepath.Join(root, digest[:2], digest+".json")
	if err := os.WriteFile(record+":attacker", []byte("named-stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(nil, digest); err == nil {
		t.Fatal("Windows named data stream was accepted")
	}
}

func TestSecurePrivateCASWindowsRejectsHardlinkAndBroadenedDACL(t *testing.T) {
	for _, target := range []string{"hardlink", "dacl"} {
		t.Run(target, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private-cas")
			store, err := openTestSecurePrivateCAS(t, root, 4096)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("windows-authority:" + target))
			if err := store.PutIfAbsent(nil, digest, []byte("authority")); err != nil {
				t.Fatal(err)
			}
			record := filepath.Join(root, digest[:2], digest+".json")
			if target == "hardlink" {
				if err := os.Link(record, record+".link"); err != nil {
					t.Fatal(err)
				}
			} else {
				handle, err := privateWindowsOpenAbsoluteDirectory(root, false)
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(handle)
				token, err := windows.OpenCurrentProcessToken()
				if err != nil {
					t.Fatal(err)
				}
				user, err := token.GetTokenUser()
				token.Close()
				if err != nil || user == nil || user.User.Sid == nil {
					t.Fatal("current Windows SID is unavailable")
				}
				descriptor, err := windows.SecurityDescriptorFromString(
					"D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FR;;;WD)",
				)
				if err != nil {
					t.Fatal(err)
				}
				dacl, _, err := descriptor.DACL()
				if err != nil {
					t.Fatal(err)
				}
				if err := windows.SetSecurityInfo(
					handle, windows.SE_FILE_OBJECT,
					windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
					nil, nil, dacl, nil,
				); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Read(nil, digest); err == nil {
				t.Fatalf("Windows %s mutation was accepted", target)
			}
		})
	}
}

func TestSecurePrivateCASWindowsStableReadRejectsNameSwap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("windows-stable-name-swap"))
	body := []byte("authority")
	if err := store.PutIfAbsent(nil, digest, body); err != nil {
		t.Fatal(err)
	}
	rootHandle, err := store.root.open()
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(rootHandle)
	shard, _, err := privateCASWindowsOpenPinnedShard(rootHandle, store.generation.shardPins, digest[:2], false)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(shard)
	record := filepath.Join(root, digest[:2], digest+".json")
	var hookErr error
	_, _, err = privateCASWindowsReadStableAtWithHook(shard, digest+".json", store.maxBytes, func() {
		if renameErr := os.Rename(record, record+".moved"); renameErr != nil {
			hookErr = renameErr
			return
		}
		handle, createErr := privateWindowsOpenRelative(
			shard, digest+".json", windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE,
			windows.FILE_CREATE, false,
		)
		if createErr != nil {
			hookErr = createErr
			return
		}
		file := os.NewFile(uintptr(handle), digest+".json")
		if file == nil {
			_ = windows.CloseHandle(handle)
			hookErr = os.ErrInvalid
			return
		}
		_, hookErr = file.Write(body)
		if hookErr == nil {
			hookErr = file.Sync()
		}
		if closeErr := file.Close(); hookErr == nil {
			hookErr = closeErr
		}
	})
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if err == nil {
		t.Fatal("byte-identical Windows FileID replacement survived stable read")
	}
}

func TestSecurePrivateCASWindowsRecoveryUsesBoundedTempBatches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("windows-bounded-recovery-temp-batches"))
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
		t.Fatalf("bounded Windows recovery failed: %v", err)
	}
	entries, err := os.ReadDir(shard)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != digest+".json" {
		t.Fatalf("bounded Windows recovery left non-canonical residue: %#v", entries)
	}
}

func TestSecurePrivateCASWindowsShardPinSurvivesLegitimateDirectoryMetadataChanges(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-cas")
	store, err := openTestSecurePrivateCAS(t, root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	first := domainsecurity.SHA256Hex([]byte("windows-shard-pin-first"))
	second := ""
	for attempt := 0; attempt < 4096; attempt++ {
		candidate := domainsecurity.SHA256Hex([]byte(fmt.Sprintf("windows-shard-pin-second-%d", attempt)))
		if candidate[:2] == first[:2] && candidate != first {
			second = candidate
			break
		}
	}
	if second == "" {
		t.Fatal("failed to construct a deterministic same-shard digest")
	}
	if err := store.PutIfAbsent(nil, first, []byte("first")); err != nil {
		t.Fatalf("first same-shard write failed: %v", err)
	}
	if body, err := store.Read(nil, first); err != nil || !bytes.Equal(body, []byte("first")) {
		t.Fatalf("read after shard metadata change failed: body=%q err=%v", body, err)
	}
	if files, err := store.List(nil); err != nil || len(files) != 1 {
		t.Fatalf("list after shard metadata change failed: files=%d err=%v", len(files), err)
	}
	if err := store.PutIfAbsent(nil, second, []byte("second")); err != nil {
		t.Fatalf("second same-shard write failed: %v", err)
	}
	if body, err := store.Read(nil, second); err != nil || !bytes.Equal(body, []byte("second")) {
		t.Fatalf("second same-shard read failed: body=%q err=%v", body, err)
	}
}

func TestSecurePrivateCASWindowsAdditionReceiptV2SpansBatchesAndSiblingStores(t *testing.T) {
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
		token, err := store.PutIfAbsentWithAdditionReceipt(
			context.Background(), digest, []byte(`{"digest":"`+digest+`"}`),
		)
		if err != nil {
			t.Fatalf("commit %s: %v", digest, err)
		}
		return token
	}
	firstToken := commit(first, digests[0])
	siblingToken := commit(sibling, digests[1])
	firstBatch, err := sibling.FinalizeCommittedAdditions(
		context.Background(), []SecurePrivateCASAdditionReceiptV2{firstToken, siblingToken},
	)
	if err != nil || len(firstBatch) != 2 {
		t.Fatalf("Windows sibling could not finalize shared receipt batch: %v", err)
	}
	for _, receipt := range firstBatch {
		if err := first.VerifyCommittedAddition(context.Background(), receipt); err != nil {
			t.Fatalf("Windows first store could not verify sibling-finalized receipt: %v", err)
		}
	}
	secondToken := commit(first, digests[2])
	secondBatch, err := first.FinalizeCommittedAdditions(
		context.Background(), []SecurePrivateCASAdditionReceiptV2{secondToken},
	)
	if err != nil || len(secondBatch) != 1 {
		t.Fatalf("finalize later Windows receipt batch: %v", err)
	}
	if err := sibling.VerifyCommittedAddition(context.Background(), secondBatch[0]); err != nil {
		t.Fatalf("Windows sibling could not verify later receipt batch: %v", err)
	}
	firstReceipt, siblingReceipt, secondReceipt := firstBatch[0], firstBatch[1], secondBatch[0]
	if firstReceipt.SchemaVersion != 2 || !firstReceipt.ShardCreatedByThisCall ||
		firstReceipt.ShardExistedBeforeCommit {
		t.Fatalf("first Windows receipt provenance is invalid: %#v", firstReceipt)
	}
	for _, receipt := range []SecurePrivateCASAdditionReceiptV2{secondReceipt, siblingReceipt} {
		if receipt.SchemaVersion != 2 || receipt.ShardCreatedByThisCall || !receipt.ShardExistedBeforeCommit {
			t.Fatalf("existing Windows shard receipt provenance is invalid: %#v", receipt)
		}
	}
}

func TestSecurePrivateCASWindowsSharedGenerationRejectsCanonicalInjectionAndRecoveryRevokesSiblings(t *testing.T) {
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
	body := []byte(`{"record":"authorized"}`)
	if err := first.PutIfAbsent(context.Background(), digest, body); err != nil {
		t.Fatal(err)
	}
	injected := "ac" + strings.Repeat("2", 62)
	if err := os.WriteFile(
		filepath.Join(root, "ac", injected+".json"), []byte(`{"record":"injected"}`), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := sibling.Read(context.Background(), digest); err == nil {
		t.Fatal("canonical Windows record injection entered shared host authority")
	}
	if err := os.Remove(filepath.Join(root, "ac", injected+".json")); err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []*SecurePrivateCAS{first, sibling} {
		if _, err := stale.Read(context.Background(), digest); err == nil {
			t.Fatal("Windows recovery retained a stale sibling root generation")
		}
	}
	reopened, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got, err := reopened.Read(context.Background(), digest); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("reopened Windows generation lost committed record: body=%q err=%v", got, err)
	}
}

func TestSecurePrivateCASWindowsAnchorsRejectShardAndRootReplacement(t *testing.T) {
	for _, target := range []string{"shard", "root"} {
		t.Run(target, func(t *testing.T) {
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
			if target == "shard" {
				shard := filepath.Join(root, digest[:2])
				if err := os.Rename(shard, shard+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(shard, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(shard, digest+".json"), body, 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := store.Read(context.Background(), digest); err == nil {
					t.Fatal("Windows replacement shard reused its anchored generation")
				}
				return
			}
			if err := os.Rename(root, root+".old"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := store.List(context.Background()); err == nil {
				t.Fatal("Windows replacement root reused its anchored generation")
			}
		})
	}
}

func TestPrivateCASWindowsRecoveryTransactionRollsBackAndResumes(t *testing.T) {
	t.Run("pre-commit rollback", func(t *testing.T) {
		first, firstTemp := preparedWindowsRecoveryTransactionFixture(t, "31")
		second, secondTemp := preparedWindowsRecoveryTransactionFixture(t, "32")
		setWindowsPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
			if phase == "after_plan_stage" && index == 0 {
				return errors.New("windows-pre-commit-cut")
			}
			return nil
		})
		if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV2(
			context.Background(), []*PreparedSecurePrivateCASRecoveryV1{first, second},
		); err == nil {
			t.Fatal("Windows pre-commit cut did not stop recovery")
		}
		for _, path := range []string{firstTemp, secondTemp} {
			if body, err := os.ReadFile(path); err != nil || string(body) != "windows-recovery-residue" {
				t.Fatalf("Windows rollback lost frozen residue %s: body=%q err=%v", path, body, err)
			}
		}
	})

	t.Run("post-marker resume", func(t *testing.T) {
		first, firstTemp := preparedWindowsRecoveryTransactionFixture(t, "41")
		second, secondTemp := preparedWindowsRecoveryTransactionFixture(t, "42")
		setWindowsPrivateCASRecoveryTransactionHook(t, func(phase string, _ int) error {
			if phase == "after_commit_marker" {
				return errors.New("windows-post-marker-cut")
			}
			return nil
		})
		if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV2(
			context.Background(), []*PreparedSecurePrivateCASRecoveryV1{first, second},
		); err == nil {
			t.Fatal("Windows post-marker cut did not stop recovery")
		}
		setWindowsPrivateCASRecoveryTransactionHook(t, nil)
		first = reprepareWindowsRecoveryTransactionFixture(t, first)
		second = reprepareWindowsRecoveryTransactionFixture(t, second)
		if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV2(
			context.Background(), []*PreparedSecurePrivateCASRecoveryV1{first, second},
		); err != nil {
			t.Fatalf("resume Windows committed recovery: %v", err)
		}
		for _, path := range []string{firstTemp, secondTemp} {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("resumed Windows recovery retained original residue %s: %v", path, err)
			}
		}
	})
}

func preparedWindowsRecoveryTransactionFixture(
	t *testing.T,
	shard string,
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
	if err := store.PutIfAbsent(context.Background(), digest, []byte(`{"record":"windows"}`)); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(root, shard, "."+digest+".json-0123456789abcdef01234567.tmp")
	if err := os.WriteFile(temp, []byte("windows-recovery-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, temp
}

func reprepareWindowsRecoveryTransactionFixture(
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

func setWindowsPrivateCASRecoveryTransactionHook(t *testing.T, hook func(string, int) error) {
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
