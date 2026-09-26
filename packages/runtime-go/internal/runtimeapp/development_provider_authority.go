package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	providerregistryfs "analytix.local/runtime-go/internal/adapters/outbound/providerregistryfs"
	secretstore "analytix.local/runtime-go/internal/adapters/outbound/secretstore"
)

func developmentProviderAuthorityDigest(config Config) string {
	if config.DevelopmentProviderAuthorityDir == "" {
		return ""
	}
	return startupValueDigest(config.DevelopmentProviderAuthorityDir)
}

// This is a separate lifecycle of the SAME Registry/Secret Store owner. Task
// data remains task-local. Registry transactions retain their OS lock and
// generation fences across processes. Semantic planning must not recover or
// initialize this independent authority; activation performs normal recovery.
func openDevelopmentProviderAuthority(ctx context.Context, config Config, simulation bool) (*providerRegistryAuthorityV1, error) {
	if err := validateDevelopmentProviderAuthority(config); err != nil {
		return nil, err
	}
	root := config.DevelopmentProviderAuthorityDir
	options := secretstore.Options{DevelopmentFileAuthority: true}
	if runtime.GOOS == "windows" {
		// Windows source development shares the same Registry/Secret Store owner,
		// with its existing CurrentUser DPAPI master-key backend.
		options = secretstore.Options{}
	}
	secrets, err := secretstore.NewWithOptions(filepath.Join(root, "private", "provider-secrets", providerRegistrySecretStoreFileV1), providerRegistrySecretAuthorizerV1{}, options)
	if err != nil {
		return nil, err
	}
	registry, err := providerregistryfs.New(root)
	if err != nil {
		_ = secrets.Close()
		return nil, err
	}
	return finishProviderRegistryAuthorityV1(ctx, registry, secrets, !simulation)
}

func validateDevelopmentProviderAuthority(config Config) error {
	root := config.DevelopmentProviderAuthorityDir
	if root == "" {
		return nil
	}
	invalid := errors.New("development Provider authority is unavailable")
	if !developmentProviderAuthorityEnabled || !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Base(root) != "provider-credentials" || len(root) > 1024 || config.DarwinSecretStoreKeychainDBPath != "" || config.DarwinSecretStoreKeychainBindingDigest != "" || config.DarwinSecretStoreKeychainSecurityDigest != "" {
		return invalid
	}
	for _, path := range []string{root, filepath.Dir(root)} {
		info, err := os.Lstat(path)
		real, realErr := filepath.EvalSymlinks(path)
		if err != nil || realErr != nil || real != path || !info.IsDir() || !developmentProviderDirectorySecure(path, info) {
			return invalid
		}
	}
	for _, taskPath := range []string{config.DataDir, config.UserDataDir} {
		if taskPath == "" {
			continue
		}
		for _, pair := range [][2]string{{root, taskPath}, {taskPath, root}} {
			rel, err := filepath.Rel(pair[0], pair[1])
			if err != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
				return invalid
			}
		}
	}
	return nil
}
