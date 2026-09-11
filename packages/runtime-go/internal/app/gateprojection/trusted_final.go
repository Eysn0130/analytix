package gateprojection

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"

	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// TrustedFinalProjectionIndex is a host-owned positive allowlist of finals
// admitted only after production preflight or the publication finalizer has
// verified registry state and durable commit authority.
type TrustedFinalProjectionIndex struct {
	authority authorityport.Authority
	readback  acceptedfinaleventport.Readback
	mu        sync.RWMutex
	records   map[string]trustedFinalProjectionEntry
	nextLease uint64
}

type trustedFinalProjectionEntry struct {
	record                domainevidence.PrivateAcceptedFinalRecord
	disposition           domainevidence.AcceptedFinalDispositionRecord
	terminalDispositionID string
	published             bool
	leaseGeneration       uint64
}

// TerminalCompleteFinalAuthorityV1 is the only admission input for a case
// final projection. The index revalidates every signed I/C/P/Daf/Dt binding;
// callers cannot turn a private final plus a non-terminal disposition into
// public authority.
type TerminalCompleteFinalAuthorityV1 struct {
	PrivateFinal             domainevidence.PrivateAcceptedFinalRecord
	Intent                   domainturnterminal.TurnTerminalIntentV1
	ProviderClosure          domaincachetelemetry.ProviderTurnClosureV1
	PublicObservation        domainevidence.AcceptedFinalCASObservationV1
	AcceptedFinalDisposition domainevidence.AcceptedFinalDispositionRecord
	TerminalDisposition      domainturnterminal.TurnTerminalDispositionV1
	FactAuthority            appturn.FactFinalMutationAuthority `json:"-"`
}

type StagedFinalProjectionLeaseV1 struct {
	index         *TrustedFinalProjectionIndex
	key           string
	digest        string
	generation    uint64
	privateFinal  domainevidence.PrivateAcceptedFinalRecord
	factAuthority appturn.FactFinalMutationAuthority
	alreadyActive bool
	mu            sync.Mutex
	consumed      bool
}

func NewTrustedFinalProjectionIndex(authority authorityport.Authority) *TrustedFinalProjectionIndex {
	return &TrustedFinalProjectionIndex{authority: authority, records: map[string]trustedFinalProjectionEntry{}}
}

func NewTrustedFinalProjectionIndexWithReadback(
	authority authorityport.Authority,
	readback acceptedfinaleventport.Readback,
) *TrustedFinalProjectionIndex {
	return &TrustedFinalProjectionIndex{
		authority: authority, readback: readback, records: map[string]trustedFinalProjectionEntry{},
	}
}

func (index *TrustedFinalProjectionIndex) SeedTerminalComplete(ctx context.Context, authorities []TerminalCompleteFinalAuthorityV1) error {
	if index == nil || index.authority == nil {
		return errors.New("trusted final projection authority is unavailable")
	}
	next := make(map[string]trustedFinalProjectionEntry, len(authorities))
	for _, terminalAuthority := range authorities {
		if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(terminalAuthority.PrivateFinal.Envelope) {
			return errors.New("fact final projection seed requires startup batch witness authority")
		}
		cloned, err := index.validateTerminalComplete(ctx, terminalAuthority)
		if err != nil {
			return err
		}
		key := trustedFinalProjectionKey(cloned.SecurityContext.ThreadID, cloned.SecurityContext.TurnID)
		if _, duplicate := next[key]; duplicate {
			return errors.New("trusted final projection turn is duplicated")
		}
		next[key] = trustedFinalProjectionEntry{
			record: cloned, disposition: terminalAuthority.AcceptedFinalDisposition,
			terminalDispositionID: terminalAuthority.TerminalDisposition.DispositionID, published: true,
		}
	}
	index.mu.Lock()
	index.records = next
	index.mu.Unlock()
	return nil
}

func (index *TrustedFinalProjectionIndex) RegisterTerminalComplete(ctx context.Context, authority TerminalCompleteFinalAuthorityV1) error {
	lease, err := index.StageTerminalComplete(ctx, authority)
	if err != nil {
		return err
	}
	defer lease.Discard()
	return lease.Activate()
}

func (index *TrustedFinalProjectionIndex) StageTerminalComplete(
	ctx context.Context,
	authority TerminalCompleteFinalAuthorityV1,
) (*StagedFinalProjectionLeaseV1, error) {
	cloned, err := index.validateTerminalComplete(ctx, authority)
	if err != nil {
		return nil, err
	}
	key := trustedFinalProjectionKey(cloned.SecurityContext.ThreadID, cloned.SecurityContext.TurnID)
	var lease *StagedFinalProjectionLeaseV1
	err = appturn.UsePrivateAcceptedFinalMutationAuthority(cloned, authority.FactAuthority, func() error {
		index.mu.Lock()
		defer index.mu.Unlock()
		entry := trustedFinalProjectionEntry{
			record: cloned, disposition: authority.AcceptedFinalDisposition,
			terminalDispositionID: authority.TerminalDisposition.DispositionID,
		}
		if current, found := index.records[key]; found {
			candidate := entry
			candidate.published = current.published
			candidate.leaseGeneration = current.leaseGeneration
			if !reflect.DeepEqual(current, candidate) {
				return errors.New("trusted final projection conflicts with committed authority")
			}
			if !current.published {
				return errors.New("trusted final projection is already owned by another staged lease")
			}
			lease = &StagedFinalProjectionLeaseV1{
				index: index, key: key, digest: cloned.AcceptedFinal.RecordDigest,
				generation: current.leaseGeneration, privateFinal: cloned,
				factAuthority: authority.FactAuthority, alreadyActive: true,
			}
			return nil
		}
		index.nextLease++
		entry.leaseGeneration = index.nextLease
		index.records[key] = entry
		lease = &StagedFinalProjectionLeaseV1{
			index: index, key: key, digest: cloned.AcceptedFinal.RecordDigest,
			generation: entry.leaseGeneration, privateFinal: cloned, factAuthority: authority.FactAuthority,
		}
		return nil
	})
	return lease, err
}

func (lease *StagedFinalProjectionLeaseV1) Activate() error {
	if lease == nil || lease.index == nil {
		return errors.New("trusted final projection lease is unavailable")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.consumed {
		return errors.New("trusted final projection lease is already consumed")
	}
	if lease.alreadyActive {
		lease.consumed = true
		return nil
	}
	err := appturn.UsePrivateAcceptedFinalMutationAuthority(lease.privateFinal, lease.factAuthority, func() error {
		lease.index.mu.Lock()
		defer lease.index.mu.Unlock()
		entry, found := lease.index.records[lease.key]
		if !found || entry.published || entry.leaseGeneration != lease.generation ||
			entry.record.AcceptedFinal.RecordDigest != lease.digest {
			return errors.New("trusted final projection staged authority is unavailable")
		}
		entry.published = true
		lease.index.records[lease.key] = entry
		return nil
	})
	if err == nil {
		lease.consumed = true
	}
	return err
}

func (lease *StagedFinalProjectionLeaseV1) SealAcceptedFinalDelivery(
	ctx context.Context,
	events []map[string]any,
) (domainevent.AcceptedFinalDeliverySealV1, error) {
	if lease == nil || lease.index == nil {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("trusted final projection lease is unavailable")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.consumed {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("trusted final projection lease is already consumed")
	}
	lease.index.mu.RLock()
	entry, found := lease.index.records[lease.key]
	lease.index.mu.RUnlock()
	if !found || entry.leaseGeneration != lease.generation ||
		entry.record.AcceptedFinal.RecordDigest != lease.digest ||
		(!lease.alreadyActive && entry.published) {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("trusted final projection staged authority is unavailable")
	}
	return lease.index.sealAcceptedFinalDeliveryForEntry(ctx, events, entry, true)
}

func (lease *StagedFinalProjectionLeaseV1) Discard() {
	if lease == nil || lease.index == nil {
		return
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.consumed {
		return
	}
	lease.index.mu.Lock()
	entry, found := lease.index.records[lease.key]
	if found && !entry.published && entry.leaseGeneration == lease.generation &&
		entry.record.AcceptedFinal.RecordDigest == lease.digest {
		delete(lease.index.records, lease.key)
	}
	lease.index.mu.Unlock()
	lease.consumed = true
}

func (index *TrustedFinalProjectionIndex) validateTerminalComplete(
	ctx context.Context,
	terminalAuthority TerminalCompleteFinalAuthorityV1,
) (domainevidence.PrivateAcceptedFinalRecord, error) {
	record := terminalAuthority.PrivateFinal
	intent := terminalAuthority.Intent
	closure := terminalAuthority.ProviderClosure
	observation := terminalAuthority.PublicObservation
	disposition := terminalAuthority.AcceptedFinalDisposition
	terminalDisposition := terminalAuthority.TerminalDisposition
	publication, publicationErr := appturn.BuildAcceptedFinalPublicationPlan(
		record.AcceptedFinal, record.RenderedText, record.PublicationIntent,
	)
	expectedStatus, statusOK := domainevidence.FinalAnswerTerminalStatus(record.AcceptedFinal.TerminalReason)
	if index == nil || index.authority == nil || publicationErr != nil || !statusOK ||
		appturn.ValidatePrivateAcceptedFinalMutationAuthority(record, terminalAuthority.FactAuthority) != nil ||
		validateTerminalIntentProjectionAuthorityV1(intent, record, terminalAuthority.FactAuthority) != nil ||
		domaincachetelemetry.ValidateProviderTurnClosureV1(closure) != nil ||
		domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil ||
		domainevidence.ValidateAcceptedFinalDispositionRecord(disposition) != nil ||
		domainturnterminal.ValidateTurnTerminalDispositionForAuthoritiesV1(
			terminalDisposition, intent, closure, disposition,
		) != nil ||
		observation.ThreadID != record.SecurityContext.ThreadID ||
		observation.TurnID != record.SecurityContext.TurnID ||
		observation.FrozenContext != record.SecurityContext || observation.Status != expectedStatus ||
		!observation.HasWinner || observation.Winner.RecordDigest != record.AcceptedFinal.RecordDigest ||
		intent.EventManifestDigest != publication.EventManifestDigest ||
		disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		disposition.State != domainevidence.AcceptedFinalCommitted ||
		disposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		disposition.AcceptedFinalDigest != record.AcceptedFinal.RecordDigest ||
		disposition.PrivateRecordDigest != record.PrivateRecordDigest ||
		disposition.ThreadID != record.SecurityContext.ThreadID || disposition.TurnID != record.SecurityContext.TurnID ||
		disposition.EventManifestDigest != publication.EventManifestDigest ||
		disposition.WinnerDigest != record.AcceptedFinal.RecordDigest ||
		disposition.TurnCASDigest != observation.TurnProjectionSHA256 ||
		disposition.DecidedAt != closure.ClosedAt {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("trusted final projection input is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record.AcceptedFinal)
	if err == nil {
		err = index.authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(record.AcceptedFinal), signature)
	}
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.Join(errors.New("trusted final projection accepted-final authority is invalid"), err)
	}
	keyID, publicKey, signature, err = domainturnterminal.TurnTerminalIntentV1AuthorityMaterial(intent)
	if err == nil {
		err = index.authority.VerifyTrusted(ctx, keyID, publicKey, domainturnterminal.TurnTerminalIntentV1SigningBytes(intent), signature)
	}
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.Join(errors.New("trusted final projection terminal intent authority is invalid"), err)
	}
	keyID, publicKey, signature, err = domaincachetelemetry.ProviderTurnClosureV1AuthorityMaterial(closure)
	if err == nil {
		err = index.authority.VerifyTrusted(ctx, keyID, publicKey, domaincachetelemetry.ProviderTurnClosureV1SigningBytes(closure), signature)
	}
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.Join(errors.New("trusted final projection provider closure authority is invalid"), err)
	}
	keyID, publicKey, signature, err = domainevidence.AcceptedFinalDispositionAuthorityMaterial(disposition)
	if err == nil {
		err = index.authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalDispositionSigningBytes(disposition), signature)
	}
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.Join(errors.New("trusted final projection disposition authority is invalid"), err)
	}
	keyID, publicKey, signature, err = domainturnterminal.TurnTerminalDispositionV1AuthorityMaterial(terminalDisposition)
	if err == nil {
		err = index.authority.VerifyTrusted(ctx, keyID, publicKey, domainturnterminal.TurnTerminalDispositionV1SigningBytes(terminalDisposition), signature)
	}
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.Join(errors.New("trusted final projection terminal disposition authority is invalid"), err)
	}
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	return domainevidence.ParsePrivateAcceptedFinalRecord(body)
}

func validateTerminalIntentProjectionAuthorityV1(
	intent domainturnterminal.TurnTerminalIntentV1,
	record domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) error {
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(record.Envelope) {
		if factAuthority == nil {
			return errors.New("fact terminal projection authority is unavailable")
		}
		return domainturnterminal.ValidateTurnTerminalIntentForPrivateFinalWithFactWitnessV1(
			intent,
			record,
			func(candidate domainevidence.PrivateAcceptedFinalRecord) error {
				return factAuthority.UseExact(candidate, func() error { return nil })
			},
		)
	}
	if factAuthority != nil {
		return errors.New("boundary terminal projection carried fact authority")
	}
	return domainturnterminal.ValidateTurnTerminalIntentForPrivateFinalV1(intent, record)
}

func (index *TrustedFinalProjectionIndex) Resolve(threadID, turnID string) (domainevidence.PrivateAcceptedFinalRecord, bool) {
	record, _, found := index.ResolveCommitted(threadID, turnID)
	return record, found
}

func (index *TrustedFinalProjectionIndex) ContainsThread(threadID string) bool {
	if index == nil {
		return false
	}
	prefix := strings.TrimSpace(threadID) + "\x00"
	index.mu.RLock()
	defer index.mu.RUnlock()
	for key := range index.records {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (index *TrustedFinalProjectionIndex) RecordsForThread(threadID string) []domainevidence.PrivateAcceptedFinalRecord {
	if index == nil {
		return nil
	}
	prefix := strings.TrimSpace(threadID) + "\x00"
	index.mu.RLock()
	entries := make([]trustedFinalProjectionEntry, 0)
	for key, entry := range index.records {
		if strings.HasPrefix(key, prefix) && entry.published {
			entries = append(entries, entry)
		}
	}
	index.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].record.SecurityContext.TurnID < entries[j].record.SecurityContext.TurnID
	})
	records := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(entries))
	for _, entry := range entries {
		body, err := domainevidence.PrivateAcceptedFinalRecordBytes(entry.record)
		if err != nil {
			return nil
		}
		cloned, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
		if err != nil {
			return nil
		}
		records = append(records, cloned)
	}
	return records
}

func (index *TrustedFinalProjectionIndex) ResolveCommitted(threadID, turnID string) (domainevidence.PrivateAcceptedFinalRecord, domainevidence.AcceptedFinalDispositionRecord, bool) {
	return index.resolve(threadID, turnID, false)
}

// TerminalProjectionPending recognizes only the two host-authorized intervals
// before accepted-final event durability activates public projection: an
// installation-signed terminal already committed to the normalized thread,
// and a validated staged lease. It never exposes the record or authorizes any
// public content.
func (index *TrustedFinalProjectionIndex) TerminalProjectionPending(threadID, turnID string, rawAcceptedFinal any) bool {
	if index == nil || index.authority == nil {
		return false
	}
	index.mu.RLock()
	entry, found := index.records[trustedFinalProjectionKey(threadID, turnID)]
	index.mu.RUnlock()
	if found {
		if entry.published {
			return false
		}
		record, err := domainevidence.ParseAcceptedFinalRecord(rawAcceptedFinal)
		return err == nil && reflect.DeepEqual(record, entry.record.AcceptedFinal)
	}
	record, err := domainevidence.ParseAcceptedFinalRecord(rawAcceptedFinal)
	if err != nil || record.ThreadID != strings.TrimSpace(threadID) || record.TurnID != strings.TrimSpace(turnID) {
		return false
	}
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record)
	return err == nil && index.authority.VerifyTrusted(
		context.Background(), keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(record), signature,
	) == nil
}

func (index *TrustedFinalProjectionIndex) ResolveForEvent(threadID, turnID string) (domainevidence.PrivateAcceptedFinalRecord, domainevidence.AcceptedFinalDispositionRecord, bool) {
	// Event projection is a public surface. A staged lease proves only that the
	// terminal authority has been validated while its durable event bundle is
	// being prepared; it is not publication authority. Keep event replay and
	// live delivery on the same committed-only allowlist as thread snapshots.
	return index.resolve(threadID, turnID, false)
}

func (index *TrustedFinalProjectionIndex) SealAcceptedFinalDelivery(
	ctx context.Context,
	events []map[string]any,
) (domainevent.AcceptedFinalDeliverySealV1, error) {
	if index == nil || index.authority == nil || len(events) == 0 {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery authority is unavailable")
	}
	threadID := strings.TrimSpace(contracts.StringField(events[0], "threadId"))
	turnID := strings.TrimSpace(contracts.StringField(events[0], "turnId"))
	index.mu.RLock()
	entry, found := index.records[trustedFinalProjectionKey(threadID, turnID)]
	index.mu.RUnlock()
	if !found || !entry.published {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery parent authority is invalid")
	}
	return index.sealAcceptedFinalDeliveryForEntry(ctx, events, entry, false)
}

func (index *TrustedFinalProjectionIndex) sealAcceptedFinalDeliveryForEntry(
	ctx context.Context,
	events []map[string]any,
	entry trustedFinalProjectionEntry,
	requireReservedTail bool,
) (domainevent.AcceptedFinalDeliverySealV1, error) {
	if index == nil || index.authority == nil || len(events) == 0 ||
		entry.record.SecurityContext.ThreadID != strings.TrimSpace(contracts.StringField(events[0], "threadId")) ||
		entry.record.SecurityContext.TurnID != strings.TrimSpace(contracts.StringField(events[0], "turnId")) ||
		entry.record.AcceptedFinal.RecordDigest != contracts.StringField(events[0], "publicationCommitId") ||
		entry.disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		entry.disposition.State != domainevidence.AcceptedFinalCommitted ||
		entry.disposition.AcceptedFinalDigest != entry.record.AcceptedFinal.RecordDigest ||
		entry.disposition.WinnerDigest != entry.record.AcceptedFinal.RecordDigest {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery parent authority is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if index.readback == nil {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery durable readback is unavailable")
	}
	var readbackErr error
	if requireReservedTail {
		readbackErr = index.readback.VerifyReservedTail(ctx, events)
	} else {
		readbackErr = index.readback.VerifyCommittedManifest(ctx, events)
	}
	if readbackErr != nil {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.Join(
			errors.New("accepted final delivery durable readback was rejected"), readbackErr,
		)
	}
	seal, err := domainevent.NewAcceptedFinalDeliverySealForEventsV2(
		events, entry.disposition.RecordDigest, entry.terminalDispositionID,
		index.authority.KeyID(), index.authority.PublicKey(),
		func(message []byte) ([]byte, error) { return index.authority.Sign(ctx, message) },
	)
	if err != nil || seal.EventManifestDigest != entry.disposition.EventManifestDigest {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.Join(errors.New("accepted final delivery manifest authority is invalid"), err)
	}
	publicKey, signature, err := acceptedFinalDeliverySealAuthorityMaterialV1(seal)
	if err != nil || index.authority.VerifyTrusted(
		ctx, seal.AuthorityKeyID, publicKey, domainevent.AcceptedFinalDeliverySealSigningBytesV1(seal), signature,
	) != nil {
		return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("accepted final delivery seal is not installation-trusted")
	}
	return seal, nil
}

func (index *TrustedFinalProjectionIndex) ValidateAcceptedFinalDelivery(
	ctx context.Context,
	batch domainevent.AcceptedFinalDeliveryBatchV2,
) error {
	if index == nil || index.authority == nil || domainevent.ValidateAcceptedFinalDeliveryBatchV2(batch) != nil {
		return errors.New("accepted final delivery batch is invalid")
	}
	index.mu.RLock()
	entry, found := index.records[trustedFinalProjectionKey(batch.ThreadID, batch.TurnID)]
	index.mu.RUnlock()
	seal := batch.PublicationAuthority
	if !found || !entry.published || entry.record.AcceptedFinal.RecordDigest != batch.PublicationCommitID ||
		entry.disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		entry.disposition.State != domainevidence.AcceptedFinalCommitted ||
		entry.disposition.AcceptedFinalDigest != batch.PublicationCommitID ||
		entry.disposition.WinnerDigest != batch.PublicationCommitID ||
		entry.disposition.EventManifestDigest != batch.EventManifestDigest ||
		seal.AcceptedFinalDispositionDigest != entry.disposition.RecordDigest ||
		seal.TerminalDispositionID != entry.terminalDispositionID ||
		seal.AuthorityKeyID != entry.record.AcceptedFinal.AuthorityKeyID ||
		seal.AuthorityPublicKey != entry.record.AcceptedFinal.AuthorityPublicKey {
		return errors.New("accepted final delivery batch is detached from committed authority")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	publicKey, signature, err := acceptedFinalDeliverySealAuthorityMaterialV1(seal)
	if err != nil {
		return err
	}
	return index.authority.VerifyTrusted(
		ctx, seal.AuthorityKeyID, publicKey, domainevent.AcceptedFinalDeliverySealSigningBytesV1(seal), signature,
	)
}

func (index *TrustedFinalProjectionIndex) resolve(threadID, turnID string, includeStaged bool) (domainevidence.PrivateAcceptedFinalRecord, domainevidence.AcceptedFinalDispositionRecord, bool) {
	if index == nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, domainevidence.AcceptedFinalDispositionRecord{}, false
	}
	index.mu.RLock()
	entry, found := index.records[trustedFinalProjectionKey(threadID, turnID)]
	index.mu.RUnlock()
	if !found || (!includeStaged && !entry.published) {
		return domainevidence.PrivateAcceptedFinalRecord{}, domainevidence.AcceptedFinalDispositionRecord{}, false
	}
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(entry.record)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, domainevidence.AcceptedFinalDispositionRecord{}, false
	}
	cloned, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, domainevidence.AcceptedFinalDispositionRecord{}, false
	}
	return cloned, entry.disposition, true
}

func trustedFinalProjectionKey(threadID, turnID string) string {
	return strings.TrimSpace(threadID) + "\x00" + strings.TrimSpace(turnID)
}

func acceptedFinalDeliverySealAuthorityMaterialV1(
	seal domainevent.AcceptedFinalDeliverySealV1,
) ([]byte, []byte, error) {
	if err := domainevent.ValidateAcceptedFinalDeliverySealV1(seal); err != nil {
		return nil, nil, err
	}
	publicKey, publicErr := base64.RawURLEncoding.Strict().DecodeString(seal.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.Strict().DecodeString(seal.AuthoritySignature)
	if publicErr != nil || signatureErr != nil {
		return nil, nil, errors.New("accepted final delivery seal authority material is invalid")
	}
	return publicKey, signature, nil
}
