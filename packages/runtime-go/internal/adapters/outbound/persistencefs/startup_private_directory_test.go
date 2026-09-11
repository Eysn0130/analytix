package persistencefs

import (
	"reflect"
	"sort"
	"testing"
)

func TestStartupPrivateDirectoryEntriesAreRepeatable(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	directory, err := secureStartupCreateDirectory(authority.namespace.root, "inventory")
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	for _, name := range []string{"first.json", "second.json"} {
		if err := directory.WriteExclusive(name, []byte(`{"ok":true}`), 64); err != nil {
			t.Fatal(err)
		}
	}
	readNames := func() []string {
		entries, err := directory.Entries()
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		return names
	}
	first := readNames()
	second := readNames()
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, []string{"first.json", "second.json"}) {
		t.Fatalf("repeated startup directory inventory changed: first=%v second=%v", first, second)
	}
}
