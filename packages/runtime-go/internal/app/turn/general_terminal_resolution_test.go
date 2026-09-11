package turn

import (
	"context"
	"errors"
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

func TestCommittedGeneralTerminalResolutionRejectsMalformedPrimaryShape(t *testing.T) {
	thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1([]domainturnterminal.GeneralTerminalPublicationCommitV1{commit})
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive)
	events, err := PrepareGeneralTerminalEventBundleV1(thread, commit.TurnID, commit, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"healthy", nil},
		{"non-object sibling turn", func(thread map[string]any) { thread["turns"] = append(thread["turns"].([]any), nil) }},
		{"non-canonical thread identity", func(thread map[string]any) { thread["id"] = " " + commit.ThreadID }},
		{"non-canonical turn identity", func(thread map[string]any) { publicationFixtureTurn(thread)["id"] = " " + commit.TurnID }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := contracts.CloneMap(thread)
			if test.mutate != nil {
				test.mutate(candidate)
			}
			tx := &generalTerminalRecoveryTransactionStub{
				threads: map[string]map[string]any{commit.ThreadID: candidate},
				replays: map[string]recoveryport.ReplaySnapshotV1{commit.ThreadID: {
					Events: events, Replayable: true, EventLogSHA256: domainsecurity.SHA256Hex([]byte("synthetic exact event log")),
				}},
			}
			store := &generalTerminalRecoveryStoreStub{tx: tx}
			primary := generalTerminalPrimaryReaderFunc(func(context.Context, string) (recoveryport.PrimaryThreadSnapshotV1, error) {
				if !store.exclusive {
					return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("primary read escaped exclusive snapshot")
				}
				return recoveryport.PrimaryThreadSnapshotV1{ThreadID: commit.ThreadID, ThreadFileSHA256: domainsecurity.SHA256Hex([]byte("synthetic primary")), Thread: candidate}, nil
			})
			// Any accidental normal read returns no thread. Only the independent
			// primary reader above can provide this candidate.
			tx.threads = nil
			resolved, err := ResolveCommittedGeneralTerminalV1(context.Background(), store, primary, commit.ThreadID, commit.TurnID)
			if test.mutate == nil {
				if err != nil || resolved.Commit.CommitDigest != commit.CommitDigest || !domainsecurity.IsSHA256Hex(resolved.ThreadFileSHA256) || !domainsecurity.IsSHA256Hex(resolved.EventLogSHA256) {
					t.Fatalf("healthy exact terminal: %v", err)
				}
			} else if err == nil {
				t.Fatal("malformed primary inventory admitted a committed ordinary slot")
			}
			if tx.recordCalls != 0 || tx.settlements != 0 {
				t.Fatal("read-only terminal resolution performed recovery effects")
			}
		})
	}
}

type generalTerminalPrimaryReaderFunc func(context.Context, string) (recoveryport.PrimaryThreadSnapshotV1, error)

func (read generalTerminalPrimaryReaderFunc) ReadPrimaryThreadSnapshotV1(ctx context.Context, threadID string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	return read(ctx, threadID)
}

func (read generalTerminalPrimaryReaderFunc) ReadCommittedEventLogSHA256V1(ctx context.Context, threadID string) (string, error) {
	return domainsecurity.SHA256Hex([]byte("synthetic exact event log")), ctx.Err()
}

func TestCommittedGeneralTerminalResolutionRequiresExactReplayAndOriginalTurn(t *testing.T) {
	for _, test := range []string{"missing event", "different event payload", "archive without original turn", "cancelled during event read"} {
		t.Run(test, func(t *testing.T) {
			thread, commit := committedGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
			archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1([]domainturnterminal.GeneralTerminalPublicationCommitV1{commit})
			if err != nil {
				t.Fatal(err)
			}
			thread[GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive)
			events, err := PrepareGeneralTerminalEventBundleV1(thread, commit.TurnID, commit, 1)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch test {
			case "missing event":
				events = events[:len(events)-1]
			case "different event payload":
				events[0]["generalTerminalPayloadDigest"] = domainsecurity.SHA256Hex([]byte("different"))
			case "archive without original turn":
				thread["turns"] = []any{}
			}
			tx := &generalTerminalRecoveryTransactionStub{replays: map[string]recoveryport.ReplaySnapshotV1{commit.ThreadID: {
				Events: events, Replayable: true, EventLogSHA256: domainsecurity.SHA256Hex([]byte("synthetic exact event log")),
			}}}
			store := &generalTerminalRecoveryStoreStub{tx: tx}
			primary := generalTerminalPrimaryReaderFunc(func(context.Context, string) (recoveryport.PrimaryThreadSnapshotV1, error) {
				if test == "cancelled during event read" {
					cancel()
				}
				return recoveryport.PrimaryThreadSnapshotV1{ThreadID: commit.ThreadID, ThreadFileSHA256: domainsecurity.SHA256Hex([]byte("synthetic primary")), Thread: thread}, nil
			})
			if _, err := ResolveCommittedGeneralTerminalV1(ctx, store, primary, commit.ThreadID, commit.TurnID); err == nil {
				t.Fatal("incomplete or cancelled ordinary terminal observation was admitted")
			}
			if tx.recordCalls != 0 || tx.settlements != 0 {
				t.Fatal("resolution repaired unproved state")
			}
		})
	}
}
