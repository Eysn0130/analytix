package artifactgeneration

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareV1ProducesClosedCanonicalContracts(t *testing.T) {
	prepared, err := PrepareV1([]SourceFileV1{
		regularSource("tables/rows.json", "rows"),
		regularSource("report.txt", "report"),
	})
	if err != nil || prepared.Validate() != nil {
		t.Fatalf("prepare failed: err=%v validate=%v", err, prepared.Validate())
	}
	inventory, err := ParseInventoryV1(prepared.InventoryBytes())
	if err != nil || inventory.Files[0].Path != "report.txt" || inventory.Files[1].Path != "tables/rows.json" {
		t.Fatalf("inventory is not canonical and sorted: inventory=%#v err=%v", inventory, err)
	}
	receipt, err := ParseReceiptV1(prepared.ReceiptBytes(), prepared.InventoryBytes())
	if err != nil || receipt.GenerationID != inventory.GenerationID || receipt.InventoryDigest != DigestBytesV1(prepared.InventoryBytes()) {
		t.Fatalf("receipt does not bind inventory: receipt=%#v err=%v", receipt, err)
	}

	files := prepared.Files()
	files[0].Body[0] = 'X'
	if bytes.Equal(files[0].Body, prepared.Files()[0].Body) {
		t.Fatal("prepared payload was mutable through a returned slice")
	}
}

func TestCanonicalContractsRejectUnknownDuplicateAndNonCanonicalFields(t *testing.T) {
	prepared, err := PrepareV1([]SourceFileV1{regularSource("payload.bin", "payload")})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"unknown": func(body []byte) []byte {
			return append(body[:len(body)-1], []byte(`,"unknown":true}`)...)
		},
		"duplicate": func(body []byte) []byte {
			return []byte(strings.Replace(string(body), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1))
		},
		"whitespace": func(body []byte) []byte { return append([]byte(" "), body...) },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseInventoryV1(mutate(prepared.InventoryBytes())); err == nil {
				t.Fatal("ambiguous inventory passed")
			}
		})
	}

	var receipt map[string]any
	if err := json.Unmarshal(prepared.ReceiptBytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	receipt["inventoryDigest"] = strings.Repeat("0", 64)
	tampered, _ := json.Marshal(receipt)
	if _, err := ParseReceiptV1(tampered, prepared.InventoryBytes()); err == nil {
		t.Fatal("receipt with a mismatched inventory digest passed")
	}
}

func TestPrepareV1RejectsUnsafeDuplicateAndReservedPaths(t *testing.T) {
	for _, files := range [][]SourceFileV1{
		{regularSource("../escape", "x")},
		{regularSource(".hidden", "x")},
		{regularSource("a\\b", "x")},
		{regularSource(InventoryFileNameV1, "x")},
		{regularSource("same", "a"), regularSource("same", "b")},
		{regularSource("parent", "a"), regularSource("parent/child", "b")},
	} {
		if _, err := PrepareV1(files); err == nil {
			t.Fatalf("unsafe source paths passed: %#v", files)
		}
	}
}

func TestPrepareV1RequiresAndBindsExplicitTypeAndMode(t *testing.T) {
	for _, source := range []SourceFileV1{
		{Path: "missing.bin", Body: []byte("x")},
		{Path: "wrong-mode.bin", Type: FileTypeRegularV1, Mode: NativeExecutableModeV1, Body: []byte("x")},
		{Path: "unknown.bin", Type: "unknown", Mode: RegularFileModeV1, Body: []byte("x")},
	} {
		if _, err := PrepareV1([]SourceFileV1{source}); err == nil {
			t.Fatalf("invalid type/mode passed: %#v", source)
		}
	}
	regular, err := PrepareV1([]SourceFileV1{regularSource("tool", "same")})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := PrepareV1([]SourceFileV1{{Path: "tool", Type: FileTypeNativeExecutableV1, Mode: NativeExecutableModeV1, Body: []byte("same")}})
	if err != nil {
		t.Fatal(err)
	}
	if regular.Receipt().GenerationID == executable.Receipt().GenerationID || executable.Inventory().Files[0].Mode != NativeExecutableModeV1 {
		t.Fatal("type/mode were not bound into the generation authority")
	}
}

func regularSource(path, body string) SourceFileV1 {
	return SourceFileV1{Path: path, Type: FileTypeRegularV1, Mode: RegularFileModeV1, Body: []byte(body)}
}
