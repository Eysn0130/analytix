package processauthority

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeBuildProbeAPIIsAbsentFromOrdinaryAndProductionSourceSets(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve source-set test path")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../.."))
	if _, err := os.Stat(filepath.Join(moduleRoot, "go.mod")); err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	type packageFiles struct {
		GoFiles []string
	}
	buildOnlyFiles := make(map[string]struct{})
	entries, err := os.ReadDir(filepath.Dir(filename))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(filepath.Dir(filename), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		firstLine, _, _ := strings.Cut(string(body), "\n")
		if strings.Contains(firstLine, "analytix_native_build_probe") &&
			strings.Contains(firstLine, "!analytix_prod") &&
			!strings.Contains(firstLine, "!analytix_native_build_probe") {
			buildOnlyFiles[entry.Name()] = struct{}{}
		}
	}
	if len(buildOnlyFiles) == 0 {
		t.Fatal("no build-only process authority files discovered")
	}
	for _, targetOS := range []string{"darwin", "linux", "windows"} {
		for _, sourceSet := range []struct {
			name string
			tags string
		}{
			{name: "ordinary"},
			{name: "production", tags: "analytix_prod"},
			{name: "production-with-probe-tag", tags: "analytix_prod,analytix_native_build_probe"},
		} {
			t.Run(targetOS+"/"+sourceSet.name, func(t *testing.T) {
				commandEnvironment := append(os.Environ(), "GOOS="+targetOS, "GOARCH=amd64", "CGO_ENABLED=0")
				arguments := []string{"list", "-json"}
				if sourceSet.tags != "" {
					arguments = append(arguments, "-tags", sourceSet.tags)
				}
				arguments = append(arguments, "./internal/adapters/outbound/processauthority")
				command := exec.Command("go", arguments...)
				command.Dir = moduleRoot
				command.Env = commandEnvironment
				body, err := command.Output()
				if err != nil {
					t.Fatalf("go list process authority: %v", err)
				}
				var listed packageFiles
				if json.Unmarshal(body, &listed) != nil {
					t.Fatalf("decode go list output: %q", body)
				}
				listedFiles := make(map[string]struct{}, len(listed.GoFiles))
				for _, filename := range listed.GoFiles {
					listedFiles[filename] = struct{}{}
					if _, forbidden := buildOnlyFiles[filename]; forbidden {
						t.Fatalf("%s includes build-only file %q", sourceSet.name, filename)
					}
				}
				if strings.Contains(sourceSet.tags, "analytix_prod") {
					for _, filename := range []string{
						"bootstrap.go",
						"bootstrap_darwin.go",
						"session_open_nonprod_darwin.go",
					} {
						if _, included := listedFiles[filename]; included {
							t.Fatalf("%s includes non-production bootstrap source %q", sourceSet.name, filename)
						}
					}
					required := []string{"session_unsupported.go"}
					if targetOS == "darwin" {
						required = []string{
							"bootstrap_disabled_prod_darwin.go",
							"bootstrap_identity_prod.go",
							"session_open_prod_darwin.go",
						}
					}
					for _, filename := range required {
						if _, included := listedFiles[filename]; !included {
							t.Fatalf("%s omits production direct-session source %q", sourceSet.name, filename)
						}
					}
				}

				arguments = []string{"list"}
				if sourceSet.tags != "" {
					arguments = append(arguments, "-tags", sourceSet.tags)
				}
				arguments = append(arguments, "./internal/adapters/outbound/nativecomponentpublication")
				command = exec.Command("go", arguments...)
				command.Dir = moduleRoot
				command.Env = commandEnvironment
				combined, listErr := command.CombinedOutput()
				if listErr == nil || !strings.Contains(string(combined), "build constraints exclude all Go files") {
					t.Fatalf("%s unexpectedly exposes native generation publisher: err=%v output=%q", sourceSet.name, listErr, combined)
				}

				arguments = []string{"list"}
				if sourceSet.tags != "" {
					arguments = append(arguments, "-tags", sourceSet.tags)
				}
				arguments = append(arguments, "./cmd/native-component-build-probe")
				command = exec.Command("go", arguments...)
				command.Dir = moduleRoot
				command.Env = commandEnvironment
				combined, listErr = command.CombinedOutput()
				if listErr == nil || !strings.Contains(string(combined), "build constraints exclude all Go files") {
					t.Fatalf("%s unexpectedly exposes build probe command: err=%v output=%q", sourceSet.name, listErr, combined)
				}

				arguments = []string{"list", "-deps"}
				if sourceSet.tags != "" {
					arguments = append(arguments, "-tags", sourceSet.tags)
				}
				arguments = append(arguments, "./cmd/runtime-server")
				command = exec.Command("go", arguments...)
				command.Dir = moduleRoot
				command.Env = commandEnvironment
				dependencies, dependencyErr := command.Output()
				if dependencyErr != nil {
					t.Fatalf("%s list runtime-server dependencies: %v", sourceSet.name, dependencyErr)
				}
				for _, dependency := range strings.Fields(string(dependencies)) {
					if strings.Contains(dependency, "nativecomponentpublication") || strings.HasSuffix(dependency, "/cmd/native-component-build-probe") {
						t.Fatalf("%s runtime-server imports build-only authority %q", sourceSet.name, dependency)
					}
				}
			})
		}
	}
}
