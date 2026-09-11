package fundsquerysource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
)

// accountFlowSourceRowResolverV1 is callback-scoped retrieval state, not an
// authority or registry. It can only compare a native locator with exact
// source-row material already bound by the live DSV2 selection.
type accountFlowSourceRowResolverV1 struct {
	lifecycle sync.RWMutex
	active    atomic.Bool
	resolving atomic.Bool

	ctx             context.Context
	manifest        domainsecurity.DatasetSnapshotManifestV2
	securityContext domainsecurity.TurnSecurityContext
	materials       datasetsnapshotport.AdmissionMaterialReaderV2
	usedLocators    map[string]struct{}

	rootLoaded bool
	root       domainevidence.SourceRowLedgerRootV1
	indexes    map[string]domainevidence.SourceRowLedgerIndexPageV1
	pages      map[string]domainevidence.SourceRowLedgerPageV1

	parsedReceiptLoaded bool
	parsedReceipt       domainevidence.ParsedGenerationReceiptV1
	parsedIndexes       []domainevidence.ParsedPageIndexV1
	parsedPages         map[string]domainevidence.ParsedPageV1
}

func newAccountFlowSourceRowResolverV1(
	ctx context.Context,
	manifest domainsecurity.DatasetSnapshotManifestV2,
	securityContext domainsecurity.TurnSecurityContext,
	materials datasetsnapshotport.AdmissionMaterialReaderV2,
) *accountFlowSourceRowResolverV1 {
	resolver := &accountFlowSourceRowResolverV1{
		ctx:             ctx,
		manifest:        manifest,
		securityContext: securityContext,
		materials:       materials,
		usedLocators:    map[string]struct{}{},
		indexes:         map[string]domainevidence.SourceRowLedgerIndexPageV1{},
		pages:           map[string]domainevidence.SourceRowLedgerPageV1{},
		parsedPages:     map[string]domainevidence.ParsedPageV1{},
	}
	resolver.active.Store(true)
	return resolver
}

func (resolver *accountFlowSourceRowResolverV1) close() {
	if resolver == nil {
		return
	}
	resolver.active.Store(false)
	resolver.lifecycle.Lock()
	resolver.ctx = nil
	resolver.manifest = domainsecurity.DatasetSnapshotManifestV2{}
	resolver.securityContext = domainsecurity.TurnSecurityContext{}
	resolver.materials = nil
	resolver.usedLocators = nil
	resolver.rootLoaded = false
	resolver.root = domainevidence.SourceRowLedgerRootV1{}
	resolver.indexes = nil
	resolver.pages = nil
	resolver.parsedReceiptLoaded = false
	resolver.parsedReceipt = domainevidence.ParsedGenerationReceiptV1{}
	resolver.parsedIndexes = nil
	resolver.parsedPages = nil
	resolver.lifecycle.Unlock()
}

func (resolver *accountFlowSourceRowResolverV1) resolve(
	sourceFileID string,
	sourceRowNumber uint64,
	consume func(string) error,
) error {
	if resolver == nil || consume == nil {
		return accountFlowResolverUnavailableV1("account-flow source-row resolver is unavailable")
	}
	if !resolver.resolving.CompareAndSwap(false, true) {
		return accountFlowResolverMismatchV1("account-flow source-row resolver does not allow concurrent or reentrant use")
	}
	defer resolver.resolving.Store(false)

	resolver.lifecycle.RLock()
	defer resolver.lifecycle.RUnlock()
	if !resolver.active.Load() || resolver.ctx == nil || resolver.ctx.Err() != nil ||
		dependencyIsNilV1(resolver.materials) {
		return accountFlowResolverUnavailableV1("account-flow source-row resolver callback is inactive")
	}

	root, policy, err := resolver.loadRootV1()
	if err != nil {
		return err
	}
	locatorInput, err := accountFlowLedgerLocatorInputV1(
		policy,
		resolver.securityContext.CaseID,
		domainevidence.SourceRowLocatorInputV1{
			SourceFileID:    sourceFileID,
			SourceRowNumber: sourceRowNumber,
		},
	)
	if err != nil {
		return err
	}
	record, err := resolver.resolveExactRecordForLocatorV1(
		root,
		policy,
		locatorInput,
	)
	if err != nil {
		return err
	}

	if !resolver.active.Load() || resolver.ctx.Err() != nil {
		return accountFlowResolverUnavailableV1("account-flow source-row resolver callback expired before consumption")
	}
	return consume(record.SourceRecordID)
}

// useExactControlledAccountV1 proves that every requested host-private source
// record belongs to the live DSV2 row ledger and to an accepted parsed row
// carrying the exact current subject account. The full account is exposed only
// to the synchronous callback and is never represented by a serializable
// return value.
func (resolver *accountFlowSourceRowResolverV1) useExactControlledAccountV1(
	expectedCanonicalAccount string,
	timezone string,
	expectedRows []domainnative.AccountFlowProviderSemanticTransactionV1,
	sourceLocators []domainevidence.SourceRowLocatorInputV1,
	use func(string) error,
) error {
	if resolver == nil || use == nil || len(expectedRows) == 0 ||
		len(expectedRows) > int(domainnative.AccountFlowMaximumEvidenceRowsV1) ||
		len(sourceLocators) != len(expectedRows) {
		return accountFlowResolverUnavailableV1("controlled account-flow source rows are unavailable")
	}
	expectedCanonicalAccount, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(
		expectedCanonicalAccount,
	)
	if err != nil {
		return accountFlowResolverMismatchV1("controlled account-flow subject account is invalid")
	}
	if _, err := controlledAccountFlowExpectedRowsV1(expectedRows); err != nil {
		return err
	}
	if !resolver.resolving.CompareAndSwap(false, true) {
		return accountFlowResolverMismatchV1("account-flow source-row resolver does not allow concurrent or reentrant use")
	}
	defer resolver.resolving.Store(false)

	resolver.lifecycle.RLock()
	defer resolver.lifecycle.RUnlock()
	if !resolver.active.Load() || resolver.ctx == nil || resolver.ctx.Err() != nil ||
		dependencyIsNilV1(resolver.materials) {
		return accountFlowResolverUnavailableV1("account-flow source-row resolver callback is inactive")
	}

	root, policy, err := resolver.loadRootV1()
	if err != nil {
		return err
	}
	for index, expected := range expectedRows {
		locatorInput, locatorErr := accountFlowLedgerLocatorInputV1(
			policy,
			resolver.securityContext.CaseID,
			sourceLocators[index],
		)
		if locatorErr != nil {
			return locatorErr
		}
		record, resolveErr := resolver.resolveExactRecordForLocatorV1(
			root,
			policy,
			locatorInput,
		)
		if resolveErr != nil {
			return resolveErr
		}
		if record.SourceRecordID != expected.EvidenceRef {
			return accountFlowResolverMismatchV1("controlled account-flow private locator identifies a different source record")
		}
		if resolveErr := resolver.validateExactTransactionForRecordV1(
			policy,
			record,
			expectedCanonicalAccount,
			timezone,
			expected,
		); resolveErr != nil {
			return resolveErr
		}
	}
	if !resolver.active.Load() || resolver.ctx.Err() != nil {
		return accountFlowResolverUnavailableV1("account-flow source-row resolver callback expired before controlled consumption")
	}
	return use(expectedCanonicalAccount)
}

// useExactAcceptedSlotEntityV1 resolves only value-free, settlement-bound
// source file/row/field lineage against the retained immutable DSV2. The
// canonical value is comparison authority only; the callback receives the
// exact raw field just read from the retained parsed row.
func (resolver *accountFlowSourceRowResolverV1) useExactAcceptedSlotEntityV1(
	bindings []domainevidence.AcceptedSlotSourceBindingV1,
	expectedCanonicalEntity string,
	use func(string) error,
) error {
	if resolver == nil || use == nil || len(bindings) == 0 ||
		len(bindings) > int(domainnative.AccountFlowMaximumEvidenceRowsV1) {
		return accountFlowResolverUnavailableV1("accepted slot source lineage is unavailable")
	}
	canonicalBindings, err := domainevidence.CanonicalAcceptedSlotSourceBindingsV1(bindings)
	if err != nil || len(canonicalBindings) != len(bindings) {
		return accountFlowResolverMismatchV1("accepted slot source lineage is invalid")
	}
	expectedCanonicalEntity, err = domaincontrolledaccount.CanonicalFinancialAccountTextV2(expectedCanonicalEntity)
	if err != nil {
		return accountFlowResolverMismatchV1("accepted slot canonical comparison authority is invalid")
	}
	if !resolver.resolving.CompareAndSwap(false, true) {
		return accountFlowResolverMismatchV1("account-flow source-row resolver does not allow concurrent or reentrant use")
	}
	defer resolver.resolving.Store(false)

	resolver.lifecycle.RLock()
	defer resolver.lifecycle.RUnlock()
	if !resolver.active.Load() || resolver.ctx == nil || resolver.ctx.Err() != nil ||
		dependencyIsNilV1(resolver.materials) {
		return accountFlowResolverUnavailableV1("accepted slot source callback is inactive")
	}
	root, policy, err := resolver.loadRootV1()
	if err != nil {
		return err
	}
	exactValue := ""
	for _, binding := range canonicalBindings {
		if !domainevidence.IsAcceptedSlotSourceFieldV1(binding.Field) {
			return accountFlowResolverMismatchV1("accepted slot requested field is not supported")
		}
		record, resolveErr := resolver.resolveExactRecordForLocatorV1(
			root,
			policy,
			domainevidence.SourceRowLocatorInputV1{
				SourceFileID: binding.SourceFileID, SourceRowNumber: binding.SourceRowNumber,
			},
		)
		if resolveErr != nil {
			return resolveErr
		}
		if record.SourceRecordID != binding.SourceRecordID {
			return accountFlowResolverMismatchV1("accepted slot source locator identifies a different record")
		}
		fields, fieldsErr := resolver.loadExactParsedFieldsForRecordV1(policy, record)
		if fieldsErr != nil {
			return fieldsErr
		}
		candidate, candidateErr := exactControlledFinancialEntitySourceFieldV1(
			fields, binding.Field, expectedCanonicalEntity,
		)
		if candidateErr != nil {
			return candidateErr
		}
		if exactValue == "" {
			exactValue = candidate
		} else if !bytes.Equal([]byte(exactValue), []byte(candidate)) {
			return accountFlowResolverMismatchV1("accepted slot source rows contain different natural values")
		}
	}
	if exactValue == "" || !resolver.active.Load() || resolver.ctx.Err() != nil {
		return accountFlowResolverUnavailableV1("accepted slot source callback expired before consumption")
	}
	return use(exactValue)
}

func (resolver *accountFlowSourceRowResolverV1) resolveExactRecordForLocatorV1(
	root domainevidence.SourceRowLedgerRootV1,
	policy domainevidence.SourceRowProducerPolicyV1,
	input domainevidence.SourceRowLocatorInputV1,
) (domainevidence.SourceRowRecordV1, error) {
	locator, err := domainevidence.NewSourceRowLocatorV1(policy, input)
	if err != nil {
		return domainevidence.SourceRowRecordV1{}, accountFlowResolverMismatchV1("account-flow source-row locator is invalid")
	}
	if _, used := resolver.usedLocators[locator.LocatorDigest]; used {
		return domainevidence.SourceRowRecordV1{}, accountFlowResolverMismatchV1("account-flow source-row locator was resolved more than once")
	}
	// A failed comparison is terminal for this callback too. Retrying the same
	// private locator must not become a presence or mutation oracle.
	resolver.usedLocators[locator.LocatorDigest] = struct{}{}

	indexDescriptor, err := exactIndexDescriptorForLocatorV1(root, locator)
	if err != nil {
		return domainevidence.SourceRowRecordV1{}, err
	}
	indexPage, err := resolver.loadIndexPageV1(root, policy, indexDescriptor)
	if err != nil {
		return domainevidence.SourceRowRecordV1{}, err
	}
	pageDescriptor, err := exactDataPageDescriptorForLocatorV1(indexPage, locator)
	if err != nil {
		return domainevidence.SourceRowRecordV1{}, err
	}
	page, err := resolver.loadDataPageV1(root, policy, pageDescriptor)
	if err != nil {
		return domainevidence.SourceRowRecordV1{}, err
	}
	record, err := exactRecordForLocatorV1(page, locator)
	if err != nil {
		return domainevidence.SourceRowRecordV1{}, err
	}
	if err := domainevidence.ValidateSourceRowWitnessLedgerMaterialV1(
		policy,
		root,
		indexPage,
		page,
		record,
	); err != nil {
		return domainevidence.SourceRowRecordV1{}, errors.Join(
			accountFlowResolverMismatchV1("account-flow source-row hierarchy membership is invalid"),
			err,
		)
	}
	if err := resolver.validateStandaloneRecordV1(record); err != nil {
		return domainevidence.SourceRowRecordV1{}, err
	}
	return record, nil
}

// accountFlowLedgerLocatorInputV1 maps the canonical CSV data-engine's
// callback-private import identity to the opaque source-file identity already
// committed by the DSV2 row ledger. The mapping is fixed by the existing
// canonical CSV producer contract and never leaves this live source callback.
// Legacy FPC1 ledgers already carry their ledger identity and remain unchanged.
func accountFlowLedgerLocatorInputV1(
	policy domainevidence.SourceRowProducerPolicyV1,
	caseID string,
	input domainevidence.SourceRowLocatorInputV1,
) (domainevidence.SourceRowLocatorInputV1, error) {
	if policy.PolicyID != domainevidence.FundsCanonicalTransactionSourceRowPolicyIDV1 {
		return input, nil
	}
	sourceFileID, err := domainevidence.DeriveFundsCanonicalCSVSourceFileIDV1(caseID, input.SourceFileID)
	if err != nil {
		return domainevidence.SourceRowLocatorInputV1{},
			accountFlowResolverMismatchV1("canonical account-flow source-file identity is invalid")
	}
	input.SourceFileID = sourceFileID
	return input, nil
}

func (resolver *accountFlowSourceRowResolverV1) validateStandaloneRecordV1(
	record domainevidence.SourceRowRecordV1,
) error {
	canonicalRecord, err := json.Marshal(record)
	if err != nil {
		return errors.Join(
			accountFlowResolverMismatchV1("account-flow source-row standalone record is invalid"),
			err,
		)
	}
	defer clear(canonicalRecord)
	standalone, err := resolver.readExactTwiceV1(
		datasetsnapshotport.MaterialSourceRowRecordV1,
		exactAccountFlowMaterialReferenceV1(canonicalRecord),
	)
	if err != nil {
		return err
	}
	defer clear(standalone)
	if !bytes.Equal(standalone, canonicalRecord) {
		return accountFlowResolverCorruptV1("account-flow source-row standalone record does not match its ledger entry")
	}
	return nil
}

func (resolver *accountFlowSourceRowResolverV1) validateExactTransactionForRecordV1(
	policy domainevidence.SourceRowProducerPolicyV1,
	record domainevidence.SourceRowRecordV1,
	expectedCanonicalAccount string,
	timezone string,
	expected domainnative.AccountFlowProviderSemanticTransactionV1,
) error {
	fields, err := resolver.loadExactParsedFieldsForRecordV1(policy, record)
	if err != nil {
		return err
	}
	return validateControlledAccountFlowParsedFieldsV1(
		record.SourceRecordID,
		fields,
		expectedCanonicalAccount,
		timezone,
		expected,
	)
}

func (resolver *accountFlowSourceRowResolverV1) loadExactParsedFieldsForRecordV1(
	policy domainevidence.SourceRowProducerPolicyV1,
	record domainevidence.SourceRowRecordV1,
) ([]domainevidence.ParsedTypedFieldV1, error) {
	lineageBody, err := resolver.readExactTwiceV1(
		datasetsnapshotport.MaterialSourceRowLineageV1,
		datasetsnapshotport.ExactMaterialReferenceV2{
			Address: record.LineageSHA256, SHA256: record.LineageSHA256,
			ByteLength: record.LineageByteLength,
		},
	)
	if err != nil {
		return nil, err
	}
	defer clear(lineageBody)
	lineage, err := domainevidence.ParseSourceRowLineageV1(lineageBody)
	if err != nil || domainevidence.ValidateSourceRowRecordAgainstLineageV1(
		policy, resolver.manifest.Binding, record, lineage,
	) != nil || domainevidence.ValidateSourceRowLineageAgainstSnapshotManifestV2(
		lineage, resolver.manifest,
	) != nil {
		return nil, errors.Join(
			accountFlowResolverMismatchV1("controlled account-flow source lineage is invalid"),
			err,
		)
	}
	receipt, indexes, err := resolver.loadParsedGenerationV1()
	if err != nil {
		return nil, err
	}
	var matchedIndex domainevidence.ParsedPageIndexV1
	var matchedDescriptor domainevidence.ParsedPageDescriptorV1
	matches := 0
	for _, index := range indexes {
		for _, descriptor := range index.PageDescriptors {
			if descriptor.PageDigest == lineage.ParsedPageDigest &&
				descriptor.PageSHA256 == lineage.ParsedPageSHA256 &&
				descriptor.PageByteLength == lineage.ParsedPageByteLength {
				matchedIndex = index
				matchedDescriptor = descriptor
				matches++
			}
		}
	}
	if matches != 1 {
		return nil, accountFlowResolverMismatchV1("controlled account-flow parsed page has no unique receipt membership")
	}
	page, err := resolver.loadParsedPageV1(matchedDescriptor)
	if err != nil {
		return nil, err
	}
	if domainevidence.ValidateSourceRowLineageAgainstParsedGenerationV1(
		lineage, receipt, matchedIndex, page,
	) != nil || uint64(lineage.ParsedRowOrdinal) >= uint64(len(page.Outcomes)) {
		return nil, accountFlowResolverMismatchV1("controlled account-flow parsed row lineage is invalid")
	}
	outcome := page.Outcomes[lineage.ParsedRowOrdinal]
	if outcome.Disposition != domainevidence.ParsedOutcomeAcceptedV1 ||
		outcome.SourceRecordID != record.SourceRecordID {
		return nil, accountFlowResolverMismatchV1("controlled account-flow parsed row does not identify the expected source record")
	}
	return append([]domainevidence.ParsedTypedFieldV1(nil), outcome.CanonicalTypedRow...), nil
}

func exactControlledFinancialEntitySourceFieldV1(
	fields []domainevidence.ParsedTypedFieldV1,
	field string,
	expectedCanonicalEntity string,
) (string, error) {
	var normalizedField, rawField string
	switch field {
	case domainevidence.AcceptedSlotSourceFieldAccountV1:
		normalizedField, rawField = "norm.clean_acct_no", "raw.acct_no"
	case domainevidence.AcceptedSlotSourceFieldCardV1:
		normalizedField, rawField = "norm.clean_card_no", "raw.card_no"
	default:
		return "", accountFlowResolverMismatchV1("controlled account-flow source field is unsupported")
	}
	byName := make(map[string]domainevidence.ParsedTypedScalarV1, 2)
	for _, field := range fields {
		if field.Name != normalizedField && field.Name != rawField {
			continue
		}
		if _, duplicate := byName[field.Name]; duplicate {
			return "", accountFlowResolverMismatchV1("controlled account-flow parsed row contains a duplicate field")
		}
		byName[field.Name] = field.Scalar
	}
	account := func(name string) (string, string, bool, error) {
		scalar, ok := byName[name]
		if !ok || (scalar.Kind != "text" && scalar.Kind != "null") ||
			scalar.Kind == "null" && scalar.Value != "" {
			return "", "", false, accountFlowResolverMismatchV1("controlled account-flow account identity field is invalid")
		}
		if scalar.Kind == "null" {
			return "", "", false, nil
		}
		canonical, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(scalar.Value)
		if err != nil {
			return "", "", false, accountFlowResolverMismatchV1("controlled account-flow account identity is invalid")
		}
		return scalar.Value, canonical, true, nil
	}
	_, normalized, hasNormalized, err := account(normalizedField)
	if err != nil {
		return "", err
	}
	exact, rawCanonical, hasRaw, err := account(rawField)
	if err != nil || !hasRaw || exact == "" ||
		hasNormalized && normalized != rawCanonical || rawCanonical != expectedCanonicalEntity {
		return "", accountFlowResolverMismatchV1("controlled account-flow source row belongs to a different financial account identity")
	}
	return exact, nil
}

func validateControlledAccountFlowParsedFieldsV1(
	sourceRecordID string,
	fields []domainevidence.ParsedTypedFieldV1,
	expectedCanonicalAccount string,
	timezone string,
	expected domainnative.AccountFlowProviderSemanticTransactionV1,
) error {
	if sourceRecordID == "" || sourceRecordID != expected.EvidenceRef {
		return accountFlowResolverMismatchV1("controlled account-flow parsed row identifies a different source record")
	}
	byName := make(map[string]domainevidence.ParsedTypedScalarV1, len(fields))
	for _, field := range fields {
		if _, duplicate := byName[field.Name]; duplicate {
			return accountFlowResolverMismatchV1("controlled account-flow parsed row contains a duplicate field")
		}
		byName[field.Name] = field.Scalar
	}
	required := func(name, kind string) (string, error) {
		scalar, ok := byName[name]
		if !ok || scalar.Kind != kind || scalar.Value == "" {
			return "", accountFlowResolverMismatchV1("controlled account-flow parsed row field is missing or invalid")
		}
		return scalar.Value, nil
	}
	optionalAccount := func(name string) (string, bool, error) {
		scalar, ok := byName[name]
		if !ok || (scalar.Kind != "text" && scalar.Kind != "null") ||
			scalar.Kind == "null" && scalar.Value != "" {
			return "", false, accountFlowResolverMismatchV1("controlled account-flow account identity field is invalid")
		}
		if scalar.Kind == "null" {
			return "", false, nil
		}
		canonical, err := domaincontrolledaccount.CanonicalFinancialAccountTextV2(scalar.Value)
		if err != nil {
			return "", false, accountFlowResolverMismatchV1("controlled account-flow account identity is invalid")
		}
		return canonical, true, nil
	}
	cleanAccount, hasCleanAccount, err := optionalAccount("norm.clean_acct_no")
	if err != nil {
		return err
	}
	rawAccount, hasRawAccount, err := optionalAccount("raw.acct_no")
	if err != nil || hasCleanAccount && hasRawAccount && cleanAccount != rawAccount {
		return accountFlowResolverMismatchV1("controlled account-flow account identity fields are inconsistent")
	}
	cleanCard, hasCleanCard, err := optionalAccount("norm.clean_card_no")
	if err != nil {
		return err
	}
	rawCard, hasRawCard, err := optionalAccount("raw.card_no")
	if err != nil || hasCleanCard && hasRawCard && cleanCard != rawCard {
		return accountFlowResolverMismatchV1("controlled account-flow card identity fields are inconsistent")
	}
	// The production materializer's ACCOUNT_KEY_CANDIDATES chooses account
	// before card and normalized values before raw fallbacks. Matching any
	// candidate would let a conflicting secondary identifier borrow the
	// primary row's evidence.
	selected := cleanAccount
	if !hasCleanAccount {
		selected = rawAccount
	}
	if !hasCleanAccount && !hasRawAccount {
		selected = cleanCard
	}
	if !hasCleanAccount && !hasRawAccount && !hasCleanCard {
		selected = rawCard
	}
	if selected == "" || !hasCleanAccount && !hasRawAccount && !hasCleanCard && !hasRawCard ||
		selected != expectedCanonicalAccount {
		return accountFlowResolverMismatchV1("controlled account-flow source row belongs to a different financial account identity")
	}

	timestamp, err := required("norm.txn_ts", "text")
	if err != nil {
		return err
	}
	parsedTimestamp, err := time.Parse("2006-01-02 15:04:05", timestamp)
	offset, errOffset := controlledAccountFlowTimezoneOffsetV1(timezone)
	if err != nil || errOffset != nil || parsedTimestamp.Format("2006-01-02 15:04:05") != timestamp {
		return accountFlowResolverMismatchV1("controlled account-flow parsed timestamp is invalid")
	}
	location := time.FixedZone("analytix-controlled-source", offset)
	occurredAt := time.Date(
		parsedTimestamp.Year(), parsedTimestamp.Month(), parsedTimestamp.Day(),
		parsedTimestamp.Hour(), parsedTimestamp.Minute(), parsedTimestamp.Second(), 0, location,
	).UTC().Format("2006-01-02T15:04:05.000000Z")
	if occurredAt != expected.OccurredAt {
		return accountFlowResolverMismatchV1("controlled account-flow source timestamp contradicts the expected transaction")
	}

	direction, err := required("norm.clean_dc_flag", "text")
	if err != nil {
		return err
	}
	expectedDirection := ""
	switch direction {
	case "进":
		expectedDirection = domainnative.AccountFlowDirectionInflowV1
	case "出":
		expectedDirection = domainnative.AccountFlowDirectionOutflowV1
	default:
		return accountFlowResolverMismatchV1("controlled account-flow parsed direction is invalid")
	}
	if expected.Direction != expectedDirection {
		return accountFlowResolverMismatchV1("controlled account-flow source direction contradicts the expected transaction")
	}

	amount, err := required("norm.clean_amount", "decimal")
	if err != nil {
		return err
	}
	amountMinor, err := controlledAccountFlowMinorUnitsV1(amount)
	if err != nil || amountMinor != expected.AmountMinor {
		return accountFlowResolverMismatchV1("controlled account-flow source amount contradicts the expected transaction")
	}
	currency, err := required("raw.currency", "text")
	if err != nil || currency != expected.Currency {
		return accountFlowResolverMismatchV1("controlled account-flow source currency contradicts the expected transaction")
	}
	return nil
}

func controlledAccountFlowExpectedRowsV1(
	rows []domainnative.AccountFlowProviderSemanticTransactionV1,
) (map[string]domainnative.AccountFlowProviderSemanticTransactionV1, error) {
	if len(rows) == 0 || len(rows) > int(domainnative.AccountFlowMaximumEvidenceRowsV1) {
		return nil, accountFlowResolverUnavailableV1("controlled account-flow source rows are unavailable")
	}
	wanted := make(map[string]domainnative.AccountFlowProviderSemanticTransactionV1, len(rows))
	for _, expected := range rows {
		sourceRecordID := expected.EvidenceRef
		if !strings.HasPrefix(sourceRecordID, "srow1_") ||
			!domainsecurity.IsSHA256Hex(strings.TrimPrefix(sourceRecordID, "srow1_")) ||
			expected.MinorUnitScale != domainnative.AccountFlowMinorUnitScaleV1 {
			return nil, accountFlowResolverMismatchV1("controlled account-flow source record identity is invalid")
		}
		if _, duplicate := wanted[sourceRecordID]; duplicate {
			return nil, accountFlowResolverMismatchV1("controlled account-flow source record identity is duplicated")
		}
		wanted[sourceRecordID] = expected
	}
	return wanted, nil
}

func controlledAccountFlowTimezoneOffsetV1(value string) (int, error) {
	if value == "Z" {
		return 0, nil
	}
	if len(value) != 6 || (value[0] != '+' && value[0] != '-') || value[3] != ':' {
		return 0, errors.New("controlled account-flow timezone is invalid")
	}
	hour := int(value[1]-'0')*10 + int(value[2]-'0')
	minute := int(value[4]-'0')*10 + int(value[5]-'0')
	if value[1] < '0' || value[1] > '9' || value[2] < '0' || value[2] > '9' ||
		value[4] < '0' || value[4] > '9' || value[5] < '0' || value[5] > '9' ||
		hour > 14 || minute > 59 || hour == 14 && minute != 0 {
		return 0, errors.New("controlled account-flow timezone is invalid")
	}
	offset := (hour*60 + minute) * 60
	if value[0] == '-' {
		offset = -offset
	}
	return offset, nil
}

func controlledAccountFlowMinorUnitsV1(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "eE") {
		return "", errors.New("controlled account-flow amount is invalid")
	}
	negative := false
	unsigned := value
	if unsigned[0] == '-' || unsigned[0] == '+' {
		negative = unsigned[0] == '-'
		unsigned = unsigned[1:]
	}
	parts := strings.Split(unsigned, ".")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" ||
		len(parts) == 2 && parts[1] == "" || len(parts) == 2 && len(parts[1]) > 2 {
		return "", errors.New("controlled account-flow amount is invalid")
	}
	for _, part := range parts {
		for index := range part {
			if part[index] < '0' || part[index] > '9' {
				return "", errors.New("controlled account-flow amount is invalid")
			}
		}
	}
	digits := parts[0]
	if len(parts) == 1 {
		digits += "00"
	} else if len(parts[1]) == 1 {
		digits += parts[1] + "0"
	} else {
		digits += parts[1]
	}
	minor := new(big.Int)
	if _, ok := minor.SetString(digits, 10); !ok || minor.BitLen() > 127 {
		return "", errors.New("controlled account-flow amount is invalid")
	}
	if negative {
		minor.Neg(minor)
	}
	minor.Abs(minor)
	return minor.String(), nil
}

func (resolver *accountFlowSourceRowResolverV1) loadParsedGenerationV1() (
	domainevidence.ParsedGenerationReceiptV1,
	[]domainevidence.ParsedPageIndexV1,
	error,
) {
	if resolver.parsedReceiptLoaded {
		return resolver.parsedReceipt, resolver.parsedIndexes, nil
	}
	receiptBody, err := resolver.readExactTwiceV1(
		datasetsnapshotport.MaterialParsedReceiptV1,
		datasetsnapshotport.ExactMaterialReferenceV2{
			Address:    resolver.manifest.ParsedGenerationReceiptSHA256,
			SHA256:     resolver.manifest.ParsedGenerationReceiptSHA256,
			ByteLength: resolver.manifest.ParsedGenerationReceiptByteLength,
		},
	)
	if err != nil {
		return domainevidence.ParsedGenerationReceiptV1{}, nil, err
	}
	defer clear(receiptBody)
	receipt, err := domainevidence.ParseParsedGenerationReceiptV1(receiptBody)
	if err != nil || domainevidence.ValidateDatasetSnapshotManifestV2ParsedGenerationV1(
		resolver.manifest, receipt,
	) != nil {
		return domainevidence.ParsedGenerationReceiptV1{}, nil, errors.Join(
			accountFlowResolverMismatchV1("controlled account-flow parsed generation receipt is invalid"),
			err,
		)
	}
	indexes := make([]domainevidence.ParsedPageIndexV1, len(receipt.IndexPageDescriptors))
	for index, descriptor := range receipt.IndexPageDescriptors {
		body, loadErr := resolver.readExactTwiceV1(
			datasetsnapshotport.MaterialParsedIndexPageV1,
			datasetsnapshotport.ExactMaterialReferenceV2{
				Address: descriptor.IndexPageSHA256, SHA256: descriptor.IndexPageSHA256,
				ByteLength: descriptor.IndexPageByteLength,
			},
		)
		if loadErr != nil {
			return domainevidence.ParsedGenerationReceiptV1{}, nil, loadErr
		}
		parsed, parseErr := domainevidence.ParseParsedPageIndexV1(body)
		clear(body)
		if parseErr != nil || domainevidence.ValidateParsedPageIndexAgainstDescriptorV1(
			descriptor, parsed,
		) != nil {
			return domainevidence.ParsedGenerationReceiptV1{}, nil, errors.Join(
				accountFlowResolverMismatchV1("controlled account-flow parsed index is invalid"),
				parseErr,
			)
		}
		indexes[index] = parsed
	}
	if domainevidence.ValidateParsedGenerationReceiptHierarchyV1(receipt, indexes) != nil {
		return domainevidence.ParsedGenerationReceiptV1{}, nil,
			accountFlowResolverMismatchV1("controlled account-flow parsed generation hierarchy is incomplete")
	}
	resolver.parsedReceipt = receipt
	resolver.parsedIndexes = indexes
	resolver.parsedReceiptLoaded = true
	return receipt, indexes, nil
}

func (resolver *accountFlowSourceRowResolverV1) loadParsedPageV1(
	descriptor domainevidence.ParsedPageDescriptorV1,
) (domainevidence.ParsedPageV1, error) {
	if cached, ok := resolver.parsedPages[descriptor.DescriptorDigest]; ok {
		return cached, nil
	}
	body, err := resolver.readExactTwiceV1(
		datasetsnapshotport.MaterialParsedPageV1,
		datasetsnapshotport.ExactMaterialReferenceV2{
			Address: descriptor.PageSHA256, SHA256: descriptor.PageSHA256,
			ByteLength: descriptor.PageByteLength,
		},
	)
	if err != nil {
		return domainevidence.ParsedPageV1{}, err
	}
	defer clear(body)
	page, err := domainevidence.ParseParsedPageV1(body)
	if err != nil || domainevidence.ValidateParsedPageAgainstDescriptorV1(descriptor, page) != nil {
		return domainevidence.ParsedPageV1{}, errors.Join(
			accountFlowResolverMismatchV1("controlled account-flow parsed page is invalid"),
			err,
		)
	}
	resolver.parsedPages[descriptor.DescriptorDigest] = page
	return page, nil
}

func (resolver *accountFlowSourceRowResolverV1) loadRootV1() (
	domainevidence.SourceRowLedgerRootV1,
	domainevidence.SourceRowProducerPolicyV1,
	error,
) {
	if resolver.rootLoaded {
		policy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(resolver.root.PolicyID)
		if !ok || !accountFlowTransactionPolicyIDV1(policy.PolicyID) {
			return domainevidence.SourceRowLedgerRootV1{}, domainevidence.SourceRowProducerPolicyV1{},
				accountFlowResolverMismatchV1("account-flow source-row policy is not the transaction ledger policy")
		}
		return resolver.root, policy, nil
	}
	reference := datasetsnapshotport.ExactMaterialReferenceV2{
		Address:    resolver.manifest.SourceRowLedgerRootSHA256,
		SHA256:     resolver.manifest.SourceRowLedgerRootSHA256,
		ByteLength: resolver.manifest.SourceRowLedgerRootByteLength,
	}
	body, err := resolver.readExactTwiceV1(
		datasetsnapshotport.MaterialSourceRowLedgerRootV1,
		reference,
	)
	if err != nil {
		return domainevidence.SourceRowLedgerRootV1{}, domainevidence.SourceRowProducerPolicyV1{}, err
	}
	defer clear(body)
	root, err := domainevidence.ParseSourceRowLedgerRootV1(body)
	if err != nil || domainevidence.ValidateDatasetSnapshotManifestV2RowRootStructureV1(
		resolver.manifest,
		root,
	) != nil || domainevidence.ValidateSourceRowLedgerRootForContextV1(
		root,
		resolver.securityContext,
	) != nil {
		return domainevidence.SourceRowLedgerRootV1{}, domainevidence.SourceRowProducerPolicyV1{}, errors.Join(
			accountFlowResolverMismatchV1("account-flow source-row root does not match the live DSV2 context"),
			err,
		)
	}
	policy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok || !accountFlowTransactionPolicyIDV1(policy.PolicyID) ||
		domainevidence.ValidateSourceRowLedgerRootV1(policy, root) != nil {
		return domainevidence.SourceRowLedgerRootV1{}, domainevidence.SourceRowProducerPolicyV1{},
			accountFlowResolverMismatchV1("account-flow source-row root is not a transaction ledger")
	}
	resolver.root = root
	resolver.rootLoaded = true
	return root, policy, nil
}

func accountFlowTransactionPolicyIDV1(policyID string) bool {
	return policyID == domainevidence.FundsTransactionSourceRowPolicyIDV1 ||
		policyID == domainevidence.FundsCanonicalTransactionSourceRowPolicyIDV1
}

func (resolver *accountFlowSourceRowResolverV1) loadIndexPageV1(
	root domainevidence.SourceRowLedgerRootV1,
	policy domainevidence.SourceRowProducerPolicyV1,
	descriptor domainevidence.SourceRowLedgerIndexPageDescriptorV1,
) (domainevidence.SourceRowLedgerIndexPageV1, error) {
	if cached, ok := resolver.indexes[descriptor.DescriptorDigest]; ok {
		return cached, nil
	}
	body, err := resolver.readExactTwiceV1(
		datasetsnapshotport.MaterialSourceRowIndexPageV1,
		datasetsnapshotport.ExactMaterialReferenceV2{
			Address: descriptor.IndexPageSHA256, SHA256: descriptor.IndexPageSHA256,
			ByteLength: descriptor.IndexPageByteLength,
		},
	)
	if err != nil {
		return domainevidence.SourceRowLedgerIndexPageV1{}, err
	}
	defer clear(body)
	page, err := domainevidence.ParseSourceRowLedgerIndexPageV1(body)
	exact, exactErr := domainevidence.NewSourceRowLedgerIndexPageDescriptorV1(policy, root.Binding, page)
	if err != nil || exactErr != nil || exact != descriptor {
		return domainevidence.SourceRowLedgerIndexPageV1{}, errors.Join(
			accountFlowResolverMismatchV1("account-flow source-row index page does not match its root descriptor"),
			err,
			exactErr,
		)
	}
	resolver.indexes[descriptor.DescriptorDigest] = page
	return page, nil
}

func (resolver *accountFlowSourceRowResolverV1) loadDataPageV1(
	root domainevidence.SourceRowLedgerRootV1,
	policy domainevidence.SourceRowProducerPolicyV1,
	descriptor domainevidence.SourceRowLedgerPageDescriptorV1,
) (domainevidence.SourceRowLedgerPageV1, error) {
	if cached, ok := resolver.pages[descriptor.DescriptorDigest]; ok {
		return cached, nil
	}
	body, err := resolver.readExactTwiceV1(
		datasetsnapshotport.MaterialSourceRowPageV1,
		datasetsnapshotport.ExactMaterialReferenceV2{
			Address: descriptor.PageSHA256, SHA256: descriptor.PageSHA256,
			ByteLength: descriptor.PageByteLength,
		},
	)
	if err != nil {
		return domainevidence.SourceRowLedgerPageV1{}, err
	}
	defer clear(body)
	page, err := domainevidence.ParseSourceRowLedgerPageV1(body)
	exact, exactErr := domainevidence.NewSourceRowLedgerPageDescriptorV1(policy, root.Binding, page)
	if err != nil || exactErr != nil || exact != descriptor {
		return domainevidence.SourceRowLedgerPageV1{}, errors.Join(
			accountFlowResolverMismatchV1("account-flow source-row data page does not match its index descriptor"),
			err,
			exactErr,
		)
	}
	resolver.pages[descriptor.DescriptorDigest] = page
	return page, nil
}

func (resolver *accountFlowSourceRowResolverV1) readExactTwiceV1(
	kind datasetsnapshotport.MaterialKindV2,
	reference datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	if reference.Address != reference.SHA256 || !domainsecurity.IsSHA256Hex(reference.SHA256) ||
		reference.ByteLength == 0 {
		return nil, accountFlowResolverMismatchV1("account-flow source-row exact material reference is invalid")
	}
	firstSource, err := resolver.materials.ResolveExact(resolver.ctx, kind, reference)
	if err != nil {
		return nil, errors.Join(
			accountFlowResolverUnavailableV1("account-flow source-row exact material is unavailable"),
			err,
		)
	}
	defer clear(firstSource)
	first := append([]byte(nil), firstSource...)
	secondSource, err := resolver.materials.ResolveExact(resolver.ctx, kind, reference)
	if err != nil {
		clear(first)
		return nil, errors.Join(
			accountFlowResolverUnavailableV1("account-flow source-row exact material replay is unavailable"),
			err,
		)
	}
	defer clear(secondSource)
	second := append([]byte(nil), secondSource...)
	defer clear(second)
	if !bytes.Equal(first, second) || uint64(len(first)) != reference.ByteLength ||
		domainsecurity.SHA256Hex(first) != reference.SHA256 {
		clear(first)
		return nil, accountFlowResolverCorruptV1("account-flow source-row exact material replay is inconsistent")
	}
	return first, nil
}

func exactIndexDescriptorForLocatorV1(
	root domainevidence.SourceRowLedgerRootV1,
	locator domainevidence.SourceRowLocatorV1,
) (domainevidence.SourceRowLedgerIndexPageDescriptorV1, error) {
	var match domainevidence.SourceRowLedgerIndexPageDescriptorV1
	matches := 0
	for _, descriptor := range root.IndexPageDescriptors {
		if sourceRowLocatorWithinV1(
			locator,
			descriptor.FirstSourceFileIDDigest,
			descriptor.FirstSourceRowNumber,
			descriptor.LastSourceFileIDDigest,
			descriptor.LastSourceRowNumber,
		) {
			match = descriptor
			matches++
		}
	}
	if matches != 1 {
		return domainevidence.SourceRowLedgerIndexPageDescriptorV1{},
			accountFlowResolverMismatchV1("account-flow source-row locator has no unique root index match")
	}
	return match, nil
}

func exactDataPageDescriptorForLocatorV1(
	indexPage domainevidence.SourceRowLedgerIndexPageV1,
	locator domainevidence.SourceRowLocatorV1,
) (domainevidence.SourceRowLedgerPageDescriptorV1, error) {
	var match domainevidence.SourceRowLedgerPageDescriptorV1
	matches := 0
	for _, descriptor := range indexPage.PageDescriptors {
		if sourceRowLocatorWithinV1(
			locator,
			descriptor.FirstSourceFileIDDigest,
			descriptor.FirstSourceRowNumber,
			descriptor.LastSourceFileIDDigest,
			descriptor.LastSourceRowNumber,
		) {
			match = descriptor
			matches++
		}
	}
	if matches != 1 {
		return domainevidence.SourceRowLedgerPageDescriptorV1{},
			accountFlowResolverMismatchV1("account-flow source-row locator has no unique data-page match")
	}
	return match, nil
}

func exactRecordForLocatorV1(
	page domainevidence.SourceRowLedgerPageV1,
	locator domainevidence.SourceRowLocatorV1,
) (domainevidence.SourceRowRecordV1, error) {
	var match domainevidence.SourceRowRecordV1
	matches := 0
	for _, entry := range page.Entries {
		if entry.Record.Locator == locator {
			match = entry.Record
			matches++
		}
	}
	if matches != 1 {
		return domainevidence.SourceRowRecordV1{},
			accountFlowResolverMismatchV1("account-flow source-row locator has no unique ledger record")
	}
	return match, nil
}

func sourceRowLocatorWithinV1(
	locator domainevidence.SourceRowLocatorV1,
	firstDigest string,
	firstRow uint64,
	lastDigest string,
	lastRow uint64,
) bool {
	return compareSourceRowLocatorV1(locator.SourceFileIDDigest, locator.SourceRowNumber, firstDigest, firstRow) >= 0 &&
		compareSourceRowLocatorV1(locator.SourceFileIDDigest, locator.SourceRowNumber, lastDigest, lastRow) <= 0
}

func compareSourceRowLocatorV1(leftDigest string, leftRow uint64, rightDigest string, rightRow uint64) int {
	if leftDigest < rightDigest {
		return -1
	}
	if leftDigest > rightDigest {
		return 1
	}
	if leftRow < rightRow {
		return -1
	}
	if leftRow > rightRow {
		return 1
	}
	return 0
}

func exactAccountFlowMaterialReferenceV1(body []byte) datasetsnapshotport.ExactMaterialReferenceV2 {
	digest := domainsecurity.SHA256Hex(body)
	return datasetsnapshotport.ExactMaterialReferenceV2{
		Address: digest, SHA256: digest, ByteLength: uint64(len(body)),
	}
}

func accountFlowResolverUnavailableV1(message string) error {
	return errors.Join(
		fundsquerysourceport.ErrUnavailable,
		datasetsnapshotport.ErrUnavailable,
		errors.New(message),
	)
}

func accountFlowResolverMismatchV1(message string) error {
	return errors.Join(
		fundsquerysourceport.ErrMismatch,
		datasetsnapshotport.ErrMismatch,
		errors.New(message),
	)
}

func accountFlowResolverCorruptV1(message string) error {
	return errors.Join(
		fundsquerysourceport.ErrMismatch,
		datasetsnapshotport.ErrCorrupt,
		errors.New(message),
	)
}

var _ domainnative.AccountFlowSourceRowResolverV1 = (*accountFlowSourceRowResolverV1)(nil).resolve
