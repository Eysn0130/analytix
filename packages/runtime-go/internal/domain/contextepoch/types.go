package contextepoch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

const ContractVersion = 1

type PromptBoundary string

const (
	BoundaryStablePrefix    PromptBoundary = "stable-prefix"
	BoundaryDynamicContext  PromptBoundary = "dynamic-context"
	BoundaryTurnTail        PromptBoundary = "turn-tail"
	BoundaryStoreOnly       PromptBoundary = "store-only"
	BoundaryUIOnly          PromptBoundary = "ui-only"
	BoundaryDiagnosticsOnly PromptBoundary = "diagnostics-only"
)

type TrustState string

const (
	TrustTrusted     TrustState = "trusted"
	TrustUntrusted   TrustState = "untrusted"
	TrustUnavailable TrustState = "unavailable"
)

type ActivationState string

const (
	ActivationInactive ActivationState = "inactive"
	ActivationActive   ActivationState = "active"
)

type ChangeReason string

const (
	ReasonSourceAdded            ChangeReason = "source-added"
	ReasonSourceRemoved          ChangeReason = "source-removed"
	ReasonSourceDigestChanged    ChangeReason = "source-digest-changed"
	ReasonActivationChanged      ChangeReason = "activation-changed"
	ReasonBudgetChanged          ChangeReason = "budget-changed"
	ReasonBoundaryChanged        ChangeReason = "prompt-boundary-changed"
	ReasonTrustChanged           ChangeReason = "trust-state-changed"
	ReasonReferenceChanged       ChangeReason = "source-reference-changed"
	ReasonSourceUnavailable      ChangeReason = "source-unavailable"
	ReasonCompactionRecovery     ChangeReason = "compaction-recovery"
	ReasonRestartReconcile       ChangeReason = "restart-reconcile"
	ReasonToolSchemaChanged      ChangeReason = "tool-schema-changed"
	ReasonSecurityContextChanged ChangeReason = "security-context-changed"
)

type SourceEntry struct {
	Version          int             `json:"version"`
	SourceID         string          `json:"sourceId"`
	Kind             string          `json:"kind"`
	Reference        string          `json:"reference,omitempty"`
	Digest           string          `json:"digest"`
	Sequence         uint64          `json:"sequence"`
	TrustState       TrustState      `json:"trustState"`
	PromptBoundary   PromptBoundary  `json:"promptBoundary"`
	TokenBudget      int             `json:"tokenBudget"`
	ActivationState  ActivationState `json:"activationState"`
	ActivationReason string          `json:"activationReason,omitempty"`
}

type SourceSnapshot struct {
	SourceID         string          `json:"sourceId"`
	Kind             string          `json:"kind"`
	ReferenceDigest  string          `json:"referenceDigest,omitempty"`
	Digest           string          `json:"digest"`
	Sequence         uint64          `json:"sequence"`
	TrustState       TrustState      `json:"trustState"`
	PromptBoundary   PromptBoundary  `json:"promptBoundary"`
	TokenBudget      int             `json:"tokenBudget"`
	ActivationState  ActivationState `json:"activationState"`
	ActivationReason string          `json:"activationReason,omitempty"`
}

type ChangeImpact struct {
	StablePrefix    bool `json:"stablePrefix"`
	DynamicContext  bool `json:"dynamicContext"`
	TurnTail        bool `json:"turnTail"`
	DiagnosticsOnly bool `json:"diagnosticsOnly"`
}

type Snapshot struct {
	Version          int              `json:"version"`
	ThreadID         string           `json:"threadId"`
	Epoch            uint64           `json:"epoch"`
	BaselineSequence uint64           `json:"baselineSequence"`
	RegistryDigest   string           `json:"registryDigest"`
	Sources          []SourceSnapshot `json:"sources"`
	RecoveryDigest   string           `json:"recoveryDigest,omitempty"`
	AcceptedAt       string           `json:"acceptedAt"`
	ChangeReasons    []ChangeReason   `json:"changeReasons"`
	Impact           ChangeImpact     `json:"impact"`
	ContextDigest    string           `json:"contextDigest"`
}

type State struct {
	Version          int           `json:"version"`
	ThreadID         string        `json:"threadId"`
	Registry         []SourceEntry `json:"registry"`
	AcceptedSnapshot Snapshot      `json:"acceptedSnapshot"`
	StateDigest      string        `json:"stateDigest"`
}

type ProviderFragment struct {
	Boundary PromptBoundary
	Content  string
}

type UnavailableSource struct {
	SourceID string
	Reason   string
}

type ProviderContext struct {
	StablePrefix []ProviderFragment
	Dynamic      []ProviderFragment
	TurnTail     []ProviderFragment
	Unavailable  []UnavailableSource
}

var codePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func IsProviderVisibleBoundary(boundary PromptBoundary) bool {
	switch boundary {
	case BoundaryStablePrefix, BoundaryDynamicContext, BoundaryTurnTail:
		return true
	default:
		return false
	}
}

func IsValidPromptBoundary(boundary PromptBoundary) bool {
	switch boundary {
	case BoundaryStablePrefix, BoundaryDynamicContext, BoundaryTurnTail, BoundaryStoreOnly, BoundaryUIOnly, BoundaryDiagnosticsOnly:
		return true
	default:
		return false
	}
}

func IsValidTrustState(state TrustState) bool {
	switch state {
	case TrustTrusted, TrustUntrusted, TrustUnavailable:
		return true
	default:
		return false
	}
}

func IsValidActivationState(state ActivationState) bool {
	return state == ActivationInactive || state == ActivationActive
}

func IsValidChangeReason(reason ChangeReason) bool {
	switch reason {
	case ReasonSourceAdded, ReasonSourceRemoved, ReasonSourceDigestChanged, ReasonActivationChanged,
		ReasonBudgetChanged, ReasonBoundaryChanged, ReasonTrustChanged, ReasonReferenceChanged,
		ReasonSourceUnavailable, ReasonCompactionRecovery, ReasonRestartReconcile,
		ReasonToolSchemaChanged, ReasonSecurityContextChanged:
		return true
	default:
		return false
	}
}

func SHA256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func SourceSnapshotFromEntry(entry SourceEntry) SourceSnapshot {
	referenceDigest := ""
	if reference := strings.TrimSpace(entry.Reference); reference != "" {
		referenceDigest = SHA256Hex([]byte(reference))
	}
	return SourceSnapshot{
		SourceID:         strings.TrimSpace(entry.SourceID),
		Kind:             strings.TrimSpace(entry.Kind),
		ReferenceDigest:  referenceDigest,
		Digest:           strings.TrimSpace(entry.Digest),
		Sequence:         entry.Sequence,
		TrustState:       entry.TrustState,
		PromptBoundary:   entry.PromptBoundary,
		TokenBudget:      entry.TokenBudget,
		ActivationState:  entry.ActivationState,
		ActivationReason: strings.TrimSpace(entry.ActivationReason),
	}
}

func CanonicalRegistry(entries []SourceEntry) []SourceEntry {
	out := append([]SourceEntry(nil), entries...)
	for index := range out {
		out[index].Version = ContractVersion
		out[index].SourceID = strings.TrimSpace(out[index].SourceID)
		out[index].Kind = strings.TrimSpace(out[index].Kind)
		out[index].Reference = strings.TrimSpace(out[index].Reference)
		out[index].Digest = strings.ToLower(strings.TrimSpace(out[index].Digest))
		out[index].ActivationReason = strings.TrimSpace(out[index].ActivationReason)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceID < out[j].SourceID })
	return out
}

func RegistryDigest(entries []SourceEntry) string {
	data, _ := json.Marshal(CanonicalRegistry(entries))
	return SHA256Hex(data)
}

func SealSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Version = ContractVersion
	snapshot.ThreadID = strings.TrimSpace(snapshot.ThreadID)
	snapshot.RegistryDigest = strings.ToLower(strings.TrimSpace(snapshot.RegistryDigest))
	snapshot.RecoveryDigest = strings.ToLower(strings.TrimSpace(snapshot.RecoveryDigest))
	snapshot.AcceptedAt = strings.TrimSpace(snapshot.AcceptedAt)
	snapshot.Sources = append([]SourceSnapshot(nil), snapshot.Sources...)
	sort.Slice(snapshot.Sources, func(i, j int) bool { return snapshot.Sources[i].SourceID < snapshot.Sources[j].SourceID })
	snapshot.ChangeReasons = canonicalReasons(snapshot.ChangeReasons)
	snapshot.ContextDigest = ""
	data, _ := json.Marshal(snapshot)
	snapshot.ContextDigest = SHA256Hex(data)
	return snapshot
}

func SealState(state State) State {
	state.Version = ContractVersion
	state.ThreadID = strings.TrimSpace(state.ThreadID)
	state.Registry = CanonicalRegistry(state.Registry)
	state.AcceptedSnapshot = SealSnapshot(state.AcceptedSnapshot)
	state.StateDigest = ""
	data, _ := json.Marshal(state)
	state.StateDigest = SHA256Hex(data)
	return state
}

func ParseState(value any) (State, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return State{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var state State
	if err := decoder.Decode(&state); err != nil {
		return State{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return State{}, errors.New("context epoch state contains trailing JSON")
	}
	if err := ValidateState(state); err != nil {
		return State{}, err
	}
	return state, nil
}

func ValidateState(state State) error {
	if state.Version != ContractVersion || !codePattern.MatchString(strings.TrimSpace(state.ThreadID)) {
		return errors.New("context epoch state identity is invalid")
	}
	seen := map[string]bool{}
	for _, entry := range state.Registry {
		if err := ValidateSourceEntry(entry); err != nil {
			return err
		}
		if seen[entry.SourceID] {
			return errors.New("context epoch source registry contains duplicate sourceId")
		}
		seen[entry.SourceID] = true
	}
	if err := ValidateSnapshot(state.AcceptedSnapshot); err != nil {
		return err
	}
	if state.AcceptedSnapshot.ThreadID != state.ThreadID {
		return errors.New("context epoch snapshot thread mismatch")
	}
	expected := SealState(state)
	if strings.TrimSpace(state.StateDigest) == "" || state.StateDigest != expected.StateDigest {
		return errors.New("context epoch state integrity is invalid")
	}
	return nil
}

func ValidateSourceEntry(entry SourceEntry) error {
	if entry.Version != ContractVersion || !codePattern.MatchString(strings.TrimSpace(entry.SourceID)) ||
		!codePattern.MatchString(strings.TrimSpace(entry.Kind)) || !validSHA256(entry.Digest) || entry.Sequence == 0 ||
		!IsValidTrustState(entry.TrustState) || !IsValidPromptBoundary(entry.PromptBoundary) ||
		!IsValidActivationState(entry.ActivationState) || entry.TokenBudget < 0 {
		return errors.New("context epoch source entry is invalid")
	}
	if entry.ActivationReason != "" && !codePattern.MatchString(strings.TrimSpace(entry.ActivationReason)) {
		return errors.New("context epoch activation reason is not a sanitized code")
	}
	if entry.ActivationState == ActivationActive && IsProviderVisibleBoundary(entry.PromptBoundary) && entry.TokenBudget == 0 {
		return errors.New("active provider-visible context source requires a token budget")
	}
	return nil
}

func ValidateSnapshot(snapshot Snapshot) error {
	if snapshot.Version != ContractVersion || !codePattern.MatchString(strings.TrimSpace(snapshot.ThreadID)) ||
		snapshot.Epoch == 0 || !validSHA256(snapshot.RegistryDigest) || strings.TrimSpace(snapshot.AcceptedAt) == "" {
		return errors.New("context epoch snapshot is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, snapshot.AcceptedAt); err != nil {
		return errors.New("context epoch snapshot acceptedAt is invalid")
	}
	if snapshot.RecoveryDigest != "" && !validSHA256(snapshot.RecoveryDigest) {
		return errors.New("context epoch recovery digest is invalid")
	}
	seen := map[string]bool{}
	for _, source := range snapshot.Sources {
		entry := SourceEntry{
			Version: ContractVersion, SourceID: source.SourceID, Kind: source.Kind, Digest: source.Digest,
			Sequence: source.Sequence, TrustState: source.TrustState, PromptBoundary: source.PromptBoundary,
			TokenBudget: source.TokenBudget, ActivationState: source.ActivationState, ActivationReason: source.ActivationReason,
		}
		if err := ValidateSourceEntry(entry); err != nil {
			return err
		}
		if source.ReferenceDigest != "" && !validSHA256(source.ReferenceDigest) {
			return errors.New("context epoch snapshot reference digest is invalid")
		}
		if seen[source.SourceID] {
			return errors.New("context epoch snapshot contains duplicate sourceId")
		}
		seen[source.SourceID] = true
	}
	for _, reason := range snapshot.ChangeReasons {
		if !IsValidChangeReason(reason) {
			return errors.New("context epoch snapshot contains an invalid change reason")
		}
	}
	expected := SealSnapshot(snapshot)
	if strings.TrimSpace(snapshot.ContextDigest) == "" || snapshot.ContextDigest != expected.ContextDigest {
		return errors.New("context epoch snapshot integrity is invalid")
	}
	return nil
}

func canonicalReasons(reasons []ChangeReason) []ChangeReason {
	seen := map[ChangeReason]bool{}
	out := make([]ChangeReason, 0, len(reasons))
	for _, reason := range reasons {
		if reason == "" || seen[reason] {
			continue
		}
		seen[reason] = true
		out = append(out, reason)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func validSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
