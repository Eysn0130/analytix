package securegeneration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	domainartifact "analytix.local/runtime-go/internal/domain/artifactgeneration"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	journalSchemaVersionV1 = 1
	defaultMaxFiles        = 4096
	defaultMaxFileBytes    = 512 << 20
	defaultMaxTotalBytes   = 2 << 30
	defaultMaxDepth        = 32
	maxJournalBytes        = 16 << 10
	commitMarkerFormatV1   = "immutable_generation_commit_v1"
)

var (
	ErrUnsupported             = errors.New("secure_generation_unsupported")
	ErrUnsafeRoot              = errors.New("secure_generation_unsafe_root")
	ErrInvalidInput            = errors.New("secure_generation_invalid_input")
	ErrResidue                 = errors.New("secure_generation_residue")
	ErrCleanupIndeterminate    = errors.New("secure_generation_cleanup_indeterminate")
	ErrRecoveryRequired        = errors.New("secure_generation_recovery_required")
	ErrCurrentGeneration       = errors.New("secure_generation_current_mismatch")
	ErrCommitIndeterminate     = errors.New("secure_generation_commit_indeterminate")
	ErrCommittedCleanupPending = errors.New("secure_generation_committed_cleanup_pending")
)

type CommitState string

const (
	NotCommitted  CommitState = "not_committed"
	Committed     CommitState = "committed"
	Indeterminate CommitState = "indeterminate"
)

type Limits struct {
	MaxFiles      int
	MaxFileBytes  int64
	MaxTotalBytes int64
	MaxDepth      int
}

type PublishResult struct {
	State   CommitState
	Receipt domainartifact.ReceiptV1
}

type Observation struct {
	Installed bool
	Current   domainartifact.ReceiptV1
	Previous  *domainartifact.ReceiptV1
}

// DiscardCurrentResult reports what descriptor-pinned state was removed by a
// cleanup-only reconciliation. Installed is false only when the generation
// root was present and empty. A successful result never authorizes a caller to
// treat an earlier publication attempt as successful.
type DiscardCurrentResult struct {
	Installed     bool
	Current       domainartifact.ReceiptV1
	ReceiptDigest string
}

type ExpectedCurrentKind string

const (
	ExpectedCurrentAbsent     ExpectedCurrentKind = "absent"
	ExpectedCurrentGeneration ExpectedCurrentKind = "generation"
)

// ExpectedCurrent is checked while the root lock is held and before a
// publication creates transaction residue. Repeating an already-installed
// byte-identical generation remains an idempotent success.
type ExpectedCurrent struct {
	Kind         ExpectedCurrentKind
	GenerationID string
}

// Store owns one public immutable generation and, after a successful update,
// its exact predecessor. All names are relative to an identity-pinned private
// root; callers never provide stage, journal, or previous paths.
type Store struct {
	root         rootAuthority
	publicName   string
	previousName string
	journalName  string
	limits       Limits
	gate         chan struct{}
	faults       *faultPlan
}

type faultPlan struct {
	cut   func(string) error
	fsync func(string, int) error
}

type generationRecord struct {
	inventory      domainartifact.InventoryV1
	receipt        domainartifact.ReceiptV1
	inventoryBytes []byte
	receiptBytes   []byte
}

type journalV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	OperationID             string `json:"operationId"`
	PublicName              string `json:"publicName"`
	StageName               string `json:"stageName"`
	RetainedName            string `json:"retainedName"`
	CommitName              string `json:"commitName"`
	HadPrior                bool   `json:"hadPrior"`
	PriorGenerationID       string `json:"priorGenerationId"`
	PriorInventoryDigest    string `json:"priorInventoryDigest"`
	PriorReceiptDigest      string `json:"priorReceiptDigest"`
	HadRetainedPrevious     bool   `json:"hadRetainedPrevious"`
	RetainedGenerationID    string `json:"retainedGenerationId"`
	RetainedInventoryDigest string `json:"retainedInventoryDigest"`
	RetainedReceiptDigest   string `json:"retainedReceiptDigest"`
	NextGenerationID        string `json:"nextGenerationId"`
	NextInventoryDigest     string `json:"nextInventoryDigest"`
	NextReceiptDigest       string `json:"nextReceiptDigest"`
}

var journalFieldsV1 = [...]string{
	"schemaVersion", "operationId", "publicName", "stageName", "retainedName", "commitName", "hadPrior",
	"priorGenerationId", "priorInventoryDigest", "priorReceiptDigest",
	"hadRetainedPrevious", "retainedGenerationId", "retainedInventoryDigest", "retainedReceiptDigest",
	"nextGenerationId", "nextInventoryDigest", "nextReceiptDigest",
}

type commitMarkerV1 struct {
	SchemaVersion int       `json:"schemaVersion"`
	Format        string    `json:"format"`
	Journal       journalV1 `json:"journal"`
}

var commitMarkerFieldsV1 = [...]string{"schemaVersion", "format", "journal"}

func Open(root, publicName string, limits Limits) (*Store, error) {
	return openStore(root, publicName, limits, true)
}

// OpenExisting pins an already-created private store root without creating a
// missing path. CAS updates use it so a stale/missing-current request has no
// directory-creation side effect.
func OpenExisting(root, publicName string, limits Limits) (*Store, error) {
	return openStore(root, publicName, limits, false)
}

func openStore(root, publicName string, limits Limits, create bool) (*Store, error) {
	limits = normalizeLimits(limits)
	if !validStoreConfiguration(publicName, limits) {
		return nil, ErrInvalidInput
	}
	authority, err := openRootAuthority(root, create)
	if err != nil {
		return nil, err
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &Store{
		root: authority, publicName: publicName,
		previousName: "." + publicName + ".previous",
		journalName:  "." + publicName + ".journal.v1.json",
		limits:       limits, gate: gate,
	}, nil
}

func openStoreUnder(parent *os.File, rootName, publicName string, limits Limits, create bool) (*Store, error) {
	limits = normalizeLimits(limits)
	if parent == nil || !validPublicName(rootName) || !validStoreConfiguration(publicName, limits) {
		return nil, ErrInvalidInput
	}
	authority, err := openRootAuthorityUnder(parent, rootName, create)
	if err != nil {
		return nil, err
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &Store{
		root: authority, publicName: publicName,
		previousName: "." + publicName + ".previous",
		journalName:  "." + publicName + ".journal.v1.json",
		limits:       limits, gate: gate,
	}, nil
}

// PublishExpectedUnder performs one descriptor-anchored publication. The
// caller-selected parent path is never re-resolved; rootName and publicName
// are closed single-component names and the exact parent descriptor remains
// authoritative through the transaction's stable readback. The returned
// receipt is that transaction proof; a later ObserveUnder call is an
// independent observation and does not extend the publication's root identity.
func PublishExpectedUnder(
	ctx context.Context,
	parent *os.File,
	rootName string,
	publicName string,
	prepared domainartifact.PreparedV1,
	expected ExpectedCurrent,
	limits Limits,
) (PublishResult, error) {
	limits = normalizeLimits(limits)
	if parent == nil || !validPublicName(rootName) || !validStoreConfiguration(publicName, limits) ||
		prepared.Validate() != nil || !preparedWithinLimits(prepared, limits) || !validExpectedCurrent(expected) {
		return PublishResult{State: NotCommitted}, ErrInvalidInput
	}
	if err := contextError(ctx); err != nil {
		return PublishResult{State: NotCommitted}, err
	}
	store, err := openStoreUnder(
		parent,
		rootName,
		publicName,
		limits,
		expected.Kind == ExpectedCurrentAbsent,
	)
	if err != nil {
		if errors.Is(err, ErrCleanupIndeterminate) {
			return PublishResult{State: Indeterminate, Receipt: prepared.Receipt()}, errors.Join(ErrCommitIndeterminate, err)
		}
		return PublishResult{State: NotCommitted}, err
	}
	result, publishErr := store.PublishExpected(ctx, prepared, expected)
	if result.State == NotCommitted && rootAuthorityCreatedByCall(store.root) {
		if cleanupErr := rollbackCreatedRootAuthority(store.root); cleanupErr != nil {
			result = PublishResult{State: Indeterminate, Receipt: prepared.Receipt()}
			publishErr = errors.Join(ErrCommitIndeterminate, publishErr, cleanupErr)
		}
	}
	closeErr := closeRootAuthority(store.root)
	return result, errors.Join(publishErr, closeErr)
}

// ObserveUnder reopens one descriptor-anchored generation root without
// creating any path or recovery residue.
func ObserveUnder(
	ctx context.Context,
	parent *os.File,
	rootName string,
	publicName string,
	limits Limits,
) (Observation, error) {
	store, err := openStoreUnder(parent, rootName, publicName, limits, false)
	if err != nil {
		return Observation{}, err
	}
	observation, observeErr := store.Observe(ctx)
	return observation, errors.Join(observeErr, closeRootAuthority(store.root))
}

// DiscardExpectedUnder destroys one exact disposable generation and its empty
// descriptor-pinned root. It never performs recovery or guesses which
// generation won: any predecessor, journal, stage, receipt mismatch, or
// durability ambiguity fails closed for whole-parent quarantine by the caller.
func DiscardExpectedUnder(
	ctx context.Context,
	parent *os.File,
	rootName string,
	publicName string,
	expected domainartifact.ReceiptV1,
	limits Limits,
) error {
	limits = normalizeLimits(limits)
	if parent == nil || !validPublicName(rootName) || !validStoreConfiguration(publicName, limits) ||
		!validExpectedReceipt(expected, limits) {
		return ErrInvalidInput
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	store, err := openStoreUnder(parent, rootName, publicName, limits, false)
	if err != nil {
		return err
	}
	discardErr := discardExpectedUnder(store, ctx, expected)
	return errors.Join(discardErr, closeRootAuthority(store.root))
}

// DiscardCurrentUnder removes either one clean current generation or an empty
// descriptor-pinned root. It is intentionally a cleanup primitive: callers do
// not provide a generation identity and must not use its result to upgrade an
// indeterminate create/publish operation to success. Any predecessor,
// transaction residue, unknown entry, or durability ambiguity fails closed.
func DiscardCurrentUnder(
	ctx context.Context,
	parent *os.File,
	rootName string,
	publicName string,
	limits Limits,
) (DiscardCurrentResult, error) {
	limits = normalizeLimits(limits)
	if parent == nil || !validPublicName(rootName) || !validStoreConfiguration(publicName, limits) {
		return DiscardCurrentResult{}, ErrInvalidInput
	}
	if err := contextError(ctx); err != nil {
		return DiscardCurrentResult{}, err
	}
	store, err := openStoreUnder(parent, rootName, publicName, limits, false)
	if err != nil {
		return DiscardCurrentResult{}, err
	}
	result, discardErr := discardCurrentUnder(store, ctx)
	if err := errors.Join(discardErr, closeRootAuthority(store.root)); err != nil {
		return DiscardCurrentResult{}, err
	}
	return result, nil
}

func (store *Store) Observe(ctx context.Context) (Observation, error) {
	if store == nil {
		return Observation{}, ErrInvalidInput
	}
	if err := store.acquire(ctx); err != nil {
		return Observation{}, err
	}
	defer store.release()
	return observe(store, ctx)
}

func (store *Store) Publish(ctx context.Context, prepared domainartifact.PreparedV1) (PublishResult, error) {
	if store == nil || prepared.Validate() != nil || !store.withinLimits(prepared) {
		return PublishResult{State: NotCommitted}, ErrInvalidInput
	}
	if err := store.acquire(ctx); err != nil {
		return PublishResult{State: NotCommitted}, err
	}
	defer store.release()
	return publish(store, ctx, prepared, nil)
}

func (store *Store) PublishExpected(
	ctx context.Context,
	prepared domainartifact.PreparedV1,
	expected ExpectedCurrent,
) (PublishResult, error) {
	if store == nil || prepared.Validate() != nil || !store.withinLimits(prepared) || !validExpectedCurrent(expected) {
		return PublishResult{State: NotCommitted}, ErrInvalidInput
	}
	if err := store.acquire(ctx); err != nil {
		return PublishResult{State: NotCommitted}, err
	}
	defer store.release()
	return publish(store, ctx, prepared, &expected)
}

// Recover rolls a journal-only publication back to its exact full pre-state.
// A durable commit marker is the sole authority to finish post-commit GC; the
// local generation layout alone never selects a winner. Unknown, malformed,
// or multiple residues are returned unchanged for operator quarantine.
func (store *Store) Recover(ctx context.Context) (Observation, error) {
	if store == nil {
		return Observation{}, ErrInvalidInput
	}
	if err := store.acquire(ctx); err != nil {
		return Observation{}, err
	}
	defer store.release()
	return recoverStore(store, ctx)
}

func (store *Store) acquire(ctx context.Context) error {
	if ctx == nil {
		<-store.gate
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-store.gate:
		return nil
	}
}

func (store *Store) release() { store.gate <- struct{}{} }

func (store *Store) withinLimits(prepared domainartifact.PreparedV1) bool {
	return store != nil && preparedWithinLimits(prepared, store.limits)
}

func preparedWithinLimits(prepared domainartifact.PreparedV1, limits Limits) bool {
	inventory := prepared.Inventory()
	if int(inventory.FileCount) > limits.MaxFiles || inventory.TotalBytes > uint64(limits.MaxTotalBytes) {
		return false
	}
	for _, entry := range inventory.Files {
		if entry.Size > uint64(limits.MaxFileBytes) || strings.Count(entry.Path, "/")+1 > limits.MaxDepth {
			return false
		}
	}
	return true
}

func normalizeLimits(limits Limits) Limits {
	if limits.MaxFiles == 0 {
		limits.MaxFiles = defaultMaxFiles
	}
	if limits.MaxFileBytes == 0 {
		limits.MaxFileBytes = defaultMaxFileBytes
	}
	if limits.MaxTotalBytes == 0 {
		limits.MaxTotalBytes = defaultMaxTotalBytes
	}
	if limits.MaxDepth == 0 {
		limits.MaxDepth = defaultMaxDepth
	}
	return limits
}

func validStoreConfiguration(publicName string, limits Limits) bool {
	return validPublicName(publicName) && limits.MaxFiles > 0 && limits.MaxFiles <= domainartifact.MaxFilesV1 &&
		limits.MaxFileBytes > 0 && limits.MaxTotalBytes > 0 && limits.MaxTotalBytes <= domainartifact.MaxTotalBytesV1 &&
		limits.MaxFileBytes <= limits.MaxTotalBytes && limits.MaxDepth > 0 && limits.MaxDepth <= 128
}

func validPublicName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, "/") || len(name) > domainartifact.MaxComponentBytesV1 {
		return false
	}
	return domainartifact.ValidatePayloadPathV1(name) == nil && name != domainartifact.InventoryFileNameV1 && name != domainartifact.ReceiptFileNameV1
}

func validExpectedCurrent(expected ExpectedCurrent) bool {
	switch expected.Kind {
	case ExpectedCurrentAbsent:
		return expected.GenerationID == ""
	case ExpectedCurrentGeneration:
		return domainartifact.ValidDigestV1(expected.GenerationID)
	default:
		return false
	}
}

func validExpectedReceipt(receipt domainartifact.ReceiptV1, limits Limits) bool {
	return receipt.SchemaVersion == domainartifact.ReceiptSchemaVersionV1 &&
		receipt.Format == domainartifact.ReceiptFormatV1 &&
		domainartifact.ValidDigestV1(receipt.GenerationID) &&
		domainartifact.ValidDigestV1(receipt.InventoryDigest) &&
		receipt.DirectoryMode == domainartifact.DirectoryModeV1 &&
		receipt.InventoryType == domainartifact.FileTypeRegularV1 &&
		receipt.InventoryMode == domainartifact.RegularFileModeV1 &&
		receipt.ReceiptType == domainartifact.FileTypeRegularV1 &&
		receipt.ReceiptMode == domainartifact.RegularFileModeV1 &&
		receipt.FileCount <= uint64(limits.MaxFiles) && receipt.TotalBytes <= uint64(limits.MaxTotalBytes)
}

func newJournalV1(publicName, operationID string, prior, retained *generationRecord, next generationRecord) (journalV1, error) {
	journal := journalV1{
		SchemaVersion:       journalSchemaVersionV1,
		OperationID:         operationID,
		PublicName:          publicName,
		StageName:           "." + publicName + ".stage-" + operationID,
		CommitName:          "." + publicName + ".commit-" + operationID + ".v1.json",
		NextGenerationID:    next.receipt.GenerationID,
		NextInventoryDigest: domainartifact.DigestBytesV1(next.inventoryBytes),
		NextReceiptDigest:   domainartifact.DigestBytesV1(next.receiptBytes),
	}
	if prior != nil {
		journal.HadPrior = true
		journal.PriorGenerationID = prior.receipt.GenerationID
		journal.PriorInventoryDigest = domainartifact.DigestBytesV1(prior.inventoryBytes)
		journal.PriorReceiptDigest = domainartifact.DigestBytesV1(prior.receiptBytes)
	}
	if retained != nil {
		journal.HadRetainedPrevious = true
		journal.RetainedName = "." + publicName + ".third-" + operationID
		journal.RetainedGenerationID = retained.receipt.GenerationID
		journal.RetainedInventoryDigest = domainartifact.DigestBytesV1(retained.inventoryBytes)
		journal.RetainedReceiptDigest = domainartifact.DigestBytesV1(retained.receiptBytes)
	}
	if err := validateJournalV1(journal, publicName); err != nil {
		return journalV1{}, err
	}
	return journal, nil
}

func canonicalJournalBytesV1(journal journalV1, publicName string) ([]byte, error) {
	if err := validateJournalV1(journal, publicName); err != nil {
		return nil, err
	}
	return json.Marshal(journal)
}

func parseJournalV1(body []byte, publicName string) (journalV1, error) {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxJournalBytes, MaxDepth: 4, MaxTokens: 64, MaxStringBytes: 512,
	})
	if err != nil || len(object) != len(journalFieldsV1) {
		return journalV1{}, errors.Join(ErrResidue, err)
	}
	for _, field := range journalFieldsV1 {
		if raw, ok := object[field]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return journalV1{}, ErrResidue
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var journal journalV1
	if err := decoder.Decode(&journal); err != nil {
		return journalV1{}, errors.Join(ErrResidue, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return journalV1{}, ErrResidue
	}
	canonical, err := json.Marshal(journal)
	if err != nil || !bytes.Equal(canonical, body) || validateJournalV1(journal, publicName) != nil {
		return journalV1{}, errors.Join(ErrResidue, err)
	}
	return journal, nil
}

func validateJournalV1(journal journalV1, publicName string) error {
	if journal.SchemaVersion != journalSchemaVersionV1 || journal.PublicName != publicName ||
		!validOperationID(journal.OperationID) || journal.StageName != "."+publicName+".stage-"+journal.OperationID ||
		journal.CommitName != "."+publicName+".commit-"+journal.OperationID+".v1.json" ||
		!domainartifact.ValidDigestV1(journal.NextGenerationID) || !domainartifact.ValidDigestV1(journal.NextInventoryDigest) ||
		!domainartifact.ValidDigestV1(journal.NextReceiptDigest) {
		return ErrResidue
	}
	if journal.HadPrior {
		if !domainartifact.ValidDigestV1(journal.PriorGenerationID) || !domainartifact.ValidDigestV1(journal.PriorInventoryDigest) ||
			!domainartifact.ValidDigestV1(journal.PriorReceiptDigest) {
			return ErrResidue
		}
	} else if journal.PriorGenerationID != "" || journal.PriorInventoryDigest != "" || journal.PriorReceiptDigest != "" {
		return ErrResidue
	}
	if journal.HadRetainedPrevious {
		if !journal.HadPrior || !domainartifact.ValidDigestV1(journal.RetainedGenerationID) ||
			!domainartifact.ValidDigestV1(journal.RetainedInventoryDigest) || !domainartifact.ValidDigestV1(journal.RetainedReceiptDigest) ||
			journal.RetainedGenerationID == journal.PriorGenerationID || journal.RetainedName != "."+publicName+".third-"+journal.OperationID {
			return ErrResidue
		}
	} else if journal.RetainedName != "" || journal.RetainedGenerationID != "" || journal.RetainedInventoryDigest != "" || journal.RetainedReceiptDigest != "" {
		return ErrResidue
	}
	return nil
}

func canonicalCommitMarkerBytesV1(journal journalV1, publicName string) ([]byte, error) {
	if err := validateJournalV1(journal, publicName); err != nil {
		return nil, err
	}
	return json.Marshal(commitMarkerV1{SchemaVersion: journalSchemaVersionV1, Format: commitMarkerFormatV1, Journal: journal})
}

func parseCommitMarkerV1(body []byte, publicName string) (commitMarkerV1, error) {
	object, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxJournalBytes, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 512,
	})
	if err != nil || len(object) != len(commitMarkerFieldsV1) {
		return commitMarkerV1{}, errors.Join(ErrResidue, err)
	}
	for _, field := range commitMarkerFieldsV1 {
		if raw, ok := object[field]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return commitMarkerV1{}, ErrResidue
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var marker commitMarkerV1
	if err := decoder.Decode(&marker); err != nil {
		return commitMarkerV1{}, errors.Join(ErrResidue, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return commitMarkerV1{}, ErrResidue
	}
	canonical, err := canonicalCommitMarkerBytesV1(marker.Journal, publicName)
	if err != nil || marker.SchemaVersion != journalSchemaVersionV1 || marker.Format != commitMarkerFormatV1 || !bytes.Equal(canonical, body) {
		return commitMarkerV1{}, errors.Join(ErrResidue, err)
	}
	return marker, nil
}

func validOperationID(value string) bool {
	if len(value) != 24 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func operationID() (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func contextDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return make(chan struct{})
	}
	return ctx.Done()
}
