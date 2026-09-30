// Package evidenceauthorityhostlocal contains the explicitly marked local
// currentness profile for the existing evidence authority children. The
// witnessed evidenceauthority adapter remains unable to select a local head.
package evidenceauthorityhostlocal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityprojectionport "analytix.local/runtime-go/internal/ports/authorityprojection"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const (
	hostLocalSelectorFileNameV1 = "host-local-head-v1.json"
	maxHostLocalHeadBytesV1     = 64 << 10
	maxHostLocalHeadCountV1     = 100_000
)

var (
	ErrHostLocalConflict      = errors.New("host-local evidence currentness conflicts with durable state")
	ErrHostLocalIndeterminate = errors.New("host-local evidence currentness commit is indeterminate")
)

// HostLocalStore is a bounded local selector for the existing three evidence
// children. It is not an independently witnessed authority and cannot detect
// whole-profile rollback. Its caller owns the complete startup fixed point,
// empty-lineage admission, process-owner lease, and child-record readback.
type HostLocalStore struct {
	history           *finalauthorityadapter.SecurePrivateCAS
	selector          authorityprojectionport.Store
	installationID    string
	rootBindingDigest string
	authority         finalauthorityport.Authority
	mu                sync.RWMutex
	mutationMu        sync.Mutex
	poisoned          bool
}

func OpenHostLocalStore(
	ctx context.Context,
	historyRoot, selectorRoot string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
	installationID, rootBindingDigest string,
	authority finalauthorityport.Authority,
) (*HostLocalStore, error) {
	if ctx == nil || ctx.Err() != nil || !domainsecurity.IsSHA256Hex(installationID) ||
		!domainsecurity.IsSHA256Hex(rootBindingDigest) || authority == nil ||
		authority.KeyID() != domainsecurity.SHA256Hex(authority.PublicKey()) ||
		strings.TrimSpace(historyRoot) == "" || strings.TrimSpace(selectorRoot) == "" ||
		access == nil {
		return nil, errors.New("host-local store authority is invalid")
	}
	history, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, historyRoot, maxHostLocalHeadBytesV1, access,
	)
	if err != nil {
		return nil, err
	}
	selector, err := finalauthorityadapter.OpenSecureMutableProjection(
		selectorRoot, hostLocalSelectorFileNameV1, maxHostLocalHeadBytesV1,
	)
	if err != nil {
		return nil, errors.Join(err, history.Close())
	}
	return &HostLocalStore{
		history: history, selector: selector, installationID: installationID,
		rootBindingDigest: rootBindingDigest, authority: authority,
	}, nil
}

func (store *HostLocalStore) Close() error {
	if store == nil || store.history == nil {
		return nil
	}
	return store.history.Close()
}

// WithProtectedMutation serializes candidate creation for every child through
// final head CAS inside this process. The runtime process owner lease supplies
// cross-process serialization.
func (store *HostLocalStore) WithProtectedMutation(ctx context.Context, use func(context.Context, evidenceauthorityport.HostLocalMutation) error) error {
	if store == nil || ctx == nil || use == nil {
		return ErrHostLocalConflict
	}
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	capability := &hostLocalMutation{store: store, active: true}
	defer capability.close()
	return use(ctx, capability)
}

type hostLocalMutation struct {
	mu     sync.Mutex
	store  *HostLocalStore
	active bool
}

func (mutation *hostLocalMutation) AdvanceExact(ctx context.Context, expected, next domainhost.HeadV1) error {
	if mutation == nil {
		return ErrHostLocalConflict
	}
	mutation.mu.Lock()
	defer mutation.mu.Unlock()
	if !mutation.active || mutation.store == nil {
		return ErrHostLocalConflict
	}
	return mutation.store.advanceExact(ctx, expected, next)
}

func (mutation *hostLocalMutation) close() {
	mutation.mu.Lock()
	mutation.active = false
	mutation.mu.Unlock()
}

// Current rechecks the selector around a complete bounded immutable inventory.
// Neither an unselected prepared head nor a maximum-generation scan can choose
// current state. A residue is never silently reconciled from local files.
func (store *HostLocalStore) Current(ctx context.Context) (domainhost.HeadV1, bool, error) {
	if store == nil || store.history == nil || store.selector == nil || ctx == nil {
		return domainhost.HeadV1{}, false, ErrHostLocalIndeterminate
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.currentLocked(ctx)
}

// CurrentModeCommitment returns the signed generation-zero head only after
// validating the selected complete chain. It is for startup mode binding;
// protected effects must continue to call Current at their exact boundary.
func (store *HostLocalStore) CurrentModeCommitment(ctx context.Context) (domainhost.HeadV1, error) {
	selected, found, err := store.Current(ctx)
	if err != nil || !found {
		return domainhost.HeadV1{}, errors.Join(ErrHostLocalConflict, err)
	}
	head := selected
	for head.Generation > 0 {
		body, readErr := store.history.Read(ctx, head.PreviousHeadDigest)
		if readErr != nil {
			return domainhost.HeadV1{}, errors.Join(ErrHostLocalConflict, readErr)
		}
		previous, parseErr := domainhost.ParseHeadV1(body)
		if parseErr != nil || previous.RecordDigest != head.PreviousHeadDigest ||
			store.validate(previous) != nil || domainhost.ValidateHeadTransitionV1(previous, head) != nil {
			return domainhost.HeadV1{}, errors.Join(ErrHostLocalConflict, parseErr)
		}
		head = previous
	}
	if validateHostLocalGenesisV1(head) != nil {
		return domainhost.HeadV1{}, ErrHostLocalConflict
	}
	confirmed, confirmedFound, err := store.Current(ctx)
	if err != nil || !confirmedFound || confirmed != selected {
		return domainhost.HeadV1{}, errors.Join(ErrHostLocalIndeterminate, err)
	}
	return head, nil
}

// WithProtectedFinalReadV1 holds the child-writer serializer for the callback.
func (store *HostLocalStore) WithProtectedFinalReadV1(ctx context.Context, use func(context.Context) error) error {
	if store == nil || ctx == nil || use == nil || ctx.Err() != nil {
		return ErrHostLocalConflict
	}
	// Never wait while a caller may hold the durable session mutex. A Final
	// writer owns this same serializer before entering its terminal CAS.
	if !store.mutationMu.TryLock() {
		return errors.New("host-local Final authority is busy")
	}
	defer store.mutationMu.Unlock()
	lease, cancel := context.WithCancel(ctx)
	defer cancel()
	return errors.Join(use(lease), lease.Err())
}

func (store *HostLocalStore) ResolveRetainedHeadV1(ctx context.Context, digest string) (domainhost.HeadV1, error) {
	if ctx == nil || !domainsecurity.IsSHA256Hex(digest) {
		return domainhost.HeadV1{}, ErrHostLocalConflict
	}
	selected, found, err := store.Current(ctx)
	if err != nil || !found {
		return domainhost.HeadV1{}, errors.Join(ErrHostLocalConflict, err)
	}
	head := selected
	for head.RecordDigest != digest {
		if head.Generation == 0 {
			return domainhost.HeadV1{}, ErrHostLocalConflict
		}
		body, err := store.history.Read(ctx, head.PreviousHeadDigest)
		if err != nil {
			return domainhost.HeadV1{}, err
		}
		previous, err := domainhost.ParseHeadV1(body)
		if err != nil || previous.RecordDigest != head.PreviousHeadDigest || store.validate(previous) != nil || domainhost.ValidateHeadTransitionV1(previous, head) != nil {
			return domainhost.HeadV1{}, errors.Join(ErrHostLocalConflict, err)
		}
		head = previous
	}
	current, currentFound, err := store.Current(ctx)
	if err != nil || !currentFound || current != selected {
		return domainhost.HeadV1{}, errors.Join(ErrHostLocalIndeterminate, err)
	}
	return head, ctx.Err()
}

// currentLocked runs while the process-local state lock is held. An exact
// selector read can never race this process's unresolved write into a success.
// It verifies every signed record belongs to the selected chain.
func (store *HostLocalStore) currentLocked(ctx context.Context) (domainhost.HeadV1, bool, error) {
	if store.poisoned {
		return domainhost.HeadV1{}, false, ErrHostLocalIndeterminate
	}
	first, err := store.selector.Observe(ctx)
	if err != nil {
		return domainhost.HeadV1{}, false, errors.Join(ErrHostLocalIndeterminate, err)
	}
	heads := make(map[string]domainhost.HeadV1)
	bodies := make(map[string][]byte)
	err = store.history.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		if len(heads) >= maxHostLocalHeadCountV1 {
			return ErrHostLocalConflict
		}
		head, parseErr := domainhost.ParseHeadV1(file.Body)
		if parseErr != nil || head.RecordDigest != file.Digest || store.validate(head) != nil {
			return errors.Join(ErrHostLocalConflict, parseErr)
		}
		if _, exists := heads[file.Digest]; exists {
			return ErrHostLocalConflict
		}
		heads[file.Digest] = head
		bodies[file.Digest] = append([]byte(nil), file.Body...)
		return nil
	})
	if err != nil {
		return domainhost.HeadV1{}, false, errors.Join(ErrHostLocalConflict, err)
	}
	second, err := store.selector.Observe(ctx)
	if err != nil || !equalHostLocalObservationV1(first, second) {
		return domainhost.HeadV1{}, false, errors.Join(ErrHostLocalIndeterminate, err)
	}
	if !first.Present {
		if first.Digest != "" || first.Body != nil || len(heads) != 0 {
			return domainhost.HeadV1{}, false, ErrHostLocalConflict
		}
		return domainhost.HeadV1{}, false, nil
	}
	selected, err := domainhost.ParseHeadV1(first.Body)
	if err != nil || store.validate(selected) != nil || !bytes.Equal(bodies[selected.RecordDigest], first.Body) {
		return domainhost.HeadV1{}, false, errors.Join(ErrHostLocalConflict, err)
	}
	if err := validateHostLocalSelectedChainV1(selected, heads, len(heads)); err != nil {
		return domainhost.HeadV1{}, false, err
	}
	return selected, true, nil
}

// CommitGenesis requires its caller to have proved a complete empty protected
// lineage and retained the process-owner lease. The signed generation-zero
// selector is both the mode commitment and the local commit point.
func (store *HostLocalStore) CommitGenesis(ctx context.Context, genesis domainhost.HeadV1) error {
	if err := store.validate(genesis); err != nil || validateHostLocalGenesisV1(genesis) != nil {
		return errors.Join(ErrHostLocalConflict, err)
	}
	return store.installAndSelect(ctx, nil, genesis)
}

// AdvanceExact changes exactly one existing child. The caller must still
// revalidate the selected head at its concrete protected effect boundary.
func (store *HostLocalStore) advanceExact(ctx context.Context, expected, next domainhost.HeadV1) error {
	if store.validate(expected) != nil || store.validate(next) != nil ||
		domainhost.ValidateHeadTransitionV1(expected, next) != nil {
		return ErrHostLocalConflict
	}
	return store.installAndSelect(ctx, &expected, next)
}

func (store *HostLocalStore) installAndSelect(ctx context.Context, expected *domainhost.HeadV1, next domainhost.HeadV1) error {
	body, err := domainhost.HeadBytesV1(next)
	if err != nil {
		return errors.Join(ErrHostLocalConflict, err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.poisoned {
		return ErrHostLocalIndeterminate
	}
	current, found, err := store.currentLocked(ctx)
	if err != nil {
		return errors.Join(ErrHostLocalConflict, err)
	}
	if expected == nil && found || expected != nil && (!found || current != *expected) {
		return ErrHostLocalConflict
	}
	expectedSelectorDigest := ""
	if expected != nil {
		previousBody, err := domainhost.HeadBytesV1(*expected)
		if err != nil {
			return errors.Join(ErrHostLocalConflict, err)
		}
		expectedSelectorDigest = domainsecurity.SHA256Hex(previousBody)
	}
	if err := store.history.PutIfAbsent(ctx, next.RecordDigest, body); err != nil {
		store.poisoned = true
		return errors.Join(ErrHostLocalIndeterminate, err)
	}
	result, replaceErr := store.selector.ReplaceExact(ctx, expectedSelectorDigest, body)
	if result.State == authorityprojectionport.Committed {
		selected, found, readErr := store.currentLocked(ctx)
		if readErr == nil && found && selected == next {
			return nil
		}
		store.poisoned = true
		return errors.Join(ErrHostLocalIndeterminate, replaceErr, readErr)
	}
	// Visible target bytes after an atomic rename do not prove that the
	// directory sync completed. No local readback may settle this outcome.
	store.poisoned = true
	if result.State == authorityprojectionport.Indeterminate {
		return errors.Join(ErrHostLocalIndeterminate, replaceErr)
	}
	if errors.Is(replaceErr, authorityprojectionport.ErrCompareFailed) {
		return errors.Join(ErrHostLocalConflict, replaceErr)
	}
	return errors.Join(ErrHostLocalIndeterminate, replaceErr)
}

func (store *HostLocalStore) validate(head domainhost.HeadV1) error {
	if store == nil || store.authority == nil {
		return ErrHostLocalConflict
	}
	return domainhost.ValidateHeadForInstallationV1(head, store.installationID,
		store.rootBindingDigest, store.authority.KeyID(), store.authority.PublicKey())
}

func validateHostLocalGenesisV1(head domainhost.HeadV1) error {
	if head.Generation != 0 || head.PreviousHeadDigest != "" ||
		head.DatasetSnapshotIndexDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() ||
		head.EvidenceRegistryIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() ||
		head.PublicationIndexDigest != domainpublication.PublicationIndexGenesisDigestV1() {
		return ErrHostLocalConflict
	}
	return nil
}

func validateHostLocalSelectedChainV1(selected domainhost.HeadV1, heads map[string]domainhost.HeadV1, inventoryCount int) error {
	visited := make(map[string]struct{}, inventoryCount)
	current := selected
	for {
		if _, repeated := visited[current.RecordDigest]; repeated {
			return ErrHostLocalConflict
		}
		visited[current.RecordDigest] = struct{}{}
		if current.Generation == 0 {
			if validateHostLocalGenesisV1(current) != nil || len(visited) != inventoryCount {
				return ErrHostLocalConflict
			}
			return nil
		}
		previous, ok := heads[current.PreviousHeadDigest]
		if !ok || domainhost.ValidateHeadTransitionV1(previous, current) != nil {
			return fmt.Errorf("%w: missing or invalid predecessor", ErrHostLocalConflict)
		}
		current = previous
	}
}

func equalHostLocalObservationV1(left, right authorityprojectionport.Observation) bool {
	return left.Present == right.Present && left.Digest == right.Digest && bytes.Equal(left.Body, right.Body)
}
