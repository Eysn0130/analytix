package localdisplay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
)

type syntheticImportMappingReaderV1 struct {
	authority   domainlocaldisplay.ImportMappingAuthorityV1
	rows        []domainlocaldisplay.ImportMappingRowV1
	hasMore     bool
	useCalls    int
	validations int
	validateErr error
	reuse       bool
	mutate      func(*domainlocaldisplay.ImportMappingAuthorityV1, *[]domainlocaldisplay.ImportMappingRowV1)
}

func (reader *syntheticImportMappingReaderV1) UseCurrentImportMappingPreviewV1(
	ctx context.Context,
	_ domainidentity.PrincipalV1,
	selector string,
	fields []string,
	rowOffset uint32,
	rowLimit uint16,
	use func(context.Context, domainlocaldisplay.ImportMappingAuthorityV1, []domainlocaldisplay.ImportMappingRowV1, bool) error,
) error {
	reader.useCalls++
	if selector != reader.authority.Selector || rowOffset > domainlocaldisplay.MaximumRowOffsetV1 ||
		rowLimit == 0 || !reflect.DeepEqual(fields, []string{"sourceColumn", "sampleValue"}) {
		return errors.New("synthetic import selector mismatch")
	}
	authority := reader.authority
	rows := append([]domainlocaldisplay.ImportMappingRowV1(nil), reader.rows...)
	if reader.mutate != nil {
		reader.mutate(&authority, &rows)
	}
	err := use(ctx, authority, rows, reader.hasMore)
	if err == nil && reader.reuse {
		return use(ctx, authority, rows, reader.hasMore)
	}
	return err
}

func (reader *syntheticImportMappingReaderV1) ValidateCurrentImportMappingPreviewV1(
	_ context.Context,
	_ domainidentity.PrincipalV1,
	authority domainlocaldisplay.ImportMappingAuthorityV1,
) error {
	reader.validations++
	if reader.validateErr != nil {
		return reader.validateErr
	}
	if authority != reader.authority {
		return errors.New("synthetic import generation changed")
	}
	return nil
}

type syntheticCleaningDiffReaderV1 struct {
	authority   domainlocaldisplay.CleaningDiffAuthorityV1
	rows        []domainlocaldisplay.CleaningDiffRowV1
	hasMore     bool
	useCalls    int
	validations int
	validateErr error
	mutate      func(*domainlocaldisplay.CleaningDiffAuthorityV1, *[]domainlocaldisplay.CleaningDiffRowV1)
}

func (reader *syntheticCleaningDiffReaderV1) UseCurrentCleaningDiffPreviewV1(
	ctx context.Context,
	_ domainidentity.PrincipalV1,
	selector string,
	fields []string,
	rowOffset uint32,
	rowLimit uint16,
	use func(context.Context, domainlocaldisplay.CleaningDiffAuthorityV1, []domainlocaldisplay.CleaningDiffRowV1, bool) error,
) error {
	reader.useCalls++
	if selector != reader.authority.Selector || rowOffset > domainlocaldisplay.MaximumRowOffsetV1 ||
		rowLimit == 0 || !reflect.DeepEqual(fields, []string{"account", "amountText"}) {
		return errors.New("synthetic cleaning selector mismatch")
	}
	authority := reader.authority
	rows := append([]domainlocaldisplay.CleaningDiffRowV1(nil), reader.rows...)
	if reader.mutate != nil {
		reader.mutate(&authority, &rows)
	}
	return use(ctx, authority, rows, reader.hasMore)
}

func (reader *syntheticCleaningDiffReaderV1) ValidateCurrentCleaningDiffPreviewV1(
	_ context.Context,
	_ domainidentity.PrincipalV1,
	authority domainlocaldisplay.CleaningDiffAuthorityV1,
) error {
	reader.validations++
	if reader.validateErr != nil {
		return reader.validateErr
	}
	if authority != reader.authority {
		return errors.New("synthetic cleaning generation changed")
	}
	return nil
}

func typedFamilyDigestV1(label string) string {
	digest := sha256.Sum256([]byte(label))
	return hex.EncodeToString(digest[:])
}

func typedFamilyExactV1(t *testing.T, value string) domainlocaldisplay.ExactValueV1 {
	t.Helper()
	exact, err := domainlocaldisplay.NewExactValueV1(value)
	if err != nil {
		t.Fatalf("new synthetic exact value: %v", err)
	}
	return exact
}

func typedFamilyJSONPropertiesV1(value any) []string {
	typeOf := reflect.TypeOf(value)
	properties := make([]string, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		properties = append(properties, typeOf.Field(index).Tag.Get("json"))
	}
	return properties
}

func TestTypedFamilyGoResponsesMatchCanonicalPropertySets(t *testing.T) {
	for _, test := range []struct {
		name     string
		kind     string
		response any
		lineage  any
		row      any
		cell     any
	}{
		{
			name: "import", kind: domainlocaldisplay.KindImportMappingPreviewV1,
			response: ImportMappingPreviewResponseV1{}, lineage: ImportMappingLineageV1{},
			row: ImportMappingPreviewRowV1{}, cell: ImportMappingPreviewCellV1{},
		},
		{
			name: "cleaning", kind: domainlocaldisplay.KindCleaningDiffPreviewV1,
			response: CleaningDiffPreviewResponseV1{}, lineage: CleaningDiffLineageV1{},
			row: CleaningDiffPreviewRowV1{}, cell: CleaningDiffPreviewCellV1{},
		},
		{
			name: "direct", kind: domainlocaldisplay.KindDirectSourcePreviewV1,
			response: DirectSourcePreviewResponseV1{}, row: DirectSourcePreviewRowV1{},
			cell: DirectSourcePreviewCellV1{},
		},
		{
			name: "accepted", kind: domainlocaldisplay.KindAcceptedSlotDisplayV1,
			response: ResponseV1{}, row: SlotV1{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, want := typedFamilyJSONPropertiesV1(test.response), domainlocaldisplay.ContractResponsePropertiesV1(test.kind); !reflect.DeepEqual(got, want) {
				t.Fatalf("Go response properties drifted: got=%v want=%v", got, want)
			}
			if test.lineage != nil {
				if got, want := typedFamilyJSONPropertiesV1(test.lineage), domainlocaldisplay.ContractLineagePropertiesV1(test.kind); !reflect.DeepEqual(got, want) {
					t.Fatalf("Go lineage properties drifted: got=%v want=%v", got, want)
				}
			}
			if got, want := typedFamilyJSONPropertiesV1(test.row), domainlocaldisplay.ContractRowPropertiesV1(test.kind); !reflect.DeepEqual(got, want) {
				t.Fatalf("Go row properties drifted: got=%v want=%v", got, want)
			}
			if test.cell != nil {
				if got, want := typedFamilyJSONPropertiesV1(test.cell), domainlocaldisplay.ContractCellPropertiesV1(test.kind); !reflect.DeepEqual(got, want) {
					t.Fatalf("Go cell properties drifted: got=%v want=%v", got, want)
				}
			}
		})
	}
}

func typedFamilyImportReaderV1(t *testing.T) *syntheticImportMappingReaderV1 {
	t.Helper()
	return &syntheticImportMappingReaderV1{
		authority: domainlocaldisplay.ImportMappingAuthorityV1{
			Selector:             "tlsel1_" + typedFamilyDigestV1("import-selector"),
			ImportGeneration:     "tlgen1_" + typedFamilyDigestV1("import-generation"),
			SourceItemGeneration: "tlgen1_" + typedFamilyDigestV1("source-item-generation"),
			ParserGeneration:     "tlgen1_" + typedFamilyDigestV1("parser-generation"),
			MappingGeneration:    "tlgen1_" + typedFamilyDigestV1("mapping-generation"),
		},
		rows: []domainlocaldisplay.ImportMappingRowV1{{
			RowIndex: 0, ParseStatus: "parsed", MappingStatus: "mapped",
			Cells: []domainlocaldisplay.ImportMappingCellV1{
				{Field: "sourceColumn", Value: typedFamilyExactV1(t, "SOURCE_COLUMN_CANARY")},
				{Field: "sampleValue", Value: typedFamilyExactV1(t, "SOURCE_EXACT_IMPORT")},
			},
		}},
	}
}

func typedFamilyCleaningReaderV1(t *testing.T) *syntheticCleaningDiffReaderV1 {
	t.Helper()
	return &syntheticCleaningDiffReaderV1{
		authority: domainlocaldisplay.CleaningDiffAuthorityV1{
			Selector:         "tlsel1_" + typedFamilyDigestV1("clean-selector"),
			InputSnapshot:    "tlsnap1_" + typedFamilyDigestV1("input-snapshot"),
			RuleGeneration:   "tlgen1_" + typedFamilyDigestV1("rule-generation"),
			RuleDigest:       typedFamilyDigestV1("rule-digest"),
			OutputSnapshot:   "tlsnap1_" + typedFamilyDigestV1("output-snapshot"),
			TransformLineage: "tllin1_" + typedFamilyDigestV1("transform-lineage"),
		},
		rows: []domainlocaldisplay.CleaningDiffRowV1{{
			RowIndex: 0, Status: "changed",
			Cells: []domainlocaldisplay.CleaningDiffCellV1{
				{Field: "account", Before: typedFamilyExactV1(t, "BEFORE-ACCOUNT-1234"), After: typedFamilyExactV1(t, "AFTER-ACCOUNT-5678")},
				{Field: "amountText", Before: typedFamilyExactV1(t, "12.00"), After: typedFamilyExactV1(t, "21.00")},
			},
		}},
	}
}

func TestTypedFamilySyntheticAuthoritiesProjectFullAndMaskedWithoutLineageDrift(t *testing.T) {
	identity := newLocalDisplayIdentityStubV1(t)
	imports := typedFamilyImportReaderV1(t)
	cleaning := typedFamilyCleaningReaderV1(t)
	service := NewServiceWithTypedLocalDataSurface(
		nil, nil,
		ImportMappingPreviewDependenciesV1{Identity: identity, Reader: imports},
		CleaningDiffPreviewDependenciesV1{Identity: identity, Reader: cleaning},
		DirectSourcePreviewDependenciesV1{}, AcceptedSlotDisplayDependenciesV1{},
	)

	var fullImport ImportMappingPreviewResponseV1
	var fullCleaning CleaningDiffPreviewResponseV1
	for _, mode := range []string{DisplayModeFull, DisplayModeMasked} {
		importResponse, err := service.ImportMappingPreview(context.Background(), ImportMappingPreviewInputV1{
			Selector: imports.authority.Selector, Fields: []string{"sourceColumn", "sampleValue"},
			RowLimit: 25, DisplayMode: mode,
		})
		if err != nil || importResponse.Kind != domainlocaldisplay.KindImportMappingPreviewV1 ||
			importResponse.Selector != imports.authority.Selector || len(importResponse.Rows) != 1 {
			t.Fatalf("synthetic import projection failed for %s: response=%#v err=%v", mode, importResponse, err)
		}
		cleaningResponse, err := service.CleaningDiffPreview(context.Background(), CleaningDiffPreviewInputV1{
			Selector: cleaning.authority.Selector, Fields: []string{"account", "amountText"},
			RowLimit: 25, DisplayMode: mode,
		})
		if err != nil || cleaningResponse.Kind != domainlocaldisplay.KindCleaningDiffPreviewV1 ||
			cleaningResponse.Selector != cleaning.authority.Selector || len(cleaningResponse.Rows) != 1 {
			t.Fatalf("synthetic cleaning projection failed for %s: response=%#v err=%v", mode, cleaningResponse, err)
		}
		if mode == DisplayModeFull {
			fullImport, fullCleaning = importResponse, cleaningResponse
			if importResponse.Rows[0].Cells[1].DisplayValue != "SOURCE_EXACT_IMPORT" ||
				cleaningResponse.Rows[0].Cells[0].BeforeDisplayValue != "BEFORE-ACCOUNT-1234" {
				t.Fatal("full mode did not preserve synthetic exact display bytes")
			}
		} else {
			if importResponse.Rows[0].Cells[1].DisplayValue != "****PORT" ||
				cleaningResponse.Rows[0].Cells[0].BeforeDisplayValue != "****1234" ||
				cleaningResponse.Rows[0].Cells[1].BeforeDisplayValue != "12.00" {
				t.Fatal("host masking did not preserve the closed safe-field behavior")
			}
			if importResponse.Lineage != fullImport.Lineage || importResponse.Selector != fullImport.Selector ||
				cleaningResponse.Lineage != fullCleaning.Lineage || cleaningResponse.Selector != fullCleaning.Selector {
				t.Fatal("display mode changed selector or lineage authority")
			}
		}
	}
	if imports.useCalls != 2 || imports.validations != 2 || cleaning.useCalls != 2 || cleaning.validations != 2 {
		t.Fatal("synthetic current authority was not revalidated exactly once per request")
	}
}

func TestTypedFamilyProductionAndAuthorityMismatchFailClosed(t *testing.T) {
	service := NewServiceWithTypedLocalDataSurface(
		nil, nil, ImportMappingPreviewDependenciesV1{}, CleaningDiffPreviewDependenciesV1{},
		DirectSourcePreviewDependenciesV1{}, AcceptedSlotDisplayDependenciesV1{},
	)
	selector := "tlsel1_" + typedFamilyDigestV1("missing-production-authority")
	if response, err := service.ImportMappingPreview(context.Background(), ImportMappingPreviewInputV1{
		Selector: selector, Fields: []string{"sourceColumn"}, RowLimit: 1, DisplayMode: DisplayModeFull,
	}); !errors.Is(err, ErrUnavailable) || response.SchemaVersion != 0 {
		t.Fatal("missing production import authority did not fail closed")
	}
	if response, err := service.CleaningDiffPreview(context.Background(), CleaningDiffPreviewInputV1{
		Selector: selector, Fields: []string{"account"}, RowLimit: 1, DisplayMode: DisplayModeFull,
	}); !errors.Is(err, ErrUnavailable) || response.SchemaVersion != 0 {
		t.Fatal("missing production cleaning authority did not fail closed")
	}

	for name, mutate := range map[string]func(*syntheticImportMappingReaderV1){
		"wrong generation": func(reader *syntheticImportMappingReaderV1) {
			reader.mutate = func(authority *domainlocaldisplay.ImportMappingAuthorityV1, _ *[]domainlocaldisplay.ImportMappingRowV1) {
				authority.MappingGeneration = "tlgen1_" + typedFamilyDigestV1("wrong-generation")
			}
		},
		"reordered cells": func(reader *syntheticImportMappingReaderV1) {
			reader.mutate = func(_ *domainlocaldisplay.ImportMappingAuthorityV1, rows *[]domainlocaldisplay.ImportMappingRowV1) {
				(*rows)[0].Cells[0], (*rows)[0].Cells[1] = (*rows)[0].Cells[1], (*rows)[0].Cells[0]
			}
		},
		"missing cell": func(reader *syntheticImportMappingReaderV1) {
			reader.mutate = func(_ *domainlocaldisplay.ImportMappingAuthorityV1, rows *[]domainlocaldisplay.ImportMappingRowV1) {
				(*rows)[0].Cells = (*rows)[0].Cells[:1]
			}
		},
		"callback reuse": func(reader *syntheticImportMappingReaderV1) {
			reader.reuse = true
		},
		"late authority drift": func(reader *syntheticImportMappingReaderV1) {
			reader.validateErr = errors.New("generation changed after read")
		},
	} {
		t.Run(name, func(t *testing.T) {
			identity := newLocalDisplayIdentityStubV1(t)
			reader := typedFamilyImportReaderV1(t)
			mutate(reader)
			service := NewServiceWithTypedLocalDataSurface(
				nil, nil, ImportMappingPreviewDependenciesV1{Identity: identity, Reader: reader},
				CleaningDiffPreviewDependenciesV1{}, DirectSourcePreviewDependenciesV1{}, AcceptedSlotDisplayDependenciesV1{},
			)
			response, err := service.ImportMappingPreview(context.Background(), ImportMappingPreviewInputV1{
				Selector: reader.authority.Selector, Fields: []string{"sourceColumn", "sampleValue"},
				RowLimit: 25, DisplayMode: DisplayModeFull,
			})
			if !errors.Is(err, ErrUnavailable) || response.SchemaVersion != 0 {
				t.Fatalf("authority mismatch projected a response: response=%#v err=%v", response, err)
			}
		})
	}

	for name, mutate := range map[string]func(*syntheticCleaningDiffReaderV1){
		"wrong input snapshot": func(reader *syntheticCleaningDiffReaderV1) {
			reader.mutate = func(authority *domainlocaldisplay.CleaningDiffAuthorityV1, _ *[]domainlocaldisplay.CleaningDiffRowV1) {
				authority.InputSnapshot = "tlsnap1_" + typedFamilyDigestV1("wrong-input")
			}
		},
		"wrong rule": func(reader *syntheticCleaningDiffReaderV1) {
			reader.mutate = func(authority *domainlocaldisplay.CleaningDiffAuthorityV1, _ *[]domainlocaldisplay.CleaningDiffRowV1) {
				authority.RuleDigest = typedFamilyDigestV1("wrong-rule")
			}
		},
		"wrong output snapshot": func(reader *syntheticCleaningDiffReaderV1) {
			reader.mutate = func(authority *domainlocaldisplay.CleaningDiffAuthorityV1, _ *[]domainlocaldisplay.CleaningDiffRowV1) {
				authority.OutputSnapshot = "tlsnap1_" + typedFamilyDigestV1("wrong-output")
			}
		},
		"wrong transform lineage": func(reader *syntheticCleaningDiffReaderV1) {
			reader.mutate = func(authority *domainlocaldisplay.CleaningDiffAuthorityV1, _ *[]domainlocaldisplay.CleaningDiffRowV1) {
				authority.TransformLineage = "tllin1_" + typedFamilyDigestV1("wrong-lineage")
			}
		},
		"oversized logical cell": func(reader *syntheticCleaningDiffReaderV1) {
			reader.mutate = func(_ *domainlocaldisplay.CleaningDiffAuthorityV1, rows *[]domainlocaldisplay.CleaningDiffRowV1) {
				(*rows)[0].Cells[0].Before = typedFamilyExactV1(t, strings.Repeat("a", 3_000))
				(*rows)[0].Cells[0].After = typedFamilyExactV1(t, strings.Repeat("b", 3_000))
			}
		},
		"late cleaning drift": func(reader *syntheticCleaningDiffReaderV1) {
			reader.validateErr = errors.New("cleaning authority changed after read")
		},
	} {
		t.Run(name, func(t *testing.T) {
			identity := newLocalDisplayIdentityStubV1(t)
			reader := typedFamilyCleaningReaderV1(t)
			mutate(reader)
			service := NewServiceWithTypedLocalDataSurface(
				nil, nil, ImportMappingPreviewDependenciesV1{},
				CleaningDiffPreviewDependenciesV1{Identity: identity, Reader: reader},
				DirectSourcePreviewDependenciesV1{}, AcceptedSlotDisplayDependenciesV1{},
			)
			response, err := service.CleaningDiffPreview(context.Background(), CleaningDiffPreviewInputV1{
				Selector: reader.authority.Selector, Fields: []string{"account", "amountText"},
				RowLimit: 25, DisplayMode: DisplayModeFull,
			})
			if !errors.Is(err, ErrUnavailable) || response.SchemaVersion != 0 {
				t.Fatalf("cleaning authority mismatch projected a response: response=%#v err=%v", response, err)
			}
		})
	}
}
