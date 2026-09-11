package turnterminal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
)

type ProviderTurnCloser interface {
	CloseTurn(context.Context, domainsecurity.TurnSecurityContext,
		domaincachetelemetry.ProviderTurnTerminalReasonV1, time.Time) (domaincachetelemetry.ProviderTurnClosureV1, error)
}

type ProviderTurnClosureObserver interface {
	ObserveTurnClosureV1(context.Context, domainsecurity.TurnSecurityContext) (domaincachetelemetry.ProviderTurnClosureV1, bool, error)
	VisitTurnClosuresV1(context.Context, func(domaincachetelemetry.ProviderTurnClosureV1) error) error
}

type ProviderTurnAuthority interface {
	ProviderTurnCloser
	ProviderTurnClosureObserver
}

type Coordinator struct {
	mu                    sync.Mutex
	authority             finalauthorityport.Authority
	privateFinals         finalauthorityport.PrivateFinalStore
	terminals             turnterminalstoreport.Store
	providerTurns         ProviderTurnAuthority
	restartPreserved      *restartPreservationV1
	restartEffectsStarted bool
}

type CommitInputV1 struct {
	CompletionStore appturn.AcceptedFinalCompletionStore
	CASReader       finalauthorityport.AcceptedFinalCASReader
	PrivateFinal    domainevidence.PrivateAcceptedFinalRecord
	FactAuthority   appturn.FactFinalMutationAuthority
}

type CommitResultV1 struct {
	Persistence              appturn.PersistAcceptedFinalResult
	Intent                   domainturnterminal.TurnTerminalIntentV1
	ProviderClosure          domaincachetelemetry.ProviderTurnClosureV1
	PublicObservation        domainevidence.AcceptedFinalCASObservationV1
	AcceptedFinalDisposition domainevidence.AcceptedFinalDispositionRecord
	TerminalDisposition      domainturnterminal.TurnTerminalDispositionV1
}

func NewCoordinator(authority finalauthorityport.Authority, privateFinals finalauthorityport.PrivateFinalStore,
	terminals turnterminalstoreport.Store, providerTurns ProviderTurnAuthority) (*Coordinator, error) {
	if authority == nil || privateFinals == nil || terminals == nil || providerTurns == nil {
		return nil, errors.New("turn terminal coordinator authority is incomplete")
	}
	return &Coordinator{
		authority: authority, privateFinals: privateFinals, terminals: terminals,
		providerTurns: providerTurns,
	}, nil
}

// CommitV1 is the only application transaction allowed to move a gated case
// final from private authority into the canonical public turn. The order is:
// terminal intent -> provider closure -> public CAS -> accepted-final CAS
// disposition -> terminal disposition. Any earlier failure prevents every
// later mutation.
func (coordinator *Coordinator) CommitV1(ctx context.Context, input CommitInputV1) (CommitResultV1, error) {
	if coordinator == nil || coordinator.authority == nil || coordinator.privateFinals == nil || coordinator.terminals == nil ||
		coordinator.providerTurns == nil || input.CompletionStore == nil || input.CASReader == nil {
		return CommitResultV1{}, errors.New("turn terminal coordinator is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CommitResultV1{}, err
	}
	privateFinal := input.PrivateFinal
	if err := verifyFactFinalMutationAuthorityV1(privateFinal, input.FactAuthority); err != nil {
		return CommitResultV1{}, err
	}
	publication, err := appturn.BuildAcceptedFinalPublicationPlan(
		privateFinal.AcceptedFinal, privateFinal.RenderedText, privateFinal.PublicationIntent,
	)
	if err != nil {
		return CommitResultV1{}, err
	}
	acceptedAt, err := time.Parse(time.RFC3339Nano, privateFinal.AcceptedFinal.AcceptedAt)
	if err != nil || acceptedAt.Location() != time.UTC || acceptedAt.Format(time.RFC3339Nano) != privateFinal.AcceptedFinal.AcceptedAt {
		return CommitResultV1{}, errors.New("turn terminal accepted time is not canonical UTC")
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.restartEffectsStarted = true
	if coordinator.restartPreserved.ownsThread(privateFinal.SecurityContext.ThreadID) {
		return CommitResultV1{}, ErrRestartPreserved
	}
	storedPrivate, err := coordinator.privateFinals.Resolve(ctx, privateFinal.AcceptedFinal.RecordDigest)
	if err != nil || storedPrivate.StoreDigest != privateFinal.StoreDigest {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal private final readback is unavailable"), err)
	}
	if err := coordinator.verifyAcceptedFinalTrustedV1(ctx, privateFinal.AcceptedFinal); err != nil {
		return CommitResultV1{}, err
	}
	preObservation, err := input.CASReader.ReadAcceptedFinalCASObservation(
		ctx, privateFinal.SecurityContext.ThreadID, privateFinal.SecurityContext.TurnID,
	)
	if err != nil || !terminalObservationMatchesContextV1(preObservation, privateFinal.SecurityContext) {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal pre-CAS observation is invalid"), err)
	}
	if preObservation.HasWinner && preObservation.Winner.RecordDigest != privateFinal.AcceptedFinal.RecordDigest {
		return CommitResultV1{}, errors.New("turn terminal context already has another public winner")
	}
	if preObservation.HasWinner {
		if _, intentErr := coordinator.terminals.ReadIntent(ctx, privateFinal.SecurityContext.ContextDigest); errors.Is(intentErr, turnterminalstoreport.ErrNotFound) {
			return CommitResultV1{}, errors.New("turn terminal public winner predates terminal intent authority")
		} else if intentErr != nil {
			return CommitResultV1{}, errors.Join(errors.New("turn terminal intent preflight failed"), intentErr)
		}
	}
	if !preObservation.HasWinner && !turnTerminalCASStatusIsActiveV1(preObservation.Status) {
		return CommitResultV1{}, errors.New("turn terminal context is no longer active and has no public winner")
	}
	intent, err := coordinator.ensureIntentV1(ctx, privateFinal, publication.EventManifestDigest, input.FactAuthority)
	if err != nil {
		return CommitResultV1{}, err
	}
	closure, err := coordinator.providerTurns.CloseTurn(ctx, privateFinal.SecurityContext, intent.TerminalReasonCode, acceptedAt)
	if err != nil {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal provider closure failed"), err)
	}
	if err := coordinator.verifyProviderClosureV1(ctx, intent, closure); err != nil {
		return CommitResultV1{}, err
	}
	persistence, persistErr := appturn.PersistAcceptedFinalTerminal(appturn.PersistAcceptedFinalInput{
		Store: input.CompletionStore, ThreadID: privateFinal.SecurityContext.ThreadID,
		TurnID: privateFinal.SecurityContext.TurnID, RenderedText: privateFinal.RenderedText,
		AcceptedFinal: privateFinal.AcceptedFinal, PublicationIntent: privateFinal.PublicationIntent,
		PrivateFinal: privateFinal, FactAuthority: input.FactAuthority,
	})
	if persistence.Publication.EventManifestDigest != publication.EventManifestDigest {
		return CommitResultV1{}, errors.Join(persistErr, errors.New("turn terminal public publication plan changed"))
	}
	observation, observationErr := input.CASReader.ReadAcceptedFinalCASObservation(
		ctx, privateFinal.SecurityContext.ThreadID, privateFinal.SecurityContext.TurnID,
	)
	if observationErr != nil || !terminalObservationMatchesContextV1(observation, privateFinal.SecurityContext) ||
		!observation.HasWinner || observation.Winner.RecordDigest != privateFinal.AcceptedFinal.RecordDigest {
		return CommitResultV1{}, errors.Join(persistErr, observationErr, errors.New("turn terminal public CAS lacks the exact accepted winner"))
	}
	acceptedDisposition, err := coordinator.ensureAcceptedFinalDispositionV1(
		ctx, privateFinal, publication.EventManifestDigest, observation, closure,
	)
	if err != nil {
		return CommitResultV1{}, errors.Join(persistErr, err)
	}
	terminalDisposition, err := coordinator.ensureTerminalDispositionV1(ctx, intent, closure, acceptedDisposition)
	if err != nil {
		return CommitResultV1{}, errors.Join(persistErr, err)
	}
	return CommitResultV1{
		Persistence: persistence, Intent: intent, ProviderClosure: closure, PublicObservation: observation,
		AcceptedFinalDisposition: acceptedDisposition, TerminalDisposition: terminalDisposition,
	}, nil
}

// ResolveCommittedV1 reconstructs an already-committed terminal authority
// without creating a new contender or mutating any terminal store. It is used
// to reconcile a public event tail after the terminal CAS already won.
func (coordinator *Coordinator) ResolveCommittedV1(ctx context.Context, input CommitInputV1) (CommitResultV1, error) {
	if coordinator == nil || coordinator.authority == nil || coordinator.privateFinals == nil || coordinator.terminals == nil ||
		coordinator.providerTurns == nil || input.CompletionStore == nil || input.CASReader == nil {
		return CommitResultV1{}, errors.New("turn terminal coordinator is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CommitResultV1{}, err
	}
	privateFinal := input.PrivateFinal
	if err := verifyFactFinalMutationAuthorityV1(privateFinal, input.FactAuthority); err != nil {
		return CommitResultV1{}, err
	}
	publication, err := appturn.BuildAcceptedFinalPublicationPlan(
		privateFinal.AcceptedFinal, privateFinal.RenderedText, privateFinal.PublicationIntent,
	)
	if err != nil {
		return CommitResultV1{}, err
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.restartEffectsStarted = true
	if coordinator.restartPreserved.ownsThread(privateFinal.SecurityContext.ThreadID) {
		return CommitResultV1{}, ErrRestartPreserved
	}
	storedPrivate, err := coordinator.privateFinals.Resolve(ctx, privateFinal.AcceptedFinal.RecordDigest)
	if err != nil || storedPrivate.StoreDigest != privateFinal.StoreDigest {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal private final readback is unavailable"), err)
	}
	if err := coordinator.verifyAcceptedFinalTrustedV1(ctx, privateFinal.AcceptedFinal); err != nil {
		return CommitResultV1{}, err
	}
	observation, err := input.CASReader.ReadAcceptedFinalCASObservation(
		ctx, privateFinal.SecurityContext.ThreadID, privateFinal.SecurityContext.TurnID,
	)
	expectedStatus, statusOK := domainevidence.FinalAnswerTerminalStatus(privateFinal.AcceptedFinal.TerminalReason)
	if err != nil || !statusOK || !terminalObservationMatchesContextV1(observation, privateFinal.SecurityContext) ||
		observation.Status != expectedStatus || !observation.HasWinner ||
		observation.Winner.RecordDigest != privateFinal.AcceptedFinal.RecordDigest {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal committed CAS observation is invalid"), err)
	}
	intent, err := coordinator.terminals.ReadIntent(ctx, privateFinal.SecurityContext.ContextDigest)
	if err != nil || coordinator.verifyIntentV1(ctx, intent, privateFinal, publication.EventManifestDigest, input.FactAuthority) != nil {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal committed intent is invalid"), err)
	}
	closure, found, err := coordinator.providerTurns.ObserveTurnClosureV1(ctx, privateFinal.SecurityContext)
	if err != nil || !found || coordinator.verifyProviderClosureV1(ctx, intent, closure) != nil {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal committed provider closure is invalid"), err)
	}
	acceptedDisposition, err := coordinator.privateFinals.ResolveDisposition(ctx, privateFinal.AcceptedFinal.RecordDigest)
	if err != nil || coordinator.verifyAcceptedFinalDispositionV1(
		ctx, privateFinal, publication.EventManifestDigest, observation, closure, acceptedDisposition, input.FactAuthority,
	) != nil {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal committed accepted-final disposition is invalid"), err)
	}
	terminalDisposition, err := coordinator.terminals.ReadDisposition(ctx, privateFinal.SecurityContext.ContextDigest)
	if err != nil || coordinator.verifyTerminalDispositionV1(ctx, terminalDisposition, intent, closure, acceptedDisposition) != nil {
		return CommitResultV1{}, errors.Join(errors.New("turn terminal committed disposition is invalid"), err)
	}
	return CommitResultV1{
		Persistence: appturn.PersistAcceptedFinalResult{
			CompletionRecord: publication.Completion, AcceptedFinal: privateFinal.AcceptedFinal,
			Publication: publication, Changed: false, Status: observation.Status,
		},
		Intent: intent, ProviderClosure: closure, PublicObservation: observation,
		AcceptedFinalDisposition: acceptedDisposition, TerminalDisposition: terminalDisposition,
	}, nil
}

func verifyFactFinalMutationAuthorityV1(
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	authority appturn.FactFinalMutationAuthority,
) error {
	if err := appturn.ValidatePrivateAcceptedFinalMutationAuthority(privateFinal, authority); err != nil {
		return errors.Join(errors.New("turn terminal private final lacks mutation authority"), err)
	}
	return nil
}

func (coordinator *Coordinator) ensureIntentV1(ctx context.Context, privateFinal domainevidence.PrivateAcceptedFinalRecord,
	eventManifestDigest string, factAuthority appturn.FactFinalMutationAuthority) (domainturnterminal.TurnTerminalIntentV1, error) {
	contextDigest := privateFinal.SecurityContext.ContextDigest
	intent, err := coordinator.terminals.ReadIntent(ctx, contextDigest)
	if errors.Is(err, turnterminalstoreport.ErrNotFound) {
		input := domainturnterminal.TurnTerminalIntentInputV1{
			PrivateFinal: privateFinal, EventManifestDigest: eventManifestDigest,
			AuthorityKeyID: coordinator.authority.KeyID(), AuthorityPublicKey: coordinator.authority.PublicKey(),
		}
		sign := func(message []byte) ([]byte, error) { return coordinator.authority.Sign(ctx, message) }
		if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
			intent, err = domainturnterminal.NewTurnTerminalIntentWithFactWitnessV1(
				input, factFinalReplayVerifierV1(factAuthority), sign,
			)
		} else {
			intent, err = domainturnterminal.NewTurnTerminalIntentV1(input, sign)
		}
		if err != nil {
			return domainturnterminal.TurnTerminalIntentV1{}, err
		}
		if err := coordinator.terminals.PutIntentIfAbsent(ctx, intent); err != nil {
			return domainturnterminal.TurnTerminalIntentV1{}, err
		}
		intent, err = coordinator.terminals.ReadIntent(ctx, contextDigest)
	}
	if err != nil || validateTurnTerminalIntentMutationAuthorityV1(intent, privateFinal, factAuthority) != nil ||
		intent.EventManifestDigest != eventManifestDigest {
		return domainturnterminal.TurnTerminalIntentV1{}, errors.Join(errors.New("turn terminal intent readback is inconsistent"), err)
	}
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalIntentV1AuthorityMaterial(intent)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainturnterminal.TurnTerminalIntentV1SigningBytes(intent), signature) != nil {
		return domainturnterminal.TurnTerminalIntentV1{}, errors.New("turn terminal intent is not installation-trusted")
	}
	return intent, nil
}

func (coordinator *Coordinator) ensureAcceptedFinalDispositionV1(ctx context.Context,
	privateFinal domainevidence.PrivateAcceptedFinalRecord, eventManifestDigest string,
	observation domainevidence.AcceptedFinalCASObservationV1,
	closure domaincachetelemetry.ProviderTurnClosureV1) (domainevidence.AcceptedFinalDispositionRecord, error) {
	acceptedFinal := privateFinal.AcceptedFinal
	closedAt, err := time.Parse(time.RFC3339Nano, closure.ClosedAt)
	if err != nil {
		return domainevidence.AcceptedFinalDispositionRecord{}, err
	}
	expected, err := domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: acceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: eventManifestDigest, DecidedAt: closedAt,
		AuthorityKeyID: coordinator.authority.KeyID(), AuthorityPublicKey: coordinator.authority.PublicKey(),
	}, observation, domainevidence.AcceptedFinalDecisionSamePublicWinner,
		func(message []byte) ([]byte, error) { return coordinator.authority.Sign(ctx, message) })
	if err != nil {
		return domainevidence.AcceptedFinalDispositionRecord{}, err
	}
	if err := coordinator.privateFinals.PutDispositionIfAbsent(ctx, expected); err != nil {
		return domainevidence.AcceptedFinalDispositionRecord{}, err
	}
	disposition, err := coordinator.privateFinals.ResolveDisposition(ctx, acceptedFinal.RecordDigest)
	if err != nil || disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		disposition.RecordDigest != expected.RecordDigest ||
		disposition.State != domainevidence.AcceptedFinalCommitted ||
		disposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		disposition.AcceptedFinalDigest != acceptedFinal.RecordDigest || disposition.WinnerDigest != acceptedFinal.RecordDigest ||
		disposition.EventManifestDigest != eventManifestDigest || disposition.TurnCASDigest != observation.TurnProjectionSHA256 {
		return domainevidence.AcceptedFinalDispositionRecord{}, errors.Join(errors.New("accepted-final committed disposition readback is inconsistent"), err)
	}
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalDispositionAuthorityMaterial(disposition)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainevidence.AcceptedFinalDispositionSigningBytes(disposition), signature) != nil {
		return domainevidence.AcceptedFinalDispositionRecord{}, errors.New("accepted-final disposition is not installation-trusted")
	}
	return disposition, nil
}

func (coordinator *Coordinator) ensureTerminalDispositionV1(ctx context.Context,
	intent domainturnterminal.TurnTerminalIntentV1, closure domaincachetelemetry.ProviderTurnClosureV1,
	acceptedDisposition domainevidence.AcceptedFinalDispositionRecord) (domainturnterminal.TurnTerminalDispositionV1, error) {
	contextDigest := intent.SecurityContext.ContextDigest
	disposition, err := coordinator.terminals.ReadDisposition(ctx, contextDigest)
	if errors.Is(err, turnterminalstoreport.ErrNotFound) {
		disposition, err = domainturnterminal.NewTurnTerminalDispositionV1(domainturnterminal.TurnTerminalDispositionInputV1{
			Intent: intent, ProviderClosure: closure, AcceptedFinalDisposition: acceptedDisposition,
			AuthorityKeyID: coordinator.authority.KeyID(), AuthorityPublicKey: coordinator.authority.PublicKey(),
		}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(ctx, message) })
		if err != nil {
			return domainturnterminal.TurnTerminalDispositionV1{}, err
		}
		if err := coordinator.terminals.PutDispositionIfAbsent(ctx, disposition); err != nil {
			return domainturnterminal.TurnTerminalDispositionV1{}, err
		}
		disposition, err = coordinator.terminals.ReadDisposition(ctx, contextDigest)
	}
	if err != nil || domainturnterminal.ValidateTurnTerminalDispositionForAuthoritiesV1(
		disposition, intent, closure, acceptedDisposition,
	) != nil {
		return domainturnterminal.TurnTerminalDispositionV1{}, errors.Join(errors.New("turn terminal disposition readback is inconsistent"), err)
	}
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalDispositionV1AuthorityMaterial(disposition)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainturnterminal.TurnTerminalDispositionV1SigningBytes(disposition), signature) != nil {
		return domainturnterminal.TurnTerminalDispositionV1{}, errors.New("turn terminal disposition is not installation-trusted")
	}
	return disposition, nil
}

func (coordinator *Coordinator) verifyAcceptedFinalTrustedV1(ctx context.Context, record domainevidence.AcceptedFinalRecord) error {
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainevidence.AcceptedFinalSigningBytes(record), signature) != nil {
		return errors.New("turn terminal accepted final is not installation-trusted")
	}
	return nil
}

func (coordinator *Coordinator) verifyIntentV1(
	ctx context.Context,
	intent domainturnterminal.TurnTerminalIntentV1,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	eventManifestDigest string,
	factAuthority appturn.FactFinalMutationAuthority,
) error {
	if validateTurnTerminalIntentMutationAuthorityV1(intent, privateFinal, factAuthority) != nil ||
		intent.EventManifestDigest != eventManifestDigest {
		return errors.New("turn terminal intent does not bind the exact publication")
	}
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalIntentV1AuthorityMaterial(intent)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainturnterminal.TurnTerminalIntentV1SigningBytes(intent), signature) != nil {
		return errors.New("turn terminal intent is not installation-trusted")
	}
	return nil
}

func validateTurnTerminalIntentMutationAuthorityV1(
	intent domainturnterminal.TurnTerminalIntentV1,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) error {
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		if factAuthority == nil {
			return errors.New("fact terminal intent authority is unavailable")
		}
		return domainturnterminal.ValidateTurnTerminalIntentForPrivateFinalWithFactWitnessV1(
			intent,
			privateFinal,
			factFinalReplayVerifierV1(factAuthority),
		)
	}
	if factAuthority != nil {
		return errors.New("boundary terminal intent carried fact authority")
	}
	return domainturnterminal.ValidateTurnTerminalIntentForPrivateFinalV1(intent, privateFinal)
}

func factFinalReplayVerifierV1(authority appturn.FactFinalMutationAuthority) domainevidence.FactFinalWitnessReplayVerifierV1 {
	if authority == nil {
		return nil
	}
	return func(record domainevidence.PrivateAcceptedFinalRecord) error {
		return authority.UseExact(record, func() error { return nil })
	}
}

func (coordinator *Coordinator) verifyIntentAuditV1(
	ctx context.Context,
	intent domainturnterminal.TurnTerminalIntentV1,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	eventManifestDigest string,
) error {
	if domainturnterminal.ValidateTurnTerminalIntentForPrivateFinalAuditV1(intent, privateFinal) != nil ||
		intent.EventManifestDigest != eventManifestDigest || eventManifestDigest == "" {
		return errors.New("turn terminal audit intent does not bind the exact historical publication")
	}
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalIntentV1AuthorityMaterial(intent)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainturnterminal.TurnTerminalIntentV1SigningBytes(intent), signature) != nil {
		return errors.New("turn terminal audit intent is not installation-trusted")
	}
	return nil
}

func (coordinator *Coordinator) verifyAcceptedFinalDispositionV1(
	ctx context.Context,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	eventManifestDigest string,
	observation domainevidence.AcceptedFinalCASObservationV1,
	closure domaincachetelemetry.ProviderTurnClosureV1,
	disposition domainevidence.AcceptedFinalDispositionRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) error {
	acceptedFinal := privateFinal.AcceptedFinal
	if appturn.ValidatePrivateAcceptedFinalMutationAuthority(privateFinal, factAuthority) != nil ||
		domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil ||
		domainevidence.ValidateAcceptedFinalDispositionRecord(disposition) != nil ||
		!observation.HasWinner || observation.Winner.RecordDigest != acceptedFinal.RecordDigest ||
		disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		disposition.State != domainevidence.AcceptedFinalCommitted ||
		disposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		disposition.AcceptedFinalDigest != acceptedFinal.RecordDigest ||
		disposition.PrivateRecordDigest != acceptedFinal.PrivateRecordDigest ||
		disposition.ThreadID != privateFinal.SecurityContext.ThreadID ||
		disposition.TurnID != privateFinal.SecurityContext.TurnID ||
		disposition.EventManifestDigest != eventManifestDigest ||
		disposition.TurnCASDigest != observation.TurnProjectionSHA256 ||
		disposition.WinnerDigest != acceptedFinal.RecordDigest ||
		disposition.DecidedAt != closure.ClosedAt ||
		disposition.AuthorityKeyID != closure.AuthorityKeyID ||
		disposition.AuthorityPublicKey != closure.AuthorityPublicKey {
		return errors.New("accepted-final disposition does not bind the exact terminal authorities")
	}
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalDispositionAuthorityMaterial(disposition)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainevidence.AcceptedFinalDispositionSigningBytes(disposition), signature) != nil {
		return errors.New("accepted-final disposition is not installation-trusted")
	}
	return nil
}

func (coordinator *Coordinator) verifyTerminalDispositionV1(
	ctx context.Context,
	disposition domainturnterminal.TurnTerminalDispositionV1,
	intent domainturnterminal.TurnTerminalIntentV1,
	closure domaincachetelemetry.ProviderTurnClosureV1,
	acceptedDisposition domainevidence.AcceptedFinalDispositionRecord,
) error {
	if domainturnterminal.ValidateTurnTerminalDispositionForAuthoritiesV1(
		disposition, intent, closure, acceptedDisposition,
	) != nil {
		return errors.New("turn terminal disposition does not bind the exact terminal authorities")
	}
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalDispositionV1AuthorityMaterial(disposition)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainturnterminal.TurnTerminalDispositionV1SigningBytes(disposition), signature) != nil {
		return errors.New("turn terminal disposition is not installation-trusted")
	}
	return nil
}

func (coordinator *Coordinator) verifyProviderClosureV1(ctx context.Context,
	intent domainturnterminal.TurnTerminalIntentV1, closure domaincachetelemetry.ProviderTurnClosureV1) error {
	keyID, publicKey, signature, err := domaincachetelemetry.ProviderTurnClosureV1AuthorityMaterial(closure)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domaincachetelemetry.ProviderTurnClosureV1SigningBytes(closure), signature) != nil ||
		closure.TerminalReasonCode != intent.TerminalReasonCode || closure.AuthorityKeyID != intent.AuthorityKeyID ||
		closure.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("turn terminal provider closure is inconsistent or untrusted")
	}
	preparedAt, preparedErr := time.Parse(time.RFC3339Nano, intent.PreparedAt)
	closedAt, closedErr := time.Parse(time.RFC3339Nano, closure.ClosedAt)
	if preparedErr != nil || closedErr != nil || closedAt.Before(preparedAt) {
		return errors.New("turn terminal provider closure predates its intent")
	}
	return nil
}

func terminalObservationMatchesContextV1(observation domainevidence.AcceptedFinalCASObservationV1,
	securityContext domainsecurity.TurnSecurityContext) bool {
	return terminalObservationMatchesFrozenContextV1(observation, securityContext) &&
		observation.CurrentContext == securityContext
}

func terminalObservationMatchesFrozenContextV1(observation domainevidence.AcceptedFinalCASObservationV1,
	securityContext domainsecurity.TurnSecurityContext) bool {
	return domainevidence.ValidateAcceptedFinalCASObservationV1(observation) == nil &&
		observation.ThreadID == securityContext.ThreadID && observation.TurnID == securityContext.TurnID &&
		observation.FrozenContext == securityContext && strings.TrimSpace(observation.Status) != ""
}

func turnTerminalCASStatusIsActiveV1(status string) bool {
	switch strings.TrimSpace(status) {
	case "running", "queued", "waiting":
		return true
	default:
		return false
	}
}
