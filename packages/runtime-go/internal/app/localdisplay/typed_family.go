package localdisplay

import (
	"context"
	"encoding/json"
	"errors"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

// ImportMappingPreviewReaderV1 is the future host-staged import authority
// seam. A renderer selector is lookup-only: the reader must resolve and
// revalidate the current staged generation, source item, parser generation,
// and mapping generation before exact bytes may leave the callback.
type ImportMappingPreviewReaderV1 interface {
	UseCurrentImportMappingPreviewV1(
		context.Context,
		domainidentity.PrincipalV1,
		string,
		[]string,
		uint32,
		uint16,
		func(context.Context, domainlocaldisplay.ImportMappingAuthorityV1, []domainlocaldisplay.ImportMappingRowV1, bool) error,
	) error
	ValidateCurrentImportMappingPreviewV1(
		context.Context,
		domainidentity.PrincipalV1,
		domainlocaldisplay.ImportMappingAuthorityV1,
	) error
}

// CleaningDiffPreviewReaderV1 is the future deterministic cleaning authority
// seam. It owns input/rule/output/transform currentness; no current database,
// log, history, or renderer-supplied approximation can satisfy it.
type CleaningDiffPreviewReaderV1 interface {
	UseCurrentCleaningDiffPreviewV1(
		context.Context,
		domainidentity.PrincipalV1,
		string,
		[]string,
		uint32,
		uint16,
		func(context.Context, domainlocaldisplay.CleaningDiffAuthorityV1, []domainlocaldisplay.CleaningDiffRowV1, bool) error,
	) error
	ValidateCurrentCleaningDiffPreviewV1(
		context.Context,
		domainidentity.PrincipalV1,
		domainlocaldisplay.CleaningDiffAuthorityV1,
	) error
}

type ImportMappingPreviewDependenciesV1 struct {
	Identity identityport.Authority
	Reader   ImportMappingPreviewReaderV1
}

type CleaningDiffPreviewDependenciesV1 struct {
	Identity identityport.Authority
	Reader   CleaningDiffPreviewReaderV1
}

type ImportMappingPreviewInputV1 struct {
	Selector    string
	Fields      []string
	RowOffset   uint32
	RowLimit    uint16
	DisplayMode string
}

type CleaningDiffPreviewInputV1 struct {
	Selector    string
	Fields      []string
	RowOffset   uint32
	RowLimit    uint16
	DisplayMode string
}

type ImportMappingLineageV1 struct {
	ImportGeneration     string `json:"importGeneration"`
	SourceItemGeneration string `json:"sourceItemGeneration"`
	ParserGeneration     string `json:"parserGeneration"`
	MappingGeneration    string `json:"mappingGeneration"`
}

type ImportMappingPreviewCellV1 struct {
	Field        string `json:"field"`
	DisplayValue string `json:"displayValue"`
}

type ImportMappingPreviewRowV1 struct {
	RowIndex      uint32                       `json:"rowIndex"`
	ParseStatus   string                       `json:"parseStatus"`
	MappingStatus string                       `json:"mappingStatus"`
	Cells         []ImportMappingPreviewCellV1 `json:"cells"`
}

type ImportMappingPreviewResponseV1 struct {
	SchemaVersion int                         `json:"schemaVersion"`
	Kind          string                      `json:"kind"`
	Selector      string                      `json:"selector"`
	Lineage       ImportMappingLineageV1      `json:"lineage"`
	DisplayMode   string                      `json:"displayMode"`
	Fields        []string                    `json:"fields"`
	RowOffset     uint32                      `json:"rowOffset"`
	RowLimit      uint16                      `json:"rowLimit"`
	HasMore       bool                        `json:"hasMore"`
	Rows          []ImportMappingPreviewRowV1 `json:"rows"`
}

type CleaningDiffLineageV1 struct {
	InputSnapshot    string `json:"inputSnapshot"`
	RuleGeneration   string `json:"ruleGeneration"`
	RuleDigest       string `json:"ruleDigest"`
	OutputSnapshot   string `json:"outputSnapshot"`
	TransformLineage string `json:"transformLineage"`
}

type CleaningDiffPreviewCellV1 struct {
	Field              string `json:"field"`
	BeforeDisplayValue string `json:"beforeDisplayValue"`
	AfterDisplayValue  string `json:"afterDisplayValue"`
}

type CleaningDiffPreviewRowV1 struct {
	RowIndex uint32                      `json:"rowIndex"`
	Status   string                      `json:"status"`
	Cells    []CleaningDiffPreviewCellV1 `json:"cells"`
}

type CleaningDiffPreviewResponseV1 struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Kind          string                     `json:"kind"`
	Selector      string                     `json:"selector"`
	Lineage       CleaningDiffLineageV1      `json:"lineage"`
	DisplayMode   string                     `json:"displayMode"`
	Fields        []string                   `json:"fields"`
	RowOffset     uint32                     `json:"rowOffset"`
	RowLimit      uint16                     `json:"rowLimit"`
	HasMore       bool                       `json:"hasMore"`
	Rows          []CleaningDiffPreviewRowV1 `json:"rows"`
}

func (service *Service) ImportMappingPreview(
	ctx context.Context,
	input ImportMappingPreviewInputV1,
) (ImportMappingPreviewResponseV1, error) {
	if service == nil || ctx == nil || validateTypedFamilyInputV1(
		input.Selector, input.Fields, input.RowOffset, input.RowLimit,
		input.DisplayMode, domainlocaldisplay.ImportMappingPreviewFieldsV1(),
	) != nil {
		return ImportMappingPreviewResponseV1{}, ErrInvalidRequest
	}
	if dependencyIsNilV1(service.importMapping.Identity) || dependencyIsNilV1(service.importMapping.Reader) {
		return ImportMappingPreviewResponseV1{}, ErrUnavailable
	}
	principal, err := resolveTypedFamilyPrincipalV1(ctx, service.importMapping.Identity)
	if err != nil {
		return ImportMappingPreviewResponseV1{}, ErrUnavailable
	}

	var response ImportMappingPreviewResponseV1
	var authority domainlocaldisplay.ImportMappingAuthorityV1
	callbackCalls := 0
	err = service.importMapping.Reader.UseCurrentImportMappingPreviewV1(
		ctx, principal, input.Selector, append([]string(nil), input.Fields...), input.RowOffset, input.RowLimit,
		func(sourceContext context.Context, current domainlocaldisplay.ImportMappingAuthorityV1, rows []domainlocaldisplay.ImportMappingRowV1, hasMore bool) error {
			callbackCalls++
			if sourceContext == nil || callbackCalls != 1 || current.Selector != input.Selector ||
				domainlocaldisplay.ValidateImportMappingAuthorityV1(current) != nil ||
				validateImportRowsV1(input, rows) != nil {
				return errors.New("typed-local import authority result is invalid")
			}
			candidate, projectionErr := projectImportRowsV1(input, current, rows, hasMore)
			if projectionErr != nil || validateResponseBytesV1(candidate) != nil {
				return errors.New("typed-local import projection is invalid")
			}
			authority = current
			response = candidate
			return nil
		},
	)
	if err != nil || callbackCalls != 1 ||
		service.importMapping.Reader.ValidateCurrentImportMappingPreviewV1(ctx, principal, authority) != nil ||
		service.importMapping.Identity.ValidateCurrent(ctx, principal) != nil {
		return ImportMappingPreviewResponseV1{}, ErrUnavailable
	}
	return response, nil
}

func (service *Service) CleaningDiffPreview(
	ctx context.Context,
	input CleaningDiffPreviewInputV1,
) (CleaningDiffPreviewResponseV1, error) {
	if service == nil || ctx == nil || validateTypedFamilyInputV1(
		input.Selector, input.Fields, input.RowOffset, input.RowLimit,
		input.DisplayMode, domainlocaldisplay.CleaningDiffPreviewFieldsV1(),
	) != nil {
		return CleaningDiffPreviewResponseV1{}, ErrInvalidRequest
	}
	if dependencyIsNilV1(service.cleaningDiff.Identity) || dependencyIsNilV1(service.cleaningDiff.Reader) {
		return CleaningDiffPreviewResponseV1{}, ErrUnavailable
	}
	principal, err := resolveTypedFamilyPrincipalV1(ctx, service.cleaningDiff.Identity)
	if err != nil {
		return CleaningDiffPreviewResponseV1{}, ErrUnavailable
	}

	var response CleaningDiffPreviewResponseV1
	var authority domainlocaldisplay.CleaningDiffAuthorityV1
	callbackCalls := 0
	err = service.cleaningDiff.Reader.UseCurrentCleaningDiffPreviewV1(
		ctx, principal, input.Selector, append([]string(nil), input.Fields...), input.RowOffset, input.RowLimit,
		func(sourceContext context.Context, current domainlocaldisplay.CleaningDiffAuthorityV1, rows []domainlocaldisplay.CleaningDiffRowV1, hasMore bool) error {
			callbackCalls++
			if sourceContext == nil || callbackCalls != 1 || current.Selector != input.Selector ||
				domainlocaldisplay.ValidateCleaningDiffAuthorityV1(current) != nil ||
				validateCleaningRowsV1(input, rows) != nil {
				return errors.New("typed-local cleaning authority result is invalid")
			}
			candidate, projectionErr := projectCleaningRowsV1(input, current, rows, hasMore)
			if projectionErr != nil || validateResponseBytesV1(candidate) != nil {
				return errors.New("typed-local cleaning projection is invalid")
			}
			authority = current
			response = candidate
			return nil
		},
	)
	if err != nil || callbackCalls != 1 ||
		service.cleaningDiff.Reader.ValidateCurrentCleaningDiffPreviewV1(ctx, principal, authority) != nil ||
		service.cleaningDiff.Identity.ValidateCurrent(ctx, principal) != nil {
		return CleaningDiffPreviewResponseV1{}, ErrUnavailable
	}
	return response, nil
}

func resolveTypedFamilyPrincipalV1(ctx context.Context, authority identityport.Authority) (domainidentity.PrincipalV1, error) {
	principal, err := authority.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil || authority.ValidateCurrent(ctx, principal) != nil {
		return domainidentity.PrincipalV1{}, ErrUnavailable
	}
	return principal, nil
}

func validateTypedFamilyInputV1(
	selector string,
	fields []string,
	rowOffset uint32,
	rowLimit uint16,
	displayMode string,
	allowedFields []string,
) error {
	if !domainlocaldisplay.ValidSelectorV1(selector) || !domainlocaldisplay.ValidDisplayModeV1(displayMode) ||
		rowOffset > domainlocaldisplay.MaximumRowOffsetV1 || rowLimit == 0 ||
		rowLimit > domainlocaldisplay.MaximumRowLimitV1 ||
		domainlocaldisplay.ValidateFieldsV1(fields, allowedFields) != nil {
		return ErrInvalidRequest
	}
	return nil
}

func validateImportRowsV1(input ImportMappingPreviewInputV1, rows []domainlocaldisplay.ImportMappingRowV1) error {
	if len(rows) > int(input.RowLimit) {
		return ErrUnavailable
	}
	for rowPosition, row := range rows {
		if row.RowIndex > domainlocaldisplay.MaximumRowOffsetV1 ||
			row.RowIndex != input.RowOffset+uint32(rowPosition) ||
			!domainlocaldisplay.ValidImportParseStatusV1(row.ParseStatus) ||
			!domainlocaldisplay.ValidImportMappingStatusV1(row.MappingStatus) ||
			len(row.Cells) != len(input.Fields) {
			return ErrUnavailable
		}
		for cellPosition, cell := range row.Cells {
			statusMismatch := false
			if cell.Field != input.Fields[cellPosition] || cell.Value.UseExactV1(func(value string) error {
				statusMismatch = (cell.Field == "parseStatus" && value != row.ParseStatus) ||
					(cell.Field == "mappingStatus" && value != row.MappingStatus)
				return nil
			}) != nil || statusMismatch {
				return ErrUnavailable
			}
		}
	}
	return nil
}

func validateCleaningRowsV1(input CleaningDiffPreviewInputV1, rows []domainlocaldisplay.CleaningDiffRowV1) error {
	if len(rows) > int(input.RowLimit) {
		return ErrUnavailable
	}
	for rowPosition, row := range rows {
		if row.RowIndex > domainlocaldisplay.MaximumRowOffsetV1 ||
			row.RowIndex != input.RowOffset+uint32(rowPosition) ||
			!domainlocaldisplay.ValidCleaningStatusV1(row.Status) || len(row.Cells) != len(input.Fields) {
			return ErrUnavailable
		}
		for cellPosition, cell := range row.Cells {
			cellBytes := 0
			if cell.Field != input.Fields[cellPosition] ||
				cell.Before.UseExactV1(func(value string) error { cellBytes += len([]byte(value)); return nil }) != nil ||
				cell.After.UseExactV1(func(value string) error { cellBytes += len([]byte(value)); return nil }) != nil ||
				cellBytes > domainlocaldisplay.MaximumCellBytesV1 {
				return ErrUnavailable
			}
		}
	}
	return nil
}

func projectImportRowsV1(
	input ImportMappingPreviewInputV1,
	authority domainlocaldisplay.ImportMappingAuthorityV1,
	rows []domainlocaldisplay.ImportMappingRowV1,
	hasMore bool,
) (ImportMappingPreviewResponseV1, error) {
	response := ImportMappingPreviewResponseV1{
		SchemaVersion: ResponseSchemaVersionV1, Kind: domainlocaldisplay.KindImportMappingPreviewV1,
		Selector: input.Selector,
		Lineage: ImportMappingLineageV1{
			ImportGeneration: authority.ImportGeneration, SourceItemGeneration: authority.SourceItemGeneration,
			ParserGeneration: authority.ParserGeneration, MappingGeneration: authority.MappingGeneration,
		},
		DisplayMode: input.DisplayMode, Fields: append([]string(nil), input.Fields...),
		RowOffset: input.RowOffset, RowLimit: input.RowLimit, HasMore: hasMore,
		Rows: make([]ImportMappingPreviewRowV1, len(rows)),
	}
	for rowPosition, row := range rows {
		response.Rows[rowPosition] = ImportMappingPreviewRowV1{
			RowIndex: row.RowIndex, ParseStatus: row.ParseStatus, MappingStatus: row.MappingStatus,
			Cells: make([]ImportMappingPreviewCellV1, len(row.Cells)),
		}
		for cellPosition, cell := range row.Cells {
			var displayValue string
			if cell.Value.UseExactV1(func(exact string) error {
				displayValue = importDisplayValueV1(input.DisplayMode, cell.Field, exact)
				return nil
			}) != nil {
				return ImportMappingPreviewResponseV1{}, ErrUnavailable
			}
			response.Rows[rowPosition].Cells[cellPosition] = ImportMappingPreviewCellV1{
				Field: cell.Field, DisplayValue: displayValue,
			}
		}
	}
	return response, nil
}

func importDisplayValueV1(mode, field, exact string) string {
	if mode == DisplayModeFull {
		return exact
	}
	switch field {
	case "inferredType", "targetField", "parseStatus", "mappingStatus":
		return exact
	default:
		return localDisplayValue(exact, DisplayModeMasked)
	}
}

func projectCleaningRowsV1(
	input CleaningDiffPreviewInputV1,
	authority domainlocaldisplay.CleaningDiffAuthorityV1,
	rows []domainlocaldisplay.CleaningDiffRowV1,
	hasMore bool,
) (CleaningDiffPreviewResponseV1, error) {
	response := CleaningDiffPreviewResponseV1{
		SchemaVersion: ResponseSchemaVersionV1, Kind: domainlocaldisplay.KindCleaningDiffPreviewV1,
		Selector: input.Selector,
		Lineage: CleaningDiffLineageV1{
			InputSnapshot: authority.InputSnapshot, RuleGeneration: authority.RuleGeneration,
			RuleDigest: authority.RuleDigest, OutputSnapshot: authority.OutputSnapshot,
			TransformLineage: authority.TransformLineage,
		},
		DisplayMode: input.DisplayMode, Fields: append([]string(nil), input.Fields...),
		RowOffset: input.RowOffset, RowLimit: input.RowLimit, HasMore: hasMore,
		Rows: make([]CleaningDiffPreviewRowV1, len(rows)),
	}
	for rowPosition, row := range rows {
		response.Rows[rowPosition] = CleaningDiffPreviewRowV1{
			RowIndex: row.RowIndex, Status: row.Status,
			Cells: make([]CleaningDiffPreviewCellV1, len(row.Cells)),
		}
		for cellPosition, cell := range row.Cells {
			var before, after string
			if cell.Before.UseExactV1(func(exact string) error {
				before = cleaningDisplayValueV1(input.DisplayMode, cell.Field, exact)
				return nil
			}) != nil || cell.After.UseExactV1(func(exact string) error {
				after = cleaningDisplayValueV1(input.DisplayMode, cell.Field, exact)
				return nil
			}) != nil {
				return CleaningDiffPreviewResponseV1{}, ErrUnavailable
			}
			response.Rows[rowPosition].Cells[cellPosition] = CleaningDiffPreviewCellV1{
				Field: cell.Field, BeforeDisplayValue: before, AfterDisplayValue: after,
			}
		}
	}
	return response, nil
}

func cleaningDisplayValueV1(mode, field, exact string) string {
	if mode == DisplayModeFull || directSourcePreviewSafeDisplayFieldV1(field) {
		return exact
	}
	return localDisplayValue(exact, DisplayModeMasked)
}

func validateResponseBytesV1(response any) error {
	body, err := json.Marshal(response)
	if err != nil || len(body) > domainlocaldisplay.MaximumResponseBytesV1 {
		return ErrUnavailable
	}
	return nil
}
