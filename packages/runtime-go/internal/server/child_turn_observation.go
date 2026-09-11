package server

import (
	"context"
	"errors"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// observeRuntimeChildFirstTurnV1 separates an exact committed first turn from
// an absent, signed reservation. Read errors and missing frozen authority are
// unavailable, never evidence of absence. This observation grants no execution
// or retry authority; pending steering must also hold the live start barrier.
func (h *runtimeServerHandler) observeRuntimeChildFirstTurnV1(ctx context.Context, record domainjob.Record) (domainsecurity.TurnSecurityContext, bool, error) {
	unavailable := func() (domainsecurity.TurnSecurityContext, bool, error) {
		return domainsecurity.TurnSecurityContext{}, false, errors.New("child first-turn authority is unavailable")
	}
	if h == nil || h.store == nil || ctx == nil || ctx.Err() != nil ||
		domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil || record.ChildTurnID == "" ||
		record.ParentThreadID != record.SecurityBinding.ParentThreadID || record.ParentTurnID != record.SecurityBinding.ParentTurnID {
		return unavailable()
	}
	reader, err := finalauthorityadapter.NewAcceptedFinalCASReader(h.store.root)
	if err != nil {
		return unavailable()
	}
	snapshot, err := reader.ReadPrimaryThreadSnapshotV1(ctx, record.ChildThreadID)
	if err != nil {
		return unavailable()
	}
	for _, value := range snapshot.Thread["turns"].([]any) {
		if value.(map[string]any)["id"] != record.ChildTurnID {
			continue
		}
		frozen, err := appturn.FrozenSecurityContextForTurn(snapshot.Thread, record.ChildTurnID)
		if err != nil || frozen.ThreadID != record.ChildThreadID || frozen.TurnID != record.ChildTurnID ||
			!domainjob.SecurityBindingMatchesWorkspaceScope(record.SecurityBinding, frozen) {
			return unavailable()
		}
		return frozen, true, nil
	}
	if h.pendingWork == nil {
		return unavailable()
	}
	inventory, err := h.pendingWork.TrustedInventoryV1(ctx)
	if err != nil {
		return unavailable()
	}
	jobs, turns := make(map[string]bool), make(map[string]bool)
	found := false
	for _, receipt := range inventory.Receipts {
		producer := receipt.ChildProducer
		if producer == nil {
			continue
		}
		// Include closed, expired and missing-job reservations. None can donate
		// its identities to another producer or make a missing witness valid.
		for _, child := range producer.Children {
			if jobs[child.JobID] || turns[child.ChildTurnID] {
				return unavailable()
			}
			jobs[child.JobID], turns[child.ChildTurnID] = true, true
			if child.JobID != record.ID {
				continue
			}
			binding := record.SecurityBinding
			if child.ChildThreadID != record.ChildThreadID || child.ChildTurnID != record.ChildTurnID ||
				producer.ParentBindingDigest != binding.BindingDigest || receipt.Context.ThreadID != binding.ParentThreadID ||
				receipt.Context.TurnID != binding.ParentTurnID || receipt.Context.ContextDigest != binding.ParentContextDigest ||
				receipt.Context.ContextEpoch != binding.ParentContextEpoch || receipt.Context.CaseBindingHash != binding.ParentCaseBindingHash ||
				receipt.Context.DatasetSnapshotID != binding.ParentDatasetSnapshot || receipt.Context.SourceManifestHash != binding.ParentSourceManifest ||
				len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != binding.ParentExecutionGrantID {
				return unavailable()
			}
			found = true
		}
	}
	if !found || ctx.Err() != nil {
		return unavailable()
	}
	return domainsecurity.TurnSecurityContext{}, false, nil
}
