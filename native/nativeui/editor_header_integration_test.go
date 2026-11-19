package nativeui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	noteeditor "muse/nativeui/workbench/features/editor/surface"
	propertypanel "muse/nativeui/workbench/features/properties"
)

func TestEditorHeaderPropertiesAvailability(t *testing.T) {
	for _, test := range []struct {
		name     string
		markdown bool
		want     bool
	}{
		{name: "markdown", markdown: true, want: true},
		{name: "unsupported document", markdown: false, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			panel := propertypanel.NewHost(propertypanel.Ports{
				Project: func(map[string]propertypanel.FieldPreference) propertypanel.Projection {
					return propertypanel.Projection{Markdown: test.markdown}
				},
			})
			app := &App{propertiesPanel: panel}
			var model noteeditor.HeaderModel
			tester := ui.NewTester(func(ctx *ui.Context) {
				model = app.editorHeaderModel(ctx, false)
			}, 800, 600)
			tester.Frame()
			if model.Properties != test.want {
				t.Fatalf("Properties = %v, want %v", model.Properties, test.want)
			}
		})
	}
}
