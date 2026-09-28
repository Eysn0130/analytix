//go:build darwin || linux

// Package sharedwitnessownerfs persists the SharedEvidence monotonic-head
// witness independently of the Go runtime data roots. It owns no transport,
// enrollment, or packaged-startup bootstrap.
package sharedwitnessownerfs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	monotonichead "analytix.local/runtime-go/internal/ports/monotonichead"
)

const (
	OwnerRootName            = "runtime-shared-evidence-witness-owner-v1"
	stateDirectoryName       = "state-v1"
	genesisFileName          = "genesis.json"
	genesisPurpose           = "analytix.shared-evidence-witness-owner-genesis/v1"
	journalPurpose           = "analytix.shared-evidence-witness-owner-journal/v1"
	maxRecordBytes     int64 = 64 << 10
	maxJournalFiles          = 100_000
)

// Anchor must come from the accepted installation enrollment, not from the
// owner root being opened. In particular, opening a missing owner never creates
// a new seed or substitutes its own genesis for the enrolled checkpoint.
type Anchor struct {
	InstallationID     string
	AuthorityKeyID     string
	AuthorityPublicKey ed25519.PublicKey
	EnrollmentID       string
	GenesisCheckpoint  domainsecurity.MonotonicHeadCheckpointV1
}

type genesisRecord struct {
	FormatVersion      int                                      `json:"formatVersion"`
	Purpose            string                                   `json:"purpose"`
	InstallationID     string                                   `json:"installationId"`
	AuthorityKeyID     string                                   `json:"authorityKeyId"`
	AuthorityPublicKey string                                   `json:"authorityPublicKey"`
	EnrollmentID       string                                   `json:"enrollmentId"`
	WitnessSeed        []byte                                   `json:"witnessSeed"`
	GenesisCheckpoint  domainsecurity.MonotonicHeadCheckpointV1 `json:"genesisCheckpoint"`
}

type journalRecord struct {
	FormatVersion        int                                          `json:"formatVersion"`
	Purpose              string                                       `json:"purpose"`
	PreviousRecordDigest string                                       `json:"previousRecordDigest"`
	AdvanceRequest       domainsecurity.MonotonicHeadAdvanceRequestV1 `json:"advanceRequest"`
	AdvanceReceipt       domainsecurity.MonotonicHeadAdvanceReceiptV1 `json:"advanceReceipt"`
	NextCheckpoint       domainsecurity.MonotonicHeadCheckpointV1     `json:"nextCheckpoint"`
}

type committedMutation struct {
	request domainsecurity.MonotonicHeadAdvanceRequestV1
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1
}

type Owner struct {
	mu               sync.Mutex
	lease            *persistencefs.SeparateOwnerLifetimeLease
	anchor           Anchor
	private          ed25519.PrivateKey
	stateDirectory   persistencefs.SeparateOwnerDirectoryState
	checkpoint       domainsecurity.MonotonicHeadCheckpointV1
	lastRecordDigest string
	committed        map[string]committedMutation
	poisoned         bool
	closed           bool
	writeFault       persistencefs.SeparateOwnerWriteFault
}

var _ monotonichead.MutationRecoveryWitness = (*Owner)(nil)

// createForIsolatedFixture initializes an already-created 0700 owner root for
// deterministic store tests. Production enrollment must introduce its own
// guarded creation path inside one Prepare and lease transaction.
func createForIsolatedFixture(ctx context.Context, userDataDir string, anchor Anchor, witnessPrivate ed25519.PrivateKey) (*Owner, error) {
	root, err := ownerRootPath(userDataDir)
	if err != nil || validateAnchor(anchor) != nil || len(witnessPrivate) != ed25519.PrivateKeySize ||
		!bytes.Equal(witnessPrivate.Public().(ed25519.PublicKey), mustWitnessPublic(anchor.GenesisCheckpoint)) {
		return nil, errors.New("shared witness owner initialization authority is invalid")
	}
	lease, err := persistencefs.AcquireSeparateOwnerLifetimeLease(root)
	if err != nil {
		return nil, err
	}
	defer func() {
		if lease != nil {
			_ = lease.Close()
		}
	}()
	record := genesisRecord{
		FormatVersion: 1, Purpose: genesisPurpose,
		InstallationID: anchor.InstallationID, AuthorityKeyID: anchor.AuthorityKeyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(anchor.AuthorityPublicKey),
		EnrollmentID:       anchor.EnrollmentID,
		WitnessSeed:        append([]byte(nil), witnessPrivate.Seed()...),
		GenesisCheckpoint:  anchor.GenesisCheckpoint,
	}
	body, err := json.Marshal(record)
	clear(record.WitnessSeed)
	if err != nil {
		return nil, err
	}
	defer clear(body)
	err = lease.WithRootAccess(ctx, func(access *persistencefs.SeparateOwnerRootAccess) error {
		rootDir, err := access.Directory()
		if err != nil {
			return err
		}
		stateDir, err := rootDir.CreatePrivateDirectory(ctx, stateDirectoryName)
		if err != nil {
			return err
		}
		defer stateDir.Close()
		return stateDir.WriteExclusiveAtomic(ctx, genesisFileName, body, maxRecordBytes, nil)
	})
	if err != nil {
		return nil, err
	}
	owner, err := recoverOwner(ctx, lease, anchor, nil)
	if err != nil {
		return nil, err
	}
	lease = nil
	return owner, nil
}

// OpenExisting strictly recovers the exact enrolled owner; no missing or
// partial state is repaired by generating fresh identity or a new head.
func OpenExisting(ctx context.Context, userDataDir string, anchor Anchor) (*Owner, error) {
	return openExistingWithRecoverySync(ctx, userDataDir, anchor, nil)
}

// A non-nil syncDirectory is used only by isolated recovery fault tests.
func openExistingWithRecoverySync(ctx context.Context, userDataDir string, anchor Anchor,
	syncDirectory func(*persistencefs.SeparateOwnerDirectory) error) (*Owner, error) {
	root, err := ownerRootPath(userDataDir)
	if err != nil || validateAnchor(anchor) != nil {
		return nil, errors.New("shared witness owner enrollment anchor is invalid")
	}
	lease, err := persistencefs.AcquireSeparateOwnerLifetimeLease(root)
	if err != nil {
		return nil, err
	}
	owner, err := recoverOwner(ctx, lease, anchor, syncDirectory)
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	return owner, nil
}

func ownerRootPath(userDataDir string) (string, error) {
	if userDataDir == "" || !filepath.IsAbs(userDataDir) || filepath.Clean(userDataDir) != userDataDir ||
		strings.ContainsRune(userDataDir, 0) {
		return "", errors.New("Electron userData path is invalid")
	}
	canonical, err := filepath.EvalSymlinks(userDataDir)
	if err != nil || canonical != userDataDir {
		return "", errors.New("Electron userData path is not canonical")
	}
	root := filepath.Join(userDataDir, OwnerRootName)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return "", errors.New("shared witness owner root is unavailable or not private")
	}
	return root, nil
}

func validateAnchor(anchor Anchor) error {
	checkpoint := anchor.GenesisCheckpoint
	witnessPublic := mustWitnessPublic(checkpoint)
	if anchor.InstallationID == "" || anchor.AuthorityKeyID != domainsecurity.SHA256Hex(anchor.AuthorityPublicKey) ||
		len(anchor.AuthorityPublicKey) != ed25519.PublicKeySize || anchor.EnrollmentID == "" ||
		checkpoint.Generation != 0 || checkpoint.Namespace != domainenrollment.SharedEvidenceNamespaceV1 ||
		checkpoint.PreviousCheckpointDigest != "" || checkpoint.PreviousStateDigest != "" || checkpoint.MutationID != "" ||
		domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(checkpoint, anchor.InstallationID,
			anchor.EnrollmentID, checkpoint.WitnessKeyID, witnessPublic) != nil {
		return errors.New("shared witness owner gen0 anchor is invalid")
	}
	return nil
}

func mustWitnessPublic(checkpoint domainsecurity.MonotonicHeadCheckpointV1) ed25519.PublicKey {
	public, _ := base64.RawURLEncoding.DecodeString(checkpoint.WitnessPublicKey)
	return ed25519.PublicKey(public)
}

func journalName(generation uint64) string { return fmt.Sprintf("%020d.json", generation) }

func strictRecord(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: int(maxRecordBytes), MaxDepth: 8, MaxTokens: 256,
		MaxStringBytes: 8192, MaxNumberBytes: 24,
	}); err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return err
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(body, canonical) {
		return errors.New("shared witness owner record is not canonical")
	}
	return nil
}

func recoverOwner(ctx context.Context, lease *persistencefs.SeparateOwnerLifetimeLease, anchor Anchor,
	syncDirectory func(*persistencefs.SeparateOwnerDirectory) error) (*Owner, error) {
	owner := &Owner{lease: lease, anchor: Anchor{
		InstallationID: anchor.InstallationID, AuthorityKeyID: anchor.AuthorityKeyID,
		AuthorityPublicKey: append(ed25519.PublicKey(nil), anchor.AuthorityPublicKey...),
		EnrollmentID:       anchor.EnrollmentID, GenesisCheckpoint: anchor.GenesisCheckpoint,
	}, committed: map[string]committedMutation{}}
	err := lease.WithRootAccess(ctx, func(access *persistencefs.SeparateOwnerRootAccess) error {
		rootDir, err := access.Directory()
		if err != nil {
			return err
		}
		stateDir, present, err := rootDir.OpenDirectory(ctx, stateDirectoryName, true)
		if err != nil || !present {
			return errors.New("shared witness owner state directory is unavailable")
		}
		defer stateDir.Close()
		names, err := stateDir.Entries(ctx, maxJournalFiles+2)
		if err != nil {
			return err
		}
		generations := make([]uint64, 0, len(names))
		temps := make([]string, 0)
		foundGenesis := false
		for _, name := range names {
			if name == genesisFileName {
				foundGenesis = true
				continue
			}
			if strings.HasSuffix(name, ".tmp") {
				temps = append(temps, name)
				continue
			}
			if len(name) != len(journalName(1)) || !strings.HasSuffix(name, ".json") {
				return errors.New("shared witness owner inventory contains an unknown file")
			}
			generation, parseErr := strconv.ParseUint(strings.TrimSuffix(name, ".json"), 10, 64)
			if parseErr != nil || generation == 0 || journalName(generation) != name {
				return errors.New("shared witness owner journal name is invalid")
			}
			generations = append(generations, generation)
		}
		if !foundGenesis || len(generations) > maxJournalFiles {
			return errors.New("shared witness owner genesis or journal is unavailable")
		}
		_, genesisBody, present, err := stateDir.CaptureFile(ctx, genesisFileName, maxRecordBytes, false, true)
		if err != nil || !present {
			return errors.New("shared witness owner genesis cannot be read")
		}
		var genesis genesisRecord
		if err := strictRecord(genesisBody, &genesis); err != nil {
			clear(genesisBody)
			clear(genesis.WitnessSeed)
			return err
		}
		owner.lastRecordDigest = domainsecurity.SHA256Hex(genesisBody)
		clear(genesisBody)
		if genesis.FormatVersion != 1 || genesis.Purpose != genesisPurpose ||
			genesis.InstallationID != anchor.InstallationID || genesis.AuthorityKeyID != anchor.AuthorityKeyID ||
			genesis.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(anchor.AuthorityPublicKey) ||
			genesis.EnrollmentID != anchor.EnrollmentID || genesis.GenesisCheckpoint != anchor.GenesisCheckpoint ||
			len(genesis.WitnessSeed) != ed25519.SeedSize {
			clear(genesis.WitnessSeed)
			return errors.New("shared witness owner genesis does not match enrollment")
		}
		owner.private = ed25519.NewKeyFromSeed(genesis.WitnessSeed)
		clear(genesis.WitnessSeed)
		if !bytes.Equal(owner.private.Public().(ed25519.PublicKey), mustWitnessPublic(anchor.GenesisCheckpoint)) {
			return errors.New("shared witness owner signing key does not match enrollment")
		}
		owner.checkpoint = anchor.GenesisCheckpoint
		sort.Slice(generations, func(i, j int) bool { return generations[i] < generations[j] })
		for index, generation := range generations {
			if generation != uint64(index+1) {
				return errors.New("shared witness owner journal has a gap")
			}
			_, body, present, readErr := stateDir.CaptureFile(ctx, journalName(generation), maxRecordBytes, false, true)
			if readErr != nil || !present {
				return errors.New("shared witness owner journal cannot be read")
			}
			var record journalRecord
			parseErr := strictRecord(body, &record)
			if parseErr != nil {
				clear(body)
				return parseErr
			}
			if record.FormatVersion != 1 || record.Purpose != journalPurpose ||
				record.PreviousRecordDigest != owner.lastRecordDigest ||
				record.AdvanceRequest.NextGeneration != generation ||
				record.NextCheckpoint != record.AdvanceReceipt.Checkpoint ||
				domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(owner.checkpoint,
					record.AdvanceRequest, record.AdvanceReceipt, anchor.InstallationID,
					anchor.AuthorityKeyID, anchor.AuthorityPublicKey, anchor.EnrollmentID,
					anchor.GenesisCheckpoint.WitnessKeyID, mustWitnessPublic(anchor.GenesisCheckpoint)) != nil {
				clear(body)
				return errors.New("shared witness owner journal chain is invalid")
			}
			if _, duplicate := owner.committed[record.AdvanceRequest.MutationID]; duplicate {
				clear(body)
				return errors.New("shared witness owner journal repeats a mutation ID")
			}
			owner.lastRecordDigest = domainsecurity.SHA256Hex(body)
			clear(body)
			owner.checkpoint = record.NextCheckpoint
			owner.committed[record.AdvanceRequest.MutationID] = committedMutation{record.AdvanceRequest, record.AdvanceReceipt}
		}
		// An unpublished atomic-write temp cannot be a committed generation.
		// Validate the entire temp inventory before deleting any residue. An
		// unexpected later entry must leave the original evidence untouched.
		verifiedTemps := make(map[string]persistencefs.SeparateOwnerFileState, len(temps))
		for _, name := range temps {
			if !persistencefs.IsSeparateOwnerAtomicTempName(journalName(owner.checkpoint.Generation+1), name) {
				return errors.New("shared witness owner has an unexpected temporary file")
			}
			state, _, present, err := stateDir.CaptureFile(ctx, name, maxRecordBytes, true, true)
			if err != nil || !present {
				return errors.New("shared witness owner temporary file is unsafe")
			}
			verifiedTemps[name] = state
		}
		if err := stateDir.RemoveKnownTempFiles(ctx, verifiedTemps); err != nil {
			return err
		}
		// A record may have been published before the writer could fsync its
		// directory. Recovery must make every accepted directory entry durable
		// before this owner can sign or return the recovered head.
		if syncDirectory == nil {
			syncDirectory = (*persistencefs.SeparateOwnerDirectory).Sync
		}
		if err := syncDirectory(stateDir); err != nil {
			return err
		}
		owner.stateDirectory = stateDir.State()
		if !owner.stateDirectory.Valid() || !owner.stateDirectory.Protected {
			return errors.New("shared witness owner state directory changed during recovery")
		}
		return nil
	})
	if err != nil {
		clear(owner.private)
		return nil, err
	}
	return owner, nil
}

func (owner *Owner) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(owner.private, message), nil
}

func (owner *Owner) liveLocked() bool {
	return owner != nil && !owner.closed && !owner.poisoned && owner.lease != nil &&
		owner.stateDirectory.Valid() && owner.stateDirectory.Protected && len(owner.private) == ed25519.PrivateKeySize
}

func (owner *Owner) withStateAccess(ctx context.Context, access func(*persistencefs.SeparateOwnerDirectory) error) error {
	return owner.lease.WithRootAccess(ctx, func(rootAccess *persistencefs.SeparateOwnerRootAccess) error {
		rootDir, err := rootAccess.Directory()
		if err != nil {
			return err
		}
		stateDir, present, err := rootDir.OpenDirectory(ctx, stateDirectoryName, true)
		if err != nil || !present {
			return errors.New("shared witness owner state directory is unavailable")
		}
		defer stateDir.Close()
		if stateDir.State() != owner.stateDirectory {
			return errors.New("shared witness owner state directory identity changed")
		}
		var accessErr error
		if access != nil {
			accessErr = access(stateDir)
		}
		if err := stateDir.Validate(); err != nil {
			return errors.New("shared witness owner state directory changed during access")
		}
		return accessErr
	})
}

func (owner *Owner) Observe(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1) (domainsecurity.MonotonicHeadObservationV1, error) {
	if owner == nil {
		return domainsecurity.MonotonicHeadObservationV1{}, monotonichead.ErrUnavailable
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if !owner.liveLocked() {
		return domainsecurity.MonotonicHeadObservationV1{}, monotonichead.ErrUnavailable
	}
	if domainsecurity.ValidateMonotonicHeadObserveRequestForInstallationV1(request, owner.anchor.InstallationID,
		owner.anchor.AuthorityKeyID, owner.anchor.AuthorityPublicKey) != nil ||
		request.EnrollmentID != owner.anchor.EnrollmentID || request.Namespace != domainenrollment.SharedEvidenceNamespaceV1 {
		return domainsecurity.MonotonicHeadObservationV1{}, monotonichead.ErrInvalidReceipt
	}
	var observation domainsecurity.MonotonicHeadObservationV1
	if err := owner.withStateAccess(ctx, func(*persistencefs.SeparateOwnerDirectory) error {
		var signErr error
		observation, signErr = domainsecurity.NewMonotonicHeadObservationV1(request, owner.checkpoint, owner.sign)
		return signErr
	}); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, errors.Join(monotonichead.ErrUnavailable, err)
	}
	return observation, nil
}

func (owner *Owner) Advance(ctx context.Context, request domainsecurity.MonotonicHeadAdvanceRequestV1) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	if owner == nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrUnavailable
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if !owner.liveLocked() {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrUnavailable
	}
	if domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(request, owner.anchor.InstallationID,
		owner.anchor.AuthorityKeyID, owner.anchor.AuthorityPublicKey) != nil ||
		request.EnrollmentID != owner.anchor.EnrollmentID || request.Namespace != domainenrollment.SharedEvidenceNamespaceV1 {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrInvalidReceipt
	}
	var replay *domainsecurity.MonotonicHeadAdvanceReceiptV1
	if err := owner.withStateAccess(ctx, func(*persistencefs.SeparateOwnerDirectory) error {
		if stored, exists := owner.committed[request.MutationID]; exists {
			if stored.request != request {
				return monotonichead.ErrMutationConflict
			}
			original := stored.receipt
			replay = &original
		}
		return nil
	}); err != nil {
		if errors.Is(err, monotonichead.ErrMutationConflict) {
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrMutationConflict
		}
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.Join(monotonichead.ErrUnavailable, err)
	}
	if replay != nil {
		return *replay, nil
	}
	current := owner.checkpoint
	if current.Generation >= maxJournalFiles {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrUnavailable
	}
	if request.ExpectedGeneration != current.Generation || request.ExpectedCheckpointDigest != current.CheckpointDigest ||
		request.ExpectedStateDigest != current.CurrentStateDigest || request.ExpectedFenceNonce != current.FenceNonce ||
		request.NextGeneration != current.Generation+1 {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrCASConflict
	}
	fenceBytes := make([]byte, 32)
	if _, err := rand.Read(fenceBytes); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.Join(monotonichead.ErrUnavailable, err)
	}
	fence := domainsecurity.SHA256Hex(fenceBytes)
	clear(fenceBytes)
	if fence == current.FenceNonce {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonichead.ErrUnavailable
	}
	next, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: owner.anchor.InstallationID, EnrollmentID: owner.anchor.EnrollmentID,
		Namespace: domainenrollment.SharedEvidenceNamespaceV1, Generation: request.NextGeneration,
		CurrentStateDigest: request.NextStateDigest, PreviousStateDigest: current.CurrentStateDigest,
		PreviousCheckpointDigest: current.CheckpointDigest, FenceNonce: fence, MutationID: request.MutationID,
		WitnessKeyID: current.WitnessKeyID, WitnessPublicKey: mustWitnessPublic(current),
	}, owner.sign)
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.Join(monotonichead.ErrUnavailable, err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, next, owner.sign)
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.Join(monotonichead.ErrUnavailable, err)
	}
	record := journalRecord{1, journalPurpose, owner.lastRecordDigest, request, receipt, next}
	body, err := json.Marshal(record)
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.Join(monotonichead.ErrUnavailable, err)
	}
	defer clear(body)
	err = owner.withStateAccess(ctx, func(stateDir *persistencefs.SeparateOwnerDirectory) error {
		return stateDir.WriteExclusiveAtomic(ctx, journalName(next.Generation), body, maxRecordBytes, owner.writeFault)
	})
	if err != nil {
		// The target may already be published. No stale Observe or second Advance
		// is served until strict restart recovery determines the outcome.
		owner.poisoned = true
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, errors.Join(monotonichead.ErrIndeterminate, err)
	}
	owner.lastRecordDigest = domainsecurity.SHA256Hex(body)
	owner.checkpoint = next
	owner.committed[request.MutationID] = committedMutation{request, receipt}
	return receipt, nil
}

func (owner *Owner) ResolveMutation(ctx context.Context, request domainsecurity.MonotonicHeadMutationResolveRequestV1) (domainsecurity.MonotonicHeadMutationResolutionV1, error) {
	if owner == nil {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonichead.ErrUnavailable
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if !owner.liveLocked() {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonichead.ErrUnavailable
	}
	if domainsecurity.ValidateMonotonicHeadMutationResolveRequestForInstallationV1(request, owner.anchor.InstallationID,
		owner.anchor.AuthorityKeyID, owner.anchor.AuthorityPublicKey) != nil ||
		request.AdvanceRequest.EnrollmentID != owner.anchor.EnrollmentID || request.AdvanceRequest.Namespace != domainenrollment.SharedEvidenceNamespaceV1 {
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonichead.ErrInvalidReceipt
	}
	var resolution domainsecurity.MonotonicHeadMutationResolutionV1
	if err := owner.withStateAccess(ctx, func(*persistencefs.SeparateOwnerDirectory) error {
		var committed *domainsecurity.MonotonicHeadCommittedMutationV1
		if stored, exists := owner.committed[request.AdvanceRequest.MutationID]; exists {
			if stored.request != request.AdvanceRequest {
				return monotonichead.ErrMutationConflict
			}
			committed = &domainsecurity.MonotonicHeadCommittedMutationV1{AdvanceRequest: stored.request, AdvanceReceipt: stored.receipt}
		}
		var signErr error
		resolution, signErr = domainsecurity.NewMonotonicHeadMutationResolutionV1(request, owner.checkpoint, committed, owner.sign)
		return signErr
	}); err != nil {
		if errors.Is(err, monotonichead.ErrMutationConflict) {
			return domainsecurity.MonotonicHeadMutationResolutionV1{}, monotonichead.ErrMutationConflict
		}
		return domainsecurity.MonotonicHeadMutationResolutionV1{}, errors.Join(monotonichead.ErrUnavailable, err)
	}
	return resolution, nil
}

func (owner *Owner) Close() error {
	if owner == nil {
		return nil
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.closed && owner.lease == nil {
		return nil
	}
	owner.closed = true
	clear(owner.private)
	if owner.lease == nil {
		return nil
	}
	err := owner.lease.Close()
	if err == nil {
		owner.lease = nil
	}
	return err
}
