package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	piistore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	publicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// A nonnil startup observation records an explicit routing decision even when
// no original creation residue is retained. It never grants recovery or write
// authority and cannot acquire additional original names during activation.
type runtimeOriginalCreateStartupV1 struct {
	roots       persistencefs.RootSet
	attachment  *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1
	publication map[string]*finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1
	combined    *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1
	plain       *finalauthority.OriginalPlainResiduesV1
}

type runtimeStartupPlanOutputV1 struct {
	configurationDigest       string
	originalCreates           *runtimeOriginalCreateStartupV1
	finalHistoryQualification *runtimeFinalHistoryQualificationV1
}

func prepareRuntimeOriginalCreateStartupV1(ctx context.Context, roots persistencefs.RootSet, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*runtimeOriginalCreateStartupV1, error) {
	plan, err := finalauthority.PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, roots.DataDir, access)
	if err != nil {
		return nil, err
	}
	owners := []string{"private/attachment-authority"}
	for _, owner := range runtimePublicationOwnersV1 {
		owners = append(owners, "private/"+owner)
	}
	combined, err := plan.OriginalCreateResiduesForOwnersV1(ctx, owners)
	if err != nil {
		return nil, err
	}
	proof, err := combined.ProjectOwnerV1(ctx, "private/attachment-authority")
	if err != nil {
		return nil, err
	}
	publication := map[string]*finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1{}
	for _, owner := range runtimePublicationOwnersV1 {
		publication[owner], err = combined.ProjectOwnerV1(ctx, "private/"+owner)
		if err != nil {
			return nil, err
		}
	}
	observations := []*finalauthority.OriginalFixedOwnerObservationV1{}
	for _, owner := range runtimePublicationOwnersV1 {
		root := filepath.Join(roots.DataDir, "private", owner)
		var observation *finalauthority.OriginalFixedOwnerObservationV1
		switch owner {
		case "pii-authorization":
			observation, err = piistore.PrepareOriginalObservationV1(ctx, root, access, publication[owner])
		case "report-publication":
			observation, err = publicationstore.PrepareOriginalObservationV1(ctx, root, access, publication[owner])
		case "controlled-artifact-access":
			observation, err = piistore.PrepareOriginalAccessObservationV1(ctx, root, access, publication[owner])
		case "controlled-artifact-access-v2":
			observation, err = piistore.PrepareOriginalAccessObservationV2(ctx, root, access, publication[owner])
		}
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	plain, err := finalauthority.FreezeOriginalPlainResiduesV1(ctx, observations)
	if err != nil {
		return nil, err
	}
	return &runtimeOriginalCreateStartupV1{roots: roots, attachment: proof, publication: publication, combined: combined, plain: plain}, nil
}

func (original *runtimeOriginalCreateStartupV1) proofV1() privatecasport.OriginalCreateResiduesV1 {
	if original == nil {
		return nil
	}
	if original.combined != nil {
		return original.combined
	}
	if original.attachment != nil {
		return original.attachment
	}
	return nil
}

func (original *runtimeOriginalCreateStartupV1) plainProofV1() privatecasport.OriginalPlainResiduesV1 {
	if original == nil || original.plain == nil {
		return nil
	}
	return original.plain
}

func (original *runtimeOriginalCreateStartupV1) revalidateV1(ctx context.Context, roots persistencefs.RootSet) error {
	if ctx == nil || original == nil || original.roots != roots {
		return errors.New("runtime original creation observation is unavailable or changed roots")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if original.attachment != nil && (original.attachment.DataRootV1() != roots.DataDir || original.attachment.OwnerRootV1() != filepath.Join(roots.DataDir, "private", "attachment-authority")) {
		return errors.New("runtime original creation owner binding changed")
	}
	for owner, proof := range original.publication {
		if !runtimePublicationDomainOwner(owner) || proof == nil || proof.DataRootV1() != roots.DataDir || proof.OwnerRootV1() != filepath.Join(roots.DataDir, "private", owner) {
			return errors.New("runtime original publication creation binding changed")
		}
	}
	if original.plain != nil {
		if original.plain.DataRootV1() != roots.DataDir {
			return errors.New("runtime original plain residue roots differ")
		}
		if err := original.plain.Revalidate(ctx); err != nil {
			return err
		}
	}
	if proof := original.proofV1(); proof != nil {
		return proof.Revalidate(ctx)
	}
	return nil
}

func (original *runtimeOriginalCreateStartupV1) retainedV1(ctx context.Context, preserved runtimeReportRestartPreservationV1) (*runtimeOriginalCreateStartupV1, error) {
	if original == nil {
		return nil, errors.New("runtime original creation observation was not captured")
	}
	retained := &runtimeOriginalCreateStartupV1{roots: original.roots}
	var owners []string
	if preserved.attachments != nil {
		if original.attachment == nil || preserved.attachments.createResidues != original.attachment {
			return nil, errors.New("runtime attachment preservation replaced its original creation proof")
		}
		retained.attachment = original.attachment
		owners = append(owners, "private/attachment-authority")
	}
	if preserved.publication.frozenV1() {
		retained.publication = original.publication
		for _, owner := range runtimePublicationOwnersV1 {
			if original.publication[owner] == nil {
				return nil, errors.New("runtime unavailable publication creation proof is incomplete")
			}
			owners = append(owners, "private/"+owner)
		}
	}
	if len(owners) != 0 && original.combined != nil {
		var err error
		retained.combined, err = original.combined.SelectOwnersV1(ctx, owners)
		if err != nil {
			return nil, err
		}
	}
	if original.plain != nil {
		var err error
		retained.plain, err = original.plain.SelectOwnersV1(ctx, owners)
		if err != nil {
			return nil, err
		}
	}
	return retained, nil
}

func (original *runtimeOriginalCreateStartupV1) copiedV1(ctx context.Context, roots persistencefs.RootSet, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*runtimeOriginalCreateStartupV1, error) {
	if original == nil || roots == original.roots {
		return nil, errors.New("runtime original creation stage roots are invalid")
	}
	if err := original.revalidateV1(ctx, original.roots); err != nil {
		return nil, err
	}
	copied := &runtimeOriginalCreateStartupV1{roots: roots}
	if original.plain != nil {
		observed, err := original.plain.ObserveCopiedOriginalPlainResiduesV1(ctx, roots.DataDir, access)
		if err != nil {
			return nil, err
		}
		var ok bool
		copied.plain, ok = observed.(*finalauthority.OriginalPlainResiduesV1)
		if !ok || copied.plain == nil {
			return nil, errors.New("runtime original plain copy producer changed")
		}
	}
	if original.combined != nil {
		observed, err := original.combined.ObserveCopiedOriginalCreateResiduesV1(ctx, roots.DataDir, access)
		if err != nil {
			return nil, err
		}
		var ok bool
		copied.combined, ok = observed.(*finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1)
		if !ok || copied.combined == nil {
			return nil, errors.New("runtime creation set copy producer changed")
		}
		if original.attachment != nil {
			copied.attachment, err = copied.combined.ProjectOwnerV1(ctx, "private/attachment-authority")
			if err != nil {
				return nil, err
			}
		}
		if len(original.publication) != 0 {
			copied.publication = map[string]*finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1{}
			for _, owner := range runtimePublicationOwnersV1 {
				copied.publication[owner], err = copied.combined.ProjectOwnerV1(ctx, "private/"+owner)
				if err != nil {
					return nil, err
				}
			}
		}
		return copied, copied.revalidateV1(ctx, roots)
	}
	if original.attachment != nil {
		proof, err := original.attachment.ObserveCopiedOriginalCreateResiduesV1(ctx, roots.DataDir, access)
		if err != nil {
			return nil, err
		}
		var ok bool
		copied.attachment, ok = proof.(*finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1)
		if !ok || copied.attachment == nil {
			return nil, errors.New("runtime original creation stage proof producer changed")
		}
	}
	return copied, copied.revalidateV1(ctx, roots)
}

func (core *runtimeChildIdentityStartupV1) originalCreateProofV1() privatecasport.OriginalCreateResiduesV1 {
	if core == nil {
		return nil
	}
	return core.originalCreates.proofV1()
}
