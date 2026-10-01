package filestore

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type officeTestPart struct {
	name  string
	body  []byte
	mode  os.FileMode
	extra []byte
}

func officeTestParts(kind string) []officeTestPart {
	format, _ := officeKindFor(kind)
	main := map[string]string{"docx": "word/document.xml", "xlsx": "xl/workbook.xml", "pptx": "ppt/presentation.xml"}[kind]
	return []officeTestPart{
		{name: "[Content_Types].xml", body: []byte(`<Types xmlns="` + officeContentTypesNS + `"><Default Extension="rels" ContentType="` + officeRelationshipsType + `"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="bin" ContentType="application/octet-stream"/><Override PartName="/` + main + `" ContentType="` + format.mainType + `"/></Types>`)},
		{name: "_rels/.rels", body: []byte(`<Relationships xmlns="` + officeRelationshipsNS + `"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="` + main + `"/></Relationships>`)},
		{name: main, body: []byte(`<?xml version="1.0" encoding="UTF-8"?><` + format.root + ` xmlns="` + format.namespace + `"/>`)},
	}
}

func officeTestZIP(t *testing.T, parts []officeTestPart) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, part := range parts {
		header := &zip.FileHeader{Name: part.name, Method: zip.Deflate, Extra: part.extra}
		if part.mode != 0 {
			header.SetMode(part.mode)
		}
		stream, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = stream.Write(part.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func officeTestReplace(parts []officeTestPart, index int, old, next string) {
	parts[index].body = []byte(strings.ReplaceAll(string(parts[index].body), old, next))
}
func officeTestOverride(parts []officeTestPart, name, contentType string) {
	officeTestReplace(parts, 0, "</Types>", `<Override PartName="/`+name+`" ContentType="`+contentType+`"/></Types>`)
}
func officeTestRels(body string) []byte {
	return []byte(`<Relationships xmlns="` + officeRelationshipsNS + `">` + body + `</Relationships>`)
}
func officeTestRelation(id, kind, target string) string {
	return `<Relationship Id="` + id + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/` + kind + `" Target="` + target + `"/>`
}

func TestOfficePackageAcceptsThreeKindsAndBenignParts(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			parts := officeTestParts(kind)
			if err := InspectOfficePackage(officeTestZIP(t, parts), kind); err != nil {
				t.Fatal(err)
			}
			format, _ := officeKindFor(kind)
			officeTestReplace(parts, 2, format.namespace, format.strictNamespace)
			if err := InspectOfficePackage(officeTestZIP(t, parts), kind); err != nil {
				t.Fatalf("strict namespace: %v", err)
			}
		})
	}
	parts := officeTestParts("pptx")
	additional := []officeTestPart{
		{name: "ppt/media/image.png", body: []byte("\x89PNG\r\n\x1a\nfixture")},
		{name: "ppt/fonts/font.fntdata", body: []byte("\x00\x01\x00\x00font")},
		{name: "ppt/charts/chart.xml", body: []byte(`<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart"/>`)},
		{name: "ppt/embeddings/chart.xlsx", body: officeTestZIP(t, officeTestParts("xlsx"))},
		{name: "ppt/_rels/presentation.xml.rels", body: officeTestRels(officeTestRelation("image", "image", "media/image.png") + officeTestRelation("font", "font", "fonts/font.fntdata") + officeTestRelation("chart", "chart", "charts/chart.xml"))},
		{name: "ppt/charts/_rels/chart.xml.rels", body: officeTestRels(officeTestRelation("data", "package", "../embeddings/chart.xlsx"))},
	}
	for _, part := range additional {
		parts = append(parts, part)
	}
	officeTestOverride(parts, "ppt/media/image.png", "image/png")
	officeTestOverride(parts, "ppt/fonts/font.fntdata", "application/x-fontdata")
	officeTestOverride(parts, "ppt/embeddings/chart.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := InspectOfficePackage(officeTestZIP(t, parts), "pptx"); err != nil {
		t.Fatalf("image/font/chart/embedded workbook: %v", err)
	}
}

func TestOfficePackageRejectsPartNamesAndActiveContent(t *testing.T) {
	cases := []struct {
		name string
		part officeTestPart
	}{
		{"traversal", officeTestPart{name: "../evil.bin"}},
		{"absolute", officeTestPart{name: "/evil.bin"}},
		{"drive", officeTestPart{name: "C:/evil.bin"}},
		{"backslash", officeTestPart{name: `word\evil.bin`}},
		{"nul", officeTestPart{name: "word/evil\x00.bin"}},
		{"dot_segment", officeTestPart{name: "word/./evil.bin"}},
		{"empty_segment", officeTestPart{name: "word//evil.bin"}},
		{"escaped_segment", officeTestPart{name: "word/%2e%2e/evil.bin"}},
		{"duplicate", officeTestPart{name: "word/document.xml"}},
		{"case_collision", officeTestPart{name: "WORD/document.xml"}},
		{"symlink", officeTestPart{name: "word/link.bin", body: []byte("outside"), mode: os.ModeSymlink | 0777}},
		{"vba", officeTestPart{name: "word/vbaProject.bin"}},
		{"activex", officeTestPart{name: "word/activeX/control.bin"}},
		{"ole_name", officeTestPart{name: "word/embeddings/oleObject1.bin"}},
		{"script", officeTestPart{name: "word/embeddings/run.js"}},
		{"macro_extension", officeTestPart{name: "word/embeddings/sheet.xlsm"}},
		{"pe_magic", officeTestPart{name: "word/embeddings/data.bin", body: []byte("MZtest")}},
		{"elf_magic", officeTestPart{name: "word/embeddings/data.bin", body: []byte("\x7fELFtest")}},
		{"ole_magic", officeTestPart{name: "word/embeddings/data.bin", body: []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}}},
		{"file_ancestor", officeTestPart{name: "word/document.xml/child.bin"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			parts := append(officeTestParts("docx"), test.part)
			if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != ErrOfficePackage {
				t.Fatalf("expected closed rejection, got %v", err)
			}
		})
	}
	for _, contentType := range []string{"application/vnd.ms-word.document.macroEnabled.main+xml", "application/vnd.ms-office.vbaProject", "application/vnd.ms-office.activeX", "application/vnd.openxmlformats-officedocument.oleObject", "application/x-msdownload"} {
		t.Run(contentType, func(t *testing.T) {
			parts := append(officeTestParts("docx"), officeTestPart{name: "word/hidden.bin", body: []byte("inert fixture")})
			officeTestOverride(parts, "word/hidden.bin", contentType)
			if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != ErrOfficePackage {
				t.Fatal(err)
			}
		})
	}
}

func TestOfficePackageRejectsMalformedXMLAndMainContract(t *testing.T) {
	cases := []struct {
		name   string
		modify func([]officeTestPart) []officeTestPart
	}{
		{"missing_main", func(p []officeTestPart) []officeTestPart { return p[:2] }},
		{"wrong_main_type", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 0, "wordprocessingml.document.main+xml", "spreadsheetml.sheet.main+xml")
			return p
		}},
		{"wrong_main_root", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 2, "<document ", "<workbook ")
			return p
		}},
		{"wrong_namespace", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 2, "wordprocessingml/2006/main", "spreadsheetml/2006/main")
			return p
		}},
		{"duplicate_default", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 0, "</Types>", `<Default Extension="XML" ContentType="application/xml"/></Types>`)
			return p
		}},
		{"duplicate_override", func(p []officeTestPart) []officeTestPart {
			officeTestOverride(p, "word/document.xml", "application/xml")
			return p
		}},
		{"duplicate_attribute", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 1, `Id="rId1"`, `Id="rId1" Id="rId2"`)
			return p
		}},
		{"unknown_control_attribute", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 1, `Id="rId1"`, `Id="rId1" command="shell"`)
			return p
		}},
		{"control_text", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 0, "</Types>", "invalid</Types>")
			return p
		}},
		{"wrong_control_namespace", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 0, officeContentTypesNS, "urn:wrong")
			return p
		}},
		{"DTD", func(p []officeTestPart) []officeTestPart {
			p[2].body = []byte(`<!DOCTYPE document [<!ENTITY x "value">]><document xmlns="http://schemas.openxmlformats.org/wordprocessingml/2006/main">&x;</document>`)
			return p
		}},
		{"external_entity", func(p []officeTestPart) []officeTestPart {
			p[2].body = []byte(`<!DOCTYPE document SYSTEM "file:///private/fixture"><document/>`)
			return p
		}},
		{"unused_part_DTD", func(p []officeTestPart) []officeTestPart {
			return append(p, officeTestPart{name: "word/unused.xml", body: []byte(`<!DOCTYPE x><x/>`)})
		}},
		{"unknown_entity", func(p []officeTestPart) []officeTestPart {
			p[2].body = []byte(`<document>&unresolved;</document>`)
			return p
		}},
		{"processing_instruction", func(p []officeTestPart) []officeTestPart {
			p[2].body = append([]byte(`<?xml-stylesheet href="file:///private/fixture"?>`), p[2].body...)
			return p
		}},
		{"two_roots", func(p []officeTestPart) []officeTestPart {
			p[2].body = append(p[2].body, []byte(`<second/>`)...)
			return p
		}},
		{"deep_xml", func(p []officeTestPart) []officeTestPart {
			p[2].body = []byte(strings.Repeat("<x>", 129) + strings.Repeat("</x>", 129))
			return p
		}},
		{"svg_script", func(p []officeTestPart) []officeTestPart {
			return append(p, officeTestPart{name: "word/unused.xml", body: []byte(`<svg><script>fixture</script></svg>`)})
		}},
		{"dde", func(p []officeTestPart) []officeTestPart {
			return append(p, officeTestPart{name: "word/unused.xml", body: []byte(`<x><ddeLink/></x>`)})
		}},
		{"macro_action", func(p []officeTestPart) []officeTestPart {
			return append(p, officeTestPart{name: "word/unused.xml", body: []byte(`<x action="ppaction://macro?name=fixture"/>`)})
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := InspectOfficePackage(officeTestZIP(t, test.modify(officeTestParts("docx"))), "docx"); err != ErrOfficePackage {
				t.Fatalf("expected closed rejection, got %v", err)
			}
		})
	}
	for _, kind := range []string{"", "DOCX", "doc", "odt"} {
		if err := InspectOfficePackage(officeTestZIP(t, officeTestParts("docx")), kind); err != ErrOfficePackage {
			t.Fatalf("kind %q: %v", kind, err)
		}
	}
}

func TestOfficePackageRelationships(t *testing.T) {
	for _, target := range []string{"word/document.xml", "/word/document.xml", "word/./document.xml", "word/sub/../document.xml"} {
		t.Run("valid_"+target, func(t *testing.T) {
			parts := officeTestParts("docx")
			officeTestReplace(parts, 1, `Target="word/document.xml"`, `Target="`+target+`"`)
			if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, target := range []string{"../word/document.xml", "%2e%2e/word/document.xml", "/../word/document.xml", "word/missing.xml", "https://example.invalid/document.xml", "//example.invalid/document.xml", `word\document.xml`, "word/document.xml?query=1", "[Content_Types].xml"} {
		t.Run("invalid_"+target, func(t *testing.T) {
			parts := officeTestParts("docx")
			officeTestReplace(parts, 1, `Target="word/document.xml"`, `Target="`+target+`"`)
			if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != ErrOfficePackage {
				t.Fatal(err)
			}
		})
	}
	for _, mode := range []string{"External", "external", "Unknown"} {
		t.Run(mode, func(t *testing.T) {
			parts := officeTestParts("docx")
			officeTestReplace(parts, 1, `Id="rId1"`, `Id="rId1" TargetMode="`+mode+`"`)
			if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != ErrOfficePackage {
				t.Fatal(err)
			}
		})
	}
	cases := []struct{ name, body string }{
		{"unused_external", `<Relationship Id="r" Type="http://example.invalid/image" Target="https://example.invalid/image.png" TargetMode="External"/>`},
		{"duplicate_ids", officeTestRelation("same", "image", "document.xml") + officeTestRelation("same", "image", "document.xml")},
		{"active_ole_relation", officeTestRelation("r", "oleObject", "document.xml")},
		{"active_template_relation", officeTestRelation("r", "attachedTemplate", "document.xml")},
		{"active_altchunk_relation", officeTestRelation("r", "aFChunk", "document.xml")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			parts := append(officeTestParts("docx"), officeTestPart{name: "word/_rels/document.xml.rels", body: officeTestRels(test.body)})
			if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != ErrOfficePackage {
				t.Fatal(err)
			}
		})
	}
}

func TestOfficePackageEmbeddedWorkbookIsRecursivelyChecked(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func([]officeTestPart) []officeTestPart
	}{
		{"clean", func(p []officeTestPart) []officeTestPart { return p }},
		{"vba", func(p []officeTestPart) []officeTestPart { return append(p, officeTestPart{name: "xl/vbaProject.bin"}) }},
		{"external", func(p []officeTestPart) []officeTestPart {
			officeTestReplace(p, 1, `Id="rId1"`, `Id="rId1" TargetMode="External"`)
			return p
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			parts := append(officeTestParts("pptx"), officeTestPart{name: "ppt/embeddings/data.xlsx", body: officeTestZIP(t, test.mutate(officeTestParts("xlsx")))})
			officeTestOverride(parts, "ppt/embeddings/data.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
			err := InspectOfficePackage(officeTestZIP(t, parts), "pptx")
			if (err == nil) != (test.name == "clean") {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
	parts := append(officeTestParts("pptx"), officeTestPart{name: "ppt/embeddings/data.bin", body: officeTestZIP(t, officeTestParts("xlsx"))})
	if err := InspectOfficePackage(officeTestZIP(t, parts), "pptx"); err != ErrOfficePackage {
		t.Fatal("unknown embedded ZIP accepted")
	}
}

func TestOfficePackageZIPIntegrityAndResourceBounds(t *testing.T) {
	original := officeTestZIP(t, officeTestParts("docx"))
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"encrypted", func(b []byte) []byte { officeTestZIPField(b, 6, 8, 1, true); return b }},
		{"strong_encryption", func(b []byte) []byte { officeTestZIPField(b, 6, 8, 64, true); return b }},
		{"unsupported_compression", func(b []byte) []byte { officeTestZIPField(b, 8, 10, 99, false); return b }},
		{"local_name_mismatch", func(b []byte) []byte { b[30] = 'X'; return b }},
		{"CRC_corruption", func(b []byte) []byte {
			reader, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
			offset, _ := reader.File[0].DataOffset()
			b[offset] ^= 0xff
			return b
		}},
		{"trailing_data", func(b []byte) []byte { return append(b, 1) }},
		{"truncated", func(b []byte) []byte { return b[:len(b)-1] }},
		{"SFX_prefix", func(b []byte) []byte { return append([]byte("MZ"), b...) }},
		{"entry_size", func(b []byte) []byte {
			central := bytes.Index(b, []byte("PK\x01\x02"))
			binary.LittleEndian.PutUint32(b[central+24:], MaxOfficePackageEntryBytes+1)
			return b
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := InspectOfficePackage(test.mutate(bytes.Clone(original)), "docx"); err != ErrOfficePackage {
				t.Fatal(err)
			}
		})
	}
	t.Run("compressed_size", func(t *testing.T) {
		if err := InspectOfficePackage(make([]byte, MaxOfficePackageBytes+1), "docx"); err != ErrOfficePackage {
			t.Fatal(err)
		}
	})
	t.Run("entry_count", func(t *testing.T) {
		parts := officeTestParts("docx")
		for len(parts) <= MaxOfficePackageEntries {
			parts = append(parts, officeTestPart{name: fmt.Sprintf("word/part%d.bin", len(parts))})
		}
		if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != ErrOfficePackage {
			t.Fatal(err)
		}
	})
	// Shared counters prove nested containers cannot reset the outer budgets.
	for _, test := range []struct {
		name   string
		budget officeBudget
	}{
		{"aggregate_entries", officeBudget{entries: MaxOfficePackageEntries - 2}},
		{"aggregate_expansion", officeBudget{expanded: MaxOfficePackageExpandedBytes}},
		{"aggregate_xml_tokens", officeBudget{xmlTokens: 4_000_000}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := inspectOfficePackage(original, "docx", &test.budget, 0); err != ErrOfficePackage {
				t.Fatal(err)
			}
		})
	}
	if err := inspectOfficePackage(original, "docx", &officeBudget{}, 5); err != ErrOfficePackage {
		t.Fatal("nested depth accepted")
	}
	// A real compressed oversized entry is small on disk and must fail before decompression.
	t.Run("expansion_bomb", func(t *testing.T) {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, part := range officeTestParts("docx") {
			stream, _ := writer.Create(part.name)
			_, _ = stream.Write(part.body)
		}
		stream, err := writer.Create("word/bomb.bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.CopyN(stream, officeTestZeroReader{}, MaxOfficePackageEntryBytes+1); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		if buffer.Len() > 1<<20 {
			t.Fatal("fixture is not a small compressed archive")
		}
		if err = InspectOfficePackage(buffer.Bytes(), "docx"); err != ErrOfficePackage {
			t.Fatal(err)
		}
	})
}

type officeTestZeroReader struct{}

func (officeTestZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
func officeTestZIPField(b []byte, localOffset, centralOffset int, value uint16, or bool) {
	central := bytes.Index(b, []byte("PK\x01\x02"))
	if or {
		binary.LittleEndian.PutUint16(b[localOffset:], binary.LittleEndian.Uint16(b[localOffset:])|value)
		binary.LittleEndian.PutUint16(b[central+centralOffset:], binary.LittleEndian.Uint16(b[central+centralOffset:])|value)
	} else {
		binary.LittleEndian.PutUint16(b[localOffset:], value)
		binary.LittleEndian.PutUint16(b[central+centralOffset:], value)
	}
}

// Optional read-only compatibility evidence from explicitly supplied synthetic
// engine outputs. CI regression coverage above is entirely self-contained.
func TestOfficePackageSavedSyntheticFixtures(t *testing.T) {
	directory := os.Getenv("ANALYTIX_OFFICE_PREFLIGHT_FIXTURE_DIR")
	if directory == "" {
		t.Skip("synthetic saved-package fixture directory not supplied")
	}
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(directory, "saved."+kind))
			if err != nil {
				t.Fatal("cannot read synthetic fixture")
			}
			if err = InspectOfficePackage(content, kind); err != nil {
				t.Fatalf("synthetic fixture: %v", err)
			}
			reader, _ := zip.NewReader(bytes.NewReader(content), int64(len(content)))
			var expanded uint64
			for _, entry := range reader.File {
				expanded += entry.UncompressedSize64
			}
			t.Logf("kind=%s bytes=%d entries=%d expanded=%d sha256=%x", kind, len(content), len(reader.File), expanded, sha256.Sum256(content))
		})
	}
}

func TestOfficePackageZIPAliasesAndTruncation(t *testing.T) {
	for _, kind := range []uint16{0x0001, 0x7075, 0x0017, 0x9901} {
		t.Run(fmt.Sprintf("extra_%04x", kind), func(t *testing.T) {
			parts := officeTestParts("docx")
			extra := make([]byte, 4)
			binary.LittleEndian.PutUint16(extra, kind)
			parts[0].extra = extra
			if err := InspectOfficePackage(officeTestZIP(t, parts), "docx"); err != ErrOfficePackage {
				t.Fatal(err)
			}
		})
	}
	original := officeTestZIP(t, officeTestParts("docx"))
	for size := 0; size < len(original); size++ {
		if err := InspectOfficePackage(original[:size], "docx"); err != ErrOfficePackage {
			t.Fatalf("truncated size %d accepted", size)
		}
	}
	// Deterministic byte corruption exercises offset/length guards without a
	// fuzzing process, external corpus, or unbounded resource consumption.
	for offset := 0; offset < len(original); offset += 7 {
		mutated := bytes.Clone(original)
		mutated[offset] ^= 0xff
		err := InspectOfficePackage(mutated, "docx")
		if err != nil && err != ErrOfficePackage {
			t.Fatal("non-closed error")
		}
	}
}
