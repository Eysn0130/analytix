package generalterminalrecovery

import "context"

// PrimaryThreadSnapshotV1 is a raw, strict-decoded primary record from an
// anchored storage root. It is an ephemeral observation, not execution or
// publication authority. Sidecar reconstruction and normalization are forbidden.
type PrimaryThreadSnapshotV1 struct {
	ThreadID         string
	ThreadFileSHA256 string
	Thread           map[string]any
}

type PrimaryThreadReaderV1 interface {
	ReadPrimaryThreadSnapshotV1(context.Context, string) (PrimaryThreadSnapshotV1, error)
	ReadCommittedEventLogSHA256V1(context.Context, string) (string, error)
}

type ReplaySnapshotV1 struct {
	Events         []map[string]any
	Replayable     bool
	EventLogSHA256 string
}

// TransactionV1 is an exclusive, no-lock view of the durable event store.
// Implementations must keep one store-wide lock across the complete callback.
type TransactionV1 interface {
	ThreadIDs() ([]string, error)
	ReadCanonicalThread(string) (map[string]any, error)
	PublicationThreadView(string, map[string]any) (map[string]any, error)
	LoadEvents(string) (ReplaySnapshotV1, error)
	// ObserveEvents must not recover, normalize persistence, or remove journals.
	ObserveEvents(context.Context, string) (ReplaySnapshotV1, error)
	RecordTerminalBundle(string, string) error
	SettleTerminalUsage(map[string]any) error
}

type Store interface {
	WithGeneralTerminalRecoveryExclusiveV1(context.Context, func(TransactionV1) error) error
}

// StartupPreservationV1 carries the startup owner's already verified original
// hold into the exclusive transaction. Revalidation checks the complete held
// file inventory; it does not recover or classify any held publication.
type StartupPreservationV1 interface {
	RestartPreservesThreadV1(string) bool
	RevalidateStartupPreservationV1(context.Context) error
}
