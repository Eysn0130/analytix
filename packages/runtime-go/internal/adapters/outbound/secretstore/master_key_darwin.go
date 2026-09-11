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

type securityCommandRunner struct{}

func (securityCommandRunner) Run(ctx context.Context, arguments []string, stdin []byte) (keychainCommandResult, error) {
	command := exec.CommandContext(ctx, keychainSecurityPath, arguments...)
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
	if provider.keychainDBPath != "" {
		refreshedSecurityDigest, err = explicitDarwinKeychainSecurityDigest(provider.keychainDBPath)
		if err != nil {
			clearBytes(candidate)
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		refreshedProvider := *provider
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
		if err := bindExplicitDarwinKeychain(directory, bindingPath, authorityDigest); err != nil {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		return &keychainMasterKeyProvider{
			runner: securityCommandRunner{}, random: rand.Reader,
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

func explicitDarwinKeychainSecurityDigest(path string) (string, error) {
	if !darwinSafeTaskKeychainPath(path) {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	identity, err := os.Lstat(path)
	if err != nil || !identity.Mode().IsRegular() || identity.Mode().Perm() != 0o600 || identity.Size() <= 0 {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	stat, ok := identity.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	pathHash := sha256.Sum256([]byte(path))
	document := darwinKeychainSecurityDocumentV1{
		SchemaVersion: 1, PathDigest: hex.EncodeToString(pathHash[:]),
		Device: strconv.FormatUint(uint64(stat.Dev), 10), Inode: strconv.FormatUint(stat.Ino, 10),
		Owner: strconv.FormatUint(uint64(stat.Uid), 10), Mode: strconv.FormatUint(uint64(identity.Mode().Perm()), 8),
		Links: strconv.FormatUint(uint64(stat.Nlink), 10),
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", portsecretstore.ErrMasterKeyUnavailable
	}
	digest := sha256.Sum256(encoded)
	clear(encoded)
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

func bindExplicitDarwinKeychain(directory string, path string, digest string) error {
	if err := ensurePrivateStoreDirectory(directory); err != nil {
		return err
	}
	content := []byte(digest + "\n")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if writeErr := writeAll(file, content); writeErr != nil {
			_ = file.Close()
			_ = os.Remove(path)
			clearBytes(content)
			return writeErr
		}
		err = errors.Join(file.Sync(), file.Close(), syncPrivateStoreDirectory(directory))
		if err != nil {
			clearBytes(content)
			return err
		}
	} else if !errors.Is(err, os.ErrExist) {
		clearBytes(content)
		return err
	}
	committed, readErr := readPrivateCommittedFile(path, 128)
	defer clearBytes(committed)
	match := readErr == nil && bytes.Equal(committed, content)
	clearBytes(content)
	if !match {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	return nil
}
