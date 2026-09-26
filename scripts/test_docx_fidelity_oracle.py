#!/usr/bin/env python3
"""Oracle self-tests on real, first-party synthetic ZIP files; no Office engine."""

import copy
import importlib.util
import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
import warnings
import zipfile
from pathlib import Path
from unittest.mock import patch
from xml.etree import ElementTree as ET

SCRIPT = Path(__file__).with_name("docx-fidelity-oracle.py")
SPEC = importlib.util.spec_from_file_location("docx_oracle", SCRIPT)
oracle = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(oracle)


class SyntheticDocxOracleTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="synthetic-docx-oracle-", dir=os.environ.get("TMPDIR"))
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        oracle.generate(self.directory)
        self.original = self.directory / "original.docx"
        self.expected = self.directory / "expected-edit.docx"
        self.parts = oracle.load_package(self.original)
        self.edit = oracle.read_edit(self.directory / "edit.json")

    def write(self, name, parts):
        path = self.directory / name
        oracle.write_package(path, parts)
        return path

    def altered_document(self, parts, change):
        result = dict(parts)
        document = oracle.parse_xml(result[oracle.DOCUMENT])
        change(document)
        result[oracle.DOCUMENT] = ET.tostring(document, encoding="utf-8", xml_declaration=True)
        return result

    def compare(self, edited=None, noop=None, edit=None, compatible=False):
        return oracle.compare(self.original, noop or self.original, edited or self.expected,
                              self.edit if edit is None else edit, compatible)

    def test_generation_is_byte_deterministic_and_explicitly_synthetic(self):
        second = self.directory / "second"
        oracle.generate(second)
        for name in ("original.docx", "expected-edit.docx", "edit.json", "manifest.json"):
            self.assertEqual((self.directory / name).read_bytes(), (second / name).read_bytes())
        manifest = json.loads((self.directory / "manifest.json").read_text())
        self.assertFalse(manifest["native_engine_run"])
        self.assertTrue(all(item["origin"] == "first-party-synthetic" for item in manifest["files"].values()))
        self.assertFalse((self.directory / "noop.docx").exists())
        self.assertFalse((self.directory / "edited.docx").exists())
        with self.assertRaises(oracle.OracleError):
            oracle.generate(self.directory)

    def test_fixture_contains_rich_runs_chinese_links_fields_tables_and_duplicates(self):
        document = oracle.parse_xml(self.parts[oracle.DOCUMENT])
        for tag in ("rPr", "pPr", "rFonts", "color", "sz", "hyperlink", "bookmarkStart", "bookmarkEnd", "fldSimple", "fldChar", "instrText", "tbl"):
            self.assertIsNotNone(document.find(".//" + oracle.w(tag)), tag)
        text = "".join(node.text or "" for node in document.iter(oracle.w("t")))
        self.assertGreaterEqual(text.count("重复文本"), 4)

    def test_explicit_synthetic_three_input_self_check_and_input_hashes(self):
        result = self.compare()
        self.assertTrue(result["ok"])
        self.assertTrue(result["noop_vs_original"]["ok"])
        self.assertTrue(result["edited_vs_noop"]["ok"])
        for origin in result["inputs"].values():
            self.assertEqual(origin["source"], "caller-supplied")
            self.assertEqual(origin["native_provenance"], "not-established")
            self.assertEqual(len(origin["sha256"]), 64)
        self.assertEqual(result["inputs"]["original"]["sha256"], result["inputs"]["noop"]["sha256"])
        self.assertNotEqual(result["inputs"]["original"]["sha256"], result["inputs"]["edited"]["sha256"])

    def test_exact_paragraph_run_range_changes_only_first_duplicate(self):
        edit = {"paragraph": 0, "run": 1, "start": 0, "end": 4, "before": "重复文本", "after": "已审核"}
        changed = oracle.apply_edit(self.parts, oracle.resolve_edit(self.parts, edit))
        edited = self.write("synthetic-first-occurrence.docx", changed)
        self.assertTrue(self.compare(edited=edited, edit=edit)["ok"])
        document = oracle.parse_xml(changed[oracle.DOCUMENT])
        second = list(document.iter(oracle.w("p")))[1]
        self.assertIn("重复文本", "".join(node.text or "" for node in second.iter(oracle.w("t"))))

    def test_cross_run_range_preserves_properties_and_unselected_suffix(self):
        document = oracle.parse_xml(oracle.load_package(self.expected)[oracle.DOCUMENT])
        paragraph = list(document.iter(oracle.w("p")))[5]
        self.assertEqual([run.find(oracle.w("t")).text for run in paragraph], ["跨运行", "精确修改", "结束"])
        self.assertTrue(all(run.find(oracle.w("rPr")) is not None for run in paragraph))

    def test_identical_text_edit_and_no_edit_both_require_no_drift(self):
        edit = dict(self.edit, after=self.edit["before"])
        self.assertTrue(self.compare(edited=self.original, edit=edit)["ok"])
        self.assertTrue(oracle.compare(self.original, self.original, self.original)["ok"])
        self.assertFalse(oracle.compare(self.original, self.original, self.expected)["ok"])

    def test_xml_indentation_and_attribute_order_do_not_count_as_drift(self):
        def reformat(document):
            for node in document.iter():
                attributes = list(node.attrib.items())
                node.attrib.clear()
                node.attrib.update(reversed(attributes))
            ET.indent(document)
        formatted = self.write("synthetic-format-only.docx", self.altered_document(self.parts, reformat))
        result = self.compare(noop=formatted)
        self.assertTrue(result["ok"])
        self.assertNotEqual(result["inputs"]["original"]["sha256"], result["inputs"]["noop"]["sha256"])

    def test_style_link_field_and_unselected_corruption_probes_fail(self):
        edited_parts = oracle.load_package(self.expected)

        def remove_first(document, tag):
            child = document.find(".//" + oracle.w(tag))
            parent = next(parent for parent in document.iter() if child in list(parent))
            parent.remove(child)

        def unwrap(document, tag):
            child = document.find(".//" + oracle.w(tag))
            parent = next(parent for parent in document.iter() if child in list(parent))
            offset = list(parent).index(child)
            parent.remove(child)
            for index, nested in enumerate(child):
                parent.insert(offset + index, nested)

        def change_duplicate(document):
            paragraph = list(document.iter(oracle.w("p")))[1]
            next(node for node in paragraph.iter(oracle.w("t")) if node.text == "重复文本").text = "精确修改"

        probes = {
            "run_properties": lambda doc: remove_first(doc, "rPr"),
            "paragraph_properties": lambda doc: remove_first(doc, "pPr"),
            "color": lambda doc: doc.find(".//" + oracle.w("color")).set(oracle.w("val"), "FFFFFF"),
            "font": lambda doc: doc.find(".//" + oracle.w("rFonts")).set(oracle.w("eastAsia"), "Other"),
            "size": lambda doc: doc.find(".//" + oracle.w("sz")).set(oracle.w("val"), "12"),
            "hyperlink": lambda doc: unwrap(doc, "hyperlink"),
            "bookmark": lambda doc: doc.find(".//" + oracle.w("bookmarkStart")).set(oracle.w("name"), "Wrong"),
            "field_instruction": lambda doc: setattr(doc.find(".//" + oracle.w("instrText")), "text", " REF Wrong "),
            "field_marker": lambda doc: remove_first(doc, "fldChar"),
            "simple_field": lambda doc: unwrap(doc, "fldSimple"),
            "table": lambda doc: setattr(doc.find(".//" + oracle.w("tbl") + "//" + oracle.w("t")), "text", "Changed"),
            "table_width": lambda doc: doc.find(".//" + oracle.w("tblW")).set(oracle.w("w"), "100"),
            "table_grid": lambda doc: doc.find(".//" + oracle.w("gridCol")).set(oracle.w("w"), "100"),
            "second_occurrence": change_duplicate,
        }
        for name, change in probes.items():
            with self.subTest(probe=name):
                file = self.write("synthetic-corrupt-" + name + ".docx", self.altered_document(edited_parts, change))
                result = self.compare(edited=file)
                self.assertTrue(result["noop_vs_original"]["ok"])
                self.assertFalse(result["edited_vs_noop"]["ok"])
                self.assertFalse(result["ok"])

    def test_noop_loss_cannot_be_hidden_by_an_equally_damaged_edit(self):
        damaged = self.altered_document(self.parts, lambda doc: doc.find(".//" + oracle.w("color")).set(oracle.w("val"), "FFFFFF"))
        noop = self.write("synthetic-damaged-noop.docx", damaged)
        edited = self.write("synthetic-damaged-edited.docx", oracle.apply_edit(damaged, self.edit))
        result = self.compare(edited=edited, noop=noop)
        self.assertFalse(result["noop_vs_original"]["ok"])
        self.assertTrue(result["edited_vs_noop"]["ok"])
        self.assertFalse(result["ok"])

    def test_noop_selected_text_drift_is_reported_without_reanchoring(self):
        def change(document):
            list(document.iter(oracle.w("p")))[5][1][-1].text = "别的"
        noop = self.write("synthetic-text-drift-noop.docx", self.altered_document(self.parts, change))
        result = self.compare(noop=noop)
        self.assertFalse(result["noop_vs_original"]["ok"])
        self.assertFalse(result["edited_vs_noop"]["ok"])

    def test_equivalent_splits_and_merges_require_explicit_selected_paragraph_opt_in(self):
        def split(document):
            paragraph = list(document.iter(oracle.w("p")))[5]
            run = paragraph[1]
            other = copy.deepcopy(run)
            run[-1].text, other[-1].text = run[-1].text[:1], run[-1].text[1:]
            paragraph.insert(2, other)
        split_parts = self.altered_document(oracle.load_package(self.expected), split)
        split_file = self.write("synthetic-split-edit.docx", split_parts)
        self.assertFalse(self.compare(edited=split_file)["ok"])
        self.assertTrue(self.compare(edited=split_file, compatible=True)["ok"])
        def merge(document):
            oracle.merge_equivalent_runs(list(document.iter(oracle.w("p")))[5])
        merged_noop = self.write("synthetic-merged-noop.docx", self.altered_document(self.parts, merge))
        self.assertTrue(self.compare(edited=split_file, noop=merged_noop, compatible=True)["ok"])

    def test_split_compatibility_preserves_run_properties_and_scope(self):
        def corrupt(document):
            paragraph = list(document.iter(oracle.w("p")))[5]
            paragraph[1].find(oracle.w("rPr")).remove(paragraph[1].find(".//" + oracle.w("color")))
        corrupt_file = self.write("synthetic-split-style-loss.docx", self.altered_document(oracle.load_package(self.expected), corrupt))
        self.assertFalse(self.compare(edited=corrupt_file, compatible=True)["ok"])
        def split_elsewhere(document):
            paragraph = list(document.iter(oracle.w("p")))[1]
            other = copy.deepcopy(paragraph[1])
            paragraph[1][-1].text, other[-1].text = "重", "复文本"
            paragraph.insert(2, other)
        outside = self.write("synthetic-unselected-split.docx", self.altered_document(oracle.load_package(self.expected), split_elsewhere))
        self.assertFalse(self.compare(edited=outside, compatible=True)["ok"])

    def test_compatibility_cannot_cross_bookmarks_links_or_fields(self):
        for index in (2, 3, 4):
            with self.subTest(paragraph=index):
                document = oracle.parse_xml(self.parts[oracle.DOCUMENT])
                with self.assertRaises(oracle.OracleError):
                    oracle.merge_equivalent_runs(list(document.iter(oracle.w("p")))[index])

    def test_noop_compatibility_failure_still_reports_both_comparisons(self):
        def add_boundary(document):
            paragraph = list(document.iter(oracle.w("p")))[5]
            paragraph.insert(0, ET.Element(oracle.w("bookmarkStart"), {oracle.w("id"): "9", oracle.w("name"): "Added"}))
            paragraph.append(ET.Element(oracle.w("bookmarkEnd"), {oracle.w("id"): "9"}))
        noop = self.write("synthetic-noop-added-boundary.docx", self.altered_document(self.parts, add_boundary))
        result = self.compare(noop=noop, compatible=True)
        self.assertFalse(result["ok"])
        self.assertFalse(result["noop_vs_original"]["ok"])
        self.assertIn("edited_vs_noop", result)

    def test_non_document_parts_and_binary_parts_are_compared(self):
        for name, body in (("word/styles.xml", b'<w:styles xmlns:w="' + oracle.W.encode() + b'"/>'), ("word/media/synthetic.bin", b"new bytes")):
            with self.subTest(part=name):
                changed = dict(oracle.load_package(self.expected))
                changed[name] = body
                if name.endswith(".bin"):
                    types = oracle.parse_xml(changed["[Content_Types].xml"])
                    ET.SubElement(types, "{" + oracle.CT + "}Default", {"Extension": "bin", "ContentType": "application/octet-stream"})
                    changed["[Content_Types].xml"] = ET.tostring(types)
                edited = self.write("synthetic-part-" + str(len(list(self.directory.iterdir()))) + ".docx", changed)
                self.assertFalse(self.compare(edited=edited)["ok"])

    def test_edit_schema_rejects_wrong_text_ranges_unknown_fields_and_types(self):
        for mutation in ({"paragraph": 100}, {"run": 100}, {"start": -1}, {"end": 100}, {"start": True},
                         {"after": None}, {"before": "wrong"}, {"unexpected": 1}, {"end": 3}, {"after": "\x00"}):
            with self.subTest(mutation=mutation):
                with self.assertRaises(oracle.OracleError):
                    oracle.resolve_edit(self.parts, dict(self.edit, **mutation))
        duplicate = self.directory / "synthetic-duplicate.json"
        duplicate.write_text('{"paragraph":0,"paragraph":1}', encoding="utf-8")
        with self.assertRaises(oracle.OracleError):
            oracle.read_edit(duplicate)

    def test_external_missing_and_traversing_relationships_are_rejected(self):
        for target, mode in (("https://example.invalid/file", "External"), ("../outside.xml", "Internal"),
                             ("/word/document.xml", "Internal"), ("word/missing.xml", "Internal")):
            with self.subTest(target=target):
                parts = dict(self.parts)
                root = oracle.parse_xml(parts["_rels/.rels"])
                root[0].set("Target", target)
                root[0].set("TargetMode", mode)
                parts["_rels/.rels"] = ET.tostring(root)
                path = self.write("synthetic-relationship-" + str(len(list(self.directory.iterdir()))) + ".docx", parts)
                with self.assertRaises(oracle.OracleError):
                    oracle.load_package(path)

    def test_zip_paths_and_case_aliases_are_rejected(self):
        for name in ("../escape", "/absolute", "C:/drive", "word/../escape", "word\\escape", "word//escape", "word/%2e%2e/escape", "WORD/DOCUMENT.XML"):
            with self.subTest(name=name):
                parts = dict(self.parts)
                parts[name] = b"synthetic"
                path = self.write("synthetic-path-" + str(len(list(self.directory.iterdir()))) + ".docx", parts)
                with self.assertRaises(oracle.OracleError):
                    oracle.load_package(path)

    def test_duplicate_zip_entries_and_symlinks_are_rejected(self):
        duplicate = self.directory / "synthetic-duplicate.docx"
        with warnings.catch_warnings():
            warnings.simplefilter("ignore", UserWarning)
            with zipfile.ZipFile(duplicate, "w") as archive:
                for name, body in self.parts.items():
                    archive.writestr(name, body)
                archive.writestr(oracle.DOCUMENT, self.parts[oracle.DOCUMENT])
        with self.assertRaises(oracle.OracleError):
            oracle.load_package(duplicate)
        symlink = self.directory / "synthetic-symlink.docx"
        with zipfile.ZipFile(symlink, "w") as archive:
            for name, body in self.parts.items():
                archive.writestr(name, body)
            info = zipfile.ZipInfo("word/link")
            info.create_system = 3
            info.external_attr = (stat.S_IFLNK | 0o777) << 16
            archive.writestr(info, "document.xml")
        with self.assertRaises(oracle.OracleError):
            oracle.load_package(symlink)

    def test_dtd_entities_utf16_and_malformed_xml_are_rejected(self):
        bodies = (b'<!DOCTYPE x [<!ENTITY value "expanded">]><x>&value;</x>',
                  '<!DOCTYPE x [<!ENTITY value "expanded">]><x/>'.encode("utf-16"), b"<unclosed>")
        for index, body in enumerate(bodies):
            parts = dict(self.parts)
            parts[oracle.DOCUMENT] = body
            path = self.write("synthetic-xml-" + str(index) + ".docx", parts)
            with self.assertRaises(oracle.OracleError):
                oracle.load_package(path)

    def test_xml_content_types_cannot_hide_entities_or_relationships_in_binary_names(self):
        for index, body in enumerate((b'<!DOCTYPE x [<!ENTITY value "expanded">]><x/>',
                                      ('<Relationships xmlns="' + oracle.REL + '"/>').encode())):
            parts = dict(self.parts)
            types = oracle.parse_xml(parts["[Content_Types].xml"])
            ET.SubElement(types, "{" + oracle.CT + "}Override", {"PartName": "/word/disguised.bin", "ContentType": "application/xml"})
            parts["[Content_Types].xml"] = ET.tostring(types)
            parts["word/disguised.bin"] = body
            path = self.write("synthetic-disguised-" + str(index) + ".docx", parts)
            with self.assertRaises(oracle.OracleError):
                oracle.load_package(path)

    def test_zip_and_xml_resource_budgets_are_enforced(self):
        for limit, value in (("MAX_ARCHIVE", 10), ("MAX_PART", 10), ("MAX_TOTAL", 100), ("MAX_ENTRIES", 2), ("MAX_NODES", 10), ("MAX_DEPTH", 2)):
            with self.subTest(limit=limit), patch.object(oracle, limit, value):
                with self.assertRaises(oracle.OracleError):
                    oracle.load_package(self.original)
        bomb = self.directory / "synthetic-compressed-bomb.docx"
        with zipfile.ZipFile(bomb, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
            for name, body in self.parts.items():
                archive.writestr(name, body)
            archive.writestr("word/bomb.bin", b"a" * (1 << 20))
        with self.assertRaises(oracle.OracleError):
            oracle.load_package(bomb)

    def test_crc_corruption_is_rejected(self):
        raw = bytearray(self.original.read_bytes())
        with zipfile.ZipFile(self.original) as archive:
            entry = archive.getinfo(oracle.DOCUMENT)
            offset = entry.header_offset + 30 + len(entry.filename.encode()) + len(entry.extra)
        raw[offset] ^= 1
        broken = self.directory / "synthetic-bad-crc.docx"
        broken.write_bytes(raw)
        with self.assertRaises(oracle.OracleError):
            oracle.load_package(broken)

    def test_cli_exit_codes_distinguish_match_drift_and_unsafe_input(self):
        command = [sys.executable, str(SCRIPT), "compare", "--original", str(self.original),
                   "--noop", str(self.original), "--edited", str(self.expected)]
        good = subprocess.run(command + ["--edit-json", str(self.directory / "edit.json")], capture_output=True, text=True, check=False)
        self.assertEqual(good.returncode, 0, good.stdout + good.stderr)
        self.assertTrue(json.loads(good.stdout)["ok"])
        drift = subprocess.run(command, capture_output=True, text=True, check=False)
        self.assertEqual(drift.returncode, 1, drift.stdout + drift.stderr)
        bad = self.directory / "synthetic-invalid.docx"
        bad.write_bytes(b"not a ZIP")
        unsafe = subprocess.run(command[:-1] + [str(bad)], capture_output=True, text=True, check=False)
        self.assertEqual(unsafe.returncode, 2, unsafe.stdout + unsafe.stderr)
        self.assertFalse(json.loads(unsafe.stdout)["ok"])


if __name__ == "__main__":
    unittest.main()
