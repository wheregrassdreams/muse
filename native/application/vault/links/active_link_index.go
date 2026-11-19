package links

import (
	"sync"
	"time"
)

const ActiveLinkIndexDebounce = 120 * time.Millisecond

// ActiveLinkIndex owns the latest-wins debounce token for applying the active
// editor's source to the vault link graph.
type ActiveLinkIndex struct {
	mu         sync.Mutex
	due        time.Time
	generation uint64
	path       string
	source     string
}

type ActiveLinkIndexTicket struct {
	Generation uint64
	Path       string
	Source     string
}

func (c *ActiveLinkIndex) Schedule(path, source string, now time.Time) ActiveLinkIndexTicket {
	if c == nil {
		return ActiveLinkIndexTicket{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.path, c.source = path, source
	c.due = now.Add(ActiveLinkIndexDebounce)
	return c.ticketLocked()
}

// Take returns the current due ticket, or the remaining delay if it is not
// ready. The due time is cleared when a ticket is taken; later edits schedule
// a new generation and invalidate the prior ticket.
func (c *ActiveLinkIndex) Take(now time.Time) (ActiveLinkIndexTicket, time.Duration, bool) {
	if c == nil {
		return ActiveLinkIndexTicket{}, 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.due.IsZero() {
		return ActiveLinkIndexTicket{}, 0, false
	}
	if wait := c.due.Sub(now); wait > 0 {
		return ActiveLinkIndexTicket{}, wait, false
	}
	c.due = time.Time{}
	return c.ticketLocked(), 0, true
}

func (c *ActiveLinkIndex) IsCurrent(ticket ActiveLinkIndexTicket) bool {
	if c == nil || ticket.Generation == 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation == ticket.Generation && c.path == ticket.Path && c.source == ticket.Source
}

func (c *ActiveLinkIndex) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.generation++
	c.due = time.Time{}
	c.path, c.source = "", ""
	c.mu.Unlock()
}

func (c *ActiveLinkIndex) ticketLocked() ActiveLinkIndexTicket {
	return ActiveLinkIndexTicket{Generation: c.generation, Path: c.path, Source: c.source}
}
