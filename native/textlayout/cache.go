package textlayout

import (
	"reflect"
	"slices"

	"github.com/egoist/mygo/ui"
)

// Cache belongs to one editor and is used under its existing frame lock.
// Both entry count and retained text size are capped; it is not a vault index.
type Cache struct {
	entries []cachedLayout
	units   int
}

type cachedLayout struct {
	spans  []ui.Span
	font   ui.Font
	shape  ui.TextShape
	layout *Layout
	units  int
}

func (c *Cache) Clear() { c.entries, c.units = nil, 0 }

func (c *Cache) Shape(spans []ui.Span, font ui.Font, shape ui.TextShape) *Layout {
	for i, entry := range c.entries {
		if entry.font == font && entry.shape == shape && reflect.DeepEqual(entry.spans, spans) {
			copy(c.entries[1:i+1], c.entries[:i])
			c.entries[0] = entry
			return entry.layout
		}
	}
	layout := Shape(spans, font, shape, true)
	units := 0
	for _, span := range spans {
		units += utf16Length(span.Text)
	}
	if units > 4096 {
		return layout
	}
	for len(c.entries) > 0 && (len(c.entries) >= 16 || c.units+units > 16384) {
		last := len(c.entries) - 1
		c.units -= c.entries[last].units
		c.entries[last] = cachedLayout{}
		c.entries = c.entries[:last]
	}
	c.entries = append(c.entries, cachedLayout{})
	copy(c.entries[1:], c.entries[:len(c.entries)-1])
	c.entries[0] = cachedLayout{spans: slices.Clone(spans), font: font, shape: shape, layout: layout, units: units}
	c.units += units
	return layout
}
