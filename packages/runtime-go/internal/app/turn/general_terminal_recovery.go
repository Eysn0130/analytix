package turn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

type GeneralTerminalPublicationRecoveryPlanV1 struct {
	ThreadID     string                                     `json:"threadId"`
	ThreadDigest string                                     `json:"threadDigest"`
	EventsDigest string                                     `json:"eventsDigest"`
	RepairTurns  []GeneralTerminalPublicationRecoveryTurnV1 `json:"repairTurns"`
}

type GeneralTerminalPublicationRecoveryTurnV1 struct {
	TurnID       string `json:"turnId"`
	CommitDigest string `json:"commitDigest"`
}

type generalTerminalStartupInspectionV1 struct {
	ThreadID string
	Entries  []GeneralTerminalPublicationInventoryEntryV1
	Repairs  []GeneralTerminalPublicationRecoveryTurnV1
}

// RecoverGeneralTerminalPublicationsAtStartupV1 validates the complete
// durable inventory once under one exclusive transaction. It retains only the
// small terminal-publication inventory, performs no recovery write until every
// thread has passed strict replay, and rereads only threads that actually need
// an outbox repair. The public two-phase recovery API remains available for
// explicit stale-plan workflows.
func RecoverGeneralTerminalPublicationsAtStartupV1(ctx context.Context, store recoveryport.Store) error {
	if ctx == nil || store == nil {
		return errors.New("general terminal recovery store is unavailable")
	}
	return store.WithGeneralTerminalRecoveryExclusiveV1(ctx, func(tx recoveryport.TransactionV1) error {
		if tx == nil {
			return errors.New("general terminal recovery transaction is unavailable")
		}
		threadIDs, err := tx.ThreadIDs()
		if err != nil {
			return err
		}
		if err := validateGeneralTerminalRecoveryThreadInventoryV1(threadIDs); err != nil {
			return err
		}
		preserved, _ := tx.(recoveryport.StartupPreservationV1)
		revalidatePreserved := func() error {
			if preserved != nil {
				return preserved.RevalidateStartupPreservationV1(ctx)
			}
			return ctx.Err()
		}
		if err := revalidatePreserved(); err != nil {
			return err
		}

		inspections := make([]generalTerminalStartupInspectionV1, 0, len(threadIDs))
		for _, threadID := range threadIDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if preserved != nil && preserved.RestartPreservesThreadV1(threadID) {
				continue
			}
			thread, err := tx.ReadCanonicalThread(threadID)
			if err != nil || thread == nil {
				return errors.Join(err, errors.New("general terminal recovery cannot read a canonical thread"))
			}
			threadView, err := tx.PublicationThreadView(threadID, thread)
			if err != nil {
				return err
			}
			replay, err := tx.LoadEvents(threadID)
			if err != nil || !replay.Replayable {
				return errors.Join(err, errors.New("general terminal recovery event inventory is not replayable"))
			}
			entries, err := PreflightGeneralTerminalPublicationInventoryV1(threadView, replay.Events)
			if err != nil {
				return err
			}
			inspections = append(inspections, generalTerminalStartupInspectionV1{
				ThreadID: threadID,
				Entries:  entries,
				Repairs:  missingGeneralTerminalRecoveryTurnsV1(entries),
			})
		}

		if err := revalidatePreserved(); err != nil {
			return err
		}
		for _, inspection := range inspections {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries := inspection.Entries
			if len(inspection.Repairs) > 0 {
				for _, repair := range inspection.Repairs {
					if err := ctx.Err(); err != nil {
						return err
					}
					if err := tx.RecordTerminalBundle(inspection.ThreadID, repair.TurnID); err != nil {
						return err
					}
				}
				thread, err := tx.ReadCanonicalThread(inspection.ThreadID)
				if err != nil || thread == nil {
					return errors.Join(err, errors.New("general terminal recovery lost its canonical thread"))
				}
				threadView, err := tx.PublicationThreadView(inspection.ThreadID, thread)
				if err != nil {
					return err
				}
				replay, err := tx.LoadEvents(inspection.ThreadID)
				if err != nil || !replay.Replayable {
					return errors.Join(err, errors.New("general terminal recovery readback is not replayable"))
				}
				entries, err = PreflightGeneralTerminalPublicationInventoryV1(threadView, replay.Events)
				if err != nil {
					return err
				}
			}
			usageEvents := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				if entry.State != GeneralTerminalPublicationCompleteV1 {
					return errors.New("general terminal recovery left an unsettled publication bundle")
				}
				if usage := GeneralTerminalUsageEventV1(entry.Events); usage != nil {
					usageEvents = append(usageEvents, usage)
				}
			}
			if len(usageEvents) != 0 {
				if err := tx.SettleTerminalUsageBatch(usageEvents); err != nil {
					return err
				}
			}
		}
		return revalidatePreserved()
	})
}

// PreflightGeneralTerminalPublicationRecoveryV1 validates the complete
// durable inventory under one read-only exclusive transaction. It never
// repairs a thread while a different thread remains unverified.
func PreflightGeneralTerminalPublicationRecoveryV1(
	ctx context.Context,
	store recoveryport.Store,
) ([]GeneralTerminalPublicationRecoveryPlanV1, error) {
	if ctx == nil || store == nil {
		return nil, errors.New("general terminal recovery store is unavailable")
	}
	plans := []GeneralTerminalPublicationRecoveryPlanV1{}
	err := store.WithGeneralTerminalRecoveryExclusiveV1(ctx, func(tx recoveryport.TransactionV1) error {
		var err error
		plans, err = preflightGeneralTerminalRecoveryTransactionV1(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return plans, nil
}

func preflightGeneralTerminalRecoveryTransactionV1(
	ctx context.Context,
	tx recoveryport.TransactionV1,
) ([]GeneralTerminalPublicationRecoveryPlanV1, error) {
	if tx == nil {
		return nil, errors.New("general terminal recovery transaction is unavailable")
	}
	threadIDs, err := tx.ThreadIDs()
	if err != nil {
		return nil, err
	}
	if err := validateGeneralTerminalRecoveryThreadInventoryV1(threadIDs); err != nil {
		return nil, err
	}
	plans := make([]GeneralTerminalPublicationRecoveryPlanV1, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		thread, err := tx.ReadCanonicalThread(threadID)
		if err != nil || thread == nil {
			return nil, errors.Join(err, errors.New("general terminal recovery cannot read a canonical thread"))
		}
		threadView, err := tx.PublicationThreadView(threadID, thread)
		if err != nil {
			return nil, err
		}
		replay, err := tx.LoadEvents(threadID)
		if err != nil || !replay.Replayable {
			return nil, errors.Join(err, errors.New("general terminal recovery event inventory is not replayable"))
		}
		entries, err := PreflightGeneralTerminalPublicationInventoryV1(threadView, replay.Events)
		if err != nil {
			return nil, err
		}
		plans = append(plans, GeneralTerminalPublicationRecoveryPlanV1{
			ThreadID: threadID, ThreadDigest: generalTerminalRecoveryDigestV1(thread),
			EventsDigest: generalTerminalRecoveryReplayDigestV1(replay), RepairTurns: missingGeneralTerminalRecoveryTurnsV1(entries),
		})
	}
	return plans, nil
}

// ApplyGeneralTerminalPublicationRecoveryV1 revalidates every plan under one
// exclusive transaction before the first write, then verifies every repaired
// bundle by readback. Baseline drift rejects the whole batch without mutation.
func ApplyGeneralTerminalPublicationRecoveryV1(
	ctx context.Context,
	store recoveryport.Store,
	plans []GeneralTerminalPublicationRecoveryPlanV1,
) error {
	if ctx == nil || store == nil {
		return errors.New("general terminal recovery store is unavailable")
	}
	return store.WithGeneralTerminalRecoveryExclusiveV1(ctx, func(tx recoveryport.TransactionV1) error {
		return applyGeneralTerminalRecoveryTransactionV1(ctx, tx, plans)
	})
}

func applyGeneralTerminalRecoveryTransactionV1(
	ctx context.Context,
	tx recoveryport.TransactionV1,
	plans []GeneralTerminalPublicationRecoveryPlanV1,
) error {
	if tx == nil {
		return errors.New("general terminal recovery transaction is unavailable")
	}
	threadIDs, err := tx.ThreadIDs()
	if err != nil {
		return err
	}
	if err := validateGeneralTerminalRecoveryThreadInventoryV1(threadIDs); err != nil {
		return err
	}
	if len(threadIDs) != len(plans) {
		return errors.New("general terminal recovery thread inventory changed")
	}
	for index, threadID := range threadIDs {
		if plans[index].ThreadID != threadID {
			return errors.New("general terminal recovery thread inventory changed")
		}
	}
	seen := map[string]bool{}
	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			return err
		}
		if strings.TrimSpace(plan.ThreadID) == "" || seen[plan.ThreadID] || plan.RepairTurns == nil ||
			contracts.SafeRecordID(plan.ThreadID) != plan.ThreadID ||
			!domainsecurity.IsSHA256Hex(plan.ThreadDigest) || !domainsecurity.IsSHA256Hex(plan.EventsDigest) {
			return errors.New("general terminal recovery plan is invalid")
		}
		seen[plan.ThreadID] = true
		repairSeen := map[string]bool{}
		for _, repair := range plan.RepairTurns {
			if strings.TrimSpace(repair.TurnID) == "" || repairSeen[repair.TurnID] ||
				!domainsecurity.IsSHA256Hex(repair.CommitDigest) {
				return errors.New("general terminal recovery repair turn is invalid")
			}
			repairSeen[repair.TurnID] = true
		}
		thread, err := tx.ReadCanonicalThread(plan.ThreadID)
		if err != nil || thread == nil || generalTerminalRecoveryDigestV1(thread) != plan.ThreadDigest {
			return errors.Join(err, errors.New("general terminal recovery thread baseline changed"))
		}
		threadView, err := tx.PublicationThreadView(plan.ThreadID, thread)
		if err != nil {
			return err
		}
		replay, err := tx.LoadEvents(plan.ThreadID)
		if err != nil || !replay.Replayable || generalTerminalRecoveryReplayDigestV1(replay) != plan.EventsDigest {
			return errors.Join(err, errors.New("general terminal recovery event baseline changed"))
		}
		entries, err := PreflightGeneralTerminalPublicationInventoryV1(threadView, replay.Events)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(missingGeneralTerminalRecoveryTurnsV1(entries), plan.RepairTurns) {
			return errors.New("general terminal recovery canonical winner changed")
		}
	}

	for _, plan := range plans {
		for _, repair := range plan.RepairTurns {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := tx.RecordTerminalBundle(plan.ThreadID, repair.TurnID); err != nil {
				return err
			}
		}
		thread, err := tx.ReadCanonicalThread(plan.ThreadID)
		if err != nil || thread == nil {
			return errors.Join(err, errors.New("general terminal recovery lost its canonical thread"))
		}
		threadView, err := tx.PublicationThreadView(plan.ThreadID, thread)
		if err != nil {
			return err
		}
		replay, err := tx.LoadEvents(plan.ThreadID)
		if err != nil || !replay.Replayable {
			return errors.Join(err, errors.New("general terminal recovery readback is not replayable"))
		}
		entries, err := PreflightGeneralTerminalPublicationInventoryV1(threadView, replay.Events)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.State != GeneralTerminalPublicationCompleteV1 {
				return errors.New("general terminal recovery left an unsettled publication bundle")
			}
			if usage := GeneralTerminalUsageEventV1(entry.Events); usage != nil {
				if err := tx.SettleTerminalUsage(usage); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func generalTerminalRecoveryReplayDigestV1(replay recoveryport.ReplaySnapshotV1) string {
	if domainsecurity.IsSHA256Hex(strings.TrimSpace(replay.EventLogSHA256)) {
		return strings.TrimSpace(replay.EventLogSHA256)
	}
	return generalTerminalRecoveryDigestV1(replay.Events)
}

func missingGeneralTerminalRecoveryTurnsV1(entries []GeneralTerminalPublicationInventoryEntryV1) []GeneralTerminalPublicationRecoveryTurnV1 {
	repairs := make([]GeneralTerminalPublicationRecoveryTurnV1, 0)
	for _, entry := range entries {
		if entry.State == GeneralTerminalPublicationMissingV1 {
			repairs = append(repairs, GeneralTerminalPublicationRecoveryTurnV1{
				TurnID: entry.TurnID, CommitDigest: entry.Commit.CommitDigest,
			})
		}
	}
	return repairs
}

func validateGeneralTerminalRecoveryThreadInventoryV1(threadIDs []string) error {
	seen := map[string]bool{}
	for _, threadID := range threadIDs {
		if strings.TrimSpace(threadID) == "" || contracts.SafeRecordID(threadID) != threadID || seen[threadID] {
			return errors.New("general terminal recovery thread inventory is invalid")
		}
		seen[threadID] = true
	}
	if !sort.StringsAreSorted(threadIDs) {
		return errors.New("general terminal recovery thread inventory is not canonical")
	}
	return nil
}

func GeneralTerminalUsageEventV1(events []map[string]any) map[string]any {
	for _, event := range events {
		if stringField(event, "generalTerminalSlot") == "usage" && stringField(event, "kind") == "usage" {
			return event
		}
	}
	return nil
}

func generalTerminalRecoveryDigestV1(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
