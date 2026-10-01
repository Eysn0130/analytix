//go:build darwin

package packagedbuildauthorityfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

const packageAnchorOutputLimitV2 = 64 << 10
const packagedBundleIdentifierV2 = "com.analytix.desktop"
const packageAnchorVerificationBudgetV2 = 60 * time.Second
const packageAnchorDevelopmentVerificationBudgetV2 = 180 * time.Second

type packageAnchorCodesignRunnerV2 func(context.Context, ...string) ([]byte, error)

func verifyPackageAnchorV2(ctx context.Context, resources string, authority domainauthority.ParsedAuthorityV2) (string, error) {
	return verifyPackageAnchorWithRunnerV2(ctx, resources, authority, runBoundedCodesignV2)
}

func verifyPackageAnchorWithRunnerV2(
	ctx context.Context,
	resources string,
	authority domainauthority.ParsedAuthorityV2,
	runCodesign packageAnchorCodesignRunnerV2,
) (string, error) {
	if ctx == nil || ctx.Err() != nil {
		return "", errors.New("packaged resource-seal verification was canceled")
	}
	if runCodesign == nil {
		return "", errors.New("packaged resource-seal verifier is unavailable")
	}
	appBundle := filepath.Dir(filepath.Dir(resources))
	if !strings.HasSuffix(filepath.Base(appBundle), ".app") {
		return "", errors.New("packaged resource seal is outside a macOS app bundle")
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, packageAnchorBudgetV2(authority))
	defer cancel()
	arguments := []string{"--verify", "--deep", "--strict", "--verbose=2"}
	anchor := "macos_nonpublishable_resource_seal"
	if authority.DeveloperIDTeam() != "" {
		team := authority.DeveloperIDTeam()
		arguments = append(arguments, "-R="+developerIDRequirementV2(team))
		anchor = "macos_developer_id_resource_seal"
	}
	arguments = append(arguments, appBundle)
	if _, err := runCodesign(deadlineCtx, arguments...); err != nil {
		return "", errors.New("macOS packaged resource seal is invalid")
	}
	metadata, err := runCodesign(deadlineCtx, "--display", "--verbose=4", appBundle)
	if err != nil {
		return "", errors.New("macOS packaged signature metadata is unavailable")
	}
	if metadataValueV2(metadata, "Identifier") != packagedBundleIdentifierV2 {
		return "", errors.New("macOS packaged bundle identifier is invalid")
	}
	expectedRunner := filepath.Join(filepath.Dir(resources), "MacOS", "analytix")
	if metadataValueV2(metadata, "Executable") != expectedRunner {
		return "", errors.New("macOS packaged application runner identity is invalid")
	}
	if authority.DeveloperIDTeam() != "" {
		team := authority.DeveloperIDTeam()
		if metadataValueV2(metadata, "TeamIdentifier") != team || metadataValueV2(metadata, "Timestamp") == "" ||
			!hasMetadataPrefixV2(metadata, "Authority", "Developer ID Application:") || !hasHardenedRuntimeFlagV2(metadata) {
			return "", errors.New("macOS packaged Developer ID identity is invalid")
		}
	} else if metadataValueV2(metadata, "Signature") != "adhoc" || metadataValueV2(metadata, "TeamIdentifier") != "not set" || hasMetadataPrefixV2(metadata, "Authority", "") {
		return "", errors.New("macOS nonpublishable package is not ad-hoc signed")
	}
	return anchor, nil
}

func packageAnchorBudgetV2(authority domainauthority.ParsedAuthorityV2) time.Duration {
	if authority.Development != nil && authority.Controlled == nil &&
		authority.Development.Kind == domainauthority.DevelopmentDispositionKindV2 &&
		authority.DispositionKind() == domainauthority.DevelopmentDispositionKindV2 {
		return packageAnchorDevelopmentVerificationBudgetV2
	}
	return packageAnchorVerificationBudgetV2
}

func runBoundedCodesignV2(ctx context.Context, arguments ...string) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, context.Canceled
	}
	var output boundedBufferV2
	command := exec.CommandContext(ctx, "/usr/bin/codesign", arguments...)
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil || output.overflow || ctx.Err() != nil {
		return nil, errors.New("codesign verification failed")
	}
	return append([]byte(nil), output.body.Bytes()...), nil
}

type boundedBufferV2 struct {
	body     bytes.Buffer
	overflow bool
}

func (buffer *boundedBufferV2) Write(body []byte) (int, error) {
	if buffer == nil {
		return 0, errors.New("codesign output buffer is unavailable")
	}
	written := len(body)
	remaining := packageAnchorOutputLimitV2 - buffer.body.Len()
	if remaining <= 0 {
		buffer.overflow = true
		return written, nil
	}
	if len(body) > remaining {
		buffer.overflow = true
		body = body[:remaining]
	}
	_, _ = buffer.body.Write(body)
	return written, nil
}

func developerIDRequirementV2(team string) string {
	return fmt.Sprintf(
		`identifier "%s" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "%s"`,
		packagedBundleIdentifierV2,
		team,
	)
}

func metadataValueV2(body []byte, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func hasMetadataPrefixV2(body []byte, key, valuePrefix string) bool {
	prefix := key + "="
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, prefix) && strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(line, prefix)), valuePrefix) {
			return true
		}
	}
	return false
}

func hasHardenedRuntimeFlagV2(body []byte) bool {
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.Contains(line, "flags=0x") {
			continue
		}
		start := strings.Index(line, "flags=0x") + len("flags=0x")
		end := start
		for end < len(line) && ((line[end] >= '0' && line[end] <= '9') || (line[end] >= 'a' && line[end] <= 'f') || (line[end] >= 'A' && line[end] <= 'F')) {
			end++
		}
		if end == start {
			return false
		}
		var flags uint64
		if _, err := fmt.Sscanf(line[start:end], "%x", &flags); err != nil {
			return false
		}
		return flags&0x10000 != 0 || strings.Contains(line[end:], "runtime")
	}
	return false
}
