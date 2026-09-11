//go:build darwin || linux

package finalauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestPreparedSecurePrivateCASRecoveryStreamsWithoutRetainingBodies(t *testing.T) {
	root := t.TempDir() + "/private-cas"
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 2<<20, authority)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	want := map[string][]byte{}
	for index, prefix := range []string{"11", "77", "ee"} {
		body := []byte(strings.Repeat(string(rune('a'+index)), 1<<20))
		digest := prefix + domainsecurity.SHA256Hex(body)[2:]
		if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
			t.Fatal(err)
		}
		want[digest] = body
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(
		context.Background(),
		root,
		2<<20,
		authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recoveryTypeRetainsByteSlice(reflect.TypeOf(prepared.observation), map[reflect.Type]bool{}) {
		t.Fatal("prepared private CAS recovery retained committed record bytes")
	}

	visited := map[string]bool{}
	if err := prepared.VisitCommittedFiles(context.Background(), func(file SecurePrivateCASFile) error {
		expected, ok := want[file.Digest]
		if !ok || !reflect.DeepEqual(file.Body, expected) {
			return errors.New("prepared visitor returned unexpected committed body")
		}
		visited[file.Digest] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(visited) != len(want) {
		t.Fatalf("prepared visitor count = %d, want %d", len(visited), len(want))
	}
	materials := map[string]bool{}
	if err := prepared.VisitCommittedMaterials(
		context.Background(),
		func(material SecurePrivateCASPreparedMaterialV1) error {
			expected, ok := want[material.Digest]
			if !ok || material.BodySHA256 != domainsecurity.SHA256Hex(expected) ||
				material.ByteLength != uint64(len(expected)) {
				return errors.New("prepared material visitor returned unexpected metadata")
			}
			materials[material.Digest] = true
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if len(materials) != len(want) {
		t.Fatalf("prepared material visitor count = %d, want %d", len(materials), len(want))
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorErrorAndCancellationAreFailClosed(t *testing.T) {
	root := t.TempDir() + "/private-cas"
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	body := []byte(`{"record":"streamed"}`)
	digest := domainsecurity.SHA256Hex(body)
	if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("semantic visitor rejected record")
	if err := prepared.VisitCommittedFiles(context.Background(), func(SecurePrivateCASFile) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("prepared visitor error = %v, want sentinel", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := prepared.VisitCommittedFiles(cancelled, func(SecurePrivateCASFile) error {
		t.Fatal("cancelled visitor received a record")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled prepared visitor error = %v", err)
	}
	written, err := store.Read(context.Background(), digest)
	if err != nil || !reflect.DeepEqual(written, body) {
		t.Fatalf("failed semantic visit changed committed record: body=%q err=%v", written, err)
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorCanReenterSameStoreRead(t *testing.T) {
	store, prepared, digest, body := newPreparedStreamingRecoveryFixture(t)
	defer store.Close()

	visited := 0
	err := visitPreparedRecoveryWithWatchdog(t, prepared, func(ctx context.Context, file SecurePrivateCASFile) error {
		visited++
		if file.Digest != digest || !reflect.DeepEqual(file.Body, body) {
			return errors.New("prepared visitor returned unexpected committed record")
		}
		readBack, err := store.Read(ctx, file.Digest)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(readBack, body) {
			return errors.New("reentrant private CAS read returned different bytes")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("prepared visitor could not reenter the same store Read: %v", err)
	}
	if visited != 1 {
		t.Fatalf("prepared visitor count = %d, want 1", visited)
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorCanRevalidateSamePlan(t *testing.T) {
	store, prepared, _, _ := newPreparedStreamingRecoveryFixture(t)
	defer store.Close()

	visited := 0
	err := visitPreparedRecoveryWithWatchdog(t, prepared, func(ctx context.Context, _ SecurePrivateCASFile) error {
		visited++
		return prepared.Revalidate(ctx)
	})
	if err != nil {
		t.Fatalf("prepared visitor could not revalidate the same plan: %v", err)
	}
	if visited != 1 {
		t.Fatalf("prepared visitor count = %d, want 1", visited)
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorRejectsLateResidueBeforeFirstCallback(t *testing.T) {
	store, prepared, digest, _ := newPreparedStreamingRecoveryFixture(t)
	defer store.Close()

	lateResidue := filepath.Join(
		prepared.RootPath(),
		digest[:2],
		"."+digest+".json-89abcdef0123456701234567.tmp",
	)
	if err := os.WriteFile(lateResidue, []byte(`{"late":`), 0o600); err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	err := prepared.VisitCommittedFiles(context.Background(), func(SecurePrivateCASFile) error {
		callbacks++
		return nil
	})
	if err == nil {
		t.Fatal("prepared visitor accepted residue added after preparation")
	}
	if !strings.Contains(err.Error(), "inventory changed") {
		t.Fatalf("late residue rejection = %v, want frozen inventory drift", err)
	}
	if callbacks != 0 {
		t.Fatalf("late residue reached %d callbacks, want zero", callbacks)
	}
	if _, statErr := os.Lstat(lateResidue); statErr != nil {
		t.Fatalf("failed visitor mutated late residue: %v", statErr)
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorRejectsSecondRecordReplacementBeforeCallback(t *testing.T) {
	for _, test := range []struct {
		name        string
		replacement []byte
	}{
		{name: "same body new identity", replacement: []byte(`{"record":"second"}`)},
		{name: "different body", replacement: []byte(`{"record":"replaced"}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			firstDigest := "11" + strings.Repeat("1", 62)
			secondDigest := "ee" + strings.Repeat("e", 62)
			secondBody := []byte(`{"record":"second"}`)
			store, prepared := newPreparedStreamingRecoveryRecordsFixture(t, []streamingRecoveryRecordV1{
				{digest: firstDigest, body: []byte(`{"record":"first"}`)},
				{digest: secondDigest, body: secondBody},
			})
			defer store.Close()

			callbacks := 0
			err := prepared.VisitCommittedFiles(context.Background(), func(file SecurePrivateCASFile) error {
				callbacks++
				if callbacks != 1 || file.Digest != firstDigest {
					return errors.New("replacement reached a callback outside the first frozen record")
				}
				return replaceStreamingRecoveryRecord(
					filepath.Join(prepared.RootPath(), secondDigest[:2], secondDigest+".json"),
					test.replacement,
				)
			})
			if err == nil {
				t.Fatal("prepared visitor accepted a replaced later record")
			}
			if callbacks != 1 {
				t.Fatalf("prepared visitor callbacks = %d, want only the first record", callbacks)
			}
		})
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorCancellationStopsBeforeNextRecord(t *testing.T) {
	firstDigest := "11" + strings.Repeat("1", 62)
	secondDigest := "ee" + strings.Repeat("e", 62)
	store, prepared := newPreparedStreamingRecoveryRecordsFixture(t, []streamingRecoveryRecordV1{
		{digest: firstDigest, body: []byte(`{"record":"first"}`)},
		{digest: secondDigest, body: []byte(`{"record":"second"}`)},
	})
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	callbacks := 0
	err := prepared.VisitCommittedFiles(ctx, func(file SecurePrivateCASFile) error {
		callbacks++
		if callbacks != 1 || file.Digest != firstDigest {
			return errors.New("cancelled visitor reached a later frozen record")
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("prepared visitor cancellation = %v, want context canceled", err)
	}
	if callbacks != 1 {
		t.Fatalf("cancelled prepared visitor callbacks = %d, want 1", callbacks)
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorMutationFailsFinalRevalidation(t *testing.T) {
	store, prepared, _, _ := newPreparedStreamingRecoveryFixture(t)
	defer store.Close()

	addedDigest := "ff" + strings.Repeat("f", 62)
	addedBody := []byte(`{"record":"callback-addition"}`)
	callbacks := 0
	err := visitPreparedRecoveryWithWatchdog(t, prepared, func(ctx context.Context, _ SecurePrivateCASFile) error {
		callbacks++
		return store.PutIfAbsent(ctx, addedDigest, addedBody)
	})
	if err == nil {
		t.Fatal("prepared visitor accepted a CAS mutation performed by its callback")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("callback mutation deadlocked instead of failing final revalidation: %v", err)
	}
	if !strings.Contains(err.Error(), "inventory changed") {
		t.Fatalf("callback mutation rejection = %v, want final inventory drift", err)
	}
	if callbacks != 1 {
		t.Fatalf("mutating prepared visitor callbacks = %d, want 1", callbacks)
	}
	added, readErr := store.Read(context.Background(), addedDigest)
	if readErr != nil || !reflect.DeepEqual(added, addedBody) {
		t.Fatalf("callback mutation did not complete before final rejection: body=%q err=%v", added, readErr)
	}
}

func TestPreparedSecurePrivateCASRecoveryVisitorExcludesApplyUntilCallbackReturns(t *testing.T) {
	digest := "77" + strings.Repeat("7", 62)
	residue := ""
	store, prepared := newPreparedStreamingRecoveryRecordsFixtureWithSetup(
		t,
		[]streamingRecoveryRecordV1{{digest: digest, body: []byte(`{"record":"apply-exclusion"}`)}},
		func(root string) error {
			residue = filepath.Join(
				root,
				digest[:2],
				"."+digest+".json-0123456789abcdef01234567.tmp",
			)
			return os.WriteFile(residue, []byte(`{"partial":`), 0o600)
		},
	)
	defer store.Close()
	var releaseCallbackOnce sync.Once
	releaseCallback := make(chan struct{})
	t.Cleanup(func() { releaseCallbackOnce.Do(func() { close(releaseCallback) }) })
	callbackEntered := make(chan struct{})
	visitDone := make(chan error, 1)
	visitCtx, cancelVisit := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelVisit()
	go func() {
		visitDone <- prepared.VisitCommittedFiles(visitCtx, func(SecurePrivateCASFile) error {
			close(callbackEntered)
			select {
			case <-releaseCallback:
				return nil
			case <-visitCtx.Done():
				return visitCtx.Err()
			}
		})
	}()
	select {
	case <-callbackEntered:
	case err := <-visitDone:
		t.Fatalf("prepared visitor ended before callback blocking point: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("prepared visitor did not reach its callback")
	}

	var releaseApplyOnce sync.Once
	releaseApply := make(chan struct{})
	t.Cleanup(func() { releaseApplyOnce.Do(func() { close(releaseApply) }) })
	applyEnteredTransaction := make(chan struct{})
	setPrivateCASRecoveryTransactionHook(t, func(phase string, index int) error {
		if phase == "before_plan_stage" && index == 0 {
			close(applyEnteredTransaction)
			<-releaseApply
		}
		return nil
	})
	applyStarted := make(chan struct{})
	applyDone := make(chan error, 1)
	applyCtx, cancelApply := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelApply()
	go func() {
		close(applyStarted)
		applyDone <- prepared.Apply(applyCtx)
	}()
	<-applyStarted
	select {
	case <-applyEnteredTransaction:
		t.Fatal("Apply entered its modifying transaction while the Visit callback was blocked")
	case err := <-applyDone:
		t.Fatalf("Apply ended while the Visit callback was blocked: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := os.Lstat(residue); err != nil {
		t.Fatalf("Apply changed prepared residue while Visit callback was blocked: %v", err)
	}

	releaseCallbackOnce.Do(func() { close(releaseCallback) })
	select {
	case err := <-visitDone:
		if err != nil {
			t.Fatalf("prepared visitor did not finish after callback release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prepared visitor did not finish after callback release")
	}
	select {
	case <-applyEnteredTransaction:
	case err := <-applyDone:
		t.Fatalf("Apply ended before reaching its transaction after Visit release: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Apply did not enter its transaction after Visit release")
	}
	if _, err := os.Lstat(residue); err != nil {
		t.Fatalf("Apply changed residue before its transaction hook was released: %v", err)
	}
	releaseApplyOnce.Do(func() { close(releaseApply) })
	select {
	case err := <-applyDone:
		if err != nil {
			t.Fatalf("Apply did not finish after Visit and transaction release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Apply did not finish after Visit and transaction release")
	}
	if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful Apply retained exact prepared residue: %v", err)
	}
}

type streamingRecoveryRecordV1 struct {
	digest string
	body   []byte
}

func newPreparedStreamingRecoveryFixture(
	t *testing.T,
) (*SecurePrivateCAS, *PreparedSecurePrivateCASRecoveryV1, string, []byte) {
	t.Helper()
	body := []byte(`{"record":"reentrant-stream"}`)
	digest := domainsecurity.SHA256Hex(body)
	store, prepared := newPreparedStreamingRecoveryRecordsFixture(t, []streamingRecoveryRecordV1{
		{digest: digest, body: body},
	})
	return store, prepared, digest, body
}

func newPreparedStreamingRecoveryRecordsFixture(
	t *testing.T,
	records []streamingRecoveryRecordV1,
) (*SecurePrivateCAS, *PreparedSecurePrivateCASRecoveryV1) {
	return newPreparedStreamingRecoveryRecordsFixtureWithSetup(t, records, nil)
}

func newPreparedStreamingRecoveryRecordsFixtureWithSetup(
	t *testing.T,
	records []streamingRecoveryRecordV1,
	beforePrepare func(string) error,
) (*SecurePrivateCAS, *PreparedSecurePrivateCASRecoveryV1) {
	t.Helper()
	root := t.TempDir() + "/private-cas"
	authority, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if err := store.PutIfAbsent(context.Background(), record.digest, record.body); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
	}
	if beforePrepare != nil {
		if err := beforePrepare(root); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
	}
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, authority)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return store, prepared
}

func replaceStreamingRecoveryRecord(path string, body []byte) error {
	replacement, err := os.CreateTemp(filepath.Dir(path), ".streaming-recovery-replacement-*")
	if err != nil {
		return err
	}
	replacementPath := replacement.Name()
	defer os.Remove(replacementPath)
	if _, err := replacement.Write(body); err != nil {
		_ = replacement.Close()
		return err
	}
	if err := replacement.Sync(); err != nil {
		_ = replacement.Close()
		return err
	}
	if err := replacement.Close(); err != nil {
		return err
	}
	return os.Rename(replacementPath, path)
}

func visitPreparedRecoveryWithWatchdog(
	t *testing.T,
	prepared *PreparedSecurePrivateCASRecoveryV1,
	visit func(context.Context, SecurePrivateCASFile) error,
) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- prepared.VisitCommittedFiles(ctx, func(file SecurePrivateCASFile) error {
			return visit(ctx, file)
		})
	}()
	select {
	case err := <-result:
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("reentrant prepared recovery visitor blocked until its context deadline: %v", err)
		}
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("reentrant prepared recovery visitor ignored its context deadline")
		return nil
	}
}

func recoveryTypeRetainsByteSlice(current reflect.Type, seen map[reflect.Type]bool) bool {
	if current == nil || seen[current] {
		return false
	}
	seen[current] = true
	if current.Kind() == reflect.Slice && current.Elem().Kind() == reflect.Uint8 {
		return true
	}
	switch current.Kind() {
	case reflect.Array, reflect.Pointer, reflect.Slice:
		return recoveryTypeRetainsByteSlice(current.Elem(), seen)
	case reflect.Map:
		return recoveryTypeRetainsByteSlice(current.Key(), seen) ||
			recoveryTypeRetainsByteSlice(current.Elem(), seen)
	case reflect.Struct:
		for index := 0; index < current.NumField(); index++ {
			if recoveryTypeRetainsByteSlice(current.Field(index).Type, seen) {
				return true
			}
		}
	}
	return false
}
