package nativecomponent

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// This oracle covers the declared projection operators, not DuckDB execution,
// model quality, actual identity matching, or installed-product acceptance.
// Truth uses rational decimal arithmetic rather than the production minor-unit
// parser/summer; raw identity values below are synthetic test-only inputs.
func TestAccountFlowProjectionMatchesIndependentDecimalAndIdentityOracle(t *testing.T) {
	type rawRow struct {
		direction string
		decimal   string
		account   int
	}
	vectors := map[string][]rawRow{
		"same amount independent transactions": {
			{AccountFlowDirectionInflowV1, "10.00", 1},
			{AccountFlowDirectionInflowV1, "10.00", 1},
			{AccountFlowDirectionInflowV1, "10.00", 2},
		},
		"recorded reversal and same name distinct accounts": {
			{AccountFlowDirectionInflowV1, "11.25", 1},
			{AccountFlowDirectionOutflowV1, "11.25", 1},
			{AccountFlowDirectionInflowV1, "11.25", 2},
		},
		"above float exact integer range": {
			{AccountFlowDirectionInflowV1, "90071992547409.93", 1},
			{AccountFlowDirectionOutflowV1, "90071992547409.92", 2},
		},
	}
	for name, raw := range vectors {
		t.Run(name, func(t *testing.T) {
			securityContext := nativeComponentTestContext(t, "case-projection-oracle", time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC))
			arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
			arguments.evidenceRowLimit = uint32(len(raw))
			result := nativeComponentTestAccountFlowResult(arguments)
			inflow, outflow := new(big.Rat), new(big.Rat)
			rows := make([]AccountFlowEvidenceRowV1, len(raw))
			amounts := make([]string, len(raw))
			accounts := map[string]int{}
			for i, row := range raw {
				amount, ok := new(big.Rat).SetString(row.decimal)
				if !ok || amount.Sign() < 0 {
					t.Fatal("invalid independent decimal fixture")
				}
				amounts[i] = projectionOracleMinorUnitsV1(t, amount)
				if row.direction == AccountFlowDirectionInflowV1 {
					inflow.Add(inflow, amount)
				} else {
					outflow.Add(outflow, amount)
				}
				account := fmt.Sprintf("621700987654321098%d", row.account)
				accounts[account] = row.account
				name, bank := "Synthetic Same Name", "Synthetic Test Bank"
				rows[i] = AccountFlowEvidenceRowV1{
					SubjectRef:      arguments.subjectRef,
					private:         newAccountFlowEvidencePrivateV1("oracle-private-source", &account, &name, &bank),
					SourceRowNumber: uint64(i + 1), OccurredAt: "2026-01-01T00:00:00.000000Z",
					Direction: row.direction, AmountMinor: amounts[i], Currency: arguments.expectedCurrency,
					MinorUnitScale: arguments.minorUnitScale,
				}
			}
			wantInflow, wantOutflow := projectionOracleMinorUnitsV1(t, inflow), projectionOracleMinorUnitsV1(t, outflow)
			wantNet := projectionOracleMinorUnitsV1(t, new(big.Rat).Sub(inflow, outflow))
			result.InflowMinor, result.OutflowMinor, result.NetMinor = wantInflow, wantOutflow, wantNet
			result.TransactionCount = uint64(len(raw))
			result.Coverage.NormalizedSnapshotRows = uint64(len(raw))
			result.Coverage.AcceptedSnapshotRows = uint64(len(raw))
			result.Coverage.ObservedMatchingRows = uint64(len(raw))
			result.evidenceRows = newAccountFlowEvidenceRowsPrivateV1(rows)
			result.ResultHash = accountFlowResultHashV1(result)
			resolver := func(account, bank string, consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
				ordinal, exists := accounts[account]
				if !exists {
					return ErrResultInvalid
				}
				reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(domainsecurity.SHA256Hex([]byte("case-projection-oracle\x00" + account)))
				if err != nil {
					return err
				}
				display, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
					EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
					StableOrdinal: uint32(ordinal + 1), SafeSuffix: account[len(account)-4:],
					Institution: bank, AccountType: AccountFlowCounterpartyAccountTypeV1,
				})
				if err != nil {
					return err
				}
				return consume(reference, display)
			}
			semantic, retained, err := ProjectAnalyzeAccountFlowsResultV1(result, arguments,
				func(source string, number uint64, use func(string) error) error {
					return use(accountFlowProjectionTestSourceRecordIDV1(source, number))
				}, resolver)
			if err != nil {
				t.Fatalf("production projection rejected the independent oracle: %v", err)
			}
			defer DiscardAccountFlowHostEvidenceProjectionV1(retained)
			body, err := CanonicalAccountFlowProviderModelOutputV1(AccountFlowProviderModelOutputV1{
				SchemaVersion: 3, Purpose: AccountFlowProviderModelPurposeV1,
				SemanticStatus: domainevidence.SemanticSuccess, Data: semantic,
			})
			if err != nil {
				t.Fatalf("provider consumer rejected the oracle projection: %v", err)
			}
			var observed AccountFlowProviderModelOutputV1
			if err := json.Unmarshal(body, &observed); err != nil {
				t.Fatal(err)
			}
			data := observed.Data
			if data.InflowMinor != wantInflow || data.OutflowMinor != wantOutflow || data.NetMinor != wantNet ||
				data.TransactionCount != uint64(len(raw)) || len(data.Transactions) != len(raw) ||
				!data.CounterpartySemanticsComplete || data.Outcome.FactAnswerAllowed {
				t.Fatal("projection changed exact facts/coverage or granted factual authority")
			}
			refs := map[string]bool{}
			for i, row := range data.Transactions {
				wantAlias := domaincaseentity.ModelEntityAliasV1(fmt.Sprintf("acct:%d", raw[i].account+1))
				if row.AmountMinor != amounts[i] || row.Direction != raw[i].direction ||
					row.OccurredAt != rows[i].OccurredAt || row.Currency != arguments.expectedCurrency ||
					row.Counterparty.Alias != wantAlias || refs[row.EvidenceRef] {
					t.Fatal("projection collapsed an independent transaction, identity or time/currency value")
				}
				refs[row.EvidenceRef] = true
			}
			for _, withheld := range []string{"Synthetic Same Name", "Synthetic Test Bank", "oracle-private-source", "621700987654321098"} {
				if strings.Contains(string(body), withheld) {
					t.Fatal("provider projection retained a synthetic raw identifier")
				}
			}
		})
	}
}

func TestAccountFlowProjectionBindsSubjectAndCounterpartyAliasIdentity(t *testing.T) {
	securityContext := nativeComponentTestContext(t, "case-projection-subject-identity", time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC))
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	otherReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		reference domaincaseentity.ReferenceV1
		ordinal   uint32
		valid     bool
	}{
		{"distinct counterparty must not use subject alias", otherReference, 1, false},
		{"subject reference must not use another alias", domaincaseentity.ReferenceV1(arguments.subjectRef), 2, false},
		{"self transfer retains subject identity", domaincaseentity.ReferenceV1(arguments.subjectRef), 1, true},
		{"distinct counterparty retains its own alias", otherReference, 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			display, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
				EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
				StableOrdinal: test.ordinal, SafeSuffix: "0987", Institution: "Synthetic Test Bank",
				AccountType: AccountFlowCounterpartyAccountTypeV1,
			})
			if err != nil {
				t.Fatal(err)
			}
			semantic, retained, err := ProjectAnalyzeAccountFlowsResultV1(result, arguments,
				func(source string, number uint64, use func(string) error) error {
					return use(accountFlowProjectionTestSourceRecordIDV1(source, number))
				}, func(_, _ string, use func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
					return use(test.reference, display)
				})
			defer DiscardAccountFlowHostEvidenceProjectionV1(retained)
			if !test.valid {
				if !errors.Is(err, ErrResultInvalid) || retained.use != nil || retained.close != nil || semantic.Transactions != nil {
					t.Fatal("projection published an alias with inconsistent subject/counterparty identity")
				}
				return
			}
			if err != nil || !semantic.CounterpartySemanticsComplete || len(semantic.Transactions) != result.EvidenceRowCountV1() {
				t.Fatalf("projection rejected a consistent subject/counterparty identity: %v", err)
			}
			wantAlias, err := domaincaseentity.NewModelEntityAliasV1(display.EntityType, display.StableOrdinal)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range semantic.Transactions {
				if row.Counterparty.Alias != wantAlias {
					t.Fatal("projection changed a consistent subject/counterparty identity")
				}
			}
		})
	}
}

func projectionOracleMinorUnitsV1(t *testing.T, amount *big.Rat) string {
	t.Helper()
	scaled := new(big.Rat).Mul(new(big.Rat).Set(amount), big.NewRat(100, 1))
	if !scaled.IsInt() {
		t.Fatal("independent decimal fixture exceeds the admitted two-decimal currency")
	}
	return scaled.Num().String()
}
