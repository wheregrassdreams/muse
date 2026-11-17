package agent

import "muse/config"

type ToolGroup string

const (
	BrowseTools ToolGroup = "browse"
	EditTools   ToolGroup = "edit"
	ManageTools ToolGroup = "manage"
)

func GroupForTool(name string) ToolGroup {
	switch name {
	case "list_documents", "read_document", "search_document", "read_outline", "workspace_tree", "document_stats", "open_document":
		return BrowseTools
	case "edit_document", "insert_into_document":
		return EditTools
	case "write_document", "rename_document", "delete_document", "create_folder":
		return ManageTools
	default:
		return ""
	}
}

func ToolEnabled(settings config.AssistantSettings, name string) bool {
	switch GroupForTool(name) {
	case BrowseTools:
		return settings.Tools.Browse
	case EditTools:
		return settings.Tools.Edit
	case ManageTools:
		return settings.Tools.Manage
	default:
		return true
	}
}

func ToolRequiresApproval(name string) bool {
	switch name {
	case "edit_document", "write_document", "insert_into_document", "rename_document", "delete_document", "create_folder":
		return true
	default:
		return false
	}
}

// DefaultToolPolicy owns static tool grouping, enablement, and approval rules.
// Per-conversation permission is mutable session state and belongs to Session.
type DefaultToolPolicy struct{}

func (DefaultToolPolicy) Enabled(settings config.AssistantSettings, name string) bool {
	return ToolEnabled(settings, name)
}

func (DefaultToolPolicy) RequiresApproval(name string) bool {
	return ToolRequiresApproval(name)
}

func (DefaultToolPolicy) Schemas(settings config.AssistantSettings) []map[string]any {
	return ToolSchemas(settings)
}

var _ ToolPolicy = DefaultToolPolicy{}
