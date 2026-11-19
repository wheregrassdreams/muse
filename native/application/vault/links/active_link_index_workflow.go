package links

import (
	"strings"
	"time"

	vaultapp "muse/application/vault"
	doctypes "muse/document/types"
)

// ActiveLinkGraphPort is the session capability consumed when an active
// Markdown source is applied to the vault link graph.
type ActiveLinkGraphPort interface {
	ApplyActiveDocument(path, source string) error
	Snapshot() vaultapp.Snapshot
}

// ActiveLinkIndexResult is the detached result of applying one debounce
// ticket. Consumers must pass it through Accept before publishing the new
// snapshot to views.
type ActiveLinkIndexResult struct {
	Ticket   ActiveLinkIndexTicket
	Snapshot vaultapp.Snapshot
	Err      error
}

// ScheduleDocument applies active-document policy before scheduling graph
// work. Only a named Markdown document contributes an active source overlay;
// other documents invalidate any pending ticket.
func (c *ActiveLinkIndex) ScheduleDocument(path, source string, now time.Time) (ActiveLinkIndexTicket, bool) {
	if c == nil {
		return ActiveLinkIndexTicket{}, false
	}
	if strings.TrimSpace(path) == "" || !doctypes.IsMarkdownPath(path) {
		c.Clear()
		return ActiveLinkIndexTicket{}, false
	}
	return c.Schedule(path, source, now), true
}

// Apply applies a ticket only while it is still current. The workflow lock
// serializes a graph update against a new Schedule/Clear, so a stale ticket
// cannot overwrite a newer active document between validation and update.
func (c *ActiveLinkIndex) Apply(ticket ActiveLinkIndexTicket, session ActiveLinkGraphPort) ActiveLinkIndexResult {
	result := ActiveLinkIndexResult{Ticket: ticket}
	if c == nil || ticket.Generation == 0 {
		return result
	}
	if session == nil {
		result.Err = vaultapp.ErrSessionClosed
		return result
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.currentLocked(ticket) {
		return result
	}
	if err := session.ApplyActiveDocument(ticket.Path, ticket.Source); err != nil {
		result.Err = err
		return result
	}
	result.Snapshot = session.Snapshot()
	return result
}

// Accept reports whether a completed update still describes the active
// editor. Call Clear whenever the active vault session is replaced; that
// advances the generation and rejects results from the previous session.
func (c *ActiveLinkIndex) Accept(result ActiveLinkIndexResult, activePath, activeSource string) (vaultapp.Snapshot, bool) {
	if c == nil || result.Err != nil || result.Ticket.Generation == 0 {
		return vaultapp.Snapshot{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.currentLocked(result.Ticket) || activePath != result.Ticket.Path || activeSource != result.Ticket.Source {
		return vaultapp.Snapshot{}, false
	}
	return result.Snapshot, true
}

func (c *ActiveLinkIndex) currentLocked(ticket ActiveLinkIndexTicket) bool {
	return c.generation == ticket.Generation && c.path == ticket.Path && c.source == ticket.Source
}
