//go:build darwin || linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	formalauthority "analytix.local/runtime-go/internal/formalauthority"
)

const bootstrapPurposeV1 = "analytix.runtime-main-owned-authority/v1"
const bootstrapFileNameV1 = "authority-bootstrap-v1.json"
const readyMarkerV1 = "ANALYTIX_FORMAL_AUTHORITY_READY_V1 "

type bootstrapV1 struct {
	SchemaVersion                  int    `json:"schemaVersion"`
	Purpose                        string `json:"purpose"`
	AuthorityAnchorV1              string `json:"authorityAnchorV1"`
	AuthorityManifestRoot          string `json:"authorityManifestRoot"`
	AuthorityCredentialProfileRoot string `json:"authorityCredentialProfileRoot"`
	AuthorityCredentialBundleRoot  string `json:"authorityCredentialBundleRoot"`
}

type readyV1 struct {
	SchemaVersion                  int    `json:"schemaVersion"`
	BootstrapSHA256                string `json:"bootstrapSha256"`
	InstallationAuthorityKeySHA256 string `json:"installationAuthorityKeySha256"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "formal authority sidecar failed closed")
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("formal-authority-sidecar", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ownerRoot := flags.String("owner-root", "", "pre-authorized formal product owner root")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || output == nil {
		return errors.New("formal authority command is invalid")
	}
	root := strings.TrimSpace(*ownerRoot)
	if root == "" || root != *ownerRoot || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("formal authority owner root is invalid")
	}
	if err := prepareFormalOwnerRoot(root); err != nil {
		return err
	}
	service, err := formalauthority.New(root)
	if err != nil {
		return err
	}
	defer service.Close()
	bootstrap := bootstrapV1{
		SchemaVersion:                  1,
		Purpose:                        bootstrapPurposeV1,
		AuthorityAnchorV1:              service.AnchorEnvelope,
		AuthorityManifestRoot:          service.ManifestRoot,
		AuthorityCredentialProfileRoot: service.CredentialProfileRoot,
		AuthorityCredentialBundleRoot:  service.CredentialBundleRoot,
	}
	body, err := json.Marshal(bootstrap)
	if err != nil {
		return err
	}
	bootstrapPath := filepath.Join(root, bootstrapFileNameV1)
	if err := writePrivateExclusive(bootstrapPath, body); err != nil {
		return err
	}
	keyBody, err := os.ReadFile(service.AuthorityPath)
	if err != nil {
		return err
	}
	bootstrapDigest := sha256.Sum256(body)
	keyDigest := sha256.Sum256(keyBody)
	for index := range keyBody {
		keyBody[index] = 0
	}
	ready, err := json.Marshal(readyV1{
		SchemaVersion:                  1,
		BootstrapSHA256:                hex.EncodeToString(bootstrapDigest[:]),
		InstallationAuthorityKeySHA256: hex.EncodeToString(keyDigest[:]),
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "%s%s\n", readyMarkerV1, ready); err != nil {
		return err
	}
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalChannel)
	<-signalChannel
	return nil
}

func prepareFormalOwnerRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Dir(root) == root {
		return errors.New("formal authority owner root is invalid")
	}
	if _, err := os.Lstat(root); err == nil || !errors.Is(err, os.ErrNotExist) {
		return errors.New("formal authority owner root must be fresh")
	}
	parentInput := filepath.Dir(root)
	inputState, inputErr := os.Lstat(parentInput)
	if inputErr != nil || !inputState.IsDir() || inputState.Mode()&os.ModeSymlink != 0 {
		return errors.New("formal authority owner parent is invalid")
	}
	parent, err := filepath.EvalSymlinks(parentInput)
	if err != nil || !filepath.IsAbs(parent) || filepath.Clean(parent) != parent || parent != parentInput {
		return errors.New("formal authority owner parent is invalid")
	}
	state, err := os.Lstat(parent)
	if err != nil || !state.IsDir() || state.Mode()&os.ModeSymlink != 0 ||
		state.Mode().Perm() != 0o700 || state.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("formal authority owner parent is not private")
	}
	currentRoot, err := filepath.Abs(".")
	if err != nil {
		return errors.New("formal authority source root is unavailable")
	}
	currentRoot, err = filepath.EvalSymlinks(currentRoot)
	if err != nil || !filepath.IsAbs(currentRoot) || filepath.Clean(currentRoot) != currentRoot {
		return errors.New("formal authority source root is unavailable")
	}
	for _, protectedRoot := range []string{currentRoot, "/Volumes/AnalytixCache"} {
		realProtected, realErr := filepath.EvalSymlinks(protectedRoot)
		if realErr != nil {
			continue
		}
		if pathContainedByV1(realProtected, root) || pathContainedByV1(root, realProtected) {
			return errors.New("formal authority owner root overlaps protected storage")
		}
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return errors.New("formal authority owner root creation failed")
	}
	state, err = os.Lstat(root)
	if err != nil || !state.IsDir() || state.Mode()&os.ModeSymlink != 0 || state.Mode().Perm() != 0o700 {
		return errors.New("formal authority owner root creation failed")
	}
	return nil
}

func pathContainedByV1(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func writePrivateExclusive(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(body)
	if writeErr == nil && written != len(body) {
		writeErr = io.ErrShortWrite
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}
