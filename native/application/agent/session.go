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
}

type StreamResult struct {
	Aborted bool
	Err     error
}

type StreamClient interface {
	Stream(Request, func(string)) StreamResult
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
	messages  []Message
	busy      bool
	phase     string
	lastError string
	revision  uint64
	requestID uint64
	approvals map[string]approval
	closed    bool
}

func NewSession(settings config.AssistantSettings, client StreamClient) *Session {
	return &Session{settings: settings, client: client, approvals: make(map[string]approval)}
}

func (s *Session) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{Revision: s.revision, Messages: append([]Message(nil), s.messages...), Busy: s.busy, Phase: s.phase, Error: s.lastError}
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
	request := Request{Settings: s.settings, Messages: append([]Message(nil), s.messages...)}
	s.revision++
	s.mu.Unlock()

	go s.stream(id, request)
	return nil
}

func (s *Session) stream(id uint64, request Request) {
	result := s.client.Stream(request, func(delta string) { s.appendDelta(id, delta) })
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

func (s *Session) appendDelta(id uint64, delta string) {
	if delta == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.requestID || s.closed || len(s.messages) == 0 || s.messages[len(s.messages)-1].Role != Assistant {
		return
	}
	last := &s.messages[len(s.messages)-1]
	last.Content += delta
	s.revision++
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
