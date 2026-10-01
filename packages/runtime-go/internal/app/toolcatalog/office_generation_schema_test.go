package toolcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"analytix.local/runtime-go/internal/domain/officegeneration"
)

func TestGenerationPivotProviderContract(t *testing.T) {
	workbook := `{"sheets":[{"id":"data","name":"Data","cells":[{"address":"A1","type":"text","value":"Region"},{"address":"B1","type":"text","value":"Sales"},{"address":"A2","type":"text","value":"East"},{"address":"B2","type":"number","value":12}]},{"id":"summary","name":"Summary","cells":[]}],"pivots":[{"id":"SalesPivot","sourceSheetId":"data","sourceRange":"A1:B2","targetSheetId":"summary","targetRange":"A1:D6","rows":["Region"],"values":[{"field":"Sales","aggregate":"sum"}]}]}`
	if _, err := officegeneration.ParseWorkbook([]byte(workbook)); err != nil {
		t.Fatal(err)
	}
	schemas := []domainmodel.ToolSchema{{Name: "generate_office_document", Parameters: GenerationToolParameters([]string{"xlsx"})}}
	if err := ValidateProviderVisibleToolSchemas(schemas); err != nil {
		t.Fatal(err)
	}
	arguments := `{"path":"summary.xlsx","kind":"xlsx","workbook":` + workbook + `}`
	if out, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: schemas[0].Name, Arguments: json.RawMessage(arguments)}, schemas); blocked {
		t.Fatalf("supported pivot blocked: %v", out)
	}
	for _, replacement := range []string{`"aggregate":"execute"`, `"aggregate":"sum","script":"arbitrary"`} {
		bad := strings.Replace(arguments, `"aggregate":"sum"`, replacement, 1)
		if _, blocked := ValidateToolCallArguments(domainmodel.ToolCall{Name: schemas[0].Name, Arguments: json.RawMessage(bad)}, schemas); !blocked {
			t.Fatal("unsupported pivot admitted")
		}
	}
}
