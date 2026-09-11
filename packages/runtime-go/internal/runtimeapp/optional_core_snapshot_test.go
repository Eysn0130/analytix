package runtimeapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	serverapp "analytix.local/runtime-go/internal/server"
)

func TestRuntimeOrdinaryTerminalResolutionUsesStrictCoreSnapshot(t *testing.T) {
	// The helper completes actual HTTP/provider/tool turns around a protected
	// denial and restarts. All runtime handlers are closed when it returns.
	config, threadID, turnIDs := runRuntimePlanThenProtectedOrdinaryRestartV1(t, nil)
	primary, err := finalauthority.NewAcceptedFinalCASReader(config.ProductionDurableRoot)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := primary.ReadPrimaryThreadSnapshotV1(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	for index, turnID := range turnIDs {
		frozen, contextErr := appturn.FrozenSecurityContextForTurn(initial.Thread, turnID)
		var hasGeneral, hasAccepted bool
		for _, raw := range initial.Thread["turns"].([]any) {
			turn := raw.(map[string]any)
			if turn["id"] == turnID {
				hasGeneral = turn["generalTerminalPublication"] != nil
				hasAccepted = turn["acceptedFinal"] != nil
			}
		}
		if contextErr != nil || (index == 0 && (!hasGeneral || hasAccepted || !domainsecurity.TurnSecurityContextIsGeneral(frozen))) ||
			(index == 1 && (hasGeneral || !hasAccepted || !domainsecurity.TurnSecurityContextIsCaseSensitive(frozen))) {
			t.Fatal("actual fixture lost its pre-case general slot or post-case risk floor")
		}
	}
	store, err := serverapp.NewProductionDurableEventSessionStore(config.ProductionDurableRoot)
	if err != nil {
		t.Fatal(err)
	}
	current, err := primary.ReadPrimaryThreadSnapshotV1(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ThreadFileSHA256 != initial.ThreadFileSHA256 {
		t.Fatal("reopening store changed primary before strict observation")
	}
	before := startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot)
	for index, turnID := range turnIDs {
		resolved, err := appturn.ResolveCommittedGeneralTerminalV1(context.Background(), store, primary, threadID, turnID)
		if index == 1 {
			// An ordinary typed result after the protected request retains the
			// Case risk floor and AcceptedFinal lane; it is not general authority.
			if err == nil {
				t.Fatal("case-lane ordinary result was promoted to general authority")
			}
			continue
		}
		if err != nil || resolved.Commit.ThreadID != threadID || resolved.Commit.TurnID != turnID ||
			!domainsecurity.TurnSecurityContextIsGeneral(resolved.SecurityContext) ||
			!domainsecurity.IsSHA256Hex(resolved.ThreadFileSHA256) || !domainsecurity.IsSHA256Hex(resolved.EventLogSHA256) {
			t.Fatalf("actual independently committed ordinary slot resolution failed: %v", err)
		}
	}
	if startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot) != before {
		t.Fatal("strict Core observation changed durable state")
	}
	journal := filepath.Join(config.ProductionDurableRoot, "threads", threadID, ".events-bundle-v1.tmp")
	if err := os.WriteFile(journal, []byte("R129_SYNTHETIC_UNRESOLVED_EVENT_TRANSACTION"), 0o600); err != nil {
		t.Fatal(err)
	}
	before = startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot)
	if _, err := appturn.ResolveCommittedGeneralTerminalV1(context.Background(), store, primary, threadID, turnIDs[0]); err == nil {
		t.Fatal("pending event transaction entered ordinary slot admission")
	}
	if startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot) != before {
		t.Fatal("ordinary slot resolution recovered an unresolved transaction")
	}
}
