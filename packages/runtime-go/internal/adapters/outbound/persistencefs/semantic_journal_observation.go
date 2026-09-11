package persistencefs

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"context"
	"errors"
	"os"
	"path/filepath"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// AuthenticatedSemanticJournalObservationV1 is a read-only observation of an
// existing signed transaction. It retains the complete program and exact
// cursor, not a recovery capability. Before states contain metadata only;
// callers must never treat an After blob as a missing original record.
type AuthenticatedSemanticJournalObservationV1 struct {
	roots           RootSet
	namespace       *JournalNamespaceAuthority
	journal         semanticJournalV1
	originalCreates privatecasport.OriginalCreateResiduesV1
}

// ObserveAuthenticatedSemanticJournalV1 performs the existing complete
// signature/root/stage/current-state preflight without creating, applying,
// retiring, cleaning, or writing a migration. The existing Unix v3 decoder
// retains its in-memory normalization/signing behavior. Absent and rollback-only journals
// return nil; no business operation can have run in a preparing journal.
func ObserveAuthenticatedSemanticJournalV1(ctx context.Context, roots RootSet,
	originals ...privatecasport.OriginalCreateResiduesV1,
) (*AuthenticatedSemanticJournalObservationV1, error) {
	if ctx == nil {
		return nil, errors.New("semantic journal observation context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	namespace, err := FreezeExistingJournalNamespaceAuthorityForRoots(roots)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	digest, err := authenticatedSemanticRecoveryConfigurationDigestWithPreservationV1(ctx, roots, namespace, nil, originals...)
	if err != nil {
		return nil, err
	}
	if digest == authenticatedSemanticRecoveryEmptyConfigurationDigest {
		return nil, nil
	}
	journal, err := readSemanticJournalObservationV1(ctx, roots, namespace)
	if errors.Is(err, os.ErrNotExist) {
		// Only a still-authenticated retirement/rollback-only residue may
		// explain the missing live journal; disappearance is not absence proof.
		currentDigest, currentErr := authenticatedSemanticRecoveryConfigurationDigestWithPreservationV1(ctx, roots, namespace, nil, originals...)
		if currentErr != nil || currentDigest != digest {
			return nil, errors.Join(errors.New("semantic journal disappeared during observation"), currentErr)
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if journal.State == semanticJournalPreparing {
		return nil, nil
	}
	proof, _, err := originalCreateSnapshotProofV1(ctx, roots, originals)
	if err != nil {
		return nil, err
	}
	observed := &AuthenticatedSemanticJournalObservationV1{roots: roots, namespace: namespace, journal: journal, originalCreates: proof}
	if err := observed.Revalidate(ctx); err != nil {
		return nil, err
	}
	return observed, nil
}

func readSemanticJournalObservationV1(ctx context.Context, roots RootSet, namespace *JournalNamespaceAuthority) (semanticJournalV1, error) {
	if err := contextError(ctx); err != nil {
		return semanticJournalV1{}, err
	}
	if namespace == nil || namespace.Validate() != nil || !namespace.matchesRoots(roots) {
		return semanticJournalV1{}, errors.New("semantic journal observation namespace is invalid")
	}
	journalRoot := filepath.Join(namespace.path(), "journal")
	directory, err := secureStartupOpenDirectory(namespace.root, "journal", "")
	if err != nil {
		return semanticJournalV1{}, err
	}
	defer directory.Close()
	authority, err := loadStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		return semanticJournalV1{}, err
	}
	body, _, err := directory.ReadFile("journal.json", maxSemanticJournalBytes, false)
	if err != nil {
		return semanticJournalV1{}, err
	}
	// The retirement decoder validates legacy signatures without invoking the
	// writable legacy journal migration used by the execution reader.
	journal, err := decodeSemanticJournalForRetirement(directory, body, authority)
	if err != nil {
		return semanticJournalV1{}, err
	}
	if journal.JournalRootIdentity != directory.Identity() || journal.RootBindingDigest != rootBindingDigest(roots) {
		return semanticJournalV1{}, errors.New("semantic journal observation root changed")
	}
	return journal, nil
}

func (observed *AuthenticatedSemanticJournalObservationV1) OperationsV1() []domainstartup.SemanticStartupOperationV1 {
	if observed == nil {
		return nil
	}
	return append([]domainstartup.SemanticStartupOperationV1(nil), observed.journal.Plan.Operations...)
}

func (observed *AuthenticatedSemanticJournalObservationV1) NextOperationV1() int {
	if observed == nil {
		return 0
	}
	return observed.journal.NextOperation
}

func (observed *AuthenticatedSemanticJournalObservationV1) Revalidate(ctx context.Context) error {
	if observed == nil || ctx == nil {
		return errors.New("semantic journal observation is unavailable")
	}
	digest, err := authenticatedSemanticRecoveryConfigurationDigestWithPreservationV1(ctx, observed.roots, observed.namespace, nil, observed.originalCreates)
	if err != nil {
		return err
	}
	current, err := readSemanticJournalObservationV1(ctx, observed.roots, observed.namespace)
	if err != nil {
		return err
	}
	if digest != observed.journal.Plan.ConfigurationDigest || current.JournalDigest != observed.journal.JournalDigest || current.AuthoritySignature != observed.journal.AuthoritySignature {
		return errors.New("semantic journal observation changed")
	}
	return nil
}

// PhysicallyAfterV1 proves the complete current entry state of one exact
// signed operation, including mode. Equal content alone cannot distinguish a
// cursor whose Before and After differ only in filesystem state.
func (observed *AuthenticatedSemanticJournalObservationV1) PhysicallyAfterV1(ctx context.Context, operation domainstartup.SemanticStartupOperationV1) (bool, error) {
	if observed == nil {
		return false, errors.New("semantic physical observation is unavailable")
	}
	found := false
	for _, candidate := range observed.journal.Plan.Operations {
		if candidate == operation {
			found = true
			break
		}
	}
	if !found {
		return false, errors.New("semantic physical observation is outside the signed program")
	}
	if err := observed.Revalidate(ctx); err != nil {
		return false, err
	}
	rootAuthority, err := restoreRootAuthorityFromJournal(observed.roots, observed.journal)
	if err != nil {
		return false, err
	}
	root, relative, err := managedRootRelative(observed.roots, operation.Path)
	if err != nil {
		return false, err
	}
	after, err := secureManagedTargetMatches(rootAuthority, root, relative, operation.After)
	if err != nil {
		return false, err
	}
	if err := observed.Revalidate(ctx); err != nil {
		return false, err
	}
	return after, nil
}

func (observed *AuthenticatedSemanticJournalObservationV1) ReadAfterV1(ctx context.Context, operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
	if observed == nil || ctx == nil || operation.Kind != domainstartup.SemanticOperationInstallFile {
		return nil, errors.New("semantic journal after observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	found := false
	for _, candidate := range observed.journal.Plan.Operations {
		if candidate == operation {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("semantic journal after observation is outside the signed program")
	}
	check := func() error {
		current, err := readSemanticJournalObservationV1(ctx, observed.roots, observed.namespace)
		if err != nil {
			return err
		}
		if current.JournalDigest != observed.journal.JournalDigest || current.AuthoritySignature != observed.journal.AuthoritySignature {
			return errors.New("semantic journal after observation changed")
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}
	directory, err := secureStartupOpenDirectory(observed.namespace.root, "journal", observed.journal.JournalRootIdentity)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	stage, err := directory.OpenDirectory("stage", observed.journal.JournalStageIdentity)
	if err != nil {
		return nil, err
	}
	defer stage.Close()
	body, _, err := stage.ReadFile(operation.OperationID, semanticFileSizeLimit(operation.After.Size), true)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
		return nil, errors.New("semantic journal after observation lost integrity")
	}
	if err := check(); err != nil {
		return nil, err
	}
	return body, nil
}
