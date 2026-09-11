package usage

import (
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPrefixBaselineNeverComparesAcrossCaseSnapshotEpochOrProviderNamespace(t *testing.T) {
	baseContext := prefixBaselineContext(t, "thread-1", "turn-1", "case-a", "snapshot-a", 3)
	shape := domainmodel.PrefixShape{
		PrefixHash: "prefix-a", Provider: "deepseek", ProviderID: "deepseek",
		EndpointFormat: "chat_completions", Model: "deepseek-chat", Route: "chat_completions",
	}
	base, ok := NewPrefixBaseline(baseContext, shape)
	if !ok {
		t.Fatal("base cache prefix baseline was rejected")
	}

	for name, context := range map[string]domainsecurity.TurnSecurityContext{
		"case":     prefixBaselineContext(t, "thread-1", "turn-2", "case-b", "snapshot-b", 4),
		"snapshot": prefixBaselineContext(t, "thread-1", "turn-2", "case-a", "snapshot-b", 4),
		"epoch":    prefixBaselineContext(t, "thread-1", "turn-2", "case-a", "snapshot-a", 4),
	} {
		t.Run(name, func(t *testing.T) {
			current, ok := NewPrefixBaseline(context, shape)
			if !ok || ComparablePrefix(base, current).PrefixHash != "" {
				t.Fatalf("incompatible %s scope reused the old baseline: %#v", name, current)
			}
		})
	}

	otherProvider := shape
	otherProvider.Model = "deepseek-reasoner"
	current, ok := NewPrefixBaseline(baseContext, otherProvider)
	if !ok || ComparablePrefix(base, current).PrefixHash != "" {
		t.Fatalf("provider namespace change reused the old baseline: %#v", current)
	}
}

func TestPrefixBaselineContinuesAcrossTurnsOnlyWithinExactScope(t *testing.T) {
	first := prefixBaselineContext(t, "thread-1", "turn-1", "case-a", "snapshot-a", 3)
	second := prefixBaselineContext(t, "thread-1", "turn-2", "case-a", "snapshot-a", 3)
	shape := domainmodel.PrefixShape{PrefixHash: "prefix-a", Provider: "deepseek", ProviderID: "deepseek", EndpointFormat: "chat_completions", Model: "deepseek-chat"}
	previous, previousOK := NewPrefixBaseline(first, shape)
	current, currentOK := NewPrefixBaseline(second, shape)
	if !previousOK || !currentOK || ComparablePrefix(previous, current).PrefixHash != "prefix-a" || previous.ContinuityDigest != current.ContinuityDigest {
		t.Fatalf("compatible adjacent turns did not share a baseline: previous=%#v current=%#v", previous, current)
	}
	if diagnostics := PrefixBaselineDiagnostics(current); diagnostics["cacheBaselineSchema"] != PrefixBaselineSchemaV1 || diagnostics["cacheContinuityDigest"] == "" {
		t.Fatalf("cache baseline diagnostics are incomplete: %#v", diagnostics)
	}
}

func TestPrefixBaselineRejectsZeroProviderObservation(t *testing.T) {
	context := prefixBaselineContext(t, "thread-1", "turn-1", "case-a", "snapshot-a", 3)
	if baseline, ok := NewPrefixBaseline(context, domainmodel.PrefixShape{}); ok || baseline.SchemaVersion != "" || baseline.Shape.PrefixHash != "" {
		t.Fatalf("zero provider observation advanced the cache baseline: %#v", baseline)
	}
}

func prefixBaselineContext(t *testing.T, threadID, turnID, caseID, snapshotID string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	workspace := "/workspace/analytix"
	context, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: caseID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte(caseID + "-binding")), DatasetSnapshotID: securitytest.DatasetSnapshotID(snapshotID),
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: epoch,
		IssuedAt: time.Date(2026, 7, 13, 12, 0, int(epoch), 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("build cache baseline security context: %v", err)
	}
	return context
}
