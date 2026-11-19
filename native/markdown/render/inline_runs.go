package render

import editorrender "github.com/wheregrassdreams/muse-editor/markdown/render"

func InlineRuns(src string) []InlineRun { return editorrender.InlineRuns(src) }
