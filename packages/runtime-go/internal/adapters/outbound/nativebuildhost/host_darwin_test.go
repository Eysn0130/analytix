//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativebuildhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	appnativebuild "analytix.local/runtime-go/internal/app/nativebuild"
)

func TestProductionHostBlocksBeforeAnyBuildEffectWithoutExternalAuthority(t *testing.T) {
	repository, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	publication := filepath.Join(repository, "runtime", "native-components", hostTargetV1())
	if err := os.MkdirAll(filepath.Dir(publication), 0o700); err != nil {
		t.Fatal(err)
	}
	host, err := OpenProductionV1(ConfigV1{
		RepositoryRoot: repository, PublicationRoot: publication, TargetKey: hostTargetV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := appnativebuild.NewCoordinator(host)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.RunV1(context.Background(), appnativebuild.RequestV1{
		RequestNonce: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != appnativebuild.StatusBlocked || result.Blocker != "authority_identity_unbound" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Lstat(publication); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blocked preflight created publication state: %v", err)
	}
}

func TestProductionHostRejectsCallerSelectedTargetAndEscapingPublicationRoot(t *testing.T) {
	repository, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wrongTarget := "darwin-arm64"
	if hostTargetV1() == wrongTarget {
		wrongTarget = "darwin-x64"
	}
	for name, config := range map[string]ConfigV1{
		"wrong target": {
			RepositoryRoot: repository, PublicationRoot: filepath.Join(repository, "native"), TargetKey: wrongTarget,
		},
		"outside repository": {
			RepositoryRoot: repository, PublicationRoot: filepath.Join(filepath.Dir(repository), "native"), TargetKey: hostTargetV1(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := OpenProductionV1(config); !errors.Is(err, ErrConfigInvalid) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func hostTargetV1() string {
	if runtime.GOARCH == "arm64" {
		return "darwin-arm64"
	}
	return "darwin-x64"
}
