package contextepoch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	contextsourceport "analytix.local/runtime-go/internal/ports/contextsource"
)

var ErrMidStreamMutation = errors.New("context_epoch_mid_stream_mutation_rejected")

type ReconcileCause string

const (
	CauseRequestBoundary ReconcileCause = "request-boundary"
	CauseCompaction      ReconcileCause = "compaction"
	CauseRestart         ReconcileCause = "restart"
	CauseWorkspaceRebind ReconcileCause = "workspace-rebind"
)

type ReconcileInput struct {
	State          domaincontextepoch.State
	Cause          ReconcileCause
	RecoveryDigest string
	At             time.Time
	ActiveStream   bool
}

type ReconcileResult struct {
	State   domaincontextepoch.State
	Changed bool
}

func BootstrapState(threadID string, epoch uint64, registry []domaincontextepoch.SourceEntry, at time.Time) (domaincontextepoch.State, error) {
	threadID = strings.TrimSpace(threadID)
	if epoch == 0 {
		epoch = 1
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	registry = domaincontextepoch.CanonicalRegistry(registry)
	for _, entry := range registry {
		if err := domaincontextepoch.ValidateSourceEntry(entry); err != nil {
			return domaincontextepoch.State{}, err
		}
	}
	snapshot := domaincontextepoch.SealSnapshot(domaincontextepoch.Snapshot{
		ThreadID:         threadID,
		Epoch:            epoch,
		BaselineSequence: maxSequence(registry),
		RegistryDigest:   domaincontextepoch.RegistryDigest(registry),
		Sources:          sourceSnapshots(registry),
		AcceptedAt:       at.UTC().Format(time.RFC3339Nano),
		ChangeReasons:    []domaincontextepoch.ChangeReason{},
	})
	state := domaincontextepoch.SealState(domaincontextepoch.State{
		ThreadID: threadID, Registry: registry, AcceptedSnapshot: snapshot,
	})
	if err := domaincontextepoch.ValidateState(state); err != nil {
		return domaincontextepoch.State{}, err
	}
	return state, nil
}

func DefaultState(threadID string, epoch uint64, at time.Time) (domaincontextepoch.State, error) {
	return BootstrapState(threadID, epoch, nil, at)
}

func ParseOrDefault(value any, threadID string, epoch uint64, at time.Time) (domaincontextepoch.State, bool, error) {
	if value == nil {
		state, err := DefaultState(threadID, epoch, at)
		return state, true, err
	}
	state, err := domaincontextepoch.ParseState(value)
	if err != nil {
		return domaincontextepoch.State{}, false, fmt.Errorf("invalid persisted context epoch state: %w", err)
	}
	if state.ThreadID != strings.TrimSpace(threadID) {
		return domaincontextepoch.State{}, false, errors.New("context epoch state thread mismatch")
	}
	return state, false, nil
}

func Upsert(state domaincontextepoch.State, entry domaincontextepoch.SourceEntry, activeStream bool) (domaincontextepoch.State, bool, error) {
	if activeStream {
		return state, false, ErrMidStreamMutation
	}
	if err := domaincontextepoch.ValidateState(state); err != nil {
		return state, false, err
	}
	entry.Version = domaincontextepoch.ContractVersion
	entry.SourceID = strings.TrimSpace(entry.SourceID)
	entry.Kind = strings.TrimSpace(entry.Kind)
	entry.Reference = strings.TrimSpace(entry.Reference)
	entry.Digest = strings.ToLower(strings.TrimSpace(entry.Digest))
	entry.ActivationReason = strings.TrimSpace(entry.ActivationReason)
	if entry.Sequence == 0 {
		entry.Sequence = 1
	}
	registry := append([]domaincontextepoch.SourceEntry(nil), state.Registry...)
	index := -1
	for candidate := range registry {
		if registry[candidate].SourceID == entry.SourceID {
			index = candidate
			break
		}
	}
	if index >= 0 && sameSourceConfiguration(registry[index], entry) {
		return state, false, nil
	}
	entry.Sequence = maxSequence(registry) + 1
	if index >= 0 {
		registry[index] = entry
	} else {
		registry = append(registry, entry)
	}
	if err := domaincontextepoch.ValidateSourceEntry(entry); err != nil {
		return state, false, err
	}
	state.Registry = registry
	state = domaincontextepoch.SealState(state)
	return state, true, nil
}

func Remove(state domaincontextepoch.State, sourceID string, activeStream bool) (domaincontextepoch.State, bool, error) {
	if activeStream {
		return state, false, ErrMidStreamMutation
	}
	if err := domaincontextepoch.ValidateState(state); err != nil {
		return state, false, err
	}
	sourceID = strings.TrimSpace(sourceID)
	registry := make([]domaincontextepoch.SourceEntry, 0, len(state.Registry))
	removed := false
	for _, entry := range state.Registry {
		if entry.SourceID == sourceID {
			removed = true
			continue
		}
		registry = append(registry, entry)
	}
	if !removed {
		return state, false, nil
	}
	state.Registry = registry
	state = domaincontextepoch.SealState(state)
	return state, true, nil
}

func Reconcile(input ReconcileInput) (ReconcileResult, error) {
	if input.ActiveStream {
		return ReconcileResult{State: input.State}, ErrMidStreamMutation
	}
	state := input.State
	if err := domaincontextepoch.ValidateState(state); err != nil {
		return ReconcileResult{}, err
	}
	recoveryDigest := strings.ToLower(strings.TrimSpace(input.RecoveryDigest))
	if recoveryDigest == "" && input.Cause != CauseCompaction {
		recoveryDigest = state.AcceptedSnapshot.RecoveryDigest
	}
	registryDigest := domaincontextepoch.RegistryDigest(state.Registry)
	changed := registryDigest != state.AcceptedSnapshot.RegistryDigest || recoveryDigest != state.AcceptedSnapshot.RecoveryDigest
	if !changed {
		return ReconcileResult{State: state}, nil
	}
	reasons, impact := diffSnapshot(state.AcceptedSnapshot, state.Registry)
	if recoveryDigest != state.AcceptedSnapshot.RecoveryDigest && input.Cause == CauseCompaction {
		reasons = append(reasons, domaincontextepoch.ReasonCompactionRecovery)
	}
	if input.Cause == CauseRestart {
		reasons = append(reasons, domaincontextepoch.ReasonRestartReconcile)
	}
	if len(reasons) == 0 {
		reasons = append(reasons, domaincontextepoch.ReasonSecurityContextChanged)
	}
	if !impact.StablePrefix && !impact.DynamicContext && !impact.TurnTail {
		impact.DiagnosticsOnly = true
	}
	at := input.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	state.AcceptedSnapshot = domaincontextepoch.SealSnapshot(domaincontextepoch.Snapshot{
		ThreadID:         state.ThreadID,
		Epoch:            state.AcceptedSnapshot.Epoch + 1,
		BaselineSequence: maxSequence(state.Registry),
		RegistryDigest:   registryDigest,
		Sources:          sourceSnapshots(state.Registry),
		RecoveryDigest:   recoveryDigest,
		AcceptedAt:       at.UTC().Format(time.RFC3339Nano),
		ChangeReasons:    reasons,
		Impact:           impact,
	})
	state = domaincontextepoch.SealState(state)
	if err := domaincontextepoch.ValidateState(state); err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{State: state, Changed: true}, nil
}

func BuildProviderContext(ctx context.Context, state domaincontextepoch.State, reader contextsourceport.Reader) (domaincontextepoch.ProviderContext, error) {
	if err := domaincontextepoch.ValidateState(state); err != nil {
		return domaincontextepoch.ProviderContext{}, err
	}
	entryByID := map[string]domaincontextepoch.SourceEntry{}
	for _, entry := range state.Registry {
		entryByID[entry.SourceID] = entry
	}
	type selected struct {
		id       string
		fragment domaincontextepoch.ProviderFragment
	}
	selectedFragments := []selected{}
	providerContext := domaincontextepoch.ProviderContext{}
	for _, source := range state.AcceptedSnapshot.Sources {
		if source.ActivationState != domaincontextepoch.ActivationActive || !domaincontextepoch.IsProviderVisibleBoundary(source.PromptBoundary) {
			continue
		}
		entry, ok := entryByID[source.SourceID]
		if !ok || entry.TrustState == domaincontextepoch.TrustUnavailable {
			providerContext.Unavailable = append(providerContext.Unavailable, unavailable(source.SourceID, "source-unavailable"))
			continue
		}
		if source.PromptBoundary == domaincontextepoch.BoundaryStablePrefix && entry.TrustState != domaincontextepoch.TrustTrusted {
			providerContext.Unavailable = append(providerContext.Unavailable, unavailable(source.SourceID, "stable-prefix-requires-trusted-source"))
			continue
		}
		if reader == nil {
			providerContext.Unavailable = append(providerContext.Unavailable, unavailable(source.SourceID, "source-reader-unavailable"))
			continue
		}
		content, err := reader.ReadContextSource(ctx, state.ThreadID, entry)
		if err != nil {
			providerContext.Unavailable = append(providerContext.Unavailable, unavailable(source.SourceID, "source-read-failed"))
			continue
		}
		actualDigest := domaincontextepoch.SHA256Hex([]byte(content.Text))
		declaredDigest := strings.ToLower(strings.TrimSpace(content.Digest))
		if actualDigest != entry.Digest || (declaredDigest != "" && declaredDigest != actualDigest) {
			providerContext.Unavailable = append(providerContext.Unavailable, unavailable(source.SourceID, "source-digest-mismatch"))
			continue
		}
		if EstimateTokens(content.Text) > entry.TokenBudget {
			providerContext.Unavailable = append(providerContext.Unavailable, unavailable(source.SourceID, "source-token-budget-exceeded"))
			continue
		}
		if content.Text == "" {
			continue
		}
		selectedFragments = append(selectedFragments, selected{
			id: source.SourceID,
			fragment: domaincontextepoch.ProviderFragment{
				Boundary: source.PromptBoundary,
				Content:  content.Text,
			},
		})
	}
	sort.Slice(selectedFragments, func(i, j int) bool { return selectedFragments[i].id < selectedFragments[j].id })
	for _, selected := range selectedFragments {
		switch selected.fragment.Boundary {
		case domaincontextepoch.BoundaryStablePrefix:
			providerContext.StablePrefix = append(providerContext.StablePrefix, selected.fragment)
		case domaincontextepoch.BoundaryDynamicContext:
			providerContext.Dynamic = append(providerContext.Dynamic, selected.fragment)
		case domaincontextepoch.BoundaryTurnTail:
			providerContext.TurnTail = append(providerContext.TurnTail, selected.fragment)
		}
	}
	sort.Slice(providerContext.Unavailable, func(i, j int) bool {
		return providerContext.Unavailable[i].SourceID < providerContext.Unavailable[j].SourceID
	})
	return providerContext, nil
}

func Recover(ctx context.Context, state domaincontextepoch.State, reader contextsourceport.Reader, at time.Time) (ReconcileResult, error) {
	providerContext, err := BuildProviderContext(ctx, state, reader)
	if err != nil {
		return ReconcileResult{}, err
	}
	next := state
	for _, unavailableSource := range providerContext.Unavailable {
		for _, entry := range next.Registry {
			if entry.SourceID != unavailableSource.SourceID || entry.TrustState == domaincontextepoch.TrustUnavailable {
				continue
			}
			entry.TrustState = domaincontextepoch.TrustUnavailable
			var changed bool
			next, changed, err = Upsert(next, entry, false)
			if err != nil {
				return ReconcileResult{}, err
			}
			_ = changed
			break
		}
	}
	return Reconcile(ReconcileInput{State: next, Cause: CauseRestart, At: at})
}

func EstimateTokens(content string) int {
	if content == "" {
		return 0
	}
	return (len([]byte(content)) + 3) / 4
}

func PublicState(state domaincontextepoch.State) map[string]any {
	body, _ := json.Marshal(state)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func PublicSnapshot(snapshot domaincontextepoch.Snapshot) map[string]any {
	body, _ := json.Marshal(snapshot)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func sourceSnapshots(entries []domaincontextepoch.SourceEntry) []domaincontextepoch.SourceSnapshot {
	entries = domaincontextepoch.CanonicalRegistry(entries)
	out := make([]domaincontextepoch.SourceSnapshot, 0, len(entries))
	for _, entry := range entries {
		out = append(out, domaincontextepoch.SourceSnapshotFromEntry(entry))
	}
	return out
}

func maxSequence(entries []domaincontextepoch.SourceEntry) uint64 {
	var maximum uint64
	for _, entry := range entries {
		if entry.Sequence > maximum {
			maximum = entry.Sequence
		}
	}
	return maximum
}

func sameSourceConfiguration(left domaincontextepoch.SourceEntry, right domaincontextepoch.SourceEntry) bool {
	left.Sequence = 0
	right.Sequence = 0
	left.Version = domaincontextepoch.ContractVersion
	right.Version = domaincontextepoch.ContractVersion
	leftData, _ := json.Marshal(left)
	rightData, _ := json.Marshal(right)
	return string(leftData) == string(rightData)
}

func diffSnapshot(previous domaincontextepoch.Snapshot, registry []domaincontextepoch.SourceEntry) ([]domaincontextepoch.ChangeReason, domaincontextepoch.ChangeImpact) {
	previousByID := map[string]domaincontextepoch.SourceSnapshot{}
	for _, source := range previous.Sources {
		previousByID[source.SourceID] = source
	}
	currentByID := map[string]domaincontextepoch.SourceSnapshot{}
	for _, entry := range registry {
		currentByID[entry.SourceID] = domaincontextepoch.SourceSnapshotFromEntry(entry)
	}
	reasons := []domaincontextepoch.ChangeReason{}
	impact := domaincontextepoch.ChangeImpact{}
	ids := map[string]bool{}
	for id := range previousByID {
		ids[id] = true
	}
	for id := range currentByID {
		ids[id] = true
	}
	for id := range ids {
		previousSource, hadPrevious := previousByID[id]
		currentSource, hasCurrent := currentByID[id]
		switch {
		case !hadPrevious:
			reasons = append(reasons, domaincontextepoch.ReasonSourceAdded)
		case !hasCurrent:
			reasons = append(reasons, domaincontextepoch.ReasonSourceRemoved)
		default:
			if previousSource.Digest != currentSource.Digest || previousSource.Kind != currentSource.Kind {
				reasons = append(reasons, domaincontextepoch.ReasonSourceDigestChanged)
			}
			if previousSource.ReferenceDigest != currentSource.ReferenceDigest {
				reasons = append(reasons, domaincontextepoch.ReasonReferenceChanged)
			}
			if previousSource.ActivationState != currentSource.ActivationState || previousSource.ActivationReason != currentSource.ActivationReason {
				reasons = append(reasons, domaincontextepoch.ReasonActivationChanged)
			}
			if previousSource.TokenBudget != currentSource.TokenBudget {
				reasons = append(reasons, domaincontextepoch.ReasonBudgetChanged)
			}
			if previousSource.PromptBoundary != currentSource.PromptBoundary {
				reasons = append(reasons, domaincontextepoch.ReasonBoundaryChanged)
			}
			if previousSource.TrustState != currentSource.TrustState {
				reasons = append(reasons, domaincontextepoch.ReasonTrustChanged)
				if currentSource.TrustState == domaincontextepoch.TrustUnavailable {
					reasons = append(reasons, domaincontextepoch.ReasonSourceUnavailable)
				}
			}
		}
		applySourceImpact(&impact, previousSource, hadPrevious)
		applySourceImpact(&impact, currentSource, hasCurrent)
	}
	return reasons, impact
}

func applySourceImpact(impact *domaincontextepoch.ChangeImpact, source domaincontextepoch.SourceSnapshot, exists bool) {
	if !exists || source.ActivationState != domaincontextepoch.ActivationActive {
		return
	}
	switch source.PromptBoundary {
	case domaincontextepoch.BoundaryStablePrefix:
		impact.StablePrefix = true
	case domaincontextepoch.BoundaryDynamicContext:
		impact.DynamicContext = true
	case domaincontextepoch.BoundaryTurnTail:
		impact.TurnTail = true
	}
}

func unavailable(sourceID string, reason string) domaincontextepoch.UnavailableSource {
	return domaincontextepoch.UnavailableSource{SourceID: strings.TrimSpace(sourceID), Reason: reason}
}
