// Package inline preserves Muse's historical import path for the independent
// Markdown inline source model.
package inline

import model "github.com/wheregrassdreams/muse-editor/markdown/inline"

type AutoPairInput = model.AutoPairInput
type FormatPlan = model.FormatPlan
type HighlightMark = model.HighlightMark
type LinkCandidate = model.LinkCandidate
type LinkCandidateKind = model.LinkCandidateKind
type MarkerKind = model.MarkerKind
type MathSession = model.MathSession
type Range = model.Range
type WikiLink = model.WikiLink

const (
	LinkCandidateText  = model.LinkCandidateText
	LinkCandidateLink  = model.LinkCandidateLink
	LinkCandidateImage = model.LinkCandidateImage

	MarkerGeneric       = model.MarkerGeneric
	MarkerCode          = model.MarkerCode
	MarkerEmphasis      = model.MarkerEmphasis
	MarkerStrong        = model.MarkerStrong
	MarkerStrikethrough = model.MarkerStrikethrough
)

var (
	AllHighlightRanges = model.AllHighlightRanges
	HighlightRanges    = model.HighlightRanges
	IsImageName        = model.IsImageName
	LinkDestination    = model.LinkDestination
	LinkRangeAt        = model.LinkRangeAt
	LinkTargetAt       = model.LinkTargetAt
	MarkerLengths      = model.MarkerLengths
	MathContentBounds  = model.MathContentBounds
	ParseWikiLinks     = model.ParseWikiLinks
	PlanAutoPair       = model.PlanAutoPair
	PlanAutoPairAt     = model.PlanAutoPairAt
	PlanLink           = model.PlanLink
	ShouldSkipCloser   = model.ShouldSkipCloser
	ShouldSkipCloserAt = model.ShouldSkipCloserAt
	TagAt              = model.TagAt
	UnwrapSelection    = model.UnwrapSelection
	WikiLinkAt         = model.WikiLinkAt
	WrapAtCaret        = model.WrapAtCaret
	WrapPairFor        = model.WrapPairFor
	WrapSelection      = model.WrapSelection
)
