package turn

import (
	"context"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

type CommittedGeneralTerminalResolutionV1 struct {
	SecurityContext  domainsecurity.TurnSecurityContext
	Commit           domainturnterminal.GeneralTerminalPublicationCommitV1
	ThreadFileSHA256 string
	EventLogSHA256   string
}

// ResolveCommittedGeneralTerminalV1 resolves one exact ordinary terminal from
// a single exclusive durable-store snapshot. The returned commit is audit
// metadata only; it never grants evidence or case-fact publication authority.
func ResolveCommittedGeneralTerminalV1(
	ctx context.Context,
	store recoveryport.Store,
	primary recoveryport.PrimaryThreadReaderV1,
	threadID string,
	turnID string,
) (CommittedGeneralTerminalResolutionV1, error) {
	if ctx == nil || store == nil || primary == nil ||
		!domainthread.IsCanonicalRecordID(threadID) || !domainthread.IsCanonicalRecordID(turnID) {
		return CommittedGeneralTerminalResolutionV1{},
			errors.New("general terminal resolution identity is invalid")
	}
	var result CommittedGeneralTerminalResolutionV1
	err := store.WithGeneralTerminalRecoveryExclusiveV1(ctx, func(tx recoveryport.TransactionV1) error {
		if tx == nil {
			return errors.New("general terminal resolution transaction is unavailable")
		}
		// The store lock binds the strict primary and event observations. The
		// normal read/view APIs may normalize or hydrate sidecars and therefore
		// cannot establish independent ordinary-slot admission.
		snapshot, err := primary.ReadPrimaryThreadSnapshotV1(ctx, threadID)
		if err != nil {
			return err
		}
		if snapshot.ThreadID != threadID || !domainsecurity.IsSHA256Hex(snapshot.ThreadFileSHA256) {
			return errors.New("general terminal primary snapshot binding is invalid")
		}
		thread := snapshot.Thread
		if err := domainthread.ValidatePrimaryIdentityV1(threadID, thread); err != nil {
			return err
		}
		eventDigest, err := primary.ReadCommittedEventLogSHA256V1(ctx, threadID)
		if err != nil {
			return err
		}
		replay, err := tx.ObserveEvents(ctx, threadID)
		if err != nil || !replay.Replayable || !domainsecurity.IsSHA256Hex(replay.EventLogSHA256) || eventDigest != replay.EventLogSHA256 {
			return errors.Join(err, errors.New("general terminal resolution event log is not replayable"))
		}
		entries, err := PreflightGeneralTerminalPublicationInventoryV1(thread, replay.Events)
		if err != nil {
			return err
		}
		entry, found := GeneralTerminalPublicationEntryForTurnV1(entries, turnID)
		if !found || entry.ArchivedOnly || entry.State != GeneralTerminalPublicationCompleteV1 {
			return errors.New("general terminal resolution requires a complete committed bundle")
		}
		resolved, err := FrozenSecurityContextForTurn(thread, turnID)
		if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(resolved) != nil ||
			!domainsecurity.TurnSecurityContextIsGeneral(resolved) ||
			entry.Commit.ContextDigest != resolved.ContextDigest || entry.Commit.ContextEpoch != resolved.ContextEpoch ||
			entry.Commit.DatasetSnapshotID != resolved.DatasetSnapshotID {
			return errors.Join(err, errors.New("general terminal resolution context is invalid"))
		}
		current, err := primary.ReadPrimaryThreadSnapshotV1(ctx, threadID)
		if err != nil {
			return err
		}
		if current.ThreadID != threadID || current.ThreadFileSHA256 != snapshot.ThreadFileSHA256 {
			return errors.New("general terminal primary snapshot changed during observation")
		}
		currentEventDigest, err := primary.ReadCommittedEventLogSHA256V1(ctx, threadID)
		if err != nil {
			return err
		}
		if currentEventDigest != eventDigest {
			return errors.New("general terminal event snapshot changed during observation")
		}
		result = CommittedGeneralTerminalResolutionV1{
			SecurityContext: resolved, Commit: entry.Commit,
			ThreadFileSHA256: snapshot.ThreadFileSHA256, EventLogSHA256: replay.EventLogSHA256,
		}
		return ctx.Err()
	})
	if err != nil {
		return CommittedGeneralTerminalResolutionV1{}, err
	}
	return result, nil
}
