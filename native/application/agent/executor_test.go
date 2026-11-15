package agent

import (
	"context"
	"testing"
)

type testOperations struct {
	Operations
	got ToolInput
}

func (o *testOperations) ReadDocument(_ context.Context, input ToolInput) (any, error) {
	o.got = input
	return map[string]any{"content": "note"}, nil
}

func TestExecutorValidatesAndDispatchesTypedToolArguments(t *testing.T) {
	operations := &testOperations{}
	executor := Executor{Operations: operations}
	result := executor.Execute(context.Background(), ToolRequest{Name: "read_document", Arguments: `{"path":"notes/a.md","offset":4,"limit":12}`})
	if result.Err != nil || result.Content == "" || operations.got.Path != "notes/a.md" || operations.got.Offset != 4 || operations.got.Limit != 12 {
		t.Fatalf("result=%+v input=%+v", result, operations.got)
	}
	if result := executor.Execute(context.Background(), ToolRequest{Name: "read_document", Arguments: `{"path":1}`}); result.Err == nil {
		t.Fatal("accepted invalid typed arguments")
	}
	if result := executor.Execute(context.Background(), ToolRequest{Name: "unexpected", Arguments: `{}`}); result.Err == nil {
		t.Fatal("accepted unknown tool")
	}
}
