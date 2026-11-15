package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ToolInput is the validated, typed argument surface for assistant tools.
type ToolInput struct {
	Path     string `json:"path,omitempty"`
	NewPath  string `json:"new_path,omitempty"`
	Query    string `json:"query,omitempty"`
	Content  string `json:"content,omitempty"`
	OldText  string `json:"old_text,omitempty"`
	NewText  string `json:"new_text,omitempty"`
	Text     string `json:"text,omitempty"`
	Position string `json:"position,omitempty"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Depth    int    `json:"depth,omitempty"`
	IsRegex  bool   `json:"is_regex,omitempty"`
}

// Operations is the capability port implemented by the desktop composition
// adapter. Methods correspond to business operations, not UI widgets.
type Operations interface {
	ListDocuments(context.Context, ToolInput) (any, error)
	ReadDocument(context.Context, ToolInput) (any, error)
	SearchDocuments(context.Context, ToolInput) (any, error)
	ReadOutline(context.Context, ToolInput) (any, error)
	WorkspaceTree(context.Context, ToolInput) (any, error)
	DocumentStats(context.Context, ToolInput) (any, error)
	OpenDocument(context.Context, ToolInput) (any, error)
	EditDocument(context.Context, ToolInput) (any, error)
	InsertIntoDocument(context.Context, ToolInput) (any, error)
	WriteDocument(context.Context, ToolInput) (any, error)
	RenameDocument(context.Context, ToolInput) (any, error)
	DeleteDocument(context.Context, ToolInput) (any, error)
	CreateFolder(context.Context, ToolInput) (any, error)
}

type Executor struct{ Operations Operations }

func (e Executor) Execute(ctx context.Context, request ToolRequest) ToolResult {
	if e.Operations == nil {
		return ToolResult{Err: errors.New("assistant operations are unavailable")}
	}
	var input ToolInput
	decoder := json.NewDecoder(strings.NewReader(request.Arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return ToolResult{Err: fmt.Errorf("invalid %s arguments: %w", request.Name, err)}
	}
	var value any
	var err error
	switch request.Name {
	case "list_documents":
		value, err = e.Operations.ListDocuments(ctx, input)
	case "read_document":
		value, err = e.Operations.ReadDocument(ctx, input)
	case "search_document":
		value, err = e.Operations.SearchDocuments(ctx, input)
	case "read_outline":
		value, err = e.Operations.ReadOutline(ctx, input)
	case "workspace_tree":
		value, err = e.Operations.WorkspaceTree(ctx, input)
	case "document_stats":
		value, err = e.Operations.DocumentStats(ctx, input)
	case "open_document":
		value, err = e.Operations.OpenDocument(ctx, input)
	case "edit_document":
		value, err = e.Operations.EditDocument(ctx, input)
	case "insert_into_document":
		value, err = e.Operations.InsertIntoDocument(ctx, input)
	case "write_document":
		value, err = e.Operations.WriteDocument(ctx, input)
	case "rename_document":
		value, err = e.Operations.RenameDocument(ctx, input)
	case "delete_document":
		value, err = e.Operations.DeleteDocument(ctx, input)
	case "create_folder":
		value, err = e.Operations.CreateFolder(ctx, input)
	default:
		return ToolResult{Err: fmt.Errorf("unknown assistant tool %q", request.Name)}
	}
	if err != nil {
		return ToolResult{Err: err}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ToolResult{Err: err}
	}
	return ToolResult{Content: string(encoded)}
}
