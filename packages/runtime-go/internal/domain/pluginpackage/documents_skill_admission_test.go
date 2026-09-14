package pluginpackage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDocumentsSkillLegacyRegistrationDigestV1(t *testing.T) {
	// Recorded against 7ba416fd2 before adding Documents Skill admission.
	for id, expected := range map[string]string{
		"analytix-documents":     "15952bea5def731ae070eec8bfa6d5ebd4a566b6382e7cd1027ffbc48e6c7cf8",
		"analytix-spreadsheets":  "1dfb6c240f68c3e333c1a043ee7a83848c813467a7a8e3ec1a817d5f53b42fb3",
		"analytix-presentations": "f52ebd1889be5b124eebe72c2aa12438cefcad2e1da0edeeee08ccc346eb708b",
	} {
		registration := developmentRegistrationFixtureV1(t, id)
		body, err := DevelopmentSourceRegistrationV1Bytes(registration)
		var fields map[string]json.RawMessage
		if err != nil || json.Unmarshal(body, &fields) != nil || len(fields) != 15 ||
			DevelopmentSourceRegistrationSHA256V1(registration) != expected ||
			registration.ContributionsSHA256 != "6b0bce24a7fcc903eb658b32173d8e54a45a78f0863e03c881b6bd97f840df0f" {
			t.Fatal("legacy registration shape or digest changed", id, err)
		}
		if _, exists := fields["documentsSkillSha256"]; exists {
			t.Fatal("legacy registration gained a Skill field")
		}
	}
}

func documentsSkillDeclarationV1() DeclarationV1 {
	declaration := developmentDeclarationFixtureV1("analytix-documents")
	declaration.Contributions.Skills = []PathContributionV1{{ID: DocumentsSkillContributionIDV1, Path: DocumentsSkillRelativePathV1}}
	declaration.RequestedCapabilities = append(declaration.RequestedCapabilities, CapabilityRequestV1{
		ID: "office.document-generation", ProtocolVersion: 1, ScopeConstraints: []string{"new-file", "current-conversation"},
	})
	return declaration
}

func documentsSkillRegistrationV1(t *testing.T) DevelopmentSourceRegistrationV1 {
	t.Helper()
	declaration := documentsSkillDeclarationV1()
	canonical, err := CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	registration := developmentRegistrationFixtureV1(t, "analytix-documents")
	registration.DeclarationCanonicalJSON = string(canonical)
	registration.DeclarationRawSHA256 = developmentSHA256V1(canonical)
	registration.DeclarationCanonicalSHA256 = developmentSHA256V1(canonical)
	registration.DocumentsSkillSHA256 = developmentSHA256V1([]byte("# Synthetic Documents\n"))
	registration.SourceTreeFileCount = 5
	registration, err = NewDevelopmentSourceRegistrationV1(registration)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func TestDocumentsSkillDeclarationRequiresExactPairedCapabilityV1(t *testing.T) {
	declaration := documentsSkillDeclarationV1()
	if err := ValidateDevelopmentSourceDeclarationV1(declaration); err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDeclarationV1(canonical)
	if err != nil || ValidateDevelopmentSourceDeclarationV1(parsed) != nil {
		t.Fatal("canonical capability ordering rejected", err)
	}
	for name, mutate := range map[string]func(*DeclarationV1){
		"spreadsheets":       func(d *DeclarationV1) { d.PackageID = "analytix-spreadsheets" },
		"presentations":      func(d *DeclarationV1) { d.PackageID = "analytix-presentations" },
		"missing skill":      func(d *DeclarationV1) { d.Contributions.Skills = []PathContributionV1{} },
		"missing capability": func(d *DeclarationV1) { d.RequestedCapabilities = d.RequestedCapabilities[:1] },
		"wrong id":           func(d *DeclarationV1) { d.Contributions.Skills[0].ID = "other" },
		"wrong path":         func(d *DeclarationV1) { d.Contributions.Skills[0].Path = "skills/other/SKILL.md" },
		"extra skill": func(d *DeclarationV1) {
			d.Contributions.Skills = append(d.Contributions.Skills, PathContributionV1{ID: "other", Path: "skills/other/SKILL.md"})
		},
		"protocol":     func(d *DeclarationV1) { d.RequestedCapabilities[1].ProtocolVersion = 2 },
		"overwrite":    func(d *DeclarationV1) { d.RequestedCapabilities[1].ScopeConstraints[0] = "overwrite" },
		"cross thread": func(d *DeclarationV1) { d.RequestedCapabilities[1].ScopeConstraints[1] = "all-conversations" },
		"extra scope": func(d *DeclarationV1) {
			d.RequestedCapabilities[1].ScopeConstraints = append(d.RequestedCapabilities[1].ScopeConstraints, "shell")
		},
		"missing preview": func(d *DeclarationV1) { d.RequestedCapabilities = d.RequestedCapabilities[1:] },
		"preview write":   func(d *DeclarationV1) { d.RequestedCapabilities[0].ScopeConstraints[1] = "explicit-save" },
		"historical edit mixed": func(d *DeclarationV1) {
			d.RequestedCapabilities[0].ID = "office.local-edit"
			d.RequestedCapabilities[0].ScopeConstraints[1] = "explicit-save"
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := documentsSkillDeclarationV1()
			mutate(&invalid)
			if ValidateDevelopmentSourceDeclarationV1(invalid) == nil || validateDevelopmentSourceDeclarationV1(invalid, true) == nil {
				t.Fatal("widened Documents Skill declaration accepted")
			}
		})
	}
}

func TestDocumentsSkillRegistrationCanonicalBindingV1(t *testing.T) {
	registration := documentsSkillRegistrationV1(t)
	body, err := DevelopmentSourceRegistrationV1Bytes(registration)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || len(fields) != 16 {
		t.Fatal("new registration did not use exact 16-field shape")
	}
	parsed, err := ParseDevelopmentSourceRegistrationV1(body)
	if err != nil || parsed != registration {
		t.Fatal("new registration roundtrip failed", err)
	}
	historical, historicalDigest, err := ReconstructHistoricalDevelopmentSourceRegistrationV1(registration)
	if err != nil || !bytes.Equal(historical, body) || historicalDigest != DevelopmentSourceRegistrationSHA256V1(registration) {
		t.Fatal("installed new registration reconstruction drifted", err)
	}
	changed := registration
	changed.DocumentsSkillSHA256 = strings.Repeat("e", 64)
	if ValidateDevelopmentSourceRegistrationV1(changed) == nil {
		t.Fatal("forged Skill hash retained contribution authority")
	}
	changed, err = NewDevelopmentSourceRegistrationV1(changed)
	if err != nil || changed.ContributionsSHA256 == registration.ContributionsSHA256 || DevelopmentSourceRegistrationSHA256V1(changed) == DevelopmentSourceRegistrationSHA256V1(registration) {
		t.Fatal("changed Skill did not change contributions and registration identity", err)
	}
	for _, hash := range []string{"", "not-a-hash", strings.Repeat("E", 64)} {
		invalid := registration
		invalid.DocumentsSkillSHA256 = hash
		if _, err := NewDevelopmentSourceRegistrationV1(invalid); err == nil {
			t.Fatal("invalid paired Skill hash accepted")
		}
	}
	legacy := developmentRegistrationFixtureV1(t, "analytix-documents")
	legacy.DocumentsSkillSHA256 = registration.DocumentsSkillSHA256
	if _, err := NewDevelopmentSourceRegistrationV1(legacy); err == nil {
		t.Fatal("undeclared Skill hash accepted")
	}
	field := []byte(`"documentsSkillSha256":"` + registration.DocumentsSkillSHA256 + `",`)
	for name, invalid := range map[string][]byte{
		"missing":   bytes.Replace(body, field, nil, 1),
		"empty":     bytes.Replace(body, field, []byte(`"documentsSkillSha256":"",`), 1),
		"null":      bytes.Replace(body, field, []byte(`"documentsSkillSha256":null,`), 1),
		"unknown":   bytes.Replace(body, field, []byte(`"arbitrarySkillSha256":"`+registration.DocumentsSkillSHA256+`",`), 1),
		"duplicate": bytes.Replace(body, field, append(append([]byte{}, field...), field...), 1),
		"extra":     append(append([]byte{}, body[:len(body)-1]...), []byte(`,"extra":false}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDevelopmentSourceRegistrationV1(invalid); err == nil {
				t.Fatal("noncanonical registration accepted")
			}
		})
	}
	legacy = developmentRegistrationFixtureV1(t, "analytix-documents")
	legacyBytes, _ := DevelopmentSourceRegistrationV1Bytes(legacy)
	for _, value := range []string{`""`, `null`, `"` + registration.DocumentsSkillSHA256 + `"`} {
		invalid := bytes.Replace(legacyBytes, []byte(`"contributionsSha256"`), []byte(`"documentsSkillSha256":`+value+`,"contributionsSha256"`), 1)
		if _, err := ParseDevelopmentSourceRegistrationV1(invalid); err == nil {
			t.Fatal("legacy shape admitted optional unpaired field")
		}
	}
}

func TestOfficeSkillRegistrationCannotMixPackageAuthorities(t *testing.T) {
	for _, id := range []string{"analytix-spreadsheets", "analytix-presentations"} {
		t.Run(id, func(t *testing.T) {
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
			r.DeclarationRawSHA256 = developmentSHA256V1(canonical)
			r.DeclarationCanonicalSHA256 = developmentSHA256V1(canonical)
			r.SourceTreeFileCount = 5
			if id == "analytix-spreadsheets" {
				r.SpreadsheetsSkillSHA256 = strings.Repeat("e", 64)
			} else {
				r.PresentationsSkillSHA256 = strings.Repeat("e", 64)
			}
			r, err = NewDevelopmentSourceRegistrationV1(r)
			if err != nil {
				t.Fatal("exact package contribution rejected", err)
			}
			body, err := DevelopmentSourceRegistrationV1Bytes(r)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseDevelopmentSourceRegistrationV1(body)
			if err != nil || parsed != r {
				t.Fatal("exact registration changed", err)
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(body, &fields) != nil || len(fields) != 16 {
				t.Fatal("nonminimal registration shape")
			}
			r.DocumentsSkillSHA256 = strings.Repeat("e", 64)
			if _, err := NewDevelopmentSourceRegistrationV1(r); err == nil {
				t.Fatal("another package skill hash admitted")
			}
			d.RequestedCapabilities[len(d.RequestedCapabilities)-1].ID = "office.document-generation"
			if ValidateDevelopmentSourceDeclarationV1(d) == nil {
				t.Fatal("another package generation capability admitted")
			}
		})
	}
}
