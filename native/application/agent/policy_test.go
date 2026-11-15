package agent

import (
	"muse/config"
	"testing"
)

func TestToolPolicyGroupsAndApproval(t *testing.T) {
	settings := config.AssistantSettings{Tools: config.AssistantTools{Browse: true, Edit: false, Manage: true}}
	for name, want := range map[string]bool{"read_document": true, "edit_document": false, "write_document": true, "custom_tool": true} {
		if got := ToolEnabled(settings, name); got != want {
			t.Errorf("ToolEnabled(%q)=%v want %v", name, got, want)
		}
	}
	for name, want := range map[string]bool{"read_document": false, "edit_document": true, "create_folder": true, "custom_tool": false} {
		if got := ToolRequiresApproval(name); got != want {
			t.Errorf("ToolRequiresApproval(%q)=%v want %v", name, got, want)
		}
	}
}

func TestToolSchemasRespectCapabilitySettings(t *testing.T) {
	got := ToolSchemas(config.AssistantSettings{Tools: config.AssistantTools{Browse: true}})
	if len(got) != 7 {
		t.Fatalf("schemas=%d want browse-only 7", len(got))
	}
	for _, schema := range got {
		name := schema["function"].(map[string]any)["name"].(string)
		if GroupForTool(name) != BrowseTools {
			t.Fatalf("disabled tool advertised: %s", name)
		}
	}
}
