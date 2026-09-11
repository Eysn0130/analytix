package evidenceauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

const maxBundleChainDepth = 100_000
const evidenceAuthorityNonceReplayWindow = 4_096

var (
	ErrInvalidInput        = errors.New("witnessed evidence authority input is invalid")
	ErrUnavailable         = errors.New("witnessed evidence authority is unavailable")
	ErrIntegrity           = errors.New("witnessed evidence authority integrity failure")
	ErrNotInitialized      = errors.New("witnessed evidence authority is not initialized")
	ErrAlreadyInitialized  = errors.New("witnessed evidence authority is already initialized")
	ErrCurrentChanged      = errors.New("witnessed evidence authority head changed")
	ErrAdvanceNotCommitted = errors.New("witnessed evidence authority advance was not committed")
)

type Child string

const (
	ChildDatasetSnapshot  Child = "dataset_snapshot"
	ChildEvidenceRegistry Child = "evidence_registry"
	ChildPublication      Child = "publication"
)

type Genesis struct {
	DatasetSnapshotIndexDigest  string
	EvidenceRegistryIndexDigest string
	PublicationIndexDigest      string
}

type Config struct {
	InstallationID   string
	EnrollmentID     string
	Authority        finalauthorityport.Authority
	WitnessKeyID     string
	WitnessPublicKey []byte
	Random           io.Reader
	Witness          monotonicheadport.Witness
	CheckpointFloor  monotonicheadport.CheckpointFloor
	Bundles          storeport.BundleStore
	Observations     storeport.ObservationStore
	Projection       storeport.Projection
	Genesis          Genesis
}

// AdvanceChildInput binds an append to the exact bundle the caller inspected.
// This is mandatory in addition to the application's fresh Observe so two
// concurrent callers starting from one head cannot both succeed serially.
type AdvanceChildInput struct {
	ExpectedBundleDigest string
	Child                Child
	NextIndexDigest      string
}

// Head is an exact result of a current-run witness challenge. Generation zero
// is represented only by HasBundle=false; no local record is consulted.
type Head struct {
	HasBundle   bool
	Bundle      domainevidence.EvidenceAuthorityBundleV1
	Request     domainsecurity.MonotonicHeadObserveRequestV1
	Observation domainsecurity.MonotonicHeadObservationV1
}

// Authority never infers current from bundle, observation, or projection
// storage. The process mutex prevents accidental local duplicate dispatch;
// the independently enrolled witness remains cross-process CAS authority.
type Authority struct {
	installationID  string
	enrollmentID    string
	authority       finalauthorityport.Authority
	authorityKeyID  string
	authorityKey    []byte
	witnessKeyID    string
	witnessKey      []byte
	random          io.Reader
	witness         monotonicheadport.Witness
	checkpointFloor monotonicheadport.CheckpointFloor
	bundles         storeport.BundleStore
	observations    storeport.ObservationStore
	projection      storeport.Projection
	genesis         Genesis

	mu             sync.Mutex
	usedNonces     map[string]struct{}
	nonceOrder     []string
	nonceCursor    int
	lastCheckpoint *domainsecurity.MonotonicHeadCheckpointV1
	lastBundle     *domainevidence.EvidenceAuthorityBundleV1
}

type boundFreshHeadChallengeV1 struct {
	mu        sync.Mutex
	condition *sync.Cond
	active    bool
	inFlight  bool
	authority *Authority
	expected  domainevidence.EvidenceAuthorityBundleV1
}

var _ storeport.DatasetCoordinator = (*Authority)(nil)
var _ storeport.RegistryCoordinator = (*Authority)(nil)
var _ storeport.PublicationCoordinator = (*Authority)(nil)
var _ storeport.WitnessBindingChainResolver = (*Authority)(nil)

func New(config Config) (*Authority, error) {
	if config.Random == nil {
		config.Random = cryptorand.Reader
	}
	publicKey := []byte(nil)
	keyID := ""
	if config.Authority != nil {
		publicKey = append([]byte(nil), config.Authority.PublicKey()...)
		keyID = config.Authority.KeyID()
	}
	witnessKey := append([]byte(nil), config.WitnessPublicKey...)
	if !canonicalDigest(config.InstallationID) || !canonicalDigest(config.EnrollmentID) ||
		config.Authority == nil || len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		!canonicalDigest(config.WitnessKeyID) || len(witnessKey) != ed25519.PublicKeySize || config.WitnessKeyID != domainsecurity.SHA256Hex(witnessKey) ||
		config.Random == nil || config.Witness == nil || config.CheckpointFloor == nil || config.Bundles == nil || config.Observations == nil || config.Projection == nil ||
		config.Genesis.DatasetSnapshotIndexDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() ||
		config.Genesis.EvidenceRegistryIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() ||
		config.Genesis.PublicationIndexDigest != domainpublication.PublicationIndexGenesisDigestV1() {
		return nil, ErrInvalidInput
	}
	return &Authority{
		installationID:  config.InstallationID,
		enrollmentID:    config.EnrollmentID,
		authority:       config.Authority,
		authorityKeyID:  keyID,
		authorityKey:    publicKey,
		witnessKeyID:    config.WitnessKeyID,
		witnessKey:      witnessKey,
		random:          config.Random,
		witness:         config.Witness,
		checkpointFloor: config.CheckpointFloor,
		bundles:         config.Bundles,
		observations:    config.Observations,
		projection:      config.Projection,
		genesis:         config.Genesis,
		usedNonces:      make(map[string]struct{}),
		nonceOrder:      make([]string, 0, evidenceAuthorityNonceReplayWindow),
	}, nil
}

// Current always performs a fresh random challenge to the enrolled witness.
func (authority *Authority) Current(ctx context.Context) (Head, error) {
	if authority == nil || ctx == nil {
		return Head{}, ErrInvalidInput
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.observeLocked(ctx)
}

// ObserveFresh implements the narrow shared-authority port without exposing a
// local selector or allowing the caller to choose another child namespace.
func (authority *Authority) ObserveFresh(ctx context.Context) (storeport.FreshHead, error) {
	head, err := authority.Current(ctx)
	return freshHead(head), err
}

// WithFreshHeadChallenge supplies one process-local challenge closure bound to
// the exact bundle selected by a complete fresh observation. The closure is
// valid only during callback and accepts no caller-selected namespace, child,
// head, or digest. Its repeated inner challenges retain witness and checkpoint
// validation while deliberately avoiding a second bundle/observation store
// traversal or durable projection.
func (authority *Authority) WithFreshHeadChallenge(
	ctx context.Context,
	use func(storeport.FreshHead, func(context.Context) error) error,
) error {
	if authority == nil || ctx == nil || use == nil {
		return ErrInvalidInput
	}
	authority.mu.Lock()
	head, err := authority.observeLocked(ctx)
	authority.mu.Unlock()
	if err != nil {
		return err
	}
	if !head.HasBundle {
		return ErrNotInitialized
	}
	challenge := &boundFreshHeadChallengeV1{
		active: true, authority: authority, expected: head.Bundle,
	}
	challenge.condition = sync.NewCond(&challenge.mu)
	defer challenge.close()
	return use(freshHead(head), challenge.use)
}

func (challenge *boundFreshHeadChallengeV1) use(ctx context.Context) error {
	if challenge == nil || ctx == nil {
		return ErrInvalidInput
	}
	challenge.mu.Lock()
	if !challenge.active || challenge.inFlight || challenge.authority == nil {
		challenge.mu.Unlock()
		return ErrUnavailable
	}
	challenge.inFlight = true
	authority := challenge.authority
	expected := challenge.expected
	challenge.mu.Unlock()
	defer func() {
		challenge.mu.Lock()
		challenge.inFlight = false
		if challenge.condition != nil {
			challenge.condition.Broadcast()
		}
		challenge.mu.Unlock()
	}()

	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.challengeExpectedBundleLocked(ctx, expected)
}

func (challenge *boundFreshHeadChallengeV1) close() {
	if challenge == nil {
		return
	}
	challenge.mu.Lock()
	challenge.active = false
	for challenge.inFlight {
		challenge.condition.Wait()
	}
	challenge.authority = nil
	challenge.expected = domainevidence.EvidenceAuthorityBundleV1{}
	challenge.mu.Unlock()
}

func (authority *Authority) challengeExpectedBundleLocked(
	ctx context.Context,
	expected domainevidence.EvidenceAuthorityBundleV1,
) error {
	_, observation, err := authority.freshObservationLocked(ctx)
	if err != nil {
		return err
	}
	if authority.verifyBundle(ctx, expected) != nil {
		return ErrIntegrity
	}
	if authority.lastBundle == nil || !equalBundleRecord(*authority.lastBundle, expected) ||
		observation.Checkpoint.CurrentStateDigest != expected.RecordDigest ||
		domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(expected, observation.Checkpoint) != nil {
		return ErrCurrentChanged
	}
	return nil
}

// ResolveWitnessBindingOnFreshChain never treats the persisted historical
// exchange as freshness authority. It first performs a new random witness
// challenge, then accepts the historical exchange only when every trust
// anchor matches and its signed bundle is the selected head or an exact
// ancestor of that freshly witnessed head.
func (authority *Authority) ResolveWitnessBindingOnFreshChain(
	ctx context.Context,
	binding domainevidence.EvidenceAuthorityWitnessBindingV1,
) (storeport.WitnessBindingChainResolution, error) {
	if authority == nil || ctx == nil || domainevidence.ValidateEvidenceAuthorityWitnessBindingV1(binding) != nil {
		return storeport.WitnessBindingChainResolution{}, ErrInvalidInput
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return storeport.WitnessBindingChainResolution{}, err
	}
	current, err := authority.observeLocked(ctx)
	if err != nil {
		return storeport.WitnessBindingChainResolution{}, err
	}
	if !current.HasBundle {
		return storeport.WitnessBindingChainResolution{}, ErrNotInitialized
	}
	historical, err := authority.observations.Resolve(ctx, binding.ObservationDigest)
	if err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return storeport.WitnessBindingChainResolution{}, contextErr
		}
		return storeport.WitnessBindingChainResolution{}, ErrIntegrity
	}
	if authority.validateObservationBundle(ctx, historical) != nil ||
		domainevidence.ValidateEvidenceAuthorityWitnessBindingExactV1(
			binding, historical.Bundle, historical.Request, historical.Observation,
			authority.installationID, authority.enrollmentID, authority.authorityKeyID, authority.authorityKey,
			authority.witnessKeyID, authority.witnessKey,
		) != nil || !authority.bundleIsAncestor(ctx, current.Bundle, historical.Bundle) {
		return storeport.WitnessBindingChainResolution{}, ErrIntegrity
	}
	return storeport.WitnessBindingChainResolution{Current: freshHead(current), Historical: historical}, nil
}

// AdvanceDatasetSnapshot advances only the dataset child from the exact
// freshly observed bundle named by the caller.
func (authority *Authority) AdvanceDatasetSnapshot(ctx context.Context, input storeport.DatasetAdvanceInput) (storeport.FreshHead, error) {
	head, err := authority.AdvanceChild(ctx, AdvanceChildInput{
		ExpectedBundleDigest: input.ExpectedBundleDigest,
		Child:                ChildDatasetSnapshot,
		NextIndexDigest:      input.NextIndexDigest,
	})
	return freshHead(head), err
}

// AdvanceEvidenceRegistry advances only the registry child from the exact
// freshly witnessed bundle named by the caller.
func (authority *Authority) AdvanceEvidenceRegistry(ctx context.Context, input storeport.RegistryAdvanceInput) (storeport.FreshHead, error) {
	head, err := authority.AdvanceChild(ctx, AdvanceChildInput{
		ExpectedBundleDigest: input.ExpectedBundleDigest,
		Child:                ChildEvidenceRegistry,
		NextIndexDigest:      input.NextIndexDigest,
	})
	return freshHead(head), err
}

// AdvancePublication advances only the publication child. The shared bundle
// CAS guarantees that a concurrent dataset or registry change invalidates the
// report's inspected parent rather than being silently serialized behind it.
func (authority *Authority) AdvancePublication(ctx context.Context, input storeport.PublicationAdvanceInput) (storeport.FreshHead, error) {
	head, err := authority.AdvanceChild(ctx, AdvanceChildInput{
		ExpectedBundleDigest: input.ExpectedBundleDigest,
		Child:                ChildPublication,
		NextIndexDigest:      input.NextIndexDigest,
	})
	return freshHead(head), err
}

func freshHead(head Head) storeport.FreshHead {
	return storeport.FreshHead{
		HasBundle: head.HasBundle, Bundle: head.Bundle, Request: head.Request, Observation: head.Observation,
	}
}

// Initialize creates the one canonical generation-one bundle. All three
// configured child roots start at count zero. A previously initialized witness
// is rejected rather than silently treating a local candidate as committed.
func (authority *Authority) Initialize(ctx context.Context) (Head, error) {
	if authority == nil || ctx == nil {
		return Head{}, ErrInvalidInput
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Head{}, err
	}
	previous, err := authority.observeLocked(ctx)
	if err != nil {
		return Head{}, err
	}
	if previous.HasBundle {
		return Head{}, ErrAlreadyInitialized
	}
	mutationID, err := authority.freshNonceLocked(ctx, "mutation")
	if err != nil {
		return Head{}, err
	}
	next, err := authority.newBundle(ctx, nil, mutationID, "", authority.genesis)
	if err != nil {
		return Head{}, err
	}
	return authority.dispatchLocked(ctx, previous, next, mutationID)
}

// AdvanceChild advances exactly one child digest and its count by one. The
// other two child pairs remain byte-exact to the freshly witnessed parent.
func (authority *Authority) AdvanceChild(ctx context.Context, input AdvanceChildInput) (Head, error) {
	if authority == nil || ctx == nil || !canonicalDigest(input.ExpectedBundleDigest) || !canonicalDigest(input.NextIndexDigest) || !validChild(input.Child) {
		return Head{}, ErrInvalidInput
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Head{}, err
	}
	previous, err := authority.observeLocked(ctx)
	if err != nil {
		return Head{}, err
	}
	if !previous.HasBundle {
		return Head{}, ErrNotInitialized
	}
	if previous.Bundle.RecordDigest != input.ExpectedBundleDigest {
		return Head{}, errors.Join(ErrCurrentChanged, monotonicheadport.ErrCASConflict)
	}
	if previous.Bundle.Generation == ^uint64(0) {
		return Head{}, ErrIntegrity
	}
	nextRoots := Genesis{
		DatasetSnapshotIndexDigest:  previous.Bundle.DatasetSnapshotIndexDigest,
		EvidenceRegistryIndexDigest: previous.Bundle.EvidenceRegistryIndexDigest,
		PublicationIndexDigest:      previous.Bundle.PublicationIndexDigest,
	}
	switch input.Child {
	case ChildDatasetSnapshot:
		if previous.Bundle.DatasetSnapshotCount == ^uint64(0) || input.NextIndexDigest == previous.Bundle.DatasetSnapshotIndexDigest {
			return Head{}, ErrInvalidInput
		}
		nextRoots.DatasetSnapshotIndexDigest = input.NextIndexDigest
	case ChildEvidenceRegistry:
		if previous.Bundle.EvidenceRegistryCount == ^uint64(0) || input.NextIndexDigest == previous.Bundle.EvidenceRegistryIndexDigest {
			return Head{}, ErrInvalidInput
		}
		nextRoots.EvidenceRegistryIndexDigest = input.NextIndexDigest
	case ChildPublication:
		if previous.Bundle.PublicationCount == ^uint64(0) || input.NextIndexDigest == previous.Bundle.PublicationIndexDigest {
			return Head{}, ErrInvalidInput
		}
		nextRoots.PublicationIndexDigest = input.NextIndexDigest
	}
	mutationID, err := authority.freshNonceLocked(ctx, "mutation")
	if err != nil {
		return Head{}, err
	}
	next, err := authority.newBundle(ctx, &previous.Bundle, mutationID, string(input.Child), nextRoots)
	if err != nil {
		return Head{}, err
	}
	return authority.dispatchLocked(ctx, previous, next, mutationID)
}

func (authority *Authority) dispatchLocked(ctx context.Context, previous Head, next domainevidence.EvidenceAuthorityBundleV1, mutationID string) (Head, error) {
	// The immutable candidate must survive before any witness dispatch. Its
	// mere presence is never authority because no port can enumerate it.
	if err := authority.persistBundleExact(ctx, next); err != nil {
		return Head{}, err
	}
	request, err := authority.newAdvanceRequest(ctx, previous.Observation.Checkpoint, next, mutationID)
	if err != nil {
		return Head{}, err
	}
	receipt, advanceErr := authority.witness.Advance(ctx, request)
	if advanceErr != nil {
		if errors.Is(advanceErr, monotonicheadport.ErrIndeterminate) {
			return authority.reconcileIndeterminateLocked(ctx, previous, next)
		}
		if errors.Is(advanceErr, context.Canceled) || errors.Is(advanceErr, context.DeadlineExceeded) ||
			errors.Is(advanceErr, monotonicheadport.ErrCASConflict) || errors.Is(advanceErr, monotonicheadport.ErrMutationConflict) ||
			errors.Is(advanceErr, monotonicheadport.ErrUnavailable) || errors.Is(advanceErr, monotonicheadport.ErrEquivocation) {
			return Head{}, advanceErr
		}
		return Head{}, ErrUnavailable
	}
	if err := authority.validateAdvance(previous, next, request, receipt); err != nil {
		return Head{}, err
	}
	committed, err := authority.observeLocked(ctx)
	if err != nil {
		return Head{}, err
	}
	if !committed.HasBundle || !authority.bundleIsAncestor(ctx, committed.Bundle, next) {
		if contextErr := contextError(ctx); contextErr != nil {
			return Head{}, contextErr
		}
		return Head{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
	}
	return committed, nil
}

func (authority *Authority) observeLocked(ctx context.Context) (Head, error) {
	request, observation, err := authority.freshObservationLocked(ctx)
	if err != nil {
		return Head{}, err
	}
	return authority.headFromObservationLocked(ctx, request, observation)
}

func (authority *Authority) freshObservationLocked(ctx context.Context) (domainsecurity.MonotonicHeadObserveRequestV1, domainsecurity.MonotonicHeadObservationV1, error) {
	if err := contextError(ctx); err != nil {
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, err
	}
	challenge, err := authority.freshNonceLocked(ctx, "observe")
	if err != nil {
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, err
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: authority.installationID, EnrollmentID: authority.enrollmentID,
		Namespace: domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1, ChallengeNonce: challenge,
		AuthorityKeyID: authority.authorityKeyID, AuthorityPublicKey: authority.authorityKey,
	}, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil || authority.verifyObserveRequest(ctx, request) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, contextErr
		}
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, ErrIntegrity
	}
	observation, err := authority.witness.Observe(ctx, request)
	if err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(contextErr, err)
		}
		budget := 64
		unavailable, marker, pure := witnessObservationErrorLeavesV1(err, &budget)
		if pure && unavailable && !marker {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(ErrWitnessObserveUnavailableV1, err)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, monotonicheadport.ErrUnavailable) ||
			errors.Is(err, monotonicheadport.ErrNotEnrolled) || errors.Is(err, monotonicheadport.ErrEquivocation) {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, err
		}
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, ErrUnavailable
	}
	if domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
		observation, request, authority.installationID, authority.authorityKeyID, authority.authorityKey,
		authority.enrollmentID, authority.witnessKeyID, authority.witnessKey,
	) != nil || observation.Checkpoint.Namespace != domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1 {
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
	}
	if err := authority.checkpointFloor.ProjectWitnessSelected(ctx, observation.Checkpoint); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, contextErr
		}
		if errors.Is(err, monotonicheadport.ErrCheckpointFloorUnavailable) ||
			errors.Is(err, monotonicheadport.ErrCheckpointFloorIndeterminate) {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(ErrUnavailable, err)
		}
		if errors.Is(err, monotonicheadport.ErrCheckpointFloorBootstrap) {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(ErrIntegrity, err)
		}
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation, err)
	}
	if authority.lastCheckpoint != nil {
		previous := *authority.lastCheckpoint
		current := observation.Checkpoint
		if current.Generation < previous.Generation || (current.Generation == previous.Generation && current != previous) ||
			(current.Generation == previous.Generation+1 &&
				(current.PreviousCheckpointDigest != previous.CheckpointDigest || current.PreviousStateDigest != previous.CurrentStateDigest)) {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
		}
	}
	checkpoint := observation.Checkpoint
	authority.lastCheckpoint = &checkpoint
	return request, observation, nil
}

func (authority *Authority) headFromObservationLocked(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1, observation domainsecurity.MonotonicHeadObservationV1) (Head, error) {
	head := Head{Request: request, Observation: observation}
	if observation.Checkpoint.Generation == 0 {
		if err := authority.projection.ValidateWitnessEmpty(ctx); err != nil {
			if contextErr := contextError(ctx); contextErr != nil {
				return Head{}, contextErr
			}
			return Head{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation, err)
		}
		return head, nil
	}
	bundle, err := authority.bundles.Resolve(ctx, observation.Checkpoint.CurrentStateDigest)
	if err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return Head{}, contextErr
		}
		return Head{}, ErrIntegrity
	}
	if bundle.RecordDigest != observation.Checkpoint.CurrentStateDigest ||
		domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(bundle, observation.Checkpoint) != nil {
		return Head{}, ErrIntegrity
	}
	if err := authority.validateBundleChain(ctx, bundle); err != nil {
		return Head{}, err
	}
	if authority.lastBundle != nil {
		previous := *authority.lastBundle
		if bundle.Generation < previous.Generation ||
			(bundle.Generation == previous.Generation && !equalBundleRecord(bundle, previous)) ||
			(bundle.Generation > previous.Generation && !authority.bundleIsAncestor(ctx, bundle, previous)) {
			return Head{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
		}
	}
	selected := bundle
	authority.lastBundle = &selected
	historical := storeport.ObservationBundle{Bundle: bundle, Request: request, Observation: observation}
	if err := authority.persistObservationExact(ctx, historical); err != nil {
		return Head{}, err
	}
	if err := authority.projection.ProjectWitnessSelected(ctx, bundle); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return Head{}, contextErr
		}
		return Head{}, ErrUnavailable
	}
	head.HasBundle = true
	head.Bundle = bundle
	return head, nil
}

func (authority *Authority) newBundle(ctx context.Context, previous *domainevidence.EvidenceAuthorityBundleV1, mutationID, changedChild string, roots Genesis) (domainevidence.EvidenceAuthorityBundleV1, error) {
	input := domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: authority.installationID, EnrollmentID: authority.enrollmentID,
		Generation: 1, MutationID: mutationID,
		DatasetSnapshotIndexDigest:  roots.DatasetSnapshotIndexDigest,
		EvidenceRegistryIndexDigest: roots.EvidenceRegistryIndexDigest,
		PublicationIndexDigest:      roots.PublicationIndexDigest,
		AuthorityKeyID:              authority.authorityKeyID, AuthorityPublicKey: authority.authorityKey,
	}
	if previous != nil {
		input.Generation = previous.Generation + 1
		input.PreviousBundleDigest = previous.RecordDigest
		input.DatasetSnapshotCount = previous.DatasetSnapshotCount
		input.EvidenceRegistryCount = previous.EvidenceRegistryCount
		input.PublicationCount = previous.PublicationCount
		switch Child(changedChild) {
		case ChildDatasetSnapshot:
			input.DatasetSnapshotCount++
		case ChildEvidenceRegistry:
			input.EvidenceRegistryCount++
		case ChildPublication:
			input.PublicationCount++
		default:
			return domainevidence.EvidenceAuthorityBundleV1{}, ErrInvalidInput
		}
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(input, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil || authority.verifyBundle(ctx, bundle) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainevidence.EvidenceAuthorityBundleV1{}, contextErr
		}
		return domainevidence.EvidenceAuthorityBundleV1{}, ErrIntegrity
	}
	if previous != nil && domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(*previous, bundle) != nil {
		return domainevidence.EvidenceAuthorityBundleV1{}, ErrIntegrity
	}
	return bundle, nil
}

func (authority *Authority) newAdvanceRequest(ctx context.Context, previous domainsecurity.MonotonicHeadCheckpointV1, next domainevidence.EvidenceAuthorityBundleV1, mutationID string) (domainsecurity.MonotonicHeadAdvanceRequestV1, error) {
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: authority.installationID, EnrollmentID: authority.enrollmentID,
		Namespace:          domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		ExpectedGeneration: previous.Generation, ExpectedCheckpointDigest: previous.CheckpointDigest,
		ExpectedStateDigest: previous.CurrentStateDigest, NextGeneration: next.Generation,
		NextStateDigest: next.RecordDigest, ExpectedFenceNonce: previous.FenceNonce, MutationID: mutationID,
		AuthorityKeyID: authority.authorityKeyID, AuthorityPublicKey: authority.authorityKey,
	}, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil || authority.verifyAdvanceRequest(ctx, request) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.MonotonicHeadAdvanceRequestV1{}, contextErr
		}
		return domainsecurity.MonotonicHeadAdvanceRequestV1{}, ErrIntegrity
	}
	return request, nil
}

func (authority *Authority) validateAdvance(previous Head, next domainevidence.EvidenceAuthorityBundleV1, request domainsecurity.MonotonicHeadAdvanceRequestV1, receipt domainsecurity.MonotonicHeadAdvanceReceiptV1) error {
	checkpoint := previous.Observation.Checkpoint
	if domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
		checkpoint, request, receipt, authority.installationID, authority.authorityKeyID, authority.authorityKey,
		authority.enrollmentID, authority.witnessKeyID, authority.witnessKey,
	) != nil {
		return ErrIntegrity
	}
	if previous.HasBundle {
		if domainevidence.ValidateEvidenceAuthorityBundleWitnessAdvanceV1(previous.Bundle, next, checkpoint, request, receipt) != nil {
			return ErrIntegrity
		}
	} else if domainevidence.ValidateEvidenceAuthorityBundleFirstWitnessAdvanceV1(next, checkpoint, request, receipt) != nil {
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) reconcileIndeterminateLocked(ctx context.Context, previous Head, next domainevidence.EvidenceAuthorityBundleV1) (Head, error) {
	request, observation, err := authority.freshObservationLocked(ctx)
	if err != nil {
		return Head{}, err
	}
	checkpoint := observation.Checkpoint
	if checkpoint.Generation == next.Generation && checkpoint.CurrentStateDigest == next.RecordDigest {
		observed, headErr := authority.headFromObservationLocked(ctx, request, observation)
		if headErr != nil || !observed.HasBundle || observed.Bundle.RecordDigest != next.RecordDigest {
			if headErr != nil {
				return Head{}, headErr
			}
			return Head{}, ErrIntegrity
		}
		return observed, nil
	}
	if checkpoint == previous.Observation.Checkpoint {
		return Head{}, errors.Join(monotonicheadport.ErrIndeterminate, ErrAdvanceNotCommitted)
	}
	return Head{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
}

func (authority *Authority) persistBundleExact(ctx context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	if authority.verifyBundle(ctx, bundle) != nil {
		return ErrIntegrity
	}
	if err := authority.bundles.PutIfAbsent(ctx, bundle); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrUnavailable
	}
	readback, err := authority.bundles.Resolve(ctx, bundle.RecordDigest)
	if err != nil || !equalBundleRecord(readback, bundle) || authority.verifyBundle(ctx, readback) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) persistObservationExact(ctx context.Context, bundle storeport.ObservationBundle) error {
	if authority.validateObservationBundle(ctx, bundle) != nil {
		return ErrIntegrity
	}
	if err := authority.observations.PutIfAbsent(ctx, bundle); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrUnavailable
	}
	readback, err := authority.observations.Resolve(ctx, bundle.Observation.ObservationDigest)
	if err != nil || !equalObservationBundle(readback, bundle) || authority.validateObservationBundle(ctx, readback) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) validateObservationBundle(ctx context.Context, bundle storeport.ObservationBundle) error {
	if authority.verifyBundle(ctx, bundle.Bundle) != nil || authority.verifyObserveRequest(ctx, bundle.Request) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			bundle.Observation, bundle.Request, authority.installationID, authority.authorityKeyID, authority.authorityKey,
			authority.enrollmentID, authority.witnessKeyID, authority.witnessKey,
		) != nil || domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(bundle.Bundle, bundle.Observation.Checkpoint) != nil {
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) validateBundleChain(ctx context.Context, current domainevidence.EvidenceAuthorityBundleV1) error {
	if authority.verifyBundle(ctx, current) != nil {
		return ErrIntegrity
	}
	cursor := current
	for depth := 0; cursor.Generation > 1; depth++ {
		if depth >= maxBundleChainDepth {
			return ErrIntegrity
		}
		previous, err := authority.bundles.Resolve(ctx, cursor.PreviousBundleDigest)
		if err != nil || previous.RecordDigest != cursor.PreviousBundleDigest || authority.verifyBundle(ctx, previous) != nil ||
			domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, cursor) != nil {
			if contextErr := contextError(ctx); contextErr != nil {
				return contextErr
			}
			return ErrIntegrity
		}
		cursor = previous
	}
	if cursor.Generation != 1 || cursor.PreviousBundleDigest != "" ||
		cursor.DatasetSnapshotIndexDigest != authority.genesis.DatasetSnapshotIndexDigest || cursor.DatasetSnapshotCount != 0 ||
		cursor.EvidenceRegistryIndexDigest != authority.genesis.EvidenceRegistryIndexDigest || cursor.EvidenceRegistryCount != 0 ||
		cursor.PublicationIndexDigest != authority.genesis.PublicationIndexDigest || cursor.PublicationCount != 0 {
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) bundleIsAncestor(ctx context.Context, current, ancestor domainevidence.EvidenceAuthorityBundleV1) bool {
	if current.Generation < ancestor.Generation {
		return false
	}
	cursor := current
	for cursor.Generation > ancestor.Generation {
		previous, err := authority.bundles.Resolve(ctx, cursor.PreviousBundleDigest)
		if err != nil || authority.verifyBundle(ctx, previous) != nil ||
			domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, cursor) != nil {
			return false
		}
		cursor = previous
	}
	return equalBundleRecord(cursor, ancestor)
}

func (authority *Authority) verifyBundle(ctx context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	if domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
		bundle, authority.installationID, authority.enrollmentID, authority.authorityKeyID, authority.authorityKey,
	) != nil || bundle.Namespace != domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1 {
		return ErrIntegrity
	}
	return authority.verifySignature(ctx, bundle.AuthorityKeyID, bundle.AuthorityPublicKey, bundle.AuthoritySignature,
		domainevidence.EvidenceAuthorityBundleSigningBytesV1(bundle))
}

func (authority *Authority) verifyObserveRequest(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1) error {
	if domainsecurity.ValidateMonotonicHeadObserveRequestForInstallationV1(request, authority.installationID, authority.authorityKeyID, authority.authorityKey) != nil ||
		request.EnrollmentID != authority.enrollmentID || request.Namespace != domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1 {
		return ErrIntegrity
	}
	return authority.verifySignature(ctx, request.AuthorityKeyID, request.AuthorityPublicKey, request.AuthoritySignature,
		domainsecurity.MonotonicHeadObserveRequestSigningBytesV1(request))
}

func (authority *Authority) verifyAdvanceRequest(ctx context.Context, request domainsecurity.MonotonicHeadAdvanceRequestV1) error {
	if domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(request, authority.installationID, authority.authorityKeyID, authority.authorityKey) != nil ||
		request.EnrollmentID != authority.enrollmentID || request.Namespace != domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1 {
		return ErrIntegrity
	}
	return authority.verifySignature(ctx, request.AuthorityKeyID, request.AuthorityPublicKey, request.AuthoritySignature,
		domainsecurity.MonotonicHeadAdvanceRequestSigningBytesV1(request))
}

func (authority *Authority) verifySignature(ctx context.Context, keyID, encodedPublicKey, encodedSignature string, message []byte) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(encodedPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(encodedSignature)
	if publicErr != nil || signatureErr != nil || authority.authority.VerifyTrusted(ctx, keyID, publicKey, message, signature) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) freshNonceLocked(ctx context.Context, purpose string) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	var randomBytes [32]byte
	if _, err := io.ReadFull(authority.random, randomBytes[:]); err != nil {
		return "", ErrUnavailable
	}
	nonce := domainsecurity.SHA256Hex(append([]byte("analytix.evidence-authority/"+purpose+"/v1\x00"), randomBytes[:]...))
	if _, reused := authority.usedNonces[nonce]; reused {
		return "", ErrIntegrity
	}
	if len(authority.nonceOrder) < evidenceAuthorityNonceReplayWindow {
		authority.nonceOrder = append(authority.nonceOrder, nonce)
	} else {
		delete(authority.usedNonces, authority.nonceOrder[authority.nonceCursor])
		authority.nonceOrder[authority.nonceCursor] = nonce
		authority.nonceCursor = (authority.nonceCursor + 1) % evidenceAuthorityNonceReplayWindow
	}
	authority.usedNonces[nonce] = struct{}{}
	return nonce, nil
}

func validChild(child Child) bool {
	switch child {
	case ChildDatasetSnapshot, ChildEvidenceRegistry, ChildPublication:
		return true
	default:
		return false
	}
}

func canonicalDigest(value string) bool {
	return value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func equalBundleRecord(left, right domainevidence.EvidenceAuthorityBundleV1) bool {
	leftBytes, leftErr := domainevidence.EvidenceAuthorityBundleV1Bytes(left)
	rightBytes, rightErr := domainevidence.EvidenceAuthorityBundleV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBytes, rightBytes)
}

func equalObservationBundle(left, right storeport.ObservationBundle) bool {
	if !equalBundleRecord(left.Bundle, right.Bundle) {
		return false
	}
	leftRequest, leftRequestErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(left.Request)
	rightRequest, rightRequestErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(right.Request)
	leftObservation, leftObservationErr := domainsecurity.MonotonicHeadObservationV1Bytes(left.Observation)
	rightObservation, rightObservationErr := domainsecurity.MonotonicHeadObservationV1Bytes(right.Observation)
	return leftRequestErr == nil && rightRequestErr == nil && leftObservationErr == nil && rightObservationErr == nil &&
		bytes.Equal(leftRequest, rightRequest) && bytes.Equal(leftObservation, rightObservation)
}
