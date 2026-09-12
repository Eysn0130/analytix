package architecture_test

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// These are deliberately complete method contracts, not a file exemption.
// Whitespace is irrelevant, but changing inputs, error propagation, owner
// selection or lock lifetime requires an explicit contract review.
const registryCommitForwarderContract = `
func (owner *runtimeImportActivatedRegistryV1) CommitPrepared(ctx context.Context, input registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	registry, release, err := owner.current()
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	defer release()
	return registry.CommitPrepared(ctx, input)
}`

const registryCurrentGuardContract = `
func (owner *runtimeImportActivatedRegistryV1) current() (*evidenceregistryapp.Service, func(), error) {
	owner.mu.RLock()
	if owner.closed || owner.registry == nil {
		owner.mu.RUnlock()
		return nil, nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
	}
	return owner.registry, owner.mu.RUnlock, nil
}`

func exactRegistryMethod(function *ast.FuncDecl, contract string) bool {
	parsed, err := parser.ParseFile(token.NewFileSet(), "contract.go", "package contract\n"+contract, parser.SkipObjectResolution)
	if err != nil || len(parsed.Decls) != 1 {
		return false
	}
	var actual, expected bytes.Buffer
	return format.Node(&actual, token.NewFileSet(), function) == nil &&
		format.Node(&expected, token.NewFileSet(), parsed.Decls[0]) == nil &&
		bytes.Equal(actual.Bytes(), expected.Bytes())
}

func hasExactRegistryCurrentGuard(file *ast.File) bool {
	count := 0
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && exactRegistryMethod(function, registryCurrentGuardContract) {
			count++
		}
	}
	return count == 1
}

func TestRegistryForwarderGuardRejectsAuthorityAndInputDrift(t *testing.T) {
	for _, contract := range []string{registryCommitForwarderContract, registryCurrentGuardContract} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\n"+contract, parser.SkipObjectResolution)
		if err != nil || !exactRegistryMethod(parsed.Decls[0].(*ast.FuncDecl), contract) {
			t.Fatal("exact forwarding contract was rejected")
		}
		mutations := [][2]string{
			{"ctx, input", "context.Background(), input"},
			{"ctx, input", "ctx, replacement"},
			{"defer release()", "release()"},
			{"return domainevidence.EvidenceReceipt{}, err", "return domainevidence.EvidenceReceipt{}, nil"},
			{"owner.current()", "another.current()"},
			{"owner.mu.RLock()", ""},
			{"owner.closed || owner.registry == nil", "owner.registry == nil"},
			{"owner.registry, owner.mu.RUnlock", "another.registry, owner.mu.RUnlock"},
		}
		for _, mutation := range mutations {
			if !strings.Contains(contract, mutation[0]) {
				continue
			}
			changed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\n"+strings.ReplaceAll(contract, mutation[0], mutation[1]), parser.SkipObjectResolution)
			if err != nil || exactRegistryMethod(changed.Decls[0].(*ast.FuncDecl), contract) {
				t.Fatalf("guard did not reject %q", mutation[0])
			}
		}
	}
}
