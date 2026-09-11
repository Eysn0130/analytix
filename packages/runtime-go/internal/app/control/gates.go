package control

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

type GateRecord struct {
	ThreadID              string
	TurnID                string
	ItemID                string
	ToolName              string
	Prompt                string
	ContinuationReceiptID string
}

type PendingGateState[T any] struct {
	Record   GateRecord
	Pending  T
	Restored bool
}

type DrainedGate[T any] struct {
	Kind                 string
	ID                   string
	ClaimToken           uint64
	Record               GateRecord
	Pending              T
	ResolutionStatus     string
	ResolutionReasonCode string
	TerminalStatus       string
	TerminalReasonCode   string
}

// GateClaim is the exclusive same-process continuation owner. The monotonic
// token prevents an old owner from committing or promoting a later claim for
// the same public gate identity.
type GateClaim[T any] struct {
	Kind                 string
	ID                   string
	ClaimToken           uint64
	Record               GateRecord
	Pending              T
	ResolutionStatus     string
	ResolutionReasonCode string
}

type GateRegistry[T any] struct {
	mu                 sync.Mutex
	approvalRecords    map[string]GateRecord
	inputRecords       map[string]GateRecord
	reservedApprovals  map[string]T
	reservedInputs     map[string]T
	pendingApprovals   map[string]T
	pendingInputs      map[string]T
	continuationClaims map[string]GateClaim[T]
	terminalClaims     map[string]DrainedGate[T]
	nextClaimToken     uint64
}

func NewGateRegistry[T any]() *GateRegistry[T] {
	return &GateRegistry[T]{
		approvalRecords:    map[string]GateRecord{},
		inputRecords:       map[string]GateRecord{},
		reservedApprovals:  map[string]T{},
		reservedInputs:     map[string]T{},
		pendingApprovals:   map[string]T{},
		pendingInputs:      map[string]T{},
		continuationClaims: map[string]GateClaim[T]{},
		terminalClaims:     map[string]DrainedGate[T]{},
	}
}

// ReservePendingApproval installs a non-executable request reservation. An
// exact retry is idempotent, while any reuse of the gate id with different
// authority fails closed. ClaimApproval cannot observe the reservation until
// ActivateReservedApproval commits the complete durable public projection.
func (r *GateRegistry[T]) ReservePendingApproval(id string, record GateRecord, pending T) bool {
	return r.reservePending("approval", id, record, pending)
}

// ReservePendingUserInput is the user-input counterpart of
// ReservePendingApproval.
func (r *GateRegistry[T]) ReservePendingUserInput(id string, record GateRecord, pending T) bool {
	return r.reservePending("user_input", id, record, pending)
}

func (r *GateRegistry[T]) reservePending(kind, id string, record GateRecord, pending T) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	record = normalizeGateRecord(record)
	if id == "" {
		return false
	}
	switch kind {
	case "approval":
		if existing, ok := r.approvalRecords[id]; ok {
			if !reflect.DeepEqual(existing, record) {
				return false
			}
			if reserved, ok := r.reservedApprovals[id]; ok {
				return reflect.DeepEqual(reserved, pending)
			}
			if active, ok := r.pendingApprovals[id]; ok {
				return reflect.DeepEqual(active, pending)
			}
			return false
		}
		if r.gateIDExists(id) {
			return false
		}
		r.approvalRecords[id] = record
		r.reservedApprovals[id] = pending
		return true
	case "user_input":
		if existing, ok := r.inputRecords[id]; ok {
			if !reflect.DeepEqual(existing, record) {
				return false
			}
			if reserved, ok := r.reservedInputs[id]; ok {
				return reflect.DeepEqual(reserved, pending)
			}
			if active, ok := r.pendingInputs[id]; ok {
				return reflect.DeepEqual(active, pending)
			}
			return false
		}
		if r.gateIDExists(id) {
			return false
		}
		r.inputRecords[id] = record
		r.reservedInputs[id] = pending
		return true
	default:
		return false
	}
}

// ActivateReservedApproval is the only transition that makes a staged
// approval claimable by a provider-originated response. Exact retries after a
// committed activation are idempotent.
func (r *GateRegistry[T]) ActivateReservedApproval(id string, pending T) bool {
	return r.activateReserved("approval", id, pending)
}

// ActivateReservedUserInput is the user-input counterpart of
// ActivateReservedApproval.
func (r *GateRegistry[T]) ActivateReservedUserInput(id string, pending T) bool {
	return r.activateReserved("user_input", id, pending)
}

func (r *GateRegistry[T]) activateReserved(kind, id string, pending T) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	switch kind {
	case "approval":
		if active, ok := r.pendingApprovals[id]; ok {
			return reflect.DeepEqual(active, pending)
		}
		reserved, ok := r.reservedApprovals[id]
		if !ok || !reflect.DeepEqual(reserved, pending) {
			return false
		}
		delete(r.reservedApprovals, id)
		r.pendingApprovals[id] = reserved
		return true
	case "user_input":
		if active, ok := r.pendingInputs[id]; ok {
			return reflect.DeepEqual(active, pending)
		}
		reserved, ok := r.reservedInputs[id]
		if !ok || !reflect.DeepEqual(reserved, pending) {
			return false
		}
		delete(r.reservedInputs, id)
		r.pendingInputs[id] = reserved
		return true
	default:
		return false
	}
}

func TurnKey(threadID, turnID string) string {
	return strings.TrimSpace(threadID) + "\x00" + strings.TrimSpace(turnID)
}

func (r *GateRegistry[T]) RegisterApproval(id string, record GateRecord) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.gateIDExists(id) {
		return false
	}
	r.approvalRecords[id] = normalizeGateRecord(record)
	return true
}

func (r *GateRegistry[T]) RegisterUserInput(id string, record GateRecord) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.gateIDExists(id) {
		return false
	}
	r.inputRecords[id] = normalizeGateRecord(record)
	return true
}

func (r *GateRegistry[T]) SetPendingApproval(id string, pending T) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.pendingIDExists(id) {
		return false
	}
	if _, ok := r.approvalRecords[id]; !ok {
		return false
	}
	r.pendingApprovals[id] = pending
	return true
}

func (r *GateRegistry[T]) SetPendingUserInput(id string, pending T) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.pendingIDExists(id) {
		return false
	}
	if _, ok := r.inputRecords[id]; !ok {
		return false
	}
	r.pendingInputs[id] = pending
	return true
}

func (r *GateRegistry[T]) RegisterPendingApproval(id string, record GateRecord, pending T) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.gateIDExists(id) {
		return false
	}
	r.approvalRecords[id] = normalizeGateRecord(record)
	r.pendingApprovals[id] = pending
	return true
}

func (r *GateRegistry[T]) RegisterPendingUserInput(id string, record GateRecord, pending T) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.gateIDExists(id) {
		return false
	}
	r.inputRecords[id] = normalizeGateRecord(record)
	r.pendingInputs[id] = pending
	return true
}

func (r *GateRegistry[T]) RestoreApproval(id string, state PendingGateState[T]) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.gateIDExists(id) {
		return false
	}
	r.approvalRecords[id] = normalizeGateRecord(state.Record)
	return true
}

func (r *GateRegistry[T]) RestoreUserInput(id string, state PendingGateState[T]) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" || r.gateIDExists(id) {
		return false
	}
	r.inputRecords[id] = normalizeGateRecord(state.Record)
	return true
}

func (r *GateRegistry[T]) DeleteApproval(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	delete(r.approvalRecords, id)
	delete(r.reservedApprovals, id)
	delete(r.pendingApprovals, id)
	delete(r.continuationClaims, id)
	delete(r.terminalClaims, id)
}

func (r *GateRegistry[T]) DeleteUserInput(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	delete(r.inputRecords, id)
	delete(r.reservedInputs, id)
	delete(r.pendingInputs, id)
	delete(r.continuationClaims, id)
	delete(r.terminalClaims, id)
}

func (r *GateRegistry[T]) ClaimApproval(id string) (GateClaim[T], bool) {
	return r.claimContinuation("approval", id)
}

func (r *GateRegistry[T]) ClaimUserInput(id string) (GateClaim[T], bool) {
	return r.claimContinuation("user_input", id)
}

func (r *GateRegistry[T]) claimContinuation(kind, id string) (GateClaim[T], bool) {
	var zero GateClaim[T]
	if r == nil {
		return zero, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return zero, false
	}
	var record GateRecord
	var pending T
	var exists bool
	switch kind {
	case "approval":
		record, exists = r.approvalRecords[id]
		if !exists {
			return zero, false
		}
		pending, exists = r.pendingApprovals[id]
		if exists {
			delete(r.pendingApprovals, id)
		}
	case "user_input":
		record, exists = r.inputRecords[id]
		if !exists {
			return zero, false
		}
		pending, exists = r.pendingInputs[id]
		if exists {
			delete(r.pendingInputs, id)
		}
	default:
		return zero, false
	}
	if !exists {
		return zero, false
	}
	r.nextClaimToken++
	if r.nextClaimToken == 0 {
		r.nextClaimToken++
	}
	claim := GateClaim[T]{
		Kind: kind, ID: id, ClaimToken: r.nextClaimToken,
		Record: record, Pending: pending,
	}
	r.continuationClaims[id] = claim
	return claim, true
}

func (r *GateRegistry[T]) PromoteClaimToTerminal(claim GateClaim[T]) (DrainedGate[T], bool) {
	return r.PromoteClaimToTerminalWithIntent(claim, "", "")
}

func (r *GateRegistry[T]) PromoteClaimToTerminalWithIntent(
	claim GateClaim[T],
	status, reasonCode string,
) (DrainedGate[T], bool) {
	var zero DrainedGate[T]
	if r == nil {
		return zero, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	status = strings.TrimSpace(status)
	reasonCode = strings.TrimSpace(reasonCode)
	if (status == "") != (reasonCode == "") ||
		(status != "" && status != domaincontinuation.StatusInterrupted && status != domaincontinuation.StatusRejected) {
		return zero, false
	}
	stored, exists := r.continuationClaims[strings.TrimSpace(claim.ID)]
	if !exists || !reflect.DeepEqual(stored, claim) {
		return zero, false
	}
	drained := DrainedGate[T]{
		Kind: stored.Kind, ID: stored.ID, ClaimToken: stored.ClaimToken,
		Record: stored.Record, Pending: stored.Pending,
		ResolutionStatus: stored.ResolutionStatus, ResolutionReasonCode: stored.ResolutionReasonCode,
		TerminalStatus: status, TerminalReasonCode: reasonCode,
	}
	delete(r.continuationClaims, stored.ID)
	r.terminalClaims[stored.ID] = drained
	return drained, true
}

func (r *GateRegistry[T]) MarkClaimDisposition(
	claim GateClaim[T],
	status, reasonCode string,
) (GateClaim[T], bool) {
	var zero GateClaim[T]
	if r == nil {
		return zero, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := strings.TrimSpace(claim.ID)
	stored, exists := r.continuationClaims[id]
	status = strings.TrimSpace(status)
	reasonCode = strings.TrimSpace(reasonCode)
	if !exists || !reflect.DeepEqual(stored, claim) || status == "" || reasonCode == "" ||
		stored.ResolutionStatus != "" || stored.ResolutionReasonCode != "" {
		return zero, false
	}
	if _, ok := domaincontinuation.ResolveProjection(stored.Kind, status, reasonCode); !ok {
		return zero, false
	}
	stored.ResolutionStatus = status
	stored.ResolutionReasonCode = reasonCode
	r.continuationClaims[id] = stored
	return stored, true
}

func (r *GateRegistry[T]) CommitContinuationClaim(claim GateClaim[T]) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := strings.TrimSpace(claim.ID)
	stored, exists := r.continuationClaims[id]
	if !exists || !reflect.DeepEqual(stored, claim) {
		return false
	}
	delete(r.continuationClaims, id)
	return true
}

func (r *GateRegistry[T]) ResolveApproval(id string) (GateRecord, T, bool, bool) {
	var zero T
	if r == nil {
		return GateRecord{}, zero, false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	record, exists := r.approvalRecords[id]
	pending, hasPending := r.pendingApprovals[id]
	if hasPending {
		delete(r.pendingApprovals, id)
	}
	return record, pending, exists, hasPending
}

func (r *GateRegistry[T]) PeekApproval(id string) (GateRecord, T, bool, bool) {
	var zero T
	if r == nil {
		return GateRecord{}, zero, false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	record, exists := r.approvalRecords[id]
	pending, hasPending := r.pendingApprovals[id]
	return record, pending, exists, hasPending
}

func (r *GateRegistry[T]) ResolveUserInput(id string) (GateRecord, T, bool, bool) {
	var zero T
	if r == nil {
		return GateRecord{}, zero, false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	record, exists := r.inputRecords[id]
	pending, hasPending := r.pendingInputs[id]
	if hasPending {
		delete(r.pendingInputs, id)
	}
	return record, pending, exists, hasPending
}

func (r *GateRegistry[T]) PeekUserInput(id string) (GateRecord, T, bool, bool) {
	var zero T
	if r == nil {
		return GateRecord{}, zero, false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id = strings.TrimSpace(id)
	record, exists := r.inputRecords[id]
	pending, hasPending := r.pendingInputs[id]
	return record, pending, exists, hasPending
}

func (r *GateRegistry[T]) DrainForTurn(threadID, turnID string) []DrainedGate[T] {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	drained := []DrainedGate[T]{}
	for id, record := range r.approvalRecords {
		if record.ThreadID != threadID || record.TurnID != turnID {
			continue
		}
		if pending, ok := r.reservedApprovals[id]; ok {
			delete(r.reservedApprovals, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("approval", id, record, pending)
		} else if pending, ok := r.pendingApprovals[id]; ok {
			delete(r.pendingApprovals, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("approval", id, record, pending)
		}
	}
	for id, record := range r.inputRecords {
		if record.ThreadID != threadID || record.TurnID != turnID {
			continue
		}
		if pending, ok := r.reservedInputs[id]; ok {
			delete(r.reservedInputs, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("user_input", id, record, pending)
		} else if pending, ok := r.pendingInputs[id]; ok {
			delete(r.pendingInputs, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("user_input", id, record, pending)
		}
	}
	r.promoteContinuationClaimsForTurnLocked(threadID, turnID)
	for _, claim := range r.terminalClaims {
		if claim.Record.ThreadID == threadID && claim.Record.TurnID == turnID {
			drained = append(drained, claim)
		}
	}
	sortDrainedGates(drained)
	return drained
}

// DrainForThread atomically claims every executable paused gate owned by one
// thread. It is used by workspace/epoch mutations after writer reservation
// and before the old authority can be invalidated.
func (r *GateRegistry[T]) DrainForThread(threadID string) []DrainedGate[T] {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	threadID = strings.TrimSpace(threadID)
	drained := []DrainedGate[T]{}
	for id, record := range r.approvalRecords {
		if record.ThreadID != threadID {
			continue
		}
		if pending, ok := r.reservedApprovals[id]; ok {
			delete(r.reservedApprovals, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("approval", id, record, pending)
		} else if pending, ok := r.pendingApprovals[id]; ok {
			delete(r.pendingApprovals, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("approval", id, record, pending)
		}
	}
	for id, record := range r.inputRecords {
		if record.ThreadID != threadID {
			continue
		}
		if pending, ok := r.reservedInputs[id]; ok {
			delete(r.reservedInputs, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("user_input", id, record, pending)
		} else if pending, ok := r.pendingInputs[id]; ok {
			delete(r.pendingInputs, id)
			r.terminalClaims[id] = r.newTerminalClaimLocked("user_input", id, record, pending)
		}
	}
	r.promoteContinuationClaimsForThreadLocked(threadID)
	for _, claim := range r.terminalClaims {
		if claim.Record.ThreadID == threadID {
			drained = append(drained, claim)
		}
	}
	sortDrainedGates(drained)
	return drained
}

// DrainAll atomically removes every executable pending gate while preserving
// its immutable display record for replay. Results are sorted so shutdown and
// recovery settlement are deterministic.
func (r *GateRegistry[T]) DrainAll() []DrainedGate[T] {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	drained := make([]DrainedGate[T], 0, len(r.reservedApprovals)+len(r.reservedInputs)+len(r.pendingApprovals)+len(r.pendingInputs)+len(r.continuationClaims)+len(r.terminalClaims))
	for id, pending := range r.reservedApprovals {
		r.terminalClaims[id] = r.newTerminalClaimLocked("approval", id, r.approvalRecords[id], pending)
		delete(r.reservedApprovals, id)
	}
	for id, pending := range r.reservedInputs {
		r.terminalClaims[id] = r.newTerminalClaimLocked("user_input", id, r.inputRecords[id], pending)
		delete(r.reservedInputs, id)
	}
	for id, pending := range r.pendingApprovals {
		r.terminalClaims[id] = r.newTerminalClaimLocked("approval", id, r.approvalRecords[id], pending)
		delete(r.pendingApprovals, id)
	}
	for id, pending := range r.pendingInputs {
		r.terminalClaims[id] = r.newTerminalClaimLocked("user_input", id, r.inputRecords[id], pending)
		delete(r.pendingInputs, id)
	}
	for id, claim := range r.continuationClaims {
		r.terminalClaims[id] = terminalClaimFromContinuation(claim)
		delete(r.continuationClaims, id)
	}
	for _, claim := range r.terminalClaims {
		drained = append(drained, claim)
	}
	sortDrainedGates(drained)
	return drained
}

func (r *GateRegistry[T]) promoteContinuationClaimsForTurnLocked(threadID, turnID string) {
	for id, claim := range r.continuationClaims {
		if claim.Record.ThreadID == threadID && claim.Record.TurnID == turnID {
			r.terminalClaims[id] = terminalClaimFromContinuation(claim)
			delete(r.continuationClaims, id)
		}
	}
}

func (r *GateRegistry[T]) promoteContinuationClaimsForThreadLocked(threadID string) {
	for id, claim := range r.continuationClaims {
		if claim.Record.ThreadID == threadID {
			r.terminalClaims[id] = terminalClaimFromContinuation(claim)
			delete(r.continuationClaims, id)
		}
	}
}

func terminalClaimFromContinuation[T any](claim GateClaim[T]) DrainedGate[T] {
	return DrainedGate[T]{
		Kind: claim.Kind, ID: claim.ID, ClaimToken: claim.ClaimToken,
		Record: claim.Record, Pending: claim.Pending,
		ResolutionStatus: claim.ResolutionStatus, ResolutionReasonCode: claim.ResolutionReasonCode,
	}
}

func (r *GateRegistry[T]) newTerminalClaimLocked(kind, id string, record GateRecord, pending T) DrainedGate[T] {
	r.nextClaimToken++
	if r.nextClaimToken == 0 {
		r.nextClaimToken++
	}
	return DrainedGate[T]{
		Kind: kind, ID: id, ClaimToken: r.nextClaimToken,
		Record: record, Pending: pending,
	}
}

func sortDrainedGates[T any](drained []DrainedGate[T]) {
	sort.Slice(drained, func(left, right int) bool {
		if drained[left].Record.ThreadID != drained[right].Record.ThreadID {
			return drained[left].Record.ThreadID < drained[right].Record.ThreadID
		}
		if drained[left].Record.TurnID != drained[right].Record.TurnID {
			return drained[left].Record.TurnID < drained[right].Record.TurnID
		}
		if drained[left].Kind != drained[right].Kind {
			return drained[left].Kind < drained[right].Kind
		}
		return drained[left].ID < drained[right].ID
	})
}

// RestoreDrained verifies that an uncommitted terminal claim remains retained.
// It deliberately does not return the gate to executable pending state: a
// partially settled receipt/item/event must be retryable only by the same
// host terminal path, never by a user approval or input response.
func (r *GateRegistry[T]) RestoreDrained(drained []DrainedGate[T]) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, gate := range drained {
		id := strings.TrimSpace(gate.ID)
		claim, exists := r.terminalClaims[id]
		if id == "" || !exists || !terminalClaimMatches(claim, gate) {
			return false
		}
	}
	return true
}

// BindTerminalIntent freezes the first host terminal disposition for every
// exact claim before external side effects begin. A later retry receives the
// frozen intent even when a different shutdown or context-transition reason
// triggered that retry.
func (r *GateRegistry[T]) BindTerminalIntent(
	drained []DrainedGate[T],
	defaultStatus, defaultReason string,
) ([]DrainedGate[T], bool) {
	if r == nil {
		return nil, false
	}
	defaultStatus = strings.TrimSpace(defaultStatus)
	defaultReason = strings.TrimSpace(defaultReason)
	if defaultStatus == "" || defaultReason == "" {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bound := make([]DrainedGate[T], len(drained))
	for index, gate := range drained {
		id := strings.TrimSpace(gate.ID)
		stored, exists := r.terminalClaims[id]
		if id == "" || !exists || !terminalClaimMatches(stored, gate) {
			return nil, false
		}
		status := strings.TrimSpace(stored.TerminalStatus)
		reason := strings.TrimSpace(stored.TerminalReasonCode)
		if (status == "") != (reason == "") {
			return nil, false
		}
		if status == "" {
			stored.TerminalStatus = defaultStatus
			stored.TerminalReasonCode = defaultReason
			r.terminalClaims[id] = stored
		}
		bound[index] = stored
	}
	return bound, true
}

// CommitDrained removes exact terminal claims only after every gate side
// effect and the owning turn terminal have committed. Validation is atomic:
// a mismatched batch removes nothing.
func (r *GateRegistry[T]) CommitDrained(drained []DrainedGate[T]) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, gate := range drained {
		id := strings.TrimSpace(gate.ID)
		claim, exists := r.terminalClaims[id]
		if id == "" || !exists || !terminalClaimMatches(claim, gate) {
			return false
		}
	}
	for _, gate := range drained {
		delete(r.terminalClaims, strings.TrimSpace(gate.ID))
	}
	return true
}

func terminalClaimMatches[T any](stored, candidate DrainedGate[T]) bool {
	if stored.Kind != candidate.Kind || stored.ID != candidate.ID || stored.ClaimToken != candidate.ClaimToken ||
		!reflect.DeepEqual(stored.Record, candidate.Record) || !reflect.DeepEqual(stored.Pending, candidate.Pending) ||
		stored.ResolutionStatus != candidate.ResolutionStatus || stored.ResolutionReasonCode != candidate.ResolutionReasonCode {
		return false
	}
	candidateStatus := strings.TrimSpace(candidate.TerminalStatus)
	candidateReason := strings.TrimSpace(candidate.TerminalReasonCode)
	if (candidateStatus == "") != (candidateReason == "") {
		return false
	}
	if candidateStatus != "" && (stored.TerminalStatus != candidateStatus || stored.TerminalReasonCode != candidateReason) {
		return false
	}
	return true
}

func (r *GateRegistry[T]) gateIDExists(id string) bool {
	if _, ok := r.approvalRecords[id]; ok {
		return true
	}
	if _, ok := r.inputRecords[id]; ok {
		return true
	}
	return r.pendingIDExists(id)
}

func (r *GateRegistry[T]) pendingIDExists(id string) bool {
	if _, ok := r.continuationClaims[id]; ok {
		return true
	}
	if _, ok := r.terminalClaims[id]; ok {
		return true
	}
	if _, ok := r.reservedApprovals[id]; ok {
		return true
	}
	if _, ok := r.reservedInputs[id]; ok {
		return true
	}
	if _, ok := r.pendingApprovals[id]; ok {
		return true
	}
	_, ok := r.pendingInputs[id]
	return ok
}

func DeletePendingGateStatesForTurn[T any](
	approvals map[string]PendingGateState[T],
	inputs map[string]PendingGateState[T],
	threadID string,
	turnID string,
) {
	if strings.TrimSpace(turnID) == "" {
		return
	}
	for approvalID, state := range approvals {
		if state.Record.ThreadID == threadID && state.Record.TurnID == turnID {
			delete(approvals, approvalID)
		}
	}
	for inputID, state := range inputs {
		if state.Record.ThreadID == threadID && state.Record.TurnID == turnID {
			delete(inputs, inputID)
		}
	}
}

func GateRecordToolName(record GateRecord) string {
	toolName := strings.TrimSpace(record.ToolName)
	if toolName == "" {
		return "tool"
	}
	return toolName
}

func GateRecordPrompt(record GateRecord) string {
	prompt := strings.TrimSpace(record.Prompt)
	if prompt == "" {
		return "User input required"
	}
	return prompt
}

func normalizeGateRecord(record GateRecord) GateRecord {
	record.ThreadID = strings.TrimSpace(record.ThreadID)
	record.TurnID = strings.TrimSpace(record.TurnID)
	record.ItemID = strings.TrimSpace(record.ItemID)
	record.ToolName = strings.TrimSpace(record.ToolName)
	record.Prompt = strings.TrimSpace(record.Prompt)
	record.ContinuationReceiptID = strings.TrimSpace(record.ContinuationReceiptID)
	return record
}
