package finalauthority

import (
	"context"
	"encoding/base64"
	"errors"
)

// DomainRecordUnavailableError means only the owner-supplied, in-memory domain
// checks failed. It is issued after every physical leaf and parent topology
// have been revalidated. It does not grant permission to use any owner record.
type DomainRecordUnavailableError struct {
	cause error
}

func (err *DomainRecordUnavailableError) Error() string {
	return "private domain records are unavailable"
}

func (err *DomainRecordUnavailableError) Unwrap() error { return err.cause }

type PrivateCASDomainVisitor func(string, func(SecurePrivateCASFile) error) error
type PrivateCASDomainMaterialVisitor func(string, func(SecurePrivateCASPreparedMaterialV1) error) error

// ValidateDomainInstallation checks already parsed, self-authenticated domain
// records against an independently enrolled installation. Key reads and root
// identity checks stay outside the pure domain callback, including on failure.
func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) ValidateDomainInstallation(
	ctx context.Context,
	authority *AnchoredFileAuthority,
	validate func(PrivateCASDomainVisitor, PrivateCASDomainMaterialVisitor, func(string, string) error) error,
) error {
	if authority == nil || validate == nil {
		return errors.New("private domain installation validation is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := authority.ValidateCurrentInstallation(ctx); err != nil {
		return err
	}
	keyID, publicKey := authority.KeyID(), base64.RawURLEncoding.EncodeToString(authority.PublicKey())
	check := func(recordKeyID, recordPublicKey string) error {
		if recordKeyID != keyID || recordPublicKey != publicKey {
			return errors.New("private domain record belongs to another installation")
		}
		return nil
	}
	domainErr := prepared.ValidateDomainSemantics(ctx, func(visit PrivateCASDomainVisitor, materials PrivateCASDomainMaterialVisitor) error {
		return validate(visit, materials, check)
	})
	if err := authority.ValidateCurrentInstallation(ctx); err != nil {
		return errors.Join(err, domainErr)
	}
	return domainErr
}

// ValidateDomainSemantics keeps storage errors separate from pure record and
// graph validation. The callback may only inspect the supplied record bytes
// and in-memory domain state; it must not read storage or perform effects.
// A rejected record never skips the remaining physical reads or final owner
// revalidation, including leaves the domain validator did not reach.
func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) ValidateDomainSemantics(
	ctx context.Context,
	validate func(PrivateCASDomainVisitor, PrivateCASDomainMaterialVisitor) error,
) error {
	if prepared == nil || validate == nil {
		return errors.New("private CAS domain validation is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return validatePrivateCASDomainSemantics(ctx, prepared.Revalidate,
		func(leaf string, visit func(SecurePrivateCASFile) error) error {
			return prepared.VisitCommittedFiles(ctx, leaf, visit)
		}, func(leaf string, visit func(SecurePrivateCASPreparedMaterialV1) error) error {
			return prepared.VisitCommittedMaterials(ctx, leaf, visit)
		}, validate)
}

// ValidatePreparedPrivateCASDomainSemanticsV1 applies the same domain/storage
// separation to owners whose CAS leaves and parent topology were prepared
// separately. The physical callback must revalidate the complete owner; the
// domain callback may only inspect supplied bytes and in-memory graph state.
func ValidatePreparedPrivateCASDomainSemanticsV1(
	ctx context.Context,
	plans map[string]*PreparedSecurePrivateCASRecoveryV1,
	revalidatePhysical func(context.Context) error,
	validate func(PrivateCASDomainVisitor) error,
) error {
	if revalidatePhysical == nil || validate == nil || len(plans) == 0 {
		return errors.New("prepared private CAS domain validation is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return validatePrivateCASDomainSemantics(ctx, revalidatePhysical,
		func(leaf string, visit func(SecurePrivateCASFile) error) error {
			plan := plans[leaf]
			if plan == nil {
				return errors.New("prepared private CAS domain leaf is unavailable")
			}
			return plan.VisitCommittedFiles(ctx, visit)
		}, nil, func(visit PrivateCASDomainVisitor, _ PrivateCASDomainMaterialVisitor) error {
			return validate(visit)
		})
}

func validatePrivateCASDomainSemantics(
	ctx context.Context,
	revalidate func(context.Context) error,
	readFiles PrivateCASDomainVisitor,
	readMaterials PrivateCASDomainMaterialVisitor,
	validate func(PrivateCASDomainVisitor, PrivateCASDomainMaterialVisitor) error,
) error {
	if err := revalidate(ctx); err != nil {
		return err
	}
	var storageErr error
	visit := func(leaf string, check func(SecurePrivateCASFile) error) error {
		if check == nil {
			storageErr = errors.New("private CAS domain visitor is invalid")
			return storageErr
		}
		var domainErr error
		err := readFiles(leaf, func(file SecurePrivateCASFile) error {
			if domainErr == nil {
				domainErr = check(file)
			}
			return nil
		})
		if err != nil {
			storageErr = errors.Join(storageErr, err)
			return err
		}
		return domainErr
	}
	visitMaterials := func(leaf string, check func(SecurePrivateCASPreparedMaterialV1) error) error {
		if check == nil || readMaterials == nil {
			storageErr = errors.New("private CAS domain material visitor is invalid")
			return storageErr
		}
		var domainErr error
		err := readMaterials(leaf, func(material SecurePrivateCASPreparedMaterialV1) error {
			if domainErr == nil {
				domainErr = check(material)
			}
			return nil
		})
		if err != nil {
			storageErr = errors.Join(storageErr, err)
			return err
		}
		return domainErr
	}
	domainErr := validate(visit, visitMaterials)
	if err := errors.Join(storageErr, revalidate(ctx), ctx.Err()); err != nil {
		return err
	}
	if errors.Is(domainErr, context.Canceled) || errors.Is(domainErr, context.DeadlineExceeded) {
		return domainErr
	}
	if domainErr != nil {
		return &DomainRecordUnavailableError{cause: domainErr}
	}
	return nil
}
