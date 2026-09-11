package authorityadvance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	journalport "analytix.local/runtime-go/internal/ports/authorityadvance"
	evidenceport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
	riskport "analytix.local/runtime-go/internal/ports/threadriskauthority"
)

var (
	ErrInvalidInput       = errors.New("authority advance coordinator input is invalid")
	ErrUnavailable        = errors.New("authority advance journal is unavailable")
	ErrIntegrity          = errors.New("authority advance journal integrity failure")
	ErrUnresolved         = errors.New("authority advance remains unresolved")
	ErrCommittedUnsettled = errors.New("authority advance committed but settlement is not durable")
)

type Config struct {
	InstallationID   string
	EnrollmentID     string
	Namespace        string
	Authority        finalauthorityport.Authority
	WitnessKeyID     string
	WitnessPublicKey []byte
	Witness          monotonicheadport.MutationRecoveryWitness
	Random           io.Reader
	Intents          journalport.IntentStore
	Settlements      journalport.SettlementStore
	RiskIndexes      riskport.IndexStore
	EvidenceBundles  evidenceport.BundleStore
}

type CommitResultV2 struct {
	Intent     domainauthority.MonotonicAdvanceIntentV2
	Settlement domainauthority.MonotonicAdvanceSettlementV2
	Replayed   bool
}

// Coordinator writes no projection and never infers current from local
// inventory. Its only mutation effect is the exact request already signed into
// a durable intent; callers must separately reconcile a fresh witness head.
type Coordinator struct {
	installationID  string
	enrollmentID    string
	namespace       string
	authority       finalauthorityport.Authority
	authorityKeyID  string
	authorityKey    []byte
	witnessKeyID    string
	witnessKey      []byte
	witness         monotonicheadport.MutationRecoveryWitness
	random          io.Reader
	intents         journalport.IntentStore
	settlements     journalport.SettlementStore
	riskIndexes     riskport.IndexStore
	evidenceBundles evidenceport.BundleStore

	admission      chan struct{}
	challenges     map[string]struct{}
	challengeOrder []string
}

const recoveryChallengeWindowV2 = 4_096

func New(config Config) (*Coordinator, error) {
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
	if !domainsecurity.IsSHA256Hex(config.InstallationID) || config.InstallationID != strings.TrimSpace(config.InstallationID) ||
		!domainsecurity.IsSHA256Hex(config.EnrollmentID) || config.EnrollmentID != strings.TrimSpace(config.EnrollmentID) ||
		(config.Namespace != domainsecurity.ThreadRiskAuthorityNamespaceV1 &&
			config.Namespace != domainsecurity.EvidenceRegistryAuthorityNamespaceV1) ||
		config.Authority == nil || len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		len(witnessKey) != ed25519.PublicKeySize || config.WitnessKeyID != domainsecurity.SHA256Hex(witnessKey) ||
		config.Witness == nil || config.Random == nil || config.Intents == nil || config.Settlements == nil ||
		(config.Namespace == domainsecurity.ThreadRiskAuthorityNamespaceV1 && config.RiskIndexes == nil) ||
		(config.Namespace == domainsecurity.EvidenceRegistryAuthorityNamespaceV1 && config.EvidenceBundles == nil) {
		return nil, ErrInvalidInput
	}
	return &Coordinator{
		installationID: config.InstallationID, enrollmentID: config.EnrollmentID, namespace: config.Namespace,
		authority: config.Authority, authorityKeyID: keyID, authorityKey: publicKey,
		witnessKeyID: config.WitnessKeyID, witnessKey: witnessKey,
		witness: config.Witness, intents: config.Intents, settlements: config.Settlements,
		random:      config.Random,
		riskIndexes: config.RiskIndexes, evidenceBundles: config.EvidenceBundles,
		admission: make(chan struct{}, 1), challenges: make(map[string]struct{}),
		challengeOrder: make([]string, 0, recoveryChallengeWindowV2),
	}, nil
}

func (coordinator *Coordinator) CommitExact(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) (CommitResultV2, error) {
	if coordinator == nil || ctx == nil {
		return CommitResultV2{}, ErrInvalidInput
	}
	if err := coordinator.acquire(ctx); err != nil {
		return CommitResultV2{}, err
	}
	defer coordinator.release()
	if err := coordinator.validateIntent(ctx, intent); err != nil {
		return CommitResultV2{}, err
	}
	if err := coordinator.validateCandidateReadback(ctx, intent, false); err != nil {
		persisted, persistedErr := coordinator.persistedIntentExact(ctx, intent)
		if persistedErr != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return CommitResultV2{}, contextErr
			}
			if errors.Is(err, ErrIntegrity) {
				return CommitResultV2{}, errors.Join(err, persistedErr)
			}
			return CommitResultV2{}, persistedErr
		}
		if persisted {
			return CommitResultV2{}, errors.Join(ErrUnresolved, ErrIntegrity)
		}
		return CommitResultV2{}, err
	}
	if err := coordinator.persistIntentExact(ctx, intent); err != nil {
		return CommitResultV2{}, err
	}
	return coordinator.commitPersistedIntent(ctx, intent)
}

func (coordinator *Coordinator) RecoverExact(
	ctx context.Context,
	mutationID string,
) (CommitResultV2, error) {
	if coordinator == nil || ctx == nil || mutationID != strings.TrimSpace(mutationID) ||
		!domainsecurity.IsSHA256Hex(mutationID) {
		return CommitResultV2{}, ErrInvalidInput
	}
	if err := coordinator.acquire(ctx); err != nil {
		return CommitResultV2{}, err
	}
	defer coordinator.release()
	intent, err := coordinator.intents.ResolveIntent(ctx, mutationID)
	if err != nil {
		return CommitResultV2{}, classifyReadErrorV2(ctx, err)
	}
	if intent.MutationID != mutationID {
		return CommitResultV2{}, ErrIntegrity
	}
	if err := coordinator.validateIntent(ctx, intent); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return CommitResultV2{}, contextErr
		}
		return CommitResultV2{}, ErrIntegrity
	}
	if err := coordinator.validateCandidateReadback(ctx, intent, true); err != nil {
		return CommitResultV2{}, err
	}
	settlement, err := coordinator.settlements.ResolveSettlement(ctx, intent.MutationID)
	if errors.Is(err, journalport.ErrNotFound) {
		return coordinator.recoverUnsettledIntent(ctx, intent)
	}
	if err != nil {
		return CommitResultV2{}, classifyReadErrorV2(ctx, err)
	}
	if domainauthority.ValidateMonotonicAdvanceSettlementIntentHeaderV2(settlement, intent) != nil {
		return CommitResultV2{}, ErrIntegrity
	}
	if settlement.Kind == domainauthority.MonotonicAdvanceSettlementSupersededV2 {
		return CommitResultV2{}, ErrUnresolved
	}
	if !equalSettlementForIntentV2(settlement, intent) {
		return CommitResultV2{}, ErrIntegrity
	}
	// This is historical settlement replay only. It cannot authorize current
	// projection without a fresh live-anchor witness observation.
	return CommitResultV2{Intent: intent, Settlement: settlement, Replayed: true}, nil
}

func (coordinator *Coordinator) recoverUnsettledIntent(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) (CommitResultV2, error) {
	challenge, err := coordinator.freshRecoveryChallenge(ctx, intent.MutationID)
	if err != nil {
		return CommitResultV2{}, err
	}
	request, err := domainsecurity.NewMonotonicHeadMutationResolveRequestV1(
		intent.AdvanceRequest,
		challenge,
		func(message []byte) ([]byte, error) { return coordinator.authority.Sign(ctx, message) },
	)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return CommitResultV2{}, contextErr
		}
		return CommitResultV2{}, errors.Join(ErrUnresolved, ErrIntegrity)
	}
	resolution, err := coordinator.witness.ResolveMutation(ctx, request)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return CommitResultV2{}, contextErr
		}
		if errors.Is(err, monotonicheadport.ErrInvalidReceipt) || errors.Is(err, monotonicheadport.ErrEquivocation) ||
			errors.Is(err, monotonicheadport.ErrMutationConflict) {
			return CommitResultV2{}, errors.Join(ErrUnresolved, ErrIntegrity, err)
		}
		return CommitResultV2{}, errors.Join(ErrUnresolved, err)
	}
	if err := domainsecurity.ValidateMonotonicHeadMutationResolutionForRequestV1(
		resolution,
		request,
		coordinator.installationID,
		coordinator.authorityKeyID,
		coordinator.authorityKey,
		coordinator.enrollmentID,
		coordinator.witnessKeyID,
		coordinator.witnessKey,
	); err != nil {
		return CommitResultV2{}, errors.Join(ErrUnresolved, ErrIntegrity, monotonicheadport.ErrInvalidReceipt)
	}
	if resolution.Status == domainsecurity.MonotonicHeadMutationResolutionAbsentV1 {
		return CommitResultV2{}, ErrUnresolved
	}
	committed := resolution.Committed
	if committed == nil || domainauthority.ValidateMonotonicAdvanceIntentExactReplayV2(intent, committed.AdvanceRequest) != nil ||
		domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
			intent.PreviousCheckpoint,
			committed.AdvanceRequest,
			committed.AdvanceReceipt,
			coordinator.installationID,
			coordinator.authorityKeyID,
			coordinator.authorityKey,
			coordinator.enrollmentID,
			coordinator.witnessKeyID,
			coordinator.witnessKey,
		) != nil {
		return CommitResultV2{}, errors.Join(ErrUnresolved, ErrIntegrity, monotonicheadport.ErrInvalidReceipt)
	}
	return coordinator.persistCommittedSettlement(ctx, intent, committed.AdvanceReceipt, true)
}

func (coordinator *Coordinator) commitPersistedIntent(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) (CommitResultV2, error) {
	if settlement, err := coordinator.settlements.ResolveSettlement(ctx, intent.MutationID); err == nil {
		if domainauthority.ValidateMonotonicAdvanceSettlementIntentHeaderV2(settlement, intent) != nil {
			return CommitResultV2{}, ErrIntegrity
		}
		if settlement.Kind == domainauthority.MonotonicAdvanceSettlementSupersededV2 {
			return CommitResultV2{}, ErrUnresolved
		}
		if !equalSettlementForIntentV2(settlement, intent) {
			return CommitResultV2{}, ErrIntegrity
		}
		return CommitResultV2{Intent: intent, Settlement: settlement, Replayed: true}, nil
	} else if !errors.Is(err, journalport.ErrNotFound) {
		return CommitResultV2{}, classifyReadErrorV2(ctx, err)
	}

	receipt, err := coordinator.witness.Advance(ctx, intent.AdvanceRequest)
	if err != nil {
		if errors.Is(err, monotonicheadport.ErrMutationConflict) ||
			errors.Is(err, monotonicheadport.ErrEquivocation) ||
			errors.Is(err, monotonicheadport.ErrInvalidReceipt) {
			if errors.Is(err, monotonicheadport.ErrInvalidReceipt) {
				return CommitResultV2{}, errors.Join(
					ErrUnresolved, ErrIntegrity, monotonicheadport.ErrIndeterminate, err,
				)
			}
			return CommitResultV2{}, errors.Join(ErrUnresolved, ErrIntegrity, err)
		}
		if errors.Is(err, monotonicheadport.ErrCASConflict) ||
			errors.Is(err, monotonicheadport.ErrIndeterminate) {
			return CommitResultV2{}, errors.Join(ErrUnresolved, err)
		}
		return CommitResultV2{}, err
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
		intent.PreviousCheckpoint,
		intent.AdvanceRequest,
		receipt,
		coordinator.installationID,
		coordinator.authorityKeyID,
		coordinator.authorityKey,
		coordinator.enrollmentID,
		coordinator.witnessKeyID,
		coordinator.witnessKey,
	); err != nil {
		return CommitResultV2{}, errors.Join(
			ErrUnresolved, ErrIntegrity, monotonicheadport.ErrIndeterminate,
		)
	}
	return coordinator.persistCommittedSettlement(ctx, intent, receipt, false)
}

func (coordinator *Coordinator) persistCommittedSettlement(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
	receipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
	replayed bool,
) (CommitResultV2, error) {
	settlement, err := domainauthority.NewCommittedMonotonicAdvanceSettlementV2(
		intent,
		receipt,
		func(message []byte) ([]byte, error) { return coordinator.authority.Sign(ctx, message) },
	)
	if err != nil {
		return CommitResultV2{}, errors.Join(ErrCommittedUnsettled, err)
	}
	if err := coordinator.settlements.PutSettlementIfAbsent(ctx, settlement); err != nil {
		return CommitResultV2{}, errors.Join(ErrCommittedUnsettled, classifyReadErrorV2(ctx, err))
	}
	readback, err := coordinator.settlements.ResolveSettlement(ctx, intent.MutationID)
	if err != nil {
		return CommitResultV2{}, errors.Join(ErrCommittedUnsettled, classifyReadErrorV2(ctx, err))
	}
	if !equalSettlementV2(readback, settlement) || !equalSettlementForIntentV2(readback, intent) {
		return CommitResultV2{}, errors.Join(ErrCommittedUnsettled, ErrIntegrity)
	}
	return CommitResultV2{Intent: intent, Settlement: settlement, Replayed: replayed}, nil
}

func (coordinator *Coordinator) freshRecoveryChallenge(ctx context.Context, mutationID string) (string, error) {
	if coordinator == nil || ctx == nil || coordinator.random == nil || !domainsecurity.IsSHA256Hex(mutationID) {
		return "", ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var randomBytes [32]byte
	if _, err := io.ReadFull(coordinator.random, randomBytes[:]); err != nil {
		return "", errors.Join(ErrUnresolved, ErrUnavailable)
	}
	payload := append([]byte("analytix.authority-advance/recover-challenge/v2\x00"), []byte(mutationID)...)
	payload = append(payload, randomBytes[:]...)
	challenge := domainsecurity.SHA256Hex(payload)
	if _, reused := coordinator.challenges[challenge]; reused {
		return "", errors.Join(ErrUnresolved, ErrIntegrity)
	}
	coordinator.challenges[challenge] = struct{}{}
	coordinator.challengeOrder = append(coordinator.challengeOrder, challenge)
	if len(coordinator.challengeOrder) > recoveryChallengeWindowV2 {
		evicted := coordinator.challengeOrder[0]
		coordinator.challengeOrder = coordinator.challengeOrder[1:]
		delete(coordinator.challenges, evicted)
	}
	return challenge, nil
}

func (coordinator *Coordinator) validateIntent(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) error {
	if err := domainauthority.ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return ErrInvalidInput
	}
	if intent.InstallationID != coordinator.installationID || intent.EnrollmentID != coordinator.enrollmentID ||
		intent.Namespace != coordinator.namespace || intent.AuthorityKeyID != coordinator.authorityKeyID ||
		intent.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(coordinator.authorityKey) {
		return ErrInvalidInput
	}
	namespace, err := domainauthority.NamespaceForAdvanceRootV2(intent.Root)
	if err != nil || namespace != coordinator.namespace {
		return ErrInvalidInput
	}
	if err := domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(
		intent.PreviousCheckpoint,
		coordinator.installationID,
		coordinator.enrollmentID,
		coordinator.witnessKeyID,
		coordinator.witnessKey,
	); err != nil {
		return ErrIntegrity
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(
		intent.AdvanceRequest,
		coordinator.installationID,
		coordinator.authorityKeyID,
		coordinator.authorityKey,
	); err != nil {
		return ErrIntegrity
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (coordinator *Coordinator) validateCandidateReadback(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
	persisted bool,
) error {
	switch intent.Root {
	case domainauthority.AdvanceRootThreadRiskV2:
		var expected domainsecurity.ThreadRiskAuthorityIndexV1
		if intent.Transition.ThreadRiskGenesis != nil {
			expected = intent.Transition.ThreadRiskGenesis.FirstIndex
		} else {
			expected = intent.Transition.ThreadRisk.NextIndex
		}
		stored, err := coordinator.riskIndexes.Resolve(ctx, expected.IndexDigest)
		if err != nil {
			if persisted && ctx.Err() == nil {
				return errors.Join(ErrUnresolved, ErrIntegrity, err)
			}
			return classifyReadErrorV2(ctx, err)
		}
		if domainsecurity.ValidateThreadRiskAuthorityIndexForInstallationV1(
			stored, coordinator.installationID, coordinator.authorityKeyID, coordinator.authorityKey,
		) != nil || !equalRiskIndexV1(stored, expected) {
			if persisted {
				return errors.Join(ErrUnresolved, ErrIntegrity)
			}
			return ErrIntegrity
		}
	case domainauthority.AdvanceRootEvidenceGenesisV2,
		domainauthority.AdvanceRootDatasetSnapshotV2,
		domainauthority.AdvanceRootEvidenceRegistryV2,
		domainauthority.AdvanceRootPublicationV2:
		var expected domainevidence.EvidenceAuthorityBundleV1
		if intent.Transition.EvidenceGenesis != nil {
			expected = intent.Transition.EvidenceGenesis.FirstBundle
		} else {
			expected = intent.Transition.EvidenceBundle.NextBundle
		}
		stored, err := coordinator.evidenceBundles.Resolve(ctx, expected.RecordDigest)
		if err != nil {
			if persisted && ctx.Err() == nil {
				return errors.Join(ErrUnresolved, ErrIntegrity, err)
			}
			return classifyReadErrorV2(ctx, err)
		}
		if domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
			stored,
			coordinator.installationID,
			coordinator.enrollmentID,
			coordinator.authorityKeyID,
			coordinator.authorityKey,
		) != nil || !equalEvidenceBundleV1(stored, expected) {
			if persisted {
				return errors.Join(ErrUnresolved, ErrIntegrity)
			}
			return ErrIntegrity
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func (coordinator *Coordinator) persistIntentExact(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) error {
	if existing, err := coordinator.intents.ResolveIntent(ctx, intent.MutationID); err == nil {
		if !equalIntentV2(existing, intent) {
			return ErrIntegrity
		}
		if err := coordinator.validateIntent(ctx, existing); err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return contextErr
			}
			return ErrIntegrity
		}
		return nil
	} else if !errors.Is(err, journalport.ErrNotFound) {
		return classifyReadErrorV2(ctx, err)
	}
	// A terminal record without its write-ahead intent is deletion/rollback
	// evidence. Never reconstruct the missing authority from caller input.
	if _, err := coordinator.settlements.ResolveSettlement(ctx, intent.MutationID); err == nil {
		return ErrIntegrity
	} else if !errors.Is(err, journalport.ErrNotFound) {
		return classifyReadErrorV2(ctx, err)
	}
	if err := coordinator.intents.PutIntentIfAbsent(ctx, intent); err != nil {
		return classifyReadErrorV2(ctx, err)
	}
	readback, err := coordinator.intents.ResolveIntent(ctx, intent.MutationID)
	if err != nil {
		return classifyReadErrorV2(ctx, err)
	}
	if !equalIntentV2(readback, intent) {
		return ErrIntegrity
	}
	if err := coordinator.validateIntent(ctx, readback); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return ErrIntegrity
	}
	return nil
}

func (coordinator *Coordinator) persistedIntentExact(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) (bool, error) {
	existing, err := coordinator.intents.ResolveIntent(ctx, intent.MutationID)
	if errors.Is(err, journalport.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, classifyReadErrorV2(ctx, err)
	}
	if !equalIntentV2(existing, intent) {
		return false, ErrIntegrity
	}
	if err := coordinator.validateIntent(ctx, existing); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		return false, ErrIntegrity
	}
	return true, nil
}

func classifyReadErrorV2(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, journalport.ErrConflict) || errors.Is(err, journalport.ErrCorrupt) {
		return errors.Join(ErrIntegrity, err)
	}
	return errors.Join(ErrUnavailable, err)
}

func (coordinator *Coordinator) acquire(ctx context.Context) error {
	if coordinator == nil || ctx == nil || coordinator.admission == nil {
		return ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case coordinator.admission <- struct{}{}:
		if err := ctx.Err(); err != nil {
			coordinator.release()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (coordinator *Coordinator) release() {
	<-coordinator.admission
}

func equalIntentV2(left, right domainauthority.MonotonicAdvanceIntentV2) bool {
	leftBody, leftErr := domainauthority.MonotonicAdvanceIntentV2Bytes(left)
	rightBody, rightErr := domainauthority.MonotonicAdvanceIntentV2Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func equalSettlementV2(left, right domainauthority.MonotonicAdvanceSettlementV2) bool {
	leftBody, leftErr := domainauthority.MonotonicAdvanceSettlementV2Bytes(left)
	rightBody, rightErr := domainauthority.MonotonicAdvanceSettlementV2Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func equalSettlementForIntentV2(
	settlement domainauthority.MonotonicAdvanceSettlementV2,
	intent domainauthority.MonotonicAdvanceIntentV2,
) bool {
	return settlement.Kind == domainauthority.MonotonicAdvanceSettlementCommittedV2 &&
		domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, nil) == nil
}

func equalRiskIndexV1(left, right domainsecurity.ThreadRiskAuthorityIndexV1) bool {
	leftBody, leftErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(left)
	rightBody, rightErr := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func equalEvidenceBundleV1(left, right domainevidence.EvidenceAuthorityBundleV1) bool {
	leftBody, leftErr := domainevidence.EvidenceAuthorityBundleV1Bytes(left)
	rightBody, rightErr := domainevidence.EvidenceAuthorityBundleV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
