package electronlegacytask

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	BackgroundTaskFileV1          = "background-tasks.json"
	MaxBackgroundTaskBytesV1      = int64(16 << 20)
	journalDirectoryV1            = ".analytix-electron-task-retirement-v1"
	journalPlanFileV1             = "plan.json"
	journalProtectionFileV1       = "protected.json"
	journalRemovalIntentFileV1    = "removal-intent.json"
	journalCommitFileV1           = "committed.json"
	journalQuarantineFileV1       = "payload"
	journalSchemaVersionV2        = 2
	journalPurposeV1              = "analytix.electron-background-task-retirement/v2"
	journalProtectionPurposeV1    = "analytix.electron-background-task-retirement-protection/v2"
	journalRemovalIntentPurposeV1 = "analytix.electron-background-task-retirement-removal-intent/v2"
	journalCommitPurposeV1        = "analytix.electron-background-task-retirement-commit/v2"
	maxJournalBytesV1             = int64(64 << 10)
	maxJournalEntriesV1           = 16
	journalAuthorityAlgorithmV2   = "Ed25519"
	journalPlanDigestDomainV2     = "analytix.electron-background-task-retirement-plan/digest/v2\x00"
	journalProtectionDigestV2     = "analytix.electron-background-task-retirement-protection/digest/v2\x00"
	journalRemovalIntentDigestV2  = "analytix.electron-background-task-retirement-removal-intent/digest/v2\x00"
	journalCommitDigestDomainV2   = "analytix.electron-background-task-retirement-commit/digest/v2\x00"
)

// InventoryV1 is the complete owner-specific fixed point. It contains only
// host-issued filesystem state; the opaque Electron payload is never decoded.
type InventoryV1 struct {
	AuthorityDigest string
	RootPresent     bool
	Root            persistencefs.SeparateOwnerDirectoryState
	TargetPresent   bool
	Target          persistencefs.SeparateOwnerFileState
	Digest          string
}

type fileStateV1 struct {
	Identity         string `json:"identity"`
	SecurityDigest   string `json:"securityDigest"`
	Size             int64  `json:"size"`
	ModifiedUnixNano int64  `json:"modifiedUnixNano"`
	SHA256           string `json:"sha256"`
}

type directoryStateV1 struct {
	Identity       string `json:"identity"`
	SecurityDigest string `json:"securityDigest"`
	Protected      bool   `json:"protected"`
}

type journalPlanV1 struct {
	SchemaVersion      int              `json:"schemaVersion"`
	Purpose            string           `json:"purpose"`
	RootBindingDigest  string           `json:"rootBindingDigest"`
	AuthorityDigest    string           `json:"authorityDigest"`
	Root               directoryStateV1 `json:"root"`
	JournalDirectory   directoryStateV1 `json:"journalDirectory"`
	Target             fileStateV1      `json:"target"`
	ProtectedTarget    fileStateV1      `json:"protectedTarget"`
	AuthorityAlgorithm string           `json:"authorityAlgorithm"`
	AuthorityKeyID     string           `json:"authorityKeyId"`
	PlanDigest         string           `json:"planDigest"`
	AuthoritySignature string           `json:"authoritySignature"`
}

type journalProtectionV1 struct {
	SchemaVersion      int         `json:"schemaVersion"`
	Purpose            string      `json:"purpose"`
	RootBindingDigest  string      `json:"rootBindingDigest"`
	PlanDigest         string      `json:"planDigest"`
	ProtectedTarget    fileStateV1 `json:"protectedTarget"`
	AuthorityAlgorithm string      `json:"authorityAlgorithm"`
	AuthorityKeyID     string      `json:"authorityKeyId"`
	ProofDigest        string      `json:"proofDigest"`
	AuthoritySignature string      `json:"authoritySignature"`
}

type journalCommitV1 struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Purpose             string `json:"purpose"`
	RootBindingDigest   string `json:"rootBindingDigest"`
	PlanDigest          string `json:"planDigest"`
	ProtectionDigest    string `json:"protectionDigest"`
	RemovalIntentDigest string `json:"removalIntentDigest"`
	TargetState         string `json:"targetState"`
	AuthorityAlgorithm  string `json:"authorityAlgorithm"`
	AuthorityKeyID      string `json:"authorityKeyId"`
	CommitDigest        string `json:"commitDigest"`
	AuthoritySignature  string `json:"authoritySignature"`
}

type journalRemovalIntentV1 struct {
	SchemaVersion      int         `json:"schemaVersion"`
	Purpose            string      `json:"purpose"`
	RootBindingDigest  string      `json:"rootBindingDigest"`
	PlanDigest         string      `json:"planDigest"`
	ProtectionDigest   string      `json:"protectionDigest"`
	Target             fileStateV1 `json:"target"`
	AuthorityAlgorithm string      `json:"authorityAlgorithm"`
	AuthorityKeyID     string      `json:"authorityKeyId"`
	IntentDigest       string      `json:"intentDigest"`
	AuthoritySignature string      `json:"authoritySignature"`
}

type Store struct {
	root             string
	lease            *persistencefs.CompositeLease
	authority        *persistencefs.SeparateOwnerRootAuthority
	journalAuthority *persistencefs.ElectronTaskRetirementJournalAuthorityV2
	hooks            storeHooks
}

type storeHooks struct {
	afterJournalCreated     func() error
	afterTargetProtected    func() error
	afterProtectionPrepared func() error
	afterJournalPrepared    func() error
	beforeDetach            func() error
	afterDetach             func() error
	beforeQuarantineRemove  func() error
	afterRemovalPrepared    func() error
	afterQuarantineRemoved  func() error
	beforeCommit            func() error
	afterCommitPrepared     func() error
	atomicWriteFault        func(string) error
	removeFileFault         persistencefs.SeparateOwnerWriteFault
}

type PreparedPlanV1 struct {
	store       *Store
	observation ObservationV1
	inventory   InventoryV1
	present     bool
}

// ObservationV1 is a read-only, deterministic view of the exact target and
// migration-owned journal. Call ValidateObservation before acting on it.
type ObservationV1 struct {
	digest           string
	recoveryRequired bool
	targetPresent    bool
	journalPresent   bool
	committed        bool
}

func (observation ObservationV1) Digest() string         { return observation.digest }
func (observation ObservationV1) RecoveryRequired() bool { return observation.recoveryRequired }
func (observation ObservationV1) TargetPresent() bool    { return observation.targetPresent }
func (observation ObservationV1) JournalPresent() bool   { return observation.journalPresent }
func (observation ObservationV1) Committed() bool        { return observation.committed }
func (observation ObservationV1) Retired() bool {
	return !observation.targetPresent && !observation.recoveryRequired
}

// NewStore binds the owner adapter to the exact separate-owner authority held
// by one live CompositeLease. A path string alone can never authorize access.
func NewStore(lease *persistencefs.CompositeLease, root string) (*Store, error) {
	if lease == nil {
		return nil, errors.New("legacy Electron owner lease is unavailable")
	}
	authority, ok := lease.FrozenSeparateOwnerAuthority(root)
	if !ok || authority == nil || authority.Root() == "" {
		return nil, errors.New("legacy Electron owner authority is unavailable")
	}
	journalAuthority, err := persistencefs.IssueElectronTaskRetirementJournalAuthorityV2(lease, authority.Root())
	if err != nil {
		return nil, err
	}
	return &Store{
		root: authority.Root(), lease: lease, authority: authority, journalAuthority: journalAuthority,
	}, nil
}

func (store *Store) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}

func (store *Store) withAccess(
	ctx context.Context,
	fn func(*persistencefs.SeparateOwnerRootAccess) error,
) error {
	if store == nil || store.lease == nil || store.authority == nil || store.root == "" || fn == nil {
		return errors.New("legacy Electron owner is unavailable")
	}
	return store.lease.WithSeparateOwnerRootAccess(ctx, store.authority, fn)
}

func (store *Store) Recover(ctx context.Context) error {
	observation, err := store.Observe(ctx)
	if err != nil {
		return err
	}
	if !observation.RecoveryRequired() {
		return nil
	}
	return store.withAccess(ctx, func(access *persistencefs.SeparateOwnerRootAccess) error {
		inventory, journal, err := store.capture(access, ctx)
		if err != nil || !journal.present {
			return errors.New("legacy Electron owner recovery journal disappeared")
		}
		if !journal.planValid {
			return store.recoverUnauthenticated(ctx, access, inventory, journal)
		}
		return store.completeAuthenticated(ctx, access, inventory, journal)
	})
}

func (store *Store) Observe(ctx context.Context) (ObservationV1, error) {
	var result ObservationV1
	err := store.withAccess(ctx, func(access *persistencefs.SeparateOwnerRootAccess) error {
		inventory, journal, err := store.capture(access, ctx)
		if err != nil {
			return err
		}
		result, err = newObservation(inventory, journal)
		return err
	})
	return result, err
}

func (store *Store) ValidateObservation(ctx context.Context, expected ObservationV1) error {
	if !isSHA256(expected.digest) {
		return errors.New("legacy Electron owner observation is invalid")
	}
	current, err := store.Observe(ctx)
	if err != nil || current != expected {
		return errors.New("legacy Electron owner observation changed")
	}
	return nil
}

func (store *Store) Prepare(ctx context.Context) (*PreparedPlanV1, error) {
	observation, err := store.Observe(ctx)
	if err != nil {
		return nil, err
	}
	if observation.RecoveryRequired() {
		return nil, errors.New("legacy Electron owner transaction requires recovery")
	}
	var inventory InventoryV1
	err = store.withAccess(ctx, func(access *persistencefs.SeparateOwnerRootAccess) error {
		var journal journalViewV1
		var captureErr error
		inventory, journal, captureErr = store.capture(access, ctx)
		if captureErr != nil {
			return captureErr
		}
		current, currentErr := newObservation(inventory, journal)
		if currentErr != nil || current != observation {
			return errors.New("legacy Electron owner changed during prepare")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &PreparedPlanV1{
		store: store, observation: observation, inventory: inventory,
		present: inventory.TargetPresent && !observation.Committed(),
	}, nil
}

func (plan *PreparedPlanV1) Present() bool { return plan != nil && plan.present }

func (plan *PreparedPlanV1) Digest() string {
	if plan == nil {
		return ""
	}
	return plan.inventory.Digest
}

func (plan *PreparedPlanV1) Validate(ctx context.Context) error {
	if plan == nil || plan.store == nil || !validInventory(plan.inventory) || !isSHA256(plan.observation.digest) {
		return errors.New("legacy Electron owner plan is invalid")
	}
	if err := plan.store.ValidateObservation(ctx, plan.observation); err != nil {
		return err
	}
	return nil
}

func (plan *PreparedPlanV1) Apply(ctx context.Context) error {
	if err := plan.Validate(ctx); err != nil {
		return err
	}
	if !plan.present {
		return nil
	}
	signing, err := plan.store.journalAuthority.PrepareNewTransaction(ctx)
	if err != nil {
		return errors.Join(errors.New("legacy Electron owner signing authority preparation failed"), err)
	}
	return plan.store.withAccess(ctx, func(access *persistencefs.SeparateOwnerRootAccess) error {
		inventory, journal, err := plan.store.capture(access, ctx)
		if err != nil || journal.present || inventory != plan.inventory {
			return errors.New("legacy Electron owner changed before journal creation")
		}
		root, err := access.Directory()
		if err != nil {
			return err
		}
		projected, err := root.ProjectProtectedFileStateExact(
			ctx, BackgroundTaskFileV1, plan.inventory.Target,
		)
		if err != nil {
			return errors.Join(errors.New("legacy Electron owner protected state projection failed"), err)
		}
		journalDirectory, err := root.CreatePrivateDirectory(ctx, journalDirectoryV1)
		if err != nil {
			return errors.New("legacy Electron owner journal creation failed")
		}
		defer journalDirectory.Close()
		if err := root.Sync(); err != nil {
			return err
		}
		if err := callHook(plan.store.hooks.afterJournalCreated); err != nil {
			return err
		}
		record, err := journalPlanFromInventory(
			ctx, signing, plan.inventory, projected, journalDirectory.State(),
		)
		if err != nil {
			return err
		}
		body, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if err := journalDirectory.WriteExclusiveAtomic(
			ctx, journalPlanFileV1, body, maxJournalBytesV1, plan.store.atomicFault("plan"),
		); err != nil {
			return err
		}
		if err := callHook(plan.store.hooks.afterJournalPrepared); err != nil {
			return err
		}
		inventory, journal, err = plan.store.captureOpened(ctx, access, root)
		if err != nil || !journal.planValid {
			return errors.New("legacy Electron owner durable plan readback failed")
		}
		return plan.store.completeAuthenticated(ctx, access, inventory, journal)
	})
}

type journalViewV1 struct {
	present            bool
	directory          persistencefs.SeparateOwnerDirectoryState
	digest             string
	planState          persistencefs.SeparateOwnerFileState
	plan               *journalPlanV1
	planValid          bool
	protectionState    persistencefs.SeparateOwnerFileState
	protection         *journalProtectionV1
	protectionValid    bool
	commitState        persistencefs.SeparateOwnerFileState
	commit             *journalCommitV1
	commitValid        bool
	removalIntentState persistencefs.SeparateOwnerFileState
	removalIntent      *journalRemovalIntentV1
	removalIntentValid bool
	payload            persistencefs.SeparateOwnerFileState
	payloadPresent     bool
	temps              map[string]persistencefs.SeparateOwnerFileState
}

func (store *Store) capture(
	access *persistencefs.SeparateOwnerRootAccess,
	ctx context.Context,
) (InventoryV1, journalViewV1, error) {
	if access == nil {
		return InventoryV1{}, journalViewV1{}, errors.New("legacy Electron owner access is unavailable")
	}
	if !access.Present() {
		inventory, err := newInventory(InventoryV1{AuthorityDigest: store.authority.Digest()})
		return inventory, missingJournalView(), err
	}
	root, err := access.Directory()
	if err != nil {
		return InventoryV1{}, journalViewV1{}, err
	}
	return store.captureOpened(ctx, access, root)
}

func (store *Store) captureOpened(
	ctx context.Context,
	access *persistencefs.SeparateOwnerRootAccess,
	root *persistencefs.SeparateOwnerDirectory,
) (InventoryV1, journalViewV1, error) {
	if access == nil || root == nil || !access.Present() {
		return InventoryV1{}, journalViewV1{}, errors.New("legacy Electron owner root is unavailable")
	}
	inventory, err := store.captureInventory(ctx, root)
	if err != nil {
		return InventoryV1{}, journalViewV1{}, err
	}
	journal, present, err := root.OpenDirectory(ctx, journalDirectoryV1, true)
	if err != nil {
		return InventoryV1{}, journalViewV1{}, err
	}
	view := missingJournalView()
	if present {
		defer journal.Close()
		var signing *persistencefs.ElectronTaskRetirementSigningSessionV2
		signing, err = store.journalAuthority.OpenExisting(ctx)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return InventoryV1{}, journalViewV1{}, errors.Join(
				errors.New("legacy Electron owner journal authority is invalid"), err,
			)
		}
		view, err = captureJournalView(ctx, journal, signing)
		if err != nil {
			return InventoryV1{}, journalViewV1{}, err
		}
	}
	after, err := store.captureInventory(ctx, root)
	if err != nil || after != inventory {
		return InventoryV1{}, journalViewV1{}, errors.New("legacy Electron owner inventory changed during journal capture")
	}
	finalJournal, finalPresent, err := root.OpenDirectory(ctx, journalDirectoryV1, true)
	if err != nil || finalPresent != present {
		if finalJournal != nil {
			_ = finalJournal.Close()
		}
		return InventoryV1{}, journalViewV1{}, errors.New("legacy Electron owner journal presence changed during capture")
	}
	if finalPresent {
		finalState := finalJournal.State()
		closeErr := finalJournal.Close()
		if closeErr != nil || finalState != view.directory {
			return InventoryV1{}, journalViewV1{}, errors.New("legacy Electron owner journal identity changed during capture")
		}
	}
	return after, view, nil
}

func (store *Store) captureInventory(
	ctx context.Context,
	root *persistencefs.SeparateOwnerDirectory,
) (InventoryV1, error) {
	target, _, targetPresent, err := root.CaptureFile(ctx, BackgroundTaskFileV1, MaxBackgroundTaskBytesV1, true, false)
	if err != nil {
		return InventoryV1{}, err
	}
	inventory, err := newInventory(InventoryV1{
		AuthorityDigest: store.authority.Digest(), RootPresent: true, Root: root.State(),
		TargetPresent: targetPresent, Target: target,
	})
	return inventory, err
}

func captureJournalView(
	ctx context.Context,
	directory *persistencefs.SeparateOwnerDirectory,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
) (journalViewV1, error) {
	before, err := captureJournalViewOnce(ctx, directory, signing)
	if err != nil {
		return journalViewV1{}, err
	}
	after, err := captureJournalViewOnce(ctx, directory, signing)
	if err != nil || after.digest != before.digest || after.directory != before.directory {
		return journalViewV1{}, errors.New("legacy Electron owner journal changed during capture")
	}
	return after, nil
}

func captureJournalViewOnce(
	ctx context.Context,
	directory *persistencefs.SeparateOwnerDirectory,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
) (journalViewV1, error) {
	view := journalViewV1{present: true, directory: directory.State(), temps: map[string]persistencefs.SeparateOwnerFileState{}}
	names, err := directory.Entries(ctx, maxJournalEntriesV1)
	if err != nil {
		return journalViewV1{}, err
	}
	sort.Strings(names)
	for _, name := range names {
		switch {
		case name == journalPlanFileV1:
			state, body, present, captureErr := directory.CaptureFile(ctx, name, maxJournalBytesV1, true, true)
			if captureErr != nil || !present {
				return journalViewV1{}, errors.New("legacy Electron owner journal plan changed")
			}
			view.planState = state
			plan, decodeErr := decodeStrict[journalPlanV1](body, maxJournalBytesV1)
			clear(body)
			if decodeErr == nil && signing != nil &&
				validateJournalPlan(ctx, plan, directory.State(), signing) == nil {
				view.plan = &plan
				view.planValid = true
			}
		case name == journalProtectionFileV1:
			state, body, present, captureErr := directory.CaptureFile(ctx, name, maxJournalBytesV1, true, true)
			if captureErr != nil || !present {
				return journalViewV1{}, errors.New("legacy Electron owner journal protection proof changed")
			}
			view.protectionState = state
			protection, decodeErr := decodeStrict[journalProtectionV1](body, maxJournalBytesV1)
			clear(body)
			if decodeErr == nil {
				view.protection = &protection
			}
		case name == journalCommitFileV1:
			state, body, present, captureErr := directory.CaptureFile(ctx, name, maxJournalBytesV1, true, true)
			if captureErr != nil || !present {
				return journalViewV1{}, errors.New("legacy Electron owner journal commit changed")
			}
			view.commitState = state
			commit, decodeErr := decodeStrict[journalCommitV1](body, maxJournalBytesV1)
			clear(body)
			if decodeErr == nil {
				view.commit = &commit
			}
		case name == journalRemovalIntentFileV1:
			state, body, present, captureErr := directory.CaptureFile(ctx, name, maxJournalBytesV1, true, true)
			if captureErr != nil || !present {
				return journalViewV1{}, errors.New("legacy Electron owner journal removal intent changed")
			}
			view.removalIntentState = state
			intent, decodeErr := decodeStrict[journalRemovalIntentV1](body, maxJournalBytesV1)
			clear(body)
			if decodeErr == nil {
				view.removalIntent = &intent
			}
		case name == journalQuarantineFileV1:
			state, _, present, captureErr := directory.CaptureFile(ctx, name, MaxBackgroundTaskBytesV1, true, true)
			if captureErr != nil || !present {
				return journalViewV1{}, errors.New("legacy Electron owner quarantine changed")
			}
			view.payload = state
			view.payloadPresent = true
		case persistencefs.IsSeparateOwnerAtomicTempName(journalPlanFileV1, name),
			persistencefs.IsSeparateOwnerAtomicTempName(journalProtectionFileV1, name),
			persistencefs.IsSeparateOwnerAtomicTempName(journalRemovalIntentFileV1, name),
			persistencefs.IsSeparateOwnerAtomicTempName(journalCommitFileV1, name):
			state, _, present, captureErr := directory.CaptureFile(ctx, name, maxJournalBytesV1, true, true)
			if captureErr != nil || !present {
				return journalViewV1{}, errors.New("legacy Electron owner journal temp changed")
			}
			view.temps[name] = state
		default:
			return journalViewV1{}, errors.New("legacy Electron owner journal contains an unknown object")
		}
	}
	if view.planValid && view.protection != nil &&
		validateJournalProtection(ctx, *view.protection, *view.plan, signing) == nil &&
		view.protection.ProtectedTarget == view.plan.ProtectedTarget {
		view.protectionValid = true
	}
	if view.planValid && view.protectionValid && view.removalIntent != nil &&
		validateJournalRemovalIntent(ctx, *view.removalIntent, *view.plan, *view.protection, signing) == nil &&
		platformFileState(view.removalIntent.Target) == platformFileState(view.plan.ProtectedTarget) {
		view.removalIntentValid = true
	}
	if view.planValid && view.protectionValid && view.removalIntentValid && view.commit != nil &&
		validateJournalCommit(ctx, *view.commit, *view.plan, *view.protection, *view.removalIntent, signing) == nil {
		view.commitValid = true
	}
	view.digest = digestJournalView(view)
	return view, nil
}

func missingJournalView() journalViewV1 {
	view := journalViewV1{temps: map[string]persistencefs.SeparateOwnerFileState{}}
	view.digest = digestJournalView(view)
	return view
}

func newObservation(inventory InventoryV1, journal journalViewV1) (ObservationV1, error) {
	if !validInventory(inventory) || !isSHA256(journal.digest) {
		return ObservationV1{}, errors.New("legacy Electron owner observation inventory is invalid")
	}
	if journal.present && !journal.planValid && !inventory.TargetPresent && !journal.payloadPresent {
		return ObservationV1{}, errors.New("legacy Electron owner journal cannot prove absent target custody")
	}
	if journal.planValid && (journal.plan == nil || !inventory.RootPresent ||
		journal.plan.AuthorityDigest != inventory.AuthorityDigest ||
		journal.plan.Root != directoryStateFromPlatform(inventory.Root)) {
		return ObservationV1{}, errors.New("legacy Electron owner journal does not belong to the current authority")
	}
	committed := journal.present && journal.planValid && journal.protectionValid &&
		journal.removalIntentValid && journal.commitValid && len(journal.temps) == 0
	if committed && (inventory.TargetPresent || journal.payloadPresent) {
		return ObservationV1{}, errors.New("committed legacy Electron owner target reappeared")
	}
	recovery := journal.present && !committed
	body, _ := json.Marshal(struct {
		SchemaVersion   int    `json:"schemaVersion"`
		InventoryDigest string `json:"inventoryDigest"`
		JournalDigest   string `json:"journalDigest"`
		Recovery        bool   `json:"recovery"`
		Committed       bool   `json:"committed"`
	}{1, inventory.Digest, journal.digest, recovery, committed})
	return ObservationV1{
		digest: sha256Hex(body), recoveryRequired: recovery,
		targetPresent: inventory.TargetPresent, journalPresent: journal.present, committed: committed,
	}, nil
}

func (store *Store) recoverUnauthenticated(
	ctx context.Context,
	access *persistencefs.SeparateOwnerRootAccess,
	inventory InventoryV1,
	journal journalViewV1,
) error {
	root, err := access.Directory()
	if err != nil {
		return err
	}
	directory, present, err := root.OpenDirectory(ctx, journalDirectoryV1, true)
	if err != nil || !present || directory.State() != journal.directory {
		return errors.New("legacy Electron unauthenticated journal changed")
	}
	defer directory.Close()
	if journal.payloadPresent {
		// A valid host transaction durably publishes its authenticated plan
		// before detaching the target. Without that plan there is no authority
		// binding this payload to background-tasks.json, so restoring it would
		// turn attacker-controlled journal bytes into an executable task store.
		return errors.New("legacy Electron unauthenticated journal contains an unbound payload")
	}
	if !inventory.TargetPresent {
		return errors.New("legacy Electron unauthenticated journal cannot prove payload custody")
	}
	if err := cleanupJournalFiles(ctx, directory, journal); err != nil {
		return err
	}
	return root.RemoveEmptyDirectoryExact(ctx, journalDirectoryV1, journal.directory)
}

func (store *Store) completeAuthenticated(
	ctx context.Context,
	access *persistencefs.SeparateOwnerRootAccess,
	inventory InventoryV1,
	journal journalViewV1,
) error {
	root, err := access.Directory()
	if err != nil {
		return err
	}
	directory, present, err := root.OpenDirectory(ctx, journalDirectoryV1, true)
	if err != nil || !present || directory.State() != journal.directory {
		return errors.New("legacy Electron owner journal changed")
	}
	defer directory.Close()
	signing, err := store.journalAuthority.OpenExisting(ctx)
	if err != nil || journal.plan == nil ||
		validateJournalPlan(ctx, *journal.plan, directory.State(), signing) != nil || !inventory.RootPresent ||
		journal.plan.AuthorityDigest != inventory.AuthorityDigest ||
		journal.plan.Root != directoryStateFromPlatform(inventory.Root) {
		return errors.Join(errors.New("legacy Electron owner journal authority changed"), err)
	}
	expected := platformFileState(journal.plan.Target)
	expectedProtected := platformFileState(journal.plan.ProtectedTarget)
	if !expected.Valid() || !expectedProtected.Valid() || !sameFileMaterial(expected, expectedProtected) {
		return errors.New("legacy Electron owner journal target state is invalid")
	}
	if journal.commitValid {
		if inventory.TargetPresent || journal.payloadPresent {
			return errors.New("committed legacy Electron owner target reappeared")
		}
		return directory.RemoveKnownTempFiles(ctx, journal.temps)
	}
	if journal.protectionState.Valid() && !journal.protectionValid {
		return errors.New("legacy Electron owner protection proof is invalid")
	}
	if journal.protectionValid && inventory.TargetPresent && inventory.Target != expectedProtected {
		return errors.New("legacy Electron owner target regressed after protection proof")
	}
	if journal.removalIntentState.Valid() && !journal.removalIntentValid {
		return errors.New("legacy Electron owner removal intent is invalid")
	}
	if journal.removalIntentValid && inventory.TargetPresent {
		return errors.New("legacy Electron owner target reappeared after removal authorization")
	}
	if journal.commitState.Valid() && !journal.commitValid {
		return errors.New("legacy Electron owner commit is invalid")
	}
	if !inventory.TargetPresent && !journal.payloadPresent && !journal.removalIntentValid {
		return errors.New("legacy Electron owner target absence lacks a removal intent")
	}
	if inventory.TargetPresent && journal.payloadPresent {
		return errors.New("legacy Electron owner transaction has two live payloads")
	}
	if journal.payloadPresent && (!journal.protectionValid || journal.payload != expectedProtected) {
		return errors.New("legacy Electron owner quarantine changed")
	}
	custody := journal.payload
	if journal.removalIntentValid && journal.removalIntent != nil {
		custody = platformFileState(journal.removalIntent.Target)
		if journal.payloadPresent && journal.payload != custody {
			return errors.New("legacy Electron owner quarantine changed after removal authorization")
		}
	}
	targetNeedsProtection := false
	if inventory.TargetPresent {
		if inventory.Target == expected {
			targetNeedsProtection = true
		} else if inventory.Target == expectedProtected {
			protected, _, present, captureErr := root.CaptureFile(
				ctx, BackgroundTaskFileV1, MaxBackgroundTaskBytesV1, true, true,
			)
			if captureErr != nil || !present || protected != expectedProtected {
				return errors.New("legacy Electron owner protected target changed")
			}
			custody = protected
		} else {
			return errors.New("legacy Electron owner target security state is neither planned state")
		}
	}
	// All plan, authority, payload, and target semantics are proven before the
	// first recovery mutation. Crash residue never masks a later owner failure.
	if err := directory.RemoveKnownTempFiles(ctx, journal.temps); err != nil {
		return err
	}
	if inventory.TargetPresent {
		if targetNeedsProtection {
			protected, protectErr := root.ProtectFileExact(ctx, BackgroundTaskFileV1, expected)
			if protectErr != nil || protected != expectedProtected {
				return errors.Join(errors.New("legacy Electron owner target protection failed"), protectErr)
			}
			custody = protected
			if err := callHook(store.hooks.afterTargetProtected); err != nil {
				return err
			}
		}
		if custody != expectedProtected {
			return errors.New("legacy Electron owner protected target does not match the authenticated projection")
		}
		if !journal.protectionValid {
			proof, err := newJournalProtection(ctx, signing, *journal.plan, custody)
			if err != nil {
				return err
			}
			body, err := json.Marshal(proof)
			if err != nil {
				return err
			}
			if err := directory.WriteExclusiveAtomic(
				ctx, journalProtectionFileV1, body, maxJournalBytesV1, store.atomicFault("protection"),
			); err != nil {
				return err
			}
			state, proofBody, proofPresent, captureErr := directory.CaptureFile(
				ctx, journalProtectionFileV1, maxJournalBytesV1, true, true,
			)
			persisted, decodeErr := decodeStrict[journalProtectionV1](proofBody, maxJournalBytesV1)
			clear(proofBody)
			if captureErr != nil || decodeErr != nil || !proofPresent || !state.Valid() ||
				persisted != proof || validateJournalProtection(ctx, persisted, *journal.plan, signing) != nil ||
				platformFileState(persisted.ProtectedTarget) != expectedProtected {
				return errors.Join(
					errors.New("legacy Electron owner durable protection proof readback failed"),
					captureErr, decodeErr,
				)
			}
			journal.protection = &persisted
			journal.protectionState = state
			journal.protectionValid = true
			if err := callHook(store.hooks.afterProtectionPrepared); err != nil {
				return err
			}
		}
		if !journal.protectionValid {
			return errors.New("legacy Electron owner target detach lacks durable protection proof")
		}
		if err := callHook(store.hooks.beforeDetach); err != nil {
			return err
		}
		current, _, stillPresent, captureErr := root.CaptureFile(ctx, BackgroundTaskFileV1, MaxBackgroundTaskBytesV1, true, true)
		if captureErr != nil || !stillPresent || current != custody {
			return errors.New("legacy Electron owner target changed before detach")
		}
		if err := root.MoveFileNoReplace(
			ctx, BackgroundTaskFileV1, directory, journalQuarantineFileV1, custody, true,
		); err != nil {
			return err
		}
		inventory.TargetPresent = false
		journal.payload = custody
		journal.payloadPresent = true
		if err := callHook(store.hooks.afterDetach); err != nil {
			return err
		}
	}
	removedThisRun := false
	if journal.payloadPresent {
		custody = journal.payload
		if err := callHook(store.hooks.beforeQuarantineRemove); err != nil {
			return err
		}
		current, _, stillPresent, captureErr := directory.CaptureFile(ctx, journalQuarantineFileV1, MaxBackgroundTaskBytesV1, true, true)
		if captureErr != nil || !stillPresent || current != custody || !sameFileMaterial(current, expected) {
			return errors.New("legacy Electron owner quarantine changed before removal")
		}
		if !journal.removalIntentValid {
			if journal.protection == nil || !journal.protectionValid {
				return errors.New("legacy Electron owner removal intent lacks a verified protection proof")
			}
			intent, err := newJournalRemovalIntent(
				ctx, signing, *journal.plan, *journal.protection, custody,
			)
			if err != nil {
				return err
			}
			body, err := json.Marshal(intent)
			if err != nil {
				return err
			}
			if err := directory.WriteExclusiveAtomic(
				ctx, journalRemovalIntentFileV1, body, maxJournalBytesV1, store.atomicFault("removal_intent"),
			); err != nil {
				return err
			}
			state, intentBody, intentPresent, captureErr := directory.CaptureFile(
				ctx, journalRemovalIntentFileV1, maxJournalBytesV1, true, true,
			)
			persisted, decodeErr := decodeStrict[journalRemovalIntentV1](intentBody, maxJournalBytesV1)
			clear(intentBody)
			if captureErr != nil || decodeErr != nil || !intentPresent || !state.Valid() ||
				persisted != intent ||
				validateJournalRemovalIntent(
					ctx, persisted, *journal.plan, *journal.protection, signing,
				) != nil || platformFileState(persisted.Target) != custody {
				return errors.Join(
					errors.New("legacy Electron owner durable removal intent readback failed"),
					captureErr, decodeErr,
				)
			}
			journal.removalIntent = &persisted
			journal.removalIntentState = state
			journal.removalIntentValid = true
			if err := callHook(store.hooks.afterRemovalPrepared); err != nil {
				return err
			}
		}
		if err := directory.RemoveFileExactObserved(
			ctx, journalQuarantineFileV1, custody, true, store.hooks.removeFileFault,
		); err != nil {
			return err
		}
		journal.payloadPresent = false
		removedThisRun = true
	}
	if _, _, targetPresent, err := root.CaptureFile(ctx, BackgroundTaskFileV1, MaxBackgroundTaskBytesV1, true, false); err != nil || targetPresent {
		return errors.New("legacy Electron owner target removal readback failed")
	}
	if !journal.removalIntentValid {
		return errors.New("legacy Electron owner target absence lacks durable removal authorization")
	}
	if removedThisRun {
		if err := callHook(store.hooks.afterQuarantineRemoved); err != nil {
			return err
		}
	}
	if err := callHook(store.hooks.beforeCommit); err != nil {
		return err
	}
	if err := requireLegacyTargetAbsent(ctx, root); err != nil {
		return err
	}
	if journal.protection == nil || journal.removalIntent == nil {
		return errors.New("legacy Electron owner commit lacks a verified custody chain")
	}
	commit, err := newJournalCommit(
		ctx, signing, *journal.plan, *journal.protection, *journal.removalIntent,
	)
	if err != nil {
		return err
	}
	body, err := json.Marshal(commit)
	if err != nil {
		return err
	}
	if err := directory.WriteExclusiveAtomic(
		ctx, journalCommitFileV1, body, maxJournalBytesV1, store.atomicFault("commit"),
	); err != nil {
		return err
	}
	if err := callHook(store.hooks.afterCommitPrepared); err != nil {
		return err
	}
	finalInventory, finalJournal, err := store.captureOpened(ctx, access, root)
	if err != nil {
		return errors.Join(errors.New("legacy Electron owner commit readback failed"), err)
	}
	observation, err := newObservation(finalInventory, finalJournal)
	if err != nil || !observation.Committed() || observation.RecoveryRequired() || observation.TargetPresent() {
		return errors.Join(errors.New("legacy Electron owner commit did not reach the authenticated fixed point"), err)
	}
	return nil
}

func requireLegacyTargetAbsent(
	ctx context.Context,
	root *persistencefs.SeparateOwnerDirectory,
) error {
	if root == nil {
		return errors.New("legacy Electron owner target absence authority is unavailable")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, _, present, err := root.CaptureFile(
			ctx, BackgroundTaskFileV1, MaxBackgroundTaskBytesV1, true, false,
		); err != nil || present {
			return errors.Join(errors.New("legacy Electron owner target reappeared before commit"), err)
		}
	}
	return nil
}

func sameFileMaterial(left, right persistencefs.SeparateOwnerFileState) bool {
	return left.Valid() && right.Valid() && left.Identity == right.Identity && left.Size == right.Size &&
		left.ModifiedUnixNano == right.ModifiedUnixNano && left.SHA256 == right.SHA256
}

func cleanupJournalFiles(
	ctx context.Context,
	directory *persistencefs.SeparateOwnerDirectory,
	journal journalViewV1,
) error {
	if err := directory.RemoveKnownTempFiles(ctx, journal.temps); err != nil {
		return err
	}
	if journal.commitState.Valid() {
		if err := directory.RemoveFileExact(ctx, journalCommitFileV1, journal.commitState, true); err != nil {
			return err
		}
	}
	if journal.removalIntentState.Valid() {
		if err := directory.RemoveFileExact(ctx, journalRemovalIntentFileV1, journal.removalIntentState, true); err != nil {
			return err
		}
	}
	if journal.protectionState.Valid() {
		if err := directory.RemoveFileExact(ctx, journalProtectionFileV1, journal.protectionState, true); err != nil {
			return err
		}
	}
	if journal.planState.Valid() {
		if err := directory.RemoveFileExact(ctx, journalPlanFileV1, journal.planState, true); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) atomicFault(prefix string) persistencefs.SeparateOwnerWriteFault {
	if store == nil || store.hooks.atomicWriteFault == nil {
		return nil
	}
	return func(stage string) error { return store.hooks.atomicWriteFault(prefix + "_" + stage) }
}

func callHook(hook func() error) error {
	if hook == nil {
		return nil
	}
	return hook()
}

func journalPlanFromInventory(
	ctx context.Context,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
	inventory InventoryV1,
	protected persistencefs.SeparateOwnerFileState,
	journalDirectory persistencefs.SeparateOwnerDirectoryState,
) (journalPlanV1, error) {
	identity, err := signing.Identity(ctx)
	if err != nil {
		return journalPlanV1{}, err
	}
	plan := journalPlanV1{
		SchemaVersion:      journalSchemaVersionV2,
		Purpose:            journalPurposeV1,
		RootBindingDigest:  identity.RootBindingDigest,
		AuthorityDigest:    inventory.AuthorityDigest,
		Root:               directoryStateFromPlatform(inventory.Root),
		JournalDirectory:   directoryStateFromPlatform(journalDirectory),
		Target:             fileStateFromPlatform(inventory.Target),
		ProtectedTarget:    fileStateFromPlatform(protected),
		AuthorityAlgorithm: identity.AuthorityAlgorithm,
		AuthorityKeyID:     identity.AuthorityKeyID,
	}
	plan.PlanDigest = digestJournalPlan(plan)
	message, err := journalPlanSigningBytes(plan)
	if err != nil {
		return journalPlanV1{}, err
	}
	plan.AuthoritySignature, err = signing.SignPlanV2(ctx, message)
	if err != nil {
		return journalPlanV1{}, err
	}
	return plan, nil
}

func validateJournalPlan(
	ctx context.Context,
	plan journalPlanV1,
	journalDirectory persistencefs.SeparateOwnerDirectoryState,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
) error {
	target := platformFileState(plan.Target)
	protected := platformFileState(plan.ProtectedTarget)
	identity, identityErr := signing.Identity(ctx)
	if identityErr != nil || plan.SchemaVersion != journalSchemaVersionV2 || plan.Purpose != journalPurposeV1 ||
		plan.RootBindingDigest != identity.RootBindingDigest || !isSHA256(plan.RootBindingDigest) ||
		!isSHA256(plan.AuthorityDigest) || !platformDirectoryState(plan.Root).Valid() ||
		!platformDirectoryState(plan.JournalDirectory).Valid() ||
		platformDirectoryState(plan.JournalDirectory) != journalDirectory || !target.Valid() || !protected.Valid() ||
		!sameFileMaterial(target, protected) ||
		plan.AuthorityAlgorithm != journalAuthorityAlgorithmV2 ||
		plan.AuthorityAlgorithm != identity.AuthorityAlgorithm || plan.AuthorityKeyID != identity.AuthorityKeyID ||
		!isSHA256(plan.AuthorityKeyID) || !isSHA256(plan.PlanDigest) || digestJournalPlan(plan) != plan.PlanDigest ||
		plan.AuthoritySignature == "" {
		return errors.New("legacy Electron owner journal plan integrity is invalid")
	}
	message, err := journalPlanSigningBytes(plan)
	if err != nil {
		return err
	}
	return signing.VerifyPlanV2(ctx, plan.AuthorityKeyID, message, plan.AuthoritySignature)
}

func digestJournalPlan(plan journalPlanV1) string {
	body, _ := json.Marshal(struct {
		SchemaVersion      int              `json:"schemaVersion"`
		Purpose            string           `json:"purpose"`
		RootBindingDigest  string           `json:"rootBindingDigest"`
		AuthorityDigest    string           `json:"authorityDigest"`
		Root               directoryStateV1 `json:"root"`
		JournalDirectory   directoryStateV1 `json:"journalDirectory"`
		Target             fileStateV1      `json:"target"`
		ProtectedTarget    fileStateV1      `json:"protectedTarget"`
		AuthorityAlgorithm string           `json:"authorityAlgorithm"`
		AuthorityKeyID     string           `json:"authorityKeyId"`
	}{
		plan.SchemaVersion, plan.Purpose, plan.RootBindingDigest, plan.AuthorityDigest,
		plan.Root, plan.JournalDirectory, plan.Target, plan.ProtectedTarget,
		plan.AuthorityAlgorithm, plan.AuthorityKeyID,
	})
	return sha256DomainHex(journalPlanDigestDomainV2, body)
}

func journalPlanSigningBytes(plan journalPlanV1) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion      int              `json:"schemaVersion"`
		Purpose            string           `json:"purpose"`
		RootBindingDigest  string           `json:"rootBindingDigest"`
		AuthorityDigest    string           `json:"authorityDigest"`
		Root               directoryStateV1 `json:"root"`
		JournalDirectory   directoryStateV1 `json:"journalDirectory"`
		Target             fileStateV1      `json:"target"`
		ProtectedTarget    fileStateV1      `json:"protectedTarget"`
		AuthorityAlgorithm string           `json:"authorityAlgorithm"`
		AuthorityKeyID     string           `json:"authorityKeyId"`
		PlanDigest         string           `json:"planDigest"`
	}{
		plan.SchemaVersion, plan.Purpose, plan.RootBindingDigest, plan.AuthorityDigest,
		plan.Root, plan.JournalDirectory, plan.Target, plan.ProtectedTarget,
		plan.AuthorityAlgorithm, plan.AuthorityKeyID, plan.PlanDigest,
	})
}

func newJournalProtection(
	ctx context.Context,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
	plan journalPlanV1,
	target persistencefs.SeparateOwnerFileState,
) (journalProtectionV1, error) {
	proof := journalProtectionV1{
		SchemaVersion:      journalSchemaVersionV2,
		Purpose:            journalProtectionPurposeV1,
		RootBindingDigest:  plan.RootBindingDigest,
		PlanDigest:         plan.PlanDigest,
		ProtectedTarget:    fileStateFromPlatform(target),
		AuthorityAlgorithm: plan.AuthorityAlgorithm,
		AuthorityKeyID:     plan.AuthorityKeyID,
	}
	proof.ProofDigest = digestJournalProtection(proof)
	message, err := journalProtectionSigningBytes(proof)
	if err != nil {
		return journalProtectionV1{}, err
	}
	proof.AuthoritySignature, err = signing.SignProtectionV2(ctx, message)
	if err != nil {
		return journalProtectionV1{}, err
	}
	return proof, nil
}

func validateJournalProtection(
	ctx context.Context,
	proof journalProtectionV1,
	plan journalPlanV1,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
) error {
	if proof.SchemaVersion != journalSchemaVersionV2 || proof.Purpose != journalProtectionPurposeV1 ||
		proof.RootBindingDigest != plan.RootBindingDigest || proof.PlanDigest != plan.PlanDigest ||
		proof.AuthorityAlgorithm != plan.AuthorityAlgorithm || proof.AuthorityKeyID != plan.AuthorityKeyID ||
		!platformFileState(proof.ProtectedTarget).Valid() || !isSHA256(proof.ProofDigest) ||
		digestJournalProtection(proof) != proof.ProofDigest || proof.AuthoritySignature == "" {
		return errors.New("legacy Electron owner journal protection proof integrity is invalid")
	}
	message, err := journalProtectionSigningBytes(proof)
	if err != nil {
		return err
	}
	return signing.VerifyProtectionV2(ctx, proof.AuthorityKeyID, message, proof.AuthoritySignature)
}

func digestJournalProtection(proof journalProtectionV1) string {
	body, _ := json.Marshal(struct {
		SchemaVersion      int         `json:"schemaVersion"`
		Purpose            string      `json:"purpose"`
		RootBindingDigest  string      `json:"rootBindingDigest"`
		PlanDigest         string      `json:"planDigest"`
		ProtectedTarget    fileStateV1 `json:"protectedTarget"`
		AuthorityAlgorithm string      `json:"authorityAlgorithm"`
		AuthorityKeyID     string      `json:"authorityKeyId"`
	}{
		proof.SchemaVersion, proof.Purpose, proof.RootBindingDigest, proof.PlanDigest,
		proof.ProtectedTarget, proof.AuthorityAlgorithm, proof.AuthorityKeyID,
	})
	return sha256DomainHex(journalProtectionDigestV2, body)
}

func journalProtectionSigningBytes(proof journalProtectionV1) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion      int         `json:"schemaVersion"`
		Purpose            string      `json:"purpose"`
		RootBindingDigest  string      `json:"rootBindingDigest"`
		PlanDigest         string      `json:"planDigest"`
		ProtectedTarget    fileStateV1 `json:"protectedTarget"`
		AuthorityAlgorithm string      `json:"authorityAlgorithm"`
		AuthorityKeyID     string      `json:"authorityKeyId"`
		ProofDigest        string      `json:"proofDigest"`
	}{
		proof.SchemaVersion, proof.Purpose, proof.RootBindingDigest, proof.PlanDigest,
		proof.ProtectedTarget, proof.AuthorityAlgorithm, proof.AuthorityKeyID, proof.ProofDigest,
	})
}

func newJournalRemovalIntent(
	ctx context.Context,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
	plan journalPlanV1,
	proof journalProtectionV1,
	target persistencefs.SeparateOwnerFileState,
) (journalRemovalIntentV1, error) {
	intent := journalRemovalIntentV1{
		SchemaVersion:      journalSchemaVersionV2,
		Purpose:            journalRemovalIntentPurposeV1,
		RootBindingDigest:  plan.RootBindingDigest,
		PlanDigest:         plan.PlanDigest,
		ProtectionDigest:   proof.ProofDigest,
		Target:             fileStateFromPlatform(target),
		AuthorityAlgorithm: plan.AuthorityAlgorithm,
		AuthorityKeyID:     plan.AuthorityKeyID,
	}
	intent.IntentDigest = digestJournalRemovalIntent(intent)
	message, err := journalRemovalIntentSigningBytes(intent)
	if err != nil {
		return journalRemovalIntentV1{}, err
	}
	intent.AuthoritySignature, err = signing.SignRemovalIntentV2(ctx, message)
	if err != nil {
		return journalRemovalIntentV1{}, err
	}
	return intent, nil
}

func validateJournalRemovalIntent(
	ctx context.Context,
	intent journalRemovalIntentV1,
	plan journalPlanV1,
	proof journalProtectionV1,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
) error {
	if intent.SchemaVersion != journalSchemaVersionV2 || intent.Purpose != journalRemovalIntentPurposeV1 ||
		intent.RootBindingDigest != plan.RootBindingDigest || intent.PlanDigest != plan.PlanDigest ||
		intent.ProtectionDigest != proof.ProofDigest || intent.AuthorityAlgorithm != plan.AuthorityAlgorithm ||
		intent.AuthorityKeyID != plan.AuthorityKeyID || !platformFileState(intent.Target).Valid() ||
		!isSHA256(intent.IntentDigest) || digestJournalRemovalIntent(intent) != intent.IntentDigest ||
		intent.AuthoritySignature == "" {
		return errors.New("legacy Electron owner journal removal intent integrity is invalid")
	}
	message, err := journalRemovalIntentSigningBytes(intent)
	if err != nil {
		return err
	}
	return signing.VerifyRemovalIntentV2(ctx, intent.AuthorityKeyID, message, intent.AuthoritySignature)
}

func digestJournalRemovalIntent(intent journalRemovalIntentV1) string {
	body, _ := json.Marshal(struct {
		SchemaVersion      int         `json:"schemaVersion"`
		Purpose            string      `json:"purpose"`
		RootBindingDigest  string      `json:"rootBindingDigest"`
		PlanDigest         string      `json:"planDigest"`
		ProtectionDigest   string      `json:"protectionDigest"`
		Target             fileStateV1 `json:"target"`
		AuthorityAlgorithm string      `json:"authorityAlgorithm"`
		AuthorityKeyID     string      `json:"authorityKeyId"`
	}{
		intent.SchemaVersion, intent.Purpose, intent.RootBindingDigest, intent.PlanDigest,
		intent.ProtectionDigest, intent.Target, intent.AuthorityAlgorithm, intent.AuthorityKeyID,
	})
	return sha256DomainHex(journalRemovalIntentDigestV2, body)
}

func journalRemovalIntentSigningBytes(intent journalRemovalIntentV1) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion      int         `json:"schemaVersion"`
		Purpose            string      `json:"purpose"`
		RootBindingDigest  string      `json:"rootBindingDigest"`
		PlanDigest         string      `json:"planDigest"`
		ProtectionDigest   string      `json:"protectionDigest"`
		Target             fileStateV1 `json:"target"`
		AuthorityAlgorithm string      `json:"authorityAlgorithm"`
		AuthorityKeyID     string      `json:"authorityKeyId"`
		IntentDigest       string      `json:"intentDigest"`
	}{
		intent.SchemaVersion, intent.Purpose, intent.RootBindingDigest, intent.PlanDigest,
		intent.ProtectionDigest, intent.Target, intent.AuthorityAlgorithm, intent.AuthorityKeyID,
		intent.IntentDigest,
	})
}

func newJournalCommit(
	ctx context.Context,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
	plan journalPlanV1,
	proof journalProtectionV1,
	intent journalRemovalIntentV1,
) (journalCommitV1, error) {
	commit := journalCommitV1{
		SchemaVersion:       journalSchemaVersionV2,
		Purpose:             journalCommitPurposeV1,
		RootBindingDigest:   plan.RootBindingDigest,
		PlanDigest:          plan.PlanDigest,
		ProtectionDigest:    proof.ProofDigest,
		RemovalIntentDigest: intent.IntentDigest,
		TargetState:         "absent",
		AuthorityAlgorithm:  plan.AuthorityAlgorithm,
		AuthorityKeyID:      plan.AuthorityKeyID,
	}
	commit.CommitDigest = digestJournalCommit(commit)
	message, err := journalCommitSigningBytes(commit)
	if err != nil {
		return journalCommitV1{}, err
	}
	commit.AuthoritySignature, err = signing.SignCommitV2(ctx, message)
	if err != nil {
		return journalCommitV1{}, err
	}
	return commit, nil
}

func validateJournalCommit(
	ctx context.Context,
	commit journalCommitV1,
	plan journalPlanV1,
	proof journalProtectionV1,
	intent journalRemovalIntentV1,
	signing *persistencefs.ElectronTaskRetirementSigningSessionV2,
) error {
	if commit.SchemaVersion != journalSchemaVersionV2 || commit.Purpose != journalCommitPurposeV1 ||
		commit.RootBindingDigest != plan.RootBindingDigest || commit.PlanDigest != plan.PlanDigest ||
		commit.ProtectionDigest != proof.ProofDigest || commit.RemovalIntentDigest != intent.IntentDigest ||
		commit.TargetState != "absent" || commit.AuthorityAlgorithm != plan.AuthorityAlgorithm ||
		commit.AuthorityKeyID != plan.AuthorityKeyID || !isSHA256(commit.CommitDigest) ||
		digestJournalCommit(commit) != commit.CommitDigest || commit.AuthoritySignature == "" {
		return errors.New("legacy Electron owner journal commit integrity is invalid")
	}
	message, err := journalCommitSigningBytes(commit)
	if err != nil {
		return err
	}
	return signing.VerifyCommitV2(ctx, commit.AuthorityKeyID, message, commit.AuthoritySignature)
}

func digestJournalCommit(commit journalCommitV1) string {
	body, _ := json.Marshal(struct {
		SchemaVersion       int    `json:"schemaVersion"`
		Purpose             string `json:"purpose"`
		RootBindingDigest   string `json:"rootBindingDigest"`
		PlanDigest          string `json:"planDigest"`
		ProtectionDigest    string `json:"protectionDigest"`
		RemovalIntentDigest string `json:"removalIntentDigest"`
		TargetState         string `json:"targetState"`
		AuthorityAlgorithm  string `json:"authorityAlgorithm"`
		AuthorityKeyID      string `json:"authorityKeyId"`
	}{
		commit.SchemaVersion, commit.Purpose, commit.RootBindingDigest, commit.PlanDigest,
		commit.ProtectionDigest, commit.RemovalIntentDigest, commit.TargetState,
		commit.AuthorityAlgorithm, commit.AuthorityKeyID,
	})
	return sha256DomainHex(journalCommitDigestDomainV2, body)
}

func journalCommitSigningBytes(commit journalCommitV1) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion       int    `json:"schemaVersion"`
		Purpose             string `json:"purpose"`
		RootBindingDigest   string `json:"rootBindingDigest"`
		PlanDigest          string `json:"planDigest"`
		ProtectionDigest    string `json:"protectionDigest"`
		RemovalIntentDigest string `json:"removalIntentDigest"`
		TargetState         string `json:"targetState"`
		AuthorityAlgorithm  string `json:"authorityAlgorithm"`
		AuthorityKeyID      string `json:"authorityKeyId"`
		CommitDigest        string `json:"commitDigest"`
	}{
		commit.SchemaVersion, commit.Purpose, commit.RootBindingDigest, commit.PlanDigest,
		commit.ProtectionDigest, commit.RemovalIntentDigest, commit.TargetState,
		commit.AuthorityAlgorithm, commit.AuthorityKeyID, commit.CommitDigest,
	})
}

func newInventory(inventory InventoryV1) (InventoryV1, error) {
	if !isSHA256(inventory.AuthorityDigest) || inventory.RootPresent != inventory.Root.Valid() ||
		inventory.TargetPresent && (!inventory.RootPresent || !inventory.Target.Valid()) ||
		!inventory.TargetPresent && inventory.Target != (persistencefs.SeparateOwnerFileState{}) {
		return InventoryV1{}, errors.New("legacy Electron owner inventory is invalid")
	}
	body, _ := json.Marshal(struct {
		SchemaVersion   int              `json:"schemaVersion"`
		AuthorityDigest string           `json:"authorityDigest"`
		RootPresent     bool             `json:"rootPresent"`
		Root            directoryStateV1 `json:"root"`
		TargetPresent   bool             `json:"targetPresent"`
		Target          fileStateV1      `json:"target"`
	}{1, inventory.AuthorityDigest, inventory.RootPresent, directoryStateFromPlatform(inventory.Root), inventory.TargetPresent, fileStateFromPlatform(inventory.Target)})
	inventory.Digest = sha256Hex(body)
	return inventory, nil
}

func validInventory(inventory InventoryV1) bool {
	expected, err := newInventory(InventoryV1{
		AuthorityDigest: inventory.AuthorityDigest, RootPresent: inventory.RootPresent, Root: inventory.Root,
		TargetPresent: inventory.TargetPresent, Target: inventory.Target,
	})
	return err == nil && expected == inventory
}

func digestJournalView(view journalViewV1) string {
	tempNames := make([]string, 0, len(view.temps))
	for name := range view.temps {
		tempNames = append(tempNames, name)
	}
	sort.Strings(tempNames)
	temps := make([]struct {
		Name  string      `json:"name"`
		State fileStateV1 `json:"state"`
	}, 0, len(tempNames))
	for _, name := range tempNames {
		temps = append(temps, struct {
			Name  string      `json:"name"`
			State fileStateV1 `json:"state"`
		}{name, fileStateFromPlatform(view.temps[name])})
	}
	body, _ := json.Marshal(struct {
		SchemaVersion      int              `json:"schemaVersion"`
		Present            bool             `json:"present"`
		Directory          directoryStateV1 `json:"directory"`
		PlanState          fileStateV1      `json:"planState"`
		PlanValid          bool             `json:"planValid"`
		ProtectionState    fileStateV1      `json:"protectionState"`
		ProtectionValid    bool             `json:"protectionValid"`
		RemovalIntentState fileStateV1      `json:"removalIntentState"`
		RemovalIntentValid bool             `json:"removalIntentValid"`
		CommitState        fileStateV1      `json:"commitState"`
		CommitValid        bool             `json:"commitValid"`
		Payload            fileStateV1      `json:"payload"`
		PayloadPresent     bool             `json:"payloadPresent"`
		Temps              any              `json:"temps"`
	}{1, view.present, directoryStateFromPlatform(view.directory), fileStateFromPlatform(view.planState), view.planValid,
		fileStateFromPlatform(view.protectionState), view.protectionValid,
		fileStateFromPlatform(view.removalIntentState), view.removalIntentValid, fileStateFromPlatform(view.commitState), view.commitValid,
		fileStateFromPlatform(view.payload), view.payloadPresent, temps})
	return sha256Hex(body)
}

func fileStateFromPlatform(state persistencefs.SeparateOwnerFileState) fileStateV1 {
	return fileStateV1{
		Identity: state.Identity, SecurityDigest: state.SecurityDigest, Size: state.Size,
		ModifiedUnixNano: state.ModifiedUnixNano, SHA256: state.SHA256,
	}
}

func platformFileState(state fileStateV1) persistencefs.SeparateOwnerFileState {
	return persistencefs.SeparateOwnerFileState{
		Identity: state.Identity, SecurityDigest: state.SecurityDigest, Size: state.Size,
		ModifiedUnixNano: state.ModifiedUnixNano, SHA256: state.SHA256,
	}
}

func directoryStateFromPlatform(state persistencefs.SeparateOwnerDirectoryState) directoryStateV1 {
	return directoryStateV1{Identity: state.Identity, SecurityDigest: state.SecurityDigest, Protected: state.Protected}
}

func platformDirectoryState(state directoryStateV1) persistencefs.SeparateOwnerDirectoryState {
	return persistencefs.SeparateOwnerDirectoryState{
		Identity: state.Identity, SecurityDigest: state.SecurityDigest, Protected: state.Protected,
	}
}

func decodeStrict[T any](body []byte, maxBytes int64) (T, error) {
	var zero T
	if int64(len(body)) > maxBytes || len(body) == 0 {
		return zero, errors.New("legacy Electron owner journal file is invalid")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: int(maxBytes), MaxTokens: 4096, MaxStringBytes: 4096,
	}); err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var result T
	if err := decoder.Decode(&result); err != nil {
		return zero, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return zero, errors.New("legacy Electron owner journal has trailing data")
	}
	canonical, err := json.Marshal(result)
	if err != nil || !bytes.Equal(canonical, body) {
		return zero, errors.New("legacy Electron owner journal is not canonical")
	}
	return result, nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sha256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func sha256DomainHex(domain string, body []byte) string {
	payload := make([]byte, 0, len(domain)+len(body))
	payload = append(payload, domain...)
	payload = append(payload, body...)
	return sha256Hex(payload)
}
