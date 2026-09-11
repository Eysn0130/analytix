package filetools

import (
	"bytes"
	"testing"
)

func TestWindowReadTextViewUsesOneBasedOffsets(t *testing.T) {
	view := WindowReadTextView("a\nb\nc\n", 2, 1)
	if view.Content != "b\n\n[2 more lines in file. Use offset=3 to continue.]" {
		t.Fatalf("unexpected content: %q", view.Content)
	}
	if view.RawContent != "b" || view.StartLine != 2 || view.EndLine != 2 || view.TotalLines != 4 || !view.Truncated || view.TruncatedBy != "lines" {
		t.Fatalf("unexpected view: %#v", view)
	}
}

func TestNumberedReadTextViewUsesZeroBasedOffsetsAndLineNumbers(t *testing.T) {
	view := NumberedReadTextView("a\nb\nc\n", 1, 2)
	if view.Content != "2→b\n3→c\n" {
		t.Fatalf("unexpected content: %q", view.Content)
	}
	if view.RawContent != "b\nc" || view.StartLine != 2 || view.EndLine != 3 || view.TotalLines != 3 || view.Truncated {
		t.Fatalf("unexpected view: %#v", view)
	}
}

func TestReadTextDefaultsWindowOffsetWhenAbsent(t *testing.T) {
	view := ReadText("a\nb", 0, 1, ReadModeWindow, false)
	if view.RawContent != "a" || view.StartLine != 1 {
		t.Fatalf("unexpected default view: %#v", view)
	}
}

func TestBuildReadTextToolOutputIncludesStableFields(t *testing.T) {
	output := BuildReadTextToolOutput(ReadTextToolOutputInput{
		Path:         "/work/a.txt",
		RelativePath: "a.txt",
		Encoding:     TextEncodingUTF8,
		View:         ReadTextView{Content: "hello", RawContent: "hello", StartLine: 1, EndLine: 1, TotalLines: 2, Truncated: true, TruncatedBy: "lines"},
	})
	if output["path"] != "/work/a.txt" || output["relative_path"] != "a.txt" || output["content"] != "hello" || output["truncation_by"] != "lines" {
		t.Fatalf("read output mismatch: %#v", output)
	}
}

func TestTextCodecRoundTripsEncodings(t *testing.T) {
	for _, encoding := range []string{
		TextEncodingUTF8,
		TextEncodingUTF8BOM,
		TextEncodingUTF16LE,
		TextEncodingUTF16BE,
		TextEncodingUTF16LENoBOM,
		TextEncodingUTF16BENoBOM,
	} {
		encoded := EncodeTextBytes("hello 世界", encoding)
		decoded, gotEncoding, ok := DecodeTextBytes(encoded)
		if !ok || decoded != "hello 世界" || gotEncoding != encoding {
			t.Fatalf("roundtrip %s decoded=%q gotEncoding=%s ok=%t", encoding, decoded, gotEncoding, ok)
		}
	}
}

func TestDecodeTextBytesRejectsBinaryNUL(t *testing.T) {
	if _, _, ok := DecodeTextBytes([]byte{'a', 0, 'b'}); ok {
		t.Fatal("expected binary NUL payload to be rejected")
	}
}

func TestLooksUTF16TextDetectsNoBOM(t *testing.T) {
	encoded := EncodeTextBytes("abcdefghijklmnop", TextEncodingUTF16LENoBOM)
	if !LooksUTF16Text(encoded) {
		t.Fatal("expected UTF-16LE without BOM to be detected")
	}
	if !bytes.Contains(encoded, []byte{0}) {
		t.Fatal("test fixture should contain NUL bytes")
	}
}
