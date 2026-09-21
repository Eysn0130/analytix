// Package presentationcodec verifies native PPTX candidates without rewriting
// imported packages or granting filesystem authority.
package presentationcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
)

var ErrUnsupportedPresentation = errors.New("presentation_unsupported")

const (
	pNS    = "http://schemas.openxmlformats.org/presentationml/2006/main"
	aNS    = "http://schemas.openxmlformats.org/drawingml/2006/main"
	rNS    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	relsNS = "http://schemas.openxmlformats.org/package/2006/relationships"
)

type node struct {
	Name    xml.Name
	Attr    []xml.Attr
	Content []content
}
type content struct {
	Element *node
	Text    string
	Comment string
}

func parseXML(body []byte) (*node, error) {
	if len(body) > 2<<20 {
		return nil, ErrUnsupportedPresentation
	}
	d := xml.NewDecoder(bytes.NewReader(body))
	var root *node
	var stack []*node
	for tokens := 0; ; tokens++ {
		if tokens > 100000 || len(stack) > 64 {
			return nil, ErrUnsupportedPresentation
		}
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrUnsupportedPresentation
		}
		switch v := t.(type) {
		case xml.StartElement:
			n := &node{Name: v.Name}
			seen := map[xml.Name]bool{}
			for _, a := range v.Attr {
				if seen[a.Name] {
					return nil, ErrUnsupportedPresentation
				}
				seen[a.Name] = true
				if a.Name.Space == "xmlns" || a.Name.Space == "" && a.Name.Local == "xmlns" {
					continue
				}
				n.Attr = append(n.Attr, a)
			}
			sort.Slice(n.Attr, func(i, j int) bool {
				a, b := n.Attr[i].Name, n.Attr[j].Name
				return a.Space < b.Space || a.Space == b.Space && a.Local < b.Local
			})
			if len(stack) == 0 {
				if root != nil {
					return nil, ErrUnsupportedPresentation
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.Content = append(parent.Content, content{Element: n})
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, ErrUnsupportedPresentation
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(v)) != "" {
					return nil, ErrUnsupportedPresentation
				}
				continue
			}
			parent := stack[len(stack)-1]
			// Whitespace in text nodes is semantic; indentation between elements is not.
			if strings.TrimSpace(string(v)) != "" || parent.Name == (xml.Name{Space: aNS, Local: "t"}) {
				parent.Content = append(parent.Content, content{Text: string(v)})
			}
		case xml.Comment:
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Content = append(parent.Content, content{Comment: string(v)})
			}
		case xml.ProcInst:
			if v.Target != "xml" || root != nil {
				return nil, ErrUnsupportedPresentation
			}
		default:
			return nil, ErrUnsupportedPresentation
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, ErrUnsupportedPresentation
	}
	return root, nil
}

func children(n *node, space, local string) []*node {
	var out []*node
	if n != nil {
		for _, c := range n.Content {
			if c.Element != nil && c.Element.Name == (xml.Name{Space: space, Local: local}) {
				out = append(out, c.Element)
			}
		}
	}
	return out
}
func one(n *node, space, local string) *node {
	v := children(n, space, local)
	if len(v) == 1 {
		return v[0]
	}
	return nil
}
func attr(n *node, space, local string) string {
	if n != nil {
		for _, a := range n.Attr {
			if a.Name == (xml.Name{Space: space, Local: local}) {
				return a.Value
			}
		}
	}
	return ""
}
func setAttr(n *node, local, value string) {
	for i := range n.Attr {
		if n.Attr[i].Name == (xml.Name{Local: local}) {
			n.Attr[i].Value = value
			return
		}
	}
}
func only(n *node, names ...xml.Name) bool {
	if n == nil {
		return false
	}
	for _, c := range n.Content {
		if c.Element == nil {
			if c.Text != "" {
				return false
			}
			continue
		}
		found := false
		for _, name := range names {
			if c.Element.Name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func emu(value string) (int, error) {
	v, e := strconv.ParseInt(value, 10, 64)
	if e != nil || v < 0 || v > int64(office.PresentationMaxDimension)*360 || v%360 != 0 {
		return 0, ErrUnsupportedPresentation
	}
	return int(v / 360), nil
}
func readZIP(ctx context.Context, body []byte) (map[string][]byte, error) {
	if len(body) == 0 || len(body) > 16<<20 {
		return nil, ErrUnsupportedPresentation
	}
	z, e := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if e != nil || len(z.File) > 2048 {
		return nil, ErrUnsupportedPresentation
	}
	out := map[string][]byte{}
	var total uint64
	for _, f := range z.File {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if f.Name == "" || f.Name != path.Clean(f.Name) || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") || strings.ContainsAny(f.Name, "\\\x00") || f.UncompressedSize64 > 16<<20 || out[f.Name] != nil {
			return nil, ErrUnsupportedPresentation
		}
		total += f.UncompressedSize64
		if total > 64<<20 {
			return nil, ErrUnsupportedPresentation
		}
		r, err := f.Open()
		if err != nil {
			return nil, ErrUnsupportedPresentation
		}
		b, err := io.ReadAll(io.LimitReader(r, 16<<20+1))
		closeErr := r.Close()
		if err != nil || closeErr != nil || len(b) > 16<<20 || uint64(len(b)) != f.UncompressedSize64 {
			return nil, ErrUnsupportedPresentation
		}
		out[f.Name] = b
	}
	return out, nil
}

type page struct {
	files     map[string][]byte
	part      string
	root      *node
	shapes    []*node
	selection office.PresentationSelection
}

func loadPage(ctx context.Context, body []byte, pageIndex, targetIndex int) (page, error) {
	var out page
	files, e := readZIP(ctx, body)
	if e != nil {
		return out, e
	}
	out.files = files
	p, e := parseXML(files["ppt/presentation.xml"])
	if e != nil || p.Name != (xml.Name{Space: pNS, Local: "presentation"}) {
		return out, ErrUnsupportedPresentation
	}
	size := one(p, pNS, "sldSz")
	w, e1 := emu(attr(size, "", "cx"))
	h, e2 := emu(attr(size, "", "cy"))
	if e1 != nil || e2 != nil || w == 0 || h == 0 {
		return out, ErrUnsupportedPresentation
	}
	idList := one(p, pNS, "sldIdLst")
	if !only(idList, xml.Name{Space: pNS, Local: "sldId"}) {
		return out, ErrUnsupportedPresentation
	}
	ids := children(idList, pNS, "sldId")
	if len(ids) == 0 || len(ids) > 512 || pageIndex < 0 || pageIndex >= len(ids) {
		return out, ErrUnsupportedPresentation
	}
	rels, e := parseXML(files["ppt/_rels/presentation.xml.rels"])
	if e != nil || rels.Name != (xml.Name{Space: relsNS, Local: "Relationships"}) {
		return out, ErrUnsupportedPresentation
	}
	byID := map[string]string{}
	relIDs := map[string]bool{}
	for _, r := range children(rels, relsNS, "Relationship") {
		id := attr(r, "", "Id")
		if id == "" || relIDs[id] {
			return out, ErrUnsupportedPresentation
		}
		relIDs[id] = true
		if attr(r, "", "Type") != rNS+"/slide" {
			continue
		}
		target := attr(r, "", "Target")
		mode := attr(r, "", "TargetMode")
		if target == "" || strings.ContainsAny(target, "\\:#?%") || strings.HasPrefix(target, "/") || (mode != "" && mode != "Internal") {
			return out, ErrUnsupportedPresentation
		}
		part := path.Join("ppt", target)
		if !strings.HasPrefix(part, "ppt/") {
			return out, ErrUnsupportedPresentation
		}
		byID[id] = part
	}
	seen := map[string]bool{}
	for i, id := range ids {
		part := byID[attr(id, rNS, "id")]
		if part == "" || seen[part] {
			return out, ErrUnsupportedPresentation
		}
		seen[part] = true
		if i == pageIndex {
			out.part = part
		}
	}
	root, e := parseXML(files[out.part])
	if e != nil || root.Name != (xml.Name{Space: pNS, Local: "sld"}) {
		return out, ErrUnsupportedPresentation
	}
	out.root = root
	if v := attr(root, "", "show"); v != "" && v != "1" && v != "true" {
		return out, ErrUnsupportedPresentation
	}
	tree := one(one(root, pNS, "cSld"), pNS, "spTree")
	if !only(tree, xml.Name{Space: pNS, Local: "nvGrpSpPr"}, xml.Name{Space: pNS, Local: "grpSpPr"}, xml.Name{Space: pNS, Local: "sp"}) {
		return out, ErrUnsupportedPresentation
	}
	if len(children(tree, pNS, "grpSpPr")) != 1 || len(children(tree, pNS, "nvGrpSpPr")) != 1 {
		return out, ErrUnsupportedPresentation
	}
	// A root group transform changes the coordinate space. Only the conventional
	// zero root transform is admitted; nested groups are never interpreted.
	if group := one(tree, pNS, "grpSpPr"); group != nil {
		for _, c := range group.Content {
			if c.Element == nil {
				continue
			}
			x := c.Element
			if x.Name != (xml.Name{Space: aNS, Local: "xfrm"}) || len(x.Attr) != 0 {
				return out, ErrUnsupportedPresentation
			}
			for _, v := range x.Content {
				if v.Element == nil {
					continue
				}
				if v.Element.Name.Space != aNS || !strings.Contains(" off ext chOff chExt ", " "+v.Element.Name.Local+" ") {
					return out, ErrUnsupportedPresentation
				}
				for _, a := range v.Element.Attr {
					if a.Name.Space != "" || a.Value != "0" {
						return out, ErrUnsupportedPresentation
					}
				}
			}
		}
	}
	out.shapes = children(tree, pNS, "sp")
	s := office.PresentationSelection{PageIndex: pageIndex, TargetShapeIndex: targetIndex, PageWidth100thMM: w, PageHeight100thMM: h}
	shapeIDs := map[string]bool{}
	for i, sp := range out.shapes {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		v, err := shapeSnapshot(sp, i)
		if err != nil {
			return out, err
		}
		id := attr(one(one(sp, pNS, "nvSpPr"), pNS, "cNvPr"), "", "id")
		if id == "" || shapeIDs[id] {
			return out, ErrUnsupportedPresentation
		}
		shapeIDs[id] = true
		s.Shapes = append(s.Shapes, v)
	}
	if s.Validate() != nil {
		return out, ErrUnsupportedPresentation
	}
	out.selection = s
	return out, nil
}

func shapeSnapshot(sp *node, index int) (office.PresentationShape, error) {
	s := office.PresentationShape{ShapeIndex: index}
	if !only(sp, xml.Name{Space: pNS, Local: "nvSpPr"}, xml.Name{Space: pNS, Local: "spPr"}, xml.Name{Space: pNS, Local: "txBody"}) || len(sp.Attr) != 0 || len(children(sp, pNS, "txBody")) > 1 {
		return s, ErrUnsupportedPresentation
	}
	nv := one(sp, pNS, "nvSpPr")
	nvpr := one(nv, pNS, "nvPr")
	cnv := one(nv, pNS, "cNvPr")
	cnvsp := one(nv, pNS, "cNvSpPr")
	if !only(nv, xml.Name{Space: pNS, Local: "cNvPr"}, xml.Name{Space: pNS, Local: "cNvSpPr"}, xml.Name{Space: pNS, Local: "nvPr"}) || !only(nvpr) || cnv == nil || cnvsp == nil || attr(cnv, "", "hidden") != "" && attr(cnv, "", "hidden") != "0" && attr(cnv, "", "hidden") != "false" {
		return s, ErrUnsupportedPresentation
	}
	s.Name = attr(cnv, "", "name")
	props := one(sp, pNS, "spPr")
	x := one(props, aNS, "xfrm")
	if x == nil || !only(x, xml.Name{Space: aNS, Local: "off"}, xml.Name{Space: aNS, Local: "ext"}) {
		return s, ErrUnsupportedPresentation
	}
	for _, a := range x.Attr {
		if a.Name.Space != "" || (a.Name.Local != "rot" && a.Name.Local != "flipH" && a.Name.Local != "flipV") || (a.Value != "0" && (a.Name.Local == "rot" || a.Value != "false")) {
			return s, ErrUnsupportedPresentation
		}
	}
	off, ext := one(x, aNS, "off"), one(x, aNS, "ext")
	var err error
	for _, v := range []struct {
		n  *node
		k  string
		to *int
	}{{off, "x", &s.X100thMM}, {off, "y", &s.Y100thMM}, {ext, "cx", &s.Width100thMM}, {ext, "cy", &s.Height100thMM}} {
		*v.to, err = emu(attr(v.n, "", v.k))
		if err != nil {
			return s, err
		}
	}
	geom := one(props, aNS, "prstGeom")
	preset := attr(geom, "", "prst")
	if preset == "rect" {
		s.Kind = "rectangle"
	} else if preset == "ellipse" {
		s.Kind = "ellipse"
	} else {
		return s, ErrUnsupportedPresentation
	}
	if !only(geom, xml.Name{Space: aNS, Local: "avLst"}) || !only(one(geom, aNS, "avLst")) {
		return s, ErrUnsupportedPresentation
	}
	if box := attr(cnvsp, "", "txBox"); box == "1" || box == "true" {
		if s.Kind != "rectangle" {
			return s, ErrUnsupportedPresentation
		}
		s.Kind = "text"
	} else if box != "" && box != "0" && box != "false" {
		return s, ErrUnsupportedPresentation
	}
	fill := one(props, aNS, "solidFill")
	color := one(fill, aNS, "srgbClr")
	if !only(fill, xml.Name{Space: aNS, Local: "srgbClr"}) || color == nil || !only(color) || len(color.Attr) != 1 {
		return s, ErrUnsupportedPresentation
	}
	for _, name := range []string{"noFill", "gradFill", "blipFill", "pattFill", "grpFill", "custGeom", "scene3d", "sp3d"} {
		if len(children(props, aNS, name)) != 0 {
			return s, ErrUnsupportedPresentation
		}
	}
	s.FillRGB = "#" + strings.ToLower(attr(color, "", "val"))
	if tx := one(sp, pNS, "txBody"); tx != nil {
		if !only(tx, xml.Name{Space: aNS, Local: "bodyPr"}, xml.Name{Space: aNS, Local: "lstStyle"}, xml.Name{Space: aNS, Local: "p"}) {
			return s, ErrUnsupportedPresentation
		}
		var paragraphs []string
		for _, p := range children(tx, aNS, "p") {
			if !only(p, xml.Name{Space: aNS, Local: "pPr"}, xml.Name{Space: aNS, Local: "r"}, xml.Name{Space: aNS, Local: "endParaRPr"}) {
				return s, ErrUnsupportedPresentation
			}
			var b strings.Builder
			for _, r := range children(p, aNS, "r") {
				if !only(r, xml.Name{Space: aNS, Local: "rPr"}, xml.Name{Space: aNS, Local: "t"}) {
					return s, ErrUnsupportedPresentation
				}
				t := one(r, aNS, "t")
				if t == nil {
					return s, ErrUnsupportedPresentation
				}
				for _, c := range t.Content {
					if c.Element != nil {
						return s, ErrUnsupportedPresentation
					}
					b.WriteString(c.Text)
				}
			}
			paragraphs = append(paragraphs, b.String())
		}
		s.Text = strings.Join(paragraphs, "\n")
	}
	return s, nil
}

func VerifySelectionPackage(ctx context.Context, body []byte, s office.PresentationSelection) error {
	if s.Validate() != nil {
		return office.ErrInvalidPresentationPatch
	}
	p, e := loadPage(ctx, body, s.PageIndex, s.TargetShapeIndex)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(p.selection, s) {
		return office.ErrInvalidPresentationPatch
	}
	return nil
}

func VerifyPatchedPackage(ctx context.Context, original, candidate []byte, r office.PresentationReview) error {
	if office.ValidatePresentationReview(r) != nil {
		return office.ErrInvalidPresentationPatch
	}
	a, e := loadPage(ctx, original, r.Before.PageIndex, r.Before.TargetShapeIndex)
	if e != nil {
		return e
	}
	b, e := loadPage(ctx, candidate, r.After.PageIndex, r.After.TargetShapeIndex)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(a.selection, r.Before) || !reflect.DeepEqual(b.selection, r.After) || a.part != b.part || len(a.files) != len(b.files) {
		return office.ErrInvalidPresentationPatch
	}
	// Every other OPC part is opaque and must remain byte-identical. Unknown
	// exporter changes are rejected, not erased by a broad normalization rule.
	for name, body := range a.files {
		if name != a.part && !bytes.Equal(body, b.files[name]) {
			return office.ErrInvalidPresentationPatch
		}
	}
	before, after := r.Before.Shapes[r.Before.TargetShapeIndex], r.After.Shapes[r.After.TargetShapeIndex]
	props := one(a.shapes[r.Before.TargetShapeIndex], pNS, "spPr")
	if before.FillRGB != after.FillRGB {
		// The candidate snapshot already proved the exact RGB value. Preserve its
		// legal hex casing at the one approved attribute; other colors stay opaque.
		candidateProps := one(b.shapes[r.After.TargetShapeIndex], pNS, "spPr")
		color := one(one(candidateProps, aNS, "solidFill"), aNS, "srgbClr")
		setAttr(one(one(props, aNS, "solidFill"), aNS, "srgbClr"), "val", attr(color, "", "val"))
	} else {
		x := one(props, aNS, "xfrm")
		off, ext := one(x, aNS, "off"), one(x, aNS, "ext")
		for _, v := range []struct {
			n     *node
			k     string
			value int
		}{{off, "x", after.X100thMM}, {off, "y", after.Y100thMM}, {ext, "cx", after.Width100thMM}, {ext, "cy", after.Height100thMM}} {
			setAttr(v.n, v.k, strconv.FormatInt(int64(v.value)*360, 10))
		}
	}
	if !reflect.DeepEqual(a.root, b.root) {
		return office.ErrInvalidPresentationPatch
	}
	return nil
}
