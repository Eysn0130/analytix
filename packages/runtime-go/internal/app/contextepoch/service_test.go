package contextepoch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	contextsourceport "analytix.local/runtime-go/internal/ports/contextsource"
)

type contextSourceReaderStub struct {
	content contextsourceport.Content
	err     error
	calls   int
}

func (stub *contextSourceReaderStub) ReadContextSource(context.Context, string, domaincontextepoch.SourceEntry) (contextsourceport.Content, error) {
	stub.calls++
	return stub.content, stub.err
}

func TestInactiveRegistryReconcileChangesEpochWithoutProviderContext(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	state, err := DefaultState("thread-1", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	entry := domaincontextepoch.SourceEntry{
		SourceID: "memory-a", Kind: "memory", Digest: domaincontextepoch.SHA256Hex([]byte("alpha")),
		TrustState: domaincontextepoch.TrustUntrusted, PromptBoundary: domaincontextepoch.BoundaryDynamicContext,
		TokenBudget: 16, ActivationState: domaincontextepoch.ActivationInactive,
	}
	state, changed, err := Upsert(state, entry, false)
	if err != nil || !changed {
		t.Fatalf("upsert failed: changed=%v err=%v", changed, err)
	}
	result, err := Reconcile(ReconcileInput{State: state, Cause: CauseRequestBoundary, At: now.Add(time.Second)})
	if err != nil || !result.Changed {
		t.Fatalf("reconcile failed: %#v err=%v", result, err)
	}
	if result.State.AcceptedSnapshot.Epoch != 2 || !result.State.AcceptedSnapshot.Impact.DiagnosticsOnly {
		t.Fatalf("inactive registry changes must be diagnostics-only: %#v", result.State.AcceptedSnapshot)
	}
	reader := &contextSourceReaderStub{content: contextsourceport.Content{Text: "alpha"}}
	providerContext, err := BuildProviderContext(context.Background(), result.State, reader)
	if err != nil || reader.calls != 0 || len(providerContext.Dynamic) != 0 || len(providerContext.Unavailable) != 0 {
		t.Fatalf("inactive sources must remain prompt-invisible: context=%#v calls=%d err=%v", providerContext, reader.calls, err)
	}
	removed, didRemove, err := Remove(result.State, "memory-a", false)
	if err != nil || !didRemove {
		t.Fatalf("remove failed: removed=%v err=%v", didRemove, err)
	}
	removedResult, err := Reconcile(ReconcileInput{State: removed, Cause: CauseRequestBoundary, At: now.Add(2 * time.Second)})
	if err != nil || !removedResult.Changed || removedResult.State.AcceptedSnapshot.Epoch != 3 ||
		!containsReason(removedResult.State.AcceptedSnapshot.ChangeReasons, domaincontextepoch.ReasonSourceRemoved) {
		t.Fatalf("source removal was not reconciled: %#v err=%v", removedResult, err)
	}
}

func TestActivatedDynamicContextRequiresDigestAndBudget(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	content := "bounded source content"
	entry := domaincontextepoch.SourceEntry{
		Version: domaincontextepoch.ContractVersion, SourceID: "memory-a", Kind: "memory",
		Reference: "/Users/alice/private/case-source.txt",
		Digest:    domaincontextepoch.SHA256Hex([]byte(content)), Sequence: 1,
		TrustState: domaincontextepoch.TrustUntrusted, PromptBoundary: domaincontextepoch.BoundaryDynamicContext,
		TokenBudget: 16, ActivationState: domaincontextepoch.ActivationActive, ActivationReason: "explicit-turn-selection",
	}
	state, err := BootstrapState("thread-1", 1, []domaincontextepoch.SourceEntry{entry}, now)
	if err != nil {
		t.Fatal(err)
	}
	reader := &contextSourceReaderStub{content: contextsourceport.Content{Text: content, Digest: entry.Digest}}
	providerContext, err := BuildProviderContext(context.Background(), state, reader)
	if err != nil || len(providerContext.Dynamic) != 1 || providerContext.Dynamic[0].Content != content || len(providerContext.Unavailable) != 0 {
		t.Fatalf("valid selected content was not admitted: %#v err=%v", providerContext, err)
	}
	serialized, _ := json.Marshal(providerContext)
	for _, forbidden := range []string{entry.SourceID, entry.Reference, entry.Digest} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("registry metadata must remain prompt-invisible (%q): %s", forbidden, serialized)
		}
	}

	reader.content = contextsourceport.Content{Text: "different content"}
	providerContext, err = BuildProviderContext(context.Background(), state, reader)
	if err != nil || len(providerContext.Dynamic) != 0 || len(providerContext.Unavailable) != 1 || providerContext.Unavailable[0].Reason != "source-digest-mismatch" {
		t.Fatalf("digest mismatch must fail closed: %#v err=%v", providerContext, err)
	}

	entry.Digest = domaincontextepoch.SHA256Hex([]byte(content))
	entry.TokenBudget = 1
	state, err = BootstrapState("thread-1", 1, []domaincontextepoch.SourceEntry{entry}, now)
	if err != nil {
		t.Fatal(err)
	}
	reader.content = contextsourceport.Content{Text: content}
	providerContext, err = BuildProviderContext(context.Background(), state, reader)
	if err != nil || len(providerContext.Dynamic) != 0 || len(providerContext.Unavailable) != 1 || providerContext.Unavailable[0].Reason != "source-token-budget-exceeded" {
		t.Fatalf("over-budget context must fail closed: %#v err=%v", providerContext, err)
	}
}

func TestMidStreamMutationRejectedAndRestartReasonOnlyOnChange(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	state, err := DefaultState("thread-1", 4, now)
	if err != nil {
		t.Fatal(err)
	}
	entry := domaincontextepoch.SourceEntry{
		SourceID: "diagnostics-a", Kind: "diagnostics", Digest: domaincontextepoch.SHA256Hex([]byte("alpha")),
		TrustState: domaincontextepoch.TrustTrusted, PromptBoundary: domaincontextepoch.BoundaryDiagnosticsOnly,
		ActivationState: domaincontextepoch.ActivationInactive,
	}
	if _, changed, err := Upsert(state, entry, true); !errors.Is(err, ErrMidStreamMutation) || changed {
		t.Fatalf("mid-stream mutation must be rejected: changed=%v err=%v", changed, err)
	}
	unchanged, err := Reconcile(ReconcileInput{State: state, Cause: CauseRestart, At: now.Add(time.Second)})
	if err != nil || unchanged.Changed || unchanged.State.AcceptedSnapshot.Epoch != 4 {
		t.Fatalf("unchanged restart must not manufacture an epoch: %#v err=%v", unchanged, err)
	}
	state, _, err = Upsert(state, entry, false)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := Reconcile(ReconcileInput{State: state, Cause: CauseRestart, At: now.Add(2 * time.Second)})
	if err != nil || !changed.Changed || changed.State.AcceptedSnapshot.Epoch != 5 {
		t.Fatalf("changed restart reconcile failed: %#v err=%v", changed, err)
	}
	if !containsReason(changed.State.AcceptedSnapshot.ChangeReasons, domaincontextepoch.ReasonRestartReconcile) {
		t.Fatalf("changed restart must record a sanitized reason: %#v", changed.State.AcceptedSnapshot.ChangeReasons)
	}
}

func TestStablePrefixActivationRequiresEpochBumpAndTrustedSource(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	content := "approved project rule"
	entry := domaincontextepoch.SourceEntry{
		Version: domaincontextepoch.ContractVersion, SourceID: "project-rule-a", Kind: "project-rule",
		Digest: domaincontextepoch.SHA256Hex([]byte(content)), Sequence: 1,
		TrustState: domaincontextepoch.TrustTrusted, PromptBoundary: domaincontextepoch.BoundaryStablePrefix,
		TokenBudget: 16, ActivationState: domaincontextepoch.ActivationInactive,
	}
	state, err := BootstrapState("thread-1", 1, []domaincontextepoch.SourceEntry{entry}, now)
	if err != nil {
		t.Fatal(err)
	}
	entry.ActivationState = domaincontextepoch.ActivationActive
	entry.ActivationReason = "explicit-epoch-bump"
	state, changed, err := Upsert(state, entry, false)
	if err != nil || !changed {
		t.Fatalf("stable source activation failed: changed=%v err=%v", changed, err)
	}
	reconciled, err := Reconcile(ReconcileInput{State: state, Cause: CauseRequestBoundary, At: now.Add(time.Second)})
	if err != nil || !reconciled.Changed || reconciled.State.AcceptedSnapshot.Epoch != 2 || !reconciled.State.AcceptedSnapshot.Impact.StablePrefix {
		t.Fatalf("stable source activation must create an explicit measurable epoch: %#v err=%v", reconciled, err)
	}
	reader := &contextSourceReaderStub{content: contextsourceport.Content{Text: content}}
	providerContext, err := BuildProviderContext(context.Background(), reconciled.State, reader)
	if err != nil || len(providerContext.StablePrefix) != 1 || len(providerContext.Unavailable) != 0 {
		t.Fatalf("trusted stable source was not admitted after epoch bump: %#v err=%v", providerContext, err)
	}
	entry.TrustState = domaincontextepoch.TrustUntrusted
	entry.Sequence = 1
	untrusted, err := BootstrapState("thread-1", 1, []domaincontextepoch.SourceEntry{entry}, now)
	if err != nil {
		t.Fatal(err)
	}
	providerContext, err = BuildProviderContext(context.Background(), untrusted, reader)
	if err != nil || len(providerContext.StablePrefix) != 0 || len(providerContext.Unavailable) != 1 || providerContext.Unavailable[0].Reason != "stable-prefix-requires-trusted-source" {
		t.Fatalf("untrusted stable prefix must fail closed: %#v err=%v", providerContext, err)
	}
}

func TestCompactionRecoveryDigestDoesNotCreateRepeatedLoop(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	state, err := DefaultState("thread-1", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	digest := domaincontextepoch.SHA256Hex([]byte("compacted-items"))
	first, err := Reconcile(ReconcileInput{State: state, Cause: CauseCompaction, RecoveryDigest: digest, At: now.Add(time.Second)})
	if err != nil || !first.Changed || first.State.AcceptedSnapshot.Epoch != 2 {
		t.Fatalf("first compaction reconcile failed: %#v err=%v", first, err)
	}
	second, err := Reconcile(ReconcileInput{State: first.State, Cause: CauseCompaction, RecoveryDigest: digest, At: now.Add(2 * time.Second)})
	if err != nil || second.Changed || second.State.AcceptedSnapshot.Epoch != 2 {
		t.Fatalf("same compaction digest must not loop: %#v err=%v", second, err)
	}
	requestBoundary, err := Reconcile(ReconcileInput{State: first.State, Cause: CauseRequestBoundary, At: now.Add(3 * time.Second)})
	if err != nil || requestBoundary.Changed || requestBoundary.State.AcceptedSnapshot.RecoveryDigest != digest {
		t.Fatalf("normal request reconcile must preserve compact recovery metadata: %#v err=%v", requestBoundary, err)
	}
}

func TestRestartMarksUnreadableActiveSourceUnavailableOnce(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	content := "restart source"
	entry := domaincontextepoch.SourceEntry{
		Version: domaincontextepoch.ContractVersion, SourceID: "memory-a", Kind: "memory",
		Digest: domaincontextepoch.SHA256Hex([]byte(content)), Sequence: 1,
		TrustState: domaincontextepoch.TrustUntrusted, PromptBoundary: domaincontextepoch.BoundaryDynamicContext,
		TokenBudget: 16, ActivationState: domaincontextepoch.ActivationActive, ActivationReason: "explicit-turn-selection",
	}
	state, err := BootstrapState("thread-1", 2, []domaincontextepoch.SourceEntry{entry}, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Recover(context.Background(), state, nil, now.Add(time.Second))
	if err != nil || !first.Changed || first.State.AcceptedSnapshot.Epoch != 3 {
		t.Fatalf("unreadable restart source must advance a fail-closed epoch: %#v err=%v", first, err)
	}
	if first.State.Registry[0].TrustState != domaincontextepoch.TrustUnavailable ||
		!containsReason(first.State.AcceptedSnapshot.ChangeReasons, domaincontextepoch.ReasonSourceUnavailable) ||
		!containsReason(first.State.AcceptedSnapshot.ChangeReasons, domaincontextepoch.ReasonRestartReconcile) {
		t.Fatalf("restart unavailable state/reasons missing: %#v", first.State)
	}
	second, err := Recover(context.Background(), first.State, nil, now.Add(2*time.Second))
	if err != nil || second.Changed || second.State.AcceptedSnapshot.Epoch != 3 {
		t.Fatalf("already unavailable restart source must not loop: %#v err=%v", second, err)
	}
}

func containsReason(reasons []domaincontextepoch.ChangeReason, expected domaincontextepoch.ChangeReason) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}
