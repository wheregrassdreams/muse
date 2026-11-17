package nativeui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	applicationagent "muse/application/agent"
	applicationconfig "muse/application/config"
	applicationvault "muse/application/vault"
	"muse/config"
	"muse/document"
	"muse/locales"
	mdoc "muse/markdown"
	componenticons "muse/nativeui/components/icons"
	agentui "muse/nativeui/workbench/features/agent"
	markdownui "muse/nativeui/workbench/features/markdown"
	settingsui "muse/nativeui/workbench/features/settings"
)

// newAssistantController is the NativeUI composition root for the agent
// feature. App-specific workspace mutations and editor actions are adapted to
// the feature's narrow typed ports here, while session/tool policy stays in
// application/agent.
func newAssistantController(app *App) *agentui.Controller {
	if app == nil {
		return nil
	}
	session := applicationagent.NewSession(app.ai, app.assistantStream, applicationagent.Capabilities{
		Tools:  applicationagent.Executor{Operations: applicationagent.DynamicWorkspaceOperations{Provider: appAssistantWorkspaceProvider{app: app}}},
		Policy: applicationagent.DefaultToolPolicy{},
	})
	return agentui.NewController(agentui.ControllerOptions{
		Session:  session,
		Settings: app.ai,
		Ports: agentui.Ports{
			Icon: func() *ui.SVG { return componenticons.AI() },
			RenderMarkdown: func(c *ui.Context, source string, width float32) {
				doc := mdoc.Parse(source)
				markdownui.BlocksView(c, doc.Blocks, assistantMarkdownOptions(app, width))
			},
			OpenSettings:      func() { app.settingsPanel.OpenAt(settingsui.AssistantPage) },
			OpenImageDocument: func(path string) { app.openImageDocument(path, true) },
			OpenDocument:      app.requestOpen,
			InsertAtStart:     app.insertAIContentAtStart,
			InsertAtEnd:       app.insertAIContent,
			SaveSettings: func(settings config.AssistantSettings) error {
				if app.configRepository == nil {
					return applicationagent.ErrUnavailable
				}
				return applicationconfig.NewService(app.configRepository).SaveAssistantSettings(settings)
			},
			Provider:  appAssistantModelProvider{app: app},
			RunOnMain: mygo.RunOnMain,
			SetDefaultModel: func(model string) {
				app.onMain(func() {
					if strings.TrimSpace(app.ai.Model) == "" {
						app.ai.Model = model
						app.invalidate()
					}
				})
			},
			Notify:     app.notice,
			Invalidate: app.invalidate,
		},
	})
}

func (a *App) aiView(c *ui.Context) {
	if a.assistant != nil {
		a.assistant.Render(c, a.ai)
	}
}

func (a *App) saveAssistantSettings() error {
	if a == nil || a.assistant == nil {
		return applicationagent.ErrUnavailable
	}
	return a.assistant.SaveSettings(a.ai)
}

func (a *App) saveAITools() {
	if err := a.saveAssistantSettings(); err != nil {
		a.notice(locales.CommonSaveFailed(err.Error()))
	}
	a.invalidate()
}

func (a *App) setAIService(magpie bool) {
	if a == nil || a.assistant == nil {
		return
	}
	updated := a.assistant.SelectProvider(a.ai, magpie)
	if updated == a.ai {
		return
	}
	a.ai = updated
	a.invalidate()
}

func (a *App) insertAIContent(text string) {
	if a == nil || a.noteEditor() == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.noteEditor().FormattingCommands().AppendBlock(text)
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

func (a *App) insertAIContentAtStart(text string) {
	if a == nil || a.noteEditor() == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.noteEditor().FormattingCommands().PrependBlock(text)
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

func (a *App) fetchAIModels() {
	if a != nil && a.assistant != nil {
		a.assistant.FetchModels(a.ai)
	}
}

func assistantMarkdownOptions(app *App, width float32) markdownui.Options {
	if app == nil {
		return markdownui.Options{}
	}
	return markdownui.Options{
		BodyFontSize: 12.5, SoftBreaksAsSpaces: true, ContentWidth: width,
		BaseDir: app.docDir(), AssetRoot: app.root, LocalFiles: app.imageLocalFiles,
		ImageCacheStore: app.imageCacheStore, OnImagePreview: app.openImagePreview,
		Padding: 0, IndentStep: 14,
	}
}

type appAssistantModelProvider struct{ app *App }

func (p appAssistantModelProvider) ListModels(ctx context.Context, credentials applicationagent.ProviderCredentials) (applicationagent.ProviderModelList, error) {
	if p.app == nil || p.app.assistantProvider == nil {
		return applicationagent.ProviderModelList{}, applicationagent.ErrUnavailable
	}
	return p.app.assistantProvider.ListModels(ctx, credentials)
}

type appAssistantWorkspaceProvider struct{ app *App }

func (p appAssistantWorkspaceProvider) Workspace(ctx context.Context) (applicationagent.WorkspaceMutationPort, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.app == nil {
		return nil, applicationagent.ErrUnavailable
	}
	var root, path, source string
	p.app.onMain(func() {
		root, path, source = p.app.root, p.app.documentPath(), p.app.documentSource()
	})
	if root == "" {
		return nil, errors.New("workspace is unavailable")
	}
	if p.app.newAgentWorkspace == nil {
		return nil, errors.New("assistant workspace adapter is not configured")
	}
	workspace, err := p.app.newAgentWorkspace(root, func() (string, string) { return path, source })
	if err != nil {
		return nil, err
	}
	directory, _ := workspace.(applicationagent.WorkspaceDirectoryPort)
	return &appAssistantWorkspace{app: p.app, WorkspacePort: workspace, Directory: directory}, nil
}

type appAssistantWorkspace struct {
	app *App
	applicationagent.WorkspacePort
	Directory applicationagent.WorkspaceDirectoryPort
}

func (w *appAssistantWorkspace) DestinationRevision(path string) (string, error) {
	revision, err := w.app.destinationRevision(path)
	return string(revision), err
}

func (w *appAssistantWorkspace) CurrentDocumentDirty() bool {
	var dirty bool
	w.app.onMain(func() { dirty = w.app.documentDirty() })
	return dirty
}

func (w *appAssistantWorkspace) SaveDocument(path, content, expected string) error {
	return w.app.saveExternalDocument(path, content, document.Revision(expected))
}

func (w *appAssistantWorkspace) ApplyCurrentDocument(content string) error {
	var err error
	w.app.onMain(func() {
		if w.app.documentPath() == "" && w.app.documentSource() == "" {
			err = errors.New("current document is empty")
			return
		}
		w.app.editDoc(content)
	})
	return err
}

func (w *appAssistantWorkspace) MoveDocument(root, source, target string) (string, error) {
	files := applicationvault.NewFileUseCases(root, w.app.fileOperations, w.app.isVaultDirectory)
	mutation, err := files.MoveFileTo(source, target)
	if err != nil {
		return "", err
	}
	if len(mutation.Moves) == 0 {
		return "", applicationvault.ErrFileOperationsUnavailable
	}
	w.app.onMain(func() {
		w.app.clearTreeCache()
		w.app.remapTreePaths(mutation.Moves)
		w.app.addRecent(target, false)
		w.app.invalidate()
	})
	w.refresh(source, target)
	return mutation.Moves[0].Target, nil
}

func (w *appAssistantWorkspace) TrashDocuments(root string, paths []string) error {
	files := applicationvault.NewFileUseCases(root, w.app.fileOperations, w.app.isVaultDirectory)
	mutation, err := files.TrashFiles(paths)
	if err != nil {
		return err
	}
	w.app.onMain(func() {
		for _, path := range mutation.Trashed {
			w.app.clearTreeCache()
			if filepath.Clean(path) == filepath.Clean(w.app.documentPath()) {
				w.app.resetDoc()
			}
		}
		w.app.invalidate()
	})
	w.refresh(mutation.Trashed...)
	return nil
}

func (w *appAssistantWorkspace) CreateFolder(path string) error {
	if w.app == nil {
		return applicationvault.ErrFileOperationsUnavailable
	}
	root := ""
	w.app.onMain(func() { root = w.app.root })
	files := applicationvault.NewFileUseCases(root, w.app.fileOperations, w.app.isVaultDirectory)
	if _, err := files.CreateFolderAt(path); err != nil {
		return err
	}
	w.app.onMain(func() {
		w.app.clearTreeCache()
		w.app.invalidate()
	})
	w.refresh(path)
	return nil
}

func (w *appAssistantWorkspace) Exists(path string) (bool, error) {
	if w.Directory == nil {
		return false, errors.New("workspace directory operations are unavailable")
	}
	return w.Directory.Exists(path)
}

func (w *appAssistantWorkspace) refresh(paths ...string) {
	var vault *applicationvault.Session
	w.app.onMain(func() { vault = w.app.vault })
	if vault != nil {
		_ = vault.Refresh(paths...)
	}
}

var _ applicationagent.WorkspaceMutationPort = (*appAssistantWorkspace)(nil)
