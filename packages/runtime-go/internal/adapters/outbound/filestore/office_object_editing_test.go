//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
)

// Build native OOXML entirely in the test. These are structural fixtures, not
// Office engine/GUI acceptance evidence. Existing package helpers own ZIP/OPC.
func officeEditingNativeBytes(t *testing.T, kind, text string) []byte {
	t.Helper()
	parts := officeTestParts(kind)
	switch kind {
	case "docx":
		parts[2].body = []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`)
	case "xlsx":
		parts[2].body = []byte(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`)
		parts = append(parts,
			officeTestPart{name: "xl/_rels/workbook.xml.rels", body: officeTestRels(officeTestRelation("rId1", "worksheet", "worksheets/sheet1.xml"))},
			officeTestPart{name: "xl/worksheets/sheet1.xml", body: []byte(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>` + text + `</t></is></c></row></sheetData></worksheet>`)},
		)
		officeTestOverride(parts, "xl/worksheets/sheet1.xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml")
	case "pptx":
		parts[2].body = []byte(`<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>`)
		parts = append(parts,
			officeTestPart{name: "ppt/_rels/presentation.xml.rels", body: officeTestRels(officeTestRelation("rId1", "slide", "slides/slide1.xml"))},
			officeTestPart{name: "ppt/slides/slide1.xml", body: []byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`)},
		)
		officeTestOverride(parts, "ppt/slides/slide1.xml", "application/vnd.openxmlformats-officedocument.presentationml.slide+xml")
	default:
		t.Fatal("unknown fixture kind")
	}
	raw := officeTestZIP(t, parts)
	if err := InspectOfficePackage(raw, kind); err != nil {
		t.Fatalf("invalid native fixture: %v", err)
	}
	return raw
}

func officeEditingNativeFixture(t *testing.T, kind string, raw []byte) (*ObjectEditingFiles, string, string) {
	t.Helper()
	textStore, workspace, textPath := objectEditingFixture(t, raw)
	path := filepath.Join(workspace, "report."+kind)
	if err := os.Rename(textPath, path); err != nil {
		t.Fatal(err)
	}
	store, err := NewOfficeObjectEditingFiles(textStore.receiptRoot, nil, kind)
	if err != nil {
		t.Fatal(err)
	}
	return store, workspace, path
}

func officeEditingAssertBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("native bytes changed unexpectedly: %v", err)
	}
}

func TestOfficeObjectEditingPreviewAndDirectCommitForbidden(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			before := officeEditingNativeBytes(t, kind, "Original-office-marker")
			after := officeEditingNativeBytes(t, kind, "Requested-write")
			store, workspace, path := officeEditingNativeFixture(t, kind, before)
			input := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
			infoBefore, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, current := range []*ObjectEditingFiles{store, restarted} {
				current.replaceDocument = func(atomicTextReplaceRequest) error { t.Fatal("preview wrote the document"); return nil }
				current.replaceJournal = func(atomicTextReplaceRequest) error { t.Fatal("preview created a save journal"); return nil }
				document, err := current.Read(ctx, workspace, path)
				if err != nil || document.Content != base64.StdEncoding.EncodeToString(before) || document.Encoding != "office-base64" || document.Revision != digestAtomicText(before) {
					t.Fatalf("open native preview: %#v %v", document, err)
				}
				for attempt := 0; attempt < 2; attempt++ {
					if receipt, err := current.Commit(ctx, input); !errors.Is(err, objectediting.ErrForbidden) || receipt != (objectediting.Receipt{}) {
						t.Fatalf("direct commit: %#v %v", receipt, err)
					}
				}
				officeEditingAssertBytes(t, path, before)
			}
			infoAfter, err := os.Stat(path)
			if err != nil || !os.SameFile(infoBefore, infoAfter) || infoBefore.Mode() != infoAfter.Mode() || !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
				t.Fatalf("preview changed file identity, mode or modification time: %v", err)
			}
			entries, err := os.ReadDir(store.receiptRoot)
			if err != nil || len(entries) != 0 {
				t.Fatalf("preview journal entries: %v %v", entries, err)
			}

			// An external change is visible on the next read; stale preview input
			// remains unable to overwrite it or create a conflict/save receipt.
			external := officeEditingNativeBytes(t, kind, "External-change")
			if err := os.WriteFile(path, external, 0o640); err != nil {
				t.Fatal(err)
			}
			document, err := restarted.Read(ctx, workspace, path)
			if err != nil || document.Content != base64.StdEncoding.EncodeToString(external) || document.Revision != digestAtomicText(external) {
				t.Fatalf("external change preview: %#v %v", document, err)
			}
			if receipt, err := restarted.Commit(ctx, input); !errors.Is(err, objectediting.ErrForbidden) || receipt != (objectediting.Receipt{}) {
				t.Fatalf("stale direct commit: %#v %v", receipt, err)
			}
			officeEditingAssertBytes(t, path, external)
		})
	}
}

func TestOfficeObjectEditingEncodeAlwaysForbidden(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		store := &ObjectEditingFiles{officeKind: kind}
		for _, content := range []string{"", "invalid!", base64.StdEncoding.EncodeToString(officeEditingNativeBytes(t, kind, "Preview"))} {
			for _, encoding := range []string{"office-base64", "utf-8", "unknown"} {
				if raw, err := store.encodeObject(content, encoding); !errors.Is(err, objectediting.ErrForbidden) || raw != nil {
					t.Fatalf("Office codec allowed encoding: %v", err)
				}
			}
		}
	}
}

func TestOfficeObjectEditingRejectsInvalidPackagesWithoutMutation(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			before := officeEditingNativeBytes(t, kind, "Before")
			otherKind := map[string]string{"docx": "xlsx", "xlsx": "pptx", "pptx": "docx"}[kind]
			wrong := officeEditingNativeBytes(t, otherKind, "Wrong-kind")
			for _, test := range []struct {
				name, content string
				want          error
			}{
				{"not_zip", base64.StdEncoding.EncodeToString([]byte("not a ZIP archive")), objectediting.ErrForbidden},
				{"truncated_zip", base64.StdEncoding.EncodeToString(before[:len(before)-1]), objectediting.ErrForbidden},
				{"wrong_kind", base64.StdEncoding.EncodeToString(wrong), objectediting.ErrForbidden},
				{"bad_base64", "invalid!", objectediting.ErrForbidden},
				{"noncanonical_base64", base64.StdEncoding.EncodeToString(before) + "\n", objectediting.ErrForbidden},
			} {
				t.Run(test.name, func(t *testing.T) {
					store, workspace, path := officeEditingNativeFixture(t, kind, before)
					input := objectEditingInput(t, store, workspace, path, test.content)
					if receipt, err := store.Commit(ctx, input); !errors.Is(err, test.want) || receipt != (objectediting.Receipt{}) {
						t.Fatalf("invalid save: %#v %v", receipt, err)
					}
					officeEditingAssertBytes(t, path, before)
					entries, err := os.ReadDir(store.receiptRoot)
					if err != nil || len(entries) != 0 {
						t.Fatalf("invalid input created journal entries: %v %v", entries, err)
					}
				})
			}
			for name, raw := range map[string][]byte{"not_zip": []byte("plain text"), "truncated_zip": before[:len(before)-1], "wrong_kind": wrong} {
				t.Run("open_"+name, func(t *testing.T) {
					store, workspace, path := officeEditingNativeFixture(t, kind, raw)
					if _, err := store.Read(ctx, workspace, path); !errors.Is(err, objectediting.ErrNotText) {
						t.Fatalf("invalid open: %v", err)
					}
					officeEditingAssertBytes(t, path, raw)
				})
			}
			store, workspace, path := officeEditingNativeFixture(t, kind, before)
			wrongExtension := filepath.Join(workspace, "renamed."+otherKind)
			if err := os.Rename(path, wrongExtension); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Read(ctx, workspace, wrongExtension); !errors.Is(err, objectediting.ErrNotText) {
				t.Fatalf("wrong extension accepted: %v", err)
			}
			officeEditingAssertBytes(t, wrongExtension, before)
			if _, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "doc"); !errors.Is(err, objectediting.ErrInvalidInput) {
				t.Fatalf("unsupported kind accepted: %v", err)
			}
		})
	}
}

func TestOfficeObjectEditingDoesNotChangeDefaultTextCodec(t *testing.T) {
	ctx := context.Background()
	for _, encoding := range []string{filetoolsapp.TextEncodingUTF8, filetoolsapp.TextEncodingUTF8BOM, filetoolsapp.TextEncodingUTF16LE, filetoolsapp.TextEncodingUTF16BE} {
		t.Run(encoding, func(t *testing.T) {
			before := filetoolsapp.EncodeTextBytes("Original text 中文\r\n", encoding)
			store, workspace, path := objectEditingFixture(t, before)
			if _, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "docx"); err != nil {
				t.Fatal(err)
			}
			document, err := store.Read(ctx, workspace, path)
			if err != nil || document.Content != "Original text 中文\r\n" || document.Encoding != encoding {
				t.Fatalf("text open changed: %#v %v", document, err)
			}
			input := objectEditingInput(t, store, workspace, path, "Edited text 中文 😀\r\n")
			receipt, err := store.Commit(ctx, input)
			want := filetoolsapp.EncodeTextBytes(input.Content, encoding)
			if err != nil || receipt.Status != objectediting.StatusCommitted || receipt.Revision != digestAtomicText(want) {
				t.Fatalf("text save changed: %#v %v", receipt, err)
			}
			officeEditingAssertBytes(t, path, want)
			if store.maxObjectBytes() != objectediting.MaxTextBytes || store.maxContentBytes() != objectediting.MaxTextBytes {
				t.Fatal("default text limits changed")
			}
			if err := os.WriteFile(path, officeEditingNativeBytes(t, "docx", "Binary"), 0o640); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Read(ctx, workspace, path); !errors.Is(err, objectediting.ErrNotText) {
				t.Fatalf("default text codec accepted native ZIP: %v", err)
			}
		})
	}
}
