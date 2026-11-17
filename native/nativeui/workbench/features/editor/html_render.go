package editor

import (
	render "muse/nativeui/workbench/features/editor/render"
)

// imgSize is the source-facing image size hint used by the editor's layout
// model. Parsing the HTML tag itself belongs to the render presentation API.
type imgSize struct{ w, h, zoom float32 }

func imgTag(tag string) (src, alt string, size imgSize, ok bool) {
	src, alt, requested, ok := render.HTMLImageTag(tag)
	if !ok {
		return "", "", imgSize{}, false
	}
	return src, alt, imgSize{w: requested.Width, h: requested.Height, zoom: requested.Zoom}, true
}

// htmlTagStyle adapts the editor's inline style flags to the neutral style
// value owned and interpreted by the render package.
func htmlTagStyle(name, tag string, style istyle) istyle {
	styled := render.HTMLTagStyle(name, tag, render.InlineStyle{
		Bold: style.bold, Italic: style.italic, Strike: style.strike, Code: style.code,
		Mono: style.mono, Link: style.link, Tag: style.tag, Under: style.under,
		Mark: style.mark, Sup: style.sup, Sub: style.sub, Kbd: style.kbd,
		LinkHover: style.linkHover, LinkInvalid: style.linkInvalid,
		CodeJoined: style.codeJoined, Marker: style.marker, Muted: style.muted,
		Heading: style.heading, Color: style.color, Background: style.bg,
	})
	style.bold, style.italic, style.strike, style.code = styled.Bold, styled.Italic, styled.Strike, styled.Code
	style.mono, style.link, style.tag, style.under = styled.Mono, styled.Link, styled.Tag, styled.Under
	style.mark, style.sup, style.sub, style.kbd = styled.Mark, styled.Sup, styled.Sub, styled.Kbd
	style.linkHover, style.linkInvalid = styled.LinkHover, styled.LinkInvalid
	style.codeJoined, style.marker, style.muted = styled.CodeJoined, styled.Marker, styled.Muted
	style.heading, style.color, style.bg = styled.Heading, styled.Color, styled.Background
	return style
}
