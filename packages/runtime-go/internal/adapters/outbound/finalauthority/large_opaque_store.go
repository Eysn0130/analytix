package finalauthority

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const maxLargeOpaqueObjectBytes = uint64(1 << 40)

var (
	ErrLargeOpaqueIntegrity           = largeopaqueport.ErrIntegrity
	ErrLargeOpaqueIndeterminate       = largeopaqueport.ErrIndeterminate
	ErrLargeOpaqueSourceMismatch      = largeopaqueport.ErrSourceMismatch
	ErrLargeOpaqueSourceRead          = largeopaqueport.ErrSourceRead
	ErrLargeOpaqueDelivery            = largeopaqueport.ErrDelivery
	ErrLargeOpaqueUnsupportedPlatform = largeopaqueport.ErrUnsupportedPlatform
)

// LargeOpaqueStore is a streaming sibling of SecurePrivateCAS. It reuses the
// same frozen-root and handle-relative filesystem primitives, but owns no
// materialized inventory or authority generation and exposes no enumeration.
// Its outcomes are inert storage facts, never evidence or snapshot authority.
type LargeOpaqueStore struct {
	rootPath string
	maxBytes uint64
	access   privatecasport.RecoveryAccessAuthority
}

var _ largeopaqueport.Store = (*LargeOpaqueStore)(nil)

func NewLargeOpaqueStore(
	root string,
	maxBytes uint64,
	access privatecasport.RecoveryAccessAuthority,
) (*LargeOpaqueStore, error) {
	if largeOpaqueNilInterface(access) || maxBytes == 0 || maxBytes > maxLargeOpaqueObjectBytes {
		return nil, errors.New("large opaque store configuration is invalid")
	}
	rootPath, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return nil, errors.Join(errors.New("large opaque store root is invalid"), err)
	}
	return &LargeOpaqueStore{rootPath: rootPath, maxBytes: maxBytes, access: access}, nil
}

func (store *LargeOpaqueStore) PutExact(
	ctx context.Context,
	reference largeopaqueport.ExactRef,
	reader io.Reader,
) (largeopaqueport.PutOutcome, error) {
	if store == nil || largeOpaqueNilInterface(store.access) || largeOpaqueNilInterface(reader) || !store.validReference(reference) {
		return 0, errors.New("large opaque store write input is invalid")
	}
	ctx = largeOpaqueContext(ctx)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !largeOpaquePutInputSupported(reader) {
		return 0, ErrLargeOpaqueUnsupportedPlatform
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return 0, err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	outcome := largeopaqueport.PutOutcome(0)
	err := withExistingPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil {
			return err
		}
		if !present {
			return errors.Join(
				ErrLargeOpaqueIndeterminate,
				errors.New("large opaque store root requires authenticated provisioning"),
			)
		}
		outcome, err = largeOpaquePutExactPlatform(ctx, authority, reference, reader)
		return err
	})
	if err != nil {
		return 0, errors.Join(errors.New("large opaque store write failed"), err)
	}
	if outcome != largeopaqueport.PutOutcomeCreated && outcome != largeopaqueport.PutOutcomeExistingEqual {
		return 0, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque store returned an invalid write outcome"))
	}
	return outcome, nil
}

func (store *LargeOpaqueStore) ReadExact(
	ctx context.Context,
	reference largeopaqueport.ExactRef,
	writer io.Writer,
) error {
	if store == nil || largeOpaqueNilInterface(store.access) || largeOpaqueNilInterface(writer) || !store.validReference(reference) {
		return errors.New("large opaque store read input is invalid")
	}
	ctx = largeOpaqueContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	err := withExistingPrivateCASAccess(ctx, store.access, store.rootPath, func(binding privatecasport.RootBinding) error {
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil {
			return err
		}
		if !present {
			return os.ErrNotExist
		}
		return largeOpaqueReadExactPlatform(ctx, authority, reference, writer)
	})
	if err != nil {
		return errors.Join(errors.New("large opaque store exact read failed"), err)
	}
	return nil
}

func (store *LargeOpaqueStore) validReference(reference largeopaqueport.ExactRef) bool {
	return validPrivateDigest(reference.Address) && validPrivateDigest(reference.SHA256) &&
		reference.ByteLength > 0 && reference.ByteLength <= store.maxBytes
}

func largeOpaqueContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func largeOpaqueNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func largeOpaqueCanonicalName(address string) string {
	return address + ".blob"
}

func largeOpaqueValidateCanonicalName(address, name string) bool {
	return validPrivateDigest(address) && name == largeOpaqueCanonicalName(address) &&
		filepath.Base(name) == name && !strings.ContainsRune(name, 0)
}
