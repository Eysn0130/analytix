package presentationcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
)

func testShape(id, name, text, preset, color string, x int) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%s" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="360000"/><a:ext cx="1440000" cy="720000"/></a:xfrm><a:prstGeom prst="%s"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`, id, name, x, preset, color, text)
}
func testParts() map[string][]byte {
	slide := func(shapes string) []byte {
		return []byte(`<p:sld xmlns:p="` + pNS + `" xmlns:a="` + aNS + `"><p:cSld><p:spTree><p:nvGrpSpPr/><p:grpSpPr/>` + shapes + `</p:spTree></p:cSld></p:sld>`)
	}
	return map[string][]byte{
		"[Content_Types].xml":             []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`),
		"ppt/presentation.xml":            []byte(`<p:presentation xmlns:p="` + pNS + `" xmlns:r="` + rNS + `"><p:sldIdLst><p:sldId id="256" r:id="second-file"/><p:sldId id="257" r:id="first-file"/></p:sldIdLst><p:sldSz cx="10080000" cy="5670000"/></p:presentation>`),
		"ppt/_rels/presentation.xml.rels": []byte(`<Relationships xmlns="` + relsNS + `"><Relationship Id="second-file" Type="` + rNS + `/slide" Target="slides/slide2.xml"/><Relationship Id="first-file" Type="` + rNS + `/slide" Target="slides/slide1.xml"/></Relationships>`),
		"ppt/slides/slide2.xml":           slide(testShape("7", "Target", "Synthetic", "rect", "ffffff", 360000) + testShape("9", "Neighbor", "Other", "ellipse", "3478a0", 2160000)),
		"ppt/slides/slide1.xml":           slide(testShape("1", "Other slide", "Unchanged", "rect", "ffffff", 360000)),
		"ppt/notesSlides/notesSlide1.xml": []byte(`<notes>Unchanged note</notes>`),
		"ppt/theme/theme1.xml":            []byte(`<theme>Unchanged theme</theme>`),
		"ppt/media/image.png":             []byte("synthetic-media"),
	}
}
func testZIP(t *testing.T, parts map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	names := make([]string, 0, len(parts))
	for n := range parts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(parts[name]); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func testSelection() office.PresentationSelection {
	return office.PresentationSelection{PageWidth100thMM: 28000, PageHeight100thMM: 15750, Shapes: []office.PresentationShape{{Kind: "rectangle", Name: "Target", Text: "Synthetic", X100thMM: 1000, Y100thMM: 1000, Width100thMM: 4000, Height100thMM: 2000, FillRGB: "#ffffff"}, {ShapeIndex: 1, Kind: "ellipse", Name: "Neighbor", Text: "Other", X100thMM: 6000, Y100thMM: 1000, Width100thMM: 4000, Height100thMM: 2000, FillRGB: "#3478a0"}}}
}
func TestPresentationVerifiedNativeChangesKeepWholePackageOutsideTarget(t *testing.T) {
	ctx := context.Background()
	selected := testSelection()
	original := testZIP(t, testParts())
	if e := VerifySelectionPackage(ctx, original, selected); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"shape-fill", "shape-geometry"} {
		t.Run(kind, func(t *testing.T) {
			patch := office.PresentationPatch{Kind: kind, RGB: "#12abef"}
			x, y, w, h := 2000, 3000, 5000, 2500
			if kind == "shape-geometry" {
				patch = office.PresentationPatch{Kind: kind, X100thMM: &x, Y100thMM: &y, Width100thMM: &w, Height100thMM: &h}
			}
			review, e := office.ApplyPresentationPatch(selected, patch)
			if e != nil {
				t.Fatal(e)
			}
			parts := testParts()
			body := string(parts["ppt/slides/slide2.xml"])
			if kind == "shape-fill" {
				body = strings.Replace(body, `val="ffffff"`, `val="12ABEF"`, 1)
			} else {
				body = strings.Replace(body, `<a:off x="360000" y="360000"/><a:ext cx="1440000" cy="720000"/>`, `<a:off x="720000" y="1080000"/><a:ext cx="1800000" cy="900000"/>`, 1)
			}
			parts["ppt/slides/slide2.xml"] = []byte(body)
			if e = VerifyPatchedPackage(ctx, original, testZIP(t, parts), review); e != nil {
				t.Fatal("valid candidate", e)
			}
			for _, part := range []string{"ppt/notesSlides/notesSlide1.xml", "ppt/theme/theme1.xml", "ppt/media/image.png", "ppt/slides/slide1.xml", "ppt/_rels/presentation.xml.rels"} {
				old := parts[part]
				parts[part] = append(append([]byte{}, old...), []byte(" ")...)
				if VerifyPatchedPackage(ctx, original, testZIP(t, parts), review) == nil {
					t.Fatal("outside part changed", part)
				}
				parts[part] = old
			}
			for _, corrupt := range []string{strings.Replace(body, "Other", "Changed neighbor", 1), strings.Replace(body, `name="Target"`, `name="Changed target name"`, 1), strings.Replace(body, `<a:bodyPr/>`, `<a:bodyPr wrap="none"/>`, 1), strings.Replace(body, `<p:spPr>`, `<p:spPr><a:effectLst/></p:spPr><p:spPr>`, 1)} {
				parts["ppt/slides/slide2.xml"] = []byte(corrupt)
				if VerifyPatchedPackage(ctx, original, testZIP(t, parts), review) == nil {
					t.Fatal("outside-property corruption accepted")
				}
			}
		})
	}
}
func TestPresentationCaptureRejectsUnmappedAndUnsupportedObjects(t *testing.T) {
	ctx := context.Background()
	s := testSelection()
	for _, replacement := range [][2]string{{`<a:xfrm>`, `<a:xfrm rot="false">`}, {`<p:grpSpPr/>`, `<p:grpSpPr/><p:grpSpPr/>`}, {`<p:txBody>`, `<p:txBody/><p:txBody>`}, {`x="360000"`, `x="360001"`}, {`<a:xfrm>`, `<a:xfrm rot="1">`}, {`<a:xfrm>`, `<a:xfrm flipH="1">`}, {`<p:nvPr/>`, `<p:nvPr><p:ph/></p:nvPr>`}, {`<p:spPr>`, `<p:style/><p:spPr>`}, {`<a:srgbClr val="ffffff"/>`, `<a:schemeClr val="accent1"/>`}, {`<a:srgbClr val="ffffff"/>`, `<a:srgbClr val="ffffff"><a:alpha val="50000"/></a:srgbClr>`}, {`<p:spTree>`, `<p:spTree><p:grpSp/>`}, {`<p:spTree>`, `<p:spTree><p:graphicFrame/>`}, {`<a:avLst/>`, `<a:avLst><a:gd name="adj" fmla="val 1"/></a:avLst>`}} {
		parts := testParts()
		parts["ppt/slides/slide2.xml"] = []byte(strings.Replace(string(parts["ppt/slides/slide2.xml"]), replacement[0], replacement[1], 1))
		if VerifySelectionPackage(ctx, testZIP(t, parts), s) == nil {
			t.Fatal("unsupported shape accepted", replacement)
		}
	}
	for _, mutate := range []func(*office.PresentationSelection){func(s *office.PresentationSelection) { s.Shapes = s.Shapes[:1] }, func(s *office.PresentationSelection) {
		s.Shapes[0], s.Shapes[1] = s.Shapes[1], s.Shapes[0]
		s.Shapes[0].ShapeIndex = 0
		s.Shapes[1].ShapeIndex = 1
	}, func(s *office.PresentationSelection) { s.PageIndex = 1 }, func(s *office.PresentationSelection) { s.Shapes[0].FillRGB = "#000000" }} {
		bad := testSelection()
		mutate(&bad)
		if VerifySelectionPackage(ctx, testZIP(t, testParts()), bad) == nil {
			t.Fatal("forged snapshot accepted")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if VerifySelectionPackage(ctx, testZIP(t, testParts()), s) == nil {
		t.Fatal("cancellation ignored")
	}
}
