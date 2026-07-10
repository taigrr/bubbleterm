package emulator

import "image/color"

// CursorStyle represents a cursor shape.
type CursorStyle int

const (
	CursorBlock CursorStyle = iota
	CursorUnderline
	CursorBar
)

// ChangeReason says what kind of change caused the region to change, for optimization etc.
type ChangeReason int

const (
	// CRText means text is being printed normally.
	CRText ChangeReason = iota

	// CRClear means some area has been cleared
	CRClear

	// CRScroll means an area has been scrolled
	CRScroll

	// CRScreenSwitch means the screen has been switched between main and alt
	CRScreenSwitch

	// CRRedraw means the application requested a redraw with RedrawAll
	CRRedraw
)

// LineDamage represents a changed region on a single row.
type LineDamage struct {
	Row    int
	X1     int
	X2     int
	Reason ChangeReason
}

// Pos represents a position on the screen
type Pos struct {
	X int
	Y int
}

// CursorAppearance holds the cursor shape, blink state, and color
// as set by the running application via DECSCUSR and OSC 12.
type CursorAppearance struct {
	Style CursorStyle
	Blink bool
	// Color is the cursor color set via OSC 12. Nil means no color has been
	// explicitly set. Note: after an OSC 112 reset, the upstream vt package
	// returns the default color (typically white) rather than nil.
	Color color.Color
}
