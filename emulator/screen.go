package emulator

import (
	"bytes"
	"fmt"
	"strings"
)

// lineDamageState tracks per-line dirty state.
type lineDamageState struct {
	dirty  bool
	x1, x2 int
	reason ChangeReason
}

type screen struct {
	chars       [][]rune
	backColors  [][]Color
	frontColors [][]Color

	frontColor Color
	backColor  Color

	// preallocated for fast copying
	frontColorBuf []Color
	backColorBuf  []Color

	size Pos

	cursorPos Pos

	topMargin, bottomMargin int

	autoWrap bool

	// damage tracks which lines have changed since the last consume.
	damage []lineDamageState
}

func newScreen(cols, rows int) *screen {
	s := &screen{}
	s.setSize(cols, rows)
	s.setColors(ColWhite, ColBlack)
	s.bottomMargin = s.size.Y - 1
	s.eraseRegion(Region{X: 0, Y: 0, X2: s.size.X, Y2: s.size.Y}, CRClear)
	s.markDamageAll(CRRedraw)
	return s
}

func (s *screen) getLine(y int) []rune {
	if y >= len(s.chars) {
		return nil
	}
	return s.chars[y]
}

func (s *screen) GetLineColors(y int) ([]Color, []Color) {
	if y >= len(s.frontColors) {
		return nil, nil
	}
	return s.frontColors[y], s.backColors[y]
}

func (s *screen) StyledLine(x, w, y int) *Line {
	if y >= len(s.chars) {
		return &Line{}
	}

	text := s.getLine(y)
	fgs := s.frontColors[y]
	bgs := s.backColors[y]

	var spans []StyledSpan

	if w < 0 || x+w > len(fgs) {
		w = len(fgs) - x
	}
	if w <= 0 {
		return &Line{}
	}

	for i := x; i < x+w; {
		fg := fgs[i]
		bg := bgs[i]
		width := uint32(1)
		i++

		for i < x+w && fg == fgs[i] && bg == bgs[i] {
			i++
			width++
		}
		spans = append(spans, StyledSpan{fg, bg, width})
	}
	return &Line{
		Spans: spans,
		Text:  append([]rune(nil), text[x:x+w]...), // copy
		Width: uint32(w),
	}
}

func (s *screen) StyledLines(r Region) []*Line {
	var lines []*Line
	for y := r.Y; y < r.Y2; y++ {
		lines = append(lines, s.StyledLine(r.X, r.X2-r.X, y))
	}
	return lines
}

func (s *screen) renderLineANSI(y int) string {
	if y >= len(s.chars) {
		return ""
	}

	line := s.getLine(y)
	if len(line) == 0 {
		return "\033[0m" // Reset and return empty line
	}

	var fg, bg Color
	buf := bytes.NewBuffer(make([]byte, 0, len(line)+10))
	x := 0
	for x < len(line) {
		fg = s.frontColors[y][x]
		bg = s.backColors[y][x]
		buf.Write(ANSIEscape(fg, bg))

		for x < len(line) && fg == s.frontColors[y][x] && bg == s.backColors[y][x] {
			buf.WriteRune(line[x])
			x++
		}
	}

	// Reset colors at the end of each line to prevent bleeding
	buf.WriteString("\033[0m")

	return buf.String()
}

func (s *screen) setColors(front Color, back Color) {
	s.frontColor = front
	s.backColor = back

	for i := range s.frontColorBuf {
		s.frontColorBuf[i] = front
	}
	for i := range s.backColorBuf {
		s.backColorBuf[i] = back
	}
}

func (s *screen) setSize(w, h int) {
	if w <= 0 || h <= 0 {
		panic("Size must be > 0")
	}

	// resize screen. copy current screen to upper-left corner of new screen

	minW := min(w, s.size.X)

	rect := make([][]rune, h)
	raw := make([]rune, w*h)
	for i := range rect {
		rect[i], raw = raw[:w], raw[w:]
		if i < s.size.Y {
			copy(rect[i][:minW], s.chars[i][:minW])

			for x := minW; x < w; x++ {
				rect[i][x] = ' '
			}
		} else {
			for x := range w {
				rect[i][x] = ' '
			}
		}
	}
	s.chars = rect

	for pi, p := range []*[][]Color{&s.backColors, &s.frontColors} {
		col := s.backColor
		if pi == 1 {
			col = s.frontColor
		}

		rect := make([][]Color, h)
		raw := make([]Color, w*h)
		for i := range rect {
			rect[i], raw = raw[:w], raw[w:]
			if i < s.size.Y {
				copy(rect[i][:minW], (*p)[i][:minW])

				for x := minW; x < w; x++ {
					rect[i][x] = col
				}
			} else {
				for x := range w {
					rect[i][x] = col
				}
			}
		}
		*p = rect
	}

	s.bottomMargin = h - (s.size.Y - s.bottomMargin)

	s.size = Pos{X: w, Y: h}

	// TODO: Logic for cursor position on resize?
	if s.cursorPos.X > w {
		s.cursorPos.X = 0
	}
	if s.cursorPos.Y > h {
		s.cursorPos.Y = 0
	}

	s.frontColorBuf = make([]Color, w)
	s.backColorBuf = make([]Color, w)
	s.damage = make([]lineDamageState, h)
	s.setColors(s.frontColor, s.backColor)
}

func (s *screen) eraseRegion(r Region, cr ChangeReason) {
	r = s.clampRegion(r)
	bytes := make([]rune, r.X2-r.X)
	for i := range bytes {
		bytes[i] = ' '
	}
	for i := r.Y; i < r.Y2; i++ {
		s.rawWriteRunes(r.X, i, bytes, cr)
	}
}

// This is a very raw write function. It wraps as necessary, but assumes all
// the bytes are printable bytes
func (s *screen) writeRunes(b []rune) {
	for len(b) > 0 {
		l := min(s.size.X-s.cursorPos.X, len(b))

		s.rawWriteRunes(s.cursorPos.X, s.cursorPos.Y, b[:l], CRText)
		b = b[l:]
		s.moveCursor(l, 0, true, true)
	}
}

// This is a very raw write function. It assumes all the bytes are printable bytes
// If you use this to write beyond the end of the line, it will panic.
func (s *screen) rawWriteRunes(x int, y int, b []rune, reason ChangeReason) {
	if y >= s.size.Y || x+len(b) > s.size.X {
		fmt.Printf("rawWriteRunes out of range: %v  %v,%v,%v %v %#v, %v,%v\n", s.size, x, y, x+len(b), len(b), string(b), len(s.chars), len(s.chars[0]))
		return
	}
	copy(s.chars[y][x:x+len(b)], b)
	s.rawWriteColors(y, x, x+len(b))
	s.markDamageLine(y, x, x+len(b), reason)
}

// rawWriteColors copies one line of current colors to the screen, from x1 to x2
func (s *screen) rawWriteColors(y int, x1 int, x2 int) {
	copy(s.frontColors[y][x1:x2], s.frontColorBuf[x1:x2])
	copy(s.backColors[y][x1:x2], s.backColorBuf[x1:x2])
}

func (s *screen) setCursorPos(x, y int) {
	s.cursorPos.X = clamp(x, 0, s.size.X-1)
	s.cursorPos.Y = clamp(y, 0, s.size.Y-1)
}

func (s *screen) setScrollMarginTopBottom(top, bottom int) {
	s.topMargin = clamp(top, 0, s.size.Y-1)
	s.bottomMargin = clamp(bottom, 0, s.size.Y-1)
}

func (s *screen) scroll(y1 int, y2 int, dy int) {
	y1 = clamp(y1, 0, s.size.Y-1)
	y2 = clamp(y2, 0, s.size.Y-1)

	if y1 > y2 {
		fmt.Printf("scroll ys out of order %d %d %d\n", y1, y2, dy)
		return
	}

	if dy > 0 {
		s.markDamageRegion(Region{X: 0, Y: y1, X2: s.size.X, Y2: y2 + 1}, CRScroll)
		for y := y2; y >= y1+dy; y-- {
			copy(s.chars[y], s.chars[y-dy])
			copy(s.frontColors[y], s.frontColors[y-dy])
			copy(s.backColors[y], s.backColors[y-dy])
		}
		s.eraseRegion(Region{Y: y1, Y2: y1 + dy, X: 0, X2: s.size.X}, CRScroll)
	} else {
		s.markDamageRegion(Region{X: 0, Y: y1, X2: s.size.X, Y2: y2 + 1}, CRScroll)
		for y := y1; y <= y2+dy; y++ {
			copy(s.chars[y], s.chars[y-dy])
			copy(s.frontColors[y], s.frontColors[y-dy])
			copy(s.backColors[y], s.backColors[y-dy])
		}
		s.eraseRegion(Region{Y: y2 + dy + 1, Y2: y2 + 1, X: 0, X2: s.size.X}, CRScroll)
	}
}

func (s *screen) clampRegion(r Region) Region {
	r.X = clamp(r.X, 0, s.size.X)
	r.Y = clamp(r.Y, 0, s.size.Y)
	r.X2 = clamp(r.X2, 0, s.size.X)
	r.Y2 = clamp(r.Y2, 0, s.size.Y)
	return r
}

func (s *screen) moveCursor(dx, dy int, wrap bool, scroll bool) {
	if wrap && s.autoWrap {
		s.cursorPos.X += dx
		for s.cursorPos.X < 0 {
			s.cursorPos.X += s.size.X
			s.cursorPos.Y--
		}
		for s.cursorPos.X >= s.size.X {
			s.cursorPos.X -= s.size.X
			s.cursorPos.Y++
		}
	} else {
		s.cursorPos.X += dx
		s.cursorPos.X = clamp(s.cursorPos.X, 0, s.size.X-1)
	}

	s.cursorPos.Y += dy
	if scroll {
		if s.cursorPos.Y < s.topMargin {
			s.scroll(s.topMargin, s.bottomMargin, s.topMargin-s.cursorPos.Y)
			s.cursorPos.Y = s.topMargin
		}
		if s.cursorPos.Y > s.bottomMargin {
			s.scroll(s.topMargin, s.bottomMargin, s.bottomMargin-s.cursorPos.Y)
			s.cursorPos.Y = s.bottomMargin
		}
	} else {
		s.cursorPos.Y = clamp(s.cursorPos.Y, 0, s.size.Y-1)
	}
	if s.cursorPos.Y >= s.size.Y {
		panic(fmt.Sprintf("moveCursor outside, %v %v  %v, %v, %v, %v", s.cursorPos, s.size, dx, dy, wrap, scroll))
	}
}

func (s *screen) PrintScreen() {
	w, h := s.size.X, s.size.Y
	fmt.Print("+")
	for range w {
		fmt.Print("-")
	}
	fmt.Println("+")
	for i := range h {
		lstr := s.renderLineANSI(i)
		lstr = strings.ReplaceAll(lstr, "\000", " ")
		fmt.Printf("\033[m|%s\033[m|\n", lstr)
	}
	fmt.Print("+")
	for range w {
		fmt.Print("-")
	}
	fmt.Println("+")
}

func (s *screen) lineString(y int) string {
	if y >= len(s.chars) {
		return ""
	}
	return string(s.chars[y])
}

func (s *screen) markDamageLine(y, x1, x2 int, reason ChangeReason) {
	if y < 0 || y >= len(s.damage) {
		return
	}
	if x1 < 0 {
		x1 = 0
	}
	if x2 > s.size.X {
		x2 = s.size.X
	}
	if x1 >= x2 {
		return
	}
	d := &s.damage[y]
	if !d.dirty {
		d.dirty = true
		d.x1 = x1
		d.x2 = x2
		d.reason = reason
		return
	}
	if x1 < d.x1 {
		d.x1 = x1
	}
	if x2 > d.x2 {
		d.x2 = x2
	}
	d.reason = reason
}

func (s *screen) markDamageRegion(r Region, reason ChangeReason) {
	r = s.clampRegion(r)
	if r.X == r.X2 || r.Y == r.Y2 {
		return
	}
	for y := r.Y; y < r.Y2; y++ {
		s.markDamageLine(y, r.X, r.X2, reason)
	}
}

func (s *screen) markDamageAll(reason ChangeReason) {
	for y := range s.damage {
		s.damage[y] = lineDamageState{
			dirty:  true,
			x1:     0,
			x2:     s.size.X,
			reason: reason,
		}
	}
}

func (s *screen) consumeDamage() []LineDamage {
	damages := make([]LineDamage, 0, len(s.damage))
	for y, d := range s.damage {
		if !d.dirty {
			continue
		}
		damages = append(damages, LineDamage{
			Row:    y,
			X1:     d.x1,
			X2:     d.x2,
			Reason: d.reason,
		})
		s.damage[y] = lineDamageState{}
	}
	return damages
}
