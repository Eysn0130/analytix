//go:build darwin

package secretstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	keychainSecurityPath      = "/usr/bin/security"
	keychainService           = "com.analytix.desktop.secret-store"
	keychainAccount           = "master-key-v1"
	keychainNotFoundExit      = 44
	darwinAuthorityFile       = "authority.v1"
	darwinExplicitBindingFile = "explicit-task-keychain-binding.v1"
)

var errKeychainItemNotFound = errors.New("secret store: keychain item not found")

type keychainCommandResult struct {
	stdout   []byte
	exitCode int
}

type keychainCommandRunner interface {
	Run(context.Context, []string, []byte) (keychainCommandResult, error)
}

type securityCommandRunner struct {
	keychainDBPath string
	checkUnlocked  func(context.Context, string) bool
}

func (runner securityCommandRunner) Run(ctx context.Context, arguments []string, stdin []byte) (keychainCommandResult, error) {
	// A locked Keychain may wait indefinitely for SecurityAgent interaction.
	// Unlock is an explicit host operation; credential reads/writes must return
	// a bounded failure and preserve any unconfirmed write outcome.
	if ctx == nil || ctx.Err() != nil {
		return keychainCommandResult{}, portsecretstore.ErrMasterKeyUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if runner.keychainDBPath != "" {
		check := runner.checkUnlocked
		if check == nil {
			check = taskKeychainUnlocked
		}
		// Metadata reads must not turn an already locked task Keychain into a
		// background password dialog. Keep the command deadline as a second
		// boundary: the Keychain can still lock after this preflight.
		if !check(ctx, runner.keychainDBPath) {
			return keychainCommandResult{}, portsecretstore.ErrMasterKeyUnavailable
		}
	}
	command := exec.CommandContext(ctx, keychainSecurityPath, arguments...)
	command.WaitDelay = time.Second
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	var stdout bytes.Buffer
	interactiveAdd := len(arguments) == 1 && arguments[0] == "-i"
	if interactiveAdd {
		command.Stdout = io.Discard
	} else {
		command.Stdout = &stdout
	}
	command.Stderr = io.Discard
	err := command.Run()
	result := keychainCommandResult{exitCode: 0}
	if !interactiveAdd {
		result.stdout = bytes.Clone(stdout.Bytes())
	}
	clearBytes(stdout.Bytes())
	if err == nil {
		return result, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		clearBytes(result.stdout)
		return keychainCommandResult{}, errOSCredentialUnavailable
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, portsecretstore.ErrMasterKeyUnavailable
	}
	clearBytes(result.stdout)
	return keychainCommandResult{}, portsecretstore.ErrMasterKeyUnavailable
}

type keychainMasterKeyProvider struct {
	runner                 keychainCommandRunner
	random                 io.Reader
	keychainDBPath         string
	keychainSecurityDigest string
	bindingPath            string
	binding                darwinExplicitKeychainBindingV2
	commitHooks            *atomicCommitHooks
}

func (provider *keychainMasterKeyProvider) LoadOrCreate(ctx context.Context) ([]byte, error) {
	key, err := provider.find(ctx)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, errKeychainItemNotFound) {
		return nil, err
	}
	candidate, err := (randomMasterKeySource{random: provider.random}).Generate()
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(candidate))+1)
	base64.StdEncoding.Encode(encoded[:len(encoded)-1], candidate)
	encoded[len(encoded)-1] = '\n'
	if err := provider.validateExplicitBinding(); err != nil {
		clearBytes(candidate)
		return nil, err
	}
	arguments := []string{"add-generic-password", "-s", keychainService, "-a", keychainAccount}
	addInput := encoded
	if provider.keychainDBPath != "" {
		arguments = []string{"-i"}
		addInput, err = explicitDarwinKeychainAddInput(encoded[:len(encoded)-1], provider.keychainDBPath)
		if err != nil {
			clearBytes(encoded)
			clearBytes(candidate)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
	} else {
		arguments = append(arguments, "-w")
	}
	result, addErr := provider.runner.Run(ctx, arguments, addInput)
	if provider.keychainDBPath != "" {
		clearBytes(addInput)
	}
	clearBytes(encoded)
	clearBytes(result.stdout)
	if errors.Is(addErr, errOSCredentialUnavailable) {
		clearBytes(candidate)
		return nil, errOSCredentialUnavailable
	}
	if provider.keychainDBPath != "" && addErr != nil {
		clearBytes(candidate)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	readbackProvider := provider
	refreshedSecurityDigest := ""
	var refreshedIdentity darwinKeychainSecurityDocumentV1
	if provider.keychainDBPath != "" {
		refreshedSecurityDigest, err = explicitDarwinKeychainSecurityDigest(provider.keychainDBPath)
		if err != nil {
			clearBytes(candidate)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		refreshedIdentity, err = explicitDarwinKeychainSecurityIdentity(provider.keychainDBPath)
		if err != nil || (provider.bindingPath != "" && !sameDarwinKeychainStableIdentity(provider.binding.Security, refreshedIdentity)) {
			clearBytes(candidate)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		refreshedProvider := *provider
		// A temporary readback pin does not commit or replace restart authority.
		refreshedProvider.bindingPath = ""
		refreshedProvider.keychainSecurityDigest = refreshedSecurityDigest
		readbackProvider = &refreshedProvider
	}
	readback, readErr := readbackProvider.find(ctx)
	if addErr != nil {
		clearBytes(candidate)
		if readErr != nil {
			clearBytes(readback)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		return readback, nil
	}
	matched := bytes.Equal(candidate, readback)
	if provider.keychainDBPath != "" {
		matched = len(candidate) == len(readback) && subtle.ConstantTimeCompare(candidate, readback) == 1
	}
	if readErr != nil || !matched {
		clearBytes(candidate)
		clearBytes(readback)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	if provider.keychainDBPath != "" {
		if err := provider.commitExplicitIdentity(refreshedIdentity); err != nil {
			clearBytes(candidate)
			clearBytes(readback)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		provider.keychainSecurityDigest = refreshedSecurityDigest
	}
	clearBytes(readback)
	return candidate, nil
}

func explicitDarwinKeychainAddInput(candidateToken []byte, databasePath string) ([]byte, error) {
	if len(candidateToken) != base64.StdEncoding.EncodedLen(masterKeySize) ||
		!darwinSafeTaskKeychainPath(databasePath) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	decoded := make([]byte, masterKeySize)
	defer clearBytes(decoded)
	written, err := base64.StdEncoding.Strict().Decode(decoded, candidateToken)
	if err != nil || written != masterKeySize {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	canonical := make([]byte, base64.StdEncoding.EncodedLen(masterKeySize))
	defer clearBytes(canonical)
	base64.StdEncoding.Encode(canonical, decoded)
	if subtle.ConstantTimeCompare(canonical, candidateToken) != 1 {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	prefix := []byte("add-generic-password -s " + keychainService + " -a " + keychainAccount + " -w ")
	input := make([]byte, 0, len(prefix)+len(candidateToken)+1+len(databasePath)+1)
	input = append(input, prefix...)
	input = append(input, candidateToken...)
	input = append(input, ' ')
	input = append(input, databasePath...)
	input = append(input, '\n')
	return input, nil
}

func (provider *keychainMasterKeyProvider) find(ctx context.Context) ([]byte, error) {
	if err := provider.validateExplicitBinding(); err != nil {
		return nil, err
	}
	arguments := []string{
		"find-generic-password",
		"-s", keychainService,
		"-a", keychainAccount,
		"-w",
	}
	if provider.keychainDBPath != "" {
		arguments = append(arguments, provider.keychainDBPath)
	}
	result, err := provider.runner.Run(ctx, arguments, nil)
	defer clearBytes(result.stdout)
	if err != nil {
		if errors.Is(err, errOSCredentialUnavailable) {
			return nil, errOSCredentialUnavailable
		}
		if result.exitCode == keychainNotFoundExit {
			return nil, errKeychainItemNotFound
		}
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	if err := provider.validateExplicitBinding(); err != nil {
		return nil, err
	}
	encoded, err := strictKeychainBase64Line(result.stdout)
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	key := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	written, decodeErr := base64.StdEncoding.Strict().Decode(key, encoded)
	if decodeErr != nil || written != masterKeySize {
		clearBytes(key)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return key[:written], nil
}

func (provider *keychainMasterKeyProvider) validateExplicitBinding() error {
	if provider.bindingPath != "" {
		committed, err := readExplicitDarwinBinding(provider.bindingPath)
		if err != nil || committed != provider.binding {
			return portsecretstore.ErrMasterKeyUnavailable
		}
	}
	if provider.keychainDBPath == "" && provider.keychainSecurityDigest == "" {
		return nil
	}
	if err := validateExplicitDarwinKeychainDB(provider.keychainDBPath, provider.keychainSecurityDigest); err != nil {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	return nil
}

func strictKeychainBase64Line(output []byte) ([]byte, error) {
	encoded := output
	if bytes.HasSuffix(encoded, []byte{'\r', '\n'}) {
		encoded = encoded[:len(encoded)-2]
	} else if bytes.HasSuffix(encoded, []byte{'\n'}) {
		encoded = encoded[:len(encoded)-1]
	}
	if len(encoded) == 0 || bytes.ContainsAny(encoded, " \t\r\n") {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return encoded, nil
}

type darwinMasterKeyAuthority string

const (
	darwinAuthorityKeychain darwinMasterKeyAuthority = "analytix-master-key-authority:v1:keychain\n"
	darwinAuthorityFallback darwinMasterKeyAuthority = "analytix-master-key-authority:v1:fallback\n"
)

type darwinMasterKeyProvider struct {
	authorityPath string
	keychain      *keychainMasterKeyProvider
	fallback      *fallbackMasterKeyProvider
	mu            sync.Mutex
}

func (provider *darwinMasterKeyProvider) LoadOrCreate(ctx context.Context) ([]byte, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	authority, err := provider.readAuthority()
	if err == nil {
		return provider.loadSelected(ctx, authority)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}

	existingFallback, err := provider.fallback.readWithBoundedInitializationWait()
	if err == nil {
		winner, selectErr := provider.selectAuthority(darwinAuthorityFallback)
		if selectErr != nil {
			clearBytes(existingFallback)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		if winner == darwinAuthorityFallback {
			return existingFallback, nil
		}
		clearBytes(existingFallback)
		return provider.loadSelected(ctx, winner)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}

	proposed := darwinAuthorityKeychain
	probedKey, probeErr := provider.keychain.find(ctx)
	switch {
	case probeErr == nil:
	case errors.Is(probeErr, errKeychainItemNotFound):
	case errors.Is(probeErr, errOSCredentialUnavailable):
		proposed = darwinAuthorityFallback
	default:
		clearBytes(probedKey)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	winner, err := provider.selectAuthority(proposed)
	if err != nil {
		clearBytes(probedKey)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	if winner == darwinAuthorityKeychain && proposed == darwinAuthorityKeychain && len(probedKey) == masterKeySize {
		return probedKey, nil
	}
	clearBytes(probedKey)
	return provider.loadSelected(ctx, winner)
}

func (provider *darwinMasterKeyProvider) loadSelected(ctx context.Context, authority darwinMasterKeyAuthority) ([]byte, error) {
	switch authority {
	case darwinAuthorityFallback:
		return provider.fallback.LoadOrCreate(ctx)
	case darwinAuthorityKeychain:
		fallbackKey, err := provider.fallback.readWithBoundedInitializationWait()
		if err == nil {
			clearBytes(fallbackKey)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		key, err := provider.keychain.LoadOrCreate(ctx)
		if err != nil {
			clearBytes(key)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		return key, nil
	default:
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
}

func (provider *darwinMasterKeyProvider) selectAuthority(proposed darwinMasterKeyAuthority) (darwinMasterKeyAuthority, error) {
	if proposed != darwinAuthorityKeychain && proposed != darwinAuthorityFallback {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	if err := ensurePrivateStoreDirectory(filepath.Dir(provider.authorityPath)); err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	temporary, err := os.CreateTemp(filepath.Dir(provider.authorityPath), "."+filepath.Base(provider.authorityPath)+".selection-")
	if err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if info, statErr := temporary.Stat(); statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	content := []byte(proposed)
	if err := writeAll(temporary, content); err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	if err := temporary.Sync(); err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	if err := temporary.Close(); err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	if err := os.Link(temporaryPath, provider.authorityPath); err != nil && !errors.Is(err, os.ErrExist) {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	if err := syncPrivateStoreDirectory(filepath.Dir(provider.authorityPath)); err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	winner, err := provider.readAuthority()
	if err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	return winner, nil
}

func (provider *darwinMasterKeyProvider) readAuthority() (darwinMasterKeyAuthority, error) {
	content, err := readPrivateCommittedFile(provider.authorityPath, 64)
	if err != nil {
		return "", err
	}
	defer clearBytes(content)
	authority := darwinMasterKeyAuthority(content)
	if authority != darwinAuthorityKeychain && authority != darwinAuthorityFallback {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	return authority, nil
}

func defaultMasterKeyProvider(storePath string, options Options) (masterKeyProvider, error) {
	if storePath == "" || !filepath.IsAbs(storePath) || filepath.Clean(storePath) != storePath {
		return nil, portsecretstore.ErrInvalidRequest
	}
	directory := filepath.Join(filepath.Dir(storePath), "master-key")
	bindingPath := filepath.Join(directory, darwinExplicitBindingFile)
	if !options.empty() {
		if options.DarwinKeychainDBPath == "" || options.DarwinKeychainBindingDigest == "" ||
			options.DarwinKeychainSecurityDigest == "" || options.DarwinKeychainAuthorityStorePath == "" ||
			validateExplicitDarwinKeychainDB(options.DarwinKeychainDBPath, options.DarwinKeychainSecurityDigest) != nil ||
			!isSHA256Hex(options.DarwinKeychainBindingDigest) ||
			validateExplicitDarwinKeychainAuthorityStorePath(storePath, options.DarwinKeychainAuthorityStorePath) != nil {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		authorityDigest, err := explicitDarwinKeychainAuthorityDigest(
			options.DarwinKeychainAuthorityStorePath,
			options.DarwinKeychainDBPath,
		)
		if err != nil {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		if _, err := os.Lstat(filepath.Join(directory, darwinAuthorityFile)); err == nil || !errors.Is(err, os.ErrNotExist) {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		if _, err := os.Lstat(filepath.Join(directory, "master.key")); err == nil || !errors.Is(err, os.ErrNotExist) {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		identity, err := explicitDarwinKeychainSecurityIdentity(options.DarwinKeychainDBPath)
		if err != nil {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		binding := darwinExplicitKeychainBindingV2{SchemaVersion: 2, AuthorityDigest: authorityDigest, Security: identity}
		if err := bindExplicitDarwinKeychain(directory, bindingPath, storePath, binding); err != nil {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		return &keychainMasterKeyProvider{
			runner: securityCommandRunner{keychainDBPath: options.DarwinKeychainDBPath}, random: rand.Reader,
			bindingPath: bindingPath, binding: binding,
			keychainDBPath:         options.DarwinKeychainDBPath,
			keychainSecurityDigest: options.DarwinKeychainSecurityDigest,
		}, nil
	}
	if _, err := os.Lstat(bindingPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return &darwinMasterKeyProvider{
		authorityPath: filepath.Join(directory, darwinAuthorityFile),
		keychain: &keychainMasterKeyProvider{
			runner: securityCommandRunner{},
			random: rand.Reader,
		},
		fallback: newFallbackMasterKeyProvider(filepath.Join(directory, "master.key"), rand.Reader),
	}, nil
}

func validateExplicitDarwinKeychainAuthorityStorePath(storePath string, authorityStorePath string) error {
	if !darwinSafeAbsoluteToken(storePath) || !darwinSafeAbsoluteToken(authorityStorePath) {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	storeRole := filepath.Join(
		filepath.Base(filepath.Dir(filepath.Dir(storePath))),
		filepath.Base(filepath.Dir(storePath)),
		filepath.Base(storePath),
	)
	authorityRole := filepath.Join(
		filepath.Base(filepath.Dir(filepath.Dir(authorityStorePath))),
		filepath.Base(filepath.Dir(authorityStorePath)),
		filepath.Base(authorityStorePath),
	)
	if storeRole != filepath.Join("private", "provider-secrets", "credentials.v1.json") || authorityRole != storeRole {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	return nil
}

type darwinKeychainSecurityDocumentV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	PathDigest    string `json:"pathDigest"`
	Device        string `json:"device"`
	Inode         string `json:"inode"`
	Owner         string `json:"owner"`
	Mode          string `json:"mode"`
	Links         string `json:"links"`
}

type darwinExplicitKeychainAuthorityDocumentV1 struct {
	SchemaVersion        int    `json:"schemaVersion"`
	Purpose              string `json:"purpose"`
	StorePathDigest      string `json:"storePathDigest"`
	KeychainDBPathDigest string `json:"keychainDBPathDigest"`
	KeychainService      string `json:"keychainService"`
	KeychainAccount      string `json:"keychainAccount"`
}

func explicitDarwinKeychainAuthorityDigest(storePath string, databasePath string) (string, error) {
	if !darwinSafeAbsoluteToken(storePath) || !darwinSafeTaskKeychainPath(databasePath) {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	storePathHash := sha256.Sum256([]byte(storePath))
	databasePathHash := sha256.Sum256([]byte(databasePath))
	document := darwinExplicitKeychainAuthorityDocumentV1{
		SchemaVersion:        1,
		Purpose:              "analytix-explicit-task-keychain-authority:v1",
		StorePathDigest:      hex.EncodeToString(storePathHash[:]),
		KeychainDBPathDigest: hex.EncodeToString(databasePathHash[:]),
		KeychainService:      keychainService,
		KeychainAccount:      keychainAccount,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateExplicitDarwinKeychainDB(path string, expectedDigest string) error {
	if !darwinSafeTaskKeychainPath(path) || !isSHA256Hex(expectedDigest) {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	digest, err := explicitDarwinKeychainSecurityDigest(path)
	if err != nil || digest != expectedDigest {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	return nil
}

func darwinSafeAbsoluteToken(value string) bool {
	if value == "" || len(value) > 1024 || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '/' || character == '.' ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func darwinSafeTaskKeychainPath(value string) bool {
	return darwinSafeAbsoluteToken(value) && !strings.Contains(strings.ToLower(value), "login.keychain") &&
		filepath.Base(value) == "analytix-task.keychain-db"
}

func explicitDarwinKeychainSecurityIdentity(path string) (darwinKeychainSecurityDocumentV1, error) {
	if !darwinSafeTaskKeychainPath(path) {
		return darwinKeychainSecurityDocumentV1{}, portsecretstore.ErrMasterKeyUnavailable
	}
	identity, err := os.Lstat(path)
	if err != nil || !identity.Mode().IsRegular() || identity.Mode().Perm() != 0o600 || identity.Size() <= 0 {
		return darwinKeychainSecurityDocumentV1{}, portsecretstore.ErrMasterKeyUnavailable
	}
	stat, ok := identity.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return darwinKeychainSecurityDocumentV1{}, portsecretstore.ErrMasterKeyUnavailable
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return darwinKeychainSecurityDocumentV1{}, portsecretstore.ErrMasterKeyUnavailable
	}
	pathHash := sha256.Sum256([]byte(path))
	document := darwinKeychainSecurityDocumentV1{
		SchemaVersion: 1, PathDigest: hex.EncodeToString(pathHash[:]),
		Device: strconv.FormatUint(uint64(stat.Dev), 10), Inode: strconv.FormatUint(stat.Ino, 10),
		Owner: strconv.FormatUint(uint64(stat.Uid), 10), Mode: strconv.FormatUint(uint64(identity.Mode().Perm()), 8),
		Links: strconv.FormatUint(uint64(stat.Nlink), 10),
	}
	return document, nil
}

func explicitDarwinKeychainSecurityDigest(path string) (string, error) {
	document, err := explicitDarwinKeychainSecurityIdentity(path)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	valid := err == nil && len(decoded) == sha256.Size
	clear(decoded)
	return valid
}

// The legacy filename is retained for inventory compatibility. Digest-only V1
// records cannot prove a past physical identity and must not be silently upgraded.
type darwinExplicitKeychainBindingV2 struct {
	SchemaVersion   int                              `json:"schemaVersion"`
	AuthorityDigest string                           `json:"authorityDigest"`
	Security        darwinKeychainSecurityDocumentV1 `json:"security"`
}

func readExplicitDarwinBinding(path string) (darwinExplicitKeychainBindingV2, error) {
	var record darwinExplicitKeychainBindingV2
	content, err := readPrivateCommittedFileWithLinks(path, 2048, true)
	if err != nil {
		return record, err
	}
	defer clearBytes(content)
	return parseExplicitDarwinBinding(content)
}

func parseExplicitDarwinBinding(content []byte) (darwinExplicitKeychainBindingV2, error) {
	var record darwinExplicitKeychainBindingV2
	if len(content) == 0 || len(content) > 2048 {
		return record, portsecretstore.ErrMasterKeyUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil || record.SchemaVersion != 2 || !isSHA256Hex(record.AuthorityDigest) {
		return darwinExplicitKeychainBindingV2{}, portsecretstore.ErrMasterKeyUnavailable
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return darwinExplicitKeychainBindingV2{}, portsecretstore.ErrMasterKeyUnavailable
	}
	// Our writer has one canonical encoding. This rejects duplicate keys,
	// conflicting representations and unsupported hand-written authority.
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(content, canonical) {
		return darwinExplicitKeychainBindingV2{}, portsecretstore.ErrMasterKeyUnavailable
	}
	return record, nil
}

func bindExplicitDarwinKeychain(directory, path, storePath string, expected darwinExplicitKeychainBindingV2) error {
	// Only a fresh master-key directory may establish initial authority. A missing
	// marker in an existing profile is evidence loss, not first provisioning.
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		// Successful startup owns both trees, including an empty Provider Registry.
		// Losing the whole key directory must not turn an old profile into genesis.
		secretRoot := filepath.Dir(directory)
		registryRoot := filepath.Join(filepath.Dir(secretRoot), "provider-registry")
		for _, retained := range []string{secretRoot, registryRoot} {
			if _, err := os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
				return portsecretstore.ErrMasterKeyUnavailable
			}
		}
		if _, err := os.Lstat(storePath); !errors.Is(err, os.ErrNotExist) {
			return portsecretstore.ErrMasterKeyUnavailable
		}
		if err := ensurePrivateStoreDirectory(filepath.Dir(directory)); err != nil {
			return err
		}
		if err := os.Mkdir(directory, 0o700); err != nil {
			return err
		}
		content, err := json.Marshal(expected)
		if err != nil {
			return err
		}
		if err := createPrivateExclusiveFile(path, content); err != nil {
			return err
		}
		if err := syncPrivateStoreDirectory(directory); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := ensurePrivateStoreDirectory(directory); err != nil {
		return err
	}
	// Reject unknown targets before recovery could overwrite them from backup.
	observed, err := readExplicitDarwinBinding(path)
	if err != nil || observed != expected {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	if err := recoverPrivateFileCommitValidated(path, func(backup []byte, present bool) error {
		record, err := parseExplicitDarwinBinding(backup)
		if !present || err != nil || record != expected {
			return portsecretstore.ErrMasterKeyUnavailable
		}
		current, err := readExplicitDarwinBinding(path)
		if err != nil || current != expected {
			return portsecretstore.ErrMasterKeyUnavailable
		}
		return nil
	}); err != nil {
		return err
	}
	committed, err := readExplicitDarwinBinding(path)
	if err != nil || committed != expected {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	return nil
}

func sameDarwinKeychainStableIdentity(before, after darwinKeychainSecurityDocumentV1) bool {
	before.Inode = after.Inode
	return before == after
}

func (provider *keychainMasterKeyProvider) commitExplicitIdentity(identity darwinKeychainSecurityDocumentV1) error {
	if provider.bindingPath == "" {
		return nil
	} // Nonpersistent protocol test provider.
	expected := provider.binding
	if !sameDarwinKeychainStableIdentity(expected.Security, identity) {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	committed, err := readExplicitDarwinBinding(provider.bindingPath)
	if err != nil || committed != expected {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	current, err := explicitDarwinKeychainSecurityIdentity(provider.keychainDBPath)
	if err != nil || current != identity {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	next := expected
	next.Security = identity
	content, err := json.Marshal(next)
	if err != nil {
		return err
	}
	// Runtime owns the exclusive profile lease. Recheck its pinned record at the
	// actual replacement seam; never overwrite a concurrently changed authority.
	hooks := atomicCommitHooks{}
	if provider.commitHooks != nil {
		hooks = *provider.commitHooks
	}
	beforeReplace := hooks.beforeReplace
	hooks.beforeReplace = func() error {
		var hookErr error
		if beforeReplace != nil {
			hookErr = beforeReplace()
		}
		prior, err := readExplicitDarwinBinding(provider.bindingPath)
		if err != nil || prior != expected {
			return errAtomicCommitAuthorityChanged
		}
		if hookErr != nil {
			return hookErr
		}
		now, err := explicitDarwinKeychainSecurityIdentity(provider.keychainDBPath)
		if err != nil || now != identity {
			return portsecretstore.ErrMasterKeyUnavailable
		}
		return nil
	}
	if err := writePrivateFileAtomically(provider.bindingPath, content, &hooks); err != nil {
		return err
	}
	provider.binding = next
	return nil
}
