//go:build darwin

package nativecomponentregistry

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	processauthority "analytix.local/runtime-go/internal/adapters/outbound/processauthority"

	"golang.org/x/sys/unix"
)

const (
	darwinVerificationDeadline = 8 * time.Second
	darwinCommandOutputLimit   = 1024 * 1024
	darwinCDHashHexLength      = 40
)

var darwinCodeFlagsPattern = regexp.MustCompile(`flags=0x([0-9a-fA-F]+)(?:\(([^)]*)\))?`)

type PlatformVerifierConfig struct {
	WorkingDirectory          string
	WorkingDirectoryAuthority *os.File
	StagingRoot               string
	StagingRootAuthority      *os.File
	Policy                    PlatformPolicy
}

type darwinPlatformVerifier struct {
	workingDirectory          string
	workingDirectoryAuthority *os.File
	stagingRoot               string
	stagingRootAuthority      *os.File
	policy                    PlatformPolicy
}

func NewPlatformVerifier(config PlatformVerifierConfig) (PlatformVerifier, error) {
	if strings.TrimSpace(config.WorkingDirectory) == "" || config.WorkingDirectoryAuthority == nil ||
		strings.TrimSpace(config.StagingRoot) == "" || config.StagingRootAuthority == nil ||
		filepath.Clean(config.WorkingDirectory) == filepath.Clean(config.StagingRoot) ||
		!validPlatformPolicy(config.Policy) {
		return nil, ErrTrustInvalid
	}
	workingDirectory, err := openPinnedObject(config.WorkingDirectory, true)
	if err != nil {
		return nil, ErrTrustInvalid
	}
	defer workingDirectory.file.Close()
	root, err := openPinnedObject(config.StagingRoot, true)
	if err != nil {
		return nil, ErrTrustInvalid
	}
	defer root.file.Close()
	if !pinnedDarwinVerificationRoot(workingDirectory) ||
		!darwinDirectoryAuthorityMatches(workingDirectory, config.WorkingDirectoryAuthority) ||
		!pinnedDarwinVerificationRoot(root) ||
		!darwinDirectoryAuthorityMatches(root, config.StagingRootAuthority) {
		return nil, ErrTrustInvalid
	}
	return &darwinPlatformVerifier{
		workingDirectory:          filepath.Clean(config.WorkingDirectory),
		workingDirectoryAuthority: config.WorkingDirectoryAuthority,
		stagingRoot:               filepath.Clean(config.StagingRoot),
		stagingRootAuthority:      config.StagingRootAuthority,
		policy:                    config.Policy,
	}, nil
}

func validPlatformPolicy(policy PlatformPolicy) bool {
	if !validSHA256(policy.SHA256) {
		return false
	}
	switch policy.Mode {
	case "ad-hoc":
		return policy.AppleTeamIdentifier == ""
	case "developer-id":
		return validAppleTeamIdentifier(policy.AppleTeamIdentifier)
	default:
		return false
	}
}

func (verifier *darwinPlatformVerifier) Policy() PlatformPolicy {
	if verifier == nil {
		return PlatformPolicy{}
	}
	return verifier.policy
}

func (verifier *darwinPlatformVerifier) Verify(path string, opened *os.File, component ComponentReceipt) error {
	if verifier == nil || opened == nil || filepath.Base(path) != component.BinaryName ||
		!validPlatformPolicy(verifier.policy) {
		return ErrComponentInvalid
	}
	root, err := openPinnedObject(verifier.stagingRoot, true)
	if err != nil {
		return ErrComponentInvalid
	}
	defer root.file.Close()
	if !pinnedDarwinVerificationRoot(root) || !darwinDirectoryAuthorityMatches(root, verifier.stagingRootAuthority) {
		return ErrComponentInvalid
	}
	openedInfo, err := opened.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() <= 0 || openedInfo.Size() > MaxNativeBinaryBytes {
		return ErrComponentInvalid
	}
	currentSHA256, err := fullSHA256(opened, openedInfo.Size())
	if err != nil {
		return ErrComponentInvalid
	}
	deadline := time.Now().Add(darwinVerificationDeadline)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	err = processauthority.InspectSuspendedDarwinExecutable(
		ctx,
		processauthority.SuspendedInspectionConfig{
			Executable: opened, ExpectedExecutableSHA256: currentSHA256,
			ExpectedExecutableSize: openedInfo.Size(), StagingRoot: verifier.stagingRoot,
			StagingRootAuthority: verifier.stagingRootAuthority,
		},
		func(_ context.Context, identity processauthority.SuspendedProcessIdentity) error {
			pidTarget := strconv.Itoa(identity.PID)
			dynamicPIDTarget := "+" + pidTarget
			verifyArguments := []string{"--verify", "--strict", "--verbose=2"}
			if verifier.policy.Mode == "developer-id" {
				verifyArguments = append(verifyArguments, "-R="+darwinDeveloperIDRequirement(verifier.policy.AppleTeamIdentifier))
			}
			verifyArguments = append(verifyArguments, pidTarget)
			if _, commandErr := verifier.runDarwinVerificationCommand(deadline, verifyArguments...); commandErr != nil {
				return ErrComponentInvalid
			}
			metadata, commandErr := verifier.runDarwinVerificationCommand(
				deadline, "--display", "--verbose=4", dynamicPIDTarget,
			)
			if commandErr != nil || validateDarwinSignatureMetadata(metadata, verifier.policy, identity.CDHash) != nil {
				return ErrComponentInvalid
			}
			entitlementOutput, commandErr := verifier.runDarwinVerificationCommand(
				deadline, "--display", "--entitlements", ":-", dynamicPIDTarget,
			)
			if commandErr != nil || validateEmptyDarwinEntitlements(entitlementOutput) != nil {
				return ErrComponentInvalid
			}
			return nil
		},
	)
	if err != nil || !pinnedDarwinVerificationRoot(root) ||
		!darwinDirectoryAuthorityMatches(root, verifier.stagingRootAuthority) {
		return ErrComponentInvalid
	}
	return nil
}

func darwinDirectoryAuthorityMatches(opened *pinnedObject, authority *os.File) bool {
	if opened == nil || opened.file == nil || authority == nil {
		return false
	}
	var current unix.Stat_t
	return unix.Fstat(int(authority.Fd()), &current) == nil && current.Mode&unix.S_IFMT == unix.S_IFDIR &&
		current.Uid == uint32(os.Geteuid()) && current.Mode&0o077 == 0 && current.Dev == opened.stat.Dev &&
		current.Ino == opened.stat.Ino && current.Mode == opened.stat.Mode && current.Uid == opened.stat.Uid
}

func pinnedDarwinVerificationRoot(root *pinnedObject) bool {
	if root == nil || root.file == nil || root.stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		root.stat.Uid != uint32(os.Geteuid()) || root.stat.Mode&0o077 != 0 {
		return false
	}
	var current unix.Stat_t
	return unix.Fstat(int(root.file.Fd()), &current) == nil && current.Dev == root.stat.Dev &&
		current.Ino == root.stat.Ino && current.Mode == root.stat.Mode && current.Uid == root.stat.Uid
}

func darwinDeveloperIDRequirement(teamIdentifier string) string {
	return fmt.Sprintf(
		`anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "%s"`,
		teamIdentifier,
	)
}

func (verifier *darwinPlatformVerifier) runDarwinVerificationCommand(
	deadline time.Time,
	arguments ...string,
) ([]byte, error) {
	if verifier == nil || verifier.workingDirectoryAuthority == nil || verifier.stagingRootAuthority == nil {
		return nil, ErrComponentInvalid
	}
	if time.Until(deadline) <= 0 {
		return nil, context.DeadlineExceeded
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	result, err := processauthority.ExecuteSystemTool(ctx, processauthority.SystemToolRequest{
		Tool:                      processauthority.SystemToolDarwinCodeSign,
		Arguments:                 append([]string(nil), arguments...),
		WorkingDirectory:          verifier.workingDirectory,
		WorkingDirectoryAuthority: verifier.workingDirectoryAuthority,
		Timeout:                   time.Until(deadline),
	})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || result.ExitCode != 0 || result.TerminationStatus != processauthority.TerminationExited {
		return nil, ErrComponentInvalid
	}
	output := make([]byte, 0, len(result.Stdout)+len(result.Stderr))
	output = append(output, result.Stdout...)
	output = append(output, result.Stderr...)
	if len(output) > darwinCommandOutputLimit {
		return nil, ErrComponentInvalid
	}
	return output, nil
}

func validateDarwinSignatureMetadata(raw []byte, policy PlatformPolicy, loadedCDHash string) error {
	if len(raw) == 0 || len(raw) > darwinCommandOutputLimit || !validPlatformPolicy(policy) ||
		len(loadedCDHash) != darwinCDHashHexLength || strings.ToLower(loadedCDHash) != loadedCDHash {
		return ErrComponentInvalid
	}
	text := string(raw)
	flags := darwinCodeFlagsPattern.FindStringSubmatch(text)
	if len(flags) < 2 {
		return ErrComponentInvalid
	}
	numericFlags, err := strconv.ParseUint(flags[1], 16, 64)
	if err != nil || numericFlags&0x10000 == 0 || metadataLine(text, "Format") != "pid diskrep" ||
		metadataLine(text, "CDHash") != loadedCDHash {
		return ErrComponentInvalid
	}
	team := metadataLine(text, "TeamIdentifier")
	authorities := metadataLines(text, "Authority")
	switch policy.Mode {
	case "ad-hoc":
		if metadataLine(text, "Signature") != "adhoc" || team != "not set" || len(authorities) != 0 || numericFlags&0x2 == 0 {
			return ErrComponentInvalid
		}
	case "developer-id":
		if team != policy.AppleTeamIdentifier || metadataLine(text, "Timestamp") == "" {
			return ErrComponentInvalid
		}
		found := false
		for _, authority := range authorities {
			if strings.HasPrefix(authority, "Developer ID Application:") {
				found = true
			}
		}
		if !found {
			return ErrComponentInvalid
		}
	default:
		return ErrComponentInvalid
	}
	return nil
}

func metadataLine(text string, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func metadataLines(text string, key string) []string {
	prefix := key + "="
	var values []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			values = append(values, strings.TrimSpace(strings.TrimPrefix(line, prefix)))
		}
	}
	return values
}

func validateEmptyDarwinEntitlements(raw []byte) error {
	if len(raw) > darwinCommandOutputLimit {
		return ErrComponentInvalid
	}
	start := bytes.Index(raw, []byte("<?xml"))
	if start < 0 {
		return nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw[start:]))
	dictDepth := -1
	depth := 0
	seenDict := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ErrComponentInvalid
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			if value.Name.Local == "dict" {
				if seenDict {
					return ErrComponentInvalid
				}
				seenDict = true
				dictDepth = depth
			} else if seenDict && depth > dictDepth {
				return ErrComponentInvalid
			}
		case xml.CharData:
			if seenDict && depth >= dictDepth && strings.TrimSpace(string(value)) != "" {
				return ErrComponentInvalid
			}
		case xml.EndElement:
			depth--
			if depth < 0 {
				return ErrComponentInvalid
			}
		}
	}
	if !seenDict || depth != 0 {
		return ErrComponentInvalid
	}
	return nil
}
