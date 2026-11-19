package render

import editorrender "github.com/wheregrassdreams/muse-editor/markdown/render"

type HeadingStyle = editorrender.HeadingStyle

func StyleHeading(level int, body float32) HeadingStyle {
	return editorrender.StyleHeading(level, body)
}
