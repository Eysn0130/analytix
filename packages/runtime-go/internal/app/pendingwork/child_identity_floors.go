package pendingwork

import (
	"errors"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

var ErrChildProducerInventoryIncomplete = errors.New("Core child producer inventory lacks exact trusted association")

// DeriveChildIdentityFloorsV1 consumes the complete already-verified signed
// inventory and strict actual primary/job records. Disposition, expiry, job
// existence, and status never filter a signed allocation out of the floor.
// Missing jobs remain absent; this function has no writer or allocator.
func DeriveChildIdentityFloorsV1(inventory TrustedInventoryV1, records []domainjob.Record, primaryThreads []map[string]any) (domainpendingwork.ChildIdentityFloorsV1, error) {
	var floors domainpendingwork.ChildIdentityFloorsV1
	type association struct {
		target  domainpendingwork.ChildProducerTargetV1
		receipt domainpendingwork.PendingWorkReceiptV1
	}
	byJob := map[string]association{}
	byTurn := map[string]string{}
	observe := func(jobID, threadID, turnID string) error {
		if err := floors.Observe(jobID, threadID, turnID); err != nil {
			return err
		}
		return nil
	}
	for _, receipt := range inventory.Receipts {
		if err := observe("", receipt.Context.ThreadID, receipt.Context.TurnID); err != nil {
			return domainpendingwork.ChildIdentityFloorsV1{}, err
		}
		if receipt.ChildProducer == nil {
			continue
		}
		if receipt.Kind != domainpendingwork.KindSideEffectIntent || domainpendingwork.ValidateChildProducerV1(receipt.ChildProducer) != nil {
			return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
		}
		for _, target := range receipt.ChildProducer.Children {
			if _, exists := byJob[target.JobID]; exists || byTurn[target.ChildTurnID] != "" || target.ChildThreadID == receipt.Context.ThreadID || target.ChildTurnID == receipt.Context.TurnID {
				return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
			}
			byJob[target.JobID] = association{target, receipt}
			byTurn[target.ChildTurnID] = target.ChildThreadID
			if err := observe(target.JobID, target.ChildThreadID, target.ChildTurnID); err != nil {
				return domainpendingwork.ChildIdentityFloorsV1{}, err
			}
		}
	}
	seenJobs := map[string]bool{}
	for _, record := range records {
		if record.ID == "" || seenJobs[record.ID] {
			return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
		}
		seenJobs[record.ID] = true
		if err := observe(record.ID, record.ChildThreadID, record.ChildTurnID); err != nil {
			return domainpendingwork.ChildIdentityFloorsV1{}, err
		}
		if err := observe("", record.ParentThreadID, record.ParentTurnID); err != nil {
			return domainpendingwork.ChildIdentityFloorsV1{}, err
		}
		if err := observe("", "", record.AutoContinueTurnID); err != nil {
			return domainpendingwork.ChildIdentityFloorsV1{}, err
		}
		bound, signed := byJob[record.ID]
		if strings.TrimSpace(record.Kind) != "subagent" {
			if signed {
				return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
			}
			continue
		}
		// Legacy records retain audit validity, but cannot supply missing child
		// producer authority to a startup that will run recovery consumers.
		binding := record.SecurityBinding
		if !signed || domainjob.ValidateSecurityBinding(binding) != nil ||
			bound.target.ChildThreadID != record.ChildThreadID || bound.target.ChildTurnID != record.ChildTurnID ||
			bound.receipt.ChildProducer.ParentBindingDigest != binding.BindingDigest ||
			record.ParentThreadID != binding.ParentThreadID || record.ParentTurnID != binding.ParentTurnID || record.ParentToolCallID != binding.ParentToolCallID ||
			bound.receipt.Context.ThreadID != binding.ParentThreadID || bound.receipt.Context.TurnID != binding.ParentTurnID ||
			bound.receipt.Context.ContextDigest != binding.ParentContextDigest || bound.receipt.Context.ContextEpoch != binding.ParentContextEpoch ||
			bound.receipt.Context.CaseBindingHash != binding.ParentCaseBindingHash || bound.receipt.Context.DatasetSnapshotID != binding.ParentDatasetSnapshot ||
			bound.receipt.Context.SourceManifestHash != binding.ParentSourceManifest || len(bound.receipt.GrantMembers) != 1 ||
			bound.receipt.GrantMembers[0].GrantID != binding.ParentExecutionGrantID {
			return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
		}
		ordinal := record.ParallelIndex
		if ordinal == 0 {
			ordinal = 1
		}
		if ordinal < 1 || uint64(ordinal) != uint64(bound.target.Ordinal) {
			return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
		}
	}
	seenThreads := map[string]bool{}
	for _, thread := range primaryThreads {
		id, _ := thread["id"].(string)
		if domainthread.ValidatePrimaryIdentityV1(id, thread) != nil || seenThreads[id] {
			return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
		}
		seenThreads[id] = true
		if err := observe("", id, ""); err != nil {
			return domainpendingwork.ChildIdentityFloorsV1{}, err
		}
		for _, value := range thread["turns"].([]any) {
			turn := value.(map[string]any)
			turnID := turn["id"].(string)
			// Fork/resume history retains IDs but strips frozen authority. Such
			// copies raise floors without claiming another execution slot.
			_, carriesFrozenAuthority := turn["securityContext"]
			if targetThread := byTurn[turnID]; targetThread != "" && targetThread != id && carriesFrozenAuthority {
				return domainpendingwork.ChildIdentityFloorsV1{}, ErrChildProducerInventoryIncomplete
			}
			if err := observe("", "", turnID); err != nil {
				return domainpendingwork.ChildIdentityFloorsV1{}, err
			}
		}
	}
	return floors, nil
}
