//go:build unix

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

func TestRuntimeDarwinSecretStoreKeychainBindingMatchesCLIAndCurrentFilesystem(t *testing.T) {
	binding := runtimeDarwinKeychainBindingFixtureV1(t)
	config, err := binding.config(binding.UserDataDir, binding.DataDir)
	if err != nil {
		t.Fatalf("config() error = %v", err)
	}
	if config.DBPath != binding.KeychainDBPath || config.BindingDigest != binding.BindingDigest ||
		config.SecurityDigest != binding.KeychainSecurityDigest {
		t.Fatalf("private Keychain config changed: %#v", config)
	}
	if _, err := binding.config(filepath.Join(binding.IsolationRoot, "other-user-data"), binding.DataDir); err == nil {
		t.Fatal("mismatched userDataDir was accepted")
	}
	if _, err := binding.config(binding.UserDataDir, filepath.Join(binding.IsolationRoot, "other-data")); err == nil {
		t.Fatal("mismatched dataDir was accepted")
	}
}

func TestRuntimeDarwinSecretStoreKeychainBindingRejectsPermissionLinkAndIdentityDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, runtimeDarwinSecretStoreKeychainBindingV1)
	}{
		{"permission", func(t *testing.T, binding runtimeDarwinSecretStoreKeychainBindingV1) {
			if err := os.Chmod(binding.KeychainDBPath, 0o640); err != nil {
				t.Fatal(err)
			}
		}},
		{"hardlink", func(t *testing.T, binding runtimeDarwinSecretStoreKeychainBindingV1) {
			if err := os.Link(binding.KeychainDBPath, binding.KeychainDBPath+".hardlink"); err != nil {
				t.Fatal(err)
			}
		}},
		{"drift", func(t *testing.T, binding runtimeDarwinSecretStoreKeychainBindingV1) {
			if err := os.Remove(binding.KeychainDBPath); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(binding.KeychainDBPath, []byte("replacement"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			binding := runtimeDarwinKeychainBindingFixtureV1(t)
			test.mutate(t, binding)
			if _, err := binding.config(binding.UserDataDir, binding.DataDir); err == nil {
				t.Fatal("invalid current Keychain identity was accepted")
			}
		})
	}
}

func TestRuntimeDarwinSecretStoreKeychainBindingRejectsUnsafeInteractiveTokenPaths(t *testing.T) {
	binding := runtimeDarwinKeychainBindingFixtureV1(t)
	for _, unsafe := range []string{
		filepath.Join(binding.IsolationRoot, "runtime data"),
		filepath.Join(binding.IsolationRoot, "runtime'data"),
		binding.DataDir + "\nquit",
		binding.DataDir + "\\child",
	} {
		mutated := binding
		mutated.DataDir = unsafe
		if mutated.validateDocument() == nil {
			t.Fatalf("unsafe interactive-token path was accepted")
		}
	}
}

func TestRuntimeDarwinSecretStoreKeychainBindingRejectsLoginSubstringBeforeFilesystem(t *testing.T) {
	root := "/private/synthetic-task"
	binding := runtimeDarwinSecretStoreKeychainBindingV1{
		SchemaVersion: 1, Purpose: runtimeDarwinSecretStoreKeychainBindingPurposeV1,
		ExternalStateMode: runtimeExternalStateModeIsolatedLocalV1,
		IsolationRoot:     root, UserDataDir: root + "/user-data", DataDir: root + "/runtime-data",
		KeychainDBPath:         root + "/darwin-secret-store-keychain/analytix-task.keychain-db",
		KeychainSecurityDigest: runtimeSHA256HexBytesV1([]byte("synthetic")),
	}
	if err := runtimeSignDarwinKeychainBindingFixtureV1(binding).validateDocument(); err != nil {
		t.Fatal("valid pure document rejected")
	}
	for _, unsafe := range []string{
		"/private/prefix-login.keychain-suffix/task", "/private/LoGiN.KeYcHaIn/task",
		root + "/darwin-secret-store-keychain/login.keychain-db", root + "/child/../runtime-data",
		root + "//runtime-data", root + "/runtime-data\nquit",
	} {
		for field := 0; field < 4; field++ {
			mutated := binding
			switch field {
			case 0:
				mutated.IsolationRoot = unsafe
			case 1:
				mutated.UserDataDir = unsafe
			case 2:
				mutated.DataDir = unsafe
			case 3:
				mutated.KeychainDBPath = unsafe
			}
			// Re-sign so rejection proves path admission, not a stale hash.
			if runtimeSignDarwinKeychainBindingFixtureV1(mutated).validateDocument() == nil {
				t.Fatal("unsafe pure binding admitted")
			}
		}
	}
}

func runtimeDarwinKeychainBindingFixtureV1(t *testing.T) runtimeDarwinSecretStoreKeychainBindingV1 {
	t.Helper()
	root := filepath.Join(t.TempDir(), "isolated")
	userDataDir := filepath.Join(root, "user-data")
	dataDir := filepath.Join(root, "runtime-data")
	keychainDir := filepath.Join(root, "darwin-secret-store-keychain")
	for _, path := range []string{root, userDataDir, dataDir, keychainDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	databasePath := filepath.Join(keychainDir, "analytix-task.keychain-db")
	if err := os.WriteFile(databasePath, []byte("synthetic-keychain-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := os.Lstat(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	stat := identity.Sys().(*syscall.Stat_t)
	securityDocument := struct {
		SchemaVersion int    `json:"schemaVersion"`
		PathDigest    string `json:"pathDigest"`
		Device        string `json:"device"`
		Inode         string `json:"inode"`
		Owner         string `json:"owner"`
		Mode          string `json:"mode"`
		Links         string `json:"links"`
	}{1, runtimeSHA256HexBytesV1([]byte(databasePath)), strconv.FormatUint(uint64(stat.Dev), 10),
		strconv.FormatUint(stat.Ino, 10), strconv.FormatUint(uint64(stat.Uid), 10),
		strconv.FormatUint(uint64(identity.Mode().Perm()), 8), strconv.FormatUint(uint64(stat.Nlink), 10)}
	securityJSON, _ := json.Marshal(securityDocument)
	binding := runtimeDarwinSecretStoreKeychainBindingV1{
		SchemaVersion: 1, Purpose: runtimeDarwinSecretStoreKeychainBindingPurposeV1,
		ExternalStateMode: runtimeExternalStateModeIsolatedLocalV1,
		IsolationRoot:     root, UserDataDir: userDataDir, DataDir: dataDir,
		KeychainDBPath: databasePath, KeychainSecurityDigest: runtimeSHA256HexBytesV1(securityJSON),
	}
	return runtimeSignDarwinKeychainBindingFixtureV1(binding)
}

func runtimeSignDarwinKeychainBindingFixtureV1(binding runtimeDarwinSecretStoreKeychainBindingV1) runtimeDarwinSecretStoreKeychainBindingV1 {
	document := struct {
		SchemaVersion          int    `json:"schemaVersion"`
		Purpose                string `json:"purpose"`
		ExternalStateMode      string `json:"externalStateMode"`
		IsolationRoot          string `json:"isolationRoot"`
		UserDataDir            string `json:"userDataDir"`
		DataDir                string `json:"dataDir"`
		KeychainDBPath         string `json:"keychainDBPath"`
		KeychainSecurityDigest string `json:"keychainSecurityDigest"`
	}{1, binding.Purpose, binding.ExternalStateMode, binding.IsolationRoot, binding.UserDataDir,
		binding.DataDir, binding.KeychainDBPath, binding.KeychainSecurityDigest}
	bindingJSON, _ := json.Marshal(document)
	binding.BindingDigest = runtimeSHA256HexBytesV1(bindingJSON)
	return binding
}
