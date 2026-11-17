// Package agent defines provider-neutral conversation and tool value types.
// Session lifecycle and permission policy belong to application/agent; provider
// protocols belong to platform adapters.
package agent

import "time"

type Role uint8

const (
	User Role = iota + 1
	Assistant
)

type Message struct {
	Role    Role
	Content string
	Parts   []Part
}

type PartKind uint8

const (
	TextPart PartKind = iota + 1
	ThoughtPart
	ToolPart
)

type PartStatus uint8

const (
	PartRunning PartStatus = iota + 1
	PartApproval
	PartSucceeded
	PartFailed
)

type Part struct {
	Kind     PartKind
	Text     string
	Duration time.Duration
	Expanded bool
	Tool     ToolPartState
}

type ToolPartState struct {
	ID, Name, Arguments, Result, Error string
	Status                             PartStatus
}

type ToolRequest struct {
	ID, Name, Arguments string
}

type ToolResult struct {
	Content string
	Err     error
}
