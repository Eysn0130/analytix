package threadriskauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
	storeport "analytix.local/runtime-go/internal/ports/threadriskauthority"
	policyport "analytix.local/runtime-go/internal/ports/threadriskpolicy"
)

const maxAuthorityChainDepth = 100_000

var (
	ErrInvalidInput                = errors.New("witnessed thread risk authority input is invalid")
	ErrUnavailable                 = errors.New("witnessed thread risk authority is unavailable")
	ErrIntegrity                   = errors.New("witnessed thread risk authority integrity failure")
	ErrAdvanceNotCommitted         = errors.New("witnessed thread risk authority advance was not committed")
	ErrCurrentThreadChanged        = errors.New("witnessed thread risk authority thread head changed")
	ErrHistoricalObservationAbsent = errors.New("witnessed thread risk authority observation is absent")
)

type Config struct {
	InstallationID   string
	EnrollmentID     string
	Namespace        string
	Authority        finalauthorityport.Authority
	WitnessKeyID     string
	WitnessPublicKey []byte
	Random           io.Reader
	Witness          monotonicheadport.Witness
	CheckpointFloor  monotonicheadport.CheckpointFloor
	Indexes          storeport.IndexStore
	Observations     storeport.ObservationStore
	Projection       storeport.Projection
	Policies         policyport.Store
}

type ResolveOrRaiseInput struct {
	ThreadID             string
	WorkspaceRealPath    string
	BindingObservation   domainsecurity.CaseBindingObservationV1
	RequestedRisk        string
	Origin               string
	SignalsDigest        string
	PreviousPolicyDigest string
	IssuedAt             time.Time
}

// Head is the closed authority-result union used by turn security. A witnessed
// result carries HasIndex plus Index/Request/Observation/Policy. The
// non-elevating host-general-only result carries GeneralOnlyPolicy, Found=true,
// HasIndex=false, and no witness fields. Mixing variants is rejected by the
// resolver. Found describes only the requested thread entry.
type Head struct {
	HasIndex             bool
	Index                domainsecurity.ThreadRiskAuthorityIndexV1
	Request              domainsecurity.MonotonicHeadObserveRequestV1
	Observation          domainsecurity.MonotonicHeadObservationV1
	RiskAuthorityBinding domainsecurity.RiskAuthorityBindingV1
	Policy               domainsecurity.ThreadRiskPolicyV1
	GeneralOnlyPolicy    domainsecurity.GeneralOnlyRiskPolicyV1
	Found                bool
}

// Authority never infers current from local records. The mutex serializes
// callers in this process; the independently enrolled witness remains the
// cross-process compare-and-swap authority.
type Authority struct {
	installationID  string
	enrollmentID    string
	namespace       string
	authority       finalauthorityport.Authority
	witnessKeyID    string
	witnessKey      []byte
	random          io.Reader
	witness         monotonicheadport.Witness
	checkpointFloor monotonicheadport.CheckpointFloor
	indexes         storeport.IndexStore
	observations    storeport.ObservationStore
	projection      storeport.Projection
	policies        policyport.Store

	mu             sync.Mutex
	usedNonces     map[string]struct{}
	lastCheckpoint *domainsecurity.MonotonicHeadCheckpointV1
}

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
	if config.InstallationID != strings.TrimSpace(config.InstallationID) || !domainsecurity.IsSHA256Hex(config.InstallationID) ||
		config.EnrollmentID != strings.TrimSpace(config.EnrollmentID) || !domainsecurity.IsSHA256Hex(config.EnrollmentID) ||
		config.Namespace != domainsecurity.ThreadRiskAuthorityNamespaceV1 ||
		config.Authority == nil || len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		len(witnessKey) != ed25519.PublicKeySize || config.WitnessKeyID != strings.TrimSpace(config.WitnessKeyID) ||
		config.WitnessKeyID != domainsecurity.SHA256Hex(witnessKey) || config.Random == nil || config.Witness == nil || config.CheckpointFloor == nil ||
		config.Indexes == nil || config.Observations == nil || config.Projection == nil || config.Policies == nil {
		return nil, ErrInvalidInput
	}
	return &Authority{
		installationID:  config.InstallationID,
		enrollmentID:    config.EnrollmentID,
		namespace:       config.Namespace,
		authority:       config.Authority,
		witnessKeyID:    config.WitnessKeyID,
		witnessKey:      witnessKey,
		random:          config.Random,
		witness:         config.Witness,
		checkpointFloor: config.CheckpointFloor,
		indexes:         config.Indexes,
		observations:    config.Observations,
		projection:      config.Projection,
		policies:        config.Policies,
		usedNonces:      make(map[string]struct{}),
	}, nil
}

// Current obtains a fresh witness observation on every call. Local index,
// policy, observation, or projection records can only satisfy exact lookups
// selected by that observation; they never select a head.
func (authority *Authority) Current(ctx context.Context, threadID string) (Head, error) {
	if authority == nil || strings.TrimSpace(threadID) == "" || threadID != strings.TrimSpace(threadID) {
		return Head{}, ErrInvalidInput
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.observeLocked(ctx, threadID)
}

// ResolveCurrent is intentionally identical to Current rather than a local
// content-address lookup: resolving authority always requires a fresh observe.
func (authority *Authority) ResolveCurrent(ctx context.Context, threadID string) (Head, error) {
	return authority.Current(ctx, threadID)
}

// ResolveOrRaise returns a fresh witnessed head. It never retries an Advance,
// never changes a mutation ID after dispatch, and never guesses a CAS winner
// from locally present immutable records.
func (authority *Authority) ResolveOrRaise(ctx context.Context, input ResolveOrRaiseInput) (Head, error) {
	if authority == nil || validateResolveInput(input) != nil {
		return Head{}, ErrInvalidInput
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Head{}, err
	}

	previous, err := authority.observeLocked(ctx, input.ThreadID)
	if err != nil {
		return Head{}, err
	}
	if previous.Found {
		current := previous.Policy
		if current.WorkspaceRealPath != input.WorkspaceRealPath {
			if current.RiskClass == domainsecurity.RiskClassCase || input.RequestedRisk != domainsecurity.RiskClassGeneral {
				return Head{}, ErrInvalidInput
			}
		} else if current.RiskClass == input.RequestedRisk || current.RiskClass == domainsecurity.RiskClassCase {
			return previous, nil
		} else if current.RiskClass != domainsecurity.RiskClassGeneral || input.RequestedRisk != domainsecurity.RiskClassCase {
			return Head{}, ErrInvalidInput
		}
	}

	mutationID, err := authority.freshNonceLocked(ctx, "mutation")
	if err != nil {
		return Head{}, err
	}
	policy, err := authority.newPolicy(ctx, input, previous)
	if err != nil {
		return Head{}, err
	}
	if err := authority.persistPolicyExact(ctx, policy); err != nil {
		return Head{}, err
	}
	next, err := authority.newIndex(ctx, previous, policy, mutationID)
	if err != nil {
		return Head{}, err
	}
	if err := authority.persistIndexExact(ctx, next); err != nil {
		return Head{}, err
	}

	advance, err := authority.newAdvanceRequest(ctx, previous.Observation.Checkpoint, next, mutationID)
	if err != nil {
		return Head{}, err
	}
	receipt, advanceErr := authority.witness.Advance(ctx, advance)
	if advanceErr != nil {
		if errors.Is(advanceErr, monotonicheadport.ErrIndeterminate) {
			return authority.reconcileIndeterminateLocked(ctx, input.ThreadID, previous, next)
		}
		if errors.Is(advanceErr, context.Canceled) || errors.Is(advanceErr, context.DeadlineExceeded) ||
			errors.Is(advanceErr, monotonicheadport.ErrCASConflict) || errors.Is(advanceErr, monotonicheadport.ErrMutationConflict) ||
			errors.Is(advanceErr, monotonicheadport.ErrUnavailable) || errors.Is(advanceErr, monotonicheadport.ErrEquivocation) {
			return Head{}, advanceErr
		}
		return Head{}, ErrUnavailable
	}
	if err := authority.validateAdvance(previous, next, advance, receipt); err != nil {
		return Head{}, err
	}

	committed, err := authority.observeLocked(ctx, input.ThreadID)
	if err != nil {
		return Head{}, err
	}
	if !committed.HasIndex || !authority.indexIsAncestor(ctx, committed.Index, next) {
		return Head{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
	}
	return committed, nil
}

// ValidateCurrent validates both issuance-time membership and a new current
// witness observation. Unrelated global index advances are allowed only when
// the exact thread entry is unchanged; any loss, risk change, policy change,
// or workspace change rejects the old TSC.
func (authority *Authority) ValidateCurrent(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if authority == nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil ||
		securityContext.RiskAuthorityBinding.State != domainsecurity.RiskAuthorityBindingStateWitnessed {
		return ErrInvalidInput
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}

	bundle, err := authority.observations.Resolve(ctx, securityContext.RiskAuthorityBinding.ObservationDigest)
	if err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrHistoricalObservationAbsent
	}
	if err := authority.validateBundle(ctx, bundle); err != nil {
		return err
	}
	if err := domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
		securityContext.RiskAuthorityBinding, bundle.Index, bundle.Request, bundle.Observation,
	); err != nil {
		return ErrIntegrity
	}
	storedOld, err := authority.indexes.Resolve(ctx, bundle.Index.IndexDigest)
	if err != nil || !equalIndex(storedOld, bundle.Index) {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	if err := authority.validateIndexAndPolicyChains(ctx, bundle.Index); err != nil {
		return err
	}
	oldEntry, oldPolicy, found, err := authority.entryAndPolicy(ctx, bundle.Index, securityContext.ThreadID)
	if err != nil || !found || oldPolicy.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, oldPolicy) != nil {
		return ErrIntegrity
	}

	current, err := authority.observeLocked(ctx, securityContext.ThreadID)
	if err != nil {
		return err
	}
	if current.HasIndex && current.Index.Generation == bundle.Index.Generation &&
		current.Observation.Checkpoint != bundle.Observation.Checkpoint {
		return errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
	}
	if !current.HasIndex || !current.Found || !authority.indexIsAncestor(ctx, current.Index, bundle.Index) {
		return errors.Join(ErrCurrentThreadChanged, ErrIntegrity)
	}
	currentEntry, currentPolicy, found, err := authority.entryAndPolicy(ctx, current.Index, securityContext.ThreadID)
	if err != nil || !found || currentEntry != oldEntry || currentPolicy.PolicyDigest != oldPolicy.PolicyDigest {
		return ErrCurrentThreadChanged
	}
	return nil
}

func (authority *Authority) observeLocked(ctx context.Context, threadID string) (Head, error) {
	request, observation, err := authority.freshObservationLocked(ctx)
	if err != nil {
		return Head{}, err
	}
	return authority.headFromObservationLocked(ctx, threadID, request, observation)
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
		InstallationID:     authority.installationID,
		EnrollmentID:       authority.enrollmentID,
		Namespace:          authority.namespace,
		ChallengeNonce:     challenge,
		AuthorityKeyID:     authority.authority.KeyID(),
		AuthorityPublicKey: authority.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil || authority.verifyObserveRequest(ctx, request) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, contextErr
		}
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, ErrIntegrity
	}
	observation, err := authority.witness.Observe(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, monotonicheadport.ErrUnavailable) || errors.Is(err, monotonicheadport.ErrNotEnrolled) ||
			errors.Is(err, monotonicheadport.ErrEquivocation) {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, err
		}
		return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, ErrUnavailable
	}
	if err := domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
		observation, request, authority.installationID, authority.authority.KeyID(), authority.authority.PublicKey(),
		authority.enrollmentID, authority.witnessKeyID, authority.witnessKey,
	); err != nil || observation.Checkpoint.Namespace != authority.namespace {
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
		if current.Generation < previous.Generation ||
			(current.Generation == previous.Generation && current != previous) ||
			(current.Generation == previous.Generation+1 &&
				(current.PreviousCheckpointDigest != previous.CheckpointDigest || current.PreviousStateDigest != previous.CurrentStateDigest)) {
			return domainsecurity.MonotonicHeadObserveRequestV1{}, domainsecurity.MonotonicHeadObservationV1{}, errors.Join(ErrIntegrity, monotonicheadport.ErrEquivocation)
		}
	}
	checkpoint := observation.Checkpoint
	authority.lastCheckpoint = &checkpoint
	return request, observation, nil
}

func (authority *Authority) headFromObservationLocked(ctx context.Context, threadID string, request domainsecurity.MonotonicHeadObserveRequestV1, observation domainsecurity.MonotonicHeadObservationV1) (Head, error) {
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
	index, err := authority.indexes.Resolve(ctx, observation.Checkpoint.CurrentStateDigest)
	if err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return Head{}, contextErr
		}
		return Head{}, ErrIntegrity
	}
	if index.IndexDigest != observation.Checkpoint.CurrentStateDigest ||
		domainsecurity.ValidateThreadRiskAuthorityIndexCheckpointV1(index, observation.Checkpoint) != nil {
		return Head{}, ErrIntegrity
	}
	if err := authority.validateIndexAndPolicyChains(ctx, index); err != nil {
		return Head{}, err
	}
	bundle := storeport.ObservationBundle{Index: index, Request: request, Observation: observation}
	if err := authority.persistBundleExact(ctx, bundle); err != nil {
		return Head{}, err
	}
	binding, err := domainsecurity.NewWitnessedRiskAuthorityBindingV1(index, request, observation)
	if err != nil {
		return Head{}, ErrIntegrity
	}
	if err := authority.projection.ProjectWitnessSelected(ctx, index); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return Head{}, contextErr
		}
		return Head{}, ErrUnavailable
	}
	head.HasIndex = true
	head.Index = index
	head.RiskAuthorityBinding = binding
	_, policy, found, err := authority.entryAndPolicy(ctx, index, threadID)
	if err != nil {
		return Head{}, err
	}
	head.Policy = policy
	head.Found = found
	return head, nil
}

func (authority *Authority) newPolicy(ctx context.Context, input ResolveOrRaiseInput, previous Head) (domainsecurity.ThreadRiskPolicyV1, error) {
	predecessor := ""
	if previous.Found {
		predecessor = previous.Policy.PolicyDigest
	}
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID:                input.ThreadID,
		WorkspaceRealPath:       input.WorkspaceRealPath,
		RiskClass:               input.RequestedRisk,
		Origin:                  input.Origin,
		SignalsDigest:           input.SignalsDigest,
		PredecessorPolicyDigest: predecessor,
		IssuedAt:                input.IssuedAt,
		AuthorityKeyID:          authority.authority.KeyID(),
		AuthorityPublicKey:      authority.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil || authority.verifyPolicy(ctx, policy) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, contextErr
		}
		return domainsecurity.ThreadRiskPolicyV1{}, ErrIntegrity
	}
	if previous.Found && domainsecurity.ValidateThreadRiskPolicyTransitionV1(previous.Policy, policy) != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, ErrInvalidInput
	}
	return policy, nil
}

func (authority *Authority) newIndex(ctx context.Context, previous Head, policy domainsecurity.ThreadRiskPolicyV1, mutationID string) (domainsecurity.ThreadRiskAuthorityIndexV1, error) {
	entries := make([]domainsecurity.ThreadRiskAuthorityEntryV1, 0, 1)
	generation := uint64(1)
	previousDigest := ""
	if previous.HasIndex {
		entries = append(entries, previous.Index.Entries...)
		generation = previous.Index.Generation + 1
		previousDigest = previous.Index.IndexDigest
	}
	entry, err := domainsecurity.ThreadRiskAuthorityEntryFromPolicyV1(policy)
	if err != nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, ErrIntegrity
	}
	replaced := false
	for position := range entries {
		if entries[position].ThreadID == entry.ThreadID {
			entries[position] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	next, err := domainsecurity.NewThreadRiskAuthorityIndexV1(domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID:      authority.installationID,
		EnrollmentID:        authority.enrollmentID,
		Namespace:           authority.namespace,
		Generation:          generation,
		Entries:             entries,
		PreviousIndexDigest: previousDigest,
		MutationID:          mutationID,
		AuthorityKeyID:      authority.authority.KeyID(),
		AuthorityPublicKey:  authority.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil || authority.verifyIndex(ctx, next) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.ThreadRiskAuthorityIndexV1{}, contextErr
		}
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, ErrIntegrity
	}
	if previous.HasIndex && domainsecurity.ValidateThreadRiskAuthorityIndexTransitionV1(previous.Index, next) != nil {
		return domainsecurity.ThreadRiskAuthorityIndexV1{}, ErrIntegrity
	}
	return next, nil
}

func (authority *Authority) newAdvanceRequest(ctx context.Context, previous domainsecurity.MonotonicHeadCheckpointV1, next domainsecurity.ThreadRiskAuthorityIndexV1, mutationID string) (domainsecurity.MonotonicHeadAdvanceRequestV1, error) {
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID:           authority.installationID,
		EnrollmentID:             authority.enrollmentID,
		Namespace:                authority.namespace,
		ExpectedGeneration:       previous.Generation,
		ExpectedCheckpointDigest: previous.CheckpointDigest,
		ExpectedStateDigest:      previous.CurrentStateDigest,
		NextGeneration:           next.Generation,
		NextStateDigest:          next.IndexDigest,
		ExpectedFenceNonce:       previous.FenceNonce,
		MutationID:               mutationID,
		AuthorityKeyID:           authority.authority.KeyID(),
		AuthorityPublicKey:       authority.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.authority.Sign(ctx, message) })
	if err != nil || authority.verifyAdvanceRequest(ctx, request) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.MonotonicHeadAdvanceRequestV1{}, contextErr
		}
		return domainsecurity.MonotonicHeadAdvanceRequestV1{}, ErrIntegrity
	}
	return request, nil
}

func (authority *Authority) validateAdvance(previous Head, next domainsecurity.ThreadRiskAuthorityIndexV1, request domainsecurity.MonotonicHeadAdvanceRequestV1, receipt domainsecurity.MonotonicHeadAdvanceReceiptV1) error {
	checkpoint := previous.Observation.Checkpoint
	if domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
		checkpoint, request, receipt, authority.installationID, authority.authority.KeyID(), authority.authority.PublicKey(),
		authority.enrollmentID, authority.witnessKeyID, authority.witnessKey,
	) != nil {
		return ErrIntegrity
	}
	if previous.HasIndex {
		if domainsecurity.ValidateThreadRiskAuthorityWitnessAdvanceV1(previous.Index, next, checkpoint, request, receipt) != nil {
			return ErrIntegrity
		}
	} else if domainsecurity.ValidateThreadRiskAuthorityFirstWitnessAdvanceV1(next, checkpoint, request, receipt) != nil {
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) reconcileIndeterminateLocked(ctx context.Context, threadID string, previous Head, next domainsecurity.ThreadRiskAuthorityIndexV1) (Head, error) {
	request, observation, err := authority.freshObservationLocked(ctx)
	if err != nil {
		return Head{}, err
	}
	checkpoint := observation.Checkpoint
	if checkpoint.Generation == next.Generation && checkpoint.CurrentStateDigest == next.IndexDigest {
		observed, headErr := authority.headFromObservationLocked(ctx, threadID, request, observation)
		if headErr != nil || !observed.HasIndex || observed.Index.IndexDigest != next.IndexDigest {
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

func (authority *Authority) persistPolicyExact(ctx context.Context, policy domainsecurity.ThreadRiskPolicyV1) error {
	if err := authority.policies.PutIfAbsent(ctx, policy); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrUnavailable
	}
	readback, err := authority.policies.Resolve(ctx, policy.PolicyDigest)
	if err != nil || !equalPolicy(readback, policy) || authority.verifyPolicy(ctx, readback) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) persistIndexExact(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if err := authority.indexes.PutIfAbsent(ctx, index); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrUnavailable
	}
	readback, err := authority.indexes.Resolve(ctx, index.IndexDigest)
	if err != nil || !equalIndex(readback, index) || authority.verifyIndex(ctx, readback) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) persistBundleExact(ctx context.Context, bundle storeport.ObservationBundle) error {
	if err := authority.validateBundle(ctx, bundle); err != nil {
		return err
	}
	if err := authority.observations.PutIfAbsent(ctx, bundle); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrUnavailable
	}
	readback, err := authority.observations.Resolve(ctx, bundle.Observation.ObservationDigest)
	if err != nil || !equalBundle(readback, bundle) {
		if contextErr := contextError(ctx); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	return authority.validateBundle(ctx, readback)
}

func (authority *Authority) validateBundle(ctx context.Context, bundle storeport.ObservationBundle) error {
	if authority.verifyIndex(ctx, bundle.Index) != nil || authority.verifyObserveRequest(ctx, bundle.Request) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			bundle.Observation, bundle.Request, authority.installationID, authority.authority.KeyID(), authority.authority.PublicKey(),
			authority.enrollmentID, authority.witnessKeyID, authority.witnessKey,
		) != nil || domainsecurity.ValidateThreadRiskAuthorityIndexCheckpointV1(bundle.Index, bundle.Observation.Checkpoint) != nil {
		return ErrIntegrity
	}
	return nil
}

func (authority *Authority) validateIndexAndPolicyChains(ctx context.Context, current domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if authority.verifyIndex(ctx, current) != nil {
		return ErrIntegrity
	}
	chain := []domainsecurity.ThreadRiskAuthorityIndexV1{current}
	cursor := current
	for cursor.Generation > 1 {
		if len(chain) >= maxAuthorityChainDepth {
			return ErrIntegrity
		}
		previous, err := authority.indexes.Resolve(ctx, cursor.PreviousIndexDigest)
		if err != nil || authority.verifyIndex(ctx, previous) != nil || previous.IndexDigest != cursor.PreviousIndexDigest {
			if contextErr := contextError(ctx); contextErr != nil {
				return contextErr
			}
			return ErrIntegrity
		}
		chain = append(chain, previous)
		cursor = previous
	}
	if cursor.Generation != 1 || cursor.PreviousIndexDigest != "" {
		return ErrIntegrity
	}
	for position := len(chain) - 1; position > 0; position-- {
		if domainsecurity.ValidateThreadRiskAuthorityIndexTransitionV1(chain[position], chain[position-1]) != nil {
			return ErrIntegrity
		}
	}
	for _, entry := range current.Entries {
		if _, _, found, err := authority.entryAndPolicy(ctx, current, entry.ThreadID); err != nil || !found {
			return ErrIntegrity
		}
	}
	return nil
}

func (authority *Authority) entryAndPolicy(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1, threadID string) (domainsecurity.ThreadRiskAuthorityEntryV1, domainsecurity.ThreadRiskPolicyV1, bool, error) {
	position := sort.Search(len(index.Entries), func(position int) bool { return index.Entries[position].ThreadID >= threadID })
	if position >= len(index.Entries) || index.Entries[position].ThreadID != threadID {
		return domainsecurity.ThreadRiskAuthorityEntryV1{}, domainsecurity.ThreadRiskPolicyV1{}, false, nil
	}
	entry := index.Entries[position]
	policy, err := authority.validatePolicyChain(ctx, entry)
	if err != nil {
		return domainsecurity.ThreadRiskAuthorityEntryV1{}, domainsecurity.ThreadRiskPolicyV1{}, false, err
	}
	return entry, policy, true, nil
}

func (authority *Authority) validatePolicyChain(ctx context.Context, entry domainsecurity.ThreadRiskAuthorityEntryV1) (domainsecurity.ThreadRiskPolicyV1, error) {
	head, err := authority.policies.Resolve(ctx, entry.CurrentPolicyDigest)
	if err != nil || authority.verifyPolicy(ctx, head) != nil || domainsecurity.ValidateThreadRiskAuthorityEntryForPolicyV1(entry, head) != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, contextErr
		}
		return domainsecurity.ThreadRiskPolicyV1{}, ErrIntegrity
	}
	current := head
	seen := make(map[string]struct{})
	for depth := 0; ; depth++ {
		if depth >= maxAuthorityChainDepth {
			return domainsecurity.ThreadRiskPolicyV1{}, ErrIntegrity
		}
		if _, duplicate := seen[current.PolicyDigest]; duplicate {
			return domainsecurity.ThreadRiskPolicyV1{}, ErrIntegrity
		}
		seen[current.PolicyDigest] = struct{}{}
		if current.PredecessorPolicyDigest == "" {
			break
		}
		previous, resolveErr := authority.policies.Resolve(ctx, current.PredecessorPolicyDigest)
		if resolveErr != nil || authority.verifyPolicy(ctx, previous) != nil ||
			previous.PolicyDigest != current.PredecessorPolicyDigest ||
			domainsecurity.ValidateThreadRiskPolicyTransitionV1(previous, current) != nil {
			if contextErr := contextError(ctx); contextErr != nil {
				return domainsecurity.ThreadRiskPolicyV1{}, contextErr
			}
			return domainsecurity.ThreadRiskPolicyV1{}, ErrIntegrity
		}
		current = previous
	}
	return head, nil
}

func (authority *Authority) indexIsAncestor(ctx context.Context, current, ancestor domainsecurity.ThreadRiskAuthorityIndexV1) bool {
	if current.Generation < ancestor.Generation {
		return false
	}
	cursor := current
	for cursor.Generation > ancestor.Generation {
		previous, err := authority.indexes.Resolve(ctx, cursor.PreviousIndexDigest)
		if err != nil || authority.verifyIndex(ctx, previous) != nil ||
			domainsecurity.ValidateThreadRiskAuthorityIndexTransitionV1(previous, cursor) != nil {
			return false
		}
		cursor = previous
	}
	return equalIndex(cursor, ancestor)
}

func (authority *Authority) verifyPolicy(ctx context.Context, policy domainsecurity.ThreadRiskPolicyV1) error {
	if domainsecurity.ValidateThreadRiskPolicyV1ForInstallation(policy, authority.authority.KeyID(), authority.authority.PublicKey()) != nil {
		return ErrIntegrity
	}
	return authority.verifySignature(ctx, policy.AuthorityKeyID, policy.AuthorityPublicKey, policy.AuthoritySignature,
		domainsecurity.ThreadRiskPolicySigningBytesV1(policy))
}

func (authority *Authority) verifyIndex(ctx context.Context, index domainsecurity.ThreadRiskAuthorityIndexV1) error {
	if domainsecurity.ValidateThreadRiskAuthorityIndexForInstallationV1(index, authority.installationID, authority.authority.KeyID(), authority.authority.PublicKey()) != nil ||
		index.EnrollmentID != authority.enrollmentID || index.Namespace != authority.namespace {
		return ErrIntegrity
	}
	return authority.verifySignature(ctx, index.AuthorityKeyID, index.AuthorityPublicKey, index.AuthoritySignature,
		domainsecurity.ThreadRiskAuthorityIndexSigningBytesV1(index))
}

func (authority *Authority) verifyObserveRequest(ctx context.Context, request domainsecurity.MonotonicHeadObserveRequestV1) error {
	if domainsecurity.ValidateMonotonicHeadObserveRequestForInstallationV1(request, authority.installationID, authority.authority.KeyID(), authority.authority.PublicKey()) != nil ||
		request.EnrollmentID != authority.enrollmentID || request.Namespace != authority.namespace {
		return ErrIntegrity
	}
	return authority.verifySignature(ctx, request.AuthorityKeyID, request.AuthorityPublicKey, request.AuthoritySignature,
		domainsecurity.MonotonicHeadObserveRequestSigningBytesV1(request))
}

func (authority *Authority) verifyAdvanceRequest(ctx context.Context, request domainsecurity.MonotonicHeadAdvanceRequestV1) error {
	if domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(request, authority.installationID, authority.authority.KeyID(), authority.authority.PublicKey()) != nil ||
		request.EnrollmentID != authority.enrollmentID || request.Namespace != authority.namespace {
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
	nonce := domainsecurity.SHA256Hex(append(append([]byte("analytix.thread-risk-authority/"+purpose+"/v1\x00"), randomBytes[:]...)))
	if _, reused := authority.usedNonces[nonce]; reused {
		return "", ErrIntegrity
	}
	authority.usedNonces[nonce] = struct{}{}
	return nonce, nil
}

func validateResolveInput(input ResolveOrRaiseInput) error {
	if input.ThreadID == "" || input.ThreadID != strings.TrimSpace(input.ThreadID) ||
		input.WorkspaceRealPath == "" || input.WorkspaceRealPath != strings.TrimSpace(input.WorkspaceRealPath) ||
		input.SignalsDigest != strings.TrimSpace(input.SignalsDigest) || !domainsecurity.IsSHA256Hex(input.SignalsDigest) ||
		(input.PreviousPolicyDigest != "" &&
			(input.PreviousPolicyDigest != strings.TrimSpace(input.PreviousPolicyDigest) ||
				!domainsecurity.IsSHA256Hex(input.PreviousPolicyDigest))) ||
		input.IssuedAt.IsZero() {
		return ErrInvalidInput
	}
	switch input.RequestedRisk {
	case domainsecurity.RiskClassGeneral:
		if input.Origin != domainsecurity.RiskPolicyOriginGeneralWorkspace && input.Origin != domainsecurity.RiskPolicyOriginLegacyMigration {
			return ErrInvalidInput
		}
	case domainsecurity.RiskClassCase:
		switch input.Origin {
		case domainsecurity.RiskPolicyOriginDesktopCaseEntry, domainsecurity.RiskPolicyOriginBindingMarkerPresent,
			domainsecurity.RiskPolicyOriginValidCaseBinding, domainsecurity.RiskPolicyOriginSignedCaseLineage,
			domainsecurity.RiskPolicyOriginTrustedCaseFinal, domainsecurity.RiskPolicyOriginLegacyMigration,
			domainsecurity.RiskPolicyOriginLexicalGuard, domainsecurity.RiskPolicyOriginHostInputContextChange:
		default:
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func equalPolicy(left, right domainsecurity.ThreadRiskPolicyV1) bool {
	leftBytes, leftErr := domainsecurity.ThreadRiskPolicyV1Bytes(left)
	rightBytes, rightErr := domainsecurity.ThreadRiskPolicyV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBytes, rightBytes)
}

func equalIndex(left, right domainsecurity.ThreadRiskAuthorityIndexV1) bool {
	leftBytes, leftErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(left)
	rightBytes, rightErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBytes, rightBytes)
}

func equalBundle(left, right storeport.ObservationBundle) bool {
	if !equalIndex(left.Index, right.Index) {
		return false
	}
	leftRequest, leftRequestErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(left.Request)
	rightRequest, rightRequestErr := domainsecurity.MonotonicHeadObserveRequestV1Bytes(right.Request)
	leftObservation, leftObservationErr := domainsecurity.MonotonicHeadObservationV1Bytes(left.Observation)
	rightObservation, rightObservationErr := domainsecurity.MonotonicHeadObservationV1Bytes(right.Observation)
	return leftRequestErr == nil && rightRequestErr == nil && leftObservationErr == nil && rightObservationErr == nil &&
		bytes.Equal(leftRequest, rightRequest) && bytes.Equal(leftObservation, rightObservation)
}
