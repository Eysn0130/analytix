package architecture_test

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPublicationAuthorityStoresCannotSelectLocalCurrent(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, directory := range []string{
		filepath.Join(root, "internal", "ports", "reportpublication"),
		filepath.Join(root, "internal", "adapters", "outbound", "reportpublication"),
	} {
		for _, path := range goFiles(t, directory) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			parsed := parseGoFile(t, path, 0)
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.SelectorExpr:
					switch typed.Sel.Name {
					case "ReadDir", "Walk", "WalkDir", "Glob":
						t.Errorf("%s invokes forbidden local publication inventory %s", rel(t, root, path), typed.Sel.Name)
					}
				case *ast.TypeSpec:
					interfaceType, ok := typed.Type.(*ast.InterfaceType)
					if !ok {
						return true
					}
					for _, method := range interfaceType.Methods.List {
						for _, name := range method.Names {
							switch name.Name {
							case "List", "Current", "ResolveCurrent", "Active", "Latest":
								t.Errorf("%s exposes forbidden publication selector %s", rel(t, root, path), name.Name)
							}
						}
					}
				}
				return true
			})
		}
	}
}

func TestPublicationChildAdvanceAndDeliveryHaveSingleProductionCaller(t *testing.T) {
	root := runtimeGoRoot(t)
	advanceAllowed := map[string]bool{
		"internal/app/reportpublication/service.go":       true,
		"internal/app/reportpublication/restart_apply.go": true,
	}
	deliveryAllowed := map[string]bool{
		"internal/app/reportpublication/restart_apply.go": true,
	}
	for _, path := range goFiles(t, root) {
		if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, path, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "AdvancePublication":
				if !advanceAllowed[rel(t, root, path)] {
					t.Fatalf("%s bypasses the sole witnessed report publication path via %s", rel(t, root, path), selector.Sel.Name)
				}
			case "ProjectCommitted":
				t.Fatalf("%s retains forbidden commit-only report delivery", rel(t, root, path))
			case "ProjectAdmitted", "ResolveAdmitted":
				t.Fatalf("%s retains forbidden decision-only report delivery via %s", rel(t, root, path), selector.Sel.Name)
			case "ProjectCompleted":
				if !deliveryAllowed[rel(t, root, path)] {
					t.Fatalf("%s bypasses the completion-bound report delivery path", rel(t, root, path))
				}
			case "CreateOutcomeExclusive":
				if !deliveryAllowed[rel(t, root, path)] {
					t.Fatalf("%s bypasses the sole shared report delivery outcome CAS", rel(t, root, path))
				}
			case "NewReportDeliveryRejectionV1":
				if !deliveryAllowed[rel(t, root, path)] {
					t.Fatalf("%s mints a durable report delivery rejection outside the witnessed restart gate", rel(t, root, path))
				}
			}
			return true
		})
	}
}

func TestPrivateReportDeliveryAuthorityStaysInsideOwner(t *testing.T) {
	root := runtimeGoRoot(t)
	allowedRoots := []string{
		"internal/domain/reportpublication/",
		"internal/app/reportpublication/",
		"internal/ports/reportpublication/",
		"internal/adapters/outbound/reportpublication/",
	}
	privateFamilies := []string{
		"ReportDeliveryOutcome",
		"ReportDeliveryProjection",
		"ReportDeliveryRejection",
		"ReportStageCompletion",
		"ReportGrantSettlement",
	}
	purposeOwners := map[string]string{
		"analytix.report-delivery-projection/v1": "internal/domain/reportpublication/delivery_projection.go",
		"analytix.report-delivery-rejection/v1":  "internal/domain/reportpublication/delivery_rejection.go",
		"analytix.report-stage-completion/v1":    "internal/domain/reportpublication/stage_completion.go",
		"analytix.report-grant-settlement/v1":    "internal/domain/reportpublication/grant_settlement.go",
	}
	for _, path := range goFiles(t, root) {
		relative := rel(t, root, path)
		if strings.HasSuffix(relative, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		privateOwner := hasAnyReportPathPrefixV1(relative, allowedRoots)
		parsed := parseGoFile(t, path, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			if identifier, ok := node.(*ast.Ident); ok && !privateOwner {
				for _, family := range privateFamilies {
					if strings.Contains(identifier.Name, family) {
						t.Fatalf("%s references private report authority %s", relative, identifier.Name)
					}
				}
			}
			text, ok := staticReportStringV1(node)
			if !ok {
				return true
			}
			for purpose, owner := range purposeOwners {
				if strings.Contains(text, purpose) && relative != owner {
					t.Fatalf("%s embeds private report purpose %q outside %s", relative, purpose, owner)
				}
			}
			return true
		})
	}
}

func TestPublicBoundariesCannotImportPrivateReportOwners(t *testing.T) {
	root := runtimeGoRoot(t)
	privateOwners := map[string]bool{
		"analytix.local/runtime-go/internal/app/reportpublication":               true,
		"analytix.local/runtime-go/internal/ports/reportpublication":             true,
		"analytix.local/runtime-go/internal/adapters/outbound/reportpublication": true,
	}
	const domainOwner = "analytix.local/runtime-go/internal/domain/reportpublication"
	for _, path := range goFiles(t, root) {
		relative := rel(t, root, path)
		if strings.HasSuffix(relative, "_test.go") || hasBuildTag(t, path, "!analytix_prod") ||
			!isReportPublicBoundaryV1(relative) {
			continue
		}
		parsed := parseGoFile(t, path, 0)
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", relative, err)
			}
			if privateOwners[importPath] {
				t.Fatalf("%s imports private report owner %q", relative, importPath)
			}
			if importPath == domainOwner && imported.Name != nil &&
				(imported.Name.Name == "." || imported.Name.Name == "_") {
				t.Fatalf("%s uses opaque reportpublication import mode %q", relative, imported.Name.Name)
			}
		}
	}
}

func TestArtifactDeliveryBoundaryDoesNotReexportPrivateReportAuthority(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, path := range goFiles(t, root) {
		relative := rel(t, root, path)
		if strings.HasSuffix(relative, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
			continue
		}
		parsed := parseGoFile(t, path, 0)
		if strings.HasPrefix(relative, "internal/app/piiauthorization/") && strings.HasSuffix(relative, "_v2.go") {
			for _, imported := range parsed.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatalf("unquote import in %s: %v", relative, err)
				}
				if importPath == "analytix.local/runtime-go/internal/domain/reportpublication" ||
					importPath == "analytix.local/runtime-go/internal/ports/reportpublication" {
					t.Fatalf("%s bypasses the neutral artifact-delivery boundary via %q", relative, importPath)
				}
			}
		}
		if strings.HasPrefix(relative, "internal/app/reportpublication/") {
			continue
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "VerifiedArtifactDeliveryV1" {
				t.Fatalf("%s constructs a verified artifact delivery outside the report owner", relative)
			}
			return true
		})
	}
}

func TestReportPublicationSemanticGateMustPrecedeProductionComposition(t *testing.T) {
	root := runtimeGoRoot(t)
	const ownerImport = "analytix.local/runtime-go/internal/app/reportpublication"
	allowedRecoverySelectors := map[string]bool{
		"VerifyTrustedInventoryV1": true,
		"PreflightRestartV1":       true,
		"RestartPreflightConfigV1": true,
	}
	for _, directory := range []string{
		filepath.Join(root, "internal", "runtimeapp"),
		filepath.Join(root, "internal", "server"),
		filepath.Join(root, "internal", "adapters", "inbound"),
		filepath.Join(root, "cmd"),
	} {
		for _, path := range goFiles(t, directory) {
			if strings.HasSuffix(path, "_test.go") || hasBuildTag(t, path, "!analytix_prod") {
				continue
			}
			parsed := parseGoFile(t, path, 0)
			alias := ""
			for _, imported := range parsed.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatalf("unquote import in %s: %v", rel(t, root, path), err)
				}
				if importPath != ownerImport {
					continue
				}
				alias = "reportpublication"
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				if alias == "." || alias == "_" || strings.TrimSpace(alias) == "" {
					t.Fatalf("%s uses non-auditable report publication import mode %q", rel(t, root, path), alias)
				}
			}
			if alias == "" {
				continue
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				owner, ok := selector.X.(*ast.Ident)
				if !ok || owner.Name != alias {
					return true
				}
				if !allowedRecoverySelectors[selector.Sel.Name] {
					t.Fatalf(
						"%s composes report publication selector %s before the deterministic report semantic gate exists",
						rel(t, root, path), selector.Sel.Name,
					)
				}
				return true
			})
		}
	}
}

func hasAnyReportPathPrefixV1(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isReportPublicBoundaryV1(path string) bool {
	for _, prefix := range []string{
		"internal/adapters/inbound/httpapi/",
		"internal/adapters/inbound/sse/",
		"internal/app/thread/",
		"internal/domain/event/",
		"internal/adapters/outbound/eventlog/",
		"internal/server/",
		"cmd/runtime-server/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	switch path {
	case "internal/app/model/provider_history.go", "internal/app/turn/compaction.go", "internal/app/contextepoch/history_mutation.go":
		return true
	default:
		return false
	}
}

func staticReportStringV1(node ast.Node) (string, bool) {
	switch value := node.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(value.Value)
		return text, err == nil
	case *ast.ParenExpr:
		return staticReportStringV1(value.X)
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := staticReportStringV1(value.X)
		right, rightOK := staticReportStringV1(value.Y)
		if !leftOK || !rightOK {
			return "", false
		}
		return left + right, true
	default:
		return "", false
	}
}
