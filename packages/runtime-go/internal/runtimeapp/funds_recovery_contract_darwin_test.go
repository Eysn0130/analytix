//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// Offline independent CSV truth checks the numerical/multiplicity assertion
// used by the native vector without executing a native adapter or Provider.
func TestFundsDeliveryNineRowCSVIndependentGold(t *testing.T) {
	source := deliveryCNYCSV(t)
	rows, err := csv.NewReader(bytes.NewReader(source)).ReadAll()
	if err != nil || len(rows) != 10 {
		t.Fatal("delivery source must retain the original nine imported observations")
	}
	flow := b1ReferenceFlowFromCSVInWindow(t, source, "2026-09-")
	if flow.inflowMinor != "1301001" || flow.outflowMinor != "120060" || flow.netMinor != "1180941" || flow.transactionCount != 7 {
		t.Fatal("independent CSV/big.Int truth differs from the existing nine-row/seven-transaction contract")
	}
	transactions := deliveryReferenceTransactions(t, source)
	count := 0
	for _, occurrences := range transactions {
		count += occurrences
	}
	if count != 7 {
		t.Fatal("independent transaction multiset lost target observations")
	}
	t.Logf("independent delivery gold source_sha256=%s imported_rows=9 target_transactions=7 inflow_minor=%s outflow_minor=%s net_minor=%s", domainsecurity.SHA256Hex(source), flow.inflowMinor, flow.outflowMinor, flow.netMinor)
}

func TestFundsDeliveryIndependentCSVTruthRejectsSemanticDrift(t *testing.T) {
	source := []byte("交易账号,交易时间,交易金额,收付标志,交易币种\n" +
		rev14PrivateAccount + ",2026-09-01 10:00:00,90071992547409.93,进,CNY\n" +
		rev14PrivateAccount + ",2026-09-01 10:00:00,90071992547409.93,进,CNY\n" +
		"other-account,2026-09-01 10:00:00,1.00,进,CNY\n")
	expected := deliveryReferenceTransactions(t, source)
	want := deliveryTransactionValue{"2026-09-01T10:00:00Z", domainnative.AccountFlowDirectionInflowV1, "9007199254740993", "CNY", 2}
	if len(expected) != 1 || expected[want] != 2 {
		t.Fatal("independent CSV truth lost exact integer or row multiplicity")
	}
	rows := []domainnative.AccountFlowProviderSemanticTransactionV1{}
	for _, seed := range []string{"a", "b"} {
		rows = append(rows, domainnative.AccountFlowProviderSemanticTransactionV1{
			EvidenceRef: "srow1_" + domainsecurity.SHA256Hex([]byte(seed)), OccurredAt: want.occurredAt, Direction: want.direction, AmountMinor: want.amountMinor, Currency: want.currency, MinorUnitScale: want.minorUnitScale,
		})
	}
	if !deliveryTransactionsMatch(expected, rows) {
		t.Fatal("exact independently computed rows were rejected")
	}
	for _, mutate := range []func([]domainnative.AccountFlowProviderSemanticTransactionV1) []domainnative.AccountFlowProviderSemanticTransactionV1{
		func(r []domainnative.AccountFlowProviderSemanticTransactionV1) []domainnative.AccountFlowProviderSemanticTransactionV1 {
			return r[:1]
		},
		func(r []domainnative.AccountFlowProviderSemanticTransactionV1) []domainnative.AccountFlowProviderSemanticTransactionV1 {
			r[1].EvidenceRef = r[0].EvidenceRef
			return r
		},
		func(r []domainnative.AccountFlowProviderSemanticTransactionV1) []domainnative.AccountFlowProviderSemanticTransactionV1 {
			r[0].AmountMinor = "9007199254740992"
			return r
		},
		func(r []domainnative.AccountFlowProviderSemanticTransactionV1) []domainnative.AccountFlowProviderSemanticTransactionV1 {
			r[0].Currency = "USD"
			return r
		},
		func(r []domainnative.AccountFlowProviderSemanticTransactionV1) []domainnative.AccountFlowProviderSemanticTransactionV1 {
			r[0].Direction = domainnative.AccountFlowDirectionOutflowV1
			return r
		},
	} {
		if deliveryTransactionsMatch(expected, mutate(append([]domainnative.AccountFlowProviderSemanticTransactionV1(nil), rows...))) {
			t.Fatal("semantic drift passed independent CSV truth")
		}
	}
}

func TestFundsRecoveryReadsEveryExpectedThreadAndFinal(t *testing.T) {
	for _, twoThreads := range []bool{false, true} {
		name := "same-thread-source-update"
		expected := []b1RecoveryExpectedFinal{
			{ThreadID: "thread-a", TurnID: "turn-a", FinalDigest: strings.Repeat("a", 64), HistoryState: "retained_snapshot"},
			{ThreadID: "thread-a", TurnID: "turn-b", FinalDigest: strings.Repeat("b", 64)},
		}
		if twoThreads {
			name = "two-thread-vector"
			expected[0].HistoryState = ""
			expected[1].ThreadID = "thread-b"
		}
		t.Run(name, func(t *testing.T) {
			reads := map[string]int{}
			var checked []b1RecoveryExpectedFinal
			err := b1VisitRecoveryFinals(expected, func(threadID string) (map[string]any, error) {
				reads[threadID]++
				return map[string]any{"owner": threadID}, nil
			}, func(final b1RecoveryExpectedFinal, thread map[string]any) error {
				if thread["owner"] != final.ThreadID {
					return errors.New("final was read from another thread")
				}
				checked = append(checked, final)
				return nil
			})
			if err != nil || !reflect.DeepEqual(checked, expected) || reads["thread-a"] != 1 || (twoThreads && reads["thread-b"] != 1) || (!twoThreads && len(reads) != 1) {
				t.Fatal("recovery did not read and check the full per-thread expectation set")
			}
			if !b1RecoveryAdmissionMatches(factRecoveryObservationV1{Candidates: 2, Admitted: 2}, expected) {
				t.Fatal("complete two-final admission was rejected")
			}
			for _, report := range []factRecoveryObservationV1{
				{Candidates: 1, Admitted: 1}, {Candidates: 3, Admitted: 3}, {Candidates: 2, Admitted: 1, Held: 1}, {Candidates: 2, Admitted: 2, Held: 1},
			} {
				if b1RecoveryAdmissionMatches(report, expected) {
					t.Fatal("incomplete, extra or held fact admission was accepted")
				}
			}
		})
	}
}

func TestFundsRecoveryRejectsIncompleteOrRepeatedExpectations(t *testing.T) {
	valid := b1RecoveryExpectedFinal{ThreadID: "thread", TurnID: "turn", FinalDigest: strings.Repeat("a", 64)}
	invalid := [][]b1RecoveryExpectedFinal{nil, {valid, valid}, {valid, {ThreadID: "thread", TurnID: "other", FinalDigest: strings.Repeat("a", 64), HistoryState: "unknown"}}}
	for field := 0; field < 3; field++ {
		bad := valid
		switch field {
		case 0:
			bad.ThreadID = ""
		case 1:
			bad.TurnID = ""
		case 2:
			bad.FinalDigest = "invalid"
		}
		invalid = append(invalid, []b1RecoveryExpectedFinal{bad})
	}
	for _, expected := range invalid {
		called := false
		err := b1VisitRecoveryFinals(expected, func(string) (map[string]any, error) {
			called = true
			return nil, nil
		}, func(b1RecoveryExpectedFinal, map[string]any) error { called = true; return nil })
		if err == nil || called || b1RecoveryAdmissionMatches(factRecoveryObservationV1{}, expected) {
			t.Fatal("invalid expectation set reached readback or admission")
		}
	}
	for _, failRead := range []bool{true, false} {
		err := b1VisitRecoveryFinals([]b1RecoveryExpectedFinal{valid}, func(string) (map[string]any, error) {
			if failRead {
				return nil, errors.New("read failed")
			}
			return nil, nil
		}, func(b1RecoveryExpectedFinal, map[string]any) error { return errors.New("check failed") })
		if err == nil {
			t.Fatal("readback failure was ignored")
		}
	}
}

func TestFundsRecoveryPhaseJournalIsBoundedPrivateAndImmediate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phases.jsonl")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	w := &b1RecoveryPhaseWriter{persist: func(body []byte) error {
		if _, err := file.Write(body); err != nil {
			return err
		}
		return file.Sync()
	}}
	encoded, _ := json.Marshal(b1RecoveryPhase{Phase: "assembly-begin"})
	line := []byte(b1RecoveryPhasePrefix + string(encoded) + "\n")
	for _, part := range [][]byte{line[:9], line[9:]} {
		if n, err := w.Write(part); err != nil || n != len(part) {
			t.Fatal("fragmented child phase write failed")
		}
	}
	body, err := os.ReadFile(path)
	info, statErr := os.Stat(path)
	if err != nil || statErr != nil || info.Mode().Perm() != 0o600 || !bytes.Equal(body, append(encoded, '\n')) {
		t.Fatal("phase was not durably available before child completion")
	}
	unsafe := []string{
		"panic: secret synthetic account or path\n",
		b1RecoveryPhasePrefix + `{"phase":"unknown-sensitive-text"}` + "\n",
		b1RecoveryPhasePrefix + `{"phase":"assembly-end","raw":"sensitive synthetic body"}` + "\n",
		b1RecoveryPhasePrefix + `{"phase":"assembly-end","ordinal":-1}` + "\n",
		b1RecoveryPhasePrefix + `{"phase":"assembly-end","candidates":1,"admitted":2}` + "\n",
		b1RecoveryPhasePrefix + `{"phase":"assembly-end"} {}` + "\n",
		strings.Repeat("x", 100_000) + "\n",
	}
	for _, line := range unsafe {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) || len(w.records) != 1 || cap(w.line) > 512 {
		t.Fatal("raw, unknown, malformed or oversized child output entered the journal")
	}
	for range 100 {
		if _, err := w.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	bounded, err := os.ReadFile(path)
	if err != nil || len(w.records) != 64 || len(bounded) > 16<<10 || !bytes.Equal(bounded, bytes.Repeat(append(encoded, '\n'), 64)) {
		t.Fatal("child phase journal exceeded its record or byte bound")
	}
	failed := &b1RecoveryPhaseWriter{persist: func([]byte) error { return errors.New("journal unavailable") }}
	if _, err := failed.Write(line); err == nil {
		t.Fatal("phase journal persistence failure was ignored")
	}
}

func TestFundsRecoveryFinalReadbackFailureClassifiesStrictBoundaries(t *testing.T) {
	expected := b1RecoveryExpectedFinal{ThreadID: "thread", TurnID: "turn", FinalDigest: strings.Repeat("a", 64)}
	view := domainevidence.AcceptedFinalPublicViewV3{
		SchemaVersion: domainevidence.AcceptedFinalPublicViewV3Version, AcceptedFinalDigest: expected.FinalDigest,
		PublicationState: domainevidence.AcceptedFinalPublicationAccepted, Variant: domainevidence.GeneralGuidanceAnswer,
		CoverageStatus: domainevidence.AcceptedFinalCoverageGuidanceOnly, ClaimTypes: []string{}, AcceptedAt: "2026-10-02T00:00:00Z",
		ReceiptMetadata: domainevidence.AcceptedFinalPublicReceiptMetadataV1{Projection: domainevidence.AcceptedFinalReceiptProjection, SetDigest: strings.Repeat("b", 64), Citations: []domainevidence.AcceptedFinalPublicCitationV1{}},
	}
	valid := domainevidence.AcceptedFinalPublicViewRecordV3(view)
	if valid == nil {
		t.Fatal("invalid independent public-view fixture")
	}
	for _, tc := range []struct {
		name string
		turn map[string]any
		want string
	}{
		{"turn-missing", nil, "final-turn-missing"},
		{"view-missing", map[string]any{"id": "turn"}, "final-view-missing"},
		{"view-invalid-before-label", map[string]any{"id": "turn", "acceptedFinalView": map[string]any{}, "factHistoryState": "retained_snapshot"}, "final-view-invalid"},
		{"identity-before-label", map[string]any{"id": "turn", "acceptedFinalView": func() map[string]any {
			bad := view
			bad.AcceptedFinalDigest = strings.Repeat("c", 64)
			return domainevidence.AcceptedFinalPublicViewRecordV3(bad)
		}(), "factHistoryState": "retained_snapshot"}, "final-identity-mismatch"},
		{"label-mismatch", map[string]any{"id": "turn", "acceptedFinalView": valid, "factHistoryState": "retained_snapshot"}, "final-history-label-mismatch"},
		{"current", map[string]any{"id": "turn", "acceptedFinalView": valid}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			thread := map[string]any{"turns": []any{tc.turn}}
			if got := b1RecoveryFinalReadbackFailure(expected, thread); got != tc.want {
				t.Fatal("strict readback boundary was misclassified")
			}
		})
	}
	expected.HistoryState = "retained_snapshot"
	if b1RecoveryFinalReadbackFailure(expected, map[string]any{"turns": []any{map[string]any{"id": "turn", "acceptedFinalView": valid, "factHistoryState": "retained_snapshot"}}}) != "" {
		t.Fatal("exact retained expectation rejected")
	}
}
