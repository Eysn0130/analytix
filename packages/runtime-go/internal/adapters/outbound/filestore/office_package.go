package filestore

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
)

const (
	MaxOfficePackageBytes         = 32 << 20
	MaxOfficePackageEntries       = 4096
	MaxOfficePackageExpandedBytes = 128 << 20
	MaxOfficePackageEntryBytes    = 32 << 20
	officeContentTypesNS          = "http://schemas.openxmlformats.org/package/2006/content-types"
	officeRelationshipsNS         = "http://schemas.openxmlformats.org/package/2006/relationships"
	officeRelationshipsType       = "application/vnd.openxmlformats-package.relationships+xml"
)

// ErrOfficePackage deliberately contains no package paths, XML, relationships or
// archive diagnostics. This is a bounded structural preflight, not a guarantee
// against malicious documents or vulnerabilities in an Office/media renderer.
var ErrOfficePackage = errors.New("office_package_invalid")

type officeBudget struct {
	entries   int
	expanded  uint64
	xmlTokens int
}
type officePart struct {
	body        []byte
	contentType string
}
type officeKind struct{ mainType, root, namespace, strictNamespace string }

func officeKindFor(kind string) (officeKind, bool) {
	switch kind {
	case "docx":
		return officeKind{"application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", "document", "http://schemas.openxmlformats.org/wordprocessingml/2006/main", "http://purl.oclc.org/ooxml/wordprocessingml/main"}, true
	case "xlsx":
		return officeKind{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml", "workbook", "http://schemas.openxmlformats.org/spreadsheetml/2006/main", "http://purl.oclc.org/ooxml/spreadsheetml/main"}, true
	case "pptx":
		return officeKind{"application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml", "presentation", "http://schemas.openxmlformats.org/presentationml/2006/main", "http://purl.oclc.org/ooxml/presentationml/main"}, true
	}
	return officeKind{}, false
}

// InspectOfficePackage never extracts files, resolves external resources, starts
// an engine or returns document content. Embedded OOXML (e.g. a chart workbook)
// shares the outer entry/expanded-byte/XML-token budget, with depth at most four.
// ZIP64, non-ASCII/percent-escaped archive part names, XML encodings other than
// UTF-8, DTDs, processing instructions other than XML declarations, and every
// external relationship are intentionally outside this conservative profile.
func InspectOfficePackage(content []byte, kind string) error {
	if err := inspectOfficePackage(content, kind, &officeBudget{}, 0); err != nil {
		return ErrOfficePackage
	}
	return nil
}

func inspectOfficePackage(content []byte, kind string, budget *officeBudget, nesting int) error {
	format, ok := officeKindFor(kind)
	if !ok || nesting > 4 || len(content) == 0 || len(content) > MaxOfficePackageBytes {
		return ErrOfficePackage
	}
	files, err := officeZIPFiles(content)
	if err != nil {
		return err
	}
	if budget.entries > MaxOfficePackageEntries-len(files) {
		return ErrOfficePackage
	}
	budget.entries += len(files)
	parts := make(map[string]officePart, len(files))
	names := make(map[string]bool, len(files))
	folded := make(map[string]bool, len(files))
	var declared uint64
	for _, file := range files {
		directory := file.FileInfo().IsDir()
		name := strings.TrimSuffix(file.Name, "/")
		if !officePartName(name) || folded[strings.ToLower(name)] || file.Mode()&os.ModeSymlink != 0 || (!directory && !file.Mode().IsRegular()) || officeForbiddenName(name) {
			return ErrOfficePackage
		}
		folded[strings.ToLower(name)] = true
		names[name] = directory
		if file.UncompressedSize64 > MaxOfficePackageEntryBytes || file.CompressedSize64 > MaxOfficePackageBytes || declared > MaxOfficePackageExpandedBytes-file.UncompressedSize64 {
			return ErrOfficePackage
		}
		declared += file.UncompressedSize64
		if directory && (file.UncompressedSize64 != 0 || file.CompressedSize64 != 0) {
			return ErrOfficePackage
		}
	}
	if budget.expanded > MaxOfficePackageExpandedBytes-declared {
		return ErrOfficePackage
	}
	budget.expanded += declared
	for name := range names {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if directory, exists := names[parent]; exists && !directory {
				return ErrOfficePackage
			}
		}
	}
	for _, file := range files {
		if file.FileInfo().IsDir() {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return ErrOfficePackage
		}
		body, readErr := io.ReadAll(io.LimitReader(reader, MaxOfficePackageEntryBytes+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || uint64(len(body)) != file.UncompressedSize64 || len(body) > MaxOfficePackageEntryBytes || officeExecutableMagic(body) {
			return ErrOfficePackage
		}
		parts[file.Name] = officePart{body: body}
	}
	contentTypes, exists := parts["[Content_Types].xml"]
	if !exists {
		return ErrOfficePackage
	}
	defaults, overrides, err := officeContentTypes(contentTypes.body, parts, budget)
	if err != nil {
		return err
	}
	for name, part := range parts {
		if name == "[Content_Types].xml" {
			continue
		}
		part.contentType = overrides[name]
		if part.contentType == "" {
			part.contentType = defaults[strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))]
		}
		if part.contentType == "" || officeForbiddenType(part.contentType) {
			return ErrOfficePackage
		}
		parts[name] = part
	}
	main := ""
	mainCount := 0
	for name, part := range parts {
		if name == "[Content_Types].xml" {
			continue
		}
		if strings.EqualFold(path.Ext(name), ".rels") {
			if part.contentType != officeRelationshipsType {
				return ErrOfficePackage
			}
			target, err := officeRelationships(name, part.body, parts, budget)
			if err != nil {
				return err
			}
			if name == "_rels/.rels" {
				main = target
			}
		} else if part.contentType == officeRelationshipsType {
			return ErrOfficePackage
		}
		if part.contentType == format.mainType {
			mainCount++
		}
	}
	if main == "" || mainCount != 1 || parts[main].contentType != format.mainType {
		return ErrOfficePackage
	}
	for name, part := range parts {
		if name == "[Content_Types].xml" || strings.EqualFold(path.Ext(name), ".rels") {
			continue
		}
		embedded := officeEmbeddedKind(part.contentType)
		extension := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
		if embedded != "" {
			if extension != embedded || inspectOfficePackage(part.body, embedded, budget, nesting+1) != nil {
				return ErrOfficePackage
			}
			continue
		}
		if _, known := officeKindFor(extension); known || bytes.HasPrefix(part.body, []byte("PK\x03\x04")) {
			return ErrOfficePackage
		}
		if officeXMLPart(name, part.contentType, part.body) {
			root, err := officeScanXML(part.body, budget, nil)
			if err != nil {
				return err
			}
			if name == main && (root.Local != format.root || (root.Space != format.namespace && root.Space != format.strictNamespace)) {
				return ErrOfficePackage
			}
		} else if name == main {
			return ErrOfficePackage
		}
	}
	return nil
}

func officePartName(name string) bool {
	if name == "" || len(name) > 1024 || name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.ContainsAny(name, "\\\x00:%?#") {
		return false
	}
	for _, character := range name {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.TrimSpace(segment) != segment || strings.HasSuffix(segment, ".") {
			return false
		}
	}
	return true
}

func officeForbiddenName(name string) bool {
	lower := strings.ToLower(name)
	for _, segment := range strings.Split(lower, "/") {
		if strings.Contains(segment, "vba") || strings.Contains(segment, "activex") || strings.HasPrefix(segment, "oleobject") {
			return true
		}
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".exe", ".dll", ".com", ".scr", ".cpl", ".msi", ".msp", ".bat", ".cmd", ".ps1", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh", ".hta", ".jar", ".class", ".dylib", ".so", ".sh", ".py", ".pl", ".wasm", ".docm", ".dotm", ".xlsm", ".xltm", ".xlam", ".xlsb", ".pptm", ".potm", ".ppam", ".sldm":
		return true
	}
	return false
}

func officeForbiddenType(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"vba", "macroenabled", "macro-enabled", "macrosheet", "activex", "oleobject", "ole-object", "x-msdownload", "x-msdos-program", "x-executable", "javascript", "x-shockwave-flash", "x-silverlight"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func officeExecutableMagic(body []byte) bool {
	for _, prefix := range [][]byte{[]byte("MZ"), []byte("\x7fELF"), {0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, {0xfe, 0xed, 0xfa, 0xce}, {0xce, 0xfa, 0xed, 0xfe}, {0xfe, 0xed, 0xfa, 0xcf}, {0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}, {0xbe, 0xba, 0xfe, 0xca}, []byte("\x00asm"), []byte("#!")} {
		if bytes.HasPrefix(body, prefix) {
			return true
		}
	}
	return false
}

func officeEmbeddedKind(contentType string) string {
	switch contentType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx"
	}
	return ""
}

func officeXMLPart(name, contentType string, body []byte) bool {
	lower := strings.ToLower(contentType)
	ext := strings.ToLower(path.Ext(name))
	return ext == ".xml" || ext == ".vml" || ext == ".svg" || strings.HasSuffix(lower, "+xml") || lower == "application/xml" || lower == "text/xml" || bytes.HasPrefix(bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})), []byte("<"))
}

// Read XML tokens without an entity resolver. Reject directives and executable
// processing instructions in every XML part, including otherwise unused parts.
func officeScanXML(body []byte, budget *officeBudget, visit func(xml.StartElement, int) error) (xml.Name, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	depth := 0
	roots := 0
	declaration := false
	var root xml.Name
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return root, ErrOfficePackage
		}
		budget.xmlTokens++
		if budget.xmlTokens > 4_000_000 {
			return root, ErrOfficePackage
		}
		switch token := token.(type) {
		case xml.Directive:
			return root, ErrOfficePackage
		case xml.ProcInst:
			if token.Target != "xml" || declaration || roots != 0 || depth != 0 {
				return root, ErrOfficePackage
			}
			declaration = true
		case xml.StartElement:
			depth++
			if depth > 128 || len(token.Attr) > 256 {
				return root, ErrOfficePackage
			}
			if depth == 1 {
				roots++
				root = token.Name
				if roots != 1 {
					return root, ErrOfficePackage
				}
			}
			seen := map[xml.Name]bool{}
			for _, attribute := range token.Attr {
				if seen[attribute.Name] {
					return root, ErrOfficePackage
				}
				seen[attribute.Name] = true
			}
			lower := strings.ToLower(token.Name.Local)
			if lower == "oleobject" || lower == "ddelink" || lower == "script" || lower == "foreignobject" {
				return root, ErrOfficePackage
			}
			for _, attribute := range token.Attr {
				value := strings.ToLower(strings.TrimSpace(attribute.Value))
				if strings.HasPrefix(value, "ppaction://macro") || strings.HasPrefix(value, "ppaction://program") {
					return root, ErrOfficePackage
				}
			}
			if visit != nil {
				if err := visit(token, depth); err != nil {
					return root, err
				}
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if (depth == 0 || visit != nil) && len(bytes.TrimSpace(token)) != 0 {
				return root, ErrOfficePackage
			}
		}
	}
	if roots != 1 || depth != 0 {
		return root, ErrOfficePackage
	}
	return root, nil
}

func officeAttributes(element xml.StartElement, allowed ...string) (map[string]string, error) {
	attributes := map[string]string{}
	for _, attribute := range element.Attr {
		if attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns") {
			continue
		}
		if attribute.Name.Space != "" {
			return nil, ErrOfficePackage
		}
		ok := false
		for _, name := range allowed {
			if name == attribute.Name.Local {
				ok = true
				break
			}
		}
		if !ok || attribute.Value == "" {
			return nil, ErrOfficePackage
		}
		attributes[attribute.Name.Local] = attribute.Value
	}
	return attributes, nil
}

func officeContentTypes(body []byte, parts map[string]officePart, budget *officeBudget) (map[string]string, map[string]string, error) {
	defaults := map[string]string{}
	overrides := map[string]string{}
	folded := map[string]bool{}
	_, err := officeScanXML(body, budget, func(element xml.StartElement, depth int) error {
		if element.Name.Space != officeContentTypesNS {
			return ErrOfficePackage
		}
		if depth == 1 {
			if element.Name.Local != "Types" {
				return ErrOfficePackage
			}
			_, err := officeAttributes(element)
			return err
		}
		if depth != 2 {
			return ErrOfficePackage
		}
		attributes, err := officeAttributes(element, "Extension", "PartName", "ContentType")
		if err != nil || len(attributes) != 2 || officeForbiddenType(attributes["ContentType"]) {
			return ErrOfficePackage
		}
		contentType := attributes["ContentType"]
		parsedType, parameters, parseErr := mime.ParseMediaType(contentType)
		if parseErr != nil || parsedType != strings.ToLower(contentType) || len(parameters) != 0 || strings.TrimSpace(contentType) != contentType {
			return ErrOfficePackage
		}
		contentType = parsedType
		switch element.Name.Local {
		case "Default":
			extension := strings.ToLower(attributes["Extension"])
			if extension == "" || defaults[extension] != "" {
				return ErrOfficePackage
			}
			for _, character := range extension {
				if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
					return ErrOfficePackage
				}
			}
			defaults[extension] = contentType
		case "Override":
			partName := attributes["PartName"]
			if !strings.HasPrefix(partName, "/") {
				return ErrOfficePackage
			}
			partName = strings.TrimPrefix(partName, "/")
			if partName == "[Content_Types].xml" || !officePartName(partName) || folded[strings.ToLower(partName)] {
				return ErrOfficePackage
			}
			if _, exists := parts[partName]; !exists {
				return ErrOfficePackage
			}
			folded[strings.ToLower(partName)] = true
			overrides[partName] = contentType
		default:
			return ErrOfficePackage
		}
		return nil
	})
	return defaults, overrides, err
}

func officeRelationships(name string, body []byte, parts map[string]officePart, budget *officeBudget) (string, error) {
	source := ""
	if name != "_rels/.rels" {
		if path.Base(path.Dir(name)) != "_rels" {
			return "", ErrOfficePackage
		}
		source = path.Join(path.Dir(path.Dir(name)), strings.TrimSuffix(path.Base(name), ".rels"))
		if _, exists := parts[source]; !exists {
			return "", ErrOfficePackage
		}
	}
	ids := map[string]bool{}
	main := ""
	_, err := officeScanXML(body, budget, func(element xml.StartElement, depth int) error {
		if element.Name.Space != officeRelationshipsNS {
			return ErrOfficePackage
		}
		if depth == 1 {
			if element.Name.Local != "Relationships" {
				return ErrOfficePackage
			}
			_, err := officeAttributes(element)
			return err
		}
		if depth != 2 || element.Name.Local != "Relationship" {
			return ErrOfficePackage
		}
		attributes, err := officeAttributes(element, "Id", "Type", "Target", "TargetMode")
		if err != nil || len(attributes) < 3 || attributes["Id"] == "" || ids[attributes["Id"]] || attributes["Type"] == "" || attributes["Target"] == "" {
			return ErrOfficePackage
		}
		ids[attributes["Id"]] = true
		if mode := attributes["TargetMode"]; mode != "" && mode != "Internal" {
			return ErrOfficePackage
		}
		relationType := attributes["Type"]
		parsedType, parseErr := url.Parse(relationType)
		if parseErr != nil || parsedType.Scheme == "" || strings.TrimSpace(relationType) != relationType || officeForbiddenType(relationType) {
			return ErrOfficePackage
		}
		lower := strings.ToLower(relationType)
		for _, suffix := range []string{"/control", "/ctrlprop", "/attachedtemplate", "/externallink", "/connection", "/afchunk"} {
			if strings.HasSuffix(lower, suffix) {
				return ErrOfficePackage
			}
		}
		target, err := officeInternalTarget(source, attributes["Target"])
		if err != nil {
			return err
		}
		if _, exists := parts[target]; !exists || target == "[Content_Types].xml" || strings.HasSuffix(strings.ToLower(target), ".rels") {
			return ErrOfficePackage
		}
		if relationType == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" || relationType == "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument" {
			if name != "_rels/.rels" || main != "" {
				return ErrOfficePackage
			}
			main = target
		}
		return nil
	})
	return main, err
}

func officeInternalTarget(source, target string) (string, error) {
	if target == "" || strings.TrimSpace(target) != target || strings.ContainsAny(target, "\\\x00\r\n") {
		return "", ErrOfficePackage
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.ForceQuery {
		return "", ErrOfficePackage
	}
	decoded := parsed.Path
	if strings.ContainsAny(decoded, "\\\x00:%?#") {
		return "", ErrOfficePackage
	}
	var segments []string
	if !strings.HasPrefix(decoded, "/") && source != "" {
		base := path.Dir(source)
		if base != "." {
			segments = strings.Split(base, "/")
		}
	}
	if decoded == "" {
		if source == "" {
			return "", ErrOfficePackage
		}
		return source, nil
	}
	for _, segment := range strings.Split(strings.TrimPrefix(decoded, "/"), "/") {
		switch segment {
		case "":
			return "", ErrOfficePackage
		case ".":
			continue
		case "..":
			if len(segments) == 0 {
				return "", ErrOfficePackage
			}
			segments = segments[:len(segments)-1]
		default:
			segments = append(segments, segment)
		}
	}
	resolved := strings.Join(segments, "/")
	if !officePartName(resolved) {
		return "", ErrOfficePackage
	}
	return resolved, nil
}

// Bound the central-directory inventory before archive/zip allocates File
// records. Require one ZIP32 volume with coherent local/central names, flags,
// methods and payload ranges; no SFX prefix, overlapping entries or trailing ZIP.
func officeZIPFiles(content []byte) ([]*zip.File, error) {
	if len(content) < 22 || !bytes.HasPrefix(content, []byte("PK\x03\x04")) {
		return nil, ErrOfficePackage
	}
	end := -1
	start := len(content) - 22 - 65535
	if start < 0 {
		start = 0
	}
	for offset := len(content) - 22; offset >= start; offset-- {
		if binary.LittleEndian.Uint32(content[offset:]) == 0x06054b50 && offset+22+int(binary.LittleEndian.Uint16(content[offset+20:])) == len(content) {
			end = offset
			break
		}
	}
	if end < 0 {
		return nil, ErrOfficePackage
	}
	eocd := content[end:]
	count := int(binary.LittleEndian.Uint16(eocd[10:]))
	directorySize := uint64(binary.LittleEndian.Uint32(eocd[12:]))
	directoryOffset := uint64(binary.LittleEndian.Uint32(eocd[16:]))
	if binary.LittleEndian.Uint16(eocd[4:]) != 0 || binary.LittleEndian.Uint16(eocd[6:]) != 0 || int(binary.LittleEndian.Uint16(eocd[8:])) != count || count == 0 || count > MaxOfficePackageEntries || directoryOffset+directorySize != uint64(end) {
		return nil, ErrOfficePackage
	}
	cursor := int(directoryOffset)
	offsets := make([]int, 0, count)
	for cursor < end {
		if len(offsets) >= count || end-cursor < 46 || binary.LittleEndian.Uint32(content[cursor:]) != 0x02014b50 {
			return nil, ErrOfficePackage
		}
		header := content[cursor:]
		length := 46 + int(binary.LittleEndian.Uint16(header[28:])) + int(binary.LittleEndian.Uint16(header[30:])) + int(binary.LittleEndian.Uint16(header[32:]))
		local := uint64(binary.LittleEndian.Uint32(header[42:]))
		if length > end-cursor || binary.LittleEndian.Uint16(header[34:]) != 0 || local >= directoryOffset || binary.LittleEndian.Uint32(header[20:]) == 0xffffffff || binary.LittleEndian.Uint32(header[24:]) == 0xffffffff {
			return nil, ErrOfficePackage
		}
		extraStart := 46 + int(binary.LittleEndian.Uint16(header[28:]))
		extraEnd := extraStart + int(binary.LittleEndian.Uint16(header[30:]))
		if !officeZIPExtra(header[extraStart:extraEnd]) {
			return nil, ErrOfficePackage
		}
		offsets = append(offsets, int(local))
		cursor += length
	}
	if cursor != end || len(offsets) != count {
		return nil, ErrOfficePackage
	}
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil || len(reader.File) != count {
		return nil, ErrOfficePackage
	}
	type span struct{ start, end int }
	spans := make([]span, 0, count)
	for index, file := range reader.File {
		offset := offsets[index]
		if offset < 0 || offset > int(directoryOffset)-30 {
			return nil, ErrOfficePackage
		}
		header := content[offset:]
		if binary.LittleEndian.Uint32(header) != 0x04034b50 || binary.LittleEndian.Uint16(header[6:]) != file.Flags || binary.LittleEndian.Uint16(header[8:]) != file.Method || file.Flags&(1|64|8192) != 0 || (file.Method != zip.Store && file.Method != zip.Deflate) {
			return nil, ErrOfficePackage
		}
		nameLength := int(binary.LittleEndian.Uint16(header[26:]))
		extraLength := int(binary.LittleEndian.Uint16(header[28:]))
		data := offset + 30 + nameLength + extraLength
		if data > int(directoryOffset) || string(content[offset+30:offset+30+nameLength]) != file.Name || file.CompressedSize64 > uint64(int(directoryOffset)-data) {
			return nil, ErrOfficePackage
		}
		if !officeZIPExtra(content[offset+30+nameLength : data]) {
			return nil, ErrOfficePackage
		}
		dataOffset, err := file.DataOffset()
		if err != nil || dataOffset != int64(data) {
			return nil, ErrOfficePackage
		}
		finish := data + int(file.CompressedSize64)
		if file.Flags&8 == 0 {
			if binary.LittleEndian.Uint32(header[14:]) != file.CRC32 || uint64(binary.LittleEndian.Uint32(header[18:])) != file.CompressedSize64 || uint64(binary.LittleEndian.Uint32(header[22:])) != file.UncompressedSize64 {
				return nil, ErrOfficePackage
			}
		} else {
			if finish > int(directoryOffset)-12 {
				return nil, ErrOfficePackage
			}
			descriptor := content[finish:]
			if binary.LittleEndian.Uint32(descriptor) == 0x08074b50 {
				finish += 4
				if finish > int(directoryOffset)-12 {
					return nil, ErrOfficePackage
				}
				descriptor = content[finish:]
			}
			if binary.LittleEndian.Uint32(descriptor) != file.CRC32 || uint64(binary.LittleEndian.Uint32(descriptor[4:])) != file.CompressedSize64 || uint64(binary.LittleEndian.Uint32(descriptor[8:])) != file.UncompressedSize64 {
				return nil, ErrOfficePackage
			}
			finish += 12
		}
		spans = append(spans, span{offset, finish})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	cursor = 0
	for _, span := range spans {
		if span.start != cursor || span.end < span.start {
			return nil, ErrOfficePackage
		}
		cursor = span.end
	}
	if cursor != int(directoryOffset) {
		return nil, ErrOfficePackage
	}
	return reader.File, nil
}

// Alternate Unicode path fields can make other ZIP readers observe a different
// name from archive/zip. Reject these aliases and ZIP64/encryption extensions;
// bounded metadata such as timestamps and UID/GID fields may remain.
func officeZIPExtra(extra []byte) bool {
	for len(extra) != 0 {
		if len(extra) < 4 {
			return false
		}
		kind := binary.LittleEndian.Uint16(extra)
		length := int(binary.LittleEndian.Uint16(extra[2:]))
		if length > len(extra)-4 {
			return false
		}
		switch kind {
		case 0x0001, 0x7075, 0x0017, 0x9901:
			return false
		}
		extra = extra[4+length:]
	}
	return true
}
