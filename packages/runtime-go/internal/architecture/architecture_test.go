package architecture_test

import (
	"encoding/json"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRuntimeGoLayerImportBoundaries(t *testing.T) {
	root := runtimeGoRoot(t)
	checkImports(t, root, "internal/domain", []string{
		"net/http",
		"os",
		"os/exec",
		"io/fs",
		"path/filepath",
		"analytix.local/runtime-go/internal/app",
		"analytix.local/runtime-go/internal/adapters",
		"analytix.local/runtime-go/internal/conformance",
		"analytix.local/runtime-go/internal/ports",
		"analytix.local/runtime-go/internal/server",
	})
	checkImports(t, root, "internal/app", []string{
		"net/http",
		"os",
		"os/exec",
		"io/fs",
		"path/filepath",
		"analytix.local/runtime-go/internal/adapters",
		"analytix.local/runtime-go/internal/agent",
		"analytix.local/runtime-go/internal/conformance",
		"analytix.local/runtime-go/internal/jobs",
		"analytix.local/runtime-go/internal/goal",
		"analytix.local/runtime-go/internal/mcp",
		"analytix.local/runtime-go/internal/proc",
		"analytix.local/runtime-go/internal/provider",
		"analytix.local/runtime-go/internal/server",
	})
	checkImports(t, root, "internal/adapters/inbound", []string{
		"analytix.local/runtime-go/internal/adapters/outbound",
	})
	checkImports(t, root, "internal/adapters/outbound", []string{
		"analytix.local/runtime-go/internal/adapters/inbound",
		"analytix.local/runtime-go/internal/conformance",
		"analytix.local/runtime-go/internal/server",
	})
}

func TestRuntimeGoConformanceCodeStaysOutOfProduction(t *testing.T) {
	root := runtimeGoRoot(t)
	conformanceDir := filepath.Join(root, "internal", "conformance")
	if _, err := os.Stat(conformanceDir); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("stat conformance dir: %v", err)
	}
	for _, file := range goFiles(t, conformanceDir) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		if !hasBuildTag(t, file, "!analytix_prod") {
			t.Fatalf("%s must be guarded by //go:build !analytix_prod", rel(t, root, file))
		}
	}
}

func TestRuntimeGoProductionSurfaceDoesNotExposeUpstreamRoutes(t *testing.T) {
	root := runtimeGoRoot(t)
	forbidden := []string{"/v1/reasonix", "/v1/subagents", "/v1/workflow", "/v1/conformance"}
	for _, dir := range []string{
		filepath.Join(root, "internal", "server"),
		filepath.Join(root, "internal", "adapters", "inbound"),
	} {
		for _, file := range goFiles(t, dir) {
			if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
				continue
			}
			parsed := parseGoFile(t, file, parser.ParseComments)
			ast.Inspect(parsed, func(node ast.Node) bool {
				lit, ok := node.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				for _, route := range forbidden {
					if strings.Contains(value, route) {
						t.Fatalf("%s exposes forbidden production route string %q", rel(t, root, file), route)
					}
				}
				return true
			})
		}
	}
}

func TestProviderDraftTextHasNoDirectPublicPersistencePath(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, relative := range []string{
		"internal/app/loop/runtime_runner.go",
		"internal/server/agent_loop.go",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"AssistantTextDeltaWithTrace(",
			"persistRuntimeAssistantProcessText(",
			"PersistAssistantProcessText(",
		} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("provider draft loop %s contains direct public persistence call %q", relative, forbidden)
			}
		}
	}
}

func TestPersistenceKernelDeviceHasSingleProductionOwner(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/adapters/outbound/persistencefs/lease_kernel_unix.go"
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err == nil && value == "/dev/zero" && rel(t, root, file) != allowed {
				t.Fatalf("%s opens the persistence kernel arbitration device outside its sole owner", rel(t, root, file))
			}
			return true
		})
	}
}

func TestEvidenceRegistryPreparedCommitHasSingleProductionCaller(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/app/evidence/receipt_settlement.go"
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "CommitPrepared" {
				return true
			}
			if rel(t, root, file) != allowed {
				t.Fatalf("%s bypasses the sole settlement-backed evidence registry writer", rel(t, root, file))
			}
			return true
		})
	}
}

func TestNativeComponentRequestHasSingleProductionConstructor(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/app/nativecomponent/service.go"
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		aliases := importedAliases(parsed, "analytix.local/runtime-go/internal/domain/nativecomponent")
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, identifierOK := selector.X.(*ast.Ident)
			if identifierOK && selector.Sel.Name == "Request" && aliases[identifier.Name] && rel(t, root, file) != allowed {
				t.Fatalf("%s constructs a native request outside durable app admission", rel(t, root, file))
			}
			return true
		})
	}
}

func TestNativeBuildLiveAuthorityHasSingleBuildOnlyCoordinator(t *testing.T) {
	root := runtimeGoRoot(t)
	authorityImport := "analytix.local/runtime-go/internal/domain/nativebuild"
	authoritySymbols := map[string]bool{
		"CargoExecutionObserverV1":         true,
		"LiveCargoExecutionV1":             true,
		"PublicationPermitV1":              true,
		"PublicationBindingV1":             true,
		"BeginCargoExecutionObservationV1": true,
		"FinalizeObservedCargoExecutionV1": true,
		"AuthorizeForPublicationV1":        true,
		"ConsumeExactPublicationPermitV1":  true,
	}
	allowed := map[string]map[string]bool{
		"internal/app/nativebuild/coordinator.go": {
			"BeginCargoExecutionObservationV1": true, "FinalizeObservedCargoExecutionV1": true,
			"AuthorizeForPublicationV1": true, "PublicationBindingV1": true,
		},
		"internal/ports/nativebuild/host.go": {
			"PublicationPermitV1": true, "PublicationBindingV1": true,
		},
		"internal/adapters/outbound/nativecomponentpublication/publisher_darwin.go": {
			"PublicationPermitV1": true, "PublicationBindingV1": true,
			"ConsumeExactPublicationPermitV1": true,
		},
		"internal/adapters/outbound/nativebuildhost/host_darwin.go": {
			"PublicationPermitV1": true, "PublicationBindingV1": true,
		},
	}
	seen := map[string]int{}
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, parser.ParseComments)
		location := rel(t, root, file)
		aliases := map[string]bool{}
		for _, item := range parsed.Imports {
			path, err := strconv.Unquote(item.Path.Value)
			if err != nil || path != authorityImport {
				continue
			}
			alias := "nativebuild"
			if item.Name != nil {
				alias = item.Name.Name
			}
			if alias == "." || alias == "_" {
				t.Fatalf("%s uses forbidden native-build authority import mode %q", location, alias)
			}
			if _, ok := allowed[location]; !ok {
				t.Fatalf("%s imports native-build live authority outside the exact build-only closure", location)
			}
			aliases[alias] = true
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !authoritySymbols[selector.Sel.Name] {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok || !aliases[identifier.Name] {
				return true
			}
			if !allowed[location][selector.Sel.Name] {
				t.Fatalf("%s references unadvertised native-build authority %s", location, selector.Sel.Name)
			}
			seen[location+"#"+selector.Sel.Name]++
			return true
		})
		for _, comment := range parsed.Comments {
			for _, line := range comment.List {
				if strings.Contains(line.Text, "go:linkname") {
					for symbol := range authoritySymbols {
						if strings.Contains(line.Text, symbol) {
							t.Fatalf("%s links to native-build live authority %s", location, symbol)
						}
					}
				}
			}
		}
	}
	for key, want := range map[string]int{
		"internal/app/nativebuild/coordinator.go#BeginCargoExecutionObservationV1":                                  1,
		"internal/app/nativebuild/coordinator.go#FinalizeObservedCargoExecutionV1":                                  1,
		"internal/app/nativebuild/coordinator.go#AuthorizeForPublicationV1":                                         1,
		"internal/adapters/outbound/nativecomponentpublication/publisher_darwin.go#ConsumeExactPublicationPermitV1": 1,
	} {
		if seen[key] != want {
			t.Fatalf("native-build authority reference %s count = %d, want %d", key, seen[key], want)
		}
	}
	for location := range allowed {
		if !buildConstraintImpliesNativeProbeOnly(t, filepath.Join(root, filepath.FromSlash(location))) {
			t.Fatalf("%s build constraint must imply analytix_native_build_probe && !analytix_prod", location)
		}
	}
	authorityFile := filepath.Join(root, "internal", "domain", "nativebuild", "cargo_execution.go")
	if !buildConstraintEquivalentV1(t, authorityFile, "analytix_native_build_probe && !analytix_prod") {
		t.Fatal("native-build authority file must use the exact probe-only constraint")
	}
	for _, file := range goFiles(t, filepath.Dir(authorityFile)) {
		if strings.HasSuffix(file, "_test.go") || file == authorityFile {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && authoritySymbols[identifier.Name] {
				t.Fatalf("%s references live authority inside the same package", rel(t, root, file))
			}
			return true
		})
	}
	for _, relative := range []string{
		"internal/runtimeapp",
		"internal/server",
		"cmd/runtime-server",
	} {
		for _, file := range goFiles(t, filepath.Join(root, filepath.FromSlash(relative))) {
			parsed := parseGoFile(t, file, 0)
			for _, importPath := range []string{
				"analytix.local/runtime-go/internal/app/nativebuild",
				"analytix.local/runtime-go/internal/ports/nativebuild",
			} {
				if len(importedAliases(parsed, importPath)) != 0 {
					t.Fatalf("%s imports build-only native coordinator surface", rel(t, root, file))
				}
			}
		}
	}
}

func TestNativeBuildAuthorityAbsentFromProductionSourceSets(t *testing.T) {
	root := runtimeGoRoot(t)
	files := []string{
		"internal/domain/nativebuild/cargo_execution.go",
		"internal/app/nativebuild/coordinator.go",
		"internal/ports/nativebuild/host.go",
		"internal/adapters/outbound/nativecomponentpublication/publisher_darwin.go",
		"internal/adapters/outbound/nativebuildhost/host_darwin.go",
	}
	for _, platform := range [][2]string{{"darwin", "arm64"}, {"darwin", "amd64"}, {"linux", "amd64"}, {"windows", "amd64"}} {
		for _, relative := range files {
			file := filepath.Join(root, filepath.FromSlash(relative))
			context := build.Default
			context.GOOS, context.GOARCH = platform[0], platform[1]
			context.BuildTags = []string{"analytix_prod", "analytix_native_build_probe"}
			matched, err := context.MatchFile(filepath.Dir(file), filepath.Base(file))
			if err != nil {
				t.Fatalf("match %s for %s/%s: %v", relative, platform[0], platform[1], err)
			}
			if matched {
				t.Fatalf("%s entered analytix_prod source set for %s/%s", relative, platform[0], platform[1])
			}
		}
	}
}

func TestNativeBuildConstraintImplicationRejectsLookalikes(t *testing.T) {
	target, err := constraint.Parse("//go:build analytix_native_build_probe && !analytix_prod")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		expression string
		want       bool
	}{
		{expression: "analytix_native_build_probe && !analytix_prod", want: true},
		{expression: "darwin && analytix_native_build_probe && !analytix_prod", want: true},
		{expression: "analytix_native_build_probe && !analytix_prod_fake", want: false},
		{expression: "(analytix_native_build_probe && !analytix_prod) || debug", want: false},
		{expression: "!analytix_prod", want: false},
	}
	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			actual, parseErr := constraint.Parse("//go:build " + test.expression)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			if got := buildConstraintImplicationV1(t, actual, target); got != test.want {
				t.Fatalf("implication=%v, want %v", got, test.want)
			}
		})
	}
}

func TestProcessAuthorityProductionCallersAreClosed(t *testing.T) {
	root := runtimeGoRoot(t)
	authorityPath := "analytix.local/runtime-go/internal/adapters/outbound/processauthority"
	expected := map[string]map[string]bool{
		"internal/adapters/outbound/nativecomponentrunner/runner.go": {
			"Session": true,
		},
		"internal/adapters/outbound/nativecomponentrunner/runner_darwin.go": {
			"OpenSession": true, "SessionConfig": true, "ReadOnlyInput": true, "ReadWriteOutput": true,
			"ErrProtocol": true, "ErrOutputLimit": true,
			"ErrAbnormalExit": true, "ErrExecutableIdentity": true, "ErrTermination": true,
			"ErrCanceled": true, "ErrTimeout": true,
		},
		"internal/adapters/outbound/nativecomponentregistry/platform_verifier_darwin.go": {
			"InspectSuspendedDarwinExecutable": true, "SuspendedInspectionConfig": true,
			"SuspendedProcessIdentity": true, "ExecuteSystemTool": true, "SystemToolRequest": true,
			"SystemToolDarwinCodeSign": true, "TerminationExited": true,
		},
	}
	seen := map[string]bool{}
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		alias := ""
		for _, item := range parsed.Imports {
			path, err := strconv.Unquote(item.Path.Value)
			if err != nil || path != authorityPath {
				continue
			}
			if alias != "" {
				t.Fatalf("%s imports processauthority more than once", rel(t, root, file))
			}
			alias = "processauthority"
			if item.Name != nil {
				if item.Name.Name == "." || item.Name.Name == "_" {
					t.Fatalf("%s uses a forbidden processauthority import mode %q", rel(t, root, file), item.Name.Name)
				}
				alias = item.Name.Name
			}
		}
		if alias == "" {
			continue
		}
		relative := rel(t, root, file)
		allowed, ok := expected[relative]
		if !ok {
			t.Fatalf("%s imports processauthority outside the exact production allowlist", relative)
		}
		actual := map[string]bool{}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, identifierOK := selector.X.(*ast.Ident)
			if !identifierOK || identifier.Name != alias {
				return true
			}
			if !allowed[selector.Sel.Name] {
				t.Fatalf("%s uses unadvertised processauthority.%s", relative, selector.Sel.Name)
			}
			actual[selector.Sel.Name] = true
			return true
		})
		if len(actual) != len(allowed) {
			t.Fatalf("%s processauthority selectors = %#v, want %#v", relative, actual, allowed)
		}
		seen[relative] = true
	}
	for file := range expected {
		if !seen[file] {
			t.Fatalf("%s lost its exact processauthority production binding", file)
		}
	}
}

func TestRuntimeServerProductionDarwinDirectSessionIsClosed(t *testing.T) {
	root := runtimeGoRoot(t)
	mainPath := filepath.Join(root, "cmd", "runtime-server", "main.go")
	mainFile := parseGoFile(t, mainPath, 0)
	for _, item := range mainFile.Imports {
		importPath, err := strconv.Unquote(item.Path.Value)
		if err == nil && importPath == "analytix.local/runtime-go/internal/adapters/outbound/processauthority" {
			t.Fatal("runtime-server reintroduced the processauthority bootstrap import")
		}
	}
	authorityDirectory := filepath.Join(root, "internal", "adapters", "outbound", "processauthority")
	productionStub := filepath.Join(authorityDirectory, "session_unsupported.go")
	for goos, architectures := range map[string][]string{
		"darwin":  {"amd64", "arm64"},
		"linux":   {"amd64"},
		"windows": {"amd64"},
	} {
		for _, goarch := range architectures {
			t.Run(goos+"/"+goarch+" source set", func(t *testing.T) {
				files := analytixProdGoSourceSet(t, authorityDirectory, goos, goarch)
				openSessionDefinitions := []string{}
				initDefinitions := []string{}
				execDefinitions := []string{}
				sourceSet := map[string]bool{}
				for _, file := range files {
					base := filepath.Base(file)
					sourceSet[base] = true
					parsed := parseGoFile(t, file, 0)
					execAliases := map[string]bool{}
					for _, item := range parsed.Imports {
						importPath, err := strconv.Unquote(item.Path.Value)
						if err != nil || (importPath != "syscall" && importPath != "golang.org/x/sys/unix") {
							continue
						}
						alias := filepath.Base(importPath)
						if item.Name != nil {
							if item.Name.Name == "." || item.Name.Name == "_" {
								t.Fatalf("%s uses forbidden production import mode %q for %s", rel(t, root, file), item.Name.Name, importPath)
							}
							alias = item.Name.Name
						}
						execAliases[alias] = true
					}
					for _, declaration := range parsed.Decls {
						function, ok := declaration.(*ast.FuncDecl)
						if !ok {
							continue
						}
						if function.Recv != nil {
							continue
						}
						switch function.Name.Name {
						case "init":
							initDefinitions = append(initDefinitions, file)
						case "openSession":
							openSessionDefinitions = append(openSessionDefinitions, file)
						}
					}
					ast.Inspect(parsed, func(node ast.Node) bool {
						selector, ok := node.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "Exec" {
							return true
						}
						identifier, ok := selector.X.(*ast.Ident)
						if ok && execAliases[identifier.Name] {
							execDefinitions = append(execDefinitions, file)
						}
						return true
					})
				}
				wantSession := "session_unsupported.go"
				required := []string{wantSession}
				forbidden := []string{
					"bootstrap.go", "bootstrap_darwin.go", "bootstrap_unsupported.go",
					"bootstrap_identity_nonprod.go", "bootstrap_identity_prod.go",
					"session_darwin.go", "session_open_nonprod_darwin.go", "session_open_prod_darwin.go",
				}
				if goos == "darwin" {
					wantSession = "session_open_prod_darwin.go"
					required = []string{
						"bootstrap_identity_prod.go", "session_darwin.go", wantSession,
						"session_support_darwin.go", "owner_liveness_darwin.go", "process_transition_darwin.go",
					}
					forbidden = []string{
						"bootstrap.go", "bootstrap_darwin.go", "bootstrap_unsupported.go",
						"bootstrap_identity_nonprod.go", "session_unsupported.go", "session_open_nonprod_darwin.go",
					}
				}
				if len(openSessionDefinitions) != 1 || filepath.Base(openSessionDefinitions[0]) != wantSession {
					t.Fatalf("%s/%s analytix_prod openSession definitions = %#v, want only %s", goos, goarch, openSessionDefinitions, wantSession)
				}
				if len(initDefinitions) != 0 {
					t.Fatalf("%s/%s analytix_prod init definitions = %#v, want none", goos, goarch, initDefinitions)
				}
				if len(execDefinitions) != 0 {
					t.Fatalf("%s/%s analytix_prod syscall.Exec definitions = %#v, want none", goos, goarch, execDefinitions)
				}
				for _, base := range required {
					if !sourceSet[base] {
						t.Fatalf("%s/%s analytix_prod source set lost %s", goos, goarch, base)
					}
				}
				for _, base := range forbidden {
					if sourceSet[base] {
						t.Fatalf("%s/%s analytix_prod source set unexpectedly includes %s", goos, goarch, base)
					}
				}
			})
		}
	}
	assertExactSource(t, productionStub, `//go:build !darwin

package processauthority

import "context"

func openSession(_ context.Context, config SessionConfig) (Session, error) {
	if config.Executable != nil {
		_ = config.Executable.Close()
	}
	if config.ReadOnlyInput != nil && config.ReadOnlyInput.File != nil {
		_ = config.ReadOnlyInput.File.Close()
	}
	if config.ReadWriteOutput != nil && config.ReadWriteOutput.File != nil {
		_ = config.ReadWriteOutput.File.Close()
	}
	return nil, ErrUnavailable
}
`)
	assertExactSource(t, filepath.Join(authorityDirectory, "bootstrap_identity_prod.go"), `//go:build darwin && analytix_prod

package processauthority

func currentBootstrapMainAllowed() bool {
	return false
}
`)
	assertExactSource(t, filepath.Join(authorityDirectory, "session_support_darwin.go"), `//go:build darwin

package processauthority

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const darwinSessionCleanupDeadline = 5 * time.Second

func darwinDirectoryAuthorityMatches(opened *pinnedDarwinObject, authority *os.File) bool {
	if opened == nil || opened.file == nil || authority == nil {
		return false
	}
	var current unix.Stat_t
	return unix.Fstat(int(authority.Fd()), &current) == nil && current.Mode&unix.S_IFMT == unix.S_IFDIR &&
		current.Uid == uint32(os.Geteuid()) && current.Mode&0o077 == 0 && current.Dev == opened.stat.Dev &&
		current.Ino == opened.stat.Ino && current.Mode == opened.stat.Mode && current.Uid == opened.stat.Uid
}

func darwinDirectoryPathMatchesAuthority(path string, authority *os.File) bool {
	opened, err := openPinnedDarwinObject(path, true)
	if err != nil {
		return false
	}
	defer opened.file.Close()
	return darwinDirectoryAuthorityMatches(opened, authority)
}

func componentDarwinFileIdentity(stat unix.Stat_t) [8]uint64 {
	return [8]uint64{
		uint64(stat.Dev), stat.Ino, uint64(stat.Size), uint64(stat.Mode),
		uint64(stat.Mtim.Sec), uint64(stat.Mtim.Nsec), uint64(stat.Ctim.Sec), uint64(stat.Ctim.Nsec),
	}
}
`)
	for _, file := range goFiles(t, authorityDirectory) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && function.Name.Name == "DispatchBootstrap" {
				t.Fatalf("%s reintroduced an exported bootstrap dispatcher", rel(t, root, file))
			}
		}
	}
	directSessionPath := filepath.Join(authorityDirectory, "session_open_prod_darwin.go")
	body, err := os.ReadFile(directSessionPath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"takeDarwinReadOnlyInput(",
		"takeDarwinReadWriteOutput(",
		"validateDarwinReadOnlyInput(",
		"validateDarwinReadWriteOutput(",
		"openedFileCodeDirectoryHashes(",
		"sha256OpenedFile(",
		"openDarwinOwnerLivenessPipe()",
		"target: darwinOwnerLivenessTargetFD",
		"target: darwinReadOnlyInputTargetFD",
		"target: darwinReadWriteOutputTargetFD",
		"spawnDirectWithExtraFiles(",
		"openDarwinProcessTransitionMonitor(pid)",
		"rootState.Status == darwinProcStatusStopped",
		"rootState.Status == darwinProcStatusZombie",
		"directSession.healthyDirectLocked(ctx)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Darwin production direct session lost closed host-owned invariant %q", required)
		}
	}
	for _, forbidden := range []string{
		"openSessionWithTargetArm(",
		"openCurrentDarwinBootstrap(",
		"syscall.Exec(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("Darwin production direct session reintroduced bootstrap behavior %q", forbidden)
		}
	}
	sessionPath := filepath.Join(authorityDirectory, "session_darwin.go")
	sessionBody, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	sessionSource := string(sessionBody)
	for _, required := range []string{
		"func (session *darwinSession) healthyDirectLocked(ctx context.Context) bool",
		"session.transitions.Unchanged()",
		"listDarwinDirectChildren(ctx, session.identity)",
		"len(children) != 0",
		"loadedProcessCDHash(session.pid)",
		"codeDirectoryHashMatches(loadedCDHash, session.targetCDHashes)",
		"loadedDarwinExecutableMatchesOpenedFile(",
		"state.Status == darwinProcStatusStopped",
		"state.Status == darwinProcStatusZombie",
	} {
		if !strings.Contains(sessionSource, required) {
			t.Fatalf("Darwin shared direct-session guard lost closed process invariant %q", required)
		}
	}
	ownerLivenessPath := filepath.Join(authorityDirectory, "owner_liveness_darwin.go")
	ownerLivenessBody, err := os.ReadFile(ownerLivenessPath)
	if err != nil {
		t.Fatal(err)
	}
	ownerLivenessSource := string(ownerLivenessBody)
	for _, required := range []string{
		"const darwinOwnerLivenessTargetFD = 4",
		"os.Pipe()",
		"unix.FD_CLOEXEC != 0",
		"unix.S_IFIFO",
		"unix.O_RDONLY",
		"unix.O_WRONLY",
	} {
		if !strings.Contains(ownerLivenessSource, required) {
			t.Fatalf("Darwin production owner liveness lost descriptor invariant %q", required)
		}
	}
}

func TestProcessAuthorityGenericExecuteSurfaceDoesNotExist(t *testing.T) {
	root := runtimeGoRoot(t)
	directory := filepath.Join(root, "internal", "adapters", "outbound", "processauthority")
	for _, file := range goFiles(t, directory) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		for _, declaration := range parsed.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if typed.Recv == nil && typed.Name.Name == "Execute" {
					t.Fatalf("%s reintroduced generic processauthority.Execute", rel(t, root, file))
				}
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if ok && typeSpec.Name.Name == "Request" {
						t.Fatalf("%s reintroduced generic processauthority.Request", rel(t, root, file))
					}
				}
			}
		}
	}
}

func TestNativeExecutionLeaseHasSingleProductionConsumer(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/adapters/outbound/nativecomponentrunner/runner_darwin.go"
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "AcquireExecutionLease" && selector.Sel.Name != "TakeExecutionFile") {
				return true
			}
			if rel(t, root, file) != allowed {
				t.Fatalf("%s consumes native execution authority outside its sole runner", rel(t, root, file))
			}
			return true
		})
	}
}

func TestEveryProductionDataProcessUsesNativeAuthority(t *testing.T) {
	runtimeRoot := runtimeGoRoot(t)
	repoRoot := filepath.Dir(filepath.Dir(runtimeRoot))
	for _, file := range goFiles(t, runtimeRoot) {
		relative := rel(t, runtimeRoot, file)
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") ||
			(!strings.Contains(relative, "/nativecomponent") &&
				!strings.HasPrefix(relative, "internal/app/nativecomponent/")) {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		if len(importedAliases(parsed, "os/exec")) != 0 {
			t.Fatalf("%s reintroduced raw os/exec into a production native-component path", relative)
		}
		for _, forbidden := range []string{"StartProcess", "ForkExec"} {
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == forbidden {
					t.Fatalf("%s reintroduced raw process primitive %s", relative, forbidden)
				}
				return true
			})
		}
	}

	managerPath := filepath.Join(repoRoot, "src", "main", "data-analysis", "backend-manager.ts")
	managerBody, err := os.ReadFile(managerPath)
	if err != nil {
		t.Fatal(err)
	}
	managerSource := string(managerBody)
	if !strings.Contains(managerSource, "data_analysis_native_authority_unavailable") {
		t.Fatal("Electron data-analysis manager lost its fixed native-authority boundary")
	}
	for _, forbidden := range []string{
		"node:child_process", "ANALYTIX_DATA_ANALYSIS_PYTHON", "ANALYTIX_DATA_ANALYSIS_BACKEND_DIR",
		"ANALYTIX_DATA_ANALYSIS_PORT", "spawn(", "execFile(", "mkdir(", "fetch(",
	} {
		if strings.Contains(managerSource, forbidden) {
			t.Fatalf("Electron data-analysis manager reintroduced direct process or fallback authority %q", forbidden)
		}
	}

	pluginRoot := filepath.Join(repoRoot, "plugins", "analytix-fund-analysis")
	entrypoint := filepath.Join(pluginRoot, "mcp", "server.mjs")
	graph := productionESMImportGraph(t, entrypoint)
	for file, body := range graph {
		for _, forbidden := range []string{"node:child_process", "runPythonDuckdb", "executeLocalDuckdbWorkbenchSkill"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("funds production import graph %s contains direct data process authority %q", rel(t, repoRoot, file), forbidden)
			}
		}
		if strings.HasSuffix(file, "duckdb-workbench-runtime.mjs") || strings.HasSuffix(file, "tool-call-runtime.mjs") {
			t.Fatalf("funds production entrypoint imports quarantined runtime %s", rel(t, repoRoot, file))
		}
	}
	configBody, err := os.ReadFile(filepath.Join(pluginRoot, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configBody), "ANALYTIX_FUNDS_DUCKDB_PYTHON") {
		t.Fatal("funds MCP configuration reintroduced an interpreter selector")
	}
	var fundsConfig struct {
		MCPServers map[string]struct {
			Disabled bool `json:"disabled"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(configBody, &fundsConfig); err != nil || !fundsConfig.MCPServers["analytix_funds"].Disabled {
		t.Fatalf("funds MCP must remain disabled before PATH/process resolution: err=%v config=%#v", err, fundsConfig)
	}
	ownerProbePath := filepath.Join(repoRoot, "scripts", "verify-data-engine-single-owner.cjs")
	ownerProbeBody, err := os.ReadFile(ownerProbePath)
	if err != nil {
		t.Fatal(err)
	}
	ownerProbeSource := string(ownerProbeBody)
	if !strings.Contains(ownerProbeSource, "managed_process_execution_authority_unavailable") {
		t.Fatal("Windows data-engine release probe lost its fixed unavailable boundary")
	}
	for _, forbidden := range []string{
		"node:child_process", "child_process", "spawn(", "spawnSync(", "execFile(", "execFileSync(",
		"fork(", "process.env", "child.kill(", "stdout +=", "stderr +=",
	} {
		if strings.Contains(ownerProbeSource, forbidden) {
			t.Fatalf("Windows data-engine release probe bypasses the Go execution authority with %q", forbidden)
		}
	}
	goManagerPath := filepath.Join(runtimeRoot, "internal", "mcp", "manager.go")
	goManagerBody, err := os.ReadFile(goManagerPath)
	if err != nil {
		t.Fatal(err)
	}
	goManagerSource := string(goManagerBody)
	for _, required := range []string{
		"reservedCaseFactMCPServerID(spec.ID)",
		"caseFactHostQuarantineTransport",
		"case data source native authority is unavailable",
	} {
		if !strings.Contains(goManagerSource, required) {
			t.Fatalf("Go MCP loader lost pre-process funds quarantine %q", required)
		}
	}
}

func TestDarwinCodesignTargetsSuspendedNumericPID(t *testing.T) {
	root := runtimeGoRoot(t)
	verifierPath := filepath.Join(
		root,
		"internal", "adapters", "outbound", "nativecomponentregistry", "platform_verifier_darwin.go",
	)
	verifierBody, err := os.ReadFile(verifierPath)
	if err != nil {
		t.Fatal(err)
	}
	verifierSource := string(verifierBody)
	for _, required := range []string{
		"processauthority.InspectSuspendedDarwinExecutable",
		"processauthority.ExecuteSystemTool",
		"processauthority.SystemToolDarwinCodeSign",
		"pidTarget := strconv.Itoa(identity.PID)",
		`dynamicPIDTarget := "+" + pidTarget`,
		`"--verify", "--strict", "--verbose=2"`,
		`"--display", "--verbose=4", dynamicPIDTarget`,
		`"--entitlements", ":-", dynamicPIDTarget`,
		"identity.CDHash",
	} {
		if !strings.Contains(verifierSource, required) {
			t.Fatalf("Darwin platform verification lost loaded-process binding %q", required)
		}
	}
	for _, forbidden := range []string{
		"stagedPath",
		"darwinPathMatchesPinned",
		`"/usr/bin/codesign"`,
		`"os/exec"`,
		"exec.Command",
	} {
		if strings.Contains(verifierSource, forbidden) {
			t.Fatalf("Darwin codesign verification restored replaceable path target %q", forbidden)
		}
	}

	authorityPath := filepath.Join(
		root,
		"internal", "adapters", "outbound", "processauthority", "executor_darwin.go",
	)
	authorityBody, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	authoritySource := string(authorityBody)
	for _, required := range []string{
		`return "/usr/bin/codesign", true`,
		"trustedDarwinSystemExecutable",
		"filesystem.Flags&unix.MNT_RDONLY",
		"darwinPathMatchesPinnedObject",
		"loadedDarwinExecutableMatchesOpenedFile",
		"validDarwinCodeSignArguments",
	} {
		if !strings.Contains(authoritySource, required) {
			t.Fatalf("Darwin system-tool authority lost fixed codesign invariant %q", required)
		}
	}

	inspectionPath := filepath.Join(
		root,
		"internal", "adapters", "outbound", "processauthority", "inspection_darwin.go",
	)
	inspectionBody, err := os.ReadFile(inspectionPath)
	if err != nil {
		t.Fatal(err)
	}
	inspectionSource := string(inspectionBody)
	if !strings.Contains(inspectionSource, "spawnSuspended(") ||
		strings.Contains(inspectionSource, "SIGCONT") ||
		strings.Count(inspectionSource, "suspendedDarwinImageIdentity(") < 3 ||
		!strings.Contains(inspectionSource, "killAndWaitDarwinProcess(process, pid)") {
		t.Fatal("Darwin signature inspection must bind before and after inspection and terminate without resume")
	}
}

var esmRelativeImport = regexp.MustCompile(`(?:from\s+)?["'](\.[^"']+)["']`)

func productionESMImportGraph(t *testing.T, entrypoint string) map[string]string {
	t.Helper()
	pending := []string{entrypoint}
	visited := map[string]string{}
	for len(pending) > 0 {
		file := filepath.Clean(pending[len(pending)-1])
		pending = pending[:len(pending)-1]
		if _, seen := visited[file]; seen {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read production ESM module %s: %v", file, err)
		}
		visited[file] = string(body)
		for _, specifier := range relativeESMImports(string(body)) {
			candidate := filepath.Join(filepath.Dir(file), filepath.FromSlash(specifier))
			if filepath.Ext(candidate) == "" {
				candidate += ".mjs"
			}
			pending = append(pending, candidate)
		}
	}
	return visited
}

func relativeESMImports(body string) []string {
	imports := []string{}
	statement := ""
	collecting := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !collecting && (strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "export {") ||
			strings.HasPrefix(trimmed, "export *")) {
			collecting = true
			statement = ""
		}
		if !collecting {
			continue
		}
		statement += " " + trimmed
		if !strings.Contains(trimmed, ";") {
			continue
		}
		if match := esmRelativeImport.FindStringSubmatch(statement); len(match) == 2 {
			imports = append(imports, match[1])
		}
		statement = ""
		collecting = false
	}
	return imports
}

func TestToolSettlementNeverReturnsRawPreparedOutput(t *testing.T) {
	root := runtimeGoRoot(t)
	path := filepath.Join(root, "internal", "server", "tool_settlement.go")
	parsed := parseGoFile(t, path, 0)
	if violation := toolSettlementClosedProjectionViolationV1(parsed); violation != "" {
		t.Fatal(violation)
	}

	for _, test := range []struct {
		name          string
		body          string
		wantViolation bool
	}{
		{
			name: "path count independent",
			body: `package server
func settleRuntimeToolResultWithEffectAuthority() {
	prepared, _ := providerAttempt.PrepareSettlementV1(settlement.Output, settlement.IsError)
	publicOutput := prepared.PublicOutput
	if a { return apploop.SettledToolExecution{Output: publicOutput} }
	if b { return apploop.SettledToolExecution{Output: publicOutput} }
	if c { return apploop.SettledToolExecution{Output: publicOutput} }
	if d { return apploop.SettledToolExecution{Output: publicOutput} }
	return apploop.SettledToolExecution{Output: publicOutput}
}`,
		},
		{
			name: "raw settlement output",
			body: `package server
func settleRuntimeToolResultWithEffectAuthority() {
	prepared := struct{ PublicOutput any }{}
	publicOutput := prepared.PublicOutput
	return apploop.SettledToolExecution{Output: settlement.Output}
}`,
			wantViolation: true,
		},
		{
			name: "alternate output",
			body: `package server
func settleRuntimeToolResultWithEffectAuthority() {
	prepared := struct{ PublicOutput any }{}
	publicOutput := prepared.PublicOutput
	return apploop.SettledToolExecution{Output: otherOutput}
}`,
			wantViolation: true,
		},
		{
			name: "assigned raw settlement output",
			body: `package server
func settleRuntimeToolResultWithEffectAuthority() {
	prepared := struct{ PublicOutput any }{}
	publicOutput := prepared.PublicOutput
	result := apploop.SettledToolExecution{Output: settlement.Output}
	return result
}`,
			wantViolation: true,
		},
		{
			name: "assigned alternate output",
			body: `package server
func settleRuntimeToolResultWithEffectAuthority() {
	prepared := struct{ PublicOutput any }{}
	publicOutput := prepared.PublicOutput
	result := apploop.SettledToolExecution{Output: otherOutput}
	return result
}`,
			wantViolation: true,
		},
		{
			name: "helper return raw settlement output",
			body: `package server
func settleRuntimeToolResultWithEffectAuthority() {
	prepared := struct{ PublicOutput any }{}
	publicOutput := prepared.PublicOutput
	return closeResult(apploop.SettledToolExecution{Output: settlement.Output})
}`,
			wantViolation: true,
		},
		{
			name: "rebound public output",
			body: `package server
func settleRuntimeToolResultWithEffectAuthority() {
	prepared := struct{ PublicOutput any }{}
	publicOutput := prepared.PublicOutput
	publicOutput = settlement.Output
	return apploop.SettledToolExecution{Output: publicOutput}
}`,
			wantViolation: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parser.ParseFile(token.NewFileSet(), "tool_settlement_fixture.go", test.body, 0)
			if err != nil {
				t.Fatal(err)
			}
			violation := toolSettlementClosedProjectionViolationV1(parsed)
			if (violation != "") != test.wantViolation {
				t.Fatalf("closed projection violation = %q, wantViolation=%t", violation, test.wantViolation)
			}
		})
	}
}

func toolSettlementClosedProjectionViolationV1(parsed *ast.File) string {
	var target *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "settleRuntimeToolResultWithEffectAuthority" {
			target = function
			break
		}
	}
	if target == nil || target.Body == nil {
		return "tool settlement effect-authority owner is missing"
	}

	publicOutputDefinitions := 0
	outputBearingLiterals := 0
	violation := ""
	ast.Inspect(target.Body, func(node ast.Node) bool {
		if violation != "" {
			return false
		}
		switch current := node.(type) {
		case *ast.AssignStmt:
			for index, left := range current.Lhs {
				identifier, ok := left.(*ast.Ident)
				if !ok || identifier.Name != "publicOutput" {
					continue
				}
				if current.Tok != token.DEFINE || len(current.Lhs) != 1 || len(current.Rhs) != 1 || index != 0 {
					violation = "closed public tool projection must be defined exactly once"
					return false
				}
				selector, ok := current.Rhs[0].(*ast.SelectorExpr)
				if !ok {
					violation = "closed public tool projection must come from prepared.PublicOutput"
					return false
				}
				owner, ownerOK := selector.X.(*ast.Ident)
				if !ownerOK || owner.Name != "prepared" || selector.Sel.Name != "PublicOutput" {
					violation = "closed public tool projection must come from prepared.PublicOutput"
					return false
				}
				publicOutputDefinitions++
			}
		case *ast.CompositeLit:
			selector, ok := current.Type.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "SettledToolExecution" {
				return true
			}
			for _, element := range current.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, keyOK := field.Key.(*ast.Ident)
				if !keyOK || key.Name != "Output" {
					continue
				}
				outputBearingLiterals++
				value, ok := field.Value.(*ast.Ident)
				if !ok || value.Name != "publicOutput" {
					violation = "every tool settlement output literal must use the closed public host projection"
					return false
				}
			}
		}
		return true
	})
	if violation != "" {
		return violation
	}
	if publicOutputDefinitions != 1 {
		return "closed public tool projection must be defined exactly once"
	}
	if outputBearingLiterals == 0 {
		return "tool settlement has no output-bearing settled execution literal"
	}
	return ""
}

func TestProductionHasNoRawToolResultMediaFollowupAPI(t *testing.T) {
	root := runtimeGoRoot(t)
	forbidden := []string{
		"NativeToolResultImageMessages",
		"NativeToolResultImagesFunc",
		"runtimeNativeToolResultImageMessages",
		"nativeImageFollowups",
	}
	for _, path := range goFiles(t, root) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel(t, root, path), err)
		}
		for _, symbol := range forbidden {
			if strings.Contains(string(data), symbol) {
				t.Fatalf("%s restores forbidden raw tool-result media follow-up API %q", rel(t, root, path), symbol)
			}
		}
	}
}

func TestProductionToolResultRecordsHaveSingleClosedProjectionWriter(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/app/toolcatalog/settlement.go"
	for _, file := range goFiles(t, root) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, file, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, keyOK := pair.Key.(*ast.BasicLit)
				value, valueOK := pair.Value.(*ast.BasicLit)
				if !keyOK || !valueOK || key.Kind != token.STRING || value.Kind != token.STRING {
					continue
				}
				keyText, keyErr := strconv.Unquote(key.Value)
				valueText, valueErr := strconv.Unquote(value.Value)
				if keyErr == nil && valueErr == nil && keyText == "kind" && valueText == "tool_result" && rel(t, root, file) != allowed {
					t.Fatalf("%s constructs a tool_result outside the closed projection writer", rel(t, root, file))
				}
			}
			return true
		})
	}
}

func TestRuntimeServerGravityWellSplitAnchors(t *testing.T) {
	root := runtimeGoRoot(t)
	serverDir := filepath.Join(root, "internal", "server")
	for _, name := range []string{
		"runtime_components.go",
		"runtime_handler.go",
		"routes.go",
		"runtime_info_routes.go",
		"capabilities.go",
		"tool_catalog.go",
		"skills.go",
		"subagents_config.go",
		"runtime_settings.go",
		"checkpoint.go",
		"checkpoint_routes.go",
		"gates_routes.go",
		"durable_core.go",
		"durable_event_log.go",
		"durable_goals_todos.go",
		"durable_threads.go",
		"durable_turns.go",
		"durable_recovery.go",
		"durable_steering.go",
		"thread_routes.go",
		"turn_start.go",
		"turn_cancel.go",
		"turn_routes.go",
		"turn_terminal.go",
		"runtime_restore.go",
		"agent_loop.go",
		"tools_execution.go",
		"goal_tools.go",
		"tool_settlement.go",
		"plan_tools.go",
		"subagent_jobs.go",
		"native_read_tools.go",
		"web_fetch_tool.go",
		"terminal.go",
		"file_mutation_tools.go",
		"turn_finalize.go",
		"vision_bridge.go",
		"runtime_helpers.go",
	} {
		if _, err := os.Stat(filepath.Join(serverDir, name)); err != nil {
			t.Fatalf("expected migration split anchor %s: %v", name, err)
		}
	}
}

func TestRuntimeServerGravityWellBudgetDoesNotRegrow(t *testing.T) {
	root := runtimeGoRoot(t)
	serverDir := filepath.Join(root, "internal", "server")
	files := []string{}
	lineCount := 0
	for _, file := range goFiles(t, serverDir) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		files = append(files, file)
		lineCount += countEffectiveLines(t, file)
	}
	if len(files) > 54 {
		t.Fatalf("internal/server production file budget regressed: %d files", len(files))
	}
	if lineCount > 8649 {
		t.Fatalf("internal/server production effective line budget regressed: %d lines", lineCount)
	}
}

func TestCheckpointCaptureExactlyOnceLivesOutsideTransitionalServer(t *testing.T) {
	root := runtimeGoRoot(t)
	legacy := filepath.Join(root, "internal", "server", "durable_checkpoint_capture.go")
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("checkpoint-specific exactly-once logic must not live in internal/server: %v", err)
	}
	for _, relative := range []string{
		"internal/app/checkpoint/captured_event_reconciler.go",
		"internal/ports/checkpointcapture/store.go",
		"internal/adapters/outbound/checkpointcapture/store.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("checkpoint capture layer owner %s is missing: %v", relative, err)
		}
	}
	for _, file := range goFiles(t, filepath.Join(root, "internal", "server")) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, forbidden := range []string{"ExactCapturedEventCount", "checkpoint captured atomic append did not reconcile"} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("%s retains checkpoint-specific exactly-once behavior %q", rel(t, root, file), forbidden)
			}
		}
	}
}

func TestRuntimeControlOwnsTurnActionEntryPoints(t *testing.T) {
	root := runtimeGoRoot(t)
	required := map[string]string{
		"internal/server/turn_start.go":   "httpapi.StartTurnHandlers{Control: h.runtimeControl()}",
		"internal/server/turn_routes.go":  "httpapi.TurnActionHandlers{Control: h.runtimeControl()}",
		"internal/server/gates_routes.go": "httpapi.GateHandlers{Control: h.runtimeControl()}",
		"internal/server/turn_cancel.go":  "CancelAllRegisteredTurns",
	}
	for name, needle := range required {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), needle) {
			t.Fatalf("%s must route runtime control through app/control (%q missing)", name, needle)
		}
	}
	handler, err := os.ReadFile(filepath.Join(root, "internal", "server", "runtime_handler.go"))
	if err != nil {
		t.Fatalf("read runtime_handler.go: %v", err)
	}
	if strings.Contains(string(handler), "activeTurnCancels") {
		t.Fatalf("runtime_handler.go must not own active turn cancel state; app/control owns the registry")
	}
}

func TestRuntimeAppOwnsCompositionRootAssembly(t *testing.T) {
	root := runtimeGoRoot(t)
	appFile := filepath.Join(root, "internal", "runtimeapp", "app.go")
	data, err := os.ReadFile(appFile)
	if err != nil {
		t.Fatalf("read runtimeapp composition root: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		"filestore.NewPersistentAttachmentStore",
		"jobs.NewManager",
		"mcp.NewProductionManagerWithOptions",
		"providerclient.NewProductionHTTPProviderClient",
		"server.NewRuntimeServerHandlerFromComponents",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("runtimeapp composition root missing assembly anchor %q", required)
		}
	}
	if strings.Contains(source, "server.NewRuntimeServerHandler(") ||
		strings.Contains(source, "server.newRuntimeServerContractTestHandler(t, ") {
		t.Fatalf("runtimeapp must assemble components directly, not delegate to server factory facade")
	}
	if strings.Contains(source, "providerclient.NewHTTPProviderClient(") {
		t.Fatal("runtimeapp must not construct a production provider client without durable telemetry")
	}
	for _, forbidden := range []string{
		"server.LoadMCPServerSpecs",
		"server.LoadRuntimeMCPSearchSettings",
		"server.LoadRuntimeSkillCatalog",
		"server.LoadRuntimeSubagentConfig",
		"server.LoadRuntimeSandboxSettings",
		"server.LoadRuntimeWebConfig",
		"server.LoadRuntimeVisionBridgeConfig",
		"server.LoadRuntimeStepLimitConfig",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("runtimeapp must own runtime dependency config assembly, found %q", forbidden)
		}
	}
}

func TestProductionRuntimeCLIHoldsPersistenceLeaseForHandlerLifecycle(t *testing.T) {
	root := runtimeGoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "cmd", "runtime-server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{
		"AcquireRuntimePersistenceLease(config)",
		"PrepareRuntimeServerStartupWithPersistenceLeaseContextE(ctx, config, lease)",
		"prepared.ActivateContext(runtimeCtx, config)",
		"notifyContext: signal.NotifyContext",
		"lease.Close()",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("production runtime CLI lease lifecycle is missing %q", required)
		}
	}
	if strings.Contains(source, "runtimeapp.NewRuntimeServerHandlerE(config)") ||
		strings.Contains(source, "runtimeapp.NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease)") {
		t.Fatal("production runtime CLI bypasses its frozen persistence lease")
	}
}

func TestProductionRootFactoryOwnsPersistenceLease(t *testing.T) {
	root := runtimeGoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "runtime_server_prod.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "NewRuntimeServerHandlerWithOwnedPersistenceLeaseE(config)") ||
		strings.Contains(source, "runtimeapp.NewRuntimeServerHandler(config)") {
		t.Fatal("analytix_prod root factory can bypass composite persistence ownership")
	}
}

func TestProductionAcceptedFinalEventsUseAtomicBundlePersistence(t *testing.T) {
	root := runtimeGoRoot(t)
	appData, err := os.ReadFile(filepath.Join(root, "internal", "runtimeapp", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	appSource := string(appData)
	for _, required := range []string{
		"finalEventDelivery := store.AcceptedFinalEventDelivery()",
		"AppendEvents:", "finalEventDelivery.Stage(ctx, events)",
		"Readback: finalEventDelivery",
		"WithEventReservation:",
		"finalEventDelivery.WithReservation(ctx, threadID, commitID, work)",
		"ActivateAndPublishEvents:",
		"finalEventDelivery.ActivateAndPublish(ctx, events, seal, activate)",
		"NewTrustedFinalProjectionIndexWithReadback(finalAuthority, finalEventDelivery)",
	} {
		if !strings.Contains(appSource, required) {
			t.Fatalf("production accepted-final composition is missing %q", required)
		}
	}
	if strings.Contains(appSource, "\n\t\tPublishEvents:") || strings.Contains(appSource, "PrepareEvents:") ||
		strings.Contains(appSource, "PublishAcceptedFinalEventBundle(events, seal)") {
		t.Fatal("production accepted-final composition retains the unreserved live publish path")
	}
	reconcileData, err := os.ReadFile(filepath.Join(root, "internal", "app", "evidence", "final_publication_reconcile.go"))
	if err != nil {
		t.Fatal(err)
	}
	reconcileSource := string(reconcileData)
	for _, required := range []string{
		"io.AppendEvents(ctx, store, drafts)",
		"FinalizeAcceptedFinalEventDeliveryV1",
		"projectionLease.SealAcceptedFinalDelivery(reservedContext, verifiedEvents)",
		"reservedContext, store, verifiedEvents, deliverySeal, projectionLease.Activate",
	} {
		if !strings.Contains(reconcileSource, required) {
			t.Fatalf("accepted-final reconciliation is missing reservation-bound step %q", required)
		}
	}
	if strings.Contains(reconcileSource, "io.AppendEvent(") {
		t.Fatal("accepted-final reconciliation can bypass atomic bundle persistence")
	}
}

func TestProductionPublicProjectionUsesCurrentCaseAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "runtimeapp", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{
		"NewCurrentCaseThreadAuthorityValidator(caseThreads, filestore.CaseBindingReader{}",
		"NewTrustedPublicProjectorWithPrimaryCAS(",
		"trustedFinals, caseThreads, currentCaseAuthority, acceptedFinalCASReader",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("production public projection is missing live case authority wiring %q", required)
		}
	}
	if strings.Contains(source, "NewTrustedPublicProjectorWithCurrentCaseAuthority(trustedFinals") {
		t.Fatal("production public projection can omit strict primary CAS authority")
	}
}

func TestAcceptedFinalTurnCASDigestHasSingleProductionReaderOwner(t *testing.T) {
	root := runtimeGoRoot(t)
	allowed := "internal/adapters/outbound/finalauthority/cas_reader.go"
	for _, file := range goFiles(t, filepath.Join(root, "internal")) {
		if strings.HasSuffix(file, "_test.go") || hasBuildTag(t, file, "!analytix_prod") {
			continue
		}
		relative := rel(t, root, file)
		if relative == "internal/domain/evidence/accepted_final_cas.go" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "AcceptedFinalTurnProjectionSHA256V1(") && relative != allowed {
			t.Fatalf("%s recomputes accepted-final CAS outside the strict primary reader", relative)
		}
	}
}

func TestRuntimeGoCrossPackageExtractionAnchors(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, name := range []string{
		"internal/domain/model/endpoint.go",
		"internal/domain/model/attachment.go",
		"internal/domain/model/hash.go",
		"internal/domain/model/schema.go",
		"internal/domain/goal/evidence.go",
		"internal/upstreamaudit/runtime_absorption_capabilities.go",
		"internal/protocol/route_replay.go",
		"internal/conformance/g5_shadow.go",
		"internal/conformance/livelocal/common.go",
		"internal/conformance/livelocal/g1_handler.go",
		"internal/conformance/livelocal/g2_handler.go",
		"internal/conformance/livelocal/harness.go",
		"internal/conformance/livelocal/store.go",
		"internal/conformance/livelocal/durable_handler.go",
		"internal/conformance/livelocal/loop_handler.go",
		"internal/conformance/livelocal/kernel_handler.go",
		"internal/conformance/livelocal/production_candidate.go",
		"internal/conformance/livelocal/production_candidate_handler.go",
		"internal/app/filetools/text.go",
		"internal/app/filetools/notebook.go",
		"internal/app/filetools/edit.go",
		"internal/app/filetools/read_guard.go",
		"internal/app/filetools/read_registry.go",
		"internal/app/filetools/delete_symbol.go",
		"internal/app/filetools/code_index.go",
		"internal/app/filetools/search.go",
		"internal/app/checkpoint/checkpoint.go",
		"internal/app/checkpoint/apply.go",
		"internal/app/checkpoint/snapshot.go",
		"internal/app/goal/closure.go",
		"internal/app/goal/state.go",
		"internal/app/goal/tool_service.go",
		"internal/app/control/service.go",
		"internal/app/control/approval_user_input.go",
		"internal/app/control/gates.go",
		"internal/app/control/gate_events.go",
		"internal/app/plan/clarifying.go",
		"internal/app/plan/create_plan.go",
		"internal/app/model/provider_config.go",
		"internal/app/model/prefix_shape.go",
		"internal/app/model/pending_tool.go",
		"internal/app/model/pending_tool_rebuild.go",
		"internal/app/model/service.go",
		"internal/app/model/steering_service.go",
		"internal/app/model/tool_pairing.go",
		"internal/app/model/provider_history.go",
		"internal/app/runtimeinfo/config.go",
		"internal/app/runtimeinfo/contract_capabilities.go",
		"internal/app/loop/event_recorder.go",
		"internal/app/loop/events.go",
		"internal/app/loop/provider_stream.go",
		"internal/app/loop/prompts.go",
		"internal/app/loop/recovery.go",
		"internal/app/loop/result.go",
		"internal/app/loop/step_limits.go",
		"internal/app/loop/tool_guards.go",
		"internal/app/mcp/catalog_event.go",
		"internal/app/mcp/tool_runner.go",
		"internal/app/turn/attachments.go",
		"internal/app/turn/failure.go",
		"internal/app/turn/gate_continuation.go",
		"internal/app/turn/gate_cancellation.go",
		"internal/app/turn/gate_requests.go",
		"internal/app/turn/tool_call_ready.go",
		"internal/app/turn/start.go",
		"internal/app/turn/start_plan.go",
		"internal/app/turn/compaction.go",
		"internal/app/turn/completion.go",
		"internal/app/turn/items.go",
		"internal/app/turn/settlement.go",
		"internal/app/turn/terminal_update.go",
		"internal/app/session/workspace_status.go",
		"internal/app/runtimeinfo/capabilities.go",
		"internal/app/runtimeinfo/vision_bridge.go",
		"internal/app/subagent/config.go",
		"internal/app/subagent/capabilities.go",
		"internal/app/subagent/child_thread.go",
		"internal/app/subagent/job_view.go",
		"internal/app/subagent/output.go",
		"internal/app/subagent/parallel_runner.go",
		"internal/app/subagent/progress_events.go",
		"internal/app/subagent/profile.go",
		"internal/app/subagent/requests.go",
		"internal/app/subagent/runtime_state.go",
		"internal/app/subagent/skill_request.go",
		"internal/app/subagent/task_job_service.go",
		"internal/app/subagent/task_job_tools.go",
		"internal/app/subagent/tool_scope.go",
		"internal/app/terminal/diagnostics.go",
		"internal/app/terminal/buffer.go",
		"internal/app/terminal/bash_tool_request.go",
		"internal/app/terminal/bash.go",
		"internal/app/terminal/background_bash.go",
		"internal/app/terminal/tool_runner.go",
		"internal/app/thread/service.go",
		"internal/app/thread/fork_projection.go",
		"internal/app/thread/lifecycle.go",
		"internal/app/thread/recovered_state.go",
		"internal/app/thread/review.go",
		"internal/app/thread/review_service.go",
		"internal/app/thread/rewind.go",
		"internal/app/thread/sidecar.go",
		"internal/app/thread/title.go",
		"internal/app/thread/turn_append.go",
		"internal/app/thread/todos.go",
		"internal/app/threadsummary/service.go",
		"internal/app/threadsummary/projection.go",
		"internal/app/threadsummary/commands.go",
		"internal/app/threadsummary/subagents.go",
		"internal/app/toolcatalog/diagnostics.go",
		"internal/app/toolcatalog/builtin_tools.go",
		"internal/app/toolcatalog/batch.go",
		"internal/app/toolcatalog/materialize.go",
		"internal/app/toolcatalog/progress_events.go",
		"internal/app/toolcatalog/skills.go",
		"internal/app/toolcatalog/skills_loader.go",
		"internal/app/toolcatalog/goal_tools.go",
		"internal/app/toolcatalog/plan_tools.go",
		"internal/app/toolcatalog/settlement.go",
		"internal/app/toolcatalog/subagent_tools.go",
		"internal/app/toolcatalog/routing.go",
		"internal/app/toolcatalog/service.go",
		"internal/app/toolcatalog/user_input.go",
		"internal/app/loop/events.go",
		"internal/app/control/start_turn.go",
		"internal/app/usage/service.go",
		"internal/app/usage/runtime.go",
		"internal/app/usage/cache_diagnostics.go",
		"internal/app/usage/records.go",
		"internal/app/usage/query.go",
		"internal/app/usage/index.go",
		"internal/ports/ports.go",
		"internal/app/thread/errors.go",
		"internal/app/thread/list_projection.go",
		"internal/adapters/inbound/httpapi/handler.go",
		"internal/adapters/inbound/httpapi/routes.go",
		"internal/adapters/inbound/httpapi/response.go",
		"internal/adapters/inbound/httpapi/thread.go",
		"internal/adapters/inbound/httpapi/checkpoint.go",
		"internal/adapters/inbound/httpapi/runtime_diagnostics.go",
		"internal/adapters/inbound/httpapi/skills.go",
		"internal/adapters/inbound/httpapi/thread_summary.go",
		"internal/adapters/inbound/httpapi/review.go",
		"internal/adapters/inbound/httpapi/control_result.go",
		"internal/adapters/inbound/httpapi/gates.go",
		"internal/adapters/inbound/httpapi/turn_actions.go",
		"internal/adapters/inbound/httpapi/start_turn.go",
		"internal/adapters/inbound/httpapi/replay.go",
		"internal/adapters/inbound/httpapi/attachments.go",
		"internal/adapters/inbound/httpapi/memory.go",
		"internal/adapters/inbound/httpapi/usage.go",
		"internal/adapters/inbound/httpapi/task_jobs.go",
		"internal/adapters/inbound/httpapi/session.go",
		"internal/adapters/inbound/httpapi/workspace_status.go",
		"internal/adapters/inbound/sse/events.go",
		"internal/adapters/outbound/filestore/files.go",
		"internal/adapters/outbound/filestore/json_map.go",
		"internal/adapters/outbound/filestore/jsonl.go",
		"internal/adapters/outbound/filestore/path_policy.go",
		"internal/adapters/outbound/filestore/attachments.go",
		"internal/adapters/outbound/filestore/memory.go",
		"internal/adapters/outbound/filestore/case_binding.go",
		"internal/adapters/outbound/filestore/config_document.go",
		"internal/adapters/outbound/filestore/checkpoint.go",
		"internal/adapters/outbound/filestore/skills.go",
		"internal/adapters/outbound/filestore/text.go",
		"internal/adapters/outbound/filestore/code_index.go",
		"internal/adapters/outbound/filestore/file_mutation_tool_runner.go",
		"internal/adapters/outbound/filestore/file_tool_runner.go",
		"internal/adapters/outbound/filestore/gitignore.go",
		"internal/adapters/outbound/filestore/grep.go",
		"internal/adapters/outbound/filestore/plan_tool_runner.go",
		"internal/adapters/outbound/filestore/search.go",
		"internal/adapters/outbound/eventlog/store.go",
		"internal/adapters/outbound/process/command_probe.go",
		"internal/adapters/outbound/process/git.go",
		"internal/adapters/outbound/process/shell_runner.go",
		"internal/adapters/outbound/process/workspace_status.go",
		"internal/adapters/outbound/provider/client/client.go",
		"internal/adapters/outbound/mcp/cache/cache.go",
		"internal/adapters/outbound/mcp/http/client.go",
		"internal/adapters/outbound/mcp/protocol/protocol.go",
		"internal/adapters/outbound/mcp/redaction/redaction.go",
		"internal/adapters/outbound/mcp/stdio/process.go",
		"internal/adapters/outbound/provider/anthropic/request.go",
		"internal/adapters/outbound/provider/compat/request.go",
		"internal/adapters/outbound/provider/openai/request.go",
		"internal/adapters/outbound/provider/responses/request.go",
		"internal/adapters/outbound/provider/schema/request.go",
		"internal/adapters/outbound/provider/stream/stream.go",
		"internal/adapters/outbound/provider/usage/usage.go",
		"internal/adapters/outbound/webfetch/client.go",
		"internal/adapters/outbound/webfetch/tool_runner.go",
		"internal/runtimeapp/app.go",
		"internal/runtimeapp/config.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatalf("expected cross-package extraction anchor %s: %v", name, err)
		}
	}
}

func TestSemanticStartupDoesNotBufferManagedFilesOrUseSharedTempPlanning(t *testing.T) {
	root := runtimeGoRoot(t)
	startupPath := filepath.Join(root, "internal", "adapters", "outbound", "persistencefs", "semantic_startup.go")
	startupBody, err := os.ReadFile(startupPath)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(startupBody), "func copyVerifiedFile(")
	end := strings.Index(string(startupBody), "func semanticOperations(")
	if start < 0 || end <= start {
		t.Fatal("semantic startup bounded-copy implementation is missing")
	}
	copyBody := string(startupBody[start:end])
	for _, forbidden := range []string{"os.ReadFile(", "io.ReadAll("} {
		if strings.Contains(copyBody, forbidden) {
			t.Fatalf("semantic startup managed copy reintroduced unbounded buffering: %s", forbidden)
		}
	}
	if !strings.Contains(copyBody, "make([]byte, 1<<20)") || !strings.Contains(copyBody, "contextError(ctx)") {
		t.Fatal("semantic startup managed copy lost its 1 MiB buffer or cancellation checkpoint")
	}
	namespacePath := filepath.Join(root, "internal", "adapters", "outbound", "persistencefs", "journal_namespace.go")
	namespaceBody, err := os.ReadFile(namespacePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(namespaceBody), "os.TempDir(") || strings.Contains(string(namespaceBody), "analytix-semantic-startup-") {
		t.Fatal("semantic startup planning reintroduced the shared operating-system temp directory")
	}
	snapshotPath := filepath.Join(root, "internal", "adapters", "outbound", "persistencefs", "snapshot.go")
	snapshotBody, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(snapshotBody), "filepath.WalkDir(") || !strings.Contains(string(snapshotBody), ".ReadDir(256)") {
		t.Fatal("semantic startup snapshot lost its paged directory traversal")
	}
	if strings.Contains(string(snapshotBody), "Files   []FileRecord") || strings.Contains(string(snapshotBody), "Files []FileRecord") {
		t.Fatal("semantic startup snapshot reintroduced the redundant full file index")
	}
	persistenceDir := filepath.Join(root, "internal", "adapters", "outbound", "persistencefs")
	for _, file := range goFiles(t, persistenceDir) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"ReadDir(-1)", "filepath.WalkDir(", "filepath.Glob(", "os.ReadFile("} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s reintroduced an unbounded persistence filesystem API: %s", rel(t, root, file), forbidden)
			}
		}
	}
}

func checkImports(t *testing.T, root string, relDir string, banned []string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relDir))
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("stat %s: %v", relDir, err)
	}
	for _, file := range goFiles(t, dir) {
		parsed := parseGoFile(t, file, parser.ImportsOnly)
		for _, imp := range parsed.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", rel(t, root, file), err)
			}
			for _, rule := range banned {
				if path == rule || strings.HasPrefix(path, rule+"/") {
					t.Fatalf("%s imports forbidden dependency %q for %s", rel(t, root, file), path, relDir)
				}
			}
		}
	}
}

func runtimeGoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			t.Fatal("could not locate runtime-go root")
		}
		cwd = parent
	}
}

func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	files := []string{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return files
	} else if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "testdata", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

func parseGoFile(t *testing.T, file string, mode parser.Mode) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, mode)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return parsed
}

func importedAliases(file *ast.File, wanted string) map[string]bool {
	aliases := map[string]bool{}
	if file == nil {
		return aliases
	}
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil || path != wanted {
			continue
		}
		name := filepath.Base(path)
		if item.Name != nil {
			name = item.Name.Name
		}
		if name != "" && name != "_" && name != "." {
			aliases[name] = true
		}
	}
	return aliases
}

func buildConstraintImpliesNativeProbeOnly(t *testing.T, file string) bool {
	t.Helper()
	actual, ok := readBuildConstraintV1(t, file)
	if !ok {
		return false
	}
	target, err := constraint.Parse("//go:build analytix_native_build_probe && !analytix_prod")
	if err != nil {
		t.Fatal(err)
	}
	return buildConstraintImplicationV1(t, actual, target)
}

func buildConstraintEquivalentV1(t *testing.T, file string, wanted string) bool {
	t.Helper()
	actual, ok := readBuildConstraintV1(t, file)
	if !ok {
		return false
	}
	target, err := constraint.Parse("//go:build " + wanted)
	if err != nil {
		t.Fatalf("parse target build constraint: %v", err)
	}
	return buildConstraintImplicationV1(t, actual, target) && buildConstraintImplicationV1(t, target, actual)
}

func readBuildConstraintV1(t *testing.T, file string) (constraint.Expr, bool) {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			break
		}
		if !strings.HasPrefix(trimmed, "//go:build ") {
			continue
		}
		expression, parseErr := constraint.Parse(trimmed)
		if parseErr != nil {
			t.Fatalf("parse %s build constraint: %v", file, parseErr)
		}
		return expression, true
	}
	return nil, false
}

func buildConstraintImplicationV1(t *testing.T, premise constraint.Expr, consequence constraint.Expr) bool {
	t.Helper()
	tags := map[string]bool{}
	collectBuildConstraintTagsV1(premise, tags)
	collectBuildConstraintTagsV1(consequence, tags)
	if len(tags) > 20 {
		t.Fatalf("build constraint truth table has too many tags: %d", len(tags))
	}
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	for mask := 0; mask < 1<<len(names); mask++ {
		assignment := func(tag string) bool {
			for index, name := range names {
				if name == tag {
					return mask&(1<<index) != 0
				}
			}
			return false
		}
		if premise.Eval(assignment) && !consequence.Eval(assignment) {
			return false
		}
	}
	return true
}

func collectBuildConstraintTagsV1(expression constraint.Expr, tags map[string]bool) {
	switch value := expression.(type) {
	case *constraint.TagExpr:
		tags[value.Tag] = true
	case *constraint.NotExpr:
		collectBuildConstraintTagsV1(value.X, tags)
	case *constraint.AndExpr:
		collectBuildConstraintTagsV1(value.X, tags)
		collectBuildConstraintTagsV1(value.Y, tags)
	case *constraint.OrExpr:
		collectBuildConstraintTagsV1(value.X, tags)
		collectBuildConstraintTagsV1(value.Y, tags)
	}
}

func hasBuildTag(t *testing.T, file string, tag string) bool {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	header := string(data)
	if idx := strings.Index(header, "\npackage "); idx >= 0 {
		header = header[:idx]
	}
	return strings.Contains(header, "//go:build") && strings.Contains(header, tag)
}

func analytixProdGoSourceSet(t *testing.T, directory string, goos string, goarch string) []string {
	t.Helper()
	context := build.Default
	context.GOOS = goos
	context.GOARCH = goarch
	context.BuildTags = []string{"analytix_prod"}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}
	files := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		matched, matchErr := context.MatchFile(directory, name)
		if matchErr != nil {
			t.Fatalf("evaluate %s for %s/%s analytix_prod: %v", name, goos, goarch, matchErr)
		}
		if matched {
			files = append(files, filepath.Join(directory, name))
		}
	}
	if len(files) == 0 {
		t.Fatalf("%s/%s analytix_prod source set is empty", goos, goarch)
	}
	return files
}

func assertExactSource(t *testing.T, file string, expected string) {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if string(data) != expected {
		t.Fatalf("%s changed outside its exact production safety allowlist", file)
	}
}

func countLines(t *testing.T, file string) int {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if len(data) == 0 {
		return 0
	}
	lines := strings.Count(string(data), "\n")
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

func countEffectiveLines(t *testing.T, file string) int {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		count++
	}
	return count
}

func rel(t *testing.T, root string, file string) string {
	t.Helper()
	relative, err := filepath.Rel(root, file)
	if err != nil {
		return file
	}
	return filepath.ToSlash(relative)
}
