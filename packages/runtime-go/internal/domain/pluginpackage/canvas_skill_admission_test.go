package pluginpackage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func canvasDeclarationFixtureV1() DeclarationV1 {
	d := developmentDeclarationFixtureV1("analytix-canvas")
	d.Contributions.Skills = []PathContributionV1{{ID: CanvasSkillContributionIDV1, Path: CanvasSkillRelativePathV1}}
	d.RequestedCapabilities = []CapabilityRequestV1{
		{ID: "canvas.local-preview", ProtocolVersion: 1, ScopeConstraints: []string{"user-selected-object", "read-only"}},
		{ID: "canvas.local-edit", ProtocolVersion: 1, ScopeConstraints: []string{"user-selected-object", "reviewed-commit"}},
		{ID: "canvas.generation", ProtocolVersion: 1, ScopeConstraints: []string{"new-file", "current-conversation"}},
	}
	return d
}

func canvasRegistrationFixtureV1(t *testing.T) DevelopmentSourceRegistrationV1 {
	t.Helper()
	d := canvasDeclarationFixtureV1()
	body, err := CanonicalDeclarationV1Bytes(d)
	if err != nil {
		t.Fatal(err)
	}
	r := developmentRegistrationFixtureV1(t, "analytix-documents")
	r.Identity = d.IdentityV1()
	r.DeclarationCanonicalJSON = string(body)
	r.DeclarationRawSHA256 = developmentSHA256V1(body)
	r.DeclarationCanonicalSHA256 = developmentSHA256V1(body)
	r.CanvasSkillSHA256 = developmentSHA256V1([]byte("Synthetic Canvas instructions"))
	r.SourceTreeFileCount = 5
	r, err = NewDevelopmentSourceRegistrationV1(r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCanvasDeclarationRequiresExactSkillAndThreeCapabilitiesV1(t *testing.T) {
	d := canvasDeclarationFixtureV1()
	body, err := CanonicalDeclarationV1Bytes(d)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDeclarationV1(body)
	if err != nil || ValidateDevelopmentSourceDeclarationV1(parsed) != nil {
		t.Fatal("canonical Canvas rejected", err)
	}
	for name, mutate := range map[string]func(*DeclarationV1){
		"no skill":    func(d *DeclarationV1) { d.Contributions.Skills = nil },
		"wrong skill": func(d *DeclarationV1) { d.Contributions.Skills[0].ID = "documents" },
		"wrong path":  func(d *DeclarationV1) { d.Contributions.Skills[0].Path = "skills/other/SKILL.md" },
		"extra skill": func(d *DeclarationV1) {
			d.Contributions.Skills = append(d.Contributions.Skills, PathContributionV1{ID: "extra", Path: "extra/SKILL.md"})
		},
		"missing capability":   func(d *DeclarationV1) { d.RequestedCapabilities = d.RequestedCapabilities[:2] },
		"duplicate capability": func(d *DeclarationV1) { d.RequestedCapabilities[1] = d.RequestedCapabilities[0] },
		"office capability":    func(d *DeclarationV1) { d.RequestedCapabilities[0].ID = "office.local-preview" },
		"preview write":        func(d *DeclarationV1) { d.RequestedCapabilities[0].ScopeConstraints[1] = "reviewed-commit" },
		"unreviewed edit":      func(d *DeclarationV1) { d.RequestedCapabilities[1].ScopeConstraints[1] = "explicit-save" },
		"overwrite":            func(d *DeclarationV1) { d.RequestedCapabilities[2].ScopeConstraints[0] = "overwrite" },
		"cross conversation":   func(d *DeclarationV1) { d.RequestedCapabilities[2].ScopeConstraints[1] = "all-conversations" },
		"extra scope": func(d *DeclarationV1) {
			d.RequestedCapabilities[2].ScopeConstraints = append(d.RequestedCapabilities[2].ScopeConstraints, "network")
		},
		"protocol":         func(d *DeclarationV1) { d.RequestedCapabilities[1].ProtocolVersion = 2 },
		"Office identity":  func(d *DeclarationV1) { d.PackageID = "analytix-documents" },
		"unknown identity": func(d *DeclarationV1) { d.PackageID = "analytix-other" },
		"hook":             func(d *DeclarationV1) { d.Contributions.Hooks = []PathContributionV1{{ID: "run", Path: "run.js"}} },
	} {
		t.Run(name, func(t *testing.T) {
			d := canvasDeclarationFixtureV1()
			mutate(&d)
			if ValidateDevelopmentSourceDeclarationV1(d) == nil || validateDevelopmentSourceDeclarationV1(d, true) == nil {
				t.Fatal("widened Canvas admitted")
			}
		})
	}
	if _, _, ok := OfficeSkillContributionV1("analytix-canvas"); ok {
		t.Fatal("Canvas acquired Office generation identity")
	}
	if _, ok := StaticEditorSkillContributionV1("analytix-other"); ok {
		t.Fatal("arbitrary static skill admitted")
	}
}

func TestCanvasRegistrationRequiresItsOwnCanonicalSkillHashV1(t *testing.T) {
	r := canvasRegistrationFixtureV1(t)
	body, err := DevelopmentSourceRegistrationV1Bytes(r)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || len(fields) != 16 {
		t.Fatal("Canvas shape not exact")
	}
	parsed, err := ParseDevelopmentSourceRegistrationV1(body)
	if err != nil || parsed != r {
		t.Fatal("roundtrip", err)
	}
	historical, digest, err := ReconstructHistoricalDevelopmentSourceRegistrationV1(r)
	if err != nil || !bytes.Equal(historical, body) || digest != DevelopmentSourceRegistrationSHA256V1(r) {
		t.Fatal("installed reconstruction drift", err)
	}
	if r.OfficeSkillSHA256V1() != "" || r.StaticEditorSkillSHA256V1() != r.CanvasSkillSHA256 {
		t.Fatal("cross-package skill authority")
	}
	changed := r
	changed.CanvasSkillSHA256 = strings.Repeat("e", 64)
	if ValidateDevelopmentSourceRegistrationV1(changed) == nil {
		t.Fatal("forged hash retained digest")
	}
	changed, err = NewDevelopmentSourceRegistrationV1(changed)
	if err != nil || changed.ContributionsSHA256 == r.ContributionsSHA256 || DevelopmentSourceRegistrationSHA256V1(changed) == digest {
		t.Fatal("changed bytes retained identity", err)
	}
	for _, hash := range []string{"", "invalid", strings.Repeat("E", 64)} {
		bad := r
		bad.CanvasSkillSHA256 = hash
		if _, err := NewDevelopmentSourceRegistrationV1(bad); err == nil {
			t.Fatal("invalid Canvas hash admitted")
		}
	}
	for _, field := range []string{"documentsSkillSha256", "spreadsheetsSkillSha256", "presentationsSkillSha256"} {
		bad := bytes.Replace(body, []byte("\"canvasSkillSha256\""), []byte("\""+field+"\""), 1)
		if _, err := ParseDevelopmentSourceRegistrationV1(bad); err == nil {
			t.Fatal("foreign hash admitted", field)
		}
	}
	field := []byte(`"canvasSkillSha256":"` + r.CanvasSkillSHA256 + `",`)
	for name, bad := range map[string][]byte{
		"missing":   bytes.Replace(body, field, nil, 1),
		"empty":     bytes.Replace(body, field, []byte(`"canvasSkillSha256":"",`), 1),
		"null":      bytes.Replace(body, field, []byte(`"canvasSkillSha256":null,`), 1),
		"unknown":   bytes.Replace(body, field, []byte(`"unknownSkillSha256":"`+r.CanvasSkillSHA256+`",`), 1),
		"duplicate": bytes.Replace(body, field, append(append([]byte{}, field...), field...), 1),
		"extra":     bytes.Replace(body, field, append(append([]byte{}, field...), []byte(`"documentsSkillSha256":"`+r.CanvasSkillSHA256+`",`)...), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDevelopmentSourceRegistrationV1(bad); err == nil {
				t.Fatal("invalid canonical shape admitted")
			}
		})
	}
	for _, id := range []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations"} {
		old := developmentRegistrationFixtureV1(t, id)
		old.CanvasSkillSHA256 = r.CanvasSkillSHA256
		if _, err := NewDevelopmentSourceRegistrationV1(old); err == nil {
			t.Fatal("Canvas hash widened Office", id)
		}
	}
}

// Encode the pre-Canvas 16-field wire shape independently of the registration
// struct. Adding Canvas must not rotate installed Office registration identity.
func TestCanvasAdmissionPreservesOfficeSkillWireIdentityV1(t *testing.T) {
	for _, id := range []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations"} {
		d := developmentDeclarationFixtureV1(id)
		skill, capability, _ := OfficeSkillContributionV1(id)
		d.Contributions.Skills = []PathContributionV1{skill}
		d.RequestedCapabilities = append(d.RequestedCapabilities, capability)
		canonical, err := CanonicalDeclarationV1Bytes(d)
		if err != nil {
			t.Fatal(err)
		}
		r := developmentRegistrationFixtureV1(t, id)
		r.DeclarationCanonicalJSON = string(canonical)
		r.DeclarationCanonicalSHA256 = developmentSHA256V1(canonical)
		r.DeclarationRawSHA256 = r.DeclarationCanonicalSHA256
		r.SourceTreeFileCount = 5
		hash := strings.Repeat("e", 64)
		skillField := "documentsSkillSha256"
		switch id {
		case "analytix-documents":
			r.DocumentsSkillSHA256 = hash
		case "analytix-spreadsheets":
			r.SpreadsheetsSkillSHA256 = hash
			skillField = "spreadsheetsSkillSha256"
		case "analytix-presentations":
			r.PresentationsSkillSHA256 = hash
			skillField = "presentationsSkillSha256"
		}
		r, err = NewDevelopmentSourceRegistrationV1(r)
		if err != nil {
			t.Fatal(err)
		}
		legacyContributions := `[{"kind":"publicUi","id":"workspace-editor","path":"ui/editor.json","sha256":"` + r.PublicUISHA256 + `"},{"kind":"assets","id":"editor-adapter","path":"assets/adapter.json","sha256":"` + r.AdapterSHA256 + `"},{"kind":"skills","id":"` + skill.ID + `","path":"` + skill.Path + `","sha256":"` + hash + `"}]`
		legacyContributionDigest := developmentSHA256V1(append([]byte("analytix.development-source-contributions/v1\x00"), []byte(legacyContributions)...))
		if r.ContributionsSHA256 != legacyContributionDigest {
			t.Fatal("Office contribution digest changed", id)
		}
		fields := []struct {
			key   string
			value any
		}{
			{"schemaVersion", 1}, {"origin", "development-source"}, {"executionMode", "source-experiment"}, {"identity", r.Identity},
			{"declarationRawSha256", r.DeclarationRawSHA256}, {"declarationCanonicalSha256", r.DeclarationCanonicalSHA256}, {"declarationCanonicalJson", r.DeclarationCanonicalJSON},
			{"sourceTreeSha256", r.SourceTreeSHA256}, {"sourceTreeFileCount", r.SourceTreeFileCount}, {"manifestSha256", r.ManifestSHA256}, {"publicUiSha256", r.PublicUISHA256}, {"adapterSha256", r.AdapterSHA256},
			{skillField, hash}, {"contributionsSha256", legacyContributionDigest}, {"publishable", false}, {"factToolsEnabled", false},
		}
		var legacy bytes.Buffer
		legacy.WriteByte('{')
		for i, field := range fields {
			if i > 0 {
				legacy.WriteByte(',')
			}
			key, _ := json.Marshal(field.key)
			value, _ := json.Marshal(field.value)
			legacy.Write(key)
			legacy.WriteByte(':')
			legacy.Write(value)
		}
		legacy.WriteByte('}')
		body, err := DevelopmentSourceRegistrationV1Bytes(r)
		expectedDigest := developmentSHA256V1(append([]byte("analytix.development-source-registration/v1\x00"), legacy.Bytes()...))
		if err != nil || !bytes.Equal(body, legacy.Bytes()) || DevelopmentSourceRegistrationSHA256V1(r) != expectedDigest {
			t.Fatal("Office 16-field wire identity changed", id, err)
		}
	}
}
