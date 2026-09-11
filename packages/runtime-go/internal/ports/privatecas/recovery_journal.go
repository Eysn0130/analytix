package privatecas

import (
	"context"
	"errors"
	"time"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
)

var ErrJournalAbsent = errors.New("private CAS recovery journal is absent")

// RecoveryPreparationRequestV1 contains only the authority facts established
// by the caller's complete read-only global preflight. The persistence adapter
// fills root binding, installation-key, and handle-bound journal directory
// identities so callers cannot guess or self-issue those fields.
type RecoveryPreparationRequestV1 struct {
	AuthoritySetDigest string
	ParticipantCount   uint32
	PlanCount          uint32
	TopologyCount      uint32
	PreparedAt         time.Time
}

// RecoveryRetirementRequestV1 names both durable selectors required to
// retire a completed journal. A typed request prevents transaction, receipt,
// and inventory digests from being silently interchanged at the authority
// boundary.
type RecoveryRetirementRequestV1 struct {
	TransactionID           string
	CompletionReceiptDigest string
}

// RecoveryJournalSigningAuthorityV1 is implemented by the persistence
// adapter. Domain contracts expose only deterministic signing bytes and never
// load or use a host private key themselves.
type RecoveryJournalSigningAuthorityV1 interface {
	KeyID(context.Context) (string, error)
	Sign(context.Context, []byte) (string, error)
	Verify(context.Context, string, []byte, string) error
}

// RecoveryJournalStoreV1 persists one append-only recovery session. Each Put
// is no-replace: an existing record is accepted only as an authenticated,
// byte-exact replay. Load returns ErrJournalAbsent only when no preparation is
// present; malformed or partial state must return a distinct fail-closed error.
type RecoveryJournalStoreV1 interface {
	Load(context.Context) (domainprivatecas.RecoveryJournalSessionV1, error)
	PutPreparationIfAbsent(context.Context, domainprivatecas.RecoveryJournalPreparationV1) error
	PutTargetChunkIfAbsent(context.Context, domainprivatecas.RecoveryTargetChunkV1) error
	PutManifestIfAbsent(context.Context, domainprivatecas.RecoveryJournalManifestV1) error
	PutCommitWitnessIfAbsent(context.Context, domainprivatecas.RecoveryJournalCommitWitnessV1) error
	PutCompletionReceiptIfAbsent(context.Context, domainprivatecas.RecoveryJournalCompletionReceiptV1) error
	Retire(context.Context, RecoveryRetirementRequestV1) error
}

// RecoveryJournalV1 is the complete adapter boundary used by recovery. It
// deliberately owns neither CAS semantic decisions nor topology discovery.
type RecoveryJournalV1 interface {
	RecoveryJournalSigningAuthorityV1
	RecoveryJournalStoreV1
	BeginPreparationAfterValidatedPreflight(context.Context, RecoveryPreparationRequestV1) (domainprivatecas.RecoveryJournalPreparationV1, error)
}
