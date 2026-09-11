package toolcatalog

import "testing"

func TestGoalAndTodoToolSchemasExposeEvidenceClosureContract(t *testing.T) {
	tools := GoalAndTodoToolSchemas()
	byName := map[string]bool{}
	bySource := map[string]string{}
	for _, tool := range tools {
		byName[tool.Name] = true
		bySource[tool.Name] = tool.Source
		if len(tool.Parameters) == 0 {
			t.Fatalf("%s schema has empty parameters", tool.Name)
		}
	}
	for _, name := range []string{"get_goal", "create_goal", "complete_step", "update_goal", "todo_list", "todo_write", "todo_ops", "todo_patch"} {
		if !byName[name] {
			t.Fatalf("missing goal/todo tool %s in %#v", name, tools)
		}
	}
	if bySource["complete_step"] != "goal" || bySource["todo_write"] != "todo" || bySource["todo_ops"] != "todo" || bySource["todo_patch"] != "todo" {
		t.Fatalf("unexpected goal/todo sources: %#v", bySource)
	}
	if hash := ToolSchemaHash(tools); hash == "" {
		t.Fatal("goal/todo schema hash should be stable and non-empty")
	}
}
