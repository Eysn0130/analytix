package loop

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestHostCaseEntitySelectionUsesOnlyExactCurrentIngressScope(t *testing.T) {
	securityContext := hostCaseEntitySelectionContextV1(t, "case-selection-a", 4)
	referenceA := domaincaseentity.ReferenceV1(
		"cer1_" + strings.Repeat("a", 64),
	)
	referenceB := domaincaseentity.ReferenceV1(
		"cer1_" + strings.Repeat("b", 64),
	)
	record := caseentityapp.PrivateRecordReferenceV1{
		RecordID: domainsecurity.SHA256Hex(
			[]byte("host-case-entity-selection-record"),
		),
		RecordDigest: domainsecurity.SHA256Hex(
			[]byte("host-case-entity-selection-record-digest"),
		),
	}
	selection, err := NewHostCaseEntitySelectionFromPersistedIngressV1(
		securityContext,
		record,
		[]domaincaseentity.ReferenceV1{referenceB, referenceA},
	)
	if err != nil || !selection.AvailableV1() {
		t.Fatalf("valid host case-entity selection was unavailable: %v", err)
	}
	want := []domaincaseentity.ReferenceV1{referenceA, referenceB}
	uses := 0
	err = selection.UseExactV1(
		securityContext,
		func(view HostCaseEntitySelectionViewV1) error {
			uses++
			if view.IngressRecordID != record.RecordID ||
				view.IngressRecordDigest != record.RecordDigest ||
				!domainsecurity.IsSHA256Hex(view.SelectionDigest) ||
				!reflect.DeepEqual(view.EntityReferences, want) {
				t.Fatalf("host selection view changed provenance: %#v", view)
			}
			view.EntityReferences[0] = referenceB
			return nil
		},
	)
	if err != nil || uses != 1 || !selection.AvailableV1() {
		t.Fatalf("host selection was not callback-scoped: uses=%d err=%v", uses, err)
	}

	otherContext := hostCaseEntitySelectionContextV1(t, "case-selection-b", 5)
	if err := selection.UseExactV1(
		otherContext,
		func(HostCaseEntitySelectionViewV1) error { return nil },
	); err == nil {
		t.Fatal("cross-case host selection was accepted")
	}

	tampered := selection
	tampered.selectionDigest = domainsecurity.SHA256Hex([]byte("tampered"))
	if tampered.AvailableV1() || tampered.UseExactV1(
		securityContext,
		func(HostCaseEntitySelectionViewV1) error { return nil },
	) == nil {
		t.Fatal("tampered host selection remained usable")
	}
}

func TestHostCaseEntitySelectionRejectsEmptyInvalidAndDuplicateReferences(
	t *testing.T,
) {
	securityContext := hostCaseEntitySelectionContextV1(t, "case-selection", 4)
	record := caseentityapp.PrivateRecordReferenceV1{
		RecordID:     domainsecurity.SHA256Hex([]byte("selection-record")),
		RecordDigest: domainsecurity.SHA256Hex([]byte("selection-record-digest")),
	}
	valid := domaincaseentity.ReferenceV1(
		"cer1_" + strings.Repeat("c", 64),
	)
	tests := []struct {
		name       string
		references []domaincaseentity.ReferenceV1
	}{
		{name: "empty"},
		{name: "invalid", references: []domaincaseentity.ReferenceV1{"cer1_invalid"}},
		{name: "duplicate", references: []domaincaseentity.ReferenceV1{valid, valid}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection, err := NewHostCaseEntitySelectionFromPersistedIngressV1(
				securityContext,
				record,
				test.references,
			)
			if err == nil || selection.AvailableV1() {
				t.Fatalf("invalid selection was accepted: %#v", selection)
			}
		})
	}
}

func TestExplicitCaseModelAliasesRejectDuplicatesUnsupportedAndOverflow(t *testing.T) {
	aliases, err := explicitCaseModelAliasesV1("analyze acct:1 and card:2")
	if err != nil || !reflect.DeepEqual(aliases, []domaincaseentity.ModelEntityAliasV1{"acct:1", "card:2"}) {
		t.Fatalf("canonical explicit aliases mismatch: aliases=%#v err=%v", aliases, err)
	}
	for _, prompt := range []string{
		"analyze acct:1 then acct:1",
		"analyze person:1",
		"analyze acct:4294967296",
		"analyze ACCT:1",
	} {
		if aliases, err := explicitCaseModelAliasesV1(prompt); err == nil || aliases != nil {
			t.Fatalf("invalid explicit alias was accepted: prompt=%q aliases=%#v err=%v", prompt, aliases, err)
		}
	}
}

func TestHostCaseEntitySelectionCannotEnterOrdinarySerialization(t *testing.T) {
	securityContext := hostCaseEntitySelectionContextV1(t, "case-selection", 4)
	reference := domaincaseentity.ReferenceV1(
		"cer1_" + strings.Repeat("d", 64),
	)
	selection, err := NewHostCaseEntitySelectionFromPersistedIngressV1(
		securityContext,
		caseentityapp.PrivateRecordReferenceV1{
			RecordID:     domainsecurity.SHA256Hex([]byte("serialization-record")),
			RecordDigest: domainsecurity.SHA256Hex([]byte("serialization-record-digest")),
		},
		[]domaincaseentity.ReferenceV1{reference},
	)
	if err != nil {
		t.Fatal(err)
	}
	if body, err := json.Marshal(selection); err == nil || len(body) != 0 {
		t.Fatalf("private selection serialized into ordinary JSON: body=%q err=%v", body, err)
	}
	formatted := fmt.Sprintf("%v %#v", selection, selection)
	if strings.Contains(formatted, string(reference)) ||
		!strings.Contains(formatted, "[REDACTED]") {
		t.Fatalf("private selection escaped through formatting: %s", formatted)
	}
}

func hostCaseEntitySelectionContextV1(
	t *testing.T,
	caseID string,
	epoch uint64,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := testsecurity.CaseExecutionContextV2(
		domainsecurity.TurnSecurityContextInput{
			ThreadID:          "thread-host-selection",
			TurnID:            "turn-host-selection",
			WorkspaceRealPath: "/workspace/host-selection",
			CaseID:            caseID,
			CaseBindingHash: domainsecurity.SHA256Hex(
				[]byte("host-selection-binding-" + caseID),
			),
			ContextEpoch: epoch,
			IssuedAt: time.Date(
				2026, 7, 29, 10, 0, 0, 0, time.UTC,
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
