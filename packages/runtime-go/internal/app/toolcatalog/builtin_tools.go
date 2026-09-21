package toolcatalog

import (
	"encoding/json"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type BuiltinToolSchemaInput struct {
	DocumentGeneration      bool
	DocumentGenerationKinds []string
	NativeSelections        bool
	AllowBackgroundBash     bool
	WebFetch                bool
}

func BuiltinToolSchemas(input BuiltinToolSchemaInput) []domainmodel.ToolSchema {
	bashParameters := json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"timeout":{"type":"number","description":"Optional timeout in seconds; the runtime default and maximum are 120 seconds. Omit unless the user explicitly requests a timeout field."},"run_in_background":{"type":"boolean","description":"Optional background execution. Omit unless the user explicitly requests background execution."},"runInBackground":{"type":"boolean","description":"Optional background execution. Omit unless the user explicitly requests background execution."}},"required":["command"],"additionalProperties":false}`)
	if !input.AllowBackgroundBash {
		bashParameters = json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"timeout":{"type":"number","description":"Optional timeout in seconds; the runtime default and maximum are 120 seconds. Omit unless the user explicitly requests a timeout field."}},"required":["command"],"additionalProperties":false}`)
	}
	tools := []domainmodel.ToolSchema{
		{
			Name:        "read",
			Description: "Read a file. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured read root, including read-root aliases. Supports optional 1-based line offset and limit for large files.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","description":"1-based line number to start reading from (default 1)","minimum":1},"limit":{"type":"integer","description":"Maximum lines to return (default 2000)","minimum":1}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Name:        "bash",
			Description: "Execute a shell command in the workspace using the host platform shell. When the user supplies an exact command string, copy it unchanged into command. Do not add timeout unless the user explicitly requests a timeout. Windows uses PowerShell when available and falls back to cmd.exe, so prefer host-appropriate syntax. Requires approval unless policy is auto.",
			Parameters:  bashParameters,
		},
		{
			Name:        "write",
			Description: "Create or overwrite a file with the provided content. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Requires approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`),
		},
		{
			Name:        "edit",
			Description: "Edit a file by replacing exact text previously observed with the read tool. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Requires approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"oldText":{"type":"string"},"old_text":{"type":"string"},"old_string":{"type":"string"},"newText":{"type":"string"},"new_text":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"},"replaceAll":{"type":"boolean"},"edits":{"type":"array","items":{"type":"object","properties":{"oldText":{"type":"string"},"old_text":{"type":"string"},"old_string":{"type":"string"},"newText":{"type":"string"},"new_text":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"},"replaceAll":{"type":"boolean"}},"additionalProperties":false}}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Name:        "grep",
			Description: "Search file contents for a pattern and return matching lines with paths, line numbers, columns, and optional context. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured read root, including read-root aliases.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string"},"ignoreCase":{"type":"boolean"},"literal":{"type":"boolean"},"context":{"type":"number"},"limit":{"type":"number"}},"required":["pattern"],"additionalProperties":false}`),
		},
		{
			Name:        "find",
			Description: "Find files by glob-like pattern. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured read root, including read-root aliases.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"limit":{"type":"number"}},"required":["pattern"],"additionalProperties":false}`),
		},
		{
			Name:        "glob",
			Description: "Find files matching a glob pattern. Relative patterns resolve inside the active workspace; explicit external absolute patterns are allowed only with danger-full-access or a configured read root. Supports *, ?, [] and recursive ** matching; skips nested dependency and VCS directories unless explicitly targeted.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Glob pattern, e.g. \"*.go\", \"internal/*/*.go\", \"**/*.test.ts\", or an explicit absolute pattern when external reads are allowed"},"limit":{"type":"number"}},"required":["pattern"],"additionalProperties":false}`),
		},
		{
			Name:        "code_index",
			Description: "Lightweight read-only code symbol index for local file outlines and symbol definition candidates. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured read root, including read-root aliases. Prefer precise reads before editing and language/MCP tools when deeper semantics are required.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["outline","search"],"description":"outline lists symbols under path; search finds symbol definition candidates by name"},"path":{"type":"string","description":"File or directory path to inspect; defaults to workspace root"},"query":{"type":"string","description":"Symbol name or substring for action=search"},"kind":{"type":"string","description":"Optional symbol kind filter such as func, method, class, type, interface, const, var, struct, enum, trait"},"limit":{"type":"integer","description":"Maximum symbols to return (default 100, max 200)","minimum":1}},"required":["action"],"additionalProperties":false}`),
		},
		{
			Name:        "ls",
			Description: "List directory contents. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured read root, including read-root aliases. Returns entries sorted alphabetically and marks directories.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"limit":{"type":"number"}},"additionalProperties":false}`),
		},
		{
			Name:        "read_file",
			Description: "Read a text file. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured read root, including read-root aliases. Uses 0-based offset/limit pagination and prefixes each returned line with its 1-based line number.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","description":"0-based line offset to start reading from (default 0)","minimum":0},"limit":{"type":"integer","description":"Maximum lines to return (default 2000)","minimum":1}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Name:        "write_file",
			Description: "Write text content to a file. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Requires approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`),
		},
		{
			Name:        "edit_file",
			Description: "Edit a file by replacing exact text previously observed with the read tool. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Supports old_string/new_string aliases, ordered multi-edit batches, and replace_all. Requires approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"oldText":{"type":"string"},"old_text":{"type":"string"},"old_string":{"type":"string"},"newText":{"type":"string"},"new_text":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"},"replaceAll":{"type":"boolean"},"edits":{"type":"array","items":{"type":"object","properties":{"oldText":{"type":"string"},"old_text":{"type":"string"},"old_string":{"type":"string"},"newText":{"type":"string"},"new_text":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"},"replaceAll":{"type":"boolean"}},"additionalProperties":false}}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Name:        "multi_edit",
			Description: "Apply ordered exact-text edits to one file atomically. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Each edit runs against the result of the previous edit; the file is written only if every edit succeeds. Requires approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","minItems":1,"items":{"type":"object","properties":{"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["old_string","new_string"],"additionalProperties":false}}},"required":["path","edits"],"additionalProperties":false}`),
		},
		{
			Name:        "move_file",
			Description: "Move or rename one file from source_path to destination_path. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Creates the destination parent directory as needed and refuses to overwrite existing files. Requires approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"source_path":{"type":"string","description":"Existing file path to move"},"destination_path":{"type":"string","description":"Destination file path; must not already exist"}},"required":["source_path","destination_path"],"additionalProperties":false}`),
		},
		{
			Name:        "notebook_edit",
			Description: "Edit one cell of a Jupyter .ipynb notebook while preserving valid JSON. Supports replace, insert, and delete by 0-based cell_number or cell_id. Editing a code cell clears outputs. Requires approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the .ipynb notebook"},"cell_number":{"type":"integer","description":"0-based target cell index; for insert the new cell goes after this one, and -1 prepends"},"cell_id":{"type":"string","description":"Target cell id for replace/delete"},"new_source":{"type":"string","description":"Replacement or inserted cell source"},"content":{"type":"string","description":"Alias for new_source"},"source":{"type":"string","description":"Alias for new_source"},"new_string":{"type":"string","description":"Alias for new_source"},"cell_type":{"type":"string","enum":["code","markdown"],"description":"Cell type for insert, or optional retype for replace"},"edit_mode":{"type":"string","enum":["replace","insert","delete"],"description":"replace (default), insert, or delete"}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Name:        "delete_range",
			Description: "Delete a contiguous text range from a file using exact unique start/end line anchors. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Defaults to deleting the anchor lines too. Requires a fresh read of the file and approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"start_anchor":{"type":"string","description":"Exact text of the first line to delete; must be unique in the file"},"end_anchor":{"type":"string","description":"Exact text of the last line to delete; must be unique in the file"},"inclusive":{"type":"boolean","description":"Whether to include the anchor lines in the deletion (default true)"}},"required":["path","start_anchor","end_anchor"],"additionalProperties":false}`),
		},
		{
			Name:        "delete_symbol",
			Description: "Delete a named Go symbol from a .go file using AST parsing. Relative paths resolve inside the active workspace; explicit external paths are allowed with danger-full-access or a configured allow_write root. Supports func, method, type, interface, const, and var; use kind and parent to disambiguate methods. Requires a fresh read of the file and approval unless policy is auto.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Go source file path"},"name":{"type":"string","description":"Symbol name to delete"},"kind":{"type":"string","description":"Optional kind filter: func, method, type, interface, const, var"},"parent":{"type":"string","description":"Optional receiver/parent type for method disambiguation"}},"required":["path","name"],"additionalProperties":false}`),
		},
	}
	if input.DocumentGeneration {
		tools = append(tools, domainmodel.ToolSchema{
			Name:        "generate_office_document",
			Description: "Create a new native Office file: DOCX from Markdown, XLSX from typed sheets and checked formulas, or PPTX from structured slides. Only currently activated plugin kinds are available. Never overwrites an existing file. Use image identifiers in Markdown, not URLs or local image paths. The complete request is limited to 4 MiB of JSON and each string to 1 MiB of UTF-8. Core validates the file and confirms saving before returning an artifact. Follow the matching Office plugin skill; normal file permission and approval policy apply.",
			Parameters:  GenerationToolParameters(input.DocumentGenerationKinds),
		})
	}
	if input.NativeSelections {
		tools = append(tools, []domainmodel.ToolSchema{
			{Name: "native_selection_read", Description: "Read the model-safe projection of a native Office, Canvas, browser selected-text or image-region selection explicitly attached to this conversation. Browser selections are discussion-only, privately revalidated at each read, and prove selected-text freshness only. An image-region scope contains geometry and projected noteParts only; imageObservation is unavailable and no image pixels are supplied. Use only its opaque scopeId. Protected parts are immutable references. This does not read a file path or expose raw document bytes.", Parameters: json.RawMessage(`{"type":"object","properties":{"scopeId":{"type":"string","pattern":"^[a-f0-9]{48}$"}},"required":["scopeId"],"additionalProperties":false}`)},
			{Name: "native_selection_propose", Description: "Propose a replacement for an editable native Office or Canvas selection in this conversation. For a Canvas scope use canvas presentation-only operations with selected opaque object aliases; source facts, provenance and unselected endpoints are not editable. Use native_selection_read first. For a typed workbook scope use workbook kind number/formula (single cell) or range with unique scope-relative rowOffset/columnOffset; formula references must remain inside the captured rectangle. For a typed presentation scope use presentation kind shape-geometry (integer 1/100 mm coordinates and size) or shape-fill (direct solid RGB); only the captured target may change. Chart data, groups, rotation, inherited styles and arbitrary shape selectors are unsupported. For selected shape text or other text scopes use parts instead of presentation. Never coerce numbers or formulas through text. Return typed literal/protected parts; include every protected reference exactly once in original order, never substitute or disclose protected text. This creates a proposal for user approval; it does not apply, save or overwrite the document. Use a stable operationId for retries.", Parameters: json.RawMessage(`{"type":"object","properties":{"scopeId":{"type":"string","pattern":"^[a-f0-9]{48}$"},"operationId":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$"},"parts":{"type":"array","maxItems":256,"items":{"oneOf":[{"type":"object","properties":{"kind":{"type":"string","enum":["literal"]},"text":{"type":"string","maxLength":65536}},"required":["kind","text"],"additionalProperties":false},{"type":"object","properties":{"kind":{"type":"string","enum":["protected"]},"protectedRef":{"type":"string","pattern":"^protected_[a-f0-9]{48}$"}},"required":["kind","protectedRef"],"additionalProperties":false}]}},"workbook":{"oneOf":[{"type":"object","properties":{"kind":{"const":"number"},"value":{"type":"number"}},"required":["kind","value"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"formula"},"formula":{"type":"string","minLength":1,"maxLength":1024}},"required":["kind","formula"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"range"},"cells":{"type":"array","minItems":1,"maxItems":256,"items":{"oneOf":[{"type":"object","properties":{"rowOffset":{"type":"integer","minimum":0,"maximum":255},"columnOffset":{"type":"integer","minimum":0,"maximum":255},"type":{"const":"number"},"value":{"type":"number"}},"required":["rowOffset","columnOffset","type","value"],"additionalProperties":false},{"type":"object","properties":{"rowOffset":{"type":"integer","minimum":0,"maximum":255},"columnOffset":{"type":"integer","minimum":0,"maximum":255},"type":{"const":"formula"},"formula":{"type":"string","minLength":1,"maxLength":1024}},"required":["rowOffset","columnOffset","type","formula"],"additionalProperties":false},{"type":"object","properties":{"rowOffset":{"type":"integer","minimum":0,"maximum":255},"columnOffset":{"type":"integer","minimum":0,"maximum":255},"type":{"const":"text"},"text":{"type":"string","maxLength":4096}},"required":["rowOffset","columnOffset","type","text"],"additionalProperties":false}]}}},"required":["kind","cells"],"additionalProperties":false}]},"canvas":{"type":"array","minItems":1,"maxItems":128,"items":{"oneOf":[{"type":"object","properties":{"kind":{"const":"set-node-layout"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"layout":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000},"width":{"type":"number","exclusiveMinimum":0,"maximum":1000000},"height":{"type":"number","exclusiveMinimum":0,"maximum":1000000}},"required":["x","y","width","height"],"additionalProperties":false}},"required":["kind","id","layout"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"set-edge-route"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"points":{"type":"array","maxItems":32,"items":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000}},"required":["x","y"],"additionalProperties":false},"anyOf":[{"type":"array","items":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000}},"required":["x","y"],"additionalProperties":false},"maxItems":0},{"type":"array","items":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000}},"required":["x","y"],"additionalProperties":false},"minItems":2,"maxItems":32}]}},"required":["kind","id","points"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"set-display-label"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"target":{"enum":["node","edge"]},"displayLabel":{"type":"string","maxLength":1024}},"required":["kind","id","target","displayLabel"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"set-style"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"target":{"enum":["node","edge"]},"style":{"type":"object","properties":{"fill":{"type":"string","pattern":"^#[0-9A-Fa-f]{6}$"},"stroke":{"type":"string","pattern":"^#[0-9A-Fa-f]{6}$"},"strokeWidth":{"type":"integer","minimum":1,"maximum":16},"dash":{"type":"string","enum":["solid","dashed"]},"shape":{"type":"string","enum":["rectangle","rounded","ellipse","line"]}},"required":["fill","stroke","strokeWidth","dash","shape"],"additionalProperties":false}},"required":["kind","id","target","style"],"additionalProperties":false}]}},"presentation":{"oneOf":[{"type":"object","properties":{"kind":{"const":"shape-geometry"},"x100thMm":{"type":"integer","minimum":0,"maximum":1000000},"y100thMm":{"type":"integer","minimum":0,"maximum":1000000},"width100thMm":{"type":"integer","minimum":1,"maximum":1000000},"height100thMm":{"type":"integer","minimum":1,"maximum":1000000}},"required":["kind","x100thMm","y100thMm","width100thMm","height100thMm"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"shape-fill"},"rgb":{"type":"string","pattern":"^#[0-9a-f]{6}$"}},"required":["kind","rgb"],"additionalProperties":false}]}},"required":["scopeId","operationId"],"additionalProperties":false,"oneOf":[{"type":"object","properties":{"scopeId":{"type":"string","pattern":"^[a-f0-9]{48}$"},"operationId":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$"},"parts":{"type":"array","maxItems":256,"items":{"oneOf":[{"type":"object","properties":{"kind":{"type":"string","enum":["literal"]},"text":{"type":"string","maxLength":65536}},"required":["kind","text"],"additionalProperties":false},{"type":"object","properties":{"kind":{"type":"string","enum":["protected"]},"protectedRef":{"type":"string","pattern":"^protected_[a-f0-9]{48}$"}},"required":["kind","protectedRef"],"additionalProperties":false}]}}},"required":["scopeId","operationId","parts"],"additionalProperties":false},{"type":"object","properties":{"scopeId":{"type":"string","pattern":"^[a-f0-9]{48}$"},"operationId":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$"},"workbook":{"oneOf":[{"type":"object","properties":{"kind":{"const":"number"},"value":{"type":"number"}},"required":["kind","value"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"formula"},"formula":{"type":"string","minLength":1,"maxLength":1024}},"required":["kind","formula"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"range"},"cells":{"type":"array","minItems":1,"maxItems":256,"items":{"oneOf":[{"type":"object","properties":{"rowOffset":{"type":"integer","minimum":0,"maximum":255},"columnOffset":{"type":"integer","minimum":0,"maximum":255},"type":{"const":"number"},"value":{"type":"number"}},"required":["rowOffset","columnOffset","type","value"],"additionalProperties":false},{"type":"object","properties":{"rowOffset":{"type":"integer","minimum":0,"maximum":255},"columnOffset":{"type":"integer","minimum":0,"maximum":255},"type":{"const":"formula"},"formula":{"type":"string","minLength":1,"maxLength":1024}},"required":["rowOffset","columnOffset","type","formula"],"additionalProperties":false},{"type":"object","properties":{"rowOffset":{"type":"integer","minimum":0,"maximum":255},"columnOffset":{"type":"integer","minimum":0,"maximum":255},"type":{"const":"text"},"text":{"type":"string","maxLength":4096}},"required":["rowOffset","columnOffset","type","text"],"additionalProperties":false}]}}},"required":["kind","cells"],"additionalProperties":false}]}},"required":["scopeId","operationId","workbook"],"additionalProperties":false},{"type":"object","properties":{"scopeId":{"type":"string","pattern":"^[a-f0-9]{48}$"},"operationId":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$"},"canvas":{"type":"array","minItems":1,"maxItems":128,"items":{"oneOf":[{"type":"object","properties":{"kind":{"const":"set-node-layout"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"layout":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000},"width":{"type":"number","exclusiveMinimum":0,"maximum":1000000},"height":{"type":"number","exclusiveMinimum":0,"maximum":1000000}},"required":["x","y","width","height"],"additionalProperties":false}},"required":["kind","id","layout"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"set-edge-route"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"points":{"type":"array","maxItems":32,"items":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000}},"required":["x","y"],"additionalProperties":false},"anyOf":[{"type":"array","items":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000}},"required":["x","y"],"additionalProperties":false},"maxItems":0},{"type":"array","items":{"type":"object","properties":{"x":{"type":"number","minimum":-1000000,"maximum":1000000},"y":{"type":"number","minimum":-1000000,"maximum":1000000}},"required":["x","y"],"additionalProperties":false},"minItems":2,"maxItems":32}]}},"required":["kind","id","points"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"set-display-label"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"target":{"enum":["node","edge"]},"displayLabel":{"type":"string","maxLength":1024}},"required":["kind","id","target","displayLabel"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"set-style"},"id":{"type":"string","pattern":"^canvas_[a-f]{48}$"},"target":{"enum":["node","edge"]},"style":{"type":"object","properties":{"fill":{"type":"string","pattern":"^#[0-9A-Fa-f]{6}$"},"stroke":{"type":"string","pattern":"^#[0-9A-Fa-f]{6}$"},"strokeWidth":{"type":"integer","minimum":1,"maximum":16},"dash":{"type":"string","enum":["solid","dashed"]},"shape":{"type":"string","enum":["rectangle","rounded","ellipse","line"]}},"required":["fill","stroke","strokeWidth","dash","shape"],"additionalProperties":false}},"required":["kind","id","target","style"],"additionalProperties":false}]}}},"required":["scopeId","operationId","canvas"],"additionalProperties":false},{"type":"object","properties":{"scopeId":{"type":"string","pattern":"^[a-f0-9]{48}$"},"operationId":{"type":"string","pattern":"^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$"},"presentation":{"oneOf":[{"type":"object","properties":{"kind":{"const":"shape-geometry"},"x100thMm":{"type":"integer","minimum":0,"maximum":1000000},"y100thMm":{"type":"integer","minimum":0,"maximum":1000000},"width100thMm":{"type":"integer","minimum":1,"maximum":1000000},"height100thMm":{"type":"integer","minimum":1,"maximum":1000000}},"required":["kind","x100thMm","y100thMm","width100thMm","height100thMm"],"additionalProperties":false},{"type":"object","properties":{"kind":{"const":"shape-fill"},"rgb":{"type":"string","pattern":"^#[0-9a-f]{6}$"}},"required":["kind","rgb"],"additionalProperties":false}]}},"required":["scopeId","operationId","presentation"],"additionalProperties":false}]}`)},
		}...)
	}
	if input.WebFetch {
		tools = append(tools, domainmodel.ToolSchema{
			Name:        "web_fetch",
			Description: "Fetch an allowed HTTP or HTTPS URL and return extracted text with source metadata. Uses Analytix web access settings, network proxy diagnostics, SSRF guards, and bounded body reads.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"Absolute URL beginning with http:// or https://"},"max_bytes":{"type":"number","description":"Maximum response bytes to read before extraction"},"timeout_ms":{"type":"number","description":"Request timeout in milliseconds, capped by the runtime"}},"required":["url"],"additionalProperties":false}`),
		})
	}
	MarkToolSource(tools, "builtin")
	return tools
}

func MarkToolSource(tools []domainmodel.ToolSchema, source string) {
	for index := range tools {
		if tools[index].Source == "" {
			tools[index].Source = source
		}
	}
}

func FilterToolSchemas(tools []domainmodel.ToolSchema, toolScope []string) []domainmodel.ToolSchema {
	if len(toolScope) == 0 {
		return tools
	}
	allowed := map[string]bool{}
	for _, name := range toolScope {
		name = strings.TrimSpace(name)
		if name != "" {
			allowed[name] = true
		}
	}
	filtered := make([]domainmodel.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		if allowed[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}
