package reportpublication

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
)

// HasPreparedAttemptsV1 is a conservative pre-recovery presence probe of the
// exact attempts leaf. It grants no domain validity or activation authority.
// Empty sibling create residues remain for the normal recovery phase; a
// positive result requires full owner/dependency validation before use.
func HasPreparedAttemptsV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, originals ...*finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1) (bool, error) {
	if ctx == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return false, errors.New("report publication attempt presence root is invalid")
	}
	if len(originals) > 1 {
		return false, errors.New("report attempt original creation proof is ambiguous")
	}
	var prepared *finalauthority.PreparedSecurePrivateCASRecoveryV1
	var err error
	if len(originals) == 1 && originals[0] != nil {
		if originals[0].OwnerRootV1() != root {
			return false, errors.New("report attempt original creation root differs")
		}
		prepared, err = finalauthority.PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx, filepath.Join(root, "attempts"), maxPublicationAttemptBytes, access, originals[0])
	} else {
		prepared, err = finalauthority.PrepareSecurePrivateCASRecoveryIfPresent(ctx, filepath.Join(root, "attempts"), maxPublicationAttemptBytes, access)
	}
	if err != nil {
		return false, err
	}
	present := false
	if err := prepared.VisitCommittedMaterials(ctx, func(finalauthority.SecurePrivateCASPreparedMaterialV1) error {
		present = true
		return nil
	}); err != nil {
		return false, err
	}
	return present, nil
}

// PreparedAttemptInventoryV1 freezes the complete canonical attempt leaf. It
// exposes observation only, without recovery, signing, or publication authority.
type PreparedAttemptInventoryV1 struct {
	prepared *finalauthority.PreparedSecurePrivateCASRecoveryV1
	bodies   [][]byte
}

// PrepareAttemptInventoryV1 is for callers that have already validated the
// entire optional publication dependency closure. Rejected owners stay opaque.
func PrepareAttemptInventoryV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*PreparedAttemptInventoryV1, error) {
	if ctx == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("report publication attempt inventory root is invalid")
	}
	prepared, err := finalauthority.PrepareSecurePrivateCASRecoveryIfPresent(ctx, filepath.Join(root, "attempts"), maxPublicationAttemptBytes, access)
	if err != nil {
		return nil, err
	}
	snapshot := &PreparedAttemptInventoryV1{prepared: prepared}
	seen := map[string]bool{}
	if err := prepared.VisitCommittedFiles(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		attempt, err := domainpublication.ParsePublicationAttemptV1(file.Body)
		if err != nil {
			return err
		}
		canonical, err := domainpublication.PublicationAttemptV1Bytes(attempt)
		if err != nil || attempt.AttemptID != file.Digest || !bytes.Equal(canonical, file.Body) || seen[attempt.AttemptID] {
			return errors.New("report publication prepared attempt is invalid")
		}
		seen[attempt.AttemptID] = true
		snapshot.bodies = append(snapshot.bodies, append([]byte(nil), canonical...))
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (snapshot *PreparedAttemptInventoryV1) Revalidate(ctx context.Context) error {
	if snapshot == nil || snapshot.prepared == nil || ctx == nil {
		return errors.New("report publication prepared attempt inventory is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return snapshot.prepared.Revalidate(ctx)
}

func (snapshot *PreparedAttemptInventoryV1) VisitAttempts(ctx context.Context, visit func(domainpublication.PublicationAttemptV1) error) error {
	if visit == nil {
		return errors.New("report publication prepared attempt visitor is required")
	}
	if err := snapshot.Revalidate(ctx); err != nil {
		return err
	}
	for _, body := range snapshot.bodies {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt, err := domainpublication.ParsePublicationAttemptV1(body)
		if err != nil {
			return err
		}
		if err := visit(attempt); err != nil {
			return err
		}
	}
	return snapshot.Revalidate(ctx)
}
