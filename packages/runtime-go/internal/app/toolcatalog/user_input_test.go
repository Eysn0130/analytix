package toolcatalog

import "testing"

func TestUserInputToolSchemasKeepGateAliasPair(t *testing.T) {
	schemas := UserInputToolSchemas()
	if len(schemas) != 2 {
		t.Fatalf("user input schema count drifted: %d", len(schemas))
	}
	for _, schema := range schemas {
		if schema.Source != "user_input" {
			t.Fatalf("schema %s source drifted: %#v", schema.Name, schema)
		}
	}
	if schemas[0].Name != "user_input" || schemas[1].Name != "request_user_input" {
		t.Fatalf("user input schema alias order drifted: %#v", schemas)
	}
}
