package reportpublication

import (
	"context"
	"crypto/ed25519"
	"errors"
	"reflect"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

const publicationEffectReadbackTimeoutV1 = 5 * time.Second

type CommitSelectorConfigV1 struct {
	InstallationID string
	EnrollmentID   string
	Authority      finalauthorityport.Authority
	WitnessKeyID   string
	WitnessKey     []byte
	HeadReader     evidenceauthorityport.FreshHeadReader
	Bundles        evidenceauthorityport.BundleStore
	Observations   evidenceauthorityport.ObservationStore
	Selections     publicationport.CommitSelectionStore
	Commits        publicationport.CommitReceiptStore
}

func selectAndSignPublicationCommitV1(
	ctx context.Context,
	attempt domainpublication.PublicationAttemptV1,
	candidate domainpublication.PublicationReceiptV1,
	index domainpublication.PublicationIndexV1,
	settlementDigest string,
	proposed *evidenceauthorityport.FreshHead,
	config CommitSelectorConfigV1,
) (domainpublication.PublicationCommitReceiptV1, bool, error) {
	if ctx == nil || validateCommitSelectorConfigV1(config) != nil {
		return domainpublication.PublicationCommitReceiptV1{}, false, ErrPublicationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainpublication.PublicationCommitReceiptV1{}, false, err
	}
	if domainpublication.ValidatePublicationAttemptV1(attempt) != nil ||
		domainpublication.ValidatePublicationIndexReceiptV1(index, candidate) != nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(settlementDigest)) {
		return domainpublication.PublicationCommitReceiptV1{}, false, ErrPublicationIntegrity
	}
	selectionID := domainpublication.PublicationCommitSelectionIDV1(
		config.InstallationID, config.EnrollmentID, attempt.AttemptID,
	)
	selection, err := config.Selections.Resolve(ctx, selectionID)
	if errors.Is(err, publicationport.ErrNotFound) {
		selection, err = competePublicationCommitSelectionV1(
			ctx, attempt, candidate, index, settlementDigest, proposed, config,
		)
	} else if err != nil {
		return domainpublication.PublicationCommitReceiptV1{}, false, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	if err != nil {
		return domainpublication.PublicationCommitReceiptV1{}, false, err
	}
	input, err := resolvePublicationCommitSelectionInputV1(
		ctx, selection, attempt, candidate, index, settlementDigest, config,
	)
	if err != nil {
		return domainpublication.PublicationCommitReceiptV1{}, false, err
	}
	commit, err := domainpublication.NewPublicationCommitReceiptV1(
		input,
		func(message []byte) ([]byte, error) { return config.Authority.Sign(ctx, message) },
	)
	if err != nil || domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, commit) != nil {
		return domainpublication.PublicationCommitReceiptV1{}, false, ErrPublicationIntegrity
	}
	if stored, resolveErr := config.Commits.Resolve(ctx, commit.RecordDigest); resolveErr == nil {
		if !reflect.DeepEqual(stored, commit) ||
			domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, stored) != nil {
			return domainpublication.PublicationCommitReceiptV1{}, false, ErrPublicationIntegrity
		}
		return commit, false, nil
	} else if !errors.Is(resolveErr, publicationport.ErrNotFound) {
		return domainpublication.PublicationCommitReceiptV1{}, false, errors.Join(ErrPublicationRestartUnresolved, resolveErr)
	}
	putErr := config.Commits.PutIfAbsent(ctx, commit)
	readbackCtx, cancel := detachedPublicationEffectReadbackContextV1(ctx)
	defer cancel()
	stored, resolveErr := config.Commits.Resolve(readbackCtx, commit.RecordDigest)
	if resolveErr != nil || !reflect.DeepEqual(stored, commit) ||
		domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, stored) != nil {
		if putErr != nil || resolveErr != nil {
			return domainpublication.PublicationCommitReceiptV1{}, false, errors.Join(
				ErrPublicationRestartUnresolved, putErr, resolveErr,
			)
		}
		return domainpublication.PublicationCommitReceiptV1{}, false, ErrPublicationIntegrity
	}
	return commit, true, nil
}

func competePublicationCommitSelectionV1(
	ctx context.Context,
	attempt domainpublication.PublicationAttemptV1,
	candidate domainpublication.PublicationReceiptV1,
	index domainpublication.PublicationIndexV1,
	settlementDigest string,
	proposed *evidenceauthorityport.FreshHead,
	config CommitSelectorConfigV1,
) (domainpublication.PublicationCommitSelectionV1, error) {
	head := evidenceauthorityport.FreshHead{}
	if proposed != nil {
		head = *proposed
	} else {
		var err error
		head, err = config.HeadReader.ObserveFresh(ctx)
		if err != nil {
			return domainpublication.PublicationCommitSelectionV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
		}
	}
	if validateFreshPublicationHeadForSelectorV1(head, config) != nil ||
		head.Bundle.RecordDigest != attempt.NextEvidenceBundleDigest {
		return domainpublication.PublicationCommitSelectionV1{}, ErrPublicationIntegrity
	}
	if err := persistPublicationObservationExactV1(ctx, config.Observations, head); err != nil {
		return domainpublication.PublicationCommitSelectionV1{}, err
	}
	previous, err := config.Bundles.Resolve(ctx, attempt.ExpectedEvidenceBundleDigest)
	if err != nil {
		return domainpublication.PublicationCommitSelectionV1{}, commitSelectorResolveErrorV1(err)
	}
	if previous.RecordDigest != attempt.ExpectedEvidenceBundleDigest {
		return domainpublication.PublicationCommitSelectionV1{}, ErrPublicationIntegrity
	}
	input := publicationCommitInputV1(previous, head.Bundle, head.Request, head.Observation, candidate, index, config)
	proposal, err := domainpublication.NewPublicationCommitSelectionV1(
		domainpublication.PublicationCommitSelectionInputV1{
			Attempt: attempt, SettlementDigest: settlementDigest, CommitInput: input,
		},
		func(message []byte) ([]byte, error) { return config.Authority.Sign(ctx, message) },
	)
	if err != nil {
		return domainpublication.PublicationCommitSelectionV1{}, err
	}
	_, createErr := config.Selections.CreateExclusive(ctx, proposal)
	readbackCtx, cancel := detachedPublicationEffectReadbackContextV1(ctx)
	defer cancel()
	winner, resolveErr := config.Selections.Resolve(readbackCtx, proposal.SelectionID)
	if resolveErr != nil {
		return domainpublication.PublicationCommitSelectionV1{}, errors.Join(
			ErrPublicationRestartUnresolved, createErr, resolveErr,
		)
	}
	if _, err := resolvePublicationCommitSelectionInputV1(
		readbackCtx, winner, attempt, candidate, index, settlementDigest, config,
	); err != nil {
		return domainpublication.PublicationCommitSelectionV1{}, err
	}
	return winner, nil
}

func resolvePublicationCommitSelectionInputV1(
	ctx context.Context,
	selection domainpublication.PublicationCommitSelectionV1,
	attempt domainpublication.PublicationAttemptV1,
	candidate domainpublication.PublicationReceiptV1,
	index domainpublication.PublicationIndexV1,
	settlementDigest string,
	config CommitSelectorConfigV1,
) (domainpublication.PublicationCommitReceiptInputV1, error) {
	if domainpublication.ValidatePublicationCommitSelectionMaterialsV1(
		selection, attempt, candidate, index, settlementDigest,
	) != nil {
		return domainpublication.PublicationCommitReceiptInputV1{}, ErrPublicationIntegrity
	}
	keyID, publicKey, signature, err := domainpublication.PublicationCommitSelectionAuthorityMaterialV1(selection)
	if err != nil || config.Authority.VerifyTrusted(
		ctx, keyID, publicKey, domainpublication.PublicationCommitSelectionSigningBytesV1(selection), signature,
	) != nil || keyID != config.Authority.KeyID() || !reflect.DeepEqual(publicKey, config.Authority.PublicKey()) {
		return domainpublication.PublicationCommitReceiptInputV1{}, ErrPublicationIntegrity
	}
	previous, err := config.Bundles.Resolve(ctx, selection.PreviousEvidenceBundleDigest)
	if err != nil {
		return domainpublication.PublicationCommitReceiptInputV1{}, commitSelectorResolveErrorV1(err)
	}
	if previous.RecordDigest != selection.PreviousEvidenceBundleDigest {
		return domainpublication.PublicationCommitReceiptInputV1{}, ErrPublicationIntegrity
	}
	committed, err := config.Bundles.Resolve(ctx, selection.CommittedEvidenceBundleDigest)
	if err != nil {
		return domainpublication.PublicationCommitReceiptInputV1{}, commitSelectorResolveErrorV1(err)
	}
	if committed.RecordDigest != selection.CommittedEvidenceBundleDigest {
		return domainpublication.PublicationCommitReceiptInputV1{}, ErrPublicationIntegrity
	}
	observation, err := config.Observations.Resolve(ctx, selection.ObservationDigest)
	if err != nil {
		return domainpublication.PublicationCommitReceiptInputV1{}, commitSelectorResolveErrorV1(err)
	}
	if !reflect.DeepEqual(observation.Bundle, committed) ||
		observation.Request.RequestDigest != selection.ObserveRequestDigest ||
		observation.Observation.ObservationDigest != selection.ObservationDigest {
		return domainpublication.PublicationCommitReceiptInputV1{}, ErrPublicationIntegrity
	}
	input := publicationCommitInputV1(
		previous, committed, observation.Request, observation.Observation, candidate, index, config,
	)
	if domainpublication.ValidatePublicationCommitSelectionExactV1(
		selection,
		domainpublication.PublicationCommitSelectionInputV1{
			Attempt: attempt, SettlementDigest: settlementDigest, CommitInput: input,
		},
	) != nil {
		return domainpublication.PublicationCommitReceiptInputV1{}, ErrPublicationIntegrity
	}
	return input, nil
}

func commitSelectorResolveErrorV1(err error) error {
	if errors.Is(err, publicationport.ErrNotFound) {
		return ErrPublicationIntegrity
	}
	return errors.Join(ErrPublicationRestartUnresolved, err)
}

func publicationCommitInputV1(
	previous, committedBundle domainevidence.EvidenceAuthorityBundleV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	candidate domainpublication.PublicationReceiptV1,
	index domainpublication.PublicationIndexV1,
	config CommitSelectorConfigV1,
) domainpublication.PublicationCommitReceiptInputV1 {
	return domainpublication.PublicationCommitReceiptInputV1{
		PreviousBundle: previous, CommittedBundle: committedBundle,
		ObserveRequest: request, Observation: observation, Candidate: candidate, Index: index,
		InstallationID: config.InstallationID, EnrollmentID: config.EnrollmentID,
		AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
		WitnessKeyID: config.WitnessKeyID, WitnessPublicKey: config.WitnessKey,
	}
}

func validateFreshPublicationHeadForSelectorV1(head evidenceauthorityport.FreshHead, config CommitSelectorConfigV1) error {
	if !head.HasBundle || config.Authority == nil ||
		domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
			head.Bundle, config.InstallationID, config.EnrollmentID, config.Authority.KeyID(), config.Authority.PublicKey(),
		) != nil || domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(head.Bundle, head.Observation.Checkpoint) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			head.Observation, head.Request, config.InstallationID, config.Authority.KeyID(), config.Authority.PublicKey(),
			config.EnrollmentID, config.WitnessKeyID, config.WitnessKey,
		) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func validateCommitSelectorConfigV1(config CommitSelectorConfigV1) error {
	publicKey := []byte(nil)
	keyID := ""
	if config.Authority != nil {
		publicKey = config.Authority.PublicKey()
		keyID = strings.TrimSpace(config.Authority.KeyID())
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(config.InstallationID)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(config.EnrollmentID)) || config.Authority == nil ||
		len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		len(config.WitnessKey) != ed25519.PublicKeySize || strings.TrimSpace(config.WitnessKeyID) != domainsecurity.SHA256Hex(config.WitnessKey) ||
		config.HeadReader == nil || config.Bundles == nil || config.Observations == nil || config.Selections == nil || config.Commits == nil {
		return ErrPublicationUnavailable
	}
	return nil
}

func detachedPublicationEffectReadbackContextV1(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), publicationEffectReadbackTimeoutV1)
}
