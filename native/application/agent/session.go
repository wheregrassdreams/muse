// Package agent owns assistant conversation state and stream lifecycle. The
// desktop panel is a presenter over snapshots; providers and tool execution
// enter through typed capabilities.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"muse/config"
)

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

type Snapshot struct {
	Revision uint64
	Messages []Message
	Busy     bool
	Phase    string
	Error    string
}

type Request struct {
	Settings config.AssistantSettings
	Messages []Message
	Tools    []map[string]any
}

type StreamEvent struct{ Text, Thought, Phase string }

type StreamResult struct {
	Aborted bool
	Err     error
	Tools   []ToolRequest
}

type StreamClient interface {
	Stream(Request, func(StreamEvent)) StreamResult
	Abort()
}

type ToolRequest struct {
	ID, Name, Arguments string
}

type ToolResult struct {
	Content string
	Err     error
}

type ToolExecutor interface {
	Execute(context.Context, ToolRequest) ToolResult
}

type Permission uint8

const (
	AskBeforeChanges Permission = iota + 1
	AllowChanges
	ReadOnly
)

type ToolPolicy interface {
	Enabled(config.AssistantSettings, string) bool
	RequiresApproval(string) bool
	Permission() Permission
	Schemas(config.AssistantSettings) []map[string]any
}

type Capabilities struct {
	Tools  ToolExecutor
	Policy ToolPolicy
}

type Decision uint8

const (
	Reject Decision = iota + 1
	Approve
	AlwaysApprove
)

type approval struct{ decision chan Decision }

var (
	ErrUnavailable = errors.New("assistant session: stream client is unavailable")
	ErrBusy        = errors.New("assistant session: a response is already in progress")
	ErrEmptyPrompt = errors.New("assistant session: prompt is empty")
	ErrNoApproval  = errors.New("assistant session: approval request is no longer active")
)

// Session serializes transcript updates and owns cancellation/approval
// lifecycle. Client callbacks may arrive on background goroutines.
type Session struct {
	mu        sync.RWMutex
	client    StreamClient
	settings  config.AssistantSettings
	tools     ToolExecutor
	policy    ToolPolicy
	messages  []Message
	busy      bool
	phase     string
	lastError string
	revision  uint64
	requestID uint64
	approvals map[string]approval
	closed    bool
}

func NewSession(settings config.AssistantSettings, client StreamClient, capabilities ...Capabilities) *Session {
	s := &Session{settings: settings, client: client, approvals: make(map[string]approval)}
	if len(capabilities) > 0 {
		s.tools, s.policy = capabilities[0].Tools, capabilities[0].Policy
	}
	return s
}

func (s *Session) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{Revision: s.revision, Messages: append([]Message(nil), s.messages...), Busy: s.busy, Phase: s.phase, Error: s.lastError}
}

func (s *Session) RestoreMessages(messages []Message) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.busy && !s.closed {
		s.messages = cloneMessages(messages)
		s.revision++
	}
	s.mu.Unlock()
}

func cloneMessages(messages []Message) []Message {
	copyMessages := make([]Message, len(messages))
	for i, message := range messages {
		copyMessages[i] = message
		copyMessages[i].Parts = append([]Part(nil), message.Parts...)
	}
	return copyMessages
}

func (s *Session) SetSettings(settings config.AssistantSettings) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.settings = settings
	s.revision++
	s.mu.Unlock()
}

func (s *Session) Send(prompt string) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ErrEmptyPrompt
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrUnavailable
	}
	if s.busy {
		s.mu.Unlock()
		return ErrBusy
	}
	s.messages = append(s.messages, Message{Role: User, Content: prompt}, Message{Role: Assistant})
	s.busy, s.phase, s.lastError = true, "thinking", ""
	s.requestID++
	id := s.requestID
	request := s.requestLocked()
	s.revision++
	s.mu.Unlock()

	go s.stream(id, request)
	return nil
}

func (s *Session) stream(id uint64, request Request) {
	var result StreamResult
	for turn := 0; turn < 8; turn++ {
		result = s.client.Stream(request, func(event StreamEvent) { s.applyEvent(id, event) })
		if result.Err != nil || result.Aborted || len(result.Tools) == 0 {
			break
		}
		for _, call := range result.Tools {
			s.appendTool(id, ToolPartState{ID: call.ID, Name: call.Name, Arguments: call.Arguments, Status: PartRunning})
			outcome := s.ExecuteTool(context.Background(), call)
			status := PartSucceeded
			errorText := ""
			if outcome.Err != nil {
				status, errorText = PartFailed, outcome.Err.Error()
			}
			s.finishTool(id, call.ID, ToolPartState{ID: call.ID, Name: call.Name, Arguments: call.Arguments, Result: outcome.Content, Error: errorText, Status: status})
		}
		s.mu.Lock()
		if id != s.requestID || s.closed {
			s.mu.Unlock()
			return
		}
		request = s.requestLocked()
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.requestID || s.closed {
		return
	}
	s.busy, s.phase = false, ""
	if result.Err != nil && !result.Aborted {
		s.lastError = result.Err.Error()
	}
	s.revision++
}

func (s *Session) requestLocked() Request {
	request := Request{Settings: s.settings, Messages: append([]Message(nil), s.messages...)}
	if s.policy != nil {
		request.Tools = s.policy.Schemas(s.settings)
	}
	return request
}

func (s *Session) appendTool(id uint64, tool ToolPartState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.requestID || s.closed || len(s.messages) == 0 {
		return
	}
	last := &s.messages[len(s.messages)-1]
	last.Parts = append(last.Parts, Part{Kind: ToolPart, Tool: tool})
	s.revision++
}

func (s *Session) finishTool(id uint64, toolID string, value ToolPartState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.requestID || s.closed || len(s.messages) == 0 {
		return
	}
	last := &s.messages[len(s.messages)-1]
	for i := len(last.Parts) - 1; i >= 0; i-- {
		if last.Parts[i].Kind == ToolPart && last.Parts[i].Tool.ID == toolID {
			last.Parts[i].Tool = value
			break
		}
	}
	s.revision++
}

func (s *Session) applyEvent(id uint64, event StreamEvent) {
	if event.Text == "" && event.Thought == "" && event.Phase == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.requestID || s.closed || len(s.messages) == 0 || s.messages[len(s.messages)-1].Role != Assistant {
		return
	}
	last := &s.messages[len(s.messages)-1]
	if event.Phase != "" {
		s.phase = event.Phase
	}
	if event.Thought != "" {
		appendPartDelta(last, ThoughtPart, event.Thought)
	}
	if event.Text != "" {
		appendPartDelta(last, TextPart, event.Text)
		last.Content += event.Text
	}
	s.revision++
}

func appendPartDelta(message *Message, kind PartKind, value string) {
	if n := len(message.Parts); n > 0 && message.Parts[n-1].Kind == kind {
		message.Parts[n-1].Text += value
		return
	}
	message.Parts = append(message.Parts, Part{Kind: kind, Text: value})
}

func (s *Session) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.busy {
		s.mu.Unlock()
		return
	}
	s.requestID++
	s.busy, s.phase = false, ""
	for key, pending := range s.approvals {
		delete(s.approvals, key)
		select {
		case pending.decision <- Reject:
		default:
		}
	}
	s.revision++
	client := s.client
	s.mu.Unlock()
	if client != nil {
		client.Abort()
	}
}

func (s *Session) Clear() error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return ErrBusy
	}
	s.messages = nil
	s.lastError = ""
	s.revision++
	return nil
}

func (s *Session) TogglePart(messageIndex, partIndex int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if messageIndex < 0 || messageIndex >= len(s.messages) || partIndex < 0 || partIndex >= len(s.messages[messageIndex].Parts) {
		return
	}
	s.messages[messageIndex].Parts[partIndex].Expanded = !s.messages[messageIndex].Parts[partIndex].Expanded
	s.revision++
}

func (s *Session) RequestApproval(ctx context.Context, request ToolRequest) (Decision, error) {
	if s == nil || request.ID == "" {
		return Reject, ErrNoApproval
	}
	entry := approval{decision: make(chan Decision, 1)}
	s.mu.Lock()
	if s.closed || !s.busy {
		s.mu.Unlock()
		return Reject, ErrNoApproval
	}
	if _, exists := s.approvals[request.ID]; exists {
		s.mu.Unlock()
		return Reject, fmt.Errorf("assistant session: duplicate approval id %q", request.ID)
	}
	s.approvals[request.ID] = entry
	s.phase = "waiting for approval"
	s.revision++
	s.mu.Unlock()
	select {
	case decision := <-entry.decision:
		s.mu.Lock()
		delete(s.approvals, request.ID)
		s.revision++
		s.mu.Unlock()
		return decision, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.approvals, request.ID)
		s.revision++
		s.mu.Unlock()
		return Reject, ctx.Err()
	}
}

func (s *Session) DecideApproval(id string, decision Decision) error {
	if s == nil {
		return ErrNoApproval
	}
	s.mu.RLock()
	entry, ok := s.approvals[id]
	s.mu.RUnlock()
	if !ok {
		return ErrNoApproval
	}
	select {
	case entry.decision <- decision:
		return nil
	default:
		return ErrNoApproval
	}
}

// ExecuteTool applies the injected capability and permission policy before
// delegating a typed operation. It never interprets UI state or raw JSON.
func (s *Session) ExecuteTool(ctx context.Context, request ToolRequest) ToolResult {
	if s == nil || s.tools == nil || s.policy == nil {
		return ToolResult{Err: ErrUnavailable}
	}
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	if !s.policy.Enabled(settings, request.Name) {
		return ToolResult{Err: fmt.Errorf("assistant tool %q is disabled", request.Name)}
	}
	if s.policy.RequiresApproval(request.Name) {
		switch s.policy.Permission() {
		case ReadOnly:
			return ToolResult{Err: errors.New("assistant session is read-only")}
		case AskBeforeChanges:
			decision, err := s.RequestApproval(ctx, request)
			if err != nil {
				return ToolResult{Err: err}
			}
			if decision == Reject {
				return ToolResult{Err: errors.New("assistant operation was rejected")}
			}
		}
	}
	return s.tools.Execute(ctx, request)
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	s.requestID++
	s.busy = false
	for id, entry := range s.approvals {
		delete(s.approvals, id)
		select {
		case entry.decision <- Reject:
		default:
		}
	}
	client := s.client
	s.mu.Unlock()
	if client != nil {
		client.Abort()
	}
}

// Wait is intended for deterministic lifecycle tests and non-UI callers.
func (s *Session) Wait(ctx context.Context) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if !s.Snapshot().Busy {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
