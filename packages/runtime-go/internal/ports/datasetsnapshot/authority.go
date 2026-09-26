package datasetsnapshot

import (
	"context"
	"encoding/json"
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

var (
	ErrUnavailable = errors.New("dataset snapshot authority is unavailable")
	ErrCorrupt     = errors.New("dataset snapshot authority is corrupt")
	ErrMismatch    = errors.New("dataset snapshot authority binding mismatch")
	ErrStale       = errors.New("dataset snapshot authority is stale")
	ErrNotFound    = errors.New("dataset snapshot immutable object is absent")
)

// Authority resolves the independently selected current immutable snapshot
// for one exact host case-binding observation. Implementations must anchor the
// returned record to the installation key and current index before returning.
// A record's own signature is authenticity, not current-head proof.
//
// This intentionally exposes no Put/Import/Accept method to provider, MCP, or
// ordinary turn callers. Snapshot acceptance is a separate host-only workflow.
type Authority interface {
	ResolveWitnessed(context.Context, ResolveInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error)
}

type ResolveInput struct {
	TenantID    string
	UserID      string
	Observation domainsecurity.CaseBindingObservationV1
}

// ResolveInputV2 selects the newest witnessed V2 snapshot for an exact case
// binding. ExpectedDatasetSnapshotID is optional for initial resolution; when
// present it is an equality constraint, never a local selector or fallback.
type ResolveInputV2 struct {
	TenantID                  string
	UserID                    string
	Observation               domainsecurity.CaseBindingObservationV1
	ExpectedDatasetSnapshotID string
}

// ResolvedSnapshotV2 contains the exact immutable values re-read from private
// storage under a fresh witnessed index head. It is evidence material, not a
// fact-publication capability.
type ResolvedSnapshotV2 struct {
	Record                 domainsecurity.DatasetSnapshotAuthorityRecordV2
	Manifest               domainsecurity.DatasetSnapshotManifestV2
	FundsProducerContent   domainsecurity.FundsProducerContentManifestV1
	FundsProducerContentV2 domainsecurity.FundsProducerContentManifestV2
}

// MarshalJSON preserves the exact legacy V1 selection encoding while binding
// V2 selections to their V2 producer value without serializing an additional
// empty producer variant.
func (snapshot ResolvedSnapshotV2) MarshalJSON() ([]byte, error) {
	if snapshot.Manifest.ProducerContentContract ==
		domainsecurity.FundsProducerContentManifestContractV2 {
		return json.Marshal(struct {
			Record                 domainsecurity.DatasetSnapshotAuthorityRecordV2
			Manifest               domainsecurity.DatasetSnapshotManifestV2
			FundsProducerContentV2 domainsecurity.FundsProducerContentManifestV2
		}{
			Record: snapshot.Record, Manifest: snapshot.Manifest,
			FundsProducerContentV2: snapshot.FundsProducerContentV2,
		})
	}
	return json.Marshal(struct {
		Record               domainsecurity.DatasetSnapshotAuthorityRecordV2
		Manifest             domainsecurity.DatasetSnapshotManifestV2
		FundsProducerContent domainsecurity.FundsProducerContentManifestV1
	}{
		Record: snapshot.Record, Manifest: snapshot.Manifest,
		FundsProducerContent: snapshot.FundsProducerContent,
	})
}

type AuthorityV2 interface {
	ResolveWitnessedV2(context.Context, ResolveInputV2) (ResolvedSnapshotV2, error)
}

// CurrentSelectionV2 is the complete read-only material selected from one
// freshly challenged shared EvidenceAuthority head. DatasetIndexPath is cloned
// in root-to-genesis order and proves that SelectedIndex is a member of the
// exact dataset child named by Head. Snapshot contains the exact DSV2
// record/manifest/producer graph selected by that index.
//
// This value is audit material only. Copying, serializing, or retaining it
// outside the issuing callback does not preserve currentness or effect
// authority; only CurrentSelectionCapabilityV2 can authorize an exact use.
type CurrentSelectionV2 struct {
	Head             evidenceauthorityport.FreshHead
	DatasetIndexPath []domainsecurity.DatasetSnapshotIndexV1
	SelectedIndex    domainsecurity.DatasetSnapshotIndexV1
	Snapshot         ResolvedSnapshotV2
	SelectionDigest  string
}

// CurrentSelectionCapabilityV2 is an opaque callback-scoped authority for one
// exact CurrentSelectionV2 and TurnSecurityContext V2. UseExact re-challenges
// the shared witness before and after use, supplies a cancellation-scoped
// lease context, and fails after the issuing callback closes.
type CurrentSelectionCapabilityV2 interface {
	UseExact(
		CurrentSelectionV2,
		domainsecurity.TurnSecurityContext,
		func(context.Context) error,
	) error
}

// RegistryCommitCapabilityV1 is a separate single-effect surface. It accepts
// only the actual registry owner's exact prepared commit and does not relax
// or advance CurrentSelectionCapabilityV2's read-only authority.
type RegistryCommitCapabilityV1 interface {
	UseExactRegistryCommit(CurrentSelectionV2, domainsecurity.TurnSecurityContext,
		domainevidence.PreparedEvidenceSettlement, domainevidence.HostEvidenceSettlementMarker,
		func(context.Context) error) error
}

// CurrentAuthorityV2 resolves and exposes exact DSV2 currentness only inside
// one callback. ResolveInputV2 must exactly describe the supplied immutable
// case-evidence TurnSecurityContext, including its expected snapshot id.
type CurrentAuthorityV2 interface {
	WithCurrentSelectionV2(
		context.Context,
		ResolveInputV2,
		domainsecurity.TurnSecurityContext,
		func(CurrentSelectionV2, CurrentSelectionCapabilityV2) error,
	) error
}

// RetainedSelectionInputV2 identifies one caller-owned historical snapshot
// through the current, witnessed selection. The retained id and source
// manifest are equality constraints only; callers cannot provide a digest,
// path, query, or independent inventory selector.
type RetainedSelectionInputV2 struct {
	CurrentResolveInput        ResolveInputV2
	CurrentSecurityContext     domainsecurity.TurnSecurityContext
	RetainedDatasetSnapshotID  string
	RetainedSourceManifestHash string
}

// RetainedSelectionV2 is callback-scoped read-only material for one historical
// DSV2 node that was found in the current selection's witnessed index path.
// It carries no independent authority: the reader must keep the current
// CurrentSelectionCapabilityV2 lease live while invoking the callback.
type RetainedSelectionV2 struct {
	Current       CurrentSelectionV2
	SelectedIndex domainsecurity.DatasetSnapshotIndexV1
	Snapshot      ResolvedSnapshotV2
}

// RetainedSelectionReaderV2 reuses the existing current DSV2 authority to
// revalidate and read one exact retained immutable snapshot. Implementations
// must not expose latest, inventory, filesystem, SQL, or caller-selected path
// access through this seam.
type RetainedSelectionReaderV2 interface {
	WithRetainedSelectionV2(
		context.Context,
		RetainedSelectionInputV2,
		func(context.Context, RetainedSelectionV2) error,
	) error
}

// ExactMaterialReferenceV2 is one parent-addressed private-CAS object. The
// logical contract digest is validated by the app after canonical parsing;
// this reference binds only the physical address, full SHA-256, and length.
type ExactMaterialReferenceV2 struct {
	Address    string
	SHA256     string
	ByteLength uint64
}

type MaterialKindV2 string

const (
	MaterialSnapshotManifestV2     MaterialKindV2 = "dataset-snapshot-manifest-v2"
	MaterialFundsProducerContentV1 MaterialKindV2 = "funds-producer-content-v1"
	MaterialFundsProducerContentV2 MaterialKindV2 = "funds-producer-content-v2"
	MaterialRawAcquisitionIntentV1 MaterialKindV2 = "raw-acquisition-intent-v1"
	MaterialRawManifestV1          MaterialKindV2 = "raw-manifest-v1"
	MaterialRawManifestPageV1      MaterialKindV2 = "raw-manifest-page-v1"
	MaterialRawEntryV1             MaterialKindV2 = "raw-entry-v1"
	MaterialRawSourceLocatorV1     MaterialKindV2 = "raw-source-locator-v1"
	MaterialRawContentRootV1       MaterialKindV2 = "raw-content-root-v1"
	MaterialRawContentIndexPageV1  MaterialKindV2 = "raw-content-index-page-v1"
	MaterialRawContentChunkV1      MaterialKindV2 = "raw-content-chunk-v1"
	MaterialParsedConfigurationV1  MaterialKindV2 = "parsed-configuration-v1"
	MaterialParsedIdentityV1       MaterialKindV2 = "parsed-identity-v1"
	MaterialParsedReceiptV1        MaterialKindV2 = "parsed-receipt-v1"
	MaterialClassificationLedgerV1 MaterialKindV2 = "classification-ledger-v1"
	MaterialParsedIndexPageV1      MaterialKindV2 = "parsed-index-page-v1"
	MaterialParsedPageV1           MaterialKindV2 = "parsed-page-v1"
	MaterialSourceRowLedgerRootV1  MaterialKindV2 = "source-row-ledger-root-v1"
	MaterialSourceRowIndexPageV1   MaterialKindV2 = "source-row-index-page-v1"
	MaterialSourceRowPageV1        MaterialKindV2 = "source-row-page-v1"
	MaterialSourceRowRecordV1      MaterialKindV2 = "source-row-record-v1"
	MaterialSourceRowLineageV1     MaterialKindV2 = "source-row-lineage-v1"
)

// ProducerMaterialKindForManifestV2 selects one closed material parser from
// the producer-content contract sealed by DSV2. Unknown contracts never fall
// back to a legacy parser.
func ProducerMaterialKindForManifestV2(
	manifest domainsecurity.DatasetSnapshotManifestV2,
) (MaterialKindV2, error) {
	if err := domainsecurity.ValidateDatasetSnapshotManifestV2(manifest); err != nil {
		return "", errors.Join(ErrMismatch, errors.New("dataset snapshot producer contract is invalid"))
	}
	switch manifest.ProducerContentContract {
	case domainsecurity.FundsProducerContentManifestContractV1:
		return MaterialFundsProducerContentV1, nil
	case domainsecurity.FundsProducerContentManifestContractV2:
		return MaterialFundsProducerContentV2, nil
	default:
		return "", errors.Join(ErrMismatch, errors.New("dataset snapshot producer contract is unsupported"))
	}
}

// ValidateSnapshotProducerVariantV2 enforces an exactly-one closed producer
// union. The manifest contract chooses the validator; an empty, additional,
// cross-labelled, or coerced producer value is rejected.
func ValidateSnapshotProducerVariantV2(
	manifest domainsecurity.DatasetSnapshotManifestV2,
	producerV1 domainsecurity.FundsProducerContentManifestV1,
	producerV2 domainsecurity.FundsProducerContentManifestV2,
) error {
	switch manifest.ProducerContentContract {
	case domainsecurity.FundsProducerContentManifestContractV1:
		if producerV2 != (domainsecurity.FundsProducerContentManifestV2{}) ||
			domainsecurity.ValidateDatasetSnapshotManifestV2FundsProducerContentV1(
				manifest,
				producerV1,
			) != nil {
			return errors.Join(ErrMismatch, errors.New("dataset snapshot producer v1 variant is invalid"))
		}
	case domainsecurity.FundsProducerContentManifestContractV2:
		if producerV1 != (domainsecurity.FundsProducerContentManifestV1{}) ||
			domainsecurity.ValidateDatasetSnapshotManifestV2FundsProducerContentV2(
				manifest,
				producerV2,
			) != nil {
			return errors.Join(ErrMismatch, errors.New("dataset snapshot producer v2 variant is invalid"))
		}
	default:
		return errors.Join(ErrMismatch, errors.New("dataset snapshot producer variant is unsupported"))
	}
	return nil
}

// ValidateResolvedSnapshotV2 binds the signed authority record to exactly one
// producer-content version. Currentness and witnessed index membership remain
// separate app-layer obligations.
func ValidateResolvedSnapshotV2(snapshot ResolvedSnapshotV2) error {
	if err := ValidateSnapshotProducerVariantV2(
		snapshot.Manifest,
		snapshot.FundsProducerContent,
		snapshot.FundsProducerContentV2,
	); err != nil {
		return err
	}
	switch snapshot.Manifest.ProducerContentContract {
	case domainsecurity.FundsProducerContentManifestContractV1:
		if domainsecurity.ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(
			snapshot.Record,
			snapshot.Manifest,
			snapshot.FundsProducerContent,
		) != nil {
			return errors.Join(ErrMismatch, errors.New("dataset snapshot producer v1 authority is invalid"))
		}
	case domainsecurity.FundsProducerContentManifestContractV2:
		if domainsecurity.ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentManifestV2(
			snapshot.Record,
			snapshot.Manifest,
			snapshot.FundsProducerContentV2,
		) != nil {
			return errors.Join(ErrMismatch, errors.New("dataset snapshot producer v2 authority is invalid"))
		}
	default:
		return errors.Join(ErrMismatch, errors.New("dataset snapshot producer authority is unsupported"))
	}
	return nil
}

// AdmissionMaterialReaderV2 exposes no inventory or latest selector. Every
// read is addressed by an exact parent reference and the app performs two
// independent reads before using the bytes.
type AdmissionMaterialReaderV2 interface {
	ResolveExact(context.Context, MaterialKindV2, ExactMaterialReferenceV2) ([]byte, error)
}

// AuthorityBundleV2 is stored as one immutable CAS object so the record and
// its exact canonical manifest cannot be torn across a crash boundary.
type AuthorityBundleV2 struct {
	Record   domainsecurity.DatasetSnapshotAuthorityRecordV2
	Manifest domainsecurity.DatasetSnapshotManifestV2
}

type AuthorityBundleStoreV2 interface {
	PutIfAbsent(context.Context, AuthorityBundleV2) error
	Resolve(context.Context, string) (AuthorityBundleV2, error)
}

// RecordStore and IndexStore are immutable content-addressed stores. Neither
// exposes inventory, latest, active, directory, timestamp, or mutable-head
// selection; a fresh shared evidence-authority witness supplies the only
// digest from which traversal may start.
type RecordStore interface {
	PutIfAbsent(context.Context, domainsecurity.DatasetSnapshotAuthorityRecordV1) error
	Resolve(context.Context, string) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error)
}

type IndexStore interface {
	PutIfAbsent(context.Context, domainsecurity.DatasetSnapshotIndexV1) error
	Resolve(context.Context, string) (domainsecurity.DatasetSnapshotIndexV1, error)
}

// HistoricalFactAuthorityV2 exposes exact original publication material only
// while its immutable index remains on a newly challenged retained chain.
// No query/commit capability escapes; current access permission is separate.
type HistoricalFactAuthorityV2 interface {
	WithHistoricalFactSelectionV2(context.Context, ResolveInputV2, domainsecurity.TurnSecurityContext, evidenceauthorityport.FreshHead, func(CurrentSelectionV2) error) error
}
