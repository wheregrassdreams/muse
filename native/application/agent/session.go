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

	conversation "muse/agent"
	"muse/config"
)

type Snapshot struct {
	Revision   uint64
	Messages   []conversation.Message
	Busy       bool
	Phase      string
	Error      string
	Permission Permission
}

type Request struct {
	Settings config.AssistantSettings
	Messages []conversation.Message
	Tools    []map[string]any
}

type StreamEvent struct{ Text, Thought, Phase string }

type StreamResult struct {
	Aborted bool
	Err     error
	Tools   []conversation.ToolRequest
}

type StreamClient interface {
	Stream(Request, func(StreamEvent)) StreamResult
	Abort()
}

type ToolExecutor interface {
	Execute(context.Context, conversation.ToolRequest) conversation.ToolResult
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
	ErrUnavailable       = errors.New("assistant session: stream client is unavailable")
	ErrBusy              = errors.New("assistant session: a response is already in progress")
	ErrEmptyPrompt       = errors.New("assistant session: prompt is empty")
	ErrNoApproval        = errors.New("assistant session: approval request is no longer active")
	ErrInvalidPermission = errors.New("assistant session: invalid permission")
	ErrInvalidDecision   = errors.New("assistant session: invalid approval decision")
)

// Session serializes transcript updates and owns cancellation/approval
// lifecycle. Client callbacks may arrive on background goroutines.
type Session struct {
	mu         sync.RWMutex
	client     StreamClient
	settings   config.AssistantSettings
	permission Permission
	tools      ToolExecutor
	policy     ToolPolicy
	messages   []conversation.Message
	busy       bool
	phase      string
	lastError  string
	revision   uint64
	requestID  uint64
	approvals  map[string]approval
	closed     bool
}

func NewSession(settings config.AssistantSettings, client StreamClient, capabilities ...Capabilities) *Session {
	s := &Session{settings: settings, permission: AskBeforeChanges, client: client, approvals: make(map[string]approval)}
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
	return Snapshot{Revision: s.revision, Messages: append([]conversation.Message(nil), s.messages...), Busy: s.busy, Phase: s.phase, Error: s.lastError, Permission: s.permission}
}

// Permission returns the current conversation-scoped tool permission.
func (s *Session) Permission() Permission {
	if s == nil {
		return ReadOnly
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.permission == 0 {
		return AskBeforeChanges
	}
	return s.permission
}

// SetPermission changes the permission for this conversation. Approval
// decisions use DecideApproval so changing permission and releasing the
// corresponding tool call can happen atomically.
func (s *Session) SetPermission(permission Permission) error {
	if s == nil {
		return ErrUnavailable
	}
	if !validPermission(permission) {
		return ErrInvalidPermission
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrUnavailable
	}
	if s.permission != permission {
		s.permission = permission
		s.revision++
	}
	return nil
}

func validPermission(permission Permission) bool {
	return permission == AskBeforeChanges || permission == AllowChanges || permission == ReadOnly
}

func (s *Session) RestoreMessages(messages []conversation.Message) {
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

func cloneMessages(messages []conversation.Message) []conversation.Message {
	copyMessages := make([]conversation.Message, len(messages))
	for i, message := range messages {
		copyMessages[i] = message
		copyMessages[i].Parts = append([]conversation.Part(nil), message.Parts...)
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

// SetClient injects or replaces the provider while the session is idle. The
// client is stable for the lifetime of an active stream so Abort always targets
// the request that Send started.
func (s *Session) SetClient(client StreamClient) error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrUnavailable
	}
	if s.busy {
		return ErrBusy
	}
	s.client = client
	s.revision++
	return nil
}

func (s *Session) Send(prompt string) error {
	if s == nil {
		return ErrUnavailable
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ErrEmptyPrompt
	}
	s.mu.Lock()
	if s.closed || s.client == nil {
		s.mu.Unlock()
		return ErrUnavailable
	}
	if s.busy {
		s.mu.Unlock()
		return ErrBusy
	}
	s.messages = append(s.messages, conversation.Message{Role: conversation.User, Content: prompt}, conversation.Message{Role: conversation.Assistant})
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
			s.appendTool(id, conversation.ToolPartState{ID: call.ID, Name: call.Name, Arguments: call.Arguments, Status: conversation.PartRunning})
			outcome := s.ExecuteTool(context.Background(), call)
			status := conversation.PartSucceeded
			errorText := ""
			if outcome.Err != nil {
				status, errorText = conversation.PartFailed, outcome.Err.Error()
			}
			s.finishTool(id, call.ID, conversation.ToolPartState{ID: call.ID, Name: call.Name, Arguments: call.Arguments, Result: outcome.Content, Error: errorText, Status: status})
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
	request := Request{Settings: s.settings, Messages: append([]conversation.Message(nil), s.messages...)}
	if s.policy != nil {
		request.Tools = s.policy.Schemas(s.settings)
	}
	return request
}

func (s *Session) appendTool(id uint64, tool conversation.ToolPartState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.requestID || s.closed || len(s.messages) == 0 {
		return
	}
	last := &s.messages[len(s.messages)-1]
	last.Parts = append(last.Parts, conversation.Part{Kind: conversation.ToolPart, Tool: tool})
	s.revision++
}

func (s *Session) finishTool(id uint64, toolID string, value conversation.ToolPartState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.requestID || s.closed || len(s.messages) == 0 {
		return
	}
	last := &s.messages[len(s.messages)-1]
	for i := len(last.Parts) - 1; i >= 0; i-- {
		if last.Parts[i].Kind == conversation.ToolPart && last.Parts[i].Tool.ID == toolID {
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
	if id != s.requestID || s.closed || len(s.messages) == 0 || s.messages[len(s.messages)-1].Role != conversation.Assistant {
		return
	}
	last := &s.messages[len(s.messages)-1]
	if event.Phase != "" {
		s.phase = event.Phase
	}
	if event.Thought != "" {
		appendPartDelta(last, conversation.ThoughtPart, event.Thought)
	}
	if event.Text != "" {
		appendPartDelta(last, conversation.TextPart, event.Text)
		last.Content += event.Text
	}
	s.revision++
}

func appendPartDelta(message *conversation.Message, kind conversation.PartKind, value string) {
	if n := len(message.Parts); n > 0 && message.Parts[n-1].Kind == kind {
		message.Parts[n-1].Text += value
		return
	}
	message.Parts = append(message.Parts, conversation.Part{Kind: kind, Text: value})
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

func (s *Session) RequestApproval(ctx context.Context, request conversation.ToolRequest) (Decision, error) {
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
		s.finishApproval(request.ID, entry)
		return decision, nil
	case <-ctx.Done():
		// Serialize the cancellation result with DecideApproval. If a user
		// decision was accepted first, honor it; otherwise remove the request
		// before a later UI action can change conversation permission.
		s.mu.Lock()
		if current, ok := s.approvals[request.ID]; ok && current.decision == entry.decision {
			select {
			case decision := <-entry.decision:
				delete(s.approvals, request.ID)
				s.revision++
				s.mu.Unlock()
				return decision, nil
			default:
				delete(s.approvals, request.ID)
				s.revision++
			}
		}
		s.mu.Unlock()
		return Reject, ctx.Err()
	}
}

func (s *Session) finishApproval(id string, entry approval) {
	s.mu.Lock()
	if current, ok := s.approvals[id]; ok && current.decision == entry.decision {
		delete(s.approvals, id)
		s.revision++
	}
	s.mu.Unlock()
}

func (s *Session) DecideApproval(id string, decision Decision) error {
	if s == nil {
		return ErrNoApproval
	}
	if decision != Reject && decision != Approve && decision != AlwaysApprove {
		return ErrInvalidDecision
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.approvals[id]
	if !ok {
		return ErrNoApproval
	}
	select {
	case entry.decision <- decision:
		if decision == AlwaysApprove && s.permission != AllowChanges {
			s.permission = AllowChanges
			s.revision++
		}
		return nil
	default:
		return ErrNoApproval
	}
}

// ExecuteTool applies the injected capability and permission policy before
// delegating a typed operation. It never interprets UI state or raw JSON.
func (s *Session) ExecuteTool(ctx context.Context, request conversation.ToolRequest) conversation.ToolResult {
	if s == nil || s.tools == nil || s.policy == nil {
		return conversation.ToolResult{Err: ErrUnavailable}
	}
	s.mu.RLock()
	settings, permission := s.settings, s.permission
	s.mu.RUnlock()
	if permission == 0 {
		permission = AskBeforeChanges
	}
	if !s.policy.Enabled(settings, request.Name) {
		return conversation.ToolResult{Err: fmt.Errorf("assistant tool %q is disabled", request.Name)}
	}
	if s.policy.RequiresApproval(request.Name) {
		switch permission {
		case ReadOnly:
			return conversation.ToolResult{Err: errors.New("assistant session is read-only")}
		case AskBeforeChanges:
			decision, err := s.RequestApproval(ctx, request)
			if err != nil {
				return conversation.ToolResult{Err: err}
			}
			if decision == Reject {
				return conversation.ToolResult{Err: errors.New("assistant operation was rejected")}
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
