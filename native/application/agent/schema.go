package agent

import "muse/config"

// ToolSchemas builds the provider function definitions from the same policy
// groups used for execution, so disabled capabilities are never advertised.
func ToolSchemas(settings config.AssistantSettings) []map[string]any {
	type field struct {
		kind     string
		required bool
	}
	defs := []struct {
		name   string
		group  ToolGroup
		fields map[string]field
	}{
		{"list_documents", BrowseTools, map[string]field{"path": {"string", false}}},
		{"read_document", BrowseTools, map[string]field{"path": {"string", true}, "offset": {"integer", false}, "limit": {"integer", false}}},
		{"search_document", BrowseTools, map[string]field{"query": {"string", true}, "path": {"string", false}, "is_regex": {"boolean", false}}},
		{"read_outline", BrowseTools, map[string]field{"path": {"string", true}}},
		{"workspace_tree", BrowseTools, map[string]field{"path": {"string", false}, "depth": {"integer", false}}},
		{"document_stats", BrowseTools, map[string]field{"path": {"string", true}}},
		{"open_document", BrowseTools, map[string]field{"path": {"string", true}}},
		{"edit_document", EditTools, map[string]field{"path": {"string", true}, "old_text": {"string", true}, "new_text": {"string", true}}},
		{"insert_into_document", EditTools, map[string]field{"text": {"string", true}, "position": {"string", true}}},
		{"write_document", ManageTools, map[string]field{"path": {"string", true}, "content": {"string", true}}},
		{"rename_document", ManageTools, map[string]field{"path": {"string", true}, "new_path": {"string", true}}},
		{"delete_document", ManageTools, map[string]field{"path": {"string", true}}},
		{"create_folder", ManageTools, map[string]field{"path": {"string", true}}},
	}
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		if !ToolEnabled(settings, d.name) {
			continue
		}
		props := map[string]any{}
		required := []string{}
		for name, f := range d.fields {
			props[name] = map[string]any{"type": f.kind}
			if f.required {
				required = append(required, name)
			}
		}
		out = append(out, map[string]any{"type": "function", "function": map[string]any{"name": d.name, "description": d.name, "parameters": map[string]any{"type": "object", "properties": props, "required": required}}})
	}
	return out
}
