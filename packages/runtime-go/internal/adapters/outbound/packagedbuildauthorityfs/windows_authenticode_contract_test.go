package packagedbuildauthorityfs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWindowsAuthenticodeProofRequiresValidSameSignerExecutables(t *testing.T) {
	applicationPath := `C:\Program Files\Analytix\analytix.exe`
	runtimePath := `C:\Program Files\Analytix\resources\runtime-go\bin\runtime-server.exe`
	proof := windowsAuthenticodeProofV1{
		SchemaVersion: 1,
		Contract:      windowsAuthenticodeContractV1,
		Application: windowsAuthenticodeSignatureV1{
			Path: applicationPath, Status: "Valid", SignatureType: "Authenticode",
			SignerThumbprint: strings.Repeat("a", 40), SignerSubject: "CN=Analytix Release",
		},
		RuntimeServer: windowsAuthenticodeSignatureV1{
			Path: runtimePath, Status: "Valid", SignatureType: "Authenticode",
			SignerThumbprint: strings.Repeat("A", 40), SignerSubject: "CN=Analytix Release",
		},
	}
	body, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseWindowsAuthenticodeProofV1(body, applicationPath, runtimePath); err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}

	for name, mutate := range map[string]func(*windowsAuthenticodeProofV1){
		"invalid status": func(value *windowsAuthenticodeProofV1) { value.RuntimeServer.Status = "HashMismatch" },
		"different signer": func(value *windowsAuthenticodeProofV1) {
			value.RuntimeServer.SignerThumbprint = strings.Repeat("b", 40)
		},
		"different subject": func(value *windowsAuthenticodeProofV1) { value.RuntimeServer.SignerSubject = "CN=Other" },
		"wrong path":        func(value *windowsAuthenticodeProofV1) { value.RuntimeServer.Path += ".forged" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := proof
			mutate(&candidate)
			candidateBody, marshalErr := json.Marshal(candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, parseErr := parseWindowsAuthenticodeProofV1(candidateBody, applicationPath, runtimePath); parseErr == nil {
				t.Fatal("invalid proof accepted")
			}
		})
	}
}

func TestWindowsAuthenticodeProofRejectsDuplicateAndUnknownFields(t *testing.T) {
	applicationPath := `C:\Analytix\analytix.exe`
	runtimePath := `C:\Analytix\resources\runtime-go\bin\runtime-server.exe`
	valid := `{"schemaVersion":1,"contract":"` + windowsAuthenticodeContractV1 + `","application":{"path":"C:\\Analytix\\analytix.exe","status":"Valid","signatureType":"Authenticode","signerThumbprint":"` + strings.Repeat("a", 40) + `","signerSubject":"CN=Analytix Release"},"runtimeServer":{"path":"C:\\Analytix\\resources\\runtime-go\\bin\\runtime-server.exe","status":"Valid","signatureType":"Authenticode","signerThumbprint":"` + strings.Repeat("a", 40) + `","signerSubject":"CN=Analytix Release"}}`
	duplicate := strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)
	if _, err := parseWindowsAuthenticodeProofV1([]byte(duplicate), applicationPath, runtimePath); err == nil {
		t.Fatal("duplicate-key proof accepted")
	}
	unknown := strings.Replace(valid, `"contract":`, `"unknown":true,"contract":`, 1)
	if _, err := parseWindowsAuthenticodeProofV1([]byte(unknown), applicationPath, runtimePath); err == nil {
		t.Fatal("unknown-field proof accepted")
	}
}
