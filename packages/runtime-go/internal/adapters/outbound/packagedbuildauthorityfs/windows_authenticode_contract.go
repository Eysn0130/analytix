package packagedbuildauthorityfs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	windowsAuthenticodeContractV1 = "analytix.windows-authenticode-proof/v1"
	maxWindowsAuthenticodeBytesV1 = 64 << 10
)

type windowsAuthenticodeSignatureV1 struct {
	Path             string `json:"path"`
	Status           string `json:"status"`
	SignatureType    string `json:"signatureType"`
	SignerThumbprint string `json:"signerThumbprint"`
	SignerSubject    string `json:"signerSubject"`
}

type windowsAuthenticodeProofV1 struct {
	SchemaVersion int                            `json:"schemaVersion"`
	Contract      string                         `json:"contract"`
	Application   windowsAuthenticodeSignatureV1 `json:"application"`
	RuntimeServer windowsAuthenticodeSignatureV1 `json:"runtimeServer"`
}

// parseWindowsAuthenticodeProofV1 verifies the OS trust result for both signed
// executables and requires one signer identity. It deliberately does not turn
// that identity into a package-resource authority: Authenticode on these two
// executables does not cover the sibling authority JSON or plugin tree.
func parseWindowsAuthenticodeProofV1(body []byte, applicationPath, runtimeServerPath string) (windowsAuthenticodeProofV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxWindowsAuthenticodeBytesV1,
		MaxDepth:       4,
		MaxTokens:      64,
		MaxStringBytes: 16 << 10,
	}); err != nil {
		return windowsAuthenticodeProofV1{}, errors.New("Windows Authenticode proof JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var proof windowsAuthenticodeProofV1
	if err := decoder.Decode(&proof); err != nil {
		return windowsAuthenticodeProofV1{}, errors.New("Windows Authenticode proof shape is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return windowsAuthenticodeProofV1{}, errors.New("Windows Authenticode proof contains trailing JSON")
	}
	if proof.SchemaVersion != 1 || proof.Contract != windowsAuthenticodeContractV1 ||
		!validWindowsAuthenticodeSignatureV1(proof.Application, applicationPath) ||
		!validWindowsAuthenticodeSignatureV1(proof.RuntimeServer, runtimeServerPath) ||
		!strings.EqualFold(proof.Application.SignerThumbprint, proof.RuntimeServer.SignerThumbprint) ||
		proof.Application.SignerSubject != proof.RuntimeServer.SignerSubject {
		return windowsAuthenticodeProofV1{}, errors.New("Windows Authenticode proof is invalid")
	}
	return proof, nil
}

func validWindowsAuthenticodeSignatureV1(value windowsAuthenticodeSignatureV1, expectedPath string) bool {
	return value.Path == expectedPath && value.Status == "Valid" && value.SignatureType == "Authenticode" &&
		validWindowsCertificateThumbprintV1(value.SignerThumbprint) &&
		value.SignerSubject != "" && value.SignerSubject == strings.TrimSpace(value.SignerSubject)
}

func validWindowsCertificateThumbprintV1(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}
