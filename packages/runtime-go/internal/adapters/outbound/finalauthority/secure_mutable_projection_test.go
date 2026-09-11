//go:build darwin || linux

package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

const secureMutableProjectionTestName = "head.json"

func TestMain(m *testing.M) {
	if os.Getenv("ANALYTIX_MUTABLE_PROJECTION_HELPER") == "1" {
		if err := runSecureMutableProjectionProcessHelper(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	userconfigtest.Run(m)
}

func TestSecureMutableProjectionExactCreateReplaceAndObserve(t *testing.T) {
	store := newSecureMutableProjectionTestStore(t, filepath.Join(t.TempDir(), "projection"), 4096)
	absent, err := store.Observe(context.Background())
	if err != nil || absent.Present || absent.Digest != "" || absent.Body != nil {
		t.Fatalf("absent observation is not exact: observation=%#v err=%v", absent, err)
	}
	first := []byte(`{"generation":1}`)
	created, err := store.ReplaceExact(context.Background(), "", first)
	if err != nil || created.State != SecureMutableProjectionCommitted || created.Digest != secureMutableProjectionDigest(first) {
		t.Fatalf("initial CAS did not commit exactly: result=%#v err=%v", created, err)
	}
	observed, err := store.Observe(context.Background())
	if err != nil || !observed.Present || observed.Digest != created.Digest || !bytes.Equal(observed.Body, first) {
		t.Fatalf("committed projection did not round-trip: observation=%#v err=%v", observed, err)
	}
	second := []byte(`{"generation":2}`)
	replaced, err := store.ReplaceExact(context.Background(), observed.Digest, second)
	if err != nil || replaced.State != SecureMutableProjectionCommitted || replaced.Digest != secureMutableProjectionDigest(second) {
		t.Fatalf("replacement CAS did not commit exactly: result=%#v err=%v", replaced, err)
	}
	final, err := store.Observe(context.Background())
	if err != nil || !bytes.Equal(final.Body, second) || final.Digest != replaced.Digest {
		t.Fatalf("replacement was not the exact final projection: observation=%#v err=%v", final, err)
	}
	entries, err := os.ReadDir(store.root.path)
	if err != nil || len(entries) != 1 || entries[0].Name() != secureMutableProjectionTestName {
		t.Fatalf("projection root is not canonical after replacement: entries=%v err=%v", entries, err)
	}
}

func TestSecureMutableProjectionRejectsOldExpectedWithoutMutation(t *testing.T) {
	store := newSecureMutableProjectionTestStore(t, filepath.Join(t.TempDir(), "projection"), 4096)
	first := []byte("first")
	created, err := store.ReplaceExact(context.Background(), "", first)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ReplaceExact(context.Background(), strings.Repeat("0", 64), []byte("second"))
	if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
		t.Fatalf("stale expected digest was not rejected: result=%#v err=%v", result, err)
	}
	observed, err := store.Observe(context.Background())
	if err != nil || observed.Digest != created.Digest || !bytes.Equal(observed.Body, first) {
		t.Fatalf("stale CAS mutated the projection: observation=%#v err=%v", observed, err)
	}
}

func TestSecureMutableProjectionConcurrentCASHasOneWinner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	first := newSecureMutableProjectionTestStore(t, root, 4096)
	second := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := first.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result SecureMutableProjectionReplaceResult
		err    error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var wait sync.WaitGroup
	for index, candidate := range [][]byte{[]byte("candidate-a"), []byte("candidate-b")} {
		store := first
		if index == 1 {
			store = second
		}
		wait.Add(1)
		go func(store *SecureMutableProjection, body []byte) {
			defer wait.Done()
			<-start
			result, err := store.ReplaceExact(context.Background(), initial.Digest, body)
			results <- outcome{result: result, err: err}
		}(store, candidate)
	}
	close(start)
	wait.Wait()
	close(results)
	committed := 0
	rejected := 0
	for outcome := range results {
		switch outcome.result.State {
		case SecureMutableProjectionCommitted:
			if outcome.err != nil {
				t.Fatalf("winner returned an error: %v", outcome.err)
			}
			committed++
		case SecureMutableProjectionNotCommitted:
			if !errors.Is(outcome.err, ErrSecureMutableProjectionCompareFailed) {
				t.Fatalf("loser did not report compare failure: %v", outcome.err)
			}
			rejected++
		default:
			t.Fatalf("concurrent CAS was indeterminate: %#v err=%v", outcome.result, outcome.err)
		}
	}
	if committed != 1 || rejected != 1 {
		t.Fatalf("concurrent CAS winners=%d rejected=%d", committed, rejected)
	}
}

func TestSecureMutableProjectionKernelLockWaitHonorsCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	first := newSecureMutableProjectionTestStore(t, root, 4096)
	second := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := first.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	first.faults = &secureMutableProjectionFaults{BeforeExpectedRecheck: func() {
		close(entered)
		<-release
	}}
	firstDone := make(chan error, 1)
	go func() {
		_, err := first.ReplaceExact(context.Background(), initial.Digest, []byte("winner"))
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first CAS did not reach the locked cut point")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := second.ReplaceExact(ctx, initial.Digest, []byte("blocked"))
	if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("kernel-lock wait ignored cancellation: result=%#v err=%v", result, err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("lock holder failed after release: %v", err)
	}
}

func TestSecureMutableProjectionCrossProcessCASHasOneWinner(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	startPath := filepath.Join(parent, "start")
	type child struct {
		command *exec.Cmd
		output  bytes.Buffer
		result  string
		ready   string
	}
	children := make([]*child, 0, 2)
	for index := 0; index < 2; index++ {
		child := &child{
			result: filepath.Join(parent, fmt.Sprintf("result-%d", index)),
			ready:  filepath.Join(parent, fmt.Sprintf("ready-%d", index)),
		}
		child.command = exec.Command(os.Args[0], "-test.run=^$", "-test.count=1")
		child.command.Env = append(os.Environ(),
			"ANALYTIX_MUTABLE_PROJECTION_HELPER=1",
			"ANALYTIX_MUTABLE_PROJECTION_ROOT="+root,
			"ANALYTIX_MUTABLE_PROJECTION_EXPECTED="+initial.Digest,
			"ANALYTIX_MUTABLE_PROJECTION_START="+startPath,
			"ANALYTIX_MUTABLE_PROJECTION_READY="+child.ready,
			"ANALYTIX_MUTABLE_PROJECTION_RESULT="+child.result,
			fmt.Sprintf("ANALYTIX_MUTABLE_PROJECTION_CANDIDATE=candidate-%d", index),
		)
		child.command.Stdout = &child.output
		child.command.Stderr = &child.output
		if err := child.command.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
	}
	for _, child := range children {
		waitForSecureMutableProjectionPath(t, child.ready)
	}
	if err := os.WriteFile(startPath, []byte("start"), 0o600); err != nil {
		t.Fatal(err)
	}
	states := make([]string, 0, 2)
	for _, child := range children {
		if err := child.command.Wait(); err != nil {
			t.Fatalf("CAS child failed: %v\n%s", err, child.output.String())
		}
		body, err := os.ReadFile(child.result)
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, string(body))
	}
	sort.Strings(states)
	expected := []string{string(SecureMutableProjectionCommitted), string(SecureMutableProjectionNotCommitted)}
	if fmt.Sprint(states) != fmt.Sprint(expected) {
		t.Fatalf("cross-process CAS outcomes=%v, want %v", states, expected)
	}
}

func runSecureMutableProjectionProcessHelper() error {
	store, err := OpenSecureMutableProjection(os.Getenv("ANALYTIX_MUTABLE_PROJECTION_ROOT"), secureMutableProjectionTestName, 4096)
	if err != nil {
		return err
	}
	if err := os.WriteFile(os.Getenv("ANALYTIX_MUTABLE_PROJECTION_READY"), []byte("ready"), 0o600); err != nil {
		return err
	}
	if err := waitForSecureMutableProjectionPathE(os.Getenv("ANALYTIX_MUTABLE_PROJECTION_START")); err != nil {
		return err
	}
	result, replaceErr := store.ReplaceExact(
		context.Background(),
		os.Getenv("ANALYTIX_MUTABLE_PROJECTION_EXPECTED"),
		[]byte(os.Getenv("ANALYTIX_MUTABLE_PROJECTION_CANDIDATE")),
	)
	if result.State == SecureMutableProjectionCommitted && replaceErr != nil {
		return replaceErr
	}
	if result.State == SecureMutableProjectionNotCommitted && !errors.Is(replaceErr, ErrSecureMutableProjectionCompareFailed) {
		return fmt.Errorf("losing helper returned unexpected error: %w", replaceErr)
	}
	if result.State == SecureMutableProjectionIndeterminate {
		return fmt.Errorf("helper CAS was indeterminate: %w", replaceErr)
	}
	return os.WriteFile(os.Getenv("ANALYTIX_MUTABLE_PROJECTION_RESULT"), []byte(result.State), 0o600)
}

func TestSecureMutableProjectionRejectsSwapAfterExpectedRecheck(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("expected"))
	if err != nil {
		t.Fatal(err)
	}
	store.faults = &secureMutableProjectionFaults{BeforeAtomicReplace: func() {
		replaceSecureMutableProjectionTestPath(t, parent, root, []byte("intruder"))
	}}
	result, err := store.ReplaceExact(context.Background(), initial.Digest, []byte("candidate"))
	if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
		t.Fatalf("path swap was not rejected and rolled back: result=%#v err=%v", result, err)
	}
	store.faults = nil
	observed, err := store.Observe(context.Background())
	if err != nil || string(observed.Body) != "intruder" {
		t.Fatalf("failed CAS did not preserve the concurrently installed projection: observation=%#v err=%v", observed, err)
	}
}

func TestSecureMutableProjectionReadbackSwapIsIndeterminate(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("expected"))
	if err != nil {
		t.Fatal(err)
	}
	store.faults = &secureMutableProjectionFaults{BeforeReadback: func() {
		replaceSecureMutableProjectionTestPath(t, parent, root, []byte("intruder"))
	}}
	result, err := store.ReplaceExact(context.Background(), initial.Digest, []byte("candidate"))
	if result.State != SecureMutableProjectionIndeterminate || !errors.Is(err, ErrSecureMutableProjectionCommitIndeterminate) {
		t.Fatalf("readback path swap did not produce indeterminate: result=%#v err=%v", result, err)
	}
}

func TestSecureMutableProjectionCommitOutcomeSurvivesPostCommitErrorAndCancellation(t *testing.T) {
	store := newSecureMutableProjectionTestStore(t, filepath.Join(t.TempDir(), "projection"), 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	postCommit := errors.New("post-commit fault")
	store.faults = &secureMutableProjectionFaults{AfterDirectorySync: func() error { return postCommit }}
	result, err := store.ReplaceExact(context.Background(), initial.Digest, []byte("durable"))
	if result.State != SecureMutableProjectionCommitted || !errors.Is(err, postCommit) {
		t.Fatalf("post-commit error hid committed outcome: result=%#v err=%v", result, err)
	}
	observed, err := store.Observe(context.Background())
	if err != nil || string(observed.Body) != "durable" {
		t.Fatalf("post-commit fault lost durable projection: observation=%#v err=%v", observed, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.faults = &secureMutableProjectionFaults{AfterDirectorySync: func() error {
		cancel()
		return nil
	}}
	result, err = store.ReplaceExact(ctx, observed.Digest, []byte("cancelled-after-commit"))
	if result.State != SecureMutableProjectionCommitted || !errors.Is(err, context.Canceled) {
		t.Fatalf("post-commit cancellation hid committed outcome: result=%#v err=%v", result, err)
	}
}

func TestSecureMutableProjectionPreDurabilityFaultIsIndeterminate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	fault := errors.New("fault after atomic replace")
	store.faults = &secureMutableProjectionFaults{AfterAtomicReplace: func() error { return fault }}
	result, err := store.ReplaceExact(context.Background(), initial.Digest, []byte("candidate"))
	if result.State != SecureMutableProjectionIndeterminate || result.Digest != secureMutableProjectionDigest([]byte("candidate")) ||
		!errors.Is(err, ErrSecureMutableProjectionCommitIndeterminate) || !errors.Is(err, fault) {
		t.Fatalf("pre-durability fault was misclassified: result=%#v err=%v", result, err)
	}
	// Re-opening does not choose the visible candidate over the exchanged-out
	// predecessor. It exposes both exact states and quarantines further CAS.
	reopened := newSecureMutableProjectionTestStore(t, root, 4096)
	observed, err := reopened.Observe(context.Background())
	var residueErr *SecureMutableProjectionResidueError
	if string(observed.Body) != "candidate" || !errors.As(err, &residueErr) || len(residueErr.Residues) != 1 ||
		string(residueErr.Residues[0].Body) != "initial" || residueErr.Residues[0].Digest != initial.Digest {
		t.Fatalf("reopen did not expose exact ambiguous states: observation=%#v residue=%#v err=%v", observed, residueErr, err)
	}
	blocked, err := reopened.ReplaceExact(context.Background(), observed.Digest, []byte("must-not-write"))
	if blocked.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionResidue) {
		t.Fatalf("residue did not quarantine replacement: result=%#v err=%v", blocked, err)
	}
	// A semantic witness/index digest is not the digest of either canonical
	// projection candidate and therefore cannot select or clean local state.
	semanticStateDigest := secureMutableProjectionDigest([]byte(`{"indexDigest":"semantic-head"}`))
	wrong, err := reopened.ReconcileExact(context.Background(), observed.Digest, semanticStateDigest)
	if wrong.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
		t.Fatalf("semantic state digest was mistaken for exact projection bytes: result=%#v err=%v", wrong, err)
	}
	stillAmbiguous, err := reopened.Observe(context.Background())
	if stillAmbiguous.Digest != observed.Digest || !errors.Is(err, ErrSecureMutableProjectionResidue) {
		t.Fatalf("failed semantic-digest reconciliation mutated or cleaned local candidates: observation=%#v err=%v", stillAmbiguous, err)
	}
	reconciled, err := reopened.ReconcileExact(context.Background(), observed.Digest, initial.Digest)
	if err != nil || reconciled.State != SecureMutableProjectionCommitted || reconciled.Digest != initial.Digest {
		t.Fatalf("external predecessor witness did not reconcile exactly: result=%#v err=%v", reconciled, err)
	}
	final, err := reopened.Observe(context.Background())
	if err != nil || final.Digest != initial.Digest || string(final.Body) != "initial" {
		t.Fatalf("witness reconciliation did not restore predecessor: observation=%#v err=%v", final, err)
	}
}

func TestSecureMutableProjectionExternalWitnessCanConfirmVisibleCrashCandidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	fault := errors.New("fault after atomic replace")
	store.faults = &secureMutableProjectionFaults{AfterAtomicReplace: func() error { return fault }}
	candidate := []byte("candidate")
	result, err := store.ReplaceExact(context.Background(), initial.Digest, candidate)
	if result.State != SecureMutableProjectionIndeterminate || !errors.Is(err, ErrSecureMutableProjectionCommitIndeterminate) {
		t.Fatalf("fault cut was not indeterminate: result=%#v err=%v", result, err)
	}
	reopened := newSecureMutableProjectionTestStore(t, root, 4096)
	candidateDigest := secureMutableProjectionDigest(candidate)
	reconciled, err := reopened.ReconcileExact(context.Background(), candidateDigest, candidateDigest)
	if err != nil || reconciled.State != SecureMutableProjectionCommitted {
		t.Fatalf("external visible-candidate witness did not reconcile: result=%#v err=%v", reconciled, err)
	}
	final, err := reopened.Observe(context.Background())
	if err != nil || string(final.Body) != "candidate" {
		t.Fatalf("visible-candidate reconciliation failed: observation=%#v err=%v", final, err)
	}
}

func TestSecureMutableProjectionCommittedCleanupErrorRemainsQuarantinedUntilExactReconcile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	cleanupFault := errors.New("cleanup fault")
	store.faults = &secureMutableProjectionFaults{BeforeCleanup: func() error { return cleanupFault }}
	next := []byte("next")
	result, err := store.ReplaceExact(context.Background(), initial.Digest, next)
	if result.State != SecureMutableProjectionCommitted || !errors.Is(err, cleanupFault) {
		t.Fatalf("cleanup fault hid the committed target: result=%#v err=%v", result, err)
	}
	store.faults = nil
	observed, err := store.Observe(context.Background())
	var residueErr *SecureMutableProjectionResidueError
	if observed.Digest != secureMutableProjectionDigest(next) || !errors.As(err, &residueErr) || len(residueErr.Residues) != 1 || residueErr.Residues[0].Digest != initial.Digest {
		t.Fatalf("committed cleanup error did not expose exact residue: observation=%#v residue=%#v err=%v", observed, residueErr, err)
	}
	nextDigest := secureMutableProjectionDigest(next)
	reconciled, err := store.ReconcileExact(context.Background(), nextDigest, nextDigest)
	if err != nil || reconciled.State != SecureMutableProjectionCommitted {
		t.Fatalf("exact visible projection digest did not reconcile cleanup residue: result=%#v err=%v", reconciled, err)
	}
	final, err := store.Observe(context.Background())
	if err != nil || final.Digest != secureMutableProjectionDigest(next) {
		t.Fatalf("cleanup reconciliation did not clear quarantine: observation=%#v err=%v", final, err)
	}
}

func TestSecureMutableProjectionStaleReconcileCannotRollbackNewerTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	first := []byte("first")
	firstResult, err := store.ReplaceExact(context.Background(), initial.Digest, first)
	if err != nil || firstResult.State != SecureMutableProjectionCommitted {
		t.Fatalf("first projection failed: result=%#v err=%v", firstResult, err)
	}
	stale, err := store.Observe(context.Background())
	if err != nil || stale.Digest != secureMutableProjectionDigest(first) {
		t.Fatalf("failed to capture stale target: observation=%#v err=%v", stale, err)
	}

	cleanupFault := errors.New("leave predecessor residue")
	store.faults = &secureMutableProjectionFaults{BeforeCleanup: func() error { return cleanupFault }}
	second := []byte("second")
	secondResult, err := store.ReplaceExact(context.Background(), stale.Digest, second)
	if secondResult.State != SecureMutableProjectionCommitted || !errors.Is(err, cleanupFault) {
		t.Fatalf("second projection did not leave the expected residue: result=%#v err=%v", secondResult, err)
	}
	store.faults = nil
	secondDigest := secureMutableProjectionDigest(second)
	result, err := store.ReconcileExact(context.Background(), stale.Digest, stale.Digest)
	if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
		t.Fatalf("stale reconciliation was not rejected: result=%#v err=%v", result, err)
	}
	observed, observeErr := store.Observe(context.Background())
	if observed.Digest != secondDigest || !errors.Is(observeErr, ErrSecureMutableProjectionResidue) {
		t.Fatalf("stale reconciliation changed the newer target: observation=%#v err=%v", observed, observeErr)
	}
	result, err = store.ReconcileExact(context.Background(), secondDigest, secondDigest)
	if result.State != SecureMutableProjectionCommitted || err != nil {
		t.Fatalf("fresh exact reconciliation failed: result=%#v err=%v", result, err)
	}
}

func TestSecureMutableProjectionReconcileRejectsTargetSwapAfterCompare(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "projection")
	store := newSecureMutableProjectionTestStore(t, root, 4096)
	initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}
	candidate := []byte("candidate")
	crash := errors.New("candidate commit is indeterminate")
	store.faults = &secureMutableProjectionFaults{AfterAtomicReplace: func() error { return crash }}
	result, err := store.ReplaceExact(context.Background(), initial.Digest, candidate)
	if result.State != SecureMutableProjectionIndeterminate || !errors.Is(err, crash) {
		t.Fatalf("failed to create reconciliation residue: result=%#v err=%v", result, err)
	}
	store.faults = &secureMutableProjectionFaults{BeforeReconcileExchange: func() {
		replaceSecureMutableProjectionTestPath(t, parent, root, []byte("third-target"))
	}}
	candidateDigest := secureMutableProjectionDigest(candidate)
	result, err = store.ReconcileExact(context.Background(), candidateDigest, initial.Digest)
	if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
		t.Fatalf("target swap during reconciliation was not rejected: result=%#v err=%v", result, err)
	}
	store.faults = nil
	observed, observeErr := store.Observe(context.Background())
	if observed.Digest != secureMutableProjectionDigest([]byte("third-target")) || !errors.Is(observeErr, ErrSecureMutableProjectionResidue) {
		t.Fatalf("reconciliation rolled back or erased the swapped target: observation=%#v err=%v", observed, observeErr)
	}
}

func TestSecureMutableProjectionCleanupRevalidatesExpectedTarget(t *testing.T) {
	t.Run("selected-target", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "projection")
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		initial, err := store.ReplaceExact(context.Background(), "", []byte("initial"))
		if err != nil {
			t.Fatal(err)
		}
		selected := []byte("selected")
		cleanupFault := errors.New("leave cleanup residue")
		store.faults = &secureMutableProjectionFaults{BeforeCleanup: func() error { return cleanupFault }}
		result, err := store.ReplaceExact(context.Background(), initial.Digest, selected)
		if result.State != SecureMutableProjectionCommitted || !errors.Is(err, cleanupFault) {
			t.Fatalf("failed to create cleanup residue: result=%#v err=%v", result, err)
		}
		store.faults = &secureMutableProjectionFaults{BeforeReconcileCleanup: func() {
			replaceSecureMutableProjectionTestPath(t, parent, root, []byte("third-target"))
		}}
		selectedDigest := secureMutableProjectionDigest(selected)
		result, err = store.ReconcileExact(context.Background(), selectedDigest, selectedDigest)
		if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
			t.Fatalf("cleanup accepted a swapped selected target: result=%#v err=%v", result, err)
		}
		store.faults = nil
		observed, observeErr := store.Observe(context.Background())
		var residueErr *SecureMutableProjectionResidueError
		if observed.Digest != secureMutableProjectionDigest([]byte("third-target")) || !errors.As(observeErr, &residueErr) ||
			len(residueErr.Residues) != 1 || residueErr.Residues[0].Digest != initial.Digest {
			t.Fatalf("cleanup erased residue after target swap: observation=%#v residue=%#v err=%v", observed, residueErr, observeErr)
		}
	})

	t.Run("selected-absence", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		residueName := "." + secureMutableProjectionTestName + "-" + strings.Repeat("b", 24) + ".tmp"
		if err := os.WriteFile(filepath.Join(root, residueName), []byte("prepared"), 0o600); err != nil {
			t.Fatal(err)
		}
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		store.faults = &secureMutableProjectionFaults{BeforeReconcileCleanup: func() {
			if err := os.WriteFile(filepath.Join(root, secureMutableProjectionTestName), []byte("third-target"), 0o600); err != nil {
				t.Fatal(err)
			}
		}}
		result, err := store.ReconcileExact(context.Background(), "", "")
		if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, ErrSecureMutableProjectionCompareFailed) {
			t.Fatalf("absence cleanup accepted a newly created target: result=%#v err=%v", result, err)
		}
		store.faults = nil
		observed, observeErr := store.Observe(context.Background())
		var residueErr *SecureMutableProjectionResidueError
		if observed.Digest != secureMutableProjectionDigest([]byte("third-target")) || !errors.As(observeErr, &residueErr) || len(residueErr.Residues) != 1 {
			t.Fatalf("absence cleanup erased residue after target creation: observation=%#v residue=%#v err=%v", observed, residueErr, observeErr)
		}
	})
}

func TestSecureMutableProjectionCancellationBeforeCommitIsNotCommitted(t *testing.T) {
	store := newSecureMutableProjectionTestStore(t, filepath.Join(t.TempDir(), "projection"), 4096)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := store.ReplaceExact(ctx, "", []byte("candidate"))
	if result.State != SecureMutableProjectionNotCommitted || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-commit cancellation was misclassified: result=%#v err=%v", result, err)
	}
	observed, err := store.Observe(context.Background())
	if err != nil || observed.Present {
		t.Fatalf("cancelled CAS mutated projection: observation=%#v err=%v", observed, err)
	}
}

func TestSecureMutableProjectionRejectsUnsafePathsLinksModesAndResidue(t *testing.T) {
	t.Run("root-symlink", func(t *testing.T) {
		parent := t.TempDir()
		realRoot := filepath.Join(parent, "real")
		if err := os.Mkdir(realRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(parent, "link")
		if err := os.Symlink(realRoot, link); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSecureMutableProjection(link, secureMutableProjectionTestName, 4096); err == nil {
			t.Fatal("symlink root was accepted")
		}
	})
	t.Run("root-mode", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSecureMutableProjection(root, secureMutableProjectionTestName, 4096); err == nil {
			t.Fatal("broad root permissions were accepted")
		}
	})
	t.Run("target-symlink", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "projection")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(parent, "outside")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, secureMutableProjectionTestName)); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSecureMutableProjection(root, secureMutableProjectionTestName, 4096); err == nil {
			t.Fatal("symlink projection was accepted")
		}
	})
	t.Run("target-hardlink", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "projection")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(parent, "outside")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(outside, filepath.Join(root, secureMutableProjectionTestName)); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSecureMutableProjection(root, secureMutableProjectionTestName, 4096); err == nil {
			t.Fatal("multi-link projection was accepted")
		}
	})
	t.Run("target-mode", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, secureMutableProjectionTestName), []byte("unsafe"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, secureMutableProjectionTestName), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSecureMutableProjection(root, secureMutableProjectionTestName, 4096); err == nil {
			t.Fatal("broad projection permissions were accepted")
		}
	})
	t.Run("unknown-residue", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("unsafe"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenSecureMutableProjection(root, secureMutableProjectionTestName, 4096); err == nil {
			t.Fatal("unknown root residue was accepted")
		}
	})
	t.Run("residue-added-after-open", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("unsafe"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Observe(context.Background()); err == nil {
			t.Fatal("unknown residue added after open was accepted")
		}
	})
	t.Run("crash-temp-quarantine", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "projection")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		temporary := "." + secureMutableProjectionTestName + "-" + strings.Repeat("a", 24) + ".tmp"
		if err := os.WriteFile(filepath.Join(root, temporary), []byte("prepared"), 0o600); err != nil {
			t.Fatal(err)
		}
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		observed, err := store.Observe(context.Background())
		var residueErr *SecureMutableProjectionResidueError
		if observed.Present || !errors.As(err, &residueErr) || len(residueErr.Residues) != 1 || string(residueErr.Residues[0].Body) != "prepared" {
			t.Fatalf("prepared temp was not quarantined exactly: observation=%#v residue=%#v err=%v", observed, residueErr, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 || entries[0].Name() != temporary {
			t.Fatalf("Open/Observe silently mutated crash residue: entries=%v err=%v", entries, err)
		}
		reconciled, err := store.ReconcileExact(context.Background(), "", "")
		if err != nil || reconciled.State != SecureMutableProjectionCommitted || reconciled.Digest != "" {
			t.Fatalf("external absent witness did not reconcile prepared temp: result=%#v err=%v", reconciled, err)
		}
		entries, err = os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("witness-authorized absent reconciliation left residue: entries=%v err=%v", entries, err)
		}
	})
	t.Run("root-path-swap", func(t *testing.T) {
		parent := t.TempDir()
		root := filepath.Join(parent, "projection")
		store := newSecureMutableProjectionTestStore(t, root, 4096)
		if err := os.Rename(root, filepath.Join(parent, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Observe(context.Background()); err == nil {
			t.Fatal("swapped root identity was accepted")
		}
	})
}

func TestSecureMutableProjectionRejectsInvalidAndOversizeInput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	if _, err := OpenSecureMutableProjection(root, "../head.json", 4096); err == nil {
		t.Fatal("non-relative fixed file name was accepted")
	}
	if _, err := OpenSecureMutableProjection(root, secureMutableProjectionTestName, maxPrivateAcceptedFinalBytes+1); err == nil {
		t.Fatal("oversize projection configuration was accepted")
	}
	store := newSecureMutableProjectionTestStore(t, root, 4)
	for name, input := range map[string][]byte{"empty": nil, "oversize": []byte("12345")} {
		t.Run(name, func(t *testing.T) {
			result, err := store.ReplaceExact(context.Background(), "", input)
			if result.State != SecureMutableProjectionNotCommitted || err == nil {
				t.Fatalf("invalid body was accepted: result=%#v err=%v", result, err)
			}
		})
	}
	result, err := store.ReplaceExact(context.Background(), "not-a-digest", []byte("ok"))
	if result.State != SecureMutableProjectionNotCommitted || err == nil {
		t.Fatalf("invalid expected digest was accepted: result=%#v err=%v", result, err)
	}
}

func newSecureMutableProjectionTestStore(t *testing.T, root string, maxBytes int) *SecureMutableProjection {
	t.Helper()
	store, err := OpenSecureMutableProjection(root, secureMutableProjectionTestName, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func replaceSecureMutableProjectionTestPath(t *testing.T, parent, root string, body []byte) {
	t.Helper()
	target := filepath.Join(root, secureMutableProjectionTestName)
	backup := filepath.Join(parent, fmt.Sprintf("backup-%d", time.Now().UnixNano()))
	if err := os.Rename(target, backup); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(parent, fmt.Sprintf("intruder-%d", time.Now().UnixNano()))
	if err := os.WriteFile(staged, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staged, target); err != nil {
		t.Fatal(err)
	}
}

func waitForSecureMutableProjectionPath(t *testing.T, path string) {
	t.Helper()
	if err := waitForSecureMutableProjectionPathE(path); err != nil {
		t.Fatal(err)
	}
}

func waitForSecureMutableProjectionPathE(path string) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
