//go:build darwin

package secretstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

type scriptedKeychainStep struct {
	result keychainCommandResult
	err    error
}

func TestLockedTaskKeychainDoesNotStartSecurityCommand(t *testing.T) {
	for _, arguments := range [][]string{{"help"}, {"-i"}} {
		checked := false
		runner := securityCommandRunner{
			keychainDBPath: "/synthetic/analytix-task.keychain-db",
			checkUnlocked: func(ctx context.Context, database string) bool {
				checked = true
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 10*time.Second || database != "/synthetic/analytix-task.keychain-db" {
					t.Fatal("preflight did not retain the operation deadline and exact binding")
				}
				return false
			},
		}
		result, err := runner.Run(context.Background(), arguments, nil)
		if !checked || !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) || len(result.stdout) != 0 {
			t.Fatal("locked Keychain was not rejected before the otherwise successful security command")
		}
	}
}

func TestTaskKeychainMetadataProbeFailsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if taskKeychainUnlocked(ctx, "relative") || taskKeychainUnlocked(ctx, filepath.Join(t.TempDir(), "analytix-task.keychain-db")) {
		t.Fatal("invalid or nonexistent Keychain admitted")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "synthetic-database")
	if err := os.WriteFile(target, []byte("not a keychain"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "analytix-task.keychain-db")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if taskKeychainUnlocked(ctx, link) {
		t.Fatal("symlink admitted by private metadata entry")
	}
	cancel()
	if taskKeychainUnlocked(ctx, "/synthetic/analytix-task.keychain-db") {
		t.Fatal("cancelled preflight admitted")
	}
}

func TestSecurityCommandRejectsCancelledContextBeforeStartingV1(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, input := range []context.Context{nil, ctx} {
		result, err := (securityCommandRunner{}).Run(input, []string{"find-generic-password"}, nil)
		if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) || len(result.stdout) != 0 {
			t.Fatal("cancelled credential operation did not fail without output")
		}
	}
}

type keychainCall struct {
	arguments []string
	stdin     []byte
}

type scriptedKeychainRunner struct {
	steps     []scriptedKeychainStep
	calls     []keychainCall
	beforeRun func()
}

type privacySafeRealKeychainCall struct {
	kind        string
	exitCode    int
	stdoutBytes int
	stdinBytes  int
	failed      bool
}

type privacySafeRealKeychainRunner struct {
	delegate securityCommandRunner
	calls    []privacySafeRealKeychainCall
}

func (runner *privacySafeRealKeychainRunner) Run(
	ctx context.Context,
	arguments []string,
	stdin []byte,
) (keychainCommandResult, error) {
	kind := "invalid"
	if slices.Equal(arguments, []string{"-i"}) {
		kind = "interactive_add"
	} else if len(arguments) > 0 && arguments[0] == "find-generic-password" {
		kind = "explicit_find"
	}
	result, err := runner.delegate.Run(ctx, arguments, stdin)
	runner.calls = append(runner.calls, privacySafeRealKeychainCall{
		kind: kind, exitCode: result.exitCode, stdoutBytes: len(result.stdout),
		stdinBytes: len(stdin), failed: err != nil,
	})
	return result, err
}

func (runner *scriptedKeychainRunner) Run(_ context.Context, arguments []string, stdin []byte) (keychainCommandResult, error) {
	runner.calls = append(runner.calls, keychainCall{
		arguments: append([]string(nil), arguments...),
		stdin:     bytes.Clone(stdin),
	})
	if runner.beforeRun != nil {
		beforeRun := runner.beforeRun
		runner.beforeRun = nil
		beforeRun()
	}
	if len(runner.steps) == 0 {
		return keychainCommandResult{}, errors.New("unexpected synthetic keychain call")
	}
	step := runner.steps[0]
	runner.steps = runner.steps[1:]
	return step.result, step.err
}

func TestDarwinMasterKeyMixedProposalLoserObeysDurableWinner(t *testing.T) {
	t.Run("keychain proposal obeys fallback winner", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "master-key")
		authorityPath := filepath.Join(directory, darwinAuthorityFile)
		fallbackPath := filepath.Join(directory, "master.key")
		fallbackKey := bytes.Repeat([]byte{0xd1}, masterKeySize)
		keychainKey := bytes.Repeat([]byte{0xd2}, masterKeySize)
		runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
			result: keychainCommandResult{stdout: []byte(base64.StdEncoding.EncodeToString(keychainKey) + "\n")},
		}}}
		runner.beforeRun = func() {
			winnerFallback := newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(fallbackKey))
			winnerKey, err := winnerFallback.LoadOrCreate(context.Background())
			if err != nil {
				t.Fatalf("winner fallback LoadOrCreate() error = %v", err)
			}
			clearBytes(winnerKey)
			winner := &darwinMasterKeyProvider{authorityPath: authorityPath}
			selected, err := winner.selectAuthority(darwinAuthorityFallback)
			if err != nil || selected != darwinAuthorityFallback {
				t.Fatalf("winner selectAuthority() = %q, %v", selected, err)
			}
		}
		provider := &darwinMasterKeyProvider{
			authorityPath: authorityPath,
			keychain:      &keychainMasterKeyProvider{runner: runner},
			fallback:      newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0xd3}, masterKeySize))),
		}
		selected, err := provider.LoadOrCreate(context.Background())
		if err != nil {
			t.Fatalf("loser LoadOrCreate() error = %v", err)
		}
		defer clearBytes(selected)
		if !bytes.Equal(selected, fallbackKey) {
			t.Fatal("keychain proposal did not obey the durable fallback winner")
		}
		if len(runner.calls) != 1 {
			t.Fatalf("Keychain calls = %d, want only the losing probe", len(runner.calls))
		}

		restartRunner := &scriptedKeychainRunner{}
		restarted := &darwinMasterKeyProvider{
			authorityPath: authorityPath,
			keychain:      &keychainMasterKeyProvider{runner: restartRunner},
			fallback:      newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0xd4}, masterKeySize))),
		}
		restartedKey, err := restarted.LoadOrCreate(context.Background())
		if err != nil {
			t.Fatalf("restart LoadOrCreate() error = %v", err)
		}
		defer clearBytes(restartedKey)
		if !bytes.Equal(restartedKey, fallbackKey) || len(restartRunner.calls) != 0 {
			t.Fatal("restart did not obey the durable fallback winner")
		}
	})

	t.Run("fallback proposal obeys keychain winner", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "master-key")
		authorityPath := filepath.Join(directory, darwinAuthorityFile)
		fallbackPath := filepath.Join(directory, "master.key")
		keychainKey := bytes.Repeat([]byte{0xe1}, masterKeySize)
		runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
			{err: errOSCredentialUnavailable},
			{result: keychainCommandResult{stdout: []byte(base64.StdEncoding.EncodeToString(keychainKey) + "\n")}},
		}}
		runner.beforeRun = func() {
			winner := &darwinMasterKeyProvider{authorityPath: authorityPath}
			selected, err := winner.selectAuthority(darwinAuthorityKeychain)
			if err != nil || selected != darwinAuthorityKeychain {
				t.Fatalf("winner selectAuthority() = %q, %v", selected, err)
			}
		}
		provider := &darwinMasterKeyProvider{
			authorityPath: authorityPath,
			keychain:      &keychainMasterKeyProvider{runner: runner},
			fallback:      newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0xe2}, masterKeySize))),
		}
		selected, err := provider.LoadOrCreate(context.Background())
		if err != nil {
			t.Fatalf("loser LoadOrCreate() error = %v", err)
		}
		defer clearBytes(selected)
		if !bytes.Equal(selected, keychainKey) {
			t.Fatal("fallback proposal did not obey the durable Keychain winner")
		}
		if _, statErr := os.Lstat(fallbackPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("fallback proposal created a second master-key authority")
		}

		restartRunner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
			result: keychainCommandResult{stdout: []byte(base64.StdEncoding.EncodeToString(keychainKey) + "\n")},
		}}}
		restarted := &darwinMasterKeyProvider{
			authorityPath: authorityPath,
			keychain:      &keychainMasterKeyProvider{runner: restartRunner},
			fallback:      newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0xe3}, masterKeySize))),
		}
		restartedKey, err := restarted.LoadOrCreate(context.Background())
		if err != nil {
			t.Fatalf("restart LoadOrCreate() error = %v", err)
		}
		defer clearBytes(restartedKey)
		if !bytes.Equal(restartedKey, keychainKey) || len(restartRunner.calls) != 1 {
			t.Fatal("restart did not obey the durable Keychain winner")
		}
	})
}

func TestKeychainReadsExistingMasterKeyWithoutMutation(t *testing.T) {
	t.Parallel()

	winner := bytes.Repeat([]byte{0x51}, masterKeySize)
	runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
		result: keychainCommandResult{stdout: []byte(base64.StdEncoding.EncodeToString(winner) + "\n")},
	}}}
	provider := &keychainMasterKeyProvider{runner: runner, random: bytes.NewReader(bytes.Repeat([]byte{0x99}, masterKeySize))}
	key, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	defer clearBytes(key)
	if !bytes.Equal(key, winner) {
		t.Fatal("LoadOrCreate() did not return the existing Keychain key")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("Keychain calls = %d, want 1 find", len(runner.calls))
	}
	assertFindKeychainArguments(t, runner.calls[0])
}

func TestKeychainInitializationSuppliesSecretOnlyOnStdinAndVerifiesReadback(t *testing.T) {
	t.Parallel()

	candidate := bytes.Repeat([]byte{0x5c}, masterKeySize)
	encoded := base64.StdEncoding.EncodeToString(candidate)
	runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{},
		{result: keychainCommandResult{stdout: []byte(encoded + "\n")}},
	}}
	provider := &keychainMasterKeyProvider{runner: runner, random: bytes.NewReader(candidate)}
	key, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	defer clearBytes(key)
	if !bytes.Equal(key, candidate) {
		t.Fatal("LoadOrCreate() returned a key that differs from verified readback")
	}
	if len(runner.calls) != 3 {
		t.Fatalf("Keychain calls = %d, want find/add/find", len(runner.calls))
	}
	assertFindKeychainArguments(t, runner.calls[0])
	add := runner.calls[1]
	if !slices.Equal(add.arguments, []string{"-i"}) {
		t.Fatalf("default Keychain add argv = %v, want only -i", add.arguments)
	}
	wantInteractiveInput := []byte("add-generic-password -s " + keychainService +
		" -a " + keychainAccount + " -w " + encoded + "\n")
	if !bytes.Equal(add.stdin, wantInteractiveInput) {
		t.Fatal("default Keychain add did not supply one bounded private command on stdin")
	}
	assertFindKeychainArguments(t, runner.calls[2])
}

func TestExplicitTaskKeychainUsesExactDatabaseForEverySecurityCall(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Repeat([]byte{0x5d}, masterKeySize)
	encoded := base64.StdEncoding.EncodeToString(candidate)
	runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{},
		{result: keychainCommandResult{stdout: []byte(encoded + "\n")}},
	}}
	provider := &keychainMasterKeyProvider{
		runner: runner, random: bytes.NewReader(candidate),
		keychainDBPath: databasePath, keychainSecurityDigest: digest,
	}
	key, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	defer clearBytes(key)
	if len(runner.calls) != 3 {
		t.Fatalf("security calls = %d, want find/add/readback", len(runner.calls))
	}
	for _, call := range []keychainCall{runner.calls[0], runner.calls[2]} {
		if call.arguments[len(call.arguments)-1] != databasePath {
			t.Fatalf("explicit find/readback argv does not end in exact task DB: %v", call.arguments)
		}
		for _, argument := range call.arguments {
			if argument == "default-keychain" || argument == "list-keychains" {
				t.Fatalf("security argv used default/search-list command: %v", call.arguments)
			}
		}
	}
	if !slices.Equal(runner.calls[1].arguments, []string{"-i"}) {
		t.Fatalf("explicit Keychain add argv = %v, want only -i", runner.calls[1].arguments)
	}
	wantInteractiveInput := []byte("add-generic-password -s " + keychainService +
		" -a " + keychainAccount + " -w " + encoded + " " + databasePath + "\n")
	if !bytes.Equal(runner.calls[1].stdin, wantInteractiveInput) ||
		bytes.Count(runner.calls[1].stdin, []byte{'\n'}) != 1 ||
		runner.calls[1].stdin[len(runner.calls[1].stdin)-1] != '\n' {
		t.Fatal("explicit Keychain add did not use exactly one canonical newline-terminated stdin command")
	}
}

func TestExplicitTaskKeychainAcceptsOnlyVerifiedPostAddDatabaseIdentityTransition(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Repeat([]byte{0x5f}, masterKeySize)
	encoded := base64.StdEncoding.EncodeToString(candidate)
	runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{},
		{result: keychainCommandResult{stdout: []byte(encoded + "\n")}},
	}}
	runner.beforeRun = func() {
		runner.beforeRun = func() {
			if err := os.Rename(databasePath, databasePath+".prior"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(databasePath, []byte("synthetic-updated-keychain-database"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	provider := &keychainMasterKeyProvider{
		runner: runner, random: bytes.NewReader(candidate),
		keychainDBPath: databasePath, keychainSecurityDigest: digest,
	}
	key, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	defer clearBytes(key)
	if !bytes.Equal(key, candidate) || len(runner.calls) != 3 {
		t.Fatal("verified post-add identity transition did not complete exact readback")
	}
	if err := provider.validateExplicitBinding(); err != nil {
		t.Fatal("verified post-add identity transition did not refresh the private binding")
	}
}

func TestExplicitTaskKeychainTwoProcessRestartSurvivesVerifiedDatabaseIdentityTransition(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	databasePath := filepath.Join(root, "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	preAddSecurityDigest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(root, "runtime-data", "private", "provider-secrets", "credentials.v1.json")
	firstOptions := Options{
		DarwinKeychainDBPath:             databasePath,
		DarwinKeychainBindingDigest:      strings.Repeat("a", 64),
		DarwinKeychainSecurityDigest:     preAddSecurityDigest,
		DarwinKeychainAuthorityStorePath: storePath,
	}
	firstMasterKeys, err := defaultMasterKeyProvider(storePath, firstOptions)
	if err != nil {
		t.Fatalf("first-process defaultMasterKeyProvider() error = %v", err)
	}
	candidate := bytes.Repeat([]byte{0x67}, masterKeySize)
	encodedCandidate := base64.StdEncoding.EncodeToString(candidate)
	firstRunner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{},
		{result: keychainCommandResult{stdout: []byte(encodedCandidate + "\n")}},
	}}
	firstRunner.beforeRun = func() {
		firstRunner.beforeRun = func() {
			if err := os.Rename(databasePath, databasePath+".prior"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(databasePath, []byte("synthetic-updated-keychain-database"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	firstProvider, ok := firstMasterKeys.(*keychainMasterKeyProvider)
	if !ok {
		t.Fatal("first-process explicit provider has unexpected type")
	}
	firstProvider.runner = firstRunner
	firstProvider.random = bytes.NewReader(candidate)
	firstStore, err := newStore(storePath, firstProvider, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("first-process NewWithOptions composition error = %v", err)
	}
	purpose := portsecretstore.Purpose("provider-api-key")
	secret := []byte("synthetic-two-process-provider-secret")
	credentialRef, err := firstStore.Put(ctx, purpose, secret)
	if err != nil {
		t.Fatalf("first-process Put() error = %v", err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("first-process Close() error = %v", err)
	}
	postAddSecurityDigest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if postAddSecurityDigest == preAddSecurityDigest {
		t.Fatal("synthetic add did not rotate the database identity")
	}
	secondOptions := firstOptions
	secondOptions.DarwinKeychainBindingDigest = strings.Repeat("b", 64)
	secondOptions.DarwinKeychainSecurityDigest = postAddSecurityDigest
	secondProcessStore, err := NewWithOptions(storePath, allowCredentialConsumer{}, secondOptions)
	if err != nil {
		t.Fatalf("second-process NewWithOptions() error = %v", err)
	}
	if err := secondProcessStore.Close(); err != nil {
		t.Fatalf("second-process constructor Close() error = %v", err)
	}
	secondMasterKeys, err := defaultMasterKeyProvider(storePath, secondOptions)
	if err != nil {
		t.Fatalf("second-process defaultMasterKeyProvider() error = %v", err)
	}
	secondProvider, ok := secondMasterKeys.(*keychainMasterKeyProvider)
	if !ok {
		t.Fatal("second-process explicit provider has unexpected type")
	}
	secondRunner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
		result: keychainCommandResult{stdout: []byte(encodedCandidate + "\n")},
	}}}
	secondProvider.runner = secondRunner
	secondStore, err := newStore(storePath, secondProvider, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("second-process NewWithOptions composition error = %v", err)
	}
	reopened, err := secondStore.GetForAuthorizedConsumer(ctx, portsecretstore.AccessRequest{
		CredentialRef: credentialRef, Purpose: purpose, Consumer: "provider-registry-manager",
	})
	if err != nil {
		t.Fatalf("second-process GetForAuthorizedConsumer() error = %v", err)
	}
	defer clearBytes(reopened)
	if !bytes.Equal(reopened, secret) || len(firstRunner.calls) != 3 || len(secondRunner.calls) != 1 {
		t.Fatal("full two-process lifecycle did not reopen the committed ciphertext through the same authority")
	}
}

func TestExplicitTaskKeychainInteractiveAddInputRejectsInvalidTokenPathAndSecondCommand(t *testing.T) {
	t.Parallel()
	validToken := []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, masterKeySize)))
	validPath := "/private/task/analytix-task.keychain-db"
	if input, err := explicitDarwinKeychainAddInput(validToken, validPath); err != nil {
		t.Fatalf("valid interactive input error = %v", err)
	} else {
		clearBytes(input)
	}
	for _, testCase := range []struct {
		name  string
		token []byte
		path  string
	}{
		{name: "empty token", token: nil, path: validPath},
		{name: "short token", token: []byte("YQ=="), path: validPath},
		{name: "whitespace token", token: append(bytes.Clone(validToken), '\n'), path: validPath},
		{name: "quote token", token: append(bytes.Clone(validToken[:len(validToken)-1]), '\''), path: validPath},
		{name: "relative path", token: validToken, path: "private/task/analytix-task.keychain-db"},
		{name: "space path", token: validToken, path: "/private/task one/analytix-task.keychain-db"},
		{name: "quote path", token: validToken, path: "/private/task'/analytix-task.keychain-db"},
		{name: "backslash path", token: validToken, path: "/private/task\\analytix-task.keychain-db"},
		{name: "second command path", token: validToken, path: "/private/task/analytix-task.keychain-db\nquit"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input, err := explicitDarwinKeychainAddInput(testCase.token, testCase.path)
			clearBytes(input)
			if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatalf("error = %v, want unavailable", err)
			}
		})
	}
}

func TestDefaultKeychainInteractiveAddInputRejectsInvalidToken(t *testing.T) {
	t.Parallel()
	validToken := []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, masterKeySize)))
	input, err := darwinKeychainAddInput(validToken, "")
	if err != nil {
		t.Fatalf("valid default Keychain command error = %v", err)
	}
	defer clearBytes(input)
	if bytes.Count(input, []byte{'\n'}) != 1 || input[len(input)-1] != '\n' ||
		!bytes.HasSuffix(input, append(bytes.Clone(validToken), '\n')) {
		t.Fatal("default Keychain command is not one bounded input line")
	}
	for _, token := range [][]byte{
		nil,
		[]byte("YQ=="),
		append(bytes.Clone(validToken), '\n'),
		append(bytes.Clone(validToken[:len(validToken)-1]), '\''),
	} {
		candidate, err := darwinKeychainAddInput(token, "")
		clearBytes(candidate)
		if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
			t.Fatalf("invalid default Keychain token error = %v, want unavailable", err)
		}
	}
}

func TestExplicitTaskKeychainInteractiveAddNonzeroEmptyAndWrongReadbackFailClosed(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	candidate := bytes.Repeat([]byte{0x43}, masterKeySize)
	encodedCandidate := base64.StdEncoding.EncodeToString(candidate)
	wrong := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x44}, masterKeySize))
	for _, testCase := range []struct {
		name  string
		steps []scriptedKeychainStep
	}{
		{
			name: "nonzero add",
			steps: []scriptedKeychainStep{
				{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
				{result: keychainCommandResult{exitCode: 1}, err: portsecretstore.ErrMasterKeyUnavailable},
				{result: keychainCommandResult{stdout: []byte(encodedCandidate + "\n")}},
			},
		},
		{
			name: "empty readback",
			steps: []scriptedKeychainStep{
				{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
				{},
				{result: keychainCommandResult{stdout: []byte("\n")}},
			},
		},
		{
			name: "wrong readback",
			steps: []scriptedKeychainStep{
				{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
				{},
				{result: keychainCommandResult{stdout: []byte(wrong + "\n")}},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := &scriptedKeychainRunner{steps: testCase.steps}
			provider := &keychainMasterKeyProvider{
				runner: runner, random: bytes.NewReader(candidate),
				keychainDBPath: databasePath, keychainSecurityDigest: digest,
			}
			key, loadErr := provider.LoadOrCreate(context.Background())
			clearBytes(key)
			if !errors.Is(loadErr, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatalf("LoadOrCreate() error = %v, want unavailable", loadErr)
			}
		})
	}
}

func TestK10RealTaskOwnedExplicitKeychainProbe(t *testing.T) {
	if os.Getenv("ANALYTIX_K10_REAL_KEYCHAIN_PROBE") != "1" {
		t.Skip("task-owned real Keychain probe is explicitly gated")
	}
	databasePath := os.Getenv("ANALYTIX_K10_REAL_KEYCHAIN_DB_PATH")
	if !darwinSafeAbsoluteToken(databasePath) {
		t.Fatal("task-owned real Keychain probe binding is invalid")
	}
	digest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal("task-owned real Keychain probe identity is unavailable")
	}
	provider := &keychainMasterKeyProvider{
		runner: &privacySafeRealKeychainRunner{}, random: rand.Reader,
		keychainDBPath: databasePath, keychainSecurityDigest: digest,
	}
	candidate, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		clearBytes(candidate)
		realRunner := provider.runner.(*privacySafeRealKeychainRunner)
		t.Fatalf("task-owned real Keychain probe add/readback failed; privacySafeCalls=%+v", realRunner.calls)
	}
	defer clearBytes(candidate)
	currentRead, err := provider.find(context.Background())
	if err != nil {
		clearBytes(currentRead)
		realRunner := provider.runner.(*privacySafeRealKeychainRunner)
		t.Fatalf("task-owned real Keychain probe same-process current read failed; privacySafeCalls=%+v", realRunner.calls)
	}
	defer clearBytes(currentRead)
	matched := len(candidate) == masterKeySize && len(currentRead) == masterKeySize &&
		subtle.ConstantTimeCompare(candidate, currentRead) == 1
	if !matched {
		t.Fatal("task-owned real Keychain probe same-process current read mismatch")
	}
	realRunner := provider.runner.(*privacySafeRealKeychainRunner)
	if len(realRunner.calls) != 4 ||
		realRunner.calls[0].kind != "explicit_find" || realRunner.calls[0].exitCode != keychainNotFoundExit || !realRunner.calls[0].failed ||
		realRunner.calls[1].kind != "interactive_add" || realRunner.calls[1].exitCode != 0 || realRunner.calls[1].failed ||
		realRunner.calls[2].kind != "explicit_find" || realRunner.calls[2].exitCode != 0 || realRunner.calls[2].failed ||
		realRunner.calls[3].kind != "explicit_find" || realRunner.calls[3].exitCode != 0 || realRunner.calls[3].failed {
		t.Fatalf("task-owned real Keychain probe call sequence invalid; privacySafeCalls=%+v", realRunner.calls)
	}
	t.Logf("k10-real-task-keychain-probe: createdFilenameExact=true databaseUnlocked=true initialFindExpectedAbsent=true initialFindExitCode=%d interactiveAddExitCode=%d interactiveAddStdinBytes=%d readbackExitCode=%d readbackStdoutBytes=%d currentReadExitCode=%d currentReadStdoutBytes=%d storedByteCount=%d candidateMatched=true sameProcessCurrentReadMatched=true explicitDatabaseFinal=true defaultSearchListMutationCount=0 homeOverride=false",
		realRunner.calls[0].exitCode,
		realRunner.calls[1].exitCode,
		realRunner.calls[1].stdinBytes,
		realRunner.calls[2].exitCode,
		realRunner.calls[2].stdoutBytes,
		realRunner.calls[3].exitCode,
		realRunner.calls[3].stdoutBytes,
		len(currentRead),
	)
}

func TestExplicitTaskKeychainDriftFailsBeforeSecurityCall(t *testing.T) {
	t.Parallel()
	databasePath := filepath.Join(t.TempDir(), "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(databasePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(databasePath, []byte("replacement-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &scriptedKeychainRunner{}
	provider := &keychainMasterKeyProvider{
		runner: runner, random: bytes.NewReader(bytes.Repeat([]byte{0x5e}, masterKeySize)),
		keychainDBPath: databasePath, keychainSecurityDigest: digest,
	}
	key, err := provider.LoadOrCreate(context.Background())
	clearBytes(key)
	if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) || len(runner.calls) != 0 {
		t.Fatalf("drift result err=%v security calls=%d", err, len(runner.calls))
	}
}

func TestExplicitTaskKeychainRejectsLoginAndAliasPathsBeforeAnySecurityCall(t *testing.T) {
	validToken := []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, masterKeySize)))
	for _, path := range []string{
		"/private/task/login.keychain-db", "/private/task/LoGiN.KeYcHaIn-db",
		"/private/prefix-login.keychain-suffix/analytix-task.keychain-db",
		"/private/LoGiN.KeYcHaIn/analytix-task.keychain-db",
		"/private/task/../task/analytix-task.keychain-db", "/private/task//analytix-task.keychain-db",
		"/private/task/analytix-task.keychain-db\nquit", "/private/task/arbitrary.keychain-db",
	} {
		runner := &scriptedKeychainRunner{}
		provider := &keychainMasterKeyProvider{runner: runner, random: bytes.NewReader(make([]byte, masterKeySize)),
			keychainDBPath: path, keychainSecurityDigest: strings.Repeat("a", 64)}
		key, err := provider.LoadOrCreate(context.Background())
		if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) || len(key) != 0 || len(runner.calls) != 0 {
			t.Fatal("unsafe explicit path reached a Security effect")
		}
		if input, err := explicitDarwinKeychainAddInput(validToken, path); err == nil || input != nil {
			t.Fatal("unsafe explicit path produced a Security command")
		}
		if digest, err := explicitDarwinKeychainSecurityDigest(path); err == nil || digest != "" {
			t.Fatal("unsafe explicit path produced an identity")
		}
	}
}

func TestExplicitTaskKeychainStableAuthorityMarkerPreventsDefaultOrCrossRootReuse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	databasePath := filepath.Join(root, "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	securityDigest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(root, "runtime-data", "private", "provider-secrets", "credentials.v1.json")
	first := Options{
		DarwinKeychainDBPath:             databasePath,
		DarwinKeychainBindingDigest:      strings.Repeat("a", 64),
		DarwinKeychainSecurityDigest:     securityDigest,
		DarwinKeychainAuthorityStorePath: storePath,
	}
	if _, err := defaultMasterKeyProvider(storePath, first); err != nil {
		t.Fatalf("first explicit provider error = %v", err)
	}
	if _, err := defaultMasterKeyProvider(storePath, first); err != nil {
		t.Fatalf("same explicit provider restart error = %v", err)
	}
	wantStableDigest, err := explicitDarwinKeychainAuthorityDigest(storePath, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	bindingPath := filepath.Join(filepath.Dir(storePath), "master-key", darwinExplicitBindingFile)
	committedMarker, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatal(err)
	}
	var committedIdentity darwinExplicitKeychainBindingV2
	if err := json.Unmarshal(committedMarker, &committedIdentity); err != nil || committedIdentity.SchemaVersion != 2 || committedIdentity.AuthorityDigest != wantStableDigest || committedIdentity.Security.Inode == "" {
		t.Fatal("explicit authority marker did not persist stable authority and committed physical identity")
	}
	if _, err := defaultMasterKeyProvider(storePath, Options{}); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("default reuse error = %v", err)
	}
	currentBinding := first
	currentBinding.DarwinKeychainBindingDigest = strings.Repeat("b", 64)
	if _, err := defaultMasterKeyProvider(storePath, currentBinding); err != nil {
		t.Fatalf("same stable authority with refreshed launch binding error = %v", err)
	}
	otherRoot := t.TempDir()
	otherDatabasePath := filepath.Join(otherRoot, "analytix-task.keychain-db")
	if err := os.WriteFile(otherDatabasePath, []byte("synthetic-other-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	otherSecurityDigest, err := explicitDarwinKeychainSecurityDigest(otherDatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	crossRoot := currentBinding
	crossRoot.DarwinKeychainDBPath = otherDatabasePath
	crossRoot.DarwinKeychainSecurityDigest = otherSecurityDigest
	if _, err := defaultMasterKeyProvider(storePath, crossRoot); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("cross-root authority reuse error = %v", err)
	}
	if err := os.WriteFile(bindingPath, []byte(strings.Repeat("f", 64)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := defaultMasterKeyProvider(storePath, currentBinding); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("mismatched stable authority marker error = %v", err)
	}
}

func TestExplicitTaskKeychainSemanticStageReopensAtCanonicalAuthorityStore(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	databasePath := filepath.Join(root, "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	securityDigest, err := explicitDarwinKeychainSecurityDigest(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	stageStorePath := filepath.Join(root, "semantic-stage", "private", "provider-secrets", "credentials.v1.json")
	authorityStorePath := filepath.Join(root, "runtime-data", "private", "provider-secrets", "credentials.v1.json")
	options := Options{
		DarwinKeychainDBPath:             databasePath,
		DarwinKeychainBindingDigest:      strings.Repeat("a", 64),
		DarwinKeychainSecurityDigest:     securityDigest,
		DarwinKeychainAuthorityStorePath: authorityStorePath,
	}
	if _, err := defaultMasterKeyProvider(stageStorePath, options); err != nil {
		t.Fatalf("semantic-stage explicit provider error = %v", err)
	}
	stageBindingPath := filepath.Join(filepath.Dir(stageStorePath), "master-key", darwinExplicitBindingFile)
	committedMarker, err := os.ReadFile(stageBindingPath)
	if err != nil {
		t.Fatal(err)
	}
	authorityBindingPath := filepath.Join(filepath.Dir(authorityStorePath), "master-key", darwinExplicitBindingFile)
	if err := os.MkdirAll(filepath.Dir(authorityBindingPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authorityBindingPath, committedMarker, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := defaultMasterKeyProvider(authorityStorePath, options); err != nil {
		t.Fatalf("canonical explicit provider after semantic apply error = %v", err)
	}
}

func TestKeychainConcurrentAddLoserRereadsWinner(t *testing.T) {
	t.Parallel()

	winner := bytes.Repeat([]byte{0x63}, masterKeySize)
	runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{result: keychainCommandResult{exitCode: 45}, err: portsecretstore.ErrMasterKeyUnavailable},
		{result: keychainCommandResult{stdout: []byte(base64.StdEncoding.EncodeToString(winner) + "\n")}},
	}}
	provider := &keychainMasterKeyProvider{runner: runner, random: bytes.NewReader(bytes.Repeat([]byte{0x64}, masterKeySize))}
	key, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	defer clearBytes(key)
	if !bytes.Equal(key, winner) {
		t.Fatal("concurrent add loser did not return the re-read winner")
	}
}

func TestKeychainReadbackMismatchFailsClosed(t *testing.T) {
	t.Parallel()

	candidate := bytes.Repeat([]byte{0x70}, masterKeySize)
	winner := bytes.Repeat([]byte{0x71}, masterKeySize)
	runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{},
		{result: keychainCommandResult{stdout: []byte(base64.StdEncoding.EncodeToString(winner) + "\n")}},
	}}
	provider := &keychainMasterKeyProvider{runner: runner, random: bytes.NewReader(candidate)}
	key, err := provider.LoadOrCreate(context.Background())
	clearBytes(key)
	if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("LoadOrCreate() error = %v, want unavailable", err)
	}
	assertRedactedError(t, err, base64.StdEncoding.EncodeToString(candidate), base64.StdEncoding.EncodeToString(winner))
}

func TestKeychainMalformedExistingValueFailsClosedWithoutInitialization(t *testing.T) {
	t.Parallel()

	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x73}, masterKeySize))
	runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
		result: keychainCommandResult{stdout: []byte(" " + encoded + "\n")},
	}}}
	provider := &keychainMasterKeyProvider{runner: runner, random: bytes.NewReader(bytes.Repeat([]byte{0x74}, masterKeySize))}
	key, err := provider.LoadOrCreate(context.Background())
	clearBytes(key)
	if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("LoadOrCreate() error = %v, want unavailable", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("Keychain calls = %d, want no initialization after malformed existing value", len(runner.calls))
	}
	assertRedactedError(t, err, encoded)
}

func TestKeychainFallbackOccursOnlyWhenCommandIsUnavailable(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name           string
		primaryError   error
		primaryResult  keychainCommandResult
		wantError      error
		wantFallback   bool
		wantSuccessful bool
	}{
		{
			name:          "locked or denied",
			primaryError:  portsecretstore.ErrMasterKeyUnavailable,
			primaryResult: keychainCommandResult{exitCode: 36, stdout: []byte("synthetic-command-stdout")},
			wantError:     portsecretstore.ErrMasterKeyUnavailable,
		},
		{
			name:           "command unavailable",
			primaryError:   errOSCredentialUnavailable,
			wantFallback:   true,
			wantSuccessful: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "master-key")
			runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
				result: testCase.primaryResult,
				err:    testCase.primaryError,
			}}}
			fallbackPath := filepath.Join(directory, "master.key")
			provider := &darwinMasterKeyProvider{
				authorityPath: filepath.Join(directory, darwinAuthorityFile),
				keychain:      &keychainMasterKeyProvider{runner: runner},
				fallback: newFallbackMasterKeyProvider(
					fallbackPath,
					bytes.NewReader(bytes.Repeat([]byte{0x75}, masterKeySize)),
				),
			}
			key, err := provider.LoadOrCreate(context.Background())
			clearBytes(key)
			if testCase.wantSuccessful {
				if err != nil {
					t.Fatalf("LoadOrCreate() error = %v", err)
				}
			} else if !errors.Is(err, testCase.wantError) {
				t.Fatalf("LoadOrCreate() error = %v, want %v", err, testCase.wantError)
			}
			if err != nil {
				assertRedactedError(t, err, "synthetic-command-stdout")
			}
			_, fallbackErr := os.Lstat(fallbackPath)
			if testCase.wantFallback && fallbackErr != nil {
				t.Fatalf("fallback key was not created: %v", fallbackErr)
			}
			if !testCase.wantFallback && !errors.Is(fallbackErr, os.ErrNotExist) {
				t.Fatal("operational Keychain failure created a fallback authority")
			}
		})
	}
}

func TestDarwinMasterKeySelectionKeepsFallbackAuthoritativeAfterKeychainRecovers(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "master-key")
	fallbackPath := filepath.Join(directory, "master.key")
	fallbackKey := bytes.Repeat([]byte{0x7a}, masterKeySize)
	firstRunner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
		err: errOSCredentialUnavailable,
	}}}
	first := &darwinMasterKeyProvider{
		authorityPath: filepath.Join(directory, darwinAuthorityFile),
		keychain:      &keychainMasterKeyProvider{runner: firstRunner},
		fallback: newFallbackMasterKeyProvider(
			fallbackPath,
			bytes.NewReader(fallbackKey),
		),
	}
	selected, err := first.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("first LoadOrCreate() error = %v", err)
	}
	defer clearBytes(selected)
	if !bytes.Equal(selected, fallbackKey) {
		t.Fatal("first unavailable Keychain run did not select the fallback key")
	}

	keychainCandidate := bytes.Repeat([]byte{0x7b}, masterKeySize)
	encodedCandidate := base64.StdEncoding.EncodeToString(keychainCandidate)
	recoveredRunner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{},
		{result: keychainCommandResult{stdout: []byte(encodedCandidate + "\n")}},
	}}
	restarted := &darwinMasterKeyProvider{
		authorityPath: filepath.Join(directory, darwinAuthorityFile),
		keychain: &keychainMasterKeyProvider{
			runner: recoveredRunner,
			random: bytes.NewReader(keychainCandidate),
		},
		fallback: newFallbackMasterKeyProvider(
			fallbackPath,
			bytes.NewReader(bytes.Repeat([]byte{0x7c}, masterKeySize)),
		),
	}
	afterRecovery, err := restarted.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("restart LoadOrCreate() error = %v", err)
	}
	defer clearBytes(afterRecovery)
	if !bytes.Equal(afterRecovery, fallbackKey) {
		t.Fatal("Keychain recovery silently switched away from the durable fallback authority")
	}
	if len(recoveredRunner.calls) != 0 {
		t.Fatal("restart consulted or created a second Keychain authority despite an existing fallback key")
	}
}

func TestDarwinMasterKeySelectionKeepsKeychainAuthorityWhenCommandBecomesUnavailable(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "master-key")
	authorityPath := filepath.Join(directory, darwinAuthorityFile)
	fallbackPath := filepath.Join(directory, "master.key")
	keychainKey := bytes.Repeat([]byte{0x81}, masterKeySize)
	encoded := base64.StdEncoding.EncodeToString(keychainKey)
	initialRunner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
		{},
		{result: keychainCommandResult{stdout: []byte(encoded + "\n")}},
	}}
	initial := &darwinMasterKeyProvider{
		authorityPath: authorityPath,
		keychain: &keychainMasterKeyProvider{
			runner: initialRunner,
			random: bytes.NewReader(keychainKey),
		},
		fallback: newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0x82}, masterKeySize))),
	}
	selected, err := initial.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("initial LoadOrCreate() error = %v", err)
	}
	clearBytes(selected)

	unavailableRunner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{{err: errOSCredentialUnavailable}}}
	restarted := &darwinMasterKeyProvider{
		authorityPath: authorityPath,
		keychain:      &keychainMasterKeyProvider{runner: unavailableRunner},
		fallback:      newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0x83}, masterKeySize))),
	}
	key, err := restarted.LoadOrCreate(context.Background())
	clearBytes(key)
	if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("restart LoadOrCreate() error = %v, want unavailable", err)
	}
	if _, err := os.Lstat(fallbackPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temporarily unavailable selected Keychain created a fallback authority")
	}
	authority, err := restarted.readAuthority()
	if err != nil || authority != darwinAuthorityKeychain {
		t.Fatalf("durable authority = %q, error = %v, want Keychain", authority, err)
	}
}

func TestDarwinMasterKeyConcurrentFirstSelectionRereadsFallbackWinner(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "master-key")
	authorityPath := filepath.Join(directory, darwinAuthorityFile)
	fallbackPath := filepath.Join(directory, "master.key")
	providers := []*darwinMasterKeyProvider{
		{
			authorityPath: authorityPath,
			keychain: &keychainMasterKeyProvider{runner: &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
				err: errOSCredentialUnavailable,
			}}}},
			fallback: newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0x91}, masterKeySize))),
		},
		{
			authorityPath: authorityPath,
			keychain: &keychainMasterKeyProvider{runner: &scriptedKeychainRunner{steps: []scriptedKeychainStep{{
				err: errOSCredentialUnavailable,
			}}}},
			fallback: newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0x92}, masterKeySize))),
		},
	}
	results := make([][]byte, len(providers))
	errorsSeen := make([]error, len(providers))
	var waitGroup sync.WaitGroup
	for index, provider := range providers {
		waitGroup.Add(1)
		go func(index int, provider *darwinMasterKeyProvider) {
			defer waitGroup.Done()
			results[index], errorsSeen[index] = provider.LoadOrCreate(context.Background())
		}(index, provider)
	}
	waitGroup.Wait()
	for index, err := range errorsSeen {
		if err != nil {
			t.Fatalf("provider %d LoadOrCreate() error = %v", index, err)
		}
		defer clearBytes(results[index])
	}
	if !bytes.Equal(results[0], results[1]) {
		t.Fatal("concurrent first-selection loser did not re-read the fallback winner")
	}
	authority, err := providers[0].readAuthority()
	if err != nil || authority != darwinAuthorityFallback {
		t.Fatalf("durable authority = %q, error = %v, want fallback", authority, err)
	}
}

func TestDarwinMasterKeyAdoptsExistingFallbackBeforeKeychainProbe(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "master-key")
	fallbackPath := filepath.Join(directory, "master.key")
	fallbackKey := bytes.Repeat([]byte{0xa1}, masterKeySize)
	legacyFallback := newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(fallbackKey))
	created, err := legacyFallback.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("fallback LoadOrCreate() error = %v", err)
	}
	clearBytes(created)
	recoveredRunner := &scriptedKeychainRunner{}
	provider := &darwinMasterKeyProvider{
		authorityPath: filepath.Join(directory, darwinAuthorityFile),
		keychain:      &keychainMasterKeyProvider{runner: recoveredRunner},
		fallback:      newFallbackMasterKeyProvider(fallbackPath, bytes.NewReader(bytes.Repeat([]byte{0xa2}, masterKeySize))),
	}
	selected, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	defer clearBytes(selected)
	if !bytes.Equal(selected, fallbackKey) {
		t.Fatal("existing fallback was not adopted as the durable authority")
	}
	if len(recoveredRunner.calls) != 0 {
		t.Fatal("existing fallback adoption unexpectedly probed Keychain")
	}
}

func TestDarwinMasterKeyMalformedAuthorityFailsClosedWithoutRewrite(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "master-key")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	authorityPath := filepath.Join(directory, darwinAuthorityFile)
	malformed := []byte("analytix-master-key-authority:v1:")
	if err := os.WriteFile(authorityPath, malformed, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runner := &scriptedKeychainRunner{}
	provider := &darwinMasterKeyProvider{
		authorityPath: authorityPath,
		keychain:      &keychainMasterKeyProvider{runner: runner},
		fallback:      newFallbackMasterKeyProvider(filepath.Join(directory, "master.key"), bytes.NewReader(bytes.Repeat([]byte{0xb1}, masterKeySize))),
	}
	key, err := provider.LoadOrCreate(context.Background())
	clearBytes(key)
	if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("LoadOrCreate() error = %v, want unavailable", err)
	}
	if after := mustReadFile(t, authorityPath); !bytes.Equal(after, malformed) {
		t.Fatal("malformed durable authority was overwritten")
	}
	if len(runner.calls) != 0 {
		t.Fatal("malformed durable authority triggered a backend probe")
	}
}

func TestDarwinMasterKeyAuthorityPermissionMismatchFailsClosedWithoutRewrite(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "master-key")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	authorityPath := filepath.Join(directory, darwinAuthorityFile)
	before := []byte(darwinAuthorityFallback)
	if err := os.WriteFile(authorityPath, before, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(authorityPath, 0o644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	runner := &scriptedKeychainRunner{}
	provider := &darwinMasterKeyProvider{
		authorityPath: authorityPath,
		keychain:      &keychainMasterKeyProvider{runner: runner},
		fallback:      newFallbackMasterKeyProvider(filepath.Join(directory, "master.key"), bytes.NewReader(bytes.Repeat([]byte{0xc1}, masterKeySize))),
	}
	key, err := provider.LoadOrCreate(context.Background())
	clearBytes(key)
	if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("LoadOrCreate() error = %v, want unavailable", err)
	}
	if after := mustReadFile(t, authorityPath); !bytes.Equal(after, before) {
		t.Fatal("permission-mismatched durable authority was overwritten")
	}
	if info, statErr := os.Lstat(authorityPath); statErr != nil || info.Mode().Perm() != 0o644 {
		t.Fatal("permission-mismatched durable authority was chmod-repaired")
	}
	if len(runner.calls) != 0 {
		t.Fatal("permission-mismatched durable authority triggered a backend probe")
	}
}

func assertFindKeychainArguments(t *testing.T, call keychainCall) {
	t.Helper()
	want := []string{
		"find-generic-password",
		"-s", keychainService,
		"-a", keychainAccount,
		"-w",
	}
	if len(call.arguments) != len(want) {
		t.Fatalf("find arguments = %v", call.arguments)
	}
	for index := range want {
		if call.arguments[index] != want[index] {
			t.Fatalf("find arguments = %v", call.arguments)
		}
	}
	if len(call.stdin) != 0 {
		t.Fatal("Keychain find unexpectedly received stdin")
	}
}

func TestExplicitTaskKeychainRestartRejectsReplacementAndLostBinding(t *testing.T) {
	for _, change := range []string{"exact-byte-clone", "different-key", "missing", "missing-master-directory", "missing-secret-owner", "corrupt", "legacy-v1"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			database := filepath.Join(root, "analytix-task.keychain-db")
			original := []byte("synthetic-keychain-database")
			if err := os.WriteFile(database, original, 0o600); err != nil {
				t.Fatal(err)
			}
			digest, err := explicitDarwinKeychainSecurityDigest(database)
			if err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(root, "data", "private", "provider-secrets", "credentials.v1.json")
			options := Options{DarwinKeychainDBPath: database, DarwinKeychainBindingDigest: strings.Repeat("a", 64), DarwinKeychainSecurityDigest: digest, DarwinKeychainAuthorityStorePath: store}
			first, err := defaultMasterKeyProvider(store, options)
			if err != nil {
				t.Fatal(err)
			}
			provider := first.(*keychainMasterKeyProvider)
			marker := provider.bindingPath
			switch change {
			case "exact-byte-clone", "different-key":
				if err := os.Rename(database, database+".retained"); err != nil {
					t.Fatal(err)
				}
				value := original
				if change == "different-key" {
					value = []byte("synthetic-different-key-database")
				}
				if err := os.WriteFile(database, value, 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(marker); err != nil {
					t.Fatal(err)
				}
			case "missing-master-directory":
				if err := os.RemoveAll(filepath.Dir(marker)); err != nil {
					t.Fatal(err)
				}
			case "missing-secret-owner":
				registryRoot := filepath.Join(root, "data", "private", "provider-registry")
				if err := os.Mkdir(registryRoot, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(filepath.Dir(filepath.Dir(marker))); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(marker, []byte("corrupt"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "legacy-v1":
				if err := os.WriteFile(marker, []byte(provider.binding.AuthorityDigest+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// Main observes the new inode on each launch. That observation must never
			// replace the previous committed identity, even for an exact-byte clone.
			options.DarwinKeychainSecurityDigest, err = explicitDarwinKeychainSecurityDigest(database)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := defaultMasterKeyProvider(store, options); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatal("restart adopted uncommitted authority")
			}
			actual, err := os.ReadFile(database)
			if err != nil {
				t.Fatal(err)
			}
			if change != "different-key" && !bytes.Equal(actual, original) {
				t.Fatal("failed admission changed retained database")
			}
		})
	}
}

func TestExplicitTaskKeychainUncommittedWriteIdentityRemainsUnavailable(t *testing.T) {
	for _, cut := range []string{"add-error", "wrong-readback", "before-record-replace", "after-record-replace"} {
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			database := filepath.Join(root, "analytix-task.keychain-db")
			if err := os.WriteFile(database, []byte("synthetic-prior-database"), 0o600); err != nil {
				t.Fatal(err)
			}
			digest, err := explicitDarwinKeychainSecurityDigest(database)
			if err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(root, "data", "private", "provider-secrets", "credentials.v1.json")
			options := Options{DarwinKeychainDBPath: database, DarwinKeychainBindingDigest: strings.Repeat("a", 64), DarwinKeychainSecurityDigest: digest, DarwinKeychainAuthorityStorePath: store}
			master, err := defaultMasterKeyProvider(store, options)
			if err != nil {
				t.Fatal(err)
			}
			provider := master.(*keychainMasterKeyProvider)
			before, err := os.ReadFile(provider.bindingPath)
			if err != nil {
				t.Fatal(err)
			}
			candidate := bytes.Repeat([]byte{0x67}, masterKeySize)
			readback := candidate
			if cut == "wrong-readback" {
				readback = bytes.Repeat([]byte{0x68}, masterKeySize)
			}
			runner := &scriptedKeychainRunner{steps: []scriptedKeychainStep{
				{result: keychainCommandResult{exitCode: keychainNotFoundExit}, err: portsecretstore.ErrMasterKeyUnavailable},
				{}, {result: keychainCommandResult{stdout: []byte(base64.StdEncoding.EncodeToString(readback) + "\n")}},
			}}
			if cut == "add-error" {
				runner.steps[1].err = portsecretstore.ErrMasterKeyUnavailable
			}
			runner.beforeRun = func() {
				runner.beforeRun = func() {
					runner.beforeRun = nil
					if err := os.Rename(database, database+".prior"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(database, []byte("synthetic-owned-add-result"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			fault := func() error { return errors.New("synthetic identity commit cut") }
			if cut == "before-record-replace" {
				provider.commitHooks = &atomicCommitHooks{beforeReplace: fault}
			}
			if cut == "after-record-replace" {
				provider.commitHooks = &atomicCommitHooks{afterReplaceBeforeDirectorySync: fault}
			}
			provider.runner = runner
			provider.random = bytes.NewReader(candidate)
			key, err := provider.LoadOrCreate(context.Background())
			clearBytes(key)
			if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatalf("cut returned success: %v", err)
			}
			options.DarwinKeychainSecurityDigest, err = explicitDarwinKeychainSecurityDigest(database)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := defaultMasterKeyProvider(store, options); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatal("restart adopted uncommitted Security effect")
			}
			after, err := os.ReadFile(provider.bindingPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("unconfirmed write changed committed identity")
			}
		})
	}
}

func TestExplicitTaskKeychainUnknownRecordAtCommitIsPreserved(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "analytix-task.keychain-db")
	if err := os.WriteFile(database, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := explicitDarwinKeychainSecurityDigest(database)
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "data", "private", "provider-secrets", "credentials.v1.json")
	options := Options{DarwinKeychainDBPath: database, DarwinKeychainBindingDigest: strings.Repeat("a", 64), DarwinKeychainSecurityDigest: digest, DarwinKeychainAuthorityStorePath: store}
	master, err := defaultMasterKeyProvider(store, options)
	if err != nil {
		t.Fatal(err)
	}
	provider := master.(*keychainMasterKeyProvider)
	unknown := []byte("synthetic-unknown-authority")
	provider.commitHooks = &atomicCommitHooks{beforeReplace: func() error {
		if err := os.WriteFile(provider.bindingPath, unknown, 0o600); err != nil {
			t.Fatal(err)
		}
		return errors.New("synthetic concurrent authority change")
	}}
	if err := provider.commitExplicitIdentity(provider.binding.Security); !errors.Is(err, errAtomicCommitAuthorityChanged) {
		t.Fatal("unknown record did not HOLD")
	}
	assertPreserved := func() {
		value, err := os.ReadFile(provider.bindingPath)
		if err != nil || !bytes.Equal(value, unknown) {
			t.Fatal("unknown record overwritten by rollback")
		}
	}
	assertPreserved()
	if _, err := defaultMasterKeyProvider(store, options); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatal("restart adopted unknown target")
	}
	assertPreserved()
}

func TestExplicitTaskKeychainUnknownBackupAndAmbiguousRecordArePreserved(t *testing.T) {
	for _, mode := range []string{"unknown-backup", "foreign-backup", "duplicate-record", "hardlink-record"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			database := filepath.Join(root, "analytix-task.keychain-db")
			if err := os.WriteFile(database, []byte("synthetic-keychain"), 0o600); err != nil {
				t.Fatal(err)
			}
			digest, err := explicitDarwinKeychainSecurityDigest(database)
			if err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(root, "data", "private", "provider-secrets", "credentials.v1.json")
			options := Options{DarwinKeychainDBPath: database, DarwinKeychainBindingDigest: strings.Repeat("a", 64), DarwinKeychainSecurityDigest: digest, DarwinKeychainAuthorityStorePath: store}
			master, err := defaultMasterKeyProvider(store, options)
			if err != nil {
				t.Fatal(err)
			}
			provider := master.(*keychainMasterKeyProvider)
			binding := provider.bindingPath
			paths := []string{binding}
			switch mode {
			case "unknown-backup", "foreign-backup":
				journal, backup, committed := atomicRecoveryPaths(binding)
				rollback := atomicRollbackRequiredPath(binding)
				body := []byte("unknown-authority")
				if mode == "foreign-backup" {
					record := provider.binding
					record.AuthorityDigest = strings.Repeat("b", 64)
					body, _ = json.Marshal(record)
				}
				for path, content := range map[string][]byte{journal: atomicJournalPriorPresent, backup: body, committed: atomicCommittedMarker, rollback: atomicRollbackRequired} {
					if err := os.WriteFile(path, content, 0o600); err != nil {
						t.Fatal(err)
					}
					paths = append(paths, path)
				}
			case "duplicate-record":
				body, err := os.ReadFile(binding)
				if err != nil {
					t.Fatal(err)
				}
				body = append([]byte(`{"authorityDigest":"ambiguous",`), body[1:]...)
				if err := os.WriteFile(binding, body, 0o600); err != nil {
					t.Fatal(err)
				}
			case "hardlink-record":
				if err := os.Link(binding, binding+".retained-link"); err != nil {
					t.Fatal(err)
				}
			}
			before := map[string][]byte{}
			for _, path := range paths {
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = content
			}
			if _, err := defaultMasterKeyProvider(store, options); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatal("unknown authority recovery did not HOLD")
			}
			for _, path := range paths {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before[path], after) {
					t.Fatal("rejected authority recovery mutated retained evidence")
				}
			}
		})
	}
}

func TestDevelopmentFileAuthorityNeverProbesKeychainOrAdoptsKeychainWinner(t *testing.T) {
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(directory, "credentials.v1.json")
	selected, err := defaultMasterKeyProvider(storePath, Options{DevelopmentFileAuthority: true})
	if err != nil {
		t.Fatal(err)
	}
	provider := selected.(*darwinMasterKeyProvider)
	provider.keychain = nil // Any accidental Keychain path would panic.
	first, err := provider.LoadOrCreate(context.Background())
	if err != nil || len(first) != masterKeySize {
		t.Fatal("development file initialization failed")
	}
	defer clearBytes(first)
	second, err := provider.LoadOrCreate(context.Background())
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("development restart authority changed")
	}
	clearBytes(second)
	if err := os.WriteFile(provider.authorityPath, []byte(darwinAuthorityKeychain), 0600); err != nil {
		t.Fatal(err)
	}
	if value, err := provider.LoadOrCreate(context.Background()); err == nil {
		clearBytes(value)
		t.Fatal("development replaced an OS-backed authority")
	}
	if _, err := defaultMasterKeyProvider(storePath, Options{DevelopmentFileAuthority: true, DarwinKeychainDBPath: "/synthetic/qa.keychain-db"}); err == nil {
		t.Fatal("mixed QA and development options accepted")
	}
}
