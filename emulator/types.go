package emulator

// Region is a non-inclusive rectangle on the screen. X2 and Y2 are not included in the region.
type Region struct {
	X, Y, X2, Y2 int
}

func (r Region) Add(x, y int) Region {
	return Region{
		X:  r.X + x,
		X2: r.X2 + x,
		Y:  r.Y + y,
		Y2: r.Y2 + y,
	}
}

func clamp(v int, low, high int) int {
	if v < low {
		v = low
	}
	if v > high {
		v = high
	}
	return v
}

func (r Region) Clamp(rc Region) Region {
	nx := clamp(r.X, rc.X, rc.X2)
	ny := clamp(r.Y, rc.Y, rc.Y2)
	return Region{
		X:  nx,
		X2: clamp(r.X2, nx, rc.X2),
		Y:  ny,
		Y2: clamp(r.Y2, ny, rc.Y2),
	}
}

// ViewFlag is an enum of boolean flags on a terminal
type ViewFlag int

const (
	VFBlinkCursor ViewFlag = iota
	VFShowCursor
	VFReportFocus
	VFBracketedPaste
	viewFlagCount
)

// ViewInt is an enum of integer settings on a terminal
type ViewInt int

const (
	VIMouseMode ViewInt = iota
	VIMouseEncoding
	viewIntCount
)

// ViewString is an enum of string settings on a terminal
type ViewString int

const (
	VSWindowTitle ViewString = iota
	VSCurrentDirectory
	VSCurrentFile
	viewStringCount
)

// Mouse modes for VIMouseMode
const (
	MMNone int = iota
	MMPress
	MMPressRelease
	MMPressReleaseMove
	MMPressReleaseMoveAll
)

// Mouse encodings for VIMouseEncoding
const (
	MEX10 int = iota
	MEUTF8
	MESGR
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

// Pos represents a position on the screen
type Pos struct {
	X int
	Y int
}

// Line holds a list of text blocks with associated colors
type Line struct {
	Spans []StyledSpan
	Text  []rune
	Width uint32
}

// StyledSpan has style colors, and a width
type StyledSpan struct {
	FG, BG Color
	// todo: should distinguish between width of characters on screen
	// and length in terms of number of runes
	Width uint32
}

func (l *Line) Append(text string, fg Color, bg Color) {
	runes := []rune(text)
	l.AppendRunes(runes, fg, bg)
}

func (l *Line) AppendRunes(runes []rune, fg Color, bg Color) {
	l.Text = append(l.Text, runes...)
	l.Spans = append(l.Spans,
		StyledSpan{
			FG:    fg,
			BG:    bg,
			Width: uint32(len(runes)),
		})
	l.Width += uint32(len(runes))
}

func (l *Line) Repeat(r rune, rep uint, fg Color, bg Color) {
	runes := make([]rune, rep)
	for i := range runes {
		runes[i] = r
	}
	l.AppendRunes(runes, fg, bg)
}