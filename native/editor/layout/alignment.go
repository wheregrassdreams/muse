// Package layout defines toolkit-independent viewport placement policy.
package layout

// PageJumpAlignment describes how a source navigation target should land in
// the viewport. It is independent of the input command that initiated it.
type PageJumpAlignment uint8

const (
	PageJumpNearest PageJumpAlignment = iota
	PageJumpTopMargin
	PageJumpCenter
	PageJumpTop
	PageJumpBottom
)
