// Package paragraph preserves Muse's historical import path for the
// independent Markdown paragraph model and source editing policies.
package paragraph

import model "github.com/wheregrassdreams/muse-editor/markdown/paragraph"

type BlankInput = model.BlankInput
type BlankInputPlan = model.BlankInputPlan
type BlockRange = model.BlockRange
type BlockShortcutInput = model.BlockShortcutInput
type BlockShortcutPlan = model.BlockShortcutPlan
type BreakProjection = model.BreakProjection
type Breaks = model.Breaks
type ContainerKind = model.ContainerKind
type Edit = model.Edit
type FoldOperation = model.FoldOperation
type FoldResult = model.FoldResult
type FoldSection = model.FoldSection
type HeadingInput = model.HeadingInput
type HeadingPlan = model.HeadingPlan
type HeadingRange = model.HeadingRange
type HeadingShape = model.HeadingShape
type LinePlan = model.LinePlan
type ListAction = model.ListAction
type ListInput = model.ListInput
type ListPlan = model.ListPlan

const (
	FoldToggle          = model.FoldToggle
	FoldClose           = model.FoldClose
	FoldOpen            = model.FoldOpen
	FoldToggleRecursive = model.FoldToggleRecursive
	FoldCloseRecursive  = model.FoldCloseRecursive
	FoldOpenRecursive   = model.FoldOpenRecursive
	FoldReveal          = model.FoldReveal
	FoldCloseAll        = model.FoldCloseAll
	FoldOpenAll         = model.FoldOpenAll

	ParagraphText = model.ParagraphText
	ATXHeading    = model.ATXHeading
	SetextHeading = model.SetextHeading

	ListNoop  = model.ListNoop
	ListLeave = model.ListLeave
	ListEdit  = model.ListEdit

	ListContainer  = model.ListContainer
	QuoteContainer = model.QuoteContainer
)

var (
	ApplyFold             = model.ApplyFold
	ApplyFoldAll          = model.ApplyFoldAll
	ApplyFoldAtOffset     = model.ApplyFoldAtOffset
	BlankListMarkers      = model.BlankListMarkers
	BlankParagraphPrefix  = model.BlankParagraphPrefix
	BlockShortcut         = model.BlockShortcut
	BuildFoldSections     = model.BuildFoldSections
	ContinuationPrefix    = model.ContinuationPrefix
	ExpandFoldsAt         = model.ExpandFoldsAt
	ExpandFoldsForRange   = model.ExpandFoldsForRange
	FoldedContains        = model.FoldedContains
	FoldSectionAtOffset   = model.FoldSectionAtOffset
	HeadingContentStart   = model.HeadingContentStart
	MarkerWidth           = model.MarkerWidth
	PlanBlankInput        = model.PlanBlankInput
	PlanBlockShortcut     = model.PlanBlockShortcut
	PlanHeading           = model.PlanHeading
	PlanIndentLines       = model.PlanIndentLines
	PlanListToggle        = model.PlanListToggle
	PlanOutdentLines      = model.PlanOutdentLines
	PlanUnquote           = model.PlanUnquote
	ShiftHeadingLevel     = model.ShiftHeadingLevel
	ShouldProcessInput    = model.ShouldProcessInput
	ShouldTrackBlankInput = model.ShouldTrackBlankInput
	SourcePrefix          = model.SourcePrefix
)
