package finalauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const (
	maxSecurePrivateCASShards             = 256
	privateCASScanPageEntries             = 256
	maxPrivateCASRecoveryBatches          = 1024
	maxSecurePrivateCASListRecords        = 65_536
	maxSecurePrivateCASListAggregateBytes = 128 << 20
)

var ErrSecurePrivateCASMaterializationLimit = errors.New("private CAS eager materialization limit exceeded")
var ErrSecurePrivateCASIntegrity = errors.New("private CAS integrity validation failed")

// SecurePrivateCASAtRestEncryptionEnabled is an executable product posture,
// not a capability flag. Current records are plaintext protected by private
// filesystem ownership, immutable identity, and integrity checks. It must stay
// false until the accepted POST_RC encryption decision and migration are
// implemented atomically in this owner.
const SecurePrivateCASAtRestEncryptionEnabled = false
const SecurePrivateCASAtRestProtectionV1 = "plaintext_integrity_only"

// SecurePrivateCASAccessAuthority is implemented by the host persistence
// lease. Its callback must keep that lease live across the complete operation
// and bind the requested CAS root to a frozen persistence-root identity.
type SecurePrivateCASAccessAuthority = privatecasport.AccessAuthority
type SecurePrivateCASRecoveryAccessAuthority = privatecasport.RecoveryAccessAuthority

var privateCASProcessGates = func() [64]chan struct{} {
	var gates [64]chan struct{}
	for index := range gates {
		gates[index] = make(chan struct{}, 1)
		gates[index] <- struct{}{}
	}
	return gates
}()

// Recovery holds this process-wide barrier from the final global revalidation
// through marker finalization, committed-inventory validation, and
// root-generation revocation. Live/open operations may proceed concurrently,
// but once recovery enters no preopened sibling can commit between owner or
// shard transaction phases. Waits remain context-cancellable.
var privateCASRecoveryExclusion = newPrivateCASLiveRecoveryBarrier()

type privateCASLiveRecoveryBarrier struct {
	mu       sync.Mutex
	live     int
	recovery bool
	changed  chan struct{}
}

func newPrivateCASLiveRecoveryBarrier() *privateCASLiveRecoveryBarrier {
	return &privateCASLiveRecoveryBarrier{changed: make(chan struct{})}
}

func (barrier *privateCASLiveRecoveryBarrier) acquireLive(ctx context.Context) error {
	if barrier == nil {
		return errors.New("private CAS recovery exclusion is unavailable")
	}
	for {
		barrier.mu.Lock()
		if !barrier.recovery {
			barrier.live++
			barrier.mu.Unlock()
			return nil
		}
		changed := barrier.changed
		barrier.mu.Unlock()
		if ctx == nil {
			<-changed
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (barrier *privateCASLiveRecoveryBarrier) releaseLive() {
	barrier.mu.Lock()
	if barrier.live <= 0 {
		barrier.mu.Unlock()
		panic("private CAS live recovery barrier release is unbalanced")
	}
	barrier.live--
	if barrier.live == 0 {
		barrier.notifyLocked()
	}
	barrier.mu.Unlock()
}

func (barrier *privateCASLiveRecoveryBarrier) acquireRecovery(ctx context.Context) error {
	if barrier == nil {
		return errors.New("private CAS recovery exclusion is unavailable")
	}
	for {
		barrier.mu.Lock()
		if !barrier.recovery && barrier.live == 0 {
			barrier.recovery = true
			barrier.mu.Unlock()
			return nil
		}
		changed := barrier.changed
		barrier.mu.Unlock()
		if ctx == nil {
			<-changed
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (barrier *privateCASLiveRecoveryBarrier) releaseRecovery() {
	barrier.mu.Lock()
	if !barrier.recovery {
		barrier.mu.Unlock()
		panic("private CAS recovery barrier release is unbalanced")
	}
	barrier.recovery = false
	barrier.notifyLocked()
	barrier.mu.Unlock()
}

func (barrier *privateCASLiveRecoveryBarrier) notifyLocked() {
	close(barrier.changed)
	barrier.changed = make(chan struct{})
}

// All live stores for one frozen binding and canonical CAS root share one
// host-issued root generation. The generation retains root/shard handles so a
// deleted object cannot be accepted through inode/FileID reuse, and it carries
// the complete record inventory authorized by successful no-replace commits.
// Filesystem discovery never extends this authority.
var privateCASRootGenerations = struct {
	sync.Mutex
	byRoot map[privateCASRootGenerationKey]*privateCASRootGeneration
}{byRoot: make(map[privateCASRootGenerationKey]*privateCASRootGeneration)}

// privateCASRecoveryTransactionTestHook is a deterministic same-package fault
// seam. Production never installs it. It exists so crash and drift tests can
// cut the transaction at boundaries that filesystem timing cannot reliably
// target.
var privateCASRecoveryTransactionTestHooks = struct {
	sync.RWMutex
	hook func(string, int) error
}{}

func privateCASRecoveryTransactionTestCut(phase string, index int) error {
	privateCASRecoveryTransactionTestHooks.RLock()
	hook := privateCASRecoveryTransactionTestHooks.hook
	privateCASRecoveryTransactionTestHooks.RUnlock()
	if hook == nil {
		return nil
	}
	return hook(phase, index)
}

type privateCASRootGenerationKey struct {
	binding  privatecasport.RootBinding
	rootPath string
}

type privateCASAuthorizedRecord struct {
	bodySHA256 string
}

type privateCASRootGeneration struct {
	mu               sync.Mutex
	key              privateCASRootGenerationKey
	root             privateCASRootAuthority
	rootAnchor       privateCASRootAnchor
	shardPins        map[string]privateCASShardIdentity
	shardAnchors     map[string]privateCASShardAnchor
	records          map[string]privateCASAuthorizedRecord
	additionReceipts map[string]SecurePrivateCASAdditionReceiptV2
	originalResidues privateCASOriginalResiduesV1
	originalCreates  *PreparedSecurePrivateCASOriginalCreateResiduesV1
	// Same-package deterministic fault seam. Production leaves it nil.
	beforeOriginalReadRevalidation func()
	beforeOriginalRecordStage      func() error
	maxBytes                       int
	refs                           int
	revoked                        bool
}

type privateCASInventoryFingerprint struct {
	digest  [32]byte
	records uint64
	bytes   uint64
}

// SecurePrivateCAS exposes the handle-relative immutable keyed-storage primitive
// shared by host-owned security authorities. Callers still own semantic
// parsing and signature verification. A key is caller-defined and only has a
// canonical SHA-256 shape; it is not necessarily SHA256(body). This type owns
// filesystem identity, canonical inventory, single-link files, and exact
// no-replace commits.
type SecurePrivateCAS struct {
	gate       chan struct{}
	root       privateCASRootAuthority
	binding    privatecasport.RootBinding
	rootPath   string
	maxBytes   int
	generation *privateCASRootGeneration
	access     SecurePrivateCASAccessAuthority
	closeOnce  sync.Once
	closeErr   error
	// Test-only deterministic crash/concurrency cut. Production constructors
	// leave this nil.
	beforeCommit func()
}

type SecurePrivateCASFile struct {
	Digest string
	Body   []byte
}

type SecurePrivateCASSnapshotMetadataV1 = privatecasport.SnapshotMetadataV1

const securePrivateCASAdditionReceiptVersion = 2

// SecurePrivateCASAdditionReceiptV2 binds one newly committed immutable
// record to the exact root, shard, record identities and snapshot-visible
// metadata observed through protected handles. VerifyCommittedAddition must
// sandwich the generic managed snapshot before it can authorize a rebase.
type SecurePrivateCASAdditionReceiptV2 = privatecasport.AdditionReceiptV2

type privateCASAdditionObservation struct {
	body                 []byte
	rootIdentityDigest   string
	shardIdentityDigest  string
	recordIdentityDigest string
	rootMetadata         SecurePrivateCASSnapshotMetadataV1
	shardMetadata        SecurePrivateCASSnapshotMetadataV1
	recordMetadata       SecurePrivateCASSnapshotMetadataV1
}

type privateCASRecoveryObservation struct {
	fingerprint        [32]byte
	committedMaterials []SecurePrivateCASPreparedMaterialV1
	plan               privateCASPreparedRecoveryPlan
}

// PreparedSecurePrivateCASRecoveryV1 freezes one optional CAS root and its
// complete committed/residue inventory. Revalidate is required globally before
// any owner applies cleanup; Apply never promotes a missing root.
type PreparedSecurePrivateCASRecoveryV1 struct {
	rootPath        string
	maxBytes        int
	access          SecurePrivateCASRecoveryAccessAuthority
	binding         privatecasport.RootBinding
	gate            chan struct{}
	present         bool
	authority       privateCASRootAuthority
	observation     privateCASRecoveryObservation
	originalCreates *PreparedSecurePrivateCASOriginalCreateResiduesV1
}

// SecurePrivateCASPreparedMaterialV1 exposes only the immutable metadata
// frozen by recovery preparation. It carries no record bytes and is suitable
// only for semantic validators that can prove their relation from exact
// content hashes and lengths.
type SecurePrivateCASPreparedMaterialV1 struct {
	Digest     string
	BodySHA256 string
	ByteLength uint64
}

type privateCASRecoveryTransactionState struct {
	transactionID string
	staged        int
	committed     int
	plain         int
	residues      int
}

// PreparedSecurePrivateCASRecoveryAuthorityV3 retains the owner-specific
// semantic and topology authority around one or more frozen CAS leaf plans.
// The V3 transaction revalidates this complete authority while holding the
// process-wide recovery exclusion instead of discarding it after the initial
// startup barrier.
type PreparedSecurePrivateCASRecoveryAuthorityV3 interface {
	Revalidate(context.Context) error
	SecurePrivateCASRecoveryPlansV2() []*PreparedSecurePrivateCASRecoveryV1
	PrivateCASRecoveryTopologiesV3() []SecurePrivateCASRecoveryTopologyAuthorityV3
}

type preparedSecurePrivateCASRecoveryParticipantV3 struct {
	authority     PreparedSecurePrivateCASRecoveryAuthorityV3
	plans         []*PreparedSecurePrivateCASRecoveryV1
	topologies    []SecurePrivateCASRecoveryTopologyAuthorityV3
	topologyRoots []string
}

type preparedSecurePrivateCASRecoveryAuthoritySetV3 struct {
	participants []preparedSecurePrivateCASRecoveryParticipantV3
	plans        []*PreparedSecurePrivateCASRecoveryV1
	planOwners   []int
}

func prepareSecurePrivateCASRecoveryAuthoritySetV3(
	ctx context.Context,
	authorities []PreparedSecurePrivateCASRecoveryAuthorityV3,
) (*preparedSecurePrivateCASRecoveryAuthoritySetV3, error) {
	if len(authorities) == 0 {
		return nil, errors.New("private CAS recovery transaction has no authorities")
	}
	prepared := &preparedSecurePrivateCASRecoveryAuthoritySetV3{
		participants: make([]preparedSecurePrivateCASRecoveryParticipantV3, 0, len(authorities)),
	}
	seenRoots := make(map[string]struct{})
	seenTopologyRoots := make(map[string]struct{})
	for _, authority := range authorities {
		if authority == nil {
			return nil, errors.New("private CAS recovery transaction authority is invalid")
		}
		if err := authority.Revalidate(ctx); err != nil {
			return nil, err
		}
		plans := authority.SecurePrivateCASRecoveryPlansV2()
		if len(plans) == 0 {
			return nil, errors.New("private CAS recovery transaction authority has no participants")
		}
		frozen := make([]*PreparedSecurePrivateCASRecoveryV1, len(plans))
		copy(frozen, plans)
		for _, plan := range frozen {
			if plan == nil || plan.rootPath == "" {
				return nil, errors.New("private CAS recovery transaction participant is invalid")
			}
			rootKey := strings.ToLower(filepath.Clean(plan.rootPath))
			if _, duplicate := seenRoots[rootKey]; duplicate {
				return nil, errors.New("private CAS recovery transaction repeats a root")
			}
			seenRoots[rootKey] = struct{}{}
			prepared.plans = append(prepared.plans, plan)
			prepared.planOwners = append(prepared.planOwners, len(prepared.participants))
		}
		topologies := authority.PrivateCASRecoveryTopologiesV3()
		frozenTopologies := make([]SecurePrivateCASRecoveryTopologyAuthorityV3, len(topologies))
		copy(frozenTopologies, topologies)
		frozenTopologyRoots := make([]string, 0, len(frozenTopologies))
		for _, topology := range frozenTopologies {
			if topology == nil {
				return nil, errors.New("private CAS recovery topology authority is invalid")
			}
			root := topology.PrivateCASRecoveryTopologyRootV3()
			if root == "" {
				return nil, errors.New("private CAS recovery topology root is invalid")
			}
			rootKey := strings.ToLower(filepath.Clean(root))
			if _, duplicate := seenTopologyRoots[rootKey]; duplicate {
				return nil, errors.New("private CAS recovery transaction repeats an owner topology")
			}
			seenTopologyRoots[rootKey] = struct{}{}
			frozenTopologyRoots = append(frozenTopologyRoots, root)
			if err := topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
				return nil, err
			}
		}
		prepared.participants = append(prepared.participants, preparedSecurePrivateCASRecoveryParticipantV3{
			authority:     authority,
			plans:         frozen,
			topologies:    frozenTopologies,
			topologyRoots: frozenTopologyRoots,
		})
	}
	if err := prepared.revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *preparedSecurePrivateCASRecoveryAuthoritySetV3) revalidate(ctx context.Context) error {
	if prepared == nil || len(prepared.participants) == 0 || len(prepared.plans) == 0 {
		return errors.New("private CAS recovery authority set is invalid")
	}
	for _, participant := range prepared.participants {
		if err := participant.authority.Revalidate(ctx); err != nil {
			return err
		}
		current := participant.authority.SecurePrivateCASRecoveryPlansV2()
		if len(current) != len(participant.plans) {
			return errors.New("private CAS recovery authority participant set changed")
		}
		for index := range current {
			if current[index] != participant.plans[index] {
				return errors.New("private CAS recovery authority participant identity changed")
			}
		}
		currentTopologies := participant.authority.PrivateCASRecoveryTopologiesV3()
		if len(currentTopologies) != len(participant.topologies) {
			return errors.New("private CAS recovery topology authority set changed")
		}
		for index := range currentTopologies {
			if currentTopologies[index] != participant.topologies[index] ||
				currentTopologies[index].PrivateCASRecoveryTopologyRootV3() != participant.topologyRoots[index] {
				return errors.New("private CAS recovery topology authority identity changed")
			}
		}
	}
	return nil
}

func (prepared *preparedSecurePrivateCASRecoveryAuthoritySetV3) revalidateTopology(ctx context.Context) error {
	if prepared == nil || len(prepared.participants) == 0 {
		return errors.New("private CAS recovery topology authority set is invalid")
	}
	for _, participant := range prepared.participants {
		for index, topology := range participant.topologies {
			if topology.PrivateCASRecoveryTopologyRootV3() != participant.topologyRoots[index] {
				return errors.New("private CAS recovery topology root changed")
			}
			if err := topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (prepared *preparedSecurePrivateCASRecoveryAuthoritySetV3) revalidatePlanTopology(ctx context.Context, planIndex int) error {
	if prepared == nil || planIndex < 0 || planIndex >= len(prepared.planOwners) {
		return errors.New("private CAS recovery plan topology index is invalid")
	}
	ownerIndex := prepared.planOwners[planIndex]
	if ownerIndex < 0 || ownerIndex >= len(prepared.participants) {
		return errors.New("private CAS recovery plan topology owner is invalid")
	}
	for index, topology := range prepared.participants[ownerIndex].topologies {
		if topology.PrivateCASRecoveryTopologyRootV3() != prepared.participants[ownerIndex].topologyRoots[index] {
			return errors.New("private CAS recovery plan topology root changed")
		}
		if err := topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) transactionState() (privateCASRecoveryTransactionState, error) {
	state := privateCASRecoveryTransactionState{}
	if prepared == nil || !prepared.present {
		return state, nil
	}
	for _, shard := range prepared.observation.plan.shards {
		for _, temp := range shard.temps {
			state.residues++
			switch temp.phase {
			case privateCASRecoveryQuarantinePlain:
				state.plain++
			case privateCASRecoveryQuarantineStaged, privateCASRecoveryQuarantineCommitted:
				if !validPrivateDigest(temp.transactionID) {
					return privateCASRecoveryTransactionState{}, errors.New("private CAS recovery quarantine transaction is invalid")
				}
				if state.transactionID != "" && state.transactionID != temp.transactionID {
					return privateCASRecoveryTransactionState{}, errors.New("private CAS recovery contains multiple quarantine transactions")
				}
				state.transactionID = temp.transactionID
				if temp.phase == privateCASRecoveryQuarantineCommitted {
					state.committed++
				} else {
					state.staged++
				}
			default:
				return privateCASRecoveryTransactionState{}, errors.New("private CAS recovery quarantine phase is invalid")
			}
		}
	}
	return state, nil
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) withMutationAuthority(
	ctx context.Context,
	mutate func(privateCASRootAuthority) error,
) error {
	if prepared != nil && prepared.originalCreates != nil {
		return errors.New("original creation residue CAS observation has no cleanup authority")
	}
	if prepared == nil || prepared.access == nil || prepared.gate == nil || mutate == nil {
		return errors.New("private CAS recovery mutation authority is invalid")
	}
	return withExistingPrivateCASAccess(ctx, prepared.access, prepared.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS recovery binding changed before mutation")
		}
		if err := acquirePrivateCASGate(ctx, prepared.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(prepared.gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || present != prepared.present {
			return errors.Join(errors.New("private CAS recovery presence changed before mutation"), err)
		}
		if !present {
			return mutate(privateCASRootAuthority{})
		}
		if authority != prepared.authority {
			return errors.New("private CAS recovery root identity changed before mutation")
		}
		return mutate(authority)
	})
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) createTransactionMarker(ctx context.Context, transactionID string) (bool, error) {
	created := false
	err := prepared.withMutationAuthority(ctx, func(authority privateCASRootAuthority) error {
		var err error
		created, err = securePrivateCASCreateRecoveryMarker(ctx, authority, &prepared.observation, prepared.maxBytes, transactionID)
		return err
	})
	return created, err
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) stageTransaction(ctx context.Context, transactionID string) error {
	return prepared.withMutationAuthority(ctx, func(authority privateCASRootAuthority) error {
		return securePrivateCASStagePreparedRecovery(ctx, authority, &prepared.observation, prepared.maxBytes, transactionID)
	})
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) rollbackTransaction(ctx context.Context) error {
	return prepared.withMutationAuthority(ctx, func(authority privateCASRootAuthority) error {
		return securePrivateCASRollbackPreparedRecovery(ctx, authority, &prepared.observation, prepared.maxBytes)
	})
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) markTransactionCommit(ctx context.Context, transactionID string) (bool, error) {
	marked := false
	err := prepared.withMutationAuthority(ctx, func(authority privateCASRootAuthority) error {
		var err error
		marked, err = securePrivateCASMarkPreparedRecoveryCommit(ctx, authority, &prepared.observation, prepared.maxBytes, transactionID)
		return err
	})
	return marked, err
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) commitTransaction(
	ctx context.Context,
	transactionID string,
	preserveMarker bool,
) error {
	return prepared.withMutationAuthority(ctx, func(authority privateCASRootAuthority) error {
		return securePrivateCASCommitPreparedRecovery(
			ctx, authority, &prepared.observation, prepared.maxBytes, transactionID, preserveMarker,
		)
	})
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) finalizeTransactionCommit(ctx context.Context, transactionID string) error {
	return prepared.withMutationAuthority(ctx, func(authority privateCASRootAuthority) error {
		return securePrivateCASFinalizePreparedRecoveryCommit(
			ctx, authority, &prepared.observation, prepared.maxBytes, transactionID,
		)
	})
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) validateCommittedInventory(ctx context.Context) error {
	return prepared.withMutationAuthority(ctx, func(authority privateCASRootAuthority) error {
		if !prepared.present {
			return nil
		}
		pins, err := capturePrivateCASShardIdentities(authority)
		if err != nil {
			return err
		}
		return securePrivateCASValidateInventory(authority, pins, prepared.maxBytes)
	})
}

func rollbackPreparedPrivateCASRecoveryTransactionV3(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV3,
	lastPlan int,
) error {
	if lastPlan < 0 {
		return prepared.revalidate(ctx)
	}
	if lastPlan >= len(prepared.plans) {
		return errors.New("private CAS recovery rollback range is invalid")
	}
	if err := prepared.revalidateTopology(ctx); err != nil {
		return err
	}
	var result error
	for index := lastPlan; index >= 0; index-- {
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return errors.Join(result, err)
		}
		result = errors.Join(result, prepared.plans[index].rollbackTransaction(ctx))
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return errors.Join(result, err)
		}
	}
	return errors.Join(result, prepared.revalidate(ctx))
}

func rollbackPreparedPrivateCASRecoveryTransactionV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	lastPlan int,
) error {
	if prepared == nil || prepared.v3 == nil || len(prepared.plans) != len(prepared.v3.plans) {
		return errors.New("private CAS recovery V4 rollback authority is invalid")
	}
	if lastPlan < 0 {
		return prepared.revalidate(ctx)
	}
	if lastPlan >= len(prepared.v3.plans) {
		return errors.New("private CAS recovery V4 rollback range is invalid")
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	var result error
	for index := lastPlan; index >= 0; index-- {
		if err := prepared.v3.revalidatePlanTopology(ctx, index); err != nil {
			return errors.Join(result, err)
		}
		plan := prepared.v3.plans[index]
		if prepared.plans[index].recoveryTargetsFrozen {
			state, err := plan.transactionState()
			if err != nil || state.staged != 0 || state.committed != 0 || state.transactionID != "" {
				return errors.Join(result, errors.New("frozen private CAS recovery plan contains a transaction phase"), err)
			}
		} else {
			result = errors.Join(result, plan.rollbackTransaction(ctx))
		}
		if err := prepared.v3.revalidatePlanTopology(ctx, index); err != nil {
			return errors.Join(result, err)
		}
	}
	return errors.Join(result, prepared.revalidate(ctx))
}

func commitPreparedPrivateCASRecoveryWithWitnessV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV3,
	witness verifiedPrivateCASCommitWitnessV4,
) error {
	if prepared == nil {
		return errors.New("private CAS recovery V4 commit authority is invalid")
	}
	return commitPreparedPrivateCASRecoveryWithPlanFreezeV4(
		ctx, prepared, make([]bool, len(prepared.plans)), prepared.revalidate, witness,
	)
}

func commitPreparedPrivateCASRecoveryAuthoritySetWithWitnessV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	witness verifiedPrivateCASCommitWitnessV4,
) error {
	if prepared == nil || prepared.v3 == nil || len(prepared.plans) != len(prepared.v3.plans) {
		return errors.New("private CAS recovery V4 commit authority is invalid")
	}
	frozen := make([]bool, len(prepared.plans))
	for index := range prepared.plans {
		frozen[index] = prepared.plans[index].recoveryTargetsFrozen
	}
	return commitPreparedPrivateCASRecoveryWithPlanFreezeV4(ctx, prepared.v3, frozen, prepared.revalidate, witness)
}

func commitPreparedPrivateCASRecoveryWithPlanFreezeV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV3,
	recoveryTargetsFrozen []bool,
	revalidate func(context.Context) error,
	witness verifiedPrivateCASCommitWitnessV4,
) error {
	if prepared == nil || len(recoveryTargetsFrozen) != len(prepared.plans) || revalidate == nil {
		return errors.New("private CAS recovery V4 commit plan authority is invalid")
	}
	if !validPrivateDigest(witness.transactionID) ||
		!validPrivateDigest(witness.witnessDigest) ||
		!validPrivateDigest(witness.commitTargetID) {
		return errors.New("private CAS recovery deletion witness is invalid")
	}
	transactionID := witness.transactionID
	if err := revalidate(ctx); err != nil {
		return err
	}
	plans := prepared.plans
	markerPlan := -1
	for index, plan := range plans {
		state, err := plan.transactionState()
		if err != nil {
			return err
		}
		if recoveryTargetsFrozen[index] {
			if state.staged != 0 || state.committed != 0 || state.transactionID != "" {
				return errors.New("frozen private CAS recovery plan contains a transaction phase")
			}
			continue
		}
		if state.committed == 1 {
			if markerPlan >= 0 || state.transactionID != transactionID {
				return errors.New("private CAS recovery transaction has an ambiguous commit marker")
			}
			markerPlan = index
		}
	}
	if markerPlan < 0 {
		return errors.New("private CAS recovery transaction commit marker is missing")
	}
	if err := revalidate(ctx); err != nil {
		return err
	}
	for index, plan := range plans {
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if recoveryTargetsFrozen[index] {
			continue
		}
		if err := plan.commitTransaction(ctx, transactionID, index == markerPlan); err != nil {
			return err
		}
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if err := privateCASRecoveryTransactionTestCut("after_plan_commit", index); err != nil {
			return err
		}
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
	}
	if err := revalidate(ctx); err != nil {
		return err
	}
	if err := privateCASRecoveryTransactionTestCut("before_commit_finalize", markerPlan); err != nil {
		return err
	}
	if err := revalidate(ctx); err != nil {
		return err
	}
	if err := prepared.revalidatePlanTopology(ctx, markerPlan); err != nil {
		return err
	}
	if err := plans[markerPlan].finalizeTransactionCommit(ctx, transactionID); err != nil {
		return err
	}
	if err := prepared.revalidatePlanTopology(ctx, markerPlan); err != nil {
		return err
	}
	if err := revalidate(ctx); err != nil {
		return err
	}
	for index, plan := range plans {
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if recoveryTargetsFrozen[index] {
			continue
		}
		if err := plan.validateCommittedInventory(ctx); err != nil {
			return err
		}
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if err := revokePrivateCASRootGeneration(plan.binding, plan.rootPath); err != nil {
			return err
		}
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
	}
	return revalidate(ctx)
}

func OpenSecurePrivateCASWithAccessAuthority(
	root string,
	maxBytes int,
	access SecurePrivateCASAccessAuthority,
) (*SecurePrivateCAS, error) {
	return OpenSecurePrivateCASWithAccessAuthorityContext(context.Background(), root, maxBytes, access)
}

func OpenSecurePrivateCASWithAccessAuthorityContext(
	ctx context.Context,
	root string,
	maxBytes int,
	access SecurePrivateCASAccessAuthority,
) (*SecurePrivateCAS, error) {
	if access == nil {
		return nil, errors.New("private CAS access authority is required")
	}
	return openSecurePrivateCAS(ctx, root, maxBytes, access)
}

// OpenExistingSecurePrivateCASWithAccessAuthorityContext opens an already
// initialized private CAS without creating its root or consuming crash
// residue. The returned boolean is false only when the bound root is absent.
// Hosts use this after owner recovery when a missing optional authority must
// remain missing and a partial multi-store layout must fail closed.
func OpenExistingSecurePrivateCASWithAccessAuthorityContext(
	ctx context.Context,
	root string,
	maxBytes int,
	access SecurePrivateCASAccessAuthority,
) (*SecurePrivateCAS, bool, error) {
	if strings.TrimSpace(root) == "" || maxBytes <= 0 || maxBytes > maxPrivateAcceptedFinalBytes || access == nil {
		return nil, false, errors.New("existing private CAS configuration is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return nil, false, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	requestedRoot, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return nil, false, err
	}
	var store *SecurePrivateCAS
	present := false
	err = withExistingPrivateCASAccess(ctx, access, requestedRoot, func(binding privatecasport.RootBinding) error {
		gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
		if err := acquirePrivateCASGate(ctx, gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(gate)
		authority, found, err := existingPrivateCASRootAuthority(binding)
		if err != nil || !found {
			return err
		}
		if err := securePreflightPrivateCASRoot(ctx, authority, maxBytes); err != nil {
			return err
		}
		pins, err := capturePrivateCASShardIdentities(authority)
		if err != nil {
			return err
		}
		generation, err := attachPrivateCASRootGeneration(ctx, privateCASRootGenerationKey{
			binding: binding, rootPath: requestedRoot,
		}, authority, pins, maxBytes)
		if err != nil {
			return err
		}
		candidate := &SecurePrivateCAS{
			gate: gate, root: authority, binding: binding, rootPath: requestedRoot, maxBytes: maxBytes,
			generation: generation, access: access,
		}
		runtime.SetFinalizer(candidate, func(current *SecurePrivateCAS) { _ = current.Close() })
		store = candidate
		present = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return store, present, nil
}

// SecurePrivateCASDirectoryPresentWithAccessAuthorityContext performs a
// handle-relative, no-create presence check for a private authority
// directory. It validates the bound directory identity and permissions but
// deliberately does not interpret its child inventory as a CAS.
func SecurePrivateCASDirectoryPresentWithAccessAuthorityContext(
	ctx context.Context,
	root string,
	access SecurePrivateCASAccessAuthority,
) (bool, error) {
	if strings.TrimSpace(root) == "" || access == nil {
		return false, errors.New("private authority directory configuration is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return false, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	requestedRoot, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return false, err
	}
	present := false
	err = withExistingPrivateCASAccess(ctx, access, requestedRoot, func(binding privatecasport.RootBinding) error {
		gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
		if err := acquirePrivateCASGate(ctx, gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(gate)
		_, found, err := existingPrivateCASRootAuthority(binding)
		if err != nil {
			return err
		}
		present = found
		return nil
	})
	return present, err
}

// OpenPreservingOriginalResiduesV1 opens an existing, unchanged prepared leaf
// without recovering its original plain residues or empty shards. A linked
// residue must name its exact committed partner, with exactly two links and no
// unobserved aliases. The caller owns complete owner semantics and permission
// to write independent records. Signed transaction phases and incomplete owner
// topology remain excluded.
func (prepared *PreparedSecurePrivateCASRecoveryV1) OpenPreservingOriginalResiduesV1(ctx context.Context) (*SecurePrivateCAS, error) {
	if prepared == nil || !prepared.present || prepared.access == nil || prepared.gate == nil {
		return nil, errors.New("private CAS original opening requires a present prepared leaf")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prepared.originalCreates != nil {
		if err := prepared.originalCreates.Revalidate(ctx); err != nil {
			return nil, err
		}
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return nil, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	var store *SecurePrivateCAS
	err := withExistingPrivateCASAccess(ctx, prepared.access, prepared.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS original opening binding changed")
		}
		if err := acquirePrivateCASGate(ctx, prepared.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(prepared.gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || !present || authority != prepared.authority {
			return errors.Join(errors.New("private CAS original opening identity changed"), err)
		}
		observation, err := observePrivateCASWithOriginalCreatesV1(ctx, authority, prepared.rootPath, prepared.maxBytes, prepared.originalCreates)
		if err != nil || observation.fingerprint != prepared.observation.fingerprint {
			return errors.Join(errors.New("private CAS original opening inventory changed"), err)
		}
		pins := privateCASOriginalShardPinsV1(observation.plan)
		residues, err := privateCASOriginalResiduesFromPlanV1(observation.plan, pins)
		if err != nil {
			return err
		}
		generation, err := attachPrivateCASRootGeneration(ctx, privateCASRootGenerationKey{binding: binding, rootPath: prepared.rootPath}, authority, pins, prepared.maxBytes, privateCASOriginalOpeningV1{residues: residues, creates: prepared.originalCreates})
		if err != nil {
			return err
		}
		store = &SecurePrivateCAS{gate: prepared.gate, root: authority, binding: binding, rootPath: prepared.rootPath, maxBytes: prepared.maxBytes, generation: generation, access: prepared.access}
		current, err := observePrivateCASWithOriginalCreatesV1(ctx, authority, prepared.rootPath, prepared.maxBytes, prepared.originalCreates)
		if err != nil || current.fingerprint != observation.fingerprint {
			return errors.Join(errors.New("private CAS original opening changed before activation"), err)
		}
		return ctx.Err()
	})
	if prepared.originalCreates != nil {
		err = errors.Join(err, prepared.originalCreates.Revalidate(ctx))
	}
	if err != nil {
		if store != nil {
			err = errors.Join(err, store.Close())
		}
		return nil, err
	}
	runtime.SetFinalizer(store, func(current *SecurePrivateCAS) { _ = current.Close() })
	return store, nil
}

func openSecurePrivateCAS(
	ctx context.Context,
	root string,
	maxBytes int,
	access SecurePrivateCASAccessAuthority,
) (*SecurePrivateCAS, error) {
	if strings.TrimSpace(root) == "" || maxBytes <= 0 || maxBytes > maxPrivateAcceptedFinalBytes {
		return nil, errors.New("private CAS configuration is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return nil, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	requestedRoot, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return nil, err
	}
	var store *SecurePrivateCAS
	err = withPrivateCASAccess(ctx, access, requestedRoot, func(binding privatecasport.RootBinding) error {
		gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
		if err := acquirePrivateCASGate(ctx, gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(gate)
		authority, err := newPrivateCASRootAuthority(binding)
		if err != nil {
			return err
		}
		pins, err := capturePrivateCASShardIdentities(authority)
		if err != nil {
			return err
		}
		generation, err := attachPrivateCASRootGeneration(ctx, privateCASRootGenerationKey{
			binding: binding, rootPath: requestedRoot,
		}, authority, pins, maxBytes)
		if err != nil {
			return err
		}
		candidate := &SecurePrivateCAS{
			gate: gate, root: authority, binding: binding, rootPath: requestedRoot, maxBytes: maxBytes,
			generation: generation, access: access,
		}
		runtime.SetFinalizer(candidate, func(current *SecurePrivateCAS) { _ = current.Close() })
		store = candidate
		return nil
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

func PrepareSecurePrivateCASRecoveryIfPresent(
	ctx context.Context,
	root string,
	maxBytes int,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedSecurePrivateCASRecoveryV1, error) {
	return prepareSecurePrivateCASRecoveryIfPresentV1(ctx, root, maxBytes, access, nil)
}

func prepareSecurePrivateCASRecoveryIfPresentV1(ctx context.Context, root string, maxBytes int, access SecurePrivateCASRecoveryAccessAuthority, originalCreates *PreparedSecurePrivateCASOriginalCreateResiduesV1) (*PreparedSecurePrivateCASRecoveryV1, error) {
	if strings.TrimSpace(root) == "" || maxBytes <= 0 || maxBytes > maxPrivateAcceptedFinalBytes || access == nil {
		return nil, errors.New("private CAS prepared recovery configuration is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	releaseObservation, observationErr := privateCASRecoveryExclusion.acquireObservationV1(ctx)
	if observationErr != nil {
		return nil, observationErr
	}
	defer releaseObservation()
	requestedRoot, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return nil, err
	}
	var prepared *PreparedSecurePrivateCASRecoveryV1
	err = withExistingPrivateCASAccess(ctx, access, requestedRoot, func(binding privatecasport.RootBinding) error {
		if originalCreates != nil {
			if err := originalCreates.validateLeafBindingV1(requestedRoot, binding); err != nil {
				return err
			}
		}
		gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
		if err := acquirePrivateCASGate(ctx, gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil {
			return err
		}
		prepared = &PreparedSecurePrivateCASRecoveryV1{
			rootPath: requestedRoot, maxBytes: maxBytes, access: access, binding: binding,
			gate: gate, present: present, authority: authority, originalCreates: originalCreates,
		}
		if !present {
			return nil
		}
		observation, err := observePrivateCASWithOriginalCreatesV1(ctx, authority, requestedRoot, maxBytes, originalCreates)
		if err != nil {
			return err
		}
		prepared.observation = observation
		return nil
	})
	if err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, errors.New("private CAS prepared recovery was not bound")
	}
	return prepared, nil
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) Present() bool {
	return prepared != nil && prepared.present
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) RootPath() string {
	if prepared == nil {
		return ""
	}
	return prepared.rootPath
}

// VisitCommittedFiles streams the exact committed inventory frozen by this
// recovery plan. Each exact record read completes and releases its bound
// filesystem authority before the semantic callback runs, so a callback
// cannot deadlock by re-entering a CAS that shares the same process gate.
// Complete fingerprint revalidation sandwiches the callback sequence.
// Callbacks must remain semantic-only: any mutation they perform causes the
// final revalidation to fail closed.
func (prepared *PreparedSecurePrivateCASRecoveryV1) VisitCommittedFiles(
	ctx context.Context,
	visit func(SecurePrivateCASFile) error,
) error {
	return prepared.VisitSelectedCommittedFiles(ctx, nil, visit)
}

// VisitSelectedCommittedFiles materializes only selected committed bodies.
// Selection is a pure in-memory decision over the frozen material inventory;
// every file still belongs to the complete initial/final physical revalidation.
// A nil selection visits every file, exactly as VisitCommittedFiles does.
func (prepared *PreparedSecurePrivateCASRecoveryV1) VisitSelectedCommittedFiles(
	ctx context.Context,
	selectMaterial func(SecurePrivateCASPreparedMaterialV1) bool,
	visit func(SecurePrivateCASFile) error,
) error {
	if prepared == nil || prepared.access == nil || prepared.gate == nil ||
		prepared.rootPath == "" || visit == nil {
		return errors.New("private CAS prepared recovery visitor is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	releaseObservation, observationErr := privateCASRecoveryExclusion.acquireObservationV1(ctx)
	if observationErr != nil {
		return observationErr
	}
	defer releaseObservation()
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	materials := append(
		[]SecurePrivateCASPreparedMaterialV1(nil),
		prepared.observation.committedMaterials...,
	)
	for _, material := range materials {
		if selectMaterial != nil && !selectMaterial(material) {
			if err := ctx.Err(); err != nil {
				return err
			}
			continue
		}
		body, err := prepared.readPreparedCommittedFile(ctx, material.Digest)
		if err != nil {
			return err
		}
		bodySHA256 := sha256.Sum256(body)
		if hex.EncodeToString(bodySHA256[:]) != material.BodySHA256 ||
			uint64(len(body)) != material.ByteLength {
			return errors.New("private CAS prepared recovery body disagrees with frozen material")
		}
		if err := visit(SecurePrivateCASFile{Digest: material.Digest, Body: body}); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return prepared.Revalidate(ctx)
}

// VisitCommittedMaterials visits the same frozen committed inventory without
// materializing record bodies. The initial and final complete revalidations
// make the metadata callback disposable if any concurrent mutation occurs.
func (prepared *PreparedSecurePrivateCASRecoveryV1) VisitCommittedMaterials(
	ctx context.Context,
	visit func(SecurePrivateCASPreparedMaterialV1) error,
) error {
	if prepared == nil || prepared.access == nil || prepared.gate == nil ||
		prepared.rootPath == "" || visit == nil {
		return errors.New("private CAS prepared recovery material visitor is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	releaseObservation, observationErr := privateCASRecoveryExclusion.acquireObservationV1(ctx)
	if observationErr != nil {
		return observationErr
	}
	defer releaseObservation()
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	materials := append(
		[]SecurePrivateCASPreparedMaterialV1(nil),
		prepared.observation.committedMaterials...,
	)
	for _, material := range materials {
		if !validPrivateDigest(material.Digest) || !validPrivateDigest(material.BodySHA256) ||
			material.ByteLength == 0 || material.ByteLength > uint64(prepared.maxBytes) {
			return errors.New("private CAS prepared recovery material is invalid")
		}
		if err := visit(material); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return prepared.Revalidate(ctx)
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) readPreparedCommittedFile(
	ctx context.Context,
	digest string,
) ([]byte, error) {
	if !validPrivateDigest(digest) {
		return nil, errors.New("private CAS prepared recovery record digest is invalid")
	}
	var body []byte
	err := withExistingPrivateCASAccess(ctx, prepared.access, prepared.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS prepared recovery reader binding changed")
		}
		if err := acquirePrivateCASGate(ctx, prepared.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(prepared.gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || present != prepared.present || !present {
			return errors.Join(errors.New("private CAS prepared recovery reader presence changed"), err)
		}
		if authority != prepared.authority {
			return errors.New("private CAS prepared recovery reader root identity changed")
		}
		var readErr error
		body, readErr = securePrivateCASReadPreparedCommitted(
			ctx,
			authority,
			prepared.observation,
			prepared.maxBytes,
			digest,
		)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) Revalidate(ctx context.Context) (resultErr error) {
	if prepared == nil || prepared.access == nil || prepared.gate == nil || prepared.rootPath == "" {
		return errors.New("private CAS prepared recovery is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Value(privateCASSemanticObservationContextKeyV1{}) != nil {
		releaseObservation, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
		if err != nil {
			return err
		}
		defer releaseObservation()
	}
	if prepared.originalCreates != nil {
		if err := prepared.originalCreates.Revalidate(ctx); err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, prepared.originalCreates.Revalidate(ctx)) }()
	}
	return prepared.revalidateOriginalLeafV1(ctx)
}

// Internal owner batches sandwich all leaves with the same original creation
// proof. Keep each leaf's full local observation without rescanning the whole
// catalog for every leaf. Independent callers still use Revalidate above.
func (prepared *PreparedSecurePrivateCASRecoveryV1) revalidateOriginalLeafV1(ctx context.Context) error {
	if prepared == nil || prepared.access == nil || prepared.gate == nil || prepared.rootPath == "" {
		return errors.New("private CAS prepared recovery is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Value(privateCASSemanticObservationContextKeyV1{}) != nil {
		releaseObservation, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
		if err != nil {
			return err
		}
		defer releaseObservation()
	}
	return withExistingPrivateCASAccess(ctx, prepared.access, prepared.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS prepared recovery binding changed")
		}
		if err := acquirePrivateCASGate(ctx, prepared.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(prepared.gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || present != prepared.present {
			return errors.Join(errors.New("private CAS prepared recovery presence changed"), err)
		}
		if !present {
			return nil
		}
		if authority != prepared.authority {
			return errors.New("private CAS prepared recovery root identity changed")
		}
		observation, err := observePrivateCASWithOriginalCreatesV1(ctx, authority, prepared.rootPath, prepared.maxBytes, prepared.originalCreates)
		if err != nil || observation.fingerprint != prepared.observation.fingerprint {
			return errors.Join(errors.New("private CAS prepared recovery inventory changed"), err)
		}
		return nil
	})
}

// PreflightSecurePrivateCASRecoveryIfPresent validates a current owner root
// without creating it or consuming crash residue. Hosts use this to preflight
// every owner before any owner is allowed to recover.
func PreflightSecurePrivateCASRecoveryIfPresent(
	ctx context.Context,
	root string,
	maxBytes int,
	access SecurePrivateCASRecoveryAccessAuthority,
) error {
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(ctx, root, maxBytes, access)
	if err != nil {
		return err
	}
	return prepared.Revalidate(ctx)
}

func withPrivateCASAccess(
	ctx context.Context,
	authority SecurePrivateCASAccessAuthority,
	root string,
	access func(privatecasport.RootBinding) error,
) error {
	if access == nil {
		return errors.New("private CAS access callback is required")
	}
	if authority == nil {
		return errors.New("private CAS access authority is required")
	}
	if err := authority.WithPrivateCASAccess(ctx, root, func(binding privatecasport.RootBinding) error {
		if err := validatePrivateCASRootBinding(root, binding); err != nil {
			return err
		}
		return access(binding)
	}); err != nil {
		return errors.Join(errors.New("private CAS access authority is invalid"), err)
	}
	return nil
}

func withExistingPrivateCASAccess(
	ctx context.Context,
	authority SecurePrivateCASAccessAuthority,
	root string,
	access func(privatecasport.RootBinding) error,
) error {
	if access == nil {
		return errors.New("existing private CAS access callback is required")
	}
	existingAuthority, ok := authority.(privatecasport.ExistingAccessAuthority)
	if !ok {
		return errors.New("existing private CAS access authority is required")
	}
	if err := existingAuthority.WithExistingPrivateCASAccess(ctx, root, func(binding privatecasport.RootBinding) error {
		if err := validatePrivateCASRootBinding(root, binding); err != nil {
			return err
		}
		return access(binding)
	}); err != nil {
		return errors.Join(errors.New("existing private CAS access authority is invalid"), err)
	}
	return nil
}

func validatePrivateCASRootBinding(requestedRoot string, binding privatecasport.RootBinding) error {
	if !filepath.IsAbs(requestedRoot) || !filepath.IsAbs(binding.RootPath) ||
		filepath.Clean(requestedRoot) != requestedRoot || filepath.Clean(binding.RootPath) != binding.RootPath ||
		binding.RelativePath == "" || binding.RelativePath == "." || filepath.IsAbs(binding.RelativePath) ||
		filepath.Clean(binding.RelativePath) != binding.RelativePath || binding.RelativePath == ".." ||
		strings.HasPrefix(binding.RelativePath, ".."+string(filepath.Separator)) ||
		!privateCASBindingPathEqual(filepath.Join(binding.RootPath, binding.RelativePath), requestedRoot) {
		return errors.New("private CAS root binding path is invalid")
	}
	identity := binding.RootIdentity
	switch identity.Kind {
	case privatecasport.DirectoryIdentityUnix:
		if identity.Device == 0 || identity.Inode == 0 || identity.VolumeSerial != 0 || identity.FileID != [16]byte{} {
			return errors.New("private CAS Unix root identity is non-canonical")
		}
	case privatecasport.DirectoryIdentityWindows:
		if identity.Device != 0 || identity.Inode != 0 || identity.VolumeSerial == 0 || identity.FileID == [16]byte{} {
			return errors.New("private CAS Windows root identity is non-canonical")
		}
	default:
		return errors.New("private CAS root identity kind is unsupported")
	}
	return nil
}

func privateCASWriteFingerprintField(writer hash.Hash, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write(value)
}

func clonePrivateCASShardPins(source map[string]privateCASShardIdentity) map[string]privateCASShardIdentity {
	cloned := make(map[string]privateCASShardIdentity, len(source))
	for name, identity := range source {
		cloned[name] = identity
	}
	return cloned
}

func privateCASReplacePreparedRecoveryEntry(entries []string, before, after string) {
	for index := range entries {
		if entries[index] == before {
			entries[index] = after
			return
		}
	}
}

func privateCASAuthorizedInventory(
	ctx context.Context,
	authority privateCASRootAuthority,
	rootPath string,
	pins map[string]privateCASShardIdentity,
	maxBytes int,
	originalResidues privateCASOriginalResiduesV1,
	originalCreates *PreparedSecurePrivateCASOriginalCreateResiduesV1,
) (map[string]privateCASAuthorizedRecord, error) {
	if originalResidues != nil || originalCreates != nil {
		observation, err := observePrivateCASOriginalResiduesV1(ctx, authority, rootPath, pins, maxBytes, originalResidues, originalCreates)
		if err != nil {
			return nil, err
		}
		return privateCASOriginalCommittedRecordsV1(observation), nil
	}
	files, err := securePrivateCASList(ctx, authority, clonePrivateCASShardPins(pins), maxBytes)
	if err != nil {
		return nil, err
	}
	records := make(map[string]privateCASAuthorizedRecord, len(files))
	for _, file := range files {
		if _, duplicate := records[file.Digest]; duplicate {
			return nil, errors.New("private CAS authority inventory repeats a record")
		}
		records[file.Digest] = privateCASAuthorizedRecord{bodySHA256: privateCASBodySHA256(file.Body)}
	}
	return records, nil
}

func attachPrivateCASRootGeneration(
	ctx context.Context,
	key privateCASRootGenerationKey,
	authority privateCASRootAuthority,
	pins map[string]privateCASShardIdentity,
	maxBytes int,
	preservations ...privateCASOriginalOpeningV1,
) (*privateCASRootGeneration, error) {
	if len(preservations) > 1 {
		return nil, errors.New("private CAS original preservation is ambiguous")
	}
	var originalResidues privateCASOriginalResiduesV1
	var originalCreates *PreparedSecurePrivateCASOriginalCreateResiduesV1
	if len(preservations) == 1 {
		originalResidues = preservations[0].residues
		originalCreates = preservations[0].creates
	}
	privateCASRootGenerations.Lock()
	defer privateCASRootGenerations.Unlock()
	if existing := privateCASRootGenerations.byRoot[key]; existing != nil {
		existing.mu.Lock()
		defer existing.mu.Unlock()
		if existing.revoked || existing.root != authority || existing.maxBytes != maxBytes {
			return nil, errors.New("private CAS root generation conflicts with a live authority")
		}
		if !privateCASOriginalResiduesEqualV1(existing.originalResidues, originalResidues) &&
			(len(preservations) != 1 || !privateCASOriginalResiduesCompatibleOpeningV1(existing.originalResidues, originalResidues)) {
			return nil, errors.New("private CAS original opening conflicts with the live preservation proof")
		}
		if !privateCASOriginalCreateOpeningCompatibleV1(existing.originalCreates, originalCreates) {
			return nil, errors.New("private CAS original creation proof conflicts with live authority")
		}
		if err := existing.validateLocked(ctx); err != nil {
			return nil, err
		}
		existing.refs++
		return existing, nil
	}
	records, err := privateCASAuthorizedInventory(ctx, authority, key.rootPath, pins, maxBytes, originalResidues, originalCreates)
	if err != nil {
		return nil, err
	}
	rootAnchor, err := privateCASOpenRootAnchor(authority)
	if err != nil {
		return nil, err
	}
	generation := &privateCASRootGeneration{
		key: key, root: authority, rootAnchor: rootAnchor,
		shardPins: clonePrivateCASShardPins(pins), shardAnchors: make(map[string]privateCASShardAnchor, len(pins)),
		records: records, additionReceipts: make(map[string]SecurePrivateCASAdditionReceiptV2), maxBytes: maxBytes, refs: 1,
		originalResidues: originalResidues, originalCreates: originalCreates,
	}
	for name, identity := range pins {
		anchor, anchorErr := privateCASOpenShardAnchor(authority, name, identity)
		if anchorErr != nil {
			generation.closeAnchorsLocked()
			return nil, anchorErr
		}
		generation.shardAnchors[name] = anchor
	}
	if err := generation.validateLocked(ctx); err != nil {
		generation.closeAnchorsLocked()
		return nil, err
	}
	privateCASRootGenerations.byRoot[key] = generation
	return generation, nil
}

func (generation *privateCASRootGeneration) validateLocked(ctx context.Context) error {
	if generation == nil || generation.revoked || generation.refs <= 0 {
		return errors.New("private CAS root generation is revoked")
	}
	if err := privateCASContextError(ctx); err != nil {
		return err
	}
	if err := privateCASValidateRootAnchor(generation.rootAnchor, generation.root); err != nil {
		return err
	}
	if len(generation.shardAnchors) != len(generation.shardPins) {
		return errors.New("private CAS root generation shard anchor inventory is incomplete")
	}
	for name, identity := range generation.shardPins {
		anchor, found := generation.shardAnchors[name]
		if !found {
			return errors.New("private CAS root generation lacks a shard anchor")
		}
		if err := privateCASValidateShardAnchor(generation.root, name, identity, anchor); err != nil {
			return err
		}
	}
	current, err := privateCASAuthorizedInventory(ctx, generation.root, generation.key.rootPath, generation.shardPins, generation.maxBytes, generation.originalResidues, generation.originalCreates)
	if err != nil {
		return err
	}
	if !privateCASAuthorizedRecordsEqual(current, generation.records) {
		return errors.Join(
			ErrSecurePrivateCASIntegrity,
			errors.New("private CAS filesystem inventory differs from host authority"),
		)
	}
	return nil
}

func privateCASAuthorizedRecordsEqual(left, right map[string]privateCASAuthorizedRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for digest, record := range left {
		if expected, found := right[digest]; !found || expected != record {
			return false
		}
	}
	return true
}

func (generation *privateCASRootGeneration) authorizeAdditionLocked(
	ctx context.Context,
	digest string,
	body []byte,
	commitPins map[string]privateCASShardIdentity,
) error {
	if generation == nil || generation.revoked {
		return errors.New("private CAS root generation is revoked before commit authorization")
	}
	if _, exists := generation.records[digest]; exists {
		return errors.New("private CAS host authority already contains the committed record")
	}
	shardName := digest[:2]
	newShard := false
	for name, identity := range commitPins {
		expected, found := generation.shardPins[name]
		if found {
			if expected != identity {
				return errors.New("private CAS commit changed a host-authorized shard identity")
			}
			continue
		}
		if name != shardName || newShard {
			return errors.New("private CAS commit introduced an unauthorized shard delta")
		}
		newShard = true
	}
	if len(commitPins) != len(generation.shardPins)+boolInt(newShard) {
		return errors.New("private CAS commit shard inventory is not an exact delta")
	}
	for name := range generation.shardPins {
		if _, found := commitPins[name]; !found {
			return errors.New("private CAS commit removed an authorized shard")
		}
	}
	var candidateAnchor privateCASShardAnchor
	if newShard {
		identity := commitPins[shardName]
		var err error
		candidateAnchor, err = privateCASOpenShardAnchor(generation.root, shardName, identity)
		if err != nil {
			return err
		}
	}
	accepted := false
	defer func() {
		if newShard && !accepted {
			_ = privateCASCloseShardAnchor(candidateAnchor)
		}
	}()
	current, err := privateCASAuthorizedInventory(ctx, generation.root, generation.key.rootPath, commitPins, generation.maxBytes, generation.originalResidues, generation.originalCreates)
	if err != nil {
		return err
	}
	expectedRecords := make(map[string]privateCASAuthorizedRecord, len(generation.records)+1)
	for name, record := range generation.records {
		expectedRecords[name] = record
	}
	expectedRecords[digest] = privateCASAuthorizedRecord{bodySHA256: privateCASBodySHA256(body)}
	if !privateCASAuthorizedRecordsEqual(current, expectedRecords) {
		return errors.New("private CAS commit did not produce the exact authorized record delta")
	}
	if newShard {
		generation.shardPins[shardName] = commitPins[shardName]
		generation.shardAnchors[shardName] = candidateAnchor
		accepted = true
	}
	generation.records = expectedRecords
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (generation *privateCASRootGeneration) closeAnchorsLocked() error {
	if generation == nil {
		return nil
	}
	var result error
	for name, anchor := range generation.shardAnchors {
		result = errors.Join(result, privateCASCloseShardAnchor(anchor))
		delete(generation.shardAnchors, name)
	}
	result = errors.Join(result, privateCASCloseRootAnchor(generation.rootAnchor))
	generation.rootAnchor = privateCASRootAnchor{}
	return result
}

func revokePrivateCASRootGeneration(binding privatecasport.RootBinding, rootPath string) error {
	key := privateCASRootGenerationKey{binding: binding, rootPath: rootPath}
	privateCASRootGenerations.Lock()
	defer privateCASRootGenerations.Unlock()
	generation := privateCASRootGenerations.byRoot[key]
	if generation == nil {
		return nil
	}
	generation.mu.Lock()
	defer generation.mu.Unlock()
	generation.revoked = true
	delete(privateCASRootGenerations.byRoot, key)
	return generation.closeAnchorsLocked()
}

func (store *SecurePrivateCAS) lockGeneration(ctx context.Context) (map[string]privateCASShardIdentity, error) {
	if store == nil || store.generation == nil {
		return nil, errors.New("private CAS root generation is unavailable")
	}
	store.generation.mu.Lock()
	if err := store.generation.validateLocked(ctx); err != nil {
		store.generation.mu.Unlock()
		return nil, err
	}
	return clonePrivateCASShardPins(store.generation.shardPins), nil
}

func (store *SecurePrivateCAS) unlockGeneration() {
	store.generation.mu.Unlock()
}

// Close releases this store's reference to the shared root-generation
// anchors. Other live sibling stores retain the generation. A finalizer is a
// fallback only; production owners should close stores deterministically.
func (store *SecurePrivateCAS) Close() error {
	if store == nil {
		return nil
	}
	store.closeOnce.Do(func() {
		runtime.SetFinalizer(store, nil)
		privateCASRootGenerations.Lock()
		defer privateCASRootGenerations.Unlock()
		generation := store.generation
		if generation == nil {
			return
		}
		generation.mu.Lock()
		defer generation.mu.Unlock()
		if generation.refs > 0 {
			generation.refs--
		}
		if generation.refs == 0 {
			generation.revoked = true
			if privateCASRootGenerations.byRoot[generation.key] == generation {
				delete(privateCASRootGenerations.byRoot, generation.key)
			}
			store.closeErr = generation.closeAnchorsLocked()
		}
	})
	return store.closeErr
}

func privateCASAccessGateIndex(binding privatecasport.RootBinding) uint8 {
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix-private-cas-root-gate-v1"))
	privateCASWriteFingerprintField(hasher, []byte(binding.RootIdentity.Kind))
	var number [8]byte
	for _, value := range []uint64{
		binding.RootIdentity.Device, binding.RootIdentity.Inode, binding.RootIdentity.VolumeSerial,
	} {
		binary.BigEndian.PutUint64(number[:], value)
		privateCASWriteFingerprintField(hasher, number[:])
	}
	privateCASWriteFingerprintField(hasher, binding.RootIdentity.FileID[:])
	digest := hasher.Sum(nil)
	return digest[0] & 63
}

func (store *SecurePrivateCAS) PutIfAbsent(ctx context.Context, digest string, body []byte) error {
	if store == nil || !validPrivateDigest(strings.TrimSpace(digest)) || len(body) == 0 || len(body) > store.maxBytes {
		return errors.New("private CAS write input is invalid")
	}
	if err := privateCASContextError(ctx); err != nil {
		return err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	stableBody := append([]byte(nil), body...)
	return withPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != store.binding {
			return errors.New("private CAS persistence binding changed")
		}
		if err := store.acquire(ctx); err != nil {
			return err
		}
		defer store.release()
		commitPins, err := store.lockGeneration(ctx)
		if err != nil {
			return err
		}
		defer store.unlockGeneration()
		_, shardCreated, err := securePrivateCASWriteWithAdditionReceipt(
			store.root, commitPins, strings.TrimSpace(digest), stableBody, store.maxBytes, store.beforeCommit, privateCASOriginalWriteValidationV1{ctx: ctx, generation: store.generation},
		)
		if err != nil {
			return err
		}
		_ = shardCreated
		return store.generation.authorizeAdditionLocked(ctx, strings.TrimSpace(digest), stableBody, commitPins)
	})
}

// PutIfAbsentWithAdditionReceipt issues a receipt only when this invocation's
// no-replace commit installed the exact staged object. os.ErrExist never
// yields a receipt, even when the existing bytes are semantically equal.
func (store *SecurePrivateCAS) PutIfAbsentWithAdditionReceipt(
	ctx context.Context,
	digest string,
	body []byte,
) (SecurePrivateCASAdditionReceiptV2, error) {
	digest = strings.TrimSpace(digest)
	if store == nil || !validPrivateDigest(digest) || len(body) == 0 || len(body) > store.maxBytes {
		return SecurePrivateCASAdditionReceiptV2{}, errors.New("private CAS addition commit input is invalid")
	}
	if err := privateCASContextError(ctx); err != nil {
		return SecurePrivateCASAdditionReceiptV2{}, err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return SecurePrivateCASAdditionReceiptV2{}, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	stableBody := append([]byte(nil), body...)
	var receipt SecurePrivateCASAdditionReceiptV2
	err := withPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != store.binding {
			return errors.New("private CAS persistence binding changed")
		}
		if err := store.acquire(ctx); err != nil {
			return err
		}
		defer store.release()
		commitPins, err := store.lockGeneration(ctx)
		if err != nil {
			return err
		}
		defer store.unlockGeneration()
		_, shardExisted := commitPins[digest[:2]]
		observation, shardCreated, err := securePrivateCASWriteWithAdditionReceipt(
			store.root, commitPins, digest, stableBody, store.maxBytes, store.beforeCommit, privateCASOriginalWriteValidationV1{ctx: ctx, generation: store.generation},
		)
		if err != nil {
			return err
		}
		if !equalPrivateCASBytes(observation.body, stableBody) {
			return errors.New("private CAS committed addition body does not match staged authority")
		}
		if err := store.generation.authorizeAdditionLocked(ctx, digest, stableBody, commitPins); err != nil {
			return err
		}
		receipt = newSecurePrivateCASAdditionReceipt(
			store.rootPath, digest, true, shardCreated, shardExisted, false, observation,
		)
		if err := validateSecurePrivateCASAdditionReceipt(receipt); err != nil {
			return err
		}
		store.generation.additionReceipts[receipt.ReceiptDigest] = receipt
		return nil
	})
	if err != nil {
		return SecurePrivateCASAdditionReceiptV2{}, err
	}
	return receipt, nil
}

func (store *SecurePrivateCAS) VerifyCommittedAddition(
	ctx context.Context,
	receipt SecurePrivateCASAdditionReceiptV2,
) error {
	if store == nil || validateSecurePrivateCASAdditionReceipt(receipt) != nil || !receipt.Finalized || receipt.RootPath != store.rootPath {
		return errors.New("private CAS addition receipt is invalid for this store")
	}
	if err := privateCASContextError(ctx); err != nil {
		return err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	return withPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != store.binding {
			return errors.New("private CAS persistence binding changed")
		}
		if err := store.acquire(ctx); err != nil {
			return err
		}
		defer store.release()
		pins, err := store.lockGeneration(ctx)
		if err != nil {
			return err
		}
		defer store.unlockGeneration()
		issued, found := store.generation.additionReceipts[receipt.ReceiptDigest]
		if !found || issued != receipt {
			return errors.New("private CAS addition receipt is not present in the host issuance registry")
		}
		observation, err := securePrivateCASObserveAddition(
			store.root, pins, receipt.RecordDigest, store.maxBytes, privateCASOriginalWriteValidationV1{ctx: ctx, generation: store.generation},
		)
		if err != nil {
			return err
		}
		current := newSecurePrivateCASAdditionReceipt(
			store.rootPath, receipt.RecordDigest, receipt.CreatedByThisCall,
			receipt.ShardCreatedByThisCall, receipt.ShardExistedBeforeCommit, true, observation,
		)
		if current != receipt {
			return errors.New("private CAS committed addition identity or metadata changed")
		}
		return nil
	})
}

// FinalizeCommittedAdditions converts current-call provenance tokens into a
// batch snapshot manifest. Directory metadata is frozen only after every
// commit, because later records in the same shard legitimately update that
// shard and root. Each V2 token records the shard state immediately before its
// own no-replace commit, independent of store-open or batch boundaries.
func (store *SecurePrivateCAS) FinalizeCommittedAdditions(
	ctx context.Context,
	provisional []SecurePrivateCASAdditionReceiptV2,
) ([]SecurePrivateCASAdditionReceiptV2, error) {
	if store == nil || len(provisional) == 0 {
		return nil, errors.New("private CAS addition batch is empty")
	}
	if err := privateCASContextError(ctx); err != nil {
		return nil, err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return nil, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	finalized := make([]SecurePrivateCASAdditionReceiptV2, 0, len(provisional))
	err := withPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != store.binding {
			return errors.New("private CAS persistence binding changed")
		}
		if err := store.acquire(ctx); err != nil {
			return err
		}
		defer store.release()
		pins, err := store.lockGeneration(ctx)
		if err != nil {
			return err
		}
		defer store.unlockGeneration()
		seenRecords := map[string]struct{}{}
		rootIdentity := ""
		var rootMetadata SecurePrivateCASSnapshotMetadataV1
		shardIdentity := map[string]string{}
		shardMetadata := map[string]SecurePrivateCASSnapshotMetadataV1{}
		for _, token := range provisional {
			if validateSecurePrivateCASAdditionReceipt(token) != nil || token.Finalized || token.RootPath != store.rootPath {
				return errors.New("private CAS addition provenance token is invalid")
			}
			issued, found := store.generation.additionReceipts[token.ReceiptDigest]
			if !found || issued != token {
				return errors.New("private CAS addition provenance token is not host-issued")
			}
			if _, duplicate := seenRecords[token.RecordDigest]; duplicate {
				return errors.New("private CAS addition batch repeats a record")
			}
			seenRecords[token.RecordDigest] = struct{}{}
			observation, err := securePrivateCASObserveAddition(
				store.root, pins, token.RecordDigest, store.maxBytes, privateCASOriginalWriteValidationV1{ctx: ctx, generation: store.generation},
			)
			if err != nil || !privateCASAdditionProvenanceMatches(token, observation) {
				return errors.Join(errors.New("private CAS addition provenance changed before finalization"), err)
			}
			candidate := newSecurePrivateCASAdditionReceipt(
				store.rootPath, token.RecordDigest, true, token.ShardCreatedByThisCall,
				token.ShardExistedBeforeCommit, true, observation,
			)
			if rootIdentity == "" {
				rootIdentity, rootMetadata = candidate.RootIdentityDigest, candidate.RootMetadata
			} else if rootIdentity != candidate.RootIdentityDigest || rootMetadata != candidate.RootMetadata {
				return errors.New("private CAS root changed during addition batch finalization")
			}
			if identity, found := shardIdentity[candidate.ShardName]; found {
				if identity != candidate.ShardIdentityDigest || shardMetadata[candidate.ShardName] != candidate.ShardMetadata {
					return errors.New("private CAS shard changed during addition batch finalization")
				}
			} else {
				shardIdentity[candidate.ShardName] = candidate.ShardIdentityDigest
				shardMetadata[candidate.ShardName] = candidate.ShardMetadata
			}
			finalized = append(finalized, candidate)
		}
		for index, token := range provisional {
			delete(store.generation.additionReceipts, token.ReceiptDigest)
			store.generation.additionReceipts[finalized[index].ReceiptDigest] = finalized[index]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return finalized, nil
}

func privateCASAdditionProvenanceMatches(
	token SecurePrivateCASAdditionReceiptV2,
	observation privateCASAdditionObservation,
) bool {
	return token.BodySHA256 == privateCASBodySHA256(observation.body) &&
		token.RootIdentityDigest == observation.rootIdentityDigest &&
		token.ShardIdentityDigest == observation.shardIdentityDigest &&
		token.RecordIdentityDigest == observation.recordIdentityDigest &&
		token.RecordMetadata == observation.recordMetadata
}

func newSecurePrivateCASAdditionReceipt(
	rootPath string,
	digest string,
	createdByThisCall bool,
	shardCreatedByThisCall bool,
	shardExisted bool,
	finalized bool,
	observation privateCASAdditionObservation,
) SecurePrivateCASAdditionReceiptV2 {
	receipt := SecurePrivateCASAdditionReceiptV2{
		SchemaVersion: securePrivateCASAdditionReceiptVersion,
		RootPath:      rootPath, RecordDigest: digest, BodySHA256: privateCASBodySHA256(observation.body),
		ShardName: digest[:2], CreatedByThisCall: createdByThisCall,
		ShardCreatedByThisCall: shardCreatedByThisCall, ShardExistedBeforeCommit: shardExisted, Finalized: finalized,
		RootIdentityDigest: observation.rootIdentityDigest, ShardIdentityDigest: observation.shardIdentityDigest,
		RecordIdentityDigest: observation.recordIdentityDigest, RootMetadata: observation.rootMetadata,
		ShardMetadata: observation.shardMetadata, RecordMetadata: observation.recordMetadata,
	}
	receipt.ReceiptDigest = securePrivateCASAdditionReceiptDigest(receipt)
	return receipt
}

func validateSecurePrivateCASAdditionReceipt(receipt SecurePrivateCASAdditionReceiptV2) error {
	if strings.TrimSpace(receipt.RootPath) == "" || !filepath.IsAbs(receipt.RootPath) || filepath.Clean(receipt.RootPath) != receipt.RootPath ||
		!validPrivateDigest(receipt.RecordDigest) || receipt.ShardName != receipt.RecordDigest[:2] ||
		receipt.SchemaVersion != securePrivateCASAdditionReceiptVersion || !receipt.CreatedByThisCall ||
		receipt.ShardCreatedByThisCall == receipt.ShardExistedBeforeCommit ||
		!validPrivateDigest(receipt.BodySHA256) || !validPrivateDigest(receipt.RootIdentityDigest) ||
		!validPrivateDigest(receipt.ShardIdentityDigest) || !validPrivateDigest(receipt.RecordIdentityDigest) ||
		receipt.RecordMetadata.Size <= 0 || receipt.ReceiptDigest != securePrivateCASAdditionReceiptDigest(receipt) {
		return errors.New("private CAS addition receipt fields are invalid")
	}
	return nil
}

func securePrivateCASAdditionReceiptDigest(receipt SecurePrivateCASAdditionReceiptV2) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return privateCASBodySHA256(body)
}

func privateCASBodySHA256(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func equalPrivateCASBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func (store *SecurePrivateCAS) Read(ctx context.Context, digest string) ([]byte, error) {
	if store == nil || !validPrivateDigest(strings.TrimSpace(digest)) {
		return nil, errors.New("private CAS content address is invalid")
	}
	if err := privateCASContextError(ctx); err != nil {
		return nil, err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return nil, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	var body []byte
	err := withPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != store.binding {
			return errors.New("private CAS persistence binding changed")
		}
		if err := store.acquire(ctx); err != nil {
			return err
		}
		defer store.release()
		pins, err := store.lockGeneration(ctx)
		if err != nil {
			return err
		}
		defer store.unlockGeneration()
		var readErr error
		if store.generation.originalResidues != nil || store.generation.originalCreates != nil {
			body, readErr = store.generation.readOriginalLockedV1(ctx, strings.TrimSpace(digest))
		} else {
			body, readErr = securePrivateCASRead(store.root, pins, strings.TrimSpace(digest), store.maxBytes)
		}
		return readErr
	})
	return body, err
}

func (store *SecurePrivateCAS) List(ctx context.Context) ([]SecurePrivateCASFile, error) {
	if store == nil {
		return nil, errors.New("private CAS is unavailable")
	}
	if err := privateCASContextError(ctx); err != nil {
		return nil, err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return nil, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	var files []SecurePrivateCASFile
	err := withPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != store.binding {
			return errors.New("private CAS persistence binding changed")
		}
		if err := store.acquire(ctx); err != nil {
			return err
		}
		defer store.release()
		pins, err := store.lockGeneration(ctx)
		if err != nil {
			return err
		}
		defer store.unlockGeneration()
		var listErr error
		if store.generation.originalResidues != nil || store.generation.originalCreates != nil {
			listErr = store.generation.visitOriginalLockedV1(ctx, func(file SecurePrivateCASFile) error {
				files = append(files, file)
				return nil
			})
			if listErr != nil {
				files = nil
			}
		} else {
			files, listErr = securePrivateCASList(ctx, store.root, pins, store.maxBytes)
		}
		return listErr
	})
	return files, err
}

// Visit streams a fully validated inventory while retaining the persistence
// lease and process gate. Callback output is provisional until Visit returns
// nil and callbacks must not re-enter this CAS.
func (store *SecurePrivateCAS) Visit(ctx context.Context, visit func(SecurePrivateCASFile) error) error {
	if store == nil || visit == nil {
		return errors.New("private CAS visitor is unavailable")
	}
	if err := privateCASContextError(ctx); err != nil {
		return err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	return withPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != store.binding {
			return errors.New("private CAS persistence binding changed")
		}
		if err := store.acquire(ctx); err != nil {
			return err
		}
		defer store.release()
		pins, err := store.lockGeneration(ctx)
		if err != nil {
			return err
		}
		defer store.unlockGeneration()
		if store.generation.originalResidues != nil || store.generation.originalCreates != nil {
			return store.generation.visitOriginalLockedV1(ctx, visit)
		}
		return securePrivateCASVisit(ctx, store.root, pins, store.maxBytes, visit)
	})
}

func (store *SecurePrivateCAS) acquire(ctx context.Context) error {
	return acquirePrivateCASGate(ctx, store.gate)
}

func acquirePrivateCASGate(ctx context.Context, gate chan struct{}) error {
	if gate == nil {
		return errors.New("private CAS process gate is unavailable")
	}
	if ctx == nil {
		<-gate
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gate:
		return nil
	}
}

func (store *SecurePrivateCAS) release() {
	releasePrivateCASGate(store.gate)
}

func releasePrivateCASGate(gate chan struct{}) {
	gate <- struct{}{}
}

func privateCASContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func privateCASHashExact(
	ctx context.Context,
	reader io.Reader,
	expectedSize int64,
) ([32]byte, error) {
	if reader == nil || expectedSize < 0 {
		return [32]byte{}, errors.New("private CAS exact hash input is invalid")
	}
	hasher := sha256.New()
	var buffer [64 << 10]byte
	defer clear(buffer[:])
	remaining := expectedSize
	sawEOF := false
	for remaining > 0 {
		if err := privateCASContextError(ctx); err != nil {
			return [32]byte{}, err
		}
		limit := int64(len(buffer))
		if remaining < limit {
			limit = remaining
		}
		count, err := reader.Read(buffer[:int(limit)])
		if count < 0 || int64(count) > limit {
			return [32]byte{}, errors.New("private CAS exact hash reader returned an invalid count")
		}
		if count > 0 {
			_, _ = hasher.Write(buffer[:count])
			remaining -= int64(count)
		}
		if errors.Is(err, io.EOF) {
			if remaining != 0 {
				return [32]byte{}, errors.New("private CAS exact hash ended before the expected size")
			}
			sawEOF = true
			break
		}
		if err != nil {
			return [32]byte{}, errors.New("private CAS exact hash read failed")
		}
		if count == 0 {
			return [32]byte{}, errors.New("private CAS exact hash reader made no progress")
		}
	}
	if !sawEOF {
		var extra [1]byte
		count, err := reader.Read(extra[:])
		if count != 0 || !errors.Is(err, io.EOF) {
			return [32]byte{}, errors.New("private CAS exact hash exceeded the expected size")
		}
	}
	var digest [32]byte
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

func privateCASReadBodyExact(
	ctx context.Context,
	reader io.Reader,
	expectedSize int64,
) ([]byte, [32]byte, error) {
	maxInt := int64(^uint(0) >> 1)
	if reader == nil || expectedSize < 0 || expectedSize > maxInt {
		return nil, [32]byte{}, errors.New("private CAS exact body input is invalid")
	}
	body := make([]byte, int(expectedSize))
	success := false
	defer func() {
		if !success {
			clear(body)
		}
	}()
	hasher := sha256.New()
	offset := 0
	sawEOF := false
	for offset < len(body) {
		if err := privateCASContextError(ctx); err != nil {
			return nil, [32]byte{}, err
		}
		end := offset + (64 << 10)
		if end > len(body) {
			end = len(body)
		}
		count, err := reader.Read(body[offset:end])
		if count < 0 || offset+count > end {
			return nil, [32]byte{}, errors.New("private CAS exact body reader returned an invalid count")
		}
		if count > 0 {
			_, _ = hasher.Write(body[offset : offset+count])
			offset += count
		}
		if errors.Is(err, io.EOF) {
			if offset != len(body) {
				return nil, [32]byte{}, errors.New("private CAS exact body ended before the expected size")
			}
			sawEOF = true
			break
		}
		if err != nil {
			return nil, [32]byte{}, errors.New("private CAS exact body read failed")
		}
		if count == 0 {
			return nil, [32]byte{}, errors.New("private CAS exact body reader made no progress")
		}
	}
	if !sawEOF {
		var extra [1]byte
		count, err := reader.Read(extra[:])
		if count != 0 || !errors.Is(err, io.EOF) {
			return nil, [32]byte{}, errors.New("private CAS exact body exceeded the expected size")
		}
	}
	var digest [32]byte
	copy(digest[:], hasher.Sum(nil))
	success = true
	return body, digest, nil
}

func lexicalPrivateCASRootPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	absolute, err := filepath.Abs(value)
	if err != nil || value == "" {
		return "", errors.New("private CAS requested root is invalid")
	}
	return filepath.Clean(absolute), nil
}
