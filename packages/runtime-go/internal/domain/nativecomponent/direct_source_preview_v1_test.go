package nativecomponent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func directSourcePreviewTestArgumentsV1() DirectSourcePreviewArgumentsV1 {
	return DirectSourcePreviewArgumentsV1{
		CaseID: "case-preview", DatasetSnapshotID: "dsv2_" + strings.Repeat("1", 64),
		CaseBindingHash:                strings.Repeat("2", 64),
		ExpectedProducerContentID:      "fpc1_" + strings.Repeat("3", 64),
		ExpectedProducerManifestSHA256: strings.Repeat("4", 64),
		Fields: []DirectSourcePreviewFieldV1{
			DirectSourcePreviewFieldAccountV1,
			DirectSourcePreviewFieldAmountTextV1,
			DirectSourcePreviewFieldCounterpartyNameV1,
		},
		RowOffset: 4, RowLimit: 25, DatasetUTCOffsetMinutes: 480,
		ExpectedCurrency: "CNY", MinorUnitScale: 2,
	}
}

func directSourcePreviewTestResultWireV1(arguments DirectSourcePreviewArgumentsV1) directSourcePreviewResultWireV1 {
	rows := []directSourcePreviewRowWireV1{{
		RowIndex: arguments.RowOffset,
		Cells: []directSourcePreviewCellWireV1{
			{Field: DirectSourcePreviewFieldAccountV1, Value: "0012345678901234"},
			{Field: DirectSourcePreviewFieldAmountTextV1, Value: "123.45"},
			{Field: DirectSourcePreviewFieldCounterpartyNameV1, Value: "source-exact-name"},
		},
	}}
	wire := directSourcePreviewResultWireV1{
		SchemaVersion: 1, Purpose: DirectSourcePreviewPurposeV1,
		DatasetSnapshotID: arguments.DatasetSnapshotID,
		Fields:            append([]DirectSourcePreviewFieldV1(nil), arguments.Fields...),
		RowOffset:         arguments.RowOffset, RowLimit: arguments.RowLimit,
		Rows: rows, HasMore: false, QueryHash: strings.Repeat("5", 64),
		Currentness: DirectSourcePreviewCurrentnessRequiredV1,
	}
	wire.ResultHash = directSourcePreviewResultHashV1(directSourcePreviewResultHashMaterialV1{
		SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose,
		DatasetSnapshotID: wire.DatasetSnapshotID, Fields: wire.Fields,
		RowOffset: wire.RowOffset, RowLimit: wire.RowLimit, Rows: wire.Rows,
		HasMore: wire.HasMore, QueryHash: wire.QueryHash, Currentness: wire.Currentness,
	})
	return wire
}

func TestDirectSourcePreviewNativeRequestIsFixedAndStrict(t *testing.T) {
	arguments := directSourcePreviewTestArgumentsV1()
	frame, err := EncodeDirectSourcePreviewNativeRequestFrameV1(strings.Repeat("a", 64), arguments)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(frame, []byte(`"command":"funds.direct_source_preview"`)) ||
		bytes.Contains(frame, []byte("dbPath")) || bytes.Contains(frame, []byte("sql")) ||
		bytes.Contains(frame, []byte("threadId")) || bytes.Contains(frame, []byte("turnId")) {
		t.Fatalf("unexpected native frame: %s", frame)
	}
	duplicate := arguments
	duplicate.Fields = append(duplicate.Fields, duplicate.Fields[0])
	if ValidateDirectSourcePreviewArgumentsV1(duplicate) == nil {
		t.Fatal("duplicate field was accepted")
	}
}

func TestDirectSourcePreviewResultKeepsExactCellsBehindTypedUse(t *testing.T) {
	arguments := directSourcePreviewTestArgumentsV1()
	wire := directSourcePreviewTestResultWireV1(arguments)
	body, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseDirectSourcePreviewResultV1(body, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var exact string
	if err := result.Rows[0].Cells[0].UseExactV1(func(value string) error {
		exact = value
		return nil
	}); err != nil || exact != "0012345678901234" {
		t.Fatalf("exact cell use failed: value=%q err=%v", exact, err)
	}
	for _, rendered := range []string{fmt.Sprint(result), fmt.Sprintf("%#v", result), fmt.Sprint(result.Rows[0].Cells[0])} {
		if strings.Contains(rendered, exact) || strings.Contains(rendered, "source-exact-name") {
			t.Fatalf("private cell escaped through fmt: %s", rendered)
		}
	}
	if encoded, err := json.Marshal(result); err == nil || encoded != nil {
		t.Fatalf("private result JSON unexpectedly succeeded: %s", encoded)
	}
}

func TestDirectSourcePreviewResultRejectsFieldOrderHashAndUnknownDrift(t *testing.T) {
	arguments := directSourcePreviewTestArgumentsV1()
	wire := directSourcePreviewTestResultWireV1(arguments)

	wire.Rows[0].Cells[0], wire.Rows[0].Cells[1] = wire.Rows[0].Cells[1], wire.Rows[0].Cells[0]
	body, _ := json.Marshal(wire)
	if _, err := ParseDirectSourcePreviewResultV1(body, arguments); err == nil {
		t.Fatal("reordered cells were accepted")
	}

	wire = directSourcePreviewTestResultWireV1(arguments)
	wire.Rows[0].Cells[0].Value = "mutated-private-value"
	body, _ = json.Marshal(wire)
	if _, err := ParseDirectSourcePreviewResultV1(body, arguments); err == nil {
		t.Fatal("result hash drift was accepted")
	}

	wire = directSourcePreviewTestResultWireV1(arguments)
	body, _ = json.Marshal(wire)
	body = bytes.Replace(body, []byte(`"purpose":`), []byte(`"unexpected":"private","purpose":`), 1)
	if _, err := ParseDirectSourcePreviewResultV1(body, arguments); err == nil {
		t.Fatal("unknown result field was accepted")
	}

	arguments = directSourcePreviewTestArgumentsV1()
	arguments.RowOffset = DirectSourcePreviewMaximumOffsetV1
	wire = directSourcePreviewTestResultWireV1(arguments)
	second := wire.Rows[0]
	second.RowIndex++
	wire.Rows = append(wire.Rows, second)
	wire.ResultHash = directSourcePreviewResultHashV1(directSourcePreviewResultHashMaterialV1{
		SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose,
		DatasetSnapshotID: wire.DatasetSnapshotID, Fields: wire.Fields,
		RowOffset: wire.RowOffset, RowLimit: wire.RowLimit, Rows: wire.Rows,
		HasMore: wire.HasMore, QueryHash: wire.QueryHash, Currentness: wire.Currentness,
	})
	body, _ = json.Marshal(wire)
	if _, err := ParseDirectSourcePreviewResultV1(body, arguments); err == nil {
		t.Fatal("row window crossed the canonical maximum offset")
	}
}
