package render

import "github.com/egoist/mygo/ui"

// ResolveInlineStyle turns detached Markdown/HTML inline semantics into the
// concrete MyGo text span used by a visible frame. Theme resolution remains
// with the host; style precedence and typography rules live here.
func ResolveInlineStyle(base Style, st InlineStyle, size float32) ui.Span {
	span := ui.Span{Color: base.Text}
	if st.Heading {
		span.Color = base.Heading
	}
	if st.Muted {
		span.Color = base.Text.Mix(base.Muted, .25)
		if st.Strike {
			span.Color = base.Muted
		}
	}
	if st.Bold {
		span.Weight = 600
	}
	if st.Italic {
		span.Italic = true
	}
	if st.Strike {
		span.Strikethrough = true
		span.Color = base.Text.Mix(base.Muted, .4)
	}
	if st.Under {
		span.Underline = true
	}
	if st.Link {
		span.Color = base.Link
		span.Underline = true
		if st.LinkInvalid {
			span.Color = span.Color.Mix(base.Muted, .45)
			span.DottedUnderline = true
		}
		if st.LinkHover {
			span.Color = base.LinkHover
		}
	}
	if st.Mark {
		span.Background = base.MarkBackground
	}
	if st.Code {
		span.Font = base.Mono
		span.Size = size * .875
		if st.CodeJoined {
			span.Background = ui.Color{}
		} else {
			span.Background = base.InlineCodeBackground
			span.Radius = max(3, size/5)
			span.PaddingX = size * .2
			span.PaddingY = size * .1
		}
		if !st.Link {
			span.Color = base.InlineCodeText
		}
	}
	if st.Kbd {
		span.Font = base.Mono
		span.Size = size * .85
		span.Background = base.InlineCodeBackground
		span.Radius = 3
	}
	if st.Mono {
		span.Font = base.Mono
		span.Size = size * .9
	}
	if st.Color.A != 0 {
		span.Color = st.Color
	}
	if st.Background.A != 0 {
		span.Background = st.Background
	}
	if st.Sup || st.Sub {
		span.Size = size * .75
		span.BaselineShift = size * .35
		if st.Sub {
			span.BaselineShift = -size * .2
		}
	}
	if st.Marker {
		if !st.CodeJoined {
			span.Font = base.Font
			span.Background = ui.Color{}
			span.Radius = 0
			span.PaddingX, span.PaddingY = 0, 0
		}
		span.Italic = false
		span.Color = base.Marker
		span.Underline, span.Strikethrough = false, false
	}
	return span
}
