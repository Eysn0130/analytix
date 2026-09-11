//go:build !analytix_prod

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
	threadriskauthorityport "analytix.local/runtime-go/internal/ports/threadriskauthority"
	threadriskpolicyport "analytix.local/runtime-go/internal/ports/threadriskpolicy"
)

// This witness exists only for contract and test runtimes selected by an
// explicit DurableTempDir. It is intentionally process-local and therefore is
// not a production freshness authority. The analytix_prod implementation
// below never enables a local fallback.
const contractRiskWitnessSeedDomain = "analytix.contract-thread-risk-witness/nonprod/v1"

var (
	errContractRiskRecordAbsent   = errors.New("contract thread risk authority record is absent")
	errContractRiskRecordConflict = errors.New("contract thread risk authority record conflicts with its content address")
	errContractRiskScanForbidden  = errors.New("contract thread risk authority scan is forbidden")
)

func newContractThreadRiskAuthority(
	config Config,
	installationAuthority finalauthorityport.Authority,
) (turnsecurityapp.RiskPolicyAuthority, bool, error) {
	if strings.TrimSpace(config.DurableTempDir) == "" ||
		strings.TrimSpace(config.ProductionDurableRoot) != "" ||
		strings.TrimSpace(config.CandidateDurableRoot) != "" {
		return nil, false, nil
	}
	if installationAuthority == nil {
		return nil, true, errors.New("contract thread risk authority requires an installation authority")
	}
	authorityPublicKey := append([]byte(nil), installationAuthority.PublicKey()...)
	if len(authorityPublicKey) != ed25519.PublicKeySize || installationAuthority.KeyID() != domainsecurity.SHA256Hex(authorityPublicKey) {
		return nil, true, errors.New("contract thread risk installation authority is invalid")
	}

	seed := sha256.Sum256([]byte(contractRiskWitnessSeedDomain))
	witnessPrivateKey := ed25519.NewKeyFromSeed(seed[:])
	witnessPublicKey := append(ed25519.PublicKey(nil), witnessPrivateKey.Public().(ed25519.PublicKey)...)
	installationID := domainsecurity.SHA256Hex(bytes.Join([][]byte{
		[]byte(contractRiskWitnessSeedDomain + "/installation\x00"),
		[]byte(installationAuthority.KeyID()),
		authorityPublicKey,
	}, nil))
	enrollmentID := domainsecurity.SHA256Hex(bytes.Join([][]byte{
		[]byte(contractRiskWitnessSeedDomain + "/enrollment\x00"),
		[]byte(installationID),
		witnessPublicKey,
	}, nil))
	witness, err := newContractMonotonicWitness(
		installationID,
		enrollmentID,
		installationAuthority.KeyID(),
		authorityPublicKey,
		witnessPrivateKey,
	)
	if err != nil {
		return nil, true, err
	}

	authority, err := threadriskauthorityapp.New(threadriskauthorityapp.Config{
		InstallationID:   installationID,
		EnrollmentID:     enrollmentID,
		Namespace:        domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Authority:        installationAuthority,
		WitnessKeyID:     domainsecurity.SHA256Hex(witnessPublicKey),
		WitnessPublicKey: witnessPublicKey,
		Random:           cryptorand.Reader,
		Witness:          witness,
		CheckpointFloor:  &contractCheckpointFloor{initial: witness.checkpoint},
		Indexes:          newContractRiskIndexStore(),
		Observations:     newContractRiskObservationStore(),
		Projection:       &contractRiskProjection{},
		Policies:         newContractRiskPolicyStore(),
	})
	if err != nil {
		return nil, true, err
	}
	return authority, true, nil
}

type contractCheckpointFloor struct {
	mu         sync.Mutex
	initial    domainsecurity.MonotonicHeadCheckpointV1
	checkpoint *domainsecurity.MonotonicHeadCheckpointV1
}

func (floor *contractCheckpointFloor) ProjectWitnessSelected(ctx context.Context, selected domainsecurity.MonotonicHeadCheckpointV1) error {
	if err := contractRiskContextError(ctx); err != nil {
		return err
	}
	floor.mu.Lock()
	defer floor.mu.Unlock()
	if floor.checkpoint == nil {
		if selected != floor.initial {
			return monotonicheadport.ErrCheckpointFloorBootstrap
		}
		copy := selected
		floor.checkpoint = &copy
		return nil
	}
	if *floor.checkpoint == selected {
		return nil
	}
	if domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(*floor.checkpoint, selected) != nil {
		return monotonicheadport.ErrCheckpointFloorConflict
	}
	copy := selected
	floor.checkpoint = &copy
	return nil
}

// contractMonotonicWitness stores only the monotonic checkpoint and exact
// mutation request/receipt contracts. In particular, it never receives or
// stores thread IDs, workspace paths, case bindings, source data, or PII.
type contractMonotonicWitness struct {
	mu                 sync.Mutex
	installationID     string
	enrollmentID       string
	authorityKeyID     string
	authorityPublicKey []byte
	witnessPrivateKey  ed25519.PrivateKey
	witnessPublicKey   ed25519.PublicKey
	checkpoint         domainsecurity.MonotonicHeadCheckpointV1
	mutations          map[string]contractRiskMutation
}

type contractRiskMutation struct {
	request domainsecurity.MonotonicHeadAdvanceRequestV1
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1
}

func newContractMonotonicWitness(
	installationID string,
	enrollmentID string,
	authorityKeyID string,
	authorityPublicKey []byte,
	witnessPrivateKey ed25519.PrivateKey,
) (*contractMonotonicWitness, error) {
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	witnessPrivateKey = append(ed25519.PrivateKey(nil), witnessPrivateKey...)
	if !domainsecurity.IsSHA256Hex(installationID) || !domainsecurity.IsSHA256Hex(enrollmentID) ||
		len(authorityPublicKey) != ed25519.PublicKeySize || authorityKeyID != domainsecurity.SHA256Hex(authorityPublicKey) ||
		len(witnessPrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("contract monotonic witness enrollment is invalid")
	}
	witnessPublicKey := append(ed25519.PublicKey(nil), witnessPrivateKey.Public().(ed25519.PublicKey)...)
	witness := &contractMonotonicWitness{
		installationID:     installationID,
		enrollmentID:       enrollmentID,
		authorityKeyID:     authorityKeyID,
		authorityPublicKey: authorityPublicKey,
		witnessPrivateKey:  witnessPrivateKey,
		witnessPublicKey:   witnessPublicKey,
		mutations:          make(map[string]contractRiskMutation),
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:     installationID,
		EnrollmentID:       enrollmentID,
		Namespace:          domainsecurity.ThreadRiskAuthorityNamespaceV1,
		Generation:         0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte(contractRiskWitnessSeedDomain + "/empty-state\x00" + installationID + enrollmentID)),
		FenceNonce:         domainsecurity.SHA256Hex([]byte(contractRiskWitnessSeedDomain + "/enrollment-fence\x00" + installationID + enrollmentID)),
		WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublicKey),
		WitnessPublicKey:   witnessPublicKey,
	}, witness.sign)
	if err != nil {
		return nil, err
	}
	witness.checkpoint = checkpoint
	return witness, nil
}

func (witness *contractMonotonicWitness) Observe(
	ctx context.Context,
	request domainsecurity.MonotonicHeadObserveRequestV1,
) (domainsecurity.MonotonicHeadObservationV1, error) {
	if err := contractRiskContextError(ctx); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	if err := domainsecurity.ValidateMonotonicHeadObserveRequestForInstallationV1(
		request, witness.installationID, witness.authorityKeyID, witness.authorityPublicKey,
	); err != nil || request.EnrollmentID != witness.enrollmentID || request.Namespace != domainsecurity.ThreadRiskAuthorityNamespaceV1 {
		return domainsecurity.MonotonicHeadObservationV1{}, monotonicheadport.ErrNotEnrolled
	}
	witness.mu.Lock()
	defer witness.mu.Unlock()
	if err := contractRiskContextError(ctx); err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, witness.checkpoint, witness.sign)
	if err != nil {
		return domainsecurity.MonotonicHeadObservationV1{}, monotonicheadport.ErrInvalidReceipt
	}
	return observation, nil
}

func (witness *contractMonotonicWitness) Advance(
	ctx context.Context,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
) (domainsecurity.MonotonicHeadAdvanceReceiptV1, error) {
	if err := contractRiskContextError(ctx); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(
		request, witness.installationID, witness.authorityKeyID, witness.authorityPublicKey,
	); err != nil || request.EnrollmentID != witness.enrollmentID || request.Namespace != domainsecurity.ThreadRiskAuthorityNamespaceV1 {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrNotEnrolled
	}
	requestBytes, err := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(request)
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrInvalidReceipt
	}

	witness.mu.Lock()
	defer witness.mu.Unlock()
	if err := contractRiskContextError(ctx); err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, err
	}
	if committed, ok := witness.mutations[request.MutationID]; ok {
		committedBytes, _ := domainsecurity.MonotonicHeadAdvanceRequestV1Bytes(committed.request)
		if !bytes.Equal(committedBytes, requestBytes) {
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrMutationConflict
		}
		if domainsecurity.ValidateMonotonicHeadIdempotentReplayV1(
			committed.request, committed.receipt, request, committed.receipt,
		) != nil {
			return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrInvalidReceipt
		}
		return committed.receipt, nil
	}

	previous := witness.checkpoint
	if request.ExpectedGeneration != previous.Generation || request.ExpectedCheckpointDigest != previous.CheckpointDigest ||
		request.ExpectedStateDigest != previous.CurrentStateDigest || request.ExpectedFenceNonce != previous.FenceNonce ||
		previous.Generation == ^uint64(0) || request.NextGeneration != previous.Generation+1 {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrCASConflict
	}
	next, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           previous.InstallationID,
		EnrollmentID:             previous.EnrollmentID,
		Namespace:                previous.Namespace,
		Generation:               request.NextGeneration,
		CurrentStateDigest:       request.NextStateDigest,
		PreviousStateDigest:      previous.CurrentStateDigest,
		PreviousCheckpointDigest: previous.CheckpointDigest,
		FenceNonce: domainsecurity.SHA256Hex(bytes.Join([][]byte{
			[]byte(contractRiskWitnessSeedDomain + "/fence\x00"),
			[]byte(previous.CheckpointDigest),
			[]byte(request.RequestDigest),
			[]byte(request.MutationID),
		}, nil)),
		MutationID:       request.MutationID,
		WitnessKeyID:     domainsecurity.SHA256Hex(witness.witnessPublicKey),
		WitnessPublicKey: witness.witnessPublicKey,
	}, witness.sign)
	if err != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrInvalidReceipt
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, next, witness.sign)
	if err != nil || domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
		previous, request, receipt,
		witness.installationID, witness.authorityKeyID, witness.authorityPublicKey,
		witness.enrollmentID, domainsecurity.SHA256Hex(witness.witnessPublicKey), witness.witnessPublicKey,
	) != nil {
		return domainsecurity.MonotonicHeadAdvanceReceiptV1{}, monotonicheadport.ErrInvalidReceipt
	}
	witness.checkpoint = next
	witness.mutations[request.MutationID] = contractRiskMutation{request: request, receipt: receipt}
	return receipt, nil
}

func (witness *contractMonotonicWitness) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(witness.witnessPrivateKey, message), nil
}

type contractRiskIndexStore struct {
	mu      sync.RWMutex
	records map[string]domainsecurity.ThreadRiskAuthorityIndexV1
}

func newContractRiskIndexStore() *contractRiskIndexStore {
	return &contractRiskIndexStore{records: make(map[string]domainsecurity.ThreadRiskAuthorityIndexV1)}
}

func (store *contractRiskIndexStore) PutIfAbsent(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if err := contractRiskContextError(ctx); err != nil {
		return err
	}
	if _, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(index); err != nil {
		return errContractRiskRecordConflict
	}
	index = cloneContractRiskIndex(index)
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, found := store.records[index.IndexDigest]; found {
		if !equalContractRiskIndexes(current, index) {
			return errContractRiskRecordConflict
		}
		return nil
	}
	store.records[index.IndexDigest] = index
	return nil
}

func (store *contractRiskIndexStore) Resolve(ctx context.Context, digest string) (domainsecurity.ThreadRiskAuthorityIndexV1, error) {
	if err := contractRiskContextError(ctx); err != nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	index, found := store.records[digest]
	if !found {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, errContractRiskRecordAbsent
	}
	return cloneContractRiskIndex(index), nil
}

type contractRiskObservationStore struct {
	mu      sync.RWMutex
	records map[string]threadriskauthorityport.ObservationBundle
}

func newContractRiskObservationStore() *contractRiskObservationStore {
	return &contractRiskObservationStore{records: make(map[string]threadriskauthorityport.ObservationBundle)}
}

func (store *contractRiskObservationStore) PutIfAbsent(ctx context.Context, bundle threadriskauthorityport.ObservationBundle) error {
	if err := contractRiskContextError(ctx); err != nil {
		return err
	}
	if domainsecurity.ValidateThreadRiskAuthorityIndexV1(bundle.Index) != nil ||
		domainsecurity.ValidateMonotonicHeadObserveRequestV1(bundle.Request) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationV1(bundle.Observation) != nil ||
		domainsecurity.ValidateThreadRiskAuthorityIndexCheckpointV1(bundle.Index, bundle.Observation.Checkpoint) != nil ||
		bundle.Observation.RequestDigest != bundle.Request.RequestDigest ||
		bundle.Observation.ChallengeNonce != bundle.Request.ChallengeNonce {
		return errContractRiskRecordConflict
	}
	bundle = cloneContractRiskObservationBundle(bundle)
	digest := bundle.Observation.ObservationDigest
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, found := store.records[digest]; found {
		if !equalContractRiskObservationBundles(current, bundle) {
			return errContractRiskRecordConflict
		}
		return nil
	}
	store.records[digest] = bundle
	return nil
}

func (store *contractRiskObservationStore) Resolve(ctx context.Context, digest string) (threadriskauthorityport.ObservationBundle, error) {
	if err := contractRiskContextError(ctx); err != nil {
		return threadriskauthorityport.ObservationBundle{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	bundle, found := store.records[digest]
	if !found {
		return threadriskauthorityport.ObservationBundle{}, errContractRiskRecordAbsent
	}
	return cloneContractRiskObservationBundle(bundle), nil
}

type contractRiskPolicyStore struct {
	mu      sync.RWMutex
	records map[string]domainsecurity.ThreadRiskPolicyV1
}

func newContractRiskPolicyStore() *contractRiskPolicyStore {
	return &contractRiskPolicyStore{records: make(map[string]domainsecurity.ThreadRiskPolicyV1)}
}

func (store *contractRiskPolicyStore) PutIfAbsent(ctx context.Context, policy domainsecurity.ThreadRiskPolicyV1) error {
	if err := contractRiskContextError(ctx); err != nil {
		return err
	}
	if _, err := domainsecurity.ThreadRiskPolicyV1Bytes(policy); err != nil {
		return errContractRiskRecordConflict
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, found := store.records[policy.PolicyDigest]; found {
		if !equalContractRiskPolicies(current, policy) {
			return errContractRiskRecordConflict
		}
		return nil
	}
	store.records[policy.PolicyDigest] = policy
	return nil
}

func (store *contractRiskPolicyStore) Resolve(ctx context.Context, digest string) (domainsecurity.ThreadRiskPolicyV1, error) {
	if err := contractRiskContextError(ctx); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	policy, found := store.records[digest]
	if !found {
		return domainsecurity.ThreadRiskPolicyV1{}, errContractRiskRecordAbsent
	}
	return policy, nil
}

func (*contractRiskPolicyStore) List(context.Context) ([]domainsecurity.ThreadRiskPolicyV1, error) {
	return nil, errContractRiskScanForbidden
}

func (*contractRiskPolicyStore) HasRecords(context.Context) (bool, error) {
	return false, errContractRiskScanForbidden
}

// contractRiskProjection deliberately retains only opaque digests and a
// generation. It has no read method and cannot select an authority head.
type contractRiskProjection struct {
	mu         sync.Mutex
	generation uint64
	index      string
	state      string
}

func (projection *contractRiskProjection) ValidateWitnessEmpty(ctx context.Context) error {
	if err := contractRiskContextError(ctx); err != nil {
		return err
	}
	projection.mu.Lock()
	defer projection.mu.Unlock()
	if projection.generation != 0 || projection.index != "" || projection.state != "" {
		return errContractRiskRecordConflict
	}
	return nil
}

func (projection *contractRiskProjection) ProjectWitnessSelected(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if err := contractRiskContextError(ctx); err != nil {
		return err
	}
	if domainsecurity.ValidateThreadRiskAuthorityIndexV1(index) != nil {
		return errContractRiskRecordConflict
	}
	projection.mu.Lock()
	projection.generation = index.Generation
	projection.index = index.IndexDigest
	projection.state = index.StateDigest
	projection.mu.Unlock()
	return nil
}

func contractRiskContextError(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}

func cloneContractRiskIndex(index domainsecurity.ThreadRiskAuthorityIndexV1) domainsecurity.ThreadRiskAuthorityIndexV1 {
	index.Entries = append([]domainsecurity.ThreadRiskAuthorityEntryV1(nil), index.Entries...)
	return index
}

func cloneContractRiskObservationBundle(bundle threadriskauthorityport.ObservationBundle) threadriskauthorityport.ObservationBundle {
	bundle.Index = cloneContractRiskIndex(bundle.Index)
	return bundle
}

func equalContractRiskIndexes(left, right domainsecurity.ThreadRiskAuthorityIndexV1) bool {
	leftBytes, leftErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(left)
	rightBytes, rightErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBytes, rightBytes)
}

func equalContractRiskPolicies(left, right domainsecurity.ThreadRiskPolicyV1) bool {
	leftBytes, leftErr := domainsecurity.ThreadRiskPolicyV1Bytes(left)
	rightBytes, rightErr := domainsecurity.ThreadRiskPolicyV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBytes, rightBytes)
}

func equalContractRiskObservationBundles(left, right threadriskauthorityport.ObservationBundle) bool {
	if !equalContractRiskIndexes(left.Index, right.Index) {
		return false
	}
	leftRequest, leftRequestErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(left.Request)
	rightRequest, rightRequestErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(right.Request)
	leftObservation, leftObservationErr := domainsecurity.MonotonicHeadObservationV1Bytes(left.Observation)
	rightObservation, rightObservationErr := domainsecurity.MonotonicHeadObservationV1Bytes(right.Observation)
	return leftRequestErr == nil && rightRequestErr == nil && leftObservationErr == nil && rightObservationErr == nil &&
		bytes.Equal(leftRequest, rightRequest) && bytes.Equal(leftObservation, rightObservation)
}

var (
	_ monotonicheadport.Witness                = (*contractMonotonicWitness)(nil)
	_ threadriskauthorityport.IndexStore       = (*contractRiskIndexStore)(nil)
	_ threadriskauthorityport.ObservationStore = (*contractRiskObservationStore)(nil)
	_ threadriskauthorityport.Projection       = (*contractRiskProjection)(nil)
	_ threadriskpolicyport.Store               = (*contractRiskPolicyStore)(nil)
)
