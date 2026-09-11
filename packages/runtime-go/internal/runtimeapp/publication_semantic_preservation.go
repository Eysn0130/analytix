package runtimeapp

import (
	"context"
	"encoding/base64"
	"errors"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	piistore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	publicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

var runtimePublicationOwnersV1 = [...]string{"pii-authorization", "report-publication", "controlled-artifact-access", "controlled-artifact-access-v2"}

type runtimePublicationSemanticPreservationV1 struct {
	core                 *runtimeChildIdentityStartupV1
	installation         *finalauthority.AnchoredFileAuthority
	localKey             *finalauthority.ExistingFileVerificationV1
	original             map[string]runtimeOriginalSemanticFilesV1
	recoveryBefore       map[string]runtimeOriginalSemanticFilesV1
	unavailable          bool
	deferred             *runtimeDeferredReportHistoryV1
	closed               *runtimeOriginalReportHistoryV1
	originalOwnerCreates runtimeOriginalSemanticFilesV1
}

func (preserved *runtimePublicationSemanticPreservationV1) frozenV1() bool {
	return preserved != nil && (preserved.unavailable || preserved.recoveryDeferredV1())
}

func (preserved *runtimePublicationSemanticPreservationV1) recoveryDeferredV1() bool {
	return preserved != nil && (preserved.deferred != nil || preserved.closed != nil)
}

type runtimePublicationOriginalObservationV1 struct {
	owners               []*finalauthority.OriginalFixedOwnerObservationV1
	journal              *persistencefs.AuthenticatedSemanticJournalObservationV1
	installation         *finalauthority.AnchoredFileAuthority
	originalOwnerCreates runtimeOriginalSemanticFilesV1
	finalOwnerCreates    runtimeOriginalSemanticFilesV1
}

func (observation *runtimePublicationOriginalObservationV1) Revalidate(ctx context.Context) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	var err error
	if observation.installation != nil {
		err = observation.installation.ValidateCurrentInstallation(ctx)
	}
	for _, owner := range observation.owners {
		err = errors.Join(err, owner.RevalidatePhysicalV1(ctx))
	}
	if observation.journal != nil {
		err = errors.Join(err, observation.journal.Revalidate(ctx))
	}
	return errors.Join(err, context.Cause(ctx))
}

// Every full4 Before is reconstructed before classifying availability. A
// deleted original cannot become healthy empty state, and a newly installed
// dependency cannot retroactively make an unavailable Original authoritative.
func (preserved *runtimePublicationSemanticPreservationV1) readV1(ctx context.Context) (_ map[string]runtimeOriginalSemanticFilesV1, _ map[string]runtimeOriginalSemanticFilesV1, _ *runtimePublicationOriginalObservationV1, resultErr error) {
	if ctx == nil || preserved == nil || preserved.core == nil {
		return nil, nil, nil, errors.New("original publication observation is unavailable")
	}
	core := preserved.core
	if preserved.localKey != nil {
		if err := preserved.localKey.Revalidate(ctx); err != nil {
			return nil, nil, nil, err
		}
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, nil, nil, err
	}
	observation := &runtimePublicationOriginalObservationV1{journal: journal, installation: preserved.installation}
	defer func() { resultErr = errors.Join(resultErr, observation.Revalidate(ctx)) }()
	originals, finals := map[string]runtimeOriginalSemanticFilesV1{}, map[string]runtimeOriginalSemanticFilesV1{}
	for _, owner := range runtimePublicationOwnersV1 {
		root := filepath.Join(core.roots.DataDir, "private", owner)
		var physicalOwner *finalauthority.OriginalFixedOwnerObservationV1
		switch owner {
		case "pii-authorization":
			physicalOwner, err = piistore.PrepareOriginalObservationV1(ctx, root, core.access, preserved.creationProofV1(owner))
		case "report-publication":
			physicalOwner, err = publicationstore.PrepareOriginalObservationV1(ctx, root, core.access, preserved.creationProofV1(owner))
		case "controlled-artifact-access":
			physicalOwner, err = piistore.PrepareOriginalAccessObservationV1(ctx, root, core.access, preserved.creationProofV1(owner))
		case "controlled-artifact-access-v2":
			physicalOwner, err = piistore.PrepareOriginalAccessObservationV2(ctx, root, core.access, preserved.creationProofV1(owner))
		}
		if err != nil {
			return nil, nil, nil, err
		}
		observation.owners = append(observation.owners, physicalOwner)
		raw, err := physicalOwner.SnapshotOriginalFilesV1(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		physical := runtimeOriginalSemanticFilesV1(raw)
		original, final := physical.cloneV1(), physical.cloneV1()
		for index, operation := range journal.OperationsV1() {
			if err := validateRuntimeOriginalSemanticAncestorV1("data/private/"+owner, operation); err != nil {
				return nil, nil, nil, err
			}
			name, owned := runtimeAssociatedRelativeV1(owner, operation)
			if !owned {
				continue
			}
			entry, found := physical[name]
			state := runtimeOriginalSemanticStateV1(entry, found)
			if state != operation.Before {
				if index > journal.NextOperationV1() || state != operation.After {
					return nil, nil, nil, errors.New("original publication physical state is outside signed prefix")
				}
				after, err := journal.PhysicallyAfterV1(ctx, operation)
				if err != nil || !after {
					return nil, nil, nil, errors.Join(errors.New("original publication state has not reached signed After"), err)
				}
			}
			switch operation.Before.Type {
			case domainstartup.ManagedEntryTypeAbsent:
				delete(original, name)
			case domainstartup.ManagedEntryTypeDirectory:
				original[name] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: operation.Before.Mode & 0o777}
			case domainstartup.ManagedEntryTypeFile:
				if !found || entry.Directory || int64(len(entry.Body)) != operation.Before.Size || domainsecurity.SHA256Hex(entry.Body) != operation.Before.SHA256 {
					previous, exists := preserved.recoveryBefore[owner][name]
					if !exists || runtimeOriginalSemanticStateV1(previous, true) != operation.Before {
						return nil, nil, nil, errors.New("original publication Before bytes are unavailable")
					}
					entry = previous
				}
				entry.Mode = operation.Before.Mode
				original[name] = entry
			default:
				return nil, nil, nil, errors.New("original publication Before type is invalid")
			}
			if err := final.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return nil, nil, nil, err
			}
		}
		originals[owner], finals[owner] = original, final
	}
	observation.originalOwnerCreates, observation.finalOwnerCreates, err = preserved.readOwnerCreatesV1(ctx, journal)
	if err != nil {
		return nil, nil, nil, err
	}
	if preserved.originalOwnerCreates != nil {
		if err := preserved.validateOwnerCreatesV1(observation.originalOwnerCreates, observation.finalOwnerCreates); err != nil {
			return nil, nil, nil, err
		}
	}
	return originals, finals, observation, nil
}

func (preserved *runtimePublicationSemanticPreservationV1) originalUnavailableV1(ctx context.Context, files map[string]runtimeOriginalSemanticFilesV1) (_ bool, resultErr error) {
	installation := preserved.installation
	var keyErr error
	defer func() {
		if preserved.localKey != nil {
			keyErr = errors.Join(keyErr, preserved.localKey.Revalidate(ctx))
		}
		if keyErr != nil {
			resultErr = errors.Join(resultErr, keyErr)
		}
	}()
	localCheck := func(keyID, publicKey string) error {
		if preserved.localKey == nil {
			root, err := persistencefs.FreezeRootAuthority(preserved.core.roots)
			if err == nil {
				preserved.localKey, err = finalauthority.OpenExistingFileVerificationV1(root)
			}
			if err != nil {
				keyErr = err
				return err
			}
		}
		if err := preserved.localKey.Revalidate(ctx); err != nil {
			keyErr = err
			return err
		}
		if keyID != preserved.localKey.KeyID() || publicKey != base64.RawURLEncoding.EncodeToString(preserved.localKey.PublicKey()) {
			return errors.New("original publication record belongs to another local signing key")
		}
		return nil
	}
	unavailable := false
	for _, owner := range runtimePublicationOwnersV1 {
		raw, found := files[owner]
		if !found {
			return false, errors.New("original publication closure is incomplete")
		}
		var err error
		switch owner {
		case "pii-authorization":
			err = piistore.ValidateOriginalEntriesV1(ctx, raw, installation, localCheck, preserved.creationProofV1(owner))
		case "report-publication":
			err = publicationstore.ValidateOriginalEntriesV1(ctx, raw, installation, localCheck, preserved.creationProofV1(owner))
		case "controlled-artifact-access":
			err = piistore.ValidateOriginalAccessEntriesV1(ctx, raw, installation, localCheck, preserved.creationProofV1(owner))
		case "controlled-artifact-access-v2":
			err = piistore.ValidateOriginalAccessEntriesV2(ctx, raw, installation, localCheck, preserved.creationProofV1(owner))
		}
		if err != nil {
			if _, domainOnly := err.(*finalauthority.DomainRecordUnavailableError); !domainOnly {
				return false, err
			}
			unavailable = true
		}
	}
	if !unavailable {
		visit := func(owner, leaf string, collect func(finalauthority.SecurePrivateCASFile)) error {
			names := []string{}
			for name, entry := range files[owner] {
				if entry.Directory || !strings.HasPrefix(name, leaf+"/") {
					continue
				}
				digest := strings.TrimSuffix(path.Base(name), ".json")
				if domainsecurity.IsSHA256Hex(digest) && name == leaf+"/"+digest[:2]+"/"+digest+".json" {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			for _, name := range names {
				if err := context.Cause(ctx); err != nil {
					return err
				}
				collect(finalauthority.SecurePrivateCASFile{Digest: strings.TrimSuffix(path.Base(name), ".json"), Body: files[owner][name].Body})
			}
			return nil
		}
		visitArtifacts := func(required map[string]bool, read func(finalauthority.SecurePrivateCASFile) error) error {
			var result error
			err := visit("report-publication", "artifacts", func(file finalauthority.SecurePrivateCASFile) {
				if required[file.Digest] {
					result = errors.Join(result, read(file))
				}
			})
			return errors.Join(result, err)
		}
		domainErr, readErr := validateRuntimePublicationGraphV1(visit, visitArtifacts)
		if readErr != nil {
			return false, readErr
		}
		unavailable = domainErr != nil
	}
	if err := context.Cause(ctx); err != nil {
		return false, err
	}
	if installation != nil {
		if err := installation.ValidateCurrentInstallation(ctx); err != nil {
			return false, err
		}
	} else if unavailable {
		return false, errRuntimeOptionalDomainInstallationRequired
	}
	return unavailable, nil
}

func prepareRuntimePublicationSemanticPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1, installation *finalauthority.AnchoredFileAuthority) (_ *runtimePublicationSemanticPreservationV1, resultErr error) {
	preserved := &runtimePublicationSemanticPreservationV1{core: core, installation: installation}
	originals, finals, observation, err := preserved.readV1(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, observation.Revalidate(ctx)) }()
	preserved.unavailable, err = preserved.originalUnavailableV1(ctx, originals)
	if err != nil {
		return nil, err
	}
	preserved.original, preserved.recoveryBefore = originals, originals
	preserved.originalOwnerCreates = observation.originalOwnerCreates
	if err := preserved.validateOwnerCreatesV1(observation.originalOwnerCreates, observation.finalOwnerCreates); err != nil {
		return nil, err
	}
	if err := preserved.validateEndpointV1(ctx, finals); err != nil {
		return nil, err
	}
	return preserved, nil
}

func (preserved *runtimePublicationSemanticPreservationV1) validateEndpointV1(ctx context.Context, files map[string]runtimeOriginalSemanticFilesV1) error {
	if preserved.frozenV1() {
		if !reflect.DeepEqual(preserved.original, files) {
			return errors.New("original preserved publication bytes, mode or absence changed")
		}
		return nil
	}
	unavailable, err := preserved.originalUnavailableV1(ctx, files)
	if err != nil || unavailable {
		return errors.Join(errors.New("original publication candidate closure is unavailable"), err)
	}
	return nil
}

func (preserved *runtimePublicationSemanticPreservationV1) ValidateSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	originals, candidates, observation, err := preserved.readV1(ctx)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx))
		if resultErr == nil {
			preserved.recoveryBefore = originals
		}
	}()
	if err := preserved.validateEndpointV1(ctx, originals); err != nil {
		return err
	}
	if err := preserved.validateEndpointV1(ctx, candidates); err != nil {
		return err
	}
	for _, operation := range operations {
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue
		}
		if name, owned, err := runtimePublicationOwnerCreatePathV1(operation); err != nil {
			return err
		} else if owned {
			if err := observation.finalOwnerCreates.applyV1(operation, name, readAfter); err != nil {
				return err
			}
		}
		for _, owner := range runtimePublicationOwnersV1 {
			if err := validateRuntimeOriginalSemanticAncestorV1("data/private/"+owner, operation); err != nil {
				return err
			}
			name, owned := runtimeAssociatedRelativeV1(owner, operation)
			if owned {
				if err := candidates[owner].applyV1(operation, name, readAfter); err != nil {
					return err
				}
			}
		}
	}
	return errors.Join(preserved.validateEndpointV1(ctx, candidates), preserved.validateOwnerCreatesV1(observation.originalOwnerCreates, observation.finalOwnerCreates))
}

// Owner creation residues live beside the owner, so they have their own exact
// physical inventory. They never turn the private parent into a CAS owner.
func runtimePublicationOwnerCreatePathV1(operation domainstartup.SemanticStartupOperationV1) (string, bool, error) {
	for _, owner := range runtimePublicationOwnersV1 {
		name := "private/" + domainprivatecas.CreateDirectoryResidueNameV1(owner)
		if strings.HasPrefix(operation.Path, "data/"+name+"/") {
			return "", false, errors.New("publication owner creation residue is not empty")
		}
		if operation.Path != "data/"+name {
			continue
		}
		for _, state := range []domainstartup.SemanticEntryStateV1{operation.Before, operation.After} {
			if state.Type != domainstartup.ManagedEntryTypeAbsent && state.Type != domainstartup.ManagedEntryTypeDirectory {
				return "", false, errors.New("publication owner creation residue type is invalid")
			}
		}
		return name, true, nil
	}
	return "", false, nil
}

func (preserved *runtimePublicationSemanticPreservationV1) readOwnerCreatesV1(ctx context.Context, journal *persistencefs.AuthenticatedSemanticJournalObservationV1) (runtimeOriginalSemanticFilesV1, runtimeOriginalSemanticFilesV1, error) {
	physical := runtimeOriginalSemanticFilesV1{}
	for _, owner := range runtimePublicationOwnersV1 {
		name := "private/" + domainprivatecas.CreateDirectoryResidueNameV1(owner)
		if proof := preserved.creationProofV1(owner); proof != nil {
			for _, state := range proof.DirectoryStatesV1() {
				if state.RelativePath == name {
					physical[name] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: state.Mode & 0o777}
				}
			}
		}
	}
	original, final := physical.cloneV1(), physical.cloneV1()
	for index, operation := range journal.OperationsV1() {
		name, owned, err := runtimePublicationOwnerCreatePathV1(operation)
		if err != nil {
			return nil, nil, err
		}
		if !owned {
			continue
		}
		entry, found := physical[name]
		state := runtimeOriginalSemanticStateV1(entry, found)
		if state != operation.Before {
			if index > journal.NextOperationV1() || state != operation.After {
				return nil, nil, errors.New("publication owner creation state is outside signed prefix")
			}
			after, err := journal.PhysicallyAfterV1(ctx, operation)
			if err != nil || !after {
				return nil, nil, errors.Join(errors.New("publication owner creation state has not reached signed After"), err)
			}
		}
		if operation.Before.Type == domainstartup.ManagedEntryTypeAbsent {
			delete(original, name)
		} else {
			original[name] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: operation.Before.Mode & 0o777}
		}
		if err := final.applyV1(operation, name, nil); err != nil {
			return nil, nil, err
		}
	}
	return original, final, nil
}

func (preserved *runtimePublicationSemanticPreservationV1) validateOwnerCreatesV1(original, final runtimeOriginalSemanticFilesV1) error {
	if preserved.frozenV1() && (!reflect.DeepEqual(preserved.originalOwnerCreates, original) || !reflect.DeepEqual(preserved.originalOwnerCreates, final)) {
		return errors.New("original unavailable publication owner creation directory changed")
	}
	return nil
}

func (preserved *runtimePublicationSemanticPreservationV1) prepareOriginalDirectoriesV1(ctx context.Context) ([]string, func(context.Context) error, error) {
	if !preserved.frozenV1() {
		return nil, nil, nil
	}
	originals, finals, observation, err := preserved.readV1(ctx)
	if err != nil {
		return nil, nil, err
	}
	if observation.journal != nil {
		return nil, nil, errors.New("publication orphan cleanup cannot interleave with semantic recovery")
	}
	if err := preserved.validateEndpointV1(ctx, originals); err != nil {
		return nil, nil, err
	}
	if err := preserved.validateEndpointV1(ctx, finals); err != nil {
		return nil, nil, err
	}
	var directories []string
	for _, owner := range runtimePublicationOwnersV1 {
		creates := map[string]bool{}
		if proof := preserved.creationProofV1(owner); proof != nil {
			for _, name := range proof.RelativePathsV1() {
				creates[name] = true
			}
		}
		for name, entry := range preserved.original[owner] {
			relative := path.Join("private", owner, name)
			if entry.Directory && !creates[relative] {
				directories = append(directories, relative)
			}
		}
	}
	sort.Strings(directories)
	if err := observation.Revalidate(ctx); err != nil {
		return nil, nil, err
	}
	return directories, observation.Revalidate, nil
}

func (preserved *runtimePublicationSemanticPreservationV1) preparePhysicalRevalidationV1(ctx context.Context) (func(context.Context) error, error) {
	if !preserved.frozenV1() {
		return nil, nil
	}
	original, final, observation, err := preserved.readV1(ctx)
	if err != nil {
		return nil, err
	}
	if err := preserved.validateEndpointV1(ctx, original); err != nil {
		return nil, err
	}
	if err := preserved.validateEndpointV1(ctx, final); err != nil {
		return nil, err
	}
	return observation.Revalidate, observation.Revalidate(ctx)
}

func (preserved *runtimePublicationSemanticPreservationV1) creationProofV1(owner string) *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1 {
	if preserved == nil || preserved.core == nil || preserved.core.originalCreates == nil {
		return nil
	}
	return preserved.core.originalCreates.publication[owner]
}

// This projection runs outside recovery exclusion. Inside the transaction the
// boundaries revalidate only these already prepared physical/key/C proofs.
func prepareRuntimeFrozenPublicationOwnersV1(ctx context.Context, dataDir string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, installation *finalauthority.AnchoredFileAuthority, originals ...*runtimePublicationSemanticPreservationV1) (map[string]*runtimeOptionalDomainBoundary, error) {
	if len(originals) > 1 {
		return nil, errors.New("original publication recovery binding is ambiguous")
	}
	if len(originals) == 0 || !originals[0].frozenV1() {
		return nil, nil
	}
	preserved := originals[0]
	if preserved.core == nil || preserved.core.roots.DataDir != dataDir || preserved.core.access != access || installation == nil || preserved.installation != installation {
		return nil, errors.New("original publication recovery binding differs")
	}
	original, final, observation, err := preserved.readV1(ctx)
	if err != nil {
		return nil, err
	}
	if err := preserved.validateEndpointV1(ctx, original); err != nil {
		return nil, err
	}
	if err := preserved.validateEndpointV1(ctx, final); err != nil {
		return nil, err
	}
	boundaries := map[string]*runtimeOptionalDomainBoundary{}
	for index, name := range runtimePublicationOwnersV1 {
		frozen, err := observation.owners[index].FreezeRecoveryV1(ctx)
		if err != nil {
			return nil, err
		}
		boundary, err := newRuntimeOptionalDomainBoundary(runtimePrivateCASOwnerRecovery{name: name, expectedRoots: runtimePrivateCASExpectedRoots(dataDir, name)}, frozen, installation)
		if err != nil {
			return nil, err
		}
		boundary.reportDeferred = preserved.recoveryDeferredV1()
		boundaries[name] = boundary
	}
	return boundaries, observation.Revalidate(ctx)
}
