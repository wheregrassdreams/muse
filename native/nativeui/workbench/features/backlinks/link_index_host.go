package nativeui


func (a *App) queueLinkIndexSnapshot() {
	if a.vault == nil || a.documentPath() == "" || !doctypes.IsMarkdownPath(a.documentPath()) {
		a.activeLinkIndex.Clear()
		return
	}
	a.activeLinkIndex.Schedule(a.documentPath(), a.diskSource(), time.Now())
}

func (a *App) linkIndexTick(c *ui.Context) {
	if a.vault == nil {
		return
	}
	ticket, remaining, ready := a.activeLinkIndex.Take(time.Now())
	if remaining > 0 {
		c.After(remaining)
		return
	}
	if !ready {
		return
	}
	session, path, source := a.vault, ticket.Path, ticket.Source
	go func() {
		if err := session.ApplyActiveDocument(path, source); err != nil {
			return
		}
		a.onMain(func() {
			if a.vault != session || a.documentPath() != path || a.diskSource() != source || !a.activeLinkIndex.IsCurrent(ticket) {
				return
			}
			a.vaultSnapshot = session.Snapshot()
			if a.noteEditor() != nil {
				a.configureActiveNote(0)
			}
			a.invalidate()
		})
	}()
}

func (a *App) indexedLink(target string) bool {
	if a.vault == nil {
		return false
	}
	root := a.vaultSnapshot.Root
	current, err := filepath.Rel(root, a.documentPath())
	if err != nil {
		return false
	}
	if location, ok := a.vault.ResolveLink(filepath.ToSlash(current), target); ok {
		a.navigateIndexed(location.Path, location.Offset)
		return true
	}
	return false
}

func (a *App) openLinks() {
	a.enableDocumentServices()
	session := a.vault
	if session == nil || !a.vaultSnapshot.Ready {
		a.pendingLinks = true
		return
	}
	if a.windows == nil {
		return
	}
	if a.windows.Contains(a.backlinksHandle) {
		_ = a.windows.Focus(a.backlinksHandle)
		return
	}
	a.backlinksHandle = window.Handle{}
	path := a.documentPath()
	if a.imageDocumentOpen() && a.imageDocumentPath() != "" {
		path = a.imageDocumentPath()
	}
	rel, err := filepath.Rel(a.vaultSnapshot.Root, path)
	if err != nil {
		return
	}
	if err := session.OpenLinks(filepath.ToSlash(rel)); err != nil {
		a.notice(locales.VaultIndexBuildingShort())
		return
	}
	rel = filepath.ToSlash(rel)
	content := backlinks.NewContent(backlinks.Ports{
		Data: func() backlinks.Model {
			if a.vault != session {
				return backlinks.Model{}
			}
			snapshot := session.Snapshot()
			return backlinks.Model{
				Ready:    snapshot.Ready,
				Busy:     snapshot.LinksBusy,
				Incoming: session.BacklinkSources(rel),
				Outgoing: snapshot.Outgoing,
			}
		},
		OpenSource: func(path string, offset int) {
			if a.vault == session {
				a.navigateIndexed(path, offset)
			}
		},
		OpenTarget: func(target string) {
			if a.vault == session {
				a.indexedLink(target)
			}
		},
		OnClose: func() {
			_ = session.CloseLinks()
			if a.vault == session {
				a.vaultSnapshot = session.Snapshot()
			}
			a.backlinksHandle = window.Handle{}
			a.invalidate()
		},
	})
	handle, err := a.windows.Open(content, window.Placement{Host: "main", Layer: window.ModalOverlay})
	if err != nil {
		_ = session.CloseLinks()
		return
	}
	a.vaultSnapshot = session.Snapshot()
	a.backlinksHandle = handle
	a.invalidate()
}

func (a *App) closeLinks() {
	if a.windows != nil && a.windows.Contains(a.backlinksHandle) {
		_ = a.windows.Close(a.backlinksHandle)
		return
	}
	if a.vault != nil {
		_ = a.vault.CloseLinks()
		a.vaultSnapshot = a.vault.Snapshot()
	}
}
