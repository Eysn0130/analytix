package processdisplay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	continuation "analytix.local/runtime-go/internal/domain/continuation"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	toolcall "analytix.local/runtime-go/internal/domain/toolcall"
	toolresult "analytix.local/runtime-go/internal/domain/toolresult"
	port "analytix.local/runtime-go/internal/ports/processdisplay"
)

type LimitsV1 struct {
	MaxTurns, MaxBlocks, MaxViewers, MaxTombstones int
	RawBytes, DisplayBytes                         int
	IdleTTL, TerminalTTL, AuthorityCheckInterval   time.Duration
}

func DefaultLimitsV1() LimitsV1 {
	return LimitsV1{16, 128, 32, 128, 1 << 20, 256 << 10, 5 * time.Minute, 10 * time.Minute, 100 * time.Millisecond}
}

type ResponseV1 struct {
	SchemaVersion int                 `json:"schemaVersion,omitempty"`
	Presence      *display.PresenceV2 `json:"presence,omitempty"`
	Units         []display.UnitV2    `json:"units,omitempty"`
	Unit          *display.UnitV2     `json:"unit,omitempty"`
	Status        string              `json:"status"`
	Revision      string              `json:"revision,omitempty"`
	Scope         *display.ScopeV1    `json:"scope,omitempty"`
	Blocks        []display.BlockV1   `json:"blocks,omitempty"`
	Block         *display.BlockV1    `json:"block,omitempty"`
}

// WebLinkResponseV1 is Main-only: the sealed target never enters an ordinary
// process snapshot, public DTO or renderer-provided request.
type WebLinkResponseV1 struct {
	SchemaVersion int              `json:"schemaVersion,omitempty"`
	Status        string           `json:"status"`
	Revision      string           `json:"revision,omitempty"`
	Scope         *display.ScopeV1 `json:"scope,omitempty"`
	BlockID       string           `json:"blockId,omitempty"`
	InputDigest   string           `json:"inputDigest,omitempty"`
	Role          string           `json:"role,omitempty"`
	TargetURL     string           `json:"targetUrl,omitempty"`
}

type rawBlockV1 struct {
	inputBytes        int
	preparation       *display.PartialArgumentsV1
	preparationName   string
	preparationSealed bool
	value             display.BlockV1
	raw               string
	webRequestURL     string
	overflow          bool
	started           time.Time
	producerHandle    string
	providerSequence  uint64
	key               string
}
type unitV2 struct {
	value               display.UnitV2
	namespace           continuation.ProviderContinuationNamespaceV1
	active              bool
	retired             bool
	preparationObserved bool
}
type turnV1 struct {
	historyAnchor          *display.SettledHistoryAnchorV1
	units                  []*unitV2
	byNamespace            map[string]*unitV2
	byCall                 map[string]*unitV2
	sequenceUnits          map[string]*unitV2
	producerIDs            map[string]string
	sourceOrdinal          uint64
	preparationOrigins     map[string]uint64
	callRefs               int
	binding                port.BindingV1
	incarnation            string
	revision               uint64
	blocks                 []*rawBlockV1
	byKey                  map[string]*rawBlockV1
	wake                   chan struct{}
	activity               time.Time
	terminal               bool
	rawBytes, displayBytes int
	handles                map[string]bool
	currentAttempts        map[string]uint64
	terminalAttempts       map[string]bool
}
type viewerV1 struct {
	token, document string
	thread          string
	turn            *turnV1
	wake            chan struct{}
	polling         bool
	active          bool
	revoked         bool
}
type ProducerV1 struct {
	owner   *ServiceV1
	turn    *turnV1
	handle  string
	secrets []string
}
type ServiceV1 struct {
	mu              sync.Mutex
	authority       port.ReadAuthorityV1
	limits          LimitsV1
	turns           map[string]*turnV1
	viewers         map[string]*viewerV1
	tombstones      map[string]time.Time
	closed          bool
	shutdownDrain   bool
	now             func() time.Time
	janitorStop     chan struct{}
	janitorDone     chan struct{}
	janitorRunning  bool
	janitorInterval time.Duration
	closeOnce       sync.Once
}

func NewServiceV1(authority port.ReadAuthorityV1, limits LimitsV1) *ServiceV1 {
	defaults := DefaultLimitsV1()
	if limits.MaxTurns <= 0 || limits.MaxTurns > defaults.MaxTurns {
		limits.MaxTurns = defaults.MaxTurns
	}
	if limits.MaxBlocks <= 0 || limits.MaxBlocks > defaults.MaxBlocks {
		limits.MaxBlocks = defaults.MaxBlocks
	}
	if limits.MaxViewers <= 0 || limits.MaxViewers > defaults.MaxViewers {
		limits.MaxViewers = defaults.MaxViewers
	}
	if limits.MaxTombstones <= 0 || limits.MaxTombstones > defaults.MaxTombstones {
		limits.MaxTombstones = defaults.MaxTombstones
	}
	if limits.RawBytes <= 0 || limits.RawBytes > display.RawCaptureByteLimitV1 {
		limits.RawBytes = defaults.RawBytes
	}
	if limits.DisplayBytes <= 0 || limits.DisplayBytes > defaults.DisplayBytes {
		limits.DisplayBytes = defaults.DisplayBytes
	}
	if limits.IdleTTL <= 0 {
		limits.IdleTTL = defaults.IdleTTL
	}
	if limits.TerminalTTL <= 0 {
		limits.TerminalTTL = defaults.TerminalTTL
	}
	if limits.AuthorityCheckInterval <= 0 || limits.AuthorityCheckInterval > time.Second {
		limits.AuthorityCheckInterval = defaults.AuthorityCheckInterval
	}
	service := &ServiceV1{authority: authority, limits: limits, turns: map[string]*turnV1{}, viewers: map[string]*viewerV1{}, tombstones: map[string]time.Time{}, now: time.Now, janitorStop: make(chan struct{}), janitorDone: make(chan struct{})}
	// At most one active owner janitor bounds retention even without API
	// activity. It starts only with a turn and exits when no turns remain.
	interval := time.Second
	if limits.IdleTTL < interval {
		interval = limits.IdleTTL
	}
	if limits.TerminalTTL < interval {
		interval = limits.TerminalTTL
	}
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	service.janitorInterval = interval
	close(service.janitorDone) // No janitor is owned before the first turn.
	return service
}

func (s *ServiceV1) startJanitorLockedV1() {
	if s.closed || s.janitorRunning || len(s.turns) == 0 && !s.hasHistoryViewerLockedV1() {
		return
	}
	s.janitorRunning = true
	s.janitorDone = make(chan struct{})
	go s.runJanitorV1(s.janitorDone)
}

func (s *ServiceV1) finishJanitorLockedV1(done chan struct{}) {
	s.janitorRunning = false
	close(done)
}

func (s *ServiceV1) runJanitorV1(done chan struct{}) {
	ticker := time.NewTicker(s.janitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.janitorStop:
			s.mu.Lock()
			s.finishJanitorLockedV1(done)
			s.mu.Unlock()
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.closed {
				s.sweepLockedV1()
			}
			if s.closed || len(s.turns) == 0 && !s.hasHistoryViewerLockedV1() {
				// Mark the worker retired while holding the same gate as
				// Begin, so a concurrent new turn starts its own worker.
				s.finishJanitorLockedV1(done)
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()
		}
	}
}

func scopeKeyV1(thread, turn string) string { return thread + "/" + turn }
func randomIDV1() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes[:])
}
func validBindingV1(b port.BindingV1) bool {
	return domainthread.IsCanonicalRecordID(b.ThreadID) && domainthread.IsCanonicalRecordID(b.TurnID) &&
		domainidentity.ValidatePrincipalV1(b.Principal) == nil && domainsecurity.ValidateTurnSecurityContext(b.Original) == nil &&
		b.Original.ThreadID == b.ThreadID && b.Original.TurnID == b.TurnID && b.Original.TenantID == b.Principal.TenantID && b.Original.UserID == b.Principal.UserID
}

// BeginV1 is called only by the real host producer, never by HTTP authorization.
func (s *ServiceV1) BeginV1(binding port.BindingV1, secrets ...string) *ProducerV1 {
	if s == nil || !validBindingV1(binding) || len(secrets) > 64 {
		return nil
	}
	for _, secret := range secrets {
		if len(secret) > display.RawCaptureByteLimitV1 {
			return nil
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if s.closed || s.shutdownDrain {
		return nil
	}
	key := scopeKeyV1(binding.ThreadID, binding.TurnID)
	turn := s.turns[key]
	if turn != nil && turn.binding != binding {
		return nil
	}
	if turn == nil {
		if len(s.turns) >= s.limits.MaxTurns {
			return nil
		}
		id := randomIDV1()
		if id == "" {
			return nil
		}
		turn = &turnV1{binding: binding, incarnation: id, revision: 1, byKey: map[string]*rawBlockV1{}, preparationOrigins: map[string]uint64{}, wake: make(chan struct{}), activity: s.now(), handles: map[string]bool{}, currentAttempts: map[string]uint64{}, terminalAttempts: map[string]bool{}, byNamespace: map[string]*unitV2{}, byCall: map[string]*unitV2{}, sequenceUnits: map[string]*unitV2{}, producerIDs: map[string]string{}}
		s.turns[key] = turn
	}
	s.startJanitorLockedV1()
	if len(turn.handles) >= s.limits.MaxBlocks {
		return nil
	}
	handle := randomIDV1()
	if handle == "" {
		return nil
	}
	turn.handles[handle] = true
	turn.terminal = false
	turn.activity = s.now()
	return &ProducerV1{s, turn, handle, append([]string(nil), secrets...)}
}
func (p *ProducerV1) validLockedV1() bool {
	return p != nil && p.owner != nil && !p.owner.closed && p.owner.turns[scopeKeyV1(p.turn.binding.ThreadID, p.turn.binding.TurnID)] == p.turn && p.turn.handles[p.handle]
}
func (p *ProducerV1) FinishV1() {
	if p == nil {
		return
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() { p.secrets = nil }()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return
	}
	for _, block := range p.turn.blocks {
		if block.producerHandle != p.handle || (block.value.State != "running" && block.value.State != "preparing") {
			continue
		}
		s.retireLiveBlockLockedV1(p.turn, block, "unknown")
	}

	delete(p.turn.handles, p.handle)
	p.secrets = nil
	p.turn.terminal = len(p.turn.handles) == 0
	p.turn.activity = s.now()
	s.touchLockedV1(p.turn)
}
func frameKeyV1(f domainmodel.ProcessFrameV1) string {
	if f.Source == "provider_reasoning" {
		return "provider_reasoning/" + strconv.FormatUint(f.ProviderSequence, 10) + "/" + strconv.FormatUint(f.Attempt, 10) + "/" + strconv.FormatUint(f.HostSegmentOrdinal, 10)
	}
	return strconv.FormatUint(f.ProviderSequence, 10) + "/" + strconv.FormatUint(f.Attempt, 10) + "/" + strconv.Itoa(f.NativeIndex) + "/" + f.NativeID
}

// Registration is volatile, bounded and idempotent. Namespace equality uses
// the real host continuation value; opaque IDs are labels, never authority.
func namespaceKeyV2(n continuation.ProviderContinuationNamespaceV1) string {
	return string(n.UsageSource) + "/" + n.ChildRunID + "/" + strconv.FormatUint(n.ProviderCallSequence, 10)
}
func producerKeyV2(n continuation.ProviderContinuationNamespaceV1) string {
	return string(n.UsageSource) + "/" + n.ChildRunID
}
func validNamespaceV2(n continuation.ProviderContinuationNamespaceV1, scope []string) bool {
	depth := 0
	if n.DelegatedToolManifest != nil {
		depth = 1
	}
	return continuation.ValidateProviderContinuationNamespaceV1(n, depth, scope) == nil
}
func (p *ProducerV1) unitLockedV2(n continuation.ProviderContinuationNamespaceV1, scope []string) *unitV2 {
	if !p.validLockedV1() || !validNamespaceV2(n, scope) {
		return nil
	}
	t := p.turn
	key := namespaceKeyV2(n)
	if u := t.byNamespace[key]; u != nil {
		if !reflect.DeepEqual(u.namespace, n) {
			return nil
		}
		return u
	}
	if len(t.units) >= p.owner.limits.MaxBlocks || len(t.sequenceUnits) >= p.owner.limits.MaxBlocks {
		return nil
	}
	producerKey := producerKeyV2(n)
	producerID := t.producerIDs[producerKey]
	if producerID == "" {
		producerID = randomIDV1()
		if producerID == "" {
			return nil
		}
		t.producerIDs[producerKey] = producerID
	}
	id := randomIDV1()
	if id == "" {
		return nil
	}
	u := &unitV2{value: display.UnitV2{UnitID: id, ProducerID: producerID, ProviderSequence: strconv.FormatUint(n.ProviderCallSequence, 10), Placement: "pending", Calls: []display.CallAnchorV2{}, PrecedingCalls: []display.CallAnchorV2{}}, namespace: continuation.CloneProviderContinuationNamespaceV1(n), active: true}
	// The previous logical unit of this same producer supplies a real boundary.
	var previous *unitV2
	for _, candidate := range t.units {
		if candidate.value.ProducerID == producerID && candidate.namespace.ProviderCallSequence < n.ProviderCallSequence && (previous == nil || candidate.namespace.ProviderCallSequence > previous.namespace.ProviderCallSequence) {
			previous = candidate
		}
	}
	if previous != nil && len(previous.value.Calls) <= p.owner.limits.MaxBlocks-t.callRefs {
		u.value.PrecedingCalls = append(u.value.PrecedingCalls, previous.value.Calls...)
		t.callRefs += len(u.value.PrecedingCalls)
		if len(u.value.PrecedingCalls) > 0 {
			u.value.Placement = "anchored"
		}
	}
	t.units = append(t.units, u)
	t.byNamespace[key] = u
	p.owner.touchLockedV1(t)
	return u
}
func (p *ProducerV1) UnitV2(n continuation.ProviderContinuationNamespaceV1, active bool, scope ...string) bool {
	if p == nil {
		return false
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return false
	}
	key := p.handle + "/" + strconv.FormatUint(n.ProviderCallSequence, 10)
	// A native coordinate is immutable, including an already retained fallback.
	// Refuse before unit creation so conflicts cannot change rows or revision.
	if existing := p.turn.sequenceUnits[key]; existing != nil && !reflect.DeepEqual(existing.namespace, n) {
		return false
	}
	u := p.unitLockedV2(n, scope)
	if u != nil {
		if p.turn.sequenceUnits[key] == nil && len(p.turn.sequenceUnits) >= s.limits.MaxBlocks {
			return false
		}
		p.turn.sequenceUnits[key] = u
	}
	if u == nil {
		return false
	}
	if u.active && !active {
		u.active = false
		if len(u.value.Calls)+len(u.value.PrecedingCalls) == 0 {
			u.value.Placement = "unit_only"
		}
		s.touchLockedV1(p.turn)
	}
	return true
}
func (p *ProducerV1) ToolReadyV2(n continuation.ProviderContinuationNamespaceV1, callID, itemID string, scope ...string) bool {
	if p == nil || !domainmodel.IsHostToolCallIDV1(callID) || itemID != toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, callID) {
		return false
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if previous := p.turn.byCall[callID]; previous != nil && !reflect.DeepEqual(previous.namespace, n) {
		return false
	}
	u := p.unitLockedV2(n, scope)
	if u == nil {
		return false
	}
	if previous := p.turn.byCall[callID]; previous != nil {
		if previous != u {
			return false
		}
		for _, c := range u.value.Calls {
			if c.CallID == callID {
				return c.ToolCallItemID == itemID
			}
		}
		return false
	}
	if p.turn.callRefs >= s.limits.MaxBlocks {
		return false
	}
	u.value.Calls = append(u.value.Calls, display.CallAnchorV2{CallID: callID, ToolCallItemID: itemID})
	u.value.Placement = "anchored"
	for _, block := range p.turn.blocks {
		if block.value.Kind == "tool_preparation" && block.value.Source.CallID == callID && block.value.Source.UnitID == u.value.UnitID && block.value.State == "preparing" {
			s.retireLiveBlockLockedV1(p.turn, block, "unknown")
		}
	}
	p.turn.byCall[callID] = u
	p.turn.callRefs++
	s.touchLockedV1(p.turn)
	return true
}
func (p *ProducerV1) ToolMetadataV2(n continuation.ProviderContinuationNamespaceV1, callID, itemID, resultID string, startedSeq uint64) bool {
	if p == nil {
		return false
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return false
	}
	return s.toolMetadataLockedV2(p.turn, n, callID, itemID, resultID, startedSeq)
}

// ToolMetadataV2 enriches an existing host anchor without creating a producer,
// turn or unit. The caller verifies actual settlement authority; delivery still
// requires the independent current-scope lease. Exact replay does not revive
// producer activity or extend retention.
func (s *ServiceV1) ToolMetadataV2(binding port.BindingV1, n continuation.ProviderContinuationNamespaceV1, callID, itemID, resultID string, startedSeq uint64) bool {
	if s == nil || !validBindingV1(binding) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	t := s.turns[scopeKeyV1(binding.ThreadID, binding.TurnID)]
	if s.closed || t == nil || t.binding != binding {
		return false
	}
	return s.toolMetadataLockedV2(t, n, callID, itemID, resultID, startedSeq)
}

func (s *ServiceV1) toolMetadataLockedV2(t *turnV1, n continuation.ProviderContinuationNamespaceV1, callID, itemID, resultID string, startedSeq uint64) bool {
	u := t.byCall[callID]
	if u == nil || !reflect.DeepEqual(u.namespace, n) || !domainmodel.IsHostToolCallIDV1(callID) || itemID != toolcall.ToolCallItemIDV1(t.binding.TurnID, callID) || resultID != "" && resultID != toolresult.ToolResultItemIDV1(t.binding.TurnID, callID) {
		return false
	}
	var origin *display.CallAnchorV2
	for i := range u.value.Calls {
		if u.value.Calls[i].CallID == callID {
			if origin != nil {
				return false
			}
			origin = &u.value.Calls[i]
		}
	}
	if origin == nil || origin.ToolCallItemID != itemID {
		return false
	}
	metadata := *origin
	if resultID != "" {
		if metadata.ToolResultItemID != "" && metadata.ToolResultItemID != resultID {
			return false
		}
		metadata.ToolResultItemID = resultID
	}
	if startedSeq > 0 {
		seq := strconv.FormatUint(startedSeq, 10)
		if metadata.StartedEventSeq != "" && metadata.StartedEventSeq != seq {
			return false
		}
		metadata.StartedEventSeq = seq
	}
	// Preflight every retained copy before changing any field. A late callback
	// may enrich a copied preceding boundary, never replace or reorder it.
	anchors := []*display.CallAnchorV2{origin}
	for _, candidate := range t.units {
		for i := range candidate.value.PrecedingCalls {
			c := &candidate.value.PrecedingCalls[i]
			if c.CallID != callID {
				continue
			}
			if candidate.value.ProducerID != u.value.ProducerID || c.ToolCallItemID != itemID || c.ToolResultItemID != "" && c.ToolResultItemID != metadata.ToolResultItemID || c.StartedEventSeq != "" && c.StartedEventSeq != metadata.StartedEventSeq {
				return false
			}
			anchors = append(anchors, c)
		}
	}
	changed := false
	for _, c := range anchors {
		if c.ToolResultItemID == "" && metadata.ToolResultItemID != "" {
			c.ToolResultItemID = metadata.ToolResultItemID
			changed = true
		}
		if c.StartedEventSeq == "" && metadata.StartedEventSeq != "" {
			c.StartedEventSeq = metadata.StartedEventSeq
			changed = true
		}
	}
	if resultID != "" {
		retained := t.blocks[:0]
		for _, block := range t.blocks {
			if block.value.Kind == "tool_input" && block.value.Source.CallID == callID {
				s.retireLiveBlockLockedV1(t, block, "unknown")
				block.value.Source.ToolResultItemID = resultID
				changed = true
				// A settled input is no longer a process observation. Preserve its
				// cleared keyed slot only for an actual typed producer to replace;
				// the unit's real result anchor does not depend on this block.
				continue
			}
			if block.value.Kind == "shell" && block.value.Source.CallID == callID && (block.value.State == "running" || block.value.State == "unknown" && block.value.Source.ToolResultItemID == "") {
				if block.value.State == "running" {
					s.retireLiveBlockLockedV1(t, block, "unknown")
				}
				// A retired command may acquire its real result coordinate, never
				// regain command or output content from metadata alone.
				block.value.Source.ToolResultItemID = resultID
				changed = true
			}
			if block.value.Kind == "web_fetch" && block.value.Source.CallID == callID && block.value.State == "running" {
				s.retireWebInputLockedV1(t, block)
				block.value.Source.ToolResultItemID = resultID
				changed = true
			}
			retained = append(retained, block)
		}
		clear(t.blocks[len(retained):])
		t.blocks = retained
	}
	if changed {
		s.touchLockedV1(t)
	}
	return true
}
func (p *ProducerV1) nativeUnitLockedV2(sequence uint64) *unitV2 {
	t := p.turn
	key := p.handle + "/" + strconv.FormatUint(sequence, 10)
	if u := t.sequenceUnits[key]; u != nil {
		return u
	}
	// Legacy/restored producers without a proven namespace remain unit-only.
	if len(t.units) >= p.owner.limits.MaxBlocks || len(t.sequenceUnits) >= p.owner.limits.MaxBlocks {
		return nil
	}
	id := randomIDV1()
	if id == "" {
		return nil
	}
	u := &unitV2{value: display.UnitV2{UnitID: id, ProducerID: p.handle, ProviderSequence: strconv.FormatUint(sequence, 10), Placement: "unit_only", Calls: []display.CallAnchorV2{}, PrecedingCalls: []display.CallAnchorV2{}}}
	t.units = append(t.units, u)
	t.sequenceUnits[key] = u
	return u
}
func (p *ProducerV1) FrameV1(frame domainmodel.ProcessFrameV1) {
	if p == nil {
		return
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return
	}
	t := p.turn
	// Source authority loss is permanent for this logical unit. Registration
	// replay or a late reset must not reopen any native, draft or argument frame.
	if unit := t.sequenceUnits[p.handle+"/"+strconv.FormatUint(frame.ProviderSequence, 10)]; unit != nil && (unit.retired || unit.namespace.Version != 0 && !unit.active) {
		return
	}
	if frame.Source != "" && frame.Source != "assistant_draft" && frame.Source != "provider_reasoning" && frame.Source != "tool_preparation" {
		return
	}
	if frame.ReasoningForm != "" && (frame.ReasoningForm != "summary" || frame.Source != "" && frame.Source != "provider_reasoning") {
		return
	}
	if frame.Attempt == 0 || frame.Attempt > 1<<53-1 || frame.NativeIndex < 0 || uint64(frame.NativeIndex) > 1<<53-1 || len(frame.NativeID) > 256 {
		return
	}
	attemptKey := p.handle + "/" + strconv.FormatUint(frame.ProviderSequence, 10)
	currentAttempt := t.currentAttempts[attemptKey]
	if currentAttempt == 0 {
		if len(t.currentAttempts) >= s.limits.MaxBlocks {
			return
		}
		t.currentAttempts[attemptKey] = frame.Attempt
	} else if frame.Attempt < currentAttempt {
		return
	}
	if frame.Kind == "reset" {
		if frame.Attempt == currentAttempt {
			return
		}
		t.currentAttempts[attemptKey] = frame.Attempt
		t.terminalAttempts[attemptKey] = false
	} else if frame.Attempt != t.currentAttempts[attemptKey] || t.terminalAttempts[attemptKey] {
		return
	}
	if frame.Kind == "terminal" {
		t.terminalAttempts[attemptKey] = true
	}
	if frame.Kind == "reset" || frame.Kind == "terminal" {
		for _, block := range t.blocks {
			if (block.value.Kind != "native_reasoning" && block.value.Kind != "assistant_draft" && block.value.Kind != "tool_preparation") || block.providerSequence != frame.ProviderSequence || block.producerHandle != p.handle {
				continue
			}
			if frame.Kind == "reset" && block.value.Attempt < frame.Attempt || frame.Kind == "terminal" && (block.value.State == "running" || block.value.State == "preparing" && !block.preparationSealed) && block.value.Attempt == frame.Attempt {
				state := "unknown"
				if frame.Kind == "reset" {
					state = "superseded"
				}
				s.retireLiveBlockLockedV1(t, block, state)
			}
		}
		t.activity = s.now()
		s.touchLockedV1(t)
		return
	}
	if frame.Source == "tool_preparation" {
		p.toolPreparationLockedV1(frame)
		return
	}
	if frame.Source == "provider_reasoning" && (frame.HostSegmentOrdinal == 0 || frame.HostSegmentOrdinal > uint64(s.limits.MaxBlocks)) {
		return
	}
	if frame.Source == "assistant_draft" {
		p.assistantDraftLockedV1(frame)
		return
	}
	key := p.handle + "/" + frameKeyV1(frame)
	block := t.byKey[key]
	if block != nil && block.value.Source.ReasoningForm != frame.ReasoningForm {
		return
	}
	switch frame.Kind {
	case "start":
		if block != nil || len(t.blocks) >= s.limits.MaxBlocks {
			return
		}
		index := frame.NativeIndex
		unit := t.sequenceUnits[p.handle+"/"+strconv.FormatUint(frame.ProviderSequence, 10)]
		if frame.Source == "provider_reasoning" {
			if unit == nil || !unit.active || unit.namespace.ProviderCallSequence != frame.ProviderSequence {
				return
			}
		} else {
			if unit != nil && unit.namespace.ProviderCallSequence != 0 && !unit.active {
				return
			}
			unit = p.nativeUnitLockedV2(frame.ProviderSequence)
		}
		if unit == nil || t.sourceOrdinal == ^uint64(0) {
			return
		}
		for _, previous := range t.blocks {
			if frame.Source == "" && previous.value.Source.Kind == "native" && previous.value.Kind == "native_reasoning" && previous.value.Source.UnitID == unit.value.UnitID && previous.value.Attempt == frame.Attempt && previous.value.NativeIndex != nil && *previous.value.NativeIndex == index {
				return
			}
		}
		t.sourceOrdinal++
		source := display.SourceV2{Kind: "native", UnitID: unit.value.UnitID, ProducerID: unit.value.ProducerID, ProviderSequence: unit.value.ProviderSequence, Attempt: frame.Attempt, NativeIndex: index, SourceOrdinal: strconv.FormatUint(t.sourceOrdinal, 10), ReasoningForm: frame.ReasoningForm}
		var nativeIndex *int = &index
		if frame.Source == "provider_reasoning" {
			source.Kind = "provider_reasoning"
			source.NativeIndex = 0 // Not part of this source's wire identity.
			nativeIndex = nil
		}
		block = &rawBlockV1{value: display.BlockV1{Source: source, BlockID: randomIDV1(), Kind: "native_reasoning", State: "running", Attempt: frame.Attempt, NativeIndex: nativeIndex, Classification: "withheld"}, started: s.now(), producerHandle: p.handle, providerSequence: frame.ProviderSequence, key: key}
		if block.value.BlockID == "" {
			return
		}
		t.blocks = append(t.blocks, block)
		t.byKey[key] = block
	case "delta":
		if block == nil || block.value.State != "running" {
			return
		}
	case "end":
		if block == nil || block.value.State != "running" {
			return
		}
	default:
		return
	}
	if frame.Text != "" && !block.overflow {
		if len(frame.Text) > s.limits.RawBytes-t.rawBytes {
			t.rawBytes -= len(block.raw)
			block.raw = ""
			block.overflow = true
		} else {
			block.raw += frame.Text
			t.rawBytes += len(frame.Text)
		}
	}
	if frame.Kind != "end" {
		unit := t.sequenceUnits[p.handle+"/"+strconv.FormatUint(frame.ProviderSequence, 10)]
		if unit != nil && unit.active && unit.namespace.ProviderCallSequence == frame.ProviderSequence {
			s.projectLiveReasoningLockedV1(t, block, append(append([]string(nil), p.secrets...), frame.ProjectionSecrets...))
		}
	}
	if frame.Kind == "end" {
		t.displayBytes -= blockBytesV1(block.value)
		clearLiveContentV1(&block.value)
		raw := display.RawCaptureV1{Kind: "native_reasoning", Status: "completed", Complete: true, Overflow: block.overflow, Body: block.raw, Title: reasoningTitleV1(block.value.Source), DurationMs: s.now().Sub(block.started).Milliseconds()}
		s.completeLockedV1(t, block, raw, append(append([]string(nil), p.secrets...), frame.ProjectionSecrets...))
		t.rawBytes -= len(block.raw)
		block.raw = ""
	}
	t.activity = s.now()
	s.touchLockedV1(t)
}

// The assistant draft source is the actual host logical unit and physical
// attempt. It cannot fall back to a native index or create a tool identity.
func (p *ProducerV1) assistantDraftLockedV1(frame domainmodel.ProcessFrameV1) {
	if frame.Kind != "delta" || frame.Text == "" || (frame.ContentClassification != "provider_text" && frame.ContentClassification != "unclassified_inline") {
		return
	}
	s, t := p.owner, p.turn
	unit := t.sequenceUnits[p.handle+"/"+strconv.FormatUint(frame.ProviderSequence, 10)]
	if unit == nil || !unit.active || unit.namespace.ProviderCallSequence != frame.ProviderSequence {
		return
	}
	key := p.handle + "/draft/" + strconv.FormatUint(frame.ProviderSequence, 10) + "/" + strconv.FormatUint(frame.Attempt, 10)
	block := t.byKey[key]
	if block == nil {
		if len(t.blocks) >= s.limits.MaxBlocks {
			return
		}
		source := display.SourceV2{Kind: "assistant_draft", UnitID: unit.value.UnitID, ProducerID: unit.value.ProducerID, ProviderSequence: unit.value.ProviderSequence, Attempt: frame.Attempt}
		block = &rawBlockV1{value: display.BlockV1{Source: source, BlockID: randomIDV1(), Kind: "assistant_draft", State: "running", Attempt: frame.Attempt, Classification: "withheld", ContentClassification: frame.ContentClassification}, started: s.now(), producerHandle: p.handle, providerSequence: frame.ProviderSequence, key: key}
		if block.value.BlockID == "" {
			return
		}
		t.blocks = append(t.blocks, block)
		t.byKey[key] = block
	}
	if block.value.State != "running" || block.overflow {
		return
	}
	if block.value.ContentClassification != frame.ContentClassification {
		block.overflow = true
	}
	t.displayBytes -= blockBytesV1(block.value)
	clearAssistantDraftV1(&block.value)
	if block.overflow || len(frame.Text) > s.limits.RawBytes-t.rawBytes {
		t.rawBytes -= len(block.raw)
		block.raw, block.overflow = "", true
	} else {
		block.raw += frame.Text
		t.rawBytes += len(frame.Text)
		capture := display.ProjectAssistantDraftV1(block.raw, append(append([]string(nil), p.secrets...), frame.ProjectionSecrets...)...)
		if len(capture.Body) <= s.limits.DisplayBytes-t.displayBytes && capture.Classification != "withheld" {
			block.value.DraftSegments = display.SegmentAssistantDraftV1(capture.Body)
			block.value.DraftBytes = strconv.Itoa(len(capture.Body))
			block.value.Classification = capture.Classification
			t.displayBytes += blockBytesV1(block.value)
		}
	}
	t.activity = s.now()
	s.touchLockedV1(t)
}

func (s *ServiceV1) retireAssistantDraftUnitLockedV1(t *turnV1, unitID string) {
	for _, unit := range t.units {
		if unit.value.UnitID == unitID {
			unit.active = false
		}
	}
	for _, block := range t.blocks {
		if block.value.Source.Kind != "assistant_draft" || block.value.Source.UnitID != unitID {
			continue
		}
		t.rawBytes -= len(block.raw)
		block.raw = ""
		t.displayBytes -= blockBytesV1(block.value)
		clearAssistantDraftV1(&block.value)
		if block.value.State == "running" {
			block.value.State = "unknown"
		}
	}
	s.touchLockedV1(t)
}

// Losing source/current authority retires the running source; cancelling one
// viewer request or explicitly revoking one pane does not retire sibling panes.
func (s *ServiceV1) retireRunningAssistantDraftsV1(ctx context.Context, t *turnV1) {
	if ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.closed || s.turns[scopeKeyV1(t.binding.ThreadID, t.binding.TurnID)] != t {
		return
	}
	units := map[string]bool{}
	for _, unit := range t.units {
		if unit.active {
			units[unit.value.UnitID] = true
		}
	}
	for _, block := range t.blocks {
		if (block.value.Kind == "native_reasoning" || block.value.Kind == "assistant_draft") && block.value.State == "running" || block.value.Kind == "tool_preparation" && block.value.State == "preparing" {
			units[block.value.Source.UnitID] = true
		}
	}
	for unitID := range units {
		s.retireProviderUnitLockedV1(t, unitID)
	}
	for _, block := range t.blocks {
		if block.value.Source.Kind == "tool" && block.value.State == "running" {
			if block.value.Kind == "web_fetch" {
				s.retireWebInputLockedV1(t, block)
			} else {
				s.retireLiveBlockLockedV1(t, block, "unknown")
			}
			s.touchLockedV1(t)
		}
	}
}

func clearAssistantDraftV1(block *display.BlockV1) {
	block.DraftSegments = nil
	block.DraftBytes = ""
	block.Classification = "withheld"
}

// ToolInputV2 shares the actual call's existing Bash/Web slot where present.
// No decoded execution map or public argument projection supplies these bytes.
func (p *ProducerV1) ToolInputV2(callID string, arguments json.RawMessage, tool display.ToolBindingV1) bool {
	if p == nil || !domainmodel.IsHostToolCallIDV1(callID) || tool.CallID != callID || strings.TrimSpace(tool.ToolName) == "" || tool.ResultItemID != "" || !domainsecurity.IsSHA256Hex(tool.GrantID) || !domainsecurity.IsSHA256Hex(tool.InputArgsHash) || domainsecurity.CanonicalJSONHash(arguments) != tool.InputArgsHash {
		return false
	}
	input, classification := display.ProjectToolInputV2(arguments, p.secrets...)
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return false
	}
	t := p.turn
	unit := t.byCall[callID]
	if unit == nil {
		return false
	}
	var anchor *display.CallAnchorV2
	for i := range unit.value.Calls {
		if unit.value.Calls[i].CallID == callID {
			anchor = &unit.value.Calls[i]
			break
		}
	}
	if anchor == nil || anchor.ToolResultItemID != "" || anchor.StartedEventSeq == "" {
		return false
	}
	key, kind := "tool/"+callID, "tool_input"
	if tool.ToolName == "bash" {
		kind = "shell"
	}
	if tool.ToolName == "web_fetch" {
		key, kind = "web_input/"+callID, "web_fetch"
	}
	if existing := t.byKey[key]; existing != nil {
		return existing.value.State == "running" && existing.value.ToolBinding == tool && reflect.DeepEqual(existing.value.Input, &input)
	}
	if len(t.blocks) >= s.limits.MaxBlocks {
		return false
	}
	inputBytes := 0
	if input.Availability == "available" {
		if len(arguments) > s.limits.RawBytes-t.rawBytes || len(input.InputJSON) > s.limits.DisplayBytes-t.displayBytes {
			input = display.InputV2{Availability: "unavailable", Reason: "limit"}
			classification = "withheld"
		} else {
			inputBytes = len(arguments)
		}
	}
	block := &rawBlockV1{value: display.BlockV1{BlockID: randomIDV1(), Kind: kind, State: "running", Source: display.SourceV2{Kind: "tool", UnitID: unit.value.UnitID, CallID: callID, ToolCallItemID: anchor.ToolCallItemID, StartedEventSeq: anchor.StartedEventSeq}, ToolBinding: tool, Input: &input, Classification: classification}, inputBytes: inputBytes, key: key, started: s.now()}
	if block.value.BlockID == "" {
		return false
	}
	t.blocks = append(t.blocks, block)
	t.byKey[key] = block
	t.rawBytes += inputBytes
	t.displayBytes += blockBytesV1(block.value)
	t.activity = s.now()
	s.touchLockedV1(t)
	return true
}

// WebInputV1 publishes only a complete parsed argument from the actual active
// execution. Caller validation binds the private hash to its signed grant;
// every read and target resolution validates that active authority again.
func (p *ProducerV1) WebInputV1(callID, requestURL string, tool display.ToolBindingV1) bool {
	if p == nil || !display.ValidWebRequestURLV1(requestURL) || !domainmodel.IsHostToolCallIDV1(callID) || tool.CallID != callID || tool.ToolName != "web_fetch" || tool.ResultItemID != "" || !domainsecurity.IsSHA256Hex(tool.GrantID) || !domainsecurity.IsSHA256Hex(tool.InputArgsHash) {
		return false
	}
	// Apply the existing credential floor. A changed/masked URL is withheld as a
	// whole; it cannot become a clickable target derived from display text.
	safe := display.ProjectCaptureV1(display.RawCaptureV1{Kind: "web_fetch", Complete: true, Body: requestURL}, p.secrets...)
	if safe.Classification != "source_exact_local" || safe.Truncated || safe.Body != requestURL {
		return false
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return false
	}
	t := p.turn
	unit := t.byCall[callID]
	if unit == nil {
		return false
	}
	var anchor *display.CallAnchorV2
	for i := range unit.value.Calls {
		if unit.value.Calls[i].CallID == callID {
			anchor = &unit.value.Calls[i]
			break
		}
	}
	if anchor == nil || anchor.ToolResultItemID != "" || anchor.StartedEventSeq == "" {
		return false
	}
	key := "web_input/" + callID
	if existing := t.byKey[key]; existing != nil {
		if existing.value.Input != nil && existing.value.Classification == "credential_redacted" {
			return false
		}
		if existing.value.State == "running" && existing.value.ToolBinding == tool && existing.webRequestURL == "" && existing.value.Input != nil && len(requestURL) <= s.limits.RawBytes-t.rawBytes && len(requestURL) <= s.limits.DisplayBytes-t.displayBytes {
			existing.webRequestURL, existing.value.RequestURL = requestURL, requestURL
			existing.value.InputDigest = display.WebInputDigestV1(tool, requestURL)
			t.rawBytes += len(requestURL)
			t.displayBytes += len(requestURL)
			if existing.value.Classification == "withheld" {
				existing.value.Classification = "source_exact_local"
			}
			s.touchLockedV1(t)
			return true
		}
		return existing.value.State == "running" && existing.value.ToolBinding == tool && existing.webRequestURL == requestURL
	}
	if len(t.blocks) >= s.limits.MaxBlocks || len(requestURL) > s.limits.RawBytes-t.rawBytes || len(requestURL) > s.limits.DisplayBytes-t.displayBytes {
		return false
	}
	block := &rawBlockV1{value: display.BlockV1{BlockID: randomIDV1(), Source: display.SourceV2{Kind: "tool", UnitID: unit.value.UnitID, CallID: callID, ToolCallItemID: anchor.ToolCallItemID, StartedEventSeq: anchor.StartedEventSeq}, Kind: "web_fetch", State: "running", ToolBinding: tool, RequestURL: requestURL, InputDigest: display.WebInputDigestV1(tool, requestURL), Classification: "source_exact_local"}, webRequestURL: requestURL, key: key, started: s.now()}
	if block.value.BlockID == "" {
		return false
	}
	t.blocks = append(t.blocks, block)
	t.byKey[key] = block
	t.rawBytes += len(requestURL)
	t.displayBytes += blockBytesV1(block.value)
	t.activity = s.now()
	s.touchLockedV1(t)
	return true
}
func (s *ServiceV1) retireWebInputLockedV1(t *turnV1, block *rawBlockV1) {
	t.rawBytes -= block.inputBytes
	block.inputBytes = 0
	t.rawBytes -= len(block.webRequestURL)
	t.displayBytes -= blockBytesV1(block.value)
	block.webRequestURL = ""
	block.value.RequestURL = ""
	block.value.InputDigest = ""
	block.value.Input = nil
	block.value.ToolBinding = display.ToolBindingV1{}
	block.value.Classification = "withheld"
	block.value.State = "unknown"
}

func (p *ProducerV1) ToolV1(callID string, raw display.RawCaptureV1) {
	if p == nil || !domainmodel.IsHostToolCallIDV1(callID) || (raw.Kind != "shell" && raw.Kind != "write" && raw.Kind != "edit") {
		return
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return
	}
	t := p.turn
	unit := t.byCall[callID]
	if unit == nil {
		return
	}
	var anchor *display.CallAnchorV2
	for i := range unit.value.Calls {
		if unit.value.Calls[i].CallID == callID {
			anchor = &unit.value.Calls[i]
			break
		}
	}
	if anchor == nil || anchor.ToolResultItemID == "" || anchor.ToolResultItemID != raw.ToolBinding.ResultItemID {
		return
	}
	source := display.SourceV2{Kind: "tool", UnitID: unit.value.UnitID, CallID: callID, ToolCallItemID: anchor.ToolCallItemID, ToolResultItemID: anchor.ToolResultItemID, StartedEventSeq: anchor.StartedEventSeq}
	key := "tool/" + callID
	block := t.byKey[key]
	if block != nil {
		if (block.value.Kind != "tool_input" && (raw.Kind != "shell" || block.value.Kind != "shell")) || (block.value.State != "running" && block.value.State != "unknown") || block.value.ToolBinding.InputArgsHash == "" || block.value.Source.UnitID != source.UnitID || block.value.Source.CallID != source.CallID || block.value.Source.ToolCallItemID != source.ToolCallItemID || block.value.Source.StartedEventSeq != source.StartedEventSeq || block.value.ToolBinding.ToolName != raw.ToolBinding.ToolName || block.value.ToolBinding.GrantID != raw.ToolBinding.GrantID {
			return
		}
		retained := false
		for _, candidate := range t.blocks {
			if candidate == block {
				retained = true
				break
			}
		}
		if !retained {
			if len(t.blocks) >= s.limits.MaxBlocks {
				return
			}
			t.blocks = append(t.blocks, block)
		}
		t.displayBytes -= blockBytesV1(block.value)
		clearLiveContentV1(&block.value)
		block.value.Source = source
		block.value.Kind = raw.Kind
		block.producerHandle = ""
	} else {
		if len(t.blocks) >= s.limits.MaxBlocks {
			return
		}
		block = &rawBlockV1{value: display.BlockV1{Source: source, BlockID: randomIDV1(), Kind: raw.Kind, Classification: "withheld"}, key: key, started: s.now()}
		if block.value.BlockID == "" {
			return
		}
		t.blocks = append(t.blocks, block)
		t.byKey[key] = block
	}
	s.completeLockedV1(t, block, raw, p.secrets)
	t.activity = s.now()
	s.touchLockedV1(t)
}
func blockBytesV1(b display.BlockV1) int {
	n := len(b.ToolName) + len(b.Title) + len(b.Preview) + len(b.Path) + len(b.RequestURL)
	if b.Input != nil {
		n += len(b.Input.InputJSON)
	}
	for _, segment := range b.DraftSegments {
		n += len(segment.Text)
	}
	if b.Body != nil {
		n += len(b.Body.Text) + len(b.Body.Before) + len(b.Body.After) + len(b.Body.Error)
	}
	return n
}
func (s *ServiceV1) completeLockedV1(t *turnV1, block *rawBlockV1, raw display.RawCaptureV1, secrets []string) {
	capture := display.ProjectCaptureV1(raw, secrets...)
	b := &block.value
	b.State = raw.Status
	switch b.State {
	case "completed", "failed", "timeout", "canceled", "unknown":
	default:
		b.State = "unknown"
	}
	b.DurationMs = raw.DurationMs
	if b.DurationMs < 0 {
		b.DurationMs = 0
	}
	b.Classification = capture.Classification
	b.Truncated = capture.Truncated
	if b.Kind == "write" || b.Kind == "edit" {
		if raw.EffectApplied != nil {
			value := *raw.EffectApplied
			b.EffectApplied = &value
		}
		switch raw.SettlementStatus {
		case "completed", "failed", "quarantined", "not_required", "no_effect", "aborted", "unknown":
			b.SettlementStatus = raw.SettlementStatus
		default:
			b.SettlementStatus = "unknown"
		}
	}
	if b.Kind == "shell" {
		if raw.ExitCode != nil {
			value := *raw.ExitCode
			b.ExitCode = &value
		}
	}
	if b.Kind != "native_reasoning" {
		b.ToolBinding = raw.ToolBinding
	}
	if capture.Classification == "withheld" || b.State == "unknown" || b.Kind == "native_reasoning" && b.State != "completed" {
		b.Classification = "withheld"
		return
	}
	// This number is private source-bearing metadata, available only after the
	// same complete projection/state checks as the original capture.
	if (b.Kind == "write" || b.Kind == "edit") && raw.InputUtf16CodeUnits != nil && *raw.InputUtf16CodeUnits >= 0 && *raw.InputUtf16CodeUnits <= display.MaxInputUtf16CodeUnitsV1 {
		value := *raw.InputUtf16CodeUnits
		b.InputUtf16CodeUnits = &value
	}
	b.Title = capture.Title
	b.Preview = capture.Preview
	if b.Kind != "shell" {
		b.Path = capture.Path
	}
	b.Body = &display.BodyV1{Text: capture.Body, Before: capture.Before, After: capture.After, Error: capture.Error, AddedLines: raw.AddedLines, RemovedLines: raw.RemovedLines, Approximate: raw.Approximate}
	if size := blockBytesV1(*b); size > s.limits.DisplayBytes-t.displayBytes {
		b.Title = ""
		b.Preview = ""
		b.Path = ""
		b.Body = nil
		b.InputUtf16CodeUnits = nil
		b.Classification = "withheld"
		return
	} else {
		t.displayBytes += size
	}
	b.CaptureDigest = display.CaptureDigestV1(*b)
}
func (s *ServiceV1) touchLockedV1(t *turnV1) {
	t.revision++
	close(t.wake)
	t.wake = make(chan struct{})
}
func (s *ServiceV1) retireViewerLockedV1(v *viewerV1) {
	if v.revoked {
		return
	}
	v.revoked = true
	close(v.wake)
	if s.viewers[v.token] == v {
		delete(s.viewers, v.token)
	}
	s.tombstoneLockedV1(v.token)
	if v.turn != nil && v.turn.historyAnchor != nil {
		for _, b := range v.turn.blocks {
			b.value = display.BlockV1{}
		}
		v.turn.blocks = nil
		v.turn.units = nil
		v.turn.historyAnchor = nil
	}
}
func (s *ServiceV1) tombstoneLockedV1(token string) {
	if len(s.tombstones) >= s.limits.MaxTombstones {
		var oldest string
		var at time.Time
		for token, t := range s.tombstones {
			if oldest == "" || t.Before(at) {
				oldest = token
				at = t
			}
		}
		delete(s.tombstones, oldest)
	}
	s.tombstones[token] = s.now()
}
func (s *ServiceV1) sweepLockedV1() {
	now := s.now()
	for _, v := range s.viewers {
		if v.turn != nil && v.turn.historyAnchor != nil && now.Sub(v.turn.activity) >= s.limits.TerminalTTL {
			s.retireViewerLockedV1(v)
		}
	}
	for key, t := range s.turns {
		ttl := s.limits.IdleTTL
		if t.terminal {
			ttl = s.limits.TerminalTTL
		}
		if now.Sub(t.activity) < ttl {
			continue
		}
		s.dropTurnLockedV1(key, t)
	}
}
func (s *ServiceV1) dropTurnLockedV1(key string, t *turnV1) {
	for _, v := range s.viewers {
		if v.turn == t {
			s.retireViewerLockedV1(v)
		}
	}
	for _, b := range t.blocks {
		b.raw = ""
		b.webRequestURL = ""
		b.value = display.BlockV1{}
	}
	t.blocks = nil
	t.byKey = nil
	t.preparationOrigins = nil
	t.handles = nil
	t.units = nil
	t.byNamespace = nil
	t.byCall = nil
	t.sequenceUnits = nil
	t.producerIDs = nil
	t.callRefs = 0
	t.currentAttempts = nil
	t.terminalAttempts = nil
	t.rawBytes = 0
	t.displayBytes = 0
	close(t.wake)
	t.wake = make(chan struct{})
	delete(s.turns, key)
}

var viewerTokenV1 = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func (s *ServiceV1) AuthorizeV1(ctx context.Context, token, document, thread, turn string) ResponseV1 {
	if s == nil || ctx == nil || !viewerTokenV1.MatchString(token) || !viewerTokenV1.MatchString(document) || s.authority == nil {
		return ResponseV1{Status: "unavailable"}
	}
	s.mu.Lock()
	s.sweepLockedV1()
	if s.closed || s.shutdownDrain || ctx.Err() != nil || s.viewers[token] != nil || !s.tombstones[token].IsZero() || len(s.viewers) >= s.limits.MaxViewers {
		s.mu.Unlock()
		return ResponseV1{Status: "unavailable"}
	}
	t := s.turns[scopeKeyV1(thread, turn)]
	// Pending historical reads consume the same bounded viewer reservation as
	// live authorization, before any CAS I/O or record allocation.
	reservation := &viewerV1{token: token, document: document, thread: thread, turn: t, wake: make(chan struct{})}
	s.viewers[token] = reservation
	s.mu.Unlock()
	accepted := false
	defer func() {
		if !accepted {
			s.mu.Lock()
			s.retireViewerLockedV1(reservation)
			s.mu.Unlock()
		}
	}()
	if t == nil {
		reader, ok := s.authority.(port.SettledHistoryReadAuthorityV1)
		if !ok {
			return ResponseV1{Status: "unavailable"}
		}
		record, anchor, err := reader.ReadSettledHistoryV1(ctx, thread, turn)
		if err != nil {
			return ResponseV1{Status: "unavailable"}
		}
		s.mu.Lock()
		at := s.now()
		if s.closed || ctx.Err() != nil || reservation.revoked || s.viewers[token] != reservation {
			s.mu.Unlock()
			return ResponseV1{Status: "unavailable"}
		}
		t = s.turns[scopeKeyV1(thread, turn)]
		if t == nil {
			t = historyTurnV1(record, anchor, at)
		}
		reservation.turn = t
		s.mu.Unlock()
		if t == nil {
			return ResponseV1{Status: "unavailable"}
		}
	}
	s.mu.Lock()
	var historyAnchor *display.SettledHistoryAnchorV1
	if t.historyAnchor != nil {
		copy := *t.historyAnchor
		historyAnchor = &copy
	}
	s.mu.Unlock()
	a, release, err := s.authority.AcquireCurrent(ctx, t.binding)
	if err != nil || release == nil {
		return ResponseV1{Status: "unavailable"}
	}
	defer release()
	if a.Binding != t.binding || s.authority.ValidateCurrent(ctx, a) != nil {
		return ResponseV1{Status: "unavailable"}
	}
	if historyAnchor != nil {
		reader, ok := s.authority.(port.SettledHistoryReadAuthorityV1)
		if !ok || reader.ValidateSettledHistoryCurrentV1(ctx, t.binding, *historyAnchor) != nil {
			return ResponseV1{Status: "unavailable"}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if s.closed || ctx.Err() != nil || reservation.revoked || s.viewers[token] != reservation || !s.validViewerTurnLockedV1(t) {
		return ResponseV1{Status: "unavailable"}
	}
	accepted = true
	reservation.active = true
	s.startJanitorLockedV1()
	return s.responseLockedV1(t, "authorized", nil)
}
func (s *ServiceV1) RevokeV1(token string) ResponseV1 {
	if s == nil || !viewerTokenV1.MatchString(token) {
		return ResponseV1{Status: "unavailable"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.viewers[token]; v != nil {
		s.retireViewerLockedV1(v)
	} else {
		s.tombstoneLockedV1(token)
	}
	return ResponseV1{Status: "revoked"}
}
func (s *ServiceV1) responseLockedV1(t *turnV1, status string, expanded map[string]bool) ResponseV1 {
	b := t.binding
	scope := display.ScopeV1{ThreadID: b.ThreadID, TurnID: b.TurnID, ContextDigest: b.Original.ContextDigest, ContextEpoch: strconv.FormatUint(b.Original.ContextEpoch, 10), ProcessIncarnation: t.incarnation}
	response := ResponseV1{SchemaVersion: 2, Status: status, Revision: strconv.FormatUint(t.revision, 10), Scope: &scope}
	if status == "authorized" {
		presence := display.PresenceV2{}
		for _, block := range t.blocks {
			if block.value.Kind == "assistant_draft" {
				presence.AssistantDraftCount++
			} else if block.value.Kind == "native_reasoning" {
				presence.NativeBlockCount++
			} else {
				presence.ToolCaptureCount++
			}
		}
		response.Presence = &presence
	}
	if status != "snapshot" {
		return response
	}
	response.Units = make([]display.UnitV2, 0, len(t.units))
	for _, unit := range t.units {
		value := unit.value
		value.Calls = append([]display.CallAnchorV2{}, value.Calls...)
		value.PrecedingCalls = append([]display.CallAnchorV2{}, value.PrecedingCalls...)
		response.Units = append(response.Units, value)
	}
	response.Blocks = make([]display.BlockV1, 0, len(t.blocks))
	for _, raw := range t.blocks {
		block := raw.value
		if block.NativeIndex != nil {
			value := *block.NativeIndex
			block.NativeIndex = &value
		}
		if block.ExitCode != nil {
			value := *block.ExitCode
			block.ExitCode = &value
		}
		if block.InputUtf16CodeUnits != nil {
			value := *block.InputUtf16CodeUnits
			block.InputUtf16CodeUnits = &value
		}
		if block.EffectApplied != nil {
			value := *block.EffectApplied
			block.EffectApplied = &value
		}
		if block.State == "running" {
			block.DurationMs = s.now().Sub(raw.started).Milliseconds()
		}
		if !expanded[block.BlockID] {
			block.Input = nil
			block.DraftSegments = nil
			block.DraftBytes = ""
			block.Body = nil
		} else if block.Body != nil {
			body := *block.Body
			block.Body = &body
		}
		if expanded[block.BlockID] && block.Input != nil {
			input := *block.Input
			block.Input = &input
		}
		if expanded[block.BlockID] && block.DraftSegments != nil {
			block.DraftSegments = append([]display.DraftSegmentV1(nil), block.DraftSegments...)
		}
		response.Blocks = append(response.Blocks, block)
	}
	return response
}
func parseRevisionV1(after string) (uint64, bool) {
	if after == "" {
		return 0, true
	}
	n, err := strconv.ParseUint(after, 10, 64)
	return n, err == nil && strconv.FormatUint(n, 10) == after
}

// An already-expanded running input may settle before its next poll. Its
// cleared, actual-result-bound slot is a retired selection, not an unknown ID
// and not permission to reopen raw content or another block.
func retiredToolInputSelectionV2(t *turnV1, id string) bool {
	for _, raw := range t.byKey {
		b := raw.value
		if b.BlockID != id || b.Kind != "tool_input" || b.State != "unknown" || b.Classification != "withheld" || b.Input != nil || b.Body != nil || b.Source.Kind != "tool" || b.Source.ToolResultItemID == "" || raw.inputBytes != 0 || raw.raw != "" {
			continue
		}
		unit := t.byCall[b.Source.CallID]
		if unit == nil || unit.value.UnitID != b.Source.UnitID {
			continue
		}
		for _, anchor := range unit.value.Calls {
			if anchor.CallID == b.Source.CallID && anchor.ToolCallItemID == b.Source.ToolCallItemID && anchor.ToolResultItemID == b.Source.ToolResultItemID && anchor.StartedEventSeq == b.Source.StartedEventSeq {
				return true
			}
		}
	}
	return false
}

func (s *ServiceV1) ReadV1(ctx context.Context, token, after string, wait time.Duration, expandedIDs []string) ResponseV1 {
	if s == nil || ctx == nil || s.authority == nil || len(expandedIDs) > 128 || wait < 0 || wait > 25*time.Second {
		return ResponseV1{Status: "unavailable"}
	}
	revision, ok := parseRevisionV1(after)
	if !ok {
		return ResponseV1{Status: "unavailable"}
	}
	expanded := map[string]bool{}
	for _, id := range expandedIDs {
		if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" || expanded[id] {
			return ResponseV1{Status: "unavailable"}
		}
		expanded[id] = true
	}
	s.mu.Lock()
	s.sweepLockedV1()
	v := s.viewers[token]
	if v == nil || !v.active || v.revoked || s.closed {
		s.mu.Unlock()
		return ResponseV1{Status: "unavailable"}
	}
	if v.polling {
		s.mu.Unlock()
		return ResponseV1{Status: "busy"}
	}
	v.polling = true
	deadline := s.now().Add(wait)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); v.polling = false; s.mu.Unlock() }()
	for {
		a, release, err := s.authority.AcquireCurrent(ctx, v.turn.binding)
		if err != nil || release == nil {
			s.retireRunningAssistantDraftsV1(ctx, v.turn)
			s.RevokeV1(token)
			return ResponseV1{Status: "unavailable"}
		}
		if a.Binding != v.turn.binding || s.authority.ValidateCurrent(ctx, a) != nil {
			release()
			s.retireRunningAssistantDraftsV1(ctx, v.turn)
			s.RevokeV1(token)
			return ResponseV1{Status: "unavailable"}
		}
		s.mu.Lock()
		s.sweepLockedV1()
		if s.closed || ctx.Err() != nil || v.revoked || s.viewers[token] != v || !s.validViewerTurnLockedV1(v.turn) || revision > v.turn.revision {
			s.mu.Unlock()
			release()
			return ResponseV1{Status: "unavailable"}
		}
		if s.validateHistoryCurrentV1(ctx, v.turn) != nil {
			s.retireViewerLockedV1(v)
			s.mu.Unlock()
			release()
			return ResponseV1{Status: "unavailable"}
		}
		for id := range expanded {
			found := false
			for _, b := range v.turn.blocks {
				if b.value.BlockID == id {
					found = true
					break
				}
			}
			if !found {
				if retiredToolInputSelectionV2(v.turn, id) {
					delete(expanded, id)
					continue
				}
				s.mu.Unlock()
				release()
				return ResponseV1{Status: "unavailable"}
			}
		}
		checkedSourceUnits := make(map[string]bool, len(v.turn.units))
		for _, unit := range v.turn.units {
			if !unit.active || !unit.preparationObserved {
				continue
			}
			// An unnamed origin owns no block, but it still belongs to this
			// real live source. Reuse the existing current-source fence.
			authority, ok := s.authority.(port.AssistantDraftReadAuthorityV1)
			if !ok || authority.ValidateAssistantDraftCurrent(ctx, v.turn.binding) != nil {
				if ctx.Err() == nil {
					s.retireProviderUnitLockedV1(v.turn, unit.value.UnitID)
				}
				s.mu.Unlock()
				release()
				s.RevokeV1(token)
				return ResponseV1{Status: "unavailable"}
			}
			checkedSourceUnits[unit.value.UnitID] = true
		}
		for _, block := range v.turn.blocks {
			if !checkedSourceUnits[block.value.Source.UnitID] && ((block.value.Source.Kind == "assistant_draft" || block.value.Kind == "native_reasoning") && block.value.State == "running" && block.value.Classification != "withheld" || block.value.Source.Kind == "tool_preparation" && block.value.State == "preparing") {
				authority, ok := s.authority.(port.AssistantDraftReadAuthorityV1)
				if !ok || authority.ValidateAssistantDraftCurrent(ctx, v.turn.binding) != nil {
					if ctx.Err() == nil {
						s.retireProviderUnitLockedV1(v.turn, block.value.Source.UnitID)
					}
					s.mu.Unlock()
					release()
					s.RevokeV1(token)
					return ResponseV1{Status: "unavailable"}
				}
				checkedSourceUnits[block.value.Source.UnitID] = true
			}
			if block.value.Source.Kind == "tool" && (block.value.Classification != "withheld" || block.value.State == "running") && s.authority.ValidateToolCurrent(ctx, v.turn.binding, block.value.ToolBinding) != nil {
				if ctx.Err() == nil && block.value.State == "running" {
					if block.value.Kind == "web_fetch" {
						s.retireWebInputLockedV1(v.turn, block)
					} else {
						s.retireLiveBlockLockedV1(v.turn, block, "unknown")
					}
					s.touchLockedV1(v.turn)
				}
				s.mu.Unlock()
				release()
				s.RevokeV1(token)
				return ResponseV1{Status: "unavailable"}
			}
		}
		// This bounded copy is the read linearization point under fresh authority.
		if revision < v.turn.revision || after == "" {
			response := s.responseLockedV1(v.turn, "snapshot", expanded)
			s.mu.Unlock()
			release()
			return response
		}
		remaining := deadline.Sub(s.now())
		if remaining <= 0 {
			response := s.responseLockedV1(v.turn, "unchanged", nil)
			s.mu.Unlock()
			release()
			return response
		}
		// Revision observation and broadcast channel registration are atomic.
		wake, revoked := v.turn.wake, v.wake
		s.mu.Unlock()
		release()
		interval := s.limits.AuthorityCheckInterval
		if remaining < interval {
			interval = remaining
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ResponseV1{Status: "unavailable"}
		case <-revoked:
			timer.Stop()
			return ResponseV1{Status: "unavailable"}
		case <-wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// OpenWebLinkV1 resolves a sealed source under the same current native lease.
// Main performs its normal document/effect checks and a second fresh resolution
// before opening the target. No renderer-supplied URL is accepted here.
func (s *ServiceV1) OpenWebLinkV1(ctx context.Context, token, id, digest, role string) WebLinkResponseV1 {
	unavailable := WebLinkResponseV1{Status: "unavailable"}
	if s == nil || ctx == nil || ctx.Err() != nil || s.authority == nil || role != "request" || !domainsecurity.IsSHA256Hex(digest) {
		return unavailable
	}
	s.mu.Lock()
	s.sweepLockedV1()
	viewer := s.viewers[token]
	if s.closed || viewer == nil || !viewer.active || viewer.revoked {
		s.mu.Unlock()
		return unavailable
	}
	binding := viewer.turn.binding
	s.mu.Unlock()
	authority, release, err := s.authority.AcquireCurrent(ctx, binding)
	if err != nil || release == nil {
		return unavailable
	}
	defer release()
	if authority.Binding != binding || s.authority.ValidateCurrent(ctx, authority) != nil {
		return unavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if s.closed || ctx.Err() != nil || viewer.revoked || s.viewers[token] != viewer || s.turns[scopeKeyV1(binding.ThreadID, binding.TurnID)] != viewer.turn {
		return unavailable
	}
	for _, block := range viewer.turn.blocks {
		b := block.value
		if b.BlockID != id {
			continue
		}
		if b.Kind != "web_fetch" || b.State != "running" || b.Classification != "source_exact_local" || b.InputDigest != digest || b.RequestURL != block.webRequestURL || !display.ValidWebRequestURLV1(block.webRequestURL) || s.authority.ValidateToolCurrent(ctx, binding, b.ToolBinding) != nil {
			return unavailable
		}
		scope := display.ScopeV1{ContextEpoch: strconv.FormatUint(binding.Original.ContextEpoch, 10), ProcessIncarnation: viewer.turn.incarnation, ThreadID: binding.ThreadID, TurnID: binding.TurnID, ContextDigest: binding.Original.ContextDigest}
		return WebLinkResponseV1{SchemaVersion: 2, Status: "web_link", Revision: strconv.FormatUint(viewer.turn.revision, 10), Scope: &scope, BlockID: id, InputDigest: digest, Role: role, TargetURL: block.webRequestURL}
	}
	return unavailable
}

func (s *ServiceV1) OpenCaptureV1(ctx context.Context, token, id, digest string) ResponseV1 {
	response := s.ReadV1(ctx, token, "", 0, []string{id})
	if response.Status != "snapshot" {
		return response
	}
	for _, block := range response.Blocks {
		if block.BlockID == id && block.CaptureDigest == digest && block.Body != nil {
			for _, unit := range response.Units {
				if unit.UnitID == block.Source.UnitID {
					return ResponseV1{SchemaVersion: 2, Status: "capture", Revision: response.Revision, Scope: response.Scope, Block: &block, Unit: &unit}
				}
			}
			return ResponseV1{Status: "unavailable"}
		}
	}
	return ResponseV1{Status: "unavailable"}
}

// BeginShutdownDrainV1 retires viewer authority and new producer admission,
// while existing producers may close their sources under the real terminal
// drain. It neither invents a terminal witness nor promotes unfinished text.
func (s *ServiceV1) BeginShutdownDrainV1() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.shutdownDrain = true
	for _, v := range s.viewers {
		s.retireViewerLockedV1(v)
	}
}

func (s *ServiceV1) ShutdownDrainingV1() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdownDrain && !s.closed
}

// RevokeThreadV1 is called only after the real Primary tombstone. Pending
// historical reservations carry their requested thread even before CAS I/O
// returns, so they cannot reintroduce a detached body after this retirement.
// No persistence object or other thread's owner is removed here.
func (s *ServiceV1) RevokeThreadV1(thread string) {
	if s == nil || !domainthread.IsCanonicalRecordID(thread) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.viewers {
		if v.thread == thread || v.turn != nil && v.turn.binding.ThreadID == thread {
			s.retireViewerLockedV1(v)
		}
	}
	for key, t := range s.turns {
		if t.binding.ThreadID == thread {
			s.dropTurnLockedV1(key, t)
		}
	}
}

func (s *ServiceV1) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.shutdownDrain = false
		close(s.janitorStop)
		done := s.janitorDone
		for _, v := range s.viewers {
			s.retireViewerLockedV1(v)
		}
		for key, t := range s.turns {
			s.dropTurnLockedV1(key, t)
		}
		s.mu.Unlock()
		// Wait only after releasing the owner mutex. sync.Once also makes
		// concurrent Close callers wait until this worker is quiescent.
		<-done
	})
	return nil
}

// Closed response variants ensure an empty snapshot still contains blocks:[]
// and no scope/revision/body can accompany a denial.
func (r ResponseV1) MarshalJSON() ([]byte, error) {
	base := map[string]any{"status": r.Status}
	switch r.Status {
	case "authorized", "unchanged", "snapshot", "capture":
		base["schemaVersion"] = 2
		base["revision"] = r.Revision
		base["scope"] = r.Scope
		switch r.Status {
		case "authorized":
			base["presence"] = r.Presence
		case "snapshot":
			blocks := r.Blocks
			if blocks == nil {
				blocks = []display.BlockV1{}
			}
			units := r.Units
			if units == nil {
				units = []display.UnitV2{}
			}
			base["blocks"] = blocks
			base["units"] = units
		case "capture":
			base["block"] = r.Block
			base["unit"] = r.Unit
		}
	default:
		if r.Status != "busy" && r.Status != "revoked" {
			base["status"] = "unavailable"
		}
	}
	return json.Marshal(base)
}

// BindingV2 is host-only, never a display DTO or an HTTP input.
func (p *ProducerV1) BindingV2() port.BindingV1 {
	if p == nil {
		return port.BindingV1{}
	}
	return p.turn.binding
}

// clearLiveContentV1 removes source-bearing values without changing immutable
// coordinates or claiming settlement. Every snapshot is a complete replacement.
func clearLiveContentV1(b *display.BlockV1) {
	b.Input = nil
	b.ToolName, b.Title, b.Preview, b.Path, b.CaptureDigest = "", "", "", "", ""
	b.Body, b.InputUtf16CodeUnits = nil, nil
	b.DraftSegments, b.DraftBytes = nil, ""
	b.Classification = "withheld"
	b.Truncated = false
}
func (s *ServiceV1) retireLiveBlockLockedV1(t *turnV1, b *rawBlockV1, state string) {
	t.rawBytes -= b.inputBytes
	b.inputBytes = 0
	t.rawBytes -= len(b.raw)
	b.raw = ""
	if b.preparation != nil {
		t.rawBytes -= b.preparation.RawBytesV1()
	}
	b.preparation, b.preparationName = nil, ""
	t.displayBytes -= blockBytesV1(b.value)
	clearLiveContentV1(&b.value)
	b.value.State = state
	b.value.DurationMs = s.now().Sub(b.started).Milliseconds()
}
func (s *ServiceV1) retireProviderUnitLockedV1(t *turnV1, unitID string) {
	for _, unit := range t.units {
		if unit.value.UnitID == unitID {
			unit.active, unit.retired = false, true
		}
	}
	for _, block := range t.blocks {
		if block.value.Source.UnitID == unitID && (block.value.Kind == "native_reasoning" && block.value.State == "running" || block.value.Kind == "assistant_draft" && block.value.State == "running" || block.value.Kind == "tool_preparation" && block.value.State == "preparing") {
			s.retireLiveBlockLockedV1(t, block, "unknown")
		}
	}
	s.touchLockedV1(t)
}
func boundProcessTextV1(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	end := limit
	for end > 0 && !utf8.ValidString(text[:end]) {
		end--
	}
	return text[:end], true
}

var reasoningParagraphSeparatorV1 = regexp.MustCompile(`\r?\n(?:[\t ]*\r?\n)+`)

func liveReasoningPreviewV1(text string) string {
	summary, start := "", 0
	for _, separator := range reasoningParagraphSeparatorV1.FindAllStringIndex(text, -1) {
		end := separator[0] + strings.IndexByte(text[separator[0]:separator[1]], '\n')
		if newline := strings.IndexByte(text[start:], '\n'); newline >= 0 && start+newline <= end {
			if candidate := strings.TrimSpace(text[start : start+newline]); candidate != "" {
				summary = candidate
			}
		}
		start = separator[1]
	}
	if newline := strings.IndexByte(text[start:], '\n'); newline >= 0 {
		if candidate := strings.TrimSpace(text[start : start+newline]); candidate != "" {
			summary = candidate
		}
	}
	preview, _ := boundProcessTextV1(strings.ReplaceAll(summary, "**", ""), display.ReasoningPreviewByteLimitV1)
	return preview
}
func (s *ServiceV1) projectLiveReasoningLockedV1(t *turnV1, b *rawBlockV1, secrets []string) {
	t.displayBytes -= blockBytesV1(b.value)
	clearLiveContentV1(&b.value)
	if b.overflow {
		return
	}
	label := display.ProjectCaptureV1(display.RawCaptureV1{Complete: true, Title: reasoningTitleV1(b.value.Source)}, secrets...)
	if label.Classification == "withheld" {
		return
	}
	b.value.Title = label.Title
	b.value.Body = &display.BodyV1{}
	b.value.Classification = label.Classification
	capture := display.ProjectAssistantDraftV1(b.raw, secrets...)
	if capture.Classification != "withheld" {
		text, cut := boundProcessTextV1(capture.Body, display.CaptureBodyByteLimitV1)
		b.value.Body.Text = text
		b.value.Truncated = cut || label.Truncated
		if capture.Classification == "credential_redacted" {
			b.value.Classification = capture.Classification
		}
		b.value.Preview = liveReasoningPreviewV1(capture.Body)
	}
	if size := blockBytesV1(b.value); size <= s.limits.DisplayBytes-t.displayBytes {
		t.displayBytes += size
	} else {
		clearLiveContentV1(&b.value)
	}
}

func reasoningTitleV1(source display.SourceV2) string {
	if source.ReasoningForm == "summary" {
		return "Reasoning summary"
	}
	return "Think"
}

func (p *ProducerV1) toolPreparationLockedV1(frame domainmodel.ProcessFrameV1) {
	f := frame.ToolPreparation
	if f == nil || f.Index < 0 || uint64(f.Index) > 1<<53-1 || (frame.Kind != "start" && frame.Kind != "delta" && frame.Kind != "end") {
		return
	}
	s, t := p.owner, p.turn
	unit := t.sequenceUnits[p.handle+"/"+strconv.FormatUint(frame.ProviderSequence, 10)]
	if unit == nil || !unit.active || unit.namespace.ProviderCallSequence != frame.ProviderSequence {
		return
	}
	key := p.handle + "/preparation/" + strconv.FormatUint(frame.ProviderSequence, 10) + "/" + strconv.FormatUint(frame.Attempt, 10) + "/" + strconv.Itoa(f.Index)
	if frame.Kind == "start" {
		if f.CallID != "" || f.ToolName != "" || f.Arguments != "" || frame.Text != "" || t.preparationOrigins[key] != 0 || len(t.preparationOrigins) >= s.limits.MaxBlocks || t.sourceOrdinal == ^uint64(0) {
			return
		}
		// One bounded control coordinate, allocated at the actual first slot
		// observation. Unnamed slots own no display block, reader or bytes.
		t.sourceOrdinal++
		t.preparationOrigins[key] = t.sourceOrdinal
		unit.preparationObserved = true
		return
	}
	ordinal := t.preparationOrigins[key]
	if ordinal == 0 || !domainmodel.IsHostToolCallIDV1(f.CallID) || t.byCall[f.CallID] != nil {
		return
	}
	block := t.byKey[key]
	if block == nil {
		// Only the real named delta opens a seat. Final bytes cannot invent it.
		if frame.Kind != "delta" || strings.TrimSpace(f.ToolName) == "" || len(f.ToolName) > 1024 || len(t.blocks) >= s.limits.MaxBlocks {
			return
		}
		for _, existing := range t.blocks {
			if existing.value.Source.UnitID == unit.value.UnitID && existing.value.Source.CallID == f.CallID && existing.value.Attempt == frame.Attempt {
				return
			}
		}
		block = &rawBlockV1{value: display.BlockV1{BlockID: randomIDV1(), Kind: "tool_preparation", State: "preparing", Attempt: frame.Attempt, Source: display.SourceV2{Kind: "tool_preparation", UnitID: unit.value.UnitID, ProducerID: unit.value.ProducerID, ProviderSequence: unit.value.ProviderSequence, Attempt: frame.Attempt, CallID: f.CallID, SourceOrdinal: strconv.FormatUint(ordinal, 10)}, Classification: "withheld"}, preparation: display.NewPartialArgumentsV1(f.ToolName), preparationName: f.ToolName, producerHandle: p.handle, providerSequence: frame.ProviderSequence, key: key, started: s.now()}
		if block.value.BlockID == "" {
			return
		}
		t.blocks = append(t.blocks, block)
		t.byKey[key] = block
	}
	if block.value.State != "preparing" || block.preparationSealed || block.overflow || block.value.Source.CallID != f.CallID || f.ToolName != "" && f.ToolName != block.preparationName {
		return
	}
	before := block.preparation.RawBytesV1()
	needed := len(f.Arguments)
	if frame.Kind == "delta" {
		needed += before
	}
	if needed > s.limits.RawBytes-t.rawBytes+before {
		s.retireLiveBlockLockedV1(t, block, "preparing")
		block.overflow = true
		s.touchLockedV1(t)
		return
	}
	ok := false
	if frame.Kind == "end" {
		ok = block.preparation.SettleV1(f.Arguments)
		block.preparationSealed = true
	} else {
		ok = block.preparation.AppendV1(f.Arguments)
	}
	if !ok {
		s.retireLiveBlockLockedV1(t, block, "preparing")
		block.overflow = true
		s.touchLockedV1(t)
		return
	}
	t.rawBytes += block.preparation.RawBytesV1() - before
	t.displayBytes -= blockBytesV1(block.value)
	clearLiveContentV1(&block.value)
	projection := block.preparation.ProjectV1(append(append([]string(nil), p.secrets...), frame.ProjectionSecrets...)...)
	if projection.Classification != "withheld" {
		block.value.ToolName, block.value.Preview, block.value.Path, block.value.InputUtf16CodeUnits = projection.ToolName, projection.Preview, projection.Path, projection.InputUtf16CodeUnits
		block.value.Classification = projection.Classification
		if size := blockBytesV1(block.value); size <= s.limits.DisplayBytes-t.displayBytes {
			t.displayBytes += size
		} else {
			clearLiveContentV1(&block.value)
		}
	}
	t.activity = s.now()
	s.touchLockedV1(t)
}

// ShellInputV1 publishes only the parsed command of an actual dispatched
// foreground call. It does not claim OS startup or observe stdout. The command
// outlives this short publisher; every read revalidates its active grant/hash.
func (p *ProducerV1) ShellInputV1(callID, command string, tool display.ToolBindingV1) bool {
	if p == nil || strings.TrimSpace(command) == "" || !domainmodel.IsHostToolCallIDV1(callID) || tool.CallID != callID || tool.ToolName != "bash" || tool.ResultItemID != "" || !domainsecurity.IsSHA256Hex(tool.GrantID) || !domainsecurity.IsSHA256Hex(tool.InputArgsHash) {
		return false
	}
	capture := display.ProjectCaptureV1(display.RawCaptureV1{Complete: true, Title: command}, p.secrets...)
	if capture.Classification == "withheld" || capture.Title == "" {
		return false
	}
	s := p.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLockedV1()
	if !p.validLockedV1() {
		return false
	}
	t := p.turn
	unit := t.byCall[callID]
	if unit == nil {
		return false
	}
	var anchor *display.CallAnchorV2
	for i := range unit.value.Calls {
		if unit.value.Calls[i].CallID == callID {
			anchor = &unit.value.Calls[i]
			break
		}
	}
	if anchor == nil || anchor.ToolResultItemID != "" || anchor.StartedEventSeq == "" {
		return false
	}
	key := "tool/" + callID
	if existing := t.byKey[key]; existing != nil {
		if existing.value.State == "running" && existing.value.ToolBinding == tool && existing.value.Title == "" && existing.value.Input != nil && len(capture.Title) <= s.limits.DisplayBytes-t.displayBytes {
			existing.value.Title, existing.value.Truncated = capture.Title, capture.Truncated
			if capture.Classification == "credential_redacted" || existing.value.Classification == "withheld" {
				existing.value.Classification = capture.Classification
			}
			t.displayBytes += len(capture.Title)
			s.touchLockedV1(t)
			return true
		}
		return existing.value.State == "running" && existing.value.ToolBinding == tool && existing.value.Title == capture.Title
	}
	if len(t.blocks) >= s.limits.MaxBlocks || len(capture.Title) > s.limits.DisplayBytes-t.displayBytes {
		return false
	}
	block := &rawBlockV1{value: display.BlockV1{BlockID: randomIDV1(), Kind: "shell", State: "running", Source: display.SourceV2{Kind: "tool", UnitID: unit.value.UnitID, CallID: callID, ToolCallItemID: anchor.ToolCallItemID, StartedEventSeq: anchor.StartedEventSeq}, ToolBinding: tool, Title: capture.Title, Classification: capture.Classification, Truncated: capture.Truncated}, key: key, started: s.now()}
	if block.value.BlockID == "" {
		return false
	}
	t.blocks = append(t.blocks, block)
	t.byKey[key] = block
	t.displayBytes += blockBytesV1(block.value)
	t.activity = s.now()
	s.touchLockedV1(t)
	return true
}
