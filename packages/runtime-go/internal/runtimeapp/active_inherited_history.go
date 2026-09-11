package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"

	finaladapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	"analytix.local/runtime-go/internal/jobs"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

func runtimeActiveHistoryIdentityValidatorV1(dataDir string, pending *pendingapp.Service, finals *finaladapter.PrivateStore, authority authorityport.Authority) func(context.Context, string, []string) error {
	return func(ctx context.Context, threadID string, turnIDs []string) error {
		ids := map[string]bool{}
		for _, id := range turnIDs {
			ids[id] = true
		}
		inventory, err := pending.TrustedInventoryV1(ctx)
		if err != nil {
			return err
		}
		for _, receipt := range inventory.Receipts {
			if receipt.Context.ThreadID == threadID && ids[receipt.Context.TurnID] {
				return errors.New("inherited history has a pending execution identity")
			}
			if receipt.ChildProducer != nil {
				for _, child := range receipt.ChildProducer.Children {
					if child.ChildThreadID == threadID && ids[child.ChildTurnID] {
						return errors.New("inherited history has a reserved child execution identity")
					}
				}
			}
		}
		runs, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(dataDir, "child-runs"))
		if err != nil {
			return err
		}
		if runs.HasLegacyTypeScript {
			return errors.New("active history child identity inventory is incomplete")
		}
		for _, run := range runs.Records {
			if (run.ParentThreadID == threadID && (ids[run.ParentTurnID] || ids[run.AutoContinueTurnID])) || (run.ChildThreadID == threadID && ids[run.ChildTurnID]) {
				return errors.New("inherited history has a child job execution identity")
			}
		}
		records, err := finals.List(ctx)
		if err != nil {
			return err
		}
		for _, record := range records {
			keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record.AcceptedFinal)
			if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(record.AcceptedFinal), signature) != nil {
				return errors.New("active history final identity inventory is not trusted")
			}
			if record.SecurityContext.ThreadID == threadID && ids[record.SecurityContext.TurnID] {
				return errors.New("inherited history has an accepted final identity")
			}
		}
		return ctx.Err()
	}
}
