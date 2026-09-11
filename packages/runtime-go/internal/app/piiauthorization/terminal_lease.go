package piiauthorization

import (
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"sync"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrControlledPIITerminalLeaseInvalid       = errors.New("controlled PII terminal lease is invalid")
	ErrControlledPIITerminalLeaseConsumed      = errors.New("controlled PII terminal lease is not available")
	ErrControlledPIITerminalLeaseConsumerPanic = errors.New("controlled PII terminal lease consumer panicked")
	ErrControlledPIITerminalLeaseIndeterminate = errors.New("controlled PII terminal handoff is indeterminate")
	ErrControlledPIITerminalReaderClosed       = errors.New("controlled PII terminal reader is closed")
)

const (
	ControlledPIITerminalHandoffConsumedV1      = "consumed"
	ControlledPIITerminalHandoffDiscardedV1     = "discarded"
	ControlledPIITerminalHandoffIndeterminateV1 = "indeterminate"

	ControlledPIITerminalReasonHandoffCompleteV1  = "handoff_complete"
	ControlledPIITerminalReasonAbandonedV1        = "abandoned"
	ControlledPIITerminalReasonContextCancelledV1 = "context_cancelled"
	ControlledPIITerminalReasonStaleContextV1     = "stale_context"
	ControlledPIITerminalReasonConsumerErrorV1    = "consumer_error"
	ControlledPIITerminalReasonConsumerPanicV1    = "consumer_panic"
	ControlledPIITerminalReasonPartialReadV1      = "partial_read"
	ControlledPIITerminalReasonIntegrityFailureV1 = "integrity_failure"
)

const (
	controlledPIITerminalStateReadyV1      = "ready"
	controlledPIITerminalStateConsumingV1  = "consuming"
	controlledPIITerminalStateFinalizingV1 = "finalizing"
)

// ControlledPIITerminalLeaseMetadataV1 contains only hashes and bounded
// authority metadata. It is safe to persist for diagnostics, but it is not a
// PublicationReceipt and grants no access to controlled bytes.
type ControlledPIITerminalLeaseMetadataV1 struct {
	ContextDigest          string
	ContextEpoch           uint64
	DatasetSnapshotID      string
	ClaimLedgerDigest      string
	TargetIdentityDigest   string
	ArtifactSHA256         string
	ArtifactByteLength     uint64
	MediaType              string
	PIIAuthorizationDigest string
	PIIProjectionDigest    string
	ArtifactMetadata       domainpii.ControlledPIIArtifactMetadataV1
}

// ControlledPIITerminalDispositionV1 records only what the host-side meter
// observed during an in-process handoff. A consumed disposition is neither a
// PublicationReceipt nor proof of a later user display/export effect.
type ControlledPIITerminalDispositionV1 struct {
	Status             string
	ReasonCode         string
	ArtifactSHA256     string
	ArtifactByteLength uint64
	ObservedByteLength uint64
}

type ControlledPIITerminalConsumerV1 func(
	context.Context,
	io.Reader,
	ControlledPIITerminalLeaseMetadataV1,
) error

// ControlledPIITerminalLeaseV1 is a pointer-only, single-consumer carrier.
// Its body has no exported accessor or serialization path and is zeroed on
// every terminal path, including cancellation, consumer error, and panic.
type ControlledPIITerminalLeaseV1 struct {
	mu          sync.Mutex
	metadata    ControlledPIITerminalLeaseMetadataV1
	body        []byte
	state       string
	cancel      context.CancelFunc
	disposition *ControlledPIITerminalDispositionV1
}

func newControlledPIITerminalLeaseV1(
	metadata ControlledPIITerminalLeaseMetadataV1,
	body []byte,
) (*ControlledPIITerminalLeaseV1, error) {
	if validateControlledPIITerminalLeaseMetadataV1(metadata) != nil ||
		uint64(len(body)) != metadata.ArtifactByteLength || domainsecurity.SHA256Hex(body) != metadata.ArtifactSHA256 {
		clearControlledArtifactBytesV1(body)
		return nil, ErrControlledPIITerminalLeaseInvalid
	}
	return &ControlledPIITerminalLeaseV1{metadata: metadata, body: body, state: controlledPIITerminalStateReadyV1}, nil
}

func (lease *ControlledPIITerminalLeaseV1) Metadata() (ControlledPIITerminalLeaseMetadataV1, error) {
	if lease == nil {
		return ControlledPIITerminalLeaseMetadataV1{}, ErrControlledPIITerminalLeaseInvalid
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if validateControlledPIITerminalLeaseMetadataV1(lease.metadata) != nil {
		return ControlledPIITerminalLeaseMetadataV1{}, ErrControlledPIITerminalLeaseInvalid
	}
	return lease.metadata, nil
}

func (lease *ControlledPIITerminalLeaseV1) Disposition() (ControlledPIITerminalDispositionV1, bool) {
	if lease == nil {
		return ControlledPIITerminalDispositionV1{}, false
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.disposition == nil {
		return ControlledPIITerminalDispositionV1{}, false
	}
	return *lease.disposition, true
}

// Consume transfers the controlled body to one synchronous trusted consumer.
// The callback runs outside the lease lock and cannot self-assert success: the
// host meter must observe every byte and the exact approved SHA-256.
func (lease *ControlledPIITerminalLeaseV1) Consume(
	ctx context.Context,
	consumer ControlledPIITerminalConsumerV1,
) (ControlledPIITerminalDispositionV1, error) {
	if lease == nil || ctx == nil || consumer == nil {
		return ControlledPIITerminalDispositionV1{}, ErrControlledPIITerminalLeaseInvalid
	}

	lease.mu.Lock()
	if lease.state != controlledPIITerminalStateReadyV1 || lease.disposition != nil {
		lease.mu.Unlock()
		return ControlledPIITerminalDispositionV1{}, ErrControlledPIITerminalLeaseConsumed
	}
	body := lease.body
	lease.body = nil
	if validateControlledPIITerminalLeaseMetadataV1(lease.metadata) != nil ||
		uint64(len(body)) != lease.metadata.ArtifactByteLength || domainsecurity.SHA256Hex(body) != lease.metadata.ArtifactSHA256 {
		lease.state = controlledPIITerminalStateFinalizingV1
		lease.mu.Unlock()
		disposition := lease.finishDetachedV1(body, ControlledPIITerminalHandoffDiscardedV1,
			ControlledPIITerminalReasonIntegrityFailureV1, 0)
		return disposition, ErrControlledPIITerminalLeaseInvalid
	}
	if ctx.Err() != nil {
		lease.state = controlledPIITerminalStateFinalizingV1
		lease.mu.Unlock()
		disposition := lease.finishDetachedV1(body, ControlledPIITerminalHandoffDiscardedV1,
			ControlledPIITerminalReasonContextCancelledV1, 0)
		return disposition, ctx.Err()
	}
	consumeCtx, cancel := context.WithCancel(ctx)
	lease.state = controlledPIITerminalStateConsumingV1
	lease.cancel = cancel
	metadata := lease.metadata
	lease.mu.Unlock()

	reader := newControlledPIIMeteredReaderV1(body)
	var consumerErr error
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		consumerErr = consumer(consumeCtx, reader, metadata)
	}()
	observedLength, observedSHA256 := reader.closeAndSnapshotV1()
	consumeContextErr := consumeCtx.Err()
	cancel()

	status := ControlledPIITerminalHandoffIndeterminateV1
	reason := ControlledPIITerminalReasonIntegrityFailureV1
	resultErr := ErrControlledPIITerminalLeaseIndeterminate
	switch {
	case panicked:
		reason = ControlledPIITerminalReasonConsumerPanicV1
		resultErr = ErrControlledPIITerminalLeaseConsumerPanic
	case consumeContextErr != nil:
		reason = ControlledPIITerminalReasonContextCancelledV1
	case consumerErr != nil:
		reason = ControlledPIITerminalReasonConsumerErrorV1
	case observedLength != metadata.ArtifactByteLength:
		reason = ControlledPIITerminalReasonPartialReadV1
	case observedSHA256 != metadata.ArtifactSHA256:
		reason = ControlledPIITerminalReasonIntegrityFailureV1
	default:
		status = ControlledPIITerminalHandoffConsumedV1
		reason = ControlledPIITerminalReasonHandoffCompleteV1
		resultErr = nil
	}
	disposition := lease.finishDetachedV1(body, status, reason, observedLength)
	return disposition, resultErr
}

// Close abandons an unused lease, is idempotent after terminalization, and
// cancels an in-flight consumer without clearing bytes underneath its reader.
func (lease *ControlledPIITerminalLeaseV1) Close() error {
	if lease == nil {
		return ErrControlledPIITerminalLeaseInvalid
	}
	lease.mu.Lock()
	switch lease.state {
	case controlledPIITerminalStateReadyV1:
		body := lease.body
		lease.body = nil
		lease.state = controlledPIITerminalStateFinalizingV1
		lease.mu.Unlock()
		lease.finishDetachedV1(body, ControlledPIITerminalHandoffDiscardedV1,
			ControlledPIITerminalReasonAbandonedV1, 0)
		return nil
	case controlledPIITerminalStateConsumingV1:
		cancel := lease.cancel
		lease.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return nil
	case controlledPIITerminalStateFinalizingV1,
		ControlledPIITerminalHandoffConsumedV1,
		ControlledPIITerminalHandoffDiscardedV1,
		ControlledPIITerminalHandoffIndeterminateV1:
		lease.mu.Unlock()
		return nil
	default:
		body := lease.body
		lease.body = nil
		lease.state = controlledPIITerminalStateFinalizingV1
		lease.mu.Unlock()
		lease.finishDetachedV1(body, ControlledPIITerminalHandoffDiscardedV1,
			ControlledPIITerminalReasonIntegrityFailureV1, 0)
		return ErrControlledPIITerminalLeaseInvalid
	}
}

func (lease *ControlledPIITerminalLeaseV1) finishDetachedV1(
	body []byte,
	status string,
	reason string,
	observed uint64,
) ControlledPIITerminalDispositionV1 {
	clearControlledArtifactBytesV1(body)
	disposition := ControlledPIITerminalDispositionV1{
		Status: status, ReasonCode: reason, ArtifactSHA256: lease.metadata.ArtifactSHA256,
		ArtifactByteLength: lease.metadata.ArtifactByteLength, ObservedByteLength: observed,
	}
	lease.mu.Lock()
	lease.cancel = nil
	lease.state = status
	lease.disposition = &disposition
	lease.mu.Unlock()
	return disposition
}

// Serialization and formatting are intentionally closed. Metadata must be
// requested explicitly; generic diagnostics never get a reversible []byte
// representation of the lease.
func (*ControlledPIITerminalLeaseV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("controlled PII terminal lease cannot be serialized")
}

func (*ControlledPIITerminalLeaseV1) MarshalText() ([]byte, error) {
	return nil, errors.New("controlled PII terminal lease cannot be serialized")
}

func (*ControlledPIITerminalLeaseV1) MarshalBinary() ([]byte, error) {
	return nil, errors.New("controlled PII terminal lease cannot be serialized")
}

func (*ControlledPIITerminalLeaseV1) GobEncode() ([]byte, error) {
	return nil, errors.New("controlled PII terminal lease cannot be serialized")
}

func (*ControlledPIITerminalLeaseV1) String() string {
	return "<controlled-pii-terminal-lease>"
}

func (*ControlledPIITerminalLeaseV1) GoString() string {
	return "<controlled-pii-terminal-lease>"
}

func (*ControlledPIITerminalLeaseV1) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<controlled-pii-terminal-lease>")
}

func validateControlledPIITerminalLeaseMetadataV1(metadata ControlledPIITerminalLeaseMetadataV1) error {
	if !domainsecurity.IsSHA256Hex(metadata.ContextDigest) || metadata.ContextEpoch == 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(metadata.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(metadata.ClaimLedgerDigest) || !domainsecurity.IsSHA256Hex(metadata.TargetIdentityDigest) ||
		!domainsecurity.IsSHA256Hex(metadata.ArtifactSHA256) || metadata.ArtifactByteLength == 0 ||
		metadata.ArtifactByteLength > domainpii.MaxControlledPIIArtifactBytesV1 || metadata.MediaType != domainpii.ControlledPIIArtifactMediaTypeV1 ||
		!domainsecurity.IsSHA256Hex(metadata.PIIAuthorizationDigest) || !domainsecurity.IsSHA256Hex(metadata.PIIProjectionDigest) ||
		metadata.ArtifactMetadata.SHA256 != metadata.ArtifactSHA256 ||
		metadata.ArtifactMetadata.ByteLength != metadata.ArtifactByteLength ||
		metadata.ArtifactMetadata.MediaType != metadata.MediaType ||
		metadata.ArtifactMetadata.ClaimLedgerDigest != metadata.ClaimLedgerDigest ||
		metadata.ArtifactMetadata.TargetIdentityDigest != metadata.TargetIdentityDigest {
		return ErrControlledPIITerminalLeaseInvalid
	}
	return nil
}

type controlledPIIMeteredReaderV1 struct {
	mu     sync.Mutex
	body   []byte
	offset int
	hash   hash.Hash
	closed bool
}

func newControlledPIIMeteredReaderV1(body []byte) *controlledPIIMeteredReaderV1 {
	return &controlledPIIMeteredReaderV1{body: body, hash: sha256.New()}
}

func (reader *controlledPIIMeteredReaderV1) Read(target []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed {
		return 0, ErrControlledPIITerminalReaderClosed
	}
	if reader.offset >= len(reader.body) {
		return 0, io.EOF
	}
	count := copy(target, reader.body[reader.offset:])
	if count > 0 {
		_, _ = reader.hash.Write(reader.body[reader.offset : reader.offset+count])
		reader.offset += count
	}
	return count, nil
}

func (reader *controlledPIIMeteredReaderV1) closeAndSnapshotV1() (uint64, string) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.closed = true
	reader.body = nil
	return uint64(reader.offset), hex.EncodeToString(reader.hash.Sum(nil))
}

var (
	_ io.Closer                = (*ControlledPIITerminalLeaseV1)(nil)
	_ json.Marshaler           = (*ControlledPIITerminalLeaseV1)(nil)
	_ encoding.TextMarshaler   = (*ControlledPIITerminalLeaseV1)(nil)
	_ encoding.BinaryMarshaler = (*ControlledPIITerminalLeaseV1)(nil)
	_ fmt.Stringer             = (*ControlledPIITerminalLeaseV1)(nil)
	_ fmt.GoStringer           = (*ControlledPIITerminalLeaseV1)(nil)
	_ fmt.Formatter            = (*ControlledPIITerminalLeaseV1)(nil)
)
