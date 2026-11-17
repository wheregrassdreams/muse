package workspace

// DocumentHistory is transient navigation history for the active document
// workspace. A new Push truncates the forward branch, as browser history does.
type DocumentHistory struct {
	paths []string
	index int
}

// NavigationTarget is a validated candidate movement. Commit rechecks it so
// an asynchronous dirty-document confirmation cannot apply a stale target.
type NavigationTarget struct {
	Path  string
	index int
}

func NewDocumentHistory() *DocumentHistory { return &DocumentHistory{index: -1} }

func (h *DocumentHistory) Push(path string) {
	if h == nil || path == "" {
		return
	}
	if h.index >= 0 && h.index < len(h.paths) && h.paths[h.index] == path {
		return
	}
	if h.index+1 < len(h.paths) {
		h.paths = h.paths[:h.index+1]
	}
	h.paths = append(h.paths, path)
	h.index = len(h.paths) - 1
}

func (h *DocumentHistory) Plan(delta int) (NavigationTarget, bool) {
	if h == nil || delta == 0 {
		return NavigationTarget{}, false
	}
	index := h.index + delta
	if index < 0 || index >= len(h.paths) {
		return NavigationTarget{}, false
	}
	return NavigationTarget{Path: h.paths[index], index: index}, true
}

func (h *DocumentHistory) CanMove(delta int) bool {
	_, ok := h.Plan(delta)
	return ok
}

func (h *DocumentHistory) Commit(target NavigationTarget) bool {
	if h == nil || target.index < 0 || target.index >= len(h.paths) || h.paths[target.index] != target.Path {
		return false
	}
	h.index = target.index
	return true
}

func (h *DocumentHistory) Current() (string, bool) {
	if h == nil || h.index < 0 || h.index >= len(h.paths) {
		return "", false
	}
	return h.paths[h.index], true
}

// Paths returns a detached snapshot for diagnostics and presentation tests.
func (h *DocumentHistory) Paths() []string {
	if h == nil {
		return nil
	}
	return append([]string(nil), h.paths...)
}
