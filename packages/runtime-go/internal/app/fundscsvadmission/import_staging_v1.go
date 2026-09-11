package fundscsvadmission

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"

	"golang.org/x/text/cases"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	ImportStagingMaximumSourceBytesV1   = uint64(64 * 1024 * 1024)
	ImportStagingMaximumMembersV1       = 16
	ImportStagingMaximumMemberBytesV1   = uint64(48 * 1024 * 1024)
	ImportStagingMaximumExpandedBytesV1 = uint64(64 * 1024 * 1024)
	ImportStagingMaximumExpandedRatioV1 = uint64(200)
	ImportStagingRatioAllowanceBytesV1  = uint64(1 * 1024 * 1024)
	ImportStagingMaximumRowsV1          = uint64(100_000)
	ImportStagingMaximumColumnsV1       = 64
	ImportStagingMaximumCellBytesV1     = uint64(domainlocaldisplay.MaximumCellBytesV1)
	ImportStagingMaximumHeaderBytesV1   = uint64(16 * 1024)
	ImportStagingLifetimeV1             = 10 * time.Minute
	importStagingParserPolicyV1         = "analytix.host-import-parser/utf8-bom-or-gb18030-strict/csv-rfc4180-width-exact/v1"
	importStagingMappingPolicyV1        = "analytix.host-import-mapping/funds-canonical-34-closed-synonyms/v1"
	importStagingInventoryPolicyV1      = "analytix.host-import-inventory/stable-private-source-items/v1"
	importStagingReadyV1                = "ready"
	importStagingMappingInvalidV1       = "mapping_invalid"
)

type ImportSourceItemResultV1 struct {
	Selector    string `json:"selector"`
	SourceIndex uint16 `json:"sourceIndex"`
	SourceCount uint16 `json:"sourceCount"`
	SourceLabel string `json:"sourceLabel"`
	RowCount    uint64 `json:"rowCount"`
	ColumnCount uint16 `json:"columnCount"`
	Status      string `json:"status"`
}

type ImportStageResultV1 struct {
	Status        string                     `json:"status"`
	TotalRowCount uint64                     `json:"totalRowCount"`
	Items         []ImportSourceItemResultV1 `json:"items"`
}

type ImportStatusResultV1 struct {
	Status        string                   `json:"status"`
	TotalRowCount uint64                   `json:"totalRowCount"`
	Item          ImportSourceItemResultV1 `json:"item"`
}

type ImportCancelResultV1 struct {
	Canceled bool `json:"canceled"`
}

type stagedMappingColumnV1 struct {
	sourceColumn  []byte
	sampleValue   []byte
	inferredType  string
	targetField   string
	parseStatus   string
	mappingStatus string
}

type stagedImportItemV1 struct {
	selector             string
	sourceItemGeneration string
	mappingGeneration    string
	rowCount             uint64
	columns              []stagedMappingColumnV1
	status               string
}

type activeImportGenerationV1 struct {
	workspace        string
	format           string
	observation      domainsecurity.CaseBindingObservationV1
	principal        domainidentity.PrincipalV1
	expiresAt        time.Time
	importGeneration string
	parserGeneration string
	inventoryDigest  string
	sourceSHA256     string
	canonicalSHA256  string
	canonicalBody    []byte
	rowCount         uint64
	items            []stagedImportItemV1
	status           string
	revalidate       FrozenSourceRevalidatorV1
	confirming       bool
	confirmCancel    context.CancelFunc
}

type parsedImportV1 struct {
	items           []parsedImportItemV1
	inventoryDigest string
	canonicalBody   []byte
	canonicalSHA256 string
	rowCount        uint64
	status          string
}

type parsedImportItemV1 struct {
	privateOrderKey string
	bodySHA256      string
	rowCount        uint64
	columns         []stagedMappingColumnV1
	mappingDigest   string
	canonicalRows   []byte
	status          string
}

type stageInvocationV1 struct {
	epoch  uint64
	cancel context.CancelFunc
}

func (service *ServiceV1) StageMainSelectedImportV1(
	ctx context.Context,
	input StageInputV1,
) (ImportStageResultV1, error) {
	if service == nil || ctx == nil || ctx.Err() != nil || service.identity == nil ||
		service.readImportSource == nil || service.now == nil || service.random == nil {
		return ImportStageResultV1{}, ErrUnavailable
	}
	service.stagingMu.Lock()
	stageContext, invocation, beginErr := service.beginStageInvocationLockedV1(ctx)
	service.stagingMu.Unlock()
	if beginErr != nil {
		return ImportStageResultV1{}, beginErr
	}
	defer service.finishStageInvocationV1(invocation)

	principal, observation, err := service.resolveImportAuthorityV1(stageContext, input.WorkspaceRoot)
	if err != nil {
		return ImportStageResultV1{}, err
	}
	var result ImportStageResultV1
	err = service.readImportSource(stageContext, input.WorkspaceRoot, input.SourcePath, func(
		readContext context.Context,
		workspace string,
		format string,
		sourceBody []byte,
		sourceSHA256 string,
		revalidate func(context.Context, func(context.Context, []byte, string) error) error,
	) error {
		if readContext == nil || stageContext.Err() != nil || readContext.Err() != nil ||
			workspace != observation.WorkspaceRealPath || uint64(len(sourceBody)) == 0 ||
			uint64(len(sourceBody)) > ImportStagingMaximumSourceBytesV1 ||
			!domainsecurity.IsSHA256Hex(sourceSHA256) || revalidate == nil {
			return ErrInvalidRequest
		}
		parsed, parseErr := parseImportSourceV1(stageContext, format, sourceBody)
		if parseErr != nil {
			return errors.Join(ErrInvalidRequest, parseErr)
		}
		defer clearParsedImportV1(&parsed)
		if stageContext.Err() != nil {
			return stageContext.Err()
		}
		if service.identity.ValidateCurrent(stageContext, principal) != nil {
			return ErrUnavailable
		}
		current, observeErr := service.observer.Observe(workspace)
		if observeErr != nil || current != observation {
			return ErrInvalidRequest
		}
		nonce := make([]byte, 32)
		if _, randomErr := io.ReadFull(service.random, nonce); randomErr != nil {
			return errors.Join(ErrUnavailable, randomErr)
		}
		defer clear(nonce)
		if stageContext.Err() != nil {
			return stageContext.Err()
		}
		parserGeneration := generationV1(importStagingParserPolicyV1)
		importGeneration := generationV1(
			"analytix.host-import-generation/v1", principal.PrincipalDigest,
			observation.ObservationDigest, sourceSHA256, parsed.inventoryDigest,
			parserGeneration, importStagingMappingPolicyV1, string(nonce),
		)
		active := &activeImportGenerationV1{
			workspace: workspace, format: format, observation: observation, principal: principal,
			expiresAt:        service.now().UTC().Add(ImportStagingLifetimeV1),
			importGeneration: importGeneration, parserGeneration: parserGeneration,
			inventoryDigest: parsed.inventoryDigest, sourceSHA256: sourceSHA256,
			canonicalSHA256: parsed.canonicalSHA256,
			canonicalBody:   append([]byte(nil), parsed.canonicalBody...), rowCount: parsed.rowCount,
			status: parsed.status, revalidate: revalidate,
			items: make([]stagedImportItemV1, len(parsed.items)),
		}
		for index := range parsed.items {
			parsedItem := &parsed.items[index]
			sourceGeneration := generationV1(
				"analytix.host-import-source-item/v1", importGeneration,
				fmt.Sprintf("%d", index+1), parsedItem.privateOrderKey,
				parsedItem.bodySHA256, parsedItem.mappingDigest,
			)
			selector := "tlsel1_" + digestFieldsV1(
				"analytix.host-import-selector/v1", importGeneration, sourceGeneration,
			)
			active.items[index] = stagedImportItemV1{
				selector: selector, sourceItemGeneration: sourceGeneration,
				mappingGeneration: generationV1(importStagingMappingPolicyV1, parsedItem.mappingDigest),
				rowCount:          parsedItem.rowCount, status: parsedItem.status,
				columns: cloneStagedMappingColumnsV1(parsedItem.columns),
			}
		}
		service.stagingMu.Lock()
		defer service.stagingMu.Unlock()
		if service.stageInFlight != invocation || service.stageEpoch != invocation.epoch || stageContext.Err() != nil ||
			service.identity.ValidateCurrent(stageContext, principal) != nil {
			clearActiveImportGenerationV1(active)
			return ErrUnavailable
		}
		latest, latestErr := service.observer.Observe(workspace)
		if latestErr != nil || latest != observation {
			clearActiveImportGenerationV1(active)
			return ErrInvalidRequest
		}
		service.activeImport = active
		generation := active.importGeneration
		service.stagingExpiryTimer = time.AfterFunc(ImportStagingLifetimeV1, func() {
			service.stagingMu.Lock()
			defer service.stagingMu.Unlock()
			if service.activeImport != nil && service.activeImport.importGeneration == generation {
				service.clearActiveImportLockedV1()
			}
		})
		result = stageResultFromActiveV1(active)
		return nil
	})
	if err != nil {
		return ImportStageResultV1{}, normalizeImportErrorV1(err)
	}
	service.stagingMu.Lock()
	stillCurrent := service.stageInFlight == invocation && service.stageEpoch == invocation.epoch &&
		stageContext.Err() == nil && service.activeImport != nil &&
		service.activeImport.importGeneration != "" && len(result.Items) != 0
	service.stagingMu.Unlock()
	if !stillCurrent {
		return ImportStageResultV1{}, ErrUnavailable
	}
	return result, nil
}

func (service *ServiceV1) ConfirmImportV1(
	ctx context.Context,
	selector string,
) (StageResultV1, error) {
	if service == nil || !domainlocaldisplay.ValidSelectorV1(selector) {
		return StageResultV1{}, ErrInvalidRequest
	}
	if ctx == nil || ctx.Err() != nil {
		service.stagingMu.Lock()
		if service.activeImport != nil && currentImportItemV1(service.activeImport, selector) != nil {
			service.clearActiveImportLockedV1()
		}
		service.stagingMu.Unlock()
		return StageResultV1{}, ErrInvalidRequest
	}
	service.stagingMu.Lock()
	active, err := service.currentImportLockedV1(ctx, selector)
	if err != nil {
		service.stagingMu.Unlock()
		return StageResultV1{}, err
	}
	if active.confirming || active.status != importStagingReadyV1 || len(active.canonicalBody) == 0 ||
		service.commitExact == nil {
		service.clearActiveImportLockedV1()
		service.stagingMu.Unlock()
		return StageResultV1{}, ErrInvalidRequest
	}
	confirmContext, confirmCancel := context.WithCancel(ctx)
	active.confirming = true
	active.confirmCancel = confirmCancel
	workspace := active.workspace
	format := active.format
	stagedObservation := active.observation
	stagedPrincipal := active.principal
	stagedInventoryDigest := active.inventoryDigest
	stagedSourceSHA256 := active.sourceSHA256
	stagedCanonicalSHA256 := active.canonicalSHA256
	stagedCanonicalBody := append([]byte(nil), active.canonicalBody...)
	stagedRowCount := active.rowCount
	revalidate := active.revalidate
	service.stagingMu.Unlock()
	defer func() {
		confirmCancel()
		clear(stagedCanonicalBody)
		service.stagingMu.Lock()
		defer service.stagingMu.Unlock()
		if service.activeImport == active {
			service.clearActiveImportLockedV1()
		}
	}()
	var reparsed parsedImportV1
	err = revalidate(confirmContext, func(revalidateContext context.Context, body []byte, sourceSHA256 string) error {
		if sourceSHA256 != stagedSourceSHA256 {
			return ErrInvalidRequest
		}
		var parseErr error
		reparsed, parseErr = parseImportSourceV1(revalidateContext, format, body)
		return parseErr
	})
	if err != nil {
		clearParsedImportV1(&reparsed)
		return StageResultV1{}, normalizeImportErrorV1(err)
	}
	defer clearParsedImportV1(&reparsed)
	if reparsed.status != importStagingReadyV1 || reparsed.inventoryDigest != stagedInventoryDigest ||
		reparsed.canonicalSHA256 != stagedCanonicalSHA256 || reparsed.rowCount != stagedRowCount ||
		!bytes.Equal(reparsed.canonicalBody, stagedCanonicalBody) {
		return StageResultV1{}, ErrInvalidRequest
	}
	currentPrincipal, currentObservation, authorityErr := service.resolveImportAuthorityV1(confirmContext, workspace)
	if authorityErr != nil || !domainidentity.SamePrincipalV1(currentPrincipal, stagedPrincipal) ||
		currentObservation != stagedObservation {
		if authorityErr != nil {
			return StageResultV1{}, authorityErr
		}
		return StageResultV1{}, ErrUnavailable
	}
	service.stagingMu.Lock()
	stillCurrent := service.activeImport == active && active.confirming &&
		active.importGeneration != "" && currentImportItemV1(active, selector) != nil
	service.stagingMu.Unlock()
	if !stillCurrent || confirmContext.Err() != nil {
		return StageResultV1{}, ErrUnavailable
	}
	// commitExact owns the DSV2 linearization point. A successful return is not
	// rolled back if the caller is canceled or loses the HTTP response afterward;
	// only a failure proven before its AdmitExactV2 CAS preserves the old head.
	result, err := service.commitExact(
		confirmContext, workspace, stagedCanonicalBody, stagedCanonicalSHA256, stagedRowCount,
	)
	if err != nil {
		return StageResultV1{}, err
	}
	if result.SourceArtifactSHA256 != stagedCanonicalSHA256 ||
		result.SourceArtifactByteLength != uint64(len(stagedCanonicalBody)) ||
		result.SourceRowCount != stagedRowCount {
		return StageResultV1{}, ErrUnavailable
	}
	return result, nil
}

func (service *ServiceV1) CancelImportV1(
	ctx context.Context,
	selector string,
) (ImportCancelResultV1, error) {
	if service == nil || ctx == nil || ctx.Err() != nil || !domainlocaldisplay.ValidSelectorV1(selector) {
		return ImportCancelResultV1{}, ErrInvalidRequest
	}
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	if _, err := service.currentImportLockedV1(ctx, selector); err != nil {
		return ImportCancelResultV1{}, err
	}
	service.clearActiveImportLockedV1()
	return ImportCancelResultV1{Canceled: true}, nil
}

func (service *ServiceV1) ImportStatusV1(
	ctx context.Context,
	selector string,
) (ImportStatusResultV1, error) {
	if service == nil || ctx == nil || ctx.Err() != nil || !domainlocaldisplay.ValidSelectorV1(selector) {
		return ImportStatusResultV1{}, ErrInvalidRequest
	}
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	active, err := service.currentImportLockedV1(ctx, selector)
	if err != nil {
		return ImportStatusResultV1{}, err
	}
	if active.confirming {
		return ImportStatusResultV1{}, ErrUnavailable
	}
	for index := range active.items {
		if active.items[index].selector == selector {
			return ImportStatusResultV1{
				Status: active.status, TotalRowCount: active.rowCount,
				Item: importSourceItemResultV1(active, index),
			}, nil
		}
	}
	return ImportStatusResultV1{}, ErrInvalidRequest
}

func (service *ServiceV1) UseCurrentImportMappingPreviewV1(
	ctx context.Context,
	principal domainidentity.PrincipalV1,
	selector string,
	fields []string,
	rowOffset uint32,
	rowLimit uint16,
	use func(context.Context, domainlocaldisplay.ImportMappingAuthorityV1, []domainlocaldisplay.ImportMappingRowV1, bool) error,
) error {
	if service == nil || ctx == nil || use == nil || rowLimit == 0 ||
		domainidentity.ValidatePrincipalV1(principal) != nil ||
		domainlocaldisplay.ValidateFieldsV1(fields, domainlocaldisplay.ImportMappingPreviewFieldsV1()) != nil {
		return ErrInvalidRequest
	}
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	active, err := service.currentImportLockedV1(ctx, selector)
	if err != nil || active.confirming || !domainidentity.SamePrincipalV1(principal, active.principal) {
		return ErrUnavailable
	}
	item := currentImportItemV1(active, selector)
	if item == nil || uint64(rowOffset) > uint64(len(item.columns)) {
		return ErrInvalidRequest
	}
	end := uint64(rowOffset) + uint64(rowLimit)
	if end > uint64(len(item.columns)) {
		end = uint64(len(item.columns))
	}
	rows := make([]domainlocaldisplay.ImportMappingRowV1, 0, end-uint64(rowOffset))
	for index := uint64(rowOffset); index < end; index++ {
		column := &item.columns[index]
		cells := make([]domainlocaldisplay.ImportMappingCellV1, len(fields))
		for fieldIndex, field := range fields {
			value, valueErr := domainlocaldisplay.NewExactValueV1(importColumnFieldV1(column, field))
			if valueErr != nil {
				return ErrUnavailable
			}
			cells[fieldIndex] = domainlocaldisplay.ImportMappingCellV1{Field: field, Value: value}
		}
		rows = append(rows, domainlocaldisplay.ImportMappingRowV1{
			RowIndex: uint32(index), ParseStatus: column.parseStatus,
			MappingStatus: column.mappingStatus, Cells: cells,
		})
	}
	authority := domainlocaldisplay.ImportMappingAuthorityV1{
		Selector: selector, ImportGeneration: active.importGeneration,
		SourceItemGeneration: item.sourceItemGeneration,
		ParserGeneration:     active.parserGeneration, MappingGeneration: item.mappingGeneration,
	}
	if err := use(ctx, authority, rows, end < uint64(len(item.columns))); err != nil {
		return err
	}
	if service.identity.ValidateCurrent(ctx, active.principal) != nil {
		return ErrUnavailable
	}
	latest, observeErr := service.observer.Observe(active.workspace)
	if observeErr != nil || latest != active.observation {
		return ErrUnavailable
	}
	return nil
}

func (service *ServiceV1) ValidateCurrentImportMappingPreviewV1(
	ctx context.Context,
	principal domainidentity.PrincipalV1,
	authority domainlocaldisplay.ImportMappingAuthorityV1,
) error {
	if service == nil || ctx == nil || domainlocaldisplay.ValidateImportMappingAuthorityV1(authority) != nil ||
		domainidentity.ValidatePrincipalV1(principal) != nil {
		return ErrInvalidRequest
	}
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	active, err := service.currentImportLockedV1(ctx, authority.Selector)
	if err != nil || active.confirming || !domainidentity.SamePrincipalV1(principal, active.principal) {
		return ErrUnavailable
	}
	item := currentImportItemV1(active, authority.Selector)
	if item == nil || authority.ImportGeneration != active.importGeneration ||
		authority.ParserGeneration != active.parserGeneration ||
		authority.SourceItemGeneration != item.sourceItemGeneration ||
		authority.MappingGeneration != item.mappingGeneration {
		return ErrUnavailable
	}
	return nil
}

func (service *ServiceV1) Close() error {
	if service == nil {
		return nil
	}
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	service.cancelStageInvocationLockedV1()
	service.clearActiveImportLockedV1()
	return nil
}

func (service *ServiceV1) beginStageInvocationLockedV1(ctx context.Context) (
	context.Context,
	*stageInvocationV1,
	error,
) {
	service.cancelStageInvocationLockedV1()
	service.clearActiveImportLockedV1()
	if service.stageEpoch == ^uint64(0) {
		return nil, nil, ErrUnavailable
	}
	service.stageEpoch++
	stageContext, cancel := context.WithCancel(ctx)
	invocation := &stageInvocationV1{epoch: service.stageEpoch, cancel: cancel}
	service.stageInFlight = invocation
	return stageContext, invocation, nil
}

func (service *ServiceV1) finishStageInvocationV1(invocation *stageInvocationV1) {
	if invocation == nil {
		return
	}
	invocation.cancel()
	service.stagingMu.Lock()
	defer service.stagingMu.Unlock()
	if service.stageInFlight == invocation && service.stageEpoch == invocation.epoch {
		service.stageInFlight = nil
	}
}

func (service *ServiceV1) cancelStageInvocationLockedV1() {
	if service.stageInFlight == nil {
		return
	}
	service.stageInFlight.cancel()
	service.stageInFlight = nil
}

func (service *ServiceV1) resolveImportAuthorityV1(
	ctx context.Context,
	workspace string,
) (domainidentity.PrincipalV1, domainsecurity.CaseBindingObservationV1, error) {
	principal, err := service.identity.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil ||
		service.identity.ValidateCurrent(ctx, principal) != nil {
		return domainidentity.PrincipalV1{}, domainsecurity.CaseBindingObservationV1{}, ErrUnavailable
	}
	observation, err := service.observer.Observe(workspace)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.State != domainsecurity.CaseBindingStateValid ||
		observation.WorkspaceRealPath != workspace {
		return domainidentity.PrincipalV1{}, domainsecurity.CaseBindingObservationV1{}, ErrInvalidRequest
	}
	return principal, observation, nil
}

func (service *ServiceV1) currentImportLockedV1(
	ctx context.Context,
	selector string,
) (*activeImportGenerationV1, error) {
	active := service.activeImport
	if active == nil {
		return nil, ErrUnavailable
	}
	if service.now().UTC().After(active.expiresAt) {
		service.clearActiveImportLockedV1()
		return nil, ErrUnavailable
	}
	if !domainlocaldisplay.ValidSelectorV1(selector) {
		return nil, ErrUnavailable
	}
	if service.identity.ValidateCurrent(ctx, active.principal) != nil {
		service.clearActiveImportLockedV1()
		return nil, ErrUnavailable
	}
	observation, err := service.observer.Observe(active.workspace)
	if err != nil || observation != active.observation {
		service.clearActiveImportLockedV1()
		return nil, ErrUnavailable
	}
	if currentImportItemV1(active, selector) == nil {
		// A stale or foreign selector has no authority over the current
		// generation. Reject it without revoking a newer reselection.
		return nil, ErrUnavailable
	}
	return active, nil
}

func (service *ServiceV1) clearActiveImportLockedV1() {
	if service.stagingExpiryTimer != nil {
		service.stagingExpiryTimer.Stop()
		service.stagingExpiryTimer = nil
	}
	clearActiveImportGenerationV1(service.activeImport)
	service.activeImport = nil
}

func clearActiveImportGenerationV1(active *activeImportGenerationV1) {
	if active == nil {
		return
	}
	if active.confirmCancel != nil {
		active.confirmCancel()
		active.confirmCancel = nil
	}
	active.confirming = false
	clear(active.canonicalBody)
	active.canonicalBody = nil
	for itemIndex := range active.items {
		for columnIndex := range active.items[itemIndex].columns {
			clear(active.items[itemIndex].columns[columnIndex].sourceColumn)
			clear(active.items[itemIndex].columns[columnIndex].sampleValue)
		}
		active.items[itemIndex].columns = nil
	}
	active.items = nil
	active.revalidate = nil
	active.sourceSHA256 = ""
	active.canonicalSHA256 = ""
}

func parseImportSourceV1(ctx context.Context, format string, source []byte) (parsedImportV1, error) {
	if ctx == nil || ctx.Err() != nil || len(source) == 0 || uint64(len(source)) > ImportStagingMaximumSourceBytesV1 {
		return parsedImportV1{}, ErrInvalidRequest
	}
	var members []importMemberV1
	switch format {
	case "csv":
		if len(source) >= 4 && bytes.HasPrefix(source, []byte("PK")) {
			return parsedImportV1{}, errors.New("import source format is mismatched")
		}
		members = []importMemberV1{{orderKey: "plain-source", body: append([]byte(nil), source...)}}
	case "zip":
		var err error
		members, err = readZIPMembersV1(ctx, source)
		if err != nil {
			return parsedImportV1{}, err
		}
	default:
		return parsedImportV1{}, errors.New("import source format is unsupported")
	}
	defer clearImportMembersV1(members)

	parsed := parsedImportV1{items: make([]parsedImportItemV1, 0, len(members)), status: importStagingReadyV1}
	canonicalColumns := domainevidence.FundsTransactionCSVColumnsV1()
	var canonical bytes.Buffer
	writer := csv.NewWriter(&canonical)
	writer.UseCRLF = true
	headers := make([]string, len(canonicalColumns))
	for index := range canonicalColumns {
		headers[index] = canonicalColumns[index].Header
	}
	if err := writer.Write(headers); err != nil {
		return parsedImportV1{}, err
	}
	writer.Flush()
	if writer.Error() != nil {
		return parsedImportV1{}, writer.Error()
	}
	for index := range members {
		if ctx.Err() != nil {
			return parsedImportV1{}, ctx.Err()
		}
		item, err := parseCSVMemberV1(ctx, members[index], canonicalColumns)
		if err != nil {
			clearParsedImportV1(&parsed)
			return parsedImportV1{}, err
		}
		if item.status != importStagingReadyV1 {
			parsed.status = importStagingMappingInvalidV1
		}
		if parsed.rowCount > ImportStagingMaximumRowsV1-item.rowCount {
			clearParsedImportItemV1(&item)
			clearParsedImportV1(&parsed)
			return parsedImportV1{}, errors.New("import row limit exceeded")
		}
		parsed.rowCount += item.rowCount
		if canonical.Len()+len(item.canonicalRows) > int(domainnative.FundsCanonicalCSVMaximumSourceBytesV1) {
			clearParsedImportItemV1(&item)
			clearParsedImportV1(&parsed)
			return parsedImportV1{}, errors.New("canonical import size limit exceeded")
		}
		_, _ = canonical.Write(item.canonicalRows)
		parsed.items = append(parsed.items, item)
	}
	if parsed.rowCount == 0 || canonical.Len() > int(domainnative.FundsCanonicalCSVMaximumSourceBytesV1) {
		clearParsedImportV1(&parsed)
		return parsedImportV1{}, errors.New("canonical import is invalid")
	}
	parsed.canonicalBody = append([]byte(nil), canonical.Bytes()...)
	parsed.canonicalSHA256 = domainsecurity.SHA256Hex(parsed.canonicalBody)
	inventoryFields := []string{importStagingInventoryPolicyV1, format, fmt.Sprintf("%d", len(parsed.items))}
	for index := range parsed.items {
		item := &parsed.items[index]
		inventoryFields = append(inventoryFields, fmt.Sprintf("%d", index+1), item.privateOrderKey,
			item.bodySHA256, fmt.Sprintf("%d", item.rowCount), fmt.Sprintf("%d", len(item.columns)), item.mappingDigest)
	}
	parsed.inventoryDigest = digestFieldsV1(inventoryFields...)
	return parsed, nil
}

type importMemberV1 struct {
	orderKey string
	body     []byte
}

func readZIPMembersV1(ctx context.Context, source []byte) ([]importMemberV1, error) {
	if err := preflightZIPArchiveV1(source); err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > ImportStagingMaximumMembersV1 {
		return nil, errors.New("ZIP inventory is invalid")
	}
	members := make([]importMemberV1, 0, len(reader.File))
	seen := make(map[string]struct{}, len(reader.File))
	var aggregate uint64
	for _, file := range reader.File {
		if ctx.Err() != nil {
			clearImportMembersV1(members)
			return nil, ctx.Err()
		}
		orderKey, valid := validZIPMemberNameV1(file.Name)
		mode := file.Mode()
		if !valid || file.NonUTF8 || !validZIPFlagsV1(file.Flags, file.Method) ||
			(file.Method != zip.Store && file.Method != zip.Deflate) ||
			mode&(^mode.Perm()) != 0 || !mode.IsRegular() || hasZIP64ExtraV1(file.Extra) ||
			// Slice 5 admits only non-empty CSV members. A zero compressed size is
			// not, by itself, a general ZIP-format error.
			file.UncompressedSize64 == 0 || file.UncompressedSize64 > ImportStagingMaximumMemberBytesV1 ||
			file.CompressedSize64 == 0 || file.CompressedSize64 > ImportStagingMaximumSourceBytesV1 ||
			expansionRatioExceededV1(file.UncompressedSize64, file.CompressedSize64) ||
			!strings.EqualFold(path.Ext(file.Name), ".csv") {
			clearImportMembersV1(members)
			return nil, errors.New("ZIP member is unsupported")
		}
		if _, duplicate := seen[orderKey]; duplicate {
			clearImportMembersV1(members)
			return nil, errors.New("ZIP member identity collides")
		}
		seen[orderKey] = struct{}{}
		nextAggregate, fits := addImportExpandedBytesV1(aggregate, file.UncompressedSize64)
		if !fits {
			clearImportMembersV1(members)
			return nil, errors.New("ZIP expanded size limit exceeded")
		}
		aggregate = nextAggregate
		opened, openErr := file.Open()
		if openErr != nil {
			clearImportMembersV1(members)
			return nil, errors.New("ZIP member cannot be opened")
		}
		body, readErr := io.ReadAll(io.LimitReader(&importContextReaderV1{ctx: ctx, reader: opened}, int64(ImportStagingMaximumMemberBytesV1)+1))
		closeErr := opened.Close()
		if readErr != nil || closeErr != nil || uint64(len(body)) != file.UncompressedSize64 {
			clear(body)
			clearImportMembersV1(members)
			return nil, errors.New("ZIP member identity is invalid")
		}
		members = append(members, importMemberV1{orderKey: orderKey, body: body})
	}
	if aggregate > ImportStagingMaximumExpandedBytesV1 ||
		expansionRatioExceededV1(aggregate, uint64(len(source))) {
		clearImportMembersV1(members)
		return nil, errors.New("ZIP expansion budget exceeded")
	}
	sort.Slice(members, func(left, right int) bool { return members[left].orderKey < members[right].orderKey })
	return members, nil
}

func preflightZIPArchiveV1(source []byte) error {
	const (
		zipCentralHeaderLength = uint64(46)
		zipLocalHeaderLength   = uint64(30)
		zipCentralSignature    = uint32(0x02014b50)
		zipLocalSignature      = uint32(0x04034b50)
		zip16Sentinel          = uint16(0xffff)
		zip32Sentinel          = uint32(0xffffffff)
	)
	if len(source) == 0 || uint64(len(source)) > ImportStagingMaximumSourceBytesV1 {
		return errors.New("ZIP source size is invalid")
	}
	eocd := findZIPEndOfCentralDirectoryV1(source)
	if eocd < 0 {
		return errors.New("ZIP end record is invalid")
	}
	if containsZIP64StructureV1(source) {
		return errors.New("ZIP64 archives are unsupported")
	}
	disk := binary.LittleEndian.Uint16(source[eocd+4 : eocd+6])
	centralDisk := binary.LittleEndian.Uint16(source[eocd+6 : eocd+8])
	entriesOnDisk := binary.LittleEndian.Uint16(source[eocd+8 : eocd+10])
	entries := binary.LittleEndian.Uint16(source[eocd+10 : eocd+12])
	centralSize32 := binary.LittleEndian.Uint32(source[eocd+12 : eocd+16])
	centralOffset32 := binary.LittleEndian.Uint32(source[eocd+16 : eocd+20])
	if disk != 0 || centralDisk != 0 || entriesOnDisk != entries || entries == 0 ||
		entries > ImportStagingMaximumMembersV1 || entries == zip16Sentinel ||
		centralSize32 == zip32Sentinel || centralOffset32 == zip32Sentinel {
		return errors.New("ZIP central inventory is invalid")
	}
	centralOffset := uint64(centralOffset32)
	centralSize := uint64(centralSize32)
	centralEnd, ok := checkedZIPRangeEndV1(centralOffset, centralSize, uint64(eocd))
	minimumCentralSize := uint64(entries) * zipCentralHeaderLength
	if !ok || centralEnd != uint64(eocd) || centralSize < minimumCentralSize {
		return errors.New("ZIP central directory is invalid")
	}

	cursor := centralOffset
	for index := uint16(0); index < entries; index++ {
		headerEnd, fits := checkedZIPRangeEndV1(cursor, zipCentralHeaderLength, centralEnd)
		if !fits || binary.LittleEndian.Uint32(source[int(cursor):int(cursor)+4]) != zipCentralSignature {
			return errors.New("ZIP central member is invalid")
		}
		flags := binary.LittleEndian.Uint16(source[int(cursor)+8 : int(cursor)+10])
		method := binary.LittleEndian.Uint16(source[int(cursor)+10 : int(cursor)+12])
		compressedSize := binary.LittleEndian.Uint32(source[int(cursor)+20 : int(cursor)+24])
		uncompressedSize := binary.LittleEndian.Uint32(source[int(cursor)+24 : int(cursor)+28])
		nameLength := uint64(binary.LittleEndian.Uint16(source[int(cursor)+28 : int(cursor)+30]))
		extraLength := uint64(binary.LittleEndian.Uint16(source[int(cursor)+30 : int(cursor)+32]))
		commentLength := uint64(binary.LittleEndian.Uint16(source[int(cursor)+32 : int(cursor)+34]))
		diskStart := binary.LittleEndian.Uint16(source[int(cursor)+34 : int(cursor)+36])
		localOffset32 := binary.LittleEndian.Uint32(source[int(cursor)+42 : int(cursor)+46])
		variableLength, variableFits := checkedZIPAddV1(nameLength, extraLength, commentLength)
		next, nextFits := checkedZIPRangeEndV1(headerEnd, variableLength, centralEnd)
		if !variableFits || !nextFits || diskStart != 0 || compressedSize == zip32Sentinel ||
			uncompressedSize == zip32Sentinel || localOffset32 == zip32Sentinel ||
			(method != zip.Store && method != zip.Deflate) || !validZIPFlagsV1(flags, method) {
			return errors.New("ZIP central member is unsupported")
		}
		centralNameStart := headerEnd
		centralNameEnd := centralNameStart + nameLength
		centralExtraEnd := centralNameEnd + extraLength
		if hasZIP64ExtraV1(source[int(centralNameEnd):int(centralExtraEnd)]) {
			return errors.New("ZIP64 members are unsupported")
		}

		localOffset := uint64(localOffset32)
		localHeaderEnd, localFits := checkedZIPRangeEndV1(localOffset, zipLocalHeaderLength, centralOffset)
		if !localFits || binary.LittleEndian.Uint32(source[int(localOffset):int(localOffset)+4]) != zipLocalSignature {
			return errors.New("ZIP local member is invalid")
		}
		localFlags := binary.LittleEndian.Uint16(source[int(localOffset)+6 : int(localOffset)+8])
		localMethod := binary.LittleEndian.Uint16(source[int(localOffset)+8 : int(localOffset)+10])
		localCRC := binary.LittleEndian.Uint32(source[int(localOffset)+14 : int(localOffset)+18])
		localCompressedSize := binary.LittleEndian.Uint32(source[int(localOffset)+18 : int(localOffset)+22])
		localUncompressedSize := binary.LittleEndian.Uint32(source[int(localOffset)+22 : int(localOffset)+26])
		localNameLength := uint64(binary.LittleEndian.Uint16(source[int(localOffset)+26 : int(localOffset)+28]))
		localExtraLength := uint64(binary.LittleEndian.Uint16(source[int(localOffset)+28 : int(localOffset)+30]))
		localVariableLength, localVariableFits := checkedZIPAddV1(localNameLength, localExtraLength)
		localDataStart, localDataFits := checkedZIPRangeEndV1(localHeaderEnd, localVariableLength, centralOffset)
		localDataEnd, compressedFits := checkedZIPRangeEndV1(localDataStart, uint64(compressedSize), centralOffset)
		if !localVariableFits || !localDataFits || !compressedFits || localDataEnd > centralOffset ||
			localCompressedSize == zip32Sentinel || localUncompressedSize == zip32Sentinel ||
			localFlags != flags || localMethod != method || localNameLength != nameLength {
			return errors.New("ZIP local and central headers disagree")
		}
		localNameEnd := localHeaderEnd + localNameLength
		localExtraEnd := localNameEnd + localExtraLength
		if !bytes.Equal(source[int(localHeaderEnd):int(localNameEnd)], source[int(centralNameStart):int(centralNameEnd)]) ||
			hasZIP64ExtraV1(source[int(localNameEnd):int(localExtraEnd)]) {
			return errors.New("ZIP local member identity is invalid")
		}
		centralCRC := binary.LittleEndian.Uint32(source[int(cursor)+16 : int(cursor)+20])
		if flags&0x0008 == 0 && (localCRC != centralCRC || localCompressedSize != compressedSize ||
			localUncompressedSize != uncompressedSize) {
			return errors.New("ZIP local member declaration is invalid")
		}
		cursor = next
	}
	if cursor != centralEnd {
		return errors.New("ZIP central member count is invalid")
	}
	return nil
}

func validZIPFlagsV1(flags uint16, method uint16) bool {
	allowed := uint16(0x0008 | 0x0800) // data descriptor and UTF-8 names
	if method == zip.Deflate {
		allowed |= 0x0006 // DEFLATE compression-level options
	}
	return flags & ^allowed == 0
}

func checkedZIPRangeEndV1(start uint64, size uint64, limit uint64) (uint64, bool) {
	if start > limit || size > limit-start {
		return 0, false
	}
	return start + size, true
}

func checkedZIPAddV1(values ...uint64) (uint64, bool) {
	var total uint64
	for _, value := range values {
		if value > ^uint64(0)-total {
			return 0, false
		}
		total += value
	}
	return total, true
}

func containsZIP64StructureV1(source []byte) bool {
	eocd := findZIPEndOfCentralDirectoryV1(source)
	if eocd < 0 {
		return false
	}
	const (
		zip16Sentinel = uint16(0xffff)
		zip32Sentinel = uint32(0xffffffff)
	)
	if binary.LittleEndian.Uint16(source[eocd+8:eocd+10]) == zip16Sentinel ||
		binary.LittleEndian.Uint16(source[eocd+10:eocd+12]) == zip16Sentinel ||
		binary.LittleEndian.Uint32(source[eocd+12:eocd+16]) == zip32Sentinel ||
		binary.LittleEndian.Uint32(source[eocd+16:eocd+20]) == zip32Sentinel {
		return true
	}
	return hasStructuredZIP64LocatorV1(source, eocd)
}

func hasStructuredZIP64LocatorV1(source []byte, eocd int) bool {
	const (
		zip64LocatorLength = 20
		zip64LocatorSig    = uint32(0x07064b50)
		zip64EndSig        = uint32(0x06064b50)
		zip64EndMinimum    = uint64(44)
	)
	locator := eocd - zip64LocatorLength
	if locator < 0 || binary.LittleEndian.Uint32(source[locator:locator+4]) != zip64LocatorSig ||
		binary.LittleEndian.Uint32(source[locator+4:locator+8]) != 0 ||
		binary.LittleEndian.Uint32(source[locator+16:locator+20]) != 1 {
		return false
	}
	zip64Offset := binary.LittleEndian.Uint64(source[locator+8 : locator+16])
	centralSize := uint64(binary.LittleEndian.Uint32(source[eocd+12 : eocd+16]))
	centralOffset := uint64(binary.LittleEndian.Uint32(source[eocd+16 : eocd+20]))
	centralEnd, centralFits := checkedZIPRangeEndV1(centralOffset, centralSize, uint64(locator))
	if !centralFits || centralEnd != zip64Offset || zip64Offset > uint64(locator) ||
		uint64(locator)-zip64Offset < 12 ||
		binary.LittleEndian.Uint32(source[int(zip64Offset):int(zip64Offset)+4]) != zip64EndSig {
		return false
	}
	recordSize := binary.LittleEndian.Uint64(source[int(zip64Offset)+4 : int(zip64Offset)+12])
	recordLength, lengthFits := checkedZIPAddV1(12, recordSize)
	if !lengthFits {
		return false
	}
	recordEnd, recordFits := checkedZIPRangeEndV1(zip64Offset, recordLength, uint64(locator))
	return recordFits && recordSize >= zip64EndMinimum && recordEnd == uint64(locator)
}

func findZIPEndOfCentralDirectoryV1(source []byte) int {
	const (
		zipEndHeaderLength = 22
		zipMaximumComment  = 1<<16 - 1
		zipEndSignature    = uint32(0x06054b50)
	)
	if len(source) < zipEndHeaderLength {
		return -1
	}
	minimum := len(source) - zipEndHeaderLength - zipMaximumComment
	if minimum < 0 {
		minimum = 0
	}
	for offset := len(source) - zipEndHeaderLength; offset >= minimum; offset-- {
		if binary.LittleEndian.Uint32(source[offset:offset+4]) != zipEndSignature {
			continue
		}
		commentLength := int(binary.LittleEndian.Uint16(source[offset+20 : offset+22]))
		if offset+zipEndHeaderLength+commentLength == len(source) {
			return offset
		}
	}
	return -1
}

func validZIPMemberNameV1(name string) (string, bool) {
	if name == "" || !utf8.ValidString(name) || name != norm.NFC.String(name) || strings.Contains(name, "\\") ||
		strings.HasPrefix(name, "/") || strings.HasPrefix(name, "//") ||
		(len(name) >= 2 && unicode.IsLetter(rune(name[0])) && name[1] == ':') {
		return "", false
	}
	segments := strings.Split(name, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." ||
			strings.IndexFunc(segment, func(value rune) bool { return value == 0 || unicode.IsControl(value) }) >= 0 {
			return "", false
		}
	}
	cleaned := path.Clean(name)
	if cleaned != name || cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return norm.NFC.String(cases.Fold().String(cleaned)), true
}

type importContextReaderV1 struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *importContextReaderV1) Read(target []byte) (int, error) {
	if reader == nil || reader.ctx == nil || reader.ctx.Err() != nil {
		return 0, context.Canceled
	}
	return reader.reader.Read(target)
}

func hasZIP64ExtraV1(extra []byte) bool {
	for len(extra) >= 4 {
		identifier := binary.LittleEndian.Uint16(extra[:2])
		length := int(binary.LittleEndian.Uint16(extra[2:4]))
		extra = extra[4:]
		if length > len(extra) {
			return true
		}
		if identifier == 0x0001 {
			return true
		}
		extra = extra[length:]
	}
	return len(extra) != 0
}

func expansionRatioExceededV1(expanded, compressed uint64) bool {
	if expanded <= ImportStagingRatioAllowanceBytesV1 {
		return false
	}
	if compressed == 0 || compressed > (^uint64(0)-ImportStagingRatioAllowanceBytesV1)/ImportStagingMaximumExpandedRatioV1 {
		return compressed == 0
	}
	return expanded > compressed*ImportStagingMaximumExpandedRatioV1+ImportStagingRatioAllowanceBytesV1
}

func addImportExpandedBytesV1(current, next uint64) (uint64, bool) {
	if next > ImportStagingMaximumExpandedBytesV1 ||
		current > ImportStagingMaximumExpandedBytesV1-next {
		return 0, false
	}
	return current + next, true
}

func parseCSVMemberV1(
	ctx context.Context,
	member importMemberV1,
	canonicalColumns []domainevidence.FundsTransactionCSVColumnV1,
) (parsedImportItemV1, error) {
	decoded, err := decodeImportTextV1(member.body)
	if err != nil {
		return parsedImportItemV1{}, err
	}
	defer clear(decoded)
	reader := csv.NewReader(bytes.NewReader(decoded))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	reader.LazyQuotes = false
	reader.TrimLeadingSpace = false
	header, err := reader.Read()
	if err != nil || len(header) == 0 || len(header) > ImportStagingMaximumColumnsV1 ||
		reader.InputOffset() > int64(ImportStagingMaximumHeaderBytesV1) {
		return parsedImportItemV1{}, errors.New("CSV header is invalid")
	}
	mapping := mapImportHeaderV1(header, canonicalColumns)
	item := parsedImportItemV1{
		privateOrderKey: member.orderKey,
		bodySHA256:      domainsecurity.SHA256Hex(member.body),
		columns:         mapping.columns,
		mappingDigest:   mapping.digest,
		status:          mapping.status,
	}
	var canonical bytes.Buffer
	writer := csv.NewWriter(&canonical)
	writer.UseCRLF = true
	for {
		if ctx.Err() != nil {
			clearParsedImportItemV1(&item)
			return parsedImportItemV1{}, ctx.Err()
		}
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil || len(record) != len(header) {
			clearParsedImportItemV1(&item)
			return parsedImportItemV1{}, errors.New("CSV record width is invalid")
		}
		if item.rowCount == ImportStagingMaximumRowsV1 {
			clearParsedImportItemV1(&item)
			return parsedImportItemV1{}, errors.New("CSV row limit exceeded")
		}
		canonicalRecord := make([]string, len(canonicalColumns))
		for columnIndex, value := range record {
			if !validImportCellV1(value) {
				clearParsedImportItemV1(&item)
				return parsedImportItemV1{}, errors.New("CSV cell is invalid")
			}
			if item.rowCount == 0 {
				item.columns[columnIndex].sampleValue = append([]byte(nil), []byte(value)...)
			}
			if target := mapping.targets[columnIndex]; target >= 0 {
				canonicalRecord[target] = value
			}
		}
		if err := writer.Write(canonicalRecord); err != nil {
			clearParsedImportItemV1(&item)
			return parsedImportItemV1{}, err
		}
		item.rowCount++
	}
	writer.Flush()
	if writer.Error() != nil || item.rowCount == 0 {
		clearParsedImportItemV1(&item)
		return parsedImportItemV1{}, errors.New("CSV contains no importable rows")
	}
	item.canonicalRows = append([]byte(nil), canonical.Bytes()...)
	return item, nil
}

type importHeaderMappingV1 struct {
	targets []int
	columns []stagedMappingColumnV1
	digest  string
	status  string
}

func mapImportHeaderV1(
	header []string,
	canonical []domainevidence.FundsTransactionCSVColumnV1,
) importHeaderMappingV1 {
	aliases := importHeaderAliasesV1(canonical)
	result := importHeaderMappingV1{
		targets: make([]int, len(header)), columns: make([]stagedMappingColumnV1, len(header)),
		status: importStagingReadyV1,
	}
	for index := range result.targets {
		result.targets[index] = -1
	}
	seenSource := make(map[string][]int, len(header))
	seenTarget := make(map[int][]int, len(header))
	for index, source := range header {
		column := stagedMappingColumnV1{
			sourceColumn: append([]byte(nil), []byte(source)...), inferredType: "unknown",
			parseStatus: "parsed", mappingStatus: "unmapped",
		}
		if !validImportCellV1(source) || source == "" {
			column.parseStatus = "invalid"
			result.status = importStagingMappingInvalidV1
		}
		normalized := strings.ToLower(norm.NFC.String(source))
		seenSource[normalized] = append(seenSource[normalized], index)
		if target, ok := aliases[source]; ok {
			result.targets[index] = target
			column.targetField = canonical[target].Field
			column.inferredType = inferredImportTypeV1(canonical[target].Field)
			column.mappingStatus = "mapped"
			seenTarget[target] = append(seenTarget[target], index)
		} else if target, ok := aliases[strings.ToLower(source)]; ok {
			result.targets[index] = target
			column.targetField = canonical[target].Field
			column.inferredType = inferredImportTypeV1(canonical[target].Field)
			column.mappingStatus = "mapped"
			seenTarget[target] = append(seenTarget[target], index)
		} else {
			result.status = importStagingMappingInvalidV1
		}
		result.columns[index] = column
	}
	for _, indexes := range seenSource {
		if len(indexes) > 1 {
			result.status = importStagingMappingInvalidV1
			for _, index := range indexes {
				result.columns[index].mappingStatus = "conflict"
				result.targets[index] = -1
			}
		}
	}
	for _, indexes := range seenTarget {
		if len(indexes) > 1 {
			result.status = importStagingMappingInvalidV1
			for _, index := range indexes {
				result.columns[index].mappingStatus = "conflict"
				result.targets[index] = -1
			}
		}
	}
	required := []int{4, 5, 7, 14}
	for _, target := range required {
		if len(seenTarget[target]) != 1 {
			result.status = importStagingMappingInvalidV1
		}
	}
	if len(seenTarget[0]) != 1 && len(seenTarget[1]) != 1 {
		result.status = importStagingMappingInvalidV1
	}
	digestValues := []string{importStagingMappingPolicyV1, result.status}
	for index := range header {
		digestValues = append(digestValues, header[index], fmt.Sprintf("%d", result.targets[index]),
			result.columns[index].mappingStatus)
	}
	result.digest = digestFieldsV1(digestValues...)
	return result
}

func importHeaderAliasesV1(canonical []domainevidence.FundsTransactionCSVColumnV1) map[string]int {
	aliases := make(map[string]int, len(canonical)*3)
	for index, column := range canonical {
		aliases[column.Header] = index
		aliases[column.Field] = index
		aliases[strings.TrimPrefix(column.Field, "raw.")] = index
	}
	for alias, index := range map[string]int{
		"card": 0, "card_number": 0, "银行卡号": 0,
		"account": 1, "account_number": 1, "账户号": 1,
		"account_name": 2, "户名": 2,
		"identity_number": 3, "identity_no": 3, "证件号码": 3,
		"transaction_time": 4, "timestamp": 4,
		"transaction_amount": 5, "amount": 5,
		"transaction_balance": 6, "balance": 6,
		"direction": 7, "debit_credit": 7,
		"counterparty_account": 8, "对方账号": 8,
		"counterparty_name": 10, "对方户名": 10,
		"counterparty_identity_number": 11, "对方证件号码": 11,
		"counterparty_bank": 12, "对方开户行": 12,
		"description": 13, "摘要": 13,
		"currency": 14, "币种": 14,
		"merchant": 29, "merchant_name": 29,
		"note": 31, "remark": 31,
	} {
		aliases[alias] = index
	}
	return aliases
}

func decodeImportTextV1(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("import text is empty")
	}
	body := raw
	hasUTF8BOM := bytes.HasPrefix(body, []byte{0xef, 0xbb, 0xbf})
	if hasUTF8BOM {
		body = body[3:]
	}
	if len(body) == 0 || bytes.Contains(body, []byte{0xef, 0xbb, 0xbf}) {
		return nil, errors.New("UTF-8 BOM placement is invalid")
	}
	if utf8.Valid(body) {
		if hasUTF8BOM {
			return append([]byte(nil), body...), nil
		}
		gbDecoded, gbValid := decodeGB18030StrictV1(body)
		if gbValid && !bytes.Equal(gbDecoded, body) {
			clear(gbDecoded)
			return nil, errors.New("import encoding is ambiguous")
		}
		clear(gbDecoded)
		return append([]byte(nil), body...), nil
	}
	if hasUTF8BOM {
		return nil, errors.New("BOM-marked UTF-8 encoding is invalid")
	}
	decoded, valid := decodeGB18030StrictV1(body)
	if !valid {
		return nil, errors.New("import encoding is invalid")
	}
	return decoded, nil
}

func decodeGB18030StrictV1(body []byte) ([]byte, bool) {
	decoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), body)
	if err != nil || !utf8.Valid(decoded) || bytes.Contains(decoded, []byte{0xef, 0xbf, 0xbd}) {
		clear(decoded)
		return nil, false
	}
	roundTrip, _, encodeErr := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), decoded)
	valid := encodeErr == nil && bytes.Equal(roundTrip, body)
	clear(roundTrip)
	if !valid {
		clear(decoded)
		return nil, false
	}
	return decoded, true
}

func validImportCellV1(value string) bool {
	return utf8.ValidString(value) && uint64(len([]byte(value))) <= ImportStagingMaximumCellBytesV1 &&
		strings.IndexFunc(value, func(character rune) bool {
			return unicode.IsControl(character) || character == '\ufeff'
		}) < 0
}

func inferredImportTypeV1(target string) string {
	switch target {
	case "raw.txn_time":
		return "timestamp"
	case "raw.amount", "raw.balance", "raw.counterparty_balance":
		return "decimal"
	case "raw.is_success":
		return "boolean"
	default:
		return "text"
	}
}

func importColumnFieldV1(column *stagedMappingColumnV1, field string) string {
	switch field {
	case "sourceColumn":
		return string(column.sourceColumn)
	case "sampleValue":
		return string(column.sampleValue)
	case "inferredType":
		return column.inferredType
	case "targetField":
		return column.targetField
	case "parseStatus":
		return column.parseStatus
	case "mappingStatus":
		return column.mappingStatus
	default:
		return ""
	}
}

func generationV1(values ...string) string {
	return "tlgen1_" + digestFieldsV1(values...)
}

func digestFieldsV1(values ...string) string {
	var framed bytes.Buffer
	for _, value := range values {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		_, _ = framed.Write(length[:])
		_, _ = framed.WriteString(value)
	}
	return domainsecurity.SHA256Hex(framed.Bytes())
}

func stageResultFromActiveV1(active *activeImportGenerationV1) ImportStageResultV1 {
	result := ImportStageResultV1{
		Status: active.status, TotalRowCount: active.rowCount,
		Items: make([]ImportSourceItemResultV1, len(active.items)),
	}
	for index := range active.items {
		result.Items[index] = importSourceItemResultV1(active, index)
	}
	return result
}

func importSourceItemResultV1(active *activeImportGenerationV1, index int) ImportSourceItemResultV1 {
	return ImportSourceItemResultV1{
		Selector: active.items[index].selector, SourceIndex: uint16(index + 1),
		SourceCount: uint16(len(active.items)),
		SourceLabel: fmt.Sprintf("Source %d of %d", index+1, len(active.items)),
		RowCount:    active.items[index].rowCount, ColumnCount: uint16(len(active.items[index].columns)),
		Status: active.items[index].status,
	}
}

func currentImportItemV1(active *activeImportGenerationV1, selector string) *stagedImportItemV1 {
	if active == nil {
		return nil
	}
	for index := range active.items {
		if active.items[index].selector == selector {
			return &active.items[index]
		}
	}
	return nil
}

func cloneStagedMappingColumnsV1(columns []stagedMappingColumnV1) []stagedMappingColumnV1 {
	cloned := make([]stagedMappingColumnV1, len(columns))
	for index := range columns {
		cloned[index] = columns[index]
		cloned[index].sourceColumn = append([]byte(nil), columns[index].sourceColumn...)
		cloned[index].sampleValue = append([]byte(nil), columns[index].sampleValue...)
	}
	return cloned
}

func clearParsedImportV1(parsed *parsedImportV1) {
	if parsed == nil {
		return
	}
	clear(parsed.canonicalBody)
	parsed.canonicalBody = nil
	for index := range parsed.items {
		clearParsedImportItemV1(&parsed.items[index])
	}
	parsed.items = nil
}

func clearParsedImportItemV1(item *parsedImportItemV1) {
	if item == nil {
		return
	}
	clear(item.canonicalRows)
	item.canonicalRows = nil
	for index := range item.columns {
		clear(item.columns[index].sourceColumn)
		clear(item.columns[index].sampleValue)
	}
	item.columns = nil
}

func clearImportMembersV1(members []importMemberV1) {
	for index := range members {
		clear(members[index].body)
		members[index].body = nil
	}
}

func normalizeImportErrorV1(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrUnavailable) {
		return errors.Join(ErrUnavailable, err)
	}
	return errors.Join(ErrInvalidRequest, err)
}
