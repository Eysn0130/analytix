package continuation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestUserInputSubmissionDispositionRoundTripPreservesOriginalValues(t *testing.T) {
	now := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	payload, private := continuationPayloadFixture(t, KindUserInput, "request_user_input", "not_required", now)
	public := private.Public().(ed25519.PublicKey)
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil }
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(public), public, sign)
	if err != nil {
		t.Fatal(err)
	}
	answers := json.RawMessage(fmt.Sprintf(`[{"id":%q,"kind":"answer","optionIds":[],"customText":"  原值\n  "},{"id":%q,"kind":"skip"}]`, payload.GateID+"_1", payload.GateID+"_2"))
	disposition, err := NewUserInputSubmittedDisposition(receipt, UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: answers}, now.Add(time.Minute), domainsecurity.SHA256Hex(public), public, sign)
	if err != nil {
		t.Fatal(err)
	}
	body, err := DispositionBytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDisposition(body)
	if err != nil || parsed.UserInputSubmission == nil {
		t.Fatal("private submission did not round trip", err)
	}
	var records []UserInputStructuredAnswerV1
	if err := json.Unmarshal(parsed.UserInputSubmission.Answers, &records); err != nil {
		t.Fatal(err)
	}
	if records[0].OptionIDs == nil || len(*records[0].OptionIDs) != 0 || records[0].CustomText == nil || *records[0].CustomText != "  原值\n  " || records[1].Kind != "skip" || records[1].OptionIDs != nil || records[1].CustomText != nil {
		t.Fatal("private empty-list, whitespace, or skip values changed")
	}
	_, _, signature, err := DispositionAuthorityMaterial(parsed)
	if err != nil || !ed25519.Verify(public, DispositionSigningBytes(parsed), signature) {
		t.Fatal("private submission lost disposition signature")
	}
	parsed.UserInputSubmission = func() *UserInputSubmissionV1 {
		copy := CloneUserInputSubmissionV1(*parsed.UserInputSubmission)
		copy.Answers = bytes.Replace(copy.Answers, []byte("原值"), []byte("伪造"), 1)
		return &copy
	}()
	parsed.DispositionID = dispositionDigest(parsed)
	if ed25519.Verify(public, DispositionSigningBytes(parsed), signature) {
		t.Fatal("changed answers acquired the original signature")
	}
}

func TestUserInputSubmissionRejectsOpenOrAmbiguousCarriers(t *testing.T) {
	valid := `{"schemaVersion":1,"format":"structured","answers":[{"id":"input_host_1","kind":"answer","optionIds":[],"customText":" value "}]}`
	for name, body := range map[string]string{
		"unknown payload field":   strings.TrimSuffix(valid, "}") + `,"extra":true}`,
		"duplicate payload field": strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		"duplicate answer field":  strings.Replace(valid, `"kind":"answer"`, `"kind":"answer","kind":"answer"`, 1),
		"unknown answer field":    strings.Replace(valid, `"optionIds":[]`, `"optionIds":[],"extra":"private"`, 1),
		"missing list":            strings.Replace(valid, `"optionIds":[],`, "", 1),
		"null list":               strings.Replace(valid, `"optionIds":[]`, `"optionIds":null`, 1),
		"blank custom":            strings.Replace(valid, `" value "`, `"  "`, 1),
		"skip contents":           strings.Replace(valid, `"kind":"answer"`, `"kind":"skip"`, 1),
		"unknown version":         strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":2`, 1),
		"mixed legacy":            strings.Replace(valid, `"optionIds":[]`, `"optionIds":[],"questionId":"input_host_1"`, 1),
		"duplicate question":      `{"schemaVersion":1,"format":"structured","answers":[{"id":"input_host_1","kind":"skip"},{"id":"input_host_1","kind":"skip"}]}`,
		"duplicate option":        `{"schemaVersion":1,"format":"structured","answers":[{"id":"input_host_1","kind":"answer","optionIds":["option","option"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var submission UserInputSubmissionV1
			if json.Unmarshal([]byte(body), &submission) == nil {
				t.Fatal("invalid private carrier accepted")
			}
		})
	}
	var validSubmission UserInputSubmissionV1
	if json.Unmarshal([]byte(valid), &validSubmission) != nil {
		t.Fatal("valid source values rejected")
	}
	invalidUTF8 := []byte(valid)
	invalidUTF8[bytes.Index(invalidUTF8, []byte("value"))] = 0xff
	if json.Unmarshal(invalidUTF8, &validSubmission) == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestUserInputSubmissionOnlyBelongsToSubmittedCurrentQuestionDisposition(t *testing.T) {
	now := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	payload, private := continuationPayloadFixture(t, KindUserInput, "request_user_input", "not_required", now)
	public := private.Public().(ed25519.PublicKey)
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil }
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(public), public, sign)
	if err != nil {
		t.Fatal(err)
	}
	submission := UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: json.RawMessage(fmt.Sprintf(`[{"id":%q,"kind":"skip"}]`, payload.GateID+"_1"))}
	original, err := NewUserInputSubmittedDisposition(receipt, submission, now.Add(time.Minute), domainsecurity.SHA256Hex(public), public, sign)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Disposition){
		"legacy version": func(value *Disposition) { value.Version = ContractVersionV2 },
		"cancelled":      func(value *Disposition) { value.Status = StatusCancelled; value.ReasonCode = "user_input_cancelled" },
		"approval":       func(value *Disposition) { value.Kind = KindApproval },
		"foreign question": func(value *Disposition) {
			copy := CloneUserInputSubmissionV1(*value.UserInputSubmission)
			copy.Answers = json.RawMessage(`[{"id":"input_foreign_1","kind":"skip"}]`)
			value.UserInputSubmission = &copy
		},
		"cross question option": func(value *Disposition) {
			copy := CloneUserInputSubmissionV1(*value.UserInputSubmission)
			copy.Answers = json.RawMessage(fmt.Sprintf(`[{"id":%q,"kind":"answer","optionIds":[%q]}]`, payload.GateID+"_1", payload.GateID+"_2_option_1"))
			value.UserInputSubmission = &copy
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := original
			change(&value)
			value.DispositionID = dispositionDigest(value)
			if ValidateDisposition(value) == nil {
				t.Fatal("invalid private disposition accepted")
			}
		})
	}
	body, _ := DispositionBytes(original)
	nullBody := bytes.Replace(body, []byte(`"userInputSubmission":`+string(mustSubmissionJSON(t, submission))), []byte(`"userInputSubmission":null`), 1)
	if bytes.Equal(nullBody, body) {
		t.Fatal("null fixture did not replace payload")
	}
	if _, err := ParseDisposition(nullBody); err == nil {
		t.Fatal("explicit null private payload accepted")
	}
}

func mustSubmissionJSON(t *testing.T, submission UserInputSubmissionV1) []byte {
	t.Helper()
	body, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestUserInputAbsentSubmissionKeepsLegacyV2V3BytesAndSignatures(t *testing.T) {
	now := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	payload, private := continuationPayloadFixture(t, KindUserInput, "request_user_input", "not_required", now)
	public := private.Public().(ed25519.PublicKey)
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(public), public, func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{ContractVersionV2, ContractVersionV3} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			value, err := NewDisposition(receipt, StatusSubmitted, "user_input_submitted", now.Add(time.Minute), domainsecurity.SHA256Hex(public), public, func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil })
			if err != nil {
				t.Fatal(err)
			}
			value.Version = version
			value.DispositionID = dispositionDigest(value)
			value.AuthoritySignature = ""
			// This is the pre-extension schema, including its original field order.
			legacy := struct {
				Version            int    `json:"version"`
				DispositionID      string `json:"dispositionId"`
				GateID             string `json:"gateId"`
				ReceiptID          string `json:"receiptId"`
				Kind               string `json:"kind"`
				Status             string `json:"status"`
				ReasonCode         string `json:"reasonCode"`
				DisposedAt         string `json:"disposedAt"`
				AuthorityAlgorithm string `json:"authorityAlgorithm"`
				AuthorityKeyID     string `json:"authorityKeyId"`
				AuthorityPublicKey string `json:"authorityPublicKey"`
				AuthoritySignature string `json:"authoritySignature"`
			}{value.Version, value.DispositionID, value.GateID, value.ReceiptID, value.Kind, value.Status, value.ReasonCode, value.DisposedAt, value.AuthorityAlgorithm, value.AuthorityKeyID, value.AuthorityPublicKey, ""}
			unsigned, _ := json.Marshal(legacy)
			signing := append([]byte(dispositionSigningDomainForVersion(version)), unsigned...)
			if !bytes.Equal(signing, DispositionSigningBytes(value)) {
				t.Fatal("old signing bytes changed")
			}
			signature := ed25519.Sign(private, signing)
			value.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
			legacy.AuthoritySignature = value.AuthoritySignature
			want, _ := json.Marshal(legacy)
			got, err := DispositionBytes(value)
			if err != nil || !bytes.Equal(want, got) || bytes.Contains(got, []byte("userInputSubmission")) {
				t.Fatal("old stored bytes changed")
			}
			parsed, err := ParseDisposition(want)
			if err != nil || parsed.UserInputSubmission != nil || !ed25519.Verify(public, DispositionSigningBytes(parsed), signature) {
				t.Fatal("old disposition became unreadable")
			}
		})
	}
}

func TestUserInputSubmissionRejectsEscapedWireExpansionBeforeSigning(t *testing.T) {
	now := time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC)
	payload, private := continuationPayloadFixture(t, KindUserInput, "request_user_input", "not_required", now)
	public := private.Public().(ed25519.PublicKey)
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(public), public, func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil })
	if err != nil {
		t.Fatal(err)
	}
	answers := json.RawMessage(fmt.Sprintf(`[{"id":%q,"kind":"answer","optionIds":[],"customText":"%s"}]`, payload.GateID+"_1", strings.Repeat("<", 750_000)))
	wire, err := json.Marshal(answers)
	if err != nil || len(answers) >= MaxUserInputSubmissionBytesV1 || len(wire) <= MaxUserInputSubmissionBytesV1 {
		t.Fatal("fixture did not cross the escaped wire bound", err)
	}
	submission := UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: answers}
	if ValidateUserInputSubmissionV1(submission) == nil {
		t.Fatal("escaped source could exceed its own signed record reader")
	}
	signCalls := 0
	if _, err := NewUserInputSubmittedDisposition(receipt, submission, now.Add(time.Minute), domainsecurity.SHA256Hex(public), public, func(body []byte) ([]byte, error) {
		signCalls++
		return ed25519.Sign(private, body), nil
	}); err == nil || signCalls != 0 {
		t.Fatal("oversized wire capture reached signing")
	}
}
