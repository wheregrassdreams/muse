package workspace

// WorkspaceRestored is published after a persisted workspace snapshot has
// been loaded and validated. The application layer decides how to present it.
type WorkspaceRestored struct{ State State }
