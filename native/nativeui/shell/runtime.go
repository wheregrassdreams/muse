// Package shell owns the MyGo application and window lifecycle. Product
// workflows and feature construction are supplied through narrow callbacks.
package shell

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type CloseDecision uint8

const (
	CloseProceed CloseDecision = iota
	ClosePrevent
	CloseConfirm
)

// CloseResolution is the explicit user choice from the unsaved-changes
// dialog. Save is committed by the application save workflow before close;
// Discard closes immediately; Cancel keeps the window open.
type CloseResolution uint8

const (
	CloseCancel CloseResolution = iota
	CloseSave
	CloseDiscard
)

func ApproveClose(resolution CloseResolution, save func() error) bool {
	switch resolution {
	case CloseDiscard:
		return true
	case CloseSave:
		return save != nil && save() == nil
	default:
		return false
	}
}

// CloseGate models the one-shot permit required by native window APIs that
// emit another close event after a confirmation callback closes the workspaceui.
type CloseGate struct {
	mu        sync.Mutex
	pending   bool
	confirmed bool
}

func (g *CloseGate) Request(dirty, busy bool) CloseDecision {
	if g == nil {
		return CloseProceed
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending {
		return ClosePrevent
	}
	if g.confirmed {
		g.confirmed = false
		return CloseProceed
	}
	if !dirty {
		return CloseProceed
	}
	if busy {
		return ClosePrevent
	}
	g.pending = true
	return CloseConfirm
}

func (g *CloseGate) Finish(approved bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.pending = false
	g.confirmed = approved
	g.mu.Unlock()
}

type RuntimeOptions struct {
	Name                 string
	Title                string
	Width, Height        int
	MinWidth, MinHeight  int
	BackgroundColor      string
	StateKey             string
	Transparent          bool
	Vibrancy             mygo.Vibrancy
	TitleBarStyle        mygo.TitleBarStyle
	TitleBarHeight       int
	TrafficLightPosition *mygo.Point
	HideWindowButtons    bool
	View                 func(*ui.Context)
	InstallMenu          func(*mygo.Window)
	OnReady              func()
	OnActivate           func(hasVisibleWindows bool)
	OnLocaleChanged      func(string)
	OnThemeUpdated       func()
	OnWindowAllClosed    func()
	OnWillQuit           func(*mygo.QuitEvent)
	IsDirty              func() bool
	IsBusy               func() bool
	CloseDecision        func() CloseDecision
	CompleteClose        func(approved bool)
	ConfirmClose         func(done func(CloseResolution))
	SaveBeforeClose      func() error
	OnWindowClose        func()
	OnRunExit            func()
	OnMaximize           func()
	UpdateTitle          func(*mygo.Window)
	CapturePath          string
	CaptureDelay         time.Duration
}

type Runtime struct {
	options RuntimeOptions
	window  *mygo.Window
}

func NewRuntime(options RuntimeOptions) *Runtime { return &Runtime{options: options} }

func (r *Runtime) Run() error {
	if r == nil {
		return nil
	}
	if r.options.Name != "" {
		mygo.App.SetName(r.options.Name)
	}
	if r.options.OnLocaleChanged != nil {
		mygo.App.OnLocaleChanged(r.options.OnLocaleChanged)
	}
	if r.options.OnThemeUpdated != nil {
		mygo.Theme.OnUpdated(r.options.OnThemeUpdated)
	}
	mygo.App.WhenReady(func() {
		if r.options.OnReady != nil {
			r.options.OnReady()
		}
		r.openWindow()
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if r.options.OnActivate != nil {
			r.options.OnActivate(hasVisibleWindows)
		}
		if !hasVisibleWindows {
			r.openWindow()
		}
	})
	mygo.App.OnWindowAllClosed(func() {
		if r.options.OnWindowAllClosed != nil {
			r.options.OnWindowAllClosed()
		} else if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	if r.options.OnWillQuit != nil {
		mygo.App.OnWillQuit(r.options.OnWillQuit)
	}
	err := mygo.App.Run()
	if r.options.OnRunExit != nil {
		r.options.OnRunExit()
	}
	return err
}

func (r *Runtime) Window() *mygo.Window {
	if r == nil {
		return nil
	}
	return r.window
}

func (r *Runtime) OpenWindow() *mygo.Window { return r.openWindow() }

func (r *Runtime) openWindow() *mygo.Window {
	if r == nil {
		return nil
	}
	if r.window != nil && !r.window.IsDestroyed() {
		r.window.Show()
		r.window.Focus()
		return r.window
	}
	o := r.options
	r.window = mygo.NewWindow(mygo.WindowOptions{
		Title: o.Title, Width: o.Width, Height: o.Height,
		MinWidth: o.MinWidth, MinHeight: o.MinHeight,
		BackgroundColor: o.BackgroundColor, StateKey: o.StateKey,
		Content: ui.View(o.View), Transparent: o.Transparent, Vibrancy: o.Vibrancy,
		TitleBarStyle: o.TitleBarStyle, TitleBarHeight: o.TitleBarHeight,
		TrafficLightPosition: o.TrafficLightPosition,
		HideWindowButtons:    o.HideWindowButtons,
	})
	if o.InstallMenu != nil {
		o.InstallMenu(r.window)
	}
	if o.UpdateTitle != nil {
		o.UpdateTitle(r.window)
	}
	if o.OnMaximize != nil {
		r.window.OnMaximize(o.OnMaximize)
	}
	r.window.OnClose(func(event *mygo.CloseEvent) {
		decision := CloseProceed
		if o.CloseDecision != nil {
			decision = o.CloseDecision()
		} else if o.IsDirty != nil && o.IsDirty() {
			busy := o.IsBusy != nil && o.IsBusy()
			if busy {
				decision = ClosePrevent
			} else {
				decision = CloseConfirm
			}
		}
		switch decision {
		case ClosePrevent:
			event.PreventDefault()
		case CloseConfirm:
			event.PreventDefault()
			if o.ConfirmClose != nil {
				go o.ConfirmClose(func(resolution CloseResolution) {
					mygo.RunOnMain(func() {
						approved := ApproveClose(resolution, o.SaveBeforeClose)
						if !approved {
							if o.CompleteClose != nil {
								o.CompleteClose(false)
							}
							r.window.Invalidate()
							return
						}
						if o.CompleteClose != nil {
							o.CompleteClose(true)
						}
						r.window.Close()
					})
				})
			}
		case CloseProceed:
			if o.OnWindowClose != nil {
				o.OnWindowClose()
			}
		}
	})
	r.window.Show()
	r.window.Focus()
	mygo.App.Focus()
	if o.CapturePath != "" {
		delay := o.CaptureDelay
		if delay <= 0 {
			delay = 1200 * time.Millisecond
		}
		go func(window *mygo.Window) {
			time.Sleep(delay)
			if png, err := window.CapturePage(); err == nil {
				_ = os.WriteFile(o.CapturePath, png, 0o644)
			}
			mygo.App.Exit(0)
		}(r.window)
	}
	return r.window
}

func CaptureDelayFromEnv() time.Duration {
	if value := os.Getenv("MUSE_CAPTURE_DELAY_MS"); value != "" {
		if ms, err := strconv.Atoi(value); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 1200 * time.Millisecond
}
