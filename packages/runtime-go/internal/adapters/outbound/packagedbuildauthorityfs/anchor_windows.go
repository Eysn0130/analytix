//go:build windows

package packagedbuildauthorityfs

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"unicode/utf16"

	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

const windowsAuthenticodeScriptV1 = `$ErrorActionPreference = 'Stop'
function Read-AnalytixSignature([string]$Path) {
  $signature = Get-AuthenticodeSignature -LiteralPath $Path
  if ($null -eq $signature.SignerCertificate) { throw 'Signer certificate is unavailable.' }
  [ordered]@{
    path = $Path
    status = [string]$signature.Status
    signatureType = [string]$signature.SignatureType
    signerThumbprint = [string]$signature.SignerCertificate.Thumbprint
    signerSubject = [string]$signature.SignerCertificate.Subject
  }
}
[ordered]@{
  schemaVersion = 1
  contract = 'analytix.windows-authenticode-proof/v1'
  application = Read-AnalytixSignature $env:ANALYTIX_AUTHENTICODE_APPLICATION
  runtimeServer = Read-AnalytixSignature $env:ANALYTIX_AUTHENTICODE_RUNTIME
} | ConvertTo-Json -Compress -Depth 4`

func verifyPackageAnchorV2(ctx context.Context, resources string, authority domainauthority.ParsedAuthorityV2) (string, error) {
	if authority.Controlled == nil {
		return "", errors.New("Windows development package has no independent external-resource authority")
	}
	applicationPath := filepath.Join(filepath.Dir(resources), "analytix.exe")
	runtimeServerPath := filepath.Join(resources, "runtime-go", "bin", "runtime-server.exe")
	body, err := readWindowsAuthenticodeProofV1(ctx, applicationPath, runtimeServerPath)
	if err != nil {
		return "", err
	}
	if _, err := parseWindowsAuthenticodeProofV1(body, applicationPath, runtimeServerPath); err != nil {
		return "", err
	}
	// The signer result covers the two executable files only. Authority V2 has
	// no signed catalog or detached signature covering its sibling JSON and the
	// bundled plugin tree, so it must never authorize materialization by itself.
	return "", errors.New("Windows Authenticode is valid but packaged authority v2 has no independently signed external-resource manifest")
}

func readWindowsAuthenticodeProofV1(ctx context.Context, applicationPath, runtimeServerPath string) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("Windows Authenticode verification was canceled")
	}
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" || !filepath.IsAbs(systemRoot) {
		return nil, errors.New("Windows Authenticode verifier is unavailable")
	}
	powershell := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShellCommandV1(windowsAuthenticodeScriptV1))
	command.Env = []string{
		"SystemRoot=" + systemRoot,
		"WINDIR=" + systemRoot,
		"PATH=" + filepath.Join(systemRoot, "System32") + ";" + filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0"),
		"ANALYTIX_AUTHENTICODE_APPLICATION=" + applicationPath,
		"ANALYTIX_AUTHENTICODE_RUNTIME=" + runtimeServerPath,
	}
	stdout := &boundedWindowsOutputV1{limit: maxWindowsAuthenticodeBytesV1}
	command.Stdout = stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil || stdout.overflow || ctx.Err() != nil {
		return nil, errors.New("Windows Authenticode verification failed")
	}
	return bytes.TrimSpace(stdout.buffer.Bytes()), nil
}

func encodePowerShellCommandV1(source string) string {
	encoded := utf16.Encode([]rune(source))
	body := make([]byte, len(encoded)*2)
	for index, value := range encoded {
		body[index*2] = byte(value)
		body[index*2+1] = byte(value >> 8)
	}
	return base64.StdEncoding.EncodeToString(body)
}

type boundedWindowsOutputV1 struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (writer *boundedWindowsOutputV1) Write(body []byte) (int, error) {
	length := len(body)
	remaining := writer.limit - writer.buffer.Len()
	if remaining > 0 {
		if remaining > length {
			remaining = length
		}
		_, _ = writer.buffer.Write(body[:remaining])
	}
	if remaining < length {
		writer.overflow = true
	}
	return length, nil
}
