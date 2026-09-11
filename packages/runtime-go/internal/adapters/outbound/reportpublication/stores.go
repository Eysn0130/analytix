package reportpublication

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

const (
	maxPublicationAttemptBytes    = 512 << 10
	maxPublicationReceiptBytes    = 512 << 10
	maxPublicationCommitBytes     = 512 << 10
	maxPublicationSelectionBytes  = 1 << 20
	maxPublicationDecisionBytes   = 512 << 10
	maxReportGrantSettlementBytes = 512 << 10
	maxReportStageCompletionBytes = 512 << 10
	maxReportDeliveryOutcomeBytes = 512 << 10
	maxPublicationIndexBytes      = 256 << 10
	maxPublicationLedgerBytes     = 8 << 20
	maxPublicationProjectionBytes = 128 << 10
	maxPublishedArtifactBytes     = 16 << 20
)

type AttemptStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type ReceiptStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type CommitReceiptStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type CommitSelectionStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type DeliveryDecisionStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type ReportGrantSettlementStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type ReportStageCompletionStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type ReportDeliveryOutcomeStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type IndexStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type ClaimLedgerStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type PIIProjectionStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type RenderInspectionStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}
type ArtifactStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

type Stores struct {
	Attempts         *AttemptStore
	Receipts         *ReceiptStore
	Commits          *CommitReceiptStore
	Selections       *CommitSelectionStore
	Decisions        *DeliveryDecisionStore
	GrantSettlements *ReportGrantSettlementStore
	StageCompletions *ReportStageCompletionStore
	DeliveryOutcomes *ReportDeliveryOutcomeStore
	Indexes          *IndexStore
	Ledgers          *ClaimLedgerStore
	PIIProjections   *PIIProjectionStore
	Inspections      *RenderInspectionStore
	Artifacts        *ArtifactStore

	closeOnce sync.Once
	closeErr  error
	owners    []*finalauthorityadapter.SecurePrivateCAS
}

type publicationCASVisitor interface {
	Visit(context.Context, func(finalauthorityadapter.SecurePrivateCASFile) error) error
}

var _ publicationport.AttemptStore = (*AttemptStore)(nil)
var _ publicationport.AttemptInventoryStore = (*AttemptStore)(nil)
var _ publicationport.ReceiptStore = (*ReceiptStore)(nil)
var _ publicationport.ReceiptInventoryStore = (*ReceiptStore)(nil)
var _ publicationport.CommitReceiptStore = (*CommitReceiptStore)(nil)
var _ publicationport.CommitReceiptInventoryStore = (*CommitReceiptStore)(nil)
var _ publicationport.CommitSelectionStore = (*CommitSelectionStore)(nil)
var _ publicationport.CommitSelectionInventoryStore = (*CommitSelectionStore)(nil)
var _ publicationport.DeliveryDecisionStore = (*DeliveryDecisionStore)(nil)
var _ publicationport.DeliveryDecisionInventoryStore = (*DeliveryDecisionStore)(nil)
var _ publicationport.ReportGrantSettlementStore = (*ReportGrantSettlementStore)(nil)
var _ publicationport.ReportGrantSettlementInventoryStore = (*ReportGrantSettlementStore)(nil)
var _ publicationport.ReportStageCompletionStore = (*ReportStageCompletionStore)(nil)
var _ publicationport.ReportStageCompletionInventoryStore = (*ReportStageCompletionStore)(nil)
var _ publicationport.DeliveryOutcomeStore = (*ReportDeliveryOutcomeStore)(nil)
var _ publicationport.DeliveryOutcomeInventoryStore = (*ReportDeliveryOutcomeStore)(nil)
var _ publicationport.IndexStore = (*IndexStore)(nil)
var _ publicationport.IndexInventoryStore = (*IndexStore)(nil)
var _ publicationport.ClaimLedgerStore = (*ClaimLedgerStore)(nil)
var _ publicationport.PIIProjectionStore = (*PIIProjectionStore)(nil)
var _ publicationport.RenderInspectionStore = (*RenderInspectionStore)(nil)
var _ publicationport.ArtifactStore = (*ArtifactStore)(nil)
var _ publicationport.ControlledArtifactMetadataStore = (*ArtifactStore)(nil)

func NewStores(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (_ *Stores, resultErr error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("publication authority owner root is invalid")
	}
	stores := &Stores{}
	complete := false
	defer func() {
		if !complete {
			resultErr = errors.Join(resultErr, stores.Close())
		}
	}()
	var err error
	stores.Attempts, err = newAttemptStore(filepath.Join(root, "attempts"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Attempts.cas)
	stores.Receipts, err = newReceiptStore(filepath.Join(root, "receipts"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Receipts.cas)
	stores.Commits, err = newCommitReceiptStore(filepath.Join(root, "commit-receipts"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Commits.cas)
	stores.Selections, err = newCommitSelectionStore(filepath.Join(root, "commit-selections"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Selections.cas)
	stores.Decisions, err = newDeliveryDecisionStore(filepath.Join(root, "delivery-decisions"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Decisions.cas)
	stores.GrantSettlements, err = newReportGrantSettlementStore(filepath.Join(root, "grant-settlements"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.GrantSettlements.cas)
	stores.StageCompletions, err = newReportStageCompletionStore(filepath.Join(root, "stage-completions"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.StageCompletions.cas)
	// The on-disk leaf name is retained for byte-compatible recovery of V1
	// projection files. Its logical contract now admits exactly one projected
	// or rejected outcome at the same completion-derived key.
	stores.DeliveryOutcomes, err = newReportDeliveryOutcomeStore(filepath.Join(root, "delivery-projections"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.DeliveryOutcomes.cas)
	stores.Indexes, err = newIndexStore(filepath.Join(root, "indexes"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Indexes.cas)
	stores.Ledgers, err = newClaimLedgerStore(filepath.Join(root, "claim-ledgers"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Ledgers.cas)
	stores.PIIProjections, err = newPIIProjectionStore(filepath.Join(root, "pii-projections"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.PIIProjections.cas)
	stores.Inspections, err = newRenderInspectionStore(filepath.Join(root, "render-inspections"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Inspections.cas)
	stores.Artifacts, err = newArtifactStore(filepath.Join(root, "artifacts"), access)
	if err != nil {
		return nil, err
	}
	stores.owners = append(stores.owners, stores.Artifacts.cas)
	complete = true
	return stores, nil
}

// Close releases every private-CAS root-generation reference in reverse
// construction order. It is safe to call on partial construction and is
// idempotent so one lifecycle owner can use it on every failure/shutdown path.
func (stores *Stores) Close() error {
	if stores == nil {
		return nil
	}
	stores.closeOnce.Do(func() {
		for index := len(stores.owners) - 1; index >= 0; index-- {
			stores.closeErr = errors.Join(stores.closeErr, stores.owners[index].Close())
		}
	})
	return stores.closeErr
}

func (stores *Stores) HasRecords(ctx context.Context) (bool, error) {
	if stores == nil || stores.Attempts == nil || stores.Receipts == nil || stores.Commits == nil || stores.Selections == nil || stores.Decisions == nil || stores.GrantSettlements == nil || stores.StageCompletions == nil || stores.DeliveryOutcomes == nil || stores.Indexes == nil || stores.Ledgers == nil ||
		stores.PIIProjections == nil || stores.Inspections == nil || stores.Artifacts == nil {
		return false, errors.New("publication authority stores are unavailable")
	}
	for _, cas := range []publicationCASVisitor{
		stores.Attempts.cas, stores.Receipts.cas, stores.Commits.cas, stores.Selections.cas, stores.Decisions.cas,
		stores.GrantSettlements.cas, stores.StageCompletions.cas, stores.DeliveryOutcomes.cas, stores.Indexes.cas, stores.Ledgers.cas,
		stores.PIIProjections.cas, stores.Inspections.cas, stores.Artifacts.cas,
	} {
		found := errors.New("publication authority record found")
		err := cas.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
		if errors.Is(err, found) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

func newAttemptStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*AttemptStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationAttemptBytes, access)
	return &AttemptStore{cas: cas}, err
}
func newReceiptStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ReceiptStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationReceiptBytes, access)
	return &ReceiptStore{cas: cas}, err
}
func newCommitReceiptStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*CommitReceiptStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationCommitBytes, access)
	return &CommitReceiptStore{cas: cas}, err
}
func newCommitSelectionStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*CommitSelectionStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationSelectionBytes, access)
	return &CommitSelectionStore{cas: cas}, err
}
func newDeliveryDecisionStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*DeliveryDecisionStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationDecisionBytes, access)
	return &DeliveryDecisionStore{cas: cas}, err
}
func newReportGrantSettlementStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ReportGrantSettlementStore, error) {
	cas, err := openPublicationCAS(root, maxReportGrantSettlementBytes, access)
	return &ReportGrantSettlementStore{cas: cas}, err
}
func newReportStageCompletionStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ReportStageCompletionStore, error) {
	cas, err := openPublicationCAS(root, maxReportStageCompletionBytes, access)
	return &ReportStageCompletionStore{cas: cas}, err
}
func newReportDeliveryOutcomeStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ReportDeliveryOutcomeStore, error) {
	cas, err := openPublicationCAS(root, maxReportDeliveryOutcomeBytes, access)
	return &ReportDeliveryOutcomeStore{cas: cas}, err
}
func newIndexStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*IndexStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationIndexBytes, access)
	return &IndexStore{cas: cas}, err
}
func newClaimLedgerStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ClaimLedgerStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationLedgerBytes, access)
	return &ClaimLedgerStore{cas: cas}, err
}
func newPIIProjectionStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*PIIProjectionStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationProjectionBytes, access)
	return &PIIProjectionStore{cas: cas}, err
}
func newRenderInspectionStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*RenderInspectionStore, error) {
	cas, err := openPublicationCAS(root, maxPublicationProjectionBytes, access)
	return &RenderInspectionStore{cas: cas}, err
}
func newArtifactStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ArtifactStore, error) {
	cas, err := openPublicationCAS(root, maxPublishedArtifactBytes, access)
	return &ArtifactStore{cas: cas}, err
}

func (store *AttemptStore) CreateExclusive(ctx context.Context, attempt domainpublication.PublicationAttemptV1) (bool, error) {
	body, err := domainpublication.PublicationAttemptV1Bytes(attempt)
	if err != nil {
		return false, err
	}
	if store == nil || store.cas == nil {
		return false, errors.New("publication attempt store is unavailable")
	}
	if current, readErr := store.cas.Read(ctx, attempt.AttemptID); readErr == nil {
		parsed, parseErr := domainpublication.ParsePublicationAttemptV1(current)
		if parseErr != nil || parsed.AttemptID != attempt.AttemptID || !bytes.Equal(current, body) {
			return false, errors.New("publication attempt conflicts with the reserved report-stage identity")
		}
		return false, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	if err := store.cas.PutIfAbsent(ctx, attempt.AttemptID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return false, err
		}
		current, readErr := store.cas.Read(ctx, attempt.AttemptID)
		if readErr != nil {
			return false, readErr
		}
		parsed, parseErr := domainpublication.ParsePublicationAttemptV1(current)
		if parseErr != nil || parsed.AttemptID != attempt.AttemptID || !bytes.Equal(current, body) {
			return false, errors.New("publication attempt conflicts with the reserved report-stage identity")
		}
		return false, nil
	}
	written, err := store.cas.Read(ctx, attempt.AttemptID)
	if err != nil || !bytes.Equal(written, body) {
		return false, errors.New("publication attempt write readback failed")
	}
	parsed, err := domainpublication.ParsePublicationAttemptV1(written)
	if err != nil || parsed.AttemptID != attempt.AttemptID || parsed.RecordDigest != attempt.RecordDigest {
		return false, errors.New("publication attempt write semantic verification failed")
	}
	return true, nil
}

func (store *AttemptStore) Resolve(ctx context.Context, attemptID string) (domainpublication.PublicationAttemptV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, attemptID)
	if err != nil {
		return domainpublication.PublicationAttemptV1{}, err
	}
	attempt, err := domainpublication.ParsePublicationAttemptV1(body)
	if err != nil || attempt.AttemptID != attemptID {
		return domainpublication.PublicationAttemptV1{}, errors.New("publication attempt identity mismatch")
	}
	return attempt, nil
}

func (store *AttemptStore) VisitAttempts(ctx context.Context, visit func(domainpublication.PublicationAttemptV1) error) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("publication attempt inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		attempt, err := domainpublication.ParsePublicationAttemptV1(file.Body)
		if err != nil || attempt.AttemptID != file.Digest {
			return errors.New("publication attempt inventory is corrupt")
		}
		return visit(attempt)
	})
}

func (store *ReceiptStore) PutIfAbsent(ctx context.Context, receipt domainpublication.PublicationReceiptV1) error {
	body, err := domainpublication.PublicationReceiptV1Bytes(receipt)
	if err != nil {
		return err
	}
	return putSemanticRecord(ctx, store.cas, receipt.RecordDigest, body, func(raw []byte) (string, error) {
		parsed, parseErr := domainpublication.ParsePublicationReceiptV1(raw)
		return parsed.RecordDigest, parseErr
	})
}
func (store *ReceiptStore) Resolve(ctx context.Context, digest string) (domainpublication.PublicationReceiptV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, digest)
	if err != nil {
		return domainpublication.PublicationReceiptV1{}, err
	}
	receipt, err := domainpublication.ParsePublicationReceiptV1(body)
	if err != nil || receipt.RecordDigest != digest {
		return domainpublication.PublicationReceiptV1{}, errors.New("publication receipt content address mismatch")
	}
	return receipt, nil
}

func (store *ReceiptStore) VisitReceipts(ctx context.Context, visit func(domainpublication.PublicationReceiptV1) error) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("publication receipt inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainpublication.ParsePublicationReceiptV1(file.Body)
		if err != nil || receipt.RecordDigest != file.Digest {
			return errors.New("publication receipt inventory is corrupt")
		}
		return visit(receipt)
	})
}

func (store *CommitReceiptStore) PutIfAbsent(ctx context.Context, receipt domainpublication.PublicationCommitReceiptV1) error {
	body, err := domainpublication.PublicationCommitReceiptV1Bytes(receipt)
	if err != nil {
		return err
	}
	return putSemanticRecord(ctx, store.cas, receipt.RecordDigest, body, func(raw []byte) (string, error) {
		parsed, parseErr := domainpublication.ParsePublicationCommitReceiptV1(raw)
		return parsed.RecordDigest, parseErr
	})
}

func (store *CommitReceiptStore) Resolve(ctx context.Context, digest string) (domainpublication.PublicationCommitReceiptV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, digest)
	if err != nil {
		return domainpublication.PublicationCommitReceiptV1{}, err
	}
	receipt, err := domainpublication.ParsePublicationCommitReceiptV1(body)
	if err != nil || receipt.RecordDigest != digest {
		return domainpublication.PublicationCommitReceiptV1{}, errors.New("publication commit receipt content address mismatch")
	}
	return receipt, nil
}

func (store *CommitReceiptStore) VisitCommitReceipts(ctx context.Context, visit func(domainpublication.PublicationCommitReceiptV1) error) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("publication commit receipt inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainpublication.ParsePublicationCommitReceiptV1(file.Body)
		if err != nil || receipt.RecordDigest != file.Digest {
			return errors.New("publication commit receipt inventory is corrupt")
		}
		return visit(receipt)
	})
}

func (store *CommitSelectionStore) CreateExclusive(
	ctx context.Context,
	selection domainpublication.PublicationCommitSelectionV1,
) (bool, error) {
	body, err := domainpublication.PublicationCommitSelectionV1Bytes(selection)
	if err != nil {
		return false, err
	}
	if store == nil || store.cas == nil {
		return false, errors.New("publication commit selection store is unavailable")
	}
	if current, readErr := store.cas.Read(ctx, selection.SelectionID); readErr == nil {
		parsed, parseErr := domainpublication.ParsePublicationCommitSelectionV1(current)
		if parseErr != nil || parsed.SelectionID != selection.SelectionID {
			return false, errors.New("publication commit selection winner is corrupt")
		}
		return false, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	if err := store.cas.PutIfAbsent(ctx, selection.SelectionID, body); err != nil {
		return false, err
	}
	written, err := store.cas.Read(ctx, selection.SelectionID)
	if err != nil {
		return false, err
	}
	parsed, parseErr := domainpublication.ParsePublicationCommitSelectionV1(written)
	if parseErr != nil || parsed.SelectionID != selection.SelectionID {
		return false, errors.New("publication commit selection readback is corrupt")
	}
	return bytes.Equal(written, body), nil
}

func (store *CommitSelectionStore) Resolve(
	ctx context.Context,
	selectionID string,
) (domainpublication.PublicationCommitSelectionV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, selectionID)
	if err != nil {
		return domainpublication.PublicationCommitSelectionV1{}, err
	}
	selection, err := domainpublication.ParsePublicationCommitSelectionV1(body)
	if err != nil || selection.SelectionID != selectionID {
		return domainpublication.PublicationCommitSelectionV1{}, errors.New("publication commit selection stable identity mismatch")
	}
	return selection, nil
}

func (store *CommitSelectionStore) VisitCommitSelections(
	ctx context.Context,
	visit func(domainpublication.PublicationCommitSelectionV1) error,
) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("publication commit selection inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		selection, err := domainpublication.ParsePublicationCommitSelectionV1(file.Body)
		if err != nil || selection.SelectionID != file.Digest {
			return errors.New("publication commit selection inventory is corrupt")
		}
		return visit(selection)
	})
}

func (store *DeliveryDecisionStore) CreateExclusive(
	ctx context.Context,
	decision domainpublication.ReportDeliveryDecisionV1,
) (bool, error) {
	body, err := domainpublication.ReportDeliveryDecisionV1Bytes(decision)
	if err != nil {
		return false, err
	}
	if store == nil || store.cas == nil {
		return false, errors.New("report delivery decision store is unavailable")
	}
	if current, readErr := store.cas.Read(ctx, decision.DecisionID); readErr == nil {
		parsed, parseErr := domainpublication.ParseReportDeliveryDecisionV1(current)
		if parseErr != nil || parsed.DecisionID != decision.DecisionID || !bytes.Equal(current, body) {
			return false, errors.New("report delivery decision conflicts with the admitted attempt identity")
		}
		return false, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	if err := store.cas.PutIfAbsent(ctx, decision.DecisionID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return false, err
		}
		current, readErr := store.cas.Read(ctx, decision.DecisionID)
		if readErr != nil {
			return false, readErr
		}
		parsed, parseErr := domainpublication.ParseReportDeliveryDecisionV1(current)
		if parseErr != nil || parsed.DecisionID != decision.DecisionID || !bytes.Equal(current, body) {
			return false, errors.New("report delivery decision conflicts with the admitted attempt identity")
		}
		return false, nil
	}
	written, err := store.cas.Read(ctx, decision.DecisionID)
	if err != nil || !bytes.Equal(written, body) {
		return false, errors.Join(errors.New("report delivery decision write readback failed"), err)
	}
	parsed, err := domainpublication.ParseReportDeliveryDecisionV1(written)
	if err != nil || parsed.DecisionID != decision.DecisionID || parsed.RecordDigest != decision.RecordDigest {
		return false, errors.Join(errors.New("report delivery decision write semantic verification failed"), err)
	}
	return true, nil
}

func (store *DeliveryDecisionStore) Resolve(
	ctx context.Context,
	decisionID string,
) (domainpublication.ReportDeliveryDecisionV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, decisionID)
	if err != nil {
		return domainpublication.ReportDeliveryDecisionV1{}, err
	}
	decision, err := domainpublication.ParseReportDeliveryDecisionV1(body)
	if err != nil || decision.DecisionID != decisionID {
		return domainpublication.ReportDeliveryDecisionV1{}, errors.New("report delivery decision stable identity mismatch")
	}
	return decision, nil
}

func (store *DeliveryDecisionStore) VisitDeliveryDecisions(
	ctx context.Context,
	visit func(domainpublication.ReportDeliveryDecisionV1) error,
) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("report delivery decision inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		decision, err := domainpublication.ParseReportDeliveryDecisionV1(file.Body)
		if err != nil || decision.DecisionID != file.Digest {
			return errors.New("report delivery decision inventory is corrupt")
		}
		return visit(decision)
	})
}

func (store *ReportGrantSettlementStore) CreateExclusive(
	ctx context.Context,
	settlement domainpublication.ReportGrantSettlementV1,
) (bool, error) {
	body, err := domainpublication.ReportGrantSettlementV1Bytes(settlement)
	if err != nil {
		return false, err
	}
	if store == nil || store.cas == nil {
		return false, errors.New("report grant settlement store is unavailable")
	}
	if current, readErr := store.cas.Read(ctx, settlement.SettlementID); readErr == nil {
		parsed, parseErr := domainpublication.ParseReportGrantSettlementV1(current)
		if parseErr != nil || parsed.SettlementID != settlement.SettlementID || !bytes.Equal(current, body) {
			return false, errors.New("report grant settlement conflicts with the decision and result identity")
		}
		return false, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	if err := store.cas.PutIfAbsent(ctx, settlement.SettlementID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return false, err
		}
		current, readErr := store.cas.Read(ctx, settlement.SettlementID)
		if readErr != nil {
			return false, readErr
		}
		parsed, parseErr := domainpublication.ParseReportGrantSettlementV1(current)
		if parseErr != nil || parsed.SettlementID != settlement.SettlementID || !bytes.Equal(current, body) {
			return false, errors.New("report grant settlement conflicts with the decision and result identity")
		}
		return false, nil
	}
	written, err := store.cas.Read(ctx, settlement.SettlementID)
	if err != nil || !bytes.Equal(written, body) {
		return false, errors.Join(errors.New("report grant settlement write readback failed"), err)
	}
	parsed, err := domainpublication.ParseReportGrantSettlementV1(written)
	if err != nil || parsed.SettlementID != settlement.SettlementID || parsed.RecordDigest != settlement.RecordDigest {
		return false, errors.Join(errors.New("report grant settlement write semantic verification failed"), err)
	}
	return true, nil
}

func (store *ReportGrantSettlementStore) Resolve(
	ctx context.Context,
	settlementID string,
) (domainpublication.ReportGrantSettlementV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, settlementID)
	if err != nil {
		return domainpublication.ReportGrantSettlementV1{}, err
	}
	settlement, err := domainpublication.ParseReportGrantSettlementV1(body)
	if err != nil || settlement.SettlementID != settlementID {
		return domainpublication.ReportGrantSettlementV1{}, errors.New("report grant settlement stable identity mismatch")
	}
	return settlement, nil
}

func (store *ReportGrantSettlementStore) VisitGrantSettlements(
	ctx context.Context,
	visit func(domainpublication.ReportGrantSettlementV1) error,
) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("report grant settlement inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		settlement, err := domainpublication.ParseReportGrantSettlementV1(file.Body)
		if err != nil || settlement.SettlementID != file.Digest {
			return errors.New("report grant settlement inventory is corrupt")
		}
		return visit(settlement)
	})
}

func (store *ReportStageCompletionStore) CreateExclusive(
	ctx context.Context,
	completion domainpublication.ReportStageCompletionV1,
) (bool, error) {
	body, err := domainpublication.ReportStageCompletionV1Bytes(completion)
	if err != nil {
		return false, err
	}
	if store == nil || store.cas == nil {
		return false, errors.New("report stage completion store is unavailable")
	}
	if current, readErr := store.cas.Read(ctx, completion.CompletionID); readErr == nil {
		parsed, parseErr := domainpublication.ParseReportStageCompletionV1(current)
		if parseErr != nil || parsed.CompletionID != completion.CompletionID || !bytes.Equal(current, body) {
			return false, errors.New("report stage completion conflicts with the admitted decision identity")
		}
		return false, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	if err := store.cas.PutIfAbsent(ctx, completion.CompletionID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return false, err
		}
		current, readErr := store.cas.Read(ctx, completion.CompletionID)
		if readErr != nil {
			return false, readErr
		}
		parsed, parseErr := domainpublication.ParseReportStageCompletionV1(current)
		if parseErr != nil || parsed.CompletionID != completion.CompletionID || !bytes.Equal(current, body) {
			return false, errors.New("report stage completion conflicts with the admitted decision identity")
		}
		return false, nil
	}
	written, err := store.cas.Read(ctx, completion.CompletionID)
	if err != nil || !bytes.Equal(written, body) {
		return false, errors.Join(errors.New("report stage completion write readback failed"), err)
	}
	parsed, err := domainpublication.ParseReportStageCompletionV1(written)
	if err != nil || parsed.CompletionID != completion.CompletionID || parsed.RecordDigest != completion.RecordDigest {
		return false, errors.Join(errors.New("report stage completion write semantic verification failed"), err)
	}
	return true, nil
}

func (store *ReportStageCompletionStore) Resolve(
	ctx context.Context,
	completionID string,
) (domainpublication.ReportStageCompletionV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, completionID)
	if err != nil {
		return domainpublication.ReportStageCompletionV1{}, err
	}
	completion, err := domainpublication.ParseReportStageCompletionV1(body)
	if err != nil || completion.CompletionID != completionID {
		return domainpublication.ReportStageCompletionV1{}, errors.New("report stage completion stable identity mismatch")
	}
	return completion, nil
}

func (store *ReportStageCompletionStore) VisitStageCompletions(
	ctx context.Context,
	visit func(domainpublication.ReportStageCompletionV1) error,
) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("report stage completion inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		completion, err := domainpublication.ParseReportStageCompletionV1(file.Body)
		if err != nil || completion.CompletionID != file.Digest {
			return errors.New("report stage completion inventory is corrupt")
		}
		return visit(completion)
	})
}

func (store *ReportDeliveryOutcomeStore) CreateOutcomeExclusive(
	ctx context.Context,
	completion domainpublication.ReportStageCompletionV1,
	candidate domainpublication.ReportDeliveryOutcomeV1,
) (domainpublication.ReportDeliveryOutcomeV1, bool, error) {
	body, err := domainpublication.ReportDeliveryOutcomeV1Bytes(candidate)
	deliveryID := domainpublication.ReportDeliveryOutcomeID(candidate)
	if err != nil || domainpublication.ValidateReportDeliveryOutcomeCompletionV1(candidate, completion) != nil ||
		deliveryID != domainpublication.ReportDeliveryOutcomeIDV1(completion.InstallationID, completion.EnrollmentID, completion.CompletionID) {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, errors.Join(
			errors.New("report delivery outcome does not bind the supplied completion"), err,
		)
	}
	if store == nil || store.cas == nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, errors.New("report delivery outcome store is unavailable")
	}
	if current, readErr := store.cas.Read(ctx, deliveryID); readErr == nil {
		winner, parseErr := domainpublication.ParseReportDeliveryOutcomeV1(current)
		if parseErr != nil || domainpublication.ReportDeliveryOutcomeID(winner) != deliveryID ||
			domainpublication.ValidateReportDeliveryOutcomeCompletionV1(winner, completion) != nil {
			return domainpublication.ReportDeliveryOutcomeV1{}, false, errors.New("report delivery outcome winner is corrupt or mismatched")
		}
		return winner, false, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, readErr
	}
	putErr := store.cas.PutIfAbsent(ctx, deliveryID, body)
	if putErr != nil && !errors.Is(putErr, os.ErrExist) {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, putErr
	}
	written, readErr := store.cas.Read(ctx, deliveryID)
	if readErr != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, errors.Join(
			errors.New("report delivery outcome write readback failed"), putErr, readErr,
		)
	}
	winner, parseErr := domainpublication.ParseReportDeliveryOutcomeV1(written)
	if parseErr != nil || domainpublication.ReportDeliveryOutcomeID(winner) != deliveryID ||
		domainpublication.ValidateReportDeliveryOutcomeCompletionV1(winner, completion) != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, errors.Join(
			errors.New("report delivery outcome write semantic verification failed"), parseErr,
		)
	}
	return winner, putErr == nil && bytes.Equal(written, body), nil
}

func (store *ReportDeliveryOutcomeStore) ResolveOutcome(
	ctx context.Context,
	deliveryID string,
) (domainpublication.ReportDeliveryOutcomeV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, deliveryID)
	if err != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, err
	}
	outcome, err := domainpublication.ParseReportDeliveryOutcomeV1(body)
	if err != nil || domainpublication.ReportDeliveryOutcomeID(outcome) != deliveryID {
		return domainpublication.ReportDeliveryOutcomeV1{}, errors.New("report delivery outcome stable identity mismatch")
	}
	return outcome, nil
}

func (store *ReportDeliveryOutcomeStore) VisitDeliveryOutcomes(
	ctx context.Context,
	visit func(domainpublication.ReportDeliveryOutcomeV1) error,
) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("report delivery outcome inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		outcome, err := domainpublication.ParseReportDeliveryOutcomeV1(file.Body)
		if err != nil || domainpublication.ReportDeliveryOutcomeID(outcome) != file.Digest {
			return errors.New("report delivery outcome inventory is corrupt")
		}
		return visit(outcome)
	})
}

func (store *IndexStore) PutIfAbsent(ctx context.Context, index domainpublication.PublicationIndexV1) error {
	body, err := domainpublication.PublicationIndexV1Bytes(index)
	if err != nil {
		return err
	}
	return putSemanticRecord(ctx, store.cas, index.IndexDigest, body, func(raw []byte) (string, error) {
		parsed, parseErr := domainpublication.ParsePublicationIndexV1(raw)
		return parsed.IndexDigest, parseErr
	})
}
func (store *IndexStore) Resolve(ctx context.Context, digest string) (domainpublication.PublicationIndexV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, digest)
	if err != nil {
		return domainpublication.PublicationIndexV1{}, err
	}
	index, err := domainpublication.ParsePublicationIndexV1(body)
	if err != nil || index.IndexDigest != digest {
		return domainpublication.PublicationIndexV1{}, errors.New("publication index content address mismatch")
	}
	return index, nil
}

func (store *IndexStore) VisitIndexes(ctx context.Context, visit func(domainpublication.PublicationIndexV1) error) error {
	if store == nil || store.cas == nil || ctx == nil || visit == nil {
		return errors.New("publication index inventory is unavailable")
	}
	return store.cas.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		index, err := domainpublication.ParsePublicationIndexV1(file.Body)
		if err != nil || index.IndexDigest != file.Digest {
			return errors.New("publication index inventory is corrupt")
		}
		return visit(index)
	})
}

func (store *ClaimLedgerStore) PutIfAbsent(ctx context.Context, ledger domainpublication.ClaimLedgerV1) error {
	body, err := domainpublication.ClaimLedgerV1Bytes(ledger)
	if err != nil {
		return err
	}
	return putSemanticRecord(ctx, store.cas, ledger.LedgerDigest, body, func(raw []byte) (string, error) {
		parsed, parseErr := domainpublication.ParseClaimLedgerV1(raw)
		return parsed.LedgerDigest, parseErr
	})
}
func (store *ClaimLedgerStore) Resolve(ctx context.Context, digest string) (domainpublication.ClaimLedgerV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, digest)
	if err != nil {
		return domainpublication.ClaimLedgerV1{}, err
	}
	ledger, err := domainpublication.ParseClaimLedgerV1(body)
	if err != nil || ledger.LedgerDigest != digest {
		return domainpublication.ClaimLedgerV1{}, errors.New("claim ledger content address mismatch")
	}
	return ledger, nil
}

func (store *PIIProjectionStore) PutIfAbsent(ctx context.Context, projection domainpublication.PIIProjectionV1) error {
	body, err := domainpublication.PIIProjectionV1Bytes(projection)
	if err != nil {
		return err
	}
	return putSemanticRecord(ctx, store.cas, projection.ProjectionDigest, body, func(raw []byte) (string, error) {
		parsed, parseErr := domainpublication.ParsePIIProjectionV1(raw)
		return parsed.ProjectionDigest, parseErr
	})
}
func (store *PIIProjectionStore) Resolve(ctx context.Context, digest string) (domainpublication.PIIProjectionV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, digest)
	if err != nil {
		return domainpublication.PIIProjectionV1{}, err
	}
	projection, err := domainpublication.ParsePIIProjectionV1(body)
	if err != nil || projection.ProjectionDigest != digest {
		return domainpublication.PIIProjectionV1{}, errors.New("PII projection content address mismatch")
	}
	return projection, nil
}

func (store *RenderInspectionStore) PutIfAbsent(ctx context.Context, inspection domainpublication.RenderInspectionV1) error {
	body, err := domainpublication.RenderInspectionV1Bytes(inspection)
	if err != nil {
		return err
	}
	return putSemanticRecord(ctx, store.cas, inspection.InspectionDigest, body, func(raw []byte) (string, error) {
		parsed, parseErr := domainpublication.ParseRenderInspectionV1(raw)
		return parsed.InspectionDigest, parseErr
	})
}
func (store *RenderInspectionStore) Resolve(ctx context.Context, digest string) (domainpublication.RenderInspectionV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, digest)
	if err != nil {
		return domainpublication.RenderInspectionV1{}, err
	}
	inspection, err := domainpublication.ParseRenderInspectionV1(body)
	if err != nil || inspection.InspectionDigest != digest {
		return domainpublication.RenderInspectionV1{}, errors.New("render inspection content address mismatch")
	}
	return inspection, nil
}

func (store *ArtifactStore) InstallNoReplace(ctx context.Context, targetIdentityDigest string, body []byte) error {
	if store == nil || store.cas == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(targetIdentityDigest)) ||
		len(body) == 0 || len(body) > maxPublishedArtifactBytes {
		return errors.New("published artifact input is invalid")
	}
	if existing, err := store.cas.Read(ctx, targetIdentityDigest); err == nil {
		if bytes.Equal(existing, body) {
			return nil
		}
		return errors.New("published artifact target already contains different bytes")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := store.cas.PutIfAbsent(ctx, targetIdentityDigest, append([]byte(nil), body...)); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	written, err := store.cas.Read(ctx, targetIdentityDigest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("published artifact no-replace readback failed")
	}
	return nil
}

func (store *ArtifactStore) ResolveExact(ctx context.Context, targetIdentityDigest string) ([]byte, error) {
	return readPublicationRecord(ctx, store.cas, targetIdentityDigest)
}

func (store *ArtifactStore) ResolveControlledMetadata(
	ctx context.Context,
	targetIdentityDigest string,
) (domainpii.ControlledPIIArtifactMetadataV1, error) {
	body, err := readPublicationRecord(ctx, store.cas, targetIdentityDigest)
	if err != nil {
		return domainpii.ControlledPIIArtifactMetadataV1{}, err
	}
	return domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
}

func openPublicationCAS(root string, maxBytes int, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*finalauthorityadapter.SecurePrivateCAS, error) {
	if strings.TrimSpace(root) == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("publication authority store root is invalid")
	}
	return finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxBytes, access)
}

func putSemanticRecord(ctx context.Context, cas *finalauthorityadapter.SecurePrivateCAS, digest string, body []byte, parseDigest func([]byte) (string, error)) error {
	if cas == nil || !domainsecurity.IsSHA256Hex(digest) || len(body) == 0 || parseDigest == nil {
		return errors.New("publication authority CAS record is invalid")
	}
	if current, err := cas.Read(ctx, digest); err == nil {
		parsedDigest, parseErr := parseDigest(current)
		if parseErr != nil || parsedDigest != digest || !bytes.Equal(current, body) {
			return errors.New("publication authority CAS record conflicts with its content address")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := cas.PutIfAbsent(ctx, digest, body); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	written, err := cas.Read(ctx, digest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("publication authority CAS write readback failed")
	}
	parsedDigest, err := parseDigest(written)
	if err != nil || parsedDigest != digest {
		return errors.New("publication authority CAS write semantic verification failed")
	}
	return nil
}

func readPublicationRecord(ctx context.Context, cas *finalauthorityadapter.SecurePrivateCAS, digest string) ([]byte, error) {
	if cas == nil || strings.TrimSpace(digest) != digest || !domainsecurity.IsSHA256Hex(digest) {
		return nil, errors.New("publication authority content address is invalid")
	}
	body, err := cas.Read(ctx, digest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, publicationport.ErrNotFound
	}
	return body, err
}
