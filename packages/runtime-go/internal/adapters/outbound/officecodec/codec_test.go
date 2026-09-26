package officecodec_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/documentcodec"
	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	"analytix.local/runtime-go/internal/adapters/outbound/officecodec"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
	"github.com/xuri/excelize/v2"
)

var workbookJSON = json.RawMessage(`{"sheets":[{"id":"source_data","name":"资金统计","cells":[{"address":"A1","type":"text","value":"项目"},{"address":"B1","type":"text","value":"金额"},{"address":"A2","type":"text","value":"甲"},{"address":"B2","type":"number","value":100},{"address":"A3","type":"text","value":"乙"},{"address":"B3","type":"number","value":200},{"address":"A4","type":"text","value":"合计"},{"address":"B4","type":"formula","formula":"SUM(B2:B3)"}],"charts":[{"id":"amount_chart","type":"bar","title":"金额比较","anchor":"D2","series":[{"name":"B1","categories":"A2:A3","values":"B2:B3"}]}]}]}`)
var presentationJSON = json.RawMessage(`{"slides":[{"id":"summary_slide","objects":[{"id":"summary_title","kind":"text","x":1,"y":0.5,"w":10,"h":1,"text":"资金统计：合计 300"},{"id":"amount_chart","kind":"chart","x":1,"y":2,"w":10,"h":4,"chartType":"bar","categories":["甲","乙"],"series":[{"name":"金额","values":[100,200]}]}]}]}`)

func TestWorkbookCodecRemainsAvailableWithoutNode(t *testing.T) {
	codec := officecodec.New(nil)
	if !codec.Supports("xlsx") || codec.Supports("pptx") || codec.Supports("docx") {
		t.Fatal("incorrect codec availability")
	}
	body, err := codec.Encode(context.Background(), codecport.Input{SchemaVersion: 1, Kind: "xlsx", Workbook: workbookJSON})
	if err != nil || filestore.InspectOfficePackage(body, "xlsx") != nil {
		t.Fatal("real workbook invalid", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	value, err := f.CalcCellValue("资金统计", "B4")
	if err != nil || value != "300" {
		t.Fatal("real reopened formula mismatch", value, err)
	}
	if _, err := codec.Encode(context.Background(), codecport.Input{SchemaVersion: 1, Kind: "docx", Markdown: "no node"}); err == nil {
		t.Fatal("missing Node codec accepted")
	}
}

// This opt-in check exercises the actual configured Main bundle, not a test
// implementation. CI without a built desktop entry explicitly reports skipped.
func TestBundledOfficeCodecsFromCommonSource(t *testing.T) {
	executable, entry := os.Getenv("ANALYTIX_CODEC_TEST_EXECUTABLE"), os.Getenv("ANALYTIX_CODEC_TEST_ENTRY")
	if executable == "" || entry == "" {
		t.Skip("requires an explicitly built desktop codec entry")
	}
	codec := officecodec.New(documentcodec.New(executable, entry))
	inputs := []codecport.Input{{SchemaVersion: 1, Kind: "docx", Markdown: "# 资金统计\n\n| 项目 | 金额 |\n| --- | --- |\n| 甲 | 100 |\n| 乙 | 200 |\n| 合计 | 300 |"}, {SchemaVersion: 1, Kind: "xlsx", Workbook: workbookJSON}, {SchemaVersion: 1, Kind: "pptx", Presentation: presentationJSON}}
	for _, input := range inputs {
		t.Run(input.Kind, func(t *testing.T) {
			body, err := codec.Encode(context.Background(), input)
			if err != nil || filestore.InspectOfficePackage(body, input.Kind) != nil {
				t.Fatal("real codec or Core package inspection failed", err)
			}
			z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
			if err != nil {
				t.Fatal(err)
			}
			parts := map[string]string{}
			for _, part := range z.File {
				if strings.HasSuffix(part.Name, ".xml") {
					r, err := part.Open()
					if err != nil {
						t.Fatal(err)
					}
					b, err := io.ReadAll(r)
					r.Close()
					if err != nil {
						t.Fatal(err)
					}
					parts[part.Name] = string(b)
				}
			}
			switch input.Kind {
			case "docx":
				if !strings.Contains(parts["word/document.xml"], "300") || !strings.Contains(parts["word/document.xml"], "<w:tbl>") {
					t.Fatal("DOCX source table missing")
				}
			case "xlsx":
				if !strings.Contains(parts["xl/worksheets/sheet1.xml"], "SUM(B2:B3)") || !officeXMLHasText(parts["xl/charts/chart1.xml"], "'资金统计'!$B$2:$B$3") {
					t.Fatal("XLSX formula/chart structure missing")
				}
			case "pptx":
				if !strings.Contains(parts["ppt/slides/slide1.xml"], `name="summary_slide"`) || !strings.Contains(parts["ppt/slides/slide1.xml"], "合计 300") || !strings.Contains(parts["ppt/charts/chart1.xml"], "200") {
					t.Fatal("PPTX source chart or stable identity missing")
				}
			}
			if output := os.Getenv("ANALYTIX_CODEC_TEST_OUTPUT"); output != "" {
				if !filepath.IsAbs(output) {
					t.Fatal("absolute evidence output required")
				}
				if err := os.MkdirAll(output, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(output, "资金统计."+input.Kind), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func officeXMLHasText(source, want string) bool {
	decoder := xml.NewDecoder(strings.NewReader(source))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if value, ok := token.(xml.CharData); ok && string(value) == want {
			return true
		}
	}
}
