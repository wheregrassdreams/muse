// Package files coordinates native file and folder chooser requests.
package files

import "context"

type SelectionKind uint8

const (
	SelectFile SelectionKind = iota + 1
	SelectFolder
)

// Request is a backend-neutral native chooser request.
type Request struct {
	Kind        SelectionKind
	Title       string
	FilterName  string
	DefaultPath string
	Extensions  []string
}

// Ports keep platform dialogs and shell scheduling outside this feature.
type Ports struct {
	Busy       func() bool
	Choose     func(context.Context, Request) ([]string, error)
	ChooseSave func(context.Context, Request) (string, error)
	Start      func()
	Finish     func(func())
	OnFile     func(string)
	OnFolder   func(string)
	OnSave     func(string)
}

type Host struct{ ports Ports }

func NewHost(ports Ports) *Host { return &Host{ports: ports} }

func (h *Host) OpenFile(request Request) {
	request.Kind = SelectFile
	h.open(request)
}

func (h *Host) OpenFolder(request Request) {
	request.Kind = SelectFolder
	h.open(request)
}

func (h *Host) SaveFile(request Request) {
	if h == nil || h.ports.ChooseSave == nil || h.ports.Finish == nil ||
		(h.ports.Busy != nil && h.ports.Busy()) {
		return
	}
	if h.ports.Start != nil {
		h.ports.Start()
	}
	go func() {
		path, err := h.ports.ChooseSave(context.Background(), request)
		h.ports.Finish(func() {
			if err == nil && path != "" && h.ports.OnSave != nil {
				h.ports.OnSave(path)
			}
		})
	}()
}

func (h *Host) open(request Request) {
	if h == nil || h.ports.Choose == nil || h.ports.Finish == nil ||
		(h.ports.Busy != nil && h.ports.Busy()) {
		return
	}
	if h.ports.Start != nil {
		h.ports.Start()
	}
	go func() {
		paths, err := h.ports.Choose(context.Background(), request)
		h.ports.Finish(func() {
			if err != nil || len(paths) == 0 || paths[0] == "" {
				return
			}
			if request.Kind == SelectFolder {
				if h.ports.OnFolder != nil {
					h.ports.OnFolder(paths[0])
				}
				return
			}
			if h.ports.OnFile != nil {
				h.ports.OnFile(paths[0])
			}
		})
	}()
}
