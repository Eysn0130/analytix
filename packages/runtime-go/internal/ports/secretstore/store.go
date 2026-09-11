package secretstore

import (
	"bytes"
	"context"
	"errors"
	"strings"
)

const (
	credentialRefPrefix = "cred_"
	credentialRefLength = len(credentialRefPrefix) + 43
	maxSecretBytes      = 1 << 20
)

var (
	ErrInvalidRequest       = errors.New("secret store: invalid request")
	ErrUnauthorized         = errors.New("secret store: unauthorized")
	ErrNotFound             = errors.New("secret store: not found")
	ErrTombstoned           = errors.New("secret store: tombstoned")
	ErrConflict             = errors.New("secret store: conflict")
	ErrClosed               = errors.New("secret store: closed")
	ErrPersistence          = errors.New("secret store: persistence failure")
	ErrMasterKeyUnavailable = errors.New("secret store: master key unavailable")
	ErrCryptographicFailure = errors.New("secret store: cryptographic failure")
)

type Purpose string

func NormalizePurpose(raw string) (Purpose, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if len(normalized) == 0 || len(normalized) > 96 {
		return "", ErrInvalidRequest
	}
	for index, value := range []byte(normalized) {
		if isLowerAlphaNumeric(value) {
			continue
		}
		if index > 0 && (value == '-' || value == '_' || value == '.' || value == ':' || value == '/') {
			continue
		}
		return "", ErrInvalidRequest
	}
	return Purpose(normalized), nil
}

type CredentialRef string

func ValidateCredentialRef(ref CredentialRef) error {
	value := string(ref)
	if len(value) != credentialRefLength || !strings.HasPrefix(value, credentialRefPrefix) {
		return ErrInvalidRequest
	}
	for _, current := range []byte(value[len(credentialRefPrefix):]) {
		if isLowerAlphaNumeric(current) || (current >= 'A' && current <= 'Z') || current == '-' || current == '_' {
			continue
		}
		return ErrInvalidRequest
	}
	return nil
}

type AccessRequest struct {
	CredentialRef CredentialRef
	Purpose       Purpose
	Consumer      string
}

func (request AccessRequest) Validate() error {
	if err := ValidateCredentialRef(request.CredentialRef); err != nil {
		return ErrInvalidRequest
	}
	normalized, err := NormalizePurpose(string(request.Purpose))
	if err != nil || normalized != request.Purpose {
		return ErrInvalidRequest
	}
	if !validConsumer(request.Consumer) {
		return ErrInvalidRequest
	}
	return nil
}

type ConsumerAuthorizer interface {
	AuthorizeCredentialAccess(context.Context, AccessRequest) error
}

// PreparedCandidate is the narrow K1 seam used by the Provider Registry. The
// opaque reference can be durably named by a Registry transaction before the
// encrypted candidate is committed. It exposes neither plaintext nor an
// enumeration/read bypass.
type PreparedCandidate interface {
	CredentialRef() CredentialRef
	Commit(context.Context) error
	Abort()
}

type RegistryStore interface {
	PreparePut(context.Context, Purpose, []byte) (PreparedCandidate, error)
	GetForAuthorizedConsumer(context.Context, AccessRequest) ([]byte, error)
	Tombstone(context.Context, CredentialRef, Purpose) error
	ExplicitDelete(context.Context, CredentialRef, Purpose, CredentialMutation) error
}

type MutationKind uint8

const (
	MutationInvalid MutationKind = iota
	MutationKeep
	MutationSet
	MutationExplicitDelete
)

type CredentialMutation struct {
	kind   MutationKind
	secret []byte
}

func KeepCredential() CredentialMutation {
	return CredentialMutation{kind: MutationKeep}
}

func SetCredential(secret []byte) (CredentialMutation, error) {
	if len(secret) == 0 || len(secret) > maxSecretBytes {
		return CredentialMutation{}, ErrInvalidRequest
	}
	return CredentialMutation{kind: MutationSet, secret: bytes.Clone(secret)}, nil
}

func ExplicitlyDeleteCredential() CredentialMutation {
	return CredentialMutation{kind: MutationExplicitDelete}
}

func (mutation CredentialMutation) Kind() MutationKind {
	return mutation.kind
}

func (mutation CredentialMutation) Secret() []byte {
	return bytes.Clone(mutation.secret)
}

func (mutation CredentialMutation) Validate() error {
	switch mutation.kind {
	case MutationKeep, MutationExplicitDelete:
		if len(mutation.secret) != 0 {
			return ErrInvalidRequest
		}
		return nil
	case MutationSet:
		if len(mutation.secret) == 0 || len(mutation.secret) > maxSecretBytes {
			return ErrInvalidRequest
		}
		return nil
	default:
		return ErrInvalidRequest
	}
}

func isLowerAlphaNumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9')
}

func validConsumer(value string) bool {
	if len(value) == 0 || len(value) > 96 || value != strings.TrimSpace(value) {
		return false
	}
	for index, current := range []byte(value) {
		if isLowerAlphaNumeric(current) || (current >= 'A' && current <= 'Z') {
			continue
		}
		if index > 0 && (current == '-' || current == '_' || current == '.' || current == ':' || current == '/') {
			continue
		}
		return false
	}
	return true
}
