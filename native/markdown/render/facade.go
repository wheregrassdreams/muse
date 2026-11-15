package render

// InlineRun is a backend-neutral visible inline run with source mapping.
type InlineRun struct {
	Text        string
	Bold        bool
	Italic      bool
	Code        bool
	Strike      bool
	Link        string
	Math        bool
	Display     string // rendered-for-editing text; empty means Text
	SourceStart int
	SourceEnd   int
}
