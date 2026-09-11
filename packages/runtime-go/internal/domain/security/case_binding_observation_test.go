package security

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCaseBindingObservationV1RoundTripAndClosedJSON(t *testing.T) {
	observation, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace", State: CaseBindingStateValid, CaseID: "case-1234",
		BindingSHA256: SHA256Hex([]byte("binding")), CaseBindingHash: SHA256Hex([]byte("context")),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := CaseBindingObservationV1Bytes(observation)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCaseBindingObservationV1(body)
	if err != nil || parsed != observation {
		t.Fatalf("round trip mismatch: parsed=%#v err=%v", parsed, err)
	}

	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["safeToAnswer"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseCaseBindingObservationV1(unknown); err == nil {
		t.Fatal("unknown observation property was accepted")
	}
	duplicate := strings.Replace(string(body), `"state":"valid"`, `"state":"valid","state":"missing"`, 1)
	if _, err := ParseCaseBindingObservationV1([]byte(duplicate)); err == nil {
		t.Fatal("duplicate observation property was accepted")
	}
	if _, err := ParseCaseBindingObservationV1(append(body, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing observation JSON was accepted")
	}
}

func TestCaseBindingObservationV1LegalStateShapes(t *testing.T) {
	states := []string{
		CaseBindingStateNotApplicable, CaseBindingStateMissing, CaseBindingStateUnreadable,
		CaseBindingStateUnstable, CaseBindingStateWorkspaceMissing,
	}
	for _, state := range states {
		t.Run(state, func(t *testing.T) {
			if _, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{WorkspaceRealPath: "/workspace", State: state}); err != nil {
				t.Fatalf("legal %s observation rejected: %v", state, err)
			}
		})
	}
	if _, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace", State: CaseBindingStateInvalid, BindingSHA256: SHA256Hex([]byte("invalid-body")),
	}); err != nil {
		t.Fatalf("invalid content observation with raw digest rejected: %v", err)
	}

	invalid := []CaseBindingObservationInputV1{
		{WorkspaceRealPath: "/workspace", State: CaseBindingStateValid},
		{WorkspaceRealPath: "/workspace", State: CaseBindingStateMissing, CaseID: "case-forged"},
		{WorkspaceRealPath: "/workspace", State: CaseBindingStateInvalid, CaseBindingHash: SHA256Hex([]byte("forged"))},
		{WorkspaceRealPath: "/workspace", State: CaseBindingStateChangedUnaccepted},
		{WorkspaceRealPath: "", State: CaseBindingStateMissing},
	}
	for index, input := range invalid {
		if _, err := NewCaseBindingObservationV1(input); err == nil {
			t.Fatalf("invalid observation %d was accepted", index)
		}
	}
}

func TestCaseBindingObservationDigestBindsEveryField(t *testing.T) {
	base, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{WorkspaceRealPath: "/workspace", State: CaseBindingStateMissing})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{WorkspaceRealPath: "/workspace/other", State: CaseBindingStateMissing})
	if err != nil {
		t.Fatal(err)
	}
	if base.ObservationDigest == changed.ObservationDigest {
		t.Fatal("observation digest did not bind workspace path")
	}
	tampered := base
	tampered.State = CaseBindingStateUnreadable
	if ValidateCaseBindingObservationV1(tampered) == nil {
		t.Fatal("tampered observation retained authority")
	}
}
