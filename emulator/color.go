package emulator

import (
	"fmt"
	"strconv"
)

type Color uint32

type (
	ColorMode uint32
	ColorType uint32
)

const (
	mask256color Color = 0xff
	maskRGBcolor Color = 0xffffff // 24-bit

	ColDefault Color = mask256color + 1 // outside the range
	ColBlack   Color = 0
	ColRed     Color = 1
	ColGreen   Color = 2
	ColYellow  Color = 3
	ColBlue    Color = 4
	ColMagenta Color = 5
	ColCyan    Color = 6
	ColWhite   Color = 7

	ModeBold      ColorMode = 1 << (0 + 24)
	ModeDim       ColorMode = 1 << (1 + 24)
	ModeItalic    ColorMode = 1 << (2 + 24)
	ModeUnderline ColorMode = 1 << (3 + 24)
	ModeBlink     ColorMode = 1 << (4 + 24) // or
	ModeReverse   ColorMode = 1 << (5 + 24)
	ModeInvisible ColorMode = 1 << (6 + 24)

	colorTypeShift           = 7 + 24
	colorTypeMask            = 1 << colorTypeShift
	ColorType256   ColorType = 0
	ColorTypeRGB   ColorType = 1
)

var Colors8 = []Color{
	ColBlack,
	ColRed,
	ColGreen,
	ColYellow,
	ColBlue,
	ColMagenta,
	ColCyan,
	ColWhite,
}

var ColorModes = []ColorMode{
	ModeBold,
	ModeDim,
	ModeItalic,
	ModeUnderline,
	ModeBlink,
	ModeReverse,
	ModeInvisible,
}

var colorMasks = []Color{
	mask256color,
	maskRGBcolor,
}

func (c Color) SetColor(color Color) Color {
	c &= ^mask256color
	color &= mask256color
	return (c | color) & ^Color(colorTypeMask)
}

func (c Color) SetColorRGB(r int, g int, b int) Color {
	r &= 0xff
	g &= 0xff
	b &= 0xff
	c &= ^maskRGBcolor
	return (c | Color(r<<16|g<<8|b)) | Color(colorTypeMask)
}

func (c Color) Color() int {
	t := c.ColorType()
	mask := colorMasks[int(t)]
	return int(c & mask)
}

func (c Color) ColorRGB() (int, int, int) {
	i := c.Color()
	return i >> 16, (i >> 8) & 0xff, i & 0xff
}

func (c Color) ColorType() ColorType {
	return ColorType(c&colorTypeMask) >> colorTypeShift
}

func (c Color) SetMode(mode ColorMode) Color {
	return c | Color(mode)
}

func (c Color) ResetMode(mode ColorMode) Color {
	return c & ^Color(mode)
}

func (c Color) ResetModes() Color {
	result := c
	for _, m := range ColorModes {
		result = result.ResetMode(m)
	}
	return result
}

func (c Color) TestMode(mode ColorMode) bool {
	return c&Color(mode) != 0
}

func (c Color) Modes() []ColorMode {
	count := 0
	for _, m := range ColorModes {
		if c.TestMode(m) {
			count += 1
		}
	}

	modes := make([]ColorMode, count)
	i := 0
	for _, m := range ColorModes {
		if c.TestMode(m) {
			modes[i] = m
			i++
		}
	}
	return modes
}

const ESC byte = 27

// only set color, no escape, no modes
func ansiEscapeColor(c Color, ctype byte) []byte {
	var seq []byte

	color := c.Color()
	colorStr := strconv.Itoa(color)
	t := c.ColorType()
	switch t {
	case ColorType256:
		if color < 8 {
			seq = append(seq, ESC, '[', ctype, byte(colorStr[0]), 'm')
		} else if c == ColDefault {
			// ANSIEscape below already resets colors, so we don't have to do anything here
		} else {
			seq = append(seq, ESC, '[', ctype, '8', ';', '5', ';')
			seq = append(seq, []byte(colorStr+"m")...)
		}
	case ColorTypeRGB:
		seq = append(seq, ESC, '[', ctype, '8', ';', '2', ';')
		r, g, b := c.ColorRGB()
		seq = append(seq, fmt.Append(nil, r, ';', g, ';', b, 'm')...)
	}
	return seq
}

// ANSIEscape sequence to set this color
func ANSIEscape(fg Color, bg Color) []byte {
	seq := []byte{ESC, '[', '0', 'm'}
	for i, m := range ColorModes {
		if fg.TestMode(m) {
			seq = append(seq, ESC, '[')
			seq = append(seq, []byte(strconv.Itoa(1+i))...)
			seq = append(seq, 'm')
		}
	}

	seq = append(seq, ansiEscapeColor(fg, '3')...)
	seq = append(seq, ansiEscapeColor(bg, '4')...)

	return seq
}

