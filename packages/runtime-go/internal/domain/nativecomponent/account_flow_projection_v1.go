package nativecomponent

import (
	"errors"
	"strings"
	"sync"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const accountFlowSourceRecordIDPrefixV1 = "srow1_"

const (
	AccountFlowGapCounterpartyResolutionV1  = "counterparty_resolution"
	AccountFlowProviderCurrentnessCurrentV1 = "current"

	AccountFlowCounterpartyResolvedV1   = "resolved"
	AccountFlowCounterpartyPartialV1    = "partial"
	AccountFlowCounterpartyUnresolvedV1 = "unresolved"

	AccountFlowCounterpartyAccountTypeV1 = "交易对手账户"
)

var ErrAccountFlowCounterpartySemanticUnavailableV1 = errors.New(
	"account flow counterparty semantic is unavailable",
)

// AccountFlowProviderSemanticCoverageV1 is the provider-safe coverage view of
// one fixed account-flow query. Complete aggregate coverage and complete row
// evidence remain separate because a bounded result may contain an exact
// aggregate while carrying only a prefix of its evidence rows.
type AccountFlowProviderSemanticCoverageV1 struct {
	State                  string   `json:"state"`
	Gaps                   []string `json:"gaps"`
	NormalizedSnapshotRows uint64   `json:"normalizedSnapshotRows"`
	AcceptedSnapshotRows   uint64   `json:"acceptedSnapshotRows"`
	RejectedSnapshotRows   uint64   `json:"rejectedSnapshotRows"`
	DuplicateSnapshotRows  uint64   `json:"duplicateSnapshotRows"`
	UntimedSubjectRows     uint64   `json:"untimedSubjectRows"`
	ObservedMatchingRows   uint64   `json:"observedMatchingRows"`
}

// AccountFlowProviderSemanticTransactionV1 contains useful transaction
// semantics plus one opaque, case/source-artifact-bound evidence reference.
// EvidenceRef is the canonical host-resolved SourceRowRecordV1 identity, not a
// source locator; snapshot/currentness is established by the enclosing result
// and receipt binding. The source file/row tuple, counterparty values, database
// path and query text remain host-private.
type AccountFlowProviderSemanticTransactionV1 struct {
	EvidenceRef    string                            `json:"evidenceRef"`
	Counterparty   AccountFlowProviderCounterpartyV1 `json:"counterparty"`
	OccurredAt     string                            `json:"occurredAt"`
	Direction      string                            `json:"direction"`
	AmountMinor    string                            `json:"amountMinor"`
	Currency       string                            `json:"currency"`
	MinorUnitScale uint8                             `json:"minorUnitScale"`
}

// AccountFlowProviderCounterpartyV1 is the provider-safe identity projection
// for one evidence row. Alias is meaningful only in the frozen current case;
// authority refs, stable-ordinal metadata, suffixes, institution and other
// source-derived display semantics, raw values, and generated display text
// remain host-private.
type AccountFlowProviderCounterpartyV1 struct {
	Status      string                              `json:"status"`
	Alias       domaincaseentity.ModelEntityAliasV1 `json:"alias,omitempty"`
	EntityType  string                              `json:"entityType,omitempty"`
	AccountType string                              `json:"accountType,omitempty"`
}

// AccountFlowProviderOutcomeV1 is a proposal-time, non-authoritative view of
// independent eligibility axes. The host has verified the query scope and
// current native result, but receipt lineage, Final Gate eligibility, factual
// authority, and local display remain deliberately pending.
type AccountFlowProviderOutcomeV1 struct {
	AggregateCompleteness         string                                                `json:"aggregateCompleteness"`
	EvidenceRowsCompleteness      string                                                `json:"evidenceRowsCompleteness"`
	TypedSlotEligibility          string                                                `json:"typedSlotEligibility"`
	LocalDisplayAvailability      string                                                `json:"localDisplayAvailability"`
	LocalDisplayCompletion        string                                                `json:"localDisplayCompletion"`
	FactAnswerAllowed             bool                                                  `json:"factAnswerAllowed"`
	QueryScopeBindingEligibility  string                                                `json:"queryScopeBindingEligibility"`
	CurrentnessEligibility        string                                                `json:"currentnessEligibility"`
	LineageEligibility            string                                                `json:"lineageEligibility"`
	SourceFieldBindingEligibility string                                                `json:"sourceFieldBindingEligibility"`
	SourceFieldReference          domainevidence.AccountFlowTypedSourceFieldReferenceV1 `json:"sourceFieldReference"`
}

// AccountFlowProviderSemanticResultV1 is safe to place in an ordinary model
// tool result. SubjectAlias is the closed, case-scoped selector used for model
// reasoning; authority and evidence-version bindings stay in the synchronous
// host-private projection below.
type AccountFlowProviderSemanticResultV1 struct {
	SubjectAlias                  string                                     `json:"subjectAlias"`
	StartInclusive                string                                     `json:"startInclusive"`
	EndInclusive                  string                                     `json:"endInclusive"`
	Timezone                      string                                     `json:"timezone"`
	Currency                      string                                     `json:"currency"`
	MinorUnitScale                uint8                                      `json:"minorUnitScale"`
	InflowMinor                   string                                     `json:"inflowMinor"`
	OutflowMinor                  string                                     `json:"outflowMinor"`
	NetMinor                      string                                     `json:"netMinor"`
	TransactionCount              uint64                                     `json:"transactionCount"`
	EvidenceTransactionCount      uint64                                     `json:"evidenceTransactionCount"`
	EvidenceRowLimit              uint32                                     `json:"evidenceRowLimit"`
	AggregateComplete             bool                                       `json:"aggregateComplete"`
	EvidenceRowsComplete          bool                                       `json:"evidenceRowsComplete"`
	CounterpartySemanticsComplete bool                                       `json:"counterpartySemanticsComplete"`
	Currentness                   string                                     `json:"currentness"`
	Coverage                      AccountFlowProviderSemanticCoverageV1      `json:"coverage"`
	Transactions                  []AccountFlowProviderSemanticTransactionV1 `json:"transactions"`
	QueryHash                     string                                     `json:"queryHash"`
	ResultHash                    string                                     `json:"resultHash"`
	Outcome                       AccountFlowProviderOutcomeV1               `json:"outcome"`
}

func NewAccountFlowProviderOutcomeV1(
	subjectAlias string,
	aggregateComplete bool,
	evidenceRowsComplete bool,
	queryHash string,
	resultHash string,
) (AccountFlowProviderOutcomeV1, error) {
	field, err := domainevidence.AcceptedSlotSourceFieldForModelEntityAliasV1(subjectAlias)
	if err != nil {
		return AccountFlowProviderOutcomeV1{}, ErrResultInvalid
	}
	reference, err := domainevidence.NewAccountFlowTypedSourceFieldReferenceV1(queryHash, resultHash, field)
	if err != nil {
		return AccountFlowProviderOutcomeV1{}, ErrResultInvalid
	}
	aggregateCompleteness := domainevidence.AccountFlowOutcomeCompletenessIncompleteV1
	if aggregateComplete {
		aggregateCompleteness = domainevidence.AccountFlowOutcomeCompletenessCompleteV1
	}
	evidenceCompleteness := domainevidence.AccountFlowOutcomeCompletenessIncompleteV1
	if evidenceRowsComplete {
		evidenceCompleteness = domainevidence.AccountFlowOutcomeCompletenessCompleteV1
	}
	return AccountFlowProviderOutcomeV1{
		AggregateCompleteness:         aggregateCompleteness,
		EvidenceRowsCompleteness:      evidenceCompleteness,
		TypedSlotEligibility:          domainevidence.AccountFlowOutcomeTypedSlotPendingFinalGateV1,
		LocalDisplayAvailability:      domainevidence.AccountFlowOutcomeDisplayPendingFinalGateV1,
		LocalDisplayCompletion:        domainevidence.AccountFlowOutcomeDisplayNotRequestedV1,
		FactAnswerAllowed:             false,
		QueryScopeBindingEligibility:  domainevidence.AccountFlowOutcomeQueryScopeHostVerifiedV1,
		CurrentnessEligibility:        domainevidence.AccountFlowOutcomeCurrentnessCurrentV1,
		LineageEligibility:            domainevidence.AccountFlowOutcomeLineagePendingReceiptV1,
		SourceFieldBindingEligibility: domainevidence.AccountFlowOutcomeSourceFieldPendingReceiptV1,
		SourceFieldReference:          reference,
	}, nil
}

func ValidateAccountFlowProviderOutcomeV1(semantic AccountFlowProviderSemanticResultV1) error {
	want, err := NewAccountFlowProviderOutcomeV1(
		semantic.SubjectAlias,
		semantic.AggregateComplete,
		semantic.EvidenceRowsComplete,
		semantic.QueryHash,
		semantic.ResultHash,
	)
	if err != nil || semantic.Outcome != want {
		return ErrResultInvalid
	}
	return nil
}

// AccountFlowSourceRowResolverV1 resolves one exact native source locator to
// an existing canonical SourceRowRecordV1 identity. The projector deliberately
// cannot derive, mint or guess this identity. The resolver must invoke consume
// exactly once before it returns.
type AccountFlowSourceRowResolverV1 func(
	sourceFileID string,
	sourceRowNumber uint64,
	consume func(sourceRecordID string) error,
) error

// AccountFlowCounterpartyResolverV1 binds one source-exact counterparty
// account through the existing case-entity store while the caller's DSV2 and
// exact-source lease remains live. It returns only safe identity semantics via
// consume. The unavailable sentinel produces an explicit partial row; any
// other error invalidates the complete projection.
type AccountFlowCounterpartyResolverV1 func(
	sourceExactAccount string,
	bankInstitution string,
	consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
) error

// AccountFlowHostEvidenceSummaryConsumerV1 receives the exact host-private
// evidence summary while AccountFlowHostEvidenceProjectionV1 remains active.
// It is intentionally a callback rather than a serializable value.
type AccountFlowHostEvidenceSummaryConsumerV1 func(
	subjectRef string,
	datasetSnapshotID string,
	contextEpoch uint64,
	contextDigest string,
	caseBindingHash string,
	startInclusive string,
	endInclusive string,
	timezone string,
	currency string,
	minorUnitScale uint8,
	inflowMinor string,
	outflowMinor string,
	netMinor string,
	transactionCount uint64,
	aggregateComplete bool,
	evidenceRowsComplete bool,
	evidenceRowCount uint64,
	coverage AccountFlowProviderSemanticCoverageV1,
	queryHash string,
	resultHash string,
) error

// AccountFlowHostEvidenceRowConsumerV1 receives one exact host-private row
// binding. sourceRecordID is an already-existing canonical srow1_ identity.
// sourceFileID and sourceRowNumber remain inside the synchronous host-private
// evidence path so a later DSV2 revalidation can address the exact row without
// scanning the entire ledger. Counterparty PII never leaves the projector.
type AccountFlowHostEvidenceRowConsumerV1 func(
	index int,
	subjectRef string,
	sourceRecordID string,
	sourceFileID string,
	sourceRowNumber uint64,
	occurredAt string,
	direction string,
	amountMinor string,
	currency string,
	minorUnitScale uint8,
) error

// AccountFlowHostEvidenceProjectionConsumerV1 consumes the one-use private
// projection synchronously into process-local memory only. It must not perform
// persistence, publication, settlement, network, filesystem, or other effects.
// Retaining the projection does not retain usable evidence after the host
// consumption boundary closes.
type AccountFlowHostEvidenceProjectionConsumerV1 func(AccountFlowHostEvidenceProjectionV1) error

// AccountFlowHostEvidenceProjectionV1 is a callback-only carrier. Its closure
// holds subject/evidence references and exact evidence semantics; the struct
// itself contains no serializable private field.
type AccountFlowHostEvidenceProjectionV1 struct {
	use func(
		AccountFlowHostEvidenceSummaryConsumerV1,
		AccountFlowHostEvidenceRowConsumerV1,
	) error
	beginConsume func() bool
	close        func() (uint32, bool)
	discard      func()
}

// UseExactV1 consumes the summary once followed by every evidence row in
// native order. It succeeds at most once and only inside the deferred
// synchronous host-consumption boundary.
func (projection AccountFlowHostEvidenceProjectionV1) UseExactV1(
	consumeSummary AccountFlowHostEvidenceSummaryConsumerV1,
	consumeRow AccountFlowHostEvidenceRowConsumerV1,
) error {
	if projection.use == nil || consumeSummary == nil || consumeRow == nil {
		return ErrResultInvalid
	}
	return projection.use(consumeSummary, consumeRow)
}

func (AccountFlowHostEvidenceProjectionV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}

func (AccountFlowHostEvidenceProjectionV1) String() string {
	return "AccountFlowHostEvidenceProjectionV1{private:[REDACTED]}"
}

func (projection AccountFlowHostEvidenceProjectionV1) GoString() string {
	return projection.String()
}

// ConsumeAccountFlowHostEvidenceProjectionV1 opens the deferred projection for
// one synchronous, process-local in-memory consumer and burns it on every
// return or panic path. Consumer panics are converted to ErrResultInvalid so a
// retained carrier cannot survive by unwinding the host boundary.
func ConsumeAccountFlowHostEvidenceProjectionV1(
	projection AccountFlowHostEvidenceProjectionV1,
	consumeEvidence AccountFlowHostEvidenceProjectionConsumerV1,
) (err error) {
	if projection.use == nil || projection.beginConsume == nil || projection.close == nil ||
		projection.discard == nil || consumeEvidence == nil {
		if projection.discard != nil {
			projection.discard()
		}
		return ErrResultInvalid
	}
	if !projection.beginConsume() {
		return ErrResultInvalid
	}
	var attempts uint32
	var completed bool
	defer func() {
		recovered := recover()
		attempts, completed = projection.close()
		if recovered != nil {
			err = ErrResultInvalid
			return
		}
		if err == nil && (attempts != 1 || !completed) {
			err = ErrResultInvalid
		}
	}()
	err = consumeEvidence(projection)
	return err
}

// DiscardAccountFlowHostEvidenceProjectionV1 immediately deactivates a
// deferred projection and requests its private capture be burned. It never
// waits for an in-flight exact use, so a summary or row callback may fail
// closed by discarding its own carrier without deadlocking. If an exact use is
// already in flight, the capture is burned as that callback unwinds. The
// operation is idempotent and has no external effect.
func DiscardAccountFlowHostEvidenceProjectionV1(
	projection AccountFlowHostEvidenceProjectionV1,
) {
	if projection.discard != nil {
		projection.discard()
	}
}

type accountFlowHostEvidenceSummaryV1 struct {
	subjectRef           string
	datasetSnapshotID    string
	contextEpoch         uint64
	contextDigest        string
	caseBindingHash      string
	startInclusive       string
	endInclusive         string
	timezone             string
	currency             string
	minorUnitScale       uint8
	inflowMinor          string
	outflowMinor         string
	netMinor             string
	transactionCount     uint64
	aggregateComplete    bool
	evidenceRowsComplete bool
	evidenceRowCount     uint64
	coverage             AccountFlowProviderSemanticCoverageV1
	queryHash            string
	resultHash           string
}

type accountFlowHostEvidenceRowProjectionV1 struct {
	subjectRef      string
	sourceRecordID  string
	sourceFileID    string
	sourceRowNumber uint64
	occurredAt      string
	direction       string
	amountMinor     string
	currency        string
	minorUnitScale  uint8
}

type accountFlowSourceRowResolutionStateV1 struct {
	mu             sync.Mutex
	active         bool
	calls          uint32
	completed      bool
	sourceRecordID string
}

type accountFlowCounterpartyResolutionStateV1 struct {
	mu        sync.Mutex
	active    bool
	calls     uint32
	completed bool
	reference domaincaseentity.ReferenceV1
	display   domaincaseentity.DisplayLabelV1
}

func newAccountFlowCounterpartyResolutionStateV1() *accountFlowCounterpartyResolutionStateV1 {
	return &accountFlowCounterpartyResolutionStateV1{active: true}
}

func (state *accountFlowCounterpartyResolutionStateV1) accept(
	reference domaincaseentity.ReferenceV1,
	display domaincaseentity.DisplayLabelV1,
) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.calls++
	if !state.active || state.calls != 1 ||
		domaincaseentity.ValidateReferenceV1(string(reference)) != nil ||
		domaincaseentity.ValidateDisplayLabelV1(display) != nil ||
		display.EntityType != domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 ||
		display.AccountType != AccountFlowCounterpartyAccountTypeV1 {
		return ErrResultInvalid
	}
	state.reference = reference
	state.display = display
	state.completed = true
	return nil
}

func (state *accountFlowCounterpartyResolutionStateV1) close() (
	uint32,
	bool,
	domaincaseentity.ReferenceV1,
	domaincaseentity.DisplayLabelV1,
) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.active = false
	return state.calls, state.completed, state.reference, state.display
}

func newAccountFlowSourceRowResolutionStateV1() *accountFlowSourceRowResolutionStateV1 {
	return &accountFlowSourceRowResolutionStateV1{active: true}
}

func (state *accountFlowSourceRowResolutionStateV1) accept(candidate string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.calls++
	if !state.active || state.calls != 1 || !validAccountFlowSourceRecordIDV1(candidate) {
		return ErrResultInvalid
	}
	state.sourceRecordID = candidate
	state.completed = true
	return nil
}

func (state *accountFlowSourceRowResolutionStateV1) close() (uint32, bool, string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.active = false
	return state.calls, state.completed, state.sourceRecordID
}

type accountFlowHostEvidenceProjectionUseStateV1 struct {
	mu               sync.Mutex
	condition        *sync.Cond
	active           bool
	consumerActive   bool
	consumerAttempts uint32
	attempts         uint32
	inUse            bool
	completed        bool
	invalid          bool
	cleanup          func()
	cleanupInFlight  bool
}

func newAccountFlowHostEvidenceProjectionUseStateV1(
	cleanup func(),
) *accountFlowHostEvidenceProjectionUseStateV1 {
	state := &accountFlowHostEvidenceProjectionUseStateV1{
		active:  true,
		cleanup: cleanup,
	}
	state.condition = sync.NewCond(&state.mu)
	return state
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) beginConsumer() bool {
	state.mu.Lock()
	state.consumerAttempts++
	if state.active && !state.consumerActive && state.consumerAttempts == 1 {
		state.consumerActive = true
		state.mu.Unlock()
		return true
	}
	state.active = false
	state.invalid = true
	cleanup := state.takeCleanupIfIdleLocked()
	state.mu.Unlock()
	state.runCleanup(cleanup)
	return false
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) begin() bool {
	state.mu.Lock()
	state.attempts++
	if !state.active || !state.consumerActive || state.attempts != 1 || state.inUse {
		state.active = false
		state.invalid = true
		cleanup := state.takeCleanupIfIdleLocked()
		state.mu.Unlock()
		state.runCleanup(cleanup)
		return false
	}
	state.inUse = true
	state.mu.Unlock()
	return true
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) isActive() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.active
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) end(succeeded bool) {
	state.mu.Lock()
	if state.inUse {
		state.completed = succeeded && state.active && !state.invalid &&
			state.consumerAttempts == 1 && state.attempts == 1
		state.inUse = false
	}
	cleanup := state.takeCleanupIfIdleLocked()
	state.condition.Broadcast()
	state.mu.Unlock()
	state.runCleanup(cleanup)
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) close() (uint32, bool) {
	state.mu.Lock()
	state.active = false
	state.consumerActive = false
	for state.inUse || state.cleanupInFlight {
		state.condition.Wait()
	}
	attempts := state.attempts
	completed := state.completed && !state.invalid && state.consumerAttempts == 1 && attempts == 1
	cleanup := state.takeCleanupIfIdleLocked()
	state.mu.Unlock()
	state.runCleanup(cleanup)
	return attempts, completed
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) discard() {
	state.mu.Lock()
	state.active = false
	state.invalid = true
	cleanup := state.takeCleanupIfIdleLocked()
	state.mu.Unlock()
	state.runCleanup(cleanup)
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) takeCleanupIfIdleLocked() func() {
	if state.active || state.inUse || state.cleanup == nil {
		return nil
	}
	cleanup := state.cleanup
	state.cleanup = nil
	state.cleanupInFlight = true
	return cleanup
}

func (state *accountFlowHostEvidenceProjectionUseStateV1) runCleanup(cleanup func()) {
	if cleanup == nil {
		return
	}
	defer func() {
		state.mu.Lock()
		state.cleanupInFlight = false
		state.condition.Broadcast()
		state.mu.Unlock()
	}()
	cleanup()
}

// ProjectAnalyzeAccountFlowsResultV1 performs the sole transition from the
// exact native result into provider-safe semantics plus a deferred one-use
// host-private projection. The exact result is fully revalidated first. Every
// native source locator is then resolved exactly once by the caller; only
// canonical pre-existing SourceRowRecordV1 identities are admitted. No
// external consumer is invoked by this function.
func ProjectAnalyzeAccountFlowsResultV1(
	result AnalyzeAccountFlowsResultV1,
	arguments AnalyzeAccountFlowsArgumentsV1,
	resolveSourceRow AccountFlowSourceRowResolverV1,
	resolveCounterparty AccountFlowCounterpartyResolverV1,
) (AccountFlowProviderSemanticResultV1, AccountFlowHostEvidenceProjectionV1, error) {
	zero := AccountFlowProviderSemanticResultV1{}
	zeroProjection := AccountFlowHostEvidenceProjectionV1{}
	if resolveSourceRow == nil || resolveCounterparty == nil ||
		ValidateAnalyzeAccountFlowsResultV1(result, arguments) != nil {
		return zero, zeroProjection, ErrResultInvalid
	}

	coverage := projectAccountFlowProviderCoverageV1(result.Coverage)
	semanticRows := make([]AccountFlowProviderSemanticTransactionV1, result.evidenceRows.count)
	hostRows := make([]accountFlowHostEvidenceRowProjectionV1, result.evidenceRows.count)
	seenSourceRecordIDs := make(map[string]struct{}, result.evidenceRows.count)
	resolvedReferenceByAccount := make(map[string]domaincaseentity.ReferenceV1, result.evidenceRows.count)
	resolvedAccountByReference := make(map[domaincaseentity.ReferenceV1]string, result.evidenceRows.count)
	resolvedReferenceByAlias := make(map[domaincaseentity.ModelEntityAliasV1]domaincaseentity.ReferenceV1, result.evidenceRows.count)
	resolvedDisplayByReference := make(map[domaincaseentity.ReferenceV1]domaincaseentity.DisplayLabelV1, result.evidenceRows.count)
	resolvedReferenceByDisplay := make(map[string]domaincaseentity.ReferenceV1, result.evidenceRows.count)
	rowIndexesByReference := make(map[domaincaseentity.ReferenceV1][]int, result.evidenceRows.count)
	conflictingReferences := make(map[domaincaseentity.ReferenceV1]struct{}, result.evidenceRows.count)
	counterpartySemanticsComplete := true
	var sourceRowResolutionErr error
	var counterpartyResolutionErr error
	if err := result.evidenceRows.useExact(func(index int, row AccountFlowEvidenceRowV1) error {
		var sourceRecordID string
		var privateSourceFileID string
		counterparty := AccountFlowProviderCounterpartyV1{Status: AccountFlowCounterpartyUnresolvedV1}
		var sourceResolutionErr error
		if err := row.private.useExact(func(private accountFlowEvidencePrivateValuesV1) error {
			privateSourceFileID = strings.Clone(private.sourceFileID)
			resolutionState := newAccountFlowSourceRowResolutionStateV1()
			var resolveErr error
			var acceptCalls uint32
			var acceptCompleted bool
			var resolved string
			func() {
				defer func() {
					acceptCalls, acceptCompleted, resolved = resolutionState.close()
				}()
				resolveErr = resolveSourceRow(
					private.sourceFileID,
					row.SourceRowNumber,
					resolutionState.accept,
				)
			}()
			if resolveErr != nil {
				sourceResolutionErr = resolveErr
				return ErrResultInvalid
			}
			if acceptCalls != 1 || !acceptCompleted ||
				!validAccountFlowSourceRecordIDV1(resolved) {
				return ErrResultInvalid
			}
			sourceRecordID = resolved

			if !private.counterpartyKeyPresent {
				counterpartySemanticsComplete = false
				return nil
			}
			canonicalAccount, canonicalErr := domaincontrolledaccount.CanonicalFinancialAccountTextV2(
				private.counterpartyKey,
			)
			if canonicalErr != nil {
				counterpartySemanticsComplete = false
				return nil
			}
			bankInstitution := ""
			if private.counterpartyBankPresent {
				bankInstitution = private.counterpartyBank
			}
			counterpartyState := newAccountFlowCounterpartyResolutionStateV1()
			var counterpartyResolveErr error
			var counterpartyAcceptCalls uint32
			var counterpartyAcceptCompleted bool
			var reference domaincaseentity.ReferenceV1
			var display domaincaseentity.DisplayLabelV1
			func() {
				defer func() {
					counterpartyAcceptCalls, counterpartyAcceptCompleted, reference, display = counterpartyState.close()
				}()
				counterpartyResolveErr = resolveCounterparty(
					canonicalAccount,
					bankInstitution,
					counterpartyState.accept,
				)
			}()
			if errors.Is(counterpartyResolveErr, ErrAccountFlowCounterpartySemanticUnavailableV1) {
				if counterpartyAcceptCalls != 0 || counterpartyAcceptCompleted {
					return ErrResultInvalid
				}
				counterpartySemanticsComplete = false
				return nil
			}
			if counterpartyResolveErr != nil {
				counterpartyResolutionErr = counterpartyResolveErr
				return ErrResultInvalid
			}
			if counterpartyAcceptCalls != 1 || !counterpartyAcceptCompleted ||
				domaincaseentity.ValidateReferenceV1(string(reference)) != nil ||
				domaincaseentity.ValidateDisplayLabelV1(display) != nil {
				return ErrResultInvalid
			}
			alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(display.EntityType, display.StableOrdinal)
			if aliasErr != nil {
				return ErrResultInvalid
			}
			if previous, exists := resolvedReferenceByAccount[canonicalAccount]; exists && previous != reference {
				return ErrResultInvalid
			}
			if previous, exists := resolvedAccountByReference[reference]; exists && previous != canonicalAccount {
				return ErrResultInvalid
			}
			if previous, exists := resolvedReferenceByAlias[alias]; exists && previous != reference {
				return ErrResultInvalid
			}
			if previous, exists := resolvedReferenceByDisplay[display.Text]; exists && previous != reference {
				return ErrResultInvalid
			}
			resolvedReferenceByAccount[canonicalAccount] = reference
			resolvedAccountByReference[reference] = canonicalAccount
			resolvedReferenceByAlias[alias] = reference
			resolvedReferenceByDisplay[display.Text] = reference
			status := AccountFlowCounterpartyResolvedV1
			if display.Institution == "" {
				status = AccountFlowCounterpartyPartialV1
				counterpartySemanticsComplete = false
			}
			if previous, exists := resolvedDisplayByReference[reference]; exists && previous != display {
				status = AccountFlowCounterpartyUnresolvedV1
				counterpartySemanticsComplete = false
				conflictingReferences[reference] = struct{}{}
				for _, previousIndex := range rowIndexesByReference[reference] {
					semanticRows[previousIndex].Counterparty = AccountFlowProviderCounterpartyV1{
						Status: AccountFlowCounterpartyUnresolvedV1,
					}
				}
			} else if !exists {
				resolvedDisplayByReference[reference] = display
			}
			if _, conflicting := conflictingReferences[reference]; conflicting {
				status = AccountFlowCounterpartyUnresolvedV1
				counterpartySemanticsComplete = false
			}
			rowIndexesByReference[reference] = append(rowIndexesByReference[reference], index)
			counterparty = AccountFlowProviderCounterpartyV1{Status: status}
			if status != AccountFlowCounterpartyUnresolvedV1 {
				counterparty.Alias = alias
				counterparty.EntityType = display.EntityType
				counterparty.AccountType = display.AccountType
			}
			return nil
		}); err != nil {
			if sourceResolutionErr != nil {
				sourceRowResolutionErr = sourceResolutionErr
				return sourceResolutionErr
			}
			if counterpartyResolutionErr != nil {
				return counterpartyResolutionErr
			}
			return err
		}
		if _, duplicate := seenSourceRecordIDs[sourceRecordID]; duplicate {
			return ErrResultInvalid
		}
		seenSourceRecordIDs[sourceRecordID] = struct{}{}
		semanticRows[index] = AccountFlowProviderSemanticTransactionV1{
			EvidenceRef:    sourceRecordID,
			Counterparty:   counterparty,
			OccurredAt:     row.OccurredAt,
			Direction:      row.Direction,
			AmountMinor:    row.AmountMinor,
			Currency:       row.Currency,
			MinorUnitScale: row.MinorUnitScale,
		}
		hostRows[index] = accountFlowHostEvidenceRowProjectionV1{
			subjectRef:      strings.Clone(row.SubjectRef),
			sourceRecordID:  strings.Clone(sourceRecordID),
			sourceFileID:    strings.Clone(privateSourceFileID),
			sourceRowNumber: row.SourceRowNumber,
			occurredAt:      strings.Clone(row.OccurredAt),
			direction:       strings.Clone(row.Direction),
			amountMinor:     strings.Clone(row.AmountMinor),
			currency:        strings.Clone(row.Currency),
			minorUnitScale:  row.MinorUnitScale,
		}
		return nil
	}); err != nil {
		if sourceRowResolutionErr != nil {
			return zero, zeroProjection, sourceRowResolutionErr
		}
		if counterpartyResolutionErr != nil {
			return zero, zeroProjection, counterpartyResolutionErr
		}
		return zero, zeroProjection, err
	}
	if len(semanticRows) != result.EvidenceRowCountV1() || len(hostRows) != result.EvidenceRowCountV1() {
		return zero, zeroProjection, ErrResultInvalid
	}
	if !counterpartySemanticsComplete {
		coverage.State = AccountFlowCoveragePartialV1
		coverage.Gaps = append(coverage.Gaps, AccountFlowGapCounterpartyResolutionV1)
	}

	semantic := AccountFlowProviderSemanticResultV1{
		SubjectAlias:                  arguments.subjectAlias,
		StartInclusive:                result.StartInclusive,
		EndInclusive:                  result.EndInclusive,
		Timezone:                      result.Timezone,
		Currency:                      result.Currency,
		MinorUnitScale:                result.MinorUnitScale,
		InflowMinor:                   result.InflowMinor,
		OutflowMinor:                  result.OutflowMinor,
		NetMinor:                      result.NetMinor,
		TransactionCount:              result.TransactionCount,
		EvidenceTransactionCount:      uint64(len(semanticRows)),
		EvidenceRowLimit:              arguments.evidenceRowLimit,
		AggregateComplete:             result.AggregateComplete,
		EvidenceRowsComplete:          result.EvidenceRowsComplete,
		CounterpartySemanticsComplete: counterpartySemanticsComplete,
		Currentness:                   AccountFlowProviderCurrentnessCurrentV1,
		Coverage:                      cloneAccountFlowProviderCoverageV1(coverage),
		Transactions:                  semanticRows,
		QueryHash:                     result.QueryHash,
		ResultHash:                    result.ResultHash,
	}
	providerOutcome, err := NewAccountFlowProviderOutcomeV1(
		semantic.SubjectAlias,
		semantic.AggregateComplete,
		semantic.EvidenceRowsComplete,
		semantic.QueryHash,
		semantic.ResultHash,
	)
	if err != nil {
		return zero, zeroProjection, ErrResultInvalid
	}
	semantic.Outcome = providerOutcome
	summary := accountFlowHostEvidenceSummaryV1{
		subjectRef:           strings.Clone(result.SubjectRef),
		datasetSnapshotID:    strings.Clone(result.Provenance.DatasetSnapshotID),
		contextEpoch:         result.Provenance.ContextEpoch,
		contextDigest:        strings.Clone(result.Provenance.ContextDigest),
		caseBindingHash:      strings.Clone(result.Provenance.CaseBindingHash),
		startInclusive:       strings.Clone(result.StartInclusive),
		endInclusive:         strings.Clone(result.EndInclusive),
		timezone:             strings.Clone(result.Timezone),
		currency:             strings.Clone(result.Currency),
		minorUnitScale:       result.MinorUnitScale,
		inflowMinor:          strings.Clone(result.InflowMinor),
		outflowMinor:         strings.Clone(result.OutflowMinor),
		netMinor:             strings.Clone(result.NetMinor),
		transactionCount:     result.TransactionCount,
		aggregateComplete:    result.AggregateComplete,
		evidenceRowsComplete: result.EvidenceRowsComplete,
		evidenceRowCount:     uint64(len(hostRows)),
		coverage:             cloneAccountFlowProviderCoverageV1(coverage),
		queryHash:            strings.Clone(result.QueryHash),
		resultHash:           strings.Clone(result.ResultHash),
	}

	useState := newAccountFlowHostEvidenceProjectionUseStateV1(
		func() {
			summary = accountFlowHostEvidenceSummaryV1{}
			clear(hostRows)
			hostRows = nil
		},
	)
	projection := AccountFlowHostEvidenceProjectionV1{
		use: func(
			consumeSummary AccountFlowHostEvidenceSummaryConsumerV1,
			consumeRow AccountFlowHostEvidenceRowConsumerV1,
		) error {
			if !useState.begin() {
				return ErrResultInvalid
			}
			succeeded := false
			defer func() { useState.end(succeeded) }()
			if consumeSummary == nil || consumeRow == nil {
				return ErrResultInvalid
			}
			if err := consumeSummary(
				summary.subjectRef,
				summary.datasetSnapshotID,
				summary.contextEpoch,
				summary.contextDigest,
				summary.caseBindingHash,
				summary.startInclusive,
				summary.endInclusive,
				summary.timezone,
				summary.currency,
				summary.minorUnitScale,
				summary.inflowMinor,
				summary.outflowMinor,
				summary.netMinor,
				summary.transactionCount,
				summary.aggregateComplete,
				summary.evidenceRowsComplete,
				summary.evidenceRowCount,
				cloneAccountFlowProviderCoverageV1(summary.coverage),
				summary.queryHash,
				summary.resultHash,
			); err != nil {
				return err
			}
			for index, row := range hostRows {
				if !useState.isActive() {
					return ErrResultInvalid
				}
				if err := consumeRow(
					index,
					row.subjectRef,
					row.sourceRecordID,
					row.sourceFileID,
					row.sourceRowNumber,
					row.occurredAt,
					row.direction,
					row.amountMinor,
					row.currency,
					row.minorUnitScale,
				); err != nil {
					return err
				}
			}
			if !useState.isActive() {
				return ErrResultInvalid
			}
			succeeded = true
			return nil
		},
		beginConsume: useState.beginConsumer,
		close:        useState.close,
		discard:      useState.discard,
	}
	return semantic, projection, nil
}

func projectAccountFlowProviderCoverageV1(coverage AccountFlowCoverageV1) AccountFlowProviderSemanticCoverageV1 {
	return AccountFlowProviderSemanticCoverageV1{
		State:                  coverage.State,
		Gaps:                   cloneAccountFlowStringsV1(coverage.Gaps),
		NormalizedSnapshotRows: coverage.NormalizedSnapshotRows,
		AcceptedSnapshotRows:   coverage.AcceptedSnapshotRows,
		RejectedSnapshotRows:   coverage.RejectedSnapshotRows,
		DuplicateSnapshotRows:  coverage.DuplicateSnapshotRows,
		UntimedSubjectRows:     coverage.UntimedSubjectRows,
		ObservedMatchingRows:   coverage.ObservedMatchingRows,
	}
}

func cloneAccountFlowProviderCoverageV1(
	coverage AccountFlowProviderSemanticCoverageV1,
) AccountFlowProviderSemanticCoverageV1 {
	coverage.Gaps = cloneAccountFlowStringsV1(coverage.Gaps)
	return coverage
}

func ValidateAccountFlowProviderCounterpartyV1(
	counterparty AccountFlowProviderCounterpartyV1,
) error {
	switch counterparty.Status {
	case AccountFlowCounterpartyResolvedV1:
		if counterparty.Alias == "" {
			return ErrResultInvalid
		}
	case AccountFlowCounterpartyPartialV1:
		if counterparty.Alias == "" {
			return ErrResultInvalid
		}
	case AccountFlowCounterpartyUnresolvedV1:
		if counterparty.Alias != "" || counterparty.EntityType != "" ||
			counterparty.AccountType != "" {
			return ErrResultInvalid
		}
		return nil
	default:
		return ErrResultInvalid
	}
	entityType, _, err := domaincaseentity.ParseModelEntityAliasV1(string(counterparty.Alias))
	if err != nil || counterparty.EntityType !=
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 ||
		entityType != counterparty.EntityType ||
		counterparty.AccountType != AccountFlowCounterpartyAccountTypeV1 {
		return ErrResultInvalid
	}
	return nil
}

func validAccountFlowSourceRecordIDV1(value string) bool {
	return value == strings.TrimSpace(value) && strings.HasPrefix(value, accountFlowSourceRecordIDPrefixV1) &&
		domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, accountFlowSourceRecordIDPrefixV1))
}
