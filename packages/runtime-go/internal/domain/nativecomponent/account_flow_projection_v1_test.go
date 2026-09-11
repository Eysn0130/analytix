package nativecomponent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAccountFlowProjectionSeparatesProviderSemanticsFromExactHostEvidence(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection",
		time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)

	resolverCalls := 0
	resolverAcceptCalls := 0
	capture := accountFlowHostEvidenceCaptureV1{}
	semantic, retained, err := ProjectAnalyzeAccountFlowsResultV1(
		result,
		arguments,
		func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
			resolverCalls++
			resolverAcceptCalls++
			return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
		},
		accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection"),
	)
	if err != nil {
		t.Fatalf("project exact account flow: %v", err)
	}
	if _, marshalErr := json.Marshal(retained); marshalErr == nil {
		t.Fatal("host-private evidence projection became JSON serializable")
	}
	type projectionWithoutMethods AccountFlowHostEvidenceProjectionV1
	aliasJSON, marshalErr := json.Marshal(projectionWithoutMethods(retained))
	if marshalErr != nil {
		t.Fatalf("defined projection type JSON: %v", marshalErr)
	}
	formatted := fmt.Sprintf("%v %+v %#v", retained, retained, retained)
	formatted += fmt.Sprintf("%v %+v %#v", projectionWithoutMethods(retained), projectionWithoutMethods(retained), projectionWithoutMethods(retained))
	for _, private := range []string{
		result.SubjectRef,
		"private-file-1",
		"6217009876543210987",
		"Private A&B Counterparty",
		accountFlowProjectionTestSourceRecordIDV1("private-file-1", 1001),
	} {
		if strings.Contains(string(aliasJSON), private) || strings.Contains(formatted, private) {
			t.Fatalf("host-private projection leaked %q through JSON/fmt: json=%s formatted=%s", private, aliasJSON, formatted)
		}
	}
	if err := ConsumeAccountFlowHostEvidenceProjectionV1(retained, capture.consume); err != nil {
		t.Fatalf("consume deferred account-flow projection: %v", err)
	}
	if resolverCalls != result.EvidenceRowCountV1() || resolverAcceptCalls != result.EvidenceRowCountV1() {
		t.Fatalf("resolver contract calls=%d accepts=%d rows=%d", resolverCalls, resolverAcceptCalls, result.EvidenceRowCountV1())
	}
	if capture.summaryCalls != 1 || capture.rowCalls != result.EvidenceRowCountV1() {
		t.Fatalf("host evidence calls summary=%d rows=%d", capture.summaryCalls, capture.rowCalls)
	}
	if capture.subjectRef != result.SubjectRef || capture.datasetSnapshotID != result.Provenance.DatasetSnapshotID ||
		capture.contextEpoch != result.Provenance.ContextEpoch || capture.contextDigest != result.Provenance.ContextDigest ||
		capture.caseBindingHash != result.Provenance.CaseBindingHash || capture.queryHash != result.QueryHash ||
		capture.resultHash != result.ResultHash || capture.inflowMinor != "1000" || capture.outflowMinor != "300" ||
		capture.netMinor != "700" || capture.transactionCount != 2 || capture.evidenceRowCount != 2 ||
		!capture.aggregateComplete || !capture.evidenceRowsComplete ||
		capture.coverage.State != AccountFlowCoverageCompleteV1 || len(capture.rows) != 2 {
		t.Fatalf("host evidence projection drifted: %#v", capture)
	}
	for index, row := range capture.rows {
		if row.subjectRef != result.SubjectRef || !validAccountFlowSourceRecordIDV1(row.sourceRecordID) ||
			row.sourceRecordID != accountFlowProjectionTestSourceRecordIDV1("private-file-1", uint64(1001+index)) ||
			row.sourceFileID != "private-file-1" || row.sourceRowNumber != uint64(1001+index) {
			t.Fatalf("host evidence row %d drifted: %#v", index, row)
		}
	}

	if semantic.SubjectAlias != arguments.subjectAlias ||
		semantic.StartInclusive != result.StartInclusive || semantic.EndInclusive != result.EndInclusive ||
		semantic.InflowMinor != "1000" || semantic.OutflowMinor != "300" || semantic.NetMinor != "700" ||
		semantic.TransactionCount != 2 || semantic.EvidenceTransactionCount != 2 ||
		!semantic.AggregateComplete || !semantic.EvidenceRowsComplete ||
		!semantic.CounterpartySemanticsComplete || semantic.QueryHash != result.QueryHash ||
		semantic.ResultHash != result.ResultHash ||
		semantic.Coverage.State != AccountFlowCoverageCompleteV1 || len(semantic.Transactions) != 2 {
		t.Fatalf("provider semantic projection drifted: %#v", semantic)
	}
	for index, row := range semantic.Transactions {
		want := accountFlowProjectionTestSourceRecordIDV1("private-file-1", uint64(1001+index))
		if row.EvidenceRef != want || !validAccountFlowSourceRecordIDV1(row.EvidenceRef) ||
			row.EvidenceRef != capture.rows[index].sourceRecordID ||
			row.Counterparty.Status != AccountFlowCounterpartyResolvedV1 ||
			ValidateAccountFlowProviderCounterpartyV1(row.Counterparty) != nil ||
			row.Counterparty.Alias != domaincaseentity.ModelEntityAliasV1("acct:1") ||
			row.Counterparty.EntityType != domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1 {
			t.Fatalf("provider evidence reference %d drifted: row=%#v host=%#v", index, row, capture.rows[index])
		}
		if index > 0 && row.Counterparty.Alias != semantic.Transactions[0].Counterparty.Alias {
			t.Fatalf("same counterparty did not retain stable alias: %#v", semantic.Transactions)
		}
	}
	providerJSON, err := json.Marshal(semantic)
	if err != nil {
		t.Fatalf("provider semantic JSON: %v", err)
	}
	if strings.Count(string(providerJSON), arguments.subjectAlias) < 1 ||
		strings.Contains(string(providerJSON), result.SubjectRef) {
		t.Fatalf("provider semantic projection did not isolate alias from authority ref: %s", providerJSON)
	}
	for _, forbidden := range []string{
		"private-file-1",
		"6217009876543210987",
		"Private A&B Counterparty",
		"Analytix Test Bank",
		"duckdb",
		"SELECT ",
	} {
		if strings.Contains(string(providerJSON), forbidden) {
			t.Fatalf("provider semantic projection leaked %q: %s", forbidden, providerJSON)
		}
	}

	detachedSummaryCalls := 0
	detachedRowCalls := 0
	if err := retained.UseExactV1(
		func(
			string, string, uint64, string, string, string, string, string, string, uint8,
			string, string, string, uint64, bool, bool, uint64, AccountFlowProviderSemanticCoverageV1, string, string,
		) error {
			detachedSummaryCalls++
			return nil
		},
		func(int, string, string, string, uint64, string, string, string, string, uint8) error {
			detachedRowCalls++
			return nil
		},
	); !errors.Is(err, ErrResultInvalid) || detachedSummaryCalls != 0 || detachedRowCalls != 0 {
		t.Fatalf("detached evidence projection remained usable: summary=%d rows=%d err=%v", detachedSummaryCalls, detachedRowCalls, err)
	}
}

func TestAccountFlowProjectionRejectsResolverAndConsumerContractViolations(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection-contract",
		time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	validID := accountFlowProjectionTestSourceRecordIDV1("fixed", 1)
	resolverFailure := errors.New("resolver authority failed")

	tests := map[string]AccountFlowSourceRowResolverV1{
		"resolver omits callback": func(string, uint64, func(string) error) error {
			return nil
		},
		"resolver calls callback twice": func(_ string, _ uint64, consume func(string) error) error {
			_ = consume(validID)
			_ = consume(validID)
			return nil
		},
		"resolver returns malformed identity": func(_ string, _ uint64, consume func(string) error) error {
			return consume("srow1_not-a-sha256")
		},
		"resolver aliases distinct locators": func(_ string, _ uint64, consume func(string) error) error {
			return consume(validID)
		},
		"resolver fails": func(string, uint64, func(string) error) error {
			return resolverFailure
		},
	}
	for name, resolver := range tests {
		t.Run(name, func(t *testing.T) {
			_, projection, err := ProjectAnalyzeAccountFlowsResultV1(
				result,
				arguments,
				resolver,
				accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-contract"),
			)
			if err == nil || projection.use != nil || projection.close != nil {
				t.Fatalf("invalid resolver survived: projection=%#v err=%v", projection, err)
			}
			if name == "resolver fails" && !errors.Is(err, resolverFailure) {
				t.Fatalf("resolver failure was lost: %v", err)
			}
		})
	}

	var detachedAccept func(string) error
	_, projection, err := ProjectAnalyzeAccountFlowsResultV1(
		result,
		arguments,
		func(_ string, _ uint64, consume func(string) error) error {
			detachedAccept = consume
			return nil
		},
		accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-contract"),
	)
	if !errors.Is(err, ErrResultInvalid) || projection.use != nil || projection.close != nil ||
		detachedAccept == nil || detachedAccept(validID) == nil {
		t.Fatalf("detached resolver callback remained usable: callback=%v err=%v", detachedAccept != nil, err)
	}

	validResolver := func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
		return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
	}
	for name, consumeEvidence := range map[string]AccountFlowHostEvidenceProjectionConsumerV1{
		"consumer omits exact use": func(AccountFlowHostEvidenceProjectionV1) error { return nil },
		"consumer invokes exact use twice": func(projection AccountFlowHostEvidenceProjectionV1) error {
			capture := accountFlowHostEvidenceCaptureV1{}
			_ = capture.consume(projection)
			_ = capture.consume(projection)
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			projected, projection, err := ProjectAnalyzeAccountFlowsResultV1(
				result,
				arguments,
				validResolver,
				accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-contract"),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := ConsumeAccountFlowHostEvidenceProjectionV1(projection, consumeEvidence); !errors.Is(err, ErrResultInvalid) || projected.Transactions == nil {
				t.Fatalf("invalid evidence consumer survived: projected=%#v err=%v", projected, err)
			}
		})
	}
}

func TestAccountFlowProjectionCounterpartyResolverFailsClosed(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-counterparty-contract",
		time.Date(2026, 7, 28, 2, 15, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	validSourceResolver := func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
		return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
	}
	validReference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	validDisplay, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: 1,
		SafeSuffix:    "0987",
		Institution:   "Analytix Test Bank",
		AccountType:   AccountFlowCounterpartyAccountTypeV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolverFailure := errors.New("counterparty authority failed")
	tests := map[string]AccountFlowCounterpartyResolverV1{
		"resolver omits callback": func(string, string, func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			return nil
		},
		"resolver calls callback twice": func(_ string, _ string, consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			_ = consume(validReference, validDisplay)
			_ = consume(validReference, validDisplay)
			return nil
		},
		"resolver returns malformed reference": func(_ string, _ string, consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			return consume("cer1_invalid", validDisplay)
		},
		"resolver returns malformed display": func(_ string, _ string, consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			invalid := validDisplay
			invalid.Text = "6217009876543210987"
			return consume(validReference, invalid)
		},
		"resolver fails": func(string, string, func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			return resolverFailure
		},
	}
	for name, resolver := range tests {
		t.Run(name, func(t *testing.T) {
			_, projection, projectionErr := ProjectAnalyzeAccountFlowsResultV1(
				result,
				arguments,
				validSourceResolver,
				resolver,
			)
			if projectionErr == nil || projection.use != nil || projection.close != nil {
				t.Fatalf("invalid counterparty resolver survived: projection=%#v err=%v", projection, projectionErr)
			}
			if name == "resolver fails" && !errors.Is(projectionErr, resolverFailure) {
				t.Fatalf("counterparty resolver failure was lost: %v", projectionErr)
			}
		})
	}

	var retainedAccept func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error
	_, projection, err := ProjectAnalyzeAccountFlowsResultV1(
		result,
		arguments,
		validSourceResolver,
		func(_ string, _ string, consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			retainedAccept = consume
			return nil
		},
	)
	if !errors.Is(err, ErrResultInvalid) || projection.use != nil || projection.close != nil ||
		retainedAccept == nil || !errors.Is(retainedAccept(validReference, validDisplay), ErrResultInvalid) {
		t.Fatalf("detached counterparty resolver callback remained usable: callback=%v err=%v", retainedAccept != nil, err)
	}
}

func TestAccountFlowProjectionCounterpartyGapsRemainExplicitlyPartial(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-counterparty-partial",
		time.Date(2026, 7, 28, 2, 20, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	validSourceResolver := func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
		return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
	}

	for name, mutate := range map[string]func(*AnalyzeAccountFlowsResultV1){
		"missing account": func(result *AnalyzeAccountFlowsResultV1) {
			mutateAccountFlowEvidenceRowsV1(result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
				rows[0].CounterpartyKey = nil
				return rows
			})
		},
		"placeholder account": func(result *AnalyzeAccountFlowsResultV1) {
			placeholder := "private-counterparty-key"
			mutateAccountFlowEvidenceRowsV1(result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
				rows[0].CounterpartyKey = &placeholder
				return rows
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := nativeComponentTestAccountFlowResult(arguments)
			mutate(&result)
			result.ResultHash = accountFlowResultHashV1(result)
			semantic, projection, err := ProjectAnalyzeAccountFlowsResultV1(
				result,
				arguments,
				validSourceResolver,
				accountFlowProjectionTestCounterpartyResolverV1("case-flow-counterparty-partial"),
			)
			if err != nil {
				t.Fatalf("project partial counterparty: %v", err)
			}
			defer DiscardAccountFlowHostEvidenceProjectionV1(projection)
			if semantic.CounterpartySemanticsComplete || semantic.Coverage.State != AccountFlowCoveragePartialV1 ||
				len(semantic.Coverage.Gaps) != 1 || semantic.Coverage.Gaps[0] != AccountFlowGapCounterpartyResolutionV1 ||
				semantic.Transactions[0].Counterparty.Status != AccountFlowCounterpartyUnresolvedV1 ||
				ValidateAccountFlowProviderCounterpartyV1(semantic.Transactions[0].Counterparty) != nil {
				t.Fatalf("counterparty gap was promoted to complete: %#v", semantic)
			}
		})
	}

	t.Run("unsafe source semantic", func(t *testing.T) {
		result := nativeComponentTestAccountFlowResult(arguments)
		resolverCalls := 0
		semantic, projection, err := ProjectAnalyzeAccountFlowsResultV1(
			result,
			arguments,
			validSourceResolver,
			func(string, string, func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
				resolverCalls++
				return ErrAccountFlowCounterpartySemanticUnavailableV1
			},
		)
		if err != nil {
			t.Fatalf("project unavailable counterparty semantic: %v", err)
		}
		defer DiscardAccountFlowHostEvidenceProjectionV1(projection)
		if resolverCalls != 2 || semantic.CounterpartySemanticsComplete ||
			semantic.Coverage.State != AccountFlowCoveragePartialV1 ||
			len(semantic.Coverage.Gaps) != 1 ||
			semantic.Coverage.Gaps[0] != AccountFlowGapCounterpartyResolutionV1 {
			t.Fatalf("unavailable counterparty semantic was promoted: %#v", semantic)
		}
		for _, row := range semantic.Transactions {
			if row.Counterparty.Status != AccountFlowCounterpartyUnresolvedV1 ||
				row.Counterparty.Alias != "" || row.Counterparty.EntityType != "" ||
				row.Counterparty.AccountType != "" {
				t.Fatalf("unavailable counterparty leaked identity semantics: %#v", row.Counterparty)
			}
		}
	})
}

func TestAccountFlowProjectionConflictingDisplayNeverRemainsResolved(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-counterparty-conflict",
		time.Date(2026, 7, 28, 2, 25, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	baseResolver := accountFlowProjectionTestCounterpartyResolverV1("case-flow-counterparty-conflict")
	resolverCalls := 0
	semantic, projection, err := ProjectAnalyzeAccountFlowsResultV1(
		result,
		arguments,
		func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
			return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
		},
		func(account string, bank string, consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			resolverCalls++
			return baseResolver(account, bank, func(reference domaincaseentity.ReferenceV1, display domaincaseentity.DisplayLabelV1) error {
				if resolverCalls == 2 {
					var labelErr error
					display, labelErr = domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
						EntityType:    display.EntityType,
						StableOrdinal: display.StableOrdinal,
						SafeSuffix:    display.SafeSuffix,
						Institution:   "Conflicting Bank",
						AccountType:   display.AccountType,
					})
					if labelErr != nil {
						return labelErr
					}
				}
				return consume(reference, display)
			})
		},
	)
	if err != nil {
		t.Fatalf("project conflicting safe display: %v", err)
	}
	defer DiscardAccountFlowHostEvidenceProjectionV1(projection)
	if semantic.CounterpartySemanticsComplete || semantic.Coverage.State != AccountFlowCoveragePartialV1 ||
		len(semantic.Coverage.Gaps) != 1 || semantic.Coverage.Gaps[0] != AccountFlowGapCounterpartyResolutionV1 {
		t.Fatalf("conflicting display was promoted to complete: %#v", semantic)
	}
	for _, row := range semantic.Transactions {
		if row.Counterparty.Status != AccountFlowCounterpartyUnresolvedV1 ||
			row.Counterparty.Alias != "" || row.Counterparty.EntityType != "" ||
			row.Counterparty.AccountType != "" ||
			ValidateAccountFlowProviderCounterpartyV1(row.Counterparty) != nil {
			t.Fatalf("conflicting reference retained resolved fact: %#v", row.Counterparty)
		}
	}
}

func TestAccountFlowProjectionRejectsDistinctReferencesSharingOneDisplayLabel(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-counterparty-label-collision",
		time.Date(2026, 7, 28, 2, 27, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	secondCounterparty := "6222021234567890123"
	mutateAccountFlowEvidenceRowsV1(&result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
		rows[1].CounterpartyKey = &secondCounterparty
		return rows
	})
	result.ResultHash = accountFlowResultHashV1(result)
	collidingDisplay, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
		EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		StableOrdinal: 9,
		SafeSuffix:    "0123",
		Institution:   "Collision Bank",
		AccountType:   AccountFlowCounterpartyAccountTypeV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, projection, err := ProjectAnalyzeAccountFlowsResultV1(
		result,
		arguments,
		func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
			return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
		},
		func(account string, _ string, consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error) error {
			reference, referenceErr := domaincaseentity.NewReferenceV1FromKeyedDigest(
				domainsecurity.SHA256Hex([]byte("collision\x00" + account)),
			)
			if referenceErr != nil {
				return referenceErr
			}
			return consume(reference, collidingDisplay)
		},
	)
	if !errors.Is(err, ErrResultInvalid) || projection.use != nil || projection.close != nil {
		t.Fatalf("distinct counterparty references shared one display label: projection=%#v err=%v", projection, err)
	}
}

func TestAccountFlowProjectionClosesRetainedCallbacksWhenResolverOrConsumerPanics(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection-panic",
		time.Date(2026, 7, 28, 2, 30, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	validID := accountFlowProjectionTestSourceRecordIDV1("panic", 1)

	t.Run("resolver callback is burned before panic escapes", func(t *testing.T) {
		var retainedAccept func(string) error
		var recovered any
		func() {
			defer func() { recovered = recover() }()
			_, _, _ = ProjectAnalyzeAccountFlowsResultV1(
				result,
				arguments,
				func(_ string, _ uint64, consume func(string) error) error {
					retainedAccept = consume
					panic("resolver panic")
				},
				accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-panic"),
			)
		}()
		if recovered == nil || retainedAccept == nil || !errors.Is(retainedAccept(validID), ErrResultInvalid) {
			t.Fatalf("resolver panic retained authority: recovered=%v callback=%v", recovered, retainedAccept != nil)
		}
	})

	t.Run("evidence carrier is burned when consumer panics", func(t *testing.T) {
		_, retained, err := ProjectAnalyzeAccountFlowsResultV1(
			result,
			arguments,
			func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
				return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
			},
			accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-panic"),
		)
		if err != nil {
			t.Fatal(err)
		}
		consumeErr := ConsumeAccountFlowHostEvidenceProjectionV1(
			retained,
			func(AccountFlowHostEvidenceProjectionV1) error {
				panic("evidence consumer panic")
			},
		)
		summaryCalls := 0
		rowCalls := 0
		err = retained.UseExactV1(
			func(
				string, string, uint64, string, string, string, string, string, string, uint8,
				string, string, string, uint64, bool, bool, uint64, AccountFlowProviderSemanticCoverageV1, string, string,
			) error {
				summaryCalls++
				return nil
			},
			func(int, string, string, string, uint64, string, string, string, string, uint8) error {
				rowCalls++
				return nil
			},
		)
		if !errors.Is(consumeErr, ErrResultInvalid) || !errors.Is(err, ErrResultInvalid) || summaryCalls != 0 || rowCalls != 0 {
			t.Fatalf("consumer panic retained authority: consume=%v summary=%d rows=%d err=%v", consumeErr, summaryCalls, rowCalls, err)
		}
	})
}

func TestAccountFlowProjectionInvalidatesInFlightEvidenceUseAtOuterCallbackBoundary(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection-concurrent-close",
		time.Date(2026, 7, 28, 2, 45, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	entered := make(chan struct{})
	release := make(chan struct{})
	consumerReturned := make(chan struct{})
	useDone := make(chan error, 1)
	consumeDone := make(chan error, 1)
	_, retained, err := ProjectAnalyzeAccountFlowsResultV1(
		result,
		arguments,
		func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
			return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
		},
		accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-concurrent-close"),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		consumeDone <- ConsumeAccountFlowHostEvidenceProjectionV1(
			retained,
			func(projection AccountFlowHostEvidenceProjectionV1) error {
				go func() {
					useDone <- projection.UseExactV1(
						func(
							string, string, uint64, string, string, string, string, string, string, uint8,
							string, string, string, uint64, bool, bool, uint64, AccountFlowProviderSemanticCoverageV1, string, string,
						) error {
							close(entered)
							<-release
							return nil
						},
						accountFlowProjectionNoopRowV1,
					)
				}()
				<-entered
				close(consumerReturned)
				return nil
			},
		)
	}()

	<-consumerReturned
	select {
	case consumeErr := <-consumeDone:
		t.Fatalf("consumer returned before in-flight private use closed: %v", consumeErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-useDone; !errors.Is(err, ErrResultInvalid) {
		t.Fatalf("in-flight private use crossed the outer callback boundary: %v", err)
	}
	if consumeErr := <-consumeDone; !errors.Is(consumeErr, ErrResultInvalid) {
		t.Fatalf("invalidated private use returned success: %v", consumeErr)
	}
	if err := retained.UseExactV1(accountFlowProjectionNoopSummaryV1, accountFlowProjectionNoopRowV1); !errors.Is(err, ErrResultInvalid) {
		t.Fatalf("closed concurrent carrier remained reusable: %v", err)
	}
}

func TestAccountFlowProjectionReentrantLifecycleCallsFailClosedWithoutDeadlock(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection-reentrant-lifecycle",
		time.Date(2026, 7, 28, 2, 47, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	project := func(t *testing.T) AccountFlowHostEvidenceProjectionV1 {
		t.Helper()
		_, projection, err := ProjectAnalyzeAccountFlowsResultV1(
			result,
			arguments,
			func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
				return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
			},
			accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-reentrant-lifecycle"),
		)
		if err != nil {
			t.Fatal(err)
		}
		return projection
	}
	await := func(t *testing.T, done <-chan error) error {
		t.Helper()
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			t.Fatal("reentrant projection lifecycle call deadlocked")
			return nil
		}
	}

	t.Run("discard from summary callback", func(t *testing.T) {
		projection := project(t)
		done := make(chan error, 1)
		go func() {
			done <- ConsumeAccountFlowHostEvidenceProjectionV1(
				projection,
				func(current AccountFlowHostEvidenceProjectionV1) error {
					return current.UseExactV1(
						func(
							string, string, uint64, string, string, string, string, string, string, uint8,
							string, string, string, uint64, bool, bool, uint64, AccountFlowProviderSemanticCoverageV1, string, string,
						) error {
							DiscardAccountFlowHostEvidenceProjectionV1(current)
							return nil
						},
						accountFlowProjectionNoopRowV1,
					)
				},
			)
		}()
		if err := await(t, done); !errors.Is(err, ErrResultInvalid) {
			t.Fatalf("reentrant summary discard survived: %v", err)
		}
		if err := projection.UseExactV1(accountFlowProjectionNoopSummaryV1, accountFlowProjectionNoopRowV1); !errors.Is(err, ErrResultInvalid) {
			t.Fatalf("reentrant summary discard left carrier usable: %v", err)
		}
	})

	t.Run("discard from row callback", func(t *testing.T) {
		projection := project(t)
		done := make(chan error, 1)
		go func() {
			done <- ConsumeAccountFlowHostEvidenceProjectionV1(
				projection,
				func(current AccountFlowHostEvidenceProjectionV1) error {
					return current.UseExactV1(
						accountFlowProjectionNoopSummaryV1,
						func(int, string, string, string, uint64, string, string, string, string, uint8) error {
							DiscardAccountFlowHostEvidenceProjectionV1(current)
							return nil
						},
					)
				},
			)
		}()
		if err := await(t, done); !errors.Is(err, ErrResultInvalid) {
			t.Fatalf("reentrant row discard survived: %v", err)
		}
		if err := projection.UseExactV1(accountFlowProjectionNoopSummaryV1, accountFlowProjectionNoopRowV1); !errors.Is(err, ErrResultInvalid) {
			t.Fatalf("reentrant row discard left carrier usable: %v", err)
		}
	})

	t.Run("nested consume from summary callback", func(t *testing.T) {
		projection := project(t)
		done := make(chan error, 1)
		var nestedErr error
		nestedConsumerCalls := 0
		go func() {
			done <- ConsumeAccountFlowHostEvidenceProjectionV1(
				projection,
				func(current AccountFlowHostEvidenceProjectionV1) error {
					return current.UseExactV1(
						func(
							string, string, uint64, string, string, string, string, string, string, uint8,
							string, string, string, uint64, bool, bool, uint64, AccountFlowProviderSemanticCoverageV1, string, string,
						) error {
							nestedErr = ConsumeAccountFlowHostEvidenceProjectionV1(
								current,
								func(AccountFlowHostEvidenceProjectionV1) error {
									nestedConsumerCalls++
									return nil
								},
							)
							return nil
						},
						accountFlowProjectionNoopRowV1,
					)
				},
			)
		}()
		if err := await(t, done); !errors.Is(err, ErrResultInvalid) ||
			!errors.Is(nestedErr, ErrResultInvalid) || nestedConsumerCalls != 0 {
			t.Fatalf(
				"nested consume survived: outer=%v nested=%v consumer_calls=%d",
				err,
				nestedErr,
				nestedConsumerCalls,
			)
		}
		if err := projection.UseExactV1(accountFlowProjectionNoopSummaryV1, accountFlowProjectionNoopRowV1); !errors.Is(err, ErrResultInvalid) {
			t.Fatalf("nested consume left carrier usable: %v", err)
		}
	})

	t.Run("detached direct use burns carrier", func(t *testing.T) {
		projection := project(t)
		if err := projection.UseExactV1(accountFlowProjectionNoopSummaryV1, accountFlowProjectionNoopRowV1); !errors.Is(err, ErrResultInvalid) {
			t.Fatalf("detached direct use succeeded: %v", err)
		}
		consumerCalls := 0
		if err := ConsumeAccountFlowHostEvidenceProjectionV1(
			projection,
			func(AccountFlowHostEvidenceProjectionV1) error {
				consumerCalls++
				return nil
			},
		); !errors.Is(err, ErrResultInvalid) || consumerCalls != 0 {
			t.Fatalf("detached direct use left consume authority: err=%v calls=%d", err, consumerCalls)
		}
	})
}

func TestAccountFlowProjectionCloseWaitsForInFlightUseAndPrivateCleanup(t *testing.T) {
	cleanupStarted := make(chan struct{})
	cleanupRelease := make(chan struct{})
	state := newAccountFlowHostEvidenceProjectionUseStateV1(func() {
		close(cleanupStarted)
		<-cleanupRelease
	})
	if !state.beginConsumer() || !state.begin() {
		t.Fatal("projection lifecycle did not enter exact use")
	}
	type closeResult struct {
		attempts  uint32
		completed bool
	}
	closeDone := make(chan closeResult, 1)
	go func() {
		attempts, completed := state.close()
		closeDone <- closeResult{attempts: attempts, completed: completed}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		state.mu.Lock()
		active := state.active
		state.mu.Unlock()
		if !active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("outer projection close did not deactivate in-flight use")
		}
		time.Sleep(time.Millisecond)
	}
	endDone := make(chan struct{})
	go func() {
		state.end(false)
		close(endDone)
	}()
	select {
	case <-cleanupStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight use did not start private cleanup")
	}
	select {
	case result := <-closeDone:
		t.Fatalf("outer close returned before private cleanup: %#v", result)
	default:
	}
	select {
	case <-endDone:
		t.Fatal("exact use returned before private cleanup")
	default:
	}
	close(cleanupRelease)
	select {
	case <-endDone:
	case <-time.After(2 * time.Second):
		t.Fatal("exact use did not finish after private cleanup")
	}
	select {
	case result := <-closeDone:
		if result.attempts != 1 || result.completed {
			t.Fatalf("outer close returned unexpected lifecycle state: %#v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("outer close did not finish after private cleanup")
	}
}

func TestAccountFlowProjectionConsumerFailureReturnsZeroAndBurnsCarrier(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection-consumer-failure",
		time.Date(2026, 7, 28, 2, 50, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	failure := errors.New("private evidence consumer failed")
	for _, test := range []struct {
		name    string
		summary AccountFlowHostEvidenceSummaryConsumerV1
		row     AccountFlowHostEvidenceRowConsumerV1
	}{
		{name: "summary", summary: func(
			string, string, uint64, string, string, string, string, string, string, uint8,
			string, string, string, uint64, bool, bool, uint64, AccountFlowProviderSemanticCoverageV1, string, string,
		) error {
			return failure
		}, row: accountFlowProjectionNoopRowV1},
		{name: "row", summary: accountFlowProjectionNoopSummaryV1, row: func(int, string, string, string, uint64, string, string, string, string, uint8) error { return failure }},
	} {
		t.Run(test.name, func(t *testing.T) {
			semantic, retained, err := ProjectAnalyzeAccountFlowsResultV1(
				result,
				arguments,
				func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
					return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
				},
				accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-consumer-failure"),
			)
			if err != nil {
				t.Fatal(err)
			}
			err = ConsumeAccountFlowHostEvidenceProjectionV1(
				retained,
				func(projection AccountFlowHostEvidenceProjectionV1) error {
					return projection.UseExactV1(test.summary, test.row)
				},
			)
			if !errors.Is(err, failure) || semantic.Transactions == nil || semantic.TransactionCount != 2 {
				t.Fatalf("consumer failure changed deferred semantic: semantic=%#v err=%v", semantic, err)
			}
			if retryErr := retained.UseExactV1(accountFlowProjectionNoopSummaryV1, accountFlowProjectionNoopRowV1); !errors.Is(retryErr, ErrResultInvalid) {
				t.Fatalf("failed private carrier remained reusable: %v", retryErr)
			}
		})
	}
}

func TestAccountFlowProjectionRevalidatesAggregationRowsAndSourceUniquenessBeforeResolution(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection-integrity",
		time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC),
	)
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	valid := nativeComponentTestAccountFlowResult(arguments)

	tests := map[string]func(*AnalyzeAccountFlowsResultV1){
		"aggregate detached from exact rows": func(result *AnalyzeAccountFlowsResultV1) {
			result.InflowMinor = "1001"
			result.NetMinor = "701"
		},
		"complete evidence row count mismatch": func(result *AnalyzeAccountFlowsResultV1) {
			mutateAccountFlowEvidenceRowsV1(result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
				return rows[:1]
			})
		},
		"duplicate native source row": func(result *AnalyzeAccountFlowsResultV1) {
			mutateAccountFlowEvidenceRowsV1(result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
				rows[1].SourceRowNumber = rows[0].SourceRowNumber
				return rows
			})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			tampered := cloneAccountFlowResultV1(valid)
			mutate(&tampered)
			tampered.ResultHash = accountFlowResultHashV1(tampered)
			resolverCalls := 0
			_, projection, err := ProjectAnalyzeAccountFlowsResultV1(
				tampered,
				arguments,
				func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
					resolverCalls++
					return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
				},
				accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-integrity"),
			)
			if !errors.Is(err, ErrResultInvalid) || resolverCalls != 0 || projection.use != nil || projection.close != nil {
				t.Fatalf("invalid exact result reached projection: resolvers=%d projection=%#v err=%v", resolverCalls, projection, err)
			}
		})
	}
}

func TestAccountFlowProjectionPreservesPartialAndNoHitOutcomes(t *testing.T) {
	securityContext := nativeComponentTestContext(
		t,
		"case-flow-projection-outcomes",
		time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC),
	)

	t.Run("bounded rows stay explicitly partial", func(t *testing.T) {
		arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
		arguments.evidenceRowLimit = 1
		result := nativeComponentTestAccountFlowResult(arguments)
		mutateAccountFlowEvidenceRowsV1(&result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
			return rows[:1]
		})
		result.EvidenceRowsComplete = false
		result.Coverage.State = AccountFlowCoveragePartialV1
		result.Coverage.Gaps = []string{AccountFlowGapEvidenceRowLimitV1}
		result.QueryHash = accountFlowQueryHashV1(arguments, result.Provenance.DuckDBContentSnapshotDigest)
		result.ResultHash = accountFlowResultHashV1(result)

		capture := accountFlowHostEvidenceCaptureV1{}
		semantic, projection, err := ProjectAnalyzeAccountFlowsResultV1(
			result,
			arguments,
			func(sourceFileID string, sourceRowNumber uint64, consume func(string) error) error {
				return consume(accountFlowProjectionTestSourceRecordIDV1(sourceFileID, sourceRowNumber))
			},
			accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-outcomes"),
		)
		if err != nil {
			t.Fatalf("project partial result: %v", err)
		}
		semantic.Coverage.Gaps[0] = "caller-mutated-gap"
		if err := ConsumeAccountFlowHostEvidenceProjectionV1(projection, capture.consume); err != nil {
			t.Fatalf("consume partial result: %v", err)
		}
		if semantic.Coverage.State != AccountFlowCoveragePartialV1 ||
			!semantic.AggregateComplete || semantic.EvidenceRowsComplete ||
			semantic.TransactionCount != 2 || semantic.EvidenceTransactionCount != 1 || len(semantic.Transactions) != 1 ||
			capture.coverage.State != AccountFlowCoveragePartialV1 || capture.evidenceRowsComplete ||
			len(capture.coverage.Gaps) != 1 || capture.coverage.Gaps[0] != AccountFlowGapEvidenceRowLimitV1 ||
			capture.evidenceRowCount != 1 || capture.transactionCount != 2 {
			t.Fatalf("partial result was promoted to complete: semantic=%#v capture=%#v", semantic, capture)
		}
	})

	t.Run("zero observations stay pending host binding", func(t *testing.T) {
		arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
		result := nativeComponentTestAccountFlowResult(arguments)
		mutateAccountFlowEvidenceRowsV1(&result, func([]accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
			return []accountFlowEvidenceRowWireV1{}
		})
		result.InflowMinor = "0"
		result.OutflowMinor = "0"
		result.NetMinor = "0"
		result.TransactionCount = 0
		result.Coverage = AccountFlowCoverageV1{
			State: AccountFlowCoverageObservedNoHitPendingBindingV1,
			Gaps:  []string{},
		}
		result.QueryHash = accountFlowQueryHashV1(arguments, result.Provenance.DuckDBContentSnapshotDigest)
		result.ResultHash = accountFlowResultHashV1(result)
		resolverCalls := 0
		capture := accountFlowHostEvidenceCaptureV1{}
		semantic, projection, err := ProjectAnalyzeAccountFlowsResultV1(
			result,
			arguments,
			func(string, uint64, func(string) error) error {
				resolverCalls++
				return nil
			},
			accountFlowProjectionTestCounterpartyResolverV1("case-flow-projection-outcomes"),
		)
		if err != nil {
			t.Fatalf("project no-hit result: %v", err)
		}
		if err := ConsumeAccountFlowHostEvidenceProjectionV1(projection, capture.consume); err != nil {
			t.Fatalf("consume no-hit result: %v", err)
		}
		if resolverCalls != 0 || semantic.Coverage.State != AccountFlowCoverageObservedNoHitPendingBindingV1 ||
			semantic.TransactionCount != 0 || semantic.EvidenceTransactionCount != 0 ||
			len(semantic.Transactions) != 0 || capture.coverage.State != AccountFlowCoverageObservedNoHitPendingBindingV1 ||
			capture.transactionCount != 0 || capture.evidenceRowCount != 0 || capture.rowCalls != 0 {
			t.Fatalf("no-hit result was fabricated as positive/complete: resolvers=%d semantic=%#v capture=%#v", resolverCalls, semantic, capture)
		}
	})
}

type accountFlowHostEvidenceCaptureV1 struct {
	summaryCalls         int
	rowCalls             int
	subjectRef           string
	datasetSnapshotID    string
	contextEpoch         uint64
	contextDigest        string
	caseBindingHash      string
	startInclusive       string
	endInclusive         string
	timezone             string
	currency             string
	minorUnitScale       uint8
	inflowMinor          string
	outflowMinor         string
	netMinor             string
	transactionCount     uint64
	aggregateComplete    bool
	evidenceRowsComplete bool
	evidenceRowCount     uint64
	coverage             AccountFlowProviderSemanticCoverageV1
	queryHash            string
	resultHash           string
	rows                 []accountFlowHostEvidenceRowCaptureV1
}

type accountFlowHostEvidenceRowCaptureV1 struct {
	index           int
	subjectRef      string
	sourceRecordID  string
	sourceFileID    string
	sourceRowNumber uint64
	occurredAt      string
	direction       string
	amountMinor     string
	currency        string
	minorUnitScale  uint8
}

func (capture *accountFlowHostEvidenceCaptureV1) consume(projection AccountFlowHostEvidenceProjectionV1) error {
	return projection.UseExactV1(capture.consumeSummary, capture.consumeRow)
}

func (capture *accountFlowHostEvidenceCaptureV1) consumeSummary(
	subjectRef string,
	datasetSnapshotID string,
	contextEpoch uint64,
	contextDigest string,
	caseBindingHash string,
	startInclusive string,
	endInclusive string,
	timezone string,
	currency string,
	minorUnitScale uint8,
	inflowMinor string,
	outflowMinor string,
	netMinor string,
	transactionCount uint64,
	aggregateComplete bool,
	evidenceRowsComplete bool,
	evidenceRowCount uint64,
	coverage AccountFlowProviderSemanticCoverageV1,
	queryHash string,
	resultHash string,
) error {
	capture.summaryCalls++
	capture.subjectRef = subjectRef
	capture.datasetSnapshotID = datasetSnapshotID
	capture.contextEpoch = contextEpoch
	capture.contextDigest = contextDigest
	capture.caseBindingHash = caseBindingHash
	capture.startInclusive = startInclusive
	capture.endInclusive = endInclusive
	capture.timezone = timezone
	capture.currency = currency
	capture.minorUnitScale = minorUnitScale
	capture.inflowMinor = inflowMinor
	capture.outflowMinor = outflowMinor
	capture.netMinor = netMinor
	capture.transactionCount = transactionCount
	capture.aggregateComplete = aggregateComplete
	capture.evidenceRowsComplete = evidenceRowsComplete
	capture.evidenceRowCount = evidenceRowCount
	capture.coverage = coverage
	capture.queryHash = queryHash
	capture.resultHash = resultHash
	return nil
}

func (capture *accountFlowHostEvidenceCaptureV1) consumeRow(
	index int,
	subjectRef string,
	sourceRecordID string,
	sourceFileID string,
	sourceRowNumber uint64,
	occurredAt string,
	direction string,
	amountMinor string,
	currency string,
	minorUnitScale uint8,
) error {
	capture.rowCalls++
	capture.rows = append(capture.rows, accountFlowHostEvidenceRowCaptureV1{
		index:           index,
		subjectRef:      subjectRef,
		sourceRecordID:  sourceRecordID,
		sourceFileID:    sourceFileID,
		sourceRowNumber: sourceRowNumber,
		occurredAt:      occurredAt,
		direction:       direction,
		amountMinor:     amountMinor,
		currency:        currency,
		minorUnitScale:  minorUnitScale,
	})
	return nil
}

func accountFlowProjectionTestSourceRecordIDV1(sourceFileID string, sourceRowNumber uint64) string {
	return accountFlowSourceRecordIDPrefixV1 + domainsecurity.SHA256Hex(
		[]byte(fmt.Sprintf("%s\x00%d", sourceFileID, sourceRowNumber)),
	)
}

func accountFlowProjectionTestCounterpartyResolverV1(caseSeed string) AccountFlowCounterpartyResolverV1 {
	return func(
		sourceExactAccount string,
		bankInstitution string,
		consume func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
	) error {
		reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(
			domainsecurity.SHA256Hex([]byte(caseSeed + "\x00" + sourceExactAccount)),
		)
		if err != nil {
			return err
		}
		safeSuffix := ""
		if len(sourceExactAccount) >= 4 {
			safeSuffix = sourceExactAccount[len(sourceExactAccount)-4:]
		}
		display, err := domaincaseentity.NewDisplayLabelV1(domaincaseentity.DisplayLabelInputV1{
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1,
			SafeSuffix:    safeSuffix,
			Institution:   bankInstitution,
			AccountType:   AccountFlowCounterpartyAccountTypeV1,
		})
		if err != nil {
			return err
		}
		return consume(reference, display)
	}
}

func accountFlowProjectionNoopSummaryV1(
	string,
	string,
	uint64,
	string,
	string,
	string,
	string,
	string,
	string,
	uint8,
	string,
	string,
	string,
	uint64,
	bool,
	bool,
	uint64,
	AccountFlowProviderSemanticCoverageV1,
	string,
	string,
) error {
	return nil
}

func accountFlowProjectionNoopRowV1(int, string, string, string, uint64, string, string, string, string, uint8) error {
	return nil
}
