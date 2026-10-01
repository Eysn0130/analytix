package filestore

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	codecadapter "analytix.local/runtime-go/internal/adapters/outbound/documentcodec"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

// Explicit bundled-codec integration seam. Ordinary Go unit tests do not assume
// a JavaScript toolchain or an Electron output directory on the test host.
func TestGeneratedDocumentBundledCodecPackageContract(t *testing.T) {
	executable, entry := os.Getenv("ANALYTIX_TEST_DOCUMENT_CODEC_EXECUTABLE"), os.Getenv("ANALYTIX_TEST_DOCUMENT_CODEC_ENTRY")
	if executable == "" || entry == "" {
		t.Skip("bundled document codec is not configured")
	}
	codec := codecadapter.New(executable, entry)
	if codec == nil {
		t.Fatal("codec paths must be absolute")
	}
	data, err := codec.Encode(context.Background(), codecport.Input{SchemaVersion: 1, Kind: "docx", Title: "合成验收报告", Markdown: "# 合成验收报告\n\n来源：合成数据。\n\n- 第一项\n- 第二项\n\n| 项目 | 数值 |\n| --- | --- |\n| 合计 | 42 |\n\n## 结论\n\n**中文格式保留**。"})
	if err != nil {
		t.Fatal("bundled codec failed", err)
	}
	if err := InspectOfficePackage(data, "docx"); err != nil {
		t.Fatal("codec output was rejected by the production Office package inspector", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var document string
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		document = string(body)
	}
	for _, expected := range []string{"合成验收报告", "合计", "42", "中文格式保留", "w:tbl", "w:numPr", "w:b"} {
		if !strings.Contains(document, expected) {
			t.Fatalf("generated DOCX lacks expected structure %q", expected)
		}
	}
}
