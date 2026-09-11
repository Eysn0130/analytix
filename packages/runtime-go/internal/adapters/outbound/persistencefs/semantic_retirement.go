package persistencefs

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	semanticJournalRetirementSchemaVersion = 2
	semanticJournalRetirementFile          = "retirement.json"
	semanticJournalRetirementRollback      = "rollback"
	semanticJournalRetirementPublish       = "committed"
	semanticJournalRetirementEmpty         = "empty"
	maxSemanticRetirementBytes             = 64 << 10
)

var (
	semanticJournalRetirementSignatureDomain   = []byte("analytix.semantic-startup-journal-retirement/v2\x00")
	semanticJournalRetirementV1SignatureDomain = []byte("analytix.semantic-startup-journal-retirement/v1\x00")
)

// semanticJournalRetirementV1 is host-issued deletion authority. A directory
// name is never evidence that a journal may be removed: the tombstone binds the
// installation key, directory inode, signed journal generation and operation
// frontier, plus the exact top-level residue present before retirement.
type semanticJournalRetirementV1 struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Disposition         string `json:"disposition"`
	JournalRootIdentity string `json:"journalRootIdentity"`
	RootBindingDigest   string `json:"rootBindingDigest"`
	ConfigurationDigest string `json:"configurationDigest"`
	SourceEntry         string `json:"sourceEntry"`
	JournalBodyDigest   string `json:"journalBodyDigest"`
	JournalDigest       string `json:"journalDigest"`
	JournalState        string `json:"journalState"`
	PlanDigest          string `json:"planDigest"`
	NextOperation       int    `json:"nextOperation"`
	OperationCount      int    `json:"operationCount"`
	ResidueDigest       string `json:"residueDigest"`
	AuthorityKeyID      string `json:"authorityKeyId"`
	TombstoneDigest     string `json:"tombstoneDigest"`
	AuthoritySignature  string `json:"authoritySignature"`
}

type semanticRetirementResidueV1 struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Identity string `json:"identity,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
}

func retireSemanticJournal(
	namespace *JournalNamespaceAuthority,
	expectedIdentity string,
	roots RootSet,
	configurationDigest string,
	fault func(string, int) error,
) error {
	return retireSemanticJournalContext(context.Background(), namespace, expectedIdentity, roots, configurationDigest, fault)
}

func retireSemanticJournalContext(
	ctx context.Context,
	namespace *JournalNamespaceAuthority,
	expectedIdentity string,
	roots RootSet,
	configurationDigest string,
	fault func(string, int) error,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if namespace == nil || namespace.Validate() != nil || strings.TrimSpace(expectedIdentity) == "" ||
		!domainsecurity.IsSHA256Hex(configurationDigest) {
		return errors.New("semantic startup journal retirement authority is invalid")
	}
	journalRoot := filepath.Join(namespace.path(), "journal")
	authority, err := loadStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		return errors.New("semantic startup journal retirement signing authority is unavailable")
	}
	directory, err := secureStartupOpenDirectory(namespace.root, "journal", expectedIdentity)
	if err != nil {
		return err
	}
	record, err := semanticRetirementRecordForDirectoryContext(ctx, directory, authority, roots, configurationDigest)
	if err == nil {
		err = contextError(ctx)
	}
	if err == nil {
		err = ensureSemanticRetirementTombstone(directory, authority, roots, record)
	}
	if err == nil {
		err = directory.PreflightTreeContext(ctx)
	}
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	if err := semanticFault(fault, "after_journal_retirement_tombstone", -1); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	retiredName, retired, err := secureStartupRetireDirectory(namespace.root, "journal", expectedIdentity, ".retired-journal-")
	if err != nil || retired == nil {
		return err
	}
	defer retired.Close()
	if err := verifySemanticRetirementDirectoryContext(ctx, retired, authority, roots, originals...); err != nil {
		return err
	}
	if err := semanticFault(fault, "after_journal_retired", -1); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return secureStartupRemoveRetiredDirectoryContext(ctx, namespace.root, retiredName, retired)
}

func recoverRetiredSemanticJournals(namespace *JournalNamespaceAuthority, roots RootSet) error {
	return recoverRetiredSemanticJournalsContext(context.Background(), namespace, roots)
}

type retiredSemanticJournalRecoveryPlan struct {
	name     string
	identity string
}

func preflightRetiredSemanticJournalsContext(
	ctx context.Context,
	namespace *JournalNamespaceAuthority,
	roots RootSet,
	originals ...privatecasport.OriginalCreateResiduesV1,
) ([]retiredSemanticJournalRecoveryPlan, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if namespace == nil || namespace.Validate() != nil {
		return nil, errors.New("semantic startup retired journal authority is invalid")
	}
	root, err := secureStartupOpenRootDirectory(namespace.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := root.ReadEntriesBoundedContext(ctx, maxSemanticNamespaceEntries)
	if err != nil {
		return nil, err
	}
	retiredNames := make([]string, 0)
	for _, entry := range entries {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(entry.Name(), ".retired-journal-") {
			continue
		}
		random := strings.TrimPrefix(entry.Name(), ".retired-journal-")
		if !entry.IsDir() || len(random) != 32 || !isLowerHex(random) {
			return nil, errors.New("semantic startup retired journal residue is unsafe")
		}
		retiredNames = append(retiredNames, entry.Name())
	}
	if len(retiredNames) == 0 {
		return nil, nil
	}
	authority, err := loadStartupJournalAuthority(namespace, filepath.Join(namespace.path(), "journal"))
	if err != nil {
		return nil, errors.New("semantic startup retired journal signing authority is unavailable")
	}
	sort.Strings(retiredNames)
	plans := make([]retiredSemanticJournalRecoveryPlan, 0, len(retiredNames))
	for _, name := range retiredNames {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		retired, err := root.OpenDirectory(name, "")
		if err != nil {
			return nil, err
		}
		if err := retired.PreflightTreeContext(ctx); err != nil {
			_ = retired.Close()
			return nil, err
		}
		if err := verifySemanticRetirementDirectoryContext(ctx, retired, authority, roots, originals...); err != nil {
			_ = retired.Close()
			return nil, err
		}
		plans = append(plans, retiredSemanticJournalRecoveryPlan{name: name, identity: retired.Identity()})
		if err := retired.Close(); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func recoverRetiredSemanticJournalsContext(ctx context.Context, namespace *JournalNamespaceAuthority, roots RootSet,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	plans, err := preflightRetiredSemanticJournalsContext(ctx, namespace, roots, originals...)
	if err != nil || len(plans) == 0 {
		return err
	}
	root, err := secureStartupOpenRootDirectory(namespace.root)
	if err != nil {
		return err
	}
	defer root.Close()
	authority, err := loadStartupJournalAuthority(namespace, filepath.Join(namespace.path(), "journal"))
	if err != nil {
		return errors.New("semantic startup retired journal signing authority is unavailable")
	}
	for _, plan := range plans {
		if err := contextError(ctx); err != nil {
			return err
		}
		retired, err := root.OpenDirectory(plan.name, plan.identity)
		if err != nil {
			return err
		}
		if err := retired.PreflightTreeContext(ctx); err != nil {
			_ = retired.Close()
			return err
		}
		if err := verifySemanticRetirementDirectoryContext(ctx, retired, authority, roots, originals...); err != nil {
			_ = retired.Close()
			return err
		}
		if err := root.RemoveRetiredDirectoryContext(ctx, plan.name, retired); err != nil {
			_ = retired.Close()
			return err
		}
		if err := retired.Close(); err != nil {
			return err
		}
	}
	return nil
}

func ensureSemanticRetirementTombstone(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
	record semanticJournalRetirementV1,
) error {
	body, _, err := directory.ReadFile(semanticJournalRetirementFile, maxSemanticRetirementBytes, false)
	if err == nil {
		existing, _, decodeErr := decodeSemanticRetirementForDirectory(body, directory, authority, roots)
		if decodeErr != nil {
			return decodeErr
		}
		if !sameSemanticRetirement(existing, record) {
			return errors.New("semantic startup journal retirement tombstone conflicts with current state")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	record.AuthoritySignature = ""
	record.TombstoneDigest = semanticRetirementDigest(record)
	signature, err := authority.signDomain(semanticJournalRetirementSignatureDomain, semanticRetirementSigningBytes(record))
	if err != nil {
		return err
	}
	record.AuthoritySignature = signature
	if err := validateSemanticRetirement(record, authority); err != nil {
		return err
	}
	body, err = json.Marshal(record)
	if err != nil {
		return err
	}
	return directory.WriteExclusive(semanticJournalRetirementFile, body, maxSemanticRetirementBytes)
}

func verifySemanticRetirementDirectory(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
) error {
	return verifySemanticRetirementDirectoryContext(context.Background(), directory, authority, roots)
}

func verifySemanticRetirementDirectoryContext(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if directory == nil || strings.TrimSpace(directory.Identity()) == "" {
		return errors.New("semantic startup retired journal directory is unavailable")
	}
	body, _, err := directory.ReadFile(semanticJournalRetirementFile, maxSemanticRetirementBytes, false)
	if err != nil {
		return errors.New("semantic startup retired journal has no signed retirement tombstone")
	}
	recorded, _, err := decodeSemanticRetirementForDirectory(body, directory, authority, roots)
	if err != nil {
		return err
	}
	if recorded.RootBindingDigest != rootBindingDigest(roots) {
		return errors.New("semantic startup retired journal belongs to different persistence roots")
	}
	current, err := semanticRetirementRecordForDirectoryContext(ctx, directory, authority, roots, recorded.ConfigurationDigest)
	if err != nil {
		return err
	}
	if !sameSemanticRetirement(recorded, current) {
		return errors.New("semantic startup retired journal no longer matches its signed retirement frontier")
	}
	return validateSemanticRetirementManagedStateContext(ctx, directory, authority, roots, recorded, originals...)
}

func validateSemanticRetirementManagedState(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
	record semanticJournalRetirementV1,
) error {
	return validateSemanticRetirementManagedStateContext(context.Background(), directory, authority, roots, record)
}

func validateSemanticRetirementManagedStateContext(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
	record semanticJournalRetirementV1,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if record.JournalState == semanticJournalRetirementEmpty {
		return nil
	}
	body, _, err := directory.ReadFile(record.SourceEntry, maxSemanticJournalBytes, false)
	if err != nil || domainsecurity.SHA256Hex(body) != record.JournalBodyDigest {
		return errors.New("semantic startup retirement journal body changed before cleanup")
	}
	journal, err := decodeSemanticJournalForRetirement(directory, body, authority)
	if err != nil || journal.JournalDigest != record.JournalDigest || journal.State != record.JournalState ||
		journal.NextOperation != record.NextOperation || len(journal.Plan.Operations) != record.OperationCount {
		return errors.New("semantic startup retirement journal frontier changed before cleanup")
	}
	rootAuthority, err := restoreRootAuthorityFromJournal(roots, journal)
	if err != nil {
		return err
	}
	return validateSemanticJournalCurrentStateContext(ctx, roots, rootAuthority, journal, originals...)
}

func semanticRetirementRecordForDirectory(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
	configurationDigest string,
) (semanticJournalRetirementV1, error) {
	return semanticRetirementRecordForDirectoryContext(context.Background(), directory, authority, roots, configurationDigest)
}

func semanticRetirementRecordForDirectoryContext(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
	configurationDigest string,
) (semanticJournalRetirementV1, error) {
	if err := contextError(ctx); err != nil {
		return semanticJournalRetirementV1{}, err
	}
	if directory == nil || authority == nil || directory.Identity() == "" ||
		!domainsecurity.IsSHA256Hex(configurationDigest) {
		return semanticJournalRetirementV1{}, errors.New("semantic startup retirement inspection authority is invalid")
	}
	entries, err := directory.ReadEntriesBoundedContext(ctx, maxSemanticRetirementTreeEntries)
	if err != nil {
		return semanticJournalRetirementV1{}, err
	}
	visible := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != semanticJournalRetirementFile {
			visible = append(visible, entry)
		}
	}
	base := semanticJournalRetirementV1{
		SchemaVersion: semanticJournalRetirementSchemaVersion, JournalRootIdentity: directory.Identity(),
		RootBindingDigest: rootBindingDigest(roots), ConfigurationDigest: configurationDigest,
		AuthorityKeyID: authority.keyID,
	}
	journalEntry := findSemanticRetirementEntry(visible, "journal.json")
	if journalEntry != nil {
		if journalEntry.IsDir() {
			return semanticJournalRetirementV1{}, errors.New("semantic startup retirement journal entry is unsafe")
		}
		body, _, err := directory.ReadFile("journal.json", maxSemanticJournalBytes, false)
		if err != nil {
			return semanticJournalRetirementV1{}, err
		}
		journal, err := decodeSemanticJournalForRetirement(directory, body, authority)
		if err != nil {
			return semanticJournalRetirementV1{}, err
		}
		if err := populateSemanticRetirementFromJournal(&base, journal, "journal.json", body, roots, configurationDigest); err != nil {
			return semanticJournalRetirementV1{}, err
		}
	} else {
		candidates := make([]os.DirEntry, 0, 1)
		for _, entry := range visible {
			if validSemanticJournalTempName(entry) {
				candidates = append(candidates, entry)
			}
		}
		switch {
		case len(visible) == 0:
			base.Disposition = semanticJournalRetirementRollback
			base.SourceEntry = ""
			base.JournalBodyDigest = domainsecurity.SHA256Hex(nil)
			base.JournalDigest = domainsecurity.SHA256Hex(nil)
			base.JournalState = semanticJournalRetirementEmpty
			base.PlanDigest = domainsecurity.SHA256Hex(nil)
		case len(visible) == 1 && len(candidates) == 1:
			body, _, err := directory.ReadFile(candidates[0].Name(), maxSemanticJournalBytes, false)
			if err != nil {
				return semanticJournalRetirementV1{}, err
			}
			journal, err := decodeSemanticJournalForRetirement(directory, body, authority)
			if err != nil || journal.State != semanticJournalPreparing {
				return semanticJournalRetirementV1{}, errors.New("semantic startup retirement temp is not a signed rollback frontier")
			}
			if err := populateSemanticRetirementFromJournal(&base, journal, candidates[0].Name(), body, roots, configurationDigest); err != nil {
				return semanticJournalRetirementV1{}, err
			}
		default:
			return semanticJournalRetirementV1{}, errors.New("semantic startup retirement journal residue is ambiguous")
		}
	}
	residueDigest, err := semanticRetirementResidueDigestContext(ctx, directory, authority, visible, base)
	if err != nil {
		return semanticJournalRetirementV1{}, err
	}
	base.ResidueDigest = residueDigest
	base.TombstoneDigest = semanticRetirementDigest(base)
	if err := validateSemanticRetirementShape(base); err != nil {
		return semanticJournalRetirementV1{}, err
	}
	return base, nil
}

func populateSemanticRetirementFromJournal(
	record *semanticJournalRetirementV1,
	journal semanticJournalV1,
	sourceEntry string,
	body []byte,
	roots RootSet,
	configurationDigest string,
) error {
	if record == nil || journal.JournalRootIdentity != record.JournalRootIdentity ||
		journal.RootBindingDigest != rootBindingDigest(roots) || journal.Plan.ConfigurationDigest != configurationDigest {
		return errors.New("semantic startup retirement journal belongs to different startup authority")
	}
	switch journal.State {
	case semanticJournalPreparing:
		record.Disposition = semanticJournalRetirementRollback
	case semanticJournalCommitted:
		record.Disposition = semanticJournalRetirementPublish
	default:
		return errors.New("semantic startup journal frontier is not retirement eligible")
	}
	record.SourceEntry = sourceEntry
	record.JournalBodyDigest = domainsecurity.SHA256Hex(body)
	record.JournalDigest = journal.JournalDigest
	record.JournalState = journal.State
	record.PlanDigest = journal.Plan.PlanDigest
	record.NextOperation = journal.NextOperation
	record.OperationCount = len(journal.Plan.Operations)
	return nil
}

func semanticRetirementResidueDigest(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	entries []os.DirEntry,
	record semanticJournalRetirementV1,
) (string, error) {
	return semanticRetirementResidueDigestContext(context.Background(), directory, authority, entries, record)
}

func semanticRetirementResidueDigestContext(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	entries []os.DirEntry,
	record semanticJournalRetirementV1,
) (string, error) {
	residue, err := semanticRetirementResidueInventoryContext(ctx, directory, authority, entries, record)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(residue)
	return domainsecurity.SHA256Hex(body), nil
}

func semanticRetirementResidueInventory(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	entries []os.DirEntry,
	record semanticJournalRetirementV1,
) ([]semanticRetirementResidueV1, error) {
	return semanticRetirementResidueInventoryContext(context.Background(), directory, authority, entries, record)
}

func semanticRetirementResidueInventoryContext(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	entries []os.DirEntry,
	record semanticJournalRetirementV1,
) ([]semanticRetirementResidueV1, error) {
	residue := make([]semanticRetirementResidueV1, 0, len(entries))
	for _, entry := range entries {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		name := entry.Name()
		if name == "stage" {
			if !entry.IsDir() || record.JournalState == semanticJournalRetirementEmpty {
				return nil, errors.New("semantic startup retirement stage residue is invalid")
			}
			journalBody, _, err := directory.ReadFile(record.SourceEntry, maxSemanticJournalBytes, false)
			if err != nil {
				return nil, err
			}
			journal, err := decodeSemanticJournalForRetirement(directory, journalBody, authority)
			if err != nil {
				return nil, errors.New("semantic startup retirement stage has no signed journal")
			}
			stage, err := directory.OpenDirectory("stage", journal.JournalStageIdentity)
			if err != nil {
				return nil, err
			}
			residue = append(residue, semanticRetirementResidueV1{Name: name, Kind: "directory", Identity: stage.Identity()})
			if err := stage.Close(); err != nil {
				return nil, err
			}
			continue
		}
		if entry.IsDir() || name != "journal.json" && !validSemanticJournalTempName(entry) {
			return nil, errors.New("semantic startup retirement contains unknown residue")
		}
		body, identity, err := directory.ReadFile(name, maxSemanticJournalBytes, false)
		if err != nil {
			return nil, err
		}
		candidate, err := decodeSemanticJournalForRetirement(directory, body, authority)
		if err != nil || candidate.JournalRootIdentity != record.JournalRootIdentity ||
			candidate.RootBindingDigest != record.RootBindingDigest || candidate.Plan.ConfigurationDigest != record.ConfigurationDigest {
			return nil, errors.New("semantic startup retirement contains an untrusted journal residue")
		}
		residue = append(residue, semanticRetirementResidueV1{Name: name, Kind: "file", Identity: identity, SHA256: domainsecurity.SHA256Hex(body)})
	}
	sort.Slice(residue, func(left, right int) bool { return residue[left].Name < residue[right].Name })
	return residue, nil
}

func findSemanticRetirementEntry(entries []os.DirEntry, name string) os.DirEntry {
	for _, entry := range entries {
		if entry.Name() == name {
			return entry
		}
	}
	return nil
}

func semanticRetirementDigest(record semanticJournalRetirementV1) string {
	record.TombstoneDigest = ""
	record.AuthoritySignature = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func semanticRetirementSigningBytes(record semanticJournalRetirementV1) []byte {
	record.AuthoritySignature = ""
	body, _ := json.Marshal(record)
	return body
}

func sameSemanticRetirement(left, right semanticJournalRetirementV1) bool {
	left.AuthoritySignature = ""
	right.AuthoritySignature = ""
	return left == right
}

func validateSemanticRetirementShape(record semanticJournalRetirementV1) error {
	if record.SchemaVersion != semanticJournalRetirementSchemaVersion ||
		strings.TrimSpace(record.JournalRootIdentity) == "" || !domainsecurity.IsSHA256Hex(record.RootBindingDigest) ||
		!domainsecurity.IsSHA256Hex(record.ConfigurationDigest) || !domainsecurity.IsSHA256Hex(record.JournalBodyDigest) ||
		!domainsecurity.IsSHA256Hex(record.JournalDigest) || !domainsecurity.IsSHA256Hex(record.PlanDigest) ||
		!domainsecurity.IsSHA256Hex(record.ResidueDigest) || !domainsecurity.IsSHA256Hex(record.AuthorityKeyID) ||
		!domainsecurity.IsSHA256Hex(record.TombstoneDigest) || semanticRetirementDigest(record) != record.TombstoneDigest ||
		record.NextOperation < 0 || record.OperationCount < 0 || record.NextOperation > record.OperationCount {
		return errors.New("semantic startup journal retirement tombstone is invalid")
	}
	if _, err := parseStrongDirectoryIdentity(record.JournalRootIdentity); err != nil {
		return errors.New("semantic startup journal retirement identity is invalid")
	}
	return validateSemanticRetirementFrontier(record)
}

func validateSemanticRetirementFrontier(record semanticJournalRetirementV1) error {
	switch record.JournalState {
	case semanticJournalRetirementEmpty:
		if record.Disposition != semanticJournalRetirementRollback || record.SourceEntry != "" ||
			record.NextOperation != 0 || record.OperationCount != 0 {
			return errors.New("semantic startup empty journal retirement frontier is invalid")
		}
	case semanticJournalPreparing:
		if record.Disposition != semanticJournalRetirementRollback || record.SourceEntry == "" || record.NextOperation != 0 {
			return errors.New("semantic startup preparing journal retirement frontier is invalid")
		}
	case semanticJournalCommitted:
		if record.Disposition != semanticJournalRetirementPublish || record.SourceEntry != "journal.json" ||
			record.NextOperation != record.OperationCount {
			return errors.New("semantic startup committed journal retirement frontier is invalid")
		}
	default:
		return errors.New("semantic startup journal state is not retirement eligible")
	}
	return nil
}

func validateSemanticRetirementV1(record semanticJournalRetirementV1, authority *startupJournalAuthority) error {
	if record.SchemaVersion != 1 || !validLegacyObjectIdentityV3(record.JournalRootIdentity) ||
		!domainsecurity.IsSHA256Hex(record.RootBindingDigest) || !domainsecurity.IsSHA256Hex(record.ConfigurationDigest) ||
		!domainsecurity.IsSHA256Hex(record.JournalBodyDigest) || !domainsecurity.IsSHA256Hex(record.JournalDigest) ||
		!domainsecurity.IsSHA256Hex(record.PlanDigest) || !domainsecurity.IsSHA256Hex(record.ResidueDigest) ||
		!domainsecurity.IsSHA256Hex(record.AuthorityKeyID) || !domainsecurity.IsSHA256Hex(record.TombstoneDigest) ||
		semanticRetirementDigest(record) != record.TombstoneDigest || record.NextOperation < 0 ||
		record.OperationCount < 0 || record.NextOperation > record.OperationCount ||
		validateSemanticRetirementFrontier(record) != nil {
		return errors.New("semantic startup V1 journal retirement tombstone is invalid")
	}
	if strings.TrimSpace(record.AuthoritySignature) == "" || authority == nil ||
		authority.verifyDomain(
			semanticJournalRetirementV1SignatureDomain,
			record.AuthorityKeyID,
			semanticRetirementSigningBytes(record),
			record.AuthoritySignature,
		) != nil {
		return errors.New("semantic startup V1 journal retirement signature is invalid")
	}
	return nil
}

func validateSemanticRetirement(record semanticJournalRetirementV1, authority *startupJournalAuthority) error {
	if err := validateSemanticRetirementShape(record); err != nil {
		return err
	}
	if strings.TrimSpace(record.AuthoritySignature) == "" || authority == nil ||
		authority.verifyDomain(semanticJournalRetirementSignatureDomain, record.AuthorityKeyID, semanticRetirementSigningBytes(record), record.AuthoritySignature) != nil {
		return errors.New("semantic startup journal retirement signature is invalid")
	}
	return nil
}

func decodeSemanticRetirement(body []byte, authority *startupJournalAuthority) (semanticJournalRetirementV1, error) {
	if len(body) == 0 || len(body) > maxSemanticRetirementBytes {
		return semanticJournalRetirementV1{}, StartupResourceLimitError{Code: "retirement_bytes", Limit: maxSemanticRetirementBytes}
	}
	if err := validateSemanticControlJSON(body, maxSemanticRetirementBytes); err != nil {
		return semanticJournalRetirementV1{}, errors.New("semantic startup journal retirement JSON is invalid")
	}
	var envelope struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return semanticJournalRetirementV1{}, errors.New("semantic startup journal retirement version is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var record semanticJournalRetirementV1
	if err := decoder.Decode(&record); err != nil {
		return semanticJournalRetirementV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return semanticJournalRetirementV1{}, errors.New("semantic startup journal retirement contains trailing JSON")
	}
	switch envelope.SchemaVersion {
	case 1:
		if err := validateSemanticRetirementV1(record, authority); err != nil {
			return semanticJournalRetirementV1{}, err
		}
		return semanticJournalRetirementV1{}, authenticatedSemanticRetirementV1{
			record: record, blocker: semanticRetirementV1Blocker(),
		}
	case semanticJournalRetirementSchemaVersion:
		if err := validateSemanticRetirement(record, authority); err != nil {
			return semanticJournalRetirementV1{}, err
		}
	default:
		return semanticJournalRetirementV1{}, errors.New("semantic startup journal retirement schema version is unsupported")
	}
	return record, nil
}

func decodeSemanticRetirementForDirectory(
	body []byte,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	roots RootSet,
) (semanticJournalRetirementV1, bool, error) {
	record, err := decodeSemanticRetirement(body, authority)
	var legacy authenticatedSemanticRetirementV1
	if errors.As(err, &legacy) {
		migrated, migrationErr := migrateAuthenticatedSemanticRetirementV1(directory, legacy.record, authority, roots)
		return migrated, true, migrationErr
	}
	return record, false, err
}
