package evidenceauthorityhostlocal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type indeterminateHostLocalProjection struct {
	inner       authorityprojectionport.Store
	afterCommit func()
}

func advanceHostLocalForTest(ctx context.Context, store *HostLocalStore, expected, next domainhost.HeadV1) error {
	return store.WithProtectedMutation(ctx, func(mutationContext context.Context, mutation evidenceauthorityport.HostLocalMutation) error {
		return mutation.AdvanceExact(mutationContext, expected, next)
	})
}

func (projection indeterminateHostLocalProjection) Observe(ctx context.Context) (authorityprojectionport.Observation, error) {
	return projection.inner.Observe(ctx)
}

func (projection indeterminateHostLocalProjection) ReplaceExact(ctx context.Context, expected string, body []byte) (authorityprojectionport.ReplaceResult, error) {
	result, err := projection.inner.ReplaceExact(ctx, expected, body)
	if err != nil || result.State != authorityprojectionport.Committed {
		return result, err
	}
	if projection.afterCommit != nil {
		projection.afterCommit()
	}
	return authorityprojectionport.ReplaceResult{State: authorityprojectionport.Indeterminate, Digest: result.Digest},
		authorityprojectionport.ErrCommitIndeterminate
}

func (projection indeterminateHostLocalProjection) ReconcileExact(ctx context.Context, expected, selected string) (authorityprojectionport.ReplaceResult, error) {
	return projection.inner.ReconcileExact(ctx, expected, selected)
}

func TestHostLocalStoreCommitRestartAndExactSuccessor(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	privateRoot := filepath.Join(root, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	installationID := domainsecurity.SHA256Hex([]byte("host-local-installation"))
	rootBinding := domainsecurity.SHA256Hex([]byte("host-local-root"))
	historyRoot := filepath.Join(privateRoot, "evidence-authority-host-local", "heads")
	selectorRoot := filepath.Join(privateRoot, "evidence-authority-host-local-projection")
	open := func() *HostLocalStore {
		t.Helper()
		store, err := OpenHostLocalStore(ctx, historyRoot, selectorRoot, access, installationID, rootBinding, authority)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	store := open()
	if _, found, err := store.Current(ctx); err != nil || found {
		t.Fatalf("empty store acquired authority: found=%v err=%v", found, err)
	}
	input := domainhost.HeadInputV1{
		InstallationID: installationID, RootBindingDigest: rootBinding,
		MutationID:                  domainsecurity.SHA256Hex([]byte("host mode commitment")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}
	sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
	genesis, err := domainhost.NewHeadV1(input, sign)
	if err != nil || store.CommitGenesis(ctx, genesis) != nil {
		t.Fatalf("host mode was not durably committed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = open()
	current, found, err := store.Current(ctx)
	if err != nil || !found || current != genesis {
		t.Fatalf("restart lost mode commitment: found=%v err=%v", found, err)
	}
	input.Generation, input.PreviousHeadDigest = 1, genesis.RecordDigest
	input.MutationID = domainsecurity.SHA256Hex([]byte("first dataset"))
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("dataset index"))
	input.DatasetSnapshotCount = 1
	next, err := domainhost.NewHeadV1(input, sign)
	if err != nil || advanceHostLocalForTest(ctx, store, genesis, next) != nil {
		t.Fatalf("exact successor did not commit: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = open()
	defer store.Close()
	current, found, err = store.Current(ctx)
	if err != nil || !found || current != next {
		t.Fatalf("restart did not select exact successor: found=%v err=%v", found, err)
	}
	committedMode, err := store.CurrentModeCommitment(ctx)
	if err != nil || committedMode != genesis {
		t.Fatalf("restart lost exact signed mode commitment: %v", err)
	}
	input.MutationID = domainsecurity.SHA256Hex([]byte("stale dataset"))
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("other dataset index"))
	stale, err := domainhost.NewHeadV1(input, sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := advanceHostLocalForTest(ctx, store, genesis, stale); err == nil {
		t.Fatal("stale expected head advanced local currentness")
	}
	current, found, err = store.Current(ctx)
	if err != nil || !found || current != next {
		t.Fatalf("stale writer changed selected head: found=%v err=%v", found, err)
	}
	input.Generation, input.PreviousHeadDigest = 2, next.RecordDigest
	input.MutationID = domainsecurity.SHA256Hex([]byte("second dataset"))
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("dataset index 2"))
	input.DatasetSnapshotCount = 2
	second, err := domainhost.NewHeadV1(input, sign)
	if err != nil || advanceHostLocalForTest(ctx, store, next, second) != nil {
		t.Fatalf("lease-serialized second write failed after refreshing head: %v", err)
	}
	current, found, err = store.Current(ctx)
	if err != nil || !found || current != second {
		t.Fatalf("second write did not select its exact head: found=%v err=%v", found, err)
	}
	input.Generation, input.PreviousHeadDigest = 3, second.RecordDigest
	input.DatasetSnapshotCount = 3
	input.MutationID = domainsecurity.SHA256Hex([]byte("racing child A"))
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("racing index A"))
	childA, err := domainhost.NewHeadV1(input, sign)
	if err != nil {
		t.Fatal(err)
	}
	input.MutationID = domainsecurity.SHA256Hex([]byte("racing child B"))
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("racing index B"))
	childB, err := domainhost.NewHeadV1(input, sign)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, child := range []domainhost.HeadV1{childA, childB} {
		child := child
		go func() {
			<-start
			results <- advanceHostLocalForTest(ctx, store, second, child)
		}()
	}
	close(start)
	firstResult, secondResult := <-results, <-results
	if !((firstResult == nil && errors.Is(secondResult, ErrHostLocalConflict)) ||
		(secondResult == nil && errors.Is(firstResult, ErrHostLocalConflict))) {
		t.Fatalf("same-process racing children did not cleanly serialize: first=%v second=%v", firstResult, secondResult)
	}
	current, found, err = store.Current(ctx)
	if err != nil || !found || (current != childA && current != childB) {
		t.Fatalf("racing child left an orphan or ambiguous selector: found=%v err=%v", found, err)
	}
}

func TestHostLocalStoreCASLoserQuarantinesRatherThanPromotesOrphan(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(root, "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenHostLocalStore(ctx, filepath.Join(root, "heads"), filepath.Join(root, "selector"), access,
		domainsecurity.SHA256Hex([]byte("installation")), domainsecurity.SHA256Hex([]byte("root")), authority)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	input := domainhost.HeadInputV1{
		InstallationID: store.installationID, RootBindingDigest: store.rootBindingDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("mode")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}
	sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
	genesis, err := domainhost.NewHeadV1(input, sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitGenesis(ctx, genesis); err != nil {
		t.Fatal(err)
	}
	input.Generation, input.PreviousHeadDigest = 1, genesis.RecordDigest
	input.MutationID = domainsecurity.SHA256Hex([]byte("winner"))
	input.DatasetSnapshotCount = 1
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("winner index"))
	winner, err := domainhost.NewHeadV1(input, sign)
	if err != nil || advanceHostLocalForTest(ctx, store, genesis, winner) != nil {
		t.Fatalf("could not establish CAS winner: %v", err)
	}
	input.MutationID = domainsecurity.SHA256Hex([]byte("loser"))
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("loser index"))
	loser, err := domainhost.NewHeadV1(input, sign)
	if err != nil {
		t.Fatal(err)
	}
	genesisBody, err := domainhost.HeadBytesV1(genesis)
	if err != nil {
		t.Fatal(err)
	}
	loserBody, err := domainhost.HeadBytesV1(loser)
	if err != nil || store.history.PutIfAbsent(ctx, loser.RecordDigest, loserBody) != nil {
		t.Fatalf("external candidate could not be prepared: %v", err)
	}
	result, err := store.selector.ReplaceExact(ctx, domainsecurity.SHA256Hex(genesisBody), loserBody)
	if result.State != authorityprojectionport.NotCommitted || !errors.Is(err, authorityprojectionport.ErrCompareFailed) {
		t.Fatalf("external CAS loser did not receive conflict: result=%#v err=%v", result, err)
	}
	if _, found, err := store.Current(ctx); !errors.Is(err, ErrHostLocalConflict) || found {
		t.Fatalf("loser orphan was promoted or silently ignored: found=%v err=%v", found, err)
	}
}

func TestHostLocalStoreRejectsPreparedOrphanBeforeSelection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(root, "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenHostLocalStore(ctx, filepath.Join(root, "heads"), filepath.Join(root, "selector"), access,
		domainsecurity.SHA256Hex([]byte("installation")), domainsecurity.SHA256Hex([]byte("root")), authority)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: store.installationID, RootBindingDigest: store.rootBindingDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("mode")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainhost.HeadBytesV1(genesis)
	if err != nil || store.history.PutIfAbsent(ctx, genesis.RecordDigest, body) != nil {
		t.Fatalf("could not prepare crash-cut orphan: %v", err)
	}
	if _, found, err := store.Current(ctx); err == nil || found {
		t.Fatalf("unselected signed orphan became current: found=%v err=%v", found, err)
	}
	if err := store.CommitGenesis(ctx, genesis); err == nil {
		t.Fatal("prepared orphan silently became mode commitment")
	}
	if _, err := os.Stat(filepath.Join(root, "selector", hostLocalSelectorFileNameV1)); !os.IsNotExist(err) {
		t.Fatalf("failed genesis created selector: %v", err)
	}
}

func TestHostLocalModeIndeterminateCommitPoisonsProcessUntilFreshAdmission(t *testing.T) {
	for _, cancelAfterCommit := range []bool{false, true} {
		name := "visible-pre-durability-cut"
		if cancelAfterCommit {
			name = "post-commit-cancellation"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			privateRoot := filepath.Join(t.TempDir(), "private")
			access, err := privatecastest.NewAccessAuthority(privateRoot)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "key.json"), false)
			if err != nil {
				t.Fatal(err)
			}
			historyOwner := filepath.Join(privateRoot, "evidence-authority-host-local")
			selectorRoot := filepath.Join(privateRoot, "evidence-authority-host-local-projection")
			installationID := domainsecurity.SHA256Hex([]byte("installation"))
			rootBinding := domainsecurity.SHA256Hex([]byte("protected roots"))
			store, err := OpenHostLocalStore(ctx, filepath.Join(historyOwner, "heads"), selectorRoot,
				access, installationID, rootBinding, authority)
			if err != nil {
				t.Fatal(err)
			}
			genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
				InstallationID: installationID, RootBindingDigest: rootBinding,
				MutationID:                  domainsecurity.SHA256Hex([]byte("mode")),
				DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
				EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
				PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
				AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
			}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
			if err != nil {
				t.Fatal(err)
			}
			projection := indeterminateHostLocalProjection{inner: store.selector}
			if cancelAfterCommit {
				projection.afterCommit = cancel
			}
			store.selector = projection
			if err := store.CommitGenesis(ctx, genesis); !errors.Is(err, ErrHostLocalIndeterminate) {
				t.Fatalf("visible but uncertain mode commitment was admitted: %v", err)
			}
			fresh := context.Background()
			if _, found, err := store.Current(fresh); !errors.Is(err, ErrHostLocalIndeterminate) || found {
				t.Fatalf("new context escaped process poison: found=%v err=%v", found, err)
			}
			if err := store.CommitGenesis(fresh, genesis); err == nil {
				t.Fatal("poisoned process retried protected mode commitment")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			prepared, err := PrepareRecoveryV1(fresh, historyOwner, access)
			if err != nil || prepared.ValidateSemantics(fresh) != nil || prepared.Revalidate(fresh) != nil {
				t.Fatalf("fresh startup immutable inventory failed: %v", err)
			}
			reopened, err := OpenHostLocalStore(fresh, filepath.Join(historyOwner, "heads"), selectorRoot,
				access, installationID, rootBinding, authority)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if selected, found, err := reopened.Current(fresh); err != nil || !found || selected != genesis {
				t.Fatalf("fresh inventory did not read committed mode: found=%v err=%v", found, err)
			}
		})
	}
}

func TestHostLocalCurrentCannotEscapeConcurrentIndeterminateCommit(t *testing.T) {
	ctx := context.Background()
	privateRoot := filepath.Join(t.TempDir(), "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenHostLocalStore(ctx, filepath.Join(privateRoot, "evidence-authority-host-local", "heads"),
		filepath.Join(privateRoot, "evidence-authority-host-local-projection"), access,
		domainsecurity.SHA256Hex([]byte("installation")), domainsecurity.SHA256Hex([]byte("protected roots")), authority)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: store.installationID, RootBindingDigest: store.rootBindingDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("mode")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	store.selector = indeterminateHostLocalProjection{
		inner: store.selector,
		afterCommit: func() {
			close(entered)
			<-release
		},
	}
	commitResult := make(chan error, 1)
	go func() { commitResult <- store.CommitGenesis(ctx, genesis) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("commit did not reach visible selector cut")
	}
	readResult := make(chan error, 1)
	go func() {
		_, _, readErr := store.Current(ctx)
		readResult <- readErr
	}()
	select {
	case err := <-readResult:
		t.Fatalf("current escaped while commit outcome was unresolved: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-commitResult; !errors.Is(err, ErrHostLocalIndeterminate) {
		t.Fatalf("commit result = %v", err)
	}
	if err := <-readResult; !errors.Is(err, ErrHostLocalIndeterminate) {
		t.Fatalf("concurrent current escaped process poison: %v", err)
	}
}

func TestHostLocalRecoveryCannotAdmitForeignOrMissingSelector(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	privateRoot := filepath.Join(root, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	foreignAuthority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(root, "foreign-authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	historyOwner := filepath.Join(privateRoot, "evidence-authority-host-local")
	selectorRoot := filepath.Join(privateRoot, "evidence-authority-host-local-projection")
	foreignID := domainsecurity.SHA256Hex([]byte("foreign installation"))
	localID := domainsecurity.SHA256Hex([]byte("local installation"))
	rootBinding := domainsecurity.SHA256Hex([]byte("protected roots"))
	store, err := OpenHostLocalStore(ctx, filepath.Join(historyOwner, "heads"), selectorRoot,
		access, foreignID, rootBinding, foreignAuthority)
	if err != nil {
		t.Fatal(err)
	}
	genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: foreignID, RootBindingDigest: rootBinding,
		MutationID:                  domainsecurity.SHA256Hex([]byte("foreign mode")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              foreignAuthority.KeyID(), AuthorityPublicKey: foreignAuthority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return foreignAuthority.Sign(ctx, body) })
	if err != nil || store.CommitGenesis(ctx, genesis) != nil {
		t.Fatalf("foreign fixture could not commit: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(ctx, historyOwner, access)
	if err != nil || prepared.ValidateSemantics(ctx) != nil || prepared.Revalidate(ctx) != nil {
		t.Fatalf("signed immutable recovery plan was invalid: %v", err)
	}
	localAuthority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(root, "local-authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	local, err := OpenHostLocalStore(ctx, filepath.Join(historyOwner, "heads"), selectorRoot,
		access, localID, rootBinding, localAuthority)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := local.Current(ctx); !errors.Is(err, ErrHostLocalConflict) || found {
		t.Fatalf("foreign signed head gained installation authority after recovery: found=%v err=%v", found, err)
	}
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(selectorRoot, hostLocalSelectorFileNameV1)); err != nil {
		t.Fatal(err)
	}
	prepared, err = PrepareRecoveryV1(ctx, historyOwner, access)
	if err != nil || prepared.ValidateSemantics(ctx) != nil || prepared.Revalidate(ctx) != nil {
		t.Fatalf("immutable recovery after selector loss was invalid: %v", err)
	}
	foreign, err := OpenHostLocalStore(ctx, filepath.Join(historyOwner, "heads"), selectorRoot,
		access, foreignID, rootBinding, foreignAuthority)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	if _, found, err := foreign.Current(ctx); !errors.Is(err, ErrHostLocalConflict) || found {
		t.Fatalf("missing selector was reconstructed from recovered history: found=%v err=%v", found, err)
	}
}
