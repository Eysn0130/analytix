#!/usr/bin/env python3
"""First-party synthetic DOCX fixtures and a deliberately conservative oracle.

This checks package/XML semantics, not native rendering or Office fidelity.
Only Python's standard library is used; packages are never extracted.
"""

import argparse
import hashlib
import io
import json
import posixpath
import re
import stat
import sys
import zipfile
import zlib
from pathlib import Path
from xml.etree import ElementTree as ET

W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
REL = "http://schemas.openxmlformats.org/package/2006/relationships"
OFFICE_REL = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
CT = "http://schemas.openxmlformats.org/package/2006/content-types"
XML_SPACE = "{http://www.w3.org/XML/1998/namespace}space"
MAX_ARCHIVE = 8 << 20
MAX_PART = 2 << 20
MAX_TOTAL = 16 << 20
MAX_ENTRIES = 64
MAX_NODES = 50000
MAX_DEPTH = 80
MAX_EDIT = 65536
DOCUMENT = "word/document.xml"
ET.register_namespace("w", W)


class LoadedPackage(dict):
    def __init__(self, parts, archive_bytes):
        super().__init__(parts)
        self.origin = {"source": "caller-supplied", "sha256": hashlib.sha256(archive_bytes).hexdigest(),
                       "bytes": len(archive_bytes), "native_provenance": "not-established"}


class OracleError(ValueError):
    """Invalid or unsupported input; never include document text in errors."""


def w(local):
    return "{" + W + "}" + local


def safe_part_name(name):
    return (isinstance(name, str) and len(name) <= 240
            and re.fullmatch(r"[A-Za-z0-9_./\[\]-]+", name) is not None
            and all(piece not in ("", ".", "..") for piece in name.split("/")))


def parse_xml(data):
    try:
        source = data.decode("utf-8-sig")
    except UnicodeError as exc:
        raise OracleError("XML must be UTF-8") from exc
    if "\x00" in source or re.search(r"<!\s*(DOCTYPE|ENTITY)\b", source, re.I):
        raise OracleError("DTD and entity declarations are forbidden")
    encoding = re.match(r"\s*<\?xml[^?]*encoding\s*=\s*['\"]([^'\"]+)", source, re.I)
    if encoding and encoding.group(1).lower() not in ("utf-8", "utf8"):
        raise OracleError("XML declaration must specify UTF-8")
    try:
        root = ET.fromstring(source, parser=ET.XMLParser(
            target=ET.TreeBuilder(insert_comments=True, insert_pis=True)))
    except ET.ParseError as exc:
        raise OracleError("malformed XML") from exc
    stack, count = [(root, 1)], 0
    while stack:
        node, depth = stack.pop()
        count += 1
        if count > MAX_NODES or depth > MAX_DEPTH:
            raise OracleError("XML structural budget exceeded")
        stack.extend((child, depth + 1) for child in node)
    return root


def xml_part_names(parts):
    """Content types also identify XML hidden behind a non-.xml extension."""
    root = parse_xml(parts["[Content_Types].xml"])
    if root.tag != "{" + CT + "}Types":
        raise OracleError("invalid content types root")
    defaults, overrides = {}, {}
    for item in root:
        mime = item.get("ContentType", "")
        if not re.fullmatch(r"[A-Za-z0-9.+-]+/[A-Za-z0-9.+-]+", mime):
            raise OracleError("invalid part content type")
        if item.tag == "{" + CT + "}Default":
            extension = item.get("Extension", "")
            if not re.fullmatch(r"[A-Za-z0-9]+", extension) or extension.lower() in defaults:
                raise OracleError("invalid or duplicate default content type")
            defaults[extension.lower()] = mime
        elif item.tag == "{" + CT + "}Override":
            name = item.get("PartName", "")
            if not name.startswith("/") or not safe_part_name(name[1:]) or name[1:] not in parts or name[1:] in overrides:
                raise OracleError("invalid or duplicate part content type")
            overrides[name[1:]] = mime
        else:
            raise OracleError("unsupported content type declaration")
    xml_names = {"[Content_Types].xml"}
    for name in parts:
        if name == "[Content_Types].xml":
            continue
        mime = overrides.get(name, defaults.get(name.rsplit(".", 1)[-1].lower(), ""))
        if not mime:
            raise OracleError("part content type is missing")
        if name.endswith((".xml", ".rels")) or mime.endswith("+xml") or mime in ("application/xml", "text/xml"):
            xml_names.add(name)
    return xml_names


def load_package(path):
    path = Path(path)
    if not path.is_file() or path.stat().st_size > MAX_ARCHIVE:
        raise OracleError("archive is missing or exceeds byte budget")
    with path.open("rb") as stream:
        archive_bytes = stream.read(MAX_ARCHIVE + 1)
    if len(archive_bytes) > MAX_ARCHIVE:
        raise OracleError("archive exceeds byte budget")
    parts = {}
    names = set()
    try:
        with zipfile.ZipFile(io.BytesIO(archive_bytes)) as archive:
            entries = archive.infolist()
            if len(entries) > MAX_ENTRIES:
                raise OracleError("ZIP entry budget exceeded")
            total = 0
            for entry in entries:
                mode = entry.external_attr >> 16
                if (not safe_part_name(entry.filename) or entry.filename != entry.orig_filename
                        or entry.filename.casefold() in names or entry.is_dir()
                        or stat.S_IFMT(mode) not in (0, stat.S_IFREG)):
                    raise OracleError("unsafe or duplicate ZIP entry")
                if entry.flag_bits & 1 or entry.compress_type not in (zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED):
                    raise OracleError("unsupported ZIP encoding")
                total += entry.file_size
                if (entry.file_size > MAX_PART or total > MAX_TOTAL
                        or entry.file_size > max(1, entry.compress_size) * 1000):
                    raise OracleError("ZIP expansion budget exceeded")
                with archive.open(entry) as stream:
                    body = stream.read(MAX_PART + 1)
                if len(body) != entry.file_size or len(body) > MAX_PART:
                    raise OracleError("ZIP part size mismatch")
                parts[entry.filename] = body
                names.add(entry.filename.casefold())
    except (zipfile.BadZipFile, NotImplementedError, RuntimeError, EOFError, zlib.error) as exc:
        raise OracleError("invalid ZIP package") from exc
    for required in ("[Content_Types].xml", "_rels/.rels", DOCUMENT):
        if required not in parts:
            raise OracleError("required DOCX part is missing")
    trees, node_budget = {}, MAX_NODES
    xml_names = xml_part_names(parts)
    for name, body in parts.items():
        if name in xml_names:
            tree = parse_xml(body)
            node_budget -= sum(1 for _ in tree.iter())
            if node_budget < 0:
                raise OracleError("package XML node budget exceeded")
            trees[name] = tree
    document = trees[DOCUMENT]
    if document.tag != w("document") or len(document.findall(w("body"))) != 1:
        raise OracleError("invalid document root")
    for name, root in trees.items():
        if not name.endswith(".rels") and root.tag != "{" + REL + "}Relationships":
            continue
        if root.tag != "{" + REL + "}Relationships":
            raise OracleError("invalid relationships root")
        if name == "_rels/.rels":
            base = ""
        else:
            parent, leaf = posixpath.split(name)
            if not name.endswith(".rels") or posixpath.basename(parent) != "_rels":
                raise OracleError("invalid relationships part path")
            base = posixpath.dirname(parent)
            if posixpath.join(base, leaf[:-5]) not in parts:
                raise OracleError("orphan relationships part")
        ids = set()
        for relation in root:
            target, identity = relation.get("Target", ""), relation.get("Id", "")
            if (relation.tag != "{" + REL + "}Relationship" or not identity or identity in ids
                    or relation.get("TargetMode", "Internal") != "Internal"
                    or not safe_part_name(target) or not relation.get("Type")):
                raise OracleError("external or malformed relationship")
            ids.add(identity)
            if posixpath.join(base, target) not in parts:
                raise OracleError("relationship target is missing")
    return LoadedPackage(parts, archive_bytes)


def canonical(node):
    tag = node.tag if isinstance(node.tag, str) else "#comment" if node.tag is ET.Comment else "#pi"
    text = node.text or ""
    if len(node) and not text.strip():
        text = ""
    return (tag, tuple(sorted(node.attrib.items())), text,
            tuple((canonical(child), child.tail if child.tail and child.tail.strip() else "") for child in node))


def plain_text_run(run):
    children = list(run)
    return (run.tag == w("r") and len(children) in (1, 2)
            and children[-1].tag == w("t") and not list(children[-1])
            and not (run.text or "").strip() and not (run.tail or "").strip()
            and all(not (child.tail or "").strip() for child in children)
            and (len(children) == 1 or children[0].tag == w("rPr")))


def merge_equivalent_runs(paragraph):
    # No field state, bookmark boundary, hyperlink ancestry or other non-text
    # structure may be crossed, even if its displayed string looks identical.
    if any(child.tag != w("pPr") and not plain_text_run(child) for child in paragraph):
        raise OracleError("run compatibility requires a plain-text paragraph")
    previous = None
    for run in list(paragraph):
        if run.tag != w("r"):
            previous = None
            continue
        properties = run.find(w("rPr"))
        key = (tuple(sorted(run.attrib.items())), canonical(properties) if properties is not None else None,
               tuple(sorted(run[-1].attrib.items())))
        if previous is not None and previous[0] == key:
            previous[1][-1].text = (previous[1][-1].text or "") + (run[-1].text or "")
            paragraph.remove(run)
        else:
            previous = (key, run)


def package_semantics(parts, compatible_paragraph=None):
    result = {}
    xml_names = xml_part_names(parts)
    for name, body in sorted(parts.items()):
        if name in xml_names:
            tree = parse_xml(body)
            if name == DOCUMENT and compatible_paragraph is not None:
                paragraphs = list(tree.iter(w("p")))
                if compatible_paragraph >= len(paragraphs):
                    raise OracleError("selected paragraph is missing")
                merge_equivalent_runs(paragraphs[compatible_paragraph])
            result[name] = canonical(tree)
        else:
            result[name] = ("binary-sha256", hashlib.sha256(body).hexdigest())
    return result


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise OracleError("duplicate edit JSON field")
        result[key] = value
    return result


def read_edit(path):
    with open(path, "rb") as stream:
        data = stream.read(MAX_EDIT * 8 + 1)
    if len(data) > MAX_EDIT * 8:
        raise OracleError("edit JSON exceeds byte budget")
    try:
        value = json.loads(data, object_pairs_hook=unique_object)
    except (ValueError, UnicodeError) as exc:
        raise OracleError("invalid edit JSON") from exc
    validate_edit(value)
    return value


def validate_edit(edit):
    required = {"paragraph", "start", "end", "before", "after"}
    if not isinstance(edit, dict) or not required <= edit.keys() or edit.keys() - required - {"run"}:
        raise OracleError("invalid edit fields")
    for field in ("paragraph", "start", "end", "run"):
        if field in edit and (type(edit[field]) is not int or edit[field] < 0):
            raise OracleError("edit indexes must be nonnegative integers")
    if edit["start"] >= edit["end"]:
        raise OracleError("expected replacement requires a nonempty source range")
    for field in ("before", "after"):
        value = edit[field]
        if (not isinstance(value, str) or len(value) > MAX_EDIT
                or any(not (ord(char) in (9, 10, 13) or 0x20 <= ord(char) <= 0xD7FF
                            or 0xE000 <= ord(char) <= 0xFFFD or 0x10000 <= ord(char) <= 0x10FFFF) for char in value)):
            raise OracleError("invalid expected edit text")


def resolve_edit(parts, edit):
    validate_edit(edit)
    tree = parse_xml(parts[DOCUMENT])
    paragraphs = list(tree.iter(w("p")))
    if edit["paragraph"] >= len(paragraphs):
        raise OracleError("selected paragraph is missing")
    paragraph = paragraphs[edit["paragraph"]]
    normalized = {key: value for key, value in edit.items() if key != "run"}
    if "run" in edit:
        runs = list(paragraph.iter(w("r")))
        if edit["run"] >= len(runs):
            raise OracleError("selected run is missing")
        run = runs[edit["run"]]
        text = "".join(node.text or "" for node in run.iter(w("t")))
        if not 0 <= edit["start"] <= edit["end"] <= len(text):
            raise OracleError("run range is out of bounds")
        offset = sum(len(node.text or "") for item in runs[:edit["run"]] for node in item.iter(w("t")))
        normalized["start"] += offset
        normalized["end"] += offset
    apply_edit(parts, normalized)  # Validate the selected text and structural boundaries now.
    return normalized


def apply_edit(parts, edit):
    tree = parse_xml(parts[DOCUMENT])
    paragraphs = list(tree.iter(w("p")))
    if edit["paragraph"] >= len(paragraphs):
        raise OracleError("selected paragraph is missing")
    paragraph = paragraphs[edit["paragraph"]]
    nodes = list(paragraph.iter(w("t")))
    text = "".join(node.text or "" for node in nodes)
    start, end = edit["start"], edit["end"]
    if not 0 <= start <= end <= len(text) or text[start:end] != edit["before"]:
        raise OracleError("selected source text does not match the expected range")
    parents = {child: parent for parent in paragraph.iter() for child in parent}
    selected, offset = [], 0
    for node in nodes:
        length = len(node.text or "")
        if offset < end and offset + length > start:
            selected.append((node, offset))
        offset += length
    if not selected:
        raise OracleError("selected range has no text node")
    runs = []
    for node, _ in selected:
        run = parents[node]
        if parents.get(run) is not paragraph or not plain_text_run(run):
            raise OracleError("selected range must use direct plain-text runs")
        runs.append(run)
    children = list(paragraph)
    between = children[children.index(runs[0]):children.index(runs[-1]) + 1]
    if any(not plain_text_run(child) for child in between):
        raise OracleError("selected range crosses a structural boundary")
    if edit["before"] == edit["after"]:
        return dict(parts)
    for index, (node, offset) in enumerate(selected):
        original = node.text or ""
        left, right = max(0, start - offset), min(len(original), end - offset)
        node.text = original[:left] + (edit["after"] if index == 0 else "") + original[right:]
    result = dict(parts)
    result[DOCUMENT] = ET.tostring(tree, encoding="utf-8", xml_declaration=True)
    return result


def semantic_difference(expected, actual):
    changed = sorted(name for name in expected.keys() | actual.keys() if expected.get(name) != actual.get(name))
    digest = lambda value: hashlib.sha256(json.dumps(value, ensure_ascii=False, sort_keys=True).encode()).hexdigest()
    return {"ok": not changed, "changed_parts": changed,
            "expected_sha256": digest(expected), "actual_sha256": digest(actual)}


def compare(original, noop, edited, edit=None, allow_equivalent_run_splits=False):
    original, noop, edited = (load_package(path) for path in (original, noop, edited))
    edit = resolve_edit(original, edit) if edit is not None else None
    if allow_equivalent_run_splits and edit is None:
        raise OracleError("run compatibility requires an explicit selected paragraph")
    compatible = edit["paragraph"] if allow_equivalent_run_splits else None
    original_semantics = package_semantics(original, compatible)
    try:
        baseline = semantic_difference(original_semantics, package_semantics(noop, compatible))
    except OracleError as exc:
        # An engine-added structural boundary is a failed no-op comparison,
        # not a reason to omit the baseline result or silently broaden merging.
        baseline = semantic_difference(package_semantics(original), package_semantics(noop))
        baseline.update(ok=False, error=str(exc))
    try:
        expected = apply_edit(noop, edit) if edit is not None else noop
        change = semantic_difference(package_semantics(expected, compatible), package_semantics(edited, compatible))
    except OracleError as exc:
        change = {"ok": False, "error": str(exc)}
    return {"ok": baseline["ok"] and change["ok"],
            "inputs": {"original": original.origin, "noop": noop.origin, "edited": edited.origin},
            "noop_vs_original": baseline,
            "edited_vs_noop": change, "run_compatibility": allow_equivalent_run_splits}


def synthetic_parts():
    document = ET.Element(w("document"))
    body = ET.SubElement(document, w("body"))

    def run(parent, text, color="333333", size="24", bold=False):
        node = ET.SubElement(parent, w("r"))
        properties = ET.SubElement(node, w("rPr"))
        ET.SubElement(properties, w("rFonts"), {w("ascii"): "Calibri", w("eastAsia"): "宋体"})
        if bold:
            ET.SubElement(properties, w("b"))
        ET.SubElement(properties, w("color"), {w("val"): color})
        ET.SubElement(properties, w("sz"), {w("val"): size})
        ET.SubElement(node, w("t"), {XML_SPACE: "preserve"}).text = text
        return node

    for ordinal in ("第一", "第二"):
        paragraph = ET.SubElement(body, w("p"))
        properties = ET.SubElement(paragraph, w("pPr"))
        ET.SubElement(properties, w("spacing"), {w("after"): "120"})
        run(paragraph, ordinal + "处相同文本：", bold=True)
        run(paragraph, "重复文本", color="0070C0", size="28")
        run(paragraph, "，不得误改。")
    paragraph = ET.SubElement(body, w("p"))
    ET.SubElement(paragraph, w("bookmarkStart"), {w("id"): "7", w("name"): "TargetOne"})
    run(paragraph, "内部书签目标。")
    ET.SubElement(paragraph, w("bookmarkEnd"), {w("id"): "7"})
    paragraph = ET.SubElement(body, w("p"))
    link = ET.SubElement(paragraph, w("hyperlink"), {w("anchor"): "TargetOne", w("history"): "1"})
    linked = run(link, "跳到目标", color="0563C1")
    ET.SubElement(linked.find(w("rPr")), w("u"), {w("val"): "single"})
    paragraph = ET.SubElement(body, w("p"))
    simple = ET.SubElement(paragraph, w("fldSimple"), {w("instr"): 'DATE \\@ "yyyy-MM-dd"'})
    run(simple, "2026-09-20")
    for field_type in ("begin", "separate", "end"):
        node = ET.SubElement(paragraph, w("r"))
        ET.SubElement(node, w("fldChar"), {w("fldCharType"): field_type})
        if field_type == "begin":
            node = ET.SubElement(paragraph, w("r"))
            ET.SubElement(node, w("instrText"), {XML_SPACE: "preserve"}).text = " REF TargetOne \\h "
        if field_type == "separate":
            run(paragraph, "内部书签目标。", color="880000")
    paragraph = ET.SubElement(body, w("p"))
    for text in ("跨运行", "重复", "文本结束"):
        run(paragraph, text, color="AA3300")
    table = ET.SubElement(body, w("tbl"))
    properties = ET.SubElement(table, w("tblPr"))
    ET.SubElement(properties, w("tblW"), {w("w"): "4800", w("type"): "dxa"})
    grid = ET.SubElement(table, w("tblGrid"))
    for _ in range(2):
        ET.SubElement(grid, w("gridCol"), {w("w"): "2400"})
    for cells in (("栏目", "中文值"), ("重复文本", "42")):
        row = ET.SubElement(table, w("tr"))
        for value in cells:
            cell = ET.SubElement(row, w("tc"))
            run(ET.SubElement(cell, w("p")), value)
    ET.SubElement(body, w("sectPr"))
    content_types = ET.Element("{" + CT + "}Types")
    for extension, mime in (("rels", "application/vnd.openxmlformats-package.relationships+xml"), ("xml", "application/xml")):
        ET.SubElement(content_types, "{" + CT + "}Default", {"Extension": extension, "ContentType": mime})
    for part, mime in ((DOCUMENT, "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"),
                       ("word/styles.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml")):
        ET.SubElement(content_types, "{" + CT + "}Override", {"PartName": "/" + part, "ContentType": mime})
    relationships = ET.Element("{" + REL + "}Relationships")
    ET.SubElement(relationships, "{" + REL + "}Relationship", {"Id": "rId1", "Type": OFFICE_REL + "officeDocument", "Target": DOCUMENT})
    document_relationships = ET.Element("{" + REL + "}Relationships")
    ET.SubElement(document_relationships, "{" + REL + "}Relationship", {"Id": "rId1", "Type": OFFICE_REL + "styles", "Target": "styles.xml"})
    styles = ET.Element(w("styles"))
    style = ET.SubElement(styles, w("style"), {w("type"): "paragraph", w("default"): "1", w("styleId"): "Normal"})
    ET.SubElement(style, w("name"), {w("val"): "Normal"})
    return {name: ET.tostring(tree, encoding="utf-8", xml_declaration=True) for name, tree in {
        "[Content_Types].xml": content_types, "_rels/.rels": relationships, DOCUMENT: document,
        "word/_rels/document.xml.rels": document_relationships, "word/styles.xml": styles}.items()}


def write_package(path, parts):
    with open(path, "xb") as output, zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        for name, body in sorted(parts.items()):
            info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = (stat.S_IFREG | 0o600) << 16
            archive.writestr(info, body)


def generate(directory):
    directory = Path(directory)
    names = ("original.docx", "expected-edit.docx", "edit.json", "manifest.json")
    if any((directory / name).exists() for name in names):
        raise OracleError("fixture output already exists; choose a fresh directory")
    directory.mkdir(parents=True, exist_ok=True)
    parts = synthetic_parts()
    edit = {"paragraph": 5, "start": 3, "end": 7, "before": "重复文本", "after": "精确修改"}
    write_package(directory / names[0], parts)
    write_package(directory / names[1], apply_edit(parts, resolve_edit(parts, edit)))
    with open(directory / names[2], "x", encoding="utf-8") as stream:
        json.dump(edit, stream, ensure_ascii=False, sort_keys=True, indent=2)
        stream.write("\n")
    manifest = {"kind": "analytix.synthetic-docx-fixture/v1", "native_engine_run": False,
                "files": {name: {"origin": "first-party-synthetic",
                          "role": "synthetic-input" if name == names[0] else "synthetic-expected-edit" if name == names[1] else "expected-edit-specification",
                          "sha256": hashlib.sha256((directory / name).read_bytes()).hexdigest()} for name in names[:3]}}
    with open(directory / names[3], "x", encoding="utf-8") as stream:
        json.dump(manifest, stream, ensure_ascii=False, sort_keys=True, indent=2)
        stream.write("\n")
    return {"ok": True, "output_directory": str(directory), "files": list(names),
            "scope": "synthetic fixture and oracle only", "native_engine_run": False}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    fixture = commands.add_parser("generate", help="write first-party synthetic fixtures to a fresh directory")
    fixture.add_argument("--output-dir", required=True)
    comparison = commands.add_parser("compare", help="compare original, no-op and edited package semantics")
    for name in ("original", "noop", "edited"):
        comparison.add_argument("--" + name, required=True)
    comparison.add_argument("--edit-json", help="one exact expected edit; omit for text no-op")
    comparison.add_argument("--allow-equivalent-run-splits", action="store_true")
    arguments = parser.parse_args(argv)
    try:
        if arguments.command == "generate":
            result = generate(arguments.output_dir)
        else:
            edit = read_edit(arguments.edit_json) if arguments.edit_json else None
            result = compare(arguments.original, arguments.noop, arguments.edited, edit, arguments.allow_equivalent_run_splits)
        print(json.dumps(result, ensure_ascii=False, sort_keys=True))
        return 0 if result["ok"] else 1
    except (OracleError, OSError) as exc:
        print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
        return 2


if __name__ == "__main__":
    sys.exit(main())
